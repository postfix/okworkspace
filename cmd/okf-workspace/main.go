// Command okf-workspace is the single static binary for OKF Workspace: it
// serves the embedded React SPA and the REST API from one process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
	"github.com/postfix/okworkspace/internal/web"
)

// healthAdapter adapts *gitstore.GitStore to server.HealthChecker without the
// server package importing gitstore.
type healthAdapter struct{ gs *gitstore.GitStore }

func (a healthAdapter) RepoHealth(ctx context.Context) (server.RepoHealth, error) {
	h, err := a.gs.Health(ctx)
	if err != nil {
		return server.RepoHealth{}, err
	}
	return server.RepoHealth{
		OK:         h.OK,
		Diverged:   h.Diverged,
		SelfHealed: h.SelfHealed,
		Detail:     h.Detail,
	}, nil
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "okf-workspace",
		Short: "OKF Workspace — a self-hosted, OKF-native wiki for the agent era",
	}
	root.AddCommand(serveCmd())
	return root
}

func serveCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the OKF Workspace server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
			return runServe(cmd.Context(), logger, configPath)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "config.yaml", "path to config.yaml")
	return cmd
}

// runServe performs the Phase-0 Task-1 startup sequence: load config -> open
// store -> migrate. Full server wiring (bootstrap admin + HTTP listen) is added
// in Task 2/3.
func runServe(ctx context.Context, logger *slog.Logger, configPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Info("config loaded",
		slog.String("config_path", configPath),
		slog.String("listen", cfg.Server.Listen),
		slog.String("data_dir", cfg.Storage.DataDir),
	)

	if cfg.Storage.DataDir != "" {
		if err := os.MkdirAll(cfg.Storage.DataDir, 0o750); err != nil {
			return fmt.Errorf("create data dir %q: %w", cfg.Storage.DataDir, err)
		}
	}

	dbPath := filepath.Join(cfg.Storage.DataDir, "app.db")
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate store: %w", err)
	}
	logger.Info("store ready", slog.String("db_path", dbPath))

	// Bootstrap the admin user on first run (D-01): print the one-time password
	// exactly once. Never logs plaintext on any other path.
	userRepo := users.NewRepository(st.DB())
	adminUser, adminPassword, created, err := users.BootstrapAdmin(ctx, userRepo, cfg)
	if err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	if created {
		logger.Warn("admin user created — save this password, it will NOT be shown again",
			slog.String("username", adminUser),
			slog.String("one_time_password", adminPassword),
			slog.Bool("must_change_password", true),
		)
	}

	// --- Storage + safety spines (Plan 02) ---
	// Startup order (after bootstrap admin, before the HTTP server):
	//   repo/data-dir init -> gitstore.Init -> SelfHealStaleLock ->
	//   (PullOnStartup if remote) -> seed (if new+empty) -> job worker Start.
	repoDir := cfg.Storage.RepoDir
	if repoDir == "" {
		repoDir = filepath.Join(cfg.Storage.DataDir, "repo")
	}
	contentRepo, err := repo.New(repoDir)
	if err != nil {
		return fmt.Errorf("init repo: %w", err)
	}
	defer func() { _ = contentRepo.Close() }()

	gs := gitstore.New(contentRepo, cfg.Git)
	if cfg.Git.Enabled {
		if err := gs.Init(ctx); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
		healed, err := gs.SelfHealStaleLock(ctx)
		if err != nil {
			logger.Warn("git self-heal reported an issue", slog.Any("error", err))
		} else if healed {
			logger.Warn("recovered from an interrupted save (stale git lock cleared)")
		}
		if err := gs.PullOnStartup(ctx); err != nil {
			return fmt.Errorf("git pull-on-startup: %w", err)
		}
		// First-run repo seed (SPEC §9 starter layout) is wired in Task 3.
	}

	worker := jobs.New(st.DB(), jobs.Config{})
	worker.Register("commit", func(_ context.Context, _ string) error { return nil })
	worker.Start(ctx)
	defer worker.Stop()
	logger.Info("job worker started")

	spa, err := web.Handler()
	if err != nil {
		return fmt.Errorf("build SPA handler: %w", err)
	}
	handler, err := server.New(server.Deps{
		Store:      st,
		Config:     cfg,
		UserRepo:   userRepo,
		SPAHandler: spa,
		Health:     healthAdapter{gs: gs},
	})
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	logger.Info("listening", slog.String("addr", cfg.Server.Listen))
	srv := &http.Server{Addr: cfg.Server.Listen, Handler: handler}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

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

	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
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
	root.AddCommand(adminCmd())
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

	// SEC-05 audit log: records key actions to the SQLite mirror + a structured
	// slog line. Shared by the startup path (bootstrap/seed) and the server.
	auditLog := audit.New(st.DB(), logger)

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
		// Audit the bootstrap (actor=system); the one-time password is NEVER
		// recorded in the audit trail (T-00.04-02).
		_ = auditLog.Record(ctx, audit.Event{
			Action: audit.ActionBootstrap,
			Actor:  "system",
			Target: adminUser,
			Source: "bootstrap",
		})
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
		// Seed a brand-new, empty repo with the SPEC §9 starter layout as the
		// first commit through the single-writer service (D-08/D-10). An
		// existing/pulled repo is left untouched (D-09).
		seeded, err := users.SeedStarterRepo(ctx, gs, contentRepo, adminUser)
		if err != nil {
			return fmt.Errorf("seed starter repo: %w", err)
		}
		if seeded {
			logger.Info("seeded starter repository layout", slog.String("user", adminUser))
			_ = auditLog.Record(ctx, audit.Event{
				Action: audit.ActionSeed,
				Actor:  "system",
				Target: adminUser,
				Source: "bootstrap",
			})
		}
	}

	worker := jobs.New(st.DB(), jobs.Config{})
	worker.SetLogger(logger)
	// Single-writer commit spine (D-04): a page write becomes one hidden Git
	// commit through this handler. Replaces the Phase-0 no-op stub.
	worker.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
	// Text-extraction spine (ATT-08): a separate KindExtract handler on the SAME
	// single drain goroutine reads a committed attachment binary, extracts text via
	// the pure-Go extractors, and commits the <id>.txt sidecar through the ONE
	// KindCommit handler above (no second commit kind). A parser panic on an
	// adversarial file is recovered inside the handler so the drain survives.
	worker.Register(attachments.KindExtract, attachments.ExtractHandler(contentRepo, worker, st.DB(), cfg.Git.PushOnCommit))

	// Full-text search index (Phase 3). The Bleve scorch index lives UNDER the data
	// dir (a derived, rebuildable artifact, NEVER inside the content/Git repo). It is
	// opened once and shared (single worker writer + many HTTP readers). The KindIndex
	// handler maintains it; a startup HEAD-drift check (below, after ReconcileTrash)
	// triggers a rebuild-from-files when the working tree moved out-of-band. Search is
	// always wired; cfg.Search.Enabled is left for a future flag — when the index fails
	// to open the routes return the generic 500 like any other optional dependency.
	indexDir := cfg.Search.IndexDir
	if indexDir == "" {
		indexDir = filepath.Join(cfg.Storage.DataDir, "index")
	}
	searchIdx, err := search.OpenOrCreate(indexDir)
	if err != nil {
		return fmt.Errorf("open search index: %w", err)
	}
	defer func() { _ = searchIdx.Close() }()
	searchIdx.SetRepo(contentRepo)
	searchIdx.SetDB(st.DB())
	worker.Register(search.KindIndex, search.IndexHandler(searchIdx, contentRepo))

	worker.Start(ctx)
	defer worker.Stop()
	logger.Info("job worker started")

	// Page lifecycle service: create/get/save/folder + the nested tree, all
	// mutations flowing through the single-writer CommitJob above. PushOnCommit is
	// threaded from config so Plan 05 only flips the config value.
	pagesSvc := pages.NewService(contentRepo, gs, worker, st.DB(), cfg.Git.PushOnCommit)

	// Attachment lifecycle service: upload/list/download, every write flowing
	// through the SAME single-writer CommitJob registered above (no second commit
	// kind). The size cap comes from config.Storage.MaxUploadMB; the MIME-sniff
	// allow-list from config.Attachments.AllowedExtensions.
	attachSvc := attachments.NewService(contentRepo, worker, st.DB(), cfg.Attachments, cfg.Storage.MaxUploadMB, cfg.Git.PushOnCommit)

	// Reconcile the trash table against the working tree at startup: a prior
	// Delete/Restore whose async commit failed can leave a SQLite trash row that
	// points at a trash_path never written to disk (WR-01). Prune those phantom
	// rows so the trash view and the on-disk state reconverge. Best-effort: a
	// reconcile error must not prevent the server from starting.
	if pruned, err := pagesSvc.ReconcileTrash(context.Background()); err != nil {
		logger.Warn("trash reconcile failed at startup", slog.String("error", err.Error()))
	} else if pruned > 0 {
		logger.Info("pruned phantom trash rows at startup", slog.Int("pruned", pruned))
	}

	// Search drift recovery: if the index was built against a different Git HEAD than
	// the current working tree (an out-of-band pull/restore, or a crash between commit
	// and index), enqueue a rebuild-from-files. Best-effort and fire-and-forget — this
	// is a startup goroutine, not the single drain goroutine, so Enqueue (NOT
	// EnqueueAndWait) is correct (CR-01); a drift-check error must never block startup.
	if drifted, derr := searchIdx.DriftCheck(context.Background(), gs); derr != nil {
		logger.Warn("search drift check failed at startup", slog.String("error", derr.Error()))
	} else if drifted {
		if eerr := worker.Enqueue(context.Background(), search.KindIndex, search.RebuildPayload()); eerr != nil {
			logger.Warn("search rebuild enqueue failed at startup", slog.String("error", eerr.Error()))
		} else {
			logger.Info("search index drift detected — rebuild enqueued")
		}
	}

	spa, err := web.Handler()
	if err != nil {
		return fmt.Errorf("build SPA handler: %w", err)
	}
	handler, err := server.New(server.Deps{
		Store:       st,
		Config:      cfg,
		UserRepo:    userRepo,
		SPAHandler:  spa,
		Health:      healthAdapter{gs: gs},
		Audit:       auditLog,
		Pages:       pagesSvc,
		Attachments: attachSvc,
		Search:      searchIdx,
		SearchJobs:  worker,
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

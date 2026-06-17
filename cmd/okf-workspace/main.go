// Command okf-workspace is the single static binary for OKF Workspace: it
// serves the embedded React SPA and the REST API from one process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/store"
)

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

	// Task 2/3 wire bootstrap-admin and the HTTP server here.
	logger.Info("startup complete (Task 1 scope: config + store + migrate)")
	return nil
}

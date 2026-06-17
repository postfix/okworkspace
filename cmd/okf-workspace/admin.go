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
	"github.com/postfix/okworkspace/internal/users"
)

// adminCmd is the `admin` parent command grouping operator recovery actions
// that share the bootstrap trust boundary (they require shell access to the
// box, D-04).
func adminCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Operator administration commands (require shell access)",
	}
	cmd.PersistentFlags().StringVar(&configPath, "config", "config.yaml", "path to config.yaml")
	cmd.AddCommand(adminResetPasswordCmd(&configPath))
	return cmd
}

// adminResetPasswordCmd implements `admin reset-password <username>`: it resets
// the named user's password, prints the one-time password exactly once, and
// exits 0. An unknown username exits non-zero with a clear message.
func adminResetPasswordCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "reset-password <username>",
		Short: "Reset a user's password and print a one-time credential (recovery)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
			cfg, err := config.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			dbPath := filepath.Join(cfg.Storage.DataDir, "app.db")

			otp, err := runAdminResetPassword(cmd.Context(), dbPath, args[0])
			if err != nil {
				return err
			}
			// Same trust boundary as bootstrap: print the one-time password once
			// to the operator log (T-00.03-07, accepted by design).
			logger.Warn("password reset — save this one-time password, it will NOT be shown again",
				slog.String("username", args[0]),
				slog.String("one_time_password", otp),
				slog.Bool("must_change_password", true),
			)
			return nil
		},
	}
}

// runAdminResetPassword opens the store at dbPath, resets the named user's
// password via users.ResetPassword, and returns the one-time plaintext. It
// returns a non-nil error (mapped to a non-zero exit) when the user is unknown
// or the store cannot be opened.
func runAdminResetPassword(ctx context.Context, dbPath, username string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("open store %q: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		return "", fmt.Errorf("migrate store: %w", err)
	}

	repo := users.NewRepository(st.DB())
	u, err := repo.GetByUsername(ctx, username)
	if err != nil {
		return "", fmt.Errorf("no user named %q: %w", username, err)
	}
	otp, err := users.ResetPassword(ctx, repo, u.ID)
	if err != nil {
		return "", fmt.Errorf("reset password for %q: %w", username, err)
	}
	return otp, nil
}

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
	"time"

	"github.com/spf13/cobra"

	"github.com/postfix/okworkspace/internal/agent"
	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/locks"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
	"github.com/postfix/okworkspace/internal/web"
)

// Soft-lock timing envelope (COLL-02). A lock expires lockExpiry after its last
// heartbeat; the lock_gc ticker reaps expired locks every lockGCInterval. The
// interval is kept strictly below the expiry so a crashed/idle session's lock is
// reaped within roughly one TTL window (RESEARCH A5: interval < expiry).
const (
	lockExpiry     = 2 * time.Minute
	lockGCInterval = 60 * time.Second
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
	// A corrupt existing index (e.g. a torn scorch segment after an unclean
	// shutdown — Pitfall 3) must NOT take the server down: OpenOrRecover wipes and
	// recreates a fresh empty index and signals a rebuild-from-files rather than
	// aborting startup (WR-02). The drift check below would also catch this, but the
	// explicit recovery rebuild repopulates the freshly-emptied index promptly.
	searchIdx, indexRecovered, err := search.OpenOrRecover(indexDir)
	if err != nil {
		return fmt.Errorf("open search index: %w", err)
	}
	defer func() { _ = searchIdx.Close() }()
	searchIdx.SetRepo(contentRepo)
	searchIdx.SetDB(st.DB())
	searchIdx.SetGit(gs)
	worker.Register(search.KindIndex, search.IndexHandler(searchIdx, contentRepo))

	// Derived link/tag graph (Phase 8, LINK-01). Unlike Bleve, the graph adjacency
	// (page_links/page_tags) lives in the operational SQLite db — the tables are
	// created by migration 0009 applied at store open, so there is no separate index
	// dir to manage. The Store is opened over the shared *sql.DB and wired to the
	// content repo (read .md files through the SEC-01 resolver during rebuild/upsert)
	// and the same Git HEAD provider search uses (record last_graphed_head after a
	// rebuild for the startup drift backstop). The KindGraph handler maintains the
	// adjacency on the SAME single drain goroutine; a startup HEAD-drift check (below,
	// beside the search drift block) triggers a rebuild-from-files when the working
	// tree moved out-of-band — which also self-heals an empty graph on a fresh boot.
	graphStore := graph.OpenStore(st.DB())
	graphStore.SetRepo(contentRepo)
	graphStore.SetGit(gs)
	worker.Register(graph.KindGraph, graph.GraphHandler(graphStore, contentRepo))

	// Soft-lock store (COLL-02): the server-authoritative, file-backed lock store
	// every later collaboration slice (soft locks, presence) depends on. All lock
	// I/O routes through contentRepo (SEC-01), never os.*. Locks live under
	// `.okf-workspace/locks/` — a derived coordination artifact, not page content.
	// The lock_gc handler reaps expired lock files; it is registered on the SAME
	// single drain goroutine (before Start) and driven by a ctx-gated ticker below.
	lockStore := locks.NewService(contentRepo, lockExpiry)
	worker.Register(locks.KindGC, locks.GCHandler(lockStore))

	worker.Start(ctx)
	defer worker.Stop()
	logger.Info("job worker started")

	// Lock GC ticker: fire-and-forget enqueue a lock_gc job every lockGCInterval
	// (inside the TTL envelope) so a crashed/idle session's lock self-reaps. The
	// SAME ctx that drives worker.Start cancels this goroutine on shutdown — no
	// separate graceful-shutdown scope is added (RESEARCH caveat). Enqueue (not
	// EnqueueAndWait) keeps it off the request path, mirroring the search rebuild
	// enqueues below.
	go func() {
		t := time.NewTicker(lockGCInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if eerr := worker.Enqueue(context.Background(), locks.KindGC, ""); eerr != nil {
					logger.Warn("lock_gc enqueue failed", slog.String("error", eerr.Error()))
				}
			}
		}
	}()

	// Page lifecycle service: create/get/save/folder + the nested tree, all
	// mutations flowing through the single-writer CommitJob above. PushOnCommit is
	// threaded from config so Plan 05 only flips the config value.
	pagesSvc := pages.NewService(contentRepo, gs, worker, st.DB(), cfg.Git.PushOnCommit)

	// Attachment lifecycle service: upload/list/download, every write flowing
	// through the SAME single-writer CommitJob registered above (no second commit
	// kind). The size cap comes from config.Storage.MaxUploadMB; the MIME-sniff
	// allow-list from config.Attachments.AllowedExtensions.
	attachSvc := attachments.NewService(contentRepo, worker, st.DB(), cfg.Attachments, cfg.Storage.MaxUploadMB, cfg.Git.PushOnCommit)

	// Eino agent service (Phase 4): backs POST /agent/chat (Ask). Its five read-
	// only tools route through the SAME repo.Resolve-backed services — pagesSvc
	// (read_page/list_tree), searchIdx (search_pages/search_attachments), and
	// attachSvc.ExtractedText (read_attachment_text) — never os.ReadFile, and no
	// write/apply tool exists. The API key is read once via cfg.Agent.APIKey()
	// inside the service and never logged. When cfg.Agent.Enabled is false the
	// service still constructs (disabled) so the handler can return the off-state
	// rather than a hang.
	//
	// NOTE (WR-02): searchIdx is a SINGLE process-wide index injected here — agent
	// retrieval is NOT role-scoped. This is acceptable only while the page-read
	// model is "any authenticated user reads everything" (no per-page ACL). When
	// per-page ACLs land, a role-scoped Search must be threaded per request (see
	// agent.runSearch's TODO) before the workspace Ask can be trusted not to leak.
	agentSvc := agent.NewService(cfg.Agent, &agent.Deps{
		Pages:       pagesSvc,
		Search:      searchIdx,
		Attachments: attachSvc,
		Audit:       auditLog,
	})

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

	// Corrupt-index recovery (WR-02): OpenOrRecover already wiped a corrupt index and
	// recreated it empty; enqueue a rebuild-from-files to repopulate it. Fire-and-
	// forget on the single worker (Enqueue, never EnqueueAndWait — CR-01).
	if indexRecovered {
		if eerr := worker.Enqueue(context.Background(), search.KindIndex, search.RebuildPayload()); eerr != nil {
			logger.Warn("search rebuild enqueue after corrupt-index recovery failed", slog.String("error", eerr.Error()))
		} else {
			logger.Info("search index was corrupt — recreated empty and rebuild enqueued")
		}
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

	// Link/tag graph drift recovery: if the graph was built against a different Git
	// HEAD than the current working tree (an out-of-band pull/restore, a crash
	// between commit and graph update, or simply a fresh install with an empty
	// graph_meta over a populated repo), enqueue a rebuild-from-files. This mirrors
	// the search drift block exactly: best-effort and fire-and-forget — a startup
	// goroutine, so Enqueue (NOT EnqueueAndWait) is correct (CR-01), and a drift-check
	// error must never block startup. A fresh boot reads as drift and self-heals the
	// empty graph into the initial adjacency build.
	if drifted, derr := graphStore.DriftCheck(context.Background(), gs); derr != nil {
		logger.Warn("link graph drift check failed at startup", slog.String("error", derr.Error()))
	} else if drifted {
		if eerr := worker.Enqueue(context.Background(), graph.KindGraph, graph.RebuildPayload()); eerr != nil {
			logger.Warn("link graph rebuild enqueue failed at startup", slog.String("error", eerr.Error()))
		} else {
			logger.Info("link graph drift detected — rebuild enqueued")
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
		GraphJobs:   worker,
		Graph:       graphStore,
		Agent:       agentSvc,
		Locks:       lockStore,
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

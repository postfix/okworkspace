package jobs

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Worker is the single-writer async job worker. One background goroutine drains
// due jobs FIFO so handlers never run concurrently (the single-writer guarantee
// that serializes Git operations, PITFALLS.md Pitfall 2). Register handlers
// before Start.
type Worker struct {
	db  *sql.DB
	cfg Config
	log *slog.Logger

	mu       sync.RWMutex
	handlers map[string]Handler

	stop     chan struct{}
	stopped  chan struct{}
	startOne sync.Once
	stopOnce sync.Once
}

// New constructs a Worker over the shared *sql.DB. It logs via slog.Default();
// use SetLogger to inject a specific logger before Start.
func New(db *sql.DB, cfg Config) *Worker {
	return &Worker{
		db:       db,
		cfg:      cfg.withDefaults(),
		log:      slog.Default(),
		handlers: make(map[string]Handler),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// SetLogger overrides the worker's logger (WR-04 observability). Call before
// Start; a nil logger leaves the existing default in place.
func (w *Worker) SetLogger(l *slog.Logger) {
	if l != nil {
		w.log = l
	}
}

// Register binds a handler to a job kind. Not safe to call after Start for the
// same kind concurrently with a drain; register all handlers during wiring.
func (w *Worker) Register(kind string, h Handler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[kind] = h
}

func (w *Worker) handlerFor(kind string) (Handler, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	h, ok := w.handlers[kind]
	return h, ok
}

// Start launches the single drain goroutine. Safe to call once; subsequent
// calls are no-ops.
func (w *Worker) Start(ctx context.Context) {
	w.startOne.Do(func() {
		go w.loop(ctx)
	})
}

// Stop signals the drain goroutine to finish the in-flight job and exit,
// blocking until it has. Safe to call multiple times.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.stopped
}

// loop is the single drain goroutine: claim the next due job, run its handler,
// and on failure apply exponential backoff up to MaxAttempts (then fail). One
// job runs at a time — this IS the single-writer serialization.
func (w *Worker) loop(ctx context.Context) {
	defer close(w.stopped)
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		// Drain all currently-due jobs before sleeping, so a burst of enqueues
		// is processed promptly while still being strictly serialized.
		for {
			if w.shouldStop(ctx) {
				return
			}
			if more := w.drainOne(ctx); !more {
				break
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) shouldStop(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-w.stop:
		return true
	default:
		return false
	}
}

// drainOne claims and runs at most one job, returning true if a job was
// processed (so the caller keeps draining) or false when the queue is empty.
func (w *Worker) drainOne(ctx context.Context) bool {
	jr, err := w.claimNextDue(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		// WR-04: a non-ErrNoRows claim error (schema mismatch, corrupted row,
		// context cancellation) was previously swallowed, leaving a wedged worker
		// spinning silently every tick. Log it so the failure is observable.
		w.log.WarnContext(ctx, "jobs: claim next due failed", "error", err)
		return false // transient DB error; retry on next tick
	}

	h, ok := w.handlerFor(jr.kind)
	if !ok {
		// No handler registered: fail terminally rather than spin forever.
		if err := w.markFailed(ctx, jr.id, errors.New("no handler registered for kind "+jr.kind)); err != nil {
			w.log.WarnContext(ctx, "jobs: mark failed (no handler) failed", "job_id", jr.id, "error", err)
		}
		return true
	}

	if runErr := h(ctx, jr.payload); runErr != nil {
		nextAttempt := jr.attempts + 1
		if nextAttempt >= w.cfg.MaxAttempts {
			if err := w.markFailed(ctx, jr.id, runErr); err != nil {
				w.log.WarnContext(ctx, "jobs: mark failed failed", "job_id", jr.id, "error", err)
			}
		} else {
			if err := w.markRetry(ctx, jr.id, runErr, w.backoff(nextAttempt)); err != nil {
				w.log.WarnContext(ctx, "jobs: mark retry failed", "job_id", jr.id, "error", err)
			}
		}
		return true
	}

	if err := w.markDone(ctx, jr.id); err != nil {
		w.log.WarnContext(ctx, "jobs: mark done failed", "job_id", jr.id, "error", err)
	}
	return true
}

// backoff returns the exponential backoff for the given attempt number
// (1-based), capped at MaxBackoff.
func (w *Worker) backoff(attempt int) time.Duration {
	d := w.cfg.BaseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= w.cfg.MaxBackoff {
			return w.cfg.MaxBackoff
		}
	}
	if d > w.cfg.MaxBackoff {
		return w.cfg.MaxBackoff
	}
	return d
}

// Package jobs is the async job-worker spine (SPEC §16.1 Job service). A single
// background goroutine drains a SQLite-persisted FIFO queue, runs the
// registered handler for each job kind, and applies exponential backoff with a
// retry cap (then marks the job failed — never an infinite loop). It is the
// reused spine for CommitJob/ExtractJob/IndexJob in later phases; this phase
// registers a commit handler only.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Job status values persisted in the jobs table.
const (
	statusPending = "pending"
	statusRunning = "running"
	statusDone    = "done"
	statusFailed  = "failed"
)

// ErrJobTimeout is returned by WaitForJob when the job neither completes nor
// fails within the supplied timeout (or its context is cancelled before a
// terminal state is reached). It is distinct from a job that actually reports
// failed so callers can choose to treat a timeout as a soft outcome (e.g. a
// slow remote push) rather than a hard failure.
var ErrJobTimeout = errors.New("jobs: wait for job timed out")

// waitPollInterval is how often WaitForJob re-reads a job's status. Kept short
// (vs. the worker's drain PollInterval) so a user-facing mutation that blocks on
// WaitForJob returns promptly once the single-writer worker lands the commit.
const waitPollInterval = 20 * time.Millisecond

// Handler executes one job of a given kind. payload is the opaque string stored
// at enqueue time (e.g. a JSON-encoded CommitSpec in later phases). A non-nil
// error triggers retry-with-backoff up to MaxAttempts.
type Handler func(ctx context.Context, payload string) error

// Config tunes the worker's polling and retry behavior.
type Config struct {
	PollInterval time.Duration // how often to scan for due jobs
	BaseBackoff  time.Duration // first retry delay; doubles each attempt
	MaxBackoff   time.Duration // backoff ceiling
	MaxAttempts  int           // attempts before a job is marked failed
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = 250 * time.Millisecond
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 5 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	return c
}

// nowEpoch returns the current time as fractional Unix epoch seconds.
func nowEpoch() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

// Enqueue persists a new pending job to run as soon as the worker drains it. It
// is a thin wrapper over EnqueueJob that discards the inserted id, preserving
// the original fire-and-forget call sites.
func (w *Worker) Enqueue(ctx context.Context, kind, payload string) error {
	_, err := w.EnqueueJob(ctx, kind, payload)
	return err
}

// EnqueueJob persists a new pending job and returns its row id so a caller can
// observe its completion via WaitForJob. The worker stays a single-writer drain;
// returning the id only lets a caller watch a specific job, it does not change
// how jobs are executed.
func (w *Worker) EnqueueJob(ctx context.Context, kind, payload string) (int64, error) {
	res, err := w.db.ExecContext(ctx,
		`INSERT INTO jobs (kind, payload, status, attempts, run_after, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, datetime('now'), datetime('now'))`,
		kind, payload, statusPending, nowEpoch())
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue %q: %w", kind, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue %q last insert id: %w", kind, err)
	}
	return id, nil
}

// WaitForJob blocks until the job with the given id reaches a terminal state,
// polling the jobs table on a short interval (it does not require any
// worker-internal channel, so it works even against a job enqueued on a worker
// that is not running). It returns:
//   - nil when the job is done,
//   - a non-nil error wrapping the stored last_error when the job is failed,
//   - ErrJobTimeout when the job is still pending/running after timeout, or
//   - ctx.Err() if ctx is cancelled first.
//
// A non-positive timeout means "wait until done/failed or ctx cancellation"
// (no deadline).
func (w *Worker) WaitForJob(ctx context.Context, id int64, timeout time.Duration) error {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}

	ticker := time.NewTicker(waitPollInterval)
	defer ticker.Stop()

	for {
		status, lastErr, err := w.jobStatus(ctx, id)
		if err != nil {
			return err
		}
		switch status {
		case statusDone:
			return nil
		case statusFailed:
			if lastErr != "" {
				return fmt.Errorf("jobs: job %d failed: %s", id, lastErr)
			}
			return fmt.Errorf("jobs: job %d failed", id)
		}

		if timeout > 0 && time.Now().After(deadline) {
			return ErrJobTimeout
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// jobStatus reads a job's current status and last_error. A missing row (the job
// id is unknown) is reported as ErrNoRows-wrapped so callers do not spin forever
// on a typo'd id.
func (w *Worker) jobStatus(ctx context.Context, id int64) (status, lastErr string, err error) {
	row := w.db.QueryRowContext(ctx,
		`SELECT status, COALESCE(last_error, '') FROM jobs WHERE id = ?`, id)
	if scanErr := row.Scan(&status, &lastErr); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return "", "", fmt.Errorf("jobs: job %d not found: %w", id, scanErr)
		}
		return "", "", fmt.Errorf("jobs: read job %d status: %w", id, scanErr)
	}
	return status, lastErr, nil
}

// EnqueueAndWait enqueues a job and blocks until it lands (or times out). It is
// the synchronous counterpart to Enqueue for user-facing mutations that must not
// return before their effect is on disk. The returned error follows WaitForJob's
// contract: nil on done, a wrapped failure on failed, ErrJobTimeout on timeout,
// or ctx.Err() on cancellation. Timeout-as-soft-success is a CALLER policy
// (pages logs+succeeds on ErrJobTimeout); this method reports it faithfully.
func (w *Worker) EnqueueAndWait(ctx context.Context, kind, payload string, timeout time.Duration) error {
	id, err := w.EnqueueJob(ctx, kind, payload)
	if err != nil {
		return err
	}
	return w.WaitForJob(ctx, id, timeout)
}

// jobRow is one persisted job claimed for execution.
type jobRow struct {
	id       int64
	kind     string
	payload  string
	attempts int
}

// claimNextDue atomically claims the oldest due pending job (status=pending,
// run_after <= now), flipping it to running so no second drain picks it up.
// Returns sql.ErrNoRows when nothing is due. The store uses a single SQLite
// connection (MaxOpenConns=1), which serializes this read-modify-write.
func (w *Worker) claimNextDue(ctx context.Context) (jobRow, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return jobRow{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var jr jobRow
	err = tx.QueryRowContext(ctx,
		`SELECT id, kind, payload, attempts FROM jobs
		 WHERE status=? AND run_after <= ?
		 ORDER BY run_after ASC, id ASC LIMIT 1`, statusPending, nowEpoch()).
		Scan(&jr.id, &jr.kind, &jr.payload, &jr.attempts)
	if err != nil {
		return jobRow{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status=?, updated_at=datetime('now') WHERE id=?`,
		statusRunning, jr.id); err != nil {
		return jobRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return jobRow{}, err
	}
	return jr, nil
}

// markDone flips a job to done. It does NOT increment attempts (WR-03): the
// attempts column counts tries CONSUMED before this outcome, consistent with
// markRetry/markFailed. drainOne computes the retry/fail decision from
// jr.attempts+1, so a job that succeeds on its first try correctly persists
// attempts=0, not 1.
func (w *Worker) markDone(ctx context.Context, id int64) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, last_error='', updated_at=datetime('now') WHERE id=?`,
		statusDone, id)
	return err
}

// markRetry increments attempts, records the error, and schedules the next run
// after the backoff delay, returning status to pending.
func (w *Worker) markRetry(ctx context.Context, id int64, runErr error, backoff time.Duration) error {
	runAfter := nowEpoch() + backoff.Seconds()
	_, err := w.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, attempts=attempts+1, last_error=?,
		 run_after=?, updated_at=datetime('now') WHERE id=?`,
		statusPending, runErr.Error(), runAfter, id)
	return err
}

// markFailed terminally fails a job after the retry cap is reached.
func (w *Worker) markFailed(ctx context.Context, id int64, runErr error) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, attempts=attempts+1, last_error=?, updated_at=datetime('now') WHERE id=?`,
		statusFailed, runErr.Error(), id)
	return err
}

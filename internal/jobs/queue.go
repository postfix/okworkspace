// Package jobs is the async job-worker spine (SPEC §16.1 Job service). A single
// background goroutine drains a SQLite-persisted FIFO queue, runs the
// registered handler for each job kind, and applies exponential backoff with a
// retry cap (then marks the job failed — never an infinite loop). It is the
// reused spine for CommitJob/ExtractJob/IndexJob in later phases; this phase
// registers a commit handler only.
package jobs

import (
	"context"
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

// Enqueue persists a new pending job to run as soon as the worker drains it.
func (w *Worker) Enqueue(ctx context.Context, kind, payload string) error {
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO jobs (kind, payload, status, attempts, run_after, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, datetime('now'), datetime('now'))`,
		kind, payload, statusPending, nowEpoch())
	if err != nil {
		return fmt.Errorf("jobs: enqueue %q: %w", kind, err)
	}
	return nil
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

// markDone flips a job to done.
func (w *Worker) markDone(ctx context.Context, id int64) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, attempts=attempts+1, last_error='', updated_at=datetime('now') WHERE id=?`,
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

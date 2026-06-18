// Package pages implements the page lifecycle on top of the Phase-0 spines.
// This file holds the CommitJob: the single-writer commit handler (D-04) that
// every batched page write (D-01) flows through. It is the ONLY code path that
// writes a canonical .md file and creates a Git commit — HTTP handlers enqueue a
// commit job and never touch git or the filesystem directly (D-02/D-04). The
// canonical .md is written only when this job fires (D-02), keeping the working
// tree byte-equal to the last commit.
package pages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
)

// KindCommit is the job kind registered on the worker for page commits.
const KindCommit = "commit"

// fileWrite is one file to materialize before committing. Bytes are the exact
// emitted page bytes (typically from okf.Doc.Emit) — already byte-stable. Path
// is repo-relative and is re-validated by repo.Write's resolver (SEC-01).
type fileWrite struct {
	Path  string `json:"path"`
	Bytes []byte `json:"bytes"`
}

// commitPayload is the JSON payload enqueued for a commit job. It batches one or
// more writes (and optional removes) into exactly one commit (D-01). Spec reuses
// gitstore.CommitSpec verbatim (no parallel commit type). Push activates the
// optional remote push added in Plan 05.
//
// Removes lists repo-relative paths to delete from the working tree before the
// commit (the source path of a rename/move). The delete plus the new-path write
// are staged in the SAME commit so git's rename detection traces history across
// the move (`git log --follow`) — D-07/D-08.
type commitPayload struct {
	Writes  []fileWrite         `json:"writes"`
	Removes []string            `json:"removes,omitempty"`
	Spec    gitstore.CommitSpec `json:"spec"`
	Push    bool                `json:"push"`
}

// CommitHandler returns the jobs.Handler registered for KindCommit. On each job
// it unmarshals the payload, writes every file through the safe-path resolver
// (never os.*), then creates exactly one commit through the single-writer Git
// service. It NEVER shells out to git directly — gitstore.Commit owns that.
func CommitHandler(r *repo.Repo, g *gitstore.GitStore) jobs.Handler {
	return func(ctx context.Context, payload string) error {
		var p commitPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("pages: commit payload: %w", err)
		}
		if len(p.Writes) == 0 {
			return fmt.Errorf("pages: commit requires at least one write")
		}

		// Materialize each file through the resolver-gated writer. The .md is
		// written here and only here (D-02).
		for _, fw := range p.Writes {
			if err := r.Write(fw.Path, fw.Bytes); err != nil {
				return fmt.Errorf("pages: write %q: %w", fw.Path, err)
			}
		}

		// Remove the source path(s) of a rename/move from the working tree. The
		// deletion is staged in the SAME commit as the new-path write (the Spec
		// already lists both paths) so git records the move as a rename and
		// `git log --follow` keeps history continuous (D-07/D-08).
		for _, rm := range p.Removes {
			if err := r.Remove(rm); err != nil {
				return fmt.Errorf("pages: remove %q: %w", rm, err)
			}
		}

		// One commit for the whole batch through the single-writer spine (D-04).
		if err := g.Commit(ctx, p.Spec); err != nil {
			return fmt.Errorf("pages: commit: %w", err)
		}

		// Optional remote push AFTER the commit (VER-04). Push is config-gated
		// (RemoteEnabled+PushOnCommit+Remote) inside gitstore.Push, so this is a
		// no-op when no remote is configured even though p.Push is true. On
		// divergence Push alerts (sets diverged) and returns nil — it never
		// force-pushes or auto-merges (T-05-05). p.Push carries config.Git.
		// PushOnCommit threaded from every EnqueueCommit call site, so toggling
		// the config flag alone enables push for every mutation.
		if p.Push {
			if err := g.Push(ctx); err != nil {
				return fmt.Errorf("pages: push: %w", err)
			}
		}
		return nil
	}
}

// EnqueueCommit marshals p and enqueues a commit job on the worker. HTTP
// handlers call this (never git/os directly) so every mutation serializes
// through the single drain goroutine. It accepts the minimal enqueuer interface
// (*jobs.Worker satisfies it) so a test can inject a fake worker.
func EnqueueCommit(ctx context.Context, w enqueuer, p commitPayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("pages: marshal commit payload: %w", err)
	}
	return w.Enqueue(ctx, KindCommit, string(raw))
}

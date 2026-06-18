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
// more writes into exactly one commit (D-01). Spec reuses gitstore.CommitSpec
// verbatim (no parallel commit type). Push activates the optional remote push
// added in Plan 05.
type commitPayload struct {
	Writes []fileWrite         `json:"writes"`
	Spec   gitstore.CommitSpec `json:"spec"`
	Push   bool                `json:"push"`
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

		// One commit for the whole batch through the single-writer spine (D-04).
		if err := g.Commit(ctx, p.Spec); err != nil {
			return fmt.Errorf("pages: commit: %w", err)
		}

		// Push branch is activated in Plan 05 (gitstore.Push). Until then a
		// commitPayload.Push is recorded but not acted on; the commit is local.
		if p.Push {
			_ = p.Push // Plan 05: g.Push(ctx) after Commit when remote is enabled.
		}
		return nil
	}
}

// EnqueueCommit marshals p and enqueues a commit job on the worker. HTTP
// handlers call this (never git/os directly) so every mutation serializes
// through the single drain goroutine.
func EnqueueCommit(ctx context.Context, w *jobs.Worker, p commitPayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("pages: marshal commit payload: %w", err)
	}
	return w.Enqueue(ctx, KindCommit, string(raw))
}

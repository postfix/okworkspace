package search

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestIndex_ConcurrentReadWrite (Pitfall 4, T-03-19): the ONE shared bleve.Index
// is accessed by many concurrent readers (Query) and the single worker writer
// (Index/Delete via indexPage/deletePage/indexAttachment) PLUS the occasional
// atomic rebuild-swap. This test runs all three concurrently and asserts no panic
// and no data race. Run with -race (CGO_ENABLED=1 at test time only — the shipped
// binary stays CGO_ENABLED=0; the race detector is a test-time tool).
//
// The withIndex RLock guards in-flight queries against a concurrent swap pulling
// the index pointer out from under them; this exercises exactly that contract.
func TestIndex_ConcurrentReadWrite(t *testing.T) {
	h := newHarness(t)

	// Seed a handful of pages + attachments so queries return hits while writes and
	// a rebuild run concurrently.
	const seedPages = 8
	for i := 0; i < seedPages; i++ {
		p := fmt.Sprintf("page-%d.md", i)
		h.writePage(t, p, fmt.Sprintf("Deploy Runbook %d", i), []string{"ops"}, fmt.Sprintf("body text about deploy and rollback %d", i))
		if err := h.idx.indexPage(context.Background(), p); err != nil {
			t.Fatalf("seed indexPage %q: %v", p, err)
		}
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("01ATT%05d", i)
		h.writeAttachment(t, id, fmt.Sprintf("manual-%d.pdf", i), "page-0.md", fmt.Sprintf("attachment extracted deploy text %d", i))
		if err := h.idx.indexAttachment(id); err != nil {
			t.Fatalf("seed indexAttachment %q: %v", id, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 64)

	// READERS: many concurrent queries against the shared index.
	const readers = 8
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				if _, err := h.idx.Query(ctx, "deploy"); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("query: %w", err)
					return
				}
			}
		}()
	}

	// WRITER (single, mirroring the single worker drain goroutine): interleave
	// page upserts, page deletes, and attachment upserts on the shared index.
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for ctx.Err() == nil {
			p := fmt.Sprintf("page-%d.md", i%seedPages)
			switch i % 3 {
			case 0:
				h.writePage(t, p, fmt.Sprintf("Deploy Runbook %d edit", i), []string{"ops"}, fmt.Sprintf("revised deploy body %d", i))
				if err := h.idx.indexPage(ctx, p); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("indexPage: %w", err)
					return
				}
			case 1:
				if err := h.idx.deletePage(ctx, p); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("deletePage: %w", err)
					return
				}
			default:
				id := fmt.Sprintf("01ATT%05d", i%4)
				if err := h.idx.indexAttachment(id); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("indexAttachment: %w", err)
					return
				}
			}
			i++
		}
	}()

	// REBUILD-SWAP under load: periodically rebuild the whole index, exercising the
	// atomic dir-swap mutex while readers query and the writer mutates. In-flight
	// queries must read the old index until the swap completes (no panic/race).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			if err := h.idx.RebuildIndex(ctx); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("rebuild: %w", err)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent read/write error: %v", err)
	}
}

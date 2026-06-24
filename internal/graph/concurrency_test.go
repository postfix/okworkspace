package graph

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestGraph_ConcurrentReadWrite (CR-01 analog): the KindGraph job runs on the
// single jobs drain goroutine, but this test deliberately drives concurrent
// upsert / delete / RebuildGraph goroutines against the shared Store to prove the
// SQLite per-page tx discipline is -race clean and never deadlocks. Run with
// -race (CGO_ENABLED=1 at test time only — the shipped binary stays
// CGO_ENABLED=0; the race detector is a test-time tool).
func TestGraph_ConcurrentReadWrite(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	const seedPages = 8
	for i := 0; i < seedPages; i++ {
		p := fmt.Sprintf("page-%d.md", i)
		nxt := fmt.Sprintf("page-%d.md", (i+1)%seedPages)
		h.writePage(t, p, fmt.Sprintf("Page %d", i), []string{"ops"},
			fmt.Sprintf("links to [next](%s) body %d", nxt, i))
	}
	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("seed RebuildGraph: %v", err)
	}

	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 64)

	// READERS: concurrent backlink queries (the reverse query on page_links).
	const readers = 6
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for runCtx.Err() == nil {
				rows, err := h.db.DB().QueryContext(runCtx,
					`SELECT src_path FROM page_links WHERE dst_path=?`, "page-0.md")
				if err != nil {
					if runCtx.Err() == nil {
						errCh <- fmt.Errorf("backlink query: %w", err)
					}
					return
				}
				for rows.Next() {
					var s string
					_ = rows.Scan(&s)
				}
				_ = rows.Close()
			}
		}()
	}

	// WRITERS: interleave per-page upsert, delete, and full rebuild.
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for runCtx.Err() == nil {
			p := fmt.Sprintf("page-%d.md", i%seedPages)
			switch i % 3 {
			case 0:
				if err := h.st.upsertPage(runCtx, p); err != nil && runCtx.Err() == nil {
					errCh <- fmt.Errorf("upsertPage: %w", err)
					return
				}
			case 1:
				if err := h.st.deletePage(runCtx, p); err != nil && runCtx.Err() == nil {
					errCh <- fmt.Errorf("deletePage: %w", err)
					return
				}
			default:
				if err := h.st.RebuildGraph(runCtx); err != nil && runCtx.Err() == nil {
					errCh <- fmt.Errorf("rebuild: %w", err)
					return
				}
			}
			i++
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent read/write error: %v", err)
	}
}

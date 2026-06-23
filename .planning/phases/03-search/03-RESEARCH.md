# Phase 3: Search - Research

**Researched:** 2026-06-21
**Domain:** Full-text search over Markdown pages + extracted attachment text (Bleve v2.6.0, pure-Go, single binary)
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Index architecture & lifecycle (Area 1 — accepted)**
- **Bleve index on disk under the data dir** (e.g. `<data_dir>/index/`), **NOT in Git** — it is a derived, rebuildable artifact (files remain the source of truth).
- **Incremental `IndexJob`** on the EXISTING `internal/jobs` worker (new `KindIndex`), triggered on page-save (CommitJob done) and extraction-done, **plus a full rebuild-from-files** path.
- **Drift recovery:** persist the last-indexed Git HEAD; on startup, if HEAD differs from the last-indexed HEAD, trigger a rebuild (defense against SQLite/Bleve/disk drift). Also expose an **admin "Reindex" action**.
- **Indexed documents:** pages (title / body / tags / headings) and attachments (original filename + extracted `.txt` text), each as a TYPED Bleve document.

**Query & relevance (Area 2 — accepted)**
- **Per-field index mapping:** title (boosted for relevance), body, tags (keyword/faceted), filename, extracted-text. Headings indexed for deep-link results.
- **Match query with prefix + fuzzy (typo tolerance) + phrase support**; title boosted.
- **Facet by result type** (page / heading / attachment) — the richer faceting that motivated choosing Bleve over SQLite FTS5.
- **Bleve fragment highlighting** of matched terms for result snippets.

**Result types & UX (Area 3 — accepted)**
- **Typed results (SRCH-06):** page / heading / attachment. A heading result deep-links to the page section; an attachment result links to its **owning page** (SRCH-05).
- **Obsidian-style ⌘K quick-switcher** (top-bar / keyboard-triggered command palette) opening a results panel — fits the "mimic Obsidian" UI direction (team are ex-Obsidian users).
- **Result row:** type badge + title + highlighted snippet; click navigates in-app. No Git vocabulary anywhere.

**Scope & access (Area 4 — accepted)**
- **Any authenticated user** may search (matches the page-read authorization model).
- **Trashed pages are EXCLUDED** from results (live pages only).
- **Reindex triggers:** page create / edit / rename / move / delete + attachment upload / replace / remove + extraction-done all keep the index live (incremental).
- **Clear empty + "no results" states**, no Git vocabulary.

### Claude's Discretion
- Exact Bleve index mapping/analyzer config, query builder shape, snippet length, ⌘K component layout, and the index-version/HEAD bookkeeping mechanism are at Claude's discretion, consistent with Phase 0–2 patterns.

### Deferred Ideas (OUT OF SCOPE)
- Search analytics / ranking tuning beyond Bleve defaults + title boost.
- Cross-workspace or saved searches.
- The Eino agent (Phase 4), collaboration (Phase 5).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SRCH-01 | User can search page titles | Per-field `title` text field with boost; `NewMatchQuery(...).SetField("title").SetBoost(...)`. See Standard Stack + Pattern 2. |
| SRCH-02 | User can search page body full text | Opaque `Body` bytes from `okf.Doc.Body` indexed into a `body` text field. See Pattern 1 (page → typed doc). |
| SRCH-03 | User can search by tag | Tags read from frontmatter sequence node; indexed as a `keyword`-analyzer field AND used as a `type`-free facet/term. See "Reading tags" note + Pattern 1. |
| SRCH-04 | User can search attachment filenames | `AttachmentMeta.OriginalName` indexed into a `filename` field on attachment-typed docs. See Pattern 1 (attachment → typed doc). |
| SRCH-05 | User can search extracted attachment text + find owning page | `<id>.txt` indexed into `extracted_text`; `AttachmentMeta.PagePath` stored as `page_path` so a hit links to its owning page (no scan needed — the meta already carries it). See Pattern 1 + "Owning-page mapping". |
| SRCH-06 | Typed results (page / attachment / heading) | A `type` field on every doc + `DefaultMapping`/per-type `AddDocumentMapping`; facet by `type`; headings indexed as separate `heading`-typed sub-documents that deep-link to a section. See Pattern 1 + Pattern 3. |
</phase_requirements>

## Summary

Phase 3 makes the **already-LOCKED** stack — Bleve v2.6.0, pure-Go, scorch on-disk index — implementable. Bleve v2.6.0 is the current latest (released 2026-04-30) [VERIFIED: proxy.golang.org/github.com/blevesearch/bleve/v2/@latest]. It is a drop-in addition to the existing single-binary architecture: scorch (Bleve's default index type) is pure-Go and needs no cgo, so the `CGO_ENABLED=0` build promise holds. There are **no new external services** and no library swaps to evaluate — this research is about wiring the concrete v2.6.0 API into the existing `internal/jobs` worker, `internal/okf` parser, and `internal/attachments` sidecar model.

The dominant engineering concern is **index lifecycle, not query syntax**. The query layer is a small, well-trodden surface (a per-field index mapping, a disjunction of match/prefix/fuzzy/phrase queries with a title boost, a `type` facet, and fragment highlighting). The hard part is keeping a *derived, non-Git* artifact consistent with the files-are-truth source across save/rename/move/trash/extract events and across crashes. The accepted design solves this with three layers: (1) a new fire-and-forget `KindIndex` job on the existing worker for incremental updates (critically: `Enqueue`, **never** `EnqueueAndWait` from inside a handler — the Phase 2 CR-01 deadlock lesson); (2) an idempotent full rebuild-from-files that walks live pages via `internal/pages`/`internal/okf` and attachments via `internal/attachments`; and (3) a last-indexed-HEAD bookkeeping value that triggers an auto-rebuild on startup when Git HEAD has drifted, plus an admin reindex endpoint.

Two structural facts make several requirements trivial that would otherwise need scanning: the attachment `<id>.json` meta sidecar **already carries `PagePath`** (so SRCH-05 owning-page linking is a stored field, not a tree scan), and `okf.Doc.Body` is **already isolated opaque text** (so body indexing never touches the byte-stable round-trip). The one genuinely new parsing task is heading extraction (no heading extractor exists in `internal/okf` yet) — a small ATX-heading scanner over `Doc.Body`, indexed as separate `heading`-typed sub-documents that deep-link to `#anchor` sections.

**Primary recommendation:** Add `github.com/blevesearch/bleve/v2 v2.6.0`; build a typed index mapping (`type` field + per-type document mappings) under `<data_dir>/index/` (outside the Git repo); wire a new `KindIndex` handler on the existing worker using fire-and-forget `Enqueue`; make the rebuild-from-files job the robust, idempotent core; persist last-indexed HEAD in SQLite for startup drift detection; and serve a single authed `GET /api/v1/search` endpoint returning typed results with weight-only-safe highlight fragments.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Build/open the Bleve index | Backend (Go, new `internal/search`) | Database/Storage (`<data_dir>/index/`) | Index is a server-owned derived artifact; lives on disk outside Git. |
| Incremental index update on mutation | Backend (job worker, new `KindIndex`) | — | Must serialize behind the single-writer worker; reuses the existing spine. |
| Full rebuild-from-files | Backend (job worker / startup) | Database/Storage | Walks files (source of truth) → rebuilds the derived index idempotently. |
| Drift detection (HEAD bookkeeping) | Backend (startup) | Database/Storage (SQLite metadata) | Compares Git HEAD to last-indexed HEAD; operational metadata only. |
| Heading extraction from page body | Backend (`internal/okf` extension) | — | Pure text scan over `Doc.Body`; no AST, preserves round-trip. |
| Query parsing + relevance | Backend (`internal/search`) | — | Bleve query construction + boosts + facets + highlight server-side. |
| Search HTTP endpoint | API/Backend (chi authed group) | — | Authz from session role (any authed user); mirrors page-read model. |
| Admin reindex action | API/Backend (chi admin subgroup) | — | Privileged operational action; mirrors existing admin subgroup. |
| ⌘K palette + result rendering | Browser/Client (React SPA) | — | Keyboard-first overlay; renders server-provided typed results + snippets. |
| Highlight rendering (XSS-safe) | Browser/Client | API/Backend (sanitizes fragments) | Server returns weight-only-safe markup; SPA renders without `dangerouslySetInnerHTML` of raw server HTML. |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/blevesearch/bleve/v2` | v2.6.0 | Pure-Go full-text index (scorch), per-field mapping, facets, fragment highlighting | LOCKED (CLAUDE.md). Latest, released 2026-04-30 [VERIFIED: proxy.golang.org]. Embeddable, no external service, no cgo with scorch — preserves single static binary + `CGO_ENABLED=0`. |

### Supporting (already in the codebase — reuse, do not re-add)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `internal/jobs` | in-repo | Single-writer async worker; new `KindIndex` handler | Incremental index + rebuild jobs run here (mirrors `KindCommit`/`KindExtract`). |
| `internal/okf` | in-repo | Parse page frontmatter (`Field`) + opaque `Body`; NEW heading scan | Read title/tags + body for indexing; extract headings. |
| `internal/attachments` | in-repo | `AttachmentMeta` (`OriginalName`, `PagePath`, `Ext`), `TxtPath(id)`, `MetaPath(id)` | Index attachment filename + extracted text; owning-page link via `PagePath`. |
| `internal/pages` | in-repo | `Service.Tree` / `repo.Tree()` for the rebuild walk; `trash.go` (trashDir) for exclusion | Enumerate live pages for rebuild; exclude `.okf-workspace/trash`. |
| `internal/repo` | in-repo | Safe-path `Read`/`Tree`/`Resolve` (SEC-01) | All file reads for indexing route through the resolver. |
| `internal/config` | in-repo | `SearchConfig{Enabled, Engine}` placeholder; `Storage.DataDir` | Gate search on; derive index dir from `data_dir`. |
| `internal/gitstore` | in-repo | `git` CLI wrapper; NEW exported `HeadSHA(ctx)` needed | HEAD bookkeeping for drift detection (see Open Questions Q1). |

### Frontend (already locked in CLAUDE.md — reuse)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `@tanstack/react-query` | 5.101.0 | Debounced search query + cache | The ⌘K query → results fetch. |
| `zustand` | 5.0.14 | Palette open/closed + active-row UI state | Per UI-SPEC component contract. |
| `react-router-dom` | 7.18.0 | In-app navigation on result open (page / `#anchor` / owning page) | Enter/click navigation. |
| `lucide-react` | (dep) | `FileText` / `Hash` / `Paperclip` / `Search` icons | Result type icons per UI-SPEC. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Bleve scorch (pure-Go) | SQLite FTS5 | **REJECTED / LOCKED** in CLAUDE.md "What NOT to Use": Bleve chosen for richer relevance/faceting and to keep content out of SQLite. Do not relitigate. |
| Bleve `upsidedown` + boltdb store | Bleve `scorch` | scorch is the modern default, pure-Go, better write amplification; `bleve.New` uses scorch by default. Use scorch. |
| `NewQueryStringQuery` (Lucene-ish syntax) | Hand-built disjunction of typed match/prefix/fuzzy/phrase | Query-string exposes operator syntax to end users and is harder to field-boost predictably. Build the query programmatically for control over per-field boosts and fuzziness (Pattern 2). |
| Custom highlighter | Bleve HTML highlighter + custom fragment formatter | Bleve's default HTML highlighter wraps matches in `<mark>` (a background fill) — the UI-SPEC forbids that. Use a custom fragment formatter that emits `<strong>` (or plain text + offsets), then sanitize to weight-only (see Pitfall 6). |

**Installation:**
```bash
go get github.com/blevesearch/bleve/v2@v2.6.0
# brings in blevesearch/* sub-deps (scorch, vellum, etc.) — all pure-Go for scorch.
go mod tidy
# Verify the static-binary promise still holds:
CGO_ENABLED=0 go build ./...
```

**Version verification (done this session):**
```
proxy.golang.org/github.com/blevesearch/bleve/v2/@latest
→ {"Version":"v2.6.0","Time":"2026-04-30T16:09:25Z", ... refs/tags/v2.6.0}
```
[VERIFIED: proxy.golang.org] v2.6.0 is both the locked version AND the current latest.

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `github.com/blevesearch/bleve/v2` | Go proxy (proxy.golang.org) | tag v2.6.0 dated 2026-04-30; project ~9 yrs | n/a (Go modules) | github.com/blevesearch/bleve | OK | Approved (LOCKED in CLAUDE.md) |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

Notes: Go ecosystem (no npm/PyPI). The `package-legitimacy check` seam targets npm/pypi/crates and does not cover the Go proxy; legitimacy here is established by (a) the module being the canonical, long-lived `blevesearch` org repo, (b) the version being LOCKED in CLAUDE.md after prior validation, and (c) the tag being confirmed present and current on proxy.golang.org this session. Transitive `blevesearch/*` deps are pulled by the canonical module's own `go.mod` and pinned via `go.sum` — commit the lockfile after `go mod tidy`.

## Architecture Patterns

### System Architecture Diagram

```
WRITE / INDEX-MAINTENANCE PATH (all serialized on the single worker)
─────────────────────────────────────────────────────────────────────
  page save / rename / move / delete-to-trash        attachment upload /
  (pages.Service → EnqueueCommit KindCommit)         replace / remove
            │                                                │
            │  (after commit lands)                          │ extraction done
            ▼                                                ▼ (ExtractHandler)
  ┌──────────────────────────────────────────────────────────────────┐
  │  worker.Enqueue(KindIndex, payload)   ← FIRE-AND-FORGET (CR-01)    │
  │  payload = {op: upsert|delete, kind: page|attachment, path/id}    │
  └──────────────────────────────────────────────────────────────────┘
            │ (next drain iteration, single goroutine)
            ▼
  ┌──────────────────────────────────────────────────────────────────┐
  │ IndexHandler:                                                     │
  │   upsert page → okf.Parse → {title,tags,body} + heading scan      │
  │                → index.Index(pageDocID, pageDoc)                  │
  │                → for each heading: index.Index(headingDocID, …)   │
  │   upsert attach → readMeta(id) {OriginalName,PagePath} + .txt     │
  │                → index.Index(attachDocID, attachDoc)              │
  │   delete → index.Delete(docID) (+ delete child heading docs)      │
  └──────────────────────────────────────────────────────────────────┘
            │
            ▼   <data_dir>/index/   (scorch, on disk, OUTSIDE Git)

REBUILD / DRIFT PATH
────────────────────
  startup → read last_indexed_head (SQLite) vs gitstore.HeadSHA()
          → if differ (or no index dir) → enqueue KindIndex{op: rebuild}
  admin POST /search/reindex → enqueue KindIndex{op: rebuild}
            │
            ▼
  RebuildIndex (idempotent): build NEW index in <data_dir>/index.tmp/ →
    walk live pages (skip .okf-workspace/trash) + attachments →
    Index all → close → atomically swap dir → store last_indexed_head

READ PATH
─────────
  SPA ⌘K (react-query, debounced)
    → GET /api/v1/search?q=…  (authed group; any authenticated user)
    → search.Query(q): disjunction(match+prefix+fuzzy+phrase, title boost)
                       + Highlight + Facet(type) + exclude trashed
    → typed results [{type,title,snippet(weight-safe),pagePath,anchor}]
    → SPA renders grouped rows; Enter navigates in-app
```

### Recommended Project Structure
```
internal/search/
├── index.go        # OpenOrCreate(<data_dir>/index), buildMapping(), Close
├── mapping.go      # typed index mapping: type field + per-type doc mappings
├── doc.go          # pageDoc / headingDoc / attachmentDoc structs + IDs
├── indexjob.go     # KindIndex handler: upsert/delete/rebuild dispatch (mirrors extractjob.go)
├── rebuild.go      # RebuildIndex: idempotent walk-from-files + atomic dir swap
├── query.go        # Query(q) → SearchRequest builder + result mapping
├── headings.go     # NEW ATX heading scan over okf.Doc.Body (or place in internal/okf)
├── service.go      # Service wiring (repo, worker, db, gitstore, cfg)
└── *_test.go
internal/server/
├── handlers_search.go   # GET /search (authed) + POST /search/reindex (admin)
web/src/components/search/
├── SearchPalette.tsx + SearchPalette.css   # ⌘K quick-switcher (per UI-SPEC)
```

### Pattern 1: Typed index mapping + typed documents (SRCH-06)
**What:** One Bleve index, every document carries a `type` field (`page`/`heading`/`attachment`); a per-type document mapping configures fields. The `type` field also drives the result-type facet.
**When to use:** Always for this phase — it is how typed results and faceting work.
**Example:**
```go
// Source: bleve v2.6.0 mapping/index.go, mapping/document.go, mapping/field.go
//         (signatures verified against refs/tags/v2.6.0)
package search

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

const (
	TypePage       = "page"
	TypeHeading    = "heading"
	TypeAttachment = "attachment"
)

func buildMapping() mapping.IndexMapping {
	im := bleve.NewIndexMapping()        // *mapping.IndexMappingImpl
	im.TypeField = "type"                // tells bleve which field selects the per-type mapping
	im.DefaultAnalyzer = "en"            // English analyzer: stemming + stop words for body/title

	// Reusable field mappings.
	titleFM := bleve.NewTextFieldMapping()   // analyzed text; boost is applied at QUERY time, not here
	titleFM.Store = true
	titleFM.IncludeTermVectors = true        // REQUIRED for phrase queries + fragment highlighting

	bodyFM := bleve.NewTextFieldMapping()
	bodyFM.Store = true                      // store so highlighter can build fragments from source
	bodyFM.IncludeTermVectors = true

	keywordFM := bleve.NewKeywordFieldMapping() // 'keyword' analyzer: whole-token, exact (tags, type)
	keywordFM.Store = true

	filenameFM := bleve.NewTextFieldMapping()
	filenameFM.Store = true
	filenameFM.IncludeTermVectors = true

	textFM := bleve.NewTextFieldMapping()    // extracted attachment text
	textFM.Store = true
	textFM.IncludeTermVectors = true

	typeFM := bleve.NewKeywordFieldMapping() // facetable, exact
	typeFM.Store = true

	pathFM := bleve.NewKeywordFieldMapping()
	pathFM.Store = true                      // stored, returned for navigation; not really searched

	// Page documents.
	page := bleve.NewDocumentMapping()
	page.AddFieldMappingsAt("type", typeFM)
	page.AddFieldMappingsAt("title", titleFM)
	page.AddFieldMappingsAt("body", bodyFM)
	page.AddFieldMappingsAt("tags", keywordFM)
	page.AddFieldMappingsAt("page_path", pathFM)

	// Heading documents (deep-link sub-docs).
	heading := bleve.NewDocumentMapping()
	heading.AddFieldMappingsAt("type", typeFM)
	heading.AddFieldMappingsAt("title", titleFM)        // heading text
	heading.AddFieldMappingsAt("page_path", pathFM)
	heading.AddFieldMappingsAt("anchor", pathFM)        // "#some-section"
	heading.AddFieldMappingsAt("page_title", pathFM)    // for the "in {page title}" sub-line

	// Attachment documents.
	attach := bleve.NewDocumentMapping()
	attach.AddFieldMappingsAt("type", typeFM)
	attach.AddFieldMappingsAt("filename", filenameFM)   // OriginalName
	attach.AddFieldMappingsAt("extracted_text", textFM)
	attach.AddFieldMappingsAt("page_path", pathFM)      // OWNING page (SRCH-05)
	attach.AddFieldMappingsAt("page_title", pathFM)

	im.AddDocumentMapping(TypePage, page)
	im.AddDocumentMapping(TypeHeading, heading)
	im.AddDocumentMapping(TypeAttachment, attach)
	return im
}
```
[VERIFIED: github.com/blevesearch/bleve@v2.6.0 mapping/index.go, mapping/document.go, mapping/field.go]
`im.TypeField`, `im.DefaultAnalyzer`, `AddDocumentMapping(doctype string, dm *DocumentMapping)`, `DocumentMapping.AddFieldMappingsAt(property string, fms ...*FieldMapping)`, and `FieldMapping{Store, IncludeTermVectors, ...}` are all confirmed present at v2.6.0. `IncludeTermVectors` docstring: *"Term vectors are required to perform phrase queries or terms highlighting in source documents."*

### Pattern 2: Query builder — match + prefix + fuzzy + phrase, title boost (SRCH-01..05)
**What:** A disjunction (OR) of several typed queries so a single user string matches exact terms, prefixes (as-you-type), typos (fuzzy), and exact phrases; title is boosted; facet by `type`; highlight on.
**When to use:** The single `Query(q)` entry point behind `GET /search`.
**Example:**
```go
// Source: bleve v2.6.0 search.go, query.go, search/query/match.go
func (s *Service) Query(q string, size int) (*bleve.SearchResult, error) {
	dis := bleve.NewDisjunctionQuery()

	// Title — boosted, fuzzy for typo tolerance.
	mt := bleve.NewMatchQuery(q)
	mt.SetField("title")
	mt.SetBoost(3.0)
	mt.SetFuzziness(1)           // 0 = exact (default); 1 enables 1-edit typo tolerance
	dis.AddQuery(mt)

	// Body / extracted text — normal weight, fuzzy.
	for _, f := range []string{"body", "extracted_text", "filename"} {
		m := bleve.NewMatchQuery(q)
		m.SetField(f)
		m.SetFuzziness(1)
		dis.AddQuery(m)
	}

	// Tags — exact keyword term (SRCH-03).
	tag := bleve.NewTermQuery(q)
	tag.SetField("tags")
	dis.AddQuery(tag)

	// Prefix on title for instant as-you-type matches.
	pre := bleve.NewPrefixQuery(q)
	pre.SetField("title")
	pre.SetBoost(2.0)
	dis.AddQuery(pre)

	// Exact phrase across title+body (boosted).
	ph := bleve.NewMatchPhraseQuery(q)
	ph.SetField("body")
	dis.AddQuery(ph)

	req := bleve.NewSearchRequestOptions(dis, size, 0, false)
	req.Fields = []string{"type", "title", "page_path", "page_title", "anchor", "filename"} // returned stored fields
	req.Highlight = bleve.NewHighlightWithStyle("html")  // see Pitfall 6 for weight-only override
	req.Highlight.AddField("title")
	req.Highlight.AddField("body")
	req.Highlight.AddField("extracted_text")
	req.AddFacet("types", bleve.NewFacetRequest("type", 3)) // facet by result type (SRCH-06)

	return s.index.Search(req)
}
```
[VERIFIED: github.com/blevesearch/bleve@v2.6.0 search.go, query.go, search/query/match.go]
Confirmed: `NewSearchRequestOptions(q query.Query, size, from int, explain bool) *SearchRequest`; `SearchRequest` fields `Fields`, `Highlight`, `Facets`; `(*SearchRequest).AddFacet(name string, f *FacetRequest)`; `NewHighlight()` / `NewHighlightWithStyle(style string) *HighlightRequest`; `(*HighlightRequest).AddField(field string)`; `NewFacetRequest(field string, size int) *FacetRequest`; `MatchQuery.SetField/SetBoost/SetFuzziness/SetPrefix`; `NewPrefixQuery/NewFuzzyQuery/NewMatchPhraseQuery/NewTermQuery/NewDisjunctionQuery/NewConjunctionQuery`. **Default `MatchQuery.Fuzziness` is 0 (exact)** — fuzzy only activates when `SetFuzziness(n>0)` is called.

**Reading results:** iterate `result.Hits` (a `DocumentMatchCollection`); each `DocumentMatch` has `ID`, `Score`, `Fields map[string]interface{}` (the stored fields you requested), and `Fragments map[string][]string` (highlighted snippets per field). `result.Facets["types"]` gives the per-type counts. [VERIFIED: bleve@v2.6.0 search.go DocumentMatch/SearchResult].

### Pattern 3: Headings as deep-link sub-documents (SRCH-06 heading type)
**What:** For each ATX heading (`#`..`######`) in `okf.Doc.Body`, index a separate `heading`-typed document whose ID is derived from the page path + a GitHub-style anchor slug, storing `title` (heading text), `page_path`, `page_title`, and `anchor` (`#slug`).
**When to use:** During page upsert, alongside the page document.
**Example:**
```go
// NEW scanner — internal/okf has NO heading extractor today.
// Scan Doc.Body line-by-line; SKIP fenced code blocks (reuse okf/links.go's
// fence-skipping helpers as the model so a "# " inside ``` is not a heading).
type Heading struct {
	Level  int
	Text   string
	Anchor string // "#some-slug" (GitHub-style: lowercase, spaces→-, drop punctuation)
}

func headingDocID(pagePath, anchor string) string { return pagePath + anchor }

// On page upsert:
for _, h := range scanHeadings(doc.Body) {
	id := headingDocID(pagePath, h.Anchor)
	_ = idx.Index(id, map[string]interface{}{
		"type":       TypeHeading,
		"title":      h.Text,
		"page_path":  pagePath,
		"page_title": pageTitle,
		"anchor":     h.Anchor,
	})
}
```
**Critical:** when a page is **deleted or its body changes**, stale heading docs must be removed. Because heading IDs are `pagePath + "#" + slug`, a body edit that renames a heading orphans the old ID. Simplest robust approach: on every page upsert, first **delete all heading docs for that page**, then re-index the current set. Bleve has no "delete by prefix"; track a page's heading IDs (e.g. re-derive from the previous committed body, or keep the set is hard) — see Open Questions Q2. The **rebuild path sidesteps this entirely** (fresh index), which is another reason the rebuild job is the safety net.

### Pattern 4: Fire-and-forget incremental indexing on the existing worker (the CR-01 lesson)
**What:** Index maintenance is a new `KindIndex` job, enqueued with `worker.Enqueue` (fire-and-forget) from mutation code paths — **never** `EnqueueAndWait` from inside a handler.
**When to use:** Every page/attachment mutation and extraction-done event.
**Example:**
```go
// Mirror extractjob.go exactly. KindIndex constant + payload struct + handler.
const KindIndex = "index"

type indexPayload struct {
	Op   string `json:"op"`   // "upsert" | "delete" | "rebuild"
	Kind string `json:"kind"` // "page" | "attachment"
	Path string `json:"path"` // page path
	ID   string `json:"id"`   // attachment id (for attachment ops)
}

// Enqueued from pages.Service AFTER EnqueueCommit returns, and from
// attachments.ExtractHandler / lifecycle. From a handler running ON the worker
// (e.g. extraction-done), use plain Enqueue — NOT EnqueueAndWait — exactly as
// extractjob.go enqueues its commit (CR-01: waiting on the same worker deadlocks).
_ = worker.Enqueue(ctx, KindIndex, string(raw))
```
[CITED: internal/attachments/extractjob.go — the CR-01 fire-and-forget pattern this MUST follow]

### Anti-Patterns to Avoid
- **Putting the index inside the Git repo / `repo_dir`.** It is derived, churny, and large; it MUST live under `<data_dir>/index/` (sibling of `app.db`), and the repo's `.gitignore`/tree-walk already skips non-`.md` and `.okf-workspace`, but the index dir must be entirely outside the content repo so the safe-path resolver and Git never see it.
- **`EnqueueAndWait` from inside a job handler.** Deadlocks the single drain goroutine (Phase 2 CR-01). Always `Enqueue`.
- **Opening the index per request.** Open once at startup, hold one `bleve.Index`, share it (it is concurrency-safe for reads + a single writer; the single-writer worker owns all writes). Closing/reopening per query corrupts/locks.
- **Indexing trashed pages.** The rebuild walk and incremental upserts must skip `.okf-workspace/trash` (the `isSkippedDir` rule in `pages/tree.go`); a delete-to-trash must `index.Delete` the page (and its heading docs).
- **Routing the page body through a Markdown AST to index it.** Index `okf.Doc.Body` as opaque text; never reformat it. (The body never round-trips back to disk from search.)
- **Returning raw Bleve `<mark>`-wrapped fragments to the SPA and rendering with `dangerouslySetInnerHTML`.** Stored-XSS vector + violates the UI-SPEC weight-only highlight rule. See Pitfall 6.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Full-text relevance ranking | TF-IDF/BM25 scorer, tokenizer, stemmer | Bleve `en` analyzer + scoring | Tokenization, stemming, stop words, scoring are deep; Bleve ships them. |
| Typo tolerance | Levenshtein matcher over your terms | `MatchQuery.SetFuzziness(1)` | Bleve does bounded edit-distance term expansion over the index efficiently. |
| Result snippets | Substring + manual term bolding | Bleve `Highlight` + `Fragments` | Bleve finds best fragment windows around matches via term vectors. |
| Faceting by type | Group + count in Go after fetch | `AddFacet` + `result.Facets` | Computed in the index pass; correct across paging. |
| Persistent on-disk index | Custom file format / SQLite blob | scorch (`bleve.New`/`bleve.Open`) | Crash-tolerant segmented store, pure-Go, no service. |
| Owning-page lookup for an attachment | Scan every page for the download ref | `AttachmentMeta.PagePath` (already stored) | The meta sidecar already records the owning page — index it as `page_path`. |
| Heading anchors | Invent your own anchor scheme | GitHub-style slug (lowercase, spaces→`-`, strip punctuation) | Matches what a Markdown renderer/`react-markdown` produces, so `#anchor` deep-links resolve. (Confirm against the SPA renderer — Open Questions Q3.) |

**Key insight:** Bleve's value is precisely the parts that look simple but aren't (ranking, fuzzy, highlight, facets). The phase's *own* engineering is the lifecycle glue (jobs, rebuild, drift), not the search algorithm.

## Runtime State Inventory

> This is NOT a rename/refactor phase, but it INTRODUCES a new derived runtime artifact whose lifecycle is the phase's core risk. Documenting the new state explicitly per the same discipline.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data (new derived) | Bleve scorch index at `<data_dir>/index/` — NOT in Git, rebuildable | New: create on startup; rebuild-from-files is the recovery path. Must be excluded from Git and from the content repo entirely. |
| Stored data (new metadata) | `last_indexed_head` (Git SHA) — operational bookkeeping for drift detection | New SQLite metadata row (new migration, e.g. `0007_search.sql`, or a `kv`/`meta` table). Written after each successful rebuild/incremental batch. |
| Live service config | `SearchConfig{Enabled, Engine}` already parsed-but-unused in `config.go` | Wire it: gate the endpoint + worker registration on `Search.Enabled`; `Engine` defaults to `bleve`. No new top-level config section needed. |
| Secrets/env vars | None — search touches no secrets. | None. |
| Build artifacts | `go.mod`/`go.sum` gain `blevesearch/*` modules; binary grows | Commit `go.sum` after `go mod tidy`; confirm `CGO_ENABLED=0 go build` still succeeds. |
| OS-registered state | None. | None — verified: no systemd/cron/OS registration in this phase. |

**Critical lifecycle invariant:** the index is a *cache of the files*. Any time the two can diverge (crash mid-batch, manual Git pull, restore-from-trash, failed incremental job), the **rebuild-from-files** path must be able to reconverge them. The startup HEAD check is the automatic trigger; the admin reindex is the manual one.

## Common Pitfalls

### Pitfall 1: Index/files drift (the phase's primary risk)
**What goes wrong:** A page is committed but its index update is lost (worker crash, failed job, process restart between commit and index), or Git HEAD changes out-of-band (`pull_on_startup`, restore). Search then shows stale/missing/ghost results.
**Why it happens:** The index is a separate, non-transactional store from the Git working tree; incremental jobs are best-effort.
**How to avoid:** Persist `last_indexed_head`; on startup compare to `gitstore.HeadSHA()` and rebuild on mismatch (or when the index dir is absent). Make `RebuildIndex` idempotent and atomic (build into `index.tmp/`, then swap dir). Provide the admin reindex. Treat incremental indexing as an optimization, rebuild as the source of correctness.
**Warning signs:** Search returns a page that was deleted; a renamed page appears under both names; a new page is unfindable until restart.

### Pitfall 2: `EnqueueAndWait` from a worker handler → deadlock (Phase 2 CR-01, already bitten)
**What goes wrong:** The single drain goroutine waits on a job it itself must drain → stalls every queued job until timeout.
**Why it happens:** One worker, one goroutine; waiting inside a handler blocks the drainer.
**How to avoid:** Incremental indexing uses `worker.Enqueue` (fire-and-forget). Extraction-done → index follows the exact pattern in `extractjob.go`. Do not block.
**Warning signs:** Saves/uploads hang ~`commitWaitTimeout`; worker appears wedged.

### Pitfall 3: Bleve index corruption / lock on crash
**What goes wrong:** A `bleve.Open` fails after an unclean shutdown, or two writers touch the index.
**Why it happens:** scorch is crash-tolerant but a half-written rebuild dir or a second writer breaks invariants.
**How to avoid:** Single writer only (the worker). On `bleve.Open` error at startup, log and fall through to a full rebuild into a fresh dir (never try to repair in place). Build rebuilds in `index.tmp/` and swap atomically so a crashed rebuild never replaces a good index. Always `Close()` the index on shutdown (defer in main, after `worker.Stop()`).
**Warning signs:** `bleve.Open` returns an error; search 500s on startup.

### Pitfall 4: Concurrent index access (single worker writes vs. HTTP read queries)
**What goes wrong:** Assuming reads must be serialized with writes, or sharing a writer across goroutines.
**Why it happens:** Misunderstanding Bleve's concurrency model.
**How to avoid:** Hold ONE shared `bleve.Index`. `Search` (reads) are safe concurrently with the single worker's `Index`/`Delete` (writes). Never create a second writer. During an atomic rebuild swap, briefly hold a mutex around the index pointer so in-flight queries use the old index until the swap, then the new one.
**Warning signs:** Data races (run tests with `-race`); panics under concurrent search + index.

### Pitfall 5: Stale heading sub-documents after a body edit
**What goes wrong:** Editing a heading's text leaves the old heading doc (old anchor ID) in the index → duplicate/ghost heading results.
**Why it happens:** Heading doc IDs encode the anchor; changing the heading changes the ID, orphaning the old one. Bleve has no delete-by-prefix.
**How to avoid:** On page upsert, delete the page's prior heading docs before re-indexing the new set. Track the prior set (e.g. re-scan the previous committed body via `gitstore`/last-known, or store the page→headingIDs set in SQLite), OR accept that the periodic/triggered rebuild reconciles it. Document the chosen approach (Open Questions Q2). The rebuild path is always correct because it starts from an empty index.
**Warning signs:** A heading that was renamed still shows under its old name.

### Pitfall 6: Highlight markup XSS + UI-SPEC violation (carries the Phase 1 stored-XSS guard)
**What goes wrong:** Bleve's default `html` highlighter wraps matched terms in `<mark>...</mark>` (a background fill). Returning that to the SPA and rendering via `dangerouslySetInnerHTML` is a stored-XSS vector AND violates the UI-SPEC (highlight must be weight-only `<strong>`, no `<mark>` fill, no accent color).
**Why it happens:** The convenient path is "return server HTML, dump into the DOM."
**How to avoid:** Two safe options: (a) configure a **custom fragment formatter** that wraps matches in `<strong>` (or a known-safe `<span class="search-hl">`) instead of `<mark>`, and on the SPA map ONLY those known tags to React elements (no raw HTML injection); or (b) have the server return the **plain fragment text plus match offsets** and let the SPA bold substrings itself. Either way: the user's query/content text is escaped, `rehype-raw` stays OFF (consistent with Phase 1), and no raw server HTML is injected. The HTML highlighter style id is the string `"html"` [VERIFIED: bleve@v2.6.0 search/highlight/highlighter/html] — its default formatter uses `<mark>`, which is why a custom formatter or offset approach is required.
**Warning signs:** `<mark>` tags or yellow highlight in results; `dangerouslySetInnerHTML` anywhere in the palette.

### Pitfall 7: Reading tags from frontmatter (scalar vs. sequence)
**What goes wrong:** `okf.Field(doc, okf.FieldTags)` returns a scalar; tags are typically a YAML **sequence** (`tags: [a, b]`), so `Field` may return "" or the wrong shape.
**Why it happens:** `okf.Field` reads top-level **scalar** nodes only (see `repair.go`); it is not built for sequences.
**How to avoid:** Read the tags node from `doc.Front` (the parsed `yaml.Node`) directly and collect sequence items, or add a small `okf.FieldList(doc, FieldTags) []string` helper. Index each tag as a keyword. Do NOT rely on `okf.Field` for tags. (Body and title are fine: `Body` is a struct field; title is a scalar via `Field`.)
**Warning signs:** Tag search (SRCH-03) returns nothing for pages that clearly have tags.

## Code Examples

### Open-or-create the on-disk index (startup)
```go
// Source: bleve v2.6.0 index.go — New(path, mapping) / Open(path)
func OpenOrCreate(dir string) (bleve.Index, error) {
	idx, err := bleve.Open(dir)        // existing index
	if err == nil {
		return idx, nil
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		return bleve.New(dir, buildMapping())  // scorch by default
	}
	// Any other open error (corruption): caller logs and triggers a rebuild
	// into a fresh dir rather than repairing in place (Pitfall 3).
	return nil, err
}
```
[VERIFIED: bleve@v2.6.0 index.go — `func New(path string, mapping mapping.IndexMapping) (Index, error)`, `func Open(path string) (Index, error)`]

### Upsert / delete a page document
```go
// Index = upsert (same ID overwrites). Delete removes by ID.
func (s *Service) indexPage(pagePath string) error {
	raw, err := s.repo.Read(pagePath)
	if err != nil { return err }
	doc, err := okf.Parse(raw)
	if err != nil { return err }
	title := okf.Field(doc, okf.FieldTitle)
	tags := readTags(doc)               // sequence-aware (Pitfall 7)
	if err := s.index.Index(pagePath, map[string]interface{}{
		"type":      TypePage,
		"title":     title,
		"body":      string(doc.Body),  // opaque text; never re-emitted
		"tags":      tags,
		"page_path": pagePath,
	}); err != nil { return err }
	return s.indexHeadings(pagePath, title, doc.Body)
}

func (s *Service) deletePage(pagePath string) error {
	// also delete heading docs for this page (Pitfall 5)
	return s.index.Delete(pagePath)
}
```
[VERIFIED: bleve@v2.6.0 — `Index.Index(id string, data interface{}) error`, `Index.Delete(id string) error` (index.go Index interface)]

### Idempotent atomic rebuild
```go
func (s *Service) RebuildIndex(ctx context.Context) error {
	tmp := s.dir + ".tmp"
	_ = os.RemoveAll(tmp)
	idx, err := bleve.New(tmp, buildMapping())
	if err != nil { return err }
	// Walk live pages (skip .okf-workspace/trash), then attachments.
	items, err := s.repo.Tree()
	if err != nil { idx.Close(); return err }
	batch := idx.NewBatch()
	for _, it := range items {
		if it.IsDir || isSkipped(it.Path) { continue }
		if strings.HasSuffix(it.Path, ".md") { /* batch.Index page + headings */ }
	}
	for _, m := range s.allAttachmentMetas() { /* batch.Index attachment with PagePath */ }
	if err := idx.Batch(batch); err != nil { idx.Close(); return err }
	if err := idx.Close(); err != nil { return err }
	// Atomic swap: close the live index pointer under a mutex, os.Rename tmp→dir.
	return s.swapIndexDir(tmp)
}
```
[VERIFIED: bleve@v2.6.0 — `Index.NewBatch() *Batch`, `Batch.Index(id, data)`, `Index.Batch(*Batch) error` (index.go); `os.Rename` for atomic dir swap on same filesystem]

### Owning-page mapping for attachments (SRCH-05)
```go
// The meta sidecar ALREADY carries PagePath — no page scan needed.
meta, err := readMeta(s.repo, id)         // internal/attachments
_ = s.index.Index("att:"+id, map[string]interface{}{
	"type":           TypeAttachment,
	"filename":       meta.OriginalName,
	"extracted_text": string(txtBytes),    // from TxtPath(id)
	"page_path":      meta.PagePath,       // owning page → result links here
	"page_title":     s.titleOfPage(meta.PagePath),
})
```
[CITED: internal/attachments/types.go (`AttachmentMeta.PagePath`), internal/attachments/id.go (`TxtPath`, `MetaPath`)]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Bleve `upsidedown` + boltdb KV store | `scorch` segmented index (default) | scorch GA in bleve v2 | Pure-Go, better write throughput, crash-tolerant; `bleve.New` selects scorch by default — no `NewUsing` needed. |
| Return server HTML highlight + inject | Weight-only fragments / offsets, sanitized | UI-SPEC + Phase 1 XSS guard | Avoids stored-XSS; matches Obsidian-style weight-only highlight. |

**Deprecated/outdated:**
- Default `<mark>` HTML highlighter for end-user-facing fragments: not deprecated in Bleve, but **not acceptable** here (use custom formatter / offsets per Pitfall 6).
- SQLite FTS5: explicitly rejected/LOCKED-against in CLAUDE.md for this project.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `gitstore` needs a small new exported `HeadSHA(ctx)` (`git rev-parse HEAD`); the existing `git` runner is unexported and only `BlobRevision` (per-path) is exposed. | Standard Stack / Pitfall 1 | If a HEAD accessor already exists under another name, the new method is redundant (harmless). Low risk. |
| A2 | The `en` (English) analyzer is the right default for title/body/extracted-text (stemming + stop words). | Pattern 1 | If content is heavily non-English, recall suffers; `standard` analyzer is the fallback. Mappable later; no schema break for a rebuild. |
| A3 | GitHub-style heading anchors (`lowercase, spaces→-, strip punctuation`) match the SPA's Markdown renderer's heading IDs, so `#anchor` deep-links resolve. | Pattern 3 / Don't Hand-Roll | If `react-markdown` (or its slug plugin) uses a different slug algorithm, heading-result deep-links land at the page top, not the section. Must verify against the actual renderer (Q3). MEDIUM risk. |
| A4 | `bleve.ErrorIndexPathDoesNotExist` is the sentinel returned by `Open` for a missing index dir at v2.6.0. | Code Examples | If the sentinel name differs, the open-or-create branch needs the correct comparison; verify at implementation time. Low risk (well-known API). |
| A5 | scorch (default index type) needs no cgo, so `CGO_ENABLED=0 go build` still produces one static binary after adding bleve. | Summary / Runtime State | If a transitive dep pulls cgo, the single-binary promise breaks; mitigated by the explicit `CGO_ENABLED=0 go build ./...` verification step. Low risk (scorch is pure-Go by design). |

## Open Questions

1. **HEAD accessor for drift bookkeeping.**
   - What we know: `gitstore` shells `git` via an unexported `g.git(ctx, ...)`; it exposes `BlobRevision` (per-path) and runs `rev-parse` internally.
   - What's unclear: there is no exported `HeadSHA`/`Head` method.
   - Recommendation: add `func (g *GitStore) HeadSHA(ctx) (string, error)` (`git rev-parse HEAD`, empty on no-HEAD) — one small method, mirrors `BlobRevision`. Plan a task for it.

2. **Stale heading sub-doc cleanup strategy on body edit.**
   - What we know: heading IDs encode the anchor; Bleve has no delete-by-prefix; the rebuild path is always correct.
   - What's unclear: whether to (a) track page→headingIDs in SQLite, (b) re-derive the prior set from the last committed body, or (c) rely solely on periodic/triggered rebuild for heading reconciliation.
   - Recommendation: simplest robust MVP = on each page upsert, re-derive the prior heading set from the page's previous on-disk/committed body before the edit is applied is awkward; instead track the page's current heading IDs in a tiny SQLite table (`page_headings(page_path, heading_id)`), delete that set, re-index, and rewrite the set. The rebuild remains the backstop. Decide at planning.

3. **Heading anchor algorithm must match the SPA renderer.**
   - What we know: deep-link results jump to `#anchor`; the anchor must equal the id the rendered page assigns to that heading.
   - What's unclear: which slug algorithm the SPA's `react-markdown` setup uses (raw `react-markdown` does not add heading ids by default — a `rehype-slug`-style plugin would).
   - Recommendation: inspect `web/src` page renderer during planning; if no heading-id plugin exists, either add `rehype-slug` (GitHub slugs) on the read view and mirror its algorithm in the Go scanner, or have the SPA scroll-to-heading by text match. This is the one cross-tier coupling to nail down.

4. **`config.json` `nyquist_validation` is true** → Validation Architecture section included below.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build the whole binary | ✓ | go1.26.0 | — |
| `git` CLI | HEAD bookkeeping (drift) + existing commit spine | ✓ | (used by gitstore already) | — |
| `github.com/blevesearch/bleve/v2` | The index | ✗ (not yet in go.mod) | v2.6.0 (to add) | none — it is the locked engine; `go get` adds it |
| cgo / C toolchain | NOT required (scorch is pure-Go) | n/a | — | build with `CGO_ENABLED=0` |

**Missing dependencies with no fallback:** `bleve/v2` is not yet a module dependency — the first task must `go get github.com/blevesearch/bleve/v2@v2.6.0 && go mod tidy` and confirm `CGO_ENABLED=0 go build ./...` succeeds. This is expected (the phase introduces it), not a blocker.
**Missing dependencies with fallback:** none.

Note: this environment reports `CGO_ENABLED=1` by default, but CLAUDE.md mandates `CGO_ENABLED=0` builds; scorch supports that. Verify explicitly after adding bleve.

## Validation Architecture

> `workflow.nyquist_validation` is `true` in `.planning/config.json` — section included.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (table-driven; `t.TempDir()` real-repo/DB harnesses — see `internal/attachments/extractjob_test.go`, `internal/pages/service_test.go`) |
| Config file | none (Go convention; `go test ./...`) |
| Quick run command | `go test ./internal/search/... -run TestSearch -count=1` |
| Full suite command | `go test ./... -race` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SRCH-01 | Title search ranks title matches above body | unit | `go test ./internal/search/ -run TestQuery_TitleBoost` | ❌ Wave 0 |
| SRCH-02 | Body full-text search finds a term in body | unit | `go test ./internal/search/ -run TestQuery_Body` | ❌ Wave 0 |
| SRCH-03 | Tag search (keyword) finds a page by tag | unit | `go test ./internal/search/ -run TestQuery_Tag` | ❌ Wave 0 |
| SRCH-04 | Filename search finds an attachment by original name | unit | `go test ./internal/search/ -run TestQuery_Filename` | ❌ Wave 0 |
| SRCH-05 | Extracted-text hit returns the owning page path | unit | `go test ./internal/search/ -run TestQuery_AttachmentOwningPage` | ❌ Wave 0 |
| SRCH-06 | Results carry type page/heading/attachment + facet | unit | `go test ./internal/search/ -run TestQuery_TypedResultsAndFacet` | ❌ Wave 0 |
| (lifecycle) | Rebuild-from-files is idempotent; excludes trash | unit | `go test ./internal/search/ -run TestRebuild_Idempotent` | ❌ Wave 0 |
| (lifecycle) | Delete-to-trash removes page + its heading docs | unit | `go test ./internal/search/ -run TestIndex_DeleteRemovesHeadings` | ❌ Wave 0 |
| (drift) | Startup HEAD-mismatch triggers rebuild | unit | `go test ./internal/search/ -run TestDrift_HeadMismatchRebuilds` | ❌ Wave 0 |
| (concurrency) | Concurrent search + index does not race | unit | `go test ./internal/search/ -race -run TestIndex_ConcurrentReadWrite` | ❌ Wave 0 |
| (HTTP) | `GET /search` authed-only; returns typed JSON | integration | `go test ./internal/server/ -run TestSearchEndpoint` | ❌ Wave 0 |
| (HTTP) | `POST /search/reindex` admin-only (403 for editor) | integration | `go test ./internal/server/ -run TestReindexAdminOnly` | ❌ Wave 0 |
| (XSS) | Highlight fragments are weight-only / no `<mark>` raw HTML | unit | `go test ./internal/search/ -run TestHighlight_WeightOnlySafe` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/search/... -count=1` (+ `go vet ./internal/search/...`)
- **Per wave merge:** `go test ./... -race`
- **Phase gate:** `CGO_ENABLED=0 go build ./...` green + full `go test ./... -race` green before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] `internal/search/query_test.go` — SRCH-01..06 query behavior (build a small in-memory or `t.TempDir()` index, index fixtures, assert hits/types/facets)
- [ ] `internal/search/rebuild_test.go` — idempotent rebuild, trash exclusion, drift trigger
- [ ] `internal/search/indexjob_test.go` — fire-and-forget `KindIndex` handler (reuse the `fakeEnqueuer` pattern from `extractjob_test.go`)
- [ ] `internal/server/handlers_search_test.go` — authz (authed search, admin reindex), JSON shape, no Git vocabulary in errors
- [ ] Shared fixtures: a tiny corpus (1–2 pages with headings + tags, 1 attachment meta+txt) under `internal/search/testdata/`
- [ ] Module add: `go get github.com/blevesearch/bleve/v2@v2.6.0 && go mod tidy` (Wave 0 / first task)

## Security Domain

> `security_enforcement: true`, `security_asvs_level: 1`, `security_block_on: high`.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Reuse existing SCS session + `loadCurrentUser`; `GET /search` under the authed group. No new auth code. |
| V3 Session Management | yes (inherited) | Existing HTTPOnly/SameSite session cookie; search adds no new session surface. |
| V4 Access Control | yes | Search = any authenticated user (matches page-read model); admin reindex gated by `RequireRole(auth.RoleAdmin)` in the admin subgroup. Authz from SESSION role, never client input (T-00.03-01 pattern). Trashed pages excluded so search never reveals deleted content. |
| V5 Input Validation | yes | The `q` query param is untrusted: pass it to the Bleve query builder (parameterized — no string-built query DSL); cap length; reject empty. Path/id values used for indexing route through `repo.Resolve` (SEC-01). |
| V6 Cryptography | no | Search handles no secrets/crypto. |
| V7 Error Handling/Logging | yes | Error state must never leak server/index internals to the client (UI-SPEC: "Search is unavailable"); log details server-side only (mirrors the Phase 2 T-02-12 rule). Optionally audit admin reindex (SEC-05 pattern). |
| V12/V5 Output Encoding (XSS) | yes | Highlight fragments are the XSS surface — weight-only `<strong>`/offsets, sanitized, `rehype-raw` OFF, no `dangerouslySetInnerHTML` of raw server HTML (Pitfall 6; Phase 1 stored-XSS guard carried over). |

### Known Threat Patterns for Go + Bleve + React SPA

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Stored XSS via highlighted snippet markup | Tampering / Elevation | Custom weight-only fragment formatter or plain-text+offsets; SPA renders known tags only; `rehype-raw` off. |
| Information disclosure of trashed/deleted content via search | Information Disclosure | Exclude `.okf-workspace/trash`; `index.Delete` on delete-to-trash. |
| Path traversal while reading files to index | Tampering | All reads via `repo.Read`/`repo.Tree`/`repo.Resolve` (SEC-01); never `os.*`. |
| Index path escaping into the Git repo | Tampering | Index lives under `<data_dir>/index/`, outside the content repo entirely. |
| DoS via pathological query (huge fuzzy/prefix expansion) | DoS | Cap query length and result `Size`; fuzziness fixed at 1; single small workspace (~5 users) bounds load. |
| Leaking index/Git internals in error responses | Information Disclosure | Generic client error; details to slog only. |
| Privilege escalation on reindex | Elevation | Admin-only subgroup + CSRF (existing nosurf) on the POST. |

## Sources

### Primary (HIGH confidence)
- `proxy.golang.org/github.com/blevesearch/bleve/v2/@latest` and `/@v/v2.6.0.info` — confirmed v2.6.0 is latest, dated 2026-04-30 [VERIFIED]
- `github.com/blevesearch/bleve` tag `v2.6.0` source (raw.githubusercontent.com, refs/tags/v2.6.0):
  - `index.go` — `New`, `Open`, `NewUsing`, `NewMemOnly`, `OpenUsing`, `Index`/`Delete`/`NewBatch`/`Batch` interface [VERIFIED]
  - `mapping.go` / `mapping/index.go` / `mapping/document.go` / `mapping/field.go` — `NewIndexMapping`, `NewDocumentMapping`, `NewTextFieldMapping`, `NewKeywordFieldMapping`, `IndexMappingImpl{TypeField,DefaultAnalyzer,DefaultMapping,TypeMapping}`, `AddDocumentMapping`, `AddFieldMappingsAt`, `AddSubDocumentMapping`, `FieldMapping{Store,IncludeTermVectors,...}` [VERIFIED]
  - `search.go` — `NewSearchRequest(Options)`, `SearchRequest{Fields,Highlight,Facets}`, `AddFacet`, `NewHighlight(WithStyle)`, `NewFacetRequest`, `SearchResult{Hits,Facets,Total}`, `DocumentMatch{ID,Score,Fields,Fragments}` [VERIFIED]
  - `query.go` / `search/query/match.go` — `NewMatchQuery/NewMatchPhraseQuery/NewPrefixQuery/NewFuzzyQuery/NewTermQuery/NewConjunctionQuery/NewDisjunctionQuery/NewBooleanQuery/NewQueryStringQuery`, `MatchQuery.SetField/SetBoost/SetFuzziness/SetPrefix`, default fuzziness 0 [VERIFIED]
  - `search/highlight/highlighter/html/html.go` — highlighter style id `"html"` [VERIFIED]
- Codebase (this session, direct read): `internal/jobs/{queue,worker}.go`, `internal/pages/{commitjob,trash,tree}.go`, `internal/attachments/{extractjob,types,meta,refs,id}.go`, `internal/config/config.go`, `internal/server/router.go`, `internal/repo/*`, `internal/gitstore/{git,read}.go`, `cmd/okf-workspace/main.go` [VERIFIED]
- CLAUDE.md (Bleve v2.6.0 LOCKED; SQLite FTS5 rejected; content stays in files), 03-CONTEXT.md, 03-UI-SPEC.md, ROADMAP.md, REQUIREMENTS.md [CITED]

### Secondary (MEDIUM confidence)
- Bleve docstring on `IncludeTermVectors` ("required for phrase queries + term highlighting") — quoted from v2.6.0 source comment.

### Tertiary (LOW confidence)
- Heading anchor slug equivalence with the SPA renderer (Open Question 3) — must be verified against `web/src` at planning time.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — single locked library; version + all needed v2.6.0 APIs verified against tagged source.
- Architecture: HIGH — integration points read directly from the codebase; mirrors proven Phase 2 patterns (KindExtract/CR-01).
- Pitfalls: HIGH — drift/CR-01/XSS/concurrency are grounded in this codebase's own prior incidents and verified Bleve behavior; heading-cleanup + anchor-slug are flagged MEDIUM and routed to Open Questions.

**Research date:** 2026-06-21
**Valid until:** 2026-07-21 (stable; Bleve v2.6.0 is current. Re-verify only if a newer bleve major appears or the SPA Markdown renderer changes its heading-id scheme.)

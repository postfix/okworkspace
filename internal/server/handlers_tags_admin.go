// handlers_tags_admin.go — Phase 19 / v1.3 Tag Management Admin endpoints.
//
// Three admin-only endpoints for workspace-wide tag operations:
//   POST /admin/tags/rename  — rename a tag across all pages (TM-01)
//   POST /admin/tags/merge   — merge source tag into target (TM-02)
//   POST /admin/tags/delete  — remove a tag from all pages (TM-03)
//
// Every operation follows the preview→approve safety contract:
//   - preview=true returns the affected-page set without writing
//   - approve applies the mutation through SetTagsFrontmatter → ApplyTagsBatch
//     (N pages → ONE commit, the Phase 12 pattern)
//
// All three ride the EXISTING byte-stable SetTagsFrontmatter builder and the
// EXISTING single-writer EnqueueCommit path — they share the ONE tags-region
// write implementation, never introducing a second write path.
//
// Security: Admin-only RBAC (RequireRole(admin) + nosurf CSRF), server-side
// tag re-validation, audit logging, per-page 409 handling.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/tagmeta"
	"gopkg.in/yaml.v3"
)

// --- Request/response types ---

type tagRenameRequest struct {
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
	Preview bool   `json:"preview"` // true → return affected pages only, no write
}

type tagMergeRequest struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Preview bool   `json:"preview"`
}

type tagDeleteRequest struct {
	Tag     string `json:"tag"`
	Preview bool   `json:"preview"`
}

type tagBulkRequest struct {
	PagePaths  []string `json:"page_paths"`  // explicit selection
	FilterTags []string `json:"filter_tags"` // existing-tag filter (union)
	AddTags    []string `json:"add_tags"`
	RemoveTags []string `json:"remove_tags"`
	Preview    bool     `json:"preview"`
}

type tagPreviewResponse struct {
	Tag           string           `json:"tag"`
	AffectedPages []tagPreviewPage `json:"affected_pages"`
	TotalAffected int              `json:"total_affected"`
}

type tagPreviewPage struct {
	PagePath    string   `json:"page_path"`
	CurrentTags []string `json:"current_tags"`
	NewTags     []string `json:"new_tags"`
	// BodyAffected reports that the page's BODY #tags will also be rewritten (not
	// just the frontmatter tags:) — TR-05, so the admin sees prose will change.
	BodyAffected bool `json:"body_affected"`
}

type tagApplyResponse struct {
	Action  string                 `json:"action"`
	Results []pages.TagApplyResult `json:"results"`
}

// tagMetaRequest is the body of POST /admin/tags/meta (Phase 32 / TF-02,
// Phase 33 / TF-03). An admin sets a tag's color + description + parent.
// Empty color + description + parent = clear (the file is deleted).
type tagMetaRequest struct {
	Tag         string `json:"tag"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Parent      string `json:"parent"`
}

// tagMetaResponse is the return shape: the updated metadata for the tag.
type tagMetaResponse struct {
	Tag         string `json:"tag"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Parent      string `json:"parent"`
}

// normalizeTag trims whitespace and folds to lowercase. An empty string after
// normalization is rejected by the caller — normalizeTag itself doesn't error.
func normalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// readPageTags from raw bytes: parse with okf.Parse, extract the tags sequence
// from the frontmatter. Mirrors graph.pageTags' YAML-walk pattern exactly.
func readPageTags(raw []byte) ([]string, error) {
	doc, err := okf.Parse(raw)
	if err != nil || !doc.HasFrontmatter {
		return nil, nil
	}
	return extractTagsYAML(&doc.Front), nil
}

func extractTagsYAML(front *yaml.Node) []string {
	n := front
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value != okf.FieldTags {
			continue
		}
		val := n.Content[i+1]
		switch val.Kind {
		case yaml.SequenceNode:
			out := make([]string, 0, len(val.Content))
			for _, item := range val.Content {
				if item.Kind == yaml.ScalarNode {
					if t := strings.TrimSpace(item.Value); t != "" {
						out = append(out, t)
					}
				}
			}
			return out
		case yaml.ScalarNode:
			if t := strings.TrimSpace(val.Value); t != "" {
				return []string{t}
			}
		}
		return nil
	}
	return nil
}

// computeRenameTags replaces oldName with newName in the current tags.
// Deduplicates after normalization.
func computeRenameTags(current []string, oldName, newName string) ([]string, bool) {
	changed := false
	out := make([]string, 0, len(current))
	for _, t := range current {
		normalized := normalizeTag(t)
		if normalized == oldName {
			if !containsNormalized(out, newName) {
				out = append(out, newName)
			}
			changed = true
		} else {
			if !containsNormalized(out, normalized) {
				out = append(out, t)
			}
		}
	}
	return out, changed
}

func computeMergeTags(current []string, source, target string) ([]string, bool) {
	changed := false
	hasTarget := false
	out := make([]string, 0, len(current))
	for _, t := range current {
		normalized := normalizeTag(t)
		if normalized == source {
			changed = true
			continue // drop source
		}
		if normalized == target {
			hasTarget = true
		}
		if !containsNormalized(out, normalized) {
			out = append(out, t)
		}
	}
	if !hasTarget {
		out = append(out, target)
		changed = true
	}
	return out, changed
}

func computeDeleteTags(current []string, tag string) ([]string, bool) {
	changed := false
	out := make([]string, 0, len(current))
	for _, t := range current {
		normalized := normalizeTag(t)
		if normalized == tag {
			changed = true
			continue
		}
		if !containsNormalized(out, normalized) {
			out = append(out, t)
		}
	}
	return out, changed
}

func containsNormalized(tags []string, want string) bool {
	for _, t := range tags {
		if normalizeTag(t) == want {
			return true
		}
	}
	return false
}

// sameTagSet reports whether two tag slices hold the same normalized set
// (order-insensitive). nil and []string{} are equivalent.
func sameTagSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[normalizeTag(t)] = true
	}
	for _, t := range b {
		n := normalizeTag(t)
		if !set[n] {
			return false
		}
		delete(set, n)
	}
	return len(set) == 0
}

// computeBulkTags returns a compute callback that yields (current ∪ addTags)
// \ removeTags, normalized + deduped, preserving the original tag ordering of
// current then appending new tags in addTags order. The bool is true when the
// resulting set differs from current (order-insensitive).
func computeBulkTags(addTags, removeTags []string) func(current []string) ([]string, bool) {
	removeSet := make(map[string]bool)
	for _, t := range removeTags {
		removeSet[normalizeTag(t)] = true
	}
	return func(current []string) ([]string, bool) {
		seen := make(map[string]bool)
		var result []string
		for _, t := range current {
			n := normalizeTag(t)
			if removeSet[n] {
				continue
			}
			if !seen[n] {
				seen[n] = true
				result = append(result, t)
			}
		}
		for _, t := range addTags {
			n := normalizeTag(t)
			if removeSet[n] {
				continue
			}
			if !seen[n] {
				seen[n] = true
				result = append(result, n)
			}
		}
		changed := !sameTagSet(current, result)
		return result, changed
	}
}

// buildTagApplyItems reads each page via pages.Get, computes the new tag list,
// and builds TagApplyItem entries for ApplyTagsBatch. bodyOld/bodyNew carry the
// op's BODY-rewrite intent (rename: old→new; merge: source→target; delete:
// tag→""; bodyOld=="" disables the body channel). A page is included when its
// frontmatter tag set changes OR its body #tags will change (the widened
// predicate, TR-03) — so a body-only page (tag only in prose, frontmatter
// unchanged) is no longer dropped. For a body-only page SkipFrontmatter keeps the
// tags region byte-identical; only the body is rewritten. The authoritative body
// rewrite happens in the batch from ITS freshly-read body (TOCTOU-safe) — here we
// compute bodyWillChange from the handler's read only to decide inclusion + mark
// the preview.
func (h *authHandlers) buildTagApplyItems(
	ctx context.Context,
	pagePaths []string,
	compute func(current []string) ([]string, bool),
	bodyOld, bodyNew string,
) ([]pages.TagApplyItem, []tagPreviewPage, error) {
	items := make([]pages.TagApplyItem, 0, len(pagePaths))
	preview := make([]tagPreviewPage, 0, len(pagePaths))

	for _, p := range pagePaths {
		pg, err := h.pages.Get(ctx, p)
		if err != nil {
			return nil, nil, fmt.Errorf("get %q: %w", p, err)
		}
		src := pages.AssembleSource(pg.Frontmatter, pg.Body)
		currentTags, err := readPageTags(src)
		if err != nil {
			return nil, nil, fmt.Errorf("parse tags %q: %w", p, err)
		}
		if currentTags == nil {
			currentTags = []string{}
		}
		newTags, changed := compute(currentTags)

		bodyWillChange := false
		if bodyOld != "" {
			_, bodyWillChange = okf.RewriteHashtags([]byte(pg.Body), bodyOld, bodyNew)
		}

		preview = append(preview, tagPreviewPage{
			PagePath:     p,
			CurrentTags:  currentTags,
			NewTags:      newTags,
			BodyAffected: bodyWillChange,
		})
		if changed || bodyWillChange {
			items = append(items, pages.TagApplyItem{
				PagePath:        p,
				Tags:            newTags,
				BaseRevision:    pg.Revision,
				SkipFrontmatter: !changed, // body-only page: leave tags region as-is
				BodyOld:         bodyOld,
				BodyNew:         bodyNew,
			})
		}
	}
	return items, preview, nil
}

// actorFromSession extracts the actor name from the session-bound user.
func (h *authHandlers) actorFromSession(r *http.Request) string {
	if _, ok := auth.CurrentUser(r.Context()); ok {
		return h.actorUsername(r.Context())
	}
	// This should never happen — RequireRole middleware ensures a session exists.
	return "unknown"
}

// --- Handlers ---

// handleTagRename handles POST /admin/tags/rename (TM-01).
func (h *authHandlers) handleTagRename(w http.ResponseWriter, r *http.Request) {
	var req tagRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	oldName := normalizeTag(req.OldName)
	newName := normalizeTag(req.NewName)
	if oldName == "" || newName == "" {
		writeError(w, http.StatusBadRequest, "Tag name must not be empty.")
		return
	}
	if err := okf.ValidateInlineTag(newName); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag name: "+err.Error())
		return
	}
	// SEC-v1.5-F01: the target is written to BOTH the body (splice-safe →
	// ValidateInlineTag) AND the frontmatter tags: key (charset-safe →
	// ValidateTagCharset). The frontmatter charset forbids '/', which the body
	// grammar permits, so a nested target like "a/b" passes the inline gate but
	// would fail at the frontmatter write. Require the INTERSECTION here so such a
	// target is a clean 400 at the gate, not a 500 at apply.
	if err := okf.ValidateTagCharset(newName); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag name: "+err.Error())
		return
	}

	// Case-only rename → no-op.
	if oldName == newName {
		writeJSON(w, http.StatusOK, tagApplyResponse{
			Action: "rename", Results: []pages.TagApplyResult{},
		})
		return
	}

	actor := h.actorFromSession(r)
	ctx := r.Context()

	fmPaths, err := h.graph.PagesByTag(ctx, oldName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list affected pages.")
		return
	}
	bodyPaths, err := h.pages.PagesWithBodyTag(ctx, oldName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list affected pages.")
		return
	}
	pagePaths := uniqueStrings(append(fmPaths, bodyPaths...))

	computeFn := func(current []string) ([]string, bool) {
		return computeRenameTags(current, oldName, newName)
	}
	items, preview, err := h.buildTagApplyItems(ctx, pagePaths, computeFn, oldName, newName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.Preview {
		writeJSON(w, http.StatusOK, tagPreviewResponse{
			Tag:           oldName,
			AffectedPages: preview,
			TotalAffected: len(preview),
		})
		return
	}

	if len(items) == 0 {
		writeJSON(w, http.StatusOK, tagApplyResponse{
			Action: "rename", Results: []pages.TagApplyResult{},
		})
		return
	}

	results, err := h.pages.ApplyTagsBatch(ctx, items, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to apply tag rename.")
		return
	}
	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionTagRename,
		Actor:  actor,
		Target: oldName + "→" + newName,
		Detail: fmt.Sprintf("%d pages", len(items)),
	})
	h.journalOp(ctx, "rename", map[string]string{"old_name": oldName, "new_name": newName}, preview)
	writeJSON(w, http.StatusOK, tagApplyResponse{Action: "rename", Results: results})
}

// handleTagMerge handles POST /admin/tags/merge (TM-02).
func (h *authHandlers) handleTagMerge(w http.ResponseWriter, r *http.Request) {
	var req tagMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	source := normalizeTag(req.Source)
	target := normalizeTag(req.Target)
	if source == "" || target == "" {
		writeError(w, http.StatusBadRequest, "Tag name must not be empty.")
		return
	}
	if source == target {
		writeError(w, http.StatusBadRequest, "Source and target tags must differ.")
		return
	}
	if err := okf.ValidateInlineTag(target); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag name: "+err.Error())
		return
	}
	// SEC-v1.5-F01: target must satisfy BOTH the body-splice (ValidateInlineTag)
	// and frontmatter-charset (ValidateTagCharset) invariants — see handleTagRename.
	// A nested target like "a/b" is a clean 400 here, not a 500 at the frontmatter write.
	if err := okf.ValidateTagCharset(target); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag name: "+err.Error())
		return
	}

	actor := h.actorFromSession(r)
	ctx := r.Context()

	// Frontmatter source + target pages (target pages may need a frontmatter merge)
	// unioned with body pages carrying #source in prose (their #source→#target body
	// rewrite). Body pages for #target are NOT enumerated — the merge only rewrites
	// #source, and adding #target to a page that merely mentions it would be wrong.
	sourcePaths, _ := h.graph.PagesByTag(ctx, source)
	targetPaths, _ := h.graph.PagesByTag(ctx, target)
	bodySourcePaths, err := h.pages.PagesWithBodyTag(ctx, source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list affected pages.")
		return
	}
	allPaths := uniqueStrings(append(append(sourcePaths, targetPaths...), bodySourcePaths...))

	computeFn := func(current []string) ([]string, bool) {
		return computeMergeTags(current, source, target)
	}
	items, preview, err := h.buildTagApplyItems(ctx, allPaths, computeFn, source, target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.Preview {
		writeJSON(w, http.StatusOK, tagPreviewResponse{
			Tag:           source + "→" + target,
			AffectedPages: preview,
			TotalAffected: len(preview),
		})
		return
	}

	if len(items) == 0 {
		writeJSON(w, http.StatusOK, tagApplyResponse{
			Action: "merge", Results: []pages.TagApplyResult{},
		})
		return
	}

	results, err := h.pages.ApplyTagsBatch(ctx, items, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to apply tag merge.")
		return
	}
	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionTagMerge,
		Actor:  actor,
		Target: source + "→" + target,
		Detail: fmt.Sprintf("%d pages", len(items)),
	})
	h.journalOp(ctx, "merge", map[string]string{"source": source, "target": target}, preview)
	writeJSON(w, http.StatusOK, tagApplyResponse{Action: "merge", Results: results})
}

// handleTagDelete handles POST /admin/tags/delete (TM-03).
func (h *authHandlers) handleTagDelete(w http.ResponseWriter, r *http.Request) {
	var req tagDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	tag := normalizeTag(req.Tag)
	if tag == "" {
		writeError(w, http.StatusBadRequest, "Tag name must not be empty.")
		return
	}

	actor := h.actorFromSession(r)
	ctx := r.Context()

	fmPaths, err := h.graph.PagesByTag(ctx, tag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list affected pages.")
		return
	}
	bodyPaths, err := h.pages.PagesWithBodyTag(ctx, tag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list affected pages.")
		return
	}
	pagePaths := uniqueStrings(append(fmPaths, bodyPaths...))

	computeFn := func(current []string) ([]string, bool) {
		return computeDeleteTags(current, tag)
	}
	// Delete: body #tag removed (new==""); no new-token validation needed.
	items, preview, err := h.buildTagApplyItems(ctx, pagePaths, computeFn, tag, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.Preview {
		writeJSON(w, http.StatusOK, tagPreviewResponse{
			Tag:           tag,
			AffectedPages: preview,
			TotalAffected: len(preview),
		})
		return
	}

	if len(items) == 0 {
		writeJSON(w, http.StatusOK, tagApplyResponse{
			Action: "delete", Results: []pages.TagApplyResult{},
		})
		return
	}

	results, err := h.pages.ApplyTagsBatch(ctx, items, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to apply tag delete.")
		return
	}
	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionTagDelete,
		Actor:  actor,
		Target: tag,
		Detail: fmt.Sprintf("%d pages", len(items)),
	})
	h.journalOp(ctx, "delete", map[string]string{"tag": tag}, preview)
	writeJSON(w, http.StatusOK, tagApplyResponse{Action: "delete", Results: results})
}

// handleTagBulk handles POST /admin/tags/bulk (TF-01). An admin adds and/or
// removes multiple tags across a selected set of pages (explicit paths ∪
// filter-tags) in ONE byte-stable preview→approve batch commit, reusing
// buildTagApplyItems + ApplyTagsBatch. Frontmatter-only: bodyOld/bodyNew are
// "" so the body channel is disabled (no RewriteHashtags). Each added tag is
// re-validated via okf.ValidateTagCharset (the frontmatter charset gate).
// The page set is re-resolved server-side at BOTH preview and apply (TOCTOU).
func (h *authHandlers) handleTagBulk(w http.ResponseWriter, r *http.Request) {
	var req tagBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	// Normalize inputs.
	addTags := make([]string, 0, len(req.AddTags))
	for _, t := range req.AddTags {
		n := normalizeTag(t)
		if n != "" {
			addTags = append(addTags, n)
		}
	}
	removeTags := make([]string, 0, len(req.RemoveTags))
	for _, t := range req.RemoveTags {
		n := normalizeTag(t)
		if n != "" {
			removeTags = append(removeTags, n)
		}
	}
	filterTags := make([]string, 0, len(req.FilterTags))
	for _, t := range req.FilterTags {
		n := normalizeTag(t)
		if n != "" {
			filterTags = append(filterTags, n)
		}
	}
	pagePaths := make([]string, 0, len(req.PagePaths))
	for _, p := range req.PagePaths {
		p = strings.TrimSpace(p)
		if p != "" {
			pagePaths = append(pagePaths, p)
		}
	}

	// Validate: need a page-set spec AND an op spec.
	if len(pagePaths) == 0 && len(filterTags) == 0 {
		writeError(w, http.StatusBadRequest, "Provide page_paths or filter_tags to select pages.")
		return
	}
	if len(addTags) == 0 && len(removeTags) == 0 {
		writeError(w, http.StatusBadRequest, "Provide add_tags or remove_tags to change.")
		return
	}

	// Validate each added tag's charset (frontmatter gate — added tags are
	// written; removed/filter tags are matched, not written, so no charset gate).
	for _, t := range addTags {
		if err := okf.ValidateTagCharset(t); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid tag name: "+err.Error())
			return
		}
	}

	actor := h.actorFromSession(r)
	ctx := r.Context()

	// Resolve the page set server-side: explicit paths ∪ pages matching any
	// filter-tag (union). Deduped + sorted for determinism.
	resolved := make(map[string]bool, len(pagePaths))
	for _, p := range pagePaths {
		resolved[p] = true
	}
	for _, ft := range filterTags {
		paths, err := h.graph.PagesByTag(ctx, ft)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to resolve filter tags.")
			return
		}
		for _, p := range paths {
			resolved[p] = true
		}
	}
	pageSet := make([]string, 0, len(resolved))
	for p := range resolved {
		pageSet = append(pageSet, p)
	}
	sort.Strings(pageSet)

	computeFn := computeBulkTags(addTags, removeTags)
	// Frontmatter-only: bodyOld="" disables the body channel (no body rewrite).
	items, preview, err := h.buildTagApplyItems(ctx, pageSet, computeFn, "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.Preview {
		writeJSON(w, http.StatusOK, tagPreviewResponse{
			Tag:           "bulk",
			AffectedPages: preview,
			TotalAffected: len(preview),
		})
		return
	}

	if len(items) == 0 {
		writeJSON(w, http.StatusOK, tagApplyResponse{
			Action: "bulk", Results: []pages.TagApplyResult{},
		})
		return
	}

	results, err := h.pages.ApplyTagsBatch(ctx, items, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to apply bulk tag change.")
		return
	}
	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionTagBulk,
		Actor:  actor,
		Target: "bulk",
		Detail: fmt.Sprintf("%d pages, +%v -%v", len(items), addTags, removeTags),
	})
	h.journalOp(ctx, "bulk", map[string]interface{}{"add_tags": addTags, "remove_tags": removeTags}, preview)
	writeJSON(w, http.StatusOK, tagApplyResponse{Action: "bulk", Results: results})
}

// handleTagList handles GET /admin/tags (TM-04). Returns the workspace tag
// vocabulary with per-tag page counts from the derived graph cache, merged
// with per-tag color + description + parent from the tag-metadata cache
// (Phase 32 / TF-02, Phase 33 / TF-03). Tags with no metadata get empty
// color/description/parent (graceful).
func (h *authHandlers) handleTagList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.graph == nil {
		writeError(w, http.StatusInternalServerError, "Graph store not available.")
		return
	}
	counts, err := h.graph.TagPageCounts(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list tags.")
		return
	}
	// Build a tag→Meta lookup from the metadata cache (one call) so the list
	// merges color/description/parent inline without a second fetch per tag.
	metaByTag := map[string]tagmeta.Meta{}
	if h.tagmeta != nil {
		metas, merr := h.tagmeta.AllMeta(ctx)
		if merr == nil {
			for _, m := range metas {
				metaByTag[m.Tag] = m
			}
		}
		// A metadata read error is non-fatal: the list still renders tags with
		// empty color/description/parent (graceful degradation, same as no metadata).
	}
	type tagEntry struct {
		Tag         string `json:"tag"`
		Count       int    `json:"count"`
		Color       string `json:"color,omitempty"`
		Description string `json:"description,omitempty"`
		Parent      string `json:"parent,omitempty"`
	}
	out := make([]tagEntry, 0, len(counts))
	for tag, c := range counts {
		m := metaByTag[tag] // zero-value Meta{} when absent → empty fields
		out = append(out, tagEntry{Tag: tag, Count: c, Color: m.Color, Description: m.Description, Parent: m.Parent})
	}
	writeJSON(w, http.StatusOK, out)
}

// tagCountPair is a {tag, count} entry in the analytics summary.
type tagCountPair struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// tagAnalyticsResponse is the JSON payload for GET /admin/tags/analytics
// (Phase 34 / TF-04). Read-only: it never mutates. Sources per-tag counts from
// graph.Store.TagPageCounts (the existing page_tags cache query — no new scan),
// tagged/untagged page totals from the live page tree + the distinct-tagged-page
// count, most/least used (top/bottom 5 by count), and orphans (tags that exist as
// metadata but are carried by zero pages). The scope is frontmatter page_tags
// only — body-only #tags are counted only after materialization.
type tagAnalyticsResponse struct {
	TotalTags     int            `json:"total_tags"`
	TotalPages    int            `json:"total_pages"`
	TaggedPages   int            `json:"tagged_pages"`
	UntaggedPages int            `json:"untagged_pages"`
	MostUsed      []tagCountPair `json:"most_used"`
	LeastUsed     []tagCountPair `json:"least_used"`
	Orphans       []tagCountPair `json:"orphans"`
	Tags          []tagCountPair `json:"tags"`
}

// handleTagAnalytics handles GET /admin/tags/analytics (Phase 34 / TF-04).
// Admin-only, read-only: returns a per-tag usage summary sourced from
// graph.Store.TagPageCounts (the existing page_tags cache query — no new heavy
// scan). Computes totals, most/least used (top/bottom 5), and orphans (tags
// defined as metadata but carried by zero pages). The scope is frontmatter
// page_tags only; body-only #tags are counted only after materialization.
func (h *authHandlers) handleTagAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.graph == nil {
		writeError(w, http.StatusInternalServerError, "Graph store not available.")
		return
	}
	counts, err := h.graph.TagPageCounts(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read tag counts.")
		return
	}
	taggedPages, err := h.graph.TaggedPageCount(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read tagged-page count.")
		return
	}
	// Total live pages from the page tree (a recursive walk is not needed: Tree
	// already flattens to leaf nodes; we count type=="page"). When the pages
	// service is unavailable, total_pages falls back to the tagged count so
	// untagged is reported as 0 rather than a meaningless negative.
	totalPages := taggedPages
	if h.pages != nil {
		nodes, perr := h.pages.Tree(ctx)
		if perr == nil {
			totalPages = countPageNodes(nodes)
		}
	}
	if totalPages < taggedPages {
		totalPages = taggedPages
	}
	// Orphans: tags that exist as metadata but are carried by zero pages.
	var orphans []tagCountPair
	if h.tagmeta != nil {
		metas, merr := h.tagmeta.AllMeta(ctx)
		if merr == nil {
			for _, m := range metas {
				if counts[m.Tag] == 0 {
					orphans = append(orphans, tagCountPair{Tag: m.Tag, Count: 0})
				}
			}
		}
		// A metadata read error is non-fatal: orphans is left empty.
	}

	// Build the flat per-tag list + most/least used (top/bottom 5 by count).
	tags := make([]tagCountPair, 0, len(counts))
	for tag, c := range counts {
		tags = append(tags, tagCountPair{Tag: tag, Count: c})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Tag < tags[j].Tag })

	mostUsed := topNByCount(tags, 5, true)
	leastUsed := topNByCount(tags, 5, false)

	if orphans == nil {
		orphans = []tagCountPair{}
	}
	if mostUsed == nil {
		mostUsed = []tagCountPair{}
	}
	if leastUsed == nil {
		leastUsed = []tagCountPair{}
	}

	writeJSON(w, http.StatusOK, tagAnalyticsResponse{
		TotalTags:     len(counts),
		TotalPages:    totalPages,
		TaggedPages:   taggedPages,
		UntaggedPages: totalPages - taggedPages,
		MostUsed:      mostUsed,
		LeastUsed:     leastUsed,
		Orphans:       orphans,
		Tags:          tags,
	})
}

// countPageNodes recursively counts Tree nodes of type "page".
func countPageNodes(nodes []pages.Node) int {
	n := 0
	for _, node := range nodes {
		if node.Type == "page" {
			n++
		}
		if len(node.Children) > 0 {
			n += countPageNodes(node.Children)
		}
	}
	return n
}

// topNByCount returns the top (desc=true) or bottom (desc=false) N entries by
// count from a copy of tags, sorted by count then name for stable ordering.
func topNByCount(tags []tagCountPair, n int, desc bool) []tagCountPair {
	if len(tags) == 0 {
		return []tagCountPair{}
	}
	sorted := make([]tagCountPair, len(tags))
	copy(sorted, tags)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			if desc {
				return sorted[i].Count > sorted[j].Count
			}
			return sorted[i].Count < sorted[j].Count
		}
		return sorted[i].Tag < sorted[j].Tag
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

// handleTagVocabulary handles GET /tags/vocabulary — any-authed read returning
// the sorted distinct tag list for autocomplete UIs.
func (h *authHandlers) handleTagVocabulary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.graph == nil {
		writeError(w, http.StatusInternalServerError, "Graph store not available.")
		return
	}
	vocab, err := h.graph.Vocabulary(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read tag vocabulary.")
		return
	}
	writeJSON(w, http.StatusOK, vocab)
}

// handleTagMeta handles POST /admin/tags/meta (Phase 32 / TF-02, Phase 33 /
// TF-03). An admin sets or clears a tag's color + description + parent. The
// metadata persists on disk as Tags/{tag}.md via the single-writer commit path;
// the SQLite cache is updated inline. Server validation: tag via
// ValidateTagCharset, color via tagmeta.ValidateColor (hex/palette subset),
// description via tagmeta.ValidateDescription (single-line, HTML-safe, ≤200),
// parent via tagmeta.ValidateParent (valid tag name or empty). The cycle check
// (tagmeta.checkCycle) runs BEFORE the write — a cycle-producing parent is
// rejected with 400, never written to disk (D-02a). Empty color + description +
// parent = clear (the file is deleted). Admin-only RBAC + nosurf CSRF are
// inherited from the admin route subgroup.
func (h *authHandlers) handleTagMeta(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.tagmeta == nil {
		writeError(w, http.StatusInternalServerError, "Tag metadata store not available.")
		return
	}
	var req tagMetaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	tag := normalizeTag(req.Tag)
	if tag == "" {
		writeError(w, http.StatusBadRequest, "Tag is required.")
		return
	}
	if err := okf.ValidateTagCharset(tag); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag name.")
		return
	}
	if err := tagmeta.ValidateColor(req.Color); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid color.")
		return
	}
	if err := tagmeta.ValidateDescription(req.Description); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid description.")
		return
	}
	parent := normalizeTag(req.Parent)
	if err := tagmeta.ValidateParent(parent); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid parent tag name.")
		return
	}
	// Cycle check BEFORE the write (D-02a): a cycle-producing parent is rejected
	// with 400, never written to disk (which would create an un-rebuildable cycle).
	if err := h.tagmeta.CheckCycle(ctx, tag, parent); err != nil {
		writeError(w, http.StatusBadRequest, "Tag hierarchy cycle: "+err.Error())
		return
	}
	// Thread the authenticated actor onto the commit spec + audit record.
	actor := h.actorFromSession(r)
	h.tagmeta.SetUser(actor)
	if err := h.tagmeta.SetMeta(ctx, tag, req.Color, req.Description, parent); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to set tag metadata.")
		return
	}
	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionTagMeta,
		Actor:  actor,
		Target: tag,
		Detail: fmt.Sprintf("color=%s parent=%s", req.Color, parent),
		Source: "web-ui",
	})
	writeJSON(w, http.StatusOK, tagMetaResponse{
		Tag:         tag,
		Color:       req.Color,
		Description: req.Description,
		Parent:      parent,
	})
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// --- Tag-operation undo journal (Phase 35 / TF-05) ---

// journalPage is one page's invertible record captured at op time: its path,
// the frontmatter tags it carried BEFORE the op, and whether the op also
// rewrote its body #tags.
type journalPage struct {
	PagePath      string   `json:"page_path"`
	PreOpTags    []string `json:"pre_op_tags"`
	BodyAffected bool     `json:"body_affected"`
}

// journalRecord is the invertible record stored per tag-admin op: the op type,
// the op-specific detail (old/new names, add/remove sets, etc.), and the
// per-page pre-op state. It is marshalled to JSON columns in tag_op_journal.
type journalRecord struct {
	OpType   string        `json:"op_type"`
	OpDetail interface{}   `json:"op_detail"`
	Pages    []journalPage `json:"pages"`
}

// journalOp inserts one append-only row into tag_op_journal recording the op
// just applied, built from the per-page preview (CurrentTags = pre-op tags,
// BodyAffected = body channel flag). Non-fatal: a DB error is logged but never
// breaks the request (the op already committed; the journal is best-effort so
// a transient SQLite hiccup can't sink a successful apply).
func (h *authHandlers) journalOp(ctx context.Context, opType string, opDetail interface{}, preview []tagPreviewPage) {
	if h.store == nil {
		return
	}
	pages := make([]journalPage, len(preview))
	for i, p := range preview {
		pages[i] = journalPage{PagePath: p.PagePath, PreOpTags: p.CurrentTags, BodyAffected: p.BodyAffected}
	}
	detailJSON, err := json.Marshal(opDetail)
	if err != nil {
		return
	}
	pagesJSON, err := json.Marshal(pages)
	if err != nil {
		return
	}
	if _, err := h.store.DB().ExecContext(ctx,
		`INSERT INTO tag_op_journal (op_type, op_detail, pages) VALUES (?, ?, ?)`,
		opType, string(detailJSON), string(pagesJSON)); err != nil {
		// Non-fatal: the op already committed; the journal is best-effort.
		slog.WarnContext(ctx, "tag_op_journal insert failed",
			slog.String("op_type", opType), slog.Any("error", err))
	}
}

// handleTagUndo handles POST /admin/tags/undo (TF-05). It reads the last
// tag_op_journal row, builds the inverse TagApplyItems (swap bodyOld/bodyNew
// for rename/merge; frontmatter-only for delete/bulk — delete body #tags are
// NOT invertible, a stated limitation), replays them through ApplyTagsBatch
// (one forward commit, per-page 409-guarded), audits the undo, and journals
// the undo op itself so undo-of-undo (redo) works for free.
func (h *authHandlers) handleTagUndo(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "Tag undo unavailable.")
		return
	}
	actor := h.actorFromSession(r)
	ctx := r.Context()

	// Read the LAST journal row (last-op undo, v1.6).
	var id int64
	var opType, opDetailJSON, pagesJSON string
	err := h.store.DB().QueryRowContext(ctx,
		`SELECT id, op_type, op_detail, pages FROM tag_op_journal ORDER BY id DESC LIMIT 1`).
		Scan(&id, &opType, &opDetailJSON, &pagesJSON)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusBadRequest, "No tag operation to undo.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read undo journal.")
		return
	}

	var jpages []journalPage
	if err := json.Unmarshal([]byte(pagesJSON), &jpages); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to parse undo journal.")
		return
	}
	if len(jpages) == 0 {
		writeError(w, http.StatusBadRequest, "No tag operation to undo.")
		return
	}

	// Build the inverse TagApplyItems. BaseRevision is read FRESH per page at
	// undo time (NOT a stored one — the journal carries pre_op_tags, not
	// revisions). The 409-guard in ApplyTagsBatch protects against a race between
	// this read and the commit. A SEMANTIC stale check ALSO runs here: if the
	// page's current frontmatter tags no longer match what the op would have
	// produced from pre_op_tags, the page was edited since the op → report stale
	// and skip (never clobber a concurrent edit).
	items := make([]pages.TagApplyItem, 0, len(jpages))
	results := make([]pages.TagApplyResult, 0, len(jpages))
	preUndoTags := make([][]string, 0, len(jpages)) // current tags BEFORE undo (for the undo journal row)
	for _, jp := range jpages {
		pg, err := h.pages.Get(ctx, jp.PagePath)
		if err != nil {
			// A page gone since the op can't be restored — report notfound.
			results = append(results, pages.TagApplyResult{PagePath: jp.PagePath, Status: pages.TagApplyNotFound})
			continue
		}
		var bodyOld, bodyNew string
		var expectedPostOp func([]string) ([]string, bool)
		switch opType {
		case "rename":
			var d struct {
				OldName string `json:"old_name"`
				NewName string `json:"new_name"`
			}
			_ = json.Unmarshal([]byte(opDetailJSON), &d)
			bodyOld, bodyNew = d.NewName, d.OldName // swap
			oa, na := d.OldName, d.NewName
			expectedPostOp = func(c []string) ([]string, bool) { return computeRenameTags(c, oa, na) }
		case "merge":
			var d struct {
				Source string `json:"source"`
				Target string `json:"target"`
			}
			_ = json.Unmarshal([]byte(opDetailJSON), &d)
			bodyOld, bodyNew = d.Target, d.Source // swap
			src, tgt := d.Source, d.Target
			expectedPostOp = func(c []string) ([]string, bool) { return computeMergeTags(c, src, tgt) }
		case "delete":
			var d struct {
				Tag string `json:"tag"`
			}
			_ = json.Unmarshal([]byte(opDetailJSON), &d)
			tg := d.Tag
			expectedPostOp = func(c []string) ([]string, bool) { return computeDeleteTags(c, tg) }
		case "bulk":
			var d struct {
				AddTags    []string `json:"add_tags"`
				RemoveTags []string `json:"remove_tags"`
			}
			_ = json.Unmarshal([]byte(opDetailJSON), &d)
			expectedPostOp = computeBulkTags(d.AddTags, d.RemoveTags)
		case "undo":
			// Undo-of-undo (redo): the undo row's op_detail carries the ORIGINAL
			// op's type + detail. The redo reapplies the ORIGINAL op's body
			// direction (NOT a swap-of-swap — the undo already swapped once;
			// redo = original direction again). The semantic stale check is
			// skipped for undo rows (the 409-guard still applies; the fresh read
			// is authoritative).
			var inner struct {
				OpType   string          `json:"op_type"`
				OpDetail  json.RawMessage `json:"op_detail"`
			}
			_ = json.Unmarshal([]byte(opDetailJSON), &inner)
			switch inner.OpType {
			case "rename":
				var d struct {
					OldName string `json:"old_name"`
					NewName string `json:"new_name"`
				}
				_ = json.Unmarshal(inner.OpDetail, &d)
				bodyOld, bodyNew = d.OldName, d.NewName // original direction
			case "merge":
				var d struct {
					Source string `json:"source"`
					Target string `json:"target"`
				}
				_ = json.Unmarshal(inner.OpDetail, &d)
				bodyOld, bodyNew = d.Source, d.Target // original direction
			}
		}

	// Read the page's current frontmatter tags (needed for the stale check AND
	// for the undo journal row's pre_op_tags — the state BEFORE the undo).
	src := pages.AssembleSource(pg.Frontmatter, pg.Body)
	currentTags, _ := readPageTags([]byte(src))
	if currentTags == nil {
		currentTags = []string{}
	}

	// Semantic stale check: compare the page's CURRENT frontmatter tags
	// against what the op would have produced from pre_op_tags. A mismatch
	// means the page was edited since the op → stale, skip (never clobber).
	if expectedPostOp != nil {
		expectedTags, _ := expectedPostOp(jp.PreOpTags)
		if !sameTagSet(currentTags, expectedTags) {
			results = append(results, pages.TagApplyResult{PagePath: jp.PagePath, Status: pages.TagApplyStale})
			continue
		}
	}

	tags := jp.PreOpTags
	if tags == nil {
		tags = []string{}
	}
	preUndoTags = append(preUndoTags, currentTags)
	items = append(items, pages.TagApplyItem{
		PagePath:        jp.PagePath,
		Tags:            tags,
		BaseRevision:    pg.Revision,
		SkipFrontmatter: false,
		BodyOld:         bodyOld,
		BodyNew:         bodyNew,
	})
	}

	if len(items) == 0 {
		// Every page was stale or notfound — no commit, return the outcomes.
		writeJSON(w, http.StatusOK, tagApplyResponse{Action: "undo", Results: results})
		return
	}

	applied, err := h.pages.ApplyTagsBatch(ctx, items, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to apply tag undo.")
		return
	}
	// Merge the pre-built stale/notfound results with the apply outcomes.
	results = append(results, applied...)

	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionTagUndo,
		Actor:  actor,
		Target: opType,
		Detail: fmt.Sprintf("undo %s (%d pages)", opType, len(items)),
		Source: auditSourceWeb,
	})

	// Journal the undo op itself so undo-of-undo (redo) works for free. The
	// pre_op_tags are the CURRENT tags (the state BEFORE the undo) — so a redo
	// restores the post-original-op state. body_affected mirrors the original
	// op's body channel. The op_detail carries the ORIGINAL op's type + detail
	// so a redo can swap the body back the same way the original applied it.
	undoPreview := make([]tagPreviewPage, len(items))
	for i, it := range items {
		undoPreview[i] = tagPreviewPage{
			PagePath:     it.PagePath,
			CurrentTags:  preUndoTags[i],
			BodyAffected: it.BodyOld != "",
		}
	}
	h.journalOp(ctx, "undo", map[string]interface{}{"undid": opType, "op_type": opType, "op_detail": json.RawMessage(opDetailJSON)}, undoPreview)

	resp := tagApplyResponse{Action: "undo", Results: results}
	if opType == "delete" {
		// Surface the delete body-#tag limitation so the admin knows body #tags
		// removed by the original delete are NOT re-inserted by the undo.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"action":  resp.Action,
			"results": resp.Results,
			"note":    "Undo restores frontmatter tags only; body #tags removed by the original delete are not re-inserted (they self-heal on the next edit).",
		})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

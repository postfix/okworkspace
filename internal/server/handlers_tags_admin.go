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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/pages"
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
	writeJSON(w, http.StatusOK, tagApplyResponse{Action: "delete", Results: results})
}

// handleTagList handles GET /admin/tags (TM-04). Returns the workspace tag
// vocabulary with per-tag page counts from the derived graph cache.
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
	type tagEntry struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	out := make([]tagEntry, 0, len(counts))
	for tag, c := range counts {
		out = append(out, tagEntry{Tag: tag, Count: c})
	}
	writeJSON(w, http.StatusOK, out)
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

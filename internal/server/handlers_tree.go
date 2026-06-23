package server

import (
	"context"
	"net/http"

	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/pages"
)

// handleTree returns the nested navigation tree (SPEC §17.2). Available to any
// authenticated user (readers included) — reading the tree is not gated by the
// editor role. Each page node is enriched with its uploaded attachments as leaf
// children so they show in the file panel alongside pages (Obsidian-style).
func (h *authHandlers) handleTree(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	nodes, err := h.pages.Tree(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your pages — try again.")
		return
	}
	if h.attachments != nil {
		enrichTreeAttachments(r.Context(), h.attachments, nodes)
	}
	writeJSON(w, http.StatusOK, nodes)
}

// enrichTreeAttachments walks the nested tree and hangs each page's attachments
// off it as leaf nodes (type "attachment", Path = attachment id, Title = original
// filename). A per-page list error is tolerated (that page just shows no
// attachments) so a single bad read never breaks navigation.
func enrichTreeAttachments(ctx context.Context, as *attachments.Service, nodes []pages.Node) {
	for i := range nodes {
		switch nodes[i].Type {
		case "folder":
			enrichTreeAttachments(ctx, as, nodes[i].Children)
		case "page":
			items, err := as.List(ctx, nodes[i].Path)
			if err != nil {
				continue
			}
			for _, it := range items {
				nodes[i].Attachments = append(nodes[i].Attachments, pages.Node{
					Type:  "attachment",
					Path:  it.ID,
					Title: it.OriginalName,
				})
			}
		}
	}
}

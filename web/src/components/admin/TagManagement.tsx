// TagManagement (TM-04 / v1.3) — workspace-wide tag management surface.
// Renders the tag table with rename/merge/delete + bulk multi-tag dialogs,
// embedded in the Admin panel. Every destructive operation follows the
// preview→approve contract. Phase 34 / TF-04 adds an Analytics tab with
// per-tag usage counts, most/least used, orphans, and totals.
import { useState, useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, GitMerge, Trash2, AlertTriangle, Loader2, Layers, Palette, BarChart3 } from "lucide-react";
import {
  listTags,
  renameTag,
  mergeTag,
  deleteTag,
  bulkTag,
  setTagMeta,
  getTree,
  getTagAnalytics,
  type TagEntry,
  type TagPreview,
  type TreeNode,
  type TagAnalyticsSummary,
} from "../../api/client";
import Dialog from "../../components/Dialog";
import "./TagManagement.css";

const TAGS_KEY = ["admin", "tags"];
const ANALYTICS_KEY = ["admin", "tags", "analytics"];

type Tab = "tags" | "analytics";

export default function TagManagement() {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("tags");
  const { data: tags = [], isLoading, isError } = useQuery<TagEntry[]>({
    queryKey: TAGS_KEY,
    queryFn: listTags,
  });

  return (
    <div className="tagmgmt">
      <div className="tagmgmt-tabs" role="tablist">
        <button
          className={`btn btn-sm tagmgmt-tab ${tab === "tags" ? "tagmgmt-tab-active" : ""}`}
          role="tab"
          aria-selected={tab === "tags"}
          onClick={() => setTab("tags")}
        >
          Tags
        </button>
        <button
          className={`btn btn-sm tagmgmt-tab ${tab === "analytics" ? "tagmgmt-tab-active" : ""}`}
          role="tab"
          aria-selected={tab === "analytics"}
          onClick={() => setTab("analytics")}
        >
          <BarChart3 size={14} /> Analytics
        </button>
      </div>

      {tab === "tags" && (
        <>
          {isLoading && <p className="tagmgmt-status"><Loader2 size={14} className="spinner" /> Loading tags…</p>}
          {isError && <p className="tagmgmt-status tagmgmt-error">Couldn't load tags.</p>}
          {!isLoading && !isError && tags.length === 0 && (
            <p className="tagmgmt-status">No tags yet. Tags appear when pages carry them.</p>
          )}
          {!isLoading && !isError && tags.length > 0 && (
            <TagTable tags={tags} onDone={() => queryClient.invalidateQueries({ queryKey: TAGS_KEY })} />
          )}
        </>
      )}

      {tab === "analytics" && (
        <AnalyticsPanel />
      )}
    </div>
  );
}

function TagTable({ tags, onDone }: { tags: TagEntry[]; onDone: () => void }) {
  const [renameTarget, setRenameTarget] = useState<TagEntry | null>(null);
  const [mergeSource, setMergeSource] = useState<TagEntry | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<TagEntry | null>(null);
  const [metaTarget, setMetaTarget] = useState<TagEntry | null>(null);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [query, setQuery] = useState("");

  const filtered = query.trim()
    ? tags.filter((t) => t.tag.includes(query.trim().toLowerCase()))
    : tags;

  // Compute the grouped view: roots (no parent or orphaned parent) with their
  // children indented beneath. A tag whose parent doesn't exist in the list is
  // treated as a root (orphaned, per D-02b). Tags are sorted alphabetically;
  // children under each root are also sorted alphabetically.
  const tagSet = new Set(filtered.map((t) => t.tag));
  const roots = filtered.filter((t) => !t.parent || !tagSet.has(t.parent));
  const childrenByParent = new Map<string, TagEntry[]>();
  for (const t of filtered) {
    if (t.parent && tagSet.has(t.parent)) {
      const arr = childrenByParent.get(t.parent) ?? [];
      arr.push(t);
      childrenByParent.set(t.parent, arr);
    }
  }
  // Sort roots and children alphabetically by tag.
  roots.sort((a, b) => a.tag.localeCompare(b.tag));
  for (const arr of childrenByParent.values()) {
    arr.sort((a, b) => a.tag.localeCompare(b.tag));
  }

  // Build the flat render order: root, then its children (indented), then next root.
  type renderRow = { tag: TagEntry; depth: number };
  const rows: renderRow[] = [];
  for (const root of roots) {
    rows.push({ tag: root, depth: 0 });
    const kids = childrenByParent.get(root.tag) ?? [];
    for (const k of kids) {
      rows.push({ tag: k, depth: 1 });
    }
  }

  return (
    <>
      <input
        type="text"
        className="input tagmgmt-search"
        placeholder="Filter tags…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        aria-label="Filter tags"
      />
      <button className="btn btn-sm tagmgmt-bulk-btn" onClick={() => setBulkOpen(true)} title="Bulk add/remove tags across pages">
        <Layers size={14} /> Bulk add/remove
      </button>
      <table className="tagmgmt-table">
        <thead>
          <tr>
            <th>Tag</th>
            <th className="tagmgmt-count-col">Pages</th>
            <th className="tagmgmt-actions-col">Actions</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr><td className="tagmgmt-empty-row" colSpan={3}>No tags match “{query}”.</td></tr>
          ) : (
            rows.map(({ tag: t, depth }) => (
              <tr key={t.tag} className={depth > 0 ? "tagmgmt-child-row" : ""}>
                <td style={{ paddingLeft: depth > 0 ? "24px" : undefined }}>
                  {t.color && <span className="tagmgmt-color-dot" style={{ backgroundColor: t.color }} title={t.description || undefined} />}
                  {depth > 0 && <span className="tagmgmt-tree-marker">↳</span>}
                  <code>{t.tag}</code>
                  {depth > 0 && t.parent && <span className="tagmgmt-parent-note"> (parent: {t.parent})</span>}
                </td>
                <td className="tagmgmt-count-col">{t.count}</td>
                <td className="tagmgmt-actions-col">
                  <button className="btn btn-sm btn-ghost" onClick={() => setMetaTarget(t)} title="Edit metadata"><Palette size={14} /></button>
                  <button className="btn btn-sm btn-ghost" onClick={() => setRenameTarget(t)} title="Rename"><Pencil size={14} /></button>
                  <button className="btn btn-sm btn-ghost" onClick={() => setMergeSource(t)} title="Merge"><GitMerge size={14} /></button>
                  <button className="btn btn-sm btn-ghost" onClick={() => setDeleteTarget(t)} title="Delete"><Trash2 size={14} /></button>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      {renameTarget && (
        <RenameDialog tag={renameTarget} onClose={() => setRenameTarget(null)} onDone={onDone} />
      )}
      {mergeSource && (
        <MergeDialog source={mergeSource} allTags={tags} onClose={() => setMergeSource(null)} onDone={onDone} />
      )}
      {deleteTarget && (
        <DeleteDialog tag={deleteTarget} onClose={() => setDeleteTarget(null)} onDone={onDone} />
      )}
      {metaTarget && (
        <TagMetaDialog tag={metaTarget} allTags={tags} onClose={() => setMetaTarget(null)} onDone={onDone} />
      )}
      {bulkOpen && (
        <BulkTagDialog allTags={tags} onClose={() => setBulkOpen(false)} onDone={onDone} />
      )}
    </>
  );
}

// AnalyticsPanel (Phase 34 / TF-04) — read-only admin analytics view over
// graph.TagPageCounts. Shows summary cards (total tags, tagged/untagged
// pages), a per-tag counts table (client-side sortable by count or name),
// most-used + least-used sections (top 5 each), and orphan tags (metadata-only
// tags with zero pages). A scope note states that counts reflect frontmatter
// page_tags only — body-only #tags are counted after materialization.
function AnalyticsPanel() {
  const [sortKey, setSortKey] = useState<"count" | "name">("count");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const { data, isLoading, isError } = useQuery<TagAnalyticsSummary>({
    queryKey: ANALYTICS_KEY,
    queryFn: getTagAnalytics,
  });

  if (isLoading) {
    return <p className="tagmgmt-status"><Loader2 size={14} className="spinner" /> Loading analytics…</p>;
  }
  if (isError) {
    return <p className="tagmgmt-status tagmgmt-error">Couldn't load tag analytics.</p>;
  }
  if (!data) {
    return <p className="tagmgmt-status">No analytics data.</p>;
  }

  const sortedTags = [...data.tags].sort((a, b) => {
    let cmp: number;
    if (sortKey === "count") {
      cmp = a.count - b.count;
    } else {
      cmp = a.tag.localeCompare(b.tag);
    }
    return sortDir === "desc" ? -cmp : cmp;
  });

  const toggleSort = (key: "count" | "name") => {
    if (sortKey === key) {
      setSortDir(sortDir === "desc" ? "asc" : "desc");
    } else {
      setSortKey(key);
      setSortDir(key === "count" ? "desc" : "asc");
    }
  };

  return (
    <div className="tagmgmt-analytics">
      <div className="tagmgmt-analytics-cards">
        <div className="tagmgmt-analytics-card">
          <span className="tagmgmt-analytics-card-value">{data.total_tags}</span>
          <span className="tagmgmt-analytics-card-label">Total tags</span>
        </div>
        <div className="tagmgmt-analytics-card">
          <span className="tagmgmt-analytics-card-value">{data.tagged_pages}</span>
          <span className="tagmgmt-analytics-card-label">Tagged pages</span>
        </div>
        <div className="tagmgmt-analytics-card">
          <span className="tagmgmt-analytics-card-value">{data.untagged_pages}</span>
          <span className="tagmgmt-analytics-card-label">Untagged pages</span>
        </div>
        <div className="tagmgmt-analytics-card">
          <span className="tagmgmt-analytics-card-value">{data.total_pages}</span>
          <span className="tagmgmt-analytics-card-label">Total pages</span>
        </div>
      </div>

      <div className="tagmgmt-analytics-sections">
        <div className="tagmgmt-analytics-section">
          <h4 className="tagmgmt-analytics-heading">Most used</h4>
          <ul className="tagmgmt-analytics-list">
            {data.most_used.length === 0 ? (
              <li className="tagmgmt-analytics-empty">No tags yet.</li>
            ) : (
              data.most_used.map((t) => (
                <li key={t.tag}><code>{t.tag}</code> <span className="tagmgmt-analytics-count">{t.count}</span></li>
              ))
            )}
          </ul>
        </div>
        <div className="tagmgmt-analytics-section">
          <h4 className="tagmgmt-analytics-heading">Least used</h4>
          <ul className="tagmgmt-analytics-list">
            {data.least_used.length === 0 ? (
              <li className="tagmgmt-analytics-empty">No tags yet.</li>
            ) : (
              data.least_used.map((t) => (
                <li key={t.tag}><code>{t.tag}</code> <span className="tagmgmt-analytics-count">{t.count}</span></li>
              ))
            )}
          </ul>
        </div>
        <div className="tagmgmt-analytics-section">
          <h4 className="tagmgmt-analytics-heading">Orphan tags (0 pages)</h4>
          <ul className="tagmgmt-analytics-list">
            {data.orphans.length === 0 ? (
              <li className="tagmgmt-analytics-empty">No orphan tags.</li>
            ) : (
              data.orphans.map((t) => (
                <li key={t.tag}><code>{t.tag}</code> <span className="tagmgmt-analytics-count">{t.count}</span></li>
              ))
            )}
          </ul>
        </div>
      </div>

      <table className="tagmgmt-table tagmgmt-analytics-table">
        <thead>
          <tr>
            <th className="tagmgmt-analytics-sort" onClick={() => toggleSort("name")}>
              Tag {sortKey === "name" && (sortDir === "desc" ? "▼" : "▲")}
            </th>
            <th className="tagmgmt-count-col tagmgmt-analytics-sort" onClick={() => toggleSort("count")}>
              Pages {sortKey === "count" && (sortDir === "desc" ? "▼" : "▲")}
            </th>
          </tr>
        </thead>
        <tbody>
          {sortedTags.length === 0 ? (
            <tr><td className="tagmgmt-empty-row" colSpan={2}>No tags.</td></tr>
          ) : (
            sortedTags.map((t) => (
              <tr key={t.tag}>
                <td><code>{t.tag}</code></td>
                <td className="tagmgmt-count-col">{t.count}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      <p className="tagmgmt-analytics-scope">
        Counts reflect frontmatter tags (the page_tags cache). Body-only #tags
        are counted only after materialization.
      </p>
    </div>
  );
}

function RenameDialog({ tag, onClose, onDone }: { tag: TagEntry; onClose: () => void; onDone: () => void }) {
  const [newName, setNewName] = useState("");
  const [preview, setPreview] = useState<TagPreview | null>(null);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");

  const handlePreview = async () => {
    if (!newName.trim()) return;
    setError("");
    try {
      const result = await renameTag({ old_name: tag.tag, new_name: newName.trim(), preview: true });
      if ("affected_pages" in result) setPreview(result);
    } catch {
      setError("Failed to preview rename.");
    }
  };

  const handleApply = async () => {
    setApplying(true);
    setError("");
    try {
      await renameTag({ old_name: tag.tag, new_name: newName.trim() });
      onDone();
      onClose();
    } catch {
      setError("Failed to apply rename.");
    } finally {
      setApplying(false);
    }
  };

  return (
    <Dialog open title={`Rename tag "${tag.tag}"`} confirmLabel={preview ? "Apply rename" : "Preview"} cancelLabel="Cancel"
      busy={applying}
      onCancel={onClose}
      onConfirm={preview ? handleApply : handlePreview}
    >
      <div className="tagmgmt-dialog-body">
        <label className="tagmgmt-field">
          New name
          <input type="text" className="input" value={newName} onChange={(e) => setNewName(e.target.value)}
            placeholder="new-tag-name" autoFocus />
        </label>
        {preview && (
          <div className="tagmgmt-preview">
            <p>{preview.total_affected} page(s) affected:</p>
            {preview.affected_pages.some((p) => p.body_affected) && (
              <p className="tagmgmt-body-note">
                {preview.affected_pages.filter((p) => p.body_affected).length} page(s) also have body #tags that will be rewritten.
              </p>
            )}
            <ul className="tagmgmt-preview-list">
              {preview.affected_pages.slice(0, 20).map((p) => (
                <li key={p.page_path}>
                  <code>{p.page_path}</code>
                  <span className="tagmgmt-tag-change">{p.current_tags.join(", ")} → {p.new_tags.join(", ")}</span>
                  {p.body_affected && <span className="tagmgmt-body-badge">prose</span>}
                </li>
              ))}
              {preview.total_affected > 20 && <li>…and {preview.total_affected - 20} more</li>}
            </ul>
          </div>
        )}
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}

function MergeDialog({ source, allTags, onClose, onDone }: { source: TagEntry; allTags: TagEntry[]; onClose: () => void; onDone: () => void }) {
  const [target, setTarget] = useState("");
  const [preview, setPreview] = useState<TagPreview | null>(null);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");

  const handlePreview = async () => {
    if (!target) return;
    setError("");
    try {
      const result = await mergeTag({ source: source.tag, target, preview: true });
      if ("affected_pages" in result) setPreview(result);
    } catch {
      setError("Failed to preview merge.");
    }
  };

  const handleApply = async () => {
    setApplying(true);
    setError("");
    try {
      await mergeTag({ source: source.tag, target });
      onDone();
      onClose();
    } catch {
      setError("Failed to apply merge.");
    } finally {
      setApplying(false);
    }
  };

  return (
    <Dialog open title={`Merge "${source.tag}" into…`} confirmLabel={preview ? "Apply merge" : "Preview"} cancelLabel="Cancel"
      busy={applying}
      onCancel={onClose}
      onConfirm={preview ? handleApply : handlePreview}
    >
      <div className="tagmgmt-dialog-body">
        <label className="tagmgmt-field">
          Target tag
          <select className="input" value={target} onChange={(e) => setTarget(e.target.value)} autoFocus>
            <option value="">Select a tag…</option>
            {allTags.filter((t) => t.tag !== source.tag).map((t) => (
              <option key={t.tag} value={t.tag}>{t.tag} ({t.count} pages)</option>
            ))}
          </select>
        </label>
        {preview && (
          <div className="tagmgmt-preview">
            <p>{preview.total_affected} page(s) affected:</p>
            {preview.affected_pages.some((p) => p.body_affected) && (
              <p className="tagmgmt-body-note">
                {preview.affected_pages.filter((p) => p.body_affected).length} page(s) also have body #tags that will be rewritten.
              </p>
            )}
            <ul className="tagmgmt-preview-list">
              {preview.affected_pages.slice(0, 20).map((p) => (
                <li key={p.page_path}>
                  <code>{p.page_path}</code>
                  <span className="tagmgmt-tag-change">{p.current_tags.join(", ")} → {p.new_tags.join(", ")}</span>
                  {p.body_affected && <span className="tagmgmt-body-badge">prose</span>}
                </li>
              ))}
              {preview.total_affected > 20 && <li>…and {preview.total_affected - 20} more</li>}
            </ul>
          </div>
        )}
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}

function DeleteDialog({ tag, onClose, onDone }: { tag: TagEntry; onClose: () => void; onDone: () => void }) {
  const [preview, setPreview] = useState<TagPreview | null>(null);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");

  const handlePreview = async () => {
    setError("");
    try {
      const result = await deleteTag({ tag: tag.tag, preview: true });
      if ("affected_pages" in result) setPreview(result);
    } catch {
      setError("Failed to preview deletion.");
    }
  };

  const handleApply = async () => {
    setApplying(true);
    setError("");
    try {
      await deleteTag({ tag: tag.tag });
      onDone();
      onClose();
    } catch {
      setError("Failed to delete tag.");
    } finally {
      setApplying(false);
    }
  };

  // Auto-preview on open — runs once when the dialog mounts.
  useEffect(() => {
    handlePreview();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <Dialog open title={`Delete tag "${tag.tag}"`} confirmLabel="Delete" cancelLabel="Cancel"
      destructive busy={applying}
      onCancel={onClose}
      onConfirm={preview ? handleApply : undefined}
    >
      <div className="tagmgmt-dialog-body">
        {preview ? (
          <>
            <p>Remove <strong>{tag.tag}</strong> from all <strong>{preview.total_affected}</strong> page(s). This cannot be undone.</p>
            {preview.affected_pages.some((p) => p.body_affected) && (
              <p className="tagmgmt-body-note">
                {preview.affected_pages.filter((p) => p.body_affected).length} page(s) also have body #tags that will be rewritten.
              </p>
            )}
          </>
        ) : (
          <p><Loader2 size={14} className="spinner" /> Checking affected pages…</p>
        )}
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}

// flattenTree walks the nested TreeNode[] and returns all type==="page" paths.
function flattenTree(nodes: TreeNode[]): string[] {
  const out: string[] = [];
  const walk = (ns: TreeNode[]) => {
    for (const n of ns) {
      if (n.type === "page") out.push(n.path);
      if (n.children) walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

// parseTagInput splits a comma-separated string into trimmed non-empty tags.
function parseTagInput(s: string): string[] {
  return s
    .split(",")
    .map((t) => t.trim())
    .filter((t) => t.length > 0);
}

// BulkTagDialog (Phase 31 / TF-01) — add and/or remove multiple tags across a
// selected set of pages (explicit checkboxes ∪ filter-tags) in one
// preview→approve batch. Mirrors the RenameDialog/MergeDialog/DeleteDialog
// preview-table + approve-button pattern.
function BulkTagDialog({ allTags, onClose, onDone }: { allTags: TagEntry[]; onClose: () => void; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [addTagsStr, setAddTagsStr] = useState("");
  const [removeTagsStr, setRemoveTagsStr] = useState("");
  const [filterTagsStr, setFilterTagsStr] = useState("");
  const [selectedPages, setSelectedPages] = useState<Set<string>>(new Set());
  const [preview, setPreview] = useState<TagPreview | null>(null);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");

  const { data: tree = [] } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
  });
  const pagePaths = flattenTree(tree);
  const vocab = allTags.map((t) => t.tag);

  const togglePage = (path: string) => {
    setSelectedPages((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const buildOpts = (previewFlag: boolean) => ({
    page_paths: Array.from(selectedPages),
    filter_tags: parseTagInput(filterTagsStr),
    add_tags: parseTagInput(addTagsStr),
    remove_tags: parseTagInput(removeTagsStr),
    preview: previewFlag,
  });

  const handlePreview = async () => {
    setError("");
    const opts = buildOpts(true);
    if (opts.page_paths.length === 0 && opts.filter_tags.length === 0) {
      setError("Select pages or enter filter tags.");
      return;
    }
    if (opts.add_tags.length === 0 && opts.remove_tags.length === 0) {
      setError("Enter tags to add or remove.");
      return;
    }
    try {
      const result = await bulkTag(opts);
      if ("affected_pages" in result) setPreview(result);
    } catch {
      setError("Failed to preview bulk tag change.");
    }
  };

  const handleApply = async () => {
    setApplying(true);
    setError("");
    try {
      await bulkTag(buildOpts(false));
      queryClient.invalidateQueries({ queryKey: TAGS_KEY });
      onDone();
      onClose();
    } catch {
      setError("Failed to apply bulk tag change.");
    } finally {
      setApplying(false);
    }
  };

  return (
    <Dialog open title="Bulk add/remove tags" confirmLabel={preview ? "Apply" : "Preview"} cancelLabel="Cancel"
      busy={applying}
      onCancel={onClose}
      onConfirm={preview ? handleApply : handlePreview}
    >
      <div className="tagmgmt-dialog-body tagmgmt-bulk-body">
        <fieldset className="tagmgmt-bulk-section">
          <legend>Filter by tag (union)</legend>
          <input
            type="text"
            className="input"
            value={filterTagsStr}
            onChange={(e) => setFilterTagsStr(e.target.value)}
            placeholder={vocab.length > 0 ? vocab.slice(0, 4).join(", ") + ", …" : "tag1, tag2"}
            aria-label="Filter by tag (comma-separated)"
          />
          <p className="tagmgmt-bulk-hint">Pages carrying any of these tags are included.</p>
        </fieldset>

        <fieldset className="tagmgmt-bulk-section">
          <legend>Or select pages</legend>
          <div className="tagmgmt-bulk-pagelist">
            {pagePaths.length === 0 ? (
              <p className="tagmgmt-bulk-hint">No pages.</p>
            ) : (
              pagePaths.map((p) => (
                <label key={p} className="tagmgmt-bulk-pagecheck">
                  <input
                    type="checkbox"
                    checked={selectedPages.has(p)}
                    onChange={() => togglePage(p)}
                  />
                  <code>{p}</code>
                </label>
              ))
            )}
          </div>
        </fieldset>

        <div className="tagmgmt-bulk-ops">
          <label className="tagmgmt-field">
            Tags to add
            <input
              type="text"
              className="input"
              value={addTagsStr}
              onChange={(e) => setAddTagsStr(e.target.value)}
              placeholder="new-tag, another-tag"
              aria-label="Tags to add (comma-separated)"
            />
          </label>
          <label className="tagmgmt-field">
            Tags to remove
            <input
              type="text"
              className="input"
              value={removeTagsStr}
              onChange={(e) => setRemoveTagsStr(e.target.value)}
              placeholder="old-tag, stale-tag"
              aria-label="Tags to remove (comma-separated)"
            />
          </label>
        </div>

        {preview && (
          <div className="tagmgmt-preview">
            <p>{preview.total_affected} page(s) affected:</p>
            <ul className="tagmgmt-preview-list">
              {preview.affected_pages.slice(0, 30).map((p) => (
                <li key={p.page_path}>
                  <code>{p.page_path}</code>
                  <span className="tagmgmt-tag-change">{p.current_tags.join(", ")} → {p.new_tags.join(", ")}</span>
                </li>
              ))}
              {preview.total_affected > 30 && <li>…and {preview.total_affected - 30} more</li>}
            </ul>
          </div>
        )}
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}


// TagMetaDialog (Phase 32 / TF-02, Phase 33 / TF-03) — set or clear a tag's
// color + description + parent. The color is a hex value via <input type="color">
// or a palette token via <select>; the description is a single-line text input
// (max 200 chars); the parent is a <select> of the tag vocabulary (excluding the
// tag itself to prevent self-parenting in the UI — the server cycle check is the
// authoritative gate). Save calls setTagMeta; clearing (empty color + description +
// parent) deletes the metadata.
function TagMetaDialog({ tag, allTags, onClose, onDone }: { tag: TagEntry; allTags: TagEntry[]; onClose: () => void; onDone: () => void }) {
  const [color, setColor] = useState(tag.color || "");
  const [description, setDescription] = useState(tag.description || "");
  const [parent, setParent] = useState(tag.parent || "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const handleSave = async () => {
    setSaving(true);
    setError("");
    try {
      await setTagMeta({ tag: tag.tag, color, description, parent });
      onDone();
      onClose();
    } catch {
      setError("Failed to save tag metadata.");
    } finally {
      setSaving(false);
    }
  };

  const palette = ["", "red", "blue", "green", "amber", "purple", "gray"];
  const parentOptions = allTags
    .map((t) => t.tag)
    .filter((t) => t !== tag.tag)
    .sort((a, b) => a.localeCompare(b));

  return (
    <Dialog open title={`Tag metadata "${tag.tag}"`} confirmLabel="Save" cancelLabel="Cancel"
      busy={saving}
      onCancel={onClose}
      onConfirm={handleSave}
    >
      <div className="tagmgmt-dialog-body">
        <label className="tagmgmt-field">
          Color
          <div className="tagmgmt-color-row">
            <input
              type="color"
              className="tagmgmt-color-picker"
              value={color.startsWith("#") ? color : "#000000"}
              onChange={(e) => setColor(e.target.value)}
            />
            <select
              className="input tagmgmt-color-palette"
              value={color.startsWith("#") ? "" : color}
              onChange={(e) => setColor(e.target.value)}
            >
              {palette.map((p) => (
                <option key={p} value={p}>{p || "— palette —"}</option>
              ))}
            </select>
            <button className="btn btn-sm btn-ghost" type="button" onClick={() => setColor("")} title="Clear color">Clear</button>
          </div>
        </label>
        <label className="tagmgmt-field">
          Description
          <input
            type="text"
            className="input"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={200}
            placeholder="Short description (optional)"
          />
        </label>
        <label className="tagmgmt-field">
          Parent
          <select
            className="input"
            value={parent}
            onChange={(e) => setParent(e.target.value)}
          >
            <option value="">— root (no parent) —</option>
            {parentOptions.map((p) => (
              <option key={p} value={p}>{p}</option>
            ))}
          </select>
        </label>
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}
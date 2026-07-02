// TagManagement (TM-04 / v1.3) — workspace-wide tag management surface.
// Renders the tag table with rename/merge/delete + bulk multi-tag dialogs,
// embedded in the Admin panel. Every destructive operation follows the
// preview→approve contract.
import { useState, useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, GitMerge, Trash2, AlertTriangle, Loader2, Layers, Palette } from "lucide-react";
import {
  listTags,
  renameTag,
  mergeTag,
  deleteTag,
  bulkTag,
  setTagMeta,
  getTree,
  type TagEntry,
  type TagPreview,
  type TreeNode,
} from "../../api/client";
import Dialog from "../../components/Dialog";
import "./TagManagement.css";

const TAGS_KEY = ["admin", "tags"];

export default function TagManagement() {
  const queryClient = useQueryClient();
  const { data: tags = [], isLoading, isError } = useQuery<TagEntry[]>({
    queryKey: TAGS_KEY,
    queryFn: listTags,
  });

  return (
    <div className="tagmgmt">
      {isLoading && <p className="tagmgmt-status"><Loader2 size={14} className="spinner" /> Loading tags…</p>}
      {isError && <p className="tagmgmt-status tagmgmt-error">Couldn't load tags.</p>}
      {!isLoading && !isError && tags.length === 0 && (
        <p className="tagmgmt-status">No tags yet. Tags appear when pages carry them.</p>
      )}
      {!isLoading && !isError && tags.length > 0 && (
        <TagTable tags={tags} onDone={() => queryClient.invalidateQueries({ queryKey: TAGS_KEY })} />
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
          {filtered.length === 0 ? (
            <tr><td className="tagmgmt-empty-row" colSpan={3}>No tags match “{query}”.</td></tr>
          ) : (
            filtered.map((t) => (
              <tr key={t.tag}>
                <td>
                  {t.color && <span className="tagmgmt-color-dot" style={{ backgroundColor: t.color }} title={t.description || undefined} />}
                  <code>{t.tag}</code>
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
        <TagMetaDialog tag={metaTarget} onClose={() => setMetaTarget(null)} onDone={onDone} />
      )}
      {bulkOpen && (
        <BulkTagDialog allTags={tags} onClose={() => setBulkOpen(false)} onDone={onDone} />
      )}
    </>
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


// TagMetaDialog (Phase 32 / TF-02) — set or clear a tag's color + description.
// The color is a hex value via <input type="color"> or a palette token via
// <select>; the description is a single-line text input (max 200 chars). Save
// calls setTagMeta; clearing (empty color + description) deletes the metadata.
function TagMetaDialog({ tag, onClose, onDone }: { tag: TagEntry; onClose: () => void; onDone: () => void }) {
  const [color, setColor] = useState(tag.color || "");
  const [description, setDescription] = useState(tag.description || "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const handleSave = async () => {
    setSaving(true);
    setError("");
    try {
      await setTagMeta({ tag: tag.tag, color, description });
      onDone();
      onClose();
    } catch {
      setError("Failed to save tag metadata.");
    } finally {
      setSaving(false);
    }
  };

  const palette = ["", "red", "blue", "green", "amber", "purple", "gray"];

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
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}
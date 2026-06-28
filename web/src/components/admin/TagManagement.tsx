// TagManagement (TM-04 / v1.3) — workspace-wide tag management surface.
// TagManageLink: a small inline link for the page editor (opens as dialog).
// TagManagement: the full tag table with rename/merge/delete dialogs.
// Every destructive operation follows the preview→approve contract.
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, GitMerge, Trash2, AlertTriangle, Loader2 } from "lucide-react";
import {
  listTags,
  renameTag,
  mergeTag,
  deleteTag,
  type TagEntry,
  type TagPreview,
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
    <section className="tagmgmt">
      <h2 className="tagmgmt-heading">Tag Management</h2>
      {isLoading && <p className="tagmgmt-status"><Loader2 size={14} className="spinner" /> Loading tags…</p>}
      {isError && <p className="tagmgmt-status tagmgmt-error">Couldn't load tags.</p>}
      {!isLoading && !isError && tags.length === 0 && (
        <p className="tagmgmt-status">No tags yet. Tags appear when pages carry them.</p>
      )}
      {!isLoading && !isError && tags.length > 0 && (
        <TagTable tags={tags} onDone={() => queryClient.invalidateQueries({ queryKey: TAGS_KEY })} />
      )}
    </section>
  );
}

function TagTable({ tags, onDone }: { tags: TagEntry[]; onDone: () => void }) {
  const [renameTarget, setRenameTarget] = useState<TagEntry | null>(null);
  const [mergeSource, setMergeSource] = useState<TagEntry | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<TagEntry | null>(null);

  return (
    <>
      <table className="tagmgmt-table">
        <thead>
          <tr>
            <th>Tag</th>
            <th className="tagmgmt-count-col">Pages</th>
            <th className="tagmgmt-actions-col">Actions</th>
          </tr>
        </thead>
        <tbody>
          {tags.map((t) => (
            <tr key={t.tag}>
              <td><code>{t.tag}</code></td>
              <td className="tagmgmt-count-col">{t.count}</td>
              <td className="tagmgmt-actions-col">
                <button className="btn btn-sm btn-ghost" onClick={() => setRenameTarget(t)} title="Rename"><Pencil size={14} /></button>
                <button className="btn btn-sm btn-ghost" onClick={() => setMergeSource(t)} title="Merge"><GitMerge size={14} /></button>
                <button className="btn btn-sm btn-ghost" onClick={() => setDeleteTarget(t)} title="Delete"><Trash2 size={14} /></button>
              </td>
            </tr>
          ))}
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
            <ul className="tagmgmt-preview-list">
              {preview.affected_pages.slice(0, 20).map((p) => (
                <li key={p.page_path}>
                  <code>{p.page_path}</code>
                  <span className="tagmgmt-tag-change">{p.current_tags.join(", ")} → {p.new_tags.join(", ")}</span>
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
            <ul className="tagmgmt-preview-list">
              {preview.affected_pages.slice(0, 20).map((p) => (
                <li key={p.page_path}>
                  <code>{p.page_path}</code>
                  <span className="tagmgmt-tag-change">{p.current_tags.join(", ")} → {p.new_tags.join(", ")}</span>
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

  // Auto-preview on open
  if (!preview && !error) {
    handlePreview();
  }

  return (
    <Dialog open title={`Delete tag "${tag.tag}"`} confirmLabel="Delete" cancelLabel="Cancel"
      destructive busy={applying}
      onCancel={onClose}
      onConfirm={preview ? handleApply : undefined}
    >
      <div className="tagmgmt-dialog-body">
        {preview ? (
          <p>Remove <strong>{tag.tag}</strong> from all <strong>{preview.total_affected}</strong> page(s). This cannot be undone.</p>
        ) : (
          <p><Loader2 size={14} className="spinner" /> Checking affected pages…</p>
        )}
        {error && <p className="tagmgmt-error"><AlertTriangle size={14} /> {error}</p>}
      </div>
    </Dialog>
  );
}

// TagManageLink is a small inline link that opens the full TagManagement as a
// dialog. Placed next to TagSuggest in the page editor so workspace-wide tag
// ops are reachable where users actually work with tags.
export function TagManageLink() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" className="tagmgmt-link" onClick={() => setOpen(true)}>
        Manage workspace tags
      </button>
      {open && (
        <Dialog open title="Manage workspace tags" confirmLabel="Close" cancelLabel=""
          onCancel={() => setOpen(false)}
          onConfirm={() => setOpen(false)}
        >
          <TagManagement />
        </Dialog>
      )}
    </>
  );
}

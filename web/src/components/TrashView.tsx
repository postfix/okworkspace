import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Undo2, Trash2 } from "lucide-react";

import {
  listTrash,
  restoreFromTrash,
  restoreFolderGroup,
  type TrashEntry,
} from "../api/client";
import "./TrashView.css";

// relativeTime renders an ISO-8601 timestamp as a friendly relative time
// ("just now", "2 hours ago", "yesterday", "3 days ago"). No Git vocabulary,
// no raw timestamps surfaced to the user.
export function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const secs = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (secs < 60) return "just now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return mins === 1 ? "1 minute ago" : `${mins} minutes ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours === 1 ? "1 hour ago" : `${hours} hours ago`;
  const days = Math.floor(hours / 24);
  if (days === 1) return "yesterday";
  if (days < 30) return `${days} days ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return months === 1 ? "1 month ago" : `${months} months ago`;
  const years = Math.floor(months / 12);
  return years === 1 ? "1 year ago" : `${years} years ago`;
}

// A SoloRow is a per-page trash entry (no delete_group_id); a GroupRow bundles
// every entry sharing one delete_group_id (one folder-delete, TREE-04/05) so the
// list renders it as ONE restorable unit.
type Row =
  | { kind: "solo"; entry: TrashEntry }
  | {
      kind: "group";
      groupId: string;
      folderName: string;
      count: number;
      // The newest deleted_at + deleter in the group drive the meta line; the
      // group was created in one action so they share a timestamp.
      deletedBy: string;
      deletedAt: string;
    };

// folderNameForGroup derives the folder's display name from the common ancestor
// of the group's original paths. The grouped delete always includes the folder's
// index.md, so the folder dir is the parent of the shallowest path.
function folderNameForGroup(entries: TrashEntry[]): string {
  // The shallowest original_path (fewest segments) belongs to the folder's
  // index.md; its directory is the folder. Fall back to the first entry's dir.
  let shallowest = entries[0]?.original_path ?? "";
  for (const e of entries) {
    if (e.original_path.split("/").length < shallowest.split("/").length) {
      shallowest = e.original_path;
    }
  }
  const dir = shallowest.includes("/")
    ? shallowest.slice(0, shallowest.lastIndexOf("/"))
    : shallowest;
  const name = dir.includes("/") ? dir.slice(dir.lastIndexOf("/") + 1) : dir;
  return name || "folder";
}

// buildRows folds the flat trash listing into grouped + solo rows, preserving the
// original (newest-first) order: a group takes the position of its first entry.
export function buildRows(entries: TrashEntry[]): Row[] {
  const rows: Row[] = [];
  const seenGroups = new Set<string>();
  const byGroup = new Map<string, TrashEntry[]>();
  for (const e of entries) {
    if (e.delete_group_id) {
      const list = byGroup.get(e.delete_group_id) ?? [];
      list.push(e);
      byGroup.set(e.delete_group_id, list);
    }
  }
  for (const e of entries) {
    if (e.delete_group_id) {
      if (seenGroups.has(e.delete_group_id)) continue;
      seenGroups.add(e.delete_group_id);
      const group = byGroup.get(e.delete_group_id) ?? [];
      rows.push({
        kind: "group",
        groupId: e.delete_group_id,
        folderName: folderNameForGroup(group),
        count: group.length,
        deletedBy: e.deleted_by,
        deletedAt: e.deleted_at,
      });
    } else {
      rows.push({ kind: "solo", entry: e });
    }
  }
  return rows;
}

// TrashView lists deleted pages and folders and lets an editor restore each one.
// Delete is a recycle bin: every entry shows its title (or folder name), who
// deleted it, and when (relative time) — never a path-as-identity or any Git
// vocabulary. Pages trashed together by one folder-delete (a shared
// delete_group_id) render as a single "Folder '{name}' · {N} pages" row with a
// Restore folder action; individually-trashed pages keep their per-page row.
// Restoring auto-suffixes on a name collision so a live page is never clobbered
// (D-10); when that happens an informational notice is shown.
export default function TrashView() {
  const queryClient = useQueryClient();
  const [notice, setNotice] = useState<string | null>(null);

  const { data, isLoading, isError } = useQuery<TrashEntry[]>({
    queryKey: ["trash"],
    queryFn: listTrash,
  });

  const restoreMut = useMutation({
    mutationFn: (entry: TrashEntry) => restoreFromTrash(entry.id),
    onSuccess: (res, entry) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["trash"] });
      // If the restored path differs from the original, a same-named page already
      // existed and the backend auto-suffixed (D-10) — tell the user calmly.
      if (res.path !== entry.original_path) {
        setNotice(
          `A page with that name already exists, so this one was restored as '${entry.title} (restored)'.`,
        );
      } else {
        setNotice(null);
      }
    },
    onError: () => {
      setNotice("We couldn't restore that page just now. Try again.");
    },
  });

  const restoreGroupMut = useMutation({
    mutationFn: (groupId: string) => restoreFolderGroup(groupId),
    onSuccess: (res, groupId) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["trash"] });
      // The grouped restore reports the actual restored paths; if any page was
      // auto-suffixed (its restored path was made unique) surface the batched
      // notice. We detect a suffix by the "(restored)"-style "-N" tail the server
      // adds, or — more robustly — when the restored set is shorter than expected
      // is NOT a signal, so we compare against the group's original paths.
      const entries = (data ?? []).filter(
        (e) => e.delete_group_id === groupId,
      );
      const originals = new Set(entries.map((e) => e.original_path));
      const anySuffixed = res.paths.some((p) => !originals.has(p));
      if (anySuffixed) {
        setNotice(
          "Some pages already existed, so they were restored with a '(restored)' suffix.",
        );
      } else {
        setNotice(null);
      }
    },
    onError: () => {
      setNotice("We couldn't restore that folder just now. Try again.");
    },
  });

  if (isLoading) {
    return <p className="trashview-status">Loading…</p>;
  }
  if (isError) {
    return <p className="trashview-status">Couldn't load Trash — try again.</p>;
  }

  const entries = data ?? [];
  const rows = buildRows(entries);

  return (
    <section className="trashview">
      <header className="trashview-header">
        <h1 className="trashview-title">
          <Trash2 size={20} aria-hidden="true" />
          <span>Trash</span>
        </h1>
      </header>

      {notice && (
        <div className="trashview-notice" role="status">
          {notice}
        </div>
      )}

      {rows.length === 0 ? (
        <div className="trashview-empty">
          <h2 className="trashview-empty-heading">Trash is empty</h2>
          <p className="trashview-empty-body">
            Pages you delete will appear here, and you can restore them anytime.
          </p>
        </div>
      ) : (
        <ul className="trashview-list">
          {rows.map((row) =>
            row.kind === "group" ? (
              <li className="trashview-row" key={`group-${row.groupId}`}>
                <div className="trashview-row-main">
                  <span className="trashview-row-title">
                    Folder '{row.folderName}' · {row.count}{" "}
                    {row.count === 1 ? "page" : "pages"}
                  </span>
                  <span className="trashview-row-meta">
                    Deleted by {row.deletedBy} · {relativeTime(row.deletedAt)}
                  </span>
                </div>
                <button
                  type="button"
                  className="btn btn-primary trashview-restore"
                  onClick={() => restoreGroupMut.mutate(row.groupId)}
                  disabled={restoreGroupMut.isPending}
                >
                  <Undo2 size={16} aria-hidden="true" />
                  <span>Restore folder</span>
                </button>
              </li>
            ) : (
              <li className="trashview-row" key={row.entry.id}>
                <div className="trashview-row-main">
                  <span className="trashview-row-title">{row.entry.title}</span>
                  <span className="trashview-row-meta">
                    Deleted by {row.entry.deleted_by} ·{" "}
                    {relativeTime(row.entry.deleted_at)}
                  </span>
                </div>
                <button
                  type="button"
                  className="btn btn-secondary trashview-restore"
                  onClick={() => restoreMut.mutate(row.entry)}
                  disabled={restoreMut.isPending}
                >
                  <Undo2 size={16} aria-hidden="true" />
                  <span>Restore page</span>
                </button>
              </li>
            ),
          )}
        </ul>
      )}
    </section>
  );
}

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Undo2, Trash2 } from "lucide-react";

import { listTrash, restoreFromTrash, type TrashEntry } from "../api/client";
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

// TrashView lists deleted pages and lets an editor restore each one. Delete is a
// recycle bin: every entry shows its title, who deleted it, and when (relative
// time) — never a path-as-identity or any Git vocabulary. Restoring auto-suffixes
// on a name collision so a live page is never clobbered (D-10); when that
// happens an informational notice is shown.
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

  if (isLoading) {
    return <p className="trashview-status">Loading…</p>;
  }
  if (isError) {
    return <p className="trashview-status">Couldn't load Trash — try again.</p>;
  }

  const entries = data ?? [];

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

      {entries.length === 0 ? (
        <div className="trashview-empty">
          <h2 className="trashview-empty-heading">Trash is empty</h2>
          <p className="trashview-empty-body">
            Pages you delete will appear here, and you can restore them anytime.
          </p>
        </div>
      ) : (
        <ul className="trashview-list">
          {entries.map((e) => (
            <li className="trashview-row" key={e.id}>
              <div className="trashview-row-main">
                <span className="trashview-row-title">{e.title}</span>
                <span className="trashview-row-meta">
                  Deleted by {e.deleted_by} · {relativeTime(e.deleted_at)}
                </span>
              </div>
              <button
                type="button"
                className="btn btn-secondary trashview-restore"
                onClick={() => restoreMut.mutate(e)}
                disabled={restoreMut.isPending}
              >
                <Undo2 size={16} aria-hidden="true" />
                <span>Restore page</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

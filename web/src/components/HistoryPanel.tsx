import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  getHistory,
  viewVersion,
  type HistoryEntry,
  type Page,
} from "../api/client";
import Dialog from "./Dialog";
import MarkdownProse from "./MarkdownProse";
import RestoreConfirmDialog from "./RestoreConfirmDialog";
import { relativeTime } from "./TrashView";
import "./HistoryPanel.css";

export interface HistoryPanelProps {
  open: boolean;
  path: string;
  onClose: () => void;
}

// actionLabel maps a backend action token to a calm, human, NON-Git verb. The
// UI must never show the raw action token if it reads like Git; these labels are
// all plain English ("Edited", "Created", …). Unknown actions fall back to
// "Updated" so a new action token never leaks Git vocabulary.
function actionLabel(action: string): string {
  switch (action) {
    case "create":
      return "Created";
    case "edit":
      return "Edited";
    case "rename":
      return "Renamed";
    case "move":
      return "Moved";
    case "trash":
      return "Deleted";
    case "restore":
    case "restore-version":
      return "Restored";
    default:
      return "Updated";
  }
}

// HistoryPanel shows a page's version history as a plain-language list (VER-02):
// each row reads "{Edited} by {name} · {relative time}" with ZERO Git vocabulary
// and NO version token shown. A row can be previewed ("View this version" renders
// the old version via MarkdownProse) and restored ("Restore this version" opens
// the non-destructive RestoreConfirmDialog). When a page has only its first
// version, the single-version empty state is shown.
export default function HistoryPanel({
  open,
  path,
  onClose,
}: HistoryPanelProps) {
  const [viewing, setViewing] = useState<HistoryEntry | null>(null);
  const [restoring, setRestoring] = useState<HistoryEntry | null>(null);

  const { data, isLoading, isError } = useQuery<HistoryEntry[]>({
    queryKey: ["history", path],
    queryFn: () => getHistory(path),
    enabled: open && path !== "",
  });

  const { data: viewedPage } = useQuery<Page>({
    queryKey: ["version", path, viewing?.version],
    queryFn: () => viewVersion(path, viewing!.version),
    enabled: viewing !== null,
  });

  const entries = data ?? [];

  return (
    <Dialog open={open} title="Version history" onCancel={onClose} hideFooter>
      {isLoading && <p className="historypanel-status">Loading…</p>}
      {isError && (
        <p className="historypanel-status">
          Couldn't load this page's history — try again.
        </p>
      )}

      {!isLoading && !isError && entries.length <= 1 && (
        <p className="historypanel-empty">
          This page has only its first version so far. Every time you save, a new
          version is added here.
        </p>
      )}

      {!isLoading && !isError && entries.length > 1 && (
        <ul className="historypanel-list">
          {entries.map((e) => (
            <li className="historypanel-row" key={e.version}>
              <span className="historypanel-row-meta">
                {actionLabel(e.action)} by {e.who} · {relativeTime(e.when)}
              </span>
              <span className="historypanel-row-actions">
                <button
                  type="button"
                  className="btn btn-ghost historypanel-view"
                  onClick={() =>
                    setViewing((cur) =>
                      cur?.version === e.version ? null : e,
                    )
                  }
                >
                  {viewing?.version === e.version
                    ? "Hide this version"
                    : "View this version"}
                </button>
                <button
                  type="button"
                  className="btn btn-primary historypanel-restore"
                  onClick={() => setRestoring(e)}
                >
                  Restore this version
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {viewing && viewedPage && (
        <div className="historypanel-preview">
          <MarkdownProse body={viewedPage.body} />
        </div>
      )}

      {restoring && (
        <RestoreConfirmDialog
          open={restoring !== null}
          path={path}
          version={restoring.version}
          dateLabel={relativeTime(restoring.when)}
          onClose={() => setRestoring(null)}
          onRestored={onClose}
        />
      )}
    </Dialog>
  );
}

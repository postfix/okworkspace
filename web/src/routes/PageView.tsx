import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Pencil } from "lucide-react";

import { getPage, me, type Me, type Page } from "../api/client";
import { okfTitle } from "../lib/frontmatter";
import { useRecent } from "../stores/recent";
import MarkdownProse from "../components/MarkdownProse";
import PageActionMenu from "../components/PageActionMenu";
import RenameModal from "../components/RenameModal";
import MoveDialog from "../components/MoveDialog";
import DeleteConfirmDialog from "../components/DeleteConfirmDialog";
import "./PageView.css";

// PageView is Read mode (/app/page/:path). It renders the committed Markdown via
// MarkdownProse (sanitized), records the opened page in the client-side recent
// store, and shows an "Edit page" affordance to editors only.
export default function PageView() {
  const params = useParams();
  const navigate = useNavigate();
  const path = params["*"] ?? "";
  const visit = useRecent((s) => s.visit);

  const { data: meData } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const canEdit = meData?.role === "editor" || meData?.role === "admin";
  const [renameOpen, setRenameOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const { data, isLoading, isError, error } = useQuery<Page>({
    queryKey: ["page", path],
    queryFn: () => getPage(path),
    retry: false,
    enabled: path !== "",
  });

  const title = data ? okfTitle(data.frontmatter, path) : path;

  // Record the opened page (NAV-05) once it has loaded.
  useEffect(() => {
    if (data && path) {
      visit({ path, title });
    }
    // title is derived from data; depending on data+path is sufficient.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, path]);

  if (isLoading) {
    return <p className="pageview-status">Loading…</p>;
  }
  if (isError) {
    const status = (error as Error & { status?: number })?.status;
    if (status === 404) {
      return (
        <div className="pageview-status">
          <p>This page no longer exists. It may have been moved or deleted.</p>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => navigate("/app")}
          >
            Back to your workspace
          </button>
        </div>
      );
    }
    return <p className="pageview-status">Something went wrong. Check your connection and try again.</p>;
  }

  const body = data?.body ?? "";

  return (
    <article className="pageview">
      <header className="pageview-header">
        <h1 className="pageview-title">{title}</h1>
        <div className="pageview-actions">
          {canEdit && (
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => navigate(`/app/edit/${path}`)}
            >
              <Pencil size={16} aria-hidden="true" />
              <span>Edit page</span>
            </button>
          )}
          <PageActionMenu
            canEdit={canEdit}
            onEdit={() => navigate(`/app/edit/${path}`)}
            onRename={() => setRenameOpen(true)}
            onMove={() => setMoveOpen(true)}
            onHistory={() => {
              /* Version history panel arrives in a later plan (VER-02). */
            }}
            onDelete={() => setDeleteOpen(true)}
          />
        </div>
      </header>
      {body.trim() === "" ? (
        <p className="pageview-empty">This page is empty. Select Edit to start writing.</p>
      ) : (
        <MarkdownProse body={body} />
      )}
      <RenameModal
        open={renameOpen}
        path={path}
        currentTitle={title}
        onClose={() => setRenameOpen(false)}
      />
      <MoveDialog open={moveOpen} path={path} onClose={() => setMoveOpen(false)} />
      <DeleteConfirmDialog
        open={deleteOpen}
        path={path}
        title={title}
        onClose={() => setDeleteOpen(false)}
      />
    </article>
  );
}

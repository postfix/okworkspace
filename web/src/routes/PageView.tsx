import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { BookOpen, Paperclip } from "lucide-react";

import Dialog from "../components/Dialog";

import { getPage, me, type Me, type Page } from "../api/client";
import { okfTitle } from "../lib/frontmatter";
import { useRecent } from "../stores/recent";
import LivePreviewEditor from "../components/LivePreviewEditor";
import BacklinksPanel from "../components/BacklinksPanel";
import AttachmentsSection from "../components/attachments/AttachmentsSection";
import PageActionMenu from "../components/PageActionMenu";
import RenameModal from "../components/RenameModal";
import MoveDialog from "../components/MoveDialog";
import DeleteConfirmDialog from "../components/DeleteConfirmDialog";
import HistoryPanel from "../components/HistoryPanel";
import Breadcrumb from "../components/Breadcrumb";
import PropertiesPanel from "../components/PropertiesPanel";
import DocStatusBar from "../components/DocStatusBar";
import "./PageView.css";

// PageView is Read mode (/app/page/:path). It renders the committed Markdown on the
// UNIFIED read-only live-preview surface (a read-only LivePreviewEditor) — pixel-
// identical to edit Live mode, with github-slugger heading ids + #hash deep-link
// scroll (SRCH-06) and internal `.md` SPA navigation, but non-editable. It records
// the opened page in the client-side recent store and shows an "Edit page"
// affordance to editors only.
export default function PageView() {
  const params = useParams();
  const navigate = useNavigate();
  const path = params["*"] ?? "";
  const visit = useRecent((s) => s.visit);
  // The header paperclip opens the attachments in a centered modal.
  const [attachmentsOpen, setAttachmentsOpen] = useState(false);

  const { data: meData } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const canEdit = meData?.role === "editor" || meData?.role === "admin";
  const [renameOpen, setRenameOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);

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
        <div className="pageview-heading">
          <Breadcrumb path={path} title={title} />
          <h1 className="pageview-title">{title}</h1>
        </div>
        <div className="pageview-actions">
          {canEdit && (
            <button
              type="button"
              className="btn btn-ghost icon-btn"
              onClick={() => navigate(`/app/edit/${path}`)}
              aria-label="Switch to editing"
              title="Switch to editing"
            >
              {/* Reading view: the icon reflects the CURRENT mode (a book = you are
                  reading); the tooltip says what a click does (switch to editing). */}
              <BookOpen size={18} aria-hidden="true" />
            </button>
          )}
          <button
            type="button"
            className="btn btn-ghost icon-btn"
            onClick={() => setAttachmentsOpen(true)}
            aria-label="Attachments"
            title="Attachments"
          >
            <Paperclip size={18} aria-hidden="true" />
          </button>
          <PageActionMenu
            canEdit={canEdit}
            onEdit={() => navigate(`/app/edit/${path}`)}
            onRename={() => setRenameOpen(true)}
            onMove={() => setMoveOpen(true)}
            onHistory={() => setHistoryOpen(true)}
            onDelete={() => setDeleteOpen(true)}
          />
        </div>
      </header>
      {/* Read-only Properties (tags + description + scalar keys) under the title,
          mirroring the editor's metadata block — Obsidian parity. Renders nothing
          when the page has no displayable properties. */}
      {data && (
        <PropertiesPanel body={body} frontmatter={data.frontmatter} readOnly />
      )}
      {body.trim() === "" ? (
        <p className="pageview-empty">This page is empty. Select Edit to start writing.</p>
      ) : (
        <LivePreviewEditor
          value={body}
          onChange={() => {}}
          currentPath={path}
          mode="live"
          readOnly
        />
      )}
      <DocStatusBar path={path} body={body} />
      <BacklinksPanel path={path} />
      <Dialog
        open={attachmentsOpen}
        title="Attachments"
        onCancel={() => setAttachmentsOpen(false)}
        hideFooter
        className="dialog-wide"
      >
        <AttachmentsSection
          pagePath={path}
          canEdit={canEdit}
          maxUploadMB={100}
          allowedTypes={["pdf", "docx", "txt", "png", "jpg", "svg"]}
          hideHeader
        />
      </Dialog>
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
      <HistoryPanel
        open={historyOpen}
        path={path}
        onClose={() => setHistoryOpen(false)}
      />
    </article>
  );
}

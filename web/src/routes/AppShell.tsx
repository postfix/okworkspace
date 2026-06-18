import { useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FilePlus, FolderPlus, Shield, Trash2 } from "lucide-react";

import { health, logout, me, type Me, type RepoHealth } from "../api/client";
import UserMenu from "../components/UserMenu";
import LeftTree from "../components/LeftTree";
import RecentList from "../components/RecentList";
import CreatePageModal from "../components/CreatePageModal";
import CreateFolderModal from "../components/CreateFolderModal";
import "./AppShell.css";

// AppShell is the authenticated chrome (top bar + nav rail + main pane). When
// given children it renders them in the main pane (e.g. Admin, Profile);
// otherwise it shows the empty-state. The nav rail hosts the live page tree
// (LeftTree) and the client-side recent list (RecentList).
export default function AppShell({ children }: { children?: ReactNode }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const { data: repoHealth } = useQuery<RepoHealth>({
    queryKey: ["health"],
    queryFn: health,
  });

  const isAdmin = data?.role === "admin";
  // Editors (and admins) can create pages/folders; readers cannot (RBAC mirror
  // of the server gate — the create affordances are hidden for readers).
  const canEdit = data?.role === "editor" || data?.role === "admin";
  const [createPageOpen, setCreatePageOpen] = useState(false);
  const [createFolderOpen, setCreateFolderOpen] = useState(false);

  async function handleLogout() {
    await logout();
    queryClient.removeQueries({ queryKey: ["me"] });
    navigate("/login", { replace: true });
  }

  return (
    <div className="appshell">
      <header className="topbar">
        <button
          type="button"
          className="topbar-wordmark topbar-wordmark-btn"
          onClick={() => navigate("/app")}
        >
          OKF Workspace
        </button>
        <div className="topbar-right">
          {repoHealth?.ok && (
            <span className="repo-health" title={repoHealth.detail}>
              <span className="repo-health-dot" aria-hidden="true" />
              <span>Storage healthy</span>
            </span>
          )}
          <UserMenu
            displayName={data?.display_name ?? ""}
            onProfile={() => navigate("/profile")}
            onLogout={handleLogout}
          />
        </div>
      </header>

      <div className="appshell-body">
        <nav className="navrail" aria-label="Workspace navigation">
          {canEdit && (
            <div className="navrail-actions">
              <button
                type="button"
                className="navrow navrow-action"
                onClick={() => setCreatePageOpen(true)}
              >
                <FilePlus size={16} aria-hidden="true" />
                <span>New page</span>
              </button>
              <button
                type="button"
                className="navrow navrow-action"
                onClick={() => setCreateFolderOpen(true)}
              >
                <FolderPlus size={16} aria-hidden="true" />
                <span>New folder</span>
              </button>
            </div>
          )}

          <LeftTree />
          <RecentList />

          <div className="navrail-trash">
            <button
              type="button"
              className="navrow navrow-action"
              onClick={() => navigate("/trash")}
            >
              <Trash2 size={16} aria-hidden="true" />
              <span>Trash</span>
            </button>
          </div>

          {isAdmin && (
            <div className="navrail-admin">
              <button
                type="button"
                className="navrow navrow-action"
                onClick={() => navigate("/admin")}
              >
                <Shield size={16} aria-hidden="true" />
                <span>Admin</span>
              </button>
            </div>
          )}
        </nav>

        <main className="mainpane">
          {repoHealth && !repoHealth.ok && !repoHealth.diverged && (
            <div className="banner banner-warning" role="alert">
              Storage is reporting a problem. Your work may not be saving —
              contact your administrator.
            </div>
          )}
          {repoHealth?.diverged && (
            <div className="banner banner-warning" role="alert">
              The remote repository has diverged. Automatic sync was paused to
              protect your data — contact your administrator.
            </div>
          )}
          {repoHealth?.self_healed && !repoHealth.diverged && (
            <div className="banner banner-warning" role="status">
              Recovered from an interrupted save. Everything looks fine.
            </div>
          )}

          {children ? (
            children
          ) : (
            <div className="empty-state">
              <h1 className="empty-state-heading">Pick a page to get started</h1>
              <p className="empty-state-body">
                Choose a page from the left, or create a new one.
              </p>
            </div>
          )}
        </main>
      </div>

      <CreatePageModal
        open={createPageOpen}
        folder=""
        folderName="your workspace"
        onClose={() => setCreatePageOpen(false)}
      />
      <CreateFolderModal
        open={createFolderOpen}
        parent=""
        onClose={() => setCreateFolderOpen(false)}
      />
    </div>
  );
}

import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Folder, Shield } from "lucide-react";

import { health, logout, me, type Me, type RepoHealth } from "../api/client";
import UserMenu from "../components/UserMenu";
import "./AppShell.css";

// Static placeholder tree (read-only/disabled). The real seeded tree is wired
// in a later phase; here it only communicates "editing arrives next".
const PLACEHOLDER_TREE = [
  { label: "index.md", kind: "file" as const },
  { label: "runbooks", kind: "folder" as const },
  { label: "architecture", kind: "folder" as const },
  { label: "decisions", kind: "folder" as const },
];

// AppShell is the authenticated chrome (top bar + nav rail + main pane). When
// given children it renders them in the main pane (e.g. Admin, Profile);
// otherwise it shows the Phase-0 empty-state placeholder.
export default function AppShell({ children }: { children?: ReactNode }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const { data: repoHealth } = useQuery<RepoHealth>({
    queryKey: ["health"],
    queryFn: health,
  });

  const isAdmin = data?.role === "admin";

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
          <ul className="navtree">
            {PLACEHOLDER_TREE.map((node) => (
              <li
                key={node.label}
                className="navrow navrow-disabled"
                aria-disabled="true"
              >
                {node.kind === "folder" ? (
                  <Folder size={16} aria-hidden="true" />
                ) : (
                  <FileText size={16} aria-hidden="true" />
                )}
                <span>{node.label}</span>
              </li>
            ))}
          </ul>

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
              <h1 className="empty-state-heading">Your workspace is ready</h1>
              <p className="empty-state-body">
                Page editing arrives in the next release. For now you can browse
                the starter structure on the left.
                {isAdmin
                  ? " Your admin can add teammates from the admin screen."
                  : ""}
              </p>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

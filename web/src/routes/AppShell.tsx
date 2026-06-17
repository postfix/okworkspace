import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Folder, LogOut } from "lucide-react";

import { logout, me, type Me } from "../api/client";
import "./AppShell.css";

// Static placeholder tree (read-only/disabled). The real seeded tree is wired
// in Plan 02; here it only communicates "editing arrives next".
const PLACEHOLDER_TREE = [
  { label: "index.md", kind: "file" as const, depth: 0 },
  { label: "runbooks", kind: "folder" as const, depth: 0 },
  { label: "architecture", kind: "folder" as const, depth: 0 },
  { label: "decisions", kind: "folder" as const, depth: 0 },
];

export default function AppShell() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data } = useQuery<Me>({ queryKey: ["me"], queryFn: me });

  const isAdmin = data?.role === "admin";

  async function handleLogout() {
    await logout();
    queryClient.removeQueries({ queryKey: ["me"] });
    navigate("/login", { replace: true });
  }

  return (
    <div className="appshell">
      <header className="topbar">
        <div className="topbar-wordmark">OKF Workspace</div>
        <div className="topbar-right">
          <span className="topbar-displayname">{data?.display_name}</span>
          <button className="btn-ghost" type="button" onClick={handleLogout}>
            <LogOut size={16} aria-hidden="true" />
            <span>Log out</span>
          </button>
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
        </nav>

        <main className="mainpane">
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
        </main>
      </div>
    </div>
  );
}

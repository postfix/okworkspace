import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, UserPlus } from "lucide-react";

import {
  createUser,
  deactivateUser,
  listUsers,
  me,
  reindexSearch,
  resetUserPassword,
  setUserRole,
  type AdminUser,
  type Me,
  type UserRole,
} from "../api/client";
import Table, { type Column } from "../components/Table";
import RoleBadge from "../components/RoleBadge";
import Dialog from "../components/Dialog";
import "./Admin.css";

const USERS_KEY = ["admin", "users"];

export default function Admin() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data: users = [], isLoading } = useQuery<AdminUser[]>({
    queryKey: USERS_KEY,
    queryFn: listUsers,
  });
  const { data: current } = useQuery<Me>({ queryKey: ["me"], queryFn: me });

  // CR-03 (UI mirror of the server invariant): the server rejects demoting or
  // deactivating the last active admin and self-lockout. Mirror that here so the
  // destructive control is hidden rather than failing the user with a 409.
  const activeAdminCount = users.filter(
    (u) => u.role === "admin" && u.active,
  ).length;

  // canDeactivate reports whether the Deactivate control should be shown for u.
  // It hides the control for the signed-in user (no self-deactivate) and for the
  // last active admin (would lock the instance out of admin functions).
  function canDeactivate(u: AdminUser): boolean {
    if (!u.active) return false;
    if (current && u.username === current.username) return false;
    if (u.role === "admin" && activeAdminCount <= 1) return false;
    return true;
  }

  // isLastActiveAdmin mirrors the server invariant (ErrLastAdmin): demoting the
  // only active admin would lock the instance out, so the dialog disables the
  // non-admin role options for that user.
  function isLastActiveAdmin(u: AdminUser): boolean {
    return u.role === "admin" && u.active && activeAdminCount <= 1;
  }

  // Add-user dialog state.
  const [addOpen, setAddOpen] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newDisplayName, setNewDisplayName] = useState("");
  const [newRole, setNewRole] = useState<UserRole>("reader");
  const [addError, setAddError] = useState<string | null>(null);

  // One-time-password display (after create or reset).
  const [otpNotice, setOtpNotice] = useState<{ name: string; otp: string } | null>(
    null,
  );

  // Confirmation dialogs.
  const [resetTarget, setResetTarget] = useState<AdminUser | null>(null);
  const [deactivateTarget, setDeactivateTarget] = useState<AdminUser | null>(null);

  // Change-role dialog state.
  const [roleTarget, setRoleTarget] = useState<AdminUser | null>(null);
  const [roleValue, setRoleValue] = useState<UserRole>("reader");
  const [roleError, setRoleError] = useState<string | null>(null);

  function refresh() {
    queryClient.invalidateQueries({ queryKey: USERS_KEY });
  }

  const createMut = useMutation({
    mutationFn: () =>
      createUser(newUsername.trim(), newDisplayName.trim(), newRole),
    onSuccess: (created) => {
      setOtpNotice({ name: created.display_name, otp: created.one_time_password });
      setAddOpen(false);
      setNewUsername("");
      setNewDisplayName("");
      setNewRole("reader");
      setAddError(null);
      refresh();
    },
    onError: (err: Error) => {
      setAddError(err.message);
    },
  });

  const resetMut = useMutation({
    mutationFn: (id: number) => resetUserPassword(id),
    onSuccess: (res) => {
      if (resetTarget) {
        setOtpNotice({ name: resetTarget.display_name, otp: res.one_time_password });
      }
      setResetTarget(null);
      refresh();
    },
  });

  const deactivateMut = useMutation({
    mutationFn: (id: number) => deactivateUser(id),
    onSuccess: () => {
      setDeactivateTarget(null);
      refresh();
    },
  });

  const roleMut = useMutation({
    mutationFn: ({ id, role }: { id: number; role: UserRole }) =>
      setUserRole(id, role),
    onSuccess: () => {
      setRoleTarget(null);
      setRoleError(null);
      refresh();
    },
    onError: (err: Error) => {
      setRoleError(err.message);
    },
  });

  // Rebuild-search-index action (SRCH admin operational story). The endpoint is
  // admin-only + CSRF on the server; the button only renders in this admin route.
  // The rebuild is async (202), so success shows a brief muted confirmation rather
  // than blocking. Hidden-Git rule: the label says "Rebuild search index", never
  // "Reindex Bleve"; errors carry no Git/index vocabulary.
  const [reindexNotice, setReindexNotice] = useState<string | null>(null);
  const [reindexError, setReindexError] = useState<string | null>(null);
  const reindexMut = useMutation({
    mutationFn: () => reindexSearch(),
    onSuccess: () => {
      setReindexError(null);
      setReindexNotice("Search index rebuild started.");
    },
    onError: (err: Error) => {
      setReindexNotice(null);
      setReindexError(err.message);
    },
  });

  function openRoleDialog(u: AdminUser) {
    setRoleTarget(u);
    setRoleValue(u.role as UserRole);
    setRoleError(null);
  }

  function handleAddSubmit(e: FormEvent) {
    e.preventDefault();
    setAddError(null);
    createMut.mutate();
  }

  const columns: Column<AdminUser>[] = [
    { key: "display_name", header: "Display name", render: (u) => u.display_name },
    { key: "username", header: "Username", render: (u) => u.username },
    { key: "role", header: "Role", render: (u) => <RoleBadge role={u.role} /> },
    {
      key: "status",
      header: "Status",
      render: (u) => (
        <span className={u.active ? "status-active" : "status-inactive"}>
          {u.active ? "Active" : "Deactivated"}
        </span>
      ),
    },
  ];

  return (
    <div className="admin">
      <button
        type="button"
        className="btn btn-ghost admin-back"
        onClick={() => navigate("/app")}
      >
        <ArrowLeft size={16} aria-hidden="true" />
        <span>Back to workspace</span>
      </button>

      <div className="admin-header">
        <h1 className="admin-heading">Users</h1>
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => {
            setAddError(null);
            setAddOpen(true);
          }}
        >
          <UserPlus size={16} aria-hidden="true" />
          <span>Create user</span>
        </button>
      </div>

      {otpNotice && (
        <div className="banner banner-success admin-otp" role="status">
          One-time password for {otpNotice.name}:{" "}
          <code className="admin-otp-code">{otpNotice.otp}</code>. Share it
          securely — it won't be shown again.
          <button
            type="button"
            className="btn btn-ghost admin-otp-dismiss"
            onClick={() => setOtpNotice(null)}
          >
            Dismiss
          </button>
        </div>
      )}

      <section className="admin-section">
        <h2 className="admin-section-heading">Search</h2>
        <p className="admin-muted">
          Rebuild the search index if results look out of date. This runs in the
          background and won't interrupt anyone.
        </p>
        <div className="admin-section-row">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => reindexMut.mutate()}
            disabled={reindexMut.isPending}
          >
            {reindexMut.isPending ? "Starting…" : "Rebuild search index"}
          </button>
          {reindexNotice && (
            <span className="admin-muted" role="status">
              {reindexNotice}
            </span>
          )}
        </div>
        {reindexError && (
          <div className="field-error" role="alert">
            {reindexError}
          </div>
        )}
      </section>

      {isLoading ? (
        <p className="admin-muted">Loading…</p>
      ) : users.length === 0 ? (
        <div className="admin-empty">
          <h2 className="admin-empty-heading">No teammates yet</h2>
          <p className="admin-muted">
            Add your team so they can sign in. Use Create user to get started.
          </p>
        </div>
      ) : (
        <Table<AdminUser>
          columns={columns}
          rows={users}
          rowKey={(u) => u.id}
          actions={(u) => (
            <>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => openRoleDialog(u)}
              >
                Change role
              </button>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setResetTarget(u)}
              >
                Reset password
              </button>
              {canDeactivate(u) && (
                <button
                  type="button"
                  className="btn btn-ghost-destructive"
                  onClick={() => setDeactivateTarget(u)}
                >
                  Deactivate
                </button>
              )}
            </>
          )}
        />
      )}

      {/* Add-user dialog (its own form footer). */}
      <Dialog
        open={addOpen}
        title="Create user"
        onCancel={() => setAddOpen(false)}
        hideFooter
      >
        <form className="admin-add-form" onSubmit={handleAddSubmit}>
          <div className="field">
            <label className="field-label" htmlFor="add-username">
              Username
            </label>
            <input
              id="add-username"
              className="input"
              type="text"
              value={newUsername}
              onChange={(e) => setNewUsername(e.target.value)}
              disabled={createMut.isPending}
              required
            />
          </div>
          <div className="field">
            <label className="field-label" htmlFor="add-displayname">
              Display name
            </label>
            <input
              id="add-displayname"
              className="input"
              type="text"
              value={newDisplayName}
              onChange={(e) => setNewDisplayName(e.target.value)}
              disabled={createMut.isPending}
              required
            />
          </div>
          <div className="field">
            <label className="field-label" htmlFor="add-role">
              Role
            </label>
            <select
              id="add-role"
              className="select"
              value={newRole}
              onChange={(e) => setNewRole(e.target.value as UserRole)}
              disabled={createMut.isPending}
            >
              <option value="reader">Reader</option>
              <option value="editor">Editor</option>
              <option value="admin">Admin</option>
            </select>
          </div>
          {addError && (
            <div className="field-error" role="alert">
              {addError}
            </div>
          )}
          <div className="dialog-footer">
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setAddOpen(false)}
              disabled={createMut.isPending}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={createMut.isPending}
            >
              {createMut.isPending ? "Creating…" : "Create user"}
            </button>
          </div>
        </form>
      </Dialog>

      {/* Change-role dialog (custom footer so confirm can disable on invalid). */}
      <Dialog
        open={roleTarget !== null}
        title={`Change role for ${roleTarget?.display_name ?? ""}`}
        onCancel={() => setRoleTarget(null)}
        hideFooter
      >
        <div className="field">
          <label className="field-label" htmlFor="change-role">
            Role
          </label>
          <select
            id="change-role"
            className="select"
            value={roleValue}
            onChange={(e) => setRoleValue(e.target.value as UserRole)}
            disabled={roleMut.isPending}
          >
            <option value="reader">Reader</option>
            <option value="editor">Editor</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        {roleTarget && isLastActiveAdmin(roleTarget) && (
          <p className="admin-muted">
            This is the last admin — promote another admin before changing this
            role.
          </p>
        )}
        {roleError && (
          <div className="field-error" role="alert">
            {roleError}
          </div>
        )}
        <div className="dialog-footer">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => setRoleTarget(null)}
            disabled={roleMut.isPending}
          >
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={
              roleMut.isPending ||
              !roleTarget ||
              roleValue === roleTarget.role ||
              (isLastActiveAdmin(roleTarget) && roleValue !== "admin")
            }
            onClick={() =>
              roleTarget && roleMut.mutate({ id: roleTarget.id, role: roleValue })
            }
          >
            {roleMut.isPending ? "Working…" : "Save role"}
          </button>
        </div>
      </Dialog>

      {/* Reset-password confirmation. */}
      <Dialog
        open={resetTarget !== null}
        title={`Reset password for ${resetTarget?.display_name ?? ""}?`}
        confirmLabel="Reset password"
        cancelLabel="Keep current password"
        busy={resetMut.isPending}
        onCancel={() => setResetTarget(null)}
        onConfirm={() => resetTarget && resetMut.mutate(resetTarget.id)}
      >
        This generates a new temporary password they must change on next sign-in.
      </Dialog>

      {/* Deactivate confirmation (destructive). */}
      <Dialog
        open={deactivateTarget !== null}
        title={`Deactivate ${deactivateTarget?.display_name ?? ""}?`}
        confirmLabel="Deactivate"
        cancelLabel="Keep account active"
        destructive
        busy={deactivateMut.isPending}
        onCancel={() => setDeactivateTarget(null)}
        onConfirm={() =>
          deactivateTarget && deactivateMut.mutate(deactivateTarget.id)
        }
      >
        They won't be able to sign in until reactivated. Their account and history
        are kept.
      </Dialog>
    </div>
  );
}

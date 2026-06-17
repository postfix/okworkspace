import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";

import { changePassword, me, updateProfile, type Me } from "../api/client";
import "./Profile.css";

// Profile is the self-service screen: a user changes their own display name and
// password, but NEVER their own role (D-06 — there is no role control here, and
// the server ignores any role field). Rendered inside the AppShell chrome.
export default function Profile() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data } = useQuery<Me>({ queryKey: ["me"], queryFn: me });

  const [displayName, setDisplayName] = useState(data?.display_name ?? "");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function handleSave(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setNotice(null);

    // Validate the password change only if the user is attempting one.
    const changingPassword = newPassword.length > 0 || confirmPassword.length > 0;
    if (changingPassword) {
      if (newPassword.length < 12) {
        setError("Choose a longer password — at least 12 characters.");
        return;
      }
      if (newPassword !== confirmPassword) {
        setError("The two passwords don't match.");
        return;
      }
    }

    setSaving(true);
    try {
      const trimmed = displayName.trim();
      if (trimmed && trimmed !== data?.display_name) {
        await updateProfile(trimmed);
      }
      if (changingPassword) {
        await changePassword(currentPassword, newPassword);
        setCurrentPassword("");
        setNewPassword("");
        setConfirmPassword("");
      }
      const fresh = await me();
      queryClient.setQueryData(["me"], fresh);
      setNotice("Your profile is saved.");
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Something went wrong. Check your connection and try again.",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="profile">
      <button
        type="button"
        className="btn btn-ghost profile-back"
        onClick={() => navigate("/app")}
      >
        <ArrowLeft size={16} aria-hidden="true" />
        <span>Back to workspace</span>
      </button>

      <h1 className="profile-heading">Profile</h1>

      <form className="profile-form" onSubmit={handleSave}>
        <div className="field">
          <label className="field-label" htmlFor="profile-displayname">
            Display name
          </label>
          <input
            id="profile-displayname"
            className="input"
            type="text"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            disabled={saving}
          />
        </div>

        <h2 className="profile-subheading">Change password</h2>

        <div className="field">
          <label className="field-label" htmlFor="profile-current">
            Current password
          </label>
          <input
            id="profile-current"
            className="input"
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            disabled={saving}
          />
        </div>

        <div className="field">
          <label className="field-label" htmlFor="profile-new">
            New password
          </label>
          <input
            id="profile-new"
            className="input"
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            disabled={saving}
          />
          <span className="field-help">At least 12 characters.</span>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="profile-confirm">
            Confirm new password
          </label>
          <input
            id="profile-confirm"
            className="input"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            disabled={saving}
          />
        </div>

        {error && (
          <div className="field-error" role="alert">
            {error}
          </div>
        )}
        {notice && (
          <div className="banner banner-success" role="status">
            {notice}
          </div>
        )}

        <div>
          <button className="btn btn-primary" type="submit" disabled={saving}>
            {saving ? "Saving…" : "Save profile"}
          </button>
        </div>
      </form>
    </div>
  );
}

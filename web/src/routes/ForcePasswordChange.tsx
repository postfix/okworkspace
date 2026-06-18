import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";

import { changePassword, me } from "../api/client";
import "./CenteredCard.css";

// ForcePasswordChange gates the app when /auth/me reports must_change_password
// (D-02). It is rendered by the App router guard, NOT reachable as a normal
// route, so a user with a temporary password cannot bypass it. The server is
// the authority for the gate (CR-01) — any authenticated route except the
// password change is rejected while must_change_password is set.
//
// WR-06: this component DELIBERATELY takes no props. The temporary password is
// never threaded through props/state from a parent — the user re-enters it in
// the field below, so plaintext credentials cannot leak via React
// devtools/error boundaries/props inspection.
export default function ForcePasswordChange() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (next.length < 12) {
      setError("Choose a longer password — at least 12 characters.");
      return;
    }
    if (next !== confirm) {
      setError("The two passwords don't match.");
      return;
    }
    setSubmitting(true);
    try {
      await changePassword(current, next);
      // Refresh the session so must_change_password clears, then enter the app.
      const fresh = await me();
      queryClient.setQueryData(["me"], fresh);
      navigate("/app", { replace: true });
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Something went wrong. Check your connection and try again.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="centered-screen">
      <form className="centered-card" onSubmit={handleSubmit}>
        <h1 className="centered-title">Set a new password</h1>
        <div className="banner banner-warning" role="status">
          You're using a temporary password. Choose a new one to continue.
        </div>

        <div className="field">
          <label className="field-label" htmlFor="fpc-current">
            Temporary password
          </label>
          <input
            id="fpc-current"
            className="input"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            disabled={submitting}
            required
          />
        </div>

        <div className="field">
          <label className="field-label" htmlFor="fpc-new">
            New password
          </label>
          <input
            id="fpc-new"
            className="input"
            type="password"
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            disabled={submitting}
            required
          />
          <span className="field-help">At least 12 characters.</span>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="fpc-confirm">
            Confirm new password
          </label>
          <input
            id="fpc-confirm"
            className="input"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            disabled={submitting}
            required
          />
        </div>

        {error && (
          <div className="field-error" role="alert">
            {error}
          </div>
        )}

        <button className="btn btn-primary" type="submit" disabled={submitting}>
          {submitting ? (
            <>
              <Loader2 className="spinner" size={18} aria-hidden="true" />
              <span>Updating…</span>
            </>
          ) : (
            "Update password"
          )}
        </button>
      </form>
    </div>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  acquireLock,
  forceLock,
  getPage,
  releaseLock,
  savePage,
  type Page,
} from "../api/client";
import { readField, setField } from "../lib/frontmatter";
import { getConnId } from "../lib/connId";
import AutosaveStatus, { type SaveState } from "../components/AutosaveStatus";
import LinkPicker from "../components/LinkPicker";
import LivePreviewEditor from "../components/LivePreviewEditor";
import PresenceIndicator from "../components/PresenceIndicator";
import SoftLockBanner from "../components/SoftLockBanner";
import { useEditorMode } from "../stores/editorMode";
import "./PageEditor.css";

// LOCK_HEARTBEAT_MS is the soft-lock refresh interval (COLL-02). It sits well
// inside the server's 2-minute lock TTL so a held lock never lapses mid-edit; a
// crashed/closed session stops refreshing and is reaped by GC within one TTL
// window. Acquire doubles as the refresh (a same-session Acquire re-stamps TTL).
const LOCK_HEARTBEAT_MS = 30_000;

// Debounce interval (RESEARCH Open Q2: client-driven idle save). A keystroke
// schedules an autosave ~1s after typing stops. Every write goes through the same
// PUT (the backend cuts a hidden version on each write in Phase 1). A single
// serialized coalescing saver replaces the earlier draft+idle two-timer scheme,
// which could fire overlapping saves and let a stale snapshot clobber a newer one.
const DRAFT_DEBOUNCE_MS = 1000;

// PageEditor is Edit mode. The body is edited as a raw Markdown string (never a
// block model — protects the byte-stable round-trip); a small frontmatter form
// edits title/tags/description as text patched into the raw YAML. Autosave never
// blocks typing; a 409 surfaces the ConflictBanner with a Reload action.
export default function PageEditor() {
  const params = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const path = params["*"] ?? "";

  // Live/Source editor mode — a persisted per-device UI preference (default Live).
  // The CM6 surface reads `mode`; the toolbar toggle and the Cmd/Ctrl-E shortcut
  // (mode.ts toggleKeymap) both drive this single store. Switching modes never
  // touches the document bytes (EDIT-02).
  const mode = useEditorMode((s) => s.mode);
  const setMode = useEditorMode((s) => s.setMode);

  const { data, isLoading } = useQuery<Page>({
    queryKey: ["page", path],
    queryFn: () => getPage(path),
    retry: false,
    enabled: path !== "",
  });

  const [body, setBody] = useState("");
  const [frontmatter, setFrontmatter] = useState("");
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [conflict, setConflict] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // Soft-lock state (COLL-02). When another live session holds the lock, lockedBy
  // is their username → the editor renders read-only beneath the SoftLockBanner.
  // It is null when you hold the lock (or after you Force edit) → the surface is
  // editable. forceBusy/forceFailed drive the banner's take-over button states.
  const [lockedBy, setLockedBy] = useState<string | null>(null);
  const [forceBusy, setForceBusy] = useState(false);
  const [forceFailed, setForceFailed] = useState(false);
  // The stable per-tab connection id is the lock SessionID (client-generated,
  // opaque). useRef so it is read once and shared by every lock call this mount.
  const connId = useRef(getConnId());
  // readOnly mirrors lockedBy for the editor surface + Save gating. Held-by-other
  // ⇒ genuinely read-only (no caret, Save disabled); never a mere visual dim.
  const readOnly = lockedBy !== null;

  // The base revision is the optimistic-concurrency token read at open time; it
  // advances after each successful save so subsequent saves are not self-409s.
  const baseRevision = useRef("");
  const draftTimer = useRef<number | null>(null);

  // saving is the in-flight guard (WR-03): it drops any overlapping save so two
  // PUTs never race on the same base revision and self-409. bodyRef/frontmatterRef
  // always hold the LATEST edited content so a timer scheduled before a state
  // update still saves fresh content (fixes the useCallback stale-closure).
  const saving = useRef(false);
  const bodyRef = useRef("");
  const frontmatterRef = useRef("");
  // The exact content the last successful save SENT to the server. After a save
  // settles we compare the live refs against this to detect a trailing edit typed
  // while the save was in flight, so it is flushed instead of silently lost.
  const lastSavedBody = useRef("");
  const lastSavedFrontmatter = useRef("");

  // Seed local state ONCE, on first load. It must NOT re-run on later `data`
  // changes (e.g. the post-save invalidate refetch) — doing so would overwrite
  // edits the user typed while a save was in flight (silent lost write).
  const seeded = useRef(false);
  useEffect(() => {
    if (data && !seeded.current) {
      seeded.current = true;
      setBody(data.body);
      setFrontmatter(data.frontmatter);
      bodyRef.current = data.body;
      frontmatterRef.current = data.frontmatter;
      baseRevision.current = data.revision;
      lastSavedBody.current = data.body;
      lastSavedFrontmatter.current = data.frontmatter;
    }
  }, [data]);

  // Soft-lock lifecycle (COLL-02): acquire the lock on entering Edit, refresh it
  // on a ~30s heartbeat while we hold it, and best-effort release it on unmount /
  // path change. Acquire doubles as the heartbeat refresh (a same-session Acquire
  // re-stamps the TTL); a held-by-other result flips the surface read-only and
  // names the holder. The effect re-runs per page path so navigating between pages
  // releases the old lock and acquires the new one.
  useEffect(() => {
    if (path === "") return;
    const conn = connId.current;
    let cancelled = false;

    async function tryAcquire() {
      try {
        const res = await acquireLock(path, conn);
        if (cancelled) return;
        // Don't override an in-progress force-edit takeover with a stale heartbeat.
        setLockedBy(res.result === "held-by-other" ? res.holder?.username ?? "Someone" : null);
      } catch {
        // A failed acquire/heartbeat must not break editing — the worst case is we
        // briefly don't know the lock state; the next tick retries. Leave the
        // current read-only state as-is (GC reaps a truly abandoned lock).
      }
    }

    void tryAcquire();
    const timer = window.setInterval(() => void tryAcquire(), LOCK_HEARTBEAT_MS);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
      // Best-effort release; GC is the backstop, so swallow any error.
      void releaseLock(path, conn).catch(() => {});
    };
  }, [path]);

  // runSaver is a single-flight, serialized, coalescing saver. Only one loop runs
  // at a time (the `saving` guard → WR-03: two PUTs never race on a base revision).
  // The loop keeps saving until the on-disk content matches the live editor, so a
  // trailing edit made while a save was in flight is ALWAYS persisted (never a
  // silent lost write) and a stale snapshot can never clobber a newer one. Each
  // iteration reads the freshest content from the refs and re-reads the advanced
  // base revision, so there is no stale-closure or stale-revision window.
  const runSaver = useCallback(
    async (force: boolean) => {
      if (saving.current) return; // a saver loop is already running
      saving.current = true;
      setSaveError(null);
      try {
        // `force` guarantees at least one PUT for an explicit Save click even when
        // content is unchanged (e.g. to record a deliberate version).
        let mustSave = force;
        while (
          mustSave ||
          bodyRef.current !== lastSavedBody.current ||
          frontmatterRef.current !== lastSavedFrontmatter.current
        ) {
          mustSave = false;
          const sentBody = bodyRef.current;
          const sentFrontmatter = frontmatterRef.current;
          setSaveState("saving");
          try {
            await savePage(path, {
              body: sentBody,
              frontmatter: sentFrontmatter,
              base_revision: baseRevision.current,
            });
          } catch (err) {
            const status = (err as Error & { status?: number }).status;
            if (status === 409) {
              setConflict(true);
              setSaveState("idle");
              return;
            }
            setSaveError(
              "We couldn't save your page just now. Your changes are kept here — check your connection and try Save again.",
            );
            setSaveState("idle");
            return;
          }
          // Read back the advanced revision (and any frontmatter repair) so the
          // next loop iteration saves against fresh state — never a stale 409.
          // The save already succeeded; if this read-back fails (transient error,
          // server restart, timeout) we must NOT leave the UI stuck on "Saving…"
          // nor advance lastSavedBody/lastSavedFrontmatter or baseRevision past
          // what was confirmed — a stale baseRevision would 409-falsely on the
          // next autosave. Surface the same recoverable save-error and bail; the
          // `finally` below still releases the single-flight `saving` guard.
          let fresh: Awaited<ReturnType<typeof getPage>>;
          try {
            fresh = await getPage(path);
          } catch {
            setSaveError(
              "We couldn't save your page just now. Your changes are kept here — check your connection and try Save again.",
            );
            setSaveState("idle");
            return;
          }
          baseRevision.current = fresh.revision;
          lastSavedBody.current = sentBody;
          lastSavedFrontmatter.current = sentFrontmatter;
          // Refresh the soft lock on a successful save (COLL-02): a save is proof
          // of an active editing session, so re-stamp the TTL alongside the ~30s
          // heartbeat. Fire-and-forget — a failed refresh is reaped by GC and must
          // never block or fail the save that already succeeded.
          void acquireLock(path, connId.current).catch(() => {});
          queryClient.invalidateQueries({ queryKey: ["page", path] });
          queryClient.invalidateQueries({ queryKey: ["tree"] });
          // Loop re-checks: if the user typed during the awaits above, save again.
        }
        // Caught up: the server now holds exactly what the editor shows.
        setSaveState("saved");
      } finally {
        saving.current = false;
      }
    },
    [path, queryClient],
  );

  // Schedule an autosave on each edit. A single debounce timer is armed and
  // rescheduled on every change so typing never triggers a mid-keystroke save.
  // The saver itself coalesces (and never drops) trailing edits, so one timer is
  // enough — no separate idle-escalation timer that could race and clobber.
  const scheduleAutosave = useCallback(() => {
    if (draftTimer.current) window.clearTimeout(draftTimer.current);
    draftTimer.current = window.setTimeout(() => void runSaver(false), DRAFT_DEBOUNCE_MS);
  }, [runSaver]);

  useEffect(
    () => () => {
      if (draftTimer.current) window.clearTimeout(draftTimer.current);
    },
    [],
  );

  function onBodyChange(value?: string) {
    const next = value ?? "";
    setBody(next);
    bodyRef.current = next; // keep the ref the saver reads in sync (WR-03)
    setSaveState("idle");
    scheduleAutosave();
  }

  // insertLink appends a relative `.md` Markdown link emitted by the LinkPicker
  // to the body (D-05/D-06) and schedules an autosave.
  function insertLink(markdown: string) {
    setBody((b) => {
      const next = b === "" ? markdown : `${b} ${markdown}`;
      bodyRef.current = next;
      return next;
    });
    setSaveState("idle");
    scheduleAutosave();
  }

  function onFieldChange(field: string, value: string) {
    setFrontmatter((fm) => {
      const next = setField(fm, field, value);
      frontmatterRef.current = next;
      return next;
    });
    setSaveState("idle");
    scheduleAutosave();
  }

  function onSaveClick() {
    if (draftTimer.current) window.clearTimeout(draftTimer.current);
    void runSaver(true);
  }

  // onForceEdit takes over the soft lock so this session may type (COLL-02). It
  // calls forceLock ALONE — it never calls savePage and never touches
  // baseRevision: force-edit is "take over editing", NOT a save bypass. A
  // subsequent save still runs the revision check (and 409s into the conflict path
  // if someone committed in between) — the load-bearing safety rule. On success the
  // read-only wash lifts and the caret enters; on a transient failure the banner
  // shows the retry copy and stays read-only.
  async function onForceEdit() {
    setForceBusy(true);
    setForceFailed(false);
    try {
      await forceLock(path, connId.current);
      setLockedBy(null);
    } catch {
      setForceFailed(true);
    } finally {
      setForceBusy(false);
    }
  }

  if (isLoading) {
    return <p className="pageeditor-status">Loading…</p>;
  }

  return (
    <div className="pageeditor">
      {lockedBy && (
        <SoftLockBanner
          holderName={lockedBy}
          busy={forceBusy}
          failed={forceFailed}
          onForceEdit={() => void onForceEdit()}
        />
      )}
      {conflict && (
        <div className="banner banner-warning" role="alert">
          This page was changed somewhere else since you opened it. Reload to see
          the latest version before saving again.
          <button
            type="button"
            className="btn btn-secondary pageeditor-reload"
            onClick={() => navigate(0)}
          >
            Reload page
          </button>
        </div>
      )}
      {saveError && (
        <div className="banner banner-warning" role="alert">
          {saveError}
        </div>
      )}

      <div className="pageeditor-frontmatter">
        <div className="field">
          <label className="field-label" htmlFor="fm-title">
            Title
          </label>
          <input
            id="fm-title"
            className="input"
            type="text"
            value={readField(frontmatter, "title")}
            onChange={(e) => onFieldChange("title", e.target.value)}
          />
        </div>
        <div className="field">
          <label className="field-label" htmlFor="fm-description">
            Description
          </label>
          <input
            id="fm-description"
            className="input"
            type="text"
            value={readField(frontmatter, "description")}
            onChange={(e) => onFieldChange("description", e.target.value)}
          />
        </div>
      </div>

      <div className="pageeditor-toolbar">
        <LinkPicker fromPath={path} onInsert={insertLink} />
        {/* Quiet "who else is editing" line (COLL-01), left of the flex spacer so
            the Live/Source mode segment stays right-aligned. It is editor-only
            (PageEditor IS Edit mode — readers never reach it) and reuses the
            Plan-02 per-tab connection id so your own presence is excluded and two
            tabs are distinguishable. The component owns its own subscribe/
            unsubscribe lifecycle; PageEditor only mounts it. */}
        <PresenceIndicator path={path} conn={connId.current} />
        <div
          className="pageeditor-mode"
          role="group"
          aria-label="Editor mode"
          title="Toggle live preview (⌘E / Ctrl+E)"
        >
          <button
            type="button"
            className="btn btn-secondary pageeditor-mode-btn"
            aria-pressed={mode === "live"}
            onClick={() => setMode("live")}
          >
            Live
          </button>
          <button
            type="button"
            className="btn btn-secondary pageeditor-mode-btn"
            aria-pressed={mode === "source"}
            onClick={() => setMode("source")}
          >
            Source
          </button>
        </div>
      </div>

      {readOnly && (
        <p className="pageeditor-readonly-hint" aria-hidden="true">
          View only — {lockedBy} is editing. Choose <strong>Force edit</strong> to
          take over.
        </p>
      )}

      <div
        className={
          readOnly ? "pageeditor-surface pageeditor-surface-readonly" : "pageeditor-surface"
        }
      >
        <LivePreviewEditor
          value={body}
          onChange={onBodyChange}
          currentPath={path}
          mode={mode}
          readOnly={readOnly}
        />
      </div>

      <div className="pageeditor-actions">
        <AutosaveStatus state={saveState} />
        <button
          type="button"
          className="btn btn-primary"
          onClick={onSaveClick}
          disabled={readOnly}
        >
          Save page
        </button>
      </div>
    </div>
  );
}

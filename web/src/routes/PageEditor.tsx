import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  acquireLock,
  createPage,
  deletePage,
  forceLock,
  getPage,
  releaseLock,
  savePage,
  type Page,
} from "../api/client";
import { readField, setField } from "../lib/frontmatter";
import { getConnId } from "../lib/connId";
import AutosaveStatus, { type SaveState } from "../components/AutosaveStatus";
import DiffReviewDialog, {
  type ConflictBusy,
} from "../components/DiffReviewDialog";
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
  // Conflict state (COLL-04). On a stale-save 409 the editor fetches the current
  // server version and opens DiffReviewDialog in conflict mode (oldText = their
  // server body, newText = my body) — superseding the Phase 1 banner. conflict is
  // null when no collision is open. conflictBusy labels the in-flight resolution
  // button (Overwrite → "Saving…", Save-as-copy → "Saving copy…").
  const [conflict, setConflict] = useState<{
    serverBody: string;
    serverRevision: string;
  } | null>(null);
  const [conflictBusy, setConflictBusy] = useState<ConflictBusy>(null);
  // mergeReference holds the server version surfaced for reference after a Manual
  // merge (the server version MUST stay visible while reconciling). null = hidden.
  const [mergeReference, setMergeReference] = useState<string | null>(null);
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
  // forcedRef short-circuits a single in-flight stale "held-by-other" heartbeat that
  // resolves right AFTER a successful Force edit — without it that late response would
  // wash the surface back to read-only even though we now hold the lock (WR-01). It is
  // set on force success and cleared as soon as a heartbeat confirms we hold the lock
  // ("acquired"), so a genuine later takeover by someone else still re-locks this tab.
  const forcedRef = useRef(false);
  // readOnly mirrors lockedBy for the editor surface + Save gating. Held-by-other
  // ⇒ genuinely read-only (no caret, Save disabled); never a mere visual dim.
  const readOnly = lockedBy !== null;
  // readOnlyRef mirrors readOnly for the save path, which reads it from stable
  // callbacks/timers (runSaver, scheduleAutosave) without taking readOnly as a dep.
  // It is the defense-in-depth gate: a save is never armed or run while this session
  // does not hold the lock, so even a stray edit (or an autosave timer armed a beat
  // before the lock arrived) can never persist a held-by-other session's changes.
  const readOnlyRef = useRef(false);
  useEffect(() => {
    readOnlyRef.current = readOnly;
  }, [readOnly]);

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
    // New page path → no force takeover is in flight yet; start from a clean guard.
    forcedRef.current = false;

    async function tryAcquire() {
      try {
        const res = await acquireLock(path, conn);
        if (cancelled) return;
        if (res.result === "held-by-other") {
          // Ignore a stale heartbeat that was already in flight when we Force-took the
          // lock — applying it would wash us back to read-only (WR-01). A real later
          // takeover is still surfaced once forcedRef has been cleared by an "acquired".
          if (forcedRef.current) return;
          setLockedBy(res.holder?.username ?? "Someone");
        } else {
          // Confirmed this session holds the lock: clear the force guard so a genuine
          // later takeover by someone else can re-lock us on a subsequent heartbeat.
          forcedRef.current = false;
          setLockedBy(null);
        }
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
      // Never save while another session holds the lock (COLL-02). The save path is
      // deliberately lock-independent at the server, so this client gate is what keeps
      // a held-by-other "View only" session from autosaving its edits — the banner's
      // "won't be saved until you take over" promise. Force edit clears readOnly first.
      if (readOnlyRef.current) return;
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
              // A stale save collided with someone else's commit. Fetch the CURRENT
              // server version and open the conflict dialog (oldText = their body,
              // newText = mine) — the resolution surface that supersedes the banner.
              // baseRevision is deliberately NOT advanced: my edits stay intact and
              // every resolution choice re-routes through the revision-checked save.
              try {
                const server = await getPage(path);
                setConflict({
                  serverBody: server.body,
                  serverRevision: server.revision,
                });
              } catch {
                // If we can't fetch the server version we can't show a real diff;
                // fall back to the recoverable save-error rather than a prose-only
                // or fabricated conflict surface.
                setSaveError(
                  "We couldn't save your page just now. Your changes are kept here — check your connection and try Save again.",
                );
              }
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
  // conflictOpenRef mirrors `conflict !== null` for the autosave gate. A ref (not
  // the state) so scheduleAutosave's identity is stable and a timer armed BEFORE
  // the conflict opened still sees the latest gate when it fires (Pitfall 5).
  const conflictOpenRef = useRef(false);
  useEffect(() => {
    conflictOpenRef.current = conflict !== null;
  }, [conflict]);

  const scheduleAutosave = useCallback(() => {
    // Do NOT re-arm autosave while a conflict dialog is open — otherwise every
    // debounce would fire a save that 409s again and re-open the dialog (thrash).
    // The in-flight edit is preserved in the editor; it saves once the conflict is
    // resolved (which advances baseRevision and resumes autosave). (Pitfall 5.)
    if (conflictOpenRef.current) return;
    // Do NOT arm autosave while another session holds the lock (held-by-other). The
    // editor is read-only in that state, so there should be nothing to save — but this
    // is the belt-and-suspenders gate that guarantees a locked-out session never
    // persists edits even if a change slips through (COLL-02).
    if (readOnlyRef.current) return;
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
      // We now hold the lock. Guard against a stale in-flight heartbeat (dispatched
      // while the other session still held it) re-locking us read-only (WR-01).
      forcedRef.current = true;
    } catch {
      setForceFailed(true);
    } finally {
      setForceBusy(false);
    }
  }

  // --- Conflict resolution handlers (COLL-04) ---
  // Every choice routes through the EXISTING revision-checked save path — there is
  // no conflict endpoint and no silent overwrite. Overwrite saves at the CURRENT
  // revision (re-409s if another commit lands mid-overwrite → re-open); Save-as-copy
  // Creates a fresh deduped page and writes my body there (the original is never
  // touched, proven by TestSaveAsCopyLeavesOriginal); Manual merge keeps my body
  // with the server version visible, then lets a normal Save run.

  // onConflictOverwrite saves MY version against the page's CURRENT revision. It
  // re-fetches the revision first (the conflict may be stale by now), then Saves. A
  // re-409 (another commit landed between the fetch and the save) re-opens the
  // dialog with the NEWER server version — never a silent clobber.
  async function onConflictOverwrite() {
    setConflictBusy("overwrite");
    setSaveError(null);
    try {
      const fresh = await getPage(path);
      try {
        await savePage(path, {
          body: bodyRef.current,
          frontmatter: frontmatterRef.current,
          base_revision: fresh.revision,
        });
      } catch (err) {
        const status = (err as Error & { status?: number }).status;
        if (status === 409) {
          // Yet another commit landed — re-open with the newer server version.
          const server = await getPage(path);
          setConflict({
            serverBody: server.body,
            serverRevision: server.revision,
          });
          return;
        }
        throw err;
      }
      // Overwrite landed. Advance the base revision so the next save is not a
      // self-409, sync the last-saved refs, resolve the conflict, and resume.
      const after = await getPage(path);
      baseRevision.current = after.revision;
      lastSavedBody.current = bodyRef.current;
      lastSavedFrontmatter.current = frontmatterRef.current;
      setConflict(null);
      setMergeReference(null);
      setSaveState("saved");
      void acquireLock(path, connId.current).catch(() => {});
      queryClient.invalidateQueries({ queryKey: ["page", path] });
      queryClient.invalidateQueries({ queryKey: ["tree"] });
    } catch {
      setSaveError(
        "We couldn't save your page just now. Your changes are kept here — check your connection and try Save again.",
      );
    } finally {
      setConflictBusy(null);
    }
  }

  // onConflictManualMerge closes the dialog, keeps my body in the editor, and
  // surfaces the server version for reference (it MUST stay visible while I
  // reconcile). No backend call here — the eventual Save runs the revision check.
  function onConflictManualMerge() {
    if (conflict) setMergeReference(conflict.serverBody);
    setConflict(null);
    // Advance baseRevision to the server's so a normal Save after reconciling does
    // not instantly re-409 against the version I am now merging against.
    if (conflict) baseRevision.current = conflict.serverRevision;
    setSaveState("idle");
  }

  // onConflictSaveAsCopy creates a NEW deduped page ("{title} (Copy)") and writes
  // my body into it at its FRESH revision, then navigates there. The original page
  // is NEVER written and never carries the conflicted base revision (the zero-loss
  // escape hatch; backend proof: TestSaveAsCopyLeavesOriginal).
  async function onConflictSaveAsCopy() {
    setConflictBusy("copy");
    setSaveError(null);
    // Track the freshly-minted copy so we can compensate if writing its body fails —
    // otherwise an empty "{title} (Copy)" stub would orphan in the tree (IN-02).
    let createdPath: string | null = null;
    try {
      const title = readField(frontmatterRef.current, "title") || "Untitled";
      const slash = path.lastIndexOf("/");
      const folder = slash === -1 ? "" : path.slice(0, slash);
      const { path: newPath } = await createPage(folder, `${title} (Copy)`);
      createdPath = newPath;
      const fresh = await getPage(newPath);
      await savePage(newPath, {
        body: bodyRef.current,
        frontmatter: setField(fresh.frontmatter, "title", `${title} (Copy)`),
        base_revision: fresh.revision,
      });
      createdPath = null; // copy fully persisted — nothing to compensate
      setConflict(null);
      setMergeReference(null);
      setSaveState("saved");
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      // Open the copy (transient "Saved as a copy. Opening it now.").
      navigate(`/app/edit/${newPath}`);
    } catch {
      // Best-effort: remove the empty copy we created before the body landed, then
      // reconcile the tree so no orphaned stub lingers (IN-02). The original page is
      // never touched, and the user's body stays in the editor.
      if (createdPath) await deletePage(createdPath).catch(() => {});
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      setSaveError(
        "We couldn't save a copy just now. Your changes are kept here — check your connection and try again.",
      );
    } finally {
      setConflictBusy(null);
    }
  }

  // onConflictCancel (Esc / backdrop) applies NOTHING: my edits stay in the editor,
  // the server is untouched, and the conflict is re-resolvable on the next save.
  function onConflictCancel() {
    if (conflictBusy) return; // don't dismiss mid-save
    setConflict(null);
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
      {/* COLL-04: a stale-save 409 opens the conflict dialog (a REAL diff with
          three risk-ranked, safe-by-default choices) — superseding the Phase 1
          banner. Every choice routes through the revision-checked save path; no
          choice can silently overwrite. */}
      <DiffReviewDialog
        open={conflict !== null}
        mode="conflict"
        title="This page was changed somewhere else"
        summary="Someone saved a different version while you were editing. Compare your version with theirs, then choose what to keep."
        columnCaption="Left: the saved version · Right: your unsaved version"
        oldText={conflict?.serverBody ?? ""}
        newText={body}
        conflictBusy={conflictBusy}
        onReject={onConflictCancel}
        onOverwrite={() => void onConflictOverwrite()}
        onManualMerge={onConflictManualMerge}
        onSaveAsCopy={() => void onConflictSaveAsCopy()}
      />
      {mergeReference !== null && (
        <details className="pageeditor-merge-reference" open>
          <summary>Their saved version (for reference)</summary>
          <pre className="pageeditor-merge-reference-body">{mergeReference}</pre>
          <button
            type="button"
            className="btn btn-ghost pageeditor-merge-reference-dismiss"
            onClick={() => setMergeReference(null)}
          >
            Hide
          </button>
        </details>
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

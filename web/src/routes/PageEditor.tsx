import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { getPage, savePage, type Page } from "../api/client";
import { readField, setField } from "../lib/frontmatter";
import AutosaveStatus, { type SaveState } from "../components/AutosaveStatus";
import LinkPicker from "../components/LinkPicker";
import LivePreviewEditor from "../components/LivePreviewEditor";
import { useEditorMode } from "../stores/editorMode";
import "./PageEditor.css";

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

  if (isLoading) {
    return <p className="pageeditor-status">Loading…</p>;
  }

  return (
    <div className="pageeditor">
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

      <div className="pageeditor-surface">
        <LivePreviewEditor
          value={body}
          onChange={onBodyChange}
          currentPath={path}
          mode={mode}
        />
      </div>

      <div className="pageeditor-actions">
        <AutosaveStatus state={saveState} />
        <button type="button" className="btn btn-primary" onClick={onSaveClick}>
          Save page
        </button>
      </div>
    </div>
  );
}

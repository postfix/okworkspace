import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import MDEditor from "@uiw/react-md-editor";

import { getPage, savePage, type Page } from "../api/client";
import { readField, setField } from "../lib/frontmatter";
import AutosaveStatus, { type SaveState } from "../components/AutosaveStatus";
import LinkPicker from "../components/LinkPicker";
import "./PageEditor.css";

// Debounce intervals (RESEARCH Open Q2: client-driven idle save). A keystroke
// schedules a draft autosave ~1s later; ~6s of idle escalates to a version save.
// Both go through the same PUT (the backend cuts a hidden version on every write
// in Phase 1; the draft/version distinction is reflected in the status copy).
const DRAFT_DEBOUNCE_MS = 1000;
const IDLE_COMMIT_MS = 6000;

// PageEditor is Edit mode. The body is edited as a raw Markdown string (never a
// block model — protects the byte-stable round-trip); a small frontmatter form
// edits title/tags/description as text patched into the raw YAML. Autosave never
// blocks typing; a 409 surfaces the ConflictBanner with a Reload action.
export default function PageEditor() {
  const params = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const path = params["*"] ?? "";

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
  const idleTimer = useRef<number | null>(null);

  // Seed local state once the page loads.
  useEffect(() => {
    if (data) {
      setBody(data.body);
      setFrontmatter(data.frontmatter);
      baseRevision.current = data.revision;
    }
  }, [data]);

  const doSave = useCallback(
    async (cutVersion: boolean) => {
      setSaveState("saving");
      setSaveError(null);
      try {
        await savePage(path, {
          body,
          frontmatter,
          base_revision: baseRevision.current,
        });
        // Refetch to pick up the new revision (and any frontmatter repair).
        const fresh = await getPage(path);
        baseRevision.current = fresh.revision;
        queryClient.invalidateQueries({ queryKey: ["page", path] });
        queryClient.invalidateQueries({ queryKey: ["tree"] });
        setSaveState(cutVersion ? "saved" : "draft-saved");
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
      }
    },
    [body, frontmatter, path, queryClient],
  );

  // Schedule a draft autosave and an idle version save on each edit. Rescheduled
  // on every change so typing never triggers a mid-keystroke save.
  const scheduleAutosave = useCallback(() => {
    if (draftTimer.current) window.clearTimeout(draftTimer.current);
    if (idleTimer.current) window.clearTimeout(idleTimer.current);
    draftTimer.current = window.setTimeout(() => void doSave(false), DRAFT_DEBOUNCE_MS);
    idleTimer.current = window.setTimeout(() => void doSave(true), IDLE_COMMIT_MS);
  }, [doSave]);

  useEffect(
    () => () => {
      if (draftTimer.current) window.clearTimeout(draftTimer.current);
      if (idleTimer.current) window.clearTimeout(idleTimer.current);
    },
    [],
  );

  function onBodyChange(value?: string) {
    setBody(value ?? "");
    setSaveState("idle");
    scheduleAutosave();
  }

  // insertLink appends a relative `.md` Markdown link emitted by the LinkPicker
  // to the body (D-05/D-06) and schedules an autosave.
  function insertLink(markdown: string) {
    setBody((b) => (b === "" ? markdown : `${b} ${markdown}`));
    setSaveState("idle");
    scheduleAutosave();
  }

  function onFieldChange(field: string, value: string) {
    setFrontmatter((fm) => setField(fm, field, value));
    setSaveState("idle");
    scheduleAutosave();
  }

  function onSaveClick() {
    if (draftTimer.current) window.clearTimeout(draftTimer.current);
    if (idleTimer.current) window.clearTimeout(idleTimer.current);
    void doSave(true);
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
      </div>

      <div className="pageeditor-surface" data-color-mode="light">
        <MDEditor value={body} onChange={onBodyChange} height={480} />
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

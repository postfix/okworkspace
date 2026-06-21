import { useRef, useState, type KeyboardEvent } from "react";
import { Library, Loader2, SendHorizontal, FileText, AlertTriangle } from "lucide-react";

import "./PromptBar.css";

// AgentMode is the assistant mode the PromptBar offers. "propose" opens the
// DiffReviewDialog flow; the rest stream/return an answer into the AgentPanel.
// Shared with AppShell (the session owner) and AgentPanel (the meta line).
export type AgentMode = "ask" | "summarize" | "rewrite" | "draft" | "propose";

// PromptBarStatus is the in-flight indicator state. "thinking" is before the
// first token (tool calls may take a beat); "streaming" once tokens arrive.
export type PromptBarStatus = "idle" | "thinking" | "streaming";

// PromptScope is the auto-detected (or workspace-overridden) context the prompt
// runs against. The PromptBar reflects it in the read-only context chip.
export type PromptScope = "page" | "workspace";

export interface PromptBarProps {
  mode: AgentMode;
  onModeChange: (mode: AgentMode) => void;
  // canEdit gates the editor-only modes (Rewrite/Propose). Readers may Ask/
  // Summarize/Draft but never reach a write proposal — Propose is disabled.
  canEdit: boolean;
  // hasPage is true when a page is open (page scope available). With no page open
  // the scope defaults to workspace and page-scoped modes are unavailable.
  hasPage: boolean;
  pageTitle?: string;
  // workspace reflects the "Whole workspace" toggle; onWorkspaceToggle flips it.
  workspace: boolean;
  onWorkspaceToggle: () => void;
  status: PromptBarStatus;
  // disabledReason, when set, renders the bar disabled with an inline explanation
  // (agent off / provider unreachable) — fail-closed, never a silent hang.
  disabledReason?: string | null;
  // unreachable distinguishes a transient provider failure (AlertTriangle) from
  // the agent being turned off (plain copy), per UI-SPEC.
  unreachable?: boolean;
  // error is a transient last-request failure shown until the next submit.
  error?: string | null;
  onSubmit: (prompt: string) => void;
  onCancel: () => void;
}

// Per-mode copy (placeholder + submit label). The label tracks the mode (verb +
// implied noun); the placeholder hints what to type. No Git vocabulary.
const MODE_COPY: Record<AgentMode, { placeholder: string; submit: string; label: string }> = {
  ask: { placeholder: "Ask the assistant…", submit: "Ask", label: "Ask" },
  summarize: {
    placeholder: "Summarize this page…",
    submit: "Summarize",
    label: "Summarize",
  },
  rewrite: { placeholder: "Describe the rewrite…", submit: "Rewrite", label: "Rewrite" },
  draft: { placeholder: "Describe the page to draft…", submit: "Draft", label: "Draft" },
  propose: {
    placeholder: "Describe the change to propose…",
    submit: "Propose",
    label: "Propose a patch",
  },
};

export default function PromptBar({
  mode,
  onModeChange,
  canEdit,
  hasPage,
  pageTitle,
  workspace,
  onWorkspaceToggle,
  status,
  disabledReason,
  unreachable,
  error,
  onSubmit,
  onCancel,
}: PromptBarProps) {
  const [value, setValue] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const inFlight = status !== "idle";
  const disabled = !!disabledReason;
  const copy = MODE_COPY[mode];
  // Propose/Rewrite need editor role; Propose additionally needs a page in scope.
  const proposeAvailable = canEdit && hasPage && !workspace;
  const rewriteAvailable = canEdit;

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      // Enter submits; Shift+Enter inserts a newline.
      e.preventDefault();
      submit();
    } else if (e.key === "Escape" && inFlight) {
      // Esc cancels an in-flight request.
      e.preventDefault();
      onCancel();
    }
  }

  function submit() {
    const trimmed = value.trim();
    if (!trimmed || disabled || inFlight) return;
    onSubmit(trimmed);
  }

  const scopeChip = workspace
    ? { icon: <Library size={14} aria-hidden="true" />, label: "Whole workspace" }
    : hasPage
      ? { icon: <FileText size={14} aria-hidden="true" />, label: pageTitle || "This page" }
      : { icon: <Library size={14} aria-hidden="true" />, label: "Whole workspace" };

  return (
    <div className="promptbar" role="region" aria-label="Assistant prompt">
      <div className="promptbar-row">
        <select
          className="select promptbar-mode"
          aria-label="Assistant mode"
          value={mode}
          disabled={disabled || inFlight}
          onChange={(e) => onModeChange(e.target.value as AgentMode)}
        >
          <option value="ask">Ask</option>
          <option value="summarize">Summarize</option>
          <option value="rewrite" disabled={!rewriteAvailable}>
            Rewrite
          </option>
          <option value="draft">Draft</option>
          <option value="propose" disabled={!proposeAvailable}>
            Propose a patch
          </option>
        </select>

        {/* Read-only context chip — reflects scope, never captures focus. */}
        <span className="promptbar-chip" aria-hidden="true" title={scopeChip.label}>
          {scopeChip.icon}
          <span className="promptbar-chip-label">{scopeChip.label}</span>
        </span>

        <button
          type="button"
          className="btn btn-ghost promptbar-workspace"
          aria-pressed={workspace}
          aria-label="Search the whole workspace"
          disabled={disabled || inFlight}
          onClick={onWorkspaceToggle}
        >
          Whole workspace
        </button>

        <textarea
          ref={textareaRef}
          className="input promptbar-input"
          rows={1}
          placeholder={copy.placeholder}
          value={value}
          disabled={disabled || inFlight}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          aria-label="Prompt"
        />

        <div className="promptbar-status" aria-live="polite">
          {status === "thinking" && (
            <>
              <Loader2 size={16} className="spinner" aria-hidden="true" />
              <span>Thinking…</span>
            </>
          )}
          {status === "streaming" && (
            <>
              <Loader2 size={16} className="spinner" aria-hidden="true" />
              <span>Streaming…</span>
            </>
          )}
        </div>

        {inFlight ? (
          <button
            type="button"
            className="btn btn-secondary promptbar-submit"
            onClick={onCancel}
          >
            Stop
          </button>
        ) : (
          <button
            type="button"
            className="btn btn-primary promptbar-submit"
            disabled={disabled || value.trim() === ""}
            onClick={submit}
          >
            <SendHorizontal size={16} aria-hidden="true" />
            {copy.submit}
          </button>
        )}
      </div>

      {disabledReason && (
        <p className="promptbar-note" role="status">
          {unreachable && <AlertTriangle size={14} aria-hidden="true" />}
          <span>{disabledReason}</span>
        </p>
      )}
      {!disabledReason && error && (
        <p className="promptbar-note promptbar-note-error" role="alert">
          <AlertTriangle size={14} aria-hidden="true" />
          <span>{error}</span>
        </p>
      )}
    </div>
  );
}

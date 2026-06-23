import { useRef, useState, type KeyboardEvent } from "react";
import { Loader2, AlertTriangle } from "lucide-react";

import "./PromptBar.css";

// AgentMode is the assistant action a prompt runs as. "ask" is the default
// conversational chat; the others are surfaced as suggestion chips inside the
// Assistant window (not as a mode picker on the prompt). Shared with AppShell (the
// session owner) and AgentPanel (the suggestion chips + meta line).
export type AgentMode = "ask" | "summarize" | "rewrite" | "draft" | "propose";

// PromptBarStatus is the in-flight indicator state. "thinking" is before the
// first token (tool calls may take a beat); "streaming" once tokens arrive.
export type PromptBarStatus = "idle" | "thinking" | "streaming";

// PromptScope is the auto-detected context the prompt runs against (page when one
// is open, else the whole workspace). It is shown as a muted hint, never a toggle.
export type PromptScope = "page" | "workspace";

export interface PromptBarProps {
  // placeholder reflects the active action (AppShell computes it from the mode a
  // suggestion chip primed — "Ask anything…" by default).
  placeholder: string;
  // contextLabel is the muted scope hint ("This page" / "Whole workspace" /
  // "Selection (N chars)" / an attachment name). Read-only, never interactive.
  contextLabel: string;
  status: PromptBarStatus;
  // disabledReason, when set, renders the input disabled with an inline note
  // (agent off / provider unreachable) — fail-closed, never a silent hang.
  disabledReason?: string | null;
  // unreachable distinguishes a transient provider failure (AlertTriangle) from
  // the agent being turned off (plain copy).
  unreachable?: boolean;
  // error is a transient last-request failure shown until the next submit.
  error?: string | null;
  onSubmit: (prompt: string) => void;
  // onCancel is Esc: it stops an in-flight stream AND clears any primed action,
  // returning the prompt to plain ask.
  onCancel: () => void;
}

// PromptBar is the chat entry point — a single textarea, Enter to send. There are
// no mode/scope/submit buttons: the assistant runs conversationally by default and
// the editing actions (summarize/rewrite/draft/propose) are offered as suggestion
// chips inside the Assistant window (AgentPanel). It is docked at the bottom of the
// panel, Cursor/Copilot-style.
export default function PromptBar({
  placeholder,
  contextLabel,
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

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      // Enter submits; Shift+Enter inserts a newline.
      e.preventDefault();
      submit();
    } else if (e.key === "Escape") {
      // Esc stops an in-flight stream and/or clears a primed action (→ plain ask).
      e.preventDefault();
      onCancel();
    }
  }

  function submit() {
    const trimmed = value.trim();
    if (!trimmed || disabled || inFlight) return;
    onSubmit(trimmed);
    setValue("");
  }

  return (
    <div className="promptbar" role="region" aria-label="Assistant prompt">
      <textarea
        ref={textareaRef}
        className="input promptbar-input"
        rows={2}
        placeholder={placeholder}
        value={value}
        disabled={disabled}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        aria-label="Prompt"
      />

      <div className="promptbar-foot">
        <span className="promptbar-context" title={contextLabel}>
          {contextLabel}
        </span>
        <span className="promptbar-hint" aria-live="polite">
          {status === "thinking" && (
            <>
              <Loader2 size={13} className="spinner" aria-hidden="true" /> Thinking… ·
              Esc to stop
            </>
          )}
          {status === "streaming" && (
            <>
              <Loader2 size={13} className="spinner" aria-hidden="true" /> Streaming… ·
              Esc to stop
            </>
          )}
          {status === "idle" && "Enter to send · Shift+Enter for a newline"}
        </span>
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

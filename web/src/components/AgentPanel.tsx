import type { ReactNode } from "react";
import { Loader2, PanelRightClose, Sparkles, X } from "lucide-react";

import AgentAnswer from "./AgentAnswer";
import type { AgentMode, PromptBarStatus } from "./PromptBar";
import "./AgentPanel.css";

// AgentPanel is the right-side Assistant column (the third column of
// .appshell-body), modelled on a Cursor/Copilot chat panel: a header, the answer
// conversation (or an empty state with quick-action suggestions), a row of
// suggestion chips, and the prompt docked at the very bottom. AppShell owns the
// session state and passes the prompt input + the suggestions in.
export interface AgentSuggestion {
  // key is a stable identity for the chip (and its accessible action).
  key: string;
  label: string;
  // run dispatches the action (Summarize runs immediately; Rewrite/Draft/Propose
  // prime the prompt for the next message). AppShell wires these to the agent.
  run: () => void;
}

export interface AgentPanelProps {
  open: boolean;
  onClose: () => void;
  mode: AgentMode;
  // scopeLabel is the human "This page" / "Whole workspace" meta for the header.
  scopeLabel: string;
  status: PromptBarStatus;
  // answer is the accumulated streamed Markdown; empty before the first prompt.
  answer: string;
  citations?: string[];
  error?: string | null;
  currentPath: string;
  // submitted is true once the user has sent at least one prompt this session
  // (drives the empty vs. loading/answer states).
  submitted: boolean;
  // suggestions are the quick actions surfaced inside the window (summarize,
  // rewrite, draft, propose) — gated by role/context upstream. Empty for readers
  // with nothing actionable.
  suggestions: AgentSuggestion[];
  // promptBar is the docked chat input rendered at the bottom of the panel.
  promptBar: ReactNode;
  // onClearScope, when set, means a specific file/selection is pinned as the
  // ask context — renders a clear (✕) control next to the scope meta so the user
  // can return to page/workspace scope without reloading.
  onClearScope?: () => void;
}

// MODE_LABEL is the human label for the header meta line.
const MODE_LABEL: Record<AgentMode, string> = {
  ask: "Ask",
  summarize: "Summarize",
  rewrite: "Rewrite",
  draft: "Draft",
  propose: "Propose an edit",
};

export default function AgentPanel({
  open,
  onClose,
  mode,
  scopeLabel,
  status,
  answer,
  citations,
  error,
  currentPath,
  submitted,
  suggestions,
  promptBar,
  onClearScope,
}: AgentPanelProps) {
  if (!open) return null;

  const streaming = status === "streaming";
  const thinking = status === "thinking";

  return (
    <aside className="agentpanel" aria-label="Assistant">
      <header className="agentpanel-header">
        <div className="agentpanel-heading">
          <h2 className="agentpanel-title">Assistant</h2>
          <span className="agentpanel-meta">
            {MODE_LABEL[mode]} · {scopeLabel}
            {onClearScope && (
              <button
                type="button"
                className="agentpanel-clear-scope"
                onClick={onClearScope}
                aria-label="Clear file context"
                title="Stop asking about this file"
              >
                <X size={12} aria-hidden="true" />
              </button>
            )}
          </span>
        </div>
        <button
          type="button"
          className="btn btn-ghost agentpanel-collapse"
          aria-label="Hide assistant"
          onClick={onClose}
        >
          <PanelRightClose size={16} aria-hidden="true" />
        </button>
      </header>

      <div className="agentpanel-body">
        {!submitted && (
          <div className="agentpanel-empty">
            <Sparkles size={24} className="agentpanel-empty-icon" aria-hidden="true" />
            <h3 className="agentpanel-empty-heading">Ask the assistant anything</h3>
            <p className="agentpanel-empty-body">
              Type below and press Enter. Ask about this page or the whole
              workspace — or use a suggestion to summarize, draft, or propose an
              edit you approve.
            </p>
          </div>
        )}

        {submitted && thinking && !answer && (
          <p className="agentpanel-loading" aria-live="polite">
            <Loader2 size={16} className="spinner" aria-hidden="true" />
            <span>Thinking…</span>
          </p>
        )}

        {submitted && (answer || streaming || error) && (
          <AgentAnswer
            answer={answer}
            streaming={streaming}
            citations={citations}
            error={error}
            currentPath={currentPath}
          />
        )}
      </div>

      {/* The dock: quick-action suggestions surfaced in the window (no mode picker
          on the prompt), then the chat input at the very bottom. */}
      {suggestions.length > 0 && (
        <div className="agentpanel-suggestions" aria-label="Suggested actions">
          {suggestions.map((s) => (
            <button
              key={s.key}
              type="button"
              className="btn btn-ghost agentpanel-suggestion"
              onClick={s.run}
            >
              {s.label}
            </button>
          ))}
        </div>
      )}

      {promptBar}
    </aside>
  );
}

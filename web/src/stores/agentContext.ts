// agentContext store (AGNT-02 selection Ask, AGNT-03/06 attachment Ask/summarize,
// AGNT-07 rewrite). This is the project's established answer to "two sibling
// components under the router need to share UI state": it carries the LIVE editor
// selection (verbatim text + raw length) and the chosen ATTACHMENT (id + name)
// from the route children (LivePreviewEditor / AttachmentCard) up to the AppShell
// agent session, which has no prop path across the router. It mirrors the
// agentPanel.ts / editorMode.ts zustand pattern with ONE deliberate difference:
//
//   *** This store is NOT persisted. ***
//
// The selection and the attachment are EPHEMERAL per-session context — a reload
// must start with no selection and no attachment (a stale selection persisted
// across reloads would silently scope the next prompt to text the user is no
// longer looking at). agentPanel/editorMode persist a UI *preference*; this
// carries *transient content*, so it deliberately omits the persist middleware.
import { create } from "zustand";

// AgentAttachmentContext is the minimal attachment identity the agent session
// needs: the opaque id (sent as attachment_id) and the original filename (shown
// in the PromptBar chip). Nothing else from AttachmentMeta crosses this channel.
export interface AgentAttachmentContext {
  id: string;
  name: string;
}

interface AgentContextState {
  // selection is the verbatim text of the current editor selection ("" when the
  // caret is collapsed or no selection). It is UNTRUSTED page content; AppShell
  // forwards it as the `selection` field and the server delimits it.
  selection: string;
  // selectionLength is the RAW character count of `selection` (not trimmed) used
  // by the "Selection (N chars)" chip copy and to drive rewriteAvailable.
  selectionLength: number;
  // attachment is the chosen attachment context (null when none is chosen).
  attachment: AgentAttachmentContext | null;
  // setSelection stores the verbatim text and derives the raw length. Passing ""
  // clears the selection (equivalent to clearSelection).
  setSelection: (text: string) => void;
  // clearSelection resets the selection to "" / 0 (e.g. caret collapse, unmount).
  clearSelection: () => void;
  // setAttachment sets the attachment context (e.g. "Ask about this file").
  setAttachment: (att: AgentAttachmentContext) => void;
  // clearAttachment resets the attachment context to null.
  clearAttachment: () => void;
}

export const useAgentContext = create<AgentContextState>((set) => ({
  selection: "",
  selectionLength: 0,
  attachment: null,
  setSelection: (text) => set({ selection: text, selectionLength: text.length }),
  clearSelection: () => set({ selection: "", selectionLength: 0 }),
  setAttachment: (att) => set({ attachment: att }),
  clearAttachment: () => set({ attachment: null }),
}));

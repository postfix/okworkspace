import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronsDownUp,
  ChevronsUpDown,
  FilePlus,
  FolderPlus,
  PanelLeft,
  PanelLeftClose,
  PanelRightOpen,
  Search,
  Settings,
  Sparkles,
  Trash2,
} from "lucide-react";

import {
  applyPatch,
  draft,
  health,
  logout,
  me,
  proposePatch,
  rewrite,
  subscribeAgentChat,
  summarizeAttachment,
  summarizePage,
  type AgentScope,
  type Me,
  type ProposePatchResult,
  type RepoHealth,
} from "../api/client";
import UserMenu from "../components/UserMenu";
import LeftTree, { type LeftTreeHandle } from "../components/LeftTree";
import RecentList from "../components/RecentList";
import ResizeHandle from "../components/ResizeHandle";
import { usePanelSizes } from "../stores/panelSizes";
import { useNavrail } from "../stores/navrail";
import { useTreeFold } from "../stores/treeFold";
import { useTreeFilter } from "../stores/treeFilter";
import SearchPalette from "../components/search/SearchPalette";
import PromptBar, {
  type AgentMode,
  type PromptBarStatus,
} from "../components/PromptBar";
import AgentPanel, { type AgentSuggestion } from "../components/AgentPanel";
import DiffReviewDialog from "../components/DiffReviewDialog";
import { useAgentPanel } from "../stores/agentPanel";
import { useAgentContext } from "../stores/agentContext";
import { useSearchStore } from "../store/searchStore";
import "./AppShell.css";

// PROMPT_PLACEHOLDER is the chat input hint per active action. Plain ask by
// default; a suggestion chip primes one of the others for the next message.
const PROMPT_PLACEHOLDER: Record<AgentMode, string> = {
  ask: "Ask anything…",
  summarize: "Summarize this page…",
  rewrite: "Describe the rewrite…",
  draft: "Describe the page to draft…",
  propose: "Describe the change to propose…",
};

// AppShell is the authenticated chrome (top bar + nav rail + main pane) and the
// AGENT SESSION OWNER: it holds the prompt/answer streaming state, drives the
// right-side AgentPanel + bottom PromptBar, and gates the propose/apply trust
// flow (DiffReviewDialog) to editors. When given children it renders them in the
// main pane (e.g. Admin, Profile); otherwise it shows the empty-state.
export default function AppShell({ children }: { children?: ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const { data } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const { data: repoHealth } = useQuery<RepoHealth>({
    queryKey: ["health"],
    queryFn: health,
  });

  const isAdmin = data?.role === "admin";
  // Editors (and admins) can create pages/folders AND reach the agent's
  // propose/apply path; readers cannot (RBAC mirror of the server gate).
  const canEdit = data?.role === "editor" || data?.role === "admin";
  const treeRef = useRef<LeftTreeHandle>(null);
  const setSearchOpen = useSearchStore((s) => s.setOpen);

  // ── Resizable column widths (persisted, drag-driven) ────────────────────────
  // The navrail + assistant widths live in a store and feed the --navrail-width /
  // --agentpanel-width CSS vars (set inline on .appshell below); the ResizeHandle
  // gutters nudge them on drag.
  const navWidth = usePanelSizes((s) => s.navWidth);
  const agentWidth = usePanelSizes((s) => s.agentWidth);
  const nudgeNav = usePanelSizes((s) => s.nudgeNav);
  const nudgeAgent = usePanelSizes((s) => s.nudgeAgent);

  // ── File-tree panel hide/show (Obsidian-style, persisted) ───────────────────
  const navOpen = useNavrail((s) => s.open);
  const toggleNav = useNavrail((s) => s.toggle);

  // Tree collapse-all / expand-all toggle (drives every FolderRow).
  const treeCollapsed = useTreeFold((s) => s.collapsed);
  const toggleFold = useTreeFold((s) => s.toggle);

  // File-tree filter query (lives in a store so its input sits in the fixed
  // navrail header while the tree scrolls below).
  const filterQuery = useTreeFilter((s) => s.query);
  const setFilterQuery = useTreeFilter((s) => s.setQuery);

  // ── Explicit viewport sizing (extension-proof fill) ─────────────────────────
  // position:fixed + inset:0 normally fills the window, but a browser extension
  // that wraps the page in a transform'd container re-anchors fixed elements to
  // that wrapper (which can be shrink-wrapped), leaving the shell short. Sizing
  // to window.innerWidth/Height directly is immune to that, and tracks resize.
  const [vp, setVp] = useState(() => ({
    w: typeof window === "undefined" ? 0 : window.innerWidth,
    h: typeof window === "undefined" ? 0 : window.innerHeight,
  }));
  useEffect(() => {
    const update = () => setVp({ w: window.innerWidth, h: window.innerHeight });
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  // ── Agent panel open/collapse (persisted) ──────────────────────────────────
  const panelOpen = useAgentPanel((s) => s.open);
  const setPanelOpen = useAgentPanel((s) => s.setOpen);
  const togglePanel = useAgentPanel((s) => s.toggle);

  // ── Current page scope (auto-detected from the route) ───────────────────────
  // /app/page/<path> and /app/edit/<path> carry the open page; everything else
  // (admin/profile/trash/empty) has no page → workspace scope only.
  const currentPath = pagePathFromLocation(location.pathname);
  const hasPage = currentPath !== "";

  // ── Live agent context (selection + attachment), published by route children ──
  // The CM6 editor publishes the current selection; an attachment card sets the
  // chosen attachment. Selector reads so the shell re-renders (and the PromptBar
  // chips / rewriteAvailable update) the moment either changes. This is ephemeral
  // session context — never persisted (agentContext.ts).
  const selection = useAgentContext((s) => s.selection);
  const selectionLength = useAgentContext((s) => s.selectionLength);
  const attachment = useAgentContext((s) => s.attachment);
  const clearAttachment = useAgentContext((s) => s.clearAttachment);
  const hasSelection = selectionLength > 0;

  // ── Agent prompt session state ──────────────────────────────────────────────
  const [mode, setMode] = useState<AgentMode>("ask");
  const [status, setStatus] = useState<PromptBarStatus>("idle");
  const [answer, setAnswer] = useState("");
  const [citations, setCitations] = useState<string[]>([]);
  const [submitted, setSubmitted] = useState(false);
  const [answerError, setAnswerError] = useState<string | null>(null);
  const [barError, setBarError] = useState<string | null>(null);
  // disabledReason is set when a submit hit the agent-off (503) or unreachable
  // (502) gate — the PromptBar then renders disabled with the explanation.
  const [disabledReason, setDisabledReason] = useState<string | null>(null);
  const [unreachable, setUnreachable] = useState(false);
  // The active stream's unsubscribe (AbortController.abort) — used to cancel.
  const unsubRef = useRef<(() => void) | null>(null);

  // Tear the stream down on unmount.
  useEffect(() => () => unsubRef.current?.(), []);

  // Effective scope is auto-detected (no toggle): a live selection (AGNT-02) wins,
  // then a chosen attachment (AGNT-03), then the open page, then the whole
  // workspace when nothing else is in scope. The server still derives the retrieval
  // ROLE from the session — this only chooses what context the prompt runs against.
  const effectiveScope: AgentScope = hasSelection
    ? "selection"
    : attachment
      ? "attachment"
      : hasPage
        ? "page"
        : "workspace";

  const scopeLabel =
    effectiveScope === "workspace"
      ? "Whole workspace"
      : effectiveScope === "selection"
        ? `Selection (${selectionLength} chars)`
        : effectiveScope === "attachment"
          ? (attachment?.name ?? "Attachment")
          : "This page";

  const cancelStream = useCallback(() => {
    unsubRef.current?.();
    unsubRef.current = null;
    setStatus("idle");
  }, []);

  // answerRef mirrors answer so a stable closure can read the latest value without
  // re-subscribing each render.
  const answerRef = useRef("");
  useEffect(() => {
    answerRef.current = answer;
  }, [answer]);

  // ── Propose / apply trust flow (editor-gated) ───────────────────────────────
  // Declared before handleSubmit so the Propose mode branch can drive it.
  const [proposal, setProposal] = useState<ProposePatchResult | null>(null);
  const [proposeError, setProposeError] = useState<string | null>(null);
  const [stale, setStale] = useState(false);

  // ── Rewrite proposal (AGNT-07) ──────────────────────────────────────────────
  // A rewrite is a PROPOSAL: rewrite() returns the rewritten span, which we route
  // through the SAME DiffReviewDialog (old = the original selection, new = the
  // rewrite). It NEVER auto-applies — Approve replaces the selection span in the
  // page body and saves through the existing applyPatch path; Reject discards.
  // We hold the original selection so apply can locate and replace exactly it.
  const [rewriteProposal, setRewriteProposal] = useState<{
    selection: string;
    rewritten: string;
  } | null>(null);

  const proposeMutation = useMutation({
    mutationFn: (instruction: string) => proposePatch(currentPath, instruction),
    onSuccess: (res) => {
      setStale(false);
      setProposal(res);
    },
    onError: (err: Error) => {
      setProposeError(
        err.message || "The assistant couldn't propose a change. Try again.",
      );
    },
  });

  const applyMutation = useMutation({
    mutationFn: (p: ProposePatchResult) =>
      applyPatch({
        page_path: p.page_path,
        new_body: p.new_body,
        // The frontmatter is preserved by the proposal (body-only); we re-read the
        // page's frontmatter from the cache so apply re-assembles the exact source.
        // The proposal's new_body is the body only (CR-01).
        frontmatter: frontmatterFromCache(queryClient, p.page_path),
        base_revision: p.base_revision,
      }),
    onSuccess: () => {
      setProposal(null);
      setStale(false);
      // The saved change is now the latest version — refresh the page view.
      queryClient.invalidateQueries({ queryKey: ["page", currentPath] });
    },
    onError: (err: Error & { status?: number }) => {
      if (err.status === 409) {
        // The page moved while the user reviewed — block, never overwrite.
        setStale(true);
      } else {
        setProposeError(
          err.message || "We couldn't apply that change just now. Try again.",
        );
      }
    },
  });

  // ── Rewrite: request the rewrite, then apply it on explicit Approve ──────────
  // rewriteMutation only PROPOSES (it sets the diff state); it never writes. The
  // returned span is shown in the DiffReviewDialog. Apply happens through
  // applyRewriteMutation below, gated on the user's Approve.
  const rewriteMutation = useMutation({
    mutationFn: ({ selection: sel, instruction }: { selection: string; instruction: string }) =>
      rewrite(sel, instruction),
    onSuccess: (rewritten, { selection: sel }) => {
      setStatus("idle");
      setStale(false);
      setRewriteProposal({ selection: sel, rewritten });
    },
    onError: (err: Error) => {
      setStatus("idle");
      // Fail-closed: surface the server's message, never a silent wrong action.
      setBarError(
        err.message || "The assistant couldn't rewrite that. Try again.",
      );
    },
  });

  // applyRewriteMutation applies an APPROVED rewrite: it replaces the original
  // selection span in the page body with the rewritten text and saves through the
  // existing apply-patch path (editor + CSRF, optimistic concurrency). It re-reads
  // the page body/frontmatter/revision from the cache so the write re-assembles the
  // exact source — no new write endpoint. A stale revision 409s into the dialog's
  // stale state, never overwriting a concurrent edit.
  const applyRewriteMutation = useMutation({
    mutationFn: (rp: { selection: string; rewritten: string }) => {
      const cached = queryClient.getQueryData<{
        body?: string;
        frontmatter?: string;
        revision?: string;
      }>(["page", currentPath]);
      const body = cached?.body ?? "";
      const idx = body.indexOf(rp.selection);
      if (idx === -1) {
        // The selection is no longer in the page body (it changed under us) —
        // treat as stale rather than writing a guessed span.
        return Promise.reject(
          Object.assign(new Error("selection no longer present"), { status: 409 }),
        );
      }
      const newBody =
        body.slice(0, idx) + rp.rewritten + body.slice(idx + rp.selection.length);
      return applyPatch({
        page_path: currentPath,
        new_body: newBody,
        frontmatter: cached?.frontmatter ?? "",
        base_revision: cached?.revision ?? "",
      });
    },
    onSuccess: () => {
      setRewriteProposal(null);
      setStale(false);
      queryClient.invalidateQueries({ queryKey: ["page", currentPath] });
    },
    onError: (err: Error & { status?: number }) => {
      if (err.status === 409) {
        setStale(true);
      } else {
        setProposeError(
          err.message || "We couldn't apply that rewrite just now. Try again.",
        );
      }
    },
  });

  // resetSubmitState clears per-request UI state (kept-prompt contract on error)
  // and opens the panel so the result is visible. Shared by every mode.
  const resetSubmitState = useCallback(() => {
    cancelStream();
    setSubmitted(true);
    setAnswer("");
    setCitations([]);
    setAnswerError(null);
    setBarError(null);
    setDisabledReason(null);
    setUnreachable(false);
    setStatus("thinking");
    if (!panelOpen) setPanelOpen(true);
  }, [cancelStream, panelOpen, setPanelOpen]);

  // runAsk streams the token-by-token Ask answer (the default mode).
  const runAsk = useCallback(
    (prompt: string) => {
      const unsub = subscribeAgentChat(
        {
          prompt,
          scope: effectiveScope,
          // Page path travels for page/selection scope (the selection lives in the
          // open page); selection text scopes the answer (AGNT-02); attachment_id
          // scopes it to the chosen file (AGNT-03). The server still derives the
          // retrieval role from the session, never from these fields.
          page_path:
            effectiveScope === "page" || effectiveScope === "selection"
              ? hasPage
                ? currentPath
                : undefined
              : undefined,
          selection: effectiveScope === "selection" ? selection : undefined,
          attachment_id:
            effectiveScope === "attachment" ? attachment?.id : undefined,
        },
        {
          onToken: (delta) => {
            setStatus("streaming");
            setAnswer((a) => a + delta);
          },
          onCitation: (paths) => setCitations(paths),
          onDone: () => {
            setStatus("idle");
            unsubRef.current = null;
          },
          onError: (message, { disabled, unreachable: isUnreachable }) => {
            setStatus("idle");
            unsubRef.current = null;
            if (disabled || isUnreachable) {
              setDisabledReason(message);
              setUnreachable(isUnreachable);
            } else if (answerRef.current) {
              setAnswerError(message);
            } else {
              setBarError(message);
            }
          },
        },
      );
      unsubRef.current = unsub;
    },
    [effectiveScope, hasPage, currentPath, selection, attachment],
  );

  // runSingleShot runs an AWAITED mode (Summarize/Draft) and renders the whole
  // result into the AgentPanel answer. Unlike Ask there is no token stream — the
  // status goes thinking → idle and the answer is set once. A failure routes to
  // the bar (no partial answer to preserve). Fail-closed: a thrown server error
  // carries its message; we never silently swallow a wrong action.
  const runSingleShot = useCallback((run: () => Promise<string>) => {
    run()
      .then((result) => {
        setStatus("idle");
        setAnswer(result);
      })
      .catch((err: Error) => {
        setStatus("idle");
        setBarError(
          err.message || "The assistant couldn't finish that. Try again.",
        );
      });
  }, []);

  // runAgent dispatches one action (WR-01): Ask streams; Summarize and Draft are
  // awaited single-shot calls; Rewrite/Propose open the diff flow. The action is
  // explicit (not read from state) so a suggestion chip can run the right thing
  // without racing a setMode. A mode never silently runs the wrong action.
  const runAgent = useCallback(
    (prompt: string, action: AgentMode) => {
      resetSubmitState();
      switch (action) {
        case "summarize":
          // An attachment in context summarizes the file (AGNT-06); otherwise an
          // open page summarizes the page (AGNT-05, unchanged).
          if (attachment) {
            runSingleShot(() => summarizeAttachment(attachment.id));
            return;
          }
          if (!hasPage) {
            // Workspace summarize is not an MVP endpoint; require an open page.
            setStatus("idle");
            setBarError("Open a page to summarize.");
            return;
          }
          runSingleShot(() => summarizePage(currentPath));
          return;
        case "draft":
          runSingleShot(() => draft(prompt));
          return;
        case "rewrite":
          // Rewrite acts on the captured editor selection (AGNT-07). Defend here
          // even though the PromptBar disables Rewrite without a selection — never
          // run a wrong action. The result ALWAYS routes through DiffReviewDialog
          // (it never auto-applies).
          if (!hasSelection) {
            setStatus("idle");
            setBarError("Select some text in the editor to rewrite it.");
            return;
          }
          rewriteMutation.mutate({ selection, instruction: prompt });
          return;
        case "propose":
          // Propose routes through the editor-gated diff flow, not the answer pane.
          setStatus("idle");
          if (!canEdit || !hasPage) {
            setBarError("Open a page you can edit to propose a change.");
            return;
          }
          setProposeError(null);
          setStale(false);
          proposeMutation.mutate(prompt);
          return;
        case "ask":
        default:
          runAsk(prompt);
          return;
      }
    },
    [
      resetSubmitState,
      hasPage,
      currentPath,
      canEdit,
      hasSelection,
      selection,
      attachment,
      runSingleShot,
      runAsk,
      proposeMutation,
      rewriteMutation,
    ],
  );

  // handleSubmit runs the chat input. It dispatches the active action — plain ask
  // by default, or whatever a suggestion chip primed (rewrite/draft/propose) for
  // this one message — then returns the prompt to conversational ask.
  const handleSubmit = useCallback(
    (prompt: string) => {
      runAgent(prompt, mode);
      if (mode !== "ask") setMode("ask");
    },
    [runAgent, mode],
  );

  // Propose from a completed Ask/Summarize answer: re-use the answer as the
  // instruction so the assistant proposes a concrete page change.
  const proposeFromAnswer = useCallback(() => {
    if (!canEdit || !hasPage) return;
    setProposeError(null);
    setStale(false);
    proposeMutation.mutate(answerRef.current || "Apply the change discussed above.");
  }, [canEdit, hasPage, proposeMutation]);

  // The chat placeholder reflects a primed action (default conversational ask).
  const promptPlaceholder = PROMPT_PLACEHOLDER[mode];

  // Quick-action suggestions surfaced INSIDE the Assistant window — the prompt
  // itself has no mode buttons. Summarize runs immediately; rewrite/draft/propose
  // prime the prompt for the next message. Gated by role + context so readers never
  // see a write action and Rewrite only appears with a live selection.
  const suggestions: AgentSuggestion[] = [];
  if (hasPage) {
    suggestions.push({
      key: "summarize",
      label: attachment ? `Summarize ${attachment.name}` : "Summarize this page",
      run: () => runAgent("", "summarize"),
    });
  }
  if (canEdit && hasSelection) {
    suggestions.push({
      key: "rewrite",
      label: "Rewrite selection",
      run: () => setMode("rewrite"),
    });
  }
  if (canEdit) {
    suggestions.push({ key: "draft", label: "Draft a page", run: () => setMode("draft") });
  }
  if (canEdit && hasPage) {
    suggestions.push({
      key: "propose",
      label: "Propose an edit",
      run: () => setMode("propose"),
    });
  }
  // Once an answer has streamed, offer to turn it into a reviewable patch.
  if (submitted && status === "idle" && answer && !answerError && canEdit && hasPage) {
    suggestions.push({
      key: "propose-answer",
      label: "Propose this as an edit",
      run: proposeFromAnswer,
    });
  }

  // Global ⌘K / Ctrl K opens the search palette from anywhere.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && (e.key === "k" || e.key === "K")) {
        e.preventDefault();
        setSearchOpen(true);
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [setSearchOpen]);

  async function handleLogout() {
    await logout();
    queryClient.removeQueries({ queryKey: ["me"] });
    navigate("/login", { replace: true });
  }

  return (
    <div
      className="appshell"
      style={
        {
          "--navrail-width": `${navWidth}px`,
          "--agentpanel-width": `${agentWidth}px`,
          width: vp.w ? `${vp.w}px` : "100vw",
          height: vp.h ? `${vp.h}px` : "100vh",
        } as CSSProperties
      }
    >
      <SearchPalette />
      <header className="topbar">
        {/* Left: the file-tree toggle. Search now lives in the navrail footer. */}
        <div className="topbar-left">
          <button
            type="button"
            className="btn btn-ghost icon-btn topbar-nav-toggle"
            onClick={toggleNav}
            aria-label={navOpen ? "Hide file tree" : "Show file tree"}
            aria-pressed={navOpen}
            title={navOpen ? "Hide file tree" : "Show file tree"}
          >
            {navOpen ? (
              <PanelLeftClose size={16} aria-hidden="true" />
            ) : (
              <PanelLeft size={16} aria-hidden="true" />
            )}
          </button>
        </div>
        <div className="topbar-right">
          {repoHealth?.ok && (
            <span className="repo-health" title={repoHealth.detail}>
              <span className="repo-health-dot" aria-hidden="true" />
              <span>Storage healthy</span>
            </span>
          )}
          <button
            type="button"
            className={`btn btn-ghost topbar-agent${status === "streaming" ? " is-active" : ""}`}
            onClick={togglePanel}
            aria-label={panelOpen ? "Hide assistant" : "Show assistant"}
            aria-pressed={panelOpen}
          >
            {panelOpen ? (
              <PanelRightOpen size={16} aria-hidden="true" />
            ) : (
              <Sparkles size={16} aria-hidden="true" />
            )}
            <span>Assistant</span>
          </button>
          <UserMenu
            displayName={data?.display_name ?? ""}
            onProfile={() => navigate("/profile")}
            onLogout={handleLogout}
          />
        </div>
      </header>

      <div className="appshell-body">
        {navOpen && (
          <>
        <nav className="navrail" aria-label="Workspace navigation">
          {/* Fixed header — toolbar + filter never scroll. */}
          <div className="navrail-header">
            <div className="navrail-toolbar">
              {canEdit && (
                <button
                  type="button"
                  className="navrail-tool"
                  onClick={() => treeRef.current?.openCreatePage()}
                  aria-label="New page"
                  title="New page"
                >
                  <FilePlus size={16} aria-hidden="true" />
                </button>
              )}
              {canEdit && (
                <button
                  type="button"
                  className="navrail-tool"
                  onClick={() => treeRef.current?.openCreateFolder()}
                  aria-label="New folder"
                  title="New folder"
                >
                  <FolderPlus size={16} aria-hidden="true" />
                </button>
              )}
              <button
                type="button"
                className="navrail-tool"
                onClick={toggleFold}
                aria-label={treeCollapsed ? "Expand all folders" : "Collapse all folders"}
                title={treeCollapsed ? "Expand all" : "Collapse all"}
              >
                {treeCollapsed ? (
                  <ChevronsUpDown size={16} aria-hidden="true" />
                ) : (
                  <ChevronsDownUp size={16} aria-hidden="true" />
                )}
              </button>
            </div>
            <div className="navrail-filter">
              <Search size={15} aria-hidden="true" className="navrail-filter-icon" />
              <input
                type="text"
                className="navrail-filter-input"
                placeholder="Filter files…"
                value={filterQuery}
                onChange={(e) => setFilterQuery(e.target.value)}
                aria-label="Filter files"
              />
            </div>
          </div>

          {/* Scrolling body — the file tree (with Trash at its bottom) + recent. */}
          <div className="navrail-body">
            <LeftTree ref={treeRef} />
            <button
              type="button"
              className="navrow navrail-trash-row"
              onClick={() => navigate("/trash")}
            >
              <Trash2 size={16} aria-hidden="true" className="tree-icon" />
              <span className="tree-label">Trash</span>
            </button>
            <RecentList />
          </div>

          {/* Fixed footer — Search (left) + Settings/Admin (right). */}
          <div className="navrail-footer">
            <button
              type="button"
              className="btn btn-ghost navrail-foot-search"
              onClick={() => setSearchOpen(true)}
              aria-label="Search workspace (⌘K)"
            >
              <Search size={16} aria-hidden="true" />
              <span>Search</span>
              <kbd className="keycap">⌘K</kbd>
            </button>
            {isAdmin && (
              <button
                type="button"
                className="btn btn-ghost icon-btn"
                onClick={() => navigate("/admin")}
                aria-label="Admin settings"
                title="Admin settings"
              >
                <Settings size={16} aria-hidden="true" />
              </button>
            )}
          </div>
        </nav>

        <ResizeHandle ariaLabel="Resize sidebar" onResize={nudgeNav} />
          </>
        )}

        <main className="mainpane">
          {repoHealth && !repoHealth.ok && !repoHealth.diverged && (
            <div className="banner banner-warning" role="alert">
              Storage is reporting a problem. Your work may not be saving —
              contact your administrator.
            </div>
          )}
          {repoHealth?.diverged && (
            <div className="banner banner-warning" role="alert">
              The remote repository has diverged. Automatic sync was paused to
              protect your data — contact your administrator.
            </div>
          )}
          {repoHealth?.self_healed && !repoHealth.diverged && (
            <div className="banner banner-warning" role="status">
              Recovered from an interrupted save. Everything looks fine.
            </div>
          )}

          {children ? (
            children
          ) : (
            <div className="empty-state">
              <h1 className="empty-state-heading">Pick a page to get started</h1>
              <p className="empty-state-body">
                Choose a page from the left, or create a new one.
              </p>
            </div>
          )}
        </main>

        {panelOpen && (
          <ResizeHandle
            ariaLabel="Resize assistant"
            onResize={(dx) => nudgeAgent(-dx)}
          />
        )}

        <AgentPanel
          open={panelOpen}
          onClose={() => setPanelOpen(false)}
          mode={mode}
          scopeLabel={scopeLabel}
          status={status}
          answer={answer}
          citations={citations}
          error={answerError}
          currentPath={currentPath}
          submitted={submitted}
          suggestions={suggestions}
          onClearScope={attachment ? clearAttachment : undefined}
          promptBar={
            <PromptBar
              placeholder={promptPlaceholder}
              contextLabel={scopeLabel}
              status={status}
              disabledReason={disabledReason}
              unreachable={unreachable}
              error={barError}
              onSubmit={handleSubmit}
              onCancel={() => {
                cancelStream();
                setMode("ask");
              }}
            />
          }
        />
      </div>

      {/* The trust gate (ONE dialog, two drivers). Opens with a real diff once a
          propose patch OR a rewrite returns; a 409 flips it into the stale state
          (Approve removed). A rewrite NEVER auto-applies — Approve replaces the
          selection span and saves through the same apply path. Editor-gated. */}
      <DiffReviewDialog
        open={proposal !== null || rewriteProposal !== null}
        title={rewriteProposal ? "Review the rewrite" : "Review this change"}
        oldText={rewriteProposal ? rewriteProposal.selection : proposal?.old_body ?? ""}
        newText={rewriteProposal ? rewriteProposal.rewritten : proposal?.new_body ?? ""}
        stale={stale}
        busy={applyMutation.isPending || applyRewriteMutation.isPending}
        onApprove={() => {
          if (rewriteProposal) {
            applyRewriteMutation.mutate(rewriteProposal);
          } else if (proposal) {
            applyMutation.mutate(proposal);
          }
        }}
        onReject={() => {
          setProposal(null);
          setRewriteProposal(null);
          setStale(false);
        }}
        onRerun={() => {
          setStale(false);
          if (rewriteProposal) {
            // Re-run the rewrite against the (possibly updated) live selection.
            const sel = rewriteProposal.selection;
            setRewriteProposal(null);
            if (hasSelection) {
              rewriteMutation.mutate({ selection, instruction: "Rewrite the selection." });
            } else {
              rewriteMutation.mutate({ selection: sel, instruction: "Rewrite the selection." });
            }
          } else {
            setProposal(null);
            proposeFromAnswer();
          }
        }}
      />

      {proposeError && (
        <div className="banner banner-warning agent-propose-error" role="alert">
          {proposeError}
        </div>
      )}
    </div>
  );
}

// pagePathFromLocation extracts the workspace-relative page path from the route.
// /app/page/<path> and /app/edit/<path> carry an open page; any other route
// (admin, profile, trash, the bare /app empty state) has no page → "".
function pagePathFromLocation(pathname: string): string {
  const m = pathname.match(/^\/app\/(?:page|edit)\/(.+)$/);
  return m ? decodeURIComponent(m[1]) : "";
}

// frontmatterFromCache reads the cached Page's frontmatter region so applyPatch
// re-assembles the exact source bytes (the proposal returns the body only). If
// the page isn't cached (edge case), an empty frontmatter is sent and the server
// re-validates — it never fabricates frontmatter.
function frontmatterFromCache(
  queryClient: ReturnType<typeof useQueryClient>,
  pagePath: string,
): string {
  const cached = queryClient.getQueryData<{ frontmatter?: string }>([
    "page",
    pagePath,
  ]);
  return cached?.frontmatter ?? "";
}

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  FilePlus,
  FolderPlus,
  PanelRightOpen,
  Search,
  Shield,
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
  subscribeAgentChat,
  summarizePage,
  type AgentScope,
  type Me,
  type ProposePatchResult,
  type RepoHealth,
} from "../api/client";
import UserMenu from "../components/UserMenu";
import LeftTree, { type LeftTreeHandle } from "../components/LeftTree";
import RecentList from "../components/RecentList";
import SearchPalette from "../components/search/SearchPalette";
import PromptBar, {
  type AgentMode,
  type PromptBarStatus,
} from "../components/PromptBar";
import AgentPanel from "../components/AgentPanel";
import DiffReviewDialog from "../components/DiffReviewDialog";
import { useAgentPanel } from "../stores/agentPanel";
import { useSearchStore } from "../store/searchStore";
import "./AppShell.css";

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

  // ── Agent panel open/collapse (persisted) ──────────────────────────────────
  const panelOpen = useAgentPanel((s) => s.open);
  const setPanelOpen = useAgentPanel((s) => s.setOpen);
  const togglePanel = useAgentPanel((s) => s.toggle);

  // ── Current page scope (auto-detected from the route) ───────────────────────
  // /app/page/<path> and /app/edit/<path> carry the open page; everything else
  // (admin/profile/trash/empty) has no page → workspace scope only.
  const currentPath = pagePathFromLocation(location.pathname);
  const hasPage = currentPath !== "";

  // ── Agent prompt session state ──────────────────────────────────────────────
  const [mode, setMode] = useState<AgentMode>("ask");
  const [workspace, setWorkspace] = useState(false);
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

  // Reaching a workspace scope (toggle on, or no page open) means Propose/Rewrite
  // page-scoped modes don't apply; coerce the mode back to Ask if it became
  // unavailable so the bar never shows a disabled-mode submit.
  const effectiveScope: AgentScope = workspace || !hasPage ? "workspace" : "page";

  const scopeLabel = effectiveScope === "workspace" ? "Whole workspace" : "This page";

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
          page_path: hasPage ? currentPath : undefined,
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
    [effectiveScope, hasPage, currentPath],
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

  // handleSubmit dispatches on the selected mode (WR-01): Ask streams; Summarize
  // and Draft are awaited single-shot calls; Propose opens the diff flow. Rewrite
  // needs a captured editor selection (not yet plumbed), so the PromptBar disables
  // it — it never reaches here. A mode never silently runs the wrong action.
  const handleSubmit = useCallback(
    (prompt: string) => {
      resetSubmitState();
      switch (mode) {
        case "summarize":
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
          // Rewrite needs a captured editor selection (not yet plumbed). The bar
          // disables it; if it is somehow active, refuse rather than run an Ask.
          setStatus("idle");
          setBarError("Select some text in the editor to rewrite it.");
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
      mode,
      hasPage,
      currentPath,
      canEdit,
      runSingleShot,
      runAsk,
      proposeMutation,
    ],
  );

  // Propose from a completed Ask/Summarize answer: re-use the answer as the
  // instruction so the assistant proposes a concrete page change.
  const proposeFromAnswer = useCallback(() => {
    if (!canEdit || !hasPage) return;
    setProposeError(null);
    setStale(false);
    proposeMutation.mutate(answerRef.current || "Apply the change discussed above.");
  }, [canEdit, hasPage, proposeMutation]);

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
    <div className="appshell">
      <SearchPalette />
      <header className="topbar">
        <button
          type="button"
          className="topbar-wordmark topbar-wordmark-btn"
          onClick={() => navigate("/app")}
        >
          OKF Workspace
        </button>
        <div className="topbar-right">
          {repoHealth?.ok && (
            <span className="repo-health" title={repoHealth.detail}>
              <span className="repo-health-dot" aria-hidden="true" />
              <span>Storage healthy</span>
            </span>
          )}
          <button
            type="button"
            className="btn btn-ghost topbar-search"
            onClick={() => setSearchOpen(true)}
            aria-label="Search workspace (⌘K)"
          >
            <Search size={16} aria-hidden="true" />
            <span>Search</span>
            <kbd className="keycap">⌘K</kbd>
          </button>
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
        <nav className="navrail" aria-label="Workspace navigation">
          {canEdit && (
            <div className="navrail-actions">
              <button
                type="button"
                className="navrow navrow-action"
                onClick={() => treeRef.current?.openCreatePage()}
              >
                <FilePlus size={16} aria-hidden="true" />
                <span>New page</span>
              </button>
              <button
                type="button"
                className="navrow navrow-action"
                onClick={() => treeRef.current?.openCreateFolder()}
              >
                <FolderPlus size={16} aria-hidden="true" />
                <span>New folder</span>
              </button>
            </div>
          )}

          <LeftTree ref={treeRef} />
          <RecentList />

          <div className="navrail-trash">
            <button
              type="button"
              className="navrow navrow-action"
              onClick={() => navigate("/trash")}
            >
              <Trash2 size={16} aria-hidden="true" />
              <span>Trash</span>
            </button>
          </div>

          {isAdmin && (
            <div className="navrail-admin">
              <button
                type="button"
                className="navrow navrow-action"
                onClick={() => navigate("/admin")}
              >
                <Shield size={16} aria-hidden="true" />
                <span>Admin</span>
              </button>
            </div>
          )}
        </nav>

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
          canPropose={canEdit && hasPage}
          onProposeFromAnswer={proposeFromAnswer}
        />
      </div>

      <PromptBar
        mode={mode}
        onModeChange={setMode}
        canEdit={canEdit}
        hasPage={hasPage}
        pageTitle={hasPage ? "This page" : undefined}
        workspace={workspace}
        onWorkspaceToggle={() => setWorkspace((w) => !w)}
        status={status}
        disabledReason={disabledReason}
        unreachable={unreachable}
        error={barError}
        onSubmit={handleSubmit}
        onCancel={cancelStream}
      />

      {/* The trust gate. Opens with a real diff once a proposal returns; a 409
          flips it into the stale state (Approve removed). Editor-gated above. */}
      <DiffReviewDialog
        open={proposal !== null}
        title="Review this change"
        oldText={proposal?.old_body ?? ""}
        newText={proposal?.new_body ?? ""}
        stale={stale}
        busy={applyMutation.isPending}
        onApprove={() => proposal && applyMutation.mutate(proposal)}
        onReject={() => {
          setProposal(null);
          setStale(false);
        }}
        onRerun={() => {
          setProposal(null);
          setStale(false);
          proposeFromAnswer();
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

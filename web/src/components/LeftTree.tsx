import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useState,
  type DragEvent,
  type MouseEvent,
} from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, ChevronDown, FileText, Folder } from "lucide-react";

import { getTree, me, type Me, type TreeNode } from "../api/client";
import {
  PAGE_DRAG_TYPE,
  FOLDER_DRAG_TYPE,
  dropAllowed,
  useTreeMove,
  type DragKind,
} from "./hooks/useTreeMutations";
import TreeContextMenu, { type TreeContextMenuItem } from "./TreeContextMenu";
import CreatePageModal from "./CreatePageModal";
import CreateFolderModal from "./CreateFolderModal";
import RenameModal from "./RenameModal";
import MoveDialog from "./MoveDialog";
import DeleteConfirmDialog from "./DeleteConfirmDialog";
import DeleteFolderDialog from "./DeleteFolderDialog";
import HistoryPanel from "./HistoryPanel";
import "./LeftTree.css";

// LeftTreeHandle lets the parent (AppShell) drive the root-scoped create modals
// from its top "New page" / "New folder" buttons while the tree itself drives
// folder-scoped create via the right-click context menu.
export interface LeftTreeHandle {
  openCreatePage: () => void;
  openCreateFolder: () => void;
}

// MenuTarget identifies which node (or root) was right-clicked — the input to
// menuItems(). Folders now carry the full 5-action menu (create + rename/move/
// delete); pages keep their rename/move/history/delete set.
type MenuTarget =
  | { kind: "folder"; path: string; title: string }
  | { kind: "page"; path: string; title: string }
  | { kind: "root" };

interface MenuState {
  x: number;
  y: number;
  target: MenuTarget;
}

// CreateState records which create modal is open and the folder/parent it targets
// ("" = root).
type CreateState =
  | { kind: "page"; folder: string; folderName: string }
  | { kind: "folder"; parent: string }
  | null;

// PageActionState records which page-action modal is open and the page it targets.
// These reuse the same modals PageActionMenu wires in the page header —
// parameterized by the right-clicked node rather than the currently-open page.
type PageActionState =
  | { kind: "rename" | "move" | "delete" | "history"; path: string; title: string }
  | null;

// FolderActionState records which folder-action dialog is open and the folder it
// targets. Rename/Move reuse the kind="folder" branch of the shared dialogs
// (built in 07-03); Delete opens the net-new DeleteFolderDialog.
type FolderActionState =
  | { kind: "rename" | "move" | "delete"; path: string; title: string }
  | null;

// ROOT_FOLDER_NAME is the human label shown when creating at the top level.
const ROOT_FOLDER_NAME = "your workspace";

// LeftTree renders the live navigation tree (NAV-01) with Obsidian-style
// affordances: right-click a folder to create inside it, right-click a page to
// rename/move/delete/view history, right-click empty space to create at root,
// and drag a page onto a folder (or the root drop zone) to move it. Folders
// expand/collapse (NAV-02); the active page row is highlighted (NAV-04). Page
// mutations reuse the existing modals/APIs; the move mutation invalidates the
// ["tree"]/["page"] queries on success (the optimistic-update upgrade is Plan 04).
const LeftTree = forwardRef<LeftTreeHandle>(function LeftTree(_props, ref) {
  const { data, isLoading, isError } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
  });
  const { data: user } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  // Editors and admins may mutate; readers get a read-only menu and no DnD (the
  // client mirror of the authoritative server gate PageActionMenu enforces).
  const canEdit = user?.role === "editor" || user?.role === "admin";

  // Coalesce null/undefined to []. The endpoint can serialize an empty repo's
  // tree to JSON `null`; a `= []` default only guards undefined, so without this
  // nodes.map would throw on null and white-screen the app (UAT blocker).
  const nodes = data ?? [];

  const [menu, setMenu] = useState<MenuState | null>(null);
  const [create, setCreate] = useState<CreateState>(null);
  const [pageAction, setPageAction] = useState<PageActionState>(null);
  const [folderAction, setFolderAction] = useState<FolderActionState>(null);
  // moveError surfaces the optimistic-rollback copy when a DnD move fails and the
  // tree snaps back (UI-SPEC). It's an ephemeral banner above the tree.
  const [moveError, setMoveError] = useState<string | null>(null);

  // Expose root-scoped create to the parent's top buttons.
  useImperativeHandle(
    ref,
    () => ({
      openCreatePage: () =>
        setCreate({ kind: "page", folder: "", folderName: ROOT_FOLDER_NAME }),
      openCreateFolder: () => setCreate({ kind: "folder", parent: "" }),
    }),
    [],
  );

  // The optimistic move mutation (page + folder). onMutate applies applyMove to
  // the ["tree"] cache immediately so a drop feels instant; onError rolls the
  // snapshot back and surfaces the rollback copy; onSettled invalidates to
  // reconcile (the commit-wait correctness backstop).
  const moveMut = useTreeMove(setMoveError);

  // dropNode dispatches a validated drop. dropAllowed already rejected self/
  // descendant/same-parent during dragover, but we re-check here so a stray drop
  // event can never round-trip a no-op move (TREE-06 belt-and-suspenders).
  const dropNode = useCallback(
    (kind: DragKind, srcPath: string, destFolder: string) => {
      if (!dropAllowed(kind, srcPath, destFolder)) return;
      setMoveError(null);
      moveMut.mutate({ src: srcPath, dest: destFolder, kind });
    },
    [moveMut],
  );

  const openMenu = useCallback((e: MouseEvent, target: MenuTarget) => {
    e.preventDefault();
    e.stopPropagation();
    setMenu({ x: e.clientX, y: e.clientY, target });
  }, []);

  // menuItems builds the context-menu items for the right-clicked target:
  //   folder → create-only (folder rename/move/delete arrive in Plan 04)
  //   page (editor) → Rename / Move / Version history / Delete
  //   page (reader) → Version history only (RBAC)
  //   root → create at top level
  function menuItems(target: MenuTarget): TreeContextMenuItem[] {
    switch (target.kind) {
      case "folder":
        return canEdit ? folderMenuItems(target.path, target.title) : [];
      case "page":
        return pageMenuItems(target.path, target.title);
      case "root":
        return canEdit ? rootMenuItems() : [];
    }
  }

  function folderMenuItems(path: string, title: string): TreeContextMenuItem[] {
    return [
      {
        label: "New page here",
        onSelect: () =>
          setCreate({ kind: "page", folder: path, folderName: title }),
      },
      {
        label: "New folder here",
        onSelect: () => setCreate({ kind: "folder", parent: path }),
      },
      {
        label: "Rename",
        onSelect: () => setFolderAction({ kind: "rename", path, title }),
      },
      {
        label: "Move",
        onSelect: () => setFolderAction({ kind: "move", path, title }),
      },
      {
        label: "Delete",
        danger: true,
        onSelect: () => setFolderAction({ kind: "delete", path, title }),
      },
    ];
  }

  function pageMenuItems(path: string, title: string): TreeContextMenuItem[] {
    const items: TreeContextMenuItem[] = [];
    if (canEdit) {
      items.push({
        label: "Rename",
        onSelect: () => setPageAction({ kind: "rename", path, title }),
      });
      items.push({
        label: "Move",
        onSelect: () => setPageAction({ kind: "move", path, title }),
      });
    }
    items.push({
      label: "Version history",
      onSelect: () => setPageAction({ kind: "history", path, title }),
    });
    if (canEdit) {
      items.push({
        label: "Delete",
        danger: true,
        onSelect: () => setPageAction({ kind: "delete", path, title }),
      });
    }
    return items;
  }

  function rootMenuItems(): TreeContextMenuItem[] {
    return [
      {
        label: "New page",
        onSelect: () =>
          setCreate({ kind: "page", folder: "", folderName: ROOT_FOLDER_NAME }),
      },
      {
        label: "New folder",
        onSelect: () => setCreate({ kind: "folder", parent: "" }),
      },
    ];
  }

  if (isLoading) {
    return (
      <div className="lefttree-status" role="status">
        Loading…
      </div>
    );
  }
  if (isError) {
    return (
      <div className="lefttree-status" role="alert">
        Couldn't load your pages — try again.
      </div>
    );
  }

  return (
    <div className="lefttree">
      {moveError && (
        <div className="lefttree-status" role="alert">
          {moveError}
        </div>
      )}
      <ul
        className="navtree"
        aria-label="Pages"
        onContextMenu={
          canEdit ? (e) => openMenu(e, { kind: "root" }) : undefined
        }
      >
        {nodes.map((node) => (
          <TreeRow
            key={node.path}
            node={node}
            depth={0}
            canEdit={canEdit}
            onContext={openMenu}
            onDropNode={dropNode}
          />
        ))}
      </ul>

      {/* The blank nav space below the tree right-clicks / drops to the root.
          It's presentational (aria-hidden); keyboard users reach root-create via
          the top "New page"/"New folder" buttons. */}
      <RootDropZone
        canEdit={canEdit}
        onContext={openMenu}
        onDropNode={dropNode}
      />

      {menu && menuItems(menu.target).length > 0 && (
        <TreeContextMenu
          x={menu.x}
          y={menu.y}
          items={menuItems(menu.target)}
          onClose={() => setMenu(null)}
        />
      )}

      {create?.kind === "page" && (
        <CreatePageModal
          open
          folder={create.folder}
          folderName={create.folderName}
          onClose={() => setCreate(null)}
        />
      )}
      {create?.kind === "folder" && (
        <CreateFolderModal
          open
          parent={create.parent}
          onClose={() => setCreate(null)}
        />
      )}

      {pageAction?.kind === "rename" && (
        <RenameModal
          open
          path={pageAction.path}
          currentTitle={pageAction.title}
          onClose={() => setPageAction(null)}
        />
      )}
      {pageAction?.kind === "move" && (
        <MoveDialog
          open
          path={pageAction.path}
          onClose={() => setPageAction(null)}
        />
      )}
      {pageAction?.kind === "delete" && (
        <DeleteConfirmDialog
          open
          path={pageAction.path}
          title={pageAction.title}
          onClose={() => setPageAction(null)}
        />
      )}
      {pageAction?.kind === "history" && (
        <HistoryPanel
          open
          path={pageAction.path}
          onClose={() => setPageAction(null)}
        />
      )}

      {folderAction?.kind === "rename" && (
        <RenameModal
          open
          kind="folder"
          path={folderAction.path}
          currentTitle={folderAction.title}
          onClose={() => setFolderAction(null)}
        />
      )}
      {folderAction?.kind === "move" && (
        <MoveDialog
          open
          kind="folder"
          path={folderAction.path}
          onClose={() => setFolderAction(null)}
        />
      )}
      {folderAction?.kind === "delete" && (
        <DeleteFolderDialog
          open
          dir={folderAction.path}
          title={folderAction.title}
          onClose={() => setFolderAction(null)}
        />
      )}
    </div>
  );
});

export default LeftTree;

// dragInfo reads which kind of node is being dragged from a dataTransfer's type
// list (set DURING dragstart). Returns null when neither okf drag type is present
// so non-okf drags (e.g. external files) never light up a drop target.
function dragKindFromTypes(types: readonly string[]): DragKind | null {
  if (types.includes(FOLDER_DRAG_TYPE)) return "folder";
  if (types.includes(PAGE_DRAG_TYPE)) return "page";
  return null;
}

// useNodeDropZone encapsulates the drag-over highlight + drop handling shared by
// folder rows and the root zone, for BOTH page and folder drags. dest is the
// destination folder a dropped node moves into ("" = root). During dragover it
// computes dropAllowed(kind, dragPath, dest) and ONLY preventDefault()+highlights
// when allowed; an invalid drop (self/descendant/same-parent) leaves the row in
// its resting state and shows cursor:not-allowed (no highlight — UI-SPEC). The
// dragged path travels through dataTransfer (readable on drop) but folder paths
// are NOT exposed as a custom type during dragover in jsdom, so the guard reads
// the path from the active-drag ref the dragstart populated.
function useNodeDropZone(
  dest: string,
  onDropNode: (kind: DragKind, srcPath: string, destFolder: string) => void,
) {
  const [active, setActive] = useState(false);

  function onDragOver(e: DragEvent) {
    const kind = dragKindFromTypes(e.dataTransfer.types);
    if (!kind) return;
    // The dragged path is the live module-level active drag (set on dragstart);
    // dataTransfer.getData is empty during dragover by spec, so we cannot read
    // the path here. dropAllowed needs the path → consult activeDragPath.
    const srcPath = activeDragPath;
    if (srcPath !== null && !dropAllowed(kind, srcPath, dest)) {
      // Invalid: do NOT preventDefault (so the native not-allowed cursor shows)
      // and do NOT highlight — the row stays resting.
      return;
    }
    e.preventDefault();
    setActive(true);
  }
  function onDragLeave() {
    setActive(false);
  }
  function onDrop(e: DragEvent) {
    setActive(false);
    const kind = dragKindFromTypes(e.dataTransfer.types);
    if (!kind) return;
    const type = kind === "folder" ? FOLDER_DRAG_TYPE : PAGE_DRAG_TYPE;
    const srcPath = e.dataTransfer.getData(type) || activeDragPath || "";
    if (!srcPath) return;
    e.preventDefault();
    onDropNode(kind, srcPath, dest);
  }

  return { active, onDragOver, onDragLeave, onDrop };
}

// activeDragPath holds the path of the node currently being dragged. The HTML5
// DnD spec makes dataTransfer.getData() return "" during dragover (data is only
// readable on drop), so the drop-target validity check (which must run DURING
// dragover to show the correct affordance) reads the path from here instead.
// It's set on dragstart and cleared on dragend.
let activeDragPath: string | null = null;

// RootDropZone is the blank area below the tree: drop a page/folder here to move
// it to the top level, or right-click for root-scoped create.
function RootDropZone({
  canEdit,
  onContext,
  onDropNode,
}: {
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
  onDropNode: (kind: DragKind, srcPath: string, destFolder: string) => void;
}) {
  const drop = useNodeDropZone("", onDropNode);
  return (
    <div
      className={`lefttree-root-drop${
        drop.active ? " lefttree-droptarget" : ""
      }`}
      aria-hidden="true"
      onContextMenu={canEdit ? (e) => onContext(e, { kind: "root" }) : undefined}
      onDragOver={drop.onDragOver}
      onDragLeave={drop.onDragLeave}
      onDrop={drop.onDrop}
    />
  );
}

// TreeRow recursively renders one tree node. A folder row carries an
// expand/collapse caret, is draggable (editor only), and is a drop target for
// both pages and folders; a page row is draggable (editor only), navigable,
// right-clickable, and highlights when active.
function TreeRow({
  node,
  depth,
  canEdit,
  onContext,
  onDropNode,
}: {
  node: TreeNode;
  depth: number;
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
  onDropNode: (kind: DragKind, srcPath: string, destFolder: string) => void;
}) {
  const indentStyle = {
    paddingLeft: `calc(${depth} * var(--tree-indent) + var(--space-sm))`,
  };
  if (node.type === "folder") {
    return (
      <FolderRow
        node={node}
        depth={depth}
        indentStyle={indentStyle}
        canEdit={canEdit}
        onContext={onContext}
        onDropNode={onDropNode}
      />
    );
  }
  return (
    <PageRow
      node={node}
      indentStyle={indentStyle}
      canEdit={canEdit}
      onContext={onContext}
    />
  );
}

function FolderRow({
  node,
  depth,
  indentStyle,
  canEdit,
  onContext,
  onDropNode,
}: {
  node: TreeNode;
  depth: number;
  indentStyle: { paddingLeft: string };
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
  onDropNode: (kind: DragKind, srcPath: string, destFolder: string) => void;
}) {
  const [expanded, setExpanded] = useState(true);
  const drop = useNodeDropZone(node.path, onDropNode);

  return (
    <li>
      <div
        className={`navrow navrow-folder${
          drop.active ? " lefttree-droptarget" : ""
        }`}
        style={indentStyle}
        // Editors can drag a folder to relocate its whole subtree; the second
        // okf drag type keeps folder and page drags unambiguous on a shared
        // drop target.
        draggable={canEdit}
        onDragStart={(e: DragEvent) => {
          e.dataTransfer.setData(FOLDER_DRAG_TYPE, node.path);
          e.dataTransfer.effectAllowed = "move";
          activeDragPath = node.path;
        }}
        onDragEnd={() => {
          activeDragPath = null;
        }}
        onContextMenu={(e) =>
          onContext(e, { kind: "folder", path: node.path, title: node.title })
        }
        onDragOver={drop.onDragOver}
        onDragLeave={drop.onDragLeave}
        onDrop={drop.onDrop}
      >
        <button
          type="button"
          className="tree-caret"
          aria-label={`${expanded ? "Collapse" : "Expand"} ${node.title}`}
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? (
            <ChevronDown size={16} aria-hidden="true" />
          ) : (
            <ChevronRight size={16} aria-hidden="true" />
          )}
        </button>
        <Folder size={16} aria-hidden="true" className="tree-icon" />
        <span className="tree-label">{node.title}</span>
      </div>
      {expanded && node.children && node.children.length > 0 && (
        <ul className="navtree">
          {node.children.map((child) => (
            <TreeRow
              key={child.path}
              node={child}
              depth={depth + 1}
              canEdit={canEdit}
              onContext={onContext}
              onDropNode={onDropNode}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

function PageRow({
  node,
  indentStyle,
  canEdit,
  onContext,
}: {
  node: TreeNode;
  indentStyle: { paddingLeft: string };
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
}) {
  const navigate = useNavigate();
  const params = useParams();

  // The active page is the one whose path matches the /app/page/:path route.
  const activePath = params["*"] ?? params.path ?? "";
  const isActive = activePath === node.path;

  return (
    <li>
      <button
        type="button"
        className={`navrow navrow-page${isActive ? " navrow-active" : ""}`}
        style={indentStyle}
        aria-current={isActive ? "page" : undefined}
        // Only editors can move pages; readers' rows are non-draggable.
        draggable={canEdit}
        onDragStart={(e: DragEvent) => {
          e.dataTransfer.setData(PAGE_DRAG_TYPE, node.path);
          e.dataTransfer.effectAllowed = "move";
          activeDragPath = node.path;
        }}
        onDragEnd={() => {
          activeDragPath = null;
        }}
        onContextMenu={(e) =>
          onContext(e, { kind: "page", path: node.path, title: node.title })
        }
        onClick={() => navigate(`/app/page/${node.path}`)}
      >
        <FileText size={16} aria-hidden="true" className="tree-icon" />
        <span className="tree-label">{node.title}</span>
      </button>
    </li>
  );
}

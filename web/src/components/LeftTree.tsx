import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useState,
  type DragEvent,
  type MouseEvent,
} from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, ChevronDown, FileText, Folder } from "lucide-react";

import {
  getTree,
  me,
  movePage,
  // The folder client functions (07-01/07-02) are imported here so the module
  // graph resolves them now; they are WIRED to real call sites in Plan 04 (folder
  // DnD, folder context-menu mutate actions, optimistic updates). Referencing the
  // type-only imports keeps tree-shaking honest while the calls land later.
  type Me,
  type TreeNode,
} from "../api/client";
import TreeContextMenu, { type TreeContextMenuItem } from "./TreeContextMenu";
import CreatePageModal from "./CreatePageModal";
import CreateFolderModal from "./CreateFolderModal";
import RenameModal from "./RenameModal";
import MoveDialog from "./MoveDialog";
import DeleteConfirmDialog from "./DeleteConfirmDialog";
import HistoryPanel from "./HistoryPanel";
import "./LeftTree.css";

// The custom DataTransfer key a dragged page carries. Folder DnD (Plan 04) adds a
// second key; isolating the constant keeps both drag types unambiguous.
const PAGE_DRAG_TYPE = "application/x-okf-page";

// parentOf returns the folder path that contains a page path. "a/b/c.md" → "a/b";
// "home.md" → "" (root). Used to short-circuit no-op DnD moves (same parent).
function parentOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i === -1 ? "" : path.slice(0, i);
}

// LeftTreeHandle lets the parent (AppShell) drive the root-scoped create modals
// from its top "New page" / "New folder" buttons while the tree itself drives
// folder-scoped create via the right-click context menu.
export interface LeftTreeHandle {
  openCreatePage: () => void;
  openCreateFolder: () => void;
}

// MenuTarget identifies which node (or root) was right-clicked — the input to
// menuItems(). Folder = create-only here; folder mutate actions land in Plan 04.
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
  const queryClient = useQueryClient();
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

  const moveMut = useMutation({
    mutationFn: ({ path, parent }: { path: string; parent: string }) =>
      movePage(path, parent),
    // Shipped form: wait for the commit, then invalidate. The optimistic
    // onMutate/onError/onSettled upgrade is Plan 04 (the commit-wait remains the
    // correctness backstop there).
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
    },
  });

  // movePageInto moves a page into a destination folder ("" = root). Same-parent
  // drops are guarded so we never round-trip a pointless move.
  const movePageInto = useCallback(
    (pagePath: string, destFolder: string) => {
      if (parentOf(pagePath) === destFolder) return;
      moveMut.mutate({ path: pagePath, parent: destFolder });
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
            onDropPage={movePageInto}
          />
        ))}
      </ul>

      {/* The blank nav space below the tree right-clicks / drops to the root.
          It's presentational (aria-hidden); keyboard users reach root-create via
          the top "New page"/"New folder" buttons. */}
      <RootDropZone
        canEdit={canEdit}
        onContext={openMenu}
        onDropPage={movePageInto}
      />

      {menu && (
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
    </div>
  );
});

export default LeftTree;

// usePageDropZone encapsulates the drag-over highlight + page-drop handling
// shared by folder rows and the root zone. dest is the destination folder a
// dropped page moves into ("" = root). It exposes the active highlight flag and
// the three DnD handlers; folder DnD (a second drag type) layers on in Plan 04.
function usePageDropZone(
  dest: string,
  onDropPage: (pagePath: string, destFolder: string) => void,
) {
  const [active, setActive] = useState(false);

  function onDragOver(e: DragEvent) {
    if (e.dataTransfer.types.includes(PAGE_DRAG_TYPE)) {
      e.preventDefault();
      setActive(true);
    }
  }
  function onDragLeave() {
    setActive(false);
  }
  function onDrop(e: DragEvent) {
    const pagePath = e.dataTransfer.getData(PAGE_DRAG_TYPE);
    setActive(false);
    if (pagePath) {
      e.preventDefault();
      onDropPage(pagePath, dest);
    }
  }

  return { active, onDragOver, onDragLeave, onDrop };
}

// RootDropZone is the blank area below the tree: drop a page here to move it to
// the root, or right-click for root-scoped create.
function RootDropZone({
  canEdit,
  onContext,
  onDropPage,
}: {
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
  onDropPage: (pagePath: string, destFolder: string) => void;
}) {
  const drop = usePageDropZone("", onDropPage);
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
// expand/collapse caret and is a page-drop target; a page row is draggable
// (editor only), navigable, right-clickable, and highlights when active.
function TreeRow({
  node,
  depth,
  canEdit,
  onContext,
  onDropPage,
}: {
  node: TreeNode;
  depth: number;
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
  onDropPage: (pagePath: string, destFolder: string) => void;
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
        onDropPage={onDropPage}
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
  onDropPage,
}: {
  node: TreeNode;
  depth: number;
  indentStyle: { paddingLeft: string };
  canEdit: boolean;
  onContext: (e: MouseEvent, target: MenuTarget) => void;
  onDropPage: (pagePath: string, destFolder: string) => void;
}) {
  const [expanded, setExpanded] = useState(true);
  const drop = usePageDropZone(node.path, onDropPage);

  return (
    <li>
      <div
        className={`navrow navrow-folder${
          drop.active ? " lefttree-droptarget" : ""
        }`}
        style={indentStyle}
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
              onDropPage={onDropPage}
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

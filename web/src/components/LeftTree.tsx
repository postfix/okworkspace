import {
  forwardRef,
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

// Context-menu state: which node (or root) was right-clicked, and where.
type MenuTarget =
  | { kind: "folder"; path: string; title: string }
  | { kind: "page"; path: string; title: string }
  | { kind: "root" };

interface MenuState {
  x: number;
  y: number;
  target: MenuTarget;
}

// Which create modal is open and the folder/parent it targets ("" = root).
type CreateState =
  | { kind: "page"; folder: string; folderName: string }
  | { kind: "folder"; parent: string }
  | null;

// Which page-action modal is open and the page it targets. These reuse the very
// same modals PageActionMenu wires in the page header — parameterized by the
// right-clicked node rather than the currently-open page.
type PageActionState =
  | { kind: "rename" | "move" | "delete" | "history"; path: string; title: string }
  | null;

// LeftTree renders the live navigation tree (NAV-01) with Obsidian-style
// affordances: right-click a folder to create inside it, right-click a page to
// rename/move/delete/view history, right-click empty space to create at root,
// and drag a page onto a folder (or the root drop zone) to move it. Folders
// expand/collapse (NAV-02); the active page row is highlighted (NAV-04). All
// mutations reuse the existing modals/APIs and rely on the ["tree"] query
// invalidation those modals already perform — no manual refresh.
const LeftTree = forwardRef<LeftTreeHandle>(function LeftTree(_props, ref) {
  const queryClient = useQueryClient();
  const { data, isLoading, isError } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
  });
  const { data: user } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  // Editors and admins may mutate; readers get a read-only menu (RBAC mirror of
  // the server gate — same policy PageActionMenu enforces).
  const canEdit = user?.role === "editor" || user?.role === "admin";

  // Coalesce null/undefined to []. The endpoint can serialize an empty repo's
  // tree to JSON `null`; a `= []` default only guards undefined, so without this
  // nodes.map would throw on null and white-screen the app (UAT blocker).
  const nodes = data ?? [];

  const [menu, setMenu] = useState<MenuState | null>(null);
  const [create, setCreate] = useState<CreateState>(null);
  const [pageAction, setPageAction] = useState<PageActionState>(null);
  const [rootDropActive, setRootDropActive] = useState(false);

  // Expose root-scoped create to the parent's top buttons.
  useImperativeHandle(
    ref,
    () => ({
      openCreatePage: () =>
        setCreate({ kind: "page", folder: "", folderName: "your workspace" }),
      openCreateFolder: () => setCreate({ kind: "folder", parent: "" }),
    }),
    [],
  );

  const moveMut = useMutation({
    mutationFn: ({ path, parent }: { path: string; parent: string }) =>
      movePage(path, parent),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
    },
  });

  // dropPageInto moves a page into a destination folder ("" = root). No-op drops
  // (same parent) are guarded so we never round-trip a pointless move.
  function dropPageInto(pagePath: string, destFolder: string) {
    if (parentOf(pagePath) === destFolder) return;
    moveMut.mutate({ path: pagePath, parent: destFolder });
  }

  function openMenu(e: MouseEvent, target: MenuTarget) {
    e.preventDefault();
    e.stopPropagation();
    setMenu({ x: e.clientX, y: e.clientY, target });
  }

  // Build the menu items for the current target. Folder = create-only (no
  // backend folder rename/move/delete). Page (editor) = rename/move/delete +
  // history; page (reader) = history only. Root = create at top level.
  function menuItems(target: MenuTarget): TreeContextMenuItem[] {
    if (target.kind === "folder") {
      if (!canEdit) return [];
      return [
        {
          label: "New page here",
          onSelect: () =>
            setCreate({
              kind: "page",
              folder: target.path,
              folderName: target.title,
            }),
        },
        {
          label: "New folder here",
          onSelect: () => setCreate({ kind: "folder", parent: target.path }),
        },
      ];
    }
    if (target.kind === "page") {
      const items: TreeContextMenuItem[] = [];
      if (canEdit) {
        items.push({
          label: "Rename",
          onSelect: () =>
            setPageAction({ kind: "rename", path: target.path, title: target.title }),
        });
        items.push({
          label: "Move",
          onSelect: () =>
            setPageAction({ kind: "move", path: target.path, title: target.title }),
        });
      }
      items.push({
        label: "Version history",
        onSelect: () =>
          setPageAction({ kind: "history", path: target.path, title: target.title }),
      });
      if (canEdit) {
        items.push({
          label: "Delete",
          danger: true,
          onSelect: () =>
            setPageAction({ kind: "delete", path: target.path, title: target.title }),
        });
      }
      return items;
    }
    // root
    if (!canEdit) return [];
    return [
      {
        label: "New page",
        onSelect: () =>
          setCreate({ kind: "page", folder: "", folderName: "your workspace" }),
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

  // The empty-area target sits below the tree so right-click / drop on blank nav
  // space targets the root. It's a div (presentational); keyboard users reach
  // root-create via the top "New page"/"New folder" buttons.
  return (
    <div className="lefttree">
      <ul
        className="navtree"
        aria-label="Pages"
        onContextMenu={canEdit ? (e) => openMenu(e, { kind: "root" }) : undefined}
      >
        {nodes.map((node) => (
          <TreeRow
            key={node.path}
            node={node}
            depth={0}
            canEdit={canEdit}
            onContext={openMenu}
            onDropPage={dropPageInto}
          />
        ))}
      </ul>

      <div
        className={`lefttree-root-drop${rootDropActive ? " lefttree-droptarget" : ""}`}
        aria-hidden="true"
        onContextMenu={canEdit ? (e) => openMenu(e, { kind: "root" }) : undefined}
        onDragOver={(e) => {
          if (e.dataTransfer.types.includes("application/x-okf-page")) {
            e.preventDefault();
            setRootDropActive(true);
          }
        }}
        onDragLeave={() => setRootDropActive(false)}
        onDrop={(e) => {
          const pagePath = e.dataTransfer.getData("application/x-okf-page");
          setRootDropActive(false);
          if (pagePath) {
            e.preventDefault();
            dropPageInto(pagePath, "");
          }
        }}
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
  const navigate = useNavigate();
  const params = useParams();
  const [expanded, setExpanded] = useState(true);
  const [dropActive, setDropActive] = useState(false);

  // The active page is the one whose path matches the /app/page/:path route.
  const activePath = params["*"] ?? params.path ?? "";
  const isActive = node.type === "page" && activePath === node.path;

  const indentStyle = {
    paddingLeft: `calc(${depth} * var(--tree-indent) + var(--space-sm))`,
  };

  if (node.type === "folder") {
    return (
      <li>
        <div
          className={`navrow navrow-folder${dropActive ? " lefttree-droptarget" : ""}`}
          style={indentStyle}
          onContextMenu={(e) =>
            onContext(e, { kind: "folder", path: node.path, title: node.title })
          }
          // A page dragged onto a folder row moves it into that folder.
          onDragOver={(e) => {
            if (e.dataTransfer.types.includes("application/x-okf-page")) {
              e.preventDefault();
              setDropActive(true);
            }
          }}
          onDragLeave={() => setDropActive(false)}
          onDrop={(e) => {
            const pagePath = e.dataTransfer.getData("application/x-okf-page");
            setDropActive(false);
            if (pagePath) {
              e.preventDefault();
              onDropPage(pagePath, node.path);
            }
          }}
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
          e.dataTransfer.setData("application/x-okf-page", node.path);
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

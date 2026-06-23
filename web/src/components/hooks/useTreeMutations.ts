import { useMutation, useQueryClient } from "@tanstack/react-query";

import {
  deleteFolder,
  moveFolder,
  movePage,
  renameFolder,
  type TreeNode,
} from "../../api/client";

// useTreeMutations centralises every OPTIMISTIC ["tree"] mutation for the folder
// tree (CONTEXT override replacing the shipped wait-then-refetch with onMutate →
// onError → onSettled). The pure helpers (applyMove, dropAllowed, parentOf,
// basename, destDir) mirror the server's literal prefix-swap semantics
// (relocateFolder, Pitfall 6) so the optimistic view matches the eventual refetch
// and the node never "jumps" on reconcile.

// The custom DataTransfer keys a dragged page / folder carries. Isolating these
// keeps the two drag types unambiguous on a shared drop target.
export const PAGE_DRAG_TYPE = "application/x-okf-page";
export const FOLDER_DRAG_TYPE = "application/x-okf-folder";

// DragKind discriminates the dragged node so dropAllowed/applyMove pick the right
// path math (a folder rewrites every descendant; a page rewrites only itself).
export type DragKind = "page" | "folder";

// The optimistic-rollback copy surfaced when a move fails and is rolled back
// (UI-SPEC Copywriting Contract).
export const ROLLBACK_COPY =
  "We couldn't move that just now — it's back where it was. Check your connection and try again.";

// parentOf returns the folder path that contains a page OR folder path.
// "a/b/c.md" → "a/b"; "a/b" → "a"; "home.md" / "a" → "" (root).
export function parentOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i === -1 ? "" : path.slice(0, i);
}

// basename returns the last path segment ("a/b/c.md" → "c.md"; "a/b" → "b").
export function basename(path: string): string {
  const i = path.lastIndexOf("/");
  return i === -1 ? path : path.slice(i + 1);
}

// destDir computes the new directory/path a node lands at when moved into
// destFolder ("" = root) — exactly the server's prefix-swap target. For a page
// "home.md" into "runbooks" → "runbooks/home.md"; into root → "home.md". For a
// folder "a/b" into "x" → "x/b"; into root → "b".
export function destDir(srcPath: string, destFolder: string): string {
  const name = basename(srcPath);
  return destFolder === "" ? name : `${destFolder}/${name}`;
}

// dropAllowed is the client-side validity guard computed DURING dragover so the
// affordance is correct before the drop (TREE-06). A folder cannot drop onto
// itself or any descendant; a same-parent drop is a no-op for both kinds.
export function dropAllowed(
  dragKind: DragKind,
  dragPath: string,
  targetFolder: string,
): boolean {
  if (dragKind === "folder") {
    if (targetFolder === dragPath) return false;
    if (targetFolder.startsWith(`${dragPath}/`)) return false;
    if (parentOf(dragPath) === targetFolder) return false;
  } else {
    if (parentOf(dragPath) === targetFolder) return false;
  }
  return true;
}

// rewritePath applies a literal prefix swap to one path: if it equals oldDir or
// starts with oldDir + "/", its oldDir prefix is replaced with newDir. Otherwise
// the path is returned unchanged. This is the exact server relocateFolder math
// (Pitfall 6) used to rewrite a moved folder's own path + every descendant path.
function rewritePath(path: string, oldDir: string, newDir: string): string {
  if (path === oldDir) return newDir;
  if (path.startsWith(`${oldDir}/`)) return newDir + path.slice(oldDir.length);
  return path;
}

// removeNode returns a NEW tree with the node at srcPath removed from its parent.
// It never mutates the input (TanStack snapshot must stay intact for rollback).
function removeNode(nodes: TreeNode[], srcPath: string): TreeNode[] {
  const out: TreeNode[] = [];
  for (const n of nodes) {
    if (n.path === srcPath) continue;
    out.push(
      n.children ? { ...n, children: removeNode(n.children, srcPath) } : n,
    );
  }
  return out;
}

// rewriteSubtree returns a NEW node whose path (and, recursively, every
// descendant path) has had oldDir → newDir applied (literal prefix swap).
function rewriteSubtree(
  node: TreeNode,
  oldDir: string,
  newDir: string,
): TreeNode {
  const next: TreeNode = { ...node, path: rewritePath(node.path, oldDir, newDir) };
  if (node.children) {
    next.children = node.children.map((c) =>
      rewriteSubtree(c, oldDir, newDir),
    );
  }
  return next;
}

// findNode locates the node with the given path anywhere in the tree.
function findNode(nodes: TreeNode[], path: string): TreeNode | null {
  for (const n of nodes) {
    if (n.path === path) return n;
    if (n.children) {
      const hit = findNode(n.children, path);
      if (hit) return hit;
    }
  }
  return null;
}

// insertInto returns a NEW tree with `node` inserted as a child of the folder at
// destFolder ("" = root → top level). Children are kept sorted folders-first then
// by title to match the server's tree ordering as closely as the cache can.
function insertInto(
  nodes: TreeNode[],
  destFolder: string,
  node: TreeNode,
): TreeNode[] {
  if (destFolder === "") {
    return sortNodes([...nodes, node]);
  }
  return nodes.map((n) => {
    if (n.path === destFolder && n.type === "folder") {
      const children = sortNodes([...(n.children ?? []), node]);
      return { ...n, children };
    }
    if (n.children) {
      return { ...n, children: insertInto(n.children, destFolder, node) };
    }
    return n;
  });
}

// sortNodes orders a sibling list folders-first, then case-insensitively by
// title — the conventional Obsidian-style tree order; the onSettled refetch is
// the authoritative reconciliation if the server orders differently.
function sortNodes(nodes: TreeNode[]): TreeNode[] {
  return [...nodes].sort((a, b) => {
    if (a.type !== b.type) return a.type === "folder" ? -1 : 1;
    return a.title.localeCompare(b.title, undefined, { sensitivity: "base" });
  });
}

// applyMove is a PURE transform over TreeNode[]: it removes the node at srcPath
// from its old parent, rewrites its path (for a folder, every descendant path)
// by literal prefix swap (oldDir → newDir), and re-inserts it under destFolder.
// The new path equals what the server relocateFolder/movePage produces, so the
// optimistic tree matches the eventual refetch (Pitfall 6). A no-op (node missing
// or same-parent) returns the tree unchanged.
export function applyMove(
  tree: TreeNode[],
  srcPath: string,
  destFolder: string,
  kind: DragKind,
): TreeNode[] {
  const node = findNode(tree, srcPath);
  if (!node) return tree;
  if (parentOf(srcPath) === destFolder) return tree;

  const newPath = destDir(srcPath, destFolder);
  const moved =
    kind === "folder"
      ? rewriteSubtree(node, srcPath, newPath)
      : { ...node, path: newPath };

  const without = removeNode(tree, srcPath);
  return insertInto(without, destFolder, moved);
}

// countFolderPages estimates how many Markdown files the folder at dirPath moves
// to Trash, for the DeleteFolderDialog's "{N} pages" copy. Each page descendant
// is one .md file; each DESCENDANT FOLDER node also contributes its own index.md
// (the tree never surfaces a folder's index.md as a page node — it is represented
// by the folder node itself), so descendant folders are counted too (IN-01). The
// dirPath folder's OWN index.md is implicit in the "This folder and …" copy and
// is not double-counted here. This stays a best-effort UI estimate; the backend
// DeleteFolder is authoritative for what actually moves to trash.
export function countFolderPages(tree: TreeNode[], dirPath: string): number {
  const folder = findNode(tree, dirPath);
  if (!folder) return 0;
  let n = 0;
  function walk(node: TreeNode) {
    for (const c of node.children ?? []) {
      // A page child is one .md file; a descendant folder child contributes its
      // own index.md (not surfaced as a page node) — both add one trashed file.
      n++;
      if (c.children) walk(c);
    }
  }
  walk(folder);
  return n;
}

// MoveVars drives the optimistic move mutation: which node, where to, and its
// kind (so applyMove and the client call pick page vs folder semantics).
export interface MoveVars {
  src: string;
  dest: string;
  kind: DragKind;
}

// useTreeMove returns the OPTIMISTIC move mutation for BOTH pages and folders:
// onMutate cancels in-flight ["tree"] refetches, snapshots the cache, applies
// applyMove immediately, and returns the snapshot; onError rolls the snapshot
// back and reports the rollback copy; onSettled invalidates ["tree"]+["page"] to
// reconcile with the server (the commit-wait correctness backstop). onRollback
// lets the caller surface ROLLBACK_COPY in its own UI surface.
export function useTreeMove(onRollback?: (message: string) => void) {
  const queryClient = useQueryClient();
  return useMutation<{ path: string }, Error, MoveVars, { prev?: TreeNode[] }>({
    mutationFn: ({ src, dest, kind }) =>
      kind === "folder" ? moveFolder(src, dest) : movePage(src, dest),
    onMutate: async ({ src, dest, kind }) => {
      await queryClient.cancelQueries({ queryKey: ["tree"] });
      const prev = queryClient.getQueryData<TreeNode[]>(["tree"]);
      queryClient.setQueryData<TreeNode[]>(["tree"], (old) =>
        applyMove(old ?? [], src, dest, kind),
      );
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(["tree"], ctx.prev);
      onRollback?.(ROLLBACK_COPY);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
    },
  });
}

// useFolderDelete returns the optimistic folder-delete mutation: onMutate snapshots
// the cache and prunes the folder subtree immediately; onError restores it;
// onSettled invalidates ["tree"]+["trash"] (the deleted folder reappears in Trash
// as a grouped row). It mirrors the page delete's recoverable-trash semantics.
export function useFolderDelete(onError?: (message: string) => void) {
  const queryClient = useQueryClient();
  return useMutation<void, Error, { dir: string }, { prev?: TreeNode[] }>({
    mutationFn: ({ dir }) => deleteFolder(dir),
    onMutate: async ({ dir }) => {
      await queryClient.cancelQueries({ queryKey: ["tree"] });
      const prev = queryClient.getQueryData<TreeNode[]>(["tree"]);
      queryClient.setQueryData<TreeNode[]>(["tree"], (old) =>
        removeNode(old ?? [], dir),
      );
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(["tree"], ctx.prev);
      onError?.("We couldn't delete that folder just now. Try again.");
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["trash"] });
    },
  });
}

// useFolderRename is a thin convenience wrapper for the folder-rename mutation used
// where a non-dialog rename is needed; the RenameModal owns its own mutation, so
// this stays available for completeness and keeps the renameFolder import honest.
export function useFolderRename() {
  const queryClient = useQueryClient();
  return useMutation<{ path: string }, Error, { dir: string; newName: string }>({
    mutationFn: ({ dir, newName }) => renameFolder(dir, newName),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
    },
  });
}

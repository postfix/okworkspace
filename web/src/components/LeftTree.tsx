import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, ChevronDown, FileText, Folder } from "lucide-react";

import { getTree, type TreeNode } from "../api/client";
import "./LeftTree.css";

// LeftTree replaces the Phase-0 PLACEHOLDER_TREE with the live navigation tree
// (NAV-01). Folders expand/collapse (NAV-02); the row matching the open page is
// highlighted (NAV-04). Page rows are interactive (no more navrow-disabled).
export default function LeftTree() {
  const { data, isLoading, isError } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
  });
  // Coalesce null/undefined to []. The endpoint can serialize an empty repo's
  // tree to JSON `null`; a `= []` default only guards undefined, so without this
  // nodes.map would throw on null and white-screen the app (UAT blocker).
  const nodes = data ?? [];

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
    <ul className="navtree" aria-label="Pages">
      {nodes.map((node) => (
        <TreeRow key={node.path} node={node} depth={0} />
      ))}
    </ul>
  );
}

function TreeRow({ node, depth }: { node: TreeNode; depth: number }) {
  const navigate = useNavigate();
  const params = useParams();
  const [expanded, setExpanded] = useState(true);

  // The active page is the one whose path matches the /app/page/:path route.
  const activePath = params["*"] ?? params.path ?? "";
  const isActive = node.type === "page" && activePath === node.path;

  const indentStyle = {
    paddingLeft: `calc(${depth} * var(--tree-indent) + var(--space-sm))`,
  };

  if (node.type === "folder") {
    return (
      <li>
        <div className="navrow navrow-folder" style={indentStyle}>
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
              <TreeRow key={child.path} node={child} depth={depth + 1} />
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
        onClick={() => navigate(`/app/page/${node.path}`)}
      >
        <FileText size={16} aria-hidden="true" className="tree-icon" />
        <span className="tree-label">{node.title}</span>
      </button>
    </li>
  );
}

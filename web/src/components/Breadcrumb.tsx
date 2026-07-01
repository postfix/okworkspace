import { Fragment } from "react";
import { useNavigate } from "react-router-dom";

import "./Breadcrumb.css";

// Breadcrumb renders the folder trail above a page (Obsidian parity). Given a page
// path like "Cooking/Recipes/Salo.md" it shows "Cooking / Recipes" as clickable
// crumbs that navigate to each folder's index page (folder/index.md), followed by
// the current page's title as a non-interactive tail. The single-segment case
// (a top-level page) renders nothing — there is no parent trail to show.
//
// A folder crumb navigates to `folder/index.md`; if that page does not exist the
// read view will surface its own not-found state (we do not pre-check existence —
// the trail mirrors the on-disk path structure, which is the honest breadcrumb).
export default function Breadcrumb({
  path,
  title,
}: {
  path: string;
  title: string;
}) {
  const navigate = useNavigate();
  const segments = path.split("/").filter((s) => s !== "");
  // The last segment is the current file; the crumbs are its ancestor folders.
  const folders = segments.slice(0, -1);
  if (folders.length === 0) return null;

  return (
    <nav className="breadcrumb" aria-label="Breadcrumb">
      {folders.map((folder, i) => {
        const folderPath = folders.slice(0, i + 1).join("/");
        return (
          <Fragment key={folderPath}>
            <button
              type="button"
              className="breadcrumb-crumb"
              onClick={() => navigate(`/app/page/${folderPath}/index.md`)}
            >
              {folder}
            </button>
            <span className="breadcrumb-sep" aria-hidden="true">
              /
            </span>
          </Fragment>
        );
      })}
      <span className="breadcrumb-current">{title}</span>
    </nav>
  );
}

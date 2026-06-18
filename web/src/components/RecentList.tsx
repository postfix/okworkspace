import { useNavigate } from "react-router-dom";
import { Clock } from "lucide-react";

import { useRecent } from "../stores/recent";
import "./RecentList.css";

// RecentList shows the client-side recently-visited pages (NAV-05), most recent
// first. Empty state uses the UI-SPEC muted copy.
export default function RecentList() {
  const navigate = useNavigate();
  const recents = useRecent((s) => s.recents);

  return (
    <div className="recentlist">
      <div className="recentlist-label">Recent</div>
      {recents.length === 0 ? (
        <p className="recentlist-empty">
          Pages you open will show up here for quick access.
        </p>
      ) : (
        <ul className="navtree">
          {recents.map((r) => (
            <li key={r.path}>
              <button
                type="button"
                className="navrow navrow-page"
                onClick={() => navigate(`/app/page/${r.path}`)}
              >
                <Clock size={16} aria-hidden="true" className="tree-icon" />
                <span className="tree-label">{r.title}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

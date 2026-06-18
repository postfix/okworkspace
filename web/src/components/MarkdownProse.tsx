import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize from "rehype-sanitize";

import { resolveRelativeMdLink } from "../lib/mdlink";
import "./MarkdownProse.css";

// MarkdownProse renders committed page Markdown safely (T-02-03): react-markdown
// with no inner-HTML injection, remark-gfm to match the server's Goldmark GFM,
// and rehype-sanitize ON. The raw-HTML rehype plugin is deliberately NOT enabled
// so raw HTML in page content can never become stored XSS. Internal `.md` links
// navigate within the app (D-06) rather than leaving to a raw file; they are
// resolved against `currentPath` (the linking page's workspace-relative path),
// matching the LinkPicker's relative-to-the-page emit convention (D-05).
export default function MarkdownProse({
  body,
  currentPath,
}: {
  body: string;
  currentPath: string;
}) {
  const navigate = useNavigate();

  const components = useMemo(
    () => ({
      a({ href, children, ...props }: { href?: string; children?: React.ReactNode }) {
        // Resolve a relative .md link against the current page's directory to a
        // clean workspace-relative path; null means it is an external/non-.md
        // link that must be rendered unchanged (WR-02, D-06).
        const target = resolveRelativeMdLink(currentPath, href);
        if (target != null) {
          return (
            <a
              href={`/app/page/${target}`}
              className="prose-link"
              onClick={(e) => {
                e.preventDefault();
                navigate(`/app/page/${target}`);
              }}
              {...props}
            >
              {children}
            </a>
          );
        }
        return (
          <a href={href} className="prose-link" {...props}>
            {children}
          </a>
        );
      },
    }),
    [navigate, currentPath],
  );

  return (
    <div className="markdown-prose">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSanitize]}
        components={components}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize from "rehype-sanitize";

import "./MarkdownProse.css";

// MarkdownProse renders committed page Markdown safely (T-02-03): react-markdown
// with no inner-HTML injection, remark-gfm to match the server's Goldmark GFM,
// and rehype-sanitize ON. The raw-HTML rehype plugin is deliberately NOT enabled
// so raw HTML in page content can never become stored XSS. Internal `.md` links
// navigate within the app (D-06) rather than leaving to a raw file.
export default function MarkdownProse({ body }: { body: string }) {
  const navigate = useNavigate();

  const components = useMemo(
    () => ({
      a({ href, children, ...props }: { href?: string; children?: React.ReactNode }) {
        const isInternalMd =
          href != null && !/^[a-z]+:\/\//i.test(href) && href.endsWith(".md");
        if (isInternalMd) {
          // Resolve a relative .md link to an in-app page route (D-06).
          const target = href!.replace(/^\.?\//, "").replace(/^\.\.\//, "");
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
    [navigate],
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

import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSlug from "rehype-slug";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";

import { resolveRelativeMdLink } from "../lib/mdlink";
import "./MarkdownProse.css";

// headingIdSchema is the rehype-sanitize default schema with one change: `id` is
// removed from the `clobber` list. rehype-sanitize otherwise PREFIXES every id
// with `user-content-` (clobberPrefix), which would turn rehype-slug's
// `id="my-section"` into `id="user-content-my-section"` and break heading
// deep-links — the rendered id MUST equal okf.ScanHeadings's GitHub-style anchor
// (T-03-15). `id` is already in the default global allow-list, so no attribute
// is newly permitted; raw HTML stays disallowed (rehype-raw OFF), preserving the
// Phase 1 stored-XSS guard (T-03-14).
const headingIdSchema = {
  ...defaultSchema,
  clobber: (defaultSchema.clobber ?? []).filter((attr) => attr !== "id"),
};

// MarkdownProse renders committed page Markdown safely (T-02-03): react-markdown
// with no inner-HTML injection, remark-gfm to match the server's Goldmark GFM,
// and rehype-sanitize ON. The raw-HTML rehype plugin is deliberately NOT enabled
// so raw HTML in page content can never become stored XSS. rehype-slug runs
// BEFORE rehype-sanitize to give each rendered heading a GitHub-style `id`
// (via github-slugger — the same algorithm okf.ScanHeadings mirrors) so heading
// search results deep-link to the right section (SRCH-06). Internal `.md` links
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
        rehypePlugins={[rehypeSlug, [rehypeSanitize, headingIdSchema]]}
        components={components}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

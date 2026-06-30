// TagBadgeBar — the ED-02c read-only effective-tags display that replaces the
// InlineTagEditor chip field. It shows the page's effective tags as small pills that
// echo the in-editor `.cm-hashtag` badge look:
//   effective = union( readTags(frontmatter), inlineTagsFromBody(body) )
//   display order = frontmatter order, then inline-only tags appended (de-duped, ci).
//
// A tag is INLINE-ORIGIN iff it appears in the body (∈ inlineTagsFromBody(body)) →
// it carries NO ✕ (it is managed by editing the `#tag` in the body; removing the ✕
// would desync from the body on the next save's reconcile). A NON-INLINE tag
// (frontmatter − inline — i.e. an LLM/imported tag) gets an ✕ that does a byte-stable
// frontmatter removal via `onRemoveTag`. The bar reflects the LIVE body, so typing
// `#x` shows an un-removable badge immediately, before save. Read-only otherwise — the
// text-input + autocomplete surface is gone (tags are authored as `#tag` in the body).
import { X } from "lucide-react";
import { readTags } from "../lib/tag-frontmatter";
import { inlineTagsFromBody } from "../lib/cm/hashtag";
import "./TagBadgeBar.css";

interface Props {
  body: string;
  frontmatter: string;
  onRemoveTag: (tag: string) => void;
}

export default function TagBadgeBar({ body, frontmatter, onRemoveTag }: Props) {
  const inlineSet = new Set(inlineTagsFromBody(body)); // already lower-cased + de-duped
  const fm = readTags(frontmatter);

  // Build the effective list: frontmatter tags in their order, then inline-only tags
  // appended. De-dupe case-insensitively so a body `#Deploy` matching a frontmatter
  // `deploy` shows ONE badge. inlineOrigin drives whether the ✕ is shown.
  const seen = new Set<string>();
  const effective: { tag: string; inlineOrigin: boolean }[] = [];
  for (const t of fm) {
    const lower = t.toLowerCase();
    if (seen.has(lower)) continue;
    seen.add(lower);
    effective.push({ tag: t, inlineOrigin: inlineSet.has(lower) });
  }
  for (const t of inlineSet) {
    if (seen.has(t)) continue; // t is already lower-cased
    seen.add(t);
    effective.push({ tag: t, inlineOrigin: true });
  }

  if (effective.length === 0) return null;

  return (
    <div className="field">
      <label className="field-label">Tags</label>
      <div className="tag-badge-bar">
        {effective.map(({ tag, inlineOrigin }) => (
          <span
            key={tag.toLowerCase()}
            className={
              inlineOrigin ? "tag-badge tag-badge-inline" : "tag-badge"
            }
            title={
              inlineOrigin
                ? "Managed in the body — edit the #tag to change it"
                : undefined
            }
          >
            {/* "#" + tag are inert text nodes (no innerHTML) — untrusted tag bytes
                can never become live markup, mirroring the HashtagWidget. */}
            #{tag}
            {!inlineOrigin && (
              <button
                type="button"
                className="tag-badge-x"
                aria-label={`Remove tag ${tag}`}
                onClick={() => onRemoveTag(tag)}
              >
                <X size={12} />
              </button>
            )}
          </span>
        ))}
      </div>
    </div>
  );
}

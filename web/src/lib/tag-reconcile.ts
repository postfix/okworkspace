// tag-reconcile — the ED-02b materialization rule, as a PURE (UI-free) function so it
// can be golden-tested without React. On save, body `#tags` are reconciled into the
// canonical frontmatter `tags:` as a DIFF (add AND remove) — NOT a monotonic union
// (which could never remove a deleted `#tag`), and NOT a clobber (an LLM/imported tag
// that was never inline must survive).
//
// THE RULE (CONTEXT D-02), computed against the LAST SAVED body so the diff window is
// exactly "what changed since the server's copy":
//   prevInline    = inlineTagsFromBody(lastSavedBody)
//   curInline     = inlineTagsFromBody(currentBody)
//   removedInline = prevInline − curInline            (case-insensitive)
//   reconciled    = normalize( (frontmatterTags − removedInline) ∪ curInline )
// An LLM-only tag is in `frontmatterTags`, never in `removedInline`, so it persists.
//
// BYTE-STABILITY (CONTEXT D-03): `setTags` rewrites the whole tags block (and folds
// flow→block form), so calling it when nothing changed would churn bytes and could
// 409-thrash autosave. We compute `changed` by comparing the tag SETs case-insensitively
// and only call `setTags` when the set actually changed; otherwise the ORIGINAL
// frontmatter string is returned verbatim (true byte-stable round-trip).
import { readTags, setTags } from "./tag-frontmatter";
import { inlineTagsFromBody } from "./cm/hashtag";

const norm = (t: string): string => t.trim().toLowerCase();

// sameSet compares two tag lists as case-insensitive, trim-folded sets (membership
// only — order and case are byte-stability concerns handled by the caller's guard).
function sameSet(a: string[], b: string[]): boolean {
  const sa = new Set(a.map(norm).filter(Boolean));
  const sb = new Set(b.map(norm).filter(Boolean));
  if (sa.size !== sb.size) return false;
  for (const x of sa) if (!sb.has(x)) return false;
  return true;
}

export interface ReconcileArgs {
  /** The body the server last has (PageEditor's lastSavedBody.current). */
  lastSavedBody: string;
  /** The body about to be saved (PageEditor's sentBody). */
  currentBody: string;
  /** The frontmatter about to be saved (PageEditor's sentFrontmatter). */
  frontmatter: string;
}

export interface ReconcileResult {
  /** The (possibly rewritten) frontmatter; byte-identical to the input when unchanged. */
  frontmatter: string;
  /** True iff the tag set changed and `frontmatter` was rewritten via setTags. */
  changed: boolean;
}

export function reconcileFrontmatterTags(args: ReconcileArgs): ReconcileResult {
  const prevInline = inlineTagsFromBody(args.lastSavedBody);
  const curInline = inlineTagsFromBody(args.currentBody); // already lower-cased + de-duped

  // removedInline = prevInline − curInline (case-insensitive). A `#tag` that was inline
  // at the last save and is no longer inline now is a deletion the user made in the body.
  const curLower = new Set(curInline.map(norm));
  const removed = new Set(prevInline.map(norm).filter((t) => !curLower.has(t)));

  const fmTags = readTags(args.frontmatter);

  // reconciled = normalize( (frontmatterTags − removedInline) ∪ curInline ).
  // Order is deterministic for byte-stability: surviving frontmatter tags in their
  // existing order, then inline-only tags appended in document order. De-dupe + lower-
  // case via `seen` (so a case-collision like `#Deploy` body vs `deploy` fm → one tag).
  const seen = new Set<string>();
  const reconciled: string[] = [];
  const push = (raw: string) => {
    const t = norm(raw);
    if (!t || seen.has(t)) return;
    seen.add(t);
    reconciled.push(t);
  };
  for (const t of fmTags) {
    if (removed.has(norm(t))) continue; // a body-deleted inline tag is dropped
    push(t);
  }
  for (const t of curInline) push(t); // additions (and inline tags already in fm de-dupe)

  const changed = !sameSet(fmTags, reconciled);
  return {
    frontmatter: changed ? setTags(args.frontmatter, reconciled) : args.frontmatter,
    changed,
  };
}

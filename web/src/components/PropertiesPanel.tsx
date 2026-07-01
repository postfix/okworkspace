import { useMemo, useState, type ReactNode } from "react";
import { AlignLeft, Hash, Plus, Tags, X } from "lucide-react";

import { listScalarFields } from "../lib/frontmatter";
import { readTags } from "../lib/tag-frontmatter";
import { inlineTagsFromBody } from "../lib/cm/hashtag";
import "./PropertiesPanel.css";

// PropertiesPanel is the Obsidian-style metadata table under the title: a compact
// key → value grid for tags, description, and arbitrary top-level scalar frontmatter
// keys. It replaces the three full-width action ROWS that used to sit in the reading
// column (TagBadgeBar / TagSuggest / DescriptionGenerate) — those verbs now live as
// small inline affordances INSIDE the relevant property row (passed as slots), so the
// reading column stays document, not form.
//
// PURE presentational + generic-scalar editing. It never holds the canonical
// frontmatter: every mutation is an OPERATION callback the host applies against its
// LATEST frontmatter ref (no stale-closure clobber). Tags/description are NOT
// manually editable here (tags are authored as `#tag` in the body or via ✨ suggest;
// description is ✨ LLM-generated, ED-01a) — the panel shows them + their ✨ slot.
// Arbitrary scalar keys (author, status, …) ARE freely editable, matching Obsidian.

interface Props {
  body: string;
  frontmatter: string;
  // Read mode (PageView) shows values only — no ✕, no inputs, no add-row, no slots.
  readOnly: boolean;
  // Edit-mode operation callbacks — applied by the host against its latest ref.
  onRemoveTag?: (tag: string) => void;
  onSetField?: (key: string, value: string) => void;
  onRemoveField?: (key: string) => void;
  // The compact ✨ LLM affordances, wired by the host to its reconcile callbacks.
  tagSuggestSlot?: ReactNode;
  descriptionGenerateSlot?: ReactNode;
}

// Keys with dedicated rows / their own authoring path — excluded from the generic
// scalar-key rows so they are not rendered twice.
const RESERVED_KEYS: Record<string, true> = {
  title: true,
  tags: true,
  description: true,
};

export default function PropertiesPanel({
  body,
  frontmatter,
  readOnly,
  onRemoveTag,
  onSetField,
  onRemoveField,
  tagSuggestSlot,
  descriptionGenerateSlot,
}: Props) {
  // Effective tags = frontmatter order, then inline-only body tags, de-duped ci.
  // inlineOrigin tags are managed in the body (no ✕); mirrors TagBadgeBar exactly.
  const inlineSet = useMemo(() => new Set(inlineTagsFromBody(body)), [body]);
  const effectiveTags = useMemo(() => {
    const seen = new Set<string>();
    const out: { tag: string; inlineOrigin: boolean }[] = [];
    for (const t of readTags(frontmatter)) {
      const lower = t.toLowerCase();
      if (seen.has(lower)) continue;
      seen.add(lower);
      out.push({ tag: t, inlineOrigin: inlineSet.has(lower) });
    }
    for (const t of inlineSet) {
      if (seen.has(t)) continue;
      seen.add(t);
      out.push({ tag: t, inlineOrigin: true });
    }
    return out;
  }, [frontmatter, inlineSet]);

  const description = useMemo(() => {
    const m = frontmatter.match(/^description:\s*(.*)$/m);
    if (!m) return "";
    const v = m[1].trim();
    return v.replace(/^["']|["']$/g, "");
  }, [frontmatter]);

  const scalarFields = useMemo(
    () => listScalarFields(frontmatter).filter((f) => !RESERVED_KEYS[f.key]),
    [frontmatter],
  );

  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  const hasTags = effectiveTags.length > 0;
  const hasDescription = description !== "";
  const hasScalars = scalarFields.length > 0;

  // In read mode with nothing to show, render nothing (Obsidian hides an empty
  // Properties block on a bare page). In edit mode always render so a user can add.
  if (readOnly && !hasTags && !hasDescription && !hasScalars) return null;

  function addProperty() {
    const key = newKey.trim();
    if (key === "" || RESERVED_KEYS[key.toLowerCase()]) return;
    onSetField?.(key, newValue);
    setNewKey("");
    setNewValue("");
  }

  return (
    <section className="properties" aria-label="Properties">
      {/* Tags row — chips + (edit) compact ✨ suggest. Shown when there are tags,
          or in edit mode so the ✨ affordance is always reachable. */}
      {(hasTags || !readOnly) && (
        <div className="properties-row">
          <div className="properties-key">
            <Tags size={15} aria-hidden="true" />
            <span>tags</span>
          </div>
          <div className="properties-value properties-value-tags">
            {hasTags ? (
              <div className="tag-badge-bar">
                {effectiveTags.map(({ tag, inlineOrigin }) => (
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
                    #{tag}
                    {!readOnly && !inlineOrigin && (
                      <button
                        type="button"
                        className="tag-badge-x"
                        aria-label={`Remove tag ${tag}`}
                        onClick={() => onRemoveTag?.(tag)}
                      >
                        <X size={12} />
                      </button>
                    )}
                  </span>
                ))}
              </div>
            ) : (
              <span className="properties-empty">Empty</span>
            )}
            {!readOnly && tagSuggestSlot}
          </div>
        </div>
      )}

      {/* Description row — value + (edit) compact ✨ generate. */}
      {(hasDescription || !readOnly) && (
        <div className="properties-row">
          <div className="properties-key">
            <AlignLeft size={15} aria-hidden="true" />
            <span>description</span>
          </div>
          <div className="properties-value">
            {hasDescription ? (
              <span className="properties-text">{description}</span>
            ) : (
              <span className="properties-empty">Empty</span>
            )}
            {!readOnly && descriptionGenerateSlot}
          </div>
        </div>
      )}

      {/* Arbitrary scalar keys — freely editable in edit mode, plain text in read. */}
      {scalarFields.map(({ key, value }) => (
        <div className="properties-row" key={key}>
          <div className="properties-key">
            <Hash size={15} aria-hidden="true" />
            <span>{key}</span>
          </div>
          <div className="properties-value">
            {readOnly ? (
              <span className="properties-text">{value}</span>
            ) : (
              <>
                <input
                  className="properties-input"
                  type="text"
                  value={value}
                  aria-label={`${key} value`}
                  onChange={(e) => onSetField?.(key, e.target.value)}
                />
                <button
                  type="button"
                  className="btn btn-ghost icon-btn properties-remove"
                  aria-label={`Remove property ${key}`}
                  title="Remove property"
                  onClick={() => onRemoveField?.(key)}
                >
                  <X size={14} />
                </button>
              </>
            )}
          </div>
        </div>
      ))}

      {/* Add-property row (edit only). */}
      {!readOnly && (
        <div className="properties-add">
          <input
            className="properties-input properties-add-key"
            type="text"
            value={newKey}
            placeholder="Add property"
            aria-label="New property name"
            onChange={(e) => setNewKey(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                addProperty();
              }
            }}
          />
          <input
            className="properties-input properties-add-value"
            type="text"
            value={newValue}
            placeholder="Value"
            aria-label="New property value"
            onChange={(e) => setNewValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                addProperty();
              }
            }}
          />
          <button
            type="button"
            className="btn btn-ghost icon-btn"
            aria-label="Add property"
            title="Add property"
            disabled={newKey.trim() === ""}
            onClick={addProperty}
          >
            <Plus size={16} />
          </button>
        </div>
      )}
    </section>
  );
}

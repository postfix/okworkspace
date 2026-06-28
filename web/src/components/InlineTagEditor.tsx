// InlineTagEditor — inline tag editing with autocomplete + LLM suggest.
// Replaces the dialog-based TagSuggest flow. Tags are edited directly in the
// page editor frontmatter form: type to add (autocomplete from vocabulary),
// click X to remove, press "Suggest" to fill via LLM.
import { useState, useRef, useEffect, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { X, Sparkles, Loader2 } from "lucide-react";
import { suggestTags } from "../api/client";
import "./InlineTagEditor.css";

interface Props {
  pagePath: string;
  tags: string[];
  onTagsChange: (tags: string[]) => void;
}

const VOCAB_KEY = ["tags", "vocabulary"];

export default function InlineTagEditor({ pagePath, tags, onTagsChange }: Props) {
  const [input, setInput] = useState("");
  const [showDropdown, setShowDropdown] = useState(false);
  const [suggesting, setSuggesting] = useState(false);
  const [suggestError, setSuggestError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const { data: vocabulary = [] } = useQuery<string[]>({
    queryKey: VOCAB_KEY,
    queryFn: async () => {
      const res = await fetch("/api/v1/tags/vocabulary", {
        credentials: "same-origin",
      });
      if (!res.ok) return [];
      return (await res.json()) as string[];
    },
    staleTime: 60_000,
  });

  // Filter vocabulary by input, excluding already-added tags
  const matches = vocabulary.filter(
    (t) =>
      t.toLowerCase().includes(input.toLowerCase()) &&
      !tags.some((existing) => existing.toLowerCase() === t.toLowerCase()),
  );

  const addTag = useCallback(
    (tag: string) => {
      const trimmed = tag.trim();
      if (!trimmed) return;
      if (tags.some((t) => t.toLowerCase() === trimmed.toLowerCase())) return;
      onTagsChange([...tags, trimmed]);
      setInput("");
      setShowDropdown(false);
    },
    [tags, onTagsChange],
  );

  const removeTag = useCallback(
    (tag: string) => {
      onTagsChange(tags.filter((t) => t !== tag));
    },
    [tags, onTagsChange],
  );

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(input);
    } else if (e.key === "Backspace" && input === "" && tags.length > 0) {
      removeTag(tags[tags.length - 1]);
    } else if (e.key === "Escape") {
      setShowDropdown(false);
    }
  };

  const handleSuggest = async () => {
    setSuggesting(true);
    setSuggestError("");
    try {
      const result = await suggestTags(pagePath);
      const newTags = result.suggestions.map((s) => s.tag);
      // Merge with existing: keep tags not in suggestions, add suggestions
      const merged = [
        ...tags.filter(
          (t) => !newTags.some((s) => s.toLowerCase() === t.toLowerCase()),
        ),
        ...newTags,
      ];
      onTagsChange(merged);
    } catch {
      setSuggestError("Suggestions unavailable right now.");
    } finally {
      setSuggesting(false);
    }
  };

  // Close dropdown on outside click
  useEffect(() => {
    if (!showDropdown) return;
    const handler = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        inputRef.current &&
        !inputRef.current.contains(e.target as Node)
      ) {
        setShowDropdown(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [showDropdown]);

  return (
    <div className="field">
      <label className="field-label">Tags</label>
      <div className="inline-tags-wrap">
        {tags.map((tag) => (
          <span key={tag} className="inline-tag-chip">
            {tag}
            <button
              type="button"
              className="inline-tag-chip-x"
              onClick={() => removeTag(tag)}
              aria-label={`Remove tag ${tag}`}
            >
              <X size={12} />
            </button>
          </span>
        ))}
        <div className="inline-tags-input-area" ref={dropdownRef}>
          <input
            ref={inputRef}
            type="text"
            className="inline-tags-input"
            placeholder={tags.length === 0 ? "Add tags…" : "Add tag…"}
            value={input}
            onChange={(e) => {
              setInput(e.target.value);
              setShowDropdown(true);
            }}
            onFocus={() => input && setShowDropdown(true)}
            onKeyDown={handleKeyDown}
          />
          {showDropdown && matches.length > 0 && (
            <div className="inline-tags-dropdown">
              {matches.slice(0, 8).map((t) => (
                <button
                  key={t}
                  type="button"
                  className="inline-tags-dropdown-item"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    addTag(t);
                  }}
                >
                  {t}
                </button>
              ))}
            </div>
          )}
        </div>
        <button
          type="button"
          className="btn btn-ghost inline-tags-suggest"
          onClick={handleSuggest}
          disabled={suggesting}
          title="Suggest tags"
        >
          {suggesting ? (
            <Loader2 size={14} className="spinner" />
          ) : (
            <Sparkles size={14} />
          )}
        </button>
      </div>
      {suggestError && (
        <p className="inline-tags-error">{suggestError}</p>
      )}
    </div>
  );
}

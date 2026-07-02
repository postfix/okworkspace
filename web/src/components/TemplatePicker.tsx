import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileText } from "lucide-react";
import { listTemplates, type TemplateInfo } from "../api/client";
import "./TemplatePicker.css";

export interface TemplatePickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  disabled?: boolean;
  // onSelect fires when the user picks a template; the caller fetches the body,
  // substitutes placeholders, and inserts at the cursor via a CM6 transaction.
  onSelect: (tpl: TemplateInfo) => void;
}

// TemplatePicker is the editor "Insert template" affordance (EP-03). It opens a
// searchable list of Templates/*.md (fetched via listTemplates); selecting one
// fires onSelect so the caller can fetch the body, substitute placeholders
// ({{title}}/{{date}}/{{time}}), and insert at the cursor. Mirrors the
// LinkPicker pattern (button + popover). The list is fetched on-demand (only
// when the popover opens) and is cached by react-query.
export default function TemplatePicker({
  open,
  onOpenChange,
  disabled = false,
  onSelect,
}: TemplatePickerProps) {
  const [query, setQuery] = useState("");

  const { data: templates } = useQuery<TemplateInfo[]>({
    queryKey: ["templates"],
    queryFn: listTemplates,
    enabled: open,
  });

  const matches = (() => {
    const q = query.trim().toLowerCase();
    const list = templates ?? [];
    if (q === "") return list;
    return list.filter((t) => t.name.toLowerCase().includes(q));
  })();

  function select(tpl: TemplateInfo) {
    onSelect(tpl);
    setQuery("");
  }

  return (
    <div className="templatepicker">
      <button
        type="button"
        className="btn btn-secondary"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
        aria-haspopup="listbox"
        disabled={disabled}
        title="Insert template"
      >
        <FileText size={16} aria-hidden="true" />
        <span>Insert template</span>
      </button>
      {open && (
        <div className="templatepicker-popover">
          <input
            className="input"
            type="text"
            placeholder="Search templates…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Search templates"
            autoFocus
          />
          {matches.length === 0 ? (
            <p className="templatepicker-empty">
              {templates && templates.length === 0
                ? "No templates found in Templates/."
                : "No templates match your search."}
            </p>
          ) : (
            <ul className="templatepicker-list" role="listbox">
              {matches.map((t) => (
                <li key={t.path}>
                  <button
                    type="button"
                    className="templatepicker-item"
                    role="option"
                    aria-selected="false"
                    onClick={() => select(t)}
                  >
                    {t.name}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
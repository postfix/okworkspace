import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { createPage, getPage, listTemplates } from "../api/client";
import { substitutePlaceholders } from "../lib/templates";
import Dialog from "./Dialog";

// CreatePageModal asks only for a title (D-12); the backend slugs the filename
// and the path is never shown. On success it invalidates the tree and navigates
// to the new page. Copy follows the UI-SPEC contract exactly.
export default function CreatePageModal({
  open,
  folder,
  folderName,
  onClose,
}: {
  open: boolean;
  folder: string;
  folderName: string;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  // "" = Blank (the default, unchanged title-only create); otherwise the chosen
  // template's repo-relative path.
  const [templatePath, setTemplatePath] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Templates list (ED-06a). A failed/empty load simply leaves only "Blank".
  const { data: templates } = useQuery({
    queryKey: ["templates"],
    queryFn: listTemplates,
  });

  const createMut = useMutation({
    mutationFn: async () => {
      const t = title.trim();
      if (templatePath) {
        // Read the chosen template, substitute the three placeholders client-side
        // (D-03), then POST the seeded content — the server does the robust merge.
        const page = await getPage(templatePath);
        const now = new Date();
        return createPage(folder, t, {
          // {{title}} is substituted in the BODY only; the server's okf merge
          // owns the frontmatter title (and yaml-quotes it), so injecting a raw
          // title into frontmatter would corrupt valid templates (e.g. a colon
          // in the title). {{date}}/{{time}} are substituted in both.
          frontmatter: substitutePlaceholders(page.frontmatter, { now }),
          body: substitutePlaceholders(page.body, { title: t, now }),
        });
      }
      return createPage(folder, t);
    },
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      setTitle("");
      setTemplatePath("");
      setError(null);
      onClose();
      navigate(`/app/page/${res.path}`);
    },
    onError: (err: Error) => setError(err.message),
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (title.trim() === "") {
      setError("Give your page a title to create it.");
      return;
    }
    setError(null);
    createMut.mutate();
  }

  return (
    <Dialog open={open} title="New page" onCancel={onClose} hideFooter>
      <form onSubmit={handleSubmit}>
        <div className="field">
          <label className="field-label" htmlFor="new-page-title">
            Page title
          </label>
          <input
            id="new-page-title"
            className="input"
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            disabled={createMut.isPending}
            autoFocus
          />
          <p className="field-help">We'll create it in {folderName}.</p>
        </div>
        {templates && templates.length > 0 && (
          <div className="field">
            <label className="field-label" htmlFor="new-page-template">
              Template
            </label>
            <select
              id="new-page-template"
              className="input"
              value={templatePath}
              onChange={(e) => setTemplatePath(e.target.value)}
              disabled={createMut.isPending}
            >
              <option value="">Blank</option>
              {templates.map((tpl) => (
                <option key={tpl.path} value={tpl.path}>
                  {tpl.name}
                </option>
              ))}
            </select>
          </div>
        )}
        {error && (
          <div className="field-error" role="alert">
            {error}
          </div>
        )}
        <div className="dialog-footer">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onClose}
            disabled={createMut.isPending}
          >
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={createMut.isPending}
          >
            {createMut.isPending ? "Creating…" : "Create page"}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

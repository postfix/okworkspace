import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { createPage } from "../api/client";
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
  const [error, setError] = useState<string | null>(null);

  const createMut = useMutation({
    mutationFn: () => createPage(folder, title.trim()),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      setTitle("");
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

import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { createFolder } from "../api/client";
import Dialog from "./Dialog";

// CreateFolderModal asks for a folder name (NAV-03). On success it invalidates
// the tree. Copy follows the UI-SPEC contract.
export default function CreateFolderModal({
  open,
  parent,
  onClose,
}: {
  open: boolean;
  parent: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  const createMut = useMutation({
    mutationFn: () => createFolder(parent, name.trim()),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      setName("");
      setError(null);
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (name.trim() === "") {
      setError("Give your folder a name to create it.");
      return;
    }
    setError(null);
    createMut.mutate();
  }

  return (
    <Dialog open={open} title="New folder" onCancel={onClose} hideFooter>
      <form onSubmit={handleSubmit}>
        <div className="field">
          <label className="field-label" htmlFor="new-folder-name">
            Folder name
          </label>
          <input
            id="new-folder-name"
            className="input"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={createMut.isPending}
            autoFocus
          />
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
            {createMut.isPending ? "Creating…" : "Create folder"}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

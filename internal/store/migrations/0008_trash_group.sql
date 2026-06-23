-- 0008_trash_group: tag the trash rows produced by ONE folder-delete operation
-- with a shared delete_group_id so a folder delete can be restored as a unit
-- (TREE-04/TREE-05). Operational/derived data only — the canonical page content
-- still lives on disk under .okf-workspace/trash/ as real Git commits (delete = a
-- git mv per page, never a git rm), NOT in this table.
--
-- The column is nullable and additive: existing rows (and every future SOLO
-- per-page delete) read delete_group_id = NULL, which the listing surfaces as the
-- empty string and which RestoreGroup never matches — so per-page delete/restore
-- stay byte-identical. A folder delete generates one opaque group id (crypto/rand)
-- and binds it on every member row's INSERT (parameterized — never interpolated).
-- No index: the column is queried by equality at 5-user scale where an index is
-- gold-plating.

ALTER TABLE trash ADD COLUMN delete_group_id TEXT;

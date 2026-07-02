-- 0012_tag_meta_parent: add parent column to tag_meta (Phase 33, TF-03).
-- Additive: existing rows get '' (root). The parent field is part of the
-- files-as-truth Tags/*.md frontmatter; this column is the rebuildable cache.
ALTER TABLE tag_meta ADD COLUMN parent TEXT NOT NULL DEFAULT '';
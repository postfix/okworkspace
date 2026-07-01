---
title: Phase 27 Page Templates (ED-06a) — engineer
tags: [m1.4, phase27, templates, okf, frontmatter-merge]
---

# Phase 27 — Page Templates (ED-06a)

## Load-bearing pattern: okf frontmatter merge is FAITHFUL, not deduping
- `okf.Parse` builds a yaml.Node and does NOT reject duplicate keys; `okf.SetField`
  only updates the FIRST occurrence. So merging untrusted template frontmatter +
  Emit can produce a corrupt duplicate-key page.
- Fix used: `okf.ValidateFrontmatter(doc)` — strict `yaml.Unmarshal(d.RawFront, &map[string]any)`
  REJECTS duplicate keys + malformed YAML (verified). Call it after Parse, before
  SetField/Repair/Emit, to fail closed with a clean typed error (ErrTemplateInvalid→400).
  Keeps YAML confined to the okf package (pages doesn't import yaml).

## Template-seeded create = mirror pages.Create
- CreateFromTemplate seeds doc from okf.Parse(AssembleSource(fm, body)), SetField(title)
  (user title wins over template title:), Repair, then the SAME uniquePath +
  enqueueWrite "create" + index/graph upsert. Parse+validate BEFORE lockMutation (pure,
  fail-fast). Server never trusts a template PATH — only substituted CONTENT.
- SetField-then-Repair order is safe: Repair only appends MISSING required fields.

## Client substitution gotcha (code-review MAJOR, fixed)
- NEVER substitute a raw user {{title}} into YAML frontmatter — a colon title
  ("Meeting: Kickoff") makes `title: {{title}}` invalid YAML → 400, and "#" can
  truncate a value. The SERVER owns the frontmatter title (SetField yaml-quotes it).
- substitutePlaceholders takes an OPTIONAL title: substitute it in the BODY only
  (opaque Markdown, safe); date/time (YAML-safe scalars) in both. Verified server
  emits `title: 'Meeting: Kickoff'`.

## ListTemplates
- Reuse repo.Tree() (full recursive walk, fine at this scale), filter flat
  Templates/*.md (CutPrefix + reject Contains "/"), re-Resolve each (SEC-01). Empty
  when no folder.

## Known fork (routed up): getPage does not encodeURIComponent the path — pre-existing
app-wide; template filenames with #/?/% mis-route. Out of phase scope.

# qa — Phase 27 Page Templates (ED-06a) VERIFIED (m1.4 final phase)

**Verdict: PASS** (all 7 truths). Commits `4336980^..81a0ef8`. SMTC was down; verified via Bash/Read + live builds.

## Evidence at HEAD
- Backend: `go build/vet/test ./...` exit 0; fresh `-count=1 ./internal/{server,pages,okf,repo}` exit 0.
- Stale-diagnostics (DuplicateDecl indexOf/contains, unused handleListTemplates) CONFIRMED RESOLVED:
  build clean, test pkgs compile, route wired at router.go:177.
- Web: tsc 0, vitest 774/774 (63 files), build "✓ built in 537ms" (only pre-existing
  INEFFECTIVE_DYNAMIC_IMPORT warnings). Prod diff confined to 4 Go + 3 web files.

## Load-bearing pattern: robust frontmatter merge SERVER-side via okf
- okf's yaml.Node model is FAITHFUL — does NOT dedup duplicate keys; SetField only fixes first
  occurrence → a dup-key template would Emit a corrupt two-`title:` page.
- Fix: `okf.ValidateFrontmatter` (okf.go:107) = strict `map[string]any` decode that rejects dup keys +
  malformed YAML → `ErrTemplateInvalid` → 400. Run BEFORE the mutation lock (fail-fast, pure).
- Merge pipeline (pages/templates.go:99): Parse → ValidateFrontmatter → SetField(title, user-wins) →
  Repair → Emit → SAME single-writer Create pipeline (lockMutation/uniquePath/enqueueWrite "create"/
  index+graph upsert). Path derives from user title only — no template-PATH param, only CONTENT.

## Verification technique worth reusing
- Adversarial on-disk oracle (`assertSingleValidBlock`): read page off disk → assert exactly one
  fence + re-parse via okf.Parse + strict yaml decode (no dup/malformed) + user title wins + 5 required
  fields. Paired with a 39-case table + FuzzTemplateMerge totality invariant + alias-bomb timeout.
- Colon-title MAJOR fix (code-review): client substitutes {{title}} in BODY ONLY; server SetField owns
  frontmatter title: and yaml-quotes via Emit. So "Meeting: Kickoff" can't make `title: {{title}}`
  invalid YAML. Verify by: client passes title=undefined for frontmatter substitution.

## NON-gap routed up (milestone-wide, pre-existing)
- `client.ts getPage(path)` interpolates path RAW (no encodeURIComponent) into `/api/v1/pages/${path}`
  — app-wide convention (PageView + 8 PageEditor call sites). Template names with #/?/% mis-route.
  Happy path (normal filenames) works → did NOT fail Phase 27. Recommend a focused milestone-wide
  getPage-encoding fix.

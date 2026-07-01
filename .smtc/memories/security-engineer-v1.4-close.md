---
slug: security-engineer-v1.4-close
title: v1.4 security review — SHIP (mermaid/highlight/templates)
milestone: v1.4
---

# v1.4 Editor Enhancements — security review: SHIP (no fix required)

Review: `.planning/v1.4-SECURITY-REVIEW.md`. Verdict SHIP; no critical/high; 2 LOW accept-for-ship.

## Load-bearing facts (reuse next milestone)
- **Mermaid XSS defense is DELEGATED to upstream** (F-01, track): DOMParser('image/svg+xml')+importNode+appendChild
  stops `<script>` exec (already-started flag) but does NOT strip event-handler attrs (onload/onclick) —
  those rest ENTIRELY on mermaid securityLevel:'strict'→DOMPurify. Sound (dompurify 3.4.11, mermaid 11.16.0,
  npm audit 0). Re-trace on any mermaid MAJOR bump; keep the `^` ranges pinned + npm audit in CI.
- **No server-side `{{}}` template engine** — substitution is client-only; server just okf.Parse+ValidateFrontmatter.
  So the "template injection/DoS" worry has no server surface. Don't re-derive.
- **Path safety canon = `internal/repo/path.go` Resolve** (SEC-01, Phase 16): lexical ../NUL/absolute reject +
  percent-decode + Windows-sep + os.Root syscall guard + EvalSymlinks boundary. `folder` in Create/CreateFromTemplate
  is NOT cleanPathString'd in the handler but IS gated by uniquePath→Resolve → fail-closed. Trust Resolve.
- **okf.ValidateFrontmatter** (okf.go:107) = strict map[string]any decode → rejects dup-key+malformed YAML →
  ErrTemplateInvalid→400. yaml.v3 bounds alias expansion. This is the fail-closed gate for untrusted frontmatter.
- **No-innerHTML invariant (T-06-08)** holds across all cm widgets: createElement+textContent/createTextNode only.
  Verified by grep (only comments/tests match innerHTML). CodeBlockWidget/highlight tokens safe.

## Residual / recommended (non-blocking)
- F-02: `POST/PUT /pages` JSON bodies have NO MaxBytesReader (attachments DO). Pre-existing, milestone-wide DoS.
  Recommend a global JSON body cap. Not a v1.4 regression.
- F-03: govulncheck panics (ForEachElement TypeParam — Go 1.26 toolchain bug), Go dep-vuln scan UNAVAILABLE here.
  Mitigated: go.mod diff empty this milestone. Use `npm audit` for frontend; note govulncheck gap each session.

## Method note
Authz map lives in router.go: any-authed group vs editor subgroup (RequireRole from session) + global nosurf CSRF
(router.go:359, wraps everything, exempts GET). GET /templates=any-authed read; POST /pages=editor+CSRF.

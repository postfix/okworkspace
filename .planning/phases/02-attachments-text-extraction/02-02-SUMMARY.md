---
phase: 02-attachments-text-extraction
plan: 02
subsystem: web
tags: [attachments, attachment-card, image-preview, lucide-icons, dialog, stored-xss-guard, hidden-git]

# Dependency graph
requires:
  - phase: 02 (plan 01)
    provides: internal/attachments service, GET /attachments/* list + {id}/download, AttachmentMeta + downloadAttachmentUrl/listAttachments in api/client.ts, minimal AttachmentCard + AttachmentsSection mounted in PageView, SEC-02 inline/attachment disposition + nosniff
  - phase: 01
    provides: focus-trapped Dialog primitive, TrashView card/row CSS pattern, tokens.css design system, lucide-react icons, @tanstack/react-query
provides:
  - full ATT-03 attachment card (64x64 image thumbnail or lucide type icon, emphasised filename, human size · uploader · date meta line, Download)
  - humanFileSize / humanDate / isPreviewableImage pure helpers in api/client.ts
  - ImagePreviewDialog (full-size inline image preview via <img src> only, focus-trapped)
  - TestInlineImageDisposition pinning ATT-04 (png/jpg/svg -> inline + real Content-Type + nosniff)
  - pixel.jpg / pixel.svg test fixtures
affects: [02-03 extraction-status chip on the card, 02-04 replace/remove icon actions on the card]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Image preview is rendered via <img src={downloadAttachmentUrl}> only — never dangerouslySetInnerHTML / inlined markup — so an uploaded SVG cannot execute script (ATT-04, mirrors MarkdownProse raw-HTML-off)"
    - "Previewable-image detection (isPreviewableImage) mirrors the server isInlineImage allow-list (png/jpeg/svg) so client and server agree on which types are images"
    - "Card media is a fixed 64x64 square (a media dimension, not spacing); thumbnail is a chromeless button that opens the focus-trapped Dialog"

key-files:
  created:
    - web/src/components/attachments/ImagePreviewDialog.tsx
    - web/src/components/attachments/ImagePreviewDialog.css
    - testdata/attachments/pixel.jpg
    - testdata/attachments/pixel.svg
  modified:
    - web/src/api/client.ts (humanFileSize, humanDate, isPreviewableImage helpers)
    - web/src/components/attachments/AttachmentCard.tsx (full UI-SPEC card + preview wiring)
    - web/src/components/attachments/AttachmentCard.css (media square, icon placeholder, meta line, shadow-card)
    - internal/server/handlers_attachments_test.go (TestInlineImageDisposition)

key-decisions:
  - "humanFileSize uses the DECIMAL (SI, 1000-based) convention so '1.4 MB' reads as the UI-SPEC shows and matches OS file managers; documented in the helper"
  - "handlers_attachments.go was NOT modified — the SEC-02 inline branch and isInlineImage (exactly image/png, image/jpeg, image/svg+xml) already existed from 02-01; this slice pins ATT-04 with a dedicated test rather than changing behaviour"
  - "Thumbnail is a chromeless <button> (not a clickable <img>) so keyboard users get a real focusable control with an accessible label ('Preview {filename}')"

patterns-established:
  - "Pattern: local open/close state on the card (useState) mirrors DeleteConfirmDialog's open-state pattern; the dialog only mounts for previewable image types"
  - "Pattern: hidden-Git copywriting continues — the card shows original filename + human size/date/uploader only; the opaque on-disk id and any version-control vocabulary never surface"

requirements-completed: [ATT-03, ATT-04]

# Metrics
duration: ~20min
completed: 2026-06-21
---

# Phase 2 Plan 02: Full Attachment Card + Inline Image Preview Summary

**Turned the minimal 02-01 filename card into the full ATT-03 attachment card (64×64 image thumbnail or lucide type icon, emphasised filename, human `size · uploader · date` meta line, Download) and added ATT-04 inline image preview: clicking an image thumbnail opens a focus-trapped full-size Dialog that renders the image via `<img src>` only, so an uploaded SVG cannot execute script — pinned by a dedicated inline-disposition test.**

## Performance
- **Duration:** ~20 min
- **Completed:** 2026-06-21
- **Tasks:** 2 (both type=auto)
- **Files created/modified:** 8

## Accomplishments
- ATT-03: every attachment now renders as the full UI-SPEC card — a 64×64 media square (image thumbnail for png/jpg/svg via `<img src>`, or a `--color-surface` square with a centered lucide type icon — `FileText` for pdf/docx/txt, generic file icon otherwise), the original filename at Body/600 with ellipsis truncation + full name in `title`, and a muted Label-size `{humanFileSize} · {uploader} · {humanDate}` meta line.
- Added three pure presentation helpers to `api/client.ts`: `humanFileSize` (decimal/SI, "1.4 MB"), `humanDate` (`toLocaleDateString`, "21 Jun 2026" — never a raw timestamp/SHA), and `isPreviewableImage` (mirrors the server allow-list).
- ATT-04: `ImagePreviewDialog` reuses the existing focus-trapped `Dialog` (Esc/backdrop close, backdrop never mutates) to show a full-size preview rendered via `<img src>` ONLY — no `dangerouslySetInnerHTML`, no inlined markup — so an SVG is consumed as an image resource and cannot run script (stored-XSS guard, paired with the server's `nosniff`).
- Pinned ATT-04 with `TestInlineImageDisposition`: png/jpg/svg each download as `Content-Disposition: inline` + their real image `Content-Type` + `X-Content-Type-Options: nosniff`. Added `pixel.jpg` / `pixel.svg` fixtures (verified to sniff to image/jpeg and image/svg+xml).
- Hidden-Git preserved: the opaque on-disk id and all version-control vocabulary stay out of the rendered card.

## Task Commits
1. **Task 1: Full attachment card + formatting helpers (ATT-03)** — `788a66e` (feat)
2. **Task 2: Inline image preview dialog + ATT-04 disposition test** — `9be7de0` (feat)

## Files Created/Modified
See frontmatter `key-files`. Highlights:
- `web/src/components/attachments/AttachmentCard.tsx` — full card: media square (thumbnail/icon), filename, meta line, Download, preview wiring.
- `web/src/components/attachments/ImagePreviewDialog.tsx` / `.css` — full-size `<img src>` preview in the focus-trapped Dialog.
- `web/src/api/client.ts` — `humanFileSize` / `humanDate` / `isPreviewableImage`.
- `internal/server/handlers_attachments_test.go` — `TestInlineImageDisposition` (ATT-04).

## Decisions Made
See frontmatter `key-decisions`. No architectural (Rule 4) changes were required.

## Deviations from Plan
None — plan executed as written. The only judgement call was leaving `internal/server/handlers_attachments.go` unmodified: the SEC-02 inline branch and the exact `isInlineImage` allow-list (image/png, image/jpeg, image/svg+xml) already existed from 02-01, so the plan's "confirm the inline-image branch" was satisfied by adding `TestInlineImageDisposition` rather than editing the handler. This matches the plan's own task wording ("the disposition logic already exists from 02-01; this test pins ATT-04 explicitly").

## Known Stubs
None. The card is wired entirely from the live `["attachments", pagePath]` react-query data; no hardcoded/placeholder content. The extraction-status chip and replace/remove icon actions are intentionally NOT in this slice (02-03 and 02-04 respectively, per the phase plan).

## Issues Encountered
- The repo shipped only `pixel.png` as an image fixture; ATT-04 needs jpg + svg too. Generated `pixel.jpg` (ImageMagick `convert -size 1x1 xc:black`) and a minimal valid `pixel.svg`, then verified both sniff to image/jpeg and image/svg+xml via `mimetype.Detect` before relying on them. Both are within the test harness's allow-list (pdf,docx,txt,png,jpg,svg).
- The `dangerouslySetInnerHTML` safety grep matches a single line — the comment in `ImagePreviewDialog.tsx` documenting that it is deliberately NOT used. There is no actual usage (`grep ... | grep -v "//"` returns nothing), so the stored-XSS guard holds.

## Verification
- `go test ./internal/server/ -run TestInlineImageDisposition -count=1` → ok (ATT-04).
- `cd web && npm run build && npx tsc --noEmit` → both pass.
- `go build ./...` + `go test ./...` → all packages green.
- No-Git-vocab grep on `AttachmentCard.tsx` → `NO-GIT-VOCAB`.
- `grep -rn "dangerouslySetInnerHTML" web/src/components/attachments/` → comment only (no real usage).

## User Setup Required
None — no external service or configuration changes.

## Next Phase Readiness
- 02-03 (extraction status + SSE): the card layout reserves the meta stack; an `ExtractionStatus` chip can be dropped under the meta line (`attachment-card-main`) without restructuring. `extraction_status` is already on `AttachmentMeta`.
- 02-04 (replace/remove/orphan-delete): the card's right side currently hosts Download; the icon-only Replace/Remove actions (editor-gated, 44px targets) slot beside it per UI-SPEC.

## Self-Check: PASSED
All created files exist on disk and both task commits (`788a66e`, `9be7de0`) are present in git history.

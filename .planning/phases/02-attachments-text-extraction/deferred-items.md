# Deferred Items — Phase 02

## Out-of-scope lint findings (not introduced by 02-04)

- **`react-hooks/static-components` in `web/src/components/attachments/AttachmentCard.tsx`**
  - Line: `const Icon = typeIconFor(meta);` (pre-existing from 02-02, unchanged by 02-04).
  - ESLint flags creating a component reference during render. The project build
    (`tsc -b && vite build`) does NOT run ESLint, and the 02-04 verify command
    (`npm run build && npx tsc --noEmit`) passes. The line predates this plan and
    is unrelated to the replace/remove work, so it is left untouched per the
    executor SCOPE BOUNDARY rule.
  - Suggested fix (future): assign the icon component to a capitalized variable
    outside render, or render `typeIconFor(meta)` inline with `createElement`.

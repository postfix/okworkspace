---
status: passed
phase: 11-per-page-llm-tag-suggestion
source: [11-VERIFICATION.md]
started: 2026-06-24T14:00:00Z
updated: 2026-06-24T14:02:00Z
---

## Current Test

number: 1
name: Live LLM tag suggest → approve → byte-stable apply
expected: |
  With a real LLM key, "Suggest tags" returns sensible normalized tags (≤5);
  approving writes them into the page's YAML frontmatter byte-stably (body + other
  fields unchanged) via the single-writer commit; stale revision 409s.
awaiting: none — passed

## Tests

### 1. Live LLM tag suggest → approve → apply
expected: Real model returns ≤5 normalized tags; apply writes tags-only block-style, body intact, commits.
result: passed — Live on :8098 with real deepseek-v4-flash. suggest-tags → 200 (deployment/build/test/server/binary); apply-tags → 204; deploy.md frontmatter now has block-style tags with type/title/description + timestamp intact; hidden-Git commit landed (data/repo 090e098). Gap found+fixed live (a15a0dc): parser now tolerates real-model prose/fence/object-wrapped arrays. apply-409 + 5-tool boundary + byte-stable golden all unit-proven.

## Summary

total: 1
passed: 1
issues: 0
pending: 0
skipped: 0

## Gaps

One gap found during live validation (real-model reply wrapping caused 422) — FIXED in a15a0dc and re-verified live (200 + applied). No open gaps.

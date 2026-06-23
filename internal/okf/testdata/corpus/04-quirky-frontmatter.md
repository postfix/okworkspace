---
# A YAML comment that MUST survive the round-trip.
type: Page
title: "Quirky: a title with a colon and quotes"
description: 'single-quoted scalar with #hash inside'
tags:
  - "quoted-tag"
  - plain
timestamp: 2026-06-18T10:15:00Z
custom_field: preserved
nested:
  a: 1
  b: 2
---

# Body

Frontmatter above has a comment, unusually-quoted scalars, and extra fields
that all must be preserved verbatim.

---
type: Page
title: Code Block Containing a Fence
description: The body has a fenced code block whose contents include a literal --- line and a key value line.
tags: []
timestamp: 2026-06-18T10:05:00Z
---

# Tricky Body

The following code block contains lines that LOOK like frontmatter but are body:

```yaml
---
type: NotFrontmatter
title: This is inside a code block
description: parsers must not treat this as a fence
---
```

And an inline `key: value` outside a block should also stay body text.

key: value

Done.

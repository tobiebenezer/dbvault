---
session: ses_fd11
updated: 2026-08-23T14:20:09.526Z
---

The doctor fixture needs an actual `secret_providers:` entry (`file` driver) that repositories can reference — empty `key_provider` falls back to `"local-files"` internally. And I need the full prometheus/webhook test output. Investigating both:

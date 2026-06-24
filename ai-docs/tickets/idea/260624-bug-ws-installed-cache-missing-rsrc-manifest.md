---
title: ws installed cache missing rsrc manifest
related:
  260525-bug-codex-local-marketplace-worktree-cache-regression: adjacent installed-cache / local plugin refresh behavior
related-mental-model:
  - plugin-runtime
---

# ws installed cache missing rsrc manifest

## Background

During a dashboard `ws:lead-discuss` session on 2026-06-24, the installed
Codex plugin cache at
`/home/swkang/.codex/plugins/cache/kang-sw-devenv/ws/0.30.2/` had no
`rsrc/manifest.json`. `ws/playbook.print(name: "lead-discuss")` and
`ws/playbook.print(name: "lead-workflow-manual")` returned a manifest-missing
error, and `ws.mercenary.register` failed while trying to load delegate
orientation from the same missing manifest.

The session recovered by using the already-loaded playbook context and
host-native multi-agent tooling, but this installed-cache state breaks normal
ws skill execution and managed delegation. Investigate whether this is a plugin
cache refresh/regeneration failure, local marketplace/worktree cache regression,
or a runtime fallback bug when the manifest is absent.

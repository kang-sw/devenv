---
title: ws setup cwd literal can bind to plugin cache root
related:
  260505-lead-skill-namespace-surface: workflow manual documents setup bootstrap primitives
---

# ws setup cwd literal can bind to plugin cache root

## Background

During a lead-discuss dogfood run, `ws/setup(method: "lead-workflow-bootstrap", root: "<cwd>")` failed because the runtime resolved the literal cwd placeholder to the installed plugin cache path instead of the caller's repository root:

```text
root "/Users/kang-sw/.codex/plugins/cache/kang-sw-devenv/ws/0.28.1" is not inside a Git worktree
```

Passing the absolute repository path `/Users/kang-sw/devenv` succeeded.

The workflow manual says callers may pass `"<cwd>"` or an absolute repository path. The behavior is surprising when the MCP server process root differs from the active workspace root, especially in installed-plugin sessions.

## Questions

- Should `ws/setup(root: "<cwd>")` resolve against the host conversation workspace, the MCP session default root, or the server process root?
- Should installed-plugin sessions reject `"<cwd>"` with clearer guidance when the server cannot know the caller workspace?
- Should lead workflow skills prefer absolute roots in examples until the runtime can resolve `"<cwd>"` reliably across hosts?

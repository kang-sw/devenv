---
title: ws setup cwd literal can bind to plugin cache root
related:
  260505-lead-skill-namespace-surface: workflow manual documents setup bootstrap primitives
---

# ws setup cwd literal can bind to plugin cache root

## Disposition (2026-06-19): dropped, the bug surface no longer exists

M3 has landed. The `ws/setup` tool — the exact surface this bug lived on — was
removed (`24569308`, "remove setup tool dispatch and schema"); a non-test source
grep for `ws.setup` now returns zero hits. The session-auth contract (`ws.ferrule`
mints a word-chain key against an explicit `root`, mandatory per-call keys, no
ambient cwd auto-derivation) replaced it. The closing condition stated below ("Drop
or close it when M3 lands the session-auth contract") is satisfied. Dropped as
resolved-by-deletion; retained here only as historical design input to M3.

## Superseded-by-redesign (2026-06-09): folded into M3 session-auth

Do not fix this in isolation. `260605-epic-ws-playbook-factory-pivot` (M3)
replaces `ws/setup` with a `login`-style session-auth model that returns a
word-chain session key and takes an explicit root, with mandatory per-call keys
and no ambient cwd auto-derivation. That redesign reworks the exact surface this
bug lives on, so this ticket is retained as **design input to M3** (a concrete
failure the new contract must not reproduce), not as a standalone fix. Drop or
close it when M3 lands the session-auth contract.

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

---
title: Subquery working-directory stderr during dogfood runs
related:
  260524-bug-subquery-non-head-history-evidence: adjacent subquery evidence reliability issue
  260524-mcp-actor-setup-bootstrap: recent setup/root recovery context
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# Subquery working-directory stderr during dogfood runs

## Pending Removal (2026-06-09)

Resolved-by-deletion candidate under `260605-epic-ws-playbook-factory-pivot`.
`ws/subquery` is absorbed into the harness-native Explore agent and removed at
milestone M2, so this subquery working-directory stderr bug ceases to exist. Do
not invest in a standalone fix; drop this ticket to `.dropped/` in the same
commit that removes subquery. Retained in place until then so git blame and
ticket scans surface the coupling.

## Background

During a `ws:lead-discuss` dogfood run on 2026-05-24, a delegated subquery
emitted repeated shell stderr lines like `getcwd: cannot access parent
directories: No such file or directory` and `job-working-directory: error
retrieving current directory` while inspecting the repository. The parent
discussion could proceed, and the subquery recovered enough context through
explicit `root` arguments, but the stderr indicates that a child process can
inherit or start from a deleted or otherwise inaccessible current working
directory.

The expected behavior is that delegated agents and their shell calls start from
a stable, explicit working directory derived from the resolved ws root or from a
known runtime-safe fallback, rather than relying on process cwd after plugin
cache refreshes, stale launcher paths, or deleted temporary directories.

## Investigation Notes

- Reproduce with a subquery that runs shell commands after actor/setup recovery.
- Check whether the backend process starts before or after `ws.setup(root: ...)`
  has established a usable root.
- Verify whether Codex plugin cache refresh, ws named-agent resume, or nested
  subquery execution can leave the worker process cwd pointing at a removed
  directory.
- Preserve explicit-root tool behavior while fixing process-level cwd for
  shell-backed child work.

---
title: Playbook render uses stale plugin cache during source dogfood
sage-review-design: blocked
sage-review-completeness: required
related:
  260612-bug-ws-rsrc-dev-server-new-file-staleness: nearly identical symptom shape (manifest/text changes not visible until restart) — may share root cause
---

# Playbook render uses stale plugin cache during source dogfood

## Background

During Phase 2 dogfooding for
`260627-feat-lead-implement-rendered-delegate-prompts`, the source tree already
contained the Phase 1 `implementer` render variables (`BriefPath`, `PlanPath`,
`VerificationHint`, `ResultExpectations`, `CommitRangeHint`). Calling
`ws/playbook.render(name: "implementer", context: ...)` through the installed MCP
runtime still failed with `variable "PlanPath" is not declared in playbook`.

The installed plugin cache at
`~/.codex/plugins/cache/kang-sw-devenv/ws/0.30.13/rsrc/implementer/implementer.md`
was stale and only declared `RoleModel`.

## Impact

Source-branch dogfood of playbook changes cannot reliably use MCP render output
until the plugin cache is refreshed. That makes follow-up phases awkward because
the workflow wants to render delegate prompts from the branch-local source tree
before the branch is merged or reinstalled.

## Research Direction

Investigate a branch-local rsrc render path for development workflows, or add a
clear dogfood/runbook step that refreshes the plugin cache before using MCP
rendered prompts from just-edited rsrc files.

## Blocked (2026-07-13)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Ticket omits the existing `WS_RSRC_ROOT` dev affordance (`ai-docs/spec/mcp-tools.md {#260609-rsrc-playbook-distribution}`) that already does what Research Direction #1 proposes investigating; no account of whether it was set during the reported dogfood session | critical | missing |
| 2 | Two divergent solution directions (branch-local render path vs. runbook step) left undecided with no criteria to choose between them, and the cheaper one may already be moot given issue 1 | important | missing |

Before this can proceed: confirm whether `WS_RSRC_ROOT` was set during the
2026-07-03 dogfood session that reported this, and determine whether this is
the same defect as `260612-bug-ws-rsrc-dev-server-new-file-staleness` (dev MCP
server caches its in-process rsrc file set at startup independent of manifest
re-reads) rather than a separate stale-plugin-cache issue.

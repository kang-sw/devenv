---
title: Playbook render uses stale plugin cache during source dogfood
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

---
title: Investigate unavailable tools in Pi delegated sessions
related:
  260907-bug-ws-pi-fork-first-call-prompt-cache-miss: observed during documentation closeout; causal relationship unconfirmed
---

# Investigate unavailable tools in Pi delegated sessions

## Background

During fork-cache documentation closeout on 2026-09-08, a lateral `ws-fork` child reported only edit/write/parallel tools, with no read, shell or `ws-report-to-lead`. It could neither inspect documentation nor emit the required completion report. Repeated requests to report produced another incomplete-run advisory rather than completion. It reported no repository changes.

A replacement ordinary spawned delegate had read/shell/edit/write but reported no ws tools, preventing the explicitly requested ws commit. Its implementer prompt also required plan-local authorization before reading the ticket. Work resumed only after the lead supplied that authorization and retained ws verification/commit responsibility.

These are observed child reports, not a verified root-cause diagnosis. The installed adapter may differ from reviewed working-tree code; do not infer a regression in that code from this incident alone. Distinguish missing callable tools from prompt-level restrictions and from missing tool exposure in inherited context.

## Phases

### Phase 1: Reproduce and resolve delegated tool availability mismatch

Record installed adapter/runtime versions, launch mode, actual child registrations and active tools, and effective delegate constraints for both lateral forks and ordinary spawned agents. Determine whether each missing tool is intentional role policy, a launch/registration failure, or a prompt/runtime mismatch. Reproduce without paid provider requests where possible.

Ensure supported delegated tasks have the tools their contract requires, or surface an actionable limitation before dispatch. Verify completion reporting for forks and a documented lead-owned alternative for deliberately unavailable ws operations. Do not broaden role capabilities or alter fork cache-prefix guarantees without a separately reviewed decision.

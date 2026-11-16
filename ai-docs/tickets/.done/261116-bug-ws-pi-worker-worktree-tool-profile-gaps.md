---
title: "Pi worker profile omits tools needed beside worktree acquisition"
completed: 2026-11-16
related:
  261116-bug-ws-exe-launch-elimination: "Phase 1 item 2 owns tickets.create_empty exposure and executable-fallback elimination; do not duplicate that change"
  261116-feat-ws-subagent-worktree-lease: "owns scoped acquire/release permissions; this ticket covers adjacent worker primitives"
---

# Pi worker profile omits tools needed beside worktree acquisition

## Background

Dogfood #17, 2026-11-16: the Pi worker tool profile lacks
`git.resolve_impl_branch` and `tickets.create_empty` alongside the worktree
workflow it is asked to follow. This worker directly encountered both missing
tools in its exposed inventory; the 10:07 implementation assignment stopped
because `worktree.acquire` and `git.resolve_impl_branch` were unavailable, and
the lead subsequently acquired the worktree. Missing creation access also
caused the earlier executable-CLI fallback captured by the related exe ticket.

That ticket's Phase 1 item 2 already adds `tickets.create_empty` and audits tool
omissions. **Do not reimplement that addition or any exe-launch lifecycle work.**
Scope this ticket to `git.resolve_impl_branch` and the remaining routinely needed
worker primitives, coordinating the audit with that producer and the separate
acquire/release permission ticket.

## Decisions

Compatibility target: **T0**, additive exposure of existing authorized tools,
with current schemas, scope/depth ceilings and key/worktree binding preserved.
This does not authorize exposing every lead-only tool to workers.

Inventory audit (2026-11-16):
- Add `git.resolve_impl_branch` to `pi-worker`; the inherited mini-lead
  inventory also needs the same branch-identity primitive. It is already
  registered and delegate-authorized; no resolver or authorization change.
- Add `tickets.verify` to `pi-worker` for pre-commit ticket guardrails. Mini-lead
  already exposed it; inherit it once instead of duplicating its entry.
- Keep `tickets.create_empty` from `74924b4c0` unchanged. Ticket discovery,
  acquire/release, move/close, route/checklist, context reads and review rendering
  remain mini-lead capabilities, not an expansion of the ordinary coding worker.
- The rendered ticket-worker procedure requires those mini-lead capabilities;
  use that profile for a worker that owns the full ticket lifecycle. Ordinary
  implementation/review workers need only the bounded additions above.
- `worktree.acquire`/`release` remain outside both inventories and retain the
  existing lead-scope gate. Their scoped worker permissions belong to
  `261116-feat-ws-subagent-worktree-lease`. The occupied-index move refusal belongs
  to `261116-bug-ws-tickets-move-refuses-own-impl-worker`.
- The reported verification failure supplied the original-checkout session key,
  not the housekeeping worker's bound key. `resolveToolRoot` already selects the
  supplied key's root for verify/query/create/move/close; the Pi bridge forwards
  explicit keys unchanged. Preserve this authority instead of falling back to
  server cwd or adding a root override. Cross-worktree dispatch regressions
  cover valid worker keys, the original key, unknown keys and rejected overrides.
  No production routing defect was reproduced.

## Phases

### Phase 1: Complete the bounded worker worktree-tool inventory

Compare the worker's rendered workflow requirements with its actual profile,
registration and dispatch permissions. Which adjacent primitives are still
missing after the related create_empty/acquire-release changes? Restore the
required supported subset, with explicit justification for each addition.
Verify an authorized worker can resolve an implementation branch and complete
its assigned worktree/ticket flow through native tools, while unrelated lead
operations and privilege escalation remain refused. No executable fallback,
profile implementation or new permissions are introduced by this capture.

### Result (a995e895f) - 2026-11-16

Added the existing branch resolver and ticket preflight verifier to `pi-worker`;
mini-lead inherits both, with one verifier entry. Updated the runbook inventory.
No create-empty, resolver, routing, session schema, or authorization changes.

Verification on remote w5090 through `D:/sys_lock.sh remote ... run --sync`:
- Targeted profile/launch/snapshot/verification/session-binding tests passed.
- Full `go test ./internal/wsagent ./internal/mcp ./internal/wsdoc -count=1`
  passed (74.004s, 190.257s, 1.351s respectively); remote gofmt checks passed.
- Cross-worktree dispatch checks cover worker and mini-lead allowlists with
  child-lead and delegate keys: verify/create use the bound linked worktree;
  mini-lead query does too. The original key checks the original checkout,
  unknown keys cannot fall back, and explicit root overrides are rejected.
- Launch-surface tests expose the additions to Pi and MCP. Existing launched
  parent snapshots cannot gain new grants; unrelated lead operations and
  delegate lease escalation remain refused. `git diff --check` passed.
- Independent R1: no Critical/Important findings; one Minor for stale runbook
  inventory. Fixed, and fixes-only R2 confirmed resolution. Review used the
  read-only Pi profile because the running parent snapshot could not authorize
  the newer ordinary worker's existing `tickets.create_empty` grant.

No local Go, test/ws executable or launcher ran. No daemon rebuild/restart,
profile override, or push. The observed wrong-root call was a caller-key mismatch,
not a reproduced production dispatch defect. Separate worktree lease and
occupied-index move policy remain with their related tickets.

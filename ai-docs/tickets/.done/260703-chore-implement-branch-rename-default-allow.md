---
title: "Default-allow branch rename in lead-implement's enter.implement branch plan"
sage-review: completed
completed: 2026-07-07
---

# Default-allow branch rename in lead-implement's enter.implement branch plan

## Background

The user's original ask referenced "proceed 스킬" but the actual mechanism
lives in `lead-implement`, not `lead-proceed`: `lead-proceed` has no
branch-policy concept at all. Branch rename is `enter.implement`'s
`policy.branch.allow_rename` fact, consumed by
`deriveImplementBranchPlan` in `agents-plugin-tool/internal/mcp/implement_resolver.go:600-641`.

That function only reaches the rename branch when the lead is **already on
an `implement/*` branch** whose name doesn't match the newly-resolved
target scope slug (fresh-branch creation from a non-`implement/*` branch is
a separate `action: "create"` path, unaffected by this policy). Today,
`AllowRename != "yes"` (i.e. unset/`unknown`/`no`) stops with "current
implementation branch differs from target scope and rename is not
allowed" — forcing the lead to explicitly ask the user and set
`policy.branch.allow_rename=yes` before a rename verdict is possible. The
`lead-implement` playbook's Policy rules bullet documents this: "Set
`policy.branch.allow_rename=yes` only when the caller accepts pre-edit
branch rename."

Critically, the resolver **already has safety guardrails on the rename
path itself**, independent of the `AllowRename` flag
(`implement_resolver.go:627-636`): it stops if the target branch name
already exists (`TargetExists`), or if the current branch has upstream
tracking / ahead / behind state (`Upstream != "" || Ahead != 0 || Behind !=
0`) — "rename is ambiguous". So the existing implementation already
mirrors the kind of skip-condition discipline used by `lead-implement`'s
Branch Cleanup step (§8: skip deletion if checked out, worktree-linked, or
ambiguous merge target) — this ticket does not need to invent a new
skip-condition set, only decide whether the *default value* of the
`AllowRename` input should flip.

## Decisions

- **Default `policy.branch.allow_rename` to `yes`** when the lead has no
  explicit signal to the contrary, instead of requiring an explicit
  per-invocation user accept before a rename verdict is reachable. The
  existing `TargetExists` / `Upstream`/`Ahead`/`Behind` guardrails in
  `deriveImplementBranchPlan` remain the safety net and are not changed by
  this ticket — they already stop the rename and fall back to `action:
  "stop"` when the branch is ambiguous or ahead/behind of an upstream.
  Because those guardrails are structural rather than confirmation-based,
  the risk profile of removing the confirmation step is limited to: silent
  local branch renames on branches with no upstream state and no naming
  collision, which is a low-risk, easily-reversible git operation.
- Where to change the default is an implementation-time question: either
  in the `lead-implement` playbook's Policy rules (change the rule to "set
  `yes` unless the caller has explicitly withheld consent") or as a
  resolver-side default when the fact is `unknown`/absent
  (`implement_resolver.go` normalization, `factOr(policy.Branch.AllowRename,
  "unknown")` at line ~510) — whichever keeps the deterministic-facts
  discipline `enter.implement` otherwise follows (prefer the resolver-side
  default if it doesn't collapse the `unknown` state's meaning for other
  future consumers of that fact).
- Out of scope: any change to the `TargetExists` / `Upstream`/`Ahead`/`Behind`
  guardrail logic itself, and any change to the fresh-branch `action:
  "create"` path (unaffected by `AllowRename`).

## Phases

### Phase 1: Flip the branch-rename default and update the playbook rule

- Survey `implement_resolver.go`'s fact-normalization path to confirm the
  best place to default `AllowRename` to `yes` (resolver-side vs.
  playbook-prose-side) without breaking existing `unknown`-state tests
  (`implement_resolver_test.go`).
- Update the `lead-implement` playbook's Policy rules bullet to describe
  the new default-allow behavior and when a lead should still withhold
  consent (e.g. explicit user request to keep the current branch name).
- Update or add tests in `implement_resolver_test.go` /
  `session_state_test.go` covering: default-allow now reaches a `rename`
  verdict when no explicit policy is given and no guardrail trips; explicit
  `allow_rename: "no"` still stops.
- Update the affected `enter.implement` spec entry (branch-plan default
  posture) alongside the code change; this phase is not complete until
  that spec update lands.

### Result (5f442651)

Flipped `normalizeImplementFacts`'s `AllowRename` fallback from `unknown` to
`yes` (single consumer, confirmed via survey); an explicit `no` still stops
the rename, unaffected. Updated `lead-implement`'s Policy rules bullet, the
`allow_rename` tool-schema description, and the `enter.implement` spec entry
in `ai-docs/spec/mcp-tools.md` to describe the new default-yes posture.
Wsflow rsrc mirror and manifest hashes regenerated and confirmed
byte-identical to canonical. Correctness/fit/test review: all three
partitions clean on first pass, no fix cycle needed.

## Spec Impact

`enter.implement`'s branch-plan behavior is documented caller-visible
contract surface (used by `lead-implement`); changing the default posture
needs spec addressing as part of Phase 1 (see the added spec-update bullet
above), not deferred past it. Contract-first spec: no — this is a
default-value change to existing documented resolver behavior, not a new
planned contract.

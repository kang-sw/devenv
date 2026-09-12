---
title: "Clarify the ws/git.merge boundary for epic-to-develop landing"
related:
  260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming: established the impl-only merge authority now exposed by ws/git.merge
  260909-epic-ws-worker-interpreter-refoundation: dogfood source; its epic/refound branch needed promotion to develop
---

# Clarify the ws/git.merge boundary for epic-to-develop landing

## Background

During the refoundation landing on 2026-09-12, the user asked to test the
reloaded `ws/git.merge` tool for the `epic/refound` to `develop` merge. The lead
incorrectly interpreted the request as permission to run native
`git merge --no-ff`, then discovered that the MCP tool rejects the requested
source branch before it can inspect or merge it.

The incident exposes a gap between the generic tool name and its impl-only
contract at a real repository integration boundary. The native merge succeeded
locally, so this ticket is follow-up research rather than a blocker for that
landing.

## Observed boundary

The reloaded tool schema describes `ws/git.merge` as merging a local
`impl/<root>/<stem>` branch into its encoded root. Calling it with
`branch: "epic/refound"` and `target: "develop"` returned:

```text
git.merge requires impl/<root>/<stem>, got "epic/refound"
```

The release-target acknowledgement added for `main` and `master` remains scoped
to `impl/main/*` or `impl/master/*`; it does not make `ws/git.merge` a general
branch-promotion primitive. The existing lifecycle research intentionally
centralized only impl-branch merges and therefore does not settle the
epic-to-review-track boundary.

## Outcome Ledger

### Verified Findings
<!-- Evidence-backed observations. These may support later tickets but do not choose behavior. -->

- `ws/git.merge` rejects `epic/refound` because its source contract requires
  `impl/<root>/<stem>`.
- The new release-target diagnostics and acknowledgement fields apply only after
  an impl branch encodes `main` or `master` as its root.
- The current `lead-run` playbook gives native Git an explicit exception for
  goal-to-parent promotion, but it does not define an epic-to-develop terminal.
- The refoundation epic was merged into local `develop` with native
  `git merge --no-ff` before this contract mismatch was recognized.

### Confirmed Decisions
<!-- Normative choices explicitly confirmed by the user. -->

- Generalize `ws/git.merge` to accept any exact local source branch and exact
  local target branch. Leads should not need to classify a branch before
  deciding whether the merge tool is callable.
- Preserve the impl lifecycle invariant: when the source is `impl/*`, its
  explicit target must match the root encoded in the source branch name.
- Keep the existing lead-only authority, `--no-ff` merge, structured merge
  record, exact-ref checks, clean-state checks, merge-in-progress refusal,
  containment checks, source and target OID rechecks, and conflict handoff.
- Keep the default-deny `main` and `master` policy, structured diagnostics, and
  OID-bound `release_target_override`; the override never waives a
  `must_resolve` finding.
- Delete `impl/*` and `goal/*` source branches only after a successful merge.
  Goal branches are convention-managed one-shot branches. Preserve `epic/*`
  and other source branches after a successful merge.
- Route goal-to-parent, epic-to-review-track, and other lead-owned branch
  integrations through `ws/git.merge` instead of branch-type-specific native
  Git fallbacks.

### Proposals
<!-- Unconfirmed candidates. Never treat these as actionable authority. -->

### Open Questions
<!-- Unresolved choices that require further investigation or user input. -->

### Rejected Alternatives
<!-- Alternatives explicitly rejected, with the reason when useful. -->

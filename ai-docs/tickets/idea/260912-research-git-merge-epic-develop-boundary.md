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

### Proposals
<!-- Unconfirmed candidates. Never treat these as actionable authority. -->

### Open Questions
<!-- Unresolved choices that require further investigation or user input. -->

- Should `ws/git.merge` accept additional structured lifecycle sources such as
  `epic/*`, or should epic-to-review-track promotion use a distinct lead-owned
  primitive?
- If the boundary stays impl-only, where should the supported epic-to-develop
  path be made explicit so a caller does not infer a generic merge surface from
  the tool name?
- Which existing safety checks, OID pinning, review evidence, and branch cleanup
  behavior should apply to a non-impl promotion path?

### Rejected Alternatives
<!-- Alternatives explicitly rejected, with the reason when useful. -->

---
title: "Make git.merge explain and safely acknowledge release-target policy"
---

# Make git.merge explain and safely acknowledge release-target policy

## Background

Downstream dogfood reached an `impl/main/<stem>` or `impl/master/<stem>`
handoff and `git.merge` refused the encoded target without explaining the
repository-policy reason or providing a supported resolution path. The route
resolver can create those branches in a trunk-only repository, while the merge
tool classifies both target names as release-class unconditionally.

The current behavior applies this repository's `develop` to `main` release
topology to downstream repositories whose ordinary integration branch may be
`main` or `master`.

## Outcome Ledger

### Verified Findings
<!-- Evidence-backed observations. These may support later tickets but do not choose behavior. -->

- `route.resolve_implement` may create `impl/main/<stem>` or
  `impl/master/<stem>` from ordinary work on those branches.
- `git.merge` accepts only `impl/<root>/<stem>`, derives the target from
  `<root>`, and rejects `main` and `master` before its ref, worktree, and merge
  safety checks.
- The refusal reports only `forbidden merge target <target>`; it does not name
  the release-policy premise, inspect review state, or prescribe a resolution.
- Existing review metadata can identify a clearing frontier and whether a
  candidate contains commits beyond it, but it cannot prove review quality or
  enumerate unresolved findings.
- Existing Git safety checks cover ref validity, clean state, an existing merge,
  containment, checkout identity, and conflict preservation independently of
  the release-target policy.

### Confirmed Decisions
<!-- Normative choices explicitly confirmed by the user. -->

### Proposals
<!-- Unconfirmed candidates. Never treat these as actionable authority. -->

- Replace the unconditional name-based refusal with a structured diagnostic
  that reports the current policy reasons, review evidence available to the
  tool, and the actions that can resolve each reason.
- Add an explicit retry input, defaulting to refusal, that acknowledges the
  release-class target after the caller has seen those diagnostics.
- Keep Git integrity and worktree safety failures non-waivable; scope any
  acknowledgement to the release-target policy only.
- Bind an acknowledgement to the inspected source and target tips, and
  optionally the review-policy snapshot, so a changed merge candidate requires
  fresh diagnostics.

### Open Questions
<!-- Unresolved choices that require further investigation or user input. -->

- Should the retry surface be a boolean such as `override: true`, a narrowly
  named release-target acknowledgement plus expected OIDs, or an opaque token
  returned by the refusal?
- Which review facts are blocking reasons versus advisory context when the MCP
  cannot infer whether a downstream `main` or `master` merge is an ordinary
  integration or a release?
- Can declared `review-track` and `release-boundary` posture distinguish a
  trunk-only integration target from a release target without importing a
  project-specific branch topology?

### Rejected Alternatives
<!-- Alternatives explicitly rejected, with the reason when useful. -->

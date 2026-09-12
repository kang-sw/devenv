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

- Replace the unconditional name-based refusal with a structured
  `policy_blocked` result that reports every current refusal reason and a
  resolution for each.
- Classify each diagnostic as `must_resolve` or `overrideable`. An override
  acknowledges only `overrideable` policy findings; ref validity, target
  identity, worktree state, an existing merge, containment, and changed
  inspected tips remain `must_resolve`.
- Use `release_target_override: false` as the default retry input. A true value
  requires the source and target OIDs returned by the refusal, and the tool
  rechecks them immediately before mutation.
- Include the available review frontier and candidate range in diagnostics.
  Describe an uncovered range as review evidence, not as proof that no review
  occurred or that findings remain unresolved.
- When a Git result cannot be parsed reliably into a structured diagnostic,
  include the command and bounded raw output on that diagnostic entry.
- Return actionable next steps: resolve every `must_resolve` item, satisfy an
  `overrideable` item through the indicated review or release procedure, or
  retry with the release-target acknowledgement and inspected OIDs.

### Proposals
<!-- Unconfirmed candidates. Never treat these as actionable authority. -->

### Open Questions
<!-- Unresolved choices that require further investigation or user input. -->

- Can declared `review-track` and `release-boundary` posture distinguish a
  trunk-only integration target from a release target without importing a
  project-specific branch topology?

### Rejected Alternatives
<!-- Alternatives explicitly rejected, with the reason when useful. -->

- A generic bare `override: true` is not bound to the candidate the caller
  inspected and has no stable meaning as more refusal classes are added.
- An opaque server-issued acknowledgement token adds state and machinery that
  source and target OID assertions provide without another lifecycle.

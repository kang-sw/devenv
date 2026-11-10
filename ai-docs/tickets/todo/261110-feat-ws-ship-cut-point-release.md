---
title: Ship a release from a review-cleared cut point while develop keeps moving
related:
  260824-epic-review-watermark-model: the review-watermark ledger and release gate this extends
---

# Ship a release from a review-cleared cut point while develop keeps moving

## Background

The release gate (`lead-ship` with `release-boundary: present`) reviews
`<frontier-head>..<review-track tip>` as one verdict. A single blocking merge at
the end of the range blocks the clean commits before it, and the ws ship
config's R4 re-assert aborts Publish whenever the review track moved while the
lead waited for approval. During fast development the project therefore has to
stop merging for the length of a release, even though the CI gate and the main
promotion already target one pinned SHA.

The ws 0.48.16 release showed the alternative works. The gate review was split
at the version-bump commit `fd82c26de`. The prefix `9e9f33b16..fd82c26de` was
clean and stamped `concern` (ledger commit `501296b90`). All blocking findings
were in the later daemon-hosted proxy merge. The release then shipped from a
`release/0.48.16` branch at `fd82c26de` (CI dispatched on that ref, main
fast-forwarded to it) while develop kept taking merges.

## Decisions

- D1 (user, board 668 a). Cut-point release becomes the general `lead-ship`
  protocol in the shipped playbook, and this project's
  `ai-docs/ship/ws.md` adopts it.
  A release ships a pinned commit at or before the review frontier, the *cut
  point*, not the review-track tip. The review track keeps taking merges during
  the release.
  Rejected: a ws.md-only change (downstream projects with fast development hit
  the same stall, and the gate rule lives in the shipped playbook); keeping
  this a one-off exception.
- D2 (user, board 669 a). The version bump goes on a release branch cut from
  the cut point. The release target (`main` here) fast-forwards to the release
  branch head, and the release branch is then merged back into the review
  track. That keeps the release target an ancestor of the review track, so the
  next release still fast-forwards.
  Rejected: forcing the bump commit to be the cut point. That means holding
  merges after the bump until the review ends, which is the same stall.
- D3 (user, board 670 ok). The gate review judges its range per first-parent
  merge segment and stamps the longest clean prefix as its own entry
  (`pass`/`concern`), then the blocking remainder as `block`. A block in one
  merge no longer holds back the clean commits before it.
  Rejected: one verdict over the whole range, which is today's behavior and
  the cause of the stall.
- D4 (user, board 671 ok). A cut-point release drops the tip re-assert (ws.md
  R4): it verifies the pinned SHA only. CI and the main promotion already
  target that SHA, so comparing the moving tip adds waiting and no safety.
  Rejected: keeping R4 as an abort on any tip movement.

## Constraints

- Shipped-surface boundary (`ai-docs/manuals/shipped-surface-boundary.md`).
  The `lead-ship` and `lead-review` playbook text stays project-neutral. Branch
  names (`develop`, `main`, `release/<version>`) and CI commands belong to the
  project ship config, not to the playbook. The playbook speaks of the review
  track, the release branch, and the config's steps.
- Skill authoring (`ai-docs/manuals/skill-authoring.md`) applies to every changed
  playbook line. wsflow mirroring (`ai-docs/manuals/wsflow-mirroring.md`) applies
  to the shared `agents-plugin/rsrc/` playbooks and the `rsrc/manifest.json`
  hashes.
- The review ledger stays append-only. A prefix stamp followed by a block stamp
  uses the existing `review.stamp` entries. No new verdict kind and no ledger
  format change.
- The gate never stamps (lead-ship rule 5 stands). Segment stamping happens
  inside `lead-review`'s range scenario.

## Phases

### Phase 1: Cut-point release in lead-ship, segment stamping in lead-review, ws.md adoption

- `agents-plugin/rsrc/lead-ship/` release gate and Execute:
  - The gate's cleared head is the frontier head after the gate's review. When
    that is behind the review-track tip (a blocked remainder, or merges after
    the review), the release may ship from a cut point at the frontier head
    instead of waiting. Offer it at the gate-decision stop. The publish
    confirmation names the cut point.
  - Execute step 3's "re-check the source branch tip; if it moved, abort" no
    longer applies to a cut-point release. The pinned cut point is what
    publishes.
  - The version bump: when the config's version strategy makes a bump commit,
    it lands on a release branch at the cut point (D2). The config names the
    merge back into the review track as a publish or post-ship step.
- `agents-plugin/rsrc/lead-review/` range scenario (D3): when the aggregate
  verdict is NEEDS FIX or BLOCKED-by-findings and the findings sit in a later
  first-parent segment, stamp `<base>..<last clean segment head>` with its
  clearing verdict first. Then stamp `<base>..<head>` or the remainder as the
  block the rules already give, in the order that keeps the frontier at the
  clean prefix. Check `review.marker`'s frontier semantics ("a trailing
  block/routed tail does not advance it") and state the order that works.
- `ai-docs/ship/ws.md`:
  - Pre-flight: the cut point replaces "develop tip" as the release commit when
    the gate cleared only a prefix.
  - Version bump on a `release/<version>` branch at the cut point. Drop the R4
    re-assert for cut-point releases (D4).
  - CI dispatch: `gh workflow run ws-mcp-ci.yml --ref release/<version>` with
    the headSha check against the cut point (or the bump commit on top of it).
  - Main promotion: `--ff-only` to the release head. Push main and the tag.
  - Post-ship: merge the release branch back into develop when it carries a
    bump commit, then push develop. Delete the remote release branch.
  - The `.github/workflows/ws-mcp-release.yml` error hint that says
    `--ref develop` mentions the release ref as well.
- Mirror and manifest updates per the wsflow-mirroring manual. Run the
  playbook render tests that cover `lead-ship` and `lead-review` remotely
  through `ws-rgo`.

Verification:
- Render tests for the changed playbooks pass. The shipped text names no
  devenv branch or command (the shipped-surface boundary tests stay green).
- `ws.md` steps can be followed by hand for a dry run against a scratch branch
  layout without contradicting the playbook: cut point, release branch, bump,
  ff, merge back.

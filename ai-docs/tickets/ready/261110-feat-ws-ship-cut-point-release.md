---
title: Ship a release from a review-cleared cut point while develop keeps moving
related:
  260824-epic-review-watermark-model: the review-watermark ledger and release gate this extends
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 00088ca9f60975ca
sage-review-completeness-reviewed: 00088ca9f60975ca
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
were in the later daemon-hosted proxy merge. The release then proceeded from a
`release/0.48.16` branch at `fd82c26de` while develop kept taking merges
(`origin/release/0.48.16` resolves to `fd82c26de`; at fact population,
2026-11-10, `main` and `origin/main` were still at `b701bc650`, the v0.48.15
bump, and `git ls-remote --tags origin` listed no `v0.48.16` tag, so the main
fast-forward and tag had not landed yet).

## Decisions

- D1 (user, board 668 a). Cut-point release becomes the general `lead-ship`
  protocol in the shipped playbook, and this project's
  `ai-docs/ship/ws.md` adopts it.
  A release ships a pinned commit at or before the review frontier (narrowed
  to exactly the frontier head by D6), the *cut point*, not the review-track
  tip. The review track keeps taking merges during
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
  (`pass`/`concern`), then the full range `<base>..<head>` as `block`. A block in one
  merge no longer holds back the clean commits before it.
  Rejected: one verdict over the whole range, which is today's behavior and
  the cause of the stall.
- D4 (user, board 671 ok). A cut-point release drops the tip re-assert (ws.md
  R4): it verifies the pinned SHA only. CI and the main promotion already
  target that SHA, so comparing the moving tip adds waiting and no safety.
  Rejected: keeping R4 as an abort on any tip movement.

- D5 (lead, auto-confirmed from design review). One flow, gated by the
  config. When the project's ship config declares release-branch steps (cut,
  bump, promote, merge back), every gated release cuts at the frontier head
  after the gate's review. The frontier head may equal the reviewed tip. There
  is no tip re-check. For these releases this retires Execute step 3's
  "cleared head is the tip after the gate's own lead-review, its ledger commit
  included" pin (dcaaad22, d7ce74dd), since the ledger commit is no longer
  promoted. The gate's dedicated `chore(review)` commit before its review
  step (d7ce74dd) stays: it lies inside the reviewed range, so it sits below
  the frontier and ships with the cut point. A config without those steps keeps today's tip-promotion path
  with the step-3 re-check: the playbook never offers a cut point the config
  cannot execute (lead-ship: "the config is the only source of steps").
  Rejected: two modes chosen per release ("behind the tip" always holds after
  the gate's own ledger commit, so the trigger would always fire); offering a
  cut point to a config that only promotes its tip (the confirmed commit would
  not be the one that ships).
- D6 (lead, auto-confirmed). The cut point is the frontier head, not "any
  commit at or before it". This keeps the playbook to one pin. An earlier
  cut needs an earlier clearing ledger entry, which is not offered.
- D7 (lead, auto-confirmed). Segment stamping is part of lead-review's range
  scenario as a whole, not only the gate's review. A sweep that finds a
  blocking later segment also records its clean prefix; the ledger is
  append-only and the frontier rule is unchanged, so the wider reach is harmless.
  It applies to a NEEDS FIX verdict only: BLOCKED is the blocked-path stop, and
  OPEN already stamps `concern`. Prefixes are judged cumulatively
  (`<base>..<segment head>`), so a segment that blocked once clears after its
  fix lands. After a prefix stamp, the NEEDS FIX re-review of the remainder
  uses the new frontier head as its base.
  Rejected: judging each segment in isolation (a once-blocked segment could
  never clear).

- D8 (user, board 672 b). A prefix stamp vouches for a reviewed state. The
  candidate prefix `<base>..<cut>` is re-reviewed as its own range, and only a
  clearing verdict on that review is stamped.
  Rejected: stamping the finding-free prefix of the aggregate review directly.
  Reviewers judge the head state of the whole range, so a defect introduced in
  the prefix and fixed or masked later never shows in the aggregate findings.
- D9 (user, board 673 b). When the release target is not an ancestor of the
  cut point (the cut predates the previous release's merge-back), the release
  branch merges the release target before the bump and proceeds. The release
  target gains one merge commit. The guard "the release target must be an
  ancestor of the cut point" is checked before the bump in every case and
  picks this path. The guard is the playbook's; the merge itself is a step the
  config declares among its release-branch steps (D5), and a config that
  declares none stops at the guard.
  Rejected: refusing a cut-point release until a cleared prefix reaches past
  the merge-back, which brings the stall back.

## Constraints

- Shipped-surface boundary (`ai-docs/manuals/shipped-surface-boundary.md`).
  The `lead-ship` and `lead-review` playbook text stays project-neutral. Branch
  names (`develop`, `main`, `release/<version>`) and CI commands belong to the
  project ship config, not to the playbook. The playbook speaks of the review
  track, the release branch, and the config's steps.
- Skill authoring (`ai-docs/manuals/skill-authoring.md`) applies to every changed
  playbook line. wsflow mirroring (`ai-docs/manuals/wsflow-mirroring.md`) applies
  to the shared `agents-plugin/rsrc/` playbooks and the `rsrc/manifest.json`
  hashes. The Pi copies (`agents-plugin-pi/rsrc/lead-ship/lead-ship.md`,
  `agents-plugin-pi/rsrc/lead-review/lead-review.md`) are updated byte-identically
  too, with `agents-plugin-pi/rsrc/manifest.json`, because
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go` pins them to the ws copies.
- The review ledger stays append-only. A prefix stamp followed by a block stamp
  uses the existing `review.stamp` entries. No new verdict kind and no ledger
  format change.
- Worktrees (user, board 674). The shipped `lead-ship` playbook never creates
  a worktree. Where the release branch is checked out is decided by the
  project ship config's steps; a config that says nothing works in the current
  checkout. This repository's `ws.md` does the release-branch work in a
  `ws/worktree.acquire` (profile `full`) worktree released afterwards (board
  675), because parallel lead sessions share its main checkout and moving
  that checkout's HEAD would land their commits on the release branch.
- The gate never stamps (lead-ship Release gate step 5 stands; agents-plugin/rsrc/lead-ship/lead-ship.md#L52-L54). Segment stamping happens
  inside `lead-review`'s range scenario.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 99b30dd2 (2026-11-10, commit): "User decisions on the board: (668) a general lead-ship protocol + ws.md adoption; (669) a bump on a release branch at the cut point, merged back; (670) per-merge segment stamping, ok; (671) drop the R4 tip re-assert" — bearing: supports
- 501296b9 (2026-11-10, commit): "The 0.48.16 release-gate review split the range at the version bump fd82c26de. Part 1 (9e9f33b16..fd82c26de) came back clean apart from accepted Minors." — bearing: supports
- dcaaad22 (2026-10-02, commit): "Rejected: exempting ledger-only commits from the tip re-check by path, which would put a path rule into prose; defining the pin point after the gate's lead-review is simpler" — bearing: constrains
- d7ce74dd (2026-10-02, commit): "A dedicated chore(review) commit is made during the gate, before the review step, so the review frontier covers it rather than moving the tip past the reviewed head (which Execute step 3 would abort on)." — bearing: constrains
- 260909-refactor-lead-surface-collapse-worker-stop-protocol (2026-09-09, commit 11cd2603): "lead-ship's promotion re-check named the cleared SHA three ways and assumed the gate had run; unified on one name and scoped it to the case where the gate applied." — bearing: constrains
- 2bbf96e9 (2026-09-15, commit): "frontier f46c314f already covers all reviewable content, and both commits above it (version bump + this review record) are ship-mechanical, not reviewable behavior." — bearing: supports
- 260824-review-release-gate-policy-p2 (2026-08-30, commit 5e346763): "The gate is a playbook branch, not a config bullet, specifically to satisfy R5 (un-omittable)" — bearing: constrains
- 260824-feat-review-release-gate-policy (2026-08-30, commit f3ac6a20): "the existing empty/non-empty rev-list handling was correct once found:true, so this fix inserts a new guard step ahead of it rather than restructuring it." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-ship/lead-ship.md, agents-plugin/rsrc/lead-review/lead-review.md, wsflow and Pi mirrors under agents-plugin-wsflow/rsrc/ and agents-plugin-pi/rsrc/ both currently identical to the ws copies, rsrc manifest.json files, ai-docs/ship/ws.md, .github/workflows/ws-mcp-release.yml |
| scope.surface | public-interface | shipped lead-ship and lead-review playbook prose read by downstream leads; no code symbol |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; Constraints forbid a new verdict kind or ledger format change |
| scope.test_surface | existing | agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py, agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin-tool/internal/mcp/review_watermark_checkpoint_test.go, agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go |
| complexity.reuse_points | confirmed | review.stamp and review.marker frontier resolver, agents-plugin-tool/internal/mcp/server.go#L5173 and agents-plugin-tool/internal/wsreview/ledger_test.go#L279 confirm a trailing block does not advance the frontier |
| complexity.side_effect_risk | moderate | changes which commit a release publishes and drops the tip re-check for cut-point releases in both shipped playbook and ws.md |
| risk.correctness | moderate | stamp ordering against frontier semantics and the release-branch merge-back must keep the release target an ancestor of the review track |
| risk.fit | moderate | shipped playbook text must stay project-neutral while ws.md carries branch names and CI commands |
| risk.test | moderate | prose change verified by render and boundary tests plus a manual dry run; no automated test exercises the release flow |
| risk.security_or_contract | moderate | alters the release gate contract and the R4 safeguard in a shipped playbook, ledger format unchanged |

## Phases

### Phase 1: Cut-point release in lead-ship, segment stamping in lead-review, ws.md adoption

- `agents-plugin/rsrc/lead-ship/` release gate and Execute (D1, D2, D4-D6):
  - Release gate step 4, for a config with release-branch steps: after the
    gate's review, the gate proceeds when the frontier head is not an ancestor
    of the release target (there is newly cleared content), even when the
    review's own verdict is NEEDS FIX over a blocked remainder. It stops with
    "nothing newly cleared" otherwise. The user's explicit override keeps its
    current meaning, shipping uncleared content: it pins the review-track tip
    at override time as the cut point and leaves the marker where it was
    (step 5). Without release-branch steps, step 4 stays as written.
  - When the config declares release-branch steps, the release ships from a
    cut point at the frontier head read after the gate's review. The publish
    confirmation names the cut point, the release branch, and the version.
  - Execute step 3's tip re-check and its "ledger commit included" pin do not
    apply to such a release. The pinned cut point and the release branch head
    on top of it are what publishes. A config without release-branch steps
    keeps step 3 as written.
  - Guard before the bump: when the release target is not an ancestor of the
    cut point, merge the release target into the release branch first (D9).
  - The version bump, when the config's version strategy makes one, lands on
    the release branch (D2). The config names the merge back into the review
    track as a publish or post-ship step.
- `agents-plugin/rsrc/lead-review/` range scenario (D3, D7, D8): on a NEEDS
  FIX verdict whose findings all sit after some first-parent segment boundary,
  the lead picks the candidate prefix by mapping each finding's path and line
  to the first-parent segment that introduced it (`git log --first-parent
  <range> -- <path>`; a path several segments touch maps to the earliest of
  them, and a finding with no path yields no prefix candidate). The mapping
  only proposes a candidate; D8's re-review decides. The reviewers then run again over `<base>..<cut>` as
  its own range (D8). The prefix is stamped only when that review clears.
  There is one re-review per gate: when it does not clear, nothing further is
  searched and the review reports the uncleared range. The clearing
  prefix entry and the block
  entry for the full range are both written. `ParseFrontier`
  (`agents-plugin-tool/internal/wsreview/ledger.go`) takes the last
  pass/concern/bootstrap line in file order, so either order leaves the
  frontier at the prefix; state prefix first, block second.
- `ai-docs/ship/ws.md` (one flow, D5):
  - Pre-flight: fetch, gate, and resolve the cut point (the frontier head).
    Order: start a local working branch at the cut point. When `main` is not
    an ancestor of the cut point, merge `main` into it (D9). Only then does
    Version Strategy read `runtime.json` and the remote tag, so it sees the
    previous release's version. Name the branch `release/<version>` once the
    version is known. The bump script runs and commits there. `go test ./...`, the release-contract check, and the
    marketplace check run on the release branch head. Drop the R4 pin and
    re-assert (D4) and the "develop must be a linear descendant of main" check,
    which the ancestor guard replaces.
  - A restart after red CI deletes and recreates `release/<version>` locally
    and on origin, because its bump never reached develop and Version Strategy
    derives the same version again.
  - Publish (after the final gate, which covers every push below):
    1. Push `release/<version>`.
    2. Dispatch `gh workflow run ws-mcp-ci.yml --ref release/<version>`. The
       run's headSha must equal the release branch head, the commit that gets
       tagged (`ws-mcp-release.yml` L70 requires a green ws-mcp CI run on the
       tag's `GITHUB_SHA`).
    3. Once every job is green: `main` `--ff-only` to the release head, tag
       `v<version>` there, then push main and the tag.
    There is no "push develop first" step: develop pushes are ordinary.
  - Post-ship: merge `release/<version>` back into develop when it carries a
    bump commit, push develop, delete the remote release branch.
  - `.github/workflows/ws-mcp-release.yml` L75: the error hint names
    `--ref release/<version>` instead of `--ref develop`.
- Mirror and manifest updates per the wsflow-mirroring manual, plus the Pi
  copies and Pi manifest (see Constraints).

Verification:
- Go tests run remotely through `~/.local/bin/ws-rgo` (this machine forbids
  heavy local Go builds; see the machine note `remote-go-build`):
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go`,
  `review_watermark_checkpoint_test.go`,
  `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go`,
  `pi_mirror_test.go`, and `manifest_shipped_test.go` pass.
- `agents-plugin/tests/test_skill_dispatch_contracts.py` and
  `test_shipped_surfaces_downstream_neutral.py` pass locally (with `nice -n 19`).
  The shipped text names no devenv branch or command.
- A hand dry run of the `ws.md` steps against a scratch repository (cut point,
  release branch, bump, ff, merge back, then a second release whose cut
  predates the merge-back and takes the D9 merge) follows the playbook without
  contradiction.

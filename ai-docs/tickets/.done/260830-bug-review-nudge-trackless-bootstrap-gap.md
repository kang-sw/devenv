---
title: Review baseline nudge is unreachable on trackless projects
related:
  260824-feat-review-release-gate-policy: introduced ReadAgentsReviewPolicy + ResolveTrack precedence and the review-track config nudge; this gap is in the surrounding nudge wiring
related-mental-model:
  - review-watermark-ledger
completed: 2026-08-30
---

# Review baseline nudge is unreachable on trackless projects

## Problem

On a project with no `review-track` declared in AGENTS.md AND no resolvable
git-default branch (no `origin/HEAD` symbolic-ref, no local `main`, no local
`master`), the review-watermark **baseline onboarding prompt is unreachable**,
even though establishing a baseline does not need a track branch.

Surfaced during a goal-run debrief: "리뷰 제안 자체가, main/master/develop도 없는
프로젝트에서는 조용히 잠길 수 있다는 문제인가요?"

## Mechanism

Two nudge surfaces behave differently for the trackless-and-unconfigured case:

- **`reviewTrackNudge`** (`agents-plugin-tool/internal/mcp/review_track_alarm.go`)
  keys only on whether `ReadAgentsReviewPolicy(root).ReviewTrack != ""`. It
  **fires** the "declare `review-track:`" hint — good, the user is told to
  configure a track. But its wording reassures that the sweep "falls back to the
  git-default branch heuristic," which is **misleading in exactly this edge
  case**: on a repo with no `origin/HEAD`/`main`/`master`, that heuristic
  (`ResolveTrack`) also fails.

- **`CheckpointNudge`** (`agents-plugin-tool/internal/wsreview/checkpoint.go`)
  calls `ResolveTrack` as its **first** step and returns `""` on failure —
  before it ever reaches the `!found` branch that emits
  `"no review ledger yet for this project; run a sweep ... to establish a
  baseline"`. So on a trackless repo, the **baseline-onboarding prompt never
  fires**, even though `wsreview.Bootstrap` resolves `HEAD` (not the track) and
  would bootstrap the ledger fine.

Net effect: the user is told "declare a track" but never told "run a baseline
sweep," and if no track is declared the watermark advisory goes fully dark —
despite the ledger being bootstrappable against HEAD.

## Severity / scope

Not Critical: review is not *broken* — `lead-review range: <base>..<head>` and
`wsreview.Bootstrap` still work by hand. This is a **quiet-failure onboarding
gap**: the advisory whose whole purpose is to prompt review self-suppresses on
precisely the unconfigured projects that most need onboarding.

## Candidate directions (not yet decided)

- Decouple `CheckpointNudge`'s no-ledger baseline prompt from `ResolveTrack`
  success: the "establish a baseline" message needs only `Read(root)` /
  `Bootstrap` (HEAD-based), not the track. Only the "N commits behind <track>"
  staleness arm needs a resolved track.
- When `ResolveTrack` fails entirely (not just "unconfigured"), surface an
  actionable trackless-onboarding hint ("declare `review-track:` or create a
  main/develop branch") rather than going silent.
- Fix `reviewTrackNudge`'s wording so it does not promise heuristic fallback
  coverage that does not exist when no default branch is present.

## Notes

- Confirmed by direct read of `checkpoint.go` (ResolveTrack-first ordering),
  `track.go` (three-tier fail-open resolution), `ledger.go` `Bootstrap`
  (HEAD-based, track-independent), and `review_track_alarm.go` (declaration-only
  keying).
- `release-boundary` defaults to `absent`, so this is orthogonal to the
  (still-blocked) mandatory release gate of
  `260824-feat-review-release-gate-policy` Phase 2 — but the same trackless /
  `absent` bootstrapping question should inform that gate's design (R5).

## Resolution (2026-08-30, directly fixed)

Fixed directly (user-approved minimal direction, not promoted through the ticket
ceremony):

- `wsreview/checkpoint.go` `CheckpointNudge`: read the ledger **before**
  resolving the track. The "no review ledger yet ... establish a baseline"
  prompt now fires regardless of track resolution (baselining uses HEAD, not the
  track); `ResolveTrack` failure now silently skips **only** the "N commits
  behind <track>" staleness/size arms.
- `mcp/review_track_alarm.go` `reviewTrackNudge`: reworded so it no longer
  promises heuristic fallback coverage that does not exist when the repo has no
  `origin/HEAD`/`main`/`master`.
- Tests: rewrote `TestCheckpointNudgeSkipsSilentlyWhenTrackUnresolvable` (which
  had pinned the buggy "trackless => silent" behavior) into
  `...SkipsBehindTrackArmWhenTrackUnresolvable` (ledger seeded, no track => "");
  added `TestCheckpointNudgeSurfacesBaselineWithoutTrack` (no track + no ledger
  => baseline advisory, still no file write).

Adjacent fix surfaced during verification (in-scope, same nudge plumbing):

- `mcp/session_state.go` `handleEnterProceed` appended the watermark line only in
  the text branch, so the JSON result's `Raw` field silently diverged from the
  text output whenever the nudge was non-empty. Hoisted the append before the
  format branch so `TestEnterProceedJSONIncludesRawVerdict`'s parity holds for
  any non-empty nudge, not just the (previously masked) empty case.

Known sibling issue left for a follow-up (NOT fixed here, out of approved scope,
no failing test): the identical text-only watermark append exists in
`handleEnterImplement`'s target path (`session_state.go` ~L1067-1075) — the
`enter.implement` JSON path (`implementResultJSON`) omits the watermark line the
text path adds. Same one-line hoist would fix it; capture separately if it
matters.

Onboarding-recurrence note (answers the once-per-session concern): the
`reviewTrackNudge` config hint is once-per-session (`ReviewTrackNudgeShown`), so
a missed session loses it until the next session. The `CheckpointNudge` baseline
prompt is **not** session-gated — it recurs on every proceed/manual load until a
ledger exists — so with this fix the trackless onboarding path is persistently
prompted, not shown-once.

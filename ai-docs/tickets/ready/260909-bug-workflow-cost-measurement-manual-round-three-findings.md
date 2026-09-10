---
title: "Fix the workflow-cost measurement manual's round-three findings and re-run the baseline"
related:
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; placed the manual and recorded the first baseline
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ed1f3fe34b450cd8
sage-review-completeness-reviewed: ed1f3fe34b450cd8
---

# Fix the workflow-cost measurement manual's round-three findings and re-run the baseline

## Background

The measurement-manual ticket landed `ai-docs/manuals/workflow-cost-measurement.md`
and its first baseline (Edition `6a5947ab` on that ticket's Phase 1 Result).
The worker that ran it opened a third review round on the committed state;
the lead stopped the run by directive before the worker acted on that
round, and the findings below were reported unfixed. They are recorded here
verbatim from the worker's terminal report so the epic's after-run does not
inherit a manual whose Critical indicators are known to be wrong.

The baseline is cheap to recompute: the manual is read-only over history and
pins its measured commit (`develop@84b1f825`), so fixing the manual and
re-running "before" at the same commit costs one run and no coordination.
That is why these findings do not block the epic's removals; they block the
after-run.

## Decisions

- **The fixed manual is the one both halves use.** The after-run must use
  the same manual text as the before-run it is compared against. Every
  Critical below changes an indicator's meaning, so the baseline recorded in
  the placed ticket's Edition is superseded once this ticket lands, and the
  new baseline is recorded here. *Rejected: keep the `6a5947ab` baseline and
  fix only prose* — indicators 1 and 6 produce different numbers after the
  fix, so the old figures would not be comparable.
- **Findings are triaged here, not re-reviewed.** Each finding is a quote
  from a reviewer that already read the file; the worker classifies
  `fix` / `risk accepted` (with cost) / `intentional difference` and edits
  only `fix`. A second fresh sweep of the manual is exactly what the epic's
  Cross-Child Decision 20 forbids. *Rejected: a new round-1 review of the
  manual* — the third round already found what a fresh reviewer finds.
  This does not waive the route's own review of this ticket's diff: Decision
  20 mandates round 1 on the change at the route's allocation and forbids
  only a third round on the same work. Round 1 here is scoped to the diff;
  a reviewer finding about manual text the diff did not touch goes to the
  report's `unresolved:` line, not into the finding list above.
- **Every Critical is a `fix`.** The classification freedom above applies to
  Major and Minor findings only. *Rejected: risk-accepting a Critical* — the
  Background bars a manual whose Critical indicators are known to be wrong
  from the after-run, so a risk-accepted Critical would defeat the ticket.
- **An unexplained baseline delta is recorded, not re-reviewed.** When the
  new baseline differs from the superseded one on an indicator that no
  finding explains, the worker records the delta and its most plausible
  cause in the Result and proceeds (the epic's Decision 5 default: the
  worker decides, records, and continues). *Rejected: a fourth review round*
  — the lead stopped the third by directive; *rejected: silent acceptance* —
  the after-run compares against these figures.

## Constraints

- Read-only over git history, as the manual's own Rules require; the
  measured commit stays `develop@84b1f825` so the two baselines are directly
  comparable.
- Shipped-surface leak rule (`AGENTS.md` Architecture Rule 4) applies to the
  manual: no stems, hashes, or repository layout names in its body.
- The placed ticket is `.done/`; do not edit it. Record the superseding
  baseline in this ticket's Result and cite the old Edition by hash.

## Findings (verbatim from the worker's report, severity as reported)

Critical:

- `$BRANCH` is a moving ref while the run is defined "at the measured
  commit", so a retroactive before-run counts commits made after the change.
- `git archive "$BRANCH"` materialization reads the branch tip while the
  default `TICKETS` form reads the working tree at the measured commit; the
  two documented forms are not equivalent.
- indicator 6 "dropped phases" uses an unanchored `grep -l '\[dropped\]'`
  that matches inline-code prose — reports 8, true count 4, and the
  measurement ticket itself is one of the false positives.
- indicator 6 "blocked notes" is wrong three ways that nearly cancel
  (`grep -l` counts files not notes; a fenced convention example
  self-matches; a bare `## Blocked` is missed) — reports 9, true count 10.
- indicator 6 "goal merges" conflates merges into goal branches (7), merges
  onto main (2), and real goal runs (~5), then reads against "goal branches
  not merged: 5" as a completion rate.
- indicator 1's first-parent gap is not measured along `$BRANCH`'s
  first-parent chain on 17 of 20 rows; comparable one-day tickets differ
  14×, and older windows reach 502, purely by merge topology — the exact
  thing a workflow change alters.
- the Window has no unavailable condition and no execution floor, so a
  project without ws ticket conventions gets a silent empty window presented
  as success.

Major:

- indicator 1's `first` is a bare `--grep` with no stated limit and selects
  other tickets' commits.
- `first_impl`'s scope check compares only the six-digit date, not the stem,
  so a same-day sibling ticket's full-stem scope passes; the prose says
  otherwise and same-day siblings are the norm in this window.
- "Every indicator loops over `$STEMS` … so all six measure the same window"
  is false — indicator 6 loops over the tree and the manual contradicts
  itself 220 lines later.
- indicator 6's stated unavailable condition is unreachable by any command
  in the manual; all four lines print an unconditional integer and cannot
  distinguish "convention absent" from "count zero".
- indicator 3's stated unavailable condition omits chore/plan/ticket types,
  which also produce the row (2 of the 5 observed), corrupting the "rise
  means convention change" reading.
- the Indicators preamble demands a distribution for indicators 1, 3 and 4,
  but 1 emits two values per stem and 3 emits a pair whose only natural
  reduction is the percentage indicator 3 forbids; indicator 4's
  distribution is ambiguous between printed and judged counts.
- `## Record` omits the exact commands, the mandated by-hand judgments, the
  unavailable/n-a counts, and the before/after commit rationale; a run with
  every hand-read skipped passes its completion test.
- the setup block points at `AGENTS.md ### Review Policy` review-track,
  which a fresh reader's project may not have.
- 48 skips against a 20-window silently redefine the sample and no threshold
  is stated at which the window stops being a window.
- the Edition's closing sentence "the sh blocks are byte-identical to those
  verified … in c96b4db2" is false — 31 lines changed at 6a5947ab; the blocks
  were re-run across three shells at 6a5947ab, so the claim is misattributed
  rather than unverified. (Edition is frozen; record the correction in this
  ticket's Result.)
- the Edition pairs "1 of 52" with "10 soundly anchored tickets"; 52 spans all
  12 measurable and the 10 sound ones total 46. (Same handling.)
- `f3ac6a20` is mischaracterized as a later phase's implementation — its body
  opens "Critical review finding:" and it repairs Phase 2; three fix commits
  closed that phase and none counts as corrective, which is the window's
  strongest evidence for the Edition's own thesis and is left unstated.

Minor (fourteen, reported by the worker as one line; itemized here so each
carries its own classification):

- indicator 5's "order the bullets were written in" is backwards.
- an `@@` diff-hunk line inside a bullet truncates it.
- indicator 4's awk terminator misses `# `.
- `{2,3}` intervals are absent from older mawk.
- the read-only Rule does not cover mktemp/git archive/tar and the temp tree
  is never cleaned.
- indicator 2's unavailable condition is unsurfaced and a zero-commit stem
  prints a bare unlabelled row.
- `epoch()`'s BSD fallback can silently return a constant.
- indicator 5 folds indented sub-bullets and states no "what a movement does
  not license".
- indicator 5's unavailable message can state the wrong reason.
- `git branch --list` sees local refs only.
- `printf '%s\n' $STEMS | wc -l` reports 1 for an empty window, disagreeing
  with `window | wc -l`.
- `comm -13` needs lexical sort while window emits date order.
- three Edition description errors (`cd878e98`'s commit shape, `0221f6fe`'s
  heading and effect, the n/a row's cause) whose classifications remain
  correct. (Edition is frozen; restate the corrections in this ticket's
  Result.)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | ai-docs/manuals/workflow-cost-measurement.md (the fix target); this ticket file (Result recording the re-run baseline) |
| scope.surface | internal | no exported code symbol; the file is markdown prose plus embedded sh/awk blocks, not compiled/imported code |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/manuals_test.go covers manuals-inventory listing only (generic summary: frontmatter check); no test asserts this manual's indicator output, so verification is the phase's own three-shell verbatim run |
| complexity.reuse_points | not-applicable | self-contained edit to one existing manual's prose and shell blocks; no separate component is reused |
| complexity.side_effect_risk | low | read-only git-history procedure with no runtime code path; Constraints bar any shipped-surface (agents-plugin*/) edit |
| risk.correctness | moderate | six indicators' shell/awk selection logic (BRANCH pinning, first_impl scope match, indicator 6's four counters) are being corrected together and must stay byte-identical across sh/bash/dash |
| risk.fit | low | follows the prerequisite ticket's already-established manual structure, Rules, and Decisions rather than introducing a new shape |
| risk.test | moderate | no automated test covers indicator correctness; the only verification is a manual three-shell re-run plus the hand-read judgments the manual itself mandates (indicators 3, 4, 5) |
| risk.security_or_contract | low | no public API, security boundary, or code contract touched; the manual only reads git history and the ticket tree |

## Phases

### Phase 1: Triage and fix the manual, then re-run the baseline

Goal: every finding above carries a classification in this phase's Result;
every `fix` is applied to `ai-docs/manuals/workflow-cost-measurement.md`;
the manual is re-run end to end at `develop@84b1f825` under `sh`, `bash`,
and `dash` with byte-identical output; the resulting figures are recorded
in the Result as the epic's before-run baseline, superseding Edition
`6a5947ab` on the placed ticket.

Order the Criticals first: the `$BRANCH` / `git archive` pair (one fix: pin
both forms to the measured commit), then indicator 1's first-parent chain,
then indicator 6's four lines (the three Criticals above plus the Major
unreachable-unavailable-condition finding, which lives in the same block and
lands in the same step), then the Window's unavailable condition and
execution floor. The Majors that describe prose contradicted by the
commands are fixed by changing whichever side is wrong, stated in the
Result. The Edition-description findings cannot be fixed in place (the
Edition is frozen); the Result restates the corrected claims.

Two Criticals name the defect but not the target: indicator 6's goal-merges
line (what it should count) and the Window's execution floor (its value,
reconciled with the existing "fewer than 20 records all of them and says
so" sentence). The manual's own Rules constrain the shape — no score, pairs
rather than percentages, unavailable never substituted — so the worker
chooses within them and the Result states the shape chosen, since both
halves of the comparison are then locked to it. Indicator 6's two
`git branch --list` lines read the live local ref set, not the measured
commit, so a re-run "at the measured commit" cannot pin them as written:
make them derivable from the measured commit or emit them as unavailable,
and state which in the Result.

Verification: the three-shell verbatim run; a leak grep on the manual; a
diff of the new baseline against the superseded one, each changed indicator
explained by the finding that changed it.

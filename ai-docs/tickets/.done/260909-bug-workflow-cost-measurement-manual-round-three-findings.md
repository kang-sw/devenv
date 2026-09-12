---
title: "Fix the workflow-cost measurement manual's round-three findings and re-run the baseline"
related:
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; placed the manual and recorded the first baseline
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ed1f3fe34b450cd8
sage-review-completeness-reviewed: ed1f3fe34b450cd8
completed: 2026-09-10
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

### Result (d0cb62b5) - 2026-09-10

**Behavioral delta.** `ai-docs/manuals/workflow-cost-measurement.md` is
rewritten: every command reads an immutable `$COMMIT` instead of a branch
name, the ticket tree is always materialized from that same hash, indicator 1
measures its gap along the measured first-parent chain, indicator 6 counts
marker *lines* outside fenced blocks and splits its goal-merge count by
topology, and the Window, the Indicators preamble, and `## Record` say what
they previously only implied. Nothing outside that one file changed. The
baseline below supersedes Edition `6a5947ab` on the prerequisite ticket.

#### Every finding, classified

Criticals are `fix` by Decision 3. Findings whose target is the frozen Edition
text cannot be fixed in place; they are classified `Edition` and the corrected
claim is restated under *Frozen-Edition corrections* below.

Critical (7) — all `fix`:

| finding | fix |
|---|---|
| `$BRANCH` is a moving ref | `COMMIT` is taken from the environment (a retroactive run exports the hash its counterpart recorded); only a tip run lets the block resolve it, and must record what it resolved. No command reads a branch as a revision. |
| the two `TICKETS` forms are not equivalent | one form only. The tree is always materialized from `$COMMIT`; the working-tree form is gone, so it cannot disagree. |
| dropped phases: unanchored `grep -l` | `count_marker '^#+ .*[[]dropped[]]'`, counting lines outside fenced blocks. **8 → 4.** |
| blocked notes: files not notes, self-matching fence, bare heading missed | `count_marker '^## Blocked([ (]\|$)'`, same helper. **9 → 10.** |
| goal merges conflate three populations | split by topology: first-parent membership separates *landed on the measured line* (5) from *other `merge(goal)` merges reachable* (9), stated as a lower bound and stated as not a completion rate. |
| indicator 1's gap is not a first-parent distance | new `fp_pos` resolves each endpoint to a position on the measured chain — its own, or the oldest chain commit that has it as an ancestor — and subtracts. |
| the Window has no unavailable condition and no execution floor | four named sizes, printed by a command: `unavailable` (zero stems, run ends), `below floor` (`FLOOR=5`, comparison withdrawn), `short`, `full`. |

Major (the ticket lists 12):

| finding | class | disposition |
|---|---|---|
| indicator 1's `first` is a bare `--grep` with no stated limit | fix | the limit is stated in indicator 1's Limits, in the same terms indicator 2 already used, and the consequence (the row starts earlier than the work did) is named. |
| `first_impl`'s scope check compares only the date | fix | replaced with a stem-boundary prefix test: a full different same-day stem no longer passes, a date-only scope still does. The prose was right and the command was wrong. The residual — a date-only scope credited to every ticket created that day — is stated and added to `## Record`. **Changes 0 of 570 rows at the measured commit; preventive.** |
| "Every indicator loops over `$STEMS`" is false | fix | the preamble now says indicators 1–5 loop over `$STEMS` and indicator 6 is a whole-tree count, not a window indicator. The command was right and the prose was wrong. |
| indicator 6's unavailable condition is unreachable | fix | four `USES_*` flags with no default. A run that has not checked prints `unavailable` four times. |
| indicator 3's unavailable condition omits chore/plan/ticket | fix | reworded to "a workflow-administration type (`docs`, `chore`, `plan`, `ticket`, `merge`, or any other type outside the product list)". The command was right and the prose was wrong. |
| the preamble demands a distribution 1, 3 and 4 cannot give | fix | the preamble now names what "the distribution" means per indicator: two for indicator 1, none for indicator 3 (pairs, plus the window pair and the unavailable/`n/a` counts), and the *judged* number for indicator 4. |
| `## Record` omits commands, judgments, unavailable counts, commit rationale | fix | four of the six Record bullets rewritten, a new **Judgments** bullet added, and the completion test now fails a run whose mandated reads are missing. |
| the setup block points at a section a fresh reader's project may not have | fix | reworded to "some projects declare a tracked branch in their agent-context file, and if yours does, use that". |
| 48 skips against a 20-window redefine the sample with no stated threshold | fix | the skip count is now a second axis with its own printed verdict: above `SIZE` the run prints `window unrepresentative` and the honest reading is restricted to the shared calendar period. |
| the Edition's "byte-identical … in `c96b4db2`" is false | Edition | restated below. |
| the Edition pairs "1 of 52" with "10 soundly anchored tickets" | Edition | restated below. |
| `f3ac6a20` is mischaracterized as a later phase's implementation | Edition | restated below, **and** fixed forward: indicator 3's anchor prose now names the repair-anchor case as a distinct, stronger `unavailable`-in-disguise signal than the late-phase case. |

Minor (the heading says fourteen; the ticket itemizes 13 bullets, one of
which carries three Edition errors — all 13 are classified here):

| finding | class | disposition |
|---|---|---|
| indicator 5's "order the bullets were written in" is backwards | fix | now "stem by stem in window order, and within one stem newest commit first". |
| an `@@` diff-hunk line truncates a bullet | fix | sentinel changed to `@@commit@@`. |
| indicator 4's awk terminator misses `# ` | fix | `/^(#\|##\|###) /`. |
| `{2,3}` intervals are absent from older mawk | fix | no interval remains in any awk pattern; verified by running every block with mawk shadowing awk. |
| the read-only Rule does not cover mktemp/archive/tar, and the temp tree leaks | fix | the Rule names the scratch tree as the one thing a run writes, and teardown is now a command block, not a comment. |
| indicator 2's unavailable condition is unsurfaced | fix | a stem no commit names prints `unavailable (no commit names the stem)`. |
| `epoch()`'s BSD fallback can silently return a constant | fix | a shape test rejects anything that is not a bare `YYYY-MM-DD` before either `date` runs. |
| indicator 5 folds sub-bullets and states no "does not license" | fix | the folding is stated as deliberate (a sub-bullet qualifies its parent), and a "does not license" paragraph added. |
| indicator 5's unavailable message can state the wrong reason | fix | two messages, chosen by a printed distinct-commit count. |
| `git branch --list` sees local refs only | fix | stated in indicator 6's Limits, together with the sharper point that a ref set is not history. |
| `printf '%s\n' $STEMS \| wc -l` reports 1 for an empty window | fix | the window block computes `N` with `grep -c .` on a quoted expansion. |
| `comm -13` needs lexical sort while `window` emits date order | fix | the comparison step says to sort both files first and why. |
| three Edition description errors (`cd878e98`, `0221f6fe`, the n/a row) | Edition | the ticket records that their classifications remain correct; no figure depends on them, and no restatement is available beyond that. |

#### Shape decisions the phase left to the worker

- **Indicator 6's goal-merges line counts goal runs that landed on the
  measured line**, identified by first-parent membership rather than by
  subject wording, because the `merge(goal)` subject is written both for a
  landing and for an update *into* a live goal branch. The remainder is
  reported as a second, separate number. It is a lower bound, and the manual
  says so: a landing merge that was later re-parented falls into the
  remainder. The two lines are explicitly not a completion rate.
- **The execution floor is 5**, and it does not replace the "fewer than 20
  records all of them" sentence — it sits below it. Four sizes: `unavailable`
  at zero (the run ends), `below floor` under 5 (indicators recorded, the
  indicator-by-indicator comparison withdrawn), `short` under `SIZE`, `full`
  at `SIZE`.
- **Indicator 6's two `git branch --list` lines stay live-ref reads, and are
  emitted as `unavailable` when the run is retroactive.** They cannot be made
  derivable from the measured commit: a ref set is not history, and a branch
  created or deleted since the measured commit is invisible either way.
  `--no-merged "$COMMIT"` pins the merged-ness test but not the ref set, so
  the block derives `AT_TIP` and prints
  `unavailable (live ref set, run is retroactive)` when it is `no`. This run
  is at the tip (`develop` still points at the measured commit), so the
  listing is recorded below as observed.

#### The new baseline

**Run header.** Measured commit `84b1f825`, branch `develop`, 2026-09-10.
`AT_TIP=yes` — `develop` still points at the measured commit, so the run is
not retroactive. `TICKETS` is the materialized tree (now the only form).
Convention flags all `yes`, checked against this project's ticket conventions.
Commit convention in force is unchanged from the superseded run: lead-authored
commits, roughly one per phase plus a separate `docs(ticket)` closure commit,
with implementation landing on `impl/*`/`goal/*` branches merged `--no-ff`.
This commit was chosen because the superseded baseline used it, and the two
must be directly comparable.

**Commands as run.** `BRANCH=develop`, `COMMIT` resolved to `84b1f825…`,
`SIZE=20`, `FLOOR=5`, `USES_BLOCKED=USES_DROPPED_PHASE=USES_GOAL_MERGE=`
`USES_GOAL_BRANCH=yes`. The nine `sh` blocks were extracted in document order
and run verbatim; nothing was edited for the run.

**Window.** `window: full (20)`; `skipped (no completed:): 48`;
`window unrepresentative (skipped 48 > SIZE 20)`. The 20 stems are unchanged
from the superseded run, oldest first:
`260824-feat-lead-review-range-scenario`,
`260824-feat-review-release-gate-policy`,
`260824-feat-review-watermark-ledger`,
`260828-refactor-per-slice-review-relay`,
`260830-bug-review-nudge-trackless-bootstrap-gap`,
`260830-feat-sage-freshness-content-baseline`,
`260831-bug-survey-plan-unilateral-scope-reduction`,
`260831-refactor-severity-graded-per-slice-review-relay`,
`260901-feat-note-oversize-layer-aware-clone-path`,
`260903-refactor-mcp-read-surface-collapse`,
`260903-refactor-mcp-todo-signature-merge`,
`260903-refactor-mcp-verb-vocabulary-unification`,
`260904-bug-windows-parent-watch-pid-reuse-flake`,
`260904-refactor-enter-affordance-rename-route-opaque`,
`260906-bug-route-opaque-params-handler-mismatch`,
`260907-feat-ws-project-tree-parent-nested-ticket-render`,
`260908-feat-survey-plan-is-route-not-contract`,
`260908-bug-sage-gate-stale-completed-has-no-rerun-path`,
`260908-bug-shipped-surfaces-carry-devenv-only-content`,
`260908-feat-implement-skip-survey-for-localized-ticket-target`.
The `unrepresentative` verdict is new information, not a new fact: the 48
skips were recorded before, but nothing said what they meant for the sample.

**1. Ticket latency.** Days unchanged: min 0, median 0.5, max 6 (0×10, 1×6,
2×1, 6×3). First-parent gap **min 0, median 6.5, max 31** (was min 0, median
14, max 57), no `unavailable` and no `inverted` row:

```text
260824-feat-lead-review-range-scenario 6/31
260824-feat-review-release-gate-policy 6/31
260824-feat-review-watermark-ledger 6/31
260828-refactor-per-slice-review-relay 2/11
260830-bug-review-nudge-trackless-bootstrap-gap 0/0
260830-feat-sage-freshness-content-baseline 0/3
260831-bug-survey-plan-unilateral-scope-reduction 0/2
260831-refactor-severity-graded-per-slice-review-relay 0/7
260901-feat-note-oversize-layer-aware-clone-path 0/1
260903-refactor-mcp-read-surface-collapse 1/9
260903-refactor-mcp-todo-signature-merge 1/9
260903-refactor-mcp-verb-vocabulary-unification 1/9
260904-bug-windows-parent-watch-pid-reuse-flake 0/1
260904-refactor-enter-affordance-rename-route-opaque 0/6
260906-bug-route-opaque-params-handler-mismatch 0/1
260907-feat-ws-project-tree-parent-nested-ticket-render 0/4
260908-feat-survey-plan-is-route-not-contract 0/12
260908-bug-sage-gate-stale-completed-has-no-rerun-path 1/7
260908-bug-shipped-surfaces-carry-devenv-only-content 1/4
260908-feat-implement-skip-survey-for-localized-ticket-target 1/6
```

**2. Commits per ticket, by type.** Unchanged, byte for byte, from the
superseded run: 137 per-row commit references across 20 tickets — `docs` 81,
`chore` 17, `merge` 17, `feat` 10, `fix` 4, `refactor` 4, `test` 3, `plan` 1 —
over **115 distinct commits**. The seven tickets with no product-typed commit
naming them on the measured line are the same seven. No row reported
`unavailable`.

**3. Corrective share.** **1 corrective of 52 post-implementation commits
across the 12 measurable tickets**; 7 `unavailable (no implementation
commit)`, 1 `n/a`. Per-ticket pairs: range-scenario 0/3; release-gate-policy
0/2 (defective anchor); watermark-ledger 1/8; sage-freshness 0/3;
survey-plan-unilateral 0/4; note-oversize 0/4; read-surface-collapse 0/3;
verb-vocabulary 0/3; windows-parent-watch 0/1; enter-affordance-rename 0/7;
survey-plan-is-route 0/4 (defective anchor); shipped-surfaces 0/10.
Anchor read: **10 sound, 2 defective**, which restricted to soundly anchored
tickets is **1 of 46 across 10**. Date-only scope: 3 rows (all three `260824`
tickets) were selected through a scope several same-day tickets share; each
resolves to a distinct commit, so no row is misattributed.

**4. Escalations recorded in `### Result`.** 18 measurable, 2 `unavailable (no
phase Result)`. **10 lines printed across 2 tickets, 0 judged to record an
actual stop** — all 10 are the tickets' own `[escalate-to-lead]` /
`[escalate-to-research]` design vocabulary discussed in the abstract.

**5. Judgment items in `## AI Context`.** 115 distinct commits, **326 bullets**
— the extracted bullet set is byte-identical to the superseded run's (verified
by `diff` on the sorted output; the `@@commit@@` sentinel changed no bullet
because no window commit body carries a `@@` line). The classification is
therefore carried forward unchanged: **172 judgment, 154 narration**, by four
blind readers over four equal partitions, per-partition share 41 %–69 %.
Re-classifying an identical input would have added measurer noise and nothing
else; the protocol itself is still not encoded in the manual, so an after-run
that wants comparability must copy it from here.

**6. Aborted-run indicators.** `blocked notes: 10` (was 9); `dropped phases: 4`
(was 8); `dropped with implementation commits: 9 printed, 4 real` (unchanged —
the same 4 genuine and 5 false-positive rows the Edition identified);
`goal runs landed on the measured line: 5`; `other merge(goal) merges
reachable: 9` (together the 14 the superseded run reported as one number);
`goal branches not merged: 5` — `goal/drain-ready-queue`,
`goal/ws-dashboard-dev/copper-heron-vale`,
`goal/ws-dashboard-dev/marlin-cove-thistle`,
`goal/ws-dashboard-related-tickets`, `goal/ws-dashboard-tickets`, all last
committed in July and hundreds of commits off the measured line: abandoned,
not in flight.

**Every delta from the superseded baseline, and its cause.** There is no
unexplained delta.

| indicator | superseded | new | cause |
|---|---|---|---|
| 1, gap distribution | min 0 / median 14 / max 57 | min 0 / median 6.5 / max 31 | the first-parent Critical. The three same-day `260824` tickets collapse from 34/57/41 to 31/31/31, and the three same-day `260903` tickets from 14/14/14 to 9/9/9. |
| 6, blocked notes | 9 | 10 | `grep -l` counted files: 11 raw heading lines, minus 1 fenced convention example, over 9 files. |
| 6, dropped phases | 8 | 4 | anchoring plus fence exclusion; 4 of the 8 were prose mentions, one of them in the prerequisite ticket itself. |
| 6, goal merges | 14 | 5 landed + 9 other | the conflation Critical. |
| everything else | — | identical | indicators 2, 3, 4, 5 and the window reproduce byte for byte. |

**Ambiguities the run hit.** One: `## Record` asks for "the exact commands as
run", and with the convention flags now unset by default there are two honest
runs of the same document — a verbatim one that answers nothing and prints
four `unavailable` lines, and the measured one that exports the four flags.
The run recorded both; the manual could say which is the run of record. Not
fixed, because the answer is obvious from `## Record`'s flag field and
inventing a name for the distinction would be worse than the ambiguity.

#### Frozen-Edition corrections

The Edition at `6a5947ab` on the prerequisite ticket cannot be edited. These
of its claims are wrong; where they disagree with this Result, this Result is
the baseline.

- *"the `sh` blocks are byte-identical to those verified … in `c96b4db2`"* is
  false. 31 lines of `sh` changed at `6a5947ab`. The blocks were in fact
  re-run across three shells at `6a5947ab`, so the verification happened and
  the claim merely misattributes it to the earlier commit.
- *"1 of 52 across 10 soundly anchored tickets"* pairs two figures that do not
  go together: 52 is the post-implementation total over all **12** measurable
  tickets. The 10 soundly anchored ones total **46**, so the sound-anchor
  reading is **1 of 46 across 10**, and the all-measurable reading is 1 of 52
  across 12. Both are reproduced above.
- *`f3ac6a20` characterized as "a later phase's implementation"* is wrong in a
  way that matters. Its body opens "Critical review finding:" and it repairs
  Phase 2 of the ticket it anchors; three `fix` commits closed that phase and
  the corrective rule counted none of them, because the anchor is itself the
  repair and a commit cannot be corrective against itself. That is the
  window's strongest evidence for the Edition's own thesis, and the Edition
  left it unstated. The repaired manual now names this case separately in
  indicator 3's anchor prose as the stronger of the two `unavailable`-in-
  disguise signals.
- The three Edition description errors the ticket lists as Minor
  (`cd878e98`'s commit shape, `0221f6fe`'s heading and effect, the `n/a` row's
  cause) do not change any figure; the classifications they support remain
  correct, and the dropped-row baseline stands at 9 printed / 4 real.

#### Verification

- **Verbatim, nothing exported.** The nine `sh` blocks extracted in document
  order and run from the repository root under `sh`, `bash`, `dash`, and
  again with `mawk` shadowing `awk`: byte-identical output in all four
  (103 687 bytes), exit 0, stderr exactly the 48 documented
  `skipped (no completed:)` lines and nothing else. Indicator 6 prints
  `unavailable (convention not used)` four times, which is the correct output
  for a run that has answered nothing.
- **The measured baseline.** Same nine blocks, same four interpreters, with
  the four convention flags exported: byte-identical in all four
  (103 845 bytes), exit 0, same stderr. Indicators 1–5 byte-identical to the
  pre-review-fix run of the same document.
- **Every `unavailable` branch exercised**, on throwaway git fixtures, because
  the real tree reaches none of them: a project with no `.dropped/` directory
  (the review-1 Critical — real counts, no awk abort, exit 0), `below floor
  (3 of 5)`, `short (6 of 20)`, `full (20)`,
  `unavailable (no closed actionable ticket window)` with and without a ticket
  tree at all, a retroactive run printing
  `unavailable (live ref set, run is retroactive)`, an unparseable
  `completed:` printing `days=unavailable`, a stem named by no commit printing
  indicator 2's and 3's `unavailable` rows, a commit with no `## AI Context`
  printing indicator 5's second message, and a side-branch topology printing
  `commits=inverted`.
- **`first_impl`'s scope fix changes nothing here.** Old and new selectors run
  side by side over every `.done/` and `.dropped/` stem at the measured
  commit: 0 rows differ. The fix is preventive and the baseline is unaffected
  by it.
- **Leak grep on the manual body:** no ticket stem, no commit hash, no name of
  this repository's layout or tooling, no migration vocabulary.
- **`go test -count=1 ./internal/wsdoc/...`:** ok. The manuals inventory still
  lists the file with its one-line `summary:`.
- **Repository unmodified by the runs:** `git status` shows only the manual
  and the pre-existing untracked build artifact; every scratch tree removed.

#### Review

Partitioned round 1 (correctness, test), both fresh delegates that did not
write the change. Correctness returned 1 Critical, 2 Important, 4 Minor; test
returned 4 Important, 3 Minor. Every Critical and Important is `[fixed]` in
`d0cb62b5`, and every Minor with it, except two that are not defects in the
diff: the test reviewer's "the phase's verification evidence is not in the
range" — this Result is that evidence — and its coverage finding, which is
answered by the fixture matrix above rather than by a test, since the manual
has no automated test surface and the ticket's route facts say so. One
review-1 Critical means a Critical-scoped round 2 on the fix.

The Critical was real and I had introduced it: replacing `grep -l ...
2>/dev/null` with `awk` made an unmatched glob fatal, so a project with no
`.dropped/` directory printed an empty value and still exited 0 — the same
"silent non-answer presented as a result" shape the Window Critical objected
to. My first fix for it was itself wrong (`set --` inside a shell function
does not reach the caller), which the re-run caught before commit.

Round 2, Critical-scoped, a third fresh delegate: **clean**. It reproduced the
original failure at `37b4c0b6` on a `.dropped/`-less fixture across
{gawk, mawk} × {sh, bash, dash}, confirmed the fix on all six, confirmed the
empty-list branch reachable, and confirmed the real-tree counts still 10 and
4. It raised one observation outside its scope, recorded and not fixed: when
the measured commit has no `ai-docs/tickets` path at all, the setup block's
`git archive`/`tar` write a `pathspec did not match` message and two `tar:`
lines to stderr before the run degrades to the stated `unavailable`. The
degradation is correct and the exit status is 0; only the noise is
undocumented. Silencing it would hide a real failure of the materialization
step, so it stays.

**Unresolved / carried forward.** The pre-existing
`test_proceed_keeps_implementation_route_only` failure recorded on the
prerequisite ticket is untouched and still belongs to whichever child edits
that surface.

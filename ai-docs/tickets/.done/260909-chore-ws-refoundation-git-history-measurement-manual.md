---
title: "Git-history workflow-cost measurement manual, plus the pre-removal baseline run"
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
related:
  260909-research-ws-refoundation-evidence-audit: evidence audit; records that telemetry-before-removal was rejected in favor of this manual
  260909-refactor-drain-ready-queue-worker-spawner: consumer; must not land before this ticket's baseline run exists
  260909-refactor-lead-surface-collapse-worker-stop-protocol: consumer; must not land before this ticket's baseline run exists
sage-review-completeness: completed
sage-review-design-reviewed: 5fb4afc0098c7c4a
sage-review-completeness-reviewed: 5fb4afc0098c7c4a
completed: 2026-09-09
---

# Git-history workflow-cost measurement manual, plus the pre-removal baseline run

## Background

The epic `260909-epic-ws-worker-interpreter-refoundation` retires several
workflow layers on the strength of its truth criterion (Cross-Child Decision 1)
and it names this ticket the prerequisite for every removal. Its Completion
Criteria bind the epic's own verdict to a measurement: "the measurement manual
has been applied once before and once after the removals on this repository",
and its Dropped condition is defined in terms of that measurement showing
worker-interpreter runs abort or re-work more than the current pipeline.

Nothing in this repository can currently produce that comparison. The evidence
audit (`260909-research-ws-refoundation-evidence-audit`, "Where the cost
actually is") states plainly that no wall-clock or token telemetry has ever
existed here, and that the twelve or more prior diets all measured line counts
or tool counts — a proxy for the artifact, not for what the workflow costs to
run. A removal justified only by "fewer lines shipped" cannot be falsified.

What is available instead is history: the ticket tree records intent and
closure dates, commit messages record rationale and ticket linkage, and phase
`### Result` sections record what actually happened during a run. That material
is already written for other reasons, it is immutable once committed, and it
can be re-read at any later commit. This ticket turns it into a stated
procedure — a manual under `ai-docs/manuals/` — and runs that procedure once on
this repository's `develop` before any layer is removed, so the epic has a
baseline to compare against.

The epic's Non-Scope is explicit that this is not telemetry instrumentation:
"Measurement is a qualitative git-history analysis manual, not runtime metrics."
The evidence audit lists "telemetry before removal" under `## Rejected
alternatives`.

## Decisions

1. **Qualitative post-hoc history analysis, not telemetry.** No runtime
   instrumentation, no counters, no wall-clock or token accounting, no new
   emitted events. Every figure is derived after the fact from `git` and the
   ticket tree, at a named commit, by commands the manual states verbatim.
   *Rejected: telemetry before removal* — recorded as rejected in the evidence
   audit and in the epic's Non-Scope. Instrumenting the runtime to justify a
   removal adds a layer to the surface under audit, delays every child behind
   an instrumentation build, and still produces no history for the "before"
   half, which has already happened.
   *Rejected: line/tool-count diffing* — the prior-diet default; it measures
   the size of the artifact, not the cost of running it, and the epic's
   Dropped condition (abort and re-work rates) cannot be expressed in it.

2. **Indicators, not a score.** The manual defines each indicator, the command
   that produces it, and what a movement in it does and does not license
   concluding. It produces no composite number and no pass/fail threshold.
   *Rejected: a single workflow-cost index* — the sample is small, the window
   is confounded by model changes and topic mix, and a composite figure invites
   exactly the false precision the epic's truth criterion is trying to escape.

3. **Both halves of the comparison use one fixed window shape.** The manual
   requires the before-run and after-run to use the same window definition
   (same selector, same size, same exclusions), and requires each run to record
   the commit it was taken at, the branch, and the exact commands. A run whose
   window shape differs from its counterpart is not comparable and the manual
   says so.

4. **The manual is project-neutral; the baseline is ticket-local.** The manual
   reads only ws conventions that bootstrap installs downstream — ticket status
   directories, the `YYMMDD-<category>-<name>` stem, `completed:`, phase
   `### Result`, the commit `## AI Context` and `## Ticket Updates` sections.
   It carries no devenv-specific numbers. This repository's baseline figures
   are recorded in this ticket's Phase 1 `### Result`, which is immutable, is
   reachable by `git log --grep=`, and is the memory tier the epic keeps
   (Cross-Child Decision 2). *Rejected: a "baseline" appendix inside the
   manual* — it would give the same measurement two homes and make the manual
   drift-prone, which is what the epic's truth criterion retires layers for.

5. **Missing conventions are recorded as unavailable, never substituted.**
   A project whose commits carry no `## AI Context`, or whose tickets carry no
   phases, records those indicators as unavailable for that window. The manual
   forbids inventing a proxy, because a proxy silently changes what the
   before/after comparison compares.

6. **Window: the 20 most recently closed actionable tickets.** Selector: tickets
   under `.done/` whose stem category is not `epic`, `workset`, `research`,
   `idea`, or `design`, ordered by the frontmatter `completed:` date, ties by
   stem; the after-run uses the same size and additionally reports the
   partition of its window into tickets closed after the baseline commit and
   tickets shared with the baseline, because most of a 20-ticket after-window
   is the same tickets run under the old pipeline. Each run also records the
   commit convention in force (who commits, at what granularity), since
   indicators 2 and 3 move with that convention independently of re-work. *Rejected: a date range* — the
   after-run window would hold more or fewer tickets depending on pace, which
   breaks the fixed-shape rule. *Rejected: everything since a named commit* —
   unbounded, so the two halves would never share a shape.
7. **Both halves read `develop`.** Work lands on the review-track branch and
   the after-run happens once the epic branch has merged into it, so both
   windows are read on `develop` first-parent history, with the branch set
   explicitly in the manual's setup rather than derived from `HEAD` (the
   baseline is taken from the epic branch). *Rejected: the release
   boundary (`main`)* — release merges collapse per-ticket history, and the
   after-run would wait on a release.
8. **The after-run belongs to the epic.** Its Completion Criteria already
   assign it there; this ticket owns the manual and the baseline only.

## Constraints

- **Not a shipped surface, still downstream-facing.** `ai-docs/manuals/` is a
  ws convention that bootstrap establishes downstream, but the files in this
  repository's `ai-docs/manuals/` are not installed into downstream projects.
  The manual is therefore written to be *copied or re-derived* by a downstream
  project, not shipped by the plugin: its procedure text must not name this
  repository's tickets, epics, layout, migration vocabulary, or tooling
  (AGENTS.md `## Architecture Rules` 4). If the manual needs an example, the
  example must resolve to nothing outside this repository.
- **No shipped-surface edits in this ticket.** Nothing under
  `agents-plugin/`, `agents-plugin-wsflow/`, or `agents-plugin-tool/` changes
  here, so the wsflow mirroring procedure (`ai-docs/manuals/wsflow-mirroring.md`)
  does not apply. If a phase discovers it needs a shipped-surface change to
  produce an indicator, that is out of scope: record it and stop, rather than
  widening.
- **Manual conventions.** One file under `ai-docs/manuals/`, with a one-line
  `summary:` frontmatter line matching the shape of the existing manuals, so
  the manuals inventory surface picks it up.
- **Read-only measurement.** The procedure must not mutate the repository:
  no branch creation, no ticket moves, no rewriting of history. It runs
  against a checked-out commit and prints.
- **The baseline must precede every removal.** The epic names this ticket the
  prerequisite for its removal children; the baseline run must be committed
  before `260909-refactor-drain-ready-queue-worker-spawner` or
  `260909-refactor-lead-surface-collapse-worker-stop-protocol` land, otherwise
  the "before" half is measured against an already-changed pipeline.
- **Promotion gate.** Under the ticket conventions in force at authoring time,
  a non-epic ticket entering `ready/` still needs spec addressing. The epic's
  Cross-Child Decision 8 removes that half of the gate in a later child; until
  it lands, this ticket satisfies the gate at promotion time rather than
  assuming its removal.

## Prior Art

Found by search, not by surveyed coordinates:

- Existing manuals establish the file shape and the `summary:` frontmatter
  line: `ai-docs/manuals/` — `skill-authoring.md`, `ws-mcp.md`,
  `windows-dogfood.md`, `wsflow-mirroring.md`, `codex-integration.md`. Match
  their frontmatter and heading style.
- The manuals inventory is derived from that `summary:` line by the workflow
  manual surface; find it by grepping `agents-plugin-tool/` for `manuals` and
  for `summary` (see the manuals workflow-manual test alongside it) to confirm
  the frontmatter shape a new manual must satisfy to be listed.
- Ticket lifecycle facts the manual reads are defined in the bundled ticket
  conventions: read them through `convention.read(name: "ticket-conventions")`
  — status directories, immutable `YYMMDD` stem prefix, `completed:` on move
  to `.done/`, stable `### Phase N`, `### Result (<short-hash>)`, and the rule
  that history is queried by stem via `git log --grep`.
- Commit-message structure the manual reads (`## AI Context`,
  `## Ticket Updates`, `> Forward:`) is defined in this repository's
  `AGENTS.md` `### Commit Rules`; the equivalent downstream definition is the
  bootstrap `AGENTS.template.md` — find it by grepping
  `agents-plugin/skills/lead-bootstrap/` for `AI Context`.
- Blocked-run vocabulary already exists in the drain skill: grep
  `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` for `Blocked` and for
  `goal/` — the dated `## Blocked (YYYY-MM-DD)` ticket note and the
  `goal/<parent>/<slug>` branch naming are both aborted-run evidence the
  manual can count.
- Ticket-tree queries are already available as tools: `tickets.query` and
  `git.log` / `git.diff` in the ws MCP surface; the manual should prefer plain
  `git` invocations it can state verbatim so a downstream project without the
  MCP server can still run it.

## Phases

### Phase 1: Write the manual and record this repository's baseline

**Goal.** Add one manual under `ai-docs/manuals/` defining the measurement, and
run it once against this repository's `develop` at a named commit, recording
the figures in this phase's `### Result`.
The manual body is drafted at
`ai-docs/ref/refound-drafts/workflow-cost-measurement.md`; move it to
`ai-docs/manuals/workflow-cost-measurement.md`, run it, fix what the run
exposes in the placed copy, and delete the draft so the manual has one home.

**Manual content — the indicator set.** The manual defines at least these,
each with a stated command, a stated unit, and a stated interpretation limit:

1. *Ticket latency.* For each closed ticket in the window: calendar gap between
   the creation date encoded in the stem prefix and the `completed:` date, and
   the commit-count gap on the tracked branch between the ticket's first
   referencing commit and the commit that moved it into `.done/`. Report the
   distribution, not the mean alone.
2. *Commits per ticket.* Count of commits found by `git log --grep=<stem>`,
   broken down by the conventional-commit type prefix, so a ticket dominated by
   `docs` commits is distinguishable from one dominated by `fix`.
3. *Relay / re-work / revert ratio.* Within one ticket's commit set, the share
   of commits landing after the ticket's first implementation commit that are
   corrective — `fix(...)`, `revert`/`Revert`, and commits whose subject or
   body names a further review round or relay. The manual must state its
   matching rule explicitly, because this indicator is the epic's Dropped
   condition and an unstated rule makes the before/after halves incomparable.
4. *Escalations recorded in `### Result` sections.* Count of phase Results in
   the window whose text records a stop that reached the user or the lead.
   The manual states the vocabulary it matches and notes that this is a
   lower bound: unrecorded escalations are invisible by construction.
5. *Judgment items in `## AI Context`.* Count of `## AI Context` bullets in the
   window's commits that record a choice among workable alternatives (a
   decision the implementation could not re-derive), separated from bullets
   that restate what changed. The separation is a judgment call; the manual
   must define the call and require the run to record how it was applied.
6. *Aborted-run indicators.* Dated `## Blocked (YYYY-MM-DD)` notes on tickets;
   phases marked `[dropped]`; tickets in `.dropped/` that already carry
   implementation commits; goal branches whose run ended without draining.

**Manual content — the procedure.** Window definition (selector, size,
exclusions) and the requirement that both halves share it; the requirement to
record the commit, branch, and date of each run; the read-only constraint; the
graceful-degradation rule for missing conventions; and an explicit
"what this does not measure" section naming wall-clock time, token cost, and
run quality, so a later reader does not over-read the figures.

**Baseline run.** Execute the manual once against `develop` before any epic
child lands a removal. Record in this phase's `### Result`: the commit hash and
date the run was taken at, the window definition used, each indicator's value,
every indicator recorded as unavailable and why, and any place the manual's own
wording proved ambiguous during the run (the run is also the manual's first
usability test). Note explicitly that the matching after-run is the epic's, not
this ticket's.

**Verification expectations.**

- Every command printed in the manual runs as written from the repository root
  and produces the stated output shape; the run is the proof, and its output
  (or a faithful summary of it) is what the `### Result` records.
- The manual's frontmatter `summary:` line is present, one line, and the file
  appears in the manuals inventory surface alongside the existing manuals.
- The manual's procedure text contains no reference that resolves only inside
  this repository: no `26xxxx-` ticket stem, no epic name, no `agents-plugin*`
  or `install.sh` path, no migration vocabulary. Verify by reading the manual
  as a lead in a project that has never heard of this repository, and by
  grepping the manual for `26` followed by five digits, for `agents-plugin`,
  and for `devenv`.
- Repository test suites still pass unchanged; this phase touches no shipped
  surface, so a change in `agents-plugin*` test results means the phase
  overreached.
- Confirm the ticket tree is unmodified after the run (`git status` clean apart
  from the manual and this ticket).

**File touchpoints** (by search term, not line number):

- New: one file under `ai-docs/manuals/`; match the frontmatter of the
  existing files found by grepping `ai-docs/manuals/` for `summary:`.
- This ticket: `ai-docs/tickets/ready/260909-chore-ws-refoundation-git-history-measurement-manual.md`
  (Phase 1 `### Result`).
- Read-only during authoring: the bundled ticket conventions via
  `convention.read`; `AGENTS.md` `### Commit Rules`; the drain skill's
  `Blocked` and `goal/` vocabulary; the manuals-inventory implementation found
  by grepping `agents-plugin-tool/` for `manuals`.
- Expected to change: nothing under `agents-plugin/`,
  `agents-plugin-wsflow/`, or `agents-plugin-tool/`.

### Result (c96b4db2) - 2026-09-09

**Behavioral delta.** `ai-docs/manuals/workflow-cost-measurement.md` exists and
is listed by the manuals inventory surface. The draft under
`ai-docs/ref/refound-drafts/` is gone and its README row with it (plus a new
placement rule there so the next child drops its row too). Nothing under
`agents-plugin/`, `agents-plugin-wsflow/`, or `agents-plugin-tool/` changed.

**Run header.** Measured branch `develop` at `84b1f825`, 2026-09-09. Executed
from worktree HEAD `49f48229` on `epic/refound` (`develop` is an ancestor of
it), so `TICKETS` was a read-only `git archive develop` copy, not the working
tree; `.done/` is byte-identical between the two refs (384 files both sides).
Commit convention in force for the window: lead-authored commits, roughly one
per phase plus a separate `docs(ticket)` closure commit, with implementation
landing on `impl/*`/`goal/*` branches merged `--no-ff`.

**Window.** 20 stems, `completed:` 2026-08-30 through 2026-09-09 (11 calendar
days), oldest first: `260824-feat-lead-review-range-scenario`,
`260824-feat-review-release-gate-policy`, `260824-feat-review-watermark-ledger`,
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
48 closed actionable tickets were skipped for lacking `completed:` — every one
predates the convention, so the window is unaffected, but the after-run must
report the same figure or the two windows span different periods.

**1. Ticket latency.** Days (stem date → `completed:`): min 0, median 0.5,
max 6; distribution 0×10, 1×6, 2×1, 6×3. First-parent commit gap: min 0,
median 14, max 57. The three 6-day / 34–57-commit rows are the `260824`
review-watermark epic children, which sat in `todo/` after a user demotion; the
median row is a same-or-next-day close. Read as latency, not effort.

**2. Commits per ticket, by type.** 137 stem-referencing commits across the 20
tickets: `docs` 81 (59%), `chore` 17, `merge` 17, `feat` 10, `fix` 4,
`refactor` 4, `test` 3, `plan` 1. Seven of twenty tickets have *no* commit
typed as product work at all on `develop` (`260828-refactor-per-slice-review-relay`,
`260831-refactor-severity-graded-per-slice-review-relay`,
`260903-refactor-mcp-todo-signature-merge`,
`260906-bug-route-opaque-params-handler-mismatch`,
`260907-feat-ws-project-tree-parent-nested-ticket-render`,
`260908-bug-sage-gate-stale-completed-has-no-rerun-path`,
`260908-feat-implement-skip-survey-for-localized-ticket-target`) — the same
seven that indicator 3 reports as `unavailable`. This is the single most load-bearing
number in the baseline: under the current convention the pipeline's visible
git output is overwhelmingly ticket bookkeeping, and product commits reach
`develop` inside merges that do not name the stem.

**3. Corrective share.** 1 corrective commit out of 52 post-implementation
commits across the 12 measurable tickets; only
`260824-feat-review-watermark-ledger` has one (`fix(260824): validate Ref
shape…`, 1 of 8). 7 tickets are `unavailable (no implementation commit)` and 1
is `n/a (no post-implementation commits)` — so 8 of 20 (40%) of the window
cannot be measured on the epic's Dropped-condition indicator at all, because
their implementation commits never name the stem. **A after-run that reports a
higher corrective share must first check this count**: if the worker-interpreter
convention has one worker naming the stem in its own commits, `unavailable`
falls and `corrective` rises without any change in re-work.

**4. Escalations recorded in `### Result`.** 18 measurable; 2
`unavailable (no phase Result)` (`260830-bug-review-nudge-trackless-bootstrap-gap`,
`260904-bug-windows-parent-watch-pid-reuse-flake` — single-commit tickets whose
close carries no Result section). 2 tickets nonzero, 10 matching lines total
(`260831-bug-survey-plan-unilateral-scope-reduction` 4,
`260908-feat-survey-plan-is-route-not-contract` 6). Every one of the 10 lines
was read: all are the tickets' own design vocabulary (`[escalate-to-lead]`,
`[escalate-to-research]`) discussed in the abstract, not a recorded stop. The
honest baseline figure is therefore **zero recorded escalations in 20 tickets**,
with 10 false-positive lines, and the indicator's stated
"abstract-discussion counts" limit is not theoretical.

**5. Judgment items in `## AI Context`.** 326 bullets across 115 commits.
Classified by four independent readers over four equal partitions, each given
the manual's rule verbatim and nothing else: 172 judgment, 154 narration (53%).
The per-partition judgment share ranged 41%–69% (34/83, 40/79, 41/81, 57/83),
which is the indicator's stated measurer-dependence made visible; an after-run
must classify the same way (four blind readers, one rule, no cross-talk) or the
halves are not comparable. Borderline cases the readers flagged, with the class
assigned: *"Delegated survey plan (plan-populator-survey, sonnet); confidence
high, no escalate-to-research."* → judgment, because it names a routing
alternative not taken; *"Single-phase ticket; Phase 1 landing completes it, so
it closes to done rather than staying in ready/."* → narration, because the
named alternative is a mechanical consequence of a rule, not a choice among
workable options. That second line is the boundary the after-run should hold:
a `because` clause is not by itself a judgment item.

**6. Aborted-run indicators.** Blocked notes 9; dropped phases 8; dropped
tickets carrying implementation commits 9; `merge(goal)` merges on `develop`
14; `goal/*` branches not merged into `develop` 5 —
`goal/drain-ready-queue`, `goal/ws-dashboard-dev/copper-heron-vale`,
`goal/ws-dashboard-dev/marlin-cove-thistle`, `goal/ws-dashboard-related-tickets`,
`goal/ws-dashboard-tickets`. All five last committed in July and sit hundreds
of commits off `develop`: abandoned, not in flight. These are whole-tree
cumulative counts; the after-run compares them only as a difference.

**Deviations from the phase plan.** None in scope; the plan said to fix what
the run exposed in the placed copy, and it did. The corrections are listed in
`c96b4db2`'s `## AI Context`.

**Where the manual's own wording proved wrong (its first usability test).**
Every one of these was found by running it, not by reading it:
- *Self-name vocabulary collision.* The draft's corrective rule matched
  `relay`/`review round` anywhere in subject or body. Every commit that
  references a ticket reproduces the ticket's title, so
  `260831-refactor-severity-graded-per-slice-review-relay` scored 4 of 4
  corrective with zero actual re-work, and restricting to the subject line
  only lowered it to 3 of 4 — the feature's own name is in the subject too.
  Fixed by stripping the stem's words before matching.
- *Lifecycle commits read as implementation.* The draft excluded only `docs`
  and `merge`, so `chore(<other-stem>): promote … to ready` became a ticket's
  "first implementation commit". Fixed with an allow-list of product types,
  shared by indicators 3 and 6.
- *Administrative drop sweeps read as aborts.* Indicator 6 reported 19 dropped
  tickets with implementation commits; a single
  `chore(ticket): … drop libws/agent/leaf tickets` sweep accounted for four of
  them. With the shared selector the count is 9, and two of those nine are
  still package-scoped commits that merely mention the stem — a stated residual.
- *Wrapped bullets truncated.* Indicator 5's rule emitted only lines starting
  with `- `; commit bodies are hard-wrapped, so most bullets arrived cut
  mid-sentence and the first classification pass was made on half-sentences.
  Fixed by rejoining continuations, and the classification was re-run.
- *A merged branch counted as unmerged.* `git branch --list 'goal/*'` never
  tests merged-ness; `goal/develop/amber-lantern-drift` is fully merged and was
  counted as an abort. Fixed with `--no-merged`.
- *Absent conventions reported as zero.* The Rules said missing conventions are
  `unavailable`, but no indicator command could express it. Indicators 3, 4 and
  5 now carry explicit guards.
- *Branch/tree split.* The commit indicators read `$BRANCH` while the ticket
  reads came from the working tree, which disagree whenever the run is taken off
  the tracked branch — exactly this run's situation. The manual now
  parameterizes `TICKETS` and gives a read-only materialization step, and warns
  that git pathspecs must stay repo-relative.
- *No end state.* The draft said "record" repeatedly and never said where or
  what finished looks like. A `## Record` section now does.

**Verification.**
- Every `sh` block in the placed manual extracted in document order and run
  verbatim from the repository root under `sh`, `bash` and `dash`: identical
  output in all three, no errors, no stderr beyond the documented
  `skipped (no completed:)` lines.
- Manuals inventory (`wsdoc.ManualsList`) lists the file with its one-line
  `summary:` alongside the five existing manuals.
- Leak greps on the manual: no `26xxxx` stem, no `agents-plugin*`, no
  `install.sh`, no `wsflow`, no `devenv`, no migration vocabulary. A
  fresh-reader review that saw only this one file, with no repository access,
  raised no reference it could not resolve from the file itself.
- Go suite `./...` green uncached (14 packages).
- Repository tree unmodified by the run: `git status` shows only the manual,
  the drafts README, and a pre-existing untracked `ws-mcp` build artifact.

**Unresolved / carried forward.** `agents-plugin/tests/test_skill_dispatch_contracts.py::test_proceed_keeps_implementation_route_only` fails on
`develop` and on this branch, asserting a sentence
("Route only; do not implement or plan here.") that no longer exists in the
`lead-proceed` playbook. `agents-plugin/` is byte-identical between `develop`
and this branch and this phase touched nothing there, so the failure predates
this work; it belongs to whichever child edits that surface.

**The matching after-run is the epic's, not this ticket's.** Its Completion
Criteria own it, and its Cross-Child Decision 19 names the hand-dogfood run of
the removal children as the first after-sample — which includes this ticket's
own execution.

#### Edition (6a5947ab) - 2026-09-09

Independent review of the placed manual and of the Result above (round 2, three
reviewers, none of them the author) found the manual asserting numbers its
commands could not justify, and this Result carrying six errors of its own. The
manual fixes are in `6a5947ab`; the Result above is frozen, so the corrections
are here. **Where this Edition and the Result disagree, this Edition is the
baseline.** The baseline is pinned to the manual as of `6a5947ab`
(placed at `c96b4db2`); an after-run must use that version or later, because the
figures below are not comparable against the `c96b4db2` indicator 3 rule.

**Manual corrections and their effect on the figures.**

- *Indicator 3's free-text corrective vocabulary is gone.* The stem-word
  stripping recorded above as the fix was itself broken: `gsub` is a substring
  operation, so any `*-review-*` stem also destroys `re-review` and
  `review round` in unrelated subjects — the false positives became silent
  false negatives. The automatic rule is now typed `fix`/`revert` only, plus a
  required hand read of the printed post-implementation subjects. Re-run figure
  is unchanged at **1 corrective of 52 post-implementation commits across 12
  measurable tickets**; the hand read of all 52 subjects adds none. What
  changed is that the number is now defensible.
- *Indicator 3 prints its anchor, and two anchors in this window are wrong.*
  `260824-feat-review-release-gate-policy` anchors on `f3ac6a20`
  (`fix(lead-ship): stop for explicit decision on an empty/no-marker ledger`)
  and `260908-feat-survey-plan-is-route-not-contract` on `10d38f80`
  (`feat(agents-plugin): stop dispatching plan artifact to reviewers`). Neither
  is a first implementation commit: both are a *later phase's* implementation,
  because the earlier phase's commits never name the stem on `develop`. Their
  denominators (2 and 4) cover the tail of the ticket only. So the honest
  reading of indicator 3 in this window is **1 of 52 across 10 soundly anchored
  tickets, 2 anchored late, 7 unavailable, 1 with no post-implementation
  commits** — 10 of 20 tickets (50%), not 8 of 20 (40%), carry a measurement
  defect traceable to the same cause.
- *Indicator 4 prints its matched lines and asks for two numbers.* Re-run:
  **10 lines read, 0 judged to record an actual stop.** The Result above says
  the same thing; the manual now requires both numbers rather than leaving the
  second to the measurer's discretion. Of the 10, nine are plainly abstract
  design vocabulary; the tenth
  (`260908-feat-survey-plan-is-route-not-contract`, "ruling — only strings that
  already named `[escalate-to-research]` …") records a lead ruling the ticket
  received, which is adjacent to a stop but is not one. Zero stands.
- *Indicator 6's dropped-ticket rows are printed and were read.* Re-run prints
  9 rows; **4 are genuine** (`260405-research-marathon-delegation-hardening`,
  `260425-chore-mental-model-index-migration` — a `revert:` of shipped work,
  the strongest abort in the tree —, and both
  `260626-bug-prefer-subagent-*` tickets) and **5 are false positives**: two
  `feat(ticket): <stem>` commits that only create the ticket
  (`260421-feat-rebuild-spec-skill`, `260429-feat-api-deps`), one follow-up
  note under another ticket's `## Ticket Updates`
  (`260505-bug-plugin-managed-default-root-discovery`), one forward dependency
  (`260513-feat-async-exec-output-reader`), and one commit that *is* the drop
  (`260524-bug-wsstore-ci-sqlite-busy`). The Result above's "two of those nine
  are still package-scoped commits" understates the residual by more than half.
  Baseline: **dropped with implementation commits: 9 printed, 4 real.**

**Corrections to this Result's own claims.**

- *"48 closed actionable tickets were skipped … every one predates the
  convention"* is false. The 48 span `completed:`-less closures from
  2026-04-29 to 2026-08-27 and are convention gaps, not a pre-convention era.
  The conclusion still holds for a different reason: the newest of the 48
  closed on 2026-08-27, below the window floor of 2026-08-30, so none of them
  would have entered the window. The after-run must still compare the count.
- *"137 stem-referencing commits"* is a sum of per-row counts; commits that
  name two stems are counted twice. **Distinct commits: 115.** The type
  breakdown is likewise per-row. Both windows must be counted the same way for
  the comparison to hold.
- *"product commits reach `develop` inside merges that do not name the stem"*
  is backwards. The merge commits do name the stem — that is why they appear in
  the `merge 17` row. It is the product commits *inside* those merges that do
  not. The consequence is the same and the sentence was wrong.
- *"alongside the five existing manuals"* — there are **six**.
- *Indicator 5's "53%"* is the judgment share (172 of 326), not a narration or
  error rate; the Result reads ambiguously.
- The four-blind-reader classification protocol used for indicator 5 is
  **not encoded in the manual**. The manual states the measurer-dependence and
  says to keep the classifier fixed; it does not prescribe four partitions and
  four independent readers. An after-run that wants comparability must copy the
  protocol from this Edition, not from the manual.

**Per-ticket rows the Result summarized instead of listing.**

Indicator 1, `<stem> <days> / <first-parent commit gap>`, window order:

```text
260824-feat-lead-review-range-scenario 6/34
260824-feat-review-release-gate-policy 6/57
260824-feat-review-watermark-ledger 6/41
260828-refactor-per-slice-review-relay 2/24
260830-bug-review-nudge-trackless-bootstrap-gap 0/0
260830-feat-sage-freshness-content-baseline 0/9
260831-bug-survey-plan-unilateral-scope-reduction 0/2
260831-refactor-severity-graded-per-slice-review-relay 0/7
260901-feat-note-oversize-layer-aware-clone-path 0/6
260903-refactor-mcp-read-surface-collapse 1/14
260903-refactor-mcp-todo-signature-merge 1/14
260903-refactor-mcp-verb-vocabulary-unification 1/14
260904-bug-windows-parent-watch-pid-reuse-flake 0/1
260904-refactor-enter-affordance-rename-route-opaque 0/15
260906-bug-route-opaque-params-handler-mismatch 0/5
260907-feat-ws-project-tree-parent-nested-ticket-render 0/6
260908-feat-survey-plan-is-route-not-contract 0/17
260908-bug-sage-gate-stale-completed-has-no-rerun-path 1/15
260908-bug-shipped-surfaces-carry-devenv-only-content 1/12
260908-feat-implement-skip-survey-for-localized-ticket-target 1/16
```

Indicator 2, per-stem type counts (per-row; a commit naming two stems
appears in both rows):

```text
260824-feat-lead-review-range-scenario chore=3 docs=2 feat=2
260824-feat-review-release-gate-policy chore=4 docs=4 fix=1
260824-feat-review-watermark-ledger chore=3 docs=8 feat=2 fix=1
260828-refactor-per-slice-review-relay chore=1 docs=2
260830-bug-review-nudge-trackless-bootstrap-gap fix=1
260830-feat-sage-freshness-content-baseline chore=2 docs=2 feat=1
260831-bug-survey-plan-unilateral-scope-reduction chore=1 docs=3 feat=1 merge=1
260831-refactor-severity-graded-per-slice-review-relay chore=2 docs=1 merge=1 plan=1
260901-feat-note-oversize-layer-aware-clone-path chore=1 docs=3 feat=2
260903-refactor-mcp-read-surface-collapse docs=6 merge=1 refactor=1
260903-refactor-mcp-todo-signature-merge docs=7
260903-refactor-mcp-verb-vocabulary-unification docs=6 merge=1 refactor=1
260904-bug-windows-parent-watch-pid-reuse-flake docs=1 fix=1
260904-refactor-enter-affordance-rename-route-opaque docs=7 merge=2 refactor=2
260906-bug-route-opaque-params-handler-mismatch docs=2
260907-feat-ws-project-tree-parent-nested-ticket-render docs=4 merge=1
260908-feat-survey-plan-is-route-not-contract docs=9 feat=1 merge=2
260908-bug-sage-gate-stale-completed-has-no-rerun-path docs=5 merge=2
260908-bug-shipped-surfaces-carry-devenv-only-content docs=5 feat=1 merge=4 test=3
260908-feat-implement-skip-survey-for-localized-ticket-target docs=4 merge=2
```

The seven tickets with no product-typed commit naming them on `develop`
(indicator 2's zero rows and indicator 3's `unavailable` rows, the same set):
`260828-refactor-per-slice-review-relay`,
`260831-refactor-severity-graded-per-slice-review-relay`,
`260903-refactor-mcp-todo-signature-merge`,
`260906-bug-route-opaque-params-handler-mismatch`,
`260907-feat-ws-project-tree-parent-nested-ticket-render`,
`260908-bug-sage-gate-stale-completed-has-no-rerun-path`,
`260908-feat-implement-skip-survey-for-localized-ticket-target`.

Indicator 3 per-ticket, `corrective of post-implementation`, anchored tickets
only: `260824-feat-lead-review-range-scenario` 0/3;
`260824-feat-review-release-gate-policy` 0/2 (late anchor);
`260824-feat-review-watermark-ledger` 1/8;
`260830-bug-review-nudge-trackless-bootstrap-gap` n/a;
`260830-feat-sage-freshness-content-baseline` 0/3;
`260831-bug-survey-plan-unilateral-scope-reduction` 0/4;
`260901-feat-note-oversize-layer-aware-clone-path` 0/4;
`260903-refactor-mcp-read-surface-collapse` 0/3;
`260903-refactor-mcp-verb-vocabulary-unification` 0/3;
`260904-bug-windows-parent-watch-pid-reuse-flake` 0/1;
`260904-refactor-enter-affordance-rename-route-opaque` 0/7;
`260908-feat-survey-plan-is-route-not-contract` 0/4 (late anchor);
`260908-bug-shipped-surfaces-carry-devenv-only-content` 0/10.

**Attribution gap in this ticket's commits.** `c96b4db2` and `78573192` carry
no `Co-Authored-By`/`Claude-Session` trailers. Recreating them to add the
trailers collided with a concurrent session that landed `3fcaa003` on this
branch mid-operation; the recovery was `git reset --hard 3fcaa003`, restoring
the exact original history. The trailers are not worth rewriting shared history
under a live concurrent writer, so the gap stands. `6a5947ab` and later carry
them.

**Verification of this Edition.** Indicators 3, 4 and 6 re-run verbatim from
the `6a5947ab` manual against `develop` at `84b1f825` (unmoved since the
original run), from a read-only `git archive develop` ticket tree; the `sh`
blocks are byte-identical to those verified across `sh`/`bash`/`dash` in
`c96b4db2`.

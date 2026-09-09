---
title: "Git-history workflow-cost measurement manual, plus the pre-removal baseline run"
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: required
related:
  260909-research-ws-refoundation-evidence-audit: evidence audit; records that telemetry-before-removal was rejected in favor of this manual
  260909-refactor-drain-ready-queue-worker-spawner: consumer; must not land before this ticket's baseline run exists
  260909-refactor-lead-surface-collapse-worker-stop-protocol: consumer; must not land before this ticket's baseline run exists
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
- This ticket: `ai-docs/tickets/todo/260909-chore-ws-refoundation-git-history-measurement-manual.md`
  (Phase 1 `### Result`).
- Read-only during authoring: the bundled ticket conventions via
  `convention.read`; `AGENTS.md` `### Commit Rules`; the drain skill's
  `Blocked` and `goal/` vocabulary; the manuals-inventory implementation found
  by grepping `agents-plugin-tool/` for `manuals`.
- Expected to change: nothing under `agents-plugin/`,
  `agents-plugin-wsflow/`, or `agents-plugin-tool/`.

## Open Questions

- **Window size and selector for the baseline.** The epic settles that a
  before/after comparison happens and that both halves must be comparable, but
  not how large the window is or how tickets are selected into it (last N
  closed tickets, a date range, or all tickets closed since a named commit).
  The manual must fix one, and the choice determines whether the after-run has
  enough closed tickets to compare against. Settle at design review.
- **Which branch the after-run measures.** This repository lands work on
  `develop` and ships `develop` -> `main`. If the epic's removals land through
  goal branches merged into `develop`, the after-run window may straddle merge
  commits that the baseline window does not contain. The epic does not say
  whether the comparison is taken on `develop` in both halves or on the
  release boundary.
- **Whether the after-run is a separate ticket.** The epic's Completion
  Criteria assign the after-run to the epic itself rather than to a child.
  Left as stated; if design review wants the after-run owned by a ticket, that
  is a new child, not a phase here.

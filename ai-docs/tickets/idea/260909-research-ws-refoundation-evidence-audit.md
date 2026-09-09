---
title: "Evidence audit behind the ws refoundation: assumption verdicts, cost centers, rejected alternatives"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260605-research-ws-native-subagent-pivot: prior binding anchor; harness-infrastructure axis only
---

# Evidence audit behind the ws refoundation

## Background

The owner's thesis: the ws workflow was carried over from earlier model
generations, where it enforced common-sense implementation and respect for
project conventions; with current models it acts as ballast, because capable
models follow the workflow's conservative biases faithfully where earlier
models ignored them. The owner asked for a destructive re-evaluation: not
"adjust", but "is the workflow built on assumptions that are now false".

This ticket records the evidence gathered by five parallel audits (anchor
direction, ceremony cost, spec/mental-model efficacy, bootstrap coupling,
prior simplification history) plus one harness-capability check, and the
verdicts the owner confirmed. It is intended to become the repository's
binding anchor for the topics: lead surface, worker interpreter,
document-layer retirement, stop conditions.

## Truth criterion

Confirmed by the owner. A layer survives only if reconstructing its value on
demand with a current model (code, tests, git history, current context
budget) costs more than maintaining it. Two facts sharpen it:

- Humans never audit `ai-docs/`; every human-facing answer is an AI digest.
  A drifted document is therefore a stale intermediary with negative value.
- The owner's prior simplification path was already "deterministic, tool
  reconstructable" almost every time; this criterion pushes that to its end.

## Assumption verdicts

| # | Assumption | Evidence | Verdict |
|---|---|---|---|
| A1 | Models cannot re-derive internal structure per session, so a hand-maintained summary (mental-model) must exist | One Explore dispatch derives a domain; the "not derivable in 30 seconds" inclusion test was written for a 200K-context era. Non-derivable facts (runtime traps, rejected alternatives) already live in `## AI Context` and tickets | false |
| A2 | Docs must be synced procedurally at write time; late drift is costlier | About 4.4% of commits are `docs(spec)` and 3.3% carry `(mental-model-updated)`, yet the spec corpus was once archived wholesale and rebuilt (`b20890e9`), a sample of 8 items showed 2 contradicting code, and mental-model prose is explicitly excluded from rename discipline (`260904-refactor-mental-model-doc-drift-epic-renames`). The working maintenance mode was regeneration (`forge-*`), not increments | false |
| A3 | Lead context is scarce, so the lead stays thin and delegates with self-contained prompts | The session that ran this audit had a 15M-token budget. The delegation half stays true: information loss at delegation hops happened with current models (`260908` survey paraphrase dropped a load-bearing detail; `260831` planner narrowed scope unilaterally). Redefined: the scarce resource is lead turns and lead reasoning per turn, not lead context | half false; delegation stays the default, lead mediation does not |
| A4 | Spec must precede implementation so code is not built on an unstable contract | Spec accuracy 4/5 in sample; only one consumption point changes an agent's action (design-review spec-conflict check); the "spec drift" review checklist item never produced a recorded finding; the ticket already states the contract | true premise, wrong placement |
| A5 | The lead must not touch code before routing | The anchor kept this because eager models skipped routing; current models fail the opposite way. Moot once the lead never edits source | expired |
| A6 | The ticket board is a queue | About 120 tickets in `idea/`+`todo/`, zero in `ready/`; creation is cheap, promotion is lead-bound and expensive | false under current gates; fixed by moving promotion work off the lead |
| A7 | Independent review catches the author's blind spots | Collapsing the review relay (`260828`) raised abort rates and was partially restored six days later (`260831`); "three reviewers on a three-line change" was the over-escalation, not review itself | true |
| A8 | Procedural prose (judge tables, handlers) is how agent behavior is produced | About 5,000 lines of playbook/skill prose, 37 judge blocks, 51 handlers; neither deterministic nor leveraging model judgment; biases like "prefer triggering over skipping" and "when in doubt fall through to full routing" are what capable models obey | false |

## Where the cost actually is

Mandated reads on the routed path for a one-line fix, before any source is
read (line counts from the shipped tree at audit time):

| Document | Lines |
|---|---|
| AGENTS.md | 357 |
| lead-workflow-manual | 295 |
| lead-implement | 288 |
| lead-proceed | 118 |
| impl-playbook (Go-injected into every `prep` todo, unconditional) | 75 |
| one mental-model doc (assumed) | ~150 |
| lead-update-spec + spec-conventions + lead-write-spec | 294 |
| total | ~1,577 (about 19K tokens) |

Floor about 13.6K tokens (doc skip, current branch); ceiling about 30K with
delegation, three-way review, one relay; full ceremony adds about 7.6K before
implementation starts. The `ai-docs/spec/` and `mental-model/` files are one
row of this table; the rest is the workflow describing itself to the lead.
The `prep` guardrail is a Go template string (`session_state.go`,
`implementPrepInstruction`), invisible to a docs-only audit.

Prior diets (twelve or more landed since April 2026: playbook diet, sprint and
salvage retirement, MCP surface 62 to fewer, survey-plan removal, review
floor) all measured line or tool counts. No wall-clock or token telemetry has
ever existed in this repository.

## Spec versus mental-model

Definitions are mostly operational (anchor regex with 228 unique anchors and
zero duplicates; six enumerated sage postures; watermark frontier as a pure
function). Only `domain` and `modification-focused` are vague. The real
defect is write-heavy, read-light, drift-prone content, not vague terms.

- Mental-model holds three kinds of knowledge: derivable structure (Explore
  gets it), runtime traps (belong next to the code that bites, where a
  premise-changing edit must touch the same file), and rationale (already in
  `## AI Context`). Every kind has a better home; the layer is a copy.
- Spec is an external contract that changes only when behavior changes, so it
  drifts slowly and has a real consumer, but it was placed as a `ready/`
  precondition where it blocks promotion, while nobody checks it against code
  after implementation. With tests as the contract and no human readers, it
  is retired rather than moved.
- Repository-specific caveat: this repository's specs describe prose
  (playbooks), so drift is worse here than in a downstream code project.

## Bootstrap coupling

Downstream blast radius is small: this repository plus a disposable fixture.
The cost is internal Go coupling: ticket status directories, the
`{#YYMMDD-slug}` anchor regex as the cross-reference key across tickets,
mental-models and commit trailers, and the mental-model directory. Two
precedents (`260807` index dissolution, `260825` template convergence) both
found compiled-code dependencies (`doctor.go`, `bump-ws-version.sh`) only
during execution. Deleting a layer is cheaper than reshaping it: hidden
dependencies surface as compile and test failures.

## Harness capability premises

- Depth-1 native recursion (worker spawns Explore-class children) is
  confirmed by the owner on Claude Code, pi, and Codex. Deeper nesting is
  neither blocked nor relied upon; depth is a prose recommendation.
- The worker-as-interpreter mechanism already exists once:
  `lead-goal-fan-out-step` mints a lead-capability child key via
  `ferrule(capability: "lead", parent_session_key: ...)`, the mini-lead runs
  `lead-proceed` itself, stops at the merge gate, and the lead re-discovers
  it via `session.children`. The fan-out entry point retires; the mechanism
  moves into `drain-ready-queue`.
- No existing playbook combines that mini-lead pattern with the
  delegate-orientation rule "report user decisions to the lead". The
  stop-and-report-then-resume idiom is a new composition.
- `route.resolve_*` key requirements are undocumented; the worker holds a
  lead-scoped child key, so the question dissolves at the drain point.
- `ai-docs/ref/agent-harness-capability-tiers.md` has no Claude Code or pi
  column; recursion claims there are playbook prose, not fixtures.
- Mercenary is nowhere marked deprecated in specs or tickets; the owner's
  decision to deprecate it is new and recorded in the epic.

## Rejected alternatives

- **Destructive `ai-docs/` layout restructure as the primary lever.**
  Rejected: the layout is not the cost center; reshaping the anchor key has
  the highest Go fan-out while deleting the layer has the lowest.
- **ws runtime as the workflow interpreter (mercenary as main path).**
  Rejected by the owner: mercenary is a deprecation target; only native
  harness delegation is supported.
- **Lead does more inline with its large context ("thin lead" inverted to
  "fat lead").** Rejected by the owner: the lead is the expensive, slow,
  careful model; work goes to cheaper workers; lead judgment is spent only on
  escalation.
- **Haiku-class workers with pre-digested docs as their context.** Rejected:
  workers are current-mainstream or previous-flagship class and derive
  context from source pointers; digests are an optional untracked cache.
- **Worker stops at every phase boundary for visibility.** Rejected: each
  interim report is a lead turn; the worker stops only on the closed list in
  the epic, and batches its own decisions into the merge-stop report.
- **Merging fact population and design review into one two-tier pass.**
  Not decided; deferred in the epic.
- **Telemetry before removal.** Replaced by a qualitative git-history
  analysis manual applied before and after.

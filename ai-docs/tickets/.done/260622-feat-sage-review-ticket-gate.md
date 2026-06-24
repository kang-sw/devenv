---
title: Sage review — design-quality gate for ticket writes
related:
  260620-feat-ws-ticket-status-transition-tools: transition tool must check sage-review frontmatter field; this ticket builds the sage reviewer playbook and create-ticket surface on top
related-mental-model:
  - workflow-skills
completed: 2026-06-24
---

# Sage review — design-quality gate for ticket writes

## Background

Lead agents at Sonnet tier can miss design-level problems at ticket-write time.
Large-model (Opus) sessions are too slow for routine mechanical work. The gap:
bad design decisions get locked into tickets — especially at `todo/` level where
system-wide design accumulates — before anyone stress-tests them from a fresh
perspective.

Proposed gate: after writing a ticket to `todo/` or above, optionally spawn a
large-tier subagent as a fresh implementer — full repo access, no conversation
context injected — and ask whether it can produce a coherent implementation plan.
If it cannot, the ticket is flagged before implementation starts.

## Decisions

### Status convention

- **`idea/` = discussion scratchpad.** Tickets may be created at `idea/` early and
  updated freely as design is explored. No sage review, no spec-address gate.
  `create-ticket` tip for `idea/` creation: "promoting to `todo/` will trigger
  sage review."
- **`todo/` = semi-frozen artifact.** Landing here signals that design is committed
  and sage-reviewed. Casual design changes should demote back to `idea/` first.
- **`ready/` = picked up for implementation.**

### Write-ticket judge gate

`lead-write-ticket` classifies every write request before routing:

| Intent | Target |
|--------|--------|
| Proceed context — implementation starting now | `ready/` |
| Design confirmed (new ticket or idea/ promotion) | `todo/` + sage ask |
| Substantive design change to existing `todo/` | demote to `idea/`, continue discussion |
| Non-substantive edit to `todo/` (wording, detail, related links) | `todo/` stays, no re-review |
| Any edit to `idea/` | `idea/` stays, no sage |

### Sage review mechanics

- **Opt-in via config**: `sage_review: auto | ask | off` (default: `off`).
  Resolves through session > proj > user config layers.
- **Trigger**: any write landing at `todo/+` when config is on.
- **Fresh reader framing**: sage receives ticket + all linked docs (full repo
  access); zero conversation context injected. Goal: produce a coherent
  implementation plan sketch. Inability to do so is a signal.
- **Verdict-based, not direct-edit**: reviewers return structured verdicts; lead
  synthesizes and writes `## Blocked (YYYY-MM-DD)` + updates frontmatter. Sage
  does not edit the ticket body directly.
- **`sage-review` frontmatter field**: `pending | completed | blocked | skipped`.
  Set to `pending` on creation at `todo/+`. `skipped` bypasses the hard lock with
  an audit trail.
- **MCP transition enforcement**: the ticket transition tool (260620) checks the
  `sage-review` field when config is on — fails transition on `pending | blocked`.
  Sage-agnostic: field read only.
- **Block output**: lead writes `## Blocked (YYYY-MM-DD)` section (h2, consistent
  with ticket heading convention) from reviewer verdict; sets `sage-review:
  blocked`. Actual design edits are then discussed and demote the ticket to
  `idea/` before a new review cycle.

### Reviewer model

Two reviewers run in parallel:

| Reviewer | Default tier | Dimensions |
|----------|-------------|------------|
| Design | `large` (configurable) | design coherence, duct-tape detection, right-problem check, autonomous-vs-missing gap judgment |
| Completeness | `medium` (configurable) | ticket structure, missing fields, "unclear to fresh reader" items |

Aggregation: design `block` → final block regardless of completeness. Completeness
`concern` → lead judgment on whether to block.

Config keys (all scalar, layerable):

```
sage_review: auto | ask | off
sage_review_design_tier: large
sage_review_completeness: true | false
sage_review_completeness_tier: medium
```

### `create-ticket` MCP tool

New `ws/tickets.create(session_key, stem, initial_state)`:
- Auto-prefixes today's date to semantic `stem`.
- Writes frontmatter stub (title placeholder, `sage-review: pending` for `todo/+`).
- Returns `{path, tip}`:
  - `idea/`: tip = "promoting to `todo/` will trigger sage review."
  - `todo/+`: tip = "run sage review before promoting further."

### `create-ticket` and `lead-write-ticket` relationship

`create-ticket` is an MCP tool (stub write + frontmatter). `lead-write-ticket` is
a playbook that calls `create-ticket` for the actual file creation, then wraps it
with consent gate, intent review, spec-address check, and cross-ticket decision
review. Different layers — no conflict.

### `ask` mode interaction

When `sage_review: ask`, after the ticket write the lead agent asks the user
whether to run sage review. A "no" answer sets `sage-review: skipped` and
proceeds. A "yes" (or `auto` mode) runs the reviewers.

### `skipped` write authority

`sage-review: skipped` is set only when the user explicitly declines in `ask`
mode. It is never set when `sage_review: off` — in that mode the `sage-review`
field is not checked by the transition tool at all.

### Ticket demotion via `tickets.move`

`tickets.move` handles both upward promotion and downward demotion. On downward
move from `ready/`, the tool returns a tip: "This ticket had spec entries; clear
`spec:`, `spec-remove:`, and review `## Spec Impact` before re-promoting."

Spec cleanup is not automatic — the tip directs the caller. This matches the
`create-ticket` tip pattern. The `## Spec Impact` section is preserved as a
historical trace; only frontmatter `spec:` / `spec-remove:` entries need clearing.

`lead-workflow-manual` carries a strong directive to use `tickets.move` for all
ticket moves (promotion and demotion alike); it does not repeat spec cleanup
instructions. That detail belongs in the tool tip and ticket conventions.

260620 scope expansion required: Phase 1 of 260620 must include downward demotion
support and the ready-demotion tip alongside the existing upward promotion path.

### `todo/` direct-edit guard

`lead-write-ticket` and `lead-workflow-manual` must state: do not directly edit a
`todo/` ticket's design sections (`## Decisions`, `## Phases`, `## Constraints`,
`## Background`) without first demoting to `idea/`. File edits are too easy to
bypass otherwise. The judge gate in `lead-write-ticket` enforces this for
playbook-mediated writes; the manual sets the expectation for all paths.

### 260620 coupling

260620 implements the transition tool. The `sage-review` pre-condition hook
belongs in Phase 1 of 260620 (alongside the mutation path — natural fit).
This ticket builds the sage reviewer playbooks, `create-ticket` tool, and config
integration on top.

## Closed Questions

- **`todo/ → ready/` re-review**: no re-review. `todo/` is a "sterile room" —
  any substantive change after sage review must go through idea/ demotion + new
  review cycle first. By definition, a ticket ready for `ready/` promotion is
  already up-to-date with its last sage review. Transition tool checks
  `sage-review: completed` only.
- **`create-ticket` `parent:` inference**: out of scope for Phase 1. No
  well-defined "active epic" concept in the session; stub leaves `parent:` empty
  for the caller to fill. Separate idea ticket if needed later.

## Spec Impact

- **Phase 1** — `ai-docs/spec/mcp-tools.md`: add `{#260622-create-ticket-tool}` entry
  for the new `ws/create_ticket` tool. Contract-first spec: no (contract fully specified
  in Decisions section; spec entry is post-implementation closeout).
- **Phase 2** — `ai-docs/spec/workflow-skills.md`: add the two new reviewer playbook
  names (`ticket-reviewer-design`, `ticket-reviewer-completeness`) and the
  `lead-write-ticket` sage-gate routing addition. Contract-first spec: no.
- **Phase 3** — `ai-docs/spec/mcp-tools.md`: add `sage_review*` config key entries
  under the config section. Contract-first spec: no.

## Phases

### Phase 1: `create-ticket` MCP tool

New `ws/tickets.create(session_key, stem, initial_state)` Go MCP handler.
Auto-prefixes today's date to form the full stem. Writes a frontmatter stub
(`title:` placeholder; `sage-review: pending` for `todo/+`). Returns `{path, tip}`;
tip is non-empty for both `idea/` and `todo/+`.

`initial_state`: accepted values are `"idea"`, `"todo"`, and `"ready"`;
terminal states (`"done"`, `"dropped"`) are rejected with an error.

Constraints:
- Follow the `tickets.close`/`tickets.move` registration pattern from 260620:
  wsdoc logic layer, MCP server dispatch, `rootAwareToolSchemaRequiresSessionKey`,
  both `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`, and
  CLI mirror in `cmd/ws-mcp/main.go` with `runtimeCapabilityCommandNames` entry.
- `sage-review: pending` written only when `initial_state` is `"todo"` or `"ready"`.
- `idea/` creation writes `title:` placeholder only; no `sage-review` field.

Deferred: `parent:` inference, workset membership, any frontmatter fields beyond
`title:` and `sage-review:`.

Verification:
- Unit tests: `idea/` creation (no sage-review field), `todo/` creation (sage-review:
  pending), `ready/` creation, rejected terminal states, date auto-prefix.
- `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` passes after both
  `runtime.json` updates.
- `{#260622-create-ticket-tool}` entry written in `ai-docs/spec/mcp-tools.md`.

### Result

#### Edition (8ced5351) - 2026-06-22

`tickets.create` MCP tool + CLI mirror landed in `8ced5351` (branch
`implement/260622-sage-review-ticket-gate`). All 3 review partitions clean.
Spec entry `{#260622-create-ticket-tool}` written to `ai-docs/spec/mcp-tools.md`.
`TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` passes; full
`go test ./...` green. Version bump deferred to epic-merge per convention.

### Phase 2: Reviewer playbooks + lead-write-ticket integration

- `ticket-reviewer-design` playbook: receives ticket path, reads ticket + linked
  docs, attempts implementation plan sketch, returns structured verdict covering
  design coherence, duct-tape, right-problem, and autonomous-vs-missing gaps.
- `ticket-reviewer-completeness` playbook: receives ticket path, returns structured
  verdict on ticket structure and clarity gaps.
- Both reviewer playbooks are **read-only**: no file writes, no shell execution,
  no MCP mutations. Return verdict text only.
- `lead-write-ticket` gains the judge gate (see Decisions) and invokes both
  reviewers in parallel after ticket commit when `sage_review` config is `auto |
  ask` and landing status is `todo/+`. Lead synthesizes verdicts and writes
  `## Blocked` section on block.

**Verdict schema** (each reviewer emits):

```
verdict: pass | concern | block

issues:
  - title: <short label>
    severity: critical | important | minor
    detail: <what is unclear or wrong>
    resolution: autonomous | missing
```

**Lead synthesis output** (written to `## Blocked` section on block):

```markdown
## Blocked (YYYY-MM-DD)

### Design Reviewer — <verdict>

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | ...   | important | missing   |

### Completeness Reviewer — <verdict>

| # | Title | Severity |
|---|-------|----------|
| 1 | ...   | important |
```

Aggregation rule: design `block` → final block regardless of completeness result.
Completeness `concern` → lead judgment on whether to set `blocked`.

### Phase 3: Config integration + 260620 coordination

Add all four `sage_review*` keys to the config schema (session > proj > user),
following the `ItemPreferMercenary` registration pattern in `scope.go`.

Coordinate with 260620 Phase 1 for two additions:
- `sage-review` pre-condition check on upward moves (gated on `sage_review`
  config; fails transition on `pending | blocked`).
- Downward demotion support + ready-demotion tip (clear `spec:`/`spec-remove:`
  reminder).

If 260620 Phase 1 ships before this phase, append an Edition to its Phase 1
result for both additions.

### Result

#### Edition (1b715fa1) - 2026-06-22

Delivered on branch `implement/260622-sage-review-ticket-gate`.

- `ticket-reviewer-design` (`kind: render`, `tier: large`, `delegates: true`): reads
  ticket + linked specs/mental-models via `specs.find` / `mental_models.find`, sketches
  implementation plan, emits structured `pass|concern|block` verdict with issues list.
- `ticket-reviewer-completeness` (`kind: render`, `tier: medium`, `delegates: true`):
  reads ticket only (no linked docs), emits structured verdict on structure/clarity gaps.
- `lead-write-ticket`: `delegates: true` removed from frontmatter (flag is for
  `kind: render` only; on `kind: print` it injects a Continuity tip that broke the
  golden test). Sage Review Gate added as step 8 after Commit; "On: Sage Review Gate"
  handler added; `## Templates` section with Blocked Section Template added before Doctrine.
- Both manifests regenerated (`agents-plugin` + `agents-plugin-wsflow`).
- Golden tests added: `TestRenderGoldenShippedPhase4Delegates` and
  `TestRenderReturnsFrontmatterRecommendedTier` extended to cover both new playbooks.
- `go test ./...`: 12/12 PASS.
- Spec: `{#260624-sage-review-gate}` added to `ai-docs/spec/mcp-tools.md`.
- Mental model: `workflow-skills.md` updated with Sage Review Gate behavior + Phase 3 config note.

#### Edition (e207815e) - 2026-06-24

All four `sage_review*` config keys registered in `wsconfig/scope.go` (Phase 3 complete).
All phases done; ticket closed.

- `internal/wsconfig/scope.go`: `ItemSageReview`, `ItemSageReviewDesignTier`,
  `ItemSageReviewCompleteness`, `ItemSageReviewCompletenessTier` constants + `ScopeProject`
  `init()` registrations. All four keys now visible in `config.show` output when unset
  (matching the `ItemPreferMercenary` / `scoped_show.go:63` precedent).
- `internal/mcp/server.go`: inline `"sage_review"` string literal replaced with
  `wsconfig.ItemSageReview` typed constant.
- 260620 coordination: both required additions (sage-review upward-move pre-condition
  check + downward demotion ready-demotion tip) were already delivered in 260620 Phase 1
  (`735acfe4`). Edition appended to 260620 Phase 1 Result confirming coordination complete.
- Spec `{#260624-sage-review-gate}`: `Planned 🚧` note stripped (feature fully implemented).
- Mental model: `workflow-skills.md` stale "not yet registered until Phase 3" caveat removed (`9f410884`).
- `go test ./...`: 12/12 PASS.

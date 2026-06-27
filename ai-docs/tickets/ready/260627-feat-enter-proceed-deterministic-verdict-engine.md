---
title: Deterministic enter.proceed route and verdict resolution
sage-review: completed
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: introduced typed enter tools and session agenda/todo persistence
  260627-research-lead-proceed-route-matrix-authoring: predecessor authoring-shape research that exposed the route-table complexity
related-mental-model:
  - workflow-skills
  - mcp-runtime
---

# Deterministic enter.proceed route and verdict resolution

## Background

Dogfooding `lead-proceed` showed that the route facts and route-selection tables
are precise but too heavy for the playbook body. The lead still has to read the
conversation, ticket, specs, and related workflow artifacts, but once the routing
facts and ambiguous judgments are known, much of the remaining decision is a
deterministic rule application.

`ws.enter.proceed` is already a new typed mode-switch concept and is not yet a
stable external compatibility surface. That makes it the right boundary for this
change: the playbook gathers normalized facts and calls one MCP tool at the
"routing facts complete" boundary, and MCP owns the deterministic route/verdict
resolution plus agenda/todo update.

## Decisions

- Keep `ws.enter.proceed` as the only public MCP mode-switch call at the
  routing-facts-complete boundary.
- Do not add a separate public `ws.proceed.route` or similar helper surface.
  Reusable route resolution may exist as a private Go helper behind
  `ws.enter.proceed`.
- Do not change the `playbook.print` or `playbook.render` contracts. This work is
  about session-state enter handling and `lead-proceed` text, not playbook
  materialization.
- Move every deterministic `lead-proceed` routing rule that can be derived from
  normalized facts into MCP logic. The playbook should retain fact gathering,
  ambiguous judgments, user-facing discussion, and invocation of the returned
  next direction.
- `lead-proceed` should no longer ask the LLM to choose among deterministic route
  rows after facts are complete. It should call `ws.enter.proceed`, read the final
  MCP verdict, and proceed according to the returned `NEXT`.
- The final MCP output must include raw verdict text that lets the LLM clearly
  know which direction to proceed next without re-solving the route matrix.
- The schema and rendered conditions must preserve the current `lead-proceed`
  route vocabulary wherever it is already load-bearing. Renaming or collapsing
  facts into broad buckets is not acceptable when the current playbook has a
  specific deterministic input or stop reason.
- Conflicting or inapplicable facts should not usually block workflow progress.
  Normalize deterministically, emit non-blocking warnings, and choose the
  conservative route.
- Hard-block only malformed JSON/type failures, authentication or session-key
  failures, and enum-level values outside the accepted schema.
- The same optimization pattern is intended for `lead-implement` later: after
  implement route facts and judgments are known, deterministic implementation
  verdict text should also move into MCP. That future work is out of scope for
  this ticket.

## Contract Sketch

`ws.enter.proceed` should accept the lead `session_key`, a `target` object, and a
grouped optional/nullable `facts` object:

```json
{
  "session_key": "<lead-session-key>",
  "target": {
    "kind": "ticket-path | inline | unknown",
    "label": "string",
    "ticket_stem": "string | null",
    "ticket_path": "string | null"
  },
  "facts": {
    "ticket": {
      "ticket_missing": "yes | no | unknown | null",
      "has_ticket": "yes | no | unknown | null",
      "status": "idea | todo | ready | done | dropped | unknown | n/a | null",
      "category": "epic | workset | other | n/a | unknown | null",
      "actionable": "yes | no | unknown | null",
      "freshness": "current | missing-settled-decisions | uncertain | n/a | unknown | null",
      "phase": "string | null"
    },
    "gates": {
      "discussion_needed": "yes | no | unknown",
      "needs_ticket": "yes | no | n/a | unknown",
      "scope_blocked": "none | container-ticket | multiple-explicit-phases | too-broad | no-unfinished-phase | phase-already-complete | unknown",
      "migration_anchor": "loaded | n/a | missing | conflict | unknown"
    },
    "work": {
      "category": "implementation | ticket_write | discussion | status_report | unknown",
      "slice": "Phase N[: title] | whole target | blocked | n/a | unknown"
    }
  },
  "format": "text | json"
}
```

Schema direction:

- `target` is required; individual nested fields may be nullable when inapplicable
  or unavailable.
- `facts` groups are optional so partial callers can still enter proceed mode.
  Missing fields normalize to `unknown` or verdict-facing `n/a` according to the
  deterministic precedence table.
- `null` means the axis is inapplicable or intentionally not observed.
- `unknown` means the axis applies but the caller could not determine it.
- Contradictions are resolved by precedence, not by LLM judgment. For example,
  `target.kind=inline` makes ticket status `n/a`, and `ticket-missing=yes` wins
  over any supplied ready status.
- JSON property names may use Go/JSON-friendly snake_case, but normalized
  conditions, raw verdict lines, warnings, agenda `conditions`, and tests must
  preserve the current hyphenated route vocabulary.

Current route vocabulary that must be preserved:

| Fact | Required values |
|------|-----------------|
| `target-kind` | `ticket-path`, `inline` |
| `ticket-missing` | `yes`, `no` |
| `has-ticket` | `yes`, `no` |
| `status` | `idea`, `todo`, `ready`, `done`, `dropped`, `unknown`, `n/a` |
| `migration-anchor` | `loaded`, `n/a`, `missing`, `conflict` |
| `actionable` | `yes`, `no` |
| `discussion-needed` | `yes`, `no` |
| `needs-ticket` | `yes`, `no`, `n/a` |
| `freshness` | `current`, `missing-settled-decisions`, `uncertain`, `n/a` |
| `category` | `epic`, `workset`, `other`, `n/a` |
| `slice` | `Phase N[: title]`, `whole target`, `blocked`, `n/a` |
| `scope-blocked` | `none`, `container-ticket`, `multiple-explicit-phases`, `too-broad`, `no-unfinished-phase`, `phase-already-complete` |

The resolver may add internal enum values only when they normalize to one of
these verdict-facing values or are explicitly documented as new route vocabulary
in the implementation closeout. It must not collapse specific current blockers
such as `multiple-explicit-phases`, `too-broad`, `no-unfinished-phase`, or
`phase-already-complete` into `other`.

The internal resolver should produce a stable result object:

```json
{
  "route": "ticket-readiness.ready-actionable",
  "next": "lead-implement",
  "target": {
    "label": "260627-feat-example",
    "ticket_stem": "260627-feat-example",
    "ticket_path": "ai-docs/tickets/ready/260627-feat-example.md"
  },
  "phase": "Phase 1: Example",
  "reason": "status=ready and actionable=yes",
  "conditions": [
    "target-kind=ticket-path",
    "ticket-missing=no",
    "has-ticket=yes",
    "status=ready",
    "actionable=yes",
    "discussion-needed=no",
    "scope-blocked=none"
  ],
  "warnings": [],
  "agenda": {
    "ticket": "ai-docs/tickets/ready/260627-feat-example.md",
    "phase": "Phase 1: Example",
    "next_skill": "lead-implement",
    "conditions": [
      "status=ready",
      "actionable=yes",
      "scope-blocked=none"
    ]
  },
  "todo_replaced": true,
  "raw": "Proceed Verdict\nRoute: ticket-readiness.ready-actionable\nNEXT: lead-implement\n..."
}
```

Default text output should render the canonical `raw` verdict. JSON output, when
requested, should include the same canonical `raw` field along with the structured
fields above.

## Raw Verdict Format

The raw verdict is line-oriented and intentionally terse:

```text
Proceed Verdict
Route: ticket-readiness.ready-actionable
NEXT: lead-implement

Target: 260627-feat-example
Phase: Phase 1: Example
Reason: status=ready and actionable=yes

Conditions:
- target-kind=ticket-path
- ticket-missing=no
- has-ticket=yes
- status=ready
- actionable=yes
- discussion-needed=no
- scope-blocked=none

Warnings:
- none

Agenda:
- ticket: ai-docs/tickets/ready/260627-feat-example.md
- phase: Phase 1: Example
- next_skill: lead-implement
```

Rules:

- The first three non-empty lines are always `Proceed Verdict`, `Route: ...`, and
  `NEXT: ...`.
- `NEXT` is a closed set owned by the resolver, including at least
  `lead-implement`, `lead-discuss`, `lead-write-ticket`, `status-report`, and
  `stop`.
- `Conditions` renders normalized facts actually used by the resolver, not the
  raw caller input.
- `Warnings` renders deterministic normalization notes such as ignored,
  contradictory, or inapplicable fields; use `- none` when empty.
- `Agenda` renders the exact values written to the `proceed` agenda blob.
- Do not include long explanatory prose in raw output. The lead may add
  user-facing explanation after reading the verdict, but MCP output should remain
  stable and easy to follow.

## Phases

### Phase 1: MCP-owned proceed route verdict

Implement deterministic route/verdict resolution inside `ws.enter.proceed`.

Required behavior:

- Before the larger resolver/schema work, hotfix the existing `lead-proceed`
  handoff for implementation routes: when `NEXT` is `lead-implement`, the
  playbook must explicitly direct the lead to call
  `ws/playbook.print(name: "lead-implement")` and execute the returned playbook
  inline, rather than treating `lead-implement` as a direct tool or calling
  `ws.enter.implement` itself. This prevents proceed routing from skipping the
  implement playbook while the current `enter.implement` schema still accepts
  final verdict labels.
- Replace the current freeform `ticket`, `phase`, `next_skill`, and `conditions`
  schema with the `target` + grouped optional/nullable `facts` shape, preserving
  `session_key`.
- Add a private pure Go resolver that normalizes facts, applies deterministic
  precedence, emits warnings, and returns a stable result object plus canonical
  raw verdict text.
- Extract all deterministic route rules that can be moved from the
  `lead-proceed` playbook into that resolver.
- Preserve current verdict-facing route vocabulary exactly for the existing
  facts and blockers: `ticket-missing`, `has-ticket`, `status`, `migration-anchor`,
  `actionable`, `discussion-needed`, `needs-ticket`, `freshness`, `category`,
  `slice`, and `scope-blocked`.
- Keep ambiguous judgments in the playbook. The LLM still owns reading artifacts,
  deciding uncertain facts, and asking the user when a fact cannot safely be
  resolved.
- Update `lead-proceed` so it builds normalized facts, calls `ws.enter.proceed`,
  follows the returned `NEXT`, and no longer carries the extracted deterministic
  route matrix in prose.
- Preserve the single-next-hop rule: execute only the returned `NEXT`; do not
  print or follow a full route chain.
- Preserve the existing enter-tool mode-switch semantics: the call stores the
  `proceed` agenda and replaces the todo list for proceed mode.
- Do not change playbook rendering, playbook printing, or wsflow product-mode
  rendering contracts.
- Add focused tests for normalization, precedence, warning output, raw verdict
  stability, JSON result shape, agenda storage, todo replacement, and the
  lead-proceed shipped playbook content that now delegates deterministic routing
  to MCP.

Completion boundary:

- A fresh `lead-proceed` run can recover the intended next action from the
  `ws.enter.proceed` output without re-evaluating a route table.
- Deterministic route rows no longer live in the `lead-proceed` playbook body
  when the same decision can be made from normalized MCP facts.
- The only public MCP call introduced or changed for this route boundary is
  `ws.enter.proceed`.

Deferred scope:

- Applying the same deterministic-verdict optimization to `lead-implement`.
- Adding a public `ws.proceed.route` helper.
- Changing `playbook.print`, `playbook.render`, or playbook frontmatter
  semantics.
- Redesigning ticket conventions, spec-address gates, or sage-review routing.

Verification boundary:

- Focused Go tests for `internal/mcp` cover valid, partial, contradictory, and
  malformed inputs.
- Golden or content tests cover the updated `lead-proceed` playbook and confirm
  the deterministic route matrix was removed from skill prose where MCP now owns
  it.
- Runtime manifest and wsflow rsrc mirror are regenerated if the changed surfaces
  require them.
- `git diff --check` passes.

### Result (75e1adee)

Phase 1 is review-clean on branch
`implement/260627-enter-proceed-verdict-engine`.

Implemented:

- `ws.enter.proceed` now accepts a `target` object, grouped optional/nullable
  `facts`, and `format`, then resolves deterministic proceed routing through a
  private MCP resolver.
- The resolver preserves the existing verdict-facing route vocabulary, emits
  canonical raw verdict text plus JSON output, records the proceed agenda, and
  replaces proceed todos atomically.
- `lead-proceed` now gathers route facts, calls `ws.enter.proceed`, follows the
  returned `NEXT`, and explicitly dispatches implementation routes through
  `ws/playbook.print(name: "lead-implement")`.
- Runtime docs, workflow docs, mental models, rsrc manifests, and the wsflow
  rsrc mirror were updated for the new boundary.

Verification:

- `go test ./internal/mcp -count=1`
- `go test ./internal/wsrsrc -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check`

Review:

- Partitioned correctness, fit, and test review completed clean after one
  fix/re-review cycle.

### Phase 2: MCP-authored next action directive

Add a concrete next-action directive to the `ws.enter.proceed` result and use it
to shrink `lead-proceed` execution text.

Required behavior:

- Keep `format: "json"` as a first-class output path because agents tend to
  prefer structured tool results.
- Add a top-level `next_instruction` string to the JSON result. It is prose, not
  a nested action enum/object, but it must be concrete enough to execute without
  reinterpreting the route.
- Render the same instruction in raw output as a `Next:` line immediately after
  the `NEXT:` line block.
- For `lead-implement`, the instruction must explicitly name
  `{{namespace}}/playbook.print(name: "lead-implement")`, then say to execute the
  returned playbook inline for the current target and phase before inspecting
  source, planning, editing, or calling implementation tools.
- For `lead-write-ticket`, the instruction must explicitly name
  `{{namespace}}/playbook.print(name: "lead-write-ticket")`, then say to execute
  the returned playbook inline and rerun `ws.enter.proceed` only after a ready
  `Ticket:` path is returned.
- For `lead-discuss`, the instruction must explicitly route through the current
  skill namespace's `lead-discuss` skill with the blocker in `Reason`.
- For `status-report` and `stop`, the instruction must explicitly say to stop,
  report the blocker and safe next request from the verdict, and wait for the
  user.
- Remove the separate `Report Routing Verdict` template from `lead-proceed`; MCP
  output is the canonical verdict.
- Replace the `Execute Verdict` route-specific if-spam with a smaller rule:
  read `NEXT:` and `Next:` from MCP output, briefly state
  `Routing to next action: <NEXT>.`, then follow `Next:` exactly while preserving
  the stop and post-write reroute safety rails.
- Preserve existing mode-switch semantics: `ws.enter.proceed` still records the
  proceed agenda and replaces proceed todos.

Completion boundary:

- A fresh `lead-proceed` run can follow the `next_instruction`/`Next:` directive
  without restating a separate routing verdict or reconstructing route-specific
  execution prose.
- JSON and raw outputs carry equivalent next-action instructions.
- The lead-proceed playbook keeps only minimal execution safety rails.

Deferred scope:

- Modeling next action as a nested structured object.
- Executing downstream playbooks from inside MCP.
- Applying the same directive pattern to `lead-implement`.

Verification boundary:

- Focused Go tests cover `next_instruction` in JSON and `Next:` in raw output for
  implementation, ticket-writing, discussion, status/stop, and reroute cases.
- Playbook content tests verify the separate `Report Routing Verdict` template
  is gone and execution guidance follows `Next:`.
- Manifest and wsflow mirror are regenerated if shared rsrc changes.
- `git diff --check` passes.

### Result (f41f7d52)

Phase 2 is implemented on branch
`implement/260627-enter-proceed-verdict-engine`.

Implemented:

- `ws.enter.proceed` JSON output now includes a top-level `next_instruction`
  string.
- Raw verdict output now renders the same directive as a `Next:` line after
  `NEXT:`.
- The directive names concrete next calls such as
  `ws/playbook.print(name: "lead-implement")` or
  `ws/playbook.print(name: "lead-write-ticket")` where applicable.
- `lead-proceed` no longer emits a separate Routing Verdict block; it reads MCP's
  `NEXT:` and `Next:` lines, states `Routing to next action: <NEXT>.`, and
  follows the MCP directive with only minimal safety rails.
- Proceed session todos now track Build route context, Resolve MCP verdict, and
  Follow next instruction.

Verification:

- `go test ./internal/mcp -count=1`
- `go test ./internal/wsrsrc -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check`

### Phase 3: MCP-authored common follow rails

Move the remaining common `Follow Next Instruction` rails into MCP-authored
`Next:` / `next_instruction` text so `lead-proceed` no longer needs a separate
follow-next execution step.

Required behavior:

- Expand resolver-authored `next_instruction` text so it includes the shared
  follow rails that are still deterministic after route facts are known.
- Keep the guidance concrete and execution-oriented. The instruction should name
  the exact `{{namespace}}/...` MCP call or skill path when the next action has a
  fixed invocation shape.
- Remove the `follow-next` proceed todo. The proceed todo list should contain
  only `Build route context` and `Resolve MCP verdict`; following MCP output is
  the natural continuation after reading the verdict, not a separate tracked
  proceed task.
- Shrink `lead-proceed` to fact gathering, calling `ws.enter.proceed`, reading
  the returned MCP output, and following that output. It should not retain
  common if/then rails that MCP can safely generate into `Next:`.
- Preserve hard safety behavior for malformed tool output, explicit stop
  verdicts, and user interruption. If any safety rail cannot be expressed
  clearly in `Next:`, keep only that minimal guard in the playbook.
- Keep JSON and raw output equivalent: `next_instruction` and the raw `Next:`
  line must carry the same execution guidance.

Completion boundary:

- A fresh `lead-proceed` run has only two proceed todos after enter:
  `Build route context` and `Resolve MCP verdict`.
- The lead-proceed playbook contains no separate `Follow Next Instruction`
  section or route-specific common follow rails that duplicate MCP output.
- The MCP-authored `Next:` / `next_instruction` text is sufficient for the lead
  to continue without reconstructing execution guidance.

Deferred scope:

- Executing downstream playbooks from inside MCP.
- Modeling next actions as nested structured action objects.
- Applying the same common-rail extraction to `lead-implement`; that direction
  is recorded in
  `260627-feat-enter-implement-deterministic-verdict-engine`.

Verification boundary:

- Focused Go tests cover the updated proceed todo replacement.
- Playbook content tests verify `Follow Next Instruction` and duplicated common
  follow rails are absent from `lead-proceed`.
- Raw and JSON output tests verify the expanded `Next:` / `next_instruction`
  text remains equivalent.
- Manifest and wsflow mirror are regenerated if shared rsrc changes.
- `git diff --check` passes.

## Spec Impact

- **Target spec areas:** `workflow-skills.md` (`lead-proceed` route boundary,
  single-next-hop verdict, and future `lead-implement` optimization direction)
  and `mcp-tools.md` (`ws.enter.proceed` schema, normalization behavior, agenda
  storage, todo replacement, JSON result, and canonical raw verdict text).
- **Expected caller-visible change:** `lead-proceed` becomes lighter after facts
  are gathered, while `ws.enter.proceed` becomes the deterministic route/verdict
  resolver. Callers see a canonical raw verdict that clearly names the next
  direction, a concrete next-action instruction, and warnings for ignored or
  normalized facts.
- **Contract-first spec: no.** The ticket itself pins the intended contract for
  this implementation slice, and `ws.enter.proceed` is a new, not-yet-shipped
  concept. The implementation should update `workflow-skills.md` and
  `mcp-tools.md` during closeout to record the final shipped behavior.

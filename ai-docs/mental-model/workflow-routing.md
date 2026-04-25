---
domain: workflow-routing
description: "Contracts and coupling for /proceed's pipeline routing and prefix-stage delegation."
sources:
  - claude/skills/proceed/
  - claude/skills/write-ticket/
  - claude/skills/write-spec/
  - claude/skills/sprint/
  - claude/skills/discuss/
related:
  spec-system: "/write-spec's judge: spec-impact gate is owned by write-spec, not pre-evaluated by /proceed. /write-ticket's judge: spec-gate fires on any action resulting in todo/-or-higher status (direct creation and idea/→todo/ promotion) and may redirect to /write-spec. /discuss's promotion handler runs /write-spec before git mv so a 🚧 entry exists before the ticket reaches todo/ status."
---

# Workflow Routing

`/proceed` is the universal router for the canonical chain. It fires prefix
judges (`needs-spec`, `needs-ticket`) before the implementation pipeline judges
(`direct-edit`, `needs-plan`, `needs-skeleton`, `execution-mode`).

## Entry Points

- `claude/skills/proceed/SKILL.md` — full routing logic, judgment tables, invariants, doctrine.
- `claude/skills/write-ticket/SKILL.md` — ticket authoring protocol, including the completion artifact at step 8.
- `claude/skills/write-spec/SKILL.md` — `judge: spec-impact` gate definition.
- `claude/skills/sprint/SKILL.md` — session-container entry point for feature-branch work; independent of `/proceed`.

## Module Contracts

- `/proceed` guarantees: `judge: needs-spec` fires on **every** invocation. It is unconditional.
  `/proceed` does not pre-evaluate whether a spec is needed. It always delegates to `/write-spec`,
  which handles the no-op exit via its own `judge: spec-impact` gate.
- `/proceed` guarantees: when invoking prefix stages (`/write-spec`, `/write-ticket`) via the Skill
  tool, it passes gate-suppression context as args. This suppresses interactive confirmation gates
  inside the prefix stage so the chain does not pause for user input. The announce template surfaces
  a "Gate suppression" bullet on every routing path so the user knows suppression is active.
- `/proceed` guarantees: `judge: needs-ticket` invokes `/write-ticket` for any actionable
  inline description, regardless of scope clarity. Only an existing ticket path skips this
  step. The distinction between "vague" and "clear-scope" inline descriptions is not
  evaluated at the routing level. Both delegate to `/write-ticket`, which handles scope
  classification internally via `judge: initial-status`. An exploratory target, where the
  user is weighing approaches rather than requesting implementation, stops before
  `/write-ticket` and suggests `/discuss`. After the auto-invoke, the ticket path captured
  from `/write-ticket`'s `Ticket:` output becomes the target for all downstream stages.
  Proceed does not generate or assume a ticket path. It reads the captured artifact.
- `/write-ticket` guarantees: step 8 emits a `Ticket:` completion artifact on its own line,
  in the form `Ticket: ai-docs/tickets/<status>/<stem>.md`. This line is the handoff protocol
  for any caller that needs to chain downstream on the produced ticket.
- `/write-ticket` guarantees: `judge: spec-gate` fires on any action that results in
  `todo/`-or-higher status — direct `todo/` creation and `idea/` → `todo/` promotion moves.
  `idea/` creation is ungated. If no relevant spec file exists or no entry covers the behavior,
  write-ticket stops and redirects to `/write-spec`. The `Ticket:` artifact at step 8 is only
  emitted when spec-gate passes. Callers that chain on the `Ticket:` line must account for the
  possibility that spec-gate stops write-ticket before the artifact is produced.
- `/discuss` guarantees: when handling an `idea/` → `todo/` promotion, the handler runs
  `/write-spec` to add a `🚧` entry before executing `git mv`. This ordering ensures a spec
  entry exists before the ticket reaches `todo/` status, satisfying write-ticket's spec-gate
  when the ticket is subsequently opened or referenced.
- `/sprint` guarantees: it operates only on `sprint/`-prefixed branches. On invoke it detects
  the current branch: if on a `sprint/` branch it presents continue/wrap-up/abandon options; if
  not, it creates a new `sprint/<name>` branch. `/sprint` is not routed through `/proceed`. It is
  a standalone session container and does not invoke the `/proceed` prefix pipeline.
- `/discuss` guarantees: when invoked on a `sprint/`-prefixed branch, step 1 emits a hint
  directing the user to `/sprint` for session continuity. This hint is informational only — it
  does not prevent `/discuss` from continuing.

## Coupling

- `/proceed` → `/write-spec`: unidirectional invocation, always fires. Gate logic lives
  entirely inside `/write-spec`. Proceed never checks `judge: spec-impact` itself. Proceed
  passes gate-suppression context via args, so a `/write-spec` invoked from the chain behaves
  differently from a standalone invocation — interactive confirmation gates are suppressed.
- `/proceed` ↔ `/write-ticket`: proceed invokes write-ticket and reads back the `Ticket:` line
  to capture the path. Write-ticket's step 8 format is a contract with proceed. Changing the
  line prefix or path format breaks proceed's path-capture logic.
- `/write-ticket` → `/write-spec`: unidirectional redirect (not invocation). `judge: spec-gate`
  may stop write-ticket and suggest `/write-spec`. This gate fires on any todo/-or-higher
  status action and is independent of `/proceed`'s unconditional delegation to `/write-spec`.
- `/discuss` → `/write-spec` (promotion only): when promoting `idea/` → `todo/`, discuss
  invokes `/write-spec` first, then `git mv`. This is ordered coupling — spec entry must
  precede the directory move.
- `/discuss` → `/sprint` (hint only): when `/discuss` detects a `sprint/`-prefixed branch at
  invoke, it emits a one-line note pointing to `/sprint`. There is no invocation or delegation;
  the hint does not alter discuss behavior.
- `/sprint` is independent of `/proceed`: sprint manages its own routing table (`judge: delegate`)
  and runs a spec-update loop + `ws:mental-model-updater` + executor-wrapup at wrap-up.
  Changing `/proceed`'s prefix-stage pipeline does not affect `/sprint`.

## Extension Points & Change Recipes

- **Add a new prefix stage to `/proceed`**: implement the gate logic inside the new sub-skill,
  not inside proceed. Proceed should always fire the sub-skill unconditionally and let the
  sub-skill decide whether to act. Pass gate-suppression context in the Skill tool args so the
  new stage does not pause the chain with interactive confirmation gates. Mirroring `needs-spec`'s
  pattern — including the suppression context — is correct.
- **Change `/write-ticket`'s completion artifact format**: update the `Ticket:` line prefix or
  path shape in write-ticket step 8, then update proceed's capture logic in step 4 (`Execute`)
  to match. Both files must change together.
- **Add a caller that chains on `/write-ticket`**: read the `Ticket:` line from write-ticket's
  output at step 8. Do not assume the path from frontmatter or filename patterns. The artifact
  line is the authoritative output.

## Common Mistakes

- Routing sprint sessions through `/proceed` — `/sprint` is a standalone entry point. It manages
  its own routing loop, branch state, and wrap-up sequence. Invoking `/proceed` inside a sprint
  session bypasses sprint's doc-deferral invariant.
- Adding conditional logic in `/proceed` to decide whether to invoke `/write-spec`. The
  contract is unconditional invocation. Spec-impact gating belongs in write-spec.
- Invoking `/write-ticket` from a skill without reading the `Ticket:` completion line. The
  produced ticket path is not inferrable from arguments alone (status directory is chosen by
  write-ticket's `judge: initial-status`).
- Assuming a new prefix stage should mirror the implementation pipeline judges (returning
  yes/no to proceed). Prefix stages delegate fully. Proceed does not inspect their return
  value beyond the ticket-path artifact.
- Treating a clear-scope inline description as equivalent to a ticket path and skipping
  `/write-ticket`. Any actionable target that is not a file path must go through
  `/write-ticket`. Exploratory targets stop before `/write-ticket` is invoked.
- Assuming `/write-ticket` always emits the `Ticket:` completion artifact. `judge: spec-gate`
  fires on any todo/-or-higher status action (direct creation and idea/→todo/ promotion) and
  may stop write-ticket before step 8 is reached. When no spec coverage exists, no artifact
  is emitted and the caller must handle the missing line.
- Invoking a prefix stage without passing gate-suppression context. The stage will present
  interactive confirmation gates that pause the chain and break the no-pause invariant.
  Gate-suppression context must be included in the Skill tool args for every prefix-stage
  invocation from `/proceed`.

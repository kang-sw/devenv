---
domain: workflow-routing
description: "Contracts and coupling for /proceed's pipeline routing and prefix-stage delegation."
sources:
  - claude/skills/proceed/
  - claude/skills/write-ticket/
  - claude/skills/write-spec/
related:
  spec-system: "/write-spec's judge: spec-impact gate is owned by write-spec, not pre-evaluated by /proceed."
---

# Workflow Routing

`/proceed` is the universal router for the canonical chain. It fires prefix
judges (`needs-spec`, `needs-ticket`) before the implementation pipeline judges
(`direct-edit`, `needs-plan`, `needs-skeleton`, `execution-mode`).

## Entry Points

- `claude/skills/proceed/SKILL.md` — full routing logic, judgment tables, invariants, doctrine.
- `claude/skills/write-ticket/SKILL.md` — ticket authoring protocol, including the completion artifact at step 8.
- `claude/skills/write-spec/SKILL.md` — `judge: spec-impact` gate definition.

## Module Contracts

- `/proceed` guarantees: `judge: needs-spec` fires on **every** invocation. It is unconditional.
  `/proceed` does not pre-evaluate whether a spec is needed. It always delegates to `/write-spec`,
  which handles the no-op exit via its own `judge: spec-impact` gate.
- `/proceed` guarantees: after a `judge: needs-ticket` auto-invoke, the ticket path captured
  from `/write-ticket`'s output becomes the target for all downstream stages (skeleton, plan,
  implementation). Proceed does not generate or assume a ticket path. It reads the captured
  artifact.
- `/write-ticket` guarantees: step 8 emits a `Ticket:` completion artifact on its own line,
  in the form `Ticket: ai-docs/tickets/<status>/<stem>.md`. This line is the handoff protocol
  for any caller that needs to chain downstream on the produced ticket.

## Coupling

- `/proceed` → `/write-spec`: unidirectional invocation, always fires. Gate logic lives
  entirely inside `/write-spec`. Proceed never checks `judge: spec-impact` itself.
- `/proceed` ↔ `/write-ticket`: proceed invokes write-ticket and reads back the `Ticket:` line
  to capture the path. Write-ticket's step 8 format is a contract with proceed. Changing the
  line prefix or path format breaks proceed's path-capture logic.

## Extension Points & Change Recipes

- **Add a new prefix stage to `/proceed`**: implement the gate logic inside the new sub-skill,
  not inside proceed. Proceed should always fire the sub-skill unconditionally and let the
  sub-skill decide whether to act. Mirroring `needs-spec`'s pattern is correct.
- **Change `/write-ticket`'s completion artifact format**: update the `Ticket:` line prefix or
  path shape in write-ticket step 8, then update proceed's capture logic in step 4 (`Execute`)
  to match. Both files must change together.
- **Add a caller that chains on `/write-ticket`**: read the `Ticket:` line from write-ticket's
  output at step 8. Do not assume the path from frontmatter or filename patterns. The artifact
  line is the authoritative output.

## Common Mistakes

- Adding conditional logic in `/proceed` to decide whether to invoke `/write-spec`. The
  contract is unconditional invocation. Spec-impact gating belongs in write-spec.
- Invoking `/write-ticket` from a skill without reading the `Ticket:` completion line. The
  produced ticket path is not inferrable from arguments alone (status directory is chosen by
  write-ticket's `judge: initial-status`).
- Assuming a new prefix stage should mirror the implementation pipeline judges (returning
  yes/no to proceed). Prefix stages delegate fully. Proceed does not inspect their return
  value beyond the ticket-path artifact.

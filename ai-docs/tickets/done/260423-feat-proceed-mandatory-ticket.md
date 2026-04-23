---
title: /proceed — mandatory ticket for inline descriptions
spec:
  - 260423-proceed-needs-ticket-tighten
---

# /proceed — mandatory ticket for inline descriptions

## Background

When `/proceed` receives an inline description (not a ticket path), `judge: needs-ticket` currently has three branches: vague idea → auto-invoke `/write-ticket`; exploratory → stop; clear-scope inline → Proceed. The third branch skips ticket creation entirely, so discussion details negotiated upstream live only in conversation context and are not captured in a durable artifact. Implementers in downstream delegated sessions receive no structured context beyond the brief — decisions, constraints, and rationale are lost.

Because `/proceed` runs in the main agent (not a delegated subagent), invoking `/write-ticket` at this stage gives it full conversation context. No intermediate summary extraction step is needed — `/write-ticket` pulls discussion details directly from the active session.

## Phases

### Phase 1: Tighten judge: needs-ticket in /proceed

**Goal:** collapse the routing table so any inline description always invokes `/write-ticket`, regardless of scope clarity.

**Changes:**

`claude/skills/proceed/SKILL.md` — update `judge: needs-ticket` table:
- Remove the "clear-scope inline description → Proceed" branch.
- Merge "vague idea" and "clear-scope inline description" into one branch: "inline description (any scope) → invoke `/write-ticket`, capture `Ticket:` output, continue."
- "Existing ticket path → Proceed" branch remains unchanged (ticket already exists, no authoring needed).
- "Exploratory → stop, suggest `/discuss`" remains unchanged.

`ai-docs/mental-model/workflow-routing.md` — update the `needs-ticket` contract description to reflect the new table: inline description always delegates to `/write-ticket`; only an existing ticket path is a pass-through.

**Rationale:** Ticket is the carry medium for decisions. The distinction between "vague" and "clear-scope" inline descriptions is not meaningful at the routing level — both lack a durable artifact. A clear-scope inline description passed from a `/discuss` session looks actionable but carries no captured rationale for downstream implementers.

**Rejected alternative:** Add a `judge: already-ticketed` gate inside `/write-ticket` to exit immediately when given an existing ticket path (mirroring `needs-spec`'s unconditional-delegation pattern). Rejected because `/write-ticket` already has an edit flow for existing tickets — a pass-through gate would require distinguishing caller intent (pass-through vs. edit) from arguments alone, which is not possible.

**Integration test criteria:**
- `/proceed` with a clear-scope inline description (e.g., "add dark mode toggle to settings panel") invokes `/write-ticket` and emits a `Ticket:` path before any implementation stage begins.
- `/proceed` with an existing ticket path does not invoke `/write-ticket`.
- `/proceed` with an exploratory target stops and suggests `/discuss`.

### Result (d1d7e7e) - 2026-04-23

Implemented as specified. `judge: needs-ticket` table collapsed to three rows: exploratory → stop, existing ticket path → proceed, inline description (any scope) → invoke `/write-ticket`. Invariants bullet split into two single-sentence lines. `workflow-routing.md` Module Contracts and Common Mistakes updated to reflect the new guarantee including explicit exploratory-stop coverage. No deviations from plan. `260423-proceed-needs-ticket-tighten` 🚧 marker stripped from `workflow-skills.md`.

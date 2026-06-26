---
title: "Surface sage-review posture without a config/frontmatter dive"
sage-review: skipped
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260622-feat-sage-review-ticket-gate: current-gate
  260625-feat-ws-session-state-machine: motivating-dogfood
---

# Surface sage-review posture without a config/frontmatter dive

## Problem

During the 260625 dogfood, agents repeatedly had too little local signal for
whether sage review applies to the current ticket. The current tool behavior
pushes them toward reading `config.show` or manually inspecting frontmatter even
when the next action should be knowable from the ticket itself.

Today, `sage_review` is a project config setting (`off`, `ask`, `auto`, or
empty), while `sage-review:` is a per-ticket frontmatter field. `tickets.create`
currently stamps `sage-review: pending` for `todo/` and `ready/` tickets, and
upward ticket moves consult both config and frontmatter. The word `pending` does
not explain whether review is disabled, optional, or mandatory.

## Decisions

- Keep `sage_review` project config as the source for the default project
  posture.
- `tickets.create(initial_state: "todo"|"ready")` and upward
  `tickets.move(...)` into `todo/` or `ready/` should stamp a resolved
  `sage-review:` posture instead of always writing or preserving `pending`.
- `sage_review` `off`, empty, or unset resolves to `sage-review: skipped`.
- `sage_review: ask` resolves to `sage-review: recommended`.
- `sage_review: auto` resolves to `sage-review: required`.
- `recommended` means a run-or-skip decision is still needed. It does not mean
  sage review is mandatory.
- `required` means the sage review gate should run automatically before the
  ticket can proceed past the gate.
- The gate resolves unresolved posture to one of `completed`, `blocked`, or
  `skipped`.
- When a user declines review in `ask` mode, the gate writes
  `sage-review: skipped`.
- Agents should be able to inspect the ticket frontmatter alone to understand
  whether sage review is skipped, recommended, required, completed, or blocked.
  They should not need to dig through config only to decide whether sage review
  applies.

## Compatibility notes

Existing tickets may still contain `sage-review: pending`. Treat `pending` as a
legacy unresolved value during the migration, but do not stamp it for new
`todo/` or `ready/` tickets after this change.

If an upward move sees no `sage-review:` field, it should stamp the resolved
posture from config. If it sees an unresolved `recommended` or `required` value
when promotion requires resolution, it should stop with a message that names the
needed action: run sage review, skip recommended review, or address a blocked
review.

## Phases

### Phase 1: Resolved sage-review posture stamping

Completed behavior:

- `tickets.create` stamps `skipped`, `recommended`, or `required` for
  `todo/` and `ready/` tickets according to the resolved `sage_review` config.
- `tickets.move` upward stamps or validates the same resolved posture.
- The sage review gate treats `recommended` as a decision-required state and
  `required` as review-required state.
- Gate completion writes `completed`, `blocked`, or `skipped`, leaving no
  unresolved `recommended` or `required` state after the gate resolves.
- User-facing move/create/gate messages describe the resolved posture without
  asking the caller to inspect config.

Deferred scope:

- Adding a new `config.tuning` knob. This ticket is about making ticket
  frontmatter self-describing; it does not require changing the tuning catalog.
- Retrofitting all historical tickets unless compatibility tests require a
  targeted fixture update.

Verification boundary:

- Unit tests cover `tickets.create` for off/empty, ask, and auto posture
  resolution.
- Unit tests cover upward `tickets.move` stamping/validation for skipped,
  recommended, required, completed, and blocked values.
- The existing sage review gate tests are updated to use resolved posture
  language instead of assuming `pending` is the default state.

## Spec Impact

Target spec area: `ai-docs/spec/mcp-tools.md` ticket mutation tools and sage
review gate.

Expected caller-visible change: ticket mutation tools stamp self-describing
`sage-review:` posture values, and the gate resolves `recommended` / `required`
instead of relying on `pending`.

Contract-first spec: yes, before promotion to `ready/`.

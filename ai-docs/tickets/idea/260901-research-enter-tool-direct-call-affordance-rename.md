---
title: enter.* tools are called directly without their lead-* skill — affordance correction via mechanical rename and opaque published params
related:
  260901-bug-enter-proceed-misplaced-facts-silent-unknown-status: the recurring failure this addresses structurally; that ticket owns the immediate diagnosability hardening (reason enrichment + misplaced-key warnings), this owns the tool-surface/naming strategy
  260726-feat-enter-verdict-scenario-output: adjacent enter.* verdict-output reshape; any name/schema change here must stay consistent with that output work
  260630-bug-enter-implement-explicit-direct-edit-schema-undocumented: precedent that an enter.* schema-vs-actual-input gap is a legitimate bug
  260627-feat-enter-proceed-deterministic-verdict-engine: substrate — the deterministic pure-fact-router design whose auditability the opaque-params option must preserve
---

# enter.* tools are called directly without their lead-* skill — affordance correction via mechanical rename and opaque published params

## Background

`enter.proceed` (and its sibling `enter.implement`) is architecturally an
**inner step of a lead-* playbook**: the playbook's "On: invoke" reads
artifacts and derives route facts, then calls the resolver, which is a pure,
stateless fact-router (`proceed_resolver.go` does zero filesystem I/O; every
routing condition is read only from caller-supplied grouped facts and defaults
to `unknown` when absent). But the tool is exposed as a first-class MCP tool
named after the **user intent** (`enter.proceed` reads as "do the proceed"), so
agents repeatedly call it **directly, skipping the skill that was supposed to
build the facts**. This has recurred across multiple independent Codex sessions
— evidence of a systematic affordance mismatch, not one-off user error.

The concrete recurring failure (owned by
`260901-bug-enter-proceed-misplaced-facts-silent-unknown-status`): a direct
caller hand-builds a flat `facts` payload, the nested-only fields
(`facts.ticket.status`, …) match nothing and default to `unknown`, and a
ticket-path target short-circuits to `terminal-artifact.unknown-status` with a
bare `Reason: status=unknown` and `NEXT: stop` — no in-band recovery.

Two structural roots, distinct from the bug ticket's runtime hardening:

1. **Name = intent.** The tool's name matches what the model wants to do, so it
   is the button the model reaches for. Temptation lives in the leaf action verb.
2. **A visible, richly-documented input schema is a visible invitation.** A
   self-documenting schema is also a self-inviting one: the same nested-field
   descriptions that help a playbook-following caller let a context-free caller
   autocomplete a (wrong-shaped) direct call.

Conditional/dynamic tool exposure (hiding `enter.*` until the playbook is
active) was investigated and is **not available** in the Codex/MCP runtime, so
the only remaining levers are the tool's **name** and its **published shape**.

This is a family-level question: `enter.implement` shares the same
"Groups and fields are optional; unknown/null values are normalized by the
resolver" copy and the same inner-step role, and `enter.implement` additionally
reads git state itself — so the family is not even internally consistent about
what the tool derives vs. what the caller supplies, which trains the wrong
mental model. Any change here should apply consistently across `enter.*`.

## Direction (user-confirmed 2026-09-01)

Three orthogonal levers; the settled direction picks L1 + L3 and explicitly
rejects L2:

- **L1 — Name: intent-verb → mechanical-op.** Rename each tool **in place**
  after what it mechanically does (resolve a routing verdict from supplied
  facts), so a model wanting to "proceed" does not reach for it. Base naming
  (user-preferred): `enter.proceed → route.resolve_proceed`,
  `enter.implement → route.resolve_implement`. The neutral `route.*` namespace
  keeps the intent noun out of the leaf entirely; alternative
  `proceed.resolve_verdict`/`implement.resolve_verdict` keeps per-tool identity.
  Final leaf/namespace naming is the one open bikeshed to settle at
  implementation.

- **L2 — Cardinality: KEEP TWO TOOLS (collapse rejected).** Do **not** collapse
  into a single `ws/enter(mode, params)` / `route.resolve_verdict(mode, …)`.
  Rationale: a `mode` discriminator forces a union/oneOf published schema (or an
  under-documented blob) that is awkward to express and, once visible, re-invites
  the very discoverability the change is trying to remove; and it does not itself
  reduce temptation (a model still finds the one tool and sets `mode:"proceed"`).
  The load-bearing effect people attribute to the collapse (opaque params) is
  L3, which is achieved without collapsing. Keeping two tools preserves each
  tool's distinct typed facts schema (proceed facts ≠ implement facts) with no
  union expression.

- **L3 — Published schema: opaque `params`, internal typed validation kept.**
  Minimize the client-visible input schema to an opaque `params: object` plus a
  pointer ("Inner step of `ws:lead-proceed`; not a direct entry point; `params`
  are constructed by that skill"), while the Go decoder keeps the full typed
  struct parse + validation and returns the precise warnings/errors. Net: the
  auditability/testability the deterministic verdict engine (260627) was built
  for stays internal; only the harmful client-visible autocomplete is removed.
  The real input contract moves into the **skill body** (`lead-proceed` /
  `lead-implement`), consistent with the repo's "contracts in playbooks, tools
  thin" doctrine. With two tools (L2), each simply publishes its own
  `params: object` — no union pain.

The runtime redirect guard (a ticket-path target with `status=unknown` should
point the caller at `ws:lead-proceed` rather than dead-ending) is owned by the
companion bug ticket as the runtime surface of L3's internal validation.

## Open Questions

- Final naming: `route.resolve_proceed`/`route.resolve_implement` (preferred) vs
  `proceed.resolve_verdict`/`implement.resolve_verdict`.
- Deprecation path: old tool names appear in both `agents-plugin` and
  `agents-plugin-wsflow` lead-proceed/lead-implement playbooks, `runtime.json`,
  spec docs (`mcp-tools.md`, `workflow-skills.md`), and the resolver
  next-instruction text (`proceed_resolver.go:355` "rerun enter.proceed") — an
  alias/transition window vs a hard rename needs deciding.
- How much of the input contract genuinely moves to the skill body vs. stays as
  a minimal published hint, and whether `enter.implement`'s self-read git state
  changes the opaque-params story for that tool.
- Whether the family should extend beyond proceed/implement (any other enter.*
  survivors after the enter.sprint/enter.salvage retirement).

## Spec Impact

Not applicable at `idea/`. Any adopted change will touch the MCP tools spec
(`mcp-tools.md`) tool-name/schema contract and the workflow-skills spec where
lead-proceed/lead-implement reference the tool; scope to be addressed if this is
promoted to an actionable ticket.

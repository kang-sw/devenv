---
title: "Epic: MCP tool-surface affordance reduction — shrink count and attractiveness of the ws tool surface"
sage-review-design: completed
related:
  260901-research-enter-tool-direct-call-affordance-rename: child ① — the settled affordance/rename work (enter.* → mechanical-op + opaque params); this epic owns its scheduling and the shared execution model
  260630-epic-skill-playbook-diet: coordinate, not owned here — that epic moves contracts INTO tools (Lever B MCP-ification) and can grow the very tools this epic reshapes; sequence so a tool is not expanded immediately before it is renamed/collapsed
  260901-bug-enter-proceed-misplaced-facts-silent-unknown-status: explicitly OUT of scope — diagnosability hardening, not surface reduction
  260901-bug-ticket-scanner-silently-skips-noncanonical-status-dir: explicitly OUT of scope — scanner robustness, not surface reduction
sage-review-design-reviewed: 59ffc7858a162560
---

# Epic: MCP tool-surface affordance reduction

## Summary

The ws MCP surface exposes ~62 tools (plus gated `mercenary.*`/`exec.*`
families), organized as a flat CRUD-style palette. Two coupled problems:

1. **Attractiveness (affordance).** Intent-named, richly-documented tools
   (`enter.proceed`, `enter.implement`, and other workflow-entry-shaped tools)
   invite agents to call them **directly, skipping the lead-* skill** that was
   supposed to build their inputs. This has recurred across independent
   downstream Codex sessions — a systematic affordance mismatch, not one-off
   error. The tool surface is competing with the skills as the model's mental
   API.
2. **Weight.** The raw count carries mechanical over-splitting (e.g. `todo.*` is
   a 9-tool ordered-list CRUD) and query redundancy smell
   (`tickets.list`/`find`/`status`, three-way `specs.*`) that add grazing noise.

This epic owns shrinking both the **count** and the **attractiveness** of the
surface. It does not own moving contracts into tools (that is
`260630-epic-skill-playbook-diet`) nor router diagnosability (the two bug
tickets above).

## Scope — three layers, ordered by pain/readiness (not by effort)

Ordering key is **most-settled + most-painful first → speculative cleanup
later**, NOT lowest-effort-first (see cross-child invariant on effort below).

- **① Semantic / affordance** — the workflow-entry-shaped tools that read as
  user intent. Settled direction (L1 mechanical rename + L3 opaque published
  `params`, L2 collapse rejected) lives in child
  `260901-research-enter-tool-direct-call-affordance-rename`. Highest value:
  the only layer actively blocking downstream; and the only layer whose design
  is already frozen.
- **② Mechanical inelegance** — over-split tool families (`todo.*` 9 leaves,
  candidate `note.*`) consolidated to the operations callers actually compose.
  Contained; low value (noise reduction); needs a consolidation-shape decision.
- **③ Query redundancy** — `tickets.list`/`find`/`status`, three-way `specs.*`.
  Needs an **audit first** to confirm genuine redundancy vs distinct semantics
  (`find` vs `status` may differ) before any dedup. Lowest confidence.

## Organizing model — decision vs execution (not layer-serial)

The dominant cost of all three layers is the same: **ws-side playbook
re-authoring** (a skill-authoring ceremony per touched skill, not a
find-replace). Doing the three layers as three serial passes would re-author the
same playbooks (`lead-proceed`/`lead-implement`) up to 3× with 3× the ceremony,
deprecation windows, and drift risk. So the epic is organized on the
**decision → execution** axis instead of layer-serial:

- **Decision phase (freeze the end-state surface).** Freeze the full target
  names/shapes across the cheap-to-decide layers (① is already frozen; ②'s
  obvious consolidations) and run the ③ **audit** to decide its end-state. This
  is the genuinely hard part — the difficulty is deciding, not editing.
- **Execution phase (batch by playbook).** Re-author each affected ws skill
  **once** against the frozen surface. A skill touched by multiple layers gets a
  single coordinated rewrite; a skill touched only by ③ waits for the audit.
  **Owner of a shared-skill rewrite:** the child that already owns the widest
  reshape of that skill holds the pen; the others contribute only their frozen
  deltas into that one pass, never a second authoring pass. Concretely, the
  overlap-heavy `lead-proceed`/`lead-implement` are exactly ①'s targets, so ①
  owns their coordinated rewrite and ②/③ fold any of their deltas to those two
  skills into ①'s pass. Confirm/adjust ownership when ②/③ are created.

## Cross-child invariants

- **Authoring is ws-only, ×1.** `agents-plugin-wsflow` assets are mechanically
  mirrored from ws by script, so only ws-side playbooks are authored; the
  wsflow multiplier is ~1. Any child that hand-edits wsflow separately is a
  smell — fix the mirror instead.
- **Effort delta between layers is small; value delta is not.** Because the
  common cost floor is playbook re-authoring, the layers do not differ much in
  effort. Prioritize by value/readiness (① first), and do NOT under-scope ① as
  "the easy one" — it has the widest blast radius (ws+wsflow playbooks,
  `runtime.json`, `mcp-tools.md`/`workflow-skills.md`, resolver next-instruction
  text at `proceed_resolver.go:355`, Go tool registration).
- **Non-playbook reference surfaces are part of every freeze.** `runtime.json`,
  the MCP/workflow specs, resolver next-instruction text, and Go tool
  registration are mechanical (not ceremony) but must be captured in the frozen
  end-state so a rename/consolidation lands atomically.
- **Coordinate with skill-playbook-diet.** That epic's Lever B pushes more
  decision logic INTO `enter.*`; this epic makes those tools less visible. A
  tool must not be expanded by diet immediately before this epic renames or
  reshapes it — sequence the two.
- **Deprecation posture is per-child.** Whether a rename ships as a hard cut or
  an alias/transition window is decided per child (downstream callers exist);
  the epic requires each child to state its posture, not a single epic-wide rule.

## Child tickets

- **①** `260901-research-enter-tool-direct-call-affordance-rename` — exists at
  `idea/`; promote under this epic. Owns the affordance rename + opaque-params
  execution.
- **②** mechanical tool-family consolidation (`todo.*`, candidate `note.*`) —
  to be created.
- **③** query-tool redundancy audit → dedup (`tickets.*` read tools, `specs.*`)
  — to be created; audit gates any removal.

## Closure conditions

- The workflow-entry-shaped tools no longer read as user intent and no longer
  publish a self-inviting input schema (① landed).
- Over-split families are consolidated to composed operations, or explicitly
  decided to keep (② resolved either way).
- Query redundancy is audited and either deduped or documented as intentionally
  distinct (③ resolved either way).
- The mirror keeps wsflow in sync with no hand-authored wsflow drift introduced.

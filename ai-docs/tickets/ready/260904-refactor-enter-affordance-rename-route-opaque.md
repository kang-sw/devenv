---
title: "enter.* affordance rename — route.resolve_* + full-opaque published params"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
related:
  260901-research-enter-tool-direct-call-affordance-rename: input — the settled direction (L1 route.* rename, L3 full-opaque params, L2 collapse rejected) and its rationale; this ticket implements that research finding
  260901-bug-enter-proceed-misplaced-facts-silent-unknown-status: companion — owns the runtime redirect guard (status=unknown ticket-path target points at ws:lead-proceed), which is the runtime surface of L3's internal validation; coordinate
  260903-refactor-mcp-verb-vocabulary-unification: sibling layer ④ — ④'s scripted token substitution over lead-proceed/lead-implement runs before this ticket's authoring pass so those two skills are authored once
sage-review-design: completed
sage-review-design-reviewed: 587db921af0b881a
sage-review-completeness: completed
sage-review-completeness-reviewed: 2ce5ab8ad8931cc2
---

# enter.* affordance rename — route.resolve_* + full-opaque published params

## Background

Layer ① of `260903-epic-mcp-tool-surface-affordance-reduction`, and the
actionable implementation of the settled research finding in
`260901-research-enter-tool-direct-call-affordance-rename` (which remains the
reference for problem framing and rationale; this ticket does not restate it).

`enter.proceed`/`enter.implement` are inner steps of the `lead-proceed`/
`lead-implement` playbooks, but they are exposed as first-class MCP tools named
after user intent with a richly self-documenting input schema, so agents call
them directly and skip the skill that was supposed to build their facts —
recurring across independent downstream Codex sessions. The two remaining levers
(the runtime cannot hide the tools conditionally) are the tool **name** and its
**published shape**.

## Decisions

Both user-confirmed (2026-09-04); see the research ticket for full rationale.

- **Naming (A): `enter.proceed → route.resolve_proceed`,
  `enter.implement → route.resolve_implement`.** The `route.*` namespace keeps
  the intent noun out of the leaf entirely. The per-tool alternative
  `proceed.resolve_verdict`/`implement.resolve_verdict` was rejected for keeping
  the intent noun (`proceed`/`implement`) as the namespace, re-inviting the
  direct call.
- **Published schema (C): full opaque.** The client-visible input schema is an
  opaque `params: object` plus a skill pointer only — no residual documented hint
  fields. The Go decoder keeps the full typed struct parse + validation and
  returns precise warnings/errors; the real input contract lives solely in the
  `lead-proceed`/`lead-implement` skill body. Rationale beyond debloat: a model
  follows an input contract best when it reads it closest to the call, and skill
  prose is read immediately before the call whereas the MCP schema surface sits
  at a large context distance — so the contract belongs at minimum context
  distance (the skill body), where compliance is highest.
- **Two tools kept (L2 collapse rejected).** No `mode`-discriminated single tool;
  each of the two tools publishes its own opaque `params`, avoiding a union
  schema.

## Constraints

- **One-shot hard cut** (epic invariant): no alias/transition window. Every
  old-name consumer ships in-package and is rewritten atomically.
- **Internal auditability preserved.** The deterministic verdict engine's typed
  parse/validation (`260627`) stays internal; only the harmful client-visible
  autocomplete is removed. No behavior change to routing.
- **Single authoring pass on the shared skills.** This ticket holds the pen on
  `lead-proceed`/`lead-implement` (epic pen-holder rule); sibling ④'s scripted
  read-surface token substitution runs first, and this ticket's authoring pass
  acts on the canonical names rather than re-running ④'s rename by hand.
- **Redirect guard is not owned here.** The runtime `status=unknown` redirect to
  `ws:lead-proceed` belongs to the companion bug ticket; coordinate so the opaque
  schema and the guard message stay consistent.

## Phases

### Phase 1: Rename enter.proceed/enter.implement → route.resolve_*

One-shot hard-cut rename in place across every in-package surface: Go tool
registration + dispatch switch, `runtime.json`, MCP/workflow specs
(`mcp-tools.md`, `workflow-skills.md`), the resolver next-instruction text
(`proceed_resolver.go` "rerun enter.proceed"), the `lead-proceed`/`lead-implement`
playbook token references, and the wsflow mirror (via the mirror script, not
hand-edited). Verify by diff + tests; no alias left behind. Routing behavior
unchanged.

### Phase 2: Hollow published schema to opaque params + move contract to skills

Reduce each tool's client-visible input schema to `params: object` plus the
skill pointer, while keeping the Go decoder's full typed struct parse +
validation internal. Relocate the real input contract (the grouped facts each
resolver consumes) into the `lead-proceed`/`lead-implement` skill bodies — the
single authoring pass this ticket pen-holds, absorbing sibling ④'s frozen
read-surface token deltas so the two skills are authored once. Confirm
`enter.implement`'s self-read git state: its skill-body contract covers fewer
caller-supplied fields than proceed, but the published surface is the same opaque
`params`. Coordinate the opaque-schema pointer text with the companion bug
ticket's redirect guard so a mis-shaped direct call is pointed at the skill.

Acceptance (not "schema now shows `params: object`" alone): the Go decoder still
parses + validates the typed struct with the existing resolver/verdict tests
green (routing output byte-unchanged); the relocated skill-body contract covers
every field the resolver reads, cross-checked field-by-field against the typed
struct so none is dropped; and a mis-shaped direct call surfaces the opaque
pointer / redirect-guard message rather than a silent `status=unknown`.

## Spec Impact

Edits to existing anchors only — no new spec stem, and no heading `{#slug}`
changes (so no `renamed-spec`):

- `mcp-tools.md` `{#260625-session-state-tools}` — the "Enter (typed mode
  switches)" paragraph and the per-mode `implement`/`proceed` subsections
  (currently describing `enter.implement`/`enter.proceed` and their accepted
  fields): rename the two tools to `route.resolve_implement`/
  `route.resolve_proceed` and rewrite the published input-schema prose to the
  opaque `params` + skill-pointer contract (typed validation stays internal).
- `workflow-skills.md` `{#260505-proceed-routing-pipeline}` (proceed) and
  `{#260505-implementation-workflow-skills}` (implement) — where
  `lead-proceed`/`lead-implement` name the tool and now carry the real input
  contract in the skill body.

The `references.trace` sweep at implementation confirms no other spec anchor
names the old tools.

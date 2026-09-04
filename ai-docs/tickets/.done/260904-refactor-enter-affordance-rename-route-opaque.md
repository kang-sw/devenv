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
completed: 2026-09-04
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

### Result (7e35db10) - 2026-09-04

One-shot hard-cut rename landed: `enter.implement → route.resolve_implement` and
`enter.proceed → route.resolve_proceed` as literal-string swaps across every
in-package surface — Go dispatch switch + tool registration `"name"` fields,
behavior-visible error-prefix and branch-action/next-instruction strings
(`session_state.go`, `proceed_resolver.go`, `implement_resolver.go`), the four
sibling `_test.go` files, both `runtime.json` `"tools"` keys, the
`mcp-tools.md` / `workflow-skills.md` spec cross-references (token-only, no
`{#slug}` change), the two canonical `rsrc/lead-proceed` + `lead-implement`
playbooks, and `test_skill_dispatch_contracts.py`. The wsflow rsrc mirror and
both `manifest.json` files were regenerated via the mirror generator
(`WSRSRC_REGEN` + `WS_REGEN_WSFLOW_RSRC`, both `-count=1`), never hand-edited.
Routing logic, the published `inputSchema`, and internal Go handler identifiers
(`handleEnterImplement` etc.) were left untouched — Phase 2 owns the schema
hollowing.

Verification: `go test ./... -count=1` green across all 14 packages (including
the `TestShippedManifestUpToDate` / `TestWsflowRsrcMirrorUpToDate` drift
guards), re-confirmed on the goal branch post-merge; the
`enter\.(proceed|implement)|enter_proceed|enter_implement` grep sweep returns
zero hits across code, `runtime.json`, specs, and playbooks.

Review (partitioned, correctness opus / test sonnet): correctness clean; test
clean with 1 Minor recorded and no action — the renamed branch-action/error
literal strings in `session_state.go` and `implement_resolver.go:926` have no
exact-rendered-text assertion, a coverage gap that pre-dates the rename (the old
tokens were equally untested) with no typo found on manual inspection.

Deviations / deferred: mental-model prose still names the old tools
(`mcp-runtime.md`, `workflow-skills.md`) — deferred out of this surgical rename
per the plan's Out of Scope (same precedent as sibling ④); this accumulated
epic-rename doc drift (④'s verb unification + ①'s `route.resolve_*`) is captured
in a follow-up idea ticket rather than silently deferred. A pre-existing,
unrelated Python failure (`test_proceed_keeps_implementation_route_only`, stale
`"Route only…"` assertion) is likewise captured separately, not touched here.

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

### Result (a46d03cf) - 2026-09-04

Hollowed the client-visible `inputSchema` of `route.resolve_proceed` and
`route.resolve_implement` (`server.go`) to `session_key` + an opaque
`params: object` + a `ws:lead-proceed` / `ws:lead-implement` skill pointer (in
both the tool `description` and the `params` description), `required` reduced to
`["session_key"]`. The real field contract (`target` / `facts.*` / `policy.*` /
`format`) was relocated into new `## Fact Contract` tables in
`agents-plugin/rsrc/lead-proceed/lead-proceed.md` and `lead-implement.md`,
mirrored to wsflow with both manifests regenerated. Spec prose
(`mcp-tools.md` `{#260625-session-state-tools}`, `workflow-skills.md`
`{#260505-proceed-routing-pipeline}` / `{#260505-implementation-workflow-skills}`)
reframed to describe the opaque published schema + skill-pointer without deleting
semantic content or changing any `{#slug}`.

Key resolved design point: "opaque `params: object`" is **advertised-schema
only**. There is no JSON-schema validator in the package and callers send
top-level `target`/`facts`/`policy`, so the Go decoder is untouched and routing
is byte-unchanged — `git diff` on `proceed_resolver.go` / `implement_resolver.go`
is empty, and every existing `TestResolveProceed*` / `TestEnterProceed*` /
`TestEnterImplement*` / `TestDeriveImplementTodo*` fixture stays green unchanged.
The two old nested-schema tests were replaced with
`TestRouteResolveProceedSchemaIsOpaque` / `TestRouteResolveImplementSchemaIsOpaque`
(assert `target`/`facts`/`policy` absent, `params` a bare object, `required`, and
the pointer substring). Field-by-field cross-check of the Fact Contract tables
against the resolver structs is clean (no invented or dropped field/enum).

Verification: `go test ./... -count=1` green across all 14 packages (incl.
`TestShippedManifestUpToDate` / `TestWsflowRsrcMirrorUpToDate`).

Review (partitioned, correctness opus / fit sonnet / test sonnet): correctness
clean, test clean; fit clean with 1 Minor recorded and no action —
`containsAnyString` (`session_state_test.go:1645`) became dead when the old
schema test was deleted but was not swept alongside
`objectProperties`/`assertNullableSchema`; harmless unused test helper, left for
a next-touch cleanup.

Deferred / cross-refs: the `status=unknown` mis-shaped-facts **redirect guard**
itself remains owned by `260901-bug-enter-proceed-misplaced-facts-silent-unknown-status`
— this phase only established pointer-text/skill-name consistency for it to
reference. The skill-authoring **Layer 1 exception** (the relocated `Fact
Contract` sections are authorized restatement, since the schema is no longer
ToolSearch-discoverable once opaque) is recorded here and in the implementation
commit's `## AI Context` so a future skill audit does not strip them as drift.
Mental-model prose for these tools stays deferred to
`260904-refactor-mental-model-doc-drift-epic-renames`.

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

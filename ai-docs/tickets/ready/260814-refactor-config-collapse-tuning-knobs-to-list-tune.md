---
title: "Collapse per-knob config MCP tools into config.list + config.tune"
spec:
  - 260625-tuning-catalog
  - 260620-config-prompt-override-tuning-tools
  - 260619-layered-config-scope-model
related-mental-model:
  - mcp-runtime
related:
  260611-research-ws-per-role-delegation-tuning-config: adjacent — proposes widening the config surface toward per-role tuning; this ticket narrows the tool surface and the two must agree on the single per-key registry as the extension point
  260626-bug-sage-review-config-setter-missing: coordination — its standing plan to add a sage_review writer "surfaced through config.tuning" is invalidated by this ticket (config.tuning is removed); pulled back to idea/ with a TBD note pending this ticket's landing
sage-review-design: completed
sage-review-completeness: completed
---

# Collapse per-knob config MCP tools into `config.list` + `config.tune`

## Background

The `config.*` surface exposes each tuning knob as its own MCP tool — roughly ten:
`config.show`, `config.tuning`, `config.agents_tier`, `config.bootstrap_alarm`,
`config.doc_coverage_alarm`, `config.prompt`, `config.prompt.set`,
`config.prompt.unset`, `config.workflow_prefer_subagent`, and
`config.workflow_prefer_mercenary`. Every knob is a separate function with its own
schema, and all of them ship in `tools/list`, competing for model attention even
though the underlying operation is almost always "read the config catalog" or "set
one key to a value." This is the same "MCP tool surface wider than the operation
it serves" shape as the same-day manuals.list/find retirement.

Collapse the surface to **two** tools (~10 → 2):

- `config.list` — one read surface returning the full tunable catalog for the
  current runtime mode: keys, current resolved values, scope metadata, and the
  per-key value schema (what each knob supports). It is the authoritative,
  fully self-describing "how to call `config.tune`" surface — see Decision 8.
  Subsumes `config.show` and `config.tuning`.
- `config.tune("<key>", <value>, scope, harness)` — one write surface: set a
  single key. Designed for explicit, precise, fully-specified input read from
  `config.list` rather than relying on defaulting (Decision 8). Subsumes
  `config.agents_tier`, `config.bootstrap_alarm`, `config.doc_coverage_alarm`,
  `config.prompt.set`/`config.prompt.unset`, and both `config.workflow_prefer_*`
  writers.

## Decisions

Settled in discussion (2026-08-22 – 08-23):

1. **A single per-key registry is the linchpin.** Today validation is inline in
   each dispatch case and `config.tuning`'s catalog scrapes field/enum metadata
   directly out of the per-knob `tools/list` schema entries — so deleting those
   schemas breaks the catalog. The reshape introduces one per-key registry that is
   the single source of truth for: value schema, `allowed_scopes`, `default_scope`,
   harness applicability, no-agent (wsflow) visibility, and authority requirement.
   Both `config.list` (render the catalog) and `config.tune` (validate + dispatch
   the write) consume it, and the four currently-scattered gating tables collapse
   onto it.
2. **`config.tune` carries an explicit per-key `scope`, validated against the
   registry.** Each write states the scope; the registry rejects a scope outside
   the key's `allowed_scopes` (e.g. a global-only key written at session/project).
   `config.list` surfaces the exact scope a caller should pass per key, so the
   caller specifies it rather than leaning on a silent default (Decision 8). A
   `default_scope` may exist in the registry as a backstop, but the intended path
   is explicit specification, not defaulting. Concretely: in `config.tune`'s own
   schema the `scope` field is **optional** with `default_scope` as the backstop
   (matching today's scopeless-write behavior, spec 260619) — the "specify it
   explicitly" expectation is a caller contract carried by `config.list` (Decision
   8), not a hard schema requirement. `session_key` remains authority-only — never
   a scope selector.
3. **`config.list` returns as much as is applicable.** A tuning session touches
   little else, so context budget is not the constraint. list returns the full
   per-key schema (value domain / legal-value hint, required-vs-optional fields,
   allowed and default scopes, harness applicability) so `config.tune`'s generic
   value argument does not lose the model-facing validation the separate tools gave
   today. This is the data that makes Decision 8's "read the precise contract, then
   specify it fully" workflow possible. `config.list` must also carry forward two
   documented read features of the tools it subsumes: `config.tuning`'s
   `format:"json"` stable structured shape (spec 260625 — `lead-tune` parses it to
   build proposals) and `config.show`'s per-value "resolved-from" scope reporting
   (spec 260619). Both are preserved output contracts, not just informal fields.
4. **`config.list` preserves the no-agent filter.** "Return everything" means
   everything applicable to the current runtime mode. The wsflow/no-agent catalog
   must still omit full-ws-only knobs (today `workflow.prefer_mercenary`); this is
   a behavioral contract, not a budget choice — carried by the registry's
   no-agent-visibility flag.
5. **`harness` is load-bearing for the keys that use it; a warning only where it
   genuinely does not apply.** Two key families are harness-scoped today, not one:
   prompt-override keys (harness selects the override lookup key) **and**
   `agents.tier` (harness selects which per-harness model-alias slot is written,
   via `aliasTargetKey`). A registry that treated harness-applicability as
   "prompt-override only" would silently downgrade `agents.tier`'s meaningful
   `harness` argument to a no-op — a regression. The registry's harness dimension
   is therefore per-key and covers both families. Only keys that truly ignore
   harness (alarms, `workflow.prefer_*`) return the "specified but not applicable"
   warning; for the harness-scoped families `harness` is real input surfaced by
   `config.list`.
6. **Authority gate survives the collapse.** `config.*` stays lead-only via the
   prefix gate; global-only keys additionally require lead-key authority. Driven
   per-key by the registry's authority flag.
7. **Per-knob tools are removed outright — no deprecated aliases.** This is an
   internal integration layer with no relationship to downstream AGENTS.md; a
   downstream reinstall reloads the plugin cleanly. The landing is intentionally
   all-or-nothing: **when this ticket lands, the entire config tool surface
   changes at once**, including every consumer swept in the same landing.
8. **Conservative caller contract: `config.list` describes, `config.tune` demands
   precision.** The design center is "read the full per-key contract from
   `config.list`, then pass exactly what it specifies," not "call `config.tune`
   and rely on optional-argument defaulting." Rationale (user directive): optional
   arguments are unreliable for agent callers, so silent defaulting is minimized
   and any remaining optionality exists only where `config.list` explicitly marks
   a field optional for that key. This principle governs Decisions 2 (scope), 3
   (list returns the full schema), and 5 (harness). Per-field required-vs-optional
   is determined by applying this principle against each key's real shape and is
   surfaced through `config.list` — not invented here.
9. **`agents.tier` is a compound writer, represented faithfully — not flattened.**
   Unlike the scalar knobs (alarm booleans, `prefer_*` on/off/hide, prompt text),
   `agents.tier`'s value is multi-field: `tier` (required selector:
   `small|medium|large|xlarge`), `harness` (optional selector for the per-harness
   alias slot), and the payload `backend` / `model` / `effort`
   (`""|none|low|medium|high|xhigh`). The registry entry represents this compound
   shape as-is: the `tier` selector travels **inside** the value object, and
   `harness` stays the outer selector argument, so
   `config.tune("agents.tier", {tier, backend, model, effort}, scope, harness)`
   stays behavior-equivalent to today's tool (`tier` and `harness` are its two
   top-level selector params today). This ticket does **not**
   redesign the tier/effort enum taxonomy (the `small/…/xlarge` vs
   `none/low/…/xhigh` axes) — that is `260611`'s scope; here the existing values
   are preserved verbatim.

## Constraints

- **`config.agents_tier` bypasses the resolver today** — it calls `Load`/`save`
  directly and never routes through `Resolver.Get`/`Set`/`Unset` (documented in the
  `mcp-runtime` mental model). Folding it into a resolver-agnostic `config.tune`
  needs care. Recommended: fold it into the resolver as part of Phase 1's single-
  source convergence; fallback if a harness-map value shape blocks that cleanly:
  keep a per-key adapter behind the registry. Decide against the actual code in
  Phase 1, not from memory.
- **Multi-surface sweep, land conservatively.** The collapse touches nearly every
  surface that names a config tool. Verify each exhaustively before landing:
  - Go handlers, `tools/list` schema literals, and the four gating tables that each
    hardcode subsets of the ten names, all in the MCP server source.
  - `runtime.json` in both the `ws` and `wsflow` packages (`tools` + `commands`
    blocks) and the launcher's `runtimeCapabilityCommandNames` / no-agent command
    filter.
  - The CLI mirror (`ws-mcp config <show|agents-tier>` today; only two of the ten
    are CLI-mirrored).
  - The wsflow runtime-contract test that cross-checks `runtime.json` against live
    `runtime capabilities` output.
- **Cross-ticket: `260626` premise is invalidated by this landing.**
  `260626-bug-sage-review-config-setter-missing` (now pulled back to `idea/`) had a
  standing plan to add a `sage_review` writer "surfaced through `config.tuning`" —
  the exact read tool this ticket removes. That ticket carries a TBD note pointing
  here; its re-design (seat a `sage_review` knob as a registry entry, target
  `config.list`/`config.tune`) happens after this ticket lands, not within it. This
  ticket does not block on `260626`; the dependency runs the other way.
- **Shipped prose sweep is part of the landing.** The `ws:lead-tune` playbook is
  built around calling `config.tuning` as its knob-catalog source and mapping
  requests to `prompt.<pointId>` knobs; it and the one `config.prompt.set` mention
  in the workflow-manual playbook exist in both the `ws` and `wsflow` rsrc trees.
  Removing the tools without rewriting this prose leaves dead instructions naming
  tools that no longer exist. Mental-model and spec docs naming these tools or the
  `{#260625-tuning-catalog}` / `{#260619-layered-config-scope-model}` anchors are
  rewritten alongside the code.

## Phases

### Phase 1: Extract the per-key registry (no external surface change)

Introduce the single per-key config registry and make every existing consumer read
from it — the ten inline dispatch validators, `config.tuning`'s catalog builder
(replacing its `tools/list`-schema scraping), and the four gating tables
(lead-only, session-key-required, no-agent-hidden, workflow-preference-writer). The
MCP tool surface, `runtime.json`, and CLI stay byte-for-byte unchanged; this is a
pure internal refactor that de-risks the catalog/gating coupling before anything is
removed. Resolve the `config.agents_tier` resolver-bypass decision here against the
code (fold into resolver, else registry-backed adapter). Verification: full Go test
suite green with no tool-surface diff (`runtime.json` and `tools/list` unchanged);
the wsflow runtime-contract test unchanged and passing.

### Result (d722e864) - 2026-08-23

Caller-visible delta: none — Phase 1 landed as the pure internal refactor. Added
`agents-plugin-tool/internal/mcp/config_registry.go` as the single per-key config
registry (5 static entries + a `prompt.*` dynamic template) and rewired every
consumer to read from it: the dispatch value/selector validators,
`buildTuningCatalog` (now sourcing field/enum metadata from the registry instead
of scraping `tools()` schemas), and the three gating tables. Deleted the dead
scrape helpers (`tuningFieldFromSchema`, `toolInputSchemaDetails`,
`propertyString`, `propertyStringSlice`).

Decisions resolved against the code:
- **agents_tier resolver-bypass → adapter fallback** (the ticket's documented
  fallback, Constraints). Evidence: `wsconfig.SetAgentsTierForHarness` writes a
  structured `AgentTier{Backend,Model,Effort}` into `cfg.Agents.ModelAliases` — a
  different `Config` shape than the resolver's flat `cfg.Overrides[key]=string` —
  and the tool has no scope arg, so a resolver fold would be a capability
  extension, out of bounds for a behavior-preserving phase. `agents.tier` keeps
  its direct write (`ResolverBacked:false`); only its enum constants moved into
  the registry.
- **Authority derives from `wsconfig.GlobalOnly`**, not a duplicated hardcoded
  list; gating table 4 (`workflowPreferenceWriterTool`) collapses into a derived
  view (Decision 6).
- **`tools()` schema literals left structurally untouched** (shared enum vars),
  so `tools/list` is byte-for-byte identical (Decision 3 / no-diff bar).

Deviation from plan: added a fail-closed guard (`requireConfigKeyEntry`, tip
commit d722e864) so a registry miss in the dispatch validators errors instead of
silently skipping validation via `enumContains(nil, …)` returning true. Closes a
latent trap Phase 2's `config.tune` (arbitrary runtime keys) would otherwise hit;
behavior-preserving for Phase 1 (all current names resolve).

Verification: `go build`/`go vet` clean; full Go suite green (with
`WS_SKILLS_ROOT` set); both `runtime.json` and the ten `config.*` tools/list
schemas byte-for-byte unchanged (independently diffed at the commit boundaries via
the built `ws-mcp` binary); wsflow runtime-contract test passes unmodified.

Review: partitioned correctness/fit/test — clean. Two minors: one fixed
(fail-closed guard above); one accepted — the registry's `GlobalOnly()` /
`DefaultScope()` methods are inert scaffolding with no Phase 1 call sites, staged
for Phase 2's scope-routing consumer.

Deferred to Phase 2 (per this ticket's Spec Impact / Constraints): the
`mcp-runtime` mental-model and `mcp-tools.md` spec rewrites are assigned to the
Phase 2 external-surface landing, so they are intentionally untouched here rather
than churned across the phase boundary. A true resolver fold for `agents.tier`
remains out of this ticket's scope.

> Forward: Phase 2 builds `config.list`/`config.tune` on the registry's currently
> inert `GlobalOnly()`/`DefaultScope()` methods and must wire scope routing
> through them; the `agents.tier` adapter (`ResolverBacked:false`) must stay a
> compound writer, not be flattened.

Pre-existing, unrelated (captured separately as an idea ticket): `go test ./...`
bare fails without `WS_SKILLS_ROOT` because `TestMain` only defaults
`WS_RSRC_ROOT` — confirmed identical on the base commit.

### Phase 2: Swap the surface — add `config.list` + `config.tune`, remove the ten

Add the two new tools consuming the Phase 1 registry, remove all ten per-knob tools
outright, and sweep every consumer in one landing: `runtime.json` (both packages),
`runtimeCapabilityCommandNames` + no-agent command filter, the CLI mirror (`config
list` / `config tune`), the Go tests (per-handler test files rewritten against the
two tools), the wsflow runtime-contract test, the `ws:lead-tune` and
workflow-manual playbook prose (both rsrc trees), and the mental-model / spec docs
(`mcp-tools` anchors, `workflow-skills` lead-tune reference). `config.list` must
preserve the no-agent full-ws-only filter and expose the full per-key call
contract (value schema, required/optional fields, scope, harness applicability);
`config.tune` must enforce per-key value schema, explicit scope routing
(scope validated against `allowed_scopes`, wrong-scope rejected), the compound
`agents.tier` shape, harness as load-bearing for the harness-scoped families and a
warning only where it does not apply, and the authority gate. Verification: full Go
+ Python contract test suites green; `tools/list` shows exactly the two tools with
the per-knob names gone; wsflow catalog still omits full-ws-only knobs; an
`agents.tier` write with an explicit `harness` still targets the correct
per-harness alias slot; `ws:lead-tune` dogfood round-trips a read and a write
through the new surface.

This phase is deliberately one atomic landing, not a splittable sequence
(Decision 7): the moment the ten tools are removed, every consumer that still
names them breaks, so tool removal and the full consumer sweep must land together
or tests fail. Within the phase, a fresh implementer gets natural checkpoints by
ordering the work — (a) add `config.list` + `config.tune` alongside the existing
tools and get them green; (b) repoint the CLI mirror, `runtime.json`, and
capability-command lists; (c) rewrite the Go + Python tests against the two tools;
(d) rewrite the shipped playbook prose and the mental-model / spec docs; (e) remove
the ten tools and their gating-table entries last — but all five land as a single
reviewable commit-set.

## Spec Impact

The spec-addressed behavior lives entirely in `ai-docs/spec/mcp-tools.md` (the
three anchors below), which is why the frontmatter `spec:` list carries only those
three stems. `ai-docs/spec/workflow-skills.md`'s `ws:lead-tune` reference is
downstream prose alignment — it describes the skill consuming `config.tuning` and
is rewritten to point at `config.list`/`config.tune` in the same landing, but it
declares no config behavior of its own, so it is doc-closeout, not a spec-address
stem.

- `{#260625-tuning-catalog}` — the read contract folds from `config.tuning` into
  `config.list`; catalog field metadata now sources from the per-key registry
  rather than being scraped from per-knob `tools/list` schemas. The no-agent
  full-ws-only omission is preserved and re-expressed against the registry flag.
- `{#260620-config-prompt-override-tuning-tools}` — prompt override set/unset moves
  under `config.tune`; `harness` is documented as load-bearing here (and for
  `agents.tier`), with the "specified but not applicable" warning scoped to keys
  that genuinely ignore harness.
- `{#260619-layered-config-scope-model}` — the write contract gains explicit
  per-key `scope` routing validated against `allowed_scopes` (with `default_scope`
  as a backstop, not the intended path per Decision 8), with `session_key`
  reaffirmed as authority-only, not a scope selector.
- The per-knob writer/reader tool definitions in `mcp-tools.md` are removed and
  replaced with the `config.list` + `config.tune` contract.

Expected caller-visible change: the ten `config.*` tools are gone; callers read
through `config.list` and write through `config.tune("<key>", <value>[, scope][,
harness])`.

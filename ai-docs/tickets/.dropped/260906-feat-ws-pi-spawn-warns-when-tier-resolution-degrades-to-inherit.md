---
title: Pi spawn checks a configured tier model against Pi's catalog and warns with suggestions instead of silently inheriting
related:
  260905-feat-ws-pi-harness-config-layer: owns resolveModelForAliasViaWsMcp and its genuine-pi-hit guard
  260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model: supersedes the "exactly provider/id" value contract with backend expansion; the deferred Phase 1 spec pass writes that accept rule, not the bare-id-ambiguity sentence
  260906-bug-ws-pi-model-warning-keeps-reappearing: supersedes the "list row repeats the spawn warning" surface with a tier_rejected row field and a once-per-key advisory; the deferred Phase 1 spec pass writes those forms
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f00236defb705392
sage-review-completeness-reviewed: f00236defb705392
dropped: 2026-09-06
---

# Pi spawn checks a configured tier model against Pi's catalog and warns with suggestions instead of silently inheriting

## Background

Owner-run verification of the harness config layer, 2026-09-06: the
verification agent tuned `agents.tier` for harness `pi` with
`small: gpt-5.6-luna` (no provider prefix), read back
`config.resolve_agent(small)` as `model: gpt-5.6-luna, effort: low,
resolved_from: pi`, then spawned a small worker and found it running the
lead's own model. It reported the scenario as FAIL.

The adapter behaved as designed. `resolveModelForAliasViaWsMcp`
(`spawner.ts`) accepts a resolution only when `resolved_from === "pi"` AND
the model string contains a `/` (a Pi `provider/id`); anything else degrades
to `{model: inheritModel}` under the "missing/unmapped stays inherit, never
an error" contract. The guard exists because a partial `config.tune
agents.tier harness:pi` write can seed the `pi` bucket from a codex-shaped
default that carries no `/`. What is missing is a per-spawn signal: the
lead sees a child on an unexpected model with no indication that its tier
was consulted and rejected, and nothing at the spawn points at the value or
at `/ws-model-catalog-list` for the accepted shape.

The one signal that does exist today is misleading in this scenario. The
`workflow_manual` advisory (`MODEL_CATALOG_ADVISORY`, gated on
`computePiAliasTableUnset`) fires whenever no tier yields a genuine `pi`
hit, and its text says the `pi` tier table has no entries. A slash-less
entry is not a genuine hit, so a user who did write `small: gpt-5.6-luna`
is told the table is empty.

Related finding, 2026-09-06, while answering which model and effort
`explore` runs with (absorbed here instead of a separate bug ticket):
`explore` drops the tier's effort. `resolveExploreModel` (`spawner.ts`, by
the `explore` registration) keeps only `model` from
`resolveModelForAliasViaWsMcp`, and `buildSpawnArgs` knows `--model`,
`--tools`, `--append-system-prompt`, `--session`/`--no-session`, and the
task, nothing about thinking, so the explore child runs at Pi's default
thinking level whatever effort the owner set on `small`. The RPC-backed
engine applies effort through `setThinkingLevel` after `start()`, and its
`applyModelEffort` comment says that is because Pi has no launch-time flag;
that is stale: the installed CLI (`dist/cli/args.js`) accepts
`--thinking <level>` (`off|minimal|low|medium|high|xhigh|max`), and
`--model` takes an optional `:<thinking>` suffix.

## Decisions

Owner's direction (2026-09-06): validate the configured value against the
models Pi actually knows, and say which one was probably meant. The adapter
already holds that list in-process through `ctx.modelRegistry`, each entry
carrying `provider` and `id`. No `pi --list-models` subprocess is needed.

- **Which list.** The check validates against `ctx.modelRegistry.getAll()`,
  every model Pi's registry knows regardless of auth, because that is the
  list a child's own `--model` resolution consults (Pi `resolveCliModel`
  reads `modelRuntime.getModels()`, deliberately not the auth-filtered
  list). The session's `--models` scope (`ctx.scopedModels`) is not
  consulted: a child is a separate `pi -p --model <value>` process and the
  lead's scope does not bind it. Auth for the second warned case below is
  read through `ctx.modelRegistry.hasConfiguredAuth(model)`, the live
  per-provider predicate, not through `getAvailable()`: that method
  returns `runtime.getAvailableSnapshot()`, a cached list that starts
  empty, is filled by an availability refresh whose failure is swallowed,
  and would make every correct tier read as `no-auth` for the rest of such
  a session. Both `getAll()` and `hasConfiguredAuth` are live reads taken
  at resolution time from the `ExtensionContext` the tool `execute` (its
  fifth argument) or `explore`'s `toolCtx` already holds; there is no
  `session_start` snapshot and nothing to go stale across a
  `modelRegistry.refresh()` or a mid-session login.
- **Accept rule: catalog membership replaces the slash guard.** When
  `config.resolve_agent` answers `resolved_from: "pi"`, the model string is
  accepted only when it equals some catalog entry's `provider/id` exactly.
  Every catalog entry carries a `/`, so the current "contains a `/`" guard
  is subsumed, not weakened. The rule is stricter than the slash guard on
  purpose: for a slashed value absent from the catalog Pi's child launch
  does not fail cleanly. With a known provider and an unknown id
  (`openai-codex/gpt-5.6-lunar`) Pi builds a custom model from the
  provider's default and launches the child on a model the API will reject
  at first request; with an unknown provider the child exits with Pi's
  "Model not found" error. Honoring the configured string would therefore
  spawn a broken child, so the adapter warns and inherits instead. The
  `{#260903-pi-spawner-model-tier-inherit}` sentence that names the slash
  check as the discriminator is rewritten to name catalog membership.
- **Warned case 1, `why: "unknown"`: the value is not a `provider/id`
  entry in the catalog.** A slash-less value like `gpt-5.6-luna`, a typo
  in either half, a provider Pi has no entry for, a `:thinking` suffix, or
  an empty string. This is a rule about the value's form, not only its
  existence: Pi's own `--model` resolution would accept a bare id that is
  unique across providers and would parse a `:thinking` suffix, but the
  tier value contract is exactly `provider/id` (a bare id is ambiguous
  across providers; effort is the tier's separate field), and the warning
  wording says so rather than claiming Pi does not know the model. The
  adapter warns and inherits.
- **Warned case 2, `why: "no-auth"`: the value is in the catalog but
  `hasConfiguredAuth` is false for its provider.** Pi knows the model but
  the provider has no configured auth in this session, so a child on it
  would fail at its first request. The adapter warns and inherits, with a
  line that names the provider instead of offering suggestions.
- **Silent case.** `resolved_from !== "pi"` (no `pi` tier configured; ws
  seeds `default`/`codex`/`claude` for every tier) stays silent and
  inherits as today: the `workflow_manual` advisory already covers the
  unset table, and the live spec sentence "spawns degrade silently to
  inherit while every tier is unset" is kept.
- **Warning text with suggestions.** One line. For `unknown`:
  `warning: tier <alias> is set to "<value>" for harness pi, which is not a
  provider/id entry in Pi's model catalog; inherited <lead provider/id>.`
  followed, when the
  heuristic finds candidates, by `Did you mean <provider/id>[, <provider/id>
  ...]?` (at most three) and otherwise by `No close match in Pi's model
  catalog.` For `no-auth`: `warning: tier <alias> is set to "<value>" for
  harness pi, but provider <provider> has no configured auth; inherited
  <lead provider/id>.` When the catalog is empty (no models at all) the
  `unknown` line says `Pi's model catalog is empty.` in place of the
  suggestion tail. The heuristic is a pure function over the catalog: exact
  `id` match first (the slash-less case: `gpt-5.6-luna` matches every
  `<provider>/gpt-5.6-luna`), then case-insensitive containment in either
  direction between the value's id half and a catalog `id` (which also
  catches the `:thinking` suffix), then a bounded edit-distance match on
  `id`; ties keep the catalog's order. The TUI copy of the line (below)
  appends `See /ws-model-catalog-list for the models usable here.`; the
  tool-result copy does not, since that slash command is a human surface
  the lead model cannot run. That command intentionally keeps showing the
  narrower usable subset (`ctx.scopedModels`, else `getAvailable()`) per
  `{#260903-pi-model-catalog-config-file}`, which this ticket leaves
  unchanged; a suggested `provider/id` can therefore be absent from that
  list when the lead session is scoped or the provider is unauthenticated,
  and the pointer wording says "usable here" for that reason.
- **Surfaces.** The warning attaches to the two resolution sites that
  produce a result the lead reads: `spawnAgent` (its result text, reached
  from `ws-agent-spawn`, `ws-fork`, `ws-execute`, and the discussion fork)
  and `explore`'s implicit `small` lookup (its result text). The
  `ws-agent-list` row repeats the `spawnAgent` warning for registry
  members. In the TUI the same line goes to `ctx.ui.notify` at warning
  level once per spawn. The advisory predicate in `bridge.ts` (below) is
  not a per-spawn warning site; it feeds the advisory only.
- **Advisory reconciliation.** The `workflow_manual` advisory currently
  says the `pi` tier table has no entries whenever no tier is a genuine
  hit, which is false for a user who wrote an unknown value.
  `computePiAliasTableUnset` (boolean) becomes a per-tier report,
  `computePiAliasTableReport`, returning `{unset, rejected: [{alias,
  rejected detail}]}`, and `maybeAppendModelCatalogAdvisory` takes the
  report instead of the flag. When at least one tier is `pi`-labeled but
  rejected, the advisory lists those tiers with the same warning line
  (suggestions included) instead of the empty-table sentence; when none is
  configured it keeps the empty-table sentence. The bridge reaches the
  catalog through the `ExtensionContext` that Pi passes as the fifth
  argument of the registered tool `execute` (currently declared as
  `(_toolCallId, params)` in `startBridge`); the catalog read there is
  passed down to `computePiAliasTableReport` as a parameter, so the report
  function stays IO-free like the resolver.
- Transport failures (tool error, unparsable text) keep today's silent
  inherit; they are already covered by the launcher/version checks, and a
  warning there would fire on every spawn of a broken session.
  `resolved_from` is printed as returned when it appears in a line (it may
  carry a non-bucket value such as `tiers`).
- The warning is not persisted in the `.ws-agents.json` sidecar; a revived
  dormant record shows no warning in its row, and the list row reflects
  the resolution at spawn time only.
- Rejected: a `pi --list-models` subprocess at spawn time, since the
  registry is already in-process and the subprocess would add latency to
  every spawn. Rejected: a `session_start` catalog snapshot, since every
  resolution site already holds an `ExtensionContext` and a snapshot only
  adds a staleness window. Rejected: warn-but-honor for a slashed value
  absent from the catalog, since Pi launches such a child on a custom or
  missing model (see the accept rule). Rejected: warning on the
  never-configured state, for the reason above. Consider making
  `config.tune agents.tier harness:pi` reject or warn on a slash-less
  model in ws-mcp as a later, separate host-neutral change; this ticket is
  adapter-only.

## Spec Impact

Phase 2: `{#260903-pi-spawner-model-tier-inherit}` also states that a
resolved tier effort reaches a process-spawned child as `--thinking` at
launch and an RPC-backed child through `setThinkingLevel`, and that an
inherit or an empty effort passes no level in either path.

Phase 1: `pi-adapter-runtime`: rewrite the genuine-hit paragraph of
`{#260903-pi-spawner-model-tier-inherit}` so the accept rule is
`resolved_from === "pi"` AND exact membership in
`ctx.modelRegistry.getAll()` (the slash check is subsumed), add the two
warned cases (`unknown`, `no-auth`) with their lines beside the silent
inherit for `resolved_from !== "pi"`, and state that the catalog is read at
resolution time; amend the `ws-agent-spawn` and `ws-agent-list` bullets
under `{#260903-pi-delegation-spawner-tools}` with the warning line and
the row field; amend `{#260903-pi-model-catalog-unset-advisory}` for the
per-tier report and the rejected-tier listing while keeping its "spawns
degrade silently while every tier is unset" sentence.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change.
- The warning is one line per spawn, never repeated on dormant resume.
- `resolveModelForAliasViaWsMcp` stays pure with respect to IO: it takes
  the catalog (a list of `{provider, id, hasAuth}` built from `getAll()`
  and `hasConfiguredAuth`) as an argument and returns the optional
  `rejected` detail (Phase 1) alongside
  `model`/`effort`; the caller reads the catalog from its
  `ExtensionContext` and decides how to surface the detail. The suggestion
  heuristic is a separate pure function.
- Never-configured `pi` tiers produce no per-spawn output.

## Phases

### Phase 1: Catalog check, warning line, and suggestions

Extend `resolveModelForAliasViaWsMcp` with the catalog argument, the exact
membership accept rule, and an optional `rejected` detail (`{model,
resolvedFrom, why: "unknown" | "no-auth", suggestions}`); add the pure
suggestion heuristic; have each caller (`spawnAgent`'s context, `explore`,
and the bridge's tool `execute` via its fifth `ctx` argument) build the
catalog from `ctx.modelRegistry.getAll()` and `hasConfiguredAuth` at
resolution time; `spawnAgent` and `explore` append the warning line to
their result text, `spawnAgent` stores it on the record for the list row,
and the TUI path notifies once per spawn with the slash-command pointer.
Replace `computePiAliasTableUnset` with `computePiAliasTableReport` taking
the catalog parameter, and change `maybeAppendModelCatalogAdvisory` to
render the report per the reconciliation bullet. Tests: slash-less `pi`
value whose `id` exists yields `unknown` with the `provider/id` suggestion
and inherit; a slashed value with a typo yields `unknown` with a
containment/edit-distance suggestion and inherit; a `:thinking` suffix
yields `unknown` with the bare `provider/id` suggestion; a known
`provider/id` whose entry has `hasAuth: false` yields `no-auth` naming the
provider and inherit;
an unknown value with an empty catalog yields the empty-catalog wording; a
known and available `provider/id` yields no detail and is accepted;
`resolved_from: default` yields no detail; transport failure yields no
detail; the warning appears once in the spawn result, once in the explore
result, and in the list row; the tool-result copy omits and the TUI copy
carries the slash-command pointer; a revived dormant row carries none; the
advisory lists rejected tiers with suggestions when some exist and keeps
the empty-table sentence when none is configured. Amend the three spec
passages named under Spec Impact. Live check (owner-run): repeat the
verification scenario with `small: gpt-5.6-luna` and confirm the spawn
warning names the value, the inherited model, and the
`openai-codex/gpt-5.6-luna` suggestion, and that the `workflow_manual`
advisory no longer says the table is empty; then set the suggested
`provider/id` and confirm the small worker runs on it with no warning.

### Phase 2: Pass the resolved effort to the explore child

Independent of Phase 1's warning work; may land in either order. Extend
`BuildSpawnArgsOptions` with an optional `thinking` and append
`--thinking <level>` when it is a non-empty string; have
`resolveExploreModel` return `{model, effort}` and `exploreLeaf` forward the
effort; correct the stale `applyModelEffort` comment (the RPC path uses
`setThinkingLevel` because the level may change per restart, not because
the CLI lacks a flag); amend the spec sentence under Spec Impact. No
caller-facing effort parameter is added to `explore`; the tier stays its
only source of model and effort. A level Pi rejects makes the child exit
with Pi's own "Invalid thinking level" error through the existing
spawn-failure path; no adapter-side validation. `buildSpawnArgs` stays pure
and existing callers are unchanged. Tests: `buildSpawnArgs` emits
`--thinking` only when the option is a non-empty string, before the task
argument; `exploreLeaf` passes the tier's effort through (mock the ws-mcp
resolve to return one) and passes nothing on an inherit. Live check
(owner-run): with `small` configured with an explicit effort, confirm the
explore child's transcript shows that thinking level.


## Resolution (2026-09-06)

Absorbed into 260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model on the owner's direction (2026-09-06): a configured tier that Pi's catalog does not know now refuses the spawn instead of warning and inheriting. Phase 1's catalog check, suggestion heuristic, and per-tier advisory report landed in e5e09187 and remain the base the absorbing ticket builds on; its three deferred spec passages and Phase 2 (explore effort pass-through) move there as its Spec Impact and Phase 2.

---
title: Pi tier resolution reads ws's backend-keyed slugs and refuses a spawn whose configured tier is not in Pi's catalog
related:
  260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit: absorbed; its catalog check landed in e5e09187 and its warn-and-inherit outcome is replaced by refusal here; its explore-effort phase is Phase 2 here
  260906-bug-ws-pi-model-warning-keeps-reappearing: absorbed; its advisory dedupe is Phase 3 here, its list-row marker is dropped because a refused spawn leaves no record
  260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches: prerequisite for Phase 3 only; rewrites dispatchMappedWorkflowManual and its advisory call sites
  260906-feat-ws-config-tune-agents-tier-returns-only-the-written-harness: ws-mcp side of the same tier UX; independent
  260906-feat-ws-pi-tool-result-yaml-tui-rendering: its Phase 2 resolved-model line reads `source` from the resolver this ticket's Phase 1 adds; that dependency was previously pointed at the absorbed warning ticket
  260905-feat-ws-pi-harness-config-layer: seeded the harness-keyed pi bucket and the resolve_agent read tool
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7c1d31609df40761
sage-review-completeness-reviewed: 7c1d31609df40761
---

# Pi tier resolution reads ws's backend-keyed slugs and refuses a spawn whose configured tier is not in Pi's catalog

## Background

Owner dogfood, 2026-09-06: children requested at tier `small` and `medium`
ran on the lead's own model and provider usage was drained. Root cause,
confirmed from the persisted config, the adapter source, and the session
transcripts:

- ws stores `agents.tier` values as `{backend, model, effort}` where `model`
  is a backend-keyed slug (`gpt-5.6-luna`), the same shape as its own seed
  defaults and its `codex` bucket. The `pi` bucket in the owner's project
  config held exactly that shape for all four tiers
  (`small: codex/gpt-5.6-luna high`, `medium: codex/gpt-5.6-terra high`,
  `large: codex/gpt-6-astra medium`, `xlarge: codex/gpt-6-astra xhigh`).
  `config.tune` and `SetAgentsTierForHarness` validate effort only; the
  model string is trimmed and stored verbatim for every harness.
- The adapter's `resolveModelForAliasViaWsMcp` (`agents-plugin-pi/src/spawner.ts`)
  accepts a `resolved_from: "pi"` answer only when `model` is an exact
  `provider/id` entry of `ctx.modelRegistry.getAll()` with configured auth
  (since `e5e09187`; before it, when the value contained a `/`).
  `gpt-5.6-luna` fails both rules, so every tier resolution degraded to
  inherit: `openai-codex/gpt-6-astra`, the lead's model, at Pi's default
  thinking level `medium` (inherit passes no effort; the RPC path never
  calls `setThinkingLevel`, the process path passes no `--thinking`).
- The cascade is two-level: the `full-worker` and `execute-worker` tool
  groups include `explore`, which resolves `small` and inherits the worker's
  model in turn; `ws-execute` defaults every non-complex call to `small`.
  Transcript tally for 2026-09-06: 36 child sessions under `ws-pi-agent-*`,
  564 assistant responses, all on `openai-codex/gpt-6-astra` at `medium`,
  roughly 83 USD by catalog prices against a 50x input-price gap to
  `gpt-5.6-luna`; the two lead sessions added about 34 USD.
- `e5e09187` (the absorbed warning ticket's Phase 1) added the catalog
  check, the `unknown`/`no-auth` warning line with suggestions, the
  per-tier `workflow_manual` advisory report, and the `warning` field on
  the registry record and the `ws-agent-list` row. It changed diagnostics
  only: a rejected tier still inherits. The three Pi sessions running that
  day started before it landed and never loaded it. Its three spec
  passages were deferred and are still unwritten; the live spec still
  names the slash check as the discriminator.
- Two follow-up defects were captured against that warning design and are
  folded here. Its "bare id is ambiguous across providers" rationale
  ignored the `backend` field ws returns beside the model, which
  determines the Pi provider. And its warning cardinality was unbounded:
  the advisory is recomputed and re-appended on every `workflow_manual`
  call (every goal cycle), and the list tool re-rendered the stored
  warning for every registry member on every call.
- `explore` drops the tier's effort even on a genuine hit:
  `resolveExploreModel` keeps only `model`, and `buildSpawnArgs` knows no
  thinking flag, so the explore child runs at Pi's default thinking level.
  The `applyModelEffort` comment claiming Pi has no launch-time flag is
  stale: the installed CLI accepts `--thinking <level>`
  (`off|minimal|low|medium|high|xhigh|max`).

Owner direction (2026-09-06, after the drain): a named tier that does not
resolve to a Pi catalog entry must refuse the spawn outright, never
inherit; that covers a configured value Pi's catalog does not know and a
tier with no `pi` entry at all. Inheriting the parent is what made the
misconfiguration invisible and expensive; a refused spawn costs one error
line. Only a spawn that names no tier inherits, because that is the lead's
explicit choice. The owner re-tuned the
four `pi` tiers to `provider/id` form the same day as the immediate
mitigation, so the current adapter already resolves them; this ticket makes
the backend-keyed form work too and closes the silent path.

## Decisions

- **Derive the Pi provider from `backend` when the model has no `/`.** After
  `resolved_from === "pi"` and before the catalog check, a slash-less model
  is expanded to `<provider>/<model>` through a fixed adapter-owned map:
  `codex` -> `openai-codex`, `claude` -> `anthropic`. The expanded string
  then goes through the exact-membership and `hasConfiguredAuth` rules
  unchanged; on acceptance the tier's `effort` applies as a genuine hit. A
  model that already contains a `/` is used as written, never re-prefixed.
  A slash-less model whose `backend` is empty, `pi`, or outside the map is
  not expanded and fails the catalog check. Map entries are confirmed
  against a real `ctx.modelRegistry` entry at implementation; a wrong
  entry can only produce a refusal, never a broken child.
- **A rejected configured tier refuses the spawn.** When the tier is
  `pi`-labeled and the (expanded) model is not a catalog entry (`unknown`)
  or its provider has no configured auth (`no-auth`), `spawnAgent` throws
  before any side effect (no session directory, no registry entry, no alias
  hold), the same way its existing alias and capacity guards throw, so its
  return type and its four callers (`spawner.ts`, `fork.ts`,
  `execute-gateway.ts`, `ask.ts`) are unchanged and the thrown message
  surfaces as the tool error as today; `explore` returns its error result
  instead of running. `ws-agent-spawn`, `ws-fork`, `ws-execute`, the
  discussion fork, and both explore paths all reach one of those two
  sites, so no caller can spawn on an inherited model when a tier was
  configured and rejected. The error text is the existing warning line
  with the head `ws-pi-agent: ws-agent-spawn rejected:` (or `explore
  rejected:`) and the tail `inherited <model>` removed; it names the stored
  value, the expanded string when it differs, the reason, the suggestions
  (at most three) or the no-match/empty-catalog wording, and the
  `config.tune(key: "agents.tier", harness: "pi", ...)` call that fixes it.
  The TUI copy (`ctx.ui.notify`, once per refused spawn) keeps the
  `/ws-model-catalog-list` pointer; the tool-result copy omits it.
- **A named tier with no `pi` entry refuses too.** When the tier is named
  and `config.resolve_agent` answers with `resolved_from !== "pi"` (ws
  seeds `default`/`codex`/`claude` for every tier, so this is the
  never-configured case), the spawn is refused with `why: "unset"`: the
  line says tier `<alias>` has no `pi` entry, names the `resolved_from`
  bucket that answered, and gives the `config.tune(key: "agents.tier",
  harness: "pi", value: {tier: "<alias>", model: "<provider/id>", effort:
  ...})` call that sets it. This is what closes the partially configured
  table: `explore` and every non-complex `ws-execute` name `small`
  implicitly, and a table where `large` is set but `small` is not would
  otherwise reproduce the drain with no signal, since the advisory fires
  only while every tier is unset. On a fresh install this means `explore`
  and `ws-execute` refuse until the tiers are tuned; the advisory and the
  refusal line both say how.
- **Only an omitted tier inherits.** `ws-agent-spawn` and `ws-fork` with
  no `model_name` inherit the parent's model as today; that is the lead's
  explicit choice, not a fallback. Transport failure (an `isError` result,
  missing or unparsable text, a thrown transport error) also inherits
  silently as today: a ws-mcp transport failure breaks every ws tool the
  lead calls and is loud on its own, and refusing there would block every
  spawn of a broken session with a misleading tier message.
- **An empty catalog refuses as well, on purpose.** When
  `modelCatalogFromToolCtx` yields no entries (a missing or shape-drifted
  `ctx.modelRegistry`), every configured tier is `unknown` with the
  existing empty-catalog wording and the spawn is refused. This is the
  opposite answer from the transport carve-out above and the asymmetry is
  deliberate: a ws-mcp transport failure already breaks every ws tool the
  lead calls and is loud on its own, whereas an empty catalog would
  otherwise turn every child into a silently inherited one, which is the
  drain path this ticket closes. The refusal names the empty catalog as
  the reason so the lockout reads as an adapter or Pi-version fault, not a
  tier typo. The rule across all cases is one sentence: a named tier never
  inherits silently on a tier or catalog judgment; only a ws-mcp transport
  failure and an omitted tier inherit.
- **The resolver keeps its shape and reports its source; callers decide.**
  `resolveModelForAliasViaWsMcp` stays IO-free and still returns
  `{model, effort, rejected?}` with `model` = the inherit model on
  rejection, so `computePiAliasTableReport` keeps reading `rejected`
  unchanged; `rejected.why` widens to `"unknown" | "no-auth" | "unset"`,
  and the `unset` detail carries the answering `resolved_from`. It gains
  `source: "tier" | "inherit"`: `tier` when a genuine `pi` hit was
  accepted, `inherit` otherwise (omitted alias, transport failure, and the
  rejected cases whose callers refuse before launching). The YAML-rendering ticket's resolved-model
  line reads `inherited` from `source`, not from `rejected`, because after
  this ticket a `rejected` detail never accompanies a launched child. The parsed `config.resolve_agent`
  shape gains `backend?: string`. In the `rejected` detail, `model` is the
  string checked against the catalog (the expanded form when expansion
  applied) and a new `stored?: string` carries the raw value when it
  differs; the `no-auth` line derives the provider from `model`, never by
  slicing a slash-less value. The spawn and explore callers refuse when
  `rejected` is present and use `model`/`effort` otherwise. Resolution in
  `spawnAgent` moves ahead of `mkdtempSync` so a refusal leaves no
  directory.
- **The record and list row lose the warning.** A refused spawn creates no
  record, so the `warning` field on `RpcAgentRecord` and on the
  `ws-agent-list` row (both added by `e5e09187`) have nothing left to
  carry and are removed; `model` on the row stays the effective launched
  model with its effort suffix. Dormant resume is unaffected: the cached
  `model` on the record is reused as today.
- **Effort reaches the explore child.** `resolveExploreModel` returns
  `{model, effort}`, `exploreLeaf` forwards the effort, and
  `buildSpawnArgs` gains an optional `thinking` that appends
  `--thinking <level>` before the task argument when non-empty. An inherit
  or an empty effort passes no level. A level Pi rejects makes the child
  exit with Pi's own "Invalid thinking level" error through the existing
  spawn-failure path; no adapter-side validation. The stale
  `applyModelEffort` comment is corrected (the RPC path uses
  `setThinkingLevel` because the level may change per restart, not because
  the CLI lacks a flag). No caller-facing effort parameter is added to
  `explore`.
- **Advisory: once per distinct rejected set per session, re-armed on
  compaction.** The bridge keeps the last emitted advisory key, a stable
  string built from the sorted `<tier>=<checked model or ->:<why>` pairs
  (an all-unset table therefore keys as four `unset` rows). The block is appended when the key
  differs from the last emitted one, which covers the first
  `workflow_manual` call of a session, a tier tuned mid-session, and the
  table becoming clean (an empty key emits nothing and resets). The four
  `config.resolve_agent` round-trips per call stay; only emission is
  deduped. The key lives in a holder object owned by the bridge and passed
  into `maybeAppendModelCatalogAdvisory` as a parameter, so that function
  and `computePiAliasTableReport` stay pure for the existing direct-call
  tests and each test starts from a fresh holder. The gate sits inside
  `maybeAppendModelCatalogAdvisory`, so all three call sites are covered:
  the two in the mapped `dispatchMappedWorkflowManual` branches and the
  raw-dispatch path in `startBridge` taken by unmapped roles. The holder
  resets on the adapter's compaction boundary (the event the goal loop
  observes), so the first `workflow_manual` after a compaction re-emits the
  block: a compacted context has lost the text and the advisory is the
  only pressure while the tier is misconfigured. The advisory lists
  rejected tiers with the same line the refusal uses (without the
  `rejected:` head). Since `unset` is now a rejection, `report.unset` as
  computed today would never be true; it is redefined as "every tier is
  rejected `unset`", and the advisory renders the empty-table guidance
  block (`MODEL_CATALOG_ADVISORY`, "configure at least a `small` tier")
  in that state instead of four near-identical `unset` rows. A partially
  configured table renders per-tier rows, its unset tiers included, and
  no guidance block. The two forms are exclusive, as today.
- **Rejected: warn and inherit** (the absorbed warning ticket's outcome).
  It kept the drain path open behind one line the lead can miss; the
  owner chose refusal.
- **Rejected: validating model shape in ws-mcp per harness.** The shape a
  host expects is host logic; ws-mcp stays harness-neutral and stores what
  the caller wrote. The owner clause allows harness-keyed config surfaces,
  not host-specific value grammars.
- **Rejected: requiring the owner to spell every tier as `provider/id`.**
  ws's own seed and `config.list` output use the backend-keyed form, so
  the adapter must read it; the `provider/id` form stays accepted too.
- **Rejected: forwarding the parent's thinking level on inherit.** Inherit
  keeps passing no effort; changing that is a separate cost decision.
- **Rejected: a `tier_rejected` marker on `ws-agent-list` rows.** With
  refusal there is no rejected-tier record to mark.
- **Rejected: a `pi --list-models` subprocess or a `session_start`
  catalog snapshot.** The registry is in-process and every resolution site
  holds an `ExtensionContext`; `getAll()` and `hasConfiguredAuth` are read
  live at resolution time.

## Spec Impact

`pi-adapter-runtime`, all written by this ticket (the absorbed ticket's
deferred spec pass never ran); Phase 1 amends the four anchors below:

- `{#260903-pi-spawner-model-tier-inherit}`: rewrite the genuine-hit
  paragraph so the accept rule is `resolved_from === "pi"` AND exact
  membership in `ctx.modelRegistry.getAll()` of the model after backend
  expansion (slash-less model prefixed from the fixed `backend` -> provider
  map `codex` -> `openai-codex`, `claude` -> `anthropic`; slashed model
  used as written) AND configured auth for its provider, read live at
  resolution time; state that a `pi`-labeled value failing that rule
  refuses the spawn with the error line (`unknown` / `no-auth`, with
  suggestions) and never inherits; state that a named tier whose answer is
  not `resolved_from: "pi"` refuses as `unset` with the `config.tune` hint;
  keep inherit only for an omitted `model_name` and for transport or parse
  failures; drop the "partial tune seeds a codex-shaped default" rationale
  since such a value now resolves through expansion. Phase 2 adds that a
  resolved tier effort reaches a process-spawned child as `--thinking` and
  an RPC-backed child through `setThinkingLevel`, and that inherit or an
  empty effort passes no level in either path.
- `{#260903-pi-delegation-spawner-tools}`: the `ws-agent-spawn` bullet
  names the refusal error; the `ws-agent-list` bullet stays at status,
  alias, title and model (no warning field is added), and its clause "an
  inheriting child (no catalog entry for its alias) shows the parent's own
  concrete model" is rewritten to describe the only inheriting child left,
  one spawned with no `model_name`.
- `{#260903-pi-explore-recon-leaf}`: the lead/fork preset sentence "the
  `"small"` alias (or the inherited model when that resolves to no
  genuine hit, exactly like every other spawn)" is rewritten: `explore`
  always names `small` and refuses when `small` does not resolve to a
  genuine hit, exactly like every other named-tier spawn.
- `{#260903-pi-model-catalog-unset-advisory}`: the per-tier report applies
  the same expansion and lists rejected tiers, unset ones included, with
  their line, and shows the empty-table guidance block instead when every
  tier is `unset`; Phase 3 replaces the "recomputed and re-appended on
  every call while the condition holds" cadence with the per-session key
  rule and the compaction re-arm; the sentence "Spawns and explores still
  degrade silently to inherit while every tier is unset" is replaced: a
  named tier refuses whether unset or rejected, and only an omitted tier
  inherits.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change. Touched
  symbols by file: `src/spawner.ts` holds `resolveModelForAliasViaWsMcp`,
  `spawnAgent`, `BuildSpawnArgsOptions`, `buildSpawnArgs`, `exploreLeaf`,
  `resolveExploreModel`, and the `ws-agent-list` renderer; `src/bridge.ts`
  holds `computePiAliasTableReport`, `maybeAppendModelCatalogAdvisory`,
  `dispatchMappedWorkflowManual`, and `startBridge`; `src/model-catalog.ts`
  holds the suggestion heuristic `suggestModels`.
- The provider map is a small exported constant beside the resolver so the
  advisory and the tests share it; adding a backend is a one-line change.
- `resolveModelForAliasViaWsMcp`, the suggestion heuristic,
  `computePiAliasTableReport`, and `maybeAppendModelCatalogAdvisory` stay
  IO-free; the catalog and the advisory key holder are parameters.
- A refusal is side-effect free: no session directory, no registry entry,
  no alias hold, no orphan.
- The advisory key holder is in-process state on the bridge; it is not
  persisted, resets with the session, and resets on compaction.
- Phase 3 lands after `260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches`
  so the gate is added to the rewritten dispatch, not merged against it.
  Phases 1 and 2 do not depend on it.
- Existing tests for `provider/id` acceptance and transport failure keep
  passing unchanged; the `e5e09187` tests that assert inherit-with-warning
  on `unknown`/`no-auth`, and the older test that asserts silent inherit
  on a non-`pi` `resolved_from`, are rewritten to assert refusal.

## Phases

### Phase 1: Backend expansion and refusal on catalog miss

Add the backend -> provider map and the expansion step to
`resolveModelForAliasViaWsMcp`; widen the parsed `config.resolve_agent`
shape with `backend`; make the rejected detail's `model` the checked
(expanded) string with `stored` beside it; add the `unset` rejection for a
named tier whose answer is not `resolved_from: "pi"`; confirm the `claude` map entry
against the live catalog; move resolution in `spawnAgent` ahead of the
session-directory creation and throw on `rejected`; return the error
result from `explore` on `rejected`; add `source` to the resolver's
result; redefine `report.unset` and keep the guidance block for the
all-unset table; reword the line
into the refusal form with the `config.tune` hint; remove the `warning`
field from the record and the list row. Amend the three spec passages
under Spec Impact (the Phase 1 parts, all four anchors).
Tests: `codex` + `gpt-5.6-luna` with `openai-codex/gpt-5.6-luna` in the
catalog is accepted with the tier's effort; `claude` + `sonnet` expands to
`anthropic/sonnet`; a slashed value is never re-prefixed; empty or `pi`
backend with a slash-less model is rejected `unknown`; `codex` +
`gpt-5.6-lunar` is rejected `unknown` with the expanded string as `model`,
the raw value as `stored`, and the `openai-codex/gpt-5.6-luna` suggestion;
an expanded hit whose provider lacks auth is `no-auth` and the line names
`openai-codex`; an empty catalog gives the empty-catalog wording;
`spawnAgent` on a rejected tier throws, creates no directory, no record,
and holds no alias; a named tier answered `resolved_from: default` is
rejected `unset` and refused with the `config.tune` hint, while an omitted
`model_name` still inherits; `source` is `tier` on an accepted hit and
`inherit` on every other outcome; `explore` on a rejected tier returns the
error and runs nothing; transport failure still inherits silently; the list row carries no warning field; the
advisory report shows no rejected tier for a fully backend-keyed table,
four `unset` rejections with `unset: true` for a never-configured table
(rendered as the guidance block), and one `unset` row with `unset: false`
when only `small` is missing.
Live check (owner-run): set `small` back to the backend-keyed
`gpt-5.6-luna` form via `config.tune` and confirm a `small` worker runs on
`openai-codex/gpt-5.6-luna` at thinking `high`; set `small` to
`gpt-5.6-lunar` and confirm `ws-agent-spawn` and a lead `explore` both
return the refusal naming the suggestion, with no new `ws-agent-list` row;
reset `small` (`config.tune` with `reset: true` for that tier, or remove
its `pi` entry) and confirm both refuse as `unset` with the hint while
`ws-agent-spawn` without `model_name` still runs on the lead's model;
confirm the `workflow_manual` advisory lists the affected tier in each
state.

### Phase 2: Pass the resolved effort to the explore child

Independent of Phase 1; may land in either order. Extend
`BuildSpawnArgsOptions` with an optional `thinking` and append
`--thinking <level>` when it is a non-empty string; have
`resolveExploreModel` return `{model, effort}` and `exploreLeaf` forward
the effort; correct the stale `applyModelEffort` comment; amend the spec
sentence under Spec Impact. `buildSpawnArgs` stays pure and existing
callers are unchanged. Tests: `buildSpawnArgs` emits `--thinking` only
when the option is a non-empty string, before the task argument;
`exploreLeaf` passes the tier's effort through (mock the ws-mcp resolve to
return one) and passes nothing on an inherit. Live check (owner-run): with
`small` configured with an explicit effort, confirm the explore child's
transcript shows that thinking level.

### Phase 3: Dedupe the advisory per session

Lands after the static-body ticket. Add the key holder to the bridge,
thread it into `maybeAppendModelCatalogAdvisory`, gate emission on a key
change, and reset the holder on the compaction boundary; amend the
advisory spec passage under Spec Impact. Tests: two consecutive
`workflow_manual` calls with the same rejected set append the block once;
a changed set appends again; a clean table after a rejected one appends
nothing and resets the key so a later rejection warns again; a compaction
reset makes the next call append again with the same set; the unmapped
raw-dispatch path is gated the same way. Live check (owner-run): arm a
goal in a session with one rejected tier and confirm the advisory appears
on the first cycle only; tune the tier to a valid value and confirm no
further advisory.

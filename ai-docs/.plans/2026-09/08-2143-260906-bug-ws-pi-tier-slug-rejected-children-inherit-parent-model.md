# Plan: 260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model — Phase 1: Backend expansion and refusal on catalog miss

## Relevant Ticket Contract

- Backend expansion: after `resolved_from === "pi"`, before the catalog check, a
  slash-less `model` is prefixed via a fixed exported map `codex ->
  openai-codex`, `claude -> anthropic`; a slashed model is never re-prefixed; a
  slash-less model whose `backend` is empty/`pi`/unmapped is left as-is (and
  therefore fails the catalog check as `unknown`).
- A rejected configured tier (`unknown`/`no-auth`, post-expansion) or a named
  tier with a non-`pi` `resolved_from` (`unset`) refuses the spawn: `spawnAgent`
  throws before any side effect (before `mkdtempSync`), no directory/registry
  entry/alias hold is created, and `explore` returns its error result instead
  of running. Only an omitted `model_name` (and a transport/parse failure)
  still inherits.
- Error text: existing warning line minus the `inherited <model>` tail, plus
  the stored (raw) value, the expanded string when it differs, the
  reason/suggestions (≤3) or no-match/empty-catalog wording, and a
  `config.tune(key: "agents.tier", harness: "pi", value: {...})` hint. TUI copy
  (`ctx.ui.notify`) keeps the `/ws-model-catalog-list` pointer; the tool-result
  copy omits it.
- `resolveModelForAliasViaWsMcp` stays IO-free and keeps returning
  `{model, effort, rejected?}` with `model` = the inherit model even on
  rejection (callers decide to refuse, not the resolver). `rejected.why`
  widens to `"unknown" | "no-auth" | "unset"`; the `unset` detail carries the
  answering `resolved_from`. `source: "tier" | "inherit"` is retained/reused
  as-is. Parsed `config.resolve_agent` shape gains `backend?: string`. In
  `rejected`, `model` becomes the checked (expanded) string; a new `stored?`
  carries the raw value only when it differs; the `no-auth` line derives the
  provider from `model` (never by slicing a slash-less raw value).
- `RpcAgentRecord.warning` and the `ws-agent-list` row's `warning` field are
  removed (a refused spawn creates no record to carry one); `model` on the row
  stays the effective launched model.
- Provider map is a small exported constant beside the resolver, shared by
  tests/advisory; `resolveModelForAliasViaWsMcp`, `suggestModels`,
  `computePiAliasTableReport`, `maybeAppendModelCatalogAdvisory` stay IO-free.
- `report.unset` is redefined as "every tier is rejected `unset`"; the
  all-unset table still renders the `MODEL_CATALOG_ADVISORY` guidance block, a
  partially-configured table renders per-tier rows (unset ones included) with
  no guidance block — the two forms stay exclusive.
- Amend the Phase-1 portions of all four named spec anchors in
  `pi-adapter-runtime`.
- Existing `provider/id` acceptance and transport-failure tests keep passing
  unchanged.

## Out of Scope

- Phase 2 (effort/`--thinking` reaching a genuine-hit `explore` child via a new
  `resolveExploreModel` extraction) — **already substantially implemented** on
  the worker/deep-collection path (see Codebase Findings); do not touch unless
  a Phase-1 regression forces it.
- Phase 3 (advisory per-session dedupe key/holder) — lands after the separate
  static-body-cut ticket; not touched here.
- Any ws-mcp (`agents-plugin/`, `agents-plugin-tool/`) change — adapter-only.
- Rewriting `formatExploreTierRefusal`'s overall prose/structure beyond adding
  the `unset` `resolvedFrom` + `config.tune` hint it currently lacks.

## Codebase Findings

- `agents-plugin-pi/src/model-catalog.ts#L9-19` — `TierRejection` is
  `{model, resolvedFrom} & ({why:"unknown", suggestions} | {why:"no-auth"})`;
  needs a third `{why:"unset"}` arm and an optional `stored?: string` field
  (common to all arms). `TierFailure.kind` already includes `"unset"` — it
  stays for the `requireTier`/explore fail-closed path, now redundant with
  `rejected` for unset/unknown/no-auth but still consumed by
  `formatExploreTierRefusal`.
- `agents-plugin-pi/src/model-catalog.ts#L89-98` — `formatTierWarning` is the
  single existing "warning" line builder (`warning: tier X is set to Y ...;
  inherited Z[...]`), used by both `spawnAgent`'s old warn-and-inherit path and
  the `workflow_manual` advisory. Its `inherited <model>` tail must go and a
  `config.tune(...)` hint must be added; it has no `unset` branch today. This
  is the natural function to evolve into the shared "rejection line" builder
  (no head, no inherit tail) that both the advisory (line has no `rejected:`
  head, per spec) and the new spawn-refusal message (which prepends the head)
  can reuse — avoids duplicating the unknown/no-auth/unset prose in two
  places.
- `agents-plugin-pi/src/model-catalog.ts#L100-113` — `formatExploreTierRefusal`
  already exists and is reused by both explore call sites (see below); its
  `unset` branch (`"small is not configured for harness pi"`) needs the
  answering `resolvedFrom` bucket name and the `config.tune` hint added —
  everything else in it (no-auth provider slicing off `model`, catalog-empty,
  unknown) already matches the ticket's post-expansion behavior once `model`
  carries the expanded string.
- `agents-plugin-pi/src/spawner.ts#L403-441` (`resolveModelForAliasViaWsMcp`)
  — parses `{model, effort, resolved_from}` only (no `backend`); the
  `resolved_from !== "pi"` branch (L428-430) returns only a `failure`, not a
  `rejected` — this is the exact spot to add the `rejected` (why:"unset")
  alongside it. The catalog-membership check (L431) runs on the raw
  `parsed.model` with no expansion step — insert backend expansion between
  the `resolved_from === "pi"` gate (L428) and this line. `unknown`/`no-auth`
  paths (L432-439) already build `rejected`; wire in the expanded `model` +
  `stored` there.
- `agents-plugin-pi/src/spawner.ts` — no `BACKEND_TO_PROVIDER`-style constant
  exists yet; confirmed real provider ids from `test/spawner.test.ts:555`
  (`openai-codex`) and `test/fork-prefix.integration.test.ts:27-28`
  (`anthropic` / `openai-codex`) — the ticket's two map entries are valid
  registry provider ids, not placeholders to invent.
- `agents-plugin-pi/src/spawner.ts#L2427-2489` (`spawnAgent`) — resolution
  **already runs before `mkdtempSync`** (L2435 vs L2452), so "move resolution
  ahead of `mkdtempSync`" is already satisfied; no reordering needed. The
  `ctx.requireTier` branch (L2436-2440) already throws via
  `formatExploreTierRefusal` for explore's simple mode (covers transport/parse
  too, deliberately stricter than ordinary spawns). The gap is L2441-2442,
  L2471: for an **ordinary** spawn (`requireTier` unset/false — this is every
  `ws-agent-spawn`/`ws-fork`/`ws-execute` call), a `rejected` resolution only
  builds a `warning` string and still proceeds to launch on the inherited
  model — this is the exact drain bug. Add a second branch, gated on
  `params.modelName && resolution.rejected` (not `ctx.requireTier`), that
  throws before the `alias`/`runSpawnGuards` step (L2444+), using the new
  refusal formatter; notify via `ctx.notifyTierWarning` for the TUI copy.
  Delete the L2441-2442 warning computation and the `warning` field from the
  record literal (L2464-2488) and the return object (~L2553).
- `agents-plugin-pi/src/spawner.ts#L2431` (return type),
  `#L793` (`RpcAgentRecord.warning`), `#L2753-2768` (`listAgents` type +
  spread) — three sites to drop the `warning` field entirely, plus tool
  description strings mentioning it at `#L2998` (`ws-agent-spawn`) and
  `#L3099` (`ws-agent-list`).
- `agents-plugin-pi/src/spawner.ts#L2967-2976`
  (`resolveRequiredExploreModel`, used by the worker/deep-collection
  `explore` blocking-leaf path at `#L3229-3234`) and `#L3201-3227`
  (lead/fork persistent `explore`, `mode === "simple"` sets
  `requireTier: true`) — **both explore paths already fail closed on any
  bad resolution**, including `unset`/`unknown`/`no-auth`/transport/parse.
  "explore returns its error result instead of running" is already true;
  only the refusal *message* needs the `unset` bucket-name + `config.tune`
  hint (via `formatExploreTierRefusal`, see above).
- `agents-plugin-pi/src/spawner.ts#L241-301` (`BuildSpawnArgsOptions`,
  `buildSpawnArgs`) and `#L693-742` (`exploreLeaf`) — **Phase 2's
  `--thinking` forwarding already exists** (`opts.thinking` pushed at
  L296-298, `exploreLeaf` already threads `options.effort` into it at
  L734, and `resolveRequiredExploreModel` already returns `{model, effort}`
  consumed by the worker-recon call at L3230-3234). This was added by an
  unrelated earlier ticket (`abee6d7e feat(pi): persist two-mode exploration
  researchers`), landed after this ticket's Background was written. No
  `resolveExploreModel` function exists under that name (Phase 2 names it as
  a to-be-extracted symbol) — leave it alone; it is Phase 2/out of scope, and
  the current inline shape already satisfies the ticket's live-check intent.
- `agents-plugin-pi/src/execute-gateway.ts#L258-259`
  (`resolveExecuteModelAlias`) and `#L665-681` (`ws-execute`'s `spawnAgent`
  call) — `complex:false` (the default) passes `modelName: "small"` with no
  `requireTier`; this is the exact `ws-execute`-default-tier drain path from
  the ticket's Background and is fixed for free once `spawnAgent`'s new
  general refusal branch lands.
- `agents-plugin-pi/src/fork.ts#L717-731` (`ws-fork`) — same shape, passes
  `model_name` straight through to `spawnAgent`; fixed the same way.
- `agents-plugin-pi/src/ask.ts#L1186-1210` (discussion fork) — never sets
  `modelName` at all, so it always takes the omitted-tier inherit path;
  unaffected by this ticket, confirming the "only an omitted tier inherits"
  rule needs no special-casing there.
- `agents-plugin-pi/src/bridge.ts#L114-122` (`MODEL_CATALOG_ADVISORY`) — its
  prose currently says "ws-agent-spawn currently inherits the parent
  session's model for unmapped tiers," which becomes false once Phase 1
  lands (a named-but-unset tier now refuses too). Not named in Constraints as
  a touched symbol, but leaving it unedited would ship an advisory that
  actively contradicts the new refusal behavior — reword alongside the other
  three anchors.
- `agents-plugin-pi/src/bridge.ts#L141-149` (`computePiAliasTableReport`) —
  today `if (model !== undefined || rejected) report.unset = false;` treats
  any rejection (soon including `unset`) as proof the table isn't "unset."
  Needs: `unset` flips false on an **accept** (`model !== undefined`) or on
  any rejection whose `why !== "unset"`; a `why === "unset"` rejection is
  still pushed to `report.rejected` but does not flip `unset`. This keeps the
  existing transport/parse-failure tests (`unset: true, rejected: []`, e.g.
  `test/bridge.test.ts:499-521`) passing unchanged while making the
  "all-four-unset" test (`test/bridge.test.ts:454-459`) assert 4 `rejected`
  entries instead of `rejected: []`.
- `agents-plugin-pi/src/bridge.ts#L174-180`
  (`maybeAppendModelCatalogAdvisory`) — per-tier lines call `formatTierWarning`
  directly; once that function's tail changes (see finding above), this call
  site's output changes for free, but confirm the spec's "without the
  `rejected:` head" requirement — the shared builder must not itself carry a
  `rejected:`/`ws-agent-spawn rejected:` head, only the spawn-refusal call
  site prepends one.
- `agents-plugin-pi/test/spawner.test.ts#L539-544,559-630` — the
  `resolveModelForAliasViaWsMcp`/`formatTierWarning` unit tests assert on the
  **resolver's** contract (`model` = inherit value even when rejected), which
  the ticket keeps unchanged; these mostly stay valid but the `unset`-source
  loop at `#L576-578` (`for (const source of ["codex","default","tiers",
  "claude",""])`) must be rewritten from "stays silent" (`{model:
  "lead/model"}`) to asserting a `rejected: {why:"unset",...}` detail (per
  Constraints: "the older test that asserts silent inherit on a non-`pi`
  `resolved_from` ... rewritten to assert refusal" — refusal here means the
  resolver reports it as rejected; the actual throw is tested at the
  `spawnAgent`/tool level, not in this resolver-level describe block).
- `agents-plugin-pi/test/agent-sidecar.test.ts#L146,177-188` — "spawn-time
  warning survives parking but not sidecar rehydration" constructs a record
  with `warning` and asserts `listAgents(...)[0].warning` — this whole test
  must be deleted once the field is removed (nothing left to assert).
- `agents-plugin-pi/test/bridge.test.ts#L439-533` — `computePiAliasTableReport`
  describe block: L454-459 ("no tier resolves to pi -> true") must expect 4
  `rejected` entries (all `why:"unset"`); L462-479 ("rejected tiers replace
  the empty table sentence") already exercises `unknown`/`no-auth` mixed with
  an implicit fail (medium has no slash and no backend field, so it still
  resolves `unknown` post-expansion) — confirm its exact-text assertions
  (`Did you mean...`, `provider locked has no configured auth`) still hold
  once `formatTierWarning`'s tail changes, and update the literal expected
  strings there and at L491-493, 586.
- `ai-docs/spec/pi-adapter-runtime.md#L628-666`
  (`{#260903-pi-spawner-model-tier-inherit}`) — describes the **pre-e5e09187**
  slash-check rule (stale, confirmed by the ticket's own Background: "the
  live spec still names the slash check as the discriminator"); rewrite per
  the ticket's Spec Impact bullet (getAll-membership + configured-auth +
  backend expansion + refusal, not inherit).
- `ai-docs/spec/pi-adapter-runtime.md#L449-462`
  (`{#260903-pi-delegation-spawner-tools}`, `ws-agent-list` bullet) — line
  ~459-460 ("an inheriting child (no catalog entry for its alias) shows the
  parent's own concrete model") is the exact clause the Spec Impact bullet
  names for rewrite (describe only the omitted-`model_name` inherit case; no
  `warning` field to mention, matching current code and current spec text —
  the spec never documented `warning` on this bullet, so nothing to remove
  there).
- `ai-docs/spec/pi-adapter-runtime.md#L560-596`
  (`{#260903-pi-explore-recon-leaf}`) — **the exact sentence the ticket quotes
  for rewrite ("the `"small"` alias (or the inherited model when that resolves
  to no genuine hit...)") no longer exists verbatim** in this anchor; the text
  already reads "refuse before guards/allocation on every bad resolution,"
  reflecting the persistent-exploration work that landed after this ticket was
  drafted. Treat the ticket's quoted sentence as historical context, not a
  literal find-target: confirm the current anchor text already states the
  post-Phase-1 behavior (it appears to) and make only the minimal edit needed
  (if any) rather than searching for a sentence that isn't there.
- `ai-docs/spec/pi-adapter-runtime.md#L688-706`
  (`{#260903-pi-model-catalog-unset-advisory}`) — contains the exact sentence
  named for replacement ("Ordinary spawns still degrade silently to inherit
  while every tier is unset; simple explore and deep collection instead fail
  closed before child allocation.") at L704-705; rewrite per Spec Impact.

## Implementation Plan

1. `agents-plugin-pi/src/model-catalog.ts`:
   - Widen `TierRejection` (L9-12) to
     `{model: string; resolvedFrom: string; stored?: string} & ({why:"unknown"; suggestions: string[]} | {why:"no-auth"} | {why:"unset"})`.
   - Rework `formatTierWarning` (L89-98) into the shared no-head/no-inherit
     rejection-line builder: drop the `inherited <model>` tail; add a
     `why:"unset"` branch (names the tier alias and the answering
     `resolvedFrom` bucket); append a `config.tune(key: "agents.tier",
     harness: "pi", value: {tier: "<alias>", model: "<provider/id>", effort:
     ...})` hint to every branch. Keep the leading `warning: tier X is set to
     Y ...` phrasing usable as-is by the advisory (no head); the spawn-refusal
     call site (step 3) prepends its own head.
   - Add the `stored` value to the sentence when present (raw vs. expanded).
   - Update `formatExploreTierRefusal`'s `unset` ternary branch (L110-111) to
     name the answering `resolvedFrom` bucket and the `config.tune` hint,
     reusing the same phrasing fragment as the shared builder where practical.
2. `agents-plugin-pi/src/spawner.ts`:
   - Add an exported `BACKEND_TO_PROVIDER: Record<string, string>` constant
     next to `resolveModelForAliasViaWsMcp` (L365-402 area):
     `{ codex: "openai-codex", claude: "anthropic" }`.
   - Widen the parsed shape (L419-427) to include `backend?: unknown`,
     validating `typeof parsed.backend !== "string"` (when present) as a
     parse failure like the existing fields.
   - Between the `resolved_from !== "pi"` check (L428-430, now also building
     `rejected: {why:"unset", model: parsed.model, resolvedFrom}`) and the
     catalog lookup (L431), insert expansion: if `parsed.model` has no `/`,
     look up `BACKEND_TO_PROVIDER[parsed.backend ?? ""]`; if found, set
     `stored = parsed.model` and use `${provider}/${parsed.model}` as the
     checked model for the rest of the function (catalog lookup, and
     `rejected.model` on `unknown`/`no-auth`, with `stored` carried only when
     it differs from the checked value).
   - `spawnAgent` (L2427-2489): after the existing `requireTier` throw
     (L2436-2440), add:
     `if (!ctx.requireTier && params.modelName && resolution.rejected) throw ...`
     using the new shared formatter — TUI copy via `ctx.notifyTierWarning`
     (keeps the `/ws-model-catalog-list` suffix from
     `tierWarningNotifierFromToolCtx`), tool-error copy via the thrown
     `Error` message, both headed `ws-pi-agent: ws-agent-spawn rejected:`
     (matching the existing `reserveAgentAlias`/`evictForCapacity` head
     convention in this same function, regardless of the actual calling tool
     — do not special-case `ws-fork`/`ws-execute` heads).
   - Delete the L2441-2442 `warning` computation and every remaining
     `warning` reference: `RpcAgentRecord.warning` (L793), the record
     literal spread (~L2471), the `spawnAgent` return type (L2431) and
     return object (~L2553), `listAgents`'s return type and spread
     (L2753, L2764), and the `warning?` mentions in the `ws-agent-spawn`
     (L2998) and `ws-agent-list` (L3099) tool descriptions.
3. `agents-plugin-pi/src/bridge.ts`:
   - `computePiAliasTableReport` (L141-149): change the unset-flip condition
     to `if (model !== undefined) report.unset = false;` plus, inside the
     `if (rejected)` branch, `if (rejected.why !== "unset") report.unset =
     false;` (in addition to always pushing to `report.rejected`).
   - `maybeAppendModelCatalogAdvisory` (L174-180): no structural change beyond
     what the `formatTierWarning` rework already produces — verify the
     rendered per-tier line has no `rejected:` head.
   - `MODEL_CATALOG_ADVISORY` (L114-122): reword the "ws-agent-spawn currently
     inherits ... for unmapped tiers" sentence to describe refusal instead.
4. `ai-docs/spec/pi-adapter-runtime.md`: amend the Phase-1 portions of the
   four anchors per Spec Impact — `{#260903-pi-spawner-model-tier-inherit}`
   (L628-666, full rewrite of the accept/reject rule), the `ws-agent-list`
   clause under `{#260903-pi-delegation-spawner-tools}` (~L459-460), the
   already-mostly-current `{#260903-pi-explore-recon-leaf}` (L560-596, confirm
   text matches, make minimal edits only), and
   `{#260903-pi-model-catalog-unset-advisory}` (L688-706, replace the
   silent-inherit sentence at L704-705).
5. Tests (add/update alongside the above, per the ticket's Phase 1 "Tests:"
   list): `test/spawner.test.ts` (expansion cases, `unset` rejection shape,
   `spawnAgent`-level throw/no-directory/no-record/no-alias-hold on rejection,
   `source` field values, explore-on-rejected-returns-error,
   transport-failure-still-inherits), `test/bridge.test.ts`
   (`computePiAliasTableReport`'s redefined `unset`, advisory rendering for
   fully-keyed/all-unset/partially-unset tables), `test/agent-sidecar.test.ts`
   (delete the now-invalid `warning`-survival test).

## Verification Plan

- `cd agents-plugin-pi && npm test` (runs `node --test`, covering
  `test/spawner.test.ts`, `test/bridge.test.ts`, `test/agent-sidecar.test.ts`,
  `test/execute-gateway.test.ts`, `test/fork.test.ts`,
  `test/persistent-explore.test.ts`, and the rest of the suite) — must pass
  with zero failed/skipped, matching the existing project convention of a
  full-suite rerun after a spawner/bridge change.
- Targeted rerun during iteration:
  `node --test agents-plugin-pi/test/spawner.test.ts agents-plugin-pi/test/bridge.test.ts agents-plugin-pi/test/agent-sidecar.test.ts`.
- Live check (owner-run, not part of this survey/execution): the four
  scenarios listed in the ticket's Phase 1 body (backend-keyed `small` runs at
  `high`; a typo'd `small` refuses with a suggestion and no new
  `ws-agent-list` row; a reset `small` refuses as `unset` while an
  omitted-`model_name` spawn still runs on the lead's model; the
  `workflow_manual` advisory lists the affected tier in each state).

## Escalations

- None.

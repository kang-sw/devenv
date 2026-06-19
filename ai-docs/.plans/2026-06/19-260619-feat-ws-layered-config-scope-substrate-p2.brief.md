# Brief: 260619-feat-ws-layered-config-scope-substrate (Phase 2)

## Intent

Migrate `prefer_mercenary` from its bespoke one-way in-memory flip onto the
layered config scope substrate built in Phase 1. It becomes a `session`-default
config item with desired-state get/set: the lead can both **enable AND disable**
it on the same session key, replacing the current enable-only flip. This closes
`260618-bug-ws-prefer-mercenary-one-way-flip`. Mercenary *availability* is
unchanged — only the default-delegation-guidance toggle gains a revert path.

## Scope Boundary

In scope (Phase 2):
- Register `prefer_mercenary` as a layered config item, declared default scope
  `session`, builtin default `false`.
- `ws.lead.prefer_mercenary` becomes desired-state: accept an `enabled` boolean;
  set the value at session scope through the Phase 1 `Resolver.Set`.
- The render-guidance read path resolves `prefer_mercenary` through the
  `Resolver` (session > project > global > builtin) so guidance follows BOTH
  transitions (on and off), not a latched flag.
- Retire the bespoke one-way path: `sessionStore.setPreferMercenary`, the
  `sessionRecord.PreferMercenary` typed field, and the `sessionEntry.preferMercenary`
  cache field. The value now lives in the session `Overrides` overlay (Phase 1).
- `config.show` (with `session_key`) reports `prefer_mercenary`'s resolved value
  and scope (the "get"). Extend `ScopedShow` minimally so registered default-scope
  items are always enumerated (report builtin when unset).

Explicitly OUT of scope:
- Any change to mercenary *availability* or the always-on on-request tip.
- The `config.agents_tier` surface, `config.prompt.*` namespace, sibling children.
- New generic `config.set`/`config.get` MCP tools (still deferred).
- Config file format migration.

## Caller-Visible Contract

Spec anchor (authored, `ai-docs/spec/mcp-tools.md`):
- `#260619-prefer-mercenary-session-scope-item` — the Planned 🚧 callout under the
  Mercenary Delegation Surface. This slice implements it; strip the 🚧 marker in
  the doc pre-pass after implementation.

Observable behavior this slice must deliver:
- `ws.lead.prefer_mercenary(session_key)` (no `enabled`, legacy shape) → enables
  (backward compatible). `ws.lead.prefer_mercenary(session_key, enabled: true)`
  → enables. `ws.lead.prefer_mercenary(session_key, enabled: false)` → disables.
- After enabling, `playbook.render` of an implementer/reviewer playbook emits the
  mercenary-primary guidance block; after disabling on the same key, a subsequent
  render omits it (revert works — the 260618 closer).
- Response text: `prefer_mercenary: enabled` / `prefer_mercenary: disabled`.
- Lead-only is preserved: a non-lead key is rejected by the existing `ws.lead.*`
  prefix gate (unchanged).
- `config.show(session_key)` reports `prefer_mercenary` with its resolved scope.

## Contract Instructions

- `internal/wsconfig/scope.go`: register `prefer_mercenary` with declared default
  scope `ScopeSession` (use the existing `RegisterDefaultScope`; an `init()` or a
  named registration site is fine — follow the existing registry shape). Define a
  stable item-key constant (e.g. `ItemPreferMercenary = "prefer_mercenary"`) for
  shared use across packages, or reuse an existing constant if one exists.
- `internal/wsconfig/resolver.go`: if no boolean convenience read exists, you may
  add a small `(r *Resolver) GetBool(itemKey)` helper that resolves the string and
  treats `"true"` as true (empty/absent/builtin → false), returning the resolved
  scope too. Keep it additive; do not change `Get`'s signature.
- `internal/wsconfig/scoped_show.go`: also enumerate keys from the default-scope
  registry so registered items (like `prefer_mercenary`) always appear in
  `ResolvedOverrides`, reporting `builtin` when unset. Keep deterministic sort.
- `internal/mcp/session_auth.go`: remove `setPreferMercenary` (one-way),
  `sessionRecord.PreferMercenary`, and `sessionEntry.preferMercenary`. The value
  is read/written through the existing `getOverride`/`setOverride` accessors via
  the resolver path. Do NOT add a new session backend; reuse Phase 1 accessors.
- `internal/mcp/server.go`:
  - Read path (~L799-808, inside the `entry.scope == roleLead` block of the
    `playbook.render` dispatch): replace `preferMercenary = entry.preferMercenary`
    with a resolver read. Build a resolver with the existing `sessionConfigAdapter`
    (the Phase 1 fix-cycle pattern used by `config.show`) and call the bool read
    for `prefer_mercenary` with the lead's `session_key`.
  - Write path (`ws.lead.prefer_mercenary`, ~L815-827): read the optional
    `enabled` boolean argument (default `true` when absent for backward compat);
    call `Resolver.Set("prefer_mercenary", "true"/"false", SetOptions{})` (no
    explicit scope → declared default `session`, routed to the session store via
    the `SessionWriter` adapter). Return `prefer_mercenary: enabled`/`disabled`.
    Keep the `unknown_session` error contract. Do NOT add a second role check —
    the existing `ws.lead.*` prefix gate is the sole authority (the current code
    comment says so explicitly).
  - Tool schema (~L1829): add an optional `enabled` boolean property; update the
    description to state it sets (enable or disable) the default delegation
    guidance, replacing "Flip" with desired-state wording. Keep `session_key`
    required and `enabled` optional.
- `internal/mcp/playbook_tools.go`: no behavior change — the guidance block keys
  off the boolean passed into `renderPlaybook`; we only change how that boolean is
  sourced (resolver instead of the latched field).

Forbidden:
- No new session backend; no rewrite of the session store beyond removing the
  retired one-way field/method.
- No second role check on the tool; no change to mercenary availability.
- No temporary/mock storage; the value persists in the session `Overrides` overlay.

## Integration Test Instructions

- Boundary: `internal/mcp` package tests (the toggle + render-guidance behavior is
  the 260618 closer and lives at the render dispatch layer); add `internal/wsconfig`
  coverage for the registry/`GetBool`/ScopedShow-enumeration if logic is added there.
- New or extended test file under `internal/mcp/` (e.g. extend the existing
  session/prefer_mercenary test if present, else add one).
- Pass criteria:
  - **260618 repro**: on one session key, set `enabled:true` then render an
    implementer playbook → guidance block present; set `enabled:false` on the same
    key then render again → guidance block ABSENT. Both transitions observable.
  - Legacy call shape `ws.lead.prefer_mercenary(session_key)` (no `enabled`) still
    enables.
  - A non-lead key calling `ws.lead.prefer_mercenary` is rejected (prefix gate).
  - `prefer_mercenary` resolves with `session` scope after a set; `config.show`
    reports it.
  - Build clean; full `go test ./internal/wsconfig/... ./internal/mcp/...` read.
    Note the two known pre-existing golden-hash failures
    (`TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance`,
    `TestPlaybookPrintGoldenLeadWorkflowManual`) are unrelated to this slice; no
    NEW failures may be introduced.

## Implementation Strategy Decisions (settled — do not reopen)

- `prefer_mercenary` rides the session `Overrides` overlay (Phase 1 Lead Decision
  4 anticipated exactly this); it is NOT a typed field anymore.
- Clean-cut removal of the one-way path is safe because session keys are ephemeral
  (the ephemeral session-auth model has no cross-version persistence contract;
  cache deletion is the reset path). No read-compat shim for the legacy bool field.
- Backward-compatible call: absent `enabled` defaults to `true` (preserves the old
  enable-only call site).
- Gating stays at the `ws.lead.*` prefix gate; no redundant role check, no resolver
  `CapabilityCheck` duplication for this item (the substrate hook remains available
  for items reached via other tools).
- The render read resolves through `Resolver.Get` (full precedence), not a direct
  session-only peek, so a future project/global default could also influence it.

## Rejected Alternatives

- Keeping the typed `PreferMercenary bool` field with a read-compat fallback —
  rejected; ephemerality makes the clean cut correct and avoids dual sources of
  truth.
- Adding a separate `config.get` MCP tool for the "get" — rejected; `config.show`
  scope reporting (Phase 1) already provides it once the item is enumerated.
- A second lead-role check in the tool dispatch — rejected; contradicts the
  existing code comment and the keyed-gate-is-sole-authority rule.

## Approach

- Register the item + builtin default; add a bool resolver read if needed.
- Reroute the render read through the resolver; change the tool to desired-state.
- Remove the retired one-way field/method.
- Extend ScopedShow to enumerate registered items.
- Add the on→off→on render-guidance regression test.

## Constraints

- Additive where possible; the only removals are the retired one-way
  field/cache/method this migration explicitly replaces.
- English-only code comments.
- impl-playbook Verify: read full test/build output before claiming pass; resolve
  introduced warnings; diagnose blame before fixing.

## Out of scope

- Mercenary availability, config.agents_tier, config.prompt.*, sibling children,
  format migration. See Scope Boundary.

## Details

- Item key: `prefer_mercenary`; values `"true"`/`"false"`; builtin/unset → false.
- Declared default scope: `session` (routes to `keys/<key>.json` Overrides via the
  Phase 1 session adapter).
- Read site: `playbook.render` dispatch, lead-only block, `internal/mcp/server.go`.
- Write site: `ws.lead.prefer_mercenary` dispatch + tool schema, same file.
- Guidance injection: `internal/mcp/playbook_tools.go` `mercenaryGuidanceBlock`
  (unchanged; driven by the resolved boolean).

## Verification Contract

- `go test ./internal/wsconfig/... ./internal/mcp/...` — full output read; only the
  two pre-existing golden-hash failures may remain, no new failures.
- 260618 repro (on→off→on render guidance follows), legacy enable shape, non-lead
  rejection, session-scope resolution, and `config.show` reporting all pass.
- `go build ./...` clean, no new warnings.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` #260619-prefer-mercenary-session-scope-item - [Must] the Phase 2 contract (desired-state get/set, lead-only, availability unchanged).
- `ai-docs/spec/mcp-tools.md` #260619-layered-config-scope-model - [Must] the substrate this rides (resolver precedence, declared default scope, session overlay, config.show scope reporting).
- `ai-docs/spec/mcp-tools.md` #260610-mercenary-delegation-surface - [Must] the render-guidance toggle semantics and lead-only gating the migration must preserve.
- `ai-docs/mental-model/mcp-runtime.md` - [Must] sessionStore (`getOverride`/`setOverride`, `keys/<key>.json`), the layered config locking invariant, `ws.lead.prefer_mercenary` lead-only gate (sole keyed authority), and the add/change-MCP-tool recipe.
- `ai-docs/mental-model/prompt-bundle.md` - [Maybe] `playbook.render` guidance-injection units and the `delegates:true` continuity tip the guidance block sits beside.
- `ai-docs/mental-model/named-agent-runtime.md` - [Maybe] mercenary registration/continuation context (availability is unchanged here).

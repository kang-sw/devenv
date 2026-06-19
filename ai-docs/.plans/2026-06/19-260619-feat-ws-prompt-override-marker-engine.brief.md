# Brief: 260619-feat-ws-prompt-override-marker-engine (Phase 1)

## Intent

Implement the block-marker prompt-override engine: a render-layer pass that lets
a playbook body declare named override-points with A1 block markers, resolves
each `(pointId, harness)` against the layered config (substrate), substitutes the
stored override or falls back to the inline seed default, and strips the marker
lines from the rendered output. This is the reusable override *mechanism*; it
does not yet seed `DelegationSection` into any shipped playbook (Phase 2) and
does not add the `config.prompt.*` setter tool (sibling ticket
`260619-feat-ws-config-prompt-tool-self-doc`).

## Scope Boundary

In scope (Phase 1):
- A1 marker grammar parsing + an override-application render pass, added as a
  sibling to the existing product-mode marker pass in the MCP playbook layer.
- `(pointId, harness)` → scope resolution through the Phase-1 layered config
  resolver (`wsconfig.Resolver`), with fallback to the inline seed default.
- Marker stripping (rendered output never contains marker syntax).
- Empty-seed extension slots (render the override or nothing).
- Tests at the `internal/mcp` render layer (and `internal/wsconfig` if any pure
  helper is added there).

Explicitly OUT of scope:
- Seeding `DelegationSection` in `lead-workflow-manual` or consolidating
  delegation-posture prose (Phase 2). Do NOT add markers to any shipped rsrc
  playbook in this phase — keep the golden-hash surface unchanged.
- The `config.prompt.set` setter MCP tool and the `config.prompt()` self-doc
  listing (sibling ticket). Phase 1 is read/resolve/render only; tests write
  override values directly through the wsconfig resolver / session store, not
  through a setter tool.
- Any change to the critical-path render mechanics (continuation tip, child-key
  credential splice, prefer_mercenary guidance block).

## Caller-Visible Contract

Spec anchor (authored, contract-first 🚧):
`ai-docs/spec/mcp-tools.md` `#260619-prompt-override-marker-engine`. Implement it;
the doc pre-pass strips the 🚧 marker after implementation.

Observable behavior this phase must deliver (verified through rendering):
- A playbook body declaring an override-point with markers:
  ```
  <!-- ws:override:DelegationSection desc="how eagerly the lead delegates" -->
  seed default text
  <!-- ws:/override:DelegationSection -->
  ```
  renders, with NO override stored, as exactly `seed default text` and the marker
  lines removed.
- With an override stored for the rendered harness, the block body is replaced by
  the override text (markers removed).
- With an override stored only for `all` (the `*` bucket) and none for the
  rendered harness, the `all` override applies.
- Resolution order per point: rendered-harness override → `all` override →
  inline seed default.
- An empty-seed block (`<!-- ws:override:X ... -->` immediately followed by the
  close marker) renders the stored override if present, or nothing when none is
  set.

## Contract Instructions

Files / functions:
- `internal/mcp/playbook_tools.go`:
  - Add marker constants/grammar and a new pass function, e.g.
    `applyOverrideMarkers(body, harness string, lookup overrideLookupFn) string`.
    Keep it line-oriented like `selectProductModeBlocks`: the open marker
    `<!-- ws:override:<pointId> desc="..." -->` and close marker
    `<!-- ws:/override:<pointId> -->` each occupy their own line (match on a
    trimmed-line prefix, then parse the `<pointId>` token and the optional
    `desc="..."` attribute). The lines between open and close are the inline
    seed default.
  - Define `type overrideLookupFn func(pointId, harness string) (value string, found bool)`.
  - Resolution inside the pass: `lookup(pointId, renderedHarness)` →
    `lookup(pointId, "all")` → inline seed body. Replace the whole marker block
    (open line … close line) with the resolved text; drop the marker lines
    whether or not an override was found.
  - Wire the pass into `renderPlaybookBody` immediately before the
    `return renderProductModePlaybookBody(body), ...` line (it already has
    `harness` in scope). Add an `overrideLookup overrideLookupFn` parameter to
    `renderPlaybookBody`, and thread it through `printPlaybook` and
    `renderPlaybook`. A `nil` lookup means "no overrides" → every point renders
    its seed (so `printPlaybook`/tests can pass `nil`).
  - The override pass and the product-mode pass operate on disjoint marker sets;
    run the override pass first, then `renderProductModePlaybookBody`.
- `internal/mcp/server.go`:
  - In BOTH the `playbook.print` and `playbook.render` tool dispatch paths, build
    an `overrideLookupFn` from a resolver keyed by the caller's `session_key`,
    reusing the exact pattern already used for `prefer_mercenary` at the render
    dispatch (`adapter := sessionConfigAdapter{s: s.sessions}`;
    `resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)`;
    then `resolver.Get(...)`). The closure does:
    `v, _, _ := resolver.Get("prompt." + pointId + "." + harness)` and returns
    `(v, v != "")`.
  - Pass the closure into `renderPlaybook` / `printPlaybook`. When the dispatch
    has no usable `session_key`, pass `nil`.
- `internal/wsconfig`: no new write surface. If a tiny read helper aids the
  lookup, keep it additive; do not change `Get`'s signature. Confirm
  `Resolver.Get` returns empty (not an error) for an unregistered/unset key —
  override keys are dynamic (`prompt.<pointId>.<harness>`) and are never
  registered in the default-scope registry (registry governs WRITE defaults
  only; READ walks all scopes for any key).

Forbidden:
- No `{{.Var}}` template-variable mechanism for override-points (epic decision).
  The pointId lives only as the marker id; the seed is the inline block body.
- No setter tool, no markers added to shipped playbooks, no temporary/mock
  storage. Tests seed overrides via the real wsconfig resolver/session store.
- Do not rewrite `selectProductModeBlocks` / `renderProductModePlaybookBody`;
  add beside them.

## Integration Test Instructions

- Boundary: `internal/mcp` render-layer tests (new file, e.g.
  `prompt_override_test.go`). Use the existing rsrc-fixture helpers
  (`buildTestRsrcTree`, the implementer-playbook fixture pattern) to ship a test
  playbook carrying override markers, and drive rendering through
  `renderPlaybookBody` with an injected fake `overrideLookupFn` for the unit-level
  cases. Add at least one end-to-end case that stores an override through the
  real resolver/session store and renders via the production dispatch
  (`callToolOnce` on `playbook.render`) to prove the server-side wiring resolves
  overrides — mirroring how `TestPreferMercenaryOnOffRenderGuidanceProductionPath`
  exercises the real dispatch.
- Pass criteria:
  - no-override → seed renders, markers stripped;
  - per-harness override → body replaced for the matching harness only (a
    different harness still gets its seed);
  - `all` override → applies when no harness-specific override exists;
  - empty-seed slot → renders override-or-nothing;
  - production-path case → an override stored via the resolver is honored at
    render time and the marker syntax never appears in output.
- Build clean; full `go test ./internal/wsconfig/... ./internal/mcp/...` output
  read. The two pre-existing `lead-workflow-manual.md` golden-hash failures
  (`TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance`,
  `TestPlaybookPrintGoldenLeadWorkflowManual`) are unrelated; no NEW failures,
  and because Phase 1 adds no markers to shipped playbooks the golden set must
  not change.

## Implementation Strategy Decisions (settled — do not reopen)

- **Storage key convention:** override values are stored as layered-config items
  keyed `prompt.<pointId>.<harness>`, where `<harness>` is `claude`, `codex`, or
  `all` (the `all` bucket is the API's `*`). Phase 1 only READS these keys
  through the resolver; the sibling `config.prompt.set` ticket WRITES them to the
  same convention. The resolver's scope precedence
  (`session > project > global > builtin`) governs which stored value wins; the
  override pass does not pick scope.
- **Injection via a lookup closure**, not by threading a raw resolver into the
  pure pass — keeps the pass unit-testable with a fake lookup and keeps the
  resolver construction at the server dispatch site (consistent with the
  `prefer_mercenary` read path).
- **Resolution order** rendered-harness → `all` → inline seed; marker lines are
  always stripped.
- **Sibling pass, additive**: a new function beside `selectProductModeBlocks`;
  the existing product-mode pass is untouched.
- **Read works for unregistered keys**: override keys are dynamic and never
  registered; the resolver returns empty for unset keys, falling back to the
  seed.

## Rejected Alternatives

- `{{.Var}}` placeholder with a frontmatter default — rejected (fails loud on
  unprovided vars; forces multiline seed into YAML; moves seed text out of the
  body). Epic decision.
- Threading the resolver/session_key directly into the marker pass — rejected in
  favor of an injected lookup closure for testability and dispatch-site cohesion.
- Adding the setter or seeding shipped playbooks now — deferred to the sibling
  ticket and Phase 2 respectively.

## Approach

- Define the marker grammar constants + the parse/apply pass in
  `playbook_tools.go`.
- Thread an `overrideLookupFn` through `renderPlaybookBody` →
  `printPlaybook`/`renderPlaybook`; apply the pass before product-mode selection.
- Build the closure from a session-keyed resolver in the `server.go` print and
  render dispatch paths.
- Add render-layer tests (unit with fake lookup + one production-path case).

## Constraints

- Additive only; the sole new threading is the `overrideLookupFn` parameter.
- English-only code comments.
- impl-playbook Verify: read full test/build output before claiming pass;
  diagnose blame before fixing; resolve introduced warnings.

## Out of scope

- DelegationSection seeding / manual consolidation (Phase 2), the
  `config.prompt.*` tools (sibling), critical-path render mechanics. See Scope
  Boundary.

## Details

- Marker open: `<!-- ws:override:<pointId> desc="<short>" -->` (own line; `desc`
  optional but normally present). Close: `<!-- ws:/override:<pointId> -->` (own
  line). Body between = inline seed default.
- `pointId` is an identifier token (e.g. `DelegationSection`); harness axis is
  `claude | codex | all`.
- Read site: `renderPlaybookBody` (shared by print and render), just before
  `renderProductModePlaybookBody`.
- Closure source: `wsconfig.Resolver.Get("prompt.<pointId>.<harness>")` keyed by
  the caller `session_key` via `sessionConfigAdapter` (same as the
  `prefer_mercenary` read path, `server.go`).

## Verification Contract

- `go test ./internal/wsconfig/... ./internal/mcp/...` — full output read; only
  the two known pre-existing golden-hash failures may remain; no new failures and
  no golden-set change.
- All five Integration Test pass criteria above demonstrated, including the
  production-path case.
- `go build ./...` clean, no new warnings.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` `#260619-prompt-override-marker-engine` - [Must] the contract this phase implements (marker grammar, resolution, stripping, empty-seed, critical-path boundary).
- `ai-docs/spec/mcp-tools.md` `#260619-layered-config-scope-model` - [Must] the resolver/scope substrate the override values resolve through.
- `ai-docs/spec/mcp-tools.md` `#260513-wsflow-agentless-runtime-mode` - [Must] the product-mode marker pass this one sits beside (don't break it).
- `ai-docs/mental-model/prompt-bundle.md` - [Must] render pipeline: `renderPlaybookBody` var layers, `selectProductModeBlocks`/`renderProductModePlaybookBody`, `delegationTip`/`mercenaryGuidanceBlock`, the MCP-layer-owns-namespace-vars boundary.
- `ai-docs/mental-model/mcp-runtime.md` - [Maybe] layered config resolver + `sessionConfigAdapter`, the `ws.lead.prefer_mercenary` render read path this reuses.

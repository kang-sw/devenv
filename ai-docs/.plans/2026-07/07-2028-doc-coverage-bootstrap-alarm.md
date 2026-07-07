# Plan: 260707-feat-doc-coverage-live-bootstrap-alarm — Phase 1: Live doc-coverage check and mute config item

## Relevant Ticket Contract
- Live, stateless check at session-bootstrap time (`ws/ferrule` / `ws/workflow_manual`) only: whether `ai-docs/spec/` and `ai-docs/mental-model/` each contain at least one `.md` file with a YAML frontmatter block. No stored/settable coverage flag.
- One new combined `wsconfig.Item*` mute entry (both doc areas share one on/off knob, not one per area), builtin default `on`, exposed through `config.tuning`/`ws:lead-tune`, following `260703`'s `ItemBootstrapAlarm` shape exactly.
- No new generic config-setter tool — extend the existing named-item registry, not a parameterized `config.set_flag`.
- Warning text embeds the mute instruction (name of the setter tool) directly, no separate doc lookup.
- Injection point matches `260703`: `ferrule` (`handleLeadLogin`) and `workflow_manual` FRESH-with-root + CONTINUE branches only. `workflow_manual` FRESH-without-root never checks (no established root yet, matching the bootstrap-staleness precedent).
- Explicit reuse instruction: survey and reuse the warning-delivery-channel plumbing `260703` built in `agents-plugin-tool/internal/mcp/workflow_manual.go` (and its `bootstrap_alarm.go` sibling), not duplicate it.
- Verification (from ticket Phase 1): a test confirming the warning fires when a doc area has no frontmatter-bearing `.md` file, is silent when at least one exists, is suppressed when the mute item is off, and that `config.tuning`/`ws:lead-tune` lists and can set the new item.

## Out of Scope
- `260707-feat-forge-autonomy-bootstrap-chaining` (same-session chaining) — different ticket, not touched here.
- Any per-`ws/project_tree`-call variant of this warning.
- Changing `lead-forge-spec`/`lead-forge-mental-model` themselves.
- Spec-authoring style/format questions beyond adding the new section to `ai-docs/spec/mcp-tools.md` (Spec Impact says this needs addressing; follow the `#260703-bootstrap-staleness-warning` section as the template).

## Codebase Findings

### Sibling `260703` plumbing to reuse (do not duplicate)
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go` — the exact shape to mirror: a pure `xWarning(...) string` computer (empty string = silent) plus a tiny `injectXWarning(body, warning) string` no-op-when-empty prepender (`bootstrap_alarm.go#L62-L96`). `bootstrapStalenessWarning` reads the config item first (`resolver.Get(sessionKey, wsconfig.ItemBootstrapAlarm)`, `off` -> silent) before doing any filesystem work — the new `docCoverageWarning` should check its mute item first the same way.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L258-L264` (FRESH-with-root) and `#L280-L287` (CONTINUE) — the two call sites that resolve `skillsRoot`/build a `wsconfig.Resolver` and call `bootstrapStalenessWarning` then `injectBootstrapStalenessWarning`. Add the doc-coverage check + injection at the same two sites, using the project `root` (already in scope as `canonical`/`rec.Root`) — no `skillsRoot` needed for this check since doc coverage is purely a project-local `ai-docs/` scan, not a package-template comparison.
- `agents-plugin-tool/internal/mcp/server.go#L1475-L1488` (`handleLeadLogin`, the `ferrule` handler) — same pattern a third time: builds a resolver, computes the warning, appends it to both the JSON `result` map (`result["bootstrap_alarm"] = warning`) and the text response. Add `result["doc_coverage_alarm"] = warning` (or similar key) alongside it.
- `injectBootstrapStalenessWarning` (`bootstrap_alarm.go#L88-L96`) is already generic (prepend-if-nonempty); it can be called a second time for the doc-coverage warning at each site rather than rewritten — call order determines which banner ends up on top when both fire simultaneously (pick one order, e.g. bootstrap-staleness first then doc-coverage prepended after so it appears above; not contract-critical).

### Config-item registry pattern to follow exactly
- `agents-plugin-tool/internal/wsconfig/scope.go#L70-L82` — `ItemBootstrapAlarm` constant + doc comment + `RegisterGlobalOnly(ItemBootstrapAlarm)` in `init()`. Add a new constant (e.g. `ItemDocCoverageAlarm = "doc_coverage_alarm"`) and `RegisterGlobalOnly(...)` call the same way — this is a cross-project warning-noise preference, not a per-project opt-in, matching `260703`'s own global-only rationale.
- `agents-plugin-tool/internal/mcp/server.go#L320-L327` (`builtinConfigDefaults`) — add `wsconfig.ItemDocCoverageAlarm: "on"`.
- `agents-plugin-tool/internal/mcp/server.go#L605-L641` — the full `config.bootstrap_alarm` tool-call case (lead-session-key gate via `requireLeadSessionKey`, `reset`/`value` mutual-exclusion, `Unset`/`Get`/`Set`). Copy this verbatim for a new `config.doc_coverage_alarm` case.
- `agents-plugin-tool/internal/mcp/server.go#L70-L77` (`workflowPreferenceWriterTool`) — add the new tool name to this switch so it's lead-only gated like `config.bootstrap_alarm`.
- `agents-plugin-tool/internal/mcp/server.go#L3196-L3205` — the `config.bootstrap_alarm` MCP tool-schema registration entry (name/description/inputSchema). Add an equivalent entry for `config.doc_coverage_alarm`.
- `agents-plugin-tool/internal/mcp/server.go#L1788-L1799` — the `bootstrap_alarm` `tuningKnob` entry appended to `buildTuningCatalog`'s catalog (placed before the `if noAgentMode` cut at `#L1801`, so it stays visible in wsflow product mode same as `bootstrap_alarm`). Add an equivalent `doc_coverage_alarm` knob entry in the same block, using `currentWorkflowPreference(resolver, wsconfig.ItemDocCoverageAlarm)`.

### Frontmatter/doc-scan primitives to reuse (avoid re-parsing YAML by hand)
- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8-L67` — unexported `frontmatter(path string) map[string]any` already parses the `---`-delimited YAML block and returns `nil` when absent/malformed. This is the exact "has a frontmatter block" primitive the ticket's convention section (`ai-docs/tickets/ready/260707-...md:34-39`) is built on.
- **Caveat**: `wsdoc.SpecsList`/`wsdoc.MentalModelsList` (`spec_discovery.go#L155-L185`, `mental_model_discovery.go#L128-L158`) walk every `.md` file under `ai-docs/spec`/`ai-docs/mental-model` but do **not** filter by frontmatter presence — `readSpec`/`readMentalModel` call `frontmatter()` only to populate fields, tolerating its absence. So these list functions cannot be reused as-is to answer "does at least one file have frontmatter"; a new small exported function is needed in `wsdoc` (e.g. `wsdoc.DocAreaHasFrontmatterFile(root, "ai-docs/spec") bool` or two convenience wrappers) that walks the dir the same way `scanSpecs`/`scanMentalModels` do (`filepath.WalkDir`, skip dirs and non-`.md`) and returns true on the first `frontmatter(path) != nil` hit. A missing directory should return `false`, not an error (fresh projects legitimately lack these dirs pre-`lead-forge-spec`/`lead-forge-mental-model`).
- Adding this in `wsdoc` (not duplicating a hand-rolled YAML-fence check in `agents-plugin-tool/internal/mcp`) keeps the frontmatter parser single-sourced and is the "reuse existing mechanism" the ticket's Background section implies (`260703` reused `wsconfig`'s registry the same way).

### Test-harness helpers already available for the new tests
- `agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go` — `useLeadProfile`, `mustWrite`, `initGit`, `callLogin`, `toolText`, `callToolWithKey`, `callToolOnce`, `parseLoginResponse` are already-shared test helpers (defined elsewhere in the `mcp` test package) used by the sibling ticket's tests; reuse them directly for the new doc-coverage tests instead of re-deriving fixtures.

### Spec precedent to mirror
- `ai-docs/spec/mcp-tools.md#L423-L454` (`### Bootstrap Staleness Warning {#260703-bootstrap-staleness-warning}`) and `#L492-L497` (`config.bootstrap_alarm` config-tools paragraph) are the exact spec-section templates to copy for the new doc-coverage warning and its `config.doc_coverage_alarm` tool.

### Runtime-contract golden-list risk
- `agents-plugin-tool/cmd/ws-mcp/main_test.go` (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` and `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`) diff the live tool/command surface against `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` respectively. `260703` added a `"config.bootstrap_alarm": ">=0.33.2-dev <0.34.0"` line to both files (`agents-plugin/runtime.json:33`, `agents-plugin-wsflow/runtime.json:34`). A new `config.doc_coverage_alarm` tool needs an equivalent line in both files (version range depends on the next planned version bump) or these golden tests will fail.

## Implementation Plan
1. `agents-plugin-tool/internal/wsdoc/` — add an exported helper (new small file, e.g. `doc_coverage.go`) that answers "does `<root>/ai-docs/spec` (resp. `ai-docs/mental-model`) contain at least one `.md` file with a parsed frontmatter block", reusing the unexported `frontmatter()` parser and the same `filepath.WalkDir` shape as `scanSpecs`/`scanMentalModels`. Missing directory -> `false`, no error.
2. `agents-plugin-tool/internal/wsconfig/scope.go` — add `ItemDocCoverageAlarm = "doc_coverage_alarm"` constant (doc comment mirroring `ItemBootstrapAlarm`) and `RegisterGlobalOnly(ItemDocCoverageAlarm)` in `init()`.
3. `agents-plugin-tool/internal/mcp/` — add a new file (e.g. `doc_coverage_alarm.go`) mirroring `bootstrap_alarm.go`'s shape: a `docCoverageWarning(root string, resolver *wsconfig.Resolver, sessionKey string) string` that (a) returns `""` when `ItemDocCoverageAlarm` resolves to `off`; (b) uses the Phase-1 `wsdoc` helper to check both `ai-docs/spec` and `ai-docs/mental-model`; (c) returns `""` when both are covered; (d) otherwise returns a one-line warning naming which area(s) are missing and pointing to `config.doc_coverage_alarm(value: "off")` to silence permanently. Add an `injectDocCoverageWarning` wrapper (can just delegate to the existing generic `injectBootstrapStalenessWarning` prepend helper rather than a new copy — confirm during implementation whether renaming it to a neutral shared name is cleaner than a second near-identical function).
4. `agents-plugin-tool/internal/mcp/server.go`:
   - `builtinConfigDefaults()` (`#L320-L327`): add `wsconfig.ItemDocCoverageAlarm: "on"`.
   - `workflowPreferenceWriterTool` (`#L70-L77`): add `"config.doc_coverage_alarm"` to the switch.
   - Add a `case "config.doc_coverage_alarm":` tool-call handler copying the `config.bootstrap_alarm` case (`#L605-L641`) structure (lead-session-key gate, `reset`/`value` mutual exclusion, `Unset`/`Get`/`Set` against `ItemDocCoverageAlarm`).
   - Add the `config.doc_coverage_alarm` MCP tool-schema registration entry, mirroring `#L3196-L3205`.
   - Add a `doc_coverage_alarm` `tuningKnob` entry in `buildTuningCatalog` next to the `bootstrap_alarm` entry (`#L1788-L1799`), placed before the `noAgentMode` cut (`#L1801`).
   - `handleLeadLogin` (`#L1475-L1488`): after the existing bootstrap-staleness block, compute `docCoverageWarning(canonical, resolver, "")` and append it to both `result` (new key, e.g. `"doc_coverage_alarm"`) and `text`.
5. `agents-plugin-tool/internal/mcp/workflow_manual.go`:
   - FRESH-with-root branch (`#L258-L264`): after the existing bootstrap-staleness computation/injection, compute and inject the doc-coverage warning against `canonical`.
   - CONTINUE branch (`#L280-L287`): same, against `rec.Root`.
   - Doc-coverage check does not depend on `skillsRoot`/`wsrsrc.ResolveSkillsRoot()`, so it does not need to be nested inside that `if skillsRoot, srErr := ...; srErr == nil` block — build its own resolver (or reuse the same `warningResolver` if convenient) unconditionally.
6. `ai-docs/spec/mcp-tools.md`: add a new subsection mirroring `#260703-bootstrap-staleness-warning` (`#L423-L454`) documenting the doc-coverage warning's firing conditions, silent cases, and the `config.doc_coverage_alarm` tool paragraph mirroring `#L492-L497`.
7. `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`: add a `"config.doc_coverage_alarm"` entry mirroring the `"config.bootstrap_alarm"` line, matching whatever version range convention `260703` used (`>=0.33.2-dev <0.34.0`-shaped, adjusted to the version this ticket lands in).

## Verification Plan
- New Go test file (e.g. `agents-plugin-tool/internal/mcp/doc_coverage_alarm_test.go`), mirroring `bootstrap_alarm_test.go`'s structure, covering the four ticket-specified assertions:
  1. Warning fires at `ferrule`/`workflow_manual` when a doc area (spec or mental-model) has no frontmatter-bearing `.md` file.
  2. Silent when both areas have at least one frontmatter-bearing `.md` file.
  3. Suppressed when `config.doc_coverage_alarm` is `off`.
  4. `config.tuning` lists the `doc_coverage_alarm` knob, and `config.doc_coverage_alarm` can set/reset it (mirror `TestBootstrapAlarmTuningKnob`).
- Add a `wsdoc`-level unit test for the new coverage helper (missing dir, dir with only non-frontmatter `.md`, dir with a frontmatter file).
- Run `go test ./...`, `go vet ./...`, `go build ./...` from `agents-plugin-tool/` (matches `260703`'s verification bar) — this also re-runs the runtime.json golden-list tests, which will catch a missed `runtime.json` update.
- Run `ws/spec_index.verify()` (or the bundled equivalent) after the spec edit.

## Escalations
- None.

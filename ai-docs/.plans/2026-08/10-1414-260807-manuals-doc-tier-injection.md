# Plan: 260807-feat-manuals-doc-tier — Phase 1: manuals tier + ambient # Manuals injection

## Relevant Ticket Contract

- Recognize `ai-docs/manuals/*.md` as a first-tier doc tier with one-line
  `summary:` frontmatter, reusing the tier-agnostic frontmatter parser (do not
  fork a manuals-only parser).
- `computeManuals(root) string` walks the directory, reads each `summary:`, and
  renders a `# Manuals` block (path + summary per line), modeled on
  `scopeAnnouncement`.
- Inject the block into `workflow_manual` output on the fresh-with-root AND
  continue branches, alongside the existing scope announcement.
- Discovery parity: give manuals a list/find surface equivalent to
  `specs.*`/`mental_models.*` so a body is reachable by tool, not only by the
  injected path.
- **Ambient injection needs no applicability predicate** — every manual's path
  + summary is injected; no rule-based selection.
- **Schema is one line: `summary:`.** No `sources:`/`applies-when:` field.
- **Shared frontmatter substrate, not shared injection logic** with the 260716
  cluster (`260716-feat-mental-model-openup-injection`) — reuse the parser
  only; this tier's injector is a flat "list all manuals" emit, no telemetry.
- Verification (from ticket): (a) a manual with `summary:` appears in the next
  `workflow_manual` call's `# Manuals` block; (b) a manual with NO `summary:`
  is reported (not silently dropped); (c) the discovery surface returns the
  new manual.
- Spec Impact requires updating `spec/documentation-system.md` (new
  `ai-docs/manuals/` tier + schema + ref-vs-manuals boundary) and
  `spec/mcp-tools.md` (`workflow_manual` gains an injected `# Manuals` block).

## Out of Scope

- Phase 2 (migrating `ref/`/`_index.md` procedure content into `manuals/`,
  updating `lead-bootstrap`'s ref-handling step) — explicitly a separate phase.
- Any applicability/relevance predicate for manuals (that is the 260716
  cluster's owned selective-injection logic).
- Nested subdirectories under `ai-docs/manuals/` — the ticket names
  `ai-docs/manuals/*.md` (flat glob); do not build recursive-walk semantics
  the ticket didn't ask for. If the executor judges recursive walk trivially
  safer/simpler than flat glob (mirroring `scanMentalModels`'s `WalkDir`),
  either is acceptable, but flat is the literal contract.
- `manuals.status` tool (mental_models has one keyed on `domain`; manuals has
  no `domain` field in its one-line schema, so there is no equivalent
  selector — `manuals.list` + `manuals.find` already satisfy "reachable by
  tool, not only by the injected path").

## Codebase Findings

- `agents-plugin-tool/internal/mcp/scope_announcement.go#L19-L37` —
  `scopeAnnouncement(root) string` is the exact model named by the ticket: it
  returns `""` on any inactive/error condition (silent-by-design, never
  blocks rendering), builds a `> **...**` block with `strings.Builder`, and is
  invoked from `workflow_manual.go`. `computeManuals` should follow the same
  shape (return `""` when `ai-docs/manuals/` doesn't exist yet — this repo has
  no manuals until Phase 2 migrates content, so this is the common case in
  Phase 1, not an edge case).
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L283` — FRESH-with-root
  branch's exact injection point: `body = injectBootstrapStalenessWarning(body,
  scopeAnnouncement(canonical))`. Add a sibling call passing
  `computeManuals(canonical)` here.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L313` — CONTINUE
  branch's exact injection point: `body = injectBootstrapStalenessWarning(body,
  scopeAnnouncement(rec.Root))`. Add a sibling call passing
  `computeManuals(rec.Root)` here.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L91-L98` —
  `injectBootstrapStalenessWarning(body, warning string) string` is already a
  generic "prepend warning + blank-line separator if non-empty" combinator,
  reused verbatim by `scopeAnnouncement` and (via a one-line delegating
  wrapper `injectDocCoverageWarning`, `doc_coverage_alarm.go#L42-L45`) by the
  doc-coverage warning. Reuse it directly for the Manuals block too — no new
  inject helper is needed, though a thin delegating wrapper
  (`injectManualsBlock`) mirroring `injectDocCoverageWarning`'s naming
  convention is an acceptable style choice.
- **Injection ordering (evidence, not a strict requirement):** each
  `injectBootstrapStalenessWarning` call prepends to the *current* body, so
  the call order in `workflow_manual.go` is reverse of final top-to-bottom
  order. Currently: staleness warning injected first, then doc-coverage, then
  scope announcement last — so scope announcement renders topmost, then
  doc-coverage, then staleness, then the manual body. Placing the
  `computeManuals` call as the last injection (after `scopeAnnouncement`) puts
  `# Manuals` at the very top; placing it before puts it just under scope
  announcement. Either is acceptable; pick one and keep both branches
  consistent.
- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8-L14` — `frontmatter(path)
  map[string]any` is the tier-agnostic, unexported, in-package frontmatter
  parser named by the ticket as "already exists and must be reused, not
  forked." A new manuals scanner must call this directly (it lives in package
  `wsdoc`, so the new manuals code must also live in package `wsdoc` to call
  it — matching where `mental_models.go`/`mental_model_discovery.go` already
  live).
- `agents-plugin-tool/internal/wsdoc/mental_model_discovery.go#L128-L183` —
  `scanMentalModels`/`readMentalModel` is the closest existing pattern for a
  doc-tier scan: `filepath.WalkDir` over the tier root, skip non-`.md`,
  extract fields via `frontmatter(path)`, build one `Info` struct per file.
  Model a new `internal/wsdoc/manuals.go` on this shape:
  `type ManualInfo struct { Path string; Summary string }` +
  `func ManualsList(root string) ([]ManualInfo, error)`. Unlike
  `scanMentalModels` (which errors when the tier directory is missing —
  appropriate there since mental-model docs already exist in this repo),
  `ManualsList` should treat a missing `ai-docs/manuals/` directory as "zero
  manuals" (`nil, nil`), not an error — Phase 1 ships before any manual
  exists, and `computeManuals`/`manuals.list` must not surface a scary error
  every session pre-Phase-2.
- **Missing-summary reporting (verification requirement (b)):** neither
  `mental_models.go#L61-L66` (silently omits the `# comment` when
  `description == ""`) nor `MentalModelInfo` distinguishes "no description"
  from "empty description" is a safe model to copy verbatim — the ticket
  requires a manual with no `summary:` to be *reported*, not dropped.
  `computeManuals`'s per-line rendering must emit an explicit marker for
  `Summary == ""`, e.g. `- <path> — (no summary: add a \`summary:\`
  frontmatter line)`, rather than a bare path or a silently blank comment.
  `manuals.list`/`manuals.find` formatting should do the same (mirror
  `displayOrDash`, `agents-plugin-tool/internal/mcp/server.go#L3188`, for the
  pattern of an explicit placeholder over silent omission).
- `agents-plugin-tool/internal/wsdoc/mental_model_discovery.go#L41-L94`
  (`MentalModelsFind`) and `internal/wsdoc/query_match.go#L31`
  (`matchDocumentQuery`) — the query-matching helper is tier-agnostic
  (`docQueryCandidate{Path, Fields, BodyText}`), already reused across specs
  and mental-models discovery; reuse it for `ManualsFind(root, query string)
  ([]ManualInfo, error)` (fields: `[]string{path, summary}`) rather than
  writing new substring matching.
- `agents-plugin-tool/internal/mcp/server.go#L1183-L1223` — dispatch cases
  `mental_models.list`/`.find`/`.status` are the pattern for two new dispatch
  cases, `manuals.list` and `manuals.find`, calling
  `wsdoc.ManualsList`/`wsdoc.ManualsFind` and a new `formatManuals` helper
  (model: `formatMentalModels`, `server.go#L3104-L3126`).
- `agents-plugin-tool/internal/mcp/server.go#L4194-L4224` — tool schema block
  for `mental_models.list`/`.find`/`.status` is the pattern for the two new
  `manuals.list`/`manuals.find` schemas (properties: `query`, `format` only —
  no `domain`/`spec_stem`, since the one-line schema carries neither).
- `agents-plugin-tool/internal/mcp/server.go#L4610` — the tool-name list
  handed to `LeadToolNames`/`runtime.capabilities` includes
  `"mental_models.list", "mental_models.find", "mental_models.status",
  "references.trace"`; add `"manuals.list", "manuals.find"` to the equivalent
  list (read the surrounding block before editing — this line is inside a
  larger slice literal, not a standalone statement).
- `agents-plugin-tool/cmd/ws-mcp/main.go#L56,L229-L230,L800-L844` — CLI mirror
  pattern (`mental-models find|status` subcommands, `printTextOrFatal`,
  `mcp.FormatMentalModelFind`/`mcp.FormatMentalModels`). Per mental-model
  domain rule {#260519-workflow-command-readable-output-defaults} /
  `ai-docs/mental-model/mcp-runtime.md`'s "Updating specs.find or
  mental_models.find MCP output without the CLI mirror" common mistake, add a
  `manuals` CLI subcommand (`manuals list|find`) alongside the MCP tools in
  the same change, not as a follow-up.
- `agents-plugin/runtime.json#L43-L48` and
  `agents-plugin-wsflow/runtime.json` (sibling file, same key shape) — new
  tool capability-range entries `"manuals.list": ">=X.Y.Z-dev <X.Y.(Z+1)>"`,
  `"manuals.find": "..."` must be added to both files in the same version
  bracket as the other doc-discovery tools (mirror the existing
  `mental_models.*` entries' bracket exactly; do not hand-pick a version — per
  `AGENTS.md`, `agents-plugin-tool/scripts/bump-ws-version.sh` is the sole
  version-bump surface, run at dev-merge time, not authored by hand here).
- `ai-docs/spec/mcp-tools.md#L1351-L1358` (`## Mental-Model Discovery Tools
  {#260505-mental-model-discovery-tools}`) is the model section for a new
  `## Manuals Discovery Tools {#<new-anchor>}` section documenting
  `manuals.list`/`manuals.find`.
- `ai-docs/spec/mcp-tools.md#L523-L581` (`### Bootstrap Staleness Warning
  {#260703-bootstrap-staleness-warning}` and `### Doc Coverage Warning
  {#260707-doc-coverage-warning}`, both under `## Session State Tools
  {#260625-session-state-tools}`, near `### Workflow Manual Entry And
  Restoration {#260626-workflow-manual-restoration-entry}` at line 416) — the
  model subsection shape for a new `### Manuals Ambient Injection
  {#<new-anchor>}` documenting the `# Manuals` block, its silent-when-empty
  behavior, and the missing-summary report line.
- `ai-docs/spec/documentation-system.md#L160-L184` (`## Mental-Model Document
  System {#260505-mental-model-document-system}`) — the model section shape
  for a new `## Manuals Document System {#<new-anchor>}` documenting the
  `ai-docs/manuals/` tier, its `summary:` schema, and the `manuals` vs `ref`
  injection-boundary rule (per-file decision at migration time, not a schema
  field — Phase 2 executes the boundary, but the rule itself belongs in the
  spec now since it governs what Phase 1's tier accepts).
- `ai-docs/mental-model/documentation-system.md` and
  `ai-docs/mental-model/mcp-runtime.md` — both carry an `## Extension Points &
  Change Recipes` entry ("Add doc discovery tools: update MCP dispatch, tool
  schema, parameter validation, tests, and docs") that this change satisfies;
  no mental-model edit is strictly required by that recipe itself, but the new
  manuals tier's existence (a fourth doc tier alongside spec/mental-model/
  ticket) is a fact worth a one-line addition to
  `documentation-system.md`'s domain rules if the doc pass has budget — not
  required for Phase 1 code correctness, flagged for the executor's judgment.

## Implementation Plan

1. `agents-plugin-tool/internal/wsdoc/manuals.go` (new file, package `wsdoc`):
   define `ManualInfo{Path, Summary string}`; `ManualsList(root string)
   ([]ManualInfo, error)` — walk `ai-docs/manuals/` (flat `*.md` glob per
   ticket wording), return `nil, nil` when the directory doesn't exist, parse
   each file via the existing unexported `frontmatter(path)` helper (do not
   fork a new parser), extract `fm["summary"].(string)`, sort by `Path`.
   `ManualsFind(root, query string) ([]ManualInfo, error)` — call
   `ManualsList`, then filter/score via the existing `matchDocumentQuery`
   helper (fields: path + summary) when `query != ""`, else return the full
   list.
2. `agents-plugin-tool/internal/mcp/manuals_announcement.go` (new file,
   package `mcp`): `computeManuals(root string) string` modeled on
   `scopeAnnouncement` — call `wsdoc.ManualsList(root)`; return `""` on error
   or empty result; otherwise render a `# Manuals\n` header followed by one
   line per manual (`- <path> — <summary>`, or the explicit no-summary marker
   when `Summary == ""`).
3. `agents-plugin-tool/internal/mcp/workflow_manual.go`: add
   `body = injectBootstrapStalenessWarning(body, computeManuals(canonical))`
   in the FRESH-with-root branch (near `#L283`) and
   `body = injectBootstrapStalenessWarning(body, computeManuals(rec.Root))` in
   the CONTINUE branch (near `#L313`), placed consistently relative to the
   existing `scopeAnnouncement` call in both branches.
4. `agents-plugin-tool/internal/mcp/server.go`: add `formatManuals(manuals
   []wsdoc.ManualInfo) string` (model: `formatMentalModels`,
   `#L3104-L3126`); add dispatch cases `manuals.list` and `manuals.find`
   (model: `mental_models.list`/`.find`, `#L1183-L1208` — `manuals.find` takes
   only `query`+`format`, no `spec_stem`/`domain`); add the two tool schema
   entries (model: `#L4194-L4213`); add `"manuals.list", "manuals.find"` to
   the `LeadToolNames`-feeding tool-name list near `#L4610`.
5. `agents-plugin-tool/cmd/ws-mcp/main.go`: add a `manuals` CLI subcommand
   group (`manuals list`, `manuals find`) mirroring the `mental-models`
   subcommand block (`#L800-L844`), wired into the top-level `case` dispatch
   and usage strings (`#L56`, `#L72`, `#L75`).
6. `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`: add
   `"manuals.list"` / `"manuals.find"` capability-range entries mirroring the
   existing `mental_models.*` entries' version bracket exactly (do not
   hand-pick a version number outside that bracket).
7. Doc pass (per ticket's `## Spec Impact`):
   - `ai-docs/spec/documentation-system.md`: add a `## Manuals Document
     System {#<new-anchor>}` section (model: `## Mental-Model Document System
     {#260505-mental-model-document-system}`, `#L160-L184`) describing the
     `ai-docs/manuals/` tier, its `summary:`-only schema, and the manuals-vs-
     ref boundary rule (decided per-file at migration time).
   - `ai-docs/spec/mcp-tools.md`: add a `### Manuals Ambient Injection
     {#<new-anchor>}` subsection near the Bootstrap Staleness / Doc Coverage
     Warning subsections (`#L523-L581`) describing the injected `# Manuals`
     block (fresh-with-root + continue branches, silent when
     `ai-docs/manuals/` is absent or empty, explicit no-summary marker); add a
     `## Manuals Discovery Tools {#<new-anchor>}` section (model: `##
     Mental-Model Discovery Tools {#260505-mental-model-discovery-tools}`,
     `#L1351-L1358`) documenting `manuals.list`/`manuals.find`.
8. New unit tests (add alongside the new/changed files, following existing
   sibling test files' conventions):
   - `agents-plugin-tool/internal/wsdoc/manuals_test.go`: empty/missing
     directory → `nil, nil`; a manual with `summary:` is listed; a manual
     with no `summary:` is listed with `Summary == ""` (not dropped);
     `ManualsFind` query matching.
   - `agents-plugin-tool/internal/mcp/manuals_announcement_test.go`:
     `computeManuals` renders `""` when no manuals exist; renders the block
     with path+summary when a manual exists; renders the explicit no-summary
     marker for a manual missing `summary:`.
   - Extend `agents-plugin-tool/internal/mcp/server_test.go` (or add a
     workflow_manual-focused test) to assert the FRESH-with-root and CONTINUE
     `workflow_manual` responses contain the `# Manuals` block when a fixture
     manual exists.
   - `agents-plugin-tool/cmd/ws-mcp/main_test.go`: extend the CLI table-driven
     tests with `manuals list`/`manuals find` cases (model: the
     `mental-models find` cases at `#L411-L466`).

## Verification Plan

- `cd agents-plugin-tool && go build ./...` and `go test ./...` (covers the
  new `wsdoc`, `mcp`, and `cmd/ws-mcp` packages together; the module lives at
  `agents-plugin-tool/go.mod` per the `internal/...` import paths observed).
- Manual/functional check per the ticket's stated verification: create a
  fixture `ai-docs/manuals/test.md` with a `summary:` line, call
  `workflow_manual` (fresh-with-root and continue) and confirm the `# Manuals`
  block lists it; add a second fixture manual with no `summary:` and confirm
  it still appears with the explicit no-summary marker; call
  `manuals.list`/`manuals.find` and confirm both fixtures are returned.
- `go vet ./...` in `agents-plugin-tool/` as a cheap regression check on the
  new files.

## Escalations

- None.

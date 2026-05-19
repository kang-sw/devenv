# Implementation Plan: 260513-feat-tolerant-doc-find-queries Phase 1

## Scope

Implement only Phase 1 from `ai-docs/.plans/2026-05/19-260513-tolerant-doc-find-queries.brief.md`: tolerant broad-query candidate discovery for `specs.find` and `mental_models.find`, convention aliases for `convention.read`, grouped evidence text output, and JSON match evidence. Do not change ticket discovery, reference tracing, named-agent behavior, dashboard surfaces, or source/package layout outside `agents-plugin-tool` unless tests expose generated metadata drift.

## Source Files and Boundaries

### Query matching and evidence model

- Add a shared documentation-query helper in `agents-plugin-tool/internal/wsdoc/` (prefer a new file such as `query_match.go`) rather than duplicating logic in spec and mental-model discovery.
- Keep the helper private to `wsdoc` and responsible for:
  - normalizing query/document text across case, punctuation, hyphens, underscores, and token boundaries;
  - tokenizing broad human queries into meaningful terms;
  - scoring a document from metadata plus body text;
  - extracting line evidence with line number, matched terms, and compact snippet text;
  - sorting candidates by score descending, then path;
  - selecting/truncating evidence while preserving final display order by document and line.
- Preserve `containsFold`/`snippets` behavior for ticket discovery in `agents-plugin-tool/internal/wsdoc/tickets.go`; do not route tickets through the new tolerant helper in this phase.

### Spec discovery

- Update `agents-plugin-tool/internal/wsdoc/spec_discovery.go`:
  - extend `SpecInfo` with JSON fields such as `match_score,omitempty` and `matches,omitempty` while keeping existing document metadata fields (`path`, `filename`, `title`, `summary`, anchors, ticket refs, marker contexts);
  - consider keeping `MatchingSnippets` populated for compatibility, but make new formatter/tests depend on structured match evidence;
  - leave exact `spec_stem` and `ticket_stem` validation/filtering unchanged;
  - when `Query` is non-empty, run tolerant matching across path, filename, title, summary, anchor headings/stems, ticket refs/marker context if useful, and body text;
  - apply a minimum threshold so one weak token does not return noisy candidates;
  - sort broad-query results by match score descending, then path.

### Mental-model discovery

- Update `agents-plugin-tool/internal/wsdoc/mental_model_discovery.go`:
  - mirror the spec result shape with JSON `match_score,omitempty` and `matches,omitempty` fields on `MentalModelInfo`;
  - preserve exact `domain` filtering (`EqualFold`) and exact `spec_stem` validation/filtering;
  - when `Query` is non-empty, run the same tolerant helper across path, domain, description, sources, spec refs, and body text;
  - preserve status/list behavior and existing `AncestorHints` / `IndexHints` metadata.

### Convention aliases and diagnostics

- Update `agents-plugin-tool/internal/wsdoc/conventions.go`:
  - add a small alias/canonical-name table for at least `spec -> spec-conventions`, `ticket -> ticket-conventions`, and `mental-model -> mental-model-conventions` (optionally include obvious plural/filename variants if low-risk);
  - resolve aliases before appending `.md` and before reading the embedded FS;
  - keep path traversal rejection before lookup;
  - on missing convention, return an error listing accepted canonical names and common aliases.

### MCP/CLI text formatting

- Update `agents-plugin-tool/internal/mcp/server.go` formatting helpers or add adjacent helpers:
  - keep `formatSpecs` and `formatMentalModels` status/list-style output for exact selector calls and broad lists;
  - add query-aware formatting for `specs.find` and `mental_models.find` when a broad query was supplied, e.g. `formatSpecFind(query, result)` and `formatMentalModelFind(query, result)`;
  - output summary lines like `N candidate specs for query="..."` / `N candidate mental models for query="..."`;
  - render document lines as `<path>\tscore=<score>\thits=<count>`;
  - render evidence lines as `  <line>: <snippet>`;
  - omit `matched:` lines from default text output;
  - include a subset/truncation note in the summary when selected evidence is truncated.
- Update `agents-plugin-tool/internal/mcp/format.go` exported wrappers if CLI code needs query-aware formatter wrappers.
- Update `agents-plugin-tool/cmd/ws-mcp/main.go` so `specs find --query ...` and `mental-models find --query ...` use the same query-aware text format as MCP, while `--format json` prints the full structured results.

## Tests to Add or Update

### `wsdoc` package tests

- `agents-plugin-tool/internal/wsdoc/spec_discovery_test.go`
  - Add a test where `SpecsFind(Query: "wsflow installer marketplace release packaging")` returns multiple docs even though the full query string is not contiguous.
  - Assert result order follows score descending then path.
  - Assert JSON-facing fields include non-zero `MatchScore` and line-level matches with `Line`, `MatchedTerms`, and `Snippet`.
  - Keep/update existing exact selector test to prove invalid `spec_stem` / `ticket_stem` and exact filters are unchanged.
- `agents-plugin-tool/internal/wsdoc/mental_model_discovery_test.go`
  - Add the analogous tolerant multi-term query test for mental models.
  - Assert exact `domain` and `spec_stem` filtering still narrows results exactly.
- `agents-plugin-tool/internal/wsdoc/project_tree_test.go` or a new `conventions_test.go`
  - Add alias tests for `ReadConvention("spec")`, `ReadConvention("ticket")`, and `ReadConvention("mental-model")`.
  - Add a missing-name test that checks the error mentions accepted canonical convention names and aliases.
  - Preserve path traversal rejection.

### MCP/server formatter tests

- `agents-plugin-tool/internal/mcp/server_test.go`
  - Add or update stdio tool-call coverage for broad `specs.find` and `mental_models.find` query-only calls.
  - Assert text contains the summary line, tab-separated document lines with score/hits, and line evidence.
  - Assert text does not contain a separate `matched:` line.
  - Add JSON-format MCP calls (or direct formatter/response checks if easier) proving JSON keeps document metadata plus `matches` evidence fields.
  - Keep existing exact-selector assertions for `matches_spec_stem`, `matches_ticket_ref`, and `matches_domain` by ensuring those calls do not switch to grep-style output unless a broad query is present.

### CLI mirror tests

- `agents-plugin-tool/cmd/ws-mcp/main_test.go`
  - Extend `TestDocumentationCLICommandsDefaultToTextAndKeepJSONFormat` or add a focused test for:
    - `ws-mcp specs find --root <tmp> --query "..."` default text grouped evidence;
    - `ws-mcp specs find --root <tmp> --query "..." --format json` structured evidence;
    - `ws-mcp mental-models find --root <tmp> --query "..."` equivalent grouped text;
    - JSON remains machine-readable and not the default text format.

## Verification Commands

Run from the repository root unless noted:

```sh
cd agents-plugin-tool && go test ./...
```

After implementation and documentation wrap-up by the lead/implementer, run the MCP verification tool:

```text
ws/spec_index.verify(root: "/home/swkang/devenv")
```

Useful targeted commands during implementation:

```sh
cd agents-plugin-tool && go test ./internal/wsdoc ./internal/mcp ./cmd/ws-mcp
cd agents-plugin-tool && go test ./internal/wsdoc -run 'SpecsFind|MentalModelsFind|ReadConvention'
cd agents-plugin-tool && go test ./internal/mcp -run 'SpecsFind|MentalModelsFind|Documentation'
cd agents-plugin-tool && go test ./cmd/ws-mcp -run 'DocumentationCLI'
```

Manual smoke examples after tests pass:

```sh
cd agents-plugin-tool && go run ./cmd/ws-mcp specs find --root .. --query 'wsflow installer marketplace release packaging'
cd agents-plugin-tool && go run ./cmd/ws-mcp specs find --root .. --query 'wsflow installer marketplace release packaging' --format json
cd agents-plugin-tool && go run ./cmd/ws-mcp mental-models find --root .. --query 'runtime readable CLI mirror'
```

## Risks and Guardrails

- Exact selectors are the compatibility boundary: `spec_stem`, `ticket_stem`, and `domain` must stay exact and should be covered before changing formatter output.
- Do not let the new helper alter ticket query behavior; ticket tolerant matching is out of scope.
- Keep text output compact for LLM-readable defaults; stable consumers should use JSON evidence fields.
- If introducing truncation constants, keep them private and fixed in this phase; do not add public snippet-width or max-hit options.
- Avoid changing MCP schemas unless necessary for JSON result fields; result JSON can grow without input surface changes.

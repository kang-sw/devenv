# Brief: 260513-feat-tolerant-doc-find-queries

## Intent

Make documentation lookup tools behave like tolerant candidate discovery for
broad human queries while keeping exact selectors exact. The user-facing goal is
that callers can discover likely spec or mental-model documents from natural
topic phrases and can understand the returned candidates from line-level
evidence.

## Scope Boundary

Implement Phase 1 only: broad query behavior for `specs.find` and
`mental_models.find`, convention aliases for `convention.read`, default text
output for broad query discovery, and explicit JSON evidence fields. Do not
change ticket discovery, reference tracing, agent behavior, or dashboard
surfaces.

## Caller-Visible Contract

- `specs.find(query=...)` and `mental_models.find(query=...)` treat broad
  human query text as tolerant candidate discovery instead of requiring the full
  query string to appear contiguously.
- Exact structured selectors remain exact filters: `spec_stem`, `ticket_stem`,
  and `domain` must preserve their current validation and filtering behavior.
- Default text output for broad documentation queries is document-grouped
  grep-style evidence:
  - summary line such as `2 candidate specs for query="..."`;
  - document line formatted as `<path>\tscore=<score>\thits=<count>`;
  - evidence lines formatted as `  <line>: <snippet>`;
  - no separate `matched:` line in default text output.
- Document groups are ordered by aggregate score descending, then path. Evidence
  within each document is displayed in line-number order.
- If output is truncated, select the highest scoring documents or hits first,
  then display selected evidence in document and line order; the summary states
  that a subset is shown.
- JSON output keeps the existing document-centered metadata and adds line-level
  match evidence with line, matched terms, and snippet. A relative score may be
  exposed for ordering.
- `convention.read` accepts common aliases such as `spec`, `ticket`, and
  `mental-model`, resolving them to the canonical convention documents.
- Convention lookup failures report accepted canonical names and common aliases.

## Implementation Strategy Decisions

- Keep broad query matching in the documentation discovery layer so MCP and CLI
  mirrors share behavior.
- Keep default text output compact and human-oriented. Machine consumers should
  use JSON output for stable evidence fields.
- Use a score threshold for broad queries so a single weak token does not return
  noisy candidates.
- Do not add a public `context_tokens` option in this phase; use a fixed short
  snippet context.

## Rejected Alternatives

- Flat grep output was rejected because callers choose documents, not isolated
  hits, and document grouping better matches existing result structures.
- Default text `matched:` lines were rejected as noisy; matched terms remain
  available in JSON evidence.
- Public snippet-width options were deferred to avoid expanding the MCP and CLI
  surface before the basic UX proves useful.

## Approach

- Add line-level match evidence to spec and mental-model discovery results.
- Normalize broad query and document text across case, punctuation, hyphens,
  underscores, and token boundaries.
- Score candidate documents from matched terms across metadata and body text.
- Format broad query text output with document grouping and line evidence while
  leaving exact selector output close to current status/list style.
- Add convention alias resolution and clearer convention-not-found diagnostics.
- Update MCP and CLI tests for text and JSON behavior.

## Constraints

- Preserve exact selector validation and filtering semantics.
- Preserve explicit JSON output paths.
- Keep compact readable defaults aligned with the MCP runtime mental model.
- Keep implementation inside the existing `agents-plugin-tool` runtime and
  wsdoc/mcp formatting surfaces.

## Out of scope

- Ticket discovery tolerant matching.
- New public query options such as snippet width or max hit count.
- Dashboard or wsflow package changes unless existing tests reveal generated
  runtime metadata drift.

## Details

The broad query text output should look like:

```text
2 candidate specs for query="wsflow installer marketplace release packaging"

ai-docs/spec/plugin-runtime.md	score=18	hits=3
  18: ...agents-plugin-wsflow/ is an agentless derivative package...
  42: ...plugin-local .mcp.json through "mcpServers"...
  77: ...marketplace entries expose ws and wsflow...

ai-docs/spec/claude-compatibility.md	score=7	hits=1
  31: ...Claude-compatible plugin installs...
```

## Verification Contract

- Add or update Go tests for `SpecsFind`, `MentalModelsFind`, and
  `ReadConvention`.
- Add or update formatter/server/CLI tests proving default text output groups
  broad query evidence by document and JSON output retains structured metadata
  plus match evidence.
- Run `go test ./...` in `agents-plugin-tool`.
- Run spec index verification after documentation wrap-up.

## References

- [Must] `ai-docs/spec/mcp-tools.md#260519-tolerant-documentation-lookup-query-evidence` - planned MCP tool contract for this phase.
- [Must] `ai-docs/spec/mcp-tools.md#260512-documentation-discovery-readable-output-defaults` - compact readable output baseline.
- [Must] `ai-docs/spec/mcp-tools.md#260505-cli-mirror-coverage` - CLI mirror parity requirement.
- [Must] `ai-docs/spec/documentation-system.md#260505-documentation-convention-access` - canonical convention lookup surface.
- [Must] `ai-docs/spec/documentation-system.md#260505-spec-document-system` - spec discovery and selector expectations.
- [Must] `ai-docs/mental-model/documentation-system.md` - wsdoc discovery and convention source contracts.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP readable defaults and CLI mirror contracts.

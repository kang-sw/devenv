# Brief: 260526-bug-exec-readable-result-affordance

## Intent

Make the `exec.*` MCP surface easier for lead models to use by replacing
JSON-serialized text responses with compact readable text, shortening newly
generated exec keys, and giving `exec.result` a real wait affordance instead of
forcing callers into status polling loops.

## Scope Boundary

Implement only Phase 1 of
`260526-bug-exec-readable-result-affordance`: exec launch/status/result/abort
readability, new-key length, legacy-key compatibility, and result waiting or
guidance behavior. Do not implement `exec.ask`, dashboard UI, CLI mirrors, or
unrelated exec job lifecycle changes.

## Caller-Visible Contract

The lead-facing MCP `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`,
and `exec.abort` responses must be compact readable text, not JSON serialized
into MCP text content. Do not add or retain a public `format: json` option for
these exec tools.

`exec.result` must render metadata above a clear separator and raw stdout/stderr
below it when inline output is available. Use an obvious marker such as
`==========` so JSON stdout remains visually raw instead of escaped inside a
JSON payload. Metadata must remain compact enough for a model to reuse the
`exec_key`, inspect status/result readiness, and choose `exec.raw.*` when output
is too large.

Newly generated exec keys must be materially shorter than the current
`exec-<unix-nano>-<16hex>` shape. Use a practical worktree-local key such as
`exec-<8hex>` with collision retry. Existing persisted jobs with the old long
key shape must remain readable through status/result/raw readers.

`exec.result` must prevent the observed status-polling loop. Prefer
`timeout_seconds` on `exec.result`, with omitted or zero timeout preserving
non-blocking behavior and positive timeout waiting until the job is terminal or
the timeout expires. If implementation discovers this is structurally wrong,
escalate before choosing a separate wait primitive.

## Contract Instructions

- Update `agents-plugin-tool/internal/execjob/execjob.go` for new key generation,
  key validation compatibility, and any wait-aware result primitive needed by
  MCP dispatch.
- Update `agents-plugin-tool/internal/mcp/server.go` so the exec dispatch cases
  use exec-specific text formatters instead of `toolJSONResponse`.
- Update the `exec.result` MCP schema/description if `timeout_seconds` is added.
- Preserve file-backed payload behavior: missing stdout/stderr/combined files
  must still surface recoverable consistency warnings/errors, not empty output.
- Preserve the 4096-byte inline budget. Large outputs should return metadata
  and guidance, not inline raw output.
- Preserve `exec.raw.*` behavior except for any necessary response readability
  changes if they currently conflict with the no-JSON lead-facing direction.
- Do not add public exec CLI mirrors.
- Do not change wsflow no-agent visibility except to keep existing hidden-tool
  assertions passing.

## Integration Test Instructions

Extend or add tests around:

- short key generation and old long-key compatibility;
- MCP launch/status/result/abort responses no longer being JSON payload text;
- `exec.result` inline output using metadata + separator + raw stdout/stderr;
- JSON-shaped stdout remaining unescaped in the raw output area;
- positive `timeout_seconds` waiting for a running job to finish;
- non-blocking `exec.result` behavior still giving useful readable guidance for
  running jobs;
- large output staying out of inline result body and pointing to the raw fallback
  path;
- wsflow no-agent hidden-tool behavior remaining unchanged.

Run at minimum:

```bash
cd agents-plugin-tool && go test ./internal/execjob ./internal/mcp ./cmd/ws-mcp
python3 -m unittest discover agents-plugin-wsflow/tests
```

If runtime metadata or package contract files change, also run the matching
package contract tests that fail in the edited surface.

## Implementation Strategy Decisions

- Text-only is intentional. Do not preserve JSON as a caller-facing escape hatch
  for this exec surface.
- Use `exec.result(timeout_seconds: N)` rather than a new wait primitive unless
  code inspection reveals a concrete blocker.
- Preserve old long-key reads while generating only the new shorter key shape.
- Keep output formatting deterministic and test by exact stable markers where
  possible.

## Rejected Alternatives

- `format: json` fallback was rejected because the issue is the lead-facing
  default response becoming unreadable.
- Solving this through `exec.ask` was rejected for this slice because `exec.ask`
  is a later large-output question UX, not the basic launch/result/status
  affordance.
- Status polling loops were rejected as the primary result-retrieval behavior.

## Approach

- Add focused exec response formatting helpers in or near MCP dispatch.
- Adjust exec key generation/validation before updating MCP tests so new
  follow-up examples use the shorter token.
- Add a wait-aware result path that reconciles job status until terminal or
  timeout without blocking unrelated MCP requests.
- Update tests from JSON assertions to readable-text contract assertions.
- Update docs/specs after implementation behavior is verified.

## Constraints

- MCP responses remain MCP text content.
- Large stdout/stderr payload bodies stay file-backed.
- Long-running waits must not block `tools/list` or unrelated concurrent MCP
  calls.
- The full `exec.*` surface remains hidden in wsflow no-agent mode.

## Out of scope

- `exec.ask`.
- Dashboard rendering or Activity Console behavior.
- New CLI mirrors.
- Changing exec storage authority from SQLite metadata plus file-backed stream
  payloads.

## Details

Relevant current implementation points:

- `agents-plugin-tool/internal/execjob/execjob.go` owns `Response`, `Launch`,
  `Status`, `Result`, `Abort`, key validation/generation, legacy state import,
  stream paths, and payload consistency warnings.
- `agents-plugin-tool/internal/mcp/server.go` owns exec MCP dispatch and tool
  schemas. The current exec cases use `toolJSONResponse`.
- `agents-plugin-tool/internal/mcp/server_test.go` has end-to-end MCP assertions
  for exec launch/status/result/raw readers, running/large output, abort, and
  wsflow no-agent hiding.
- `agents-plugin-tool/internal/execjob/execjob_test.go` has manager-level
  coverage for launch/result/raw readers, large output, abort, lost workers,
  SQLite metadata, legacy import, and missing payloads.

## Verification Contract

Acceptance requires implementation tests to pass and output examples to show:

- `exec.result` no longer emits JSON object text for normal lead-facing results.
- Raw JSON stdout appears unescaped below the separator.
- Running jobs can be waited through `exec.result(timeout_seconds: N)`.
- Old persisted long keys still work in status/result/raw readers.
- wsflow still hides/rejects `exec.*`.

## References

- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP text response contracts,
  exec surface coupling, wsflow hiding, and payload consistency rules.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - runtime contract and wsflow
  package alignment rules.
- [Must] `ai-docs/spec/mcp-tools.md` - readable-output default and exec job MCP
  contracts.
- [Must] `ai-docs/tickets/ready/260526-bug-exec-readable-result-affordance.md`
  - selected Phase 1 scope and settled decisions.
- [Maybe] `ai-docs/tickets/todo/260524-feat-exec-output-ask.md` - adjacent
  future large-output UX that should not be implemented in this slice.

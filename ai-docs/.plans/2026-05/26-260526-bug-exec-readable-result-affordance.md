# Survey: 260526-bug-exec-readable-result-affordance

## Reusable Components
- `agents-plugin-tool/internal/execjob/execjob.go#L73-L89` — `Response`: shared lifecycle/output DTO for `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`, and `exec.abort`; already carries metadata, inline stdout/stderr, byte counts, result readiness, and guidance.
- `agents-plugin-tool/internal/execjob/execjob.go#L90-L104` — `RawReadResponse`, `RawTailResponse`, `RawGrepResponse`: raw-reader DTOs expose `exec_key`, selected stream, read offsets, tail text, and grep matches for text formatter inputs.
- `agents-plugin-tool/internal/execjob/execjob.go#L218-L236` — `Status`/`Result`: central follow-up readers reconcile persisted state before response creation; `Result` is the current non-terminal error point for wait/non-blocking guidance behavior.
- `agents-plugin-tool/internal/execjob/execjob.go#L237-L257` — `Abort`: cancellation primitive returns the same `Response` shape after signalling active or persisted PIDs and refreshing stream sizes.
- `agents-plugin-tool/internal/execjob/execjob.go#L259-L295` — `Tail`/`Read`/`Grep`: raw-reader semantics already route through stream path resolution, payload-presence checks, textreader defaults/caps, and path stripping for grep matches.
- `agents-plugin-tool/internal/textreader/textreader.go#L22-L42` — `ReadResult`, `GrepMatch`, `GrepResult`: lower-level result structs provide offset/next-offset/limit/size/EOF and match context fields needed for readable raw-reader output.
- `agents-plugin-tool/internal/textreader/textreader.go#L44-L119` — `Tail`/`Read`: bounded file readers preserve empty/missing file behavior, tail defaults/caps, read caps, and continuation offsets.
- `agents-plugin-tool/internal/textreader/textreader.go#L122-L185` — `Grep`: literal-by-default search with regex opt-in, before/after context, max-match cap, and truncation flag.
- `agents-plugin-tool/internal/execjob/execjob.go#L378-L405` — `responseFor`: single place enforcing missing-payload warnings, result readiness, 4096-byte inline budget, and stdout/stderr file reads for lifecycle/result responses.
- `agents-plugin-tool/internal/execjob/execjob.go#L408-L435` — `payloadConsistencyWarning`/`guidance`: existing recoverable consistency warning and follow-up guidance strings reused by launch/status/result metadata.
- `agents-plugin-tool/internal/execjob/execjob.go#L493-L522` — `jobDir`/`statePath`: key validation and worktree-local job directory mapping; compatibility point for short new keys plus old long persisted keys.
- `agents-plugin-tool/internal/execjob/execjob.go#L525-L547` — `readRecord`: SQLite-first metadata read with legacy `state.json` import fallback for old persisted jobs.
- `agents-plugin-tool/internal/execjob/execjob.go#L640-L672` — `streamPathFromStore`: stream path resolver used by raw readers and result inline output; preserves SQLite metadata first and legacy directory fallback.
- `agents-plugin-tool/internal/mcp/server.go#L2152-L2161` — `toolTextResponse`: reusable MCP text-content success envelope for all exec text-only formatters.

## Existing Patterns
- Readable text dispatch: see `agents-plugin-tool/internal/mcp/server.go#L527-L532` — tool dispatch can choose a formatter plus `toolTextResponse` instead of JSON text serialization.
- Compact labeled formatters: see `agents-plugin-tool/internal/mcp/server.go#L1569-L1590` and `agents-plugin-tool/internal/mcp/server.go#L1602-L1637` — existing MCP text formatters use `strings.Builder`, stable labels, and unescaped text output.
- JSON serialization boundary to remove from exec: see `agents-plugin-tool/internal/mcp/server.go#L2141-L2150` — `toolJSONResponse` marshals structs into JSON text content, causing escaped nested JSON for command output.
- Exec MCP dispatch grouping: see `agents-plugin-tool/internal/mcp/server.go#L454-L526` — all lifecycle and raw-reader exec tools already share adjacent dispatch cases and root resolution.
- Raw-reader manager tests: see `agents-plugin-tool/internal/execjob/execjob_test.go#L39-L67` — launch, result, tail, read, and grep are exercised together against real persisted streams.
- Raw-reader default/cap tests: see `agents-plugin-tool/internal/textreader/textreader_test.go#L10-L43` and `agents-plugin-tool/internal/textreader/textreader_test.go#L45-L87` — lower-level semantics are already covered independently from MCP formatting.
- No-agent hidden surface: see `agents-plugin-tool/internal/mcp/server.go#L2975-L2988` — the full `exec.` prefix is hidden in wsflow no-agent mode.
- Runtime no-agent verification: see `agents-plugin-tool/cmd/ws-mcp/main_test.go#L92-L116` and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L12-L44` — tests assert exec tools stay absent from no-agent runtime capabilities and package contract.
- Legacy import fixture pattern: see `agents-plugin-tool/internal/execjob/execjob_test.go#L243-L279` — tests create old long-key directories plus `state.json` to verify import and corrupt recovery.
- Missing payload assertions: see `agents-plugin-tool/internal/execjob/execjob_test.go#L281-L329` — tests remove stdout/stderr/combined files and assert status/result/raw readers surface recoverable consistency states.

## Relevant Interfaces
- `agents-plugin-tool/internal/execjob/execjob.go#L40-L49` — `LaunchOptions`: MCP dispatch constructs this directly for argv and shell launches.
- `agents-plugin-tool/internal/execjob/execjob.go#L111-L216` — `Launch`: generates the exec key, creates stream files, stores metadata, waits up to `ForegroundWindow`, and returns bounded inline output for quick terminal jobs.
- `agents-plugin-tool/internal/execjob/execjob.go#L298-L303` — `requirePayloadPresent`: raw-reader consistency gate that turns missing persisted payload files into recoverable errors instead of empty text.
- `agents-plugin-tool/internal/execjob/execjob.go#L689-L695` — `newKey`: current generator returns `exec-<unix-nano>-<16hex>`; short-key generation and collision retry belong near this interface.
- `agents-plugin-tool/internal/execjob/execjob.go#L36-L37` and `agents-plugin-tool/internal/execjob/execjob.go#L493-L496` — `keyPattern` validation: currently accepts only the old long shape, so new and legacy shapes must both pass before job path resolution.
- `agents-plugin-tool/internal/mcp/server.go#L454-L526` — exec dispatch cases: launch/status/result/abort/tail/read/grep all currently end with `toolJSONResponse` and need exec-specific text formatting.
- `agents-plugin-tool/internal/mcp/server.go#L2286-L2325` — exec tool declarations: `exec.result` currently has no `timeout_seconds`; raw-reader schemas are already adjacent.
- `agents-plugin-tool/internal/mcp/server.go#L3059-L3088` — `execKeySchema`, `execRawTailSchema`, `execRawReadSchema`, `execRawGrepSchema`: schema helpers for result timeout and raw-reader arguments.
- `ai-docs/spec/mcp-tools.md#L373-L415` — spec anchor `260524-exec-job-mcp-tools`: durable exec contract for working_dir resolution, foreground launch wait, SQLite metadata, result budget, lost-worker reconcile, and raw-reader missing-payload errors.
- `agents-plugin/runtime.json#L82-L89` — full ws runtime contract includes all exec lifecycle and raw-reader tool names.
- `agents-plugin-wsflow/runtime.json#L36-L63` — wsflow runtime contract intentionally omits exec tools.

## Constraints
- `ai-docs/mental-model/mcp-runtime.md#L44-L45` — MCP tool results are text content even when text is JSON; this change keeps MCP text but removes JSON-serialized exec payloads.
- `ai-docs/mental-model/mcp-runtime.md#L51-L52` — `ServeStdio` handles requests concurrently; `exec.result(timeout_seconds)` waits must not block `tools/list` or unrelated calls.
- `ai-docs/mental-model/mcp-runtime.md#L72-L73` — public exec schemas use `working_dir`, not `root`, and there is no public exec CLI mirror.
- `ai-docs/mental-model/mcp-runtime.md#L75-L76` — exec metadata is SQLite-backed and legacy `state.json` remains import-only.
- `ai-docs/mental-model/mcp-runtime.md#L77-L78` — missing stdout/stderr/combined files are recoverable file-backed payload consistency states, including through raw readers.
- `ai-docs/mental-model/plugin-runtime.md#L41-L44` — exec surface changes must keep the full ws runtime contract, keep exec out of wsflow no-agent contract, and avoid launcher-required CLI mirrors.
- `ai-docs/tickets/ready/260526-bug-exec-readable-result-affordance.md#L36-L44` — settled decisions reject public JSON fallback and require `exec.result` metadata above a separator with raw stdout/stderr below.
- `ai-docs/.plans/2026-05/26-260526-bug-exec-readable-result-affordance.brief.md#L37-L44` — clarified contract includes `exec.raw.*` in text-only MCP formatting while preserving raw-reader semantics.

## Risk Signals
- `agents-plugin-tool/internal/mcp/server_test.go#L2035-L2046` — Possible test/contract risk: launch text is currently parsed as JSON to recover `exec_key`; readable output needs a stable text extraction/assertion pattern.
- `agents-plugin-tool/internal/mcp/server_test.go#L2048-L2065` — Possible raw-reader test risk: status/result/tail/read/grep assertions currently look inside JSON text responses; all follow-up assertions need readable-text expectations.
- `agents-plugin-tool/internal/mcp/server_test.go#L2097-L2104` — Possible test risk: running launch response currently expects JSON status and unmarshals `exec_key`.
- `agents-plugin-tool/internal/mcp/server_test.go#L2118-L2125` — Possible test risk: abort polling checks for JSON substring `"status":"cancelled"`; readable status labels must replace it.
- `agents-plugin-tool/internal/mcp/server_test.go#L2133-L2135` — Possible large-output risk: current large-output assertions rely on JSON `"combined_bytes":5000`; formatter should expose equivalent byte metadata without inline payload.
- `agents-plugin-tool/internal/execjob/execjob.go#L225-L234` — Possible contract risk: `Result` returns an error for running jobs, which MCP reports as `isError: true`; non-blocking result guidance may need a success text response while preserving manager-level semantics or adding a wait-aware primitive.
- `agents-plugin-tool/internal/execjob/execjob.go#L259-L295` — Possible raw-format risk: raw-reader functions return only data structs; MCP formatting must not alter tail/read/grep defaults, caps, regex behavior, context lines, or stream selection.
- `agents-plugin-tool/internal/execjob/execjob.go#L689-L695` — Possible reuse risk: short-key generation needs collision retry against existing worktree-local SQLite/legacy job records, not just a shorter random token.
- `agents-plugin-tool/internal/execjob/execjob.go#L674-L687` — Possible compatibility risk: `updateSizes` reaches stream files through `jobDir`; validation changes must accept both short and old long keys or size refresh/raw compatibility breaks.
- `ai-docs/tickets/todo/260524-feat-exec-output-ask.md#L23-L27` — Possible scope risk: guidance may mention future `exec.ask`, but this slice must not implement that tool.

## Opinion
- `agents-plugin-tool/internal/mcp/server.go#L454-L526` — The clarified scope is mechanically localized in MCP dispatch/formatting plus `exec.result` schema/wait and execjob key/result support; raw-reader core semantics already live below MCP and can remain unchanged.
- `ai-docs/spec/mcp-tools.md#L399-L415` — Spec currently describes terminal-only result and raw-reader semantics but not readable raw-reader formatting; docs will need a post-implementation audit after behavior is verified.

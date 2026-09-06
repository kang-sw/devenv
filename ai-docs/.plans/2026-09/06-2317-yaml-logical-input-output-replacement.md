# Plan: yaml-logical-input-output-replacement

## Relevant Ticket Contract
- Restore YAML **display** for bridged `ws__*` output and add YAML input-parameter previews without changing MCP dispatch, tool arguments/results, `details`, or model-visible bytes.
- Both collapsed previews select the first ten newline-separated logical lines **before** Pi renders text; expanded output is full, while input remains a ten-line preview.
- Preserve raw/error/non-container/mixed-content behavior; partial streamed input arguments, including absent data, must not throw.
- Avoid the reverted custom Unicode/wrapping algorithm: use Pi's native `Text` rendering and caches. Use only Pi-supported terminal-control and, if proven necessary, bounded post-render clipping seams.

## Out of Scope
- The historical ticket's visual-row counting, ASCII fallback, grapheme/candidate-width loops, and wholesale rollback reversal.
- Dispatch-tool summaries, resolved-model lines, Explore lifecycle, custom dispatch tools, background behavior, direct read/run polish, specs, or merges.
- Owner live performance acceptance; this slice supplies automated call-count/performance evidence only.

## Codebase Findings
- `agents-plugin-pi/src/bridge.ts#L462-L539` — one registration loop owns every bridged `ws__*` tool and returns MCP `content`/`details` unchanged; shared `renderCall` and `renderResult` hooks can be added here without touching dispatch.
- `agents-plugin-pi/src/mcp-stdio-client.ts#L34-L43` — ordered, mixed MCP content and `isError` are the wire contract; a renderer must not mutate or flatten it for the model.
- `agents-plugin-pi/package.json#L13-L16` — `yaml` is not a declared dependency after rollback, although the installed tree currently has an extraneous copy; re-add the packaged serializer deliberately.
- `agents-plugin-pi/src/push-render.ts#L105-L143` — guarded dynamic `pi-tui` loading already exposes a reusable local pattern for `Text` rather than a static nested-package import.
- `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/dist/components/text.js#L20-L87` — native `Text.setText()` invalidates only changed text and `render(width)` caches its wrapping/layout by text and width.
- `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L307-L339` and `.../tool-execution.js#L248-L261` — Pi passes `lastComponent`, shared state, partial/error state, and current `expanded` state to both row slots. `renderCall` has no separate documented expanded-result options, so keep its owner-required ten-line cap rather than inventing input expansion semantics.
- `22987b75^:agents-plugin-pi/src/tool-result-render.ts#L97-L218` — the reverted renderer serializes and wraps the entire payload on each `render()`, uses custom grapheme width logic, and has no cache; do not restore that implementation.
- `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/dist/index.d.ts#L31` — Pi exports `Text`, `stripTerminalSequences`, and `truncateToWidth`; these are the supported native rendering/safety seams.

## Implementation Plan
1. In `agents-plugin-pi/package.json` and `agents-plugin-pi/package-lock.json`, restore the explicit `yaml@2.9.0` runtime dependency; create a new focused `agents-plugin-pi/src/tool-result-render.ts` rather than resurrecting the reverted module wholesale.
2. In `agents-plugin-pi/src/tool-result-render.ts`, add pure, non-mutating display preparation: serialize only JSON-container output or object-like input to YAML; leave errors, scalar/non-container JSON, malformed/prose text, later text blocks, and image/mixed content in their existing raw/display paths. Normalize CRLF and select ten logical lines before any native layout. Treat absent or malformed streamed input arguments as an empty/safe preview.
3. Reuse the existing guarded `pi-tui` host-loader pattern to obtain `Text` and `stripTerminalSequences`; implement a small row component that retains its `Text` through `context.lastComponent`, caches prepared output/input text by semantic source and mode, and calls `Text.setText()` only when that source changes. Delegate wrapping and unchanged-width redraw caching to `Text`; do not serialize, segment, or wrap the full payload in `render()`.
4. In `agents-plugin-pi/src/bridge.ts#L462-L539`, load the native renderer once during bridge initialization and attach shared `renderResult` and `renderCall` hooks to all bridged registrations. `renderResult` uses full text only when expanded; `renderCall` always uses the ten-line input preview and remains safe while `argsComplete` is false. Preserve execute/error behavior and all MCP/model data exactly.
5. Use Pi's `stripTerminalSequences` before native text rendering. Verify native output boundaries; only if a reproducible oversize-grapheme row remains, add bounded post-render `truncateToWidth` clipping through the same loaded host API—never a new Unicode or candidate-row loop.
6. In `agents-plugin-pi/test/bridge.test.ts`, restore focused bridge integration coverage and add pure/cache probes: YAML input/output, raw/error/non-container/mixed content, partial/missing arguments, ten-logical-line selection before native wrapping, expanded output, byte-identical dispatch data, native-component reuse, unchanged redraws with no extra serialization/layout calls, and changed content/width/expanded state invalidation. Retain terminal-control coverage through the native seam.

## Verification Plan
- Run `cd agents-plugin-pi && env -u WS_PI_SPAWN_ROLE npm test`.
- Run focused renderer tests with counters proving unchanged redraws do not reserialize or rewrap the full payload; cover changed result/input, width, and expansion transitions.
- Confirm via the bridge fake-MCP test that every `ws__*` registration receives both hooks and model-visible arguments/results remain byte-identical.
- Live Pi responsiveness/YAML acceptance is deferred to the owner.

## Escalations
- None.

# Plan: 260906-feat-ws-pi-tool-result-yaml-tui-rendering — Phase 1: Shared renderResult with YAML display

## Relevant Ticket Contract
- `agents-plugin-pi/` only: display JSON-object and JSON-array text results as YAML in Pi while leaving the `execute` result, ws-mcp wire payload, and model-visible JSON byte-identical.
- Convert only the first `{ type: "text" }` item when it parses to a JSON container; leave prose, bare JSON values, malformed JSON-looking text, errors, and every later content block available as raw content.
- Collapsed output is width-safe: show 10 rendered body rows, then a `… N more rows` marker with Pi's standard expand-key hint; expanded output shows all rows.
- Add `yaml@2.9.0`, retain the no-static-`@earendil-works/pi-tui` import invariant, and amend `pi-adapter-runtime` `{#260903-pi-bridge-tool-registration}`.

## Out of Scope
- Phase 2 `renderCall` dispatch summaries, resolved model/effort `details`, `spawnAgent` callbacks, and model resolver changes.
- ws-mcp behavior, model-visible output changes, a static pi-tui dependency/import, and redesigning tool-row headers.

## Codebase Findings
- `agents-plugin-pi/src/bridge.ts#L464-L539` — the sole ws-mcp registration loop has only `execute`; it dispatches by raw name and returns `{content, details: result}`, so one `renderResult` there covers all bridged `ws__*` tools without changing dispatch or model content.
- `agents-plugin-pi/src/mcp-stdio-client.ts#L28-L34` — MCP results retain an ordered `content` array of generic items; the renderer must select only the first text item for conversion and not discard later text or non-text blocks.
- `agents-plugin-pi/src/overlay-chat.ts#L200-L263` — existing structural components avoid pi-tui imports and provide ANSI-aware wide-character measurement/wrapping conventions, but its local width approximation does not cover all Unicode/grapheme cases; the result renderer needs its own width-safe boundary tests rather than assuming logical lines are terminal rows.
- `agents-plugin-pi/test/bridge.test.ts#L1-L27` — bridge pure seams are unit-tested with Node's native TypeScript stripping and no Pi process; this is the appropriate regression home for parser/serializer/render-component tests.
- `agents-plugin-pi/package.json#L13-L16` — the adapter currently has only the Pi coding-agent runtime dependency; `yaml@2.9.0` and the lockfile must be added for packaged resolution.
- `ai-docs/spec/pi-adapter-runtime.md#L23-L56` — the bridge-registration anchor already defines raw dispatch and error rethrowing, which the new display-only bullet must preserve.

## Implementation Plan
1. In `agents-plugin-pi/package.json` and `agents-plugin-pi/package-lock.json`, add the pinned runtime dependency `yaml@2.9.0` so the adapter tarball resolves the serializer independently of Pi's nested dependency tree.
2. In `agents-plugin-pi/src/bridge.ts`, add pure helpers that (a) accept only JSON objects/arrays for YAML conversion and (b) serialize parsed containers to YAML with failure fallback to the original text. Build the rendered text from the original ordered content array: convert only the first text item that the ticket selects and retain all later text blocks unchanged.
3. In `agents-plugin-pi/src/bridge.ts`, attach one structural `renderResult` to every bridge-loop registration. Its `render(width)` must wrap raw and YAML body text by display width, cap collapsed output at 10 body rows plus a width-safe `… N more rows` marker using `keyHint("app.tools.expand", "to expand")`, and return every row when expanded. Keep error rows raw by checking `context.isError`; do not import `@earendil-works/pi-tui` statically.
4. Extend `agents-plugin-pi/test/bridge.test.ts` with pure/component tests for object and array YAML, prose/bare-string/malformed JSON fallback, retained later content blocks, error raw fallback, wrapped narrow long lines, wide Unicode width boundaries, collapsed marker/count and expanded output in YAML and raw branches, byte-identical `execute` content, and the no-static-pi-tui-import invariant.
5. Amend `ai-docs/spec/pi-adapter-runtime.md` at `{#260903-pi-bridge-tool-registration}` to state that JSON-shaped text is rendered as YAML only in the Pi tool row while the model result is unchanged.

## Verification Plan
- From `agents-plugin-pi/`, run `env -u WS_PI_SPAWN_ROLE npm test` so adapter tests execute as the host lead rather than inheriting a child-role surface.
- Run the focused bridge renderer tests while checking every `render(width)` line is within the requested width and that collapsed versus expanded row counts match the 10-body-row contract.
- Owner live check: call `ws__config_resolve_agent` with `format: "json"` in a Pi lead TUI; confirm YAML preview/expand behavior and that the next model turn still parses the original JSON fields.

## Escalations
- None.

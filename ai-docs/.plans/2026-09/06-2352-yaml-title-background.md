# Plan: yaml-title-background

## Relevant Ticket Contract
- Restore a bold native registered `ws__*` tool title and give the bridged YAML input and output distinct backgrounds without changing the model payload, raw/image fallback, expansion, ten-logical-line display cap, caches, or narrow-width safety.
- Use only Pi's existing theme keys; keep native pending/success/error state visible. This inline scope supersedes historical YAML ticket phase text.
- Live visual acceptance happens after implementation; this plan makes no source changes.

## Out of Scope
- Tool unification, custom dispatch-tool rows, push styling, read/run changes, resolved-model/Phase 2 metadata, ws-mcp changes, and source edits.
- The unrelated untracked `ai-docs/.plans/2026-09/06-1203-260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches.md`.

## Codebase Findings
- `agents-plugin-pi/src/tool-result-render.ts#L105-L156` — `BoundedText` preserves native `Text` layout/cache and only post-clips indivisible overwide graphemes, but its current call/result renderers discard the supplied `theme` and return a single unstyled text component.
- `agents-plugin-pi/src/tool-result-render.ts#L174-L207` — call previews combine the registered name plus at most ten logical YAML lines; completed JSON-container result previews reuse cache by content identity and expansion, while error/partial/prose/mixed/image results throw to Pi’s fallback.
- `agents-plugin-pi/src/bridge.ts#L466-L485` — every registered bridge tool gets the same preview hooks and does not set `renderShell: "self"`, so Pi’s default tool shell remains available.
- `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/components/tool-execution.js#L31-L40,L182-L231` — the default parent `Box` selects `toolPendingBg`, `toolSuccessBg`, or `toolErrorBg`; a renderer throw falls back to Pi’s raw text/image behavior. Preserve this parent shell.
- `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/dist/components/box.js#L50-L87` and `components/text.js#L55-L98` — native `Box` backgrounds are applied after child rendering, while `Text` owns ANSI-safe wrapping, cache invalidation, and narrow-width padding.
- `agents-plugin-pi/test/tool-result-render.test.ts#L101-L247` — existing tests cover title retention, logical cap/expansion, caching, fallback, installed-shell integration, and CJK/emoji width one; extend this seam rather than adding a new renderer path.

## Implementation Plan
1. In `agents-plugin-pi/src/tool-result-render.ts`, dynamically require Pi’s native `Box` alongside `Text` and add small structural helpers that build content-only child boxes; retain the current default parent shell in `agents-plugin-pi/src/bridge.ts`.
2. Style the call title with `theme.fg("toolTitle", theme.bold(registeredName))` and its YAML input with `toolOutput`, inside an input child `Box` using `toolPendingBg`.
3. Render eligible YAML output in a separate child `Box`: `toolPendingBg` while partial, `toolSuccessBg` when settled, and let error/non-eligible paths throw to Pi’s native fallback. The parent shell continues to signal the actual pending/success/error lifecycle; nested input/output surfaces distinguish their roles using the same native semantic palette, with no invented colors.
4. Keep `logicalPreview`, `updateText`, `BoundedText`, native `Text` caching, and `truncateToWidth` unchanged in behavior; update `agents-plugin-pi/test/tool-result-render.test.ts` fakes and assertions for bold/title/output styling, child background selection, fallback, cache reuse, and width-one safety.

## Verification Plan
- From `agents-plugin-pi/`, run `npm test`.
- Confirm the existing installed `ToolExecutionComponent` integration test still exercises the default parent shell, and add focused assertions for call/output child-box palette selection across pending, success, and error fallback states.
- Manual later: invoke a fresh bridged `ws__*` JSON tool in Pi; verify bold registered title, pending input surface, distinct settled output surface, error surface, expand/collapse, and raw/image fallback.

## Escalations
- None.

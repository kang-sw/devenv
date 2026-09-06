---
title: Pi TUI renders JSON-shaped ws tool results as YAML while the model keeps receiving JSON
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 59a494f2524cf132
sage-review-completeness-reviewed: 59a494f2524cf132
---

# Pi TUI renders JSON-shaped ws tool results as YAML while the model keeps receiving JSON

## Background

Owner request, 2026-09-06, while dogfooding a Pi lead session: most bridged
ws-mcp calls (`ws__config_resolve_agent`, `ws__ferrule`, `ws__git_status`,
`ws__tickets_query`, ...) answer with JSON text, whether because the skill text
passes `format: "json"` or because the tool defaults to it. JSON is the right
shape for the model, which parses fields out of it, but in the TUI tool row it
is dense to read. The owner asks whether the plugin can keep returning JSON to
the agent and show the same result as YAML to the human.

Pi separates the two already. `pi.registerTool` takes optional `renderCall`
and `renderResult` hooks that produce the TUI row's components; the value
returned by `execute` is what the model reads and is untouched by the
renderers (`docs/extensions.md`, "Custom Rendering"). `renderResult(result,
{expanded, isPartial}, theme, context)` receives `{content, details}` (the
error flag is on `context.isError`, not on `result`), must return a TUI
`Component` (`render(width): string[]` plus optional `handleInput`/
`invalidate`), and is called again when the user toggles the row's expanded
state. The bridge's registration loop in `startBridge`
(`agents-plugin-pi/src/bridge.ts`) currently registers only `execute`, so
every `ws__*` row uses Pi's fallback renderer.

Why the rows are disruptive today even though that fallback already caps a
collapsed row at `FALLBACK_PREVIEW_LINES` (10): ws-mcp's `toolJSONResponse`
uses `json.Marshal`, so a JSON result is **one** logical line, and the cap
counts logical lines. A 2 KB single-line JSON wraps to twenty or more terminal
rows and the cap never engages. YAML output is many short lines, so a cap on
rendered rows shows the leading keys instead of the first third of an
unreadable line.

Two packages this needs are nested under Pi's own `node_modules` and are not
on the adapter's resolution path: `yaml@2.9.0` (Pi's own dependency) and
`@earendil-works/pi-tui` (the `Text` component). The adapter already
documents the pi-tui half as an invariant (`push-render.ts`,
`overlay-chat.ts`: no static pi-tui import, since `node --test` imports these
modules) and solves it with a purely structural `Component` object.

## Proposed direction

- One shared `renderResult` attached to every `ws__*` registration in the
  bridge loop. It takes the first `{type: "text"}` content item, and when
  that text parses as a JSON object or array, renders it as a YAML document;
  any other text (prose, markdown, non-container JSON) is rendered as-is.
  The `execute` return value does not change, so the wire and model
  contracts stay exactly what `{#260903-pi-bridge-tool-registration}`
  describes.
- Component shape: a structural object with `render(width)` and no pi-tui
  import, the pattern `overlay-chat.ts` already uses, so `bridge.ts` stays
  importable under `node --test`. Rejected: pinning `@earendil-works/pi-tui`
  as an adapter dependency (a second copy that drifts from Pi's) and a
  preloaded dynamic import with conditional registration (more machinery for
  a component that only emits lines).
- Collapsed rows show at most 10 **rendered rows at the given width**
  (owner's directive, 2026-09-06), wrapping counted, followed by a
  `… N more rows` marker and the standard expand key hint; expanded rows show
  all of it. The trim applies to both branches, YAML and raw text, so a long
  single-line result is trimmed by rows where Pi's logical-line cap would not
  engage. Error results (`context.isError`, where `details` is undefined
  because the bridge throws on `isError`) are not converted and are trimmed
  the same way; Pi paints the error background itself, independent of the
  renderer.
- Serializer: add `yaml` to the adapter's `dependencies` pinned to the same
  version Pi ships (`2.9.0`), so an installed tarball carries it and the
  resolution ambiguity above disappears for that package. Rejected: a
  hand-rolled JSON to YAML printer, since quoting and multi-line string rules
  are where such printers go wrong, and the real package is already on every
  Pi install.
- The JSON detection is a pure function (`text -> parsed | undefined`) and
  the YAML rendering a pure function (`parsed -> lines`), so both are
  unit-testable without a TUI; the `renderResult` hook is a thin adapter over
  them.
- Out of scope: a `renderCall` for the row header (raw ws tool name plus key
  arguments). The sanitized `ws__` name is already the row label; a header
  redesign is a separate ticket if wanted.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-bridge-tool-registration}`: add one bullet
stating that JSON-shaped text results are displayed as YAML in the TUI row via
`renderResult` while the tool result returned to the model is unchanged.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change and no change
  to the text the model receives.
- Rendering must never throw: a parse or serialize failure falls back to the
  raw text.
- No static `@earendil-works/pi-tui` import anywhere in `bridge.ts` or its
  imports; `npm test` must keep passing with `bridge.test.ts` importing it.
- Headless leads (`--mode rpc`, no TUI) are unaffected; the hook is never
  invoked there.

## Phases

### Phase 1: Shared renderResult with YAML display

Add the `yaml` dependency, the two pure functions, and the shared
`renderResult` in the bridge loop. Tests: a JSON object result renders as YAML
lines; a JSON array renders as YAML; prose and a bare JSON string render
unchanged; a malformed JSON-looking text renders unchanged; the collapsed
`render(width)` output holds at most 10 rows plus the `… N more rows` marker
and the expanded output holds all, for the YAML branch and the raw-text branch
alike; a single long line at a narrow width is counted by wrapped rows, not
logical lines; an error result (`context.isError`, `details` undefined) is not
converted but is trimmed the same way; the `execute` return for a JSON result
is byte-identical before and after; `bridge.ts` has no static pi-tui import. Amend the spec bullet
under Spec Impact. Live check (owner-run): call `ws__config_resolve_agent`
with `format: "json"` in a Pi lead session and confirm the row shows YAML,
expands to the full document, and the model's next turn still parses the
fields.

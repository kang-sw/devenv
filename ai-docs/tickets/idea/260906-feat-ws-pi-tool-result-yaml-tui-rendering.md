---
title: Pi TUI renders JSON-shaped ws tool results as YAML while the model keeps receiving JSON
spec:
  - pi-adapter-runtime
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
{expanded, isPartial}, theme, context)` receives the tool result's `content`
and `details`, must return a TUI `Component` (typically `Text` from
`@earendil-works/pi-tui`), and is called again when the user toggles the row's
expanded state. Pi's own fallback renderer shows the first
`FALLBACK_PREVIEW_LINES` (10) lines collapsed and everything when expanded.
The bridge's registration loop in `startBridge` (`agents-plugin-pi/src/bridge.ts`)
currently registers only `execute`, so every `ws__*` row uses that fallback.

For serialization, Pi depends on `yaml@2.9.0`, but that package sits under
Pi's own `node_modules` and is not hoisted into the adapter's resolution
path, so the extension cannot import it as-is.

## Proposed direction

- One shared `renderResult` attached to every `ws__*` registration in the
  bridge loop. It takes the first `{type: "text"}` content item, and when
  that text parses as a JSON object or array, renders it as a YAML document;
  any other text (prose, markdown, non-container JSON) is rendered as-is.
  The `execute` return value does not change, so the wire and model
  contracts stay exactly what `{#260903-pi-bridge-tool-registration}`
  describes.
- Collapsed rows show the first 10 lines of the YAML (matching Pi's
  fallback preview count) with the standard expand key hint; expanded rows
  show all of it. Error results (`isError`) keep Pi's error styling and are
  not converted.
- Serializer: add `yaml` to the adapter's `dependencies` pinned to the same
  version Pi ships (`2.9.0`), so an installed tarball carries it and the
  resolution ambiguity above disappears. Rejected: a hand-rolled JSON to
  YAML printer, since quoting and multi-line string rules are where such
  printers go wrong, and the real package is already on every Pi install.
- The JSON detection is a pure function (`text -> parsed | undefined`) and
  the YAML rendering a pure function (`parsed -> lines`), so both are
  unit-testable without a TUI; the `renderResult` hook is a thin adapter over
  them.
- Optional, same ticket if cheap: a `renderCall` that shows the raw ws tool
  name and its key arguments (`session_key` elided) on the row header, since
  the sanitized `ws__` name is already the row label.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-bridge-tool-registration}`: add one bullet
stating that JSON-shaped text results are displayed as YAML in the TUI row via
`renderResult` while the tool result returned to the model is unchanged.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change and no change
  to the text the model receives.
- Rendering must never throw: a parse or serialize failure falls back to the
  raw text.
- Headless leads (`--mode rpc`, no TUI) are unaffected; the hook is never
  invoked there.

## Phases

### Phase 1: Shared renderResult with YAML display

Add the `yaml` dependency, the two pure functions, and the shared
`renderResult` in the bridge loop. Tests: a JSON object result renders as YAML
lines; a JSON array renders as YAML; prose and a bare JSON string render
unchanged; a malformed JSON-looking text renders unchanged; the collapsed view
holds at most 10 lines and the expanded view holds all; the `execute` return
for a JSON result is byte-identical before and after. Amend the spec bullet
under Spec Impact. Live check (owner-run): call `ws__config_resolve_agent`
with `format: "json"` in a Pi lead session and confirm the row shows YAML,
expands to the full document, and the model's next turn still parses the
fields.

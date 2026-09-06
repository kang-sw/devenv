---
title: Pi TUI renders JSON-shaped ws tool results as YAML while the model keeps receiving JSON
related:
  - 260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit
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

Second owner feedback, 2026-09-06, same session (absorbed here from
`260906-feat-ws-pi-tool-call-rows-show-input-summary-and-resolved-model`,
now dropped): when the lead calls `explore`, `ws-execute`, `ws-agent-spawn`,
`ws-agent-send`, or `ws-fork`, the row shows the tool name and then only the
output. What the model asked for is invisible: the exploration question,
the command and its prompt, the spawned agent's alias/title and prompt, and
which model and thinking level the child actually runs with. The owner does
not want debug dumps, but must be able to tell at a glance what input a
dispatch is running with, and for every subagent which model and effort
were actually selected. Cause: a registered tool without `renderCall` falls
back to `createCallFallback()`, which renders nothing but the bold tool
name; arguments appear only in the copy-to-clipboard text. No adapter
registration defines `renderCall`. The model and effort are not in the
arguments at all: they are resolved inside `execute` (`resolveExploreModel`
for `explore`; `spawnAgent` for the other three) and never surfaced, so a
`renderCall` over the arguments alone could not show them. Pi's `execute`
receives `onUpdate(partialResult)` and can stream `details` before it
returns; `renderResult` receives those `details` for partial and final
results alike, so the resolved model can be pushed into the row as soon as
it is known.

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

### Dispatch-tool rows (Phase 2)

- **Input summary via `renderCall`** on the five dispatch tools the lead
  calls from its own TUI: `explore`, `ws-execute`, `ws-agent-spawn`,
  `ws-agent-send`, `ws-fork`. Each renders one to three lines under the
  title from its own arguments, in a fixed per-tool shape: `explore` the
  `query`; `ws-execute` the `command` when given, then the `prompt` head,
  `complex` as a tag; `ws-agent-spawn` `alias`/`title` when given, the
  `system_prompt_path` basename, the `prompt` head, and the requested
  `model_name`/`model_effort`; `ws-agent-send` the target alias/id and the
  `message` head, `interrupt` as a tag; `ws-fork` the `prompt` head,
  `model_name`, `expects_commit` as a tag. Long free text is cut to a fixed
  row budget (two rows for prompts) with `…`; the full arguments stay in
  Pi's copy output. Streaming arguments (`context.argsComplete === false`)
  render whatever has arrived. Hooks never throw on partial or malformed
  arguments.
- **Resolved model/effort line, mandatory on every child-dispatching
  tool** (owner directive, 2026-09-06). Immediately after tier resolution
  the tool calls `onUpdate` with
  `details: {resolved: {tier, model, effort, inherited: boolean}}` and
  includes the same object in its final `details`; for `ws-agent-send` it
  is the target agent's recorded model/effort. The shared `renderResult`
  prints `→ <tier or "inherit"> · <provider/id> · effort <level>` before
  anything else, in every state: partial, success, error. `model` and
  `effort` are the effective values: `(inherited lead model)` when the tier
  did not resolve, and `effort: pi-default` when the dispatch applied no
  thinking level. `inherited` is read from the `rejected` detail that
  `260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit`
  adds to `resolveModelForAliasViaWsMcp`; that ticket lands first, and this
  phase adds no second inherit signal beyond the row line.
- **Resolution callback.** Only `explore` resolves inside its own
  `execute`; `ws-agent-spawn`, `ws-fork`, and `ws-execute` resolve inside
  `spawnAgent`, whose return carries no model. `spawnAgent` gains an
  optional `onModelResolved(resolved)` option, called right after
  resolution; the three tool bodies forward it to `onUpdate`, the non-tool
  caller in `ask.ts` passes none. If
  `260906-feat-ws-pi-lead-explore-as-async-rpc-child` lands first, the
  lead-side `explore` also resolves inside `spawnAgent` and takes the
  callback like the other three; whichever of the two lands second
  reconciles explore's wiring so it lives in `spawnAgent` once.
- **Body below the line.** Attaching `renderResult` replaces Pi's fallback
  (the ten-line cap), so Phase 1's shared result renderer (YAML for JSON,
  raw otherwise, row-budget trim) is applied to these five custom tools as
  well, with the resolved line prepended. Phase 1 writes the renderer as a
  reusable helper for that reason.
- One helper module (`src/tool-row-render.ts`, pure functions) holds the
  per-tool summary builders and the resolved-model line as structural
  `Component`s; registration sites only wire the hooks. `content` is
  untouched; `details` is UI-only in Pi's contract.
- Out of scope: the worker-side gated exec tool (`ws-worker-exec`) only
  runs inside a `--mode rpc` execute-worker, where Pi builds no tool row,
  and its command and rationale already reach the lead through the
  approval-request push. A headless `--mode rpc` lead never invokes these
  hooks either.
- Rejected: dumping the JSON arguments under the title (the debug text the
  owner does not want; the copy output already has it). Rejected: embedding
  the resolved model in the text `content` (grows the model's context for a
  human-only concern).

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-bridge-tool-registration}`: add one bullet
stating that JSON-shaped text results are displayed as YAML in the TUI row via
`renderResult` while the tool result returned to the model is unchanged.

Phase 2, recorded where each tool is documented rather than under the
bridge-registration anchor: `{#260903-pi-delegation-spawner-tools}` for
`explore`, `ws-agent-spawn`, and `ws-agent-send`;
`{#260905-pi-execute-approval-gateway}` for `ws-execute`;
`{#260905-pi-side-thread-fork-task-thread}` for `ws-fork`. Each gets one
sentence: the row carries an argument summary and a mandatory resolved
model/effort line, both display-only, with the resolution outcome published
through `details`.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change and no change
  to the text the model receives.
- Rendering must never throw: a parse or serialize failure falls back to the
  raw text.
- No static `@earendil-works/pi-tui` import anywhere in `bridge.ts` or its
  imports; `npm test` must keep passing with `bridge.test.ts` importing it.
- Headless leads (`--mode rpc`, no TUI) are unaffected; the hook is never
  invoked there.
- Phase 2 depends on Phase 1's helper and on
  `260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit`
  Phase 1 for the `rejected` resolver detail; do not start it before both
  land. Keep the `details` shape additive to whatever each tool already
  returns.

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

### Phase 2: Dispatch-tool input summaries and resolved model line

Depends on Phase 1 and on the tier-warning ticket's Phase 1. Add
`src/tool-row-render.ts` with the per-tool summary builders and the
resolved-model line, wire `renderCall` on the five dispatch tools, add the
`onModelResolved` option to `spawnAgent`, publish `details.resolved` via
`onUpdate` and the final result, apply Phase 1's shared result renderer to
these tools with the resolved line prepended, and amend the three spec
passages named for Phase 2 under Spec Impact. Tests: each builder's output
for representative and for empty/partial arguments; row-budget trimming with
`…`; a builder never throws on `undefined` arguments; the resolved line for
a tier hit, a `complex:true` inherit, a tier miss, and a dispatch with no
thinking level (`effort: pi-default`); the line is present on partial,
success, and error results; a long synchronous `explore` answer is trimmed
to the row budget below the line. Live check (owner-run): in a Pi session,
`explore`, `ws-execute`, and `ws-agent-spawn` rows show the input summary
while running and the resolved model line before the child finishes.

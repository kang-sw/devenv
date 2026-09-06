---
title: Pi TUI rows for adapter tools show the call's input summary and the resolved model/effort
related:
  - 260906-feat-ws-pi-tool-result-yaml-tui-rendering
spec:
  - pi-adapter-runtime
---

# Pi TUI rows for adapter tools show the call's input summary and the resolved model/effort

## Background

Owner feedback, 2026-09-06, from a Pi dogfood session: when the lead calls
`explore`, `ws-execute`, `ws-agent-spawn`, or `ws-fork`, the TUI row shows the
tool name and then only the output. What the model asked for is invisible:
the exploration question, the command and its rationale, the spawned agent's
alias/title and prompt, and in every case which model and effort the child
actually runs with. The owner does not want debug dumps; they want to be able
to tell, at a glance, "what input is this command running with".

Cause, in Pi's interactive tool row (`tool-execution.js`,
`ToolExecutionComponent`): a registered tool without a `renderCall` hook
falls back to `createCallFallback()`, which renders nothing but the bold tool
name. Arguments are never drawn for custom tools; they appear only in the
copy-to-clipboard text (`formatToolExecution`, pretty-printed JSON). Every
adapter registration (`spawner.ts`, `execute-gateway.ts`, `fork.ts`, `ask.ts`,
`goal-loop.ts`, `bridge.ts`) omits `renderCall`, so all of them show a bare
title.

The model and effort are not in the arguments at all. `explore` resolves the
`small` tier at execute time (`resolveExploreModel` → `resolveModelForAliasViaWsMcp`),
`ws-execute` resolves `small` unless `complex:true`, `ws-agent-spawn` and
`ws-fork` resolve `model_name` the same way, all inheriting the lead's model
on a miss. The resolution outcome exists only inside `execute` and is never
surfaced, so even a `renderCall` over the arguments could not show it.

Pi's contract gives the adapter what it needs. `ToolDefinition.renderCall(args,
theme, context)` draws the call half of the row and is display-only, like the
`renderResult` hook the related YAML ticket uses. `execute` receives
`onUpdate(partialResult)` and can stream `details` before it returns;
`renderResult(result, options, theme, context)` receives those `details` for
both partial and final results, with `options.isPartial` telling them apart.
`ToolRenderContext.state` is a per-row slot shared between call and result
renders. So the resolved model can be pushed into the row as soon as it is
known, while the child is still running.

## Proposed direction

- **Input summary via `renderCall`** on the adapter tools that dispatch work:
  `explore`, `ws-execute`, `ws-agent-spawn`, `ws-agent-send`, `ws-fork`, and
  the worker-side gated exec tool. Each renders one to three lines under the
  title, built from its own arguments, in a fixed per-tool shape:
  - `explore`: the `query` (first line, trimmed to width with `…`).
  - `ws-execute`: the `command` when given, then the `prompt` head; `complex`
    shown as a tag.
  - `ws-agent-spawn`: `alias` and `title` when given, the `system_prompt_path`
    basename, the `prompt` head, and `model_name`/`model_effort` as requested.
  - `ws-agent-send`: the target alias/id, then the `message` head; `interrupt`
    as a tag.
  - `ws-fork`: the `prompt` head, `model_name`, and `expects_commit` as a tag.
  - gated exec (`do-i-really-have-to-run-this-myself`-style worker gate): the
    `command`, then the `rationale`.
  Long free text is cut to a fixed number of rendered rows (two for prompts)
  with `…`; the full arguments stay in Pi's copy output. Streaming arguments
  (`context.argsComplete === false`) render whatever has arrived.
- **Resolved model/effort line via `details`.** Each of those `execute`
  bodies, immediately after tier resolution, calls `onUpdate` with
  `details: {resolved: {tier, model, effort, inherited: boolean}}` and
  includes the same object in its final `details`. A shared `renderResult`
  for these tools prints one leading line
  `→ <tier or "inherit"> · <provider/id> · effort <level>` (with `(inherited
  lead model)` when the tier did not resolve), then the normal result body.
  With the related YAML ticket's shared renderer, this line sits above the
  YAML/trimmed body; the two renderers compose rather than compete, so the
  leading line is a small helper both call.
- **One helper module** (`src/tool-row-render.ts`, pure functions) holds the
  per-tool summary builders and the resolved-model line, returning structural
  `Component`s (no `@earendil-works/pi-tui` import, which is not resolvable
  from the adapter's module graph). Registration sites only wire the hooks.
- Nothing about what the model receives changes: `content` is untouched,
  `details` is UI-only in Pi's contract.
- Rejected: dumping the JSON arguments under the title. That is the "debug
  text" the owner does not want, and the copy output already has it.
- Rejected: embedding the resolved model in the text `content`. It would grow
  the model's context for a purely human concern.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-bridge-tool-registration}`: record that
adapter-registered dispatch tools carry `renderCall`/`renderResult` display
hooks with an argument summary and a resolved-model line, both display-only,
and that tier resolution outcome is published through `details`.

## Constraints

- Adapter-only change under `agents-plugin-pi/`; no ws-mcp change.
- Display hooks never throw on partial or malformed arguments; Pi's fallback
  path would then render the bare title again, silently.
- The summary must fit within the row width and a fixed row budget so the
  transcript stays scannable during long goal runs.
- Keep the `details` shape additive to whatever each tool already returns.

## Phases

### Phase 1: Argument summaries and resolved-model line

Add `src/tool-row-render.ts` with the per-tool summary builders and the
resolved-model line, wire `renderCall` on the listed tools, publish
`details.resolved` from each `execute` via `onUpdate` and the final result,
and amend the spec passage under Spec Impact. Tests: each builder's output for
representative arguments and for empty/partial arguments; row-budget trimming
with `…`; the resolved line for a tier hit, a `complex:true` inherit, and a
tier miss; a builder never throws on `undefined` arguments; live check in a
Pi session that `explore`, `ws-execute`, and `ws-agent-spawn` rows show the
input summary while running and the resolved model line before the child
finishes.

# Unified Pi tool presentation

## Relevant Ticket Contract

Inline owner-approved work; no ticket creation or edits. Execute with one fresh
implementer and one fresh independent reviewer. The implementer owns focused
survey and implementation in one session instead of separate planning agents.

Converge MCP-bridged and ws Pi-native tool-call/result presentation through one
shared module so a style change applies uniformly. Caller-provided arguments are
shown as generic YAML, including model/effort arguments only when actually supplied.
Completed single-text JSON object/array results show YAML; other completed
single-text results show RAW text with the same compact preview and expansion.
Do not quote/serialize RAW prose as a YAML scalar or change model-facing content.

Retain the owner-accepted existing presentation: bold title; theme-normal light
input and gray output; uniform native parent lifecycle backgrounds; one input-owned
separator above and below input (unchanged native outer padding); trim outer input
display whitespace only; four-column input starts and three-column wrapped input
continuations relative to title; conservative ASCII=1/non-ASCII-codepoint=2 wrapping;
maximum ten content rows plus separate marker; input marker gray and four-column
indented, output marker unchanged/unindented; narrow fitting; no costly omitted-row
count; input remains capped when output expands, expanded output is complete.
Display-only control/tab sanitization and native terminal fitting remain.

All ws extension-owned native tool registrations should consume the common
presentation seam, including ws-agent-send/spawn/list/stop/transcript,
ws-execute/ws-approve, explore, ws-fork, direct read/run, and other ordinary
extension tools discovered in the focused survey. Preserve custom specialized
renderers if any exist rather than blindly overwriting them; report such cases.
Do not replace Pi built-in tools or third-party registrations. Keep renderer logic
out of individual registration sites. Shared registration wiring must not alter
tool names, schemas, execute callbacks, arguments/results/details, role gating,
approvals, delivery, subprocesses, lifecycle, or headless behavior.

## Out of Scope

No actual resolved-model/effort metadata, per-tool bespoke input summaries, dispatch
plumbing, push-message renderers, new ticket, shared rsrc/ws-mcp edits, migration
semantics, dependency redesign, release or automatic merge. Existing YAML Phase 2
ticket is context only, not implementation authority. Partial/error/image/mixed
results retain native fallback. Native-helper unavailability also retains fallback.

## Codebase Findings

Prior recon: shared createToolPreviewRenderers is connected only by src/bridge.ts
MCP registration loop. Native registrations in src/spawner.ts,
src/execute-gateway.ts and src/fork.ts lack its hooks. Direct read/run return RAW
text, which currently takes native fallback. Verify current source on starting
HEAD b60c7abb5361e530517d00032a2d3647d0a67249; work has landed since the earlier
618c187b rendering merge. Main helper: src/tool-result-render.ts, with renderer
and installed-host tests in test/tool-result-render.test.ts. Other registration
sites must be enumerated locally to make convergence complete without changing
tool behavior. Relevant spec: ai-docs/spec/pi-adapter-runtime.md.

Performance history: expensive grapheme/wrapping loops caused severe observed
lag; current codepoint wrapping must remain cached and bounded on collapsed
previews. Previous review noted a Minor repeated plain-row join on expanded
redraw: avoid amplifying it as RAW output support widens usage. Cache prepared
RAW text as well as physical/native layouts; theme restyling must not reparse or
rewrap unchanged source. No source-wide width scan merely to count omitted rows.

## Implementation Plan

1. Check Git state and create impl/track/pi-agent/unified-pi-tool-presentation
from track/pi-agent at b60c7abb5361e530517d00032a2d3647d0a67249 before source edits.
Preserve unrelated untracked plan(s). Read AGENTS.md, implementation playbook,
relevant Pi docs fully and their necessary cross-links, focused spec and migration
anchor if touching registration adapter boundaries. No additional delegates.
2. Complete the focused survey in this plan, list actual tool registration
coverage and selected common seam; commit plan before source edits. Escalate only
unresolved structural choices, not internal helper naming.
3. Reuse common renderer/style ownership; centralize hook provisioning without
bridging imports into a cycle or requiring static pi-tui imports. Extend completed
single-text fallback selection to preserve RAW text while applying previews.
4. Add tests first for pure preparation/selection; implement and run focused/full
adapter tests and available package build/type checks. Commit logical source/tests.
5. Return for one independent full-scope review. Same implementer handles relays
and final verification. Lead owns post-review spec changes. No merge.

## Verification Plan

Test actual registrations, not just helper calls: representative MCP tool,
ws-agent-send, ws-approve, explore, direct read/run and remaining native tools all
receive shared hooks while role visibility and execution/approval semantics stay
unchanged. Guard uniform coverage against new registration omissions. If a change
repeats across three or more sites: before example is pi.registerTool(definition),
after is the selected shared registration helper with the same definition; list
sites in survey, stop on nonmatching specialized registration rather than force it.

Test JSON objects/arrays vs RAW prose, malformed/scalar JSON as RAW, empty/long RAW,
Unicode, tabs/controls, literal dots, ten/eleven rows, expansion, narrow widths,
whitespace fidelity and original payload identity. Retain partial/error/image/mixed
native fallbacks, streaming argument tolerance, one separator and native ANSI
backgrounds. Probe installed Pi composition and cold helper availability for native
registration before async MCP startup completes. Verify no hard dependency on MCP
startup for native rendering. Prove unchanged large RAW redraw avoids repeat
preparation/wrapping and avoid new full-source joins on cached redraws; verify
source/width/expansion/theme invalidation. Read full command output before pass
claims. Owner-live reload/display/performance acceptance stays distinct from tests.

## Focused registration survey (2026-09-07)

Source surveyed at `b60c7abb5361e530517d00032a2d3647d0a67249`. The selected
common seam is a Pi-adapter-owned registration helper in
`src/tool-result-render.ts`: it loads native TUI helpers independently of MCP
startup and decorates only ws-owned tool definitions with the common call/result
renderers, preserving each definition's schema, execution callback, and any
specialized renderer already present. Bridge registration continues to provide
its sanitized per-tool title through this seam; native registrations provide
their existing `name` as the title. The helper's unavailable-host path returns
the definition untouched.

Actual native registration coverage (all use ordinary text results; no existing
specialized `renderCall`/`renderResult` was found):

- `src/spawner.ts`: `ws-agent-spawn`, `ws-agent-send`, `ws-agent-list`,
  `ws-agent-stop`, `ws-agent-transcript`, `ws-report-to-lead`, and the
  role-gated `explore`.
- `src/execute-gateway.ts`: `ws-worker-exec`, `ws-execute`, `ws-approve`,
  `do-i-really-have-to-read-this-myself`, and
  `do-i-really-have-to-run-this-myself`.
- `src/fork.ts`: `ws-fork`; `src/ask.ts`: `ws-ask`, `ws-resolve`; and
  `src/lead-skills.ts`: `ws-skill`.
- `src/goal-loop.ts`: `goal-achieved`, `goal-blocked`, and
  `goal-compact-and-continue`.

`src/bridge.ts` is the existing MCP loop and remains in scope. Commands,
message renderers, and Pi built-ins/third-party registrations are not tool
registrations and remain untouched. No specialized renderer blocks convergence.

## Escalations

No open owner decision. Stop if convergence requires protocol changes, specialized
renderer removal, broader registration interception outside ws-owned tools, or
source changes outside Pi adapter/spec scope. Scope is common presentation, not
resolution of actual models/effort or push messages. Merge requires new approval.

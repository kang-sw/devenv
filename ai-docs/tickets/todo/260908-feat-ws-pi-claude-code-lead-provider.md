---
title: "Claude Code provider for Pi: `claude-code/<model>` backed by the Agent SDK, Pi tools reflected as parked MCP handlers"
related:
  260908-research-ws-pi-claude-code-lead-provider: research and isolated spike (all three pass criteria met on 2026-09-08); this ticket implements the A′ design it settled on
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: the thin lead surface this provider is expected to carry; the provider reflects whatever `context.tools` Pi hands it and does not depend on that ticket landing first
  260907-bug-ws-pi-fork-first-call-prompt-cache-miss: a Claude-backed fork replays Pi's history into a new claude process; the Anthropic-side cache hit across processes (spike) is what keeps that affordable
spec:
  - pi-adapter-runtime
---

# Claude Code provider for Pi: `claude-code/<model>` backed by the Agent SDK, Pi tools reflected as parked MCP handlers

## Background

`260908-research-ws-pi-claude-code-lead-provider` established that the
owner's Claude subscription can drive a Pi lead through `claude -p`
(Agent SDK, `--input-format stream-json`) and proved the design in an
isolated spike: one claude process per Pi session, Pi's tools reflected
as in-process SDK MCP tools whose handlers park until Pi executes the tool
and calls `streamSimple` again, Pi keeping ownership of history. Three
assistant calls ran in one process, Pi executed both tools through its own
loop, `pi -p` exited 0, and Anthropic's prompt cache hit across claude
processes for an identical prefix.

Owner direction (2026-09-08): implement it now, in a separate worktree in
parallel with the lead-profile work, rather than waiting for that ticket.
The provider is used while the current unified-subscription billing state
holds ("enjoy the gray zone"), and must be swappable back to an OpenAI
tier by configuration alone.

## Decisions

### Registration and model surface

- New module `agents-plugin-pi/src/claude-code-provider.ts`, registered
  from `index.ts` at extension load in every Pi process that loads the
  extension (lead, fork, ask, workers). Registration is cheap; the claude
  process starts lazily on the first `streamSimple` call, so a child whose
  tier points elsewhere never spawns one.
- Provider name `claude-code`. Models: `opus`, `sonnet`, `haiku`, ids
  passed straight through as the SDK `model` option (the spike resolved
  `sonnet` to `claude-sonnet-5`). All three `reasoning: true`, `input:
  ["text"]`, zero `cost` rates (subscription-metered, not dollar-billed),
  `contextWindow` and `maxTokens` per the current Anthropic model docs.
  `apiKey` is a fixed placeholder so `hasConfiguredAuth` reports true and
  the tier resolver accepts `claude-code/<id>` without a Pi login flow;
  real auth is the `claude` CLI's own login (`claude auth status`).
- Selecting it: Pi `defaultProvider`/`defaultModel`, `/model`, or an
  `agents.tier` entry `claude-code/<id>` for harness pi. Nothing in the
  adapter special-cases the provider by name outside this module.

### One claude process per Pi session, Pi owns history

- `query({prompt: <push-queue async iterable>, options})` with
  `systemPrompt: context.systemPrompt`, `tools: []`, `mcpServers: {pi:
  createSdkMcpServer(...)}`, `settingSources: []`, `strictMcpConfig:
  true`, `persistSession: false`, `permissionMode: "bypassPermissions"`
  plus `allowDangerouslySkipPermissions: true`, `model`, `effort` mapped
  from `options.reasoning`, `cwd: process.cwd()`, `stderr` captured to the
  adapter's diagnostics. Claude Code's built-in tools, CLAUDE.md, settings,
  hooks, and the account's connector MCP servers are all off; Pi's system
  prompt already carries AGENTS.md and the ws block.
- Thinking: Pi `ThinkingLevel` → SDK `effort` (`off` → thinking disabled,
  `minimal`/`low` → `low`, then `medium`/`high`/`xhigh`/`max` one-to-one).
  Thinking blocks in SDK `assistant` messages map to Pi `thinking` content.
- Every Pi tool in `context.tools` becomes an SDK MCP tool (JSON schema →
  zod shape; nested objects/arrays pass through as `z.any()` rather than
  being dropped). The handler ends the current Pi turn with `stopReason:
  "toolUse"` and parks a resolver keyed by the `tool_use` id. Pi's loop
  executes the tool (hooks, wrappers, gateway, `addedToolNames`, TUI all
  intact). The next `streamSimple` call whose trailing messages are
  `toolResult`s resolves the matching parked handlers with the result
  text; Claude continues in the same process. Parallel `tool_use` blocks
  arrive as one SDK `assistant` message per block; each becomes its own
  Pi turn, in order.
- A trailing `user` message (a new Pi turn) is pushed to the process as an
  SDK user message. Multi-turn in one process is the normal path.
- Usage: sum each SDK `assistant` message's `input_tokens`,
  `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`
  into the Pi turn's `Usage` so Pi's context/cache displays and the fork
  cache notice keep working; cost stays zero.
- The `ProviderConfig.streamSimple` contract: call `options.onPayload`
  before sending (with the payload the provider is about to hand the SDK,
  honoring a returned replacement) and `options.onResponse` after the
  first SDK message of the turn, matching built-in providers.

### Resync: divergence restarts the process with a replay

`streamSimple` continues the live process only when all hold: the new
`context.messages` extends the last-seen message list by appended
`toolResult`s and/or one `user` message; `systemPrompt` is unchanged; the
tool name set is unchanged; model id and effort are unchanged. Otherwise
(Pi compaction, `--fork` child start, history edit, a tool set that grew
through the deferred-load channel, a `/model` or thinking change) the
provider closes the process and starts a new one, replaying Pi's prior
history as a transcript block in the first user message (assistant text,
tool calls with arguments, tool results, prior user turns, in order) and
then the trailing new message. The SDK accepts only user input, so prior
assistant turns cannot be re-injected natively; the replay is the accepted
lossy path, and its byte-stability across identical histories is what
lets Anthropic's cache absorb it. Log every resync with its reason.

### Lifecycle and failure

- `session_shutdown` → `Query.close()`; the process must not outlive the
  Pi session (the spike's `pi -p` hung without this).
- `options.signal` abort → `Query.interrupt()`; the current Pi turn ends
  with `stopReason: "aborted"`; any parked handler for that turn is
  rejected so the SDK side sees a tool error instead of hanging. The
  process is kept; the next call's extension check decides continue vs
  resync.
- Process start or SDK error (not logged in, binary missing, transport
  failure) surfaces as a Pi error turn whose message names the cause and
  the remedy (`claude auth login`, install Claude Code, switch the tier
  back). No silent fallback to another provider.
- A `result` with `is_error` ends the turn as `error` carrying the SDK's
  error text.

### Rejected

- Per-turn `claude -p --resume` (no tool reflection, process startup per
  call). External stdio MCP server calling back into Pi (one more process
  and transport). Claude Code as host with Pi as child (retired direction).
- Waiting for the lead-profile ticket: the provider reflects whatever tool
  list Pi passes, so the surface is orthogonal.

## Constraints

- Adapter-only: `agents-plugin-pi/` changes; no ws-mcp Go, shared rsrc,
  or playbook edits authored on the Pi track.
- New dependencies in `agents-plugin-pi/package.json`:
  `@anthropic-ai/claude-agent-sdk` (0.3.x, the spike used 0.3.263), `zod`
  (the SDK's `tool()` takes a zod shape), and `@earendil-works/pi-ai`
  pinned to the version nested under the installed `pi-coding-agent`
  (0.84.4 today) for `createAssistantMessageEventStream`, unless
  `pi-coding-agent`'s public entry already exports it. Bare imports only;
  no absolute paths.
- The SDK is treated as a protocol client, not a policy surface: the
  `claude` CLI is the thing being wrapped, and the provider must keep
  working if the SDK is swapped for a hand-rolled stream-json client.
- Keep the module testable without a claude binary: the SDK entry points
  (`query`, `createSdkMcpServer`, `tool`) are injected through a seam the
  tests replace with a scripted fake; live tests run only under
  `WS_PI_LIVE_CLAUDE=1`.
- Do not touch the running dogfood worktree's `agents-plugin-pi` package;
  implement on an `impl/*` branch in a separate worktree.

## Spec Impact

Add a section to `ai-docs/spec/pi-adapter-runtime.md` (sibling of the
model-resolution sections under the delegation spawner) describing the
`claude-code` provider: registration and model ids, the one-process-per-
session/parked-handler contract, the resync rule and its lossy replay, the
lifecycle (shutdown/abort/error) rules, usage mapping, and the explicit
non-goals (no Claude Code built-in tools, settings, or connector MCPs; no
adapter special-casing outside the module). Cross-reference
`260903-pi-spawner-model-tier-inherit` for how a tier selects it.

## Phases

### Phase 1: Provider core with block-level streaming, tool round trip, resync, lifecycle

Implement the module and its registration, dependencies, the SDK seam, the
parked-handler round trip, multi-turn continuation, thinking and usage
mapping, effort mapping, the resync rule with transcript replay, shutdown,
abort, and error surfacing. Emit content at block granularity (one
`text_start/delta/end` per SDK text block, as in the spike); live partial
deltas are Phase 2. Update the spec section.

Verification must cover:

- Fake-SDK unit tests: text-only turn; single tool round trip (handler
  parks, Pi turn ends `toolUse`, next call resolves by `tool_use` id);
  two parallel `tool_use` blocks become two sequential Pi turns and both
  resolve; a second user turn reuses the process; thinking block mapping;
  usage summation; effort mapping for every Pi thinking level; each resync
  trigger (compaction-shaped history, changed system prompt, grown tool
  set, changed model/effort) closes the old process, starts a new one, and
  replays the prior history in order; abort rejects the parked handler and
  ends the turn `aborted`; `session_shutdown` closes; start failure yields
  an error turn naming the remedy; `onPayload`/`onResponse` are called in
  the required order.
- Live gate (`WS_PI_LIVE_CLAUDE=1`, subscription login): `pi -p` with
  `--no-extensions --no-skills --no-context-files --no-builtin-tools`
  plus this extension on `claude-code/sonnet`, the spike's two-tool
  prompt, exit 0 with the correct answer and one claude process; a
  parked handler that waits at least three minutes before Pi returns the
  result still resolves (MCP handler timeout check); `cacheRead > 0` on
  the second run of an identical prompt.
- Owner-run dogfood: a ws lead session on `claude-code/opus` with the
  full ws tool surface; `explore`, `ws-fork`, and `ws-ask` each round-trip;
  a `/model` switch back to the OpenAI tier mid-session resyncs without
  losing history; the adapter test suite passes.

### Phase 2: Live partial streaming and in-place tool-set growth

Depends on Phase 1. Enable `includePartialMessages` and map SDK
`stream_event` deltas (`content_block_start/delta/stop` for text,
thinking, and `input_json_delta`) onto Pi `text_delta`/`thinking_delta`/
`toolcall_delta` so the TUI renders live; keep the block-level path as
the fallback when partials are absent. Replace the grown-tool-set resync
with in-place MCP tool registration plus `list_changed` if the SDK server
supports it; otherwise record why and keep the resync. Investigate
`Query.setModel` and an effort setter for model/thinking changes without
a restart.

Verification: fake-SDK tests for delta ordering across interleaved text
and tool_use blocks; live gate showing incremental rendering in `pi`
TUI; owner-run dogfood of a deferred-tool load (fork-only tools) without
a process restart, or the documented resync fallback.

## Implementation checkpoint - 2026-09-08

Phase 1 is implemented on `impl/track/pi-agent/claude-code-provider` in a
separate worktree: provider, tests, dependencies and the `index.ts`
registration in `98a5306f`; spec section
`{#260908-pi-claude-code-provider}` and this checkpoint in the follow-up
docs commit. Base is `9e3cb14b` (the ticket-opening commit on
`track/pi-agent`). Not merged, not pushed.

Fake-SDK suite (`test/claude-code-provider.test.ts`): 28/28 pass, covering
every Phase 1 list item. Full `npm test` in `agents-plugin-pi/`: 1292 tests,
1162 pass, 129 fail, 1 skipped (the live gate). Every failure is
pre-existing and environment-bound: `fork-prefix.integration`,
`fork-lifecycle.integration` and `fork-native-transitions` hard-code the
owner's Linux global SDK path
(`/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent`),
which does not exist on the macOS machine this ran on; the `local-0.84.4`
variants of the same tests pass. No provider-related failure.

Live gate run once (`WS_PI_LIVE_CLAUDE=1`, `claude-code/sonnet`, isolated
`pi -p`, throwaway root, subscription login, 185 s parked wait): pass in
194.6 s. First run: exit 0 in 189.8 s, answer
`extra_a says: alpha-7731 extra_b echoes: kiwi`, one claude process
(model resolved `claude-sonnet-5`, MCP server `pi` connected), no resync,
stop reasons `toolUse, toolUse, stop`; the `extra_a` handler parked
10:19:04 -> 10:22:09 and still resolved (no MCP timeout). Usage per call:
1 `{input:2, output:23, cacheRead:0, cacheWrite:1889}`, 2 identical (both
`tool_use` blocks belong to the same API message), 3 `{input:2, output:1,
cacheRead:1889, cacheWrite:225}`. Second identical run: exit 0 in 4.8 s,
call 1 `cacheRead:1889, cacheWrite:0`, call 3 `cacheRead:2114`. The MCP
request `_meta` key carrying the `tool_use` id is `claudecode/toolUseId`.
An ad-hoc SDK check confirmed `Query.interrupt()` yields a trailing
`error_during_execution` result, which the abort path discards.

Deviations from the plan text, recorded in the commit `## AI Context`:
usage summed once per SDK `message.id` rather than per frame; handler
pairing by `_meta` id with a per-name FIFO fallback; `@earendil-works/pi-ai`
imported from its main entry (no subpath export); `alwaysLoad: true` on the
MCP server; the live gate shares one cwd across runs because Claude Code
folds cwd into the cached prefix.

**Remaining, owner-run:** the Phase 1 dogfood (a ws lead session on
`claude-code/opus` with the full ws tool surface; `explore`, `ws-fork`
and `ws-ask` round-trips; a mid-session `/model` switch back to the
OpenAI tier resyncing without history loss) and the merge decision. No
`### Result` is recorded; Phase 2 is untouched.

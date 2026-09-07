---
title: "Claude Code as a Pi lead model: a custom Pi provider backed by the Agent SDK (`claude -p` stream-json)"
related:
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: the thin lead surface this provider would carry; orchestrator/worker stay on Pi-native providers, only the lead's model changes
  260907-bug-ws-pi-fork-first-call-prompt-cache-miss: a Claude-backed lead re-pays every fork as a full replay on the Claude side (subscription limits, not dollars); the deferred-tool channel there does not apply to this provider
  260802-research-ws-pi-native-framework: Pi as the framework host; this ticket keeps that and only swaps the lead's model backend
---

# Claude Code as a Pi lead model: a custom Pi provider backed by the Agent SDK (`claude -p` stream-json)

## Background

The owner's Claude subscription cannot be used from Pi's built-in providers,
but Claude-tier discussion quality is what the lead role wants most: with
`260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role` the lead becomes
a discuss/decide/spawn seat with ~20 tools, and that is exactly where model
judgment matters and where token volume is lowest. The question is whether
`claude -p` (Claude Code headless mode, subscription-authenticated) can be
wrapped so Pi sees it as a model, without moving the Pi framework layer
(goal loop, fork, ask, widget, spawner) anywhere.

Owner stance (2026-09-08): use it while the current billing state holds
("enjoy the gray zone"); the provider must be swappable back to an OpenAI
tier by configuration alone.

## Billing and terms as documented (checked 2026-09-08)

- Anthropic support article "Use the Claude Agent SDK with your Claude plan"
  (support.claude.com, article 15036540): *"Claude Agent SDK, `claude -p`,
  and third-party app usage still draw from your subscription's usage
  limits."* The planned split — a separate monthly Agent SDK credit per plan
  (Pro $20, Max 5x $100, Max 20x $200, Team $20/$100) — was **paused on
  2026-06-15** with no restart date; Anthropic says it is "working to update
  the plan to better support how users build with Claude subscriptions."
- Third-party harness use: OAuth was restricted to first-party products in
  March 2026, then in May 2026 Anthropic announced permitting third-party
  agent use via that separate credit (press coverage; no standalone policy
  document in the official docs). The authentication docs
  (code.claude.com/docs/en/authentication) do not address third-party
  harnesses.
- Consequence for design: today the lead's usage would come out of the
  subscription limit; if the split resumes, it moves to the (much smaller)
  SDK credit. The provider must therefore be one tier-config line away from
  an OpenAI fallback, and the lead must stay thin.

## The Agent SDK is a process wrapper over `claude -p`

Verified locally: `@anthropic-ai/claude-agent-sdk` 0.3.263 exposes
`query({prompt: string | AsyncIterable<SDKUserMessage>, options})`,
`createSdkMcpServer({name, tools: [tool(name, description, zodShape,
handler)]})` — the handler runs **in the SDK consumer's process** — and
`canUseTool`. The `claude` binary itself carries the stdio control protocol
these ride on (`control_request`/`control_response`/`mcp_message`/
`can_use_tool` strings in the binary; `--input-format stream-json` is a CLI
flag). So SDK and bare `claude -p --output-format stream-json` are the same
process, same auth, same billing; the SDK only saves re-implementing the
control protocol, which is thinly documented and CLI-version-coupled. Use
the SDK as the protocol client; do not treat it as a policy difference.

`claude auth status` on the dev machine: `loggedIn: true, authMethod:
"claude.ai", apiProvider: "firstParty"` — subscription login, no API key.

## Pi side: `pi.registerProvider(name, { streamSimple, models })`

Pi's extension API registers a provider with a custom
`streamSimple(model, context, options) => AssistantMessageEventStream`
(`core/extensions/types.d.ts`, `ProviderConfig`). `context` is
`{systemPrompt, messages, tools}`; the stream must emit `start`, then
`text_*`/`thinking_*`/`toolcall_*` events, then `done` with a final
`AssistantMessage {content, usage, stopReason, ...}`. Models are declared
with `id`, `reasoning`, `input`, `cost`, `contextWindow`, `maxTokens`; the
tier map (`agents.tier`) can point at `claude-code/<id>` like any provider.
Registration takes effect without a Pi reload.

## Design sketch (A′)

One `claude` process per Pi session, owned by the provider module:

1. **Process.** `query({prompt: <async iterable of SDKUserMessage>, options:
   {systemPrompt: context.systemPrompt, tools: [], mcpServers: {pi:
   createSdkMcpServer(...)}, settingSources: [], persistSession: false,
   includePartialMessages: true, model, thinking/effort from Pi's thinking
   level, cwd}})`. Built-in tools off; CLAUDE.md/settings/hooks off (Pi's
   system prompt already carries AGENTS.md).
2. **Tool reflection.** Every Pi tool in `context.tools` becomes an SDK MCP
   tool whose handler *blocks*: it hands the call to the provider, which
   emits `toolcall_start/delta/end` + `done(stopReason: "toolUse")` to Pi.
   Pi's own loop executes the tool (hooks, wrappers, TUI, `addedToolNames`
   all intact). The next `streamSimple` call carries the `toolResult`
   message; the provider matches it to the parked handler and resolves it,
   so Claude continues in the same process. Pi remains the owner of history;
   Claude's session is a shadow.
3. **Resync.** If a `streamSimple` call's messages are not an extension of
   the last-seen prefix (Pi compaction, `--fork`, history edit, tool set
   changed in a way MCP cannot express), the provider drops the process and
   starts a new one, replaying Pi's full history as one prompt. Correct,
   expensive, rare.
4. **Usage.** Map `SDKResultMessage.modelUsage`/`usage` to Pi's `Usage`
   (input, output, cacheRead, cacheWrite) so Pi's cost/cache displays keep
   working; `total_cost_usd` is an estimate, subscription usage is not
   dollar-billed.
5. **Fallback.** Model id set in `agents.tier`; switching the lead back to
   an OpenAI tier is a config change only.

Rejected: wrapping bare `claude -p` per turn with `--resume` (loses tool
reflection, history ownership, and pays process startup every call);
exposing Pi tools through an external stdio MCP server that calls back into
Pi over a socket (same inversion, one more process and transport to fail);
using Claude Code as the *host* and Pi as a child (retired direction:
`260723-refactor-fork-removal-prefer-subagent`, Pi-native framework anchor).

## Known costs and open questions

- `--fork` from a Claude-backed lead is a full replay on the Claude side
  (the 260907 Pi-side fix does not apply). The spike showed Anthropic's
  prompt cache *does* hit across claude processes for an identical prefix,
  so a replay costs cache-read, not full input, as long as the provider
  replays byte-identically. Subscription limits absorb it; if the SDK
  credit split resumes this becomes the dominant cost of the lead.
- Pi's `additional_tools` deferred-load channel is meaningless for this
  provider; MCP `tools/list` changes are the equivalent and the SDK server
  instance is created per process, so tool-set growth mid-session needs
  either `list_changed` support in the SDK server or a resync.
- Thinking level mapping (Pi `ThinkingLevel` → SDK `thinking`/`effort`),
  and whether `stream_event` partials give Pi enough for live rendering.
- Whether the SDK MCP handler can stay parked across Pi turns without the
  CLI timing it out; measure.
- Interrupt/abort: Pi's `signal` must map to `Query.interrupt()` and the
  parked handler must fail cleanly.

## Spike (isolated, 2026-09-08)

Plan: a throwaway Pi extension in the scratchpad registers provider
`claude-code` with one model, reflects two tools (one of them exercised),
and runs `pi -p` in a scratch cwd with `--no-extensions --no-skills
--no-context-files --no-builtin-tools`. Pass criteria: (1) a text-only
turn returns; (2) a tool-call round trip completes with Pi executing the
tool and Claude continuing in the same process; (3) a second Pi turn reuses
the process. Result recorded below.

### Outcome: all three pass (isolated run, Pi 0.85.1, SDK 0.3.263, subscription login)

Extension: `pi.registerProvider("claude-code", {streamSimple, models:
[{id: "sonnet"}]})`; one `query()` per Pi session with a push-queue as the
streaming-input prompt, `tools: []`, `settingSources: []`,
`strictMcpConfig: true`, `persistSession: false`, `permissionMode:
"bypassPermissions"`; Pi's two spike tools reflected via
`createSdkMcpServer` with parking handlers (resolved by tool name, FIFO);
`session_shutdown` calls `Query.close()`.

Prompt: "call extra_a, then extra_b(word=kiwi), then reply with both
strings". Pi's session file recorded three assistant calls from one claude
process; `pi -p` exited 0 with the correct final line.

| Pi call | Claude side | stopReason | usage (input / cacheRead / cacheWrite) |
|---|---|---|---|
| 1 | assistant `tool_use extra_a` → MCP handler parks | `toolUse` | 2 / 1,929 / 0 |
| 2 | Pi executed `extra_a`; handler resolved; next block `tool_use extra_b` | `toolUse` | 2 / 1,929 / 0 |
| 3 | Pi executed `extra_b`; handler resolved; text | `stop` | 2 / 2,154 / 0 |

- Round trip works exactly as sketched: the handler ends Pi's turn with
  `toolUse`, Pi runs the tool through its own loop, the following
  `streamSimple` resolves the parked handler, Claude continues without a
  new process. `result` reported `num_turns: 3`, one process.
- **Anthropic-side prompt cache hits across processes.** The first run
  (same prompt, earlier) wrote 1,929 cache tokens; this run's very first
  call read them back (`cacheRead 1,929, cacheWrite 0`). So a Claude-backed
  lead keeps its cache across Pi restarts of the same context, and a
  resync/replay is cheaper than assumed as long as the prefix is identical.
- Parallel tool calls arrive as one SDK `assistant` message per
  `tool_use` block, in sequence, each followed by its handler invocation;
  treating each block as its own Pi turn gives Pi a serial
  toolCall/toolResult history while Claude's shadow history holds them as
  one message. Harmless as long as Pi owns history.
- `settingSources: []` alone still attached the account's claude.ai
  connector MCP servers (Drive/Gmail/Calendar, `needs-auth`);
  `strictMcpConfig: true` removed them. Required for the real provider.
- Without `Query.close()` on `session_shutdown` the `pi -p` process never
  exits (the claude child holds the loop). Fixed in the spike; the real
  provider must also close on `session_shutdown` and on Pi abort signals.
- Model resolution: Pi model id `sonnet` passed straight through as the SDK
  `model` option resolved to `claude-sonnet-5`; `SDKResultMessage.
  total_cost_usd` was $0.003 (estimate; subscription-metered, not billed).
- Not exercised: a second *user* turn in one Pi session (`-p` has one),
  thinking-level mapping, `stream_event` partials for live rendering,
  interrupt/abort, MCP tool-set growth mid-session, resync on Pi
  compaction/fork. These are the feature ticket's Phase 1 items.

Verdict: the A′ design is viable; promote to a feature ticket once the
lead-profile ticket lands, since the provider only needs to reflect the
thin lead surface. Spike artifacts live in the session scratchpad
(`spike-claude-provider/ext.ts`), not in the repo.

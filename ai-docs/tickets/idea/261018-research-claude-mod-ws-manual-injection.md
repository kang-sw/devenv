---
title: Inject the ws workflow manual and dedupe playbook re-reads from the Claude mod
related:
  261017-feat-claude-mod-lead-compaction-timing: the single Claude mod this feature would join; its compaction observer and lever are the reset points for dedupe state
  261018-research-claude-mod-ws-mcp-reconnect: the other pending feature for the same mod; sequencing between the two is open
---

# Inject the ws workflow manual and dedupe playbook re-reads from the Claude mod

## Background

pi already gives a ws lead three things the Claude side lacks: the static
workflow manual sits in the system prompt (`lead-bootstrap.ts`
`staticManualPart`), a `workflow_manual` call returns only the session state
behind a one-line pointer (`bridge.ts` `prependWorkflowStateLine` /
`cutStaticBody`), and repeat `playbook.read` / `workflow_manual` reads whose
text is unchanged collapse to a pointer (`playbook-read-dedupe.ts`). The user
asked (2026-10-18) whether the Claude mod can do the same, scoped down to:

- auto-inject the workflow manual prefix, only where a ws project is detected
  (`ai-docs/WORKFLOW.md`, which `lead-bootstrap` ships to downstream projects);
- `workflow_manual` calls return the state only, as on pi;
- dedupe `playbook_read` re-reads until compaction.

Session-key omission (auto-filling `session_key`) was considered and dropped
from scope by the user.

A 2026-10-18 probe answered the mechanism on macOS with Claude Code 2.1.302
(haiku-5-5, one sonnet-5-5 turn): a copy of `agents-plugin` with a probe mod as
its single hooks module, headless and interactive (tmux), with a logging
reverse proxy behind `ANTHROPIC_BASE_URL` recording each request's system
blocks as ground truth. Logs: `/tmp/manual-inject-probe.lo2Xu0/runs/`
(volatile).

## Probe results

**The main-thread system prompt is snapshotted once per conversation.**
`prompt.compose` runs once per main-thread turn (not per API request), plus
for the compaction summary request, task-notification turns, `/context`
(traits `lean, analysis, skills`), and once at headless startup. Its answer is
used only for the conversation's first request: later answers are discarded
(proxy shows the first nonce on every later request). Not refreshed by a hot
reload of the module, `$.ui.invalidate("prompt.section")`, a new section id,
`/reload-plugins`, or `/model`. Refreshed by the first request after manual
`/compact`, a plugin `$.session.compact()`, or `/clear`. The snapshot is
recorded in the transcript as a `prompt_snapshot` attachment; a `--resume`d
session in a new process sends the recorded snapshot and ignores its own
compose.

**Subagents.** Named subagents (general-purpose, plugin agents such as
`ws:effort-low`) get no compose and their own system prompt; a compose
section cannot leak into them. A fork (`subagent_type: "fork"`) reuses the
parent's snapshot byte for byte, section included.

**First-compose race.** The first compose can run before an un-awaited
`$.mcp.call` started in `session.start` resolves (28 ms after start with a 4 s
artificial delay), and a conversation that misses it stays without the
section until compaction. Awaiting the pending fetch inside compose works;
the compose hook budget is 10 s and an awaited promise counts against it.
Normal fetch latency was ~80-700 ms.

**Tool-result rewrite.** In `tool.call` for `mcp__plugin_ws_ws__*`,
`next(e)` resolves to `{ ref, result: [{type: "text", text}], text }`. The
model sees a rewritten result for `{ result: "<string>" }` (after `next` or in
place of it) and `{ result: [{ type: "text", text }] }`. Spreading the core
result keeps `ref` and the original text wins; `{ result: { content } }`
fails output validation. `agentId` is present on subagent calls and absent on
the main thread. The mod's own `$.mcp.call` does not pass through its own
`tool.call` hook.

**Cache.** A stable session section keeps cache reads high turn to turn
(~23.6k read / ~150 created). The section lands inside the single system
block (`cache_control` 1h); a 5 KB manual added ~2.8k created tokens on a cold
first request and did not disturb the ~9.2k cross-session prefix.

**Project detection.** `$.fs.exists` works from `session.start` and compose,
but `$.session.root()` is the launch directory, not the repository root
(false from a subdirectory). `(await $.session.repo())?.root` and
`$.fs.ancestors({ names: ["ai-docs/WORKFLOW.md"] })` both work from a
subdirectory.

**Lifecycle.** `/clear` fires `session.end` (reason `clear`) and no new
`session.start`. `$.session.compact()` from `prompt.submit` (even inside
`$.clock.after(0)`) is refused; from `turn.complete` it works.

## Outcome Ledger

### Verified Findings

- `prompt.compose` answers reach the API only at a conversation's first
  request and after compaction or `/clear`; a resumed session replays its
  recorded snapshot (probe i1, r01, r06; proxy ground truth).
- Named subagents run no compose and cannot see a compose section; forks
  inherit the parent snapshot (i1, r06, r07).
- An un-awaited first-compose fetch can miss the snapshot; awaiting it inside
  compose within the 10 s budget fixes it (r04, r05).
- `tool.call` can replace a ws MCP result with `{ result: string | text[] }`
  without spreading the core result; `agentId` marks subagent calls (r08, r09).
- `$.session.root()` is the launch directory; `$.session.repo().root` and
  `$.fs.ancestors` find the repository root (r11-r13).

### Confirmed Decisions

- Scope: manual prefix injection gated on `ai-docs/WORKFLOW.md`,
  state-only `workflow_manual`, playbook re-read dedupe until compaction;
  session-key omission dropped (user, 2026-10-18).
- Derive the actionable feature `261018-feat-claude-mod-ws-manual-injection`; the user approved this ledger's Proposals there, with refinements recorded in its Decisions (user, 2026-10-18).

### Proposals

- Fetch the static manual (`playbook_read` `lead-workflow-manual`) at
  `session.start`, keep it at module level, and await it inside compose
  bounded below the remaining budget; re-fetch lazily when empty (no
  `session.start` after `/clear`).
- Keep the section text deterministic, with framing that names it as the ws
  workflow manual: an updated manual reaches a conversation only after
  compaction or `/clear`.
- No subagent filter in compose; dedupe and the `workflow_manual` rewrite run
  only on main-thread calls (`agentId === undefined`).
- Reset dedupe state on `session.compact` (landed), `session.end`, and
  directly when the mod's own lever compacts (its own `session.compact` hook
  is skipped).
- Gate on the repository root from `$.session.repo()` (or `$.fs.ancestors`).
- ws-only, under the same package gate as compaction timing.

### Open Questions

- Does auto compaction refresh the snapshot as manual and plugin compaction
  do (inferred, not triggered)?
- What happens when compose overruns its 10 s budget (declarations: a failed
  hook passes through, so the snapshot would lack the section)?
- How does a real-key `workflow_manual` result split into manual and state;
  would mapping it to `workflow_state` be simpler than cutting the text?
- In r02 haiku described the appended text as "in a user message" and
  declined to follow it; does opus with the full (non-lean) prompt treat the
  section as system prompt reliably?
- Teammate (`teammate` trait) and agent-team behavior; Windows.
- Is there any re-snapshot trigger short of compaction or `/clear`
  (`prompt.context` / `prompt.attachment` invalidation untried)?

---
title: "agy 커스텀 에이전트: hosted agy lead의 Antigravity 기본 시스템 프롬프트 교체"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-antigravity-adapter: "its custom-agent removability verdict (08)=c rejected this direction; (127)=a reopens it as a probe"
  261117-feat-ws-hosted-prompt-assembly: "Phase 2 edits the agy lead row of the harness catalog; a custom agent would change how the agy lead receives the rendered manual"
  261115-research-ws-unified-agent-tool-surface: "dropped; holds the earlier agy probe of excludeDefaultComponents, tools: [] and enable_mcp_tools"
---

# agy custom agent: replace Antigravity's default system prompt for hosted agy leads

## Background

Users report, as general reputation rather than a concrete case, that
Antigravity's (agy) default system prompt yields poor results. An agy custom
agent replaces that default prompt. `261117-feat-ws-hosted-antigravity-adapter`
probed this and recorded verdict (08)=c
(`ai-docs/tickets/.done/261117-feat-ws-hosted-antigravity-adapter.md#L330-L375`):

- the custom agent body replaces agy's default prompt (measured 9135 to 3748
  tokens);
- a `tools` allowlist drops the session's MCP tools, and `mcpServers` restores
  them only through the lazy `call_mcp_tool` gateway;
- a malformed agent config silently falls back to the default agent;
- the agent file must live in the workspace or the agy home;
- the recommendation then was that the hosted lead keeps agy's default agent
  and its subagent tools.

The earlier unified-tool-surface probe of `excludeDefaultComponents`,
`tools: []` and `enable_mcp_tools` is at
`ai-docs/tickets/.dropped/261115-research-ws-unified-agent-tool-surface.md#L634-L669`.

Owner (127)=a, 2026-11-18, reopens the direction: start with a probe, then
settle the design from its results.

## Probe questions

1. Does a body-only custom agent, with no `tools` allowlist, keep the session's
   ws MCP tools as direct tools rather than only through `call_mcp_tool`?
2. What does `excludeDefaultComponents` remove, and does the hosted lead still
   work (file edits, shell, subagents, ws MCP calls) with it set?
3. Where must the agent file live for a hosted agy lead (workspace versus agy
   home), and how does the daemon detect the silent fallback to the default
   agent so a malformed file fails loudly instead?
4. Quality A/B: run the same task on a hosted agy lead with the default agent
   and with the custom agent, and compare ws procedure compliance (playbook
   reads, ticket and commit flow) and the coding result side by side.

## Outcome Ledger

### Verified Findings

- (08)=c probe facts listed in `## Background`, from
  `261117-feat-ws-hosted-antigravity-adapter`.
- Probes 1-3, agy 1.5.10, 2026-11-18 (direct ACP:
  `agy acp --process-per-session --gemini-dir <scratch> --agent <name>`,
  isolated state, a scratch MCP server standing in for ws; scratch evidence
  under `/tmp/agy-cap2/`, not tracked). Question 4 was not run.
  - Q1: a valid custom agent (frontmatter with `name` and `description`, no
    `tools` allowlist) keeps session-injected MCP tools as direct
    `mcp_ws_*` calls, with and without `excludeDefaultComponents: true`; no
    `call_mcp_tool` appeared. A frontmatter-less body-only file is not a
    custom agent: agy logs `Agent "<name>" not found, falling back to
    default`.
  - Q2: `excludeDefaultComponents: true` reduced paired first-request input
    from 3,689 to 883 tokens (tools plus context, not an isolated
    system-prompt measure). Without an allowlist, neither custom variant
    offered file-write, shell or subagent tools (model self-report, not a
    schema dump). With `excludeDefaultComponents: true`, an explicit `tools`
    list (`read_file`, `write_file`, `edit_file`, `run_command`,
    `invoke_subagent`, `define_subagent`, `manage_subagents`,
    `ask_question`) and `mcpServers: [{name: ws}]`, a file edit, a shell
    command, a native child run and an MCP call all succeeded, but MCP went
    through `call_mcp_tool` in that configuration.
  - Q3: both `<workspace>/.agents/agents/<name>.md` and
    `<gemini-dir>/config/agents/<name>.md` are discovered; the home-scoped
    file loads from any workspace. An absolute path passed to `--agent`
    falls back. A malformed file (`mcpServers: [ws]`) gives successful ACP
    responses with no selected-agent or fallback field (`session/new`
    carries `sessionId`, `models`, `modes`, `configOptions`, `_meta`); only
    agy's log distinguishes it: the fallback warning plus
    `Creating new cascade trajectory (agentScript=false)`, versus
    `agentScript=true` for a valid selection. First-request input of
    9,315/9,372 tokens (default/malformed) versus 3,689 (valid) corroborates.

### Confirmed Decisions

- (127)=a, 2026-11-18: revive the agy custom-agent direction as a probe-first
  research; the reason is user reputation about agy's default prompt quality.
- "Quality" is judged as ws procedure compliance plus the coding result,
  default agent versus custom agent, side by side.
- (105)=a, 2026-11-18: the agy worker ships on agy's default agent with the
  ws instructions as a system-prompt append; the custom-agent path is tested
  later behind a knob (owner: "기본 에이전트 경로 + 서브에이전트로 두고,
  knob를 둬서 나중에 custom agent 경로를 추가로 테스트해보는 방향으로
  갑시다"). The placement of that later path is queued as (156) in
  `261118-feat-ws-agy-tool-names-and-worker-role`. Probe 4 is held.

### Proposals

- Offset the `call_mcp_tool` gateway's lazy discovery by writing the
  schemas of the ws tools the agent may call into the custom agent body
  (owner, 2026-11-18). This removes the schema-cache read and the
  name/argument guessing; arguments are still validated only by the ws
  server, not by provider-side schema decoding, and the schema text adds
  prompt tokens in proportion to the tool set.

### Open Questions

- Whether any configuration combines the full native lead tool set with
  direct (not `call_mcp_tool`) MCP tools; probes 1-3 found either direct MCP
  without native write/shell/subagent tools, or native tools with gateway
  MCP.
- Which prompt sections `excludeDefaultComponents` removes exactly, and
  whether post-invocation hooks survive it live (the bundled changelog says
  they do; not tested).
- Fail-loud plumbing for the silent fallback: the only observed signal is
  agy's own log (`agentScript=false` / the fallback warning), which ties the
  daemon to a version-pinned log line; resume behavior and location
  precedence are untested.

- Whether a custom agent subsumes or changes the rendered-manual delivery of
  `261117-feat-ws-hosted-prompt-assembly` for the agy lead (system-prompt
  append versus agent body).
- Whether the result still matters once Pi becomes the canonical hosted
  connector (`261118-research-ws-pi-hosted-connector`); agy's native lead
  stays first-class support in the meantime.

### Rejected Alternatives

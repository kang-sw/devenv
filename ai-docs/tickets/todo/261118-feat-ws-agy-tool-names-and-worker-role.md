---
title: "agy completion: dotted tool-name aliases, harness detection, and the agy worker role"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-antigravity-adapter: "its Phase 2 (88) shipped the hosted agy lead only and deferred the worker role, naming three prerequisites; Phase 2 here picks them up"
  261117-feat-ws-agent-tools-mailbox-routing: "owns the agent.tiers() rule (C2) under which a lead-only backend has no spawnable tiers"
  261118-research-ws-agy-custom-agent-prompt: "probes 1-3 (custom agent file location, silent fallback, excludeDefaultComponents) feed Phase 2's agent file and system-prompt replacement"
  261117-feat-ws-hosted-prompt-assembly: "renders the agy lead row of the workflow manual; Phase 1 harness detection lets that render name agy"
---

# agy completion: dotted tool-name aliases, harness detection, and the agy worker role

## Background

The owner wants to move daily lead work to a hosted Antigravity (agy) lead
because Claude usage is scarce this week. Owner, 2026-11-18: "agy 워커까지
들어와야 마무리" - the agy switch is complete only when agy workers land, not
at the lead alone.

Two gaps stand in the way:

1. **Dotted tool names.** agy forwards ws MCP tools under their dotted names
   (for example `mcp_ws_tickets.query`). The Anthropic API requires tool names
   to match `^[a-zA-Z0-9_-]{1,128}$`, so every turn of a Claude model inside
   agy fails; Gemini and gpt-oss models work. The ws server also has no agy
   harness case (`agents-plugin-tool/internal/mcp/server_harness.go` L77-118),
   and `cmd/ws-mcp/agent_mcp.go` `renderLeadManual` (L71-100) sets a harness
   only for Claude, so the agy lead's manual renders with no harness.
2. **No agy worker.** `261117-feat-ws-hosted-antigravity-adapter` Phase 2
   decided (88) that the first slice is the hosted lead only
   (`ai-docs/tickets/.done/261117-feat-ws-hosted-antigravity-adapter.md`
   L358-374, L484-490, Result L542). `agentWorkerBackends` is
   `{claude, pi}` (`agents-plugin-tool/internal/wsagent/agent_tools.go`
   L44-47; refusal at L243-244; `agy.go` L21-24; pinned by `TestAgyIsLeadOnly`,
   `agy_lead_test.go` L428-429). The adapter ticket left three prerequisites
   for the worker role: a ws-written agy agent file, a worker permission
   posture, and the C2 tier-exposure rule.

## Decisions

- **(106)=a, 2026-11-18: underscore aliases on agy connections only.** The
  hosted HTTP route exposes `_` names (`tickets_query`) only to agy sessions
  and maps a called alias back to the dotted name before dispatch. Rejected:
  renaming the tools for every harness (breaks every other caller and the
  shipped playbooks' canonical names).
- **(132)=a, 2026-11-18: fix first, then one daemon swap.** Phase 1 lands
  before the owner's PC daemon moves to the track build, so a single swap
  serves both Gemini and Claude models in agy. The owner performs the swap and
  the live agy-lead check at home. Rejected: swapping right after the develop
  sync and swapping again after the fix (two swaps, each with start-time
  cleanup and failed in-flight calls).
- **(133)=no, 2026-11-18: no agy CLI on the office PC.** agy live checks and
  the custom-agent probes run on the owner's other machine.
- **(135), 2026-11-18: placement.** A `todo/` child of epic
  `261114-epic-ws-common-session-foundation` with the `related:` links above;
  the adapter ticket deferred the worker role without naming a ticket, and
  every agy ticket sits under this epic.
- **(136), 2026-11-18: agy detection from the hosted lead record.** An agy
  connection is recognized by the hosted lead record's `Backend == agy` (the
  record the hosted key already resolves for `hostedSessionKeyRule`).
  Rejected: a URL marker added in `agyHTTPServer` (`agy.go` L291-300), a
  second identity channel that can disagree with the record.
- **(137), 2026-11-18: one alias rule.** The alias mapping mirrors
  `agents-plugin-pi/src/bridge.ts` `sanitizeToolName` (L567); a collision is
  refused when the alias table is built, pinned by a test. Rejected: a new
  agy-specific naming scheme (two naming rules across harnesses).
- **(138), 2026-11-18: harness detection in Phase 1.** An agy case in
  `server_harness.go` and a `renderLeadManual` render with harness agy land
  with the aliases, since (102)=a ordered tool names (G2) and harness
  detection (G3) together.
- **(139), 2026-11-18: Phase 1 is compatibility tier T0.** Aliases exist only
  in the agy connection's `tools/list` and are reverse-mapped server-side; no
  stored shape, hello field, or tool argument changes, and non-agy callers
  see dotted names unchanged.
- **(140), 2026-11-18: Phase 2 waits for probe evidence.** The agy agent file
  location and system-prompt replacement for workers come from probes 1-3 of
  `261118-research-ws-agy-custom-agent-prompt`; Phase 2 is not promoted to
  `ready/` before they land. Rejected: choosing them now, since (08)=c showed
  a malformed agent config silently falls back to the default agent.
- **(141)=a, 2026-11-18: agy worker permission posture.** A spawned agy
  worker runs with the same posture as today's spawned Claude and Pi workers
  (unattended, no approval prompts); `261115-feat-ws-permission-profile`
  later tightens every backend uniformly. Rejected: holding Phase 2 for that
  ticket's workdir-write cap.

## Constraints

- Manuals for the touched paths: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`.
- Go builds and tests run on remote w5090 only (no local `go test` on the
  owner's Windows PC); temp files under `D:/Temp/<purpose>/`, deleted after
  use.
- No agy CLI on the office PC ((133)); a live agy check is the owner's, on
  another machine.

## Phases

### Phase 1: Underscore tool-name aliases and agy harness detection

- `advertisedToolName` (`agents-plugin-tool/internal/mcp/server_inventory.go`
  L174-176, today an identity stub) returns the underscore alias on an agy
  connection and the dotted name elsewhere.
- `callTool` (`server_dispatch.go`, about L36-71) maps an alias back to its
  dotted name after `hostedSessionKeyRule` and before `toolAllowed`, so
  allowlists and dispatch keep seeing canonical names.
- Harness detection names agy for the hosted agy lead, and the lead manual
  renders with that harness.
- Verification: unit tests for the alias table (round trip, collision refusal,
  non-agy connection unchanged) and for the agy harness render, run on w5090.
  Then the owner swaps the daemon to the track build once ((132)) and checks a
  hosted agy lead live: a Gemini model first, then a Claude model calling a
  dotted ws tool through its alias.

### Phase 2: agy worker role

- Add an `agent-worker-agy` profile and `BackendAgy` to `agentWorkerBackends`,
  so `agent.spawn agy.<tier>` is accepted and `agent.tiers()` lists agy tiers
  under the C2 rule of `261117-feat-ws-agent-tools-mailbox-routing`.
- Settle the three prerequisites the adapter ticket deferred: the ws-written
  agy agent file, the worker permission posture, and system-prompt
  replacement for workers.
- Replace `TestAgyIsLeadOnly` with tests that pin the worker role.
- Verification: unit tests on w5090, then one live `agent.spawn agy.<tier>`
  from a hosted lead on the owner's agy machine, with the worker reporting
  back through the mailbox.

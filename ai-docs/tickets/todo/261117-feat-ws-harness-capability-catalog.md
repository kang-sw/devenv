---
title: "Harness capability catalog: the declarative common interface for hosted harnesses, with Claude as the reference profile"
parent: 261114-epic-ws-common-session-foundation
related:
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions this ticket implements"
  261114-refactor-ws-common-session-interface: "done; the shared access/identity/event-adapter boundary this catalog builds on"
  261115-feat-ws-hosted-codex-adapter-contract: "stage-3 consumer; fills Codex values per harness@version range"
  261117-feat-ws-hosted-antigravity-adapter: "stage-3 consumer; fills Antigravity values per harness@version range"
  261117-feat-ws-agent-tools-mailbox-routing: "consumes the steer and native-subagent-removability items"
---

# Harness capability catalog

## Background

Owner direction (01): the daemon-hosted session is the reference tier, and the
common interface for harnesses is a declarative capability catalog (07), not a
wider external spec. `agents-plugin-tool/internal/wsagent/backend.go` already
carries the raw material as optional-control interfaces (`commandRunner`,
`turnProber`, `processControls`: approval, mode, mcp, steer, reports,
commands). This ticket promotes them into one explicit table of items, each
declared per `harness@version` range, so that later stages add a harness by
filling values and writing a backend rather than by editing shared code.

This is the first stage-2 child of the epic. The reference implementation is
Claude (25): the Claude values are filled here. Codex and Antigravity values
come with their stage-3 tickets; Pi values come with the Pi port of the Codex
adapter later (25), but the Pi HTTP MCP probe below stays the first step of this
ticket.

## Decisions

1. **The catalog is the common interface (07), (08)=c.** Promote the existing
   optional controls in `backend.go` into an explicit table. Add only items that
   Claude, Pi and the stage-3 harnesses actually need. Delegation is always
   `ws/agent.*` regardless of harness (06); harnesses whose native subagents
   cannot be removed are decided per harness at stage 3 (08)=c, not here.
2. **Admission conditions.** A harness is unusable without them: (0) headless
   JSON event streaming (the adapter converts the format); (3) HTTP MCP server
   injection (24).
3. **Items are graded, not boolean (07).** Each item is declared `native`,
   `ws-substitute` or `unsupported`, plus a range (set or grade). Items:
   - skill list (substitute: the daemon provides the playbook list);
   - skill injection (substitute: render the playbook into the prompt);
   - compaction command delivery (substitute: summarize and continue in a new
     session);
   - rewind control (hide, or rebuild the conversation);
   - default tool removal range (set-valued);
   - native subagent removability (the key item of tool removal);
   - model/effort selection range;
   - approval/permission modes;
   - mid-turn steer;
   - resume;
   - reasoning stream;
   - usage query.
4. **Version policy (20)=a, (21)=a, (22).**
   - Catalog values are per `harness@version`; item entries carry `since` /
     `until` version ranges. Version branches in adapter code exist only where
     data cannot express the difference (protocol translation).
   - At session start the adapter reads the harness version (`--version` or the
     init event) and lightly checks the init-event shape (tool names, required
     fields); mismatched capabilities are demoted to `unsupported`. The version
     number alone is not trusted.
   - Verification is per-harness-version recorded stream fixtures. The upper
     bound of the guaranteed range is the newest version that has a fixture.
   - Out of range, above or below the lower bound: allow loudly. Run with the
     nearest-version catalog profile, show an "unverified version" warning on the
     dashboard, disable mismatched capabilities at the startup check, and error
     only on an actual failure.
   - Precedents: Pi exposes `pi.*` only at >=1.0 (`piUnavailable`); Claude
     `claudeInitToolNames {"Agent":"Task"}` (`claude.go`, search for the symbol)
     is a version quirk expressed in code today.
5. **MCP transport (24).** Hosted sessions connect to the daemon by HTTP direct
   JSON-RPC with a per-session token (precedent: the Pi child capability token
   `WS_MCP_PI_TOKEN`; re-issued per process start; loopback only). Principle: no
   intermediary program (the Python adapter) between harness and daemon on the
   hosted path. The CLI tier keeps the stdio launcher (additive, T0). HTTP MCP
   injection is therefore an admission condition, not an optional item.
6. **Compaction is a common command delivered from outside the harness.** The
   user pushes it through chat; it is not a harness-internal feature (owner-
   confirmed, catalog section of the discussion; no separate board ID). A
   harness that cannot take it natively uses the `ws-substitute` (summarize and
   continue in a new session).
7. **The Pi HTTP MCP probe goes first.** Verify whether Pi supports HTTP MCP
   natively without the extension. The outcome is reported because the Pi
   extension retirement premise (04) depends on it; it does not itself approve any
   removal (per-item approval stays).
8. **Fixtures run on remote w5090 (21).** Never run `go test` locally.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If a change reaches `agents-plugin/rsrc/`,
  `agents-plugin/skills/`, `agents-plugin/hooks/` or `agents-plugin-wsflow/`, also
  read `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; interactive paths never wait on file I/O. Catalog data is
  declarative in-code data; per-session tokens are memory-only unless a restore
  need is shown. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- Shipped text must not depend on anything only this repository has
  (`ai-docs/manuals/shipped-surface-boundary.md`).
- The Claude CLI tier is frozen: keep current features, add none.
- Add only items Claude, Pi and stage-3 harnesses actually need.

## Compatibility

T0 (additive). The catalog and version policy are daemon-internal. The HTTP MCP
endpoint and per-session token are new surface; the existing stdio launcher path,
the `/mcp` hello and existing tool schemas are not altered. The concrete verdict
cites source lines for which version writes and reads what, per
`ai-docs/manuals/release-compatibility.md`, and is recorded in Phase 2's Result.
The out-of-range "allow loudly" behavior is observable behavior, owner-confirmed
by (22).

## Phases

### Phase 1: Pi HTTP MCP probe, catalog schema and Claude values

Start by verifying whether Pi supports HTTP MCP natively without the extension;
record the outcome with its evidence (it feeds the Pi extension retirement parity
criteria in the epic and in `261116-research-ws-hosted-session-first-tier`). Then
define the catalog table (items, `native` / `ws-substitute` / `unsupported` plus
range, `since` / `until`) as the promotion of the `backend.go` optional controls,
and fill the Claude (reference) values. Pi values are not filled here (they follow
the Pi port, (25)).

Deferred: Codex, Antigravity and Pi values; the HTTP transport implementation.

Verification: unit tests on catalog lookup and range resolution, run on remote
w5090; the probe outcome recorded; an inventory showing each existing optional
control in `backend.go` maps to a catalog item or is explicitly left out.

### Phase 2: HTTP MCP direct JSON-RPC with per-session token (admission condition)

Hosted Claude sessions reach the daemon by HTTP direct JSON-RPC with a
per-session token, re-issued per process start, loopback only, with no
intermediary program on the hosted path. The CLI tier keeps stdio. Record the
compatibility verdict with source citations.

Verification: tests on remote w5090 covering token issue and re-issue,
loopback-only binding, and rejection of a missing or stale token; a hosted Claude
session exercising ws MCP tools over HTTP; the stdio path unchanged.

### Phase 3: Startup version/init-shape check, loud nearest-profile policy, fixtures and compaction command

Implement the startup read of harness version plus the light init-shape check,
demotion of mismatched capabilities to `unsupported`, the nearest-profile
"unverified version" warning on the dashboard, per-version recorded stream
fixtures for the Claude versions in the guaranteed range, and compaction as a
common command delivered from outside the harness.

Verification: fixture-driven tests on remote w5090 for in-range, above-range and
below-range versions and for an init-shape mismatch; a dashboard check that the
warning shows; compaction pushed from chat reaches a hosted Claude session and is
not a harness-internal feature.

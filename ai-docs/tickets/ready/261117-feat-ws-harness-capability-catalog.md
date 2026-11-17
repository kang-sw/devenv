---
title: "Harness capability catalog: the declarative common interface for hosted harnesses, with Claude as the reference profile"
parent: 261114-epic-ws-common-session-foundation
related:
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions this ticket implements"
  261114-refactor-ws-common-session-interface: "done; the shared access/identity/event-adapter boundary this catalog builds on"
  261115-feat-ws-hosted-codex-adapter-contract: "stage-3 consumer; fills Codex values per harness@version range"
  261117-feat-ws-hosted-antigravity-adapter: "stage-3 consumer; fills Antigravity values per harness@version range"
  261117-feat-ws-agent-tools-mailbox-routing: "consumes the steer and native-subagent-removability items"
  261115-feat-ws-permission-profile: "reuses the BackendRegistration registry this catalog sits beside; no parallel descriptor"
  261115-feat-ws-claude-reasoning-summary-producer: "owns the Claude reasoning-stream value (its Phase 4 exact-version allowlist)"
  261117-feat-ws-hosted-compaction-lifecycle: "owns ws/compact, the default hosted compaction method (Decision 6); this catalog's native compaction grades feed its fallback"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c08c786a8356c83b
sage-review-completeness-reviewed: c08c786a8356c83b
---

# Harness capability catalog

## Background

Numbers in parentheses, such as (07), (24) or (61)=a, are owner decision IDs.
(01)-(29) are recorded in `261116-research-ws-hosted-session-first-tier`
Confirmed Decisions (or its ask log). (30) and later come from the 2026-11-17
lead-discuss whiteboard and are dated where this ticket cites them. This ticket
restates the decision where it matters.

Owner direction (01): the daemon-hosted session is the reference tier, and the
common interface for harnesses is a declarative capability catalog (07), not a
wider external spec. `agents-plugin-tool/internal/wsagent/backend.go` already
carries the raw material as optional-control interfaces (`commandRunner`,
`turnProber`, `processControls`: approval, mode, mcp, steer, reports,
commands, turns, recovery; `agents-plugin-tool/internal/wsagent/backend.go#L167-L203`). This ticket promotes them into one explicit table of items, each
declared per `harness@version`, so that later stages add a harness by
filling values and writing a backend rather than by editing shared code.

This is the first stage-2 child of the epic. The reference implementation is
Claude (25: Pi is driven only with GPT models, so its hosted backend is a port
of the Codex adapter and catalog profile; Pi comes last in the order Claude ->
Codex -> Antigravity -> Pi): the Claude values are filled here. Codex and Antigravity values
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
   `ws-substitute` or `unsupported`, plus a value range (the set or grade the item delivers, such as removable tools
   or model/effort choices; distinct from the `since`/`until` version range).
   Items:
   - skill list (substitute: the daemon provides the playbook list);
   - skill injection (substitute: render the playbook into the prompt);
   - compaction command delivery (substitute: summarize and continue in a new
     session);
   - rewind control (hide, or rebuild the conversation);
   - default tool removal (value range: the removable tool set);
   - native subagent removability (the key item of tool removal);
   - model/effort selection (value range: the offered choices);
   - approval/permission modes;
   - mid-turn steer (role split per Decision 9. Claude: `claudeProcess.Steer`
     writes a user message the running turn takes up, `claude.go:396-399`; the
     handle exists for the process, `claude.go:383`. Lead `native`:
     `conv.steer` uses `controlsFor(c.proc).steer` only when `c.rec.Lead`,
     `agents-plugin-tool/internal/wsagent/runner_conv.go:277-305`. Worker
     `unsupported` until a recoverable worker steer path is wired: non-leads go
     to `steerWorker`, `children.go:592-595`, which returns unless
     `rec.Backend == BackendPi`, and Claude `processControls` sets no `reports`
     handle, so Claude worker reports fall back to next turn. Consumers: ws
     mailbox delivery uses steer with next-turn fallback (owner (61)=a,
     2026-11-17), see `261117-feat-ws-agent-tools-mailbox-routing`; per
     Decision 9 a Claude worker therefore gets next-turn delivery until wired);
   - turn interrupt (owner (30), 2026-11-17: stop the current turn, the
     session stays; separate from steer. Claude: `native`, the stream control
     request `interrupt`, `claudeProcess.Interrupt`, `claude.go:392-394`, part
     of the required `Process` interface, `backend.go:105-107`; the runner
     entry is `Runner.Interrupt`, `runner.go:1056-1080`);
   - follow-up (owner (30), 2026-11-17: a message queued for next-turn
     delivery; separate from steer. Claude: `ws-substitute`, the ws inbox holds
     the message and the runner submits it when the turn ends (`Runner.Send`,
     `runner.go:1003-1054` (`func (r *Runner) Send` at 1003), `wsinbox.Send`);
     the harness is not asked to queue it. For a spawned Pi worker every
     `Runner.Send` is marked as a steer today, so follow-up and steer are not
     separate for Pi workers. A held message is cancelled from the dashboard outbox row or bubble
     (commit d33f233e1, the dashboard change that adds the cancel control for a held message, `agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js`;
     the daemon-side cancel handler is unverified here);
   - resume;
   - reasoning stream;
   - usage query.
4. **Version policy (20)=a, (21)=a, (22).**
   - Catalog values are per `harness@version`; item entries carry a `since` /
     `until` version range. Version branches in adapter code exist only where
     data cannot express the difference (protocol translation).
   - At session start the adapter reads the harness version (`--version` or the
     init event) and lightly checks the init-event shape (tool names, required
     fields); mismatched capabilities are demoted to `unsupported`. The version
     number alone is not trusted. Starting mapping for Claude (fields of
     `claudeInit`, `claude.go:236-255`; Phase 3 confirms it and documents the
     final table): `tools` -> default tool removal and native subagent
     removability (a missing or renamed tool name demotes them); `permissionMode`
     -> approval/permission modes; `slash_commands`
     -> compaction command delivery: a present list without `compact` demotes
     it, and an absent field (older harnesses) leaves the grade unchanged; `model` -> model/effort selection;
     `mcp_servers` -> admission condition (3), never demoted, a failure to attach
     is an error; `plugins` -> no item (for workers `checkClaudeInit`
     refuses out-of-profile plugins and MCP servers fatally; for leads, no
     check).
     Items with no init field (steer, interrupt, resume, usage query) are gated
     by the version only.
   - Verification is per-harness-version recorded stream fixtures. The lower
     bound of the guaranteed version range (the oldest version a grade is claimed
     for, the same lower bound for every item) is the oldest Claude version with a
     recorded fixture (existing testdata carries `claude_code_version` 2.1.319,
     2.1.322 and 2.1.323, which may seed it); the upper bound is the newest
     version with a fixture.
   - Above the upper bound or below the lower bound: allow loudly. Run with the
     nearest-version catalog profile, show an "unverified version" warning on the
     dashboard, disable mismatched capabilities at the startup check, and error
     only on an actual failure.
   - The warning is a computed, in-memory session State field with a real
     dashboard renderer; it is not persisted in `agent.json` (file-touch rule).
     `Record.PluginWarning` (`agents-plugin-tool/internal/wsagent/record.go:67-68,296`)
     is persisted and has no renderer, so it is not the vehicle.
   - Precedents: Pi exposes `pi.*` only at >=1.0 (`piUnavailable`); Claude
     `claudeInitToolNames {"Agent":"Task"}` (`claude.go`, search for the symbol)
     is a version quirk expressed in code today; its Agent/Task alias behavior is
     preserved if it moves into versioned data.
   - The existing worker capability check `checkClaudeInit`
     (`agents-plugin-tool/internal/wsagent/claude.go:267-321`, commit 087228bc
     "stop if the limit leaks") stays fatal and is not merged into demotion.
     Demotion to `unsupported` applies only to catalog capability shape. The new
     light check also runs for leads (`leadInit`, `claude.go:1061`) as demotion
     only, never fatal.
5. **MCP transport (24).** Hosted sessions connect to the daemon by HTTP direct
   JSON-RPC with a per-session token (precedent: the Pi child capability token
   `WS_MCP_PI_TOKEN`; re-issued per process start; loopback only). Principle: no
   intermediary program (the Python adapter) between harness and daemon on the
   hosted path. The CLI tier keeps the stdio launcher (additive, T0). HTTP MCP
   injection is therefore an admission condition, not an optional item.
   The per-session token resolves to the same per-connection context
   `ProxyHello` gives the stdio path today
   (`agents-plugin-tool/internal/wsdashboard/proxy.go:146-184`): mode, the tool
   allowlist (`EnvAllowedTools`, `runner_process.go:115-121`) and identity. A
   worker over HTTP must never see tools beyond its profile.
6. **Compaction is a common command delivered from outside the harness (07).**
   The user pushes it through chat; it is not a harness-internal feature (the
   owner proposed it in the message that (07) confirmed). Claude and Pi are
   graded `native`, through the existing paths: dashboard `/compact` ->
   `wsinbox.Command` -> `runner_conv.go:882` -> `runCommand`/`Send` fallback
   (`backend.go:229-234`); Claude mapper `claude.go:810-815,877-891`; Pi
   `pi.go:538-558`. The compaction `ws-substitute` (summarize and continue in a new
   session) and the compaction nudge are owned by
   `261117-feat-ws-hosted-compaction-lifecycle` (ws/compact). The
   agent-initiated `request_compaction` lever (`Event.CompactRequested`) is
   untouched. Owner (31)=a: the default compaction method for hosted sessions
   is `ws/compact`, owned by `261117-feat-ws-hosted-compaction-lifecycle`; the
   catalog's native compaction grades remain factual inputs, used for the
   fallback where the catalog grades native compaction.
7. **The Pi HTTP MCP probe goes first.** Verify whether Pi supports HTTP MCP
   natively without the extension; pass criterion: a Pi session with no
   extension loaded connects to an HTTP MCP server and calls a tool. The outcome
   is reported because the Pi
   extension retirement premise (04) depends on it; it does not itself approve any
   removal (per-item approval stays).
8. **Fixtures run on remote w5090 (21).** Never run `go test` locally.
9. **Catalog placement and consumers.** The catalog is a declared ceiling per
   `harness@version`, with a role dimension (lead/worker) where a harness needs
   one: Claude gives approval/mode/mcp handles only to leads
   (`agents-plugin-tool/internal/wsagent/claude.go:382-390`) while steer goes to
   every role whose grade is not `unsupported`. Runtime support = catalog grade AND the instance handle (`controlsFor`).
   The handle-only control decision in `registration.go:10-20` (commit 9846671b, the decision to use optional control handles rather than backend-wide boolean promises)
   stays: the catalog never replaces instance handles, and `ErrUnsupported`
   remains the restart signal. The catalog's readers are runner substitute
   selection (e.g. steer vs next-turn delivery), dashboard choices/UI (hide
   unsupported controls) and the startup demotion. It lives with or beside
   `BackendRegistration` (`registration.go:14-20`); permission-mode and
   permission-level data stays sourced from that registry and is not duplicated,
   aligning with `261115-feat-ws-permission-profile`'s "reuse this registry, no
   parallel descriptor". Rejected: a standalone table that shared code gates on
   instead of handles. (design review 2026-11-17, auto-confirmed technical)
10. **Grade semantics.** A grade states what the ws adapter actually delivers
    for that harness@version, not what the harness can do natively. Items with
    no ws wiring today are `unsupported` until wired. For Claude these are
    rewind, skill list (init `skills` not parsed, `claude.go:236-255`),
    reasoning stream (`claudeRingSkip`, `claude.go:622`) and default tool
    removal for leads. The Claude reasoning-stream value is deferred to
    `261115-feat-ws-claude-reasoning-summary-producer` (its Phase 4 exact-version
    allowlist). Rejected: grading by native harness capability. (design review
    2026-11-17, auto-confirmed technical)
11. **Hosted attach paths and token persistence.** Phase 2 covers both attach
    paths: workers use inline `--strict-mcp-config --mcp-config`
    (`claude.go:166,212-222`), and hosted leads today get ws through the bundled
    plugin's stdio server (`claudeLeadArgs`, `claude.go:102-132`,
    `claude_lead_plugins.go`); Phase 2 moves hosted leads to HTTP too, per the
    no-intermediary principle (24). The token is persisted for holder reattach
    following the Pi precedent (`rec.Capability` with holder
    `capabilityHash` / `restoreToken`, `runner_process.go:130-141`,
    `children.go:146-156`): the restore need is shown, and only what restore
    needs is persisted. Rejected: memory-only tokens, which a holder reattach
    would lose. (design review 2026-11-17, auto-confirmed technical)

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
  declarative in-code data; per-session tokens persist only what holder restore
  needs (Decision 11). Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- Shipped text must not depend on anything only this repository has
  (`ai-docs/manuals/shipped-surface-boundary.md`).
- The Claude CLI tier is frozen: keep current features, add none.
- Add only items Claude, Pi and stage-3 harnesses actually need.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Compatibility

T0 (additive). The catalog and version policy are daemon-internal. The HTTP MCP
endpoint and per-session token are new surface; the existing stdio launcher path,
the `/mcp` hello and existing tool schemas are not altered. The concrete verdict
cites source lines for which version writes and reads what, per
`ai-docs/manuals/release-compatibility.md`, and is recorded in Phase 2's Result.
It must include this scenario: an HTTP session launched by the new daemon, then
reattached by an older daemon that has no HTTP endpoint.
The out-of-range "allow loudly" behavior is observable behavior, owner-confirmed
by (22).

## Prior Decisions

- 261114-refactor-ws-hosted-common-contract-pi-claude-parity (2026-11-14, commit 9846671b): "Use optional control handles, not backend-wide boolean promises: Claude worker method sets include lead-only controls, while Pi model/effort support still depends on the argument and ErrUnsupported remains the restart signal." — bearing: constrains
- 261114-epic-ws-common-session-foundation (2026-11-17, ce49016f): "Hosted-first: the daemon-hosted session is the reference tier, Claude is the reference implementation, Claude CLI frozen, Pi extension retired after parity with per-item approval." — bearing: supports
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "(07) ok, (08)=c, (20)-(22), (24) The common harness interface is a declarative capability catalog promoting the existing `wsagent` optional controls into an explicit table" — bearing: supports
- 261117-feat-ws-hosted-antigravity-adapter (2026-11-17, Decisions): "Catalog-based (07). The work is: fill the capability catalog values per `harness@version` range (`since` / `until`), and implement the backend." — bearing: supports
- 261115-feat-ws-hosted-codex-adapter-contract (2026-11-15, Decisions): "Foundation gate satisfied; catalog gate added. The implementation prerequisite is now `261117-feat-ws-harness-capability-catalog`, and separate execution approval remains." — bearing: supports
- 261114-refactor-ws-common-session-interface (2026-11-16, Resolution): "Phase 2 implementation and source-fixture parity verification are complete. The lead owns the explicitly deferred independent review and integration of impl/develop/suing-usage-cold" — bearing: supports
- 087228bc (2026-11-09, commit): "Claude 2.1.319 launched with --tools Agent reports the tool as 'Task' in system/init, while tool_use blocks are named 'Agent'. Without the alias the init check would kill every launch." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/backend.go, claude.go, new catalog code, HTTP MCP endpoint in agents-plugin-tool/internal/mcp/, dashboard warning |
| scope.surface | cross-module | wsagent catalog and startup check, mcp HTTP endpoint with per-session token, dashboard unverified-version warning |
| scope.new_public_symbol | yes | new HTTP MCP endpoint and per-session token (Phase 2); catalog Go symbol names not yet chosen |
| scope.new_type_contract | yes | catalog item table with native, ws-substitute, unsupported grade plus since/until version range (Phase 1) |
| scope.test_surface | new-files | existing wsagent tests (controls_test.go, claude_test.go) do not cover a catalog; Phases 1-3 call for new lookup, token and fixture tests |
| complexity.reuse_points | confirmed | processControls optional interfaces at backend.go#L167-L227 read; Pi capability token EnvPiToken at children.go#L59 as precedent |
| complexity.side_effect_risk | high | new authenticated HTTP endpoint on the shared daemon plus startup demotion of capabilities in every hosted launch |
| risk.correctness | high | version-range resolution, init-shape demotion and token re-issue each fail silently if wrong; fixtures for Claude versions do not exist yet |
| risk.fit | moderate | catalog is a ceiling over lead-only versus worker handles kept by 9846671b (Decision 9); Phase 1 inventory mapping still to be recorded |
| risk.test | high | tests run on remote w5090 only, per-version recorded stream fixtures and a live hosted Claude session are required |
| risk.security_or_contract | high | new HTTP MCP endpoint with loopback-only token auth; T0 verdict still unrecorded (pending Phase 2 Result) |

## Phases

### Phase 1: Pi HTTP MCP probe, catalog schema and Claude values

Start by verifying whether Pi supports HTTP MCP natively without the extension;
record the outcome with its evidence in this phase's Result, against the pass
criterion of Decision 7 (it feeds the Pi extension retirement parity
criteria in the epic and in `261116-research-ws-hosted-session-first-tier`). Then
define the catalog table (items, `native` / `ws-substitute` / `unsupported` plus
value range, `since` / `until` version range) as the promotion of the `backend.go` optional controls,
and fill the Claude (reference) values per Decisions 9 and 10. Pi values are not filled here (they follow
the Pi port, (25)).

Deferred: Codex, Antigravity and Pi values; the HTTP transport implementation.

Verification: unit tests on catalog lookup and version-range resolution, run on remote
w5090; the probe outcome recorded in this phase's Result; an inventory showing each existing optional
control in `backend.go` maps to a catalog item or is explicitly left out.

### Phase 2: HTTP MCP direct JSON-RPC with per-session token (admission condition)

Hosted Claude sessions reach the daemon by HTTP direct JSON-RPC with a
per-session token, re-issued per process start, loopback only, with no
intermediary program on the hosted path. The token resolves to the same
per-connection context `ProxyHello` gives stdio today (mode, tool allowlist,
identity). Cover both attach paths: workers (inline `--mcp-config`) and hosted
leads (moved from the bundled plugin's stdio server to HTTP). Persist only what
holder reattach needs (Decision 11). The CLI tier keeps stdio. Record the
compatibility verdict with source citations, including the new-daemon launch /
older-daemon reattach scenario.

Verification: tests on remote w5090 covering token issue and re-issue,
loopback-only binding, rejection of a missing or stale token, a worker over HTTP
seeing no tool beyond its profile, and token restore on holder reattach; a hosted
Claude worker and lead session exercising ws MCP tools over HTTP; the stdio path unchanged.

### Phase 3: Startup version/init-shape check, loud nearest-profile policy, fixtures and compaction command

Implement the startup read of harness version plus the light init-shape check,
demotion of mismatched capabilities to `unsupported` (for workers alongside the
unchanged fatal `checkClaudeInit`, for leads demotion only), the nearest-profile
"unverified version" warning as a computed State field on the dashboard,
per-version recorded stream fixtures for the Claude versions in the guaranteed
range, and compaction: the catalog item (Claude and Pi `native`) plus routing the
existing command through it. Document the init-field -> catalog-item mapping that
gates demotion. Record the chosen lower- and upper-bound fixture versions in this
phase's Result.

Verification: fixture-driven tests on remote w5090 for in-version-range, above-range and
below-range versions and for an init-shape mismatch; a dashboard check that the
warning shows; compaction pushed from chat reaches a hosted Claude session and is
not a harness-internal feature.

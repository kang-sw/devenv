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
   Owner note 2026-11-17 22:14: Pi has built-in MCP support since 1.0 (the ws
   extension's own MCP client predates it); the installed Pi is 1.5.2, so the
   probe's open question is the HTTP transport with a per-session token header.
8. **Fixtures and test location (21), (68).** Per-version recorded stream fixtures are the verification vehicle (21). Owner (68), 2026-11-17 22:10: Go tests run locally on the macOS home machine; the earlier remote-w5090-only wording belonged to the owner's company Windows setup.
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
12. **Hosted lead keeps the bundled plugin minus its MCP entry (owner (65),
    2026-11-17 21:42).** Today a hosted lead disables installed `ws`/`wsflow`
    plugins and loads the daemon-selected bundle through `--plugin-dir`
    (`claude_lead_plugins.go:41-52`, `hosted_plugins.go` `ResolveHostedPlugin`);
    that bundle's `.mcp.json` starts the Python stdio launcher. Phase 2 keeps
    the bundle for its hooks and skills (until
    `261117-feat-ws-hosted-prompt-assembly` replaces them), blocks only the
    bundle's stdio ws MCP entry, and injects the HTTP MCP through
    `--mcp-config`, so a lead never attaches two ws servers. The blocking
    mechanism (a hosted plugin variant without `mcpServers`, or
    `--strict-mcp-config` if a probe shows it also suppresses plugin MCP
    servers) is chosen by a Phase 2 probe and recorded in its Result.
    Rejected: adding HTTP alongside the plugin's stdio server (two ws servers);
    dropping the bundle entirely (hooks and skills vanish before prompt
    assembly replaces them).
13. **A harness without a catalog profile stays handle-only (owner (69),
    2026-11-17 22:10).** Until a harness has a profile (Pi until its port of
    the Codex adapter, (25)), catalog readers do not consult the catalog for
    it: no dashboard hiding, no startup demotion and no unverified-version
    warning; runtime support is the instance handle alone, as today. The Pi
    compaction `native` grade in Decision 6 records today's path as fact; it
    is not a partial Pi profile. Rejected: a partial Pi profile filled ahead
    of the port.

## Constraints

- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
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
| risk.test | high | per-version recorded stream fixtures and a live hosted Claude session are required |
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

Verification: unit tests on catalog lookup and version-range resolution; the probe outcome recorded in this phase's Result; an inventory showing each existing optional
control in `backend.go` maps to a catalog item or is explicitly left out.

### Result (d20202cd6) - 2026-11-17

**Pi HTTP MCP probe: PASS (Pi 1.5.2), with one exposure caveat.** A stdlib
Streamable HTTP MCP server on 127.0.0.1 required `Authorization: Bearer
<token>`; Pi read the server from a project `.pi/mcp.json` entry
`{"url": "http://127.0.0.1:<port>/mcp", "headers": {"Authorization": "Bearer
${PROBE_TOKEN}"}, "exposure": "direct"}` and ran
`PROBE_TOKEN=... pi --approve -ne -e builtin:mcp --no-session -p "Call the MCP
tool probe_echo ..." </dev/null`. The server log showed `initialize`,
`notifications/initialized`, `GET /mcp` (405, tolerated), `tools/list` and
`tools/call`, each with the bearer matched; the model replied with the tool's
marker text. Evidence details:
- `-ne -e builtin:mcp` keeps the built-in MCP client; `-ne` also drops
  `builtin:codemode`, so under the default `codemode` exposure the model had
  no route to the tool (no `tools/call`). `"exposure": "direct"` in the entry,
  or also loading `-e builtin:codemode`, makes the tool callable. The hosted
  Pi port must set one of the two.
- A project `.pi/mcp.json` is read only for a trusted project; `--approve`
  grants trust for one process. `pi mcp list` takes no `--approve`.
- Wrong token: `pi mcp list` reports `failed` / "MCP server requires
  authentication" (exit 1) after two `initialize` attempts, with no OAuth
  attempt. Unset token variable: Pi refuses locally ("Failed to resolve MCP
  server ... header Authorization from environment variable") and never
  connects, so a missing token fails closed.
- No user file was modified (`~/.pi/agent/settings.json` and `trust.json`
  hashes unchanged). This result feeds the Pi extension retirement parity
  criterion; it approves no removal.

**Catalog** (`agents-plugin-tool/internal/wsagent/catalog.go`):
`CapabilityGrade` (`native` / `ws-substitute` / `unsupported`),
`CapabilityItem` (the 14 Decision 3 items: skill list, skill injection,
compaction, rewind, tool removal, subagent removal, model/effort, approval,
steer, interrupt, follow-up, resume, reasoning stream, usage query),
`CapabilityEntry` (item, optional role, `Since` inclusive / `Until`
exclusive, grade, `Values` value range, cited `Source`), `HarnessProfile`
(guaranteed range `Oldest`..`Newest` plus entries) and
`HarnessProfile.Resolve(role, version) Capabilities`. Resolve fills every
item (undeclared = `unsupported`); a version below/above the range or
unreadable resolves at the nearest bound (newest for unreadable) and sets
`Unverified`. `BackendRegistration.Catalog *HarnessProfile` carries it; the
daemon registers `ClaudeCatalog()` for claude and nil for pi (Decision 13,
handle-only).

**Claude values** (Decisions 9, 10): lead / worker
- native both: skill injection, compaction, model/effort, interrupt, resume;
- ws-substitute both: follow-up;
- unsupported both: skill list, rewind, reasoning stream;
- lead native, worker unsupported: approval, steer, usage query
  (`get_context_usage` is asked only from `leadInit`);
- worker native, lead unsupported: tool removal (`Values: ["*"]`, the
  `--tools` allowlist) and subagent removal (`Values: ["Agent"]`).
Guaranteed range seeded `2.1.319`..`2.1.323` from the existing testdata
`claude_code_version` values; Phase 3 settles it.

**Inventory of `backend.go` controls:** `approval`, `mode` -> approval;
`steer`, `reports` -> steer (reports is the worker steer path); `commands` ->
compaction; required `Interrupt` -> interrupt, `SetModel`/`SetEffort` ->
model/effort, `Send` -> follow-up. Left out: `mcp` (status/reconnect recover
the MCP-injection admission condition, never graded), `turns` and `recovery`
(rejoin and holder-replay internals, not a selectable capability).

**Verification:** `go test ./internal/wsagent/ ./cmd/ws-mcp/ -count=1`: ok
(104.7 s, 167.4 s). New tests: `TestParseHarnessVersion`,
`TestVersionInRange`, `TestResolveVersionRanges` (in range, bounds,
below/above range, unknown and unreadable versions), `TestResolveFillsEveryItem`,
`TestValidateCatalogRejectsOverlap`, `TestClaudeCatalogValues`,
`TestAgentRunnerCatalogs`.

**Decisions taken:** the catalog's `Values` stay nil for model/effort and
approval, whose choices remain sourced from `ChoiceLister` and the
registration (pinned by a test). Skill injection is `native` for Claude
(its own skills through the lead bundle or a worker profile's plugin
directories), since Decision 10 does not list it as unwired. Entry overlap is
checked by a test-side validator, not at runtime.

#### Edition (92b4c835b) - 2026-11-17

Lite review fixes. Compaction is split by role: lead `native`, worker
`unsupported`. A Claude worker reports no command list because the initialize
ask and `leadInit` run only for leads, and a worker runs with
`--disable-slash-commands` unless `Skills` is set, so the dashboard refuses
its `/compact`. The "native both" line above therefore no longer includes
compaction. The copy-isolation test now resolves twice from one shared
profile. The range bounds are checked against the recorded fixtures'
`claude_code_version` values. Open for Phase 3: the 2.1.319 fixtures are
worker turns only and the lead fixtures are 2.1.322/2.1.323, so each role's
grades at the Oldest bound still lack a fixture.

### Phase 2: HTTP MCP direct JSON-RPC with per-session token (admission condition)

Hosted Claude sessions reach the daemon by HTTP direct JSON-RPC with a
per-session token, re-issued per process start, loopback only, with no
intermediary program on the hosted path. The token resolves to the same
per-connection context `ProxyHello` gives stdio today (mode, tool allowlist,
identity). Cover both attach paths: workers (inline `--mcp-config`) and hosted
leads (moved from the bundled plugin's stdio server to HTTP; mechanism chosen
by probe, Decision 12: the lead loads a hosted variant of its bundle whose only
MCP entry is the ws server over HTTP under the same plugin server name, instead
of the bundle plus `--mcp-config`). Persist only what
holder reattach needs (Decision 11). The CLI tier keeps stdio. Record the
compatibility verdict with source citations, including the new-daemon launch /
older-daemon reattach scenario.

Verification: tests covering token issue and re-issue,
loopback-only binding, rejection of a missing or stale token, a worker over HTTP
seeing no tool beyond its profile, and token restore on holder reattach; a hosted
Claude worker and lead session exercising ws MCP tools over HTTP, with the lead
attached to exactly one ws server; the stdio path unchanged.

### Result (4e8add8d3) - 2026-11-17

**Route** (2552686ec, `internal/wsdashboard/proxy_http.go`): `POST
/mcp/session`, MCP Streamable HTTP with JSON responses only (ws-mcp sends no
server-to-client request, so no SSE). Loopback peer with no `Origin`; the
bearer token is resolved per request through
`AgentMCPTokenHost.ResolveMCPToken` to the hello the stdio launcher would
send; 401 for a missing, unknown or void token, 405 for GET/DELETE, 403 for a
refused hello. Voiding a token cancels its calls in flight and ends its
session; a first message that is not `initialize` gets a replayed one.
Claude Code 2.1.326 opens with `server/discover` (answered -32601), then
`initialize`, `notifications/initialized` and a GET (405); all handled.

**Tokens** (`internal/wsagent/mcp_session.go`): 32 random bytes (hex) per
process start; a new start voids the previous one; process end, close and a
failed start void it (`revokeToken`). The first resolution waits (up to 10 s)
for the start report, so the hello carries the harness's real environment and
PID: `WS_MCP_HARNESS_PID` is the holder's child PID, which is what ws-mcp's
hosted-session check compares with the registration row's `host_pid`
(confirmed live). URL and token reach the harness only through
`WS_MCP_SESSION_URL` / `WS_MCP_SESSION_TOKEN` (the Claude entry's header is
`Bearer ${WS_MCP_SESSION_TOKEN}`); the daemon unsets both from its own
environment at start (`DropHarnessSessionEnv`). Worker tokens live in memory
only. A held lead's token and URL are kept in its `agent.json` (0600,
`mcp_token_secret`, `mcp_url`), restored on rejoin, and cleared at process
end and failed recovery. A runner with no endpoint keeps the stdio launcher;
the CLI tier is untouched.

**Decision 12 mechanism: option (1), a hosted bundle variant.** Probe runs
p4b..p7 on Claude Code 2.1.326 (artifacts were under `/tmp`, not kept):
- `--strict-mcp-config` (option 3) drops every plugin, user and claude.ai MCP
  server, not only the bundle's ws entry (p4b, p4d): rejected.
- A variant without `mcpServers` plus `--mcp-config` (option 2) names the
  server `ws`, not `plugin:ws:ws`; the claude-mod connects through
  `$.mcp.connect($.plugin.name)` (`agents-plugin/hooks/claude-mod/register.ts`
  907), and its reconnect derives the server name from the bundle manifest's
  `mcpServers` key and gives up when there is none (1283): rejected.
- Option (1): the variant's `mcpServers.ws` is `{"type": "http", "url":
  "${WS_MCP_SESSION_URL}", "headers": {"Authorization": "Bearer
  ${WS_MCP_SESSION_TOKEN}"}}`. A variant made of symlinks fails: Claude
  refuses agent and hook paths that resolve outside the plugin directory
  (p6, `path-traversal` plugin errors for `claude-agents/*.md` and
  `claude-mod/register.ts`). A variant made of copies works (p7): only
  `plugin:ws:ws` connected, over HTTP, and the mod's `config_get` call went
  over HTTP.
- Implementation (`claude_hosted_variant.go`):
  `<daemon dir>/hosted-bundles/<name>-<16 hex of a content stamp>`; manifests,
  hooks, agents and skills are copied (a symlink inside them is copied as its
  target), `bin` is a symlink (the launcher; no plugin path points into it),
  dot entries, `tests` and `native-pi` are left out. It is built once per
  bundle content through a stage directory and a rename, reused, and never
  removed, since a running lead may still read an older one.
- Plan deviation: Decision 12 says the HTTP MCP is injected through
  `--mcp-config`; the lead's ws server instead comes from the variant's
  manifest, and the lead keeps its own `--mcp-config`-free setup. The Phase 2
  plan text above was adjusted before this Result.

**Dogfood surprise, fixed (4ebcf2ed2):** the Claude mapper read
result-carrying ws tools (tracker writes, cards, UI) only as `mcp__ws__*`,
the worker's inline server. A hosted lead's ws tools are
`mcp__plugin_<pkg>_<pkg>__*`, so a lead's tracker writes and cards never
reached its session (the claude-mod's in-process `mcp__ws__request_*` tools
are not a ws-mcp server). Leads now match either hosted package's plugin
server name.

**Stale route after a port change, fixed (found live):** a daemon that
rejoins on another port (configured port 0, the port taken, a changed
`dashboard.port`) left a held HTTP lead calling a dead URL (ECONNREFUSED).
Claude still reports an HTTP server `connected`, so the existing MCP recovery
never fires; the stdio launcher survives this only because it rereads the
endpoint file. The record now keeps `mcp_url`; a rejoin whose recorded URL is
not this daemon's restores no token and restarts the process at its first
idle point with a new token on the new route (`runner_process.go` 295-301,
`restartIfDue` in `runner_recovery.go`).

**Compatibility verdict** (`ai-docs/manuals/release-compatibility.md`):
- `/mcp` stdio route, its hello and tool schemas: unchanged, T0.
- `/mcp/session`: additive, registered only when the agent runner implements
  the interface (`daemon.go` 781): T0. Worker tokens are memory-only: T0.
- `agent.json` gains `mcp_token_secret` and `mcp_url` (`record.go` 144-145).
  A 0.50.2 writer keeps unknown top-level members across read-modify-write
  (v0.50.2 `record.go` 145-205), so an older daemon does not drop them.
- New-daemon launch, older-daemon reattach: 0.50.2 registers `/mcp` as an
  exact path (v0.50.2 `daemon.go` 753) and `/` as the index (790), and
  `POST /mcp/session` without a dashboard credential gets 401
  `login_required` (v0.50.2 `localhost_auth.go` 150). The held lead's ws
  tools fail until its process restarts, and since Claude keeps reporting the
  HTTP server connected, the old daemon's MCP recovery does not restart it.
  The old daemon writes no new data and loses none; a process it later starts
  uses the stdio bundle under a new holder (v0.50.2 `runner.go` 2492), and
  the kept `mcp_*` members then name a dead token, which the new daemon on
  rejoin forgets (see the Edition below). Reopening with the new version
  recovers fully. This is a functional degradation on the downgrade path with
  no data loss; it is not a display-only T1. Tier: pending owner decision
  (board).
- Older-daemon launch, new-daemon reattach: the record has no token, so the
  process keeps its stdio server (`runner_process.go` 302): T0.

**File touches:** one variant directory per bundle content, written once and
reused; the two `agent.json` members exist for held Claude leads only and are
written and cleared inside the existing start and end record updates.

**Live verification** (an isolated daemon built from this branch under
`/tmp`, Claude Code 2.1.326; the shared daemon was not touched): a hosted
worker with a profile allowlist listed and ran only its profile's ws tools
over HTTP; a hosted lead had exactly one ws server, `plugin:ws:ws` (80 tools)
over HTTP, no launcher child process, and its hosted session key honored; the
token appeared in no argv and no file other than the 0600 `agent.json`; the
token was restored after a same-port daemon replace; a port-changing replace
reproduced the stale route fixed above.

**Tests:** route (2552686ec, `proxy_http_test.go`):
`TestProxyHTTPRouteRefusesAMissingUnknownOrVoidToken`,
`...RefusesABrowserOriginAndANonLoopbackPeer`, `...RouteAnswersOnlyPost`,
`...RoutesAnswersByIDAndForwardsNotifications`,
`...ReissuedTokenEndsTheOldSession`, `...ReplaysInitializeForARestoredToken`,
`...CancelsARequestWhoseClientLeft`, `...BodyLimitsAndShapes`,
`...SessionDrainsWithTheDaemon`. Runner (`mcp_session_test.go`,
`claude_hosted_variant_test.go`, `claude_test.go`):
`TestMCPTokenIssueReissueAndVoid`, `TestMCPTokenVoidEndsAWaitingResolution`,
`TestMCPHelloIsTheLaunchersHello`, `TestMCPSessionEnv`,
`TestClaudeWorkerMCPOverHTTP`, `TestClaudeWorkerRunsOverHTTP` (allowlist,
token never persisted, exit voids, no endpoint keeps stdio),
`TestClaudeLeadMCPOverHTTP` (variant `--plugin-dir`, no `--mcp-config`,
token in env and record only, restore on rejoin, void on exit),
`TestClaudeLeadMCPRouteMovedRestarts` (fails with the stale-route branch
disabled), `TestClaudeHTTPVariant`, `TestClaudeHTTPVariantRefusesABadManifest`,
`TestClaudeMapperReadsResultsUnderTheAttachedServer`. End to end
(`cmd/ws-mcp/hosted_http_mcp_test.go`):
`TestHostedWorkerOverHTTPSeesOnlyItsProfile` (a real daemon and ws-mcp: 401
without or with a wrong token, `server/discover` -32601, `tools/list` is
exactly the profile's, an outside tool refused, a void token 401). Runs on
this Mac: `go test ./internal/wsagent/` ok (102.3 s); `./cmd/ws-mcp/` ok
(201.5 s); `./internal/mcp/` fails only
`TestDashboardNotesUsesHostedCallerStoreIdentity`, which fails the same way
on the tree before these commits; `./internal/wsdashboard/` fails only
`TestSearchPaletteRenderedEntries` and `TestSearchPaletteBehavior`, also
failing before them. The `./internal/mcp/` suite takes 884 s here, beyond
`go test`'s default 10 min timeout; run it with `-timeout 30m`.

**Left as is (route leaf notes):** the replayed `initialize` carries a
synthetic `clientInfo` and empty capabilities, which ws-mcp does not read for
Claude; a Pi client's experimental capability would be lost there once Pi
moves to HTTP. Voiding a token cancels its in-flight calls, a `git.commit`
included; that is the intended end of a process's session.

#### Edition (539ef4ad8) - 2026-11-18

Phase 2 lite review fixes (review of 9d65742c7..4e8add8d3; the replayDone
narrowing in e4a0dda94 is covered by the Phase 3 review):

- Rejoin no longer resolves the ws-mcp command. The record keeps the hello's
  ws-mcp environment (`mcp_env`) and the holder the token was drawn for
  (`mcp_holder_id`) beside `mcp_token_secret` / `mcp_url` (`record.go`
  142-151, written with the holder at start, `runner_process.go` 372). A
  rejoin restores the recorded token and hello (310); a token without a
  recorded hello, or on another route, restarts the process at its first
  idle point (303), so an unresolvable package at rejoin can no longer leave
  a process silently calling a token nobody accepts.
- First start loads the bundle its hello describes: the lead's ws-mcp command
  now names its bundle (`MCPCommand.Plugin`, `cmd/ws-mcp/agent_mcp.go`), and
  `claudeLeadPluginArgs` uses that root and note instead of resolving the
  selection again (`claude_lead_plugins.go` 30, `runner_process.go` 321).
- A record whose holder differs from the token's (a daemon without the route
  started the process again over stdio, keeping the members it does not know)
  forgets the token and restarts nothing (`runner_process.go` 298).
- `restartIfDue` reports a restart only when a new process runs
  (`runner_recovery.go`); after a failed one, `replayDone` falls through and
  held input starts a process as with none.
- Route: the replay's pending id is registered when the session is built,
  before it is published to its token's other requests or runs, so a
  session that ends at once can no longer panic on a nil map with `writeMu`
  held, and concurrent first requests cannot write the map together (the
  first version of this fix moved the write after publication; the Phase 3
  review caught it and a98a31f86 fixed it, with
  `TestProxyHTTPConcurrentFirstRequestsShareOneReplay` failing under `-race`
  before); a token whose void channel is already closed when it resolves is
  refused (`proxy_http.go` 96).
- Tests added: `TestClaudeLeadLoadsTheBundleItsHelloDescribes`,
  `TestClaudeLeadRejoinWithoutItsHelloRestarts`,
  `TestClaudeLeadRejoinForgetsATokenAnotherStartLeft`,
  `TestClaudeLeadMCPRouteMovedRestartsAfterItsTurn` (stale route deferred
  while a turn runs), `TestClaudeLeadStaleRouteFailedRestart` (a restart
  that cannot start leaves no process; the next message starts one on the
  new route), `TestClaudeLeadTokenVoidOnFailedStartAndClose` (spawn
  failure and close), `TestClaudeHTTPVariantConcurrentBuilds` (eight
  concurrent builds, one variant, no stage left),
  `TestProxyHTTPRouteRefusesAForeignHost` (rebound Host), the body limit at
  exactly `httpMCPBodyLimit` and one byte over in
  `TestProxyHTTPBodyLimitsAndShapes`; the void steps of
  `TestProxyHTTPRouteRefusesAMissingUnknownOrVoidToken` and
  `TestHostedWorkerOverHTTPSeesOnlyItsProfile` now keep the token resolvable
  with a closed void channel, so they fail without the route's own void
  check (mutation-checked, as were the bundle, forget and deferred-restart
  tests). Not covered by a test: the start path that revokes a token when
  the backend returns a process without a holder.
- Forward (not fixed): `.stage-*` directories of a crashed build and variants
  of retired bundles are never removed; a variant build failure fails the
  lead start rather than falling back to stdio; a session ending under a
  request answers "ended" rather than a retry hint; the token is inherited by
  the harness's own subprocesses (same user, no escalation).

### Phase 3: Startup version/init-shape check, loud nearest-profile policy, fixtures and compaction command

Implement the startup read of harness version plus the light init-shape check,
demotion of mismatched capabilities to `unsupported` (for workers alongside the
unchanged fatal `checkClaudeInit`, for leads demotion only), the nearest-profile
"unverified version" warning as a computed State field on the dashboard,
per-version recorded stream fixtures for the Claude versions in the guaranteed
range, and compaction: the catalog item (Claude `native`; Pi keeps its existing
`/compact` path handle-only until it has a profile, Decision 13) plus routing the
existing command through it. Document the init-field -> catalog-item mapping that
gates demotion. Record the chosen lower- and upper-bound fixture versions in this
phase's Result.

Verification: fixture-driven tests for in-version-range, above-range and
below-range versions and for an init-shape mismatch; a profile-less harness (Pi)
showing no demotion, hiding or warning (Decision 13); a dashboard check that the
warning shows; compaction pushed from chat reaches a hosted Claude session and is
not a harness-internal feature.

### Result (a98a31f86) - 2026-11-18

Commits: 6dc2607b7 (catalog resolution, init-shape check, command gating,
fixtures), 4667ac2e2 (dashboard notice), a98a31f86 (Phase 3 review fix).

**Version read and resolution.** The Claude mapper reports, for each init, the
`claude_code_version` and the items its init shape does not support
(`HarnessReport`, `claude.go` `harnessReport`); for a lead it is the first
event of `leadInit`, for a worker it follows the unchanged fatal
`checkClaudeInit`. The runner resolves the backend's profile at that version
(`Catalog.Resolve`), demotes the reported items to `unsupported`, and shows
the result as `State.Capabilities` (`runner_events.go` `harnessReported`): in
memory only, rebuilt from the held stream on rejoin, unchanged reports are
no-ops. A version outside the guaranteed range runs on the nearest bound's
grades with `unverified` set; nothing errors on version alone.

**Guaranteed range: 2.1.322 (lower) to 2.1.326 (upper)** (`catalog.go`
`ClaudeCatalog` Oldest/Newest). Each bound has a recorded worker and lead
fixture: `claude-worker-turn-2.1.322.jsonl`, `claude-lead-slash-commands.jsonl`
(2.1.322), `claude-worker-turn-2.1.326.jsonl`, `claude-lead-turn-2.1.326.jsonl`;
the new ones were recorded with the installed 2.1.322 and 2.1.326 binaries
(worker argv as `claudeArgs` builds it, lead argv as `claudeLeadArgs` without
`--plugin-dir`) and sanitized like the existing ones. 2.1.319 (the older
`claude-tool-turn.jsonl`) was not taken as the lower bound because no lead
fixture exists for it; it now serves as the below-range case.
`TestClaudeCatalogValues` requires a recorded fixture of each role at
each bound.

**Init-field to item mapping that gates demotion** (`claudeInitShape`; the
catalog's header comment points to it):

| init field | item(s) | demoted when |
|---|---|---|
| `tools` | `tool_removal`, `subagent_removal` | the list is absent, or a profile tool (or its legacy name, Agent/Task) is missing |
| `permissionMode` | `approval` | empty |
| `slash_commands` | `compaction` | the list is present and lacks `compact`; an absent list (older harness) changes nothing |
| `model` | `model_effort` | empty |
| `mcp_servers` | none | admission condition, never demoted (a worker's out-of-profile server stays `checkClaudeInit`'s fatal error) |
| `plugins` | none | (workers: `checkClaudeInit`; leads: no check) |

Items with no init field (steer, interrupt, follow-up, resume, usage query,
skills, rewind, reasoning stream) are gated by version only. Only items graded
better than `unsupported` are listed as demoted.

**Compaction.** The catalog item is `native` for a Claude lead and
`unsupported` for a Claude worker (Phase 1 edition). The existing path is
unchanged: dashboard `/compact` -> `wsinbox.Command` -> the lead's command
turn (`TestClaudeLeadCommandSplitsTheDrain`: `/compact keep the names` reaches
the hosted session's input from chat). The offered command list now passes
through `allowedCommands`: a command whose item resolves `unsupported` (only
`compact` -> `compaction` is mapped) is dropped, and the dashboard intake
refuses a command not in the list. Pi keeps its `/compact` path handle-only
(no profile, Decision 13).

**Dashboard warning** (4667ac2e2): a hosted session whose
`capabilities.unverified` is set, or whose `demoted` list is non-empty, shows a
status notice under the banner (`dashboard-leads.js` `harnessDraw`): "Harness
version X is outside the verified range A–B; running on the capabilities of
B." and "Unavailable on this harness version: ...". No capabilities, or
neither condition, or a closed/handed-off session shows nothing. New i18n keys
`js.lead.harness.unverified`, `js.lead.harness.unreported`,
`js.lead.harness.demoted` in all 11 tables; the vendored lucide
`triangle-alert` icon.

**Profile-less harness.** A backend without a catalog profile (Pi) gets no
`State.Capabilities`, so nothing is demoted, hidden or warned
(`TestProfilelessBackendStaysHandleOnly`).

**Compatibility.** `State.Capabilities` is a new omitempty member of the
in-memory State JSON the dashboard reads; an older dashboard asset ignores it
(T0). Nothing is persisted (`agent.json` is untouched by this phase). The
offered command list may now omit `compact` for a session whose init lacks it;
that is the intended demotion. The Phase 2 tier for the new-daemon launch /
older-daemon reattach path remains pending owner decision (board).

**Tests:** `TestClaudeFixturesResolveByVersion` (in range at both bounds for
both roles, 2.1.319 below and 2.1.399 above, run on the nearest bound marked
unverified, nothing demoted), `TestClaudeInitShapeDemotes` (each field's
mismatch, the Agent/Task alias, absent command list, worker check still
fatal), `TestAllowedCommands`, `TestLeadCapabilitiesFromItsInit`,
`TestLeadOffersNoCommandOfAnUnsupportedItem`,
`TestProfilelessBackendStaysHandleOnly`, the catalog bound/fixture tests in
`catalog_test.go`, and the harness-notice block of `lead-popover.test.cjs`
(run by `TestLeadPopoverLaunchBehavior`: unverified with and without a
version, demoted alone and together, cleared on absent capabilities and on a
closed session). Runs on this Mac (go test, `-count=1`): `./internal/wsagent/`
ok at a98a31f86 (273 s); the other suites ran at 539ef4ad8, whose only later
change is the route fix, after which the `TestProxyHTTP*` tests, the
lead-popover and lucide tests and `TestHostedWorkerOverHTTPSeesOnlyItsProfile`
were rerun and pass: `./cmd/ws-mcp/` ok (164 s);
`./internal/wsi18n/` ok; `./internal/wsdashboard/` fails only
`TestSearchPaletteRenderedEntries`, `TestSearchPaletteBehavior` and
`TestBoardStateFilterAsks`, all three failing the same way on a tree from
before these phases; `./internal/mcp/` (`-timeout 30m`, 1009 s) fails
`TestDashboardNotesUsesHostedCallerStoreIdentity` (pre-existing) and, under
the load of concurrent suites, `TestHostedLiveBusyTurnFlushesEachHeartbeat`
and `TestIntakeStopsWithinRecheckOfARemovedKey` (a TempDir cleanup race),
both of which pass when rerun alone three times. The route tests also pass
under `-race`.

**Review.** Phase 3 lite review (4e8add8d3..539ef4ad8, covering the
e4a0dda94 replayDone narrowing and the Phase 2 fix commit): one Important, a
concurrent map write on the replay's pending id introduced by 539ef4ad8,
fixed in a98a31f86; one Minor (failed stale restart untested), answered with
`TestClaudeLeadStaleRouteFailedRestart`.

**Left open:** the session-menu copy of the warning (optional in the brief)
was not added; the notice was checked in the VM test, not in a browser.

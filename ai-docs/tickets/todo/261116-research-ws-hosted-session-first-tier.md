---
title: "Hosted sessions as the first tier: move features out of the plugin/extension path, retire the Pi extension, keep a limited Claude CLI compatibility tier"
parent: 261114-epic-ws-common-session-foundation
related:
  261110-epic-ws-dashboard-agent-host: "the existing first-tier agent host epic; this research asks which harness-side features the daemon should own once hosted sessions are the reference tier"
  261114-epic-ws-common-session-foundation: "CLI-plugin entry vs MCP-only hosted entry kept distinct (its Cross-Child Decisions); this research may change the weight between those two entries and must be reconciled with that epic before any child is cut"
  261112-epic-ws-mcp-pi-subagents: "done; pi.* is the existing daemon-owned spawn path and the closest prior art for a harness-neutral agent.spawn"
  261110-feat-ws-hosted-session-mode-flag: "done; the WS_HOSTED_SESSION gate that already moves input delivery and the dashboard feed from the Claude mod and the Pi extension to the daemon"
  261116-bug-ws-hosted-pi-exec-missing-native-worker-spawn: "a hosted Pi peer lead had no spawn surface for run-worker routing; a daemon-native spawn tool is one candidate answer"
  261115-feat-ws-worker-profile-session-spawn: "session-level workers (hosted sessions reporting by mailbox); overlaps with what a daemon-native spawn tool would cover"
  261115-feat-ws-session-hire-approval: "owner-approved hosted-session creation; authority boundary any agent.spawn must not bypass"
  261115-feat-ws-permission-profile: "common permission intent for Claude/Pi sessions; a tool-whitelist based spawn surface depends on it"
  261113-research-hosted-detection-by-session-identity: "hosted-mode detection; every feature moved to the daemon relies on the mod/extension reliably standing down in hosted mode"
  261115-feat-ws-hosted-codex-adapter-contract: "a further hosted harness; a common harness interface spec would apply to it"
  261114-feat-ws-conversation-audit-event-store: "daemon-side capture of every harness stream event; subagent tracking from the daemon feeds the same store"
  261116-feat-ws-daemon-plugin-package-select: "sibling draft (in progress) on which plugin package the daemon loads into hosted sessions"
  261117-feat-ws-harness-capability-catalog: "implements decision (07): the catalog as the common harness interface"
  261117-feat-ws-agent-tools-mailbox-routing: "implements the ws/agent.* delegation decisions (06), (09), (10)"
  261117-feat-ws-single-session-holder: "stage-2 child, prerequisite of exec"
  261117-feat-ws-exec-tools: "stage-2 child implementing ws/exec"
  261117-feat-ws-mailbox-memory-first-store: "independent child of the epic"
  261117-feat-ws-hosted-antigravity-adapter: "stage-3 child, revived by (02)/(05)"
---

# Hosted sessions as the first tier

## Background

Today ws features reach a lead through two harness-side channels: the Claude
mod (`agents-plugin/hooks/claude-mod/`, a Claude Code hooks module shipped in
the ws/wsflow plugin) and the Pi extension (`agents-plugin-pi/`, shipped as the
`native-pi` payload). Hosted sessions (leads the dashboard daemon spawns and
feeds through `wsagent.Runner` and the session holder) already take over input
delivery, mailbox delivery and the dashboard feed from both channels, but a
hosted lead still loads the same mod or extension for everything else.

Owner direction, 2026-11-16 23:02, when compaction nudges moving to the daemon
came up. Earlier in the same thread the owner noted that compaction is
something a user can push from outside (`/compact`) into any headless harness,
so much of this could come from a common harness interface spec. The owner then
widened the scope (the note was cut off after a trailing "이런 접근을 하면"):

> 정확히는 플러그인 쪽 경로에서 기능을 하나씩 하나씩 빼고, 기준을 hosted 세션 기준으로 잡자는 방향입니다. 즉, hosted 세션은 상당히 제어권을 많이 갖고 있으니까, 예를 들어 에이전트 도구까지도 하네스 내부 에이전트 스폰 툴에 기대지 않고 ws/agent.spawn... 같은 도구를 제공해서 네이티브 에이전트로 연결해줄 수 있고, 이러면 서브에이전트 추적까지 이쪽에서 꽤 자연스럽게 가져올 수 있다고 보거든요. (대부분의 하네스가 툴 화이트리스트는 제공을 하니까요)

Gist: pull features out of the plugin-side path one at a time and make the
hosted session the reference. Hosted sessions have a lot of control, so even
agent spawning need not rely on the harness's own spawn tool: ws can offer a
`ws/agent.spawn`-like tool that connects to native agents, which would bring
subagent tracking to the daemon fairly naturally (most harnesses offer a tool
whitelist).

Owner approval of this research, 2026-11-16 23:07:

> 네 hosted session을 장기적으로 1tier로 가져가고, pi extension쪽은 장기적으로는 아예 걷어내고, claude cli 정도만(회사에서도 쓰는 거라) 호환성으로 제한적인 기능과 함께 남기는 방향의 리서치입니다. 장기라고는 하지만 3, 4주 안에 착수할 걸로 생각하긴 하지만요 ...

Gist: hosted (daemon-launched) sessions become tier 1 in the long run; the Pi
extension is eventually removed entirely; the plain Claude CLI plugin path
stays as a compatibility tier with limited features (the owner uses it at
work). "Long term", but the start is expected within 3-4 weeks.

Removing the Pi extension, and dropping any feature from the Claude CLI tier,
is functionality removal under AGENTS.md's "Always ask" list. This ticket
records the direction; each concrete removal still needs the owner's explicit
approval per item.

## Current topology (survey 2026-11-16)

- **Hosted Claude lead.** `claudeLeadArgs`
  (`agents-plugin-tool/internal/wsagent/claude.go:102`) runs `claude -p` with
  stream-json in and out, `--permission-prompt-tool stdio`, and no `--tools` /
  disallow list, so the harness's native Agent tool is available.
  `claudeLeadPluginArgs` (`wsagent/claude_lead_plugins.go:25`, return at :49)
  passes `--plugin-dir <bundled package>` and disables installed ws/wsflow
  copies: **the hosted Claude lead loads the same Claude mod as a CLI session.**
  The mod resolves hosted mode (`claude-mod/hosted.ts:1-16`) and then turns
  off only its mailbox wait (`claude-mod/register.ts:990`) and dashboard feed
  (`register.ts:1220`); manual delivery, preload, the compaction lever and
  advisory, the band and the watchdog still run.
- **Hosted Pi lead.** `piLeadLaunch` (`wsagent/pi_lead.go:35`) loads the
  bundled ws Pi extension (`piBundledEntries`, `pi_lead.go:130`) with
  `--no-extensions` for everything else, and falls back to a plain
  `builtin:mcp` bridge with a loud "EXTENSION NOT LOADED" warning when the
  bundle is unavailable (`pi_lead.go:42-56`). **The hosted Pi lead depends on
  the Pi extension today.**
- **Daemon Pi workers (`pi.spawn`).** Child conversations run Pi in RPC mode
  with `--no-extensions`, `builtin:mcp` plus a generated MCP bridge, and an
  explicit `--tools` whitelist (`wsagent/pi.go:272-300`). They do not load the
  ws Pi extension. The daemon owns the whole tree (`wsagent/children.go:22`,
  `MaxSpawnDepth = 2` at :63).
- **Daemon Claude children.** The runner already starts non-lead Claude
  conversations with `--tools`, `--strict-mcp-config` and `--allowedTools`
  (`claudeArgs`, `claude.go:135-196`), used today only by builtin support and
  search-answer profiles (`wsagent/builtin.go:151`, `:199`). `pi.spawn` itself
  accepts only Pi profiles (`mcp/pi_tools.go:529`).
- **Pi extension's own delegation is already retired.** Its header says native
  child management, explore, fork and question-thread entry points are retired
  and general delegation is daemon `pi.*`; only `ws-execute`/`ws-approve` uses
  the local engine (`agents-plugin-pi/src/index.ts:1-7`,
  `agents-plugin-pi/src/spawner.ts:1-5`).

## Feature inventory

"Hosted control" says whether the daemon already has the hook it would need to
own the feature for a hosted lead. The last column is a non-authoritative
proposal (see Proposals), not a decision.

| Feature | Claude CLI path (mod / plugin) | Pi extension | Daemon / hosted today | Hosted control already present? | Candidate CLI-tier fate (proposal) |
|---|---|---|---|---|---|
| Mailbox push / drain | `claude-mod/watcher.ts:4` (mailbox wait), `register.ts:1176` bind tool | `agents-plugin-pi/src/mailbox-waiter.ts` | Daemon delivers mail as user turns (`LeadMail`, `wsagent/runner.go:69`); mod and extension stand down in hosted mode | Yes, already moved | Keep (CLI has no other wake path) |
| Dashboard feed (turns, tool calls, usage, subagents) | `claude-mod/dashboard.ts` via `dashboard_intake` (`dashboard.ts:95`) | `src/dashboard-feed.ts` | Daemon maps the stream itself (`claudeMapper`, `claude.go:515`); feed off in hosted (`register.ts:1220`) | Yes, already moved | Keep or reduce (open question) |
| Session status / background task counts | Mod feed | `src/background-tasks.ts` (ws-execute tasks), worker task counts | `background_tasks_changed`, `task_started`/`task_updated` mapped (`claude.go:881-911`); host state in live snapshot (`syncHost`, `runner_recovery.go:22`) | Yes for Claude; Pi lead task counts come through RPC | Keep minimal via feed |
| Subagent tracking / visibility | Mod feed reports subagent turns, model, effort, context | `src/hosted-agent-rows.ts` (widget channel because the daemon cannot see native Pi children) | Claude: native Agent tasks observed from stream (`claudeSpawn`, `claude.go:626`; `claudeAgentTask`, :1277), telemetry only, no ownership. Pi: `pi.*` children owned by the daemon | Partial: observe-only for native Claude subagents; full for `pi.*` | Proposal: a daemon-native `agent.spawn` replaces native spawn in hosted leads; CLI keeps native spawn |
| Worker / delegate spawn | Native Agent tool, bound by `native-spawn-binding.claude.md` and `SpawnIdiom` (`mcp/playbook_tools.go:33`) | Retired; daemon `pi.*` (`src/index.ts:1-7`) | `pi.spawn`/`send`/`wait`/`list`/`stop`/`transcript` (`mcp/pi_tools.go:20`, :529); `Runner.Spawn` (`wsagent/spawn.go:113`) | Yes for Pi children; Claude-backed children supported by the runner but not exposed as a spawn tool | Keep native spawn on CLI |
| Compaction lever (`request_compaction`) | Mod tool (`register.ts:1264`, `claude-mod/compaction.ts:29`) | Pi ws-owned compaction summary (`src/lead-compaction.ts`, via `session_before_compact` in `src/goal-loop.ts:67`) | Daemon recognises the lever by name (`claudeCompactionLever`, `wsagent/claude_alerts.go:20`) and holds input until the compaction starts (`runner.go:1396`); Pi `/compact` is an RPC command (`wsagent/pi.go:551`) | Partial: daemon observes the mod's lever; it can push `/compact` itself (Pi RPC; Claude typed line per `commandRunner`, `backend.go:164`) | Keep on CLI |
| Compaction advisory / milestone wake turns | Mod (`compaction.ts`, knob `claude.compaction_advisory_enabled`, `register.ts:181`) | None found | Daemon already has the inputs: context usage and the compaction window (`askContextUsage`, `claude.go:1131`) | Yes in principle (data + `/compact` push); no daemon advisory exists | Keep on CLI |
| Post-compaction revive nudge | `SessionStart(compact)` hook `agents-plugin/hooks/compact-revive-nudge.py` (`hooks/hooks.json`) | Pi compaction summary carries state | Daemon sees `compact_boundary` / compaction start-end in the stream | Yes in principle | Keep on CLI |
| MCP watchdog / reconnect | Mod watchdog and `request_mcp_reconnect` (`claude-mod/reconnect.ts:3`, :21; `register.ts:1342`) | Extension owns its ws-mcp subprocess (`src/bridge.ts`) | In-stream `mcp_reconnect`, then restart at idle (`runner_recovery.go:10`; `MCPStatus`/`ReconnectMCP`, `claude.go:444`, :465); daemon also recognises the reconnect lever (`claude_alerts.go:15`) | Yes; the mod's watchdog also runs in hosted mode, so both act today (unverified whether that duplicates work) | Keep on CLI |
| Status band | `claude-mod/band.tsx:3` (TUI band above the prompt) | `src/agent-footer.ts` (Pi footer) | Dashboard page shows usage/status from the daemon's live snapshot | Yes (dashboard is the hosted UI); the band likely draws nothing in `-p` mode (unverified) | Keep on CLI (TUI-only value) |
| Tool-call captions / display summaries | `claude-mod/captions.tsx` | `src/display-summary*.ts` | Summary ownership code exists daemon-side (`wsagent/runner_summary.go`, `mcp/summary_owner.go`) | Partial | Candidate to drop or keep optional |
| Workflow manual injection / session key | `claude-mod/manual.ts:1` (mints key, appends manual) | `src/lead-bootstrap.ts` (system prompt block) | Daemon mints the hosted key (`WS_HOSTED_SESSION_KEY`); mod adopts it (`hosted.ts:1-9`); Pi lead bridge adds hosted identity (`writePiLeadBridge`, `pi_lead.go:110`) | Partial: key yes; manual delivery still harness-side | Keep on CLI |
| Playbook preload | `claude-mod/preload.ts:1` | `src/playbook-read-dedupe.ts` | None | No | Keep on CLI |
| Ask / answer to owner | Claude `AskUserQuestion`; hosted leads get a deny-with-instruction (`claudeChatOnlyDenial`, `claude.go:508`) | `src/ask.ts` (owner-question thread registry; the extension header lists question-thread entry points as retired, `src/index.ts:3-6`) | Whiteboard ask through MCP (`mcp/whiteboard_tools.go`), harness-neutral | Yes via whiteboard; native forms not rendered | Whiteboard already harness-neutral |
| Whiteboard / tracker / notes / tickets | MCP tools only | Re-registered MCP tools (`src/bridge.ts`) | Served by ws-mcp (`mcp/whiteboard_*.go`, `mcp/tracker_*.go`) | Yes, harness-neutral | Unchanged |
| Permission / approval | Claude's own prompt in CLI | `ws-execute`/`ws-approve` gateway (`src/execute-gateway.ts`) | `can_use_tool` answered by the daemon (`AnswerApproval`, `claude.go:476`) | Yes for Claude hosted; Pi lead uses the extension gateway | Keep Claude-native on CLI |
| Pi-only extras | n/a | Goal loop (`src/goal-loop.ts`), web search/fetch (`src/web-tools.ts`), model catalog command (`src/index.ts:734`) | None | No | Need a disposition before Pi extension removal |

## Topic: what "tier 1 hosted" changes

- The mod/extension hosted gate already exists (`WS_HOSTED_SESSION`,
  `WS_HOSTED_SESSION_KEY`, `WS_HOSTED_SESSION_ROW`, shared by the mod, the Pi
  extension and ws-mcp). Moving a feature to the daemon means the daemon gains
  it and the harness-side copy stands down in hosted mode, the same pattern
  the mailbox wait and the dashboard feed already follow.
- A harness tool whitelist is what makes a daemon-native spawn tool enforceable
  in a hosted lead: Claude has `--tools` / `--allowedTools` /
  `--disallowedTools` (used today for children, `claude.go:167`, :190), and Pi
  has `--tools` / `--no-tools` (`pi.go:297-299`). The hosted lead launches
  pass neither today.
- Claude-backed children are already a runner capability (support and
  search-answer profiles), so a harness-neutral `agent.spawn` is mostly an MCP
  surface over `Runner.Spawn` plus profile/tier resolution, not a new
  process-management layer. This is an inference from the code read, not a
  tested claim.
- The Claude mod is shipped in both ws and wsflow; any mod change is subject
  to `ai-docs/manuals/wsflow-mirroring.md` and the shipped-surface boundary.

## Topic: what the Pi extension carries that has no daemon home yet

From the inventory: the `ws-execute` approval gateway, the ws-owned compaction
summary, the system-prompt bootstrap, the goal loop, web tools, display
summaries, the Pi footer, and the hosted subagent-row channel. Retiring the
extension while hosted Pi leads remain requires each to be moved, replaced, or
explicitly dropped (each a separate owner approval). Pi workers spawned by the
daemon already run without the extension.

## Topic: common harness interface

The owner's remark that `/compact` can be pushed into any headless harness
points at a common harness interface spec: input turn, steer, interrupt,
slash-command push, model/effort/permission controls, MCP status/reconnect,
and a stream of turn/tool/task/usage/compaction events. `wsagent` already has
most of this as per-backend controls (`processControls`, `claude.go:382`;
`commandRunner`, `turnProber`, `backend.go:164-175`) and
`261114-refactor-ws-common-session-interface` (done) defined the shared
access/identity/event-adapter boundary. Whether "common harness interface
spec" means formalising that existing boundary or something wider is open.

## Outcome Ledger

### Verified Findings

- A hosted Claude lead loads the bundled ws/wsflow Claude mod through
  `--plugin-dir` (`wsagent/claude_lead_plugins.go:49`); in hosted mode the mod
  disables only its mailbox wait and dashboard feed (`claude-mod/hosted.ts:1-9`,
  `register.ts:990`, :1220).
- A hosted Pi lead loads the bundled ws Pi extension (`wsagent/pi_lead.go:35`,
  :130) and has an MCP-only fallback that loses native lead tools
  (`pi_lead.go:42-56`).
- Daemon Pi workers run without the ws Pi extension, with an explicit
  `--tools` whitelist (`wsagent/pi.go:272-300`).
- The daemon already owns mailbox delivery, the dashboard feed, background
  task counts, MCP recovery and permission answers for hosted Claude leads
  (`runner.go:69`, `claude.go:881-911`, `runner_recovery.go:10`,
  `claude.go:476`).
- The daemon observes native Claude subagents from stream-json task lines but
  does not own or steer them (`claude.go:626`, :1277); it owns `pi.*`
  children fully (`children.go:22`).
- The daemon recognises the mod's compaction and reconnect levers by tool name
  (`claude_alerts.go:15-20`) and can push `/compact` to a Pi lead over RPC
  (`pi.go:551`). It has context-usage and compaction-window figures for Claude
  (`claude.go:1131`). No daemon-side compaction advisory exists.
- The runner can start Claude-backed non-lead children with a tool whitelist
  (`claude.go:135-196`); `pi.spawn` exposes Pi profiles only
  (`mcp/pi_tools.go:529`).
- The Pi extension's native child management is already retired in favour of
  daemon `pi.*` (`agents-plugin-pi/src/index.ts:1-7`).
- Hosted leads are launched without a tool whitelist (`claude.go:102-133`,
  `pi.go:366-380`).
- The composer's `/` slash picker is fed by the session adapter's advertised
  command list (`dashboard-composer.js:818-824`, the `cmds` region with name,
  description, source, layer); a selection posts `{command, args}` as a message
  (`dashboard-composer.js:900`, :927). The frontend holds no ws-side command
  source today.
- The ws MCP server implements no `prompts/*` methods (grep for `"prompts` in
  `agents-plugin-tool/internal/mcp/` finds nothing).

### Confirmed Decisions

- Hosted (daemon-launched) sessions become the tier-1 reference; features are
  moved out of the plugin/extension path one at a time (owner, 2026-11-16
  23:02 and 23:07).
- The Pi extension is to be removed entirely in the long run (owner,
  2026-11-16 23:07). Direction only: each concrete removal is still an
  "Always ask" item needing explicit owner approval.
- The plain Claude CLI plugin path stays as a compatibility tier with limited
  features (owner, 2026-11-16 23:07).
- Start is expected within 3-4 weeks of 2026-11-16 (owner, 2026-11-16 23:07).
- Agent spawning in hosted sessions goes through a ws-provided
  `ws/agent.spawn`-style tool connected to native agents rather than the
  harness's own spawn tool, so subagent tracking comes from the daemon; this
  relies on harness tool whitelists (owner, 2026-11-16 23:02). The tool's name
  and shape are not settled.
- In hosted (daemon-launched) sessions, workflow instructions reach the agent
  through MCP playbooks, not harness skills/plugins; the agent loads playbooks
  directly (skills are already thin wrappers over `playbook.read` /
  `workflow_manual`) (owner, 2026-11-17 05:17).
- User-facing entry points for hosted sessions are a curated set of `lead-*`
  commands injected into the dashboard composer's `/` slash autocomplete list
  (owner, 2026-11-17).
- Goal: per-harness plugin loading stops mattering for hosted sessions; the
  requirement is an MCP connection plus daemon-side automatic injection where
  needed (e.g. the workflow manual) (owner, 2026-11-17).

Confirmed in the lead-discuss session of 2026-11-17 (board items (01)-(25);
epic `261114-epic-ws-common-session-foundation` carries the full set). The
resolved Open Questions are mapped by content.

- (01)=a Hosted-first: the daemon-hosted session is the reference tier. Common
  interface work targets hosted sessions (MCP, daemon injection, playbooks,
  daemon `agent.spawn`). Removal work (Pi extension, CLI-tier features) is
  approved per item, never bundled. Resolves former OQ8 (reconcile with the
  epic): the epic is revised in place (05) and its CLI-plugin versus MCP-only
  hosted distinction now reads as hosted-first with the CLI tier frozen.
- (02) Codex and Antigravity will both be supported as tier-1 hosted harnesses
  after hosted common-ization; Antigravity is not dropped (new ticket
  `261117-feat-ws-hosted-antigravity-adapter`, since the earlier ticket in
  `.dropped/` is immutable).
- (03) The Codex adapter decision (799)=a is not confirmed; the Codex
  version/protocol choice is asked when stage 3 starts.
- (04)=a Non-hosted tiers: the Claude CLI plugin is frozen (keep current
  features, add none); the Pi extension is retired after hosted parity, each
  concrete removal still needing per-item approval; Pi remains only as a hosted
  lead/worker harness. Resolves former OQ1 (Pi stays a hosted lead harness) and
  OQ2 (the Claude CLI tier is a frozen set, not a shrinking one).
- (05) Epic `261114-epic-ws-common-session-foundation` is revised in place; this
  research is its evidence child; Antigravity returns as a stage-3 child.
- (06)=a, (09)=a, (10)=c, (11), (12) Delegation is always `ws/agent.*`
  regardless of harness: `agent.spawn`, `agent.stop`, `agent.list`, with no
  `wait`, no `transcript` and no `agent.send` (a single `mailbox.send(to:
  <address>)` serves children, peers and leads). It is a new family and `pi.*`
  stays as a behavior-preserving alias (T0). Native subagents are removed in
  hosted leads (`--disallowedTools Agent` and `Task` for Claude). Hiring
  (`session.request_hire`, peer, no lineage) stays separate; worker-profile caps
  apply to `agent.spawn`. No depth cap: `MaxSpawnDepth` is removed and a
  per-root-tree concurrent-agent cap config key bounds runaway (default by
  measurement). Resolves former OQ3 (tool name and shape), the whitelist half of
  former OQ4 (native spawn removed by `--disallowedTools`, not left alongside)
  and former OQ7 (interaction with hiring, worker profiles and the depth cap).
  Implemented by `261117-feat-ws-agent-tools-mailbox-routing`.
- (07) ok, (08)=c, (20)-(22), (24) The common harness interface is a
  declarative capability catalog promoting the existing `wsagent` optional
  controls into an explicit table (items declared `native` / `ws-substitute` /
  `unsupported` plus a range, per `harness@version` with `since` / `until`),
  not a wider external spec. Admission conditions are headless JSON event
  streaming and HTTP MCP injection (direct JSON-RPC, per-session token). Out-of-
  range versions run loudly with the nearest profile. Harnesses whose native
  subagents cannot be removed are decided per harness at stage 3. Resolves
  former OQ5. Implemented by `261117-feat-ws-harness-capability-catalog`.
- (25) Implementation and verification order is Claude, Codex, Antigravity, then
  Pi; Pi is driven only with GPT models, so the Pi hosted backend is a port of
  the Codex adapter, not a Pi-first design.
- (16)-(19) `ws/exec.*` and the single PC-wide session holder are stage-2
  features (`261117-feat-ws-exec-tools`, `261117-feat-ws-single-session-holder`);
  the mailbox memory-first store is an independent child
  (`261117-feat-ws-mailbox-memory-first-store`, (13)=c, (15)).

### Proposals

Non-authoritative candidates from the survey.

- Migration order, cheapest first: (1) a daemon-side compaction advisory and
  revive nudge using the usage figures it already reads, with the mod standing
  down in hosted mode; (2) MCP watchdog stand-down in hosted mode (the daemon
  already recovers); (3) daemon-native `agent.spawn` generalising `pi.*` to
  Claude-backed children, with the hosted lead's native Agent tool removed by
  whitelist; (4) manual delivery and playbook preload through the daemon's
  instruction file / turn injection; (5) Pi extension retirement once its
  remaining features (inventory last row, execute gateway, compaction summary,
  bootstrap) have a home or an approved drop.
- `agent.spawn` as a superset of `pi.*`: same id/send/wait/list/stop/transcript
  verbs and completion mail, a `harness` or profile-selected backend, tiers
  resolved from the matching `agents.tier` bucket. Keep `pi.*` as a
  behaviour-preserving alias (T0) during the transition.
- Claude CLI tier keeps: mailbox push and drain, compaction lever and advisory,
  revive nudge, MCP watchdog/reconnect, manual injection and preload, status
  band, native Agent spawn with mod-fed subagent telemetry. Candidates to stop
  growing on the CLI tier: captions and any new dashboard-only telemetry.
- Track each moved feature with a hosted-mode stand-down in the mod so a
  hosted lead never runs both copies.
- Daemon-side slash expansion: add the ws `lead-*` set as another source for
  the composer picker; the daemon, not the frontend, expands
  `{command: "lead-ship", args}` into a harness-neutral turn ("read playbook
  lead-ship and execute"). The list and expansion rule then live in one place,
  every client (dashboard, Electron) behaves the same, and history shows
  `/lead-ship`.
- Replace what skills provide today: (1) model-side routing: skill
  descriptions sit in the system prompt so the model picks e.g. lead-ship from
  "release this"; move that routing table into the MCP initialize
  `instructions` or the workflow manual; (2) hooks (post-compaction revive
  nudge, response-language hooks) are not MCP, so the daemon injects turns from
  stream events (same path as the compaction advisory proposal above); (3)
  plugin-shipped subagent definitions (effort-tier agents) are replaced by the
  already-decided daemon-owned `agent.spawn`.
- The Claude CLI compatibility tier keeps the plugin; playbooks remain the
  single source and skills are generated shims, so dual-maintenance cost stays
  small.

### Open Questions

Former OQ1, OQ2, OQ3, OQ5, OQ7 and OQ8 are resolved (see Confirmed Decisions);
numbering of the remainder is kept stable.

4. (Residual) How do playbooks' `SpawnIdiom` / native-spawn bindings route per
   tier without naming this repository (shipped-surface boundary)? The
   whitelist half of the question is resolved: hosted leads remove the native
   spawn tool.
6. Should hosted leads keep loading the Claude mod at all once its features
   move, or load a reduced hosted profile? This is about the Claude mod in
   hosted sessions, not the Pi extension's fate, so decision (04) does not
   settle it. Interacts with `261116-feat-ws-daemon-plugin-package-select`.
9. Does Claude Code expand MCP prompts (`prompts/list`) as slash commands in
   `-p` stream-json input? If so the CLI tier could get the same command list
   via MCP prompts (the ws MCP server implements no `prompts/*` today). Needs a
   probe.
10. What exactly goes into the injected routing table, and what is its size
    budget in MCP instructions versus the workflow manual?
11. Which `lead-*` commands are curated for the slash list, and are their
    arguments free text?
12. Pi extension retirement parity criteria: the concrete list of what must hold
    before the Pi extension is retired (feature disposition for the `ws-execute`
    gateway, ws-owned compaction summary, bootstrap, goal loop, web tools, display
    summaries, footer and hosted-agent-row channel; the outcome of the Pi HTTP
    MCP probe in `261117-feat-ws-harness-capability-catalog` Phase 1). Each
    removal still needs per-item owner approval (04).

### Rejected Alternatives

## Unverified

- Whether the Claude mod's status band and captions render anything in a
  `-p` stream-json session.
- Whether the mod's MCP watchdog and the daemon's MCP recovery both act on
  the same broken server in a hosted Claude lead.
- Whether every harness the daemon may host offers a tool whitelist strong
  enough to remove its native spawn tool (verified only for the Claude and Pi
  flags the runner already uses for children).

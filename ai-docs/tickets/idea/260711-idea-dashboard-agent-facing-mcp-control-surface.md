---
title: "Dashboard-owned MCP surface for agent-driven dashboard control (open-file, execution approval, worktree management)"
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: read-only Activity/agent-client provider track this idea's write/control direction sits next to and must not be confused with
  260711-epic-ws-dashboard-command-surface: sibling epic owning the human-facing command bus/custom-button UI this control surface would let agents trigger; kept separate from this ticket's 260622 parent by owner decision (2026-07-11)
  260711-idea-dashboard-command-bus-quick-open-shortcuts: human-facing command bus/custom-button UI this control surface would let agents trigger
  260605-research-ws-native-subagent-pivot: migration anchor; distinguishes this from ws's own agents.*/mercenary subprocess-spawn MCP surface
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
---

# Dashboard-owned MCP surface for agent-driven dashboard control

## Background

Owner direction (2026-07-11 discussion): it would be useful, long-term,
for **ws-dashboard itself** to expose at least one MCP server so agents
running in a dashboard-managed session can drive dashboard-level actions
directly, instead of everything staying human-click-only. Named examples
from the discussion:

- an agent asking the dashboard to open a file and show it (in the
  existing document viewer surface) rather than just printing content to
  a terminal;
- an agent requesting execution approval through the dashboard rather
  than (or in addition to) an in-terminal prompt;
- unifying worktree management operations (which already exist as
  daemon-side git/worktree routes for human-driven UI) behind one
  agent-callable surface, alongside the same custom-command-button
  mechanism from `260711-idea-dashboard-command-bus-quick-open-shortcuts`.

This is explicitly **not** the same thing as either:

- `ai-docs/spec/mcp-tools.md` / `plugin-runtime.md` / `ref/ws-mcp.md` —
  those describe the `agents-plugin-tool` workflow-orchestration MCP
  server (skills, tickets, ferrule/session-key plumbing), unrelated to
  ws-dashboard as an application.
- `260620-feat-ws-dashboard-agent-client-activity-sources` — that track
  is the dashboard *reading from* host-owned agent-client providers
  (Codex app-server, OpenCode ACP) to render Activity, and is explicitly
  scoped read-only ("exposing start, interrupt, cancel, retry, erase,
  permission approval, or provider-specific steering controls in the
  dashboard UI requires later high-friction control tickets" —
  `260620` Decisions section).

This idea is the inverse direction: the dashboard becomes an MCP
**server** that agents call *into*, not a client that reads *from*
agents. Confirmed today: no such surface exists in `ws-dashboard` at all
(daemon has no MCP server code or endpoint of any kind).

## Tension to surface (per AGENTS.md Response Discipline: surface conflicts, don't silently resolve them)

`260622-epic-ws-dashboard-session-key-realignment`'s Non-Scope explicitly
excludes "Making the dashboard the ws MCP root authority, model backend
authority, or hidden orchestrator above top-level harness sessions," and
`260620`'s Decisions gate any control/approval-granting UI behind
"later high-friction control tickets" precisely because approval/steering
controls historically reopened harness-development scope creep (the thing
the 260605 pivot moved away from).

The three example capabilities differ in how much tension they create
with that constraint:

- **Open-file-and-show**: low tension. This is a read/display action
  routed through the existing document-viewer surface; it does not grant
  an agent control over harness lifecycle or session authority.
- **Worktree management API unification**: likely low-to-medium tension,
  since equivalent daemon routes already exist for human-driven UI
  (`root_picker.rs`, `discovery.rs`, git/worktree routes) — exposing them
  to an agent is an authorization question more than an architecture
  question, as long as the dashboard is not thereby made the authority
  that mints or governs ws session keys.
- **Execution approval**: highest tension. This is exactly the kind of
  "permission/approval-granting control" that `260620` explicitly defers
  as a high-friction follow-up, and doing it well requires deciding
  whether the dashboard becomes an approval *relay* (forwarding an
  approval decision made elsewhere) versus an approval *authority*
  (deciding for itself) — the latter reads like the "hidden orchestrator"
  shape `260622` rules out.

This ticket does not resolve the tension; it records the owner's forward
direction so a future implementation-ready pass can address open-file and
worktree-management first (lower tension) and treat execution-approval as
a separate, explicitly-flagged high-friction child once the read-only
Activity track (`260620`) and the command-bus UI
(`260711-idea-dashboard-command-bus-quick-open-shortcuts`) have landed.

## Decisions

- Owner policy (2026-07-11), a hard floor, not a per-implementation
  judgment call: any agent-initiated **registration** of a new persistent
  custom command, and any agent-initiated **execution** of a custom
  command (human- or agent-registered) through this MCP surface, always
  requires explicit human approval before taking effect. This directly
  narrows the "custom command buttons" slice of the execution-approval
  tension above — it does not resolve the broader question of whether
  the dashboard is an approval *relay* or *authority* for other kinds of
  execution (e.g. arbitrary shell commands typed by an agent outside the
  registered-custom-command mechanism), which remains open. This decision
  is mirrored in `260711-epic-ws-dashboard-command-surface`'s
  Cross-Child Decisions so it is visible from either side of the
  UI/MCP split.

- Owner clarification (2026-07-11) on the broader execution-approval
  question: individual harnesses (Codex, Claude, etc.) likely already
  have their own permission systems, and an ACP-style integration would
  surface approval prompts to the user directly anyway. So the settled
  direction is: execution-approval requests themselves always stay
  human-relay-based (the dashboard does not decide on its own — the
  "authority" branch of the tension above is rejected as a general
  policy, not just for custom commands). The safe way to reduce prompt
  fatigue is a **frontend-side "auto-approve macro"** convenience
  feature: a human explicitly authors a pattern/rule ahead of time (e.g.
  "auto-approve `npm test`"), and the dashboard applies *that
  human-authored rule* to skip a redundant prompt. This keeps the
  decision-maker human (whoever wrote the macro), so it does not turn the
  dashboard into an autonomous approval authority — the macro is a
  recorded human decision applied ahead of time, not the dashboard
  deciding for itself. This is a distinct, later idea (UI for authoring
  and managing these macros, scoping them per-workroot or per-command
  pattern, auditability of which macro fired) and is not designed here.

- **Execution-approval prompt shape** (owner, 2026-07-11): keep the
  first implementation simple and aligned with what current mainstream
  harnesses already do — a 3-way choice per prompt: **allow once** /
  **allow this command pattern going forward** (the concrete mechanism
  behind "allow this command pattern" is exactly the auto-approve-macro
  concept above — choosing it authors a macro on the spot rather than
  requiring the human to pre-author one) / **deny**. Separately, a
  **"dangerously bypass"** mode exists as an explicit, human-toggled
  frontend switch that auto-approves everything for a session/workroot
  without per-prompt relay — distinct from the per-pattern macro
  mechanism, and named/framed as a deliberately risky opt-in (mirroring
  existing harness conventions for this kind of blanket bypass), not a
  default.
- **Future stretch, explicitly deferred** (owner, 2026-07-11): an
  independent-LLM risk pre-assessment step — a separate model call
  evaluates a pending command/approval request and produces a yes/no
  verdict before (or instead of) relaying to the human, similar to
  Claude Code's own auto-approve-style mode. Owner judged this as
  something that can be slotted in later without disturbing the 3-way +
  bypass shape above (it would sit as an optional pre-filter ahead of the
  human relay, not replace it), so it is not designed further here.
- **Confirmed technical grounding for the Claude provider** (research,
  2026-07-11, see `260620-feat-ws-dashboard-agent-client-activity-sources`
  Phase 4): the `claude` CLI's `PreToolUse` hooks (`.claude/settings.json`,
  confirmed to work when driving the raw CLI binary, not an SDK-only
  feature) are the officially-supported interception point for this
  3-way relay — a hook returns allow/block/ask per invocation. The
  dangerously-bypass mode maps to `--permission-mode bypassPermissions`,
  which is confirmed to have known headless failure modes when a prompt
  still needs to surface (process halts with no human-response surface in
  TTY-less stream-json contexts) — reinforcing that bypass should stay an
  explicit, rare opt-in rather than a normal relay path.

## Open Points

- What transport: does the dashboard daemon host an MCP server directly
  (stdio or HTTP-based), or does it register with an existing host-neutral
  MCP aggregation point? Given this repo's "host-neutral first"
  architecture rule, prefer whatever shape existing MCP tooling in
  `agents-plugin-tool` already uses, if reusable, over inventing a new
  transport.
- Authn/authz for this new surface: today's daemon has a `--no-auth` debug
  mode; an agent-callable control surface needs its own authorization
  story independent of that mode, especially for anything beyond
  open-file-and-show.
- Whether "worktree management" here means read (list/status) or also
  write (create/remove/switch) operations — write operations raise the
  same authority questions as execution approval and should probably be
  split into their own phase.
- Sequencing against `260620` and
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`: this MCP
  surface is more useful once the human-facing command-bus/custom-button
  mechanism exists to define *what* an agent can trigger, so treating this
  as a later phase of that work (or a shared backend) may be more
  efficient than building it standalone first.

## Non-Goals (for this idea ticket itself)

- Deciding the transport, authz model, or phase breakdown now — this is
  a direction-recording ticket, not yet implementation-ready.
- Making the dashboard a ws session-key authority or harness orchestrator;
  any later spec must keep the dashboard as a control-surface consumer of
  the existing ferrule/session-key model per `260622`, not a competing
  authority.

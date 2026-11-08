---
title: "ws daemon agent runner: profile-driven headless agents (claude, pi) hosted by the dashboard daemon"
related:
  261108-feat-ws-tracker-support-agent-chat: first consumer (tracker support-agent chat card)
  260908-feat-ws-pi-claude-code-lead-provider: prior billing finding (system prompt content routed to extra usage)
  261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory: prior headless claude delegate, retired as unstable
  261106-feat-ws-dashboard-claude-adapter: live dashboard Claude stream-json adapter
---

# ws daemon agent runner: profile-driven headless agents (claude, pi) hosted by the dashboard daemon

## Background

The user (2026-11-08, board topic "트래커 로컬 서포트 에이전트 채팅")
wants a daemon-side local support agent for the tracker, controlled through
the dashboard daemon's REST API, and asked that the launch machinery be a
generic agent execution framework rather than a chat-only feature, because
other daemon-side agents are already in view (a local indexing agent first).
This ticket is that framework; the tracker chat card
(`261108-feat-ws-tracker-support-agent-chat`) is its first consumer.

An agent here is a headless harness process (Claude Code or Pi) the daemon
starts from a named profile, feeds user turns, and whose output it records
in the dashboard's existing live-state format so the existing conversation
renderer shows it.

## Decisions

- **Standalone ticket, consumers depend on it** (board (306) ok): the runner
  is not a tracker epic child, since its consumers are not all tracker
  features. Consumers carry `blocked-by` on this ticket.
- **Backends: claude and pi, chosen per profile** (board (307) b): the backend
  interface is designed for both from the start; the claude backend lands
  first (Phase 1) and pi second (Phase 2). Rejected: claude only, which
  leaves Pi-only downstream projects without the feature (Architecture Rule 3,
  host-neutral first).
- **Sustained process with an idle-timeout knob** (board (308) a, user note:
  mailbox and other features are tied to process lifetime): one long-lived
  backend process per agent conversation, fed turns over a bidirectional
  stream. A per-profile idle timeout ends the process after that long with
  no running turn and no input; `0` disables the idle exit (a primary,
  always-on agent). A later input restarts the process on the same backend
  session (claude `--resume <session-id>`). Rejected: a new process per turn
  with `--resume` (the lead's recommendation, rejected by the user; the prior
  research `260908-research-ws-pi-claude-code-lead-provider` also rejected it
  for per-turn startup cost).
- **Instructions through `--append-system-prompt-file`, never `--bare`**
  (board (311) ok, superseding (309)): the claude backend keeps Claude Code's
  default system prompt and appends the profile's instruction file. The
  appended prompt is recorded once and reused on resume
  (`--system-prompt-snapshot`, on by default in Claude Code 2.1.319) and
  survives compaction. `--bare` is never passed: it restricts auth to
  `ANTHROPIC_API_KEY` or an apiKeyHelper and never reads OAuth or the
  keychain (`claude --help`, 2.1.319). `--system-prompt` (replace) is never
  passed. Rejected: instructions in the first user turn (they fade at
  compaction), and replacing the system prompt.
- **Billing evidence** (git history, 2026-11-08): the discarded Pi
  `claude-code` provider (`98a5306f0`, reverted `238f2ce3b`) got
  `400 You're out of extra usage` on every subscription turn because the
  *content* of its system prompt (Pi's built-in harness prompt) is classified
  as third-party-harness usage; neither a flag nor the auth path caused it,
  and `{preset: claude_code, append: <Pi prompt>}` also failed. The later
  ws-claude delegate used the `claude_code` preset with an appended task
  frame and ran on the subscription. Consequences for this ticket:
  - profile instruction files are task instructions, never a harness
    persona or a copy of another harness's system prompt;
  - no attempt to shape or trim a prompt to evade the classification
    (the prior ticket declined that as evading a deliberate usage control);
  - Phase 1 verifies the billing path live before anything builds on it.
- **"All off" loading, each part a profile knob** (board (312) ok): the
  claude backend starts with `--disable-slash-commands` (skills and
  commands off), `--strict-mcp-config --mcp-config <only the servers the
  profile allows>`, and plugins off through `--settings`. Prior evidence:
  `settingSources: []` alone still attached the account's claude.ai connector
  MCP servers; only strict MCP config removed them
  (`260908-research-ws-pi-claude-code-lead-provider`). The pi backend follows
  the Pi spawner's launch shape (`--no-extensions`, an explicit `--extension`
  only when the profile names one, `--tools <allowlist>`; see
  `agents-plugin-pi/src/spawner.ts`, search `--no-extensions`).
- **Profiles are config knobs** (board (306) ok): a profile family
  `agent.profile.<name>.*` holding at least backend, model, effort, tool
  allowlist, MCP servers allowed (default none), plugins/skills/extensions
  (default off), instruction file, and idle timeout. The ws and wsflow
  config manifests carry the same keys and differ only in defaults
  (epic `261107-epic-ws-personal-common-store` Config decision, (138)).
- **Output through `internal/wslive`** (user direction, board topic): each
  agent conversation gets its own dashboard session directory and the runner
  writes its snapshot and ring with the `wslive` writers, so the existing
  conversation renderer and SSE serve it unchanged.
- **Control through the daemon's REST API** (user direction): consumers
  start, send to, interrupt, and stop agent conversations over daemon routes.

## Open Decision Queue

Last ID: (319)

(313) [open] [technical] Two phases: Phase 1 runner core, profiles, REST control, wslive output, and the claude backend with a live billing probe that stops the ticket if a turn is billed as extra usage; Phase 2 the pi backend - over one phase, because the billing outcome gates everything built on the claude path.
(314) [open] [technical] REST routes sit behind the daemon's existing access control (loopback, and the LAN token when LAN is on) - over a separate auth scheme, since the dashboard already gates every route that way.
(315) [open] [technical] An agent conversation's session directory is keyed by a minted reply-id under the daemon state, like a lead session's - over a new store, so `wslive` readers and paging work unchanged.
(316) [open] [policy] Runner-hosted conversations are not listed in the dashboard session index by default; a consumer embeds them where it shows them, and a profile knob can list them.
- Context: the session index lists lead sessions; a support or indexing agent listed there is noise for most users.
- Alternative: always list them as sessions; never list them.
> Not listed by default with a knob: consumers own their placement, and debugging can still list them.
(317) [open] [policy] Tool permissions are allowlist-only: tools the profile allows run, everything else is denied without a prompt (claude `--permission-mode dontAsk` with `--allowedTools`; pi `--tools`).
- Context: a headless process has no terminal to answer a permission prompt; the prior ws-claude delegate used `dontAsk`.
- Alternative: route permission prompts to the dashboard for the user to answer.
> Allowlist-only for v1: no prompt plumbing, a predictable failure mode, and routing prompts can be added later.
(318) [open] [technical] On daemon restart, sustained processes end with the daemon and each conversation restarts lazily on its next input with the backend's resume - over re-spawning every conversation at daemon start, which would wake idle agents for nothing.
(319) [open] [technical] A model or effort change from a consumer applies at the next turn boundary by restarting the process on the same backend session - over in-stream switching, which `claude --help` does not document (unverified; Phase 1 checks for an in-stream control first).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`, `agents-plugin/`, or `agents-plugin-wsflow/`;
  `ai-docs/manuals/wsflow-mirroring.md` for the wsflow config manifest; and
  `ai-docs/manuals/ws-mcp.md` if `internal/mcp/` is touched.
- The runner lives in the dashboard daemon (`agents-plugin-tool/internal/wsdashboard`
  or a new package it owns); no consumer UI is in scope here.
- Prior lessons to honor (`260908-research-ws-pi-claude-code-lead-provider`):
  close the child cleanly on shutdown (a held stream kept the parent alive);
  verify the child's init message lists only the permitted tools and MCP
  servers and fail the launch otherwise (the ws-claude profile check).
- Rejected earlier, do not revive: running Claude Code with another
  harness's system prompt; `--bare`.

## Phases

### Phase 1: runner core and the claude backend

To be written once the queue settles.

### Phase 2: pi backend

To be written once the queue settles.

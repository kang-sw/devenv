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

- **REST routes behind the daemon's existing access control** (board (314)
  ok): loopback, plus the LAN token when LAN is on, the same gate every
  dashboard route uses. Rejected: a separate auth scheme for agent routes.
- **One minted reply-id session directory per conversation** (board (315)
  ok): the runner mints a reply-id and writes under `wslive.SessionDir`
  in the daemon state, like a lead session's, so `wslive` readers, paging,
  and SSE work unchanged. Rejected: a new store for agent output.
- **Hidden from the session index by default** (board (316) a): a consumer
  embeds its conversations where it shows them; a profile knob lists them in
  the dashboard session index (for debugging). Rejected: always listed;
  never listed.
- **Allowlist-only tool permissions** (board (317) a): tools the profile
  allows run, everything else is denied without a prompt (claude
  `--permission-mode dontAsk` with `--allowedTools`; pi `--tools`). A
  headless process has no terminal to answer a prompt. Rejected for v1:
  routing permission prompts to the dashboard.
- **Lazy resume after a daemon restart** (board (318) ok): sustained
  processes end with the daemon; each conversation restarts on its next
  input with the backend's resume. Rejected: re-spawning every conversation
  at daemon start, which wakes idle agents for nothing.
- **Model or effort change at a turn boundary** (board (319) ok): a change
  applies at the next turn boundary by restarting the process on the same
  backend session. Phase 1 first checks for an in-stream control and uses it
  if one exists. Rejected: assuming in-stream switching, which
  `claude --help` (2.1.319) does not document.
- **Two phases, billing-gated** (board (313) ok): Phase 1 is the runner core
  and the claude backend with a live billing probe that stops the ticket if
  a turn is billed as extra usage; Phase 2 is the pi backend. Rejected: one
  phase, since the billing outcome gates everything built on the claude
  path.

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

1. **Billing probe first.** Before building the runner, launch Claude Code
   headless with the Phase 1 flag set (stream-json in and out,
   `--append-system-prompt-file` with a short task-instruction file,
   `--disable-slash-commands`, `--strict-mcp-config --mcp-config` with an
   empty server set, `--permission-mode dontAsk`, plugins off through
   `--settings`) on the user's subscription login, run one turn, and resume
   it once with `--resume`. If any turn is billed as extra usage or refused
   for usage, stop and report with the raw error; nothing below is built.
   Also record whether `--settings` turns plugins off and whether an
   in-stream model or effort control exists ((319)).
2. **Profiles.** Add the `agent.profile.<name>.*` family to the ws and
   wsflow config manifests (same keys, defaults differ only where the
   packages already differ): backend, model, effort, tool allowlist, MCP
   servers (default none), plugins/skills/extensions (default off),
   instruction file, idle timeout (`0` = never), and the session-index
   listing knob ((316)). Unknown backend or a missing instruction file fails
   the launch with a clear error.
3. **Runner core** in the dashboard daemon: a backend interface (start,
   send turn, interrupt, stop, resume by backend session id) designed for
   both claude and pi; one sustained process per conversation with the idle
   timer; a minted reply-id session directory per conversation ((315));
   lazy resume on the next input after an idle exit or a daemon restart
   ((318)); model/effort changes at the turn boundary ((319)); clean child
   shutdown with the daemon.
4. **Claude backend**: the flag set from step 1; parse the stream-json
   output into `wslive` snapshot and ring entries (user, reply, turn, tools)
   so the existing conversation renderer shows them; verify the child's init
   message lists only the profile's tools and MCP servers and fail the
   launch otherwise.
5. **REST control** behind the existing access control ((314)): create a
   conversation from a profile, send a turn, interrupt, stop, change model or
   effort, and read state (the session directory key, running or idle).
   Runner conversations stay out of the session index unless the profile
   lists them.

Verification: unit tests for profile parsing, the idle timer, the
lazy-resume path, and stream-json-to-`wslive` mapping (with recorded
stream-json fixtures); a fake backend drives the REST routes end to end in
tests; one live run of the claude backend through the REST routes on the
subscription login, recorded in `### Result` with the billing observation.

### Phase 2: pi backend

Implement the backend interface for Pi following the Pi spawner's launch
shape (`agents-plugin-pi/src/spawner.ts`: RPC mode, `--session`,
`--session-dir`, `--append-system-prompt`, `--no-extensions`, an explicit
`--extension` only when the profile names one, `--tools <allowlist>`), map
its RPC events to `wslive` entries, and resume by Pi session. Verification:
the Phase 1 test suite runs against the pi backend through the same fake and
recorded-event fixtures, plus one live Pi run through the REST routes,
recorded in `### Result`.

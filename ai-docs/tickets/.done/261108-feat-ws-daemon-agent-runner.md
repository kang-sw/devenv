---
title: "ws daemon agent runner: profile-driven headless agents (claude, pi) hosted by the dashboard daemon"
related:
  261108-feat-ws-tracker-support-agent-chat: first consumer (tracker support-agent chat card)
  260908-feat-ws-pi-claude-code-lead-provider: prior billing finding (system prompt content routed to extra usage)
  261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory: prior headless claude delegate, retired as unstable
  261106-feat-ws-dashboard-claude-adapter: live dashboard Claude stream-json adapter
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 72e011e469bf168d
sage-review-completeness-reviewed: 72e011e469bf168d
completed: 2026-11-08
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
- **Profiles are a built-in structured config family** (board (306) ok,
  (329) a): `agent.profile.<name>`, one object per profile read by ws-mcp
  code, following the `worktree.profile.<name>` precedent
  (`internal/mcp/worktree_profile.go`, structured overrides in
  `config_registry.go`). A profile holds at least backend, model, effort,
  tool allowlist (built-in and ws tools), MCP servers allowed (default
  none), plugins/skills/extensions (default off), instruction file, idle
  timeout, and the session-index listing knob. No profile ships by default;
  each consumer ticket ships its own profile, and users override or add
  profiles through config. Rejected: declaring the family in the ws and
  wsflow adapter config manifests with per-package defaults (the original
  (306) wording), because the manifest schema declares exact keys only, one
  namespace per file (`internal/mcp/config_manifest.go`), and a
  parameterized family would change a contract downstream adapters see.
- **Output through `internal/wslive`** (user direction, board topic): each
  agent conversation gets its own dashboard session directory and the runner
  writes its snapshot and ring through `wslive` (the reducer `wslive.NewState`
  and `State.Snapshot`, and `wslive.AppendRing`; `wslive` has no snapshot file
  writer, the only one today is `writeLiveSnapshot` in
  `agents-plugin-tool/internal/mcp/live_snapshot.go#L450-L456`, so the runner
  writes `wslive.SnapshotPath(dir)` itself), so the existing
  conversation renderer and SSE serve it unchanged.
- **Control through the daemon's REST API** (user direction): consumers
  start, send to, interrupt, and stop agent conversations over daemon routes.

- **REST routes behind the daemon's existing access control** (board (314)
  ok): loopback, plus the LAN token when LAN is on, the same gate every
  dashboard route uses. Rejected: a separate auth scheme for agent routes.
- **One minted reply-id session directory per conversation** (board (315)
  ok): the runner mints a reply-id and writes under `wsdashboard.SessionDir`
  (`agents-plugin-tool/internal/wsdashboard/registry.go#L101-L103`, not a `wslive` symbol)
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
- **A conversation is bound to a working root** (board (330) a): the
  create route carries a working root; the profile resolves through that
  root's config layers (project when present, else the global ones), the
  child runs with that root as cwd, a relative instruction-file path
  resolves against it, and the attached ws-mcp serves that root. The
  tracker chat passes the store's local clone; an indexing agent passes its
  project. Rejected: global-only profiles that each name a working
  directory. One machine-wide daemon serves every session and reads no
  config today (`cmd/ws-mcp/dashboard.go` takes only port, LAN, and build
  time), so resolution must never depend on which harness started it.
- **Sends during a running turn are held** (board (327) ok): a send while a
  turn runs is held and submitted as the next turn when the running turn
  ends, several held sends merging into one; never rejected and never
  injected into the live turn. This matches the dashboard composer's
  mid-turn behaviour for lead sessions, which the chat card reuses.
- **`stop` and `close` are separate** (board (328) a): `stop` ends the
  process only, and the conversation resumes lazily on the next send like
  an idle exit; `close` ends the conversation for good, later sends are
  refused, and the session directory stays readable. A consumer's "new
  conversation" is `close` plus create. Rejected: a single `stop` meaning
  either one.
- **ws-mcp attached with the profile's allowlist as a surface filter**
  (board (326) ok, user: "새로 열리는 에이전트 세션에 ws-mcp를 제한된
  프로필로 붙이는 방식"): a profile that allows ws tools gets ws-mcp over
  stdio in its `--mcp-config` (pi: through its extension path in Phase 2),
  launched with an environment variable carrying the profile's allowlisted
  ws tool names; ws-mcp advertises (`tools/list`) and accepts
  (`tools/call`) only those. The agent profile is the single source of the
  restriction. Precedent: the env-gated surface filter `NoAgentMode` with
  `noAgentHiddenTool` in `internal/mcp/server.go` (`WS_MCP_NO_AGENT`, `server.go#L508-L516`, `server.go#L5467-L5480`). The filter is what lets
  the init-message check (Constraints) pass, since `--allowedTools` denies
  calls but leaves tools visible. Rejected: named toolsets defined in
  ws-mcp code (two places to edit per new agent); a daemon-hosted MCP
  endpoint (a second transport and tool registry).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`, `agents-plugin/`, or `agents-plugin-wsflow/`;
  `ai-docs/manuals/wsflow-mirroring.md` if a wsflow package surface is
  touched; and `ai-docs/manuals/ws-mcp.md` if `internal/mcp/` is touched.
- The runner lives in the dashboard daemon (`agents-plugin-tool/internal/wsdashboard`
  or a new package it owns); no consumer UI is in scope here.
- Prior lessons to honor (`260908-research-ws-pi-claude-code-lead-provider`):
  close the child cleanly on shutdown (a held stream kept the parent alive);
  verify the child's init message lists only the permitted tools and MCP
  servers and fail the launch otherwise (the ws-claude profile check).
- Rejected earlier, do not revive: running Claude Code with another
  harness's system prompt; `--bare`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260908-research-ws-pi-claude-code-lead-provider (2026-09-09, Resolution): "Owner approved closing the concluded provider investigation. The positive isolated spike was superseded by the recorded real-Pi subscription-path failure and implementation revert." — bearing: constrains
- 28f43259 (2026-11-08, commit): "Git-history research found the discarded Pi claude-code provider (98a5306f0, reverted 238f2ce3b) hit extra-usage billing because of the system prompt's content (Pi harness prompt), not a flag" — bearing: constrains
- 70bd5f21 (2026-11-08, commit): "Runner phase 1 puts the live billing probe first so nothing is built on the claude path if the subscription refuses the turn" — bearing: supports
- 261106-feat-ws-dashboard-claude-adapter (2026-11-06, ticket Result): "The user prompt row is `session.append` with `door: \"prompt\"`; headless it carries `origin.kind: \"unclassified\"`" — bearing: supports
- 261030-bug-claude-mod-headless-compaction-and-captions (2026-10-31, ticket Result): "Headless streaming input (`claude -p --input-format stream-json --output-format stream-json`, stdin held open, `isInteractive=false`): the step fired 1 s after the turn end" — bearing: supports
- 260908-feat-ws-pi-claude-delegate-tool (2026-09-13, 0efb4fad): "Persist the canonical design-review task frame alongside the private Claude session ID because the SDK rebuilds the system-prompt append on each resumed process." — bearing: supports
- 260505-feat-claude-named-agent-runner (2026-05-05, ticket Decisions): "Use the legacy Claude script only for verified CLI details: output JSON shape, `--session-id`, `--resume`, `--system-prompt`, and hook settings." — bearing: supports
- 260512-feat-gemini-named-agent-backend (2026-05-12, ticket Decisions): "Use Gemini CLI headless `stream-json` output, not final `json`, so the runner matches the Codex-style streaming parser direction." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp config registry (agent.profile structured family), agents-plugin-tool/internal/wsdashboard (or a new package), agents-plugin-tool/internal/mcp/server.go, agents-plugin-pi (Phase 2) |
| scope.surface | cross-module | new daemon REST routes, the agent.profile structured config family, ws-mcp env-gated tools/list and tools/call filter |
| scope.new_public_symbol | yes | agent.profile.<name> structured config family, runner REST routes, backend interface, allowlist environment variable |
| scope.new_type_contract | yes | backend interface (start, send turn, interrupt, stop, resume) and the profile schema; neither exists yet |
| scope.test_surface | new-files | no runner test exists (search for agent runner under agents-plugin-tool/internal/wsdashboard found none); existing wsdashboard and mcp test files host only adjacent coverage |
| complexity.reuse_points | confirmed | wslive.NewState/AppendRing/SnapshotPath, wsdashboard.SessionDir, NoAgentMode filter precedent in internal/mcp/server.go, Pi spawner launch shape |
| complexity.side_effect_risk | high | spawns long-lived child processes, holds stdin streams, daemon shutdown and idle timers, ws-mcp startup side effects in agent role |
| risk.correctness | high | sustained stream-json process lifecycle, lazy resume, turn-boundary restart, init-message permission verification |
| risk.fit | moderate | wslive has no snapshot writer outside internal/mcp, so snapshot writing needs a new home in the daemon |
| risk.test | high | live billing probe and live runs on the subscription cannot be unit tested; other paths depend on recorded fixtures |
| risk.security_or_contract | high | headless process with tool access, allowlist-only permissions, REST routes that start agents, ws-mcp surface restriction |

## Phases

### Phase 1: runner core and the claude backend

1. **Billing probe first.** Before building the runner, launch Claude Code
   headless with the Phase 1 flag set (stream-json in and out,
   `--append-system-prompt-file` with a short task-instruction file,
   `--disable-slash-commands`, `--strict-mcp-config --mcp-config` with an
   empty server set, `--tools` with the profile's built-in tool list
   (`--tools ""` disables all built-ins) so the visible set matches the
   allowlist, `--permission-mode dontAsk` with `--allowedTools`, an explicit
   `--setting-sources` that keeps user, project, and local settings files
   (and their `permissions.allow` rules and hooks) out, and plugins off
   through `--settings`) on the user's subscription login, run one turn, and
   resume it once with `--resume`. Read an explicit billing signal, not only
   a refusal: the stream-json result and rate-limit events' overage or
   extra-usage fields, or, when the stream carries none, confirm that extra
   usage is off on the account for the probe so a third-party
   classification surfaces as a refusal. If any turn is billed as extra
   usage or refused for usage, stop and report with the raw output; nothing
   below is built. Also record whether `--settings` turns plugins off,
   whether subscription auth survives the chosen `--setting-sources`, and
   whether an in-stream model or effort control exists ((319)).
2. **Profiles.** Add the built-in structured family `agent.profile.<name>`
   ((329)), modelled on `worktree.profile.<name>`, with the fields of the
   profiles decision (idle timeout `0` = never; the session-index listing
   knob of (316)), resolved through the conversation's working root
   ((330)). Add `agent.` to `reservedConfigPrefixes`
   (`internal/mcp/config_manifest.go`) so no adapter manifest can claim it.
   Keep the resolver free of `internal/mcp` (which imports
   `internal/wsdashboard`), like `resolveWorktreeProfiles`, which depends only
   on `wsconfig`: put it in a lower package, or make the runner a new package
   wired from `cmd/ws-mcp/dashboard.go`. No profile ships in this ticket;
   tests define their own. An
   unknown profile or backend, or a missing instruction file, fails the
   launch with a clear error.
3. **Runner core** in the dashboard daemon: a backend interface (start,
   send turn, interrupt, stop, resume by backend session id) designed for
   both claude and pi; one sustained process per conversation with the idle
   timer; a minted reply-id session directory per conversation ((315));
   lazy resume on the next input after an idle exit or a daemon restart
   ((318)); model/effort changes at the turn boundary ((319)); clean child
   shutdown with the daemon. Each conversation's record (backend, backend
   session id, profile name, current model and effort, its ws session key)
   is persisted in its session directory beside the `wslive` files, so
   resume and turn-boundary restarts survive a daemon restart.
4. **Claude backend**: the flag set from step 1; parse the stream-json
   output into `wslive` snapshot and ring entries (user, reply, turn, tools)
   so the existing conversation renderer shows them; verify the child's init
   message lists only the profile's tools and MCP servers and fail the
   launch otherwise.
5. **ws-mcp surface filter** ((326)): ws-mcp reads the allowlist
   environment variable and limits `tools/list` and `tools/call` to it; the
   claude backend adds ws-mcp to `--mcp-config` when the profile allows any
   ws tool. Check ws-mcp's startup side effects in this role (dashboard
   ensure, session registration) and suppress the ones an agent session must
   not trigger. The runner mints one ws session key per conversation, kept
   in the conversation record so it survives resume, and hands it to the
   attached ws-mcp and the agent (environment variable or the appended
   instruction file); the implementer picks the key's role (lead-only tools
   such as `config.*` stay out unless the role allows them), with the
   allowlist filter carrying the restriction. Read
   `ai-docs/manuals/ws-mcp.md` first.
6. **REST control** behind the existing access control ((314)): create a
   conversation from a profile and a working root ((330)), send a turn
   (held while a turn runs, (327); what happens to held sends on
   interrupt, `stop`, `close`, and a daemon restart is the implementer's
   choice, tested and recorded in `### Result`), interrupt, `stop` and `close` ((328)),
   change model or effort, and read state (the session directory key;
   running, idle, or closed).
   Runner conversations stay out of the session index unless the profile
   lists them. Route paths, payload fields, and the allowlist environment
   variable name are the implementer's choice and are recorded in
   `### Result`, since the consumer ticket builds on them.

Verification: unit tests for profile parsing and working-root resolution
(project layer, else global), the persisted conversation record surviving a
simulated daemon restart, the idle timer, the
lazy-resume path, held sends, `stop` versus `close`, stream-json-to-`wslive` mapping (with recorded
stream-json fixtures), and the ws-mcp allowlist filter; a fake backend
drives the REST routes end to end in tests; one live run of the claude backend through the REST routes on the
subscription login, recorded in `### Result` with the billing observation.

### Result (85b9da5fd) - 2026-11-08

Landed in fdb391e6d (config family), 85b9da5fd (runner, claude backend,
REST), and 0a3228a5f (review fixes).

- **Billing probe and live REST run, Claude Code 2.1.319, subscription
  login: passed.** The probe ran one turn with the flag set and resumed it.
  The live run used the REST routes in a throwaway cache with the model
  `claude-haiku-5-5` and a profile allowing `Read` plus two ws tools.
  - Init reported `apiKeySource: none` and the tools `Read`,
    `mcp__ws__project_tree`, `mcp__ws__runtime_read`.
  - ws-mcp connected. The only plugins left were the non-disableable
    telemetry and privacy builtins: `--settings` turned the rest off.
  - Permission mode was `dontAsk`. `--setting-sources ""` keeps
    subscription auth.
  - rate_limit_event: `status: allowed`, `rateLimitType: seven_day`,
    `overageStatus: rejected`, `overageDisabledReason: out_of_credits`,
    `isUsingOverage: false`.
  - Each turn's cost is an estimate (0.00128 and 0.00138 USD). The page
    showed context 5589/1,000,000 and the weekly limit at 2%.
  - In-stream controls: `set_model` works and keeps the session. The
    follow-up init showed `claude-sonnet-5-5` on the same session id.
    `set_effort` is unsupported, so an effort change restarts with
    `--resume` on the same session id.
  - Held merge worked. Close left no children, and a daemon SIGTERM took
    its 2 claude children down (2 before, 0 after). After a daemon
    restart, a conversation resumed lazily and recalled a word from before
    the restart.
- **Routes**, mounted by the dashboard daemon behind its existing access
  guard:
  - `GET /agents` returns `{conversations: [State]}`.
  - `POST /agents {profile, root}` returns 201 and the `State`.
  - `GET /agents/<id>`.
  - `POST /agents/<id>/send {text}`.
  - `POST /agents/<id>/interrupt` returns the `State` plus
    `returned: [text]`.
  - `POST /agents/<id>/stop`.
  - `POST /agents/<id>/close`.
  - `POST /agents/<id>/settings {model?, effort?}`.
  - Errors are plain text: 400 for a bad launch (unknown profile or
    backend, missing instruction file, relative root), 404 for an unknown
    id, 409 for a closed conversation.
  - Origin rule: every POST needs a JSON body
    (`Content-Type: application/json`) and either no `Origin` or the
    dashboard's own origin, so no other web page can drive an agent.
- **`State` fields:** `id` (the reply-id: session directory name and page
  `/s/<id>`), `profile`, `backend`, `root`, `status`
  (`idle`|`running`|`closed`), `process`, `model`, `effort`, `held`,
  `page`, `error` (last launch or turn failure, cleared when a turn
  starts), `created_at`, `closed_at`.
- **Session directory** `<daemon dir>/sessions/<reply-id>/`:
  - `agent.json` (mode 0600): backend, backend session id, profile, root,
    model, effort, ws session key, created/closed.
  - `instructions.md`: the composed append file.
  - `inbox/` for held sends.
  - The wslive `ring/` and `snapshot.json`.
  - `registration.json`: `agent: true`, and `unlisted` unless the profile
    sets `list_in_index`.
- **Held sends:**
  - A send while a turn runs is held, and every held message is merged
    with blank lines into the next turn.
  - `interrupt` stops the turn and takes the held messages back: they are
    returned, not submitted.
  - `stop` ends the process and keeps them held.
  - `close` cancels them.
  - A daemon restart keeps them on disk, and they merge with the next send.
- **Environment:**
  - `WS_MCP_ALLOWED_TOOLS` (comma-separated ws tool names) limits the
    attached ws-mcp's `tools/list` and `tools/call`.
  - `WS_MCP_NO_DASHBOARD=1` stops it ensuring a dashboard and registering
    a session.
  - `WS_MAILBOX`/`WS_MAILBOX_AUTO` are set blank for it.
  - The harness child drops parent-session variables (`CLAUDE*` except
    auth/provider selectors, `WS_MAILBOX*`).
- **Decisions:**
  - The agent's ws key is a parentless delegate key, minted per
    conversation and kept in `agent.json`. It is re-ensured at every start
    and reaches the agent only through the composed instruction file, never
    REST or the registration. Lead-only tools stay refused even if a profile
    lists them.
  - Claude emits `system/init` per user turn, so the init check runs at the
    first turn's init and kills the child on any unlisted tool, MCP server,
    or plugin. `dontAsk` with `--allowedTools` denies unlisted calls in that
    window.
  - Model changes go in-stream; an effort change restarts with `--resume`.
  - A profile resolves through the working root's layers: project, then
    repo, then global. A repo-scope (committed) profile may not set
    `mcp_servers` or `plugins`, because a committed profile may not start
    programs. It neither launches nor resolves, and config.tune refuses that
    write.
- **Lite review**, 1 Important and 5 Minor:
  - The Important (committed profiles could start programs) is fixed in
    0a3228a5f.
  - Fixed Minors: the runner now starts before the daemon serves (a race on
    its notify hook), and `WaitDelay` bounds `Wait` when a grandchild holds
    stderr.
  - Open Minors:
    - a send that fails after the inbox drain loses the text from the inbox
      (it stays in the ring and `error`);
    - a control request can time out if 256 events queue while the loop
      waits on it;
    - after a daemon restart the snapshot's usage and lead state reset until
      the next turn.
- **Verification:**
  - `go test -count=1 ./...` in `agents-plugin-tool`: all ok.
  - `go test -race -count=2 ./internal/wsagent/`: ok.
  - gofmt and vet clean.

### Phase 2: pi backend

Implement the backend interface for Pi following the Pi spawner's launch
shape (`agents-plugin-pi/src/spawner.ts`: RPC mode, `--session`,
`--session-dir`, `--append-system-prompt`, `--no-extensions`, an explicit
`--extension` only when the profile names one, `--tools <allowlist>`), map
its RPC events to `wslive` entries, and resume by Pi session. A profile
that allows ws tools attaches ws-mcp with the same allowlist filter and
session key as Phase 1 (through the extension path), and the launch fails
unless the child's reported tool set holds only the profile's tools.
Verification:
the Phase 1 test suite runs against the pi backend through the same fake and
recorded-event fixtures, plus one live Pi run through the REST routes,
recorded in `### Result`.

### Result (dab9f6c35) - 2026-11-08

- **Launch shape** (`PiBackend`, pi 1.0.4):
  - Session and extensions: `--mode rpc --session <session
    dir>/pi/session.jsonl --session-dir <session dir>/pi --no-extensions`.
    When the profile allows ws tools or any MCP server, this adds `--extension
    builtin:mcp --extension <session dir>/pi/ws-agent-mcp.ts`, plus
    `--extension <path>` for each profile plugin.
  - Off unless the profile turns skills on: `--no-skills
    --no-prompt-templates`.
  - Always: `--no-context-files --no-approve --append-system-prompt
    <instructions file or "">`. The explicit append also keeps the user's
    `APPEND_SYSTEM.md` out.
  - Tools and model: `--tools <entries>` (or `--no-tools`), `--model`,
    `--thinking <effort>`.
  - Environment: drops `WS_PI_*` and `WS_MAILBOX*`, sets
    `PI_SKIP_VERSION_CHECK=1`.
  - Resume is by the session file, so a restart reopens the same pi session.
    `backend_session_id` records pi's session id from `get_session_stats`.
- **ws-mcp and MCP servers:**
  - A generated private extension (0600, rewritten at every start) calls
    `registerMcpServer` for exactly ws-mcp (same command,
    `WS_MCP_ALLOWED_TOOLS`, `WS_MCP_NO_DASHBOARD`, blank mailbox variables,
    key through the instruction file) and the profile's servers.
  - Exposure is `direct` unless the server object sets one.
  - The `--tools` entries are each built-in tool, `mcp__ws__<tool>` per ws
    tool (pi's sanitised names), and `mcp__<server>__*` per profile server.
- **Tool check:**
  - The system message of each run carries pi's own report of the declared
    tools: the full set on a session's first request, deltas after that.
  - Any added name outside the entries fails the launch (`launch refused: pi
    reports tools outside the profile: ...`) and kills the child.
  - A claude-style permission rule in `tools` is refused at start, because
    pi's `--tools` allows whole tools only.
- **Event mapping:**
  - assistant text → final reply;
  - each assistant message's tool calls → one tool batch, ended by
    `tool_execution_end` (`isError` → failed);
  - `compaction_start`/`_end` → compact events (threshold/overflow → auto,
    manual → manual);
  - `agent_settled` → turn end, then a `get_session_stats` request whose
    response feeds usage (context tokens, window, percent, and cost marked
    estimate) and the session id;
  - a refused or `handled` prompt response → turn end;
  - blocking extension dialogs (select/confirm/input/editor) → cancelled.
- **Controls:**
  - Interrupt sends `abort` without waiting, and the turn ends at
    `agent_settled` with no error. pi reports the abort as stopReason
    `aborted` mid-stream or `error` during a tool. An abort nobody asked for
    stays an error.
  - Model: an exact `provider/id` uses in-stream `set_model`, then re-applies
    the set thinking level, since `set_model` resets it. A fuzzy id, a
    `:thinking` suffix, or a return to the default restarts the child with
    `--model` on the same session.
  - Effort: in-stream `set_thinking_level`.
- **Live REST run** (throwaway cache, the user's pi login on
  `openai-codex/gpt-6.1-sol`, profile `tools: [read]`, `ws_tools:
  [runtime.read]`, effort low): passed.
  - Create returned 201. The command line matched the shape above, with
    `--tools read,mcp__ws__runtime_read`.
  - pi declared `[read, mcp__ws__runtime_read]`.
  - First turn: three calls, one failing (missing file), reply `alpha`. Usage
    showed context 876/500000 and an estimated cost of 0.005578 USD.
  - Interrupt during a long reply returned `["second held message"]`. The turn
    ended with stopReason `aborted` and no error.
  - Effort `medium` went in-stream (`thinking_level_changed`, same pid). The
    fuzzy model `gpt-6.1-sol` restarted on the same session (new pid) and
    recalled `beta` from memory.
  - `stop` left 0 pi and 0 ws-mcp children. A daemon SIGTERM with a live child
    left 0 and 0. After the restart, the next send resumed lazily and answered
    `alpha beta` from memory.
  - `close` left 0 children, and a later send got 409.
  - A global profile server with `exposure: codemode` was declared as `[read]`
    only: `--tools` filtered the codemode tool, and the launch was accepted.
- **Known limit:** pi has no flag to skip the user-level
  `~/.pi/agent/mcp.json` while builtin MCP is on. Servers configured there
  start, but their tools stay undeclared, because `--tools` filters by
  `mcp__` entry. A server there with the name of a registered one (`ws`
  included) replaces the registration by pi's own rule, and its tools then
  carry the same names. The model can still call only the `--tools`
  names, and the tool check still runs.
- **Lite review** (85b9da5fd..dab9f6c35): no Critical or Important, 4
  Minor.
  - Fixed: `PiBackend.Start` had no test of its own; 7e6ea7f95 drives it
    against a stand-in pi.
  - Recorded: the `mcp.json` limit above.
  - Open:
    - an interrupt that lands before pi starts the run is a no-op, the run
      completes, and its error would be read as the interrupt;
    - the post-turn statistics request is dropped when the process stops
      right after the turn, so the usage figures wait for the next turn.
- **Verification:**
  - `TestRunnerSuiteOnPi` reruns the 13 runner and REST suite tests with every
    profile on the pi backend through the fake.
  - Recorded-RPC fixtures `pi-tool-turn.jsonl` and `pi-abort-resume.jsonl`
    drive the mapper tests: tool turn, unlisted-tool refusal, abort with and
    without an interrupt, session statistics.
  - Further tests cover args, the tool surface, the bridge file, the
    environment, prompt answers, dialogs, compaction, and process commands
    against a stand-in pi.
  - `go test -count=1 ./...` in `agents-plugin-tool`: all ok. `go test -race
    -count=2 ./internal/wsagent/`: ok. gofmt and vet clean.
  - After 7e6ea7f95: `go test -race -count=1 ./internal/wsagent/` and
    `go test -count=1 ./internal/wsagent/ ./internal/wsdashboard/ ./cmd/...`:
    ok.

## Sage Review Round 1 (2026-11-08)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | agent.profile.<name>.* cannot be declared in the config manifests | important | missing |
| 2 | No project binding for a conversation in a machine-wide, sessionless daemon | important | missing |
| 3 | The init-message tool check cannot pass with the stated claude flag set | important | autonomous |
| 4 | The attached ws-mcp has no session key for the agent | important | autonomous |
| 5 | The billing probe only detects a refusal, not a turn silently billed as extra usage | minor | autonomous |
| 6 | Lazy-resume state is never said to be persisted | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | REST control semantics for send-during-turn and stop are undefined | important |
| 2 | Phase 2 omits the ws-mcp attachment and init-message verification for pi | minor |
| 3 | REST route paths, payload shapes, and the allowlist env var name are unspecified | minor |
| 4 | Board-number references are opaque to a fresh reader | minor |

---
title: "ws MCP pi.* recursion: Pi workers spawn Pi children through the daemon as router"
parent: 261112-epic-ws-mcp-pi-subagents
related:
  261112-feat-ws-mcp-pi-spawn-claude-lead-path: prerequisite
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6c103b7a493e1790
sage-review-completeness-reviewed: 6c103b7a493e1790
completed: 2026-11-13
---

# ws MCP pi.* recursion (child 1b)

## Background

Child 1 (`261112-feat-ws-mcp-pi-spawn-claude-lead-path`, done, landed on
`develop` at 3041e728e) gives a Claude lead the `pi.*` tools on the daemon's
`wsagent.Runner`, with a parent record on the spawn spec and a mailbox
completion report. Its `pi-worker` profile has no `pi.*` (`PiWorkerWSTools`,
`wsagent/builtin.go#L70-L73`), so a Pi worker can run only leaf playbooks.
The user requires Pi workers to run spawning playbooks too (the elevated
ticket worker spawns reviewers and implementers), so a Pi worker needs
`pi.*`.

Facts gathered 2026-11-12 (paths under `agents-plugin-tool/internal/`):

- A Pi child's ws tools come from an attached stdio ws-mcp the runner
  registers through the Pi MCP bridge (`wsagent/pi.go` `piMCPServers`,
  `writePiBridge`), started with `WS_MCP_ALLOWED_TOOLS` and
  `WS_MCP_NO_DASHBOARD` (`wsagent/runner.go#L1606-L1623`, constants at
  `wsagent/runner.go#L96-L102`). That server has no daemon handle
  (`ServerOptions.Agents` is set only by the hosted proxy,
  `cmd/ws-mcp/proxy.go#L100-L102`), so child 1's `pi.*` refuses there
  (child 1 Decision 3).
- Every Pi child is already the daemon's own process; the daemon persists
  its record and resumes it.
- Child 1's shipped shape this builds on: `SpawnSpec.Parent *ParentRef` and
  `Record.Parent` (`wsagent/spawn.go#L58-L78`, `wsagent/record.go#L93-L115`);
  `ParentRef{Kind, ReplyID}` has one kind, `ParentLead`, and `Runner.Spawn`
  accepts any non-blank kind with a valid reply-id; `Options.Report`
  (`wsagent/runner.go#L145-L150`) is wired to `mcp.PiTurnReporter`
  (`cmd/ws-mcp/dashboard.go#L279`), which returns early for any kind but
  `ParentLead` and otherwise posts mailbox mail
  (`mcp/pi_tools.go#L478-L502`). `Runner.Children(parent)`, `Wait`, `Watch`
  are keyed by that `ParentRef` (`wsagent/spawn.go#L260-L270`).
- A runner conversation's id is the reply-id of its own ws key
  (`wsagent/runner.go#L498`, `wsmailbox.ReplyID(key)`), and `pi.*`
  derives a caller's parent as `wsmailbox.ReplyID(session_key)`
  (`mcp/pi_tools.go#L151-L153`), so a Pi worker's own `piParent` would equal
  its conversation id; the parent chain can be walked with `Runner.Get`
  for any parent that is itself a runner conversation (a Pi worker, or a
  daemon-hosted lead), and ends at a terminal lead the runner does not hold.
- Gates a Pi worker's attached ws-mcp meets today: `filteredTools` skips
  `pi.*` unless `piListed()` (daemon handle, knob, Pi version) before it
  asks `toolAllowed`, so a `WS_MCP_ALLOWED_TOOLS` entry alone does not list
  it (`mcp/server.go#L5578-L5586`); `handlePiTool` demands a lead-scoped
  session key (`requireLeadSessionKey`, `mcp/server.go#L5722-L5736`) and
  the keyed `tools/call` gate refuses `pi.*` (`isLeadOnlyTool`) for any
  non-lead key (`mcp/server.go#L140-L142`, `#L725-L727`); the runner mints a
  Pi child's key as a parentless delegate key (`Keys.Mint`,
  `mcp/agent_keys.go#L28-L30`, `wsagent/runner.go#L493`), by design so
  lead-only tools stay refused even if a profile lists them (85b9da5f).
- The runner's HTTP routes have no spawn-spec route: `RoutePrefix` serves
  `POST /agents {profile, root}`, `/send`, `/stop`, `/close`, `/settings`
  and the lead routes (`wsagent/http.go#L10-L55`); `Runner.Spawn` is reached
  only in process, through `mcp.PiAgents`.
- Steering exists only for leads: `conv.steer` returns unless `c.rec.Lead`
  (`wsagent/runner.go#L1113-L1116`), and a plain `Runner.Send` to a running
  conversation waits for its next turn (`mcp/pi_tools.go#L201-L204`,
  `wsagent/runner.go#L621-L647`). `piProcess.Steer` writes pi's `steer`
  command (`wsagent/pi.go#L462-L466`).
- The attached ws-mcp's env is the runner's override map only
  (`wsagent/runner.go#L1606-L1623`, written to the bridge file by
  `writePiBridge`, mode 0600, at every start); `WS_MCP_CONFIG_MANIFESTS` is
  set by a plugin launcher's `.mcp.json` env, not by the runner
  (`agents-plugin/.mcp.json#L8`, `agents-plugin-wsflow/.mcp.json#L11`), so a
  key declared only in a package's manifest is undeclared to a server that
  inherits no such variable (`mcp/process_env.go#L139`, `piKnobOn` reads
  undeclared as off, `mcp/pi_tools.go#L131-L143`).

## Decisions

1. **Recursion is in scope.** The `pi-worker` profile gains `pi.*`, subject
   to the depth cap (Decision 4). Rejected: leaf-only Pi workers (the user
   requires spawning playbooks on Pi).
2. **Daemon as router.** A Pi child's attached ws-mcp stays direct, but its
   `pi.*` calls forward to the daemon's runner over the daemon's local HTTP
   API, authenticated by a per-child capability token the runner puts in
   that child's env. The daemon records the calling child as the new child's
   parent from the token, so the daemon owns the whole tree. Rejected: the
   Pi extension path (children run with extensions off, and an extension
   subagent is spawned outside the daemon's tree); moving the child's whole
   ws-mcp onto the daemon `/mcp` proxy (it changes the attached server's
   mailbox identity and dashboard registration for one tool family).
3. **Completion to a Pi parent is steered.** When a child's parent is a Pi
   conversation, the runner delivers the child's report (child id, status,
   final assistant message) into the parent conversation as a steer while
   the parent's turn runs, and as a new prompt when the parent is idle. A
   Claude lead parent still gets child 1's mailbox mail. Rejected: queueing
   the report until the parent's turn ends (the user prefers steer, as a
   native subagent completion arrives mid-turn); a mailbox for Pi children
   (a Pi child has no mailbox push, and the daemon already holds the parent
   process).
4. **Depth cap 2.** Lead -> Pi worker -> Pi grandchild; a `pi.spawn` from a
   depth-2 child refuses with a message naming the cap. Rejected: 3 levels;
   a knob; no cap (cost runaway). Depth counts from the lead: the lead
   (Claude or daemon-hosted) is depth 0, a Pi worker it spawns is depth 1,
   and that worker's child is depth 2. A depth-2 child keeps its token and
   listed `pi.*`, so its `pi.spawn` reaches the daemon and gets the cap
   message, not a token-absent refusal.
5. **Epic Decision 8 is amended.** Pi-harness lead sessions (lead-pi, with
   their own extension) still do not see `pi.*`; runner-spawned Pi workers
   do. This ticket edits the epic's Decision 8 text to say so.
6. **The token, not the key, authorizes `pi.*` on a Pi worker.** The Pi
   child's ws key stays a parentless delegate key, so every other lead-only
   tool stays refused (85b9da5f). `pi.*` is the one exception: the attached
   ws-mcp lists and accepts it when the runner put the child's capability
   token and the daemon endpoint in its env, and forwards each call to the
   daemon. The runner issues a token only to a conversation started through
   `Runner.Spawn` with a recorded parent and tier, never to one opened
   through `POST /agents` (which has neither). Child 1's knob
   (`dashboard.pi_spawn_enabled`) is checked once, at the root: a lead's
   `pi.spawn` already passed it in the lead's own hosted server, and every
   descendant inherits that; the daemon re-checks the Pi version and the
   runner on every forwarded call. The attached server's own manifest set
   therefore never matters. A token is drawn fresh at every process start
   (never persisted, like the per-start key in
   `wsdashboard/fs_capability.go`) and stops being valid when its
   conversation closes. A token scopes `pi.list`, `pi.send`, `pi.wait`,
   `pi.transcript`, and `pi.stop` to the token holder's own children.
   (626, technical, assumed per (625); refined after design review.)
   Rejected: minting the Pi child a lead-scoped key (opens every lead-only
   tool); listing `pi.*` through `WS_MCP_ALLOWED_TOOLS` alone (the gates
   refuse it, see Background); re-reading the knob per forwarded call (the
   daemon has no session-independent declared-config reader, and a ws and
   a wsflow lead may share one daemon with different defaults).
7. **A Pi-conversation parent kind.** A child spawned by a Pi worker records
   a new `ParentRef` kind for a runner conversation, whose reply-id is the
   parent conversation's id. The runner delivers that child's report itself:
   through the parent's process steer while the parent's turn runs (the
   lead-only `conv.steer` guard is widened for this delivery), and as a new
   prompt through the normal send path when the parent is idle. When the
   parent's turn ends between the check and the steer, delivery falls back
   to the prompt path. When the parent is blocked in `pi.wait` on that same
   child, the wait's return is the delivery and no steer is sent.
   `PiTurnReporter` keeps handling `ParentLead` only. (627, technical,
   assumed per (625).) Rejected: routing the report through the mailbox
   (Decision 3); delivering twice to a waiting parent.
8. **Grandchild tier is non-escalating.** A `pi.spawn` from a Pi worker may
   not request a tier above the worker's own; it refuses naming the
   worker's tier as the cap. The spawn spec and record gain a persisted
   tier (`SpawnSpec.Tier`, `Record.Tier`); a caller with no recorded tier is
   refused, never left uncapped. (629 a, assumed per (625); matches
   260912-feat-ws-pi-recursive-worker-subtree-lifecycle.) Rejected: any
   tier (cost runaway, as for Decision 4).
9. **Compatibility tier T0.** Additive only: a new `ParentRef` kind value,
   an optional persisted tier on the spawn spec and record, a new
   daemon-local HTTP route, new env variables, and `pi.*` added to the
   `pi-worker` profile; the attached ws-mcp is the daemon's own binary.
   (628, technical, assumed per (625); `ai-docs/manuals/release-compatibility.md`.)
10. **A worker's report waits for its children.** While a Pi worker has a
    direct child whose report it has not yet received, the worker's own
    settled turns are not reported upstream (neither the Claude lead's mail
    nor a Pi parent's delivery). Once every open child has reported and the
    worker's next turn settles, that turn is reported; a parent's `pi.wait`
    on that worker likewise returns only a reported turn. A child counts as
    open from an accepted `pi.spawn` or `pi.send` until its report reaches
    the worker or it is closed; a child turn that fails or is interrupted
    is a report too. The "report delivered" mark is persisted on the
    child's record, so a restarted daemon neither releases a held report
    early nor holds it forever. A worker turn that itself fails or is
    interrupted is not held: it goes upstream at once, since the lead must
    act on it. (630 a, assumed per (625); matches
    260912-feat-ws-pi-recursive-worker-subtree-lifecycle, "local settle is
    not semantic completion".) Rejected: a report per settled turn with the
    worker told to `pi.wait` (a lead reads an interim message as
    completion).
11. **Closing a worker closes its subtree.** Closing a Pi conversation
    (`Runner.CloseConversation`, which `pi.stop` and the dashboard close
    call) first closes its live descendants, deepest first. A resumable
    process stop (the dashboard `/stop`, `Runner.Stop`) and the idle-timer
    stop do not cascade: the subtree keeps running, and the next report
    resumes the worker through the prompt path (Decision 7), so an idle
    worker waiting on children under Decision 10 is never cut off. A report whose Pi parent is already closed
    is not delivered; it stays in the child's record and transcript. A
    daemon restart that interrupts a subtree reports each interrupted child
    to its parent by the same paths once that parent is resumed. (631 a,
    assumed per (625).) Rejected: reparenting orphans to the lead (the lead
    inherits work it never asked for); leaving them running (cost runaway,
    as for Decisions 4 and 8).

## Constraints

- Lands after child 1 (done, 3041e728e); builds on its spawn spec parent
  field and report path (`SpawnSpec.Parent`, `Record.Parent`,
  `Options.Report` -> `mcp.PiTurnReporter`, which posts only for
  `ParentLead`; see Background).
- How `pi-worker`'s `pi.*` meets child 1's exposure gates
  (`dashboard.pi_spawn_enabled`, Pi version, daemon handle) is settled by
  Decision 6. The gap it closes (design review of child 1, 2026-11-12),
  verified against the tree: the allowlist alone does not list
  `pi.*` (`piListed()` runs first, `mcp/server.go#L5578-L5586`); the
  attached server inherits `WS_MCP_CONFIG_MANIFESTS` only from the daemon's
  or Pi's environment, not from the runner (Background); and `pi.*` is also
  lead-only for a delegate-scoped Pi child key (`isLeadOnlyTool`,
  `requireLeadSessionKey`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/release-compatibility.md`,
  and `ai-docs/manuals/wsflow-mirroring.md` before editing.

## Prior Decisions

- 85b9da5f (2026-11-08, commit): "The agent's ws key is a parentless delegate key so lead-only tools stay refused even if a profile lists them; the key reaches the agent only through the composed instruction file" — bearing: constrains (kept by Decision 6: the key stays delegate-scoped; `pi.*` is authorized by the token)
- 7f40e811 (2026-11-13, commit): "The parent reference is a typed {kind, reply_id} so a later Pi-parent path can reuse the record and report hook (the recursion ticket)." — bearing: supports
- ea73daf5 (2026-11-12, commit): "(608) daemon-as-router, (609) user prefers steer for completion to a Pi parent, (610) a depth 2, (612) epic Decision 8 amendment recorded in child 1b." — bearing: supports
- 543481e8 (2026-11-13, commit): "The daemon handle is ServerOptions.Agents (a PiAgents interface over wsagent.Runner), so a direct-path ws-mcp has none and both hides and refuses the tools naming the dashboard daemon" — bearing: constrains
- 543481e8 (2026-11-13, commit): "The report runs in the daemon (Runner Options.Report), writing the reply registry under the daemon's cache home, which is the lead's" — bearing: constrains
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-12, Decisions): "Every spawn producing depth 2 or greater must be monotonically non-escalating relative to its direct parent." — bearing: constrains
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-12, commit f8a1a640): "Model recursion as a role-independent depth budget with a default maximum depth of two and room for terminal full workers." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsagent/runner.go, wsagent/http.go, wsagent/builtin.go, wsagent/spawn.go, wsagent/record.go, mcp/pi_tools.go, mcp/server.go, cmd/ws-mcp/dashboard.go, epic 261112-epic-ws-mcp-pi-subagents |
| scope.surface | cross-module | spawn route on the runner HTTP API, attached ws-mcp client, ParentRef kind, pi-worker profile, token env across wsagent, mcp, cmd/ws-mcp |
| scope.new_public_symbol | yes | a capability token env name, a spawn HTTP route, a new ParentRef kind constant; none named in the ticket yet |
| scope.new_type_contract | yes | ParentRef kind for a Pi conversation parent, spawn request body on the runner HTTP route |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/spawn_test.go, runner_test.go, pi_test.go, mcp/pi_tools_test.go; new files not confirmed |
| complexity.reuse_points | confirmed | Runner.Spawn, Children, Wait, Options.Report, piParent reply-id, piProcess.Steer and steerer, requireLeadSessionKey all read |
| complexity.side_effect_risk | high | a token-authenticated route that spawns daemon-owned processes, steering a parent's running turn, a per-child secret in the bridge env |
| risk.correctness | high | depth chain walk, steer versus prompt on a parent turn, conv.steer is lead-only today, forged token handling |
| risk.fit | high | allowlist does not bypass piListed, pi.* is lead-only against a delegate Pi key, attached server may not declare the knob, no spawn HTTP route exists |
| risk.test | high | acceptance needs a real Claude lead, Pi worker and Pi grandchild chain; steer and idle paths need fake backends |
| risk.security_or_contract | high | new capability token and authenticated spawn route give a Pi child bash-capable grandchildren; a narrow `pi.*` exception to the delegate-key lead-only rule in 85b9da5f (the key itself stays delegate-scoped, Decision 6) |

## Phases

### Phase 1: Pi workers spawn through the daemon

- Runner: per-child capability token in the child's env; HTTP endpoint the
  attached ws-mcp's `pi.*` forwards to; parent recorded from the token as
  the Pi-conversation kind (Decision 7); depth computed from the parent
  chain and capped (Decision 4); tier persisted and capped at the
  caller's (Decision 8); token issued only to spawned children, drawn per
  start, void on close (Decision 6).
- Attached ws-mcp: `pi.*` listed for `pi-worker` and forwarded to the
  daemon when the token and endpoint are present, despite the delegate key
  (Decision 6); refused with a clear message when the token or daemon is
  absent.
- Report delivery to a Pi parent: steer while its turn runs, prompt when
  idle, no duplicate to a parent in `pi.wait` (Decision 7); upstream report
  held while children are open (Decision 10); subtree close (Decision 11).
- Epic Decision 8 text amended; the epic's `## Child Tickets` lists child 1
  as done and this child.

Verification:
- Go tests: token-authenticated spawn records the child as parent; a
  forged, missing, or stale (closed conversation) token refuses; a
  conversation opened through `POST /agents` gets no token; a token holder
  sees and controls only its own children; spawn from the chain lead ->
  worker (depth 1) -> grandchild (depth 2) refuses with the cap message; a
  tier above the caller's refuses, and a caller with no recorded tier
  refuses; a Pi child's key still gets other lead-only tools refused; report
  to a Pi parent is steered mid-turn, prompted when idle, falls back to a
  prompt when the turn ends at the steer, and is not duplicated to a parent
  in `pi.wait`; a worker with an open child sends no upstream report until
  the child reports and its next turn settles, while a failed worker turn
  goes upstream at once; a held report survives a daemon restart (released
  neither early nor never); closing a worker closes its live grandchildren
  deepest first; a report to an already-closed Pi parent is not delivered
  and stays in the child's record; an idle-stopped worker's open grandchild
  keeps running and its report resumes the worker; after a restart an
  interrupted child reports to its parent once the parent resumes; a
  Claude parent still gets mail.
- The builtin profile test shows `pi-worker` listing `pi.*`; the epic's
  Decision 8 text and `## Child Tickets` are updated in the same branch.
- A real run: a Claude lead spawns a Pi worker that spawns a Pi grandchild;
  the grandchild's report reaches the worker, the worker's reaches the lead.

### Result (7b1e9f7ef) - 2026-11-13

Landed (e09e620a6, 1d3fe308a, 39379cfe4, 7b1e9f7ef):
- `internal/wsagent/children.go`: capability token (`WS_MCP_PI_TOKEN`,
  `WS_MCP_PI_ENDPOINT`) for a `Runner.Spawn` conversation with a parent, a
  recorded tier, and a `pi.*` profile tool, drawn per process start, never
  persisted, void on process end or close; loopback-only
  `POST /agents/by-token/<op>` (spawn, list, send, stop, transcript, wait)
  scoped to the holder's children; `ParentConversation` parent kind; depth
  cap 2; tier capped at the holder's (tierless holder refused); steer or
  prompt delivery of `pending_reports`; held upstream report while a child
  is open; deepest-first close cascade.
- `spawn.go`/`runner.go`/`pi.go`/`record.go`: `SpawnSpec.Tier`,
  `Record.tier`/`pending_reports`/`reports`, `State.Tier`; settle split into
  hold and `publish`; steer answers counted per process so `clear_queue`
  runs only after every steer is answered; reports joined into the next
  turn like held mail; `pi-worker` lists the six `pi.*` tools.
- `internal/mcp/pi_forward.go` + keyed gate: a runner-less ws-mcp with the
  token and endpoint lists `pi.*` with worker descriptions and forwards to
  the token route; the delegate key passes the gate for `pi.*` only. The
  daemon's `ChildCheck` is `mcp.PiReady`; the tier refusal and report text
  moved to `wsagent` (`PiTierRefusal`, `ReportText`).
- `ws-mcp.md` spawn-tree block and env rows; epic Decision 8 amended and
  `## Child Tickets` updated.

Verification: `go test -count=1 ./...` in `agents-plugin-tool` passes every
package except `cmd/ws-mcp` `TestHostedConnectionSeeingALANChangeReplacesTheDaemon`,
which fails identically on base e2ac4f603 (the test expects git's `master`;
this machine's `init.defaultBranch` is `main`). After the review fixes:
`go test -race ./internal/wsagent/` and `go test -run 'TestPi|Forward'
./internal/mcp/` pass. New tests: `children_test.go` (token issue and
voiding, parent and tier recording, forged/missing/stale/remote refusals,
own-children scope, depth and tier caps, steer mid-turn, prompt when idle,
steer fallback, parent wait as delivery, failed turn not held, restart
survival, cascade order, stop not cascading, closed parent keeps the
report, release on child close, a settle right after a wait's check),
`pi_forward_test.go`, and Pi mapper/process steer tests. The real
Claude lead -> Pi worker -> Pi grandchild run was not performed.

Decisions:
- Kept branch `impl/develop/pi-spawn-recursion` (lead-named) over the
  route's `impl/develop/musky-grunt-pep`.
- Compatibility T0: new optional record fields, a new parent kind, a new
  route, and new env names; nothing renamed or removed.
- Lite review: two Important findings fixed in 7b1e9f7ef (a wait's
  lost wakeup between its check and its channel read; untested Pi-side
  steer mapping). Minor, not fixed: a report steered into a running Pi
  turn lives only in memory and Pi's queue until taken, so a daemon exit in
  that window loses it.
- Known limits: after a restart a child's in-memory send count resets, so
  a held-but-undrained inbox message does not keep the parent's report
  held; a parent wait that starts after a report was delivered returns
  that turn again.

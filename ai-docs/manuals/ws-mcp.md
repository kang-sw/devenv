---
summary: MCP operational runbook, launcher environment, release and verification steps
---

# ws MCP Operations Runbook

Operational runbook for the host-neutral `ws-mcp` runtime used by the `ws`
plugin package.

This file is not the MCP tool contract or current tool inventory. That contract
is the test suite under `agents-plugin-tool/internal/`, and the implementation
coupling is the code itself:

- MCP dispatch, schemas, and tool allowlist: `agents-plugin-tool/internal/mcp/`
- plugin launcher and release packaging: `agents-plugin-tool/internal/wsrsrc/`
  and each package's `runtime.json`

For live tool schemas and current inventory, ask the runtime: use MCP
`tools/list`, `ws/runtime.read`, `ws-mcp runtime capabilities`, or the source
registry in `agents-plugin-tool/internal/mcp/server_schemas.go:11`.

## Tool Output Convention

An MCP tool's output is read by an LLM before it is read by anything else, so a
new tool's default response is compact, readable text. Return JSON only when a
caller needs stable machine parsing, protocol metadata, or a
compatibility-preserving shape, and put it behind an explicit opt-in (a
`format: "json"` argument) rather than making it the default. Nothing enforces
this - `toolTextResponse` and `toolJSONResponse` are equally available at every
dispatch site in `agents-plugin-tool/internal/mcp/server_dispatch.go:21` - so it has to be
applied when the tool is written.

## Hidden Call Summaries

Every tool accepts optional top-level `summary`, `toolSummary`, or
`tool_summary` metadata without advertising those fields in its schema. The
precheck harvests the first nonblank string in that order and strips all
undeclared aliases before argument validation, including inner calls through
`call`. A tool-declared property with one of those names remains its own input.
Descriptions reach the rows through harness-stream previews, not per-call
debug logging. Previews use the same key list and prefer the description in
running and completed tool rows; the Claude hook places these keys before
ordinary flattened arguments so truncation does not hide them. Existing
masking and display limits still apply.

This is T0 for existing calls: no summary is required and no store shape
changes. It does not make old binaries accept metadata: their precheck rejects
undeclared arguments, so callers targeting an older server must omit the
optional metadata or retry without it.

## Process Model

One dashboard daemon per cache root hosts the MCP servers and one-shot CLI
handlers. Harnesses run Python adapters, not ws executables:

```bash
python3 <plugin>/bin/ws-mcp-launcher.py serve --stdio --root <repo-root>
```

Stdin/stdout remains newline-delimited JSON-RPC (MCP protocol `2025-03-26`).
The same launcher accepts `version`, `doctor`, `runtime`, `tools`, `call`,
`config`, `path`, `git`, `tickets`, and `mailbox` commands. Only a necessary
`daemon-start`/replacement executes the staged ws binary; there are no
executable version/capability probes or direct server/CLI fallbacks.

`doctor` is a host-independent smoke check. It verifies the repository root,
`ai-docs/`, and `AGENTS.md`; downstream projects should rely on MCP tools for
bundled conventions rather than repository-local source paths.

### Mailbox store

Mailbox state lives in memory, one store per cache root
(`internal/wsmailbox/memstore.go`): every scope's presence and queues, the
reply-id registry, each session's ten-entry history window, and the
armed-wait listening markers. The daemon holds it for its life. A committed
change (a send, a recv, a heartbeat) is one JSONL record appended to the
restore ring `<cache root>/mailbox-ring/` before the call returns, so mail
survives a killed daemon; there is no fsync, so an OS crash may lose the
tail. The ring keeps ten segments of about 1 MiB, each opening with a
snapshot of the whole live state; a rotation reaps idle history windows and
expired reply entries, never live mail. The ring is read only at open; a
torn last record is cut.

The per-scope `mailbox.json` files, `mailbox-replyids.json`, and the
`mailbox-history/` entry files are the layout a previous ws-mcp version
uses. The store imports them at open (then clears their queues, so nothing
is delivered twice) and writes them back at a graceful daemon shutdown, so a
downgrade keeps its mail; the old version's changes are imported at the next
open. `mailbox-listening/` is no longer read or written.

The daemon holds the ring's lock. A directly run ws-mcp binary may still
read the ring (lock-free: every committed call is already in it), but its
mailbox writes fail while the daemon runs: direct mailbox calls are
unsupported, like direct `serve --stdio`.

### Session key store

Each session key is a directory, `<cache root>/sessions/<key>/`, holding
`key.json` (the identity - root, scope, parent - plus the session-scope
config overrides, the child note, and the once-only flags), `agenda.json`,
and `todos.json`. A pre-0.50 `whiteboard.json` remains an immutable rollback
snapshot. Whiteboards now live under the project's
`ai-docs/whiteboards/<reply-id>/`: versioned session `meta.json`, opaque topic
folders holding `meta.json`, immutable panel versions, and append-only
`log.jsonl`. The daemon renders pages from this store without writing page
artifacts. Its first board open schedules one background cleanup per project
and cache root: retired index/stamp and hashed Mermaid files, plus dead
sessions' repository embed snapshots. The cleanup preserves board data and
live embeds and uses no marker or recurring GC. The first board open (queries and
dashboard reads included) converts the old snapshot once under the folder
lock. New writes never update either old board representation, and key expiry
does not delete authoritative topic folders. An absent agenda/todo part is empty. A write replaces only the part files it changes, each atomically, and
the lookup every keyed call makes reads `key.json` alone, except
`dashboard.intake`'s: intake and the live snapshot writer's activity check
keep each key's identity (root, scope, parent, written only at mint) in
memory and stat `key.json` at most every 10 s, so a removed key stops intake
within 10 s while every other tool's gate sees it at once. Every content write,
mint's claim included, holds the key's record lock (`key.json.lock` in its
directory) across its read-modify-write, so ws processes serving one key do
not lose each other's updates; a lock not acquired within 2 s fails the tool
call. Readers take no lock, so a write of two parts (a mode switch's agenda
and todos) can be seen half-applied from another process. `key.json` is also
the key's claim (mint creates the directory exclusively) and its activity
mark: keyed calls refresh its mtime at most hourly, and the prune that
`ws.ferrule` runs at most daily deletes a key directory whose `key.json` is
older than 30 days.

The older layout, one `<cache root>/keys/<key>.json` per key, is still read:
a key with no `key.json` is served whole from its legacy file, and the
session's first write moves its session metadata into `sessions/<key>/`.
A board-bearing legacy file is retained byte-for-byte, and its original board
is copied into split `whiteboard.json` for rollback; boardless legacy files
are deleted. Reads never migrate session metadata, and no pass moves the rest;
an unwritten legacy file ages out through the same prune.

Restart every ws process after updating ws, as an install update already
expects: the session MCP servers (a host restart or MCP reconnect) and the
dashboard daemon (a newer numeric core replaces it; an equal-core build
requires explicit `daemon-start --replace`, or `POST /shutdown` then startup).
An older ws-mcp knows only `keys/`, so once a newer one has moved a session, the older one reports that key unknown. Downgrading loses
sight of moved sessions the same way; session keys are cache state, so a
fresh key is the recovery.

### Dashboard daemon

`ws-mcp dashboard` is the machine-wide daemon (one per OS environment, one
per cache root) that the plugin launcher starts when none runs and connects
every `serve --stdio` session to (see Daemon-Hosted MCP below): it hosts one
MCP server per connected session and serves the dashboard pages. Whether a
session's pages exist follows `dashboard.enabled` per session; the daemon
runs either way, and the MCP tools never depend on its pages. Its state lives
under `<cache root>/dashboard/`:

- `last-running-executable.json` - owner-only restart evidence written once by
  the lock-owning daemon: schema 1, retained executable path, version, SHA-256
  and build time. Hosted launchers reread and validate it only when a daemon
  start is needed. Older daemons ignore it; settled binaries remain available.
  The retired `hosted-plugin-roots.json` registry is ignored, not migrated.
- `hosted-packages/<ws|wsflow>/<hash>/` - retained launcher plus that package's
  five embedded manifests. Native Pi's separate content-addressed input stage
  supplies its workflow skills, resources and package-owner manifest.
- `dashboard.lock` - the single-instance OS file lock; `endpoint.json` - port,
  pid, version, and the shutdown token; `launcher-start.lock` - the persistent
  advisory lock updated Python launchers hold across a necessary daemon start;
  `dashboard.log` - the running daemon's
  log, including one line per session added to or removed from its index;
  `lan-token` - the LAN access token (LAN mode only), kept across daemon
  restarts; deleting it issues a new one at once. `localhost-secret` - the
  separate owner-only persisted key for the localhost browser cookie; deleting
  it invalidates existing cookies without a restart. `last-port` - the port the
  last daemon bound: when `dashboard.port` is taken by another program, the
  daemon binds this port before an OS-assigned one, so a restart keeps its
  origin (page localStorage, LAN bookmark); a free `dashboard.port` always
  wins.
- `sessions/<reply-id>/registration.json` - one entry per top-level lead
  session whose `dashboard.enabled` resolves on (session scope included),
  written by the serving ws-mcp at its first keyed call and heartbeated every
  2 minutes (a session the daemon hosts under `WS_HOSTED_SESSION` is
  registered by the daemon, not by the serving ws-mcp: its row carries
  `hosted: true` and `host_pid`, the session's harness process, 0 until the
  daemon has started it, with `host_stopped: true` while it has no process
  and starts none), and one per agent
  runner conversation (below), written by the runner with `agent: true`
  (and `unlisted: true` when its profile keeps it off the index page) while
  the daemon runs. A session counts as live while
  its heartbeat is under 5 minutes old and it has not ended. The daemon's
  index only reads this registry; ws-mcp prunes entries unseen for 7 days,
  except an agent conversation (an `agent` registration, or a directory
  holding `agent.json`), whose lifetime is the runner's while it may serve
  it: it heartbeats only while the daemon runs. A closed or handed-off lead
  and a closed spawned Pi child age from their last registry row write;
  pruning removes the session directory and its files. A handed-off Pi lead
  whose session file lives inside that directory is kept for its terminal.
- `sessions/<reply-id>/snapshot.json` and `sessions/<reply-id>/ring/` - the
  session's live snapshot (current lead and subagent activity, served from
  daemon RAM for slotted sessions and flushed for recovery on turn end, close,
  and debounced heartbeat) and its conversation ring (finished messages; while
  a session holds an active daemon `HostedSlot`, its ring stays pinned in RAM
  and appends to owner-only JSONL segment files `seg-<seq>.jsonl` rotated
  whole; unpinned/external writers write atomic per-entry `.json` files and
  close immediately; readers merge segments and `.json` files on load; the
  newest 2000 are kept, the page shows the newest 50 and
  pages older ones in through the session's history route).
  ws-mcp writes them from host adapter events
  (`dashboard.intake`) and from its mailbox send and recv path, only for a
  registered session; `internal/wslive` holds the schema and readers. An
  intake result carries an `accepts: status, compact, commands, command`
  line naming the optional event kinds this ws-mcp takes; an adapter sends a
  `status` event (the session's context, usage limits, and cost for the
  page's status band), a `compact` event (a lead compaction's `start`, then
  its `end` or `failed`, with an `auto` or `manual` trigger), a `commands`
  event (the slash commands it runs from a page command entry, which the
  snapshot keeps as `commands` until the next list or a reset), or a
  `command` event (a slash command it ran: its typed line and output text,
  a `command` ring entry) only while the latest result names that kind,
  since an older ws-mcp skips the kind per event, and the "dashboard off"
  result names none. A running compaction is the lead's
  `compacting` phase in the snapshot; its end or failure is a `compact` ring
  entry, and a reset or the next `turn_start` clears a compaction whose end
  never came. The snapshot's `repo` fields (the
  root's folder, branch, upstream, ahead and behind, and the counts of
  modified, staged, and untracked working-tree entries) come from ws-mcp's own
  `status --porcelain=v2 --branch --ahead-behind` read (plus short HEAD
  discovery when detached), cached per session and refreshed at most once
  per heartbeat interval from the writer's timer, never on an intake call, and
  only when a tool batch ended since the last read (an idle session runs no
  git); the snapshot write that finds the cache missing or stale (a session's
  first, then at most one per heartbeat interval, change or heartbeat alike)
  waits for the refresh, up to its 5 s timeout. There is no automatic fetch, so
  behind reflects the last one. The status read takes no optional
  lock and gets a 3 s sub-deadline within the 5 s overall timeout; when it
  fails, the reserved remainder lets cheap branch/ref reads preserve the
  available standing while counts stay zero. The
  page's status band shows only the folder; the git handle reads the rest
  itself (below).
- `sessions/<reply-id>/inbox/` - the board-answer inbox (`internal/wsinbox`):
  held messages the user sent from the page, one file each; `drained/` keeps
  the newest 50 as sent records. The adapter-only `dashboard.inbox_drain` tool
  drains it, and `mailbox wait --board-inbox` also wakes on it (peek only). A
  message promoted to steering is rewritten with `steer: true` and a fresh
  name, so it joins the end of the steer queue; a full drain claims the
  steered messages first, in the order promoted, then the rest, oldest first.
  `dashboard.inbox_drain steer_only: true` claims only
  those, and `mailbox wait --board-steer` wakes only on one (the wait an
  adapter arms mid-turn; exclusive with `--board-inbox`). An interrupt entry
  (`interrupt: true`, only its creation time as `sentAt`) asks the adapter to
  stop the running turn; it is never a message (no image cap, no
  held bubble; cancel and steer refuse it).
  `dashboard.inbox_drain interrupt_only: true` claims only those (each
  record `interrupt: true`; exclusive with `steer_only`), the steer-only
  drain leaves them, and the full drain drops them without a `drained/`
  record. `mailbox wait --board-interrupt` also wakes on one (alone, or with
  `--board-inbox` or `--board-steer`), counted as `interrupt` (its own
  `kinds` value), never as `board`. A command entry (`command: {name,
  args}`) is one slash command the page sent, only to a session whose
  snapshot lists `commands`, only from a browser on this machine (403 to any
  other viewer, a LAN token holder included), and only with a name that
  list holds (400 otherwise). Steering converts it to a steered text
  message containing its typed slash-command line, so the agent reads it
  mid-turn instead of the adapter running it natively. Left unsteered, a
  drain returns it in its place among the unsteered messages with a
  `command` field (steered messages drain ahead of it), where the adapter
  splits its batch and runs it natively. A hosted backend passes a plain
  message verbatim and runs a leading-slash line as a command, so a message
  from any viewer not on this machine whose text, trimmed of leading
  whitespace, starts with `/` is
  refused too (403); a slash later in the text is held as usual.
  A board item's sent mark comes from these records (the newest send's chip per item,
  held over drained; a chip sent before the item's latest re-ask marks
  nothing). A drained note-only chip (a note, no value) sent before the
  start of the session's newest settled turn clears its item's mark, so the
  item counts as waiting on the owner again; the note stays in the
  delivered message and the drained record. The settled turn is the ring's
  newest `turn` entry, which starts at its time minus `durationMs`. A
  `turn` entry without a duration settles nothing.
- `sessions/<reply-id>/ack-check.json` - the daemon's answer check
  (`internal/wsdashboard/ack_check.go`) record, a sibling of `label.json`:
  `{"through": <epoch ms>, "interruptedAt": <epoch ms>}`. When a session's
  turn settles, the daemon checks the batch of answers that turn received
  and holds one plain inbox message (no chips) naming the batch items still
  `open` on the board, asking the agent to record them with `wb:answer`.
  The batch is every drained value chip (an answer, with or without a note;
  never a note-only chip or a topic note) sent in `[through, settled turn
  start)`, minus the chips sent before the item's re-ask. The check runs
  about 1 s after a session change, an agent runner write, a hosted
  snapshot change, or a hosted ring append (a daemon start re-checks every
  session through the index's first refresh), and only when all of these
  hold:
  - the session source reads board items;
  - the session is indexed, has a board, and is not an agent conversation;
  - its host adapter is live;
  - the lead is idle (no turn, phase idle, no permission wait);
  - the ring's newest `turn` entry has a duration and an end `reason` other
    than `aborted` (the Claude mod's terminal stop);
  - that turn started after `interruptedAt`, which the dashboard interrupt
    route records;
  - the inbox holds no message (held input means another turn is coming).

  A skip leaves `through` as it was. A non-empty batch moves `through` to
  the turn's start, whether a nudge goes out or not, so each answer is
  checked once and a nudge (no chips) never triggers another. The
  session's first check (no `through`) only seeds it and sends nothing.
  The record is written, under the daemon's inbox lock, before the nudge,
  and only when it is unchanged since the check read it. A crash can
  therefore lose a nudge but never double one, and two checks never both
  send.
- `sessions/<reply-id>/agent.json`, `instructions.md`, and `pi/` - an agent
  runner conversation's own files (`internal/wsagent`): the owner-only
  record a process start needs after a daemon restart (profile, backend,
  working root, backend session id, model and effort, the conversation's ws
  session key), the composed instruction file appended to the harness's
  system prompt (owner-only; it carries the session key when the profile
  allows ws tools, and a Claude lead's file always carries the key and the
  `workflow_manual` instruction), and for the pi backend its session file and MCP bridge.
  The daemon reads the conversation from its live stores alone, since no
  ws-mcp serves it and it has no session record.
- `sessions/<reply-id>/permission.json` - a hosted lead's explicit common
  permission selection, separate from `agent.json`: `{permission_level}`,
  written atomically with mode 0600 and preserving unknown members. Absence
  uses a legacy-derived level without writing a selection; an existing
  unreadable or invalid selection is an error, not absence
  (`internal/wsagent/permission_store.go:11-60`,
  `internal/wsagent/permission.go:98-111`).

After a store write the serving ws-mcp sends `POST /nudge {reply_id,
project}`. The daemon wakes every `mailbox wait` it hosts on each nudge,
registered or not, before it checks the named session (at most one recheck
per wait per second, plus a trailing one). A server the daemon
hosts (Daemon-Hosted MCP below) instead hands each live snapshot to the
daemon in memory (`wsdashboard.HostedSlot`): the daemon serves that copy in
place of `snapshot.json` and wakes the session's pages with a live-only
wake, whose render re-reads only the snapshot (and the ring after an append,
which the slot signals in place of a nudge). The hosted writer still writes
`snapshot.json` for recovery: once per heartbeat interval while the state
changed, at each turn end, and when its server closes (which also drops the
copy, so the file renders again); an idle heartbeat writes nothing and only
moves the copy's `updatedAt`. A session whose directory the daemon does not
serve stays on the file-and-nudge path. The daemon runs full registry
safety discovery every five minutes, with targeted 30-second legacy
liveness/recovery checks between scans. An ended session already applied
is not re-read unless a nudge, resume, or retry requires it. Scheduled per-session re-reads are coalesced to at most one per second,
with a trailing read. A runner write refreshes only the live-store fingerprint
(snapshot and live directory entry names), retaining the last full read's
record, board, tickets, mailbox history and contacts. A nudge, periodic safety
refresh or board poke reads both parts. Known inbox-only changes on boardless
sessions list only the inbox stores; unknown runner writes still list every
live store. A re-read builds no panel; the board renders only when a page
reads the changed session. Active hosted sessions pin their conversation ring
in daemon RAM (loaded once from segments plus any `.json` entry files, with
held-open append handles released on slot close or shutdown; closed and
non-hosted rings are fingerprinted by directory metadata without loading bodies
or pinning RAM); drained-inbox entry files are parsed once per daemon and kept by
name for the window the page shows; mailbox
history, contacts, and unread counts come from the memory store (Mailbox
store below) with no file read. The small files every read and render
re-reads (session snapshots, the session record's part files) share parsed
values and stat validation for up to one second per path. Recent files are reread at validation until
outside the two-second timestamp ambiguity window. Explicit session change
signals expire the targeted source and snapshot validation before the index
reads, and write admission/capability checks read current evidence rather than
the presentation cache. A session's entries drop when it leaves the index. A
page's refresh stamp and the board memo stamp a ticket status directory by
its listing rather than each ticket file, and key the board on the
session's own reply-registry contacts and unread count rather than the
whole registry file.

The daemon serves its pages with no CORS: `/` lists sessions by project
(ended ones for a day), `/s/<reply-id>` shows one session, and `/events` and
`/s/<reply-id>/events` are their SSE streams, which send only the page regions
whose hash changed. A session with a fresh adapter snapshot gets the live
layout (lead, agent tree, conversation, board); one without gets the
board-first page. The live layout's composer carries the status band: the
context bar (percent, tokens of the window, the last response's cache hit
rate) above the message box, and below it the folder, the 5-hour and
weekly usage limits (each with its reset and its
pace, which the page computes), and the cost (`~$` for an estimate, a
trailing `+` when calls of unknown price were left out). A part with no
figure is left out; context and limits warn at 80% and turn critical at 95%;
below a 360px band only the limit under more pressure stays. The composer also
holds the `cmds` region, the snapshot's `commands` as hidden data, rendered
for a viewer on this machine only (a LAN viewer's page has no region and no
picker): typing `/`
in an empty message box opens a picker over them (grouped under user and
agent headings when any command names its `layer`), and a pick becomes a
command selection whose box text is its arguments, sent as `{command, args}`
alone (answer chips and notes stay for the next send; images refuse it). A
session with no list renders the region empty and gets no picker. A
`command` ring entry shows its typed line and output text, or an "opens in
the terminal" note when the text is empty.

The session page's drawer frame (not the `/` hub) carries a git handle left
of the panels handle: the session root's branch, abbreviated (an
`impl/<parent>/<stem>` branch as a hammer and the stem, a `goal/...` one as a
gold trophy and the stem, a branch naming both kinds as both icons; any
other branch whole, ellipsized; a detached HEAD as its short id), its ahead
and behind counts, and a dot for uncommitted changes. Its title spells the
whole branch and, for an impl or goal stem, the title of the ticket
`<root>/ai-docs/tickets/*/<stem>.md` names (front matter `title:`, else the
first `# ` heading; 16 KB read, cached a minute). The daemon reads it itself
(`internal/wsdashboard/gitbadge.go`) rather than from the snapshot, but
follows it: when a session's live snapshot brings repo figures that differ
from the ones it brought before and from the handle's last read, the stream
asks for a read as a refresh would, so the two agree once it lands. It reads
a root only while a page watches its git display: once on first use,
then checks Git administration metadata at the prior 3-minute cadence.
Unchanged metadata suppresses commands, but a complete read is forced within
five minutes for in-place edits/untracked contents. Explicit refresh bypasses
the hint, including `POST /s/<reply-id>/git` (sent when the git drawer opens; it needs
an `Origin` equal to the daemon's own, answers 204, and 409 when no open page
watches the root). One goroutine per watched root runs every read, so pages
sharing a root share one read and no two reads of a root overlap; a refresh
inside 2 seconds of the last read waits for that gap's end, and requests
arriving meanwhile fold into that one read. The last stream's close ends the
root's reads and drops its value. New WebSocket pages opt into `display=0|1`
and optional client visibility frames: hiding releases only git/rail watches,
not functional region/tracker streams; showing refreshes the display. Older
WebSocket clients and SSE retain stream-lifetime watches. A complete badge
read is porcelain-v2 status plus top-level discovery (and short HEAD when
detached), with no optional lock, in the C locale,
bounded by 10 s together, with no daemon-wide lock held; it never fetches. A
non-repository root shows no handle; a failed read shows it dimmed, with the
reason (git's message) in its title.

The handle opens the git drawer (`internal/wsdashboard/gitdrawer.go`,
`gitpush.go`), which shares the panels drawer's frame. Its right panel lists
the changed files from `GET /s/<reply-id>/git/status` (JSON: the branch
standing and the staged, unstaged, and untracked files, each with its status
letter and `diff --numstat` line delta, at most 300 per group; read when the
drawer opens and after each action, with no optional lock and no fetch) and
carries three actions, each a `POST /s/<reply-id>/git/<push|pull|fetch>`
under the same rules (POST only, the daemon's own `Origin`, the session's
root, one action per root at a time, 60 s): push re-reads the branch,
refuses a detached HEAD, a branch with no upstream, and one with nothing to
push, and runs `git push -- <remote> refs/heads/<branch>:<merge ref>` with
no option, so the remote moves by fast-forward only; pull fetches the
upstream's remote and fast-forwards only, refusing a detached HEAD, a branch
with no upstream (also one the fetch pruned), modified or staged tracked
files (untracked ones do not count), and a branch that has diverged; fetch
is a plain `git fetch`. No network step of push, pull, or fetch ever
prompts on a console (the daemon's console is hidden, so ssh runs in batch
mode): an ssh key that
needs a passphrase must be in ssh-agent, and an unknown host must be
accepted once in a terminal, while a Git Credential Manager sign-in dialog
still appears. An action answers 204, 200 with a note (nothing to
pull), 409 with the refusal's reason, or 502 with git's message (a ref's
`! [rejected]` line first, then an `error:` or `fatal:` line, over a hook's
`remote:` lines), which the panel shows inline; each re-reads the handle.
The drawer's reads parse git's stdout alone, so a warning on stderr (CRLF
conversion) never corrupts them. The wide left area is reserved for a
future commit graph. There is no commit action.

The panels drawer's pooled-worktree card (`internal/wsdashboard/worktrees.go`,
served when the daemon has a `WorktreeSource`, else 404) reads `GET
/s/<reply-id>/worktrees`: JSON `{pool, worktrees, sizing}`, the session
project's `worktree_pool` and the pooled worktrees `worktree.list` owns
(path, profile, branch or detached, lease with holder and parent reply-ids,
acquire and last-activity times, quarantine, disk size), read when the
drawer opens. Sizes are measured off the request path (two walks at once,
cached 2 minutes); an answer with a size still unmeasured says `sizing`, and
`?wait=1` holds it until those sizes land (20 s at most). `POST
/s/<reply-id>/worktrees/cleanup` (body ignored) follows the composer's send
rules below and holds one message: a line in the page's language asking the
session to clean up unused worktrees, then the pool as text, cut at 48 KiB
with a `... N more worktrees` line (singular for one). The dashboard never
removes or releases a pooled worktree itself.

Every session page carries the session rail (`internal/wsdashboard/rail.go`,
`pageview_rail.go`): the listed sessions grouped by repository (or plain
folder) and worktree. It is one shared render (region `rail`, keyed by
language and theme, no reply-id) that each session stream sends beside its
page with a broker subscription of its own, so a session's change renders
the rail at most once for all open pages and no other session's page;
unchanged visible rail inputs reuse the render. The daemon retains each
session's snapshot, repository and last message in memory, refreshing only
sessions whose index revision or named session/snapshot/ring generation moved.
Explicit current-state queries such as the hiring roster still read through.
Clock-derived liveness is recomputed from those retained inputs. A transient
snapshot or ring read failure retains last-good presentation and keeps only
that session eligible for retry on refresh, even without another wake.
Rail passes are capped at one per second per shared language/theme view, with
a trailing pass so the last wake is not lost (`internal/wsdashboard/rail.go`). Streams opt into row deltas
with `railrows=1`: the first send on each connection is a full `region`
(`name: "rail"`), even when `h` holds the current hash. Later `rail-rows`
SSE events carry `{hash, order, rows: [{key, hash, html}]}`: the full row
order and only new or changed rows, keyed by reply-id. A changed grouping
shell or an uncertain row decomposition falls back to the full region;
without the opt-in, every rail update stays a full region
(`internal/wsdashboard/sse.go`, `Daemon.stream`,
`internal/wsdashboard/rail_rows.go:28-32,88-106`). The page also uses these
event names over WebSocket, reconnecting for a full rail if it cannot apply
a row patch (`assets/dashboard-stream.js:144-175,178-191,214-224`). A session's
repository comes from the registration's `git_common_dir` (ws-mcp's one
`rev-parse --git-common-dir` at first registration), else from its root's
`.git` on disk. The worktree rows come from `git worktree list --porcelain`
through the source's optional `RepoWorktreeSource` (pooled worktrees under
the `worktree_pool` roots show only while they hold a session), read only
while a page watches the rail display: on first use, for a newly shown
repository alone, and after changed Git metadata or the five-minute safety
deadline. The prior 3-minute checks remain; metadata never replaces mutation
admission, and pool configuration is re-resolved on complete reads. Explicit
refresh and opt-in visible resumption bypass the hint, including `POST /rail/refresh` (own Origin;
409 with no streaming page). Change counts come from the newest session
snapshot in the worktree; the rail never reads a working tree. Drafts,
stars, and collapsed workspaces are browser-local (`assets/rail.js`).
A linked worktree row that no pool root holds (the source's pools, the
default `<root>/../.ws-worktrees/<name>`, the legacy `<root>/.ws-worktrees`)
offers a delete to a browser on this machine (`railworktreedelete.go`):
`POST /rail/worktrees/delete/preview` answers the check (changes, a detached
HEAD's unreached commits, initialized submodules, lock, the sessions and
terminals in it, whether force is needed, and a token of that state); `POST
/rail/worktrees/delete` checks again and answers 409 with the fresh check
when it is refused (primary, pooled, missing, locked, a live session or
terminal), needs force the submit did not carry, or was forced on a stale
token; else it runs `git worktree remove [--force]` from the primary and
keeps the branch. A viewer elsewhere gets 404.
Ended sessions fold apart under each worktree and stay listed as long as
the index holds them (their registry directory and session record exist),
past the index page's 24-hour window: on a daemon that hosts lead sessions
an ended row carries how it comes back (`RailRow.Revive`): `reopen` for a
closed hosted session, `takeover` for a claude or pi session a terminal ran
in a repository (shown disabled while its key record names no harness
conversation), nothing for a plain-folder session. Whether a terminal
harness has exited is asked when the entry is used (the **Moves** bullet
of the hosted lead sessions below). `PruneRegistrations` spares an agent runner's directory
while the runner may serve it; a lead whose record is closed or handed off,
or a closed spawned Pi child, ages by its last registry row write like any
ended session. After 7 days unseen, pruning removes its session directory,
including the Pi child's session file needed to reopen it. Open children and
other agent records (the tracker's support agent) keep the runner's lifetime.
One retired lead is never pruned: a handed-off pi session whose session file is inside its
session directory (the daemon started it), since the terminal's pi runs on
that file from the hand-off on.

On the served board and in the conversation's lead
replies, a file link renders as an action link (`data-wl="file"`, titled with
its repo-relative path) and an http(s) link is marked `data-wl="url"`: a
click opens a menu (open in new tab, open in system browser, open in native
app for an image file, copy relative or absolute path; copy URL for an
http(s) link), and Cmd/Ctrl+click opens a new tab. Every markdown renderer (the board, lead replies, tracker text) also
links a bare http(s) URL (a loopback or LAN host included) and a bare file
path (absolute or drive, two segments or more, or relative with a `/` and a
file extension; optional `:line[:col]`) outside code and raw anchors,
rendered as a written link to the same target would be. A file inside the
session's project root links `GET
/s/<reply-id>/file?path=<root-relative path>`, the file view, which serves a
regular, non-symlink file that still resolves inside the root after every
symlink (404 otherwise), to a loopback viewer and a LAN token holder alike:
an image inline, markdown rendered in the board's escaped mode with relative
links resolved against the file's own directory, other UTF-8 text as
`text/plain`, and a binary or over-2-MiB file as a page offering the path
copies. A preview card shows an HTML or SVG file inside the session's own
root in a sandboxed frame, through a 15-minute capability on
`/s/<reply-id>/fs/cap/`; for a viewer on this machine the capability route
refuses only a dot-named path, and for a LAN viewer a dot-named or
git-ignored path (failing closed when the ignore check fails), and a file it
refuses previews as its source (`internal/wsdashboard/fs_capability.go`
states the rules). A file outside the root offers the copies only, not the file view,
with two exceptions. For a viewer on this machine, a link to one on the
board or in a lead reply the live ring still holds opens in the preview
card, and an HTML file also in a sandboxed tab of its own: the click posts
`POST /s/<reply-id>/fs/outside {"path"}`, which answers only a loopback,
same-origin POST naming a path the page links (404 otherwise, a LAN token
holder included) and mints a 15-minute capability for the linked file's
folder, served to this machine only on `/s/<reply-id>/fs/ocap/` and
`/fs/oview/`. A volume root, the home, or a folder directly under either
grants the linked file alone; `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.config/gh`,
`~/.netrc`, and `~/.docker` never open; below the granted folder a
dot-named path is refused, the linked file itself excepted, while a
git-ignored one is served, as the in-root capability route serves it to this
machine (`internal/wsdashboard/fs_outside.go` states the rules). A card re-mints on
every load, and a tab whose capability expired lands on a page that re-posts
the mint. The other exception is an image
written as an absolute path in a lead reply, a board item field, or (through
the relay below) a topic body. A board image inside
the root loads through `GET /s/<reply-id>/image?path=<root-relative path>`,
which serves only an image-extension file under the same containment, and
any other image renders as its path, with that one exception: an absolute
image path outside the root (not a UNC or device path, image extension on
the name and on the file its symlinks resolve to) renders inline on the
image route and links through it (`data-out`), and the route serves it only
to a viewer on this machine (404 to a LAN viewer, token or not, and to a
request whose `Sec-Fetch-Site` is `same-site` or `cross-site`; absent,
`same-origin`, and `none`, a navigation the user started, are served), with
`Cross-Origin-Resource-Policy: same-origin`. Where such an image fails to
load (a LAN viewer, a missing file), the page puts its alt text and a
copy-only path link in its place, and its link offers the copies only; for
a viewer on this machine its menu offers open in new tab and open in native
app, not open in system browser. On a served page each topic body on the board
renders in its own sandboxed `srcdoc` frame (`allow-scripts allow-popups
allow-popups-to-escape-sandbox`, an opaque origin that sends no cookie). No
server response inlines an image as a `data:` URL: each file image in a
topic body, inside the root or outside it, stays path text marked
`data-relay` with its image route URL, and the frame element lists those
URLs as its own `data-relay`. The frame cannot load
that route itself (its opaque origin's requests are `Sec-Fetch-Site:
cross-site`, which the route 404s), so it asks the page by `postMessage`;
the page's relay handler accepts the request only from one of its own body
frames and only for a URL on that frame's list that is this session's image
route for an image name, fetches it same-origin (served only to a loopback
viewer when outside the root), and posts the frame an image `Blob`, which
the frame shows in the path text's place once it decodes. A LAN viewer's
fetch of an outside image 404s and the path text stays. The route streams
the file as is with an ETag of its size and modification time, and caches
a URL whose `&v=` matches that version as immutable. A loopback viewer is
also served a message attachment by its absolute path in a session's inbox
attachments directory, which the credential deny set otherwise refuses with
the rest of the daemon's state directory. The served board renders the 30 most
recently closed topics (with nothing waiting on the user) and a show-more
control; `GET /s/<reply-id>/closed?from=<index>&n=<count>` (from at least 1,
n default 30, at most 300) answers `{html}` with the next closed topics in
close order, rendered and framed as on the board, through the optional
`wsdashboard.ClosedTopicsSource`; a session without a board is 404. A finished lead reply draws its mermaid
fences; the reply still streaming keeps them code blocks. A finished lead
reply also keeps its inline HTML and SVG through an allow-list sanitizer: no
script, form, embed, or fetching element or attribute survives, and a raw
link or image follows the same link rule, so it renders as the same action
link or path text as its markdown form; the streaming reply, user messages,
and mail bodies show HTML as text. `POST
/s/<reply-id>/open {target}` opens an http(s) URL or this session's own file
view path in the host's default browser, and `{native}` opens an absolute
image path (inside the root or outside it, under the image rules above; the
file its symlinks resolve to is what opens) in the host's default app for
it, with the OS open command, run with no shell (`rundll32
url.dll,FileProtocolHandler` on Windows, `open` on macOS, `xdg-open`
elsewhere); a body names exactly one of them. It requires a
viewer on this machine (404 to any other, token or not) and an `Origin`
equal to the daemon's own, and the page offers the items only to such a
viewer. The live layout's
composer sends: `POST /s/<reply-id>/message {text, chips, topics, images,
refs, refNums}` holds the message in the session's inbox as its own held
message; sends never merge. `images` are new uploads `{name, data, n}` (base64,
sniffed as PNG/JPEG/GIF/WebP, at most 8 per held message), `refs` name files
a cancel returned, and `refNums` numbers them; the daemon writes the uploads
into the inbox's `attachments/` directory. The text names image `n` with an
`[Image #n]` marker, and the delivered message is the `<wb:answers>` block
of the board answers and topic notes (`wsinbox.AnswerBlock`), then the text,
then one `Image #n: <absolute path>` line per image. Numbers are per
message, as the draft numbered them: a turn that delivers several held
messages joins their texts, so the same `Image #n` can name a different
image in each, and each `[Image #n]` marker means the image of its own
message's lines. The page still reads the older `[image: <path>]` lines. `POST
/s/<reply-id>/cancel {id}` takes a held one back, answering `{text, chips,
topics, images, nums}`, and `POST
/s/<reply-id>/steer {id}` promotes a held one to steering, answering the
rewritten entry's new `{id}` (409 once the session has received it). The
adapter claims a steered message at the session's next tool boundary instead
of at the turn end. `POST /s/<reply-id>/interrupt` (body ignored) takes back
every held message, steered or not, and writes an interrupt entry in one step
under the inbox lock, answering `{held: [{id, text, chips, topics, images, nums}]}`
in queue order (steered first, then the rest), each shaped as a cancel's answer. All four require an `Origin` equal to the daemon's own,
and a session without a fresh adapter snapshot gets 409. `whiteboard.update`'s
`full:` line names the session's dashboard URL once its last nudge was answered,
and the page path otherwise. The URL never carries a login code, because tool
results land in transcripts; a browser without the localhost cookie gets the
login gate, whose clipboard button mints the single-use link out of band when
the host has a native clipboard writer.

The daemon creates one tracker host (`wstrackerhost.New`) before it starts
serving, shared by the tracker's background poll and the tracker routes, and
never a second; a host that could not be created still goes in, so every
tracker route answers 503 with its (redacted) error and the page's "+ store"
dialog shows it. The routes, JSON in and out with errors as `{"error"}`:
`GET /tracker/stores` (`{stores, rev}`), `GET /tracker/stores/<id>/view`
(`{view, seen}`; the view adds `html {items, comments}`, item bodies and
comment texts rendered as markdown under untrusted-text rules: only http(s)
links and in-page fragments become anchors, tags stay text), `GET
/tracker/stores/<id>/search?q=` (`{ids}`: the items passing the tracker
search grammar, `wstracker.ParseQuery`, the one `tracker.query`'s `q` takes;
a malformed query is 400 with the message the Items panel shows), `POST
/tracker/stores/<id>/dispatch {events}` (origin forced
to `human`, `"$N"` names the batch's Nth event; a refusal is 409 with
`refusal {index, type, reason}`), `POST .../check` and `.../sync` (a report; an
unreachable remote is `offline: true`, not an error), `POST .../seen {badge,
read}` (204), `GET`/`POST /tracker/profiles`, and `POST /tracker/register`.
The editor routes: `POST .../render {text, attachments, wiki}` (an editor's
preview through the view's renderer; `wiki` names a wiki whose tree images
also show), `POST .../attachments?name=` (raw image bytes staged for a later
write) and `GET .../attachments/<sha256>` (the bytes, magic-byte typed). The
wiki routes over the store's `refs/heads/wiki/<name>` branches: `GET
.../wiki` (the list), `GET .../wiki/<name>/pages`, `GET
.../wiki/<name>/page?path=` (content plus the commit a save sends back as
`base`), `POST .../wiki/<name>/save` (an edit; a merge conflict is 409 with
`conflict {path, kind, reason, base, theirs, oursText, theirsText}`, a
refused edit 422), and `GET .../wiki/<name>/attachments/<sha256>`.
An unknown store id is 404. Every POST requires an `Origin` equal to the
daemon's own and a body under 256 KiB, and runs to completion even when the
page closes mid-request. Every string a route returns has credentials
redacted. The tracker revision bumps after a route's write (and after a
check or sync, even a failed one), after the daemon's first poll round and
each later round that changed something (`wstrackerhost` `StartPoll`,
`PollOnce`), when the 10 s store watch sees a write by another process, and
when a reload that a page read stopped waiting for lands with a view other
than the one that read was served (`now` aside): a page read waits at most
500 ms behind whatever holds the store's lock (a wiki save, another
process's write), or a slow reload, then serves the last folded state
(`ViewWithin`). A check or sync holds that lock only for its local steps:
its fetch and push run under the store's network lock (`net.lock`), which
only a read of an attachment not yet fetched waits for, with the same
500 ms bound. The daemon's tracker git
runs resolve the git executable once per `PATH`. Both pages' SSE streams send `event:
tracker` with `{"rev": N}` when it differs from what that stream last sent
(once on connect too), and the page re-reads.

The daemon also hosts headless agent conversations (the agent runner,
`internal/wsagent`, wired in by `cmd/ws-mcp/dashboard.go`): a Claude Code or
Pi process per conversation, started from a named profile in the
`agent.profile.<name>` config family (a whole-object structured value at
project, repo, or global scope, resolved through the conversation's working
root; repo scope may not set `mcp_servers` or `plugins`; `agent.` is a
reserved prefix no adapter manifest may claim). Two profiles are built in,
`tracker-support-claude` and `tracker-support-pi` (one per harness, since a
profile fixes its backend; a project or global profile of the same name
replaces one whole); their `instructions` value `builtin:tracker-support`
names a text embedded in the binary instead of a file under the working
root, and `builtin:<name>` accepts only the embedded names. The Claude one
allows the `Agent` tool as its only built-in tool, so it may spawn
subagents: Claude applies the launch's `--tools` and dontAsk allowlist to
them too (a subagent type whose every tool is outside it is refused), and
its system/init reports the tool as `Task`, which the init check accepts
for `Agent`. Claude runs such a subagent as a background task: the
runner maps its `task_started` and terminal `task_updated` /
`task_notification` lines to a snapshot subagent (running, then done or
failed; failed too when the process ends first), which drives the
conversation's subagent badge, and drops the subagent's own
`parent_tool_use_id` lines from the lead's reply and batches. Each
conversation is a dashboard session (registration, snapshot, ring, inbox)
whose page shows it like a lead session. Its routes, behind the same access
guard: `GET /agents` (every conversation's state), `POST /agents {profile,
root}` (open one), `GET /agents/<id>`, and `POST /agents/<id>/send {text}`,
`/interrupt` (answers the held text it took back), `/stop` (ends the
process; the next send resumes the backend session), `/close` (ends the
conversation for good; later writes get 409), and `/settings {model?,
effort?}` (from the next turn). Every POST needs a JSON body
(`Content-Type: application/json`) and either no `Origin` (a client on this
machine) or the daemon's own, so no other web page can drive an agent. After
a daemon restart or replace, process continuity follows the ownership matrix
in Daemon replacement and process ownership below.

An attached ws-mcp gets
`WS_MCP_ALLOWED_TOOLS` (the profile's ws tools) and `WS_MCP_NO_DASHBOARD`,
and no mailbox identity. Its command is Python plus the selected build-owned
package's staged `bin/ws-mcp-launcher.py serve --stdio`, never the daemon's
staged executable as a stdio server.
The built-in global-only `hosted.plugin_package` key (`config.tune`, values
`ws` or `wsflow`, fixed default `wsflow`, visible with no-agent) selects the
whole package for hosted Claude/Pi leads and this attached MCP. Each process
start reads the setting; running and holder-reattached processes stay unchanged.
Both packages come from the daemon's embedded inputs (or its explicit process
source override), never a connected plugin root or registry. An invalid
hand-edited selection warns and uses `wsflow`; missing/corrupt build inputs
refuse the launch. Non-hosted connections keep their own package identity. The launcher's interpreter (`WS_MCP_HOLDER_PYTHON`) is reused
when known, else `python3` on PATH (Windows: `py -3` fallback). The conversation's
root overrides `WS_MCP_PROJECT_ROOT` in that command's environment, keeping
proxy mode's exact arguments and preventing the daemon's lead root leaking
into the worker. A proxy-hosted worker still uses its capability token for
nested `agent.*` calls (and legacy `pi.*`), not the daemon's lead runner handle.

A spawned conversation (`internal/wsagent/spawn.go`) is a non-lead
conversation another session opens with its first message (`Runner.Spawn`:
profile, root, optional cwd, model, effort, label, and parent `{kind,
reply_id}`) and steers by its id; its record keeps `cwd`, `label`, and
`parent`, and it is unlisted like a support conversation. Each turn settles
once: done, failed (the harness's error, its process ending under it, or a
process that could not start or take the message), or interrupted (an open
turn the next daemon cannot recover because its holder/process is gone,
or it ran without a holder; the record's `turn_open` marks it). A daemon
replace alone does not interrupt a holder-backed Pi turn. A settle of a
conversation with a parent goes to the runner's `Options.Report`; closing
one settles nothing.
`Runner.Wait` waits for a turn that took every message sent before the
wait (a message the page cancelled before a drain counts for no turn, and
a slash command's turn covers none of the text drained behind it); the
last reported turn is kept in the record (`last_turn`), so after a daemon
restart a wait on a conversation that settled before it answers at once.
`Runner.Watch` reports each change of a conversation with a parent.
The built-in `pi-worker` profile is Pi's coding tools plus ws's
`git.status`, `git.diff`, `git.log`, `git.merge_base`, `git.commit`,
`git.resolve_impl_branch`, `tickets.create_empty`, `tickets.verify`, and
`agent.spawn`, `agent.interrupt`, `agent.list`, `agent.tiers`, with no instruction file. The built-in `pi-mini-lead`
adds `tickets.query/acquire/release/close/move/checklist`, read-only
`note.query`, `playbook.read`, `playbook.render`, `path.generate`, and
`route.resolve_implement`. It keeps a delegate key, not lead authority, and
adds no note, whiteboard, or tracker writes. A spawn records its `tier`
(`SpawnSpec.Tier`, `Record.tier`; an unknown name is refused).

The hidden compatibility `pi.*` handlers (`internal/mcp/pi_tools.go`, lead-only)
retain these calls for older sessions: `pi.spawn {prompt, tier, profile?, cwd?, label?}` (model and effort from the
tier's entry in the `pi` harness bucket of `agents.tier`, which only the
user's `config.tune` sets; an unset tier is refused with that key named;
`cwd` relative to the session's root; `profile` names any resolvable non-lead
Pi `agent.profile.<name>`, default `pi-worker`), `pi.send` (a running child's native
Pi steer at the next tool boundary; with no turn running, a new turn at once;
a sleeping child resumes on the same session file, keeping its id, cwd, model,
effort, and label; reopening a retired child refuses a missing, inaccessible,
or non-regular session file), `pi.list` (the caller's own children, with MCP-only
`sleep` and `deleted` labels; `deleted` means a retired child's session file is
unavailable, not a complete resume-admission check), `pi.stop` (ends the process
and closes its descendants; the record stays resumable by `pi.send`; a child
already closed answers as stopped), `pi.transcript`, and
`pi.wait` (default 300 s, at most 1800 s, for the turn after the last
send; on a closed child, its last reported turn when that turn took every
message sent). The daemon mails each settled turn to the parent lead's reply-id
queue with the display-only label `agent:<subagent id>` (no reply address) and
wakes the mailbox waits; the first body line directs replies to the child's
address through `mailbox.send`, and the report head reads `[subagent
id:<id> ...]`. For other senderless system mail without a label,
`mailbox.recv` still shows `system`. For Pi completion reports, a steered message belongs to the turn that takes it, whose completion
mail covers it without an extra turn. A steer refused or not taken before
the turn ends is cleared from Pi's queue and sent as the next turn instead.
The compatibility stop itself mails nothing; a turn started by a later `pi.send`
reports as usual. New callers use `agent.spawn`, `agent.interrupt`, `agent.list`,
and `agent.tiers`, and send follow-ups through `mailbox.send`; `agent.stop` is
not a tool. Interrupt ends the turn without closing the conversation.
Pi subagents run under session holders, not as the daemon's own
child processes: a running Pi turn survives a daemon restart or replace.
The successor rejoins the same process and replays its output, including a
completion produced while no daemon ran; completion mail follows the ordinary
settled-turn path, not an interruption report. `pi.list` keeps an open turn
`running` during rejoin, then reflects its recovered settlement. A `pi.send`
accepted during rejoin waits in the inbox until replay finishes, then steers
the still-running turn or starts the next one; already-taken steers retain
their turn coverage across the swap.

This needs a surviving holder and child, and a successor using the same state
directory that can reattach. A live but unreachable or incompatible holder
leaves input waiting; `running` during rejoin is not proof of fresh progress.
If replay has gaps, Pi RPC recovery reconciles the turn; a failed recovery
ends the process and reports failure. A process crash or an explicit stop
ends the running turn, and a Windows holder marked not restart-safe may die
with its starter's job object. A runner without a holder manager cannot
preserve the process; a configured holder that fails to start refuses the
launch rather than falling back to an unheld child. If the process is gone, a later `pi.send` resumes the saved Pi session
through the ordinary lazy-start path; no second spawn or record is needed.
The parent's reply-id is derived from its session key, so restarting a lead
with the same key keeps its children addressable.
`tools/list` never shows the tools: `agent.*` and mailbox routing
replace them, and the handlers stay for sessions started on an older plugin
until the next minor deletes them. A call answers only when
`dashboard.pi_spawn_enabled` is on (ws on, wsflow and Pi off), the server is
hosted by the daemon (`ServerOptions.Agents` is the daemon's runner; a
legacy standalone ws-mcp has none and a call is refused naming the daemon), and
`pi --version`, probed once per process, is 1.0 or later.

Mail to a spawned child's address (`id:<reply-id>`, as `agent.spawn`
returns it) is ordinary `mailbox.send` to that reply-id queue in the
memory-first store: one restore-ring line per send and no `mailbox.json`
while the daemon runs, with the usual history and reply-id handling. The
runner watches every spawned child's mailbox (`Options.Mail`, the daemon's
`LeadMailbox` in `internal/mcp/lead_mail.go`), awake or asleep, and claims
what arrives. Each claimed message's body opens with a `[relation:
parent|child|peer]` line, computed at claim time from the `Record.Parent`
lineage (no Envelope field); system mail carries none. A running turn takes
the mail as a steer when the harness's catalog profile does not grade steer
`unsupported` and the process has a steer handle (Pi, with no profile: the
handle alone); a child's steered mail is held like a steered report, so its
turn's report covers it. Otherwise the mail waits in `Record.Mail` for the
next turn. Mail to a sleeping child wakes it, ignoring the width cap, and
resets its waiting clock (`LastActiveAt`); a refused wake keeps the mail and
tells each sender once (`Options.WakeRefused`, posted as `agent:<id>`
system mail). Each intake-fed
server watches the runner and feeds the owning lead's recursive spawn tree
(`pi:<id>`, `agentKind: "pi"`, linked to the child's page) into its live state;
a live session the server creates, or one an adapter resets, gets its open
descendants' rows again, so they come back after a reconnect. A dashboard-hosted
lead instead gets those rows from its runner-owned snapshot writer, including
after daemon replacement; hosted intake remains off. Descendants nest under
their spawner through ordinary `parentId` links. When a harness also reports the
same conversation (its reply-id or native conversation id), the snapshot keeps
one daemon-owned lifecycle row with the harness's activity and telemetry. An
intake-only session whose dashboard feed is off gets no row. The agents popover
shows a running Pi row with a blinking `pi` badge in place of the pulse.
The composer's subagent badge opens that popover over the conversation;
it stays visible but muted with no running agents, even on a non-live
session. Escape or a click outside closes an unpinned popover; a pin is
browser-local and reopens it on the next load, and close unpins it
(`assets/dashboard.tmpl:239-250`,
`assets/dashboard-composer.js:1459-1464,1483-1489,1525-1565`).

A spawned worker delegates through the daemon's capability-token route
(`internal/wsagent/children.go`, `internal/mcp/agent_tools.go`); the daemon
owns the whole tree. Current `agent.*` calls and the retained compatibility
Pi calls share this route; the Pi-specific legacy behavior follows:

- **Token.** A conversation `Runner.Spawn` started with a parent and a
  recorded tier, whose profile allows an `agent.*` or legacy `pi.*` tool, gets a capability token
  (`WS_MCP_PI_TOKEN`) and the daemon's loopback base URL
  (`WS_MCP_PI_ENDPOINT`, set by `ws-mcp dashboard` through
  `Runner.SetEndpoint` before `Start`) in its attached ws-mcp's environment.
  The token is drawn at every actual process launch and void once that
  process ends or the conversation closes. A holder-backed worker keeps it
  in its record; the successor restores it only after checking the recorded
  holder association, so reattachment does not revoke a surviving worker's
  ability to call delegation tools. A conversation opened through `POST /agents`, or
  spawned with no tier, gets none. Its session key stays a parent-less
  delegate key: the token authorizes delegation tools alone, so
  every other lead-only tool stays refused.
- **Forwarding.** A ws-mcp with no runner and both variables set forwards
  each `pi.*` call (the tools are not listed) to
  `POST /agents/by-token/<op>` (`spawn`, `list`, `send`, `stop`,
  `transcript`, `wait`) with the token as a bearer credential; the keyed
  gate lets a delegate key through for `pi.*` there only. The route answers
  only this machine (404 otherwise) and 401 for a missing, forged, or void
  token, and scopes every op to the holder's own children (404 for any other
  id). `dashboard.pi_spawn_enabled` is checked only at the root, on the
  lead's `pi.spawn`; the Pi version is the daemon's check on every call
  (`Options.ChildCheck`, the version probed once per daemon process). The
  worker's own server checks neither. Forwarded calls rediscover the current
  daemon endpoint. A disconnected `pi.wait` retries within its deadline;
  `spawn`, `send`, and `stop` do not automatically retry, since they may have
  taken effect before the connection broke. A failed call during replacement
  does not by itself mean the Pi process stopped.
- **Cwd.** A worker's empty `cwd` is the worker's own working directory
  (its record's `cwd`, else its root), and a relative one resolves against
  it, so a worker in a worktree spawns into that worktree. Both a lead's
  and a worker's spawn bind the Pi child's delegate key and ws MCP cwd to
  the resolved cwd. The conversation's root stays the caller's root for
  mail and config. A lead's `pi.spawn` keeps resolving cwd against the
  lead's root.
- **Caps.** A spawn records the holder as the child's parent (`parent.kind:
  "conversation"`). The lead is depth 0, its Pi worker depth 1, and that
  worker's child depth 2; the depth cap is 4. `agent.spawn` additionally limits
  width to 99 awake direct children; there is no global concurrent-agent cap.
  The compatibility Pi
  child's tier resolves in the `pi` bucket alone and may not rank above the
  holder's own (`small` < `medium` < `large` < `xlarge`); a holder with no recorded tier
  is refused.
- **Profiles.** An omitted `profile` always means `pi-worker`, even for a
  mini-lead parent. A non-lead parent's child must be a subset of its launched
  resolved `ws_tools`, `tools`, and `mcp_servers` (including each server's
  definition); an excess is refused naming both profiles and the tools.
  Overrides are honored, not ranked by profile name. Each Pi process launch
  persists the exact checked definition on its record, uses that same copy
  to launch, and checks again on restart against its parent's launched copy.
  Widening a parent's config does not widen an already-running parent's
  ceiling. A still-running process reattaches with its saved definition;
  older records without one cannot spawn children while reattached, until an
  actual process launch resolves and snapshots their definition. Lead parents
  have no subset ceiling.
- **Reports.** A child's settled turn waits in its record
  (`pending_reports`) until the runner hands it to the parent. A running
  parent turn gets it as a Pi `steer`. Otherwise it is kept in the parent's
  record (`reports`) and sent as the parent's next turn, which starts at
  once: when the parent is idle or its process stopped, or when the steer
  was refused or the run ended before taking it (`clear_queue` then drops
  it from Pi's queue). A parent blocked in `pi.wait` on that child takes
  the turn as the wait's answer instead, with nothing steered; a wait on a
  turn whose report was already handed over answers with its status alone
  (`delivered: true`, no message). A report to a Claude lead still goes by
  mail.
- **Holds.** A worker's done turn is not reported upstream (no report, no
  wait answer) while a child of its own is open: from a message it was sent
  until its report has reached the worker, or it closes. A failed or
  interrupted turn goes up at once. Pending and kept reports are in the
  records, so a daemon restart delivers them. A holder-backed Pi child's
  running turn survives the restart and reports its eventual result. Only an
  unrecovered open turn (no surviving holder/process, or no holder manager)
  reports interruption, which resumes its parent; it settles before the
  successor sends the input the record kept for the next turn, so that next
  turn stays open.
- **Close.** Closing a conversation closes its open descendants first,
  deepest first; closing a hosted lead also closes the Pi children it
  spawned. A finished child never auto-closes: its process stops after its
  idle timeout and its conversation sleeps, waking on mail. A daemon start
  restores finished children asleep; only an orphan whose conversation parent
  is closed or gone is closed at startup. Waiting children expire after
  `agent.child_waiting_days` (default 15), counted from their last activity.
  A closed Pi child's directory ages out after registry retention, counted
  from its last row write. Compatibility `pi.send` can reopen a closed Pi child
  alone on its existing session file; descendants stay closed until sent to
  themselves. A process stop (the stop route, an idle stop, a settings change)
  does not cascade, and a child's report resumes the stopped parent. A report
  to a closed parent stays in the child's record.

The runner also hosts lead sessions: a headless Claude Code, Pi, agy, or Codex
process that loads the user's own harness setup (no profile allowlist or
plugins of the runner's; a Claude lead gets one appended daemon-written
instruction file with its session key and the `workflow_manual` instruction,
so a machine that blocks some plugin hooks still teaches it), started from
the built-in `lead-claude`, `lead-pi`, `lead-agy`, or `lead-codex` profile.
`POST /agents` refuses a lead profile; a lead is spawned
by `POST /agents/leads {root, harness: <registered lead harness>, model?, effort?,
permission_mode?, plugin_isolation?}` (201 with the state; `root` must be
inside git, since the session key is minted at the canonical root).
`plugin_isolation` accepts `default` (the default, replace ws only) or
`isolated` (bundled plugins only); a Claude lead ignores it. Differences
from a support conversation:

- **Process.** The harness runs under a session holder (`internal/wsholder`,
  pipe mode), not as the daemon's child, so it outlives the daemon. The
  record (`agent.json`) keeps the holder id, the harness PID, and the last
  holder record sequence applied. A starting daemon rejoins each lead's
  holder, replays its ring up to the holder's current sequence, and applies
  only records past the recorded one: no entry is written twice, and a
  permission prompt that waited across the restart waits again. It never
  starts a `--resume` process while the holder's process runs. A lead holder
  no conversation runs under is ended after the first scan. A daemon that
  stops is restarted by the hosted session's own launcher at its ws-mcp
  reconnect (Daemon-Hosted MCP below).
- **Identity.** The daemon mints the session's lead key at spawn and writes
  the registration itself (`hosted: true`, `host_pid`), heartbeats it, and
  end-marks it at close. The harness gets `WS_HOSTED_SESSION`,
  `WS_HOSTED_SESSION_KEY`, and `WS_HOSTED_SESSION_ROW` (Environment below);
  a Claude lead's appended instruction file (`instructions.md`) is a second
  route for the key; the daemon's own `CLAUDE*`, `WS_MAILBOX*`, `WS_MCP_*`, `WS_RSRC*`,
  `WS_HOSTED_*`, and `WS_PI_*` variables are not passed on. A ws-mcp that is
  not the hosted harness's never writes, heartbeats, removes, or end-marks a
  row that carries `hosted: true`, whatever key it is given.
- **Prompt.** Each launch (a resume included) of a Claude, agy, or Codex lead
  appends `<session dir>/instructions.md` to the harness's own system
  prompt (Claude `--append-system-prompt-file`, agy's
  `_meta.antigravity.systemPrompt.append`, Codex `developerInstructions`): the session key, and the
  workflow manual the daemon renders for it as of that launch (the daemon's
  `LeadManual`, the same render as `workflow_manual` on the session's own
  connection). A failed render leaves the key sentence alone and is logged.
  A Pi lead's extension appends its own manual; its bridge fallback appends
  the daemon's render beside the key. A session under the subagent-removal
  policy (a lead or an `agent.spawn` worker) whose backend catalog names a
  native subagent tool for its role also gets a line never to use that
  tool and to delegate through `agent.spawn`/`mailbox.send`
  (`Runner.subagentSteer`); a worker gets it after its key sentence.
- **Playbook commands.** A backend with `PlaybookCommands` (Claude, agy)
  hosts leads with no ws skills: the lead's connection (not a worker's)
  renders under the `ws` server's names whatever package the daemon
  serves, a `{{ns}}:lead-*` skill reference renders as the `/lead-*`
  dashboard command, and delegation renders as `agent.spawn`. Its prompt
  says what a `/lead-*` reference is and, when the manual rendered, keeps
  it as the only copy: for fresh state, and in place of `/lead-revive`, the
  lead calls `workflow_state`. The runner adds the package's
  self-preloading lead skills (`wsrsrc.PreloadCommands`, read once per
  runner, a failed read included, the workflow manual's own left out) to
  the lead's command list whenever its conversation opens or its harness
  reports a list, replacing a harness command of the same name and any
  earlier runner's entries. Running one sends the
  playbook the daemon renders for the lead's key (`RenderLeadPlaybook`, the
  same text as `playbook.read` on its connection) as the turn; a failed
  render sends a pointer to `playbook.read`. The page's user entry stays the
  typed line.
- **Backend descriptor.** What differs per harness here is data on the
  backend's `BackendRegistration`, read through the conversation record's
  `Backend`; common code names no backend. `ManualHarness` is the harness the
  lead's manual renders under (claude, agy, codex; Pi declares none);
  `PlaybookCommands` is described above.
  `CallFallback` opts hosted lead connections into the compact `call` tool
  and a generated callable-tool index appended to their workflow manual.
  Only agy sets it today; ordinary plugin/stdio connections, other hosted
  harnesses, and every worker neither list nor accept `call`. This is a
  boolean for clients that may hide tools for any reason, not a size cap:
  agy drops definitions over 1,500 B including its 39 B description prefix;
  other clients may cap tool count. The independent listed-definition test
  cap is 1,450 B. The index uses the same permission- and role-filtered set
  as `call` list/schema discovery, with advertised names and the first
  sentence of each description. Runtime catalogs omit this conditional
  tool from their default connection surface.
  `ToolNameScheme` is how its connection's `tools/list` spells ws's dotted
  tool names: agy declares `sanitize` (each `.` becomes `_`, as `tickets_query`:
  the Pi bridge's character mapping without its `ws__` prefix, since agy adds
  `mcp_ws_` itself and the Anthropic API refuses dots), every other backend keeps
  dotted names. A call by alias maps back to the canonical name first in
  `callTool`, so allowlists, the session-key rule, and dispatch see dotted
  names; the dotted name stays accepted. The alias table refuses a collision
  when it is built. Workers identify their own record with
  `WS_MCP_AGENT_SESSION_KEY` in the attached MCP environment; this selects
  spelling only, without the hosted lead's implicit-key rule or authority.
  `WorkerCapable` and `WorkerProfile` admit worker tiers; `WorkerHTTPMCP`
  selects HTTP attachment. Claude and agy use HTTP, Pi keeps its bridge.
  agy's `WorkerAutoApprove` answers requests through `AnswerApproval`,
  preserving the permission trace. Its workers use the default agent with
  an appended instruction to use `agent.spawn`, never native subagents;
  see Daemon replacement and process ownership for worker lifetime.
  agy tier models are base ids with separate effort: launch uses
  `thought_level` when offered, otherwise a suffixed model id, and logs the
  mechanism honored. An unhonored worker effort refuses the launch.
- **No ws plugin (Claude).** A Claude lead loads no ws bundle: the installed
  `ws` and `wsflow` plugins are turned off for the launch (`--settings`
  `enabledPlugins`), its ws-mcp is the `--mcp-config` server `ws` (no
  `--strict-mcp-config`, so the user's own servers still load) with an
  `--allowedTools mcp__ws` rule, and a launch with no ws-mcp at all is
  refused. Its ws tools are `mcp__ws__*`, not `mcp__plugin_<pkg>_<pkg>__*`,
  so user permission rules written for the plugin names do not match them.
- **Codex app-server.** A hosted Codex lead uses one sustained `codex
  app-server` under the existing holder, with thread/start or thread/resume
  and thread.cliVersion as the catalog version. It shadows the installed
  plugins' `ws` and `wsflow` MCP servers with required HTTP entries, with
  the bearer token only in the child's environment; skills and the user's
  hooks remain enabled. `agents.enabled=false` removes native subagents;
  delegation is through `agent.*`.
  An `agent.spawn` worker (`agent-worker-codex`, tiers `codex.<tier>`) uses
  the same start with no holder and no lead spec, as agy workers do: the
  runner sets `StartSpec.PermissionLevel` from the child's authority.
  Workdir-write authority runs in the `workspace-write` sandbox with approval
  policy `on-request`, which `WorkerAutoApprove` answers and traces; full
  authority uses the `danger-full-access` sandbox with the same approval
  policy; read-only runs in the native `read-only` sandbox with policy
  `never` on `thread/start` and every `turn/start`, so Codex refuses
  an escalation itself and nothing reaches the auto-allow. Both are
  advisory: the sandbox does not confine a user-config MCP server shell, and
  a worker's sandbox is not changed after spawn.
  Send, steer, interrupt, next-turn model/effort and native sandbox mode
  changes, and `/compact` use app-server controls. Approvals stay binary:
  allow accepts, deny declines, permission grants last one turn; session
  grants, policy amendments, cancel and elicitation persistence are not
  offered. User-question requests get a chat-only error. Output is scoped
  by thread and turn, so late interrupted items cannot finish newer work.
  After a lost holder ring, thread/read recovers status and final output
  before held input is released. Native reply identities in the existing
  conversation-ring `source` member prevent already-persisted replies from
  being appended again; they are read once at reattach, not per message.
  Missing aggregate background telemetry stays unknown, never known zero.
- **Key rule.** On a hosted lead connection, a tool call that omits
  `session_key` (or leaves it blank) runs with the connection's key when the
  tool declares one; an explicit key must be the connection's own or the
  key of one of its descendant sessions (a runner conversation whose
  `Record.Parent` chain reaches the lead), and any other is refused naming
  the rule. A key a `ferrule` call minted under the hosted key has no runner
  record and is refused. Child and non-hosted connections are unchanged.
- **One holder.** A conversation is the daemon's or a terminal harness's,
  never both, and the registration row says which. The adapters report the
  harness conversation at login (`ferrule`'s unadvertised `harness_session`
  and `harness_cwd`, stored on the key record as `harness`,
  `harness_session`, `harness_cwd`; the directory is the harness's own, not
  the record's root), and a terminal ws-mcp writes its harness process on
  its row (`harness_pid`). Neither PID makes a session live; each only
  refuses a change of holder. A terminal re-login (`relogin_session_key`)
  to a key whose row is hosted is refused with the marker
  `hosted-session-held` while the row's process may run: `host_pid` alive,
  or no `host_pid` on a row that is neither `host_stopped` nor ended (a
  spawn under way). The adapter then stays keyless and shows the refusal
  instead of minting. The hosted harness's own login (the hosted flag, or a
  harness PID equal to `host_pid`) is accepted. Any other re-login to a
  hosted row takes it: the row becomes the terminal's (`hosted` cleared,
  `harness_pid` set), and the daemon lets the session go when it next reads
  the row - its record is marked `handed_off_at`, its mail watch and
  heartbeat end, mail it had claimed and not delivered goes back to the
  key's reply queue, and no later daemon start serves it. The daemon reads
  the row under `registration.json.lock` before every row write, mail
  claim, and spawn, and the login takes the same lock, so the two cannot
  both decide they hold the session. An accepted re-login to a row that
  is not hosted marks it at once as the terminal's (`harness_pid`, no
  `ended_at`, a fresh heartbeat), whatever the session's dashboard key
  says, so a row left ended (by an earlier run, or by a hand to the
  terminal) never reads as free for a takeover while that harness runs. A
  terminal ws-mcp whose dashboard key
  turns off leaves such a row in place (its directory holds `agent.json`)
  rather than removing it.
- **Moves.** Three control routes move a lead between the daemon and a
  terminal, each one step under the row lock and each behind the trusted
  gate for every id (`internal/wsagent/handoff.go`):
  - `POST /agents/<id>/takeover` makes the terminal session `<id>` a hosted
    one. The daemon finds its lead key by the row's root and reply-id
    (`AgentKeyStore.LeadSession`), keeps that key (no mint), and resumes
    the conversation its key record names on the next input or mail:
    `--resume <harness_session>` for Claude, `--session <file>` on the
    session's own file (no `--session-dir`) for Pi, in `harness_cwd`, which
    the record keeps (`cwd`, `session_file`) for every later start. It is
    refused (409) while the terminal harness may run: `harness_pid` alive,
    or no `harness_pid` on a row that is not ended. Also refused: a hosted
    or agent row, a harness other than claude and pi, a session with no
    lead key (every plain-folder session), one whose key record names no
    harness conversation, and a directory or Pi session file that is gone.
    A ws-mcp that outlived the session's harness re-reads the row under
    the lock before each of its row writes and stops writing once the row
    is hosted.
  - `POST /agents/<id>/handoff` hands a hosted session to the terminal:
    the process ends, the row becomes an ended terminal row (`hosted`,
    `host_pid`, `harness_pid` cleared, `ended_at` set), the record is
    marked `handed_off_at` as in an implicit hand-off, and the state
    carries `resume_command` (`cd <dir> && claude --resume <id>`, or
    `cd <dir> && pi --session <file>`), which is answered to a viewer with
    `can_control` only. It is refused (409) for a session with no harness
    conversation yet, and fails when the process could not be ended. The
    session's page carries `data-handed` while the row is an ended
    terminal row and the record is handed off, and then reads the command
    from `GET /agents/<id>`.
  - `POST /agents/<id>/reopen` opens a closed hosted session again under
    its key, conversation, and settings. It is refused (409) once the
    session's row is a terminal's, and for a handed-off record; such a
    session comes back through takeover.
- **Input.** Every input is the daemon's. A composer message is held in the
  inbox and submitted at the turn's end; a steered one is written to the
  running process at once (Claude: a user message mid-turn, Pi: RPC
  `steer`). The daemon watches the lead key's mailbox (`LeadMail`, in
  `internal/mcp/lead_mail.go`) and claims what arrives: during a turn it is
  steered into the running process like a composer steer, else it goes into
  the record and is submitted as a turn whose text is what the mailbox wait
  hands a session; the session arms no wait of its own. A turn no send started (a background
  task's result) and a compaction's replayed assistant line are handled as
  the stream reports them. A page command entry splits the drain: the
  messages before it go as one turn (with the claimed mail), the command
  runs alone as the next turn (Claude: its bare typed line; Pi: `/compact`
  as RPC `compact`, any other as a `prompt`, never `steer`), and what was
  drained after it waits in the record (`queued`) for the turns after,
  across a daemon restart; a close or hand-off drops it, an interrupt keeps
  it. The session's command list is Claude's init `slash_commands` (not
  `terminal_slash_commands`) and Pi's `get_commands` (asked at start and
  after each turn) with `compact` added.
- **Turns.** With `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1` the CLI reports
  `session_state_changed`; once one is seen, only `idle` ends a turn and
  releases held sends (a `result` does not: background work may still run).
  Without those lines a `result` ends the turn.
- **Approvals.** Claude runs with `--permission-prompt-tool stdio`. A
  `can_use_tool` request becomes a pending approval in the state
  (`approvals[]`, keyed by request id; several may wait, a subagent's with
  `agent_id`), shown on the page as a card, and is answered by `POST
  /agents/<id>/approvals/<request-id> {behavior: "allow"|"deny", message?}`
  once (a later or unknown answer is 409). `AskUserQuestion`,
  `ExitPlanMode`, and any request that needs user interaction are denied at
  once with a message telling the model to ask in chat. A harness that
  reports `requires_action` with no prompt pending for two seconds shows
  `action_needed` in the state. An answered id is kept in the record, so a
  restart does not ask again.
- **Settings.** `POST /agents/<id>/settings {model?, effort?,
  permission_mode?}` applies in-stream where the harness has a control
  (Claude: `set_model`, `apply_flag_settings` for effort,
  `set_permission_mode`; Pi: `set_model`, `set_thinking_level`) and keeps
  the process; a restart on the same backend session is the last resort.
  The record holds the current values, and every resume launches with them.
  Native permission modes: `auto` (the default), `default`, `acceptEdits`,
  `plan`, `dontAsk`, `bypassPermissions`; a Pi lead has none. The legacy
  spawn and settings URLs reject `permission_level` or
  `permission_full_ack` with 400, including explicit nulls. On a
  common-aware backend a native `permission_mode` change that raises the
  common level is 403 unless it is the local, own-Origin user's request;
  the public `Runner.SetPermissionMode` cannot grant that authority. An
  explicit selection cannot be raised to `full` through the native-only
  URL, which carries no full acknowledgement
  (`internal/wsagent/http.go:202-207,273-284`,
  `internal/wsagent/runner.go:1004-1020`,
  `internal/wsagent/permission.go:214-242`).
- **Common permissions.** `POST /agents/leads/permission {root, harness,
  permission_level, permission_full_ack?, model?, effort?, plugin_isolation?}`
  spawns a lead (201 with its state; `plugin_isolation` has the same choices
  and default as `/agents/leads`); `POST /agents/<id>/settings/permission
  {permission_level, permission_full_ack?, model?, effort?}` changes one
  (200 with its state). Both take the trusted gate, require a level from
  `read-only`, `workdir-write`, `full`, and require
  `permission_full_ack: true` for `full`. Their strict JSON objects reject
  unknown or null fields (including `permission_mode`) with 400; unsupported
  selections and worker selections are also 400. Only the trusted route
  supplies user authority; an acknowledgement alone grants none
  (`internal/wsagent/http.go:94-106,158-177,183-207,249-271,364-398`,
  `internal/wsagent/permission.go:78-96,180-210`). These are lead-only intent,
  not sandbox guarantees or worker capability changes: Claude maps the
  levels to `plan`, `acceptEdits`, `bypassPermissions` respectively, while
  Pi has no native mode mapping. Lead state includes `permission_level`,
  `permission_source` (`explicit` from `permission.json`, else
  `legacy-derived`), and `permission_enforcement`: `partial` only for
  Claude `read-only` while a running harness reports `plan`, `advisory`
  otherwise. Worker state omits these fields. Without an explicit
  selection, Claude `plan` derives `read-only`, `bypassPermissions` derives
  `full`, its other admitted modes derive `workdir-write`, and Pi derives
  `workdir-write` (`internal/wsagent/permission.go:53-75,98-109`).
- **Recovery and failures.** A Claude lead's init line lists its MCP
  servers. A server reported `failed`, a failed `mcp__*` tool call, or the
  lead's call of a `*__request_mcp_reconnect` tool makes the runner ask the
  harness in-stream, off the loop: `mcp_status` for the failed servers
  when none is named, then `mcp_reconnect {serverName}` and `mcp_status`
  for each. A server back as `connected` restarts nothing. One that is not
  is listed in the state's `mcp_broken` (and the snapshot's
  `host.mcpBroken`), and the process restarts on the same backend session
  at the first idle point with no background task (`stop` then a
  `--resume` start). A server an automatic restart was for that fails again
  without having connected restarts nothing more until it connects (no
  restart loop). `POST /agents/<id>/restart` restarts by hand. A typed failure on an
  assistant line (`error`: `authentication_failed`, `model_not_found`,
  `rate_limit`; `api_error: usage_limit_reached`, with the reset time from
  the refusal's or the last rejected `rate_limit_event`'s `resetsAt`) is a
  `notice` ring line and the state's `failure` (snapshot `host.failure`)
  until a turn ends with no failure; an `api_retry` line is the snapshot's
  `host.retry` until output resumes or the turn ends; a compaction that
  failed carries its `compact_error` on the compaction line. The
  background task count (`background_tasks_changed`, subagents included)
  is in the state and `host.backgroundTasks`; a subagent the lead's Agent
  call started takes its effort from an `effort-<level>` subagent type and
  its model and context size from its own assistant lines. Idle stop is
  the profile's `idle_timeout_seconds`: absent defaults to 0 (off) for leads
  and 600 seconds for workers; an explicit 0 disables it. Idle stop waits for
  no running turn or pending input, a zero reported background task count,
  and no active spawned child (including a child's background work or held
  report).
- **Streaming.** A hosted Claude lead starts with
  `--include-partial-messages`. The `stream_event` mapper accumulates text
  deltas into the reply so far, emitting at most once per 100 ms within a
  block; `message_start` and `content_block_start` reset the partial text
  and throttle. The whole assistant text block supplies the final reply.
  Subagent stream events and non-text deltas supply no partial reply.
  The lead's holder uses `--ring-skip` to keep matching tool-input,
  thinking, and signature delta lines out of its ring and client stream.
  It skips only whole stdout lines that fit one record and match from the
  line's start; split or unmatched lines stay. Skipped lines consume no
  sequence number.
- **Access.** Spawn, settings, stop, restart, close, and an approval answer take the
  trusted gate (`wsdashboard.TrustedWrite`): 404 to any viewer that is not a
  browser on this machine, 403 without the daemon's own `Origin`. A LAN
  viewer reads the session and may send messages and interrupt, but not slash
  commands: a command entry, or a message whose text starts with `/`, is 403
  to it; the state's `can_control` tells the page which viewer it serves.
- **Page.** The rail's new-session "+" (right after a worktree row's label,
  quiet until the row is hovered like the other rail "+" buttons; worktree
  rows only, a plain folder's bar has none, since the spawn route takes only
  a Git worktree root) opens the spawn popover, with model and effort choices from
  `GET /support/choices`. A hosted session's page reads `GET /agents/<id>`
  on load, on a head swap, when the live region's permission line changes,
  and when its menu opens (no timer): approval cards, the `action_needed`
  notice, and the harness chip's menu (model, permission mode, effort,
  stop, restart while `mcp_broken` is non-empty, close; stop, restart, and
  close confirm naming `background_tasks` when it is above zero) come from
  that state. From the snapshot's `host` block the page draws an error ring
  on the head's harness chip and the rail row plus a failure chip while
  `failure` is set, an MCP-broken chip, a retry line in the live region,
  and the background task count in the agents popover and beside the
  composer's subagent badge; `notice` ring lines and a failed compaction's
  `text` render as failure lines in the conversation. The rail and index mark a hosted row and show its waiting approval
  from the live snapshot, which names the oldest pending prompt only. The
  local-viewer controls are hidden by CSS unless `<html>` carries
  `data-sysopen`; the routes refuse regardless. A tab asks for notification
  permission on Start or a composer send and raises one notification when a
  session starts waiting (a toast without permission, which is every
  plain-http LAN tab).

A tracker store's support conversation is an ordinary runner conversation
on a support profile whose working root is the store's registry `Dir`; the
daemon keeps one per store under `/support` (`internal/wsagent/support.go`,
mounted when `cmd/ws-mcp/dashboard.go` passes it, with store roots read from
the daemon's own tracker host). Routes, behind the same guard and POST rule
as `/agents`, an unknown store 404: `GET /support/choices` (`{harnesses:
[{harness, profile, available, error?, models: [{id, thinking?}],
efforts, lead?, permission_modes?, permission_levels?}], last}`: optional
`lead` is a bool marking admission as a hosted lead, optional
`permission_modes` is that harness's native modes, and optional
`permission_levels` is its supported common lead spawn/settings selections,
not worker support; these fields belong to each harness entry. Levels are
listed in `read-only`, `workdir-write`, `full` order, restricted to the
registration's supported choices, and omitted when none are supported
(`internal/wsagent/support.go:216-230,422-430,449-450`). Support
profiles' backends resolve through the global and builtin layers, with
duplicate backends listed once; registered lead-capable backends not already
listed are appended in backend-name order, with an empty `profile`.
`BackendRegistration` is the runtime-only descriptor pairing a backend
implementation with its optional `LeadProfile`, `PermissionModes`, and
`PermissionLevels` (`internal/wsagent/registration.go:10-20`); process
controls remain instance-owned and choice listing comes from the
backend's `ChoiceLister` implementation. A harness is unavailable when the
daemon lacks the backend or, for a `ChoiceLister`, its executable is not on
PATH; Claude's models are the CLI's
version-less aliases `opus`, `sonnet`, `haiku`, `fable` and its efforts the
Claude levels, Codex's models and effort choices come from `model/list` on
its last opened app-server process while that report is younger than the
five-minute choices TTL (a newer report replaces it), else from the CLI's
`models_cache.json` (read once per TTL window with no account probe on a
dashboard read: a missing file is an empty list, an unreadable, empty, or
model-less one an error on the 30 s failure TTL), Pi's models come from `pi --offline --no-approve
--list-models` (`provider/model`, cached five minutes) and its efforts from
`--thinking`; a listing failure is that harness's `error` alone), `GET
/support/stores/<store>` (`{store, conversation: State|null, selection}`),
`POST .../send {text}` (opens the conversation with the current selection
first when none is open), `POST .../new` (closes the open one and opens a
fresh one; sends nothing), and `POST .../select {harness?, model?,
effort?}` (`{conversation, selection}`: saves the machine-wide last choice;
a harness change replaces the open conversation with one on that harness's
profile, resetting model and effort unless given; a model or effort change
goes through the runner's settings, from the next message; with no open
conversation it only saves). `<state dir>/support.json` (0600) holds `{last:
{harness, model, effort}, stores: {<store>: <reply-id>}}`, so a store keeps
the same conversation across restarts; with no saved choice, or one whose
harness is unavailable, the selection is the first available harness
(Claude, then Pi) with the profile's defaults.

The tracker gate admits a support conversation's key like a top-level
lead when the daemon holds an open agent record for the key's reply-id that
names this key at the key's root, and that record's profile, resolved at
call time, names the tool. Its writes are pinned to the store whose
canonical registry `Dir` equals that root (any other `store` is refused),
and the store commands (`tk:register`, `tk:sync`, `tk:unregister`,
`tk:relabel`, `tk:hide`, `tk:unhide`) stay refused, since the conversation
runs in its store's clone; every other spawned key is
refused as before. `tracker.show {item}` is that conversation's card call:
an item of its own store answers `card <store> <alias>` plus the mini-card
fields, which the chat draws as a card; another store's item is answered as
text; a lead gets a refusal. `tracker.ui {op, ...}` is its dashboard UI
command: `search {query}`, `focus {item, comment?}`, `scope {scope}`, or
`view {mode}`, checked first (an item of its own store, a comment on that
item's thread, the scope and mode keys of `wslive.UIScopes` /
`UIViewModes`, a query `wstracker.ParseQuery` accepts), answering `ui <op> <arg>`; the runner writes that as a `ui`
ring entry, and the store chat acts on it once, only when it arrives live
after the stream's first (history) entries region, so a reload or reconnect
draws it as an inert line. A narrow-layout search, scope, or view switch
closes the chat's stream, so a later command in that reply is lost; the
prompt and the tool's answer ask for at most one per reply. `git.search` runs `log`, `show`, `grep`,
`blame`, or `ls-files` read-only (a per-subcommand flag allowlist, no git
global options, external diff and textconv off, no pager, a 20 s timeout and
64 KiB of output) in a repository at or under an allowed root: the project
roots of non-agent dashboard registrations seen within the registry
retention, plus `agent.git_search_roots` (a global-only path list that needs
lead authority); without `repo` it lists the roots.

Pages render in one language end to end, chosen by the server from the first
byte: the `ws_dashboard_lang` cookie when it names an available language (the
gear menu's language `<select>` writes it; no cookie, or the empty choice,
follows the browser), else the best `Accept-Language` match, else English. A
language is one table file, `internal/wsi18n/tables/<code>.json`, embedded in
the binary: the file name is the code (matched case-insensitively), its
`lang.name` key is the name the menu shows, and a new file adds the language
with no code change. English is the fallback and the key set: another table
may hold only keys English has, and `internal/wsi18n`'s key sweep fails on a
key a source names that English lacks or an English key nothing uses. Keys
that start with `js.` ship to the page scripts as the page's own table.

The theme works the same way: the gear menu's theme buttons write the
`ws_dashboard_theme` cookie (`light` or `dark`; auto deletes it and follows
the OS), and the server renders the page root's `data-theme` from it on the
first byte, on every page. A topic body's frame is a separate document, so
the daemon pins the same `data-theme` on each frame document's root, on the
session page and its SSE stream; a switch needs no reload: the page sets its
root, posts the theme to every body frame (and again to a frame that reports
its height, which covers a frame swapped in after the switch), and redraws
drawn mermaid diagrams, in the page and in the frames, in the new palette.

By default the daemon binds loopback and every route requires a loopback
Host. Browser-facing routes additionally require the localhost HttpOnly,
SameSite=Strict cookie, derived from `localhost-secret` and kept for up to 400
days. Cookieless navigations show the self-contained login gate; fetches and
WebSocket handshakes answer `401 {"error":"login_required"}`. Explicit gate
copying asks the daemon to place a fresh single-use ten-minute link in the OS
clipboard, then the page reads it once and automatically logs in with the
link. The browser may prompt for clipboard permission; if the read is unavailable
or refused, or returns no login link, the copied hint stays visible for manual
paste. This route accepts only a loopback peer and Host with its own Origin and
same-origin fetch metadata when present; it is rate-limited and returns no
credential. Unavailable native writers hide the
action, while runtime write failures allow retry. Nothing reads or writes the
clipboard on page load. Settings and the desktop Connection menu provide
additional explicit login-link paths. Electron sets its native cookie
before navigation. A same-origin status probe recovers a Strict-cookie browser
that followed a cross-site link, retaining the requested fragment.

LAN mode (`dashboard.lan_enabled`, global-only, passed as `dashboard --lan`)
binds every interface. Local peer plus Host still requires the localhost cookie;
a non-local browser needs the separate LAN token. Neither credential grants
access through the other branch, and a LAN token never grants trusted writes.
The LAN token arrives as `?token=` on the Settings addresses shown only to a
local viewer and becomes its existing HttpOnly cookie. Machine routes retain
their own checks without requiring the browser cookie: `/health`, `/mcp`,
`/shutdown`, `/nudge`, `/session-hire/roster`, and `/agents/by-token/<op>`.
Nudge and roster clients now send the endpoint bearer, but enforcement is deferred
to preserve older clients. GET/HEAD public frame assets, path-capability routes
and `/pwa/*` also remain cookieless. Every other route is gated by default.
`/health` reports the mode as `lan`, and the daemon re-reads the global value
when a session connects: a changed mode replaces it through the drain path below.

At an equal numeric core, ordinary connection keeps the healthy daemon
regardless of version suffix, build time or content hash. `/health` retains
`build_time` and `content_hash` as identity, not automatic swap authority.
`daemon-start --replace` explicitly drains an equal-core daemon once, including
identical content; a newer numeric core stays protected against downgrade.
The existing LAN-mode change still drains an equal-core daemon. In local
development, `python3 scripts/dev.py rerun daemon` owns explicit build+swap
and passes `--replace` with `--build-time` to its newly built staged binary.

A daemon asked to stop (a replacement, or `/shutdown`) drains: it closes its
listener at once, so the successor can take the lock and the port, refuses
new sessions, lets the MCP calls in flight finish for up to the lock wait
minus 1.8 s (`proxyDrainBudget`), cancels long waits (a `mailbox wait`) at
once, then closes its session streams; a git commit or merge in flight is
not cut midway, and the sessions' launchers reconnect to the successor.

Dev assets: with `WS_MCP_DASHBOARD_DEV_SOURCE=<repo>/agents-plugin-tool` in the
environment of `ws-mcp dashboard`, the daemon serves the dashboard and
whiteboard page assets (`internal/wsdashboard/assets`,
`internal/wswhiteboard/assets`: templates, CSS, JS, sprites) from that working
tree instead of its embedded copies, re-reading and re-parsing them on every
request, so an edit shows on a browser reload and only Go changes need a
rebuild. Off when unset; a root without those directories fails the daemon at
startup, and a file that resolves outside the root (a symlink) is refused. The
vendored Mermaid bundle stays embedded. A running daemon keeps its mode, so shut it down first
(`POST /shutdown` above), then start
`WS_MCP_DASHBOARD_DEV_SOURCE=$PWD/agents-plugin-tool ws-mcp dashboard`; a
half-edited template answers 500 (the error is in `dashboard.log`) and the next
request retries.

An isolated probe daemon needs both `WS_CACHE_HOME` (state directory, session
keys, mailboxes) and `WS_CONFIG_HOME` (global config, the stored workspace
list, which sits beside the config file) pointed at scratch directories, and
its own `-port`: with only the cache home moved, the rail's workspace routes
write the real `~/.ws/dashboard/workspaces.json`. Hosted harnesses use the probe daemon's embedded packages and retained restart
record. Keep their cache/config environment isolated too, so their staged
launcher restarts only the probe daemon. External harness/plugin tests retain
their own runtime path; use an explicit scratch bootstrap binary for those.

Profiling: with `WS_MCP_DASHBOARD_PPROF=1` (also `true`/`yes`/`on`) in the
environment of `ws-mcp dashboard`, the daemon serves `net/http/pprof` under
`/debug/pprof/` to loopback viewers only; a LAN-mode request carrying the LAN
token is refused (403), and with the variable unset the routes do not exist
(404). The daemon the launcher starts inherits the launcher's environment
(the daemon-start step runs under it), so the simplest route is the same
restart as dev assets: `POST /shutdown`, then
`WS_MCP_DASHBOARD_PPROF=1 ws-mcp dashboard --port 29417`, then
`go tool pprof -top 'http://127.0.0.1:29417/debug/pprof/profile?seconds=30'`.
`/debug/pprof/streams` answers the live-update counters since the daemon
started as JSON: wakes by source, signals, renders, `nudges`,
`session_reads_changed` and `session_reads_unchanged` (re-reads whose
fingerprint did or did not change), `tracker_reloads`,
`tracker_late_reloads`, and `tracker_late_reloads_changed`, and each parse
cache's `<kind>_cache_hits` and `<kind>_cache_misses` (a miss reads the
file): `snapshot` (the daemon's pages), `source_snapshot` (session
re-reads), `record`, `reply_registry`, and `scope_store`.

The same route includes `file_io_total`, per-operation `file_io_<operation>`,
and `file_io_site:<package>/<file>:<line>:<operation>` counters for instrumented
hot identity, inbox, permission, evidence, snapshot and folder-store paths.
These count attempted OS API operations, including failures and retries, not
estimated costs or every kernel syscall. `read_file` includes its internal
open/read; `create_temp` includes temp creation, and `atomic_replace` counts
actual replacement attempts. Third-party flock and directory-entry `Info`
operations are not included. Counts are process-lifetime and never reset by
reading the route; compare two samples over the same observation interval.

### Daemon replacement and process ownership

For a graceful replace, the process owner determines whether an in-flight turn
continues (`internal/wsagent/runner_events.go` L804-818).

| Child kind | Owner | Survives graceful replace? | What to do |
|---|---|---|---|
| Hosted Claude, Pi, agy, or Codex lead | Session holder (`internal/wsagent/runner_process.go` L229-258, L294-312) | Yes, while holder and child survive | Replace; check rejoin if progress stalls. |
| Claude, Codex, agy, or Pi `agent.spawn` worker | Session holder (`internal/wsagent/runner_process.go`, `startWorker`) | Yes, while holder and child survive | Replace; check rejoin if progress stalls. |
| Legacy direct worker still running under an older daemon | Daemon subprocess | No; shutdown stops it | Send a new message after replace; its next launch uses a holder. |

Healthy held leads and workers can continue through a normal daemon replacement.
A successor validates the worker holder's kind, backend, reply id, cwd,
capability hash, and harness PID before restoring its delegation capability
and HTTP-MCP tuple. The tuple keeps the original token, URL, environment, and
holder binding: an already-running worker cannot learn a rotated token.
Attach uses the child's persisted authority; current-parent admission is checked
after records load and governs its next launch. Replay rebuilds turn state,
deduplicates answered controls, and auto-allows remaining requests for backends
whose worker policy enables it.

New workers use the `agent-worker` holder kind and a `handed_off_at` fence.
An older daemon sees that child as ended, starts no worker, leaves its record
unchanged, and does not recognize its holder kind. This surviving-child-row
behavior is T1. The complete downgrade surface follows owner decision (1855)=a:
T2 with the explicit exception that a downgrade beyond the 7-day
`RegistryRetention` may irrecoverably delete the child's session history.
An old daemon may also release and rewrite a held conversation parent's turn.
Live legacy `pi-worker` holders retain their unfenced shape and remain adoptable.

On re-upgrade, a fenced open child of a settled conversation parent is silently
retired only when its nonzero start strictly predates the parent's terminal
settlement stamp, the parent's sends do not exceed its last turn's mark, and
its held mail and queued input are empty. Invalid, equal, or same-second
stamps adopt the child; the parent timestamp has second precision. Retirement
clears the active turn and holder fence in one record update and leaves a
wakeable sleeping child, preserving its last result and sending no second
parent report. A lead parent instead restores its held turn, adopts the child,
and receives one late report. An old-pruned child leaves an unowned holder for
the successor's orphan sweep.

Interrupt remains turn-scoped. Idle timeout, explicit stop, close, and expiry
end the holder and revoke its tokens. A legacy direct worker's open turn
settles `interrupted` once before queued input starts its next held process.
A lost 32 MiB ring uses existing backend recovery; a backend that cannot
reconcile the lost turn reports failure honestly. Re-arming agy's model-wait
watchdog across replacement remains outside this contract, matching held leads.
MCP calls during drain may receive 503. Windows survival requires a
restart-safe holder; one still inside its starter's job object may end with it.

Session holders: a holder is a small Python process (standard library only,
3.10 or newer) that owns one child process, over pipes or a PTY (ConPTY on
Windows), and outlives the daemon, so the child survives a daemon restart
or replace while the holder and child remain alive (Windows holders marked
not restart-safe have no such guarantee). Dashboard terminals also use holders;
see Daemon replacement and process ownership above for lead and worker ownership.
The script is embedded in `ws-mcp` (`internal/wsholder/holder.py`, whose
module docstring is the descriptor and protocol reference) and staged to
`<dashboard dir>/holders/bin/holder-<content hash>.py` before a holder
starts; a running holder keeps the file it started from, and at start the
daemon removes staged scripts that no live holder's descriptor names. Each
holder has its own directory `<dashboard dir>/holders/<holder id>/` with
`descriptor.json` (owner-only; PID, loopback port, attach token, protocol
version, start time (wall clock, plus boot-clock ticks on Linux), script
hash, restart-safe flag, and the consumer's
session description) and `holder.log`. The holder rewrites a deleted
descriptor within a minute, removes its directory when it exits, and exits
on its own 24 hours after the last detach once its child has exited, never
while the child lives.

The daemon scans the holders directory at start and every 3 minutes,
attaches to every holder over loopback TCP with the descriptor's token, and
replays the holder's output ring (sequenced; 4 MiB for a terminal holder,
32 MiB for a hosted lead or worker's holder, `leadRingBytes`, so a rejoin
rebuilds a long turn whole; a reattach within one
daemon process continues after the last sequence it saw, a new daemon
process replays the whole ring). The newest authenticated attach takes over,
so a replaced daemon and its successor never both write to one child. A
descriptor whose process is gone is removed with its directory, and so is
one whose PID now belongs to another process (the endpoint does not answer
and the process under the PID did not start when the descriptor says). A
live holder stays unattached for one of three causes: its endpoint does not
answer, its protocol is neither the daemon's version nor the one before it,
or it refuses the token. `GET /holders` lists every holder with its state
and cause, the recent holder start failures, and whether the viewer may
stop one; the drawer's holders card shows the unattached holders, the ones
marked not restart-safe (a Windows holder that could not leave its
starter's job object), and the start failures, with a stop button on each
unattached one. `POST /holders/<id>/stop`
ends a holder and its child: through the protocol when attached, otherwise
by killing the holder's process group (POSIX) or process tree (Windows),
and only while the process under the PID is still the one the descriptor
records, which guards against a reused PID (409 with the reason otherwise):
on Linux the process's start in boot-clock ticks must equal the recorded
`start_ticks`, because the creation time Linux reports is derived from the
boot clock and moves with every wall-clock step; elsewhere the OS creation
time must be within 2 seconds of the recorded wall-clock start. The stop route answers only a browser on this
machine that sends the daemon's own Origin; a LAN viewer sees the card
without the stop button. Closing the daemon detaches and ends no holder.

The interpreter is `WS_MCP_HOLDER_PYTHON`, which the launcher sets to its
own interpreter when it starts the daemon (the detached daemon inherits
it); without it the daemon uses `python3` on PATH, and a holder that cannot
start is reported on the card. Holder tests: `go test ./internal/wsholder/`
runs real holders (skipped without Python 3.10 or newer on PATH), and
`python3 -m unittest discover -s agents-plugin-tool/internal/wsholder/tests`
runs the script's own tests.

## Plugin-Managed Startup

Codex plugin bundles can reference plugin-local MCP configuration:

```json
{
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}
```

The plugin-local `.mcp.json` starts the runtime through the Python launcher:

```json
{
  "mcpServers": {
    "ws": {
      "command": "python3",
      "cwd": ".",
      "args": ["./bin/ws-mcp-launcher.py", "serve", "--stdio"],
      "startup_timeout_sec": 30,
      "tool_timeout_sec": 600
    }
  }
}
```

`cwd: "."` is intentional. Codex normalizes it to the installed plugin cache so
`./bin/ws-mcp-launcher.py` resolves inside the plugin package. Because that also
makes the MCP process cwd the plugin cache, tools must not infer the project
root from process cwd.

The launcher exports `WS_MCP_PROJECT_ROOT` when it can infer the caller project
root from the parent Codex process. Skills may still pass explicit absolute
roots for compatibility, and lead actor setup should pass the repository's
absolute filesystem path.

Native Windows plugin-managed startup uses the same Python launcher. Windows
users need a working `python3` command on `PATH`; if the Windows Store alias is
present without Python installed, install Python 3 and refresh the plugin.

## Daemon-Hosted MCP

Every product command uses the Python launcher. `serve --stdio` copies MCP
bytes to the cache root's dashboard daemon; other commands run in-process
handlers there through the authenticated CLI transport. CLI commands, including
mailbox stream and one-shot waits, have network-only authority on initial
connect and retry: they never repair, stage or start a runtime. An unavailable
daemon yields an explicit error/backoff. `version` can read
live health or content-bound build metadata locally. The only ws executable
launch is a needed daemon start/replacement, never a probe or fallback.
`WS_MCP_LAUNCHER_DIRECT` is obsolete and cannot restore an executable bypass.

Finding and starting the daemon (`DaemonLink` in the launcher):

1. The daemon directory is `<cache root>/dashboard`, the cache root being
   `WS_CACHE_HOME` (absolute, symlinks resolved when it exists) or
   `~/.cache/ws@kang-sw-devenv`, as `wsstate.CacheRoot` resolves it; the test
   `test_cache_root_and_staged_name_follow_the_go_rules` pins the parity.
2. `endpoint.json` names the port and token; `GET /health` answers the
   daemon's version, `build_time`, and `content_hash` (the sha256 of the
   daemon's executable). For an unreachable endpoint, the launcher probes
   Go's `dashboard.lock` non-blockingly to prove absence; it does not consult
   the recorded PID. Only a held or unqueryable lock gets the 20-second swap
   grace. A cold cache or a stale endpoint with a free lock starts without
   that wait. After starter coordination and a health recheck, a lock still
   held or unqueryable refuses absence-based startup; a connected port that
   never answers is diagnosed as hung. TCP handshake failure alone establishes
   only unreachability. A missing health route or version field falls back to
   the existing `/mcp` hello's
   returned version, not to executable inspection.
3. Ordinary startup starts an absent daemon or replaces an older numeric
   core using the selected runtime. Equal cores keep the healthy daemon,
   regardless of suffix, timestamp or hash (`keepRunning` in Go,
   `needs_start` in Python). Explicit `daemon-start --replace` bypasses the
   launcher's lifecycle keep shortcut and requests one equal-core swap;
   Go consumes that request before checking the successor.
   The launcher copies the runtime binary to
   `<daemon dir>/bin/ws-mcp-dashboard-<sha256[:16]><ext>` (the name
   `StageExecutable` produces) and runs `<staged> daemon-start --build-time
   <mtime ms>` with its own environment: Go resolves `dashboard.port` and
   `dashboard.lan_enabled` from config as `serve` did and runs the `Ensure`
   handoff, printing one JSON line (`outcome`, `port`, `pid`, `version`).
   A starter the dashboard does not apply to (no dashboard manifest, or
   `WS_MCP_NO_DASHBOARD=1`) does not decide the LAN mode, the same rule as
   for a hosted session below: a running daemon keeps its mode, and with
   none running the global file's `dashboard.lan_enabled` decides.
   Updated launchers first serialize on `launcher-start.lock`, recheck health,
   and hold that lock through repair, staging and a healthy result or bounded
   failure. Losers poll the network within the 75-second budget. This is
   separate from the Go daemon-held singleton lock. Old launchers and Electron
   do not participate; the singleton handles a rare losing Electron starter.
   Staging reuses matching hashes and failed copies clean only their own temp
   file. Settled artifacts remain: neither age nor singleton ownership proves
   process exit or accepted/rollback safety, and no retention index exists.
   The launcher then waits up to
   `DefaultStartWait` + 5 s (Windows
   + 10 s) for a healthy daemon and applies the replace rule to the one
   that answers: when that is still the older daemon (it refused to stop,
   or won the lock race), the adapter diagnoses the failed replacement and
   retries; no second server is launched.
4. Hosted stdio launches set `WS_MCP_HOSTED_PACKAGE=1`: they neither repair nor
   download a plugin runtime and do not wait for a local rsrc tree. Healthy
   daemons are followed without package-contract-driven replacement. After
   absence is proven and starter coordination is acquired, the launcher reads
   `last-running-executable.json`, validates its retained path/content hash,
   and starts that build. Each later restart rereads the record, so a launcher
   surviving replacement never pins its original build. A missing/corrupt record
   refuses restart rather than falling back to an installed plugin. Native Pi
   leads carry the same marker for their extension's launcher. External plugin
   launchers keep their own runtime/repair path.
5. The current hello rejects another minor. A newer same-minor daemon is
   used as is, with no mandatory local binary or metadata upgrade. The
   mixed-version guards ticket separately owns cross-minor acceptance,
   notices, schema guards, and tool-list refresh.

A hosted session's global `dashboard.lan_enabled` can switch the daemon's
LAN mode (the daemon respawns itself with the other setting), but only a
session that takes part in the dashboard has a say: a hello whose server
declares no dashboard manifest, sets `WS_MCP_NO_DASHBOARD=1`, or reads a
global config other than the daemon's (a different `WS_CONFIG_HOME`) never
changes the mode, so two harnesses with different config homes cannot flip
the daemon back and forth.

Drain: a daemon being replaced drains its hosted sessions within
`DefaultLockWait` − 1.8 s, then releases its lock and endpoint so the
successor proceeds, and only then waits for the calls still running (up to
two minutes, `hostedCallLinger`) before the process exits: a `git.commit`,
`git.merge`, or `tickets.close` in flight finishes and its answer is
delivered on the old connection before it closes. A draining server refuses
a new request with `-32000` "server draining", and the drain cancels a
blocked `exec.result` wait at once, which answers with the job state it
finds. A CLI-mode `mailbox wait` is cancelled by the drain (exit 130).

Compatibility and version inspection execute zero ws binaries: the live
identity comes from `/health` or the existing hello; staged identity comes
from a SHA-256-bound `<binary>.metadata.json`. A missing stamp requests
verified release repair or a source rebuild, never a temporary MCP server or
an executable capabilities/command/version probe. Inspection alone never
starts a daemon.

Unavailability is explicit: stderr includes "dashboard daemon not reachable"
and the underlying cause. MCP requests receive `-32000` with their original
IDs when a sent request's outcome is unknown; no tool side effect is replayed.
Requests not yet sent wait up to 75 s for the daemon, re-reading its endpoint
about every 3 s; exhaustion adds `(waited 75s for the dashboard daemon)` to
the existing error. This includes MCP `initialize`, `tools/list`, and
`tools/call`, and one-shot CLI forwarding before its command-carrying hello
is sent. A failure after sending that hello or an MCP request is not retried.
Codex Stop retains its fail-open exit 0.

Reconnect: when the connection ends while the client is still attached (the
daemon was replaced or died), the session MCP proxy connects again (giving a
successor the swap grace only when the unreachable endpoint's singleton lock
is held or unqueryable, before coordinated absence startup), re-sends the
client's captured `initialize` request under an id of its own
(`ws-mcp-launcher-replay-<n>`, so a late answer from the old daemon can never
be mistaken for it) and drops its
answer, re-sends `notifications/initialized`, and answers every request that
was in flight with a JSON-RPC error (`-32000`, "ws-mcp daemon connection was
replaced"); the client never sees the server go away. Reconnecting is
paced: a connection the daemon closes within 2 s counts as a quick close;
retries wait at least 3 s, rising by 0.5 s × the count to a 5 s cap. Three
quick closes, failed starts, refused hellos, and reconnect failures never
switch to a child server. In-flight calls fail before waiting for recovery;
calls not yet sent wait within the 75 s budget instead. The stdio proxy stays
alive even after a `tools/list` wait expires, so later requests can use the
successor. Pi's runner-generated ws MCP registration allows 1860 s per request
to cover a maximum forwarded Pi wait with transport margin; it also exceeds
the launcher's 75 s recovery budget, which Pi's default 60 s timeout would
cut off. A hosted server for a reconnected session re-registers that session's
mailbox presence on its first keyed call and keeps the owner pointer and the
`WS_MAILBOX_AUTO` stem its predecessor published, provided
that record is still live (its PID alive and `last_seen` within the ten-minute
liveness threshold); a stale record under a reused PID is replaced whole, and
among several live auto records the first name in order is the one kept.

Client EOF: when the client closes the proxy's stdin, the proxy half-closes
the daemon stream for writing and keeps copying answers to its stdout until
the daemon closes the stream or 10 s pass (`PROXY_DRAIN_SECONDS`), so a
response to the client's last request is not dropped.

Wire format (`internal/wsdashboard/proxy.go`): `GET /mcp HTTP/1.1` with
`Upgrade: ws-mcp`, `Connection: Upgrade`, and `Authorization: Bearer <token
from endpoint.json>`, from a loopback peer (also in LAN mode) with no
`Origin` header; the daemon answers `101 Switching Protocols` (401 for a
missing or wrong token, 403 for a non-loopback peer or a browser Origin, 426
without the upgrade headers) and the connection becomes a raw
newline-delimited stream with no framing. The launcher's first line is the
hello, one JSON object:

```json
{"protocol": 1, "mode": "mcp" | "cli", "argv": ["serve", "--stdio"],
 "cwd": "<launcher cwd>", "project_root": "<WS_MCP_PROJECT_ROOT or empty>",
 "env": {"<every variable of the launcher's environment>": "..."},
 "harness": "", "owner_pid": <launcher pid>, "version": "<runtime.json plugin_version>"}
```

Path-valued ws variables in the hello (`WS_CACHE_HOME`, `WS_CONFIG_HOME`,
`WS_RSRC_ROOT`, `WS_SKILLS_ROOT`, `WS_MCP_PROJECT_ROOT`, `WS_WORKTREE_DIR`,
`WS_MCP_DEBUG_LOG`, `WS_MCP_RUNTIME_BINARY`, `WS_MCP_RUNTIME_DIR`, and each
entry of `WS_MCP_CONFIG_MANIFESTS`) are made absolute against the launcher's
cwd before sending: a harness may set them relative (Codex's `.mcp.json`
names `./config-manifest.json` with cwd `.`), and the daemon would resolve
them against its own cwd.

The daemon answers one line, `{"ok": true, "version": "<daemon version>",
"pid": <daemon pid>}` or `{"ok": false, "reason": "..."}` (a protocol other
than 1, an unknown mode, an empty `cwd`, a missing `env` or `owner_pid`, a
version outside the daemon's minor, or a malformed CLI hello), closing the connection after a refusal. After `ok` the stream is the MCP session in `mcp` mode: the daemon
builds one `mcp.Server` from the hello (`NewServerWithOptions`: the hello's
environment is the server's whole environment and that of every subprocess
its tools start, `owner_pid` is its PID for mailbox presence and listening
markers, the debug event ring and `WS_MCP_DEBUG_LOG` are its own, while
resource and workflow-skill/template reads select the daemon's embedded package
by `WS_MCP_NAMESPACE` (`wsflow`, otherwise `ws`). Hello resource paths and
hello `WS_RSRC_DEV_CHECKOUT` do not redirect those reads. Startup diagnostics such as a rejected
config manifest go to the daemon log and the connection's debug events). The
harness is detected from `initialize` as on stdio; the hello's `harness` is
an optional hint. Closing the connection ends the session: its dashboard
session, its mailbox presence, and any wait it holds.

HTTP route (`internal/wsdashboard/proxy_http.go`): a session the agent runner
hosts reaches the same server with no launcher, by MCP Streamable HTTP with
JSON responses only. It exists only when the daemon hosts MCP sessions and its
agent runner implements `AgentMCPTokenHost`. `POST /mcp/session` carries one
JSON-RPC message with `Authorization: Bearer <per-session token>`, from a
loopback peer with no `Origin` (403 otherwise); the token is resolved on every
request (401 for a missing, unknown, or void one) to the hello a launcher would
have sent, and other methods (a client's SSE `GET`, its exit `DELETE`) get 405.
No `Mcp-Session-Id` is issued: the token is the session, one per token, built
by the same `acceptProxy` and counted, drained, and closed with the stdio
sessions; voiding the token (re-issue, process exit, close) cancels its calls
in flight and ends it. A request
is answered in its POST's body once the session writes the answer with that
id; a notification or client response gets 202; a POST whose client goes away
sends `notifications/cancelled` for its id; a session end answers `-32000`
"ws-mcp session ended". A token's first message that is not `initialize` (its
client initialized against a replaced daemon) is preceded by a replayed
`initialize` (id `ws-mcp-http-replay-<n>`, `clientInfo` mapped from the
hello's harness, answer dropped) and `notifications/initialized`. A body is
limited to 65535 bytes (413), must be one JSON object (400: `-32700` invalid
JSON, `-32600` batch), and is compacted to one line; a duplicate in-flight id
gets `-32600`. A refused hello answers 403 and a draining daemon 503, each
with a JSON-RPC error body.

Who uses it (`internal/wsagent/mcp_session.go`): a hosted Claude process the
runner starts while it knows its daemon endpoint. Every start draws a new
token; the harness gets the URL and token in `WS_MCP_SESSION_URL` and
`WS_MCP_SESSION_TOKEN`, never in argv or a file. A worker's and a lead's
inline `--mcp-config` entry `ws` is the HTTP server; a lead loads no ws
bundle, so it has exactly one ws server, named `ws`. (A lead an older daemon
started ran a hosted variant of its bundle under `<daemon
dir>/hosted-bundles/`, whose server is `plugin:<package>:<package>`; the lead
result mapper still accepts those names for a rejoined process.)
A held lead's token is kept in its `agent.json` (`mcp_token_secret`) with its
URL (`mcp_url`), the ws-mcp environment its hello was built from (`mcp_env`:
the package its ws-mcp was resolved from) and
the holder it was drawn for (`mcp_holder_id`), so a daemon that rejoins the
process restores the token and its hello without resolving the package again.
When the recorded URL is not the rejoining daemon's (it listens on another
port) or the hello is missing, the process restarts at its first idle point
with a new token; when the record names another holder (a daemon without
this route started the process again over stdio), the token is forgotten and
nothing restarts. With no endpoint the `ws` entry is the bundled stdio
launcher instead. A daemon without this route (an older
one rejoining an HTTP lead) answers its calls `401 login_required` until the
process restarts.

In `cli` mode the daemon dispatches the existing command argv in-process.
An optional `stdin` string carries Codex Stop JSON or `git commit
--description-file -` input; older hellos omit it. A relative `--root`
resolves against the hello's `cwd`; stores, manifests, tool restrictions,
subprocess environment and owner identity belong to that hello, never another
package's daemon-wide values. The daemon answers in frames, one
JSON object per line: `{"stream": "stderr", "data": "..."}` for each stderr
write as it happens, then one `{"final": true, "exit_code": <n>, "data":
"<the whole stdout>"}`, `exit_code` always present (0 included). An explicitly
streaming command (`mailbox wait --stream`) instead sends live stdout in
`{"stream": "stdout", "data": "..."}` frames before its final frame; ordinary
one-shot stdout remains buffered. The
launcher writes them as UTF-8 bytes to its own stderr and stdout and exits
with `exit_code`, so the mod and the Pi waiter see the same output and codes
as before; closing the connection (the launcher exits, or the
daemon drains) cancels the wait at once and clears its listening marker
(`exit_code` 130, "interrupted"). A hosted wait rechecks on an in-process
signal (a mailbox send by a server the daemon hosts, a board-answer inbox
write, or a nudge) with a five-minute lost-notification backstop poll.
The daemon's undelivered-mail maintenance likewise discovers queue stores
at startup and every five minutes, visiting changed stores and pending
notice deadlines between scans; presence-only heartbeats do not schedule
maintenance. Failed notice delivery retains its 30-second retry cadence.

`mailbox wait --stream` emits one JSON line per arrival without draining and
keeps its listening marker until interruption, client disconnect, or parent
death. `--timeout` becomes the unchanged-pending-arrival reminder interval
(default 10 minutes), not a process deadline; an empty timeout emits nothing.
The Claude mod and Pi reuse that child across wakes and restart it when board
flags change. A rejected stream flag (usage/unknown-flag diagnostics, or a
nonzero exit before any arrival) selects their existing one-shot loop for the
remaining session. Model-armed waits keep the one-shot contract.

## Launcher Environment

Current launcher inputs:

| Variable | Purpose |
|----------|---------|
| `WS_MCP_RUNTIME_DIR` | Override the runtime binary directory; defaults to plugin-local `.runtime/<os>-<arch>`. |
| `WS_MCP_RUNTIME_BINARY` | Exact repaired runtime binary path exported by an external plugin launcher for async worker recovery. Hosted launchers choose the retained executable only at necessary daemon restart. |
| `WS_MCP_HOSTED_PACKAGE` | Internal hosted-launch marker (`1`): the staged launcher has no plugin runtime and restarts from the daemon's last-running-executable record. Cleared from the daemon starter's environment. |
| `WS_MCP_BOOTSTRAP_BINARY` | Copy a prebuilt local binary into the runtime directory. |
| `WS_MCP_BOOTSTRAP_URL` | Download a prebuilt binary when no runtime binary exists. |
| `WS_MCP_BOOTSTRAP_SHA256` | Optional SHA-256 checksum for `WS_MCP_BOOTSTRAP_URL`. |
| `WS_MCP_RELEASE_REPOSITORY` | Override the GitHub release repository from `runtime.json`. |
| `WS_MCP_RELEASE_TAG` | Override the release tag from `runtime.json`, for example `v0.50.8`. |
| `WS_MCP_RELEASE_BASE_URL` | Override the full release asset base URL; useful for local file or HTTP smoke tests. |
| `WS_MCP_LAUNCHER_DEBUG` | Print launcher diagnostics to stderr when set to `1`. |
| `WS_CODESIGN_IDENTITY` | macOS local dev builds only: code-signing identity name or SHA-1 passed to `codesign --force -s "$WS_CODESIGN_IDENTITY" -i dev.ws.mcp <binary>` before SHA-256 metadata generation; unset leaves builds untouched, and a signing failure aborts the build. |
| `WS_MCP_LAUNCHER_DIRECT` | Obsolete; ignored. It never enables executable fallback. |
| `WS_MCP_PROJECT_ROOT` | Project root default for root-aware tools and CLI commands when no higher-priority root exists. |
| `WS_RSRC_DEV_CHECKOUT` | Explicit daemon-process-only repository checkout for package inputs (rsrc, workflow skills/templates, manifests and native Pi). Uses the package's canonical directory, logs the override, and permits live playbook edits. Never honored from a connection hello. Production embeds always win over `WS_RSRC_ROOT`. |
| `WS_MCP_DASHBOARD_DEV_SOURCE` | Read by `ws-mcp dashboard`: the `agents-plugin-tool` source root the daemon serves dashboard and whiteboard page assets from instead of the embedded copies (see Dashboard daemon, Dev assets). Unset is off. |
| `WS_MCP_DASHBOARD_PPROF` | Read by `ws-mcp dashboard`: `1`/`true`/`yes`/`on` serves `net/http/pprof` under `/debug/pprof/` to loopback viewers only, never to a LAN-token request (see Dashboard daemon, Profiling). Unset is off: the routes do not exist. |
| `WS_MCP_HOLDER_PYTHON` | Read by `ws-mcp dashboard`: the Python interpreter that runs session holders (see Dashboard daemon, Session holders). The launcher sets it to its own interpreter on the daemon-start step unless it is already set; unset, the daemon uses `python3` on PATH. |
| `WS_MCP_NO_AGENT` | Product-mode gate for agentless distributions such as wsflow. |
| `WS_MCP_NO_DASHBOARD` | `1`/`true`/`yes`/`on` turns the dashboard off for this ws-mcp whatever `dashboard.enabled` resolves: no session registration, no prune (the daemon itself is started by the launcher, never by this ws-mcp). The agent runner sets it on the ws-mcp it attaches to a conversation. |
| `WS_HOSTED_SESSION` | `1`/`true`/`yes`/`on` marks a session the dashboard daemon hosts; the daemon sets it, with `WS_HOSTED_SESSION_KEY` and `WS_HOSTED_SESSION_ROW`, on the harness it spawns. Every process below that harness inherits the three, so each reader (this ws-mcp, the Claude mod, the Pi extension) honours them only when the row names its own harness process (see `WS_HOSTED_SESSION_ROW`), resolves that once, and never becomes hosted later. Hosted, this ws-mcp neither registers, heartbeats, nor prunes (the daemon writes the session's registration) but still nudges the daemon and names the daemon's page in `whiteboard.update`; `ferrule` returns the hosted key for a parent-less lead bootstrap and refuses a re-login with any other key; every tool call that omits `session_key` runs with the hosted key, and one that passes a key other than the hosted key or a descendant session's (Key rule above) is refused, `workflow_manual` included; child mints are unchanged. Set without a key, it changes nothing here. Unset is off. |
| `WS_HOSTED_SESSION_KEY` | The parent-less lead key the daemon minted for the session it hosts, read only while `WS_HOSTED_SESSION` is on; the session's key stays fixed to it. |
| `WS_HOSTED_SESSION_ROW` | Absolute path of the hosted session's `registration.json`. Its `host_pid` is the one harness process the flag holds for: equal to the reader's harness PID, hosted; another nonzero PID, an ordinary session, silently (a nested harness, a CLI call from the session's shell); 0, absent, or unreadable, read again about every 100 ms for about 3 s, then an ordinary session with one warning. Unset (an older daemon, a stray flag) is not hosted. The harness PID is `WS_MCP_HARNESS_PID` for ws-mcp (with a Windows `cmd.exe` wrapper skipped, see that row), the mod's own process for Claude (read as the parent of a host command; on Windows, when that parent is the `cmd.exe` shell the host command ran through, that shell's parent, by the same one-level rule), and the extension's process for Pi. |
| `WS_MCP_HARNESS_PID` | Set by the launcher on every start to its own parent PID, over any inherited value: the harness that started this ws-mcp. The runtime reads it as that harness's PID, except on Windows when that process is a `cmd.exe` (the wrapper a harness commonly starts a stdio server through): then its parent. Exactly one level is skipped, so a harness nested under another still tells itself from the outer one, and a failed process lookup keeps the PID as set. It reaches the runtime on both paths (the direct child's environment, and the hello's `env` for a daemon-hosted connection, so it is per caller). A ws-mcp started without the launcher has none and is never hosted; `ws-mcp dashboard` drops it, with the three `WS_HOSTED_SESSION*` variables, from its own environment at start, so the conversations it spawns inherit no session's. |
| `WS_MCP_ALLOWED_TOOLS` | Comma list of ws tool names; when set, `tools/list` advertises and `tools/call` accepts only these. A hosted lead whose descriptor opts into `CallFallback` also gets `call`; naming `call` in an allowlist alone never enables it. Its inner tool still obeys this allowlist and the normal dispatch gates. Fallback discovery applies permission and role gates using its optional `session_key` (implicit on hosted connections); without a known key it hides role-gated tools. The agent runner sets the allowlist to a profile's ws tools. `tracker.show`, `tracker.ui`, and `git.search` are listed and callable only when it names them. |
| `WS_MCP_PI_TOKEN` | Set by the agent runner on the ws-mcp of a spawned conversation that may spawn Pi subagents: its capability token for the daemon's `POST /agents/by-token/<op>` route, drawn at each process start and void when that process ends. With `WS_MCP_PI_ENDPOINT` and no runner of its own, this ws-mcp forwards `agent.*` and legacy `pi.*` to that route (see Dashboard daemon, the Pi spawn tree). Unset, `pi.*` is lead-only. |
| `WS_MCP_PI_ENDPOINT` | Set beside `WS_MCP_PI_TOKEN`: the dashboard daemon's loopback base URL, for example `http://127.0.0.1:29417`. |
| `WS_MCP_NAMESPACE` | MCP namespace and embedded package identity: `wsflow` selects wsflow assets; empty/unset or a custom display namespace selects ws. |
| `WS_MCP_SETUP_TOOL` | Advertised setup tool name override; empty or unset defaults to `ws.setup`. |
| `WS_MCP_CONFIG_MANIFESTS` | Adapter config key manifests, separated by the platform path-list separator (`:` POSIX, `;` Windows); `;` is accepted on every platform, which the static launcher configs use. Declared keys become `config.tune`-writable, cataloged by `config.list`, and typed in `config.get`; a rejected manifest is logged to stderr and listed by `config.list`. The ws and wsflow packages each list three package-local manifests in both launch configs: `config-manifest.json` (namespace `claude.`; the same keys, differing only in `default`, pinned by `TestShippedClaudeManifestsDifferOnlyInDefaults`), `agy-config-manifest.json` (namespace `agy.`; `agy.model_wait_timeout_seconds`, default 120 in ws and 0 in wsflow, plus `agy.model_wait_retries`, default 3 in both), and `dashboard-config-manifest.json` (namespace `dashboard.`: `dashboard.enabled`, default on in every package; `dashboard.port`, default 29417; `dashboard.lan_enabled`, global-only and default off in every package; `dashboard.pi_spawn_enabled`, which gates legacy `pi.*` calls, default on in ws and off in wsflow and Pi). Codex paths are relative to `cwd: "."` (the plugin root); Claude paths use `${CLAUDE_PLUGIN_ROOT}`. Pi lists its own config and dashboard manifests, joined with the platform separator. A path that does not resolve fails nothing: the keys stay undeclared, so check `config.list` after changing the list. |
| `WS_MCP_CLI_NAME` | Declared `ws-cli` / `wsflow-cli` shim or Python launcher command used in printed mailbox wait and re-arm guidance when it matches the supported safe form. The launcher sets it unless already set; an absent or unrecognized value uses an illustrative Python launcher path, never the runtime binary. |

Launcher diagnostics must go to stderr. Stdout belongs to the MCP JSON-RPC
stream from the moment the launcher connects to the daemon.

## Runtime Repair And Drift

The launcher reads plugin-local `runtime.json` and connects to a compatible
running daemon without requiring a local binary upgrade. When a start or
replacement is needed, it repairs a missing/incompatible runtime from
hash-bound metadata, then runs only the daemon starter.

Runtime binaries live in the installed plugin cache by default:

```text
<installed-plugin-cache>/.runtime/<os>-<arch>/ws-mcp-<plugin-version>-<runtime-json-sha12>[.exe]
```

Compatibility checks read `<binary>.metadata.json`, validating its content
SHA-256 and actual build/release version. Build-written protocol and surface
inventories are optional and checked when present. Compatibility stamps also
bind content hash and accepted actual version, not the connecting package's
desired version. Unstamped older release bytes remain usable through a
versioned release's verified SHA256SUMS or metadata-producing repair/rebuild.
Build, bootstrap, copy, stage and seed paths carry that identity; none runs
the binary to discover it.

`runtime.json` is active compatibility data, not release notes. Version, tool,
command, and prompt-bundle drift can make a working binary stale. Use the
version bump helper instead of editing release/version references by hand:

```bash
agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>
```

The helper updates plugin manifests, runtime contracts, Go runtime development
defaults, release workflow references, build script defaults, selected
documentation references, and the `agents-plugin-pi/` mirrors (`runtime.json`,
`bin/ws-mcp-launcher.py`, `rsrc/`, resynced byte-for-byte from `agents-plugin/`,
plus both `ws-pi-bridge` `package.json` versions). Development binaries such as `0.50.8-dev` satisfy the
plugin version the helper just set; older or newer minor releases are stale. The
concrete range lives in `runtime.json.required_mcp` — read it there rather than
restating a version here, which froze at `0.29.2` for seven minor releases
because no bump-script regex matches a bare `X.Y.Z`.

## Release Distribution

`ws-mcp` is distributed as prebuilt native binaries produced from
`agents-plugin-tool/`. End users should not need Go, Node, Cargo, or Visual
Studio Build Tools to run the MCP server.

Release asset names follow this pattern:

```text
ws-mcp-<os>-<arch>[.exe]
```

Initial assets:

```text
ws-mcp-darwin-arm64
ws-mcp-darwin-amd64
ws-mcp-linux-amd64
ws-mcp-linux-arm64
ws-mcp-windows-amd64.exe
ws-mcp-windows-arm64.exe
SHA256SUMS
```

`SHA256SUMS` is a plain `shasum -a 256` manifest covering every release binary.
The launcher verifies the matching checksum line before installation or daemon start. Offline or
proxy failures should keep stdout clean and report the URL or runtime directory
that needs manual repair.

Every supported build entry generates the gitignored embedded package set
before Go compilation. For a direct source build/test, run
`python3 scripts/generate-package-assets.py` from `agents-plugin-tool` first;
`--check` verifies drift. Bare `go build` compiles with a placeholder but cannot
serve playbooks without generation or the explicit dev checkout. Text-only
production edits require a rebuilt daemon; the rsrc schema version tracks
shape, not content. Skills still ship in the external plugin for harness discovery.

Desktop assembly consumes an already-built binary and checks its embedded
inventory against canonical inputs; generating assets after compilation cannot
repair that binary. It still stages the complete external package for its
outside-daemon validation/launcher/config readers. Hosted sessions no longer
use that copy.

The local build script is:

```bash
agents-plugin-tool/scripts/build-release-assets.sh [version]
```

GitHub Actions workflow `.github/workflows/ws-mcp-ci.yml` runs the regular Go
test group on Ubuntu and sharded Windows jobs for pull requests and manual
dispatch. The boolean dispatch input `long_tests` defaults to false; true adds
long-group jobs on both platforms. `scripts/ci-test-shard.sh` owns group
membership: `all` excludes long tests, `rest` also excludes `internal/mcp`, and
`rest` + every MCP shard + `long` covers the full suite. Locally, run the script
from `agents-plugin-tool` with `bash scripts/ci-test-shard.sh all`, adding a
separate `long` invocation when explicitly requested.

Workflow `.github/workflows/ws-mcp-release.yml` requires a successful CI run
on the tagged commit, allowing only the two named long jobs to be skipped.
It cross-compiles assets, uploads workflow artifacts, and publishes GitHub
release assets for pushed `v*` tags. Pull requests touching workflow or
plugin/runtime paths run checks without publishing. This project's
`ai-docs/ship/ws.md` records the owner's long-test choice and requires those
jobs to succeed when requested.

## Local Devenv Repair

Local development has one repository-specific repair exception. The
gitignored `.local-devenv-runtime` marker in the plugin directory is the only
opt-in: with no marker the launcher takes the normal release path whatever the
plugin's location. A valid contract selects source repair for a missing or
incompatible runtime when the plugin runs from a per-user host plugin tree (`~/.codex/plugins/cache/kang-sw-devenv/<ws|wsflow>/`,
`~/.claude/plugins/cache/kang-sw-devenv/<ws|wsflow>/`, or the install.sh
snapshot under `~/.claude/plugins/ws-plugin/<ws|wsflow>/`), or straight from
its source checkout, i.e. when the plugin directory lies inside the marker's own
`source_root` (resolved paths; case-insensitive on Windows). Both the Codex and
Claude plugin trees are recognized so the same source-build dogfood loop works
on either host.

The contract format is:

```json
{
  "schema_version": 1,
  "source_root": "/Users/kang-sw/devenv",
  "tool_dir": "/Users/kang-sw/devenv/agents-plugin-tool",
  "go": "/opt/homebrew/bin/go"
}
```

All paths must be absolute. `source_root` and `tool_dir` must exist, `tool_dir`
must contain `cmd/ws-mcp`, and `go` must be an existing file (an executable bit is
required on POSIX; on Windows the `go.exe` file is trusted because
`os.access(X_OK)` is not meaningful there). If the marker is missing, invalid
JSON, missing required fields, or references missing paths, local repair is
inactive and the launcher uses the normal cache/release path. Local devenv repair
is honored on all platforms including Windows; the build inherits a launch
environment with `HOME` recovered on POSIX and `USERPROFILE`/`LOCALAPPDATA`
recovered on Windows so `go build` resolves its module/build caches under a
sanitized launch env.

When the contract is valid, ordinary startup reuses a hash-bound compatible
runtime even when its source fingerprint is stale. A stale or missing source
stamp emits one stderr advisory pointing at `python3 scripts/dev.py rerun daemon`;
reuse leaves the old fingerprint intact. Missing or incompatible runtimes still
build `<tool_dir>/cmd/ws-mcp` with the declared Go executable. Explicit bootstrap
binary/URL overrides retain their installation semantics. Non-forced repair may
copy a local runtime binary from `<tool_dir>/dist/` or the contract-addressed
`<source_root>/agents-plugin/.runtime/<os>-<arch>/ws-mcp-<plugin-version>-<runtime-json-sha12>`
path before building. Legacy fixed-name source-cache binaries such as
`agents-plugin/.runtime/darwin-arm64/ws-mcp` are not local repair candidates.

If no compatible local runtime can be installed while a valid marker is active,
startup fails instead of falling back to the published release asset.

While a valid marker is active, the runtime version gate is relaxed from an exact
patch match to a **same-minor** match: a locally built runtime is accepted when
its `(major, minor)` equals the installed snapshot's `plugin_version`, regardless
of patch (a `-dev` suffix is ignored). The live source version routinely drifts a
few patches ahead of the installed snapshot's `runtime.json` between
`install.sh update` runs, and the exact-match gate would otherwise reject the
freshly built runtime after every local patch bump, breaking the MCP server until
a re-snapshot. A different minor or major is still rejected, because a minor bump
is a real contract change that must go through a re-snapshot. Released/downloaded
runtimes (no marker) keep the strict exact-match gate unchanged.

### Enabling the local dogfood loop (manual setup)

The marker is per-machine and gitignored, so place it once in the source
checkout for each package you dogfood, then let the plugin install or refresh
copy it into the plugin cache:

1. Write `agents-plugin/.local-devenv-runtime` with this machine's absolute
   paths (use `command -v go` for the Go path). If dogfooding the separate
   wsflow package, write the same marker to
   `agents-plugin-wsflow/.local-devenv-runtime`:

   ```json
   {
     "schema_version": 1,
     "source_root": "/home/you/devenv",
     "tool_dir": "/home/you/devenv/agents-plugin-tool",
     "go": "/home/linuxbrew/.linuxbrew/bin/go"
   }
   ```

2. Run `./install.sh update`. The `rsync` step copies the marker into the
   plugin snapshot and `claude plugin install` carries it into the versioned
   cache, so the next MCP launch can build when no compatible runtime exists.

After setup, edit source and run `python3 scripts/dev.py rerun daemon` at the
coordinated swap time to build, stage and seed the selected runtime. Reconnecting
alone reuses it and reports source drift. Pi lead/fork startup follows the same
reuse policy; its separate child contract-drift guard still rebuilds on a changed
`runtime.json`. Worker/explore startup remains build-free. To disable local
mode, delete the marker and re-run `install.sh update` (the launcher reverts to
the cache/release path).

This path exists only for the repository-local Codex or Claude plugin
development loop. The marker file is gitignored and should not exist in normal
GitHub release installs, downstream repositories, or Windows installs.

### Stable macOS code signing for local builds

On macOS, `ws-mcp` launches the dashboard daemon from the stable path
`<cache-root>/dashboard/bin/ws-mcp-dashboard` (hardlinked to the retained
content-addressed `ws-mcp-dashboard-<sum16>` copy). Ad-hoc linker-signed dev
rebuilds still rotate their `cdhash` on every compile, which causes macOS TCC
prompts (for example Desktop folder access) to repeat. To keep one TCC identity
across local rebuilds:

1. In **Keychain Access**, open **Certificate Assistant → Create a
   Certificate…**, choose **Self-Signed Root** with **Certificate Type: Code
   Signing**, and create it in your `login` keychain.
2. Export `WS_CODESIGN_IDENTITY` to that certificate's name (or SHA-1
   fingerprint) in your shell profile. When unset, local builds are unchanged.
3. Rebuild locally (for example `python3 scripts/dev.py rerun daemon --local` or
   a local-devenv launcher build); `codesign --force -s "$WS_CODESIGN_IDENTITY"
   -i dev.ws.mcp <binary>` runs before `.metadata.json` SHA-256 generation, and
   any signing error fails the build step rather than installing an unsigned
   binary.
4. Grant **Full Disk Access** once in **System Settings → Privacy & Security →
   Full Disk Access** to `<cache-root>/dashboard/bin/ws-mcp-dashboard`.

## Development Verification

Use three verification levels while developing `ws-mcp` and plugin-managed
runtime behavior.

Level 1 validates the Go runtime and host-independent MCP server:

```bash
cd agents-plugin-tool
bash scripts/ci-test-shard.sh all
# Only when explicitly requested:
# bash scripts/ci-test-shard.sh long
scripts/smoke-ws-mcp.sh ..
```

Level 2 validates local release assets:

```bash
cd agents-plugin-tool
scripts/build-release-assets.sh 0.50.8-dev
python3 ../agents-plugin/bin/ws-mcp-launcher.py version
cd dist
shasum -a 256 -c SHA256SUMS
```

Level 3 validates Codex plugin-managed MCP startup from the installed plugin
cache. This level requires a human-in-the-loop plugin cache refresh when plugin
files change: uninstall/install `ws` in the Codex UI or start a fresh session
after cache refresh is available. Then run:

```bash
codex mcp get ws
codex exec --dangerously-bypass-approvals-and-sandbox --json \
  'There is an enabled MCP server named ws. Use its tool named project_tree with arguments {"root":"/Users/kang-sw/devenv"}. Do not use shell commands. Reply with the exact server name, exact tool name, and the first non-empty line of the tool result.' \
  < /dev/null
```

Success means the JSONL output contains an MCP tool call with server `ws`, tool
`project_tree`, and a result whose first non-empty line is `ai-docs/`.

Use Level 1 for ordinary Go/MCP changes, Level 2 for release/build changes, and
Level 3 whenever `.codex-plugin/plugin.json`, `.mcp.json`, launcher behavior, or
installed plugin packaging changes.

## Troubleshooting

- MCP startup fails before JSON-RPC: inspect launcher stderr; stdout must remain
  clean.
- Plugin-managed root resolution points at the plugin cache: pass an explicit
  absolute `root`, call setup with the repository's absolute filesystem path, or
  check `WS_MCP_PROJECT_ROOT` propagation.
- Installed plugin files changed but Codex still starts old behavior: refresh
  the plugin cache through UI uninstall/install or a fresh Codex session.
- Runtime surface mismatch: inspect the running daemon with the Python
  launcher's `runtime capabilities`, or check the binary's hash-bound metadata
  and compare it with `runtime.json`; never execute the binary as a probe.
- Windows startup reports Python alias problems: install Python 3 and refresh
  the plugin so Codex rematerializes the MCP entry.
- Routing or implementation context lost after context compaction: expected — the
  transcript is summarized, but session state persists in
  `<cache-root>/sessions/<session-key>/` (see Session key store). Recover by calling
  `ws.workflow_manual(session_key: <key preserved in the compaction summary>)`,
  which reloads the manual primitives and restores the agenda/todo Session State.
  Do not call `ws.ferrule` to "re-enter" — it is non-idempotent and mints a new key,
  orphaning the prior state.

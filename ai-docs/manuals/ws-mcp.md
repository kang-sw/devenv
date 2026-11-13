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
registry in `agents-plugin-tool/internal/mcp/server.go`.

## Tool Output Convention

An MCP tool's output is read by an LLM before it is read by anything else, so a
new tool's default response is compact, readable text. Return JSON only when a
caller needs stable machine parsing, protocol metadata, or a
compatibility-preserving shape, and put it behind an explicit opt-in (a
`format: "json"` argument) rather than making it the default. Nothing enforces
this - `toolTextResponse` and `toolJSONResponse` are equally available at every
dispatch site in `agents-plugin-tool/internal/mcp/server.go` - so it has to be
applied when the tool is written.

## Process Model

The baseline server is stdio MCP, not a background OS daemon.

```bash
ws-mcp serve --stdio --root <repo-root>
```

The host launches the command and communicates over stdin/stdout JSON-RPC. The
server advertises MCP protocol version `2025-03-26` and tool capability.

Local command examples:

```bash
ws-mcp version
ws-mcp doctor --root <repo-root>
ws-mcp runtime info
ws-mcp runtime capabilities
ws-mcp serve --stdio --root <repo-root>
```

`doctor` is a host-independent smoke check. It verifies the repository root,
`ai-docs/`, and `AGENTS.md`; downstream projects should rely on MCP tools for
bundled conventions rather than repository-local source paths.

### Session key store

Each session key is a directory, `<cache root>/sessions/<key>/`, holding
`key.json` (the identity - root, scope, parent - plus the session-scope
config overrides, the child note, and the once-only flags), `agenda.json`,
`todos.json`, and `whiteboard.json`. An absent part file is that part
empty. A write replaces only the part files it changes, each atomically, and
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
session's first write moves it into `sessions/<key>/` and deletes the legacy
file. Reads never move it, and no pass moves the rest; an unwritten legacy
file ages out through the same prune.

Restart every ws process after updating ws, as an install update already
expects: the session MCP servers (a host restart or MCP reconnect) and the
dashboard daemon (the next launcher start of a newer build replaces it, or
`POST /shutdown`). An older ws-mcp knows only `keys/`, so once a newer one has
moved a session, the older one reports that key unknown. Downgrading loses
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

- `dashboard.lock` - the single-instance OS file lock; `endpoint.json` - port,
  pid, version, and the shutdown token; `dashboard.log` - the running daemon's
  log, including one line per session added to or removed from its index;
  `lan-token` - the LAN access token (LAN mode only), kept across daemon
  restarts; deleting it issues a new one at once. `last-port` - the port the
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
  holding `agent.json`), whose lifetime is the runner's: it heartbeats only
  while the daemon runs, so a daemon down for a week must not lose it.
- `sessions/<reply-id>/snapshot.json` and `sessions/<reply-id>/ring/` - the
  session's live snapshot (current lead and subagent activity, rewritten
  debounced and on a 30-second heartbeat, stale after 3 missed heartbeats by
  its `updatedAt`) and its conversation ring (finished messages, one
  owner-only file each; the newest 2000 are kept on disk, the page shows the
  newest 50 and pages older ones in through the session's history route).
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
  `rev-parse`, `symbolic-ref`, `rev-list --left-right --count`, and
  `status --porcelain` reads, cached per session and refreshed at most once
  per heartbeat interval from the writer's timer, never on an intake call, and
  only when a tool batch ended since the last read (an idle session runs no
  git); the snapshot write that finds the cache missing or stale (a session's
  first, then at most one per heartbeat interval, change or heartbeat alike)
  waits for the refresh, up to its 5 s timeout. There is no automatic fetch, so
  behind reflects the last one. The status read takes no optional
  lock and shares the 5 s timeout; when it fails the counts stay zero. The
  page's status band shows only the folder; the git handle reads the rest
  itself (below).
- `sessions/<reply-id>/inbox/` - the board-answer inbox (`internal/wsinbox`):
  held messages the user sent from the page, one file each; `drained/` keeps
  the newest 50 as sent records. The adapter-only `dashboard.inbox_drain` tool
  drains it, and `mailbox wait --board-inbox` also wakes on it (peek only). A
  message promoted to steering is rewritten with `steer: true` and is no
  longer a merge target; `dashboard.inbox_drain steer_only: true` claims only
  those, and `mailbox wait --board-steer` wakes only on one (the wait an
  adapter arms mid-turn; exclusive with `--board-inbox`). An interrupt entry
  (`interrupt: true`, only its creation time as `sentAt`) asks the adapter to
  stop the running turn; it is never a message (no merge, no image cap, no
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
  list holds (400 otherwise): it is never merged with another entry (a send
  after one starts a new entry), steer refuses it, and a drain returns it in
  order with a `command` field, where the adapter splits its batch and runs
  it natively. A hosted backend passes a plain message verbatim and runs a
  leading-slash line as a command, so a message from any viewer not on this
  machine whose text, trimmed of leading whitespace, starts with `/` is
  refused too (403); a slash later in the text is held as usual.
- `sessions/<reply-id>/agent.json`, `instructions.md`, and `pi/` - an agent
  runner conversation's own files (`internal/wsagent`): the owner-only
  record a process start needs after a daemon restart (profile, backend,
  working root, backend session id, model and effort, the conversation's ws
  session key), the composed instruction file appended to the harness's
  system prompt (owner-only; it carries the session key when the profile
  allows ws tools), and for the pi backend its session file and MCP bridge.
  The daemon reads the conversation from its live stores alone, since no
  ws-mcp serves it and it has no session record.

After a store write the serving ws-mcp sends `POST /nudge {reply_id,
project}`. A mailbox send by a ws-mcp outside the daemon (the direct path)
nudges too, registered or not, because the daemon wakes every `mailbox wait`
it hosts on each nudge before it checks the named session (at most one
recheck per wait per second, plus a trailing one). A server the daemon
hosts (Daemon-Hosted MCP below) instead hands each live snapshot to the
daemon in memory (`wsdashboard.HostedSlot`): the daemon serves that copy in
place of `snapshot.json` and wakes the session's pages with a live-only
wake, whose render re-reads only the snapshot (and the ring after an append,
which the slot signals in place of a nudge). The hosted writer still writes
`snapshot.json` for recovery: once per heartbeat interval while the state
changed, at each turn end, and when its server closes (which also drops the
copy, so the file renders again); an idle heartbeat writes nothing and only
moves the copy's `updatedAt`. A session whose directory the daemon does not
serve stays on the file-and-nudge path. The daemon also re-reads every
session every 30 seconds, except
an ended one it already read since it ended (a nudge or a resume still
re-reads it). A re-read compares a fingerprint taken from metadata (the
session record, live-store and mailbox-history entry names, the ticket
directories' file names, sizes, and modification times, the unread count,
the contacts, the listening marker) and builds no panel; the board renders
only when a page reads the changed session. Ring, drained-inbox, and
mailbox-history entry files are parsed once per daemon and kept by name for
the window the page shows. The small files every read and render re-reads
(session snapshots, the session record's part files, the reply registry,
the named scopes' mailbox stores) are kept parsed while a stat shows the
same size and modification time; a file modified within the last 2 seconds
is not kept, and a session's entries drop when it leaves the index. A
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
a root only while at least one session page under it holds its SSE stream
open: once when the first stream opens, every 3 minutes while one stays, and
at once on `POST /s/<reply-id>/git` (sent when the git drawer opens; it needs
an `Origin` equal to the daemon's own, answers 204, and 409 when no open page
watches the root). One goroutine per watched root runs every read, so pages
sharing a root share one read and no two reads of a root overlap; a refresh
inside 2 seconds of the last read waits for that gap's end, and requests
arriving meanwhile fold into that one read. The last stream's close ends the
root's reads and drops its value. A read is `rev-parse`, `symbolic-ref`,
`rev-list --left-right --count`, and `status --porcelain` with no optional
lock (`wsgit.BranchSync`, `wsgit.WorkingTreeCounts`), in the C locale,
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
removes or releases a worktree itself.

Every session page carries the session rail (`internal/wsdashboard/rail.go`,
`pageview_rail.go`): the listed sessions grouped by repository (or plain
folder) and worktree. It is one shared render (region `rail`, keyed by
language and theme, no reply-id) that each session stream sends beside its
page with a broker subscription of its own, so a session's change renders
the rail once for all open pages and no other session's page. A session's
repository comes from the registration's `git_common_dir` (ws-mcp's one
`rev-parse --git-common-dir` at first registration), else from its root's
`.git` on disk. The worktree rows come from `git worktree list --porcelain`
through the source's optional `RepoWorktreeSource` (pooled worktrees under
the `worktree_pool` roots show only while they hold a session), read only
while a page streams the rail: on the first such stream, every 3 minutes,
for a newly shown repository alone, and on `POST /rail/refresh` (own Origin;
409 with no streaming page). Change counts come from the newest session
snapshot in the worktree; the rail never reads a working tree. Drafts,
stars, and collapsed workspaces are browser-local (`assets/rail.js`).
Ended sessions fold apart under each worktree and stay listed as long as
the index holds them (their registry directory and session record exist),
past the index page's 24-hour window: on a daemon that hosts lead sessions
an ended row carries how it comes back (`RailRow.Revive`): `reopen` for a
closed hosted session, `takeover` for a claude or pi session a terminal ran
in a repository (shown disabled while its key record names no harness
conversation), nothing for a plain-folder session. Whether a terminal
harness has exited is asked when the entry is used (the **Moves** bullet
of the hosted lead sessions below). `PruneRegistrations` spares an agent runner's directory
while the runner may serve it; a lead whose record is closed or handed off
ages by its row like any ended session, and every other agent record (the
tracker's support agent) keeps the runner's lifetime. One retired lead is
never pruned: a handed-off pi session whose session file is inside its
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
allow-popups-to-escape-sandbox`, an opaque origin that sends no cookie), so
its images are inlined as `data:` URLs up to 2 MiB under the same containment
(`wsdashboard.ImageDataURL`); a larger image falls back to the image route,
which a LAN viewer's frame cannot load. An absolute image path outside the
root is never inlined (the frame's content is the same for every viewer):
it stays path text marked `data-relay` with its image route URL, and the
frame element lists those URLs as its own `data-relay`. The frame cannot load
that route itself (its opaque origin's requests are `Sec-Fetch-Site:
cross-site`, which the route 404s), so it asks the page by `postMessage`;
the page's relay handler accepts the request only from one of its own body
frames and only for a URL on that frame's list that is this session's image
route for an image name, fetches it same-origin (served only to a loopback
viewer), and posts the frame an image `Blob`, which the frame shows in the
path text's place once it decodes. A LAN viewer's fetch 404s and the path
text stays. The served board renders the 30 most
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
refs, refNums}` holds the message in the session's inbox (merged into a still-held
one that is not steered). `images` are new uploads `{name, data, n}` (base64,
sniffed as PNG/JPEG/GIF/WebP, at most 8 per held message), `refs` name files
a cancel returned, and `refNums` numbers them; the daemon writes the uploads
into the inbox's `attachments/` directory. The text names image `n` with an
`[Image #n]` marker, and the delivered message is the `<wb:answers>` block
of the board answers and topic notes (`wsinbox.AnswerBlock`), then the text,
then one `Image #n: <absolute path>` line per image; a send that merges renumbers its
images and markers to continue the held message's, under the daemon's inbox
lock. The page still reads the older `[image: <path>]` lines. `POST
/s/<reply-id>/cancel {id}` takes a held one back, answering `{text, chips,
topics, images, nums}`, and `POST
/s/<reply-id>/steer {id}` promotes a held one to steering, answering the
rewritten entry's new `{id}` (409 once the session has received it). The
adapter claims a steered message at the session's next tool boundary instead
of at the turn end. `POST /s/<reply-id>/interrupt` (body ignored) takes back
every held message, steered or not, and writes an interrupt entry in one step
under the inbox lock, answering `{held: [{id, text, chips, topics, images, nums}]}`
oldest first, each shaped as a cancel's answer. All four require an `Origin` equal to the daemon's own,
and a session without a fresh adapter snapshot gets 409. `whiteboard.update`'s `full:` line names
`http://127.0.0.1:<port>/s/<reply-id>` once the session's last nudge was
answered, and the page path otherwise.

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
a daemon restart the runner re-registers each conversation with no process
and starts one at its next send. An attached ws-mcp gets
`WS_MCP_ALLOWED_TOOLS` (the profile's ws tools) and `WS_MCP_NO_DASHBOARD`,
and no mailbox identity. Its command is Python plus the starting plugin's
`bin/ws-mcp-launcher.py serve --stdio`, never the daemon's staged executable.
The plugin is resolved beside `WS_RSRC_ROOT`; a missing launcher refuses the
start. The launcher's interpreter (`WS_MCP_HOLDER_PYTHON`) is reused when
known, else `python3` on PATH (Windows: `py -3` fallback). The conversation's
root overrides `WS_MCP_PROJECT_ROOT` in that command's environment, keeping
proxy mode's exact arguments and preventing the daemon's lead root leaking
into the worker. A proxy-hosted worker still uses its capability token for
nested `pi.*` calls, not the daemon's lead runner handle.

A spawned conversation (`internal/wsagent/spawn.go`) is a non-lead
conversation another session opens with its first message (`Runner.Spawn`:
profile, root, optional cwd, model, effort, label, and parent `{kind,
reply_id}`) and steers by its id; its record keeps `cwd`, `label`, and
`parent`, and it is unlisted like a support conversation. Each turn settles
once: done, failed (the harness's error, its process ending under it, or a
process that could not start or take the message), or interrupted (a turn
the daemon's exit cut off, which the record's `turn_open` marks, settled
when the next daemon loads it). A settle of a conversation with a parent
goes to the runner's `Options.Report`; closing one settles nothing.
`Runner.Wait` waits for a turn that took every message sent before the
wait (a message the page cancelled before a drain counts for no turn, and
a slash command's turn covers none of the text drained behind it); the
last reported turn is kept in the record (`last_turn`), so after a daemon
restart a wait on a conversation that settled before it answers at once.
`Runner.Watch` reports each change of a conversation with a parent.
The built-in `pi-worker` profile is Pi's coding tools plus ws's
`git.status`, `git.diff`, `git.log`, `git.merge_base`, `git.commit`, and the
six `pi.*` tools, with no instruction file. A spawn records its `tier`
(`SpawnSpec.Tier`, `Record.tier`; an unknown name is refused).

The `pi.*` tools (`internal/mcp/pi_tools.go`, lead-only) put this on a
session: `pi.spawn {prompt, tier, cwd?, label?}` (model and effort from the
tier's entry in the `pi` harness bucket of `agents.tier`, which only the
user's `config.tune` sets; an unset tier is refused with that key named;
`cwd` relative to the session's root), `pi.send` (a running child's native
Pi steer at the next tool boundary; with no turn running, a new turn at once), `pi.list` (the caller's
own children), `pi.stop` (closes; the record stays), `pi.transcript`, and
`pi.wait` (default 300 s, at most 1800 s, for the turn after the last
send). The daemon mails each settled turn to the parent lead's reply-id
queue with no sender (`mailbox.recv` shows `system`) and wakes the mailbox
waits; a steered message belongs to the turn that takes it, whose completion
mail covers it without an extra turn. A steer refused or not taken before
the turn ends is cleared from Pi's queue and sent as the next turn instead.
Nothing is mailed after `pi.stop`. `tools/list` shows the tools only
when `dashboard.pi_spawn_enabled` is on (ws on, wsflow and Pi off; read
once per server, so a change applies at the next reconnect), the server is
hosted by the daemon (`ServerOptions.Agents` is the daemon's runner; a
direct-path ws-mcp has none and a call is refused naming the daemon), and
`pi --version`, probed once per process, is 1.0 or later. Each intake-fed
server watches the runner and feeds the owning lead's recursive spawn tree
(`pi:<id>`, `agentKind: "pi"`, linked to the child's page) into its live state;
a live session the server creates, or one an adapter resets, gets its open
descendants' rows again, so they come back after a reconnect. A dashboard-hosted
lead instead gets those rows from its runner-owned snapshot writer, including
after daemon replacement; hosted intake remains off. Descendants nest under
their spawner through ordinary `parentId` links. When a harness also reports the
same conversation (its reply-id or native conversation id), the snapshot keeps
one daemon-owned lifecycle row with the harness's activity and telemetry. An
intake-only session whose dashboard feed is off gets no row. The agents panel
shows a running Pi row with a blinking `pi` badge in place of the pulse.

A spawned Pi worker spawns Pi subagents of its own through the daemon
(`internal/wsagent/children.go`, `internal/mcp/pi_forward.go`); the daemon
owns the whole tree:

- **Token.** A conversation `Runner.Spawn` started with a parent and a
  recorded tier, whose profile allows a `pi.*` tool, gets a capability token
  (`WS_MCP_PI_TOKEN`) and the daemon's loopback base URL
  (`WS_MCP_PI_ENDPOINT`, set by `ws-mcp dashboard` through
  `Runner.SetEndpoint` before `Start`) in its attached ws-mcp's environment.
  The token is drawn at every process start, never written to disk, and void
  once that process ends or the conversation closes. A conversation opened
  through `POST /agents`, or spawned with no tier, gets none. Its session key
  stays a parent-less delegate key: the token authorizes `pi.*` alone, so
  every other lead-only tool stays refused.
- **Forwarding.** A ws-mcp with no runner and both variables set lists the
  `pi.*` tools with the worker's descriptions and forwards each call to
  `POST /agents/by-token/<op>` (`spawn`, `list`, `send`, `stop`,
  `transcript`, `wait`) with the token as a bearer credential; the keyed
  gate lets a delegate key through for `pi.*` there only. The route answers
  only this machine (404 otherwise) and 401 for a missing, forged, or void
  token, and scopes every op to the holder's own children (404 for any other
  id). `dashboard.pi_spawn_enabled` is checked only at the root, on the
  lead's `pi.spawn`; the Pi version is the daemon's check on every call
  (`Options.ChildCheck`, the version probed once per daemon process). The
  worker's own server checks neither.
- **Cwd.** A worker's empty `cwd` is the worker's own working directory
  (its record's `cwd`, else its root), and a relative one resolves against
  it, so a worker in a worktree spawns into that worktree. Both a lead's
  and a worker's spawn bind the child's root (and its delegate key) to the
  Git worktree containing the resolved cwd; a cwd outside Git keeps the
  caller's root. A lead's `pi.spawn` keeps resolving cwd against the lead's
  root.
- **Caps.** A spawn records the holder as the child's parent (`parent.kind:
  "conversation"`). The lead is depth 0, its Pi worker depth 1, and that
  worker's child depth 2, which is refused a spawn with the cap named. The
  child's tier resolves in the `pi` bucket alone and may not rank above the
  holder's own (`small` < `medium` < `large` < `xlarge`); a holder with no recorded tier
  is refused.
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
  records, so a daemon restart delivers them; a child the restart
  interrupted reports that, which resumes its parent. The interrupted turn
  settles before the restart sends the input the record kept for the next
  turn, so that next turn stays open.
- **Close.** Closing a conversation closes its open descendants first,
  deepest first. A process stop (the stop route, an idle stop, a settings
  change) does not cascade, and a child's report resumes the stopped
  parent. A report to a closed parent stays in the child's record.

The runner also hosts lead sessions: a headless Claude Code or Pi process
that loads the user's own harness setup (no profile allowlist, plugins, or
system prompt of the runner's), started from the built-in `lead-claude` or
`lead-pi` profile. `POST /agents` refuses a lead profile; a lead is spawned
by `POST /agents/leads {root, harness: "claude"|"pi", model?, effort?,
permission_mode?}` (201 with the state; `root` must be inside git, since the
session key is minted at the canonical root). Differences from a support
conversation:

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
  the daemon's own `CLAUDE*`, `WS_MAILBOX*`, `WS_MCP_*`, `WS_RSRC*`,
  `WS_HOSTED_*`, and `WS_PI_*` variables are not passed on. A ws-mcp that is
  not the hosted harness's never writes, heartbeats, removes, or end-marks a
  row that carries `hosted: true`, whatever key it is given.
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
  `internal/mcp/lead_mail.go`), claims what arrives into the record, and
  submits it as a turn whose text is what the mailbox wait hands a session;
  the session arms no wait of its own. A turn no send started (a background
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
  Permission modes: `auto` (the default), `default`, `acceptEdits`, `plan`,
  `dontAsk`, `bypassPermissions`; a Pi lead has none.
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
  its model and context size from its own assistant lines. The stream
  gives no output rate (no partial messages: each block arrives whole,
  with the request's starting output count). Idle stop is the profile's
  `idle_timeout_seconds` (a Claude lead 600, a Pi lead off) and waits for
  the background task count to be zero.
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
  and the background task count in the agents panel head and beside the
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
efforts}], last}`: the harness is each support profile's backend resolved
through the global and builtin layers; unavailable when the daemon lacks the
backend or its binary is not on PATH; Claude's models are the CLI's
version-less aliases `opus`, `sonnet`, `haiku`, `fable` and its efforts the
Claude levels, Pi's models come from `pi --offline --no-approve
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
Host. LAN mode (`dashboard.lan_enabled`, read from the global config layer
only and passed as `dashboard --lan`) binds every interface: a request whose
peer address and Host are both loopback still passes with no token, and every
other request, on every route, needs the LAN access token, which is separate
from the endpoint file's shutdown token, with one exception: GET and HEAD of
the shared mermaid script (`/s/<content-hash>.js`, the vendored library under
a hash name, so it reveals nothing of any session) pass without it, because
a topic body's sandboxed frame, an opaque origin, loads it with no cookie. The token arrives as `?token=` on the
address the index page shows to a viewer on this machine only; that answer
sets an HttpOnly cookie, which the page's links, SSE stream, images, and posts
then carry, so no rendered URL holds it. `/health` reports the mode as `lan`,
and the daemon re-reads the global value when a session connects: a value
that differs from its own mode replaces it through the drain path below.

At an equal version the newer build wins: `/health` reports `build_time` (the
plugin's runtime binary's mtime in Unix milliseconds, which the launcher
passes to `daemon-start --build-time` and the daemon receives as `dashboard
--build-time`), and a launcher whose build time is newer than the daemon's
(a daemon that omits it counts as older) replaces it; an older or unknown
build time never does, so builds of one version side by side leave each
other's newer daemon alone. A rebuilt dev binary therefore replaces the
running daemon at the next session start. To force a restart anyway, `POST
/shutdown` with `Authorization: Bearer <token from endpoint.json>`; the next
session start brings a new one.

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
vendored mermaid bundle and the MCP server's own whiteboard page file and embed
stay embedded. A running daemon keeps its mode, so shut it down first
(`POST /shutdown` above), then start
`WS_MCP_DASHBOARD_DEV_SOURCE=$PWD/agents-plugin-tool ws-mcp dashboard`; a
half-edited template answers 500 (the error is in `dashboard.log`) and the next
request retries.

An isolated probe daemon needs both `WS_CACHE_HOME` (state directory, session
keys, mailboxes) and `WS_CONFIG_HOME` (global config, the stored workspace
list, which sits beside the config file) pointed at scratch directories, and
its own `-port`: with only the cache home moved, the rail's workspace routes
write the real `~/.ws/dashboard/workspaces.json`. A harness the probe daemon
hosts loads the installed plugin, whose launcher may start the installed
runtime as the daemon when the probe daemon stops; put a `claude` first on
`PATH` that `exec`s the real one with the plugin under test (an `exec`
keeps the PID the hosted-session check compares) and
`WS_MCP_BOOTSTRAP_BINARY` set to the probe binary.

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

Session holders: a holder is a small Python process (standard library only,
3.10 or newer) that owns one child process, over pipes or a PTY (ConPTY on
Windows), and outlives the daemon, so the child survives a daemon restart.
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
32 MiB for a hosted lead conversation's holder, `leadRingBytes`, so a rejoin
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

For `serve --stdio` and `mailbox wait` the launcher is a proxy: it does not
run the plugin's runtime binary but connects to the dashboard daemon of its
cache root, which hosts the MCP server for that session, and copies bytes
between its stdin/stdout and that connection. On this path the only ws-mcp
executable that runs is the daemon's, once per build (the endpoint-protector
prompt that a per-session executable caused). Every other subcommand
(`version`, `call`, `runtime`, ...) still runs the runtime binary directly.

Finding and starting the daemon (`DaemonLink` in the launcher):

1. The daemon directory is `<cache root>/dashboard`, the cache root being
   `WS_CACHE_HOME` (absolute, symlinks resolved when it exists) or
   `~/.cache/ws@kang-sw-devenv`, as `wsstate.CacheRoot` resolves it; the test
   `test_cache_root_and_staged_name_follow_the_go_rules` pins the parity.
2. `endpoint.json` names the port and token; `GET /health` answers the
   daemon's version, `build_time`, and `content_hash` (the sha256 of the
   daemon's executable). A daemon whose endpoint answers nothing within
   5 s (10 s on Windows) counts as absent; a port that accepts the
   connection but never answers within that budget is a hung daemon, and
   the launcher runs the direct path rather than starting a second one.
3. The daemon is started, or replaced, only when none answers, the running
   one's version is older than the plugin's (`runtime.json`
   `plugin_version`), or equal with an older `build_time` than the runtime
   binary's mtime *and* a different `content_hash`: the same bytes never
   replace a daemon whatever their mtime (Claude, Codex, and Pi each
   install their own copy of one binary, and a replacement drains every
   session on the machine), so build time breaks the tie only between
   different builds (`keepRunning` in Go, `needs_start` in the launcher).
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
   The launcher leaves the other staged entries alone (one of them is the
   running daemon's own executable, which it respawns from on a LAN mode
   change); the daemon that wins the lock prunes the directory down to its
   own, keeping `.tmp` files and entries younger than two minutes, which
   another launcher may still be staging. The launcher then waits up to
   `DefaultStartWait` + 5 s (Windows
   + 10 s) for a healthy daemon and applies the replace rule to the one
   that answers: when that is still the older daemon (it refused to stop,
   or won the lock race) the launcher runs the direct path rather than
   settle on it.
4. A daemon in another minor is never used: the launcher runs the direct
   path. The same minor, newer, is used as is (its tool set may be larger
   than `runtime.json` lists; compatibility on this path is the version rule
   only).

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

On the daemon path the compatibility probe (`runtime capabilities` on a
stamp miss, see Runtime Repair And Drift) also runs the staged copy, so the
plugin-directory binary never executes: one executable path per build is
what keeps the endpoint protector to one prompt per version.

Direct fallback: when the daemon cannot be reached or started within those
budgets, or refuses the hello, the launcher writes one `ws-mcp-launcher: ...;
running the direct path` line to stderr and runs the runtime binary as
before (`serve --stdio`, or the `mailbox wait`). `WS_MCP_LAUNCHER_DIRECT=1`
forces the direct path. A direct-serve child takes a new mailbox owner
identity (its own PID, not the launcher's), so an address the hosted session
held conflicts until the stale lease expires; the direct path is the rare
exception, and this is an accepted limitation.

Reconnect: when the connection ends while the client is still attached (the
daemon was replaced or died), the proxy connects again (giving a successor
1.5 s before starting one), re-sends the client's captured `initialize`
request under an id of its own (`ws-mcp-launcher-replay-<n>`, so a late
answer from the old daemon can never be mistaken for it) and drops its
answer, re-sends `notifications/initialized`, and answers every request that
was in flight with a JSON-RPC error (`-32000`, "ws-mcp daemon connection was
replaced"); the client never sees the server go away. Reconnecting is
bounded: a connection the daemon closes within 2 s of accepting it counts
as a quick close, each further attempt waits 0.5 s × the count, and after
three in a row the proxy continues on the direct path. When no daemon comes
back within the start budget, a direct `serve --stdio` child takes over the
same way. A hosted server for a reconnected session re-registers that
session's mailbox presence on its first keyed call and keeps the owner
pointer and the `WS_MAILBOX_AUTO` stem its predecessor published, provided
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
version outside the daemon's minor, an MCP hello without `WS_RSRC_ROOT`, or
a CLI argv other than `mailbox wait ...`), closing the connection after a
refusal. After `ok` the stream is the MCP session in `mcp` mode: the daemon
builds one `mcp.Server` from the hello (`NewServerWithOptions`: the hello's
environment is the server's whole environment and that of every subprocess
its tools start, `owner_pid` is its PID for mailbox presence and listening
markers, the debug event ring and `WS_MCP_DEBUG_LOG` are its own, resource
and skills roots come from `WS_RSRC_ROOT`/`WS_SKILLS_ROOT` in the hello with
no executable-relative fallback, and startup diagnostics such as a rejected
config manifest go to the daemon log and the connection's debug events). The
harness is detected from `initialize` as on stdio; the hello's `harness` is
an optional hint. Closing the connection ends the session: its dashboard
session, its mailbox presence, and any wait it holds.

In `cli` mode the daemon runs `mailbox wait` with `argv[2:]` (a relative
`--root` resolves against the hello's `cwd`, and the wait's stores follow
the hello's `WS_CONFIG_HOME`/`WS_CACHE_HOME`) and answers in frames, one
JSON object per line: `{"stream": "stderr", "data": "..."}` for each stderr
write as it happens, then one `{"final": true, "exit_code": <n>, "data":
"<the whole stdout>"}`, `exit_code` always present (0 included). The
launcher writes them as UTF-8 bytes to its own stderr and stdout and exits
with `exit_code`, so the mod and the Pi waiter see the same output and codes
as from the direct CLI; closing the connection (the launcher exits, or the
daemon drains) cancels the wait at once and clears its listening marker
(`exit_code` 130, "interrupted"). A hosted wait rechecks on an in-process
signal (a mailbox send by a server the daemon hosts, a board-answer inbox
write, or a nudge) with a 10 s backstop poll; the direct CLI wait polls
every 500 ms.

## Launcher Environment

Current launcher inputs:

| Variable | Purpose |
|----------|---------|
| `WS_MCP_RUNTIME_DIR` | Override the runtime binary directory; defaults to plugin-local `.runtime/<os>-<arch>`. |
| `WS_MCP_RUNTIME_BINARY` | Exact repaired runtime binary path exported by the launcher for async worker recovery. |
| `WS_MCP_BOOTSTRAP_BINARY` | Copy a prebuilt local binary into the runtime directory. |
| `WS_MCP_BOOTSTRAP_URL` | Download a prebuilt binary when no runtime binary exists. |
| `WS_MCP_BOOTSTRAP_SHA256` | Optional SHA-256 checksum for `WS_MCP_BOOTSTRAP_URL`. |
| `WS_MCP_RELEASE_REPOSITORY` | Override the GitHub release repository from `runtime.json`. |
| `WS_MCP_RELEASE_TAG` | Override the release tag from `runtime.json`, for example `v0.49.0`. |
| `WS_MCP_RELEASE_BASE_URL` | Override the full release asset base URL; useful for local file or HTTP smoke tests. |
| `WS_MCP_LAUNCHER_DEBUG` | Print launcher diagnostics to stderr when set to `1`. |
| `WS_MCP_LAUNCHER_DIRECT` | `1`/`true`/`yes`/`on` makes the launcher run `serve --stdio` and `mailbox wait` on the runtime binary directly instead of through the daemon (see Daemon-Hosted MCP). Unset is off. |
| `WS_MCP_PROJECT_ROOT` | Project root default for root-aware tools and CLI commands when no higher-priority root exists. |
| `WS_MCP_DASHBOARD_DEV_SOURCE` | Read by `ws-mcp dashboard`: the `agents-plugin-tool` source root the daemon serves dashboard and whiteboard page assets from instead of the embedded copies (see Dashboard daemon, Dev assets). Unset is off. |
| `WS_MCP_DASHBOARD_PPROF` | Read by `ws-mcp dashboard`: `1`/`true`/`yes`/`on` serves `net/http/pprof` under `/debug/pprof/` to loopback viewers only, never to a LAN-token request (see Dashboard daemon, Profiling). Unset is off: the routes do not exist. |
| `WS_MCP_HOLDER_PYTHON` | Read by `ws-mcp dashboard`: the Python interpreter that runs session holders (see Dashboard daemon, Session holders). The launcher sets it to its own interpreter on the daemon-start step unless it is already set; unset, the daemon uses `python3` on PATH. |
| `WS_MCP_NO_AGENT` | Product-mode gate for agentless distributions such as wsflow. |
| `WS_MCP_NO_DASHBOARD` | `1`/`true`/`yes`/`on` turns the dashboard off for this ws-mcp whatever `dashboard.enabled` resolves: no session registration, no prune (the daemon itself is started by the launcher, never by this ws-mcp). The agent runner sets it on the ws-mcp it attaches to a conversation. |
| `WS_HOSTED_SESSION` | `1`/`true`/`yes`/`on` marks a session the dashboard daemon hosts; the daemon sets it, with `WS_HOSTED_SESSION_KEY` and `WS_HOSTED_SESSION_ROW`, on the harness it spawns. Every process below that harness inherits the three, so each reader (this ws-mcp, the Claude mod, the Pi extension) honours them only when the row names its own harness process (see `WS_HOSTED_SESSION_ROW`), resolves that once, and never becomes hosted later. Hosted, this ws-mcp neither registers, heartbeats, nor prunes (the daemon writes the session's registration) but still nudges the daemon and names the daemon's page in `whiteboard.update`; `ferrule` returns the hosted key for a parent-less lead bootstrap and refuses a re-login with any other key, and `workflow_manual` refuses any other key except a parent-carrying child key; child mints are unchanged. Set without a key, it changes nothing here. Unset is off. |
| `WS_HOSTED_SESSION_KEY` | The parent-less lead key the daemon minted for the session it hosts, read only while `WS_HOSTED_SESSION` is on; the session's key stays fixed to it. |
| `WS_HOSTED_SESSION_ROW` | Absolute path of the hosted session's `registration.json`. Its `host_pid` is the one harness process the flag holds for: equal to the reader's harness PID, hosted; another nonzero PID, an ordinary session, silently (a nested harness, a CLI call from the session's shell); 0, absent, or unreadable, read again about every 100 ms for about 3 s, then an ordinary session with one warning. Unset (an older daemon, a stray flag) is not hosted. The harness PID is `WS_MCP_HARNESS_PID` for ws-mcp (with a Windows `cmd.exe` wrapper skipped, see that row), the mod's own process for Claude (read as the parent of a host command; on Windows, when that parent is the `cmd.exe` shell the host command ran through, that shell's parent, by the same one-level rule), and the extension's process for Pi. |
| `WS_MCP_HARNESS_PID` | Set by the launcher on every start to its own parent PID, over any inherited value: the harness that started this ws-mcp. The runtime reads it as that harness's PID, except on Windows when that process is a `cmd.exe` (the wrapper a harness commonly starts a stdio server through): then its parent. Exactly one level is skipped, so a harness nested under another still tells itself from the outer one, and a failed process lookup keeps the PID as set. It reaches the runtime on both paths (the direct child's environment, and the hello's `env` for a daemon-hosted connection, so it is per caller). A ws-mcp started without the launcher has none and is never hosted; `ws-mcp dashboard` drops it, with the three `WS_HOSTED_SESSION*` variables, from its own environment at start, so the conversations it spawns inherit no session's. |
| `WS_MCP_ALLOWED_TOOLS` | Comma list of ws tool names; when set, `tools/list` advertises and `tools/call` accepts only these. The agent runner sets it to a profile's ws tools. `tracker.show`, `tracker.ui`, and `git.search` are listed and callable only when it names them. |
| `WS_MCP_PI_TOKEN` | Set by the agent runner on the ws-mcp of a spawned conversation that may spawn Pi subagents: its capability token for the daemon's `POST /agents/by-token/<op>` route, drawn at each process start and void when that process ends. With `WS_MCP_PI_ENDPOINT` and no runner of its own, this ws-mcp forwards `pi.*` to that route (see Dashboard daemon, the Pi spawn tree). Unset, `pi.*` is lead-only. |
| `WS_MCP_PI_ENDPOINT` | Set beside `WS_MCP_PI_TOKEN`: the dashboard daemon's loopback base URL, for example `http://127.0.0.1:29417`. |
| `WS_MCP_NAMESPACE` | User-facing MCP namespace text override; empty or unset defaults to `ws`. |
| `WS_MCP_SETUP_TOOL` | Advertised setup tool name override; empty or unset defaults to `ws.setup`. |
| `WS_MCP_CONFIG_MANIFESTS` | Adapter config key manifests, separated by the platform path-list separator (`:` POSIX, `;` Windows); `;` is accepted on every platform, which the static launcher configs use. Declared keys become `config.tune`-writable, cataloged by `config.list`, and typed in `config.get`; a rejected manifest is logged to stderr and listed by `config.list`. The ws and wsflow packages each ship their own `config-manifest.json` (namespace `claude.`; the same keys, differing only in `default`, pinned by `TestShippedClaudeManifestsDifferOnlyInDefaults`) and set it in both launch configs: `.mcp.json` (Codex) as `./config-manifest.json`, resolved against `cwd: "."` (the plugin root), and `.claude-plugin/plugin.json` as `${CLAUDE_PLUGIN_ROOT}/config-manifest.json`. Each package (ws, wsflow, Pi) also lists its `dashboard-config-manifest.json` (namespace `dashboard.`: `dashboard.enabled`, default on in every package; `dashboard.port`, default 29417; `dashboard.lan_enabled`, global-only and default off in every package; `dashboard.pi_spawn_enabled`, which lists the `pi.*` tools, default on in ws and off in wsflow and Pi) beside its own manifest; the Pi bridge joins the two with the platform separator. A path that does not resolve fails nothing: the keys stay undeclared, so check `config.list` after changing either. |
| `WS_MCP_CLI_NAME` | Command name that printed agent-runnable CLI commands (the rendered `mailbox wait` command and its re-arm line) start with; empty or unset uses the invoked binary path (`os.Args[0]`). Set by the `ws-cli`/`wsflow-cli` shims and by the Claude manifests' `mcpServers.<name>.env`, where plugin `bin/` is on the agent's Bash PATH; left unset for Codex, which does not put plugin `bin/` on PATH. |

Launcher diagnostics must go to stderr. Stdout belongs to the MCP JSON-RPC
stream from the moment the launcher connects to the daemon or execs the
repaired runtime.

## Runtime Repair And Drift

The launcher reads plugin-local `runtime.json`, repairs a missing or
incompatible runtime binary, then connects to the daemon it starts from that
binary (Daemon-Hosted MCP above) or, for other subcommands and on the direct
fallback, execs `ws-mcp`.

Runtime binaries live in the installed plugin cache by default:

```text
<installed-plugin-cache>/.runtime/<os>-<arch>/ws-mcp-<plugin-version>-<runtime-json-sha12>[.exe]
```

Compatibility checks use `ws-mcp runtime capabilities` when available. That
single probe reports runtime version, source commit, MCP protocol version,
prompt bundle metadata, MCP tool names, and CLI command names needed by the
launcher. Older runtimes fall back to bounded legacy validation when the
contract allows it.

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
plus both `ws-pi-bridge` `package.json` versions). Development binaries such as `0.49.0-dev` satisfy the
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
The launcher verifies the matching checksum line before chmod/exec. Offline or
proxy failures should keep stdout clean and report the URL or runtime directory
that needs manual repair.

The local build script is:

```bash
agents-plugin-tool/scripts/build-release-assets.sh [version]
```

GitHub Actions workflow `.github/workflows/ws-mcp-release.yml` runs tests,
cross-compiles assets, uploads workflow artifacts, and publishes GitHub release
assets for pushed `v*` tags. Pull requests touching workflow or plugin/runtime
paths run checks without publishing.

## Local Devenv Repair

Local development has one repository-specific repair exception. The
gitignored `.local-devenv-runtime` marker in the plugin directory is the only
opt-in: with no marker the launcher takes the normal release path whatever the
plugin's location. With a valid contract, the launcher forces local runtime
repair before accepting an already compatible cache-local binary when the
plugin runs from a per-user host plugin tree (`~/.codex/plugins/cache/kang-sw-devenv/<ws|wsflow>/`,
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

When the contract is valid, the forced local path builds
`<tool_dir>/cmd/ws-mcp` with the declared Go executable first so pre-release
dogfood exercises the current checkout. Non-forced repair may copy a local
runtime binary from `<tool_dir>/dist/` or the contract-addressed
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
   cache, so the next MCP launch source-builds the current checkout.

After setup, editing the Go source and reconnecting the MCP server is enough to
pick up changes — no rebuild-and-stage step. To disable, delete the marker and
re-run `install.sh update` (the launcher reverts to the cache/release path).

This path exists only for the repository-local Codex or Claude plugin
development loop. The marker file is gitignored and should not exist in normal
GitHub release installs, downstream repositories, or Windows installs.

## Development Verification

Use three verification levels while developing `ws-mcp` and plugin-managed
runtime behavior.

Level 1 validates the Go runtime and host-independent MCP server:

```bash
cd agents-plugin-tool
go test ./...
scripts/smoke-ws-mcp.sh ..
```

Level 2 validates local release assets:

```bash
cd agents-plugin-tool
scripts/build-release-assets.sh 0.49.0-dev
dist/ws-mcp-darwin-arm64 version
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
- Runtime surface mismatch: run `ws-mcp runtime capabilities` against the
  cache-local binary and compare it with `runtime.json`.
- Windows startup reports Python alias problems: install Python 3 and refresh
  the plugin so Codex rematerializes the MCP entry.
- Routing or implementation context lost after context compaction: expected — the
  transcript is summarized, but session state persists in
  `<cache-root>/sessions/<session-key>/` (see Session key store). Recover by calling
  `ws.workflow_manual(session_key: <key preserved in the compaction summary>)`,
  which reloads the manual primitives and restores the agenda/todo Session State.
  Do not call `ws.ferrule` to "re-enter" — it is non-idempotent and mints a new key,
  orphaning the prior state.

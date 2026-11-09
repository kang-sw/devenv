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
the lookup every keyed call makes reads `key.json` alone. `key.json` is also
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
dashboard daemon (the next `serve` of a newer build replaces it, or `POST
/shutdown`). An older ws-mcp knows only `keys/`, so once a newer one has
moved a session, the older one reports that key unknown. Downgrading loses
sight of moved sessions the same way; session keys are cache state, so a
fresh key is the recovery.

### Dashboard daemon

`ws-mcp dashboard` is an optional machine-wide daemon (one per OS
environment) that `serve` starts detached when `dashboard.enabled` is on. MCP
tools never depend on it. Its state lives under `<cache root>/dashboard/`:

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
  2 minutes, and one per agent runner conversation (below), written by the
  runner with `agent: true` (and `unlisted: true` when its profile keeps it
  off the index page) while the daemon runs. A session counts as live while
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
  intake result carries an `accepts: status, compact` line naming the
  optional event kinds this ws-mcp takes; an adapter sends a `status` event
  (the session's context, usage limits, and cost for the page's status band)
  or a `compact` event (a lead compaction's `start`, then its `end` or
  `failed`, with an `auto` or `manual` trigger) only while the latest result
  names that kind, since an older ws-mcp skips the kind per event, and the
  "dashboard off" result names none. A running compaction is the lead's
  `compacting` phase in the snapshot; its end or failure is a `compact` ring
  entry, and a reset or the next `turn_start` clears a compaction whose end
  never came. The snapshot's `repo` fields (the
  root's folder, branch, upstream, ahead and behind, and the counts of
  modified, staged, and untracked working-tree entries) come from ws-mcp's own
  `rev-parse`, `symbolic-ref`, `rev-list --left-right --count`, and
  `status --porcelain` reads, cached per session and refreshed at most once
  per heartbeat interval from the writer's timer, never on an intake call; the
  snapshot write that finds the cache missing or stale (a session's first,
  then at most one per heartbeat interval, change or heartbeat alike) waits
  for the refresh, up to its 5 s timeout. There is no automatic fetch, so
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
  `kinds` value), never as `board`.
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
project}`; the daemon also re-reads every session every 30 seconds.

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
below a 360px band only the limit under more pressure stays.

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
copies. A file outside the root offers the copies only, except an image
written as an absolute path in a lead reply or a board item field (not a
topic body, not the file view). A board image inside
the root loads through `GET /s/<reply-id>/image?path=<root-relative path>`,
which serves only an image-extension file under the same containment, and
any other image renders as its path, with that one exception: an absolute
image path outside the root (not a UNC or device path, image extension on
the name and on the file its symlinks resolve to) renders inline on the
image route and links through it (`data-out`), and the route serves it only
to a viewer on this machine (404 to a LAN viewer, token or not, and to a
request whose `Sec-Fetch-Site` is present and not `same-origin`), with
`Cross-Origin-Resource-Policy: same-origin`. Where such an image fails to
load (a LAN viewer, a missing file), the page puts its alt text and a
copy-only path link in its place, and its link offers the copies only; for
a viewer on this machine its menu offers open in new tab and open in native
app, not open in system browser. On a served page each topic body on the board
renders in its own sandboxed `srcdoc` frame (`allow-scripts allow-popups
allow-popups-to-escape-sandbox`, an opaque origin that sends no cookie), so
its images are inlined as `data:` URLs up to 2 MiB under the same containment
(`wsdashboard.ImageDataURL`); a larger image falls back to the image route,
which a LAN viewer's frame cannot load. The served board renders the 30 most
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
composer sends: `POST /s/<reply-id>/message {text, chips, images, refs,
refNums}` holds the message in the session's inbox (merged into a still-held
one that is not steered). `images` are new uploads `{name, data, n}` (base64,
sniffed as PNG/JPEG/GIF/WebP, at most 8 per held message), `refs` name files
a cancel returned, and `refNums` numbers them; the daemon writes the uploads
into the inbox's `attachments/` directory. The text names image `n` with an
`[Image #n]` marker, and the delivered message is the text, one `Image #n:
<absolute path>` line per image, then the board answers; a send that merges renumbers its
images and markers to continue the held message's, under the daemon's inbox
lock. The page still reads the older `[image: <path>]` lines. `POST
/s/<reply-id>/cancel {id}` takes a held one back, answering `{text, chips,
images, nums}`, and `POST
/s/<reply-id>/steer {id}` promotes a held one to steering, answering the
rewritten entry's new `{id}` (409 once the session has received it). The
adapter claims a steered message at the session's next tool boundary instead
of at the turn end. `POST /s/<reply-id>/interrupt` (body ignored) takes back
every held message, steered or not, and writes an interrupt entry in one step
under the inbox lock, answering `{held: [{id, text, chips, images, nums}]}`
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
links and in-page fragments become anchors, tags stay text), `POST
/tracker/stores/<id>/dispatch {events}` (origin forced
to `human`, `"$N"` names the batch's Nth event; a refusal is 409 with
`refusal {index, type, reason}`), `POST .../check` and `.../sync` (a report; an
unreachable remote is `offline: true`, not an error), `POST .../seen {badge,
read}` (204), `GET`/`POST /tracker/profiles`, and `POST /tracker/register`.
An unknown store id is 404. Every POST requires an `Origin` equal to the
daemon's own and a body under 256 KiB, and runs to completion even when the
page closes mid-request. Every string a route returns has credentials
redacted. After each write the routes ran, and after each poll round, the
tracker revision bumps; both pages' SSE streams send `event: tracker` with
`{"rev": N}` when it differs from what that stream last sent (once on connect
too), and the page re-reads.

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
root, and `builtin:<name>` accepts only the embedded names. Each
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
and no mailbox identity.

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
and `tk:register` and `tk:sync` stay refused; every other spawned key is
refused as before. `tracker.show {item}` is that conversation's card call:
an item of its own store answers `card <store> <alias>` plus the mini-card
fields, which the chat draws as a card; another store's item is answered as
text; a lead gets a refusal. `git.search` runs `log`, `show`, `grep`,
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
and a `serve` whose global value differs from a same-version daemon's mode
replaces it through the normal handoff.

At an equal version the newer build wins: `/health` reports `build_time` (the
ws-mcp executable's mtime in Unix milliseconds, read from the original binary
and passed to the daemon as `dashboard --build-time`), and a `serve` whose
build time is newer than the daemon's (a daemon that omits it counts as older)
replaces it; an older or unknown build time never does, so builds of one
version side by side leave each other's newer daemon alone. A rebuilt dev
binary therefore replaces the running daemon at the next `serve`. To force a
restart anyway, `POST /shutdown` with `Authorization: Bearer <token from
endpoint.json>`; the next `serve` starts a new one.

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
| `WS_MCP_RELEASE_TAG` | Override the release tag from `runtime.json`, for example `v0.48.12`. |
| `WS_MCP_RELEASE_BASE_URL` | Override the full release asset base URL; useful for local file or HTTP smoke tests. |
| `WS_MCP_LAUNCHER_DEBUG` | Print launcher diagnostics to stderr when set to `1`. |
| `WS_MCP_PROJECT_ROOT` | Project root default for root-aware tools and CLI commands when no higher-priority root exists. |
| `WS_MCP_DASHBOARD_DEV_SOURCE` | Read by `ws-mcp dashboard`: the `agents-plugin-tool` source root the daemon serves dashboard and whiteboard page assets from instead of the embedded copies (see Dashboard daemon, Dev assets). Unset is off. |
| `WS_MCP_NO_AGENT` | Product-mode gate for agentless distributions such as wsflow. |
| `WS_MCP_NO_DASHBOARD` | `1`/`true`/`yes`/`on` turns the dashboard off for this ws-mcp whatever `dashboard.enabled` resolves: no daemon start, no session registration, no prune. The agent runner sets it on the ws-mcp it attaches to a conversation. |
| `WS_MCP_ALLOWED_TOOLS` | Comma list of ws tool names; when set, `tools/list` advertises and `tools/call` accepts only these. The agent runner sets it to a profile's ws tools. `tracker.show` and `git.search` are listed and callable only when it names them. |
| `WS_MCP_NAMESPACE` | User-facing MCP namespace text override; empty or unset defaults to `ws`. |
| `WS_MCP_SETUP_TOOL` | Advertised setup tool name override; empty or unset defaults to `ws.setup`. |
| `WS_MCP_CONFIG_MANIFESTS` | Adapter config key manifests, separated by the platform path-list separator (`:` POSIX, `;` Windows); `;` is accepted on every platform, which the static launcher configs use. Declared keys become `config.tune`-writable, cataloged by `config.list`, and typed in `config.get`; a rejected manifest is logged to stderr and listed by `config.list`. The ws and wsflow packages each ship their own `config-manifest.json` (namespace `claude.`; the same keys, differing only in `default`, pinned by `TestShippedClaudeManifestsDifferOnlyInDefaults`) and set it in both launch configs: `.mcp.json` (Codex) as `./config-manifest.json`, resolved against `cwd: "."` (the plugin root), and `.claude-plugin/plugin.json` as `${CLAUDE_PLUGIN_ROOT}/config-manifest.json`. Each package (ws, wsflow, Pi) also lists its `dashboard-config-manifest.json` (namespace `dashboard.`: `dashboard.enabled`, default on in ws and Pi, off in wsflow; `dashboard.port`, default 29417; `dashboard.lan_enabled`, global-only and default off in every package) beside its own manifest; the Pi bridge joins the two with the platform separator. A path that does not resolve fails nothing: the keys stay undeclared, so check `config.list` after changing either. |
| `WS_MCP_CLI_NAME` | Command name that printed agent-runnable CLI commands (the rendered `mailbox wait` command and its re-arm line) start with; empty or unset uses the invoked binary path (`os.Args[0]`). Set by the `ws-cli`/`wsflow-cli` shims and by the Claude manifests' `mcpServers.<name>.env`, where plugin `bin/` is on the agent's Bash PATH; left unset for Codex, which does not put plugin `bin/` on PATH. |

Launcher diagnostics must go to stderr. Stdout belongs to the MCP JSON-RPC
stream until the launcher execs the repaired runtime.

## Runtime Repair And Drift

The launcher reads plugin-local `runtime.json`, repairs a missing or
incompatible runtime binary, then execs `ws-mcp`.

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
plus both `ws-pi-bridge` `package.json` versions). Development binaries such as `0.48.12-dev` satisfy the
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

Local development has one repository-specific repair exception. When the
installed plugin path is under `~/.codex/plugins/cache/kang-sw-devenv/<ws|wsflow>/`
or `~/.claude/plugins/cache/kang-sw-devenv/<ws|wsflow>/` and the installed cache
contains a valid `.local-devenv-runtime` contract, the launcher forces local
runtime repair before accepting an already compatible cache-local binary. Both
the Codex and Claude plugin caches are recognized so the same source-build
dogfood loop works on either host.

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
scripts/build-release-assets.sh 0.48.12-dev
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

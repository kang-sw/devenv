---
title: "Dashboard search drawer with a read-only answer agent"
related:
  261114-feat-ws-dashboard-unified-search-palette: "the Ctrl+K palette this ticket moves into a drawer; its decisions stand unless superseded below"
  261114-bug-ws-dashboard-search-palette-minors: "open palette minors; two are stacking issues the drawer may change"
---

# Dashboard search drawer with a read-only answer agent

## Background

The ws dashboard's Ctrl+K search palette (shipped by
`261114-feat-ws-dashboard-unified-search-palette`) is a centered modal
`<dialog>` built by `tracker.js`. The owner decided, relayed by MAIN on
2026-11-15 at 01:12Z, to (1) move the search UI into a drawer like the
panels/git/tracker drawers, and (2) let plain Enter ask a read-only answer
agent about the current session's whiteboard and project files, with the
answer streamed into the drawer.

Evidence below was taken read-only on 2026-11-15 at develop `1b815bf67`;
paths are relative to `agents-plugin-tool/internal/`.

### Current state

- **Palette.** `#tkqpop` is a modal `<dialog>` built in
  `wsdashboard/assets/tracker.js:6736-6745` and centered by CSS
  (`wsdashboard/assets/tracker.css:79-88`). `qOpen` (`tracker.js:6483-6493`)
  calls `showModal`, restores the query and resets the slot; `qKey`
  (`tracker.js:6555-6578`) opens the selected result on Enter and passes
  keep-open on Ctrl/Cmd+Enter; `qShortcut` (`tracker.js:6583-6590`) keeps the
  IME/editor guards. The first row is auto-selected (`qIdx` defaults to 0)
  and hover moves the selection (`tracker.js:6220-6227,6472-6481,7123-7126`).
  The query lives only in the in-page `qs` object (`tracker.js:6126`). The
  button `#tkqs` sits in `.top #qsslot` (`tracker.js:6736-6738`,
  `wsdashboard/assets/dashboard.tmpl:10,110-112`). `dashboard.js` mounts it
  through `tkMount` (`dashboard.js:5688-5695`) with the host hooks
  `paletteRows` (5580), `palettePick` (5633) and `paletteFiles` (5673);
  `tracker.js` stays host-agnostic (prior ticket, U:357-369).
- **Drawer frame.** One frame (`#drawer`, template `dashboard.tmpl:122-129`,
  handle `#drhandle`) shows one sheet at a time: `drMode` is `"panels"` or
  `"git"` (`dashboard.js:3374-3457`). The hamburger handle is rightmost
  (`right: var(--dpad)`), the git handle immediately left of it
  (`dashboard.css:1139,1167-1175`); `drRoom` computes the offsets and the
  reserved header width (`dashboard.js:3537-3557`), and tracker handles
  reserve `--tkhw` (`dashboard.css:1439-1443`). Keys: `ws.dashboard.drawer`
  (panels open) and `ws.dashboard.drawer.h` (one height shared by panels,
  git and tracker drawers); the git sheet's open state is never kept
  (`dashboard.js:3385-3409,3598-3601,3650-3663,3686`).
- **Agent runner.** `wsagent.Profile` (`wsagent/profile.go:76-125`) has
  `backend` (`claude` | `pi`), `model`, `effort`, `tools`, `ws_tools`,
  `mcp_servers`, `plugins`, `skills`, `instructions`,
  `idle_timeout_seconds`, `list_in_index`, `lead`, `permission_mode`.
  Profiles resolve as whole values project → committed repo → global →
  builtin (`wsconfig/structured.go:22-58,68-100`); `agent.profile.<name>` is
  an existing dynamic config family, not a manifest field
  (`mcp/agent_profile.go:8-60`). Claude restricted launches pass
  `--tools <allowlist>`, `--permission-mode dontAsk`, `--allowedTools`,
  `--strict-mcp-config` and an init check that kills a child reporting extra
  tools (`wsagent/claude.go:132-317`); Pi launches pass `--tools <list>`,
  `--no-extensions`, `--no-context-files` and refuse extra reported tools
  (`wsagent/pi.go:154-287`). Builtins today: `tracker-support-claude`,
  `tracker-support-pi`, `pi-worker`, `pi-mini-lead`, `lead-claude`,
  `lead-pi` (`wsagent/builtin.go:17-173`). The daemon registers only the
  Claude and Pi backends and checks binaries with `exec.LookPath`
  (`cmd/ws-mcp/dashboard.go:257-281`, `wsagent/backend.go:427-480`);
  `Runner.Spawn` defaults an omitted profile to `pi-worker`
  (`wsagent/spawn.go:117-120`), while tracker support picks the first
  available harness, Claude before Pi, and answers 503 when neither exists
  (`wsagent/support.go:39-44,223-260,316-337`).
- **Process lifecycle.** A conversation keeps one sustained process; a
  finished turn arms an idle timer (default 10 min) instead of exiting
  (`wsagent/runner.go:2732-2781,3182-3186`). `CloseConversation` ends the
  process and keeps records (`wsagent/runner.go:933-972`); close escalates
  stdin-close → SIGTERM → kill at 3 s steps (`wsagent/backend.go:398-425`).
  No per-feature concurrency cap exists.
- **Whiteboard store.** The board lives at
  `<project>/ai-docs/whiteboards/<reply-id>/` (folder store, schema 2:
  topic `meta.json`, immutable `log.jsonl`, `panels/eN.json`;
  `wswhiteboard/folders.go:27-64,377-469`); reads may run recovery under
  the store lock (`folders.go:99-126`). At most 500 settled items are
  retained; evicted items are not queryable (`wswhiteboard/store.go:52-57`).
  MCP `whiteboard.query` refuses non-top-level sessions, so a spawned
  agent's key cannot read the viewed session's board
  (`mcp/agent_keys.go:27-29`, `mcp/server.go:731-758`).
- **Streaming.** SSE/WS region events `{name,hash,html}` with a 250 ms
  minimum interval (`wsdashboard/sse.go:11-45,615-635`). The tracker support
  drawer already streams an agent conversation into a drawer by subscribing
  to its `/events?regions=entries,convlive` (`dashboard.js:5859-5919`).
- **Local vs remote.** `localViewer` requires a loopback peer and a
  loopback Host (`wsdashboard/access.go:156-188`); execution-authority
  writes use `TrustedWrite` (local viewer plus exact own Origin; 404 remote,
  403 bad Origin; `access.go:162-175`). Generic non-lead agent routes are not
  loopback-only (`wsagent/http.go:45-59,120-148`), and shared region caches
  omit viewer class (`wsdashboard/stream_cache.go:33-41`).

## Decisions

Owner decisions (relayed by MAIN, 2026-11-15):

- **(852)=b: the search UI is a drawer.** A search handle sits immediately
  left of the hamburger (☰) in the top-right; the git handle shifts one slot
  left. The search UI lives in the drawer sheet; the centered modal
  `<dialog>` goes away. The drawer's open state and height are remembered
  per browser like `ws.dashboard.drawer*` (localStorage), together with the
  last query and the selected slot. Rejected: keeping the centered modal.
  This supersedes the prior ticket's rejection of localStorage query
  persistence (U:159-164).
- **(851)=a: keyboard contract.** No row is highlighted until the user moves
  with the arrow keys. Plain Enter asks the answer agent; Enter after an
  arrow selection opens that result; Cmd/Ctrl+Enter keep-open behavior for
  results is preserved. This supersedes the prior ticket's automatic first
  row selection and Enter-opens-result (U:263-295).
- **(848)=c: the answer agent is profile-based.** It runs through the
  existing daemon agent runner (`wsagent` Runner + Profile) under a builtin
  read-only profile that a project/repo/global `agent.profile.<name>` entry
  can override. It must work in downstream projects without Pi: the default
  must not require Pi. No new tool surface is invented here; the broader
  "ws-mcp provides the whole agent toolset" direction is a separate
  discussion and is not folded in.
- **(849)=a: read scope.** This session's whiteboard (including its full
  history) and files under the session's project root, tickets and docs
  included. No shell, no network tools, no writes, enforced by the
  profile's tool allowlist, not by prompt.
- **(850)=a: one-shot per Enter.** A fresh agent per question; its process
  is cleaned up after the answer. The drawer keeps the last few answers.

Announced defaults relayed by MAIN (no owner objection):

- Agent mode exists on the session page only.
- Agent mode is hidden on remote/LAN views (loopback only).
- The answer streams into the drawer through the existing SSE/WS region
  stream, with citation links to board items and file lines.

Defaults and technical calls (posted by MAIN on the owner board as
no-answer-needed, 2026-11-15; settled):

- **Full history** means every retained board entry: open and closed
  topics, the immutable `log.jsonl` entries and every retained panel
  version. Settled items the store already evicted past its 500 cap are not
  recoverable (`wswhiteboard/store.go:52-57`).
- **Harness default.** The builtin default picks the first available
  harness, Claude before Pi, following the tracker support precedent
  (`wsagent/support.go:39-44,316-337`); a Pi-less downstream with the Claude
  CLI gets Claude. With neither binary on PATH the answer request returns
  503 and the drawer says no agent harness is available.
- **Loopback enforced server-side.** Answer creation uses `TrustedWrite`
  (404 for remote viewers, 403 for a bad Origin); answer reads and streams
  are refused to non-local viewers; answers never enter a region shared with
  remote viewers (`access.go:162-175`, `wsagent/http.go:45-59`,
  `stream_cache.go:33-41`). Hiding the UI alone is not the gate.
- **Two builtin profiles**, `search-answer-claude` (tools `Read`, `Grep`,
  `Glob`) and `search-answer-pi` (tools `read`, `grep`, `find`, `ls`), each
  with no `ws_tools`, MCP servers, plugins or skills, not lead, and empty
  model/effort like the other builtins. Rejected: one profile with a
  runtime-chosen backend, because a Profile names exactly one backend
  (`profile.go:239-247`); the pair mirrors `tracker-support-*`.
- **Board input is a per-question snapshot file.** The daemon renders the
  session's board from the authoritative folder store under its lock into a
  read-only file inside a per-question temp directory the agent may read,
  and the prompt names it. Rejected: granting MCP `whiteboard.query` to the
  agent key (changes an authorization contract, `server.go:731-758`, a
  cross-version surface); letting the agent read the raw folder store
  (bypasses the recovery boundary, `folders.go:99-126`).
- **One-shot lifecycle.** The answer service spawns a conversation, waits
  for its first turn to settle, then calls `CloseConversation` on success,
  failure, timeout or cancel; hard timeout 5 minutes; at most one in-flight
  answer per session page (a new Enter stops the running one) and at most 2
  answer processes daemon-wide (further requests get 429 and a drawer
  message). Rejected: the runner's sustained process with its 10-minute idle
  timer (`runner.go:2732-2781`), which has no cap.
- **Citations.** The answer prompt asks for `[[board:<item id>]]` and
  `[[file:<root-relative path>:<line>]]` markers; the drawer validates each
  against the session's board and the project root and renders valid ones as
  links dispatched through `palettePick` and `WsPreview.open(path,{line})`
  (`dashboard.js:5633-5663,5681`, `preview.js:237-249`); invalid ones stay
  plain text. Rejected: free-form markdown links the page cannot validate.
- **Answer history.** The drawer keeps the last 5 answers (question, final
  rendered answer, time) per session in localStorage keyed by session
  reply-id. Rejected: daemon-side retention (new server state for answers
  only the local browser sees).
- **Drawer mode.** Search is a third sheet mode of the existing drawer frame
  (`drMode` `"search"` beside `"panels"`/`"git"`), sharing the one height
  key `ws.dashboard.drawer.h`, with its own open-state key
  `ws.dashboard.drawer.search` and keys for the last query and selected
  slot; Ctrl+K opens the search sheet and focuses its input. Rejected: a
  separate drawer frame, which would duplicate `drRoom` geometry and break
  the one-height rule (`dashboard.js:3385-3394,3537-3557`).
- **Keyboard details.** A query edit, slot switch or rerank clears the arrow
  selection; hover styles a row but never selects it; a mouse click on a row
  still opens it; Enter on an empty query does nothing. Rejected: keeping
  hover-select, which would arm Enter-opens without an arrow key and
  contradict (851).
- **Compatibility: T0.** Two new builtin profiles in the existing Profile
  shape, new loopback-only HTTP routes and new browser localStorage keys; no
  MCP tool or argument, `/mcp` hello, store shape, tracker event or manifest
  field changes (`agent.profile.<name>` is an existing dynamic family,
  `mcp/agent_profile.go:8-60`).
- **Drifted prose.** Update the stale "No profile is built in"
  config-family prose in `mcp/agent_profile.go:8-12` on contact (AGENTS.md
  "Update drifted docs on contact").

## Constraints

- `tracker.js` stays host-agnostic: drawer hosting, filesystem, preview,
  agent and citation integration live in `dashboard.js` mount hooks
  (prior ticket, U:357-369).
- Preserve from the prior ticket unless superseded above: the five sources,
  the jump/command strip, project-root/gitignore scope, local-only
  depth-one absolute-path mode, async cancellation/generation checks and
  stable selection across batches, IME/editor guards, Esc clears then
  closes, shell search forwarding, per-source Cmd/Ctrl+Enter keep-open
  (U:135-236,263-301,391-426).
- Never touch the shared daemon or port 29417; browser and agent tests run
  against an isolated daemon fixture with its own `WS_CACHE_HOME` (the
  existing `scripts/palette-headless.mjs` pattern). Agent backends in tests
  are fakes; no test calls a real model.
- Never run the whole `./internal/wsdashboard` Go test package; use
  `go test -run`. Offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.
  End every run with a scan for processes it started and stop only those.
- Before editing, read the manuals the AGENTS.md Implementation Conventions
  table binds to the touched paths:
  `ai-docs/manuals/shipped-surface-boundary.md` (all of
  `agents-plugin-tool/`), `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`), and
  `ai-docs/manuals/release-compatibility.md`
  (`agents-plugin-tool/internal/`).
- Out of scope: a new MCP tool or a change to `whiteboard.query`
  authorization; a Codex backend; the "ws-mcp provides the whole agent
  toolset" direction.

## Open Decision Queue

Last ID used: (866) (MAIN board IDs)

(864) [open] [policy] What an override of the answer profile may change.
- Context: an `agent.profile.search-answer-*` override replaces the whole builtin value (`structured.go:22-58`), so as written it could add `Bash`, `Edit`, MCP servers, plugins or lead mode and silently break (849)'s "no shell, no network, no writes".
- Alternative a: the answer service enforces a ceiling: an override may change `backend`, `model`, `effort`, `instructions` and may narrow `tools`; any tool outside the read set, any `ws_tools`, `mcp_servers`, `plugins`, `skills`, `lead` or a non-default `permission_mode` makes the request fail with a clear error naming the override.
- Alternative b: overrides are trusted as written (the user configured them); document that broadening breaks the read-only guarantee.
> Recommend a: (849) says the read-only scope is enforced by the allowlist, which only holds if an override cannot widen it.

(865) [open] [policy] How strictly "files under the project root" is enforced per backend.
- Context: the tool allowlist removes shell, write and network tools, but neither backend confines native read tools to a directory; `cwd` alone does not stop an absolute path (`claude.go:339-343`, `pi.go:427-429`). Claude permission rules can scope reads by path (for example `Read(//<root>/**)` under `dontAsk`, which also governs Grep/Glob - INFERRED, to be verified by the worker); Pi rejects permission-rule entries (`pi.go:381-390`).
- Alternative a: Claude answers get path-scoped read rules for the project root and the snapshot directory (hard boundary); Pi answers run with `cwd` at the root and read-only tools, with the root documented as a soft boundary.
- Alternative b: hard boundary or nothing: refuse Pi-backed answer profiles; only Claude answers.
- Alternative c: soft boundary for both (tools only, `cwd` at root).
> Recommend a: hard where the backend can enforce it, and Pi-only projects still work; the agent is local-only and read-only either way.

(866) [open] [policy] What plain Enter does where agent mode is absent (non-session pages, remote/LAN views).
- Context: (851) makes plain Enter ask the agent; agent mode exists only on local session pages.
- Alternative a: plain Enter with no arrow selection does nothing there (a hint says to use the arrows); the keyboard contract stays the same everywhere.
- Alternative b: plain Enter opens the first result there (the old behavior).
> Recommend a: one contract on every page; arrows plus Enter still open results.

## Phases

### Phase 1: Read-only answer service

Add the builtin answer profiles, the profile resolution and override ceiling,
the per-question board snapshot, the one-shot lifecycle with timeout and
admission limits, and loopback-only HTTP routes to start an answer for a
session and to stream it, following the decisions above. Agent mode reports
itself unavailable (503) when no harness exists.

Verification: Go tests with fake backends (`-run` only) for harness
selection and the missing-harness 503; the exact Claude and Pi launch flags
of the answer profiles (no shell/write/network tools; path rules where
settled); override ceiling acceptance and refusal; snapshot content
covering open, closed and log/panel history; close on success, failure,
timeout and cancel with no surviving process; per-session supersede and the
daemon-wide cap; remote viewer 404 and bad Origin 403 on creation and on the
stream.

### Phase 2: Search drawer and keyboard contract

Move the palette from the modal `<dialog>` into the search sheet of the
drawer frame with its handle between git and hamburger, persist open state,
height, last query and slot, apply the keyboard contract, and wire plain
Enter to the Phase 1 service on local session pages: stream the answer into
the drawer, render validated citations as links, keep the last answers, and
show the unavailable/limit messages.

Verification: extend `wsdashboard/search_palette_test.go`,
`wsdashboard/testdata/search-palette.test.cjs`,
`wsdashboard/drawer_height_test.go` and `scripts/palette-headless.mjs`
(isolated daemon, fake agent backend): handle order and header geometry at
narrow and wide widths with the git and tracker handles present; no initial
highlight; plain Enter asks, arrow+Enter opens, Cmd/Ctrl+Enter keeps open,
hover does not arm; reload restores open state, height, query and slot;
answer streaming, citation link dispatch and invalid-citation fallback;
agent mode absent on a remote view and on non-session pages; light and dark
screenshots at 1280 and ≤640 px. Recheck the two stacking minors of
`261114-bug-ws-dashboard-search-palette-minors` (toast behind the modal,
shell search-button overlap) and record whether the drawer changed them.

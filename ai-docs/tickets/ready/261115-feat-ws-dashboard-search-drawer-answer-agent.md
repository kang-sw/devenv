---
title: "Dashboard search drawer with a read-only answer agent"
related:
  261114-feat-ws-dashboard-unified-search-palette: "the Ctrl+K palette this ticket moves into a drawer; its decisions stand unless superseded below"
  261114-bug-ws-dashboard-search-palette-minors: "open palette minors; two are stacking issues the drawer may change"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0d53b245184af115
sage-review-completeness-reviewed: 0d53b245184af115
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
paths are relative to `agents-plugin-tool/internal/`, except
`cmd/ws-mcp/dashboard.go` (relative to `agents-plugin-tool/`),
`scripts/palette-headless.mjs` (relative to the repository root), and
explicitly rooted paths (`agents-plugin-tool/cmd/ws-mcp/dashboard.go`,
`scripts/palette-headless.mjs`).

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
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L10-L10`,
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L106-L108`). `dashboard.js` mounts it
  through `tkMount` (`dashboard.js:5688-5695`) with the host hooks
  `paletteRows` (5580), `palettePick` (5633) and `paletteFiles` (5673);
  `tracker.js` stays host-agnostic (prior ticket, U:357-369).
- **Drawer frame.** One frame (`#drawer`, template
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L124-L131`,
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
  persistence
  (`ai-docs/tickets/.done/261114-feat-ws-dashboard-unified-search-palette.md#L287-L295`).
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

Owner answers to the queue (relayed by MAIN, 2026-11-15 at 01:41Z):

- **(864)=a: overrides are capped at the read-only ceiling.** An
  `agent.profile.search-answer-*` override may change `backend`, `model`,
  `effort` and `instructions`, and may narrow `tools`. Any tool outside the
  read set, any `ws_tools`, `mcp_servers`, `plugins` or `skills`, `lead`, or
  a non-default `permission_mode` makes the answer request fail with an
  error naming the override. Rejected: trusting overrides as written, which
  would let one silently void (849).
- **(865)=a: the project-root boundary is best effort, applied only where
  the harness supports it.** Owner memo: "네 best effort로 되는 도구만
  적용으로 한정하죠." Claude answers get path-scoped read rules for the
  project root and the per-question snapshot directory (for example
  `Read(//<root>/**)` under `dontAsk`); the worker verifies whether those
  rules also govern `Grep`/`Glob` and records the result. Pi answers run with
  `cwd` at the project root and read-only tools only, because Pi rejects
  permission-rule entries (`wsagent/pi.go:381-390`). The ticket, the code
  comment at the launch site and the profile description state that the root
  boundary is best effort, not a sandbox; the no-shell, no-write and
  no-network-tool allowlist stays hard on both backends. Rejected: refusing
  Pi-backed answers (Pi-only projects would lose the feature); a soft
  boundary on Claude too.
- **(866)=a: plain Enter does nothing where agent mode is absent**
  (non-session pages, remote/LAN views); a hint points to the arrow keys, so
  the keyboard contract is the same on every page. Rejected: Enter opens the
  first result there.

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

### Confirmed Phase 2 addendum - 2026-11-15

- **(926)=a: layout A.** The answer occupies the left column; the existing
  five result cells occupy the right. The approved idle, answering and answered
  dark mockups and answered light mockup were inspected at
  `/tmp/mk917-build/917-search-agent-{idle,answering,answered}-dark.png` and
  `/tmp/mk917-build/917-search-agent-answered-light.png`. Their selected layout A
  text source is preserved at
  [layout-a.css](../assets/261115-feat-ws-dashboard-search-drawer-answer-agent/layout-a.css),
  extracted from that directory's `inject.css`; no screenshot matrix is tracked.
- **(927)=a: owner refinement (verbatim owner-supplied words):**
  > 이건 "micro compact" 같은 걸 두게 하면 어떨까 싶어요. 그러니까, 바로 앞 질문과 답변으로 계속 맥락이 유실되니까, 애초에 에이전트가 답변할 때 "맥락" + "답변"을 같이 넘기게 하는 식으로?
- **(929)=ok: confirmed interpretation.** Every answer returns two parts:
  a short rolling context summary of the conversation so far and the answer.
  Cap the context at approximately 1–2k characters. Follow-ups send the latest
  summary plus the new question, not only the immediately previous Q&A. The
  drawer client retains the summary; resetting the drawer clears it. Cancelled,
  superseded or still-streaming answers must not replace current context.
  The existing last-five-answer browser history remains in force. This
  supersedes the mockup's previous-Q&A-only follow-up wording; the one-shot
  process lifecycle and read-only authority remain unchanged.
- **Compatibility remains T0:** context is an optional additive request/payload
  field. Legacy requests and payloads without it continue to work; no existing
  process lifecycle, authorization, protocol or API meaning is changed.
- **Confirmed UI-fast execution boundary.** Implement Phase 2, then run only
  relevant scoped `wsdashboard -run` tests and `wsi18n`, offline with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Use private fake-backend fixtures;
  no real provider, Pi launch, shared daemon, runtime swap or port 29417.
  This supersedes Phase 2's old screenshot/theme/width matrix and broad browser
  verification expectations (including the corresponding Route Facts risk
  evidence); owner on-screen feedback after lead merge/swap is acceptance.
  One bounded logic-lite pass is allowed where the existing review allocation
  requires it. Preserve the Phase 1 plan and Results. The lead owns the HEAD
  handoff, integration and ticket closure; this invocation does not close it.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
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

## Prior Decisions

- 261114-feat-ws-dashboard-unified-search-palette (2026-11-14, commit): "the owner's standing rule forbids touching the shared daemon, so the script serves its own isolated daemon instead" — bearing: supports
- 4e1e5d9c (2026-11-13, commit): "Enforce subsets against persisted launched definitions, including MCP server configuration, at creation and restart; use the same checked definition to launch and reattach." — bearing: constrains
- 261113-feat-ws-dashboard-iframe-shell (2026-11-13, Decisions): "Shell contents: sidebar plus top header (global search, notifications, session title/badges); the agent panel stays in the frame." — bearing: constrains
- 2390a0f7 (2026-11-12, commit): "drApply drives visibility (drFoldShow(drOpen ? drMode : "")), so every path that ends in drApply follows" — bearing: constrains
- 88c5ea70 (2026-11-12, commit): "tracker.js stays host-neutral: a new opts.drawerHeight {get, set} pair carries the host's height; the dashboard passes drH / drSetH." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-11, Result): "The search stops when its request ends, so a search-as-you-type client's abort frees the daemon; a 10-second deadline also ends it with `more`." — bearing: supports
- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Result): "a user definition of the same name replaces one whole" — bearing: supports
- 134c1565 (2026-11-08, commit): "The git drawer reuses the panels drawer's frame with a mode switch (drMode "panels" | "git") instead of a second drawer" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/{builtin.go,profile.go,runner.go}, agents-plugin-tool/internal/wsdashboard/assets/{tracker.js,tracker.css,dashboard.js,dashboard.css,dashboard.tmpl}, agents-plugin-tool/internal/mcp/agent_profile.go; scripts/palette-headless.mjs |
| scope.surface | cross-module | Phase 1 joins wsagent profiles/lifecycle and the authoritative wswhiteboard store to new dashboard HTTP routes; Phase 2 adds host integration through dashboard.js mount hooks |
| scope.new_public_symbol | yes | New agent.profile.search-answer-claude and agent.profile.search-answer-pi builtin keys and loopback-only answer HTTP routes; route names are not specified |
| scope.new_type_contract | yes | New answer-start/stream HTTP contract, capped profile validation, board/file citation markers and host mount hooks; no existing MCP signature changes |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/search_palette_test.go, testdata/search-palette.test.cjs, drawer_height_test.go; scripts/palette-headless.mjs; existing wsagent backend tests |
| complexity.reuse_points | confirmed | Read Runner.Spawn and CloseConversation, Profile/BuiltinProfiles, support harness selection, FolderStore.Open/readState, TrustedWrite, supOpen region streaming, drRoom/drSetH and WsPreview.open |
| complexity.side_effect_risk | high | Spawns and terminates harness processes, reads retained board history and project files, and changes shared drawer geometry, keyboard handling and browser persistence |
| risk.correctness | high | First-turn settlement, supersede/cancel/timeout cleanup, daemon-wide admission, snapshot consistency and async keyboard selection must agree across service and UI |
| risk.fit | high | Generic runner routes are not local-only and shared page caches have no viewer class; the answer service must preserve local-only visibility and tracker host neutrality (wsagent/http.go#L120-L148; wsdashboard/stream_cache.go#L33-L41) |
| risk.test | high | Fake backends and source-extraction node tests cannot establish actual Claude Grep/Glob path-rule enforcement or browser stacking/focus; Phase 2 also requires isolated theme/width captures |
| risk.security_or_contract | high | Overrides must not widen the read-only tool ceiling; board/file answers and generic runner records must not leak through remote-readable routes, while root confinement is explicitly best effort |

## Phases

### Phase 1: Read-only answer service

Add the builtin answer profiles, the profile resolution and override ceiling,
the per-question board snapshot, the one-shot lifecycle with timeout and
admission limits, and loopback-only HTTP routes to start an answer for a
session and to stream it, following the decisions above. Agent mode reports
itself unavailable (503) when no harness exists.

Verification: Go tests with fake backends (`-run` only) for harness
selection and the missing-harness 503; the exact Claude and Pi launch flags
of the answer profiles (no shell/write/network tools on either backend;
Claude path-scoped read rules for the project root and the snapshot
directory; Pi `cwd` at the project root), with the observed result of
whether the Claude rules govern `Grep`/`Glob` recorded in the phase's
`### Result` and the best-effort boundary stated at the launch site and in
the profile descriptions; override ceiling acceptance of backend, model,
effort, instructions and narrowed tools, and refusal of every wider field
with an error naming the override; snapshot content
covering open, closed and log/panel history; close on success, failure,
timeout and cancel with no surviving process; per-session supersede and the
daemon-wide cap; remote viewer 404 and bad Origin 403 on creation and on the
stream.

#### Checkpoint - 2026-11-15 (unfinished)

Phase 1 source and fake-backend tests are checkpointed on
`impl/exec4/search-drawer-impl/boil-rants-plant`; this is not a Result and
Phase 1 remains unfinished. Phase 2 has not started.

- Added both builtin profiles, capped project-root profile resolution,
  private per-question Runner instances with frozen checked definitions,
  read-only board snapshot files, supersede/cancel/timeout cleanup, the
  two-process admission cap, and `/search-answers/<session>` routes.
  Private runner records live outside the public session registry so generic
  agent reads and shared page streams cannot reveal answer content. The
  current answer alone stays in memory; browser history belongs to Phase 2.
- The creation and cancel routes require TrustedWrite. State and SSE reads
  require loopback and reject a supplied bad Origin; absent Origin is allowed
  for same-origin EventSource compatibility. SSE emits the existing
  `region {name,hash,html}` shape with escaped answer text and status metadata,
  outside the shared region cache.
- Offline targeted wsagent answer/builtin tests passed in the final invocation
  (`go test ./internal/wsagent ./internal/mcp -run
  '^(TestSearchAnswer|TestDashboardSearchAnswer|TestBuiltin|TestAgentProfile)'
  -count=1 -timeout=90s`), but the combined invocation failed in
  `TestDashboardSearchAnswerSnapshotRetainedHistory`: its assertion expects
  exactly three topic entries, incorrectly excluding generated system entries
  that the snapshot correctly retains. The earlier invocation included this
  same assertion but its output was truncated at a separate builtin-count
  failure. Re-running it before diagnosing the snapshot assertion constitutes
  non-convergence, stop (f); no further fix or retry was attempted.
- Initial test-only compile mismatches (Config.Path and Event.Error), a fake
  crash-liveness assertion (fake exit closes Events without setting the Close
  flag), and the additive builtin count assertion were corrected. No failing
  implementation contract was weakened.
- Required baselines were run and match only the owner-declared failures:
  `go test ./internal/wsi18n -count=1` lacks js.card.width.extend/normal in
  the nine non-English tables; `go test ./internal/wsdashboard -run
  '^TestOutsidePreview' -count=1 -timeout=60s` fails only the known
  TestOutsidePreviewCardRemintsOnEveryLoad removeAttribute TypeError.
  Neither baseline was modified. All Go commands used GOPROXY=off,
  GOSUMDB=off, GOTOOLCHAIN=local.
- Logs: `/tmp/equity-tableful-absinthe-tests/{service,i18n,outside-preview}.log`.
  No i18n keys were added. The isolated daemon test bound an OS-selected port;
  no shared daemon, port 29417, or real model was used.
- Remaining acceptance: correct the snapshot test to count/filter authored
  entries while also checking retained system history; run broader neighbour
  and race tests and the command build; investigate actual Claude Grep/Glob
  path-rule enforcement (only supplied launch flags are established so far).
  The launch comment and builtin descriptions state best effort, not a
  sandbox. Independent review is deferred to the lead under the run's explicit
  adaptation; no review verdict is claimed.

### Result (5f281584a) - 2026-11-15

Phase 1 landed in `87f953e9c` with the owner-approved test correction
`5f281584a` ((882)=a). The snapshot test now counts three non-system entries
(the agent log and both panel versions); all snapshot content assertions are
unchanged. The four generated system entries remain in the snapshot. No
product-code change was needed on resume.

- Delivered both builtin profiles, root-layer resolution and the read-only
  override ceiling; private per-question Runner instances freeze the checked
  profile and keep their records out of generic agent/session routes and
  shared region caches. Board snapshots include retained open/closed topics,
  logs and panel history under the authoritative store's lock.
- Local-only `/search-answers/<session>` creation returns the current answer;
  `/<answer>` reads it, `/<answer>/events` streams escaped `answer` regions,
  and `/<answer>/cancel` cancels it. Creation/cancel require TrustedWrite;
  reads/streams reject remote viewers and a supplied bad Origin, while absent
  Origin is permitted for same-origin EventSource. Process cleanup precedes
  terminal status on settle, failure, cancel, timeout and supersede. At most
  two questions run daemon-wide, with one current question per session.
- Passing acceptance (all offline): targeted answer/builtin/snapshot tests;
  the same targeted contracts with `-race`; neighbouring runner lifecycle,
  profile parsing/resolution and Claude/Pi subprocess stand-ins; race-enabled
  MCP profile catalog, session source, transient-read and lock contracts;
  command startup and isolated daemon lock/shutdown tests; and
  `go build -o /tmp/scouts-sandbag-viselike-phase1/ws-mcp ./cmd/ws-mcp`.
  `git diff --check` and the repaired test's gofmt check were clean.
- `go test -race ./internal/wsdashboard -run
  '^Test(SecondDaemonIsRefused|EndpointFileRoundTrip|HealthAnswers|ShutdownRequires|NonLoopbackHost|CloseReleases|TakenPortFalls|AgentHostStarts|SupportHandlerMounted|LANModeGuardsEveryRoute|OutsidePreview)'
  -count=1 -timeout=90s` failed only the declared base
  `TestOutsidePreviewCardRemintsOnEveryLoad` removeAttribute TypeError.
  `go test ./internal/wsi18n -count=1` failed only the declared
  js.card.width.extend/normal omissions in the nine non-English tables.
  These failures remain untouched; no new i18n keys were added.
- Full logs are under `/tmp/scouts-sandbag-viselike-phase1/`:
  `race.log`, `agent-neighbours.log`, `mcp-neighbours.log`,
  `dashboard-neighbours.log`, `command-neighbours.log`, `i18n.log`,
  and `build.log`. Commands used GOPROXY=off, GOSUMDB=off,
  GOTOOLCHAIN=local; runtime fixtures used private cache/config homes and
  OS-selected ports. No shared daemon, port 29417, real model, merge or push
  was used. The owned-process scan found no matching processes remaining.
- Boundary observation: tests establish scoped Claude Read/Grep/Glob launch
  rules and Pi read-only tools at project cwd, not actual Claude Grep/Glob
  permission enforcement. That runtime property remains unverified; the
  launch comment and builtin descriptions explicitly say best effort, not a
  sandbox. No stronger confinement claim is made.
- Independent review is deferred to the lead by the explicit run adaptation;
  this Result claims no review verdict. Phase 2 remains unfinished, so the
  ticket stays in ready/.

#### Edition (54260ec29) - 2026-11-15

Fixed exactly the Critical and Important findings from the independent lite
review of `bd316e89a..92b96d89f`:

- **Critical — readable credentials:** `question/snapshot/board.json` is now
  the only material in the extra read grant. Read/Grep/Glob permission entries
  name only the project root and that snapshot subtree, while credential-bearing
  runner state stays in the sibling `question/runner/` tree. Terminal cleanup
  still removes their common question directory. The regression launches a
  question, checks exact permission entries, scans both granted trees for
  `agent.json` and the minted session key, and checks deletion of both siblings.
- **Important — ineffective freeze test:** an internal test-only hook gates
  execution after the ceiling check but before the private runner resolves.
  While gated, the test replaces the winning repo override with a same-backend
  `read,bash` definition, releases the gate, and requires the checked `read`
  definition at launch. The previous lower-priority global edit is gone.
- Negative control: temporarily omitting the fixed-profile assignment made
  `TestSearchAnswerOverrideLaunchFrozen` fail with
  `tools=[read bash] backend=pi model=custom effort=low`. Freezing was restored
  before final acceptance; no mutation probe remains in the committed source.
- Passing final verification from `agents-plugin-tool/`:
  `go test -race ./internal/wsagent ./internal/mcp -run
  '^(TestSearchAnswer|TestDashboardSearchAnswer|TestBuiltin|TestAgentProfile|TestClaudeArgs|TestPiArgs|TestClaudeBackendStart$|TestPiBackendStart$|TestCloseEscalates$|TestFatalKillsTheChild$|TestExitedChildIsNotHeldByAGrandchildsStderr$)'
  -count=1`. This includes both new/revised regressions. Focused regression
  checks, `git diff --check`, and gofmt checks also passed.
- Full output: `/tmp/scouts-sandbag-viselike-reviewfix/{regressions,freeze-disabled,acceptance}.log`.
  All Go commands used GOPROXY=off, GOSUMDB=off, GOTOOLCHAIN=local and isolated
  cache/config homes. The owned-process scan found no matching test/answer
  processes. No shared daemon, port 29417, real model, full dashboard package,
  child agent, merge or push was used. No i18n keys changed.
- This is the existing Phase 1 review-fix round, not a new phase or review
  sweep. Frozen Result text and Phase 2 remain unchanged; the previously
  recorded best-effort Claude boundary limitation is not reclassified as proof.
- Lead review record: the independent lite delta review of
  `92b96d89f..414f63f48` was clean (both findings addressed, no new
  regression; race command re-run, pass). Owner MUST-check outcome, recorded
  under (865)=a: VERIFIED that Claude launches carry scoped
  `--allowedTools '<Tool>(//<abs>/**)'` entries for only the project root and
  the snapshot subtree, with `--tools Read,Grep,Glob --permission-mode
  dontAsk --setting-sources "" --strict-mcp-config` and no bare Read/Grep/Glob
  entry, and that the repository adds no per-call containment, traversal or
  symlink check. INFERRED, not verified: that the Claude CLI denies outside
  paths, `..` traversal and symlink escapes, and that Grep/Glob honor
  path-scoped entries; no real-model probe was run (MAIN, 2026-11-15). Pi
  keeps the accepted soft path boundary.

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
agent mode absent on a remote view and on non-session pages, where plain
Enter does nothing and the arrow-key hint shows; light and dark
screenshots at 1280 and ≤640 px. Recheck the two stacking minors of
`261114-bug-ws-dashboard-search-palette-minors` (toast behind the modal,
shell search-button overlap) and record whether the drawer changed them.

---
title: "ws tracker support-agent chat card: a daemon-hosted agent conversation inside the tracker drawer"
parent: 261107-epic-ws-personal-common-store
related:
  261108-feat-ws-daemon-agent-runner: prerequisite (agent launch, profiles, wslive output, REST control)
  261108-feat-ws-tracker-caller: prerequisite (the tracker MCP tools the agent calls)
  261108-feat-ws-tracker-drawer-ui: prerequisite of Phase 2 only (the drawer this card lives in; merged a6f0b0ab1)
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: bca264cac21c1781
sage-review-design-reviewed: bca264cac21c1781
completed: 2026-11-09
---

# ws tracker support-agent chat card: a daemon-hosted agent conversation inside the tracker drawer

## Background

The user (2026-11-08) wants a local support agent in the tracker: a chat
card in the drawer's cards layout where the user talks to a daemon-hosted
agent about the open store. The mockup already places it (board (300) a,
(303) ok, merged `1d7fbce0a`): a "Support agent" card with fixed height,
internal scroll, a disabled composer, and a sample conversation that
includes a tracker card reference. This child makes it real on top of the
agent runner (`261108-feat-ws-daemon-agent-runner`).

## Decisions

- **Placement** (board (303) ok, (304) a, (305) ok): the chat is one card in
  the drawer's cards layout, fixed height with internal scroll and the newest
  message at the bottom. It has no view-mode picker.
- **Reuse the dashboard conversation view** (board (302) ok): the agent's
  conversation is rendered by the existing conversation renderer from the
  runner's `wslive` session directory; no chat-only renderer. The renderer
  gains one entry kind, a tracker card reference: a mini card (title,
  status, priority, due, assignees) of one item from the store's view state
  that opens the item's thread sheet on click.
- **Reuse the dashboard composer without its bottom badges** (user, board
  topic): the composer is shared, but the usage and other marks under it are
  left out and replaced by a model and effort selection (a chip and a
  popover, not a modal: amended by (516) below).
- **Mockup scope stays placement only** (board (300) a): chat behaviour and
  the API are settled here, not in the mockup.
- **Prerequisites** (board (321) ok): blocked by the agent runner and by the
  tracker caller children - `261108-feat-ws-tracker-caller` (the tracker MCP
  tools the agent calls, (115)) and `261108-feat-ws-tracker-drawer-ui` (the
  drawer this card lives in), split by board (408) b.
- **One ongoing conversation per store** (board (322) a): kept across drawer
  opens and days, with a "new conversation" action that starts a fresh
  backend session. Rejected for now: a conversation list per store; a fresh
  conversation per drawer open.
- **Card references come from a designated tool call** (board (323) ok): the
  agent shows a card by calling a tracker tool (for example
  `tracker.show(item)`), and the renderer draws that call as the
  card-reference entry. Rejected: a text marker inside the reply, fragile to
  parse and to quote.
- **Store writes only on request** (board (320) a): the support agent writes
  to the open store only on the user's explicit request in the chat, as the
  viewer's own member identity with agent provenance (the `agent` chip); it
  never writes to another store, and cross-store search stays read-only
  (epic (114)/(122), (123) b). Rejected: read-only everywhere; automatic
  recording like a lead session's agent.
- **Read-only local git search** (user note on (320); board (324) ok): the
  agent may search the user's other local projects through one dedicated
  read-only tool the support profile allows - log, show, grep, blame, and
  ls-files over a repository path, argument-validated, with no
  write-capable options (`ls` is covered by ls-files, file contents by
  `show <rev>:<path>`). Rejected: Bash prefix rules such as
  `Bash(git log:*)`, which cannot block `git log --output=<file>` or
  `git diff --output`, are escaped by `git -C <repo>` and `cd <repo> &&`,
  and have no Pi equivalent (`--tools` has no per-command granularity).
- **Git search scope** (board (325) a): the tool reaches every project root
  the dashboard daemon already knows (roots that have run a ws session) plus
  extra roots from a config knob, and refuses any other path. Rejected: knob
  roots only; any git repository on the machine.
- **Ships its own support profile** (runner (329) a, (330) a): the runner
  ships no profile, so this ticket adds the support agent's
  `agent.profile.<name>` default (tools: the tracker tools of the caller
  child and the read-only git search tool) and creates its conversations
  with the store's local clone as the working root. "New conversation"
  ((322)) is the runner's `close` plus create (runner (328) a).

- **Backend first, card second** (board (440) a): Phase 1 builds the
  headless backend (support profile, read-only git search tool,
  `tracker.show`, per-store conversation lifecycle over the daemon) right
  after the tracker caller merges, in parallel with the drawer UI; Phase 2
  builds the chat card on that verified API once the drawer UI lands.
  Rejected: backend and card in one pass after the drawer UI; deferring the
  phase plan to the drawer UI start.
- **Plan now, implement after the runner live check** (board (441) a,
  refining (365) b): this phase plan is written now; implementation starts
  only after the user's live check of the agent runner and the round-2
  conversation view, since this card builds on both. Rejected: live check
  before any planning; implementing without the live check.
- **`tracker.show` belongs to this ticket** (board (442) ok): it is added
  here as a third tracker tool after the caller merges, not folded into the
  caller's in-flight Phase 2. Rejected: amending the caller's two-tool
  contract (Decision 9) mid-implementation.

- **Runner-only tool listing** (board (443) ok): the git search tool and
  `tracker.show` are ws-mcp tools listed only in a runner conversation whose
  profile allows them, never in an ordinary lead session's tool list; a lead
  gains nothing from them and an always-listed tool widens the agent-facing
  contract downstream. The listing mechanism is settled by (522) below.
  Rejected: a tool every session lists.
- **Phase 1 verification includes one live conversation** (board (444) ok):
  Go tests with a fake backend (git search argument refusals and path
  scope, `tracker.show` fields, conversation keep and "new conversation")
  plus one live claude conversation over the daemon REST that calls
  `tracker.query`, `tracker.show`, and git search against the live test
  store `kang-sw/ws-tracker-live-test`. Rejected: fake-backend tests only,
  since the runner's billing path and profile loading are proven only live.
- **Stays in `todo/` until the runner live check** (board (445) ok): promotion
  to `ready/` then runs fact population and the Sage gate.
- **Live-check gate waived** (user, 2026-11-08, board (515)): the user asked
  to start the support agent now, so Phase 1 no longer waits for the runner
  live check of (441) a and (445); Phase 1's own live conversation over the
  daemon REST ((444)) stands in for it. Promotion to `ready/` still runs fact
  population and the Sage gate.

- **Selection is a chip and a popover** (board (516), from drawer decision
  (472) b): the harness, model, and effort selection is a chip under the
  composer that opens a popover; modals stay reserved for the admin and
  profile dialogs. Rejected: a modal.
- **Harness switch starts a new conversation** (board (517)): switching
  Claude <-> Pi closes the current conversation through the runner and
  creates a new one. Rejected: carrying the conversation across, since the
  two harnesses keep incompatible session formats and history.
- **Model or effort change applies from the next message** (board (518)):
  the same conversation continues with the new model or effort; the runner
  owns how it applies, and if the runner cannot resume with another model
  Phase 1 falls back to a new conversation. Rejected: always forcing a new
  conversation.
- **Default is the last used choice on this machine** (board (519) a): a new
  conversation starts with the harness, model, and effort last used on this
  machine, one choice across stores. Rejected: per store; always the
  profile default.
- **Choice lists come from the runner** (board (520)): the harness list and,
  per harness, the model list and effort levels come from a Phase 1 runner
  endpoint. Rejected: lists hard-coded in the page, since Pi may list
  hundreds of models and the choices come from the profile's backend.
- **Tracker tool gate for the support conversation** (user, board (521) a,
  amending caller decision (428) a): a runner conversation key whose agent
  profile names the tracker tools passes the tracker gate like a top-level
  lead, and its writes are pinned to the conversation's own store (any
  other `store` is refused, and the register and sync network commands
  stay refused). Every other spawned key stays refused as before.
  Rejected: separate support-only tracker tools; a read-only support agent.
- **Hidden unless allowlisted** (board (522); mechanism per Sage design
  review): `tracker.show` and the git search tool are registered hidden and
  listed only in a conversation whose allowlist names them. This is a new
  predicate, "hidden unless `explicitAllowedTools()` names it", applied in
  `filteredTools` (tools/list), `toolAllowed` (tools/call), and
  `LeadToolNames` (runtime capabilities, Pi bridge snapshot); the existing
  `noAgentHiddenTool` hides by mode before the allowlist and is not reused
  as is. Rejected: always-registered tools
  narrowed by `WS_MCP_ALLOWED_TOOLS`, since that allowlist only narrows and
  the tools would show in every lead session, against (443).
- **Daemon-local state file** (board (523)): the daemon keeps the
  store-to-conversation mapping and the last harness, model, and effort in a
  local state file on disk, per machine, outside git, following local UI
  state (451) a. Rejected: browser storage.
- **Picker interaction** (user, mockup rounds 2-3 on the board topic): the
  model list has a short fuzzy search bar; effort is a drag knob on a bar
  that snaps to the harness's levels, with an empty circle marking each
  level on the bar and the low and high ends labelled. Rejected: a row of
  effort buttons.

Settled from the Sage design review (2026-11-08, auto-confirmed reviewer
issues):

- **One shipped profile per harness**: the runner fixes the backend per
  profile (runner (307) b; `createRequest` is {profile, root}), so this
  ticket ships one builtin support profile per harness (Claude and Pi),
  sharing one instruction text; the harness list of (520) is derived from
  those profiles, and each profile name is an overridable config key.
  Rejected: a backend override on the runner's create call, which would
  amend (307) b.
- **Builtin profile layer and embedded instructions**: wsagent gains a
  builtin profile layer (the `builtinWorktreeProfiles()` precedent in
  `internal/mcp/worktree_profile.go`) and the support instruction text is
  embedded in the binary, not resolved against the working root (the store
  clone is a bare ledger repository with no instruction file). The text
  carries the behaviour the Decisions rely on: write only on the user's
  explicit request, other stores read-only, call `tracker.show` to display
  a card, how to use git search.
- **Tracker grammar reaches the admitted key**: the tracker section render
  (workflow manual and `playbook.read`, 166fd45f "matching the call gate")
  follows the widened gate of (521), so an admitted support key reads the
  `tracker` playbook through `playbook.read` in its profile's ws_tools; the
  `tracker.query` and `tracker.update` descriptions stop saying
  "Lead-only" and name who may call. Rejected: duplicating the `tk:`
  grammar in the instruction text, which would drift.
- **Gate identification and store pin**: an admitted key is identified from
  a record the daemon or runner writes for the conversation (the daemon's
  agent record found through the key's reply ID, or a new key-record
  field), never from delegate scope with no parent alone (ferrule's lead
  login also mints such keys) nor from a caller-settable environment
  variable alone. The pinned store is the registry entry whose `Dir`
  matches the conversation's working root after canonicalisation. Both gate
  sites change together - the central check (`internal/mcp/server.go`
  ~L719) and the handler re-check `trackerSessionKey`
  (`tracker_tools.go`) - plus the delegate role gate `roleAllowsTool`.
- **Git search is an allowlist, not a denylist**: per subcommand a flag
  allowlist; no git global options (`-c`, `-C`, `--git-dir`,
  `--work-tree`); `--end-of-options` before revisions and `--` before
  paths; forced `--no-ext-diff --no-textconv` and no pager; the repository
  path checked after canonicalisation. Refused with tests: `grep -O` /
  `--open-files-in-pager`, `grep --no-index`, `blame --contents`,
  `--ext-diff`, `--textconv`, the global options above, and revisions
  beginning with `-`. This replaces the "no write-capable options" wording
  of (324), whose read-only, scoped intent stands.
- **Daemon-known roots**: from the daemon's session registrations,
  excluding runner conversations (`Agent` entries, whose root is a store
  clone), canonicalised; the registry's retention window bounds which roots
  count as "roots that have run a ws session" ((325) a).
- **Choice list discovery**: no runner code lists models or levels today;
  Phase 1 finds a per-harness discovery path (Claude levels from
  `claudeagents.Levels`, models from each harness's own listing), and a
  harness whose binary is not installed is listed as unavailable. The
  runner already switches the model in-stream for both harnesses, so the
  (518) fallback is expected to be unused.
- **`tracker.show` is pinned to the conversation's store**: it takes an
  item ID and no `store` argument; an item in another store is answered as
  text, never as a card, since a card opens the open drawer's thread sheet.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`; `ai-docs/manuals/ws-mcp.md` if `internal/mcp/` is
  touched; UI strings go through the dashboard i18n tables.
- Model and effort choices offered by the popover come from the agent profile's
  backend; the runner owns how a change applies.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261108-feat-ws-tracker-caller (2026-11-08, Decisions (428) a): "only top-level leads may call the tracker tools (same isTopLevelLead gate as the whiteboard); spawned workers report to their lead." (Result: tools "always listed and top-level-lead only") — bearing: amended by (521) a for profile-named support conversation keys
- 166fd45f (2026-11-08, commit): "The tracker section never renders for a spawned key, in workflow_manual and in playbook.read, matching the call gate." — bearing: constrains
- 166fd45f (2026-11-08, commit): "origin is agent or agent-auto only, never human: the store lets a human write beat an agent's, so the agent must not claim it." — bearing: supports
- 0a3228a5 (2026-11-08, commit): "a profile resolves through the working root's committed repo layer, and the tracker chat passes a clone other contributors write, so a committed mcp_servers command or plugin hook would run as the daemon user" — bearing: constrains
- 0651c140 (2026-11-08, commit): "(326) recorded assumed: the restriction is driven by the agent profile's allowlist passed via env, not named toolsets in ws-mcp code" — bearing: supports
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Decisions): "Standalone ticket, consumers depend on it (board (306) ok): the runner is not a tracker epic child, since its consumers are not all tracker features." — bearing: supports
- ef0138ad (2026-11-08, commit): "the support-agent placeholder tab become adapter extensions, so the shipped renderer carries no simulation code and the product shows no chat tab until its own ticket." — bearing: supports
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "A local AI may search across the user's stores read-only, only to answer the user's own query. It shows the answer only to the user and writes nothing into another store." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/, internal/mcp/, internal/wsdashboard/ (Phase 1); wsdashboard/assets/ (Phase 2) |
| scope.surface | cross-module | new ws tools plus agent.profile default plus runner conversation lifecycle plus tracker tool gate in internal/mcp/server.go#L720 |
| scope.new_public_symbol | yes | tracker.show, the read-only git search tool, the extra-roots config knob |
| scope.new_type_contract | yes | tracker.show mini-card result, git search tool arguments, per-store conversation mapping |
| scope.test_surface | new-files | existing agents-plugin-tool/internal/wsagent/fake_test.go and internal/mcp/tracker_tools_test.go; git search tests have no file yet |
| complexity.reuse_points | confirmed | Runner.Create/CloseConversation (wsagent/runner.go#L190-L236, #L343-L360), WS_MCP_ALLOWED_TOOLS gate (mcp/server.go#L5358-L5363), Profile.WSTools (wsagent/profile.go#L86) |
| complexity.side_effect_risk | high | the agent's delegate key is refused tracker.* by the lead-only gate (mcp/server.go#L719-L721); (521) a widens it for profile-named runner keys with store pinning |
| risk.correctness | high | git search argument validation must block write-capable options and out-of-scope paths; the widened tracker gate must pin writes to the conversation's store |
| risk.fit | high | Phase 1 widens caller Decision (428) a's lead-only gate ((521) a) and adds hidden tools |
| risk.test | moderate | fake backend exists, but the live conversation over daemon REST is manual |
| risk.security_or_contract | high | subprocess git over arbitrary repos, agent writes into the user's store, and a profile resolved through a clone others write (0a3228a5) |

## Phases

### Phase 1: Headless backend

`261108-feat-ws-tracker-caller` has merged (`a656a34b5`); the runner
live-check gate is waived ((515)). Phase 1 does not wait for the drawer UI.

- The support agent's `agent.profile.<name>` default (tools: the caller's
  tracker tools, `tracker.show`, the read-only git search tool; working root
  the store's local clone).
- The read-only git search tool (log, show, grep, blame, ls-files; argument
  validation with no write-capable options; scope = daemon-known project
  roots plus the config-knob roots, any other path refused).
- `tracker.show(item)`: the designated card-reference call, returning the
  item's mini-card fields from the store's view state.
- Per-store conversation lifecycle on the runner: one ongoing conversation
  per store kept across drawer opens and days; "new conversation" = runner
  `close` plus create; the mapping and the last selection in a daemon-local
  state file ((523)).
- The tracker gate change of (521) a: a runner conversation key whose
  profile names the tracker tools passes like a top-level lead, writes
  pinned to its store; tests for the pin, for refused register and sync,
  and for every other spawned key still refused.
- `tracker.show` and git search registered hidden unless the allowlist names
  them ((522)); a lead session's tool list does not show them.
- A runner endpoint listing the harnesses and, per harness, models and
  effort levels ((520)); a harness switch closes and recreates the
  conversation ((517)); a model or effort change applies from the next
  message, or falls back to a new conversation where the runner cannot
  ((518)).
- The builtin support profiles (one per harness) and the embedded
  instruction text.

Verification ((444) and the review decisions above):

- Go tests with the fake backend: git search refusals (every escape listed
  above) and path scope, including a canonicalised symlink path;
  `tracker.show` fields and its store pin; conversation keep, "new
  conversation", and harness switch; the gate admitting a profile-named
  runner key, pinning its writes, refusing `tk:register` and `tk:sync`,
  and still refusing a ferrule-minted parentless delegate key and a
  worker key; a lead session's tool list not showing the hidden tools.
- One live claude conversation over the daemon REST against
  `kang-sw/ws-tracker-live-test` that calls `tracker.query`,
  `tracker.show`, git search, and one requested `tracker.update` write.

### Result (4c802f2b7) - 2026-11-08

Landed in e9a3f557e, 592ddd994, 496d8de1c, and 4c802f2b7.

- **Builtin support profiles.** `agent.profile.tracker-support-claude` and
  `agent.profile.tracker-support-pi` form a builtin floor in wsagent, and a
  user definition of the same name replaces one whole. Their ws_tools are
  `tracker.query`, `tracker.update`, `tracker.show`, `git.search`, and
  `playbook.read`. Their instructions are `builtin:tracker-support`, a text
  embedded in the binary; the new `builtin:<name>` instructions scheme
  resolves no file under the root.
- **Tracker gate (521).** Both gate sites change: the central check and
  `trackerSessionKey`. A key passes only when all of these hold:
  - the daemon's open agent record for the key's reply-id names this exact
    key at the key's canonical root;
  - the record's profile, re-resolved on each call, names the tool;
  - the root is a registered store's canonical `Dir`.

  That store is the conversation's pinned store. A write naming any other
  store is refused, and so are `tk:register` and `tk:sync`.
  `tracker.query` stays a read across stores. The `playbook.read` tracker
  render follows the widened gate. The tool descriptions and the `tracker`
  playbook name who may call.
- **`tracker.show {item}`.** Takes no store argument. An item of the pinned
  store answers `card <store> <alias>` plus title, status, priority, due,
  and open assignees; that first line is Phase 2's renderer hook. Another
  store's item is answered as text with a "no card" note. A lead is refused.
- **Hidden unless allowlisted (522).** `tracker.show` and `git.search` go
  through `hiddenUnlessAllowlisted`, applied in `toolAllowed` (tools/list
  and tools/call) and in `LeadToolNames`.
- **`git.search`.**
  - Allowlist per subcommand (log, show, grep, blame, ls-files), with no git
    global options.
  - `--end-of-options` and `--` placement was checked per subcommand on git
    2.50.1. ls-files and blame need their own shapes, and a test pins all
    five shapes.
  - Forced `--no-ext-diff` and `--no-textconv` where the subcommand accepts
    them. No pager, `core.fsmonitor=false`, every `GIT_*` variable dropped,
    discovery capped at the matched root, `GIT_NO_LAZY_FETCH=1`, a 20 s
    timeout, and a 64 KiB output cap.
  - Scope: non-agent dashboard registrations last seen within
    `RegistryRetention`, plus the global-only, lead-authority knob
    `agent.git_search_roots`, both canonicalised. A relative `repo` is
    refused.
- **Per-store lifecycle (322, 517-520, 523).** `wsagent.Support` is mounted
  at `/support` through `DaemonOptions.Support`.
  - Routes: `GET /support/choices`, `GET /support/stores/<store>`, and
    `POST .../send`, `.../new`, `.../select`.
  - State: `<dashboard dir>/support.json` (0600) holds the machine-wide last
    selection and the store-to-conversation map.
  - "New conversation" is the runner's close plus create. A harness switch
    creates on the other profile, then closes the old conversation. A model
    or effort change on the same harness goes through
    `Runner.SetSettings`, so it applies from the next message; the (518)
    fallback is unused.
  - Choices: Claude models are the version-less aliases `opus`, `sonnet`,
    `haiku`, `fable`, passed verbatim (board (524)), and its efforts are
    `claudeagents.Levels`. Pi's list is dynamic from
    `pi --offline --no-approve --list-models`, cached for 5 minutes, with
    efforts off..max. A harness whose binary is missing is listed as
    unavailable.
  - `modelPattern` accepts `~`.
  - The ws-mcp runbook documents all of the above.

Verification:

- **Go suite.** `go test ./... -count=1` in agents-plugin-tool: 29 packages
  ok, exit 0, including mcp (350 s), wsagent, wsdashboard, wsconfig,
  wsrsrc, and cmd/ws-mcp. The new tests cover:
  - every listed git-search escape, path scope, and the symlink in and out;
  - `tracker.show`'s card, other-store, and refusal answers;
  - the gate admitting the support key and pinning its writes;
  - refused register and sync, and a refused ferrule-style parentless key,
    worker key, closed record, forged record, non-store root, and profile
    without the tool;
  - the hidden tools absent from a lead's tools/list and capabilities;
  - conversation keep across restart, new conversation, harness switch,
    model and effort change, last choice as the default, the choice lists,
    and the origin and content-type refusals.
- **Live check.** A dev daemon ran on port 39431 with isolated
  `WS_CACHE_HOME` and `WS_CONFIG_HOME`, and `WS_RSRC_ROOT` pointing at the
  worktree's rsrc. `kang-sw/ws-tracker-live-test` was registered over the
  daemon REST (member `supportcheck`, existing ssh auth).
  - One claude conversation over `/support/stores/live/send` called
    `tracker.query`, `tracker.show` (card of `ada.c89ca331327b-3`), and
    `git.search` (log of this worktree).
  - After a daemon restart, the same conversation was kept with the same
    id. On request it wrote one comment through `tracker.update`
    (`supportcheck.f87f9b6f489c-3`, synced and pushed). Its attempt on
    another store was refused by the pin.
  - The first turn's `playbook.read` failed only because a binary outside
    the plugin bundle had no rsrc root, which was the live setup, not the
    code.
- **Review.** Lite review, one pass: no Critical or Important findings. The
  first Minor finding was fixed in 4c802f2b7 (open closed `""` after a
  refused SetSettings).

Decisions taken:

- **Admission.** Admission is proven from the daemon's existing agent
  record, which already carries Profile, Root, WSKey, and ClosedAt, rather
  than a new key-record field.
- **Origin.** Origin is not forced to `agent`: the instruction text carries
  "write only on request".
- **Select baseline.** Select compares against the store's open
  conversation, not the machine-wide last choice, so another store's choice
  never turns a model tweak into a harness switch.
- **Stale mapping.** A mapping whose root no longer equals the store's
  `Dir` counts as no conversation, because the gate pins by `Dir`.

#### Forward

- Phase 2 reads the card line `card <store> <alias>` from the
  `tracker.show` result and drives `/support`.
- The review's second Minor finding is left as is: a failed `support.json`
  save leaves the store mapped in memory until restart.

### Phase 2: Chat card

`261108-feat-ws-tracker-drawer-ui` has merged (`a6f0b0ab1`). Phase 2 starts
after Phase 1 and after the in-flight drawer interaction and top-bar work
lands on develop, so the card builds on the final drawer layout.

- The drawer card (fixed height, internal scroll, newest at the bottom)
  rendering the conversation with the existing renderer, plus the
  card-reference entry kind drawn from `tracker.show` calls.
- The shared composer without its bottom badges, with the selection chip
  and popover ((516)): harness segment, fuzzy-searched model list, effort
  drag knob snapping to the levels with an empty circle per level; a new
  conversation defaults to the last used choice ((519)).

Verification:

- Headless browser screenshots against a dev-mode daemon on a spare port
  (never the user's daemon port): the card in the cards layout, the
  popover with the runner-supplied lists, fuzzy search narrowing a long Pi
  list, the knob snapping to each level, the harness-switch warning.
- End to end on a dev daemon: a message typed in the composer reaches the
  conversation and the reply renders; a `tracker.show` call renders as a
  mini card that opens the item's thread sheet; "new conversation" and a
  harness switch start a fresh conversation; the last choice is the
  default after a reload.
- `node --check` on the touched scripts; Go tests for any new daemon
  route; i18n keys in `en.json` and `ko.json`.
- The "new conversation" action.

### Result (c09fcbad8) - 2026-11-09

What landed (7e99671b3, review fixes in c09fcbad8):

- **Chat tab.** When the daemon serves `/support`, each tracker store's
  drawer gets a Chat tab. The stores answer carries a new `support` flag, so
  the page never probes `/support/choices` to decide.
  - The tab is a fixed-height card: the log scrolls inside it with the
    newest entry at the bottom, and the composer sits under the log.
  - The tab tools carry a "new conversation" action.
  - The tracker renderer redraws the drawer from HTML on every change. The
    chat therefore keeps one DOM root per store and moves it into the tab's
    slot on each redraw (`ext.rendered`), so the draft, scroll offset, and
    focus survive a redraw.
- **Log.** The log is the conversation's own `entries` and `convlive`
  regions, using the existing renderer and the merge-by-key behaviour.
  - The session stream now takes `?regions=entries,convlive` and sends only
    the regions it names. With a filter, the mode pseudo-region is dropped
    and git is not watched.
  - The stream is open only while the chat shows.
  - Held-message steer and cancel controls are stripped.
- **Card entries.**
  - The Claude and Pi mappers read a successful ws `tracker.show` result's
    `card <store> <alias>` line, plus the title, into `Event.Card`.
  - The runner writes it as a `card` ring entry (`wslive.State.Card`) after
    the streamed reply and the tool gap it ends. Store and alias are checked
    against an id grammar, and the title is scrubbed and capped.
  - The entry template draws a plain reference line with
    `data-tk-store`/`data-tk-alias`. The chat upgrades it to `h.crefHtml`
    from the view the renderer drew, and clicking the card opens the item
    window. The plain line shows wherever the view does not hold the item.
- **Composer and popover.**
  - The dashboard composer's markup and CSS and its Enter/mod input mode,
    without the session badges.
  - The selection chip opens a popover with three controls:
    - a harness segment: switching an open conversation's harness first
      shows an inline warning and confirm, then starts a new conversation
      (517);
    - a fuzzy-searched model list including "default", with arrow and Enter
      keys;
    - an effort knob: role=slider, an empty circle per level, the thumb
      snapping on drag or arrow keys, the low and high ends labelled, and a
      reset to default.
  - A model or effort change applies from the next message, with a note
    saying so (518). The last choice is the default (519), and the runner
    supplies the lists (520).
- **i18n.** `entry.card`, `entry.card.title`, and 24 `js.sup.*` keys are in
  all 11 tables. The `js.sup.*` prefix keeps them out of the tracker
  mockup's generated `js.tk.*` lookup file.

Verification:

- `go test ./... -count=1 -timeout 40m` in agents-plugin-tool: 29 packages
  ok, exit 0. New tests:
  - `TestCardFollowsTheReplyItEnds` (wslive);
  - `card_test.go` (wsagent): card line parse; the Claude mapper with
    string and block results, error, another tool, and another server; the
    Pi mapper; the live writer ring entry;
  - `support_chat_test.go` (wsdashboard): entry markup, the
    `?regions=entries,convlive` stream sending only those regions, and the
    stores support flag. `TestTrackerRoutesAnswer` now pins
    `"support":false`.
- `node --check` on dashboard.js passes.
- Dev daemon on port 39457, with isolated `WS_CACHE_HOME` and
  `WS_CONFIG_HOME`, `WS_RSRC_ROOT` set to the worktree rsrc, and
  `WS_MCP_DASHBOARD_DEV_SOURCE` set. It used a local store `live` with no
  remote and three seeded items. Headless chrome-headless-shell through
  playwright-core checked:
  - the card in the cards layout (1300px) and on the narrow store page
    (900px);
  - a message typed and sent with Enter reached the claude conversation.
    The reply rendered, and a `tracker.show` call rendered as the mini
    card, which opened the item window titled "Fix the sync ribbon
    flicker". This was re-run after the review fixes;
  - the popover listed claude and pi with claude's models;
  - the knob snapped to each of low, medium, high, xhigh, and max, and the
    chip followed. A mid-drag move snapped to "high";
  - the harness-switch warning appeared, and confirming it started a fresh
    pi conversation with an empty log;
  - "sol" narrowed the 422-model Pi list to 33, and a keyboard pick changed
    the model on the same conversation;
  - after a reload, the chip showed the last choice (`pi · openai-codex/gpt-5.6-sol`);
  - "new conversation" gave a new id with an empty log;
  - a tracker write during typing kept the draft and the focus.
- Review: lite, one pass, with no Critical or Important findings. Minors 1
  and 2 were fixed in c09fcbad8: card upgrade from the renderer's drawn view,
  which removed a possible throw, and a card whose item left the view going
  back to the server line.

Decisions taken:

- **Card source.** Cards come from the runner reading tool results, not
  from a new live-intake event kind, so the adapter intake protocol is
  unchanged.
- **Stream.** The chat reuses the session stream with a region filter,
  with no new chat route.
- **Composer.** The composer is reused by markup, CSS, and input mode, not
  by code, because the dashboard composer's code is bound to the session
  page's element ids.
- **Known limits.**
  - The chat shows the newest ring page (50 entries), with no older-history
    paging.
  - The tab is decided once at mount (tracker.js merges `ext.tabs` only
    there). A host that first answers 503 and later reports support needs
    a page reload (review Minor 3, left).
  - The runner records the chat's user messages with the whiteboard origin
    tag, which predates this phase.

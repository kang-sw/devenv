---
title: Claude Code mailbox active push through a plugin mod
related:
  260913-feat-cross-session-mailbox-wake: wake path this extends; its Claude Stop-hook adapter and run_in_background wait remain the fallback
  260919-feat-mailbox-arm-wait-harness-include: arm-the-wait include whose wording an actionable child would change
  260914-feat-ws-pi-mailbox-native-steer-push: pi active push this mirrors on Claude
  261016-research-claude-mod-pi-like-compaction: sequenced after this research validates the mod path
---

# Claude Code mailbox active push through a plugin mod

## Background

Claude Code had no path for an external event to start a turn in an idle
session. The ws mailbox wake on Claude therefore relies on the model arming a
one-shot `ws-mcp mailbox wait` through `run_in_background` and on the
`Stop` command hook in `agents-plugin/hooks/hooks.json` as a turn-boundary
backstop (`260913-feat-cross-session-mailbox-wake`). The model forgetting to
re-arm loses the wake, and a session whose user is away is best-effort.

Pi has no such gap: its adapter auto-arms a session-bound waiter, drains
arriving mail through recv itself, and pushes the drained batch into the
conversation (`agents-plugin-pi/src/mailbox-waiter.ts`).

Claude Code now ships plugin mods (plugins of function hooks: a TS/JS module
named under `modules` in a plugin's `hooks/hooks.json`, exporting
`register(on, options)`). This research established, by a discussion and a
live probe on 2026-10-16, that a mod can give Claude the same pi-style active
push, and settled the design an actionable child should implement.

## Design

```text
agent ── calls ──▶ mod tool mailbox_watch(session_key)        ($.tool.register)
                     │ same key: no-op · new key: kill loop, rebind
                     ▼
                   loop: ws-mcp mailbox wait --session-key K   ($.process.spawn)
                     │ exit 0 (mail present; wait only peeks)
                     ▼
                   drain: ws MCP mailbox_recv(session_key K)   ($.tool.call)
                     ▼
                   one $.prompt.submit with the whole batch → re-arm
session.end (reason clear) ── stops the loop
```

- Wait exit codes as pi uses them: 0 mail present, 3 timed out, 130
  interrupted, other error. Errors back off (probe: 1s doubling to 60s); an
  empty drain after a 0 exit backs off instead of re-submitting.
- The drained mail is already consumed, so re-arming right after the submit
  cannot hot-loop or double-deliver; this is pi's reason for draining via recv
  rather than replaying the wait's peek.
- The probe implementation is preserved at
  `ai-docs/ref/claude-mailbox-push-probe-mod/` (controller in
  `hooks/watcher.ts`, engine wiring in `hooks/register.ts`, tests).

## Probe results (2026-10-16)

Offline, by a delegate, against the Claude Code 2.1.293 declaration file
(`plugin-authoring` skill `types/claude-code.d.ts`):

- `claude plugin validate`: passed (only warning: no author).
- `claude plugin test`: 13 pass / 0 fail, including one test through the real
  engine `$` (bind, spawn wait, exit 0, recv via `$.tool.call`, one
  `prompt.submit` with origin `{kind:'plugin'}`, re-arm; subagent call denied;
  no re-arm after `session.end` `clear`).
- `tsc -p <mod>`: clean against the engine-laid types.
- A throwaway plugin whose `hooks/hooks.json` holds the current
  `agents-plugin` classic `hooks` (Stop, SessionStart) plus `modules` passed
  `claude plugin validate`, which checks both halves (a bogus classic hook type
  is reported while the module is still listed). Runtime behavior of both
  halves from one file was not exercised.
- With one mail queued, `mailbox wait --timeout 0 --format json` exits 0 twice
  in a row (peek only); `mailbox_recv` with `format: json` returns a bare
  envelope array (`[{"reply_to": ..., "content": ..., "sent_at": ...}]`).

Live, in the lead's interactive Claude Code 2.1.300 session with dev-mods hot
reload enabled, using a throwaway child session key minted with the lead as
`parent_session_key` (a parent-less mint would rebind the lead's named-inbox
owner, per `rebindMailboxOwnerAtFerrule`):

- The registered tool appeared in the model's tool list as
  `mcp__ws-mailbox-push-probe__mailbox_watch` (served by the mod, not an MCP
  server).
- Binding spawned exactly one `mailbox wait` child; a second call with the same
  key returned `unchanged` and left one child.
- A mail sent from a separate headless session while the lead was idle started
  a new lead turn on its own, carrying the plugin-framed prompt with the
  drained message; the wait child was replaced by a fresh one (re-armed), one
  child total.
- No permission dialog appeared for the mod's `$.tool.call` of
  `mailbox_recv` under the user's `defaultMode: auto` with no explicit allow
  rule.

## Outcome Ledger

### Verified Findings

- A mod can start a turn in an idle interactive Claude Code session:
  `$.prompt.submit` "queues a prompt that starts a turn of its own once the
  session is idle (never folded into a running turn)" (reference), confirmed
  live (see Probe results).
- `$.tool.register` declares a model-callable tool named
  `mcp__<plugin>__<name>`, served by the mod's own `tool.call` hook; it rejects
  until the session binds at `session.start`.
- `tool.call` hooks receive the call's full arguments plus `agentId`, which is
  absent on the main loop and set for a subagent or teammate.
- `$.tool.call` reaches MCP server tools through the same path as a model call,
  including "the permission check and its dialog"; live, no dialog appeared
  under `defaultMode: auto`, the only mode probed. `$.mcp.call(server, tool,
  args)` is declared as the no-permission-prompt alternative; its exact
  server/tool spelling for ws is unverified.
- The permission exposure is not new with the mod: today's fallback (the
  model calls `mailbox_recv` after the background wait returns) passes the
  same permission check, so an away user under a prompting mode already
  stalls there. Whether a call prompts depends on the permission mode and the
  allow/deny rules together, and the mode can change mid-session.
- `agents-plugin-wsflow/` has no `hooks/` directory, so a wsflow mod adds the
  first Claude hooks surface to that package; `install.sh` deletes the
  `wsflow@kang-sw-devenv` Claude install record, so wsflow reaches Claude only
  through a manual marketplace install.
- `$` cannot be stored or passed around (`claude plugin validate` refuses it);
  every engine call is spelled `$.noun.method(...)` inline inside a hook, so
  side-effect edges are closures created inside `session.start`.
- A hot reload re-runs `register` and `session.start` and drops module
  variables, so a binding held in a module variable is lost and the agent must
  bind again.
- `$.process` is documented "CLI only"; `$.process.run` caps at 10 minutes, so
  a long wait uses `$.process.spawn`, whose child dies when its loop is left,
  the stream is returned, the dispatch aborts, or the module unloads.
- The mod API is documented as early access that moves between releases; the
  declaration file per build is the authority.
- The Claude `Stop` hook adapter (`ws-mcp mailbox claude-stop-hook`) exits
  at once unless the Claude process environment sets `WS_MAILBOX`
  (`mailboxClaudeStopHook`, the empty-slug branch), and it runs only at a turn
  end, so it never wakes an idle session; it has not been observed firing in
  dogfooding. The background shell-wait fallback does not depend on it.
- `ws-mcp mailbox wait` writes one listening marker per session key holding
  its PID; a replacement wait on the same key overwrites it, and each wait
  clears only a marker carrying its own PID
  (`agents-plugin-tool/internal/wsmailbox/listening.go`).
- `ws-mcp mailbox wait --timeout 0` blocks until mail
  (`agents-plugin-tool/cmd/ws-mcp/mailbox.go`, `timeout` flag); pi uses a
  finite timeout so a stuck or orphaned wait clears itself.

### Confirmed Decisions

- Ship the Claude push mod inside the existing `ws` and `wsflow` plugins, not
  as a separate opt-in plugin: a mod-API break only loses the push and falls
  back to today's behavior, an acceptable risk.
- No new ws MCP tool and no new `ws-mcp` CLI subcommand. The mod loops the
  existing one-shot `mailbox wait` as a peek-only signal.
- The agent actively binds by calling a mod-registered tool with its own
  session key; the binding is not inferred from other ws tool calls.
- The mod drains through the ws MCP recv tool itself and submits the whole
  drain as one prompt, mirroring pi.
- The drain calls recv through `$.mcp.call` first, so a mod-initiated drain
  never raises a permission prompt in an idle session: it is the ws mod
  reading its own session-bound inbox on the ws server. Only if `$.mcp.call`
  proves unusable (it prompts anyway, or ws cannot be addressed through it)
  does the design fall back to deciding in advance whether a `$.tool.call`
  would prompt.
- Binding lifecycle: one binding per Claude session, last call wins. The same
  key again is a no-op (covers revive with the preserved key); a different key
  replaces the binding (covers a new `ferrule`) without detecting either event;
  `session.end` with reason `clear` stops it; compaction keeps it.
- Pi is unchanged: its adapter keeps auto-arming and draining.
- Installing both `ws` and `wsflow` in one Claude Code is a user error and
  out of scope; there is no double-binding guard.
- The same actionable child that ships the mod retires the Claude `Stop` hook
  adapter: the `Stop` entry in `agents-plugin/hooks/hooks.json`, the
  `ws-mcp mailbox claude-stop-hook` subcommand and its tests, and the hooks
  file description's Stop sentence. `SessionStart(compact)`,
  `codex-stop-hook`, and the shared `wsmailbox.ShouldNotifyNamedInboxUnread`
  watermark stay. Lost by this: the turn-end notice for a mod-less Claude
  session launched with `WS_MAILBOX`, which still receives mail through the
  background-wait fallback.
- Fallback when the mod tool is absent (mod not loaded, API broken, surface
  without `$.process`): the existing shell `mailbox wait` armed through the
  harness background task. The `arm-the-wait` wording becomes "if the mailbox
  watch tool is available, call it with your session key; otherwise launch the
  shell wait in the background."

### Proposals

- Persist the bound key in `$.state` (session-scoped) so a mod reload re-binds
  without an agent call.
- Use a finite wait timeout (pi's `DEFAULT_WAIT_TIMEOUT_ARG`, `10m`, in
  `agents-plugin-pi/src/mailbox-waiter.ts`) instead of `--timeout 0`, so an
  orphaned wait cannot outlive its session.
- Resolve the launcher from the plugin's own root at runtime instead of the
  probe's hardcoded installed path.
- Gate the mod on `$.session.version()` against a tested range and fall back to
  the shell wait outside it.
- Refer to the mod tool in shipped playbook text by role, not by its full
  `mcp__<plugin>__<name>` spelling, which differs between `ws` and `wsflow`.

### Open Questions

- Does `$.mcp.call` reach the ws server's `mailbox_recv` without a
  permission prompt under a prompting mode (for example `default`), and what is
  its server/tool spelling for `ws` and for `wsflow`? This is the next probe.
- Does a different-key rebind actually kill the previous blocked `wait` child
  (`stream.return()` on a silent child)?
- Does `/clear` stop the loop and leave no `wait` child in a live session?
- With mail arriving mid-turn, does the submit queue as its own turn after the
  running one?
- Is the registered tool visible to subagents (the deny on `agentId` is the
  fallback guard)?
- Do the classic `hooks` and `modules` both run at runtime from one
  `agents-plugin/hooks/hooks.json`?
- Does `$.process` work in the desktop app's local Code tab and IDE-hosted
  sessions?

### Rejected Alternatives

- **Claude Code Monitor as the wake.** It re-fires per stdout line, but every
  monitor expires after at most 30 minutes and must be re-armed, costing an
  idle turn per expiry while leaving the model-forgets-to-re-arm weakness in
  place. Not documented as a fallback either.
- **A streaming `ws-mcp mailbox watch` CLI subcommand.** A restart loop is
  needed around any child process anyway, so a long-lived streaming process
  adds a new surface without removing the loop; the one-shot `wait` suffices.
- **A new ws MCP `mailbox.watch` tool.** The binding belongs to the mod's own
  tool layer; the MCP server cannot start a Claude turn, and a host-neutral MCP
  tool would add surface to every harness for one adapter.
- **A separate opt-in plugin for the mod.** Rejected in favor of shipping in
  `ws`/`wsflow` (see Confirmed Decisions).
- **Expiring a binding by detecting re-`ferrule` or revive.** A second
  `ferrule` in one worktree is an independent track, so a new key does not mean
  the old one died; last-call-wins handles both cases without detection.
- **Gating the push on detecting auto mode.** A prompt depends on the mode
  and the allow/deny rules together, and the mode changes mid-session, so a
  mode read at bind time goes stale; `$.mcp.call` removes the prompt instead.
- **A double-binding guard for `ws` plus `wsflow`** (for example the mod
  skipping its bind when another process's listening marker holds the key).
  Both installed is a user error; the guard would add a race and couple the
  mod to the ws cache layout.
- **Keeping or narrowing the Claude `Stop` hook while the mod is bound.**
  Narrowing needs the command hook to know the mod's binding state; the hook
  practically never fires, and the mod covers both idle and mid-turn mail.
- **Retiring the `Stop` hook as a separate chore ahead of the mod.** The user
  chose one child so the removal lands with its replacement.
- **Emitting one push per envelope.** One batch per drain, as pi settled after
  `261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers`.

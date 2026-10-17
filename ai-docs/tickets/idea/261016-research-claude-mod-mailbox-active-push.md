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
  under `defaultMode: auto`, the only mode probed.
- `$.mcp.call` is permission-gated at runtime, contrary to its declaration.
  The 2.1.300 declaration says "No permission prompt: the plugin's call, seen
  by the hooks above it, is the grant" (`claude-code.d.ts`:2752), but a
  headless `claude -p --permission-mode default` run with no allow rule
  refused `$.mcp.call("plugin:ws:ws", "mailbox_recv", ...)` exactly like
  `$.tool.call`, whether the mod sat in the ws plugin or in a separate one;
  the debug log shows `$.mcp.call` routed through the ordinary `tool.call`
  path with its permission check (probe 2026-10-17, Claude Code 2.1.300,
  probe mod and run logs in
  `ai-docs/ref/claude-mcp-call-probe-mod/runs/`).
- Under the same mode `$.tool.check` on `mcp__plugin_ws_ws__mailbox_recv`
  answers `{decision: "ask"}` (same probe).
- Under `--permission-mode auto` with no allow rule, `$.tool.check` on the
  recv tool still answers `ask` (the same `reason` as default mode, no
  `decider`), yet the following `$.mcp.call` drains with no prompt: the debug
  log reads `Skipping auto mode classifier for
  mcp__plugin_ws_ws__mailbox_recv: called by plugin`. `tool.check` reports the
  verdict before the mode settles an ask (`claude-code.d.ts`:4057), so the
  pre-check cannot tell an auto user from a prompting one (follow-up probe
  run 6, headless).
- Under `--permission-mode default` with `--allowedTools
  mcp__plugin_ws_ws__mailbox_recv`, `$.tool.check` answers
  `{decision: "allow", rule: "mcp__plugin_ws_ws__mailbox_recv"}` and the drain
  succeeds (run 7).
- A mod `tool.call` hook on the recv tool observes the model's own call with
  `origin: {plugin: "engine"}`, no `agentId` on the main loop, and the
  `session_key` argument readable. The mod's own `$.mcp.call` passes the same
  hook with its plugin as origin and a `toolu_plugin_` id, so the re-arm
  observer filters on `origin.plugin === "engine"` (run 7). Whether the hook
  also sees a model call the user then denies is unprobed.
- The 2.1.300 mod API has no reader for the session's current permission
  mode. The mode appears only as a subagent spawn's pinned `permissionMode`
  (`claude-code.d.ts`:323), the classic command hook input `permission_mode`
  (:861), and `prompt.attachment` types `auto_mode` / `auto_mode_exit`
  (:15211), which fire only as the next model request carries them.
- A mod `tool.check` hook returning `{decision: "allow"}` when
  `next.origin.plugin` is its own plugin let the mod's drain succeed with no
  prompt while the model's own call of the same tool stayed denied; a foreign
  plugin's hook could grant ws's tool the same way (same probe).
- Spellings: the ws server is `plugin:ws:ws` (tool `mailbox_recv`, permission
  name `mcp__plugin_ws_ws__mailbox_recv`); wsflow's is `plugin:wsflow:wsflow`
  (`mcp__plugin_wsflow_wsflow__mailbox_recv`). Inside the ws plugin,
  `$.mcp.connect("ws")` returns `{isConnected: true, server: "plugin:ws:ws"}`,
  so shipped code need not hardcode the server name (same probe).
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
- The mod never raises a permission prompt and never overrides the user's
  permission policy. Before each drain it asks `$.tool.check` for the recv
  tool's verdict:
  - `allow` (an allow rule; auto mode alone answers `ask`, see Verified
    Findings): the mod drains through recv itself
    (`$.mcp.call`, server name from `$.mcp.connect`) and submits the whole
    drain as one prompt, mirroring pi.
  - otherwise: the mod submits a notice prompt telling the model to call recv
    with its session key, so the model drains through the ordinary
    permission flow. The mod then suspends its wait loop and re-arms only
    after its `tool.call` hook observes the model's `mailbox_recv` call for
    the bound key, so one arrival yields one notice. (A peek-only wait
    re-armed over still-queued mail would exit at once and loop.)
  - This pre-checked routing is the default. A ws config knob, set through
    `config.tune`, opts into direct drain: the mod skips the pre-check, drains
    through `$.mcp.call`, and takes the notice path only when the call is
    refused. It serves auto-mode users, whose `ask` verdict would otherwise
    cost one extra model tool call per arrival; an allow rule on the recv tool
    reaches the same result under the default.
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
- Declare the direct-drain knob the way pi declares its adapter knobs: a
  plugin-local `config-manifest.json` (for example a boolean
  `claude.mailbox_direct_drain`, default `false`) that the plugin's MCP
  launcher passes through `WS_MCP_CONFIG_MANIFESTS`
  (`agents-plugin-pi/src/adapter-config.ts`). The mod reads it at each drain
  through `$.mcp.call` of `config_get`; a refused or failed read resolves to
  the default, which is the pre-checked routing.

### Open Questions

- In an interactive session under a prompting mode with no allow rule, does a
  mod's `$.mcp.call` raise a permission dialog or refuse at once? Only the
  headless ask -> refuse is observed. A refusal with no dialog would make
  direct drain harmless for every mode and could make the knob unnecessary.
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
  mode read at bind time goes stale; `$.tool.check` asks the engine for the
  actual verdict instead.
- **A self-granting `tool.check` hook** (allow the mod's own recv by origin).
  Verified to work, but it overrides the user's permission policy for ws's
  tool and leans on engine looseness (any plugin can grant another's tool)
  that a later release may close. A user who does not run auto mode already
  confirms tool calls routinely, so the notice path costs little.
- **Calling recv from the mod with no pre-check, as the default.** Least
  code, but a prompting user would face a mod-raised dialog in an idle session
  whose interactive behavior is unverified (headless only shows ask -> deny).
  Kept as the opt-in direct-drain knob instead (see Confirmed Decisions).
- **Detecting auto mode through `prompt.attachment` `auto_mode` /
  `auto_mode_exit`.** The attachment fires only when the next model request
  carries it, so a user who leaves auto mode while idle stays latched as auto
  until the next turn, and the mod would then drain into a dialog in exactly
  the idle window it acts in. Too hacky for the gain; the user chose the
  config knob.
- **Re-arming at the next `turn.complete` after a notice.** Simpler, but a
  user who declines the recv dialog would get a fresh notice every turn.
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

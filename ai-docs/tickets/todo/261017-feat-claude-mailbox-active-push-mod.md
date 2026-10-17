---
title: Claude mailbox active push through a plugin mod
related:
  261016-research-claude-mod-mailbox-active-push: source research; its Outcome Ledger is this ticket's authority
  260913-feat-cross-session-mailbox-wake: wake path this extends; its Claude Stop-hook adapter is retired here
  260919-feat-mailbox-arm-wait-harness-include: arm-the-wait include whose Claude wording changes here
  260914-feat-ws-pi-mailbox-native-steer-push: pi active push this mirrors on Claude
---

# Claude mailbox active push through a plugin mod

## Background

On Claude Code the ws mailbox wake relies on the model arming a one-shot
`ws-mcp mailbox wait` through its background-task capability, plus a `Stop`
command hook that only fires at a turn end. A model that forgets to re-arm
loses the wake. Pi has no such gap: its adapter auto-arms a session-bound
waiter, drains through recv, and pushes the drained batch
(`agents-plugin-pi/src/mailbox-waiter.ts`).

Claude Code plugin mods (a TS module listed under `modules` in a plugin's
`hooks/hooks.json`, exporting `register(on, options)`) can start a turn in an
idle session through `$.prompt.submit`.
`261016-research-claude-mod-mailbox-active-push` established by probe that a
mod can give Claude the same active push and settled the design below. Its
probe mods are preserved at `ai-docs/ref/claude-mailbox-push-probe-mod/`
(watch loop, engine wiring, tests) and `ai-docs/ref/claude-mcp-call-probe-mod/`
(permission probes, run logs).

## Decisions

- **Ship inside `ws` and `wsflow`**, not as a separate opt-in plugin: a mod-API
  break only loses the push and falls back to today's behavior.
  Rejected: a separate opt-in plugin.
- **No new ws MCP tool and no new `ws-mcp` CLI subcommand.** The mod loops the
  existing one-shot `mailbox wait` as a peek-only signal (exit 0 mail present,
  3 timed out, 130 interrupted, other error).
  Rejected: a streaming `ws-mcp mailbox watch` subcommand (a restart loop is
  needed around any child anyway); a host-neutral MCP `mailbox.watch` tool (the
  MCP server cannot start a Claude turn); Claude Code Monitor as the wake
  (expires after at most 30 minutes, an idle turn per re-arm).
- **Explicit bind.** The agent binds by calling a mod-registered tool with its
  own session key; the binding is not inferred from other ws tool calls.
- **Binding lifecycle.** One binding per Claude session, last call wins. The
  same key again is a no-op (covers revive with the preserved key); a different
  key replaces the binding and stops the previous wait loop (covers a new
  `ferrule`). `session.end` with reason `clear` stops it; compaction keeps it.
  Rejected: expiring a binding by detecting re-`ferrule` or revive (a second
  `ferrule` in one worktree is an independent track).
- **Permission routing.** The mod never raises a permission prompt and never
  overrides the user's permission policy. By default, before each drain it asks
  `$.tool.check` for the recv tool's verdict:
  - `allow` (an allow rule; auto mode alone answers `ask`): the mod drains
    through recv itself (`$.mcp.call`, server name from `$.mcp.connect`) and
    submits the whole drain as one prompt, mirroring pi.
  - otherwise: the mod submits a notice prompt telling the model to call recv
    with its session key, so the model drains through the ordinary permission
    flow. The mod suspends its wait loop and re-arms only after its
    `tool.call` hook observes the model's `mailbox_recv` call for the bound
    key, so one arrival yields one notice.
  Rejected: a self-granting `tool.check` hook (overrides the user's policy and
  leans on engine looseness a release may close); gating on detecting auto
  mode (a bind-time read goes stale; `prompt.attachment` `auto_mode` latches
  stale while idle, in the unsafe direction); re-arming at the next
  `turn.complete` after a notice (a user who declines the dialog gets a notice
  every turn); one push per envelope (one batch per drain, as pi settled in
  `261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers`).
- **Opt-in direct drain.** A ws config knob, set through `config.tune`, opts
  into direct drain: the mod skips the pre-check, drains through `$.mcp.call`,
  and takes the notice path only when the call is refused. It serves auto-mode
  users, whose `ask` verdict otherwise costs one extra model tool call per
  arrival. Rejected: direct drain without pre-check as the default (a
  prompting user could face a mod-raised dialog in an idle session).
- **Pi is unchanged**: its adapter keeps auto-arming and draining.
- **No double-binding guard.** Installing both `ws` and `wsflow` in one Claude
  Code is a user error and out of scope. Rejected: skipping the bind when
  another process's listening marker holds the key (adds a race, couples the
  mod to the ws cache layout).
- **Retire the Claude `Stop` hook adapter in this ticket**: the `Stop` entry in
  `agents-plugin/hooks/hooks.json`, the `ws-mcp mailbox claude-stop-hook`
  subcommand and its tests, and the hooks file description's Stop sentence.
  `SessionStart(compact)`, `codex-stop-hook`, and the shared
  `wsmailbox.ShouldNotifyNamedInboxUnread` watermark stay. Lost by this: the
  turn-end notice for a mod-less Claude session launched with `WS_MAILBOX`,
  which still receives mail through the background-wait fallback. Rejected:
  keeping or narrowing the Stop hook while the mod is bound; retiring it as a
  separate chore ahead of the mod.
- **Fallback when the mod tool is absent** (mod not loaded, API broken, surface
  without `$.process`): the existing shell `mailbox wait` armed through the
  harness background task. The Claude `arm-the-wait` wording becomes "if the
  mailbox watch tool is available, call it with your session key; otherwise
  launch the shell wait in the background."

- **Re-arm observer.** The mod's `tool.call` hook on the recv tool matches the
  model's call by `origin.plugin === "engine"` and the bound `session_key`;
  the mod's own `$.mcp.call` passes the same hook with its plugin as origin and
  a `toolu_plugin_` id (research run 7).
- **Server resolution.** The mod resolves its server through
  `$.mcp.connect(<own plugin name>)`; the ws and wsflow server names equal
  their plugin names (`plugin:ws:ws`, `plugin:wsflow:wsflow`). No server name
  is hardcoded.
- **Tool availability.** The mod registers its bind tool only where `$.process`
  is available, so the Claude arm-the-wait "if available" branch falls back to
  the shell wait elsewhere.
- **One mod source, mirrored.** The mod source lives in `agents-plugin/hooks/`;
  `agents-plugin-wsflow/hooks/` holds a byte-identical copy pinned by a drift
  test in the `TestPiMirrorUpToDate` style
  (`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`). Package identity
  (plugin name, root) is read from the engine at runtime. wsflow's
  `hooks/hooks.json` lists only `modules`. Rejected: per-package sources,
  which drift.
- **Claude arm-the-wait variant.** The Claude wording goes in a new
  `agents-plugin/rsrc/lead-use-mailbox/arm-the-wait.claude.md` harness variant,
  with the wsflow rsrc mirror regenerated. Rejected: editing the shared base,
  which Codex also renders and where no mod tool exists. `.claude.md` variants
  are an existing mechanism (`native-spawn-binding.claude.md`).
- **Finite wait.** The wait runs with pi's `10m` timeout
  (`DEFAULT_WAIT_TIMEOUT_ARG` in `agents-plugin-pi/src/mailbox-waiter.ts`) and
  re-arms on exit 3. Rejected: `--timeout 0`, which lets an orphaned wait
  outlive its session.
- **Launcher from the plugin root.** The mod resolves `bin/ws-mcp-launcher.py`
  from its own plugin root at runtime. Rejected: the probe's hardcoded
  installed path.
- **Bind tool shape.** The tool is `mailbox_watch` taking `{session_key}`, as
  in the probe. Shipped text refers to it by role ("the mailbox watch tool"),
  never by its full `mcp__<plugin>__<name>` spelling, which differs between ws
  and wsflow.
- **Direct-drain knob.** Boolean `claude.mailbox_direct_drain`, default
  `false`, declared in a plugin-local `config-manifest.json` in each package.
  The launcher sets `WS_MCP_CONFIG_MANIFESTS` to that file only when the
  variable is unset and the file exists; pi's bridge sets the variable itself,
  so pi is untouched. The mod reads the knob at each drain through
  `$.mcp.call` of `config_get`; a refused or failed read resolves to `false`.
  Codex's ws config listing shows the key too; accepted. Rejected: `.mcp.json`
  `env` (ws's `.mcp.json` is shared with Codex, and `${CLAUDE_PLUGIN_ROOT}`
  expands only in Claude); Claude `userConfig` (not reachable through
  `config.tune`).
- **Reload survival.** The bound key is persisted in session-scoped `$.state`
  and cleared on `session.end` reason `clear`, so a mod reload re-binds without
  an agent call. Rejected: a module variable alone, which a hot reload drops
  with no signal to the agent.
- **No version gate.** No `$.session.version()` range check. Rejected: a
  tested-range gate, which must be bumped every Claude release while a broken
  API already degrades to the shell-wait fallback.
- **Prompt framing.** The push and notice prompts follow pi's mailbox push
  framing (`agents-plugin-pi/src/mailbox-waiter.ts`) rather than new wording.
- **Concurrent recv.** A Go test proves two concurrent `mailbox_recv` calls on
  one key never return the same envelope; ws-mcp is fixed in this ticket if
  it fails. The mod's drain and a model's unprompted recv are the one overlap
  the mod opens.

## Constraints

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`
  (`agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`),
  `ai-docs/manuals/skill-authoring.md` (`agents-plugin/rsrc/`,
  `agents-plugin-wsflow/rsrc/`), `ai-docs/manuals/wsflow-mirroring.md`
  (`agents-plugin/rsrc/`, `agents-plugin-wsflow/`), and
  `ai-docs/manuals/ws-mcp.md` for any `agents-plugin-tool/internal/mcp/`
  change.
- The mod API is early access; the Claude Code build's own
  `plugin-authoring` declaration file (`types/claude-code.d.ts`) is the
  authority, and contradicts its prose in places (`$.mcp.call` is
  permission-gated at runtime despite "No permission prompt").
- Codex behavior is unchanged: `codex-stop-hook`, the base `arm-the-wait.md`,
  and the Codex manifests stay as they are.
- `install.sh` still installs only `ws` into Claude; no change.

## Phases

### Phase 1: Claude mailbox push mod in ws and wsflow, Stop hook retired

Implement every decision above in one landing, so the Stop hook adapter is
removed together with its replacement.

Verification the worker runs:

- `claude plugin validate` and `claude plugin test` pass for both
  `agents-plugin` and `agents-plugin-wsflow`.
- Engine-backed mod tests cover: bind; same-key no-op; a different-key rebind
  stops the previous wait child; `session.end` `clear` stops the loop;
  subagent bind denied (`agentId` set); `allow` verdict drains once and
  submits one batch; non-`allow` verdict submits one notice, suspends, and
  re-arms only on an engine-origin recv for the bound key (the mod's own call
  does not re-arm); knob on, refused call falls back to the notice path; a
  failed knob read behaves as `false`.
- Go tests for the Stop hook removal pass, the concurrent-recv test passes,
  and the wsflow and pi mirror guards pass.

Post-merge dogfood checklist (attended interactive session, not worker-run;
the worker cannot drive one, and headless `claude -p` ends after one turn):

- Mail arriving mid-turn queues its submit as its own turn after the running
  one.
- The classic `hooks` and the `modules` both run from one
  `agents-plugin/hooks/hooks.json`.
- `/clear` in a live session stops the loop and leaves no `wait` child.
- Under a prompting mode with no allow rule, whether a mod `$.mcp.call` raises
  a dialog or refuses (decides whether the knob is still needed).
- `$.process` in the desktop app's local Code tab and IDE-hosted sessions.

---
title: Claude mailbox active push through a plugin mod
related:
  261016-research-claude-mod-mailbox-active-push: source research; its Outcome Ledger is this ticket's authority
  260913-feat-cross-session-mailbox-wake: wake path this extends; its Claude Stop-hook adapter is retired here
  260919-feat-mailbox-arm-wait-harness-include: arm-the-wait include whose Claude wording changes here
  260914-feat-ws-pi-mailbox-native-steer-push: pi active push this mirrors on Claude
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d783802a963d35a7
sage-review-completeness-reviewed: d783802a963d35a7
completed: 2026-10-17
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
  3 timed out, 130 interrupted, other error). An error exit backs off from 1s
  doubling to 60s, and an empty drain after exit 0 backs off 2s, as the
  probe's `watcher.ts`. Rejected: an immediate respawn, which spins on a
  persistent failure.
  Rejected: a streaming `ws-mcp mailbox watch` subcommand (a restart loop is
  needed around any child anyway); a host-neutral MCP `mailbox.watch` tool (the
  MCP server cannot start a Claude turn); Claude Code Monitor as the wake
  (expires after at most 30 minutes, an idle turn per re-arm).
- **Explicit bind.** The agent binds by calling a mod-registered tool with its
  own session key; the binding is not inferred from other ws tool calls. A
  call carrying `agentId` (a subagent or teammate loop) is refused with an
  error naming the main-session-only rule, as the probe's `checkWatchCall`.
- **Binding lifecycle.** One binding per Claude session, last call wins. The
  same key again is a no-op (covers revive with the preserved key); a different
  key replaces the binding and stops the previous wait loop (covers a new
  `ferrule`). `session.end` with reason `clear` stops it; compaction keeps it.
  Rejected: expiring a binding by detecting re-`ferrule` or revive (a second
  `ferrule` in one worktree is an independent track).
- **Drain mode per package.** `ws` is the opinionated package, `wsflow` the
  conservative one:
  - `ws` always drains through recv itself (`$.mcp.call`, server name from
    `$.mcp.connect`) with no pre-check, and submits the whole drain as one
    prompt, mirroring pi. When the call is refused (a `deny` result), it
    takes the notice path below; any other failed drain (an `isError` result
    or a thrown call) backs off like a wait error and retries, since the mail
    stays queued. Accepted: a prompting-mode user may see a mod-raised recv dialog in
    an idle session (interactive behavior unprobed; see the dogfood checklist).
  - `wsflow` never drains itself. On every arrival it takes the notice path;
    it never raises a permission prompt.
  - Notice path: the mod submits a notice prompt telling the model to call recv
    with its session key, so the model drains through the ordinary permission
    flow. The mod suspends its wait loop and re-arms only when its `tool.call`
    hook sees the model's recv for the bound key complete successfully (the
    hook awaits `next`; a result carrying neither `deny` nor `isError`, since
    an interactive dialog refusal may surface as either), so one arrival
    yields one notice.
  - `$.tool.check` is not used.
  Rejected: pre-checking each drain with `$.tool.check` and draining only on
  `allow` (under auto mode it answers `ask` although the plugin call succeeds,
  so auto users would pay an extra model tool call per arrival); a
  `config.tune` direct-drain knob (`claude.mailbox_direct_drain`; replaced by
  the per-package split); a self-granting `tool.check` hook (overrides the
  user's policy and leans on engine looseness a release may close); gating on
  detecting auto mode (a bind-time read goes stale; `prompt.attachment`
  `auto_mode` latches stale while idle, in the unsafe direction); re-arming
  when the model's recv starts (a declined dialog would re-arm over queued mail
  and loop notice, dialog, notice); re-arming at the next `turn.complete`
  after a notice (a notice every turn); one push per envelope (one batch per
  drain, as pi settled in
  `261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers`).
- **Pi is unchanged**: its adapter keeps auto-arming and draining.
- **No double-binding guard.** Installing both `ws` and `wsflow` in one Claude
  Code is a user error and out of scope. Rejected: skipping the bind when
  another process's listening marker holds the key (adds a race, couples the
  mod to the ws cache layout).
- **Retire the Claude `Stop` hook adapter in this ticket**: the `Stop` entry in
  `agents-plugin/hooks/hooks.json`, the `ws-mcp mailbox claude-stop-hook`
  subcommand and its tests, the hooks file description's Stop sentence, and
  Claude Stop-hook adapter mentions in the
  `agents-plugin-tool/internal/wsmailbox/hook_peek.go` doc comments (for
  example "both Stop-hook adapters"); Codex Stop-hook mentions, such as the
  one in `listening.go`, stay.
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
- **One mod source, mirrored.** The mod source lives in `agents-plugin/hooks/`
  and picks its drain mode from the engine-reported plugin name (`ws` direct,
  `wsflow` notice-only);
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

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- The mod API is early access; the Claude Code build's own
  `plugin-authoring` declaration file (`types/claude-code.d.ts`) is the
  authority, and contradicts its prose in places (`$.mcp.call` is
  permission-gated at runtime despite "No permission prompt").
- Codex behavior is unchanged: `codex-stop-hook`, the base `arm-the-wait.md`,
  and the Codex manifests stay as they are.
- `install.sh` still installs only `ws` into Claude; no change.

## Prior Decisions

- 261016-research-claude-mod-mailbox-active-push (2026-10-16, Confirmed Decisions): "The same actionable child that ships the mod retires the Claude `Stop` hook adapter: the `Stop` entry in `agents-plugin/hooks/hooks.json`, the `ws-mcp mailbox claude-stop-hook` subcommand and its tests" — bearing: supports
- 1f59a3a4 (2026-10-17, commit): "User chose one child for mod + Stop hook removal over a separate chore ahead of it ('removal is quick'), so the removal lands with its replacement." — bearing: supports
- fd4e7675 (2026-10-17, commit): "Reverses the 2026-10-16 '$.mcp.call never prompts' decision on probe evidence (Claude Code 2.1.300; artifacts cherry-picked to ai-docs/ref/claude-mcp-call-probe-mod/ as 5ae1c2987)." — bearing: constrains
- 1f59a3a4 (2026-10-17, commit): "Double-binding guard rejected: installing ws and wsflow together is a user error; recv's consume-once semantics already prevent double delivery in that case." — bearing: supports
- 261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers (2026-10-09, e5202ac3): "the waiter admits each drain as one unit and `sendBatchToLead` queues every item before one batch submit, so a busy lead receives one steer carrying all drained envelopes." — bearing: supports
- 261008-bug-windows-mailbox-wait-outlives-pi-host (2026-10-08, 2186143a): "the watch arms on every Windows mailbox wait, including Claude/Codex background waits started through a shell, so a wait now ends when its direct parent exits." — bearing: constrains
- 0c42008d (2026-10-08, commit): "a Pi lead whose revive adopted a different key kept re-spawning `mailbox wait` with the session_start key, so every 10m re-arm warned `named inbox ... is not currently owned by this --session-key`" — bearing: constrains
- 261016-research-claude-mod-pi-like-compaction (2026-10-16, Confirmed Decisions): "Research only for now; no actionable child until the Claude mailbox mod (`261016-research-claude-mod-mailbox-active-push`) has validated the mod path." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/, agents-plugin-wsflow/hooks/, agents-plugin/rsrc/lead-use-mailbox/, agents-plugin-tool/cmd/ws-mcp/mailbox.go, agents-plugin-tool/internal/wsmailbox/ |
| scope.surface | cross-module | new mod, rsrc variant, and ws-mcp CLI subcommand removal across ws, wsflow, and the Go tool |
| scope.new_public_symbol | yes | mod-registered mailbox_watch tool |
| scope.new_type_contract | yes | mailbox_watch input {session_key} |
| scope.test_surface | new-files | engine-backed mod tests, wsflow hooks drift test, concurrent mailbox_recv Go test; existing agents-plugin/tests/test_claude_hooks_manifest.py and agents-plugin-tool/cmd/ws-mcp/mailbox_claude_hook_test.go are edited or removed; search for a concurrent recv test returned nothing |
| complexity.reuse_points | confirmed | probe mod at ai-docs/ref/claude-mailbox-push-probe-mod/ and agents-plugin-pi/src/mailbox-waiter.ts |
| complexity.side_effect_risk | high | removes a shipped hook and CLI subcommand, and adds a mod that submits prompts and drains mail in live sessions |
| risk.correctness | high | wait-loop lifecycle, rebind, suspend and re-arm on observed recv, and consume-once drain races are all stateful |
| risk.fit | moderate | early-access mod API mirrored across two packages, with a documented fallback to the shell wait |
| risk.test | high | key behaviors need an attended interactive session; the worker verifies only through engine-backed tests |
| risk.security_or_contract | high | ws drains without a pre-check and may raise a recv dialog in prompting modes; wsflow must never prompt; shipped hook and CLI surface changes |

## Phases

### Phase 1: Claude mailbox push mod in ws and wsflow, Stop hook retired

Implement every decision above in one landing, so the Stop hook adapter is
removed together with its replacement.

Verification the worker runs:

- `claude plugin validate` and `claude plugin test` pass for both
  `agents-plugin` and `agents-plugin-wsflow`.
- Engine-backed mod tests cover: bind; same-key no-op; a different-key rebind
  stops the previous wait child; `session.end` `clear` stops the loop;
  subagent bind refused (`agentId` set); `ws` drains once and submits one
  batch; `ws` with a refused drain submits one notice and suspends; `wsflow`
  submits one notice per arrival and never calls recv; after a notice the loop
  re-arms only when an engine-origin recv for the bound key completes
  successfully (the mod's own call, a `deny` result, and an `isError` result
  do not re-arm); an
  error exit backs off instead of respawning at once.
- Go tests for the Stop hook removal pass, the concurrent-recv test passes,
  and the wsflow and pi mirror guards pass.

Post-merge dogfood checklist (attended interactive session, not worker-run;
the worker cannot drive one, and headless `claude -p` ends after one turn):

- Mail arriving mid-turn queues its submit as its own turn after the running
  one.
- The classic `hooks` and the `modules` both run from one
  `agents-plugin/hooks/hooks.json`.
- `/clear` in a live session stops the loop and leaves no `wait` child.
- Under a prompting mode with no allow rule, whether the `ws` mod's
  `$.mcp.call` raises a dialog, refuses, or blocks with no dialog shown (what
  a prompting-mode `ws` user sees; a silent block would stall the push and
  call for a bound on the drain wait).
- A declined recv dialog after a notice leaves the loop suspended, with no
  repeated notice.
- `$.process` in the desktop app's local Code tab and IDE-hosted sessions.

### Result (79c1d0a80) - 2026-10-17

Landed in 0acdd4a55..79c1d0a80:

- Mod `agents-plugin/hooks/mailbox-push/` (`register.ts`, `watcher.ts`,
  `engine.test.ts`, `watcher.test.ts`), with a byte-identical copy in
  `agents-plugin-wsflow/hooks/mailbox-push/` pinned by
  `TestWsflowHooksMirrorUpToDate`. ws `hooks.json` drops `Stop` and lists the
  module; wsflow `hooks.json` lists only `modules` (99f22d9d3, 79c1d0a80).
- `ws-mcp mailbox claude-stop-hook` and its tests removed; `hook_peek.go`
  comments are Codex-only (3027fddc7, test deletion in 5fac273bd).
- `TestMailboxRecvConcurrentCallsNeverDoubleDeliver` passes with no recv fix:
  drains are flock-serialized (5fac273bd).
- `lead-use-mailbox/arm-the-wait.claude.md` variant; rsrc manifest plus the
  wsflow and pi rsrc mirrors regenerated; composition test extended
  (726155235). `wsflow-mirroring.md` records the hooks carve-out (b44aea530).

Verification: `claude plugin validate` passes for both packages;
`claude plugin test` gives 30 pass, 0 fail for each; `tsc` against the
2.1.302 types is clean; `go test ./internal/wsrsrc/... -count=1`,
`./cmd/ws-mcp/... ./internal/wsmailbox/...` pass; the concurrent recv test
passes under `-race -count=20`; `go test ./internal/mcp -count=1` passes;
`python3 -m unittest discover` passes for `agents-plugin/tests` and
`agents-plugin-wsflow/tests`.

Decisions taken at implementation:

- The bound key is persisted in `$.store` under `mailbox-watch:<session id>`
  instead of `$.state`: a `$.state` ref must spell its owning plugin as a
  literal, which a source shared by ws and wsflow cannot (`claude plugin
  validate` refused the dual-literal form). The entry is deleted at every
  `session.end`.
- The ws drain calls `$.tool.call` on the recv tool resolved from
  `$.mcp.connect` instead of `$.mcp.call`: a refusal comes back as a
  structured `{deny}` (the notice path), while `$.mcp.call` rejects with an
  unstructured message. Research runs 2, 6, and 7 show the same permission
  behavior for both.
- The wait passes `--slug` from `WS_MAILBOX`, which the retired Stop hook
  read, so a session with a named inbox is woken by mail to it. The wait
  owner-checks the slug and falls back to reply-id-only.
- `$.process` availability is probed with `python3 --version` at
  `session.start`; the tool is not registered when the probe fails.
- `session.end` stops the loop for every reason, not only `clear`. On
  `resume` the process continues under another session id, and a loop still
  bound would drain old-key mail into the new conversation. Compaction fires
  no `session.end`, so it keeps the binding (lite review fix).

Residual: a `WS_MAILBOX_AUTO` address is not known to the mod, so its named
inbox does not wake the loop; recv still drains it. A `prompt.submit`
failure after a successful ws drain leaves that mail logged only to the
debug log.

## Sage Review Round 1 (2026-10-17)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The knob read itself can raise the permission prompt the design forbids | important | missing |
| 2 | Re-arm trigger after a declined recv dialog is unspecified | important | autonomous |
| 3 | Subagent bind denial is tested but not stated as a decision | minor | autonomous |
| 4 | Leftover Stop-hook references and the wsflow launcher copy are not named | minor | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Knob read may raise a permission prompt on the default path | important |
| 2 | Wait-loop behavior on other error exit is unspecified | minor |
| 3 | Subagent bind denial appears only in the verification list | minor |
| 4 | Single large phase | minor |

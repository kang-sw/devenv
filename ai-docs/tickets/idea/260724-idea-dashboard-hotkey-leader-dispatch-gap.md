---
title: "Dashboard leader-hotkey (which-key) resolve dispatches through the command bus but most leaf commands' real side effects never fire"
related:
  260722-feat-dashboard-hotkey-config-framework: owns the leader-press
    resolve/dispatch mechanism this ticket found a gap in
  260722-feat-dashboard-which-key-hint-overlay: Phase 2 browser verification
    of the overlay surfaced this gap while proving a matched leader-sub
    sequence "resolves"
---

## Symptom

Found during `260722-feat-dashboard-which-key-hint-overlay` Phase 2 Playwright
verification (2026-07-24). Driving `<leader> t n` (`terminal.create`) through
the real global `Ctrl+Space` leader-key listener in the browser: the which-key
overlay correctly narrows into the `t` group, correctly resolves and
disappears on `n`, and the command is observably dispatched (visible via the
`.workbench-toolbar[data-last-command-id]` test hook, which reads the same
`dispatchDashboardCommand` observer every other command-routed control in this
codebase relies on) — but no new terminal pane is ever created. The advertised
action silently no-ops.

## Root Cause (confirmed by reading `App.tsx`)

`App.tsx`'s global leader-key `keydown` listener (from
`260722-feat-dashboard-hotkey-config-framework` Phase 1) resolves a matched
leader-sub binding and calls the shared `executeCommand(command)` with no
extra `handlers` argument. `executeCommand`'s body only populates
`executableHandlers[command.commandId]` for a fixed subset of payload types:
`select`, `refresh`, `workRoot.activation.set`, `gitWorktreeAdd.open`,
`gitWorktreeAdd.close`, `worktreeRemove.open`, `worktreeRemove.close`,
`settings.open`, `settings.close`, `worktree.hide`, `worktree.unhide`,
`worktreeHidden.menu.open`, `workRoot.close`, `server.off`,
`workspace.remove`.

Every other default leaf binding's real side effect is wired only at its own
UI control's *local* `onCommand(command, { <id>: <handler> })` call site (e.g.
the terminal-create toolbar button passes `{"terminal.create": onCreateTerminal}`
locally) — never merged into the App-level shared `executeCommand`. Because
the leader listener calls `executeCommand(command)` directly instead of
routing through those same local call sites (or otherwise obtaining their
handler), any matched leader-sub binding whose commandId isn't in the fixed
list above dispatches (reaches the observer/command-log) but performs no real
action.

Affected default bindings (leaf commandId not in the shared list above, so
`<leader> ...` no-ops today): `rootPicker.open` (`r o`), `terminal.create`
(`t n`), `agentChat.create` (`a n`), `git.refresh`/`git.fetch`/`git.push`/
`git.pullFfOnly`/`git.branchMenu.open`/`git.branchCreate.open` (`g r`/`g f`/
`g p`/`g l`/`g b`/`g c`), `workspace.menu.open` (`g w m`).

Unaffected (real side effect fires correctly): `workRoot.close` (`r x`),
`workRoot.activation.set` (`r t`), `gitWorktreeAdd.open` (`g w a`),
`workspace.remove` (`g w x`), `worktreeHidden.menu.open` (`g w h`).

## Impact

Roughly two-thirds of the finalized default keymap's leaf bindings
(`260722-feat-dashboard-hotkey-config-framework`'s "Default Keymap &
Interaction Spec") are currently inert when triggered via the leader key,
despite the which-key overlay correctly advertising them as available actions
and the framework's own dispatch resolving/logging them. This is silent: no
error, no visible feedback — the overlay just closes as if the action ran.

## Suggested Fix Direction (not decided here)

Likely needs the leader-listener's `executeCommand` call (or `executeCommand`
itself) to also route through whichever local handler map each control
registers, or for a single App-level dispatch table to become the sole source
of truth for every `DashboardCommandId` and have per-control click handlers
call into it instead of the reverse. Left to implementation triage on this
ticket; not resolved by `260722-feat-dashboard-which-key-hint-overlay` Phase 2,
which only verifies the overlay's own appear/narrow/dismiss UI lifecycle and
deliberately did not weaken its dismissal-path-1 assertion to hide this gap
(it asserts dispatch via the command-log hook instead of the broken real side
effect).

## Reporter Context

Found live during `dashboard-acceptance.spec.ts`'s new which-key overlay
Playwright step; reproduced by pressing `Ctrl+Space t n` in a real paired
browser session against a real daemon and observing no new terminal pane
despite a successful `data-last-command-id="terminal.create"` dispatch.

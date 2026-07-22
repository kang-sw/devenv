---
title: Dashboard hotkey configuration framework (leader-only, Ctrl+Space)
parent: 260711-epic-ws-dashboard-command-surface
related:
  260711-idea-dashboard-command-bus-quick-open-shortcuts: existing global
    shortcut-capture layer (Phase 1) and terminal passthrough guard pattern
    this framework binds onto and reuses, respectively
  260722-feat-dashboard-which-key-hint-overlay: next layer in the sequence;
    depends on this framework's binding registry
  260722-feat-dashboard-hint-click-fast-jump: last layer in the sequence;
    depends on this framework's binding registry
related-mental-model:
  - ws-web-dashboard
---

# feat: Dashboard hotkey configuration framework (leader-only, Ctrl+Space)

## Background

Agenda A UX review (owner, 2026-07-22) settled the dashboard's keyboard
interaction model and produced a decision set that is final and must be
recorded, not re-litigated. This ticket is the first implementation target:
the hotkey configuration framework that every other binding (leader-sub
actions, standalone shortcuts, the command bar, and later layers) registers
through.

`260711-idea-dashboard-command-bus-quick-open-shortcuts` already plans a
global shortcut-capture layer in its Phase 1 and specs the VSCode-style
command-bar prefix grammar. This ticket does not rewrite either; it gives
that shortcut-capture layer a formal binding-registration substrate and
defines the leader-key model it must implement.

## Decisions (final - Agenda A, 2026-07-22)

- **Interaction model: tmux-style leader-only, no modal.** The dashboard does
  not adopt a neovim-style normal/insert mode split. Instead, a leader key
  press enters a transient "dashboard command mode"; a following key (or
  short key sequence) resolves to a bound action, then the transient mode
  ends. There is no persistent mode the user must explicitly exit.
- **Leader key = `Ctrl+Space`.**
- **Binding schema:** leader-sub bindings (`<leader>` followed by a key or
  short sequence) are the first-class surface every default action is
  expressed through. Standalone (non-leader) hotkeys are a user-configurable
  layer on top — a user may bind a standalone key to any command-bus action,
  but the shipped defaults express themselves as leader-sub bindings.
- **Reserved keys** (must not be claimed by default framework bindings,
  because they are already meaningfully bound elsewhere in the dashboard):
  - `` Ctrl+` `` - terminal focus.
  - `Ctrl+R` - reverse-history-search (terminal/shell convention).
  - `Ctrl+G` and `Ctrl+Enter` - managed-CLI terminal
    (`260624-feat-ws-dashboard-managed-cli-terminal`).
- **`Ctrl+Space` caveat (recorded, accepted):** in shells using emacs-style
  keybindings, `Ctrl+Space` is conventionally `NUL` / `set-mark`. Capturing
  `Ctrl+Space` globally as the dashboard leader means a terminal pane loses
  its usual `set-mark` binding while the dashboard's global capture layer is
  active. This is judged acceptable because the leader key itself is
  user-configurable (a user who needs `set-mark` can rebind the leader), but
  the tradeoff must stay visible in documentation and must not be
  "discovered" later as an unrecorded regression.
- **Terminal-vs-dashboard passthrough** reuses the existing guard pattern
  described in `260711-idea-dashboard-command-bus-quick-open-shortcuts`
  (Background section, referencing `App.tsx:6711-6768`): skip capture when
  `isComposing` (IME), when the event target is `input`/`textarea`/
  `contentEditable`, and when focus is already inside the terminal container
  (including its `offsetParent` visibility check). Terminal raw byte input
  stays the deliberate exception this guard exists to protect — the
  framework must not weaken it while adding the leader-capture layer.
- **Dispatch spine:** bindings resolve to an existing `DashboardCommandId` +
  payload and hand off to the existing `commands.ts` / `DashboardCommand` /
  `executeCommand` bus (`App.tsx:884`). The framework registers *onto* the
  global shortcut-capture layer already planned in
  `260711-idea-dashboard-command-bus-quick-open-shortcuts` Phase 1; it does
  not introduce a second, competing capture layer.
- **Persistence:** user hotkey configuration (leader-sub rebindings and any
  user-added standalone hotkeys) must be persisted. The exact storage
  location (e.g. workroot-scoped `.ws-dashboard/` state, a global dashboard
  settings store, or browser-local storage) is an **open design point**,
  left to implementation, not decided by this ticket.
- **Architecture principle:** implement sequentially, but leave room for the
  full layer stack. This framework must expose a binding-registration API
  general enough that later layers (which-key hint overlay, command bar,
  hint-click/fast-jump) can register against it without a rewrite. The full
  intended layer sequence (see parent epic) is:
  1. Hotkey configuration framework (this ticket).
  2. Leader + which-key-style hint overlay
     (`260722-feat-dashboard-which-key-hint-overlay`).
  3. Command bar (`260711-idea-dashboard-command-bus-quick-open-shortcuts`,
     already specced — integrates with, does not replace, this framework).
  4. Hint-click / fast-jump
     (`260722-feat-dashboard-hint-click-fast-jump`).

## Non-Goals

- Rewriting the command bar's prefix grammar (`>`, `@`, `#`, `:`, `%`, `!`)
  specced in `260711-idea-dashboard-command-bus-quick-open-shortcuts` — this
  ticket only gives that layer's shortcut-capture mechanism a formal binding
  registry to sit on.
- Deciding the persistence storage location — tracked as an open point above,
  resolved during implementation.
- Building the which-key hint overlay, command bar integration, or
  hint-click/fast-jump layer — each is its own ticket, sequenced after this
  one.

## Phases

### Phase 1: Binding registry and leader-press dispatch

- Define a hotkey binding registry (leader-sub bindings as the first-class
  entries, standalone hotkeys as an additive layer) that maps a bound key or
  key sequence to a `DashboardCommandId` + payload, mirroring the shape
  `commands.ts` already expects from `executeCommand`.
- Implement `Ctrl+Space` leader capture on top of the existing (or
  concurrently landing) global shortcut-capture layer from
  `260711-idea-dashboard-command-bus-quick-open-shortcuts` Phase 1: pressing
  the leader enters a transient dashboard-command-mode window that resolves
  on the next matching key/sequence or times out/cancels on an unmatched key
  or explicit cancel (e.g. Escape), consistent with the no-modal, tmux-style
  model.
- Reuse the terminal passthrough guard pattern verbatim (IME/`isComposing`,
  input/textarea/contentEditable target check, terminal-container focus +
  `offsetParent` visibility check) so terminal raw byte input is never
  intercepted by leader capture.
- Ship the reserved-key policy as data the registry enforces: default
  bindings must not claim `` Ctrl+` ``, `Ctrl+R`, `Ctrl+G`, or `Ctrl+Enter`.
- Document the `Ctrl+Space`/emacs `set-mark` caveat in user-facing
  documentation (or in-app help) so it is discoverable, not just recorded
  here.
- Decide and implement the persistence mechanism for user hotkey
  configuration (open design point above resolves here).
- Verify: default leader-sub bindings register and dispatch correctly through
  `executeCommand`; reserved keys are rejected if a default or user config
  attempts to bind them; terminal passthrough is unaffected (existing
  terminal-focus and control-key behavior keeps working); leader-press-then-
  unmatched-key cancels cleanly without leaking into terminal input; user
  rebindings persist across a reload.

---
## Proposed Default Keymap & Interaction Spec (DRAFT — pending owner review, 2026-07-22)

> Status: draft awaiting owner sign-off on mnemonics and the R2 prerequisite command ids. Grounded in the actual `DashboardCommandId` inventory in `ws-dashboard/frontend/src/commands.ts`. `▸` marks a payload-needing action that opens a menu/picker rather than executing directly (see Rule R1).

Leader = `Ctrl+Space`. Model: leader → group key → action key (two-step mnemonic).

### Top-level (no group)
| keys | action |
|---|---|
| `<leader> <space>` | open command palette (command bar) |
| `<leader> f` | hint-click / fast-jump mode |
| `<leader> ?` | which-key: show all bindings |

### Groups
- `g` Git — `r` git.refresh · `f` git.fetch · `p` git.push · `l` git.pullFfOnly · `b` git.branchMenu.open ▸ · `c` git.branchCreate.open ▸
- `w` Worktree/WorkRoot — `a` gitWorktreeAdd.open ▸ · `x` workspace.remove (always-confirm modal) · `o` workRoot.open · `c` workRoot.close · `t` workRoot.activation.set (online/offline) · `m` workspace.menu.open ▸ · `h` hidden-worktrees submenu ▸
- `a` Agent-chat — `n` agentChat.create · `s` agentChat.prompt.send · `y` agentChat.history.open ▸ · `f` agentChat.bubble.forkFromHere · `r` agentChat.bubble.resumeFromHere · `c` agentChat.bubble.copy · `k` agentChat.thinking.toggle
- `t` Terminal — `n` terminal.create · `x` terminal.close  (scroll/clear/copy = GAP, see R2)
- `d` Document — `s` document.save · `r` document.revert · `e` document.mode.set(edit) · `v` document.mode.set(view) · `t` document.translation.toggle · `x` close editor (GAP, see R2)
- `p` Pane focus (GAP — needs `pane.focus.<kind>`, see R2) — `a` Agent · `t` Terminal · `c` Agent Chat · `e` Editor · `v` Viewer · `d` Diff · `g` Diagnostics · `k` Task · `i` Inspector
- `v` View toggle (`workbench.toggle.*`) — `v` viewer · `t` task · `d` diagnostics · `e` events · `l` layout
- `r` Root picker — `o` rootPicker.open

### which-key overlay behavior
- On leader press, enter a transient "pending" capture mode; subsequent keys are captured by command mode and NOT forwarded to terminal/inputs.
- Overlay appears after a configurable delay (default 250ms) as a bottom-docked panel (lazyvim which-key style), listing available next keys grouped: group keys as `key → +group`, leaf keys as `key → <label>` (labels from `dashboardCommandLabel`).
- Pressing a group key replaces the overlay with that group's leaves. `Esc` or a second leader press cancels and exits. An unbound key = brief flash then exit.
- No auto-timeout by default (stays until a binding resolves or Esc); configurable.

### hint-click / fast-jump behavior
- Trigger `<leader> f`. Labels every in-viewport, non-occluded actionable element (has `data-command-id` / focusable) with home-row labels (default alphabet `fjdksla;gh...`, two-char combos when needed).
- Type label chars to filter; a unique match activates (dispatches its command id / clicks). `Enter` activates the highlighted target; `Esc` cancels.
- Scope spans all visible panes plus the left nav (including other servers' worktree rows) — this enables the "jump from terminal to another server's worktree" use case.
- Performance-gated: only in-viewport/non-occluded targets; default cap (e.g. 150 targets); beyond the cap, restrict to the focused pane + nav. Alphabet and cap are configurable.

### Design rules
- **R1 — no invented targets.** A leader-sub binding resolves to either a no-payload command (execute directly) or, for a payload-needing action (branch switch, worktree/workspace select, file open, etc.), a command that opens the relevant menu/picker (marked `▸`); existing in-menu navigation then makes the selection. The keymap never fabricates the target selection.
- **R2 — prerequisite command ids (currently GAPS).** These have no command id today and must be added for their leader targets to work, so implementation does not silently drop them: `pane.focus.<kind>` (focus a workbench surface); tab next/prev + cycle; terminal scroll / clear / copy-selection; editor next/prev-file + close; left-nav row select. List them as prerequisites of this framework.

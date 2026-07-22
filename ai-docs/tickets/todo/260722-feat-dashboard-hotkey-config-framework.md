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

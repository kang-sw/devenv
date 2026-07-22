# Plan: 260722-feat-dashboard-hotkey-config-framework — Phase 1: Binding registry and leader-press dispatch

## Relevant Ticket Contract

- Leader-only, no modal: `Ctrl+Space` press enters a transient "dashboard
  command mode"; the next matching key/sequence resolves an action, then the
  transient mode ends (Decisions).
- Binding schema: leader-sub bindings (`<leader>` + key/short sequence) are
  first-class; standalone (non-leader) hotkeys are an additive
  user-configurable layer on top (Decisions).
- Reserved keys that default bindings must never claim: `` Ctrl+` ``,
  `Ctrl+R`, `Ctrl+G`, `Ctrl+Enter` (Decisions) — enforced as *data* the
  registry checks (Phase 1 bullet), not just a doc note.
- Terminal-vs-dashboard passthrough must reuse the existing guard pattern
  **verbatim**: skip capture on `isComposing` (IME), on
  `input`/`textarea`/`contentEditable` targets, and when focus is already
  inside the terminal container (incl. `offsetParent` visibility check)
  (Decisions, Phase 1 bullet).
- Dispatch spine: bindings resolve to an existing `DashboardCommandId` +
  payload and hand off to the existing `commands.ts` / `DashboardCommand` /
  `executeCommand` bus at `App.tsx:1157` (Decisions).
- The framework must expose a binding-registration API general enough that
  later layers (which-key overlay, command bar, hint-click) can register
  against it without a rewrite (Decisions, "Architecture principle").
- Persistence: user hotkey configuration must persist across reload; the
  storage location is an **open design point, left to implementation**
  (Decisions, Non-Goals) — not a decision this plan should invent policy
  around, only implement.
- Phase 1 "Verify" bullet (ticket's own acceptance list): default
  leader-sub bindings register/dispatch through `executeCommand`; reserved
  keys are rejected for both default and user config; terminal passthrough
  unaffected; leader-press-then-unmatched-key cancels cleanly without
  leaking into terminal input; user rebindings persist across reload.
- Finalized keymap spec (`## Default Keymap & Interaction Spec`) is binding
  *design* authority but Phase 1 scope is the **framework**, not wiring
  every concrete binding — ticket instructions for this task confirm this
  explicitly.
- R1 (no invented targets): a leader-sub binding either executes a
  no-payload command directly or opens the relevant menu/picker for a
  payload-needing action (`▸`); it never fabricates a selection.
- Timeout wording conflict inside the ticket itself: the Phase 1 bullet says
  a pending leader mode "times out/cancels on an unmatched key or explicit
  cancel", while the finalized spec's which-key section says "No
  auto-timeout by default (configurable)." Treat the finalized spec (dated
  same day, explicitly marked final) as authoritative: implement timeout as
  a configurable option defaulting to **off**; unmatched-key and `Escape`
  remain the default cancel paths.

## Out of Scope

- All R2-listed GAP prerequisite command ids: workRoot flat/hierarchical
  select-by-index, `pane.focus.<kind>`, tab next/prev/cycle, terminal
  scroll/clear/copy-selection, editor next/prev-file/close, left-nav row
  select, focus-git-status-inspector. The registry's key/sequence → command
  mapping must accept arbitrary future `DashboardCommandId` strings so these
  slot in later without a rewrite, but Phase 1 does not implement or wire
  any binding that depends on them.
- Wiring the full default keymap (all of groups `g`/`r`/`a`/`t`/`d`/`p`/`v`
  plus top-level `1..8`/`f`/`space`/`?`). Phase 1 ships the registry +
  leader-press dispatch mechanism plus only the subset of bindings that
  already resolve to a real `DashboardCommandId` today (see Codebase
  Findings), enough to prove the dispatch path end-to-end.
- Which-key hint overlay UI/popup rendering — separate ticket
  `260722-feat-dashboard-which-key-hint-overlay`.
- Hint-click/fast-jump — separate ticket
  `260722-feat-dashboard-hint-click-fast-jump`.
- Command bar prefix grammar (`>`, `@`, `#`, `:`, `%`, `!`) — owned by
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`; this framework
  only gives its shortcut-capture layer a registry to sit on.
- Redesigning `DashboardCommand`/`executeCommand` itself (both tickets'
  Non-Goals agree on this).
- Deciding persistence storage location as a matter of *policy* — this plan
  picks a concrete mechanism to implement (browser-local, versioned) because
  the ticket delegates the choice to implementation, but does not treat it
  as a locked-in architectural decision beyond Phase 1.

## Codebase Findings

- `ws-dashboard/frontend/src/commands.ts:2-45` — `DashboardCommandId` union
  is the authoritative command-id inventory. Cross-checking it against the
  finalized keymap's leaf actions shows the keymap assumes several ids that
  **do not exist** in this union today, beyond the ticket's own R2 list:
  `agentChat.prompt.send`, `agentChat.history.open`,
  `agentChat.bubble.forkFromHere`, `agentChat.bubble.resumeFromHere`,
  `agentChat.bubble.copy`, `agentChat.thinking.toggle`. Evidence:
  `grep -n "agentChat\." src/commands.ts` only returns `agentChat.create`.
  These strings currently exist only as `data-command-id` DOM marker
  attributes on click handlers (`src/agentChatBubbles.tsx:163,182,249`,
  `src/agentChatResumeFromHere.tsx:53`, `src/App.tsx:8230,8256`) — i.e. they
  are click-identity markers, not members of the typed union dispatched via
  `executeCommand`. Risk signal: any Phase 1 default binding must target
  only ids that are real `DashboardCommandId` union members; the
  agentChat-bubble group is effectively an undocumented extra gap beyond
  R2's list and should be called out as such rather than silently wired or
  silently dropped.
- `ws-dashboard/frontend/src/App.tsx:1157-1160+` — `executeCommand` is a
  `useCallback` closing over live component state (`selectedServerIdRef`,
  etc.) defined inside the `App` component. A global leader-dispatch effect
  needs access to this same closure (for contextual payloads such as "the
  currently selected workRootId") the same way existing click handlers do,
  which means the effect that *calls* `executeCommand` must live in
  `App.tsx` (or receive `executeCommand` + context via props), not in a
  fully decoupled module. This mirrors the existing split below.
- `ws-dashboard/frontend/src/keydownSuppression.ts` (whole file, 36 lines)
  — direct precedent for the module split to copy: a pure, DOM-free
  predicate module (`shouldSuppressBrowserShortcut`, taking a minimal
  `{ctrlKey, metaKey, key, targetIsEditable}` shape) plus a thin `App.tsx`
  `useEffect` (`App.tsx:707-731`) that reads real DOM/event state into it
  and calls `preventDefault()`. `SUPPRESSED_CTRL_KEYS` is a plain
  `Set<string>` — same shape precedent for this ticket's reserved-key set.
  Companion test `keydownSuppression.test.ts` is a plain Node script using
  a hand-rolled `assertEqual`, no test framework — copy this shape for the
  new module's tests.
- `ws-dashboard/frontend/src/App.tsx:8689-8746` — the terminal pane's own
  `keydownFallback` (`window.addEventListener("keydown", ...)`, bubble
  phase) contains the exact guard clauses the ticket says to reuse
  verbatim: `!container.offsetParent` early-return (visibility),
  `event.isComposing || event.key === "Process" || composingInput`
  (IME), `target?.isContentEditable || tagName === "input"/"textarea"/
  "select"` (editable target), and `container.contains(document.activeElement)`
  (focus already inside terminal). Important: this listener is **scoped to
  one terminal pane's own `containerRef`**, not global. The new leader
  listener is global (one document/window listener for the whole app), so
  the "focus already inside a terminal" check must be generalized —
  `ws-dashboard/frontend/src/App.tsx:9115-9121` shows every terminal pane's
  root is rendered as `<div className="terminal-pane" data-terminal-id=...>`
  wrapping the focusable `.terminal-surface` container, so
  `document.activeElement?.closest(".terminal-pane")` (or a small exported
  helper) is the generalized equivalent of the per-pane `containerRef`
  check.
- `ws-dashboard/frontend/src/App.tsx:707-731` — a second, separate global
  capture-phase `keydown` listener already exists
  (`suppressBrowserShortcut`, ticket `260721-feat-dashboard-suppress-browser-shortcuts`
  Phase 2), deliberately calling only `preventDefault()` and never
  `stopPropagation()`. This establishes precedent that multiple independent
  global keydown listeners already coexist safely in this codebase as long
  as they don't call `stopPropagation()`/`stopImmediatePropagation()`
  against each other; the new leader-capture listener should follow the
  same non-stopPropagation discipline unless it specifically needs to
  swallow a key from reaching the terminal fallback while in pending mode
  (which it does — see next finding).
- **Risk signal / scope-reality mismatch**: the ticket's own Phase 1 bullet
  says to implement leader capture "on top of the existing (or concurrently
  landing) global shortcut-capture layer from `260711-idea-dashboard-command-bus-quick-open-shortcuts`
  Phase 1." That ticket is currently in `ai-docs/tickets/todo/` (not
  `ready/`, not `.done/`) and a repo-wide search
  (`grep -rniE "leader|hotkey|keybinding|keymap" src`) found no existing
  leader/hotkey/keybinding module and no generic global-shortcut-capture
  layer beyond the two narrow listeners above (`dismissOnEscape`,
  `suppressBrowserShortcut`) plus the terminal-specific fallback. There is
  also no existing handler for `` Ctrl+` `` (terminal focus) or `Ctrl+G` /
  `Ctrl+Enter` anywhere in `App.tsx` — the "already meaningfully bound
  elsewhere" reserved-key rationale in the ticket is a forward-looking
  reservation, not a currently-active conflict for those three, except
  `Ctrl+R`, which `keydownSuppression.ts` already suppresses (as browser
  reload) with a comment tying it to "in-app reverse-history-search"
  reservation — consistent with, and independent confirmation of, this
  ticket's `Ctrl+R` reservation. **Practical implication**: Phase 1 of
  *this* ticket must implement the actual leader-press
  `document`/`window`-level `keydown` listener itself (there is no existing
  capture layer to "register onto" yet); it should be structured (e.g. a
  small exported `installLeaderCapture`-style entry point plus the pure
  registry module) so `260711`'s later shortcut-capture work can share or
  call into it instead of the two tickets independently building competing
  listeners.
- `ws-dashboard/frontend/src/workRootFiles.ts:786-792` (`browserStorage`)
  and `:460-520` (`loadReadOnlyFilePaneRestoreSnapshot` /
  `saveReadOnlyFilePaneRestoreSnapshot`) — the established persistence
  precedent: `browserStorage()` returns `window.localStorage` guarded by
  try/catch (SSR/disabled-storage safe), paired load/save functions take an
  injectable `storage` parameter (defaulting to `browserStorage()`) for
  testability, and stored JSON is version-tagged (`{version: 1, ...}`) with
  a defensive parser that silently drops anything malformed or
  version-mismatched rather than throwing. `workNavOrder.ts:122` and
  `terminals.ts:722` show the storage-key naming convention:
  `"ws-dashboard.<feature>.v<N>"` (e.g. `"ws-dashboard.workNavOrder.v1"`).
  This is the natural default to implement for the ticket's open
  persistence-location point — it directly matches three existing
  precedents and needs no daemon/backend change, though the ticket does not
  lock this in as final policy beyond what Phase 1 needs to ship.
- `ws-dashboard/frontend/package.json` `scripts` — every pure-module test
  file is wired as `"test:<name>": "tsc -p tsconfig.route-tests.json && node ./node_modules/.tmp/route-tests/<name>.test.js"`
  (e.g. `"test:keydown-suppression"`). `ws-dashboard/frontend/tsconfig.route-tests.json`
  `include` is an **explicit file allowlist**, not a glob for the whole
  `src/` tree — a new module/test pair (e.g. `hotkeys.ts` +
  `hotkeys.test.ts`) will silently be skipped by `tsc -p
  tsconfig.route-tests.json` unless both files are added to that `include`
  array, and a matching `npm run test:hotkeys`-style script must be added
  to `package.json` for it to run in CI/dogfood the same way as its
  siblings.
- `ws-dashboard/ai-docs/mental-model/ws-web-dashboard.md` (Domain Rules,
  line 16) — "Any ws-dashboard frontend implementation that changes visible
  browser UI must include browser-level visual/interaction verification,
  preferably Playwright... pure TypeScript tests, Vite build, and
  curl/API dogfood are not sufficient to close UI-facing work." Phase 1 adds
  no new visible chrome (no popup — that's the which-key ticket), but it
  does change global interactive keyboard behavior (leader capture must not
  regress terminal passthrough). This is a verification-boundary judgment
  call flagged below rather than decided here.

## Implementation Plan

1. Add a new pure, DOM-free module `ws-dashboard/frontend/src/hotkeys.ts`
   mirroring the `keydownSuppression.ts` split:
   - `RESERVED_KEYS` data (or a small typed set) covering `` Ctrl+` ``,
     `Ctrl+R`, `Ctrl+G`, `Ctrl+Enter`, expressed the same normalized way as
     `SUPPRESSED_CTRL_KEYS`.
   - A `HotkeyBinding` type: `{ keys: readonly string[]; commandId:
     DashboardCommandId; buildPayload: (ctx) => DashboardCommandPayload }`
     (or static payload) covering both leader-sub sequences (`keys` after
     the leader) and standalone hotkeys (`keys` with no leader).
   - A registration function that rejects (throws or returns an error
     result) any attempt to bind a reserved key, used for both default
     bindings (so a reserved-key default is a build-time/test-time bug) and
     user rebinding attempts (so it is a runtime-rejectable user error).
   - A pure leader-mode state machine: `idle -> pending` on leader keydown;
     `pending` + matching leaf key -> resolved command + back to `idle`;
     `pending` + a key that only narrows a group -> stays `pending` with a
     narrowed subtree; `pending` + unmatched key or `Escape` -> `idle`
     (cancel); optional configurable timeout, default disabled per the
     spec-vs-bullet reconciliation above. Keep this state machine
     independent of any DOM/React types so it stays unit-testable the same
     way `shouldSuppressBrowserShortcut` is.
2. Add a small terminal-focus guard helper (either inside `hotkeys.ts` or a
   tiny shared helper) generalizing the verbatim guard clauses found in
   `App.tsx:8689-8712`: IME (`isComposing`/`"Process"`), editable target
   (`input`/`textarea`/`select`/`isContentEditable`), and terminal-focus via
   `document.activeElement?.closest(".terminal-pane")` in place of the
   per-pane `containerRef.contains(...)` check found at that call site.
3. Add a persistence module (new file, e.g. `hotkeyConfig.ts`, or colocated
   in `hotkeys.ts`) following the `workRootFiles.ts:786-792`/`:460-520`
   pattern verbatim: injectable `storage` parameter defaulting to
   `browserStorage()` (reuse the existing exported helper, do not
   reimplement it), versioned JSON blob keyed
   `"ws-dashboard.hotkeys.v1"`, defensive parse that drops malformed/
   mismatched-version data. Persist only user rebindings/added standalone
   hotkeys (not the full default table) so future default-table changes
   don't require a migration for users who never rebound anything.
4. Wire the framework into `App.tsx` as a new `useEffect`-based global
   `keydown` listener, structured the same way as the existing
   `suppressBrowserShortcut` effect (`App.tsx:707-731`) and coexisting with
   it and with the terminal `keydownFallback` — non-`stopPropagation()`
   except where the leader is actively `pending` and must swallow the next
   key before it reaches the terminal fallback (needed to satisfy the
   ticket's own "leader-press-then-unmatched-key cancels cleanly without
   leaking into terminal input" verify bullet). Call `executeCommand`
   (`App.tsx:1157`) on a resolved match, closing over the same live context
   refs existing click handlers already use for contextual payloads (e.g.
   `selectedServerIdRef`).
5. Register only the subset of default bindings whose target is a real,
   already-existing `DashboardCommandId` (per Codebase Findings) — e.g.
   `git.refresh`/`git.fetch`/`git.push`/`git.pullFfOnly`/
   `git.branchMenu.open`/`git.branchCreate.open`, `gitWorktreeAdd.open`,
   `workspace.remove`, `workspace.menu.open`, `rootPicker.open`,
   `workRoot.close`, `workRoot.activation.set`, `terminal.create`,
   `agentChat.create`, `document.save`/`document.revert`/
   `document.mode.set`/`document.translation.toggle` — enough breadth to
   exercise no-payload dispatch, payload-needing "opens a picker" dispatch
   (R1), and the reserved-key rejection path, without inventing bindings
   for ids that don't exist yet. Explicitly do not wire the
   agentChat-bubble/prompt/history group or any R2 GAP id.
6. Register the new module/test pair in
   `ws-dashboard/frontend/tsconfig.route-tests.json` `include` and add a
   `"test:hotkeys"` script to `package.json` following the
   `"test:keydown-suppression"` pattern exactly.
7. Write `hotkeys.test.ts` (plain Node script, `assertEqual` style, no
   framework) covering: reserved-key rejection for both default-table
   registration and simulated user rebind attempts; leader state-machine
   transitions (idle→pending→resolved, pending→cancel on unmatched key,
   pending→cancel on Escape); persistence round-trip (save → load,
   malformed/version-mismatched blob → falls back to empty); and the
   terminal-focus guard helper's true/false cases mirroring
   `keydownSuppression.test.ts`'s structure.

## Verification Plan

- `cd ws-dashboard/frontend && npm run test:hotkeys` (new script added in
  step 6) — primary correctness gate for the registry/state-machine/
  persistence/guard logic.
- `cd ws-dashboard/frontend && npm run test:commands` — regression check
  that `commands.ts`/`executeCommand` behavior is unchanged (no edits to
  that file's existing exports expected, but the new code depends on its
  types).
- `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite build`) —
  whole-frontend type-check and production build gate.
- Existing terminal-focused suites as a passthrough regression check:
  `npm run test:terminals` and `npm run test:keydown-suppression` (the new
  global listener must not regress either).
- Manual/interactive check (dev server): press `Ctrl+Space` then a wired
  key (e.g. `terminal.create`'s binding) and confirm dispatch; press
  `Ctrl+Space` then an unmatched key and confirm no leak into a focused
  terminal's input; focus a terminal pane and confirm `Ctrl+Space` there
  does not disrupt normal terminal typing outside the deliberately-accepted
  `set-mark` tradeoff recorded in the ticket.
- Flag for lead/reviewer judgment (not resolved by this plan): whether the
  `ws-web-dashboard` mental-model's "visible browser UI... must include
  browser-level Playwright verification" rule applies to Phase 1, which
  changes global interactive keyboard behavior but adds no new visible
  chrome. This plan's default verification stays at the manual dev-server
  check above plus the existing `npm run test:browser` Playwright suite
  staying green (regression only); adding a *new* Playwright case is a
  judgment call left to the executor/reviewer, not decided here.

## Escalations

- None.

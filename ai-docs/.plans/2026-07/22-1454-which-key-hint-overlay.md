# Plan: 260722-feat-dashboard-which-key-hint-overlay — Phase 1: Leader-press hint overlay

## Relevant Ticket Contract

- Render a transient overlay on `Ctrl+Space` leader press listing currently
  reachable leader-sub bindings (key, bound command label), sourced from the
  hotkey framework's binding registry (Phase 1 bullet 1).
- Overlay lifecycle follows the framework's transient dashboard-command-mode
  window: appears on leader press, updates/narrows as the user types a
  partial sequence, dismisses on resolution, timeout, or cancel (Phase 1
  bullet 2).
- Purely additive UI, not a new input-capture path: must not capture or
  consume terminal input itself, must not introduce a second independent
  mode (Decisions; Phase 1 bullet 3).
- Verify boundary: overlay appears within normal input-latency budget;
  reflects live registry contents including user-configured rebindings from
  the framework's persistence layer; disappears cleanly on every dismissal
  path (match, timeout, Escape) without stale UI or blocking subsequent
  terminal input (Phase 1 verify bullet).
- Finalized "which-key overlay behavior" spec (background authority, owner-
  approved, in `260722-feat-dashboard-hotkey-config-framework`): bottom-
  right lazyvim-style popup, appears after a configurable delay (default
  250ms), lists next keys grouped as `key → +group` (group) or
  `key → <label>` (leaf, label from `dashboardCommandLabel`); a group key
  drills into its leaves; `Esc` or a second leader press cancels/exits; an
  unbound key = brief flash then exit; no auto-timeout by default
  (configurable).
- R1 (no invented targets) still applies transitively: the overlay only
  renders what the registry already resolves — it must not fabricate group
  names, labels, or bindings the registry doesn't have.

## Out of Scope

- Defining new bindings, a new dispatch path, or wiring any R2-listed GAP
  command id — this overlay only reads the registry `hotkeys.ts` Phase 1
  already ships (ticket Non-Goals).
- Command bar UI/prefix grammar (layer 3,
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`) and hint-
  click/fast-jump (layer 4, `260722-feat-dashboard-hint-click-fast-jump`) —
  separate tickets (ticket Non-Goals, Decisions "Layer sequencing").
- Building a user-facing rebind/settings UI for hotkeys — out of scope for
  this ticket; the overlay only needs to read whatever
  `hotkeyFrameworkRef.current` already resolved at mount (defaults merged
  with persisted rebinds via `applyHotkeyUserConfig`), not support live
  in-session rebinding.
- Implementing an active leader-pending timeout mechanism — `hotkeys.ts`
  ships `leaderPendingExpired`/`DEFAULT_LEADER_TIMEOUT` as pure helpers but
  no `App.tsx` timer currently calls them (confirmed: no
  `leaderPendingExpired`/`DEFAULT_LEADER_TIMEOUT` usage in `App.tsx`, and the
  default is `enabled: false`). The overlay's own dismissal only needs to
  react to `idle` state; wiring an actual timeout is the framework's
  concern, not this presentation layer's, and is not required since the
  default is off.

## Codebase Findings

- `ws-dashboard/frontend/src/hotkeys.ts#L200-L233` — `LeaderTreeNode` /
  `buildLeaderTree`: a node has an optional `binding` (leaf) and a
  `children: ReadonlyMap<string, LeaderTreeNode>`. Per the module's own
  documented invariant, **children always win**: a node with any children
  must be treated as a group even if it also carries a `binding`. The
  overlay's entry-listing logic must replicate this same precedence (group
  if `children.size > 0`, else leaf) rather than re-deriving its own rule.
- `ws-dashboard/frontend/src/hotkeys.ts#L235-L310` — `LeaderState` union
  (`{kind:"idle"}` | `{kind:"pending", node, typed, enteredAtMs}`) and
  `stepLeaderState`. The current pending `node` (whatever depth) is exactly
  what the overlay must render entries from (`node.children`). `enteredAtMs`
  is reset on every narrowing step (not just the initial leader press), so
  it cannot be used to gate the "250ms initial appearance delay" from the
  finalized spec — that delay must be tracked as the overlay's own local
  timer keyed off the idle→pending transition, not off `enteredAtMs`.
- `ws-dashboard/frontend/src/hotkeys.ts#L88-L95` — `HotkeyBinding.description`
  (optional `string`) already exists on every entry `buildDefaultHotkeyBindings`
  produces (verified: all 14 leaves set it, e.g. `"Open root picker (browse
  filesystem to add a new root)"`). Confirmed via
  `grep -rn "\.description\b" App.tsx hotkeys.ts hotkeys.test.ts commands.ts`
  that this field has **zero existing consumers** — this ticket is its
  first reader. Use `binding.description` directly for the leaf label
  instead of `dashboardCommandLabel(resolveHotkeyCommand(binding, ctx))`:
  `resolveHotkeyCommand` calls `binding.buildPayload(ctx)`, which returns
  `null` whenever `ctx.activeRoot` is null (see `activeRootBinding` at
  `hotkeys.ts#L619-L634`), so a command-based label would be undefined for
  every current default binding while no work root is selected. The
  finalized spec's "labels from `dashboardCommandLabel`" phrasing is
  satisfied in spirit by `description`, since every default binding's
  `description` was written as that same human-readable action label.
- `ws-dashboard/frontend/src/App.tsx#L706-L736` — `hotkeyFrameworkRef`
  (lazy `useRef` init) holds `{registry, leaderTree, standaloneBindings}`,
  built once from `buildDefaultHotkeyBindings()` merged with
  `loadHotkeyUserConfig()` via `applyHotkeyUserConfig`. This `leaderTree`
  already reflects persisted user rebindings at mount time — the overlay
  does not need its own registry read or persistence call; it just needs
  this same object threaded down as a prop.
- `ws-dashboard/frontend/src/App.tsx#L744-L746` — `leaderStateRef` is a
  plain `useRef<LeaderState<HotkeyDispatchContext>>`, not React state.
  Comment at `App.tsx#L737-L743` explains this is deliberate ("no re-render
  should happen mid-keystroke") for the dispatch logic's own synchronous
  read/write needs. **Risk signal**: a component cannot reactively render
  off a bare ref. The overlay needs a parallel `useState` mirror updated at
  the same two mutation points inside the keydown handler
  (`App.tsx#L1672` `leaderStateRef.current = transition.state` and
  `App.tsx#L1705-L1708` `leaderStateRef.current = enterLeaderPending(...)`)
  so the ref stays the source of truth for the synchronous dispatch path
  while the new state value drives the overlay's re-render. This is a
  small, well-contained addition (add `setLeaderUiState(...)` alongside
  each `leaderStateRef.current = ...` assignment), not a restructuring of
  the existing dispatch logic.
- `ws-dashboard/frontend/src/App.tsx#L1647-L1740` — the global capture-phase
  `document` `keydown` listener (`useEffect`, deps `[executeCommand]`) is
  the sole place `leaderStateRef` transitions happen; this is the only spot
  that needs the `setLeaderUiState` calls added. No second listener should
  be introduced (ticket Decisions: "does not introduce a second,
  independent mode of its own").
- `ws-dashboard/frontend/src/App.tsx#L1664-L1683` — while pending, Escape
  and an unmatched key both already resolve through `stepLeaderState` to
  `{kind:"idle"}` (Escape is handled inside `stepLeaderState` itself at
  `hotkeys.ts#L277-L279`); pressing `Ctrl+Space` again while pending also
  naturally cancels (space has no bound top-level child in the current
  default table, so `stepLeaderState` falls through to the "unmatched key"
  branch and returns `idle`). No extra cancel-path code is needed in
  `App.tsx` beyond mirroring the ref into the new state — every dismissal
  path (match/resolve, unmatched-key cancel, Escape cancel, "second leader
  press" cancel) already converges on `leaderStateRef.current.kind ===
  "idle"`, which the overlay can treat as its single hide condition.
- `ws-dashboard/frontend/src/App.tsx#L1846-L1928` — the root JSX return
  (`<main className="app-shell">`) already hosts several conditionally-
  rendered modal/popover components as direct children driven by state
  props (e.g. `<GitWorktreeAddModal target={gitWorktreeTarget} .../>`,
  `<LinkedServerModal state={serverModal} .../>`) — this is the existing
  pattern to follow for mounting a new `<WhichKeyOverlay .../>` sibling
  fed by the new `leaderUiState` + `hotkeyFrameworkRef.current`, rather
  than using a portal (no `createPortal` usage exists anywhere in
  `App.tsx` today — confirmed via grep).
- `ws-dashboard/frontend/src/styles.css#L2088-L2099` — `.workbench-close-
  popover` is the closest existing fixed-position popover pattern
  (`position: fixed; z-index: 1000;` plus `var(--ws-color-panel-raised)`,
  `var(--ws-color-border-strong)`, `var(--ws-space-*)` semantic tokens,
  `box-shadow`). Reuse this token vocabulary for the new overlay's chrome
  instead of hardcoding colors (mental model Domain Rule: "Dashboard
  frontend components use the local semantic token layer in `styles.css`").
  No existing `kbd`/key-chip CSS class exists anywhere in `styles.css` or
  `App.tsx` (confirmed via grep) — the per-key badge visual is net-new, not
  a reuse gap.
- `ws-dashboard/frontend/src/commands.ts#L670` — `dashboardCommandLabel`
  exists but takes a resolved `DashboardCommand` (commandId + payload), not
  a `HotkeyBinding`; per the finding above, prefer `binding.description`
  for leaf labels to avoid the null-payload gap for context-dependent
  bindings.
- `ai-docs/mental-model/ws-web-dashboard.md` Domain Rules (line ~16) — "Any
  ws-dashboard frontend implementation that changes visible browser UI must
  include browser-level visual/interaction verification, preferably
  Playwright... pure TypeScript tests, Vite build, and curl/API dogfood are
  not sufficient to close UI-facing work." This ticket adds new visible
  chrome (the overlay itself), so this rule is squarely in scope (unlike
  the framework ticket's own plan, which flagged this rule as an open
  judgment call precisely because Phase 1 there added no visible UI).
  Confirmed via `grep -rln "hotkey|Ctrl+Space|leader" e2e/` that no
  Playwright coverage of the leader system exists yet — this ticket's
  Playwright case would be the first.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` — a single large
  `test.describe.configure({mode:"serial"})` suite with one long `test(...)`
  driving the whole UI; existing `page.keyboard.press(...)` calls (e.g.
  `.press("Escape")` at line 1173, `.press(" ")` at line 615) are the
  pattern to extend with `Control+Space` + a follow-up key, then assert the
  overlay's DOM appears/disappears.
- `ws-dashboard/frontend/package.json` — `test:hotkeys` runs
  `tsc -p tsconfig.route-tests.json && node ./node_modules/.tmp/route-tests/hotkeys.test.js`
  against `hotkeys.test.ts`, a plain Node script using a hand-rolled
  `assertEqual`/`assertDeepEqual` (no test framework) — the precedent for
  any new pure-logic unit (e.g. a `computeWhichKeyEntries(node)` helper) if
  one is factored out into `hotkeys.ts` or a sibling module.

## Implementation Plan

1. In `hotkeys.ts`, add a small pure helper (e.g.
   `describeLeaderChildren(node: LeaderTreeNode<TContext>): readonly
   {key, kind: "group" | "leaf", label?: string}[]`) that maps
   `node.children` entries to overlay-ready rows using the same
   children-win precedence `stepLeaderState` already enforces (group if
   `child.children.size > 0`, else leaf using `child.binding?.description`).
   Keep this DOM-free and colocated with the rest of the leader-mode
   section so it stays unit-testable via `test:hotkeys` the same way
   `stepLeaderState`/`buildLeaderTree` are.
2. In `App.tsx`, add `const [leaderUiState, setLeaderUiState] =
   useState<LeaderState<HotkeyDispatchContext>>({kind: "idle"})` next to
   the existing `leaderStateRef` declaration (`App.tsx#L744-L746`). Inside
   the keydown handler (`App.tsx#L1647-L1740`), call `setLeaderUiState(...)`
   with the same value immediately after each `leaderStateRef.current =
   ...` assignment (the `enterLeaderPending` case and the
   `stepLeaderState` transition case), so the ref stays the synchronous
   dispatch source of truth and the new state drives rendering.
3. Add a new presentational component (new file, e.g.
   `WhichKeyOverlay.tsx`, or colocated in `App.tsx` near the other
   modal/popover components) taking `leaderState: LeaderState<HotkeyDispatchContext>`
   as a prop. Render `null` when `leaderState.kind !== "pending"`. When
   pending, use `describeLeaderChildren(leaderState.node)` (step 1) to
   render the bottom-right fixed-position popup: group rows as
   `key → +group`, leaf rows as `key → <description>`, styled with the
   `.workbench-close-popover`-style token vocabulary
   (`var(--ws-color-panel-raised)`, `var(--ws-color-border-strong)`,
   `var(--ws-space-*)`) rather than new hardcoded colors. Gate initial
   visibility behind a local ~250ms timer that starts only on the
   idle→pending transition (track "was idle last render" via a ref inside
   this component, not off `enteredAtMs`, since that resets on every
   narrowing step per the Codebase Findings above) so subsequent narrowing
   keystrokes update the already-visible popup without re-delaying.
4. Mount `<WhichKeyOverlay leaderState={leaderUiState} />` as a direct
   child of the root `<main className="app-shell">` return
   (`App.tsx#L1846` onward), alongside the existing modal/popover siblings,
   so it renders above the whole shell without a portal.
5. Add the new module/test pair to `tsconfig.route-tests.json` `include`
   and, if step 1's helper gets its own assertions, extend
   `hotkeys.test.ts` (or add a focused new pure-logic test file wired the
   same way) covering: group-vs-leaf precedence at a node with both a
   `binding` and non-empty `children` (children must win), leaf label
   sourced from `description`, and empty-children-map behavior at a true
   leaf (defensive; should not be reachable via `describeLeaderChildren`
   since it's only ever called on a `pending` node's children map, but
   still worth covering the empty-map case explicitly).
6. Add or extend a Playwright case in
   `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` (or a small new
   spec file) that: presses `Control+Space`, asserts the overlay becomes
   visible (after the delay) listing at least one known default binding
   (e.g. the `t` group or `t n` terminal.create leaf, depending on which
   depth is asserted), types a further key to narrow/resolve, and asserts
   the overlay disappears on resolution; a second case presses
   `Control+Space` then `Escape` and asserts the overlay disappears without
   leaving stale DOM. This satisfies the mental-model's browser-level
   UI-verification rule for this ticket's new visible chrome.

## Verification Plan

- `cd ws-dashboard/frontend && npm run test:hotkeys` — pure-logic gate for
  any `describeLeaderChildren`-style helper added to `hotkeys.ts`.
- `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite build`) —
  whole-frontend type-check and production build gate.
- `cd ws-dashboard/frontend && npm run test:browser` (or the specific new/
  extended Playwright case if run standalone) — required per the
  `ws-web-dashboard` mental model's browser-level UI-verification rule
  since this ticket adds new visible chrome; must cover overlay appearance
  on leader press, narrowing on partial sequence, and clean disappearance
  on both resolve and Escape-cancel dismissal paths.
- Manual dev-server spot check: confirm the overlay does not appear/steal
  focus while a terminal pane has focus and `Ctrl+Space` is pressed there
  (passthrough guard already handled by the existing framework listener;
  the overlay must not add a second capture path that could regress this).

## Escalations

- None.

---
title: Dashboard which-key-style leader hint overlay
parent: 260711-epic-ws-dashboard-command-surface
related:
  260722-feat-dashboard-hotkey-config-framework: provides the binding
    registry this overlay reads to render available leader-sub bindings
  260711-idea-dashboard-command-bus-quick-open-shortcuts: sibling command-bar
    layer; this overlay surfaces leader-sub bindings, the command bar
    surfaces prefix-triggered actions - both dispatch through the same
    command bus
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: completed
---

# feat: Dashboard which-key-style leader hint overlay

## Background

Finalized behavior: see 260722-feat-dashboard-hotkey-config-framework §
Default Keymap & Interaction Spec (which-key overlay behavior).

Agenda A UX review (owner, 2026-07-22) finalized a tmux-style, leader-only
(no modal) dashboard keyboard model with `Ctrl+Space` as the leader key; see
`260722-feat-dashboard-hotkey-config-framework` for the full decision record
this ticket builds on. That framework ticket is the foundation: it defines
the binding registry, the leader-press dispatch mechanism, and the terminal
passthrough guard. This ticket is layer 2 of the intended stack — a
which-key/lazyvim-style hint overlay that appears on leader press and shows
the user what follow-up keys are available.

This is a discoverability layer, not a new dispatch mechanism: it reads the
hotkey config framework's binding registry and renders it, it does not
define new bindings or a new command path.

## Decisions (final - Agenda A, 2026-07-22)

- Depends on `260722-feat-dashboard-hotkey-config-framework`; must not ship
  before or independently of that framework, since it has nothing to render
  without the binding registry.
- Purely a presentation layer over the existing registry: on leader
  (`Ctrl+Space`) press, show a transient overlay (which-key/lazyvim style)
  listing the currently reachable leader-sub bindings and their bound
  actions/labels.
- Must respect the same no-modal, tmux-style transient-mode semantics as the
  framework: the overlay is visible only during the transient
  dashboard-command-mode window and dismisses when that window resolves,
  times out, or is cancelled (e.g. Escape) - it does not introduce a second,
  independent mode of its own.
- Must not interfere with the terminal passthrough guard already established
  by the framework ticket; the overlay is purely additive UI, not a new
  input-capture path.
- Layer sequencing: this sits between the framework (layer 1) and the
  command bar integration (layer 3, already specced in
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`) and before
  hint-click/fast-jump (layer 4,
  `260722-feat-dashboard-hint-click-fast-jump`). Implement sequentially,
  after the framework ships, per the parent epic's ordering.

## Non-Goals

- Defining new bindings or a new dispatch path - all actions shown come from
  the framework's existing binding registry.
- Rewriting the command bar's prefix grammar or its own UI - the command bar
  remains its own surface, specced in
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`.
- The hint-click/fast-jump on-screen label system
  (`260722-feat-dashboard-hint-click-fast-jump`) - visually similar in spirit
  (on-screen hints) but a distinct, later feature with a different trigger
  and purpose (mouse-target jumping vs. leader-sub key discovery).

## Phases

### Phase 1: Leader-press hint overlay

- Render a transient overlay on `Ctrl+Space` leader press that lists
  currently reachable leader-sub bindings (key, bound command label) sourced
  from the hotkey config framework's binding registry.
- Overlay lifecycle follows the framework's transient dashboard-command-mode
  window: appears on leader press, updates or narrows as the user types a
  partial sequence (if the framework supports multi-key leader-sub
  sequences), and dismisses on resolution, timeout, or cancel.
- Ensure the overlay does not capture or consume terminal input itself; it
  is a read-only visualization of the framework's existing capture state.
- Verify: overlay appears within a normal input-latency budget on leader
  press, reflects live registry contents (including user-configured
  rebindings from the framework's persistence layer), and disappears
  cleanly on every dismissal path (match, timeout, Escape) without leaving
  stale UI or blocking subsequent terminal input.

### Result (7caaaeb6)

Overlay shipped: a pure `describeLeaderChildren` helper in `hotkeys.ts`
(mirrors `stepLeaderState`'s children-win group/leaf precedence), a parallel
`leaderUiState` React-state mirror of the existing `leaderStateRef` updated
at its two existing mutation sites in `App.tsx`'s keydown handler, a new
`WhichKeyOverlay.tsx` presentational component (250ms appearance-delay timer
anchored to the idle→pending transition, mounted as a sibling of
`shell-grid`), `.which-key-overlay*` styles in `styles.css` reusing the
`.workbench-close-popover` semantic-token vocabulary, and new
`describeLeaderChildren` assertions in `hotkeys.test.ts`. Impl commit
`7caaaeb6`.

Deviation from the plan's literal wording (documented, reasoned): leaf row
labels are sourced from the resolved `HotkeyBinding.description` field, not
from `dashboardCommandLabel(resolveHotkeyCommand(...))`. Every
context-dependent default binding's `buildPayload` returns `null` when no
work root is selected, making a command-derived label undefined in that
common state; every default leaf's `description` is already authored as the
same human-readable action label, so reading it directly satisfies the
spec's intent without the null-payload gap.

Verification run: build and `test:hotkeys` (including the new
`describeLeaderChildren` assertions) passed. Browser-level (Playwright)
verification of the overlay's appear/narrow/dismiss lifecycle across all
four dismissal paths was explicitly not run this cycle (see commit
`7caaaeb6`'s AI Context and Ticket Updates) and is not yet covered.

3-partition review outcome: correctness clean, fit clean, test 1 Important
+ 3 Minor. The Important finding is not a code defect — it is the same
disclosed gap above: per the ws-web-dashboard mental model's mandatory
UI-verification rule, Phase 1's code is complete but the phase is not fully
closeable until browser-level verification lands. See Phase 2.

### Phase 2: Browser-level (Playwright) verification

- Goal: cover the overlay's appear/narrow/dismiss lifecycle (250ms
  appearance delay, row narrowing on partial sequences, and all four
  dismissal paths — match, unmatched-key cancel, Escape, second
  `Ctrl+Space`) with Playwright acceptance coverage, closing the gap left
  open by Phase 1.
- Prior blocker (resolved 2026-07-24): this phase was blocked on
  `260722-bug-e2e-open-work-root-locator-ambiguity` — the acceptance suite's
  `openWorkRootInBrowser` locator (`[data-command-id="rootPicker.open"]`) was
  ambiguous against the `.open-work-root-empty-cta` empty-state CTA, red-lining
  the whole acceptance suite at step 1. That bug is now fixed and its ticket is
  in `.done/`, so the block is lifted and Phase 2 can proceed.
- Harness entry point: extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`,
  reusing the (now-fixed) `openWorkRootInBrowser` helper to reach a live
  dashboard, then drive `Ctrl+Space` and assert against the shipped
  `.which-key-overlay*` DOM (Phase 1 Result). Run via the `test:browser` npm
  script (builds frontend + `ws-dashboard-daemon`, then `playwright test`;
  Playwright 1.60.0 + chromium are available).
- Done when (acceptance checklist — one Playwright assertion per line):
  - Overlay does NOT appear before the 250ms appearance delay elapses after a
    leader press, and DOES appear after it.
  - Overlay rows reflect live registry contents and NARROW as a partial
    leader-sub sequence is typed.
  - Dismissal path 1 — a matched full sequence resolves and the overlay
    disappears with no stale UI.
  - Dismissal path 2 — an unmatched key cancels (flash-exit) and the overlay
    disappears.
  - Dismissal path 3 — `Escape` dismisses the overlay.
  - Dismissal path 4 — a second `Ctrl+Space` dismisses the overlay.
  - After every dismissal path, subsequent terminal input is not blocked.

### Result

Added one new Playwright `test.step` ("which-key overlay: appearance delay,
narrowing, and all four dismissal paths") to
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`, inserted right after
the existing "create terminal and run a command" step (so a live terminal and
an open, selected workRoot already exist), plus a small `expectTerminalNotBlocked`
helper reused after every dismissal path. One assertion group per Phase 2
"Done when" checklist line: appearance-delay boundary (absent at ~120ms,
visible once past 250ms via a polling `expect`, not a fixed hard-sleep past
the boundary), row narrowing (`describeLeaderChildren` output goes from the
4 root groups to 3 leaves on `<leader> r`), and all four dismissal paths
(match via `<leader> t n`, unmatched-key cancel, `Escape`, second
`Ctrl+Space`) — each followed by a real terminal echo round-trip proving the
terminal passthrough guard is not left stuck.

Deviation from the plan's literal wording (documented, reasoned): dismissal
path 1's "resolves" evidence uses the existing
`.workbench-toolbar[data-last-command-id]` command-dispatch hook (the same
idiom the earlier "activation controls are command-routed" step already
relies on) instead of asserting a new terminal pane appears. Investigation
during this phase found that the global leader-key listener's `executeCommand`
call has no registered handler for `terminal.create` (or most other default
leaf commandIds) — the real side effect is wired only at each control's own
local click handler, not the shared bus the hotkey listener dispatches
through — so `<leader> t n` reaches the command bus (observable via the
dispatch hook) but does not actually open a terminal. This is a genuine,
separate hotkey-config-framework dispatch defect, not a which-key-overlay
defect; per the phase's own instruction not to weaken assertions to hide a
found bug, the test asserts the true, in-scope thing (the overlay resolved
and dispatched) rather than the false thing (a new terminal exists) or a
silently-narrowed claim. Filed as
`260724-idea-dashboard-hotkey-leader-dispatch-gap` for separate triage.

Verification run: `npm run test:browser` (build + `cargo build -p
ws-dashboard-daemon` + `playwright test`) executed for real. The new
which-key step passed cleanly (confirmed via two full single-worker
`-g "dashboard workRoot UI browser acceptance"` runs with zero failures
inside or attributable to the step, and via the full `test:browser` run,
where it is not among the reported step failures). The overall run still
fails downstream at the pre-existing, already-tracked, deliberately-deferred
`260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` defect (the
"open new agent tab and launch a stub harness session" step) — confirmed via
`git stash` to reproduce byte-identically on the pre-this-phase baseline, so
it is neither caused by nor fixable within this phase's scope. No product
source (`WhichKeyOverlay.tsx`, `hotkeys.ts`, `App.tsx`) was changed.

#### Edition (1cff11f8) - 2026-07-24

Partitioned review (correctness + test) of the initial commit found two
Important defects and one Minor one, fixed on the same branch (test-only,
no product source touched):

- **Dismissal path 1's core assertion was vacuous.** `data-last-command-id`
  was already `"terminal.create"` from the real terminal-create button click
  earlier in the same test, so asserting it equalled `"terminal.create"`
  after `<leader> t n` passed regardless of whether the leader sequence
  dispatched anything - it did not distinguish a genuine resolve from an
  unmatched cancel. Fixed by first clicking the already-selected workRoot's
  own nav row (a real, idempotent, harmless dispatch) to set a known
  baseline of `"resource.select"`, then asserting the value transitions to
  `"terminal.create"` only after the leader sequence. Sanity-checked live:
  temporarily swapping the resolving keypress for an unmatched key made the
  fixed assertion fail as expected (observed `data-last-command-id` staying
  at `"resource.select"`), confirming the fix is no longer vacuous, before
  reverting the sanity mutation.
- **`expectTerminalNotBlocked`'s bare `.terminal-surface`/`.xterm-rows`
  locators would break under Playwright strict mode once
  `260724-idea-dashboard-hotkey-leader-dispatch-gap` is fixed** and
  `<leader> t n` starts really creating a second terminal pane. Scoped both
  to `.first()` (with a comment citing that ticket) so the helper stays
  valid once that gap is fixed.
- **Minor:** the appearance-delay "absent before" check raced a fixed
  120ms sleep against the 250ms boundary (modest CI-jitter risk). Changed to
  assert absence immediately after the leader press (~0ms elapsed, well
  inside the delay with no timing race) instead, leaving the "present after"
  side as a generous polling `expect`.

Re-verification: `npm run test:browser` executed for real again after the
fix. The which-key step still passes cleanly; the same pre-existing,
already-tracked `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden`
defect downstream is still the only failure, unchanged from before this
edition.

## Spec Impact

- No new spec surface. This ticket is a presentation-only discoverability layer
  over the hotkey config framework's existing binding registry; the underlying
  keyboard-interaction contract (leader key, transient dashboard-command-mode,
  terminal passthrough guard, 250ms which-key appearance delay, the four
  dismissal paths) is owned and specified by
  `260722-feat-dashboard-hotkey-config-framework` (§ Default Keymap &
  Interaction Spec). Phase 1 shipped against that spec; Phase 2 adds
  Playwright verification of the already-specified behavior and introduces no
  new caller-visible contract.
- Closeout only: no `spec:` addition or `spec-remove:` is required — the
  framework spec already covers the overlay's behavior. If browser verification
  surfaces a behavior gap versus that spec, capture it as a follow-up against
  the framework spec, not here.

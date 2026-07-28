# Plan: 260727-chore-merge-ws-dashboard-dev-into-goal-branch — Phase 2: resolve the six conflicts and land the merge commit

## Relevant Ticket Contract

- One merge commit: `git merge origin/ws-dashboard-dev` on the goal branch
  (never rebase, never reverse direction — Decisions). Code changes limited to
  keeping both sides' work plus the minimum to make the union compile and
  express it.
- Six conflicted files, each with a pinned resolution (Phase 2 body):
  `terminal.rs`, `settingsSections.tsx` (3 hunks), `server.rs`, `styles.css`,
  `_index.md`, `260725-research-ws-dashboard-pty-agent-pivot.md`.
- **Mandatory companion edit A** (Rust, same commit): bump
  `sessions_write_lock_sites_are_enumerated`'s expected count 4 → 5 and extend
  its enumerating comment with a `drain_all` line stating it discharges
  **neither** obligation yet.
- **Mandatory companion edit B** (frontend, same commit):
  `settingsSections.test.ts`'s `SETTINGS_SECTIONS.length` assertion 2 → 3, plus
  `advanced` descriptor assertions (id, title, `Component === AdvancedSection`,
  arity) mirroring the existing `notifications` block. Preserve the four
  `notificationAvailability` assertions verbatim (Constraints).
- A **second, separate commit** (still Phase 2): one Playwright step in
  `dashboard-acceptance.spec.ts` asserting the Settings nav lists an entry
  `hasText: "Advanced"` and that activating it mounts build-info content.
- Registry order is fixed by Decisions: `terminal`, `notifications`,
  `advanced` (index-based test assertions and `settingsModal.tsx`'s
  `sections[0]`/`sections.map` depend on this order).
- Decisions names the two stale `## Ticket Focus` bullets to drop, by stem:
  `260725-bug-dashboard-terminal-platform-macos-unsupported` and
  `260725-feat-dashboard-nav-row-two-line-open-state` (both already `.done/`
  on this branch; the dev side's bullets are stale fact, not lost work).
- Verification boundary (Phase 2): `cargo test -p ws-dashboard-daemon
  --no-fail-fast` with the failure-site list identical to Phase 1's recorded
  baseline; `npm run build`; `npm run test:settings` green; the
  `agent-attention-notification.spec.ts` tests and the acceptance file green
  apart from known pre-existing sites; all of Phase 1's five tests still
  green. Constraints require re-measuring the failure-site baseline
  immediately before this run, not diffing against Phase 1's recorded list
  (machine load moves it).
- Constraint: judge the Rust suite by failure **site**, not exit code
  (`routes.rs:1066`/`:1383` are pre-existing reds independent of this merge).
- Constraint: the merged `_index.md` will fail inventory parity (107 rows,
  128 files) — that is Phase 4's fix, not a Phase 2 defect; do not repair it
  here.
- Constraint: between this commit and Phase 3, the branch knowingly
  contradicts its own spec (`drain_all` still drops both removal
  obligations) — accepted deliberately, but Phase 3 must follow without a
  stopping point in between.

## Out of Scope

- The `drain_all` fix itself (drop-lock-then-loop `forget_token` +
  `attention.forget`, CONTRACT comment, invariants 5/6) — Phase 3.
- Rewriting the `sessions_write_lock_sites_are_enumerated` enumerating
  comment's `drain_all` line from "discharges neither" to "discharges both"
  — Phase 3, once the fix lands.
- `_index.md` inventory parity (the 21 missing rows) — Phase 4.
- Routing the Advanced-section/control-endpoint spec debt to
  `260725-feat-dashboard-graceful-shutdown-from-settings` — Phase 5.
- Any behavior change beyond what makes the union compile and its own tests
  express the union (per Decisions' reviewability argument for splitting
  `drain_all` out).

## Codebase Findings

**Conflict set re-verified against current tips** (task fact, re-confirmed
here): `git merge-tree --write-tree HEAD origin/ws-dashboard-dev` at `HEAD =
ca28eb12`, `origin/ws-dashboard-dev = cffca84c` produced tree `02b83229`, with
CONFLICT markers in exactly these six paths and no others:
`ai-docs/_index.md`, `ai-docs/tickets/idea/260725-research-ws-dashboard-pty-agent-pivot.md`,
`ws-dashboard/crates/daemon/src/server.rs`, `ws-dashboard/crates/daemon/src/terminal.rs`,
`ws-dashboard/frontend/src/settingsSections.tsx`, `ws-dashboard/frontend/src/styles.css`.
Everything else, including `router.rs`, auto-merged. **Phase 2's re-verification
requirement is discharged by this survey** — the implementer does not need to
repeat `git merge-tree` before resolving, only confirm the working tree still
matches these tips at merge time.

- `ai-docs/_index.md` — **`## Dashboard Test Hygiene` sits at line 31, entirely
  outside the conflict.** The conflict region in the merge-tree preview tree
  spans lines 287–480 (`## Ticket Focus`); lines 1–286 (which contain `##
  Dashboard Test Hygiene` at line 31) are byte-identical between the
  conflicted merge-tree blob and the current HEAD `_index.md` (`diff` verified
  empty). This section postdates the ticket's resolution rule (added in
  `ca28eb12`, after the ticket was written) and the ticket's `_index.md`
  Phase 2 rule only discusses the tickets table and `## Ticket Focus` — it is
  silent on this section because it did not exist yet. **No special handling
  is needed**: because the section sits outside every conflict hunk, a normal
  conflict-marker resolution (edit only inside `<<<<<<<`/`>>>>>>>` spans)
  cannot touch or drop it. The risk is only procedural — do not use `git
  checkout --theirs`/whole-file overwrite on this path, and do not paste in a
  reconstructed file body that omits it; resolve the markers in place.
- `ai-docs/_index.md#L287-L480` (conflicted region) — three-way split
  confirmed: HEAD's `<<<<<<<` block (287-409, our four closed-ticket
  writeups + the Ordering paragraph) / empty base (410) / `origin`'s
  `=======` block (411-479, both stale ready-bullets plus the two live dev
  entries `260725-feat-dashboard-terminal-steady-state-stream-throughput` and
  `260726-refactor-ws-dashboard-git-fs-watch-invalidation`). Ticket's
  prescription matches: keep ours entirely, keep the dev side's two live
  entries, drop the two stale ready-bullets
  (`260725-bug-dashboard-terminal-platform-macos-unsupported` at dev-side
  lines 411-422, `260725-feat-dashboard-nav-row-two-line-open-state` at
  423-431).
- `ws-dashboard/crates/daemon/src/terminal.rs#L1243-L1548` — confirmed
  structure: HEAD block (1244-1528, five items: `TerminalTurnStateRequest`,
  `post_terminal_turn_state`, `HelperEnvPlan`, `build_helper_command`,
  `resolve_create_command`) then empty base then dev block (1532-1546,
  `close_all_terminals` only, calling `state.terminals.drain_all()`). Matches
  ticket exactly; insertion point is between `close_terminal` and `impl
  TerminalSession` (line 1549).
- `ws-dashboard/crates/daemon/src/server.rs#L144-L224` — confirmed: HEAD's
  `gc_sweep_task` spawn block (145-204) then empty base then dev's
  `shutdown_notify`/`epoch_source`/`git_spawn_stats`/`watch_registry`
  construction (207-223), both immediately after the shared
  `boot_reconcile().await;` (line 143) and before the untouched
  `build_router(AppState {...})` at line 225. Matches ticket's "ours first"
  ordering.
- `ws-dashboard/crates/daemon/src/router.rs` (auto-merged, not conflicted) —
  read the full `AppState` struct (lines 84-135): it carries both sides'
  fields — `attention` (ours, line 123) and `git_probe_cache`,
  `git_spawn_stats`, `git_state_cache`, `epoch_source`, `watch_registry`,
  `shutdown` (theirs, lines 91-114 and 133) — confirming the ticket's claim
  that dropping either `server.rs` block would fail to compile rather than
  silently lose behavior.
- `ws-dashboard/frontend/src/settingsSections.tsx` — three hunks confirmed:
  (1) L1-7, import line — theirs (`createContext, useContext, useEffect,
  useState`) is a strict superset of ours, take theirs verbatim; (2) L95-405,
  section bodies — ours (`SettingsNotificationContext`, `notificationAvailability`,
  `NotificationSection`, L96-234) unions with theirs (`formatBuildTime`,
  `ConfirmButton`, `AdvancedSection`, L237-403), no shared identifiers; (3)
  L415-424, `SETTINGS_SECTIONS` array — union both descriptors after
  `terminal`, ordering `notifications` (ours) then `advanced` (theirs) per
  Decisions.
- `ws-dashboard/frontend/src/styles.css#L3939-L4064` — confirmed: HEAD's
  `.settings-notification-toggle`/`.settings-field-note` rules (3940-3950)
  then empty base then dev's Advanced-section rule block (3954-4063,
  `.settings-advanced*`, `.settings-buildinfo*`, `.settings-danger-*`,
  `.settings-ghost-button`). Straight union confirmed disjoint (no shared
  selector names observed in either block).
- `ai-docs/tickets/idea/260725-research-ws-dashboard-pty-agent-pivot.md#L560-L588`
  — confirmed: HEAD's RESOLVED rewrite (561-569) vs base's stale unresolved
  duplicate (571-572, identical wording to part of dev's block) vs dev's block
  (574-587, new Activity-Console-retirement question at 574-585 PLUS the same
  stale unresolved line repeated at 586-587). Resolution: keep HEAD's RESOLVED
  block, keep dev's Activity-Console-retirement question, drop the duplicate
  unresolved line (appears in both the base and the tail of dev's block).
- `ws-dashboard/crates/daemon/src/terminal.rs#L2411-2425` — the exact test
  companion edit A targets:
  `sessions_write_lock_sites_are_enumerated`, `assert_eq!(count, 4, ...)` at
  line 2417. Change to `5`; the enumerating CONTRACT comment directly above it
  (lines 2396-2410) already anticipates this ("Phase 2 adds `drain_all`,
  moving this count to 5 (with its own "discharges neither" line)") — extend
  it with the actual `drain_all` line rather than only bumping the number.
- `ws-dashboard/frontend/src/settingsSections.test.ts#L1-118` — companion edit
  B targets: line ~3 import list (add `AdvancedSection`), line ~30-32
  (`SETTINGS_SECTIONS.length` assertion, currently asserts `2` with message
  "Phase 8 grows the registry to Terminal + Notifications" — bump to `3` and
  update the message), and the `notifications` descriptor block at
  `SETTINGS_SECTIONS[1]` (lines ~90-118: id, title, `Component ===` identity,
  arity, context-object-type assertions) as the exact shape to mirror for
  `SETTINGS_SECTIONS[2]` (`advanced`). The four `notificationAvailability`
  assertions live further down this file (unaffected, must stay verbatim).
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1055-1097` — the
  existing "insecure context disables the Notifications toggle" `test.step`
  is the sibling to model the new Advanced-panel step on: same
  `dialog.locator(".settings-section-nav-button", { hasText: "..." })` pattern
  for nav selection. Build-info content anchor: `AdvancedSection` renders a
  `.settings-buildinfo` `<dl>` with rows labeled "Version", "Daemon binary",
  "Frontend bundle" (`settingsSections.tsx#L347-361` in the merged view) — the
  natural assertion is that this element becomes visible, or that a `dt`
  reads "Version", after clicking the Advanced nav entry. Add the new step
  after line 1097 (end of the existing Notifications step) and before the
  "add-server modal" step at line 1101, keeping the Settings-panel steps
  grouped.
- `ws-dashboard/frontend/package.json` — confirms the referenced scripts
  exist: `"build": "tsc -b && vite build"` (line 8), `"test:settings"` (line
  33, chains `tsconfig.route-tests.json` build then three route-test files
  including `settingsSections.test.js`).

## Implementation Plan

1. Confirm the working tree is still at `HEAD = ca28eb12` /
   `origin/ws-dashboard-dev = cffca84c` (or re-run `git merge-tree
   --write-tree` if either moved) before starting — this survey's
   re-verification is only valid at these tips.
2. Run `git merge origin/ws-dashboard-dev --no-commit` (or `--no-ff
   --no-commit`) on the current branch to materialize the six conflicts.
3. Resolve `ws-dashboard/crates/daemon/src/terminal.rs`: keep both blocks
   (HEAD's five items, then dev's `close_all_terminals`), matching the
   confirmed structure above.
4. Resolve `ws-dashboard/crates/daemon/src/server.rs`: concatenate HEAD's
   `gc_sweep_task` block first, then dev's `shutdown_notify`/`epoch_source`/
   `git_spawn_stats`/`watch_registry` block, both after `boot_reconcile()`
   and before the untouched `build_router(AppState {...})`.
5. Resolve `ws-dashboard/frontend/src/settingsSections.tsx`: take dev's
   import line verbatim (hunk 1); union both section bodies, ours first
   (hunk 2); build `SETTINGS_SECTIONS` as `terminal`, `notifications`,
   `advanced` (hunk 3).
6. Resolve `ws-dashboard/frontend/src/styles.css`: straight union, HEAD's
   rules then dev's Advanced-section rules, after the shared
   `.settings-field-label` rule.
7. Resolve `ai-docs/_index.md`: leave `## Dashboard Test Hygiene` (line 31)
   and everything before line 287 untouched (it is outside the conflict
   hunk — do not overwrite the file wholesale from either side). In the
   conflicted `## Ticket Focus` region, keep HEAD's block verbatim, add
   dev's `260725-feat-dashboard-terminal-steady-state-stream-throughput` and
   `260726-refactor-ws-dashboard-git-fs-watch-invalidation` entries, and drop
   dev's `260725-bug-dashboard-terminal-platform-macos-unsupported` and
   `260725-feat-dashboard-nav-row-two-line-open-state` stale bullets. Leave
   the auto-merged tickets table (rows above the conflict) untouched.
8. Resolve `ai-docs/tickets/idea/260725-research-ws-dashboard-pty-agent-pivot.md`:
   keep HEAD's RESOLVED rewrite, keep dev's Activity-Console-retirement open
   question, drop the stale duplicate unresolved line.
9. **Mandatory companion edit A** in the same working tree (before
   committing): in `terminal.rs` at the
   `sessions_write_lock_sites_are_enumerated` test (post-merge line number
   will shift from 2412 once the merge inserts ~305 lines above it — locate
   by test name), change `assert_eq!(count, 4, ...)` to `5` and extend the
   enumerating CONTRACT comment above it with a `drain_all` line stating it
   takes the write lock but discharges **neither** the token nor attention
   obligation yet.
10. **Mandatory companion edit B** in the same working tree: in
    `settingsSections.test.ts`, import `AdvancedSection`, bump
    `SETTINGS_SECTIONS.length` assertion from `2` to `3` (update its message),
    and add an `advanced`-descriptor block at `SETTINGS_SECTIONS[2]` mirroring
    the existing `notifications` block at `SETTINGS_SECTIONS[1]`: `id ===
    "advanced"`, `title === "Advanced"`, `Component === AdvancedSection`
    (module-scope identity), and `AdvancedSection.length === 0`. Do not touch
    the four `notificationAvailability` assertions.
11. Stage all resolved files plus the two companion edits and create the
    merge commit (one commit, both parents, per Decisions — do not fold in
    the Playwright step).
12. In a **separate commit**, add the Advanced-panel Playwright step to
    `dashboard-acceptance.spec.ts` (after the existing Notifications
    `test.step`, before "add-server modal opens..."): open Settings, assert a
    nav entry with `hasText: "Advanced"`, click it, assert the build-info
    content (e.g. `.settings-buildinfo` or a `dt` reading "Version") becomes
    visible.
13. Re-measure the Rust integration-test failure-site baseline immediately
    before running verification (Constraints: do not diff against Phase 1's
    recorded list under different machine load).
14. Run the Verification Plan below and record results, including which
    scope of `npm run test:browser` was actually run, in the Phase 2 Result.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --no-fail-fast` (both `--lib` and
  integration targets) — compare failure sites (not exit code) against a
  freshly re-measured baseline, not Phase 1's recorded one. Confirm all five
  Phase 1 tests are green and `sessions_write_lock_sites_are_enumerated`
  passes at count 5.
- `npm run build` (in `ws-dashboard/frontend`) — must succeed; this is the
  companion-edit-B safety net (a dropped `AdvancedSection` import would fail
  here given no linter/unused-symbol checking exists).
- `npm run test:settings` — must be green with the registry length at 3 and
  all four `notificationAvailability` assertions intact.
- Run `e2e/agent-attention-notification.spec.ts` and the new/updated step in
  `dashboard-acceptance.spec.ts` (Playwright rebuilds the bundle via
  `globalSetup` unless `WS_DASHBOARD_STATIC_DIR`/external-daemon mode is set —
  confirm the build line on stdout). State explicitly in the Result whether
  the whole `npm run test:browser` suite ran or only these spec files.
- Manual/mutation checks per ticket: confirm the merge commit's diff against
  each parent shows only "kept both sides" content plus the two named
  companion edits — no unrelated behavior change.

## Escalations

- None.

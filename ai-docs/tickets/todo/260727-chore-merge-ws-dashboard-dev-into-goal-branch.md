---
title: Merge ws-dashboard-dev into the goal branch - six trivial conflicts, one clean-merge defect
related:
  260725-feat-dashboard-pty-agent-attention-notification: its Phase 4 and Phase 5 CONTRACTs are the two obligations the incoming drain_all skips
  260725-feat-dashboard-graceful-shutdown-from-settings: the dev-side ticket d1d6bb31 landed against; kill-all is its endpoint
  260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile: source of the "std exposes no clear-flag introspection" argument behind invariant 1's test shape
sage-review-design: recommended
---

# Merge ws-dashboard-dev into the goal branch - six trivial conflicts, one clean-merge defect

## Background

PR #4 (head `goal/ws-dashboard-dev/velvet-arbor-quill`, base `ws-dashboard-dev`)
is `mergeable: CONFLICTING`, `mergeStateStatus: DIRTY`. Merge base `af241aa2`;
the goal branch is 181 commits ahead of it, `origin/ws-dashboard-dev` 85 ahead.

**The whole point of this ticket, stated first.** All six conflicts are
mechanically trivial insertions at shared anchors, and that is precisely what
makes this merge dangerous. A person who opens the conflicts, sees six clean
unions, resolves them and gets a green build will conclude the merge was
routine - and will never look at the code that merged *without* a conflict. The
real defect is there. Conflict markers are not a risk index; on this merge they
are close to an anti-index, because the only file where the two branches
genuinely disagree about behavior is a file git resolved silently.

### What was measured

Against a non-destructive `git merge-tree` preview (ours `b6aaf8ba`, theirs
`origin/ws-dashboard-dev`, base `af241aa2`) producing tree
`9b6abe64e7bb6979f44586cc63397c5907216cf7`. Read it with
`git show "9b6abe64e7bb6979f44586cc63397c5907216cf7:<path>"`; in it `<<<<<<<` is
the goal side and `>>>>>>>` is the dev side. **Every `preview:N` below is
evidence about that specific tree OID and nothing else.** They are citations,
not edit coordinates - a resolution moves all of them. Find the code by the
search named alongside.

| File | Hunks | Conflicted lines | Verdict |
|---|---|---|---|
| `ws-dashboard/crates/daemon/src/terminal.rs` | 1 | 305 | mechanical add/add, empty base |
| `ws-dashboard/frontend/src/settingsSections.tsx` | 3 | 325 | two add/add with empty base; one shared import line |
| `ws-dashboard/crates/daemon/src/server.rs` | 1 | 80 | mechanical add/add, empty base |
| `ws-dashboard/frontend/src/styles.css` | 1 | 125 | disjoint selectors, empty base |
| `ai-docs/_index.md` | 1 | 195 | union plus 2 stale entries |
| `ai-docs/tickets/idea/260725-research-ws-dashboard-pty-agent-pivot.md` | 1 | 28 | union plus 1 competing entry |

Five of the eight hunks carry an empty base section (`||||||| af241aa2`
immediately followed by `=======`), so neither side touched a line the other
touched. The three exceptions are the React import in `settingsSections.tsx`
(preview:1-7, base line 4), `_index.md`'s `## Ticket Focus` (preview base
391-412) and the research ticket's open-questions block (preview base 570-572).
Only the first is code, and there theirs is a strict superset of ours
(`createContext, useContext, useEffect, useState` vs `createContext,
useContext, useState`).

Also measured: the styles.css selector-set intersection between the two sides is
EMPTY, by sorted-list comparison of every selector each side introduces, so the
"later duplicate rule silently wins" failure does not apply here. `_index.md`'s
tickets table auto-merged to 107 rows with every row pointing at a real file, so
no manual table edit is needed. And `git grep drain_all origin/ws-dashboard-dev`
returns exactly two hits - the definition and the route registration in
`router.rs` - which is the zero-tests finding below.

Assumed, not measured: that nothing outside the six conflicted files needs a
resolution decision. `router.rs` in particular auto-merged and its `AppState`
field list has not been read line by line; Phase 2 checks it.

### The defect the conflict markers do not point at

`d1d6bb31` ("settings Advanced panel - build info, shutdown, kill-all") added
`TerminalRegistry::drain_all` against the BASE registry. Since the fork, the
goal branch gave that registry two removal obligations, each discharged under an
explicit CONTRACT comment naming the exact failure it prevents. `drain_all`
merges cleanly and discharges neither. Search `fn drain_all` in `terminal.rs`,
then read the sibling `fn remove` a few dozen lines above it in the same `impl`.

| removal path | `forget_token` | `attention.forget` |
|---|---|---|
| `remove` (preview:667-680) | yes | yes |
| `remove_for_work_roots` (preview:687-716) | yes | yes |
| `insert` eviction retain (preview:582-611) | **no, by recorded deferral** | yes |
| `drain_all` (preview:723-729) | **no** | **no** |

Two consequences after a kill-all on the merged code:

1. AttentionHub entries survive the kill. A reconnect renders terminals that no
   longer exist - verbatim the failure `remove`'s CONTRACT comment was written
   to prevent ("a reconnect's snapshot would show a phantom terminal after
   close").
2. `forget_token` is the only caller of `agent_token_store::delete_token`.
   Skipping it strands `terminal-tokens/<id>.json` on disk permanently and
   leaves `self.tokens` entries that still satisfy `token_for`. Traced through
   `post_terminal_turn_state`: `token_for` returns the stale token,
   `tokens_match` succeeds, the `state.terminals.get(&terminal_id)` lookup
   returns `None` and is treated as a benign race, and the handler falls through
   to `StatusCode::NO_CONTENT`. A killed terminal's token therefore gets 204
   where it should get 401.

**Correction to the eviction row, and it changes what a test may assert.** The
fact-gathering pass recorded `forget_token` as "n/a" for `insert`'s eviction
path. It is not n/a. Read the CONTRACT comment directly above `evicted_ids` in
`insert`: it names the eviction as a session-removal path, fixes only the
attention half, and explicitly defers "the callback-token half of this same gap"
as inherited debt. So the true post-merge rule is *four* removal paths, of which
three must discharge both obligations and one - the eviction - discharges
attention only, by a decision already recorded in the source. An invariant test
written to the literal "EVERY path calls both" would fail today, before this
merge, on debt this ticket did not create and must not silently absorb.

### Why this needs tests rather than a careful hand

The two possible resolution mistakes are wildly asymmetric.

- **Dropping the goal side's Notification work fails loudly.** `App.tsx` imports
  and provides `SettingsNotificationContext`; `settingsSections.test.ts` imports
  `notificationAvailability` and `NotificationSection`. Both `tsc -b` and
  `tsc -p tsconfig.route-tests.json` break.
- **Dropping the dev side's Advanced panel is completely silent.** `d1d6bb31`
  added no frontend test. No `noUnusedLocals` or `noUnusedParameters` appears in
  any `tsconfig*.json` under `ws-dashboard/frontend` (grep `noUnused` - zero
  hits), and no eslint, biome or oxlint config exists anywhere in that tree. A
  resolution that drops `AdvancedSection` produces a perfectly green build,
  noticed only by a human opening Settings and finding no Advanced tab.

**Second correction.** `d1d6bb31` was recorded as adding "+5 tests in
`crates/daemon/tests/routes.rs`". It added zero tests, there or anywhere:
`git show d1d6bb31 -- ws-dashboard/crates/daemon/tests/routes.rs` is five
`shutdown: Arc::new(tokio::sync::Notify::new()),` lines inserted into existing
`AppState` literals. So `build-info`, `shutdown` and `kill-all` have no HTTP
coverage either, not merely no UI coverage. Nothing anywhere exercises
`AdvancedSection` or `drain_all`.

**Third correction.** The dev side touched `terminal.rs` in exactly one commit,
`d1d6bb31`, contributing only the `close_all_terminals` handler (preview:1532-1547).
An earlier reading attributed the conflict to the git-watcher / git-exec-seam /
git-state-cache workstream that dominates the other 84 commits. It did not
touch this file.

### Pre-existing conditions - state them, do not fix them under cover of this merge

These are all true before the merge. Every one of them will look like merge
fallout to a later reader, and none of them is.

- `origin/ws-dashboard-dev` is currently RED on `npm run test:settings`: its
  registry holds 2 entries while its test asserts `length === 1, "Phase 1
  registers exactly the Terminal section"`. Verified on the branch, both sides.
- 21 ticket files under `ready/` + `todo/` + `idea/` have no row in the tickets
  table of `_index.md`. Measured against the preview tree: 128 ticket files, 107
  table rows, 21 unrowed, all dev-side-authored (`260725-epic-ws-dashboard-git-panel`,
  `260726-refactor-ws-dashboard-long-uptime-leak-hardening`,
  `260725-bug-dashboard-terminal-utf8-residual-multibyte-corruption` and 18
  more). The gap is already present in the dev side's own `_index.md`. The goal
  side had reconciled its inventory to 107/107, so the merge visibly breaks that
  parity without having caused it. Phase 4 fixes it in its own commit.
- Known pre-existing test failure sites on the goal side:
  `crates/daemon/tests/routes.rs:1066` and `:1383`, and
  `e2e/dashboard-acceptance.spec.ts:3827` plus a serial-mode cascade leaving
  `:4068` not run.

## Decisions

**Merge `origin/ws-dashboard-dev` into the goal branch and resolve there. Do not
rebase, do not reverse the direction.** PR #4's head is the goal branch, so the
only thing that turns `CONFLICTING` into mergeable without rewriting pushed
history is a merge commit on the head branch carrying the base tip as a parent.
Rebasing the goal branch onto dev would rewrite 181 already-pushed commits, and
that is not merely inconvenient here: the ticket convention pins each completed
phase to `### Result (<short-hash>)`, and this branch's `.done/` and `ready/`
tickets are dense with those hashes (`87259c93`, `2da1731d`, `f314ba41`, and
dozens more). A rebase silently orphans every one of them and there is no
mechanical repair. Rejected for the same reason: squash-merging the goal branch
into dev, and cherry-picking `d1d6bb31` onto the goal branch instead of merging
(which would leave the other 84 dev commits still conflicting later, and hide
the `server.rs` field-set conflict that is currently doing useful work).

**Registry order is `terminal`, `notifications`, `advanced`.** Order is
caller-visible twice in `settingsModal.tsx`: `useState(() => sections[0]?.id)`
makes index 0 the default active section, and `sections.map(...)` renders the
nav list in registry order. `terminal` is index 0 on both sides so the default
is unaffected either way, but `settingsSections.test.ts` indexes `[0]` and `[1]`
by position - putting `advanced` at index 1 breaks
`notificationDescriptor.id === "notifications"` for no gain. Only
`activeSection.Component` is mounted, so there is no cross-section selector
collision and `AdvancedSection`'s build-info `useEffect` stays lazy. The e2e
specs select by `hasText`, so they are order-independent and cast no vote.

**Do not restore the two stale `## Ticket Focus` entries from the dev side.**
`260725-bug-dashboard-terminal-platform-macos-unsupported` and
`260725-feat-dashboard-nav-row-two-line-open-state` appear on the dev side as
`(ready, ...)` bullets. Both files are already under `.done/` in the merged tree
- git resolved the moves cleanly - so those bullets are stale fact, not lost
work. The dev side never edited them; the goal side deleted them because both
tickets closed. Taking the union here would reintroduce a claim the same commit
disproves. (Note for the resolver: these are `## Ticket Focus` bullets, not
tickets-table rows. The table has no rows for either stem.)

**`drain_all`'s fix lands in a separate commit from the merge resolution.** The
merge commit must be reviewable as "did the resolver keep both sides' code, and
only that". Folding a behavior change into it destroys that property - a
reviewer diffing a merge commit against either parent cannot cheaply separate
"resolution choice" from "new logic", which is exactly how a wrong resolution
hides. Splitting also means `git log` attributes the fix to the ticket rather
than to a merge, and lets Phase 3 be reverted without unwinding the merge.

**Invariant tests land before the merge wherever the invariant already holds.**
A test written after a merge is a test whose meaning nobody can attribute: if it
fails you cannot tell whether the merge broke something or the test was wrong.
Four of the six invariants are already true on the goal branch and their tests
must be green on the pre-merge tree first (Phase 1). The other two describe
behavior that does not exist until `drain_all` arrives, so they are Phase 3.

**Advanced-panel coverage is one browser step, not a component-test harness.**
There is no React test renderer in this project and adding one to cover a panel
this merge merely *carries* is out of proportion. The registry unit assertion is
what makes a silent drop loud; the browser step is what proves the section
actually mounts.

## Constraints

- **`cargo test -p ws-dashboard-daemon` requires `--no-fail-fast`.** Without it
  the run aborts after the `routes` target fails and never reaches the later
  integration targets - including the ones Phase 3 adds. A run without the flag
  produces a red exit that says nothing about the code under change.
- **Judge the Rust suite by failure SITE, not by exit code.** `routes.rs:1066`
  and `:1383` fail on the goal branch today, so `cargo` exits 101 even when the
  tree is otherwise clean. Record the site list before the merge and compare
  site-to-site after; a new *site* is the only signal.
- **`origin/ws-dashboard-dev` is already RED on `npm run test:settings`,** so
  "green after the merge" is not a coherent expectation for that suite. Its own
  registry/test mismatch is inherited, and `settingsSections.test.ts`
  auto-merges to a blob byte-identical to ours (the dev side never touched it),
  asserting `length === 2`. After the registry union it is 3, so the suite fails
  until the companion edit lands. That failure is wanted: it is the loud half of
  the asymmetry.
- **When editing `settingsSections.test.ts`, the four `notificationAvailability`
  assertions must be preserved verbatim.** They pin the four states of the
  insecure-context fix from `260726-chore-dashboard-verify-notification-permission-tier-manually`
  Phase 2. A bulk rewrite that drops them while bumping the length is the one
  remaining path by which that shipped defect returns green.
- **The frontend has no linter and no unused-symbol checking of any kind.** This
  is not a style observation; it is the mechanism by which a dropped
  `AdvancedSection` compiles, bundles and passes. Do not reason about the
  frontend as if an unused import would be caught.
- **Playwright rebuilds the bundle on every run.** `playwright.config.ts`
  declares `globalSetup: "./e2e/globalSetup.ts"`, which runs `npm run build`
  unconditionally unless `WS_DASHBOARD_STATIC_DIR` or external-daemon mode is
  set. Confirm the build line on stdout rather than assuming either way.
- **The merged `_index.md` will fail an inventory parity check** (107 rows, 128
  ticket files). Do not treat that as a resolution error and do not repair it
  inside the merge commit. See Phase 4.

## Phases

Phase 1 runs against the pre-merge tree. Phases 2, 3 and 4 are strictly ordered
after it and after each other.

### Phase 1: pin the surviving invariants as tests, before the merge

Write the tests for the four invariants that already hold on the goal branch,
and get them green on the pre-merge tree. Their value is entirely in being
known-meaningful *before* the merge muddies attribution.

Where a test is a source scan rather than a behavioral assertion, say so at the
test and say why - a reader who mistakes a source scan for a behavioral proof
will over-trust it.

**Invariant 1 - exactly one production `env_clear()` in `terminal.rs`.** Search
`env_clear` in that file: the sole production call site is inside
`build_helper_command`'s `HelperEnvPlan::ClearAndSet` arm; every other hit is
either a comment or the `#[cfg(test)]` positive control inside
`helper_spawn_default_no_command_matches_existing_arg_shape`. Half of this
invariant is already discharged by the merge itself: that existing test asserts
`helper_env_plan(None, None, vec![]) == HelperEnvPlan::InheritHost`, so a
resolution that drops the goal side's block fails to *compile*. The half nothing
covers is "no SECOND site appears". Add `terminal_rs_has_exactly_one_production_env_clear`
to `terminal.rs`'s `mod tests`: `include_str!` the file, truncate at the `mod
tests` marker, drop lines whose trimmed form starts with `//`, count `.env_clear(`,
assert 1. The source-text form is forced, not preferred:
`260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile` established that
`std::process::Command` exposes no public clear-flag introspection, so there is
no runtime observable for "some other call site cleared the env".
Mutation that fails it: add a second `.env_clear()` anywhere in production code
(count 2), or delete the sole one (count 0). Falsifiable in both directions,
which the comment-stripping must not break - verify by temporarily adding a
commented `.env_clear()` line and confirming the count does not move.

**Invariant 2 - the removal-path obligations.** Two tests, because one form
cannot carry both halves.

- Behavioral: the attention half is already covered on all three existing paths
  - search `forgets_the_attention_entry` in `terminal.rs`'s `mod tests` for the
  three tests and for `insert_fake_live_session_for_test`, the helper they
  share. The token half is covered nowhere. Add
  `remove_forgets_the_callback_token` and
  `remove_for_work_roots_forgets_the_callback_token`, each asserting both that
  `token_for` stops resolving and that the on-disk `terminal-tokens/<id>.json`
  is gone. Each needs a pre-removal assertion that `token_for` *does* resolve;
  without it a test whose fixture never stored a token passes for the wrong
  reason.
  Mutation: delete the `self.forget_token(...)` call from `remove` - the
  post-removal assertion in `remove_forgets_the_callback_token` fails; the
  attention test stays green, proving the two halves are independent.
- Structural: `sessions_removal_paths_are_enumerated`, a source scan over the
  production half of `terminal.rs` counting write-lock mutations of
  `self.sessions` that can drop an id (`.remove(`, `.retain(`, `.drain(`).
  Assert the count is 3 pre-merge and 4 post-merge (Phase 3 bumps it), with a
  comment naming each path and, for `insert`'s eviction, naming it as the one
  path that discharges attention only and citing the deferral recorded in its
  own CONTRACT comment. This is the only test in the ticket that would have
  caught the actual defect: `drain_all` broke no assertion because no assertion
  knew it existed.
  Mutation: add any new `sessions`-removing method - count 4 pre-merge, fail.
  The failure message must tell the author what to do, since the whole point is
  to force a discharge decision at the moment a fifth path is written.

**Invariant 3 - `token_for` is the sole reader of `self.tokens`,
`remember_token` the sole writer.** Source scan,
`tokens_map_access_is_confined_to_its_choke_points`: count `self.tokens`
occurrences in the production half of `terminal.rs` and assert exactly three,
one each inside `token_for`, `remember_token` and `forget_token` - the three
CONTRACT comments already name themselves as those choke points, so search
`self.tokens` to find them. Behavioral verification is not available: an extra
reader elsewhere would be indistinguishable from `token_for` at runtime, which
is why the map has a comment-enforced contract rather than a type-enforced one.
Mutation: add a `self.tokens.read()` anywhere else in the impl - count 4, fail.

**Invariant 4, the half that exists pre-merge - `notificationAvailability`'s four
states stay pinned.** Search `notificationAvailability` in
`settingsSections.test.ts`: four assertions, one per state. This phase adds
nothing; it records that these four are load-bearing so the Phase 2 companion
edit does not rewrite them away. Confirm they are green pre-merge so a later
failure is attributable.
Mutation: swap the secure-context check back to global-first in
`settingsSections.tsx` - the "insecure context, global present" assertion fails.
This mutation was already run once, in
`260726-chore-dashboard-verify-notification-permission-tier-manually` Phase 2;
re-run it only if the file changed since.

Verification boundary: `cargo test -p ws-dashboard-daemon --no-fail-fast` with
the failure-site list unchanged from the recorded baseline plus the new tests
green, `npm run test:settings` green, and every mutation above run and observed
to fail at its own site. Record the Rust failure-site baseline in this phase's
Result - Phase 2 and Phase 3 compare against it.

### Phase 2: resolve the six conflicts and land the merge commit

One merge commit. Code changes limited to keeping both sides' work and the
minimum needed to make the union compile and its tests express the union. No
behavior change, no `drain_all` fix, no inventory repair.

Resolution per file. Ordering within each union is cosmetic unless stated.

- `terminal.rs`: keep both blocks. Ours contributes five new top-level items
  (search `TerminalTurnStateRequest`, `post_terminal_turn_state`,
  `HelperEnvPlan`, `build_helper_command`, `resolve_create_command`); theirs
  contributes only `close_all_terminals`. The insertion point is the gap between
  `close_terminal` and `impl TerminalSession`, empty at base. `axum::Json` is
  already imported on our side, so theirs compiles under our import set.
- `settingsSections.tsx`, three hunks: take theirs' import line verbatim (strict
  superset); union the section bodies with ours first (disjoint symbol sets - our
  notification context and `notificationAvailability`, their `formatBuildTime`,
  `ConfirmButton` and `AdvancedSection` - with no shared identifier); and build
  `SETTINGS_SECTIONS` as `terminal`, `notifications`, `advanced`, per Decisions.
  Dropping either added descriptor orphans a live component.
- `server.rs`: concatenate both `let` blocks after the shared
  `boot_reconcile().await;`, ours first to match its own CONTRACT comment's
  "immediately after `boot_reconcile`" phrasing, then the untouched
  `build_router(AppState {...})`. There are zero variable-name collisions, and
  the already-merged `AppState{...}` literal below the conflict references
  fields from both sides - so dropping either block fails to compile rather than
  losing behavior silently. That is the safety net; do not weaken it by
  "cleaning up" apparently unused bindings. Then read `router.rs`'s `AppState`
  field list, which auto-merged, and confirm it carries both sets (`attention`
  from ours; `git_probe_cache`, `git_spawn_stats`, `git_state_cache`,
  `epoch_source`, `watch_registry`, `shutdown` from theirs).
- `styles.css`: straight union after the shared `.settings-field-label` rule.
  The selector-set intersection is empty (see Background), so ordering carries
  no cascade meaning here.
- `_index.md`: keep the auto-merged tickets table untouched. In `## Ticket
  Focus`, keep our four closed-ticket writeups plus the blocked
  `260726-chore-dashboard-verify-notification-permission-tier-manually` writeup
  and the Ordering paragraph; keep their
  `260725-feat-dashboard-terminal-steady-state-stream-throughput` and
  `260726-refactor-ws-dashboard-git-fs-watch-invalidation` entries; drop the two
  stale entries per Decisions.
- `260725-research-ws-dashboard-pty-agent-pivot.md`: keep our RESOLVED rewrite of
  the "260624 supersession reversal" open question (independently confirmed -
  `260624-feat-ws-dashboard-managed-cli-terminal.md` carries a "Supersession
  REVERSED 2026-07-25" paragraph), keep their new Activity-Console-retirement
  open question, drop the stale base-identical duplicate.

Mandatory companion edit, in the same commit because the merge does not compile
its own tests without it: `settingsSections.test.ts` asserts
`SETTINGS_SECTIONS.length === 2`. Bump it to 3 and add the `advanced` descriptor
assertions mirroring the existing `notifications` ones -
`SETTINGS_SECTIONS[2].id === "advanced"`, its title, `Component ===
AdvancedSection` (stable module-scope identity, not a per-render arrow), and its
arity. Preserve the four `notificationAvailability` assertions verbatim per
Constraints. These positional assertions are the whole enforcement of invariant
4's registry half.
Mutation: drop the `advanced` descriptor from the registry - the length
assertion fails. Reorder to put `advanced` at index 1 - the
`notificationDescriptor.id` assertion fails. Replace `Component:
AdvancedSection` with an inline arrow - the identity assertion fails.

Also add the browser step that makes the panel real rather than merely
registered: in `dashboard-acceptance.spec.ts`, alongside the existing "insecure
context disables the Notifications toggle" step, assert that the Settings nav
lists an entry with `hasText: "Advanced"` and that activating it mounts content
the section owns (a build-info field is the natural anchor - search
`formatBuildTime` for what it renders).
Mutation: drop the `advanced` descriptor - the nav entry is absent and the step
fails. Note the limit honestly: with the unit assertion in place this step is
redundant *as a drop detector*; its own contribution is proving the component
mounts and its build-info effect resolves, which no unit assertion reaches.

Verification boundary: `cargo test -p ws-dashboard-daemon --no-fail-fast` with
the failure-site list identical to Phase 1's baseline; `npm run build`;
`npm run test:settings` green; the four `agent-attention-notification.spec.ts`
tests and the acceptance file green apart from the known pre-existing sites; and
all of Phase 1's tests still green, since a wrong resolution is exactly what
they exist to catch. State explicitly in the Result whether the whole
`npm run test:browser` suite was run or only individual spec files.

### Phase 3: discharge `drain_all`'s removal obligations and cover the kill-all endpoint

Separate commit, per Decisions.

Fix shape, mirroring `remove_for_work_roots`: drop the `sessions` write lock
first, THEN loop over the drained sessions calling `forget_token` and
`attention.forget` for each. Do not call either while holding the `sessions`
write lock - `forget_token` takes the `tokens` write lock, and the drop-then-loop
in `remove_for_work_roots` exists for that reason. Add the CONTRACT comment
naming this as the fourth choke point, and bump the expected count in
`sessions_removal_paths_are_enumerated` from 3 to 4 as part of the same commit.

**Invariant 5 - a killed terminal's token is rejected afterwards and its file is
gone.** Integration test `close_all_terminals_revokes_callback_tokens` in
`crates/daemon/tests/routes.rs` (search `/api/dashboard/terminals/kill-all` for
the route, registered in `router.rs`). Shape: create a terminal that has a
callback token; POST its turn-state with that token and assert 204; POST to
kill-all; POST the same turn-state again and assert 401; assert
`terminal-tokens/<id>.json` is gone from the state dir.
**The pre-kill 204 assertion is not optional and is not a courtesy.** Without
it, a test whose fixture never issued a usable token passes the 401 assertion
for entirely the wrong reason and reads afterwards as proof that the defect is
fixed. That is the failure mode this whole ticket is about, reproduced inside
its own verification.
Mutation: revert `drain_all` to its merged form - the post-kill POST returns 204
and the token file still exists, failing both assertions while the pre-kill
control stays green.

**Invariant 6 - a killed terminal leaves no AttentionHub entry.** Unit test
`drain_all_forgets_the_attention_entry` in `terminal.rs`'s `mod tests`, modeled
directly on `remove_forgets_the_attention_entry` and using the same
`insert_fake_live_session_for_test` helper. Assert the entry is present before
the drain and absent after; the before-assertion is the same non-vacuity guard
as above.
Mutation: delete the `attention.forget` call from the fixed `drain_all` - fails
at its own site while the token test stays green.

Deferred scope, stated so a reviewer does not read its absence as an oversight:
this phase does NOT close `insert`'s eviction-path token gap. That gap is
recorded in the source, predates both branches, and closing it is a behavior
change with its own blast radius. If it is not already tracked by a ticket, open
an `idea/` one rather than folding it in here.

Verification boundary: `cargo test -p ws-dashboard-daemon --no-fail-fast`, both
mutations run and observed to fail at their own sites, and the failure-site list
still matching Phase 1's baseline.

### Phase 4: restore `_index.md` inventory parity in its own commit

After the merge, 128 ticket files under `ready/` + `todo/` + `idea/` have 107
table rows in `_index.md`. Add the 21 missing rows, sourcing each row's fields
from that ticket's own frontmatter and status directory. Do not edit any ticket
body, and do not change any of the 107 existing rows - of the 92 stems common to
both branches' tables, every row is already byte-identical.

This is a separate commit with its own message precisely because it is not merge
fallout. Folding it into Phase 2 would make `git log` attribute 21 rows of
dev-side backlog debt to a merge resolution, and would make the merge commit's
diff-against-either-parent unreadable for the one property it needs to have.

Verification boundary: the file count and the row count agree, every new row
points at a file that exists, and no existing row moved.

## Promotion prerequisite

This ticket lands in `todo/` and carries no `spec:` entry, deliberately - the
merge itself changes no documented contract, since it only unions two bodies of
already-shipped work.

Before it could reach `ready/`, one spec gap has to be settled, and it belongs
to the dev side's work rather than to this merge. Grepping `ai-docs/spec/` for
`Advanced`, `kill-all`, `build-info` and `dashboard/shutdown` returns nothing:
`#260722-ws-dashboard-settings-panel` describes the settings registry without
mentioning an Advanced section, and no anchor anywhere covers the three
`/api/dashboard/*` control endpoints `d1d6bb31` added. Phase 3 also makes
kill-all's behavior caller-visible in a new way (tokens revoked, attention
cleared), which is a contract statement nothing currently carries.

Do not invent that spec text here. Route it through spec authoring at promotion
time, and decide then whether it attaches to this ticket or to
`260725-feat-dashboard-graceful-shutdown-from-settings`, which is the ticket
`d1d6bb31` actually landed against.

---
title: Merge ws-dashboard-dev into the goal branch - six trivial conflicts, one clean-merge defect
related:
  260725-feat-dashboard-pty-agent-attention-notification: its Phase 4 and Phase 5 CONTRACTs are the two obligations the incoming drain_all skips
  260725-feat-dashboard-graceful-shutdown-from-settings: the dev-side ticket d1d6bb31 landed against; kill-all is its endpoint
  260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile: source of the "std exposes no clear-flag introspection" argument behind invariant 1's test shape
spec:
  - 260726-dashboard-terminal-attention-event-stream
sage-review-design: completed
sage-review-completeness: completed
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

The preview is already slightly behind: its "ours" is one docs-only commit behind
this branch's tip, and `origin/ws-dashboard-dev` has moved on since. `terminal.rs`
is byte-unchanged across that gap, so every count Phase 1 pins was re-measured
against the live file and holds - but the conflict-set enumeration has no such
guarantee, which is why Phase 2 opens by re-running the merge-tree rather than
trusting the table above.

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
returns exactly two hits - the definition and its sole call site inside
`close_all_terminals`, both in `terminal.rs` - which is the zero-tests finding
below.

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
2. The killed terminal's `self.tokens` entry survives and still satisfies
   `token_for`, and `terminal-tokens/<id>.json` is left on disk. Traced through
   `post_terminal_turn_state`: `token_for` returns the stale token,
   `tokens_match` succeeds, the `state.terminals.get(&terminal_id)` lookup
   returns `None` and is treated as a benign race, and the handler falls through
   to `StatusCode::NO_CONTENT`. A killed terminal's token therefore gets 204
   where it should get 401.

**Consequence 1 is not merely a broken source comment - it contradicts shipped
spec text.** Search `attention entry is removed from the snapshot` in
`ai-docs/spec/ws-web-dashboard/index.md`, under
`{#260726-dashboard-terminal-attention-event-stream}`: "A terminal's attention
entry is removed from the snapshot the moment its underlying terminal session
closes (explicit close or owning workRoot/workspace removal), so a reconnect
never reports state for a terminal that no longer exists." Both readings of that
parenthetical convict the merge. On the natural reading - a kill-all *is* an
explicit close, merely a bulk one - the merged code violates the sentence
outright. On the strict enumerative reading, kill-all is a third close path the
sentence never anticipated, and the sentence is incomplete. Either way the fix is
the same in code, and the parenthetical must name the third path so the
ambiguity does not survive. This is the strongest single argument for Phase 3:
the merge does not just skip a convention recorded in a comment, it lands code
that makes a documented, already-published contract false.

Consequence 2 has no such backing. Nothing anywhere in `ai-docs/spec/` documents
the token file, the registry's token choke points, or a 401 from the turn-state
route (verified: zero hits tree-wide for `terminal-tokens`, `forget_token`,
`revoke`, `401`). The token half is genuinely undocumented behavior, which is
why `## Spec Impact` below splits the two halves rather than treating them as one
contract.

**Fourth correction: the on-disk half does not strand permanently.** An earlier
pass claimed `forget_token` is the only caller of
`agent_token_store::delete_token`. It is not - `agent_profile_gc.rs` calls it too,
for every profile directory whose id is absent from the registry's live ids, and
after a kill-all those ids are exactly the ones that are gone. So the token FILE
is eventually reclaimed by the next GC sweep. What is never reclaimed is the
in-memory `self.tokens` entry, and that is the entry `token_for` consults. The
204-instead-of-401 defect and Phase 3's fix shape are unaffected; only the
"permanently" was wrong. This also matters for Phase 3's integration test, which
stays deterministic because the GC sweep task is spawned in `server.rs`, not in
`build_router` - a test built on `build_router` never races it.

**Fifth correction: the source numbers removal paths differently than this
ticket does.** The CONTRACT comment above
`insert_forgets_the_attention_entry_of_a_session_its_own_eviction_retain_drops`
calls `insert`'s eviction retain "a FIFTH session-removal path". That FIFTH
counts something else - review findings in the ticket that introduced it, not
registry methods. This ticket counts *methods that can drop an id from
`self.sessions`*, of which there are three pre-merge and four post-merge. Where
Phase 1's structural test names the paths, it must use this ticket's counting and
say so in one clause, so a reader who greps "FIFTH" is not left arbitrating
between two live numberings.

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
- **Re-measure that site list at the start of every phase; do not diff against
  a list recorded earlier.** Phase 1 established that the integration targets are
  load-sensitive on this machine: under a few hundred leaked
  `terminal-helper`/`ws-dashboard` processes, `routes.rs`, `terminal_lifetime.rs`
  and `terminal_notify_callback_restart.rs` produce extra failures that have
  nothing to do with the tree. Comparing a clean run against a loaded run - in
  either direction - manufactures a signal. The `--lib` target is the stable one;
  the integration targets need their baseline taken under the same conditions as
  the comparison run.
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
- **Between Phase 2 and Phase 3 the branch knowingly contradicts its own spec.**
  Phase 2 lands `drain_all` unchanged, and from that commit until Phase 3 lands,
  the attention-snapshot sentence under
  `{#260726-dashboard-terminal-attention-event-stream}` is false on this branch.
  This is accepted deliberately - folding the fix into the merge commit costs the
  reviewability property the whole split exists to protect - but the two commits
  must not be separated in time or left partially landed. Do not stop after
  Phase 2.

## Spec Impact

Three distinct spec situations, deliberately not merged into one statement. Only
the first is this ticket's to author.

**1. Phase 3, attention half - addressed by an existing confirmed stem.**
`260726-dashboard-terminal-attention-event-stream` (listed in `spec:`) already
carries the contract, quoted in Background. Phase 3 does not introduce
caller-visible behavior here; it makes the code match a sentence that is already
published, and widens that sentence's parenthetical to name kill-all as a third
close path. Expected caller-visible change: none that the spec does not already
promise - a reconnect after a kill-all stops reporting terminals that no longer
exist, which is what the sentence says happens today.
**Contract-first spec: no.** The behavior is not open; it is already written
down. The only spec edit is an enumeration widened to match code the same phase
lands, which is closeout work, not a contract to settle in advance.

**2. Phase 3, token half - new behavior, no existing anchor.** Nothing under
`ai-docs/spec/` documents `terminal-tokens/<id>.json`, the registry's token
choke points, or a 401 from the turn-state route; the only token text is the
Daemon Foundation paragraph establishing that the route is authorized by an
opaque daemon-generated token, and one line under
`{#260727-dashboard-terminal-notify-failure-visibility}` acknowledging the hook
sees a "stale token" as one indistinguishable silent failure among several.
Target spec area: the turn-state route's authorization, whose natural home is
`{#260516-ws-web-dashboard-token-free-pairing-landing}` (where the token is
introduced) with a cross-reference from the attention anchor. Expected
caller-visible change: a callback token stops being accepted once its terminal
is closed by any path, so the hook's POST is rejected rather than silently
succeeding against a dead terminal.
**Contract-first spec: no.** The shape is fully determined by the three existing
removal paths this fix mirrors, and pre-written text would restate Phase 3's own
plan almost word for word. Author it at doc closeout, against what actually
landed.

**3. Phase 2's inherited gap - explicitly NOT addressed here.** The merge carries
the dev side's Advanced settings section and three `/api/dashboard/*` control
endpoints (build-info, shutdown, kill-all) onto this branch, and no spec anchor
mentions any of them - `{#260722-ws-dashboard-settings-panel}` documents exactly
two registered sections, Terminal style and Notifications, plus a planned
hotkey-rebind section. Tree-wide greps for `Advanced`, `build-info`, `buildTime`
and `kill-all` return nothing relevant.

This is inherited debt, not a gate on this ticket, and the reasoning matters
because it is the one place the spec-address gate is being answered with "no" for
caller-visible behavior. The gate exists to stop implementation from starting
against an undocumented *or unstable* contract. Here the contract is
undocumented but entirely stable: it is shipped, frozen code on
`origin/ws-dashboard-dev` that this ticket transports without reading it as a
design question. Authoring that spec here would mean documenting another
workstream's shipped behavior from the outside, on behalf of
`260725-feat-dashboard-graceful-shutdown-from-settings` - the ticket `d1d6bb31`
actually landed against, which exists only on the dev branch and therefore cannot
be edited from here until this very merge lands. **Phase 5 performs that
routing** - a deferral with no phase attached is how a declared gap becomes a
forgotten one.
**Contract-first spec: no.**

## Phases

Phase 1 runs against the pre-merge tree. Phases 2 through 5 are strictly ordered
after it and after each other. All new unit tests named below go in
`terminal.rs`'s existing `#[cfg(test)] mod terminal_portability_skeleton_tests` -
the file has no `mod tests`.

### Phase 1: pin the surviving invariants as tests, before the merge

Write the tests for the four invariants that already hold on the goal branch,
and get them green on the pre-merge tree. Their value is entirely in being
known-meaningful *before* the merge muddies attribution.

Where a test is a source scan rather than a behavioral assertion, say so at the
test and say why - a reader who mistakes a source scan for a behavioral proof
will over-trust it.

**The shared scanning helper, and why it is not optional.** Invariants 1, 2 and 3
are all source scans over "the production half of `terminal.rs`", and the naive
readings of that phrase are all wrong in ways that make the tests silently
useless. Build one helper first and let all three use it.

- **There is no `mod tests` in this file.** `grep -c "mod tests" terminal.rs`
  returns 0; the unit-test module is `mod terminal_portability_skeleton_tests`.
  Truncating at a `mod tests` marker matches nothing, scans the whole file, and
  makes invariant 1 count 3 instead of 1.
- **Production code continues AFTER the test module,** so truncating at the first
  `#[cfg(test)]` is also wrong - it would silently drop `impl TerminalError` and
  five free functions from the scanned region, giving every one of these
  invariants a blind spot exactly where a careless addition is most likely.
  There are three top-level `#[cfg(test)]` items, not one.
- The rule that works: walk lines; when a line is exactly `#[cfg(test)]` at
  column 0, skip forward past the next line that is exactly `}` at column 0;
  otherwise keep the line. Then drop lines whose trimmed form starts with `//`.
  Verified against the current file: this excises exactly the three `#[cfg(test)]`
  spans and keeps 2374 of 3566 lines, 1704 after comment-stripping.
- **A second variant is required, and this is the subtle one.** rustfmt splits
  receiver chains across lines - the file writes `self` / `.sessions` /
  `.write()` / `.expect(...)` on four separate lines. So a literal search for
  `self.sessions.write()` over the line-based text finds **zero**, and a search
  for `self.tokens` finds **2** where the honest answer is 3. An earlier draft of
  invariant 3 asserted 3 on exactly that reading and was wrong. Provide a
  `flattened()` variant that removes ALL whitespace, and do every multi-token
  count against it. Every number below is measured on that variant.
  **Flatten AFTER comment-stripping, not before** - the order is load-bearing,
  not stylistic. The three CONTRACT comments contain backticked `self.tokens`,
  and flattening collapses the backticks against the identifier, so flattening
  the un-stripped text makes invariant 3's total read 7 instead of 3. The
  read/write splits and the `sessions` count happen to be identical either way,
  which is exactly why this would be found late and by the wrong test.

**Invariant 1 - exactly one production `env_clear()` in `terminal.rs`.** Search
`env_clear` in that file: the sole production call site is inside
`build_helper_command`'s `HelperEnvPlan::ClearAndSet` arm; every other hit is
either a comment or the `#[cfg(test)]` positive control inside
`helper_spawn_default_no_command_matches_existing_arg_shape`. Half of this
invariant is already discharged by the merge itself: that existing test asserts
`helper_env_plan(None, None, vec![]) == HelperEnvPlan::InheritHost`, so a
resolution that drops the goal side's block fails to *compile*. The half nothing
covers is "no SECOND site appears". Add
`terminal_rs_has_exactly_one_production_env_clear`: count `.env_clear(` in the
comment-stripped production text and assert **1** (measured). This one needs no
flattening - the call is written on a single line. The source-text form is
forced, not preferred: `260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile`
established that `std::process::Command` exposes no public clear-flag
introspection, so there is no runtime observable for "some other call site
cleared the env".
Mutation that fails it: add a second `.env_clear()` anywhere in production code
(count 2), or delete the sole one (count 0). Falsifiable in both directions,
which the comment-stripping must not break - verify by temporarily adding a
commented `.env_clear()` line and confirming the count does not move.

**Invariant 2 - the removal-path obligations.** Two tests, because one form
cannot carry both halves.

- Behavioral: the attention half is already covered on all three existing paths
  - search `forgets_the_attention_entry` in `terminal.rs` for the three tests and
  for `insert_fake_live_session_for_test`, the helper they share. The token half
  is covered nowhere. Add `remove_forgets_the_callback_token` and
  `remove_for_work_roots_forgets_the_callback_token`, each asserting both that
  `token_for` stops resolving and that the on-disk `terminal-tokens/<id>.json`
  is gone. Each needs a pre-removal assertion that `token_for` *does* resolve;
  without it a test whose fixture never stored a token passes for the wrong
  reason.

  **Two fixture traps, both of which produce a green test that proves nothing.**
  First, `insert_fake_live_session_for_test` sets `callback_token: None`, and
  `remember_token` no-ops without one - so the helper the three attention tests
  share cannot seed a token, and a test built on it as-is passes its post-removal
  assertion vacuously. Either extend the helper to take a token or add a
  token-bearing sibling; the pre-removal assertion is what catches getting this
  wrong.
  Second, and worse: those three model tests all use `TerminalRegistry::default()`,
  whose `state_dir` is `crate::persistent_state::default_state_dir()` - the
  developer's **real** state directory. Writing the mandated on-disk assertion
  against that registry creates and deletes files under the real
  `terminal-tokens/` on every `cargo test` run, and passes, so nobody ever
  notices. These two tests MUST construct the registry through
  `TerminalRegistry::new(...)` with an explicit temp state dir; search
  `TerminalRegistry::new(` inside the test module for the one existing test that
  already does this and copy its shape.
  Mutation: delete the `self.forget_token(...)` call from `remove` - the
  post-removal assertion in `remove_forgets_the_callback_token` fails; the
  attention test stays green, proving the two halves are independent.

- Structural: `sessions_write_lock_sites_are_enumerated`. Count
  `self.sessions.write()` in the **flattened** production text and assert **4**
  pre-merge, **5** post-merge (measured; `drain_all` adds the fifth).

  Read the count as "places where the sessions map's membership can change",
  not "removal paths" - the two are deliberately different, and a bare
  `.remove(`/`.retain(`/`.drain(` scan is NOT an acceptable substitute. Measured:
  that scan flags six production functions, three of which are unrelated -
  `clear_profile_pending` mutating `pending_profile_ids`, `forget_token`
  mutating `self.tokens`, and `close_terminal`, which is a false positive for a
  different reason again: it does not mutate a map at all, it calls the
  registry's own `remove` method, so it is a delegation rather than an
  independent removal path. A scan cannot distinguish any of these, because the
  receiver sits on an earlier line. Pinning the write-lock acquisition instead is both mechanically
  unambiguous and the stronger invariant: nothing can drop an id without taking
  that lock.
  The test carries a comment enumerating all four sites with each one's
  discharge status - `insert_unchecked` adds only and owes nothing;
  `remove` and `remove_for_work_roots` discharge both obligations; `insert`'s
  eviction retain discharges attention only, by the deferral recorded in its own
  CONTRACT comment. State in that comment that this ticket counts *methods that
  take the `sessions` write lock*, which is a different count from the "FIFTH
  session-removal path" phrasing in the existing CONTRACT comment - otherwise a
  reader who greps "FIFTH" is left arbitrating between two live numberings.
  This is the only test in the ticket that would have caught the actual defect:
  `drain_all` broke no assertion because no assertion knew it existed.
  Mutation: add any new method taking the `sessions` write lock - count 5
  pre-merge, fail. The failure message must tell the author what to do, since the
  whole point is to force a discharge decision at the moment a new site is
  written.

**Invariant 3 - access to `self.tokens` is confined to its three choke points.**
Source scan, `tokens_map_access_is_confined_to_its_choke_points`, over the
**flattened** production text. Assert all three of: `.tokens` occurs **3** times,
`self.tokens.read()` **1** time (in `token_for`), and `self.tokens.write()` **2**
times (in `remember_token` and `forget_token`). All three measured. The
decomposed form is worth the extra two assertions: the total alone would stay 3
if someone converted a read into a write, and the read/write split is what the
three CONTRACT comments actually claim.
Behavioral verification is not available: an extra reader elsewhere would be
indistinguishable from `token_for` at runtime, which is why the map has a
comment-enforced contract rather than a type-enforced one.
Mutation: add a `self.tokens.read()` anywhere else in the impl - total 4 and
read-count 2, fail on both.

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

### Result (3c6b465f) - 2026-07-27

Five tests, not six - the plan's Verification Plan said six while its own
Implementation Plan named five. Five is correct: invariant 2 splits into two
behavioral tests plus one structural, invariants 1 and 3 are one each, and
invariant 4 adds no code. The implementer flagged the arithmetic rather than
inventing a sixth test to match a number.

Landed: `terminal_rs_has_exactly_one_production_env_clear`,
`remove_forgets_the_callback_token`,
`remove_for_work_roots_forgets_the_callback_token`,
`sessions_write_lock_sites_are_enumerated`,
`tokens_map_access_is_confined_to_its_choke_points`, plus the shared
`production_text()`/`flattened()` scan helpers and a token-bearing fixture
sibling.

**Baseline, and a caveat on it that Phases 2 and 3 must act on.** The recorded
site list is `crates/daemon/tests/routes.rs:1066` and `:1383`, with lib tests
237 -> 242. That list is NOT stable on this machine. A later full-suite run
during review surfaced additional failures at `routes.rs:4358`, `:13265`,
`:13388`, `terminal_lifetime.rs:191` (x3) and
`terminal_notify_callback_restart.rs:391`; the reviewer traced them to machine
state rather than the diff - `ps` showed 531 live leaked
`ws-dashboard`/`terminal-helper`/`claude` processes at the time, none of the
failing files are touched by this phase, and the isolated `--lib` target was
fully green (242 passed, 0 failed). The consequence for later phases is
concrete: **re-measure the baseline immediately before each phase's own run
instead of diffing against this recorded list.** A stale baseline would attribute
load-induced integration failures to the merge, which is the same
misattribution this ticket exists to prevent, pointed the other way.

Deviations, both accepted:
- A token-bearing sibling fixture (`insert_fake_live_session_with_token_for_test`)
  was added rather than changing `insert_fake_live_session_for_test`'s signature,
  because that helper also has a caller in `agent_profile_gc.rs` and this phase is
  scoped to one file. The ticket allowed either.
- `cargo fmt` and `cargo clippy` already fail on this branch, confirmed
  pre-existing by comparison against the unmodified tree: 29 `terminal.rs` fmt
  diffs and a compile-blocking `never_loop` deny in unrelated
  `agent_attention.rs`. No blanket `cargo fmt` was run, since it would rewrite
  29 sites outside this phase's scope. This is standing cleanup debt, untouched
  and uncaused here.

Finding the plan surfaced and the ticket had not: neither `insert` nor
`insert_unchecked` writes `terminal-tokens/<id>.json` - `remember_token`
populates the in-memory map only, and the file is written solely by
`TerminalSession::spawn`. A fixture built on the insert path therefore has an
in-memory token and no file, so the on-disk half of both token tests would have
passed vacuously. The fixture calls `agent_token_store::write_token` directly to
close that. Recorded in the terminal mental model.

Verification: every mutation was run, observed at its own site, and reverted -
including the negative control (a commented-out `.env_clear()` leaves the count
unmoved, proving comment-stripping does not break falsifiability in either
direction). `npm run test:settings` exit 0 with the four
`notificationAvailability` assertions intact, so invariant 4 was observed rather
than assumed.

Review found one Important issue, now fixed: the tokens test used three
sequential `assert_eq!`, and since `assert_eq!` panics at the first mismatch, a
single mutation could never demonstrate the "fail on both" the ticket asks for.
Restructured to compute all counts first and assert once over collected
mismatches; the re-run mutation reports both affected deltas in one panic while
the untouched `writes` count correctly does not appear. Correctness review
independently re-implemented the scan pipeline and reproduced every asserted
number, confirmed flatten-before-strip would yield 7 instead of 3, and confirmed
the scan tests' own literals sit inside the excised span so an excise slip fails
loud rather than passing silently. It also verified forward: `drain_all` on
`origin/ws-dashboard-dev` splits its receiver across lines in a form the
flattened scan does catch, so `sessions_write_lock_sites_are_enumerated` will in
fact fire at the merge.

### Phase 2: resolve the six conflicts and land the merge commit

One merge commit. Code changes limited to keeping both sides' work and the
minimum needed to make the union compile and its tests express the union. No
behavior change, no `drain_all` fix, no inventory repair.

**Re-verify the conflict set before resolving anything.** The six-file
enumeration below is pinned to a preview OID computed when this ticket was
written, and both branches can move. Re-run `git merge-tree --write-tree` against
the current tips first and confirm the conflicted-file set is still exactly these
six. If it is not, stop and re-survey rather than resolving against a stale map -
a seventh conflicted file is precisely the kind of thing this ticket argues
nobody looks for.

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

**Mandatory companion edit A, Rust.** Landing `drain_all` adds a fifth
`self.sessions.write()` site, so Phase 1's
`sessions_write_lock_sites_are_enumerated` goes red at this commit unless the
expected count moves 4 → 5 here. Do that in this commit, and extend the test's
enumerating comment with a `drain_all` line recording that it discharges
**neither** obligation. That is not a workaround for an inconvenient test - it is
the merge commit stating in its own diff exactly what it is landing, which is the
one place a reader is guaranteed to look. Phase 3 then rewrites that line when it
discharges them.
Do not instead delete, ignore or `#[ignore]` the test. Its whole purpose is to
fire here.

**Mandatory companion edit B, frontend,** in this commit because the merge does
not compile its own tests without it: `settingsSections.test.ts` asserts
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
registered - **in a separate commit after the merge commit, still within this
phase.** Unlike the two companion edits above it is not needed to make the union
compile or its existing tests express the union; it is net-new coverage, and
folding net-new coverage into the merge commit costs the same reviewability the
`drain_all` split is protecting. In `dashboard-acceptance.spec.ts`, alongside the existing "insecure
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
in `remove_for_work_roots` exists for that reason. Add the CONTRACT comment on
`drain_all` itself, mirroring the two on `remove`.

The expected count in `sessions_write_lock_sites_are_enumerated` does NOT move
here - Phase 2 already took it to 5, which is correct both before and after this
fix, since discharging the obligations does not add or remove a write-lock site.
What this commit changes in that test is its enumerating comment: the `drain_all`
line stops reading "discharges neither" and starts reading "discharges both",
alongside `remove` and `remove_for_work_roots`. Leaving that comment stale would
leave the codebase's only enumeration of these paths asserting the exact
falsehood this phase just fixed.

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
`drain_all_forgets_the_attention_entry`, modeled
directly on `remove_forgets_the_attention_entry` and using the same
`insert_fake_live_session_for_test` helper. Assert the entry is present before
the drain and absent after; the before-assertion is the same non-vacuity guard
as above.
Mutation: delete the `attention.forget` call from the fixed `drain_all` - fails
at its own site while the token test stays green.
This invariant's authority is the published spec sentence under
`{#260726-dashboard-terminal-attention-event-stream}`, not merely `remove`'s
CONTRACT comment - so this test is the one that stops a documented contract from
being false. The same phase must widen that sentence's parenthetical to name
kill-all as a third close path; leaving the code fixed and the enumeration
two-wide reproduces the exact ambiguity that let `drain_all` through.

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

### Phase 5: route the inherited spec debt to its owning ticket

`## Spec Impact` item 3 declines to document the Advanced section and the three
`/api/dashboard/*` control endpoints here, and says to route it afterwards. This
phase is that routing, and it exists because "route it afterwards" with no phase
attached is how a declared deferral becomes a silent one - the exact failure mode
this whole ticket is about, committed against itself.

Sequentially dependent on Phase 2: `260725-feat-dashboard-graceful-shutdown-from-settings`
exists only on `origin/ws-dashboard-dev` and is not editable from this branch
until the merge lands.

Deliverable: append a spec-impact note to that ticket recording that its shipped
behavior - the Advanced settings section, `build-info`, `shutdown`, `kill-all` -
has no spec anchor, and that `{#260722-ws-dashboard-settings-panel}` still
documents only two registered sections. Do not author the spec text; that is that
ticket's own work, against behavior its authors know and this one does not. If
its status or shape makes an appended note the wrong instrument, open an `idea/`
ticket instead and say why in the Result.

Also record there what Phase 3 made caller-visible (a callback token stops being
accepted once its terminal is closed by any path, including kill-all), since that
is a contract statement about their endpoint produced by this ticket's work.

Verification boundary: the note exists on a ticket that resolves on this branch,
names all four undocumented surfaces, and does not invent spec text.

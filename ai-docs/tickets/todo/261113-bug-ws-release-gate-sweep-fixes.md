---
title: "Release-gate sweep fixes: eight Important findings in c8d5479fe..be2e22005"
sage-review-design: blocked
sage-review-completeness: blocked
related:
  261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps: "landed c41f18fc0; reworked pi_lead.go launch and fallback (#5, #6)"
  261113-feat-ws-hosted-lead-bundled-plugin-only: "#6 is an owner-approved narrow deviation from its (392)=c ((526)=a)"
  261113-feat-ws-dashboard-iframe-shell: "ready, in flight; forwards shell hotkeys from session frames (#1)"
  261113-bug-ws-only-the-dashboard-daemon-runs-an-exe: "ready; its Phase 2 reworks how mailbox-waiter.ts spawns the waiter (#3)"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "ready, in flight; touches page.tmpl link/check templates (#7, text overlap)"
---

# Release-gate sweep fixes: eight Important findings in c8d5479fe..be2e22005

## Background

The release-gate sweep over `c8d5479fe9e5d4b3b944547fceae9e0aa45d6e21..be2e220051377fc68a093210c2465d6e8d599a8f`
(run by the MAIN lead, handed to EXEC 2026-11-13 by owner (514)=ok) returned
NEEDS FIX: 8 Important, 0 Critical. The range is stamped `block` in
`ai-docs/.review-ledger.md` with this ticket as its ref; the minors are
recorded as `## Follow-ups` on the ledger commit. After this ticket lands, the
range is re-reviewed with `lead-review` keeping base `c8d5479fe` and a head
that includes the fix commits.

Trap: `c8d5479fe` is not an ancestor of `b0185116f` (the ship-cut side branch
rejoins at `9fb5ce58e`), so never review this range with a two-dot tree diff.

Findings (full evidence in the reviewer reports under
`~/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/`: `5c93aa53-01-findings.md`,
`4c5dafde-01..05-sweep-*.md`; the cache path is machine-local evidence, the
findings below are self-contained):

1. **Ctrl+K and Ctrl+Space are swallowed everywhere.**
   `agents-plugin-tool/internal/wsdashboard/assets/tracker.js:5721-5725` and
   `:6263`: the search-palette capture listener takes these keys even from
   terminal cards (the xterm helper textarea) and from the composer.
2. **Memo length caps disagree** (re-graded from Minor). The server caps a
   memo at 8000 bytes (`wsdashboard/memos.go:155`); the client caps it at 8000
   UTF-16 units (`dashboard.tmpl:14`, `memo.js` `maxLength = 8000`). A CJK memo
   longer than about 2700 characters passes the client and is then refused
   with a byte-count error.
3. **A stale stream line wakes the session for mail already read.**
   `createMailboxWait` (`agents-plugin/hooks/claude-mod/register.ts:493`,
   mirrored in wsflow): a buffered `--stream` line for already-drained mail
   wakes the session again; in notice mode the extra turn's recv is empty.
   Regression from 0b5eada71 and a10df27ef. The Pi `mailbox-waiter.ts` has the
   same behaviour at minor cost.
4. **Repo-layout literal in shipped Go.**
   `agents-plugin-tool/internal/wsagent/lead_plugins.go:33` falls back to
   `runtime.Caller` + `../../../agents-plugin` for dev builds, breaking
   Architecture Rule 4; `test_go_string_literals_downstream_neutral` fails at
   be2e22005.
5. **A default-mode hosted Pi lead never starts under a symlinked state dir.**
   The main-module guard in `wsagent/pi_lead.mjs:72` (launched from
   `pi_lead.go:54-76`) compares the realpath'd `import.meta.url` with the
   unresolved `process.argv[1]`; under a symlinked state dir or macOS `/var`
   the adapter exits 0 without starting Pi.
6. **Existing hosted Pi leads on a standalone or wrapper Pi cannot restart**
   (re-graded from Minor). `wsagent/pi_lead.go:57-60`, `runner.go:503-509`:
   legacy lead records read as `default` isolation, and default mode refuses a
   Pi install without the JS SDK, so these leads fail after the upgrade.
7. **Board link fields lose characters to Markdown** (re-graded from Minor).
   `wswhiteboard/assets/page.tmpl:65`, `:95`, `:106` (from 5695371fb): link
   fields and readings render as full inline Markdown, so
   `rsrc/*.md and internal/*.go` loses its asterisks to emphasis, while
   `FormatQuery` still prints these fields with `Plain`.
8. **macOS test failures after symlink resolution of `root_override`.**
   `internal/mcp/server.go:2212-2218` (from 8d14fee74): `root_override` now
   goes through `canonicalGitRoot` (EvalSymlinks); four `internal/mcp` tests
   fail on macOS (`/var` -> `/private/var`); CI runs only ubuntu and windows. A
   symlinked override binds the child key to the resolved path.

## Decisions

Technical, set by the EXEC lead from the reviewers' proposed fixes
(2026-11-13); reviewer-proposed fixes are adopted unless noted.

1. (#1) The capture listener skips the Ctrl shortcuts for `.xterm` targets and
   for textarea and contenteditable targets, as `/` already does. The same
   target check runs wherever the keydown is seen: in any same-document
   listener and, once `261113-feat-ws-dashboard-iframe-shell` lands, in the
   frame-side forwarder before it forwards a shell hotkey (the shell then
   receives a message, not the original target). Rejected: a per-card
   opt-out (every future input surface would need it); a check in the shell
   document only (it cannot see the target of a forwarded key).
2. (#2) The client enforces the server's contract: 8000 UTF-8 bytes of the
   trimmed text (as `memos.go:154` measures), checked on input and before
   submit (`maxLength` cannot count bytes, and `maxlength=8000` stays as a
   harmless upper bound); memo.js has no counter today, so the refusal is a
   byte-count message through `say()` / `#memo-status`, localized through
   the i18n tables (1073074d: localize new memo UI strings); no new counter
   is built. The server cap stays. Rejected: raising the server cap or
   switching it to characters (changes a stored-data contract for a UI bug).
3. (#3) An arrival line buffered when the waiter re-arms is compared, by
   its envelopes (`mailboxWaitArrivalJSON` emits named/reply), against what
   the drain received; only an arrival the drain did not cover may wake the
   session. This holds in claude-mod, its wsflow mirror, and Pi
   `mailbox-waiter.ts`. Rejected: discarding buffered lines (mail arriving
   after the drain would wait for the 10-minute pending-arrival reminder,
   `cmd/ws-mcp/mailbox.go:112-113`); fixing claude-mod only (Pi keeps the
   same spurious turn). Design review 2026-11-13.
4. (#4) Remove the `runtime.Caller` repo-layout fallback; dev builds resolve
   the plugin root through the same generic mechanism release builds use
   (resource root or launcher-provided environment), never a path literal
   naming this repository's layout (`ai-docs/manuals/shipped-surface-boundary.md`).
   This deliberately reverses 52dc582b's "source checkout fallback for dev":
   Architecture Rule 4 makes the shipped-surface boundary non-negotiable, so
   a dev build that needs a source checkout gets it from the launcher
   environment instead.
5. (#5) The guard compares realpaths on both sides. Rejected: splitting the
   entry script (more files for the same check).
6. (#6) Only when default mode is impossible (the lead's Pi has no JS SDK),
   the hosted Pi lead starts in isolated mode with a visible
   `PluginWarning`; the stored record is unchanged, and default stays
   default everywhere else ((526)=a, owner 2026-11-13; queued by EXEC as
   (525) and renumbered on MAIN's board). This is a narrow, owner-approved
   deviation from `261113-feat-ws-hosted-lead-bundled-plugin-only` ((392)=c:
   a resumed session relaunches in its recorded mode). Rejected: (b) legacy
   records without an isolation field read as `isolated`; (c) keep refusing
   with an actionable relaunch error. Mechanics (design review, technical):
   the fallback triggers only on the missing-SDK case, distinguished by a
   sentinel or typed error from `piSDKPaths` (`pi_lead.go:154-185`), never on
   other path-resolution errors; because `PluginWarning` holds one value
   (`runner.go:2111-2112`), when the bridge warning and the isolated start
   both apply, the two texts are combined into one warning.
7. (#7) Link fields and readings render as plain text on the page, matching
   `FormatQuery`'s `Plain`. Rejected: escaping Markdown in the query output
   instead (the fields are paths and identifiers, not prose).
8. (#8) Tests compare against `EvalSymlinks(want)`; the `root_override`
   tool description states the resolved-path contract (a symlinked override
   binds to its resolved path). Rejected: dropping the symlink resolution
   (8d14fee74 needs one canonical key per worktree).
9. Compatibility tier: T0. Dashboard and board asset content, hook script
   behavior, launcher-internal Go, and one tool description sentence; no
   tool, argument, store, or protocol change.

## Open Decision Queue

Last ID: (533)

(533) [open] [policy] How board link fields and readings render (#7): (a) fully plain text, or (b) the same inline pipeline without emphasis
- The finding: `rsrc/*.md and internal/*.go` in a link field loses its asterisks to emphasis. Link fields and readings render through `pageData.TextInline` (`wswhiteboard/markdown.go:218-222`), added in 5695371fb to keep raw HTML literal while still honoring code spans, item/panel references, bare URLs and bare paths; `render_test.go:571` pins `see {{2}}` in a Links field rendering as a reference link. The EXEC draft decision ("plain text, matching `FormatQuery`'s `Plain`") would remove references and make URLs and paths in a link field unclickable.
- a: fully plain text (matches the query output exactly; drops references and clickable links; rewrites the pinned tests).
- b: drop only the emphasis parser from the inline pipeline for these fields (asterisks and underscores stay literal; references, code spans, URLs and paths keep working; the query output stays `Plain`).
- Readings (`Reading`, `CheckReading`) are agent prose; under (b) they follow the same rule so one field family has one renderer.
> b: it fixes the reported loss without removing the reference and link behavior 5695371fb added on purpose and the tests pin.

## Constraints

- `261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps` landed
  (c41f18fc0) and reworked `pi_lead.go`'s launch and fallback paths. #6's
  isolated fallback is its own branch, decided where `piSDKPaths` refuses
  (`pi_lead.go:57`), not the bridge `bundleErr` branch; it sets
  `PluginWarning` with its own text naming the isolated start and the missing
  JS SDK, so the bridge warning and this one stay distinguishable, and the
  `else` branch's `PluginWarning("")` reset (`pi_lead.go:48-49`) must not
  clear it (EXEC lead, technical, from fact population 2026-11-13). Rejected:
  reusing the bridge branch (it selects the MCP bridge, not isolated mode).
  Any `agents-plugin-pi/src` edit needs
  `cd agents-plugin-pi && npm install && npm run bundle` so the committed
  bundle stays fresh.
- Overlap: ready `261113-feat-ws-dashboard-iframe-shell` (in flight) forwards
  shell hotkeys from session frames; per Decision 1 the target check runs in
  the frame-side forwarder. Whichever lands second carries the check into the
  other's code.
- Overlap: ready `261113-bug-ws-only-the-dashboard-daemon-runs-an-exe` Phase 2
  reworks how `mailbox-waiter.ts` spawns the waiter; #3 edits its arrival
  handling. Whichever lands second rebases and rebuilds the Pi bundle.
- Overlap (text only): ready `261112-feat-ws-whiteboard-topic-tickets-sticky-head`
  touches `page.tmpl`'s link and check templates (#7).
- #4: tests that relied on the `runtime.Caller` fallback set `WS_RSRC_ROOT`
  (`scripts/dev.py:226` already sets it for dev; `pi_lead_test.go` stubs
  `piBundledPluginRoot`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 52dc582b (2026-11-13, commit): "Resolve the daemon package via its resource root (source checkout fallback for dev); missing sibling Pi entry uses the native MCP bridge with hosted identity context and a persisted warning." — bearing: reversed by Decision 4 (Architecture Rule 4)
- 261113-feat-ws-hosted-lead-bundled-plugin-only (2026-11-13, Decisions): "The isolation level persists on the session record; a resumed or relaunched session starts in the same isolation mode (default or isolated) it was created with." — bearing: constrains
- 261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps (2026-11-13, Decisions): "every new fallback reason (missing deps, missing or stale bundle) goes through the existing `bundleErr` branch in `piLeadLaunch` ... which sets `PluginWarning`, not a parallel branch." — bearing: constrains (that rule covers bridge fallback reasons; #6 is an isolated start, not a bridge fallback, so it is a separate branch with its own warning text)
- 261112-feat-ws-dashboard-search-palette (2026-11-13, commit c2a35b2d): "Preserve native modal focus handling, guard IME confirmation and slash in editors" — bearing: supports
- 261111-feat-ws-mailbox-wait-without-process-relaunch (2026-11-13, commit 0b5eada7): "Keep the existing per-wake controller seams and authoritative recv drains; share Pi's compatibility decision across key rebinds. ... wsflow stays byte-identical." — bearing: constrains
- 261113-bug-ws-playbook-render-root-override-key-bound-to-lead-root (2026-11-13, Result 8d14fee74): "Regression coverage uses real keys and linked worktrees for both spawn paths ... and covers relative-subdirectory and symlink render overrides." — bearing: constrains
- 261112-feat-ws-dashboard-rail-memo-pad (2026-11-13, commit 1073074d): "Forward from worker: localize new memo UI strings." — bearing: constrains
- 261113-feat-ws-dashboard-iframe-shell (2026-11-13, Decisions): "the postMessage set also carries key forwarding from a focused frame to the shell for the shell-owned hotkeys ... Rejected: leaving hotkeys frame-local" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdashboard/assets/tracker.js, memo.js, dashboard.tmpl, memos.go; claude-mod/register.ts (ws and wsflow), agents-plugin-pi/src/mailbox-waiter.ts; wsagent/lead_plugins.go, pi_lead.go, pi_lead.mjs, runner.go; wswhiteboard/assets/page.tmpl; mcp/server.go and its tests |
| scope.surface | cross-module | dashboard assets, hook scripts, launcher Go, board template, and the root_override tool description sentence in internal/mcp |
| scope.new_public_symbol | no | none; Decisions add no exported symbol |
| scope.new_type_contract | no | none; one tool description sentence changes, no signature |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs, wsdashboard/memos_test.go, wsagent/pi_lead_test.go, wsagent/lead_isolation_test.go, agents-plugin-pi/test/mailbox-waiter.test.ts, claude-mod register tests, agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py; new cases are added inside these |
| complexity.reuse_points | confirmed | wsrsrc.ResolveRoot in bundledPluginRoot (#4); PluginWarning in pi_lead.go with its own text (#6; the bridge bundleErr branch is not reused); wswhiteboard TextInline and Plain (#7); canonicalGitRoot (#8); the '/' target check in qShortcut (#1) |
| complexity.side_effect_risk | moderate | #6 changes launch mode for legacy hosted Pi lead records and #3 touches wake timing in three waiters |
| risk.correctness | moderate | eight independent fixes; #3 stale-arrival discard can drop a wake that is still wanted |
| risk.fit | moderate | #6 is a recorded deviation from (392)=c and from the extension-deps Decision 6 single-signal rule; #1 overlaps iframe-shell hotkey routing |
| risk.test | moderate | #8 failures reproduce only on macOS, which CI does not run; #5 needs a symlinked-directory adapter run |
| risk.security_or_contract | moderate | #2 settles a stored-data byte contract on the client; #6 changes a hosted launch path; #8 changes a tool description |

## Phases

### Phase 1: Fix the eight Important findings

Apply Decisions 1-8.

Verification:
- #1: a `search-palette.test.cjs` case where Ctrl+K / Ctrl+Space from an
  `.xterm` helper textarea and from the composer are not taken by the palette,
  and are still taken from the page body; when iframe-shell has landed, a
  forwarded-key case where the frame-side forwarder does not forward them
  from those targets.
- #2: a test that a CJK memo over 8000 bytes is refused by the client with a
  byte-count message and one at the limit is accepted by both client and
  server.
- #3: claude-mod (and wsflow mirror) and Pi tests where a second arrival
  lands before recv: no extra wake for drained mail; and the inverse case:
  mail that arrives after the drain and before re-arm still wakes the
  session promptly.
- #4: `test_go_string_literals_downstream_neutral` passes; a unit test that
  `bundledPluginRoot` resolves from `WS_RSRC_ROOT` / the resource root with
  no source-tree fallback.
- #5: a regression test that runs the adapter through a symlinked directory
  and sees Pi start.
- #6: a test that a non-SDK `piSDKPaths` error does not trigger the isolated
  fallback, and that combined warnings keep both texts; a runner test that a legacy lead record (no isolation field) on a Pi
  without the JS SDK starts isolated with the `PluginWarning` and its record
  unchanged, and that a Pi with the SDK still starts in default mode.
- #7: a render test that `rsrc/*.md and internal/*.go` in a link field
  renders verbatim.
- #8: run `go test ./internal/mcp/` locally on darwin (CI has no macOS
  runner); the Result names the four tests that failed before and pass
  after.
- Known darwin failures on develop (sticky-head worker, 2026-11-13), likely
  #5 / #8: `internal/mcp` `TestRenderDispatchSeparatesResourceAndWorktreeRoots`,
  `TestRenderFromWorktreeSessionCommitsInThatWorktree`,
  `TestRenderPathMintedChildRecordsLeadParent`; `internal/wsagent`
  `TestClaudeLeadLocalSettingsCanonicalWorktree`, `TestPiLeadResolverAdapter`.
  The worker confirms each cause, fixes those that belong to #5 / #8, and
  reports any other cause in its Result. `internal/mcp` needs
  `-timeout 40m` (the default 10m panics).
- `go test ./...` for the touched packages, the claude-mod / wsflow / Pi test
  suites, and the wsflow mirror drift tests pass.

## Blocked (2026-11-13)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Decision 7 plain text contradicts the tested link-field contract | important | missing |
| 2 | Decision 3 discard can lose a wanted wake | important | autonomous |
| 3 | #1 target check must run in the frame-side forwarder after iframe-shell | important | autonomous |
| 4 | #6 fallback only on the missing-SDK error; PluginWarning single value | minor | autonomous |
| 5 | #2 has no counter today; refusal text not localized | minor | autonomous |
| 6 | Pi mailbox-waiter.ts overlaps an undeclared ready ticket | minor | autonomous |
| 7 | #4 dev resolution and #8 test host | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Frontmatter lacks related links | minor |
| 2 | #4 verification not a concrete check | minor |
| 3 | #8 does not name the four failing tests | minor |
| 4 | #3 leaves discard versus re-peek open | minor |

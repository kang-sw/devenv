---
title: "Release-gate sweep fixes: eight Important findings in c8d5479fe..be2e22005"
sage-review-design: required
sage-review-completeness: required
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
   The main-module guard in `wsagent/pi_lead.mjs:62` (launched from
   `pi_lead.go:75-91`) compares the realpath'd `import.meta.url` with the
   unresolved `process.argv[1]`; under a symlinked state dir or macOS `/var`
   the adapter exits 0 without starting Pi.
6. **Existing hosted Pi leads on a standalone or wrapper Pi cannot restart**
   (re-graded from Minor). `wsagent/pi_lead.go:56-60`, `runner.go:497-503`:
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
   for textarea and contenteditable targets, as `/` already does. Rejected:
   a per-card opt-out (every future input surface would need it).
2. (#2) The client enforces the server's contract: 8000 UTF-8 bytes, counted
   on input (`maxLength` cannot count bytes), with the counter and refusal
   text in bytes; the server cap stays. Rejected: raising the server cap or
   switching it to characters (changes a stored-data contract for a UI bug).
3. (#3) Arrivals buffered before a drain completes are discarded or
   re-peeked before they may wake the session, in claude-mod, its wsflow
   mirror, and Pi `mailbox-waiter.ts`. Rejected: fixing claude-mod only (Pi
   keeps the same spurious turn).
4. (#4) Remove the `runtime.Caller` repo-layout fallback; dev builds resolve
   the plugin root through the same generic mechanism release builds use
   (resource root or launcher-provided environment), never a path literal
   naming this repository's layout (`ai-docs/manuals/shipped-surface-boundary.md`).
5. (#5) The guard compares realpaths on both sides. Rejected: splitting the
   entry script (more files for the same check).
6. (#6) Settled by Open Decision Queue item (525).
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

Last ID: (525)

(525) [open] [policy] How a hosted Pi lead restarts when default mode is impossible (its Pi has no JS SDK), including legacy records that read as `default`
- `261113-feat-ws-hosted-lead-bundled-plugin-only` (owner (392)=c) persists `default|isolated` with field default `default`, and a resumed session relaunches in the mode it was started in. Legacy records therefore read as `default`, and default mode refuses a standalone or wrapper Pi without the JS SDK, so those leads cannot restart after the upgrade. Any fix that starts them differently deviates from that decision.
- a: only when default mode is impossible (no JS SDK), start the lead in isolated mode with a visible `PluginWarning`; the stored record is unchanged and default stays default everywhere else.
- b: legacy records (no isolation field) read as `isolated`; new records keep the `default` default.
- c: keep refusing, with an actionable error telling the user to relaunch the session in isolated mode.
> a: the narrowest deviation, applies only where the recorded mode cannot run at all, and keeps the record's meaning.

## Constraints

- Overlap: ready `261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps`
  (in flight) edits `pi_lead.go`'s launch and fallback paths; #5 and #6 touch
  the same file. Route #6's fallback through the same `bundleErr` /
  `PluginWarning` branch that ticket uses (its Decision 6), and whichever
  lands second rebases.
- Overlap: ready `261113-feat-ws-dashboard-iframe-shell` (in flight) reworks
  dashboard hotkeys and focus routing; #1's target checks must survive in the
  shell document.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: Fix the eight Important findings

Apply Decisions 1-8.

Verification:
- #1: a `search-palette.test.cjs` case where Ctrl+K / Ctrl+Space from an
  `.xterm` helper textarea and from the composer are not taken by the palette,
  and are still taken from the page body.
- #2: a test that a CJK memo over 8000 bytes is refused by the client with a
  byte-count message and one at the limit is accepted by both client and
  server.
- #3: claude-mod (and wsflow mirror) and Pi tests where a second arrival
  lands before recv: no extra wake for drained mail.
- #4: `test_go_string_literals_downstream_neutral` passes; a dev build still
  finds the plugin root.
- #5: a regression test that runs the adapter through a symlinked directory
  and sees Pi start.
- #6: a runner test of the (525) outcome for a legacy lead record (no
  isolation field) on a Pi without the JS SDK.
- #7: a render test that `rsrc/*.md and internal/*.go` in a link field
  renders verbatim.
- #8: the four `internal/mcp` tests pass on macOS.
- `go test ./...` for the touched packages, the claude-mod / wsflow / Pi test
  suites, and the wsflow mirror drift tests pass.

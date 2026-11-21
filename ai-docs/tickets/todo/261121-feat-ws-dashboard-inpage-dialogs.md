---
title: "ws 대시보드 확인 대화상자와 초안 보호의 페이지 내 전환"
related:
  261117-research-werkschaft-tauri-shell-revisit: "Shell viability research and the later Tauri migration decision"
sage-review-design: blocked
sage-review-completeness: blocked
---

# Move dashboard confirmations and draft protection into the page

## Background

On 2026-11-21 the owner leaned toward returning Werkschaft to Tauri, primarily
for package size. The owner asked to first move dashboard confirmations out of
the native shell and reduce shell-owned UI, including DevTools handling. The
preparation benefits the shipped Electron app and the browser dashboard today.

This ticket covers the first three work units in the 2026-11-21 Werkschaft
Tauri return pre-study, limited to work that can ship on Electron. The pinned
Wry 0.57.2 macOS UI delegate omits the JavaScript confirm selector, for which
WebKit's documented default is cancellation. The ten confirmation gates below
therefore prevent their guarded actions on that WKWebView path. Source
inventory was checked at `1b8605ab4`.

## Decisions

- Reuse one in-page HTML `<dialog>` confirmation helper across the ten browser
  confirmation sites. Use it for the draft decision before a destructive
  iframe swap in Phase 2. Retire the Electron main-process Stay/Leave dialog
  through the approved close policy in (2020).
- Target **T0 (transparent, patch-safe)** for the presentation changes. The
  daemon serves its own dashboard bundle (`agents-plugin-tool/internal/wsdashboard/pages.go:27`,
  `agents-plugin-tool/internal/wsdashboard/page_routes.go:98-113`). Old and new
  shell versions can meet independently versioned daemon pages because
  `agents-plugin-tool/desktop-electron/daemon-controller.js:85-104` can keep a
  compatible running daemon. Keep existing action and auth contracts. Preserve
  behavior in both mixed-version directions through optional, feature-detected
  shell integration. The current login bridge is already optional in the page
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-actions.js:108-113`)
  and the server accepts the native and dashboard callers on the same link route
  (`agents-plugin-tool/internal/wsdashboard/localhost_auth.go:350-392`).
  Renderer-only changes leave legacy calls and persisted data readable and
  writable on reopen. Recheck the effective tier against the approved Phase 2
  and Phase 3 designs before implementation and release. In particular, the
  current draft writer serializes the whole record
  (`agents-plugin-tool/internal/wsdashboard/assets/rail.js:82-92`), so adding
  fields to that record alone gives an old writer a way to lose them. Any new
  persistence or shared setting needs an old/new read, write, and reopen trace
  with source citations. A T3 boundary needs a minor bump and explicit owner
  approval.

- (2020) Auto-persist and restore drafts, including pasted image blobs and
  unsent input, so operating-system window close needs no shell prompt, provided
  pasted images can be restored (see (2041)). Define recovery for failed writes
  and unacknowledged sends without duplicate delivery. Rejected: hold the native
  close request while the page asks the user to Stay or Leave.
- (2021) Keep the DevTools menu entry and shortcuts in release builds on all
  platforms. A future Tauri shell enables the `devtools` Cargo feature in release
  builds. Electron restricts release DevTools on no OS; Tauri's macOS private
  API blocks Mac App Store acceptance, while this product ships GitHub release
  zips ([Tauri debugging documentation](https://v2.tauri.app/develop/debug/)).
  Rejected: remove release DevTools only on macOS, or remove them on all
  platforms as originally announced.
- (2022) Use an in-page toast or status for daemon-replacement refusal while
  retaining the connection. Rejected: the Electron warning modal.
- (2023) Removing the native Copy Browser Login Link menu item counts as
  functionality removal; keep it outside this ticket's removal scope. Rejected:
  treat it as duplicate UI and remove it.
- (2024) Keep the browser `beforeunload` fallback for reloads and top-level
  navigation outside the iframe swap. Rejected: retire the fallback.
- (2025) Reuse the existing `rdeldlg` styling for the shared confirmation
  helper. Rejected: introduce a separate modal style.
- (2041) Persist pasted image blobs in browser IndexedDB keyed by the session
  draft, clearing them on send or discard. Keep the image leave guard until
  restore of held images is verified. Rejected: upload each paste to the daemon.
- (2046) Owner answered a on 2026-11-21: a send whose POST outcome is unknown
  at reopen stays visible in the composer marked delivery-unknown, with no
  automatic replay; the user resends or discards. This applies to normal sends
  and slash-command POSTs, and preserves later queued or unsent input. Client-only,
  T0. Rejected: a per-send idempotency key with daemon deduplication and automatic
  replay; mixed versions with an old daemon still duplicate and it adds an inbox
  API surface. The current draft writer retains the pre-send snapshot while an
  outbox entry is pending (`dashboard-lifecycle.js:128-142`); normal and command
  sends post separately (`dashboard-composer-delivery.js:259-276,306-327`,
  `dashboard-composer-pickers.js:178-215`), and the inbox returns an ID after
  sending (`inbox_routes.go:84-111,489-500,567-584`).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Electron remains the shipping shell. The Windows custom titlebar removal and
  the Tauri runtime, daemon port, updater, packaging, and platform qualification
  belong to later Tauri work. Obtain owner approval before removing any
  functionality.
- Compare before/after screenshots side by side for every UI change. Capture
  equivalent states and include the evidence with each phase result. Cover
  the browser dashboard and packaged Electron where the changed surface is
  available, including Windows-specific and macOS-specific shell entry points.
- Read and follow the matching `AGENTS.md` Implementation Conventions before
  editing. `agents-plugin-tool/` is covered by
  [shipped-surface-boundary.md](../../manuals/shipped-surface-boundary.md) and
  [performance.md](../../manuals/performance.md).
  `agents-plugin-tool/internal/` is also covered by
  [release-compatibility.md](../../manuals/release-compatibility.md). For any new
  or changed persistence, classify the data under the performance manual's
  Storage Types and account for file operations per affected action.

## Prior Decisions

- 261115-feat-werkschaft-electron-shell (2026-11-15, Decisions): "X-close shows the dashboard's close confirmation exactly as in Chrome" — bearing: constrains; (2020) approves the replacement close policy.
- 261115-feat-werkschaft-electron-daemon-override (2026-11-16, Decisions): "Replacement never stops, signals or restarts session holders" — bearing: constrains.
- 261117-research-werkschaft-tauri-shell-revisit (2026-11-17, Confirmed Decisions): "keep Electron for now" — bearing: supports.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Dashboard assets and `agents-plugin-tool/desktop-electron/main.js` across three phases. |
| scope.surface | cross-module | Dashboard frames, Electron main and preload integration, and browser draft storage interact. |
| scope.new_public_symbol | unknown | The helper and optional shell integration do not yet name their exported symbols. |
| scope.new_type_contract | unknown | The draft persistence and shell-to-page signal designs are not specified. |
| scope.test_surface | existing | `agents-plugin-tool/internal/wsdashboard/shell_frame_test.go`, `agents-plugin-tool/desktop-electron/test/main.test.js`, and `agents-plugin-tool/desktop-electron/test/smoke.cjs`. |
| complexity.reuse_points | confirmed | `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css:561-598` styles `rdeldlg`; `dashboard-lifecycle.js:259-284` has draft and cache guards. |
| complexity.side_effect_risk | high | Close, navigation, persistent drafts, sends, and daemon replacement feedback have cross-process effects. |
| risk.correctness | high | Stale decisions, failed writes, or unacknowledged sends can lose drafts or duplicate actions. |
| risk.fit | moderate | Shared dialogs must work in framed and top-level pages and preserve platform shell behavior. |
| risk.test | high | Mixed shell/daemon versions, async navigation races, and packaged platform entry points require broad coverage. |
| risk.security_or_contract | high | Login credentials, pinned-origin messaging, mixed-version persistence, and action contracts must remain safe. |

## Phases

### Phase 1: Replace browser confirmation calls with one in-page dialog

Replace all ten `window.confirm()` calls under `agents-plugin-tool/internal/wsdashboard/assets/`
with one asynchronous `<dialog>` helper usable on the dashboard and its framed
pages. Preserve the localized messages, dirty/running predicates, and actions
after consent. Cancel and Escape must leave the target intact. Only explicit
acceptance can act, and repeated activation or a stale completion must not act
twice or on a different target. Preserve keyboard operability and return focus
to the initiating control when it is still present.

| Verified site under `agents-plugin-tool/internal/wsdashboard/assets/` | Guarded action |
| --- | --- |
| `terminal.js:465` | Close a running terminal card via `closing(c)` |
| `terminal.js:811` | Close a running terminal tab via `closeTab(tid)` |
| `dashboard-git.js:187` | Stop a running header/git terminal |
| `dashboard-leads.js:931` | Stop a lead with background tasks |
| `dashboard-leads.js:941` | Restart a lead with background tasks |
| `dashboard-leads.js:946` | Hand off a lead with background tasks |
| `dashboard-leads.js:954` | Close a lead, including when it has no background tasks |
| `tracker-wiki.js:440` | Leave a dirty wiki editor via `wkLeaveOk()` |
| `tracker-forms.js:774` | Close the admin org-chart modal with staged changes |
| `tracker-view-helpers.js:191` | Delete a tracker label via `labelRemove` |

Every caller must consume the asynchronous decision before acting. In
particular, `cards.js:1482` uses `!== false` on the terminal card's synchronous
`closing` hook. Preserve that veto and the order of daemon confirmation and card
removal. Carry the decision through all four direct `wkLeaveOk()` consumers
(`tracker-wiki.js:442,457,523,524`). The org-chart continuation in
`tracker-mount.js:205,235` tests the result of `closeOrgEditor(false)` to open
another view. Keep that continuation behind confirmed closure. Preserve forced
org-chart cleanup and the prompt-free branches for exited terminals, clean
editors, and lead stop/restart/handoff with zero background tasks.

Check the accept, cancel, and keyboard paths at all ten sites, including
repeated activation while the dialog is open. For the wiki and org-chart,
verify that declining leaves the existing editor and draft intact. For terminal
closures and label deletion, verify that acceptance issues one action and
cancellation issues none. Inventory the assets again for remaining native
confirmation calls and capture the before/after comparisons.

### Phase 2: Protect unsaved drafts before iframe swaps and remove the native Stay/Leave dialog

Build on the Phase 1 helper. Resolve the active frame's draft decision before
changing or discarding it. The current guard attempts a draft save and then
detects pasted images without refs, failed draft writes, and unacknowledged
outbox entries (`dashboard-lifecycle.js:259-284` and `rail.js:647-657`). Keep
those cases protected. The current swap starts `about:blank` at
`dashboard-lifecycle.js:487-498` and uses its load to commit the destination at
`dashboard-lifecycle.js:411-438`. Replace that dependency on a native unload
decision. Preserve the safe cache path and keep the source frame, draft, and
history position when the user stays. Only the current accepted attempt may
switch frames.

Implement draft recovery under (2020), (2041), and (2046), retaining the browser
fallback under (2024). Recover unknown-outcome normal and slash-command POSTs
as delivery-unknown without automatic replay, preserving later queued or unsent
input. Prompt-free OS close applies only when the loaded renderer establishes
draft recovery. For an older renderer unable to establish recovery, including one
that vetoes unload while holding an unpersisted image, preserve the native
Stay/Leave path in `agents-plugin-tool/desktop-electron/main.js:287-304` and
Windows BaseWindow WebContents close mediation at `main.js:220-228`. Preserve
draft safety across supported old/new Electron and daemon versions; the current
renderer guard is in `dashboard-lifecycle.js:259-273`.

Verify rail selection and Back/Forward for clean and dirty frames, Stay and
Leave, rapid destination changes, delayed saves, and each existing at-risk
draft case. Test both accepted-but-reply-lost and lost-POST outcomes: reopening
shows delivery-unknown, creates no duplicate inbox entry, and keeps later input;
cover normal and slash-command POSTs. On macOS and Windows, check X-close and
reopen with a recovery-capable renderer (no prompt, draft recovered) and with a
legacy renderer (Stay/Leave still works). Check top-level reload/navigation
under the approved policy and before/after screenshots for the swap and each
affected close path. Update the affected existing dashboard and Electron
close/smoke coverage as part of implementation.

### Phase 3: Prune shell-owned UX after owner decisions

Use the release DevTools, daemon notice, and login-link decisions in
(2021)–(2023). Apply approved changes to the following current surfaces under
`agents-plugin-tool/desktop-electron/`.

- **Release DevTools.** `main.js:114-134,165,506` implements the View menu
  toggle, F12 and platform inspect accelerators, and the Windows titlebar view's
  shortcut. Exercise every packaged entry point covered by the owner decision.
- **Browser login link.** Keep the Connection menu action at
  `main.js:477-492,518` under (2023), including its machine-authenticated root
  fallback. Preserve existing link and clipboard credential boundaries and
  compatibility with the old optional login-copied hook.
- **Daemon replacement refusal.** `main.js:366-376` shows the synchronous
  warning for explicit replacement and logs automatic notices. Apply (2022)
  after acknowledgement. Preserve the current connection and refusal reason
  when replacement is unavailable, including when the dashboard reconnects
  or cannot render the notice. Test refusal while retaining the incumbent,
  successful replacement, and automatic startup/recovery. Keep the existing
  pinned-origin boundary if a shell-to-page signal is needed
  (`main.js:431-432`, `dashboard-preload.js:4-32`).

Compare before/after screenshots of the affected View and Connection menus,
Settings controls, and replacement feedback. Verify that release builds still
open DevTools through their menu and shortcuts, the Copy Browser Login Link menu
item still exists and mints usable login links, and refusal feedback reaches
users in mixed-version operation.

## Blocked (2026-11-21)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Windows packaged acceptance still requires the retired dialog | critical | autonomous |
| 2 | An accepted send and a lost POST look identical after reopen | important | missing |
| 3 | Gate native close retirement on the loaded renderer | important | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|

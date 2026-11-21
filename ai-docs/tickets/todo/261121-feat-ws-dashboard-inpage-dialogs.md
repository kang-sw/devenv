---
title: "ws 대시보드 확인 대화상자와 초안 보호의 페이지 내 전환"
related:
  261117-research-werkschaft-tauri-shell-revisit: "Shell viability research and the later Tauri migration decision"
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

## Constraints

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

## Open Decision Queue

Last ID: (2040)

(2020) [confirmed] [policy] Owner answer 2026-11-21: a, provided pasted images can be restored (they were deliberately guarded because pasted images are not kept: fda6c7a1e); see (2040). How should an operating-system window close handle unsaved dashboard drafts?

- a. Auto-persist and restore the draft, including pasted image blobs and unsent input, so closing the window needs no shell prompt. Define how input survives a failed write and how an unacknowledged send is recovered without duplicate delivery.
- b. Keep a shell close-requested check that asks the page to resolve in-page Stay/Leave for at-risk input before the shell closes. Keep the native close request pending while the user decides.

> Recommendation: a, once recovery is verified for all current guard cases. It gives browser and desktop users the same recoverable drafts and removes the shell's close-confirmation dependency.

(2021) [open] [announced] Remove the release-build DevTools menu entry and shortcuts, and keep development tooling. Owner direction on the whiteboard (1955) named DevTools among the shell clutter to clear.

(2022) [open] [technical] Use an in-page toast or status for daemon-replacement refusal over the Electron warning modal. The controller already keeps the connection, so the notice reports the outcome without gating it and can carry forward to Tauri.

(2023) [open] [policy] Does removing the native Copy Browser Login Link menu item count as removing functionality?

- a. Count removal of the native entry point as functionality removal and require explicit approval to delete it. It uses machine authentication and can copy a root link when the visible document is outside the pinned dashboard.
- b. Treat it as duplicate UI and approve removing it once the dashboard Settings action is shown to cover every owner-required copy path, including access from connection or recovery states if those are required.

> Recommendation: a. Both actions mint login links, and only the native action has the observed machine-authenticated root fallback.

(2024) [open] [technical] Keep the browser `beforeunload` fallback for reloads and top-level navigation outside the iframe swap over retiring it. It protects callers outside the swap path, and retirement requires a separate draft-recovery proof.

(2040) [open] [technical] Persist pasted image blobs browser-locally in IndexedDB, keyed by the session draft and cleared on send or discard, over uploading each paste to the daemon - keeps paste and restore off daemon file I/O (performance Rule 5) and works in Electron, browsers, WKWebView and WebView2; the image leave guard stays until a restore of held images is verified.

(2025) [open] [technical] Use the existing `rdeldlg` styling for the shared confirmation helper over introducing a separate modal style. The worktree-delete dialog already provides a labeled shell-independent modal and handles focus around rail redraws (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail-popovers.js:875-895`, `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail-popovers.js:1017-1034`, `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css:560-565`).

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
| `dashboard-leads.js:872` | Stop a lead with background tasks |
| `dashboard-leads.js:882` | Restart a lead with background tasks |
| `dashboard-leads.js:887` | Hand off a lead with background tasks |
| `dashboard-leads.js:892` | Close a lead, including when it has no background tasks |
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

After resolving (2020) and (2024), remove the native Stay/Leave dialog in
`agents-plugin-tool/desktop-electron/main.js:287-304` and update the surrounding
Windows close mediation at `main.js:220-228` as the approved policy requires.
Preserve draft safety for the supported mix of old/new Electron and daemon
versions, including an older renderer that still raises `beforeunload`.

Verify rail selection and Back/Forward for clean and dirty frames, Stay and
Leave, rapid destination changes, delayed saves, and each existing at-risk
draft case. Check Electron X-close and reopen, top-level reload/navigation
under the approved policy, and before/after screenshots for the swap and each
affected close path. Update the affected existing dashboard and Electron
close/smoke coverage as part of implementation.

### Phase 3: Prune shell-owned UX after owner decisions

Use the release DevTools, daemon notice, and login-link decisions in
(2021)–(2023). Apply approved changes to the following current surfaces under
`agents-plugin-tool/desktop-electron/`.

- **Release DevTools.** `main.js:114-134,165,506` implements the View menu
  toggle, F12 and platform inspect accelerators, and the Windows titlebar view's
  shortcut. Exercise every packaged entry point covered by the owner decision.
- **Browser login link.** `main.js:477-492,518` provides the Connection menu
  action. The Settings action is `dashboard.tmpl:141` and
  `dashboard-actions.js:98-113` under the dashboard assets. Establish parity
  for the paths the owner requires before treating the native entry as
  duplicate. Preserve existing link and clipboard credential boundaries and
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
Settings controls, and replacement feedback. Verify that the approved copy
paths still mint usable login links and that refusal feedback reaches users
in mixed-version operation.

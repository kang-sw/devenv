---
title: Settings in the usage strip and a session-first dashboard root
related:
  261115-feat-ws-whiteboard-topic-state-badge: coordination; active badge lane owns nearby dashboard assets and lands first
  261114-feat-ws-dashboard-worktree-view: future separate root destination; not implemented here
  261114-epic-ws-dashboard-file-explorer: prerequisite of the future worktree-only view, not of this interim root
  261113-refactor-ws-dashboard-drop-lan-serving: authoritative LAN removal; interim relocation never revives removed LAN
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 30b1e13958d8ecb3
sage-review-completeness-reviewed: 30b1e13958d8ecb3
completed: 2026-11-15
---

# Settings in the usage strip and a session-first dashboard root

## Background

Settings currently occupies the session screen's top-right hamburger drawer. The bottom-left rail footer already holds the agent usage gutters, but their large typography consumes more vertical space than the owner wants. Separately, `/` opens a distinct dashboard home/index rather than the ordinary session-screen layout.

MAIN relayed the owner's settings relocation request on 2026-11-15 at 01:57:09 UTC, authorizing this document through design review and readiness, not implementation. MAIN then relayed owner decision (879)=c at 02:16:01 UTC: remove the home screen now, retain `/` as the session screen with no selected session and an empty main panel, and move the home-only LAN access-token box and version label into Settings. Badge work and asset-split 793 retain their separate approved lanes and priority.

## Decisions

- Move the existing session Settings card into the bottom-left agent usage strip beneath the session list. The leftmost control is a gear button opening the same settings, including layout, input mode, theme and language; usage gutters align to the right. Remove the old hamburger Settings entry as part of this move, without deleting any settings functionality or unrelated menu entry. Rejected: leaving duplicate old/new entries or removing settings behavior.
- Substantially reduce usage-meter typography so each meter fits within approximately one text-line height. The owner's `1.2eh` wording describes that visual target; it is not a required CSS unit. Preserve all usage values, availability states, periods and interaction semantics.
- Owner (879)=c removes the separate home/index presentation. Visiting `/` shows the ordinary session-screen layout with the session rail, no selected session and an empty main panel. Do not silently select a session, send the user to a specific session, or treat an empty selection as a missing-session error.
- Retain every existing URL and entry consumer, serving the empty-selection session layout at `/` or redirecting to that same outcome. Preserve PWA launch, desktop attachment, shell framing and rail navigation; preserve LAN entry behavior only while LAN serving still exists under the cross-ticket rule below. Existing `/s/<reply-id>` links continue to select their sessions; malformed/missing/temporarily unavailable session behavior remains distinct from no selection.
- Move the version label into the relocated Settings card, preserving its existing value/source. Move home-only LAN access-token information there only while LAN serving still exists, preserving its local-viewer-only disclosure rule. MAIN relayed this as an owner-seen announced default alongside (879), with the interim cross-ticket interpretation confirmed on 2026-11-15 at 02:34:12 UTC. Rejected: deleting still-existing functionality merely with the home page, exposing a token to remote viewers, or reviving LAN after its approved removal.
- Cross-ticket landing rule, MAIN technical call at 02:34:12 UTC: (879) does not supersede `261113-refactor-ws-dashboard-drop-lan-serving`; that owner-approved removal remains authoritative. If removal lands first, Settings shows the version only and LAN is never reintroduced. If this ticket lands first, move the existing LAN box as-is with authentication/URL semantics unchanged; the removal ticket deletes it later. Neither ticket is a hard prerequisite of the other.
- The later worktree-only root view belongs to `261114-feat-ws-dashboard-worktree-view` after its file-explorer prerequisites. This ticket implements only the interim empty-selection session screen; it neither delivers nor blocks on that future view.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` for `agents-plugin-tool/internal/` changes. Read `ai-docs/manuals/ws-mcp.md` only if MCP paths actually need touching.
- Compatibility: T0 presentation/assets work consuming existing session, usage, version and LAN data. Owner-approved home removal is a UI change, not permission to remove URLs, alter API/protocol semantics, change stored data, or change authentication. A non-additive requirement stops for MAIN disposition.
- Preserve `/health` identity and the security boundaries of the landed base. While LAN serving still exists, preserve token generation, file storage, query-to-HttpOnly-cookie exchange, authentication and local/remote access boundaries; render sensitive LAN data only for authorized local viewers, outside shared SSE regions. After the authoritative removal lands, do not restore its token, cookie, access branch, data plumbing or UI. No new token-rotation action or endpoint is requested.
- Keep settings keyboard access, localized labels, focus handling, dismissal, and existing layout/input/theme/language persistence. The relocated card remains reachable with no selected session, no sessions, absent usage statistics and on narrow/overlay rail layouts.
- Preserve usage values and weekly/five-hour/expired/unavailable semantics; this is layout and typography, not a usage-accounting change. Do not hide overflow by clipping essential labels, values or settings controls.
- Update obsolete home/index-facing text, including unauthorized LAN guidance while that feature exists, to the actual Settings location without weakening authorization. Do not remove unrelated session-list or rail functionality merely because the old home presentation disappears.
- Implementation integration is lead-owned and follows the badge lane's approved sequencing. Do not import its unmerged implementation, alter asset-split comparison pins/cache artifacts, or repair unrelated rail draft-restore fixtures.
- Verification uses cached dependencies and fresh isolated fixtures with temporary WS cache/config. No provider calls, downloads, shared daemon or port 29417, owner installation, or shared-daemon build/restart/swap/smoke. MAIN owns develop integration and runtime changes. Go tests use their own timeout, never an outer kill. Retain actual exits and owned-process cleanup evidence.
- No visual mockup was approved for this ticket. Capture actual results for geometry/usability verification; the badge ticket's visual reference does not prescribe this settings/root layout.

## Grounding

Evidence inspected at approved reference `8140c823615e3735999cec29c0c325614bc7f771`; source is authoritative, line coordinates are evidence rather than edit instructions.

- VERIFIED: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` defines shared `gearitems` (layout/input only with an adapter), the session drawer's Settings card and the footer usage region (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L115-L150`, `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L361-L363`). The old index at lines 313–335 renders its title/version, LAN box and session-list region; the existing rail remains a separate render. The shell owns the visible rail on framed pages and loads UsageScript itself (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L61-L89`); the usage controller replaces the usage button's children when redrawing (`agents-plugin-tool/internal/wsdashboard/assets/usage.js#L41-L74`).
- VERIFIED: `pages.go` separates `sessionPageData`, `shellPageData` and `indexPageData` (lines 582–635). Index LAN data is deliberately outside shared regions; `handleIndex` (1075–1101) serves the root shell then the framed index. Empty shell RID is already representable; the new main-panel behavior is not implemented yet.
- VERIFIED by direct reference-tree inspection, corroborating the bounded research: `agents-plugin-tool/internal/wsdashboard/access.go#L70-L126` preserves token generation/storage, query-to-HttpOnly-cookie authentication and localhost index guidance; `agents-plugin-tool/internal/wsdashboard/access.go#L198-L216` gates LAN disclosure on LAN mode and localViewer; `agents-plugin-tool/internal/wsdashboard/access_test.go#L423-L452` covers disclosure. Session page data lacks the index's LAN field and rendered version presentation, but already embeds SessionView.Version (`agents-plugin-tool/internal/wsdashboard/pages.go#L582-L635`, `agents-plugin-tool/internal/wsdashboard/pageview.go#L71-L75`); both views receive the endpoint version through the page reader (`agents-plugin-tool/internal/wsdashboard/pages.go#L900`, `agents-plugin-tool/internal/wsdashboard/pages.go#L1049-L1063`). Relocation must preserve the security boundary across the new owning surface.
- VERIFIED by direct reference-tree inspection, corroborating the bounded research: root consumers include `agents-plugin-tool/internal/wsdashboard/assets/pwa/manifest.webmanifest#L4-L6`, `agents-plugin-tool/internal/wsdashboard/assets/shell.js#L5-L13`, the rail link (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L96`), LAN URLs (`agents-plugin-tool/internal/wsdashboard/access.go#L208-L216`) and desktop root attachment (`agents-plugin-tool/desktop/src/endpoint.rs#L66`). Existing `agents-plugin-tool/internal/wsdashboard/shell_test.go`, `shell_frame_test.go`, `pages_test.go`, `access_test.go`, `i18n_test.go`, `pwa_test.go` and `agents-plugin-tool/desktop/src/navigation.rs` provide affected regression surfaces; disclosure assertions explicitly pin the old index ownership (`agents-plugin-tool/internal/wsdashboard/access_test.go#L423-L452`).

## Prior Decisions

- 261114-feat-ws-dashboard-usage-footer-meter (2026-11-15, commit 401d2fbf): "Meter typography, spacing, signs and stale treatment match section A." — bearing: constrains
- 261113-feat-ws-dashboard-iframe-shell (2026-11-14, commit 3310356a): "Tests that read index content now ask for /?frame=1; the LAN test also pins that the shell at / carries no token." — bearing: constrains
- ef612684 (2026-11-13, commit): "Manifest: id \"/\", start_url \"/\", scope \"/\" (session pages /s/<id> open inside the app window), display standalone" — bearing: constrains
- 261113-refactor-ws-dashboard-drop-lan-serving (2026-11-13, Decisions): "Only boundaries that exist solely for LAN viewers are removed (the LAN token, its cookie, and the LAN branch of the access checks)." — bearing: constrains; MAIN resolved the apparent conflict on 2026-11-15 at 02:34:12 UTC: relocation is interim, removal remains authoritative and either landing order must not revive LAN.
- 261106-feat-ws-dashboard-browser-interaction (2026-11-07, commit 4f447bc6): "The token file is read on every check so deleting it rotates the token at once, not only at the next daemon start; a mutex serializes mint." — bearing: constrains
- b94f504f (2026-11-08, commit): "Cookie plus server-rendered root attribute, mirroring the language choice, so there is no flash of the wrong theme on load" — bearing: constrains
- 261107-feat-ws-dashboard-i18n (2026-11-07, commit 30485943): "Language is resolved server-side (cookie, then Accept-Language, then English) and carried in views (Lang/LangPref)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED: agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl, pages.go, access.go and affected tests; usage styling/controller are assets/usage.css and assets/usage.js. Evidence uses approved 8140c823615e3735999cec29c0c325614bc7f771 via git show, not sparse filesystem absence. |
| scope.surface | public-interface | VERIFIED: visible root presentation and settings entry change; existing /, /s/<reply-id>, PWA and LAN entry URLs remain contractual (Decisions; pages.go#L1075-L1101; assets/pwa/manifest.webmanifest#L4-L6). |
| scope.new_public_symbol | no | INFERRED from Phases/Constraints: reuse existing settings/data and preserve URLs/APIs; no new exported symbol or endpoint requested. |
| scope.new_type_contract | no | INFERRED: no new wire/API/store contract requested; internal page-data plumbing reuses version and relocates existing LAN data only while that feature remains (pages.go#L582-L635; pageview.go#L71-L75; confirmed cross-ticket landing rule). |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wsdashboard/{shell_test.go,shell_frame_test.go,pages_test.go,access_test.go,i18n_test.go,pwa_test.go,usage.test.js} and agents-plugin-tool/desktop/src/{endpoint.rs,navigation.rs} exist at the approved reference; no new test file mandated. |
| complexity.reuse_points | confirmed | VERIFIED: read gearitems/drcards, shell rail ownership, lanAccessFor and usage drawFooter (assets/dashboard.tmpl#L61-L150; access.go#L198-L216; assets/usage.js#L41-L74). |
| complexity.side_effect_risk | high | INFERRED: moving frame-owned settings into shell-owned rail intersects shared-region replacement, usage redraw and persisted layout/theme/input behavior (assets/dashboard.tmpl#L61-L150; assets/usage.js#L41-L74). |
| risk.correctness | high | INFERRED: no selection must remain distinct from invalid/unavailable selection across root/framed navigation while retaining all settings and usage states (Phase 2; pages.go#L1075-L1114). |
| risk.fit | moderate | INFERRED: reuse existing drawer controls and shell/rail rather than a competing settings system; current adapter-gated controls and separate shell/frame documents require deliberate integration (assets/dashboard.tmpl#L61-L150). |
| risk.test | high | INFERRED: existing access tests pin old ownership, not relocated root Settings; responsive geometry, focus and empty states require new assertions and captures, not source inspection alone (access_test.go#L423-L452; both phase verification lists). |
| risk.security_or_contract | high | INFERRED: while LAN exists, its tokens must remain local-only and outside shared SSE regions; after removal, none may be revived. Root and PWA/desktop entry contracts stay working in either order (pages.go#L622-L627; access.go#L97-L126, #L198-L216; access_test.go#L423-L452; confirmed cross-ticket landing rule). |

## Phases

### Phase 1: Move Settings into the compact agent usage strip

Move the existing settings functionality to the footer's leftmost gear, align the existing usage gutters right and reduce each meter to roughly a text-line height. Remove the session hamburger's old Settings entry, preserving other entries and all settings values, availability and persistence. This phase establishes the settings destination consumed by Phase 2.

Verification:

- Cover layout, input mode, theme and language controls, persistence and reload; keyboard activation, focus return and dismissal; localized accessible labels; no duplicate Settings entry or lost unrelated hamburger action.
- Cover normal and absent/expired usage states without changing values or accounting; preserve existing usage interactions. Check light/dark and Korean/English at desktop and narrow rail widths, including overlay/pinned layouts, with footer geometry and screenshots proving left gear, right gutters and approximately one-line meters without clipped essential content.
- Run focused affected settings/usage/layout/i18n tests and retained neighboring rail/session checks. Preserve complete stdout/stderr, actual exits, omissions and owned fixture/process cleanup. Obtain independent implementation review.

### Result (0bc7fcf24) - 2026-11-15

- Moved Settings to the leftmost rail-footer gear; retained unrelated drawer cards, all preferences and the usage popover. Compact inline meters wrap without clipping essential values. The shell owns one static dialog outside shared regions; usage redraws cannot remove its sibling gear.
- The single combined Lite review found one Important storage-denial regression. Fixed in `820c559f8`: normalized in-memory choices reach cached/reloaded frames through the existing origin/source-checked UI transport even when persistence fails. No daemon API, protocol or store semantics changed.
- Verification: nine focused ownership/drawer/page Go tests and nine usage Node tests passed; final isolated Chromium checks passed keyboard/focus/dismissal, preferences, cookies/reload, hidden drafts, narrow geometry, denied/full storage and actual send-key behavior. Localization tests passed. Minor remaining coverage is tracked in `261115-chore-settings-hidden-theme-and-rail-swap-tests`.
- Newest authority: MAIN033257 and owner899/031755 replace the older screenshot/matrix/whole-suite ceremony; MAIN034301/034648/035251 accepts the existing base without sync or repeated acceptance. No screenshots, whole suites, providers/downloads or shared-runtime operations were performed. Owner post-merge on-screen feedback is visual acceptance.

### Phase 2: Replace the home with an empty-selection session screen

After Phase 1's settings destination exists, remove the distinct home/index presentation. Preserve `/` and its consumers, rendering the session-screen layout with rail, no selected session and an empty main panel. Move the version into Settings. Relocate LAN access-token information only if LAN serving remains in the landed base, preserving local-only disclosure and existing authentication semantics; otherwise show the version only without recreating LAN. Update obsolete home-directed guidance and tests. Do not implement the future worktree-only view.

Verification:

- Cover `/` and framed root navigation with existing sessions and with none: session rail remains usable, no arbitrary selected session or finding/error page appears, main panel is empty, and Settings is reachable. Navigating to/from `/s/<reply-id>` preserves correct session selection and the root's empty state; existing unknown-session behavior remains independent.
- Exercise existing PWA start/scope, desktop origin navigation, rail destination and shell framing; exercise LAN entry URLs only while LAN serving remains. Preserve the URLs and authorized navigation applicable to the landed base, never reintroducing removed LAN behavior. Update presentation assertions while retaining security and session-route coverage.
- Cover both cross-ticket landing orders: with LAN still present, test local LAN-on/off Settings, remote viewers and unauthorized requests; token information stays local-only/outside shared stream regions, authentication stays unchanged, and guidance names Settings. With LAN removal already landed, Settings has the version but no LAN block, token/cookie/access branch or revived entry behavior. The later removal must be able to delete an interim relocated LAN block without removing Settings/version. In both cases `/health` identity and the displayed version's existing value remain unchanged.
- Capture desktop/narrow light/dark root-with-no-selection and settings results, including empty usage/session lists. Run affected shell, pages/pageview, access, i18n, PWA and relevant desktop endpoint/navigation tests; retain actual commands, full outputs, exits, explicit omissions and owned-process cleanup. Obtain independent implementation review before recording the phase result or handing the branch to MAIN.

### Result (dfe650319) - 2026-11-15

- `/` now opens the ordinary shell with no selected session and an empty main panel, retaining root/session URLs, rail navigation, framing and PWA entry. Existing missing, malformed and temporarily unavailable session handling remains distinct. The version in Settings uses the endpoint's existing value.
- LAN serving exists at base `02a2e5722`, so its optional block moved into local-only shell Settings, outside every shared region. Local framed pages and all streams carry no token; remote viewers and unauthorized requests disclose none. Token generation/storage, query-to-HttpOnly-cookie exchange, authentication and `/health` identity are unchanged. Authoritative LAN removal remains separate and must remove only this optional LAN block, not Settings/version.
- Verification: the relevant route/security/framing/PWA/ownership batch passed 24 checks; the final nine-test batch passed Settings Chromium, shell message/navigation/stream ownership, PWA links and finding/unavailable regressions (`go test ./internal/wsdashboard -run <focused selectors> -count=1 -v -timeout=90s`). The browser also covers no-session/no-usage Settings reachability. An initial narrow-root collapse was fixed; a separate Playwright opaque-origin blocker error was repaired using the established fixture pattern, without suppressing page errors. All final commands exited 0; complete outputs remain outside the checkout under `/tmp/ws-settings-*`.
- One independent combined Lite pass completed; its Important finding was fixed in `820c559f8`, with no re-review under Lite. The Minor coverage follow-up is named above. Per the latest authority recorded in Phase 1, omitted visual matrices/screenshots, unchanged desktop Rust tests, whole suites, alternate already-removed-LAN execution and runtime install/build/restart/swap. Fixtures used temporary stores/config/HOME and cached dependencies; test-owned daemons and Chromium were closed. No sync, rebase or other unmerged lane was imported.

---
title: Dashboard opens image paths outside the project root for a loopback viewer
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: abfb51cb5ac33e2a
sage-review-completeness-reviewed: abfb51cb5ac33e2a
completed: 2026-11-09
---

# Dashboard opens image paths outside the project root for a loopback viewer

## Background

On the served dashboard page, a file path in a lead reply becomes an action
link for the link menu (`servedTargets` / `FileActions` in
`agents-plugin-tool/internal/wsdashboard/file_route.go`). A path inside the
session's project root gets an `Href` to the file view, so the menu offers
"open in new tab" and, for a loopback viewer, "open in system browser". A
path outside the root gets only `Abs`, so the menu offers "copy absolute
path" and nothing else. The file view and the image route both refuse
anything outside the root by design (`relativeInside`, `containedImage`).

Agents often write screenshots and comparison images outside the root (for
example `D:/Temp/drawing-layout-p1/.../comparison-1001.png`). The user copies
each path and opens it by hand. User decision on 2026-11-09 (whiteboard
(04) a, with a note): let a loopback viewer open such images straight from
the dashboard, in the browser, and also offer opening them in the OS's
native (default) app. LAN token viewers keep copy-only for paths outside
the root.

## Decisions

- Scope is image files only, identified by the image route's extension set
  (`imageTypes` in `image_route.go`). A non-image file outside the root
  stays copy-only.
  Rejected: serving any out-of-root file to a loopback viewer, which is a
  far wider read surface than the request needs.
- Out-of-root images are served only to a loopback viewer (`localViewer`).
  A LAN viewer, token or not, gets the route's ordinary 404, so the route
  says nothing about which paths exist.
  Rejected: OS-native open only (whiteboard (04) b as the sole mechanism).
  It runs a host command for every view and is unusable from the browser
  tab the user is already in.
- An out-of-root image keeps the image route's CSP
  (`imageContentSecurityPolicy`), so an SVG never runs script in the
  daemon's origin.
- Link and image markup stays viewer-independent. An out-of-root image link
  always carries its route `Href` plus a marker attribute; the page's JS
  offers the open items for such a link only when the page is marked
  loopback (`data-sysopen`); the server enforces loopback on its own.
  Rejected: rendering per viewer, because conversation entries render once
  through the shared entry cache (`renderEntries` / `entryCache` in
  `pages.go`) for every viewer.
- An out-of-root markdown image renders inline as an `<img>` on the route,
  and the page's JS swaps an image that fails to load (a LAN viewer, a
  missing file) for its path link.
  Rejected: leaving out-of-root images as path text; inline display was
  part of the chosen option, and the swap keeps a LAN viewer from seeing a
  broken image.
- "Open in native app" applies to every image link a loopback viewer sees,
  inside or outside the root, image extensions only. Non-image files get no
  native-open item.
- Native open extends the existing system-open route (`sysopen_route.go`,
  POST /s/<reply-id>/open) with a native-app target that names an image
  path, keeping its loopback, same-Origin, and no-shell rules, and runs the
  OS open command on the resolved absolute file path (Windows rundll32
  FileProtocolHandler, macOS `open`, Linux `xdg-open`). An absolute path
  argument never reads as an option.
  Rejected: a new route; the existing one already carries every guard a
  host-command route needs.
- Containment for an out-of-root image: the path must be absolute (a
  relative path still resolves against the root, as today); Windows UNC and
  device paths (`\\server\...`, `\\?\...`, `\\.\...`) are refused before the
  filesystem is touched; the requested name and the file it resolves to
  after symlinks both carry an image extension; the resolved file is a
  regular file not on a UNC/device path.
  Rejected: refusing symlinks outright; an image extension on both ends is
  the property that matters, and is the rule the in-root image route
  already applies.
- Verification: Go tests cover the route (a loopback viewer is served an
  out-of-root image; a LAN viewer and a non-image path get 404; UNC/device
  paths are refused; the CSP header is set) and the system-open native
  target's validation. The JS menu items and the image swap are checked by
  hand, since the dashboard has no JS test harness (an open follow-up from
  the 0.48.11 review).
- Design-review refinements (2026-11-09, accepted by the lead):
  - The widening lives in served-only functions. `wsdashboard.ImageSrc`,
    `ImageDataURL`, the `DocImageSrc` -> `ImageSrc` fallback in
    `wswhiteboard/markdown.go`, and the shared `FileActions` stay
    root-only. Lead replies (`servedTargets`) and the served board's item
    fields (`servedViewInput` in `mcp/dashboard_page.go`) get the
    out-of-root variants; topic bodies keep the root-only functions for
    both images and links, so `doc-frame.js` needs no marker forwarding. The
    markdown file view (`fileViewTargets`, which today builds on
    `servedTargets`) also stays on the root-only functions: it has no link
    menu and its CSP runs no script, so it could not swap a broken image.
    Tests pin that a topic body and a file-view markdown page with an
    out-of-root image still render path text.
  - Out-of-root responses refuse, with the route's uniform 404, a request
    whose `Sec-Fetch-Site` is present and not `same-origin`, and carry
    `Cross-Origin-Resource-Policy: same-origin`, so another site open in
    the same browser cannot probe the filesystem through `<img>`
    load/error. In-root responses are unchanged, since a topic frame's
    opaque-origin request falls back to the in-root route. Route tests
    cover both.
  - The link menu shows no "open in system browser" item for a marked
    out-of-root image link; `openTarget`'s browser target is not widened
    to the image route. "Open in new tab" and "open in native app" are the
    open items for it.
  - Both link click handlers (plain and Cmd/Ctrl+click) key on the marker
    plus `data-sysopen`: on a LAN page a marked link behaves as an
    out-of-root path does today (the menu with copy items only), keeping
    the 261107-feat-ws-dashboard-link-actions rule that a modified click
    on an out-of-root path opens the menu.
  - Only a path written absolute gets the marker and route `Href`. The
    decision is made on the original path, not on `FileActions`' resolved
    `Abs`, so a relative `../x.png` that leaves the root stays copy-only.
    The native-open target carries the absolute path (`data-abs`).
  - The UNC/device refusal is a pure string check over both slash
    directions (`\\`, `//`, `\\?\`, `//?/`, `\\.\`, `//./`), so Linux tests
    exercise it.
  - The image swap uses a capture-phase document `error` listener plus a
    sweep for `complete && !naturalWidth` after each render (the page CSP
    forbids inline `onerror`; entries are re-rendered on update). The
    `<img>` carries its absolute path for the replacement copy-only link.
  - Supersedes 090f4d00 ("out-of-root paths return \"\" from ImageSrc and
    stay path text, so the page never shows a broken image") for served
    lead replies and board item fields only; the swap above keeps its
    no-broken-image intent.

## Constraints

- Out of scope: topic bodies' sandboxed-frame `data:` inlining
  (`ImageDataURL`) and pages written to disk keep their current root-only
  behavior. The loopback distinction exists only on served pages, and a
  topic body's frame has no viewer context.
- Read `ai-docs/manuals/shipped-surface-boundary.md` (all of
  `agents-plugin-tool/`) and `ai-docs/manuals/ws-mcp.md`
  (`internal/mcp/` neighbours; its dashboard and link-menu prose is the user
  doc to update) before editing.
- New UI strings need every i18n table (`TestOtherTablesHoldEveryEnglishKey`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261107-feat-ws-dashboard-link-actions (2026-11-07, Decisions): "System browser open: a daemon endpoint runs the host OS open command... shows only when the page itself was opened from the host (loopback); its targets are limited to http(s) URLs and the daemon's file view URLs" — bearing: constrains
- 0336011f (2026-11-07, commit): "Non-local viewers get 404 from the open route (not 403) so the route looks absent to a LAN token holder and TestLANModeGuardsEveryRoute stays uniform" — bearing: constrains
- 261107-feat-ws-dashboard-link-actions (2026-11-07, Result 264ec72cd): "System open. POST /s/<rid>/open {target}. Gates, in order: POST, localViewer (404 otherwise), sameOrigin (403), a known session." — bearing: supports
- 4f447bc6 (2026-11-07, commit): "Image containment: a lexical check first, so a path outside the root (a UNC path on Windows would reach the network) is refused before any filesystem call; then EvalSymlinks on root and file" — bearing: constrains
- 261107-feat-ws-dashboard-link-actions (2026-11-07, Result 264ec72cd): "Page script. A delegated capture click on a[data-wl] opens a fixed menu. Items: open in new tab; open in system browser (data-sysopen only); a file link copies its relative path and absolute path" — bearing: supports
- d60d331a (2026-11-08, commit): "Images inside a marked link (data-wl: the link menu) or an in-page anchor keep that link's behaviour; any other link around an image opens the viewer" — bearing: constrains
- 261107-feat-ws-whiteboard-now-markdown (2026-11-07, Decisions): "href and src in reply HTML go through resolveTarget like markdown links and images: network images refused, file paths mapped to the image route or the file view" — bearing: constrains
- 30925622 (2026-11-07, commit): "Browser behavior (menu, Cmd/Ctrl+click, LAN copy) is not covered by an automated test; it needs a manual check." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/file_route.go, image_route.go, sysopen_route.go, pages.go, access.go, assets/dashboard.js, wsi18n tables, ai-docs/manuals/ws-mcp.md; marker markup is built in agents-plugin-tool/internal/wswhiteboard/markdown.go#L636-L645 |
| scope.surface | cross-module | wsdashboard routes plus wswhiteboard link markup via wswhiteboard.FileAction at agents-plugin-tool/internal/wswhiteboard/view.go#L95-L105; POST /s/<id>/open request contract extended |
| scope.new_public_symbol | yes | marker for an out-of-root image link likely needs a new exported wswhiteboard.FileAction field; exact name not fixed by the ticket |
| scope.new_type_contract | yes | system-open request body gains a native-app image-path target beside target in sysopen_route.go#L42-L44 |
| scope.test_surface | existing | wsdashboard/image_route_test.go, sysopen_route_test.go, file_route_test.go, wsi18n/keys_test.go; JS has no harness |
| complexity.reuse_points | confirmed | imageTypes, imageContentSecurityPolicy, localViewer, systemOpen, data-sysopen page flag all read in tree |
| complexity.side_effect_risk | high | new loopback-only read of arbitrary absolute image paths and a host OS command on a local file path |
| risk.correctness | moderate | containment rules for absolute, UNC/device, and symlinked paths differ per OS and must stay 404-uniform |
| risk.fit | moderate | out-of-root image Href must stay viewer-independent through the shared entryCache while the server gates per viewer |
| risk.test | moderate | Windows UNC/device and rundll32 paths cannot run on the Linux host; JS menu and image swap are hand-checked only |
| risk.security_or_contract | high | widens the daemon's read surface beyond the project root and extends the host-command route's accepted targets |

## Phases

### Phase 1: Open out-of-root images from the link menu

Implement the decisions above: out-of-root image serving for a loopback
viewer, the link menu's "open in new tab" and "open in native app" items for
image links, inline out-of-root images with a load-failure fallback, the
system-open native target, tests, and the manual update. No "open in system
browser" item for a marked out-of-root image link. The native-target field
name and the marker attribute name are the implementer's choice within the
guards above. Acceptance: the Go tests named in the Decisions' Verification
and refinement bullets pass (route serving and refusals, CORP and
`Sec-Fetch-Site`, UNC/device string check, native-target validation, topic
body and file view still root-only, i18n tables), plus a by-hand check of
the menu items and the image swap.

### Result (2c1ae57e3) - 2026-11-09

Landed as specified, plus one review fix (8295d5c83).

- Hook: `OutsideImage func(path) FileAction` on `wswhiteboard.View`,
  `ViewInput`, and `TextTargets`; `FileAction` gained `Image` (`data-img`)
  and `Outside` (`data-out`). `fileTarget` consults it only after `ImageSrc`
  and `FileActions` come back empty, never while a topic body renders (new
  `pageData.inDoc`, framed or not), and only for an Href that passes
  `daemonPath`. An out-of-root image renders as
  `<img src=route data-out data-abs>`, and a link to it renders as an action
  link on the route marked `data-img data-out`. The reply sanitizer gets the
  same attributes from `imageAttrs`/`actionAttrs`, and a raw `data-out` in
  reply HTML is dropped. `ImageSrc`, `ImageDataURL`, the `DocImageSrc`
  fallback, and the file view stay root-only. `FileActions` now marks
  in-root image links `Image`.
- Wiring: `servedTargets` (lead replies) and `servedViewInput` (board item
  fields) take `wsdashboard.OutsideImage`. `fileViewTargets` does not.
- Route: `handleImage` falls back to `outsideImage` only for `localViewer`
  and when `Sec-Fetch-Site` is absent or `same-origin`. Such a response
  carries `Cross-Origin-Resource-Policy: same-origin`. Containment:
  - `absoluteImageName` requires a path written absolute, with an image
    extension and no NUL. `networkPath` is a pure string check that refuses
    two leading separators in either direction and the `\??\` prefix.
    `driveOrRootPath` accepts only a drive-letter volume on Windows.
  - The path must be lexically outside the root.
  - `resolvedImage` runs EvalSymlinks; the resolved file must have an image
    extension, must not be a UNC/device path, and must be a regular file.
- System open: `{native: <abs path>}` beside `{target}`, exactly one per
  request. It is validated by the same name and resolution checks, and
  `systemOpen` runs on the resolved path. The 500 text is now generic.
- JS (`dashboard.js`): for a `data-out` link the menu offers "open in new
  tab" only on a `data-sysopen` page, and never offers "open in system
  browser". "Open in native app" appears for any `data-img` file link on a
  `data-sysopen` page. Both click handlers treat a `data-out` link on a LAN
  page as having no href. Image swap: a capture `error` listener, plus a
  `decode()`-based sweep at init and after each region swap. `doc-frame.js`
  forwards `data-img`. New i18n keys `js.menu.native` and
  `js.toast.openednative` are in all 11 tables.
- Manual: `ws-mcp.md` link-menu, image-route, and system-open prose updated.
- Verification:
  - `go test ./internal/wsdashboard ./internal/wswhiteboard ./internal/wsi18n`
    passes. `go test -timeout 25m ./internal/mcp` passes (468 s; a default-timeout run timed out under load in an unrelated ticket-index test).
  - `GOOS=windows go vet` and test compile of wsdashboard pass, as does a
    `GOOS=darwin` build.
  - New tests:
    - `TestImageRouteServesOutsideImagesToLoopbackViewersOnly`
    - `TestNetworkPathRefusesUNCAndDevicePaths`
    - `TestDriveOrRootPathRefusesNamespaceVolumes` (its cases run on Windows
      only)
    - `TestOutsideImageMapsAbsoluteImagesOutsideTheRoot`
    - `TestServedTargetsOutsideImages`
    - `TestSystemOpenNativeTarget`
    - `TestOutsideImageServesItemsNotTopicBodies`: topic bodies stay path
      text
    - `TestRenderTextOutsideImage`: covers the sanitizer
  - Extended tests: `TestFileActionsMapsFileTargets` (image mark),
    `TestFileViewKinds` (the file view keeps an out-of-root image as path
    text), and `TestDashboardReadPageServesTheBoard` (the item field now uses
    the route).
  - By-hand check, done in headless Chromium over CDP against a fixture
    daemon, as a loopback viewer and as a LAN-token viewer:
    - Loopback: the out-of-root image loads and the missing one is swapped.
    - Loopback menus: a `data-out` link offers new tab, native app, and copy
      absolute path. An in-root image link offers all five items. A
      non-image outside the root offers copy only.
    - Loopback actions: Ctrl+click on a `data-out` link is left to the
      browser. Native app POSTed and opened the resolved path, with a toast.
    - LAN: both images are swapped, the `data-out` link offers copy only,
      and Ctrl+click opens the menu.
- Decisions: the new names are `OutsideImage`, `FileAction.Image`/`Outside`,
  `data-img`/`data-out`, and the `native` body field. An absolute path that
  is lexically inside the root but refused by the in-root rules (for
  example a symlink escaping the root) stays refused rather than falling
  through to the out-of-root rule. A topic body's in-root image link also
  gets the native item, through `data-img` forwarding.
- Open: run a Windows by-hand check of "open in native app" (rundll32
  FileProtocolHandler with a path containing spaces is untested).
  `Sec-Fetch-Site: none` loads (address bar, the browser's own "open image
  in new tab", possibly reload) get a 404, per the ticket.

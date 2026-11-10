---
title: "Dashboard agent host: a read-only file browser with image, video, and sandboxed HTML preview"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope (no file explorer) is partly reopened here, for browsing and preview only"
---

# Dashboard file preview

## Background

Track B of the epic: let the user see what an agent produced without leaving
the dashboard: browse a folder and preview HTML, images, and video
(2026-11-10 whiteboard, (01), (04)).

## Decisions

1. **Browse and preview only; editing is out of scope entirely (user,
   (04) a and note).** A file tree, and previews of HTML, images, and video,
   plus Markdown and diffs where useful, and an "open in external editor"
   action. Rejected: light single-file editing ((04) b); a full editor
   ((04) c). The 2026-07-30 drop for Orca already judged the editor side as
   territory others do better (`260730-research-ws-dashboard-drop-for-orca`).
2. **HTML and SVG render in isolation (technical, (07)).** HTML from a
   workspace renders on an origin separate from the dashboard, or in a
   sandboxed iframe without `allow-same-origin`, so it cannot reach the
   dashboard token or API; dashboard access already equals execution
   authority ((05)). SVG can carry script, so it is treated like HTML.
   Images and video are served statically from the dashboard origin.
3. **LAN token holders browse read-only inside the root (announced, (105)
   ok).** As the session file view already does (`file_route.go`); reads
   outside the root stay `localViewer` only. The epic's trusted-origin
   boundary covers approval and the terminal.
4. **HTML preview through a capability path (technical, (106) ok).** HTML
   and SVG load in an iframe with `sandbox="allow-scripts"` and no
   `allow-same-origin`, from a preview route whose path carries a
   short-lived, root-scoped read capability, so relative assets resolve
   without cookies; the route's CSP blocks requests to other origins.
   Rejected: `srcdoc`, which cannot load relative assets, while Chromium
   sends no SameSite=Lax cookie from a sandboxed frame (a068fb9f).
5. **Video streams with Range (technical, (107) ok).** mp4, webm, and ogg
   are served through `http.ServeContent`, as images are, with no size cap,
   and a test covers Range. Rejected: a size limit; the 2 MiB cap is for
   text rendering.
6. **Other text stays plain (technical, (108) ok).** No highlighting in
   Phase 1; `.diff` and `.patch` render through `gitdiff.go`. Rejected:
   vendoring a highlighter.
7. **Open in the external editor (technical, (109) ok).** A loopback-only
   route opens an in-root file in the OS default app, under the system-open
   gate. Rejected: widening the system-open route itself.

## Prior Art

- The session file view, `GET /s/<reply-id>/file?path=`
  (`internal/wsdashboard/file_route.go` header): root containment with
  symlink checks; images served under the image route's CSP so an SVG runs
  no script; Markdown rendered through the board renderer in escaped mode;
  other UTF-8 text served as `text/plain` under that CSP, so an `.html` file
  never runs today; binaries get a "not viewable" page. It serves loopback
  viewers and LAN token holders alike, on the reasoning that the token
  already lets its holder message a lead that reads any file.
- The composer's `@` file mention list, `GET /s/<reply-id>/files`
  (`files_route.go`): git's file list or a capped walk, cached per root.
- The system-open route (`sysopen_route.go`) opens an http(s) URL or this
  session's file view in the host's default browser, or an image file (an
  absolute path with an image extension, nothing else) in the host's default
  app, loopback viewers only (`sysopen_route.go#L13-L41`).

## Constraints

- Independent of the session holder and of daemon lead sessions; it can
  land at any time.
- The browsing root comes from the workspace model
  (`261110-feat-ws-dashboard-workspaces`) once it exists; before then, a
  folder the user picks or the session's working directory.
- Reads stay inside the chosen root: no path traversal out of it.
- Before promotion, an HTML mockup settles where the browser lives in the
  dashboard (user, (90) a).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "File browsing is read-only with HTML, image, and video preview; editing is out of scope; HTML and SVG render isolated ((04) a, (07)); child `261110-feat-ws-dashboard-file-preview`." — bearing: supports
- 261109-feat-ws-dashboard-composer-file-mention (2026-11-09, Decisions): "Absolute browsing is loopback only: listing a directory outside the root is served only to a loopback viewer (`localViewer`, enforced on the server, not just hidden in the page); a LAN viewer gets root search only." — bearing: constrains
- 1b06ffd4 (2026-11-09, commit, 261109-feat-ws-dashboard-open-out-of-root-images): "Go tests and headless Chromium loopback/LAN checks passed; review Critical (\??\UNC namespace bypass) fixed in 8295d5c83." — bearing: constrains
- 0336011f (2026-11-07, commit, 261107-feat-ws-dashboard-link-actions): "Non-local viewers get 404 from the open route (not 403) so the route looks absent to a LAN token holder and TestLANModeGuardsEveryRoute stays uniform; both new routes are added to that test." — bearing: constrains
- 0336011f (2026-11-07, commit, 261107-feat-ws-dashboard-link-actions): "The 2 MiB cap applies to text and markdown only; images are served as the image route serves them." — bearing: constrains
- 229596b4 (2026-11-07, commit): "Review-sweep NEEDS FIX repair for slice 2: ... m2f Now HTML moved into a sandboxed srcdoc iframe without allow-same-origin (board (99) c)." — bearing: supports
- a068fb9f (2026-11-07, commit): "Images: a measured check showed Chromium sends no SameSite=Lax cookie from a sandboxed srcdoc frame, so in LAN mode the image route would refuse Now images." — bearing: constrains
- 260525-feat-ws-dashboard-markdown-renderer-polish (2026-05-25, Decisions): "Keep raw HTML disabled or inert; this polish does not reopen the deferred sanitized/sandboxed HTML decision." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/file_route.go, files_route.go, pages.go route switch, assets/dashboard.tmpl, dashboard.css, dashboard.js |
| scope.surface | public-interface | daemon HTTP routes under /s/<reply-id>/ such as file and files, served to loopback and LAN viewers |
| scope.new_public_symbol | unknown | Phase 1 scope is unsettled; a new browse route or preview route is likely but not named |
| scope.new_type_contract | unknown | no route answer shape is specified yet; files_route.go filesAnswer is the nearest existing contract |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/file_route_test.go, files_route_test.go, access_test.go TestLANModeGuardsEveryRoute |
| complexity.reuse_points | confirmed | openContained in file_route.go, imageContentSecurityPolicy and http.ServeContent, sandboxed srcdoc doc-frame in internal/wswhiteboard/render.go#L452-L490 |
| complexity.side_effect_risk | moderate | changes what the file view serves for HTML and video to every admitted viewer including LAN token holders |
| risk.correctness | moderate | root containment, symlink and swap checks, and Range handling for video must hold on every new path |
| risk.fit | high | page placement, video limits, extra text types, and LAN access are all unsettled and the session page has no file-browser slot today |
| risk.test | moderate | sandbox isolation and video playback need headless browser checks beyond Go handler tests |
| risk.security_or_contract | high | renders workspace-authored HTML and SVG next to an origin whose token equals execution authority |

## Phases

### Phase 1: tree and previews

Scope and verification are settled at promotion, after the Constraints'
open items.

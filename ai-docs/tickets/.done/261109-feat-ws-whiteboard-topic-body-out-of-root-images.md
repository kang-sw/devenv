---
title: "Served whiteboard topic bodies show images outside the project root for a loopback viewer"
related:
  261109-feat-ws-dashboard-open-out-of-root-images: extends its out-of-root image route to topic bodies, which it left out of scope
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6115f023b65734b8
sage-review-completeness-reviewed: 6115f023b65734b8
completed: 2026-11-09
---

# Served whiteboard topic bodies show images outside the project root for a loopback viewer

## Background

261109-feat-ws-dashboard-open-out-of-root-images (1b06ffd4b) lets a loopback
viewer of the served dashboard see an absolute image path outside the
session root inline, through the loopback-only image route. It covers lead
replies and board item fields (`OutsideImage` in `servedViewInput`,
`internal/mcp/dashboard_page.go`). Topic bodies were left out: they render
in a sandboxed frame that inlines images as `data:` URLs (`DocImageSrc` ->
`wsdashboard.ImageDataURL`, root-only, up to 2 MiB) and has no viewer
context. A larger image already falls back to the image route, which only a
loopback viewer's frame can load.

A lead posted mockup screenshots from its scratchpad (`/tmp/...png`) in a
topic body, and the user saw only path text. The user asked whether
out-of-root display would never come to topic bodies ("root밖 표시는 그러면
앞으로도 안 하는 건가요?").

## Decisions

- **Topic bodies load out-of-root images through the route** (125 a, user
  2026-11-09): on the served page, a topic body's absolute image path outside
  the root loads through the same loopback-only out-of-root image route item
  fields use, and the server enforces loopback on each request.
  Rejected: inlining out-of-root images as `data:` URLs. The frame's content
  is the same for every viewer, so a LAN viewer would receive the file's
  bytes. Rejected: leaving it as is and having the lead copy images into the
  root.
- **Viewers and pages that cannot load it** (125 a): a LAN viewer, pages
  written to disk, and the embed keep today's path text for an out-of-root
  image. A LAN viewer never sees a broken image. This keeps the no-broken-image
  intent of 090f4d00 that the prior ticket preserved.

- **Parent relay** (126 a, user 2026-11-09): the sandboxed frame does not
  load the route itself. Its opaque origin most likely sends
  `Sec-Fetch-Site: cross-site`, which the out-of-root branch 404s
  (`crossSiteFetch`, CORP same-origin; 06084496). Instead the frame
  (doc-frame.js) asks the parent page by postMessage. The parent, a
  same-origin page already gated to loopback, fetches the route and hands
  the bytes back as a blob the frame shows. A failed fetch (a LAN viewer, a
  missing file) makes the frame show the path text. The route's cross-site
  gate and CORP stay exactly as recorded.
  Rejected: a signed URL that lets the route accept a cross-site request
  carrying a daemon-keyed signature. It weakens a recorded security gate for
  a convenience feature. Rejected: dropping topic bodies from scope again.

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md` (all of
  `agents-plugin-tool/`) and `ai-docs/manuals/ws-mcp.md` apply.
- Image extensions only (`imageTypes`), and the image route's CSP stays on
  every out-of-root image, as in the prior ticket.
- In-root images in topic bodies keep `data:` inlining unchanged, and an
  in-root image over 2 MiB keeps today's route fallback.
- The parent's relay handler is the security boundary and stays narrow: it
  accepts a message only when `event.source` is one of its own topic-body
  frames; it fetches only this session's out-of-root image route
  (`/s/<rid>/image?path=...`) for an image-extension path, never an arbitrary
  URL the frame names; it returns only an image blob. It is never a generic
  fetch relay. A comment at the handler and a test pin this.
- A srcdoc frame inherits the embedding page's CSP, so any CSP change for
  showing the blob widens `img-src` by `blob:` only.
- The board render stays viewer-independent: the loopback/LAN outcome is
  decided on the client (the parent's fetch succeeds or fails). No
  `localViewer` enters the board render, so the shared stream render of
  261109-bug-ws-dashboard-stream-wake-rerender-cpu stays sound.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, Decisions): "The widening lives in served-only functions. `wsdashboard.ImageSrc`, `ImageDataURL`, the `DocImageSrc` -> `ImageSrc` fallback in `wswhiteboard/markdown.go`, and the shared `FileActions` stay root-only." — bearing: constrains
- 2c1ae57e (2026-11-09, commit): "`fileTarget` consults it only after `ImageSrc` and `FileActions` come back empty, never while a topic body renders (new `pageData.inDoc`)" — bearing: constrains
- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, Decisions): "Out-of-root images are served only to a loopback viewer (`localViewer`). A LAN viewer, token or not, gets the route's ordinary 404, so the route says nothing about which paths exist." — bearing: supports
- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, Decisions): "An out-of-root image keeps the image route's CSP (`imageContentSecurityPolicy`), so an SVG never runs script in the daemon's origin." — bearing: constrains
- 06084496 (2026-11-09, commit): "Sec-Fetch-Site: none ... is a user-started navigation, not another site embedding or probing the image ... same-site/cross-site 404 cases are kept." — bearing: constrains
- 2c1ae57e (2026-11-09, commit): "The image swap sweep uses img.decode() rejection rather than naturalWidth alone, which is also 0 for a loaded SVG without intrinsic size." — bearing: supports
- a068fb9f (2026-11-07, commit): "Chromium sends no SameSite=Lax cookie from a sandboxed srcdoc frame, so in LAN mode the image route would refuse Now images ... inlines them as data: URLs" — bearing: constrains
- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, Decisions): "Topic bodies render in the sandboxed frame the Now slot used ... an opaque-origin frame that sends no cookie, one frame per body" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/dashboard_page.go, internal/wswhiteboard/markdown.go, internal/wswhiteboard/render.go, internal/wswhiteboard/view.go, internal/wswhiteboard/assets/doc-frame.js (frame request), internal/wsdashboard/assets/dashboard.js (parent relay); image_route.go server gate unchanged |
| scope.surface | cross-module | wswhiteboard View/ViewInput hooks, wsdashboard image route, and mcp servedViewInput all change together |
| scope.new_public_symbol | unknown | the phase may add a topic-body hook beside View.OutsideImage or reuse it; not settled by the ticket |
| scope.new_type_contract | unknown | a new hook signature on wswhiteboard.View is possible but unspecified |
| scope.test_surface | existing | internal/wswhiteboard/render_test.go, internal/wswhiteboard/markdown_test.go, internal/wsdashboard/image_route_test.go, internal/mcp/dashboard_page_test.go |
| complexity.reuse_points | confirmed | wsdashboard.OutsideImage and handleImage outside branch in internal/wsdashboard/image_route.go, data-out image swap in dashboard.js |
| complexity.side_effect_risk | moderate | lifts the inDoc exclusion for a marker the frame relays to its parent; the route's cross-site gate and CORP stay unchanged |
| risk.correctness | moderate | the postMessage relay and blob display, and the path-text fallback, must be proven in a headless browser |
| risk.fit | moderate | reverses the prior ticket's topic-body exclusion of OutsideImage while keeping ImageDataURL root-only |
| risk.test | moderate | the frame load and path-text fallback are browser behavior Go tests cannot observe |
| risk.security_or_contract | high | the parent relay is a new same-origin fetch on a frame's behalf; if it is not narrowed to its own frames and the image route it would break the sandbox's isolation. The route's cross-site 404 and CORP stay unchanged |


## Phases

### Phase 1: out-of-root images in served topic bodies

- Give the topic body's image resolution on the served page the out-of-root
  route for absolute out-of-root image paths, delivered through the parent
  relay above. Make whatever change the body frame's CSP needs to show a
  blob image. The route's server-side gate does not change.
- Make a failed load (a LAN viewer, a missing file) show the path text
  instead of a broken image.
- Check the relay in headless Chromium: from a loopback page, the frame shows
  the image; with the parent's fetch refused, it shows path text. Also record
  whether a direct frame load of the route is indeed refused.

#### Verification

From `agents-plugin-tool/`: `gofmt -l ./internal ./cmd`, `go vet
./internal/...`, `go test ./internal/wswhiteboard/ ./internal/wsdashboard/
./internal/mcp/ -count=1`, and `node --check` on changed JS. Tests pin:
- A served topic body with an out-of-root absolute image renders the frame
  marker the relay uses (the out-of-root route reference), not `data:`.
- The relay handler's narrowing (own frames only, image route only).
- The route answers a loopback viewer and 404s a LAN viewer.
- Pages written to disk and the embed keep path text.
- An in-root body image is still inlined.

Required: the headless Chromium check from the phase (loopback page shows
the image through the relay; a refused parent fetch shows path text; a
direct frame load of the route recorded). Report its outcome in the Result.

### Result (1e1a91a01) - 2026-11-09

Landed in 1e1a91a01, plus a comment fix in 0c54cbc64.

- **Server render.** In a framed topic body, an image that `OutsideImage` serves keeps its path text, now marked `data-relay="/s/<rid>/image?path=..."` (`pageData.relayImage`, `markdown.go`). `framedDoc` lists that body's relay URLs, deduped and space-separated, as the iframe element's own `data-relay`. `inDoc` still keeps `OutsideImage` off a body's srcs and links. `ImageDataURL`, `DocImageSrc`, and `FileActions` stay root-only. In-root images keep `data:` inlining and the over-2-MiB route fallback. Pages written to disk and the embed take no relay, and no viewer context enters the render.
- **Frame (`doc-frame.js`).** The frame posts `{wbDoc: "img", id, href}` for each marker. It asks again when the host asks for its height. It accepts an answer only from `window.parent`, and only when the answer is an `image/*` Blob. It reads the Blob as a `data:` URL and replaces the path text only after `decode()` resolves. Otherwise the path text stays.
- **Host relay (`dashboard.js`, `relayHref` / `relayImage`).**
  - It handles a request only inside the listener gated on `docFrame(e.source)`.
  - It fetches only a URL on the asking frame element's own `data-relay` list, and that URL must also be this session's image route for an image extension, with no `&`, `#`, or whitespace.
  - The fetch is same-origin with `redirect: "error"`.
  - It answers only with an `image/*` Blob, posted only to the asking frame.
  - It fetches each request once per frame element.
  - A comment at the handler states that it is never a generic fetch relay.
- **CSP.** No change was needed. The session page's CSP is `frame-ancestors` only, and the frame shows a `data:` URL, not a `blob:` URL.
- **Image route.** The server gate is unchanged.

Decisions:
- **Allowlist on the frame element.** This is narrower than the ticket's minimum ("this session's route for an image name"). Without it, a body script could read any image file on a loopback viewer's machine.
- **`data:` URL instead of `blob:`.** No CSP change and no revoke are needed, and the existing viewer opens the image unchanged.
- **Path text as the default.** The image replaces it only after decode, so no viewer ever sees a broken image.

Verification, from `agents-plugin-tool/`:
- `gofmt -l ./internal ./cmd` printed nothing.
- `go vet ./internal/...` passed.
- `node --check` passed on `dashboard.js` and `doc-frame.js`.
- `go test ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/ -count=1` passed for all three packages.
- New and updated tests:
  - `TestOutsideImageServesItemsAndRelaysFramedBodyImages` (regions): marker, frame list, dedup, a bad Href gets no relay, the unframed body gets no relay.
  - `TestDashboardServedPageRendersTopicBodyInAFrame` (mcp): marker, frame list, outside bytes never inlined, in-root image still inlined, the disk page and the embed carry no relay.
  - `TestTopicBodyImageRelayStaysNarrow` (wsdashboard): string pins on the gate and the fetch shape, plus the `relayHref` accept/reject table run under node. A mutation that drops the list check fails it.
- The loopback-200 / LAN-404 route behavior stays pinned by the existing `TestImageRouteServesOutsideImagesToLoopbackViewersOnly`.

Headless Chromium check (chrome-headless-shell 1223 over CDP, against a temporary in-package fixture daemon in LAN mode, not committed):

| Case | What happened |
|---|---|
| Loopback page | The relayed outside image showed as `<img src="data:image/png...">` (naturalWidth 1). The relay fetch was `Sec-Fetch-Site: same-origin`, status 200. |
| Missing outside file, loopback | The relay fetch got 404 and the path text stayed. |
| LAN-token host (`lan.test` mapped to 127.0.0.1) | Both relay fetches got 404 and both images stayed path text. |
| Direct frame `<img src=route>` | Sent as `Sec-Fetch-Site: cross-site` and refused (naturalWidth 0). This confirms the ticket's premise. |
| Forged request from inside the frame | A URL not on the frame's list, and a repeated id for a listed URL, each caused no fetch. |

Lite review: one round. No Critical or Important findings. One minor (garbled comment) was fixed in 0c54cbc64. Two minors remain as observations:
- The `relayHref` table is skipped when node is not installed.
- The relay fetches again after each board swap, because the frames are re-created, and a failure is not retried within one frame's life.

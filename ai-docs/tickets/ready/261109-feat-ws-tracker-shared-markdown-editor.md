---
title: "ws tracker: one markdown editor for item body, comment input, and comment edit, with preview and image paste"
parent: 261107-epic-ws-personal-common-store
related:
  261108-feat-ws-tracker-ledger-attachments: prerequisite of Phase 2 only (blob storage, upload staging, and the attachment route)
  261109-feat-ws-dashboard-composer-file-mention: its reusable popup module is a later candidate for this editor; not adopted here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ebc4ef9e4c5f5534
sage-review-completeness-reviewed: ebc4ef9e4c5f5534
---

# ws tracker: one markdown editor for item body, comment input, and comment edit, with preview and image paste

## Background

The user (2026-11-09) wants the tracker's thread comment input and thread
body editor to be one component, comments rendered by the body renderer,
with rich editing including image paste (Ctrl+V).

Today (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`):
- The item body editor (`#iw-ebody`, `startEdit` / `saveEdit`), the comment
  input (`#cbox`, the `quick()` `comment` case), and the comment edit
  (`#tkcedit`, `startCedit` / `saveCedit`) are three separate plain
  textareas. Only `#cbox` has the `@`/`#` mention popup.
- Rendering is already shared: `textBlock` shows `VS.html[kind][id]`, which
  the view endpoint builds with `trackerText` (`cmd/ws-mcp/tracker_host.go`
  `renderViewHTML`), i.e. `wswhiteboard.RenderText` in safe mode: local
  images stay path text (`TestTrackerTextSafeMode` pins this).
- The domain already carries attachment metadata
  (`attachments: [{oid,name,size,mime}]` on bodies and comments,
  `wstracker` `checkExtras`, `attachments.max_bytes`), but no markdown text
  references an attachment, the product mounts the tracker with
  `caps.attach: false`, and no tracker textarea handles paste or drop.
- Image storage is the job of `261108-feat-ws-tracker-ledger-attachments`
  (board (19) a).

## Decisions

- **Rich means a shared markdown editor** (user; board (18) a): one
  textarea-based markdown editor component used for the item body, the
  comment input, and the comment edit, with a preview toggle and image
  paste/drag. Rejected: a WYSIWYG editor (a large JS dependency and
  markdown round-trip problems; the renderer is server-side markdown).
- **Images live in the ledger** (user; board (19) a): pasted images are
  stored by the ledger attachments ticket as `attachments/<sha256>` in the
  ledger, so they are visible to every member. Rejected: a daemon-local
  store (other members would see broken images).
- **Stored image form** (user; board (41) a): a pasted image is written into
  the markdown as `![name](attachment:<sha256>)`, beside its entry in the
  body's or comment's attachment list; the paste interaction itself (insert
  at the caret) follows the dashboard composer. Rejected: a `[Image #n]`
  marker mapped to the nth attachment (short, but breaks when the list is
  reordered or an attachment removed, and is not markdown).
- **Server-rendered preview** (board (42) ok): the preview toggle renders
  through a small new tracker render route that runs `trackerText`.
  Rejected: a client-side markdown renderer (there is none in the page, and
  one renderer keeps preview and saved view identical).
- **Two phases** (board (43) ok): Phase 1 is the shared editor in all three
  places with preview and the existing mention popup working in all three;
  Phase 2 adds image paste and drag on top of the ledger attachments
  ticket. Rejected: one phase after that ticket lands (the editor work does
  not need storage).
- **Accepted images** (board (44) ok): png, jpeg, gif, webp, each capped by
  the store's `attachments.max_bytes` (the composer's `IMG_TYPES`
  precedent and ledger decision (338) a).
- **Only own attachments render** (board (45) ok): the tracker renderer
  turns into `<img>` only an image whose target is `attachment:<sha256>`
  naming an attachment of that same item body or comment; every other local
  image stays path text. Rejected: opening local images generally (keeps
  the safe-mode guarantee for everything else).
- **Upload at paste time** (board (62), auto-confirmed from fact
  population): a pasted or dropped image is uploaded to the ledger
  ticket's staged upload route at once, so the preview can show it through
  the attachment read route; an image the user never saves is left to that
  ticket's staging expiry. Rejected: holding the image in the browser and
  uploading on save like the dashboard composer
  (`261107-feat-ws-dashboard-composer-image-attach`), because the
  server-rendered preview could not show an image the server has not seen.
- **Upload request shape** (delta design review, auto-confirmed; matches
  the ledger ticket): the page sends `POST …/attachments?name=<file name>`
  with the raw bytes as the body; a dropped file sends its file name, and a
  clipboard paste with no file name sends none, so the route's default
  `image` applies.
- **Save after staging expiry** (delta design review, auto-confirmed): a
  save naming an oid whose staged copy expired (pasted, left unsaved past 24
  hours) is refused by the ledger; the editor shows that refusal as a save
  error and keeps the draft, so the user can paste the image again.
- **Images only** (board (46) ok): pasting or dropping a non-image file is
  ignored with a short notice. Rejected: general file attachments (only
  image paste was asked for, and an attachment download path is not
  designed).
- **Picks reach the saved mentions** (design review, auto-confirmed): once
  the popup works in the body editor and the comment edit, a newly picked
  `@name` or `#ref` is added to the write's `mentions`/`refs`; today
  `saveEdit`/`saveCedit` send `keptMentions(old, body)`, which keeps only
  entries already stored, so a pick there would do nothing. Entries whose
  text was deleted drop out as now.
- **Preview of unsaved images** (completeness and design review,
  auto-confirmed): the Phase 2 render request carries the editor's pending
  attachment list (saved entries plus staged `{oid,name,size,mime}`), the
  server accepts a pending entry only when that oid is staged on this clone
  or already in the ledger, and rule (45) treats that list as the text's
  own; the attachment read route serves staged blobs and blobs named by
  pending journal events (ledger ticket). Phase 1's render contract is
  text only.
- **Pruning on save** (completeness and design review, auto-confirmed): a
  save sends only attachment entries whose `attachment:<oid>` text is still
  present, as mentions and refs are kept only while in the text; a comment
  edit starts from the comment's stored list and merges new uploads into
  it. A dropped entry's blob stays in the ledger history (content-addressed,
  never deleted).
- **Where an over-limit image is refused** (completeness review,
  auto-confirmed): the page checks `attachments.max_bytes` from the view
  before uploading and shows the notice without a request; the upload route
  refuses independently.
- **Mockup without attachments** (completeness review, auto-confirmed): the
  mockup has no upload route, so it runs Phase 2 as `caps.attach: false`
  (paste and drop of images ignored with the notice).
- **Existing mention popup** (board (61) ok): the editor keeps the tracker's
  existing `@`/`#` mention popup, made to work in all three places.
  Rejected: adopting the composer file-mention ticket's new module now
  (keeps the two parallel tickets independent).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- The mockup (`ai-docs/ref/tracker-mockup`) loads this same `tracker.js`;
  keep it working and keep the mockup i18n file current
  (`TestTrackerMockupI18nIsCurrent`). Every new UI string goes into every
  dashboard i18n table.

## Prior Decisions

- 34667211 (2026-11-08, commit): "Safe mode is RenderText with the zero TextTargets, no new knob: typed HTML (inline and block) is escaped text, file:/relative/absolute paths are path text, every image is alt or path text" — bearing: constrains
- 34667211 (2026-11-08, commit): "tracker.js change is one helper (textBlock) plus the two call sites and the click guard ... The block reuses the host page's shared .md rules (WhiteboardCSS is always on the dashboard page)" — bearing: supports
- 057a2ab0 (2026-11-08, commit): "The dashboard stays free of wstrackerhost/wstracker/wsledger: the interface speaks JSON-level types and the cmd/ws-mcp adapter converts ... TestLANModeGuardsEveryRoute ... lists every tracker route" — bearing: constrains
- c779de58 (2026-11-08, commit): "The item body editor and new-item input never had a mention picker, so per plan item 5 the popup is composer-only." IME guard: keydown returns early on isComposing or keyCode 229 — bearing: constrains
- 3b260712 (2026-11-08, commit): "mentionPick extends the replaced range over the token's run after the caret ([^\s@#]*, the same class mentionToken matches), so `@ma|ry` becomes one mention." — bearing: constrains
- 261109-feat-ws-dashboard-composer-file-mention (2026-11-09, Decisions): "Tracker popup stays (board (61) ok): the tracker comment box keeps its own mention popup; migrating it to the new module is out of scope here." — bearing: supports
- 261107-feat-ws-dashboard-composer-image-attach (2026-11-07, Decisions): "The browser holds a pasted or dropped image ... and uploads it only when the message is sent (user, 2026-11-07), so an abandoned paste never touches disk." — bearing: contradiction-candidate
- 0a80fdbf (2026-11-08, commit): "Open follow-ups put to the user on the board: body-less item.edit carrying mentions/attachments flagged; ... lowering attachments.max_bytes flags edits that carry an older larger attachment." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker.js, assets/tracker.css, agents-plugin-tool/cmd/ws-mcp/tracker_host.go, internal/wsdashboard/tracker_routes.go, dashboard i18n tables, ai-docs/ref/tracker-mockup |
| scope.surface | public-interface | new POST tracker render route under /tracker/stores/<id>/, plus the TrackerHost interface the cmd/ws-mcp adapter implements |
| scope.new_public_symbol | yes | the tracker render route and its host method; Phase 2 enables caps.attach in the product mount |
| scope.new_type_contract | yes | render route request markdown text and response trackerText HTML; Phase 2 attachment:<sha256> image target form |
| scope.test_surface | existing | cmd/ws-mcp/tracker_host_test.go TestTrackerTextSafeMode, internal/wsdashboard/access_test.go TestLANModeGuardsEveryRoute, internal/wsdashboard/i18n_test.go, internal/wsdashboard/tracker_mockup_i18n_test.go; no JS test harness |
| complexity.reuse_points | confirmed | mentionToken/mentionSync/mentionDraw/mentionAnchor/mentionPlace/mentionPick tracker.js#L1842-L1910, trackerText tracker_host.go#L120-L122, decodeTrackerBody and trackerBodyLimit tracker_routes.go#L38-L39 L188-L191, dashboard.js IMG_TYPES L653 |
| complexity.side_effect_risk | moderate | replaces three textareas and their keydown, input, and draft-preserve handlers in a 3600-line tracker.js also loaded by the mockup |
| risk.correctness | moderate | mention popup generalization off fixed #cbox/#tkmpop and per-place save/cancel keys must keep IME guards and draft restore |
| risk.fit | moderate | render route must cross the JSON-only wsdashboard to cmd/ws-mcp boundary; Phase 2 depends on unlanded 261108-feat-ws-tracker-ledger-attachments |
| risk.test | high | no browser or JS harness; three editing places and paste/drop are reasoned only, browser hand check may stay unverified |
| risk.security_or_contract | high | Phase 2 opens img rendering in the safe-mode renderer for other members' text; new route needs the LAN access guard and body limit |

## Phases

### Phase 1: shared editor with preview

- Page (`assets/tracker.js`, `assets/tracker.css` or the stylesheet the
  tracker uses): one editor component (a textarea plus a write/preview
  toggle) built by a single function and used for the item body editor,
  the comment input, and the comment edit, replacing the three separate
  textareas while keeping each place's save and cancel behavior and keys.
  The `@`/`#` mention popup works in all three (generalize
  `mentionToken`/`mentionSync`/`mentionDraw`/`mentionAnchor`/`mentionPlace`/
  `mentionPick` off the fixed `#cbox`/`#tkmpop` binding to the active
  editor), and a new pick in the body editor or comment edit is added to
  the saved `mentions`/`refs`.
- Server: a tracker render route that takes markdown text and returns
  `trackerText` output (`cmd/ws-mcp/tracker_host.go`, beside
  `renderViewHTML`, and the dashboard's tracker route table), under the
  access rules of the other tracker routes and the existing tracker write
  body limit; the preview shows its result, so preview and saved view are
  the same renderer.
- The mockup (`ai-docs/ref/tracker-mockup`) loads the same `tracker.js`:
  it keeps working, and where it has no render route the preview falls back
  to a plain-text view. New UI strings go into every dashboard i18n table
  and the mockup i18n file.
- Verification: `go test` for the touched packages (render route: safe
  mode kept, access refusal, body limit); `node --check` on edited JS; the
  i18n table tests and `TestTrackerMockupI18nIsCurrent` green. There is no
  JS test harness, so the Result reasons through each place (body edit,
  comment input, comment edit: write, preview, save, cancel, mention pick)
  and lists the browser hand check as unverified if not done.

### Result (8f0adc0f5) - 2026-11-09

Landed in 212459130 (render route), 5df290c54 (shared editor), 8f0adc0f5
(review fix: keys and focus while previewing).

- Server: `POST /tracker/stores/<id>/render` takes `{"text"}` and answers
  `{"html"}`; `TrackerHost` gained `Render(ctx, id, TrackerRenderRequest)
  (string, error)`, and the `cmd/ws-mcp` adapter runs `trackerText`, the
  renderer `renderViewHTML` uses. The route is in `trackerStoreOps`, so it
  takes the listed-store check, the same-origin POST rule, the LAN guard, and
  `trackerBodyLimit`; it is a read and does not bump the tracker revision.
  `TrackerRenderRequest` is a struct so Phase 2 can add the pending
  attachment list.
- Page: `editorHtml(id, o)` builds all three editors (ids kept: `iw-ebody`,
  `cbox`, `tkcedit`): a Write / Preview toggle, a `data-mention` textarea,
  and a preview pane drawn with the saved block's class (`sh-body` or
  `ctext`). The preview calls the new optional adapter method
  `render(id, text, done)` (documented in the tracker.js header; the
  dashboard adapter posts to the route) and shows the escaped plain text
  without it (the mockup). Preview state lives in `ui.edpv`; the toggle
  works in place so focus stays, and `render()` clears it when its editor
  ends. The `@`/`#` popup follows the focused editor (`MP.box`); a pick in
  the body or comment edit goes to `mpick`, and the save sends
  `keptMentions(withPicks(stored, mpick), body)`. One keydown branch runs the
  IME guard and the popup keys for every editor before each place's own
  save/cancel keys. While previewing, Cmd/Ctrl+Enter on the toggle saves or
  sends and Esc returns to Write.
- Six `js.tk.ed.*` strings in all 11 tables; mockup `i18n-en.js`
  regenerated.
- Decisions: the `@` button stays composer-only (typing `@`/`#` works in
  all three); Esc during an IME composition in the body editor no longer
  cancels the edit, matching the comment box; Esc in preview steps back to
  Write rather than cancelling outright.
- Verification: `go test ./internal/wsdashboard ./internal/wsi18n
  ./cmd/ws-mcp` ok (new `TestDashboardTrackerRenderMatchesTheView`: the
  render equals the view's html for the same body, safe mode kept; route
  tests cover the 200 answer, the 405, unknown store, host error, malformed and
  oversize body, origin, LAN guard, and no revision bump);
  `TestTrackerMockupI18nIsCurrent` and the i18n key tests green;
  `node --check` on tracker.js and dashboard.js ok. Browser check: the
  mockup driven in headless Chromium over CDP with an injected fake
  `adapter.render` passed 27 scripted checks across body edit, comment
  input, and comment edit (write, preview, render failure, plain fallback,
  save, cancel, `@` and `#` pick, IME Enter guard, keys and focus while
  previewing). It is a one-off script, not a checked-in harness; the live
  product page (real daemon) was not hand-checked.
- Review (lite): two minors (keys and focus while previewing), both fixed
  in 8f0adc0f5.

### Phase 2: image paste and inline images

Prerequisite: Phase 1 of `261108-feat-ws-tracker-ledger-attachments`.

- Page: paste (Ctrl+V) and drag-drop of an image into any of the three
  editors uploads it to the staged upload route, then inserts
  `![name](attachment:<sha256>)` at the caret and adds the returned
  `{oid,name,size,mime}` to the attachment list the save sends (pruned of
  entries whose text was deleted; a comment edit merges into the stored
  list); a non-image or an over-limit file (checked against the view's
  `attachments.max_bytes` before upload) is refused with a short notice; the
  product mount enables `caps.attach` for this and the mockup keeps it off.
  The preview's render request carries the pending attachment list.
  Upload is the raw body with `?name=` (dropped file's name; none for a
  clipboard paste); a save refused for an expired staged oid shows the
  error and keeps the draft.
- Renderer: `trackerText` (or the tracker's call into
  `wswhiteboard.RenderText`) renders `attachment:<sha256>` as an `<img>`
  pointing at the attachment read route only when that oid is in the same
  body's or comment's attachment list (for a preview, the pending list the
  server validated against staging and the ledger); every other local image stays path
  text, and `TestTrackerTextSafeMode` is extended rather than loosened.
- Verification: `go test` for the touched packages (an own attachment
  renders as `<img>`, a foreign oid or a local path does not, a pending
  staged oid renders in preview and an unstaged one does not, the write
  commits the staged blob); `node --check`; i18n tests green. The Result
  reasons through paste, drop, refusal, and preview of a pasted image and
  lists the browser hand check as unverified if not done.

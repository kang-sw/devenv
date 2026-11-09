---
title: "ws tracker: one markdown editor for item body, comment input, and comment edit, with preview and image paste"
parent: 261107-epic-ws-personal-common-store
related:
  261108-feat-ws-tracker-ledger-attachments: prerequisite of Phase 2 only (blob storage, upload staging, and the attachment route)
  261109-feat-ws-dashboard-composer-file-mention: its reusable popup module is a later candidate for this editor; not adopted here
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
- **Images only** (board (46) ok): pasting or dropping a non-image file is
  ignored with a short notice. Rejected: general file attachments (only
  image paste was asked for, and an attachment download path is not
  designed).
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
  editor).
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

### Phase 2: image paste and inline images

Prerequisite: Phase 1 of `261108-feat-ws-tracker-ledger-attachments`.

- Page: paste (Ctrl+V) and drag-drop of an image into any of the three
  editors uploads it to the staged upload route, then inserts
  `![name](attachment:<sha256>)` at the caret and adds the returned
  `{oid,name,size,mime}` to the attachment list the save sends; a
  non-image or an over-limit file is refused with a short notice; the
  product mount enables `caps.attach` for this.
- Renderer: `trackerText` (or the tracker's call into
  `wswhiteboard.RenderText`) renders `attachment:<sha256>` as an `<img>`
  pointing at the attachment read route only when that oid is in the same
  body's or comment's attachment list; every other local image stays path
  text, and `TestTrackerTextSafeMode` is extended rather than loosened.
- Verification: `go test` for the touched packages (an own attachment
  renders as `<img>`, a foreign oid or a local path does not, the write
  commits the staged blob); `node --check`; i18n tests green. The Result
  reasons through paste, drop, refusal, and preview of a pasted image and
  lists the browser hand check as unverified if not done.

---
title: "Dashboard composer image attachment, delivered as file paths"
parent: 261107-epic-ws-dashboard-messenger-round2
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a9dc05efafa08874
sage-review-completeness-reviewed: a9dc05efafa08874
---

# Dashboard composer image attachment, delivered as file paths

## Background

While dogfooding the live session page (2026-11-07) the user went back to the
terminal only to attach a screenshot. The composer accepts text and board
answer chips; it has no way to send an image. Both hosts' agents already open
images by path: Claude Code's Read tool, and Pi's read tool ("Supports text
files and images (jpg, png, gif, webp, bmp). Images are sent as attachments" -
the tool description in `@earendil-works/pi-coding-agent`'s
`dist/core/tools/read.js`). So the message only needs to carry a path the
agent can read; no host adapter change is required.

## Decisions

- **Delivery is a file path in the message text**, not a native image block in
  the injected prompt (user, 2026-11-07). Rejected for this ticket: image
  content blocks through the Claude mod's prompt submit or Pi's follow-up
  delivery - needs adapter work on both hosts and their API support checked;
  deferred to the epic.
- **The browser holds a pasted or dropped image** (a removable thumbnail chip
  in the composer) and uploads it only when the message is sent (user,
  2026-11-07), so an abandoned paste never touches disk.
- **One request.** The existing `POST /s/<rid>/message` JSON body gains
  `images: [{name, data}]` (base64), and the daemon writes the files in the
  same request that holds the message, under the same Origin check and inbox
  lock. Rejected: a separate multipart upload route answered with file refs -
  a failed second step leaves orphan uploads, and base64's 33% overhead is
  irrelevant at screenshot sizes.
- **Formats and caps.** PNG, JPEG, GIF, and WebP only, decided by content
  sniffing (`http.DetectContentType`), not the browser's mime label; at most 8
  images per held entry and 10 MiB decoded each; the route's body limit is
  raised only for a request carrying images. Rejected: accepting whatever the
  browser labels an image - these four are what both hosts' read tools open
  (Pi also reads BMP; Claude Code's Read does not list it), and the caps bound
  an accidental or hostile paste.
- **Location and text.** Files live at
  `sessions/<rid>/inbox/attachments/<upload-id>-<n>.<ext>` in the dashboard
  state dir, where `<upload-id>` is a random id minted per message request before
  the inbox write. The name is never tied to the inbox entry name and a file
  is never renamed: entry names change on every merge (`writeEntry`) and are
  minted inside `Send`, so keying files to them would need a rename on each
  merge. Rejected: write after `Send` and rename into the entry's name. The
  attachments dir sits inside the inbox (user, 2026-11-07, option (b) of
  (34)), so the recorded "the inbox is the daemon's one write target" holds as
  written: `entryNames` skips directories and `DrainedDir` is unaffected.
  Rejected: a sibling `sessions/<rid>/attachments/` with that decision widened
  to name it. The
  held entry records their absolute paths, and the delivered text
  appends one line per image, `[image: <absolute path>]`, after the typed
  parts and before the answer block. Rejected: writing into the project tree
  (pollutes the user's repository and git status); a relative path (the
  agent's cwd is not the state dir).
- **Thumbnails.** Held and conversation bubbles show the images as thumbnails
  through a new `GET /s/<rid>/attachment/<name>` route with the image route's
  access rules (loopback, or the LAN token in LAN mode), the name confined to
  that session's attachments dir. Rejected: file names only - the page is
  meant to replace the terminal as a messenger, and the user checks what was
  sent by seeing it.
- **Merge and cancel.** Merging sends into one held entry accumulates their
  images (the 8-image cap applies to the merged entry). Cancelling a held
  entry returns its images to the composer as chips that reference the
  already-written files, and a re-send references them instead of uploading
  again; a referenced file must exist in this session's attachments dir, or the
  send is refused with an error naming it (a chip kept past the 24 h sweep).
  Rejected: delete on cancel and re-upload - after a send or a reload
  the page no longer holds the blobs, so restoring from the server files is
  the only way cancel keeps its "returns the text and chips" contract.
- **Wire details.** New uploads travel as `images: [{name, data}]`; restored
  chips travel as `refs: [<file name>]`, names only, resolved inside this
  session's attachments dir and matched against the exact
  `<upload-id>-<n>.<ext>` grammar (as `wsinbox.NameForID` does for entry ids),
  not only a containment check after joining; the attachment GET route uses
  the same check. A message with images or refs and empty text is accepted
  (`wsinbox.Normalize` counts image paths as content, not `ErrEmpty`). The cancel response adds `images: [<file name>]` so the page
  rebuilds the chips and their thumbnails. Refusals: 400 for a malformed body,
  bad base64, or a missing or out-of-dir ref (the error names it); 413 for the
  count or size caps; 415 for a type that sniffs as none of the four; 409 for
  no live adapter, as today. The body is read through a limit of the
  image-bearing maximum (8 x 10 MiB as base64, plus the existing 256 KiB), and
  a request without images or refs is still refused above 256 KiB once read.
- **Cap check before writing.** Under the inbox lock the route counts the held
  entry's images plus the request's before writing any file, so the merged
  cap can only be over-strict when a drain races it, never under-strict. Any
  refusal after files were written deletes the files that request wrote.
- **Who deletes, and where.** Trim-time deletes run inside `wsinbox.Drain`
  (the `trimDrained` step, in ws-mcp's `dashboard.inbox_drain` on the adapter
  side); the 24 h sweep runs in the daemon on each message write, under the
  inbox lock. Both delete only paths inside this session's attachments dir and
  only files no remaining held entry or drained record lists, so a file two
  records reference (a ref re-sent twice) survives until its last record goes.
  `Drain` does not take the daemon's lock, so the sweep re-reads the held list
  after reading the drained list and deletes only files absent from both
  reads; the remaining narrow gap (a ref validated, then trimmed before `Send`
  records it) is accepted, as 5a0a0a14 accepted its crash gap.
- **Conversation thumbnails** come from the `[image: <absolute path>]` lines in
  the delivered user message: a path inside this session's attachments dir maps
  to the attachment route, any other path stays plain text. Held bubbles use
  the entry's recorded images.
- **Retention follows `drained/`** (user, 2026-11-07: option (a), "개수만
  조금 더 늘리는 방향으로요. 한 50개 정도?"): a record's images (the paths it
  lists) are deleted when that record is trimmed from `drained/`. A file in
  the attachments dir older than 24 h that no held entry and no drained
  record references is swept (the cancelled-and-never-re-sent case). The kept
  count rises by raising `wsinbox.DrainedCap` from 20 to 50, so the page's
  sent records and sent marks also keep 50. Rejected: a fixed age (e.g. 7
  days) independent of the records; keeping them until the session's registry
  entry is pruned; a separate image-only cap of 50 that outlives the records
  (an image whose record is gone appears in no message, and one bound keeps
  the sweep simple; sent records are small JSON files).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Prior Decisions

- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Decisions): "**The inbox is the daemon's one write target.** The daemon reads on-disk stores only; its one write target is the board-answer inbox" — bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Result ed7fbc6e1): "A held entry is a file in `inbox/`. A drained one is in `inbox/drained/`, which keeps the newest 20 (`wsinbox.DrainedCap`)." — bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Decisions): "Cancel removes the entry from the inbox and returns its text and chips to the composer. It touches only a message the session has not received" — bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Result ed7fbc6e1): "Both require an `Origin` equal to `http://<Host>`. A missing, `null`, or cross-site Origin is 403. ... A session without a fresh adapter snapshot gets 409" — bearing: supports
- 5a0a0a14 (2026-11-06, commit): "A merge writes a fresh entry name instead of rewriting in place, so the 'entry files are never rewritten; names alone tell change' invariant ... holds for the inbox too." — bearing: constrains
- 62055a21 (2026-11-06, commit): "Sent marks derive from disk: the newest chip per item across the drained records (newest 20) and then the held entries" — bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-07, Result 5c8743c5c): "The route resolves a relative path against that root and accepts an absolute one only when it is inside the root." (board image route containment) — bearing: supports
- 261106-feat-ws-dashboard-browser-interaction (2026-11-07, Result 5c8743c5c): "LAN mode: `Handler()` wraps the mux in `lanGuard` instead of `hostGuard`" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsinbox/inbox.go, internal/wsdashboard/inbox_routes.go, pages.go (route dispatch), image_route.go, pageview_inbox.go, assets/dashboard.js, dashboard.tmpl, dashboard.css |
| scope.surface | cross-module | wsinbox entry schema and Text, wsdashboard message route body and new attachment route, browser composer; a new HTTP route and on-disk schema field |
| scope.new_public_symbol | yes | attachment route GET /s/<rid>/attachment/<name>; wsinbox Entry image-path field; messageRequest images field |
| scope.new_type_contract | yes | messageRequest gains images [{name, data}]; wsinbox.Entry gains image paths; DrainedCap 20 to 50 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsinbox/inbox_test.go, internal/wsdashboard/inbox_routes_test.go, image_route_test.go |
| complexity.reuse_points | confirmed | sameOrigin and handleInboxWrite (inbox_routes.go), Merge/Cancel/trimDrained (wsinbox/inbox.go), imageType and insideRel (image_route.go), lanGuard/hostGuard access |
| complexity.side_effect_risk | moderate | writes uploaded files to disk and deletes them on trim and sweep, outside the inbox rename-claim protocol |
| risk.correctness | high | file lifecycle must stay consistent across merge, cancel, drain and trim races (entry names change on every merge, inbox.go writeEntry) |
| risk.fit | moderate | page JS and template changes for chips and thumbnails; follows the existing composer chip pattern |
| risk.test | moderate | Go route and inbox tests are specified; the live two-host check (Claude and Pi) is manual |
| risk.security_or_contract | high | accepts untrusted base64 uploads written to disk; needs content sniffing, caps, name confinement, and a body-limit change on the write route |

## Phases

### Phase 1: Attach, deliver, show, and retain images

One behavior slice (a single reviewable unit: route, inbox schema, composer,
thumbnails, and retention land together):

- page: paste and drop into the composer create thumbnail chips (removable);
  send includes them; cancel restores them as file-referencing chips;
- daemon: the message route accepts and validates images, writes them, and
  records their paths on the held entry; the attachment route serves them;
- inbox: the entry carries image paths; the delivered text gains the
  `[image: ...]` lines; trimming a drained record deletes its images; the 24 h
  sweep of unreferenced files;
- conversation and held bubbles render thumbnails for this session's
  attachment paths.

Verification: Go tests for the route (accepted types, sniffing rejects a
mislabeled file, caps, Origin, no adapter -> 409), the inbox (merge
accumulates, cancel keeps files, text lines, trim deletes, sweep), and the
attachment route (confinement, access rules); a live check that a pasted
screenshot sent from the page reaches a Claude session and a Pi session and
each agent opens it by the delivered path.

## Sage Review Round 1 (2026-11-07)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Unnamed widening of 'the inbox is the daemon's one write target' | important | missing |
| 2 | Sweep owner and trigger unspecified | minor | autonomous |
| 3 | Confine attachment deletes, not just reads | minor | autonomous |
| 4 | Wire shape for re-sent file references and the cancel response | minor | autonomous |
| 5 | Merged-entry cap is checked after files are written | minor | autonomous |
| 6 | Body limit cannot be chosen before the body is read | minor | autonomous |
| 7 | A file listed by two records can be deleted while still in use | minor | autonomous |
| 8 | Conversation thumbnails rely on parsing the delivered text | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Re-send request shape for already-written files is unspecified | minor |
| 2 | Image-only send, error statuses, and body-limit value are not stated | minor |
| 3 | Page-side behavior has no automated check, and the conversation-bubble thumbnail source is unnamed | minor |

---
title: "Dashboard opens outside-root file links on the same machine"
---

# Dashboard opens outside-root file links on the same machine

## Background

A link the lead writes on the board (or in a reply) to a file outside the
session's git root renders today as an outside link (`data-out`, `data-abs`,
no `href`; `wswhiteboard/markdown.go` actionAttrs) and opens only through the
system opener on a `data-sysopen` page (`wsdashboard/assets/dashboard.js`
~1979-2140). The user wants such files to open in the dashboard's own file
card and file view, the way in-root files do (`wsdashboard/fs_routes.go`
preview sub-route ~422; `fs_capability.go` `mintCapability`,
`validCapability`, `previewCapability`, `capRefused`).

## Decisions

- (411) Scope is the linked file's folder: a link grants its containing
  folder (so relative CSS/images of an HTML file resolve), except that a
  folder judged too wide grants only the file itself.
- (412) Same machine only: a loopback viewer opens the card; a LAN viewer
  keeps today's copy-path behaviour.
- Dashboard daemon route plus asset change: compatibility tier T0 (additive
  route; no store or protocol change).

## Open Decision Queue

Last ID: 428

(424) [open] [technical] The open capability is minted on click, not at render - board and reply regions are streamed to LAN viewers, so a render-time capability would leak; a loopback-only same-origin route checks the path is an outside link present in the session's board or replies before minting.
(425) [open] [technical] "Too wide" folders (/, $HOME, depth <= 1 under / or $HOME, /tmp) shrink to the file alone; symlinks are resolved and the real path must stay beneath the granted folder - over trusting the link's lexical path, which a symlink escapes.
(426) [open] [technical] HTML opens through a sandboxed route like fs/cap, and text, markdown and images through the existing preview card, both under the same capability - over a new outside-file renderer.
(427) [open] [policy] Dot segments under a granted folder.
- Context: a granted folder may contain hidden paths; some lead links point into dot folders on purpose (~/.cache/...).
- Alternative a: deny dot segments only below the granted folder, plus a fixed always-deny list (~/.ssh, ~/.gnupg, ~/.aws, ~/.config/gh, ~/.netrc, ~/.docker).
- Alternative b: deny any dot segment anywhere in the path (breaks ~/.cache links).
- Alternative c: no dot rule; the granted folder is the only bound.
> Recommend a: deliberate links into dot folders keep working while secret homes never open.
(428) [open] [policy] Which outside links open the card.
- Alternative a: board links only (topic bodies and items).
- Alternative b: board links and links in the lead's replies in the conversation view.
> Recommend b: leads point at files in replies more often; the click-time check of (424) covers both.

## Phases

### Phase 1: Outside-root file card and view

Implement per `## Decisions` once the queue settles. Verification: Go
route tests (capability minted only for a loopback same-origin request
naming a link present in the session; LAN and foreign-origin refused; wide
folder shrinks to the file; symlink escape refused), node tests for the
click path, and a headless check of an outside HTML and markdown file
opening in the card.

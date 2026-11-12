---
title: "Dashboard opens outside-root file links on the same machine"
sage-review-design: completed
sage-review-completeness: completed
related:
  261110-feat-ws-dashboard-file-preview: predecessor; owns the in-root preview card, the stateless capability, and the localViewer-only rule for outside reads
  261109-feat-ws-dashboard-open-out-of-root-images: predecessor; owns the viewer-independent outside-image link markup this ticket extends
sage-review-design-reviewed: b32bd5967321e05a
sage-review-completeness-reviewed: b32bd5967321e05a
---

# Dashboard opens outside-root file links on the same machine

## Background

A link the lead writes on the board (or in a reply) to a file outside the
session's git root renders today as a file action link with no `href`
(`tabindex="0"`, `data-wl="file"`, `data-abs`, no `data-rel`); `data-out` marks
only an outside image, whose link carries the image route as its `href`
(`agents-plugin-tool/internal/wswhiteboard/markdown.go#L780-L800`,
`agents-plugin-tool/internal/wswhiteboard/view.go#L108-L124`). A non-image
outside link therefore opens nowhere: its menu offers only copying the
absolute path; an outside image opens in a new tab or the native image app
only on a `data-sysopen` page, and "open in system browser" is never offered
for a `data-out` link
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2105-L2145`).
The user wants such files to open in the dashboard's own file
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
- (424) The open capability is minted on click, not at render: a
  loopback-only, same-origin route checks that the path is an outside link
  present in the session's board or lead replies, then mints the capability
  for the granted scope. Rejected: minting at render, because board and
  reply regions are streamed to LAN viewers, so the capability would leak.
- (425) "Too wide" folders grant only the linked file: `/`, `$HOME`, any
  folder at depth <= 1 under `/` or `$HOME` (which already covers `/tmp`
  itself; a subfolder such as `/tmp/build` is not too wide). Symlinks are
  resolved and the real path must stay beneath the granted folder.
  Rejected: trusting the link's lexical path, which a symlink escapes.
- (426) HTML opens through a sandboxed route like `/fs/cap`; text, markdown
  and images open through the existing preview card; both use the same
  capability. Rejected: a separate outside-file renderer.
- (427) Dot segments: a hidden path segment below the granted folder is
  refused, and a fixed always-deny list (`~/.ssh`, `~/.gnupg`, `~/.aws`,
  `~/.config/gh`, `~/.netrc`, `~/.docker`) never opens, whatever the link.
  A link whose own folder is inside a dot folder (for example
  `~/.cache/...`) still opens. Rejected: refusing any dot segment anywhere
  (breaks deliberate `~/.cache` links); no dot rule (exposes secret homes).
- (428) Links qualify from the board (topic bodies and items) and from the
  lead's replies in the conversation view. Rejected: board links only.
- (429) Gitignored paths: the explicitly linked file always opens; its
  sibling files in the granted folder follow the in-root refusal
  (`capDenied`: dot-named and gitignored paths refused, decision 138 c).
  Rejected: refusing the linked file itself when ignored (breaks links into
  ignored build folders); no gitignore rule outside the root (serves ignored
  secrets next to the linked file). Accepted cost: an HTML page whose assets
  are all ignored may render without them.
- (440) A dot-named linked file itself (for example `/proj/.env`,
  `~/.bashrc`) opens when explicitly linked, unless it is on the always-deny
  list; its dot-named siblings in the granted folder stay refused under
  (427). The capability's linked-file field (below) carries this exemption
  together with the gitignore one. Rejected: refusing it like any hidden
  segment (the lead linked it deliberately; the read is loopback-only).
- One capability gates the reads for an outside link this ticket opens
  (review, auto-confirmed; the explorer's `root=` reads and the 261109
  inline outside-image route stay as they are): the
  existing preview and raw routes read an outside root today through the
  explorer's `root=` parameter, gated only by `localViewer` and
  `crossSiteFetch`, with no deny list, dot rule, wide-folder shrink, or
  membership check. An outside link's text, markdown, and image reads
  therefore go through a capability-scoped outside read (a sibling
  sub-route or a capability argument on the preview and raw routes), never
  through `?root=<folder>&path=<file>`, and apply (425), (427), and (429)
  exactly as the HTML route does. The card stores the absolute path and
  re-mints through the click route on every load (13d6c3c7). Rejected:
  wiring the outside card through the explorer's `root=` reads (opens
  `~/.ssh/config` from a board link, against (427)).
- Serving an outside capability is loopback-only too (review,
  auto-confirmed): `/fs/cap/` and `/fs/view/` pass `lanGuard` by URL shape,
  and `capRefused` stops only fetches and foreign `Origin` headers, so a
  leaked outside capability would let a LAN host read the granted folder
  until expiry. The serving route answers 404 to a non-`localViewer`
  request bearing an outside capability (412; 261110 "reads outside the
  root stay `localViewer` only"). Rejected: enforcing loopback at the mint
  route only.
- Capability shape (review, auto-confirmed): an outside capability uses
  route parts distinct from both in-root parts (`fsCapRoutePart`,
  `fsViewRoutePart`), so neither verifies on the other's routes; whether it
  shares the `/fs/cap/` and `/fs/view/` URL prefixes or takes new ones is
  the implementer's call, as long as its serving handler checks
  `localViewer`; its URL carries the granted root and the linked file, both
  covered by the MAC, so the serving route exempts only that file from the
  gitignore refusal (429) and the dot-name refusal (440). The wide-folder (425) and always-deny (427)
  checks run on the symlink-resolved path, and the always-deny check also
  runs on the lexical path as written, as in-root `dotDenied` does, so a
  `~/.ssh/...` link never opens wherever it resolves. On a Windows host,
  `/` in (425) means a volume root (`C:\`). Accepted: a link into a large
  mount below depth 1 (for example `/mnt/c/notes.txt` on WSL) grants that
  folder under (425); exposure stays loopback-only.
- Outside file view (review, auto-confirmed): the view tab covers outside
  HTML only, reached from a loopback-only "open in new tab" link-menu item
  that POSTs the mint and opens the returned outside view URL; text,
  markdown, and images open in the card only (426). An expired outside view
  never mints on a GET: the navigation lands on a daemon-origin,
  loopback-only page that re-POSTs the mint for the stored absolute path
  and reloads, or shows "no longer available; reopen from the board" when
  the mint is refused. Rejected: a GET mint (weakens (424)'s POST and
  `Origin` check); the in-root `FileSrc` redirect (in-root only).
- A card whose re-mint is refused (its link aged out of the live ring, or
  the board item is gone) shows a clear "no longer available" state, not a
  broken frame (review, auto-confirmed).
- The mint route's request and answer fields and its refusal status codes
  are left to planning; the route is dashboard-internal and T0 (review,
  auto-confirmed).
- Membership (review, auto-confirmed): the mint route matches the target
  the renderer resolved for the link (`FileActions` `Abs`, including
  relative and `~` forms), not raw markdown text. It is a POST, like
  `fs/act`, so the same-origin check reads a reliable `Origin`. A link in a
  lead reply that has aged out of the bounded live ring is refused at mint
  even if still on screen; that is accepted.
- Entry point (review, auto-confirmed): an outside non-image link opens the
  card from the same click and menu entry an in-root file link uses; an
  outside image link moves from today's new-tab/system-app open to the
  preview card per (426), its existing menu items kept on a `data-sysopen`
  page.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 25d09166 (2026-11-11, commit): "User decision (138) c: the preview capability keeps the whole session root but refuses dot-named and gitignored paths; a previewed file the rule refuses shows as source." — bearing: constrains
- 445e0769 (2026-11-12, commit): "The cap route lost fetch (connect-src prefix -> 'none') because its own fetch is the same request shape as the attacker's ... Module scripts, fonts, and crossorigin style sheets keep working via ACAO \"null\"" — bearing: constrains
- 7f890500 (2026-11-10, commit): "The capability is stateless: expiry plus an HMAC over reply-id, project root, and expiry under a key drawn at daemon start. ... Rejected a minted-token table (unbounded under repeated GETs)." — bearing: supports
- 7f890500 (2026-11-10, commit): "The capability response sandboxes itself (CSP \"sandbox allow-scripts\"), not only the card's iframe attribute: the same URL opened in a tab of its own would otherwise run workspace HTML in the daemon's origin." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, Decisions): "LAN token holders browse read-only inside the root ... reads outside the root stay `localViewer` only." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, commit 13d6c3c7): "A card stores the file's path, never its capability URL: every load asks the preview route again and gets a fresh capability." — bearing: constrains
- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, Decisions): "Link and image markup stays viewer-independent. An out-of-root image link always carries its route `Href` plus a marker attribute; the page's JS offers the open items ... only when the page is marked loopback" — bearing: constrains
- 756bf63d (2026-11-12, commit): "File card: (411)=b folder scope, (412)=a same machine only, (413) ticket; queue items posted on the board topic file-card." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdashboard/fs_capability.go, wsdashboard/fs_routes.go, wsdashboard/assets/dashboard.js, wsdashboard/assets/preview.js, wswhiteboard/markdown.go, plus a new mint route |
| scope.surface | public-interface | new loopback-only same-origin daemon HTTP route for minting an outside-file capability; link menu behavior change |
| scope.new_public_symbol | yes | a new daemon route under /s/reply-id/ for the click-time mint; Go helpers stay unexported |
| scope.new_type_contract | yes | the mint route's request and answer shape, and a capability whose root is the granted outside folder or file |
| scope.test_surface | existing | wsdashboard/fs_capability_test.go, wsdashboard/fs_routes_test.go; JS paths are tested under node from Go tests such as drawer_height_test.go |
| complexity.reuse_points | confirmed | mintCapability, validCapability, handleFSCapability, capRefused, dotDenied in fs_capability.go; preview sub-route in fs_routes.go#L422; window.WsPreview |
| complexity.side_effect_risk | moderate | touches the shared capability route and link menu that in-root previews and outside images already use |
| risk.correctness | high | wide-folder rules, symlink resolution and containment, dot-segment and always-deny lists, and board/reply link membership must all be exact |
| risk.fit | moderate | must extend the root-scoped HMAC capability to an outside root without weakening the in-root rules from decision 138 c |
| risk.test | moderate | needs Go route tests, node click-path tests, and a headless HTML and markdown check |
| risk.security_or_contract | high | grants browser reads of files outside the session root; loopback, same-origin, and leak-at-render boundaries are security-critical |

## Phases

### Phase 1: Outside-root file card and view

Implement per `## Decisions`. Verification:

- Go route tests: a capability is minted only for a loopback same-origin
  POST naming a link present in the session's board or lead replies (428);
  LAN and foreign-origin mints are refused; a LAN peer holding a valid
  outside capability gets 404 from the serving routes; a wide folder shrinks
  to the file (425); a symlink escape is refused; every always-deny path
  (427) is refused whatever the link; a hidden segment below the granted
  folder is refused while a link inside a dot folder opens, both for a
  depth-1 folder whose grant shrinks to the file (`~/.cache/x.txt`) and
  for a folder grant inside a dot folder (`~/.cache/proj/x.html`); a
  lexical always-deny link that resolves elsewhere is refused; a gitignored linked file opens while its
  gitignored siblings are refused (429); a dot-named linked file opens
  while its dot-named siblings are refused and an always-deny path stays
  refused (440); text, markdown, and image reads of
  an outside link apply the same refusals as the HTML route; an outside
  capability does not verify on in-root routes and the reverse; an expired
  outside view re-mints through the POST page and never through a GET.
- Node tests for the click path (outside non-image link and outside image
  link open the card; the card re-mints on each load and shows "no longer
  available" when the mint is refused; the HTML "open in new tab" item
  POSTs the mint and opens the view; a LAN page keeps copy-path).
- Headless check of an outside HTML file (relative CSS and images resolve)
  and an outside markdown file opening in the card; relative images and
  links in outside markdown render as path text, as out-of-root markdown
  does today, so the check does not expect them to resolve.

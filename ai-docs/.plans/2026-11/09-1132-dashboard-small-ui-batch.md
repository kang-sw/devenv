# Plan: dashboard small UI batch (four user-feedback items)

Four independent, small user-feedback fixes on the ws dashboard and
whiteboard renderer, all confirmed by the user on 2026-11-09. Land each as its
own commit on `develop` in the main checkout (the plan file rides the first
commit). Do not push. Never run `git stash`.

## 1. Out-of-root image route: allow `Sec-Fetch-Site: none`

Context: ticket `261109-feat-ws-dashboard-open-out-of-root-images` (merged in
1b06ffd4b) made the out-of-root image fallback in
`agents-plugin-tool/internal/wsdashboard/image_route.go` refuse with 404 when
`Sec-Fetch-Site` is present and not `same-origin`. In practice this 404s when
the user reloads a tab opened from the link menu's "new tab", uses the
browser's own "open image in new tab", or pastes the URL: those carry
`Sec-Fetch-Site: none`.

User decision (whiteboard (14) b): also accept `none`. Accepted values become
absent, `same-origin`, or `none`; `same-site` and `cross-site` stay 404.
Everything else is unchanged: loopback viewer only, the
`Cross-Origin-Resource-Policy: same-origin` header, and the path checks.
Rationale: `none` is a user-initiated navigation, not another site embedding
or fetching the image, and the loopback check still applies on the server.

- Update the check, its doc/trap comment, and the tests that pin the 404
  matrix (add a `none` -> 200 case; keep `same-site`/`cross-site` -> 404).
- Update the matching wording in `ai-docs/manuals/ws-mcp.md` if it states
  the rule.
- Do not touch the system-open route's origin checks.

## 2. Whiteboard markdown: a code span that is exactly one path becomes a path link

Problem: in board item fields and replies, a path written in backticks, e.g.
`` `ai-docs/paged/261109-tracker-intro/document.html` ``, gets no link or
highlight, while the same path written bare does. The reason is
`agents-plugin-tool/internal/wswhiteboard/bare_links.go`: code spans, code
blocks, and raw HTML keep bare URLs/paths literal by design.

User decision (whiteboard (12) a): when a code span's **whole** content
(trimmed) is a single path that the bare-path rule would link (the same
`barePath` regex and the same served-target/containment resolution a bare
path gets), render it exactly as the bare path renders (the
`<a ...><code class="fpath">path</code></a>` form, `writePathCode` or its
equivalent) instead of a plain `<code>`. Anything else in a code span stays
literal: a code span holding a path plus other text, a URL (leave URLs alone
unless the existing bare-URL rendering for code spans is trivially the same
path; do not add URL linking), code blocks, and raw HTML.

- The path-link rules stay those of the bare path in the same context: where
  a bare path would not be linked (unserved, out of root, topic body vs item
  field differences, the out-of-root image hook added by the ticket above),
  the code span stays a plain `<code>`. Reuse the existing resolution; do not
  add a second resolver.
- Tests in `markdown_test.go` (or the bare-links tests): a whole-path code
  span links; a code span with a path plus other words stays literal; a fenced
  code block holding a path stays literal; an unresolvable path in a code span
  stays a plain `<code>`.

## 3. Sent/held dim also dims the item title, lightly

Problem: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`
(~:328-331) dims a sent (`.sent-sent`, opacity .4) or held (`.sent-held`,
.65) board item's answer area, but deliberately leaves the head, so the title
(the largest text) stays full brightness and the dim barely shows.

User request: the title should dim a little too. Add a lighter dim to the
title only (`.card.sent-* .cbar .head .title` and
`.arow.sent-* .head .title`; markup is in
`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl` `policy`,
`assumed`, `ack`): about .7 for sent and .85 for held. The ID chip, the class
tag, and the `.sentmark` stay full brightness. The existing `:focus-within`
restore must also restore the title. Update the comment above the rules.

## 4. Peer lead rows show `<folder>/<address>`

Problem: the peer lead list (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`
`peerrow`) labels a peer by its mailbox address only (`id:<reply-id>` or a
named `slug@scope`). The user says the key information is which folder the
peer resides in, and wants the label shaped `<folder>/<address>`.

- Data: a served peer has `Session.Project`
  (`agents-plugin-tool/internal/wsdashboard/pageview.go` `peerGroup`, ~:400);
  an unserved one has a cwd in the mailbox records: `wsmailbox.ReplyEntry.Cwd`
  for an `id:` address, `wsmailbox.Presence.Cwd` for a named address, and
  `wsmailbox.Contact.Cwd` as known at the exchange.
  `agents-plugin-tool/internal/mcp/dashboard_page.go` (~:187-210) builds
  `wsdashboard.Contact`; add the cwd there (prefer the live registry/presence
  cwd, fall back to the contact record's) and carry it into `PeerRow`
  (served peer: its Project wins).
- Folder = the last path element, split on both `/` and `\` (a Windows cwd
  can be seen from a non-Windows daemon), with trailing separators ignored.
- Render the folder prefix plus `/` in a muted style before the address in
  the same `.pname` element (link or span), with the full path in its
  `title`. No known folder -> the address alone, as now.
- Tests: the PeerRow folder for a served peer, an unserved `id:` peer, a named
  peer, a Windows-style path, and no cwd; the template output for one row.

## Verification

- `cd agents-plugin-tool && go test ./internal/wsdashboard/ ./internal/wswhiteboard/`
  green, plus `go test ./internal/mcp/ -run Dashboard` (or the narrowest set
  covering `dashboard_page.go`). Run `go test -timeout 25m ./internal/mcp/` once
  at the end if you touched `mcp/`.
- No JS change is expected; if you change JS, `node --check` it.
- Report per item: commit SHA, what changed, tests added, anything unverified.

## Boundary

If an item turns out to need a design decision not settled above (for
example, item 2's reuse of the resolver is not possible without
restructuring), stop on that item, finish the others, and report.

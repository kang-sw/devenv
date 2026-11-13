---
title: "Dashboard web card: proxied localhost page with element pick and console-error attach"
sage-review-design: required
sage-review-completeness: required
related:
  261113-feat-ws-dashboard-iframe-shell: "frames session pages under ?frame=1; its composed session-page CSP does not apply here, because decision 19 keeps the session page at frame-src 'self'"
  261113-feat-ws-ticket-visual-reference-and-ask-classes: "co-edits the whiteboard playbook and mirrors"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "co-edits the whiteboard playbook and mirrors"
---

# Dashboard web card: proxied localhost page with element pick and console-error attach

## Background

Owner request (2026-11-13, dashboard UX side track; whiteboard topic
`web-pane`, items (101)-(104), (110)-(116)). The user and the agent should look
at the same web page inside the dashboard while doing web development: the
user opens a localhost dev-server page in a dashboard card, points at an
element ("this button"), and the pointer reaches the agent as part of the
message. The value the dashboard alone can give is the shared view plus
pointing; reading a page deeply is already covered by each harness's own
browser tools (Claude in Chrome, Playwright / chrome-devtools MCPs).

Current state (research, 2026-11-13):

- `assets/cards.js` has two card kinds: file preview (`preview.js`, sandboxed
  iframe for HTML files under `connect-src 'none'`, so a dev-server page with
  HMR or API calls cannot run there) and terminal (`terminal.js`). There is no
  URL card.
- Cards sit in the board column among topics and can be pinned into the right
  column (`aside#bpcol`). They open only from the UI (file-link click,
  "+ Terminal" button). Card state lives in browser localStorage
  `ws.dashboard.cards.<rid>`.
- The board-selection path (`hostMention` / `addMention` / `bmentions` in
  `dashboard.js`) attaches `<wb:mention>` elements to the composed message;
  a sandboxed frame forwards its selection to the host by postMessage
  (`wbDoc:"sel"`).
- The session page CSP has `frame-src 'self'`. The daemon has no reverse
  proxy. Access guards `localViewer` and `sameOrigin` exist.

## Decisions

1. **Scope: reduced variant, no new agent tools** ((110)=b). The card plus
   element pick plus console-error attach; zero new MCP tools.
   Rejected: a `browser.*` MCP tool family (snapshot / reload / navigate) -
   duplicates harness browser tools and adds schema weight to every session's
   tool list; can be added later as T0 if a need appears. Rejected: defer or
   drop the feature.
2. **A third card kind in the existing card framework** ((103), (104)
   answered: "the right card panel as is; the web card is one of the cards -
   treat this as the principle"). It follows the existing placement rules
   (board column, pinnable to `aside#bpcol`).
   Rejected: a separate split pane beside the conversation; a panel inside a
   whiteboard topic; a new browser tab.
3. **Rendering: iframe + daemon reverse proxy + injected script** ((101)=c as
   revised by (111)=a). The proxy serves the target page with a small
   injected script that does element pick and console-error collection.
   Headless Chromium / CDP screenshots are out of scope until exact
   screenshots are needed.
4. **The proxy runs on a different origin from the dashboard** ((111)=a,
   revising the same-origin part of (101); the listener layout and framing
   are settled by (522), decision 19). The dev
   page runs unrestricted (its own cookies / localStorage work) while the
   origin boundary keeps it away from dashboard routes (terminal, message
   send). The injected script talks to the dashboard only by postMessage.
   Each target gets its own loopback listener and the card frames it
   through a wrapper route; the session page CSP is unchanged (decision 19).
   Rejected: same origin + `sandbox` (breaks dev apps' localStorage and
   cookies); same origin without sandbox (a dev page script could call daemon
   routes with the dashboard's credentials).
5. **Targets: localhost dev servers and workspace static files only** ((102)=a,
   (116)). Static serving is limited to the opened file's directory subtree,
   read-only, and the proxy origin carries no ws capability or cookie
   ((502)=a, owner 2026-11-13). Rejected: serving the whole session root
   (the exposure the 261110 file-preview Result flagged: a page could read
   any file under the root and carry it to another origin).
6. **Proxy access: same-machine viewer only** ((113), announced): the
   `localViewer` rule the terminal card uses; LAN viewers cannot open or use
   the proxy. A localhost proxy is an SSRF / execution surface.
7. **Element pick attaches a new element, not `<wb:mention>`** ((114),
   technical): `<wb:pick url="..." selector="...">text</wb:pick>`, added to the
   composed message the way board mentions are. `wb:mention` requires a topic
   key and a numeric `id`; forcing a URL into `topic` blurs its meaning. The
   whiteboard playbook's Answers section gains one line describing
   `wb:pick` (additive, T0).
8. **Console-error attach in phase 1** ((112)=a): the card shows an error-count
   badge; an "attach console errors" action adds the collected errors to the
   composed message. The agent cannot read the console itself in the reduced
   variant, so this is the substitute path.
9. **Opening a web card** ((115), technical): a "+ Web" button next to
   "+ Terminal" (URL input), and a click on a localhost link in the
   conversation or board opens a web card, matching file-link -> file-card.
   The agent asks the user to look at a page by writing a localhost link.
10. **Card persistence** ((116), announced; detailed by the completeness
    review 2026-11-13, autonomous): stored per session in browser
    localStorage like the existing cards. A dev-server card stores the
    target `{url}` (never the ephemeral proxy URL); a static-file card stores
    the workspace `{path}` and re-registers it on restore. A static
    registration lasts for the daemon's lifetime; its listener is recreated
    on demand, so a restored card works after a daemon restart.
11. **Compatibility tier: T0** - dashboard asset content, an additional daemon
    listener, one additive whiteboard playbook line, and additive optional
    `wsinbox.Entry` fields plus drain description text (decision 16).
12. **Console errors attach as `<wb:console url="...">one error per
    line</wb:console>`** ((128), technical, acknowledged): mirrors `wb:pick`
    (decision 7) so the agent sees the source URL, and the playbook line
    documents both elements together.
    Rejected: an untyped text paste.
13. **Proxy wiring and hardening** ((504), technical, owner-acknowledged
    2026-11-13; header and CSP parts revised by (520) and (522)): the
    loopback allow-list refuses the daemon's own ports. Header stripping per
    decision 17; CSP wiring per decision 19. Rejected: forwarding headers
    unchanged.
14. **Proxy response hardening** (design review 2026-11-13, autonomous): the
    proxy strips the target's `X-Frame-Options` and CSP `frame-ancestors` and
    sets its own `frame-ancestors` to the dashboard origin, so a dev page
    renders in the card and no foreign site can frame the proxy. The injected
    script posts only to the dashboard origin (never `*`), and the host
    checks `event.origin` and `event.source` against the card's frame.
    Rejected: passing target framing headers through (dev servers that send
    `DENY` would never render); a `*` targetOrigin (a foreign framer would
    receive picks and console errors).
15. **Proxy mechanics** (completeness review 2026-11-13, autonomous):
    WebSocket upgrades pass through (HMR); the script is injected only into
    `text/html` responses; a redirect to a target off the allow-list is
    refused; listeners bind ephemeral loopback ports, start lazily on the
    first web card, and a bind failure shows in the card.
16. **Pick and console entries in the inbox** (design review 2026-11-13,
    autonomous): board mentions are a structured `wsinbox.Entry.Mentions`
    field (`wsinbox/mentions.go`, `inbox.go:178`, Merge at `:580`, cap at
    `:369`) persisted with held messages and described in the
    `dashboard.inbox_drain` JSON description (`mcp/server.go:5243`). "Added
    the way board mentions are" therefore means new optional `Entry` fields
    for picks and console lines, message-route parsing, Merge, restore, and
    an updated drain description, with console lines capped like mentions.
    Still T0 (additive optional fields and description text).
17. **Header stripping** ((520)=a, owner 2026-11-13): the proxy strips only
    the dashboard's named cookies (theme, lang, and the LAN token while it
    exists: `theme.go`, `lang.go`, `access.go:109`) and forwards the dev
    app's own cookies; it strips `Authorization` only when it carries the
    dashboard's bearer, otherwise forwards it. Rejected: (b) stripping all
    `Cookie` and `Authorization` (breaks cookie- and header-auth dev apps,
    contradicting decision 4).
18. **Static-file entry and directory binding** ((521)=a, owner
    2026-11-13): two entry points open a workspace static file in a web
    card: an "Open as web card" action on HTML file cards and HTML file
    links, and a workspace path typed into the "+ Web" input. Both go
    through a dashboard route guarded like decision 6 (`localViewer` +
    `sameOrigin`) that registers the file's directory with the daemon; the
    proxy serves only registered directories, read-only, confined to that
    subtree. The registration is a daemon-side binding, not a ws capability:
    it grants read access to that subtree and nothing else. Rejected: (b)
    the file-card action only; (c) dropping static files.
19. **Per-target listeners and a wrapper frame** ((522)=a, owner
    2026-11-13): one ephemeral loopback listener per target (a dev-server
    port, or a registered static directory). Each web card's iframe loads a
    small same-origin wrapper route whose own CSP sets `frame-src` to that
    card's proxy origin; the session page CSP stays `frame-src 'self'`, so
    `sessionPageCSP` and `TestSessionPageFramesOnlyItsOwnOrigin` do not
    change for this ticket and the CSP composition noted in
    `261113-feat-ws-dashboard-iframe-shell` does not apply to it. The
    wrapper relays postMessage between the card host and the proxied page
    (decision 14's origin checks apply at both hops). Rejected: (b) the
    session page allowing `frame-src http://127.0.0.1:*`; (c) one port with
    `<port>.localhost` Host routing (`loopbackHost`, `daemon.go:690`, accepts
    only exact names).
    The wrapper route and the decision 18 registration route are both
    guarded like decision 6 (`localViewer` + `sameOrigin`; completeness
    review 2026-11-13, autonomous).

## Constraints

- Shipped surfaces: the `wb:pick` (and console attach) description lands in
  the shared whiteboard playbook; read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, and `ai-docs/manuals/wsflow-mirroring.md`
  before editing it. Daemon code under `agents-plugin-tool/internal/` follows
  `ai-docs/manuals/release-compatibility.md`.
- Never restart or replace the user's running dashboard daemon for checks; use
  a separate dev instance or a rendered fixture.
- Mirrors: the whiteboard playbook line lands in
  `agents-plugin/rsrc/whiteboard/whiteboard.md` (Answers section, after the
  `wb:mention` example at `:340`) and needs its wsflow mirror
  (`agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`) and its pi mirror
  (`agents-plugin-pi/rsrc/whiteboard/whiteboard.md`), with manifests
  synchronized. Overlap: `261113-feat-ws-ticket-visual-reference-and-ask-classes`
  Phase 2 edits the same playbook (its `class` bullet at `:85-91`); the two
  edits are in different sections but land in the same file and mirrors, so
  whichever lands second rebases onto the other's manifest hashes. The same
  holds for `261112-feat-ws-whiteboard-topic-tickets-sticky-head`, which
  documents its `tickets` attribute in the same playbook and mirrors.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 445e0769 (2026-11-12, commit): "frame-src 'self' verified headless: the preview frame's self-navigation to another origin is blocked" - decision 19 keeps the session page at `frame-src 'self'`, and each wrapper route's `frame-src` names only its own card's proxy origin — bearing: constrains
- 72c8e9d2 (2026-11-07, commit): "A framed session page's own script posts carry the daemon Origin, so the same-origin/loopback guards ... all pass a clickjacked click. Refusing the frame is the only guard that works there." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-11, Result): "A previewed HTML file can read any file under the session root through its capability, and CSP does not govern a frame navigating itself, so such a page could carry what it read to another origin." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, Decisions): "HTML and SVG load in an iframe with `sandbox="allow-scripts"` and no `allow-same-origin`, from a preview route whose path carries a short-lived, root-scoped read capability" — bearing: constrains
- 69d028fe (2026-11-12, commit): "A sandboxed frame satisfies localViewer: capabilityURL returns a bare path, so the frame and its sub-resources resolve against the session page's origin" — bearing: constrains
- 94fd6845 (2026-11-12, commit): "cards.js settle moves existing nodes with moveBefore (iframe state kept), new nodes and refused/missing moveBefore use insertBefore" — bearing: constrains
- 06195171 (2026-11-13, commit): "Reload calls the card's own load(): asks the preview/outside route again, mints a fresh capability and redraws." — bearing: supports
- 260514-feat-ws-web-daemon-foundation (2026-05-14, Decisions): "Treat loopback binding as a reachability limit, not an authorization boundary. Localhost access still requires owner authentication." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard.js, pages.go, a new proxy listener, agents-plugin-tool/internal/wsinbox (Entry fields, Merge, restore), the dashboard.inbox_drain description in internal/mcp/server.go, agents-plugin/rsrc/whiteboard/whiteboard.md with wsflow and pi mirrors |
| scope.surface | cross-module | new per-target daemon listeners and proxy, wrapper and registration routes, dashboard assets, shipped whiteboard playbook (the session page CSP is unchanged, decision 19); no exported Go symbol is named |
| scope.new_public_symbol | yes | wb:pick and wb:console message elements (agent-visible grammar); the proxy listener port |
| scope.new_type_contract | yes | injected-script to host postMessage protocol; wb:pick url/selector and wb:console url element grammar |
| scope.test_surface | new-files | existing pages_test.go and access guards cover CSP and localViewer; the proxy needs new Go test files (ReverseProxy search over agents-plugin-tool returned nothing) |
| complexity.reuse_points | confirmed | cards.js kind registry (register/open at cards.js#L839-L896), hostMention/addMention/bmentions chip path (dashboard.js#L1179-L1275), localViewer (access.go#L146-L148) |
| complexity.side_effect_risk | high | new loopback listeners that proxy to arbitrary localhost targets and serve registered workspace directories |
| risk.correctness | high | proxy rewriting and script injection across HMR/websocket dev servers, plus postMessage pick and console capture |
| risk.fit | moderate | follows the card framework, but adds a per-card wrapper route with its own CSP and the iframe-shell ticket also reframes the page (pending 261113-feat-ws-dashboard-iframe-shell) |
| risk.test | high | the pick highlight, injection and proxied page need a browser LOOK capture on a dev daemon; Go tests cover only allow-list, guard, injection, and CSP |
| risk.security_or_contract | high | SSRF and execution surface, unsandboxed dev page on a sibling port, and a new shipped message grammar |

## Phases

### Phase 1: Web card with proxy, element pick, and console-error attach

- Daemon: loopback-only reverse proxy listeners, one per target (decision
  19), for localhost targets and registered workspace static directories, injecting the pick /
  console script; guarded by the same-machine rule.
- `cards.js`: third card kind `web` (iframe on the proxy origin, URL bar,
  reload), "+ Web" button, localhost link click opens it, localStorage
  `{url}` or `{path}` per decision 10.
- Injected script <-> host postMessage protocol: pick mode (hover highlight,
  click -> selector + text), console error collection and count.
- Host: attach `wb:pick` and console errors to the composed message with the
  existing mention-chip UX; error-count badge on the card.
- Wrapper route per decision 19 (the session page CSP stays `frame-src
  'self'`, `pages.go#L1276`, pinned by `TestSessionPageFramesOnlyItsOwnOrigin`).
- Static entry points and directory registration per decision 18.
- Whiteboard playbook Answers section: describe the new element(s).
- Proxy response hardening, mechanics, and inbox fields per decisions 14-19.

Verification: Go tests for the proxy (target allow-list including refusal of
the daemon's own ports, same-machine guard, static serving confined to the
opened file's directory, header stripping per decision 17 (dashboard cookies and bearer stripped, dev-app cookies forwarded), script injected only into
HTML responses, WebSocket upgrade passthrough, off-allow-list redirect
refused, target framing headers replaced by `frame-ancestors` for the
dashboard origin, only registered directories served) and for the wrapper
route's CSP (`frame-src` is exactly that card's proxy origin) with the
session page CSP unchanged; Go tests that `wsinbox` persists,
merges, restores, and caps pick and console entries; a node test of the
postMessage protocol (posts go only to the dashboard origin; the host drops
messages from another origin or another frame); a LOOK capture, on a dev
daemon instance, of a web card showing a proxied local page with HMR active,
an active pick highlight, and the composed message carrying a `wb:pick` chip,
a `wb:console` chip and the error badge; headless checks that a localhost link
click and "+ Web" open a card, reload re-requests the page, a reload of the
session page restores the card from localStorage (a static card also after a
daemon restart, re-registering its path), "Open as web card" on an HTML file
card and on an HTML file link opens a static web card, a workspace path typed
into "+ Web" does the same, and a listener bind failure shows in the card; Go
tests that the registration route and the wrapper route refuse a non-local
viewer and a cross-origin caller.

---
title: "Dashboard web card: proxied localhost page with element pick and console-error attach"
sage-review-design: required
sage-review-completeness: required
related:
  261113-feat-ws-dashboard-iframe-shell: "also changes the session-page CSP and framing; see the Composed session-page CSP decision there"
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
4. **The proxy runs on a separate daemon port (a different origin from the
   dashboard)** ((111)=a, revising the same-origin part of (101)). The dev
   page runs unrestricted (its own cookies / localStorage work) while the
   origin boundary keeps it away from dashboard routes (terminal, message
   send). The injected script talks to the dashboard only by postMessage. The
   session page CSP `frame-src` adds only that proxy origin.
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
10. **Card persistence** ((116), announced): `{url}` stored per session in
    browser localStorage like the existing cards.
11. **Compatibility tier: T0** - dashboard asset content, an additional daemon
    listener, one additive whiteboard playbook line, and additive optional
    `wsinbox.Entry` fields plus drain description text (decision 16).
12. **Console errors attach as `<wb:console url="...">one error per
    line</wb:console>`** ((128), technical, acknowledged): mirrors `wb:pick`
    (decision 7) so the agent sees the source URL, and the playbook line
    documents both elements together.
    Rejected: an untyped text paste.
13. **Proxy wiring and hardening** ((504), technical, owner-acknowledged
    2026-11-13): the proxy port is announced to the session page and the
    session CSP becomes per-request (`frame-src` adds that origin;
    `TestSessionPageFramesOnlyItsOwnOrigin` is updated). The loopback
    allow-list refuses the daemon's own ports. The proxy strips incoming
    `Cookie` and `Authorization` headers before forwarding, because cookies
    are not port-isolated and the dashboard's would otherwise reach the dev
    server. Rejected: a fixed CSP constant (the port is known only at run
    time); forwarding headers unchanged. (The Cookie part is reopened as
    (513); the CSP wiring as (515).)
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

## Open Decision Queue

Last ID: (515)

(513) [open] [policy] Which request headers the proxy strips before forwarding to the dev server (reopens decision 13's Cookie/Authorization strip)
- Decision 4 picked a separate origin so the dev page's own cookies work; decision 13 strips every incoming `Cookie`, so cookies the dev server set through the proxy never come back to it and any cookie-login dev app loops or fails. The rationale (dashboard cookies are not port-isolated) is met by stripping only the dashboard's named cookies (theme, lang, LAN token: `theme.go`, `lang.go`, `access.go:109`; the LAN token goes away with (490)).
- a: strip only the dashboard's named cookies and forward the rest; strip `Authorization` only when it carries the dashboard's bearer, otherwise forward it.
- b: keep decision 13: strip all `Cookie` and `Authorization` (safer, breaks cookie- and header-auth dev apps).
> a: keeps decision 4's promise and still keeps every dashboard credential away from the dev server.

(514) [open] [policy] How a workspace static file opens in a web card, and how the proxy learns the allowed directory
- Decision 5 allows static files confined to the opened file's directory subtree with no ws capability on the proxy origin, but decision 9's entry points (URL input, localhost link click) have no path for a workspace file, and nothing binds the proxy to a directory. A directory encoded in the URL with nothing binding it would let any local page read any directory.
- a: two entry points, an "Open as web card" action on an HTML file card (and on HTML file links) and a workspace path typed into the "+ Web" input; both go through a dashboard route (same guards as decision 6) that registers the directory with the daemon, and the proxy serves only registered directories (a daemon-side binding, not a ws capability: it grants read-only access to that subtree and nothing else).
- b: the file-card action only.
- c: drop static files from scope; web cards target dev servers only.
> a: matches file-link -> file-card from decision 9 and keeps decision 5's confinement on the daemon side.

(515) [open] [policy] How the proxy routes requests per card, and where the proxy origin is allowed in CSP (reopens decision 13's per-request session-page CSP)
- Dev servers (Vite, Next) load root-absolute paths (`/@vite/client`) and open HMR WebSockets to `location.host`, so a path-prefix scheme on one proxy port loses the target, and two cards on different dev servers cannot share one origin's routing. A per-target listener fixes routing, but its port exists only after the session page was served, so a `frame-src` computed when the page was served cannot list it.
- a: one ephemeral loopback listener per target (dev-server port, or a registered static directory per (514)); each web card's iframe loads a small same-origin wrapper route whose own CSP sets `frame-src` to that card's proxy origin, so the session page CSP stays `frame-src 'self'` and the cross-ticket CSP composition with iframe-shell becomes moot for this ticket.
- b: one listener per target, and the session page CSP allows `frame-src http://127.0.0.1:*` (any loopback port, including unproxied dev servers).
- c: one proxy port with `<port>.localhost` Host routing (`loopbackHost`, `daemon.go:690`, accepts only exact localhost names today; browser support varies).
> a: works with mainstream dev servers, keeps the 445e0769 frame-src block on the session page, and confines the relaxation to each card's own wrapper.

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

- 445e0769 (2026-11-12, commit): "frame-src 'self' verified headless: the preview frame's self-navigation to another origin is blocked" - adding the proxy origin to `frame-src` relaxes that block for every frame on the page — bearing: constrains
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
| scope.surface | cross-module | new daemon listener and proxy, dashboard assets, session page CSP, shipped whiteboard playbook; no exported Go symbol is named |
| scope.new_public_symbol | yes | wb:pick and wb:console message elements (agent-visible grammar); the proxy listener port |
| scope.new_type_contract | yes | injected-script to host postMessage protocol; wb:pick url/selector and wb:console url element grammar |
| scope.test_surface | new-files | existing pages_test.go and access guards cover CSP and localViewer; the proxy needs new Go test files (ReverseProxy search over agents-plugin-tool returned nothing) |
| complexity.reuse_points | confirmed | cards.js kind registry (register/open at cards.js#L839-L896), hostMention/addMention/bmentions chip path (dashboard.js#L1179-L1275), localViewer (access.go#L146-L148) |
| complexity.side_effect_risk | high | a new loopback listener that proxies to arbitrary localhost targets and changes the session page CSP |
| risk.correctness | high | proxy rewriting and script injection across HMR/websocket dev servers, plus postMessage pick and console capture |
| risk.fit | moderate | follows the card framework, but CSP becomes per-request and the iframe-shell ticket also reframes the page (pending 261113-feat-ws-dashboard-iframe-shell) |
| risk.test | high | the pick highlight, injection and proxied page need a browser LOOK capture on a dev daemon; Go tests cover only allow-list, guard, injection, and CSP |
| risk.security_or_contract | high | SSRF and execution surface, unsandboxed dev page on a sibling port, and a new shipped message grammar |

## Phases

### Phase 1: Web card with proxy, element pick, and console-error attach

- Daemon: a loopback-only reverse proxy listener on its own port for
  localhost targets and workspace static files, injecting the pick /
  console script; guarded by the same-machine rule.
- `cards.js`: third card kind `web` (iframe on the proxy origin, URL bar,
  reload), "+ Web" button, localhost link click opens it, localStorage `{url}`.
- Injected script <-> host postMessage protocol: pick mode (hover highlight,
  click -> selector + text), console error collection and count.
- Host: attach `wb:pick` and console errors to the composed message with the
  existing mention-chip UX; error-count badge on the card.
- Session page CSP: `frame-src` adds the proxy origin only (today
  `sessionPageCSP` is a constant, `agents-plugin-tool/internal/wsdashboard/pages.go#L1276-L1276`,
  pinned verbatim by `TestSessionPageFramesOnlyItsOwnOrigin`,
  `agents-plugin-tool/internal/wsdashboard/pages_test.go#L441-L451`; a proxy
  origin known only at run time makes the policy per-request and that test
  must change with it).
- Whiteboard playbook Answers section: describe the new element(s).
- Proxy response hardening, mechanics, and inbox fields per decisions 14-16.

Verification: Go tests for the proxy (target allow-list including refusal of
the daemon's own ports, same-machine guard, static serving confined to the
opened file's directory, header stripping per (513), script injected only into
HTML responses, WebSocket upgrade passthrough, off-allow-list redirect
refused, target framing headers replaced by `frame-ancestors` for the
dashboard origin) and for the CSP header; Go tests that `wsinbox` persists,
merges, restores, and caps pick and console entries; a node test of the
postMessage protocol (posts go only to the dashboard origin; the host drops
messages from another origin or another frame); a LOOK capture, on a dev
daemon instance, of a web card showing a proxied local page with HMR active,
an active pick highlight, and the composed message carrying a `wb:pick` chip,
a `wb:console` chip and the error badge; headless checks that a localhost link
click and "+ Web" open a card, reload re-requests the page, a reload of the
session page restores the card from localStorage, and a listener bind failure
shows in the card.

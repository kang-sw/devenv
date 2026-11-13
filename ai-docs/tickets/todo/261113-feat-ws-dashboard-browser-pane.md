---
title: "Dashboard web card: proxied localhost page with element pick and console-error attach"
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
   (116)).
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
    listener, and one additive whiteboard playbook line.
12. **Console errors attach as `<wb:console url="...">one error per
    line</wb:console>`** ((128), technical, acknowledged): mirrors `wb:pick`
    (decision 7) so the agent sees the source URL, and the playbook line
    documents both elements together.
    Rejected: an untyped text paste.

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
  whichever lands second rebases onto the other's manifest hashes.
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
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard.js, pages.go, a new proxy listener, agents-plugin/rsrc/whiteboard/whiteboard.md with wsflow and pi mirrors |
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

Verification: Go tests for the proxy (target allow-list, same-machine guard,
script injection) and for the CSP header; a LOOK capture of a web card
showing a proxied local page, an active pick highlight, and the composed
message carrying a `wb:pick` chip and the error badge.

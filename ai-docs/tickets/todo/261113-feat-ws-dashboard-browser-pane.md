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
- Session page CSP: `frame-src` adds the proxy origin only.
- Whiteboard playbook Answers section: describe the new element(s).

Verification: Go tests for the proxy (target allow-list, same-machine guard,
script injection) and for the CSP header; a LOOK capture of a web card
showing a proxied local page, an active pick highlight, and the composed
message carrying a `wb:pick` chip and the error badge.

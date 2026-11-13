---
title: "Dashboard web card: proxied localhost page with element pick and console-error attach"
sage-review-design: completed
sage-review-completeness: completed
related:
  261113-feat-ws-dashboard-iframe-shell: "frames session pages under ?frame=1; its composed session-page CSP does not apply here, because decision 19 keeps the session page at frame-src 'self'; the wrapper route is an explicit exception to its (503) rule that every non-?frame=1 response keeps frame-ancestors 'none' / DENY (decision 20)"
  261113-feat-ws-ticket-visual-reference-and-ask-classes: "co-edits the whiteboard playbook and mirrors"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "co-edits the whiteboard playbook and mirrors"
sage-review-design-reviewed: 802beb765812f714
sage-review-completeness-reviewed: 802beb765812f714
completed: 2026-11-14
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
9. **Opening a web card** ((115), technical; static entry points added by the
   UX lead review 2026-11-13, technical): a "+ Web" button next to
   "+ Terminal" (URL or workspace-path input), and a click on a localhost link
   in the conversation or board opens a web card, matching file-link ->
   file-card. A localhost link is an `http` or `https` link whose host is
   exactly `localhost`, `127.0.0.1`, or `[::1]` (the `loopbackHost` set,
   `daemon.go:690`); a link to the daemon's own port (`location.port`) falls
   back to ordinary link behaviour, and a port the daemon refuses
   (decision 13) shows the refusal in the card. The agent asks the user to look at a page by writing a localhost
   link. The static entry points are decision 18's: an "Open as web card"
   action on HTML file cards and HTML file links, and a workspace path typed
   into the "+ Web" input.
10. **Card persistence** ((116), announced; detailed by the completeness
    review 2026-11-13, autonomous): stored per session in browser
    localStorage like the existing cards. A dev-server card stores the
    target `{url}` (never the ephemeral proxy URL); a static-file card stores
    the workspace `{path}` and re-registers it on restore. A static
    registration lasts for the daemon's lifetime; its listener is recreated
    on demand, so a restored card works after a daemon restart.
11. **Compatibility tier: T0** - dashboard asset content, additional daemon
    listeners, one additive whiteboard playbook line, and additive optional
    `wsinbox.Entry` fields plus drain description text (decision 16). The
    listener registry (decision 23) and the registered directories (decision
    26) live in daemon memory only: no new store, no on-disk shape, no
    `schema_version` change (UX lead review 2026-11-13, technical).
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
    sets its own `frame-ancestors` to the dashboard origin (every loopback
    name the viewer may use, decision 21), so a dev page renders in the card
    and no foreign site can frame the proxy. Only the `frame-ancestors`
    directive is removed, from every `Content-Security-Policy` and
    `Content-Security-Policy-Report-Only` header (several headers, several
    directives each); every other directive is kept, and the proxy appends
    its own `Content-Security-Policy: frame-ancestors ...` header. The injected
    script posts only to the dashboard origin (never `*`), and the host
    checks `event.origin` and `event.source` against the card's frame.
    Rejected: passing target framing headers through (dev servers that send
    `DENY` would never render); a `*` targetOrigin (a foreign framer would
    receive picks and console errors).
15. **Proxy mechanics** (completeness review 2026-11-13, autonomous):
    WebSocket upgrades pass through (HMR); the script is injected only into
    `text/html` responses; a redirect to any origin other than the
    target's own is refused, another allow-listed localhost origin included
    (decision 25); listeners bind ephemeral loopback ports, start lazily on the
    first web card, and a bind failure shows in the card.
16. **Pick and console entries in the inbox** (design review 2026-11-13,
    autonomous): board mentions are a structured `wsinbox.Entry.Mentions`
    field (`wsinbox/mentions.go`, `inbox.go:178`, Merge at `:563`, cap at
    `:369`) persisted with held messages and described in the
    `dashboard.inbox_drain` JSON description (`mcp/server.go:5243`). "Added
    the way board mentions are" therefore means new optional `Entry` fields
    for picks and console lines, message-route parsing, Merge, restore, and
    an updated drain description, with the caps of decision 30. The new
    fields join the empty-entry check (`inbox.go:475`, so a pick-only or
    console-only message is accepted, not `ErrEmpty`) and the command check
    (`inbox.go:498`, so a command carrying picks or console lines is
    `ErrInvalid`). Still T0 (additive optional fields and description text).
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
    the file-card action only; (c) dropping static files. Registration
    mechanics and path checks: decision 26.
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
    The wrapper route and the decision 18 registration route have different
    guards, because a plain GET navigation carries no `Origin`: decision 20.
20. **Wrapper route guard and framing** (design review 2026-11-13,
    autonomous; replaces the completeness-review sentence that guarded both
    routes "like decision 6"):
    - The card's iframe loads the wrapper with a plain GET navigation, which
      sends no `Origin`, and `sameOrigin` returns false on a missing Origin
      (`inbox_routes.go:141-145`); 6c5fe1f2 made the worktree preview a POST
      for this reason. So the wrapper GET is gated by `localViewer`
      (`access.go:146`) plus `Sec-Fetch-Site: same-origin` and
      `Sec-Fetch-Dest: iframe`, not by `sameOrigin`. A non-local viewer
      answers 404, a request failing the Sec-Fetch checks (cross-site, any
      other destination, or the headers absent) answers 403, matching the
      `TrustedWrite` pair.
    - The decision 18 registration route stays a script POST gated by the
      existing `TrustedWrite` (`access.go:156`: `localViewer` -> 404,
      `sameOrigin` -> 403), not a new helper.
    - The wrapper response sends `frame-ancestors 'self'` and
      `X-Frame-Options: SAMEORIGIN`. It is an explicit exception to
      `writeHTML`'s `frameAncestorsNone` / `DENY` (`pages.go:1265-1290`) and
      to the iframe-shell ticket's (503) "every non-`?frame=1` response keeps
      `'none'` / `DENY`". `writeHTML` cannot carry it as is: it sets
      `X-Frame-Options: DENY` unconditionally (`pages.go:1289`), so the
      wrapper writes its own headers. Clickjacking stays contained because
      the wrapper has no write controls and foreign origins still cannot
      frame it (the 72c8e9d2 hazard is a framed page whose own script
      posts).
    Rejected: gating the wrapper GET by `sameOrigin` (a plain navigation
    sends no Origin, so the card would always be refused); the `writeHTML`
    headers (the session page's own iframe of the wrapper would be refused);
    a new guard helper for registration.
21. **Proxy `frame-ancestors` covers every loopback name** (design review
    2026-11-13, autonomous): the viewer may reach the dashboard as
    `localhost:P`, `127.0.0.1:P`, or `[::1]:P` (`loopbackHost`,
    `daemon.go:690`), and every ancestor of the proxied page (the wrapper,
    the session page, the iframe shell) is the dashboard origin under that
    name. Each proxy listener therefore sends
    `frame-ancestors http://localhost:P http://127.0.0.1:P http://[::1]:P`
    with P the daemon's own port (`Endpoint().Port`). Rejected: passing the
    wrapper's origin to the listener at registration (per-listener state
    that a viewer switching loopback names would outrun, and a request-derived
    value written into a header, for no isolation gain: every name reaches
    the same daemon). The daemon binds `127.0.0.1` only outside LAN mode
    (`bindHostFor`, `daemon.go:463`), so the `[::1]` entry covers
    `loopbackHost` parity rather than a reachable address today.
22. **Proxy URLs use the dashboard page's own hostname** ((530) technical,
    MAIN lead 2026-11-13; the owner's cookie answer is decision 27): the card builds a proxy URL from
    `location.hostname`, not a fixed `127.0.0.1`, so `localhost` and
    `127.0.0.1` viewers each get a proxy origin on the same host name; ports
    do not count for same-site, so the card and its proxy stay same-site
    (a dev app's `SameSite` cookies ride in the frame). The wrapper derives
    the proxy origin for its `frame-src` and inner frame from its own
    request's `Host` hostname (equal to `location.hostname`, joined with
    `net.JoinHostPort` for IPv6), first normalized to the canonical
    lowercase loopback name (`localhost`, `127.0.0.1`, or `[::1]`, never the
    raw header text) before it builds the `frame-src`, the inner frame `src`,
    and the script's baked dashboard origin. This is a request-derived value
    written into a header, the exception to what decision 21 rejected for
    `frame-ancestors`; the normalization and a code comment at the site say
    why it is safe (`hostGuard` and `localViewer` already admitted only a
    loopback name). The injected script derives the dashboard
    origin it posts to from its own `location.hostname` and the daemon port
    the listener bakes into the script, keeping decision 14's single
    `targetOrigin`. Rejected: a fixed `127.0.0.1` (a `localhost` viewer's
    card would be cross-site).
23. **Listener lifecycle** (UX lead review 2026-11-13, technical):
    `Daemon.Close` (`daemon.go:578`) shuts down `d.server` and closes
    `d.listener` only, so proxy listeners (separate servers) need their own
    teardown. A registry keyed by target (a dev-server `host:port`, or a
    registered directory's resolved path) holds one live listener per
    target, reused by every card on it; at most 16 live listeners (a new
    target beyond the cap is refused and the card shows the error, as a bind
    failure does, decision 15); a listener with no in-flight request, no
    open upgraded connection, and no live-card heartbeat for 30 minutes closes
    (the next wrapper load recreates it, on the same port when it can,
    decision 29; decision 10). `Daemon.Close` closes every
    registered listener, its server and open connections, next to the
    `d.listener.Close()` call (`daemon.go:597`) and so before
    `d.lock.Unlock()`, and the registry then refuses new listeners, so a
    successor's lock take never races a leftover port. Live proxy listener
    ports count as the daemon's own for decision 13's refusal. Tested: after
    `Close` every listener port refuses connections.
    Rejected: one shared listener (decision 19); an uncapped or never-idle
    registry (a listener per ever-opened target would live until restart).
24. **Wrapper framing details** (UX lead review 2026-11-13, technical):
    - Message path: the proxied page is a grandchild of the session page, so
      its posts arrive with `e.source` set to the inner frame's window, never
      the card iframe's `contentWindow` that `docFrame`
      (`dashboard.js:3242`) and the card host match. The relay through the
      wrapper (decision 19) is the only message path; the host drops a
      direct post from the inner frame.
    - The wrapper's CSP does not constrain the target document (a policy
      governs its own document's loads). Its `frame-src` does block the inner
      frame's off-proxy navigations (an OAuth redirect, an external link),
      the mechanism 445e0769 verified. The card therefore carries an "open
      in tab" action: the dev-server card opens its stored target `url`, a
      static card opens its proxy URL (decision 22).
    - The inner frame carries `sandbox="allow-scripts allow-same-origin
      allow-forms allow-popups allow-modals allow-downloads
      allow-popups-to-escape-sandbox"` and no `allow-top-navigation`
      ((542)=a, owner 2026-11-13: the inner frame blocks top navigation only,
      a deviation from (111)=a's "runs unrestricted" accepted by the owner).
      Otherwise the dev page runs unrestricted for itself; it cannot navigate
      the dashboard away. Scripts, the page's own cookies and
      localStorage, form posts, `window.open`, `alert`, file downloads and
      OAuth popups keep working: `allow-same-origin` keeps the proxy origin,
      not an opaque one, and a popup escapes the sandbox. `allow-same-origin`
      with `allow-scripts` is safe here because the proxy origin differs from
      the wrapper's, so the page cannot reach into the wrapper to lift the
      sandbox.
    Rejected: relaying nothing and letting the page post to the host
    directly (the source check cannot admit a grandchild); no inner sandbox
    (any dev page could navigate the dashboard top level); (b) allowing top
    navigation, as (111)=a.
25. **Proxy listener guard and request rewriting** (UX lead review
    2026-11-13, technical):
    - Every proxy listener wraps its handler in `hostGuard`
      (`daemon.go:678`): without it a DNS-rebinding page reads registered
      directories under its own host name. Listeners bind `127.0.0.1`
      explicitly, never `bindHostFor`, which returns every interface in LAN
      mode (decision 6).
    - Upstream `Host` is rewritten to the target's host:port, or Vite's host
      check and Next's `allowedDevOrigins` refuse the request. A present
      `Origin` is rewritten to the target's origin only when it is the
      listener's own origin (any loopback name on the listener's port); any
      other present `Origin` (a foreign page, or `null`) is refused with 403,
      and so is `Sec-Fetch-Site: cross-site`, on plain requests and WebSocket
      upgrades alike, before anything reaches the target. An unconditional
      rewrite would launder a foreign page's Origin past the dev server's own
      checks (Vite's WS origin check, Next's `allowedDevOrigins`, server-action
      CSRF). A request with no `Origin` (a navigation) passes.
    - An absolute `Location` naming the target's origin is rewritten to the
      proxy origin; a `Location` naming any other origin, another allow-listed
      localhost origin included, is refused (decision 15).
    - The script is injected as `<script src>` from a reserved path the
      listener serves itself on the proxy origin, never as an inline script
      (an inline script fails under the target's `script-src`). The rest of
      the target's CSP is left as is, so a page whose `script-src` excludes
      its own origin loads without the script and the card shows pick as
      unavailable when the script's ready message never arrives.
    - Content-Encoding: the proxy asks upstream for `identity`; an HTML
      response that arrives gzip-encoded anyway is decoded (`compress/gzip`),
      injected, and sent without `Content-Encoding` and `Content-Length`;
      another encoding (br, zstd) passes uninjected with the same
      unavailable state.
    Rejected: an inline injected script; injecting into a still-encoded body;
    rewriting every `Origin` to the target's (it defeats the dev server's
    origin checks).
26. **Registered directory binding** (UX lead review 2026-11-13, technical;
    mechanics of decision 18): registrations are kept in daemon memory, in
    the registry entry for the target (decision 23), so a listener closed by
    idle is recreated for the same directory and a daemon restart drops them
    (the card re-registers on restore, decision 10). Registration is a POST
    through `TrustedWrite` (decision 20). The path is resolved with
    `filepath.EvalSymlinks` and must lie under the session root, the same
    resolve-then-contain pattern as `fs_capability.go:468`; the proxy
    re-resolves every request's path and refuses a symlink that now leaves
    the registered directory. Static serving also denies dotfiles through the
    existing `dotDenied` (`fs_capability.go:466`), with the registered
    directory as its root and the request path as `rel`, so a dot-named
    segment (`.env`, `.git/`), as written or once resolved, answers 404 and a
    dot-named ancestor of the registered directory does not block it
    (lead answer 2026-11-13, technical). In memory means no new store, so T0 holds
    (decision 11). Rejected: persisting registrations (a new store and a
    schema); resolving once at registration (a symlink swapped in later
    would escape).
27. **Cookie caveats accepted** ((530)=a, owner 2026-11-13): cookies are
    keyed by host, not port, so the dashboard origin, every proxy origin on
    the same host name (decision 22), and every dev server on that name share
    one cookie jar. The owner accepts these consequences, and decision 17
    stands as written. The proxy shares cookies with every page on the same
    hostname: ports do not separate cookies. So: (i) two dev servers on
    different ports of one host see each other's host-only cookies; (ii) a dev app's cookies are sent to
    the dashboard daemon (which ignores them) and the dashboard's own
    cookies reach the proxy, where decision 17 strips the named ones from the
    forwarded request; (iii) a dev page's script or `Set-Cookie` can
    overwrite or clear the dashboard's theme and language cookies, since a
    host-keyed jar cannot tell them apart; (iv) a `localhost` card and a
    `127.0.0.1` card keep separate jars, because they are different hosts.
    The ticket defines no help or tooltip text for the card, so no UI text is
    added; any card help text written later names these caveats, first that
    cookies are shared with every page on the same hostname.
    Rejected: (b) a separate cookie jar per target (a fixed alternate host
    name per listener would break decision 22's same-site match and needs
    name resolution the daemon does not own).

28. **Wrapper request contract** (completeness review 2026-11-13,
    autonomous): the registration POST (decision 20) takes `{url}` or
    `{path}`, runs the dev-server allow-list (decision 13) or the directory
    checks (decision 26), and returns an opaque registration `{id}` for both
    target kinds; the same target returns the same id (the registry is keyed
    by target, decision 23). The card never persists the id (decision 10): on
    restore it re-registers and gets a current one. The wrapper route is
    addressed by that id; on load it looks the entry up, creates or rebinds the
    listener (decision 29), and writes the live port into its `frame-src` and
    inner frame `src` (decision 22). An unknown id (a daemon restart, a
    refused target) answers 404 and the card re-registers once. While the
    wrapper is open it POSTs a heartbeat naming the id every 60 seconds
    through `TrustedWrite` (so its CSP carries `connect-src 'self'`); a
    heartbeat in the last 3 minutes marks the entry live for decision 23's
    idle clock. Rejected: naming the target in the wrapper URL (a card could
    then address a target that was never registered); a heartbeat from the
    injected script (a page that blocks the script, or a static page, would
    never send it).
29. **Port stability** (final design review 2026-11-13, autonomous): the
    registry entry keeps the target's last port. A listener recreated after
    an idle close or a restore rebinds that port and falls back to an
    ephemeral one only when the rebind fails; idle time does not count while
    a card on the target is live (decision 28's heartbeat). Why: the page's
    origin includes the port, so localStorage, IndexedDB, service workers and
    other state keyed to the proxy origin would vanish on every recreation,
    breaking decision 4's promise that the dev page's own storage works.
    Rejected: a fresh ephemeral port on every recreation. Tested: a recreated
    listener for the same target gets the same port; a listener with a live
    card is not idle-closed.
30. **Pick and console limits, selector, and error count** (completeness
    review 2026-11-13, autonomous): named `wsinbox` constants beside
    `maxMentions` (`inbox.go:113`): `maxPicks = 50`, `maxConsoleBlocks = 10`
    per message, `maxConsoleLines = 50` per block, `maxConsoleLineBytes =
    1024`, `maxSelectorBytes = 512`, `maxPickTextBytes = 1024`; a message over a
    cap is `ErrInvalid` like the mention cap (`inbox.go:369`). The injected
    script keeps a ring of the last `maxConsoleLines` errors (`window.onerror`,
    `unhandledrejection`, `console.error`); the card badge shows the number
    collected since the page loaded or since the last attach, and attaching
    sends the ring and resets the count. The selector is `#id` when the id is
    unique in the document (`CSS.escape`); else `[data-testid="..."]` when
    unique; else a path from the nearest ancestor with a unique id (or
    `html`) of `tag:nth-of-type(n)` segments, at most 8 deep; the script
    verifies `querySelectorAll(selector).length === 1` before posting and
    otherwise sends the deepest path it built. The pick text is the element's
    trimmed `textContent`, cut to `maxPickTextBytes`.

## Constraints

- Shipped surfaces: the `wb:pick` (and console attach) description lands in
  the shared whiteboard playbook; read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, and `ai-docs/manuals/wsflow-mirroring.md`
  before editing it. Daemon code under `agents-plugin-tool/internal/` follows
  `ai-docs/manuals/release-compatibility.md`.
- Never restart or replace the user's running dashboard daemon for checks; use
  a separate dev instance or a rendered fixture.
- The wrapper's framing exception (decision 20) is a lead answer, not a
  property of the iframe-shell ticket, which stays unedited: whichever of
  iframe-shell and browser-pane lands second keeps the exception explicit in
  code next to `writeHTML`.
- The iframe-shell ticket's "Composed session-page CSP" text (`?frame=1`
  response ending `frame-src 'self' <proxy origin(s)>`) is superseded by
  decision 19 for this feature: the session page stays at `frame-src 'self'`
  and each wrapper carries its own proxy origin. The lead points the
  iframe-shell worker at decision 19.
- Mirrors: the whiteboard playbook line lands in
  `agents-plugin/rsrc/whiteboard/whiteboard.md` (Answers section, after the
  `wb:mention` example at `:350`) and needs its wsflow mirror
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
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard.js, pages.go, new web-proxy files (the registry, listeners, wrapper and registration routes; `proxy.go` and `proxy_test.go` already hold the MCP hosting route, so the new files take other names), `daemon.go` (Close tears the listeners down, decision 23), agents-plugin-tool/internal/wsinbox (Entry fields, Merge, restore), the dashboard.inbox_drain description in internal/mcp/server.go, agents-plugin/rsrc/whiteboard/whiteboard.md with wsflow and pi mirrors |
| scope.surface | cross-module | new per-target daemon listeners and proxy (registry, cap, idle close, teardown on `Daemon.Close`), a wrapper route with its own framing headers (an explicit exception to `writeHTML`'s DENY, decision 20) and a registration route, dashboard assets, shipped whiteboard playbook (the session page CSP is unchanged, decision 19); no exported Go symbol is named |
| scope.new_public_symbol | yes | wb:pick and wb:console message elements (agent-visible grammar); the proxy listener port |
| scope.new_type_contract | yes | injected-script to host postMessage protocol; wb:pick url/selector and wb:console url element grammar |
| scope.test_surface | new-files | existing pages_test.go and access guards cover CSP and localViewer; the proxy, registry, wrapper and registration routes need new Go test files with an `httptest` upstream fixture (HTML plus a WebSocket; ReverseProxy search over agents-plugin-tool returned nothing) |
| complexity.reuse_points | confirmed | cards.js kind registry (register/open at cards.js#L839-L896), bmentions chip path (dashboard.js#L1182-L1275; hostMention at #L1933, addMention at #L1982), localViewer (access.go#L146-L148), TrustedWrite (access.go#L156), hostGuard (daemon.go#L678), the resolve-then-contain pattern (fs_capability.go#L468) |
| complexity.side_effect_risk | high | new loopback listeners (registry, capped at 16, idle-closed, closed in `Daemon.Close`) that proxy to arbitrary localhost targets and serve registered workspace directories |
| risk.correctness | high | proxy rewriting and script injection across HMR/websocket dev servers, plus postMessage pick and console capture |
| risk.fit | moderate | follows the card framework, but adds a per-card wrapper route with its own CSP and framing headers (frame-ancestors 'self', an exception to the iframe-shell ticket's (503) rule) and the iframe-shell ticket also reframes the page (pending 261113-feat-ws-dashboard-iframe-shell) |
| risk.test | high | the pick highlight, injection and proxied page need a browser LOOK capture on a dev daemon; Go tests cover allow-list, guards (wrapper Sec-Fetch gate, listener host guard), Host/Origin/Location rewriting, gzip injection, wrapper headers, idle close and teardown |
| risk.security_or_contract | high | SSRF and execution surface, a dev page on a sibling port inside a sandboxed inner frame without top navigation, a wrapper framed by its own origin, DNS rebinding on the proxy listeners, and a new shipped message grammar |

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
- Wrapper route per decisions 19, 20, and 24 (the session page CSP stays
  `frame-src 'self'`, `pages.go#L1276`, pinned by
  `TestSessionPageFramesOnlyItsOwnOrigin`): Sec-Fetch-gated GET,
  `frame-ancestors 'self'` + `SAMEORIGIN`, sandboxed inner frame, relay, and
  an "open in tab" action on the card.
- Listener registry, cap, idle close, and teardown in `Daemon.Close` per
  decision 23; proxy URLs from the page's own hostname per decision 22.
- Listener guard, Host/Origin/Location rewriting, `<script src>` injection,
  and Content-Encoding handling per decision 25.
- Registration id, wrapper request contract, and heartbeat per decision 28;
  port stability per decision 29; pick and console limits, selector, and
  error count per decision 30.
- Static entry points and directory registration per decisions 18 and 26.
- Whiteboard playbook Answers section: describe the new element(s).
- Proxy response hardening, mechanics, and inbox fields per decisions 14-30.

Verification (Go and headless checks run against an `httptest` upstream
fixture, an HTML page plus a WebSocket endpoint that records the headers and
`Origin` it receives, never a real dev server):

- Go, proxy and registry:
  - target allow-list, including refusal of the daemon's own and live proxy
    listener ports; same-machine guard; the listener's host guard (a
    non-loopback `Host` answers 403);
  - header stripping per decision 17 (dashboard cookies and bearer stripped,
    dev-app cookies forwarded);
  - upstream `Host` rewritten to the target; `Origin` rewritten only when it is
    the listener's own, and a foreign-`Origin` upgrade and a foreign-`Origin`
    POST (and a `Sec-Fetch-Site: cross-site` request) are refused with 403 and
    never reach the fixture;
  - an absolute `Location` naming the target rewritten to the proxy origin,
    and a redirect to a different allow-listed localhost origin refused;
  - script injected only into HTML responses and as a `<script src>` from the
    reserved path; a gzip-encoded HTML upstream injected and served without
    `Content-Encoding`; WebSocket upgrade passthrough;
  - response CSP: only `frame-ancestors` removed from a target CSP (several
    headers and `-Report-Only` included, other directives kept), the proxy's
    own header naming `localhost`, `127.0.0.1` and `[::1]` on the daemon's
    port appended, `X-Frame-Options` removed;
  - static serving confined to the registered directory, a symlink swapped to
    leave it refused per request, a dotfile such as `.env` refused, a path
    outside the session root refused at registration;
  - registry: idle close (injectable timeout); a live-heartbeat card is not
    idle-closed; a recreated listener for the same target gets the same port;
    the 16-listener cap; `Daemon.Close` closes every listener and refuses new
    ones.
- Go, wrapper and CSP:
  - the wrapper CSP's `frame-src` is exactly that card's proxy origin, built
    from the normalized host (a mixed-case `Host` yields the canonical name);
    `frame-ancestors 'self'` and `X-Frame-Options: SAMEORIGIN` are both
    pinned; the inner frame's sandbox attribute is pinned; the session page CSP
    is unchanged;
  - the wrapper GET succeeds from the session page's iframe
    (`Sec-Fetch-Site: same-origin`, `Sec-Fetch-Dest: iframe`, a local viewer)
    and is refused for a non-local viewer (404) and for a cross-site
    `Sec-Fetch-Site` (403);
  - the registration and heartbeat routes refuse a non-local viewer and a
    cross-origin caller; an unknown id answers 404.
- Go, inbox: `wsinbox` persists, merges, restores, and caps pick and console
  entries at the decision 30 constants; a pick-only and a console-only message
  are accepted (the `inbox.go:475` check); a command carrying either is
  refused (the `inbox.go:498` check); the drain description lists the fields.
- JS and headless:
  - a node test of the postMessage protocol (posts go only to the dashboard
    origin; the host drops messages from another origin or another frame, and
    a direct post from the inner frame);
  - a node test of the selector algorithm (unique `#id`, `data-testid`, the
    `nth-of-type` path, and the uniqueness check);
  - the pick-mode toggle turns the hover highlight on and off and a click
    posts one pick; "attach console errors" adds one `wb:console` chip with
    the ring's lines and resets the badge; "open in tab" opens the stored
    `url` for a dev-server card and the proxy URL for a static card;
  - a localhost link click (the three loopback names) and "+ Web" open a card,
    a link to the daemon's own port opens normally, reload re-requests the page,
    a reload of the session page restores the card from localStorage (a static
    card also after a daemon restart, re-registering its path), "Open as web
    card" on an HTML file card and on an HTML file link opens a static card, a
    workspace path typed into "+ Web" does the same, and a listener bind failure
    shows in the card;
  - the sandboxed inner frame keeps working storage and cookies, form posts,
    `window.open`, `alert`, and a download, and cannot navigate the top level.
- LOOK, on a dev daemon instance: a web card showing a proxied local page with
  HMR active, an active pick highlight, and the composed message carrying a
  `wb:pick` chip, a `wb:console` chip and the error badge.

### Result (eee367e08) - 2026-11-14

Landed on impl/develop/pox-veal-stoke (b0408fc7a..eee367e08):

- Daemon: per-target loopback proxy listeners with a registry (cap 16, idle
  close, heartbeat live window, port stability, teardown in `Daemon.Close`),
  the wrapper route, and the registration and heartbeat routes
  (`webcard_proxy.go`, `webcard_routes.go`; the route and postMessage
  protocol is that file's comment). Pages: `assets/webframe.js` (wrapper
  relay and heartbeat), `assets/webinject.js` (pick mode, selector, console
  ring, error count), `assets/webcard.js` (the `web` card kind,
  `window.WsWeb`).
- Entry points: "+ Web" (URL or workspace path), a localhost link click on
  the page and from board frames, "Open as web card" in the link menu and on
  HTML file cards.
- Inbox: `wsinbox.Pick`/`Console` with the decision 30 caps, carried by the
  message route, the held view, cancel/restore, the draft, and
  `dashboard.inbox_drain` (`<wb:pick>` / `<wb:console>` after the mentions).
- Whiteboard playbook Answers paragraph for `wb:pick` and `wb:console`, with
  wsflow and pi mirrors and manifests.
- `scripts/webcard-headless.mjs`: a headless harness against its own
  isolated daemon (temp WS_CACHE_HOME/WS_CONFIG_HOME/HOME, `--port 0`) and a
  Node fixture upstream with an HMR-like WebSocket.

Verification:

- `go test ./internal/wsdashboard/ ./internal/wsinbox/ ./internal/wsi18n/ ./internal/wsrsrc/... -count=1`: ok.
- Node: `testdata/webinject.test.cjs` 20 pass, `testdata/webframe.test.cjs`
  6 pass, `testdata/webcard.test.cjs` 21 pass (run in Go by
  `TestWebCardScripts`).
- `node scripts/webcard-headless.mjs`: all 16 scenarios pass on the final
  state (its own daemon, stopped by PID; the shared daemon untouched).
- LOOK (isolated daemon, light and dark): the card on the proxied fixture
  page with HMR connected, the pick highlight on an element, and the
  composer with a `wb:pick` and a `wb:console` chip beside the card's error
  badge.
- `go test ./internal/mcp/ -count=1 -timeout 40m`: only
  `TestRenderDispatchSeparatesResourceAndWorktreeRoots`,
  `TestRenderFromWorktreeSessionCommitsInThatWorktree`, and
  `TestRenderPathMintedChildRecordsLeadParent` fail, and they fail on the
  base b0408fc7a too (macOS `/private/var` vs `/var`).
- Lite review: one Important (a proxied page could post the wrapper-only
  `frame`/`gone` types through the relay and point "open in tab" at another
  origin) fixed in 64ed5181a; two minors (nested session roots sharing a
  directory entry's base; a malformed gzip page passed truncated) fixed in
  eee367e08.

Decisions taken in implementation:

- Registration starts the listener at once, so a full registry or a bind
  failure reaches the card as the route's answer.
- https targets skip certificate verification (loopback only); a redirect
  to another origin answers 502, as does a gzip HTML page that does not
  decode.
- The wrapper's `p` must be a single-slash path, else `/`; a file at the
  session root's top registers the root itself (dotfiles stay refused).
- Directory entries are keyed by session root and directory, so nested
  roots each get their own base.
- `maxWebURLBytes = 2048` joins the decision 30 constants; the message
  body limit grows by 2 MiB only when picks or consoles are present.
- The pick text collapses whitespace runs before the trim and cut. When no
  selector fits 512 bytes the script sends the bare tag name (else `*`).
  The path is unique by construction unless cut at 8 steps, so only the
  `#id`, `data-testid`, and anchor candidates are checked with
  `querySelectorAll`.
- The injected script also wraps `history.pushState`/`replaceState`, so a
  client router's route change is announced; a same-document move ends pick
  mode and re-posts the error count.
- Attach sends the ring, then clears it and resets the count.
- The relay passes only the page's types; the card accepts a `frame` proxy
  only as `http://<its own host name>:<port>`, and takes a pick or a console
  block only after the user's own toggle or attach.
- The web card script is inlined only on a local viewer's page; "+ Web"
  uses the terminal's fixed slot fallback; a board frame's link report opens
  a card only under user activation; a static card's record follows only
  top-level files of its registered directory.
- Escape on the pick toggle ends pick mode as Escape inside the page does.
- Live checks ran on the harness's own isolated daemon rather than
  `scripts/dev.py rerun daemon`, which replaces the running daemon.

#### Edition (4354df8f0) - 2026-11-14

Lead pre-merge security review: the static scope is narrowed to keep
decision 5 (the opened file's directory subtree, never the whole root).

- A file at the session root's top now registers a file-only scope: that
  file serves and every other path on its listener answers 404. A file in
  a subdirectory keeps its directory subtree. The root itself (also through
  a symlink) is refused. This withdraws the Result's "a file at the session
  root's top registers the root itself".
- Registration refuses a dot-named or git-ignored path (`capDenied` on the
  registered path: `.git/`, `.cache/`, an ignored `build/`), and every
  static request applies `capDenied`, so an ignored file inside a registered
  subtree answers 404. The page runs scripts on its own origin and can
  fetch anything its listener serves, so unlike the file routes this
  applies to a local viewer too; a report in an ignored folder opens in the
  preview card, not a web card. (9a6618750)
- A static card's record takes the page's posted path only when its decoded
  name is still one file name, and picks keep a decoded `..` step encoded,
  so a forged `..%2F` path cannot point a restored card elsewhere.
  (4354df8f0)
- Verification: `go test ./internal/wsdashboard/ ./internal/wsinbox/
  ./internal/wsi18n/ ./internal/wsrsrc/... -count=1` ok; node webinject 20,
  webframe 6, webcard 21 pass; `node scripts/webcard-headless.mjs` all 16
  scenarios pass on its isolated daemon. New tests
  `TestWebCardRootFileServesOnlyItself` and
  `TestWebCardRefusesGitIgnoredPaths`, and the dot-directory refusal case,
  fail with the old scope (mutation-checked).
- Left as is, per the lead: popups escaping the sandbox, the remembered-port
  rebind on macOS with SO_REUSEADDR, registry entries never removed, the
  HTML buffering cap, and a dashboard viewed at `[::1]`.

---
title: "Require a browser credential on localhost dashboard access (cookie gate, one-time login links, Electron cookie)"
related:
  261113-refactor-ws-dashboard-drop-lan-serving: "LAN token/cookie branch this gate coexists with; whichever lands second keeps the localhost gate"
  261115-feat-werkschaft-electron-daemon-override: "Electron daemon launch shipping in the same release (1227)"
  261115-feat-werkschaft-electron-shell: "Electron shell whose navigation gets the native cookie"
  261112-feat-ws-mixed-version-compat-guards: "tier vocabulary; old-client behavior against a new daemon"
---

# Require a browser credential on localhost dashboard access

## Background

### Goal

Every browser request to the dashboard daemon from this machine needs a
credential, not only LAN requests. A browser gets it once, through a
single-use login code, and keeps it as a long-lived cookie. The Electron
shell sets that cookie itself. Machine callers that installed 0.50.1 binaries
use keep working unchanged.

### Access model today (verified at drafting; line numbers drift)

- **Loopback mode has no credential.** Without LAN mode the whole mux sits
  behind `hostGuard`, which only checks that the Host header is a loopback
  name (`wsdashboard/daemon.go:771-775,781-804`). Any local process, including
  another OS user's, can drive every route. It only has to forge `Host` and,
  for the stricter writes, `Origin`. `TrustedWrite` is `localViewer(r)` plus
  `sameOrigin(r)` (`wsdashboard/access.go:162-170`; `inbox_routes.go:154-164`).
  Both are headers that a non-browser client sets freely.
- **Agent routes are reachable without a credential.** `POST /agents`
  (create a conversation with any profile and root) and
  `POST /agents/<id>/send|interrupt|stop|close|...` pass only `writeAllowed`.
  That check accepts a JSON body with no Origin or the daemon's own Origin
  (`wsagent/http.go:150-153,217,224,324-341`). This lets any local process
  run code through a Pi worker. Lead spawn, settings, approvals and moves add
  `Trusted` (`wsagent/http.go:168-177`), which uses the same forgeable
  headers. The `/support` POST routes follow the same `writeAllowed` rule
  (`wsagent/support.go:30-31,675`).
- **LAN mode.** `lanGuard` admits a request when it is a `localViewer`
  (loopback peer and loopback Host), a `publicAsset`, a `previewCapability`,
  or when it carries the LAN token (`access.go:96-128`). The token arrives as
  the `token` query parameter and is returned as the HttpOnly cookie
  `ws_dashboard_token` with SameSite=Lax and a 400-day Max-Age
  (`access.go:58-68,109-119`). The LAN token persists in its own owner-only
  file `lan-token`. It is read on every call, and deleting the file rotates
  it without a restart (`access.go:70-94`). A LAN token never passes
  `TrustedWrite`, because a LAN peer is never a `localViewer`
  (`access.go:162-170`). Settings shows a local viewer the LAN URLs as plain
  `<code>` text with no copy button (`lanAccessFor`, `access.go:198-217`;
  `assets/dashboard.tmpl:126`).
- **The endpoint token is per daemon run, not per install.** `StartDaemon`
  mints a fresh `newToken()` (32 random bytes, hex) on every start and puts it
  in `endpoint.json` (`daemon.go:357-361,399-406,569-575`). The file is
  written 0600 through `writePrivateFile` (`endpoint.go:85-121`) and removed
  on exit when still owned (`endpoint.go:142-148`; `daemon.go:698`). The
  state directory is created 0755 (`daemon.go:324`). The token authenticates
  `/shutdown` (`daemon.go:826-835`) and `/mcp` (`proxy.go:146-161`). A cookie
  derived from it would die at every daemon restart, upgrade or dev rebuild,
  so (1229) uses a new persisted secret (see Decisions).
- **Sandboxed frames send no cookie.** A topic body's frame and an HTML
  preview frame have opaque origins (`wswhiteboard/render.go:755-761`;
  `assets/preview.js:210-216`). They load only `publicAsset` routes
  (`access.go:130-150`) and capability routes
  `/s/<rid>/fs/cap/...` and `/s/<rid>/fs/view/...`, which the path's
  capability admits (`fs_capability.go:132-133,274-296`). A topic-body image
  larger than 2 MiB is not inlined as a data: URL. It falls back to the image
  route `/s/<rid>/image?path=` (`image_route.go:71-86,209-213`), which a
  cookieless frame cannot load (`access.go:33-35` already says so for LAN).
- **Streams.** The SSE and WebSocket streams and the terminal WebSocket are
  served by the same mux, so the access guard runs before their handlers
  (`daemon.go:729-775`). They check the Origin (`stream_ws.go:47-55`;
  `sse.go:450`; `terminal_routes.go:214-230`). A browser sends a SameSite=Strict
  cookie on a same-site WebSocket handshake: Chromium treats `ws` as `http`
  when it decides whether a request is same-site, and the port is ignored.
  An EventSource and a same-origin fetch send it too.
- **Web card proxy.** The proxy strips the daemon's own cookies before it
  forwards a request upstream. Its list is `themeCookie`, the i18n cookie and
  `lanTokenCookie` (`webcard_proxy.go:577-579,629-651`). Cookies ignore the
  port, so the proxy origin on another 127.0.0.1 port receives every
  127.0.0.1 cookie.

### Route inventory

The guard is deny-by-default. Only the routes below skip the cookie, and each
one keeps the check it already has. The caller sources were verified at HEAD
and at `v0.50.1` (`d1d938e3d`) with `git show v0.50.1:<path>`.

| Route | Class | Callers (HEAD; 0.50.1) | Own check today | After this ticket |
|---|---|---|---|---|
| `GET/HEAD /health` | machine | launcher copies (`agents-plugin*/bin/ws-mcp-launcher.py:1168`; 0.50.1 `:1053`); Go `Health` (`wsdashboard/client.go:25-49`); Electron (`desktop-electron/endpoint.js:93,99`; 0.50.1 `:45`); repo scripts | none | unchanged, cookie-exempt |
| `GET /mcp` (Upgrade `ws-mcp`) | machine | launcher copies (`ws-mcp-launcher.py` ~1447; 0.50.1 `PROXY_PATH` `:879`) | loopback peer, no Origin, endpoint bearer (`proxy.go:150-161`) | unchanged, cookie-exempt |
| `POST /shutdown` | machine | `RequestShutdown` (`client.go:53-69`); Electron (`endpoint.js:114-128`) | endpoint bearer (`daemon.go:826-835`) | unchanged, cookie-exempt |
| `POST /nudge` | machine | `SendNudge` (`client.go:73-95`) from `mcp/dashboard_sessions.go:693-700`; 0.50.1 `client.go:81`; repo scripts | none: it can only cause a disk re-read (`daemon.go:841-871`) | cookie-exempt. New clients also send the endpoint bearer, which is not required yet (see Decisions, tier) |
| `GET /session-hire/roster` | machine | `mcp/session_hire.go:87-93`; 0.50.1 `:93` | none (`session_hire_routes.go:41-52`) | same as `/nudge` |
| `POST /agents/by-token/<op>` | machine | a Pi worker's attached ws-mcp (`mcp/pi_forward.go:75-85`; 0.50.1 `:80,85`) | `Trusted` local plus a per-conversation capability bearer (`wsagent/children.go:398-420`) | unchanged, cookie-exempt |
| `GET/HEAD /s/<asset>` that matches `publicAsset` | sandboxed frame | topic-body frames | none | unchanged, cookie-exempt |
| `GET/HEAD /s/<rid>/fs/cap/...`, `/s/<rid>/fs/view/...` | sandboxed frame or own tab | preview card, file view | capability in the path (`fs_capability.go:274-296`) | unchanged, cookie-exempt |
| `GET/HEAD /pwa/*` | static build asset | the browser's manifest fetch, which omits credentials | none (`pwa.go:23-50`) | cookie-exempt (new exemption) |
| new `/auth/*` routes (Phase 1) | gate and exchange | gate page, Electron, Settings | n/a | each route checks its own credential |
| everything else: `/`, `/s/...` pages and their sub-routes (message, open, terminals, fs/act, fs/outside, git, web, image, file, ...), `/w/...`, `/events`, `/rail/*`, `/session-hire` queue/approve/reject, `/agents` (every non-token route), `/support/*`, `/tracker/*`, `/holders*`, workspace routes, `/debug/pprof/` | browser | dashboard pages only. No machine caller exists at HEAD or 0.50.1: lead `pi.*` tools run in-process (`cmd/ws-mcp/proxy.go:107-108`, `mcp/pi_tools.go:159-162`; same lines at 0.50.1), and workers use only the token route | `hostGuard` only | the localhost credential is required |

Hosted lead and Pi `pi.*` calls never use HTTP. The daemon gives a hosted
session's MCP server the runner handle directly (`opts.Agents`,
`cmd/ws-mcp/proxy.go:107-108`), and `handlePiTool` calls it
(`mcp/pi_tools.go:159-250`). A Pi worker's attached ws-mcp forwards through
the token route with its capability bearer (`mcp/pi_forward.go:75-85`). This
was the same at 0.50.1.

### Dashboard URL print and open sites

| Site | Evidence | Choice |
|---|---|---|
| `whiteboard.update` `full:` line (session page URL) | `mcp/whiteboard_tools.go:327-331`; `dashboardPageURL`, `mcp/dashboard_sessions.go:727-744` | **One-time login link.** This is the URL agents hand to users, and the only path into the dashboard that every agent session has. The code is minted in-process through a hook the daemon passes into the hosted server, so `whiteboard.update` still never waits on HTTP. A server with no hook (not daemon-hosted) prints the plain URL. When the dashboard does not serve the session, the line prints the file path, unchanged. |
| Electron `loadURL` | `desktop-electron/main.js:226-229,283-285`; `endpoint.js:94,107` | **Plain URL.** The cookie is set natively before `loadURL` (1230). |
| Electron menu | `main.js:318-336` (Connection submenu) | New **"Copy browser login link"** item (Phase 2). |
| Settings popover | `assets/dashboard.tmpl:126` (`settings`, LAN box) | New **"Copy browser login link"** button for a local viewer (Phase 1). It reuses `copyText` (`assets/dashboard-actions.js:86-109`). |
| LAN Settings URLs and the LAN 401 text | `access.go:126,198-217` | Unchanged. They carry the LAN token, a separate credential. |
| Tracker hook event `url` | `wstrackerhook/event.go:38-39,118`; `cmd/ws-mcp/dashboard.go:217-218` | **Plain URL.** Hooks are user scripts that may forward the URL to logs or remote services, and one code per event would be wasted. The gate keeps the `#/...` fragment when it continues after login. |
| System-open of a content link, preview "open externally" | `sysopen_route.go:103-121`; `assets/dashboard-actions.js:358`; `assets/preview.js:84-90` | **Plain.** These open user content. A dashboard URL among them meets the gate. |
| Tauri attach-only shell | `agents-plugin-tool/desktop/src/endpoint.rs:66` | **Not changed.** That shell is being replaced by Electron (`261115-feat-werkschaft-electron-shell`). Until then it shows the gate (1234). |
| Hosted leads, Pi harness, launcher Python, hooks | no dashboard URL is printed (searched `http://` and `127.0.0.1` in those trees) | n/a |

### Settled drafting questions (lead technical calls, 2026-11-16)

The owner went offline after pre-approving a minor bump only if a T3 break is
unavoidable; the lead settled the drafting questions as technical calls.

- (1232) The `whiteboard.update` `full:` line mints a one-time login link, as
  chosen above. Rejected: a plain URL, which leaves a cookieless browser no
  path in except Electron or Settings.
- (1233) Topic-body images over 2 MiB keep working: since the read-hardening
  merge they are relayed through the same-origin host page (the same relay as
  outside-root images), so the host's cookie carries them. No functionality is
  removed.
- (1234) The Tauri shell and 0.50.1 Electron windows show the gate until they
  update (T1 display-degraded). Retiring Tauri is not part of this ticket.
- (1235) `/nudge` and `/session-hire/roster` stay credential-free through
  0.50.x; 0.50.2 clients start sending the endpoint bearer (T0). Enforcement is
  a later minor behind a T3 boundary. Rejected: enforcing now, which is a T3
  break that is avoidable here because neither route can execute anything.
- (1236) Keep SameSite=Strict, with the gate's same-origin auto-continue for a
  link followed from another site. Rejected: Lax, which sends the cookie on
  cross-site top-level navigations.
- (1237) The cookie derives from a new persisted 0600 secret file, separate
  from `lan-token`, not from `endpoint.json`'s per-start token (which would
  log every browser out on each daemon restart). The cookie being shared with
  other servers on the same loopback host name (cookies ignore ports) is an
  accepted known gap over plain http. No opt-out key ships.
- (1238) Gate UI details left open by the mockup: the Electron menu item is
  English like the rest of that menu ("Copy Browser Login Link"), and the gate
  names it as the desktop app's Connection menu; after the menu click the app
  asks the dashboard window to show the same toast as the Settings action;
  wrong, expired and used codes share one message; the Settings item shows
  only to a local viewer; a paste during checking is ignored; failed
  exchanges are rate-limited (10 per minute per daemon) with the message
  "잠시 후 다시 시도하세요."; the gate follows the theme cookie when set,
  else the OS.

## Decisions

Owner decisions (whiteboard, 2026-11-16):

- **(1226)=a: every localhost browser access needs a secret, not just LAN
  access.** Electron carries it natively. A browser gets it once, then keeps
  it as a cookie. Machine routes keep their own checks (1231).
  Rejected (b): a token only for strong routes (terminal, spawn, settings,
  sysopen and fs act, outside-root) with board reads left open. A mistake in
  classifying any one route becomes a hole. Rejected (c): leave it as is.
- **(1227)=a: ships in 0.50.2,** together with (1220)=a (localhost
  outside-root file viewing, implemented on a separate branch) and the
  Electron daemon launch (`261115-feat-werkschaft-electron-daemon-override`).
- **Owner, clipboard: pasting needs no manual click into a field.** Ctrl+V
  (Cmd+V) anywhere on the gate submits. A clipboard button reads with
  `navigator.clipboard.readText`. Reading the clipboard automatically on load
  is impossible, because browsers require a user gesture. UI mockup:
  `ai-docs/assets/mockups/261116/localhost-token-gate/` (in progress).
  Verification includes a side-by-side capture comparison against it.

Lead technical calls (announced 2026-11-16):

- **(1228) URLs carry a single-use login code with a 10-minute expiry,** which
  the first visit exchanges for the cookie. The long-lived secret never
  appears in a URL. Rejected: putting the endpoint token in URLs. URLs end up
  in chat transcripts and board embed files, and those can be committed to a
  public repository.
- **(1229) The cookie is HttpOnly, SameSite=Strict, Path=/, Max-Age 400
  days.** Its value is derived from a persisted per-install secret, for
  example an HMAC over a fixed purpose string, so rotating the secret
  invalidates every cookie. Rejected: a session cookie, which would show the
  gate again after every browser restart.
  - *Research correction (drafting):* `endpoint.json`'s `token` is per
    daemon run, not per install (Background). The per-install secret is
    therefore a new owner-only file in the dashboard state directory. It
    follows the `lanToken` pattern (`access.go:70-94`): minted when missing
    or blank, re-read so that deleting it rotates the secret without a
    restart (caching is allowed if a deletion still takes effect), written
    0600 through `writePrivateFile`. It is separate from `lan-token`, so a
    LAN-token holder cannot derive the localhost cookie.
- **(1230) Electron sets the cookie natively with `session.cookies.set`
  before `loadURL`.** Rejected: injecting a header through
  `onBeforeSendHeaders`, which does not cover WebSocket and sandboxed-frame
  requests uniformly.
- **(1231) Machine routes that installed 0.50.1 binaries call keep their
  current checks.** Only browser-facing routes require the cookie. This keeps
  old sessions working (T0/T1) and avoids a T3 lockout. The routes that stay
  as they are, which is the complete machine set at HEAD and 0.50.1, are:
  `/health`, `/mcp`, `/shutdown`, `/nudge`, `/session-hire/roster` and
  `/agents/by-token/<op>`. The frame exemptions (`publicAsset`, capability
  routes) and the static `/pwa/*` stay exempt as well (Route inventory).

Settled at drafting from the research (lead to confirm):

- **The /agents and /support hardening comes with (1226) itself (owner
  question, 23:45).** Every non-token `/agents` and `/support` route is
  browser-facing, so the deny-by-default guard puts the cookie in front of
  `writeAllowed`/`Trusted`. A non-browser local process then needs the cookie
  value, which only a reader of the 0600 secret file (the same OS user) can
  compute. No 0.50.1 binary calls those routes (Route inventory), so this
  needs no T3.
- **Compatibility tier: T1 overall, patch allowed.**
  - New daemon, 0.50.1 machine callers. They call only the exempt routes
    (Route inventory: launcher `/health` and `/mcp`, `client.go`
    `/health`, `/shutdown` and `/nudge`, `session_hire.go` roster,
    `pi_forward.go` token route), and those routes behave as before. **T0.**
  - New daemon, 0.50.1 browser surfaces (an open tab served before the
    replacement, a 0.50.1 Electron or the Tauri shell loading `/`, a plain URL
    printed by an older daemon). Requests get the gate or a 401. The user
    recovers by pasting a login code, and no data changes. **T1**
    (display-degraded).
  - Old daemon, new clients. A 0.50.1 daemon never reads the new secret file
    (no reference at `v0.50.1`) and ignores unknown cookies. The new Electron
    gets a 404 from the cookie route and loads without a cookie, which is what
    an ungated daemon needs. New bearer headers on `/nudge` and roster are
    ignored. Login links come only from a new daemon, because hosted MCP tools
    run in the daemon binary. **T0.** Residual: a 0.50.1 daemon's web card
    proxy does not strip the new cookie name (`webcard_proxy.go:579` lacks
    it at 0.50.1). That only matters while a 0.50.1 daemon serves a browser
    that a 0.50.2 daemon already logged in.
  - No hello, protocol, `endpoint.json` (`Endpoint`, `endpoint.go:52-59`) or
    shared-store shape changes. An optional `/health` capability field, if
    added, follows `HealthInfo`'s omitted-field pattern (`daemon.go:298-315`).
  - Not done: requiring a credential on `/nudge` or `/session-hire/roster`.
    0.50.1 clients call them without one (`client.go:81`, `session_hire.go:93`
    at 0.50.1), so that would be T3. They stay open in 0.50.x. Their power is
    low: `/nudge` causes only a re-read, and the roster is read-only metadata
    about a requester's child sessions. 0.50.2 clients start sending the
    endpoint bearer on both now (T0), so a later minor can enforce it behind a
    T3 boundary with the owner's approval. Sending it only from new clients is
    not by itself a fix; enforcement is the fix.

### Owner feedback during implementation (2026-11-17)

- **(1239) Clipboard writes are explicit, click-only actions.** Settings keeps
  “Copy browser login link” with subtext stating that it copies to the clipboard,
  signs another browser in after pasting, and works once within ten minutes.
  The Electron item is “Copy Browser Login Link to Clipboard”. No gate or
  logged-in page reads or writes the clipboard on load or on login; browser
  clipboard reads happen only on the paste button. Tests assert this and retain
  the single-use exchange check.
- **Owner clarification (00:04): add a native clipboard round trip from the
  unauthenticated localhost gate.** An explicit button asks the daemon to mint
  a ten-minute, single-use link and write it to this machine's OS clipboard;
  the user pastes it back into the existing gate to prove access to that
  clipboard. The route requires a loopback peer and Host, its own Origin, and
  same-origin fetch metadata when present; it is rate-limited and never returns
  the code or link. Native writers are injectable; unavailable writers disable
  or hide the action. Existing agent links, Settings copy, Electron menu, and
  paste exchange remain. The gate button's visual implementation waits for the
  updated approved mockup (imported with its implementation branch cherry-pick).
  Follow-up technical calls: A2 retains a small recopy text button; a runtime
  clipboard failure (A3) leaves retry enabled, unlike the mockup's disabled
  stand-in, and only a writer unavailable at render hides the action. All gate
  states and Settings are captured in both themes in a temporary artifact folder,
  not committed binary captures.

## Constraints

- Read before editing, per AGENTS.md `### Implementation Conventions`:
  `ai-docs/manuals/shipped-surface-boundary.md` (all `agents-plugin*/`
  paths), `ai-docs/manuals/release-compatibility.md`
  (`agents-plugin-tool/internal/`), and `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`, for the login-link hook).
- **Merge order.** The (1220) outside-root file-viewing branch merges only
  together with this ticket or after it. Outside-root viewing on localhost
  must never ship ungated.
- **Deny by default.** The guard wraps the whole mux, as `hostGuard` and
  `lanGuard` do today (`daemon.go:772-775`). The exemption list is explicit
  and short (Route inventory). Every new route is gated unless it is added to
  that list on purpose.
- **A LAN token never grants localhost access or `TrustedWrite`.** On a
  `localViewer` request only the localhost credential admits; a LAN token in
  the query string or cookie does not. The localhost credential admits only
  `localViewer` requests; LAN requests keep the LAN token path unchanged.
  `TrustedWrite` stays `localViewer` plus `sameOrigin`, behind the guard.
- **Keep the existing browser-threat boundaries:** `hostGuard`'s loopback
  Host check (DNS rebinding), `sameOrigin`, the Sec-Fetch checks, the CSPs,
  sandboxed frames and capability checks
  (`261113-refactor-ws-dashboard-drop-lan-serving` Decision 2).
- **No regression for frames.** Topic-body frames and preview and file-view
  capability loads keep working without a cookie. Large topic-body images
  that use the image route from a frame must keep loading on localhost, for
  example with a capability of the existing `fs_capability` shape on the
  framed image URL. The implementer chooses the mechanism; shipping the
  breakage needs the owner's approval (it removes functionality).
- **No opt-out.** No config key, flag or environment variable disables the
  gate in shipped code. Tests and repo scripts get the cookie through a helper
  (for example the Electron cookie route with the endpoint bearer, or an
  unexported in-package seam whose zero value is gated).
- The new cookie has its own name (not `ws_dashboard_token`) and is added to
  the web card proxy's `dashboardCookies` strip list.
- Login codes are held in daemon memory: single use, 10-minute expiry, a
  capped number outstanding (oldest evicted). Codes outstanding at a daemon
  replacement are lost, which is acceptable at a 10-minute lifetime.
  Comparisons are constant-time (`subtle.ConstantTimeCompare`, as in
  `tokenMatches`, `access.go:152-154`). Codes have at least 128 bits of
  entropy.
- Known residuals, accepted for this ticket: cookies are not port-scoped, so
  any other server on the same loopback host name that the browser visits
  receives the cookie. Cookies are per host name, so `localhost`,
  `127.0.0.1` and `[::1]` each need their own login. Printed links and the
  copy action use `127.0.0.1`, as every current print site does. On Windows,
  `0600` maps only to read-only; the user profile ACL on the cache root is the
  protection, as for `endpoint.json` and `lan-token` today.
- Gate and settings strings go through `wsi18n` and are added to every table
  under `agents-plugin-tool/internal/wsi18n/tables/`.
- Shipped Go and text name no repository-local path or dev script
  (Architecture Rule 4).

## Phases

### Phase 1: Daemon credential, gate, login codes, copy-link action

- **Secret and cookie.** Add the persisted secret file and the cookie
  derivation (1229). Gated requests verify the cookie value in constant time.
- **Guard.** Wrap the mux in one access guard that covers both modes. A
  request passes when it is exempt (Route inventory), or when it is a
  `localViewer` with a valid localhost cookie, or, in LAN mode only, when it
  is a non-local request with the LAN token (existing branch, unchanged).
  `hostGuard`'s Host check still runs in loopback mode.
- **Refusal shape.** A navigation (GET/HEAD with `Sec-Fetch-Mode: navigate`,
  or, when Sec-Fetch headers are absent, an `Accept` that includes
  `text/html`) gets 401 with the HTML gate page and `Cache-Control: no-store`.
  Every other request, including a WebSocket handshake before the upgrade,
  gets 401 JSON `{"error":"login_required"}`. The gate page is
  self-contained (inline or under an exempt path) and carries the requested
  path plus query as `next`. `next` is validated as same-origin: it starts
  with `/` and not `//`.
- **Exchange.** A navigation with a `login=<code>` query parameter goes
  through these cases:
  - valid code: set the cookie, consume the code, and answer 303 to the same
    URL without the parameter;
  - invalid code but a valid cookie: answer 303 to the clean URL;
  - neither: show the gate with an "expired or used" notice.
  The gate also posts a code to an exchange route (name chosen by the
  implementer, under `/auth/`). That route requires the daemon's own Origin,
  accepts a bare code or a pasted full login link, sets the cookie and
  answers with the validated `next`. Failed exchanges are rate-limited (for
  example 10 a minute across the daemon, then 429 with `Retry-After`).
- **Strict re-entry.** A link clicked on another website arrives without the
  SameSite=Strict cookie. On load, the gate page probes a same-origin status
  route; the probe carries the cookie. If the cookie is valid, the page
  replaces itself with `next` plus `location.hash`.
- **Minting.**
  - A login-link route for the Settings button and machine callers. A
    browser call needs the cookie (guard) plus `TrustedWrite`. A machine call
    needs a loopback peer, no Origin and the endpoint bearer, the same checks
    as `/mcp` (`proxy.go:150-161`). It answers
    `http://127.0.0.1:<port><path>?login=<code>`.
  - A bearer-only cookie route for Electron. It has the same machine checks
    and answers the cookie name, value and Max-Age.
  - An in-process `LoginLink` hook that the daemon passes into each hosted MCP
    server (`cmd/ws-mcp/proxy.go`, where `opts` are built). `dashboardPageURL`
    uses it for the `whiteboard.update` `full:` line. Without the hook the
    line keeps the plain URL.
- **Settings.** For a local viewer, the Settings popover gets a "Copy browser
  login link" button. It calls the login-link route and copies the result
  with `copyText`.
- **Other changes.**
  - Add the new cookie to `dashboardCookies` (`webcard_proxy.go:579`).
  - Exempt `/pwa/*` GET/HEAD.
  - Make large framed topic-body images load without a cookie (Constraints).
  - Have `SendNudge` and the roster client send the endpoint bearer, which
    the routes do not require.
  - Update the access-model comments in `access.go` and `daemon.go`
    (`Handler`) and the route comment in `wsagent/http.go` (lines 42-63),
    which says a non-browser client needs no credential.

Verification expectations (Go, `agents-plugin-tool`):

- Gate:
  - A loopback navigation without a cookie gets 401 with the HTML gate and
    `no-store`; an XHR/fetch gets 401 JSON.
  - A WebSocket handshake to `/events`, a session stream and a terminal
    without a cookie gets 401 and no upgrade. With the cookie, each upgrades.
  - `next` keeps path and query, and a `//host` or absolute `next` is
    refused.
- Exchange:
  - A valid code sets a cookie with HttpOnly, SameSite=Strict, Path=/ and
    Max-Age=34560000, then 303s to the clean URL.
  - The same code fails a second time (single use).
  - A code older than 10 minutes fails (inject the clock).
  - A pasted full link works in the POST exchange.
  - An invalid code with a valid cookie 303s to the clean URL.
  - After the failure threshold the route answers 429.
  - The exchange POST is refused with a foreign Origin.
- Cookie:
  - The cookie value is not the secret.
  - Deleting the secret file invalidates existing cookies without a restart.
  - The secret file is 0600 on Unix.
  - A daemon restart keeps cookies valid, because the secret persists.
- LAN:
  - In LAN mode, a loopback request with only the LAN token (query or
    cookie) is refused.
  - A non-local request with the LAN token still passes.
  - A LAN-token request still fails `TrustedWrite` on lead spawn, terminal
    create and sysopen (404).
  - The localhost cookie presented by a non-local peer does not admit it.
- Machine routes unchanged. Without any cookie:
  - `/health` answers 200;
  - `/mcp` with the endpoint bearer upgrades and without it answers 401;
  - `/shutdown` with the bearer answers 202;
  - `/nudge` answers 204/404 as before, with and without the new bearer;
  - `/session-hire/roster` answers 200;
  - `/agents/by-token/<op>` with a capability token works as before.
  Run the existing proxy, ensure and pi_forward tests unchanged.
- Agent routes gated: `POST /agents`, `POST /agents/<id>/send` and
  `POST /support/stores/<id>/send` with a JSON body and no Origin but no
  cookie get 401. The same requests with the cookie behave as before.
- Frames unchanged:
  - `publicAsset` and the `fs/cap` and `fs/view` capability routes answer
    without a cookie, as today.
  - A topic-body image over 2 MiB still renders in its frame through the
    host-page relay, with the cookie on the host fetch.
  - `/pwa/manifest.webmanifest` answers without a cookie.
- Minting:
  - The browser login-link route needs the cookie plus the own Origin.
  - The bearer login-link and cookie routes refuse an Origin, a non-loopback
    peer and a wrong bearer.
  - `whiteboard.update` with the hook prints `full:` with a `login=` code that
    exchanges once. Without the hook it prints the plain URL.
- Web card proxy: the new cookie is stripped from proxied requests.
- Regression: existing dashboard tests pass, moved onto the cookie helper
  where they hit gated routes. `go test ./...` passes.

### Result (this-commit) - 2026-11-17

Implemented the deny-by-default cookie guard, persisted owner-only HMAC secret,
constant-time checks, bounded ten-minute single-use codes, exchange throttling,
Strict re-entry probe, native cookie/link endpoints, hosted MCP login-link hook,
and local-only Settings copy action. Machine, static-frame, PWA and capability
exemptions retain their handler checks; the web-card proxy strips the new cookie.
New nudge and roster clients send the endpoint bearer without requiring it.

Owner feedback (1239) is applied: Settings explicitly names clipboard copying
before the click, with the approved Korean explanatory subtext; clipboard access
never runs on page load. The subsequent owner clarification adds `/auth/clipboard`:
local peer plus Host, own Origin and fetch metadata, independent two-second and
ten-per-minute throttles, no credential in its response, and an injectable native
writer (pbcopy, Unicode-safe PowerShell stdin, Linux writer fallback). Failed writes
revoke their unused code. The updated gate implements A/A2/A3/A4/B/C/C′/D, including
the retry-enabled A3 and small A2 recopy technical calls. Chromium verification
and visual comparison are retained for Phase 3.

Verification: narrow `TestLocalhostAuth` tests pass, including `-race`; all i18n
tests pass. MCP login-link, whiteboard pointer, machine-client, session lifecycle,
stream, hiring and Pi forwarding batches pass (22 top-level tests plus ten
subtests); existing proxy/ensure batch passes (23 plus 21); cmd hook/tracker tests
pass (two). Adapted wsagent REST/support/Pi runner routes pass (21 plus 13).
The full allowed wswhiteboard package passes. Existing dashboard callers now use
real fixture cookies rather than a shipped bypass. All 595 inventoried affected
existing tests ran in named batches (614 unique roots including adjacent prefix
matches); the final command inventory is recorded with Phase 3 verification.

Corrections against the drafted survey: the base already enforces an Origin and
fetch-metadata check on `/nudge` and already relays large framed images through
the host page; those protections remain unchanged. The temporary-capture and
narrow-test instructions supersede the plan's tracked captures/full-suite command.
The allowed full wsagent run found an unrelated lead-replay timeout, reproduced
in isolation (two of three reruns pass); it is not changed here. Seven existing
initial Chromium regression failures, and two additional browser failures plus
one shell Node fixture failure, were reproduced against an untouched base archive,
without weakening their assertions. Overall compatibility remains T1: old machine
callers use the unchanged exemptions, old browser shells display the gate, and
old daemons ignore the new cookie/bearers without any shared-store migration.

### Phase 2: Electron cookie injection and menu item

Depends on Phase 1.

- In `connect()`, after `connectDaemon` returns and before `navigate`,
  Electron reads the endpoint token. It does this the same way as the
  shutdown path: immediately before the call, checked against the observed
  port and PID (`endpoint.js:48-55,114-128`). It then posts to the
  bearer-only cookie route and calls `session.defaultSession.cookies.set`
  for the pinned origin with the returned name and value, `httpOnly: true`,
  `sameSite: 'strict'` and an expiration matching Max-Age (1230).
- A 404 (an older daemon with no gate) loads without a cookie. Any other
  failure still loads, and the page shows the gate. Retry and "Replace
  daemon" run the step again.
- The token never leaves the main process, and `inspect`/`discover` still
  return no credential. Update the comment at `endpoint.js:42-43` to name the
  new reader.
- Add "Copy browser login link" to the Connection submenu (`main.js:332-335`).
  It posts to the bearer login-link route and writes the link with Electron's
  `clipboard`, which is passed into `start()` like the other Electron modules
  for testability.

Verification expectations (`desktop-electron` node tests):

- With a fake daemon that serves the cookie route, `cookies.set` is called
  with the right url, name, value, `httpOnly`, `sameSite` and expiration
  before `loadURL`. Assert the call order.
- A 404 cookie route still calls `loadURL`, with no `cookies.set`.
- A changed endpoint identity between inspect and the cookie call is not sent
  the token.
- The menu item copies the link that the fake route returns. On failure the
  clipboard is left untouched.
- `discover`/`inspect` results still carry no token.
- The existing `main.test.js`, `endpoint.test.js` and `daemon-controller`
  tests pass.

### Result (this-commit) - 2026-11-17

Implemented checked-identity Electron cookie injection before navigation on both
window layouts, repeated on Retry and Replace. The bearer is re-read immediately
before use and checked against the observed port, process identity and credential;
neither discovery nor renderer IPC returns it. Native cookies are HttpOnly,
SameSite Strict, root-scoped and expire with the daemon's Max-Age. A 404 from an
older daemon, or any other authentication/injection failure, still navigates.

The Connection menu now has a separator and the owner-approved (1239) label
“Copy Browser Login Link to Clipboard”. Only an explicit menu click mints and
copies a link, preserving the current path, query and fragment; failures leave
the clipboard untouched. A payload-free preload event asks the dashboard to
show the same localized success toast as Settings, without exposing the link.

Verification: `cd agents-plugin-tool/desktop-electron && npm test` passes all
123 Node tests, including cookie-before-load ordering, retry/replacement,
old-daemon fallback, changed identity, click-only copying and bounded preload
notifications. No Electron process was launched. The parent also reran the named
Go page/stream/script/terminal/visibility regression batch successfully.

### Phase 3: Gate UI per mockup and Chromium acceptance

Depends on Phase 1. It can run in parallel with Phase 2.

- Build the gate page to match
  `ai-docs/assets/mockups/261116/localhost-token-gate/`:
  - a `paste` listener on the document submits pasted text from anywhere on
    the page, with no field focus needed (Ctrl+V and Cmd+V);
  - a clipboard button reads with `navigator.clipboard.readText` on click and
    submits;
  - a visible fallback field;
  - error states for an expired or used code and for rate limiting;
  - a hint naming where a link comes from (the agent's dashboard link,
    Settings in an already logged-in window, Werkschaft's "Copy browser login
    link");
  - light and dark themes.
  The gate never reads the clipboard on load.
- An open dashboard page that gets `login_required` from a post or stream
  reconnect reloads its top window into the gate, instead of failing quietly.

Verification expectations:

- Chromium acceptance, following the repo's existing headless browser test
  pattern:
  - Ctrl+V of a valid login link with no element focused lands on `next`
    with the cookie set.
  - The clipboard button, with clipboard-read granted, does the same.
  - An expired code shows the expired state.
  - A link opened from a cross-site page with an existing cookie
    auto-continues through the Strict re-entry probe.
  - The `#/...` fragment survives the login.
  - A logged-in page whose cookie was invalidated (secret file deleted)
    reloads into the gate on its next post.
- Side-by-side capture comparison of the gate (light and dark, and the error
  state) against the mockup, with captures stored under the ticket's assets
  folder.
- i18n table tests pass with the new keys in every table.

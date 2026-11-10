---
title: "Dashboard page streams move from SSE to WebSocket so they stop using up the browser's per-host connections"
sage-review-design: completed
sage-review-completeness: completed
related:
  261110-bug-ws-dashboard-daemon-lock-scope: "sibling from the same stall investigation; independent, implemented in parallel (user, whiteboard (632) a)"
  261110-bug-ws-dashboard-render-and-frontend-load-accumulation: "its hidden-tab rule (streams stay open, region swaps deferred) is kept unchanged"
  261110-bug-ws-dashboard-daemon-read-amplification: "owns the server-side read caching excluded here"
sage-review-design-reviewed: 315bd8abac9d953a
sage-review-completeness-reviewed: 315bd8abac9d953a
---

# Dashboard stream connection budget: SSE to WebSocket

## Background

The user reported that Enter in the support-agent chat sometimes gets no
response at all (whiteboard, topic "데몬 응답 지연 / 서포트 채팅 무응답").
A read-only investigation on the Windows daemon found the daemon answering
every sampled request in under 600 ms, while the dashboard browser (Orca,
an Electron app, so Chromium's network stack) held 7 connections to the
daemon, 5 of them open SSE streams. Chromium opens at most 6 HTTP/1.1
connections per host, and the daemon serves HTTP/1.1 only (browsers use
HTTP/2 only over TLS). With 5 streams open, every other request of every
tab (page GETs, tracker views, `/check`, git routes, the support send)
shares the one connection left, and with 6 streams they wait until a
stream closes. The send then queues inside the browser, which matches
"no response" without any server-side stall. This is inferred, not
observed from the browser side.

Two page behaviors use the connections up
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`):

- Every session or index tab keeps one stream open (`connect`, ~L3823),
  hidden tabs included: a hidden tab holds region updates without applying
  them, and the stream stays open so the light `tracker` events still act
  (~L3846).
- The support-agent chat opens a second stream for its conversation
  (`supOpen`, ~L3325: `<conv page>/events?regions=entries,convlive`) while
  the chat shows.

## Decisions

1. **The dashboard's live streams use WebSocket instead of SSE (user,
   whiteboard (636) a).** Chromium's six-per-host limit applies to its
   HTTP/1.1 connection pool; WebSocket connections are counted in a
   separate pool, so open streams no longer starve page GETs, tracker
   views, and the support send. Every `EventSource` the page opens moves:
   the page stream (`connect`, session and index pages) and the support
   chat's conversation stream (`supOpen`). The messages keep today's
   meaning and payloads (region HTML with its hash, the mode
   pseudo-region, `tracker` nudges with the `-1` sentinel on connect,
   `gone`, and the rest the SSE stream sends), and the query parameters
   (`h`, `regions`) keep theirs. This reverses
   260518-feat-ws-dashboard-activity-watch-stream's "prefer SSE for
   read-only feed events; WebSocket only for a concrete bidirectional
   need": the need here is the browser's connection pool, not
   bidirectionality, and the user chose it ((636) a).
2. **Nothing visible changes.** Hidden tabs keep their streams and defer
   region swaps (261110-bug-ws-dashboard-render-and-frontend-load-accumulation
   Decision 10 stands), the tab label and summary toast work as today,
   the support chat keeps its own stream and its per-connection
   first-entries-region-is-history rule (`primed`, b181144e) and the
   `c.es === es`-style batch guard, now tied to the WebSocket object, and
   live `ui` markers act on a hidden tab as today (88326de5). The page
   reconnects after a drop with today's fixed delays (3 s for the page
   stream, 2 s for the support chat; backoff is acceptable), resending
   the held hashes in `h` and re-syncing the tracker revision
   (`trackerLost`), and the connection indicator (`connected`) behaves as
   today. A close that follows `gone` is final, as `es.close()` is today,
   and `onerror` plus `onclose` schedule at most one reconnect; a page
   gone before its first render still answers 404 before the upgrade, so
   the page keeps retrying as it does today. Rejected: the SSE-side fixes worked out first (the support chat
   riding the page stream, hidden tabs closing their streams after a
   grace period with ~60 s brief reconnects, a chat-open exemption;
   (632) a, (634) b, (635) a), which (636) a replaced because WebSocket
   removes the constraint they worked around; HTTP/2, which browsers use
   only over TLS and would need a localhost certificate.
3. **The server answers both transports on the same routes.** A stream
   route upgrades a WebSocket handshake and otherwise serves SSE as
   today, so a tab still running the old page script after a daemon
   upgrade keeps working until it reloads, and the existing SSE tests
   keep their meaning. The WebSocket side reuses the stream's existing
   render and wake loop (`d.stream`); only the framing differs. The
   implementer picks the frame format (for example a JSON envelope
   `{event, data}` or a name-prefixed text frame) and records it in the
   phase Result. A hijacked connection's request context no longer ends
   when the peer leaves (the SSE loop ends on `r.Context().Done()` or
   `d.stop`, not on write errors), so the WebSocket side runs its own read
   loop for close frames and an application-level heartbeat the page
   answers, with a read deadline that ends the goroutine when a peer stops
   answering (`x/net/websocket` handles pongs inside `Conn.Read` and does
   not surface them).
   The upgrade refuses a handshake whose `Origin` header is present and
   is not the daemon's own (the `sameOrigin` rule the write routes use,
   `inbox_routes.go`), because a WebSocket handshake is not covered by
   CORS: without it any page open in the user's browser could read the
   dashboard streams from loopback, which it cannot do with SSE today. A
   handshake without `Origin` (a non-browser client, tests) is allowed,
   as browsers always send one, so the check is "Origin absent, or
   `sameOrigin(r)`" (`sameOrigin` alone refuses a missing Origin, and
   `x/net/websocket`'s default `Handler` does too, so the upgrade uses
   `websocket.Server` with its own `Handshake` or an in-tree upgrade). The
   access guard wrapping the mux applies to the upgrade as to every route:
   `lanGuard` (token query or cookie) in LAN mode, `hostGuard` (loopback
   Host) otherwise; the `access.go` comment that names EventSource
   carrying the cookie is updated with it.
4. **No new module dependency.** The WebSocket server uses
   `golang.org/x/net/websocket` (`golang.org/x/net` is already in
   `go.mod`) or a small in-tree implementation; adding another module
   needs its reason in the commit's `## AI Context`.
5. **The premise is checked first.** Before the port, the implementer
   confirms with a Chromium network stack (headless Chromium, Electron,
   or a Chromium-based browser it can drive) that with 6 or more open
   WebSocket connections to a local HTTP/1.1 server a plain `fetch` to
   the same host completes promptly, and that with 6 open SSE streams it
   does not. If WebSockets also block, the implementer stops and reports
   (the ticket's premise is broken); if no Chromium can be driven, it
   stops and reports so the lead can run the probe with the user.

## Constraints

- No server-side lock or caching change belongs here:
  `261110-bug-ws-dashboard-daemon-lock-scope` and
  `261110-bug-ws-dashboard-daemon-read-amplification` own those.
- New UI strings, if any, go to every table under
  `internal/wsi18n/tables/` (Korean in `ko.json`).
- `nice -n 19 node --check` on changed scripts is fine locally; never run
  go build/test/vet on the lead machine; run them remotely:
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- test
  ./internal/wsdashboard/... -count=1 -timeout 30m`.
- Never touch the user's dashboard daemons on ports 29417 / 45743.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin-tool/)

## Prior Decisions

- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Decisions 10): "Frontend: a hidden tab keeps its streams and defers heavy region swaps. While `document.hidden`, the main EventSource stays open; a region event ... records only its latest HTML and hash per region" — bearing: constrains
- b181144e (2026-11-10, commit, 261110-feat-ws-tracker-support-agent-ui-commands): "The flag is per-EventSource, so a first open, a reload, and a reconnect all treat their first entries region as history. ... supActUI checks c.es === es before each command." — bearing: constrains
- 7e99671b (2026-11-09, commit, 261108-feat-ws-tracker-support-agent-chat): "The chat reuses the session SSE with a region filter instead of a new chat-specific route; the stream is open only while the chat tab shows." — bearing: constrains
- 057a2ab0 (2026-11-08, commit): "The nudge rides the existing page streams (a browser holds six connections per origin across tabs), with a -1 sentinel so revision 0 goes out on connect ... Rejected: a separate tracker SSE stream." — bearing: supports
- ef0138ad (2026-11-08, commit, 261108-feat-ws-tracker-drawer-ui): "The tracker nudge rides the session page's existing SSE stream as an `event: tracker` rather than a second EventSource: browsers cap HTTP/1.1 at six connections per origin across tabs" — bearing: supports
- 75ff0c2b (2026-11-06, commit, 261106-feat-ws-dashboard-daemon-core): "One broker wakes every stream on any index change ...; each stream re-renders at most every 250 ms and every 15 s regardless, since liveness windows follow the clock." — bearing: constrains
- 260518-feat-ws-dashboard-activity-watch-stream (2026-05-18, Decisions): "Prefer SSE for read-only feed events. Use WebSocket only if implementation records a concrete bidirectional need." — bearing: contradiction-candidate
- 260516-bug-ws-web-terminal-websocket-transport (2026-05-16, Decisions): "The primary live browser terminal transport must be an owner-authenticated WebSocket, not HTTP output polling." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, agents-plugin-tool/internal/wsdashboard/sse.go, agents-plugin-tool/internal/wsdashboard/daemon.go route table, new wsdashboard tests, possibly agents-plugin-tool/go.mod |
| scope.surface | public-interface | the browser-to-daemon stream routes /events and /s/ID/events gain a WebSocket upgrade, a wire contract old and new page scripts depend on |
| scope.new_public_symbol | no | none; the stream code in sse.go is unexported and no new route path is added |
| scope.new_type_contract | yes | WebSocket message framing carrying the SSE event names region, tracker, gone and their JSON payloads |
| scope.test_surface | new-files | WebSocket stream tests and asset harness checks are new; existing SSE coverage in internal/wsdashboard/stream_cache_test.go, pages_test.go, support_chat_test.go must keep passing |
| complexity.reuse_points | confirmed | Daemon.stream at agents-plugin-tool/internal/wsdashboard/sse.go#L341 and its handleSessionEvents/handleIndexEvents callers L208 and L312; golang.org/x/net v0.35.0 already required in go.mod |
| complexity.side_effect_risk | high | every dashboard tab's live updates and the support chat move transport, so a regression stops all live pages |
| risk.correctness | high | reconnect with held hashes, trackerLost resync, per-connection primed rule, c.es batch guard, and ping-driven goroutine teardown must all carry over |
| risk.fit | moderate | dual SSE plus WebSocket serving on one route; Decision 1 records the reversal of the 260518 SSE-first preference |
| risk.test | high | the Decision 5 Chromium probe and the page reconnect and hidden-tab behaviors need a driven browser or scripted harness, not plain Go tests |
| risk.security_or_contract | high | WebSocket handshakes bypass CORS and loopback requests pass hostGuard without the LAN token, so the upgrade needs the Origin check Decision 3 states (Origin absent or sameOrigin at internal/wsdashboard/inbox_routes.go#L118-L131) |

## Phases

### Phase 1: WebSocket page streams

Implement Decisions 1-5, the Decision 5 probe first.

Verification:

- Probe (Decision 5): the result, the Chromium build used, and the
  script are recorded in the phase Result.
- Server tests: over a WebSocket client, the session, index, and support
  conversation streams send the same messages the SSE stream sends for
  the same state (initial regions, a change after a wake, `h` skipping
  unchanged regions, the `regions` filter, the tracker sentinel, `gone`);
  a request without the upgrade still gets SSE; a peer that stops
  answering is dropped and its goroutine ends; a handshake with a foreign
  `Origin` is refused, one with the daemon's own or no `Origin` is
  accepted, and a LAN request without the token is refused as today.
- Asset tests (the existing node-harness asset tests) or a scripted check: the page opens no `EventSource`; it
  reconnects after a drop with `h` and re-syncs the tracker revision; the
  support chat treats its first entries region after each connect as
  history and acts on later `ui` markers, including on a hidden tab; the
  connection indicator shows a drop and a recovery.
- The wsdashboard tests pass remotely through `ws-rgo`.

### Result (a2140c571) - 2026-11-10

Landed: the page stream (`connect`, session and index pages) and the
support chat's conversation stream (`supOpen`) open a WebSocket through one
helper, `openStream` in `dashboard.js`; the page opens no `EventSource`. The
daemon's stream routes (`/events`, `/s/<id>/events`) upgrade a WebSocket
handshake and serve SSE to every other request, over one shared loop in
`sse.go` (`Daemon.stream`, now framing through a `streamSink`: `sseSink`
or `wsSink` in the new `stream_ws.go`). The SSE bytes are unchanged.

Probe (Decision 5), run first: premise holds. Google Chrome for Testing
148.0.7778.96 (Playwright's `chromium-1223` build, `--headless`, Linux)
against a Node `http` HTTP/1.1 server on `localhost`, driven over CDP
`Runtime.evaluate`. Each trial opens N streams, waits until all are open,
then times one `fetch('/ping')` capped at 5 s:

| streams | fetch |
|---|---|
| 5 SSE | 200 in 4 ms |
| 6 SSE | aborted at 5001 ms (never sent) |
| 6 WebSocket | 200 in 3 ms |
| 10 WebSocket | 200 in 4 ms |

The script (`node probe.mjs <chrome binary>` with `<details><summary>probe.mjs</summary>

```js
// Decision 5 probe: with N open SSE streams vs N open WebSockets to one
// HTTP/1.1 host, does a plain fetch to the same host complete promptly?
// Usage: node probe.mjs <chrome-binary>
import http from 'node:http';
import crypto from 'node:crypto';
import { spawn } from 'node:child_process';

const chromeBin = process.argv[2];
const sockets = new Set();
const server = http.createServer((req, res) => {
  if (req.url === '/') {
    res.writeHead(200, { 'content-type': 'text/html' });
    res.end('<!doctype html><title>probe</title>');
  } else if (req.url.startsWith('/sse')) {
    res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-store' });
    res.write('retry: 3000\n\n');
    res.write('event: hello\ndata: {}\n\n');
  } else if (req.url.startsWith('/ping')) {
    res.writeHead(200, { 'content-type': 'text/plain' });
    res.end('ok');
  } else {
    res.writeHead(404); res.end();
  }
});
server.on('connection', (s) => { sockets.add(s); s.on('close', () => sockets.delete(s)); });
server.on('upgrade', (req, sock) => {
  const accept = crypto.createHash('sha1')
    .update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
  sock.write('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
    `Sec-WebSocket-Accept: ${accept}\r\n\r\n`);
  // One unmasked text frame "hi".
  sock.write(Buffer.from([0x81, 2, 0x68, 0x69]));
  sock.on('data', () => {});
  sock.on('error', () => {});
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const port = server.address().port;

const chrome = spawn(chromeBin, ['--headless', '--no-sandbox', '--remote-debugging-port=0',
  '--user-data-dir=' + process.env.PROBE_PROFILE, `http://localhost:${port}/`], { stdio: ['ignore', 'ignore', 'pipe'] });
const wsURL = await new Promise((resolve, reject) => {
  let buf = '';
  chrome.stderr.on('data', (d) => {
    buf += d;
    const m = buf.match(/DevTools listening on (ws:\/\/\S+)/);
    if (m) resolve(m[1]);
  });
  setTimeout(() => reject(new Error('no devtools url: ' + buf)), 20000);
});
const devPort = new URL(wsURL).port;
let target;
for (let i = 0; i < 50 && !target; i++) {
  const list = await (await fetch(`http://127.0.0.1:${devPort}/json/list`)).json();
  target = list.find((t) => t.type === 'page' && t.url.includes(`localhost:${port}`));
  if (!target) await new Promise((r) => setTimeout(r, 200));
}
const cdp = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => cdp.addEventListener('open', r));
let id = 0;
const pending = new Map();
cdp.addEventListener('message', (ev) => {
  const msg = JSON.parse(ev.data);
  if (pending.has(msg.id)) { pending.get(msg.id)(msg); pending.delete(msg.id); }
});
const evaluate = (expression) => new Promise((resolve) => {
  const mid = ++id;
  pending.set(mid, resolve);
  cdp.send(JSON.stringify({ id: mid, method: 'Runtime.evaluate', params: { expression, awaitPromise: true, returnByValue: true } }));
});

// Opens n streams of kind, waits for them all to open, then times one
// fetch with a 5 s cap. Closes the streams afterwards.
const trial = (kind, n) => `(async () => {
  const opened = [];
  const streams = [];
  for (let i = 0; i < ${n}; i++) {
    opened.push(new Promise((res) => {
      if ('${kind}' === 'sse') { const es = new EventSource('/sse?i=' + i); streams.push(es); es.addEventListener('hello', res); }
      else { const ws = new WebSocket('ws://' + location.host + '/ws?i=' + i); streams.push(ws); ws.onmessage = res; }
    }));
  }
  const openAll = await Promise.race([Promise.all(opened).then(() => 'all-open'),
    new Promise((r) => setTimeout(() => r('open-timeout'), 5000))]);
  const t0 = performance.now();
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), 5000);
  let fetchResult;
  try { const r = await fetch('/ping?t=' + Math.random(), { signal: ctl.signal, cache: 'no-store' }); fetchResult = 'status ' + r.status; }
  catch (e) { fetchResult = 'failed: ' + e.name; }
  clearTimeout(timer);
  const ms = Math.round(performance.now() - t0);
  streams.forEach((s) => s.close());
  await new Promise((r) => setTimeout(r, 500));
  return { kind: '${kind}', streams: ${n}, openAll, fetch: fetchResult, ms };
})()`;

for (let i = 0; i < 100; i++) {
  const st = await evaluate('location.href + " " + document.readyState');
  const v = st.result?.result?.value || '';
  if (v.includes('localhost:' + port) && v.endsWith('complete')) break;
  await new Promise((r) => setTimeout(r, 200));
}
const results = [];
for (const [kind, n] of [['sse', 5], ['sse', 6], ['ws', 6], ['ws', 10]]) {
  const r = await evaluate(trial(kind, n));
  results.push(r.result?.result?.value ?? r);
}
const version = await evaluate('navigator.userAgent');
console.log(JSON.stringify({ userAgent: version.result.result.value, results }, null, 2));
cdp.close();
chrome.kill();
for (const s of sockets) s.destroy();
server.close();
process.exit(0);
```

</details>_PROFILE` set to a
scratch profile directory):

<details><summary>probe.mjs</summary>

```js
// Decision 5 probe: with N open SSE streams vs N open WebSockets to one
// HTTP/1.1 host, does a plain fetch to the same host complete promptly?
// Usage: node probe.mjs <chrome-binary>
import http from 'node:http';
import crypto from 'node:crypto';
import { spawn } from 'node:child_process';

const chromeBin = process.argv[2];
const sockets = new Set();
const server = http.createServer((req, res) => {
  if (req.url === '/') {
    res.writeHead(200, { 'content-type': 'text/html' });
    res.end('<!doctype html><title>probe</title>');
  } else if (req.url.startsWith('/sse')) {
    res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-store' });
    res.write('retry: 3000\n\n');
    res.write('event: hello\ndata: {}\n\n');
  } else if (req.url.startsWith('/ping')) {
    res.writeHead(200, { 'content-type': 'text/plain' });
    res.end('ok');
  } else {
    res.writeHead(404); res.end();
  }
});
server.on('connection', (s) => { sockets.add(s); s.on('close', () => sockets.delete(s)); });
server.on('upgrade', (req, sock) => {
  const accept = crypto.createHash('sha1')
    .update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
  sock.write('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
    `Sec-WebSocket-Accept: ${accept}\r\n\r\n`);
  // One unmasked text frame "hi".
  sock.write(Buffer.from([0x81, 2, 0x68, 0x69]));
  sock.on('data', () => {});
  sock.on('error', () => {});
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const port = server.address().port;

const chrome = spawn(chromeBin, ['--headless', '--no-sandbox', '--remote-debugging-port=0',
  '--user-data-dir=' + process.env.PROBE_PROFILE, `http://localhost:${port}/`], { stdio: ['ignore', 'ignore', 'pipe'] });
const wsURL = await new Promise((resolve, reject) => {
  let buf = '';
  chrome.stderr.on('data', (d) => {
    buf += d;
    const m = buf.match(/DevTools listening on (ws:\/\/\S+)/);
    if (m) resolve(m[1]);
  });
  setTimeout(() => reject(new Error('no devtools url: ' + buf)), 20000);
});
const devPort = new URL(wsURL).port;
let target;
for (let i = 0; i < 50 && !target; i++) {
  const list = await (await fetch(`http://127.0.0.1:${devPort}/json/list`)).json();
  target = list.find((t) => t.type === 'page' && t.url.includes(`localhost:${port}`));
  if (!target) await new Promise((r) => setTimeout(r, 200));
}
const cdp = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => cdp.addEventListener('open', r));
let id = 0;
const pending = new Map();
cdp.addEventListener('message', (ev) => {
  const msg = JSON.parse(ev.data);
  if (pending.has(msg.id)) { pending.get(msg.id)(msg); pending.delete(msg.id); }
});
const evaluate = (expression) => new Promise((resolve) => {
  const mid = ++id;
  pending.set(mid, resolve);
  cdp.send(JSON.stringify({ id: mid, method: 'Runtime.evaluate', params: { expression, awaitPromise: true, returnByValue: true } }));
});

// Opens n streams of kind, waits for them all to open, then times one
// fetch with a 5 s cap. Closes the streams afterwards.
const trial = (kind, n) => `(async () => {
  const opened = [];
  const streams = [];
  for (let i = 0; i < ${n}; i++) {
    opened.push(new Promise((res) => {
      if ('${kind}' === 'sse') { const es = new EventSource('/sse?i=' + i); streams.push(es); es.addEventListener('hello', res); }
      else { const ws = new WebSocket('ws://' + location.host + '/ws?i=' + i); streams.push(ws); ws.onmessage = res; }
    }));
  }
  const openAll = await Promise.race([Promise.all(opened).then(() => 'all-open'),
    new Promise((r) => setTimeout(() => r('open-timeout'), 5000))]);
  const t0 = performance.now();
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), 5000);
  let fetchResult;
  try { const r = await fetch('/ping?t=' + Math.random(), { signal: ctl.signal, cache: 'no-store' }); fetchResult = 'status ' + r.status; }
  catch (e) { fetchResult = 'failed: ' + e.name; }
  clearTimeout(timer);
  const ms = Math.round(performance.now() - t0);
  streams.forEach((s) => s.close());
  await new Promise((r) => setTimeout(r, 500));
  return { kind: '${kind}', streams: ${n}, openAll, fetch: fetchResult, ms };
})()`;

for (let i = 0; i < 100; i++) {
  const st = await evaluate('location.href + " " + document.readyState');
  const v = st.result?.result?.value || '';
  if (v.includes('localhost:' + port) && v.endsWith('complete')) break;
  await new Promise((r) => setTimeout(r, 200));
}
const results = [];
for (const [kind, n] of [['sse', 5], ['sse', 6], ['ws', 6], ['ws', 10]]) {
  const r = await evaluate(trial(kind, n));
  results.push(r.result?.result?.value ?? r);
}
const version = await evaluate('navigator.userAgent');
console.log(JSON.stringify({ userAgent: version.result.result.value, results }, null, 2));
cdp.close();
chrome.kill();
for (const s of sockets) s.destroy();
server.close();
process.exit(0);
```

</details>

Decisions taken:

- Frame format: one text frame per event, `{"event": <SSE event name>,
  "data": <the SSE data bytes>}`, built by concatenation so the data bytes
  equal the SSE line's exactly. Heartbeat: the daemon sends
  `{"event":"ping"}` every keep-alive period (`sseKeepAlive`, 25 s, the
  SSE comment period); the page answers `{"event":"pong"}`. The daemon's
  read loop drops a connection that sends no frame within
  `wsReadWindow` (3) periods, so 75 s; a close frame or read error ends it
  at once. Frame writes carry a 10 s deadline; a page frame is capped at
  1 KiB.
- The Origin check runs before any render and only for an upgrade:
  `Origin` absent or `sameOrigin(r)`, else 403. A plain SSE request is
  served as before whatever its Origin (CORS already keeps a foreign page
  from reading it). The upgrade uses `websocket.Server` with no
  `Handshake`, since the check already ran.
- The upgrade needs `http.Hijacker` and SSE `http.Flusher`; a request whose
  writer lacks the one it needs gets 500, as SSE did.
- A stream's write error now ends it on both framings (SSE ignored write
  errors before and ended only on the request context).
- `DaemonOptions.keepAlive` (unexported, test-only, like
  `refreshInterval`) drives the idle-frame period and with it the read
  window.
- The support chat's `c.es`/`c.esFor` became `c.st`/`c.stFor`, the stream
  handle `openStream` returns; the batch guard (`c.st !== st` in `onRegion`
  and `supActUI`) compares that handle. `openStream` calls `drop` at most
  once per connection (error and close both route to it) and never after
  the caller's `close()`, so `gone` followed by the close stays final.
- No new module: `golang.org/x/net/websocket` from the already-required
  `golang.org/x/net v0.35.0`; `go.mod`/`go.sum` unchanged.

Verification:

- `stream_ws_test.go`: `TestWebSocketStreamsSendWhatSSESends` (an SSE and
  a WebSocket stream per path, compared event for event: full session
  page, `?regions=entries,convlive`, `?h=` with every region held, the
  index; on connect with the `{"rev":0}` sentinel, after
  `TrackerChanged`, after a board change, and `gone` then close on the
  session paths), `TestWebSocketHandshakeChecksOriginAndSession` (foreign,
  `null`, and other-port Origin 403; own and absent Origin 101; unknown
  session 404 before the upgrade; a plain request gets SSE),
  `TestWebSocketStreamNeedsTheLANToken` (LAN Host without the token 401,
  with the cookie 101), `TestWebSocketStreamDropsASilentPeer` (a peer not
  answering pings is closed within the window and its broker subscription
  is gone; an answering peer stays past twice the window; a closing peer's
  stream ends).
- `stream_page_test.go` `TestPageStreamsRideWebSocket` (node harness over
  the extracted `openStream`, `connected`/`connect`, `supOpen`/`supClose`,
  `supActUI` with a fake WebSocket and virtual clock): no `EventSource` in
  `dashboard.js`; URL with `h`; ping answered; regions swap, held on a
  hidden tab while `tracker` nudges; a drop turns the indicator off,
  calls `trackerLost`, reconnects once after 3 s with the hashes held
  then, and the indicator recovers on open; `gone` is final; the support
  chat's first entries region per connect is history and later ones act
  on their ui line with the tab hidden; a drop retries once after 2 s;
  `gone` closes for good.
- `ws-rgo ... test ./internal/wsdashboard/... -count=1 -timeout 30m`:
  `ok github.com/kang-sw/devenv/internal/wsdashboard 68.042s`;
  `ws-rgo ... vet ./internal/wsdashboard/`: clean; `node --check
  dashboard.js`: clean.
- Lite review (one pass): no correctness or security finding. Noted for
  the record: a background tab Chromium freezes (Energy Saver, Memory
  Saver) cannot answer pings, so the daemon drops it after 75 s and the
  page reconnects with its held hashes on thaw (the indicator shows the
  drop and recovery); under SSE such a tab kept its stream. Self-healing,
  so left as is.

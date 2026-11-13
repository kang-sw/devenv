#!/usr/bin/env node
// webcard-headless.mjs: headless browser checks for the dashboard's web card
// (webcard.js, its wrapper frame and injected script, and the daemon's
// proxy). A repo-local developer command beside dev.py; it is not shipped.
//
// Run from anywhere in this checkout:
//
//   node scripts/webcard-headless.mjs                  every scenario
//   node scripts/webcard-headless.mjs pick look        the named scenarios, in order
//   node scripts/webcard-headless.mjs --list           the scenario names
//
// Options: --shots DIR (where the look and narrow scenarios save their
// captures; default a fresh webcard-headless-shots-* folder under the OS temp
// dir for look, printed), --keep (keep the fixture's temp dir), --headed (show
// the browser). WEBCARD_TRACE=1 prints every web card route request and, from
// the session page, each card frame load and card message with its time.
//
// It never touches the shared dashboard daemon or its cache. It builds ws-mcp
// from this checkout into a temp dir and starts its own daemon there
// (`ws-mcp dashboard --port 0`) with WS_CACHE_HOME, WS_CONFIG_HOME, and HOME
// pointed into that temp dir, none of the caller's WS_* or XDG_* variables,
// and the assets served live from this checkout
// (WS_MCP_DASHBOARD_DEV_SOURCE). It reads the port from that instance's
// endpoint file; the restart scenarios stop that instance by its own PID and
// start it again on the same port (so the session page keeps its origin and
// its stored cards). At exit it kills only the process it started.
//
// The web card's upstream is a fixture server this script runs on
// 127.0.0.1 (and [::1] on the same port when the machine has it), never a
// real dev server: a page with an HMR-like WebSocket channel the script can
// push an update through, a page that raises errors (a timer throw, an
// unhandled rejection, console.error), and a page exercising localStorage, a
// cookie, a form POST, window.open, alert, a download, and an attempt to
// navigate the top level. The session root holds an HTML file in a
// subdirectory, which the board links, and the board links the fixture
// under localhost, 127.0.0.1, and [::1], and the daemon's own port.
//
// Playwright is resolved from the npx cache (~/.npm/_npx/*/node_modules/
// playwright); there is no package.json and no Playwright dependency. With
// none cached, run `npx playwright install chromium` once.
//
// Each scenario asserts and throws on failure; the run exits non-zero when
// any scenario failed. To add one, append { name, about, run } to SCENARIOS:
// run(t) gets the scenario context (makeContext below: a fresh browser
// context and page, the fixture, and the page helpers).

import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(HERE, "..");
const TOOL = path.join(REPO, "agents-plugin-tool");

// ---------------------------------------------------------------- options --
const argv = process.argv.slice(2);
const opt = { list: false, keep: false, headed: false, shots: "", names: [] };
for (let i = 0; i < argv.length; i++) {
  const a = argv[i];
  if (a === "--list") opt.list = true;
  else if (a === "--keep") opt.keep = true;
  else if (a === "--headed") opt.headed = true;
  else if (a === "--shots") opt.shots = path.resolve(argv[++i] || "");
  else if (a.startsWith("-")) die(`unknown option ${a}`);
  else opt.names.push(a);
}

function die(msg) {
  console.error(`webcard-headless: ${msg}`);
  process.exit(2);
}
function assert(ok, msg) {
  if (!ok) throw new Error(msg);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function until(what, fn, ms = 10000) {
  const end = Date.now() + ms;
  for (;;) {
    let v;
    try { v = await fn(); } catch { v = null; }
    if (v) return v;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(80);
  }
}
function write(file, text) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, text);
}

// ---------------------------------------------------------------- fixture --
// The upstream dev server. hits counts requests by "METHOD /path"; push
// sends an HMR update to every open channel.
const STYLE = `<style>body{font:15px/1.45 system-ui,sans-serif;margin:0;background:#f6f8fa;color:#1f2328}
header{background:#24292f;color:#fff;padding:12px 18px;display:flex;gap:12px;align-items:center}
.pill{padding:2px 9px;border-radius:10px;font-size:12px;background:#6e7781;color:#fff}.pill.on{background:#1a7f37}
main{padding:16px 18px}.card{background:#fff;border:1px solid #d0d7de;border-radius:8px;padding:12px 14px;margin-bottom:12px}
h2{margin:0 0 6px;font-size:18px}ul{margin:6px 0;padding-left:20px}button{font:inherit;padding:4px 10px;margin:2px}
pre{white-space:pre-wrap;margin:0}</style>`;
const PAGES = {
  "/": `<!doctype html><html><head><meta charset="utf-8"><title>Fixture app</title>${STYLE}</head><body>
<header><b>Fixture dev app</b><span id="hmr" class="pill">HMR connecting</span></header>
<main><div class="card"><h2 id="headline">Release dashboard</h2><p id="hmr-text">Initial build</p></div>
<div class="card"><ul><li>Alpha</li><li data-testid="beta-row">Beta</li><li>Gamma</li></ul><button class="cta">Save changes</button></div></main>
<script>
(function () {
  var p = document.getElementById("hmr");
  var s = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/__hmr");
  s.onopen = function () { p.textContent = "HMR connected"; p.className = "pill on"; };
  s.onmessage = function (e) {
    var m = JSON.parse(e.data);
    if (m.type === "update") { document.getElementById("hmr-text").textContent = m.text; window.__hmrUpdates = (window.__hmrUpdates || 0) + 1; }
  };
  s.onclose = function () { p.textContent = "HMR disconnected"; p.className = "pill"; };
})();
</script></body></html>`,
  "/errors": `<!doctype html><html><head><meta charset="utf-8"><title>Errors</title>${STYLE}</head><body>
<header><b>Fixture dev app</b><span class="pill">errors</span></header>
<main><div class="card"><h2 id="err-title">A page that logs errors</h2><p>It throws in a timer, leaves a rejection unhandled, and calls console.error.</p>
<button id="more">Log one more</button></div></main>
<script>
console.error("fixture: console.error call", { code: 42 });
setTimeout(function () { throw new Error("fixture: thrown in a timer"); }, 0);
Promise.reject(new Error("fixture: unhandled rejection"));
document.getElementById("more").addEventListener("click", function () { console.error("fixture: one more"); });
</script></body></html>`,
  "/sandbox": `<!doctype html><html><head><meta charset="utf-8"><title>Sandbox</title>${STYLE}</head><body>
<main><div class="card"><h2>Sandbox probes</h2>
<p id="state"></p>
<button id="store">store</button><button id="cookie">cookie</button><button id="popup">popup</button><button id="alert">alert</button><button id="top">top</button>
<a id="dl" href="/download" download>download</a>
<form id="f" method="post" action="/echo"><input name="msg" value="hello form"><button id="submit">submit</button></form>
<pre id="out"></pre></div></main>
<script>
var out = document.getElementById("out");
function say(s) { out.textContent += s + "\\n"; }
document.getElementById("state").textContent = "stored=" + localStorage.getItem("fixture-key") + " cookie=" + document.cookie;
document.getElementById("store").onclick = function () { localStorage.setItem("fixture-key", "kept"); say("store=" + localStorage.getItem("fixture-key")); };
document.getElementById("cookie").onclick = function () { document.cookie = "fixture_js=yum; path=/; SameSite=Lax"; say("cookie=" + document.cookie); };
document.getElementById("popup").onclick = function () { var w = window.open("/popup", "_blank"); say("popup=" + (w ? "window" : "null")); };
document.getElementById("alert").onclick = function () { alert("fixture alert"); say("alert=returned"); };
document.getElementById("top").onclick = function () {
  try { window.top.location.href = location.origin + "/escaped"; say("top=assigned"); } catch (e) { say("top=" + e.name); }
};
</script></body></html>`,
  "/spa": `<!doctype html><html><head><meta charset="utf-8"><title>SPA</title>${STYLE}</head><body>
<main><div class="card"><h2 id="spa-title">A client-routed page</h2>
<a id="hash" href="#details">details</a> <button id="push">push route</button> <button id="err">log an error</button>
<p id="details">Details section</p></div></main>
<script>
document.getElementById("push").onclick = function () { history.pushState({}, "", "/spa/route?tab=2"); document.getElementById("spa-title").textContent = "Routed view"; };
document.getElementById("err").onclick = function () { console.error("spa: an error"); };
</script></body></html>`,
  "/spa/route": null,
  "/popup": `<!doctype html><html><head><meta charset="utf-8"><title>Popup</title></head><body><p id="p">popup opened</p>
<script>document.getElementById("p").textContent = "popup opened, storage=" + localStorage.getItem("fixture-key");</script></body></html>`,
  "/escaped": `<!doctype html><html><body>escaped the card</body></html>`,
};

function startFixture() {
  const hits = new Map(), sockets = new Set(), seen = { wsOrigin: [] };
  const count = (k) => hits.set(k, (hits.get(k) || 0) + 1);
  const handler = (req, res) => {
    const u = new URL(req.url, "http://fixture");
    count(`${req.method} ${u.pathname}`);
    if (req.method === "POST" && u.pathname === "/echo") {
      let body = "";
      req.on("data", (c) => { body += c; });
      req.on("end", () => {
        res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
        const esc = (s) => String(s || "").replace(/[<&>]/g, (c) => ({ "<": "&lt;", "&": "&amp;", ">": "&gt;" })[c]);
        res.end(`<!doctype html><html><head><meta charset="utf-8"></head><body><pre id="echo">body=${esc(decodeURIComponent(body.replace(/\+/g, " ")))}\ncookie=${esc(req.headers.cookie)}\norigin=${esc(req.headers.origin)}</pre></body></html>`);
      });
      return;
    }
    if (u.pathname === "/download") {
      res.writeHead(200, { "Content-Type": "text/plain", "Content-Disposition": 'attachment; filename="fixture.txt"' });
      res.end("fixture download\n");
      return;
    }
    const page = u.pathname === "/spa/route" ? PAGES["/spa"] : PAGES[u.pathname];
    if (!page) { res.writeHead(404, { "Content-Type": "text/plain" }); res.end("not found"); return; }
    const head = { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" };
    // A dev server that refuses framing: the proxy must strip it.
    if (u.pathname === "/") { head["X-Frame-Options"] = "DENY"; head["Content-Security-Policy"] = "frame-ancestors 'none'"; }
    if (u.pathname === "/sandbox") head["Set-Cookie"] = "fixture_srv=1; Path=/; SameSite=Lax";
    res.writeHead(200, head);
    res.end(page);
  };
  // A minimal WebSocket server (RFC 6455): the handshake, server text
  // frames, and a close on the client's close frame. Enough for an HMR
  // channel.
  const upgrade = (req, sock) => {
    count(`UPGRADE ${new URL(req.url, "http://fixture").pathname}`);
    seen.wsOrigin.push(req.headers.origin || "");
    const key = req.headers["sec-websocket-key"];
    if (!key || !req.url.startsWith("/__hmr")) { sock.destroy(); return; }
    const accept = createHash("sha1").update(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").digest("base64");
    sock.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`);
    sockets.add(sock);
    sock.on("data", (b) => { if ((b[0] & 0x0f) === 8) sock.end(); });
    sock.on("close", () => sockets.delete(sock));
    sock.on("error", () => sockets.delete(sock));
  };
  const frame = (text) => {
    const p = Buffer.from(text);
    const h = p.length < 126 ? Buffer.from([0x81, p.length]) : Buffer.from([0x81, 126, p.length >> 8, p.length & 0xff]);
    return Buffer.concat([h, p]);
  };
  const servers = [];
  const listen = (host, port) => new Promise((resolve, reject) => {
    const s = http.createServer(handler);
    s.on("upgrade", upgrade);
    s.once("error", reject);
    s.listen(port, host, () => { servers.push(s); resolve(s.address().port); });
  });
  return (async () => {
    const port = await listen("127.0.0.1", 0);
    let v6 = true;
    try { await listen("::1", port); } catch { v6 = false; }
    return {
      port, v6, hits, seen,
      hit: (k) => hits.get(k) || 0,
      open: () => sockets.size,
      push: (m) => { for (const s of sockets) s.write(frame(JSON.stringify(m))); },
      close: () => { for (const s of sockets) s.destroy(); for (const s of servers) s.close(); },
    };
  })();
}

// The session root: an HTML file in a subdirectory (site/app.html) with a
// stylesheet beside it, and a notes file.
function seedProject(proj) {
  write(path.join(proj, "ai-docs", "_index.md"), "# webcard fixture\n");
  write(path.join(proj, "site", "style.css"), "body{font:15px/1.45 system-ui,sans-serif;margin:0;background:#fffbea;color:#3b2f00}main{padding:16px 18px}h1{font-size:20px;margin:0 0 8px}\n");
  write(path.join(proj, "site", "app.html"), `<!doctype html><html><head><meta charset="utf-8"><title>Static app</title><link rel="stylesheet" href="style.css"></head>
<body><main><h1 id="static-title">Static site page</h1><p data-testid="static-note">Served from the session root's site/ folder.</p>
<p><a id="next" href="next.html">next page</a></p></main>
<script>console.error("static: a logged error");</script></body></html>
`);
  write(path.join(proj, "site", "next.html"), `<!doctype html><html><head><meta charset="utf-8"><title>Next</title></head><body><h1 id="next-title">Next static page</h1></body></html>\n`);
  write(path.join(proj, "site", ".env"), "SECRET=1\n");
}

// ---------------------------------------------------------------- daemon --
// One isolated instance at a time; restart() stops it by its own PID and
// starts the same binary on the same port.
const inst = { proc: null, bin: "", root: "", env: null, port: 0, stopping: false };
function killDaemon() {
  const d = inst.proc;
  if (d && d.exitCode === null && d.signalCode === null) {
    try { d.kill("SIGTERM"); } catch { /* already gone */ }
  }
}
process.on("exit", killDaemon);
for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => { killDaemon(); process.exit(130); });

function isolatedEnv(root, extra) {
  const env = {};
  for (const [k, v] of Object.entries(process.env)) {
    // Nothing of the caller's ws session or XDG dirs reaches the instance.
    if (!k.startsWith("WS_") && !k.startsWith("XDG_")) env[k] = v;
  }
  Object.assign(env, {
    HOME: path.join(root, "home"),
    WS_CACHE_HOME: path.join(root, "cache"),
    WS_CONFIG_HOME: path.join(root, "config"),
    GIT_AUTHOR_NAME: "webcard", GIT_AUTHOR_EMAIL: "webcard@example.invalid",
    GIT_COMMITTER_NAME: "webcard", GIT_COMMITTER_EMAIL: "webcard@example.invalid",
  });
  return Object.assign(env, extra || {});
}

function build(root) {
  const bin = path.join(root, "bin", process.platform === "win32" ? "ws-mcp.exe" : "ws-mcp");
  console.log("building ws-mcp from this checkout ...");
  execFileSync("go", ["build", "-o", bin, "./cmd/ws-mcp"], { cwd: TOOL, stdio: "inherit" });
  return bin;
}

async function startDaemon(port) {
  const { root, env, bin } = inst;
  inst.stopping = false;
  const log = fs.openSync(path.join(root, "daemon.log"), "a");
  const d = spawn(bin, ["dashboard", "--port", String(port)], { cwd: root, env, stdio: ["ignore", log, log] });
  inst.proc = d;
  d.on("exit", (code, sig) => { if (!inst.stopping) console.error(`webcard-headless: the daemon exited early (${code ?? sig}); see ${path.join(root, "daemon.log")}`); });
  const endpoint = path.join(env.WS_CACHE_HOME, "dashboard", "endpoint.json");
  const ep = await until("the daemon's endpoint file", () => {
    const v = JSON.parse(fs.readFileSync(endpoint, "utf8"));
    return v.pid === d.pid && v.port ? v : null;
  }, 20000);
  inst.port = ep.port;
  const base = `http://127.0.0.1:${ep.port}`;
  await until("the daemon's health", async () => (await fetch(base + "/health")).ok, 20000);
  return base;
}
async function stopDaemon() {
  inst.stopping = true;
  const d = inst.proc;
  if (!d || d.exitCode !== null || d.signalCode !== null) return;
  const gone = new Promise((r) => d.once("exit", r));
  d.kill("SIGTERM");
  if (await Promise.race([gone.then(() => true), sleep(8000).then(() => false)])) return;
  d.kill("SIGKILL");
  await gone;
}

// seedSession registers one session at proj with a whiteboard topic linking
// the fixture and the static page, and makes the daemon index it.
async function seedSession(fx, base) {
  const env = isolatedEnv(inst.root, { WS_RSRC_ROOT: path.join(REPO, "agents-plugin", "rsrc"), WS_SKILLS_ROOT: path.join(inst.root, "no-skills"), WS_MCP_NO_DASHBOARD: "1" });
  const call = (tool, args) => execFileSync(inst.bin, ["call", tool, JSON.stringify(args)], { cwd: fx.proj, env, encoding: "utf8" });
  const fer = JSON.parse(call("ferrule", { root: fx.proj, format: "json" }));
  const P = fx.up.port;
  const body = [
    `Dev server: [on localhost](http://localhost:${P}/), [on loopback four](http://127.0.0.1:${P}/errors), [on loopback six](http://[::1]:${P}/sandbox).`,
    "",
    `The dashboard itself: [own port](http://127.0.0.1:${inst.port}/health).`,
    "",
    "Static page: [site/app.html](site/app.html).",
  ].join("\n");
  const board = `<wb:topic name="Web card fixture"><wb:body>${body}</wb:body>` +
    '<wb:add class="check"><wb:title>Check the web card</wb:title></wb:add></wb:topic>';
  const out = call("whiteboard.update", { session_key: fer.session_key, commands: board });
  const m = /ai-docs\/whiteboards\/([a-z0-9]+)\/index\.html/.exec(out);
  if (!m) throw new Error("no whiteboard reply id: " + out.slice(0, 400));
  fx.rid = m[1];
  fx.root = fer.root;
  await beat(fx, base);
}
// beat writes the session's registration (its heartbeat an hour ahead, so
// it stays live however long the run takes) and an adapter snapshot, and
// nudges the daemon to index it.
async function beat(fx, base) {
  const now = Date.now(), iso = (ms) => new Date(ms).toISOString().replace(/\.\d+Z$/, "Z");
  const dir = path.join(inst.env.WS_CACHE_HOME, "dashboard", "sessions", fx.rid);
  fx.at = fx.at || iso(now);
  write(path.join(dir, "registration.json"), JSON.stringify({ schema_version: 1, reply_id: fx.rid, project: fx.root, harness: "claude", registered_at: fx.at, heartbeat_at: iso(now + 3600_000) }));
  write(path.join(dir, "snapshot.json"), JSON.stringify({ schemaVersion: 1, replyId: fx.rid, adapter: "pi", updatedAt: now, heartbeatMs: 3600000, lead: { phase: "idle" } }));
  const r = await fetch(base + "/nudge", { method: "POST", body: JSON.stringify({ reply_id: fx.rid, project: fx.root }) });
  if (r.status !== 204) throw new Error(`nudge: HTTP ${r.status}`);
  await until("the session page", async () => (await fetch(`${base}/s/${fx.rid}`)).ok, 10000);
}

// ------------------------------------------------------------ playwright --
async function launchBrowser() {
  const npx = path.join(os.homedir(), ".npm", "_npx");
  let dirs = [];
  try { dirs = fs.readdirSync(npx).map((d) => path.join(npx, d, "node_modules", "playwright")).filter((d) => fs.existsSync(path.join(d, "package.json"))); } catch { /* no npx cache */ }
  dirs.sort((a, b) => fs.statSync(b).mtimeMs - fs.statSync(a).mtimeMs);
  const req = createRequire(import.meta.url), errs = [];
  for (const d of dirs) {
    try {
      const pw = req(d);
      return await pw.chromium.launch({ headless: !opt.headed });
    } catch (e) { errs.push(`${d}: ${String(e.message).split("\n")[0]}`); }
  }
  die(`no usable Playwright in the npx cache (${npx}); run \`npx playwright install chromium\` once.\n${errs.join("\n")}`);
}

// --------------------------------------------------------------- helpers --
const WEB = '.bcard[data-kind="web"]';
async function makeContext(browser, fx, o = {}) {
  const context = await browser.newContext({
    viewport: o.viewport || { width: 1700, height: 1000 },
    locale: "en-US",
    colorScheme: o.colorScheme || "light",
    reducedMotion: "reduce",
    acceptDownloads: true,
  });
  const page = await context.newPage();
  if (process.env.WEBCARD_TRACE) {
    // The session page's own view: each card frame load and each card
    // message, with the time since the page started.
    await page.addInitScript(() => {
      if (window.top !== window) return;
      const t0 = Date.now(), log = (s) => console.log(`[trace] ${(Date.now() - t0)} ${s}`);
      new MutationObserver(() => {
        for (const f of document.querySelectorAll("iframe.wc-frame:not([data-traced])")) {
          f.setAttribute("data-traced", "");
          f.addEventListener("load", () => log(`frame load ${f.getAttribute("src") || "(no src)"} connected=${f.isConnected}`));
        }
      }).observe(document, { childList: true, subtree: true });
      window.addEventListener("message", (e) => { if (e.data && e.data.wsWeb) log(`message ${e.data.wsWeb} ${e.data.path || ""}`); }, true);
    });
    page.on("console", (m) => { if (m.text().startsWith("[trace]")) console.log("    " + m.text()); });
  }
  const dialogs = [];
  page.on("dialog", (d) => { dialogs.push(d.message()); d.accept().catch(() => {}); });
  const posts = [];
  page.on("request", (r) => {
    if (r.method() === "POST") posts.push(new URL(r.url()).pathname);
    if (process.env.WEBCARD_TRACE && /\/web/.test(r.url())) console.log(`    ${Date.now() % 100000} ${r.method()} ${r.url().replace(/^http:\/\/[^/]+/, "")} [${r.frame() === page.mainFrame() ? "page" : r.frame().url().replace(/^http:\/\/[^/]+/, "")}]`);
  });
  const t = {
    page, context, fx, dialogs, posts,
    note: (s) => console.log(`  (${s})`),
    url: (host = "127.0.0.1") => `http://${host}:${fx.dport()}/s/${fx.rid}`,
    async open(host) {
      await page.goto(t.url(host));
      await until("the session page's web button", () => page.locator(".wnew").count(), 15000);
      await until("the board's topic", () => t.boardFrame(), 15000);
    },
    cards: () => page.locator(WEB),
    count: () => page.locator(WEB).count(),
    // card is the web card at index i in page order (-1: the last); a
    // scenario with one card uses it, otherwise opened() names the card.
    card: (i = -1) => (i < 0 ? page.locator(WEB).last() : page.locator(WEB).nth(i)),
    // opened runs fn and returns the web card it opened (a new card stands
    // at the head of the flow, so it is found by its id, not its place), or
    // the one card on the page when fn reopened an existing one.
    async opened(fn) {
      const ids = () => page.locator(WEB).evaluateAll((l) => l.map((e) => e.getAttribute("data-card")));
      const before = await ids();
      await fn();
      const id = await until("a new web card", async () => (await ids()).find((x) => !before.includes(x)), 6000);
      return page.locator(`${WEB}[data-card="${id}"]`);
    },
    // plusWeb opens a web card through the "+ Web" popover and returns it.
    async plusWeb(text) {
      return t.opened(async () => {
        await page.click(".wnew");
        await page.waitForSelector(".wpop:not([hidden]) .fpin");
        await page.fill(".wpop .fpin", text);
        await page.click(".wpop .btn.pr");
      });
    },
    // wrapper and inner are the card's wrapper frame and the proxied page.
    async wrapper(card) {
      return until("the card's wrapper frame", async () => {
        const h = await card.locator("iframe.wc-frame").elementHandle();
        const f = h && (await h.contentFrame());
        return f && /\/web\//.test(f.url()) ? f : null;
      });
    },
    async inner(card, want) {
      const w = await t.wrapper(card);
      return until(`the card's proxied page${want ? " at " + want : ""}`, async () => {
        const f = w.childFrames()[0];
        if (!f || f.isDetached() || !/^http:\/\/(127\.0\.0\.1|localhost):\d+\//.test(f.url())) return null;
        if (want && !new URL(f.url()).pathname.endsWith(want)) return null;
        return (await f.evaluate(() => document.readyState === "complete")) ? f : null;
      }, 12000);
    },
    // swap runs fn, which loads the card's wrapper again (a reload, the
    // address bar), and waits for the new proxied page (another frame than
    // the one shown before) and its script: until the registration answers,
    // the old page still shows and still counts as ready.
    async swap(card, fn, want) {
      const w = await t.wrapper(card), old = w.childFrames()[0];
      await fn();
      await until("the card's wrapper to load again", () => { const f = w.childFrames()[0]; return f && f !== old; }, 12000);
      await t.ready(card);
      return t.inner(card, want);
    },
    // ready waits for the card's injected script: pick usable.
    async ready(card) {
      await until("the card's pick to become usable", async () => {
        const b = card.locator(".bc-acts .wc-pick");
        return (await b.count()) && !(await b.isDisabled());
      }, 12000);
    },
    async stored() {
      const raw = await page.evaluate((k) => localStorage.getItem(k), "ws.dashboard.cards." + fx.rid);
      const v = JSON.parse(raw || "null");
      const list = Array.isArray(v) ? v : (v && (v.cards || v.list)) || [];
      return list.filter((c) => c && c.kind === "web").map((c) => c.data);
    },
    async closeWebCards() {
      for (let n = await t.count(); n > 0; n = await t.count()) {
        await t.card(0).locator(".bc-close").click();
        await until("the card to close", async () => (await t.count()) < n);
      }
    },
    // boardFrame is the topic body frame holding the fixture links.
    boardFrame: () => page.frames().find((f) => !f.isDetached() && f !== page.mainFrame() && !/\/web\//.test(f.url()) && f.url().indexOf("127.0.0.1:" + fx.up.port) < 0 && f.parentFrame() === page.mainFrame() && f.childFrames().length === 0 && f.url() !== "about:blank") || null,
    async boardLink(text) {
      return until(`the board link "${text}"`, async () => {
        for (const f of page.frames()) {
          if (f.isDetached()) continue;
          const a = f.locator("a", { hasText: text });
          if (await a.count()) return a.first();
        }
        return null;
      });
    },
    async menu() {
      return until("the link menu", async () => {
        const m = page.locator(".lmenu");
        return (await m.count()) ? (await m.locator("button").allTextContents()) : null;
      }, 4000);
    },
    async menuPick(text) {
      await page.locator(".lmenu button", { hasText: text }).click();
    },
    // newTab runs fn and returns the tab it opened.
    async newTab(fn) {
      const p = context.waitForEvent("page", { timeout: 6000 });
      await fn();
      const tab = await p;
      await tab.waitForLoadState("domcontentloaded").catch(() => {});
      return tab;
    },
  };
  return t;
}

// --------------------------------------------------------------- scenarios --
async function openDev(t, pathname = "/") {
  const card = await t.plusWeb(`127.0.0.1:${t.fx.up.port}${pathname}`);
  await t.ready(card);
  return card;
}

const SCENARIOS = [
  {
    name: "plus-web",
    about: '"+ Web" with a host:port opens a dev card on the proxied page (framing refusal stripped, HMR channel up, an update applied)',
    async run(t) {
      await t.open();
      const n0 = await t.count();
      const card = await openDev(t);
      assert((await t.count()) === n0 + 1, "no new web card");
      const f = await t.inner(card, "/");
      await until("the page's HMR channel", () => f.evaluate(() => document.getElementById("hmr").textContent === "HMR connected"));
      const wsOrigin = t.fx.up.seen.wsOrigin[t.fx.up.seen.wsOrigin.length - 1];
      assert(wsOrigin === `http://127.0.0.1:${t.fx.up.port}`, `the HMR upgrade reached the server with Origin ${wsOrigin}`);
      t.fx.up.push({ type: "update", text: "Hot update applied" });
      await until("the pushed update", () => f.evaluate(() => document.getElementById("hmr-text").textContent === "Hot update applied"));
      const stored = await t.stored();
      assert(stored.some((d) => d.url === `http://127.0.0.1:${t.fx.up.port}/`), `the card stored ${JSON.stringify(stored)}`);
      assert(!stored.some((d) => /\/web\//.test(JSON.stringify(d)) || d.id), "the card stored a proxy URL or an id");
      const proxyPort = +new URL(f.url()).port;
      assert(proxyPort && proxyPort !== t.fx.up.port && proxyPort !== t.fx.dport(), `the page is not on its own proxy origin (${f.url()})`);
    },
  },
  {
    name: "localhost-links",
    about: "a board link click on localhost, 127.0.0.1, and [::1] opens a web card; a link to the daemon's own port does not",
    async run(t) {
      await t.open();
      const P = t.fx.up.port;
      for (const [text, want] of [["on localhost", `http://localhost:${P}/`], ["on loopback four", `http://127.0.0.1:${P}/errors`], ["on loopback six", `http://[::1]:${P}/sandbox`]]) {
        const link = await t.boardLink(text);
        const card = await t.opened(() => link.click());
        if (want.includes("[::1]") && !t.fx.up.v6) {
          console.log("  (no [::1] on this machine: the [::1] card opened; its page is not checked)");
          continue;
        }
        await t.ready(card);
        const stored = await t.stored();
        assert(stored.some((d) => d.url === want), `the ${text} card stored ${JSON.stringify(stored)}, want ${want}`);
        const sub = await card.locator(".bc-sub").textContent();
        assert(sub === want, `the ${text} card's header names ${sub}`);
      }
      const n = await t.count();
      await (await t.boardLink("own port")).click();
      const items = await t.menu();
      await sleep(500);
      assert((await t.count()) === n, "a link to the daemon's own port opened a web card");
      assert(!items.includes("Open as web card"), `the own-port link's menu offers a web card: ${items.join(", ")}`);
      assert(items.includes("Open in new tab") || items.length > 0, `the own-port link's menu: ${items.join(", ")}`);
      await t.page.keyboard.press("Escape");
    },
  },
  {
    name: "localhost-viewer",
    about: "a session page viewed as localhost gets its proxy on localhost too, and its pick reaches the card",
    async run(t) {
      await t.open("localhost");
      const card = await openDev(t, "/");
      const f = await t.inner(card, "/");
      const u = new URL(f.url());
      assert(u.hostname === "localhost", `the localhost viewer's proxy page is on ${u.host}`);
      await card.locator(".bc-acts .wc-pick").click();
      const b = await f.locator("#headline").boundingBox();
      await t.page.mouse.move(b.x + 5, b.y + 5);
      await t.page.mouse.click(b.x + 5, b.y + 5);
      await until("the pick chip", async () => (await t.page.locator("#cchips .pchip").count()) === 1);
    },
  },
  {
    name: "narrow",
    about: "at a one-column width the card shows on the board tab and a pick still reaches the composer",
    contexts: [{ viewport: { width: 900, height: 900 } }],
    async run(t) {
      await t.open();
      const card = await openDev(t, "/");
      const cb = await card.boundingBox();
      assert(cb && cb.width > 300 && cb.height > 200, `the card is not shown: ${JSON.stringify(cb)}`);
      const f = await t.inner(card, "/");
      await card.locator(".bc-acts .wc-pick").click();
      const b = await f.locator("#headline").boundingBox();
      await t.page.mouse.move(b.x + 5, b.y + 5);
      await t.page.mouse.click(b.x + 5, b.y + 5);
      await until("the pick chip", async () => (await t.page.locator("#cchips .pchip").count()) === 1);
      const comp = await t.page.locator("#composer").boundingBox();
      t.note(`at 900px the composer is ${comp && comp.height > 0 ? "shown" : "hidden"} beside the card`);
      if (opt.shots) { fs.mkdirSync(opt.shots, { recursive: true }); await t.page.screenshot({ path: path.join(opt.shots, "narrow-900.png") }); }
    },
  },
  {
    name: "pick",
    about: "the pick toggle shows and hides the hover highlight; a click posts one pick and a wb:pick chip appears",
    async run(t) {
      await t.open();
      const card = await openDev(t);
      const f = await t.inner(card, "/");
      const pick = card.locator(".bc-acts .wc-pick");
      const box = () => f.evaluate(() => {
        const b = [...document.documentElement.children].find((e) => e.tagName === "DIV" && e.getAttribute("aria-hidden") === "true");
        if (!b) return null;
        const r = b.getBoundingClientRect(), s = getComputedStyle(b);
        return { display: s.display, w: Math.round(r.width), h: Math.round(r.height), x: Math.round(r.left), y: Math.round(r.top) };
      });
      const target = f.locator("#headline");
      const tb = await target.boundingBox();
      await pick.click();
      await until("aria-pressed true", async () => (await pick.getAttribute("aria-pressed")) === "true");
      await t.page.mouse.move(tb.x + 10, tb.y + tb.height / 2);
      const own = await f.evaluate(() => { const r = document.getElementById("headline").getBoundingClientRect(); return { w: Math.round(r.width), h: Math.round(r.height), x: Math.round(r.left), y: Math.round(r.top) }; });
      const b1 = await until("the hover highlight", async () => { const b = await box(); return b && b.display === "block" && b.w === own.w ? b : null; }, 4000);
      assert(b1.x === own.x && b1.y === own.y && b1.h === own.h, `the highlight ${JSON.stringify(b1)} is not over #headline ${JSON.stringify(own)}`);
      await pick.click();
      await until("aria-pressed false", async () => (await pick.getAttribute("aria-pressed")) === "false");
      assert(!(await box()), "the highlight stays after the toggle turned pick off");
      // Escape on the page's side ends pick mode (the page has the focus
      // once the pointer pressed into it; here the key goes to its frame).
      await pick.click();
      await until("pick on", async () => (await pick.getAttribute("aria-pressed")) === "true");
      await f.locator(".cta").focus();
      await t.page.keyboard.press("Escape");
      await until("Escape to end pick mode", async () => (await pick.getAttribute("aria-pressed")) === "false", 3000);
      assert(!(await box()), "the highlight stays after Escape");
      // Escape with the focus still on the card's toggle (where a click on
      // it leaves it).
      await pick.click();
      await until("pick on", async () => (await pick.getAttribute("aria-pressed")) === "true");
      await t.page.keyboard.press("Escape");
      await sleep(300);
      t.note(`Escape with the focus on the pick toggle leaves pick mode ${(await pick.getAttribute("aria-pressed")) === "true" ? "on" : "off"}`);
      if ((await pick.getAttribute("aria-pressed")) === "true") await pick.click();
      await until("pick off", async () => (await pick.getAttribute("aria-pressed")) === "false");
      const chips0 = await t.page.locator("#cchips .achip.pchip").count();
      await pick.click();
      await until("pick on again", async () => (await pick.getAttribute("aria-pressed")) === "true");
      const li = f.locator('[data-testid="beta-row"]');
      const lb = await li.boundingBox();
      await t.page.mouse.move(lb.x + 5, lb.y + lb.height / 2);
      await sleep(100);
      await t.page.mouse.click(lb.x + 5, lb.y + lb.height / 2);
      await until("the pick chip", async () => (await t.page.locator("#cchips .achip.pchip").count()) === chips0 + 1);
      await sleep(400);
      assert((await t.page.locator("#cchips .achip.pchip").count()) === chips0 + 1, "a click posted more than one pick");
      const chip = t.page.locator("#cchips .achip.pchip").last();
      const sel = await chip.locator("b").textContent(), title = await chip.getAttribute("title");
      assert(sel === '[data-testid="beta-row"]', `the chip's selector is ${sel}`);
      assert(title.startsWith(`http://127.0.0.1:${t.fx.up.port}/\n`) && title.endsWith("\nBeta"), `the chip's title is ${JSON.stringify(title)}`);
      assert((await pick.getAttribute("aria-pressed")) === "false", "pick mode stays on after a pick");
      assert(!(await box()), "the highlight stays after a pick");
      // The picked click never reached the page: a second plain click lands.
      const cta = f.locator(".cta");
      await cta.click();
      assert((await t.page.locator("#cchips .achip.pchip").count()) === chips0 + 1, "a click outside pick mode posted a pick");
      // Removing the chip.
      await chip.locator("[data-unwpick]").click();
      assert((await t.page.locator("#cchips .achip.pchip").count()) === chips0, "the chip's remove did not remove it");
    },
  },
  {
    name: "console",
    about: '"attach console errors" adds one wb:console chip with the ring\'s lines and resets the badge',
    async run(t) {
      await t.open();
      const card = await openDev(t, "/errors");
      const badge = card.locator(".bc-acts .wc-att .wc-badge");
      await until("the badge to count three errors", async () => !(await badge.isHidden()) && (await badge.textContent()) === "3", 6000);
      await card.locator(".bc-acts .wc-att").click();
      const chip = t.page.locator("#cchips .achip.cchip");
      await until("the console chip", async () => (await chip.count()) === 1);
      const label = await chip.locator("b").textContent(), title = await chip.getAttribute("title");
      const lines = title.split("\n");
      assert(label === "console ×3", `the chip says ${label}`);
      assert(lines[0] === `http://127.0.0.1:${t.fx.up.port}/errors`, `the chip's url is ${lines[0]}`);
      for (const want of ["fixture: console.error call", "fixture: thrown in a timer", "fixture: unhandled rejection"]) {
        assert(lines.some((l) => l.includes(want)), `no line holds "${want}": ${JSON.stringify(lines)}`);
      }
      assert(lines.slice(1).some((l) => l.includes('{"code":42}')), `console.error's object argument is not shown: ${JSON.stringify(lines)}`);
      await until("the badge to reset", () => badge.isHidden(), 3000);
      // A second attach with nothing new adds no chip.
      await card.locator(".bc-acts .wc-att").click();
      await sleep(500);
      assert((await chip.count()) === 1, "an attach with no errors added a chip");
      // A new error counts again.
      const f = await t.inner(card, "/errors");
      await f.locator("#more").click();
      await until("the badge to count one", async () => !(await badge.isHidden()) && (await badge.textContent()) === "1", 4000);
    },
  },
  {
    name: "same-document",
    about: "a hash change or a pushed route keeps the error badge and moves the card's record and address bar",
    async run(t) {
      await t.open();
      const card = await openDev(t, "/spa");
      const f = await t.inner(card, "/spa");
      const badge = card.locator(".wc-badge");
      await f.locator("#err").click();
      await f.locator("#err").click();
      await until("the badge to count two", async () => !(await badge.isHidden()) && (await badge.textContent()) === "2", 4000);
      const P = t.fx.up.port;
      await f.locator("#hash").click();
      await until("the record to follow the hash", async () => (await t.stored())[0].url === `http://127.0.0.1:${P}/spa#details`, 4000);
      await sleep(300);
      assert(!(await badge.isHidden()) && (await badge.textContent()) === "2", `after a hash change the badge shows ${(await badge.isHidden()) ? "nothing" : await badge.textContent()}, want 2 (the page still holds two errors)`);
      await f.locator("#push").click();
      await sleep(500);
      const bar = await card.locator(".wc-url").inputValue();
      const rec = (await t.stored())[0].url;
      assert(rec === `http://127.0.0.1:${P}/spa/route?tab=2` && bar === rec, `after a pushed route the record is ${rec} and the address bar ${bar}`);
      assert(!(await badge.isHidden()) && (await badge.textContent()) === "2", "after a pushed route the badge lost its count");
    },
  },
  {
    name: "open-tab",
    about: '"open in tab" opens the stored url for a dev card and the proxy URL for a static card',
    async run(t) {
      await t.open();
      const dev = await openDev(t, "/");
      let tab = await t.newTab(() => dev.locator(".bc-acts .wc-tab").click());
      assert(tab.url() === `http://127.0.0.1:${t.fx.up.port}/`, `the dev card's tab opened ${tab.url()}`);
      await tab.close();
      const st = await t.plusWeb("site/app.html");
      await t.ready(st);
      const f = await t.inner(st, "/app.html");
      tab = await t.newTab(() => st.locator(".bc-acts .wc-tab").click());
      assert(tab.url() === f.url(), `the static card's tab opened ${tab.url()}, want the proxy URL ${f.url()}`);
      assert(await tab.locator("#static-title").count(), "the static card's tab does not show the page");
      await tab.close();
    },
  },
  {
    name: "reload",
    about: "the reload action re-requests the page; a session-page reload restores the card from localStorage",
    async run(t) {
      await t.open();
      const card = await openDev(t, "/");
      const up = t.fx.up;
      const h0 = up.hit("GET /");
      await t.swap(card, () => card.locator('.bc-acts button[title="Reload"]').click(), "/");
      assert(up.hit("GET /") > h0, "the reload did not request the page again");
      const stored = await t.stored();
      assert(stored.length === 1 && stored[0].url === `http://127.0.0.1:${up.port}/`, `stored ${JSON.stringify(stored)}`);
      const h1 = up.hit("GET /");
      await t.page.reload();
      await until("the restored card", async () => (await t.count()) === 1, 15000);
      await t.ready(t.card());
      assert(up.hit("GET /") > h1, "the restored card did not load its page");
      const f = await t.inner(t.card(), "/");
      await until("the restored page's HMR channel", () => f.evaluate(() => document.getElementById("hmr").textContent === "HMR connected"));
      // The record follows the page the card shows: a page opened from the
      // address bar is the one a restore comes back to.
      const bar = t.card().locator(".wc-url");
      await bar.fill(`http://127.0.0.1:${up.port}/errors`);
      await t.swap(t.card(), () => bar.press("Enter"), "/errors");
      // Moves on the same server only move the wrapper; two in quick
      // succession (within the card's 150ms settle check) register nothing.
      t.posts.length = 0;
      for (const p of ["/", "/errors"]) {
        await bar.fill(`http://127.0.0.1:${up.port}${p}`);
        await t.swap(t.card(), () => bar.press("Enter"), p);
      }
      await sleep(600);
      assert(!t.posts.includes(`/s/${t.fx.rid}/web`), `quick moves on the same server registered again: ${t.posts.join(", ")}`);
      await until("the record to follow", async () => (await t.stored())[0].url === `http://127.0.0.1:${up.port}/errors`);
      await t.page.reload();
      await until("the restored card", async () => (await t.count()) === 1, 15000);
      await t.ready(t.card());
      await t.inner(t.card(), "/errors");
    },
  },
  {
    name: "static-entries",
    about: '"Open as web card" on an HTML file card and on an HTML file link, and a workspace path in "+ Web", open a static card',
    async run(t) {
      await t.open();
      const check = async (how) => {
        const card = t.card();
        await t.ready(card);
        const f = await t.inner(card, "/app.html");
        assert(await f.locator("#static-title").count(), `${how}: the static page is not shown`);
        const stored = await t.stored();
        assert(stored.some((d) => d.path === "site/app.html"), `${how}: stored ${JSON.stringify(stored)}`);
        const bg = await f.evaluate(() => getComputedStyle(document.body).backgroundColor);
        assert(bg === "rgb(255, 251, 234)", `${how}: the stylesheet beside the page did not load (${bg})`);
        await t.closeWebCards();
      };
      // The board's HTML file link: its menu offers the web card.
      await (await t.boardLink("site/app.html")).click();
      let items = await t.menu();
      assert(items.includes("Open as web card"), `the HTML file link's menu: ${items.join(", ")}`);
      await t.menuPick("Open as web card");
      await until("a web card", async () => (await t.count()) === 1);
      await check("the file link's menu");
      // The file card's action.
      await (await t.boardLink("site/app.html")).click();
      items = await t.menu();
      await t.menuPick("Preview");
      const fc = t.page.locator('.bcard:not([data-kind="web"])', { hasText: "app.html" }).last();
      await until("the file card", () => fc.count());
      const act = fc.locator('.bc-acts button[title="Open as web card"]');
      await until("the file card's web action", () => act.count());
      await act.click();
      await until("a web card", async () => (await t.count()) === 1);
      await check("the file card's action");
      // A workspace path typed into "+ Web".
      await t.plusWeb("site/app.html");
      await until("a web card", async () => (await t.count()) === 1);
      await check("a path in + Web");
      // A static card follows its links within the directory, and a dotfile
      // is not served.
      const card = await t.plusWeb("site/app.html");
      await t.ready(card);
      const f = await t.inner(card, "/app.html");
      await f.locator("#next").click();
      const g = await t.inner(card, "/next.html");
      assert(await g.locator("#next-title").count(), "the static page's link did not open next.html");
      const dot = await g.evaluate(async () => (await fetch("/.env")).status);
      assert(dot === 404, `the static listener served .env (HTTP ${dot})`);
      // A static page's pick and console blocks name its workspace path.
      await card.locator(".wc-url").fill("site/app.html");
      const h = await t.swap(card, () => card.locator(".wc-url").press("Enter"), "/app.html");
      const badge = card.locator(".wc-badge");
      await until("the static page's error badge", async () => (await badge.textContent()) === "1", 4000);
      await card.locator(".bc-acts .wc-att").click();
      const cc = t.page.locator("#cchips .cchip");
      await until("the static console chip", async () => (await cc.count()) === 1);
      const ct = (await cc.getAttribute("title")).split("\n");
      assert(ct[0] === "site/app.html" && ct[1] === "static: a logged error", `the static console chip holds ${JSON.stringify(ct)}`);
      await card.locator(".bc-acts .wc-pick").click();
      const sb = await h.locator("#static-title").boundingBox();
      await t.page.mouse.move(sb.x + 5, sb.y + 5);
      await t.page.mouse.click(sb.x + 5, sb.y + 5);
      const pc = t.page.locator("#cchips .pchip");
      await until("the static pick chip", async () => (await pc.count()) === 1);
      const pt = (await pc.getAttribute("title")).split("\n");
      assert(pt[0] === "site/app.html" && pt[1] === "#static-title", `the static pick chip holds ${JSON.stringify(pt)}`);
    },
  },
  {
    name: "sandbox",
    about: "the inner frame keeps storage, cookies, form posts, window.open, alert, and downloads, and cannot navigate the top level",
    async run(t) {
      await t.open();
      const card = await openDev(t, "/sandbox");
      const up = t.fx.up;
      let f = await t.inner(card, "/sandbox");
      const out = () => f.locator("#out").textContent();
      await f.locator("#store").click();
      await until("the store probe", async () => (await out()).includes("store=kept"));
      await f.locator("#cookie").click();
      await until("the cookie probe", async () => /cookie=.*fixture_js=yum/.test(await out()));
      assert((await out()).includes("fixture_srv=1"), `the server's Set-Cookie did not stick: ${await out()}`);
      // alert
      const d0 = t.dialogs.length;
      await f.locator("#alert").click();
      await until("the alert", async () => t.dialogs.length > d0 && (await out()).includes("alert=returned"), 4000);
      assert(t.dialogs[d0] === "fixture alert", `the dialog said ${t.dialogs[d0]}`);
      // window.open escapes the sandbox and keeps the proxy origin's storage.
      const tab = await t.newTab(() => f.locator("#popup").click());
      await until("the popup's page", () => tab.locator("#p").count());
      const ptext = await tab.locator("#p").textContent();
      assert(ptext === "popup opened, storage=kept", `the popup says ${ptext}`);
      assert(new URL(tab.url()).port === new URL(f.url()).port, `the popup opened ${tab.url()}`);
      await tab.close();
      // download
      const dl = t.page.waitForEvent("download", { timeout: 6000 });
      await f.locator("#dl").click();
      const got = await dl;
      assert(got.suggestedFilename() === "fixture.txt", `the download is ${got.suggestedFilename()}`);
      // the top level stays
      const url = t.page.url(), esc0 = up.hit("GET /escaped");
      await f.locator("#top").click();
      await sleep(1200);
      assert(t.page.url() === url, `the page navigated the top level to ${t.page.url()}`);
      assert(up.hit("GET /escaped") === esc0, "the top navigation reached the server");
      const topSays = (await out()).split("\n").find((l) => l.startsWith("top="));
      // storage after a card reload
      f = await t.swap(card, () => card.locator('.bc-acts button[title="Reload"]').click(), "/sandbox");
      const state = await f.locator("#state").textContent();
      assert(state.includes("stored=kept") && state.includes("fixture_js=yum") && state.includes("fixture_srv=1"), `after a reload the page holds ${state}`);
      // form post
      await f.locator("#submit").click();
      const g = await t.inner(card, "/echo");
      const echo = await g.locator("#echo").textContent();
      assert(/body=msg=hello form/.test(echo), `the form post echoed ${echo}`);
      assert(/cookie=.*fixture_js=yum/.test(echo) && /fixture_srv=1/.test(echo), `the form post carried cookies ${echo}`);
      assert(echo.includes(`origin=http://127.0.0.1:${up.port}`), `the form post's Origin reached the server as ${echo}`);
      console.log(`  (top navigation attempt: ${topSays || "no result line"})`);
    },
  },
  {
    name: "refusal",
    about: "a registration refusal shows in the card: the daemon's own port, and a full listener registry (16)",
    async run(t) {
      await t.open();
      // The daemon's own port, typed into "+ Web".
      const card = await t.plusWeb(`127.0.0.1:${t.fx.dport()}/`);
      const note = card.locator(".wc-note[role=alert]");
      await until("the refusal note", async () => !(await note.isHidden()) && (await note.textContent()).length > 0);
      const text = await note.textContent();
      assert(text === `The page could not be opened: port ${t.fx.dport()} is the dashboard's own`, `the note says ${text}`);
      assert(await card.locator("iframe.wc-frame").isHidden(), "the refused card still shows its frame");
      await t.closeWebCards();
      // A full registry: register targets until the route answers 503, then
      // "+ Web" one more. Each is a dev target on an unused port; its
      // listener starts at registration.
      const answers = await t.page.evaluate(async (rid) => {
        const out = [];
        for (let i = 0; i < 20; i++) {
          const r = await fetch(`/s/${rid}/web`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ url: `http://127.0.0.1:${41000 + i}/` }) });
          out.push(r.status);
          if (r.status === 503) break;
        }
        return out;
      }, t.fx.rid);
      assert(answers[answers.length - 1] === 503, `the registry never filled: ${answers.join(" ")}`);
      const full = (await t.plusWeb("127.0.0.1:41999/")).locator(".wc-note[role=alert]");
      await until("the cap note", async () => !(await full.isHidden()) && (await full.textContent()).length > 0);
      const ft = await full.textContent();
      assert(/at most 16 web card targets/.test(ft), `the cap note says ${ft}`);
      console.log(`  (registrations before the cap: ${answers.filter((s) => s === 200).length} took, then ${answers[answers.length - 1]})`);
      // Free the listeners for the scenarios after this one.
      await t.fx.restart();
    },
  },
  {
    name: "restart",
    about: "a static card comes back after a daemon restart: the page reload re-registers its path",
    async run(t) {
      await t.open();
      await t.plusWeb("site/app.html");
      await t.ready(t.card());
      await t.inner(t.card(), "/app.html");
      await t.fx.restart();
      t.posts.length = 0;
      await t.page.reload();
      await until("the restored card", async () => (await t.count()) === 1, 15000);
      await t.ready(t.card());
      const f = await t.inner(t.card(), "/app.html");
      assert(await f.locator("#static-title").count(), "the restored static card does not show its page");
      assert(t.posts.includes(`/s/${t.fx.rid}/web`), `the restored card did not register again: ${t.posts.join(", ")}`);
      // A card left open across the restart re-registers on its own.
      await t.fx.restart();
      t.posts.length = 0;
      await t.swap(t.card(), () => t.card().locator('.bc-acts button[title="Reload"]').click(), "/app.html");
      assert(t.posts.includes(`/s/${t.fx.rid}/web`), "the reload after a restart did not register again");
      // A dev card left open across a restart: moving it to another page on
      // the same server loads the wrapper under its old id, which answers
      // 404; the card registers again once and shows the page.
      await t.closeWebCards();
      const dev = await openDev(t, "/");
      await t.fx.restart();
      t.posts.length = 0;
      await dev.locator(".wc-url").fill(`http://127.0.0.1:${t.fx.up.port}/errors`);
      await t.swap(dev, () => dev.locator(".wc-url").press("Enter"), "/errors");
      assert(t.posts.filter((p) => p === `/s/${t.fx.rid}/web`).length === 1, `the stale wrapper did not register again exactly once: ${t.posts.join(", ")}`);
      assert(await dev.locator(".wc-note").isHidden(), "the re-registered card shows a note");
    },
  },
  {
    name: "heartbeat",
    about: "the wrapper's heartbeat finding its id gone after a restart makes the card register again",
    async run(t) {
      // A fake clock in every frame of this context, so the wrapper's
      // minute-long heartbeat interval can be run forward.
      await t.context.clock.install();
      await t.open();
      const dev = await openDev(t, "/");
      await t.fx.restart();
      t.posts.length = 0;
      await t.page.clock.runFor(61000);
      await until("a heartbeat and a new registration", () => t.posts.includes(`/s/${t.fx.rid}/web`), 8000);
      const beats = t.posts.filter((p) => p.endsWith("/beat")).length;
      assert(beats >= 1, `no heartbeat was posted: ${t.posts.join(", ")}`);
      await t.page.clock.runFor(1000);
      await t.ready(dev);
      await t.inner(dev, "/");
    },
  },
  {
    name: "send",
    about: "a message with a pick and a console chip is held with its picks and consoles, as the daemon stored it",
    async run(t) {
      await t.open();
      const card = await openDev(t, "/errors");
      const f = await t.inner(card, "/errors");
      await until("the badge", async () => (await card.locator(".wc-badge").textContent()) === "3", 6000);
      await card.locator(".bc-acts .wc-att").click();
      await until("the console chip", async () => (await t.page.locator("#cchips .cchip").count()) === 1);
      await card.locator(".bc-acts .wc-pick").click();
      const tb = await f.locator("#err-title").boundingBox();
      await t.page.mouse.move(tb.x + 5, tb.y + 5);
      await t.page.mouse.click(tb.x + 5, tb.y + 5);
      await until("the pick chip", async () => (await t.page.locator("#cchips .pchip").count()) === 1);
      // The draft keeps the chips across a session-page reload.
      await sleep(1200);
      await t.page.reload();
      await until("the restored draft's chips", async () => (await t.page.locator("#cchips .pchip").count()) === 1 && (await t.page.locator("#cchips .cchip").count()) === 1, 10000);
      await t.page.fill("#ctext", "Please look at the picked heading and the errors");
      await t.page.click("#csend");
      await until("the composer to clear", async () => (await t.page.locator("#cchips .achip").count()) === 0, 8000);
      const held = t.page.locator(".hchips");
      await until("the held message's chips", async () => (await held.locator(".pchip").count()) >= 1 && (await held.locator(".cchip").count()) >= 1, 8000);
      assert((await held.locator(".pchip b").last().textContent()) === "#err-title", "the held pick chip names another selector");
      // The daemon's own store of the held entry, in this instance's cache.
      const files = [];
      const walk = (d) => { for (const e of fs.readdirSync(d, { withFileTypes: true })) { const p = path.join(d, e.name); if (e.isDirectory()) walk(p); else if (e.name.endsWith(".json")) files.push(p); } };
      walk(path.join(inst.env.WS_CACHE_HOME, "dashboard", "sessions", t.fx.rid));
      const entry = files.map((p) => { try { return JSON.parse(fs.readFileSync(p, "utf8")); } catch { return null; } }).find((v) => v && Array.isArray(v.picks));
      assert(entry, `no stored entry with picks among ${files.map((p) => path.basename(p)).join(", ")}`);
      assert(entry.picks.length === 1 && entry.picks[0].selector === "#err-title" && entry.picks[0].url === `http://127.0.0.1:${t.fx.up.port}/errors`, `the stored picks ${JSON.stringify(entry.picks)}`);
      assert(Array.isArray(entry.consoles) && entry.consoles.length === 1 && entry.consoles[0].lines.length === 3, `the stored consoles ${JSON.stringify(entry.consoles)}`);
      console.log(`  (stored entry: ${JSON.stringify({ picks: entry.picks, consoles: entry.consoles.map((c) => ({ url: c.url, lines: c.lines.length })) })})`);
      // A cancel gives the held message back to the composer, chips included.
      await t.page.locator(".msg.held .hcancel").last().click();
      await until("the cancelled message's chips back in the composer", async () => (await t.page.locator("#cchips .pchip").count()) === 1 && (await t.page.locator("#cchips .cchip").count()) === 1, 8000);
      const back = await t.page.locator("#cchips .cchip").getAttribute("title");
      assert(back.split("\n").length === 4, `the returned console chip holds ${JSON.stringify(back)}`);
    },
  },
  {
    name: "look",
    about: "LOOK captures, light and dark: the card with HMR active, an active pick highlight, the composer with a wb:pick and a wb:console chip beside the error badge",
    async run(t0, browser) {
      const out = opt.shots || fs.mkdtempSync(path.join(os.tmpdir(), "webcard-headless-shots-"));
      fs.mkdirSync(out, { recursive: true });
      const shots = [];
      const snap = async (loc, name) => {
        const file = path.join(out, name);
        await loc.screenshot({ path: file, animations: "disabled" });
        shots.push(file);
      };
      for (const theme of ["light", "dark"]) {
        // Each theme in a context of its own color scheme: the topic frames
        // follow the page's theme only through its own switch.
        const t = await makeContext(browser, t0.fx, { viewport: { width: 1500, height: 1000 }, colorScheme: theme });
        try {
          await t.open();
          const card = await openDev(t, "/");
          const f = await t.inner(card, "/");
          await until("HMR connected", () => f.evaluate(() => document.getElementById("hmr").textContent === "HMR connected"));
          t.fx.up.push({ type: "update", text: `Hot update applied (${theme})` });
          await until("the update", () => f.evaluate((s) => document.getElementById("hmr-text").textContent === s, `Hot update applied (${theme})`));
          await sleep(200);
          await snap(card, `web-card-hmr-${theme}.png`);
          await card.locator(".bc-acts .wc-pick").click();
          const hb = await f.locator("#headline").boundingBox();
          await t.page.mouse.move(hb.x + 20, hb.y + hb.height / 2);
          await sleep(250);
          await snap(card, `web-card-pick-${theme}.png`);
          await t.page.mouse.click(hb.x + 20, hb.y + hb.height / 2);
          await until("the pick chip", async () => (await t.page.locator("#cchips .pchip").count()) >= 1);
          // The errors page: attach, then two more errors so the badge shows
          // beside the chips.
          await card.locator(".wc-url").fill(`http://127.0.0.1:${t.fx.up.port}/errors`);
          await t.swap(card, () => card.locator(".wc-url").press("Enter"), "/errors");
          const badge = card.locator(".wc-badge");
          await until("the badge", async () => (await badge.textContent()) === "3", 6000);
          await card.locator(".bc-acts .wc-att").click();
          await until("the console chip", async () => (await t.page.locator("#cchips .cchip").count()) >= 1);
          const g = await t.inner(card, "/errors");
          await g.locator("#more").click();
          await g.locator("#more").click();
          await until("the badge again", async () => !(await badge.isHidden()) && (await badge.textContent()) === "2", 4000);
          await t.page.mouse.move(2, 2);
          await t.page.locator("#ctext").fill("Why does this heading log errors?");
          await sleep(200);
          await snap(t.page.locator("#composer"), `composer-chips-${theme}.png`);
          await snap(t.page, `page-card-and-composer-${theme}.png`);
        } finally {
          await t.context.close();
        }
      }
      for (const f of shots) assert(fs.statSync(f).size > 2000, `${f} is empty`);
      console.log(shots.map((f) => `  ${f}`).join("\n"));
    },
  },
];

// ------------------------------------------------------------------- main --
async function main() {
  if (opt.list) {
    for (const s of SCENARIOS) console.log(`${s.name.padEnd(16)} ${s.about}`);
    return 0;
  }
  const picked = opt.names.length ? opt.names.map((n) => SCENARIOS.find((s) => s.name === n) || die(`no scenario ${n} (see --list)`)) : SCENARIOS;
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "webcard-headless-")));
  let failed = 0, browser = null, up = null;
  try {
    inst.root = root;
    inst.env = isolatedEnv(root, { WS_MCP_DASHBOARD_DEV_SOURCE: TOOL });
    for (const k of ["HOME", "WS_CONFIG_HOME"]) fs.mkdirSync(inst.env[k], { recursive: true });
    inst.bin = build(root);
    up = await startFixture();
    const proj = path.join(root, "proj");
    seedProject(proj);
    execFileSync("git", ["init", "-q", "."], { cwd: proj });
    execFileSync("git", ["-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "fixture"], { cwd: proj, env: inst.env });
    let base = await startDaemon(0);
    const fx = { up, proj, base, rid: "", root: "", dport: () => inst.port };
    fx.restart = async () => {
      const port = inst.port, old = inst.proc.pid;
      await stopDaemon();
      base = fx.base = await startDaemon(port);
      assert(inst.port === port, `the restarted daemon took port ${inst.port}, not ${port}`);
      await beat(fx, base);
      console.log(`  (restarted the isolated daemon: pid ${old} -> ${inst.proc.pid}, port ${port})`);
    };
    await seedSession(fx, base);
    console.log(`isolated daemon ${base} (pid ${inst.proc.pid}), fixture upstream 127.0.0.1:${up.port}${up.v6 ? " and [::1]" : ""}, temp ${root}`);
    browser = await launchBrowser();
    for (const s of picked) {
      const t = await makeContext(browser, fx, (s.contexts && s.contexts[0]) || {});
      const start = Date.now();
      try {
        await s.run(t, browser);
        console.log(`ok    ${s.name} (${Date.now() - start}ms)`);
      } catch (e) {
        failed++;
        console.log(`FAIL  ${s.name}: ${e.message}`);
      } finally {
        await t.context.close();
      }
    }
  } finally {
    if (browser) await browser.close();
    const pid = inst.proc && inst.proc.pid;
    await stopDaemon();
    if (up) up.close();
    if (pid) console.log(`stopped the isolated daemon (pid ${pid})`);
    if (opt.keep || failed) console.log(`fixture kept at ${root}`);
    else fs.rmSync(root, { recursive: true, force: true });
  }
  console.log(failed ? `${failed} of ${picked.length} scenarios failed` : `all ${picked.length} scenarios passed`);
  return failed ? 1 : 0;
}

main().then((code) => process.exit(code), (e) => {
  console.error(`webcard-headless: ${e.stack || e}`);
  killDaemon();
  process.exit(1);
});

#!/usr/bin/env node
// Repository-local developer command: headless browser scenarios for the
// dashboard board's motion and scroll behaviour. Not shipped with any plugin
// package.
//
//     node scripts/board-headless.mjs serve [--topics N] [--keep-tmp]
//     node scripts/board-headless.mjs <scenario> --url <session page> [--width W] [--height H] [--headed] [--trace]
//
// Scenarios run in headless Chromium, 1440x900 by default. Each checks every
// part it covers, prints what failed (with a per-frame trace for motion;
// --trace prints it on success too), and exits non-zero on any failure:
//
//   keep      Scroll compensation (261113-feat-ws-dashboard-board-reorder-hold
//             Phase 1). Scrolls to the middle, updates an off-screen topic
//             above and then one below; every topic in view that the swaps do
//             not move must hold its screen y within 1 px through the motion
//             and through the body frame of the topic updated above (a frame
//             the swap brought) growing 1 s later. Then a user
//             scroll during a session must end it (wbHistory.keeping goes
//             null, the session writes no more scrollTop).
//   toolbar   Toolbar placement, counts, placeholder, terminal availability
//             and folder popover (terminal POST is a browser-local fixture;
//             no shell process is started).
//   overview  Folded picker, anchors/restores, interactions, held swaps,
//             palette picks, storage/reload and reduced motion.
//   overview-motion  Per-rAF height/slot/ghost traces, concurrent clock,
//             reverse exit, exact live-rect landing and reduced motion.
//             Use 1900px as well as 1400/420: PIN_WIDE is 1600px.
//
// --case NAME selects an independent overview checkpoint (anchors, picker,
// rail, actions, palette, controls, reload), or a motion checkpoint (enter-top, enter-middle,
// enter-bottom, pins). Without it every check runs; failures stop the run.
// Focused cases help diagnose a blocker without repeatedly hitting it.
// --capture-dir DIR saves screenshots and JSON motion traces. These are
// evidence, not an automated visual comparison. Tests only inject into the
// browser; none of these three scenarios mutates server board data.
//
//   observer  The shared visibility source (wbHistory.watch / seen): a topic
//             scrolled fully out of view fires onOut once, and a topic a swap
//             inserts is watched from that swap on.
//
// The page kind is detected, not named: a page with #tabbar is an adapter
// page (the board column #bcol scrolls); one without is a board-only page
// (.dgrid.board, the document scrolls). Run a scenario against one URL of
// each kind:
//
//     node scripts/board-headless.mjs keep --url <adapter page>
//     node scripts/board-headless.mjs keep --url <board page>
//
// Pages. A scenario needs only --url: any dashboard session page served with
// this tree's assets. /s/<rid> is the iframe shell; the scenario then runs in
// the session page it frames (/s/<rid>?frame=1), as a user sees it. Two ways
// to get one:
//
//   - `serve` (this file): builds ws-mcp from agents-plugin-tool into a temp
//     directory and runs an isolated dashboard daemon there (its own
//     WS_CACHE_HOME and WS_CONFIG_HOME, port chosen by the OS, assets read
//     from this tree through WS_MCP_DASHBOARD_DEV_SOURCE). It seeds two
//     sessions, one with a live adapter snapshot and one without, each with a
//     board several viewports tall, prints both page URLs, and runs until
//     killed (Ctrl-C), then stops the daemon and removes the temp directory.
//     It never touches a daemon already running on this machine.
//   - `python scripts/dev.py rerun daemon` replaces this machine's shared dev
//     daemon with one built from this tree; any session page it serves
//     works. It restarts the daemon other sessions use.
//
// Board updates are injected client-side, so a scenario never writes to the
// server: an init script wraps window.WebSocket to capture the page's stream
// socket, and an update is a board region event built from the board the
// page last received (the last board event seen on the socket, else the
// board region of a fresh GET of the page, checked against the page's region
// hash), with each updated topic's last-change time bumped and the topic
// moved to the head of the open topics, as the server orders them. It is
// delivered through the socket's own onmessage, so dashboard.js's region
// path (onRegion -> swap -> morphBoard) runs unchanged. A new topic can be
// added too (a renamed copy of one on the board). Every injection checks its
// own faithfulness: the morph must treat exactly the updated topics as
// changed (a new section node, a new body frame, or a next frame loading
// beside the old one) and exactly the added ones as new; a board that
// differs from what the page last saw would show up as extra changes.
//
// The scenarios need a dashboard session page (/s/<reply id>): the
// standalone whiteboard file page has no stream to inject into.
//
// Playwright is not a dependency of this repository. It is resolved from
// the npx cache (~/.npm/_npx/*/node_modules/playwright, newest version
// first), or from BOARD_HEADLESS_PLAYWRIGHT (a playwright package
// directory). To get it:
//
//     npx playwright@latest install chromium
//
// Adding a scenario: write `async function scenarioX(ctx)` using the shared
// helpers below (openBoard, waitQuiet, scrollToMiddle, growFrame,
// segments / trace, assertChanged; in the page, window.__bh: update,
// startSampler / mark / stopSampler, layout, scroller, keeping, writesOn),
// with ctx.check for an expectation the run goes on past and assert for a
// precondition, and add it to SCENARIOS.

import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const REPO = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const TOOL_DIR = path.join(REPO, "agents-plugin-tool");

// --- arguments ---------------------------------------------------------------

function parseArgs(argv) {
  const out = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith("--")) { out._.push(a); continue; }
    const key = a.slice(2);
    const next = argv[i + 1];
    if (next === undefined || next.startsWith("--")) out[key] = true;
    else { out[key] = next; i++; }
  }
  return out;
}

class Failure extends Error {}
function assert(cond, msg) { if (!cond) throw new Failure(msg); }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
function log(...a) { console.log(...a); }

// --- Playwright from the npx cache --------------------------------------------

function versionKey(v) {
  const m = /^(\d+)\.(\d+)\.(\d+)(-.*)?$/.exec(v || "");
  if (!m) return [0, 0, 0, 0];
  return [+m[1], +m[2], +m[3], m[4] ? 0 : 1]; // a release sorts after its prereleases
}
function cmpVersion(a, b) {
  const x = versionKey(a), y = versionKey(b);
  for (let i = 0; i < 4; i++) if (x[i] !== y[i]) return x[i] - y[i];
  return 0;
}
function findPlaywright() {
  const dirs = [];
  if (process.env.BOARD_HEADLESS_PLAYWRIGHT) dirs.push(process.env.BOARD_HEADLESS_PLAYWRIGHT);
  const npx = path.join(os.homedir(), ".npm", "_npx");
  let found = [];
  try {
    for (const d of fs.readdirSync(npx)) {
      const p = path.join(npx, d, "node_modules", "playwright");
      try {
        const pkg = JSON.parse(fs.readFileSync(path.join(p, "package.json"), "utf8"));
        if (fs.existsSync(path.join(p, "index.mjs"))) found.push({ dir: p, version: pkg.version });
      } catch {}
    }
  } catch {}
  found.sort((a, b) => cmpVersion(b.version, a.version));
  return dirs.map((dir) => ({ dir, version: "(env)" })).concat(found)[0] || null;
}
async function loadPlaywright() {
  const pw = findPlaywright();
  if (!pw) {
    console.error("board-headless: Playwright not found in the npx cache (~/.npm/_npx/*/node_modules/playwright).");
    console.error("Get it with:  npx playwright@latest install chromium");
    console.error("or point BOARD_HEADLESS_PLAYWRIGHT at a playwright package directory.");
    process.exit(2);
  }
  const mod = await import(pathToFileURL(path.join(pw.dir, "index.mjs")).href);
  return { chromium: mod.chromium || (mod.default && mod.default.chromium), where: pw };
}

// --- serve: an isolated dev daemon with seeded sessions ----------------------

// childEnv is the environment the isolated ws-mcp processes run with: the
// caller's, minus every WS_* variable (a ws session's own keys, roots and
// cache homes must not leak in), plus the isolated homes.
function childEnv(tmp, extra) {
  const env = {};
  for (const [k, v] of Object.entries(process.env)) if (!k.startsWith("WS_")) env[k] = v;
  env.WS_CACHE_HOME = path.join(tmp, "cache");
  env.WS_CONFIG_HOME = path.join(tmp, "config");
  env.HOME = tmp;
  return Object.assign(env, extra || {});
}
function run(cmd, args, opts) {
  const r = spawnSync(cmd, args, Object.assign({ encoding: "utf8" }, opts));
  if (r.status !== 0) throw new Error(`${cmd} ${args.join(" ")} failed (${r.status}): ${(r.stderr || "") + (r.stdout || "")}`.slice(0, 4000));
  return r.stdout;
}

function topicCommands(i) {
  const name = `T${String(i).padStart(2, "0")}`;
  const paras = [];
  for (let p = 0; p < 9; p++) {
    paras.push(`<p>${name} paragraph ${p + 1}: board motion fixture text that wraps over a couple of lines so the topic body frame stands well above a few hundred pixels tall, which is what makes the board overflow the viewport several times over.</p>`);
  }
  return `<wb:topic name="${name}"><wb:body><h3>${name}</h3>${paras.join("")}</wb:body></wb:topic>`;
}

// seedSession makes a project with one ws session and a board of n topics,
// and returns its reply id and canonical root.
function seedSession(tmp, bin, label, n) {
  const proj = path.join(tmp, `proj-${label}`);
  fs.mkdirSync(path.join(proj, "ai-docs"), { recursive: true });
  fs.writeFileSync(path.join(proj, "ai-docs", "_index.md"), `# ${label}\n`);
  run("git", ["init", "-q", "."], { cwd: proj });
  const env = childEnv(tmp, {
    WS_RSRC_ROOT: path.join(REPO, "agents-plugin", "rsrc"),
    WS_SKILLS_ROOT: path.join(tmp, "no-skills"),
    WS_MCP_NO_DASHBOARD: "1",
  });
  const fer = JSON.parse(run(bin, ["call", "ferrule", JSON.stringify({ root: proj, format: "json" })], { cwd: proj, env }));
  let rid = null;
  // One update per topic, so each topic has its own last-change time.
  for (let i = 1; i <= n; i++) {
    const out = run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key, commands: topicCommands(i) })], { cwd: proj, env });
    const m = /ai-docs\/whiteboards\/([a-z0-9]+)\/index\.html/.exec(out);
    if (m) rid = m[1];
  }
  if (!rid) throw new Error(`no whiteboard reply id for the ${label} session`);
  return { rid, root: fer.root, key: fer.session_key };
}

async function waitFor(fn, ms, what) {
  const end = Date.now() + ms;
  for (;;) {
    try { const v = await fn(); if (v) return v; } catch {}
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(100);
  }
}

async function serve(args) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "board-headless-"));
  const n = +(args.topics || 18);
  let daemon = null, beat = null, stopping = false;
  const stop = (code) => {
    if (stopping) return;
    stopping = true;
    if (beat) clearInterval(beat);
    if (daemon && daemon.exitCode === null) {
      daemon.kill("SIGTERM");
      const t = setTimeout(() => { try { daemon.kill("SIGKILL"); } catch {} }, 5000);
      daemon.once("exit", () => clearTimeout(t));
    }
    const done = () => {
      if (!args["keep-tmp"]) fs.rmSync(tmp, { recursive: true, force: true });
      else log(`kept ${tmp}`);
      process.exit(code);
    };
    if (daemon && daemon.exitCode === null) daemon.once("exit", done); else done();
  };
  process.on("SIGINT", () => stop(0));
  process.on("SIGTERM", () => stop(0));
  try {
    const bin = path.join(tmp, "ws-mcp");
    log(`building ws-mcp from ${TOOL_DIR} ...`);
    run("go", ["build", "-o", bin, "./cmd/ws-mcp"], { cwd: TOOL_DIR });
    log(`seeding two sessions (${n} topics each) ...`);
    const adapter = seedSession(tmp, bin, "adapter", n);
    const board = seedSession(tmp, bin, "board", n);
    const dashDir = path.join(tmp, "cache", "dashboard");
    const out = fs.openSync(path.join(tmp, "daemon.log"), "a");
    daemon = spawn(bin, ["dashboard", "--port", "0"], {
      cwd: tmp, stdio: ["ignore", out, out],
      env: childEnv(tmp, { WS_MCP_DASHBOARD_DEV_SOURCE: TOOL_DIR }),
    });
    daemon.on("exit", (code) => { if (!stopping) { console.error(`board-headless: daemon exited (${code}); log: ${path.join(tmp, "daemon.log")}`); stop(1); } });
    const ep = await waitFor(() => {
      const e = JSON.parse(fs.readFileSync(path.join(dashDir, "endpoint.json"), "utf8"));
      return e.pid === daemon.pid && e.port ? e : null;
    }, 20000, "the daemon's endpoint file");
    const base = `http://127.0.0.1:${ep.port}`;
    await waitFor(async () => (await fetch(base + "/health")).ok, 10000, "/health");
    // The registry row (and, for the adapter session, a live snapshot) is
    // what a serving ws-mcp would write; refreshed on a timer so the
    // session stays inside the registry's liveness window.
    const write = () => {
      const now = new Date(), iso = now.toISOString().replace(/\.\d+Z$/, "Z");
      for (const s of [adapter, board]) {
        const dir = path.join(dashDir, "sessions", s.rid);
        fs.mkdirSync(dir, { recursive: true });
        const reg = { schema_version: 1, reply_id: s.rid, project: s.root, harness: "claude", registered_at: s.registeredAt || iso, heartbeat_at: iso };
        s.registeredAt = reg.registered_at;
        fs.writeFileSync(path.join(dir, "registration.json"), JSON.stringify(reg));
      }
      const snap = { schemaVersion: 1, replyId: adapter.rid, adapter: "pi", updatedAt: now.getTime(), heartbeatMs: 3600000, lead: { phase: "idle" } };
      fs.writeFileSync(path.join(dashDir, "sessions", adapter.rid, "snapshot.json"), JSON.stringify(snap));
    };
    write();
    for (const s of [adapter, board]) {
      const r = await fetch(base + "/nudge", { method: "POST", body: JSON.stringify({ reply_id: s.rid, project: s.root }) });
      if (r.status !== 204) throw new Error(`nudge ${s.rid}: HTTP ${r.status}`);
    }
    beat = setInterval(write, 60000);
    // Each page must be the kind it is seeded as.
    for (const [s, wantBar] of [[adapter, true], [board, false]]) {
      // /s/<rid> is the iframe shell; the session page is its frame.
      const html = await waitFor(async () => { const r = await fetch(`${base}/s/${s.rid}?frame=1`); return r.ok ? r.text() : null; }, 10000, `/s/${s.rid}?frame=1`);
      const hasBar = /id="tabbar"/.test(html);
      if (hasBar !== wantBar) throw new Error(`/s/${s.rid}: #tabbar ${hasBar ? "present" : "absent"}, expected ${wantBar ? "present" : "absent"}`);
    }
    log(`daemon pid ${daemon.pid} on ${base} (temp ${tmp})`);
    log(`adapter page: ${base}/s/${adapter.rid}`);
    log(`board page:   ${base}/s/${board.rid}`);
    log("serving; Ctrl-C stops the daemon and removes the temp directory");
  } catch (e) {
    console.error(`board-headless serve: ${e.message}`);
    stop(1);
  }
}

// --- page helpers --------------------------------------------------------------

// INIT runs in every document before its scripts. In the page document (the
// top page, or the session page the iframe shell frames, ?frame=1) it wraps
// WebSocket so the harness reaches the page's stream socket, records the
// board region events the server sends, and spies on scrollTop writes from
// script (a keep session, restoreScroll) per element.
const INIT = () => {
  // The shell's session frame is a child of the top page. Chromium may run
  // this only in the frame's initial about:blank, whose window the
  // same-origin page then reuses, so the frame's URL is not checked here.
  // The body frames are grandchildren and are skipped.
  if (window.top !== window && window.parent !== window.top) return;
  const bh = (window.__bh = { sockets: [], board: null, writes: [] });
  const Native = window.WebSocket;
  class Captured extends Native {
    constructor(...a) {
      super(...a);
      bh.sockets.push(this);
      this.addEventListener("message", (e) => {
        try {
          const m = JSON.parse(e.data);
          if (m && m.event === "region" && m.data && m.data.name === "board") bh.board = { hash: m.data.hash, html: m.data.html };
        } catch {}
      });
    }
  }
  window.WebSocket = Captured;
  const d = Object.getOwnPropertyDescriptor(Element.prototype, "scrollTop");
  Object.defineProperty(Element.prototype, "scrollTop", {
    configurable: true,
    get() { return d.get.call(this); },
    set(v) { bh.writes.push({ el: this, t: performance.now() }); d.set.call(this, v); },
  });
};

// IN_PAGE installs the in-page half of the shared helpers on window.__bh.
const IN_PAGE = () => {
  const bh = window.__bh;
  const q = (sel, root) => Array.from((root || document).querySelectorAll(sel));
  bh.board_ = () => document.getElementById("r-board");
  bh.kind = () => (document.getElementById("tabbar") ? "adapter" : "board");
  bh.scroller = () => {
    const c = document.getElementById("bcol");
    const oy = c && getComputedStyle(c).overflowY;
    return c && (oy === "auto" || oy === "scroll") && c.scrollHeight > c.clientHeight ? c : document.scrollingElement;
  };
  bh.viewTop = (s) => (s === document.scrollingElement ? 0 : Math.max(0, s.getBoundingClientRect().top + s.clientTop));
  bh.viewBottom = (s) => (s === document.scrollingElement ? innerHeight : Math.min(innerHeight, s.getBoundingClientRect().top + s.clientTop + s.clientHeight));
  bh.topics = () => q("section.topic[data-topic]", bh.board_()).filter((n) => !n.classList.contains("closed"));
  bh.section = (name) => bh.topics().find((n) => n.getAttribute("data-topic") === name) || null;
  // Topics by where they stand against the scroller's viewport.
  bh.layout = () => {
    const s = bh.scroller(), top = bh.viewTop(s), bottom = bh.viewBottom(s);
    return bh.topics().map((n) => {
      const r = n.getBoundingClientRect();
      return { name: n.getAttribute("data-topic"), top: r.top, bottom: r.bottom, where: r.bottom <= top ? "above" : r.top >= bottom ? "below" : "in" };
    });
  };
  bh.keeping = () => (window.wbHistory && window.wbHistory.keeping ? window.wbHistory.keeping(bh.scroller()) : null);
  bh.writesOn = (s) => bh.writes.filter((w) => w.el === s).length;
  bh.frames = () => q("iframe.doc-frame", bh.board_());
  // baseBoard is the board HTML the page last received, with its hash.
  bh.baseBoard = async () => {
    const live = bh.board_().getAttribute("data-h");
    if (bh.board && bh.board.hash === live) return bh.board;
    const r = await fetch(location.href, { cache: "no-store", credentials: "same-origin" });
    const page = new DOMParser().parseFromString(await r.text(), "text/html");
    const b = page.getElementById("r-board");
    if (!b || b.getAttribute("data-h") !== live) throw new Error(`the page's board (hash ${live}) is not the served one (${b ? b.getAttribute("data-h") : "none"}); reload the page`);
    return { hash: live, html: b.innerHTML };
  };
  // update builds the next board as the server would order it and
  // delivers it through the stream socket's own onmessage. Each topic in
  // names gets its last-change time set to now (so its section differs from
  // the HTML the page last saw) and moves to the head of the open topics,
  // the last named first. Each name in add is a new topic, made from a copy
  // of topic `from` under the new name (its section, keys and body frame
  // document renamed), put at the head. It reports which topics the morph
  // treated as changed: a topic whose section node, or whose body frame
  // element, is not the one it had, or that loads a next frame beside the
  // old one (frameGraft keeps a changed topic's section and loads its new
  // frame beside the old one).
  bh.update = async (names, add, options) => {
    const base = await bh.baseBoard();
    const t = document.createElement("template");
    t.innerHTML = base.html;
    const box = t.content;
    const sectionIn = (name) => Array.from(box.children).find((n) => n.matches && n.matches("section.topic") && n.getAttribute("data-topic") === name);
    const head = () => Array.from(box.children).find((n) => n.matches && n.matches("section.topic[data-topic]:not(.closed)"));
    const stamp = (sec, at) => {
      const time = sec.querySelector(".tbar time.tago[data-at]");
      if (time) { time.setAttribute("data-at", String(at)); time.setAttribute("datetime", new Date(at).toISOString().replace(/\.\d+Z$/, "Z")); }
      else sec.setAttribute("data-bh-at", String(at));
    };
    let now = Date.now();
    for (const name of names || []) {
      const sec = sectionIn(name);
      if (!sec) throw new Error(`no topic ${name} in the board`);
      if (sec.classList.contains("closed")) throw new Error(`topic ${name} is closed`);
      stamp(sec, ++now);
      if (head() !== sec) box.insertBefore(sec, head());
    }
    for (const a of add || []) {
      const src = sectionIn(a.from);
      if (!src) throw new Error(`no topic ${a.from} in the board`);
      if (sectionIn(a.name)) throw new Error(`topic ${a.name} is already on the board`);
      // Rename every occurrence of the source topic's keys (data-topic,
      // data-key, data-doc, the frame document's wb-doc-key meta) and its
      // shown name.
      const esc = (x) => x.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const html = src.outerHTML.replace(new RegExp(`(["':])${esc(a.from)}(?=["'&])`, "g"), `$1${a.name}`).replace(new RegExp(`>${esc(a.from)}<`, "g"), `>${a.name}<`);
      const tt = document.createElement("template");
      tt.innerHTML = html;
      const sec = tt.content.firstElementChild;
      stamp(sec, ++now);
      box.insertBefore(sec, head());
    }
    const html = t.innerHTML, hash = "bh" + now.toString(16);
    const ws = bh.sockets.find((s) => s.readyState === 1 && typeof s.onmessage === "function");
    if (!ws) throw new Error("no open stream socket on the page");
    const look = (n) => ({ n, frames: q("iframe.doc-frame:not(.doc-frame-next)", n) });
    const before = new Map(q("section.topic[data-topic]", bh.board_()).map((n) => [n.getAttribute("data-topic"), look(n)]));
    bh.mark("swap: " + (names || []).concat((add || []).map((a) => "+" + a.name)).join(", "));
    ws.onmessage({ data: JSON.stringify({ event: "region", data: { name: "board", hash, html } }) });
    bh.board = { hash, html };
    if (options && options.held) {
      if (bh.board_().getAttribute("data-h") === hash) throw new Error("the overview-held swap landed early");
      return { hash, html };
    }
    if (bh.board_().getAttribute("data-h") !== hash) throw new Error("the board swap did not land (held: a hidden tab, an edit, or a drag?)");
    const changed = [], added = [];
    q("section.topic[data-topic]", bh.board_()).forEach((n) => {
      const k = n.getAttribute("data-topic"), was = before.get(k), now = look(n);
      if (!was) { added.push(k); return; }
      const same = was.n === n && was.frames.length === now.frames.length && was.frames.every((f, i) => f === now.frames[i]) && !n.querySelector("iframe.doc-frame-next");
      if (!same) changed.push(k);
    });
    const order = bh.topics().map((n) => n.getAttribute("data-topic"));
    return { changed, added, order };
  };
  // The sampler records, per animation frame, each tracked topic's screen
  // top (getBoundingClientRect: what the user sees, transforms included)
  // and its own running translateY, the scroller's scrollTop, and whether a
  // keep session runs.
  bh.startSampler = (names) => {
    const s = bh.scroller(), t0 = performance.now(), samples = [];
    bh.sampling = { samples, on: true, t0 };
    const take = () => {
      const y = {}, ty = {};
      for (const name of names) {
        const n = bh.section(name);
        y[name] = n ? n.getBoundingClientRect().top : null;
        const m = n && /^matrix(3d)?\((.*)\)$/.exec(getComputedStyle(n).transform || "");
        ty[name] = m ? +m[2].split(",")[m[1] ? 13 : 5] : 0;
      }
      samples.push({ t: performance.now() - t0, st: s.scrollTop, keep: !!bh.keeping(), y, ty });
    };
    take();
    const tick = () => { if (!bh.sampling.on) return; take(); requestAnimationFrame(tick); };
    requestAnimationFrame(tick);
  };
  bh.mark = (label) => { if (bh.sampling && bh.sampling.on) bh.sampling.samples.push({ t: performance.now() - bh.sampling.t0, mark: label }); };
  bh.stopSampler = () => { bh.sampling.on = false; return bh.sampling.samples; };
};

// Browsers openBoard launched; main closes them on every exit path.
const BROWSERS = [];

// openBoard opens url in a fresh headless Chromium, waits for the board, its
// stream socket and its body frames' first heights, shows the board column
// on an adapter page, and returns { browser, page, tab, kind }. url may be the
// iframe shell (/s/<rid>): page is then the session page it frames (a
// Playwright Frame, which fills the window, so its coordinates are the
// tab's), and tab is the browser tab, which owns the mouse.
async function openBoard(ctx) {
  const { chromium } = await loadPlaywright();
  let browser;
  try {
    browser = await chromium.launch({ headless: !ctx.args.headed });
  } catch (e) {
    console.error(`board-headless: Chromium failed to launch: ${e.message.split("\n")[0]}`);
    console.error("Install it with:  npx playwright@latest install chromium");
    process.exit(2);
  }
  BROWSERS.push(browser);
  const tab = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height } });
  tab.on("pageerror", (e) => ctx.errors.push(String(e)));
  await tab.addInitScript(INIT);
  await tab.goto(ctx.args.url, { waitUntil: "load" });
  let page = tab;
  if (await tab.evaluate(() => document.documentElement.hasAttribute("data-shell"))) {
    page = await waitFor(async () => tab.frames().find((f) => f.parentFrame() === tab.mainFrame() && /[?&]frame=1(&|$)/.test(f.url())) || null, 10000, "the shell's session frame");
    await page.waitForLoadState("load");
  }
  await page.evaluate(IN_PAGE);
  await page.waitForFunction(() => document.querySelectorAll("#r-board section.topic[data-topic]").length > 0, null, { timeout: 10000 });
  await page.waitForFunction(() => window.__bh.sockets.some((s) => s.readyState === 1 && typeof s.onmessage === "function"), null, { timeout: 10000 });
  const kind = await page.evaluate(() => window.__bh.kind());
  if (kind === "adapter") {
    const shown = await page.evaluate(() => { const c = document.getElementById("bcol"); return !!c && c.getClientRects().length > 0; });
    if (!shown) await page.click('#tabbar [data-tab="board"]');
  }
  // Body frames size themselves once loaded; wait until each reported.
  await page.waitForFunction(() => window.__bh.frames().every((f) => f.style.height), null, { timeout: 15000 }).catch(() => {});
  await waitQuiet(page);
  return { browser, page, tab, kind };
}

// waitQuiet waits until no keep session runs and the layout has stood still
// for a few frames.
async function waitQuiet(page, ms = 8000) {
  await page.waitForFunction(() => {
    const bh = window.__bh, sig = JSON.stringify(bh.layout().map((t) => Math.round(t.top))) + bh.scroller().scrollTop;
    if (bh.keeping()) { bh.quietSig = null; return false; }
    if (bh.quietSig === sig) { bh.quietN = (bh.quietN || 0) + 1; } else { bh.quietSig = sig; bh.quietN = 0; }
    return bh.quietN >= 5;
  }, null, { timeout: ms, polling: "raf" });
}

// scrollToMiddle puts the scroller at the middle of its range and returns
// the topic layout there.
async function scrollToMiddle(page) {
  await page.evaluate(() => { const s = window.__bh.scroller(); s.scrollTop = Math.round((s.scrollHeight - s.clientHeight) / 2); });
  await waitQuiet(page);
  return page.evaluate(() => window.__bh.layout());
}

// growFrame grows topic name's body frame by px and has the frame report
// its new height to the page the way its own reporter does (doc-frame.js:
// {wbDoc: "height", docKey, height} posted to the parent, which dashboard.js's
// message handler applies). A spacer is appended to the frame's .doc-body
// first, so the frame's own later reports agree. The report is posted
// explicitly because Chromium throttles rendering in an off-screen
// cross-origin frame (a sandboxed srcdoc body frame is one), so its
// ResizeObserver does not run until the frame scrolls into view. Resolves
// once the page sized the frame.
async function growFrame(page, name, px) {
  // A swapped-in frame loads beside the old one (.doc-frame-next) and
  // takes its place once drawn: grow the one shown.
  await page.waitForFunction((n) => !document.querySelector(`#r-board section.topic[data-topic="${n}"] iframe.doc-frame-next`), name, { timeout: 5000 })
    .catch(() => { throw new Failure(`topic ${name}'s next body frame never took its place`); });
  const handle = await page.$(`#r-board section.topic[data-topic="${name}"] iframe.doc-frame`);
  assert(handle, `topic ${name} has no body frame`);
  const before = await handle.evaluate((f) => f.getBoundingClientRect().height);
  const frame = await handle.contentFrame();
  assert(frame, `topic ${name}'s body frame has no document`);
  await frame.evaluate((px) => {
    const body = document.querySelector(".doc-body") || document.body, d = document.createElement("div");
    d.style.height = px + "px";
    d.className = "bh-spacer";
    body.appendChild(d);
    const key = document.querySelector('meta[name="wb-doc-key"]');
    window.parent.postMessage({ wbDoc: "height", height: Math.ceil(body.getBoundingClientRect().height), docKey: key && key.getAttribute("content") }, "*");
  }, px);
  await page.waitForFunction(([el, h]) => el.getBoundingClientRect().height >= h - 1, [handle, before + px], { timeout: 3000 })
    .catch(() => { throw new Failure(`topic ${name}'s body frame did not grow by ${px}px (stayed ~${before}px)`); });
}

// segments splits samples at their marks and reports, per segment and
// tracked topic, the largest |y - baseline| (baseline: the first sample,
// before any mark) and when it happened.
function segments(samples, names) {
  const base = samples.find((s) => s.y);
  const out = [];
  let cur = { label: "before", from: 0, worst: {} };
  out.push(cur);
  for (const s of samples) {
    if (s.mark) { cur = { label: s.mark, from: s.t, worst: {} }; out.push(cur); continue; }
    for (const name of names) {
      if (s.y[name] == null || base.y[name] == null) continue;
      const d = Math.abs(s.y[name] - base.y[name]);
      if (!cur.worst[name] || d > cur.worst[name].d) cur.worst[name] = { d, t: s.t };
    }
  }
  return out;
}
// assertChanged checks an injection's faithfulness: the morph changed
// exactly the topics updated and added exactly the topics added.
function assertChanged(r, changed, added = []) {
  const same = (x, y) => x.length === y.length && x.every((v) => y.includes(v));
  assert(same(r.changed, changed) && same(r.added, added),
    `the injected board changed [${r.changed}] and added [${r.added}], expected changed [${changed}] and added [${added}]: the synthetic board is not what the page last saw`);
}
// trace prints name's per-frame samples, marks included; a run of frames
// where nothing changed prints as one line.
function trace(samples, name) {
  const lines = [];
  let last = null, run = 0;
  const key = (s) => `${s.st.toFixed(1)}|${s.y[name] == null ? "-" : s.y[name].toFixed(1)}|${s.ty[name].toFixed(1)}|${s.keep}`;
  const flush = () => { if (run) lines.push(`         ... ${run} more frame(s) unchanged`); run = 0; };
  for (const s of samples) {
    if (s.mark) { flush(); lines.push(`  ${s.t.toFixed(0).padStart(5)}ms  -- ${s.mark}`); last = null; continue; }
    const k = key(s);
    if (k === last) { run++; continue; }
    flush();
    last = k;
    lines.push(`  ${s.t.toFixed(0).padStart(5)}ms  scrollTop=${s.st.toFixed(1).padStart(8)}  ${name}.y=${s.y[name] == null ? "-" : s.y[name].toFixed(1).padStart(8)}  translateY=${s.ty[name].toFixed(1).padStart(7)}  keep=${s.keep ? 1 : 0}`);
  }
  flush();
  return lines.join("\n");
}

// --- scenarios -----------------------------------------------------------------

async function scenarioKeep(ctx) {
  const { page, tab, kind } = await openBoard(ctx);
  log(`page kind: ${kind} (scroller: ${kind === "adapter" ? "#bcol" : "document"})`);
  const scrollerName = await page.evaluate(() => { const s = window.__bh.scroller(); return s === document.scrollingElement ? "document" : "#" + s.id; });
  assert(scrollerName === (kind === "adapter" ? "#bcol" : "document"), `the board scrolls in ${scrollerName} on a ${kind} page`);
  const lay = await scrollToMiddle(page);
  const above = lay.filter((t) => t.where === "above"), below = lay.filter((t) => t.where === "below"), inView = lay.filter((t) => t.where === "in");
  assert(above.length >= 3 && below.length >= 1 && inView.length >= 1, `the board must overflow both ways from the middle (above ${above.length}, in view ${inView.length}, below ${below.length}); seed more topics`);
  // The update above is the second topic (it rises past the head, off
  // screen); the update below is the bottom open topic, which rises past
  // the viewport to the head. The frame that grows late is the one the
  // first swap brought for the topic updated above: a swap's frames report
  // their heights late (load, reveal), which is what the compensation has
  // to hold through after the swap's own motion.
  const upAbove = above[1].name, lateGrow = upAbove, upBelow = below[below.length - 1].name;
  const tracked = inView.map((t) => t.name);
  log(`in view: ${tracked.join(", ")}; update above: ${upAbove}; update below: ${upBelow}; late frame: ${lateGrow}`);

  // 1. Two swaps, then a late frame height, sampled every animation frame.
  await page.evaluate((names) => window.__bh.startSampler(names), tracked);
  await sleep(50);
  assertChanged(await page.evaluate((n) => window.__bh.update([n]), upAbove), [upAbove]);
  await sleep(300);
  const b = await page.evaluate((n) => window.__bh.update([n]), upBelow);
  assertChanged(b, [upBelow]);
  assert(b.order[0] === upBelow && b.order[1] === upAbove, `the board order after both updates starts ${b.order.slice(0, 3).join(", ")}`);
  await sleep(1000);
  const keepAtLate = await page.evaluate((n) => { window.__bh.mark(`late frame height (${n} +300px)`); return !!window.__bh.keeping(); }, lateGrow);
  await growFrame(page, lateGrow, 300);
  await sleep(600);
  const samples = await page.evaluate(() => window.__bh.stopSampler());
  log(`sampled ${samples.filter((s) => s.y).length} frames over ${samples[samples.length - 1].t.toFixed(0)}ms; keep session when the late frame reported: ${keepAtLate ? "running" : "ended"}`);
  const failed = [];
  for (const seg of segments(samples, tracked)) {
    const parts = tracked.map((n) => seg.worst[n] ? `${n} ${seg.worst[n].d.toFixed(2)}px@${seg.worst[n].t.toFixed(0)}ms` : `${n} -`);
    const bad = tracked.filter((n) => seg.worst[n] && seg.worst[n].d > 1);
    log(`  [${seg.label}] from ${seg.from.toFixed(0)}ms: max |dy| ${parts.join(", ")}${bad.length ? "  <-- over 1px" : ""}`);
    if (bad.length) failed.push(`${seg.label}: ${bad.map((n) => `${n} moved ${seg.worst[n].d.toFixed(1)}px`).join(", ")}`);
  }
  if (failed.length || ctx.args.trace) console.error(`trace of ${tracked[0]} (screen y, transforms included):\n${trace(samples, tracked[0])}`);
  ctx.check(!failed.length, `topics in view moved more than 1px: ${failed.join("; ")}`);
  if (!failed.length) log("ok: topics in view held within 1px through both swaps and the late frame height");

  // 2. A user scroll during a session ends it, and the session writes no
  // more.
  await waitQuiet(page);
  const lay2 = await page.evaluate(() => window.__bh.layout());
  const above2 = lay2.filter((t) => t.where === "above");
  assert(above2.length >= 3, "too few topics above the viewport for the user-scroll part");
  const up2 = above2[1].name, grow2 = above2[above2.length - 1].name;
  assertChanged(await page.evaluate((n) => window.__bh.update([n]), up2), [up2]);
  const running = await page.evaluate(() => !!window.__bh.keeping());
  assert(running, "no keep session runs right after a board swap");
  const box = await page.evaluate(() => { const s = window.__bh.scroller(), t = window.__bh.viewTop(s), b = window.__bh.viewBottom(s); return { x: s === document.scrollingElement ? innerWidth / 2 : s.getBoundingClientRect().left + s.clientWidth / 2, y: (t + b) / 2 }; });
  await tab.mouse.move(box.x, box.y);
  await tab.mouse.wheel(0, 240);
  const ended = await page.waitForFunction(() => !window.__bh.keeping(), null, { timeout: 2000 }).then(() => true, () => false);
  ctx.check(ended, "the keep session still runs 2s after a user wheel scroll");
  // Let the wheel's own scroll finish before measuring.
  await page.waitForFunction(() => {
    const bh = window.__bh, st = bh.scroller().scrollTop;
    if (bh.lastSt === st) bh.stillN = (bh.stillN || 0) + 1; else { bh.lastSt = st; bh.stillN = 0; }
    return bh.stillN >= 10;
  }, null, { timeout: 3000, polling: "raf" });
  const probe = await page.evaluate(() => {
    const bh = window.__bh, s = bh.scroller(), t = bh.viewTop(s);
    const n = bh.topics().find((x) => x.getBoundingClientRect().bottom > t + 1);
    return { name: n.getAttribute("data-topic"), y: n.getBoundingClientRect().top, st: s.scrollTop, writes: bh.writesOn(s), anchorCss: getComputedStyle(s === document.scrollingElement ? document.documentElement : s).overflowAnchor };
  });
  await growFrame(page, grow2, 250);
  await sleep(400);
  const after = await page.evaluate((name) => { const bh = window.__bh; return { y: bh.section(name).getBoundingClientRect().top, st: bh.scroller().scrollTop, writes: bh.writesOn(bh.scroller()), keep: !!bh.keeping() }; }, probe.name);
  log(`after the user scroll: keep session ${after.keep ? "running" : "ended"}; scrollTop writes from script ${probe.writes} -> ${after.writes}; ${probe.name}.y ${probe.y.toFixed(1)} -> ${after.y.toFixed(1)} after ${grow2} grew 250px above (overflow-anchor ${probe.anchorCss})`);
  const okUser = [
    ctx.check(!after.keep, "a keep session started again without a swap"),
    ctx.check(after.writes === probe.writes, `the scroller took ${after.writes - probe.writes} scrollTop writes from script after the user scroll ended the session`),
    // With native anchoring off on the scroller the growth above must
    // show: nothing compensates it any more, so the scroll offset stays
    // (the topic's own y also follows other frames above that re-report
    // their heights meanwhile, so it is logged, not asserted). Where
    // native anchoring is back (the document between sessions) the
    // browser itself may compensate it, so only the absence of script
    // writes is checked.
    probe.anchorCss !== "none" || ctx.check(Math.abs(after.st - probe.st) <= 1 && after.y - probe.y > 100, `the scroll moved ${probe.st} -> ${after.st} (${probe.name}.y ${probe.y.toFixed(1)} -> ${after.y.toFixed(1)}) after a 250px growth above with no session running`),
  ].every(Boolean);
  if (ended && okUser) log("ok: a user scroll ended the session and the session no longer writes");
}

async function scenarioObserver(ctx) {
  const { page, kind } = await openBoard(ctx);
  log(`page kind: ${kind}`);
  const lay = await scrollToMiddle(page);
  const inView = lay.filter((t) => t.where === "in");
  assert(inView.length >= 1, "no topic in view");
  const target = inView[0].name;
  // Subscribe and wait for the first report (in view).
  await page.evaluate((name) => {
    const bh = window.__bh;
    bh.outs = 0;
    bh.obsHandle = window.wbHistory.watch(bh.section(name), { onOut: () => { bh.outs++; } });
  }, target);
  await page.waitForFunction((name) => { const r = window.wbHistory.seen(window.__bh.section(name)); return r && r.visible; }, target, { timeout: 2000 })
    .catch(() => { throw new Failure(`no in-view report for ${target} after watch()`); });
  // Scroll it fully out of view (past its bottom), then further.
  await page.evaluate((name) => {
    const bh = window.__bh, s = bh.scroller(), r = bh.section(name).getBoundingClientRect();
    s.scrollTop += r.bottom - bh.viewTop(s) + 40;
  }, target);
  await page.waitForFunction((name) => { const r = window.wbHistory.seen(window.__bh.section(name)); return r && r.ratio === 0; }, target, { timeout: 2000 })
    .catch(() => { throw new Failure(`${target} never reported ratio 0 after scrolling it out of view`); });
  await page.evaluate(() => { window.__bh.scroller().scrollTop += 300; });
  await sleep(300);
  const outs = await page.evaluate(() => { const bh = window.__bh; bh.obsHandle.unwatch(); return bh.outs; });
  log(`onOut fired ${outs} time(s) for ${target}`);
  assert(outs === 1, `onOut fired ${outs} times for one exit from view, expected 1`);
  log("ok: scrolling a topic fully out of view fired onOut once");

  // A topic a swap inserts is watched from that swap on: a new topic
  // (the server's next topic, made from a copy of one in view) lands at the
  // head of the board.
  await waitQuiet(page);
  const lay2 = await page.evaluate(() => window.__bh.layout());
  const from = (lay2.find((t) => t.where === "in") || lay2[0]).name, fresh = "BHNEW";
  const res = await page.evaluate(async ([from, name]) => {
    const bh = window.__bh;
    const r = await bh.update([], [{ name, from }]);
    const n = bh.section(name);
    return { r, present: !!n, watched: !!(n && n.wbWatch && n.wbWatch.node === n), head: bh.topics()[0] === n };
  }, [from, fresh]);
  assertChanged(res.r, [], [fresh]);
  assert(res.present && res.head, `the inserted ${fresh} section is not at the head of the board`);
  assert(res.watched, `the inserted ${fresh} section has no dashboard subscription (section.wbWatch)`);
  await page.waitForFunction((name) => window.wbHistory.seen(window.__bh.section(name)) !== null, fresh, { timeout: 2000 })
    .catch(() => { throw new Failure(`the inserted ${fresh} section has no visibility report after the swap`); });
  const first = await page.evaluate((name) => window.wbHistory.seen(window.__bh.section(name)).visible, fresh);
  await waitQuiet(page);
  // Bring it into view and expect a visible report.
  await page.evaluate(() => { window.__bh.scroller().scrollTop = 0; });
  await page.waitForFunction((name) => { const r = window.wbHistory.seen(window.__bh.section(name)); return r && r.visible; }, fresh, { timeout: 2000 })
    .catch(() => { throw new Failure(`the inserted ${fresh} section reported no visibility after being scrolled into view`); });
  log(`ok: the inserted ${fresh} section is watched (wbWatch; first report ${first ? "in view" : "out of view"}; in view once scrolled in)`);
}

// --- toolbar / overview QA ----------------------------------------------------

// Browser-local fixtures deliberately use the real cards framework and stream
// region path. They do not replace overview, keep, placement, or input handlers.
const OVERVIEW_PAGE = () => {
  const bh = window.__bh, root = document.documentElement;
  const items = () => [...new Set([...document.querySelectorAll('#r-board section.topic[data-topic], #r-board .bcard[data-card], #bpins section.topic[data-topic], #bpins .bcard[data-card]')])];
  const id = (n) => n && (n.dataset.card ? 'card:' + n.dataset.card : 'topic:' + n.dataset.topic);
  const rect = (n) => { const r = n.getBoundingClientRect(); return { top: r.top, left: r.left, width: r.width, height: r.height, bottom: r.bottom }; };
  bh.ovItems = items;
  bh.ovId = id;
  bh.ovFind = (key) => items().find((n) => id(n) === key);
  bh.ovState = () => {
    const s = bh.scroller(), t = document.querySelector('.board-toolbar'), v = bh.viewTop(s), b = bh.viewBottom(s), tr = rect(t);
    const anchor = tr.bottom > v && tr.top < b ? null : items().find((n) => bh.board_().contains(n) && rect(n).bottom > v && rect(n).top < b);
    return { st: s.scrollTop, top: v, bottom: b, toolbar: tr, key: id(anchor), y: anchor && rect(anchor).top,
      width: rect(bh.board_()).width, pinTop: document.querySelector('#bpins').scrollTop,
      pincol: root.hasAttribute('data-pincol'), here: id(document.querySelector('.overview-here')), hereY: document.querySelector('.overview-here') && rect(document.querySelector('.overview-here')).top,
      active: WsOverview.active(), holding: WsOverview.holding(), order: items().map(id),
      storage: JSON.stringify(Object.entries(localStorage).filter(([k]) => k.startsWith('ws.dashboard.cards.'))) };
  };
  bh.ovPosition = (where) => {
    const flow = [...bh.board_().children].filter((n) => n.matches('section.topic:not(.closed)'));
    if (where === 'top') bh.scroller().scrollTop = 0;
    else {
      const n = flow[where === 'bottom' ? flow.length - 2 : Math.floor(flow.length / 2)];
      if (!n) throw new Error('fixture needs at least two unpinned topics');
      bh.scroller().scrollTop += rect(n).top - bh.viewTop(bh.scroller()) + 36;
    }
  };
  bh.ovSeed = (pins = 2, cards = true) => {
    if (cards) {
      for (const kind of ['terminal', 'file', 'preview', 'web']) {
        const name = 'bh-' + kind;
        WsCards.register(name, { base: 180, open(c) { c.title({ name: 'QA ' + kind, icon: 'file' }); c.body.textContent = 'Browser-local ' + kind + ' body'; } });
        WsCards.open(name, {});
      }
    }
    const ts = bh.topics().slice(-pins);
    ts.forEach((n, i) => WsCards.move(n, { pin: true, index: i }));
    // The serving fixture has body-only topics. Add a real-shaped recap to
    // one, so the two-line contract is exercised rather than vacuous.
    const n = bh.topics().find((n) => !n.closest('#bpins'));
    if (n && !n.querySelector('.tsub')) {
      const sub = document.createElement('div'); sub.className = 'tsub';
      sub.innerHTML = '<div class="trecap"><div class="rtext md">' + 'Long current-state recap text with several wrapped lines. '.repeat(18) + '</div></div>';
      n.querySelector('.tophead').appendChild(sub);
    }
  };
  bh.ovFold = () => items().map((n) => {
    const head = n.querySelector('.tophead, .bc-hd'), text = n.querySelector('.trecap .rtext');
    const bodies = n.matches('.bcard') ? [...n.querySelectorAll('.bc-body, .bc-grip, .bc-folded')] : [...n.children].filter((x) => x !== head);
    return { id: id(n), h: rect(n).height, head: rect(head).height, cv: getComputedStyle(n).contentVisibility,
      hidden: bodies.every((x) => getComputedStyle(x).display === 'none'),
      recap: text && rect(text).height, line: text && parseFloat(getComputedStyle(text).lineHeight),
      clamp: text && getComputedStyle(text).webkitLineClamp };
  });
  bh.ovRegion = (html) => {
    const hash = 'bhqa' + Date.now().toString(16), ws = bh.sockets.find((s) => s.readyState === 1 && typeof s.onmessage === 'function');
    if (!ws) throw new Error('no live board socket');
    ws.onmessage({ data: JSON.stringify({ event: 'region', data: { name: 'board', hash, html } }) });
    bh.board = { hash, html };
    return hash;
  };
  bh.ovLands = [];
  document.addEventListener('ws-overview-land', (e) => {
    const { target, from, to } = e.detail;
    const r = (x) => ({ top: x.top, left: x.left, width: x.width, height: x.height });
    const land = { id: id(target), from: r(from), to: r(to), connected: e.target.isConnected };
    bh.ovLands.push(land);
    requestAnimationFrame(() => { land.next = rect(target); });
  });
  bh.ovMotion = (entering) => new Promise((resolve) => {
    const before = items().map((n) => ({ id: id(n), r: rect(n) })), samples = [], landsAt = bh.ovLands.length;
    let start;
    const take = (now = performance.now()) => ({ t: Math.max(0, now - start), active: WsOverview.active(), holding: WsOverview.holding(),
      st: bh.scroller().scrollTop, here: document.querySelector('.overview-here') && rect(document.querySelector('.overview-here')).top,
      rows: items().map((n) => ({ id: id(n), r: rect(n), motion: n.hasAttribute('data-overview-motion'), height: n.style.height,
        transition: getComputedStyle(n).transitionDuration })),
      slot: rect(document.querySelector('.overview-slot') || document.querySelector('#bpins')),
      ghosts: [...document.querySelectorAll('.overview-ghost')].map((n) => ({ r: rect(n), target: (() => { const key = JSON.parse(n.dataset.overviewTarget); return rect(bh.ovFind(key.card ? 'card:' + key.card : 'topic:' + key.topic)); })(), opacity: getComputedStyle(n).opacity,
        unsafe: !!n.querySelector('iframe, .bc-body, .xterm, .doc-body, input, textarea'), children: [...n.children].map((x) => x.className) })) });
    const t0 = performance.now(), nativeNow = performance.now;
    // Observe (never change) the implementation clock origin. Sampling with
    // the rAF timestamp below avoids treating callback/layout latency as a
    // different easing, even on a 120Hz browser.
    let clockStart = null;
    performance.now = function () {
      const now = nativeNow.call(performance);
      if (clockStart === null && /at motion\b/.test(new Error().stack || '')) clockStart = now;
      return now;
    };
    try { if (entering) WsOverview.toggle(); else WsOverview.exit(); }
    finally { performance.now = nativeNow; }
    start = clockStart === null ? performance.now() : clockStart;
    const initial = take(); initial.t = 0; samples.push(initial);
    // A click while the one clock runs must not invoke an underlying control.
    if (initial.rows.some((r) => r.motion)) {
      bh.ovDuringClick = 0;
      const b = document.querySelector('.board-search');
      const listener = () => { bh.ovDuringClick++; };
      b.addEventListener('click', listener, { once: true });
      b.click(); b.removeEventListener('click', listener);
    }
    let tail = 0;
    function frame(now) {
      const s = take(now); samples.push(s);
      if (!s.rows.some((r) => r.motion) && !s.ghosts.length && (entering ? s.active : !s.holding)) tail++; else tail = 0;
      if (tail >= 3) resolve({ before, samples, lands: bh.ovLands.slice(landsAt), setupMs: start - t0, clockObserved: clockStart !== null, clicks: bh.ovDuringClick || 0 });
      else if (s.t > 2000) resolve({ before, samples, lands: bh.ovLands.slice(landsAt), timeout: true });
      else requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  });
};

function qa(cond, label, detail) {
  assert(cond, label + (detail === undefined ? '' : ': ' + JSON.stringify(detail)));
  log('ok: ' + label);
}
async function overviewReady(ctx, options = {}) {
  const p = await openBoard(ctx);
  await p.page.evaluate(OVERVIEW_PAGE);
  if (options.seed !== false) { await p.page.evaluate(([pins, cards]) => window.__bh.ovSeed(pins, cards), [options.pins || 2, options.cards !== false]); await waitQuiet(p.page); }
  log(`page kind: ${p.kind}; viewport ${ctx.width}x${ctx.height}; PIN_WIDE=1600`);
  return p;
}
async function ovDone(page, on) {
  await page.waitForFunction((on) => WsOverview.active() === on && !document.querySelector('[data-overview-motion], .overview-ghost') && (on || !WsOverview.holding()), on, { timeout: 4000, polling: 'raf' });
}
async function ovEnter(page) { await page.evaluate(() => WsOverview.toggle()); await ovDone(page, true); }
async function ovExit(page) { await page.evaluate(() => WsOverview.exit()); await ovDone(page, false); }
async function ovAt(page, where) { await page.evaluate((w) => window.__bh.ovPosition(w), where); await waitQuiet(page); return page.evaluate(() => window.__bh.ovState()); }
async function capture(ctx, tab, name, data) {
  if (!ctx.args['capture-dir']) return;
  const dir = path.resolve(ctx.args['capture-dir']); fs.mkdirSync(dir, { recursive: true });
  const stem = `${ctx.args._[0]}-${ctx.width}-${name}`;
  if (data) fs.writeFileSync(path.join(dir, stem + '.json'), JSON.stringify(data, null, 2));
  await tab.screenshot({ path: path.join(dir, stem + '.png') });
}
async function checkRestore(page, before, label) {
  await ovDone(page, false);
  // Frames hidden by the fold can report their restored heights after the
  // instant exit. The keep contract covers those late reports too.
  await waitQuiet(page);
  const after = await page.evaluate(() => window.__bh.ovState());
  qa(Math.abs(before.st - after.st) <= 1, label + ' restores scroll within 1px', { before: before.st, after: after.st });
  qa(before.storage === after.storage, label + ' never persists overview in card storage');
}
async function checkFold(page) {
  const rows = await page.evaluate(() => window.__bh.ovFold());
  qa(rows.length > 0 && rows.every((r) => r.hidden && r.h <= r.head + 2.01 && r.cv === 'visible'), 'all topics/cards (including offscreen) fold to their headers', rows.filter((r) => !r.hidden || r.h > r.head + 2.01 || r.cv !== 'visible'));
  const recaps = rows.filter((r) => r.recap !== null);
  qa(recaps.length > 0 && recaps.every((r) => r.clamp === '2' && r.recap <= r.line * 2 + 1), 'existing recap is clamped to at most two lines', recaps);
}

async function scenarioToolbar(ctx) {
  const { page, tab, kind, browser } = await overviewReady(ctx);
  const first = await page.evaluate(() => {
    const b = __bh.board_(), t = b.querySelector('.board-toolbar');
    return { first: b.firstElementChild === t, next: t.nextElementSibling.id, rail: t.hasAttribute('data-rail'), card: t.hasAttribute('data-card'), pos: getComputedStyle(t).position,
      terminal: !!document.querySelector('.tnew'), capable: document.documentElement.hasAttribute('data-term'), files: !!document.querySelector('.board-files'), pane: !!document.querySelector('.fpane'),
      order: [...t.querySelectorAll('button.board-action')].map((n) => n.className), old: !!document.querySelector('#tslot,.tslot.tfix,.composer .tnew'),
      labels: [...t.querySelectorAll('.board-label')].map((n) => ({ overview: !!n.closest('.board-overview'), shown: getComputedStyle(n).display !== 'none' })),
      width: t.clientWidth, hint: getComputedStyle(t.querySelector('kbd')).display };
  });
  qa(first.first && (!first.capable || first.terminal) && first.terminal === first.capable && first.files === first.pane && !first.old && !first.rail && !first.card && first.pos !== 'sticky', 'toolbar is first, non-card/non-rail/non-sticky, with capability-gated actions', first);
  if (ctx.width < 1600) qa(first.next === 'bpins', 'normal-width pin list follows toolbar');
  const expected = ['board-overview', ...(first.terminal ? ['tnew'] : []), ...(first.order.some((s) => /web/.test(s)) ? ['web'] : []), 'board-search', ...(first.files ? ['board-files'] : [])];
  qa(first.order.length === expected.length && expected.every((s, i) => first.order[i].includes(s)), 'toolbar action order', first.order);
  qa(first.labels.every((x) => x.shown === (x.overview || first.width > 540)) && (first.width > 540 || first.hint === 'none'), 'label collapse keys on toolbar width, preserving Overview', first);
  await capture(ctx, tab, kind + '-normal');
  const top = await page.evaluate(() => __bh.ovState().toolbar.top);
  await ovAt(page, 'middle');
  qa(await page.evaluate((y) => __bh.ovState().toolbar.top < y - 100, top), 'toolbar scrolls away with board');
  await ovAt(page, 'top');
  if (first.terminal) {
    await page.click('.tnew');
    await page.waitForFunction(() => document.querySelectorAll('.topts .topt').length >= 1);
    const folders = await page.evaluate(async () => (await (await fetch(location.pathname + '/terminals')).json()).folders);
    const pop = await page.evaluate(() => { const n = document.querySelector('.tpop'), b = document.querySelector('.tnew'), r = n.getBoundingClientRect(); return { open: !n.hidden, below: r.top >= b.getBoundingClientRect().bottom, left: r.left, right: r.right, bottom: r.bottom, options: n.querySelectorAll('.topt').length, path: !!n.querySelector('input') }; });
    qa(pop.open && pop.below && pop.left >= 0 && pop.right <= ctx.width && pop.bottom <= ctx.height && pop.path && pop.options === (folders.root ? 2 : 1), 'terminal popover below toolbar fits viewport with worktree/path and root when available', { pop, folders });
    await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => document.querySelector('.tpop').hidden && document.activeElement.matches('.tnew')), 'popover Escape restores trigger focus');
    // No PTY is started: test the real UI callback against a terminal-route
    // refusal first, then a valid exited TerminalInfo response. Backend PTY
    // lifecycle is outside this browser harness (and needs process cleanup).
    let created = false, request = null;
    await tab.route('**/terminals', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      request = route.request().postDataJSON();
      if (request.folder === 'path') return route.fulfill({ status: 400, body: 'not_absolute' });
      created = true;
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'bh-exited-terminal', session: new URL(ctx.args.url).pathname.split('/').pop(), cwd: '/tmp', state: 'exited', rows: 14, cols: 80, exit_code: 0 }) });
    });
    await page.click('.tnew'); await page.fill('.tpop input', 'relative'); await page.click('.tprow button');
    await page.waitForFunction(() => document.querySelector('.tpop .ferr').textContent.length > 0);
    qa(await page.evaluate(() => !document.querySelector('.tpop').hidden && document.activeElement.matches('.tpop input')), 'folder refusal stays inline and focuses typed path');
    await page.click('.topts .topt:first-child');
    await page.waitForFunction(() => WsCards.list('terminal').some((c) => c.data.tid === 'bh-exited-terminal'));
    qa(created && request.folder === 'worktree' && await page.evaluate(() => __bh.board_().querySelector(':scope > .bcard, :scope > section.topic').dataset.kind === 'terminal'), 'worktree pick opens terminal at head of unpinned flow (browser-local POST fixture)', request);
  }
  log('SKIP web input popover: browser-pane entry is not on this branch' + (first.order.some((x) => /web/.test(x)) ? ' (unexpected entry; inspect manually)' : ''));
  // Land a board event with a closed section and localized remainder text.
  const base = await page.evaluate(() => __bh.baseBoard());
  await page.evaluate((html) => __bh.ovRegion(html + '<section class="topic closed" data-topic="BHCLOSED"><div class="tophead"><div class="tbar">Closed fixture</div></div></section><button class="closed-more" data-closed-next="1" data-closed-more="7">not a parseable count</button>'), base.html);
  const count = await page.evaluate(() => { const all = __bh.ovItems().filter((n) => n.matches('section.topic')); return { text: document.querySelector('.board-count').textContent, open: all.filter((n) => !n.classList.contains('closed')).length, closed: all.filter((n) => n.classList.contains('closed')).length + 7, first: __bh.board_().firstElementChild.matches('.board-toolbar') }; });
  qa(count.first && count.text === `open ${count.open} · closed ${count.closed}`, 'landed swap count includes pinned, shown closed and data-closed-more remainder (not localized text)', count);
  // Close only browser-local cards, never a real daemon terminal or process.
  await page.evaluate(() => {
    WsCards.list().forEach((c) => c.close());
    [...document.querySelector('#bpins').children].forEach((n) => WsCards.move(n, { pin: false }));
    __bh.ovRegion('<div class="empty">No board items</div>');
    __bh.scroller().scrollTop = 0;
  });
  await waitQuiet(page);
  const empty = await page.evaluate(() => { const e = document.querySelector('#r-board > .empty'), b = __bh.board_(), r = e.getBoundingClientRect(), s = document.querySelector('#tabbar') ? document.querySelector('#bcol') : document.scrollingElement; return { first: b.firstElementChild.matches('.board-toolbar'), selector: e.matches('.r-board:not(:has(> .topic, > .bcard)) > .empty'), display: getComputedStyle(e).display, height: r.height, bottom: r.bottom, max: __bh.viewBottom(s), overflow: s.scrollHeight - s.clientHeight }; });
  qa(empty.first && empty.selector && empty.display !== 'none' && empty.height >= 54 && (kind !== 'adapter' || (Math.abs(empty.bottom - empty.max) <= 16 && empty.overflow <= 1)), 'topicless placeholder fills remaining column without toolbar overflow', empty);
  await capture(ctx, tab, kind + '-empty');
  // Server-rendered no-data-term document, not deleting a mounted button.
  const noTerm = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height } });
  await noTerm.route('**/*', async (route) => {
    if (!route.request().isNavigationRequest()) return route.continue();
    const response = await route.fetch();
    const body = (await response.text()).replace(/\sdata-term="[^"]*"/g, '');
    await route.fulfill({ response, body });
  });
  await noTerm.goto(ctx.args.url); await noTerm.waitForTimeout(500);
  const frame = noTerm.frames().find((f) => /[?&]frame=1/.test(f.url())) || noTerm;
  await frame.waitForSelector('.board-toolbar', { state: 'attached' });
  qa(await frame.evaluate(() => !document.documentElement.hasAttribute('data-term') && !document.querySelector('.tnew,#tslot,.tfix')), 'no data-term means no terminal entry (server-document fixture)');
  await noTerm.close();
}

async function scenarioOverview(ctx) {
  const { page, tab, kind } = await overviewReady(ctx);
  // Capture shell bridge messages, including unexpected Esc forwarding.
  await tab.evaluate(() => { window.__ovMessages = []; addEventListener('message', (e) => { if (e.data && e.data.wsShell) __ovMessages.push(e.data); }); });
  await tab.emulateMedia({ reducedMotion: 'reduce' });
  const selected = ctx.args.case;
  assert(!selected || ['anchors', 'rail', 'picker', 'actions', 'palette', 'controls', 'reload'].includes(selected), 'unknown overview --case');
  if (selected) log('focused overview case: ' + selected + ' (other checkpoints not run)');
  if (!selected || selected === 'anchors' || selected === 'rail') {
  if (selected !== 'rail') {
  await page.evaluate(() => { __bh.hints = 0; const show = WsToast.show; WsToast.show = function (text, ...args) { if (text === wsT('js.board.overview.hint')) __bh.hints++; return show.call(this, text, ...args); }; });
  for (const where of ['middle', 'bottom']) {
    const before = await ovAt(page, where);
    qa(before.key && before.y < before.top, where + ' entry has a partly-visible negative-offset anchor', before);
    await ovEnter(page);
    const folded = await page.evaluate(() => __bh.ovState());
    qa(folded.here === before.key && Math.abs(folded.hereY - before.y) <= 1 && Math.abs(folded.width - before.width) <= 1, where + ' anchor screen y and board width hold within 1px', { before, folded });
    // Test the sibling performance rule while folded, independently of
    // scroll restoration: do not change the unfolded board's intrinsic
    // sizes between recording an anchor and entering overview.
    await page.evaluate(() => { __bh.ovItems().filter((n) => n.matches('section.topic')).forEach((n) => { n.dataset.bhCv = n.style.contentVisibility; n.style.contentVisibility = 'auto'; }); });
    await checkFold(page);
    await page.evaluate(() => { __bh.ovItems().filter((n) => n.matches('section.topic')).forEach((n) => { n.style.contentVisibility = n.dataset.bhCv; delete n.dataset.bhCv; }); });
    await capture(ctx, tab, kind + '-' + where + '-overview');
    await tab.keyboard.press('Escape'); await checkRestore(page, before, where + ' Escape');
  }
  qa(await page.evaluate(() => __bh.hints === 1), 'entry hint appears once in this tab, not on second entry');
  const top = await ovAt(page, 'top');
  await ovEnter(page);
  qa(await page.evaluate(() => !document.querySelector('.overview-here')), 'toolbar-visible entry has no here anchor');
  // Actual toolbar action; unlike locator.click on an offscreen toolbar,
  // this does not silently scroll the board and invalidate an anchor test.
  await page.click('.board-overview'); await checkRestore(page, top, 'Overview button');
  qa(Math.abs((await page.evaluate(() => __bh.ovState())).toolbar.top - top.toolbar.top) <= 1, 'no-anchor exit preserves toolbar screen y');
  }
  const outsides = selected === 'rail' ? ['shell-rail'] : kind === 'adapter' ? ['composer', 'shell-header'] : ['shell-header'];
  if (!selected) outsides.push('shell-rail');
  for (const outside of outsides) {
    const before = await ovAt(page, 'middle');
    if (outside === 'shell-rail') { await tab.evaluate(() => document.querySelector('#railbtn').click()); await sleep(300); }
    await ovEnter(page); await sleep(300);
    await page.evaluate(() => { __bh.outsideFired = 0; document.querySelector('#ctext')?.addEventListener('click', () => __bh.outsideFired++); });
    const urls = tab.frames().map((f) => f.url()), requests = [];
    const onRequest = (r) => { if (r.method() !== 'GET') requests.push(r.url()); };
    tab.on('request', onRequest);
    const focus = await page.evaluate(() => __bh.ovId(document.activeElement));
    if (outside === 'composer') {
      const r = await page.locator('#ctext').boundingBox(); assert(r, 'composer must be visible for physical outside click');
      await tab.mouse.click(r.x + 5, r.y + r.height / 2);
    } else if (outside === 'shell-rail') {
      const rail = tab.locator('#rail a[href]').first(), r = await rail.boundingBox();
      assert(r && r.x >= 0, 'scenario needs an open shell rail row');
      qa(await tab.evaluate(() => getComputedStyle(document.querySelector('#rail')).pointerEvents === 'none'), 'dim shell rail passes pointer events through to frame');
      await tab.mouse.click(r.x + r.width / 2, r.y + r.height / 2);
    } else {
      const r = await tab.locator('.dwrap > .top').boundingBox(); assert(r, 'scenario needs the iframe shell header');
      await tab.mouse.click(r.x + r.width - 5, r.y + r.height / 2);
    }
    await checkRestore(page, before, outside + ' outside click');
    tab.off('request', onRequest);
    qa(!requests.length && JSON.stringify(urls) === JSON.stringify(tab.frames().map((f) => f.url())) && await page.evaluate(() => !__bh.outsideFired && document.activeElement.id !== 'ctext'), outside + ' click is swallowed (no activation, navigation, POST or composer focus)', { requests, focus });
    if (outside === 'shell-rail') await tab.evaluate(() => { if (document.documentElement.classList.contains('rail-open')) document.querySelector('#railbtn').click(); });
  }
  }
  if (!selected || selected === 'picker') {
  // Roving focus and Enter use the stack's own DOM order (toolbar excluded).
  await ovAt(page, 'middle'); await ovEnter(page);
  qa(await page.evaluate(() => {
    const n = document.querySelector('.overview-here .tbar'), r = n.getBoundingClientRect(), before = __bh.ovState().order;
    n.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, isPrimary: true, button: 0, pointerId: 42, pointerType: 'mouse', clientX: r.left + 50, clientY: r.top + 10 }));
    document.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, isPrimary: true, pointerId: 42, clientX: r.left + 100, clientY: r.top + 80 }));
    const dragging = WsCards.dragging();
    document.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, isPrimary: true, pointerId: 42 }));
    return !dragging && JSON.stringify(before) === JSON.stringify(__bh.ovState().order);
  }), 'overview prevents drag start and preserves stack order');
  const roving = await page.evaluate(() => { const a = __bh.ovItems().filter((n) => __bh.board_().contains(n)), i = a.indexOf(document.activeElement); return { first: __bh.ovId(a[i]), next: __bh.ovId(a[i + 1]), third: __bh.ovId(a[i + 2]) }; });
  assert(roving.third, 'fixture needs two stack items after anchor');
  await tab.keyboard.press('ArrowDown'); qa(await page.evaluate((k) => __bh.ovId(document.activeElement) === k, roving.next), 'ArrowDown roves to next stack item');
  await tab.keyboard.press('ArrowUp'); qa(await page.evaluate((k) => __bh.ovId(document.activeElement) === k, roving.first), 'ArrowUp roves back to anchor');
  await tab.keyboard.press('ArrowDown'); await tab.keyboard.press('ArrowDown'); await tab.keyboard.press('Enter'); await ovDone(page, false);
  qa(await page.evaluate((k) => { const n = __bh.ovFind(k); return n.classList.contains('overview-flash') && Math.abs(n.getBoundingClientRect().top - __bh.viewTop(__bh.scroller())) <= 1 && getComputedStyle(n).animationName === 'none' && parseFloat(getComputedStyle(n).outlineWidth) >= 2; }, roving.third), 'Enter on third roving item lands top with static reduced-motion flash');
  await sleep(1250); qa(await page.evaluate((k) => !__bh.ovFind(k).classList.contains('overview-flash'), roving.third), 'static landing cue removed after 1.2s');
  // Clicking a faded head control must pick, not invoke its own action.
  await ovAt(page, 'middle'); await ovEnter(page);
  const pick = await page.evaluate(() => { const n = document.querySelector('.overview-here'); const key = __bh.ovId(n); n.querySelector('.tpin').click(); return key; });
  await ovDone(page, false);
  qa(await page.evaluate((k) => { const n = __bh.ovFind(k); return !n.hasAttribute('data-pinned') && n.classList.contains('overview-flash') && Math.abs(n.getBoundingClientRect().top - __bh.viewTop(__bh.scroller())) <= 1; }, pick), 'head-control click picks/lands/flashes topic rather than pinning it');
  // Card header-only folding and landing use a real Card with inert body.
  await ovAt(page, 'top'); await ovEnter(page);
  await page.evaluate(() => __bh.board_().querySelector('.bcard').click()); await ovDone(page, false);
  qa(await page.evaluate(() => { const n = document.querySelector('.bcard.overview-flash'); return n && Math.abs(n.getBoundingClientRect().top - __bh.viewTop(__bh.scroller())) <= 1; }), 'card click lands top and flashes like a topic');
  // Inject through socket onmessage, not a DOM append or direct morph call.
  await ovAt(page, 'middle'); await ovEnter(page);
  const heldBefore = await page.evaluate(() => ({ order: __bh.ovState().order, hash: __bh.board_().dataset.h, count: document.querySelector('.board-count').textContent }));
  const held = await page.evaluate(() => __bh.update([], [{ from: __bh.topics()[0].dataset.topic, name: 'BHHELD' }], { held: true }));
  qa(await page.evaluate((b) => __bh.board_().dataset.h === b.hash && JSON.stringify(__bh.ovState().order) === JSON.stringify(b.order) && !__bh.section('BHHELD'), heldBefore), 'injected board update changes nothing during overview');
  await ovExit(page);
  qa(await page.evaluate((h) => __bh.board_().dataset.h === h && !!__bh.section('BHHELD'), held.hash), 'held update lands after exit');
  await waitQuiet(page);
  qa(await page.evaluate((s) => document.querySelector('.board-count').textContent !== s, heldBefore.count), 'toolbar count updates on released swap');
  }
  if (!selected || selected === 'actions' || selected === 'palette') {
  // Palette stays over overview until a board pick. Both key variants must
  // route through WsOverview, not a raw scroll into folded content.
  for (const keep of [false, true]) {
    await ovAt(page, 'top'); await ovEnter(page);
    if (selected === 'palette') await tab.keyboard.press('Control+k'); else await page.click('.board-search');
    await page.waitForFunction(() => document.querySelector('#tkqpop').open, null, { timeout: 3000 });
    qa(await page.evaluate(() => WsOverview.active()), 'Search keeps overview open');
    // This palette clears a nonempty query on the first Escape; test the
    // top-layer close with an empty query, not against that separate rule.
    await page.fill('#tkqin', '');
    await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => !document.querySelector('#tkqpop').open && WsOverview.active()), 'Escape closes only palette');
    // Let the native dialog close event finish before another user opens it.
    await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
    if (selected === 'palette') await tab.keyboard.press('Control+k'); else await page.click('.board-search');
    await page.waitForFunction(() => document.querySelector('#tkqpop').open, null, { timeout: 3000 });
    await page.fill('#tkqin', 'T09');
    await page.waitForFunction(() => document.querySelector('#tkqpop [data-qs="board"][role="option"]'));
    await tab.keyboard.press('Alt+Digit1');
    await tab.keyboard.press(keep ? 'Control+Enter' : 'Enter'); await ovDone(page, false);
    // The frame reporter can deliver a restored height after exit returns.
    // Give the shared keep writer its next frame, within the flash window.
    await page.waitForFunction(() => Math.abs(__bh.section('T09').getBoundingClientRect().top - __bh.viewTop(__bh.scroller())) <= 1, null, { polling: 'raf', timeout: 1000 }).catch(() => {});
    const r = await page.evaluate(() => { const n = __bh.section('T09'); return { palette: document.querySelector('#tkqpop').open, flash: n.classList.contains('overview-flash'), dy: n.getBoundingClientRect().top - __bh.viewTop(__bh.scroller()) }; });
    qa(r.palette === keep && r.flash && Math.abs(r.dy) <= 1, (keep ? 'Ctrl+Enter' : 'Enter') + ' palette board pick exits/lands/flashes with correct palette lifetime', r);
    if (keep) { await page.fill('#tkqin', ''); await tab.keyboard.press('Escape'); }
  }
  }
  if (!selected || selected === 'actions' || selected === 'controls' || selected === 'palette') {
  await ovAt(page, 'top'); await ovEnter(page);
  await page.click('.tnew'); await tab.keyboard.press('Escape');
  qa(await page.evaluate(() => document.querySelector('.tpop').hidden && WsOverview.active()), 'Escape closes only terminal popover');
  await ovExit(page);
  await sleep(50);
  const messages = await tab.evaluate(() => __ovMessages);
  qa(messages.some((m) => m.wsShell === 'layout' && Math.abs(m.dim - .72) < .001) && messages.some((m) => m.wsShell === 'layout' && m.dim === 0) && !messages.some((m) => m.wsShell === 'key' && m.key === 'esc'), 'shell receives dim .72 / 0 and no forwarded Escape', messages);
  if (selected !== 'palette') {
    await ovAt(page, 'top');
    await page.evaluate(() => WsCards.list()[0].maxBtn.click());
    qa(await page.evaluate(() => { WsOverview.toggle(); return !WsOverview.holding() && !!document.querySelector('.bcard[data-max]'); }), 'overview entry is blocked while a card is maximized');
    await page.evaluate(() => WsCards.list()[0].minBtn.click());
    let postState = null;
    await tab.route('**/terminals', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      postState = await page.evaluate(() => ({ active: WsOverview.active(), held: WsOverview.holding() }));
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'bh-overview-terminal', cwd: '/tmp', state: 'exited' }) });
    });
    await ovAt(page, 'top'); await ovEnter(page); await page.click('.tnew'); await page.click('.topts .topt:first-child');
    await page.waitForFunction(() => WsCards.list('terminal').some((c) => c.data.tid === 'bh-overview-terminal'));
    qa(postState && !postState.active && !postState.held && await page.evaluate(() => __bh.board_().querySelector(':scope > .bcard, :scope > section.topic').dataset.kind === 'terminal'), 'terminal folder pick exits fully before POST/card creation (browser-local fixture)', postState);
    await ovAt(page, 'top'); await ovEnter(page);
    await tab.keyboard.press('Control+Backquote'); await ovDone(page, false); await sleep(50);
    qa(await tab.evaluate(() => __ovMessages.some((m) => m.wsShell === 'key' && m.key === 'rail')), 'rail hotkey exits overview before forwarding rail action');
    await tab.evaluate(() => { if (document.documentElement.classList.contains('rail-open')) document.querySelector('#railbtn').click(); });
  }
  if (kind === 'adapter') {
    await ovAt(page, 'top'); await ovEnter(page); await page.click('.board-files'); await ovDone(page, false);
    qa(await page.evaluate(() => document.querySelector('.fpane').getClientRects().length && document.activeElement.closest('.fpane') && document.activeElement.matches('input')), 'Files exits overview then reveals Files and focuses filter');
    if (await page.locator('#tabbar').isVisible()) await page.click('#tabbar [data-tab="board"]');
  }
  }
  if (!selected || selected === 'reload') {
  // Restore a genuine server anchor, not the injected BHHELD topic that a
  // reload intentionally discards. Storage is per rid and session-only.
  await page.evaluate(() => { const s = __bh.scroller(), n = __bh.section('T10'); s.scrollTop += n.getBoundingClientRect().top - __bh.viewTop(s) + 36; });
  await waitQuiet(page);
  const beforeReload = await page.evaluate(() => __bh.ovState()); await ovEnter(page);
  const saved = await page.evaluate(() => Object.entries(sessionStorage).filter(([k]) => k.startsWith('ws.dashboard.overview.')));
  qa(saved.some(([k, v]) => k.endsWith(locationRid(ctx.args.url)) && JSON.parse(v).on), 'overview persists in per-session sessionStorage', saved);
  await tab.reload({ waitUntil: 'load' });
  const reloadPage = tab.frames().find((f) => /[?&]frame=1/.test(f.url())) || tab;
  await reloadPage.evaluate(IN_PAGE); await reloadPage.evaluate(OVERVIEW_PAGE);
  await reloadPage.waitForFunction(() => WsOverview.active());
  qa(await reloadPage.evaluate(() => !document.querySelector('[data-overview-motion],.overview-ghost')), 'reload opens folded without motion');
  await reloadPage.evaluate(() => WsOverview.exit()); await ovDone(reloadPage, false);
  await waitQuiet(reloadPage);
  // The injected extra topic changes absolute scrollTop across reload; the
  // actual contract is the stored anchor offset, not stale pixel scroll.
  const reloaded = await reloadPage.evaluate((k) => { const n = __bh.ovFind(k); return n && n.getBoundingClientRect().top - __bh.viewTop(__bh.scroller()); }, beforeReload.key);
  qa(Math.abs(reloaded - (beforeReload.y - beforeReload.top)) <= 1, 'reload exit restores stored anchor offset within 1px', { reloaded, before: beforeReload.y - beforeReload.top });
  await capture(ctx, tab, kind + '-reload-exit');
  if (ctx.args['other-url']) {
    await ovAt(reloadPage, 'top'); await ovEnter(reloadPage);
    const other = new URL(ctx.args['other-url']); other.searchParams.set('frame', '1');
    assert(other.origin === new URL(ctx.args.url).origin && locationRid(other.href) !== locationRid(ctx.args.url), '--other-url must be another session on the same isolated daemon');
    await tab.evaluate((src) => { const f = document.createElement('iframe'); f.id = 'bh-other'; f.src = src; f.style.cssText = 'position:fixed;inset:0;width:100%;height:100%;z-index:-1'; document.body.appendChild(f); }, other.href);
    const second = await waitFor(() => tab.frames().find((f) => f.url() === other.href), 5000, 'second session document');
    await second.waitForLoadState('load');
    qa(await second.evaluate(() => !WsOverview.active()), 'second session does not inherit first session overview state');
    await second.evaluate(() => {
      document.querySelector('#tabbar [data-tab="board"]')?.click();
      window.__otherHints = 0; const show = WsToast.show;
      WsToast.show = function (text, ...args) { if (text === wsT('js.board.overview.hint')) __otherHints++; return show.call(this, text, ...args); };
      WsOverview.toggle();
    });
    await second.waitForFunction(() => WsOverview.active());
    qa(await second.evaluate(() => !window.__otherHints), 'entry hint does not repeat in second session frame in same tab');
    await ovExit(reloadPage);
    qa(await second.evaluate(() => WsOverview.active()), 'exiting first session leaves second session overview open');
    await second.evaluate(() => WsOverview.exit());
    await tab.evaluate(() => document.querySelector('#bh-other').remove());
  } else log('SKIP second session frame: use --other-url for same-tab isolation and shared hint checks');
  log('SKIP deferred reorder-hold release: only Phase 1 is present; no release scheduler exists');
  log('SKIP real terminal PTY lifecycle and private cards.js maximize() call during overview (not exported); UI max entry guard is in controls case');
  await ovAt(reloadPage, 'top'); await ovEnter(reloadPage); await sleep(300);
  const darkroom = await reloadPage.evaluate(() => ({ drawer: document.querySelector('#drawer') && getComputedStyle(document.querySelector('#drawer')).filter, composer: document.querySelector('.composer') && getComputedStyle(document.querySelector('.composer')).filter }));
  log('LOOK diagnostic (owner owns known #drawer omission): ' + JSON.stringify(darkroom));
  await ovExit(reloadPage);
  }
}
function locationRid(url) { return new URL(url).pathname.split('/').filter(Boolean).pop(); }

function overviewEase(p) {
  let lo = 0, hi = 1, t = p;
  for (let i = 0; i < 24; i++) { t = (lo + hi) / 2; const x = .6 * t * (1 - t) + t * t * t; if (x < p) lo = t; else hi = t; }
  return p <= 0 ? 0 : p >= 1 ? 1 : 2.4 * t * (1 - t) ** 2 + 3 * t * t * (1 - t) + t ** 3;
}
function validateMotion(run, entering, anchorY) {
  const samples = run.samples, initial = samples[0], last = samples.at(-1);
  assert(!run.timeout, 'overview motion never completed');
  qa(run.clockObserved, 'sampler observes the real motion clock without changing timestamps');
  const animated = initial.rows.filter((r) => r.motion);
  qa(animated.length > 0, 'motion starts near-viewport rows synchronously');
  qa(run.clicks === 0, 'clicks during motion are swallowed');
  const end = samples.find((s) => !s.rows.some((r) => r.motion));
  const gaps = samples.slice(1).map((s, i) => s.t - samples[i].t).sort((a, b) => a - b), frame = gaps[Math.floor(gaps.length / 2)];
  qa(end.t >= 240 - frame - 2 && end.t <= 240 + frame + 2, 'one 240ms clock completes within one animation frame', { end: end.t, frame, setup: run.setupMs });
  const initialIds = animated.map((r) => r.id);
  qa(samples.every((s) => s.rows.filter((r) => r.motion).every((r) => initialIds.includes(r.id))), 'no stagger / no row starts after initial frame');
  const reference = animated.find((r) => Math.abs(last.rows.find((x) => x.id === r.id).r.height - r.r.height) > 20);
  assert(reference, 'fixture needs a row that changes height');
  const refEnd = last.rows.find((r) => r.id === reference.id).r.height;
  let worstSync = 0, worstCurve = 0, worstAnchor = 0, worstRow = null;
  for (const s of samples) {
    const ref = s.rows.find((r) => r.id === reference.id), e = (ref.r.height - reference.r.height) / (refEnd - reference.r.height);
    const clock = entering ? overviewEase(Math.min(1, s.t / 240)) : 1 - overviewEase(1 - Math.min(1, s.t / 240));
    worstCurve = Math.max(worstCurve, Math.abs(e - clock) * Math.abs(refEnd - reference.r.height));
    for (const a of animated) {
      const b = last.rows.find((r) => r.id === a.id), r = s.rows.find((r) => r.id === a.id);
      const expected = a.r.height + (b.r.height - a.r.height) * e, delta = Math.abs(r.r.height - expected);
      if (delta > worstSync) { worstSync = delta; worstRow = { id: r.id, t: s.t, expected, actual: r.r.height, inline: r.height }; }
      assert(r.transition.split(',').every((x) => parseFloat(x) === 0), 'row height must be driven by rAF, not CSS transitions');
    }
    if (anchorY !== null && s.here !== null) worstAnchor = Math.max(worstAnchor, Math.abs(s.here - anchorY));
    for (const [i, g] of s.ghosts.entries()) {
      assert(!g.unsafe && g.children.length === 2 && /tophead|bc-hd/.test(g.children[0]) && g.children[1] === 'overview-ghost-body', 'ghost must be header-only plus a plain collapsing body box');
      const from = initial.ghosts[i].r;
      assert(['top', 'left', 'width', 'height'].every((k) => Math.abs(g.r[k] - (from[k] + (g.target[k] - from[k]) * e)) <= 1), 'every ghost tracks its live target on the same curve within 1px');
    }
    if (initial.ghosts.length) assert(Math.abs(s.slot.height - (initial.slot.height + (last.slot.height - initial.slot.height) * e)) <= 1, 'pin slot and folds share the same clock within 1px');
  }
  qa(worstSync <= 1, 'all animated heights share one progress curve within 1px', { entering, worstSync, worstRow });
  qa(worstCurve <= 1, 'clock follows cubic-bezier(.2,.8,.2,1), reversed on exit, within 1px', { worstCurve });
  if (entering && anchorY !== null) qa(worstAnchor <= 1, 'anchor holds within 1px on every animation frame', { worstAnchor });
  const outside = initial.rows.filter((r) => !r.motion);
  if (entering) assert(outside.length > 0, 'entry fixture must include far-offscreen rows');
  qa(outside.every((r) => samples.every((s) => Math.abs(s.rows.find((x) => x.id === r.id).r.height - last.rows.find((x) => x.id === r.id).r.height) <= 1)), 'offscreen rows switch instantly, with no animation');
  for (const land of run.lands) {
    const diff = (a, b) => Math.max(...['top', 'left', 'width', 'height'].map((k) => Math.abs(a[k] - b[k])));
    qa(land.connected && diff(land.from, land.to) <= 1 && land.next && diff(land.to, land.next) <= 1, 'pin clone lands exactly on live target and does not jump next frame', land);
  }
  return { animated: initialIds, final: last.rows.map((r) => ({ id: r.id, h: r.r.height })) };
}
async function scenarioOverviewMotion(ctx) {
  const { page, tab, kind } = await overviewReady(ctx, { pins: ctx.width >= 1600 ? 5 : 2, cards: false });
  await tab.emulateMedia({ reducedMotion: 'no-preference' });
  const selected = ctx.args.case;
  assert(!selected || ['enter-top', 'enter-middle', 'enter-bottom', 'pins'].includes(selected), 'unknown overview-motion --case');
  if (selected) log('focused motion case: ' + selected + ' (other checkpoints not run)');
  const positions = selected === 'pins' ? [] : selected ? [selected.slice(6)] : ['middle', 'bottom'];
  for (const where of positions) {
    const before = await ovAt(page, where);
    const enter = await page.evaluate(() => __bh.ovMotion(true));
    await capture(ctx, tab, kind + '-' + where + '-enter', enter);
    let result;
    try { result = validateMotion(enter, true, before.y); }
    catch (e) { console.error('per-frame enter trace:\n' + JSON.stringify(enter, null, 2)); throw e; }
    qa(await page.evaluate((w) => Math.abs(__bh.ovState().width - w) <= 1, before.width), 'board width unchanged while pin list is in overview');
    if (selected && selected.startsWith('enter-')) { log('SKIP exit/end-state equivalence in focused enter-only case'); return; }
    const exit = await page.evaluate(() => __bh.ovMotion(false));
    await capture(ctx, tab, kind + '-' + where + '-exit', exit);
    try { validateMotion(exit, false, null); }
    catch (e) { console.error('per-frame exit trace:\n' + JSON.stringify(exit, null, 2)); throw e; }
    await checkRestore(page, before, where + ' animated exit');
    // At common eased progress, a forward and reversed curve must traverse
    // the same heights within 1px. Endpoints must be exact too.
    const enterStart = enter.samples[0], exitEnd = exit.samples.at(-1);
    qa(result.animated.every((id) => Math.abs(enterStart.rows.find((r) => r.id === id).r.height - exitEnd.rows.find((r) => r.id === id).r.height) <= 1), 'reverse motion returns exact unfolded heights');
    await tab.emulateMedia({ reducedMotion: 'reduce' });
    const instant = await page.evaluate(() => { WsOverview.toggle(); return { active: WsOverview.active(), motion: !!document.querySelector('[data-overview-motion],.overview-ghost'), heights: __bh.ovItems().map((n) => ({ id: __bh.ovId(n), h: n.getBoundingClientRect().height })) }; });
    qa(instant.active && !instant.motion && instant.heights.every((r) => Math.abs(r.h - result.final.find((x) => x.id === r.id).h) <= 1), 'reduced motion switches instantly to identical folded end state');
    await page.evaluate(() => WsOverview.exit());
    qa(await page.evaluate(() => !WsOverview.holding() && !document.querySelector('[data-overview-motion],.overview-ghost')), 'reduced-motion exit completes synchronously without transition');
    await tab.emulateMedia({ reducedMotion: 'no-preference' });
  }
  if (ctx.width >= 1600) {
    if (selected === 'pins') await tab.emulateMedia({ reducedMotion: 'reduce' });
    const wide = await ovAt(page, 'top');
    qa(wide.pincol, '1900px fixture enables actual pinned column');
    await page.evaluate(() => { const p = document.querySelector('#bpins'); p.scrollTop = 160; });
    const scroll = await page.evaluate(() => document.querySelector('#bpins').scrollTop);
    qa(scroll > 0, 'wide pinned list overflows and is independently scrolled', scroll);
    await ovEnter(page);
    qa(await page.evaluate(() => document.querySelector('#bpins').parentElement.id === 'r-board' && document.querySelector('.board-toolbar').nextElementSibling.id === 'bpins' && !document.querySelector('#bpcol').children.length && getComputedStyle(document.querySelector('#bpins'), '::before').display === 'none'), 'wide overview empties column and places pins directly under toolbar without label');
    await ovExit(page); await waitQuiet(page);
    qa(await page.evaluate((s) => document.querySelector('#bpins').parentElement.id === 'bpcol' && Math.abs(document.querySelector('#bpins').scrollTop - s) <= 1, scroll), 'pinned-column scroll restores through overview');
    await ovEnter(page);
    await tab.setViewportSize({ width: 1400, height: ctx.height }); await sleep(150);
    qa(await page.evaluate(() => WsOverview.active() && document.querySelector('#bpins').parentElement.id === 'r-board' && !document.documentElement.hasAttribute('data-pincol')), 'crossing below PIN_WIDE keeps overview pins in board');
    await ovExit(page);
    qa(await page.evaluate(() => document.querySelector('#bpins').parentElement.id === 'r-board'), 'exit uses new narrow pin placement');
    await ovEnter(page); await tab.setViewportSize({ width: 1900, height: ctx.height }); await sleep(150);
    qa(await page.evaluate(() => WsOverview.active() && document.querySelector('#bpins').parentElement.id === 'r-board' && document.documentElement.hasAttribute('data-pincol')), 'crossing above PIN_WIDE still holds pin list in board');
    await ovExit(page);
    qa(await page.evaluate(() => document.querySelector('#bpins').parentElement.id === 'bpcol'), 'exit uses new wide pin placement');
    await ovEnter(page);
    const target = await page.evaluate(() => { const n = document.querySelector('#bpins').lastElementChild; n.click(); return __bh.ovId(n); });
    await ovDone(page, false);
    qa(await page.evaluate((k) => { const n = __bh.ovFind(k), p = document.querySelector('#bpins'), r = n.getBoundingClientRect(), v = p.getBoundingClientRect(); return p.parentElement.id === 'bpcol' && n.classList.contains('overview-flash') && r.top >= v.top - 1 && r.top < v.bottom; }, target), 'pinned pick returns to column, scrolls to target and flashes');
  } else log('SKIP pin flights/column scrolling at this width: PIN_WIDE=1600 (run --width 1900)');
}

const SCENARIOS = {
  toolbar: scenarioToolbar,
  overview: scenarioOverview,
  'overview-motion': scenarioOverviewMotion,
  keep: scenarioKeep,
  observer: scenarioObserver,
};

// --- main ----------------------------------------------------------------------

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const name = args._[0];
  if (name === "serve") return serve(args);
  if (!name || !SCENARIOS[name] || !args.url || args.url === true) {
    console.error("usage: node scripts/board-headless.mjs serve [--topics N] [--keep-tmp]");
    console.error(`       node scripts/board-headless.mjs <${Object.keys(SCENARIOS).join("|")}> --url <session page> [--width W] [--height H] [--headed]`);
    process.exit(2);
  }
  // check records a failed expectation and lets the scenario go on, so one
  // run reports every part; assert stops the scenario.
  const ctx = { args, width: +(args.width || 1440), height: +(args.height || 900), errors: [], failures: [] };
  ctx.check = (cond, msg) => { if (!cond) { ctx.failures.push(msg); console.error(`FAILED: ${msg}`); } return !!cond; };
  try {
    const t0 = Date.now();
    await SCENARIOS[name](ctx);
    if (ctx.errors.length) ctx.failures.push(`page errors: ${ctx.errors.join(" | ")}`);
    if (ctx.failures.length) throw new Failure(`${ctx.failures.length} check(s) failed:\n  - ${ctx.failures.join("\n  - ")}`);
    log(`PASS ${name} (${((Date.now() - t0) / 1000).toFixed(1)}s)`);
  } catch (e) {
    console.error(`FAIL ${name}: ${e instanceof Failure ? e.message : e.stack || e}`);
    process.exitCode = 1;
  } finally {
    for (const b of BROWSERS) await b.close().catch(() => {});
  }
}

main();

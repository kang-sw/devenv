#!/usr/bin/env node
// Repository-local developer command: headless browser scenarios for the
// dashboard board's motion and scroll behaviour. Not shipped with any plugin
// package.
//
//     node scripts/board-headless.mjs serve [--topics N] [--history-entries N] [--keep-tmp]
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
//   - Any existing daemon serving this tree's assets also works. Never restart
//     or replace a shared daemon merely to run this harness; prefer `serve`.
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
//   history   640px history clips, bounded paging, late-height anchoring,
//             deep links and out-of-view collapse on dashboard AND standalone
//             full file pages. Dashboard-only: expanded swaps, overlapping keep
//             sessions and the pinned column (use --width 1800).
//   history-cards  Card/paging probes without navigation/control/live checks.
//   history-paint  Targeted deep-link post-paint/keep-lifetime reproduction.
//
// History samples retain rAF geometry as diagnostic evidence and assert <=1px
// on rAF -> zero-delay-task (post-paint) geometry, after ResizeObserver runs.
// Navigation height reports arrive <=300ms apart while the existing keep is
// OPEN. A separate post-end report may move content but must not re-arm it.
//
// `serve --history-entries 180` seeds long panel histories and prints the true
// standalone file URLs too. `history --artifacts <dir>` saves measurements,
// per-frame samples and before/after PNGs. Standalone full pages have unframed
// entry bodies, so their late-height probe grows that real body, not an invented
// dashboard iframe. Swaps/keep/observer still require a dashboard stream.
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
function seedSession(tmp, bin, label, n, historyEntries) {
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
  if (historyEntries) {
    // Real renderer output, including its 50-entry templates and disclosure
    // keys. Distinct panels avoid turning the long history into one log fold.
    const entries = Array.from({ length: historyEntries }, (_, i) =>
      `<wb:panel recap="History panel ${i + 1}"><p>History payload ${i + 1}: delayed content height fixture.</p></wb:panel>`).join("");
    run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key,
      commands: `<wb:topic name="HISTORY">${entries}</wb:topic>` })], { cwd: proj, env });
    // Keep topics on both sides, so collapse compensation cannot be masked
    // by the document's scroll-range clamp.
    run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key, commands: topicCommands(n + 1) })], { cwd: proj, env });
  }
  return { rid, root: fer.root, key: fer.session_key,
    standalone: pathToFileURL(path.join(fer.root, "ai-docs", "whiteboards", rid, "index.html")).href };
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
    run("go", ["build", "-o", bin, "./cmd/ws-mcp"], { cwd: TOOL_DIR, env: childEnv(tmp) });
    log(`seeding two sessions (${n} topics each) ...`);
    const adapter = seedSession(tmp, bin, "adapter", n, +(args["history-entries"] || 0));
    const board = seedSession(tmp, bin, "board", n, +(args["history-entries"] || 0));
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
    log(`standalone:   ${board.standalone}`);
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
  bh.board_ = () => document.getElementById("r-board") || document.querySelector("body.full .grid > main");
  bh.kind = () => (document.body.classList.contains("full") ? "standalone" : document.getElementById("tabbar") ? "adapter" : "board");
  bh.scroller = () => {
    const c = bh.focus && bh.focus.closest("#bpins") || document.getElementById("bcol");
    const oy = c && getComputedStyle(c).overflowY;
    return c && (oy === "auto" || oy === "scroll") && c.scrollHeight > c.clientHeight ? c : document.scrollingElement;
  };
  bh.viewTop = (s) => (s === document.scrollingElement ? 0 : Math.max(0, s.getBoundingClientRect().top + s.clientTop));
  bh.viewBottom = (s) => (s === document.scrollingElement ? innerHeight : Math.min(innerHeight, s.getBoundingClientRect().top + s.clientTop + s.clientHeight));
  bh.topics = () => q("section.topic[data-topic]", bh.board_()).filter((n) => !n.classList.contains("closed"));
  bh.section = (name) => q("section.topic[data-topic]").find((n) => n.getAttribute("data-topic") === name) || null;
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
  bh.update = async (names, add) => {
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
    const state = bh.sampling = { samples, on: true, t0 };
    const take = (phase) => {
      const y = {}, ty = {};
      for (const name of names) {
        const n = bh.section(name) || document.getElementById(name);
        y[name] = n ? n.getBoundingClientRect().top : null;
        const m = n && /^matrix(3d)?\((.*)\)$/.exec(getComputedStyle(n).transform || "");
        ty[name] = m ? +m[2].split(",")[m[1] ? 13 : 5] : 0;
      }
      const h = bh.history && bh.history();
      samples.push({ t: performance.now() - t0, phase, st: s.scrollTop, keep: !!bh.keeping(), y, ty,
        history: h ? { steps: h._wbHistorySteps, target: h._wbHistoryTarget?.id || null } : undefined });
    };
    take('baseline');
    const postPaint = !!bh.samplePostPaint;
    const tick = () => {
      if (!state.on) return;
      take('raf');
      // rAF precedes ResizeObserver delivery. A task queued here runs after
      // that rendering update, including RO's reveal/keep corrections. Keep
      // both phases as evidence, but history asserts the post-paint geometry.
      if (postPaint) setTimeout(() => { if (state.on) take('post-paint'); }, 0);
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  };
  bh.mark = (label) => { if (bh.sampling && bh.sampling.on) bh.sampling.samples.push({ t: performance.now() - bh.sampling.t0, mark: label }); };
  bh.stopSampler = () => { bh.sampling.on = false; return bh.sampling.samples; };
};

// Browsers openBoard launched; main closes them on every exit path.
const BROWSERS = [];

// openBoard opens url in a fresh headless Chromium, waits for the board, its
// stream socket (dashboard only) and its body frames' first heights, shows the board column
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
  await page.waitForFunction(() => window.__bh.board_()?.querySelector("section.topic[data-topic]"), null, { timeout: 10000 });
  const kind = await page.evaluate(() => window.__bh.kind());
  if (kind !== "standalone") await page.waitForFunction(() => window.__bh.sockets.some((s) => s.readyState === 1 && typeof s.onmessage === "function"), null, { timeout: 10000 });
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
async function growFrame(page, name, px, selector = `#r-board section.topic[data-topic="${name}"] iframe.doc-frame`) {
  // A swapped-in frame loads beside the old one (.doc-frame-next) and
  // takes its place once drawn: grow the one shown.
  await page.waitForFunction((n) => !document.querySelector(`#r-board section.topic[data-topic="${n}"] iframe.doc-frame-next`), name, { timeout: 5000 })
    .catch(() => { throw new Failure(`topic ${name}'s next body frame never took its place`); });
  const handle = await page.$(selector);
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
    bh.obsChanges = 0;
    bh.obsHandle = window.wbHistory.watch(bh.section(name), { onChange: () => { bh.obsChanges++; }, onOut: () => { bh.outs++; } });
  }, target);
  // seen() can still contain the dashboard's earlier report. Wait for OUR
  // subscription's first delivery before scrolling, or onOut is unarmed.
  await page.waitForFunction((name) => { const bh = window.__bh, r = window.wbHistory.seen(bh.section(name)); return bh.obsChanges > 0 && r && r.visible; }, target, { timeout: 2000 })
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

// History probes use the real host's wbHistory, event delegation, DOM and
// styles. Only payload height arrival and stream updates are synthetic.
const HISTORY_PAGE = () => {
  const bh = window.__bh;
  bh.samplePostPaint = true;
  bh.history = () => document.querySelector('.wb-history[data-history="HISTORY"]');
  bh.historyState = () => {
    const h = bh.history(), live = h.querySelector('.history-live'), clip = h.querySelector('.history-clip');
    const r = clip.getBoundingClientRect();
    return { steps: h._wbHistorySteps, pages: h.querySelectorAll('template[data-history-page]').length,
      rows: live.querySelectorAll('.wb-entry').length, live: live.scrollHeight, height: r.height,
      top: r.top, bottom: r.bottom, scrollTop: clip.scrollTop,
      maxHeight: getComputedStyle(clip).maxHeight, overflow: getComputedStyle(h).overflowY,
      button: !h.querySelector('.history-older').hidden,
      latest: Array.from(live.children).filter((n) => n.matches('.wb-entry')).at(-1)?.id,
      folds: Array.from(live.querySelectorAll('details[data-key]')).map((d) => [d.dataset.key, d.open]) };
  };
  bh.positionHistory = () => {
    const s = bh.scroller(), r = bh.history().querySelector('.history-clip').getBoundingClientRect();
    s.scrollTop += r.top - bh.viewTop(s) - 100;
  };
};

async function growEntry(page, id, px) {
  await page.evaluate(([id, px]) => window.__bh.mark(`late payload height (${id} +${px}px)`), [id, px]);
  const selector = `#${id} .entry-body iframe.doc-frame`;
  const framed = await page.locator(selector).count();
  if (framed) return growFrame(page, id, px, selector);
  // The true standalone renderer deliberately has no doc-frame iframes.
  // Grow its actual inline payload, giving ResizeObserver the same late
  // layout change; do not substitute dashboard markup for the full file.
  const delta = await page.evaluate(([id, px]) => {
    const body = document.querySelector(`#${id} .entry-body`);
    if (!body) throw new Error(`no entry body for ${id}`);
    const before = body.getBoundingClientRect().height;
    // Padding simulates payload growth without changing paragraph margin
    // collapse (an appended sibling would add a second, unrelated delta).
    body.style.paddingBottom = (parseFloat(getComputedStyle(body).paddingBottom) + px) + 'px';
    return body.getBoundingClientRect().height - before;
  }, [id, px]);
  assert(Math.abs(delta - px) <= 1, `${id}'s inline payload grew ${delta}px, expected ${px}px`);
}

async function scenarioHistory(ctx) {
  const opened = await openBoard(ctx), { page, tab, kind } = opened;
  const dir = path.resolve(ctx.args.artifacts || path.join(os.tmpdir(), `board-history-${Date.now()}`));
  fs.mkdirSync(dir, { recursive: true });
  const report = { url: ctx.args.url, kind, viewport: [ctx.width, ctx.height], measurements: [], omissions: [
    'This long-panel fixture does not exercise an underfilled multi-resize page chain (Node policy-test scope).',
    'Standalone has no stream, swap, pinned column or doc-frame iframes; its real inline payload layout is used for late heights.'
  ] };
  if (kind !== 'standalone') report.omissions.pop();
  const measure = (label, data) => { report.measurements.push({ label, ...data }); log(`${label}: ${JSON.stringify(data)}`); };
  const stable = (label, samples, ids) => {
    fs.writeFileSync(path.join(dir, `${label}.json`), JSON.stringify(samples, null, 2));
    const painted = samples.filter((s) => s.mark || s.phase !== 'raf');
    const worstOf = (list) => Math.max(0, ...segments(list, ids).flatMap((s) => ids.map((id) => s.worst[id]?.d || 0)));
    const worst = worstOf(painted), rafWorst = worstOf(samples.filter((s) => s.mark || s.phase !== 'post-paint'));
    assert(painted.some((s) => s.phase === 'post-paint'), `${label}: no post-paint samples`);
    assert(painted.filter((s) => s.y).every((s) => ids.every((id) => s.y[id] != null)), `${label}: a tracked anchor disappeared`);
    measure(label, { maxDy: worst, rafMaxDy: rafWorst, frames: painted.filter((s) => s.y).length });
    assert(worst <= 1, `${label}: post-paint screen y moved ${worst.toFixed(2)}px (>1px); evidence ${dir}/${label}.json`);
  };
  const capture = async (name) => { const p = path.join(dir, `${kind}-${name}.png`); await tab.screenshot({ path: p }); log(`capture: ${p}`); };
  async function cards() {
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    const first = await page.evaluate(() => window.__bh.historyState());
    measure('initial', first);
    assert(first.steps === 1 && Math.abs(first.height - 640) <= 1, `initial clip is ${first.height}px at step ${first.steps}, not 640px`);
    assert(first.pages >= 2 && first.rows === 50, 'need 50 live entries and at least two inert older pages');
    assert(first.overflow !== 'auto' && first.overflow !== 'scroll' && first.scrollTop === 0, 'history has an inner scroll');
    await capture('before-load-more');

    // Watch the oldest intersecting row and the newest row through a real
    // button click. No Playwright auto-scroll is allowed to move the anchor.
    const ids = await page.evaluate(() => {
      const h = window.__bh.history(), clip = h.querySelector('.history-clip').getBoundingClientRect();
      const entries = Array.from(h.querySelectorAll('.history-live > .wb-entry'));
      return [entries.find((e) => e.getBoundingClientRect().bottom > clip.top).id, entries.at(-1).id];
    });
    await page.evaluate((ids) => window.__bh.startSampler(ids), ids);
    const click = await page.evaluate(() => {
      const bh = window.__bh, before = bh.historyState();
      bh.history().querySelector('.history-older').click();
      return { before, after: bh.historyState() };
    });
    await sleep(180);
    await capture('after-load-more');
    stable('expand-anchor', await page.evaluate(() => window.__bh.stopSampler()), ids);
    assert(click.after.steps === 2 && Math.abs(click.after.height - 1280) <= 1, 'load-more did not raise the clip by exactly 640px');
    assert(click.before.pages - click.after.pages <= 1, 'one click synchronously loaded more than one page');

    // Fill successive 640px steps until an embedded page is needed. Record
    // the synchronous boundary separately from ResizeObserver's later work.
    const paging = [];
    for (let i = 0; i < 20; i++) {
      const r = await page.evaluate(() => {
        const bh = window.__bh, before = bh.historyState();
        bh.history().querySelector('.history-older').click();
        return { before, after: bh.historyState() };
      });
      paging.push(r);
      assert(r.before.pages - r.after.pages <= 1, 'one click synchronously loaded multiple pages');
      if (r.before.live >= r.after.steps * 640) assert(r.before.pages === r.after.pages, 'loaded a page even though the clip was filled');
      assert(r.after.steps === r.before.steps + 1, 'clip did not step by one');
      await sleep(100);
      r.settled = await page.evaluate(() => window.__bh.historyState());
      if (r.after.live >= r.after.steps * 640) assert(r.settled.pages === r.after.pages, 'later resize over-loaded a clip that was already filled');
      if (r.after.pages < r.before.pages) break;
    }
    measure('paging', { clicks: paging.map((r) => ({ steps: r.after.steps, pagesBefore: r.before.pages, pagesSync: r.after.pages, pagesAfterResize: r.settled.pages, liveBefore: r.before.live })) });
    assert(paging.some((r) => r.after.pages < r.before.pages), 'no embedded page was materialized');

    // Open an older payload still above the bottom-anchored window. Opening
    // and then growing it are distinct real layout changes. Check after the
    // keep quiet window as well as within it.
    const older = await page.evaluate(() => {
      const entries = Array.from(window.__bh.history().querySelectorAll('.history-live > .wb-entry'));
      entries[0].querySelector('details').open = true;
      return entries[0].id;
    });
    await sleep(150);
    await page.evaluate((id) => window.__bh.startSampler([id]), first.latest);
    await growEntry(page, older, 240);
    await sleep(750);
    await growEntry(page, older, 180);
    await sleep(200);
    stable('latest-late-heights', await page.evaluate(() => window.__bh.stopSampler()), [first.latest]);

    if (kind !== 'standalone') {
      // Expansion and the existing swap compensation must use the SAME
      // handle, not merely two sessions that happen to produce equal y.
      await waitQuiet(page);
      const res = await page.evaluate(async () => {
        const bh = window.__bh;
        const above = bh.layout().filter((t) => t.where === 'above' && t.name !== 'HISTORY');
        const name = above[0]?.name || bh.topics().find((n) => n.dataset.topic !== 'HISTORY').dataset.topic;
        const before = bh.historyState(), r = await bh.update(['HISTORY', name]);
        const swap = bh.keeping();
        bh.history().querySelector('.history-older').click();
        return { name, r, same: !!swap && swap === bh.keeping(), before, after: bh.historyState() };
      });
      assertChanged(res.r, ['HISTORY', res.name]);
      measure('expanded-swap-overlap', { sameSession: res.same, stepsBefore: res.before.steps, stepsAfter: res.after.steps, rowsBefore: res.before.rows, rowsAfter: res.after.rows });
      assert(res.same, 'history expansion did not merge into the existing swap keep session');
      assert(res.after.steps === res.before.steps + 1 && res.after.rows >= res.before.rows, 'swap lost expanded history');
      await waitQuiet(page);
    }

    // A fully out card collapses once it has been in view. Its materialized
    // entries and disclosure choices must survive; the next visible topic
    // must not jump when the card above it shrinks.
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    const collapseProbe = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), topic = h.closest('.topic'), s = bh.scroller();
      const next = bh.topics()[bh.topics().indexOf(topic) + 1];
      if (!next) throw new Error('need a topic below HISTORY');
      const before = bh.historyState();
      s.scrollTop += topic.getBoundingClientRect().bottom - bh.viewTop(s) + 20;
      bh.startSampler([next.dataset.topic]);
      return { name: next.dataset.topic, before };
    });
    await page.waitForFunction(() => window.__bh.history()._wbHistorySteps === 1, null, { timeout: 3000 });
    await sleep(160);
    stable('collapse-anchor', await page.evaluate(() => window.__bh.stopSampler()), [collapseProbe.name]);
    const collapsed = await page.evaluate(() => window.__bh.historyState());
    measure('collapsed', collapsed);
    assert(collapsed.rows === collapseProbe.before.rows && collapsed.pages === collapseProbe.before.pages, 'collapse discarded materialized history');
    assert(JSON.stringify(collapsed.folds) === JSON.stringify(collapseProbe.before.folds), 'collapse changed open/closed folds');

    // Dedicated fully-above expansion: unlike the first visible-clip click,
    // this button is deliberately clicked while its whole card is out above.
    const aboveExpansion = await page.evaluate((name) => {
      const bh = window.__bh, topic = bh.history().closest('.topic');
      const above = topic.getBoundingClientRect().bottom <= bh.viewTop(bh.scroller());
      bh.startSampler([name]);
      bh.history().querySelector('.history-older').click();
      return { above, steps: bh.historyState().steps };
    }, collapseProbe.name);
    assert(aboveExpansion.above && aboveExpansion.steps === 2, 'fully-above expansion fixture was not above or did not expand');
    await sleep(160);
    stable('fully-above-expansion', await page.evaluate(() => window.__bh.stopSampler()), [collapseProbe.name]);

    // Arm by showing the expanded card, then scroll the other way until it
    // is fully BELOW. A tall real preceding payload supplies scroll range;
    // shrinking a card below must not move that visible preceding topic.
    await page.evaluate(() => window.__bh.positionHistory());
    await page.waitForFunction(() => window.wbHistory.seen(window.__bh.history().closest('.topic'), '0px')?.visible, null, { timeout: 2000 });
    const prior = await page.evaluate(() => {
      const bh = window.__bh, topics = bh.topics();
      return topics[topics.indexOf(bh.history().closest('.topic')) - 1]?.dataset.topic;
    });
    assert(prior, 'need a topic before HISTORY for the below-collapse probe');
    if (kind !== 'standalone') await growFrame(page, prior, 1000);
    else await page.evaluate((name) => {
      const body = window.__bh.section(name).querySelector('.tdoc > .doc-body');
      if (!body) throw new Error('preceding topic has no inline payload');
      body.style.paddingBottom = (parseFloat(getComputedStyle(body).paddingBottom) + 1000) + 'px';
    }, prior);
    await waitQuiet(page);
    const belowCollapse = await page.evaluate((name) => {
      const bh = window.__bh, topic = bh.history().closest('.topic'), s = bh.scroller();
      const before = bh.historyState();
      s.scrollTop += topic.getBoundingClientRect().top - bh.viewBottom(s) - 40;
      bh.startSampler([name]);
      return { below: topic.getBoundingClientRect().top >= bh.viewBottom(s), before };
    }, prior);
    assert(belowCollapse.below && belowCollapse.before.steps === 2, 'below-collapse fixture was not expanded below the viewport');
    await page.waitForFunction(() => window.__bh.history()._wbHistorySteps === 1, null, { timeout: 3000 });
    await sleep(160);
    stable('below-viewport-collapse', await page.evaluate(() => window.__bh.stopSampler()), [prior]);
    const belowAfter = await page.evaluate(() => window.__bh.historyState());
    assert(belowAfter.rows === belowCollapse.before.rows && JSON.stringify(belowAfter.folds) === JSON.stringify(belowCollapse.before.folds),
      'below collapse lost rows or disclosure choices');

    // Reuse the dashboard's actual pin button and its real #bpins scroller.
    if (kind !== 'standalone') {
      assert(ctx.width >= 1600, 'history dashboard pinned-column check requires --width 1800');
      await page.evaluate(() => {
        const bh = window.__bh, topic = bh.history().closest('.topic');
        // A single 640px card may fit the column. Pin a tall peer as well,
        // so #bpins is already an overflowing scroller BEFORE keep chooses
        // the anchor's scroller (not only after the expansion).
        const peer = bh.topics().find((n) => n !== topic && n.querySelector('iframe.doc-frame'));
        if (!peer) throw new Error('need a tall peer for the pinned-column probe');
        bh.pinPeer = peer.dataset.topic;
        peer.querySelector('.tpin').click(); topic.querySelector('.tpin').click();
      });
      await page.waitForFunction(() => {
        const bh = window.__bh, pins = bh.history().closest('#bpins');
        return pins && document.documentElement.hasAttribute('data-pincol') && pins.scrollHeight > pins.clientHeight;
      }, null, { timeout: 3000 });
      await page.evaluate(() => { window.__bh.focus = window.__bh.history(); window.__bh.positionHistory(); });
      await waitQuiet(page);
      const pin = await page.evaluate(() => {
        const bh = window.__bh, h = bh.history(), s = bh.scroller(), before = bh.historyState();
        h.querySelector('.history-older').click();
        return { scroller: s.id, sameScroller: window.wbHistory.keeping(s)?.scroller === s, before: before.steps, after: bh.historyState().steps };
      });
      measure('pinned-column', pin);
      assert(pin.scroller === 'bpins' && pin.sameScroller && pin.after === pin.before + 1, 'pinned topic expansion did not keep in #bpins');
      await page.evaluate(() => {
        const bh = window.__bh;
        bh.history().closest('.topic').querySelector('.tpin').click();
        bh.section(bh.pinPeer).querySelector('.tpin').click(); bh.focus = null;
      });
      await waitQuiet(page);
    }
  }
  try {
    await page.evaluate(HISTORY_PAGE);
    assert(await page.evaluate(() => !!window.__bh.history()), 'seed HISTORY with serve --history-entries 180');
    if (ctx.navigationOnly) {
      // Recreate the previous 214px rAF excursion's near-full clip: load a
      // page and let an older payload grow before navigating. This is fixture
      // setup, not an alternative history implementation or keep session.
      const older = await page.evaluate(() => {
        const bh = window.__bh, h = bh.history();
        h.querySelector('.history-older').click(); h.querySelector('.history-older').click();
        const row = h.querySelector('.history-live > .wb-entry.panel');
        row.querySelector('.entry-panel').open = true;
        return row.id;
      });
      await growEntry(page, older, 420);
      await waitQuiet(page);
    } else await cards();
    if (ctx.cardsOnly) {
      report.omissions.push('history-cards intentionally excludes navigation/control/live probes; run history for those checks.');
      return;
    }

    // Force an actual first IO zero on the newly armed deep-link watch by
    // positioning the fixture card horizontally outside the viewport. The
    // real observer still runs, and vertical keep writes are not intercepted.
    const target = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), topic = h.closest('.topic');
      const pages = h.querySelectorAll('template[data-history-page]');
      const panel = pages[pages.length - 1]?.content.querySelector('.entry-panel[id]');
      if (!panel) throw new Error('need an older deep-link target');
      topic.style.transform = 'translateX(10000px)';
      const a = document.createElement('a'); a.href = '#' + panel.id; a.textContent = 'history target'; topic.appendChild(a); a.click(); a.remove();
      return panel.id;
    });
    await page.waitForFunction(() => { const bh = window.__bh, r = window.wbHistory.seen(bh.history().closest('.topic'), '0px'); return r && !r.visible; }, null, { timeout: 2000 });
    const zero = await page.evaluate(() => window.__bh.historyState());
    assert(zero.steps > 1, 'first observer zero collapsed the deep-linked card');
    measure('deep-link-initial-zero', { target, steps: zero.steps, clipScrollTop: zero.scrollTop });
    await page.evaluate(() => { window.__bh.history().closest('.topic').style.transform = ''; });
    await page.waitForFunction(() => window.wbHistory.seen(window.__bh.history().closest('.topic'), '0px')?.visible, null, { timeout: 2000 });
    const visible = await page.evaluate((id) => {
      const node = document.getElementById(id), r = node.getBoundingClientRect(), clip = node.closest('.wb-history').querySelector('.history-clip').getBoundingClientRect(), bh = window.__bh;
      return { y: r.top, clipTop: clip.top, clipBottom: clip.bottom, viewTop: bh.viewTop(bh.scroller()), viewBottom: bh.viewBottom(bh.scroller()), clipScrollTop: node.closest('.history-clip').scrollTop };
    }, target);
    measure('deep-link-visible', visible);
    assert(visible.y >= visible.clipTop - 1 && visible.y < visible.clipBottom && visible.y >= visible.viewTop && visible.y < visible.viewBottom && visible.clipScrollTop === 0, 'deep-link target not revealed in the bottom-anchored clip');
    const late = await page.evaluate((id) => {
      const entry = document.getElementById(id).closest('.wb-entry');
      let next = entry.nextElementSibling;
      while (next && !next.matches('.wb-entry.panel')) next = next.nextElementSibling;
      if (!next) throw new Error('need a panel payload below the deep-link target');
      next.querySelector('.entry-panel').open = true;
      return next.id;
    }, target);
    await sleep(100);
    await page.evaluate((id) => window.__bh.startSampler([id]), target);
    const arrivals = [];
    for (const px of [240, 180]) {
      const before = await page.evaluate(() => ({ t: performance.now(), keep: !!window.__bh.keeping(),
        target: window.__bh.history()._wbHistoryTarget?.id }));
      assert(before.keep && before.target === target, 'deep-link height report did not arrive during the open navigation session');
      if (arrivals.length) assert(before.t - arrivals.at(-1).t <= 300, 'deep-link height reports were more than 300ms apart');
      await growEntry(page, late, px);
      const after = await page.evaluate(() => ({ t: performance.now(), keep: !!window.__bh.keeping(), target: window.__bh.history()._wbHistoryTarget?.id }));
      // Actual arrival lies between before/after. This conservative upper
      // bound proves the reports themselves, not just our calls, are <=300ms.
      if (arrivals.length) assert(after.t - arrivals.at(-1).t <= 300, 'actual deep-link report spacing could exceed 300ms');
      assert(after.keep && after.target === target, 'navigation session ended while the height report was arriving');
      arrivals.push({ ...before, px, after });
      await sleep(150);
    }
    stable('deep-link-late-heights', await page.evaluate(() => window.__bh.stopSampler()), [target]);
    measure('deep-link-open-session-reports', { arrivals, gapsMs: arrivals.slice(1).map((r, i) => r.t - arrivals[i].t) });

    // Quiet/cap/user end is the existing shared helper's authority. A later
    // report may move the target, but must not resurrect navigation or raise
    // its clip. Wait for actual end, never infer it from a fixed sleep.
    await page.waitForFunction(() => !window.__bh.keeping() && window.__bh.history()._wbHistoryTarget === null, null, { timeout: 6000 });
    const ended = await page.evaluate((id) => ({ steps: window.__bh.historyState().steps,
      y: document.getElementById(id).getBoundingClientRect().top }), target);
    await page.evaluate((id) => window.__bh.startSampler([id]), target);
    await growEntry(page, late, 600);
    await sleep(200);
    const postEnd = await page.evaluate((id) => ({ steps: window.__bh.historyState().steps,
      target: window.__bh.history()._wbHistoryTarget, keep: !!window.__bh.keeping(),
      y: document.getElementById(id).getBoundingClientRect().top,
      samples: window.__bh.stopSampler() }), target);
    fs.writeFileSync(path.join(dir, 'deep-link-post-end.json'), JSON.stringify(postEnd.samples, null, 2));
    measure('deep-link-post-end', { before: ended, steps: postEnd.steps, target: postEnd.target, keep: postEnd.keep,
      dyAllowed: postEnd.y - ended.y });
    assert(postEnd.steps === ended.steps && postEnd.target === null && !postEnd.keep && postEnd.samples.filter((s) => s.y).every((s) =>
      !s.keep && s.history.steps === ended.steps && s.history.target === null),
      'post-session height report re-revealed, re-armed or resurrected navigation');

    // The deep-linked card has now really been in view: its next complete
    // exit must collapse, preserving folds, just like a manually expanded card.
    await page.evaluate(() => {
      const bh = window.__bh, s = bh.scroller(), topic = bh.history().closest('.topic');
      bh.deepFolds = bh.historyState().folds;
      s.scrollTop += topic.getBoundingClientRect().bottom - bh.viewTop(s) + 20;
    });
    await page.waitForFunction(() => window.__bh.history()._wbHistorySteps === 1, null, { timeout: 3000 });
    const deepCollapse = await page.evaluate(() => ({ steps: window.__bh.historyState().steps,
      preserved: JSON.stringify(window.__bh.deepFolds) === JSON.stringify(window.__bh.historyState().folds) }));
    measure('deep-link-in-to-out-collapse', deepCollapse);
    assert(deepCollapse.preserved, 'deep-link collapse changed disclosure choices');

    // Deep-link materialization exhausted the embedded pages. A collapsed
    // clip still needs its button; repeated clicks only raise the 640px clip
    // until all content fits, at which point the button must hide.
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    const exhausted = await page.evaluate(() => window.__bh.historyState());
    assert(exhausted.pages === 0 && exhausted.button && exhausted.live > 640, 'exhausted pages hid the button while clipped content remained');
    let fitted = exhausted, controlClicks = 0;
    while (fitted.button && controlClicks < 100) {
      const before = fitted;
      await page.evaluate(() => window.__bh.history().querySelector('.history-older').click());
      await sleep(20);
      fitted = await page.evaluate(() => window.__bh.historyState());
      assert(fitted.pages === 0 && fitted.steps === before.steps + 1, 'exhausted-page click changed more than the clip step');
      controlClicks++;
    }
    measure('exhausted-controls', { clicks: controlClicks, steps: fitted.steps, live: fitted.live, button: fitted.button });
    assert(!fitted.button && fitted.live <= fitted.steps * 640, 'button did not hide when exhausted content fitted the clip');

    // Return to the latest window through a real viewport exit. Then append
    // one cloned, rendered system-event chip as a faithful live-arrival HTML
    // fixture: no copied stream helpers or fabricated CSS, on either host.
    await page.evaluate(() => {
      const bh = window.__bh, s = bh.scroller();
      s.scrollTop += bh.history().closest('.topic').getBoundingClientRect().bottom - bh.viewTop(s) + 20;
    });
    await page.waitForFunction(() => window.__bh.history()._wbHistorySteps === 1, null, { timeout: 3000 });
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    const arrivalBefore = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), live = h.querySelector('.history-live'), before = bh.historyState();
      const src = Array.from(live.children).find((n) => n.matches('.system-run'));
      if (!src) throw new Error('no rendered system event for the arrival fixture');
      const chip = src.cloneNode(true), entry = chip.querySelector('.wb-entry'), old = entry.id;
      chip.innerHTML = chip.innerHTML.replaceAll(old, 'e999001');
      chip.dataset.key = 'system:e999001'; chip.dataset.historyAnchor = 'e999001'; chip.open = false;
      chip.querySelector('.entry-text').textContent = 'Live arrival fixture';
      chip.id = 'bh-live-chip'; live.appendChild(chip);
      return before;
    });
    await sleep(100);
    const arrival = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), clip = h.querySelector('.history-clip').getBoundingClientRect(), chip = document.getElementById('bh-live-chip');
      return { ...bh.historyState(), newestAtBottom: Math.abs(chip.getBoundingClientRect().bottom - clip.bottom) <= 1 };
    });
    measure('live-arrival-fixture', { stepsBefore: arrivalBefore.steps, stepsAfter: arrival.steps,
      rowsAdded: arrival.rows - arrivalBefore.rows, newestAtBottom: arrival.newestAtBottom, clipScrollTop: arrival.scrollTop });
    assert(arrival.steps === arrivalBefore.steps && arrival.rows === arrivalBefore.rows + 1 && arrival.newestAtBottom && arrival.scrollTop === 0,
      'live arrival did not stay at the bottom of an unchanged clip');
  } catch (e) {
    report.failure = e.message;
    report.omissions.push('Stopped at the recorded failure; subsequent checks in the scenario were not run.');
    throw e;
  } finally {
    report.errors = ctx.errors; report.failures = ctx.failures;
    fs.writeFileSync(path.join(dir, 'measurements.json'), JSON.stringify(report, null, 2));
    log(`history artifacts: ${dir}`);
  }
}

const SCENARIOS = {
  keep: scenarioKeep,
  observer: scenarioObserver,
  history: scenarioHistory,
  'history-cards': (ctx) => scenarioHistory({ ...ctx, cardsOnly: true }),
  'history-paint': (ctx) => scenarioHistory({ ...ctx, navigationOnly: true }),
};

// --- main ----------------------------------------------------------------------

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const name = args._[0];
  if (name === "serve") return serve(args);
  if (!name || !SCENARIOS[name] || !args.url || args.url === true) {
    console.error("usage: node scripts/board-headless.mjs serve [--topics N] [--history-entries N] [--keep-tmp]");
    console.error(`       node scripts/board-headless.mjs <${Object.keys(SCENARIOS).join("|")}> --url <session or full file page> [--width W] [--height H] [--headed] [--artifacts DIR]`);
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

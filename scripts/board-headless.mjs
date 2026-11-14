#!/usr/bin/env node
// Repository-local developer command: headless browser scenarios for the
// dashboard board's motion and scroll behaviour. Not shipped with any plugin
// package.
//
//     node scripts/board-headless.mjs serve [--topics N] [--pending N] [--history-entries N] [--mermaid] [--keep-tmp]
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
//   bands     Viewport mount/unmount (needs `serve --mermaid --topics 30`:
//             the oldest topic, MERMAID, holds a mermaid fence and a panel,
//             and the topics above it were never laid out at load). At load
//             the bottom topic's frame is a placeholder; a jump (location hash) to
//             its panel lands and holds within 1 px while the frames around it
//             mount, and draws its mermaid; scrolling up and back down a
//             step at a time keeps the topic in view within 1 px of where the
//             step put it (no jump), mounts frames before they enter the view,
//             turns frames far from the view into placeholders that keep
//             their height, and remounts the mermaid topic with its diagram
//             drawn again in a new frame. Also defers one body's reporting
//             script above the jump target until keep ends, then requires
//             exactly one scroll compensation and <=1px post-paint drift.
//
//   pill      Static pending counts and frame fixed point (use `serve --topics
//             30 --pending 11 --mermaid`). At two scroll stops, all N pending
//             controls must be above the view. After a bounded settling window,
//             checks every rAF for 4 seconds: count N, no scroll drift, and zero
//             iframe mounts/unmounts. Runs on both dashboard page kinds.
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
//   history-regressions  Native RO delivery for renderer-produced closed log
//             pages, collapse cancellation, and real Tab into clipped summaries.
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
  env.HOME = tmp;
  return Object.assign(env, extra || {});
}
function run(cmd, args, opts) {
  const r = spawnSync(cmd, args, Object.assign({ encoding: "utf8" }, opts));
  if (r.status !== 0) throw new Error(`${cmd} ${args.join(" ")} failed (${r.status}): ${(r.stderr || "") + (r.stdout || "")}`.slice(0, 4000));
  return r.stdout;
}

function topicCommands(i, pending = 0) {
  const name = `T${String(i).padStart(2, "0")}`;
  const paras = [];
  for (let p = 0; p < 9; p++) {
    paras.push(`<p>${name} paragraph ${p + 1}: board motion fixture text that wraps over a couple of lines so the topic body frame stands well above a few hundred pixels tall, which is what makes the board overflow the viewport several times over.</p>`);
  }
  const checks = Array.from({ length: pending }, (_, j) => `<wb:add class="check"><wb:title>Pending fixture ${j + 1}</wb:title></wb:add>`).join("");
  return `<wb:topic name="${name}"><wb:body><h3>${name}</h3>${paras.join("")}</wb:body>${checks}</wb:topic>`;
}

// seedSession makes a project with one ws session and a board of n topics,
// and returns its reply id and canonical root.
function seedSession(tmp, bin, label, n, historyEntries, mermaid, pending) {
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
  // The mermaid topic is seeded first, so it is the oldest, at the bottom:
  // a body with a mermaid fence and a panel, the bands scenario's jump
  // target.
  if (mermaid) {
    const body = "## MERMAID\n\n```mermaid\ngraph TD; A-->B; B-->C; C-->A\n```\n";
    const out = run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key,
      commands: `<wb:topic name="MERMAID"><wb:body>${body}</wb:body><wb:panel recap="Jump target"><p>Panel under the mermaid topic.</p></wb:panel></wb:topic>` })], { cwd: proj, env });
    const m = /ai-docs\/whiteboards\/([a-z0-9]+)\/index\.html/.exec(out);
    if (m) rid = m[1];
  }
  // One update per topic, so each topic has its own last-change time.
  for (let i = 1; i <= n; i++) {
    const out = run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key, commands: topicCommands(i, i === n ? pending : 0) })], { cwd: proj, env });
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
    // Renderer-produced closed log runs exercise size-neutral page joins:
    // newest 47 folded rows + three visible tail rows; older pages fold whole.
    for (const [name, count] of [['LOGCHAIN', 200], ['LOGCANCEL', 600]]) {
      const logs = Array.from({ length: count }, (_, i) => `<wb:log>Folded history line ${i + 1}</wb:log>`).join('');
      run(bin, ['call', 'whiteboard.update', JSON.stringify({
        session_key: fer.session_key, commands: `<wb:topic name="${name}">${logs}</wb:topic>`
      })], { cwd: proj, env });
    }
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
    const adapter = seedSession(tmp, bin, "adapter", n, +(args["history-entries"] || 0), !!args.mermaid, +(args.pending || 0));
    const board = seedSession(tmp, bin, "board", n, +(args["history-entries"] || 0), !!args.mermaid, +(args.pending || 0));
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
  const bh = (window.__bh = { sockets: [], board: null, writes: [], resizeDeliveries: [], historyLoads: [], observerErrors: [], observerErrorDetails: [] });
  // Observe actual native deliveries; never invoke a callback or re-observe
  // on the harness's behalf. Filter to the history's own observer, excluding
  // the shared keep observer which also watches history-live.
  window.addEventListener('error', (e) => {
    if (!/ResizeObserver.*loop/i.test(e.message || '')) return;
    bh.observerErrors.push(e.message);
    bh.observerErrorDetails.push({ message: e.message, t: performance.now(), phase: bh.regressionPhase || 'startup',
      lastHistoryDelivery: bh.resizeDeliveries.at(-1)?.id || null });
  });
  const NativeResize = window.ResizeObserver;
  window.ResizeObserver = class extends NativeResize {
    constructor(callback) {
      super((entries, observer) => {
        const targets = entries.filter((e) => e.target.matches?.('.history-live') &&
          e.target.closest('.wb-history')?._wbHistoryObserver === observer);
        const snapshot = (live) => {
          const h = live.closest('.wb-history');
          return { name: h.dataset.history, pages: h.querySelectorAll('template[data-history-page]').length,
            rows: live.querySelectorAll('.wb-entry').length, height: live.scrollHeight, steps: h._wbHistorySteps };
        };
        const record = targets.length ? { id: bh.resizeDeliveries.length + 1, t: performance.now(),
          before: targets.map((e) => snapshot(e.target)) } : null;
        const previous = bh.activeResize;
        if (record) { bh.resizeDeliveries.push(record); bh.activeResize = record.id; }
        try { callback.call(observer, entries, observer); }
        finally {
          if (record) record.after = targets.map((e) => snapshot(e.target));
          bh.activeResize = previous;
        }
      });
    }
  };
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
  // The shell can reuse its child's initial about:blank window with a NEW
  // Document. Bind document events here, not in that initial document's INIT.
  if (bh.historyLoadDocument !== document) {
    bh.historyLoadDocument = document;
    document.addEventListener('wbhistoryload', (e) => {
      const h = e.detail?.root;
      if (h?.matches('.wb-history')) bh.historyLoads.push({ t: performance.now(), name: h.dataset.history,
        delivery: bh.activeResize || null, pages: h.querySelectorAll('template[data-history-page]').length,
        rows: h.querySelectorAll('.history-live .wb-entry').length });
    });
  }
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
      // The morph compares a topic's server hash (data-th) when it has one,
      // so a changed section needs a new hash, as the server would give it.
      if (sec.hasAttribute("data-th")) sec.setAttribute("data-th", "bh" + at.toString(16));
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
    // Frames of either kind: a topic far from the view holds placeholders
    // (the viewport bands), and a changed one gets new placeholders.
    const look = (n) => ({ n, frames: q(".doc-frame:not(.doc-frame-next)", n) });
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

// scrollToMiddle puts the scroller at the middle of its range, or at most
// views viewports down when given, and returns the topic layout there.
async function scrollToMiddle(page, views) {
  await page.evaluate((v) => { const s = window.__bh.scroller(); s.scrollTop = Math.round(Math.min((s.scrollHeight - s.clientHeight) / 2, v ? v * s.clientHeight : Infinity)); }, views || 0);
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
  // At most 2.5 viewports down: the topic updated above rises to the
  // head, which must stay within the viewport bands' far band (4 viewports)
  // for its frame to stay mounted and report late.
  const lay = await scrollToMiddle(page, 2.5);
  const above = lay.filter((t) => t.where === "above"), below = lay.filter((t) => t.where === "below"), inView = lay.filter((t) => t.where === "in");
  assert(above.length >= 3 && below.length >= 1 && inView.length >= 1, `the board must overflow both ways from the middle (above ${above.length}, in view ${inView.length}, below ${below.length}); seed more topics`);
  // The update above is the second topic (it rises past the head, off
  // screen); the update below is the bottom open topic, which rises past
  // the viewport to the head. The frame that grows late is the one the
  // first swap brought for the topic updated above: a swap's frames report
  // their heights late (load, reveal), which is what the compensation has
  // to hold through after the swap's own motion.
  // History seeds also contain log-only topics. Late body-height probes
  // require a real frame, not simply the second topic in board order.
  const framedAbove = await page.evaluate((names) => names.filter((n) => window.__bh.section(n).querySelector('.tdoc iframe.doc-frame')), above.map((t) => t.name));
  assert(framedAbove.length >= 2, 'need two framed topics above for the late-height probe');
  // The framed topic above nearest the view: one far up may be unmounted,
  // and its update brings a placeholder the viewport bands mount only once
  // it comes near, not a frame that reports late.
  const upAbove = framedAbove.at(-1), lateGrow = upAbove, upBelow = below[below.length - 1].name;
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
  const framedAbove2 = await page.evaluate((names) => names.filter((n) => window.__bh.section(n).querySelector('.tdoc iframe.doc-frame')), above2.map((t) => t.name));
  assert(framedAbove2.length >= 1, 'need a framed topic above for the post-user-scroll height probe');
  const up2 = above2[1].name, grow2 = framedAbove2.at(-1);
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
  const expected = ['board-overview', ...(first.terminal ? ['tnew'] : []), ...(first.order.some((s) => /wnew/.test(s)) ? ['wnew'] : []), 'board-search', ...(first.files ? ['board-files'] : [])];
  qa(first.order.length === expected.length && expected.every((s, i) => first.order[i].includes(s)), 'toolbar action order', first.order);
  qa(first.labels.every((x) => x.shown === (x.overview || first.width > 540)) && (first.width > 540 || first.hint === 'none'), 'label collapse keys on toolbar width, preserving Overview', first);
  if (first.files) {
    // File responses are browser-local fixtures; no host files or native
    // actions are opened. Exercise the mounted explorer, not a replacement.
    await tab.route('**/fs/**', async (route) => {
      const u = new URL(route.request().url()), op = u.pathname.split('/').pop();
      let body;
      if (op === 'list') {
        const dir = u.searchParams.get('dir') || '';
        body = { root: '/fixture', name: 'fixture', session: true, abs: '/fixture' + (dir ? '/' + dir : ''), sep: '/',
          entries: dir ? [{ name: 'main.go' }] : [{ name: 'src', dir: true }, ...Array.from({ length: 100 }, (_, i) => ({ name: `file-${i}.txt` }))] };
      } else if (op === 'find') body = { items: [{ path: 'src/main.go', name: 'main.go' }] };
      else if (op === 'preview') body = { kind: 'text', text: 'Files toolbar preview fixture' };
      else return route.continue();
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
    });
    await page.evaluate(() => {
      __bh.filesNode = document.querySelector('#files');
      __bh.filesParent = __bh.filesNode.parentNode;
      __bh.filesTabs = ['data-tab', 'data-ctab'].map((k) => document.querySelector('#grid').getAttribute(k));
    });
    const opened = () => page.evaluate(() => document.querySelector('#files').hasAttribute('data-max'));
    const restored = () => page.evaluate(() => {
      const n = document.querySelector('#files');
      return n === __bh.filesNode && n.parentNode === __bh.filesParent && !n.hasAttribute('data-max') &&
        JSON.stringify(__bh.filesTabs) === JSON.stringify(['data-tab', 'data-ctab'].map((k) => document.querySelector('#grid').getAttribute(k))) &&
        document.activeElement.matches('.board-files') && !!document.querySelector('#chead .fsum .froot');
    });
    await page.click('.board-files');
    await page.waitForSelector('#files .fr[data-path="src"]');
    qa(await page.evaluate(() => {
      const n = document.querySelector('#files'), r = n.getBoundingClientRect();
      return n === __bh.filesNode && n.parentNode === __bh.filesParent && getComputedStyle(n).position === 'fixed' &&
        document.activeElement.matches('#files .fq') && document.querySelectorAll('#files').length === 1 &&
        document.querySelector('#files .fhd .froot') && r.left >= 0 && r.right <= innerWidth && r.bottom <= innerHeight && r.height > 100;
    }), 'Files toolbar opens the same explorer as a viewport-fitting floating sheet and focuses search');
    await page.fill('#files .fq', 'main');
    await page.waitForSelector('#files .hit');
    await page.click('.board-files');
    qa(await restored() && await page.inputValue('#files .fq') === 'main', 'second Files click closes, preserves query/tab/placement and returns label/focus');
    await page.click('.board-files');
    await page.click('#files .fhd button:last-child');
    await tab.keyboard.press('Escape');
    qa(await opened() && await page.evaluate(() => !document.querySelector('#files .fpop:not([hidden])')), 'dropdown consumes Escape before Files sheet');
    await page.focus('#files .fq'); await tab.keyboard.press('Escape');
    qa(await opened() && await page.inputValue('#files .fq') === '', 'search Escape clears query before closing Files');
    await page.click('#files .fr[data-path="src"]');
    await page.waitForSelector('#files .fr[data-path="src/main.go"]');
    await page.click('#files .fr[data-path="src/main.go"]', { button: 'right' });
    qa(await page.evaluate(() => {
      const n = document.querySelector('.fmenu'), r = n.getBoundingClientRect();
      return n && n.contains(document.elementFromPoint(r.left + 10, r.top + 10));
    }), 'existing row menu remains above floating Files');
    await tab.keyboard.press('Escape');
    qa(await opened() && await page.evaluate(() => !document.querySelector('.fmenu') && document.activeElement.matches('#files .fr[data-path="src/main.go"]')), 'row-menu Escape consumes the event and restores its row');
    // Let the native scroll event reach the explorer's existing scroll
    // memory before hiding it (a synthetic assignment is not a user scroll).
    await page.evaluate(() => new Promise((resolve) => {
      document.querySelector('#files .ftree').scrollTop = 100;
      requestAnimationFrame(() => requestAnimationFrame(resolve));
    }));
    await tab.keyboard.press('Escape');
    qa(await restored(), 'unconsumed Escape dismisses Files and restores focus and original tabs');
    await page.click('.board-files');
    qa(await page.evaluate(() => document.querySelector('#files .fr[data-path="src"]').getAttribute('aria-expanded') === 'true' && document.querySelector('#files .ftree').scrollTop === 100), 'reopening preserves expanded folders and tree scroll');
    await page.evaluate(() => { document.querySelector('#files .ftree').scrollTop = 0; });
    await page.click('#files .fr[data-path="src/main.go"]');
    await page.waitForSelector('.pv-txt');
    qa(!await opened() && await page.textContent('.pv-txt') === 'Files toolbar preview fixture', 'file preview remains visible in the established board slot, dismissing Files');
    await ovAt(page, 'top');
    await page.evaluate(() => document.querySelector('#ctabs [data-ctab="files"]').click());
    await page.evaluate(() => { __bh.filesTabs = ['data-tab', 'data-ctab'].map((k) => document.querySelector('#grid').getAttribute(k)); });
    await page.click('.board-files'); await tab.keyboard.press('Escape');
    qa(await restored() && await page.evaluate(() => document.querySelector('#grid').getAttribute('data-ctab') === 'files'), 'original Files-tab access and selection survive floating dismissal');
    await page.evaluate(() => document.querySelector('#ctabs [data-ctab="convo"]').click());
    await page.evaluate(() => { document.querySelector('#r-board section.topic .d-max').click(); document.querySelector('.board-files').click(); });
    qa(await opened() && await page.evaluate(() => !document.querySelector('section.topic[data-max]')), 'Files uses shared maximize exclusivity');
    await page.evaluate(() => document.querySelector('#r-board section.topic .d-max').click());
    qa(!await opened() && await page.evaluate(() => !!document.querySelector('section.topic[data-max]')), 'another maximized sheet dismisses and restores Files');
    await tab.keyboard.press('Escape');
    // Board swaps run the same decorate/follow path as production.
    await page.click('.board-files');
    await page.evaluate(async () => { const b = await __bh.baseBoard(); __bh.ovRegion(b.html); });
    await waitQuiet(page);
    qa(await opened(), 'board updates do not dismiss the borrowed explorer as a missing topic');
    await tab.keyboard.press('Escape');
    await tab.unroute('**/fs/**');
    await ovAt(page, 'top');
  }
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
  if (first.files) {
    for (const absent of ['pane', 'module']) {
      const p = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height } });
      await p.route('**/*', async (route) => {
        if (!route.request().isNavigationRequest()) return route.continue();
        const response = await route.fetch();
        let body = await response.text();
        body = absent === 'pane' ? body.replace(/<section id="files"[^>]*><\/section>/, '') : body.replace('window.WsFiles = {', 'window.__bhDisabledFiles = {');
        await route.fulfill({ response, body });
      });
      await p.goto(ctx.args.url); await p.waitForTimeout(500);
      const f = p.frames().find((f) => /[?&]frame=1/.test(f.url())) || p;
      await f.waitForSelector('.board-toolbar', { state: 'attached' });
      qa(await f.evaluate(() => !document.querySelector('.board-files')), `Files toolbar absent without ${absent} (server-document fixture)`);
      await p.close();
    }
  }
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

// BANDS_PAGE's bandTopics reads, per open topic, where it stands against the view and
// its body frame: mounted (iframe), a placeholder with its height, or none
// for a bodyless log/history topic. shown excludes deliberately closed folds.
const BANDS_PAGE = () => {
  const bh = window.__bh;
  bh.bandTopics = () => {
    const s = bh.scroller(), top = bh.viewTop(s), bottom = bh.viewBottom(s), h = bottom - top;
    return bh.topics().map((n) => {
      const r = n.getBoundingClientRect(), f = n.querySelector(".tdoc .doc-frame");
      let shown = !!f;
      for (let p = f?.parentElement; p && p !== n; p = p.parentElement) if (p.tagName === 'DETAILS' && !p.open) shown = false;
      return { name: n.getAttribute("data-topic"), top: r.top - top, bottom: r.bottom - top, view: h, win: innerHeight,
        frame: f ? (f.tagName === "IFRAME" ? "iframe" : "placeholder") : "none", shown, near: n.wbNear ?? null, height: f ? f.style.height : "" };
    });
  };
};

async function mermaidDrawn(page, name) {
  const handle = await page.$(`section.topic[data-topic="${name}"] .tdoc iframe.doc-frame`);
  if (!handle) return { frame: null, drawn: false };
  const frame = await handle.contentFrame();
  const drawn = frame ? await frame.waitForFunction(() => !!document.querySelector("pre.mermaid svg"), null, { timeout: 8000 }).then(() => true, () => false) : false;
  return { frame: handle, drawn };
}

async function scenarioBands(ctx) {
  const { page, kind } = await openBoard(ctx);
  log(`page kind: ${kind}`);
  await page.evaluate(BANDS_PAGE);
  assert(await page.evaluate(() => !!(window.wbHistory && window.wbHistory.bands)), "this browser's view kit has no scroll margins (wbHistory.bands): the viewport bands are off");
  assert(await page.evaluate(() => !!window.__bh.section("MERMAID")), "no MERMAID topic on the board: run `serve --mermaid`");
  const M = "MERMAID";
  // 1. At load the bottom topic is far below: a placeholder never mounted.
  const load = await page.evaluate(() => window.__bh.bandTopics());
  const mLoad = load.find((t) => t.name === M);
  ctx.check(mLoad.frame === "placeholder", `at load the far MERMAID topic's body frame is a ${mLoad.frame}, expected a placeholder`);
  // History fixtures also contain bodyless topics (frame: "none"). They
  // are not lazy placeholders; nor should a deliberately closed body mount.
  const inViewPlaceholders = load.filter((t) => t.top < t.view && t.bottom > 0 && t.shown && t.frame === 'placeholder');
  ctx.check(!inViewPlaceholders.length, `at load shown body placeholders remain in view: ${JSON.stringify(inViewPlaceholders)}`);
  const anchoring = await page.evaluate(() => {
    const s = window.__bh.scroller();
    return getComputedStyle(s === document.scrollingElement ? document.documentElement : s).overflowAnchor;
  });
  ctx.check(anchoring === 'none', `native anchoring is ${anchoring} on the ${kind} scroller; it would compete with topicResized`);
  // Topics never laid out (a placeholder with no height) are what grows
  // above the view on the way back up from the jump.
  const unsized = load.filter((t) => t.frame === "placeholder" && !t.height).length;
  assert(unsized >= 4, `only ${unsized} topic(s) were never laid out at load: run \`serve --mermaid --topics 30\``);

  // Select AND prepare a live placeholder in one browser task, before any
  // mount can consume its first height. A late message listener is not a
  // reliable hold: the dashboard's listener is already registered.
  const held = await page.evaluate((name) => {
    const bh = window.__bh, topics = bh.topics(), target = bh.section(name);
    const topic = topics[topics.indexOf(target) - 1], p = topic?.querySelector('.tdoc .doc-frame');
    if (!p || p.tagName !== 'DIV' || p.style.height) return { error: 'immediately preceding body must still be an unsized placeholder', topic: topic?.dataset.topic, tag: p?.tagName, height: p?.style.height };
    const source = p.getAttribute('data-srcdoc'), parsed = new DOMParser().parseFromString(source, 'text/html');
    const script = Array.from(parsed.querySelectorAll('script[src]')).find((s) => /(?:^|\/)wb-frame-[a-f0-9]+\.js$/.test(s.getAttribute('src')));
    if (!script || parsed.querySelector('pre')) return { error: 'need a plain body with the shared frame script', topic: topic.dataset.topic };
    // No synthetic height or MessageEvent: defer execution of the actual
    // reporting script, then start that same asset in the real child frame.
    script.type = 'application/x-board-headless-held';
    script.setAttribute('data-bh-band-script', '');
    p.setAttribute('data-srcdoc', '<!DOCTYPE html>\n' + parsed.documentElement.outerHTML);
    // MERMAID is last in the seeded board. Without tail room, a fragment
    // jump clamps near the bottom and the preceding topic stays IN view.
    const tail = document.createElement('div');
    tail.style.height = 3 * bh.scroller().clientHeight + 'px';
    tail.setAttribute('aria-hidden', 'true');
    bh.board_().appendChild(tail);
    bh.bandProbe = { topic: topic.dataset.topic, source, tail };
    return { topic: topic.dataset.topic };
  }, M);
  assert(!held.error, `cannot prepare first-height probe: ${JSON.stringify(held)}`);
  const heldTopic = held.topic;

  // 2. A jump to the panel under MERMAID lands and holds while the frames
  // around it mount and report their heights.
  const pid = await page.evaluate((n) => { const e = window.__bh.section(n).querySelector(".wb-entry[data-panel]"); return e && e.getAttribute("data-panel"); }, M);
  assert(pid, "the MERMAID topic has no panel to jump to");
  await page.evaluate((id) => { window.__bh.jumpTarget = id; location.hash = "#" + id; }, pid);
  await page.waitForFunction((id) => { const t = document.getElementById(id); return t && t.getBoundingClientRect().height > 0; }, pid, { timeout: 3000 })
    .catch(() => { throw new Failure(`the jump to ${pid} found no target`); });
  await sleep(100);
  const ys = [];
  const t0 = Date.now();
  while (Date.now() - t0 < 2000) {
    ys.push(await page.evaluate((id) => document.getElementById(id).getBoundingClientRect().top, pid));
    await sleep(50);
  }
  const drift = Math.max(...ys) - Math.min(...ys);
  ctx.check(drift <= 1, `the jump target ${pid} moved ${drift.toFixed(1)}px within 2s of landing (from ${ys[0].toFixed(1)})`);
  const land = await page.evaluate((id) => { const bh = window.__bh, s = bh.scroller(); return document.getElementById(id).getBoundingClientRect().top - bh.viewTop(s); }, pid);
  ctx.check(land > -2 && land < 900, `the jump target ${pid} stands ${land.toFixed(1)}px from the view's top`);
  const m1 = await mermaidDrawn(page, M);
  ctx.check(m1.frame && m1.drawn, "after the jump the MERMAID topic's frame did not mount with its diagram drawn");
  log(`jump to ${pid}: landed ${land.toFixed(1)}px from the view top, drift ${drift.toFixed(1)}px over 2s; mermaid ${m1.drawn ? "drawn" : "not drawn"}`);

  await page.waitForFunction((name) => window.__bh.section(name)?.querySelector('.tdoc iframe.doc-frame') && !window.__bh.keeping(), heldTopic, { timeout: 8000 });
  const heldHandle = await page.$(`section.topic[data-topic="${heldTopic}"] .tdoc iframe.doc-frame`);
  const heldFrame = await heldHandle.contentFrame();
  assert(heldFrame, 'held placeholder did not mount a child frame');
  await heldFrame.waitForLoadState('load');
  // Position explicitly, outside keep, with the held topic's bottom 80px
  // above the view. Tail space prevents clamping; verify it did not happen.
  await page.evaluate((name) => {
    const bh = window.__bh, s = bh.scroller();
    s.scrollTop += bh.section(name).getBoundingClientRect().bottom - bh.viewTop(s) + 80;
  }, heldTopic);
  await waitQuiet(page);
  const to = await heldFrame.evaluate(() => {
    if (!document.querySelector('script[data-bh-band-script]')) throw new Error('held reporting script is missing');
    return Math.ceil(document.querySelector('.doc-body').getBoundingClientRect().height);
  });
  const firstHeight = await page.evaluate(({ name, pid, to }) => {
    const bh = window.__bh, s = bh.scroller(), t = bh.section(name), f = t.querySelector('.tdoc iframe.doc-frame'), r = t.getBoundingClientRect();
    const before = { targetY: document.getElementById(pid).getBoundingClientRect().top, st: s.scrollTop,
      topicTop: r.top, topicBottom: r.bottom, viewTop: bh.viewTop(s), viewBottom: bh.viewBottom(s), above: r.bottom <= bh.viewTop(s),
      from: parseFloat(f.style.height) || 0, to, first: !!f.wbFirst, keeping: !!bh.keeping(), writes: bh.writesOn(s),
      anchorInline: (s === document.scrollingElement ? document.documentElement : s).style.overflowAnchor,
      anchoring: getComputedStyle(s === document.scrollingElement ? document.documentElement : s).overflowAnchor };
    bh.samplePostPaint = true;
    bh.startSampler([pid]);
    return before;
  }, { name: heldTopic, pid, to });
  assert(firstHeight.above && firstHeight.first && firstHeight.from === 0 && firstHeight.to > 0 && !firstHeight.keeping &&
    firstHeight.targetY >= firstHeight.viewTop && firstHeight.targetY < firstHeight.viewBottom,
    `delayed first-height probe must grow an above-view mount outside keep with a visible reference: ${JSON.stringify(firstHeight)}`);
  ctx.check(firstHeight.anchoring === 'none' && firstHeight.anchorInline === '',
    `probe must exercise stylesheet anchoring, not keep's inline override: ${JSON.stringify(firstHeight)}`);
  // The original doc-frame.js installs its native ResizeObserver and posts
  // its first real height. Dashboard applyFrameHeights/topicResized handle
  // it normally; no harness code writes frame height or scrollTop here.
  await heldFrame.evaluate(() => new Promise((resolve, reject) => {
    const held = document.querySelector('script[data-bh-band-script]'), live = document.createElement('script');
    live.src = held.getAttribute('src');
    live.onload = () => resolve();
    live.onerror = () => reject(new Error('held frame asset failed to load'));
    held.replaceWith(live);
  }));
  await sleep(800);
  const firstAfter = await page.evaluate(({ pid, name }) => {
    const bh = window.__bh, s = bh.scroller(), samples = bh.stopSampler().filter((v) => v.phase !== 'raf');
    const f = bh.section(name).querySelector('.tdoc iframe.doc-frame');
    bh.samplePostPaint = false;
    return { y: document.getElementById(pid).getBoundingClientRect().top, st: s.scrollTop, writes: bh.writesOn(s),
      height: parseFloat(f.style.height) || 0, first: !!f.wbFirst, keeping: !!bh.keeping(), samples };
  }, { pid, name: heldTopic });
  ctx.check(firstAfter.height === firstHeight.to && !firstAfter.first && !firstAfter.keeping,
    `real first height must land and consume wbFirst outside keep: height=${firstAfter.height}, first=${firstAfter.first}, keeping=${firstAfter.keeping}`);
  const delta = firstHeight.to - firstHeight.from;
  const firstDrift = Math.max(...firstAfter.samples.map((v) => Math.abs(v.y[pid] - firstHeight.targetY)));
  ctx.check(firstDrift <= 1 && Math.abs(firstAfter.y - firstHeight.targetY) <= 1,
    `above-view ${heldTopic} first height moved ${pid} by ${firstDrift.toFixed(1)}px post-paint`);
  ctx.check(Math.abs(firstAfter.st - firstHeight.st - delta) <= 1 && firstAfter.writes > firstHeight.writes,
    `above-view first height must compensate once: height +${delta}, scrollTop +${firstAfter.st - firstHeight.st}, script writes +${firstAfter.writes - firstHeight.writes}`);
  log(`above-view first height (${kind}): ${heldTopic} +${delta}px, scroll +${firstAfter.st - firstHeight.st}px, post-paint drift ${firstDrift.toFixed(1)}px; baseline ${JSON.stringify(firstHeight)}`);
  await page.evaluate((name) => {
    const bh = window.__bh, f = bh.section(name).querySelector('.tdoc iframe.doc-frame');
    // Restore the original payload for subsequent real unmount/remounts.
    f.setAttribute('data-srcdoc', bh.bandProbe.source);
    bh.bandProbe.tail.remove();
    delete bh.bandProbe;
  }, heldTopic);
  await waitQuiet(page);

  // 3. Up to the top a step at a time, then back down: each step keeps the
  // topic in view where the step put it, frames mount before they enter the
  // view, and frames far from the view are placeholders with their height.
  const steps = async (dir) => {
    const bad = [], late = [];
    let worst = 0;
    for (let i = 0; i < 80; i++) {
      const before = await page.evaluate(() => {
        const bh = window.__bh, s = bh.scroller(), top = bh.viewTop(s);
        const ref = bh.topics().find((n) => n.getBoundingClientRect().bottom > top + 40);
        return { st: s.scrollTop, max: s.scrollHeight - s.clientHeight, ref: ref.getAttribute("data-topic"), y: ref.getBoundingClientRect().top, h: s.clientHeight };
      });
      if (dir < 0 ? before.st <= 0 : before.st >= before.max - 1) break;
      const want = Math.round(before.h * 0.6) * dir;
      const moved = await page.evaluate((d) => { const s = window.__bh.scroller(), was = s.scrollTop; s.scrollTop = was + d; return s.scrollTop - was; }, want);
      await sleep(250);
      const after = await page.evaluate((ref) => window.__bh.section(ref).getBoundingClientRect().top, before.ref);
      const off = after - (before.y - moved);
      worst = Math.max(worst, Math.abs(off));
      if (Math.abs(off) > 1) bad.push(`${before.ref} ${off.toFixed(1)}px at scrollTop ${before.st}`);
      const lay = await page.evaluate(() => window.__bh.bandTopics());
      lay.filter((t) => t.top < t.view && t.bottom > 0 && t.shown && t.frame === "placeholder").forEach((t) => late.push(`${t.name} at scrollTop ${before.st + moved}`));
    }
    return { bad, late, worst };
  };
  const up = await steps(-1);
  ctx.check(!up.bad.length, `scrolling up moved the topic in view off its step: ${up.bad.join("; ")}`);
  ctx.check(!up.late.length, `scrolling up showed placeholders in view: ${up.late.join("; ")}`);
  const top = await page.evaluate(() => window.__bh.bandTopics());
  // The far band reaches four window heights past the view's bottom.
  const far = top.filter((t) => t.frame !== 'none' && t.top > t.view + 4 * t.win + 50);
  const mTop = top.find((t) => t.name === M);
  ctx.check(far.length > 0 && far.every((t) => t.frame === "placeholder"), `at the top a topic beyond the far band holds a mounted frame: ${far.filter((t) => t.frame !== "placeholder").map((t) => t.name).join(", ")}`);
  ctx.check(far.every((t) => /px$/.test(t.height) && parseFloat(t.height) > 0), `a far placeholder lost its height: ${far.filter((t) => !/px$/.test(t.height)).map((t) => t.name).join(", ")}`);
  ctx.check(mTop.frame === "placeholder" && parseFloat(mTop.height) > 0, `at the top the MERMAID topic holds ${mTop.frame} (height ${mTop.height || "none"}), expected a placeholder with its height`);
  log(`up: worst step offset ${up.worst.toFixed(2)}px; at the top ${far.length} topic(s) beyond the far band, all placeholders with heights`);
  const down = await steps(1);
  ctx.check(!down.bad.length, `scrolling down moved the topic in view off its step: ${down.bad.join("; ")}`);
  ctx.check(!down.late.length, `scrolling down showed placeholders in view: ${down.late.join("; ")}`);
  const m2 = await mermaidDrawn(page, M);
  ctx.check(m2.frame && m2.drawn, "back at the bottom the MERMAID topic's frame did not remount with its diagram drawn");
  const fresh = m1.frame && m2.frame ? await page.evaluate(([a, b]) => a !== b && !a.isConnected, [m1.frame, m2.frame]) : false;
  ctx.check(fresh, "the MERMAID topic's frame at the bottom is not a new one: it never unmounted");
  log(`down: worst step offset ${down.worst.toFixed(2)}px; MERMAID remounted (${fresh ? "new frame" : "same frame"}), diagram ${m2.drawn ? "drawn" : "not drawn"}`);
}

async function scenarioPill(ctx) {
  const { page, kind } = await openBoard(ctx);
  const expected = +(ctx.args.pending || 11);
  assert(Number.isInteger(expected) && expected > 0, "--pending must be a positive integer");
  assert(await page.evaluate(() => !!window.wbHistory?.bands), "viewport bands must be active for the churn regression");
  const inventory = await page.evaluate(() => {
    const items = Array.from(document.querySelectorAll('.card.policy[id^="item-"], .arow[id^="item-"], .ckrow[data-id]:not([data-assumed])'));
    const topics = [...new Set(items.map((el) => el.closest('section.topic')))];
    return { count: items.length, topic: topics.length === 1 ? topics[0]?.dataset.topic : null };
  });
  assert(inventory.count === expected && inventory.topic,
    `expected ${expected} pending controls in one topic, got ${JSON.stringify(inventory)}; use serve --topics 30 --pending ${expected} --mermaid`);

  await page.evaluate(() => {
    const bh = window.__bh;
    bh.pillChurn = { mounts: 0, unmounts: 0 };
    const frames = (n) => n.nodeType !== 1 ? 0 : (n.matches('iframe.doc-frame') ? 1 : 0) + n.querySelectorAll('iframe.doc-frame').length;
    bh.pillMO = new MutationObserver((records) => {
      for (const r of records) {
        for (const n of r.addedNodes) bh.pillChurn.mounts += frames(n);
        for (const n of r.removedNodes) bh.pillChurn.unmounts += frames(n);
      }
    });
    bh.pillMO.observe(bh.board_(), { childList: true, subtree: true });
  });
  try {
    for (const views of [2, 5]) {
      await page.evaluate(({ name, views }) => {
        const bh = window.__bh, s = bh.scroller();
        s.scrollTop += bh.section(name).getBoundingClientRect().bottom - bh.viewTop(s) + views * s.clientHeight;
      }, { name: inventory.topic, views });
      // Fixed upper bound, not an unbounded wait-until-quiet that could hide a
      // perpetual mount loop. No input, scroll, or data mutations follow.
      await sleep(3000);
      const result = await page.evaluate(async ({ name, expected }) => {
        const bh = window.__bh, s = bh.scroller(), host = document.getElementById('bcol') || document.querySelector('.wrap');
        const pill = host.querySelector('.oq-wrap.top .oq-pill'), seg = pill?.querySelector('.oq-seg.open');
        const count = () => !pill || pill.hidden || !seg || seg.hidden ? 0 : Number(seg.querySelector('.n').textContent);
        bh.pillMO.takeRecords();
        bh.pillChurn = { mounts: 0, unmounts: 0 };
        const start = performance.now(), st = s.scrollTop, counts = new Set(), failures = [];
        let samples = 0, drift = 0;
        await new Promise((resolve) => {
          function sample() {
            samples++;
            counts.add(count());
            drift = Math.max(drift, Math.abs(s.scrollTop - st));
            if (bh.section(name).getBoundingClientRect().bottom > bh.viewTop(s)) failures.push('pending topic entered view');
            if (performance.now() - start < 4000) requestAnimationFrame(sample); else resolve();
          }
          sample();
        });
        const frames = Array.from(bh.board_().querySelectorAll('.doc-frame'));
        return { expected, samples, counts: [...counts], drift, failures: [...new Set(failures)], ...bh.pillChurn,
          mounted: frames.filter((f) => f.tagName === 'IFRAME').length,
          placeholders: frames.filter((f) => f.tagName === 'DIV').length };
      }, { name: inventory.topic, expected });
      log(`pill ${kind}, ${views} viewports past pending topic: ${JSON.stringify(result)}`);
      ctx.check(result.samples > 30 && result.counts.length === 1 && result.counts[0] === expected,
        `static pending count must stay ${expected}: ${JSON.stringify(result)}`);
      ctx.check(result.mounts === 0 && result.unmounts === 0, `static viewport churned frames: ${JSON.stringify(result)}`);
      ctx.check(result.drift <= 1 && !result.failures.length, `static viewport moved: ${JSON.stringify(result)}`);
      ctx.check(result.mounted > 0 && result.placeholders > 0, 'regression must exercise mounted frames and lazy placeholders together');
    }
  } finally {
    await page.evaluate(() => window.__bh.pillMO.disconnect());
  }
}

// History probes use the real host's wbHistory, event delegation, DOM and
// styles. Only payload height arrival and stream updates are synthetic.
const HISTORY_PAGE = () => {
  const bh = window.__bh;
  bh.samplePostPaint = true;
  bh.history = (name = 'HISTORY') => Array.from(document.querySelectorAll('.wb-history')).find((h) => h.dataset.history === name);
  bh.historyState = (name = 'HISTORY') => {
    const h = bh.history(name), live = h.querySelector('.history-live'), clip = h.querySelector('.history-clip');
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
    'Size-neutral log-chain and keyboard regressions run separately in history-regressions.',
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
      const body = window.__bh.section(name).querySelector('.tdoc > .doc-body, .history-live');
      if (!body) throw new Error('preceding topic has no inline body or log payload');
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
    const observerErrors = await page.evaluate(() => window.__bh.observerErrors);
    assert(observerErrors.length === 0, `native ResizeObserver loop errors: ${observerErrors.join(' | ')}`);
  } catch (e) {
    report.failure = e.message;
    report.omissions.push('Stopped at the recorded failure; subsequent checks in the scenario were not run.');
    throw e;
  } finally {
    report.errors = ctx.errors; report.failures = ctx.failures;
    report.native = await page.evaluate(() => ({ observerErrors: window.__bh.observerErrors,
      observerErrorDetails: window.__bh.observerErrorDetails })).catch(() => null);
    fs.writeFileSync(path.join(dir, 'measurements.json'), JSON.stringify(report, null, 2));
    log(`history artifacts: ${dir}`);
  }
}

// Remediation verification, not an independent review: real renderer output,
// native RO/IO deliveries and actual sequential keyboard focus on both hosts.
async function scenarioHistoryRegressions(ctx) {
  const { page, tab, kind } = await openBoard(ctx);
  const dir = path.resolve(ctx.args.artifacts || path.join(os.tmpdir(), `board-history-regressions-${Date.now()}`));
  fs.mkdirSync(dir, { recursive: true });
  const report = { url: ctx.args.url, kind, measurements: [], omissions: [] };
  const measure = (label, data) => { report.measurements.push({ label, ...data }); log(`${label}: ${JSON.stringify(data)}`); };
  const capture = async (name) => { const p = path.join(dir, `${kind}-${name}.png`); await tab.screenshot({ path: p }); log(`capture: ${p}`); };
  const paint = () => page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => setTimeout(resolve, 0))));
  try {
    await page.evaluate(HISTORY_PAGE);
    await page.evaluate(() => { window.__bh.regressionPhase = 'keyboard'; });
    assert(await page.evaluate(() => ['HISTORY', 'LOGCHAIN', 'LOGCANCEL'].every((n) => !!window.__bh.history(n))),
      'seed renderer-produced panel/log histories with serve --history-entries 180');

    // The old failure was a real Tab into the first clipped summary, not an
    // explicit focus() on the desired row or a synthetic focusin callback.
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    await page.locator('.wb-history[data-history="HISTORY"] > .history-older').focus();
    const keyboardBefore = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), clip = h.querySelector('.history-clip').getBoundingClientRect();
      const node = h.querySelector('.history-live .entry-panel > summary'), r = node.getBoundingClientRect();
      bh.expectedTab = node;
      return { buttonFocused: document.activeElement === h.querySelector('.history-older'),
        panel: node.closest('.entry-panel').id, top: r.top, bottom: r.bottom, clipTop: clip.top, clipBottom: clip.bottom,
        clipped: r.bottom <= clip.top, clipScrollTop: h.querySelector('.history-clip').scrollTop };
    });
    measure('keyboard-before-tab', keyboardBefore);
    assert(keyboardBefore.buttonFocused && keyboardBefore.clipped && keyboardBefore.clipScrollTop === 0, 'Tab fixture must start at load-more with the next summary clipped');
    await capture('before-tab');
    await tab.keyboard.press('Tab');
    await paint();
    await sleep(100);
    const keyboardAfter = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), node = document.activeElement, r = node.getBoundingClientRect();
      const clip = h.querySelector('.history-clip').getBoundingClientRect(), s = bh.scroller();
      const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return { expected: node === bh.expectedTab, summary: node.matches('.entry-panel > summary'),
        centerExposed: !!hit && (hit === node || node.contains(hit)),
        centerHit: hit && { tag: hit.tagName, id: hit.id, classes: hit.className },
        panel: node.closest('.entry-panel')?.id, top: r.top, bottom: r.bottom, clipTop: clip.top, clipBottom: clip.bottom,
        viewTop: bh.viewTop(s), viewBottom: bh.viewBottom(s), clipScrollTop: h.querySelector('.history-clip').scrollTop,
        steps: h._wbHistorySteps };
    });
    measure('keyboard-after-tab', keyboardAfter);
    await capture('after-tab');
    assert(keyboardAfter.expected && keyboardAfter.summary && keyboardAfter.top >= keyboardAfter.clipTop - 1 &&
      keyboardAfter.bottom <= keyboardAfter.clipBottom + 1 && keyboardAfter.top >= keyboardAfter.viewTop - 1 &&
      keyboardAfter.bottom <= keyboardAfter.viewBottom + 1 && keyboardAfter.clipScrollTop === 0 && keyboardAfter.centerExposed,
      'actual Tab focused a summary outside the clip/page, under a sticky overlay, or scrolled the clip');
    await page.evaluate(() => document.activeElement.blur());

    async function settledLog(name) {
      await page.evaluate((name) => {
        const bh = window.__bh, s = bh.scroller(), topic = bh.history(name).closest('.topic');
        s.scrollTop += topic.getBoundingClientRect().top - bh.viewTop(s) - 100;
      }, name);
      await page.waitForFunction((name) => window.__bh.resizeDeliveries.some((d) => d.after?.some((r) => r.name === name && r.steps === 1)), name, { timeout: 2000 });
      await waitQuiet(page);
      // Ensure an initial observer notification cannot be mistaken for a
      // materialization-triggered delivery after our click.
      await sleep(100);
      return page.evaluate((name) => {
        const bh = window.__bh, h = bh.history(name), live = h.querySelector('.history-live');
        return { ...bh.historyState(name), folded: !!live.querySelector(':scope > details.run-fold:not([open])'),
          tailRows: live.querySelectorAll(':scope > .wb-entry').length,
          deliveries: bh.resizeDeliveries.filter((d) => d.after?.some((r) => r.name === name)).length };
      }, name);
    }
    const clickLog = (name) => page.evaluate((name) => {
      const bh = window.__bh, before = bh.historyState(name);
      bh.history(name).querySelector('.history-older').click();
      return { before, after: bh.historyState(name) };
    }, name);
    const boundedClick = (r) => {
      assert(r.before.pages - r.after.pages === 1 && r.after.rows === r.before.rows + 50,
        'closed-run click must materialize exactly one 50-row page synchronously');
      assert(r.after.steps === 2 && Math.abs(r.after.live - r.before.live) <= 1 && r.after.live < 1280,
        'closed-run merge must be size-neutral and leave the expanded clip underfilled');
    };

    await page.evaluate(() => { window.__bh.regressionPhase = 'neutral-chain'; });
    const initial = await settledLog('LOGCHAIN');
    measure('neutral-chain-initial', initial);
    assert(initial.rows === 50 && initial.pages >= 3 && initial.folded && initial.tailRows === 3 && initial.deliveries >= 1,
      'need settled, renderer-produced 47-folded/3-tail-row history and at least three older pages');
    await capture('before-neutral-chain');
    const click = await clickLog('LOGCHAIN');
    boundedClick(click);
    measure('neutral-chain-click', click);
    await page.waitForFunction(() => {
      const s = window.__bh.historyState('LOGCHAIN'); return s.pages === 0 || s.live >= s.steps * 640;
    }, null, { timeout: 3000 });
    await sleep(150);
    const drained = await page.evaluate(() => window.__bh.historyState('LOGCHAIN'));
    const chain = await page.evaluate(() => ({
      loads: window.__bh.historyLoads.filter((r) => r.name === 'LOGCHAIN'),
      deliveries: window.__bh.resizeDeliveries.filter((d) => d.before.some((r) => r.name === 'LOGCHAIN'))
    }));
    measure('neutral-chain-drained', { state: drained, ...chain });
    assert(drained.pages === 0 && drained.rows === initial.rows + initial.pages * 50 && Math.abs(drained.live - initial.live) <= 1,
      'size-neutral folded chain did not drain all pages without changing height');
    assert(chain.loads.length === initial.pages && chain.loads[0].delivery === null &&
      chain.loads.slice(1).every((l) => l.delivery !== null) && new Set(chain.loads.slice(1).map((l) => l.delivery)).size === initial.pages - 1,
      'continuation must load one page per distinct actual native RO delivery, never a synchronous multi-page loop');
    for (const d of chain.deliveries) {
      const b = d.before.find((r) => r.name === 'LOGCHAIN'), a = d.after.find((r) => r.name === 'LOGCHAIN');
      assert(b.pages - a.pages <= 1, 'one native RO callback materialized multiple pages');
    }
    await capture('after-neutral-chain');

    // A longer real log stream leaves pages pending while native IO arms
    // the newly expanded subscription. No callbacks are called manually.
    await page.evaluate(() => { window.__bh.regressionPhase = 'cancellation'; });
    const cancelInitial = await settledLog('LOGCANCEL');
    assert(cancelInitial.pages >= 8 && cancelInitial.rows === 50, 'need enough older pages to cancel a running chain');
    const cancelClick = await clickLog('LOGCANCEL');
    boundedClick(cancelClick);
    await page.evaluate(() => {
      const bh = window.__bh; bh.cancelVisible = false;
      bh.cancelWatch = window.wbHistory.watch(bh.history('LOGCANCEL').closest('.topic'), {
        onChange: (r) => { if (r.visible) bh.cancelVisible = true; }
      });
    });
    await page.waitForFunction(() => window.__bh.cancelVisible, null, { timeout: 2000 });
    const exit = await page.evaluate(() => {
      const bh = window.__bh, topic = bh.history('LOGCANCEL').closest('.topic'), s = bh.scroller();
      const before = bh.historyState('LOGCANCEL');
      s.scrollTop += topic.getBoundingClientRect().bottom - bh.viewTop(s) + 40;
      return before;
    });
    assert(exit.pages > 0 && exit.steps === 2, 'cancellation fixture drained before its visible->out transition');
    await page.waitForFunction(() => window.__bh.history('LOGCANCEL')._wbHistorySteps === 1, null, { timeout: 2000 });
    const collapsed = await page.evaluate(() => window.__bh.historyState('LOGCANCEL'));
    await sleep(350);
    const canceled = await page.evaluate(() => {
      const bh = window.__bh; bh.cancelWatch.unwatch();
      return { ...bh.historyState('LOGCANCEL'), pending: !!bh.history('LOGCANCEL')._wbHistoryReobserve };
    });
    measure('neutral-chain-canceled', { initial: cancelInitial, click: cancelClick, exit, collapsed, settled: canceled });
    assert(collapsed.pages > 0 && canceled.pages === collapsed.pages && canceled.rows === collapsed.rows && canceled.steps === 1 && !canceled.pending,
      'collapsed history continued materializing older pages or left a pending reobserve');
    await capture('after-cancellation');
    const observerErrors = await page.evaluate(() => window.__bh.observerErrors);
    assert(observerErrors.length === 0, `native ResizeObserver loop errors: ${observerErrors.join(' | ')}`);
  } catch (e) {
    report.failure = e.message;
    throw e;
  } finally {
    report.errors = ctx.errors;
    report.native = await page.evaluate(() => ({ deliveries: window.__bh.resizeDeliveries,
      loads: window.__bh.historyLoads, observerErrors: window.__bh.observerErrors,
      observerErrorDetails: window.__bh.observerErrorDetails })).catch(() => null);
    fs.writeFileSync(path.join(dir, 'measurements.json'), JSON.stringify(report, null, 2));
    log(`history regression artifacts: ${dir}`);
  }
}

const SCENARIOS = {
  toolbar: scenarioToolbar,
  overview: scenarioOverview,
  'overview-motion': scenarioOverviewMotion,
  keep: scenarioKeep,
  observer: scenarioObserver,
  bands: scenarioBands,
  pill: scenarioPill,
  history: scenarioHistory,
  'history-cards': (ctx) => scenarioHistory({ ...ctx, cardsOnly: true }),
  'history-paint': (ctx) => scenarioHistory({ ...ctx, navigationOnly: true }),
  'history-regressions': scenarioHistoryRegressions,
};

// --- main ----------------------------------------------------------------------

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const name = args._[0];
  if (name === "serve") return serve(args);
  if (!name || !SCENARIOS[name] || !args.url || args.url === true) {
    console.error("usage: node scripts/board-headless.mjs serve [--topics N] [--pending N] [--history-entries N] [--mermaid] [--keep-tmp]");
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

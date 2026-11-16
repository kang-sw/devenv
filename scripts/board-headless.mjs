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
//   shared-controls  Focused topic-body/header polish, fold/maximum and
//             arrow-to-line raise controls; no pointer-drag verification.
//             Use serve --shared-actions with an existing exited terminal.
//   shared-actions  Whole-card actions and one-time topic-relative previews.
//             Use serve --shared-actions, wide (1800) and narrow (420).
//             --case persistence skips independent card maximum geometry.
//   toolbar   Compact floating toolbar geometry, full keyboard/hit ownership,
//             real Files explorer, overview, frames, swaps and popovers.
//             Terminal POST is browser-local; no shell process is started.
//             --face-width 266 constrains the production face to the approved
//             direct-fragment narrow equivalent (not reference typography).
//   recap-jitter  Content-sized, max-four-line recap/header geometry during pinned-column scroll
//             and browser-local region updates (serve --history-entries 60).
//   overview  Folded picker, anchors/restores, interactions, held swaps,
//             palette picks, storage/reload and reduced motion.
//   overview-motion  Per-rAF height/slot/ghost traces, concurrent clock,
//             reverse exit, exact live-rect landing and reduced motion.
//             Use 1900px as well as 1400/420: PIN_WIDE is 1600px.
//
// --case NAME selects an independent overview checkpoint (recap, anchors, picker,
// rail, actions, palette, controls, reload), or a motion checkpoint (enter-top, enter-middle,
// enter-bottom, pins). Without it every check runs; failures stop the run.
// Focused cases help diagnose a blocker without repeatedly hitting it.
// --baseline-ref REF classifies only an exactly measured equivalent Web
// containment failure (same viewport/face, horizontal bounds and menu layout)
// as known-baseline-failure. Requires --capture-dir; predicates remain false
// and a separate regression verdict is printed. No other failure is waived.
// web-containment runs just that identical production/baseline probe.
// keep --framed-middle selects a real T topic in history-heavy fixtures.
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
//   history   Panel-count history chunks, bounded paging, late-height anchoring,
//             deep links and out-of-view collapse on dashboard AND standalone
//             full file pages. Dashboard-only: expanded swaps, overlapping keep
//             sessions and the pinned column (use --width 1800).
//   panel-header  Compact panel grid at 1440/800/360/320 and 420/421px actual
//             content in a wide host. Uses `serve --history-entries 180`;
//             runs on both dashboard kinds and standalone. Browser-local long
//             titles/body probes never change server data. Checks natural
//             wrap/cache stability and native observer errors at each width;
//             --capture-dir retains measurements and responsive screenshots.
//   history-cards  Card/paging probes without navigation/control/live checks.
//   history-paint  Targeted deep-link post-paint/keep-lifetime reproduction.
//   history-regressions  Thirty-entry chunks through renderer-produced closed
//             system chips (short rows bind the line cap before 1000px),
//             collapse cancellation, and real Tab past sticky pills.
//             Use serve --topics 8 --history-entries 180 --shared-actions for
//             the original e320 / p156 / e519 regression identities.
//   history-unified  Real EN/KO renderer fixtures, whole-entry/header/rich
//             contracts, 754 pN folds/unread updates and both-reference captures.
//             Generate with WS_HISTORY_FIXTURE_DIR + TestHistoryCaptureFixtures;
//             serve without --history-entries (an empty panel inventory), then
//             pass --fixtures DIR. Standalone uses a disposable live HTML copy.
//             --before captures baseline evidence without a TASK3 verdict.
//   history-fit  Controlled whole-row-budget geometry (all/two-panel/one-panel/
//             oversized), cached/first heights, wrapping without recutting.
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
// --menu-hit-targets adds a post-scenario assertion for a bounded two-menu
// prefix: both native clicks must hit the button itself, with no shell cover.
// It adds no awaited inspection between the original scenario's actions.
//
// Adding a scenario: write `async function scenarioX(ctx)` using the shared
// helpers below (openBoard, waitQuiet, scrollToMiddle, growFrame,
// segments / trace, assertChanged; in the page, window.__bh: update,
// startSampler / mark / stopSampler, layout, scroller, keeping, writesOn),
// with ctx.check for an expectation the run goes on past and assert for a
// precondition, and add it to SCENARIOS.

import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import { createHash } from 'node:crypto';
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { scenarioHistoryUnified } from "./board-history-unified.mjs";
import { createToolbarOverview } from './board-headless/toolbar-overview.mjs';
import assetSource from '../agents-plugin-tool/internal/wsdashboard/testdata/asset-source.cjs';
import { INIT, IN_PAGE } from './board-headless/in-page.mjs';
import { scenarioKeep, scenarioObserver, scenarioBands, scenarioPill } from './board-headless/motion.mjs';
import { scenarioHistoryRegressions, scenarioPanelHeader, scenarioHistoryFit } from './board-headless/history.mjs';

// Scenario modules under board-headless/ import these back; this entry runs main() on load.
export { Failure, assert, assertChanged, growFrame, log, openBoard, qa, scrollToMiddle, segments, sleep, trace, waitQuiet, HISTORY_PAGE, LOGIC_ONLY };

const { readAssetSource } = assetSource;

const REPO = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const TOOL_DIR = path.join(REPO, "agents-plugin-tool");
const LOGIC_ONLY = process.env.WS_BOARD_ACCEPTANCE_LOGIC_ONLY === '1';

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
  if (process.env.PLAYWRIGHT_MODULE) dirs.push(process.env.PLAYWRIGHT_MODULE);
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
function seedSession(tmp, bin, label, n, historyEntries, mermaid, pending, sharedActions) {
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
    // Renderer-produced closed system-event chips exercise size-neutral page
    // joins: newest 47 chip rows + three visible tail rows; older pages hold
    // whole chips. Log lines stay visible rows, so the chain is alternating
    // topic-state events (each appends one system entry) between one opening
    // log line, which creates the topic, and the three tail lines.
    for (const [name, count] of [['LOGCHAIN', 200], ['LOGCANCEL', 600]]) {
      const states = Array.from({ length: count - 4 }, (_, i) => `<wb:state value="${i % 2 ? 'discussing' : 'waiting'}"/>`).join('');
      const tail = [1, 2, 3].map((i) => `<wb:log>Visible tail line ${i}</wb:log>`).join('');
      run(bin, ['call', 'whiteboard.update', JSON.stringify({
        session_key: fer.session_key, commands: `<wb:topic name="${name}"><wb:log>Opening history line</wb:log>${states}${tail}</wb:topic>`
      })], { cwd: proj, env });
    }
    // Keep topics on both sides, so collapse compensation cannot be masked
    // by the document's scroll-range clamp.
    run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key, commands: topicCommands(n + 1) })], { cwd: proj, env });
  }
  if (sharedActions) {
    fs.mkdirSync(path.join(proj, 'docs'), { recursive: true });
    fs.writeFileSync(path.join(proj, 'docs', 'a.md'), '# Shared preview fixture\n');
    const tickets = path.join(proj, 'ai-docs', 'tickets', 'ready');
    fs.mkdirSync(tickets, { recursive: true });
    fs.writeFileSync(path.join(tickets, '261114-feat-card-fixture.md'), '---\ntitle: Card fixture\n---\n\n# Card fixture\n');
    run(bin, ['call', 'whiteboard.update', JSON.stringify({ session_key: fer.session_key,
      commands: '<wb:topic name="SHARED" tickets="261114-feat-card-fixture"><wb:body><p>Shared action fixture <a href="docs/a.md">preview A</a></p></wb:body><wb:panel recap="Expanded panel"><p>Panel payload</p></wb:panel><wb:panel recap="Closed panel"><p>Second payload</p></wb:panel><wb:log>History fixture</wb:log><wb:add class="technical"><wb:title>Open question fixture</wb:title><wb:option>Yes</wb:option><wb:option>No</wb:option><wb:link>docs/a.md</wb:link></wb:add></wb:topic>'
    })], { cwd: proj, env });
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
    const adapter = seedSession(tmp, bin, "adapter", n, +(args["history-entries"] || 0), !!args.mermaid, +(args.pending || 0), !!args['shared-actions']);
    const board = seedSession(tmp, bin, "board", n, +(args["history-entries"] || 0), !!args.mermaid, +(args.pending || 0), !!args['shared-actions']);
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

// Browsers openBoard launched; main closes them on every exit path.
const BROWSERS = [];

// openBoard opens url in a fresh headless Chromium, waits for the board, its
// stream socket (dashboard only) and its body frames' first heights, shows the board column
// on an adapter page, and returns { browser, page, tab, kind }. url may be the
// iframe shell (/s/<rid>): page is then the session page it frames (a
// Playwright Frame, which fills the window, so its coordinates are the
// tab's), and tab is the browser tab, which owns the mouse.
async function openBoard(ctx) {
  let browser = ctx.browser;
  if (!browser) {
    const { chromium } = await loadPlaywright();
    try {
      browser = await chromium.launch({ headless: !ctx.args.headed });
    } catch (e) {
      console.error(`board-headless: Chromium failed to launch: ${e.message.split("\n")[0]}`);
      console.error("Install it with:  npx playwright@latest install chromium");
      process.exit(2);
    }
    BROWSERS.push(browser);
  }
  const tab = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height }, locale: ctx.language === 'ko' ? 'ko-KR' : undefined });
  if (ctx.language && /^https?:/.test(ctx.args.url)) await tab.context().addCookies([{ name: 'ws_dashboard_lang', value: ctx.language, url: new URL(ctx.args.url).origin }]);
  tab.on("pageerror", (e) => ctx.errors.push(String(e)));
  if (ctx.args['menu-hit-targets']) {
    ctx.menuHits = [];
    tab.on('console', (message) => {
      const text = message.text(), marker = '__bh-menu-hit-target__';
      if (message.type() === 'debug' && text.startsWith(marker)) ctx.menuHits.push(JSON.parse(text.slice(marker.length)));
    });
  }
  const consoleErrors = [];
  tab.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  await tab.addInitScript(INIT, { menuHitTargets: !!ctx.args['menu-hit-targets'] });
  if (ctx.legacyPanelFold) await tab.addInitScript((legacyKey) => {
    // Seed only the fresh top document, before production reads browser state.
    // Subsequent reloads keep the migrated record rather than reseeding it.
    if (window !== window.top || localStorage.getItem('ws.dashboard.panelshut') !== null) return;
    if (localStorage.length || sessionStorage.length) throw new Error('legacy-only fixture requires empty browser storage');
    localStorage.setItem('ws.dashboard.panelshut', JSON.stringify([legacyKey]));
  }, ctx.legacyPanelFold);
  if (ctx.assetRef) {
    const files = ['wsdashboard/assets/cards.js', 'wsdashboard/assets/dashboard.css', 'wsdashboard/assets/overview.js', 'wsdashboard/assets/terminal.js', 'wswhiteboard/assets/rail.js'];
    ctx.assetReplacements = []; ctx.assetHashes = {};
    await tab.route('**/*', async (route) => {
      if (!route.request().isNavigationRequest()) return route.continue();
      const response = await route.fetch(); let body = await response.text();
      for (const file of files) {
        const relative = 'agents-plugin-tool/internal/' + file;
        const current = readAssetSource(file.split('/')[0], path.basename(file));
        const baseline = run('git', ['show', ctx.assetRef + ':' + relative], { cwd: REPO });
        if (body.includes(current)) {
          body = body.replace(current, () => baseline); ctx.assetReplacements.push(relative);
          ctx.assetHashes[relative] = { candidate: createHash('sha256').update(current).digest('hex'), baseline: createHash('sha256').update(baseline).digest('hex') };
        }
      }
      await route.fulfill({ response, body });
    });
  }
  await tab.goto(ctx.args.url, { waitUntil: "load" });
  let page = tab;
  if (await tab.evaluate(() => document.documentElement.hasAttribute("data-shell"))) {
    page = await waitFor(async () => tab.frames().find((f) => f.parentFrame() === tab.mainFrame() && /[?&]frame=1(&|$)/.test(f.url())) || null, 10000, "the shell's session frame");
    await page.waitForLoadState("load");
  }
  await page.evaluate(IN_PAGE);
  await page.waitForFunction(() => window.__bh.board_()?.querySelector("section.topic[data-topic]"), null, { timeout: 10000 });
  const kind = await page.evaluate(() => window.__bh.kind());
  if (kind !== "standalone" && ctx.requireStream !== false) await page.waitForFunction(() => window.__bh.sockets.some((s) => s.readyState === 1 && typeof s.onmessage === "function"), null, { timeout: 10000 }).catch(async (e) => {
    throw new Failure('stream setup: ' + JSON.stringify({ error: e.message, pageErrors: ctx.errors, consoleErrors,
      sockets: await page.evaluate(() => __bh.sockets.map((s) => ({ url: s.url, state: s.readyState, handler: typeof s.onmessage }))) }));
  });
  if (kind === "adapter") {
    const shown = await page.evaluate(() => { const c = document.getElementById("bcol"); return !!c && c.getClientRects().length > 0; });
    if (!shown) await page.click('#tabbar [data-tab="board"]');
  }
  // Body frames size themselves once loaded; wait until each reported.
  if (ctx.requireFrameHeights !== false) await page.waitForFunction(() => window.__bh.frames().every((f) => f.style.height), null, { timeout: 15000 }).catch(() => {});
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

// Factories retain the existing shared helpers and browser registry; they
// define scenarios only. Command registration and browser lifetime stay here.
const { qa, scenarioSharedControls, scenarioSharedActions, scenarioToolbar,
  scenarioWebContainment, scenarioRecapJitter, scenarioOverviewFocus,
  scenarioOverviewFinal, scenarioOverview, scenarioOverviewMotion } = createToolbarOverview({
  assert, log, sleep, openBoard, waitQuiet, waitFor, assertChanged, IN_PAGE, loadPlaywright, BROWSERS, run, REPO
});

// History probes use the real host's wbHistory, event delegation, DOM and
// styles. Only payload height arrival and stream updates are synthetic.
const HISTORY_PAGE = () => {
  const bh = window.__bh;
  bh.samplePostPaint = true;
  bh.history = (name = 'HISTORY') => Array.from(document.querySelectorAll('.wb-history')).find((h) => h.dataset.history === name);
  bh.historyState = (name = 'HISTORY') => {
    const h = bh.history(name), live = h.querySelector('.history-live'), clip = h.querySelector('.history-clip');
    const r = clip.getBoundingClientRect();
    const panels = Array.from(live.querySelectorAll(':scope > .wb-entry.panel'));
    const entries = Array.from(live.querySelectorAll('.wb-entry:not(.historical)'));
    return { includedRows: entries.length - entries.findIndex((e) => e.id === h._wbHistoryChunks.at(-1)), steps: h._wbHistorySteps, chunks: h._wbHistoryChunks.slice(), pending: !!h._wbHistoryPending,
      visiblePanels: panels.filter((p) => p.getBoundingClientRect().height > 0).length,
      excludedRows: live.querySelectorAll(':scope > [data-wb-history-before]').length,
      whole: Array.from(live.children).filter((n) => n.getBoundingClientRect().height > 0).every((n) => {
        const b = n.getBoundingClientRect(); return b.top >= r.top - 1 && b.bottom <= r.bottom + 1;
      }),
      fade: getComputedStyle(clip, '::after').content,
      pages: h.querySelectorAll('template[data-history-page]').length,
      rows: live.querySelectorAll('.wb-entry').length, live: live.scrollHeight, height: r.height,
      top: r.top, bottom: r.bottom, scrollTop: clip.scrollTop,
      maxHeight: getComputedStyle(clip).maxHeight, overflow: getComputedStyle(h).overflowY,
      button: !h.querySelector('.history-older').hidden,
      latest: Array.from(live.children).filter((n) => n.matches('.wb-entry')).at(-1)?.id,
      folds: Array.from(live.querySelectorAll('details[data-key]')).map((d) => [d.dataset.key, d.open]) };
  };
  bh.arrivalProbe = (id) => {
    const h = bh.history(), topic = h.closest('.topic'), head = topic.querySelector('.tophead'), s = bh.scroller();
    const node = id && document.getElementById(id), control = node && (node.querySelector('summary') || node);
    const rect = (n) => n && n.getBoundingClientRect().toJSON();
    const cr = rect(control), x = cr && (cr.left + cr.right) / 2, y = cr && (cr.top + cr.bottom) / 2;
    let owner = cr && document.elementFromPoint(x, y), ancestry = [];
    for (let p = owner; p && ancestry.length < 7; p = p.parentElement) ancestry.push({ tag: p.tagName, id: p.id, cls: p.className });
    const hs = getComputedStyle(head), keep = bh.keeping(), column = document.querySelector('#bcol');
    return { id, target: rect(node), control: cr, point: { x, y }, ancestry,
      head: { rect: rect(head), position: hs.position, styleTop: hs.top, railTop: hs.getPropertyValue('--wb-rail-top'), height: head.offsetHeight },
      computedNavigateOffset: 12 + (hs.position === 'sticky' ? (parseFloat(hs.top) || 0) + head.offsetHeight : 0),
      face: rect(document.querySelector('.board-toolbar-face')), toolbar: rect(document.querySelector('.board-toolbar')),
      floating: document.querySelector('.board-toolbar')?.hasAttribute('data-floating'),
      railInset: column ? getComputedStyle(column).getPropertyValue('--wb-rail-inset') : null, padding: getComputedStyle(s).scrollPaddingTop,
      capsules: [...document.querySelectorAll('.wb-cap')].filter((n) => !n.hidden && n.getClientRects().length).map((n) => ({ cls: n.className, rect: rect(n) })),
      keeping: !!keep, targetOffsetObserved: node && node.getBoundingClientRect().top - bh.viewTop(s), scrollTop: s.scrollTop };
  };
  // The clip lands 100px below the view top, but never so low that HISTORY's
  // own top drops into view: a sliver of the preceding topic left in view
  // would anchor the view, so its growth (and HISTORY's) could push HISTORY
  // out. The clip's offset within its topic follows the history header.
  bh.positionHistory = () => {
    const s = bh.scroller(), h = bh.history(), r = h.querySelector('.history-clip').getBoundingClientRect();
    const head = r.top - h.closest('.topic').getBoundingClientRect().top;
    s.scrollTop += r.top - bh.viewTop(s) - Math.min(100, head);
  };
};

async function growEntry(page, id, px) {
  await page.evaluate(([id, px]) => window.__bh.mark(`late payload height (${id} +${px}px)`), [id, px]);
  const excluded = await page.evaluate(([id, px]) => {
    const entry = document.getElementById(id);
    if (!entry?.closest('[data-wb-history-before]')) return false;
    const body = entry.querySelector('.entry-body');
    if (!body || entry.getBoundingClientRect().height !== 0) throw new Error('excluded whole-entry growth fixture is not actually hidden');
    body.style.paddingBottom = (parseFloat(getComputedStyle(body).paddingBottom) + px) + 'px';
    if (entry.getBoundingClientRect().height !== 0) throw new Error('excluded payload growth leaked a partial entry');
    return true;
  }, [id, px]);
  if (excluded) return; // no rendered height can arrive from a display:none prefix
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

function arrivalChecks(v) {
  return { clip: v.y >= v.clipTop - 1 && v.y < v.clipBottom, viewport: v.y >= v.viewTop && v.y < v.viewBottom,
    face: v.y >= v.faceBottom && v.controlTop >= v.faceBottom, control: v.controlBottom <= v.viewBottom,
    ownership: v.controlOwnsPoint, innerScroll: v.clipScrollTop === 0 };
}
async function scenarioHistory(ctx) {
  const dir = path.resolve(ctx.args.artifacts || path.join(os.tmpdir(), `board-history-${Date.now()}`));
  let arrivalBaseline = null;
  if (ctx.navigationOnly && ctx.args['baseline-url']) {
    const artifacts = path.join(dir, 'exact-baseline'); fs.mkdirSync(artifacts, { recursive: true });
    const argv = [fileURLToPath(import.meta.url), 'history-paint', '--url', ctx.args['baseline-url'], '--width', String(ctx.width), '--height', String(ctx.height),
      '--case', 'arrival', '--verify-ref', ctx.args['baseline-ref'], '--artifacts', artifacts];
    assert(ctx.args['baseline-ref'], 'history --baseline-url requires an explicit --baseline-ref');
    if (ctx.args['face-width']) argv.push('--face-width', String(ctx.args['face-width']));
    const result = spawnSync(process.execPath, argv, { encoding: 'utf8', timeout: 60000 });
    fs.writeFileSync(path.join(artifacts, 'command-output.log'), (result.stdout || '') + (result.stderr || ''));
    assert(result.status === 0 || result.status === 1, 'baseline history probe failed to execute');
    arrivalBaseline = JSON.parse(fs.readFileSync(path.join(artifacts, 'measurements.json'), 'utf8'));
    arrivalBaseline.command = [process.execPath, ...argv]; arrivalBaseline.exitCode = result.status;
  }
  const opened = await openBoard(ctx), { page, tab, kind } = opened;
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
  const capture = async (name) => { if (LOGIC_ONLY) return; const p = path.join(dir, `${kind}-${name}.png`); await tab.screenshot({ path: p }); log(`capture: ${p}`); };
  async function cards() {
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    const first = await page.evaluate(() => window.__bh.historyState());
    measure('initial', first);
    assert(first.steps === 1 && !first.pending && first.visiblePanels >= 1 && first.visiblePanels <= 3 &&
      (first.height <= 1000 || first.visiblePanels === 1), `invalid first panel chunk: ${JSON.stringify(first)}`);
    assert(first.pages >= 2 && first.rows === 50, 'need 50 live entries and at least two inert older pages');
    assert(first.overflow !== 'auto' && first.overflow !== 'scroll' && first.scrollTop === 0 && first.maxHeight === 'none' && first.whole && (first.fade === 'none' || first.fade === 'normal'), 'history must show whole entries without an inner scroll, height clip or fade');
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
    assert(click.after.steps === 2 && click.after.chunks[0] === first.chunks[0] && click.after.height > first.height,
      'load-more did not preserve the first boundary and add an older chunk');
    assert(click.before.pages - click.after.pages <= 1, 'one click synchronously loaded more than one page');

    // Add successive panel chunks until an embedded page is needed. Record
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
      assert(r.after.chunks.slice(0, -1).join() === r.before.chunks.join(), 'expansion recut an existing chunk');
      assert(r.after.steps === r.before.steps + 1, 'clip did not step by one');
      await sleep(100);
      r.settled = await page.evaluate(() => window.__bh.historyState());
      assert(!r.settled.pending && r.settled.chunks.slice(0, -1).join() === r.before.chunks.join(), 'late layout recut a finalized chunk');
      if (r.after.pages < r.before.pages) break;
    }
    measure('paging', { clicks: paging.map((r) => ({ steps: r.after.steps, pagesBefore: r.before.pages, pagesSync: r.after.pages, pagesAfterResize: r.settled.pages, liveBefore: r.before.live })) });
    assert(paging.some((r) => r.after.pages < r.before.pages), 'no embedded page was materialized');

    // An excluded whole payload remains excluded through payload growth,
    // before and after keep's quiet window. Its rendered height is zero;
    // genuine visible inline/frame height arrivals are covered below.
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
    measure('below-collapse-start', belowCollapse);
    assert(belowCollapse.below && belowCollapse.before.steps === 2, 'below-collapse fixture was not expanded below the viewport');
    await page.waitForFunction(() => window.__bh.history()._wbHistorySteps === 1, null, { timeout: 3000 });
    await sleep(160);
    stable('below-viewport-collapse', await page.evaluate(() => window.__bh.stopSampler()), [prior]);
    const belowAfter = await page.evaluate(() => window.__bh.historyState());
    assert(belowAfter.rows === belowCollapse.before.rows && JSON.stringify(belowAfter.folds) === JSON.stringify(belowCollapse.before.folds),
      'below collapse lost rows or disclosure choices');

    if (kind !== 'standalone') await pinnedSetup();
  }
  // Shared by the full history scenario and the comparable A/B arrival probe.
  // No TASK3-only whole-entry/count precheck belongs in this host setup.
  async function pinnedSetup() {
      assert(kind !== 'standalone' && ctx.width >= 1600, 'history dashboard pinned-column check requires --width 1800');
      await page.evaluate(() => {
        const bh = window.__bh, topic = bh.history().closest('.topic');
        // A single panel chunk may fit the column. Pin a tall peer as well,
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
  try {
    if (ctx.args['verify-ref']) {
      const html = await page.evaluate(async () => (await fetch(location.href)).text());
      report.sourceCoverage = {};
      for (const f of ['wsdashboard/assets/cards.js', 'wsdashboard/assets/dashboard.css', 'wsdashboard/assets/overview.js', 'wsdashboard/assets/terminal.js', 'wswhiteboard/assets/rail.js']) {
        const relative = 'agents-plugin-tool/internal/' + f, source = run('git', ['show', ctx.args['verify-ref'] + ':' + relative], { cwd: REPO });
        assert(html.includes(source), 'actual baseline daemon did not serve exact ref asset ' + relative);
        report.sourceCoverage[relative] = createHash('sha256').update(source).digest('hex');
      }
    }
    await page.evaluate(HISTORY_PAGE);
    if (ctx.args['face-width']) await page.evaluate((width) => {
      const bar = document.querySelector('.board-toolbar'); bar.style.width = width + 'px'; bar.style.maxWidth = '100%';
    }, +ctx.args['face-width']);
    assert(await page.evaluate(() => !!window.__bh.history()), 'seed HISTORY with serve --history-entries 180');
    if (ctx.pinnedArrivalOnly) await pinnedSetup();
    else if (ctx.navigationOnly) {
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
      bh.arrivalBefore = bh.arrivalProbe();
      topic.style.transform = 'translateX(10000px)';
      const a = document.createElement('a'); a.href = '#' + panel.id; a.textContent = 'history target'; topic.appendChild(a); a.click(); a.remove();
      bh.arrivalAfterClick = bh.arrivalProbe(panel.id);
      return panel.id;
    });
    await page.waitForFunction(() => { const bh = window.__bh, r = window.wbHistory.seen(bh.history().closest('.topic'), '0px'); return r && !r.visible; }, null, { timeout: 2000 });
    const zero = await page.evaluate(() => window.__bh.historyState());
    assert(zero.steps > 1, 'first observer zero collapsed the deep-linked card');
    measure('deep-link-initial-zero', { target, steps: zero.steps, clipScrollTop: zero.scrollTop,
      diagnostics: await page.evaluate((id) => ({ before: __bh.arrivalBefore, afterClick: __bh.arrivalAfterClick, firstIOZero: __bh.arrivalProbe(id) }), target) });
    await page.evaluate(() => { window.__bh.history().closest('.topic').style.transform = ''; });
    await page.waitForFunction(() => window.wbHistory.seen(window.__bh.history().closest('.topic'), '0px')?.visible, null, { timeout: 2000 });
    const visible = await page.evaluate((id) => {
      const node = document.getElementById(id), r = node.getBoundingClientRect(), clip = node.closest('.wb-history').querySelector('.history-clip').getBoundingClientRect(), bh = window.__bh;
      const face = document.querySelector('.board-toolbar-face')?.getBoundingClientRect();
      const control = node.querySelector('summary') || node, cr = control.getBoundingClientRect();
      const hit = document.elementFromPoint((cr.left + cr.right) / 2, (cr.top + cr.bottom) / 2);
      return { diagnostics: bh.arrivalProbe(id), y: r.top, faceBottom: face?.bottom || 0, controlTop: cr.top, controlBottom: cr.bottom, controlOwnsPoint: !!hit && (control === hit || control.contains(hit)),
        clipTop: clip.top, clipBottom: clip.bottom, viewTop: bh.viewTop(bh.scroller()), viewBottom: bh.viewBottom(bh.scroller()), clipScrollTop: node.closest('.history-clip').scrollTop };
    }, target);
    measure('deep-link-visible', visible);
    await capture('deep-link-visible');
    const checks = arrivalChecks(visible);
    try { assert(Object.values(checks).every(Boolean), 'deep-link target not revealed in the bottom-anchored clip'); }
    catch (failure) {
      if (!arrivalBaseline) throw failure;
      const b = arrivalBaseline.measurements.find((m) => m.label === 'deep-link-visible');
      const initial = report.measurements.find((m) => m.label === 'deep-link-initial-zero');
      const baseInitial = arrivalBaseline.measurements.find((m) => m.label === 'deep-link-initial-zero');
      const failed = (v) => Object.entries(arrivalChecks(v)).filter(([, ok]) => !ok).map(([k]) => k).join();
      const d = visible.diagnostics, bd = b?.diagnostics;
      const lag = (m) => m.diagnostics.firstIOZero.computedNavigateOffset - m.diagnostics.afterClick.computedNavigateOffset;
      const equivalent = b && bd && arrivalBaseline.kind === kind && JSON.stringify(arrivalBaseline.viewport) === JSON.stringify(report.viewport) &&
        Object.keys(arrivalBaseline.sourceCoverage || {}).length === 5 && failed(visible) === 'ownership' && failed(b) === 'ownership' &&
        JSON.stringify(d.ancestry.map((n) => [n.tag, n.cls])) === JSON.stringify(bd.ancestry.map((n) => [n.tag, n.cls])) &&
        Math.abs(d.control.width - bd.control.width) < .1 && Math.abs(d.control.height - bd.control.height) < .1 &&
        Math.abs((d.control.top - d.head.rect.top) - (bd.control.top - bd.head.rect.top)) < .1 &&
        Math.abs((d.targetOffsetObserved - bd.targetOffsetObserved) - (parseFloat(d.head.styleTop) - parseFloat(bd.head.styleTop))) < .1 &&
        Math.abs(lag(initial) - 44) < .1 && Math.abs(lag(baseInitial) - 44) < .1 &&
        Math.abs((parseFloat(d.head.styleTop) - parseFloat(bd.head.styleTop)) - ((parseFloat(d.railInset) || 0) - (parseFloat(bd.railInset) || 0))) < .1;
      const classification = { assertion: failure.message, candidateChecks: checks, baselineChecks: b && arrivalChecks(b), equivalent: !!equivalent,
        verdict: equivalent ? 'known-baseline-failure' : 'regression-or-unclassified', baseline: arrivalBaseline,
        attributionLimit: 'Only matching stale-capsule-offset ownership is classified; every other predicate remains required.' };
      fs.writeFileSync(path.join(dir, 'history-arrival-comparison.json'), JSON.stringify(classification, null, 2));
      measure('history-arrival-classification', { verdict: classification.verdict, candidateChecks: checks, equivalent: !!equivalent });
      if (!equivalent) throw failure;
      (ctx.knownBaselineFailures ||= []).push(classification);
    }
    if (ctx.args.case === 'arrival' || ctx.pinnedArrivalOnly) {
      report.omissions.push('Focused arrival case excludes subsequent late-height, collapse and live-arrival assertions.');
      return;
    }
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
    // history still needs its button; repeated clicks add older panel chunks
    // until all content fits, at which point the button must hide.
    await page.evaluate(() => window.__bh.positionHistory());
    await waitQuiet(page);
    const exhausted = await page.evaluate(() => window.__bh.historyState());
    assert(exhausted.pages === 0 && exhausted.button && exhausted.includedRows < exhausted.rows && exhausted.excludedRows > 0, 'exhausted pages hid the button while excluded whole entries remained');
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
    const exhaustedGeometry = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history(), live = h.querySelector('.history-live'), clip = h.querySelector('.history-clip');
      const boundary = document.getElementById(h._wbHistoryChunks.at(-1)), cr = clip.getBoundingClientRect(), lr = live.getBoundingClientRect(), br = boundary.getBoundingClientRect();
      return { boundary: boundary.id, first: live.querySelector('.wb-entry').id,
        boundaryTop: br.top, boundaryBottom: br.bottom, clipTop: cr.top, clipBottom: cr.bottom,
        liveBottom: lr.bottom, leadingMargin: br.top - lr.top, clipScrollTop: clip.scrollTop };
    });
    measure('exhausted-boundary', exhaustedGeometry);
    assert(fitted.pages === 0 && !fitted.button && fitted.includedRows === fitted.rows &&
      exhaustedGeometry.boundary === exhaustedGeometry.first && exhaustedGeometry.boundaryTop >= exhaustedGeometry.clipTop - 1 &&
      exhaustedGeometry.boundaryBottom <= exhaustedGeometry.clipBottom + 1 &&
      Math.abs(exhaustedGeometry.liveBottom - exhaustedGeometry.clipBottom) <= 1 && exhaustedGeometry.clipScrollTop === 0,
      'exhausted history did not expose every entry at the exact first-entry boundary');

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
      chip.removeAttribute('data-wb-history-before'); // a fresh server row has no private prefix marker
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

const SCENARIOS = {
  'shared-controls': scenarioSharedControls,
  'shared-actions': scenarioSharedActions,
  toolbar: scenarioToolbar,
  'panel-header': scenarioPanelHeader,
  'web-containment': scenarioWebContainment,
  overview: scenarioOverview,
  'recap-jitter': scenarioRecapJitter,
  'overview-focus': scenarioOverviewFocus,
  'overview-final': scenarioOverviewFinal,
  'overview-motion': scenarioOverviewMotion,
  keep: scenarioKeep,
  observer: scenarioObserver,
  bands: scenarioBands,
  pill: scenarioPill,
  history: scenarioHistory,
  'history-fit': scenarioHistoryFit,
  'history-cards': (ctx) => scenarioHistory({ ...ctx, cardsOnly: true }),
  'history-paint': (ctx) => scenarioHistory({ ...ctx, navigationOnly: true }),
  'history-pinned-arrival': (ctx) => scenarioHistory({ ...ctx, pinnedArrivalOnly: true }),
  'history-regressions': scenarioHistoryRegressions,
  'history-unified': (ctx) => scenarioHistoryUnified(ctx, { openBoard, waitQuiet, waitFor, bindPage: IN_PAGE, log, assert }),
};

// --- main ----------------------------------------------------------------------

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (LOGIC_ONLY) delete args['capture-dir'];
  const name = args._[0];
  if (name === "serve") return serve(args);
  if (!name || !SCENARIOS[name] || !args.url || args.url === true) {
    console.error("usage: node scripts/board-headless.mjs serve [--topics N] [--pending N] [--history-entries N] [--mermaid] [--keep-tmp]");
    console.error(`       node scripts/board-headless.mjs <${Object.keys(SCENARIOS).join("|")}> --url <session or full file page> [--width W] [--height H] [--headed] [--artifacts DIR]`);
    process.exit(2);
  }
  // check records a failed expectation and lets the scenario go on, so one
  // run reports every part; assert stops the scenario.
  const ctx = { args, language: args.lang, width: +(args.width || 1440), height: +(args.height || 900), errors: [], failures: [], knownBaselineFailures: [] };
  ctx.check = (cond, msg) => { if (!cond) { ctx.failures.push(msg); console.error(`FAILED: ${msg}`); } return !!cond; };
  try {
    const t0 = Date.now();
    await SCENARIOS[name](ctx);
    if (args['menu-hit-targets']) {
      const hits = ctx.menuHits;
      log('menu-hit-targets: ' + JSON.stringify(hits));
      assert(hits.length === 2 && hits.every((h) => h.trusted && h.first && h.localHitIsButton && (!h.framed || h.parentHitIsFrame === true)),
        'both native Preview controls must themselves own the hit point without shell interception');
    }
    if (ctx.errors.length) ctx.failures.push(`page errors: ${ctx.errors.join(" | ")}`);
    if (ctx.failures.length) throw new Failure(`${ctx.failures.length} check(s) failed:\n  - ${ctx.failures.join("\n  - ")}`);
    const verdict = ctx.skips?.length ? 'COMPLETED WITH ' + ctx.skips.length + ' SETUP SKIP(S)' :
      ctx.knownBaselineFailures?.length ? 'REGRESSION-PASS with ' + ctx.knownBaselineFailures.length + ' known-baseline-failure(s)' : 'PASS';
    log(`${verdict} ${name} (${((Date.now() - t0) / 1000).toFixed(1)}s)`);
  } catch (e) {
    console.error(`FAIL ${name}: ${e instanceof Failure ? e.message : e.stack || e}`);
    process.exitCode = 1;
  } finally {
    for (const b of BROWSERS) await b.close().catch(() => {});
  }
}

main();

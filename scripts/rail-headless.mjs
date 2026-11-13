#!/usr/bin/env node
// rail-headless.mjs: headless browser checks for the dashboard's session rail.
// A repo-local developer command beside dev.py; it is not shipped.
//
// Run from anywhere in this checkout:
//
//   node scripts/rail-headless.mjs                    every scenario
//   node scripts/rail-headless.mjs grip-reorder look  the named scenarios, in order
//   node scripts/rail-headless.mjs --list             the scenario names
//
// Options: --shots DIR (where the look and tree scenarios save their captures;
// default a fresh rail-headless-shots-* folder under the OS temp dir, printed),
// --keep (keep the fixture's temp dir), --headed (show the browser).
//
// It never touches the shared dashboard daemon or its cache. It builds ws-mcp
// from this checkout into a temp dir, seeds a fixture there (git repositories
// with a linked worktree, a plain folder, and fake session registrations:
// several workspaces and worktrees, live and ended rows, labeled ones, a
// hosted one waiting on a tool approval), and
// starts its own daemon (`ws-mcp dashboard --port 0`) with WS_CACHE_HOME,
// WS_CONFIG_HOME, and HOME pointed into that temp dir and the assets served
// live from this checkout (WS_MCP_DASHBOARD_DEV_SOURCE). It reads the port
// from that instance's endpoint file and, at exit, kills only the process it
// started.
//
// Playwright is resolved from the npx cache (~/.npm/_npx/*/node_modules/
// playwright); there is no package.json and no Playwright dependency. With
// none cached, run `npx playwright install chromium` once.
//
// Each scenario asserts and throws on failure; the run exits non-zero when
// any scenario failed. To add one, append { name, about, run } to SCENARIOS:
// run(t) gets the scenario context (makeContext below: a fresh browser
// context and page, the fixture, and the page helpers). The rail's order and
// stars are the daemon's (its rail layout), so each scenario starts from an
// empty layout (resetLayout).

import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(HERE, "..");
const TOOL = path.join(REPO, "agents-plugin-tool");
const MOCKUP = path.join(REPO, "ai-docs/tickets/assets/261113-feat-ws-dashboard-session-rail-tree/session-rail-branch-indent-mockup.html");

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
  console.error(`rail-headless: ${msg}`);
  process.exit(2);
}
function assert(ok, msg) {
  if (!ok) throw new Error(msg);
}
function near(got, want, what, tol = 0.6) {
  assert(Math.abs(got - want) <= tol, `${what} = ${got.toFixed(2)}px, want ${want}px`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function until(what, fn, ms = 8000) {
  const end = Date.now() + ms;
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(60);
  }
}

// ------------------------------------------------------------ reply-ids --
// replyID is wsmailbox.ReplyID: four words of the EFF list, each picked by
// the first 8 bytes (big-endian) of sha256("<key>#<i>") modulo the list, run
// together (wskey.DeriveFull without its hyphens).
const WORDS = fs.readFileSync(path.join(TOOL, "internal/wskey/eff_large_wordlist.txt"), "utf8")
  .split("\n").map((s) => s.trim()).filter(Boolean);
function replyID(key) {
  let out = "";
  for (let i = 0; i < 4; i++) {
    const d = createHash("sha256").update(`${key}#${i}`).digest();
    out += WORDS[Number(d.readBigUInt64BE(0) % BigInt(WORDS.length))];
  }
  return out;
}

// --------------------------------------------------------------- fixture --
// The seeded sessions. ws: the folder (alpha, beta, and gamma are
// repositories, alpha-feat alpha's linked worktree on feat/x, notes a plain
// folder); sub roots the session in a folder of it; ended folds the row;
// approval makes the row a hosted session whose present adapter reports
// its lead waiting on that tool approval (the tree scenario's roll-up).
const LONG = "A very long session label that keeps going well past the edge";
const SESSIONS = [
  ...Array.from({ length: 18 }, (_, i) => ({ key: `rail-a-${String(i + 1).padStart(2, "0")}`, ws: "alpha" })),
  { key: "rail-a-sub", ws: "alpha", sub: "sub" },
  { key: "rail-a-e1", ws: "alpha", ended: true },
  { key: "rail-a-e2", ws: "alpha", ended: true },
  { key: "rail-f-1", ws: "alpha-feat", harness: "codex" },
  { key: "rail-f-2", ws: "alpha-feat", harness: "pi" },
  { key: "rail-f-e1", ws: "alpha-feat", ended: true },
  { key: "rail-b-1", ws: "beta" },
  { key: "rail-b-2", ws: "beta" },
  { key: "rail-n-1", ws: "notes" },
  { key: "rail-g-main", ws: "gamma" },
  { key: "rail-g-exec", ws: "gamma", harness: "codex" },
  { key: "rail-g-ui", ws: "gamma", approval: { toolName: "Bash", argsPreview: "git push origin develop" } },
  { key: "rail-g-other", ws: "gamma" },
];
const LABELS = { "rail-a-02": "Build bot", "rail-a-05": LONG };

function git(cwd, ...args) {
  execFileSync("git", ["-c", "user.name=rail", "-c", "user.email=rail@example.invalid", "-c", "commit.gpgsign=false", ...args], { cwd, stdio: "pipe" });
}
function writeJSON(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, JSON.stringify(value, null, 2) + "\n");
}
function stamp(ms) { return new Date(ms).toISOString().replace(/\.\d{3}Z$/, "Z"); }

function seed(root) {
  const work = path.join(root, "work"), cache = path.join(root, "cache");
  const dirs = { alpha: path.join(work, "alpha"), "alpha-feat": path.join(work, "alpha-feat"), beta: path.join(work, "beta"), gamma: path.join(work, "gamma"), notes: path.join(work, "notes") };
  for (const name of ["alpha", "beta", "gamma"]) {
    fs.mkdirSync(dirs[name], { recursive: true });
    git(dirs[name], "init", "-q", "-b", "main");
    git(dirs[name], "commit", "--allow-empty", "-qm", "initial");
  }
  git(dirs.alpha, "worktree", "add", "-q", "-b", "feat/x", dirs["alpha-feat"]);
  fs.mkdirSync(path.join(dirs.alpha, "sub"), { recursive: true });
  fs.mkdirSync(dirs.notes, { recursive: true });
  const common = { alpha: path.join(dirs.alpha, ".git"), "alpha-feat": path.join(dirs.alpha, ".git"), beta: path.join(dirs.beta, ".git"), gamma: path.join(dirs.gamma, ".git"), notes: "" };
  const now = Date.now(), fx = { root, work, cache, dirs, rid: {}, project: {}, label: {}, live: [], ended: [] };
  SESSIONS.forEach((s, i) => {
    const rid = replyID(s.key), project = s.sub ? path.join(dirs[s.ws], s.sub) : dirs[s.ws];
    fx.rid[s.key] = rid;
    fx.project[s.key] = project;
    (s.ended ? fx.ended : fx.live).push(s.key);
    // The session record the dashboard's page source matches the
    // registration against: a top-level lead at the same root.
    writeJSON(path.join(cache, "sessions", s.key, "key.json"), { schema_version: 2, root: project, scope: "lead" });
    const dir = path.join(cache, "dashboard", "sessions", rid);
    const registered = now - (SESSIONS.length - i) * 60_000;
    const reg = { schema_version: 1, reply_id: rid, project, harness: s.harness || "claude", registered_at: stamp(registered) };
    if (common[s.ws]) reg.git_common_dir = common[s.ws];
    // A live row's heartbeat sits ahead of the clock, so the row stays live
    // however long the run takes, with no write (and no rail swap) to keep
    // it so.
    if (s.ended) reg.heartbeat_at = reg.ended_at = stamp(now - (40 + i) * 60_000);
    else reg.heartbeat_at = stamp(now + 60 * 60_000);
    if (s.approval) reg.hosted = true;
    writeJSON(path.join(dir, "registration.json"), reg);
    // A finished tool batch gives the row its last-tool time.
    const t = now - (i + 1) * 97_000;
    const snap = { schemaVersion: 1, replyId: rid, updatedAt: now, lead: { batches: [{ seq: 1, toolNames: ["Bash"], callCount: 1, failed: 0, startedAt: t - 2000, endedAt: t }] } };
    // A hosted row's approval wait is its lead's permission phase as a
    // present adapter reports it: a snapshot that names its adapter and,
    // stamped ahead of the clock like the heartbeat, never goes stale.
    if (s.approval) Object.assign(snap, { adapter: "claude", updatedAt: now + 60 * 60_000 }, { lead: { ...snap.lead, phase: "permission", permission: s.approval } });
    writeJSON(path.join(dir, "snapshot.json"), snap);
    if (LABELS[s.key]) {
      fx.label[s.key] = LABELS[s.key];
      writeJSON(path.join(dir, "label.json"), { label: LABELS[s.key] });
    }
  });
  return fx;
}

// ---------------------------------------------------------------- daemon --
let daemon = null;
function killDaemon() {
  if (daemon && daemon.exitCode === null && daemon.signalCode === null) {
    try { daemon.kill("SIGTERM"); } catch { /* already gone */ }
  }
}
process.on("exit", killDaemon);
for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => { killDaemon(); process.exit(130); });

async function startDaemon(root, fx) {
  const bin = path.join(root, "bin", process.platform === "win32" ? "ws-mcp.exe" : "ws-mcp");
  console.log("building ws-mcp from this checkout ...");
  execFileSync("go", ["build", "-o", bin, "./cmd/ws-mcp"], { cwd: TOOL, stdio: "inherit" });
  const env = {};
  for (const [k, v] of Object.entries(process.env)) {
    // Nothing of the caller's ws session or XDG dirs reaches the instance.
    if (!k.startsWith("WS_") && !k.startsWith("XDG_")) env[k] = v;
  }
  Object.assign(env, {
    HOME: path.join(root, "home"),
    WS_CACHE_HOME: fx.cache,
    WS_CONFIG_HOME: path.join(root, "config"),
    WS_MCP_DASHBOARD_DEV_SOURCE: TOOL,
  });
  fs.mkdirSync(env.HOME, { recursive: true });
  fs.mkdirSync(env.WS_CONFIG_HOME, { recursive: true });
  const log = fs.openSync(path.join(root, "daemon.log"), "a");
  daemon = spawn(bin, ["dashboard", "--port", "0"], { cwd: root, env, stdio: ["ignore", log, log] });
  daemon.on("exit", (code, sig) => { if (!stopping) console.error(`rail-headless: the daemon exited early (${code ?? sig}); see ${path.join(root, "daemon.log")}`); });
  const endpoint = path.join(fx.cache, "dashboard", "endpoint.json");
  const ep = await until("the daemon's endpoint file", () => {
    try {
      const v = JSON.parse(fs.readFileSync(endpoint, "utf8"));
      return v.pid === daemon.pid && v.port ? v : null;
    } catch { return null; }
  }, 20000);
  const base = `http://127.0.0.1:${ep.port}`;
  await until("the daemon's health", async () => { try { return (await fetch(base + "/health")).ok; } catch { return false; } }, 20000);
  return base;
}
// layoutOp posts one rail layout op as a dashboard page would (with the
// daemon's own Origin) and answers the HTTP status and the reply.
async function layoutOp(base, body) {
  const r = await fetch(base + "/rail/layout", { method: "POST", headers: { "Content-Type": "application/json", Origin: base }, body: JSON.stringify(body) });
  return { status: r.status, reply: await r.json().catch(() => ({})) };
}
// resetLayout empties the daemon's rail layout: its file goes, then a star
// and an unstar through the route (each one a wake) make the daemon render
// the rail again, so no page is sent a cached rail with the last scenario's
// order or stars. With no file there is nothing to reset.
async function resetLayout(base, fx) {
  const file = path.join(fx.cache, "dashboard", "rail-layout.json");
  if (!fs.existsSync(file)) return;
  fs.rmSync(file);
  for (const op of ["star", "unstar"]) {
    const a = await layoutOp(base, { op, rid: fx.rid[fx.live[0]] });
    assert(a.status === 200 && a.reply.applied === true, `resetting the layout (${op}): HTTP ${a.status} ${JSON.stringify(a.reply)}`);
  }
}
let stopping = false;
async function stopDaemon() {
  stopping = true;
  if (!daemon || daemon.exitCode !== null || daemon.signalCode !== null) return;
  const gone = new Promise((r) => daemon.once("exit", r));
  daemon.kill("SIGTERM");
  if (await Promise.race([gone.then(() => true), sleep(8000).then(() => false)])) return;
  daemon.kill("SIGKILL");
  await gone;
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
// LEGACY are the localStorage keys a browser kept the rail's order and stars
// under before the daemon kept them; a page imports them once.
const LEGACY = { order: "ws.dashboard.railorder", stars: "ws.dashboard.stars", wsorder: "ws.dashboard.wsorder" };

// makeContext opens a fresh browser context (its own storage, so no draft or
// collapsed workspace leaks between scenarios) and gives the scenario its
// helpers.
async function makeContext(browser, base, fx, o = {}) {
  const context = await browser.newContext({
    viewport: o.viewport || { width: 1700, height: 820 },
    locale: "en-US",
    colorScheme: o.colorScheme || "light",
    reducedMotion: "reduce",
    permissions: ["clipboard-read", "clipboard-write"],
  });
  const page = await context.newPage();
  const t = {
    page, context, base, fx,
    rid: (key) => fx.rid[key],
    sel: (key) => `#rlist .rs[data-rid="${fx.rid[key]}"]`,
    // open shows the session key's page with its rail settled: every seeded
    // row present, the worktree list read (the linked worktree's branch
    // shown), and the viewed row marked.
    async open(key) {
      await page.goto(`${base}/s/${fx.rid[key]}`);
      await until("the rail to settle", () => page.evaluate(([n, live]) => {
        const rows = document.querySelectorAll("#rlist .rs");
        const br = [...document.querySelectorAll("#rlist .wthd .br")].map((b) => b.textContent);
        return rows.length === n && document.querySelectorAll("#rlist .rs:not(.ended)").length === live && br.includes("feat/x") && !!document.querySelector("#rlist .rs.cur");
      }, [SESSIONS.length, fx.live.length]), 15000);
      await page.mouse.move(o.viewport ? o.viewport.width - 5 : 1695, 5);
    },
    railHash: () => page.evaluate(() => document.querySelector('[data-region="rail"]').getAttribute("data-h")),
    railNames: () => page.evaluate(() => [...document.querySelectorAll("#rlist .rs .rid")].map((e) => e.textContent)),
    // relabel posts a label for the session key through the label route,
    // which re-renders the rail: a real rail swap on every open page.
    async relabel(key, label) {
      const status = await page.evaluate(async ([rid, label]) => (await fetch(`/s/${rid}/label`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ label }) })).status, [fx.rid[key], label]);
      assert(status === 200, `relabel ${key}: HTTP ${status}`);
    },
    // group is the live reply-ids of the list the session key's row is in,
    // as shown.
    group: (key) => page.evaluate((rid) => {
      const r = document.querySelector(`#rlist .rs[data-rid="${rid}"]`);
      return [...r.parentNode.children].filter((c) => c.classList.contains("rs") && !c.classList.contains("ended")).map((c) => c.getAttribute("data-rid"));
    }, fx.rid[key]),
    keyOf: (rid) => Object.keys(fx.rid).find((k) => fx.rid[k] === rid),
    // wsOrder is the workspace sections' paths, as shown.
    wsOrder: () => page.evaluate(() => [...document.querySelectorAll("#rlist .rws")].map((w) => w.getAttribute("data-ws"))),
    // starred reports the session key's star shown pressed.
    starred: (key) => page.evaluate((rid) => {
      const b = document.querySelector(`#rlist .rs[data-rid="${rid}"] .star`);
      return !!b && b.getAttribute("aria-pressed") === "true";
    }, fx.rid[key]),
    // legacy is the retired local layout keys' values (null: not set).
    legacy: () => page.evaluate((keys) => keys.map((k) => localStorage.getItem(k)), Object.values(LEGACY)),
    // seedLegacy has the context's first page load find the retired local
    // layout (v: { order, stars, wsorder }, id lists) in localStorage, as a
    // browser that used the rail before the daemon kept its layout would.
    async seedLegacy(v) {
      const items = Object.entries(LEGACY).map(([name, k]) => [k, JSON.stringify(v[name] || [])]);
      await context.addInitScript((items) => {
        try {
          if (location.protocol !== "http:" || sessionStorage.getItem("rail-headless-seeded")) return;
          sessionStorage.setItem("rail-headless-seeded", "1");
          for (const [k, v] of items) localStorage.setItem(k, v);
        } catch { /* no storage here */ }
      }, items);
    },
    // barAt scrolls the rail list so the workspace's bar stands 100px below
    // the list's top (clear of the drag's edge scroll) and answers the bar's
    // box.
    async barAt(ws) {
      await page.evaluate((ws) => {
        const l = document.getElementById("rlist"), w = [...l.querySelectorAll(".rws")].find((s) => s.getAttribute("data-ws") === ws);
        l.scrollTop += w.querySelector(".wsbar").getBoundingClientRect().top - l.getBoundingClientRect().top - 100;
      }, ws);
      return t.box(`#rlist .rws[data-ws="${ws}"] > .wsbar`);
    },
    // pressBar puts the mouse down on the workspace's bar, on its name.
    async pressBar(ws) {
      const b = await t.box(`#rlist .rws[data-ws="${ws}"] > .wsbar .nm`);
      const at = { x: b.x + Math.min(10, b.width / 2), y: b.y + b.height / 2 };
      await page.mouse.move(at.x, at.y);
      await page.mouse.down();
      return at;
    },
    box: async (selector) => {
      const b = await page.locator(selector).first().boundingBox();
      assert(b, `${selector} has no box`);
      return b;
    },
    // press puts the mouse down on the grip of the session key's row.
    async press(key) {
      await page.locator(t.sel(key)).scrollIntoViewIfNeeded();
      const b = await t.box(`${t.sel(key)} .drgrip`);
      const at = { x: b.x + b.width / 2, y: b.y + 11 };
      await page.mouse.move(at.x, at.y);
      await page.mouse.down();
      return at;
    },
    dragging: () => page.evaluate(() => document.documentElement.classList.contains("rdragging")),
    line: () => page.evaluate(() => {
      const l = document.querySelector(".rdrop");
      if (!l || l.hidden) return null;
      const r = l.getBoundingClientRect();
      return { top: r.top, left: r.left, width: r.width };
    }),
    hint: () => page.evaluate(() => {
      const h = document.querySelector(".drhint");
      return h && !h.hidden ? h.textContent : "";
    }),
    listBox: () => page.evaluate(() => {
      const l = document.getElementById("rlist"), r = l.getBoundingClientRect();
      return { top: r.top, bottom: r.bottom, left: r.left, scrollTop: l.scrollTop, scroll: l.scrollHeight - l.clientHeight };
    }),
    setScroll: (top) => page.evaluate((top) => { document.getElementById("rlist").scrollTop = top; }, top),
    style: (selector, prop) => page.evaluate(([s, p]) => getComputedStyle(document.querySelector(s))[p], [selector, prop]),
    clip: () => page.evaluate(() => navigator.clipboard.readText()),
  };
  // swapWait is how long a scenario waits to be sure a swap the daemon sent
  // would have landed by now: a few times the measured round trip.
  t.swapWait = async () => {
    if (!fx.swapMs) {
      const h0 = await t.railHash(), start = Date.now();
      await t.relabel("rail-b-2", `probe ${start}`);
      await until("a probe swap", async () => (await t.railHash()) !== h0, 10000);
      fx.swapMs = Math.max(Date.now() - start, 50);
      console.log(`  (a label's rail swap lands in ${fx.swapMs}ms)`);
    }
    return Math.max(1500, fx.swapMs * 4);
  };
  return t;
}

// ------------------------------------------------------------- scenarios --
const SCENARIOS = [
  {
    name: "offsets",
    about: "workspace / branch / harness icons at 12 / 28 / 49 px, the guide line and the grip column between them",
    async run(t) {
      await t.open("rail-f-1");
      const m = await t.page.evaluate(() => {
        const L = document.getElementById("rlist").getBoundingClientRect().left;
        const left = (s) => document.querySelector(s).getBoundingClientRect().left - L;
        const g = document.querySelector("#rlist .rwt .rsessions .rs:not(.ended) .drgrip").getBoundingClientRect();
        return { ws: left("#rlist .wsbar .wsi"), br: left("#rlist .wthd > svg"), hi: left("#rlist .rs:not(.ended) .hi"), endedHi: left("#rlist .rended .rs .hi"), guide: left("#rlist .rwt > .rsessions"), gripL: g.left - L, gripW: g.width };
      });
      near(m.ws, 12, "workspace icon");
      near(m.br, 28, "branch icon");
      near(m.hi, 49, "harness icon");
      near(m.endedHi, 49, "an ended row's harness icon");
      near(m.guide, 33, "guide line");
      near(m.gripL, 35, "grip column");
      near(m.gripW, 14, "grip width");
    },
  },
  {
    name: "star",
    about: "the star follows the name, hidden until hover or focus, always on when starred; a click stars without navigating, and the daemon's render pins the row; a long name ellipsizes before it",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      const row = t.sel("rail-a-03"), url = page.url();
      const shape = await page.evaluate((s) => {
        const r = document.querySelector(s), star = r.querySelector(".star"), name = r.querySelector(".rid");
        return { prev: star.previousElementSibling.className, inLink: !!star.closest("a"), gap: star.getBoundingClientRect().left - name.getBoundingClientRect().right, w: star.getBoundingClientRect().width };
      }, row);
      assert(shape.prev === "rl" && !shape.inLink, `the star must follow the name's link, outside it (after .${shape.prev})`);
      assert(shape.gap >= 0 && shape.gap < 12, `the star stands ${shape.gap}px after the name`);
      near(shape.w, 16, "a hidden star's width");
      assert(await t.style(`${row} .star`, "opacity") === "0", "an unstarred star shows before hover");
      await page.hover(`${row} .rid`);
      await sleep(250);
      assert(+(await t.style(`${row} .star`, "opacity")) > 0, "hover does not show the star");
      await page.mouse.move(1695, 5);
      await page.focus(`${row} .rl`);
      await sleep(250);
      assert(+(await t.style(`${row} .star`, "opacity")) > 0, "focus does not show the star");
      await page.evaluate(() => document.activeElement.blur());
      await page.click(`${row} .star`);
      await sleep(300);
      assert(page.url() === url, `a star click navigated to ${page.url()}`);
      assert(await page.getAttribute(`${row} .star`, "aria-pressed") === "true", "the click did not star the row");
      await page.mouse.move(1695, 5);
      await sleep(250);
      assert(await t.style(`${row} .star`, "opacity") === "1", "a starred row hides its star");
      // The star shows pressed at once; the row moves with the daemon's
      // render after the op.
      await until("the starred row pinned first", async () => (await t.group("rail-a-03"))[0] === t.rid("rail-a-03"));
      assert(await t.starred("rail-a-03"), "the daemon's render does not show the star");
      const long = await page.evaluate((s) => {
        const r = document.querySelector(s), name = r.querySelector(".rid"), star = r.querySelector(".star");
        const n = name.getBoundingClientRect(), st = star.getBoundingClientRect(), rr = r.getBoundingClientRect();
        return { cut: name.scrollWidth > name.clientWidth + 1, ellipsis: getComputedStyle(name).textOverflow, starIn: st.left >= n.right - 0.5 && st.right <= rr.right };
      }, t.sel("rail-a-05"));
      assert(long.cut && long.ellipsis === "ellipsis", "the long name is not ellipsized");
      assert(long.starIn, "the long name's star left the row or overlaps the name");
    },
  },
  {
    name: "grip",
    about: "the grip: touch-action none, shown on hover, live rows only",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      const row = t.sel("rail-a-04");
      assert(await t.style(`${row} .drgrip`, "touchAction") === "none", "the grip's touch-action is not none");
      assert(await t.style(`${row} .drgrip`, "cursor") === "grab", "the grip's cursor is not grab");
      assert(await t.style(`${row} .drgrip`, "userSelect") === "none", "the grip selects text");
      assert(await t.style(`${row} .drgrip`, "opacity") === "0", "the grip shows before hover");
      await page.hover(`${row} .rid`);
      await sleep(250);
      assert(+(await t.style(`${row} .drgrip`, "opacity")) > 0, "hover does not show the grip");
      assert(await page.locator("#rlist .rs.ended .drgrip").count() === 0, "an ended row has a grip");
    },
  },
  {
    name: "body-press",
    about: "a press on the row body never drags, and a click on it navigates",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      const before = await t.group("rail-a-04");
      const b = await t.box(`${t.sel("rail-a-04")} .rid`);
      await page.mouse.move(b.x + 10, b.y + b.height / 2);
      await page.mouse.down();
      await page.mouse.move(b.x + 10, b.y + 80, { steps: 8 });
      assert(!(await t.dragging()) && !(await t.line()), "a press on the row body started a drag");
      await page.mouse.up();
      assert(JSON.stringify(await t.group("rail-a-04")) === JSON.stringify(before), "a body press moved a row");
      await Promise.all([page.waitForURL(`**/s/${t.rid("rail-a-04")}`), page.click(`${t.sel("rail-a-04")} .rid`)]);
      // The rest of the line past the name and star navigates too: a short
      // name leaves room there left of the hover's pencil and clipboard.
      await t.open("rail-f-1");
      const tail = await t.box(`${t.sel("rail-a-02")} .drtail`);
      assert(tail.width > 60, `the short name's tail is ${tail.width}px wide`);
      await page.hover(`${t.sel("rail-a-02")} .rid`);
      const hit = await page.evaluate(([x, y]) => !!document.elementFromPoint(x, y).closest(".drtail"), [tail.x + 6, tail.y + tail.height / 2]);
      assert(hit, "the tail's left end is covered");
      await Promise.all([page.waitForURL(`**/s/${t.rid("rail-a-02")}`), page.mouse.click(tail.x + 6, tail.y + tail.height / 2)]);
    },
  },
  {
    name: "grip-reorder",
    about: "a grip drag reorders, with a rail swap injected mid-press held until the press ends; the order is the daemon's",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      const wait = await t.swapWait();
      const g = await t.group("rail-a-04"), x = t.keyOf(g[3]), first = t.keyOf(g[0]);
      const node = await page.$(t.sel(x));
      const at = await t.press(x);
      const h0 = await t.railHash();
      await t.relabel("rail-b-1", "swap during press");
      await sleep(wait);
      assert(await t.railHash() === h0, "a rail swap landed during the press");
      assert(await node.evaluate((n) => n.isConnected), "the pressed row was replaced");
      await page.mouse.move(at.x, at.y + 8, { steps: 2 });
      assert(await t.dragging(), "the grip press did not start a drag past the slop");
      const fb = await t.box(t.sel(first));
      await page.mouse.move(at.x, fb.y + 3, { steps: 10 });
      assert(await t.line(), "no drop line over the first row");
      await page.mouse.up();
      const after = await t.group(x);
      assert(after[0] === g[3] && after.length === g.length, `the drop did not put the row first: ${after.join(" ")}`);
      await until("the held swap to land", async () => (await t.railNames()).includes("swap during press"));
      assert((await t.group(x))[0] === g[3], "the held swap undid the drop");
      // The daemon keeps the order: nothing is stored locally, and a fresh
      // load shows it.
      assert((await t.legacy()).every((v) => v === null), "the drop stored a local order");
      await t.open("rail-f-1");
      const loaded = await t.group(x);
      assert(JSON.stringify(loaded) === JSON.stringify([g[3], ...g.filter((r) => r !== g[3])]), `a fresh load shows ${loaded.join(" ")}`);
    },
  },
  {
    name: "swap-after-drop",
    about: "a rail swap right after a drop keeps the dropped order",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      await t.swapWait();
      const g = await t.group("rail-a-04"), x = t.keyOf(g[1]), last = t.keyOf(g[5]);
      const at = await t.press(x);
      await page.mouse.move(at.x, at.y + 8, { steps: 2 });
      const lb = await t.box(t.sel(last));
      await page.mouse.move(at.x, lb.y + lb.height - 3, { steps: 10 });
      await page.mouse.up();
      await t.relabel("rail-b-1", "swap after drop");
      const want = g.slice(0, 1).concat(g.slice(2, 6), [g[1]], g.slice(6));
      assert(JSON.stringify(await t.group(x)) === JSON.stringify(want), `the drop order is ${(await t.group(x)).join(" ")}`);
      await until("the swap after the drop", async () => (await t.railNames()).includes("swap after drop"));
      assert(JSON.stringify(await t.group(x)) === JSON.stringify(want), "the swap after the drop re-ordered the rows");
    },
  },
  {
    name: "refused-hint",
    about: "a refused drop shows the not-allowed cursor and a hint saying why, and moves nothing",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      const g = await t.group("rail-a-04"), feat = await t.group("rail-f-1"), beta = await t.group("rail-b-1");
      const at = await t.press("rail-a-04");
      await page.mouse.move(at.x, at.y + 8, { steps: 2 });
      await page.locator(t.sel("rail-b-1")).scrollIntoViewIfNeeded().catch(() => {});
      let b = await t.box(`${t.sel("rail-b-1")} .rid`);
      await page.mouse.move(b.x + 20, b.y + b.height / 2, { steps: 10 });
      assert(await t.hint() === "Can't move to another workspace", `over another workspace the hint is "${await t.hint()}"`);
      assert(await page.evaluate(() => getComputedStyle(document.body).cursor) === "not-allowed", "the cursor does not say no");
      assert(!(await t.line()), "a refused place shows a drop line");
      // A live row of its own workspace is a parent the dragged row may
      // attach to by its middle (the tree scenario), so the other
      // worktree's and the starred row's refusals are read at a row's top
      // edge, a line before it.
      b = await t.box(`${t.sel("rail-f-2")} .rid`);
      await page.mouse.move(b.x + 20, (await t.box(t.sel("rail-f-2"))).y + 3, { steps: 10 });
      assert(await t.hint() === "Can't move to another worktree", `over another worktree the hint is "${await t.hint()}"`);
      await page.mouse.up();
      assert(!(await t.hint()) && !(await page.evaluate(() => document.documentElement.classList.contains("drno"))), "the hint outlived the drag");
      for (const [k, want] of [["rail-a-04", g], ["rail-f-1", feat], ["rail-b-1", beta]]) {
        assert(JSON.stringify(await t.group(k)) === JSON.stringify(want), `a refused drop moved rows in ${k}'s group`);
      }
      // Across the stars: the starred row is pinned first (once the
      // daemon's render lands), so both rows are in view from the top.
      await page.click(`${t.sel("rail-a-10")} .star`);
      await until("the starred row pinned first", async () => (await t.group("rail-a-10"))[0] === t.rid("rail-a-10"));
      await t.setScroll(0);
      const at2 = await t.press("rail-a-03");
      await page.mouse.move(at2.x, at2.y + 8, { steps: 2 });
      b = await t.box(`${t.sel("rail-a-10")} .rid`);
      await page.mouse.move(b.x + 20, (await t.box(t.sel("rail-a-10"))).y + 3, { steps: 10 });
      assert(await t.hint() === "Can't move across the starred sessions", `over the starred row the hint is "${await t.hint()}"`);
      await page.mouse.up();
      // A one-row group drags (to say why it cannot go elsewhere).
      const at3 = await t.press("rail-n-1");
      await page.mouse.move(at3.x, at3.y - 8, { steps: 2 });
      assert(await t.dragging(), "a group's only row does not drag");
      b = await t.box(`${t.sel("rail-b-2")} .rid`);
      await page.mouse.move(b.x + 20, b.y + b.height / 2, { steps: 10 });
      assert(await t.hint() === "Can't move to another workspace", `the lone row over another workspace: "${await t.hint()}"`);
      await page.mouse.up();
    },
  },
  {
    name: "layout-shared",
    about: "the layout is the daemon's: a reorder, a star, and a workspace reorder in one browser show in another, and a fresh load keeps them",
    async run(t, browser) {
      const { page } = t;
      const o = await makeContext(browser, t.base, t.fx);
      try {
        await t.open("rail-f-1");
        await o.open("rail-b-1");
        // A reorder: the fourth row to the top.
        const g = await t.group("rail-a-04"), x = t.keyOf(g[3]);
        const moved = [g[3], ...g.filter((r) => r !== g[3])];
        const at = await t.press(x);
        await page.mouse.move(at.x, at.y + 8, { steps: 2 });
        const fb = await t.box(t.sel(t.keyOf(g[0])));
        await page.mouse.move(at.x, fb.y + 3, { steps: 10 });
        await page.mouse.up();
        await until("the reorder in the other browser", async () => JSON.stringify(await o.group(x)) === JSON.stringify(moved));
        // A star: pinned first in both.
        const star = "rail-a-07";
        const pinned = [t.rid(star), ...moved.filter((r) => r !== t.rid(star))];
        await page.click(`${t.sel(star)} .star`);
        await until("the star in the other browser", async () => (await o.starred(star)) && JSON.stringify(await o.group(star)) === JSON.stringify(pinned));
        await until("the star in this browser", async () => JSON.stringify(await t.group(star)) === JSON.stringify(pinned));
        // A workspace reorder: a section before the small one above it (not
        // alpha's, whose bar would scroll out of view).
        const ws = await t.wsOrder();
        const i = ws.findIndex((p, j) => p !== t.fx.dirs.alpha && j + 1 < ws.length);
        assert(i >= 0, `no small workspace with one after it: ${ws.join(" ")}`);
        const above = ws[i], mover = ws[i + 1];
        const wantWS = [...ws.slice(0, i), mover, above, ...ws.slice(i + 2)];
        const ab = await t.barAt(above);
        const bat = await t.pressBar(mover);
        await page.mouse.move(bat.x, bat.y - 8, { steps: 2 });
        assert(await t.dragging(), "the workspace bar press did not start a drag");
        await page.mouse.move(bat.x, ab.y + 3, { steps: 10 });
        assert(await t.line(), "no drop line over the workspace above");
        await page.mouse.up();
        await until("the workspace order in the other browser", async () => JSON.stringify(await o.wsOrder()) === JSON.stringify(wantWS));
        await until("the workspace order in this browser", async () => JSON.stringify(await t.wsOrder()) === JSON.stringify(wantWS));
        // A fresh load keeps all three, and the layout is on disk.
        await o.open("rail-n-1");
        assert(JSON.stringify(await o.group(star)) === JSON.stringify(pinned), `a fresh load shows ${(await o.group(star)).join(" ")}`);
        assert(await o.starred(star), "a fresh load lost the star");
        assert(JSON.stringify(await o.wsOrder()) === JSON.stringify(wantWS), `a fresh load shows the workspaces ${(await o.wsOrder()).join(" ")}`);
        const file = fs.readFileSync(path.join(t.fx.cache, "dashboard", "rail-layout.json"), "utf8");
        assert(file.includes(t.rid(star)) && file.includes(mover), "the layout file does not hold the star and the workspace order");
        assert((await t.legacy()).every((v) => v === null) && (await o.legacy()).every((v) => v === null), "a layout change was stored locally");
      } finally {
        await o.context.close();
      }
    },
  },
  {
    name: "layout-import",
    about: "an empty layout imports a browser's retired local order, stars, and workspace order once, and the browser drops them",
    async run(t, browser) {
      await t.open("rail-f-1");
      const g = await t.group("rail-a-04"), ws = await t.wsOrder();
      const order = [...g].reverse(), stars = [t.rid("rail-a-05")], wsorder = [...ws].reverse();
      const a = await makeContext(browser, t.base, t.fx);
      try {
        const posts = [];
        a.page.on("request", (r) => { if (r.method() === "POST" && r.url().endsWith("/rail/layout")) posts.push(r.postDataJSON()); });
        await a.seedLegacy({ order, stars, wsorder });
        await a.open("rail-f-1");
        await until("the import to drop the local keys", async () => (await a.legacy()).every((v) => v === null));
        const want = [stars[0], ...order.filter((r) => r !== stars[0])];
        await until("the imported layout", async () => JSON.stringify(await a.group("rail-a-04")) === JSON.stringify(want) && JSON.stringify(await a.wsOrder()) === JSON.stringify(wsorder));
        assert(await a.starred("rail-a-05"), "the imported star does not show");
        assert(posts.length === 1 && posts[0].op === "import", `the page posted ${JSON.stringify(posts)}`);
        // The other browser, with no keys, shows the imported layout too.
        await until("the import in the other browser", async () => JSON.stringify(await t.group("rail-a-04")) === JSON.stringify(want) && JSON.stringify(await t.wsOrder()) === JSON.stringify(wsorder));
        // A reload imports nothing more.
        await a.open("rail-f-1");
        await sleep(await a.swapWait());
        assert(posts.length === 1, `a reload posted ${JSON.stringify(posts.slice(1))}`);
      } finally {
        await a.context.close();
      }
    },
  },
  {
    name: "layout-stale-keys",
    about: "a later browser with retired local keys over a layout the daemon already holds drops them unimported and shows the daemon's order",
    async run(t, browser) {
      await t.open("rail-f-1");
      const g = await t.group("rail-a-04"), ws = await t.wsOrder();
      // The daemon's layout: a star and a workspace order.
      for (const body of [{ op: "star", rid: t.rid("rail-a-06") }, { op: "wsmove", path: ws[ws.length - 1], before: ws[0] }]) {
        const a = await layoutOp(t.base, body);
        assert(a.status === 200 && a.reply.applied === true, `${body.op}: HTTP ${a.status} ${JSON.stringify(a.reply)}`);
      }
      const want = [t.rid("rail-a-06"), ...g.filter((r) => r !== t.rid("rail-a-06"))];
      const wantWS = [ws[ws.length - 1], ...ws.slice(0, -1)];
      const b = await makeContext(browser, t.base, t.fx);
      try {
        const posts = [];
        b.page.on("request", (r) => { if (r.method() === "POST" && r.url().endsWith("/rail/layout")) posts.push(r.postDataJSON()); });
        await b.seedLegacy({ order: [...g].reverse(), stars: [t.rid("rail-a-09")], wsorder: [...ws].reverse() });
        await b.open("rail-f-1");
        await until("the stale keys dropped", async () => (await b.legacy()).every((v) => v === null));
        assert(JSON.stringify(await b.group("rail-a-04")) === JSON.stringify(want), `the rail shows ${(await b.group("rail-a-04")).join(" ")}`);
        assert(JSON.stringify(await b.wsOrder()) === JSON.stringify(wantWS), `the rail shows the workspaces ${(await b.wsOrder()).join(" ")}`);
        assert(await b.starred("rail-a-06") && !(await b.starred("rail-a-09")), "the stars are not the daemon's");
        await sleep(await b.swapWait());
        assert(posts.length === 0, `the page posted ${JSON.stringify(posts)}`);
      } finally {
        await b.context.close();
      }
    },
  },
  {
    name: "layout-import-race",
    about: "two browsers importing into one empty layout at once: exactly one import applies, and both drop their keys",
    async run(t, browser) {
      await t.open("rail-f-1");
      const g = await t.group("rail-a-04"), ws = await t.wsOrder();
      const b = await makeContext(browser, t.base, t.fx);
      try {
        // Both imports are held until both have posted.
        const held = [];
        for (const c of [t, b]) await c.context.route("**/rail/layout", (route) => { held.push(route); });
        const mine = { order: [...g].reverse(), stars: [t.rid("rail-a-05")], wsorder: [...ws].reverse() };
        const theirs = { order: g, stars: [t.rid("rail-a-09")], wsorder: ws };
        await t.seedLegacy(mine);
        await b.seedLegacy(theirs);
        await t.open("rail-f-1");
        await b.open("rail-b-1");
        await until("both imports to post", () => held.length === 2);
        const answers = [t.page, b.page].map((p) => p.waitForResponse((r) => r.url().endsWith("/rail/layout")).then((r) => r.json()));
        await Promise.all(held.map((r) => r.continue()));
        const got = await Promise.all(answers);
        const applied = got.filter((v) => v.applied === true), other = got.filter((v) => v.applied === false && v.reason === "not_applied");
        assert(applied.length === 1 && other.length === 1, `the imports answered ${JSON.stringify(got)}`);
        for (const c of [t, b]) await until("the local keys dropped", async () => (await c.legacy()).every((v) => v === null));
        // Both show the one import that applied.
        const won = got[0].applied ? mine : theirs;
        const want = [won.stars[0], ...won.order.filter((r) => r !== won.stars[0])];
        for (const c of [t, b]) {
          await until("the applied import's layout", async () => JSON.stringify(await c.group("rail-a-04")) === JSON.stringify(want) && JSON.stringify(await c.wsOrder()) === JSON.stringify(won.wsorder));
        }
      } finally {
        await b.context.close();
      }
    },
  },
  {
    name: "clamp",
    about: "with the pointer past the list's edge the drop line shows clamped at the edge, and the list scrolls",
    contexts: [{ viewport: { width: 1700, height: 640 } }],
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      let lb = await t.listBox();
      assert(lb.scroll > 300, `the rail list scrolls only ${lb.scroll}px; the fixture needs more rows`);
      await t.setScroll(260);
      lb = await t.listBox();
      // The first alpha row well inside the view.
      const g = await t.group("rail-a-04");
      let key = null;
      for (const rid of g) {
        const b = await t.box(`#rlist .rs[data-rid="${rid}"]`);
        if (b.y > lb.top + 70 && b.y + b.height < lb.bottom - 70) { key = t.keyOf(rid); break; }
      }
      assert(key, "no alpha row inside the view");
      const at = await t.press(key);
      await page.mouse.move(at.x, at.y + 8, { steps: 2 });
      await page.mouse.move(at.x, lb.top - 14, { steps: 6 });
      const line = await t.line(), lb2 = await t.listBox();
      assert(line, "past the list's top edge the drop line is hidden");
      assert(line.top >= lb2.top - 0.5 && line.top <= lb2.top + 2.5, `the line stands at ${line.top}, want clamped at the list's top ${lb2.top}`);
      await sleep(400);
      assert((await t.listBox()).scrollTop < lb2.scrollTop, "past the edge the list does not scroll");
      await page.mouse.up();
    },
  },
  {
    name: "edge-scroll",
    about: "a pointer 40px inside the list's edge scrolls it (the zone is 48px); 60px inside does not",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      await t.setScroll(0);
      const lb = await t.listBox();
      assert(lb.scroll > 100, "the rail list does not scroll");
      const at = await t.press("rail-a-02");
      await page.mouse.move(at.x, at.y + 8, { steps: 2 });
      await page.mouse.move(at.x, lb.bottom - 60, { steps: 8 });
      await sleep(400);
      assert((await t.listBox()).scrollTop === 0, "60px inside the edge the list scrolled");
      await page.mouse.move(at.x, lb.bottom - 40, { steps: 3 });
      await sleep(400);
      assert((await t.listBox()).scrollTop > 0, "40px inside the edge the list did not scroll");
      await page.mouse.up();
    },
  },
  {
    name: "rename",
    about: "the pencil renames through /s/<rid>/label, a swap during the edit waits, Escape cancels, the viewed row's rename reaches the head",
    async run(t) {
      const { page } = t;
      await t.open("rail-f-1");
      const wait = await t.swapWait();
      const row = t.sel("rail-a-07"), rid = t.rid("rail-a-07");
      const nameAt = await t.box(`${row} .rid`);
      await page.hover(`${row} .rid`);
      await sleep(250);
      const hov = await t.box(`${row} .rid`);
      assert(hov.x === nameAt.x && hov.width === nameAt.width, "hover moved or resized the name");
      assert(await t.style(`${row} .dracts`, "display") === "flex", "hover does not show the pencil and clipboard");
      assert(await t.style(`${row} .drslot`, "visibility") === "hidden", "hover does not hide the time");
      const posts = [];
      page.on("request", (r) => { if (r.method() === "POST" && r.url().endsWith("/label")) posts.push(new URL(r.url()).pathname); });
      await page.click(`${row} .drren`);
      assert(await page.evaluate(() => document.activeElement && document.activeElement.classList.contains("drinp")), "the pencil did not open a focused input");
      assert(await page.getAttribute(`${row} .drinp`, "placeholder") === rid, "the input's placeholder is not the reply-id");
      await page.keyboard.type("Renamed row");
      const h0 = await t.railHash();
      await t.relabel("rail-b-1", "swap during edit");
      posts.length = 0;
      await sleep(wait);
      assert(await t.railHash() === h0, "a rail swap landed during the edit");
      assert(await page.inputValue(`${row} .drinp`) === "Renamed row", "the swap dropped the typing");
      assert(await page.evaluate(() => document.activeElement && document.activeElement.classList.contains("drinp")), "the swap took the focus");
      await page.keyboard.press("Enter");
      await until("the rename to show", async () => (await page.locator(`${row} .drinp`).count()) === 0 && (await page.textContent(`${row} .rid`)) === "Renamed row");
      assert(JSON.stringify(posts) === JSON.stringify([`/s/${rid}/label`]), `the rename posted to ${posts.join(", ")}`);
      assert((await page.getAttribute(`${row} .drcopy`, "data-copy")).endsWith(">Renamed row</mailbox>"), "the copy tag does not name the new label");
      await until("the held swap to land", async () => (await t.railNames()).includes("swap during edit"));
      assert(await page.textContent(`${row} .rid`) === "Renamed row", "the server's render lost the label");
      // Escape cancels.
      posts.length = 0;
      await page.hover(`${row} .rid`);
      await page.click(`${row} .drren`);
      await page.keyboard.type("nope");
      await page.keyboard.press("Escape");
      await sleep(300);
      assert(posts.length === 0 && (await page.locator(`${row} .drinp`).count()) === 0, "Escape did not cancel");
      assert(await page.textContent(`${row} .rid`) === "Renamed row", "Escape changed the name");
      // The viewed session: the head follows.
      const vrow = t.sel("rail-f-1");
      await page.hover(`${vrow} .rid`);
      await page.click(`${vrow} .drren`);
      await page.keyboard.type("Viewed renamed");
      await page.keyboard.press("Enter");
      await until("the head to follow", async () => (await page.textContent(".sname .snm")) === "Viewed renamed" && (await page.textContent(`${vrow} .rid`)) === "Viewed renamed");
      assert((await page.getAttribute(".sname .snm", "data-copy")).endsWith(">Viewed renamed</mailbox>"), "the head's copy tag does not name the new label");
    },
  },
  {
    name: "copy",
    about: "the clipboard copies the very string the head's name copies",
    async run(t) {
      const { page } = t;
      for (const key of ["rail-f-1", "rail-a-sub", "rail-a-02"]) {
        await t.open(key);
        const row = t.sel(key);
        // Both sides have read git: the head's badge and the rail's list.
        await until(`${key}'s tags to name the branch`, async () => (await page.getAttribute(".sname .snm", "data-copy") || "").includes(' branch="') && (await page.getAttribute(`${row} .drcopy`, "data-copy") || "").includes(' branch="'));
        await page.evaluate(() => navigator.clipboard.writeText(""));
        await page.click(".sname .snm");
        const head = await until("the head's copy", async () => (await t.clip()) || null);
        await page.evaluate(() => navigator.clipboard.writeText(""));
        await page.hover(`${row} .rid`);
        await page.click(`${row} .drcopy`);
        const rail = await until("the rail's copy", async () => (await t.clip()) || null);
        assert(rail === head, `${key}: the rail copies\n  ${rail}\nthe head copies\n  ${head}`);
        assert(head.startsWith(`<mailbox id="`) && head.includes(` dir="${t.fx.project[key]}"`), `${key}: an odd tag ${head}`);
        if (key === "rail-a-sub") assert(head.includes(` root="${t.fx.dirs.alpha}"`), `a folder session's tag names no root: ${head}`);
        if (key === "rail-f-1") assert(head.includes(' branch="feat/x"') && head.includes(' harness="codex"'), `the worktree session's tag: ${head}`);
        if (key === "rail-a-02") assert(head.endsWith(">Build bot</mailbox>"), `the labeled session's tag: ${head}`);
      }
    },
  },
  {
    name: "look",
    about: "LOOK captures: the rail column and the overlay rail, idle and hovered, beside the mockup's column A",
    contexts: [{ viewport: { width: 1700, height: 820 }, colorScheme: "dark" }],
    async run(t, browser) {
      const out = opt.shots || fs.mkdtempSync(path.join(os.tmpdir(), "rail-headless-shots-"));
      fs.mkdirSync(out, { recursive: true });
      const shots = [];
      async function snap(page, selector, name) {
        const file = path.join(out, name);
        await page.locator(selector).first().screenshot({ path: file, animations: "disabled" });
        shots.push(file);
      }
      await t.open("rail-f-1");
      await snap(t.page, "#rail", "rail-column-idle.png");
      await t.page.hover(`${t.sel("rail-a-03")} .rid`);
      await sleep(250);
      await snap(t.page, "#rail", "rail-column-hover.png");
      const o = await makeContext(browser, t.base, t.fx, { viewport: { width: 1200, height: 820 }, colorScheme: "dark" });
      try {
        await o.open("rail-f-1");
        await o.page.click("#railbtn");
        await until("the overlay rail", () => o.page.evaluate(() => document.documentElement.classList.contains("rail-open")));
        await sleep(400);
        await snap(o.page, "#rail", "rail-overlay-idle.png");
        await o.page.hover(`${o.sel("rail-a-03")} .rid`);
        await sleep(250);
        await snap(o.page, "#rail", "rail-overlay-hover.png");
      } finally {
        await o.context.close();
      }
      if (fs.existsSync(MOCKUP)) {
        const m = await t.context.newPage();
        await m.setViewportSize({ width: 1100, height: 900 });
        await m.goto(pathToFileURL(MOCKUP).href);
        await snap(m, ".col.A", "mockup-A-idle.png");
        await m.click("#rmHv");
        await snap(m, ".col.A", "mockup-A-hover.png");
        await m.close();
      } else console.log(`  (no mockup at ${MOCKUP}; its captures are skipped)`);
      // One side-by-side sheet of every capture.
      const sheet = await t.context.newPage();
      await sheet.setViewportSize({ width: 2200, height: 900 });
      const imgs = shots.map((f) => `<figure style="margin:0"><figcaption style="font:12px system-ui;color:#ddd;margin:0 0 4px">${path.basename(f)}</figcaption><img src="data:image/png;base64,${fs.readFileSync(f).toString("base64")}"></figure>`).join("");
      await sheet.setContent(`<body style="margin:0;background:#3a3d44"><div id="s" style="display:flex;gap:18px;align-items:flex-start;padding:14px;width:max-content">${imgs}</div></body>`);
      const file = path.join(out, "look-side-by-side.png");
      await sheet.locator("#s").screenshot({ path: file });
      shots.push(file);
      await sheet.close();
      for (const f of shots) assert(fs.statSync(f).size > 1000, `${f} is empty`);
      console.log(shots.map((f) => `  ${f}`).join("\n"));
    },
  },
  {
    name: "tree",
    about: "a parent's middle attaches (outlined, no line), a line among the children reorders them at the child indent, a top-level line detaches; a collapsed parent rolls up a child's approval wait, its toggle and Left/Right fold it, a reload keeps the fold; TREE captures in the column and the overlay",
    contexts: [{ viewport: { width: 1700, height: 820 }, colorScheme: "dark" }],
    async run(t, browser) {
      const { page, fx } = t;
      await t.open("rail-g-main");
      for (const [k, l] of [["rail-g-main", "메인"], ["rail-g-exec", "EXEC"], ["rail-g-ui", "UI"]]) await t.relabel(k, l);
      await until("the labels in the rail", async () => { const n = await t.railNames(); return ["메인", "EXEC", "UI"].every((l) => n.includes(l)); });
      const [main, exec, ui, other] = ["rail-g-main", "rail-g-exec", "rail-g-ui", "rail-g-other"].map((k) => t.rid(k));
      const mainSel = t.sel("rail-g-main");
      const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
      // kids is the reply-ids shown under 메인; top the live top-level rows
      // of gamma's worktree; outlined the rows outlined as a drop's parent.
      const kids = () => page.evaluate((p) => [...document.querySelectorAll(`#rlist .rkids[data-parent="${p}"] > .rs`)].map((r) => r.getAttribute("data-rid")), main);
      const top = () => t.group("rail-g-other");
      const outlined = () => page.evaluate(() => [...document.querySelectorAll("#rlist .rs.rdkid")].map((r) => r.getAttribute("data-rid")));
      // drag presses key's grip with gamma's bar 100px below the list's top
      // (clear of the edge scroll), drags past the slop, and moves to y(box)
      // over the onto row; it answers what the drag shows there.
      const mid = (b) => b.y + b.height / 2, edge = (b) => b.y + 3;
      async function drag(key, onto, y) {
        await t.barAt(fx.dirs.gamma);
        const at = await t.press(key);
        await page.mouse.move(at.x, at.y + 8, { steps: 2 });
        assert(await t.dragging(), `${key}'s grip did not start a drag`);
        const box = await t.box(t.sel(onto));
        await page.mouse.move(at.x, y(box), { steps: 10 });
        return { box, line: await t.line(), hint: await t.hint(), outlined: await outlined() };
      }
      // drop releases the drag: ok holds at once (the shown drop) and
      // after the daemon's render lands (the layout op's rail swap).
      async function drop(what, ok) {
        const h0 = await t.railHash();
        await page.mouse.up();
        assert(await ok(), `the drop does not show ${what} at once`);
        assert((await outlined()).length === 0 && !(await t.line()), "the drop's outline or line outlived it");
        await until(`the daemon's render of ${what}`, async () => (await t.railHash()) !== h0 && (await ok()));
      }
      // snap captures the rail; held, as the pointer and focus leave it,
      // or else with no hover or focus on a row.
      const tree = [], out = opt.shots || fs.mkdtempSync(path.join(os.tmpdir(), "rail-headless-shots-"));
      fs.mkdirSync(out, { recursive: true });
      async function snap(p, name, held) {
        const file = path.join(out, name);
        if (!held) {
          await p.mouse.move(5, 5);
          await p.evaluate(() => document.activeElement && document.activeElement.blur());
          await sleep(250);
        }
        await p.locator("#rail").screenshot({ path: file, animations: "disabled" });
        tree.push(file);
      }
      // Attach by a parent's middle: 메인 outlined, no line, no hint.
      let s = await drag("rail-g-exec", "rail-g-main", mid);
      assert(same(s.outlined, [main]) && !s.line && !s.hint, `over 메인's middle: outlined ${s.outlined}, line ${JSON.stringify(s.line)}, hint "${s.hint}"`);
      await snap(page, "tree-column-attach-zone.png", true);
      await drop("EXEC under 메인", async () => same(await kids(), [exec]) && !(await top()).includes(exec));
      assert(await page.locator(`${mainSel} .kidtog`).count() === 1, "메인 shows no toggle once it has a child");
      s = await drag("rail-g-ui", "rail-g-main", mid);
      assert(same(s.outlined, [main]) && !s.line, `over 메인's middle: outlined ${s.outlined}, line ${JSON.stringify(s.line)}`);
      await drop("UI after EXEC under 메인", async () => same(await kids(), [exec, ui]));
      assert(await page.textContent(`${mainSel} .kidtog .kn`) === "2", "메인's toggle does not count two children");
      // A child is indented one step (16px) and shows no star.
      const ind = await page.evaluate(([p, k]) => {
        const L = document.getElementById("rlist").getBoundingClientRect().left;
        const hi = (r) => document.querySelector(`#rlist .rs[data-rid="${r}"] .hi`).getBoundingClientRect().left - L;
        return { parent: hi(p), kid: hi(k), star: !!document.querySelector(`#rlist .rs[data-rid="${k}"] .star`) };
      }, [main, exec]);
      near(ind.parent, 49, "메인's harness icon");
      near(ind.kid, 65, "a child's harness icon");
      assert(!ind.star, "a child shows a star");
      // Reorder the children: a line at the child indent, before EXEC.
      s = await drag("rail-g-ui", "rail-g-exec", edge);
      assert(s.line && !s.hint && s.outlined.length === 0, `over EXEC's top: line ${JSON.stringify(s.line)}, hint "${s.hint}", outlined ${s.outlined}`);
      await snap(page, "tree-column-child-line.png", true);
      near(s.line.left, s.box.x, "the line among the children's left");
      near(s.line.top, s.box.y - 1, "the line among the children's top", 1.5);
      const mb = await t.box(mainSel);
      assert(s.line.left > mb.x + 10, `the line among the children starts at ${s.line.left}, not indented from 메인's ${mb.x}`);
      await drop("UI before EXEC", async () => same(await kids(), [ui, exec]));
      // A top-level row held by its grip over a child's indent reads the
      // child row at its height: a line among the children. Back over the
      // held row itself it drops nothing.
      {
        await t.barAt(fx.dirs.gamma);
        const at = await t.press("rail-g-other");
        await page.mouse.move(at.x, at.y + 8, { steps: 2 });
        const eb = await t.box(t.sel("rail-g-exec"));
        assert(at.x < eb.x, `the grip (${at.x}) is not left of the child row (${eb.x})`);
        await page.mouse.move(at.x, eb.y + 3, { steps: 10 });
        const l = await t.line();
        assert(l && Math.abs(l.left - eb.x) <= 0.6 && !(await t.hint()), `over a child's indent: line ${JSON.stringify(l)}, hint "${await t.hint()}"`);
        await page.mouse.move(at.x, at.y, { steps: 10 });
        assert(!(await t.line()), "over the held row itself a line shows");
        await page.mouse.up();
        assert(same(await kids(), [ui, exec]) && (await top()).includes(other), "a drag back to its own place moved rows");
      }
      // Detach by a top-level line: before rail-g-other, at its indent.
      s = await drag("rail-g-exec", "rail-g-other", edge);
      assert(s.line && !s.hint && s.outlined.length === 0, `over rail-g-other's top: line ${JSON.stringify(s.line)}, hint "${s.hint}", outlined ${s.outlined}`);
      near(s.line.left, s.box.x, "the top-level line's left");
      near(s.line.left, mb.x, "the top-level line's left against 메인's");
      await drop("EXEC detached before rail-g-other", async () => {
        const tl = await top();
        return same(await kids(), [ui]) && tl.indexOf(exec) >= 0 && tl.indexOf(exec) === tl.indexOf(other) - 1;
      });
      // The roll-up: UI waits on an approval (the seeded snapshot); 메인
      // shows it only while collapsed, and UI's own chip only while shown.
      const kap = () => page.evaluate((s) => { const k = document.querySelector(`${s} .kap`); return !!k && getComputedStyle(k).display !== "none"; }, mainSel);
      const uiShown = () => page.locator(t.sel("rail-g-ui")).isVisible();
      const folded = async () => (await page.getAttribute(`${mainSel} .kidtog`, "aria-expanded")) === "false" && (await page.evaluate((s) => document.querySelector(s).classList.contains("kcol"), mainSel));
      assert(await page.locator(`${t.sel("rail-g-ui")}[data-ap]`).count() === 1, "UI does not wait on an approval; the seeded snapshot did not take");
      assert(await page.locator(`${mainSel} .kap`).count() === 1, "메인 carries no roll-up for UI's wait");
      assert(!(await kap()) && (await uiShown()) && (await page.locator(`${t.sel("rail-g-ui")} .chip.perm`).isVisible()), "expanded: the roll-up shows, or UI's own chip does not");
      await t.barAt(fx.dirs.gamma);
      await snap(page, "tree-column-expanded.png");
      await page.click(`${mainSel} .kidtog`);
      assert(await folded() && (await kap()) && !(await uiShown()), "the toggle did not collapse 메인 with the roll-up shown");
      assert(await page.getAttribute(`${mainSel} .kap`, "title") === "1 session under this one awaits approval", `the roll-up says "${await page.getAttribute(`${mainSel} .kap`, "title")}"`);
      await snap(page, "tree-column-collapsed.png");
      await page.focus(`${mainSel} .rl`);
      await page.keyboard.press("ArrowRight");
      assert(!(await folded()) && !(await kap()) && (await uiShown()), "ArrowRight did not expand 메인");
      await page.keyboard.press("ArrowLeft");
      assert(await folded() && (await kap()) && !(await uiShown()), "ArrowLeft did not collapse 메인");
      await t.open("rail-g-main");
      assert(await folded() && (await kap()), "a reload lost 메인's fold");
      // The overlay rail: a 12px step.
      const o = await makeContext(browser, t.base, t.fx, { viewport: { width: 1200, height: 820 }, colorScheme: "dark" });
      try {
        await o.open("rail-g-main");
        await o.page.click("#railbtn");
        await until("the overlay rail", () => o.page.evaluate(() => document.documentElement.classList.contains("rail-open")));
        await sleep(400);
        await o.barAt(fx.dirs.gamma);
        const oi = await o.page.evaluate(([p, k]) => {
          const hi = (r) => document.querySelector(`#rlist .rs[data-rid="${r}"] .hi`).getBoundingClientRect().left;
          return hi(k) - hi(p);
        }, [main, ui]);
        near(oi, 12, "the overlay's child step");
        await snap(o.page, "tree-overlay-expanded.png");
        await o.page.click(`${o.sel("rail-g-main")} .kidtog`);
        await snap(o.page, "tree-overlay-collapsed.png");
      } finally {
        await o.context.close();
      }
      for (const f of tree) assert(fs.statSync(f).size > 1000, `${f} is empty`);
      console.log(tree.map((f) => `  ${f}`).join("\n"));
    },
  },
];

// ------------------------------------------------------------------- main --
async function main() {
  if (opt.list) {
    for (const s of SCENARIOS) console.log(`${s.name.padEnd(19)} ${s.about}`);
    return 0;
  }
  const picked = opt.names.length ? opt.names.map((n) => SCENARIOS.find((s) => s.name === n) || die(`no scenario ${n} (see --list)`)) : SCENARIOS;
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "rail-headless-")));
  let failed = 0, browser = null;
  try {
    const fx = seed(root);
    const base = await startDaemon(root, fx);
    console.log(`isolated daemon ${base} (pid ${daemon.pid}), fixture ${root}`);
    browser = await launchBrowser();
    for (const s of picked) {
      await resetLayout(base, fx);
      const t = await makeContext(browser, base, fx, (s.contexts && s.contexts[0]) || {});
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
    await stopDaemon();
    if (opt.keep || failed) console.log(`fixture kept at ${root}`);
    else fs.rmSync(root, { recursive: true, force: true });
  }
  console.log(failed ? `${failed} of ${picked.length} scenarios failed` : `all ${picked.length} scenarios passed`);
  return failed ? 1 : 0;
}

main().then((code) => process.exit(code), (e) => {
  console.error(`rail-headless: ${e.stack || e}`);
  killDaemon();
  process.exit(1);
});

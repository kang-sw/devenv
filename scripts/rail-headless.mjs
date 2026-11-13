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
// Options: --shots DIR (where the look scenario saves its captures; default a
// fresh rail-headless-shots-* folder under the OS temp dir, printed), --keep
// (keep the fixture's temp dir), --headed (show the browser).
//
// It never touches the shared dashboard daemon or its cache. It builds ws-mcp
// from this checkout into a temp dir, seeds a fixture there (git repositories
// with a linked worktree, a plain folder, and fake session registrations:
// several workspaces and worktrees, live and ended rows, labeled ones), and
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
// context and page, the fixture, and the page helpers).

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
// The seeded sessions. ws: the folder (alpha and beta are repositories,
// alpha-feat alpha's linked worktree on feat/x, notes a plain folder); sub
// roots the session in a folder of it; ended folds the row.
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
  const dirs = { alpha: path.join(work, "alpha"), "alpha-feat": path.join(work, "alpha-feat"), beta: path.join(work, "beta"), notes: path.join(work, "notes") };
  for (const name of ["alpha", "beta"]) {
    fs.mkdirSync(dirs[name], { recursive: true });
    git(dirs[name], "init", "-q", "-b", "main");
    git(dirs[name], "commit", "--allow-empty", "-qm", "initial");
  }
  git(dirs.alpha, "worktree", "add", "-q", "-b", "feat/x", dirs["alpha-feat"]);
  fs.mkdirSync(path.join(dirs.alpha, "sub"), { recursive: true });
  fs.mkdirSync(dirs.notes, { recursive: true });
  const common = { alpha: path.join(dirs.alpha, ".git"), "alpha-feat": path.join(dirs.alpha, ".git"), beta: path.join(dirs.beta, ".git"), notes: "" };
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
    writeJSON(path.join(dir, "registration.json"), reg);
    // A finished tool batch gives the row its last-tool time.
    const t = now - (i + 1) * 97_000;
    writeJSON(path.join(dir, "snapshot.json"), { schemaVersion: 1, replyId: rid, updatedAt: now, lead: { batches: [{ seq: 1, toolNames: ["Bash"], callCount: 1, failed: 0, startedAt: t - 2000, endedAt: t }] } });
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
// makeContext opens a fresh browser context (its own storage, so no stars or
// order leak between scenarios) and gives the scenario its helpers.
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
    about: "the star follows the name, hidden until hover or focus, always on when starred; a click stars without navigating; a long name ellipsizes before it",
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
      assert((await t.group("rail-a-03"))[0] === t.rid("rail-a-03"), "the starred row is not pinned first");
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
    about: "a grip drag reorders, with a rail swap injected mid-press held until the press ends",
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
      const stored = JSON.parse(await page.evaluate(() => localStorage.getItem("ws.dashboard.railorder")) || "[]");
      assert(stored.indexOf(g[3]) >= 0 && stored.indexOf(g[3]) < stored.indexOf(g[0]), "the new order was not stored");
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
      b = await t.box(`${t.sel("rail-f-2")} .rid`);
      await page.mouse.move(b.x + 20, b.y + b.height / 2, { steps: 10 });
      assert(await t.hint() === "Can't move to another worktree", `over another worktree the hint is "${await t.hint()}"`);
      await page.mouse.up();
      assert(!(await t.hint()) && !(await page.evaluate(() => document.documentElement.classList.contains("drno"))), "the hint outlived the drag");
      for (const [k, want] of [["rail-a-04", g], ["rail-f-1", feat], ["rail-b-1", beta]]) {
        assert(JSON.stringify(await t.group(k)) === JSON.stringify(want), `a refused drop moved rows in ${k}'s group`);
      }
      // Across the stars: the starred row is pinned first, so both rows are
      // in view from the top.
      await page.click(`${t.sel("rail-a-10")} .star`);
      await t.setScroll(0);
      const at2 = await t.press("rail-a-03");
      await page.mouse.move(at2.x, at2.y + 8, { steps: 2 });
      b = await t.box(`${t.sel("rail-a-10")} .rid`);
      await page.mouse.move(b.x + 20, b.y + b.height / 2, { steps: 10 });
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
];

// ------------------------------------------------------------------- main --
async function main() {
  if (opt.list) {
    for (const s of SCENARIOS) console.log(`${s.name.padEnd(16)} ${s.about}`);
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

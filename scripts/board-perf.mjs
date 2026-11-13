#!/usr/bin/env node
// Board frontend performance harness. Repository-local developer command; not
// shipped with any plugin package.
//
//   node scripts/board-perf.mjs run [--tree DIR[,DIR...]] [--rates 4,6] [--runs 3]
//        [--scenarios a,b,c,d,e] [--idle 60] [--shape 8,6,4,2,60]
//        [--label NAME[,NAME...]] [--out FILE.json[,FILE.json...]] [--gc]
//   node scripts/board-perf.mjs compare BEFORE.json AFTER.json [--targets SPEC]
//   node scripts/board-perf.mjs shot [--tree DIR] [--shape ...] --out DIR
//   node scripts/board-perf.mjs diff DIR_A DIR_B
//
// `run` builds the internal/mcp test binary from DIR (an agents-plugin-tool
// tree, default this checkout's), and for every run starts a fresh
// TestBoardPerfServe (internal/mcp/board_perf_serve_test.go): an in-process
// dashboard daemon in temp directories serving a synthetic long board. It
// never touches the installed daemon. Chromium (headless, Playwright) loads
// the session page with CDP CPU throttling at each rate and records
// Performance.getMetrics deltas, long tasks, mounted and built topic frames,
// and keystroke Event Timing over the scenarios:
//   a  cold load (navigation to quiet)
//   b  idle for --idle seconds
//   c  20 scripted one-line updates into one topic, each swap landed
//   d  typing 50 characters into a topic note
//   e  scrolling the board column end to end
// Every run of b-e starts from its own cold load, which counts as an (a)
// sample, so (a) gets four times --runs samples per rate. `compare` prints
// the per-metric median change at each rate and the ticket's A/B keep rule
// verdict at 4x for the target metrics (--targets "a:Frames,e:LayoutDuration"),
// with each metric's within-side spread; several trees in one run alternate
// run by run (the noise amendment's A/B form). `shot` captures the board column at scroll
// stops (LOOK captures); `diff` compares two capture sets pixel by pixel.
//
// Playwright: the cached copy under ~/.npm/_npx/*/node_modules/playwright is
// used (run `npx playwright --version` once to get it, and `npx playwright
// install chromium` for the browser), or set PLAYWRIGHT_DIR to a playwright
// package directory. Windows runs work the same way with Chrome/Edge via
// --channel chrome|msedge.

import { spawn, execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import readline from "node:readline";
import { fileURLToPath } from "node:url";

const REPO = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const METRICS = ["Nodes", "Frames", "JSHeapUsedSize", "LayoutCount", "LayoutDuration", "RecalcStyleCount", "RecalcStyleDuration", "ScriptDuration", "TaskDuration"];
// Absolute (state) metrics; the rest are deltas over the scenario window.
const ABSOLUTE = new Set(["Nodes", "Frames", "JSHeapUsedSize", "mountedFrames"]);
// Metrics recorded but never judged by the keep rule (memory is out of
// scope, Decision 12; quietMs is noisy wall time).
const UNJUDGED = new Set(["JSHeapUsedSize", "quietMs"]);

function parseArgs(argv) {
  const out = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) {
      const k = a.slice(2);
      const v = argv[i + 1] && !argv[i + 1].startsWith("--") ? argv[++i] : "1";
      out[k] = v;
    } else out._.push(a);
  }
  return out;
}

function loadPlaywright() {
  const dirs = [];
  if (process.env.PLAYWRIGHT_DIR) dirs.push(process.env.PLAYWRIGHT_DIR);
  const npx = path.join(os.homedir(), ".npm", "_npx");
  if (fs.existsSync(npx)) {
    for (const d of fs.readdirSync(npx)) {
      const p = path.join(npx, d, "node_modules", "playwright");
      if (fs.existsSync(path.join(p, "package.json"))) dirs.push(p);
    }
  }
  for (const d of dirs) {
    try { return createRequire(path.join(d, "package.json"))(d); } catch (e) { /* next */ }
  }
  throw new Error("playwright not found: run `npx playwright --version` once, or set PLAYWRIGHT_DIR");
}

function buildServer(tree) {
  const bin = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "board-perf-")), process.platform === "win32" ? "mcp.test.exe" : "mcp.test");
  process.stderr.write(`building ${tree}/internal/mcp test binary...\n`);
  execFileSync("go", ["test", "-c", "-o", bin, "./internal/mcp"], { cwd: tree, stdio: ["ignore", "inherit", "inherit"] });
  return bin;
}

// startServer runs the serve test and resolves once it prints its URL.
function startServer(bin, shape) {
  return new Promise((resolve, reject) => {
    const env = { ...process.env, WS_BOARD_PERF_SERVE: "1" };
    if (shape) env.WS_BOARD_PERF_SHAPE = shape;
    const p = spawn(bin, ["-test.run", "^TestBoardPerfServe$", "-test.timeout", "0"], { env, stdio: ["pipe", "pipe", "inherit"] });
    const waiters = [];
    const rl = readline.createInterface({ input: p.stdout });
    let ready = false;
    rl.on("line", (line) => {
      const m = /^BOARD-PERF READY (\S+)/.exec(line);
      if (m && !ready) {
        ready = true;
        resolve({
          url: m[1],
          log(n) {
            return new Promise((res) => { waiters.push(res); p.stdin.write(`log ${n}\n`); });
          },
          stop() {
            return new Promise((res) => { p.once("exit", res); p.stdin.write("quit\n"); setTimeout(() => p.kill(), 5000); });
          },
        });
      } else if (line.startsWith("BOARD-PERF OK")) {
        const w = waiters.shift();
        if (w) w();
      } else if (line.startsWith("BOARD-PERF ERR")) process.stderr.write(line + "\n");
    });
    p.once("exit", (code) => { if (!ready) reject(new Error(`serve test exited (${code}) before it was ready`)); });
  });
}

// PAGE_PROBE runs in every document before its scripts: it counts long
// tasks, keystroke Event Timing entries, and topic frames built (a doc frame
// loading its srcdoc document), and times each keydown to the next frame.
// Since the dashboard's iframe shell, the session page is the shell's frame
// (?frame=1), whose window outlives its initial about:blank document, where
// this script runs: the observers live on the window, and the document
// listeners are hooked again on the document the frame navigates to.
const PAGE_PROBE = `(() => {
  if (location.href === "about:srcdoc") return;
  const P = window.__bp = { long: [], events: [], keys: [], built: 0 };
  try {
    new PerformanceObserver((l) => l.getEntries().forEach((e) => P.long.push([e.startTime, e.duration]))).observe({ type: "longtask", buffered: true });
  } catch (e) {}
  try {
    new PerformanceObserver((l) => l.getEntries().forEach((e) => { if (/^key|^input|^beforeinput/.test(e.name)) P.events.push([e.startTime, e.duration, e.name]); })).observe({ type: "event", durationThreshold: 16, buffered: true });
  } catch (e) {}
  function hook(d) {
    d.addEventListener("keydown", (e) => {
      const t0 = e.timeStamp;
      requestAnimationFrame(() => { const ch = new MessageChannel(); ch.port1.onmessage = () => P.keys.push(performance.now() - t0); ch.port2.postMessage(0); });
    }, true);
    // A topic frame's document is built when the frame loads its srcdoc; an
    // unmounted frame's about:blank load does not count.
    d.addEventListener("load", (e) => { const f = e.target; if (f && f.matches && f.matches("iframe.doc-frame") && f.hasAttribute("srcdoc")) P.built++; }, true);
  }
  let hooked = document;
  hook(document);
  if (location.href !== "about:blank") return;
  const iv = setInterval(() => {
    if (document === hooked) return;
    hooked = document;
    hook(document);
    if (location.href !== "about:blank") clearInterval(iv);
  }, 5);
})();`;

async function metrics(cdp) {
  const { metrics } = await cdp.send("Performance.getMetrics");
  const out = {};
  for (const m of metrics) if (METRICS.includes(m.name)) out[m.name] = m.name.endsWith("Duration") ? m.value * 1000 : m.value;
  return out;
}

// endMetrics is a scenario's closing metrics. With --gc, the absolute
// counts (Nodes, Frames, JSHeapUsedSize) are read again after a forced
// garbage collection, so they count what the page still holds rather than
// what a collection that happened or not left behind; the durations stay
// those read before it.
async function endMetrics(cdp, opts) {
  const m = await metrics(cdp);
  if (!opts.gc) return m;
  await cdp.send("HeapProfiler.collectGarbage");
  const g = await metrics(cdp);
  for (const k of ["Nodes", "Frames", "JSHeapUsedSize"]) m[k] = g[k];
  return m;
}

// sess is the frame that holds the session page: the iframe shell's
// session frame (?frame=1), else (a tree from before the shell) the page.
// Performance metrics are the page's whole renderer, the shell included,
// the same on both sides of an A/B.
function sess(page) {
  return page.frames().find((f) => /[?&]frame=1/.test(f.url())) || page.mainFrame();
}

async function probe(page) {
  return sess(page).evaluate(() => {
    const P = window.__bp;
    return { now: performance.now(), long: P.long.slice(), events: P.events.slice(), keys: P.keys.slice(), built: P.built, mounted: document.querySelectorAll("iframe.doc-frame[srcdoc]").length };
  });
}

function median(xs) {
  const s = xs.filter((x) => typeof x === "number" && isFinite(x)).sort((a, b) => a - b);
  if (!s.length) return NaN;
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}
function pct(xs, p) {
  const s = xs.slice().sort((a, b) => a - b);
  return s.length ? s[Math.min(s.length - 1, Math.floor(p * s.length))] : 0;
}

// window computes a scenario's sample from metrics and probes taken at its
// start and end.
function sample(m0, m1, p0, p1) {
  const out = {};
  for (const k of METRICS) out[k] = ABSOLUTE.has(k) ? m1[k] : m1[k] - (m0 ? m0[k] : 0);
  const from = p0 ? p0.now : 0;
  const long = p1.long.filter((l) => l[0] >= from);
  out.longTasks = long.length;
  out.longTaskMs = long.reduce((a, l) => a + l[1], 0);
  out.framesBuilt = p1.built - (p0 ? p0.built : 0);
  out.mountedFrames = p1.mounted;
  const keys = p1.keys.slice(p0 ? p0.keys.length : 0);
  const events = p1.events.filter((e) => e[0] >= from);
  if (keys.length) {
    out.keyLatMedian = median(keys);
    out.keyLatP95 = pct(keys, 0.95);
    out.keyEvents16 = events.length;
    out.keyEventMax = events.reduce((a, e) => Math.max(a, e[1]), 0);
  }
  return out;
}

// quiet waits until the page stops laying out and its tasks settle: two
// consecutive 500 ms windows with no layout and under 40 ms of task time.
async function quiet(page, cdp, maxMs = 90000) {
  const start = Date.now();
  let last = await metrics(cdp), calm = 0;
  while (Date.now() - start < maxMs) {
    await page.waitForTimeout(500);
    const m = await metrics(cdp);
    calm = m.LayoutCount === last.LayoutCount && m.TaskDuration - last.TaskDuration < 40 ? calm + 1 : 0;
    last = m;
    if (calm >= 2) break;
  }
  return Date.now() - start;
}

async function coldLoad(browser, url, rate, opts) {
  const context = await browser.newContext({ viewport: { width: 1600, height: 1000 }, colorScheme: opts.scheme || "light" });
  await context.addInitScript(PAGE_PROBE);
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("Performance.enable", { timeDomain: "timeTicks" });
  await cdp.send("Emulation.setCPUThrottlingRate", { rate });
  const m0 = await metrics(cdp);
  const t0 = Date.now();
  await page.goto(url, { waitUntil: "load", timeout: 180000 });
  await quiet(page, cdp);
  const quietMs = Date.now() - t0 - 1000; // the last two calm windows
  const p1 = await probe(page);
  const a = sample(m0, await endMetrics(cdp, opts), null, p1);
  a.quietMs = quietMs;
  return { context, page, cdp, a };
}

async function boardHash(page) { return sess(page).evaluate(() => { const b = document.getElementById("r-board"); return b ? b.getAttribute("data-h") : ""; }); }

async function scenario(name, page, cdp, server, opts) {
  if (name === "b") {
    await page.waitForTimeout(opts.idle * 1000);
  } else if (name === "c") {
    for (let i = 0; i < 20; i++) {
      const h = await boardHash(page);
      await server.log(1);
      await sess(page).waitForFunction((was) => { const b = document.getElementById("r-board"); return b && b.getAttribute("data-h") !== was; }, h, { timeout: 60000 });
      await page.waitForTimeout(300);
    }
    await quiet(page, cdp);
  } else if (name === "d") {
    await sess(page).evaluate(() => {
      const t = document.querySelector('#r-board section.topic[data-topic="T00"] .tnote-toggle');
      t.scrollIntoView({ block: "center" });
    });
    await sess(page).click('#r-board section.topic[data-topic="T00"] .tnote-toggle');
    await page.waitForTimeout(300);
    await page.keyboard.type("The quick brown fox jumps over the lazy dog 012345", { delay: 40 });
    await page.waitForTimeout(500);
  } else if (name === "e") {
    await sess(page).evaluate(() => new Promise((done) => {
      const col = document.getElementById("bcol");
      col.scrollTop = 0;
      function step() {
        const max = col.scrollHeight - col.clientHeight;
        if (col.scrollTop >= max - 1) { done(); return; }
        col.scrollTop = Math.min(max, col.scrollTop + 120);
        requestAnimationFrame(step);
      }
      requestAnimationFrame(step);
    }));
    await quiet(page, cdp);
  }
}

async function run(args) {
  // --tree may name several trees, comma-separated (with --label and --out
  // lists in step): each run then measures every tree in turn, the order
  // alternating run by run, so host drift lands on every side alike.
  const trees = (args.tree || path.join(REPO, "agents-plugin-tool")).split(",").map((t) => path.resolve(t));
  const labels = args.label ? args.label.split(",") : trees.map((t) => path.basename(t));
  const outs = args.out ? args.out.split(",") : [];
  const rates = (args.rates || "4,6").split(",").map(Number);
  const runs = +(args.runs || 3);
  const scen = (args.scenarios || "a,b,c,d,e").split(",");
  const opts = { idle: +(args.idle || 60), gc: !!args.gc };
  const bins = trees.map(buildServer);
  const { chromium } = loadPlaywright();
  const browser = await chromium.launch({ headless: true, channel: args.channel });
  const results = trees.map((tree, i) => ({ label: labels[i] || path.basename(tree), tree, shape: args.shape || "8,6,4,2,60", rates, runs, idle: opts.idle, gc: opts.gc, alternated: trees.length > 1, version: browser.version(), samples: {} }));
  const order = (r) => (r % 2 ? trees.map((_, i) => trees.length - 1 - i) : trees.map((_, i) => i));
  try {
    for (const rate of rates) {
      for (const s of scen) {
        // Every run of b-e starts with a cold load, which is an (a)
        // sample; a alone runs `runs` cold loads of its own.
        if (s === "a" && scen.some((x) => x !== "a")) continue;
        for (let r = 0; r < runs; r++) {
          for (const i of order(r)) {
            const server = await startServer(bins[i], args.shape);
            const { context, page, cdp, a } = await coldLoad(browser, server.url, rate, opts);
            if (scen.includes("a")) push(results[i], "a", rate, a);
            if (s !== "a") {
              const m0 = await metrics(cdp), p0 = await probe(page);
              await scenario(s, page, cdp, server, opts);
              const m1 = await endMetrics(cdp, opts);
              push(results[i], s, rate, sample(m0, m1, p0, await probe(page)));
            }
            await context.close();
            await server.stop();
            process.stderr.write(`${results[i].label} ${rate}x ${s}#${r + 1} done\n`);
          }
        }
      }
    }
  } finally {
    await browser.close();
  }
  results.forEach((result, i) => {
    const out = outs[i] || path.join(os.tmpdir(), `board-perf-${result.label}-${Date.now()}.json`);
    fs.writeFileSync(out, JSON.stringify(result, null, 2));
    process.stdout.write(table(result) + `\nwrote ${out}\n`);
  });
}

function push(result, s, rate, x) {
  const k = `${s}@${rate}`;
  (result.samples[k] = result.samples[k] || []).push(x);
}

function medians(result) {
  const out = {};
  for (const [k, xs] of Object.entries(result.samples)) {
    const keys = new Set(xs.flatMap((x) => Object.keys(x)));
    out[k] = {};
    for (const m of keys) out[k][m] = median(xs.map((x) => x[m]));
  }
  return out;
}

const SHOW = ["Nodes", "Frames", "mountedFrames", "framesBuilt", "LayoutCount", "LayoutDuration", "RecalcStyleDuration", "ScriptDuration", "TaskDuration", "longTasks", "longTaskMs", "keyLatMedian", "keyLatP95", "keyEvents16", "keyEventMax", "quietMs"];
function fmt(v) { return typeof v !== "number" || !isFinite(v) ? "-" : Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(1); }

function table(result) {
  const med = medians(result);
  const keys = Object.keys(med).sort();
  const cols = SHOW.filter((m) => keys.some((k) => isFinite(med[k][m])));
  let s = `${result.label} (${result.version}, shape ${result.shape}, median of ${result.runs})\n| scenario | ${cols.join(" | ")} |\n|${"---|".repeat(cols.length + 1)}\n`;
  for (const k of keys) s += `| ${k} | ${cols.map((m) => fmt(med[k][m])).join(" | ")} |\n`;
  return s;
}

// compare prints AFTER against BEFORE at each rate and the keep rule at 4x:
// keep when a target metric improves by at least 10% and no recorded metric
// in any scenario regresses by more than 10%.
function compare(args) {
  const [aPath, bPath] = args._.slice(1);
  const A = JSON.parse(fs.readFileSync(aPath, "utf8")), B = JSON.parse(fs.readFileSync(bPath, "utf8"));
  const ma = medians(A), mb = medians(B);
  const targets = (args.targets || "").split(",").filter(Boolean).map((t) => t.split(":"));
  // spread is a metric's within-side spread: max minus min over a side's
  // runs as a share of its median, the larger of the two sides. A judged
  // change within it is marked (~): the noise amendment re-runs both
  // sides, alternating, with at least 5 runs each.
  const spread = (k, m) => Math.max(...[A, B].map((R) => {
    const xs = (R.samples[k] || []).map((x) => x[m]).filter((v) => typeof v === "number" && isFinite(v));
    const md = median(xs);
    return xs.length && md ? (Math.max(...xs) - Math.min(...xs)) / Math.abs(md) : 0;
  }));
  let s = `${A.label} -> ${B.label}\n| scenario | metric | before | after | change | spread |\n|---|---|---|---|---|---|\n`;
  const regress = [], improved = [];
  for (const k of Object.keys(mb).sort()) {
    if (!ma[k]) continue;
    for (const m of SHOW) {
      const x = ma[k][m], y = mb[k][m];
      if (!isFinite(x) || !isFinite(y)) continue;
      const ch = x === 0 ? (y === 0 ? 0 : Infinity) : (y - x) / Math.abs(x);
      const sp = spread(k, m);
      s += `| ${k} | ${m} | ${fmt(x)} | ${fmt(y)} | ${isFinite(ch) ? (ch * 100).toFixed(1) + "%" : "new"} | ${(sp * 100).toFixed(0)}% |\n`;
      if (!k.endsWith("@4")) continue;
      const tag = Math.abs(ch) <= sp ? " (~ within " + (sp * 100).toFixed(0) + "% spread)" : "";
      // A regression counts past 10% and past a floor of noise in absolute
      // terms (one layout, 1 ms, one frame, one long task).
      if (!UNJUDGED.has(m) && ch > 0.1 && y - x >= 1) regress.push(`${k} ${m} ${(ch * 100).toFixed(1)}%${tag}`);
      if (targets.some(([sc, mt]) => `${sc}@4` === k && mt === m) && ch <= -0.1) improved.push(`${k} ${m} ${(ch * 100).toFixed(1)}%${tag}`);
    }
  }
  s += `\ntargets improved >=10% at 4x: ${improved.join("; ") || "none"}\nregressions >10% at 4x: ${regress.join("; ") || "none"}\n`;
  if (targets.length) s += `keep rule: ${improved.length && !regress.length ? "KEEP" : "FAIL"}\n`;
  process.stdout.write(s);
}

// shot captures the board column at scroll stops after a quiet cold load:
// LOOK captures for a before/after comparison (diff).
async function shot(args) {
  const tree = path.resolve(args.tree || path.join(REPO, "agents-plugin-tool"));
  const out = path.resolve(args.out || fs.mkdtempSync(path.join(os.tmpdir(), "board-shot-")));
  fs.mkdirSync(out, { recursive: true });
  const bin = buildServer(tree);
  const { chromium } = loadPlaywright();
  const browser = await chromium.launch({ headless: true, channel: args.channel });
  try {
    for (const scheme of ["light", "dark"]) {
      const server = await startServer(bin, args.shape);
      const { context, page, cdp } = await coldLoad(browser, server.url, 1, { scheme });
      // Open every closed topic stream and versions fold, so the frames a
      // lazy mount defers draw as well.
      await sess(page).evaluate(() => document.querySelectorAll("#bcol details.panel-versions").forEach((d) => { d.open = true; }));
      await page.waitForTimeout(1500);
      await quiet(page, cdp);
      const col = await sess(page).$("#bcol");
      const stops = await sess(page).evaluate(() => { const c = document.getElementById("bcol"); c.scrollTop = 0; return Math.ceil(c.scrollHeight / c.clientHeight); });
      for (let i = 0; i < stops; i++) {
        await sess(page).evaluate((i) => { const c = document.getElementById("bcol"); c.scrollTop = i * c.clientHeight; }, i);
        await page.waitForTimeout(700);
        await col.screenshot({ path: path.join(out, `${scheme}-${String(i).padStart(2, "0")}.png`), animations: "disabled", caret: "hide" });
      }
      await context.close();
      await server.stop();
    }
  } finally {
    await browser.close();
  }
  process.stdout.write(`wrote ${out}\n`);
}

// diff compares same-named captures in two directories in the browser
// (canvas pixels), printing the differing pixel share per capture.
async function diff(args) {
  const [da, db] = args._.slice(1).map((d) => path.resolve(d));
  const { chromium } = loadPlaywright();
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  let worst = 0;
  try {
    for (const name of fs.readdirSync(da).filter((n) => n.endsWith(".png")).sort()) {
      const pb = path.join(db, name);
      if (!fs.existsSync(pb)) { process.stdout.write(`${name}: missing in ${db}\n`); worst = 1; continue; }
      const enc = (p) => "data:image/png;base64," + fs.readFileSync(p).toString("base64");
      const r = await page.evaluate(async ([a, b]) => {
        const load = (src) => new Promise((ok) => { const i = new Image(); i.onload = () => ok(i); i.src = src; });
        const [ia, ib] = await Promise.all([load(a), load(b)]);
        if (ia.width !== ib.width || ia.height !== ib.height) return { size: `${ia.width}x${ia.height} vs ${ib.width}x${ib.height}` };
        const px = (img) => { const c = document.createElement("canvas"); c.width = img.width; c.height = img.height; const x = c.getContext("2d"); x.drawImage(img, 0, 0); return x.getImageData(0, 0, c.width, c.height).data; };
        const xa = px(ia), xb = px(ib);
        let n = 0;
        for (let i = 0; i < xa.length; i += 4) if (Math.abs(xa[i] - xb[i]) + Math.abs(xa[i + 1] - xb[i + 1]) + Math.abs(xa[i + 2] - xb[i + 2]) > 24) n++;
        return { share: n / (xa.length / 4) };
      }, [enc(path.join(da, name)), enc(pb)]);
      if (r.size) { process.stdout.write(`${name}: size differs (${r.size})\n`); worst = 1; continue; }
      worst = Math.max(worst, r.share);
      process.stdout.write(`${name}: ${(r.share * 100).toFixed(3)}% pixels differ\n`);
    }
  } finally {
    await browser.close();
  }
  process.stdout.write(`worst: ${(worst * 100).toFixed(3)}%\n`);
}

const args = parseArgs(process.argv.slice(2));
const cmd = args._[0];
const main = { run, compare, shot, diff }[cmd];
if (!main) {
  process.stderr.write("usage: node scripts/board-perf.mjs run|compare|shot|diff ... (see the header comment)\n");
  process.exit(2);
}
Promise.resolve(main(args)).catch((e) => { process.stderr.write((e && e.stack) || String(e)); process.stderr.write("\n"); process.exit(1); });

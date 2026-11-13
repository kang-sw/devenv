#!/usr/bin/env node
// Repository-local developer command: headless browser checks for the
// dashboard's search palette. Not shipped with any plugin package.
//
//     node scripts/palette-headless.mjs [--out DIR] [--only a,b] [--headed] [--keep-tmp]
//
// It builds ws-mcp from agents-plugin-tool into a temp directory and runs an
// isolated dashboard daemon there: its own WS_CACHE_HOME, WS_CONFIG_HOME and
// HOME, a port the OS chooses, assets read from this tree through
// WS_MCP_DASHBOARD_DEV_SOURCE. It never touches a daemon already running on
// this machine, and stops its own by PID when done. It seeds one session
// (an adapter snapshot, so the Files pane exists) with a whiteboard topic of
// three items, a few project files, a listing directory for path mode, and
// two tracker stores, then drives the session page through the iframe shell
// in headless Chromium.
//
// Scenarios (all by default; --only picks some):
//
//   layout   420, 820, 1240, and 1400 px viewports: the palette's width and
//            its layout step (narrow, medium, wide).
//   keys     Tab/Shift+Tab across slots (skipping empty ones), Alt+0-5,
//            Esc twice, reopen with the last query selected, Ctrl+Enter on
//            a board item flashing it behind the open palette, Ctrl+K from
//            the shell document, and / with the Files pane shown.
//   async    Ctrl+Enter on a file and a content hit opens the preview card
//            under the palette with the input keeping focus; a hit from a
//            store whose view did not load; the tracker content slot's
//            error state for a malformed query.
//   path     As a local viewer: <dir>/ lists depth 1, <dir>/fo filters,
//            Enter on a folder completes the query, Enter on a file opens
//            the preview with its open button.
//   look     LOOK captures, light and dark, at each width (strip and filled
//            slots, streaming in progress, done, cap, error, path mode),
//            and the approved mockup at its matching widths, into --out.
//
// Each check prints what failed and the run exits non-zero on any failure.
//
// Playwright is not a dependency of this repository. It is resolved from
// the npx cache (~/.npm/_npx/*/node_modules/playwright, newest first), or
// from PALETTE_HEADLESS_PLAYWRIGHT (a playwright package directory). To get
// it: npx playwright@latest install chromium

import { spawn, spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const REPO = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const TOOL_DIR = path.join(REPO, "agents-plugin-tool");
const MOCKUP = path.join(REPO, "ai-docs", "tickets", "assets", "261114-feat-ws-dashboard-unified-search-palette", "search-palette-mockup.html");
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const log = (s) => console.log(`palette-headless: ${s}`);

function args() {
  const a = { out: path.join(os.tmpdir(), "palette-look") };
  const v = process.argv.slice(2);
  for (let i = 0; i < v.length; i++) {
    const k = v[i].replace(/^--/, "");
    if (k === "headed" || k === "keep-tmp") a[k] = true;
    else a[k] = v[++i];
  }
  return a;
}

function playwright() {
  const own = process.env.PALETTE_HEADLESS_PLAYWRIGHT;
  const dirs = own ? [own] : fs.readdirSync(path.join(os.homedir(), ".npm", "_npx"), { withFileTypes: true })
    .map((d) => path.join(os.homedir(), ".npm", "_npx", d.name, "node_modules", "playwright"))
    .filter((d) => fs.existsSync(path.join(d, "package.json")))
    .sort((x, y) => cmpVer(ver(y), ver(x)));
  if (!dirs.length) throw new Error("no playwright in the npx cache; run: npx playwright@latest install chromium");
  return createRequire(path.join(dirs[0], "package.json"))(dirs[0]);
}
function ver(d) { return JSON.parse(fs.readFileSync(path.join(d, "package.json"), "utf8")).version.split(".").map(Number); }
function cmpVer(a, b) { for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] - b[i]; return 0; }

// childEnv: the caller's environment minus every WS_* variable (a ws
// session's own keys and cache homes must not leak in), plus the isolated
// homes and a git identity for the seeded repositories.
function childEnv(tmp, extra) {
  const env = {};
  for (const [k, v] of Object.entries(process.env)) if (!k.startsWith("WS_")) env[k] = v;
  Object.assign(env, {
    WS_CACHE_HOME: path.join(tmp, "cache"), WS_CONFIG_HOME: path.join(tmp, "config"), HOME: path.join(tmp, "home"),
    GIT_AUTHOR_NAME: "palette", GIT_AUTHOR_EMAIL: "palette@example.invalid", GIT_COMMITTER_NAME: "palette", GIT_COMMITTER_EMAIL: "palette@example.invalid",
  });
  return Object.assign(env, extra || {});
}
function run(cmd, argv, opts) {
  const r = spawnSync(cmd, argv, Object.assign({ encoding: "utf8" }, opts));
  if (r.status !== 0) throw new Error(`${cmd} ${argv.join(" ")} failed (${r.status}): ${(r.stderr || "") + (r.stdout || "")}`.slice(0, 4000));
  return r.stdout;
}
async function waitFor(fn, ms, what) {
  const end = Date.now() + ms;
  for (;;) {
    try { const v = await fn(); if (v) return v; } catch {}
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(100);
  }
}
function write(file, text) { fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, text); }

// --- the isolated daemon and its seeded session ----------------------------
async function serve(tmp) {
  fs.mkdirSync(path.join(tmp, "home"), { recursive: true });
  const bin = path.join(tmp, "ws-mcp");
  log("building ws-mcp ...");
  run("go", ["build", "-o", bin, "./cmd/ws-mcp"], { cwd: TOOL_DIR });
  const proj = path.join(tmp, "proj");
  write(path.join(proj, "ai-docs", "_index.md"), "# palette fixture\n");
  write(path.join(proj, "notes", "palette-guide.md"), "# Palette guide\n\nThe palette streams file and content results.\n");
  write(path.join(proj, "src", "palette_view.go"), "package src\n\n// palette rows render here\nfunc paletteRows() {}\n");
  write(path.join(proj, "src", "other.go"), "package src\n");
  run("git", ["init", "-q", "."], { cwd: proj });
  const listing = path.join(tmp, "listing");
  write(path.join(listing, "food", "inner.txt"), "inside\n");
  write(path.join(listing, "fo.txt"), "fo\n");
  write(path.join(listing, "bar.txt"), "bar\n");
  write(path.join(listing, "xfoy.md"), "x\n");
  const env = childEnv(tmp, { WS_RSRC_ROOT: path.join(REPO, "agents-plugin", "rsrc"), WS_SKILLS_ROOT: path.join(tmp, "no-skills"), WS_MCP_NO_DASHBOARD: "1" });
  const fer = JSON.parse(run(bin, ["call", "ferrule", JSON.stringify({ root: proj, format: "json" })], { cwd: proj, env }));
  const board = '<wb:topic name="Palette design"><wb:body><p>How the palette streams its slots.</p></wb:body>' +
    '<wb:add class="check"><wb:title>Check the palette width steps</wb:title></wb:add>' +
    '<wb:add class="check"><wb:title>Review streaming states</wb:title></wb:add>' +
    '<wb:add class="check"><wb:title>Path mode listing</wb:title></wb:add></wb:topic>';
  const out = run(bin, ["call", "whiteboard.update", JSON.stringify({ session_key: fer.session_key, commands: board })], { cwd: proj, env });
  const m = /ai-docs\/whiteboards\/([a-z0-9]+)\/index\.html/.exec(out);
  if (!m) throw new Error("no whiteboard reply id: " + out.slice(0, 400));
  const s = { rid: m[1], root: fer.root };
  const dashDir = path.join(tmp, "cache", "dashboard");
  const logf = fs.openSync(path.join(tmp, "daemon.log"), "a");
  const daemon = spawn(bin, ["dashboard", "--port", "0"], { cwd: tmp, stdio: ["ignore", logf, logf], env: childEnv(tmp, { WS_MCP_DASHBOARD_DEV_SOURCE: TOOL_DIR }) });
  const ep = await waitFor(() => { const e = JSON.parse(fs.readFileSync(path.join(dashDir, "endpoint.json"), "utf8")); return e.pid === daemon.pid && e.port ? e : null; }, 20000, "the daemon's endpoint file");
  const base = `http://127.0.0.1:${ep.port}`;
  await waitFor(async () => (await fetch(base + "/health")).ok, 10000, "/health");
  const beat = () => {
    const now = new Date(), iso = now.toISOString().replace(/\.\d+Z$/, "Z"), dir = path.join(dashDir, "sessions", s.rid);
    fs.mkdirSync(dir, { recursive: true });
    s.at = s.at || iso;
    fs.writeFileSync(path.join(dir, "registration.json"), JSON.stringify({ schema_version: 1, reply_id: s.rid, project: s.root, harness: "claude", registered_at: s.at, heartbeat_at: iso }));
    fs.writeFileSync(path.join(dir, "snapshot.json"), JSON.stringify({ schemaVersion: 1, replyId: s.rid, adapter: "pi", updatedAt: now.getTime(), heartbeatMs: 3600000, lead: { phase: "idle" } }));
  };
  beat();
  const r = await fetch(base + "/nudge", { method: "POST", body: JSON.stringify({ reply_id: s.rid, project: s.root }) });
  if (r.status !== 204) throw new Error(`nudge: HTTP ${r.status}`);
  await waitFor(async () => (await fetch(`${base}/s/${s.rid}`)).ok, 10000, "the session page");
  return { daemon, base, rid: s.rid, listing, timer: setInterval(beat, 60000) };
}

// --- page helpers --------------------------------------------------------------
let failures = 0;
function check(name, ok, detail) {
  if (ok) console.log(`  ok   ${name}`);
  else { failures++; console.log(`  FAIL ${name}${detail !== undefined ? ": " + JSON.stringify(detail) : ""}`); }
}
async function frameOf(page) {
  return waitFor(() => page.frames().find((f) => /[?&]frame=1/.test(f.url()) && !f.isDetached()), 15000, "the session frame");
}
// state reads the palette from the session frame's DOM.
function state(f) {
  return f.evaluate(() => {
    const d = document.getElementById("tkqpop"), inp = document.getElementById("tkqin");
    // the tabs keep their last markup while hidden: read them only at the narrow step
    const cur = d.getAttribute("data-step") === "narrow" ? d.querySelector(".tk-qtab.on[data-qslot]") : d.querySelector(".tk-qpane.tk-qfocus[data-pane]");
    const panes = {};
    d.querySelectorAll(".tk-qpane[data-pane]").forEach((p) => { panes[p.getAttribute("data-pane")] = p.innerText; });
    return { open: d.open, w: Math.round(d.getBoundingClientRect().width), step: d.getAttribute("data-step"), q: inp.value, sel: [inp.selectionStart, inp.selectionEnd],
      focus: document.activeElement && document.activeElement.id, cur: cur && (cur.getAttribute("data-pane") || cur.getAttribute("data-qslot")),
      chip: !!d.querySelector("#tkqjump .tk-qchip.on"), panes, text: d.innerText, spin: !!d.querySelector(".tk-qspin"), stale: !!d.querySelector(".tk-qstale") };
  });
}
async function settle(f) { await sleep(150); await f.waitForFunction(() => !document.querySelector("#tkqpop .tk-qspin"), null, { timeout: 10000 }); await sleep(50); }
// openKey presses Ctrl+K in the session frame; right after a load or a resize
// the frame may not hold the keyboard yet, so it tries a few times.
async function openKey(page, f) {
  for (let i = 0; ; i++) {
    await f.evaluate(() => { window.focus(); if (document.activeElement && document.activeElement.blur) document.activeElement.blur(); });
    await page.keyboard.press("Control+k");
    try { await f.waitForFunction(() => document.getElementById("tkqpop").open, null, { timeout: 1500 }); return; } catch (e) { if (i === 3) throw e; }
  }
}
async function typeQ(page, f, q) {
  await f.evaluate(() => { const i = document.getElementById("tkqin"); i.select(); });
  if (q) await page.keyboard.type(q, { delay: 10 }); else await page.keyboard.press("Backspace");
  await settle(f);
}
async function closeAll(page, f) {
  for (let i = 0; i < 3 && (await state(f)).open; i++) await page.keyboard.press("Escape");
}
async function theme(page, f, t) {
  for (const fr of [page.mainFrame(), f]) await fr.evaluate((t) => document.documentElement.setAttribute("data-theme", t), t);
}
async function shot(page, out, name) { const p = path.join(out, name + ".png"); await page.screenshot({ path: p }); return p; }

// --- scenarios -------------------------------------------------------------------
async function layout(ctx) {
  const { page } = ctx;
  for (const [W, w, step] of [[420, 396, "narrow"], [820, 796, "medium"], [1240, 1216, "wide"], [1400, 1240, "wide"]]) {
    await page.setViewportSize({ width: W, height: 860 });
    const f = await frameOf(page);
    await openKey(page, f);
    const s = await state(f);
    check(`layout ${W}: width ${w} and step ${step}`, s.w === w && s.step === step, { w: s.w, step: s.step });
    await closeAll(page, f);
  }
}

async function keys(ctx) {
  const { page } = ctx;
  await page.setViewportSize({ width: 1240, height: 860 });
  const f = await frameOf(page);
  await openKey(page, f);
  await typeQ(page, f, "palette");
  let s = await state(f);
  const filled = Object.keys(s.panes).filter((k) => !/\n(No results|Idle)/.test(s.panes[k]) && s.panes[k].split("\n").length > 2);
  check("keys: a palette query fills board, files, content, tracker", ["board", "files", "content", "tracker"].every((k) => filled.includes(k)), s.panes);
  await page.keyboard.press("Alt+Digit1");
  check("keys: Alt+1 selects the board slot", (await state(f)).cur === "board");
  const seen = [];
  for (let i = 0; i < 5; i++) { await page.keyboard.press("Tab"); seen.push((await state(f)).cur); }
  check("keys: Tab walks the filled slots and wraps", seen[seen.length - 1] !== undefined && new Set(seen).size >= 3, seen);
  const before = (await state(f)).cur;
  await page.keyboard.press("Tab"); await page.keyboard.press("Shift+Tab");
  check("keys: Shift+Tab steps back", (await state(f)).cur === before);
  await page.keyboard.press("Alt+Digit4");
  check("keys: Alt+4 selects the tracker slot", (await state(f)).cur === "tracker");
  // the strip filters with the query; an empty one lists sessions, stores, and actions
  await typeQ(page, f, "");
  await page.keyboard.press("Alt+Digit4");
  await page.keyboard.press("Alt+Digit0");
  check("keys: Alt+0 enters the strip", (await state(f)).chip);
  await page.keyboard.press("ArrowDown");
  check("keys: Down leaves the strip", !(await state(f)).chip && (await state(f)).cur === "tracker");
  check("keys: the focus stays in the input", (await state(f)).focus === "tkqin");
  // Esc twice: clear, then close.
  await typeQ(page, f, "palette");
  await page.keyboard.press("Escape");
  s = await state(f);
  check("keys: the first Esc clears the query and keeps the palette", s.open && s.q === "", s);
  await page.keyboard.press("Escape");
  check("keys: the second Esc closes", !(await state(f)).open);
  // Reopen restores the last query, selected (closed by a backdrop click).
  await openKey(page, f);
  await typeQ(page, f, "streaming");
  await page.mouse.click(4, 850);
  await sleep(100);
  check("keys: a backdrop click closes", !(await state(f)).open);
  await openKey(page, f);
  s = await state(f);
  check("keys: reopen restores the last query, selected", s.q === "streaming" && s.sel[0] === 0 && s.sel[1] === 9, { q: s.q, sel: s.sel });
  // Ctrl+Enter on a board item flashes it behind the open palette.
  await typeQ(page, f, "(2)");
  await page.keyboard.press("Control+Enter");
  await sleep(120);
  const flash = await f.evaluate(() => { const t = document.getElementById("item-2"); return !!t && t.classList.contains("palette-flash"); });
  s = await state(f);
  check("keys: Ctrl+Enter on (2) flashes item 2 behind the open palette", flash && s.open && s.focus === "tkqin", { flash, open: s.open, focus: s.focus });
  await closeAll(page, f);
  // Ctrl+K from the shell document opens the palette in the session frame.
  await page.mainFrame().evaluate(() => { window.focus(); document.body.focus(); });
  await page.keyboard.press("Control+k");
  await sleep(300);
  check("keys: Ctrl+K from the shell document opens the palette in the frame", (await state(f)).open);
  await page.keyboard.press("Escape");
  s = await state(f);
  if (s.open && s.q) { await page.keyboard.press("Escape"); s = await state(f); }
  check("keys: Esc from there closes as above", !s.open);
  // / with the Files pane shown opens the palette, not the Files pane search.
  await f.evaluate(() => { const b = document.querySelector('[data-ctab="files"]'); if (b) b.click(); });
  await sleep(300);
  const shown = await f.evaluate(() => { const p = document.getElementById("files"); return !!p && p.getBoundingClientRect().width > 0; });
  await f.evaluate(() => { window.focus(); if (document.activeElement && document.activeElement.blur) document.activeElement.blur(); });
  await page.keyboard.press("/");
  await sleep(200);
  s = await state(f);
  check("keys: / with the Files pane shown opens the palette and keeps the focus in it", shown && s.open && s.focus === "tkqin" && !s.q.includes("/"), { shown, open: s.open, focus: s.focus, q: s.q });
  await closeAll(page, f);
  await f.evaluate(() => { const b = document.querySelector('[data-ctab="convo"]'); if (b) b.click(); });
}

async function asyncs(ctx) {
  const { page } = ctx;
  await page.setViewportSize({ width: 1240, height: 860 });
  const f = await frameOf(page);
  const cards = () => f.evaluate(() => document.querySelectorAll("[data-card]").length);
  await openKey(page, f);
  await typeQ(page, f, "palette");
  const n0 = await cards();
  await page.keyboard.press("Alt+Digit2");
  await page.keyboard.press("Control+Enter");
  await sleep(500);
  let s = await state(f);
  check("async: Ctrl+Enter on a file opens a preview card under the palette", (await cards()) > n0 && s.open && s.focus === "tkqin", { cards: await cards(), n0, open: s.open, focus: s.focus });
  await page.keyboard.press("Alt+Digit3");
  await page.keyboard.press("Control+Enter");
  await sleep(500);
  const line = await f.evaluate(() => [...document.querySelectorAll("[data-card]")].some((c) => /palette/.test(c.innerText)));
  s = await state(f);
  check("async: Ctrl+Enter on a content hit previews it with the palette open", line && s.open && s.focus === "tkqin", { line, open: s.open });
  await typeQ(page, f, "beta");
  s = await state(f);
  check("async: a tracker content hit from a store whose view did not load appears", /Beta palette note/.test(s.panes.tnote || ""), s.panes.tnote);
  await typeQ(page, f, "status:bogus palette");
  s = await state(f);
  check("async: a malformed query errs the tracker content slot only", /status/i.test(s.panes.tnote || "") && /Error/.test(s.panes.tnote || "") && !/Error/.test(s.panes.files || ""), { tnote: s.panes.tnote, files: s.panes.files });
  await closeAll(page, f);
}

async function pathMode(ctx) {
  const { page, listing } = ctx;
  await page.setViewportSize({ width: 1240, height: 860 });
  const f = await frameOf(page);
  await openKey(page, f);
  await typeQ(page, f, listing + "/");
  let s = await state(f);
  check("path: <dir>/ lists depth 1", ["food", "fo.txt", "bar.txt", "xfoy.md"].every((n) => (s.panes.files || "").includes(n)) && !(s.panes.files || "").includes("inner.txt"), s.panes.files);
  await typeQ(page, f, listing + "/fo");
  s = await state(f);
  const names = (s.panes.files || "").split("\n").filter((x) => /fo/.test(x));
  check("path: <dir>/fo filters, prefix first", !(s.panes.files || "").includes("bar.txt") && names[0] && names[0].startsWith("food"), s.panes.files);
  await page.keyboard.press("Alt+Digit2");
  await page.keyboard.press("Enter");
  await settle(f);
  s = await state(f);
  check("path: Enter on a folder completes the query", s.open && s.q === listing + "/food/", s.q);
  check("path: the folder lists its file", (s.panes.files || "").includes("inner.txt"), s.panes.files);
  await page.keyboard.press("Enter");
  await sleep(600);
  s = await state(f);
  const card = await f.evaluate(() => { const c = [...document.querySelectorAll("[data-card]")].find((c) => /inner\.txt/.test(c.innerText)); return c ? c.innerText : null; });
  check("path: Enter on a file closes the palette and previews it", !s.open && !!card, card);
  await closeAll(page, f);
}

async function look(ctx) {
  const { page, out, browser, listing } = ctx;
  fs.mkdirSync(out, { recursive: true });
  const shots = [];
  for (const W of [420, 820, 1240]) {
    await page.setViewportSize({ width: W, height: 860 });
    const f = await frameOf(page);
    for (const t of ["light", "dark"]) {
      await theme(page, f, t);
      await openKey(page, f);
      await typeQ(page, f, "");
      shots.push(await shot(page, out, `idle-${W}-${t}`));
      await typeQ(page, f, "palette");
      shots.push(await shot(page, out, `filled-${W}-${t}`));
      await closeAll(page, f);
    }
    await theme(page, f, "light");
  }
  // Streaming: hold fs/search so the content slot is caught dimmed, then a capped answer.
  const f = await frameOf(page);
  let hold = null;
  await page.context().route("**/fs/search?*", async (r) => { if (hold) { await hold; } await r.continue(); });
  for (const W of [820, 1240]) {
    await page.setViewportSize({ width: W, height: 860 });
    await openKey(page, f);
    await typeQ(page, f, "palette");
    let release; hold = new Promise((r) => { release = r; });
    await f.evaluate(() => { const i = document.getElementById("tkqin"); i.value = "palette rows"; i.dispatchEvent(new Event("input")); });
    await sleep(300);
    const st = await state(f);
    check(`look ${W}: streaming keeps the previous rows dimmed`, st.stale && st.spin, { stale: st.stale, spin: st.spin });
    shots.push(await shot(page, out, `streaming-${W}`));
    hold = null; release();
    await settle(f);
    shots.push(await shot(page, out, `done-${W}`));
    await closeAll(page, f);
  }
  await page.context().unroute("**/fs/search?*");
  await page.context().route("**/fs/search?*", (r) => r.fulfill({ contentType: "application/json", body: JSON.stringify({ files: Array.from({ length: 120 }, (_, i) => ({ path: `src/f${i}.go`, matches: [{ line: i + 1, pre: "a ", hit: "palette", post: " row" }] })), more: true }) }));
  await page.setViewportSize({ width: 1240, height: 860 });
  await openKey(page, f);
  await typeQ(page, f, "palette");
  check("look: a capped content answer shows the limit state", /Limit \(120\+\)/.test((await state(f)).text));
  shots.push(await shot(page, out, "cap-1240"));
  await page.context().unroute("**/fs/search?*");
  await typeQ(page, f, "status:bogus");
  shots.push(await shot(page, out, "error-1240"));
  await closeAll(page, f);
  for (const W of [820, 1240]) {
    await page.setViewportSize({ width: W, height: 860 });
    await openKey(page, f);
    await typeQ(page, f, listing + "/fo");
    shots.push(await shot(page, out, `path-${W}`));
    await closeAll(page, f);
  }
  // The approved mockup at its matching widths.
  if (fs.existsSync(MOCKUP)) {
    const mp = await browser.newPage({ viewport: { width: 1400, height: 900 } });
    await mp.goto("file://" + MOCKUP);
    for (const w of ["narrow", "medium", "wide"]) {
      await mp.click(`[data-w="${w}"]`);
      await mp.click('[data-q="palette"]');
      await sleep(1800);
      shots.push(await shot(mp, out, `mockup-${w}`));
    }
    await mp.close();
  }
  log(`LOOK captures: ${shots.length} in ${out}`);
}

const SCENARIOS = { layout, keys, async: asyncs, path: pathMode, look };

// --- main ------------------------------------------------------------------------
async function main() {
  const a = args();
  const only = a.only ? a.only.split(",") : Object.keys(SCENARIOS);
  const { chromium } = playwright();
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "palette-headless-"));
  let srv = null, browser = null;
  try {
    srv = await serve(tmp);
    log(`daemon pid ${srv.daemon.pid} on ${srv.base} (temp ${tmp})`);
    browser = await chromium.launch({ headless: !a.headed });
    const context = await browser.newContext({ viewport: { width: 1240, height: 860 } });
    // The beta store's view never loads, so its hits come from a store that is not loaded.
    await context.route("**/tracker/stores/beta/view*", (r) => r.abort());
    const page = await context.newPage();
    page.on("pageerror", (e) => { failures++; console.log(`  FAIL page error: ${e.message}`); });
    await page.goto(`${srv.base}/s/${srv.rid}`, { waitUntil: "load" });
    let f = await frameOf(page);
    await f.waitForSelector("#tkqs", { state: "attached", timeout: 15000 });
    await seedTracker(f);
    await page.reload({ waitUntil: "load" });
    f = await frameOf(page);
    await f.waitForSelector("#tkqs", { state: "attached", timeout: 15000 });
    await sleep(800);
    const ctx = { page, browser, out: a.out, listing: srv.listing };
    for (const name of only) {
      console.log(`${name}:`);
      try { await SCENARIOS[name](ctx); } catch (e) { failures++; console.log(`  FAIL ${name} threw: ${e.stack || e}`); }
    }
  } finally {
    if (browser) await browser.close();
    if (srv) {
      clearInterval(srv.timer);
      srv.daemon.kill("SIGTERM");
      await Promise.race([new Promise((r) => srv.daemon.once("exit", r)), sleep(5000)]);
      if (srv.daemon.exitCode === null) srv.daemon.kill("SIGKILL");
    }
    if (!a["keep-tmp"]) fs.rmSync(tmp, { recursive: true, force: true }); else log(`kept ${tmp}`);
  }
  console.log(failures ? `palette-headless: ${failures} failure(s)` : "palette-headless: all checks passed");
  process.exit(failures ? 1 : 0);
}

// seedTracker registers two local stores through the page (same origin):
// alpha with an item in progress and a comment, beta with one item.
async function seedTracker(f) {
  const res = await f.evaluate(async () => {
    async function call(p, body) {
      const r = await fetch(p, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      const t = await r.text();
      if (!r.ok) throw new Error(p + ": " + r.status + " " + t);
      return t ? JSON.parse(t) : {};
    }
    const out = [];
    for (const [id, label] of [["alpha", "Alpha"], ["beta", "Beta"]]) {
      out.push(await call("/tracker/register", { id, label, remote: "", member: "ada", profile: { given: "Ada" } }));
    }
    const a = await call("/tracker/stores/alpha/dispatch", { events: [{ type: "item.create", payload: { kind: "task", title: "Palette tracker task", body: "Track the palette rollout." } }] });
    out.push(a);
    const item = (a.ids || [])[0];
    out.push(await call("/tracker/stores/alpha/dispatch", { events: [
      { type: "item.status", payload: { item, status: "doing" } },
      { type: "comment.add", payload: { item, body: "The palette streaming states look right." } }] }));
    out.push(await call("/tracker/stores/beta/dispatch", { events: [{ type: "item.create", payload: { kind: "task", title: "Beta palette note", body: "A note in a store whose view is not loaded." } }] }));
    return out;
  });
  log(`tracker seeded: ${JSON.stringify(res).slice(0, 300)}`);
}

main().catch((e) => { console.error(`palette-headless: ${e.stack || e}`); process.exit(1); });

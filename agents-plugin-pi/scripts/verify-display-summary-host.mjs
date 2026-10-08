/** Separate offline acceptance: requires installed Pi 1.0.4, never silently skips.
 * Run: WS_PI_HOST_ROOT="$(npm root -g)/@earendil-works/pi-coding-agent" node scripts/verify-display-summary-host.mjs
 * No user settings, credentials, conversation, terminal, or provider is used.
 */
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { createRequire, findPackageJSON } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const root = process.env.WS_PI_HOST_ROOT;
assert.ok(root, "Set WS_PI_HOST_ROOT explicitly using the discovery command in this script");
const host = resolve(root);
const manifest = JSON.parse(await fs.readFile(join(host, "package.json"), "utf8"));
assert.equal(manifest.version, "1.0.4", "This acceptance contract targets installed Pi 1.0.4; do not upgrade dependencies to pass");
const hostImport = (path) => import(pathToFileURL(join(host, "dist", path)).href);
const sdk = await hostImport("index.js");
const loader = await hostImport("core/extensions/loader.js");
const { ToolExecutionComponent } = await hostImport("modes/interactive/components/tool-execution.js");
const { CustomMessageComponent } = await hostImport("modes/interactive/components/custom-message.js");
const themeModule = await hostImport("modes/interactive/theme/theme.js");
const requireHost = createRequire(join(host, "dist/index.js"));
const tui = await import(pathToFileURL(requireHost.resolve("@earendil-works/pi-tui")).href);
// pi-ai exposes import-only conditions; CommonJS require.resolve cannot select them.
const aiPackage = findPackageJSON("@earendil-works/pi-ai", pathToFileURL(join(host, "dist/index.js")));
assert.ok(aiPackage, "Host pi-ai package must be discoverable through ESM");
const aiManifest = JSON.parse(await fs.readFile(aiPackage, "utf8"));
const compatEntry = aiManifest.exports?.["./compat"]?.import;
assert.equal(typeof compatEntry, "string", "Installed host must expose its compatibility ESM entry");
const ai = await import(pathToFileURL(resolve(dirname(aiPackage), compatEntry)).href);
assert.equal(typeof sdk.ExtensionRunner.prototype.resolveToolRenderers, "function", "Public resolver API is required");
assert.equal(typeof sdk.AgentSessionRuntime, "function", "Public replacement lifecycle is required");
themeModule.initTheme("dark");
const theme = themeModule.theme;
const temp = await fs.mkdtemp(join(tmpdir(), "ws-summary-host-"));
const key = `ws-summary-offline-${process.pid}`;
const probes = globalThis[key] = new Map();
const runtimes = [];
const baseline = "a8ac507da37162a7a0e9349a344882d08c59e7cd";
const summary = { toolIntention: "SAVED INTENTION", toolResult: "SAVED RESULT" };
const text = (row, width = 80) => row.render(width).map(tui.stripTerminalSequences).join("\n");
const result = { content: [{ type: "text", text: "RAW RESULT" }], details: undefined, isError: false };
const model = ai.getModel("anthropic", "claude-haiku-4-5");
const message = (id = "message") => ({ role: "custom", customType: "ws-thread-summary", content: "RAW MESSAGE", details: { wsDisplaySummaryId: id }, display: true, timestamp: 0 });
// Read the real stamp rather than fabricating a metadata convention.
let summaryIdKey;

try {
  // Baseline comes from immutable repository source, not a hand-written imitation.
  const archive = join(temp, "baseline.tar");
  execFileSync("git", ["archive", "--output", archive, baseline, "agents-plugin-pi/src"], { cwd: resolve(packageRoot, "..") });
  await fs.mkdir(join(temp, "baseline"));
  execFileSync("tar", ["-xf", archive, "-C", join(temp, "baseline")]);
  // Share unchanged adapter dependencies; Pi's loader still aliases all host SDK/TUI modules.
  await fs.symlink(join(packageRoot, "node_modules"), join(temp, "baseline/agents-plugin-pi/node_modules"), "dir");

  async function probeFile(label, source, repaired) {
    const path = join(temp, `${label}.ts`);
    const imports = ["display-summary", "display-summary-render", "display-summary-session", "tool-result-render", "push-render", "write-scopes", "summary-id"];
    const aliases = ["storeModule", "render", "sessionModule", "tool", "push", "scopes", "ids"];
    const code = imports.map((name, i) => `import * as ${aliases[i]} from ${JSON.stringify(join(source, `${name}.ts`))};`).join("\n");
    await fs.writeFile(path, `${code}
import { loadHostPiTui } from ${JSON.stringify(join(source, "pi-tui.ts"))};
export default async function(pi) {
  const modules = await loadHostPiTui();
  const store = storeModule.createDisplaySummaryStore();
  let calls = 0;
  ${repaired ? `store.enabled = false;
  render.registerDisplaySummaryToolResolver(pi, store, modules);
  await render.registerAdapterMessageRenderers(pi, store, modules);
  await push.registerPushMessageRenderers(pi, modules, store);` : ""}
  const session = sessionModule.registerDisplaySummarySession(pi, {
    store, env: {}, readConfig: async () => ({ model: ${JSON.stringify(`${model.provider}/${model.id}`)} }),
    ${repaired ? "confirmBuiltinTools: (active) => render.confirmSummarizedBuiltinTools(store, active)," : `registerBuiltinWrappers: (cwd, active) => render.registerSummarizedBuiltinTools(pi, cwd, store, active, modules),
    registerMessageRenderers: () => { void render.registerAdapterMessageRenderers(pi, store, modules); },`}
    createCompletion: () => async () => { calls++; return { role: "assistant", content: [{type:"toolCall", id:"out", name:storeModule.DISPLAY_SUMMARY_OUTPUT_TOOL, arguments:{items:[{id:"t1", toolIntention:"FRESH INTENTION", toolResult:"FRESH RESULT"}]}}], stopReason:"toolUse" }; },
  });
  globalThis[${JSON.stringify(key)}].set(${JSON.stringify(label)}, { pi, store, render, session, storeModule, sessionModule, tool, push, scopes, ids, modules, calls: () => calls });
}
`);
    return path;
  }
  // Exercise the real production factory too: it must await and install
  // presentation before any session_start, without native execution overrides.
  const inherited = Object.fromEntries(Object.entries(process.env).filter(([name]) => name.startsWith("WS_PI_") || name === "WS_MAILBOX" || name === "WS_MAILBOX_AUTO"));
  let production;
  try {
    for (const name of Object.keys(inherited)) delete process.env[name];
    production = await loader.loadExtensions([join(packageRoot, "src/index.ts")], temp);
  } finally { Object.assign(process.env, inherited); }
  assert.deepEqual(production.errors, [], "Installed host must load the actual production factory offline");
  const productionExtension = production.extensions[0];
  assert.equal(productionExtension.toolRenderers.length, 1, "Presentation resolver exists before reconstruction/startup");
  for (const customType of ["ws-thread-summary", "ws-lead-compact", "ws-lead-context-milestone", "ws-agent-report", "ws-push-batch"]) assert.ok(productionExtension.messageRenderers.has(customType), "Custom presentation is ready before reconstruction");
  assert.equal(productionExtension.tools.has("edit"), false);
  assert.equal(productionExtension.tools.has("write"), false);
  console.log("PASS actual production factory: awaited early tool/message presentation, no native execution overrides");

  const currentPath = await probeFile("current", join(packageRoot, "src"), true);
  const baselinePath = await probeFile("baseline", join(temp, "baseline/agents-plugin-pi/src"), false);

  // Load through Pi's real extension loader and its host module aliases.
  const loaded = await loader.loadExtensions([currentPath, baselinePath], temp);
  assert.deepEqual(loaded.errors, [], "Host must load both adapters");
  const current = probes.get("current");
  const old = probes.get("baseline");
  summaryIdKey = current.ids.SUMMARY_ID_KEY;
  const makeMessage = (id = "message") => ({ ...message(id), details: { [summaryIdKey]: id } });
  const customMessages = () => [makeMessage(),
    { ...makeMessage("push"), customType: "ws-agent-report", content: "[ws-agent-report] agent offline\nreport: RAW PUSH", details: { [summaryIdKey]: "push", agent_id: "offline" } },
    { ...makeMessage("batch"), customType: "ws-push-batch", content: "RAW BATCH", details: { version: 1, items: [{ customType: "ws-agent-report", content: "[ws-agent-report] agent offline\nreport: RAW BATCH", details: { [summaryIdKey]: "batch-item", agent_id: "offline" }, display: true }] } },
  ];
  const awaitReplay = (store) => new Promise((done, reject) => {
    if (store.get("saved-2")) return done();
    const timeout = setTimeout(() => reject(new Error("late ownership replay did not repaint")), 5000);
    store.requestRender = () => { if (store.get("saved-1") && store.get("saved-2")) { clearTimeout(timeout); done(); } };
  });
  assert.equal(current.modules.Text, tui.Text, "Use the host's TUI instance");
  assert.ok(loaded.extensions[0].toolRenderers.length, "registerToolRenderer must be accepted by the real host API");

  async function fixture(name) {
    const path = join(temp, `${name}.jsonl`);
    const names = ["edit", "ws__tickets_query", "do-i-really-have-to-read-this-myself"];
    const entries = names.map((toolName, i) => ({ type: "message", id: `entry-${i}`, parentId: i ? `entry-${i - 1}` : null, timestamp: "2026-10-08T00:00:00.000Z", message: { role: "toolResult", toolCallId: `saved-${i}`, toolName, ...result, timestamp: 0 } }));
    for (const [index, custom] of customMessages().entries()) entries.push({ ...custom, type: "custom_message", id: `message-entry-${index}`, parentId: index ? `message-entry-${index - 1}` : "entry-2", timestamp: "2026-10-08T00:00:00.000Z" });
    await fs.writeFile(path, [JSON.stringify({ type: "session", version: 3, id: name, cwd: temp, timestamp: "2026-10-08T00:00:00.000Z" }), ...entries.map((entry) => JSON.stringify(entry))].join("\n") + "\n");
    await fs.writeFile(path + ".ws-display-summaries.log", ["saved-0", "saved-1", "saved-2", "message", "push", "batch-item"].map((id) => JSON.stringify({ version: 1, sessionId: name, id, summary })).join("\n") + "\n");
    return path;
  }

  async function createRuntime(path, extensionPath) {
    const agentDir = join(temp, "agent-dir");
    await fs.mkdir(agentDir, { recursive: true });
    let extensions;
    const resourceLoader = {
      async reload() { extensions = await loader.loadExtensions([extensionPath], temp); assert.deepEqual(extensions.errors, []); },
      getExtensions: () => extensions,
      getSkills: () => ({ skills: [], diagnostics: [] }), getPrompts: () => ({ prompts: [], diagnostics: [] }), getThemes: () => ({ themes: [], diagnostics: [] }),
      getAgentsFiles: () => ({ agentsFiles: [] }), getSystemPrompt: () => "Offline summary lifecycle verification", getSystemPromptSource: () => undefined,
      getAppendSystemPrompt: () => [], getAppendSystemPromptSources: () => [], extendResources() {},
    };
    await resourceLoader.reload();
    const { session } = await sdk.createAgentSession({ cwd: temp, agentDir, resourceLoader, sessionManager: sdk.SessionManager.open(path), model, tools: ["edit", "write", "grep"] });
    return { session, services: { cwd: temp, agentDir }, diagnostics: [] };
  }
  function mount(session, probe) {
    const mounted = [];
    for (const [index, name] of ["edit", "ws__tickets_query", "do-i-really-have-to-read-this-myself"].entries()) {
      const base = () => session.getToolDefinition(name) ?? ({ edit: sdk.createEditToolDefinition, grep: sdk.createGrepToolDefinition }[name]?.(temp));
      const presentation = session.extensionRunner.resolveToolRenderers(name, base);
      const row = new ToolExecutionComponent(name, `saved-${index}`, { path: "synthetic" }, { showImages: false }, presentation, { requestRender() {} }, temp);
      row.updateResult(result, false);
      assert.doesNotMatch(text(row), /SAVED RESULT/, "old row starts raw before session_start");
      mounted.push(row);
    }
    const customs = customMessages().map((custom) => new CustomMessageComponent(custom, session.extensionRunner.getMessageRenderer(custom.customType)));
    for (const custom of customs) assert.match(text(custom), /RAW (MESSAGE|PUSH|BATCH)/);
    return { mounted, customs, probe };
  }
  function registerLate(probe) {
    const ref = { current: probe.modules, summaries: probe.store };
    for (const name of ["ws__tickets_query", "do-i-really-have-to-read-this-myself"]) {
      probe.tool.registerWsTool(probe.pi, { name, label: name, description: "offline", parameters: { type: "object", properties: {} }, execute: async () => result }, ref);
    }
  }

  for (const [label, extensionPath, repaired] of [["baseline", baselinePath, false], ["current", currentPath, true]]) {
    const initial = await createRuntime(await fixture(`${label}-initial`), extensionPath);
    const runtime = new sdk.AgentSessionRuntime(initial.session, initial.services, async ({ sessionManager }) => createRuntime(sessionManager.getSessionFile(), extensionPath));
    runtimes.push(runtime);
    let view = mount(runtime.session, probes.get(label));
    const bind = async (session) => { view = mount(session, probes.get(label)); await session.bindExtensions({ mode: "tui", uiContext: session.extensionRunner.getUIContext() }); registerLate(view.probe); await session.extensionRunner.emit({ type: "agent_end", messages: [] }); };
    await runtime.session.bindExtensions({ mode: "tui", uiContext: runtime.session.extensionRunner.getUIContext() });
    registerLate(view.probe);
    await runtime.session.extensionRunner.emit({ type: "agent_end", messages: [] });
    // Exercise actual AgentSession.reload's beforeSessionStart callback, not a mocked order.
    await runtime.session.reload({ beforeSessionStart: () => { view = mount(runtime.session, probes.get(label)); } });
    registerLate(view.probe);
    // Use a bounded observable wait, not timing guesses, for late ownership repaint.
    if (repaired) await awaitReplay(view.probe.store);
    for (const [index, row] of view.mounted.entries()) {
      assert.equal(/SAVED RESULT/.test(text(row)), repaired, `${label}: rebuilt tool row restoration`);
      row.setExpanded(true);
      assert.doesNotMatch(text(row), /SAVED RESULT/, "expanded never shows cached summary");
      // Native edit has no output without diff details; that is baseline behavior.
      assert.match(text(row), index === 0 ? /edit synthetic/ : /RAW RESULT/, "expanded remains native/raw");
      row.setExpanded(false);
    }
    for (const custom of view.customs) {
      assert.equal(/SAVED RESULT/.test(text(custom)), repaired, `${label}: rebuilt adapter/push/batch restoration`);
      custom.setExpanded(true); assert.match(text(custom), /RAW (MESSAGE|PUSH|BATCH)/);
      custom.setExpanded(false);
    }
    assert.equal(view.probe.calls(), 0, "old rows never request provider generation");
    const previous = view;
    let outgoingInvalidations = 0;
    previous.probe.store.trackInvalidate("saved-1", () => { outgoingInvalidations++; });
    runtime.setRebindSession(bind); // Host renders before binding the replacement session.
    await runtime.switchSession(await fixture(`${label}-replacement`));
    if (repaired) {
      await awaitReplay(view.probe.store);
      for (const row of view.mounted) assert.match(text(row), /SAVED RESULT/);
      for (const custom of view.customs) assert.match(text(custom), /SAVED RESULT/);
      assert.equal(outgoingInvalidations, 0);
      previous.probe.store.notify(["saved-1"]);
      assert.equal(outgoingInvalidations, 0, "outgoing repaint subscription retired before replacement");
      // Fresh rows are separate from restoration and still use the offline model seam.
      const fresh = new ToolExecutionComponent("ws__tickets_query", "fresh", {}, {}, runtime.session.extensionRunner.resolveToolRenderers("ws__tickets_query", () => runtime.session.getToolDefinition("ws__tickets_query")), { requestRender() {} }, temp);
      fresh.updateResult(result, false);
      assert.match(text(fresh), /RAW RESULT/);
      view.probe.session.current().observeToolStart("fresh", "ws__tickets_query", {});
      view.probe.session.current().observeToolEnd("fresh", "ws__tickets_query");
      await view.probe.session.current().flush();
      assert.match(text(fresh), /FRESH RESULT/);
      assert.equal(view.probe.calls(), 1);
    }
    const cold = await createRuntime(await fixture(`${label}-cold`), extensionPath);
    const coldRuntime = new sdk.AgentSessionRuntime(cold.session, cold.services, async ({ sessionManager }) => createRuntime(sessionManager.getSessionFile(), extensionPath));
    runtimes.push(coldRuntime);
    const coldView = mount(cold.session, probes.get(label));
    // Rows expanded before session_start are never automatically collapsed by replay.
    coldView.mounted[0].setExpanded(true); coldView.customs[0].setExpanded(true);
    await cold.session.bindExtensions({ mode: "tui", uiContext: cold.session.extensionRunner.getUIContext() });
    registerLate(coldView.probe);
    if (repaired) await awaitReplay(coldView.probe.store);
    assert.doesNotMatch(text(coldView.mounted[0]), /SAVED RESULT/);
    assert.match(text(coldView.customs[0]), /RAW MESSAGE/);
    assert.equal(/SAVED RESULT/.test(text(coldView.mounted[1])), repaired, "cold continuation restores bridge rows only with confirmed ownership");
    assert.equal(coldView.probe.calls(), 0);
    console.log(`PASS ${label}: actual reload/replacement and cold continuation, reconstruction-before-start, native/ws/one-liner/adapter/push/batch rows, old vs fresh and pre-expanded rows`);
  }

  // Failure comparison uses actual installed components; faults are isolated and synthetic.
  function renderers(probe, store, modules, raw, shell = "default") {
    return probe.render.wrapToolRenderersWithSummary("grep", raw.renderCall, raw.renderResult, store, modules, { selfFramed: shell === "self", padX: shell === "self" ? 1 : 0 });
  }
  const raw = { renderCall: () => new tui.Text("RAW CALL"), renderResult: () => new tui.Text("RAW RESULT") };
  for (const fault of ["construction", "render", "invalidate"]) {
    class FaultText extends tui.Text {
      constructor(...args) { if (fault === "construction") throw new Error("summary construction fault"); super(...args); }
      render(width) { if (fault === "render") throw new Error("summary delayed fault"); return super.render(width); }
      invalidate() { if (fault === "invalidate") throw new Error("summary invalidation fault"); super.invalidate(); }
    }
    for (const probe of [old, current]) {
      const store = probe.storeModule.createDisplaySummaryStore(); store.set("fault", summary);
      const row = new ToolExecutionComponent("grep", "fault", {}, {}, renderers(probe, store, { ...tui, Text: FaultText }, raw), { requestRender() {} }, temp);
      row.updateResult(result, false);
      const action = () => { row.render(30); if (fault === "invalidate") row.invalidate(); return row.render(30); };
      if (probe === current || fault === "construction") { assert.doesNotThrow(action); assert.match(text(row, 30), /RAW CALL|RAW RESULT/); }
      else assert.throws(action, /summary (delayed|invalidation) fault/, "pre-existing unguarded component fault recorded, not fabricated safety");
      const rawMessage = new tui.Text("RAW MESSAGE");
      const switchRow = probe.render.createSummarySwitch(store, "fault", rawMessage, () => new FaultText("SUMMARY"));
      const customRow = new CustomMessageComponent(makeMessage("fault"), () => switchRow);
      const switchAction = () => { customRow.render(30); if (fault === "invalidate") customRow.invalidate(); return customRow.render(30); };
      if (probe === current) { assert.doesNotThrow(switchAction); assert.match(text(customRow, 30), /RAW MESSAGE/); }
      else assert.throws(switchAction, /summary/);
    }
  }
  console.log("PASS baseline/current: summary construction, delayed render and invalidation fault comparison");

  // Use the real host API to compose our resolver with another extension.
  const eventBus = (await hostImport("core/event-bus.js")).createEventBus();
  async function resolverRunner(store, modules, after = []) {
    const runtime = sdk.createExtensionRuntime();
    const ours = await loader.loadExtensionFromFactory((pi) => {
      assert.equal(typeof pi.registerToolRenderer, "function");
      current.render.registerDisplaySummaryToolResolver(pi, store, modules);
    }, temp, eventBus, runtime);
    return new sdk.ExtensionRunner([ours, ...after], runtime, temp, sdk.SessionManager.inMemory(temp), {});
  }
  const store = current.storeModule.createDisplaySummaryStore();
  store.confirmTool("grep"); store.set("guard", summary);
  let composed = 0;
  const other = await loader.loadExtensionFromFactory((pi) => pi.registerToolRenderer((_name, next) => { composed++; return next() ?? raw; }), temp, eventBus, sdk.createExtensionRuntime());
  const chain = await resolverRunner(store, tui, [other]);
  const definition = chain.resolveToolRenderers("grep", () => undefined);
  const row = new ToolExecutionComponent("grep", "guard", {}, {}, definition, { requestRender() {} }, temp);
  row.updateResult(result, false);
  assert.match(text(row), /SAVED RESULT/); assert.ok(composed > 0);
  row.setExpanded(true); assert.match(text(row), /RAW RESULT/);
  const brokenNext = await resolverRunner(store, tui);
  const unguardedRunner = new sdk.ExtensionRunner([], sdk.createExtensionRuntime(), temp, sdk.SessionManager.inMemory(temp), {});
  const throwsNext = () => { throw new Error("downstream resolver fault"); };
  assert.throws(() => unguardedRunner.resolveToolRenderers("grep", throwsNext), /downstream resolver fault/, "baseline host does not catch resolver faults");
  assert.equal(brokenNext.resolveToolRenderers("grep", throwsNext), undefined, "our new resolver degrades to host raw instead of adding escaping faults");
  const malformedDefinition = { get renderCall() { throw new Error("malformed presentation metadata"); } };
  assert.equal(brokenNext.resolveToolRenderers("grep", () => malformedDefinition), undefined);
  const noApi = current.render.registerDisplaySummaryToolResolver({}, store, tui);
  assert.equal(noApi, false);
  for (const modules of [undefined, {}, { Text: null }]) {
    const renderer = await resolverRunner(store, modules);
    const rawRow = new ToolExecutionComponent("grep", "guard", {}, {}, renderer.resolveToolRenderers("grep", () => raw), { requestRender() {} }, temp);
    rawRow.updateResult(result, false); assert.match(text(rawRow), /RAW RESULT/);
  }
  const notActive = current.storeModule.createDisplaySummaryStore(); notActive.enabled = false; notActive.set("guard", summary);
  for (const [source, context] of [[notActive, { toolCallId: "guard" }], [undefined, { toolCallId: "guard" }], [() => { throw new Error("store not ready"); }, { toolCallId: "guard" }], [store, undefined], [store, {}]]) {
    const wrapper = current.render.wrapToolRenderersWithSummary("grep", raw.renderCall, raw.renderResult, source, tui);
    assert.match(wrapper.renderCall({}, theme, context).render(30).join("\n"), /RAW CALL/);
  }
  for (const probe of [old, current]) {
    for (const [source, modules, context] of [[undefined, tui, { toolCallId: "guard" }], [store, undefined, { toolCallId: "guard" }], [store, tui, {}]]) {
      const wrapper = probe.render.wrapToolRenderersWithSummary("grep", raw.renderCall, raw.renderResult, source, modules);
      assert.match(wrapper.renderCall({}, theme, context).render(30).join("\n"), /RAW CALL/, "baseline/current readiness and absent metadata remain raw");
    }
    const downstreamRaw = { renderCall: raw.renderCall, renderResult: () => ({ render() { throw new Error("downstream raw render fault"); }, invalidate() {} }) };
    const rawOnly = new ToolExecutionComponent("grep", "uncached", {}, {}, renderers(probe, undefined, tui, downstreamRaw), { requestRender() {} }, temp);
    rawOnly.updateResult(result, false);
    assert.throws(() => rawOnly.render(30), /downstream raw render fault/, "pre-existing downstream raw errors remain outside adapter safety promise");
  }
  const unknown = new ToolExecutionComponent("foreign_mcp", "guard", {}, {}, chain.resolveToolRenderers("foreign_mcp", () => raw), { requestRender() {} }, temp);
  unknown.updateResult(result, false); assert.match(text(unknown), /RAW RESULT/); assert.doesNotMatch(text(unknown), /SAVED RESULT/);
  for (const bad of [{ toolIntention: "", toolResult: "fake" }, { toolIntention: "fake", toolResult: null }, { ...summary, optionalContext: 4 }]) {
    const invalid = current.storeModule.createDisplaySummaryStore(); invalid.set("guard", bad);
    const wrapper = renderers(current, invalid, tui, raw);
    assert.match(wrapper.renderCall({}, theme, { toolCallId: "guard" }).render(30).join("\n"), /RAW CALL/);
    const custom = current.render.createSummarySwitch(invalid, "guard", new tui.Text("RAW MESSAGE"), () => { assert.fail("malformed summaries must stay raw"); });
    assert.match(custom.render(30).join("\n"), /RAW MESSAGE/);
  }
  const deferred = { current: undefined };
  const delayed = await resolverRunner(store, deferred);
  const lateRow = new ToolExecutionComponent("grep", "guard", {}, {}, delayed.resolveToolRenderers("grep", () => raw), { requestRender() {} }, temp);
  lateRow.updateResult(result, false); assert.match(text(lateRow), /RAW RESULT/);
  deferred.current = tui; store.notify(["guard"]); assert.match(text(lateRow), /SAVED RESULT/);
  const emptySlots = new ToolExecutionComponent("grep", "guard", {}, {}, (await resolverRunner(store, { ...tui, Text: class { constructor() { throw new Error("no summary"); } } })).resolveToolRenderers("grep", () => undefined), { requestRender() {} }, temp);
  emptySlots.updateResult(result, false); assert.doesNotThrow(() => emptySlots.render(12));
  assert.equal(await current.render.registerAdapterMessageRenderers({ registerMessageRenderer() { assert.fail("incompatible TUI must not register"); } }, store, {}), false);
  console.log("PASS public resolver: chain composition, missing/throwing delegation, missing/delayed/incompatible TUI, store and metadata readiness, missing slots");

  // Raw reuse must survive raw -> summary -> expanded, without passing guards to native setText.
  for (const shell of ["default", "self"]) {
    let reused = 0;
    const native = { renderCall: (_args, _theme, ctx) => { if (ctx.lastComponent) { assert.ok(ctx.lastComponent instanceof tui.Text); reused++; ctx.lastComponent.setText("RAW CALL"); return ctx.lastComponent; } return new tui.Text("RAW CALL"); }, renderResult: raw.renderResult };
    const values = current.storeModule.createDisplaySummaryStore(); values.confirmTool("grep");
    const wrap = renderers(current, values, tui, native, shell);
    const rawRow = new ToolExecutionComponent("grep", "reuse", {}, {}, { ...wrap, renderShell: shell }, { requestRender() {} }, temp); rawRow.updateResult(result, false);
    const before = rawRow.render(30);
    const baselineFrame = new ToolExecutionComponent("grep", "frame-baseline", {}, {}, { ...native, renderShell: shell }, { requestRender() {} }, temp);
    baselineFrame.updateResult(result, false);
    values.set("reuse", summary); values.notify(["reuse"]);
    for (const width of [1, 8, 16, 30, 80]) {
      // The host's default Box has a minimum inner column plus two margins at width 1.
      // Compare that baseline limit rather than claiming to repair the host frame itself.
      const limit = Math.max(width, ...baselineFrame.render(width).map(tui.visibleWidth));
      for (const line of rawRow.render(width)) assert.ok(tui.visibleWidth(line) <= limit, `${shell} preserves baseline width ${width}`);
    }
    rawRow.setExpanded(true); assert.deepEqual(rawRow.render(30), before); assert.ok(reused);
  }
  const childStore = current.storeModule.createDisplaySummaryStore();
  const runtime = sdk.createExtensionRuntime();
  const child = await loader.loadExtensionFromFactory((pi) => {
    current.scopes.registerScopedWriteTools(pi, { mode: "scoped", scopes: [{ path: join(temp, "allowed.md"), kind: "file" }] });
    current.sessionModule.registerDisplaySummarySession(pi, { store: childStore, env: { WS_PI_SPAWN_ROLE: "worker" }, readConfig: async () => ({}), confirmBuiltinTools() { assert.fail("child cannot confirm lead native ownership"); } });
  }, temp, eventBus, runtime);
  const childRunner = new sdk.ExtensionRunner([child], runtime, temp, sdk.SessionManager.inMemory(temp), {});
  const scopedWrite = childRunner.getToolDefinition("write");
  await assert.rejects(scopedWrite.execute("denied", { path: join(temp, "denied.md"), content: "must not write" }, undefined, undefined, { cwd: temp }), /outside|not authorized|scope/i);
  childRunner.setUIContext(undefined, "tui");
  await childRunner.emit({ type: "session_start" });
  assert.equal((child.toolRenderers ?? []).length, 0, "no lead presentation resolver shadows child execution");
  assert.equal(childStore.toolNames.size, 0);
  console.log("PASS raw reuse, default/self frame width parity, child scoped-write denial; no provider calls or live terminal probes");
} finally {
  for (const runtime of runtimes) await runtime.dispose();
  delete globalThis[key];
  await fs.rm(temp, { recursive: true, force: true });
}
console.log(`Installed-host acceptance PASS: Pi ${manifest.version}; live reload/resume acceptance remains user-pending.`);

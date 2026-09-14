import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { createEventBus, discoverAndLoadExtensions, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { registerAgentTools } from "../src/spawner.ts";
import { registerExecuteGateway } from "../src/execute-gateway.ts";
import { registerFork } from "../src/fork.ts";
import { startBridge } from "../src/bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolResultTuiModules } from "../src/tool-result-render.ts";
import { DELEGATION_ENV } from "../src/delegation-policy.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";

type Captured = { name: string; parameters?: { properties?: Record<string, unknown> }; execute?: (...args: never[]) => unknown; renderCall?: (...args: never[]) => unknown; renderResult?: (...args: never[]) => unknown };

class Text {
  private text = "";
  setText(text: string): void { this.text = text; }
  render(): string[] { return [this.text]; }
  invalidate(): void {}
}
class Box {
  private children: Array<{ render(width: number): string[] }> = [];
  addChild(child: { render(width: number): string[] }): void { this.children.push(child); }
  setBgFn(): void {}
  render(width: number): string[] { return this.children.flatMap((child) => child.render(width)); }
  invalidate(): void {}
}
const tui: ToolResultTuiModules = {
  Text, Box, stripTerminalSequences: (text) => text, truncateToWidth: (text) => text,
};

function harness() {
  const tools = new Map<string, Captured>();
  const pi = {
    registerTool: (definition: Captured) => tools.set(definition.name, definition),
    sendMessage() {}, sendUserMessage() {}, on() {},
  } as unknown as ExtensionAPI;
  const bridge = { client: { callTool: async () => ({ isError: false, content: [{ type: "text", text: "{}" }] }) }, wsToolNames: [], defaultSessionKeyRef: { current: undefined } } as never;
  return { tools, pi, bridge };
}

/** These invoke production registration factories; the compact helper test alone cannot catch missing shared refs. */
test("actual native registrations retain schemas/executors while sharing cold and late-filled preview refs", () => {
  const { tools, pi, bridge } = harness();
  const ref = createToolPreviewTuiRef();
  const agents = registerAgentTools(pi, bridge, { cwd: "/tmp" }, undefined, "/tmp/explore.md", ref);
  registerExecuteGateway(pi, bridge, agents.rpcRegistry, { cwd: "/tmp", executeWorkerPromptPath: "/tmp/execute.md" }, ref);
  registerFork(pi, bridge, agents.rpcRegistry, { cwd: "/tmp" }, undefined, ref);

  for (const name of [
    "ws-agent-send",
    "ws-agent-spawn",
    "explore",
    "ws-approve",
    "ws-execute",
    "ws-fork",
    "do-i-really-have-to-read-this-myself",
    "do-i-really-have-to-run-this-myself",
  ]) {
    const tool = tools.get(name);
    assert.ok(tool?.execute, `${name} remains an executable production registration`);
    assert.ok(tool?.parameters, `${name} retains its production schema`);
    assert.ok(tool?.renderCall && tool?.renderResult, `${name} receives shared presentation hooks`);
    assert.throws(() => tool!.renderCall!({ model: "caller-only", effort: "high" }, {}, { state: {}, argsComplete: true, isPartial: false }), "factory-time cold ref preserves Pi fallback");
  }
  const send = tools.get("ws-agent-send")!;
  assert.ok("agent_id" in (send.parameters!.properties ?? {}), "ws-agent-send schema is preserved");

  ref.current = tui;
  // 260906 Phase 2: ws-agent-send now owns its own custom renderCall (target
  // agent + message head), not the generic raw-arg YAML dump — this proves
  // the SAME captured closure sees the late-filled ref for its own summary.
  const rendered = send.renderCall!({ agent_id: "agent-1", message: "hello there" }, { bold: (x: string) => x, fg: (_: string, x: string) => x }, { state: {}, argsComplete: true, isPartial: false }) as { render(width: number): string[] };
  const renderedText = rendered.render(120).join("\n");
  assert.match(renderedText, /target: agent-1/, "ws-agent-send's own custom summary renders the target agent");
  assert.match(renderedText, /message: hello there/);
  agents.stopAll();
});

test("260906 Phase 1: the two direct tools cap their real registered OUTPUT preview at ten logical lines, not ten physical rows", () => {
  const { tools, pi, bridge } = harness();
  const ref = createToolPreviewTuiRef();
  const agents = registerAgentTools(pi, bridge, { cwd: "/tmp" }, undefined, "/tmp/explore.md", ref);
  registerExecuteGateway(pi, bridge, agents.rpcRegistry, { cwd: "/tmp", executeWorkerPromptPath: "/tmp/execute.md" }, ref);
  ref.current = tui;

  const theme = { bold: (x: string) => x, fg: (_: string, x: string) => x };
  // Review relay cycle 1: the fixture MUST include a single logical line
  // that wraps past ten physical rows on its own (mirroring the pure-helper
  // test "260906 Phase 1: lineBudget 'logical' lets a single long logical
  // line render past ten physical rows" at test/tool-result-render.test.ts).
  // 11 short single-row lines cannot discriminate `resultLineBudget:
  // "logical"` from the old physical-row default — both stop at row/line 10
  // and append the same marker for that shape. With a long first logical
  // line, the OLD physical-row cap (10 rows total) would cut TAILMARKEND
  // off mid-line-0 and never reach line-1..line-9 at all; only the NEW
  // logical-line cap renders line-0 in full (however many physical rows it
  // takes) before counting it as one logical line toward the ten-line budget.
  const longFirstLine = `${"x".repeat(900)}TAILMARKEND`;
  const shortLines = Array.from({ length: 9 }, (_, index) => `line-${index + 1}`);
  const eleven = [longFirstLine, ...shortLines, "overflow-marker-line"].join("\n");
  for (const name of ["do-i-really-have-to-read-this-myself", "do-i-really-have-to-run-this-myself"]) {
    const tool = tools.get(name)!;
    const collapsed = tool.renderResult!(
      { content: [{ type: "text", text: eleven }] },
      { expanded: false, isPartial: false },
      theme,
      { state: {}, argsComplete: true, isPartial: false },
    ) as { render(width: number): string[] };
    const collapsedLines = collapsed.render(80).join("\n");
    assert.match(collapsedLines, /TAILMARKEND/, `${name}: the long first logical line renders in full, not cut mid-wrap`);
    for (let index = 1; index <= 9; index += 1) assert.match(collapsedLines, new RegExp(`line-${index}\\b`), `${name}: logical line ${index} survives the collapse`);
    assert.doesNotMatch(collapsedLines, /overflow-marker-line/, `${name}: the 11th logical line is cut`);
    assert.match(collapsedLines, /\.\.\./, `${name}: an overflow marker is shown`);

    const expanded = tool.renderResult!(
      { content: [{ type: "text", text: eleven }] },
      { expanded: true, isPartial: false },
      theme,
      { state: {}, argsComplete: true, isPartial: false, lastComponent: collapsed },
    ) as { render(width: number): string[] };
    const expandedLines = expanded.render(80).join("\n");
    assert.match(expandedLines, /TAILMARKEND/, `${name}: expansion keeps the long first logical line`);
    for (let index = 1; index <= 9; index += 1) assert.match(expandedLines, new RegExp(`line-${index}\\b`), `${name}: expansion recovers logical line ${index}`);
    assert.match(expandedLines, /overflow-marker-line/, `${name}: expansion recovers the 11th logical line`);
  }
  agents.stopAll();
});

test("actual MCP startup registers through the same cold then late-filled shared ref", async () => {
  const { tools, pi } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-mcp-preview-"));
  const launcher = join(dir, "fake-mcp.py");
  writeFileSync(launcher, `import json, sys\nfor line in sys.stdin:\n req=json.loads(line); method=req['method']; result={'serverInfo':{'version':'0.46.1'}} if method=='initialize' else {'tools':[{'name':'git.status','description':'status','inputSchema':{'type':'object','properties':{},'required':[]}}]} if method=='tools/list' else {'isError':True,'content':[{'type':'text','text':'no bootstrap'}]}; print(json.dumps({'jsonrpc':'2.0','id':req['id'],'result':result}), flush=True)\n`);
  const ref = createToolPreviewTuiRef();
  const oldRole = process.env.WS_PI_SPAWN_ROLE;
  process.env.WS_PI_SPAWN_ROLE = "worker";
  try {
    const handle = await startBridge(pi, { launcherPath: launcher, pluginDir: dir, runtimeJsonPath: join(dirname(fileURLToPath(import.meta.url)), "../runtime.json"), cwd: dir, toolPreviewTuiRef: ref });
    const tool = tools.get("ws__git_status")!;
    assert.ok(tool.renderCall && tool.renderResult, "the real MCP loop supplies hooks before the ref is filled");
    assert.throws(() => tool.renderCall!({}, {}, { state: {}, argsComplete: true, isPartial: false }), "cold startup remains native fallback");
    ref.current = tui;
    const call = tool.renderCall!({}, { bold: (x: string) => x, fg: (_: string, x: string) => x }, { state: {}, argsComplete: true, isPartial: false }) as { render(width: number): string[] };
    assert.match(call.render(80).join("\n"), /ws__git_status/, "the same captured MCP registration sees the late-filled ref");
    handle.shutdown();
  } finally {
    if (oldRole === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = oldRole;
  }
});

test("a child-role Pi extension load installs executable same-name scoped edit/write overrides", async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-scoped-loader-")));
  const previousPolicy = process.env[DELEGATION_ENV];
  const previousRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  try {
    const target = join(root, "result.md");
    process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
    process.env[DELEGATION_ENV] = JSON.stringify({
      version: 1, depth: 1, maxDepth: 2, authority: "delegate", tools: ["read", "edit", "write"],
      write: { mode: "scoped", scopes: [{ path: target, kind: "file" }] },
    });
    const extensionPath = fileURLToPath(new URL("../src/index.ts", import.meta.url));
    const loaded = await discoverAndLoadExtensions([extensionPath], root, join(root, "agent"), createEventBus());
    assert.deepEqual(loaded.errors, []);
    const extension = loaded.extensions.find(candidate => candidate.resolvedPath === extensionPath);
    assert.ok(extension, "the real Pi loader loaded the child extension entry");
    const edit = extension.tools.get("edit")?.definition;
    const write = extension.tools.get("write")?.definition;
    assert.ok(edit?.execute && write?.execute, "same-name native edit/write overrides are registered on the child surface");
    const ctx = { cwd: root } as never;
    await write.execute("write", { path: target, content: "before\n" }, undefined, undefined, ctx);
    await edit.execute("edit", { path: target, edits: [{ oldText: "before", newText: "after" }] }, undefined, undefined, ctx);
    assert.equal(readFileSync(target, "utf8"), "after\n");
    const outside = join(root, "outside.md");
    await assert.rejects(write.execute("deny", { path: outside, content: "no" }, undefined, undefined, ctx), /outside delegated write scopes/);
    assert.equal(existsSync(outside), false);
    assert.equal(extension.tools.has("bash"), false, "the scope registration does not add shell or a parallel write vocabulary");
  } finally {
    if (previousPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previousPolicy;
    if (previousRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = previousRole;
    rmSync(root, { recursive: true, force: true });
  }
});

test("extension factory no longer registers the retired ws-discuss command", async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-command-loader-")));
  const previousRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  const previousPolicy = process.env[DELEGATION_ENV];
  try {
    delete process.env[WS_PI_SPAWN_ROLE_ENV];
    delete process.env[DELEGATION_ENV];
    const extensionPath = fileURLToPath(new URL("../src/index.ts", import.meta.url));
    const loaded = await discoverAndLoadExtensions([extensionPath], root, join(root, "agent"), createEventBus());
    assert.deepEqual(loaded.errors, []);
    const extension = loaded.extensions.find(candidate => candidate.resolvedPath === extensionPath);
    assert.ok(extension, "the real Pi loader loaded the adapter entry");
    assert.equal(extension.commands.has("ws-discuss"), false, "the retired command is absent from the registered command surface");
    assert.ok(extension.commands.has("ws-model-catalog-list"), "unrelated extension commands remain registered");
  } finally {
    if (previousRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = previousRole;
    if (previousPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previousPolicy;
    rmSync(root, { recursive: true, force: true });
  }
});

test("representative bridged MCP definition uses the same late-filled ref without mutating its schema or executor", () => {
  const { tools, pi } = harness();
  const ref = createToolPreviewTuiRef();
  const execute = () => ({ content: [{ type: "text", text: "raw result" }] });
  const parameters = { type: "object", properties: { session_key: { type: "string" } }, required: [] };
  registerWsTool(pi, { name: "ws__git_status", description: "MCP representative", parameters: parameters as never, execute: execute as never } as never, ref);
  const registered = tools.get("ws__git_status")!;
  assert.equal(registered.parameters, parameters, "MCP schema identity is retained");
  assert.equal(registered.execute, execute, "MCP dispatch identity is retained");
  assert.throws(() => registered.renderResult!({ content: [{ type: "text", text: "raw result" }] }, { expanded: false, isPartial: false }, {}, { state: {}, isPartial: false }));
  ref.current = tui;
  const result = registered.renderResult!({ content: [{ type: "text", text: "raw result" }] }, { expanded: false, isPartial: false }, { fg: (_: string, x: string) => x }, { state: {}, isPartial: false }) as { render(width: number): string[] };
  assert.match(result.render(120).join("\n"), /raw result$/, "RAW result reaches the shared display-only layout");
});

import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { registerAgentTools } from "../src/spawner.ts";
import { registerExecuteGateway } from "../src/execute-gateway.ts";
import { registerFork } from "../src/fork.ts";
import { startBridge } from "../src/bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolResultTuiModules } from "../src/tool-result-render.ts";

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
  const agents = registerAgentTools(pi, bridge, { cwd: "/tmp" }, undefined, async () => ({ agentId: "leaf", state: "done", output: "ok" }), "/tmp/explore.md", ref);
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

test("actual MCP startup registers through the same cold then late-filled shared ref", async () => {
  const { tools, pi } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-mcp-preview-"));
  const launcher = join(dir, "fake-mcp.py");
  writeFileSync(launcher, `import json, sys\nfor line in sys.stdin:\n req=json.loads(line); method=req['method']; result={'serverInfo':{'version':'0.45.2'}} if method=='initialize' else {'tools':[{'name':'git.status','description':'status','inputSchema':{'type':'object','properties':{},'required':[]}}]} if method=='tools/list' else {'isError':True,'content':[{'type':'text','text':'no bootstrap'}]}; print(json.dumps({'jsonrpc':'2.0','id':req['id'],'result':result}), flush=True)\n`);
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

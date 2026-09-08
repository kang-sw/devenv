/**
 * Regression fixture for `260907-bug-ws-pi-deep-explore-missing-collection-tool`
 * Phase 1: guards the `session_start` seam
 * (`bootstrapOrFailLoud`, src/index.ts) so a `startBridge`/`registerAgentTools`
 * failure never leaves a session up with only Pi's builtin `--tools`
 * (read/grep/find/ls) presented as if nothing went wrong.
 *
 * Drives the REAL `startBridge` (src/bridge.ts) against a genuinely broken
 * fake launcher (exits immediately, no JSON-RPC response at all) so
 * `client.initialize()` rejects via `mcp-stdio-client.ts`'s existing
 * "process exited unexpectedly" path — a real forced failure, not a mocked
 * allowlist. This is the same real-`startBridge`-against-a-fake-launcher
 * pattern `native-tool-registration.test.ts`'s "actual MCP startup..." test
 * already uses, just with a launcher that fails instead of one that answers.
 */
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { mock, test } from "node:test";
import type { ExtensionAPI, ExtensionUIContext } from "@earendil-works/pi-coding-agent";
import { bootstrapOrFailLoud } from "../src/index.ts";
import { startBridge } from "../src/bridge.ts";
import { registerAgentTools } from "../src/spawner.ts";
import { WS_PI_EXPLORE_MODE_ENV, WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { createToolPreviewTuiRef } from "../src/tool-result-render.ts";

type Captured = { name: string; parameters?: { properties?: Record<string, unknown> } };

function harness() {
  const tools = new Map<string, Captured>();
  const pi = {
    registerTool: (definition: Captured) => tools.set(definition.name, definition),
    sendMessage() {},
    sendUserMessage() {},
    on() {},
  } as unknown as ExtensionAPI;
  return { tools, pi };
}

const runtimeJsonPath = join(dirname(fileURLToPath(import.meta.url)), "../runtime.json");

/** A launcher that does nothing but exit immediately — no JSON-RPC response at all. */
function writeBrokenLauncher(dir: string): string {
  const launcher = join(dir, "fake-mcp.py");
  writeFileSync(launcher, "import sys\nsys.exit(1)\n");
  return launcher;
}

/** Same working fake launcher `native-tool-registration.test.ts` uses for its real-`startBridge` test. */
function writeWorkingLauncher(dir: string): string {
  const launcher = join(dir, "fake-mcp.py");
  writeFileSync(
    launcher,
    `import json, sys\nfor line in sys.stdin:\n req=json.loads(line); method=req['method']; result={'serverInfo':{'version':'0.45.2'}} if method=='initialize' else {'tools':[{'name':'git.status','description':'status','inputSchema':{'type':'object','properties':{},'required':[]}}]} if method=='tools/list' else {'isError':True,'content':[{'type':'text','text':'no bootstrap'}]}; print(json.dumps({'jsonrpc':'2.0','id':req['id'],'result':result}), flush=True)\n`,
  );
  return launcher;
}

function fakeUi(): { ui: Pick<ExtensionUIContext, "notify">; notify: ReturnType<typeof mock.fn> } {
  const notify = mock.fn();
  return { ui: { notify } as unknown as Pick<ExtensionUIContext, "notify">, notify };
}

/** Runs `fn` with the given role/mode set as env vars, restoring the previous values afterward. */
async function withRoleEnv<T>(role: string | undefined, mode: string | undefined, fn: () => Promise<T>): Promise<T> {
  const oldRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  const oldMode = process.env[WS_PI_EXPLORE_MODE_ENV];
  if (role === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV];
  else process.env[WS_PI_SPAWN_ROLE_ENV] = role;
  if (mode === undefined) delete process.env[WS_PI_EXPLORE_MODE_ENV];
  else process.env[WS_PI_EXPLORE_MODE_ENV] = mode;
  try {
    return await fn();
  } finally {
    if (oldRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV];
    else process.env[WS_PI_SPAWN_ROLE_ENV] = oldRole;
    if (oldMode === undefined) delete process.env[WS_PI_EXPLORE_MODE_ENV];
    else process.env[WS_PI_EXPLORE_MODE_ENV] = oldMode;
  }
}

test("spawned child (explore): a broken launcher notifies loud and exits the process instead of falling through toolless", async () => {
  const { pi } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-guard-child-"));
  const launcher = writeBrokenLauncher(dir);
  const ref = createToolPreviewTuiRef();
  const { ui, notify } = fakeUi();
  const exitSpy = mock.fn((_code: number) => undefined as never);

  const result = await bootstrapOrFailLoud(
    ui,
    "explore",
    async () => {
      const h = await startBridge(pi, { launcherPath: launcher, pluginDir: dir, runtimeJsonPath, cwd: dir, toolPreviewTuiRef: ref });
      throw new Error("unreachable — startBridge above must reject first");
    },
    exitSpy,
  );

  assert.equal(result, undefined, "a failed bootstrap never returns a partial handle/agentTools object");
  assert.equal(notify.mock.calls.length, 1, "exactly one loud notification");
  const [message, level] = notify.mock.calls[0].arguments;
  assert.equal(level, "error");
  assert.match(String(message), /ws-mcp process exited unexpectedly/, "the real underlying startBridge error text is surfaced, not swallowed");
  assert.equal(exitSpy.mock.calls.length, 1, "a spawned child (role !== undefined) is exited so its RPC parent sees a real error");
  assert.equal(exitSpy.mock.calls[0].arguments[0], 1);
});

test("host lead (role undefined): a broken launcher notifies loud but does not exit the interactive process", async () => {
  const { pi } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-guard-lead-"));
  const launcher = writeBrokenLauncher(dir);
  const ref = createToolPreviewTuiRef();
  const { ui, notify } = fakeUi();
  const exitSpy = mock.fn((_code: number) => undefined as never);

  const result = await bootstrapOrFailLoud(
    ui,
    undefined,
    async () => {
      const h = await startBridge(pi, { launcherPath: launcher, pluginDir: dir, runtimeJsonPath, cwd: dir, toolPreviewTuiRef: ref });
      throw new Error("unreachable — startBridge above must reject first");
    },
    exitSpy,
  );

  assert.equal(result, undefined);
  assert.equal(notify.mock.calls.length, 1, "the host lead still gets a loud notification");
  assert.equal(notify.mock.calls[0].arguments[1], "error");
  assert.equal(exitSpy.mock.calls.length, 0, "the host lead has no RPC parent to signal, so it is never hard-exited by this guard");
});

test("happy path: a working launcher never fires notify/exit and returns the real handle/agentTools", async () => {
  const { pi, tools } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-guard-happy-"));
  const launcher = writeWorkingLauncher(dir);
  const ref = createToolPreviewTuiRef();
  const { ui, notify } = fakeUi();
  const exitSpy = mock.fn((_code: number) => undefined as never);

  const result = await withRoleEnv("worker", undefined, async () =>
    bootstrapOrFailLoud(
      ui,
      "worker",
      async () => {
        const h = await startBridge(pi, { launcherPath: launcher, pluginDir: dir, runtimeJsonPath, cwd: dir, toolPreviewTuiRef: ref });
        const agentTools = registerAgentTools(pi, h, { cwd: dir }, undefined, undefined, undefined, ref);
        return { handle: h, agentTools };
      },
      exitSpy,
    ),
  );

  assert.ok(result, "the guard resolves to the real bootstrap result on success");
  assert.equal(notify.mock.calls.length, 0, "no loud notification on a healthy bootstrap");
  assert.equal(exitSpy.mock.calls.length, 0, "no exit on a healthy bootstrap");
  assert.ok(tools.has("ws__git_status"), "the real MCP tool registration still ran");

  await result!.agentTools.stopAll();
  result!.handle.shutdown();
});

test("healthy simple researcher through the guard: only read/grep/find/ls-adjacent registration, no explore, no bash", async () => {
  const { pi, tools } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-guard-simple-"));
  const launcher = writeWorkingLauncher(dir);
  const ref = createToolPreviewTuiRef();
  const { ui, notify } = fakeUi();
  const exitSpy = mock.fn((_code: number) => undefined as never);

  const result = await withRoleEnv("explore", "simple", async () =>
    bootstrapOrFailLoud(
      ui,
      "explore",
      async () => {
        const h = await startBridge(pi, { launcherPath: launcher, pluginDir: dir, runtimeJsonPath, cwd: dir, toolPreviewTuiRef: ref });
        const agentTools = registerAgentTools(pi, h, { cwd: dir }, undefined, undefined, undefined, ref);
        return { handle: h, agentTools };
      },
      exitSpy,
    ),
  );

  assert.ok(result);
  assert.equal(notify.mock.calls.length, 0);
  assert.equal(exitSpy.mock.calls.length, 0);
  assert.equal(tools.has("explore"), false, "a simple researcher must not gain the blocking deep-collection tool");
  assert.equal(tools.has("bash"), false, "this adapter never registers a native bash tool of its own");

  await result!.agentTools.stopAll();
  result!.handle.shutdown();
});

test("healthy deep researcher through the guard: explore is registered alongside the reads, still no bash", async () => {
  const { pi, tools } = harness();
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-guard-deep-"));
  const launcher = writeWorkingLauncher(dir);
  const ref = createToolPreviewTuiRef();
  const { ui, notify } = fakeUi();
  const exitSpy = mock.fn((_code: number) => undefined as never);

  const result = await withRoleEnv("explore", "deep", async () =>
    bootstrapOrFailLoud(
      ui,
      "explore",
      async () => {
        const h = await startBridge(pi, { launcherPath: launcher, pluginDir: dir, runtimeJsonPath, cwd: dir, toolPreviewTuiRef: ref });
        const agentTools = registerAgentTools(pi, h, { cwd: dir }, undefined, undefined, undefined, ref);
        return { handle: h, agentTools };
      },
      exitSpy,
    ),
  );

  assert.ok(result);
  assert.equal(notify.mock.calls.length, 0);
  assert.equal(exitSpy.mock.calls.length, 0);
  assert.ok(tools.has("explore"), "a deep researcher must have the blocking collection tool registered");
  assert.equal(tools.has("bash"), false, "this adapter never registers a native bash tool of its own");

  await result!.agentTools.stopAll();
  result!.handle.shutdown();
});

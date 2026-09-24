/**
 * Adapter-level control-channel behavior against REAL Pi children: the
 * production `spawnAgent` / `sendToAgent` / `stopAgent` paths, the real
 * extension entry (`src/index.ts`) doing its own hello and readiness, and no
 * LLM request (prompt is mocked; startup, RPC, extension load, bash are real).
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, realpathSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { CHANNEL_CREDENTIAL_ENV, CHANNEL_PROTOCOL_VERSION, connectChannelEndpoint, defaultChannelSocketDir, type ChannelEndpoint, type ParentChannel } from "../src/agent-channel.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const cli = join(dirname(fileURLToPath(import.meta.resolve("@earendil-works/pi-coding-agent"))), "cli.js");
const savedArgv = process.argv[1];
process.argv[1] = cli;
const { spawnAgent, sendToAgent, stopAgent, awaitChannelStage } = await import("../src/spawner.ts");
process.argv[1] = savedArgv;

const WEB_READINESS = { tools: ["web_search", "ws_web_fetch"] };
const LAUNCH_TIMEOUT = 90_000;

function makeRoot(t: { after(fn: () => void): void }): string {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-channel-live-")));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

function exploreContext(root: string, lead: string, extra: Record<string, unknown> = {}): any {
  return {
    cwd: packageRoot, storage: createAgentStorageContext(lead, root), wsToolNames: [], inheritModel: "openrouter/openai/gpt-4o",
    extensionPath: join(packageRoot, "src", "index.ts"), toolGroup: "read-only-explore", spawnRole: "explore", exploreMode: "code-search",
    ...extra,
  };
}

async function spawnExplore(registry: Map<string, any>, context: any, prompt = "not dispatched to a model") {
  const result = await spawnAgent(registry, context, { systemPromptPath: join(packageRoot, "explore-guide.md"), prompt });
  return registry.get(result.agent_id)!;
}

async function teardown(registry: Map<string, any>): Promise<void> {
  for (const record of registry.values()) { record.ownershipObserverStop?.(); record.channel?.close(); await record.client?.stop(); }
}

/** Raw hello against a live parent endpoint; resolves with the parent's first answer line. */
async function helloProbe(endpoint: ChannelEndpoint, hello: Record<string, unknown>): Promise<Record<string, unknown>> {
  const socket = await connectChannelEndpoint(endpoint);
  socket.setEncoding("utf8");
  const answer = new Promise<Record<string, unknown>>((resolve, reject) => {
    let buffer = "";
    socket.on("data", (chunk: string) => {
      buffer += chunk;
      const index = buffer.indexOf("\n");
      if (index >= 0) { resolve(JSON.parse(buffer.slice(0, index))); socket.destroy(); }
    });
    socket.on("error", reject);
    socket.on("close", () => reject(new Error("closed before an answer")));
  });
  socket.write(JSON.stringify({ t: "hello", v: CHANNEL_PROTOCOL_VERSION, reconnect: false, resume: {}, ...hello }) + "\n");
  return answer;
}

function socketPath(channel: ParentChannel): string | undefined {
  return channel.endpoint.kind === "pipe" && process.platform !== "win32" ? channel.endpoint.path : undefined;
}

test("spawn, stop, and dormant resume: every launch gets a fresh endpoint, credential, and generation; stop closes the channel", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  const context = exploreContext(root, "lead-a");
  try {
    const record = await spawnExplore(registry, context);
    const first = record.channel as ParentChannel;
    assert.ok(first && first.live, "the launch holds a live child connection");
    assert.equal(first.generation, 1);
    assert.equal(record.launchGeneration, 1);
    assert.equal(first.accepted, 1);
    assert.deepEqual(first.rejects, []);
    assert.equal(first.endpoint.kind, "pipe", "the pipe backend binds first");
    assert.deepEqual(await first.readiness("web"), WEB_READINESS);
    const firstSocket = socketPath(first);
    if (firstSocket) assert.ok(existsSync(firstSocket));

    // The bootstrap was read and deleted before the child's own shells exist.
    const env = await record.client.bash("env");
    const output = String(env?.output ?? "");
    assert.match(output, /WS_PI_SPAWN_ROLE=explore/, "positive control: the child's role marker is visible to its bash");
    assert.doesNotMatch(output, /WS_PI_CHANNEL_/, "no channel bootstrap value reaches the child's bash environment");

    await stopAgent(registry, record.agentId, undefined, { silent: true });
    assert.equal(record.client, undefined);
    assert.equal(record.channel, undefined, "stop releases the channel with the client");
    assert.equal(first.closed, true);
    if (firstSocket) assert.equal(existsSync(firstSocket), false, "the Unix socket is unlinked on close");
    await assert.rejects(connectChannelEndpoint(first.endpoint), "a stopped launch's endpoint refuses connections");

    await sendToAgent(registry, { cwd: packageRoot, extensionPath: context.extensionPath }, record.agentId, "resume without inference");
    const second = record.channel as ParentChannel;
    assert.ok(second && second.live);
    assert.equal(second.generation, 2);
    assert.equal(record.launchGeneration, 2);
    assert.notEqual(second.credential, first.credential);
    assert.notDeepEqual(second.endpoint, first.endpoint);
    assert.deepEqual(await second.readiness("web"), WEB_READINESS);

    // Policy on the live relaunch: the previous generation is rejected even with the current credential; a second live hello is busy.
    assert.deepEqual(await helloProbe(second.endpoint, { cred: second.credential, gen: 1 }), { t: "reject", reason: "generation" });
    assert.deepEqual(await helloProbe(second.endpoint, { cred: second.credential, gen: 2 }), { t: "reject", reason: "busy" });
    assert.deepEqual(second.rejects, ["generation", "busy"]);
    assert.equal(second.accepted, 1, "neither probe replaced the live child");
  } finally { await teardown(registry); }
});

test("concurrent siblings hold distinct credentials; one sibling's credential is rejected at the other's endpoint", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  const context = exploreContext(root, "lead-b");
  try {
    const [a, b] = await Promise.all([spawnExplore(registry, context, "a"), spawnExplore(registry, context, "b")]);
    const channelA = a.channel as ParentChannel, channelB = b.channel as ParentChannel;
    assert.notEqual(channelA.credential, channelB.credential);
    assert.notDeepEqual(channelA.endpoint, channelB.endpoint);
    assert.deepEqual(await helloProbe(channelA.endpoint, { cred: channelB.credential, gen: channelA.generation }), { t: "reject", reason: "auth" });
    assert.deepEqual(await helloProbe(channelB.endpoint, { cred: channelA.credential, gen: channelB.generation }), { t: "reject", reason: "auth" });
    assert.equal(channelA.accepted, 1);
    assert.equal(channelB.accepted, 1);
    assert.ok(channelA.live && channelB.live, "the real children stay attached");
  } finally { await teardown(registry); }
});

test("a forced drop is re-established by the real child, whose reconnect hello carries its readiness in the resume section", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-c"));
    const channel = record.channel as ParentChannel;
    const reconnected = new Promise<{ hello: any; conn: any }>(resolve => { const off = channel.onConnection((conn, hello) => { off(); resolve({ hello, conn }); }); });
    const dropped = new Promise<void>(resolve => { const off = channel.onDisconnect(() => { off(); resolve(); }); });
    channel.live!.close();
    await dropped;
    const { hello, conn } = await reconnected;
    assert.equal(hello.reconnect, true);
    assert.equal(typeof hello.pid, "number");
    assert.deepEqual(hello.resume, { readiness: { web: WEB_READINESS } }, "the resume section restores the readiness already proved");
    assert.equal(channel.accepted, 2);
    assert.equal(channel.live, conn);
    assert.deepEqual(await channel.readiness("web"), WEB_READINESS);
    assert.ok(await record.client.getState(), "the child is unaffected by the channel drop");
  } finally { await teardown(registry); }
});

test("a drop between the hello and readiness recovers through the reconnect; readiness that never arrives fails with today's error inside the bound", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  const originalStart = RpcClient.prototype.start;
  // The channel is bound before the process starts, so the first accepted
  // connection can be cut from inside `start()` before the child reaches
  // `session_start` and publishes anything.
  t.mock.method(RpcClient.prototype, "start", async function (this: RpcClient) {
    const record = [...registry.values()].find(candidate => candidate.client === this);
    const channel = record?.channel as ParentChannel | undefined;
    if (channel) { const off = channel.onConnection(conn => { off(); setTimeout(() => conn.close(), 20); }); }
    return originalStart.call(this);
  });
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-d"));
    const channel = record.channel as ParentChannel;
    assert.equal(channel.accepted, 2, "the launch completed over the child's reconnect");
    assert.ok(channel.live);
    assert.deepEqual(await channel.readiness("web"), WEB_READINESS);

    await assert.rejects(
      awaitChannelStage(record.client, channel.readiness("fork"), 300, "fork readiness", () => new Error("ws-pi-agent: fork did not publish readiness")),
      { message: "ws-pi-agent: fork did not publish readiness" },
    );
    await assert.rejects(
      awaitChannelStage(record.client, channel.readiness("fork"), 300, "web readiness", () => new Error("web-search-tool-unavailable: Explore web facade readiness was not proved")),
      { message: "web-search-tool-unavailable: Explore web facade readiness was not proved" },
    );
    assert.ok(await record.client.getState(), "a bounded wait leaves the child untouched");
  } finally { await teardown(registry); }
});

test("a hello the parent rejects exits the child at startup and the parent reports a failed launch", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  const originalStart = RpcClient.prototype.start;
  t.mock.method(RpcClient.prototype, "start", async function (this: RpcClient) {
    (this as any).options.env[CHANNEL_CREDENTIAL_ENV] = "not-the-credential";
    return originalStart.call(this);
  });
  const startedAt = Date.now();
  try {
    // The spawn-failed push itself is covered by spawner.test.ts's
    // `pushSpawnFailed` cases; here the launch outcome and the record's
    // resting shape are what a real rejected hello must produce.
    await assert.rejects(spawnExplore(registry, exploreContext(root, "lead-e")), (error: Error) => {
      assert.match(error.message, /^ws-pi-agent: child channel hello timed out after \d+ms \(child exited before channel hello: /);
      return true;
    });
    assert.ok(Date.now() - startedAt < 25_000, "the exit was observed by the lifecycle probe, not the timeout");
    const [record] = registry.values();
    assert.equal(record.client, undefined, "a failed launch leaves no live client");
    assert.equal(record.channel, undefined, "nor an open channel");
    assert.equal(record.running, false);
  } finally { await teardown(registry); }
});

test("a stale socket left in the default directory by a SIGKILLed listener is swept by the next launch", { timeout: LAUNCH_TIMEOUT, skip: process.platform === "win32" }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const dir = defaultChannelSocketDir();
  const stale = join(dir, `stale-${process.pid}.sock`);
  const listener = spawn(process.execPath, ["-e", `const net=require("net");net.createServer().listen(${JSON.stringify(stale)},()=>{process.stdout.write("listening\\n")});setInterval(()=>{},1000)`], { stdio: ["ignore", "pipe", "inherit"] });
  await new Promise<void>(resolve => listener.stdout.once("data", () => resolve()));
  assert.ok(existsSync(stale));
  listener.kill("SIGKILL");
  await new Promise<void>(resolve => listener.once("exit", () => resolve()));
  assert.ok(existsSync(stale), "SIGKILL leaves the socket file behind");
  const registry = new Map<string, any>();
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-f"));
    assert.equal(existsSync(stale), false, "the bind's sweep removed the stale socket");
    assert.ok(existsSync((record.channel as ParentChannel).endpoint.kind === "pipe" ? (record.channel.endpoint as any).path : stale), "the live socket stays");
  } finally { await teardown(registry); }
});

test("nested hop: a grandchild spawned from inside the real child's scrubbed environment gets its own channel", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-g"));
    const probe = join(packageRoot, "test", "fixtures", "nested-spawn-probe.ts");
    const nestedRoot = join(root, "nested");
    const result = await record.client.bash(`WS_PI_NESTED_PROBE=1 ${JSON.stringify(process.execPath)} ${JSON.stringify(probe)} ${JSON.stringify(nestedRoot)}`);
    const output = String(result?.output ?? "");
    const line = output.split("\n").find(candidate => candidate.startsWith("NESTED "));
    assert.ok(line, `the probe reported: ${output.slice(-2000)}`);
    const report = JSON.parse(line.slice("NESTED ".length));
    assert.deepEqual(report.inheritedBootstrap, [], "the child's bash inherits no bootstrap from its parent");
    assert.equal(report.generation, 1);
    assert.equal(report.endpointKind, "pipe");
    assert.equal(report.accepted, 1);
    assert.deepEqual(report.readiness, WEB_READINESS);
    assert.equal(report.grandchildSeesBootstrap, false);
    assert.equal(report.grandchildRole, true);
    assert.ok((record.channel as ParentChannel).live, "the outer channel is untouched by the inner hop");
  } finally { await teardown(registry); }
});

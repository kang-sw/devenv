/**
 * Adapter-level control-channel behavior against REAL Pi children: the
 * production `spawnAgent` / `sendToAgent` / `stopAgent` paths, the real
 * extension entry (`src/index.ts`) doing its own hello and readiness, and no
 * LLM request (prompt is mocked; startup, RPC, extension load, bash are real).
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { allocateAgentHome, createAgentStorageContext, persistOwnershipTelemetry, updateOwnership } from "../src/agent-storage.ts";
import { CHANNEL_CREDENTIAL_ENV, CHANNEL_PROTOCOL_VERSION, connectChannelEndpoint, type ChannelEndpoint, type ParentChannel } from "../src/agent-channel.ts";
import { APPROVAL_CONSUMED_MESSAGE } from "../src/approval-protocol.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const cli = join(dirname(fileURLToPath(import.meta.resolve("@earendil-works/pi-coding-agent"))), "cli.js");
const savedArgv = process.argv[1];
process.argv[1] = cli;
const { spawnAgent, sendToAgent, stopAgent } = await import("../src/spawner.ts");
const { sidecarPath, writeSidecarAt } = await import("../src/agent-sidecar.ts");
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

async function until(condition: () => boolean, what: string, timeoutMs = 10_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise(resolve => setTimeout(resolve, 10));
  }
}

test("a forced drop is re-established by the real child, whose reconnect hello carries its readiness and subtree snapshot in the resume section", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-c"));
    const channel = record.channel as ParentChannel;
    // The real child's session_start snapshot arrives over the channel; no file carries it.
    await until(() => record.subtreeRevision !== undefined, "the child's first subtree snapshot");
    assert.equal(record.waitingOnChildren, false, "an idle child with no descendants reads quiescent");
    assert.equal(existsSync(join(record.ownership.home, "subtree.json")), false, "no subtree.json is written");
    const revision = record.subtreeRevision;
    const reconnected = new Promise<{ hello: any; conn: any }>(resolve => { const off = channel.onConnection((conn, hello) => { off(); resolve({ hello, conn }); }); });
    const dropped = new Promise<void>(resolve => { const off = channel.onDisconnect(() => { off(); resolve(); }); });
    channel.live!.close();
    await dropped;
    assert.equal(record.waitingOnChildren, true, "a disconnected channel reads as waiting");
    const { hello, conn } = await reconnected;
    assert.equal(hello.reconnect, true);
    assert.equal(typeof hello.pid, "number");
    assert.deepEqual(hello.resume.readiness, { web: WEB_READINESS }, "the resume section restores the readiness already proved");
    assert.equal(hello.resume.subtree?.revision, revision, "the resume section carries the latest subtree snapshot");
    assert.equal(record.waitingOnChildren, false, "the reconnected quiescent snapshot restores the view");
    assert.equal(record.subtreeRevision, revision);
    assert.equal(channel.accepted, 2);
    assert.equal(channel.live, conn);
    assert.deepEqual(await channel.readiness("web"), WEB_READINESS);
    assert.ok(await record.client.getState(), "the child is unaffected by the channel drop");
  } finally { await teardown(registry); }
});

test("a drop between the hello and readiness recovers through the reconnect, whose hello carries the readiness the drop lost", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  const originalStart = RpcClient.prototype.start;
  const hellos: any[] = [];
  let readyBeforeDrop: boolean | undefined;
  // The channel is bound before the process starts, so the first accepted
  // connection is cut from inside the hello's own accept, before the child
  // reaches `session_start` and publishes anything. (Launch-path failures on
  // absent or invalid readiness are agent-channel-launch.test.ts's.)
  t.mock.method(RpcClient.prototype, "start", async function (this: RpcClient) {
    const record = [...registry.values()].find(candidate => candidate.client === this);
    const channel = record?.channel as ParentChannel | undefined;
    if (channel) {
      let ready = false;
      void channel.readiness("web").then(() => { ready = true; }, () => {});
      channel.onConnection((conn, hello) => {
        hellos.push(hello);
        if (hellos.length === 1) { readyBeforeDrop = ready; conn.close(); }
      });
    }
    return originalStart.call(this);
  });
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-d"));
    const channel = record.channel as ParentChannel;
    assert.equal(readyBeforeDrop, false, "precondition: the drop preceded the readiness message");
    assert.equal(channel.accepted, 2, "the launch completed over the child's reconnect");
    assert.equal(hellos.length, 2);
    assert.equal(hellos[1].reconnect, true);
    assert.deepEqual(hellos[1].resume.readiness, { web: WEB_READINESS }, "the readiness published while disconnected rode the reconnect hello");
    assert.ok(channel.live);
    assert.deepEqual(await channel.readiness("web"), WEB_READINESS);
    assert.ok(await record.client.getState(), "the child is unaffected by the drop");
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
      assert.match(error.message, /^ws-pi-agent: child exited before channel hello: /);
      return true;
    });
    assert.ok(Date.now() - startedAt < 25_000, "the exit was observed by the lifecycle probe, not the timeout");
    const [record] = registry.values();
    assert.equal(record.client, undefined, "a failed launch leaves no live client");
    assert.equal(record.channel, undefined, "nor an open channel");
    assert.equal(record.running, false);
  } finally { await teardown(registry); }
});

test("a stale socket left in the launch's socket directory by a SIGKILLed listener is swept by the next launch", { timeout: LAUNCH_TIMEOUT, skip: process.platform === "win32" }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  // A private directory (through the bind seam) rather than the shared
  // per-user one, which concurrent test processes sweep as well.
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-sock-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const stale = join(dir, `stale-${process.pid}.sock`);
  const listener = spawn(process.execPath, ["-e", `const net=require("net");net.createServer().listen(${JSON.stringify(stale)},()=>{process.stdout.write("listening\\n")});setInterval(()=>{},1000)`], { stdio: ["ignore", "pipe", "inherit"] });
  await new Promise<void>(resolve => listener.stdout.once("data", () => resolve()));
  assert.ok(existsSync(stale));
  listener.kill("SIGKILL");
  await new Promise<void>(resolve => listener.once("exit", () => resolve()));
  assert.ok(existsSync(stale), "SIGKILL leaves the socket file behind");
  const registry = new Map<string, any>();
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-f", { channel: { bind: { socketDir: dir } } }));
    assert.equal(existsSync(stale), false, "the bind's sweep removed the stale socket");
    const live = (record.channel as ParentChannel).endpoint;
    assert.equal(live.kind, "pipe");
    assert.ok((live as { path: string }).path.startsWith(dir), "the launch bound under the given directory");
    assert.ok(existsSync((live as { path: string }).path), "the live socket stays");
  } finally { await teardown(registry); }
});

// 260924 (channel approval decisions): a shell command the real execute-worker
// runs has no way to deliver a decision. The probe tries every path such a
// command could take — the endpoint without a hello, a hello with the
// parent's own credential, a guessed credential, the retired decision file —
// and the parent's pending request must survive all of them untouched, on
// each backend. (The child-side gate and its acknowledgment are the in-process
// contract case in agent-channel.test.ts: a real pending `ws-worker-exec`
// needs a model turn, which these tests never make.)
for (const force of ["pipe", "tcp"] as const) {
  test(`[${force}] a decision from a shell command the execute-worker runs is never consumed; only the parent's own connection carries decisions`, { timeout: LAUNCH_TIMEOUT }, async t => {
    const root = makeRoot(t);
    t.mock.method(RpcClient.prototype, "prompt", async () => {});
    const registry = new Map<string, any>();
    const context = {
      cwd: packageRoot, storage: createAgentStorageContext(`lead-h-${force}`, root), wsToolNames: [], inheritModel: "openrouter/openai/gpt-4o",
      extensionPath: join(packageRoot, "src", "index.ts"), toolGroup: "execute-worker", channel: { bind: { force } },
    };
    try {
      const result = await spawnAgent(registry, context as any, { systemPromptPath: join(packageRoot, "execute-worker-guide.md"), prompt: "not dispatched to a model" });
      const record = registry.get(result.agent_id)!;
      const channel = record.channel as ParentChannel;
      assert.equal(channel.endpoint.kind, force);
      assert.equal(channel.accepted, 1);
      const acks: unknown[] = [];
      channel.onMessage(msg => { if (msg.t === APPROVAL_CONSUMED_MESSAGE) acks.push(msg); });
      // The request a real tool_execution_start would have captured; the probe targets its cmd_id.
      record.pendingApproval = { cmdId: "call-forged", command: "echo forged", rationale: "probe" };
      const decisionPath = join(record.ownership.home, "approvals", "call-forged.decision.json");

      const probe = join(packageRoot, "test", "fixtures", "approval-forgery-probe.ts");
      const args = [JSON.stringify(channel.endpoint), channel.credential, String(channel.generation), "call-forged", decisionPath].map(arg => JSON.stringify(arg)).join(" ");
      const output = String((await record.client.bash(`WS_PI_FORGERY_PROBE=1 ${JSON.stringify(process.execPath)} ${JSON.stringify(probe)} ${args}`))?.output ?? "");
      const line = output.split("\n").find(candidate => candidate.startsWith("FORGERY "));
      assert.ok(line, `the probe reported: ${output.slice(-2000)}`);
      const report = JSON.parse(line.slice("FORGERY ".length));

      assert.deepEqual(report.envKeys, [], "the child's shell sees neither an approvals directory nor the channel bootstrap");
      assert.match(report.rawFrames, /"reason":"malformed"/, "a decision pushed without a hello is rejected");
      assert.match(report.helloWithCredential, /"reason":"busy"/, "the real child holds the one slot, even against the parent's own credential");
      assert.match(report.helloWithoutCredential, /"reason":"auth"/);
      assert.equal(report.legacyFileWritten, true);
      assert.ok(existsSync(decisionPath), "precondition: the retired rendezvous file is really there");
      assert.deepEqual(channel.rejects, ["malformed", "busy", "auth"]);
      assert.equal(channel.accepted, 1, "no forged connection was ever accepted");
      assert.ok(channel.live, "the real child stays attached throughout");
      assert.deepEqual(acks, [], "nothing acknowledged consumption");
      assert.deepEqual(record.pendingApproval, { cmdId: "call-forged", command: "echo forged", rationale: "probe" }, "the request is exactly as pending as before: no forged decision reached anything");
      assert.ok(await record.client.getState(), "the child is unaffected by the probes");
    } finally { await teardown(registry); }
  });
}

test("descendant usage: the real child's session_start revives its dormant child and reports the rebuilt value to the parent record", { timeout: LAUNCH_TIMEOUT }, async t => {
  const root = makeRoot(t);
  t.mock.method(RpcClient.prototype, "prompt", async () => {});
  const registry = new Map<string, any>();
  // The child's Pi session id is fixed before launch: the start hook writes
  // the session file named by `--session`, and Pi adopts an existing file's
  // header id. That id names the child's own owner storage, seeded here under
  // the child's agent dir (redirected to `root`): a sidecar next to its
  // session file listing one dormant grandchild whose ownership home carries
  // own plus reported descendant usage.
  const childSessionId = "usage-rollup-child";
  const grandchildOwn = { knownUsd: .5, knownContributors: 1, unknownContributors: 0, descendants: 1 };
  const grandchildDescendants = { knownUsd: .25, knownContributors: 1, unknownContributors: 0, descendants: 1 };
  const seed = (sessionPath: string) => {
    writeFileSync(sessionPath, `${JSON.stringify({ type: "session", version: 3, id: childSessionId, timestamp: new Date().toISOString(), cwd: packageRoot })}\n`);
    const ownership = allocateAgentHome(createAgentStorageContext(childSessionId, root), "grandchild", "worker");
    const telemetry = { version: 1 as const, origin: { sessionId: "grandchild-session", sessionPath: ownership.sessionPath!, emptyPrefix: true as const }, estimatedUsd: grandchildOwn.knownUsd, descendantUsage: grandchildDescendants };
    writeFileSync(ownership.sessionPath!, [
      { type: "session", version: 3, id: "grandchild-session" },
      { type: "message", id: "a1", message: { role: "assistant", usage: { input: 10, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 10, cost: { total: grandchildOwn.knownUsd } } } },
    ].map(entry => JSON.stringify(entry)).join("\n") + "\n");
    persistOwnershipTelemetry(ownership.home, telemetry);
    updateOwnership(ownership.home, { liveness: { lifecycle: "stopped", running: false } });
    writeSidecarAt(sidecarPath(sessionPath), [{
      agentId: "grandchild", sessionPath: ownership.sessionPath!, systemPromptPath: join(packageRoot, "explore-guide.md"),
      wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker", state: "idle", telemetry, ownership,
    }]);
  };
  const originalStart = RpcClient.prototype.start;
  t.mock.method(RpcClient.prototype, "start", async function (this: RpcClient) {
    const options = (this as any).options as { env: Record<string, string>; args: string[] };
    seed(options.args[options.args.indexOf("--session") + 1]);
    options.env.PI_CODING_AGENT_DIR = root;
    return originalStart.call(this);
  });
  try {
    const record = await spawnExplore(registry, exploreContext(root, "lead-h"));
    const deadline = Date.now() + 10_000;
    while (!record.descendantUsage && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 50));
    assert.equal(existsSync(sidecarPath(record.sessionPath)), false, "precondition: the child's session_start consumed its sidecar");
    assert.deepEqual(record.descendantUsage, {
      knownUsd: grandchildOwn.knownUsd + grandchildDescendants.knownUsd, knownContributors: 2, unknownContributors: 0, descendants: 2,
    }, "the grandchild's own usage plus its reported descendant usage");
    assert.deepEqual(record.descendantUsageOrder, { generation: 1, seq: 1 }, "one report, sent by the child's session_start evaluation");
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

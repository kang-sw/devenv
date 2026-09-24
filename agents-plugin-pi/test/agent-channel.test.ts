import { after, describe, test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { chmodSync, existsSync, mkdtempSync, rmSync } from "node:fs";
import net from "node:net";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  CHANNEL_BOOTSTRAP_ENVS,
  CHANNEL_CREDENTIAL_ENV,
  CHANNEL_ENDPOINT_ENV,
  CHANNEL_GENERATION_ENV,
  CHANNEL_PROTOCOL_VERSION,
  ChannelBindError,
  ChannelRejected,
  ChildChannel,
  ParentChannel,
  bindChannelEndpoint,
  connectChannelEndpoint,
  readAndDeleteChannelBootstrap,
  sweepStaleChannelSockets,
  type ChannelConnection,
} from "../src/agent-channel.ts";
import { ChildApprovalGate, approvalConsumedMessage, approvalDecisionMessage } from "../src/approval-protocol.ts";

const win = process.platform === "win32";
const roots: string[] = [];
after(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); });
function socketDir(): string {
  const dir = mkdtempSync(join(tmpdir(), "ws-ch-"));
  roots.push(dir);
  return dir;
}

async function pair(force: "pipe" | "tcp", generation = 1) {
  const parent = await ParentChannel.bind(generation, { force, socketDir: socketDir() });
  assert.equal(parent.endpoint.kind, force);
  const accepted = new Promise<ChannelConnection>(resolve => parent.onConnection(conn => resolve(conn)));
  const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { reconnect: false });
  return { parent, child, parentConn: await accepted, hello: await parent.hello() };
}

function collect(subscribe: (cb: (msg: Record<string, unknown>) => void) => void, n: number): Promise<Record<string, unknown>[]> {
  return new Promise(resolve => {
    const got: Record<string, unknown>[] = [];
    subscribe(msg => { got.push(msg); if (got.length === n) resolve(got); });
  });
}

// One protocol contract suite, run against each implemented backend, each forced explicitly.
for (const kind of ["pipe", "tcp"] as const) {
  describe(`channel contract [${kind}]`, () => {
    test("authenticated hello, both directions, generation stamped on every message", async () => {
      const { parent, child, hello } = await pair(kind, 7);
      assert.equal(hello.reconnect, false);
      assert.equal(hello.pid, process.pid);
      const up = collect(cb => parent.onMessage(cb), 1);
      child.send({ t: "up", n: 1 });
      assert.deepEqual(await up, [{ t: "up", n: 1, gen: 7 }]);
      const down = collect(cb => child.onMessage(cb), 1);
      parent.send({ t: "down", s: "line1\nline2   é" });
      assert.deepEqual(await down, [{ t: "down", s: "line1\nline2   é", gen: 7 }]);
      child.close();
      parent.close();
    });

    test("delivers in order and exactly once, 5k messages each way, including a 512 KiB frame", async () => {
      const { parent, child } = await pair(kind);
      const n = 5_000;
      const up = collect(cb => parent.onMessage(cb), n + 1);
      const down = collect(cb => child.onMessage(cb), n);
      child.send({ t: "big", big: "x".repeat(512 * 1024) });
      for (let i = 0; i < n; i++) { child.send({ t: "i", i }); parent.send({ t: "i", i }); }
      const [u, d] = await Promise.all([up, down]);
      assert.equal((u[0].big as string).length, 512 * 1024);
      assert.deepEqual(u.slice(1).map(m => m.i), [...Array(n).keys()]);
      assert.deepEqual(d.map(m => m.i), [...Array(n).keys()]);
      child.close();
      parent.close();
    });

    test("stage-2 readiness arrives before or after the parent waits, latest wins", async () => {
      const { parent, child } = await pair(kind);
      child.publishReadiness("fork", { ownSessionKey: "k1" });
      assert.deepEqual(await parent.readiness("fork"), { ownSessionKey: "k1" });
      const later = parent.readiness("web");
      child.publishReadiness("web", { tools: ["web_search"] });
      assert.deepEqual(await later, { tools: ["web_search"] });
      child.close();
      parent.close();
    });

    test("connection policy: busy while live, slot freed by the end event, stale credential/generation/version rejected", async () => {
      const { parent, child, parentConn } = await pair(kind, 3);
      const boot = readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!;
      await assert.rejects(ChildChannel.connect(boot, { reconnect: false }), (e: ChannelRejected) => e.reason === "busy");
      const alive = collect(cb => parent.onMessage(cb), 1);
      child.send({ t: "still" });
      assert.equal((await alive)[0].t, "still", "the first connection keeps working after a busy reject");
      await assert.rejects(ChildChannel.connect({ ...boot, credential: "wrong" }, { reconnect: false }), (e: ChannelRejected) => e.reason === "auth");
      await assert.rejects(ChildChannel.connect({ ...boot, generation: 2 }, { reconnect: false }), (e: ChannelRejected) => e.reason === "generation");
      const raw = await connectChannelEndpoint(parent.endpoint);
      const reply = new Promise<string>(resolve => raw.once("data", d => resolve(String(d))));
      raw.write(JSON.stringify({ t: "hello", v: CHANNEL_PROTOCOL_VERSION + 1, cred: boot.credential, gen: 3 }) + "\n");
      assert.match(await reply, /"reason":"version"/);
      raw.destroy();
      const junk = await connectChannelEndpoint(parent.endpoint); // never sends a hello: must not occupy the slot
      const ended = new Promise<void>(resolve => parent.onDisconnect(resolve));
      child.close();
      await ended;
      assert.equal(parent.live, undefined);
      assert.equal(parentConn.ended, true);
      assert.throws(() => parent.send({ t: "x" }), /no live child connection/);
      const second = await ChildChannel.connect(boot, { reconnect: false });
      assert.equal(parent.accepted, 2);
      assert.deepEqual(parent.rejects, ["busy", "auth", "generation", "version"]);
      junk.destroy();
      second.close();
      parent.close();
    });

    test("child reconnects on its own after the parent drops the connection; the reconnect hello restores published readiness", async () => {
      const parent = await ParentChannel.bind(5, { force: kind, socketDir: socketDir() });
      const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { backoffCapMs: 100, resume: () => ({ feature: "state" }) });
      child.publishReadiness("web", { tools: ["a"] });
      assert.deepEqual(await parent.readiness("web"), { tools: ["a"] });
      const reconnected = new Promise<Record<string, unknown>>(resolve => parent.onConnection((_conn, hello) => { if (hello.reconnect) resolve(hello.resume); }));
      const childSaw = new Promise<void>(resolve => child.onReconnect(resolve));
      parent.live!.close();
      assert.deepEqual(await reconnected, { feature: "state", readiness: { web: { tools: ["a"] } } });
      await childSaw;
      assert.equal(child.connections, 2);
      assert.equal(parent.accepted, 2);
      const up = collect(cb => parent.onMessage(cb), 1);
      child.send({ t: "after" });
      assert.equal((await up)[0].t, "after");
      // Readiness published while disconnected is carried by the next hello.
      const again = new Promise<Record<string, unknown>>(resolve => parent.onConnection((_conn, hello) => { if (parent.accepted === 3) resolve(hello.resume); }));
      const dropped = new Promise<void>(resolve => child.onDisconnect(resolve));
      parent.live!.close();
      await dropped;
      child.publishReadiness("fork", { ownSessionKey: "late" });
      assert.deepEqual((await again).readiness, { web: { tools: ["a"] }, fork: { ownSessionKey: "late" } });
      assert.deepEqual(await parent.readiness("fork"), { ownSessionKey: "late" });
      child.close();
      parent.close();
    });

    test("approval decision: consumed only after the acknowledgment is sent over this connection, one cmd_id never satisfies another, and a reconnect hello reports what is still waiting", async () => {
      const parent = await ParentChannel.bind(6, { force: kind, socketDir: socketDir() });
      const gate = new ChildApprovalGate();
      const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { backoffCapMs: 100, resume: () => gate.resume() });
      await parent.hello();

      // Delivered, acknowledged, consumed — in that order, with the generation stamped on the acknowledgment.
      const ack = collect(cb => parent.onMessage(cb), 1);
      const wait = gate.waitForDecision(child, "call-1", undefined);
      parent.send(approvalDecisionMessage("call-OTHER", { decision: "approve" }));
      parent.send(approvalDecisionMessage("call-1", { decision: "run-instead", command: "echo x" }));
      assert.deepEqual(await wait, { decision: "run-instead", command: "echo x" });
      assert.deepEqual(await ack, [{ ...approvalConsumedMessage("call-1"), gen: 6 }]);
      assert.deepEqual(gate.resume(), { approval: { consumed: ["call-1"] } });

      // The acknowledgment cannot be sent (the connection is gone by the time
      // the decision is handled): the decision is not consumed, the cmd_id
      // stays pending, and the child's reconnect hello reports it. The same
      // wait then consumes the decision sent over the new connection.
      let failNextSend = true;
      const link = { send: (msg: Record<string, unknown>) => { if (failNextSend) { failNextSend = false; throw new Error("ws-pi-channel: not connected to the parent"); } child.send(msg); }, onMessage: (cb: (msg: Record<string, unknown>) => void) => child.onMessage(cb) };
      const stuck = gate.waitForDecision(link, "call-2", undefined);
      const delivered = new Promise<void>(resolve => child.onMessage(msg => { if (msg.cmd_id === "call-2") resolve(); }));
      parent.send(approvalDecisionMessage("call-2", { decision: "approve" }));
      await delivered;
      assert.equal(await Promise.race([stuck, new Promise(resolve => setTimeout(() => resolve("pending"), 30))]), "pending");
      assert.equal(gate.pending, "call-2");
      const reconnected = new Promise<Record<string, unknown>>(resolve => parent.onConnection((_conn, hello) => { if (hello.reconnect) resolve(hello.resume); }));
      parent.live!.close();
      assert.deepEqual(await reconnected, { approval: { pending: "call-2", consumed: ["call-1"] } }, "still waiting, and not among the consumed");
      const ack2 = collect(cb => parent.onMessage(cb), 1);
      parent.send(approvalDecisionMessage("call-2", { decision: "deny", reason: "again" }));
      assert.deepEqual(await stuck, { decision: "deny", reason: "again" }, "only the decision over the new connection, acknowledged over it, is consumed");
      assert.deepEqual(await ack2, [{ ...approvalConsumedMessage("call-2"), gen: 6 }]);
      assert.equal(gate.pending, undefined);
      child.close();
      parent.close();
    });

    test("close(): pending hello and readiness waits reject, later connections are refused", async () => {
      const parent = await ParentChannel.bind(1, { force: kind, socketDir: socketDir() });
      const hello = parent.hello();
      const ready = parent.readiness("fork");
      const boot = readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!;
      parent.close();
      await assert.rejects(hello, /channel closed/);
      await assert.rejects(ready, /channel closed/);
      await assert.rejects(ChildChannel.connect(boot, { reconnect: false }));
      if (kind === "pipe" && !win) assert.equal(existsSync((parent.endpoint as { path: string }).path), false);
    });

    test("close(): a socket accepted before the close whose hello arrives after it is dropped, never welcomed", async () => {
      const parent = await ParentChannel.bind(4, { force: kind, socketDir: socketDir() });
      const boot = readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!;
      const early = await connectChannelEndpoint(parent.endpoint);
      early.on("error", () => { /* the parent destroyed it; the write below may fail with EPIPE */ });
      const answer = new Promise<string>(resolve => { let got = ""; early.on("data", d => { got += String(d); }); early.on("close", () => resolve(got)); });
      parent.close();
      early.write(JSON.stringify({ t: "hello", v: CHANNEL_PROTOCOL_VERSION, cred: boot.credential, gen: 4, reconnect: false, resume: {} }) + "\n");
      assert.equal(await answer, "", "no welcome and no reject line: the socket is simply destroyed");
      assert.equal(parent.live, undefined);
      assert.equal(parent.accepted, 0);
    });

    test("hello timeouts: the child gives up on a silent endpoint, the parent rejects a silent socket, and a probe that closes first records nothing", async () => {
      const silent = net.createServer();
      silent.unref();
      const target = kind === "tcp" ? { host: "127.0.0.1", port: 0 } : { path: win ? `\\\\.\\pipe\\ws-pi-silent-${process.pid}` : join(socketDir(), "silent.sock") };
      await new Promise<void>(resolve => silent.listen(target, resolve));
      const endpoint = kind === "tcp" ? { kind, host: "127.0.0.1", port: (silent.address() as net.AddressInfo).port } : { kind, path: (target as { path: string }).path };
      const startedAt = Date.now();
      await assert.rejects(ChildChannel.connect({ endpoint, credential: "c", generation: 1 }, { reconnect: false, helloTimeoutMs: 100 }), (e: ChannelRejected) => e.reason === "timeout");
      assert.ok(Date.now() - startedAt < 2_000);
      silent.close();

      // The probe runs under a bound no test run reaches, so "closed first" never
      // races a timer under load; the parent's own close handling is observed
      // directly (it listens before this test does), not inferred from timing.
      const probed = await ParentChannel.bind(1, { force: kind, socketDir: socketDir(), helloLineTimeoutMs: 600_000 });
      const internals = probed as unknown as { bound: { server: net.Server }; preHello: Set<net.Socket> };
      const probeClosed = new Promise<void>(resolve => internals.bound.server.once("connection", socket => socket.once("close", () => resolve())));
      const probe = await connectChannelEndpoint(probed.endpoint);
      probe.destroy();
      await probeClosed;
      assert.equal(internals.preHello.size, 0, "the probe that closed first is settled and its bound disarmed");
      assert.deepEqual(probed.rejects, [], "the probe that closed before the bound is not a rejected hello");
      probed.close();

      const parent = await ParentChannel.bind(1, { force: kind, socketDir: socketDir(), helloLineTimeoutMs: 100 });
      const mute = await connectChannelEndpoint(parent.endpoint);
      const answer = new Promise<string>(resolve => mute.once("data", d => resolve(String(d))));
      assert.match(await answer, /"reason":"timeout"/);
      assert.deepEqual(parent.rejects, ["timeout"], "a silent socket is rejected at the bound");
      mute.destroy();
      parent.close();
    });
  });
}

describe("bind order, fallback, and fail-closed diagnostics", () => {
  test("default order binds the pipe backend first", async () => {
    const bound = await bindChannelEndpoint({ socketDir: socketDir() });
    assert.equal(bound.endpoint.kind, "pipe");
    assert.deepEqual(bound.fallbackFailures, []);
    bound.close();
  });

  test("a pipe bind failure falls back to TCP and records the pipe failure", async () => {
    const squatted = win ? `\\\\.\\pipe\\ws-pi-test-${process.pid}` : join(socketDir(), "squat.sock");
    const squatter = net.createServer();
    await new Promise<void>(resolve => squatter.listen(squatted, resolve));
    const bound = await bindChannelEndpoint({ pipePath: squatted });
    assert.equal(bound.endpoint.kind, "tcp");
    assert.equal((bound.endpoint as { host: string }).host, "127.0.0.1");
    assert.equal(bound.fallbackFailures.length, 1);
    assert.equal(bound.fallbackFailures[0].kind, "pipe");
    assert.match(bound.fallbackFailures[0].error, /EADDRINUSE/);
    bound.close();
    squatter.close();
  });

  test("a Unix socket path too long for sun_path fails the pipe bind and falls back to TCP", { skip: win }, async () => {
    const bound = await bindChannelEndpoint({ pipePath: join(socketDir(), "x".repeat(120) + ".sock") });
    assert.equal(bound.endpoint.kind, "tcp");
    assert.match(bound.fallbackFailures[0].error, /EINVAL/);
    bound.close();
  });

  test("when every backend fails the channel fails closed and the diagnostic names both failures", async () => {
    const squatted = win ? `\\\\.\\pipe\\ws-pi-test-closed-${process.pid}` : join(socketDir(), "squat.sock");
    const squatter = net.createServer();
    await new Promise<void>(resolve => squatter.listen(squatted, resolve));
    await assert.rejects(ParentChannel.bind(1, { pipePath: squatted, tcpHost: "192.0.2.1" }), (error: ChannelBindError) => {
      assert.ok(error instanceof ChannelBindError);
      assert.deepEqual(error.failures.map(f => f.kind), ["pipe", "tcp"]);
      assert.match(error.message, /pipe: .*EADDRINUSE.*; tcp: /);
      return true;
    });
    squatter.close();
  });

  test("a socket directory that is not private to this user fails the pipe bind and falls back to TCP", { skip: win }, async () => {
    const dir = socketDir();
    chmodSync(dir, 0o755);
    const bound = await bindChannelEndpoint({ socketDir: dir });
    assert.equal(bound.endpoint.kind, "tcp");
    assert.equal(bound.fallbackFailures.length, 1);
    assert.match(bound.fallbackFailures[0].error, /accessible to other users \(mode 755\)/);
    bound.close();
    chmodSync(dir, 0o700);
    const again = await bindChannelEndpoint({ socketDir: dir });
    assert.equal(again.endpoint.kind, "pipe");
    again.close();
  });

  test("a parent that leaves through process.exit without closing still unlinks its Unix socket", { skip: win }, async () => {
    const dir = socketDir();
    const script = `import { bindChannelEndpoint } from ${JSON.stringify(fileURLToPath(new URL("../src/agent-channel.ts", import.meta.url)))};
      const bound = await bindChannelEndpoint({ socketDir: process.argv[1] });
      process.stdout.write(bound.endpoint.path + "\\n");
      process.exit(0);`;
    const child = spawn(process.execPath, ["--input-type=module", "-e", script, dir], { stdio: ["ignore", "pipe", "inherit"] });
    let output = "";
    child.stdout!.on("data", chunk => { output += String(chunk); });
    const code = await new Promise<number | null>(resolve => child.once("exit", resolve));
    assert.equal(code, 0, output);
    const path = output.trim();
    assert.ok(path.startsWith(dir), `bound under the given directory: ${output}`);
    assert.equal(existsSync(path), false, "the exit hook unlinked the socket");
  });

  test("a stale Unix socket left by a killed listener is swept before the next bind", { skip: win }, async () => {
    const dir = socketDir();
    const { pid, paths: [stale] } = await leaveRefusedSockets(dir, ["{pid}-stale.sock"]);
    assert.equal(stale, join(dir, `${pid}-stale.sock`));
    assert.equal(existsSync(stale), true);
    const live = await bindChannelEndpoint({ socketDir: dir });
    assert.equal(existsSync(stale), false, "the bind's own sweep unlinked the dead owner's refused socket");
    assert.equal(existsSync((live.endpoint as { path: string }).path), true);
    assert.deepEqual(await sweepStaleChannelSockets(dir), [], "a live socket is probed and kept");
    assert.equal(existsSync((live.endpoint as { path: string }).path), true);
    live.close();
  });

  test("the bound socket name carries the binding pid", { skip: win }, async () => {
    const bound = await bindChannelEndpoint({ socketDir: socketDir() });
    assert.match(basename((bound.endpoint as { path: string }).path), new RegExp(`^${process.pid}-[0-9a-f]{12}\\.sock$`));
    bound.close();
  });
});

/**
 * Leaves real refused socket files: a child listens on each name (`{pid}` is
 * the child's pid, `{ppid}` this test process's) and is SIGKILLed, so no
 * close unlinks them. A regular file is no substitute: connecting to one
 * fails with ENOTSOCK on macOS, not ECONNREFUSED.
 */
async function leaveRefusedSockets(dir: string, names: string[]): Promise<{ pid: number; paths: string[] }> {
  const script = `const net = require("net"), { join } = require("path");
    const [dir, ...names] = process.argv.slice(1);
    const paths = names.map(n => join(dir, n.replace("{pid}", String(process.pid)).replace("{ppid}", String(process.ppid))));
    let left = paths.length;
    for (const p of paths) net.createServer().listen(p, () => { if (--left === 0) process.stdout.write(JSON.stringify(paths) + "\\n"); });
    setInterval(() => {}, 1000);`;
  const listener = spawn(process.execPath, ["-e", script, dir, ...names], { stdio: ["ignore", "pipe", "inherit"] });
  const paths = await new Promise<string[]>(resolve => {
    let out = "";
    listener.stdout!.on("data", chunk => { out += String(chunk); if (out.includes("\n")) resolve(JSON.parse(out)); });
  });
  const exited = new Promise(resolve => listener.once("exit", resolve));
  listener.kill("SIGKILL");
  await exited;
  for (const path of paths) {
    assert.equal(existsSync(path), true, "SIGKILL leaves the socket file behind");
    const code = await new Promise<string | undefined>(resolve => {
      const probe = net.connect({ path });
      probe.once("connect", () => { probe.destroy(); resolve(undefined); });
      probe.once("error", (error: NodeJS.ErrnoException) => resolve(error.code));
    });
    assert.equal(code, "ECONNREFUSED", `${path} is refused`);
  }
  return { pid: listener.pid!, paths };
}

describe("stale socket sweep: owner death, not refusal alone", { skip: win }, () => {
  test("a refused socket named for a dead pid is unlinked; one named for a live pid is kept", async () => {
    const dir = socketDir();
    const { pid, paths: [dead, live] } = await leaveRefusedSockets(dir, ["{pid}-x.sock", "{ppid}-x.sock"]);
    assert.equal(live, join(dir, `${process.pid}-x.sock`));
    assert.throws(() => process.kill(pid, 0), (e: NodeJS.ErrnoException) => e.code === "ESRCH", "the listener's pid is dead");
    assert.deepEqual(await sweepStaleChannelSockets(dir), [dead]);
    assert.equal(existsSync(dead), false);
    assert.equal(existsSync(live), true, "a live owner's refused socket may be between its bind() and listen()");
  });

  test("a legacy name without a parsable pid is never unlinked", async () => {
    const dir = socketDir();
    const { paths } = await leaveRefusedSockets(dir, ["0123456789ab.sock", "stale.sock", "0-x.sock", "{pid}.sock"]);
    assert.deepEqual(await sweepStaleChannelSockets(dir), []);
    for (const path of paths) assert.equal(existsSync(path), true, `${basename(path)} kept`);
  });

  test("a live socket survives another process sweeping the same directory continuously", async () => {
    const dir = socketDir();
    const sweeperScript = `import { sweepStaleChannelSockets } from ${JSON.stringify(fileURLToPath(new URL("../src/agent-channel.ts", import.meta.url)))};
      let stop = false, sweeps = 0;
      const removed = [];
      process.stdin.on("end", () => { stop = true; });
      process.stdin.resume();
      while (!stop) {
        removed.push(...await sweepStaleChannelSockets(process.argv[1]));
        if (++sweeps === 1) process.stdout.write("ready\\n");
        await new Promise(resolve => setImmediate(resolve));
      }
      process.stdout.write(JSON.stringify({ sweeps, removed }) + "\\n");`;
    const sweeper = spawn(process.execPath, ["--input-type=module", "-e", sweeperScript, dir], { stdio: ["pipe", "pipe", "inherit"] });
    let output = "";
    await new Promise<void>(resolve => sweeper.stdout!.on("data", chunk => { output += String(chunk); if (output.startsWith("ready\n")) resolve(); }));
    const bound: string[] = [];
    const lost: string[] = [];
    const deadline = Date.now() + 1_500;
    while (Date.now() < deadline) {
      const endpoint = await bindChannelEndpoint({ socketDir: dir });
      assert.equal(endpoint.endpoint.kind, "pipe");
      const path = (endpoint.endpoint as { path: string }).path;
      bound.push(path);
      const ok = existsSync(path) && await new Promise<boolean>(resolve => {
        const probe = net.connect({ path });
        probe.once("connect", () => { probe.destroy(); resolve(true); });
        probe.once("error", () => resolve(false));
      });
      if (!ok) lost.push(path);
      endpoint.close();
    }
    const exited = new Promise<number | null>(resolve => sweeper.once("exit", resolve));
    sweeper.stdin!.end();
    assert.equal(await exited, 0, output);
    const { sweeps, removed } = JSON.parse(output.slice("ready\n".length)) as { sweeps: number; removed: string[] };
    assert.ok(bound.length >= 20 && sweeps >= 20, `binds and sweeps overlapped (binds=${bound.length}, sweeps=${sweeps})`);
    assert.deepEqual(lost, [], "every bound socket existed and accepted a connection after bind");
    assert.deepEqual(removed, [], "the sweeper unlinked no socket of this live process");
  });
});

describe("bootstrap read-then-delete", () => {
  test("reads the three values and removes them from the environment it was given", () => {
    const env: NodeJS.ProcessEnv = { KEEP: "1", [CHANNEL_ENDPOINT_ENV]: JSON.stringify({ kind: "tcp", host: "127.0.0.1", port: 4242 }), [CHANNEL_CREDENTIAL_ENV]: "cred", [CHANNEL_GENERATION_ENV]: "3" };
    assert.deepEqual(readAndDeleteChannelBootstrap(env), { endpoint: { kind: "tcp", host: "127.0.0.1", port: 4242 }, credential: "cred", generation: 3 });
    assert.deepEqual(env, { KEEP: "1" });
    assert.equal(readAndDeleteChannelBootstrap({ KEEP: "1" }), undefined);
    // The spawner clears inherited keys with empty strings (RpcClient overlays
    // env on process.env, so deletion would keep a grandparent's value).
    const cleared: NodeJS.ProcessEnv = Object.fromEntries(CHANNEL_BOOTSTRAP_ENVS.map(key => [key, ""]));
    assert.equal(readAndDeleteChannelBootstrap(cleared), undefined);
    assert.deepEqual(cleared, {});
  });

  test("a partial or malformed bootstrap throws, and is still deleted", () => {
    for (const env of [
      { [CHANNEL_ENDPOINT_ENV]: JSON.stringify({ kind: "tcp", host: "127.0.0.1", port: 1 }), [CHANNEL_CREDENTIAL_ENV]: "cred" },
      { [CHANNEL_ENDPOINT_ENV]: "{", [CHANNEL_CREDENTIAL_ENV]: "cred", [CHANNEL_GENERATION_ENV]: "1" },
      { [CHANNEL_ENDPOINT_ENV]: JSON.stringify({ kind: "file", path: "/x" }), [CHANNEL_CREDENTIAL_ENV]: "cred", [CHANNEL_GENERATION_ENV]: "1" },
      { [CHANNEL_ENDPOINT_ENV]: JSON.stringify({ kind: "tcp", host: "127.0.0.1", port: 1 }), [CHANNEL_CREDENTIAL_ENV]: "cred", [CHANNEL_GENERATION_ENV]: "x" },
    ] as NodeJS.ProcessEnv[]) {
      assert.throws(() => readAndDeleteChannelBootstrap(env), /ws-pi-channel/);
      for (const key of CHANNEL_BOOTSTRAP_ENVS) assert.equal(key in env, false);
    }
  });
});

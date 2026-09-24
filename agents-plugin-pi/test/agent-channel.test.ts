import { after, describe, test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import net from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
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

  test("a stale Unix socket left by a killed listener is swept before the next bind", { skip: win }, async () => {
    const dir = socketDir();
    const stale = join(dir, "stale.sock");
    const listener = spawn(process.execPath, ["-e", "require('net').createServer().listen(process.argv[1],()=>console.log('L'))", stale], { stdio: ["ignore", "pipe", "inherit"] });
    await new Promise(resolve => listener.stdout!.once("data", resolve));
    listener.kill("SIGKILL");
    await new Promise(resolve => listener.once("exit", resolve));
    assert.equal(existsSync(stale), true);
    const live = await bindChannelEndpoint({ socketDir: dir });
    assert.equal(existsSync(stale), false, "the bind's own sweep unlinked the refused socket");
    assert.equal(existsSync((live.endpoint as { path: string }).path), true);
    assert.deepEqual(await sweepStaleChannelSockets(dir), [], "a live socket is probed and kept");
    assert.equal(existsSync((live.endpoint as { path: string }).path), true);
    live.close();
  });
});

describe("bootstrap read-then-delete", () => {
  test("reads the three values and removes them from the environment it was given", () => {
    const env: NodeJS.ProcessEnv = { KEEP: "1", [CHANNEL_ENDPOINT_ENV]: JSON.stringify({ kind: "tcp", host: "127.0.0.1", port: 4242 }), [CHANNEL_CREDENTIAL_ENV]: "cred", [CHANNEL_GENERATION_ENV]: "3" };
    assert.deepEqual(readAndDeleteChannelBootstrap(env), { endpoint: { kind: "tcp", host: "127.0.0.1", port: 4242 }, credential: "cred", generation: 3 });
    assert.deepEqual(env, { KEEP: "1" });
    assert.equal(readAndDeleteChannelBootstrap({ KEEP: "1" }), undefined);
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

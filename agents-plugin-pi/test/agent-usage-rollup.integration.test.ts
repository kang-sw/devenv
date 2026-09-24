/**
 * Multi-process contract for the descendant-usage roll-up
 * (260924-feat-pi-agent-channel-usage-rollup). The root is this test process
 * with a real footer; M and G are real OS processes (`fixtures/usage-hop.ts`)
 * connected by real channel sockets; L is a file-only child of G. Usage enters
 * only as session files this test writes, each reduced by its direct parent:
 * M's by the root, G's by M, L's by G. Everything above a hop's direct
 * children arrives through the channel. No Pi, no LLM.
 *
 * The in-process ordering and fold cases live in `agent-usage-rollup.test.ts`.
 */
import assert from "node:assert/strict";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { createInterface } from "node:readline";
import { test, type TestContext } from "node:test";
import { fileURLToPath } from "node:url";
import { ChildChannel, ParentChannel } from "../src/agent-channel.ts";
import { allocateAgentHome, createAgentStorageContext, persistOwnershipTelemetry, readOwnership } from "../src/agent-storage.ts";
import { createAgentFooterController, registerAgentCostOwner, type AgentFooterComponent, type AgentFooterController } from "../src/agent-footer.ts";
import type { CumulativeCost } from "../src/agent-telemetry.ts";
import { DESCENDANT_USAGE_RESUME_KEY, attachDescendantUsage } from "../src/agent-usage-rollup.ts";
import { refreshAgentTelemetry, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { truncateToWidth, visibleWidth } from "../src/pi-tui.ts";

const HOP = join(dirname(fileURLToPath(import.meta.url)), "fixtures", "usage-hop.ts");
const CHAIN = [{ name: "M", session: "m-session" }, { name: "G", session: "g-session" }, { name: "L", session: "l-session" }];
// Binary-exact dollar amounts so every sum is exact: M .25, G .5, L 1.
const M_OWN = 0.25, G_OWN = 0.5, L_OWN = 1;
const TEST_TIMEOUT_MS = 60_000;

type Stats = { sent: number; connected: boolean; value?: CumulativeCost; stored?: CumulativeCost; childPresent: boolean; childSessionPath?: string; childProcessAlive: boolean; sessionReads: string[]; error?: string };

async function waitFor<T>(what: string, probe: () => T | Promise<T>, done: (value: T) => boolean, timeoutMs = 10_000): Promise<T> {
  const deadline = Date.now() + timeoutMs;
  let last: T;
  for (;;) {
    last = await probe();
    if (done(last)) return last;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}; last: ${JSON.stringify(last)}`);
    await new Promise(resolve => setTimeout(resolve, 20));
  }
}

function assistant(id: string, cost: number) {
  return { type: "message", id, message: { role: "assistant", usage: { input: 100, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 100, cost: { total: cost } } } };
}
/** One session file with one assistant entry per cost; appending a cost keeps the earlier entries (a longer transcript). */
function writeSession(path: string, sessionId: string, costs: number[]): void {
  const entries = [{ type: "session", version: 3, id: sessionId }, ...costs.map((cost, i) => assistant(`${sessionId}-${i}`, cost))];
  writeFileSync(path, entries.map(entry => JSON.stringify(entry)).join("\n") + "\n");
}

function footerContext() {
  let factory: any;
  const ctx = {
    mode: "tui", cwd: "/work/project",
    model: { provider: "provider", id: "model", contextWindow: 200_000 },
    sessionManager: { getEntries: () => [], getCwd: () => "/work/project" },
    getContextUsage: () => ({ tokens: 1, percent: 0, contextWindow: 200_000 }),
    ui: { setFooter(next: any) { factory = next; } },
  };
  const mount = (): AgentFooterComponent => factory(
    { requestRender() {} }, { fg: (_color: string, text: string) => text },
    { getGitBranch: () => undefined, getExtensionStatuses: () => new Map(), getAvailableProviderCount: () => 1, onBranchChange: () => () => {} },
  );
  return { ctx, mount };
}

/** A running hop process (M, with G below it) driven over stdin/stdout. */
class HopProcess {
  readonly proc: ChildProcess;
  readonly ready = new Map<string, Record<string, unknown>>();
  private readonly waiters = new Map<string, (reply: Record<string, unknown>) => void>();
  private readonly readyWaiters: Array<() => void> = [];
  private nextId = 0;
  readonly exited: Promise<void>;

  constructor(root: string, channel: ParentChannel, restart: boolean) {
    this.proc = spawn(process.execPath, [HOP], {
      env: { ...process.env, ...channel.bootstrapEnv(), WS_PI_USAGE_HOP: "1", HOP_ROOT: root, HOP_CHAIN: JSON.stringify(CHAIN), HOP_INDEX: "0", HOP_RESTART: restart ? "1" : "0" },
      stdio: ["pipe", "pipe", "inherit"],
    });
    this.exited = new Promise(resolve => this.proc.once("exit", () => resolve()));
    createInterface({ input: this.proc.stdout! }).on("line", line => {
      let msg: Record<string, unknown>;
      try { msg = JSON.parse(line); } catch { return; }
      if (msg.event === "ready") { this.ready.set(String(msg.hop), msg); for (const wake of this.readyWaiters.splice(0)) wake(); return; }
      const waiter = this.waiters.get(String(msg.id));
      if (waiter) { this.waiters.delete(String(msg.id)); waiter(msg); }
    });
  }

  async whenReady(hop: string): Promise<Record<string, unknown>> {
    while (!this.ready.has(hop)) {
      await Promise.race([
        new Promise<void>(resolve => this.readyWaiters.push(resolve)),
        this.exited.then(() => { throw new Error(`hop process exited before ${hop} was ready`); }),
      ]);
    }
    return this.ready.get(hop)!;
  }

  command<T = Stats>(hop: string, cmd: string): Promise<T> {
    const id = `c${++this.nextId}`;
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`no reply to ${cmd} from ${hop}`)), 10_000);
      this.waiters.set(id, reply => {
        clearTimeout(timer);
        if (reply.error) reject(new Error(`${hop} ${cmd}: ${reply.error}`)); else resolve(reply as T);
      });
      this.proc.stdin!.write(`${JSON.stringify({ id, hop, cmd })}\n`);
    });
  }

  /** Shutdown for restart: every hop persists its sidecar and checkpoint, then exits (G first, by M). */
  async shutdown(): Promise<void> {
    await this.command("M", "shutdown");
    await this.exited;
  }

  kill(): void { if (this.proc.exitCode === null && this.proc.signalCode === null) this.proc.kill("SIGKILL"); }
}

/** The root: M's owner, with a real footer whose D value the tests read. */
class Root {
  readonly dir: string;
  readonly storage: ReturnType<typeof createAgentStorageContext>;
  registry!: RpcAgentRegistry;
  recordM!: RpcAgentRecord;
  footer!: AgentFooterController;
  component!: AgentFooterComponent;
  channel: ParentChannel | undefined;
  hop: HopProcess | undefined;
  generation = 0;
  /** Accepted reports that changed M's stored descendant value. */
  changes = 0;
  sessions = { M: "", G: "", L: "" };
  private detach: (() => void) | undefined;
  private readonly cleanups: Array<() => void> = [];

  constructor(t: TestContext) {
    this.dir = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-rollup-it-")));
    this.storage = createAgentStorageContext("root-lead", this.dir);
    t.after(() => this.cleanup());
  }

  static async start(t: TestContext): Promise<Root> {
    const root = new Root(t);
    const ownership = allocateAgentHome(root.storage, "M", "worker");
    const record = {
      agentId: "M", sessionPath: ownership.sessionPath!, ownership, systemPromptPath: "usage-hop.md",
      telemetry: { version: 1 as const, origin: { sessionId: "m-session", sessionPath: ownership.sessionPath!, emptyPrefix: true as const } },
      wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker", streaming: false, running: false, reportLog: [],
    } as RpcAgentRecord;
    persistOwnershipTelemetry(ownership.home, record.telemetry);
    root.mountRegistry(record);
    await root.launch(false);
    root.sessions = { M: record.sessionPath, G: String(root.hop!.ready.get("M")!.childSessionPath), L: String(root.hop!.ready.get("G")!.childSessionPath) };
    writeSession(root.sessions.M, "m-session", [M_OWN]);
    writeSession(root.sessions.G, "g-session", [G_OWN]);
    writeSession(root.sessions.L, "l-session", [L_OWN]);
    return root;
  }

  mountRegistry(record: RpcAgentRecord): void {
    this.recordM = record;
    this.registry = new Map([[record.agentId, record]]);
    registerAgentCostOwner(this.registry, this.storage);
    const ui = footerContext();
    this.footer = createAgentFooterController(ui.ctx, this.registry, this.storage, { truncateToWidth, visibleWidth }, async () => undefined);
    this.component = ui.mount();
  }

  /** Launches M (and, on a first launch, G below it) on a fresh channel generation: the spawner's per-launch wiring. */
  async launch(restart: boolean): Promise<HopProcess> {
    this.detach?.();
    this.channel?.close();
    this.generation += 1;
    const channel = await ParentChannel.bind(this.generation);
    this.channel = channel;
    this.recordM.channel = channel;
    this.recordM.launchGeneration = this.generation;
    this.detach = attachDescendantUsage(this.recordM, channel, () => { this.changes += 1; this.footer.refreshAgents(); });
    const hop = new HopProcess(this.dir, channel, restart);
    this.hop = hop;
    await channel.hello();
    await hop.whenReady("M");
    if (!restart) await hop.whenReady("G");
    return hop;
  }

  /** The root's own reduction of its direct child M. */
  refreshM(): void { refreshAgentTelemetry(this.recordM); this.footer.refreshAgents(); }

  footerD(): string {
    const line = this.component.render(200)[1];
    const match = /\bD (\S+(?: \+ \?)?)/.exec(line);
    assert.ok(match, `footer stats line has a D value: ${line}`);
    return match[1];
  }

  async waitForD(expected: string): Promise<void> {
    await waitFor(`root footer D ${expected}`, () => this.footerD(), value => value === expected);
  }

  /** All three reductions, bottom-up; returns once the root footer shows the whole tree. */
  async settle(expected = usdText(M_OWN + G_OWN + L_OWN)): Promise<void> {
    this.refreshM();
    await this.hop!.command("G", "refresh");
    await this.hop!.command("M", "refresh");
    await this.waitForD(expected);
  }

  onCleanup(fn: () => void): void { this.cleanups.push(fn); }

  cleanup(): void {
    for (const fn of this.cleanups.splice(0)) { try { fn(); } catch { /* best effort */ } }
    this.hop?.kill();
    this.detach?.();
    this.channel?.close();
    try { this.footer?.stop(); } catch { /* best effort */ }
    rmSync(this.dir, { recursive: true, force: true });
  }
}

const usdText = (value: number) => `~$${value.toFixed(2)}`;
/** A fully known sum of `contributors` agents. */
const known = (knownUsd: number, contributors: number): CumulativeCost => ({ knownUsd, knownContributors: contributors, unknownContributors: 0, descendants: contributors });
/** G's whole subtree as M counts it: G's own usage plus its stored descendant value (L). */
const G_SUBTREE = known(G_OWN + L_OWN, 2);
function checkpointOf(dir: string, owner: string): { evictedBaseline: CumulativeCost; agents: Array<{ agentId: string; cost: CumulativeCost }> } {
  return JSON.parse(readFileSync(join(dir, "ws-agents", owner, ".cost-estimate", "checkpoint.json"), "utf8"));
}

test("three-level tree: the grandchild's and great-grandchild's usage reach the root footer only through the channel while M is parked", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  root.refreshM();
  assert.equal(root.footerD(), usdText(M_OWN), "before any report the root knows only M's own usage");
  assert.equal(root.changes, 0);

  // M runs no turn: it is only told to reduce its direct child, as its parked
  // process would on its own telemetry refresh.
  const g = await root.hop!.command("G", "refresh");
  assert.deepEqual(g.value, known(L_OWN, 1), "G's descendant value is L");
  const m = await root.hop!.command("M", "refresh");
  assert.deepEqual(m.value, G_SUBTREE, "M's descendant value is G's subtree");
  await root.waitForD(usdText(M_OWN + G_OWN + L_OWN));
  assert.ok(root.changes >= 1, "the root footer changed through accepted channel reports");
  assert.ok(m.sent >= 1, "M sent its descendant value over the channel");
  assert.ok(g.sent >= 1, "G sent its descendant value over the channel");
  assert.deepEqual(readOwnership(root.recordM.ownership!.home)!.telemetry?.descendantUsage, G_SUBTREE);
  assert.equal(root.recordM.telemetry?.estimatedUsd, M_OWN, "M's own usage is still the root's own reduction");
});

test("unchanged values send no further usage message at any hop", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  const beforeG = await root.hop!.command("G", "stats");
  const beforeM = await root.hop!.command("M", "stats");
  const changes = root.changes;
  for (let i = 0; i < 3; i += 1) {
    await root.hop!.command("G", "refresh");
    await root.hop!.command("M", "refresh");
    root.refreshM();
  }
  // G's and M's counters are incremented synchronously inside the refresh that
  // would send, so a reply after the refresh already reflects any send.
  assert.equal((await root.hop!.command("G", "stats")).sent, beforeG.sent, "G sent nothing for an unchanged value");
  assert.equal((await root.hop!.command("M", "stats")).sent, beforeM.sent, "M sent nothing for an unchanged value");
  assert.equal(root.changes, changes);
  assert.equal(root.footerD(), usdText(M_OWN + G_OWN + L_OWN));
});

test("child restart: M relaunched on a new generation rebuilds the same value; the root total is unchanged", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  await root.hop!.shutdown();
  const hop = await root.launch(true);
  const stats = await hop.command("M", "stats");
  assert.equal(stats.childPresent, true, "G was revived from M's sidecar, dormant");
  assert.equal(stats.childProcessAlive, false);
  assert.deepEqual(stats.value, G_SUBTREE, "M's rebuilt value is G's stored subtree, counted once");
  assert.ok(stats.sent >= 1, "the restarted M republished its value");
  await waitFor("the root to accept the new launch's report", () => root.recordM.descendantUsageOrder, order => order?.generation === root.generation);
  root.refreshM();
  assert.equal(root.footerD(), usdText(M_OWN + G_OWN + L_OWN), "no double count across the child restart");
});

test("parent restart: a fresh root rebuilt from M's ownership telemetry keeps the total, and M's next launch does not double count", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  const total = usdText(M_OWN + G_OWN + L_OWN);

  // Root shutdown: the footer persists the checkpoint and M (with G) stops.
  root.footer.stop();
  await root.hop!.shutdown();
  const home = root.recordM.ownership!.home;
  const durable = readOwnership(home)!;
  assert.deepEqual(durable.telemetry?.descendantUsage, G_SUBTREE, "the reported value is durable in M's ownership telemetry");

  // Root restart: a new registry and footer on the same root storage.
  const { version, ownerSessionId, agentId, role, sessionPath } = durable;
  const revived = {
    agentId, sessionPath: sessionPath!, ownership: { version, ownerSessionId, agentId, home, role, sessionPath }, systemPromptPath: "usage-hop.md",
    telemetry: durable.telemetry, wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker", streaming: false, running: false, reportLog: [],
  } as RpcAgentRecord;
  root.mountRegistry(revived);
  assert.equal(root.footerD(), total, "the restarted root rebuilds the total from durable state");

  // M's next launch (a child restart under the new root) reports on a higher generation.
  const hop = await root.launch(true);
  assert.deepEqual((await hop.command("M", "stats")).value, G_SUBTREE);
  await waitFor("the new root to accept M's report", () => root.recordM.descendantUsageOrder, order => order?.generation === root.generation);
  root.refreshM();
  assert.equal(root.footerD(), total, "no double count across the parent restart");
});

test("resume: a value changed while M is disconnected arrives once through the reconnect hello", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  const channel = root.channel!;
  const hop = root.hop!;
  const mPid = Number(hop.ready.get("M")!.pid);
  const sentBefore = (await hop.command("M", "stats")).sent;

  // Drop M's live connection from the root side, then hold the single live
  // slot with a same-credential stand-in so M's reconnect is refused `busy`
  // until the test releases it: the change below is deterministically
  // evaluated while M is disconnected.
  let squatter: ChildChannel | undefined;
  for (let attempt = 0; !squatter; attempt += 1) {
    assert.ok(attempt < 20, "could not hold M disconnected");
    await waitFor("M's live connection", () => channel.live, live => !!live);
    const dropped = new Promise<void>(resolve => { const off = channel.onDisconnect(() => { off(); resolve(); }); });
    channel.live!.close();
    await dropped;
    try { squatter = await ChildChannel.connect({ endpoint: channel.endpoint, credential: channel.credential, generation: channel.generation }, { reconnect: false, helloTimeoutMs: 2_000 }); }
    catch { /* M reconnected first; drop it again */ }
  }
  root.onCleanup(() => squatter?.close());
  await waitFor("M to see the drop", () => hop.command("M", "stats"), stats => !stats.connected);

  // L's usage grows; G reduces it and reports to M, whose send is skipped.
  writeSession(root.sessions.L, "l-session", [L_OWN, L_OWN]);
  await hop.command("G", "refresh");
  const parked = await waitFor("M to store G's new report", () => hop.command("M", "stats"), stats => stats.stored?.knownUsd === 2 * L_OWN);
  assert.equal(parked.connected, false);
  assert.equal(parked.sent, sentBefore, "M's send was skipped while disconnected");
  assert.deepEqual(parked.value, known(G_OWN + 2 * L_OWN, 2));
  assert.equal(root.footerD(), usdText(M_OWN + G_OWN + L_OWN), "the root has not seen the change yet");

  const changes = root.changes;
  const resumed = new Promise<Record<string, unknown>>(resolve => {
    const off = channel.onConnection((_conn, hello) => { if (hello.pid === mPid && hello.reconnect) { off(); resolve(hello.resume); } });
  });
  squatter.close();
  const resume = (await resumed)[DESCENDANT_USAGE_RESUME_KEY] as { seq: number; usage: CumulativeCost };
  assert.ok(resume, "the reconnect hello carries the latest descendant value");
  assert.deepEqual(resume.usage, known(G_OWN + 2 * L_OWN, 2));
  await root.waitForD(usdText(M_OWN + G_OWN + 2 * L_OWN));
  await new Promise(resolve => setTimeout(resolve, 100));
  assert.equal(root.changes, changes + 1, "the resumed value was accepted exactly once");
  assert.deepEqual(root.recordM.descendantUsageOrder, { generation: root.generation, seq: resume.seq });
  assert.equal((await hop.command("M", "stats")).sent, sentBefore, "the value arrived through the hello, not a resend");
  assert.equal(root.footerD(), usdText(M_OWN + G_OWN + 2 * L_OWN));
});

test("eviction then restart: M folds G's whole subtree into its evicted baseline once, and the root total survives M's restart", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  const total = usdText(M_OWN + G_OWN + L_OWN);

  const evicted = await root.hop!.command<Stats & { result: { ok: boolean; evictedLabel?: string } }>("M", "evict");
  assert.deepEqual(evicted.result, { ok: true, evictedLabel: "G" });
  assert.equal(evicted.childPresent, false, "G left M's registry");
  assert.deepEqual(checkpointOf(root.dir, "m-session").evictedBaseline, G_SUBTREE, "the fold is G's own usage plus its stored descendant value");
  assert.deepEqual(evicted.value, G_SUBTREE, "eviction moves G's subtree into the baseline without changing M's value");
  assert.equal(root.footerD(), total);

  await root.hop!.shutdown();
  const hop = await root.launch(true);
  const stats = await hop.command("M", "stats");
  assert.equal(stats.childPresent, false, "no evicted child is revived");
  assert.deepEqual(stats.value, G_SUBTREE, "the restarted M rebuilds its value from the checkpoint alone");
  assert.deepEqual(checkpointOf(root.dir, "m-session").evictedBaseline, G_SUBTREE, "folded exactly once");
  await waitFor("the root to accept the restarted M's report", () => root.recordM.descendantUsageOrder, order => order?.generation === root.generation);
  root.refreshM();
  assert.equal(root.footerD(), total, "the root total is unchanged");
});

test("a dormant direct child stays in its parent's sum", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  const stopped = await root.hop!.command("M", "stop-child");
  assert.equal(stopped.childProcessAlive, false, "G's process exited");
  assert.equal(stopped.childPresent, true, "G's record is kept, dormant");
  assert.deepEqual(stopped.value, G_SUBTREE);
  assert.equal(root.footerD(), usdText(M_OWN + G_OWN + L_OWN));

  // The dormant G's own usage grows (its final flush); M still reduces it and
  // keeps G's stored descendant value (L) in the sum.
  writeSession(root.sessions.G, "g-session", [G_OWN, G_OWN]);
  const refreshed = await root.hop!.command("M", "refresh");
  assert.deepEqual(refreshed.value, known(2 * G_OWN + L_OWN, 2));
  await root.waitForD(usdText(M_OWN + 2 * G_OWN + L_OWN));
});

test("no hop reads any session other than its direct child's, across a restart", { timeout: TEST_TIMEOUT_MS }, async t => {
  const root = await Root.start(t);
  await root.settle();
  const own = (reads: string[], child: string, hop: string) => {
    assert.ok(reads.includes(child), `positive control: ${hop} read its direct child's session`);
    assert.deepEqual(reads.filter(path => path !== child), [], `${hop} read no other session`);
  };
  own((await root.hop!.command("G", "stats")).sessionReads, root.sessions.L, "G");
  own((await root.hop!.command("M", "stats")).sessionReads, root.sessions.G, "M");

  await root.hop!.shutdown();
  const hop = await root.launch(true);
  await hop.command("M", "refresh");
  own((await hop.command("M", "stats")).sessionReads, root.sessions.G, "restarted M");
});

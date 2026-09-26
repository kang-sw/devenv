/**
 * One hop of the multi-process descendant-usage tree driven by
 * `test/agent-usage-rollup.integration.test.ts`. No Pi, no LLM: the hop runs
 * the production roll-up modules directly and mirrors the production wiring.
 *
 * - Child side (`index.ts`): reads and deletes its channel bootstrap,
 *   `ChildChannel.connect`s, installs one `createDescendantUsageReporter`,
 *   revives its registry through the production sidecar path on a restart,
 *   points the reporter at `descendantUsageValue(registry)` and evaluates once.
 * - Parent side (`spawner.ts`): one direct child. A process child gets its own
 *   `ParentChannel` and `attachDescendantUsage(record, channel, () =>
 *   evaluateDescendantUsage())` (the spawner's hook minus the footer); a file
 *   child is a record whose session file the test writes.
 *
 * The tree is `HOP_CHAIN` (`[{ name, session }]`); this hop is entry
 * `HOP_INDEX`, owns storage `session`, and its direct child is the next entry.
 * Commands arrive as JSON lines on stdin (`{ id, hop, cmd }`); a command for
 * another hop is forwarded to the process child, whose stdout lines are
 * forwarded up. Replies are `{ id, hop, ... }` lines; `{ event: "ready" }`
 * announces a started hop. Every `.jsonl` read is recorded so the test can
 * prove a hop reads no session but its direct child's.
 *
 * Guarded by an env marker because `node --test` also loads every file under
 * `test/`.
 */
import fs from "node:fs";
import { spawn, type ChildProcess } from "node:child_process";
import { syncBuiltinESMExports } from "node:module";
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";
import type { ParentChannel as ParentChannelT } from "../../src/agent-channel.ts";
import type { RpcAgentRecord } from "../../src/spawner.ts";

interface HopSpec { name: string; session: string }

if (process.env.WS_PI_USAGE_HOP === "1") await runHop();

async function runHop(): Promise<void> {
  const sessionReads = new Set<string>();
  const originalRead = fs.readFileSync;
  (fs as { readFileSync: unknown }).readFileSync = function (this: unknown, path: fs.PathOrFileDescriptor, ...rest: unknown[]) {
    if (String(path).endsWith(".jsonl")) sessionReads.add(String(path));
    return (originalRead as (...args: unknown[]) => unknown).call(this, path, ...rest);
  };
  // The telemetry refresh reads a child session incrementally through a file descriptor.
  const originalOpen = fs.openSync;
  (fs as { openSync: unknown }).openSync = function (this: unknown, path: fs.PathLike, ...rest: unknown[]) {
    if (String(path).endsWith(".jsonl")) sessionReads.add(String(path));
    return (originalOpen as (...args: unknown[]) => unknown).call(this, path, ...rest);
  };
  syncBuiltinESMExports();

  const [
    { ChildChannel, ParentChannel, readAndDeleteChannelBootstrap },
    { allocateAgentHome, createAgentStorageContext, persistOwnershipTelemetry, updateOwnership },
    { descendantUsageValue, persistAgentCostCheckpoint, registerAgentCostOwner },
    { DESCENDANT_USAGE_MESSAGE, attachDescendantUsage, createDescendantUsageReporter, descendantUsageOf, descendantUsageReporterRef, evaluateDescendantUsage },
    { evictForCapacity, refreshAgentTelemetry },
    { captureOrphans, noSessionSidecarPath, readAndClearSidecarAt, reviveOrphans, writeSidecarAt },
  ] = await Promise.all([
    import("../../src/agent-channel.ts"),
    import("../../src/agent-storage.ts"),
    import("../../src/agent-cost.ts"),
    import("../../src/agent-usage-rollup.ts"),
    import("../../src/spawner.ts"),
    import("../../src/agent-sidecar.ts"),
  ]);

  const chain = JSON.parse(process.env.HOP_CHAIN!) as HopSpec[];
  const index = Number(process.env.HOP_INDEX);
  const self = chain[index];
  const child = chain[index + 1];
  const childIsProcess = index + 2 < chain.length;
  const restart = process.env.HOP_RESTART === "1";
  const out = (line: Record<string, unknown>) => process.stdout.write(`${JSON.stringify(line)}\n`);

  // Child side: exactly the index.ts factory + session_start order.
  const bootstrap = readAndDeleteChannelBootstrap(process.env);
  if (!bootstrap) throw new Error("usage-hop: launched without a channel bootstrap");
  const channel = await ChildChannel.connect(bootstrap);
  let sent = 0;
  const send = channel.send.bind(channel);
  channel.send = (msg: Record<string, unknown>) => { send(msg); if (msg.t === DESCENDANT_USAGE_MESSAGE) sent += 1; };
  descendantUsageReporterRef.current = createDescendantUsageReporter(channel);

  const storage = createAgentStorageContext(self.session, process.env.HOP_ROOT!);
  const sidecar = noSessionSidecarPath(storage.root, storage.ownerSessionId);
  const registry: Map<string, RpcAgentRecord> = new Map();
  registerAgentCostOwner(registry, storage);
  if (restart) reviveOrphans(registry, readAndClearSidecarAt(sidecar));
  descendantUsageReporterRef.current.setSource(() => descendantUsageValue(registry));
  descendantUsageReporterRef.current.evaluate();

  // Parent side: one direct child, created only on a first launch (a restart
  // keeps whatever the sidecar revived, dormant).
  let childProc: ChildProcess | undefined;
  let childChannel: ParentChannelT | undefined;
  if (!restart) {
    const ownership = allocateAgentHome(storage, child.name, "worker");
    const record = {
      agentId: child.name, sessionPath: ownership.sessionPath!, ownership, systemPromptPath: "usage-hop.md",
      telemetry: { version: 1 as const, origin: { sessionId: child.session, sessionPath: ownership.sessionPath!, emptyPrefix: true as const } },
      wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker", streaming: false, running: false, reportLog: [],
    } as RpcAgentRecord;
    persistOwnershipTelemetry(ownership.home, record.telemetry);
    registry.set(record.agentId, record);
    if (childIsProcess) await launchChild(record, 1);
  }

  async function launchChild(record: RpcAgentRecord, generation: number): Promise<void> {
    const bound = await ParentChannel.bind(generation);
    childChannel = bound;
    record.channel = bound;
    record.launchGeneration = generation;
    attachDescendantUsage(record, bound, () => evaluateDescendantUsage());
    const proc = spawn(process.execPath, [fileURLToPath(import.meta.url)], {
      env: { ...process.env, ...bound.bootstrapEnv(), HOP_INDEX: String(index + 1), HOP_RESTART: "0" },
      stdio: ["pipe", "pipe", "inherit"],
    });
    childProc = proc;
    proc.once("exit", () => { if (childProc === proc) childProc = undefined; });
    const ready = new Promise<void>((resolve, reject) => {
      proc.once("exit", code => reject(new Error(`usage-hop: ${child.name} exited before ready (${code})`)));
      createInterface({ input: proc.stdout! }).on("line", line => {
        process.stdout.write(`${line}\n`);
        try { const parsed = JSON.parse(line); if (parsed.event === "ready" && parsed.hop === child.name) resolve(); } catch { /* forwarded verbatim */ }
      });
    });
    await bound.hello();
    await ready;
  }

  /** Stops the process child the way the parent's stopAll would: the child persists and exits; the record stays, dormant. */
  async function stopChild(): Promise<void> {
    const proc = childProc;
    if (proc) {
      const exited = new Promise<void>(resolve => { proc.once("exit", () => resolve()); });
      proc.stdin!.write(`${JSON.stringify({ id: `stop-${child.name}`, hop: child.name, cmd: "shutdown" })}\n`);
      await exited;
    }
    childChannel?.close();
    childChannel = undefined;
    const record = registry.get(child.name);
    if (record) {
      record.channel = undefined;
      if (record.ownership) updateOwnership(record.ownership.home, { liveness: { lifecycle: "stopped", running: false, observedAt: Date.now() } });
    }
  }

  const stats = () => {
    const record = registry.get(child.name);
    return {
      sent,
      connected: channel.connected,
      value: descendantUsageValue(registry),
      stored: record ? descendantUsageOf(record) : undefined,
      childPresent: !!record,
      childSessionPath: record?.sessionPath,
      childProcessAlive: !!childProc,
      sessionReads: [...sessionReads],
    };
  };

  async function handle(cmd: string): Promise<Record<string, unknown>> {
    switch (cmd) {
      case "refresh": {
        const record = registry.get(child.name);
        if (!record) throw new Error(`no child ${child.name}`);
        refreshAgentTelemetry(record);
        return stats();
      }
      case "stats": return stats();
      case "stop-child": await stopChild(); return stats();
      case "evict": {
        await stopChild();
        const result = evictForCapacity(registry, 1);
        return { result, ...stats() };
      }
      case "shutdown": {
        await stopChild();
        writeSidecarAt(sidecar, captureOrphans(registry));
        const persisted = persistAgentCostCheckpoint(registry);
        channel.close();
        return { persisted, exiting: true };
      }
      default: throw new Error(`unknown command ${cmd}`);
    }
  }

  const input = createInterface({ input: process.stdin });
  input.on("line", line => {
    const msg = JSON.parse(line) as { id: string; hop: string; cmd: string };
    if (msg.hop !== self.name) {
      if (childProc) childProc.stdin!.write(`${line}\n`);
      else out({ id: msg.id, hop: msg.hop, error: `${msg.hop} is not running` });
      return;
    }
    void handle(msg.cmd).then(
      result => {
        const line = `${JSON.stringify({ id: msg.id, hop: self.name, ...result })}\n`;
        if (result.exiting) process.stdout.write(line, () => process.exit(0));
        else process.stdout.write(line);
      },
      error => out({ id: msg.id, hop: self.name, error: String(error) }),
    );
  });
  // The parent went away (or killed this hop's stdin): take the process child down too.
  input.on("close", () => { childProc?.kill("SIGKILL"); process.exit(0); });
  process.on("exit", () => { childProc?.kill("SIGKILL"); });

  const record = registry.get(child.name);
  out({ event: "ready", hop: self.name, pid: process.pid, generation: channel.generation, childSessionPath: record?.sessionPath });
}

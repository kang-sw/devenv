/**
 * One-edge subtree lifecycle state, carried hop by hop on the parent-child
 * control channel (`agent-channel.ts`). Pi's raw agent_settled is not
 * vetoable, so a parent learns whether a direct child still has descendant
 * work only from the snapshot that child publishes.
 *
 * Child side: `SubtreeUpstream` sends a full snapshot, stamped with a revision
 * that rises on every effective change, whenever that state changes. The
 * revision lives as long as the process (one launch generation) and survives
 * reconnects; the latest snapshot rides every reconnect hello's resume
 * section. `beginSubtreeDispatch` is the busy-before-dispatch fence: no
 * grandchild starts until the parent has acknowledged the busy revision.
 * Each snapshot also carries the child's own wake accounting (`turnOwed`,
 * `turnsStarted`), so the parent never has to guess from revisions whether a
 * wake turn is coming.
 *
 * Parent side: `observeSubtreeChannel` keeps the last revision it has seen for
 * one launch (one `ParentChannel`), ignores lower ones, and acknowledges what
 * it applied. A not-yet-connected or disconnected channel reads as waiting.
 */
import type { ChildChannel, ParentChannel } from "./agent-channel.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner.ts";

export type SubtreeDescendantRole = "worker" | "execute" | "fork" | "explore";
export interface SubtreeDescendant {
  id: string;
  parentId: string | null;
  depth: number;
  role: SubtreeDescendantRole;
  live: boolean;
}
export interface SubtreeSnapshot {
  outstanding: number;
  active: number;
  deliveries: number;
  delegated: boolean;
  /**
   * The child owes itself a turn for deliveries it has already enqueued that
   * have not started one yet (a pending wake, or a turn-boundary batch Pi
   * continues with). Not part of `subtreeWaiting`: it gates only the release
   * of a settle the parent already holds.
   */
  turnOwed: boolean;
  /** Own turns (`agent_start`) this child process has started; rises monotonically within one launch generation. */
  turnsStarted: number;
  /** Rises on every effective change within one launch generation; 0 for a publisher with no parent. */
  revision: number;
  /** Advisory display identity only; never an input to wait/settle. */
  descendants: SubtreeDescendant[];
}
export const MAX_SUBTREE_DESCENDANTS = 128;
export const MAX_SUBTREE_DESCENDANT_DEPTH = 8;
const DESCENDANT_ROLES = new Set<SubtreeDescendantRole>(["worker", "execute", "fork", "explore"]);

/** Channel message types and the hello resume key this module owns. */
export const SUBTREE_MESSAGE = "subtree";
export const SUBTREE_ACK_MESSAGE = "subtree-ack";
export const SUBTREE_RESUME_KEY = "subtree";
/**
 * Bound on the busy fence's acknowledgment wait. A loopback round trip takes
 * well under a millisecond (see the latency test); the bound only has to
 * outlast a parent whose event loop is briefly busy, and a refusal is the
 * same fail-closed outcome a failed busy publication always had.
 */
export const SUBTREE_ACK_TIMEOUT_MS = 5_000;

export function parseSubtreeSnapshot(raw: unknown): SubtreeSnapshot | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const value = raw as Partial<SubtreeSnapshot> & { descendants?: unknown };
  // Every field that feeds wait or release is required: a snapshot without the
  // wake accounting is rejected (unacknowledged), so the view keeps its prior
  // state, initially waiting.
  if (![value.outstanding, value.active, value.deliveries, value.turnsStarted, value.revision].every(n => Number.isSafeInteger(n) && (n as number) >= 0)
    || typeof value.delegated !== "boolean" || typeof value.turnOwed !== "boolean") return undefined;
  const descendants = Array.isArray(value.descendants)
    ? value.descendants.filter((row): row is SubtreeDescendant => {
      if (!row || typeof row !== "object") return false;
      const candidate = row as Partial<SubtreeDescendant>;
      return typeof candidate.id === "string" && candidate.id.length > 0 &&
        (candidate.parentId === null || (typeof candidate.parentId === "string" && candidate.parentId.length > 0)) &&
        Number.isSafeInteger(candidate.depth) && (candidate.depth as number) >= 0 && (candidate.depth as number) <= MAX_SUBTREE_DESCENDANT_DEPTH &&
        typeof candidate.role === "string" && DESCENDANT_ROLES.has(candidate.role as SubtreeDescendantRole) &&
        typeof candidate.live === "boolean";
    }).slice(0, MAX_SUBTREE_DESCENDANTS)
    : [];
  return {
    outstanding: value.outstanding!, active: value.active!, deliveries: value.deliveries!,
    delegated: value.delegated, turnOwed: value.turnOwed, turnsStarted: value.turnsStarted!, revision: value.revision!, descendants,
  };
}

export function subtreeWaiting(snapshot: SubtreeSnapshot | undefined): boolean {
  // An unknown subtree is never permission to stop a process.
  return !snapshot || snapshot.outstanding > 0 || snapshot.active > 0 || snapshot.deliveries > 0;
}

type SubtreeState = Omit<SubtreeSnapshot, "revision">;
interface AckWaiter { revision: number; resolve: () => void; reject: (error: Error) => void; timer: NodeJS.Timeout }

/** The child half: one per process, shared by every publisher that process installs. */
export class SubtreeUpstream {
  private revisionCounter = 0;
  private acked = 0;
  private lastState: string | undefined;
  private latest: SubtreeSnapshot | undefined;
  private readonly waiters = new Set<AckWaiter>();
  private readonly channel: ChildChannel;
  private readonly ackTimeoutMs: number;

  constructor(channel: ChildChannel, opts: { ackTimeoutMs?: number } = {}) {
    this.channel = channel;
    this.ackTimeoutMs = opts.ackTimeoutMs ?? SUBTREE_ACK_TIMEOUT_MS;
    channel.onMessage(msg => {
      if (msg.t === SUBTREE_ACK_MESSAGE && Number.isSafeInteger(msg.revision)) this.acknowledge(msg.revision as number);
    });
    channel.onDisconnect(() => this.refuseWaiters("the channel to the parent disconnected"));
    // The hello's resume section is evaluated before the welcome, while
    // `connected` is still false, so a change published in that handshake
    // window is neither in the hello nor sent. Resending the latest snapshot
    // once connected closes the gap; the parent reads an equal revision as a
    // duplicate.
    channel.onReconnect(() => this.send());
  }

  /** Hello resume section: a reconnect restores the parent's view from the latest snapshot. */
  resume(): Record<string, unknown> {
    return this.latest ? { [SUBTREE_RESUME_KEY]: this.latest } : {};
  }

  /** Sends only an effective change; a disconnected send is carried by the next reconnect hello. */
  publish(state: SubtreeState): SubtreeSnapshot {
    const key = JSON.stringify(state);
    if (key === this.lastState && this.latest) return this.latest;
    this.lastState = key;
    this.latest = { ...state, revision: ++this.revisionCounter };
    this.send();
    return this.latest;
  }

  private send(): void {
    try { if (this.latest && this.channel.connected) this.channel.send({ t: SUBTREE_MESSAGE, snapshot: this.latest }); }
    catch { /* a socket destroyed but not yet reported ended; the resume section carries it */ }
  }

  /**
   * Nothing when the parent has already acknowledged the latest revision;
   * otherwise a wait that resolves on that acknowledgment and refuses when the
   * channel is down, drops, or the bound passes.
   */
  fence(): Promise<void> | undefined {
    const revision = this.revisionCounter;
    if (this.acked >= revision) return undefined;
    if (!this.channel.connected) return Promise.reject(fenceRefusal("the channel to the parent is down"));
    return new Promise<void>((resolve, reject) => {
      const waiter: AckWaiter = {
        revision,
        resolve,
        reject,
        timer: setTimeout(() => {
          this.waiters.delete(waiter);
          reject(fenceRefusal(`the parent did not acknowledge revision ${revision} within ${this.ackTimeoutMs}ms`));
        }, this.ackTimeoutMs),
      };
      waiter.timer.unref?.();
      this.waiters.add(waiter);
    });
  }

  private acknowledge(revision: number): void {
    this.acked = Math.max(this.acked, revision);
    for (const waiter of [...this.waiters]) {
      if (waiter.revision > this.acked) continue;
      this.waiters.delete(waiter);
      clearTimeout(waiter.timer);
      waiter.resolve();
    }
  }

  private refuseWaiters(reason: string): void {
    for (const waiter of [...this.waiters]) {
      this.waiters.delete(waiter);
      clearTimeout(waiter.timer);
      waiter.reject(fenceRefusal(reason));
    }
  }
}

function fenceRefusal(reason: string): Error {
  return new Error(`ws-pi-agent: nested dispatch refused: ${reason}`);
}

/** What the parent currently knows about one direct child's subtree. */
export interface SubtreeView {
  waiting: boolean;
  /** Last accepted snapshot of this launch; kept across a disconnect for advisory identity. */
  snapshot?: SubtreeSnapshot;
}

/**
 * The parent half for one launch. Reports the initial view (waiting: nothing
 * received yet) synchronously, then every accepted snapshot and every
 * disconnect. Each received snapshot is acknowledged with the highest
 * revision applied, after `onView` has applied it.
 */
export function observeSubtreeChannel(channel: ParentChannel, onView: (view: SubtreeView) => void): () => void {
  let lastSeen: number | undefined;
  let latest: SubtreeSnapshot | undefined;
  const receive = (raw: unknown) => {
    const snapshot = parseSubtreeSnapshot(raw);
    if (!snapshot) return;
    // Equal is a duplicate of what was applied (or a reconnect restoring it
    // after a disconnect marked the view waiting); only lower is stale.
    if (lastSeen === undefined || snapshot.revision >= lastSeen) {
      lastSeen = snapshot.revision;
      latest = snapshot;
      onView({ waiting: subtreeWaiting(snapshot), snapshot });
    }
    try { channel.send({ t: SUBTREE_ACK_MESSAGE, revision: lastSeen }); }
    catch { /* the connection is gone; the child's fence refuses on its own disconnect */ }
  };
  const offMessage = channel.onMessage(msg => { if (msg.t === SUBTREE_MESSAGE) receive(msg.snapshot); });
  const offConnection = channel.onConnection((_conn, hello) => {
    const resumed = hello.resume[SUBTREE_RESUME_KEY];
    if (resumed !== undefined) receive(resumed);
  });
  const offDisconnect = channel.onDisconnect(() => onView({ waiting: true, snapshot: latest }));
  onView({ waiting: true });
  return () => { offMessage(); offConnection(); offDisconnect(); };
}

/** This process's own-turn accounting, as `SubtreeSnapshot.turnOwed` / `turnsStarted` report it. */
export interface OwnTurnState { owed: boolean; started: number }

interface Publisher {
  delegated: boolean;
  dispatching: number;
  upstream?: SubtreeUpstream;
  deliveries: () => number;
  ownTurn: () => OwnTurnState;
}
const publishers = new WeakMap<RpcAgentRegistry, Publisher>();

export function installSubtreePublisher(
  registry: RpcAgentRegistry,
  upstream: SubtreeUpstream | undefined,
  deliveries: () => number,
  ownTurn: () => OwnTurnState,
): void {
  publishers.set(registry, { delegated: false, dispatching: 0, upstream, deliveries, ownTurn });
  publishSubtree(registry);
}
export function subtreeOutstanding(registry: RpcAgentRegistry): number {
  let count = 0;
  for (const record of registry.values()) {
    // A held settle is a terminal still to come: counting it keeps this
    // process from reading quiescent upstream between a child's view clearing
    // and that child's wake turn (or held settle's release).
    const heldSettle = record.heldSettlementGeneration !== undefined && record.heldSettlementGeneration === record.workGeneration;
    if (record.waitingOnChildren || heldSettle || (record.terminalDelivery && record.terminalDelivery.state !== "enqueued")) count++;
  }
  return count;
}
function descendantRole(record: RpcAgentRecord): SubtreeDescendantRole {
  if (record.spawnRole === "execute-worker") return "execute";
  if (record.spawnRole === "fork") return "fork";
  if (record.spawnRole === "explore") return "explore";
  return "worker";
}

/** Builds a bounded, parent-before-child advisory tree. Count accounting is intentionally separate. */
function subtreeDescendants(registry: RpcAgentRegistry): SubtreeDescendant[] {
  const rows: SubtreeDescendant[] = [];
  const included = new Set<string>();
  for (const record of registry.values()) {
    if (rows.length >= MAX_SUBTREE_DESCENDANTS || included.has(record.agentId)) continue;
    rows.push({ id: record.agentId, parentId: null, depth: 0, role: descendantRole(record), live: record.client !== undefined });
    included.add(record.agentId);
    for (const nested of record.subtreeDescendants ?? []) {
      if (rows.length >= MAX_SUBTREE_DESCENDANTS) break;
      const depth = nested.depth + 1;
      const parentId = nested.parentId ?? record.agentId;
      if (depth > MAX_SUBTREE_DESCENDANT_DEPTH || !included.has(parentId) || included.has(nested.id)) continue;
      rows.push({ ...nested, parentId, depth });
      included.add(nested.id);
    }
  }
  return rows;
}

/** Recomputes this process's snapshot and sends it upstream when it changed. Never throws. */
export function publishSubtree(registry: RpcAgentRegistry | undefined, dispatched = false): SubtreeSnapshot | undefined {
  if (!registry) return undefined;
  const p = publishers.get(registry);
  if (!p) return undefined;
  if (dispatched) p.delegated = true;
  const outstanding = subtreeOutstanding(registry);
  let active = p.dispatching;
  for (const r of registry.values()) {
    if (r.running || r.streaming) active++;
  }
  p.delegated ||= registry.size > 0;
  const ownTurn = p.ownTurn();
  const state: SubtreeState = {
    outstanding, active, deliveries: p.deliveries(), delegated: p.delegated,
    turnOwed: ownTurn.owed, turnsStarted: ownTurn.started, descendants: subtreeDescendants(registry),
  };
  return p.upstream ? p.upstream.publish(state) : { ...state, revision: 0 };
}

/**
 * The busy-before-dispatch fence: publishes the dispatch as active and waits
 * for the parent's acknowledgment before the caller may start a child.
 * Returns the finish callback synchronously when no acknowledgment is owed (a
 * process with no parent channel has no fence), so a caller's synchronous
 * check-and-claim is not split by an await it does not need. Refusal undoes
 * the admission.
 */
export function beginSubtreeDispatch(registry: RpcAgentRegistry): (() => void) | Promise<() => void> {
  const publisher = publishers.get(registry);
  if (!publisher) return () => {};
  publisher.dispatching++;
  let finished = false;
  const finish = () => {
    if (finished) return;
    finished = true;
    publisher.dispatching--;
    publishSubtree(registry);
  };
  publishSubtree(registry);
  let fence: Promise<void> | undefined;
  try { fence = publisher.upstream?.fence(); }
  catch (error) { finish(); throw error; }
  if (!fence) return finish;
  return fence.then(() => finish, (error: unknown) => { finish(); throw error; });
}

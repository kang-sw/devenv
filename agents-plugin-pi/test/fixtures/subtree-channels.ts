/**
 * Deterministic stand-ins for the two halves of the control channel as the
 * subtree state uses them (`subtree-lifecycle.ts`): the child's uplink, whose
 * acknowledgments and drops the test drives, and the parent's channel, whose
 * snapshot deliveries, reconnect hellos, and drops the test drives. Real-socket
 * behavior is covered by `subtree-lifecycle.test.ts` and the channel suites.
 *
 * `node --test` also runs this file (default glob); it defines no tests.
 */
import { ChildChannel, ParentChannel, readAndDeleteChannelBootstrap, type ChannelBindOptions, type ChannelHello } from "../../src/agent-channel.ts";
import { SUBTREE_ACK_MESSAGE, SUBTREE_MESSAGE, SubtreeUpstream, type OwnTurnState, type SubtreeSnapshot } from "../../src/subtree-lifecycle.ts";

type Listener<T extends unknown[]> = (...args: T) => void;

/** Child-side uplink: records sends; `ack`/`disconnect` drive the parent's side. Starts connected. */
export function fakeUplink() {
  const messageListeners = new Set<Listener<[Record<string, unknown>]>>();
  const disconnectListeners = new Set<Listener<[]>>();
  const reconnectListeners = new Set<Listener<[]>>();
  const link = {
    connected: true,
    sent: [] as Array<Record<string, unknown>>,
    send(msg: Record<string, unknown>) {
      if (!link.connected) throw new Error("ws-pi-channel: not connected to the parent");
      link.sent.push(msg);
    },
    onMessage(cb: Listener<[Record<string, unknown>]>) { messageListeners.add(cb); return () => { messageListeners.delete(cb); }; },
    onDisconnect(cb: Listener<[]>) { disconnectListeners.add(cb); return () => { disconnectListeners.delete(cb); }; },
    onReconnect(cb: Listener<[]>) { reconnectListeners.add(cb); return () => { reconnectListeners.delete(cb); }; },
    ack(revision: number) { for (const cb of [...messageListeners]) cb({ t: SUBTREE_ACK_MESSAGE, revision, gen: 1 }); },
    disconnect() { link.connected = false; for (const cb of [...disconnectListeners]) cb(); },
    reconnect() { link.connected = true; for (const cb of [...reconnectListeners]) cb(); },
    snapshots(): SubtreeSnapshot[] { return link.sent.filter(msg => msg.t === SUBTREE_MESSAGE).map(msg => msg.snapshot as SubtreeSnapshot); },
    /** Typed view for `new SubtreeUpstream(...)`. */
    get channel(): ChildChannel { return link as unknown as ChildChannel; },
  };
  return link;
}

/** Parent-side channel: `deliver`/`hello`/`drop` drive the observer; acknowledgments are recorded. */
export function fakeParentChannel() {
  const messageListeners = new Set<Listener<[Record<string, unknown>]>>();
  const connectionListeners = new Set<Listener<[unknown, ChannelHello]>>();
  const disconnectListeners = new Set<Listener<[]>>();
  const channel = {
    acks: [] as number[],
    closed: false,
    onMessage(cb: Listener<[Record<string, unknown>]>) { messageListeners.add(cb); return () => { messageListeners.delete(cb); }; },
    onConnection(cb: Listener<[unknown, ChannelHello]>) { connectionListeners.add(cb); return () => { connectionListeners.delete(cb); }; },
    onDisconnect(cb: Listener<[]>) { disconnectListeners.add(cb); return () => { disconnectListeners.delete(cb); }; },
    send(msg: Record<string, unknown>) { if (msg.t === SUBTREE_ACK_MESSAGE) channel.acks.push(msg.revision as number); },
    close() { channel.closed = true; },
    deliver(snapshot: unknown) { for (const cb of [...messageListeners]) cb({ t: SUBTREE_MESSAGE, snapshot, gen: 1 }); },
    hello(resume: Record<string, unknown>) { for (const cb of [...connectionListeners]) cb({}, { reconnect: true, resume }); },
    drop() { for (const cb of [...disconnectListeners]) cb(); },
    /** Typed view for `observeSubtreeChannel` and `record.channel`. */
    get parent(): ParentChannel { return channel as unknown as ParentChannel; },
  };
  return channel;
}

export function quiescentSnapshot(revision: number, counts: Partial<SubtreeSnapshot> = {}): SubtreeSnapshot {
  return { outstanding: 0, active: 0, deliveries: 0, delegated: true, turnOwed: false, turnsStarted: 0, revision, descendants: [], ...counts };
}

/** Own-turn accessor for a publisher whose process never starts a turn of its own. */
export const idleOwnTurn = (): OwnTurnState => ({ owed: false, started: 0 });

export interface UntilOptions { timeoutMs?: number; intervalMs?: number }

/** Polls `condition` until it holds; throws naming `what` after the bound. */
export async function until(condition: () => boolean, what: string, { timeoutMs = 2_000, intervalMs = 5 }: UntilOptions = {}): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise(resolve => setTimeout(resolve, intervalMs));
  }
}

/**
 * A real parent/child channel pair in this process, the child's
 * `SubtreeUpstream` riding its reconnect hello. The caller closes both.
 */
export async function subtreeChannelPair(generation: number, bind: ChannelBindOptions) {
  const parent = await ParentChannel.bind(generation, bind);
  let upstream: SubtreeUpstream | undefined;
  const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { reconnect: false, resume: () => upstream?.resume() ?? {} });
  upstream = new SubtreeUpstream(child);
  return { parent, child, upstream };
}

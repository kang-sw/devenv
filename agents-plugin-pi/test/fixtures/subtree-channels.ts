/**
 * Deterministic stand-ins for the two halves of the control channel as the
 * subtree state uses them (`subtree-lifecycle.ts`): the child's uplink, whose
 * acknowledgments and drops the test drives, and the parent's channel, whose
 * snapshot deliveries, reconnect hellos, and drops the test drives. Real-socket
 * behavior is covered by `subtree-lifecycle.test.ts` and the channel suites.
 *
 * `node --test` also runs this file (default glob); it defines no tests.
 */
import type { ChannelHello, ChildChannel, ParentChannel } from "../../src/agent-channel.ts";
import type { SubtreeSnapshot } from "../../src/subtree-lifecycle.ts";

type Listener<T extends unknown[]> = (...args: T) => void;

/** Child-side uplink: records sends; `ack`/`disconnect` drive the parent's side. Starts connected. */
export function fakeUplink() {
  const messageListeners = new Set<Listener<[Record<string, unknown>]>>();
  const disconnectListeners = new Set<Listener<[]>>();
  const link = {
    connected: true,
    sent: [] as Array<Record<string, unknown>>,
    send(msg: Record<string, unknown>) {
      if (!link.connected) throw new Error("ws-pi-channel: not connected to the parent");
      link.sent.push(msg);
    },
    onMessage(cb: Listener<[Record<string, unknown>]>) { messageListeners.add(cb); return () => { messageListeners.delete(cb); }; },
    onDisconnect(cb: Listener<[]>) { disconnectListeners.add(cb); return () => { disconnectListeners.delete(cb); }; },
    ack(revision: number) { for (const cb of [...messageListeners]) cb({ t: "subtree-ack", revision, gen: 1 }); },
    disconnect() { link.connected = false; for (const cb of [...disconnectListeners]) cb(); },
    snapshots(): SubtreeSnapshot[] { return link.sent.filter(msg => msg.t === "subtree").map(msg => msg.snapshot as SubtreeSnapshot); },
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
    send(msg: Record<string, unknown>) { if (msg.t === "subtree-ack") channel.acks.push(msg.revision as number); },
    close() { channel.closed = true; },
    deliver(snapshot: unknown) { for (const cb of [...messageListeners]) cb({ t: "subtree", snapshot, gen: 1 }); },
    hello(resume: Record<string, unknown>) { for (const cb of [...connectionListeners]) cb({}, { reconnect: true, resume }); },
    drop() { for (const cb of [...disconnectListeners]) cb(); },
    /** Typed view for `observeSubtreeChannel` and `record.channel`. */
    get parent(): ParentChannel { return channel as unknown as ParentChannel; },
  };
  return channel;
}

export function quiescentSnapshot(revision: number, counts: Partial<SubtreeSnapshot> = {}): SubtreeSnapshot {
  return { outstanding: 0, active: 0, deliveries: 0, delegated: true, revision, descendants: [], ...counts };
}

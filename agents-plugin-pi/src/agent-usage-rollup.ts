/**
 * Descendant usage roll-up over the parent-child control channel
 * (260924-feat-pi-agent-channel-usage-rollup).
 *
 * Every hop reports one value upward: its **descendant** usage, the total of
 * everything below it and never its own usage. The parent already reduces
 * each direct child's own usage from that child's session file
 * (`refreshAgentTelemetry`), so a direct child's subtree total is that own
 * usage plus the child's last reported descendant value. No hop reads any
 * session other than its direct children's.
 *
 * A hop's descendant value is its cost estimate's direct total
 * (`descendantUsageValue` in `agent-cost.ts`): the subtree totals of its
 * registry-resident direct children plus its removed children (the
 * checkpoint's legacy evicted baseline and its per-child eviction records).
 *
 * Values, not deltas, travel so a replay is harmless. Ordering is per launch:
 * every report carries the launch generation (stamped by the channel) and a
 * per-launch sequence number. The parent keeps the highest sequence within a
 * generation, lets the first value of a newer generation replace the stored
 * one, and ignores anything older. The ordering state is live-only: launch
 * generations restart with the parent process, so a restarted parent accepts
 * the first report of the child's next launch.
 */
import { isDeepStrictEqual } from "node:util";
import type { ChildChannel, ParentChannel } from "./agent-channel.ts";
import { durableDescendantUsage, persistOwnershipDescendantUsage, readOwnership } from "./agent-storage.ts";
import { parseCumulativeCost, type CumulativeCost } from "./agent-telemetry.ts";
import type { RpcAgentRecord } from "./spawner.ts";

/** Channel message type `t` of a descendant-usage report: `{ t, gen, seq, usage }`. */
export const DESCENDANT_USAGE_MESSAGE = "descendant-usage";
/** Hello resume key carrying the child's latest report `{ seq, usage }` across a reconnect. */
export const DESCENDANT_USAGE_RESUME_KEY = "descendantUsage";

export interface DescendantUsageReport { seq: number; usage: CumulativeCost }

export function parseDescendantUsageReport(value: unknown): DescendantUsageReport | undefined {
  const report = value as { seq?: unknown; usage?: unknown } | null;
  if (!report || typeof report !== "object") return undefined;
  const seq = report.seq;
  const usage = parseCumulativeCost(report.usage);
  return typeof seq === "number" && Number.isSafeInteger(seq) && seq > 0 && usage ? { seq, usage } : undefined;
}

/**
 * A direct child's last reported descendant value. The record field survives
 * a telemetry reset; `telemetry.descendantUsage` is only a legacy fallback
 * (a record or snapshot written before the sibling ownership field).
 */
export function descendantUsageOf(record: Pick<RpcAgentRecord, "descendantUsage" | "telemetry">): CumulativeCost | undefined {
  return durableDescendantUsage(record);
}

/**
 * Revival: loads the durable value (the ownership record's sibling field,
 * else its legacy telemetry copy) into a dormant record. The durable value
 * wins over a snapshot's copy, which can be older.
 */
export function restoreDescendantUsage(record: RpcAgentRecord, metadata = record.ownership ? readOwnership(record.ownership.home) : undefined): void {
  const durable = durableDescendantUsage(metadata);
  if (durable) record.descendantUsage = durable;
}

/**
 * Parent side: applies one report from `record`'s launch `generation`.
 * Returns true only when the stored value changed. The accepted value is
 * written into the child's ownership record as its sibling
 * `descendantUsage` field, whether or not the child has own-usage telemetry;
 * the parent is that record's single writer.
 */
export function acceptDescendantUsage(record: RpcAgentRecord, generation: number, value: unknown): boolean {
  const report = parseDescendantUsageReport(value);
  if (!report) return false;
  const order = record.descendantUsageOrder;
  if (order && (generation < order.generation || (generation === order.generation && report.seq <= order.seq))) return false;
  record.descendantUsageOrder = { generation, seq: report.seq };
  const previous = descendantUsageOf(record);
  record.descendantUsage = report.usage;
  if (record.ownership) persistOwnershipDescendantUsage(record.ownership.home, report.usage);
  return !isDeepStrictEqual(previous, report.usage);
}

/**
 * Parent side: wires one launch's channel to `record`. Reports arrive as
 * channel messages and, after a reconnect, in the hello's resume section.
 * `onChanged` runs after a report changed the stored value.
 */
export function attachDescendantUsage(record: RpcAgentRecord, channel: ParentChannel, onChanged: () => void): () => void {
  const accept = (value: unknown) => {
    if (record.launchGeneration !== channel.generation) return;
    if (acceptDescendantUsage(record, channel.generation, value)) onChanged();
  };
  const offMessage = channel.onMessage(msg => { if (msg.t === DESCENDANT_USAGE_MESSAGE) accept(msg); });
  const offConnection = channel.onConnection((_conn, hello) => {
    const resumed = hello.resume[DESCENDANT_USAGE_RESUME_KEY];
    if (resumed !== undefined) accept(resumed);
  });
  return () => { offMessage(); offConnection(); };
}

export interface DescendantUsageReporter {
  /** Replaces the value source (a session replacement brings a new registry); `undefined` pauses evaluation. */
  setSource(source: (() => CumulativeCost | undefined) | undefined): void;
  /** Recomputes the value and sends it only when it changed since the last report. */
  evaluate(): void;
}

/**
 * Child side: one reporter per channel, so the sequence stays monotonic for
 * the whole launch across session replacements. The initial value is zero,
 * so a hop with nothing below it never sends. A send while disconnected is
 * not retried: the reconnect hello carries the latest report, and the
 * report is sent again once the reconnect completes, because a value
 * computed between the hello and the welcome is in neither. The parent's
 * sequence check drops the duplicate.
 */
export function createDescendantUsageReporter(channel: ChildChannel): DescendantUsageReporter {
  let source: (() => CumulativeCost | undefined) | undefined;
  let seq = 0;
  let latest: CumulativeCost = { knownUsd: 0, knownContributors: 0, unknownContributors: 0, descendants: 0 };
  channel.provideResume(DESCENDANT_USAGE_RESUME_KEY, () => seq > 0 ? { seq, usage: { ...latest } } : undefined);
  channel.onReconnect(() => {
    try { if (seq > 0) channel.send({ t: DESCENDANT_USAGE_MESSAGE, seq, usage: { ...latest } }); }
    catch { /* the next reconnect hello carries it */ }
  });
  return {
    setSource(next) { source = next; },
    evaluate() {
      if (!source || channel.closed) return;
      let value: CumulativeCost | undefined;
      try { value = source(); } catch { return; /* cosmetic accounting never fails a lifecycle event */ }
      if (!value || isDeepStrictEqual(value, latest)) return;
      seq += 1;
      latest = { ...value };
      try { if (channel.connected) channel.send({ t: DESCENDANT_USAGE_MESSAGE, seq, usage: { ...latest } }); }
      catch { /* carried by the reconnect hello */ }
    },
  };
}

/**
 * The hop's reporter, installed by `index.ts` when this process has a parent
 * channel. `evaluateDescendantUsage` is called at the two evaluation points,
 * after this hop's own reduction of a direct child (`refreshAgentTelemetry`)
 * and after a direct child's report changed its stored value, and once at
 * session start to report the value rebuilt from revived records.
 */
export const descendantUsageReporterRef: { current: DescendantUsageReporter | undefined } = { current: undefined };
export function evaluateDescendantUsage(): void {
  try { descendantUsageReporterRef.current?.evaluate(); } catch { /* cosmetic accounting never fails a lifecycle event */ }
}

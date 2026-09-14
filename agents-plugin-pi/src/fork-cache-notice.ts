import type { ExtensionContext, RpcClient } from "@earendil-works/pi-coding-agent";
import type { RpcAgentRecord } from "./spawner.ts";

export type ForkCacheNoticeOwner = Pick<ExtensionContext, "mode" | "hasUI" | "ui">;

/** Cosmetic only: never treat missing/error usage as a cache miss. */
export function formatFirstForkCacheNotice(label: string, message: unknown): string {
  const m = message as { stopReason?: string; usage?: { input?: unknown; cacheRead?: unknown } } | null;
  const input = m?.usage?.input;
  const cached = m?.usage?.cacheRead;
  const prefix = `fork ${label.replace(/[\u0000-\u001f\u007f-\u009f]/g, " ").slice(0, 80)} · first response cache`;
  if (!m || !["stop", "toolUse", "length"].includes(m.stopReason ?? "") ||
      typeof input !== "number" || !Number.isFinite(input) || input < 0 ||
      typeof cached !== "number" || !Number.isFinite(cached) || cached < 0) return `${prefix} unknown`;
  const total = input + cached;
  if (!Number.isFinite(total) || total <= 0) return `${prefix} unknown`;
  const percent = cached / total * 100;
  // Avoid displaying a tiny positive hit as zero, or a partial hit as full.
  const ratio = cached > 0 && percent < 0.05 ? "<0.1" : input > 0 && percent >= 99.95 ? ">99.9" : percent.toFixed(1);
  return `${prefix} ${ratio}% (${cached.toLocaleString("en-US")} / ${total.toLocaleString("en-US")} tokens)`;
}

/** Called only by initial spawn, before prompt. RPC events contain new messages,
 * not inherited history. No state or callback is stored on the durable record.
 */
export function attachFirstTaskForkCacheNotice(
  record: Pick<RpcAgentRecord, "spawnRole" | "forkContext" | "alias" | "title" | "agentId">,
  client: Pick<RpcClient, "onEvent">,
  owner: ForkCacheNoticeOwner | undefined,
): void {
  if (record.spawnRole !== "fork" || record.forkContext?.kind !== "task" ||
      owner?.mode !== "tui" || !owner.hasUI || typeof owner.ui?.notify !== "function") return;
  let seen = false;
  client.onEvent((event) => {
    if (seen || event.type !== "message_end" || event.message?.role !== "assistant") return;
    seen = true;
    try {
      owner.ui.notify(formatFirstForkCacheNotice(record.alias || record.title || record.agentId.slice(0, 8), event.message), "info");
    } catch { /* A cosmetic UI failure must not interrupt reports or responses. */ }
  });
}

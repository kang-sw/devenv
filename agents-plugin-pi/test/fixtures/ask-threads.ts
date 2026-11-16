import {
  type ThreadRecord,
} from "../../src/ask.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../../src/push-protocol.ts";

/** Keep item-level lifecycle assertions while the transport delivers a batch envelope. */
export function capturePush(sent: Array<{ message: unknown; options: unknown }>, message: unknown, options: unknown): void {
  const batch = message as { customType?: string; details?: { items?: unknown[] } };
  if (batch.customType === PUSH_BATCH_CUSTOM_TYPE && Array.isArray(batch.details?.items)) {
    for (const item of batch.details.items) sent.push({ message: item, options });
  } else {
    sent.push({ message, options });
  }
}

export function thread(overrides: Partial<ThreadRecord> = {}): ThreadRecord {
  return {
    threadId: "q1",
    title: "a question",
    status: "pending",
    origin: "fork-raised",
    createdAt: "2026-09-05T10:00:00.000Z",
    touchedAt: "2026-09-05T10:00:00.000Z",
    ...overrides,
  };
}

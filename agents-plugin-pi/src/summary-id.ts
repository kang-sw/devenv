/**
 * Display-summary row ids for adapter custom messages.
 *
 * The lead TUI's display summarizer (display-summary.ts) keys each summary by
 * a row id. A tool row already has its tool call id; a custom message has
 * none (`agent_id` repeats across one agent's messages), so every summarized
 * send site stamps a fresh UUID into the message's `details` under
 * `SUMMARY_ID_KEY`. `details` never reaches the model (only `content` does),
 * so the stamp changes nothing the lead reads.
 *
 * Kept free of host imports: spawner.ts, goal-loop.ts and the renderers all
 * import it.
 */

import { randomUUID } from "node:crypto";

/** The `details` field carrying a custom message's display-summary row id. */
export const SUMMARY_ID_KEY = "ws_summary_id";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Copy of `details` with a fresh row id; a non-object `details` is replaced by the id alone. */
export function withSummaryId(details: unknown): Record<string, unknown> {
  return { ...(isRecord(details) ? details : {}), [SUMMARY_ID_KEY]: randomUUID() };
}

/** The row id stamped into `details`, if any. */
export function summaryIdOf(details: unknown): string | undefined {
  if (!isRecord(details)) return undefined;
  const id = details[SUMMARY_ID_KEY];
  return typeof id === "string" && id.length > 0 ? id : undefined;
}

/** Stamps every batch item with its own row id, so each item card is summarized on its own. */
export function withItemSummaryIds<T extends { details?: unknown }>(items: readonly T[]): T[] {
  return items.map((item) => ({ ...item, details: withSummaryId(item.details) }));
}

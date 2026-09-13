/** Internal wire contract shared by held-push production and TUI consumption. */
export const PUSH_BATCH_CUSTOM_TYPE = "ws-push-batch";
export const PUSH_BATCH_VERSION = "1";

export type PushBatchItemState = "informational" | "actionable" | "superseded";

/** One original custom message plus batch-only actionability metadata. */
export interface PushBatchItem {
  customType: string;
  content: unknown;
  display: boolean;
  details?: unknown;
  state: PushBatchItemState;
}

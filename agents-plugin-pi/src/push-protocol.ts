/** Internal wire contract shared by held-push production and TUI consumption. */
export const PUSH_BATCH_CUSTOM_TYPE = "ws-push-batch";
export const PUSH_BATCH_VERSION = "1";
/** A batch item recording a resolved `/goal` replacement: kept in `details.items` but never in the model content. */
export const GOAL_CONTROL_CUSTOM_TYPE = "ws-goal-control";

export type PushBatchItemState = "informational" | "actionable" | "superseded";

/** One original custom message plus batch-only actionability metadata. */
export interface PushBatchItem {
  customType: string;
  content: unknown;
  display: boolean;
  details?: unknown;
  state: PushBatchItemState;
}

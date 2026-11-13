/** Wake causes observable in Pi custom messages, never inferred from a person's prose. */
import { ADAPTER_MESSAGE_LABEL } from "./adapter-label.ts";
import { GOAL_CONTROL_CUSTOM_TYPE, PUSH_BATCH_CUSTOM_TYPE } from "./push-protocol.ts";

export type DashboardWakeCause = "subagent_report" | "subagent_done" | "subagent_failed" | "background_exit" | "monitor" | "mail" | "message" | "scheduled";
export interface DashboardWake {
  kind: "wake";
  cause: DashboardWakeCause;
  source: string;
  caption: string;
  text: string;
}

function textContent(content: unknown): string {
  if (typeof content === "string") return content;
  return Array.isArray(content) ? content.flatMap((part) => part?.type === "text" && typeof part.text === "string" ? [part.text] : []).join("\n") : "";
}

/** Unwrap the shared push FIFO's batch, retaining each original sender and payload. */
export function dashboardWakeMessages(message: unknown): DashboardWake[] {
  const m = message as { role?: unknown; customType?: unknown; content?: unknown; details?: unknown } | undefined;
  if (m?.role !== "custom" || typeof m.customType !== "string") return [];
  const details = (m.details ?? {}) as Record<string, unknown>;
  if (m.customType === PUSH_BATCH_CUSTOM_TYPE) {
    return Array.isArray(details.items) ? details.items.flatMap((item) => dashboardWakeMessages({ ...item, role: "custom" })) : [];
  }
  // Held dashboard submissions are still the person's input; controls and
  // compaction bookkeeping are context, not notifications.
  if (["ws-whiteboard", GOAL_CONTROL_CUSTOM_TYPE, "ws-lead-compact", "ws-lead-context-milestone"].includes(m.customType)) return [];
  let text = textContent(m.content);
  if (text.startsWith(`${ADAPTER_MESSAGE_LABEL}\n`)) text = text.slice(ADAPTER_MESSAGE_LABEL.length + 1);
  if (!text.trim()) return [];
  let cause: DashboardWakeCause = "message";
  if (m.customType === "ws-agent-report") cause = "subagent_report";
  else if (m.customType === "ws-agent-settled") cause = details.failed === true || details.reason === "spawn-failed" || (details.reason === "exited" && details.failed !== false) || details.error ? "subagent_failed" : "subagent_done";
  else if (m.customType === "ws-background-task") cause = details.task_kind === "monitor" ? "monitor" : "background_exit";
  else if (m.customType === "ws-monitor") cause = "monitor";
  else if (m.customType === "ws-mailbox") cause = "mail";
  else if (m.customType === "ws-scheduled") cause = "scheduled";
  const source = [details.agent_id, details.task_id, details.from, details.reply_to].find((value) => typeof value === "string" && value.trim()) as string | undefined;
  const summary = [details.report, details.why, details.content, details.last_message, details.error].find((value) => typeof value === "string" && value.trim()) as string | undefined;
  return [{ kind: "wake", cause, source: source ?? m.customType, caption: (summary ?? text).replace(/\s+/g, " ").trim().slice(0, 240), text }];
}

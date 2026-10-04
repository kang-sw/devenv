import type { CustomEntry, ExtensionAPI, SessionEntry } from "@earendil-works/pi-coding-agent";
import { humanTextOf } from "./lead-compaction.ts";
import { isChildProcess } from "./goal-loop.ts";
import { loadHostPiTui, Text, stripTerminalSequences } from "./pi-tui.ts";

export const COMPACTION_HISTORY_TYPE = "ws-lead-compaction-history";

interface HistoryMessage {
  entryId: string;
  role: "user" | "assistant";
  text: string;
}

interface CompactionHistory {
  version: 1;
  compactionId: string;
  messages: HistoryMessage[];
}

/** Original text blocks only: a mixed assistant message still counts as one message. */
export function selectCompactionHistory(entries: readonly SessionEntry[]): HistoryMessage[] {
  const messages: HistoryMessage[] = [];
  for (const entry of entries) {
    if (entry.type !== "message") continue;
    const { role, content } = entry.message;
    if (role !== "user" && role !== "assistant") continue;
    const text = typeof content === "string" ? content : content
      .filter((part) => part.type === "text")
      .map((part) => part.text)
      .join("\n");
    if (!text.trim()) continue;
    if (role === "user") {
      if (humanTextOf(text) === undefined) continue;
      // These are the other user-role injections owned by goal-loop.ts. The
      // body remains original (including skill expansions), not dialog prose.
      if (text.startsWith("Goal armed: ")) continue;
      if (/^Compaction complete\. Invoke `lead-revive` \(`ws-skill lead-revive`\) with session key /.test(text)) continue;
    }
    messages.push({ entryId: entry.id, role, text });
  }
  return messages.slice(-20);
}

/** Supported plain entries persist for humans but produce no model-context messages. */
export function registerCompactionHistory(pi: ExtensionAPI): void {
  let primitives = { Text, stripTerminalSequences };
  pi.on("session_start", async () => {
    primitives = await loadHostPiTui();
  });
  pi.registerEntryRenderer<CompactionHistory>(COMPACTION_HISTORY_TYPE, (entry, _options, theme) => {
    const { Text, stripTerminalSequences } = primitives;
    const data = entry.data;
    if (!data || data.version !== 1 || !Array.isArray(data.messages)) return undefined;
    // Render bodies literally, without Markdown reinterpretation or elision.
    // Terminal controls are stripped only at display time; storage stays raw.
    const body = data.messages.map((message) =>
      `${theme.fg("accent", message.role === "user" ? "User" : "Assistant")}\n${stripTerminalSequences(message.text)}`,
    ).join("\n\n");
    return new Text(`${theme.fg("muted", "Previous conversation · display-only")}\n${theme.fg("dim", "────────────────────")}\n${body}\n${theme.fg("dim", "──────────────────── End previous conversation")}`, 0, 1);
  });
  pi.on("session_compact", (event, ctx) => {
    if (isChildProcess(process.env)) return;
    const reported = event.compactionEntry;
    const branch = ctx.sessionManager.getBranch();
    // Manual compaction in Pi 0.84.4 reports the first stored entry with a
    // matching summary, not necessarily the entry just appended. Resolve the
    // latest successful boundary from the supported active branch instead.
    // Require the reported entry on that branch to reject abandoned events.
    if (!branch.some((entry) => entry.type === "compaction" && entry.id === reported.id)) return;
    const boundary = branch.findLastIndex((entry) => entry.type === "compaction");
    const latest = branch[boundary];
    if (latest?.type !== "compaction" || latest.summary !== reported.summary) return;
    const compactionId = latest.id;
    if (branch.some((entry) => entry.type === "custom" && entry.customType === COMPACTION_HISTORY_TYPE &&
      (entry as CustomEntry<CompactionHistory>).data?.compactionId === compactionId)) return;
    const messages = selectCompactionHistory(branch.slice(0, boundary));
    if (!messages.length) return;
    // session_compact fires after success, before Pi rebuilds the live UI. The
    // host places this before the summary live and after it on reload; neither
    // ordering changes model context. No sendMessage or continuation is needed.
    pi.appendEntry<CompactionHistory>(COMPACTION_HISTORY_TYPE, { version: 1, compactionId, messages });
  });
}

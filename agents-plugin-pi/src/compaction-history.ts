import type { CustomEntry, ExtensionAPI, SessionEntry } from "@earendil-works/pi-coding-agent";
import { getMarkdownTheme } from "@earendil-works/pi-coding-agent";
import { adapterLabeledBody } from "./adapter-label.ts";
import { GOAL_ANNOUNCEMENT_PREFIX, humanTextOf } from "./lead-compaction.ts";
import { isChildProcess } from "./goal-loop.ts";
import { loadHostPiTui, Container, Markdown, Text, stripTerminalSequences } from "./pi-tui.ts";

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
      // Labeled adapter traffic stays out even where the dialog keeps it
      // (the goal announcement collapses to `/goal <goal>` there). The body
      // remains original (including skill expansions), not dialog prose.
      if (adapterLabeledBody(text) !== undefined) continue;
      // Unlabeled goal-loop injections recorded before the label existed.
      if (text.startsWith(GOAL_ANNOUNCEMENT_PREFIX)) continue;
      if (/^Compaction complete\. Invoke `lead-revive` \(`ws-skill lead-revive`\) with session key /.test(text)) continue;
    }
    messages.push({ entryId: entry.id, role, text });
  }
  return messages.slice(-20);
}

/** Supported plain entries persist for humans but produce no model-context messages. */
export function registerCompactionHistory(pi: ExtensionAPI): void {
  let primitives = { Container, Markdown, Text, stripTerminalSequences };
  pi.on("session_start", async () => {
    primitives = await loadHostPiTui();
  });
  pi.registerEntryRenderer<CompactionHistory>(COMPACTION_HISTORY_TYPE, (entry, _options, theme) => {
    const { Container, Markdown, Text, stripTerminalSequences } = primitives;
    const data = entry.data;
    if (!data || data.version !== 1 || !Array.isArray(data.messages)) return undefined;
    const container = new Container();
    container.addChild(new Text(`${theme.fg("muted", "Previous conversation · display-only")}\n${theme.fg("dim", "────────────────────")}`, 0, 1));
    const markdownTheme = getMarkdownTheme();
    for (const [index, message] of data.messages.entries()) {
      const isUser = message.role === "user";
      if (!isUser) container.addChild(new Text(theme.fg("accent", "Assistant"), 0, 0));
      // Match native user Markdown padding/styles, but never its OSC navigation
      // markers. Strip terminal controls only at display time; storage stays raw.
      container.addChild(new Markdown(stripTerminalSequences(message.text), 1, 1, markdownTheme,
        isUser ? {
          color: (text) => theme.fg("userMessageText", text),
          bgColor: (text) => theme.bg("userMessageBg", text),
        } : undefined,
        isUser ? { preserveOrderedListMarkers: true, preserveBackslashEscapes: true } : undefined));
      // Header/footer and assistant Markdown already supply outside padding.
      // Between messages, one plain row after a user also pads the next user.
      if (isUser && index < data.messages.length - 1) {
        container.addChild({ render: () => [""], invalidate() {} });
      }
    }
    container.addChild(new Text(theme.fg("dim", "──────────────────── End previous conversation"), 0, 1));
    return container;
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

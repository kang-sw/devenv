/**
 * The source label every adapter-injected, model-visible message opens with
 * (261007). Pi hands a custom message to the model as plain user-role text
 * (`convertToLlm`), and `sendUserMessage` is user text by construction, so
 * without this first line the model reads an adapter notice as the human's
 * own words (a downstream summary quoted the 70% milestone as the user's
 * latest request).
 *
 * The label goes on once, where the message is sent, not inside each content
 * builder: a push batch carries it on the batch, never on its inner messages.
 * It is also how lead compaction's dialog filter (lead-compaction.ts) tells
 * adapter traffic from human text. A leaf module, so spawner.ts and
 * lead-compaction.ts can both import it without a cycle.
 */
export const ADAPTER_MESSAGE_LABEL = "[system message from ws-pi-plugin]";

/** `text` with the label as its own first line. */
export function labelAdapterText(text: string): string {
  return `${ADAPTER_MESSAGE_LABEL}\n${text}`;
}

/**
 * Custom-message `content` with the label as its first line: prefixed to a
 * string, or to a leading text part, otherwise added as a leading text part.
 */
export function labelAdapterContent<C>(content: string | readonly C[]): string | C[] {
  if (typeof content === "string") return labelAdapterText(content);
  const [first, ...rest] = content;
  const text = (first as { type?: unknown; text?: unknown } | undefined);
  if (text?.type === "text" && typeof text.text === "string") {
    return [{ ...(first as object), text: labelAdapterText(text.text) } as C, ...rest];
  }
  return [{ type: "text", text: ADAPTER_MESSAGE_LABEL } as C, ...content];
}

/** The text after the label line, or `undefined` when `text` does not open with the label. */
export function adapterLabeledBody(text: string): string | undefined {
  if (text === ADAPTER_MESSAGE_LABEL) return "";
  return text.startsWith(`${ADAPTER_MESSAGE_LABEL}\n`) ? text.slice(ADAPTER_MESSAGE_LABEL.length + 1) : undefined;
}

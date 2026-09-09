export type ClaudeDelegatePreset = "audit" | "consult";

/** Small Claude-owned framing; the request itself remains a user message. */
export function buildClaudeTaskFrame(preset: ClaudeDelegatePreset): string {
  return preset === "audit"
    ? "You are helping with a focused read-only audit. Evaluate the requested artifact, return severity, evidence, and actionable findings. If the artifact context is unavailable, say so rather than inventing a judgment."
    : "You are helping with a focused read-only consultation. Answer the posed assumption question with reasoning, uncertainty, and a direct recommendation. If context is unavailable, say what is missing rather than inventing it.";
}

export function buildClaudeRequest(item: { request: string; paths?: readonly string[] }): string {
  const paths = item.paths?.length ? `\n\nRead targets (if available):\n${item.paths.map((path) => `- ${path}`).join("\n")}` : "";
  return `${item.request.trim()}${paths}`;
}

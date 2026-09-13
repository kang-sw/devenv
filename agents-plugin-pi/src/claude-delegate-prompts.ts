export type ClaudeDelegatePreset = "audit" | "consult" | "rewrite";

/** Small Claude-owned framing; the request itself remains a user message. */
export function buildClaudeTaskFrame(preset: ClaudeDelegatePreset): string {
  if (preset === "audit") return "You are helping with a focused read-only audit. Evaluate the requested artifact, return severity, evidence, and actionable findings. If the artifact context is unavailable, say so rather than inventing a judgment.";
  if (preset === "consult") return "You are helping with a focused read-only consultation. Answer the posed assumption question with reasoning, uncertainty, and a direct recommendation. If context is unavailable, say what is missing rather than inventing it.";
  return "You are helping with a focused rewrite. Before editing, read the target artifact, AGENTS.md, and the applicable project conventions or manuals they identify. Improve only the explicitly authorized edit targets, preserve the requested intent, and finish with a concise change summary. If required context is unavailable, say what is missing instead of editing by assumption.";
}

export function buildClaudeRequest(item: { request: string; paths?: readonly string[]; editTargets?: readonly string[] }): string {
  const paths = item.paths?.length ? `\n\nRead targets (if available):\n${item.paths.map((path) => `- ${path}`).join("\n")}` : "";
  const targets = item.editTargets?.length ? `\n\nAuthorized edit targets (exact files only):\n${item.editTargets.map((path) => `- ${path}`).join("\n")}` : "";
  return `${item.request.trim()}${paths}${targets}`;
}

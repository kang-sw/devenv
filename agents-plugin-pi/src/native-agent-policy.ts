/** Retired extension tools are absent, not merely inactive or deferred. */
export const RETIRED_NATIVE_AGENT_TOOLS: ReadonlySet<string> = new Set([
  "ws-agent-spawn", "ws-agent-send", "ws-agent-list", "ws-agent-stop",
  "ws-agent-transcript", "explore", "ws-fork",
]);

export function withoutRetiredNativeTools(tools: readonly string[]): string[] {
  return tools.filter(name => !RETIRED_NATIVE_AGENT_TOOLS.has(name));
}

/** Do not guess that an old generic/fork/research record was an execution worker. */
export function isExecutionAgent(record: {
  toolGroup?: string;
  spawnRole?: string;
  forkContext?: unknown;
  exploreMode?: unknown;
  explicitTools?: unknown;
}): boolean {
  return record.toolGroup === "execute-worker"
    && (record.spawnRole === undefined || record.spawnRole === "execute-worker")
    && record.forkContext === undefined && record.exploreMode === undefined
    && record.explicitTools === undefined;
}

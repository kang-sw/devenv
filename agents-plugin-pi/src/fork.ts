/** Legacy thread metadata helpers. Native ws-fork spawning/registration is retired. */
import { withoutRetiredNativeTools } from "./native-agent-policy.ts";
import type { SpawnRole } from "./process-role.ts";

/** Kept for old thread data readers; never grants a retired tool. */
export function computeForkToolSurface(leadActiveTools: readonly string[]): string[] {
  return withoutRetiredNativeTools(leadActiveTools);
}

/** Transitional source helper for bootstrap consumers; no role gains ws-fork. */
export function addForkToolIfLead(activeTools: readonly string[], _role: SpawnRole | undefined): string[] {
  return withoutRetiredNativeTools(activeTools);
}

export function getForkSourceSessionFile(toolCtx: unknown): string | undefined {
  const sessionManager = (toolCtx as { sessionManager?: { getSessionFile?: () => string | undefined } } | undefined)?.sessionManager;
  const file = sessionManager?.getSessionFile?.();
  return typeof file === "string" && file.length > 0 ? file : undefined;
}

/**
 * Session wiring for the lead TUI display summarizer (display-summary.ts).
 *
 * Active only in the interactive lead session — `ctx.mode === "tui"` and no
 * spawn role — which is known only at `session_start`. Workers, explores and
 * forks keep today's rendering: no summarizer runs there, and the built-in
 * tool wrappers and new message renderers are registered only behind this
 * gate (a child's scoped `edit`/`write` overrides from write-scopes.ts are
 * therefore never shadowed).
 *
 * Event handlers never await a summary request: Pi awaits extension handlers,
 * and a slow summary must never hold the lead's agent loop. The summarizer
 * runs on its own abort controller, so Esc does not cancel it.
 */

import { randomUUID } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { readSpawnRole, type SpawnRole } from "./process-role.ts";
import {
  createDisplaySummarizer,
  createModelResolver,
  createProviderCompletion,
  type DisplaySummarizer,
  type DisplaySummaryCompletion,
  type DisplaySummaryConfigReader,
  type DisplaySummaryRegistry,
  type DisplaySummaryStore,
} from "./display-summary.ts";

/** The display summarizer runs only in the interactive lead session. */
export function shouldRunDisplaySummary(mode: string | undefined, role: SpawnRole | undefined): boolean {
  return mode === "tui" && role === undefined;
}

export interface DisplaySummarySessionDeps {
  store: DisplaySummaryStore;
  readConfig: DisplaySummaryConfigReader;
  /** Same-name wrappers over the active Pi built-in tools; called once, lead TUI only. */
  registerBuiltinWrappers(cwd: string, activeToolNames: readonly string[]): void;
  /** Renderers for adapter messages that have none today; called once, lead TUI only. */
  registerMessageRenderers(): void;
  /** Test seam: the model call bound to the session's registry. */
  createCompletion?(registry: DisplaySummaryRegistry): DisplaySummaryCompletion;
  env?: NodeJS.ProcessEnv;
}

export interface DisplaySummarySession {
  /** Queues a custom row outside the lead conversation, except display-only compaction history. */
  enqueueStandalone(id: string, label: string, text: string): void;
  /** The live summarizer, `undefined` outside the lead TUI. */
  current(): DisplaySummarizer | undefined;
}

type SessionCtx = { mode?: string; cwd: string; modelRegistry: unknown };

export function registerDisplaySummarySession(
  pi: Pick<ExtensionAPI, "on" | "getActiveTools">,
  deps: DisplaySummarySessionDeps,
): DisplaySummarySession {
  let summarizer: DisplaySummarizer | undefined;
  let registered = false;

  const stop = (): void => {
    summarizer?.dispose();
    summarizer = undefined;
  };

  pi.on("session_start", (_event, ctx) => {
    stop();
    deps.store.clear();
    const session = ctx as unknown as SessionCtx;
    if (!shouldRunDisplaySummary(session.mode, readSpawnRole(deps.env ?? process.env))) return;
    if (!registered) {
      registered = true;
      deps.registerBuiltinWrappers(session.cwd, pi.getActiveTools());
      deps.registerMessageRenderers();
    }
    const registry = session.modelRegistry as DisplaySummaryRegistry;
    summarizer = createDisplaySummarizer({
      store: deps.store,
      readConfig: deps.readConfig,
      resolveModel: createModelResolver(registry),
      complete: (deps.createCompletion ?? createProviderCompletion)(registry),
      sessionId: randomUUID(),
    });
  });
  pi.on("message_end", (event) => { summarizer?.observeMessage(event.message); });
  pi.on("tool_execution_start", (event) => { summarizer?.observeToolStart(event.toolCallId, event.toolName, event.args); });
  pi.on("tool_execution_end", (event) => { summarizer?.observeToolEnd(event.toolCallId, event.toolName); });
  // Flush points: each lead turn, and idle entry for pushes that arrive while idle.
  pi.on("turn_end", () => { void summarizer?.flush(); });
  pi.on("agent_end", () => { void summarizer?.flush(); });
  pi.on("session_compact", () => { summarizer?.reset(); });
  pi.on("session_shutdown", () => { stop(); });

  return {
    enqueueStandalone(id, label, text) { summarizer?.enqueueStandalone(id, label, text); },
    current: () => summarizer,
  };
}

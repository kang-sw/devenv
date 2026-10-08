/**
 * Session wiring for the lead TUI display summarizer (display-summary.ts).
 *
 * Active only in the interactive lead session — `ctx.mode === "tui"` and no
 * spawn role — which is known only at `session_start`. Presentation is installed
 * earlier without replacing execution definitions. Workers, explores and forks
 * keep raw rendering; only the live lead gate enables summary reads/replay.
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
import {
  cleanOrphanSummarySidecars, createSummarySidecar, seedNativeSummarySidecar,
  type SidecarIO, type SummaryConversation, type SummarySidecar,
} from "./display-summary-sidecar.ts";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";

/** The display summarizer runs only in the interactive lead session. */
export function shouldRunDisplaySummary(mode: string | undefined, role: SpawnRole | undefined): boolean {
  return mode === "tui" && role === undefined;
}

export interface DisplaySummarySessionDeps {
  store: DisplaySummaryStore;
  readConfig: DisplaySummaryConfigReader;
  /** The manifest default style; forwarded to the summarizer. */
  readDefaultStyle?(): string | undefined;
  /** Confirm the live native loadout; does not register or replace execution tools. */
  confirmBuiltinTools(activeToolNames: readonly string[]): void;
  /** Test seam: the model call bound to the session's registry. */
  createCompletion?(registry: DisplaySummaryRegistry): DisplaySummaryCompletion;
  env?: NodeJS.ProcessEnv;
  /** Filesystem seam for deterministic persistence/lifecycle tests. */
  sidecarIO?: SidecarIO;
}

export interface DisplaySummarySession {
  /** Queues a custom row outside the lead conversation, except display-only compaction history. */
  enqueueStandalone(id: string, label: string, text: string): void;
  /** The live summarizer, `undefined` outside the lead TUI. */
  current(): DisplaySummarizer | undefined;
}

type SessionCtx = {
  mode?: string;
  cwd: string;
  modelRegistry: unknown;
  sessionManager?: {
    getSessionFile(): string | undefined;
    getSessionId(): string;
    getEntries(): SessionEntry[];
  };
};

export function registerDisplaySummarySession(
  pi: Pick<ExtensionAPI, "on" | "getActiveTools">,
  deps: DisplaySummarySessionDeps,
): DisplaySummarySession {
  let summarizer: DisplaySummarizer | undefined;
  deps.store.enabled = false;
  let unsubscribeOwnership: (() => void) | undefined;

  let sidecar: SummarySidecar | undefined;
  let conversation: SummaryConversation | undefined;
  let live: { active: boolean } | undefined;

  const stop = (): Promise<void> => {
    // Reject config/provider work before awaiting disk. Only already accepted
    // batches drain; unfinished model requests must never delay replacement.
    summarizer?.dispose();
    summarizer = undefined;
    if (live) live.active = false;
    unsubscribeOwnership?.();
    unsubscribeOwnership = undefined;
    deps.store.enabled = false;
    const outgoing = sidecar;
    sidecar = undefined;
    return outgoing?.drain() ?? Promise.resolve();
  };

  pi.on("session_start", (_event, ctx) => {
    const draining = stop();
    deps.store.clear();
    const session = ctx as unknown as SessionCtx;
    if (!shouldRunDisplaySummary(session.mode, readSpawnRole(deps.env ?? process.env))) return;
    deps.store.enabled = true;
    deps.confirmBuiltinTools(pi.getActiveTools());
    const registry = session.modelRegistry as DisplaySummaryRegistry;
    const token = live = { active: true };
    const manager = session.sessionManager;
    const sessionId = manager?.getSessionId() ?? randomUUID();
    let sessionFile = manager?.getSessionFile();
    let entries = manager?.getEntries() ?? [];
    const refresh = (): void => {
      // A first save may assign a previously absent path. Bind it once, only
      // while the originating manager still has the captured Pi header ID.
      if (manager?.getSessionId() !== sessionId) return;
      sessionFile ??= manager?.getSessionFile();
      // Once bound, equal IDs at another path still cannot redirect old work.
      if (manager?.getSessionFile() === sessionFile) entries = manager?.getEntries() ?? entries;
    };
    conversation = {
      sessionId,
      get sessionFile() { refresh(); return sessionFile; },
      entries() { refresh(); return entries; },
    };
    const cache = sidecar = createSummarySidecar({
      conversation, toolNames: deps.store.toolNames, io: deps.sidecarIO,
      startAfter: draining.then(async () => {
        if (sessionFile) await cleanOrphanSummarySidecars(sessionFile, deps.sidecarIO);
      }),
      restore(batch) {
        if (!token.active || !batch.size) return;
        for (const [id, summary] of batch) deps.store.set(id, summary);
        deps.store.notify([...batch.keys()]);
      },
    });
    unsubscribeOwnership = deps.store.onToolRegistered(() => { cache.sync(); });
    summarizer = createDisplaySummarizer({
      store: deps.store,
      readConfig: deps.readConfig,
      readDefaultStyle: deps.readDefaultStyle,
      resolveModel: createModelResolver(registry),
      complete: (deps.createCompletion ?? createProviderCompletion)(registry),
      // Provider-cache UUID is intentionally unrelated to durable Pi ownership.
      sessionId: randomUUID(),
      onAccepted: (batch) => { cache.accept(batch); },
    });
    return cache.ready;
  });
  pi.on("message_end", (event) => { summarizer?.observeMessage(event.message); sidecar?.sync(); });
  pi.on("tool_execution_start", (event) => { summarizer?.observeToolStart(event.toolCallId, event.toolName, event.args); });
  pi.on("tool_execution_end", (event) => { summarizer?.observeToolEnd(event.toolCallId, event.toolName); });
  // Flush points: each lead turn, and idle entry for pushes that arrive while idle.
  pi.on("turn_end", () => { sidecar?.sync(); void summarizer?.flush(); });
  pi.on("agent_end", () => { sidecar?.sync(); void summarizer?.flush(); });
  pi.on("session_compact", () => { summarizer?.reset(); sidecar?.sync(); });
  pi.on("session_shutdown", (event) => {
    const outgoing = sidecar;
    const sourceFile = conversation?.sessionFile;
    // Pi creates the native fork/copy before shutdown, and recreates the
    // extension afterward. Seed here so accepted-but-unwritten values survive
    // even an outgoing append failure, without a process-global handoff cache.
    const transition = event as { reason?: string; targetSessionFile?: string };
    const draining = stop();
    // Pi shuts down the old runtime before rebuilding incoming components.
    // session_start resets values only: those components already own new links.
    deps.store.retire();
    return draining.then(async () => {
      if (transition.reason === "fork" && transition.targetSessionFile && sourceFile && outgoing) {
        await seedNativeSummarySidecar(transition.targetSessionFile, { sessionFile: sourceFile, summaries: outgoing.snapshot() }, deps.store.toolNames, deps.sidecarIO);
      }
    });
  });

  return {
    enqueueStandalone(id, label, text) { summarizer?.enqueueStandalone(id, label, text); },
    current: () => summarizer,
  };
}

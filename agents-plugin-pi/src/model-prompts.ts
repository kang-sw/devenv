import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";
import { registerModelPromptCommand } from "./model-prompt-command.ts";
import { createModelPromptStore } from "./model-prompt-store.ts";
import type { ModelPromptConfig, ModelPromptStore } from "./model-prompt-types.ts";

type ModelIdentity = { provider: string; id: string };

export function resolveModelPromptText(config: ModelPromptConfig, model: ModelIdentity | undefined, role: SpawnRole | undefined): string {
  if (!model) return "";
  const exact = config.rules.find(rule => rule.id === model.id && rule.provider === model.provider);
  const fallback = config.rules.find(rule => rule.id === model.id && rule.provider === undefined);
  return [exact?.general ?? fallback?.general, isLeadOrFork(role) ? exact?.leadOnly ?? fallback?.leadOnly : undefined]
    .filter(text => text !== undefined).join("\n\n");
}

const BLOCK_END = "\n</ws-model-prompts>";

/** Length framing (JS string units) permits arbitrary user text, even the closing marker itself. */
export function replaceModelPromptBlock(prompt: string, text: string): string {
  const marker = /\n\n<ws-model-prompts length="(\d+)">\n/g;
  let clean = "", cursor = 0;
  for (const match of prompt.matchAll(marker)) {
    if (match.index! < cursor) continue;
    const end = match.index! + match[0].length + Number(match[1]);
    if (!Number.isSafeInteger(end) || prompt.slice(end, end + BLOCK_END.length) !== BLOCK_END) continue;
    clean += prompt.slice(cursor, match.index);
    cursor = end + BLOCK_END.length;
  }
  clean += prompt.slice(cursor);
  return text ? `${clean}\n\n<ws-model-prompts length="${text.length}">\n${text}${BLOCK_END}` : clean;
}

function sameModel(a: ModelIdentity | undefined, b: ModelIdentity | undefined): boolean {
  return a?.provider === b?.provider && a?.id === b?.id;
}

export function createModelPromptRuntime() {
  let config: ModelPromptConfig = { version: 1, rules: [] };
  let startupModel: ModelIdentity | undefined;
  let inheritedReleased = false;
  let ran = false;
  return {
    start(snapshot: ModelPromptConfig, model: ModelIdentity | undefined, released = false) {
      config = snapshot;
      startupModel = model;
      inheritedReleased = released;
      ran = false;
    },
    isInheritedReleased: () => inheritedReleased,
    written(snapshot: ModelPromptConfig) {
      config = snapshot;
      inheritedReleased = true;
    },
    modelSelected(model: ModelIdentity, source: "set" | "cycle" | "restore") {
      // Startup/restore selection is not a request to replace the parent's cache prefix.
      if (ran && source !== "restore" && !sameModel(model, startupModel)) inheritedReleased = true;
    },
    apply(prompt: string, model: ModelIdentity | undefined, role: SpawnRole | undefined, inherited: boolean): string {
      if (!ran) startupModel = model;
      ran = true;
      if (role === "fork" && inherited && !inheritedReleased) return prompt;
      return replaceModelPromptBlock(prompt, resolveModelPromptText(config, model, role));
    },
  };
}

/** Register AFTER lead-bootstrap: that hook restores the inherited whole fork prompt before we compose. */
export function registerModelPrompts(
  pi: ExtensionAPI,
  inheritedForkPrompt: { current: string | undefined },
  store: ModelPromptStore = createModelPromptStore(),
): void {
  const runtime = createModelPromptRuntime();
  const stateType = "ws-pi-model-prompt-state";
  let sessionId: string | undefined;
  let releaseRecorded = false;
  const recordRelease = () => {
    if (readSpawnRole(process.env) !== "fork" || !sessionId || releaseRecorded || !runtime.isInheritedReleased()) return;
    // Non-conversation metadata survives /reload and dormant relaunch. Ownership
    // excludes inherited parent entries when a new fork starts on copied history.
    pi.appendEntry(stateType, { sessionId, released: true });
    releaseRecorded = true;
  };
  pi.on("session_start", async (_event, ctx) => {
    sessionId = ctx.sessionManager.getSessionId();
    releaseRecorded = ctx.sessionManager.getEntries().some(entry => {
      if (entry.type !== "custom" || entry.customType !== stateType) return false;
      const state = entry.data as { sessionId?: string; released?: boolean } | undefined;
      return state?.sessionId === sessionId && state?.released === true;
    });
    try { runtime.start(await store.load(), ctx.model, releaseRecorded); }
    catch (error) {
      runtime.start({ version: 1, rules: [] }, ctx.model, releaseRecorded);
      ctx.ui.notify(`Model prompt configuration not loaded: ${String(error)}`, "error");
    }
  });
  registerModelPromptCommand(pi, store, config => { runtime.written(config); recordRelease(); });
  pi.on("model_select", event => { runtime.modelSelected(event.model, event.source); recordRelease(); });
  pi.on("before_agent_start", (event, ctx) => {
    const systemPrompt = runtime.apply(event.systemPrompt, ctx.model, readSpawnRole(process.env), inheritedForkPrompt.current !== undefined);
    return systemPrompt === event.systemPrompt ? undefined : { systemPrompt };
  });
}

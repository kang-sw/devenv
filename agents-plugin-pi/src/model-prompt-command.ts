import type { ExtensionAPI, ExtensionCommandContext } from "@earendil-works/pi-coding-agent";
import type { ModelPromptChannel, ModelPromptConfig, ModelPromptSelector, ModelPromptStore } from "./model-prompt-types.ts";

type SelectorCommand = { verb: "set" | "show" | "clear"; selector: ModelPromptSelector; channel: ModelPromptChannel };
type ModelPromptCommand = SelectorCommand | { verb: "list" };
const USAGE = "Usage: /ws-model-prompt (list | (set|show|clear) [--lead-only] [--provider <provider>] <id>)";

/** Parse only whitespace-delimited command tokens; a slash inside the ID is never a provider separator. */
export function parseModelPromptCommand(args: string): ModelPromptCommand {
  const tokens = args.trim().split(/\s+/).filter(Boolean);
  const verb = tokens.shift();
  if (verb === "list" && tokens.length === 0) return { verb };
  if (verb !== "set" && verb !== "show" && verb !== "clear") throw new Error(USAGE);
  let channel: ModelPromptChannel = "general";
  let provider: string | undefined;
  while (tokens[0]?.startsWith("--")) {
    const flag = tokens.shift();
    if (flag === "--lead-only" && channel === "general") channel = "leadOnly";
    else if (flag === "--provider" && provider === undefined && tokens[0] && !tokens[0].startsWith("--")) provider = tokens.shift();
    else throw new Error(USAGE);
  }
  if (tokens.length !== 1 || tokens[0]?.startsWith("--")) throw new Error(USAGE);
  return { verb, selector: { id: tokens[0]!, ...(provider === undefined ? {} : { provider }) }, channel };
}

function label(selector: ModelPromptSelector): string {
  return `${selector.provider === undefined ? "" : `--provider ${selector.provider} `}${selector.id}`;
}

function exact(config: ModelPromptConfig, selector: ModelPromptSelector): (typeof config.rules)[number] | undefined {
  return config.rules.find(rule => rule.id === selector.id && rule.provider === selector.provider);
}

function catalogContains(selector: ModelPromptSelector, ctx: ExtensionCommandContext): boolean {
  const models = ctx.modelRegistry.getAll();
  return models.some(model => model.id === selector.id && (selector.provider === undefined || model.provider === selector.provider));
}

/** Writes are persisted first; onWrite installs the returned snapshot in the active process. */
export function registerModelPromptCommand(pi: ExtensionAPI, store: ModelPromptStore, onWrite: (config: ModelPromptConfig) => void): void {
  pi.registerCommand("ws-model-prompt", {
    description: "Show, list, set or clear global Pi model-specific system-prompt supplements",
    handler: async (args, ctx) => {
      try {
        const command = parseModelPromptCommand(args);
        if (command.verb === "list") {
          const rules = (await store.load()).rules;
          ctx.ui.notify(rules.length
            ? rules.map(rule => `${label(rule)}: ${[rule.general === undefined ? "" : "general", rule.leadOnly === undefined ? "" : "lead-only"].filter(Boolean).join(", ")}`).join("\n")
            : "No model prompts configured.", "info");
          return;
        }
        const { selector, channel } = command;
        const channelLabel = channel === "leadOnly" ? "lead-only" : "general";
        if (command.verb === "show") {
          const text = exact(await store.load(), selector)?.[channel];
          ctx.ui.notify(text === undefined ? `${label(selector)} (${channelLabel}): not set` : `${label(selector)} (${channelLabel}):\n${text}`, "info");
          return;
        }
        let text: string | undefined;
        if (command.verb === "set") {
          if (!ctx.hasUI) throw new Error("/ws-model-prompt set requires interactive UI (TUI or RPC)");
          text = await ctx.ui.editor(`Model prompt (${channelLabel}): ${label(selector)}`, exact(await store.load(), selector)?.[channel] ?? "");
          if (text === undefined) return; // Cancellation does not write.
          if (!text.trim()) throw new Error("Empty model prompt: use clear to remove a channel");
        }
        const saved = await store.update(selector, channel, text);
        onWrite(saved);
        ctx.ui.notify(`${label(selector)} (${channelLabel}) ${command.verb === "set" ? "saved" : "cleared"}.`, "info");
        if (command.verb === "set") {
          try {
            if (!catalogContains(selector, ctx)) ctx.ui.notify(`${label(selector)} is not in the current Pi model catalog; rule saved for future availability.`, "warning");
          } catch {
            ctx.ui.notify("Model prompt saved, but the current Pi model catalog could not be checked.", "warning");
          }
        }
      } catch (error) {
        ctx.ui.notify(`Model prompt command failed: ${(error as Error).message}`, "error");
      }
    },
  });
}

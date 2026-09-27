import { getAgentDir } from "@earendil-works/pi-coding-agent";
import { randomUUID } from "node:crypto";
import { mkdir, open, readFile, rmdir, unlink } from "node:fs/promises";
import { renameWithWindowsRetry } from "./atomic-write.ts";
import { dirname, join } from "node:path";
import type { ModelPromptChannel, ModelPromptConfig, ModelPromptRule, ModelPromptSelector, ModelPromptStore } from "./model-prompt-types.ts";

const LOCK_WAIT_MS = 3000;
const LOCK_RETRY_MS = 25;

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function keysOnly(value: Record<string, unknown>, allowed: string[]): boolean {
  return Object.keys(value).every(key => allowed.includes(key));
}

function validSelectorPart(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.trim() === value && !/\s/.test(value);
}

function selectorKey(selector: ModelPromptSelector): string {
  return JSON.stringify([selector.provider ?? null, selector.id]);
}

function validate(value: unknown): ModelPromptConfig {
  if (!object(value) || value.version !== 1 || !Array.isArray(value.rules) || !keysOnly(value, ["version", "rules"])) {
    throw new Error("Invalid or unsupported model prompt configuration (expected version 1 rules array)");
  }
  const seen = new Set<string>();
  for (const [index, entry] of value.rules.entries()) {
    if (!object(entry) || !keysOnly(entry, ["id", "provider", "general", "leadOnly"]) ||
      !validSelectorPart(entry.id) || ("provider" in entry && !validSelectorPart(entry.provider)) ||
      ("general" in entry && (typeof entry.general !== "string" || !entry.general.trim())) ||
      ("leadOnly" in entry && (typeof entry.leadOnly !== "string" || !entry.leadOnly.trim())) ||
      (!("general" in entry) && !("leadOnly" in entry))) {
      throw new Error(`Invalid model prompt rule at index ${index}`);
    }
    const key = selectorKey(entry as unknown as ModelPromptSelector);
    if (seen.has(key)) throw new Error(`Duplicate model prompt selector at index ${index}`);
    seen.add(key);
  }
  return value as unknown as ModelPromptConfig;
}

async function load(path: string): Promise<ModelPromptConfig> {
  let raw: string;
  try { raw = await readFile(path, "utf8"); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return { version: 1, rules: [] };
    throw error;
  }
  try { return validate(JSON.parse(raw)); }
  catch (error) { throw new Error(`Cannot load model prompt configuration at ${path}: ${(error as Error).message}`, { cause: error }); }
}

/** A never-stolen directory claim: an abandoned claim is deliberately a busy error, not an unsafe stale-owner guess. */
async function withLock<T>(path: string, action: () => Promise<T>): Promise<T> {
  const lock = `${path}.lock`;
  const deadline = Date.now() + LOCK_WAIT_MS;
  for (;;) {
    try { await mkdir(lock); break; }
    catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error;
      if (Date.now() >= deadline) throw new Error(`Model prompt store busy: lock ${lock} could not be acquired; check for another writer or an abandoned lock`);
      await new Promise(resolve => setTimeout(resolve, LOCK_RETRY_MS));
    }
  }
  try { return await action(); }
  finally { await rmdir(lock); }
}

async function replace(path: string, value: ModelPromptConfig): Promise<void> {
  const temp = `${path}.${process.pid}.${randomUUID()}.tmp`;
  try {
    const file = await open(temp, "wx", 0o600);
    try {
      await file.writeFile(`${JSON.stringify(value, null, 2)}\n`, "utf8");
      await file.sync();
    } finally { await file.close(); }
    renameWithWindowsRetry(temp, path);
  } finally { await unlink(temp).catch(error => { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }); }
}

/** Global Pi-owned config, independent of the ws MCP tuning and of project checkouts. */
export function createModelPromptStore(path = join(getAgentDir(), "ws", "model-prompts.json")): ModelPromptStore {
  return {
    load: () => load(path),
    async update(selector: ModelPromptSelector, channel: ModelPromptChannel, text: string | undefined): Promise<ModelPromptConfig> {
      if (!validSelectorPart(selector.id) || (selector.provider !== undefined && !validSelectorPart(selector.provider))) {
        throw new Error("Invalid model prompt selector: ID and provider must be nonempty, unspaced tokens");
      }
      if (channel !== "general" && channel !== "leadOnly") throw new Error("Invalid model prompt channel");
      if (text !== undefined && (typeof text !== "string" || !text.trim())) throw new Error("Empty model prompt text: use clear to remove a channel");
      await mkdir(dirname(path), { recursive: true });
      return withLock(path, async () => {
        const current = await load(path); // Never merge against a snapshot read before acquiring the lock.
        const rules: ModelPromptRule[] = current.rules.map(rule => ({ ...rule }));
        const key = selectorKey(selector);
        const index = rules.findIndex(rule => selectorKey(rule) === key);
        if (index >= 0) {
          const rule = rules[index]!;
          if (text === undefined) delete rule[channel];
          else rule[channel] = text;
          if (rule.general === undefined && rule.leadOnly === undefined) rules.splice(index, 1);
        } else if (text !== undefined) {
          rules.push({ id: selector.id, ...(selector.provider === undefined ? {} : { provider: selector.provider }), [channel]: text });
        }
        const next: ModelPromptConfig = { version: 1, rules };
        // Clearing a missing selector is a successful no-op; avoid an unnecessary replacement.
        if (JSON.stringify(current) !== JSON.stringify(next)) await replace(path, next);
        return next;
      });
    },
  };
}

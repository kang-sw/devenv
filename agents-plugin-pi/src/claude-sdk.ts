import { existsSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";
import type { ClaudeDelegatePreset } from "./claude-delegate-prompts.ts";
import { buildClaudeRequest, buildClaudeTaskFrame } from "./claude-delegate-prompts.ts";

export const CLAUDE_READ_TOOLS = ["Read", "Grep", "Glob", "WebSearch", "WebFetch"] as const;
export type ClaudeUsage = Record<string, unknown> | null;

export interface ClaudeQuery {
  [Symbol.asyncIterator](): AsyncIterator<Record<string, unknown>>;
  close(): void;
}
export interface ClaudeSdk { query(args: { prompt: string; options: Record<string, unknown> }): ClaudeQuery; }
export interface ClaudeSdkDependencies { loadSdk?: () => Promise<ClaudeSdk>; executable?: string; env?: NodeJS.ProcessEnv; }
export interface ClaudeRunInput { preset: ClaudeDelegatePreset; request: string; paths?: readonly string[]; model?: string; cwd: string; abortController: AbortController; }
export interface ClaudeRunOutput { output: string; usage: ClaudeUsage; }

function safeError(error: unknown): string {
  const text = error instanceof Error ? error.message : String(error);
  return text.replace(/[\r\n]+/g, " ").slice(0, 500);
}

export function delegateEnvironment(env: NodeJS.ProcessEnv = process.env): Record<string, string> {
  const allowed = ["HOME", "PATH", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "LC_CTYPE", "TMPDIR"];
  return Object.fromEntries(allowed.flatMap((key) => typeof env[key] === "string" ? [[key, env[key]!]] : []));
}

export function resolveClaudeExecutable(executable?: string): string {
  const candidate = executable ?? join(homedir(), ".local", "bin", "claude");
  if (!existsSync(candidate)) throw new Error("Claude executable is unavailable for ws-claude");
  return candidate;
}

export function buildClaudeOptions(input: ClaudeRunInput, executable: string, env: NodeJS.ProcessEnv = process.env): Record<string, unknown> {
  return {
    systemPrompt: { type: "preset", preset: "claude_code", append: buildClaudeTaskFrame(input.preset) },
    cwd: input.cwd, executable, env: delegateEnvironment(env), model: input.model,
    strictMcpConfig: true, mcpServers: {}, settingSources: [], tools: [...CLAUDE_READ_TOOLS], allowedTools: [...CLAUDE_READ_TOOLS],
    permissionMode: "dontAsk", persistSession: false, maxTurns: 20, abortController: input.abortController,
    canUseTool: async (name: string) => CLAUDE_READ_TOOLS.includes(name as never) ? { behavior: "allow" } : { behavior: "deny", message: "ws-claude permits read-only tools." },
  };
}

async function defaultSdk(): Promise<ClaudeSdk> {
  return await import("@anthropic-ai/claude-agent-sdk") as unknown as ClaudeSdk;
}

/** Consumes one isolated SDK query and retains only terminal text and usage. */
export async function runClaudeItem(input: ClaudeRunInput, dependencies: ClaudeSdkDependencies = {}): Promise<ClaudeRunOutput> {
  const sdk = await (dependencies.loadSdk ?? defaultSdk)();
  const query = sdk.query({ prompt: buildClaudeRequest(input), options: buildClaudeOptions(input, resolveClaudeExecutable(dependencies.executable), dependencies.env) });
  let result: Record<string, unknown> | undefined;
  const consume = async () => {
    for await (const message of query) {
      if (message.type === "system" && (Array.isArray(message.tools) && message.tools.some((tool) => !CLAUDE_READ_TOOLS.includes(String(tool) as never)))) throw new Error("profile_violation: unexpected tool inventory");
      if (message.type === "result") result = message;
    }
  };
  const aborted = new Promise<never>((_, reject) => input.abortController.signal.addEventListener("abort", () => {
    query.close(); reject(input.abortController.signal.reason instanceof Error ? input.abortController.signal.reason : new Error("cancelled"));
  }, { once: true }));
  try { await Promise.race([consume(), aborted]); }
  catch (error) { throw new Error(safeError(error)); }
  finally { query.close(); }
  if (!result) throw new Error("missing_result: Claude stream ended without a result");
  if (result.subtype !== "success" || result.is_error === true) throw new Error(`sdk_error: ${String(result.subtype ?? "error")}`);
  return { output: typeof result.result === "string" ? result.result : "", usage: (result.modelUsage ?? null) as ClaudeUsage };
}

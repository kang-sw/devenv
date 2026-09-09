import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { join } from "node:path";
import { homedir } from "node:os";
import type { Options, Query, SDKMessage, SpawnOptions, SpawnedProcess } from "@anthropic-ai/claude-agent-sdk";
import type { ClaudeDelegatePreset } from "./claude-delegate-prompts.ts";
import { buildClaudeRequest, buildClaudeTaskFrame } from "./claude-delegate-prompts.ts";

export const CLAUDE_READ_TOOLS = ["Read", "Grep", "Glob", "WebSearch", "WebFetch"] as const;
export type ClaudeUsage = { usage: Record<string, unknown>; model_usage: Record<string, unknown>; cost_estimate_usd: number | null } | null;
export interface ClaudeSdk { query(args: { prompt: string; options?: Options }): Query; }
export interface ClaudeSdkDependencies { loadSdk?: () => Promise<ClaudeSdk>; executable?: string; env?: NodeJS.ProcessEnv; cleanupMs?: number; spawnProcess?: (options: SpawnOptions) => SpawnedProcess; }
export interface ClaudeRunInput { preset: ClaudeDelegatePreset; request: string; paths?: readonly string[]; model?: string; cwd: string; abortController: AbortController; }
export interface ClaudeRunOutput { output: string; usage: ClaudeUsage; }
export class ClaudeDelegateError extends Error { readonly code: "timeout" | "cancelled" | "sdk_error" | "missing_result" | "profile_violation" | "cleanup_failed"; constructor(code: "timeout" | "cancelled" | "sdk_error" | "missing_result" | "profile_violation" | "cleanup_failed", message: string) { super(message); this.code = code; } }
const SAFE: Record<ClaudeDelegateError["code"], string> = { timeout: "Claude request timed out.", cancelled: "Claude request was cancelled.", sdk_error: "Claude request failed.", missing_result: "Claude returned no terminal result.", profile_violation: "Claude started with an unexpected tool profile.", cleanup_failed: "Claude child cleanup could not be confirmed." };
function fail(code: ClaudeDelegateError["code"]): never { throw new ClaudeDelegateError(code, SAFE[code]); }
export function delegateEnvironment(env: NodeJS.ProcessEnv = process.env): Record<string, string> { const keys = ["HOME", "PATH", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "LC_CTYPE", "TMPDIR"]; return Object.fromEntries(keys.flatMap((key) => typeof env[key] === "string" ? [[key, env[key]!]] : [])); }
export function resolveClaudeExecutable(executable?: string): string { const candidate = executable ?? join(homedir(), ".local", "bin", "claude"); if (!existsSync(candidate)) fail("sdk_error"); return candidate; }
export function buildClaudeOptions(input: ClaudeRunInput, executable: string, env: NodeJS.ProcessEnv = process.env, spawnClaudeCodeProcess?: (options: SpawnOptions) => SpawnedProcess): Options { return { systemPrompt: { type: "preset", preset: "claude_code", append: buildClaudeTaskFrame(input.preset) }, cwd: input.cwd, pathToClaudeCodeExecutable: executable, env: delegateEnvironment(env), ...(input.model ? { model: input.model } : {}), strictMcpConfig: true, mcpServers: {}, settingSources: [], tools: [...CLAUDE_READ_TOOLS], allowedTools: [...CLAUDE_READ_TOOLS], permissionMode: "dontAsk", persistSession: false, maxTurns: 20, abortController: input.abortController, canUseTool: async (name) => CLAUDE_READ_TOOLS.includes(name as never) ? { behavior: "allow", updatedInput: undefined } : { behavior: "deny", message: "ws-claude permits read-only tools." }, ...(spawnClaudeCodeProcess ? { spawnClaudeCodeProcess } : {}) }; }
async function defaultSdk(): Promise<ClaudeSdk> { return await import("@anthropic-ai/claude-agent-sdk") as unknown as ClaudeSdk; }
function exited(child: SpawnedProcess, ms: number): Promise<boolean> { if (child.exitCode !== null || child.signalCode) return Promise.resolve(true); return new Promise((resolve) => { const done = () => { clearTimeout(timer); resolve(true); }; const timer = setTimeout(() => { child.removeListener?.("exit", done); resolve(false); }, ms); child.on("exit", done); }); }
export async function disposeClaudeChild(query: Query | undefined, child: SpawnedProcess | undefined, cleanupMs = 2_000): Promise<boolean> { query?.close(); if (!child) return true; if (await exited(child, cleanupMs / 2)) return true; try { child.kill("SIGTERM"); } catch {} if (await exited(child, cleanupMs / 4)) return true; try { child.kill("SIGKILL"); } catch {} return exited(child, cleanupMs / 4); }
export async function runClaudeItem(input: ClaudeRunInput, dependencies: ClaudeSdkDependencies = {}): Promise<ClaudeRunOutput> {
  let query: Query | undefined; let child: SpawnedProcess | undefined; let closed = false;
  const close = () => { if (!closed) { closed = true; query?.close(); } }; const onAbort = () => close(); input.abortController.signal.addEventListener("abort", onAbort, { once: true });
  const cancelled = new Promise<never>((_, reject) => { if (input.abortController.signal.aborted) reject(new ClaudeDelegateError("cancelled", SAFE.cancelled)); else input.abortController.signal.addEventListener("abort", () => reject(new ClaudeDelegateError("cancelled", SAFE.cancelled)), { once: true }); });
  const spawnOwned = dependencies.spawnProcess ?? ((options: SpawnOptions) => spawn(options.command, options.args, { cwd: options.cwd, env: options.env, signal: options.signal }) as unknown as SpawnedProcess);
  try {
    const sdk = await Promise.race([(dependencies.loadSdk ?? defaultSdk)(), cancelled]); if (input.abortController.signal.aborted) fail("cancelled");
    query = sdk.query({ prompt: buildClaudeRequest(input), options: buildClaudeOptions(input, resolveClaudeExecutable(dependencies.executable), dependencies.env, (options) => { child = spawnOwned(options); return child; }) });
    let terminal: Extract<SDKMessage, { type: "result" }> | undefined;
    const consume = async () => { for await (const message of query!) { if (message.type === "system" && (message.subtype !== "init" || message.tools.some((tool) => !CLAUDE_READ_TOOLS.includes(tool as never)) || message.mcp_servers.length !== 0)) fail("profile_violation"); if (message.type === "result") terminal = message; } };
    await Promise.race([consume(), cancelled]); if (!terminal) fail("missing_result"); if (terminal.subtype !== "success" || terminal.is_error !== false || typeof terminal.result !== "string") fail("sdk_error");
    const usage = terminal.usage && terminal.modelUsage && typeof terminal.total_cost_usd === "number" ? { usage: terminal.usage as unknown as Record<string, unknown>, model_usage: terminal.modelUsage as Record<string, unknown>, cost_estimate_usd: terminal.total_cost_usd } : null;
    return { output: terminal.result, usage };
  } catch (error) { if (error instanceof ClaudeDelegateError) throw error; fail(input.abortController.signal.aborted ? "cancelled" : "sdk_error"); }
  finally { input.abortController.signal.removeEventListener("abort", onAbort); if (!(await disposeClaudeChild(query, child, dependencies.cleanupMs))) throw new ClaudeDelegateError("cleanup_failed", SAFE.cleanup_failed); }
}

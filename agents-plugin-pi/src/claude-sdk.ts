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
interface ChildOwnership {
  exited: boolean;
  error: Promise<never>;
  detach(): void;
}
const ownedChildren = new WeakMap<SpawnedProcess, ChildOwnership>();
function ownChild(child: SpawnedProcess): ChildOwnership {
  const existing = ownedChildren.get(child);
  if (existing) return existing;
  let rejectError!: (error: Error) => void;
  const error = new Promise<never>((_, reject) => { rejectError = reject; });
  // A process can fail during finalization, after the consumer race has ended.
  void error.catch(() => {});
  const onExit = () => { ownership.exited = true; };
  const onError = () => rejectError(new ClaudeDelegateError("sdk_error", SAFE.sdk_error));
  const streams = [child.stdin, child.stdout, (child as SpawnedProcess & { stderr?: NodeJS.ReadableStream }).stderr];
  const ownership: ChildOwnership = {
    exited: child.exitCode !== null || Boolean(child.signalCode), error,
    detach() {
      child.off("exit", onExit); child.off("error", onError);
      for (const stream of streams) stream?.off("error", onError);
      ownedChildren.delete(child);
    },
  };
  child.on("exit", onExit); child.on("error", onError);
  for (const stream of streams) stream?.on("error", onError);
  ownedChildren.set(child, ownership);
  return ownership;
}
function exited(child: SpawnedProcess, ownership: ChildOwnership, ms: number): Promise<boolean> {
  if (ownership.exited || child.exitCode !== null || child.signalCode) return Promise.resolve(true);
  return new Promise(resolve => {
    const finish = (value: boolean) => { clearTimeout(timer); child.off("exit", done); resolve(value); };
    const done = () => finish(true);
    const timer = setTimeout(() => finish(false), Math.max(0, ms));
    child.on("exit", done);
  });
}
export async function disposeClaudeChild(query: Query | undefined, child: SpawnedProcess | undefined, cleanupMs = 2_000): Promise<boolean> {
  const deadline = performance.now() + cleanupMs;
  const ownership = child ? ownChild(child) : undefined;
  // SDK teardown is advisory. It cannot bypass our process/stream ownership.
  try { query?.close(); } catch {}
  if (!child || !ownership) return true;
  let confirmed = false;
  let streamsClosed = true;
  try {
    try { child.stdin.end(); } catch {}
    confirmed = await exited(child, ownership, Math.min(cleanupMs / 2, Math.max(0, deadline - performance.now())));
    for (const signal of ["SIGTERM", "SIGKILL"] as const) {
      if (confirmed) break;
      try { child.kill(signal); } catch {}
      confirmed = await exited(child, ownership, Math.min(cleanupMs / 5, Math.max(0, deadline - performance.now())));
    }
  } finally {
    const streams = [child.stdin, child.stdout, (child as SpawnedProcess & { stderr?: import("node:stream").Readable }).stderr];
    const closed = await Promise.all(streams.filter(stream => stream !== undefined).map(stream => new Promise<boolean>(resolve => {
      if (stream.closed) { resolve(true); return; }
      const finish = (value: boolean) => { clearTimeout(timer); stream.off("close", onClose); resolve(value); };
      const onClose = () => finish(true);
      const timer = setTimeout(() => finish(false), Math.max(0, deadline - performance.now()));
      stream.on("close", onClose);
      try { stream.destroy(); } catch { finish(false); }
    })));
    streamsClosed = closed.every(Boolean);
    ownership.detach();
  }
  return confirmed && streamsClosed;
}
function spawnClaudeProcess(options: SpawnOptions): SpawnedProcess {
  // Own the forwarded signal listener too: Node's spawn({ signal }) retains
  // its exit listener after ENOENT, where no exit event will ever arrive.
  const child = spawn(options.command, options.args, { cwd: options.cwd, env: options.env });
  const onAbort = () => { try { child.kill("SIGTERM"); } catch {} };
  const detach = () => {
    options.signal?.removeEventListener("abort", onAbort);
    child.off("exit", detach); child.off("error", detach);
  };
  options.signal?.addEventListener("abort", onAbort, { once: true });
  child.once("exit", detach); child.once("error", detach);
  if (options.signal?.aborted) onAbort();
  return child;
}
export async function runClaudeItem(input: ClaudeRunInput, dependencies: ClaudeSdkDependencies = {}): Promise<ClaudeRunOutput> {
  let query: Query | undefined; let child: SpawnedProcess | undefined; let finalized = false;
  let onAbort!: () => void;
  const cancelled = new Promise<never>((_, reject) => {
    onAbort = () => reject(new ClaudeDelegateError("cancelled", SAFE.cancelled));
    if (input.abortController.signal.aborted) onAbort();
    else input.abortController.signal.addEventListener("abort", onAbort, { once: true });
  });
  let rejectProcess!: (error: Error) => void;
  const processError = new Promise<never>((_, reject) => { rejectProcess = reject; });
  void processError.catch(() => {});
  const spawnOwned = dependencies.spawnProcess ?? spawnClaudeProcess;
  try {
    const sdk = await Promise.race([(dependencies.loadSdk ?? defaultSdk)(), cancelled]); if (input.abortController.signal.aborted) fail("cancelled");
    query = sdk.query({ prompt: buildClaudeRequest(input), options: buildClaudeOptions(input, resolveClaudeExecutable(dependencies.executable), dependencies.env, (options) => { if (child || finalized || input.abortController.signal.aborted) throw new ClaudeDelegateError("cancelled", SAFE.cancelled); child = spawnOwned(options); void ownChild(child).error.catch(rejectProcess); return child; }) });
    let terminal: Extract<SDKMessage, { type: "result" }> | undefined;
    const consume = async () => { for await (const message of query!) { if (finalized || input.abortController.signal.aborted) return; if (message.type === "system" && message.subtype === "init" && (message.tools.some((tool) => !CLAUDE_READ_TOOLS.includes(tool as never)) || message.mcp_servers.length !== 0)) fail("profile_violation"); if (message.type === "result") terminal = message; } };
    await Promise.race([consume(), cancelled, processError]); if (!terminal) fail("missing_result"); if (terminal.subtype !== "success" || terminal.is_error !== false || typeof terminal.result !== "string") fail("sdk_error");
    const usage = terminal.usage && terminal.modelUsage && typeof terminal.total_cost_usd === "number" ? { usage: terminal.usage as unknown as Record<string, unknown>, model_usage: terminal.modelUsage as Record<string, unknown>, cost_estimate_usd: terminal.total_cost_usd } : null;
    return { output: terminal.result, usage };
  } catch (error) { if (error instanceof ClaudeDelegateError) throw error; fail(input.abortController.signal.aborted ? "cancelled" : "sdk_error"); }
  finally { finalized = true; input.abortController.signal.removeEventListener("abort", onAbort); input.abortController.abort(); if (!(await disposeClaudeChild(query, child, dependencies.cleanupMs))) throw new ClaudeDelegateError("cleanup_failed", SAFE.cleanup_failed); }
}

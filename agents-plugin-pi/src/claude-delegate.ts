import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { runClaudeItem, type ClaudeSdkDependencies, type ClaudeUsage } from "./claude-sdk.ts";
import type { ClaudeDelegatePreset } from "./claude-delegate-prompts.ts";

export const CLAUDE_DELEGATE_TOOL_NAME = "ws-claude";
const WORDS = ["amber", "birch", "cedar", "dawn", "elm", "fjord", "grove", "harbor", "iris", "juniper", "kite", "lumen"];
export interface ClaudeDelegateItem { preset: ClaudeDelegatePreset; request: string; paths?: string[]; model?: string; editTargets?: unknown; resume?: unknown; }
export interface ClaudeDelegateResult { id: string; status: "success" | "error"; output: string; usage: ClaudeUsage; error?: { code: string; message: string }; }
export interface ClaudeDelegateController { execute(items: unknown, signal?: AbortSignal): Promise<ClaudeDelegateResult[]>; shutdown(): Promise<void>; }
export interface ClaudeDelegateDeps extends ClaudeSdkDependencies { timeoutMs?: number; cleanupMs?: number; }

function errorResult(id: string, code: string, message: string): ClaudeDelegateResult { return { id, status: "error", output: "", usage: null, error: { code, message: message.slice(0, 500) } }; }
function validItem(value: unknown): value is ClaudeDelegateItem {
  if (!value || typeof value !== "object") return false;
  const item = value as ClaudeDelegateItem;
  return (item.preset === "audit" || item.preset === "consult") && typeof item.request === "string" && item.request.trim().length > 0 &&
    (item.paths === undefined || (Array.isArray(item.paths) && item.paths.every((path) => typeof path === "string" && path.trim().length > 0))) &&
    (item.model === undefined || (typeof item.model === "string" && item.model.trim().length > 0)) && item.editTargets === undefined && item.resume === undefined;
}
function errorCode(error: unknown, signal?: AbortSignal): "timeout" | "cancelled" | "sdk_error" | "missing_result" | "profile_violation" {
  const text = error instanceof Error ? error.message : String(error);
  if (signal?.aborted) return "cancelled";
  if (text.includes("timeout")) return "timeout";
  if (text.includes("missing_result")) return "missing_result";
  if (text.includes("profile_violation")) return "profile_violation";
  return "sdk_error";
}

export function allocateClaudeHandle(used: Set<string>): string {
  for (const a of WORDS) for (const b of WORDS) for (const c of WORDS) {
    const handle = `${a}-${b}-${c}`;
    if (!used.has(handle)) { used.add(handle); return handle; }
  }
  throw new Error("Claude delegate handle space exhausted");
}

export function createClaudeDelegateController(cwd: () => string, deps: ClaudeDelegateDeps = {}): ClaudeDelegateController {
  const used = new Set<string>(); let closed = false; let active = 0; const queue: (() => void)[] = []; const controllers = new Set<AbortController>();
  const acquire = async (signal?: AbortSignal) => new Promise<void>((resolve, reject) => {
    const start = () => { active += 1; resolve(); }; if (closed || signal?.aborted) { reject(new Error("cancelled")); return; }
    if (active < 3) start(); else { const cancel = () => { const index = queue.indexOf(start); if (index >= 0) queue.splice(index, 1); reject(new Error("cancelled")); }; signal?.addEventListener("abort", cancel, { once: true }); queue.push(start); }
  });
  const release = () => { active -= 1; queue.shift()?.(); };
  const executeOne = async (value: unknown, id: string, callerSignal?: AbortSignal): Promise<ClaudeDelegateResult> => {
    if (!validItem(value)) return errorResult(id, "invalid_item", "Each item needs a supported preset and nonblank request; edit-targets and resume are unavailable.");
    try { await acquire(callerSignal); } catch { return errorResult(id, "cancelled", "Invocation was cancelled before this item started."); }
    const abort = new AbortController(); controllers.add(abort); const onAbort = () => abort.abort(); callerSignal?.addEventListener("abort", onAbort, { once: true });
    const timeout = setTimeout(() => abort.abort(new Error("timeout")), deps.timeoutMs ?? 120_000);
    try {
      const output = await runClaudeItem({ ...value, cwd: cwd(), abortController: abort }, deps);
      return { id, status: "success", output: output.output, usage: output.usage };
    } catch (error) { return errorResult(id, errorCode(error, callerSignal), error instanceof Error ? error.message : String(error)); }
    finally { clearTimeout(timeout); callerSignal?.removeEventListener("abort", onAbort); abort.abort(); controllers.delete(abort); release(); }
  };
  return {
    async execute(items: unknown, signal?: AbortSignal) {
      if (!Array.isArray(items) || items.length === 0) throw new Error("ws-claude requires a non-empty items array");
      if (closed) throw new Error("ws-claude session is shutting down");
      const jobs = items.map((item) => { const id = allocateClaudeHandle(used); return executeOne(item, id, signal); });
      return Promise.all(jobs);
    },
    async shutdown() { closed = true; for (const abort of controllers) abort.abort(); while (queue.length) queue.shift()!(); },
  };
}

export function addClaudeDelegateIfLead(active: readonly string[], role: string | undefined): string[] {
  return role === undefined && !active.includes(CLAUDE_DELEGATE_TOOL_NAME) ? [...active, CLAUDE_DELEGATE_TOOL_NAME] : [...active];
}

export function registerClaudeDelegate(pi: ExtensionAPI, controllerRef: { current: ClaudeDelegateController | undefined }, toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef()): void {
  registerWsTool(pi, {
    name: CLAUDE_DELEGATE_TOOL_NAME, label: CLAUDE_DELEGATE_TOOL_NAME,
    description: "Run isolated, read-only Claude audit or consult requests. Supply one or more independent items.",
    parameters: { type: "object", additionalProperties: false, properties: { items: { type: "array", minItems: 1, items: { type: "object", additionalProperties: false, properties: { preset: { type: "string", enum: ["audit", "consult"] }, request: { type: "string", minLength: 1 }, paths: { type: "array", items: { type: "string", minLength: 1 } }, model: { type: "string", minLength: 1 } }, required: ["preset", "request"] } }, }, required: ["items"] } as never,
    async execute(_id, params, signal) { const results = await controllerRef.current?.execute((params as { items?: unknown }).items, signal); if (!results) throw new Error("ws-claude is unavailable outside an active lead session"); const text = JSON.stringify(results); return { content: [{ type: "text", text }], details: { items: results } }; },
  } as never, toolPreviewTuiRef);
}

import { readSpawnRole } from "./process-role.ts";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { ClaudeDelegateError, createClaudeEditScope, runClaudeItem, type ClaudeEditScope, type ClaudeSdkDependencies, type ClaudeUsage } from "./claude-sdk.ts";
import type { ClaudeDelegatePreset } from "./claude-delegate-prompts.ts";

export const CLAUDE_DELEGATE_TOOL_NAME = "ws-claude";
const WORDS = ["amber", "birch", "cedar", "dawn", "elm", "fjord", "grove", "harbor", "iris", "juniper", "kite", "lumen"];
export interface ClaudeDelegateItem { preset: ClaudeDelegatePreset; request: string; paths?: string[]; model?: string; "edit-targets"?: string[]; editTargets?: unknown; resume?: unknown; }
export interface ClaudeDelegateResult { id: string; status: "success" | "error"; output: string; usage: ClaudeUsage; changed?: string[]; error?: { code: string; message: string }; }
export interface ClaudeDelegateController { execute(items: unknown, signal?: AbortSignal): Promise<ClaudeDelegateResult[]>; shutdown(): Promise<void>; }
export interface ClaudeDelegateDeps extends ClaudeSdkDependencies { timeoutMs?: number; cleanupMs?: number; }

function errorResult(id: string, code: string, message: string, changed?: string[]): ClaudeDelegateResult { return { id, status: "error", output: "", usage: null, ...(changed ? { changed } : {}), error: { code, message: message.slice(0, 500) } }; }
function validItem(value: unknown): value is ClaudeDelegateItem {
  if (!value || typeof value !== "object") return false;
  const item = value as ClaudeDelegateItem;
  if (Object.keys(item).some((key) => !["preset", "request", "paths", "model", "edit-targets"].includes(key))) return false;
  const editTargets = item["edit-targets"];
  const preset = item.preset === "audit" || item.preset === "consult" || item.preset === "rewrite";
  const targetShape = editTargets === undefined || (Array.isArray(editTargets) && editTargets.length > 0 && editTargets.every(path => typeof path === "string" && path.trim().length > 0));
  const targetContract = item.preset === "rewrite" ? editTargets !== undefined && targetShape : editTargets === undefined;
  return preset && targetContract && typeof item.request === "string" && item.request.trim().length > 0 &&
    (item.paths === undefined || (Array.isArray(item.paths) && item.paths.every((path) => typeof path === "string" && path.trim().length > 0))) &&
    (item.model === undefined || (typeof item.model === "string" && item.model.trim().length > 0)) && item.editTargets === undefined && item.resume === undefined;
}
function errorCode(error: unknown, signal?: AbortSignal): "timeout" | "cancelled" | "sdk_error" | "missing_result" | "profile_violation" | "cleanup_failed" {
  if (error instanceof ClaudeDelegateError) return error.code;
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
  const used = new Set<string>(); let closed = false; let active = 0; let quarantined = false; const queue: { start: () => void; reject: () => void }[] = []; const controllers = new Set<AbortController>(); const running = new Set<Promise<unknown>>(); const scheduledEditTargets = new Set<string>();
  const quarantine = () => { quarantined = true; for (const entry of queue.splice(0)) entry.reject(); };
  const acquire = async (signal?: AbortSignal) => new Promise<void>((resolve, reject) => {
    const detach = () => signal?.removeEventListener("abort", cancel);
    const start = () => {
      detach();
      if (closed || quarantined || signal?.aborted) { reject(new Error("cancelled")); return; }
      active += 1; resolve();
    };
    const entry = { start, reject: () => { detach(); reject(new Error("cancelled")); } };
    const cancel = () => {
      const index = queue.indexOf(entry);
      if (index >= 0) queue.splice(index, 1);
      entry.reject();
    };
    if (closed || quarantined || signal?.aborted) { entry.reject(); return; }
    if (active < 3) start();
    else { signal?.addEventListener("abort", cancel, { once: true }); queue.push(entry); }
  });
  const release = () => { active -= 1; if (!closed && !quarantined) queue.shift()?.start(); };
  const executeOne = async (value: unknown, id: string, callerSignal?: AbortSignal): Promise<ClaudeDelegateResult> => {
    if (!validItem(value)) return errorResult(id, "invalid_item", "Each item needs a supported preset and nonblank request; rewrite alone requires non-empty edit-targets, while read-only presets forbid them.");
    let editScope: ClaudeEditScope | undefined;
    if (value.preset === "rewrite") {
      try { editScope = createClaudeEditScope(cwd(), value["edit-targets"]!); }
      catch { return errorResult(id, "invalid_edit_target", "Rewrite edit targets must be exact files inside the worktree with existing in-root parents."); }
      if (editScope.canonicalTargets.some(target => scheduledEditTargets.has(target))) return errorResult(id, "edit_target_conflict", "Rewrite edit targets overlap another scheduled item.");
      for (const target of editScope.canonicalTargets) scheduledEditTargets.add(target);
    }
    let acquired = false;
    try {
      try { await acquire(callerSignal); acquired = true; } catch { return errorResult(id, "cancelled", "Invocation was cancelled before this item started.", editScope?.changed()); }
      if (callerSignal?.aborted || closed || quarantined) return errorResult(id, "cancelled", "Invocation was cancelled before this item started.", editScope?.changed());
      const abort = new AbortController(); controllers.add(abort); const onAbort = () => abort.abort(); callerSignal?.addEventListener("abort", onAbort, { once: true });
      let timedOut = false; const timeout = setTimeout(() => { timedOut = true; abort.abort(new Error("timeout")); }, deps.timeoutMs ?? 120_000);
      try {
        const { "edit-targets": _editTargets, ...request } = value;
        const output = await runClaudeItem({ ...request, cwd: editScope?.root ?? cwd(), ...(editScope ? { editScope } : {}), abortController: abort }, deps);
        return { id, status: "success", output: output.output, usage: output.usage, ...(editScope ? { changed: editScope.changed() } : {}) };
      } catch (error) {
        const raw = errorCode(error, callerSignal); const code = raw === "cleanup_failed" ? raw : timedOut ? "timeout" : raw;
        if (code === "cleanup_failed") quarantine();
        return errorResult(id, code, code === "timeout" ? "Claude request timed out." : "Claude request failed.", editScope?.changed());
      } finally { clearTimeout(timeout); callerSignal?.removeEventListener("abort", onAbort); controllers.delete(abort); }
    } finally {
      if (acquired) release();
      for (const target of editScope?.canonicalTargets ?? []) scheduledEditTargets.delete(target);
    }
  };
  return {
    async execute(items: unknown, signal?: AbortSignal) {
      if (!Array.isArray(items) || items.length === 0) throw new Error("ws-claude requires a non-empty items array");
      if (closed) throw new Error("ws-claude session is shutting down");
      const ids: string[] = []; try { for (let index = 0; index < items.length; index += 1) ids.push(allocateClaudeHandle(used)); } catch { for (const id of ids) used.delete(id); throw new Error("Claude delegate handle space exhausted"); }
      const jobs = items.map((item, index) => { const job = executeOne(item, ids[index]!, signal); running.add(job); void job.finally(() => running.delete(job)); return job; }); return Promise.allSettled(jobs).then((settled) => settled.map((entry, index) => entry.status === "fulfilled" ? entry.value : errorResult(ids[index]!, "sdk_error", "Claude request failed.")));
    },
    async shutdown() { closed = true; for (const entry of queue.splice(0)) entry.reject(); for (const abort of controllers) abort.abort(); await Promise.allSettled([...running]); },
  };
}

export function addClaudeDelegateIfLead(active: readonly string[], role: string | undefined): string[] {
  return role === undefined && !active.includes(CLAUDE_DELEGATE_TOOL_NAME) ? [...active, CLAUDE_DELEGATE_TOOL_NAME] : [...active];
}

export function registerClaudeDelegate(pi: ExtensionAPI, controllerRef: { current: ClaudeDelegateController | undefined }, toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef()): void {
  registerWsTool(pi, {
    name: CLAUDE_DELEGATE_TOOL_NAME, label: CLAUDE_DELEGATE_TOOL_NAME,
    description: "Run isolated Claude audit, consult, or exact-file rewrite requests. Supply one or more independent items; rewrite requires edit-targets and leaves changes unstaged.",
    parameters: { type: "object", additionalProperties: false, properties: { items: { type: "array", minItems: 1, items: { type: "object", additionalProperties: false, properties: { preset: { type: "string", enum: ["audit", "consult", "rewrite"] }, request: { type: "string", minLength: 1 }, paths: { type: "array", items: { type: "string", minLength: 1 } }, model: { type: "string", minLength: 1 }, "edit-targets": { type: "array", minItems: 1, items: { type: "string", minLength: 1 } } }, required: ["preset", "request"] } }, }, required: ["items"] } as never,
    async execute(_id, params, signal) { const results = await controllerRef.current?.execute((params as { items?: unknown }).items, signal); if (!results) throw new Error("ws-claude is unavailable outside an active lead session"); const text = JSON.stringify(results); return { content: [{ type: "text", text }], details: { items: results } }; },
  } as never, toolPreviewTuiRef);
}

/** Production registration and session ownership seam; creates no SDK at registration. */
export function registerClaudeDelegateSession(pi: ExtensionAPI, toolPreviewTuiRef: ToolPreviewTuiRef, deps: ClaudeDelegateDeps = {}) {
  const ref: { current: ClaudeDelegateController | undefined } = { current: undefined };
  registerClaudeDelegate(pi, ref, toolPreviewTuiRef);
  return {
    async start(cwd: string) {
      const previous = ref.current;
      ref.current = undefined;
      await previous?.shutdown();
      const role = readSpawnRole(process.env);
      if (role === undefined || (role === "fork" && pi.getActiveTools().includes(CLAUDE_DELEGATE_TOOL_NAME))) {
        ref.current = createClaudeDelegateController(() => cwd, deps);
      }
    },
    async shutdown() {
      const previous = ref.current;
      ref.current = undefined;
      await previous?.shutdown();
    },
  };
}

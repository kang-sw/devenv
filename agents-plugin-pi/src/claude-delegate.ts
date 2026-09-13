import { readSpawnRole } from "./process-role.ts";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { ClaudeDelegateError, createClaudeEditScope, runClaudeItem, type ClaudeEditScope, type ClaudeSdkDependencies, type ClaudeUsage } from "./claude-sdk.ts";
import type { ClaudeDelegatePreset } from "./claude-delegate-prompts.ts";
import type { ClaudeDesignReviewContextProvider } from "./claude-design-review.ts";

export const CLAUDE_DELEGATE_TOOL_NAME = "ws-claude";
const WORDS = ["amber", "birch", "cedar", "dawn", "elm", "fjord", "grove", "harbor", "iris", "juniper", "kite", "lumen"];
export interface ClaudeDelegateItem { preset?: ClaudeDelegatePreset; request: string; paths?: string[]; model?: string; "edit-targets"?: string[]; editTargets?: unknown; resume?: string; }
export interface ClaudeDelegateResult { id: string; status: "success" | "error"; output: string; usage: ClaudeUsage; changed?: string[]; error?: { code: string; message: string }; }
export interface ClaudeDelegateController { execute(items: unknown, signal?: AbortSignal): Promise<ClaudeDelegateResult[]>; shutdown(): Promise<void>; }
export interface ClaudeDelegateDeps extends ClaudeSdkDependencies { timeoutMs?: number; cleanupMs?: number; designReviewContext?: ClaudeDesignReviewContextProvider; }
interface ClaudeSessionRecord { sessionId: string; preset: ClaudeDelegatePreset; paths?: string[]; model?: string; editTargets?: string[]; taskFrame?: string; }

function errorResult(id: string, code: string, message: string, changed?: string[]): ClaudeDelegateResult { return { id, status: "error", output: "", usage: null, ...(changed ? { changed } : {}), error: { code, message: message.slice(0, 500) } }; }
function resumeHandle(value: unknown): string | undefined {
  if (!value || typeof value !== "object") return undefined;
  const resume = (value as { resume?: unknown }).resume;
  return typeof resume === "string" && /^[a-z]+-[a-z]+-[a-z]+$/.test(resume) ? resume : undefined;
}
function validItem(value: unknown): value is ClaudeDelegateItem {
  if (!value || typeof value !== "object") return false;
  const item = value as ClaudeDelegateItem;
  if (Object.keys(item).some((key) => !["preset", "request", "paths", "model", "edit-targets", "resume"].includes(key))) return false;
  if (typeof item.request !== "string" || item.request.trim().length === 0) return false;
  if (item.resume !== undefined) return resumeHandle(item) !== undefined && Object.keys(item).every(key => key === "resume" || key === "request");
  const editTargets = item["edit-targets"];
  const preset = item.preset === "audit" || item.preset === "consult" || item.preset === "rewrite" || item.preset === "design-review";
  const targetShape = editTargets === undefined || (Array.isArray(editTargets) && editTargets.length > 0 && editTargets.every(path => typeof path === "string" && path.trim().length > 0));
  const targetContract = item.preset === "rewrite" ? editTargets !== undefined && targetShape : editTargets === undefined;
  return preset && targetContract &&
    (item.paths === undefined || (Array.isArray(item.paths) && item.paths.every((path) => typeof path === "string" && path.trim().length > 0))) &&
    (item.model === undefined || (typeof item.model === "string" && item.model.trim().length > 0)) && item.editTargets === undefined;
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
  const used = new Set<string>(); const sessions = new Map<string, ClaudeSessionRecord>(); const activeResumes = new Set<string>(); let closed = false; let active = 0; let quarantined = false; const queue: { start: () => void; reject: () => void }[] = []; const controllers = new Set<AbortController>(); const running = new Set<Promise<unknown>>(); const scheduledEditTargets = new Set<string>();
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
  const abortable = async <T>(promise: Promise<T>, signal: AbortSignal): Promise<T> => {
    let rejectAbort!: (error: Error) => void;
    const onAbort = () => rejectAbort(new ClaudeDelegateError("cancelled", "Claude request was cancelled."));
    const cancelled = new Promise<never>((_, reject) => { rejectAbort = reject; signal.addEventListener("abort", onAbort, { once: true }); });
    try { if (signal.aborted) onAbort(); return await Promise.race([promise, cancelled]); }
    finally { signal.removeEventListener("abort", onAbort); }
  };
  const executeOne = async (value: unknown, id: string, callerSignal?: AbortSignal): Promise<ClaudeDelegateResult> => {
    if (!validItem(value)) return errorResult(id, "invalid_item", "Each item needs a nonblank request and either a supported preset or a valid resume handle; rewrite alone requires non-empty edit-targets, while read-only presets forbid them.");
    const resumed = value.resume !== undefined;
    const session = resumed ? sessions.get(value.resume!) : undefined;
    if (resumed && !session) return errorResult(id, "unknown_resume", "The Claude resume handle is not known in this Pi session.");
    if (resumed && activeResumes.has(id)) return errorResult(id, "resume_conflict", "The Claude resume handle already has an active continuation.");
    const preset = resumed ? session!.preset : value.preset!;
    const editTargets = resumed ? session!.editTargets : value["edit-targets"];
    const paths = resumed ? session!.paths : value.paths;
    const model = resumed ? session!.model : value.model;
    let editScope: ClaudeEditScope | undefined;
    if (preset === "rewrite") {
      try { editScope = createClaudeEditScope(cwd(), editTargets!); }
      catch { return errorResult(id, "invalid_edit_target", "Rewrite edit targets must be exact files inside the worktree with existing in-root parents."); }
      if (editScope.canonicalTargets.some(target => scheduledEditTargets.has(target))) return errorResult(id, "edit_target_conflict", "Rewrite edit targets overlap another scheduled item.");
      for (const target of editScope.canonicalTargets) scheduledEditTargets.add(target);
    }
    if (resumed) activeResumes.add(id);
    let acquired = false;
    try {
      try { await acquire(callerSignal); acquired = true; } catch { return errorResult(id, "cancelled", "Invocation was cancelled before this item started.", editScope?.changed()); }
      if (callerSignal?.aborted || closed || quarantined) return errorResult(id, "cancelled", "Invocation was cancelled before this item started.", editScope?.changed());
      const abort = new AbortController(); controllers.add(abort); const onAbort = () => abort.abort(); callerSignal?.addEventListener("abort", onAbort, { once: true });
      let timedOut = false; const timeout = setTimeout(() => { timedOut = true; abort.abort(new Error("timeout")); }, deps.timeoutMs ?? 120_000);
      try {
        let request = value.request; let taskFrame = session?.taskFrame;
        if (!resumed && preset === "design-review") {
          if (!deps.designReviewContext) return errorResult(id, "missing_context", "Design-review context is unavailable.");
          try {
            const pending = deps.designReviewContext({ cwd: cwd(), request, paths, signal: abort.signal });
            const context = await abortable(pending, abort.signal);
            request = context.request; taskFrame = context.taskFrame;
          } catch (error) {
            if (abort.signal.aborted) throw error;
            return errorResult(id, "missing_context", "Design-review context is incomplete or unavailable.");
          }
        }
        const output = await runClaudeItem({ preset, request, ...(preset === "design-review" ? {} : { paths }), model, cwd: editScope?.root ?? cwd(), ...(editScope ? { editScope } : {}), ...(session ? { resumeSessionId: session.sessionId } : {}), ...(taskFrame ? { taskFrame } : {}), abortController: abort }, deps);
        sessions.set(id, { sessionId: output.sessionId, preset, ...(paths ? { paths: [...paths] } : {}), ...(model ? { model } : {}), ...(editTargets ? { editTargets: [...editTargets] } : {}), ...(taskFrame ? { taskFrame } : {}) });
        return { id, status: "success", output: output.output, usage: output.usage, ...(editScope ? { changed: editScope.changed() } : {}) };
      } catch (error) {
        const raw = errorCode(error, callerSignal); const code = raw === "cleanup_failed" ? raw : timedOut ? "timeout" : raw;
        if (code === "cleanup_failed") quarantine();
        return errorResult(id, code, code === "timeout" ? "Claude request timed out." : "Claude request failed.", editScope?.changed());
      } finally { clearTimeout(timeout); callerSignal?.removeEventListener("abort", onAbort); controllers.delete(abort); }
    } finally {
      if (acquired) release();
      if (resumed) activeResumes.delete(id);
      for (const target of editScope?.canonicalTargets ?? []) scheduledEditTargets.delete(target);
    }
  };
  return {
    async execute(items: unknown, signal?: AbortSignal) {
      if (!Array.isArray(items) || items.length === 0) throw new Error("ws-claude requires a non-empty items array");
      if (closed) throw new Error("ws-claude session is shutting down");
      const ids: string[] = []; const allocated: string[] = [];
      try {
        for (const item of items) {
          const resumed = resumeHandle(item);
          if (resumed) ids.push(resumed);
          else { const id = allocateClaudeHandle(used); ids.push(id); allocated.push(id); }
        }
      } catch { for (const id of allocated) used.delete(id); throw new Error("Claude delegate handle space exhausted"); }
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
    description: "Run isolated Claude audit, consult, exact-file rewrite, or ticket design-review requests, or continue a returned handle. Supply one or more independent items; rewrite requires edit-targets and leaves changes unstaged. Design-review request format is `Ticket: <path>\\nRelations:\\n<context|none>`.",
    parameters: { type: "object", additionalProperties: false, properties: { items: { type: "array", minItems: 1, items: { type: "object", additionalProperties: false, properties: { preset: { type: "string", enum: ["audit", "consult", "rewrite", "design-review"] }, resume: { type: "string", pattern: "^[a-z]+-[a-z]+-[a-z]+$" }, request: { type: "string", minLength: 1 }, paths: { type: "array", items: { type: "string", minLength: 1 } }, model: { type: "string", minLength: 1 }, "edit-targets": { type: "array", minItems: 1, items: { type: "string", minLength: 1 } } }, required: ["request"], oneOf: [{ required: ["preset"], not: { required: ["resume"] } }, { required: ["resume"], not: { anyOf: [{ required: ["preset"] }, { required: ["paths"] }, { required: ["model"] }, { required: ["edit-targets"] }] } }] } }, }, required: ["items"] } as never,
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

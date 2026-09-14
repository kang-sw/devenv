/**
 * 260906 Phase 2 (YAML/TUI dispatch-row rendering): pure per-tool call
 * summary builders and the resolved-model-line formatter for the five
 * dispatch tools (`explore`, `ws-execute`, `ws-agent-spawn`, `ws-agent-send`,
 * `ws-fork`). `content` (model-visible) is never touched here — everything
 * in this module is display-only, feeding `renderCall`/`renderResult`
 * through `tool-result-render.ts`'s `ToolPreviewOverrides` seam. No static
 * `@earendil-works/pi-tui` import (mirrors `tool-result-render.ts`'s own
 * guarded-import convention) — this module only ever touches the already
 * host-resolved `ToolResultTuiModules` shape via `ToolPreviewTuiRef`.
 *
 * `ResolvedModelInfo` is imported `type`-only from `spawner.ts`: this module
 * is imported for VALUES (`createDispatchToolPreview` et al.) by
 * `spawner.ts`/`execute-gateway.ts`/`fork.ts`, so a value import in the
 * other direction would create a runtime cycle — a type-only import here is
 * erased at build/runtime and creates none (same pattern as
 * `spawner.ts`'s own `ResolveAgentCallToolClient` doc comment).
 */
import { basename } from "node:path";
import {
  createToolPreviewRenderers,
  UseNativeResultFallback,
  type ToolPreviewTuiRef,
  type ToolResultTuiModules,
} from "./tool-result-render.ts";
import type { ResolvedModelInfo } from "./spawner.ts";

export type { ResolvedModelInfo };

/** Mirrors `spawner.ts`'s `EXPLORE_TITLE_CAP`/`deriveExploreTitle` head-truncation convention. */
const SUMMARY_HEAD_CAP = 60;
const SUMMARY_TRUNCATION_MARKER = "…";

/** Head-truncates `text` to `maxChars`, defaulting a missing/malformed value to `""` — never throws. */
export function truncateHead(text: string | undefined, maxChars: number): string {
  if (typeof text !== "string" || text.length === 0) return "";
  return text.length > maxChars ? `${text.slice(0, maxChars)}${SUMMARY_TRUNCATION_MARKER}` : text;
}

/**
 * The mandatory resolved-model line shared by all five dispatch tools:
 * `→ <tier> · <model> · effort <effort>`. `undefined` `resolved` (never yet
 * published) yields `undefined` — the caller (`createDispatchToolPreview`'s
 * `resolvedLine` override) leaves the previously cached line, if any, in
 * place rather than blanking it.
 */
export function formatResolvedLine(resolved: ResolvedModelInfo | undefined): string | undefined {
  if (!resolved) return undefined;
  const effort = resolved.effort && resolved.effort.length > 0 ? resolved.effort : "pi-default";
  return `→ ${resolved.tier} · ${resolved.model ?? "?"} · effort ${effort}`;
}

/** Defensive against partial/streamed/malformed tool-call arguments: always a plain object, never throws. */
function asRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

/** `explore`: `query` only (both the lead-role `spawnAgent` call and the worker-leaf direct-resolution call share the same `{query}` shape). */
export function buildExploreSummary(args: unknown): string {
  const a = asRecord(args);
  return `query: ${truncateHead(asString(a.query), SUMMARY_HEAD_CAP)}`;
}

/** `ws-execute`: `command` when given, then `prompt` head, then a `complex` tag only when `true`. */
export function buildExecuteSummary(args: unknown): string {
  const a = asRecord(args);
  const lines: string[] = [];
  const command = asString(a.command);
  if (command !== undefined) lines.push(`command: ${truncateHead(command, SUMMARY_HEAD_CAP)}`);
  lines.push(`prompt: ${truncateHead(asString(a.prompt), SUMMARY_HEAD_CAP)}`);
  if (asBoolean(a.complex) === true) lines.push("complex: true");
  return lines.join("\n");
}

/** `ws-agent-spawn`: `alias`/`title` when given, `system_prompt_path` basename, `prompt` head, requested `model_name`/`model_effort`. */
export function buildAgentSpawnSummary(args: unknown): string {
  const a = asRecord(args);
  const lines: string[] = [];
  const alias = asString(a.alias);
  if (alias !== undefined) lines.push(`alias: ${alias}`);
  const title = asString(a.title);
  if (title !== undefined) lines.push(`title: ${title}`);
  const systemPromptPath = asString(a.system_prompt_path);
  lines.push(`system_prompt: ${systemPromptPath !== undefined ? basename(systemPromptPath) : ""}`);
  lines.push(`prompt: ${truncateHead(asString(a.prompt), SUMMARY_HEAD_CAP)}`);
  const modelName = asString(a.model_name);
  if (modelName !== undefined) lines.push(`model_name: ${modelName}`);
  const modelEffort = asString(a.model_effort);
  if (modelEffort !== undefined) lines.push(`model_effort: ${modelEffort}`);
  return lines.join("\n");
}

/** `ws-agent-send`: target alias/id, `message` head, `interrupt` tag only when `true`. */
export function buildAgentSendSummary(args: unknown): string {
  const a = asRecord(args);
  const lines: string[] = [];
  lines.push(`target: ${asString(a.agent_id) ?? ""}`);
  lines.push(`message: ${truncateHead(asString(a.message), SUMMARY_HEAD_CAP)}`);
  if (asBoolean(a.interrupt) === true) lines.push("interrupt: true");
  return lines.join("\n");
}

/** `ws-fork`: `prompt` head, `model_name` when given, `expects_commit` tag only when `true`. */
export function buildForkSummary(args: unknown): string {
  const a = asRecord(args);
  const lines: string[] = [];
  lines.push(`prompt: ${truncateHead(asString(a.prompt), SUMMARY_HEAD_CAP)}`);
  const modelName = asString(a.model_name);
  if (modelName !== undefined) lines.push(`model_name: ${modelName}`);
  if (asBoolean(a.expects_commit) === true) lines.push("expects_commit: true");
  return lines.join("\n");
}

/**
 * Factory shared by the five dispatch-tool registrations: lazily resolves
 * `tuiRef.current` (throwing `UseNativeResultFallback` when cold, mirroring
 * `registerWsTool`'s own `renderers()` closure in `tool-result-render.ts`)
 * and wires `buildCallSummary` plus `formatResolvedLine` into
 * `createToolPreviewRenderers`'s `overrides`. The resolved model/effort line
 * is read from `result.details.resolved` — every caller publishes it there
 * on both the partial `onUpdate` and the final tool return.
 */
export function createDispatchToolPreview(
  tuiRef: ToolPreviewTuiRef,
  toolName: string,
  buildCallSummary: (args: unknown) => string,
): {
  renderCall: ReturnType<typeof createToolPreviewRenderers>["renderCall"];
  renderResult: ReturnType<typeof createToolPreviewRenderers>["renderResult"];
} {
  let cachedTui: ToolResultTuiModules | undefined;
  let cachedRenderers: ReturnType<typeof createToolPreviewRenderers> | undefined;
  const renderers = () => {
    const tui = tuiRef.current;
    if (!tui) throw new UseNativeResultFallback();
    if (cachedTui !== tui || !cachedRenderers) {
      cachedTui = tui;
      cachedRenderers = createToolPreviewRenderers(tui, toolName, undefined, {
        buildCallPreview: (args) => buildCallSummary(args),
        resolvedLine: (result) => formatResolvedLine((result.details as { resolved?: ResolvedModelInfo } | undefined)?.resolved),
      });
    }
    return cachedRenderers;
  };
  return {
    renderCall: (...args: Parameters<ReturnType<typeof createToolPreviewRenderers>["renderCall"]>) => renderers().renderCall(...args),
    renderResult: (...args: Parameters<ReturnType<typeof createToolPreviewRenderers>["renderResult"]>) => renderers().renderResult(...args),
  };
}

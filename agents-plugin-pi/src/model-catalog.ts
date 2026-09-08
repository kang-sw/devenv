import type { ExtensionContext } from "@earendil-works/pi-coding-agent";

export interface ModelCatalogEntry {
  provider: string;
  id: string;
  hasAuth: boolean;
}

export type TierRejection = {
  model: string;
  resolvedFrom: string;
  /** The raw configured value, present only when backend expansion changed it into `model` (the checked/expanded string). */
  stored?: string;
} & ({ why: "unknown"; suggestions: string[] } | { why: "no-auth" } | { why: "unset" });

export type TierFailure = {
  kind: "transport" | "parse" | "unset" | "unknown" | "no-auth";
  model?: string;
  resolvedFrom?: string;
  catalogEmpty?: boolean;
};

/** Read current runtime membership and configured-auth presence, never cached availability or scoped models. */
export function modelCatalogFromToolCtx(toolCtx: unknown): ModelCatalogEntry[] {
  const registry = (toolCtx as ExtensionContext | undefined)?.modelRegistry;
  if (!registry) return [];
  try {
    const models = registry.getAll();
    if (!Array.isArray(models)) return [];
    return models.map(model => ({ provider: model.provider, id: model.id, hasAuth: registry.hasConfiguredAuth(model) }));
  } catch {
    // Callers that require a configured tier treat this as an empty/unavailable
    // catalog and refuse before allocating a child. Ordinary spawns preserve
    // their historical inherit fallback.
    return [];
  }
}

/** Keep receiver binding and Pi's TUI/RPC-only notification gate. The slash-command pointer is human-only. */
export function tierWarningNotifierFromToolCtx(toolCtx: unknown): ((warning: string) => void) | undefined {
  const ctx = toolCtx as ExtensionContext | undefined;
  return ctx?.hasUI ? warning => ctx.ui.notify(`${warning} See /ws-model-catalog-list for the models usable here.`, "warning") : undefined;
}

function idHalf(value: string): string {
  const separator = value.indexOf("/");
  return separator < 0 ? value : value.slice(separator + 1);
}

/** Levenshtein distance, capped at two edits: conservative typo assistance, not fuzzy model selection. */
function withinTwoEdits(a: string, b: string): boolean {
  if (Math.abs(a.length - b.length) > 2) return false;
  let previous = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    const row = new Array<number>(b.length + 1).fill(3);
    row[0] = i;
    // Only the distance-two diagonal band can produce an accepted result.
    for (let j = Math.max(1, i - 2); j <= Math.min(b.length, i + 2); j++) {
      row[j] = Math.min(row[j - 1] + 1, previous[j] + 1, previous[j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1));
    }
    previous = row;
  }
  return previous[b.length] <= 2;
}

/** Exact id, case-insensitive containment, then distance <= 2. Stable catalog order per rank; no auth filtering. */
export function suggestModels(value: string, catalog: readonly ModelCatalogEntry[]): string[] {
  const id = idHalf(value);
  if (!id) return [];
  const lower = id.toLowerCase();
  const ranks: string[][] = [[], [], []];
  const seen = new Set<string>();
  for (const model of catalog) {
    const candidate = `${model.provider}/${model.id}`;
    if (seen.has(candidate)) continue;
    seen.add(candidate);
    const other = model.id.toLowerCase();
    const rank = model.id === id ? 0 : other && (other.includes(lower) || lower.includes(other)) ? 1 : withinTwoEdits(lower, other) ? 2 : -1;
    if (rank >= 0) ranks[rank].push(candidate);
  }
  return ranks.flat().slice(0, 3);
}

/** JSON escaping plus non-JSON line/control separators; preserve raw rejected detail separately. */
function quoted(value: string): string {
  return JSON.stringify(value).replace(/[\u007f-\u009f\u2028\u2029]/g, char => `\\u${char.charCodeAt(0).toString(16).padStart(4, "0")}`);
}
function oneLine(value: string): string { return quoted(value).slice(1, -1); }

/**
 * Canonical tool/list/advisory rejection line, with no human command pointer
 * and no head — a caller that refuses the spawn prepends its own head
 * (`ws-pi-agent: ws-agent-spawn rejected: ...`); the advisory reuses this
 * line bare. No longer names an inherit fallback (`inheritModel` is unused):
 * inheriting is now the caller's decision, not something every rejection
 * line documents.
 */
export function formatTierWarning(alias: string, rejected: TierRejection, inheritModel: string | undefined, catalogEmpty: boolean): string {
  void inheritModel;
  const hint = ` Set it via config.tune(key: "agents.tier", harness: "pi", value: {tier: ${quoted(alias)}, model: "<provider/id>"}).`;
  if (rejected.why === "unset") {
    return `warning: tier ${oneLine(alias)} is not configured for harness pi (resolved from ${quoted(rejected.resolvedFrom)}, value ${quoted(rejected.model)}).${hint}`;
  }
  const storedNote = rejected.stored !== undefined ? ` (configured as ${quoted(rejected.stored)})` : "";
  const base = `warning: tier ${oneLine(alias)} is set to ${quoted(rejected.model)}${storedNote} for harness pi, `;
  if (rejected.why === "no-auth") {
    return `${base}but provider ${oneLine(rejected.model.slice(0, rejected.model.indexOf("/")))} has no configured auth.${hint}`;
  }
  const tail = catalogEmpty ? " Pi's model catalog is empty." : rejected.suggestions.length
    ? ` Did you mean ${rejected.suggestions.map(oneLine).join(", ")}?` : " No close match in Pi's model catalog.";
  return `${base}which is not a provider/id entry in Pi's model catalog.${tail}${hint}`;
}

/** Exploration fails closed: unlike ordinary workers it must never replace an
 * unavailable cheap tier with the caller's potentially expensive model. */
export function formatExploreTierRefusal(alias: string, failure: TierFailure | undefined, rejected: TierRejection | undefined): string {
  const model = rejected?.model ?? failure?.model;
  const detail = failure?.kind === "no-auth" || rejected?.why === "no-auth"
    ? `provider ${oneLine((model ?? "configured model").split("/")[0]!)} has no configured auth`
    : failure?.catalogEmpty
      ? "Pi's model catalog is empty or unavailable"
      : failure?.kind === "unknown" || rejected?.why === "unknown"
        ? `configured model ${quoted(model ?? "unknown")} is not in Pi's model catalog`
        : failure?.kind === "unset"
          ? `not configured for harness pi (resolved from ${quoted(failure?.resolvedFrom ?? rejected?.resolvedFrom ?? "unknown")}); set it via config.tune(key: "agents.tier", harness: "pi", value: {tier: ${quoted(alias)}, model: "<provider/id>"})`
          : "small resolution failed";
  return `explore refused: tier ${oneLine(alias)} cannot select an authenticated cheap Pi model: ${detail}. Configure agents.tier for harness pi via config.tune or lead-tune.`;
}

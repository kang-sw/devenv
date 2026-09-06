import type { ExtensionContext } from "@earendil-works/pi-coding-agent";

export interface ModelCatalogEntry {
  provider: string;
  id: string;
  hasAuth: boolean;
}

export type TierRejection = {
  model: string;
  resolvedFrom: string;
} & ({ why: "unknown"; suggestions: string[] } | { why: "no-auth" });

/** Read current runtime membership and configured-auth presence, never cached availability or scoped models. */
export function modelCatalogFromToolCtx(toolCtx: unknown): ModelCatalogEntry[] {
  const registry = (toolCtx as ExtensionContext | undefined)?.modelRegistry;
  if (!registry) return [];
  return registry.getAll().map(model => ({ provider: model.provider, id: model.id, hasAuth: registry.hasConfiguredAuth(model) }));
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

/** Canonical tool/list/advisory warning, with no human command pointer. */
export function formatTierWarning(alias: string, rejected: TierRejection, inheritModel: string | undefined, catalogEmpty: boolean): string {
  const base = `warning: tier ${oneLine(alias)} is set to ${quoted(rejected.model)} for harness pi, `;
  const inherited = `inherited ${oneLine(inheritModel ?? "Pi default")}.`;
  if (rejected.why === "no-auth") {
    return `${base}but provider ${oneLine(rejected.model.slice(0, rejected.model.indexOf("/")))} has no configured auth; ${inherited}`;
  }
  const tail = catalogEmpty ? " Pi's model catalog is empty." : rejected.suggestions.length
    ? ` Did you mean ${rejected.suggestions.map(oneLine).join(", ")}?` : " No close match in Pi's model catalog.";
  return `${base}which is not a provider/id entry in Pi's model catalog; ${inherited}${tail}`;
}

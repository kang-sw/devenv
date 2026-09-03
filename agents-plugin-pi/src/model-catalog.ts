/**
 * Adapter-owned model-catalog curation data file: maps ws's canonical
 * spawn/explore tiers (`small` / `medium` / `large` / `xlarge`) to a
 * `provider/id` model string, hand-edited by the user against
 * `model-catalog.json` (package root, sibling to `runtime.json`).
 *
 * Zero Pi model strings live in ws-mcp core (golden rule) — this file and
 * its data-file counterpart are the entire curation surface. `explore` is a
 * role, not a caller-facing tier: it resolves implicitly through `"small"`
 * (see spawner.ts's exploreLeaf call site).
 *
 * NEVER HARD-FAIL: a missing or empty catalog file is the expected default
 * state ("unset"), not an error — readModelCatalog returns `undefined`
 * rather than throwing, mirroring version-check.ts's readRuntimeContract
 * pattern but tolerant instead of pin-and-fail (that check protects a
 * version contract; this one protects an optional curation surface that
 * degrades silently to inherit).
 */

import { readFileSync } from "node:fs";

export type ModelTier = "small" | "medium" | "large" | "xlarge";

export interface ModelCatalogConfig {
  tiers?: Partial<Record<ModelTier, string>>;
  catalog?: Array<{ provider: string; id: string; label?: string }>;
}

/**
 * Reads and parses the model-catalog data file. Returns `undefined` — never
 * throws — when the file is missing, unreadable, or not valid JSON, since
 * "unset" is the expected default state (a fresh checkout ships
 * `model-catalog.json` as `{}`, but even a wholly absent file must degrade
 * the same way). Read fresh on every call by design (no module-level
 * caching) — callers (bridge.ts's workflow_manual hook, spawner.ts's
 * tier-aware resolveModel) re-read on every invocation so a hand-edit to
 * the file applies immediately, mirroring the Go core's per-call recompute
 * cadence for its own bootstrap-staleness advisory
 * (agents-plugin-tool/internal/mcp/bootstrap_alarm.go).
 */
export function readModelCatalog(path: string): ModelCatalogConfig | undefined {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    return undefined;
  }
  try {
    return JSON.parse(raw) as ModelCatalogConfig;
  } catch {
    return undefined;
  }
}

/**
 * Looks up the configured `provider/id` model string for `tier`, or
 * `undefined` when the catalog is unset or that specific tier is unmapped.
 * An unrecognized/unmapped tier is treated as unset — never a validation
 * error, per the "never hard-fail" constraint.
 */
export function resolveTierModel(config: ModelCatalogConfig | undefined, tier: ModelTier): string | undefined {
  return config?.tiers?.[tier];
}

/**
 * `true` when the catalog's `small` tier is unmapped — the trigger
 * condition for the workflow_manual advisory. `explore`/recon resolves
 * implicitly through `small` (see spawner.ts), so an unset `small` tier
 * covers both "no map at all" and "map exists but explore's tier was never
 * curated," matching the ticket's "or at least the explore tier" phrasing.
 */
export function isModelCatalogUnset(config: ModelCatalogConfig | undefined): boolean {
  return !config?.tiers?.small;
}

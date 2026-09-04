/**
 * Adapter-owned model-catalog curation data file: maps a user-chosen alias
 * name (e.g. `"small"`, `"reviewer"`, anything the user names) to a
 * `provider/id` model string, hand-edited by the user against
 * `model-catalog.json` (package root, sibling to `runtime.json`).
 *
 * Zero Pi model strings live in ws-mcp core (golden rule) — this file and
 * its data-file counterpart are the entire curation surface. Phase 1 drops
 * the closed `ModelTier` union in favor of a generic name->`provider/id`
 * alias table: `ws-agent-spawn` carries no `tier` parameter at all (D-A —
 * the lead passes an optional `model_name` looked up here, or omits it to
 * inherit the parent model). `explore` is a role, not a caller-facing
 * alias: it resolves implicitly through the fixed alias key `"small"` (see
 * spawner.ts's exploreLeaf call site) — nothing stops a user from naming an
 * alias `"small"` in this generic table, so explore's cheap-model-by-default
 * behavior is unchanged in practice.
 *
 * NEVER HARD-FAIL: a missing or empty catalog file is the expected default
 * state ("unset"), not an error — readModelCatalog returns `undefined`
 * rather than throwing, mirroring version-check.ts's readRuntimeContract
 * pattern but tolerant instead of pin-and-fail (that check protects a
 * version contract; this one protects an optional curation surface that
 * degrades silently to inherit).
 */

import { readFileSync } from "node:fs";

export interface ModelCatalogConfig {
  aliases?: Record<string, string>;
  catalog?: Array<{ provider: string; id: string; label?: string }>;
}

/**
 * Reads and parses the model-catalog data file. Returns `undefined` — never
 * throws — when the file is missing, unreadable, or not valid JSON, since
 * "unset" is the expected default state (a fresh checkout ships
 * `model-catalog.json` as `{}`, but even a wholly absent file must degrade
 * the same way). Read fresh on every call by design (no module-level
 * caching) — callers (bridge.ts's workflow_manual hook, spawner.ts's
 * alias-aware resolveModel) re-read on every invocation so a hand-edit to
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
 * Looks up the configured `provider/id` model string for alias `name`, or
 * `undefined` when the catalog is unset or that specific alias is unmapped.
 * An unrecognized/unmapped alias is treated as unset — never a validation
 * error, per the "never hard-fail" constraint.
 */
export function resolveAlias(config: ModelCatalogConfig | undefined, name: string): string | undefined {
  return config?.aliases?.[name];
}

/**
 * `true` when the alias table is absent or empty — the trigger condition
 * for the workflow_manual advisory. Re-keyed from the old "`small` tier
 * unset" check to "alias table empty" (D-A): the spawn tool no longer has a
 * closed tier vocabulary to check a specific key against, so the advisory
 * now fires on the coarser "nothing curated at all" condition.
 */
export function isModelCatalogUnset(config: ModelCatalogConfig | undefined): boolean {
  return !config?.aliases || Object.keys(config.aliases).length === 0;
}

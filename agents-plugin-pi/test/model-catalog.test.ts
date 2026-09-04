/**
 * Unit tests for model-catalog.ts: readModelCatalog (missing/malformed file
 * -> undefined, never throws; empty `{}` -> `{}`; populated file -> parsed),
 * resolveAlias, isModelCatalogUnset. Phase 1 reframes the old `tiers`-shaped
 * config to a generic `aliases` map (D-A).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { readModelCatalog, resolveAlias, isModelCatalogUnset, type ModelCatalogConfig } from "../src/model-catalog.ts";

const tmpDir = mkdtempSync(join(tmpdir(), "ws-model-catalog-test-"));
after(() => {
  rmSync(tmpDir, { recursive: true, force: true });
});

function writeCatalog(name: string, contents: string): string {
  const path = join(tmpDir, name);
  writeFileSync(path, contents, "utf8");
  return path;
}

describe("readModelCatalog", () => {
  test("missing file returns undefined (never throws)", () => {
    const path = join(tmpDir, "does-not-exist.json");
    assert.doesNotThrow(() => readModelCatalog(path));
    assert.equal(readModelCatalog(path), undefined);
  });

  test("empty {} file parses to an empty object", () => {
    const path = writeCatalog("empty.json", "{}");
    assert.deepEqual(readModelCatalog(path), {});
  });

  test("populated file parses aliases and catalog", () => {
    const config: ModelCatalogConfig = {
      aliases: { small: "openrouter/cheap-model", large: "openrouter/big-model" },
      catalog: [{ provider: "openrouter", id: "cheap-model", label: "Cheap" }],
    };
    const path = writeCatalog("populated.json", JSON.stringify(config));
    assert.deepEqual(readModelCatalog(path), config);
  });

  test("malformed JSON returns undefined (never throws)", () => {
    const path = writeCatalog("malformed.json", "{not valid json");
    assert.doesNotThrow(() => readModelCatalog(path));
    assert.equal(readModelCatalog(path), undefined);
  });
});

describe("resolveAlias", () => {
  test("returns the mapped model for a configured alias", () => {
    const config: ModelCatalogConfig = { aliases: { small: "openrouter/cheap-model" } };
    assert.equal(resolveAlias(config, "small"), "openrouter/cheap-model");
  });

  test("returns undefined for an unmapped alias", () => {
    const config: ModelCatalogConfig = { aliases: { small: "openrouter/cheap-model" } };
    assert.equal(resolveAlias(config, "large"), undefined);
  });

  test("returns undefined when config is undefined (unset catalog)", () => {
    assert.equal(resolveAlias(undefined, "small"), undefined);
  });

  test("returns undefined when aliases is absent", () => {
    assert.equal(resolveAlias({}, "small"), undefined);
  });

  test("resolves an arbitrary user-chosen alias name, not just the four old tier names", () => {
    const config: ModelCatalogConfig = { aliases: { reviewer: "openrouter/big-model" } };
    assert.equal(resolveAlias(config, "reviewer"), "openrouter/big-model");
  });
});

describe("isModelCatalogUnset", () => {
  test("true when config is undefined", () => {
    assert.equal(isModelCatalogUnset(undefined), true);
  });

  test("true when config is {} (no aliases at all)", () => {
    assert.equal(isModelCatalogUnset({}), true);
  });

  test("true when aliases is present but empty", () => {
    assert.equal(isModelCatalogUnset({ aliases: {} }), true);
  });

  test("false when at least one alias is mapped", () => {
    assert.equal(isModelCatalogUnset({ aliases: { small: "openrouter/cheap-model" } }), false);
  });

  test("false when a non-small alias is mapped (coarser than the old small-only check)", () => {
    assert.equal(isModelCatalogUnset({ aliases: { reviewer: "openrouter/big-model" } }), false);
  });
});

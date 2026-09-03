/**
 * Unit tests for model-catalog.ts: readModelCatalog (missing/malformed file
 * -> undefined, never throws; empty `{}` -> `{}`; populated file -> parsed),
 * resolveTierModel, isModelCatalogUnset.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { readModelCatalog, resolveTierModel, isModelCatalogUnset, type ModelCatalogConfig } from "../src/model-catalog.ts";

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

  test("populated file parses tiers and catalog", () => {
    const config: ModelCatalogConfig = {
      tiers: { small: "openrouter/cheap-model", large: "openrouter/big-model" },
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

describe("resolveTierModel", () => {
  test("returns the mapped model for a configured tier", () => {
    const config: ModelCatalogConfig = { tiers: { small: "openrouter/cheap-model" } };
    assert.equal(resolveTierModel(config, "small"), "openrouter/cheap-model");
  });

  test("returns undefined for an unmapped tier", () => {
    const config: ModelCatalogConfig = { tiers: { small: "openrouter/cheap-model" } };
    assert.equal(resolveTierModel(config, "large"), undefined);
  });

  test("returns undefined when config is undefined (unset catalog)", () => {
    assert.equal(resolveTierModel(undefined, "small"), undefined);
  });

  test("returns undefined when tiers is absent", () => {
    assert.equal(resolveTierModel({}, "small"), undefined);
  });
});

describe("isModelCatalogUnset", () => {
  test("true when config is undefined", () => {
    assert.equal(isModelCatalogUnset(undefined), true);
  });

  test("true when config is {} (no tiers at all)", () => {
    assert.equal(isModelCatalogUnset({}), true);
  });

  test("true when tiers is present but small is unmapped", () => {
    assert.equal(isModelCatalogUnset({ tiers: { large: "openrouter/big-model" } }), true);
  });

  test("false when small is mapped", () => {
    assert.equal(isModelCatalogUnset({ tiers: { small: "openrouter/cheap-model" } }), false);
  });
});

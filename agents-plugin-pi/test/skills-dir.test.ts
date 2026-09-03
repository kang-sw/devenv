import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { join } from "node:path";
import { resolveSkillsDir } from "../src/skills-dir.ts";

describe("resolveSkillsDir", () => {
  const pluginDir = "/pkg/agents-plugin-pi";
  const repoRoot = "/pkg";

  test("prefers package-local skills/ when it exists (installed tarball)", () => {
    const local = join(pluginDir, "skills");
    assert.equal(resolveSkillsDir(pluginDir, repoRoot, (p) => p === local), local);
  });

  test("falls back to repoRoot/agents-plugin/skills when package-local absent (dev -e)", () => {
    assert.equal(
      resolveSkillsDir(pluginDir, repoRoot, () => false),
      join(repoRoot, "agents-plugin", "skills"),
    );
  });
});

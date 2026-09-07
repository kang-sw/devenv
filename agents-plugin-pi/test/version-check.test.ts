/**
 * Unit tests for version-check.ts's assertVersionPin: a matching version
 * passes silently, a mismatch throws synchronously (before any tools get
 * registered — see bridge.ts's ordering, confirmed correct in review).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { join, dirname, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";
import { assertVersionPin, type RuntimeContract } from "../src/version-check.ts";

describe("assertVersionPin", () => {
  test("does not throw when the bundled runtime.json version matches the live server version", () => {
    const runtime: RuntimeContract = { plugin: "ws", plugin_version: "0.43.4" };
    assert.doesNotThrow(() => assertVersionPin(runtime, "0.43.4"));
  });

  test("throws synchronously when the versions mismatch", () => {
    const runtime: RuntimeContract = { plugin: "ws", plugin_version: "0.43.4" };
    assert.throws(
      () => assertVersionPin(runtime, "0.44.0"),
      (err: unknown) => {
        assert.ok(err instanceof Error);
        assert.match(err.message, /0\.43\.4/);
        assert.match(err.message, /0\.44\.0/);
        return true;
      },
    );
  });
});

/**
 * Review relay #1 (Important, correctness): version-check.ts's own header
 * comment declares agents-plugin-pi/runtime.json a "hand-synced,
 * byte-identical copy of agents-plugin/runtime.json" with "no sync tooling
 * ... to keep these in lockstep automatically" — the two files desynced
 * once already (pi's copy was stuck at 0.43.4 / missing config.resolve_agent
 * while agents-plugin/runtime.json had moved to 0.44.4), and nothing in
 * `npm test` caught it, because the other tests in this file feed a runtime
 * object's own `plugin_version` back into `assertVersionPin` rather than
 * reading the bundled file from disk. This test closes that specific gap:
 * it reads both files directly and fails loudly on the next desync.
 */
describe("agents-plugin-pi/runtime.json hand-sync (review relay #1, Important #2)", () => {
  const testDir = dirname(fileURLToPath(import.meta.url));
  const piRuntimePath = join(testDir, "..", "runtime.json");
  const sourceRuntimePath = join(testDir, "..", "..", "agents-plugin", "runtime.json");

  test("is byte-identical to agents-plugin/runtime.json", () => {
    const piRuntime = readFileSync(piRuntimePath, "utf8");
    const sourceRuntime = readFileSync(sourceRuntimePath, "utf8");
    assert.equal(
      piRuntime,
      sourceRuntime,
      "agents-plugin-pi/runtime.json must be re-copied verbatim from agents-plugin/runtime.json whenever the source changes (no shared sync tooling exists yet)",
    );
  });
});

/**
 * 260906-bug-ws-pi-rsrc-mirror-drift: agents-plugin-pi/{rsrc, runtime.json,
 * bin/ws-mcp-launcher.py} are declared byte-identical hand-synced copies of
 * the same-named files under agents-plugin/. The runtime.json case above
 * guards one file; this block widens the guard to the whole rsrc/ tree and
 * the launcher. compareTrees is a pure two-root comparator so the negative
 * cases run against tmpdir fixtures; only the positive case reads the
 * committed trees. It fails on the next desync naming the offending file.
 * There is automated drift detection, not automated sync: the guard fires
 * when this suite runs, so the Pi track owner runs it when syncing develop.
 */
function listFilesRel(root: string): string[] {
  const out: string[] = [];
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile()) out.push(relative(root, full).split(sep).join("/"));
    }
  };
  walk(root);
  return out.sort();
}

// Pure comparator: [] means the mirror is byte-identical to the source and
// carries no file the source lacks; otherwise one sorted message per problem.
function compareTrees(sourceRoot: string, mirrorRoot: string): string[] {
  const problems: string[] = [];
  const mirrorRemaining = new Set(listFilesRel(mirrorRoot));
  for (const rel of listFilesRel(sourceRoot)) {
    if (!mirrorRemaining.delete(rel)) {
      problems.push(`missing in mirror: ${rel}`);
      continue;
    }
    if (!readFileSync(join(sourceRoot, rel)).equals(readFileSync(join(mirrorRoot, rel)))) {
      problems.push(`differs from source: ${rel}`);
    }
  }
  for (const extra of mirrorRemaining) {
    problems.push(`extra in mirror (source lacks it): ${extra}`);
  }
  return problems.sort();
}

describe("agents-plugin-pi hand-synced mirror (260906)", () => {
  const testDir = dirname(fileURLToPath(import.meta.url));
  const sourceRsrc = join(testDir, "..", "..", "agents-plugin", "rsrc");
  const piRsrc = join(testDir, "..", "rsrc");
  const sourceLauncher = join(testDir, "..", "..", "agents-plugin", "bin", "ws-mcp-launcher.py");
  const piLauncher = join(testDir, "..", "bin", "ws-mcp-launcher.py");

  test("rsrc/ is byte-identical to agents-plugin/rsrc/ (whole tree, no extra files)", () => {
    assert.deepEqual(
      compareTrees(sourceRsrc, piRsrc),
      [],
      "agents-plugin-pi/rsrc must be re-copied verbatim from agents-plugin/rsrc; Pi-specific wording goes in .pi.md overlays authored under the source tree, never in a diverging mirror",
    );
  });

  test("bin/ws-mcp-launcher.py is byte-identical to agents-plugin/bin/ws-mcp-launcher.py", () => {
    assert.equal(
      readFileSync(piLauncher, "utf8"),
      readFileSync(sourceLauncher, "utf8"),
      "agents-plugin-pi/bin/ws-mcp-launcher.py must be re-copied verbatim from agents-plugin/bin/ws-mcp-launcher.py",
    );
  });

  test("compareTrees names a drifted file", () => {
    const base = mkdtempSync(join(tmpdir(), "ws-mirror-"));
    try {
      const src = join(base, "src");
      const mir = join(base, "mir");
      mkdirSync(join(src, "d"), { recursive: true });
      mkdirSync(join(mir, "d"), { recursive: true });
      writeFileSync(join(src, "d", "a.md"), "alpha");
      writeFileSync(join(mir, "d", "a.md"), "ALPHA");
      assert.deepEqual(compareTrees(src, mir), ["differs from source: d/a.md"]);
    } finally {
      rmSync(base, { recursive: true, force: true });
    }
  });

  test("compareTrees rejects an extra file in the mirror and a missing one", () => {
    const base = mkdtempSync(join(tmpdir(), "ws-mirror-"));
    try {
      const src = join(base, "src");
      const mir = join(base, "mir");
      mkdirSync(src, { recursive: true });
      mkdirSync(mir, { recursive: true });
      writeFileSync(join(src, "shared.md"), "x");
      writeFileSync(join(mir, "shared.md"), "x");
      writeFileSync(join(mir, "extra.md"), "y");
      writeFileSync(join(src, "only-src.md"), "z");
      assert.deepEqual(compareTrees(src, mir), [
        "extra in mirror (source lacks it): extra.md",
        "missing in mirror: only-src.md",
      ]);
    } finally {
      rmSync(base, { recursive: true, force: true });
    }
  });
});

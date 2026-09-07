import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
import { test } from "node:test";

const srcDir = join(dirname(fileURLToPath(import.meta.url)), "../src");

const registrationCoverage: ReadonlyArray<readonly [string, number]> = [
  ["spawner.ts", 7],
  ["execute-gateway.ts", 5],
  ["fork.ts", 1],
  ["ask.ts", 2],
  ["lead-skills.ts", 1],
  ["goal-loop.ts", 3],
];

test("every ws-owned native registration uses the shared presentation seam", () => {
  for (const [file, expectedCount] of registrationCoverage) {
    const source = readFileSync(join(srcDir, file), "utf8");
    assert.doesNotMatch(source, /pi\.registerTool\(\{/, `${file} bypasses registerWsTool`);
    assert.equal(
      source.match(/registerWsTool\(pi, \{/g)?.length,
      expectedCount,
      `${file} registration inventory drifted; update the shared-seam coverage deliberately`,
    );
  }

  const bridge = readFileSync(join(srcDir, "bridge.ts"), "utf8");
  assert.match(bridge, /registerWsTool\(pi, \{/, "bridged MCP registrations use the same seam");
});

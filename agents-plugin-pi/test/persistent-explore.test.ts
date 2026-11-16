/** Native research execution is retired; its persisted history is still readable and preserved. */
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { parseOrphans, readExecutionSidecarAt, serializeOrphans } from "../src/agent-sidecar.ts";
import { EXPLORE_MODE_TIERS } from "../src/process-role.ts";

for (const mode of [...Object.keys(EXPLORE_MODE_TIERS), "simple"]) {
  test(`legacy researcher ${mode} remains readable but cannot resume`, t => {
    const root = mkdtempSync(join(tmpdir(), "retired-explore-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const history = join(root, "research.jsonl");
    writeFileSync(history, "retained research\n");
    const path = join(root, "registry.json");
    const raw = serializeOrphans([{
      agentId: "legacy-research", sessionPath: history, systemPromptPath: join(root, "guide.md"),
      spawnRole: "explore", toolGroup: mode === "simple" ? "read-only" : "read-only-explore",
      exploreMode: mode as never, modelBase: "pi/small", modelEffort: "low", wsToolNames: [], state: "running",
    }]);
    assert.equal(parseOrphans(raw).length, 1, "legacy research metadata is not unreadable history");
    writeFileSync(path, raw);
    const notices: string[] = [];
    assert.deepEqual(readExecutionSidecarAt(path, text => notices.push(text)), []);
    assert.equal(existsSync(path), false);
    const archive = readdirSync(root).find(name => name.startsWith("registry.json.retired-"))!;
    assert.equal(readFileSync(join(root, archive), "utf8"), raw);
    assert.equal(readFileSync(history, "utf8"), "retained research\n");
    assert.match(notices[0], /NOT resumed or adopted/);
    assert.match(notices[0], /preserved at/);
  });
}

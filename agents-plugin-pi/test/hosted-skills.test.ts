import assert from "node:assert/strict";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { prepareSkillsDir, registerSkillResources, resolveSkillsDir } from "../src/skills-dir.ts";
import { consumeWorkflowOwner, hostedWorkflowRoot } from "../src/workflow-owner.ts";

function snapshot(path: string): unknown {
  const stat = lstatSync(path, { bigint: true });
  return {
    ino: stat.ino, mtime: stat.mtimeNs, ctime: stat.ctimeNs,
    content: stat.isDirectory()
      ? Object.fromEntries(readdirSync(path).sort().map(name => [name, snapshot(join(path, name))]))
      : readFileSync(path, "utf8"),
  };
}

test("hosted source and prepared skill discovery uses selected package without synchronization", async t => {
  const base = mkdtempSync(join(tmpdir(), "ws-hosted-skills-"));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const canonical = join(base, "agents-plugin", "skills", "canonical-only");
  mkdirSync(canonical, { recursive: true });
  writeFileSync(join(canonical, "SKILL.md"), "canonical must not be copied");
  for (const prepared of [false, true]) {
    const pi = join(base, prepared ? "prepared" : "source");
    mkdirSync(join(pi, "skills", "adapter-only"), { recursive: true });
    mkdirSync(join(pi, "rsrc"), { recursive: true });
    writeFileSync(join(pi, "skills", "adapter-only", "SKILL.md"), "adapter");
    writeFileSync(join(pi, "rsrc", "manifest.json"), '{"files":{}}');
    if (prepared) writeFileSync(join(pi, "workflow-owner.json"), '{"plugin":"ws"}');
    for (const owner of ["ws", "wsflow"]) {
      const selected = join(base, `${prepared}-${owner}`);
      const skill = owner === "wsflow" ? "lead-proceed" : "lead-ticket";
      mkdirSync(join(selected, ".claude-plugin"), { recursive: true });
      mkdirSync(join(selected, "skills", skill), { recursive: true });
      mkdirSync(join(selected, "rsrc"), { recursive: true });
      writeFileSync(join(selected, ".claude-plugin", "plugin.json"), JSON.stringify({ name: owner }));
      writeFileSync(join(selected, "skills", skill, "SKILL.md"), `${owner}/playbook.read(name: "selected-only")`);
      const manifest = join(selected, "rsrc", "manifest.json");
      writeFileSync(manifest, '{"files":{"selected-only/selected-only.md":"hash"}}');
      const env = { WS_PI_WORKFLOW_ROOT: selected, WS_HOSTED_SESSION: "1", WS_HOSTED_SESSION_KEY: "key", WS_HOSTED_SESSION_ROW: "row" };
      await consumeWorkflowOwner(pi, env, { pid: 42, readRow: async () => '{"host_pid":42}', sleep: async () => {} });
      assert.equal(hostedWorkflowRoot(), selected);
      assert.equal(env.WS_PI_WORKFLOW_ROOT, undefined);
      const before = snapshot(selected);
      const adapterBefore = snapshot(pi);
      let discover!: () => { skillPaths: string[] };
      registerSkillResources({ on(event, handler) {
        assert.equal(event, "resources_discover");
        discover = handler;
      } }, pi, base, hostedWorkflowRoot());
      for (let reload = 0; reload < 3; reload++) {
        assert.deepEqual(discover(), { skillPaths: [join(selected, "skills")] });
        assert.equal(prepareSkillsDir(pi, base, hostedWorkflowRoot()), join(selected, "skills"));
        assert.equal(resolveSkillsDir(pi, base, undefined, hostedWorkflowRoot()), join(selected, "skills"));
      }
      assert.equal(existsSync(join(selected, "skills", "canonical-only")), false);
      assert.equal(existsSync(join(selected, "skills", "lead-proceed")), owner === "wsflow");
      assert.deepEqual(snapshot(selected), before);
      assert.deepEqual(snapshot(pi), adapterBefore);
      // The adapter manifest must not excuse a missing selected-package target.
      writeFileSync(manifest, '{"files":{}}');
      assert.throws(discover, /selected-only.*absent/);
      assert.throws(() => resolveSkillsDir(pi, base, undefined, hostedWorkflowRoot()), /selected-only.*absent/);
    }
  }
  await consumeWorkflowOwner(base, {});
  assert.equal(hostedWorkflowRoot(), undefined);
});

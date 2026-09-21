import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import {
  prepareSkillsDir,
  registerSkillResources,
  resolveSkillsDir,
  validateGeneratedSkillTargets,
} from "../src/skills-dir.ts";

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

describe("registerSkillResources", () => {
  test("the registered reload callback replaces a renamed shim and exposes the regenerated tree", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-skills-rename-"));
    try {
      const pluginDir = join(root, "agents-plugin-pi");
      const source = join(root, "agents-plugin", "skills");
      const generated = join(pluginDir, "skills");
      mkdirSync(join(source, "lead-ticket"), { recursive: true });
      mkdirSync(join(generated, "lead-write-ticket"), { recursive: true });
      mkdirSync(join(pluginDir, "rsrc"), { recursive: true });
      writeFileSync(join(source, "lead-ticket", "SKILL.md"), 'playbook.read(name: "lead-ticket")');
      writeFileSync(join(generated, "lead-write-ticket", "SKILL.md"), 'playbook.read(name: "lead-write-ticket")');
      writeFileSync(join(pluginDir, "rsrc", "manifest.json"), JSON.stringify({ schema_version: 1, files: { "lead-ticket/lead-ticket.md": "hash" } }));

      let discover: (() => { skillPaths: string[] }) | undefined;
      registerSkillResources({
        on(event, handler) {
          assert.equal(event, "resources_discover");
          discover = handler;
        },
      }, pluginDir, root);

      assert.ok(discover);
      assert.deepEqual(discover(), { skillPaths: [generated] });
      assert.equal(existsSync(join(generated, "lead-write-ticket")), false);
      assert.equal(readFileSync(join(generated, "lead-ticket", "SKILL.md"), "utf8"), 'playbook.read(name: "lead-ticket")');
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

describe("prepareSkillsDir", () => {
  test("reload regeneration removes stale extra files outside renamed entries", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-skills-extra-"));
    try {
      const pluginDir = join(root, "agents-plugin-pi");
      const source = join(root, "agents-plugin", "skills");
      const generated = join(pluginDir, "skills");
      mkdirSync(join(source, "mcp-server-repair"), { recursive: true });
      mkdirSync(join(generated, "mcp-server-repair"), { recursive: true });
      writeFileSync(join(source, "mcp-server-repair", "SKILL.md"), "No playbook target.");
      writeFileSync(join(generated, "mcp-server-repair", "SKILL.md"), "old");
      writeFileSync(join(generated, "stale-extra.md"), "must disappear");

      assert.equal(prepareSkillsDir(pluginDir, root), generated);
      assert.equal(existsSync(join(generated, "stale-extra.md")), false);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("an installed package with no canonical sibling exposes its valid carried generated tree", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-skills-installed-valid-"));
    try {
      const pluginDir = join(root, "agents-plugin-pi");
      const generated = join(pluginDir, "skills");
      mkdirSync(join(generated, "lead-ticket"), { recursive: true });
      mkdirSync(join(pluginDir, "rsrc"), { recursive: true });
      writeFileSync(join(generated, "lead-ticket", "SKILL.md"), 'playbook.read(name: "lead-ticket")');
      writeFileSync(join(pluginDir, "rsrc", "manifest.json"), JSON.stringify({ schema_version: 1, files: { "lead-ticket/lead-ticket.md": "hash" } }));

      assert.equal(prepareSkillsDir(pluginDir, root), generated);
      assert.equal(readFileSync(join(generated, "lead-ticket", "SKILL.md"), "utf8"), 'playbook.read(name: "lead-ticket")');
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("an installed package with no canonical sibling rejects an invalid carried generated tree", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-skills-installed-invalid-"));
    try {
      const pluginDir = join(root, "agents-plugin-pi");
      mkdirSync(join(pluginDir, "skills", "lead-write-ticket"), { recursive: true });
      mkdirSync(join(pluginDir, "rsrc"), { recursive: true });
      writeFileSync(join(pluginDir, "skills", "lead-write-ticket", "SKILL.md"), 'playbook.read(name: "lead-write-ticket")');
      writeFileSync(join(pluginDir, "rsrc", "manifest.json"), JSON.stringify({ schema_version: 1, files: { "lead-ticket/lead-ticket.md": "hash" } }));

      assert.throws(() => prepareSkillsDir(pluginDir, root), /lead-write-ticket.*absent from/);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

describe("copy-skills entrypoint", () => {
  test("pack-time execution cleanly replaces stale generated shims and validates the result", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-copy-skills-"));
    try {
      const sourcePackage = join(dirname(fileURLToPath(import.meta.url)), "..");
      const pluginDir = join(root, "agents-plugin-pi");
      const canonical = join(root, "agents-plugin", "skills", "lead-ticket");
      const generated = join(pluginDir, "skills");
      mkdirSync(join(pluginDir, "scripts"), { recursive: true });
      mkdirSync(join(pluginDir, "src"), { recursive: true });
      mkdirSync(join(pluginDir, "rsrc"), { recursive: true });
      mkdirSync(canonical, { recursive: true });
      mkdirSync(join(generated, "lead-write-ticket"), { recursive: true });
      copyFileSync(join(sourcePackage, "scripts", "copy-skills.mjs"), join(pluginDir, "scripts", "copy-skills.mjs"));
      copyFileSync(join(sourcePackage, "src", "skills-dir.ts"), join(pluginDir, "src", "skills-dir.ts"));
      writeFileSync(join(canonical, "SKILL.md"), 'playbook.read(name: "lead-ticket")');
      writeFileSync(join(generated, "lead-write-ticket", "SKILL.md"), 'playbook.read(name: "lead-write-ticket")');
      writeFileSync(join(generated, "stale-extra.md"), "must disappear");
      writeFileSync(join(pluginDir, "rsrc", "manifest.json"), JSON.stringify({ schema_version: 1, files: { "lead-ticket/lead-ticket.md": "hash" } }));

      const result = spawnSync(process.execPath, [join(pluginDir, "scripts", "copy-skills.mjs")], { encoding: "utf8" });
      assert.equal(result.status, 0, result.stderr || result.stdout);
      assert.match(result.stdout, /synchronized .*validated playbook targets/s);
      assert.equal(existsSync(join(generated, "lead-write-ticket")), false);
      assert.equal(existsSync(join(generated, "stale-extra.md")), false);
      assert.equal(readFileSync(join(generated, "lead-ticket", "SKILL.md"), "utf8"), 'playbook.read(name: "lead-ticket")');
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

describe("validateGeneratedSkillTargets", () => {
  test("fails loudly when a generated shim targets a playbook absent from the rsrc manifest", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-skills-target-"));
    try {
      const skills = join(root, "skills");
      const manifest = join(root, "rsrc", "manifest.json");
      mkdirSync(join(skills, "lead-write-ticket"), { recursive: true });
      mkdirSync(join(root, "rsrc"), { recursive: true });
      writeFileSync(join(skills, "lead-write-ticket", "SKILL.md"), 'Call ws/playbook.read(name: "lead-write-ticket").');
      writeFileSync(manifest, JSON.stringify({ schema_version: 1, files: { "lead-ticket/lead-ticket.md": "hash" } }));

      assert.throws(
        () => validateGeneratedSkillTargets(skills, manifest),
        /lead-write-ticket\/SKILL\.md.*lead-write-ticket.*absent from.*manifest\.json/,
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

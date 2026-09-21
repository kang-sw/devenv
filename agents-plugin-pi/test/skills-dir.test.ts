import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { chmodSync, copyFileSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, readlinkSync, renameSync, rmSync, symlinkSync, utimesSync, writeFileSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";
import {
  prepareSkillsDir,
  registerSkillResources,
  resolveSkillsDir,
  syncGeneratedSkillsDir,
  validateGeneratedSkillTargets,
} from "../src/skills-dir.ts";

const moduleURL = new URL("../src/skills-dir.ts", import.meta.url).href;
const markerName = ".ws-skills-hash";

// Isolated processes keep patched builtin exports out of the test runner. Guard
// calls, not timestamps: a destructive same-content rewrite must fail too.
const mutationGuard = `
  import fs from 'node:fs';
  import { syncBuiltinESMExports } from 'node:module';
  import assert from 'node:assert/strict';
  for (const name of ['appendFileSync', 'chmodSync', 'chownSync', 'copyFileSync',
    'cpSync', 'fchmodSync', 'fchownSync', 'fdatasyncSync', 'fsyncSync',
    'ftruncateSync', 'futimesSync', 'lchmodSync', 'lchownSync', 'linkSync',
    'lutimesSync', 'mkdirSync', 'mkdtempSync', 'renameSync', 'rmSync',
    'rmdirSync', 'symlinkSync', 'truncateSync', 'unlinkSync', 'utimesSync',
    'writeFileSync', 'writeSync', 'writevSync', 'createWriteStream']) {
    if (typeof fs[name] === 'function') fs[name] = () => { throw new Error('unexpected mutation: ' + name); };
  }
  const open = fs.openSync;
  fs.openSync = (path, flags, ...rest) => {
    assert.ok(flags === 'r' || flags === fs.constants.O_RDONLY, 'write-capable open');
    return open(path, flags, ...rest);
  };
  syncBuiltinESMExports();
`;

function runIsolated(script: string): void {
  const result = spawnSync(process.execPath, ["--input-type=module", "-e", script], {
    encoding: "utf8", timeout: 15_000,
  });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr || result.stdout);
}

function discoveryScript(pluginDir: string, root: string): string {
  return `
    const { registerSkillResources } = await import(${JSON.stringify(moduleURL)});
    for (const role of ['lead', 'worker', 'nested-child']) {
      let discover;
      registerSkillResources({ on(event, handler) {
        assert.equal(event, 'resources_discover'); discover = handler;
      } }, ${JSON.stringify(pluginDir)}, ${JSON.stringify(root)});
      for (let reload = 0; reload < 20; reload++) {
        assert.deepEqual(discover(), { skillPaths: [${JSON.stringify(join(pluginDir, "skills"))}] }, role);
      }
    }
  `;
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-skills-hash-"));
  const pluginDir = join(root, "agents-plugin-pi");
  const source = join(root, "agents-plugin", "skills");
  const generated = join(pluginDir, "skills");
  const manifest = join(pluginDir, "rsrc", "manifest.json");
  mkdirSync(join(source, "lead-ticket", "nested", "empty"), { recursive: true });
  mkdirSync(dirname(manifest), { recursive: true });
  writeFileSync(join(source, "lead-ticket", "SKILL.md"), 'playbook.read(name: "lead-ticket")');
  writeFileSync(join(source, "lead-ticket", "nested", "keep.txt"), "retain identity");
  writeFileSync(manifest, JSON.stringify({ files: { "lead-ticket/lead-ticket.md": "hash" } }));
  const sync = () => syncGeneratedSkillsDir(source, generated, manifest);
  const marker = () => readFileSync(join(generated, markerName), "utf8");
  assert.equal(sync(), true);
  assert.match(marker(), /^ws-skills-v1:sha256:[a-f0-9]{64}\n$/);
  return { root, pluginDir, source, generated, manifest, sync, marker };
}

function tree(path: string): unknown {
  const stat = lstatSync(path);
  if (stat.isSymbolicLink()) return { link: readlinkSync(path) };
  if (stat.isFile()) return { bytes: readFileSync(path).toString("hex"), mode: stat.mode & 0o777 };
  return Object.fromEntries(readdirSync(path).sort().filter(name => name !== markerName)
    .map(name => [name, tree(join(path, name))]));
}

function identity(path: string) {
  const { dev, ino, mtimeNs, ctimeNs } = lstatSync(path, { bigint: true });
  return { dev, ino, mtimeNs, ctimeNs };
}

describe("hash-gated skill synchronization", () => {
  test("lead, worker and nested discovery callbacks perform zero mutation calls on a hash hit", () => {
    const f = fixture();
    try {
      runIsolated(mutationGuard + discoveryScript(f.pluginDir, f.root));
    } finally { rmSync(f.root, { recursive: true, force: true }); }
  });

  test("concurrent real discovery processes share an already synchronized tree without writes", async () => {
    const f = fixture();
    const children: ReturnType<typeof spawn>[] = [];
    try {
      const runs = Array.from({ length: 6 }, () => {
        const child = spawn(process.execPath, ["--input-type=module", "-e", mutationGuard + `
          process.stdout.write('ready\\n');
          await new Promise(resolve => process.stdin.once('data', resolve));
          process.stdin.destroy();
        ` + discoveryScript(f.pluginDir, f.root)], { stdio: ["pipe", "pipe", "pipe"], timeout: 15_000 });
        children.push(child);
        let output = "";
        const ready = new Promise<void>((resolve, reject) => {
          child.stdout!.on("data", data => { if (String(data).includes("ready")) resolve(); });
          child.once("error", reject);
          child.once("close", () => reject(new Error("discovery exited before barrier")));
        });
        const done = new Promise<void>((resolve, reject) => {
          child.stderr!.on("data", data => { output += data; });
          child.once("error", reject);
          child.once("close", (code, signal) => {
            if (code === 0) resolve();
            else reject(new Error(`discovery exited ${code}/${signal}: ${output}`));
          });
        });
        return { ready, done };
      });
      const barrier = Promise.all(runs.map(run => run.ready)).then(() => {
        for (const child of children) child.stdin!.end("discover\n");
      });
      await Promise.all([barrier, ...runs.map(run => run.done)]);
      assert.deepEqual(tree(f.generated), tree(f.source));
    } finally {
      for (const child of children) child.kill();
      rmSync(f.root, { recursive: true, force: true });
    }
  });

  test("content, names, types, modes and link targets invalidate the marker and exactly mirror nested entries", () => {
    const f = fixture();
    try {
      const keep = join(f.generated, "lead-ticket", "nested", "keep.txt");
      const keepIdentity = identity(keep);
      const dirIdentity = lstatSync(join(f.generated, "lead-ticket")).ino;
      let previous = f.marker();
      const check = () => {
        assert.equal(f.sync(), true);
        assert.notEqual(f.marker(), previous);
        previous = f.marker();
        assert.deepEqual(tree(f.generated), tree(f.source));
        assert.deepEqual(identity(keep), keepIdentity);
        assert.equal(lstatSync(join(f.generated, "lead-ticket")).ino, dirIdentity);
      };
      const shim = join(f.source, "lead-ticket", "SKILL.md");
      const fixedTime = new Date("2020-01-01T00:00:00Z");
      utimesSync(shim, fixedTime, fixedTime);
      const original = lstatSync(shim, { bigint: true });
      writeFileSync(shim, 'playbook.read(name: "lead-ticket")\nchanged');
      utimesSync(shim, fixedTime, fixedTime);
      assert.equal(lstatSync(shim, { bigint: true }).mtimeNs, original.mtimeNs);
      check();
      const added = join(f.source, "lead-ticket", "nested", "added");
      writeFileSync(added, "new bytes"); check();
      renameSync(added, added + "-renamed"); check();
      chmodSync(added + "-renamed", 0o755); check();
      rmSync(added + "-renamed"); check();
      writeFileSync(added, "file before directory"); check();
      rmSync(added); mkdirSync(added); writeFileSync(join(added, "child"), "nested"); check();
      rmSync(added, { recursive: true }); symlinkSync("keep.txt", added); check();
      rmSync(added); symlinkSync("../SKILL.md", added); check();
      rmSync(added); writeFileSync(added, "file after link"); check();
      // Touch-only source changes are read-only, even after all mismatches above.
      utimesSync(shim, new Date(), new Date());
      runIsolated(mutationGuard + discoveryScript(f.pluginDir, f.root));
    } finally { rmSync(f.root, { recursive: true, force: true }); }
  });

  test("an empty canonical root removes every generated skill and retains only its marker", () => {
    const f = fixture();
    try {
      const previous = f.marker();
      const rootInode = lstatSync(f.generated).ino;
      rmSync(join(f.source, "lead-ticket"), { recursive: true });
      assert.equal(f.sync(), true);
      assert.deepEqual(readdirSync(f.generated), [markerName]);
      assert.match(f.marker(), /^ws-skills-v1:sha256:[a-f0-9]{64}\n$/);
      assert.notEqual(f.marker(), previous);
      assert.equal(lstatSync(f.generated).ino, rootInode);
      assert.deepEqual(tree(f.generated), tree(f.source));
      runIsolated(mutationGuard + discoveryScript(f.pluginDir, f.root));
    } finally { rmSync(f.root, { recursive: true, force: true }); }
  });

  test("matching source marker cannot hide generated drift; missing and corrupt markers recover", () => {
    const f = fixture();
    try {
      const expected = f.marker();
      writeFileSync(join(f.generated, "lead-ticket", "SKILL.md"), "generated drift");
      writeFileSync(join(f.generated, "extra"), "stale");
      rmSync(join(f.generated, "lead-ticket", "nested"), { recursive: true });
      assert.equal(f.marker(), expected);
      f.sync();
      assert.deepEqual(tree(f.generated), tree(f.source));
      assert.equal(f.marker(), expected);
      const keep = identity(join(f.generated, "lead-ticket", "SKILL.md"));
      for (const body of [null, "corrupt\n"]) {
        rmSync(join(f.generated, markerName));
        if (body !== null) writeFileSync(join(f.generated, markerName), body);
        f.sync();
        assert.equal(f.marker(), expected);
        assert.deepEqual(identity(join(f.generated, "lead-ticket", "SKILL.md")), keep);
      }
    } finally { rmSync(f.root, { recursive: true, force: true }); }
  });

  test("validation failure leaves the old marker unpublished, including manifest invalidation on hash hits", () => {
    const f = fixture();
    try {
      const previous = f.marker();
      writeFileSync(f.manifest, JSON.stringify({ files: {} }));
      runIsolated(mutationGuard + `
        const { prepareSkillsDir } = await import(${JSON.stringify(moduleURL)});
        assert.throws(() => prepareSkillsDir(${JSON.stringify(f.pluginDir)}, ${JSON.stringify(f.root)}), /lead-ticket.*absent from/);
      `);
      assert.equal(f.marker(), previous);
      writeFileSync(join(f.source, "lead-ticket", "SKILL.md"), 'playbook.read(name: "missing")');
      assert.throws(f.sync, /missing.*absent from/);
      assert.equal(f.marker(), previous);
      rmSync(join(f.generated, markerName));
      assert.throws(f.sync, /missing.*absent from/);
      assert.equal(existsSync(join(f.generated, markerName)), false);
    } finally { rmSync(f.root, { recursive: true, force: true }); }
  });

  test("actual copy and marker-publication I/O failures never publish a new success marker", () => {
    const f = fixture();
    try {
      const previous = f.marker();
      const destination = join(f.generated, "lead-ticket", "SKILL.md");
      writeFileSync(join(f.source, "lead-ticket", "SKILL.md"), 'playbook.read(name: "lead-ticket")\nnew bytes');
      for (const faultPath of [destination, join(f.generated, markerName)]) {
        runIsolated(`
          import fs from 'node:fs';
          import assert from 'node:assert/strict';
          import { syncBuiltinESMExports } from 'node:module';
          const rename = fs.renameSync;
          let injected = false;
          fs.renameSync = (from, to) => {
            if (to === ${JSON.stringify(faultPath)}) {
              injected = true;
              throw Object.assign(new Error('injected EIO'), { code: 'EIO' });
            }
            return rename(from, to);
          };
          syncBuiltinESMExports();
          const { syncGeneratedSkillsDir } = await import(${JSON.stringify(moduleURL)});
          assert.throws(() => syncGeneratedSkillsDir(${JSON.stringify(f.source)}, ${JSON.stringify(f.generated)}, ${JSON.stringify(f.manifest)}), { code: 'EIO' });
          assert.equal(injected, true);
        `);
        assert.equal(f.marker(), previous);
        assert.equal(readdirSync(f.generated).some(name => name.endsWith(".tmp")), false);
        assert.equal(readdirSync(dirname(destination)).some(name => name.endsWith(".tmp")), false);
      }
      f.sync();
      assert.notEqual(f.marker(), previous);
      assert.deepEqual(tree(f.generated), tree(f.source));
    } finally { rmSync(f.root, { recursive: true, force: true }); }
  });
});

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

      assert.equal(syncGeneratedSkillsDir(join(root, "agents-plugin", "skills"), generated, join(pluginDir, "rsrc", "manifest.json")), false);
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
      const marker = readFileSync(join(generated, markerName), "utf8");
      assert.match(marker, /^ws-skills-v1:sha256:[a-f0-9]{64}\n$/);
      const packURL = pathToFileURL(join(pluginDir, "scripts", "copy-skills.mjs")).href;
      runIsolated(mutationGuard + `await import(${JSON.stringify(packURL)});`);
      runIsolated(mutationGuard + discoveryScript(pluginDir, root));
      assert.equal(readFileSync(join(generated, markerName), "utf8"), marker);
      writeFileSync(join(canonical, "SKILL.md"), 'playbook.read(name: "lead-ticket")\npack refresh');
      runIsolated(`await import(${JSON.stringify(packURL)});`);
      assert.notEqual(readFileSync(join(generated, markerName), "utf8"), marker);
      runIsolated(mutationGuard + discoveryScript(pluginDir, root));
      writeFileSync(join(canonical, "SKILL.md"), 'playbook.read(name: "lead-ticket")');
      prepareSkillsDir(pluginDir, root);
      assert.equal(readFileSync(join(generated, markerName), "utf8"), marker);
      runIsolated(mutationGuard + `await import(${JSON.stringify(packURL)});`);
      // The carried package remains read-only even without the canonical sibling.
      rmSync(join(root, "agents-plugin"), { recursive: true });
      runIsolated(mutationGuard + `await import(${JSON.stringify(packURL)});`);
      runIsolated(mutationGuard + discoveryScript(pluginDir, root));
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

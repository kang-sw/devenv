#!/usr/bin/env node
// Pack-time copy: mirror the canonical ws skills tree into this package so the
// published/installed tarball carries it. Generated dir is gitignored, never
// committed. Source is the sibling monorepo tree; when packing from outside the
// monorepo (already-vendored tarball), the source is absent and we no-op.
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { syncGeneratedSkillsDir, validateGeneratedSkillTargets } from "../src/skills-dir.ts";

const pkgDir = dirname(dirname(fileURLToPath(import.meta.url))); // agents-plugin-pi/
const src = join(dirname(pkgDir), "agents-plugin", "skills");
const dest = join(pkgDir, "skills");

if (syncGeneratedSkillsDir(src, dest, join(pkgDir, "rsrc", "manifest.json"))) {
  console.log(`[copy-skills] synchronized ${src} -> ${dest}`);
} else {
  console.warn(`[copy-skills] source ${src} absent; leaving ${dest} as-is`);
}

if (existsSync(dest)) {
  validateGeneratedSkillTargets(dest, join(pkgDir, "rsrc", "manifest.json"));
  console.log(`[copy-skills] validated playbook targets against ${join(pkgDir, "rsrc", "manifest.json")}`);
}

#!/usr/bin/env node
// Pack-time copy: mirror the canonical ws skills tree into this package so the
// published/installed tarball carries it. Generated dir is gitignored, never
// committed. Source is the sibling monorepo tree; when packing from outside the
// monorepo (already-vendored tarball), the source is absent and we no-op.
import { cpSync, existsSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const pkgDir = dirname(dirname(fileURLToPath(import.meta.url))); // agents-plugin-pi/
const src = join(dirname(pkgDir), "agents-plugin", "skills");
const dest = join(pkgDir, "skills");

if (!existsSync(src)) {
  console.warn(`[copy-skills] source ${src} absent; leaving ${dest} as-is`);
  process.exit(0);
}
rmSync(dest, { recursive: true, force: true });
cpSync(src, dest, { recursive: true });
console.log(`[copy-skills] copied ${src} -> ${dest}`);

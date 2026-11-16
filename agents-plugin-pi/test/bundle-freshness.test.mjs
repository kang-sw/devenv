import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const pkg = dirname(dirname(fileURLToPath(import.meta.url)));

// Mirror piBundleInputHash in internal/wsagent/pi_bundle.go, independently of
// the builder: sorted slash paths, then "<path>\n<sha256 of raw file bytes>\n".
function sourceFiles(dir) {
  return readdirSync(join(pkg, dir), { withFileTypes: true }).flatMap(entry => {
    const rel = `${dir}/${entry.name}`;
    if (entry.isDirectory()) return sourceFiles(rel);
    return entry.isFile() && (entry.name.endsWith(".ts") || entry.name.endsWith(".mjs")) ? [rel] : [];
  });
}

test("committed Pi bundle matches the launcher's current input hash", () => {
  const inputs = [...sourceFiles("src"), "src/eff_large_wordlist.txt", "package-lock.json", "scripts/bundle.mjs"].sort();
  const digest = createHash("sha256");
  for (const rel of inputs) {
    const hash = createHash("sha256").update(readFileSync(join(pkg, rel))).digest("hex");
    digest.update(`${rel}\n${hash}\n`);
  }
  const current = digest.digest("hex");
  const header = readFileSync(join(pkg, "dist/index.js"), "utf8").split("\n", 1)[0].replace(/\r$/, "");
  const recorded = header.match(/^\/\/ ws-pi-bundle-inputs: sha256-([a-f0-9]{64})$/);
  assert.ok(recorded, "dist/index.js must start with its input-hash header");
  assert.equal(recorded[1], current, "dist/index.js is stale; run npm ci && npm run bundle, then commit dist/");
  console.log(`Pi bundle input hash: ${current}`);
});

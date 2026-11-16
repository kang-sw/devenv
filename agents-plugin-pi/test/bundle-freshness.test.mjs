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
  const notices = readFileSync(join(pkg, "dist/THIRD_PARTY_NOTICES"));
  for (const product of ["agents-plugin", "agents-plugin-wsflow"]) {
    const native = join(pkg, "..", product, "native-pi");
    const manifest = JSON.parse(readFileSync(join(native, "first-party-manifest.json"), "utf8"));
    assert.equal(manifest.bundle_input_hash, current, `${product} manifest is stale`);
    const nativeHeader = readFileSync(join(native, "first-party/dist/index.js"), "utf8").split("\n", 1)[0].replace(/\r$/, "");
    assert.equal(nativeHeader, header, `${product} bundle is stale`);
    const inputs = JSON.parse(readFileSync(join(native, "first-party/bundle-inputs.json"), "utf8"));
    assert.equal(inputs.sha256, current, `${product} bundle-inputs is stale`);
    assert.deepEqual(readFileSync(join(native, "first-party/dist/THIRD_PARTY_NOTICES")), notices, `${product} notices differ`);
    assert.equal(manifest.files["dist/THIRD_PARTY_NOTICES"], createHash("sha256").update(notices).digest("hex"));
  }
  const text = notices.toString("utf8");
  assert.match(text, /htmlparser2@10\.1\.0 \(MIT\)[\s\S]*?Copyright 2010, 2011, Chris Winberry/);
  assert.match(text, /domelementtype@2\.3\.0 \(BSD-2-Clause\)[\s\S]*?Copyright \(c\) Felix Böhm/);
  assert.match(text, /boolbase@1\.0\.0 \(ISC\)[\s\S]*?Copyright \(c\) 2014-2015, Felix Boehm/);
  console.log(`Pi bundle input hash: ${current}`);
});

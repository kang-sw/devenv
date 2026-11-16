// Package-local forwarding factory. Check the existing bundle contract before
// evaluating the original module; a stale factory is reported by Pi's watchdog.
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

function assertFresh(original) {
  const entry = fileURLToPath(original), root = dirname(dirname(entry));
  const header = readFileSync(entry, "utf8").split("\n", 1)[0].replace(/\r$/, "");
  const recorded = /^\/\/ ws-pi-bundle-inputs: sha256-([a-f0-9]{64})$/.exec(header)?.[1];
  if (!recorded) throw new Error("bundle input header is missing or invalid");
  let expected;
  if (existsSync(join(root, "src/index.ts"))) {
    function sources(dir) {
      return readdirSync(join(root, dir), { withFileTypes: true }).flatMap(entry => {
        const rel = `${dir}/${entry.name}`;
        if (entry.isDirectory()) return sources(rel);
        return entry.isFile() && /\.(ts|mjs)$/.test(entry.name) ? [rel] : [];
      });
    }
    // Same existing raw-byte input contract as bundle.mjs/piBundleInputHash.
    const inputs = [...sources("src"), "src/eff_large_wordlist.txt", "package-lock.json", "scripts/bundle.mjs"].sort();
    const digest = createHash("sha256");
    for (const name of inputs) {
      let bytes;
      try {
        bytes = readFileSync(join(root, name));
      } catch (error) {
        // npm omits the root lock; its producer-retained raw bytes preserve
        // the same canonical hash row without trusting a stale source record.
        if (name !== "package-lock.json" || error.code !== "ENOENT") throw error;
        bytes = readFileSync(join(root, "bundle-inputs/lock.json"));
      }
      const hash = createHash("sha256").update(bytes).digest("hex");
      digest.update(`${name}\n${hash}\n`);
    }
    expected = digest.digest("hex");
  } else {
    // Prepared native payloads intentionally omit source/lock/build inputs.
    const record = JSON.parse(readFileSync(join(root, "bundle-inputs.json"), "utf8"));
    if (record.schema_version !== 1) throw new Error("invalid prepared bundle input record");
    expected = record.sha256;
  }
  if (typeof expected !== "string" || !/^[a-f0-9]{64}$/.test(expected) || recorded !== expected) {
    throw new Error("bundle is stale (its inputs changed without a rebuild)");
  }
}

export function createPiForwarder(original) {
  return async function (...args) {
    try {
      assertFresh(original);
    } catch (cause) {
      throw new Error(`[ws Pi] EXTENSION NOT LOADED: ${cause.message}; rebuild/reinstall the bundled extension.`, { cause });
    }
    // No static original import: failed validation must prevent module effects,
    // while a valid factory keeps the actual module URL and host SDK graph.
    const extension = await import(original);
    return Reflect.apply(extension.default, this, args);
  };
}

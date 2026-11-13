// Loaded before Pi's CLI, in the same process and module graph. Do not resolve
// settings in Go or a probe process: Pi's two-pass loader owns project trust,
// package filters, discovery, ordering, initialization caching and /reload.
import { existsSync, readFileSync, realpathSync, statSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const wsNames = new Set(["ws", "wsflow", "ws-pi-bridge", "wsflow-pi-bridge"]);
function manifest(dir) {
  try { return JSON.parse(readFileSync(join(dir, "package.json"), "utf8")); }
  catch { return undefined; } // Match Pi's tolerance of absent/invalid manifests.
}
function canonical(path) {
  try { return realpathSync(path); } catch { return resolve(path); }
}
function contains(root, path) {
  const rel = relative(root, path);
  return rel === "" || (rel !== ".." && !rel.startsWith(".." + sep) && !isAbsolute(rel));
}
export function isWS(resource) {
  if (resource.path.startsWith("builtin:")) return false;
  const packageRoot = resource.metadata?.packageRoot;
  if (packageRoot && wsNames.has(manifest(packageRoot)?.name)) return true;
  // Explicit file entries and auto-discovered symlinks need not have package
  // metadata. Match the package's declared entry, not a path substring (and
  // don't mistake an unrelated .pi extension inside the ws checkout for ws).
  const path = canonical(resource.path);
  for (let dir = dirname(path); ; dir = dirname(dir)) {
    const pkg = manifest(dir);
    if (pkg && wsNames.has(pkg.name)) {
      for (const entry of pkg.pi?.extensions ?? []) {
        const declared = canonical(resolve(dir, entry));
        if (declared === path) return true;
        try { if (statSync(declared).isDirectory() && contains(declared, path)) return true; }
        catch { /* A missing declared entry cannot own this resource. */ }
      }
    }
    if (dirname(dir) === dir) return false;
  }
}

export function installResolver(DefaultPackageManager, pathKey) {
  const proto = DefaultPackageManager?.prototype;
  if (typeof proto?.resolve !== "function" || typeof proto?.resolveExtensionSources !== "function" || typeof pathKey !== "function") {
    throw new Error("Pi's JavaScript SDK has no compatible extension resolver; update Pi or select isolated mode");
  }
  const nativeResolve = proto.resolve;
  const nativeCLI = proto.resolveExtensionSources;
  const enabled = new WeakMap();
  proto.resolve = async function (...args) {
    const result = await nativeResolve.apply(this, args);
    enabled.set(this, result.extensions.filter((r) => r.enabled && !isWS(r)));
    return result;
  };
  proto.resolveExtensionSources = async function (...args) {
    const result = await nativeCLI.apply(this, args);
    // --no-extensions still suppresses ambient paths. Re-add only the native
    // resolver's enabled non-ws set, at each trust pass (and every reload).
    // Keep original metadata and native realpath/case deduplication.
    const seen = new Set(result.extensions.map((r) => pathKey(r.path)));
    for (const resource of enabled.get(this) ?? []) {
      const key = pathKey(resource.path);
      if (!seen.has(key)) {
        seen.add(key);
        result.extensions.push(resource);
      }
    }
    return result;
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [sdk, cli, ...args] = process.argv.slice(2);
  const sdkURL = pathToFileURL(sdk);
  const { DefaultPackageManager } = await import(sdkURL.href);
  // Bundled SDK/CLI share resolver classes in chunks. The pure path-key
  // helper is only published in the unbundled tree; it owns no class state.
  const localKey = new URL("./core/source-info.js", sdkURL);
  const keyURL = existsSync(fileURLToPath(localKey)) ? localKey : new URL("../core/source-info.js", sdkURL);
  const { getResourcePathKey } = await import(keyURL.href);
  installResolver(DefaultPackageManager, getResourcePathKey);
  process.argv = [process.execPath, cli, ...args];
  await import(pathToFileURL(cli).href);
}

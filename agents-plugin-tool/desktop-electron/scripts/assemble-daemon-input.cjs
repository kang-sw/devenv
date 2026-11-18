'use strict';
/*
 * Assemble repo-local, platform-specific input for the offline desktop builder.
 * From the repo root (Node + Python 3 required; no downloads/builds/daemon runs):
 *   node agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs \
 *     --binary /absolute/path/ws-mcp-darwin-arm64 --out /absolute/path/daemon-input \
 *     --platform darwin [--package wsflow|ws] [--metadata /path/to/metadata.json]
 * Windows runners: --platform win32 --binary C:\\artifacts\\ws-mcp-windows-amd64.exe
 *   --out C:\\artifacts\\daemon-input (use PowerShell's line continuation, not \\).
 * Then set WERKSCHAFT_DAEMON_INPUT to the printed absolute output path and run
 *   node agents-plugin-tool/desktop-electron/scripts/build.cjs
 * The output directory must not exist. Package defaults to wsflow (owner 1210).
 * Metadata defaults to <binary>.metadata.json; obtain it with the built release
 * asset, or stamp at build time using
 * agents-plugin-tool/scripts/write-runtime-metadata.py with the SAME
 * --version/--source-commit as the binary's build ldflags. Never infer
 * the identity of an arbitrary binary by stamping it with today's source version.
 * --platform defaults to the runner OS; architecture comes from the supplied
 * binary (architecture not inspected/executed here), and must match the later desktop build.
 * The prebuilt binary must contain this checkout's exact generated inventory.
 * This is a minimum build-provenance check, not a signature/authenticity check;
 * the existing metadata gate separately binds identity to the binary hash.
 * Assembly NEVER generates assets into, or recompiles, the supplied binary.
 * PYTHON may name a Python 3 executable; defaults: python on Windows, python3
 * elsewhere. Reuses prepare-workflow-bundles.py's package_files producer: copies
 * the generated rsrc manifest and generates the missing wsflow skills manifest
 * with its existing CRLF-normalized schema-1 algorithm. No source tree is edited.
 */
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { parseArgs } = require('node:util');
const { validateDaemonRoot } = require('../daemon-selection');
const repo = path.resolve(__dirname, '../../..');

// Import only the producer's finite package assembly function, not prepare():
// prepare() also rewrites Pi archives, which this desktop build must not touch.
const PREPARE = `
import importlib.util, pathlib, sys
sys.dont_write_bytecode = True
producer, source, destination = map(pathlib.Path, sys.argv[1:])
spec = importlib.util.spec_from_file_location('workflow_packages', producer)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
for name, data in module.package_files(source).items():
    target = destination.joinpath(*name.split('/'))
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(data)
`;

function assemble({ binary, out, package: product = 'wsflow', metadata, platform = process.platform,
  python = process.env.PYTHON || (process.platform === 'win32' ? 'python' : 'python3') } = {}) {
  if (!['ws', 'wsflow'].includes(product)) throw new Error('Package must be wsflow or ws');
  if (!['darwin', 'win32'].includes(platform)) throw new Error('Target platform must be darwin or win32');
  if (!binary || !out) throw new Error('--binary and --out are required');
  binary = path.resolve(binary); out = path.resolve(out);
  metadata = path.resolve(metadata || binary + '.metadata.json');
  for (const file of [binary, metadata]) {
    if (!fs.lstatSync(file).isFile()) throw new Error(`Expected a regular file (not a link): ${file}`);
  }
  // go:embed stores inventory.json verbatim. Require the canonical inventory
  // in the prebuilt bytes: generating files now cannot repair an old binary.
  const inventory = execFileSync(python, [path.join(repo, 'agents-plugin-tool', 'scripts',
    'generate-package-assets.py'), '--inventory'], { stdio: ['ignore', 'pipe', 'pipe'] });
  if (!fs.readFileSync(binary).includes(inventory)) {
    throw new Error('Prebuilt binary lacks the current embedded package inventory; generate assets BEFORE rebuilding it');
  }
  const source = path.join(repo, product === 'ws' ? 'agents-plugin' : 'agents-plugin-wsflow');
  const relative = path.relative(source, out);
  if (relative === '' || !relative.startsWith('..' + path.sep) && relative !== '..' && !path.isAbsolute(relative)) {
    throw new Error('Output must be outside the source plugin package');
  }
  const plugin = JSON.parse(fs.readFileSync(path.join(source, '.claude-plugin', 'plugin.json'), 'utf8'));
  const contract = JSON.parse(fs.readFileSync(path.join(source, 'runtime.json'), 'utf8'));
  if (plugin.name !== product || contract.plugin !== product || plugin.version !== contract.plugin_version) {
    throw new Error('Source plugin version/name must exactly match runtime.json');
  }
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.mkdirSync(out); // Exclusive ownership: never overwrite or clean an existing output.
  try {
    execFileSync(python, ['-c', PREPARE,
      path.join(repo, 'agents-plugin-pi', 'scripts', 'prepare-workflow-bundles.py'), source, out], { stdio: 'pipe' });
    const target = path.join(out, platform === 'win32' ? 'ws-mcp.exe' : 'ws-mcp');
    fs.copyFileSync(binary, target);
    fs.chmodSync(target, platform === 'darwin' ? 0o755 : 0o644);
    fs.copyFileSync(metadata, target + '.metadata.json');
    return validateDaemonRoot(out, 'bundled', platform);
  } catch (error) {
    fs.rmSync(out, { recursive: true, force: true });
    throw error;
  }
}

if (require.main === module) {
  try {
    const { values } = parseArgs({ options: Object.fromEntries(
      ['binary', 'out', 'package', 'metadata', 'platform'].map(name => [name, { type: 'string' }])) });
    console.log(assemble(values).root);
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
module.exports = { assemble };

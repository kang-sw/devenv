'use strict';
// Launchless Phase 2 smoke packages and signed release bundles.
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { copyPackage } = require('../../desktop-electron/daemon-stage');
const { PLATFORMS, releaseVersion, legacyName, measure, writePlatformManifest } = require('./release.cjs');
const { publicKey } = require('./release-signature.cjs');

const desktop = path.resolve(__dirname, '..');
const repo = path.resolve(desktop, '../..');

function checkArchitecture(binary, platform) {
  const bytes = fs.readFileSync(binary);
  if (platform === 'darwin') {
    // 64-bit little-endian Mach-O, arm64 (the supported macOS package).
    if (bytes.length < 8 || bytes.readUInt32LE(0) !== 0xfeedfacf || bytes.readUInt32LE(4) !== 0x0100000c)
      throw new Error(`Expected a darwin-arm64 Mach-O binary: ${binary}`);
  } else {
    const offset = bytes.length >= 64 ? bytes.readUInt32LE(0x3c) : bytes.length;
    if (bytes.length < offset + 6 || bytes.toString('ascii', 0, 2) !== 'MZ' ||
        bytes.readUInt32LE(offset) !== 0x4550 || bytes.readUInt16LE(offset + 4) !== 0x8664)
      throw new Error(`Expected a win32-x64 PE binary: ${binary}`);
  }
  return bytes;
}

function checkBinary(binary, platform) {
  const bytes = checkArchitecture(binary, platform);
  const python = process.env.PYTHON || (process.platform === 'win32' ? 'python' : 'python3');
  const inventory = execFileSync(python, [path.join(repo, 'agents-plugin-tool/scripts/generate-package-assets.py'), '--inventory']);
  if (!bytes.includes(inventory)) throw new Error('Daemon lacks this checkout’s embedded package inventory');
}

function resourceMap(root) {
  const map = {};
  function walk(dir, rel = '') {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const name = path.join(rel, entry.name), source = path.join(root, name);
      if (entry.isDirectory()) walk(source, name);
      else if (entry.isFile()) map[source] = `daemon/${name.split(path.sep).join('/')}`;
      else throw new Error(`Unexpected daemon package entry: ${source}`);
    }
  }
  walk(root);
  return map;
}

function bundleResourceMap(resources, base, paths = path) {
  // Tauri normalizes resource paths by dropping Windows drive prefixes.
  // Relative sources resolve from the desktop crate, including in build.rs.
  const entries = Object.entries(resources).map(([source, destination]) => {
    const relative = paths.relative(base, source);
    if (paths.isAbsolute(relative)) throw new Error(`Daemon resource is on a different drive: ${source}`);
    return [relative.split(paths.sep).join('/'), destination];
  });
  return Object.fromEntries(entries);
}

function stageDaemonPackage({ input = process.env.WERKSCHAFT_DAEMON_INPUT, platform = process.platform } = {}) {
  if (!['darwin', 'win32'].includes(platform)) throw new Error('Packaging requires macOS or Windows');
  if (!input || !path.isAbsolute(input)) throw new Error('WERKSCHAFT_DAEMON_INPUT must name an absolute assembled package');
  if ((platform === 'darwin' && process.arch !== 'arm64') || (platform === 'win32' && process.arch !== 'x64'))
    throw new Error('Packaging requires darwin-arm64 or win32-x64');
  // The runner's TEMP and checkout can be on different Windows drives.
  const target = path.join(desktop, 'target');
  fs.mkdirSync(target, { recursive: true });
  const stage = fs.mkdtempSync(path.join(target, 'p-'));
  try {
    const daemon = path.join(stage, 'daemon');
    const selected = copyPackage(input, daemon, 'bundled', platform);
    checkBinary(selected.binary, platform);
    return { stage, selected, platform };
  } catch (error) {
    fs.rmSync(stage, { recursive: true, force: true });
    throw error;
  }
}

function build(options) {
  const { stage, selected: { root: daemon }, platform } = stageDaemonPackage(options);
  try {
    const config = path.join(stage, 'tauri.conf.json');
    const resources = bundleResourceMap(resourceMap(daemon), desktop);
    fs.writeFileSync(config, JSON.stringify({ bundle: { resources } }));
    const cli = require.resolve('@tauri-apps/cli/tauri.js');
    const args = [cli, 'build', '--ci', '--config', config];
    if (platform === 'darwin') args.push('--bundles', 'app', '--no-sign');
    else args.push('--no-bundle');
    args.push('--', '--locked');
    execFileSync(process.execPath, args, { cwd: desktop, stdio: 'inherit' });
    const release = path.join(desktop, 'target', 'release');
    if (platform === 'darwin') {
      const output = path.join(release, 'bundle/macos/Werkschaft.app/Contents/Resources/daemon');
      // Compare and revalidate the actual bundle, not just the input.
      comparePackage(daemon, output, platform);
      console.log(`Packaged app: ${path.dirname(path.dirname(path.dirname(output)))}`);
    } else {
      // Tauri's unbundled resource_dir is the executable's directory on Windows.
      const output = path.join(release, 'phase2-smoke');
      const target = path.join(output, 'daemon');
      if (fs.existsSync(output)) throw new Error(`Remove the previous smoke output before rebuilding: ${output}`);
      fs.mkdirSync(output, { recursive: true });
      fs.copyFileSync(path.join(release, 'werkschaft-desktop.exe'), path.join(output, 'werkschaft-desktop.exe'));
      copyPackage(daemon, target, 'bundled', platform);
      comparePackage(daemon, target, platform);
      console.log(`Windows smoke payload: ${output}`);
    }
  } finally { fs.rmSync(stage, { recursive: true, force: true }); }
}

function comparePackage(source, target, platform) {
  const expected = resourceMap(source), actual = resourceMap(target);
  const sourceNames = Object.values(expected).sort(), targetNames = Object.values(actual).sort();
  if (JSON.stringify(sourceNames) !== JSON.stringify(targetNames)) throw new Error('Packaged daemon file list differs from input');
  for (const [file, name] of Object.entries(expected)) {
    const destination = path.join(target, ...name.slice('daemon/'.length).split('/'));
    if (!fs.readFileSync(file).equals(fs.readFileSync(destination))) throw new Error(`Packaged daemon differs: ${name}`);
  }
  require('../../desktop-electron/daemon-selection').validateDaemonRoot(target, 'bundled', platform);
}

function releaseConfig({ resources, platform, version, pubkey }) {
  const parsed = releaseVersion(version);
  const mac = platform === 'darwin';
  const config = {
    version,
    bundle: { active: true, targets: mac ? ['app'] : ['nsis', 'msi'], createUpdaterArtifacts: true, resources },
    plugins: { updater: { pubkey, requireSignedVersion: true, windows: { installMode: 'passive' } } },
  };
  if (mac) config.mainBinaryName = 'Werkschaft';
  else config.bundle.windows = {
    webviewInstallMode: { type: 'embedBootstrapper', silent: true },
    nsis: { installMode: 'currentUser' }, allowDowngrades: true,
    // WiX accepts numeric versions. AllowDowngrades in its stock MajorUpgrade
    // also replaces a different product with the same numeric ProductVersion.
    wix: { version: parsed.core.join('.') },
  };
  return config;
}

function bundleRelease(stage, daemon, platform, version, pubkey, env) {
  const config = path.join(stage, 'tauri.conf.json');
  const resources = bundleResourceMap(resourceMap(daemon), desktop);
  fs.writeFileSync(config, JSON.stringify(releaseConfig({ resources, platform, version, pubkey })));
  const target = platform === 'darwin' ? 'aarch64-apple-darwin' : 'x86_64-pc-windows-msvc';
  // The macOS bundle path has no version. Keep concurrent local builds
  // from reading or signing another build's bundle.
  const targetDir = path.join(stage, 'target');
  const cli = require.resolve('@tauri-apps/cli/tauri.js');
  execFileSync(process.execPath, [cli, 'build', '--ci', '--no-sign', '--target', target, '--config', config, '--', '--locked'],
    { cwd: desktop, stdio: 'inherit', env: { ...env, CARGO_TARGET_DIR: targetDir, WERKSCHAFT_UPDATER_PUBLIC_KEY: pubkey } });
  return path.join(targetDir, target, 'release');
}

function verifyMacApp(app, daemon, version) {
  const contents = path.join(app, 'Contents');
  const plist = JSON.parse(execFileSync('plutil', ['-convert', 'json', '-o', '-', path.join(contents, 'Info.plist')],
    { encoding: 'utf8' }));
  if (plist.CFBundleExecutable !== 'Werkschaft' || plist.CFBundleShortVersionString !== version ||
      plist.CFBundleVersion !== version) throw new Error('macOS bundle Info.plist identity differs from the release');
  const binary = path.join(contents, 'MacOS', 'Werkschaft');
  fs.accessSync(binary, fs.constants.X_OK);
  checkArchitecture(binary, 'darwin');
  comparePackage(daemon, path.join(contents, 'Resources', 'daemon'), 'darwin');
}

function releaseMac(stage, built, daemon, output, version) {
  const app = path.join(built, 'bundle', 'macos', 'Werkschaft.app');
  const archive = path.join(output, PLATFORMS.darwin.assets['darwin-aarch64']);
  fs.copyFileSync(`${app}.tar.gz`, archive);
  const extracted = path.join(stage, 'mac-updater');
  fs.mkdirSync(extracted);
  execFileSync('tar', ['-xf', archive, '-C', extracted]);
  if (fs.readdirSync(extracted).join() !== 'Werkschaft.app') throw new Error('Invalid macOS updater archive root');
  verifyMacApp(path.join(extracted, 'Werkschaft.app'), daemon, version);

  const legacy = legacyName('darwin', version);
  if (!legacy) return;
  const zip = path.join(output, legacy);
  execFileSync('ditto', ['-c', '-k', '--keepParent', '--norsrc', app, zip]);
  const unzipped = path.join(stage, 'mac-legacy');
  execFileSync('ditto', ['-x', '-k', zip, unzipped]);
  if (fs.readdirSync(unzipped).join() !== 'Werkschaft.app') throw new Error('Invalid macOS migration archive root');
  verifyMacApp(path.join(unzipped, 'Werkschaft.app'), daemon, version);
}

async function releaseWindows(stage, built, output, version, env) {
  checkArchitecture(path.join(built, 'werkschaft-desktop.exe'), 'win32');
  const names = PLATFORMS.win32.assets;
  const installer = path.join(output, names['windows-x86_64-nsis']);
  fs.copyFileSync(path.join(built, 'bundle', 'nsis', `Werkschaft_${version}_x64-setup.exe`), installer);
  fs.copyFileSync(path.join(built, 'bundle', 'msi', `Werkschaft_${version}_x64_en-US.msi`),
    path.join(output, names['windows-x86_64-msi']));

  const legacy = legacyName('win32', version);
  if (!legacy) return;
  const migration = path.join(stage, 'windows-legacy');
  fs.mkdirSync(migration);
  const namesAtRoot = ['Werkschaft-setup.exe', 'Werkschaft.exe'];
  const payload = path.join(migration, namesAtRoot[0]);
  fs.copyFileSync(installer, payload);
  const { sha256 } = await measure(payload);
  execFileSync('rustc', ['--edition', '2021', '--target', 'x86_64-pc-windows-msvc', '-C', 'opt-level=z',
    '-C', 'strip=symbols', path.join(desktop, 'migration', 'main.rs'), '-o', path.join(migration, namesAtRoot[1])], {
    stdio: 'inherit', env: { ...env, WERKSCHAFT_PAYLOAD_VERSION: version, WERKSCHAFT_PAYLOAD_ID: sha256 },
  });
  checkArchitecture(path.join(migration, namesAtRoot[1]), 'win32');
  const zip = path.join(output, legacy);
  const tar = path.join(env.SystemRoot || 'C:\\Windows', 'System32', 'tar.exe');
  execFileSync(tar, ['-a', '-cf', zip, ...namesAtRoot], { cwd: migration });
  const members = execFileSync(tar, ['-tf', zip], { encoding: 'utf8' }).trim().split(/\r?\n/).sort();
  if (members.join() !== namesAtRoot.join()) throw new Error('Invalid Windows migration archive root');
}

async function buildRelease() {
  const pubkey = (process.env.WERKSCHAFT_UPDATER_PUBLIC_KEY || '').trim();
  if (!pubkey) throw new Error('WERKSCHAFT_UPDATER_PUBLIC_KEY is required for a signed release');
  const key = publicKey(pubkey);
  const { TAURI_SIGNING_PRIVATE_KEY, TAURI_SIGNING_PRIVATE_KEY_PASSWORD, ...env } = process.env;
  if (!TAURI_SIGNING_PRIVATE_KEY?.trim()) throw new Error('TAURI_SIGNING_PRIVATE_KEY is required for a signed release');
  if (env.WERKSCHAFT_EVALUATION !== '1') delete env.WERKSCHAFT_EVALUATION;
  const output = env.WERKSCHAFT_RELEASE_OUTPUT;
  if (!output || !path.isAbsolute(output)) throw new Error('WERKSCHAFT_RELEASE_OUTPUT must name an absolute fresh directory');
  const { stage, selected, platform } = stageDaemonPackage();
  try {
    if (selected.contract.plugin !== 'wsflow') throw new Error('Release packaging requires an assembled wsflow package');
    const version = releaseVersion(selected.metadata.version).version;
    if (env.WERKSCHAFT_EXPECTED_VERSION && version !== env.WERKSCHAFT_EXPECTED_VERSION) {
      throw new Error(`Daemon metadata version ${version} differs from expected version ${env.WERKSCHAFT_EXPECTED_VERSION}`);
    }
    fs.mkdirSync(output);
    try {
      const built = bundleRelease(stage, selected.root, platform, version, pubkey, env);
      if (platform === 'darwin') releaseMac(stage, built, selected.root, output, version);
      else await releaseWindows(stage, built, output, version, env);
      // --no-sign excludes OS signing. Sign only the final updater filenames
      // with the lockfile-pinned Tauri signer and the exact application version.
      const cli = require.resolve('@tauri-apps/cli/tauri.js');
      for (const name of Object.values(PLATFORMS[platform].assets)) {
        execFileSync(process.execPath, [cli, 'signer', 'sign', '--app-version', version, path.join(output, name)], {
          cwd: desktop, stdio: ['ignore', 'ignore', 'inherit'],
          env: { ...env, TAURI_SIGNING_PRIVATE_KEY: TAURI_SIGNING_PRIVATE_KEY.trim(),
            TAURI_SIGNING_PRIVATE_KEY_PASSWORD: TAURI_SIGNING_PRIVATE_KEY_PASSWORD ?? '' },
        });
      }
      await writePlatformManifest(output, platform, version, key);
    } catch (error) {
      fs.rmSync(output, { recursive: true, force: true });
      throw error;
    }
    console.log(`Verified signed ${platform} assets for ${version}: ${output}`);
  } finally { fs.rmSync(stage, { recursive: true, force: true, maxRetries: 5 }); }
}

if (require.main === module) {
  Promise.resolve().then(() => {
    if (process.argv.length === 2) return build();
    if (process.argv.length === 3 && process.argv[2] === '--release') return buildRelease();
    throw new Error('Usage: node scripts/package.cjs [--release]');
  }).catch(error => { console.error(error.message); process.exitCode = 1; });
}
module.exports = { build, checkBinary, resourceMap, bundleResourceMap, comparePackage, releaseConfig };

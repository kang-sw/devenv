'use strict';
// Packaging is deliberately local-only: electronZipDir bypasses all download code.
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const pkg = require('../package.json');
const { copyPackage } = require('../daemon-stage');
const SUMS = 'Werkschaft-desktop-SHA256SUMS';

// The release assets the updater reads: Werkschaft-<platform>-<arch>.zip and
// a sha256sum-format SUMS file. macOS ditto keeps the framework symlinks;
// Windows 10+ ships bsdtar, which writes a zip for a .zip name with -a.
function archive({ out, platform, arch, host = process.platform, run = execFileSync, env = process.env }) {
  const folder = `Werkschaft-${platform}-${arch}`;
  const zip = path.join(out, `${folder}.zip`);
  fs.rmSync(zip, { force: true });
  const source = platform === 'darwin' ? path.join(out, folder, 'Werkschaft.app') : path.join(out, folder);
  if (host === 'win32') {
    run(path.join(env.SystemRoot || 'C:\\Windows', 'System32', 'tar.exe'), ['-a', '-c', '-f', zip, '-C', path.dirname(source), path.basename(source)]);
  } else run('/usr/bin/ditto', ['-c', '-k', '--keepParent', source, zip]);
  const hash = crypto.createHash('sha256').update(fs.readFileSync(zip)).digest('hex');
  // Builds of each platform run on their own machine: replace only this line.
  const sums = path.join(out, SUMS);
  const kept = fs.existsSync(sums) ? fs.readFileSync(sums, 'utf8').split(/\r?\n/)
    .filter(line => line.trim() && !line.endsWith(`  ${path.basename(zip)}`)) : [];
  fs.writeFileSync(sums, [...kept, `${hash}  ${path.basename(zip)}`].join('\n') + '\n');
  return { zip, sums, hash };
}

async function build({ platform = process.platform, arch = process.arch, daemonInput = process.env.WERKSCHAFT_DAEMON_INPUT, cache, packageApp, out = path.join(root, 'dist'), run } = {}) {
  if (!['darwin', 'win32'].includes(platform)) throw new Error('This slice packages darwin and win32 only');
  if (!daemonInput || !path.isAbsolute(daemonInput)) throw new Error('A supplied absolute preassembled daemonInput is required');
  if (platform === 'win32') arch = 'x64';
  const version = pkg.devDependencies.electron;
  const zipName = `electron-v${version}-${platform}-${arch}.zip`;
  cache ||= process.env.ELECTRON_CACHE || (platform === 'win32'
    ? path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), 'AppData/Local'), 'electron/Cache')
    : path.join(os.homedir(), 'Library/Caches/electron'));
  const candidates = [cache, ...(fs.existsSync(cache) ? fs.readdirSync(cache, { withFileTypes: true }) : [])
    .filter(e => e.isDirectory()).map(e => path.join(cache, e.name))];
  const zipDir = candidates.find(dir => fs.existsSync(path.join(dir, zipName)));
  if (!zipDir) throw new Error(`Offline prerequisite missing: cached ${zipName}; no download attempted`);
  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-package-'));
  try {
    const source = path.join(stage, 'app');
    fs.mkdirSync(source);
    for (const file of ['main.js', 'endpoint.js', 'daemon-selection.js', 'daemon-stage.js', 'daemon-controller.js', 'daemon-version.js',
      'policy.js', 'connection.html', 'python.js', 'python-missing.html', 'update.js', 'dashboard-preload.js',
      'titlebar.js', 'titlebar.html', 'titlebar-preload.js', 'titlebar-renderer.js']) {
      fs.copyFileSync(path.join(root, file), path.join(source, file));
    }
    fs.mkdirSync(path.join(source, 'icons'));
    for (const file of ['icon.png', 'icon.ico']) {
      fs.copyFileSync(path.join(root, 'icons', file), path.join(source, 'icons', file));
    }
    const selected = copyPackage(daemonInput, path.join(source, 'daemon'), 'bundled', platform);
    // The bundled runtime is the release authority, not the shell's source manifest.
    const appVersion = selected.contract.plugin_version;
    fs.writeFileSync(path.join(source, 'package.json'), JSON.stringify({
      name: pkg.name, productName: 'Werkschaft', version: appVersion, main: pkg.main, author: pkg.author,
    }));
    let icon = platform === 'win32' ? path.join(source, 'icons/icon.ico') : undefined;
    if (platform === 'darwin') {
      const iconset = path.join(stage, 'icon.iconset');
      fs.mkdirSync(iconset);
      for (const size of [16, 32, 128, 256, 512]) {
        for (const scale of [1, 2]) {
          execFileSync('/usr/bin/sips', ['-z', String(size * scale), String(size * scale),
            path.join(root, 'icons/icon.png'), '--out', path.join(iconset, `icon_${size}x${size}${scale === 2 ? '@2x' : ''}.png`)], { stdio: 'pipe' });
        }
      }
      icon = path.join(stage, 'icon.icns');
      execFileSync('/usr/bin/iconutil', ['-c', 'icns', iconset, '-o', icon]);
    }
    packageApp ||= (await import('@electron/packager')).packager;
    const outputs = await packageApp({
      dir: source, out, name: 'Werkschaft',
      platform, arch, electronVersion: version,
      electronZipDir: zipDir, appBundleId: 'dev.werkschaft.desktop',
      appVersion, buildVersion: appVersion,
      // Packager 20 defaults to ASAR. Keep every daemon input (including dot
      // directories/manifests) on disk: open/copy of packed entries extracts
      // persistent temp files even when the shell only attaches to a daemon.
      asar: { unpackDir: 'daemon' },
      icon, overwrite: true, prune: false, osxSign: false, osxNotarize: false,
    });
    console.log(outputs.join('\n'));
    const { zip, sums } = archive({ out, platform, arch, run });
    console.log(`${zip}\n${sums}`);
  } finally { fs.rmSync(stage, { recursive: true, force: true }); }
}
if (require.main === module) build().catch(error => { console.error(error.message); process.exitCode = 1; });
module.exports = { build, archive, SUMS };

'use strict';
// Packaging is deliberately local-only: electronZipDir bypasses all download code.
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const pkg = require('../package.json');

async function build({ platform = process.platform, arch = process.arch, cache, packageApp } = {}) {
  if (!['darwin', 'win32'].includes(platform)) throw new Error('This slice packages darwin and win32 only');
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
    for (const file of ['main.js', 'endpoint.js', 'policy.js', 'connection.html']) {
      fs.copyFileSync(path.join(root, file), path.join(source, file));
    }
    fs.writeFileSync(path.join(source, 'package.json'), JSON.stringify({
      name: pkg.name, productName: 'Werkschaft', version: pkg.version, main: pkg.main, author: pkg.author,
    }));
    let icon;
    // Windows acceptance uses Electron's default icon; custom delivery is Phase 3.
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
      dir: source, out: path.join(root, 'dist'), name: 'Werkschaft',
      platform, arch, electronVersion: version,
      electronZipDir: zipDir, appBundleId: 'dev.werkschaft.desktop',
      appVersion: pkg.version, buildVersion: pkg.version,
      icon, overwrite: true, prune: false, osxSign: false, osxNotarize: false,
    });
    console.log(outputs.join('\n'));
  } finally { fs.rmSync(stage, { recursive: true, force: true }); }
}
if (require.main === module) build().catch(error => { console.error(error.message); process.exitCode = 1; });
module.exports = { build };

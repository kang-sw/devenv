'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { parseArgs } = require('node:util');
const { assetName, parseVersion, SUMS } = require('../../desktop-electron/update');
const { publicKey, verifySignature } = require('./release-signature.cjs');

const FEED = 'werkschaft-tauri-update.json';
const MAX_PAYLOAD = 256 * 1024 * 1024; // Matches the native updater download limit.
const PLATFORMS = {
  darwin: { arch: 'arm64', assets: { 'darwin-aarch64': 'Werkschaft-darwin-arm64.app.tar.gz' } },
  win32: { arch: 'x64', assets: { 'windows-x86_64-nsis': 'Werkschaft-win32-x64-setup.exe',
    'windows-x86_64-msi': 'Werkschaft-win32-x64.msi' } },
};

function releaseVersion(version) {
  const parsed = parseVersion(version);
  if (typeof version !== 'string' || parsed?.version !== version) {
    throw new Error('Tauri releases require an exact version X.Y.Z or X.Y.Z-canary.N');
  }
  return parsed;
}

function legacyName(platform, version) {
  if (releaseVersion(version).channel === 'stable') return assetName(platform, PLATFORMS[platform].arch);
}

async function measure(file, signed = false) {
  if (!fs.lstatSync(file).isFile()) throw new Error(`Expected a regular release asset: ${file}`);
  const sha = createHash('sha256');
  const blake = signed && createHash('blake2b512');
  let size = 0;
  for await (const chunk of fs.createReadStream(file)) {
    size += chunk.length;
    if (signed && size > MAX_PAYLOAD) throw new Error(`Updater payload exceeds the download limit: ${file}`);
    sha.update(chunk);
    if (blake) blake.update(chunk);
  }
  if (size === 0) throw new Error(`Empty release asset: ${file}`);
  return { sha256: sha.digest('hex'), size, digest: blake ? blake.digest() : undefined };
}

function writeJSON(file, value) {
  fs.writeFileSync(file, JSON.stringify(value, null, 2) + '\n', { flag: 'wx' });
}

async function inspectAssets(dir, platform, version, key) {
  const assets = {};
  const entries = {};
  for (const [target, name] of Object.entries(PLATFORMS[platform].assets)) {
    const { digest, ...data } = await measure(path.join(dir, name), true);
    const signature = fs.readFileSync(path.join(dir, `${name}.sig`), 'utf8');
    if (signature.length > 4096) throw new Error(`Updater signature exceeds the feed limit: ${name}`);
    verifySignature(key, signature, digest, version);
    assets[name] = data;
    entries[target] = { url: `https://github.com/kang-sw/werkschaft/releases/download/v${version}/${name}`,
      signature, ...data };
  }
  const legacy = legacyName(platform, version);
  if (legacy) {
    const { sha256, size } = await measure(path.join(dir, legacy));
    assets[legacy] = { sha256, size };
  }
  return { assets, entries };
}

function checkNames(dir, expected) {
  const files = fs.readdirSync(dir, { withFileTypes: true });
  if (files.some(file => !file.isFile()) ||
      JSON.stringify(files.map(file => file.name).sort()) !== JSON.stringify(expected.sort())) {
    throw new Error(`Unexpected release files in ${dir}`);
  }
}

function assetFiles(platform, version) {
  const files = Object.values(PLATFORMS[platform].assets).flatMap(name => [name, `${name}.sig`]);
  const legacy = legacyName(platform, version);
  if (legacy) files.push(legacy);
  return files;
}

async function writePlatformManifest(dir, platform, version, key) {
  checkNames(dir, assetFiles(platform, version));
  const { assets } = await inspectAssets(dir, platform, version, key);
  writeJSON(path.join(dir, 'platform.json'), { version, assets });
}

async function combine(input, output, version, key) {
  releaseVersion(version);
  fs.mkdirSync(output);
  try {
    for (const [platform, { arch }] of Object.entries(PLATFORMS)) {
      const source = path.join(input, `tauri-${platform}-${arch}`);
      const files = assetFiles(platform, version);
      checkNames(source, [...files, 'platform.json']);
      for (const file of files) fs.copyFileSync(path.join(source, file), path.join(output, file));
    }
    const { feed, sums } = await describeRelease(output, version, key, input);
    writeJSON(path.join(output, FEED), feed);
    if (sums) fs.writeFileSync(path.join(output, SUMS), sums, { flag: 'wx' });
  } catch (error) {
    fs.rmSync(output, { recursive: true, force: true });
    throw error;
  }
}

async function describeRelease(dir, version, key, input) {
  const feed = { version, platforms: {} };
  const lines = [];
  for (const [platform, { arch }] of Object.entries(PLATFORMS)) {
    const { assets, entries } = await inspectAssets(dir, platform, version, key);
    if (input) {
      const manifest = path.join(input, `tauri-${platform}-${arch}`, 'platform.json');
      if (fs.readFileSync(manifest, 'utf8') !== JSON.stringify({ version, assets }, null, 2) + '\n') {
        throw new Error(`Version, asset names or digests differ from ${manifest}`);
      }
    }
    Object.assign(feed.platforms, entries);
    const legacy = legacyName(platform, version);
    if (legacy) lines.push(`${assets[legacy].sha256}  ${legacy}`);
  }
  return { feed, sums: lines.length ? lines.join('\n') + '\n' : '' };
}

async function verifyRelease(dir, version, key) {
  const files = [FEED, ...Object.keys(PLATFORMS).flatMap(platform => assetFiles(platform, version))];
  if (releaseVersion(version).channel === 'stable') files.push(SUMS);
  checkNames(dir, files);
  const { feed, sums } = await describeRelease(dir, version, key);
  if (fs.readFileSync(path.join(dir, FEED), 'utf8') !== JSON.stringify(feed, null, 2) + '\n' ||
      (sums && fs.readFileSync(path.join(dir, SUMS), 'utf8') !== sums)) {
    throw new Error('Combined release feed or legacy checksums differ from the payload');
  }
}

async function main() {
  const { values } = parseArgs({ options: {
    input: { type: 'string' }, output: { type: 'string' }, version: { type: 'string' }, verify: { type: 'boolean' },
  } });
  if (!values.input || !values.version || (!values.verify && !values.output)) {
    throw new Error('Use --input <directory> --version <version> and either --output <directory> or --verify');
  }
  const key = publicKey((process.env.WERKSCHAFT_UPDATER_PUBLIC_KEY || '').trim());
  if (values.verify) await verifyRelease(values.input, values.version, key);
  else await combine(values.input, values.output, values.version, key);
  console.log(`Verified Tauri release assets for ${values.version}`);
}

if (require.main === module) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
module.exports = { PLATFORMS, releaseVersion, legacyName, measure, writePlatformManifest, combine, verifyRelease };

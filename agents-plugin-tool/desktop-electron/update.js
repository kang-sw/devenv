'use strict';
// Self-update from the public GitHub release: check, download + verify a zip,
// stage it under userData, and swap the app folder from a detached helper
// after this process exits. The daemon is never touched: the shell only
// attaches to it, so replacing the shell's own folder cannot stop it.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const http = require('node:http');
const https = require('node:https');
const { execFile, spawn, spawnSync } = require('node:child_process');

const REPO = 'kang-sw/werkschaft';
const FEED = 'https://api.github.com';
const SUMS = 'Werkschaft-desktop-SHA256SUMS';
// Where a user installs a release by hand: the same release the feed reads.
const DOWNLOAD_PAGE = `https://github.com/${REPO}/releases/latest`;
const USER_AGENT = 'werkschaft-desktop';
const INTERVAL = 6 * 60 * 60 * 1000;
const FIRST_CHECK = 8000;
const LIMITS = { release: 1 << 20, sums: 64 << 10, zip: 1 << 30 };

function parseVersion(value) {
  const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(String(value ?? '').trim());
  return match ? match.slice(1).map(Number) : null;
}
// Numeric x.y.z only: a prerelease suffix never parses, so it is never newer.
function isNewer(candidate, current) {
  const a = parseVersion(candidate), b = parseVersion(current);
  if (!a || !b) return false;
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] > b[i];
  return false;
}
const assetName = (platform, arch) => `Werkschaft-${platform}-${arch}.zip`;

function parseSums(text) {
  const sums = new Map();
  for (const line of String(text).split(/\r?\n/)) {
    const match = /^([0-9a-fA-F]{64})\s+\*?(\S.*?)\s*$/.exec(line);
    if (match) sums.set(match[2], match[1].toLowerCase());
  }
  return sums;
}

// Returns { version, asset, zipURL, sumsURL } or { reason } when there is
// nothing to install (not newer, draft/prerelease, missing asset).
function selectRelease(release, { platform, arch, current }) {
  if (!release || typeof release !== 'object') return { reason: 'invalid release response' };
  if (release.draft || release.prerelease) return { reason: 'latest release is a draft or prerelease' };
  const version = parseVersion(release.tag_name)?.join('.');
  if (!version) return { reason: `unrecognized tag ${release.tag_name}` };
  if (!isNewer(version, current)) return { reason: `v${version} is not newer than v${current}` };
  const assets = Array.isArray(release.assets) ? release.assets : [];
  const url = name => assets.find(a => a && a.name === name && typeof a.browser_download_url === 'string')?.browser_download_url;
  const asset = assetName(platform, arch);
  const zipURL = url(asset), sumsURL = url(SUMS);
  if (!zipURL) return { reason: `v${version} has no ${asset}` };
  if (!sumsURL) return { reason: `v${version} has no ${SUMS}` };
  return { version, asset, zipURL, sumsURL };
}

// https anywhere; plain http only to loopback, for a local test feed.
function allowedURL(raw) {
  let url;
  try { url = new URL(raw); } catch { return null; }
  if (url.username || url.password) return null;
  if (url.protocol === 'https:') return url;
  if (url.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)) return url;
  return null;
}
function get(raw, accept, redirects = 5) {
  return new Promise((resolve, reject) => {
    const url = allowedURL(raw);
    if (!url) return reject(new Error(`Refused update URL ${raw}`));
    const request = (url.protocol === 'https:' ? https : http).get(url, {
      headers: { 'User-Agent': USER_AGENT, Accept: accept }, timeout: 30000,
    }, response => {
      const { statusCode, headers } = response;
      if ([301, 302, 303, 307, 308].includes(statusCode) && headers.location) {
        response.resume();
        if (!redirects) return reject(new Error('Too many redirects'));
        return resolve(get(new URL(headers.location, url).href, accept, redirects - 1));
      }
      if (statusCode !== 200) { response.resume(); return reject(new Error(`HTTP ${statusCode} from ${url.host}`)); }
      resolve(response);
    });
    request.on('timeout', () => request.destroy(new Error('Update request timed out')));
    request.on('error', reject);
  });
}
async function readText(url, max, accept = '*/*') {
  const response = await get(url, accept);
  const chunks = [];
  let size = 0;
  for await (const chunk of response) {
    size += chunk.length;
    if (size > max) { response.destroy(); throw new Error('Update response too large'); }
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString('utf8');
}
async function downloadTo(url, file, max) {
  const response = await get(url, 'application/octet-stream');
  const hash = crypto.createHash('sha256'), out = fs.createWriteStream(file);
  let size = 0;
  try {
    for await (const chunk of response) {
      size += chunk.length;
      if (size > max) throw new Error('Update download too large');
      hash.update(chunk);
      if (!out.write(chunk)) await new Promise(resolve => out.once('drain', resolve));
    }
  } catch (error) { response.destroy(); out.destroy(); throw error; }
  await new Promise((resolve, reject) => out.end(error => error ? reject(error) : resolve()));
  return hash.digest('hex');
}

// The folder the swap replaces: the enclosing .app on macOS, the folder that
// holds Werkschaft.exe on Windows. A dev run (electron .) has none.
function installLocation(platform, execPath, packaged) {
  if (!packaged) return null;
  if (platform === 'darwin') {
    const app = path.resolve(execPath, '../../..');
    return app.endsWith('.app') ? app : null;
  }
  if (platform === 'win32') return path.dirname(execPath);
  return null;
}
function extractCommand(platform, zip, dest, env = process.env) {
  if (platform === 'darwin') return ['/usr/bin/ditto', ['-x', '-k', zip, dest]];
  // System32 bsdtar reads zip; a Git-for-Windows GNU tar on PATH does not.
  return [path.join(env.SystemRoot || 'C:\\Windows', 'System32', 'tar.exe'), ['-x', '-f', zip, '-C', dest]];
}
// The app inside an extracted archive, or null when it is not there.
function stagedApp(platform, dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true }).filter(e => e.isDirectory());
  if (platform === 'darwin') {
    const app = path.join(dir, 'Werkschaft.app');
    return fs.existsSync(path.join(app, 'Contents/MacOS/Werkschaft')) ? app : null;
  }
  if (fs.existsSync(path.join(dir, 'Werkschaft.exe'))) return dir;
  for (const entry of entries) {
    const folder = path.join(dir, entry.name);
    if (fs.existsSync(path.join(folder, 'Werkschaft.exe'))) return folder;
  }
  return null;
}
function stagingLocation(platform, install, userData) {
  return platform === 'win32' && install ? `${install}-update-staging` : path.join(userData, 'update');
}
// Why the staged update cannot be swapped in, or null.
function applyProblem(platform, install, staging, io = fs) {
  if (!install) return 'this build cannot update itself (not a packaged app)';
  if (platform === 'darwin' && install.includes('/AppTranslocation/')) {
    return 'macOS runs this copy from a read-only location; move Werkschaft.app to Applications first';
  }
  if (platform === 'win32' && path.dirname(install).toLowerCase() !== path.dirname(staging).toLowerCase()) {
    return 'the update staging folder must be beside the app folder';
  }
  try {
    io.accessSync(path.dirname(install), fs.constants.W_OK);
    io.accessSync(install, fs.constants.W_OK);
    if (platform === 'win32') {
      // W_OK does not check Windows ACLs. Exercise create/rename/delete on
      // owned empty directories, never rename the running installation.
      for (const folder of [install, path.dirname(install)]) {
        let probe, renamed;
        try {
          probe = io.mkdtempSync(path.join(folder, '.werkschaft-update-probe-'));
          renamed = probe + '-renamed';
          io.renameSync(probe, renamed);
          io.rmdirSync(renamed);
        } finally {
          for (const owned of [renamed, probe].filter(Boolean)) {
            try { io.rmdirSync(owned); } catch (error) { if (error.code !== 'ENOENT') throw error; }
          }
        }
      }
    }
  } catch (error) { return `the app folder ${install} is not writable: ${error.message}`; }
  return null;
}

// Arguments: pid target staged backup staging relaunch(0|1) [app args...]. It
// waits for the app to exit, moves the old folder aside, moves the staged one
// in, restores the backup on failure, and touches nothing but those paths. A
// relaunch passes the original app arguments through "$@" unchanged.
const SH_HELPER = `#!/bin/sh
pid="$1"; target="$2"; staged="$3"; backup="$4"; staging="$5"; relaunch="$6"
shift 6
i=0
while kill -0 "$pid" 2>/dev/null; do
  i=$((i + 1)); if [ "$i" -gt 600 ]; then echo "app did not exit"; exit 1; fi
  sleep 0.2
done
rm -rf "$backup"
if ! mv "$target" "$backup"; then echo "could not move the old app aside"; exit 1; fi
if mv "$staged" "$target"; then
  rm -rf "$backup" "$staging"
  code=0; echo "updated $target"
else
  rm -rf "$target"
  mv "$backup" "$target"
  code=1; echo "update failed; restored the previous app"
fi
# -n: open hands --args only to a newly launched instance.
if [ "$relaunch" = 1 ]; then /usr/bin/open -n "$target" --args "$@"; fi
exit $code
`;
// RelaunchArgs is base64 (UTF-8) of the app's command line, already quoted
// for Windows by windowsCommandLine: one opaque token, so no argument text
// is ever parsed by PowerShell. Empty command lines use the parameter default.
const PS_HELPER = `param([int]$AppPid, [string]$Target, [string]$Staged, [string]$Backup, [string]$Staging, [int]$Relaunch, [string]$Log, [string]$RelaunchArgs = '-', [string]$Result = '')
$ErrorActionPreference = 'Stop'
function Finish([string]$Status, [string]$Message) {
  Add-Content -LiteralPath $Log -Encoding UTF8 -Value $Message
  if ($Result) { @{ status = $Status; message = $Message } | ConvertTo-Json -Compress | Set-Content -LiteralPath $Result -Encoding UTF8 }
}
try {
  Add-Content -LiteralPath $Log -Encoding UTF8 -Value "helper started pid=$PID app=$AppPid"
  try { Wait-Process -Id $AppPid -Timeout 120 -ErrorAction SilentlyContinue } catch {}
  if (Get-Process -Id $AppPid -ErrorAction SilentlyContinue) { throw 'app did not exit within 120 seconds' }
  $moved = $false
  # Helper processes can hold files for a moment after the main process exits.
  for ($i = 0; $i -lt 50 -and -not $moved; $i++) {
    try {
      if (Test-Path -LiteralPath $Backup) { Remove-Item -LiteralPath $Backup -Recurse -Force }
      Move-Item -LiteralPath $Target -Destination $Backup
      $moved = $true
    } catch { $moveError = $_; Start-Sleep -Milliseconds 200 }
  }
  if (-not $moved) { throw "could not move the old app aside: $moveError" }
  try {
    Move-Item -LiteralPath $Staged -Destination $Target
    $code = 0; $message = "updated $Target"
  } catch {
    if (Test-Path -LiteralPath $Target) { Remove-Item -LiteralPath $Target -Recurse -Force -ErrorAction SilentlyContinue }
    $swapError = $_
    try { Move-Item -LiteralPath $Backup -Destination $Target }
    catch { throw "update failed ($swapError); rollback failed; previous app backup is at $Backup; rollback error: $_" }
    $code = 1; $message = "update failed; restored the previous app: $swapError"
  }
  # Persist completion before launching: the new app may read it immediately.
  if ($code -eq 0) { Finish 'success' $message } else { Finish 'error' $message }
  if ($Relaunch -eq 1) {
    $exe = Join-Path $Target 'Werkschaft.exe'
    if ($RelaunchArgs -and $RelaunchArgs -ne '-') {
      # A single string goes to the new process verbatim as its command line tail.
      $argLine = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($RelaunchArgs))
      Start-Process -FilePath $exe -ArgumentList $argLine
    } else { Start-Process -FilePath $exe }
  }
  # Relaunch first: antivirus scanning the old tree must not delay the window.
  if ($code -eq 0) {
    Remove-Item -LiteralPath $Backup -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $Staging -Recurse -Force -ErrorAction SilentlyContinue
  }
  exit $code
} catch {
  Finish 'error' "helper failed: $_"
  exit 1
}
`;
// A rapid manual reopen yields its single-instance lock and executable locks
// before this independent continuation launches the installed app. Unlike a
// request-file handoff, its ownership survives the original helper completing.
const PS_REOPEN = `param([int]$AppPid, [int]$OwnerPid, [string]$OwnerCreated, [string]$Target, [string]$Result, [string]$Log, [string]$RelaunchArgs = '-')
$ErrorActionPreference = 'Stop'
try {
  Add-Content -LiteralPath $Log -Encoding UTF8 -Value "reopen helper started pid=$PID app=$AppPid"
  try { Wait-Process -Id $AppPid -Timeout 120 -ErrorAction SilentlyContinue } catch {}
  if (Get-Process -Id $AppPid -ErrorAction SilentlyContinue) { throw 'reopening app did not exit' }
  $deadline = (Get-Date).AddSeconds(180)
  while ((Get-Date) -lt $deadline) {
    if ($OwnerPid -gt 0) {
      $owner = Get-Process -Id $OwnerPid -ErrorAction SilentlyContinue
      if (-not $owner -or [string]$owner.StartTime.ToUniversalTime().Ticks -ne $OwnerCreated) { break }
    } else {
      try {
        $outcome = Get-Content -LiteralPath $Result -Raw | ConvertFrom-Json
        if ($outcome.status -eq 'success' -or $outcome.status -eq 'error') { break }
      } catch {}
    }
    Start-Sleep -Milliseconds 200
  }
  $exe = Join-Path $Target 'Werkschaft.exe'
  if ($RelaunchArgs -and $RelaunchArgs -ne '-') {
    $argLine = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($RelaunchArgs))
    Start-Process -FilePath $exe -ArgumentList $argLine
  } else { Start-Process -FilePath $exe }
  Add-Content -LiteralPath $Log -Encoding UTF8 -Value "reopened $Target"
} catch {
  Add-Content -LiteralPath $Log -Encoding UTF8 -Value "reopen helper failed: $_"
  exit 1
}
`;
function helperScript(platform, reopen = false) { return platform === 'win32' ? (reopen ? PS_REOPEN : PS_HELPER) : SH_HELPER; }
// The arguments a relaunch repeats: the original ones (process.argv.slice(1)
// in a packaged app) minus -psn_* (the process serial number older macOS
// Launch Services adds; the new launch gets its own).
function relaunchArgs(argv) {
  return argv.filter(arg => typeof arg === 'string' && !arg.startsWith('-psn_'));
}
// Quotes args into one command line that CommandLineToArgvW (and the CRT,
// which Electron uses) splits back into exactly those args.
function windowsCommandLine(args) {
  return args.map(arg => {
    if (arg && !/[\s"]/.test(arg)) return arg;
    let out = '"', slashes = 0;
    for (const ch of arg) {
      if (ch === '\\') { slashes++; continue; }
      out += '\\'.repeat(ch === '"' ? slashes * 2 + 1 : slashes) + ch;
      slashes = 0;
    }
    return out + '\\'.repeat(slashes * 2) + '"';
  }).join(' ');
}
function helperCommand(platform, script, { pid, target, staged, backup, staging, relaunch, args = [],
  logFile = path.join(path.dirname(staging), 'update-apply.log'), resultFile, reopen = false, helper }, env = process.env) {
  const flag = relaunch ? '1' : '0';
  if (platform === 'win32') {
    const line = windowsCommandLine(args);
    return [path.join(env.SystemRoot || 'C:\\Windows', 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe'),
      ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', script,
        '-AppPid', String(pid), '-Target', target,
        ...(reopen ? ['-OwnerPid', String(helper?.pid || 0), '-OwnerCreated', helper?.created || '0']
          : ['-Staged', staged, '-Backup', backup, '-Staging', staging, '-Relaunch', flag]),
        '-Log', logFile,
        ...(line ? ['-RelaunchArgs', Buffer.from(line, 'utf8').toString('base64')] : []),
        ...(resultFile ? ['-Result', resultFile] : [])]];
  }
  return ['/bin/sh', [script, String(pid), target, staged, backup, staging, flag, ...args]];
}

function runFile(command, args) {
  return new Promise((resolve, reject) => execFile(command, args, { windowsHide: true }, error => error ? reject(error) : resolve()));
}
function windowsBootstrap(command, args, cwd) {
  const quote = value => "'" + value.replace(/['\u2018-\u201B]/g, '$&$&') + "'";
  // Windows PowerShell can exit 0 before -File when created with Node's
  // detached/invalid-console handles. A short-lived, piped bootstrap uses
  // ShellExecute (Start-Process) to give the hidden helper its own lifetime.
  const script = `$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; $child = Start-Process -FilePath ${quote(command)} -WindowStyle Hidden -WorkingDirectory ${quote(cwd)} -ArgumentList ${quote(windowsCommandLine(args))} -PassThru; @{ pid = $child.Id; created = [string]$child.StartTime.ToUniversalTime().Ticks } | ConvertTo-Json -Compress`;
  return ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-OutputFormat', 'Text',
    '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')];
}
function spawnDetached(command, args, logFile, platform = process.platform, launch = spawnSync) {
  // cwd outside the app folder: on Windows a child's cwd locks that folder.
  const cwd = path.dirname(logFile);
  if (platform === 'win32') {
    fs.appendFileSync(logFile, 'starting Windows update helper via Start-Process\n');
    const result = launch(command, windowsBootstrap(command, args, cwd), {
      cwd, windowsHide: true, encoding: 'utf8', timeout: 15000,
    });
    if (result.error || result.status !== 0) {
      throw new Error(`helper launch failed: ${result.error?.message || result.stderr || `exit ${result.status}`}`);
    }
    return JSON.parse(result.stdout.trim());
  }
  const out = fs.openSync(logFile, 'a');
  try { spawn(command, args, { cwd, detached: true, stdio: ['ignore', out, out], windowsHide: true }).unref(); }
  finally { fs.closeSync(out); }
}

// Query only the helper we created, including creation identity to reject PID
// reuse. No process is signalled. Unknown observations conservatively keep its
// stage until the bounded apply window has expired.
function windowsHelperRunning(helper, launch = spawnSync) {
  if (!helper || !Number.isSafeInteger(helper.pid) || helper.pid <= 0 || !/^\d+$/.test(helper.created)) return null;
  const command = helperCommand('win32', '', { staging: '' })[0];
  const script = `$p = Get-Process -Id ${helper.pid} -ErrorAction SilentlyContinue; if ($p -and [string]$p.StartTime.ToUniversalTime().Ticks -eq '${helper.created}') { exit 0 } else { exit 1 }`;
  const result = launch(command, ['-NoProfile', '-NonInteractive', '-Command', script], { windowsHide: true, timeout: 5000 });
  return result.error || ![0, 1].includes(result.status) ? null : result.status === 0;
}

// send(status) delivers { state, version, ..., phase } to the dashboard: the
// toast keys on state (available | downloading | ready | current | error |
// download-error | apply-error), the settings line on phase, the updater's
// standing state: unchecked | none | available | ready ({ phase, version, skipped }).
// downloadPage, when present, asks the toast for an [Open download page]
// action: on every download-error / apply-error, and on available where the
// app does not update itself (it then replaces [Update]).
function createUpdater({ version, userData, packaged, platform = process.platform, arch = process.arch,
  execPath = process.execPath, argv = process.argv.slice(1), pid = process.pid, feed = process.env.WERKSCHAFT_UPDATE_FEED || FEED,
  send = () => {}, log = message => console.error(message), run = runFile, detach = spawnDetached,
  isApplying = windowsHelperRunning, now = Date.now, locationProblem = applyProblem } = {}) {
  const staging = stagingLocation(platform, installLocation(platform, execPath, packaged), userData);
  const logFile = path.join(userData, 'update-apply.log');
  const resultFile = path.join(userData, 'update-apply-result.json');
  const stateFile = path.join(userData, 'update-state.json');
  const stagedFile = path.join(staging, 'staged.json');
  let available = null, busy = false, phase = { phase: 'unchecked', version: null, skipped: false };
  const setPhase = (name, value = null, skipped = false) => { phase = { phase: name, version: value, skipped }; };
  const emit = status => send({ ...status, phase: { ...phase } });
  const readJSON = file => { try { return JSON.parse(fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, '')) || {}; } catch { return {}; } };
  const readState = () => readJSON(stateFile);
  function writeState(patch) {
    fs.mkdirSync(userData, { recursive: true });
    fs.writeFileSync(stateFile, JSON.stringify({ ...readState(), ...patch }));
  }
  let applyFailure = readState().applyFailure || null;
  const emitFailure = () => emit({ state: 'apply-error', ...applyFailure, downloadPage: DOWNLOAD_PAGE });
  function acknowledgeFailure() {
    if (applyFailure) { writeState({ applyFailure: null }); applyFailure = null; }
  }
  if (applyFailure && !isNewer(applyFailure.version, version)) acknowledgeFailure();
  // Called only after main actually sends the queued failure to the dashboard.
  function statusDelivered(status) {
    if (applyFailure && status.state === 'apply-error' && status.version === applyFailure.version
      && status.message === applyFailure.message) acknowledgeFailure();
  }
  // A staged update newer than this app whose folder is still there.
  function staged() {
    const value = readJSON(stagedFile);
    return isNewer(value.version, version) && typeof value.app === 'string' && fs.existsSync(value.app) ? value : null;
  }

  async function check(manual = false) {
    let found;
    try {
      const release = JSON.parse(await readText(`${feed.replace(/\/+$/, '')}/repos/${REPO}/releases/latest`,
        LIMITS.release, 'application/vnd.github+json'));
      found = selectRelease(release, { platform, arch, current: version });
      if (found.version) {
        const sha256 = parseSums(await readText(found.sumsURL, LIMITS.sums)).get(found.asset);
        found = sha256 ? { ...found, sha256 } : { reason: `${SUMS} has no ${found.asset} line` };
      }
    } catch (error) {
      log(`Update check failed: ${error.message}`);
      // A failed check leaves the standing phase as it was.
      if (manual) emit({ state: 'error', manual });
      return 'error';
    }
    if (!found.version) {
      log(`No update: ${found.reason}`);
      if (!staged()) setPhase('none');
      if (manual) emit({ state: 'current', version, manual });
      return 'current';
    }
    available = found;
    if (phase.phase === 'ready' && phase.version === found.version) { emit({ state: 'ready', version: found.version }); return 'ready'; }
    const skipped = readState().skipped === found.version;
    setPhase('available', found.version, skipped);
    if (skipped && !manual) return 'skipped';
    const problem = platform === 'win32' ? locationProblem(platform, installLocation(platform, execPath, packaged), staging) : null;
    emit({ state: problem ? 'apply-error' : 'available', version: found.version, current: version, skipped, manual,
      ...(problem ? { downloadPage: DOWNLOAD_PAGE, message: problem } : {}) });
    return 'available';
  }

  function skip(value) {
    if (!available || value !== available.version) return false;
    writeState({ skipped: value });
    if (phase.phase === 'available' && phase.version === value) setPhase('available', value, true);
    return true;
  }

  async function download() {
    if (busy || !available) return null;
    busy = true;
    const target = available;
    emit({ state: 'downloading', version: target.version });
    try {
      const install = installLocation(platform, execPath, packaged);
      if (!install) throw new Error('this build cannot update itself (not a packaged app)');
      const initialProblem = locationProblem(platform, install, staging);
      if (initialProblem) {
        emit({ state: 'apply-error', version: target.version, message: initialProblem, downloadPage: DOWNLOAD_PAGE });
        return 'apply-error';
      }
      fs.rmSync(staging, { recursive: true, force: true });
      fs.mkdirSync(staging, { recursive: true });
      const zip = path.join(staging, target.asset);
      const hash = await downloadTo(target.zipURL, zip, LIMITS.zip);
      if (hash !== target.sha256) throw new Error(`checksum mismatch for ${target.asset}`);
      const extract = path.join(staging, 'extract');
      fs.mkdirSync(extract);
      await run(...extractCommand(platform, zip, extract));
      const app = stagedApp(platform, extract);
      if (!app) throw new Error(`${target.asset} holds no Werkschaft app`);
      fs.rmSync(zip, { force: true });
      fs.writeFileSync(stagedFile, JSON.stringify({ version: target.version, app }));
      const problem = locationProblem(platform, install, staging);
      // Keep the verified stage either way: fixing the folder lets a later quit apply it.
      if (!problem) setPhase('ready', target.version);
      emit(problem ? { state: 'apply-error', version: target.version, message: problem, downloadPage: DOWNLOAD_PAGE }
        : { state: 'ready', version: target.version });
      return problem ? 'apply-error' : 'ready';
    } catch (error) {
      log(`Update download failed: ${error.message}`);
      fs.rmSync(staging, { recursive: true, force: true });
      emit({ state: 'download-error', version: target.version, message: error.message, downloadPage: DOWNLOAD_PAGE });
      return 'download-error';
    } finally { busy = false; }
  }

  // Starts the detached swap helper for a verified stage; true when started.
  function apply(relaunch) {
    const stage = staged();
    if (!stage) return false;
    const install = installLocation(platform, execPath, packaged);
    const problem = locationProblem(platform, install, staging);
    if (problem) { log(`Update not applied: ${problem}`); return false; }
    const script = path.join(staging, platform === 'win32' ? 'apply.ps1' : 'apply.sh');
    try {
      if (platform === 'win32') {
        fs.rmSync(resultFile, { force: true });
        writeState({ applyAttempt: { version: stage.version, startedAt: now(), target: install } });
      }
      fs.writeFileSync(script, helperScript(platform));
      const [command, args] = helperCommand(platform, script, {
        pid, target: install, staged: stage.app, backup: `${install}-update-backup`, staging, relaunch,
        args: relaunch ? relaunchArgs(argv) : [], logFile,
        ...(platform === 'win32' ? { resultFile } : {}),
      });
      const helper = detach(command, args, logFile);
      if (platform === 'win32') {
        const attempt = readState().applyAttempt;
        if (attempt && attempt.target === install && attempt.version === stage.version) {
          writeState({ applyAttempt: { ...attempt, helper } });
        }
      }
      return true;
    } catch (error) {
      const message = `Update apply failed: ${error.message}`;
      fs.appendFileSync(logFile, message + '\n');
      if (platform === 'win32') fs.writeFileSync(resultFile, JSON.stringify({ status: 'error', message }));
      emit({ state: 'apply-error', version: stage.version, message, downloadPage: DOWNLOAD_PAGE });
      return false;
    }
  }
  // The quit path: swap after exit. A plain full quit does not relaunch (the
  // user quit); restart() asks the main process to quit with relaunch.
  const applyOnQuit = (relaunch = false) => apply(relaunch);
  // True when a verified, applicable stage is ready for "Restart now".
  const canRestart = () => phase.phase === 'ready' && !!staged();
  // Launch path, before any window: a stage left by a reboot or an unclean
  // exit is swapped in and the new app relaunched. Only one launch attempt
  // per version, so a swap that keeps failing cannot loop.
  function deferLaunch(attempt) {
    try {
      const script = path.join(userData, 'update-reopen.ps1');
      fs.writeFileSync(script, helperScript('win32', true));
      const [command, args] = helperCommand('win32', script, { pid, staging,
        target: installLocation(platform, execPath, packaged), logFile, resultFile,
        reopen: true, helper: attempt.helper, args: relaunchArgs(argv) });
      detach(command, args, logFile);
      return true;
    } catch (error) {
      const message = `Update reopen failed: ${error.message}`;
      fs.appendFileSync(logFile, message + '\n');
      emit({ state: 'apply-error', version: attempt.version, message, downloadPage: DOWNLOAD_PAGE });
      return false;
    }
  }
  function applyOnLaunch() {
    const stage = staged();
    const state = readState();
    if (platform === 'win32' && state.applyAttempt && (!state.applyAttempt.target ||
      path.resolve(state.applyAttempt.target).toLowerCase() === path.resolve(installLocation(platform, execPath, packaged) || '').toLowerCase())) {
      const attempt = state.applyAttempt;
      let result = readJSON(resultFile);
      // Reconcile completion after the synchronous liveness query: the helper
      // can finish during that query. A live snapshot is not a relaunch ack.
      const running = result.status ? false : isApplying(attempt.helper);
      result = readJSON(resultFile);
      const active = !result.status && (running === true || running === null && now() - attempt.startedAt < 180000);
      if (active && attempt.target) return deferLaunch(attempt);
      if (result.status === 'success' && attempt.target && !attempt.deferred && isNewer(attempt.version, version)) {
        // Persist before launching the continuation, so an older copy cannot loop.
        writeState({ applyAttempt: { ...attempt, deferred: true } });
        return deferLaunch(attempt);
      }
      writeState({ applyAttempt: null });
      if (result.status === 'success' && attempt.deferred) return false;
      if (result.status !== 'success') {
        const message = result.status === 'error' && typeof result.message === 'string' ? result.message
          : 'the previous update did not complete; see update-apply.log';
        log(message);
        fs.appendFileSync(logFile, message + '\n');
        if (stage?.version === attempt.version) fs.rmSync(staging, { recursive: true, force: true });
        if (isNewer(attempt.version, version)) {
          applyFailure = { version: attempt.version, message };
          writeState({ applyFailure });
          emitFailure();
          return false;
        }
      }
    }
    if (applyFailure) { emitFailure(); return false; }
    if (!stage) {
      if (fs.existsSync(staging) && !busy) fs.rmSync(staging, { recursive: true, force: true });
      if (state.launchAttempt) writeState({ launchAttempt: null });
      return false;
    }
    if (state.launchAttempt === stage.version) {
      log(`Update v${stage.version} failed to apply; discarding it`);
      fs.rmSync(staging, { recursive: true, force: true });
      writeState({ launchAttempt: null });
      emit({ state: 'apply-error', version: stage.version, message: 'the previous attempt failed; see update-apply.log', downloadPage: DOWNLOAD_PAGE });
      return false;
    }
    // An unwritable folder keeps the stage (and no attempt is spent) and says so.
    const problem = locationProblem(platform, installLocation(platform, execPath, packaged), staging);
    if (problem) {
      emit({ state: 'apply-error', version: stage.version, message: problem, downloadPage: DOWNLOAD_PAGE });
      return false;
    }
    writeState({ launchAttempt: stage.version });
    return apply(true);
  }

  let timers = [];
  function schedule() {
    const first = setTimeout(() => check(false), FIRST_CHECK);
    const every = setInterval(() => check(false), INTERVAL);
    first.unref?.(); every.unref?.();
    timers = [first, every];
  }
  const stop = () => { timers.forEach(clearTimeout); timers.forEach(clearInterval); timers = []; };
  return { check, skip, download, applyOnQuit, applyOnLaunch, canRestart, phase: () => ({ ...phase }),
    schedule, stop, statusDelivered, paths: { staging, stateFile } };
}

module.exports = { REPO, FEED, SUMS, DOWNLOAD_PAGE, USER_AGENT, INTERVAL, parseVersion, isNewer, assetName, parseSums,
  selectRelease, allowedURL, readText, downloadTo, installLocation, extractCommand, stagedApp, stagingLocation, applyProblem,
  helperScript, helperCommand, relaunchArgs, windowsCommandLine, windowsBootstrap, spawnDetached, windowsHelperRunning, createUpdater };

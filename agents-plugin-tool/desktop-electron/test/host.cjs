'use strict';
const path = require('node:path');
const { execFileSync } = require('node:child_process');

function builtPaths(root, platform = process.platform, arch = process.arch) {
  if (platform === 'win32') return {
    executablePath: path.join(root, 'dist/Werkschaft-win32-x64/Werkschaft.exe'),
    daemonPath: path.join(root, 'dist/smoke-daemon.exe'),
  };
  if (platform !== 'darwin') throw new Error('Smoke supports darwin and win32 only');
  return {
    executablePath: path.join(root, `dist/Werkschaft-darwin-${arch}/Werkschaft.app/Contents/MacOS/Werkschaft`),
    daemonPath: path.join(root, 'dist/smoke-daemon'),
  };
}

function processes(platform = process.platform, run = execFileSync) {
  if (platform === 'win32') {
    // CIM includes parent and creation identity; tasklist alone cannot protect
    // cleanup against a recycled PID. Return only metadata, never signal here.
    const script = "ConvertTo-Json -Compress -InputObject @(Get-CimInstance Win32_Process | Where-Object { $_.CreationDate } | ForEach-Object { [pscustomobject]@{ pid = [int]$_.ProcessId; ppid = [int]$_.ParentProcessId; start = $_.CreationDate.ToUniversalTime().Ticks.ToString(); command = $_.CommandLine } })";
    return JSON.parse(run('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], { encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 }).trim() || '[]');
  }
  return run('ps', ['-axo', 'pid=,ppid=,lstart=,command='], { encoding: 'utf8' }).split('\n').map(line => {
    const m = line.match(/^\s*(\d+)\s+(\d+)\s+(.{24})\s+(.*)$/);
    return m && { pid: +m[1], ppid: +m[2], start: m[3], command: m[4] };
  }).filter(Boolean);
}
// Windows UTC ticks retain CIM's full 100 ns creation precision. macOS ps has
// only second precision; normalize it to the same units for ordering, retaining
// the original start string as the identity checked before every signal.
const epochTicks = 621355968000000000n;
const ticks = milliseconds => epochTicks + BigInt(milliseconds) * 10000n;
function creationTime(p) {
  if (typeof p?.start !== 'string') return null;
  if (/^\d+$/.test(p.start)) return BigInt(p.start);
  const milliseconds = Date.parse(p.start);
  return Number.isFinite(milliseconds) ? ticks(milliseconds) : null;
}
function sameIdentity(expected, actual) {
  return !!expected && !!actual && expected.pid === actual.pid && expected.start === actual.start;
}
function launchedRoot(child, all, launchedAt, parentPid = process.pid) {
  if (child.exitCode !== null || child.signalCode !== null) return null;
  const root = all.find(p => p.pid === child.pid && p.ppid === parentPid);
  const created = creationTime(root);
  if (created === null) return null;
  const earliest = /^\d+$/.test(root.start) ? launchedAt : Math.floor(launchedAt / 1000) * 1000;
  if (created < ticks(earliest)) return null;
  return Object.freeze({ ...root });
}
function descendants(root, all) {
  if (!root || !sameIdentity(root, all.find(p => p.pid === root.pid))) return [];
  const rootTime = creationTime(root);
  if (rootTime === null) return [];
  const family = new Map([[root.pid, root]]);
  let changed;
  do {
    changed = false;
    for (const p of all) {
      if (family.has(p.pid)) continue;
      const parentTime = creationTime(family.get(p.ppid));
      const childTime = creationTime(p);
      // ParentProcessId survives the parent's death on Windows. A child older
      // than either this parent identity or the launched root is not ours.
      if (parentTime === null || childTime === null || childTime < parentTime || childTime < rootTime) continue;
      family.set(p.pid, Object.freeze({ ...p }));
      changed = true;
    }
  } while (changed);
  return [...family.values()];
}
function signalIfCurrent(identity, list = processes, signal = process.kill) {
  if (!identity || !sameIdentity(identity, list().find(p => p.pid === identity.pid))) return false;
  // Refresh for each signal, not once for a batch of stale cleanup candidates.
  try { signal(identity.pid, 'SIGTERM'); return true; }
  catch (error) { if (error.code === 'ESRCH') return false; throw error; }
}
module.exports = { builtPaths, processes, launchedRoot, descendants, signalIfCurrent };

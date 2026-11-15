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
    const script = "ConvertTo-Json -Compress -InputObject @(Get-CimInstance Win32_Process | ForEach-Object { [pscustomobject]@{ pid = [int]$_.ProcessId; ppid = [int]$_.ParentProcessId; start = $_.CreationDate.ToString('o'); command = $_.CommandLine } })";
    return JSON.parse(run('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], { encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 }).trim() || '[]');
  }
  return run('ps', ['-axo', 'pid=,ppid=,lstart=,command='], { encoding: 'utf8' }).split('\n').map(line => {
    const m = line.match(/^\s*(\d+)\s+(\d+)\s+(.{24})\s+(.*)$/);
    return m && { pid: +m[1], ppid: +m[2], start: m[3], command: m[4] };
  }).filter(Boolean);
}
module.exports = { builtPaths, processes };

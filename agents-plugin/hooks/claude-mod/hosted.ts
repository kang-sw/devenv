/**
 * Hosted-session mode: a session the dashboard daemon spawned and feeds. The
 * daemon writes every input (dashboard messages, mailbox mail) as the
 * session's user turns and reads its output itself, so in this mode the mod
 * runs no mailbox wait (and with it no board-inbox drain and no interrupt
 * handling) and sends no dashboard feed, and the manual feature adopts the
 * daemon-minted key in place of minting one. Everything else (manual
 * delivery, preload, the compaction lever and advisory, the band) runs as
 * usual.
 *
 * The daemon sets three variables on the harness it spawns: the flag, the
 * key, and the path of the session's registration file, whose `host_pid` is
 * that harness process. Every process below the harness inherits them (a
 * second harness the session starts from its shell, say), so the mode holds
 * only in the process the registration names: see resolveHostedMode. The
 * same variables gate the Pi extension and ws-mcp.
 */

/** Set to a truthy value (`1`, `true`, `yes`, `on`) by the daemon on a session it hosts. */
export const HOSTED_FLAG_ENV = 'WS_HOSTED_SESSION'

/** The lead session key the daemon minted for the session it hosts. */
export const HOSTED_KEY_ENV = 'WS_HOSTED_SESSION_KEY'

/** The session's registration file, which names the hosted harness process as `host_pid`. */
export const HOSTED_ROW_ENV = 'WS_HOSTED_SESSION_ROW'

/**
 * `off`: the mod runs as everywhere else (no flag, no registration path, or
 * a registration that names another process). `hosted`: this process is the
 * one the daemon hosts. `missing-key`: the flag without a key, and
 * `unconfirmed`: a registration that never named a harness process, or a
 * process id that could not be read; both gate nothing (the mod mints and
 * waits as everywhere else) and only warn.
 */
export type HostedMode = { mode: 'off' } | { mode: 'hosted'; key: string } | { mode: 'missing-key' } | { mode: 'unconfirmed'; reason: string }

/** The truthy spellings ws-mcp's own env flags accept. */
export function flagOn(value: string | undefined): boolean {
  return ['1', 'true', 'yes', 'on'].includes((value ?? '').trim().toLowerCase())
}

/** The mode the flag and the key select, before the registration is read. */
export function hostedMode(flag: string | undefined, key: string | undefined): HostedMode {
  if (!flagOn(flag)) return { mode: 'off' }
  const trimmed = (key ?? '').trim()
  return trimmed ? { mode: 'hosted', key: trimmed } : { mode: 'missing-key' }
}

/**
 * What a registration file's text says of the process `pid`: `mine` when its
 * `host_pid` is that process, `other` when it is another one, and `pending`
 * when it names none yet (zero or absent, or the text is missing or does not
 * parse): the daemon writes the PID right after it starts the harness.
 */
export function rowVerdict(text: string | undefined, pid: number): 'mine' | 'other' | 'pending' {
  let hostPid: unknown
  try {
    hostPid = (JSON.parse(text ?? '') as { host_pid?: unknown } | null)?.host_pid
  } catch {
    return 'pending'
  }
  if (typeof hostPid !== 'number' || !Number.isInteger(hostPid) || hostPid <= 0) return 'pending'
  return hostPid === pid ? 'mine' : 'other'
}

/** How often, and for how long, a registration that names no harness yet is read again. */
export const ROW_POLL_MS = 100
export const ROW_WAIT_MS = 3_000

/** What resolveHostedMode reads the world through. */
export type HostedProbe = {
  /** This harness process's id; undefined (or a rejection) when it cannot be told. */
  pid: () => Promise<number | undefined>
  /** The text of the file at `path`; a rejection reads as not there yet. */
  readRow: (path: string) => Promise<string>
  sleep: (ms: number) => Promise<void>
}

/**
 * The mode of this process, resolved once per process by the caller:
 *
 * - flag off: `off`; flag without a key: `missing-key`.
 * - no registration path: `off`.
 * - the registration names this process: `hosted`.
 * - it names another process: `off`, silently (the variables were inherited).
 * - it names none: read again every ROW_POLL_MS for ROW_WAIT_MS, then
 *   `unconfirmed`.
 *
 * The process id is read only once the three variables are set, so an
 * ordinary session runs nothing for this.
 */
export async function resolveHostedMode(vars: { flag?: string; key?: string; row?: string }, probe: HostedProbe): Promise<HostedMode> {
  const selected = hostedMode(vars.flag, vars.key)
  if (selected.mode !== 'hosted') return selected
  const row = (vars.row ?? '').trim()
  if (!row) return { mode: 'off' }
  const pid = await probe.pid().catch(() => undefined)
  if (pid === undefined) return { mode: 'unconfirmed', reason: "this process's id could not be read" }
  for (let waited = 0; ; waited += ROW_POLL_MS) {
    const verdict = rowVerdict(await probe.readRow(row).catch(() => undefined), pid)
    if (verdict === 'mine') return selected
    if (verdict === 'other') return { mode: 'off' }
    if (waited >= ROW_WAIT_MS) return { mode: 'unconfirmed', reason: `${row} named no harness process within ${ROW_WAIT_MS / 1000} s` }
    await probe.sleep(ROW_POLL_MS)
  }
}

/**
 * What the harness-id probe runs. It prints its parent's id and, on Windows
 * when the process table can be read, that parent's own parent and image name
 * after it (`<pid> <parent> <image>`); where it cannot, just `<pid>`.
 */
const HARNESS_PID_SCRIPT = [
  'import os, sys',
  'pid = os.getppid()',
  'out = str(pid)',
  'try:',
  "    if sys.platform == 'win32':",
  '        import ctypes',
  '        from ctypes import wintypes as w',
  '        class E(ctypes.Structure):',
  "            _fields_ = [('dwSize', w.DWORD), ('cntUsage', w.DWORD), ('pid', w.DWORD), ('heap', ctypes.c_size_t), ('module', w.DWORD), ('threads', w.DWORD), ('parent', w.DWORD), ('pri', w.LONG), ('flags', w.DWORD), ('image', w.WCHAR * 260)]",
  "        k = ctypes.WinDLL('kernel32')",
  '        k.CreateToolhelp32Snapshot.argtypes = [w.DWORD, w.DWORD]',
  '        k.CreateToolhelp32Snapshot.restype = w.HANDLE',
  '        for f in (k.Process32FirstW, k.Process32NextW):',
  '            f.argtypes = [w.HANDLE, ctypes.POINTER(E)]',
  '            f.restype = w.BOOL',
  '        k.CloseHandle.argtypes = [w.HANDLE]',
  '        snap = k.CreateToolhelp32Snapshot(2, 0)',
  '        if snap is not None and snap != ctypes.c_void_p(-1).value:',
  '            e = E()',
  '            e.dwSize = ctypes.sizeof(E)',
  '            ok = k.Process32FirstW(snap, ctypes.byref(e))',
  '            while ok and e.pid != pid:',
  '                ok = k.Process32NextW(snap, ctypes.byref(e))',
  '            k.CloseHandle(snap)',
  '            if ok:',
  "                out = '%d %d %s' % (pid, e.parent, e.image)",
  'except Exception:',
  '    pass',
  'print(out)',
].join('\n')

/**
 * The argv that prints the id of the process that ran it (the harness, for a
 * host command), plus on Windows that process's parent and image name.
 *
 * Claude Code on Windows runs a host command through `cmd.exe`, which cuts the
 * command line at its first newline, so the script travels as one line: its
 * hex, which also needs no quoting, decoded and run by a one-line `-c`. The
 * wrapper that is the printed id's image is gone by the time anything else
 * could ask about it, so the same process reads the table it came from.
 */
export const HARNESS_PID_ARGV = [
  'python3',
  '-c',
  `exec(bytes.fromhex('${Array.from(HARNESS_PID_SCRIPT, (c) => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')}').decode())`,
] as const

/** The process id a HARNESS_PID_ARGV run printed alone, or undefined for anything else. */
export function parsePid(stdout: string): number | undefined {
  const text = stdout.trim()
  if (!/^[0-9]+$/.test(text)) return undefined
  const pid = Number(text)
  return Number.isSafeInteger(pid) && pid > 0 ? pid : undefined
}

/**
 * The harness's id from what a HARNESS_PID_ARGV run printed, or undefined for
 * anything else: the printed id, or its parent when the printed id is a
 * Windows `cmd.exe` (the shell Claude Code runs a host command through).
 * Exactly one level is skipped, so a second harness the session starts from
 * its shell still tells itself from the hosted one, and a bare id (not
 * Windows, or a failed lookup) is the id. The same rule as ws-mcp's
 * `wrappedHarnessPID`.
 */
export function parseHarnessPid(stdout: string): number | undefined {
  const match = /^([0-9]+)(?: ([0-9]+) (.+))?$/.exec(stdout.trim())
  const own = parsePid(match?.[1] ?? '')
  if (own === undefined || match?.[2] === undefined) return own
  return match[3]?.toLowerCase() === 'cmd.exe' ? parsePid(match[2]) ?? own : own
}

/** The transcript warning for a flag set without a key; `plugin` is the engine-reported name. */
export function missingKeyWarning(plugin: string): string {
  return `[${plugin}] ${HOSTED_FLAG_ENV} is set but ${HOSTED_KEY_ENV} is missing or empty: running as an ordinary session (own key, mailbox wait, dashboard feed).`
}

/** The transcript warning for a flag, key, and registration path this process could not be matched against. */
export function unconfirmedWarning(plugin: string, reason: string): string {
  return `[${plugin}] ${HOSTED_FLAG_ENV} is set but ${reason}: running as an ordinary session (own key, mailbox wait, dashboard feed).`
}

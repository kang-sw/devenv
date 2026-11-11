import { expect, test } from 'claude-code/testing'

import { ROW_POLL_MS, ROW_WAIT_MS, flagOn, hostedMode, missingKeyWarning, parsePid, resolveHostedMode, rowVerdict, unconfirmedWarning, type HostedProbe } from './hosted'

test('the flag takes the truthy spellings ws-mcp accepts, case and whitespace aside', () => {
  for (const value of ['1', 'true', 'TRUE', ' yes ', 'on']) expect(flagOn(value)).toBe(true)
  for (const value of [undefined, '', '0', 'false', 'off', 'no', 'hosted']) expect(flagOn(value)).toBe(false)
})

test('hosted only with the flag and a non-empty key; the flag without one is its own mode; no flag is off whatever the key', () => {
  expect(hostedMode('1', ' hosted-key ')).toEqual({ mode: 'hosted', key: 'hosted-key' })
  expect(hostedMode('1', undefined)).toEqual({ mode: 'missing-key' })
  expect(hostedMode('on', '  ')).toEqual({ mode: 'missing-key' })
  expect(hostedMode(undefined, 'hosted-key')).toEqual({ mode: 'off' })
  expect(hostedMode('0', 'hosted-key')).toEqual({ mode: 'off' })
})

test('the missing-key warning names both variables and the plugin', () => {
  const text = missingKeyWarning('ws')
  expect(text.startsWith('[ws] ')).toBe(true)
  expect(text).toContain('WS_HOSTED_SESSION ')
  expect(text).toContain('WS_HOSTED_SESSION_KEY')
})

const row = (hostPid?: number) => JSON.stringify({ schema_version: 1, reply_id: 'r', hosted: true, ...(hostPid === undefined ? {} : { host_pid: hostPid }) })

test('a registration names this process, another one, or none yet', () => {
  expect(rowVerdict(row(4242), 4242)).toBe('mine')
  expect(rowVerdict(row(5555), 4242)).toBe('other')
  for (const text of [row(0), row(), undefined, '', '{', 'null', '{"host_pid":"4242"}', '{"host_pid":-1}', '{"host_pid":1.5}']) expect(rowVerdict(text, 4242)).toBe('pending')
})

test('a process id is digits alone', () => {
  expect(parsePid('4242\n')).toBe(4242)
  for (const text of ['', 'Python 3.12.0\n', '0\n', '-1\n', '42 43\n', '4.2\n']) expect(parsePid(text)).toBe(undefined)
})

const VARS = { flag: '1', key: ' hosted-key ', row: ' /state/registration.json ' }

/** A probe over a registration a test rewrites, counting what the resolution ran. */
function probe(opts: { pid?: number | 'throws'; text?: string | undefined }) {
  const p = { text: opts.text, pidReads: 0, reads: [] as string[], slept: 0, onSleep: (_slept: number) => {} }
  const hooks: HostedProbe = {
    pid: async () => {
      p.pidReads += 1
      if (opts.pid === 'throws') throw new Error('no host commands')
      return opts.pid
    },
    readRow: async (path) => {
      p.reads.push(path)
      if (p.text === undefined) throw new Error('ENOENT')
      return p.text
    },
    sleep: async (ms) => {
      p.slept += ms
      p.onSleep(p.slept)
    },
  }
  return { p, hooks }
}

test('hosted only in the process the registration names', async () => {
  const mine = probe({ pid: 4242, text: row(4242) })
  expect(await resolveHostedMode(VARS, mine.hooks)).toEqual({ mode: 'hosted', key: 'hosted-key' })
  expect(mine.p.reads).toEqual(['/state/registration.json'])
  expect(mine.p.slept).toBe(0)
})

test('a registration that names another process is an ordinary session, at once and with no warning', async () => {
  const other = probe({ pid: 5555, text: row(4242) })
  expect(await resolveHostedMode(VARS, other.hooks)).toEqual({ mode: 'off' })
  expect(other.p.slept).toBe(0)
})

test('no registration path is an ordinary session, and nothing is run to find out', async () => {
  for (const rowPath of [undefined, '', '  ']) {
    const none = probe({ pid: 4242, text: row(4242) })
    expect(await resolveHostedMode({ ...VARS, row: rowPath }, none.hooks)).toEqual({ mode: 'off' })
    expect(none.p.pidReads).toBe(0)
    expect(none.p.reads).toEqual([])
  }
})

test('the flag off or without a key never reads the process id or the registration', async () => {
  const off = probe({ pid: 4242, text: row(4242) })
  expect(await resolveHostedMode({ ...VARS, flag: '0' }, off.hooks)).toEqual({ mode: 'off' })
  expect(await resolveHostedMode({ ...VARS, key: ' ' }, off.hooks)).toEqual({ mode: 'missing-key' })
  expect(off.p.pidReads).toBe(0)
  expect(off.p.reads).toEqual([])
})

test('a registration that names no process yet is read again until it does', async () => {
  for (const pending of [row(0), row(), undefined, '{']) {
    for (const [written, want] of [
      [row(4242), { mode: 'hosted', key: 'hosted-key' }],
      [row(5555), { mode: 'off' }],
    ] as const) {
      const late = probe({ pid: 4242, text: pending })
      late.p.onSleep = (slept) => {
        if (slept >= 5 * ROW_POLL_MS) late.p.text = written
      }
      expect(await resolveHostedMode(VARS, late.hooks)).toEqual(want)
      expect(late.p.slept).toBe(5 * ROW_POLL_MS)
      expect(late.p.reads.length).toBe(6)
    }
  }
})

test('a registration that never names a process is an ordinary session after the wait, with a reason to warn with', async () => {
  const never = probe({ pid: 4242, text: row(0) })
  const got = await resolveHostedMode(VARS, never.hooks)
  expect(got.mode).toBe('unconfirmed')
  expect(never.p.slept).toBe(ROW_WAIT_MS)
  const text = unconfirmedWarning('ws', got.mode === 'unconfirmed' ? got.reason : '')
  expect(text.startsWith('[ws] WS_HOSTED_SESSION is set but /state/registration.json named no harness process within 3 s')).toBe(true)
})

test('a process id that cannot be read is an ordinary session with a warning, and the registration is not read', async () => {
  for (const pid of [undefined, 'throws'] as const) {
    const blind = probe({ pid, text: row(4242) })
    const got = await resolveHostedMode(VARS, blind.hooks)
    expect(got.mode).toBe('unconfirmed')
    expect(blind.p.reads).toEqual([])
  }
})

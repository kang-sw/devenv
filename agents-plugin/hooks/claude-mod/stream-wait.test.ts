import { expect, test } from 'claude-code/testing'
import { createMailboxWait, observeRecv } from './register'
import { createDrainLedger, createWatcher, recvResultMail, type DrainLedger, type WatcherEdges } from './watcher'

const tick = async () => { for (let i = 0; i < 30; i++) await Promise.resolve() }

function processes(ledger?: DrainLedger) {
  type Step = { done: false; value: { stream: 'stdout' | 'stderr'; text: string } } | { done: true; value: { code: number; signal: null } }
  const children: { argv: string[]; closed: boolean; send: (text: string, stream?: 'stdout' | 'stderr') => void; exit: (code: number) => void }[] = []
  const spawn = (argv: string[]) => {
    const queued: Step[] = []
    let pending: ((step: Step) => void) | undefined
    const push = (step: Step) => {
      if (pending) { const resolve = pending; pending = undefined; resolve(step) }
      else queued.push(step)
    }
    const child = {
      argv, closed: false,
      send: (text: string, stream: 'stdout' | 'stderr' = 'stdout') => push({ done: false, value: { stream, text } }),
      exit: (code: number) => push({ done: true, value: { code, signal: null } }),
    }
    children.push(child)
    return {
      next: async () => queued.shift() ?? new Promise<Step>((resolve) => { pending = resolve }),
      return: async () => { child.closed = true; child.exit(130); return { done: true as const, value: { code: 130, signal: null } } },
    }
  }
  const wait = createMailboxWait(spawn as never, '/launcher.py', () => {}, ledger)
  const binding = new AbortController()
  const arm = (board = false, interrupt = false, signal = new AbortController().signal) => wait('k', undefined, { board, interrupt }, signal, binding.signal)
  return { children, wait, binding, arm }
}

test('one child handles split and coalesced JSON lines across wakes without timeout restarts', async () => {
  const p = processes()
  const first = p.arm(true, true)
  expect(p.children[0]!.argv).toContain('--stream')
  p.children[0]!.send('{"kinds":["bo')
  await tick()
  p.children[0]!.send('ard"]}\n{"kinds":["mail","interrupt"]}\n')
  expect(await first).toBe('board')
  expect(await p.arm(true, true)).toEqual({ interrupt: true, also: 'mail' })
  const next = p.arm(true, true)
  await tick() // no output on daemon timeout; the same child remains attached
  expect(p.children.length).toBe(1)
  p.binding.abort()
  expect(await next).toBe('stopped')
  expect(p.children[0]!.closed).toBe(true)
})

test('board flag changes replace the stream and binding abort closes it between arms', async () => {
  const p = processes()
  const first = p.arm()
  p.children[0]!.send('{"kinds":["mail"]}\n')
  expect(await first).toBe('mail')
  const second = p.arm(true)
  expect(p.children[0]!.closed).toBe(true)
  expect(p.children.length).toBe(2)
  p.children[1]!.send('{"kinds":["board"]}\n')
  expect(await second).toBe('board')
  p.binding.abort() // no arm is pending (e.g. notice mode waits for model recv)
  expect(p.children[1]!.closed).toBe(true)
})

test('per-arm cancellation restarts flags without ending the binding', async () => {
  const p = processes()
  const arm = new AbortController()
  const first = p.arm(false, false, arm.signal)
  arm.abort()
  expect(await first).toBe('stopped')
  await tick() // the cancelled reader must not mistake exit 130 for skew
  const second = p.arm(true)
  expect(p.children.length).toBe(2)
  expect(p.children[1]!.argv).toContain('--stream')
  p.children[1]!.send('{"kinds":["board"]}\n')
  expect(await second).toBe('board')
  p.binding.abort()
})

for (const [diagnostic, output, code] of [
  ['', 'stderr', 2],
  ['flag provided but not defined: -stream\nUsage: mailbox wait', 'stderr', 2],
  ['Usage: mailbox wait\n', 'stdout', 0],
] as const) {
  test(`a rejection before the first arrival selects one-shot for the whole session (${diagnostic || 'nonzero'})`, async () => {
    const p = processes()
    const first = p.arm()
    if (diagnostic) p.children[0]!.send(diagnostic, output)
    p.children[0]!.exit(code)
    await tick()
    expect(p.children.length).toBe(2)
    expect(p.children[1]!.argv.includes('--stream')).toBe(false)
    p.children[1]!.exit(0)
    expect(await first).toBe('mail')
    const second = p.arm(true)
    expect(p.children[2]!.argv.includes('--stream')).toBe(false)
    p.children[2]!.send('{"kinds":["board"]}')
    p.children[2]!.exit(0)
    expect(await second).toBe('board')
    p.binding.abort()
  })
}

test('a stream failing after an arrival is retried as a stream, not downgraded', async () => {
  const p = processes()
  const first = p.arm()
  p.children[0]!.send('{"kinds":["mail"],"reply":[{"content":"unknown flag; Usage: example"}]}\n')
  expect(await first).toBe('mail')
  const failed = p.arm()
  p.children[0]!.exit(130)
  expect(await failed).toBe('error')
  const retried = p.arm()
  expect(p.children[1]!.argv).toContain('--stream')
  p.binding.abort()
  expect(await retried).toBe('stopped')
})

test('watcher drains two arrivals through one streaming child and stops it while notice-suspended', async () => {
  for (const mode of ['direct', 'notice'] as const) {
    const p = processes()
    let drains = 0
    const prompts: string[] = []
    const edges: WatcherEdges = {
      mode, label: 'test mailbox', wait: p.wait,
      drain: async () => { drains++; return { kind: 'mail', mail: [{ content: 'hello' }] } },
      submit: async (items) => { prompts.push(...items.map((i) => i.text)) },
      append: async () => {}, turnRunning: () => false, recvTool: () => 'recv',
      sleep: async () => {}, notice: () => {}, record: () => {}, log: () => {},
    }
    const watcher = createWatcher(edges)
    watcher.bind('k')
    p.children[0]!.send('{"kinds":["mail"]}\n')
    await tick()
    expect(prompts.length).toBe(1)
    if (mode === 'notice') watcher.recvCompleted('k')
    await tick()
    p.children[0]!.send('{"kinds":["mail"]}\n')
    await tick()
    expect(prompts.length).toBe(2)
    expect(drains).toBe(mode === 'direct' ? 2 : 0)
    expect(p.children.length).toBe(1)
    watcher.stop('session ended')
    expect(p.children[0]!.closed).toBe(true)
  }
})

const M1 = { from: 'alice', content: 'one', sent_at: '2026-11-13T10:00:00Z' }
const M2 = { from: 'bob', content: 'two\nlines', sent_at: '2026-11-13T10:00:01Z' }
const M3 = { from: 'carol', content: 'three', sent_at: '2026-11-13T10:00:05Z' }
const arrival = (named: object[], kinds = ['mail']) => JSON.stringify({ timed_out: false, unread: named.length, board: 0, interrupt: 0, kinds, named, reply: [] }) + '\n'

test('a buffered arrival the drain already received does not wake; mail after the drain still does', async () => {
  const ledger = createDrainLedger()
  const p = processes(ledger)
  const first = p.arm()
  // M2 arrives before the drain: its line is buffered behind the first wake.
  p.children[0]!.send(arrival([M1]) + arrival([M2]))
  expect(await first).toBe('mail')
  ledger.record([M1, M2]) // the drain received both
  let woke: unknown
  const second = p.arm().then((o) => (woke = o))
  await tick()
  expect(woke).toBe(undefined)
  // Mail after the drain and before this arm reads it wakes at once.
  p.children[0]!.send(arrival([M3]))
  await second
  expect(woke).toBe('mail')
  expect(p.children.length).toBe(1)
  p.binding.abort()
})

test('a partly received arrival still wakes, and a received mail+board line keeps its board kind', async () => {
  const ledger = createDrainLedger()
  const p = processes(ledger)
  const first = p.arm(true)
  p.children[0]!.send(arrival([M1]))
  expect(await first).toBe('mail')
  ledger.record([M1, M2])
  const second = p.arm(true)
  p.children[0]!.send(arrival([M2, M3]))
  expect(await second).toBe('mail')
  ledger.record([M1])
  const third = p.arm(true)
  p.children[0]!.send(arrival([M1], ['mail', 'board']))
  expect(await third).toBe('board')
  p.binding.abort()
})

test('a new stream child forgets earlier drains', async () => {
  const ledger = createDrainLedger()
  const p = processes(ledger)
  const first = p.arm()
  p.children[0]!.send(arrival([M1]))
  expect(await first).toBe('mail')
  ledger.record([M1])
  const second = p.arm(true) // a flag change replaces the child
  expect(p.children.length).toBe(2)
  p.children[1]!.send(arrival([M1]))
  expect(await second).toBe('mail')
  p.binding.abort()
})

test('the line that woke a drain is not recorded, so an identical later message still wakes', () => {
  const ledger = createDrainLedger()
  const again = { ...M1, from: 'bob' } // same second, same text, sent after the drain
  expect(ledger.filter(arrival([M1]))).toBe(arrival([M1])) // wakes the drain
  ledger.record([M1])
  expect(ledger.filter(arrival([again]))).toBe(arrival([again]))
})

test('an arrival the drain did not get clears the ledger', () => {
  const ledger = createDrainLedger()
  ledger.record([M1]) // received without a line, e.g. before this child started
  expect(ledger.filter(arrival([M3]))).toBe(arrival([M3]))
  expect(ledger.filter(arrival([M1]))).toBe(arrival([M1]))
})

test('recv results give their envelopes in both forms', () => {
  expect(recvResultMail(JSON.stringify([M1, M2]))).toEqual([M1, M2])
  expect(recvResultMail(`[${M1.sent_at}] alice: one\n[${M2.sent_at}] bob: two\nlines\n`)).toEqual([
    { sent_at: M1.sent_at, content: 'one' },
    { sent_at: M2.sent_at, content: 'two\nlines' },
  ])
  expect(recvResultMail('no unread mail\n')).toEqual([])
})

test('watcher: a second arrival before recv gets no extra wake, in both modes', async () => {
  for (const mode of ['direct', 'notice'] as const) {
    const ledger = createDrainLedger()
    const p = processes(ledger)
    let queued = [M1, M2]
    const prompts: string[] = []
    const edges: WatcherEdges = {
      mode, label: 'test mailbox', wait: p.wait,
      // As register.ts wires it: the direct drain records what recv returned.
      drain: async () => { const mail = queued; queued = []; ledger.record(mail); return { kind: 'mail', mail } },
      submit: async (items) => { prompts.push(...items.map((i) => i.text)) },
      append: async () => {}, turnRunning: () => false, recvTool: () => 'mcp__ws__mailbox_recv',
      sleep: async () => {}, notice: () => {}, record: () => {}, log: () => {},
    }
    const watcher = createWatcher(edges)
    watcher.bind('k')
    p.children[0]!.send(arrival([M1]) + arrival([M2]))
    await tick()
    expect(prompts.length).toBe(1)
    if (mode === 'notice') {
      // The model's own recv, in its text form, drains both.
      const text = queued.map((m) => `[${m.sent_at}] ${m.from}: ${m.content}\n`).join('')
      queued = []
      observeRecv(watcher, { tool: 'mcp__ws__mailbox_recv', originPlugin: 'engine', sessionKey: 'k' }, { text } as never, 'mcp__ws__mailbox_recv', () => {}, ledger)
    }
    await tick()
    expect(prompts.length).toBe(1)
    queued = [M3]
    p.children[0]!.send(arrival([M3]))
    await tick()
    expect(prompts.length).toBe(2)
    watcher.stop('session ended')
  }
})

import { describe, expect, test } from 'claude-code/testing'

import { checkWatchCall, recvResultToMail, waitArgv } from '../hooks/register'
import { createWatcher, formatBatch, mapWaitExit, type MailboxEnvelope, type WaitOutcome, type WatcherEdges } from '../hooks/watcher'

type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void }
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}
const tick = async (n = 10) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

/** Fake edges: each wait is a deferred the test resolves; drains come from a queue. */
function harness() {
  const waits: { key: string; signal: AbortSignal; d: Deferred<WaitOutcome> }[] = []
  const drains: string[] = []
  const drainQueue: (MailboxEnvelope[] | Error)[] = []
  const submitted: string[] = []
  const sleeps: { ms: number; d: Deferred<void> }[] = []
  const edges: WatcherEdges = {
    wait: (key, signal) => {
      const d = deferred<WaitOutcome>()
      signal.addEventListener('abort', () => d.resolve('stopped'), { once: true })
      waits.push({ key, signal, d })
      return d.promise
    },
    drain: async (key) => {
      drains.push(key)
      const next = drainQueue.shift() ?? []
      if (next instanceof Error) throw next
      return next
    },
    submit: async (text) => {
      submitted.push(text)
    },
    sleep: (ms) => {
      const d = deferred<void>()
      sleeps.push({ ms, d })
      return d.promise
    },
    status: () => {},
    log: () => {},
  }
  return { edges, waits, drains, drainQueue, submitted, sleeps }
}

describe('watcher', () => {
  test('bind spawns one wait for the key', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    expect(w.bind('k1')).toBe('bound')
    await tick()
    expect(h.waits.length).toBe(1)
    expect(h.waits[0]!.key).toBe('k1')
  })

  test('wait exit 0 -> one recv drain -> exactly one prompt with the whole batch -> re-arm', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.drainQueue.push([
      { reply_to: 'id:a', content: 'first', sent_at: 't1' },
      { from: 'peer@machine', content: 'second', sent_at: 't2' },
    ])
    h.waits[0]!.d.resolve('mail')
    await tick(30)
    expect(h.drains).toEqual(['k1'])
    expect(h.submitted.length).toBe(1)
    expect(h.submitted[0]).toContain('2 message(s)')
    expect(h.submitted[0]).toContain('first')
    expect(h.submitted[0]).toContain('second')
    expect(h.waits.length).toBe(2) // re-armed immediately after the drain
  })

  test('same key again is a no-op', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    expect(w.bind('k1')).toBe('unchanged')
    await tick()
    expect(h.waits.length).toBe(1)
    expect(h.waits[0]!.signal.aborted).toBe(false)
  })

  test('a new key aborts the old wait and rebinds', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    expect(w.bind('k2')).toBe('rebound')
    await tick()
    expect(h.waits[0]!.signal.aborted).toBe(true)
    expect(h.waits.length).toBe(2)
    expect(h.waits[1]!.key).toBe('k2')
    expect(w.current()).toBe('k2')
  })

  test('empty drain backs off instead of hot-looping', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('mail') // drain queue empty -> []
    await tick(30)
    expect(h.drains.length).toBe(1)
    expect(h.submitted.length).toBe(0)
    expect(h.sleeps.length).toBe(1)
    expect(h.sleeps[0]!.ms).toBeGreaterThan(0)
    expect(h.waits.length).toBe(1) // not re-armed until the backoff elapses
    h.sleeps[0]!.d.resolve()
    await tick(30)
    expect(h.waits.length).toBe(2)
  })

  test('wait error and drain error back off', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('error')
    await tick(30)
    expect(h.sleeps.length).toBe(1)
    expect(h.waits.length).toBe(1)
    h.sleeps[0]!.d.resolve()
    await tick(30)
    h.drainQueue.push(new Error('boom'))
    h.waits[1]!.d.resolve('mail')
    await tick(30)
    expect(h.sleeps.length).toBe(2)
    expect(h.submitted.length).toBe(0)
  })

  test('timeout re-arms without draining', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('timeout')
    await tick(30)
    expect(h.drains.length).toBe(0)
    expect(h.waits.length).toBe(2)
  })

  test('stop (session.end clear) ends the loop', async () => {
    const h = harness()
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    w.stop('session.end clear')
    await tick(30)
    expect(h.waits[0]!.signal.aborted).toBe(true)
    expect(h.waits.length).toBe(1)
    expect(w.current()).toBe(undefined)
  })
})

describe('pure helpers', () => {
  test('subagent calls are rejected; main loop calls bind', () => {
    expect(checkWatchCall('agent-123', 'k1')).toEqual({
      deny: 'mailbox_watch binds the main session only; called from subagent agent-123.',
    })
    expect(checkWatchCall(undefined, '  k1 ')).toEqual({ key: 'k1' })
    expect('deny' in checkWatchCall(undefined, '')).toBe(true)
  })

  test('exit-code mapping matches pi', () => {
    expect(mapWaitExit(0, false)).toBe('mail')
    expect(mapWaitExit(3, false)).toBe('timeout')
    expect(mapWaitExit(130, false)).toBe('error')
    expect(mapWaitExit(1, false)).toBe('error')
    expect(mapWaitExit(null, true)).toBe('stopped')
  })

  test('recv result parsing and error mapping', () => {
    const text = '[{"reply_to":"id:x","content":"hi","sent_at":"t"}]'
    expect(recvResultToMail({ result: undefined, text } as never)).toEqual([{ reply_to: 'id:x', content: 'hi', sent_at: 't' }])
    expect(recvResultToMail({ result: { content: [{ type: 'text', text }] } } as never).length).toBe(1)
    expect(() => recvResultToMail({ deny: 'no' } as never)).toThrow('recv denied')
    expect(() => recvResultToMail({ isError: true, result: 'x', text: 'unknown_session' } as never)).toThrow('unknown_session')
    expect(recvResultToMail({ result: undefined, text: 'no unread mail\n' } as never)).toEqual([])
  })

  test('argv and batch format', () => {
    expect(waitArgv('k1').slice(1)).toEqual([
      '/home/swkang/.claude/plugins/ws-plugin/ws/bin/ws-mcp-launcher.py',
      'mailbox', 'wait', '--session-key', 'k1', '--timeout', '0', '--format', 'json',
    ])
    expect(formatBatch('k', [{ content: 'c' }])).toContain('from (unknown)')
  })
})

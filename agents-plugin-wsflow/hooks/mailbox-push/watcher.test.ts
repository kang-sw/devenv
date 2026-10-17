import { describe, expect, test } from 'claude-code/testing'

import { checkWatchCall, classifyRecv, drainModeFor, isModelRecvCompletion, launcherPath, mcpToolName } from './register'
import {
  createWatcher,
  formatBatch,
  formatNotice,
  mapWaitExit,
  nextBackoff,
  type DrainMode,
  type DrainResult,
  type WaitOutcome,
  type WatcherEdges,
} from './watcher'

type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void }
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}
const tick = async (n = 30) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

/** Fake edges: each wait is a deferred the test resolves; drains come from a queue. */
function harness(mode: DrainMode) {
  const waits: { key: string; signal: AbortSignal; d: Deferred<WaitOutcome> }[] = []
  const drains: string[] = []
  const drainQueue: DrainResult[] = []
  const submitted: string[] = []
  const submitFailures: Error[] = []
  const sleeps: { ms: number; d: Deferred<void> }[] = []
  const notices: string[] = []
  const edges: WatcherEdges = {
    mode,
    label: 'pkg mailbox',
    wait: (key, signal) => {
      const d = deferred<WaitOutcome>()
      signal.addEventListener('abort', () => d.resolve('stopped'), { once: true })
      waits.push({ key, signal, d })
      return d.promise
    },
    drain: async (key) => {
      drains.push(key)
      return drainQueue.shift() ?? { kind: 'mail', mail: [] }
    },
    submit: async (text) => {
      const failure = submitFailures.shift()
      if (failure) throw failure
      submitted.push(text)
    },
    recvTool: () => 'mcp__plugin_pkg_pkg__mailbox_recv',
    sleep: (ms, signal) => {
      const d = deferred<void>()
      signal.addEventListener('abort', () => d.resolve(), { once: true })
      sleeps.push({ ms, d })
      return d.promise
    },
    notice: (text) => notices.push(text),
    log: () => {},
  }
  return { edges, waits, drains, drainQueue, submitted, submitFailures, sleeps, notices }
}

describe('direct mode', () => {
  test('wait exit 0 -> one drain -> exactly one prompt with the whole batch -> re-arm', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    expect(w.bind('k1')).toBe('bound')
    await tick()
    h.drainQueue.push({ kind: 'mail', mail: [{ reply_to: 'id:a', content: 'first', sent_at: 't1' }, { from: 'p@machine', content: 'second' }] })
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.drains).toEqual(['k1'])
    expect(h.submitted.length).toBe(1)
    expect(h.submitted[0]).toContain('mail from id:a (t1):\nfirst')
    expect(h.submitted[0]).toContain('mail from p@machine:\nsecond')
    expect(h.waits.length).toBe(2)
  })

  test('an empty drain backs off instead of hot-looping', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.submitted.length).toBe(0)
    expect(h.sleeps.map((s) => s.ms)).toEqual([2_000])
    expect(h.waits.length).toBe(1)
    h.sleeps[0]!.d.resolve()
    await tick()
    expect(h.waits.length).toBe(2)
  })

  test('a failed drain backs off and retries; the mail stays queued', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.drainQueue.push({ kind: 'failed', reason: 'server gone' })
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.submitted.length).toBe(0)
    expect(h.sleeps.map((s) => s.ms)).toEqual([1_000])
    h.sleeps[0]!.d.resolve()
    await tick()
    h.drainQueue.push({ kind: 'failed', reason: 'server gone' })
    h.waits[1]!.d.resolve('mail')
    await tick()
    expect(h.sleeps.map((s) => s.ms)).toEqual([1_000, 2_000])
  })

  test('a refused drain submits one notice and suspends until recvCompleted', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.drainQueue.push({ kind: 'refused', reason: 'permission' })
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.submitted).toEqual([formatNotice('pkg mailbox', 'mcp__plugin_pkg_pkg__mailbox_recv', 'k1')])
    expect(w.isSuspended()).toBe(true)
    expect(h.waits.length).toBe(1)
    w.recvCompleted('k1')
    await tick()
    expect(w.isSuspended()).toBe(false)
    expect(h.waits.length).toBe(2)
  })
})

describe('notice mode', () => {
  test('never drains; one notice per arrival; recvCompleted for another key is ignored', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.drains).toEqual([])
    expect(h.submitted.length).toBe(1)
    w.recvCompleted('k2')
    await tick()
    expect(h.waits.length).toBe(1)
    w.recvCompleted('k1')
    await tick()
    expect(h.waits.length).toBe(2)
    h.waits[1]!.d.resolve('mail')
    await tick()
    expect(h.submitted.length).toBe(2)
  })

  test('a notice that cannot be submitted backs off and re-arms instead of suspending', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.submitFailures.push(new Error('no session'))
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(w.isSuspended()).toBe(false)
    expect(h.sleeps.map((s) => s.ms)).toEqual([1_000])
    h.sleeps[0]!.d.resolve()
    await tick()
    expect(h.waits.length).toBe(2)
  })

  test('a rebind while suspended drops the suspension and arms the new key', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(w.isSuspended()).toBe(true)
    expect(w.bind('k2')).toBe('rebound')
    await tick()
    expect(w.isSuspended()).toBe(false)
    expect(h.waits.length).toBe(2)
    expect(h.waits[1]!.key).toBe('k2')
    w.recvCompleted('k1')
    await tick()
    expect(h.waits.length).toBe(2)
  })
})

describe('lifecycle', () => {
  test('same key is a no-op; a new key aborts the old wait', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    expect(w.bind('k1')).toBe('unchanged')
    expect(w.bind('k2')).toBe('rebound')
    await tick()
    expect(h.waits[0]!.signal.aborted).toBe(true)
    expect(h.waits.map((x) => x.key)).toEqual(['k1', 'k2'])
    expect(w.current()).toBe('k2')
  })

  test('timeout re-arms at once; stop ends the loop', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('timeout')
    await tick()
    expect(h.drains.length).toBe(0)
    expect(h.waits.length).toBe(2)
    w.stop('session cleared')
    await tick()
    expect(h.waits[1]!.signal.aborted).toBe(true)
    expect(h.waits.length).toBe(2)
    expect(w.current()).toBe(undefined)
  })

  test('notices fire on state changes only: armed once per bind, one per failure streak', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('timeout')
    await tick()
    h.waits[1]!.d.resolve('error')
    await tick()
    h.sleeps[0]!.d.resolve()
    await tick()
    h.waits[2]!.d.resolve('error')
    await tick()
    expect(h.sleeps.length).toBe(2)
    expect(h.notices).toEqual(['pkg mailbox: armed k1', 'pkg mailbox: wait error, retrying with backoff'])
  })
})

describe('pure helpers', () => {
  test('drain mode follows the package name', () => {
    expect(drainModeFor('ws')).toBe('direct')
    expect(drainModeFor('wsflow')).toBe('notice')
    expect(drainModeFor('anything-else')).toBe('notice')
  })

  test('names and paths', () => {
    expect(mcpToolName('plugin:ws:ws', 'mailbox_recv')).toBe('mcp__plugin_ws_ws__mailbox_recv')
    expect(mcpToolName('plugin:wsflow:wsflow', 'mailbox_recv')).toBe('mcp__plugin_wsflow_wsflow__mailbox_recv')
    expect(launcherPath('/p/root/')).toBe('/p/root/bin/ws-mcp-launcher.py')
  })

  test('subagent calls are rejected; main loop calls bind', () => {
    expect(checkWatchCall('agent-123', 'k1')).toEqual({ deny: 'mailbox_watch binds the main session only; called from subagent agent-123.' })
    expect(checkWatchCall(undefined, '  k1 ')).toEqual({ key: 'k1' })
    expect('deny' in checkWatchCall(undefined, '')).toBe(true)
  })

  test('exit-code mapping matches the pi adapter', () => {
    expect(mapWaitExit(0, false)).toBe('mail')
    expect(mapWaitExit(3, false)).toBe('timeout')
    expect(mapWaitExit(130, false)).toBe('error')
    expect(mapWaitExit(1, false)).toBe('error')
    expect(mapWaitExit(null, true)).toBe('stopped')
    expect([1, 2, 3, 4, 5, 6, 7, 8].reduce((b) => nextBackoff(b), 0)).toBe(60_000)
  })

  test('recv results classify as mail, refusal, or failure', () => {
    const text = '[{"reply_to":"id:x","content":"hi","sent_at":"t"}]'
    expect(classifyRecv({ result: undefined, text } as never)).toEqual({ kind: 'mail', mail: [{ reply_to: 'id:x', content: 'hi', sent_at: 't' }] })
    expect(classifyRecv({ result: { content: [{ type: 'text', text }] } } as never)).toEqual({ kind: 'mail', mail: [{ reply_to: 'id:x', content: 'hi', sent_at: 't' }] })
    expect(classifyRecv({ result: undefined, text: 'null\n' } as never)).toEqual({ kind: 'mail', mail: [] })
    expect(classifyRecv({ deny: 'no' } as never)).toEqual({ kind: 'refused', reason: 'no' })
    expect(classifyRecv({ isError: true, result: 'x', text: 'unknown_session' } as never)).toEqual({ kind: 'failed', reason: 'unknown_session' })
  })

  test('only the model\'s own successful recv for the bound key re-arms', () => {
    const recv = 'mcp__plugin_ws_ws__mailbox_recv'
    const ok = { result: undefined, text: 'null' } as never
    const model = { tool: recv, originPlugin: 'engine', sessionKey: 'k1' }
    expect(isModelRecvCompletion(model, ok, recv, 'k1')).toBe(true)
    expect(isModelRecvCompletion({ ...model, originPlugin: 'ws' }, ok, recv, 'k1')).toBe(false)
    expect(isModelRecvCompletion({ ...model, sessionKey: 'k2' }, ok, recv, 'k1')).toBe(false)
    expect(isModelRecvCompletion({ ...model, tool: 'mcp__other__mailbox_recv' }, ok, recv, 'k1')).toBe(false)
    expect(isModelRecvCompletion(model, { deny: 'declined' } as never, recv, 'k1')).toBe(false)
    expect(isModelRecvCompletion(model, { isError: true, result: undefined, text: 'x' } as never, recv, 'k1')).toBe(false)
    expect(isModelRecvCompletion(model, ok, undefined, 'k1')).toBe(false)
    expect(isModelRecvCompletion(model, ok, recv, undefined)).toBe(false)
  })

  test('prompt framing', () => {
    expect(formatBatch('ws mailbox', 'R', 'k', [{ content: 'c' }])).toBe(
      '[ws mailbox] 1 message(s) arrived for session_key k. They were already received through R; do not call it again for these.\n\nmail from unknown:\nc',
    )
    expect(formatNotice('ws mailbox', 'R', 'k')).toBe('[ws mailbox] Mail is waiting for session_key k. Call R with session_key k to receive it.')
  })
})

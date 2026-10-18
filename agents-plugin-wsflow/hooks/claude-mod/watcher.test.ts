import { describe, expect, test } from 'claude-code/testing'

import {
  abortableSleep,
  checkWatchCall,
  classifyRecv,
  consumeWait,
  drainModeFor,
  isModelRecvCompletion,
  launcherPath,
  mcpToolName,
  observeRecv,
  selfAddress,
} from './register'
import {
  createWatcher,
  formatBatch,
  formatNotice,
  mapWaitExit,
  nextBackoff,
  parseRecvText,
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
  const waits: { key: string; slug: string | undefined; signal: AbortSignal; d: Deferred<WaitOutcome> }[] = []
  const drains: string[] = []
  const drainQueue: DrainResult[] = []
  const submitted: string[] = []
  const submitFailures: Error[] = []
  const sleeps: { ms: number; d: Deferred<void> }[] = []
  const notices: string[] = []
  const records: string[] = []
  const logs: string[] = []
  /** When set, the next drain waits on it instead of answering from the queue at once. */
  const heldDrains: Deferred<DrainResult>[] = []
  /** When set, the next submit waits on it; rejecting it fails that submit. */
  const heldSubmits: { d: Deferred<Error | undefined>; text: string }[] = []
  let holdSubmits = 0
  /** Whether a main-thread turn runs, as the turn hooks would report it. */
  const turn = { running: false }
  const appended: string[] = []
  const appendFailures: Error[] = []
  const edges: WatcherEdges = {
    mode,
    label: 'pkg mailbox',
    wait: (key, slug, signal) => {
      const d = deferred<WaitOutcome>()
      signal.addEventListener('abort', () => d.resolve('stopped'), { once: true })
      waits.push({ key, slug, signal, d })
      return d.promise
    },
    drain: async (key) => {
      drains.push(key)
      const held = heldDrains.shift()
      if (held) return held.promise
      return drainQueue.shift() ?? { kind: 'mail', mail: [] }
    },
    submit: async (text) => {
      if (holdSubmits > 0) {
        holdSubmits -= 1
        const d = deferred<Error | undefined>()
        heldSubmits.push({ d, text })
        const failure = await d.promise
        if (failure) throw failure
        submitted.push(text)
        return
      }
      const failure = submitFailures.shift()
      if (failure) throw failure
      submitted.push(text)
    },
    turnRunning: () => turn.running,
    append: async (text) => {
      const failure = appendFailures.shift()
      if (failure) throw failure
      appended.push(text)
    },
    recvTool: () => 'mcp__plugin_pkg_pkg__mailbox_recv',
    sleep: (ms, signal) => {
      const d = deferred<void>()
      signal.addEventListener('abort', () => d.resolve(), { once: true })
      sleeps.push({ ms, d })
      return d.promise
    },
    notice: (text) => notices.push(text),
    record: (text) => records.push(text),
    log: (text) => logs.push(text),
  }
  return {
    edges,
    waits,
    drains,
    drainQueue,
    heldDrains,
    submitted,
    submitFailures,
    heldSubmits,
    holdNextSubmit: () => (holdSubmits += 1),
    sleeps,
    notices,
    records,
    logs,
    turn,
    appended,
    appendFailures,
  }
}

const RECV = 'mcp__plugin_pkg_pkg__mailbox_recv'
const mailA = [{ reply_to: 'id:a', content: 'first', sent_at: 't1' }]
const mailB = [{ from: 'p@machine', content: 'second' }]

/** Resolve the latest wait with mail, with `drained` as the drain's answer (direct mode). */
async function arrive(h: ReturnType<typeof harness>, drained?: DrainResult) {
  if (drained) h.drainQueue.push(drained)
  h.waits[h.waits.length - 1]!.d.resolve('mail')
  await tick()
}

describe('mid-turn steer', () => {
  test('mail drained while a turn runs is queued, then appended as one row at the next tool boundary', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    expect(h.submitted).toEqual([])
    expect(h.appended).toEqual([])
    expect(h.waits.length).toBe(2) // the loop re-arms at once; it does not wait for the boundary
    await w.toolCompleted()
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    await w.toolCompleted()
    expect(h.appended.length).toBe(1)
    w.turnCompleted()
    await tick()
    expect(h.submitted).toEqual([])
  })

  test('several batches before one boundary join into a single row, never one per batch', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    await arrive(h, { kind: 'mail', mail: mailB })
    expect(h.waits.length).toBe(3)
    await w.toolCompleted()
    expect(h.appended).toEqual([
      `${formatBatch('pkg mailbox', RECV, 'k1', mailA)}

${formatBatch('pkg mailbox', RECV, 'k1', mailB)}`,
    ])
  })

  test('mail still queued at the turn end is submitted as one prompt', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    await arrive(h, { kind: 'mail', mail: mailB })
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.appended).toEqual([])
    expect(h.submitted).toEqual([
      `${formatBatch('pkg mailbox', RECV, 'k1', mailA)}

${formatBatch('pkg mailbox', RECV, 'k1', mailB)}`,
    ])
    w.turnCompleted()
    await tick()
    expect(h.submitted.length).toBe(1)
  })

  test('mail drained while idle is submitted as before, with nothing queued', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    await arrive(h, { kind: 'mail', mail: mailA })
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    await w.toolCompleted()
    w.turnCompleted()
    await tick()
    expect(h.appended).toEqual([])
    expect(h.submitted.length).toBe(1)
  })

  test('a refused row records each queued batch for the user', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    h.appendFailures.push(new Error('row refused: policy'))
    await w.toolCompleted()
    expect(h.appended).toEqual([])
    expect(h.records).toEqual([
      `pkg mailbox: drained mail was not submitted (row append failed: Error: row refused: policy); recorded here instead:

${formatBatch('pkg mailbox', RECV, 'k1', mailA)}`,
    ])
    w.turnCompleted()
    await tick()
    expect(h.submitted).toEqual([])
  })

  test('a failed turn-end submit records the queued batches for the user', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    h.turn.running = false
    h.submitFailures.push(new Error('prompt dropped: a Stop stands'))
    w.turnCompleted()
    await tick()
    expect(h.submitted).toEqual([])
    expect(h.records).toEqual([
      `pkg mailbox: drained mail was not submitted (prompt submit failed: Error: prompt dropped: a Stop stands); recorded here instead:

${formatBatch('pkg mailbox', RECV, 'k1', mailA)}`,
    ])
  })

  test('a stop records what is still queued; a rebind keeps it for delivery', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    w.bind('k2')
    await tick()
    expect(h.records).toEqual([])
    await arrive(h, { kind: 'mail', mail: mailB })
    w.stop('session.end clear')
    expect(h.records).toEqual([
      `pkg mailbox: drained mail was not submitted (the push loop stopped: session.end clear); recorded here instead:

${formatBatch('pkg mailbox', RECV, 'k1', mailA)}`,
      `pkg mailbox: drained mail was not submitted (the push loop stopped: session.end clear); recorded here instead:

${formatBatch('pkg mailbox', RECV, 'k2', mailB)}`,
    ])
    await w.toolCompleted()
    w.turnCompleted()
    await tick()
    expect(h.appended).toEqual([])
    expect(h.submitted).toEqual([])
  })

  test('a notice found mid-turn is queued and appended; the loop stays suspended until the model receives', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h)
    expect(h.submitted).toEqual([])
    expect(w.isSuspended()).toBe(true)
    await w.toolCompleted()
    expect(h.appended).toEqual([formatNotice('pkg mailbox', RECV, 'k1')])
    expect(w.isSuspended()).toBe(true)
    w.recvCompleted('k1')
    await tick()
    expect(h.waits.length).toBe(2)
  })

  test('a queued notice that is never delivered backs off and re-arms, as a failed notice submit does', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.turn.running = true
    await arrive(h)
    h.appendFailures.push(new Error('row refused: policy'))
    await w.toolCompleted()
    await tick()
    expect(w.isSuspended()).toBe(false)
    expect(h.records).toEqual([])
    expect(h.notices).toContain('pkg mailbox: notice submit failed, retrying with backoff')
    expect(h.sleeps.map((s) => s.ms)).toEqual([1_000])
    h.sleeps[0]!.d.resolve()
    await tick()
    expect(h.waits.length).toBe(2)
  })
})

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

  test('a failed submit after a drain records the batch for the user instead of dropping it', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    const mail = [{ from: 'p@machine', content: 'hello' }]
    h.drainQueue.push({ kind: 'mail', mail })
    h.submitFailures.push(new Error('prompt dropped: a Stop stands'))
    h.waits[0]!.d.resolve('mail')
    await tick()
    const batch = formatBatch('pkg mailbox', 'mcp__plugin_pkg_pkg__mailbox_recv', 'k1', mail)
    expect(h.submitted).toEqual([])
    expect(h.records).toEqual([
      `pkg mailbox: drained mail was not submitted (prompt submit failed: Error: prompt dropped: a Stop stands); recorded here instead:\n\n${batch}`,
    ])
    expect(h.logs).toContain(h.records[0]!)
    expect(h.waits.length).toBe(2)
  })

  test('a drain in flight when the loop stops is recorded, not submitted', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    const held = deferred<DrainResult>()
    h.heldDrains.push(held)
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.drains).toEqual(['k1'])
    w.stop('session.end clear')
    const mail = [{ reply_to: 'id:a', content: 'late' }]
    held.resolve({ kind: 'mail', mail })
    await tick()
    expect(h.submitted).toEqual([])
    expect(h.records).toEqual([
      `pkg mailbox: drained mail was not submitted (the push loop stopped: session.end clear); recorded here instead:\n\n` +
        formatBatch('pkg mailbox', 'mcp__plugin_pkg_pkg__mailbox_recv', 'k1', mail),
    ])
    expect(h.logs).toContain(h.records[0]!)
    expect(h.waits.length).toBe(1)
  })

  test('a drain in flight across a rebind is still delivered', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    const held = deferred<DrainResult>()
    h.heldDrains.push(held)
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(w.bind('k2')).toBe('rebound')
    const mail = [{ reply_to: 'id:a', content: 'raced' }]
    held.resolve({ kind: 'mail', mail })
    await tick()
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', 'mcp__plugin_pkg_pkg__mailbox_recv', 'k1', mail)])
    expect(h.records).toEqual([])
    expect(h.waits.map((x) => x.key)).toEqual(['k1', 'k2'])
  })

  test('a drain superseded by a rebind and then stopped is recorded, not submitted', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    const held = deferred<DrainResult>()
    h.heldDrains.push(held)
    h.waits[0]!.d.resolve('mail')
    await tick()
    w.bind('k2')
    await tick()
    w.stop('session.end resume')
    const mail = [{ reply_to: 'id:a', content: 'late' }]
    held.resolve({ kind: 'mail', mail })
    await tick()
    expect(h.submitted).toEqual([])
    expect(h.records.length).toBe(1)
    expect(h.records[0]).toContain('(the push loop stopped: session.end resume)')
    expect(h.records[0]).toContain(formatBatch('pkg mailbox', 'mcp__plugin_pkg_pkg__mailbox_recv', 'k1', mail))
  })

  test('unparseable recv output is submitted raw inside the batch framing', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.drainQueue.push(classifyRecv({ result: undefined, text: 'mailbox: 1 message\nhello' } as never))
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.submitted.length).toBe(1)
    expect(h.submitted[0]).toContain('1 message(s) arrived for session_key k1')
    expect(h.submitted[0]).toContain('unparsed mailbox recv output:\nmailbox: 1 message\nhello')
    expect(h.sleeps.length).toBe(0)
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

  test('a notice submit failing after a rebind leaves the new key suspended', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.holdNextSubmit()
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.heldSubmits.length).toBe(1)
    w.bind('k2')
    await tick()
    h.waits[1]!.d.resolve('mail')
    await tick()
    expect(w.isSuspended()).toBe(true)
    // The k1 loop's notice now fails: it resumes only its own suspension.
    h.heldSubmits[0]!.d.resolve(new Error('no session'))
    await tick()
    expect(w.isSuspended()).toBe(true)
    expect(h.waits.length).toBe(2)
    expect(h.notices).toEqual(['pkg mailbox: armed k1', 'pkg mailbox: armed k2'])
    w.recvCompleted('k2')
    await tick()
    expect(h.waits.length).toBe(3)
  })

  test('a failing notice submit notices once per streak', async () => {
    const h = harness('notice')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.submitFailures.push(new Error('no session'), new Error('no session'))
    h.waits[0]!.d.resolve('mail')
    await tick()
    h.sleeps[0]!.d.resolve()
    await tick()
    h.waits[1]!.d.resolve('mail')
    await tick()
    expect(h.sleeps.map((s) => s.ms)).toEqual([1_000, 2_000])
    expect(h.notices).toEqual(['pkg mailbox: armed k1', 'pkg mailbox: notice submit failed, retrying with backoff'])
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

  test('the slug travels with its binding into every wait', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    expect(w.bind('k1', 'a@worktree')).toBe('bound')
    await tick()
    expect(w.bind('k1', 'a@worktree')).toBe('unchanged')
    h.waits[0]!.d.resolve('timeout')
    await tick()
    expect(w.bind('k2', 'b@worktree')).toBe('rebound')
    await tick()
    expect(w.bind('k3')).toBe('rebound')
    await tick()
    expect(h.waits.map((x) => [x.key, x.slug])).toEqual([
      ['k1', 'a@worktree'],
      ['k1', 'a@worktree'],
      ['k2', 'b@worktree'],
      ['k3', undefined],
    ])
  })

  test('notices: a second failure streak after recovery notices again', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.waits[0]!.d.resolve('error')
    await tick()
    h.sleeps[0]!.d.resolve()
    await tick()
    h.waits[1]!.d.resolve('timeout')
    await tick()
    h.waits[2]!.d.resolve('error')
    await tick()
    expect(h.sleeps.map((s) => s.ms)).toEqual([1_000, 1_000])
    expect(h.notices).toEqual([
      'pkg mailbox: armed k1',
      'pkg mailbox: wait error, retrying with backoff',
      'pkg mailbox: wait error, retrying with backoff',
    ])
  })

  test('notices: a drain re-arm notices nothing; a rebind notices armed for the new key', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.drainQueue.push({ kind: 'mail', mail: [{ content: 'x' }] })
    h.waits[0]!.d.resolve('mail')
    await tick()
    expect(h.waits.length).toBe(2)
    expect(h.notices).toEqual(['pkg mailbox: armed k1'])
    w.bind('k2')
    await tick()
    expect(h.notices).toEqual(['pkg mailbox: armed k1', 'pkg mailbox: armed k2'])
  })

  test('notices: a drain-error streak notices once', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    h.drainQueue.push({ kind: 'failed', reason: 'server gone' }, { kind: 'failed', reason: 'server gone' })
    h.waits[0]!.d.resolve('mail')
    await tick()
    h.sleeps[0]!.d.resolve()
    await tick()
    h.waits[1]!.d.resolve('mail')
    await tick()
    expect(h.sleeps.length).toBe(2)
    expect(h.notices).toEqual(['pkg mailbox: armed k1', 'pkg mailbox: drain error, retrying with backoff'])
  })

  test('abort listeners do not accumulate across wait and sleep cycles', async () => {
    const ctrl = new AbortController()
    const signal = ctrl.signal
    const live = new Set<unknown>()
    const add = signal.addEventListener.bind(signal)
    const remove = signal.removeEventListener.bind(signal)
    signal.addEventListener = ((type: string, l: EventListener, o?: AddEventListenerOptions) => {
      live.add(l)
      add(type, l, o)
    }) as typeof signal.addEventListener
    signal.removeEventListener = ((type: string, l: EventListener, o?: EventListenerOptions) => {
      live.delete(l)
      remove(type, l, o)
    }) as typeof signal.removeEventListener
    const exited = { next: async () => ({ done: true, value: { code: 3, signal: null } }), return: async () => ({ done: true }) }
    for (let i = 0; i < 20; i++) {
      expect(await consumeWait(exited as never, signal, () => {})).toBe('timeout')
      const due: (() => void)[] = []
      const slept = abortableSleep((_ms, fn) => (due.push(fn), { cancel: () => {} }), 1_000, signal)
      due[0]!()
      await slept
    }
    expect(live.size).toBe(0)

    // Abort still ends a pending wait and sleep early.
    let returned = 0
    let cancelled = 0
    const hanging = { next: () => new Promise(() => {}), return: async () => (returned++, { done: true }) }
    const waiting = consumeWait(hanging as never, signal, () => {})
    const sleeping = abortableSleep(() => ({ cancel: () => void cancelled++ }), 1_000, signal)
    ctrl.abort()
    expect(await waiting).toBe('stopped')
    await sleeping
    expect(returned).toBe(1)
    expect(cancelled).toBe(1)
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

  test('only blank text, null, or an empty array is an empty drain; anything else is delivered', () => {
    for (const empty of [undefined, '', '  \n', 'null', 'null\n', '[]', ' [ ] ']) {
      expect(parseRecvText(empty)).toEqual([])
    }
    expect(parseRecvText('not json')).toEqual([{ raw: 'not json' }])
    expect(parseRecvText(' {"error":"x"} \n')).toEqual([{ raw: '{"error":"x"}' }])
    expect(parseRecvText('"text"')).toEqual([{ raw: '"text"' }])
    expect(parseRecvText('[{"content":"ok","from":"a"},{"from":"b"},{"content":3},7,null]')).toEqual([
      { content: 'ok', from: 'a' },
      { raw: '{"from":"b"}' },
      { raw: '{"content":3}' },
      { raw: '7' },
      { raw: 'null' },
    ])
    expect(classifyRecv({ result: undefined, text: 'garbled' } as never)).toEqual({ kind: 'mail', mail: [{ raw: 'garbled' }] })
  })

  test('raw items and odd envelope fields format without throwing', () => {
    expect(formatBatch('ws mailbox', 'R', 'k', [{ raw: '{"from":"b"}' }, { content: 'c', from: 5 as never, sent_at: 6 as never }])).toBe(
      '[ws mailbox] 2 message(s) arrived for session_key k. They were already received through R; do not call it again for these.' +
        '\n\nunparsed mailbox recv output:\n{"from":"b"}\n\nmail from unknown:\nc',
    )
  })

  test('the own named-inbox address comes from lookup_peers self.address', () => {
    const json = (v: unknown) => ({ result: undefined, text: JSON.stringify(v) }) as never
    expect(selfAddress(json({ self: { address: 'lead-ab12@worktree', auto: true }, peers: [] }))).toBe('lead-ab12@worktree')
    expect(selfAddress({ result: { content: [{ type: 'text', text: '{"self":{"address":"x@machine"}}' }] } } as never)).toBe('x@machine')
    expect(selfAddress(json({ self: { reply_id: 'id:abc' }, peers: [] }))).toBe(undefined)
    expect(selfAddress(json({ self: { address: '  ' } }))).toBe(undefined)
    expect(selfAddress(json(null))).toBe(undefined)
    expect(selfAddress({ result: undefined, text: 'self: x@machine' } as never)).toBe(undefined)
    expect(selfAddress({ deny: 'declined' } as never)).toBe(undefined)
    expect(selfAddress({ isError: true, result: undefined, text: '{"self":{"address":"x@m"}}' } as never)).toBe(undefined)
  })

  test('the recv observer never throws into the recv it observes', () => {
    const recv = 'mcp__plugin_ws_ws__mailbox_recv'
    const call = { tool: recv, originPlugin: 'engine', sessionKey: 'k1' }
    const ok = { result: undefined, text: 'null' } as never
    const logged: string[] = []
    const broken = {
      current: () => {
        throw new Error('boom')
      },
      recvCompleted: () => {},
    }
    expect(() => observeRecv(broken, call, ok, recv, (t) => logged.push(t))).not.toThrow()
    expect(logged).toEqual(['recv observer failed: Error: boom'])
    const completed: string[] = []
    observeRecv({ current: () => 'k1', recvCompleted: (k) => completed.push(k) }, call, ok, recv, () => {})
    expect(completed).toEqual(['k1'])
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

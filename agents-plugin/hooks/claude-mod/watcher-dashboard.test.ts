import { describe, expect, test } from 'claude-code/testing'

import { createWatcher, formatBatch, type DrainResult, type WakeKind } from './watcher'
import type { InboxCommand } from './dashboard'
import { arrive, deferred, harness, mailA, mailB, RECV, tick, wake } from './watcher-harness'

/** Resolve the latest wait as an interrupt wake, with `also` beside it and `drained` as the mail drain's answer. */
async function interruptWake(h: ReturnType<typeof harness>, also?: WakeKind, drained?: DrainResult) {
  if (drained) h.drainQueue.push(drained)
  h.waits[h.waits.length - 1]!.d.resolve(also ? { interrupt: true, also } : { interrupt: true })
  await tick()
}

describe('dashboard slash commands', () => {
  const cmd = (name: string, args = ''): InboxCommand => ({ command: { name, args }, text: args === '' ? `/${name}` : `/${name} ${args}` })

  /** A watcher bound to k1 with board submission on and its first wait armed. */
  async function boardOn(continuation?: () => Promise<void>) {
    const h = harness('direct')
    h.board.on = true
    if (continuation) h.edges.continuation = continuation
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    return { h, w }
  }

  test('an idle drain splits at a command: the text before it is submitted, the command runs, and the text after it goes only once the run settled', async () => {
    const { h } = await boardOn()
    const run = deferred<Error | undefined>()
    h.board.heldRuns.push(run)
    h.board.inbox.push('A', cmd('compact', 'keep the plan'), 'B')
    await wake(h, 'board')
    expect(h.board.submitted).toEqual([['A']])
    expect(h.board.runs).toEqual(['/compact keep the plan'])
    // Never joined with the text, never appended as a row.
    expect(h.board.appended).toEqual([])
    // B waits in the mod although the inbox already gave it up.
    expect(h.board.inbox).toEqual([])
    run.resolve(undefined)
    await tick()
    expect(h.board.submitted).toEqual([['A'], ['B']])
    expect(h.order).toEqual(['board', 'run /compact keep the plan', 'board'])
  })

  test('the text after a command waits for the turn the text before it started: a run that settles mid-turn holds it for that turn end', async () => {
    const { h, w } = await boardOn()
    const run = deferred<Error | undefined>()
    h.board.heldRuns.push(run)
    h.board.inbox.push('A', cmd('context'), 'B')
    await wake(h, 'board')
    h.turn.running = true
    run.resolve(undefined)
    await tick()
    expect(h.board.submitted).toEqual([['A']])
    // Held: the wait re-arms without the board flag, as for any held message.
    expect(h.waits.at(-1)!.board).toBe(false)
    await w.toolCompleted()
    expect(h.board.submitted).toEqual([['A']])
    expect(h.appended).toEqual([])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['A'], ['B']])
    expect(h.waits.at(-1)!.board).toBe(true)
  })

  test('a command sent while a turn runs waits in the inbox for the turn end, then runs', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push(cmd('compact'))
    await wake(h, 'board')
    await w.toolCompleted()
    expect(h.board.runs).toEqual([])
    expect(h.board.steerDrains).toEqual(['k1'])
    expect(h.board.inbox).toEqual([cmd('compact')])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.runs).toEqual(['/compact'])
    expect(h.board.submitted).toEqual([])
  })

  test('a command held by a compaction runs only once the compaction settled and its continuation went first', async () => {
    const cont = deferred<void>()
    const { h, w } = await boardOn(() => cont.promise)
    h.turn.running = true
    h.board.inbox.push(cmd('context'))
    await wake(h, 'board')
    w.compactStarted()
    h.turn.running = false
    // The turn end during the compaction leaves the command held.
    w.turnCompleted()
    await tick()
    expect(h.board.runs).toEqual([])
    w.compactEnded()
    await tick()
    expect(h.board.runs).toEqual([])
    cont.resolve()
    await tick()
    expect(h.board.runs).toEqual(['/context'])
  })

  test('text after a command whose run settles during a compaction is held until the compaction ends', async () => {
    const { h, w } = await boardOn()
    const run = deferred<Error | undefined>()
    h.board.heldRuns.push(run)
    h.board.inbox.push(cmd('compact'), 'B')
    await wake(h, 'board')
    expect(h.board.runs).toEqual(['/compact'])
    w.compactStarted()
    run.resolve(undefined)
    await tick()
    expect(h.board.submitted).toEqual([])
    w.compactEnded()
    await tick()
    expect(h.board.submitted).toEqual([['B']])
  })

  test('a flush while a command runs claims into the backlog and submits nothing; the backlog goes in order once the run settles', async () => {
    const { h } = await boardOn()
    const run = deferred<Error | undefined>()
    h.board.heldRuns.push(run)
    h.board.inbox.push(cmd('context'), 'B')
    await wake(h, 'board')
    h.board.inbox.push('D')
    await wake(h, 'board')
    expect(h.board.drains).toEqual(['k1', 'k1'])
    expect(h.board.inbox).toEqual([])
    expect(h.board.submitted).toEqual([])
    run.resolve(undefined)
    await tick()
    expect(h.board.submitted).toEqual([['B', 'D']])
  })

  test('two commands in one drain run one after the other, the second only once the first settled', async () => {
    const { h } = await boardOn()
    const first = deferred<Error | undefined>()
    h.board.heldRuns.push(first)
    h.board.inbox.push(cmd('context'), cmd('cost'), 'B')
    await wake(h, 'board')
    expect(h.board.runs).toEqual(['/context'])
    first.resolve(undefined)
    await tick()
    expect(h.board.runs).toEqual(['/context', '/cost'])
    expect(h.board.submitted).toEqual([['B']])
  })

  test('a refused run is logged and the text after it still goes', async () => {
    const { h } = await boardOn()
    const run = deferred<Error | undefined>()
    h.board.heldRuns.push(run)
    h.board.inbox.push(cmd('nosuch'), 'B')
    await wake(h, 'board')
    run.resolve(new Error('unknown command'))
    await tick()
    expect(h.logs.some((l) => l.includes('/nosuch failed') && l.includes('unknown command'))).toBe(true)
    expect(h.board.submitted).toEqual([['B']])
  })

  test('a stop records what waited behind a running command, its typed lines among it, and nothing goes after the run settles', async () => {
    const { h, w } = await boardOn()
    const run = deferred<Error | undefined>()
    h.board.heldRuns.push(run)
    h.board.inbox.push(cmd('context'), 'B', cmd('cost'))
    await wake(h, 'board')
    w.stop('session end')
    expect(h.records.length).toBe(1)
    expect(h.records[0]).toContain('B')
    expect(h.records[0]).toContain('/cost')
    run.resolve(undefined)
    await tick()
    expect(h.board.submitted).toEqual([])
    expect(h.board.runs).toEqual(['/context'])
  })
})

describe('dashboard interrupt', () => {
  /** A watcher bound to k1 with board submission on and its first wait armed. */
  async function boardOn() {
    const h = harness('direct')
    h.board.on = true
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    return { h, w }
  }

  /** A main-thread turn that started at 1000, on the interrupt entries' clock. */
  function turnRuns(h: ReturnType<typeof harness>) {
    h.turn.running = true
    h.board.turnStartedAt = 1_000
  }

  test('every wait carries the interrupt flag while board submission is on: idle, mid-turn, and the flagless re-arm after a held message', async () => {
    const { h } = await boardOn()
    expect(h.waits[0]).toEqual(expect.objectContaining({ board: true, interrupt: true }))
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    // Held for the turn end: the board flag drops, the interrupt flag stays.
    expect(h.waits[1]).toEqual(expect.objectContaining({ board: false, interrupt: true }))
    // With the switch off neither flag rides.
    const off = harness('direct')
    createWatcher(off.edges).bind('k1')
    await tick()
    expect(off.waits[0]).toEqual(expect.objectContaining({ board: false, interrupt: false }))
  })

  test('a running wait whose interrupt flag no longer matches the switch is re-armed', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    // The board flag reads false either way: only the interrupt flag changes.
    h.board.on = false
    w.boardChanged()
    await tick()
    expect(h.waits[1]!.signal.aborted).toBe(true)
    expect(h.waits[2]).toEqual(expect.objectContaining({ board: false, interrupt: false }))
    h.board.on = true
    w.boardChanged()
    await tick()
    expect(h.waits[3]).toEqual(expect.objectContaining({ board: false, interrupt: true }))
    w.boardChanged()
    await tick()
    expect(h.waits.length).toBe(4)
  })

  test('an interrupt wake while a turn runs claims the interrupt entries alone and aborts the turn; it is never read as a board message', async () => {
    const { h } = await boardOn()
    turnRuns(h)
    h.board.inbox.push('pick B')
    h.board.interrupts.push(2_000)
    await interruptWake(h)
    expect(h.board.interruptDrains).toEqual(['k1'])
    expect(h.board.aborts).toBe(1)
    expect(h.board.drains).toEqual([])
    expect(h.drains).toEqual([])
    expect(h.board.inbox).toEqual(['pick B'])
    // Re-armed at once with the same flags.
    expect(h.sleeps).toEqual([])
    expect(h.waits.length).toBe(2)
    expect(h.waits[1]).toEqual(expect.objectContaining({ board: true, interrupt: true }))
  })

  test('the mark is up before the abort is asked for: a turn end that comes before the abort settles already holds the queued mail', async () => {
    const { h, w } = await boardOn()
    turnRuns(h)
    await arrive(h, { kind: 'mail', mail: mailA })
    h.board.interrupts.push(2_000)
    h.board.onAbort = () => {
      h.turn.running = false
      w.turnCompleted()
    }
    await interruptWake(h)
    await tick()
    expect(h.board.aborts).toBe(1)
    expect(h.submitted).toEqual([])
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
  })

  test('an interrupt made before the running turn started is dropped without an abort; that turn ends as usual', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.turnStartedAt = 5_000
    h.board.interrupts.push(4_000, 5_000)
    await interruptWake(h)
    expect(h.board.interruptDrains).toEqual(['k1'])
    expect(h.board.interrupts).toEqual([])
    expect(h.board.aborts).toBe(0)
    expect(h.logs.some((l) => l.includes('made before the running turn started; dropped'))).toBe(true)
    await arrive(h, { kind: 'mail', mail: mailA })
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    expect(h.appended).toEqual([])
  })

  test('an interrupt claimed while no turn runs is dropped', async () => {
    const { h } = await boardOn()
    h.board.interrupts.push(2_000)
    await interruptWake(h)
    expect(h.board.interruptDrains).toEqual(['k1'])
    expect(h.board.aborts).toBe(0)
    expect(h.logs.some((l) => l.includes('claimed while no main-thread turn runs; dropped'))).toBe(true)
    expect(h.waits.length).toBe(2)
  })

  test('a refused abort is only logged: the loop re-arms and goes on', async () => {
    const { h } = await boardOn()
    turnRuns(h)
    h.board.interrupts.push(2_000)
    h.board.abortFailures.push(new Error('turn t1 is not the running turn t2'))
    await interruptWake(h)
    expect(h.board.aborts).toBe(1)
    expect(h.logs.some((l) => l.includes('aborting the running turn was refused'))).toBe(true)
    expect(h.waits.length).toBe(2)
    await arrive(h, { kind: 'mail', mail: mailA })
    expect(h.drains).toEqual(['k1'])
    expect(h.waits.length).toBe(3)
  })

  test('an interrupt that woke the wait beside mail or a board message is handled first, then the rest as before', async () => {
    const { h, w } = await boardOn()
    turnRuns(h)
    h.board.interrupts.push(2_000)
    const held = deferred<void>()
    h.board.heldInterruptDrains.push(held)
    await interruptWake(h, 'mail', { kind: 'mail', mail: mailA })
    expect(h.drains).toEqual([])
    held.resolve()
    await tick()
    expect(h.board.aborts).toBe(1)
    expect(h.drains).toEqual(['k1'])
    // Mid-turn the mail is queued for the turn, as any mid-turn mail.
    expect(h.submitted).toEqual([])
    await w.toolCompleted()
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    // Idle: the interrupt is dropped, the board message submitted.
    const idle = await boardOn()
    idle.h.board.interrupts.push(2_000)
    idle.h.board.inbox.push('pick B')
    await interruptWake(idle.h, 'board')
    expect(idle.h.board.aborts).toBe(0)
    expect(idle.h.board.interruptDrains).toEqual(['k1'])
    expect(idle.h.board.submitted).toEqual([['pick B']])
  })

  test('a failed interrupt drain backs off, still handling mail that woke the same wait; one that claims nothing re-arms at once', async () => {
    const { h } = await boardOn()
    h.board.interruptDrainFailures.push(new Error('server gone'))
    await interruptWake(h)
    expect(h.sleeps.map((x) => x.ms)).toEqual([1_000])
    expect(h.waits.length).toBe(1)
    h.sleeps[0]!.d.resolve()
    await tick()
    expect(h.waits.length).toBe(2)
    h.board.interruptDrainFailures.push(new Error('server gone'))
    await interruptWake(h, 'mail', { kind: 'mail', mail: mailA })
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    expect(h.waits.length).toBe(3)
    await interruptWake(h)
    expect(h.board.interruptDrains).toEqual(['k1', 'k1', 'k1'])
    expect(h.sleeps.length).toBe(1)
    expect(h.waits.length).toBe(4)
  })

  test('an interrupted turn end appends the queued mail and carried steered texts as rows and leaves the stranded row; a message drained fresh then still submits', async () => {
    const { h, w } = await boardOn()
    turnRuns(h)
    // A steered message whose row was refused: carried.
    h.board.steeredInbox.push('carried')
    h.board.steerFailures.push(new Error('row refused: deny'))
    await w.toolCompleted()
    // A steered message whose row went in, read by no request: stranded at the turn end.
    h.board.steeredInbox.push('stranded')
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['stranded']])
    await arrive(h, { kind: 'mail', mail: mailA })
    h.board.interrupts.push(2_000)
    await interruptWake(h)
    expect(h.board.aborts).toBe(1)
    // Sent after the click: the page took every earlier held message back.
    h.board.inbox.push('after the click')
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['after the click']])
    expect(h.board.shown).toEqual([0])
    expect(h.board.appended).toEqual([['carried']])
    expect(h.board.steered).toEqual([['stranded']])
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    expect(h.submitted).toEqual([])
    expect(h.records).toEqual([])
  })

  test('an interrupted turn end whose rows are refused records the mail and the carried texts for the user', async () => {
    const { h, w } = await boardOn()
    turnRuns(h)
    h.board.steeredInbox.push('carried')
    h.board.steerFailures.push(new Error('row refused: deny'))
    await w.toolCompleted()
    await arrive(h, { kind: 'mail', mail: mailA })
    h.board.interrupts.push(2_000)
    await interruptWake(h)
    h.board.appendFailures.push(new Error('row refused: policy'))
    h.appendFailures.push(new Error('row refused: policy'))
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.submitted).toEqual([])
    expect(h.board.submitted).toEqual([])
    expect(h.records.length).toBe(2)
    expect(h.records.some((r) => r.includes('carried'))).toBe(true)
    expect(h.records.some((r) => r.includes('first'))).toBe(true)
  })

  test('after an interrupted turn end, a steer row refused late is appended and one appended late stays a row; neither is submitted', async () => {
    for (const refused of [true, false]) {
      const { h, w } = await boardOn()
      turnRuns(h)
      h.board.steeredInbox.push('now please')
      const held = deferred<void>()
      h.board.heldSteers.push(held)
      if (refused) h.board.steerFailures.push(new Error('row refused: deny'))
      const boundary = w.toolCompleted()
      await tick()
      h.board.interrupts.push(2_000)
      await interruptWake(h)
      h.turn.running = false
      w.turnCompleted()
      await tick()
      held.resolve()
      await boundary
      await tick()
      expect(h.board.submitted).toEqual([])
      expect(h.board.appended).toEqual(refused ? [['now please']] : [])
      expect(h.board.steered).toEqual(refused ? [] : [['now please']])
      expect(h.records).toEqual([])
    }
  })

  test('the mark lasts until the next turn starts: idle mail after the interrupt submits, and a later turn end submits as before', async () => {
    const { h, w } = await boardOn()
    turnRuns(h)
    h.board.interrupts.push(2_000)
    await interruptWake(h)
    h.turn.running = false
    w.turnCompleted()
    await tick()
    // Mail arriving while idle is no leftover of the cut turn.
    await arrive(h, { kind: 'mail', mail: mailA })
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    w.turnStarted()
    h.turn.running = true
    h.board.turnStartedAt = 3_000
    h.board.steeredInbox.push('now please')
    h.board.steerFailures.push(new Error('row refused: deny'))
    await w.toolCompleted()
    await arrive(h, { kind: 'mail', mail: mailB })
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['now please']])
    expect(h.board.appended).toEqual([])
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA), formatBatch('pkg mailbox', RECV, 'k1', mailB)])
    expect(h.appended).toEqual([])
  })
})

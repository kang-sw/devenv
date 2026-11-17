import { describe, expect, test } from 'claude-code/testing'

import {
  BOARD_FIRST_WAIT_MS,
  COMPACTION_CONTINUATION_WAIT_MS,
  createWatcher,
  EMPTY_DRAIN_BACKOFF_MS,
  formatBatch,
  LEVER_HOLD_MS,
  STEER_DRAIN_WAIT_MS,
  type DrainMode,
} from './watcher'
import { arrive, deferred, harness, mailA, RECV, tick, wake } from './watcher-harness'

describe('board messages', () => {
  /** A watcher bound to k1 with board submission on and its first wait armed. */
  async function boardOn(mode: DrainMode = 'direct') {
    const h = harness(mode)
    h.board.on = true
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    return { h, w }
  }

  test('an idle wake drains once and submits at once as the user\'s message, then re-arms with the flag', async () => {
    const { h } = await boardOn()
    expect(h.waits[0]!.board).toBe(true)
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    expect(h.board.drains).toEqual(['k1'])
    expect(h.board.submitted).toEqual([['pick B']])
    // The board wake drains no mail.
    expect(h.drains).toEqual([])
    expect(h.submitted).toEqual([])
    expect(h.waits.length).toBe(2)
    expect(h.waits[1]!.board).toBe(true)
  })

  test('a mid-turn wake holds the message, re-arms without the flag, and injects only at the turn end; never at a tool boundary', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    expect(h.board.drains).toEqual([])
    expect(h.waits.length).toBe(2)
    expect(h.waits[1]!.board).toBe(false)
    await w.toolCompleted()
    await w.toolCompleted()
    expect(h.board.drains).toEqual([])
    expect(h.board.submitted).toEqual([])
    expect(h.appended).toEqual([])
    expect(h.board.inbox).toEqual(['pick B'])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.drains).toEqual(['k1'])
    expect(h.board.submitted).toEqual([['pick B']])
    // The flag is restored after the drain: the flagless wait is re-armed with it.
    expect(h.waits[1]!.signal.aborted).toBe(true)
    expect(h.waits.length).toBe(3)
    expect(h.waits[2]!.board).toBe(true)
  })

  test('a message promoted to steering is appended at the next tool boundary, ahead of the queued mail row; the unpromoted one stays held', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('later')
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    h.board.steeredInbox.push('now please')
    await w.toolCompleted()
    expect(h.board.steerDrains).toEqual(['k1'])
    expect(h.board.steered).toEqual([['now please']])
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    expect(h.order).toEqual(['steer'])
    // The full drain never ran mid-turn: the unpromoted message is still held.
    expect(h.board.drains).toEqual([])
    expect(h.board.inbox).toEqual(['later'])
    // A boundary with nothing steered appends nothing more.
    await w.toolCompleted()
    expect(h.board.steered.length).toBe(1)
    // The request after the result reads the steer row.
    w.requestStarted()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['later']])
  })

  test('a refused steer row carries the message to the turn end, submitted ahead of the held one and never lost', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('later')
    await wake(h, 'board')
    h.board.steeredInbox.push('now please')
    h.board.steerFailures.push(new Error('row refused: deny'))
    await w.toolCompleted()
    expect(h.board.steered).toEqual([])
    expect(h.board.steeredInbox).toEqual([])
    expect(h.records).toEqual([])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['now please', 'later']])
    expect(h.records).toEqual([])
  })

  test('a carried steered message is submitted at the turn end even with the switch turned off, draining nothing held', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    h.board.steerFailures.push(new Error('row refused: deny'))
    await w.toolCompleted()
    h.board.on = false
    h.board.inbox.push('later')
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['now please']])
    expect(h.board.drains).toEqual([])
    expect(h.board.inbox).toEqual(['later'])
  })

  test('a steer row refused after the turn ended (an interrupt) submits the carried message at once', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    const held = deferred<void>()
    h.board.heldSteers.push(held)
    h.board.steerFailures.push(new Error('row refused: deny'))
    const boundary = w.toolCompleted()
    await tick()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    held.resolve()
    await boundary
    await tick()
    expect(h.board.submitted).toEqual([['now please']])
  })

  test('a stop records a carried steered message instead of dropping it', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    h.board.steerFailures.push(new Error('row refused: deny'))
    await w.toolCompleted()
    w.stop('session.end clear')
    expect(h.records.length).toBe(1)
    expect(h.records[0]).toContain('now please')
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([])
  })

  test('a hung steered drain holds the tool result only STEER_DRAIN_WAIT_MS; no later boundary starts another, and the turn end still flushes', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('later')
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    h.board.heldSteerDrains.push(deferred<void>()) // never resolves
    let returned = false
    const boundary = w.toolCompleted().then(() => (returned = true))
    await tick()
    expect(returned).toBe(false)
    h.sleeps.find((x) => x.ms === STEER_DRAIN_WAIT_MS)!.d.resolve()
    await boundary
    // The boundary went on without it: the queued mail row still went in.
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    expect(h.board.steered).toEqual([])
    // While that drain is out, a later boundary claims nothing and waits on nothing.
    const sleepsBefore = h.sleeps.length
    await w.toolCompleted()
    expect(h.board.steerDrains).toEqual(['k1'])
    expect(h.sleeps.length).toBe(sleepsBefore)
    w.requestStarted()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['later']])
  })

  test('a late steer claim is delivered at the next boundary before newer messages, exactly once', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('first')
    const hung = deferred<void>()
    h.board.heldSteerDrains.push(hung)
    const boundary = w.toolCompleted()
    await tick()
    h.sleeps.find((x) => x.ms === STEER_DRAIN_WAIT_MS)!.d.resolve()
    await boundary
    hung.resolve()
    await tick()
    h.board.steeredInbox.push('second')
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['first']])
    w.requestStarted()
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['first'], ['second']])
    w.requestStarted()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([])
    expect(h.records).toEqual([])
  })

  test('a late claim follows an older refused row at the next boundary', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('first')
    h.board.steerFailures.push(new Error('refused'))
    await w.toolCompleted()
    h.board.steeredInbox.push('second')
    const drain = deferred<void>()
    h.board.heldSteerDrains.push(drain)
    const pending = w.toolCompleted()
    await tick()
    h.sleeps.filter((x) => x.ms === STEER_DRAIN_WAIT_MS).at(-1)!.d.resolve()
    await pending
    drain.resolve()
    await tick()
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['first', 'second']])
    w.requestStarted()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([])
  })

  test('a pending late-claim row prevents an overlapping boundary from overtaking it', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('first')
    const drain = deferred<void>()
    h.board.heldSteerDrains.push(drain)
    const first = w.toolCompleted()
    await tick()
    h.sleeps.find((x) => x.ms === STEER_DRAIN_WAIT_MS)!.d.resolve()
    await first
    drain.resolve()
    await tick()
    h.board.steeredInbox.push('second')
    const row = deferred<void>()
    h.board.heldSteers.push(row)
    const pending = w.toolCompleted()
    await tick()
    await w.toolCompleted()
    expect(h.board.steerDrains).toEqual(['k1'])
    expect(h.board.steered).toEqual([])
    row.resolve()
    await pending
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['first'], ['second']])
  })

  test('what a timed-out steered drain claims late is submitted at the turn end, or at once once that has passed', async () => {
    for (const lateAfterTurnEnd of [false, true]) {
      const { h, w } = await boardOn()
      h.turn.running = true
      h.board.steeredInbox.push('now please')
      const hung = deferred<void>()
      h.board.heldSteerDrains.push(hung)
      const boundary = w.toolCompleted()
      await tick()
      h.sleeps.find((x) => x.ms === STEER_DRAIN_WAIT_MS)!.d.resolve()
      await boundary
      w.requestStarted()
      if (lateAfterTurnEnd) {
        h.turn.running = false
        w.turnCompleted()
        await tick()
      }
      hung.resolve()
      await tick()
      expect(h.board.steered).toEqual([])
      expect(h.board.steeredInbox).toEqual([])
      if (!lateAfterTurnEnd) {
        expect(h.board.submitted).toEqual([])
        h.turn.running = false
        w.turnCompleted()
        await tick()
      }
      expect(h.board.submitted).toEqual([['now please']])
      expect(h.records).toEqual([])
    }
  })

  test('a steer row the turn ends without any request reading (a refused or interrupted call) is submitted again at the turn end, ahead of the held one', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('later')
    await wake(h, 'board')
    h.board.steeredInbox.push('now please')
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['now please']])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['now please', 'later']])
    // The first text is already in the conversation and on the page.
    expect(h.board.shown).toEqual([1])
  })

  test('a dropped re-submit of an unread steer row appends nothing more: the row rides the next prompt; the fresh ones still fall back to a row', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    await w.toolCompleted()
    h.board.submitFailures.push(new Error('prompt dropped: a Stop stands'))
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.appended).toEqual([])
    expect(h.records).toEqual([])
    // With a held message beside it, only the held one becomes a row.
    h.turn.running = true
    h.board.steeredInbox.push('again')
    await w.toolCompleted()
    h.board.inbox.push('later')
    h.board.submitFailures.push(new Error('prompt dropped: a Stop stands'))
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.appended).toEqual([['later']])
  })

  test('a steer row appended after its turn ended is submitted again at once', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    const held = deferred<void>()
    h.board.heldSteers.push(held)
    const boundary = w.toolCompleted()
    await tick()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    held.resolve()
    await boundary
    await tick()
    expect(h.board.steered).toEqual([['now please']])
    expect(h.board.submitted).toEqual([['now please']])
    expect(h.board.shown).toEqual([1])
  })

  test('a stop forgets an unread steer row: the conversation that held it is gone', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    await w.toolCompleted()
    w.stop('session.end clear')
    expect(h.records).toEqual([])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([])
  })

  test('with the switch off a tool boundary claims nothing steered; a failed steer drain leaves it held', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    h.board.on = false
    await w.toolCompleted()
    expect(h.board.steerDrains).toEqual([])
    h.board.on = true
    h.board.steerDrainFailures.push(new Error('server gone'))
    await w.toolCompleted()
    expect(h.board.steerDrains).toEqual(['k1'])
    expect(h.board.steeredInbox).toEqual(['now please'])
    await w.toolCompleted()
    expect(h.board.steered).toEqual([['now please']])
  })

  test('an entry cancelled while held is never injected', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    // The page cancels it before the turn ends.
    h.board.inbox.splice(0)
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.drains).toEqual(['k1'])
    expect(h.board.submitted).toEqual([])
    expect(h.board.appended).toEqual([])
    expect(h.records).toEqual([])
    expect(h.waits.at(-1)!.board).toBe(true)
  })

  test('an entry replaced while held is injected once', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    // The page merges a second send into the held entry.
    h.board.inbox[0] = 'pick B\n\nand hurry'
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['pick B\n\nand hurry']])
    // The re-armed wait finds nothing more; another turn end submits nothing.
    w.turnCompleted()
    await tick()
    expect(h.board.submitted.length).toBe(1)
  })

  test('at the turn end the held board message is submitted first, the leftover mail after it; both arrive', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    expect(h.board.drains).toEqual([])
    h.board.inbox.push('pick B')
    const held = deferred<void>()
    h.board.heldSubmits.push(held)
    h.turn.running = false
    w.turnCompleted()
    await tick()
    // The mail waits on the board message's submit.
    expect(h.submitted).toEqual([])
    held.resolve()
    await tick()
    expect(h.order).toEqual(['board', 'mail'])
    expect(h.board.submitted).toEqual([['pick B']])
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
  })

  test('a turn-end board drain that hangs holds the mail back only BOARD_FIRST_WAIT_MS', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    h.board.inbox.push('pick B')
    const hang = deferred<void>()
    h.board.heldDrains.push(hang)
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.submitted).toEqual([])
    const bounded = h.sleeps.find((x) => x.ms === BOARD_FIRST_WAIT_MS)!
    bounded.d.resolve()
    await tick()
    expect(h.order).toEqual(['mail'])
    hang.resolve()
    await tick()
    expect(h.order).toEqual(['mail', 'board'])
  })

  test('a stop while the turn end is draining records the board message and the waiting mail, submitting neither', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    h.board.inbox.push('pick B')
    const hang = deferred<void>()
    h.board.heldDrains.push(hang)
    h.turn.running = false
    w.turnCompleted()
    await tick()
    w.stop('session.end clear')
    hang.resolve()
    await tick()
    expect(h.order).toEqual([])
    expect(h.records.length).toBe(2)
    expect(h.records.some((r) => r.includes('pick B'))).toBe(true)
    expect(h.records.some((r) => r.includes('first'))).toBe(true)
  })

  test('with the switch off the wait carries no flag and a turn end drains nothing; switching on re-arms the running wait', async () => {
    const h = harness('direct')
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    expect(h.waits[0]!.board).toBe(false)
    h.turn.running = true
    await arrive(h, { kind: 'mail', mail: mailA })
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.drains).toEqual([])
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    w.boardChanged()
    expect(h.waits.length).toBe(2)
    h.board.on = true
    w.boardChanged()
    await tick()
    expect(h.waits[1]!.signal.aborted).toBe(true)
    expect(h.waits.length).toBe(3)
    expect(h.waits[2]!.board).toBe(true)
    // An unchanged switch leaves the running wait alone.
    w.boardChanged()
    await tick()
    expect(h.waits.length).toBe(3)
  })

  test('an idle mail-and-board wake submits the board message, then the mail', async () => {
    const { h } = await boardOn()
    h.board.inbox.push('pick B')
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    expect(h.order).toEqual(['board', 'mail'])
    expect(h.waits.at(-1)!.board).toBe(true)
  })

  test('a turn-end drain that fails keeps the message held and re-arms with the flag, so the idle wake retries it', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    h.board.drainFailures.push(new Error('server gone'))
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([])
    expect(h.board.inbox).toEqual(['pick B'])
    expect(h.waits.at(-1)!.board).toBe(true)
    await wake(h, 'board')
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('a mail-and-board wake mid-turn steers the mail and holds the board message', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    expect(h.drains).toEqual(['k1'])
    expect(h.board.drains).toEqual([])
    await w.toolCompleted()
    expect(h.appended).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
    expect(h.board.submitted).toEqual([])
  })

  test('an idle drain that fails backs off with the message still held; mail that woke the same wait is still delivered', async () => {
    const { h } = await boardOn()
    h.board.inbox.push('pick B')
    h.board.drainFailures.push(new Error('server gone'))
    await wake(h, 'board')
    expect(h.sleeps.map((x) => x.ms)).toEqual([1_000])
    expect(h.board.inbox).toEqual(['pick B'])
    h.sleeps[0]!.d.resolve()
    await tick()
    h.board.drainFailures.push(new Error('server gone'))
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    expect(h.submitted).toEqual([formatBatch('pkg mailbox', RECV, 'k1', mailA)])
  })

  test('an idle wake whose drain finds nothing sleeps instead of re-arming at once', async () => {
    const { h } = await boardOn()
    await wake(h, 'board')
    expect(h.board.drains).toEqual(['k1'])
    expect(h.sleeps.map((x) => x.ms)).toEqual([EMPTY_DRAIN_BACKOFF_MS])
    expect(h.waits.length).toBe(1)
  })

  test('a dropped submit appends the message as a row; with the row refused too it is recorded for the user', async () => {
    const { h } = await boardOn()
    h.board.inbox.push('pick B')
    h.board.submitFailures.push(new Error('prompt dropped: a Stop stands'))
    await wake(h, 'board')
    expect(h.board.appended).toEqual([['pick B']])
    expect(h.records).toEqual([])
    h.board.inbox.push('pick C')
    h.board.submitFailures.push(new Error('prompt dropped: a Stop stands'))
    h.board.appendFailures.push(new Error('row refused: policy'))
    await wake(h, 'board')
    expect(h.records.length).toBe(1)
    expect(h.records[0]).toContain('pick C')
    expect(h.board.submitted).toEqual([])
  })

  test('a stop forgets a held message: the next binding\'s turn end does not treat it as held', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.stop('session cleared')
    h.board.on = false
    w.bind('k2')
    await tick()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.drains).toEqual([])
  })
})

describe('board messages during a compaction', () => {
  /**
   * A watcher bound to k1 with board submission on and its first wait armed;
   * `continuation`, when given, is the compaction continuation edge.
   */
  async function boardOn(continuation?: () => Promise<void>) {
    const h = harness('direct')
    h.board.on = true
    if (continuation) h.edges.continuation = continuation
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    return { h, w }
  }

  test('a wake during an idle compaction submits nothing: the message stays held and the wait re-arms without the flag', async () => {
    const { h, w } = await boardOn()
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    expect(h.board.drains).toEqual([])
    expect(h.board.submitted).toEqual([])
    expect(h.board.inbox).toEqual(['pick B'])
    expect(h.waits.length).toBe(2)
    expect(h.waits[1]!.board).toBe(false)
    expect(h.waits[1]!.interrupt).toBe(true)
  })

  test('the compaction end with no continuation submits the held message and re-arms the wait with the flag', async () => {
    const { h, w } = await boardOn()
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.compactEnded()
    await tick()
    expect(h.board.drains).toEqual(['k1'])
    expect(h.board.submitted).toEqual([['pick B']])
    expect(h.waits[1]!.signal.aborted).toBe(true)
    expect(h.waits.at(-1)!.board).toBe(true)
  })

  test('a queued continuation goes first: its turn end takes the held message as that turn\'s leftovers', async () => {
    const cont = deferred<void>()
    const { h, w } = await boardOn(() => cont.promise)
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.compactEnded()
    await tick()
    expect(h.board.drains).toEqual([])
    // The continuation's submit settles as its turn starts.
    h.turn.running = true
    cont.resolve()
    await tick()
    expect(h.board.drains).toEqual([])
    expect(h.board.submitted).toEqual([])
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
    expect(h.waits.at(-1)!.board).toBe(true)
  })

  test('a continuation that settles with no turn running (cancelled or not submitted) leaves the held message submitted after it', async () => {
    const cont = deferred<void>()
    const { h, w } = await boardOn(() => cont.promise)
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.compactEnded()
    await tick()
    expect(h.board.submitted).toEqual([])
    cont.resolve()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('a continuation that never settles holds the message only COMPACTION_CONTINUATION_WAIT_MS', async () => {
    const { h, w } = await boardOn(() => new Promise<void>(() => {}))
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.compactEnded()
    await tick()
    expect(h.board.submitted).toEqual([])
    h.sleeps.find((x) => x.ms === COMPACTION_CONTINUATION_WAIT_MS)!.d.resolve()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('a turn end during a compaction submits the mail but leaves the board message held for the compaction end', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.order).toEqual(['mail'])
    expect(h.board.drains).toEqual([])
    w.compactEnded()
    await tick()
    expect(h.order).toEqual(['mail', 'board'])
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('a carried steered message claimed after the turn ended waits for the compaction end too', async () => {
    const { h, w } = await boardOn()
    h.turn.running = true
    h.board.steeredInbox.push('now please')
    h.board.steerFailures.push(new Error('row refused'))
    const held = deferred<void>()
    h.board.heldSteers.push(held)
    const boundary = w.toolCompleted()
    await tick()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    w.compactStarted()
    held.resolve()
    await boundary
    await tick()
    expect(h.board.submitted).toEqual([])
    w.compactEnded()
    await tick()
    expect(h.board.submitted).toEqual([['now please']])
  })

  test('a compaction end with nothing held drains nothing; a later compaction supersedes an earlier release still waiting', async () => {
    const cont = deferred<void>()
    const { h, w } = await boardOn(() => cont.promise)
    w.compactStarted()
    w.compactEnded()
    await tick()
    expect(h.board.drains).toEqual([])
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.compactEnded()
    await tick()
    w.compactStarted()
    cont.resolve()
    await tick()
    expect(h.board.drains).toEqual([])
    w.compactEnded()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('a stop during the continuation wait forgets the held message', async () => {
    const cont = deferred<void>()
    const { h, w } = await boardOn(() => cont.promise)
    w.compactStarted()
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    w.compactEnded()
    await tick()
    w.stop('session.end clear')
    cont.resolve()
    await tick()
    expect(h.board.drains).toEqual([])
  })
})

describe('board messages behind a requested compaction', () => {
  /**
   * A watcher bound to k1 with board submission on and its first wait armed;
   * `lever` is what the leverPending edge reads, `continuation` the
   * compaction continuation edge when given.
   */
  async function boardOn(continuation?: () => Promise<void>) {
    const h = harness('direct')
    h.board.on = true
    const lever = { pending: false }
    h.edges.leverPending = () => lever.pending
    if (continuation) h.edges.continuation = continuation
    const w = createWatcher(h.edges)
    w.bind('k1')
    await tick()
    return { h, w, lever }
  }

  /** A turn that ends with `pick B` held, and a lever call when `lever.pending` reads true. */
  async function heldTurnEnd(h: ReturnType<typeof harness>, w: ReturnType<typeof createWatcher>) {
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'board')
    h.turn.running = false
    w.turnCompleted()
    await tick()
  }

  const capSleep = (h: ReturnType<typeof harness>) => h.sleeps.find((x) => x.ms === LEVER_HOLD_MS)

  test('a turn end whose turn requested a compaction holds the board release, the wait re-armed without the flag', async () => {
    const { h, w, lever } = await boardOn()
    lever.pending = true
    await heldTurnEnd(h, w)
    expect(h.board.drains).toEqual([])
    expect(h.board.submitted).toEqual([])
    expect(h.board.inbox).toEqual(['pick B'])
    expect(h.waits.at(-1)!.board).toBe(false)
    expect(capSleep(h)).toBeDefined()
  })

  test('the compaction start takes the held messages to its release after the continuation, and the cap never fires after it', async () => {
    const cont = deferred<void>()
    const { h, w, lever } = await boardOn(() => cont.promise)
    lever.pending = true
    await heldTurnEnd(h, w)
    const cap = capSleep(h)!
    w.compactStarted()
    await tick()
    // The start ended the cap wait without releasing anything.
    expect(h.board.drains).toEqual([])
    w.compactEnded()
    await tick()
    expect(h.board.drains).toEqual([])
    cont.resolve()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
    // A late cap resolve releases nothing more.
    cap.d.resolve()
    await tick()
    expect(h.board.drains).toEqual(['k1'])
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('with the continue flag, the resume prompt\'s turn starts before the held message, which its turn end takes', async () => {
    const cont = deferred<void>()
    const { h, w, lever } = await boardOn(() => cont.promise)
    lever.pending = true
    await heldTurnEnd(h, w)
    w.compactStarted()
    w.compactEnded()
    await tick()
    // The resume prompt's turn starts as its submit settles.
    h.turn.running = true
    w.turnStarted()
    cont.resolve()
    await tick()
    expect(h.board.submitted).toEqual([])
    lever.pending = false
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('the cap releases the held messages when no compaction started, and the wait gets its flag back', async () => {
    const { h, w, lever } = await boardOn()
    lever.pending = true
    await heldTurnEnd(h, w)
    expect(h.board.submitted).toEqual([])
    capSleep(h)!.d.resolve()
    await tick()
    expect(h.board.drains).toEqual(['k1'])
    expect(h.board.submitted).toEqual([['pick B']])
    expect(h.waits.at(-1)!.board).toBe(true)
  })

  test('at the cap with a turn running, the messages stay held until that turn\'s end', async () => {
    const { h, w, lever } = await boardOn()
    lever.pending = true
    await heldTurnEnd(h, w)
    h.turn.running = true
    capSleep(h)!.d.resolve()
    await tick()
    expect(h.board.drains).toEqual([])
    lever.pending = false
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['pick B']])
  })

  test('a board wake in the settle gap is held until the compaction or the cap', async () => {
    const { h, w, lever } = await boardOn()
    lever.pending = true
    h.turn.running = true
    w.turnCompleted()
    h.turn.running = false
    await tick()
    h.board.inbox.push('late one')
    await wake(h, 'board')
    expect(h.board.drains).toEqual([])
    expect(h.board.submitted).toEqual([])
    expect(h.waits.at(-1)!.board).toBe(false)
    capSleep(h)!.d.resolve()
    await tick()
    expect(h.board.submitted).toEqual([['late one']])
  })

  test('a turn end with no lever call still owed (none made, an aborted lever turn, or one a landing dropped) releases at once', async () => {
    const { h, w } = await boardOn()
    await heldTurnEnd(h, w)
    expect(h.board.submitted).toEqual([['pick B']])
    expect(capSleep(h)).toBeUndefined()
  })

  test('mail queued during the lever turn is still submitted at its end', async () => {
    const { h, w, lever } = await boardOn()
    lever.pending = true
    h.turn.running = true
    h.board.inbox.push('pick B')
    await wake(h, 'both', { kind: 'mail', mail: mailA })
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.order).toEqual(['mail'])
    expect(h.board.submitted).toEqual([])
    capSleep(h)!.d.resolve()
    await tick()
    expect(h.order).toEqual(['mail', 'board'])
  })

  test('the cap wait never blocks the board chain: the next turn end releases while it is still out', async () => {
    const { h, w, lever } = await boardOn()
    // The cap's sleep never settles, not even on its abort.
    const sleep = h.edges.sleep
    h.edges.sleep = (ms, signal) => (ms === LEVER_HOLD_MS ? new Promise<void>(() => {}) : sleep(ms, signal))
    lever.pending = true
    await heldTurnEnd(h, w)
    expect(h.board.submitted).toEqual([])
    lever.pending = false
    h.board.inbox.push('next')
    h.turn.running = true
    w.turnStarted()
    h.turn.running = false
    w.turnCompleted()
    await tick()
    expect(h.board.submitted).toEqual([['pick B', 'next']])
  })

  test('a stop lifts the hold: a later cap releases nothing', async () => {
    const { h, w, lever } = await boardOn()
    lever.pending = true
    await heldTurnEnd(h, w)
    w.stop('session.end clear')
    capSleep(h)!.d.resolve()
    await tick()
    expect(h.board.drains).toEqual([])
  })
})

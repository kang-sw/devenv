import { describe, expect, test } from 'claude-code/testing'

import { compactionEnabledFor, CONTINUATION_WINDOW_MS, createCompaction, type CompactionEdges } from './compaction'
import {
  advisoryText,
  checkLeverCall,
  continuationText,
  levelAt,
  LEVER_DESCRIPTION,
  LEVER_RESULT,
  milestoneText,
  observe,
  skippedContinuationText,
  thresholdsFor,
  type CompactionWindow,
} from './timing'

// A 200k compaction window compacting automatically at 170k (85%): advisory
// 100k (50%), milestones about 123.3k (62%) and 146.7k (73%).
const RAW = 200_000
const HARD = 170_000
const WINDOW: CompactionWindow = { rawMaxTokens: RAW, autoCompactThreshold: HARD, isAutoCompactEnabled: true }
const T = thresholdsFor(WINDOW)!

const tick = async (n = 30) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

type CompactAnswer = { skip?: string } | Error

/** Fake edges: submits and compactions are recorded; scheduled steps run when the test says. */
function harness(opts: { window?: Partial<CompactionWindow> | null } = {}) {
  const h = {
    submits: [] as string[],
    /** Answers for the next compact() calls in order; an Error rejects. Absent: the compaction lands. */
    compactAnswers: [] as CompactAnswer[],
    compacts: 0,
    scheduled: [] as (() => void)[],
    /** delay() timers, run when the test says. */
    delayed: [] as { ms: number; fn: () => void }[],
    /** The next delay() calls that throw. */
    delayThrows: 0,
    logs: [] as { text: string; to: string }[],
    breakdownReads: 0,
    dropSubmits: 0,
    /** The next submits that reject. */
    rejectSubmits: 0,
    /** The next after() calls that throw. */
    afterThrows: 0,
    /** How many times the lever's own landing was reported. */
    leverLandings: 0,
    /** Whether that report throws. */
    leverLandedThrows: false,
    window: opts.window === null ? null : ({ ...WINDOW, ...opts.window } as CompactionWindow),
  }
  const c = createCompaction()
  /** Whether the fake engine routes the lever's own compaction through the feature's hook (the engine does not). */
  let ownHook = false
  const edges: CompactionEdges = {
    submit: async (text) => {
      if (h.rejectSubmits > 0) {
        h.rejectSubmits -= 1
        throw new Error('submit failed')
      }
      if (h.dropSubmits > 0) {
        h.dropSubmits -= 1
        return { drop: 'a Stop stands' } as never
      }
      h.submits.push(text)
      return { text }
    },
    compact: async () => {
      h.compacts += 1
      const answer = h.compactAnswers.shift() ?? {}
      if (answer instanceof Error) throw answer
      if (ownHook && answer.skip === undefined) c.compacted('plugin', undefined, { messages: [] })
      return answer.skip === undefined ? { messages: [] } : { skip: answer.skip }
    },
    after: (fn) => {
      if (h.afterThrows > 0) {
        h.afterThrows -= 1
        throw new Error('clock refused')
      }
      h.scheduled.push(fn)
    },
    delay: (ms, fn) => {
      if (h.delayThrows > 0) {
        h.delayThrows -= 1
        throw new Error('clock refused')
      }
      h.delayed.push({ ms, fn })
    },
    log: (text, to) => void h.logs.push({ text, to }),
    leverLanded: () => {
      h.leverLandings += 1
      if (h.leverLandedThrows) throw new Error('listener failed')
    },
  }
  c.start(edges)
  const readBreakdown = async () => {
    h.breakdownReads += 1
    return { startedAt: 0, context: { window: 1_000_000, breakdown: h.window ?? undefined }, rateLimits: [] } as never
  }
  const measure = async (tokens: number | undefined, window = 1_000_000) => {
    await c.measure({ tokens, window }, readBreakdown)
    await tick()
  }
  /** Run what turnComplete scheduled, as the engine's clock would once the hook returned. */
  const runScheduled = async () => {
    for (const fn of h.scheduled.splice(0)) fn()
    await tick()
  }
  const turnEnd = async (reason = 'answer', agentId?: string) => {
    c.turnComplete(agentId, reason)
    await runScheduled()
  }
  const transcript = () => h.logs.filter((l) => l.to === 'transcript').map((l) => l.text)
  const debugLog = () => h.logs.filter((l) => l.to === 'debug').map((l) => l.text)
  const routeOwnCompaction = () => void (ownHook = true)
  /** End the continuation window: run the delay() timers, as the engine's clock would after it. */
  const closeWindow = async () => {
    for (const t of h.delayed.splice(0)) t.fn()
    await tick()
  }
  return { h, c, measure, turnEnd, runScheduled, closeWindow, transcript, debugLog, routeOwnCompaction }
}

describe('pure core', () => {
  test('compaction runs for the ws package only', () => {
    expect(compactionEnabledFor('ws')).toBe(true)
    expect(compactionEnabledFor('wsflow')).toBe(false)
    expect(compactionEnabledFor('')).toBe(false)
  })

  test('thresholds sit at the advisory and one and two thirds of the way to the auto-compact point', () => {
    expect(T.advisory).toBe(100_000)
    expect(T.hard).toBe(170_000)
    expect(Math.round(T.m1)).toBe(123_333)
    expect(Math.round(T.m2)).toBe(146_667)
  })

  test('thresholds are off when auto compaction is off, its point is unknown, or not above the advisory', () => {
    expect(thresholdsFor({ ...WINDOW, isAutoCompactEnabled: false })).toBeUndefined()
    expect(thresholdsFor({ rawMaxTokens: RAW, isAutoCompactEnabled: true })).toBeUndefined()
    expect(thresholdsFor({ ...WINDOW, autoCompactThreshold: 100_000 })).toBeUndefined()
    expect(thresholdsFor({ ...WINDOW, autoCompactThreshold: 90_000 })).toBeUndefined()
  })

  test('the level counts the thresholds usage has reached', () => {
    expect(levelAt(T, 99_999)).toBe(0)
    expect(levelAt(T, 100_000)).toBe(1)
    expect(levelAt(T, 123_334)).toBe(2)
    expect(levelAt(T, 146_667)).toBe(3)
    expect(levelAt(T, 400_000)).toBe(3)
  })

  test('the latch baselines, delivers the advisory first, covers skipped thresholds, re-arms, and holds while blocked', () => {
    expect(observe(undefined, 2, true)).toEqual({ latch: 2 })
    expect(observe(0, 1, true)).toEqual({ latch: 1, deliver: { kind: 'advisory' } })
    // A milestone waits for the advisory: a jump past both delivers the advisory and latches past them.
    expect(observe(0, 3, true)).toEqual({ latch: 3, deliver: { kind: 'advisory' } })
    expect(observe(1, 2, true)).toEqual({ latch: 2, deliver: { kind: 'milestone', which: 1 } })
    expect(observe(1, 3, true)).toEqual({ latch: 3, deliver: { kind: 'milestone', which: 2 } })
    expect(observe(2, 2, true)).toEqual({ latch: 2 })
    expect(observe(3, 1, true)).toEqual({ latch: 1 })
    expect(observe(0, 1, false)).toEqual({ latch: 0 })
    expect(observe(3, 0, false)).toEqual({ latch: 0 })
  })

  test('the lever accepts one trimmed line from the main session only', () => {
    expect(checkLeverCall(undefined, '  continue Phase 2  ')).toEqual({ continuation: 'continue Phase 2' })
    expect(checkLeverCall('sub-1', 'x')).toEqual({ deny: 'request_compaction compacts the main session only; called from subagent sub-1.' })
    const required = { error: 'request_compaction: continuation (a non-empty, one-line string) is required.' }
    expect(checkLeverCall(undefined, '')).toEqual(required)
    expect(checkLeverCall(undefined, '   ')).toEqual(required)
    expect(checkLeverCall(undefined, undefined)).toEqual(required)
    expect(checkLeverCall(undefined, 42)).toEqual(required)
    expect(checkLeverCall(undefined, 'one\ntwo')).toEqual({ error: 'request_compaction: continuation must be one line.' })
    expect(checkLeverCall(undefined, 'one\rtwo')).toEqual({ error: 'request_compaction: continuation must be one line.' })
  })
})

describe('pinned prose', () => {
  test('advisory', () => {
    expect(advisoryText(T, 104_000)).toBe(
      `[ws compaction] Context usage is 52% of the window (advisory point: 50%). Claude Code compacts automatically at about 85%.

This is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.

From here on, look for a good moment to compact before 85%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, first bring durable records up to date with anything still needed that only this context holds, then call the compaction request tool with a one-line continuation and end your turn. Compacting at a moment you choose keeps the work coherent; at 85% it happens automatically, mid-work if need be. Reminders follow at 62% and 73%.

If now is not such a moment, end this turn without replying and keep looking for one as you work.`,
    )
  })

  test('milestones', () => {
    expect(milestoneText(1, T, 125_000)).toBe(
      '[ws compaction] Context window: 63% / 85% (automatic compaction point). Past 85%, Claude Code compacts automatically, mid-work if need be. From here on, a good boundary is worth taking; if this is one, update durable records and call the compaction request tool, as the advisory describes. Otherwise end this turn without replying.',
    )
    expect(milestoneText(2, T, 150_000)).toBe(
      '[ws compaction] Context window: 75% / 85% (automatic compaction point). This is the last reading before automatic compaction. Compacting at a boundary you choose keeps the work coherent; take this one if it fits, update durable records and call the compaction request tool, as the advisory describes. Otherwise end this turn without replying, and take the next good boundary.',
    )
  })

  test('lever description and result', () => {
    expect(LEVER_DESCRIPTION).toBe(
      'Request compaction of this conversation when the current turn ends. Before calling, bring durable records (tickets, notes, agenda) up to date with anything still needed that only this context holds. `continuation` is one line naming what to do next (for example "continue Phase 2 implementation"); it is sent as your next prompt after compaction. Main session only.',
    )
    expect(LEVER_RESULT).toBe(
      'Compaction requested. End your turn now without further tool calls; compaction runs when the turn ends, and your continuation resumes the work.',
    )
  })

  test('continuations', () => {
    expect(continuationText('continue Phase 2')).toBe('[ws compaction] Continue after compaction: continue Phase 2')
    expect(skippedContinuationText('a hook said no', 'continue Phase 2')).toBe(
      '[ws compaction] Compaction did not run (a hook said no); your context is unchanged.\nContinue: continue Phase 2',
    )
  })
})

describe('thresholds', () => {
  test('the first usage baselines the latch, so a session resumed past the advisory gets none', async () => {
    const { h, measure } = harness()
    await measure(undefined)
    expect(h.breakdownReads).toBe(0)
    await measure(110_000)
    expect(h.submits).toEqual([])
    await measure(115_000)
    expect(h.submits).toEqual([])
  })

  test('each crossing submits one wake message: the advisory, then each milestone, never twice', async () => {
    const { h, measure } = harness()
    await measure(50_000)
    await measure(104_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000)])
    await measure(110_000)
    await measure(125_000)
    await measure(130_000)
    await measure(150_000)
    await measure(160_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000), milestoneText(1, T, 125_000), milestoneText(2, T, 150_000)])
    expect(h.breakdownReads).toBe(1)
  })

  test('a jump past the advisory and a milestone delivers only the advisory', async () => {
    const { h, measure } = harness()
    await measure(50_000)
    await measure(130_000)
    await measure(140_000)
    expect(h.submits).toEqual([advisoryText(T, 130_000)])
    await measure(150_000)
    expect(h.submits).toEqual([advisoryText(T, 130_000), milestoneText(2, T, 150_000)])
  })

  test('usage below the latch re-arms it', async () => {
    const { h, measure } = harness()
    await measure(50_000)
    await measure(104_000)
    await measure(60_000)
    await measure(102_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000), advisoryText(T, 102_000)])
  })

  test('the breakdown is re-read when the model window changes, and not cached while absent', async () => {
    const { h, measure } = harness({ window: null })
    await measure(50_000)
    await measure(60_000)
    expect(h.breakdownReads).toBe(2)
    h.window = WINDOW
    await measure(60_000)
    await measure(70_000)
    expect(h.breakdownReads).toBe(3)
    await measure(70_000, 500_000)
    expect(h.breakdownReads).toBe(4)
  })

  test('with auto compaction off no threshold message is sent, and the lever still works', async () => {
    const { h, c, measure, turnEnd, closeWindow } = harness({ window: { isAutoCompactEnabled: false, autoCompactThreshold: undefined } })
    await measure(50_000)
    await measure(190_000)
    expect(h.submits).toEqual([])
    expect(c.lever(undefined, 'go on')).toEqual({ result: LEVER_RESULT })
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([continuationText('go on')])
  })

  test('no threshold message while a request is pending; the crossing waits for the next reading after it settles', async () => {
    const { h, c, measure, turnEnd, closeWindow } = harness()
    await measure(50_000)
    c.lever(undefined, 'go on')
    await measure(104_000)
    expect(h.submits).toEqual([])
    h.compactAnswers.push({ skip: 'not now' })
    await turnEnd()
    // Nor while its continuation is held for the window.
    await measure(104_500)
    expect(h.submits).toEqual([])
    await closeWindow()
    expect(h.submits).toEqual([skippedContinuationText('not now', 'go on')])
    await measure(105_000)
    expect(h.submits).toEqual([skippedContinuationText('not now', 'go on'), advisoryText(T, 105_000)])
  })

  test('a landed compaction resets the latch: usage still past the advisory after it gets the advisory again', async () => {
    const { h, c, measure } = harness()
    await measure(50_000)
    await measure(104_000)
    c.compacted('manual', undefined, { messages: [] })
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000), advisoryText(T, 101_000)])
    expect(h.breakdownReads).toBe(2)
  })

  test('a reading that awaited the breakdown across a landed compaction is dropped', async () => {
    const { h, c } = harness()
    let release!: () => void
    const gate = new Promise<void>((r) => (release = r))
    const usage = { startedAt: 0, context: { window: 1, breakdown: WINDOW }, rateLimits: [] } as never
    const late = async () => {
      await gate
      return usage
    }
    // The first reading baselines; the second awaits a fresh read (a new window) across a compaction.
    await c.measure({ tokens: 50_000, window: 1 }, async () => usage)
    const pending = c.measure({ tokens: 150_000, window: 2 }, late)
    c.compacted('auto', undefined, { messages: [] })
    release()
    await pending
    await tick()
    expect(h.submits).toEqual([])
  })

  test('session end starts the latch over: the next conversation baselines', async () => {
    const { h, c, measure } = harness()
    await measure(50_000)
    c.end('clear')
    await measure(120_000)
    expect(h.submits).toEqual([])
  })
})

describe('lever', () => {
  test('a request compacts after the next main-thread turn end, then submits its continuation once', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    expect(c.lever(undefined, 'continue Phase 2')).toEqual({ result: LEVER_RESULT })
    c.turnComplete(undefined, 'answer')
    expect(h.compacts).toBe(0) // scheduled, not run inside the hook
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([]) // held for the window
    expect(h.delayed.map((t) => t.ms)).toEqual([CONTINUATION_WINDOW_MS])
    await closeWindow()
    expect(h.submits).toEqual([continuationText('continue Phase 2')])
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('subagent and aborted turn ends leave the request pending', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, 'go on')
    await turnEnd('answer', 'sub-1')
    await turnEnd('aborted')
    expect(h.compacts).toBe(0)
    await turnEnd('error')
    expect(h.compacts).toBe(1)
  })

  test('refusals and tool errors record nothing', async () => {
    const { h, c, turnEnd } = harness()
    expect(c.lever('sub-1', 'go on')).toEqual({ deny: 'request_compaction compacts the main session only; called from subagent sub-1.' })
    expect(c.lever(undefined, 'a\nb')).toEqual({
      isError: true,
      result: 'request_compaction: continuation must be one line.',
      text: 'request_compaction: continuation must be one line.',
    })
    await turnEnd()
    expect(h.compacts).toBe(0)
  })

  test('a repeat call replaces the continuation', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    c.lever(undefined, 'first')
    c.lever(undefined, 'second')
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([continuationText('second')])
  })

  test('a repeat call after another compaction landed keeps the compact step dropped', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, 'first')
    c.compacted('manual', undefined, { messages: [] })
    c.lever(undefined, 'second')
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([continuationText('second')])
  })

  test('a repeat call while the compact step is scheduled keeps it, and the step carries the new continuation', async () => {
    const { h, c, runScheduled, closeWindow } = harness()
    c.lever(undefined, 'first')
    c.turnComplete(undefined, 'answer')
    expect(h.scheduled.length).toBe(1)
    c.lever(undefined, 'second')
    await runScheduled()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([continuationText('second')])
  })

  test('a compact step that could not be scheduled is scheduled again at the next turn end', async () => {
    const { h, c, turnEnd, closeWindow, debugLog } = harness()
    c.lever(undefined, 'go on')
    h.afterThrows = 1
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(debugLog()).toContain('ws compaction: the compaction step was not scheduled (Error: clock refused); retrying at the next turn end')
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([continuationText('go on')])
  })

  test('a compact step whose dispatch never ran is scheduled again at the next turn end, and the stale one stays inert', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    c.lever(undefined, 'go on')
    c.turnComplete(undefined, 'answer')
    const refused = h.scheduled.splice(0)
    expect(refused.length).toBe(1)
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([continuationText('go on')])
    refused[0]!()
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('a rejected compaction keeps the request for the next turn end, with no timer retry', async () => {
    const { h, c, turnEnd, runScheduled, closeWindow } = harness()
    c.lever(undefined, 'go on')
    h.compactAnswers.push(new Error('a turn is running'))
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([])
    await runScheduled()
    expect(h.compacts).toBe(1)
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(2)
    expect(h.submits).toEqual([continuationText('go on')])
  })

  test('a rejected compaction is noted in the transcript once per request', async () => {
    const { h, c, turnEnd, closeWindow, transcript, debugLog } = harness()
    const rejected = 'ws compaction: the requested compaction was rejected (Error: a turn is running); retrying at the next turn end'
    c.lever(undefined, 'go on')
    h.compactAnswers.push(new Error('a turn is running'), new Error('a turn is running'))
    await turnEnd()
    expect(transcript()).toEqual([rejected])
    await turnEnd()
    expect(h.compacts).toBe(2)
    expect(transcript()).toEqual([rejected])
    expect(debugLog()).toContain(rejected)
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([continuationText('go on')])
    c.lever(undefined, 'again')
    h.compactAnswers.push(new Error('a turn is running'))
    await turnEnd()
    expect(transcript()).toEqual([rejected, rejected])
  })

  test('a veto still submits the continuation, with the note, and says why in the transcript', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, 'go on')
    h.compactAnswers.push({ skip: 'blocked by policy' })
    await turnEnd()
    expect(h.submits).toEqual([])
    await closeWindow()
    expect(h.submits).toEqual([skippedContinuationText('blocked by policy', 'go on')])
    expect(transcript()).toEqual(['ws compaction: the requested compaction did not run (blocked by policy); the continuation follows with a note'])
  })

  test('a compaction landing elsewhere first drops the compact step; the continuation follows at the next turn end', async () => {
    for (const trigger of ['manual', 'auto', 'plugin'] as const) {
      const { h, c, turnEnd, transcript } = harness()
      c.lever(undefined, 'go on')
      c.compacted(trigger, undefined, { messages: [] })
      expect(transcript()).toEqual([
        `ws compaction: a ${trigger} compaction landed first, so the requested compaction is dropped; its continuation follows at the next turn end`,
      ])
      expect(h.submits).toEqual([])
      await turnEnd('aborted')
      expect(h.submits).toEqual([])
      await turnEnd()
      expect(h.compacts).toBe(0)
      expect(h.submits).toEqual([continuationText('go on')])
    }
  })

  test('a compaction landing between the turn end and the scheduled step drops the compact step', async () => {
    const { h, c, runScheduled } = harness()
    c.lever(undefined, 'go on')
    c.turnComplete(undefined, 'answer')
    c.compacted('auto', undefined, { messages: [] })
    await runScheduled()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([continuationText('go on')])
  })

  test('a subagent or fork compaction, a precompute, and a vetoed compaction do not count as landing', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, 'go on')
    c.compacted('auto', 'sub-1', { messages: [] })
    c.compacted('precompute', undefined, { messages: [] })
    c.compacted('manual', undefined, { skip: 'no' })
    expect(transcript()).toEqual([])
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([continuationText('go on')])
  })

  test("the lever's own landing resets the latch and the breakdown", async () => {
    const { h, c, measure, turnEnd, closeWindow, transcript } = harness()
    await measure(50_000)
    await measure(104_000)
    c.lever(undefined, 'go on')
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(transcript()).toEqual([])
    await closeWindow()
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000), continuationText('go on'), advisoryText(T, 101_000)])
    expect(h.breakdownReads).toBe(2)
    expect(h.leverLandings).toBe(1)
  })

  test("should the feature's hook see the lever's own landing, it is neither another's nor counted twice", async () => {
    const { h, c, measure, turnEnd, closeWindow, transcript, routeOwnCompaction } = harness()
    routeOwnCompaction()
    await measure(50_000)
    await measure(104_000)
    c.lever(undefined, 'go on')
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(transcript()).toEqual([])
    await closeWindow()
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000), continuationText('go on'), advisoryText(T, 101_000)])
    expect(h.breakdownReads).toBe(2)
  })

  test("a vetoed lever compaction keeps the latch", async () => {
    const { h, c, measure, turnEnd, closeWindow } = harness()
    await measure(50_000)
    await measure(104_000)
    c.lever(undefined, 'go on')
    h.compactAnswers.push({ skip: 'no' })
    await turnEnd()
    await closeWindow()
    await measure(105_000)
    expect(h.submits).toEqual([advisoryText(T, 104_000), skippedContinuationText('no', 'go on')])
    expect(h.breakdownReads).toBe(1)
    expect(h.leverLandings).toBe(0)
  })

  test("only the lever's own landing is reported as the lever's; a throwing listener changes nothing", async () => {
    const { h, c, turnEnd, closeWindow, debugLog } = harness()
    c.compacted('manual', undefined, { messages: [] })
    expect(h.leverLandings).toBe(0)
    c.lever(undefined, 'go on')
    h.compactAnswers.push(new Error('a turn is running'))
    await turnEnd()
    expect(h.leverLandings).toBe(0)
    expect(debugLog().some((l) => l.includes('landing notification failed'))).toBe(false)
    h.leverLandedThrows = true
    await turnEnd()
    await closeWindow()
    expect(h.leverLandings).toBe(1)
    expect(h.submits).toEqual([continuationText('go on')])
    expect(debugLog().some((l) => l.includes('landing notification failed'))).toBe(true)
  })

  test('session end drops a pending request', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, 'go on')
    c.end('clear')
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([])
  })

  test('a dropped continuation is recorded in the transcript', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, 'go on')
    h.dropSubmits = 1
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (dropped: a Stop stands):\n${continuationText('go on')}`])
  })

  test('a continuation whose submit rejects is recorded in the transcript', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, 'go on')
    h.rejectSubmits = 1
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (Error: submit failed):\n${continuationText('go on')}`])
  })

  test('a threshold message whose submit rejects is logged to debug only', async () => {
    const { h, measure, transcript, debugLog } = harness()
    await measure(50_000)
    h.rejectSubmits = 1
    await measure(104_000)
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([])
    expect(debugLog()).toContain('ws compaction: a prompt was not submitted (Error: submit failed)')
  })
})

describe('continuation window', () => {
  test('a prompt entering within the window takes the continuation; the window end then submits nothing', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    expect(c.claim()).toBeUndefined()
    c.lever(undefined, 'go on')
    await turnEnd()
    const claimed = c.claim()
    expect(claimed?.text).toBe(continuationText('go on'))
    expect(c.claim()).toBeUndefined()
    await closeWindow()
    expect(h.submits).toEqual([])
  })

  test('a vetoed compaction holds its noted continuation the same way', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    c.lever(undefined, 'go on')
    h.compactAnswers.push({ skip: 'blocked by policy' })
    await turnEnd()
    expect(c.claim()?.text).toBe(skippedContinuationText('blocked by policy', 'go on'))
    await closeWindow()
    expect(h.submits).toEqual([])
  })

  test('a prompt that took the continuation but did not enter gives it back: submitted once, and not again at the window end', async () => {
    const { h, c, turnEnd, runScheduled, closeWindow, debugLog } = harness()
    c.lever(undefined, 'go on')
    await turnEnd()
    const claimed = c.claim()!
    claimed.release('dropped: a Stop stands')
    await tick()
    expect(h.submits).toEqual([]) // not inside the prompt.submit hook that released it
    await runScheduled()
    expect(h.submits).toEqual([continuationText('go on')])
    expect(debugLog()).toContain(
      'ws compaction: the prompt carrying the continuation did not enter (dropped: a Stop stands); submitting the continuation',
    )
    claimed.release('again')
    await closeWindow()
    expect(h.submits).toEqual([continuationText('go on')])
  })

  test('a continuation owed after another compaction landed first is submitted at the turn end, not held', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, 'go on')
    c.compacted('auto', undefined, { messages: [] })
    await turnEnd()
    expect(h.submits).toEqual([continuationText('go on')])
    expect(h.delayed).toEqual([])
    expect(c.claim()).toBeUndefined()
  })

  test('a session end inside the window records the held continuation in the transcript and submits nothing', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, 'go on')
    await turnEnd()
    c.end('clear')
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (session.end clear):\n${continuationText('go on')}`])
    expect(c.claim()).toBeUndefined()
    await closeWindow()
    expect(h.submits).toEqual([])
  })

  test('a release whose submit cannot be scheduled records the continuation in the transcript', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, 'go on')
    await turnEnd()
    h.afterThrows = 1
    c.claim()!.release('dropped: a Stop stands')
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (Error: clock refused):\n${continuationText('go on')}`])
  })

  test('a window that cannot start submits the continuation at once', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, 'go on')
    h.delayThrows = 1
    await turnEnd()
    expect(h.submits).toEqual([continuationText('go on')])
    expect(c.claim()).toBeUndefined()
  })
})

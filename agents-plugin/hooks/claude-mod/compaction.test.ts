import { describe, expect, test } from 'claude-code/testing'

import { CONTINUATION_WINDOW_MS, createCompaction, type CompactionEdges } from './compaction'
import {
  advisoryText,
  checkLeverCall,
  levelAt,
  LEVER_DESCRIPTION,
  LEVER_RESULT,
  LEVER_RESULT_NO_RESUME,
  milestoneText,
  observe,
  parseAdvisory,
  resolveAdvisory,
  resumeText,
  thresholdsFor,
  vetoedResumeText,
  type CompactionWindow,
} from './timing'

// The harness runs as the plugin `ws` unless a test names another, so the
// texts pinned below are the ws rendering.
const RESUME_TEXT = resumeText('ws')

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
function harness(opts: { window?: Partial<CompactionWindow> | null; readAdvisory?: () => Promise<unknown>; plugin?: string } = {}) {
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
    /** Whether a main-thread turn runs, as register.ts tracks it. */
    turnRunning: false,
    /** What the next fresh usage reads return; undefined repeats the last measure's figures. */
    live: undefined as { tokens: number | undefined; window: number } | undefined,
    /** The figures of the last measure. */
    measured: { tokens: undefined as number | undefined, window: 1_000_000 },
    usageReads: 0,
    /** The next usage reads that reject. */
    rejectUsageReads: 0,
    /** Set, a usage read settles only once it resolves. */
    usageGate: undefined as Promise<void> | undefined,
  }
  const c = createCompaction()
  /** Whether the fake engine routes the lever's own compaction through the feature's hook (the engine does not). */
  let ownHook = false
  const edges: CompactionEdges = {
    submit: async ({ text }) => {
      if (h.rejectSubmits > 0) {
        h.rejectSubmits -= 1
        throw new Error('submit failed')
      }
      if (h.dropSubmits > 0) {
        h.dropSubmits -= 1
        return { drop: 'a Stop stands' }
      }
      h.submits.push(text)
      return {}
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
    plugin: opts.plugin ?? 'ws',
    turnRunning: () => h.turnRunning,
    readUsage: async () => {
      h.usageReads += 1
      if (h.usageGate) await h.usageGate
      if (h.rejectUsageReads > 0) {
        h.rejectUsageReads -= 1
        throw new Error('usage unavailable')
      }
      return { ...(h.live ?? h.measured) }
    },
    leverLanded: () => {
      h.leverLandings += 1
      if (h.leverLandedThrows) throw new Error('listener failed')
    },
    readAdvisory: opts.readAdvisory,
  }
  c.start(edges)
  const readBreakdown = async () => {
    h.breakdownReads += 1
    return { startedAt: 0, context: { window: 1_000_000, breakdown: h.window ?? undefined }, rateLimits: [] } as never
  }
  const measure = async (tokens: number | undefined, window = 1_000_000) => {
    h.measured = { tokens, window }
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
  /** A turn.start, whatever started it: the main thread's when agentId is undefined. */
  const turnStart = (agentId?: string) => c.turnStarted(agentId)
  const transcript = () => h.logs.filter((l) => l.to === 'transcript').map((l) => l.text)
  const debugLog = () => h.logs.filter((l) => l.to === 'debug').map((l) => l.text)
  const routeOwnCompaction = () => void (ownHook = true)
  /** End the resume window: run the delay() timers, as the engine's clock would after it. */
  const closeWindow = async () => {
    for (const t of h.delayed.splice(0)) t.fn()
    await tick()
  }
  /** A second session.start with the same edges. */
  const restart = () => c.start(edges)
  return { h, c, measure, turnEnd, turnStart, runScheduled, closeWindow, transcript, debugLog, routeOwnCompaction, restart }
}

/** A promise the test settles. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (err: unknown) => void
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)))
  return { promise, resolve, reject }
}

const SUBAGENT_DENY = 'request_compaction compacts the main session only; called from subagent sub-1.'

describe('pure core', () => {
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

  test('only a literal true sets the lever flag; any other value reads as unset, never as an error; a subagent is refused', () => {
    expect(checkLeverCall(undefined, true)).toEqual({ resume: true })
    // A non-boolean, and a leftover continuation string (one line or several), read as unset.
    for (const value of [undefined, false, 'true', 1, 'go on', 'one\ntwo', 'one\rtwo', '', null]) {
      expect(checkLeverCall(undefined, value)).toEqual({ resume: false })
    }
    expect(checkLeverCall('sub-1', true)).toEqual({ deny: SUBAGENT_DENY })
    expect(checkLeverCall('sub-1', undefined)).toEqual({ deny: SUBAGENT_DENY })
  })
})

describe('pinned prose', () => {
  test('advisory', () => {
    expect(advisoryText('ws', T, 104_000)).toBe(
      `[ws compaction] Context window at the end of the last turn: 52% (104k / 200k tokens) / 85% (automatic compaction point). This is past the advisory point (50%).

This is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.

From here on, look for a good moment to compact before 85%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, first bring durable records up to date with anything still needed that only this context holds, then call the compaction request tool and end your turn. Set its \`continue\` flag only when work must resume by itself after compaction; leave it unset when the next turn will come from the human or from a background agent's notification. Compacting at a moment you choose keeps the work coherent; at 85% it happens automatically, mid-work if need be. Reminders follow at 62% and 73%.

If now is not such a moment, end this turn without replying and keep looking for one as you work.`,
    )
  })

  test('milestones', () => {
    expect(milestoneText('ws', 1, T, 125_000)).toBe(
      '[ws compaction] Context window at the end of the last turn: 63% (125k / 200k tokens) / 85% (automatic compaction point). Past 85%, Claude Code compacts automatically, mid-work if need be. From here on, a good boundary is worth taking; if this is one, update durable records and call the compaction request tool, as the advisory describes (the `continue` flag only when work must resume by itself). Otherwise end this turn without replying.',
    )
    expect(milestoneText('ws', 2, T, 150_000)).toBe(
      '[ws compaction] Context window at the end of the last turn: 75% (150k / 200k tokens) / 85% (automatic compaction point). This is the last reading before automatic compaction. Compacting at a boundary you choose keeps the work coherent; take this one if it fits, update durable records and call the compaction request tool, as the advisory describes (the `continue` flag only when work must resume by itself). Otherwise end this turn without replying, and take the next good boundary.',
    )
  })

  test('lever description and result', () => {
    expect(LEVER_DESCRIPTION).toBe(
      "Request compaction of this conversation when the current turn ends. Before calling, bring durable records (tickets, notes, agenda) up to date with anything still needed that only this context holds. Set `continue` to true only when work must resume by itself after compaction: a fixed resume prompt then follows the compaction, unless another prompt (the human's, or a background agent's notification) has already started the next turn. Leave it unset when the next turn will come from the human or from a background agent's notification. Main session only.",
    )
    expect(LEVER_RESULT).toBe(
      'Compaction requested. End your turn now without further tool calls; compaction runs when the turn ends, and a resume prompt follows it unless another prompt has already started the next turn.',
    )
    expect(LEVER_RESULT_NO_RESUME).toBe(
      "Compaction requested. End your turn now without further tool calls; compaction runs when the turn ends, and no prompt follows it: the next turn comes from the human or from a background agent's notification.",
    )
  })

  test('resume prompts', () => {
    expect(RESUME_TEXT).toBe('[ws compaction] Compaction landed; resume the work in progress from the summary and your durable records.')
    expect(vetoedResumeText('ws', 'a hook said no')).toBe(
      '[ws compaction] Compaction did not run (a hook said no); your context is unchanged. Resume the work in progress.',
    )
  })

  test('a plugin not named ws spells its own name in every prompt, notice and log line, and ws nowhere', async () => {
    const { h, c, measure, turnEnd, closeWindow, transcript, debugLog } = harness({ plugin: 'pkgx' })
    await measure(50_000)
    await measure(104_000)
    await measure(125_000)
    // A lever whose compaction lands, then one a hook vetoes, each with a resume prompt.
    c.lever(undefined, true)
    await turnEnd()
    await closeWindow()
    c.lever(undefined, true)
    h.compactAnswers.push({ skip: 'blocked by policy' })
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([
      advisoryText('pkgx', T, 104_000),
      milestoneText('pkgx', 1, T, 125_000),
      resumeText('pkgx'),
      vetoedResumeText('pkgx', 'blocked by policy'),
    ])
    for (const text of h.submits) expect(text.startsWith('[pkgx compaction] '), text).toBe(true)
    expect(transcript().length).toBeGreaterThan(0)
    expect(debugLog().length).toBeGreaterThan(0)
    for (const text of [...transcript(), ...debugLog()]) expect(text.startsWith('pkgx compaction: '), text).toBe(true)
    for (const text of [...h.submits, ...h.logs.map((l) => l.text)]) expect(text).not.toMatch(/(?<![A-Za-z])ws(?![A-Za-z])/)
  })
})

describe('configured advisory point', () => {
  // The same corpus the server-side test runs against the key's manifest pattern.
  const ACCEPTED = ['50', '50%', '1', '1%', '100', '100%', '150k', '1.5m', '0.5k', '1.0004k', '007k', '50%|150k', '150k|50%', '100|1m']
  const REJECTED = ['0', '0%', '101', '101%', '050', '50.5', '-5', '', '0k', '0.0m', '00.00k', '150K', '1.5M', '15ok', 'k', '.5k', '5.k',
    '50%%', '50%|60%', '150k|1m', '50%|150k|1m', '50% | 150k', ' 50', '50 ', '|50%', '50%|', '1e3k']

  test('the grammar accepts a percent, a token count, or one of each joined by |', () => {
    for (const value of ACCEPTED) expect(parseAdvisory(value)).toBeDefined()
    for (const value of REJECTED) expect(parseAdvisory(value)).toBeUndefined()
    expect(parseAdvisory('40')).toEqual({ percent: 40 })
    expect(parseAdvisory('1.5m')).toEqual({ tokens: 1_500_000 })
    expect(parseAdvisory('0.5k')).toEqual({ tokens: 500 })
    expect(parseAdvisory('150k|40%')).toEqual({ percent: 40, tokens: 150_000 })
  })

  test('a value that does not parse, or none, resolves to 50%', () => {
    for (const value of [undefined, null, 40, 'lots', '50%|60%', {}]) expect(resolveAdvisory(value)).toEqual({ percent: 50 })
  })

  test('a percent, a token count, and the earlier of a combined value set the advisory; milestones follow it', () => {
    expect(thresholdsFor(WINDOW, { percent: 40 })?.advisory).toBe(80_000)
    expect(thresholdsFor(WINDOW, { tokens: 110_000 })?.advisory).toBe(110_000)
    expect(thresholdsFor(WINDOW, { percent: 40, tokens: 150_000 })?.advisory).toBe(80_000)
    expect(thresholdsFor(WINDOW, { percent: 60, tokens: 110_000 })?.advisory).toBe(110_000)
    const t = thresholdsFor(WINDOW, { tokens: 110_000 })!
    expect(t.m1).toBe(130_000)
    expect(t.m2).toBe(150_000)
    expect(t.hard).toBe(HARD)
  })

  test('a token term at or beyond the auto-compact point leaves the percent term to decide, and alone sends nothing', () => {
    expect(thresholdsFor(WINDOW, { percent: 40, tokens: 180_000 })?.advisory).toBe(80_000)
    expect(thresholdsFor(WINDOW, { percent: 40, tokens: 400_000 })?.advisory).toBe(80_000)
    expect(thresholdsFor(WINDOW, { tokens: 180_000 })).toBeUndefined()
    expect(thresholdsFor(WINDOW, { tokens: HARD })).toBeUndefined()
    expect(thresholdsFor(WINDOW, { percent: 90 })).toBeUndefined()
    expect(thresholdsFor(WINDOW, { percent: 90, tokens: 400_000 })).toBeUndefined()
  })

  test('the advisory text names the configured point as its percent of the window', () => {
    const t = thresholdsFor(WINDOW, { tokens: 110_000 })!
    expect(advisoryText('ws', t, 112_000)).toContain(
      '[ws compaction] Context window at the end of the last turn: 56% (112k / 200k tokens) / 85% (automatic compaction point). This is past the advisory point (55%).',
    )
    expect(advisoryText('ws', t, 112_000)).toContain('Reminders follow at 65% and 75%.')
  })

  test('with no key set the advisory stays at 50%', async () => {
    const { h, measure } = harness({ readAdvisory: async () => null })
    await measure(10_000)
    await measure(99_000)
    expect(h.submits).toEqual([])
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 101_000)])
  })

  test('a configured point read at start moves the advisory and the milestones', async () => {
    const { h, measure } = harness({ readAdvisory: async () => '40%' })
    await measure(10_000)
    await measure(81_000)
    const t = thresholdsFor(WINDOW, { percent: 40 })!
    expect(h.submits).toEqual([advisoryText('ws', t, 81_000)])
    expect(h.submits[0]).toContain('This is past the advisory point (40%).')
    await measure(111_000)
    expect(h.submits.length).toBe(2)
  })

  test('a configured point with no threshold below the auto-compact point sends nothing', async () => {
    const { h, measure } = harness({ readAdvisory: async () => '180k' })
    await measure(10_000)
    await measure(160_000)
    await measure(169_000)
    expect(h.submits).toEqual([])
  })

  test('a malformed stored value and a failed read both fall back to 50%', async () => {
    for (const readAdvisory of [async () => 'lots', async () => Promise.reject(new Error('config_get reported an error')), () => {
      throw new Error('MCP server not resolved')
    }]) {
      const { h, measure, debugLog } = harness({ readAdvisory })
      await measure(10_000)
      await measure(81_000)
      expect(h.submits).toEqual([])
      await measure(101_000)
      expect(h.submits).toEqual([advisoryText('ws', T, 101_000)])
      expect(debugLog().some((l) => l.includes('advisory point'))).toBe(true)
    }
  })

  test('a read that settles after the first observation still applies to the session', async () => {
    const read = deferred<unknown>()
    const { h, measure } = harness({ readAdvisory: () => read.promise })
    await measure(10_000)
    expect(h.breakdownReads).toBe(1)
    read.resolve('40%')
    await tick()
    await measure(81_000)
    expect(h.breakdownReads).toBe(2)
    expect(h.submits).toEqual([advisoryText('ws', thresholdsFor(WINDOW, { percent: 40 })!, 81_000)])
  })

  test('a read that settles while an observation awaits the breakdown applies to it', async () => {
    const read = deferred<unknown>()
    const { h, c } = harness({ readAdvisory: () => read.promise })
    await c.measure({ tokens: 10_000, window: 1_000_000 }, async () => {
      read.resolve('40%')
      await tick()
      return { startedAt: 0, context: { window: 1_000_000, breakdown: WINDOW }, rateLimits: [] } as never
    })
    await c.measure({ tokens: 81_000, window: 1_000_000 }, async () => {
      throw new Error('not read again')
    })
    await tick()
    expect(h.submits).toEqual([advisoryText('ws', thresholdsFor(WINDOW, { percent: 40 })!, 81_000)])
  })

  test('each session start reads again, and a read from an earlier start is dropped', async () => {
    const reads = [deferred<unknown>(), deferred<unknown>()]
    let n = 0
    const { h, measure, restart } = harness({ readAdvisory: () => reads[n++].promise })
    restart()
    await tick()
    expect(n).toBe(2)
    reads[1].resolve(null)
    reads[0].resolve('40%')
    await measure(10_000)
    await measure(81_000)
    expect(h.submits).toEqual([])
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 101_000)])
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
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000)])
    await measure(110_000)
    await measure(125_000)
    await measure(130_000)
    await measure(150_000)
    await measure(160_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), milestoneText('ws', 1, T, 125_000), milestoneText('ws', 2, T, 150_000)])
    expect(h.breakdownReads).toBe(1)
  })

  test('a jump past the advisory and a milestone delivers only the advisory', async () => {
    const { h, measure } = harness()
    await measure(50_000)
    await measure(130_000)
    await measure(140_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 130_000)])
    await measure(150_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 130_000), milestoneText('ws', 2, T, 150_000)])
  })

  test('usage below the latch re-arms it', async () => {
    const { h, measure } = harness()
    await measure(50_000)
    await measure(104_000)
    await measure(60_000)
    await measure(102_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), advisoryText('ws', T, 102_000)])
  })

  test('a crossing measured while a main-thread turn runs delivers nothing until the turn ends, then once with the fresh read', async () => {
    const { h, measure, turnEnd, debugLog } = harness()
    await measure(50_000)
    h.turnRunning = true
    await measure(104_000)
    await measure(110_000)
    expect(h.submits).toEqual([])
    expect(h.usageReads).toBe(0)
    h.turnRunning = false
    h.live = { tokens: 118_000, window: 1_000_000 }
    await turnEnd()
    expect(h.submits).toEqual([advisoryText('ws', T, 118_000)])
    expect(h.submits[0]).toContain('59% (118k / 200k tokens)')
    expect(debugLog().filter((l) => l.includes('delivered'))).toEqual([
      'ws compaction: advisory delivered: crossing measured at 104000 tokens (52%), fresh read 118000 tokens (59%), compaction window 200000 tokens, model window 1000000 tokens',
    ])
    // The end-of-turn measure after the turn end, and a later turn end, deliver nothing again.
    await measure(118_000)
    await turnEnd()
    expect(h.submits).toEqual([advisoryText('ws', T, 118_000)])
    expect(h.usageReads).toBe(1)
  })

  test('a crossing measured while idle delivers at once, with the fresh read', async () => {
    const { h, measure, debugLog } = harness()
    await measure(50_000)
    h.live = { tokens: 106_000, window: 1_000_000 }
    await measure(104_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 106_000)])
    expect(debugLog()).toContain(
      'ws compaction: advisory delivered: crossing measured at 104000 tokens (52%), fresh read 106000 tokens (53%), compaction window 200000 tokens, model window 1000000 tokens',
    )
  })

  test('a crossing that grew past the next milestone by delivery delivers the higher level once', async () => {
    const { h, measure, turnEnd } = harness()
    await measure(50_000)
    await measure(104_000)
    h.turnRunning = true
    await measure(125_000)
    h.turnRunning = false
    h.live = { tokens: 150_000, window: 1_000_000 }
    await turnEnd()
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), milestoneText('ws', 2, T, 150_000)])
    await measure(155_000)
    expect(h.submits.length).toBe(2)
  })

  test('an aborted or a subagent turn end delivers nothing; the next idle measure delivers the crossing', async () => {
    const { h, measure, turnEnd } = harness()
    await measure(50_000)
    h.turnRunning = true
    await measure(104_000)
    h.turnRunning = false
    await turnEnd('aborted')
    await turnEnd('answer', 'sub-1')
    expect(h.submits).toEqual([])
    await measure(105_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 105_000)])
  })

  test('a fresh read that fails, or a main-thread turn starting during it, keeps the crossing for the next delivery point', async () => {
    const { h, c, measure, turnEnd, debugLog } = harness()
    await measure(50_000)
    h.rejectUsageReads = 1
    await measure(104_000)
    expect(h.submits).toEqual([])
    expect(debugLog()).toContain('ws compaction: usage not read for delivery (Error: usage unavailable); the crossing stays recorded')
    // A read that settles after a main-thread turn started delivers nothing.
    const gate = deferred<void>()
    h.usageGate = gate.promise
    c.turnComplete(undefined, 'answer')
    for (const fn of h.scheduled.splice(0)) fn()
    await tick()
    h.turnRunning = true
    gate.resolve()
    await tick()
    expect(h.submits).toEqual([])
    h.usageGate = undefined
    h.turnRunning = false
    await turnEnd()
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000)])
  })

  test('a fresh read without tokens shows the crossing figure; one under another model window delivers nothing yet', async () => {
    const { h, measure, turnEnd } = harness()
    await measure(50_000)
    h.live = { tokens: 104_000, window: 500_000 }
    await measure(104_000)
    expect(h.submits).toEqual([])
    h.live = { tokens: undefined, window: 1_000_000 }
    await turnEnd()
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000)])
  })

  test('a crossing withheld at the turn end by a pending request stays latched for a later measure', async () => {
    const { h, c, measure, turnEnd } = harness()
    await measure(50_000)
    h.turnRunning = true
    await measure(104_000)
    c.lever(undefined, false)
    h.turnRunning = false
    h.compactAnswers.push({ skip: 'not now' })
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([])
    expect(h.usageReads).toBe(0)
    await measure(106_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 106_000)])
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
    expect(c.lever(undefined, true)).toEqual({ result: LEVER_RESULT })
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('no threshold message while a request is pending; the crossing waits for the next reading after it settles', async () => {
    const { h, c, measure, turnEnd, closeWindow } = harness()
    await measure(50_000)
    c.lever(undefined, true)
    await measure(104_000)
    expect(h.submits).toEqual([])
    h.compactAnswers.push({ skip: 'not now' })
    await turnEnd()
    // Nor while its resume prompt is held for the window.
    await measure(104_500)
    expect(h.submits).toEqual([])
    await closeWindow()
    expect(h.submits).toEqual([vetoedResumeText('ws', 'not now')])
    await measure(105_000)
    expect(h.submits).toEqual([vetoedResumeText('ws', 'not now'), advisoryText('ws', T, 105_000)])
  })

  test('a landed compaction resets the latch: usage still past the advisory after it gets the advisory again', async () => {
    const { h, c, measure } = harness()
    await measure(50_000)
    await measure(104_000)
    c.compacted('manual', undefined, { messages: [] })
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), advisoryText('ws', T, 101_000)])
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
  test('a request with the flag compacts after the next main-thread turn end, then submits the resume prompt once', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    expect(c.lever(undefined, true)).toEqual({ result: LEVER_RESULT })
    c.turnComplete(undefined, 'answer')
    expect(h.compacts).toBe(0) // scheduled, not run inside the hook
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([]) // held for the window
    expect(h.delayed.map((t) => t.ms)).toEqual([CONTINUATION_WINDOW_MS])
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('subagent and aborted turn ends leave the request pending', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, true)
    await turnEnd('answer', 'sub-1')
    await turnEnd('aborted')
    expect(h.compacts).toBe(0)
    await turnEnd('error')
    expect(h.compacts).toBe(1)
  })

  test('a subagent call is refused and records nothing', async () => {
    const { h, c, turnEnd } = harness()
    expect(c.lever('sub-1', true)).toEqual({ deny: SUBAGENT_DENY })
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([])
  })

  test('a repeat call with the flag compacts once and resumes once', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    c.lever(undefined, true)
    c.lever(undefined, true)
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a repeat call after another compaction landed keeps the compact step dropped', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, true)
    c.compacted('manual', undefined, { messages: [] })
    c.lever(undefined, true)
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a repeat call while the compact step is scheduled keeps it, and the step reads the latest flag', async () => {
    const a = harness()
    a.c.lever(undefined, undefined)
    a.c.turnComplete(undefined, 'answer')
    expect(a.h.scheduled.length).toBe(1)
    a.c.lever(undefined, true)
    await a.runScheduled()
    await a.closeWindow()
    expect(a.h.compacts).toBe(1)
    expect(a.h.submits).toEqual([RESUME_TEXT])

    const b = harness()
    b.c.lever(undefined, true)
    b.c.turnComplete(undefined, 'answer')
    b.c.lever(undefined, false)
    await b.runScheduled()
    await b.closeWindow()
    expect(b.h.compacts).toBe(1)
    expect(b.h.delayed).toEqual([])
    expect(b.h.submits).toEqual([])
  })

  test('a compact step that could not be scheduled is scheduled again at the next turn end', async () => {
    const { h, c, turnEnd, closeWindow, debugLog } = harness()
    c.lever(undefined, true)
    h.afterThrows = 1
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(debugLog()).toContain('ws compaction: the compaction step was not scheduled (Error: clock refused); retrying at the next turn end')
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a compact step whose dispatch never ran is scheduled again at the next turn end, and the stale one stays inert', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    c.lever(undefined, true)
    c.turnComplete(undefined, 'answer')
    const refused = h.scheduled.splice(0)
    expect(refused.length).toBe(1)
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
    refused[0]!()
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('a rejected compaction keeps the request for the next turn end, with no timer retry', async () => {
    const { h, c, turnEnd, runScheduled, closeWindow } = harness()
    c.lever(undefined, true)
    h.compactAnswers.push(new Error('a turn is running'))
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([])
    await runScheduled()
    expect(h.compacts).toBe(1)
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(2)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a rejected compaction is noted in the transcript once per request', async () => {
    const { h, c, turnEnd, closeWindow, transcript, debugLog } = harness()
    const rejected = 'ws compaction: the requested compaction was rejected (Error: a turn is running); retrying at the next turn end'
    c.lever(undefined, true)
    h.compactAnswers.push(new Error('a turn is running'), new Error('a turn is running'))
    await turnEnd()
    expect(transcript()).toEqual([rejected])
    await turnEnd()
    expect(h.compacts).toBe(2)
    expect(transcript()).toEqual([rejected])
    expect(debugLog()).toContain(rejected)
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
    c.lever(undefined, true)
    h.compactAnswers.push(new Error('a turn is running'))
    await turnEnd()
    expect(transcript()).toEqual([rejected, rejected])
  })

  test('a veto with the flag still submits a resume prompt, with the note, and says why in the transcript', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    h.compactAnswers.push({ skip: 'blocked by policy' })
    await turnEnd()
    expect(h.submits).toEqual([])
    await closeWindow()
    expect(h.submits).toEqual([vetoedResumeText('ws', 'blocked by policy')])
    expect(transcript()).toEqual(['ws compaction: the requested compaction did not run (blocked by policy); a resume prompt follows with a note'])
  })

  test('a compaction landing elsewhere first drops the compact step; the resume prompt follows at the next turn end', async () => {
    for (const trigger of ['manual', 'auto', 'plugin'] as const) {
      const { h, c, turnEnd, transcript } = harness()
      c.lever(undefined, true)
      c.compacted(trigger, undefined, { messages: [] })
      expect(transcript()).toEqual([
        `ws compaction: a ${trigger} compaction landed first, so the requested compaction is dropped; its resume prompt follows at the next turn end`,
      ])
      expect(h.submits).toEqual([])
      await turnEnd('aborted')
      expect(h.submits).toEqual([])
      await turnEnd()
      expect(h.compacts).toBe(0)
      expect(h.submits).toEqual([RESUME_TEXT])
    }
  })

  test('a compaction landing between the turn end and the scheduled step drops the compact step', async () => {
    const { h, c, runScheduled } = harness()
    c.lever(undefined, true)
    c.turnComplete(undefined, 'answer')
    c.compacted('auto', undefined, { messages: [] })
    await runScheduled()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a subagent or fork compaction, a precompute, and a vetoed compaction do not count as landing', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    c.compacted('auto', 'sub-1', { messages: [] })
    c.compacted('precompute', undefined, { messages: [] })
    c.compacted('manual', undefined, { skip: 'no' })
    expect(transcript()).toEqual([])
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test("the lever's own landing resets the latch and the breakdown", async () => {
    const { h, c, measure, turnEnd, closeWindow, transcript } = harness()
    await measure(50_000)
    await measure(104_000)
    c.lever(undefined, true)
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(transcript()).toEqual([])
    await closeWindow()
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), RESUME_TEXT, advisoryText('ws', T, 101_000)])
    expect(h.breakdownReads).toBe(2)
    expect(h.leverLandings).toBe(1)
  })

  test("should the feature's hook see the lever's own landing, it is neither another's nor counted twice", async () => {
    const { h, c, measure, turnEnd, closeWindow, transcript, routeOwnCompaction } = harness()
    routeOwnCompaction()
    await measure(50_000)
    await measure(104_000)
    c.lever(undefined, true)
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(transcript()).toEqual([])
    await closeWindow()
    await measure(101_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), RESUME_TEXT, advisoryText('ws', T, 101_000)])
    expect(h.breakdownReads).toBe(2)
  })

  test('a vetoed lever compaction keeps the latch', async () => {
    const { h, c, measure, turnEnd, closeWindow } = harness()
    await measure(50_000)
    await measure(104_000)
    c.lever(undefined, true)
    h.compactAnswers.push({ skip: 'no' })
    await turnEnd()
    await closeWindow()
    await measure(105_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 104_000), vetoedResumeText('ws', 'no')])
    expect(h.breakdownReads).toBe(1)
    expect(h.leverLandings).toBe(0)
  })

  test("only the lever's own landing is reported as the lever's; a throwing listener changes nothing", async () => {
    const { h, c, turnEnd, closeWindow, debugLog } = harness()
    c.compacted('manual', undefined, { messages: [] })
    expect(h.leverLandings).toBe(0)
    c.lever(undefined, true)
    h.compactAnswers.push(new Error('a turn is running'))
    await turnEnd()
    expect(h.leverLandings).toBe(0)
    expect(debugLog().some((l) => l.includes('landing notification failed'))).toBe(false)
    h.leverLandedThrows = true
    await turnEnd()
    await closeWindow()
    expect(h.leverLandings).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
    expect(debugLog().some((l) => l.includes('landing notification failed'))).toBe(true)
  })

  test('session end drops a pending request', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, true)
    c.end('clear')
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([])
  })

  test('a dropped resume prompt is recorded in the transcript with its fixed text', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    h.dropSubmits = 1
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (dropped: a Stop stands):\n${RESUME_TEXT}`])
  })

  test('a resume prompt whose submit rejects is recorded in the transcript with its fixed text', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    h.rejectSubmits = 1
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (Error: submit failed):\n${RESUME_TEXT}`])
  })

  test('a lost vetoed resume prompt is recorded in the transcript with its fixed text', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    h.compactAnswers.push({ skip: 'blocked by policy' })
    h.dropSubmits = 1
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([
      'ws compaction: the requested compaction did not run (blocked by policy); a resume prompt follows with a note',
      `ws compaction: a prompt was not submitted (dropped: a Stop stands):\n${vetoedResumeText('ws', 'blocked by policy')}`,
    ])
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

describe('unset flag', () => {
  const unset: [string, unknown][] = [
    ['absent', undefined],
    ['false', false],
    ['the string "true"', 'true'],
    ['a number', 1],
    ['a leftover continuation string', 'go on'],
    ['a multi-line string', 'one\ntwo'],
  ]

  test('an absent, false, or non-boolean flag is accepted with the no-resume result', () => {
    for (const [, value] of unset) {
      const { c } = harness()
      expect(c.lever(undefined, value)).toEqual({ result: LEVER_RESULT_NO_RESUME })
    }
  })

  test('a subagent call is still refused, flag or not', () => {
    const { c } = harness()
    expect(c.lever('sub-1', undefined)).toEqual({ deny: SUBAGENT_DENY })
    expect(c.lever('sub-1', true)).toEqual({ deny: SUBAGENT_DENY })
  })

  test('its own landed compaction submits nothing and opens no window', async () => {
    for (const [, value] of unset) {
      const { h, c, turnEnd, closeWindow, transcript } = harness()
      c.lever(undefined, value)
      await turnEnd()
      expect(h.compacts).toBe(1)
      expect(h.leverLandings).toBe(1)
      expect(h.delayed).toEqual([])
      await closeWindow()
      expect(h.submits).toEqual([])
      expect(transcript()).toEqual([])
      // The request is settled: a later turn end compacts nothing.
      await turnEnd()
      expect(h.compacts).toBe(1)
    }
  })

  test('a landing through the hook also submits nothing', async () => {
    const { h, c, turnEnd, closeWindow, routeOwnCompaction } = harness()
    routeOwnCompaction()
    c.lever(undefined, 'go on')
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([])
  })

  test('a compaction landing elsewhere first drops the compact step and submits nothing', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, undefined)
    c.compacted('auto', undefined, { messages: [] })
    expect(transcript()).toEqual([
      'ws compaction: a auto compaction landed first, so the requested compaction is dropped; no resume was requested, so no prompt follows',
    ])
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([])
    // Settled: a later compaction lever call compacts again.
    c.lever(undefined, undefined)
    await turnEnd()
    expect(h.compacts).toBe(1)
  })

  test('a veto submits nothing, opens no window, and is told to the person in the transcript', async () => {
    const { h, c, turnEnd, closeWindow, transcript, debugLog } = harness()
    c.lever(undefined, undefined)
    h.compactAnswers.push({ skip: 'blocked by policy' })
    await turnEnd()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(h.delayed).toEqual([])
    expect(h.leverLandings).toBe(0)
    expect(transcript()).toEqual([
      'ws compaction: the requested compaction did not run (blocked by policy); no resume was requested, so no turn follows',
    ])
    expect(debugLog().some((l) => l.includes('blocked by policy'))).toBe(false)
  })

  test('a rejected compaction keeps the request and retries at the next turn end, then submits nothing', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, undefined)
    h.compactAnswers.push(new Error('a turn started'))
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual(['ws compaction: the requested compaction was rejected (Error: a turn started); retrying at the next turn end'])
    await turnEnd()
    await closeWindow()
    expect(h.compacts).toBe(2)
    expect(h.leverLandings).toBe(1)
    expect(h.submits).toEqual([])
  })

  test('a repeat call in both orders: the latest flag wins', async () => {
    const a = harness()
    a.c.lever(undefined, true)
    a.c.lever(undefined, undefined)
    await a.turnEnd()
    await a.closeWindow()
    expect(a.h.compacts).toBe(1)
    expect(a.h.delayed).toEqual([])
    expect(a.h.submits).toEqual([])

    const b = harness()
    b.c.lever(undefined, undefined)
    b.c.lever(undefined, true)
    await b.turnEnd()
    await b.closeWindow()
    expect(b.h.compacts).toBe(1)
    expect(b.h.submits).toEqual([RESUME_TEXT])
  })

  test('a repeat call with a false flag clears an earlier true after another compaction landed first', async () => {
    const { h, c, turnEnd } = harness()
    c.lever(undefined, true)
    c.compacted('auto', undefined, { messages: [] })
    c.lever(undefined, false)
    await turnEnd()
    expect(h.compacts).toBe(0)
    expect(h.submits).toEqual([])
  })
})

describe('resume window guard', () => {
  test('no turn started within the window: the resume prompt is submitted once at its end', async () => {
    const { h, c, turnEnd, turnStart, closeWindow } = harness()
    c.lever(undefined, true)
    await turnEnd()
    expect(h.delayed.map((t) => t.ms)).toEqual([CONTINUATION_WINDOW_MS])
    expect(h.submits).toEqual([])
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
    // The turn the resume prompt starts is past the window, and nothing is owed any more.
    turnStart()
    await closeWindow()
    await turnEnd()
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a main-thread turn started within the window, whatever started it, cancels the resume prompt', async () => {
    const { h, c, turnEnd, turnStart, closeWindow, transcript, debugLog } = harness()
    c.lever(undefined, true)
    await turnEnd()
    turnStart()
    await closeWindow()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([])
    expect(debugLog()).toContain('ws compaction: a turn started within the resume window; the resume prompt is not submitted')
    // Settled: that turn's end compacts nothing and owes nothing.
    await turnEnd()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([])
  })

  test('a subagent turn started within the window does not cancel the resume prompt', async () => {
    const { h, c, turnEnd, turnStart, closeWindow } = harness()
    c.lever(undefined, true)
    await turnEnd()
    turnStart('sub-1')
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a main-thread turn started before the compaction settled does not cancel the window that follows', async () => {
    const { h, c, runScheduled, turnStart, closeWindow } = harness()
    turnStart()
    c.lever(undefined, true)
    c.turnComplete(undefined, 'answer')
    // Between the turn end and the compact step: the step has not settled yet.
    turnStart()
    await runScheduled()
    await closeWindow()
    expect(h.compacts).toBe(1)
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a veto with the flag holds its resume prompt under the same guard', async () => {
    const a = harness()
    a.c.lever(undefined, true)
    a.h.compactAnswers.push({ skip: 'blocked by policy' })
    await a.turnEnd()
    expect(a.h.delayed.map((t) => t.ms)).toEqual([CONTINUATION_WINDOW_MS])
    await a.closeWindow()
    expect(a.h.submits).toEqual([vetoedResumeText('ws', 'blocked by policy')])

    const b = harness()
    b.c.lever(undefined, true)
    b.h.compactAnswers.push({ skip: 'blocked by policy' })
    await b.turnEnd()
    b.turnStart()
    await b.closeWindow()
    expect(b.h.submits).toEqual([])
  })

  test('the resume prompt owed after another compaction landed first is submitted at the turn end, unguarded', async () => {
    const { h, c, turnEnd, turnStart, closeWindow } = harness()
    c.lever(undefined, true)
    c.compacted('auto', undefined, { messages: [] })
    // A turn start before that turn end does not cancel it: no window is open.
    turnStart()
    await turnEnd()
    expect(h.submits).toEqual([RESUME_TEXT])
    expect(h.delayed).toEqual([])
    turnStart()
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('the window holds threshold messages until a started turn cancels it', async () => {
    const { h, c, measure, turnEnd, turnStart } = harness()
    await measure(50_000)
    c.lever(undefined, true)
    await turnEnd()
    await measure(104_000)
    expect(h.submits).toEqual([])
    turnStart()
    await measure(105_000)
    expect(h.submits).toEqual([advisoryText('ws', T, 105_000)])
  })

  test('a second lever compaction within an open window leaves one resume prompt', async () => {
    const { h, c, turnEnd, closeWindow } = harness()
    c.lever(undefined, true)
    await turnEnd()
    c.lever(undefined, true)
    await turnEnd()
    expect(h.compacts).toBe(2)
    expect(h.delayed.length).toBe(2)
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
  })

  test('a session end inside the window records the held resume prompt in the transcript and submits nothing', async () => {
    const { h, c, turnEnd, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    await turnEnd()
    c.end('clear')
    expect(transcript()).toEqual([`ws compaction: a prompt was not submitted (session.end clear):\n${RESUME_TEXT}`])
    await closeWindow()
    expect(h.submits).toEqual([])
  })

  test('a session end after a started turn cancelled the window records nothing', async () => {
    const { h, c, turnEnd, turnStart, closeWindow, transcript } = harness()
    c.lever(undefined, true)
    await turnEnd()
    turnStart()
    c.end('clear')
    await closeWindow()
    expect(transcript()).toEqual([])
    expect(h.submits).toEqual([])
  })

  test('a window that cannot start submits the resume prompt at once', async () => {
    const { h, c, turnEnd, turnStart, closeWindow } = harness()
    c.lever(undefined, true)
    h.delayThrows = 1
    await turnEnd()
    expect(h.submits).toEqual([RESUME_TEXT])
    turnStart()
    await closeWindow()
    expect(h.submits).toEqual([RESUME_TEXT])
  })
})

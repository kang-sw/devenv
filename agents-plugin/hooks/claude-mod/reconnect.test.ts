import { describe, expect, test } from 'claude-code/testing'

import {
  checkReconnectCall,
  commandRefusal,
  createReconnect,
  failureText,
  reasonCommandAnswered,
  reasonCommandFailed,
  REASON_NO_SERVER,
  REASON_TOOLS_ABSENT,
  RECONNECT_RESULT,
  RECONNECT_RESULT_NO_RESUME,
  reconnectDescription,
  RESUME_SENTENCE,
  successText,
  TOOLS_POLL_MS,
  TOOLS_WAIT_MS,
  WATCHDOG_MAX_FAILURES,
  watchdogFailureText,
  watchdogSuccessText,
  type ReconnectEdges,
} from './reconnect'

const tick = async (n = 30) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

const SERVER = 'plugin:ws:ws'

/** Fake edges: reconnects, tool-list reads, and submits are recorded; scheduled steps run when the test says. */
function harness(opts: { plugin?: string; server?: string | null } = {}) {
  const h = {
    server: opts.server === null ? undefined : (opts.server ?? SERVER),
    reconnects: [] as string[],
    /** The next reconnect answers in order; an Error rejects. Absent: `{}`, as an interactive reconnect answers. */
    reconnectAnswers: [] as ({ text?: string } | Error)[],
    /** Pending reconnects the test settles itself, when set. */
    holdReconnect: false,
    held: [] as ((v: { text?: string }) => void)[],
    /** The next tool-list reads in order; absent: listed. */
    listed: [] as (boolean | Error)[],
    listReads: 0,
    slept: 0,
    resolves: 0,
    submits: [] as string[],
    dropSubmits: 0,
    /** The next submits that reject. */
    rejectSubmits: 0,
    /** Run at each sleep of the tool-list wait, while the step is pending. */
    onSleep: undefined as (() => void) | undefined,
    scheduled: [] as (() => void)[],
    afterThrows: 0,
    logs: [] as { text: string; to: string }[],
    /** Whether a main-thread turn runs, as register.ts reports it. */
    turnRunning: false,
    /** Armed watchdog timers: the period, the tick, and whether it was stopped. */
    timers: [] as { ms: number; fn: () => void; stopped: boolean }[],
  }
  const r = createReconnect()
  const edges: ReconnectEdges = {
    plugin: opts.plugin ?? 'ws',
    server: () => h.server,
    reconnect: async (server) => {
      h.reconnects.push(server)
      if (h.holdReconnect) return new Promise((resolve) => h.held.push(resolve))
      const answer = h.reconnectAnswers.shift() ?? {}
      if (answer instanceof Error) throw answer
      return answer
    },
    toolsListed: async () => {
      h.listReads += 1
      const answer = h.listed.shift() ?? true
      if (answer instanceof Error) throw answer
      return answer
    },
    resolve: async () => void (h.resolves += 1),
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
    after: (fn) => {
      if (h.afterThrows > 0) {
        h.afterThrows -= 1
        throw new Error('clock refused')
      }
      h.scheduled.push(fn)
    },
    sleep: async (ms) => {
      h.slept += ms
      h.onSleep?.()
    },
    log: (text, to) => void h.logs.push({ text, to }),
    turnRunning: () => h.turnRunning,
    every: (ms, fn) => {
      const timer = { ms, fn, stopped: false }
      h.timers.push(timer)
      return () => void (timer.stopped = true)
    },
  }
  r.start(edges)
  /** Run what turnComplete scheduled, as the engine's clock would once the hook returned. */
  const runScheduled = async () => {
    for (const fn of h.scheduled.splice(0)) fn()
    await tick()
  }
  const turnEnd = async (reason = 'answer', agentId?: string) => {
    r.turnComplete(agentId, reason)
    await runScheduled()
  }
  /** A turn.start, whatever started it: the main thread's when agentId is undefined. */
  const turnStart = (agentId?: string) => r.turnStarted(agentId)
  const transcript = () => h.logs.filter((l) => l.to === 'transcript').map((l) => l.text)
  const debugLog = () => h.logs.filter((l) => l.to === 'debug').map((l) => l.text)
  /** One period of the live watchdog timer, as the engine's clock would fire it. */
  const fire = async () => {
    const live = h.timers.filter((t) => !t.stopped)
    expect(live.length).toBe(1)
    live[0]!.fn()
    await tick(1000)
  }
  return { h, r, edges, turnEnd, turnStart, runScheduled, transcript, debugLog, fire }
}

const SUBAGENT_DENY = 'request_mcp_reconnect serves the main session only; called from subagent sub-1.'

/** Make every tool-list read of the step answer not listed, so the wait runs out. */
const neverBack = (h: ReturnType<typeof harness>['h']) => h.listed.push(...Array.from({ length: 100 }, () => false))

describe('pure core', () => {
  test('only a literal true sets the flag; any other value reads as unset, never as an error; a subagent is refused', () => {
    expect(checkReconnectCall(undefined, true)).toEqual({ resume: true })
    // A non-boolean, and a leftover continuation string (one line or several), read as unset.
    for (const value of [undefined, false, 'true', 1, 'go on', 'one\ntwo', 'one\rtwo', '', null]) {
      expect(checkReconnectCall(undefined, value)).toEqual({ resume: false })
    }
    expect(checkReconnectCall('sub-1', true)).toEqual({ deny: SUBAGENT_DENY })
    expect(checkReconnectCall('sub-1', undefined)).toEqual({ deny: SUBAGENT_DENY })
  })

  test('a reconnect answer is a refusal only when it has words that do not say it reconnected', () => {
    expect(commandRefusal(undefined)).toBeUndefined()
    expect(commandRefusal({})).toBeUndefined()
    expect(commandRefusal({ text: '  ' })).toBeUndefined()
    expect(commandRefusal({ text: 'Successfully reconnected to plugin:ws:ws' })).toBeUndefined()
    expect(commandRefusal({ text: "Reconnect, enable, and disable aren't available in this session." })).toBe(
      "Reconnect, enable, and disable aren't available in this session.",
    )
    expect(commandRefusal({ text: 'Failed to reconnect to plugin:ws:ws' })).toBe('Failed to reconnect to plugin:ws:ws')
  })
})

describe('pinned prose', () => {
  test('tool description and result', () => {
    expect(reconnectDescription('ws')).toBe(
      "Call this first when the ws MCP server's tools have left your tool list or a call reports it not connected. It reconnects the ws MCP server when the current turn ends. Set `continue` to true only when work must resume by itself after the reconnect attempt: a fixed resume prompt then follows a successful reconnect, unless another prompt has already started the next turn. Leave it unset when the next turn will come from the human or from a background agent's notification; a failed reconnect is always reported to you. Main session only.",
    )
    expect(RECONNECT_RESULT).toBe(
      'Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends, and a resume prompt follows a success unless another prompt has already started the next turn. A failed reconnect is reported to you in a prompt.',
    )
    expect(RECONNECT_RESULT_NO_RESUME).toBe(
      "Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends. A successful reconnect submits no prompt: the next turn comes from the human or from a background agent's notification. A failed reconnect is reported to you in a prompt.",
    )
  })

  test('success and failure reports', () => {
    expect(RESUME_SENTENCE).toBe('Resume the interrupted work.')
    expect(successText('wsflow')).toBe(
      '[wsflow mcp] The wsflow MCP server reconnected; its tools are available again. Resume the interrupted work.',
    )
    expect(failureText('wsflow', 'its tools were not back within 10 s', false)).toBe(
      '[wsflow mcp] The wsflow MCP server did not come back (its tools were not back within 10 s). Do not call request_mcp_reconnect again for this drop: a reconnect that just failed fails the same way. Ask the user to re-enable the server from /mcp, run /reload-plugins, or restart the session.',
    )
    expect(failureText('wsflow', 'its tools were not back within 10 s', true)).toBe(
      '[wsflow mcp] The wsflow MCP server did not come back (its tools were not back within 10 s). Do not call request_mcp_reconnect again for this drop: a reconnect that just failed fails the same way. Ask the user to re-enable the server from /mcp, run /reload-plugins, or restart the session. Resume the interrupted work.',
    )
  })

  test('failure reasons', () => {
    expect(REASON_NO_SERVER).toBe('its server name was never resolved in this session')
    expect(REASON_TOOLS_ABSENT).toBe('its tools were not back within 10 s')
    expect(reasonCommandFailed(new Error('boom'))).toBe('the reconnect command failed: Error: boom')
    expect(reasonCommandAnswered('nope')).toBe('the reconnect command answered: nope')
  })
})

describe('lever', () => {
  test('a subagent call is refused and requests nothing', async () => {
    const { h, r, turnEnd } = harness()
    expect(r.lever('sub-1', true)).toEqual({ deny: SUBAGENT_DENY })
    expect(r.lever('sub-1', undefined)).toEqual({ deny: SUBAGENT_DENY })
    await turnEnd()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([])
  })

  test('an absent, false, or non-boolean flag is accepted with the no-resume result', () => {
    for (const value of [undefined, false, 'true', 1, 'go on', 'one\ntwo']) {
      const { r } = harness()
      expect(r.lever(undefined, value)).toEqual({ result: RECONNECT_RESULT_NO_RESUME })
    }
    expect(harness().r.lever(undefined, true)).toEqual({ result: RECONNECT_RESULT })
  })

  test('without the flag a successful reconnect still runs and submits nothing', async () => {
    for (const value of [undefined, false, 'go on']) {
      const { h, r, turnEnd, transcript } = harness()
      r.lever(undefined, value)
      await turnEnd()
      expect(h.reconnects).toEqual([SERVER])
      expect(h.resolves).toBe(1)
      expect(h.submits).toEqual([])
      expect(transcript()).toEqual([])
      await turnEnd()
      expect(h.reconnects.length).toBe(1)
    }
  })

  test('without the flag a failed reconnect submits its failure report, with no resume sentence', async () => {
    const a = harness()
    neverBack(a.h)
    a.r.lever(undefined, 'go on')
    await a.turnEnd()
    await tick(1000)
    expect(a.h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, false)])
    expect(a.h.submits[0]).not.toContain(RESUME_SENTENCE)

    const b = harness({ server: null })
    b.r.lever(undefined, undefined)
    await b.turnEnd()
    expect(b.h.submits).toEqual([failureText('ws', REASON_NO_SERVER, false)])
  })

  test('a repeat call in both orders: the latest flag wins', async () => {
    const a = harness()
    a.r.lever(undefined, true)
    a.r.lever(undefined, undefined)
    await a.turnEnd()
    expect(a.h.reconnects.length).toBe(1)
    expect(a.h.submits).toEqual([])

    const b = harness()
    b.r.lever(undefined, undefined)
    b.r.lever(undefined, true)
    await b.turnEnd()
    expect(b.h.reconnects.length).toBe(1)
    expect(b.h.submits).toEqual([successText('ws')])
  })

  test('the reconnect waits for a non-aborted main-thread turn end, then runs after the hook returned', async () => {
    const { h, r, turnEnd } = harness()
    expect(r.lever(undefined, true)).toEqual({ result: RECONNECT_RESULT })
    await turnEnd('answer', 'sub-1')
    await turnEnd('aborted')
    expect(h.reconnects).toEqual([])
    r.turnComplete(undefined, 'answer')
    // Scheduled, not run: nothing happens inside turnComplete itself.
    expect(h.scheduled.length).toBe(1)
    expect(h.reconnects).toEqual([])
  })

  test('success with the flag: the tools are listed after the command, the name is resolved again, and one resume prompt follows', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER])
    expect(h.resolves).toBe(1)
    expect(h.submits).toEqual([successText('ws')])
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('the reconnect runs even when the tools are listed before it', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER])
  })

  test('a repeat call with the flag while pending reconnects once', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, true)
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
    expect(h.submits).toEqual([successText('ws')])
  })

  test('the tool list is polled for a bounded time before success', async () => {
    const { h, r, turnEnd } = harness()
    h.listed.push(false, false, false)
    r.lever(undefined, true)
    await turnEnd()
    expect(h.listReads).toBe(4)
    expect(h.slept).toBe(3 * TOOLS_POLL_MS)
    expect(h.submits).toEqual([successText('ws')])
  })

  test('failure: the tools not back within the wait', async () => {
    const { h, r, turnEnd } = harness()
    neverBack(h)
    r.lever(undefined, true)
    await turnEnd()
    await tick(1000)
    expect(h.slept).toBe(TOOLS_WAIT_MS)
    expect(h.resolves).toBe(0)
    expect(h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, true)])
  })

  test('a failing tool-list read counts as not listed', async () => {
    const { h, r, turnEnd } = harness()
    h.listed.push(new Error('list failed'))
    r.lever(undefined, true)
    await turnEnd()
    expect(h.listReads).toBe(2)
    expect(h.submits).toEqual([successText('ws')])
  })

  test('failure: the command rejects', async () => {
    const { h, r, turnEnd } = harness()
    h.reconnectAnswers.push(new Error('past a Stop'))
    r.lever(undefined, true)
    await turnEnd()
    expect(h.listReads).toBe(0)
    expect(h.submits).toEqual([failureText('ws', reasonCommandFailed(new Error('past a Stop')), true)])
  })

  test('failure: the command answers that it cannot reconnect, though the tools stay listed (headless)', async () => {
    const { h, r, turnEnd } = harness()
    const headless = "Reconnect, enable, and disable aren't available in this session."
    h.reconnectAnswers.push({ text: headless })
    r.lever(undefined, true)
    await turnEnd()
    expect(h.submits).toEqual([failureText('ws', reasonCommandAnswered(headless), true)])
  })

  test('failure without reconnecting: no server name was resolved', async () => {
    const { h, r, turnEnd } = harness({ server: null })
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([failureText('ws', REASON_NO_SERVER, true)])
  })

  test('a call made while the step runs is a new request for a later turn end', async () => {
    const { h, r, turnEnd } = harness()
    h.holdReconnect = true
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
    r.lever(undefined, true)
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([successText('ws')])
    h.holdReconnect = false
    await turnEnd()
    expect(h.reconnects.length).toBe(2)
    expect(h.submits).toEqual([successText('ws'), successText('ws')])
  })

  test('a call without the flag made while the step runs does not change that step', async () => {
    const { h, r, turnEnd } = harness()
    h.holdReconnect = true
    r.lever(undefined, true)
    await turnEnd()
    r.lever(undefined, undefined)
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([successText('ws')])
    h.holdReconnect = false
    await turnEnd()
    expect(h.reconnects.length).toBe(2)
    expect(h.submits).toEqual([successText('ws')])
  })

  test('a step still queued at a later turn end is queued again; the stale one stays inert', async () => {
    const { h, r, runScheduled } = harness()
    r.lever(undefined, true)
    r.turnComplete(undefined, 'answer')
    const stale = h.scheduled.splice(0)
    r.turnComplete(undefined, 'answer')
    await runScheduled()
    for (const fn of stale) fn()
    await tick()
    expect(h.reconnects.length).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('a step the clock refused is retried at the next turn end', async () => {
    const { h, r, turnEnd } = harness()
    h.afterThrows = 1
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects).toEqual([])
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
  })

  test('a dropped resume prompt is written to the transcript with its fixed text', async () => {
    const { h, r, turnEnd, transcript } = harness()
    h.dropSubmits = 1
    r.lever(undefined, true)
    await turnEnd()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws mcp reconnect: a prompt was not submitted (dropped: a Stop stands):\n${successText('ws')}`])
  })

  test('a resume prompt whose submit rejects is written to the transcript with its fixed text', async () => {
    const { h, r, turnEnd, transcript } = harness()
    h.rejectSubmits = 1
    r.lever(undefined, true)
    await turnEnd()
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([`ws mcp reconnect: a prompt was not submitted (Error: submit failed):\n${successText('ws')}`])
  })

  test('a lost failure report is written to the transcript', async () => {
    const { h, r, turnEnd, transcript } = harness({ server: null })
    h.dropSubmits = 1
    r.lever(undefined, undefined)
    await turnEnd()
    expect(transcript()).toEqual([
      `ws mcp reconnect: a prompt was not submitted (dropped: a Stop stands):\n${failureText('ws', REASON_NO_SERVER, false)}`,
    ])
  })
})

describe('turn-start guard', () => {
  test('a main-thread turn started while the reconnect command is pending: a success submits no resume prompt', async () => {
    const { h, r, turnEnd, turnStart, debugLog } = harness()
    h.holdReconnect = true
    r.lever(undefined, true)
    await turnEnd()
    turnStart()
    h.held.shift()!({})
    await tick()
    expect(h.resolves).toBe(1)
    expect(h.submits).toEqual([])
    expect(debugLog()).toContain('ws mcp reconnect: a turn started during the reconnect step; the resume prompt is not submitted')
  })

  test('a main-thread turn started during the tool-list wait: a success submits no resume prompt', async () => {
    const { h, r, turnEnd, turnStart } = harness()
    h.listed.push(false, false)
    h.onSleep = () => {
      h.onSleep = undefined
      turnStart()
    }
    r.lever(undefined, true)
    await turnEnd()
    expect(h.listReads).toBe(3)
    expect(h.submits).toEqual([])
  })

  test('a subagent turn started during the step does not suppress the resume prompt', async () => {
    const { h, r, turnEnd, turnStart } = harness()
    h.holdReconnect = true
    r.lever(undefined, true)
    await turnEnd()
    turnStart('sub-1')
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([successText('ws')])
  })

  test('a main-thread turn started before the step starts does not suppress its resume prompt', async () => {
    const { h, r, runScheduled, turnStart } = harness()
    turnStart()
    r.lever(undefined, true)
    r.turnComplete(undefined, 'answer')
    turnStart()
    await runScheduled()
    expect(h.submits).toEqual([successText('ws')])
  })

  test('a failure is reported whatever turn started during the step; with the flag it ends with the resume sentence', async () => {
    const a = harness()
    a.h.holdReconnect = true
    a.r.lever(undefined, true)
    await a.turnEnd()
    a.turnStart()
    a.h.held.shift()!({ text: 'Failed to reconnect' })
    await tick()
    expect(a.h.submits).toEqual([failureText('ws', reasonCommandAnswered('Failed to reconnect'), true)])
    expect(a.h.submits[0]!.endsWith(` ${RESUME_SENTENCE}`)).toBe(true)

    const b = harness()
    b.h.holdReconnect = true
    b.r.lever(undefined, undefined)
    await b.turnEnd()
    b.turnStart()
    b.h.held.shift()!({ text: 'Failed to reconnect' })
    await tick()
    expect(b.h.submits).toEqual([failureText('ws', reasonCommandAnswered('Failed to reconnect'), false)])
    expect(b.h.submits[0]).not.toContain(RESUME_SENTENCE)
  })

  test('a failure with no intervening turn is reported the same way', async () => {
    const a = harness()
    neverBack(a.h)
    a.r.lever(undefined, true)
    await a.turnEnd()
    await tick(1000)
    expect(a.h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, true)])

    const b = harness()
    neverBack(b.h)
    b.r.lever(undefined, false)
    await b.turnEnd()
    await tick(1000)
    expect(b.h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, false)])
  })

  test('a turn the step suppressed does not carry over: the next request with the flag resumes', async () => {
    const { h, r, turnEnd, turnStart } = harness()
    h.holdReconnect = true
    r.lever(undefined, true)
    await turnEnd()
    turnStart()
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([])
    h.holdReconnect = false
    r.lever(undefined, true)
    await turnEnd()
    expect(h.submits).toEqual([successText('ws')])
  })
})

describe('session end', () => {
  test('drops a pending request', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, true)
    r.end('clear')
    await turnEnd()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([])
  })

  test('drops the report of a step that was running', async () => {
    const { h, r, turnEnd } = harness()
    h.holdReconnect = true
    r.lever(undefined, true)
    await turnEnd()
    r.end('resume')
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([])
  })

  test('a scheduled step that had not started does nothing after the end', async () => {
    const { h, r, runScheduled } = harness()
    r.lever(undefined, true)
    r.turnComplete(undefined, 'answer')
    r.end('clear')
    await runScheduled()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([])
  })
})

describe('watchdog', () => {
  const MS = 10 * 60_000
  /** A watchdog armed over a server whose tools are gone until the reconnect brings them back. */
  const dropped = () => {
    const w = harness()
    w.r.watch(MS)
    // One read finds the tools gone; the attempt's first poll finds them back.
    w.h.listed.push(false)
    return w
  }

  test('watch arms one timer with the interval; a second watch replaces it; a session start stops it', () => {
    const { h, r, edges } = harness()
    expect(h.timers).toEqual([])
    r.watch(MS)
    expect(h.timers.map((t) => [t.ms, t.stopped])).toEqual([[MS, false]])
    r.watch(3 * 60_000)
    expect(h.timers.map((t) => [t.ms, t.stopped])).toEqual([
      [MS, true],
      [3 * 60_000, false],
    ])
    r.end('clear')
    expect(h.timers[1]!.stopped).toBe(false)
    r.start(edges)
    expect(h.timers[1]!.stopped).toBe(true)
  })

  test('a tick with the tools listed does nothing', async () => {
    const { h, r, fire, transcript } = harness()
    r.watch(MS)
    await fire()
    expect(h.listReads).toBe(1)
    expect(h.reconnects).toEqual([])
    expect(transcript()).toEqual([])
  })

  test('a tick with the tools gone runs one reconnect, writes one transcript line, and submits no prompt', async () => {
    const { h, fire, transcript } = dropped()
    await fire()
    expect(h.reconnects).toEqual([SERVER])
    expect(h.resolves).toBe(1)
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([watchdogSuccessText('ws')])
  })

  test('a tick while a main-thread turn runs does nothing', async () => {
    const { h, fire } = dropped()
    h.turnRunning = true
    await fire()
    expect(h.listReads).toBe(0)
    expect(h.reconnects).toEqual([])
    h.turnRunning = false
    await fire()
    expect(h.reconnects).toEqual([SERVER])
  })

  test('a tick while a lever step is scheduled or running does nothing', async () => {
    const { h, r, fire, runScheduled } = dropped()
    r.lever(undefined, undefined)
    r.turnComplete(undefined, 'answer')
    await fire()
    expect(h.listReads).toBe(0)
    h.holdReconnect = true
    await runScheduled()
    expect(h.reconnects).toEqual([SERVER])
    await fire()
    expect(h.reconnects).toEqual([SERVER])
    h.held.shift()!({})
    await tick(1000)
  })

  test('a lever request left pending by an aborted turn does not hold a tick off, and still runs at its next turn end', async () => {
    const { h, r, fire, turnEnd } = dropped()
    r.lever(undefined, undefined)
    await turnEnd('aborted')
    await fire()
    expect(h.reconnects).toEqual([SERVER])
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER, SERVER])
  })

  test('a turn starting during the tick tool-list read holds the attempt off', async () => {
    const { h, r, fire } = harness()
    r.watch(MS)
    // The read flips the turn on, as a turn starting while it is pending would.
    const answers: (boolean | Error)[] = [false]
    h.listed = new Proxy(answers, {
      get: (target, prop, receiver) =>
        prop === 'shift'
          ? () => {
              h.turnRunning = true
              return target.shift()
            }
          : Reflect.get(target, prop, receiver),
    })
    await fire()
    expect(h.listReads).toBe(1)
    expect(h.reconnects).toEqual([])
  })

  test('a lever step due while a watchdog attempt is in flight reports that attempt instead of reconnecting again', async () => {
    const { h, r, fire, turnEnd, transcript } = dropped()
    h.holdReconnect = true
    await fire()
    expect(h.reconnects).toEqual([SERVER])
    r.lever(undefined, true)
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER])
    h.held.shift()!({})
    await tick(1000)
    expect(h.reconnects).toEqual([SERVER])
    expect(h.submits).toEqual([successText('ws')])
    expect(transcript()).toEqual([watchdogSuccessText('ws')])
  })

  test('a lever step reports a failed watchdog attempt it waited on', async () => {
    const { h, r, fire, turnEnd } = harness()
    r.watch(MS)
    neverBack(h)
    h.holdReconnect = true
    await fire()
    r.lever(undefined, undefined)
    await turnEnd()
    h.held.shift()!({})
    await tick(1000)
    expect(h.reconnects).toEqual([SERVER])
    expect(h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, false)])
  })

  test(`${WATCHDOG_MAX_FAILURES} failed attempts in a row stop it, with one notice per streak; a later resolution restarts it`, async () => {
    const { h, r, fire, transcript } = harness()
    r.watch(MS)
    neverBack(h)
    for (let i = 0; i < WATCHDOG_MAX_FAILURES; i++) await fire()
    expect(h.reconnects.length).toBe(WATCHDOG_MAX_FAILURES)
    expect(transcript()).toEqual([watchdogFailureText('ws', REASON_TOOLS_ABSENT, MS)])
    expect(h.submits).toEqual([])
    await fire()
    await fire()
    expect(h.reconnects.length).toBe(WATCHDOG_MAX_FAILURES)
    // A lever reconnect or a session start resolves the server again.
    r.resolved()
    await fire()
    expect(h.reconnects.length).toBe(WATCHDOG_MAX_FAILURES + 1)
    expect(transcript()).toEqual([watchdogFailureText('ws', REASON_TOOLS_ABSENT, MS), watchdogFailureText('ws', REASON_TOOLS_ABSENT, MS)])
  })

  test('a session start re-arms a stopped watchdog with a fresh failure streak', async () => {
    const { h, r, edges, fire } = harness()
    r.watch(MS)
    neverBack(h)
    for (let i = 0; i < WATCHDOG_MAX_FAILURES; i++) await fire()
    await fire()
    expect(h.reconnects.length).toBe(WATCHDOG_MAX_FAILURES)
    r.start(edges)
    r.watch(MS)
    await fire()
    expect(h.reconnects.length).toBe(WATCHDOG_MAX_FAILURES + 1)
  })

  test('a stopped watchdog that finds the tools listed again restarts', async () => {
    const { h, r, fire } = harness()
    r.watch(MS)
    neverBack(h)
    for (let i = 0; i < WATCHDOG_MAX_FAILURES; i++) await fire()
    h.listed.length = 0
    await fire()
    h.listed.push(false)
    await fire()
    expect(h.reconnects.length).toBe(WATCHDOG_MAX_FAILURES + 1)
  })

  test('a success ends a failure streak: the next failure notices again', async () => {
    const { h, r, fire, transcript } = harness()
    r.watch(MS)
    h.listed.push(false, ...Array.from({ length: 30 }, () => false))
    await fire()
    expect(transcript()).toEqual([watchdogFailureText('ws', REASON_TOOLS_ABSENT, MS)])
    h.listed.length = 0
    h.listed.push(false)
    await fire()
    h.listed.push(false, ...Array.from({ length: 30 }, () => false))
    await fire()
    expect(transcript()).toEqual([
      watchdogFailureText('ws', REASON_TOOLS_ABSENT, MS),
      watchdogSuccessText('ws'),
      watchdogFailureText('ws', REASON_TOOLS_ABSENT, MS),
    ])
  })

  test('a tick with no server name does nothing', async () => {
    const { h, r, fire, transcript } = harness({ server: null })
    r.watch(MS)
    await fire()
    expect(h.listReads).toBe(0)
    expect(h.reconnects).toEqual([])
    expect(transcript()).toEqual([])
  })

  test('a failing tool-list read skips the tick without attempting', async () => {
    const { h, r, fire } = harness()
    r.watch(MS)
    h.listed.push(new Error('list failed'))
    await fire()
    expect(h.reconnects).toEqual([])
  })
})

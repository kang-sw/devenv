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
  RECONNECT_RESULT_NO_CONTINUATION,
  reconnectDescription,
  successText,
  TOOLS_POLL_MS,
  TOOLS_WAIT_MS,
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
    scheduled: [] as (() => void)[],
    afterThrows: 0,
    logs: [] as { text: string; to: string }[],
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
    sleep: async (ms) => void (h.slept += ms),
    log: (text, to) => void h.logs.push({ text, to }),
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
  const transcript = () => h.logs.filter((l) => l.to === 'transcript').map((l) => l.text)
  return { h, r, edges, turnEnd, runScheduled, transcript }
}

describe('pure core', () => {
  test('the tool accepts one trimmed line from the main session only', () => {
    expect(checkReconnectCall(undefined, '  resume the review  ')).toEqual({ continuation: 'resume the review' })
    expect(checkReconnectCall('sub-1', 'x')).toEqual({ deny: 'request_mcp_reconnect serves the main session only; called from subagent sub-1.' })
    const omitted = { continuation: undefined }
    expect(checkReconnectCall(undefined, '')).toEqual(omitted)
    expect(checkReconnectCall(undefined, '   ')).toEqual(omitted)
    expect(checkReconnectCall(undefined, undefined)).toEqual(omitted)
    expect(checkReconnectCall(undefined, 42)).toEqual(omitted)
    expect(checkReconnectCall(undefined, 'one\ntwo')).toEqual({ error: 'request_mcp_reconnect: continuation must be one line.' })
    expect(checkReconnectCall(undefined, 'one\rtwo')).toEqual({ error: 'request_mcp_reconnect: continuation must be one line.' })
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
      "Reconnect the ws MCP server when the current turn ends. Use it when that server's tools have left your tool list or a call reports the server not connected. `continuation` is optional: pass it only when work must resume by itself after the reconnect attempt, as one line naming what to do next; it is sent as your next prompt with the outcome. Omit it when the next turn will come from the human or from a background agent's notification; a failed reconnect is still reported to you. Main session only.",
    )
    expect(RECONNECT_RESULT).toBe(
      'Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends, and your continuation resumes the work.',
    )
    expect(RECONNECT_RESULT_NO_CONTINUATION).toBe(
      "Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends. A successful reconnect submits no prompt: the next turn comes from the human or from a background agent's notification. A failed reconnect is reported to you in a prompt.",
    )
  })

  test('continuations', () => {
    expect(successText('wsflow', 'resume the review')).toBe(
      '[wsflow mcp] The wsflow MCP server reconnected; its tools are available again.\nContinue: resume the review',
    )
    expect(failureText('wsflow', 'its tools were not back within 10 s', undefined)).toBe(
      '[wsflow mcp] The wsflow MCP server did not come back (its tools were not back within 10 s). Follow the wsflow:mcp-server-repair skill: keep working through its CLI fallback and relay its reconnect steps to the user.',
    )
    expect(failureText('wsflow', 'its tools were not back within 10 s', 'resume the review')).toBe(
      '[wsflow mcp] The wsflow MCP server did not come back (its tools were not back within 10 s). Follow the wsflow:mcp-server-repair skill: keep working through its CLI fallback and relay its reconnect steps to the user.\nContinue: resume the review',
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
  test('a subagent call is refused and a multi-line continuation is a tool error; neither requests', async () => {
    const { h, r, turnEnd } = harness()
    expect(r.lever('sub-1', 'go on')).toEqual({ deny: 'request_mcp_reconnect serves the main session only; called from subagent sub-1.' })
    expect(r.lever(undefined, 'one\ntwo')).toEqual({
      isError: true,
      result: 'request_mcp_reconnect: continuation must be one line.',
      text: 'request_mcp_reconnect: continuation must be one line.',
    })
    await turnEnd()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([])
  })

  test('an omitted, whitespace-only, or non-string continuation is accepted with the no-continuation result', () => {
    for (const value of [undefined, '', '  \t ', 42]) {
      const { r } = harness()
      expect(r.lever(undefined, value)).toEqual({ result: RECONNECT_RESULT_NO_CONTINUATION })
    }
    expect(harness().r.lever('sub-1', undefined)).toEqual({ deny: 'request_mcp_reconnect serves the main session only; called from subagent sub-1.' })
  })

  test('without a continuation a successful reconnect still runs and submits nothing', async () => {
    const { h, r, turnEnd, transcript } = harness()
    r.lever(undefined, undefined)
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER])
    expect(h.resolves).toBe(1)
    expect(h.submits).toEqual([])
    expect(transcript()).toEqual([])
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
  })

  test('without a continuation a failed reconnect submits its failure notice, with no Continue line', async () => {
    const a = harness()
    a.h.listed.push(...Array.from({ length: 100 }, () => false))
    a.r.lever(undefined, '  ')
    await a.turnEnd()
    await tick(1000)
    expect(a.h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, undefined)])
    expect(a.h.submits[0]).not.toContain('Continue:')

    const b = harness({ server: null })
    b.r.lever(undefined, undefined)
    await b.turnEnd()
    expect(b.h.submits).toEqual([failureText('ws', REASON_NO_SERVER, undefined)])
  })

  test('a repeat call in both orders: an omitted one clears an earlier continuation, a supplied one sets it', async () => {
    const a = harness()
    a.r.lever(undefined, 'first')
    a.r.lever(undefined, undefined)
    await a.turnEnd()
    expect(a.h.reconnects.length).toBe(1)
    expect(a.h.submits).toEqual([])

    const b = harness()
    b.r.lever(undefined, undefined)
    b.r.lever(undefined, 'second')
    await b.turnEnd()
    expect(b.h.reconnects.length).toBe(1)
    expect(b.h.submits).toEqual([successText('ws', 'second')])
  })

  test('the reconnect waits for a non-aborted main-thread turn end, then runs after the hook returned', async () => {
    const { h, r, turnEnd } = harness()
    expect(r.lever(undefined, 'go on')).toEqual({ result: RECONNECT_RESULT })
    await turnEnd('answer', 'sub-1')
    await turnEnd('aborted')
    expect(h.reconnects).toEqual([])
    r.turnComplete(undefined, 'answer')
    // Scheduled, not run: nothing happens inside turnComplete itself.
    expect(h.scheduled.length).toBe(1)
    expect(h.reconnects).toEqual([])
  })

  test('success: the tools are listed after the command, the name is resolved again, and one continuation follows', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER])
    expect(h.resolves).toBe(1)
    expect(h.submits).toEqual([successText('ws', 'go on')])
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
    expect(h.submits.length).toBe(1)
  })

  test('the reconnect runs even when the tools are listed before it', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.reconnects).toEqual([SERVER])
  })

  test('a repeat call while pending replaces the continuation; one reconnect', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, 'first')
    r.lever(undefined, 'second')
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
    expect(h.submits).toEqual([successText('ws', 'second')])
  })

  test('the tool list is polled for a bounded time before success', async () => {
    const { h, r, turnEnd } = harness()
    h.listed.push(false, false, false)
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.listReads).toBe(4)
    expect(h.slept).toBe(3 * TOOLS_POLL_MS)
    expect(h.submits).toEqual([successText('ws', 'go on')])
  })

  test('failure: the tools not back within the wait', async () => {
    const { h, r, turnEnd } = harness()
    h.listed.push(...Array.from({ length: 100 }, () => false))
    r.lever(undefined, 'go on')
    await turnEnd()
    await tick(1000)
    expect(h.slept).toBe(TOOLS_WAIT_MS)
    expect(h.resolves).toBe(0)
    expect(h.submits).toEqual([failureText('ws', REASON_TOOLS_ABSENT, 'go on')])
  })

  test('a failing tool-list read counts as not listed', async () => {
    const { h, r, turnEnd } = harness()
    h.listed.push(new Error('list failed'))
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.listReads).toBe(2)
    expect(h.submits).toEqual([successText('ws', 'go on')])
  })

  test('failure: the command rejects', async () => {
    const { h, r, turnEnd } = harness()
    h.reconnectAnswers.push(new Error('past a Stop'))
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.listReads).toBe(0)
    expect(h.submits).toEqual([failureText('ws', reasonCommandFailed(new Error('past a Stop')), 'go on')])
  })

  test('failure: the command answers that it cannot reconnect, though the tools stay listed (headless)', async () => {
    const { h, r, turnEnd } = harness()
    const headless = "Reconnect, enable, and disable aren't available in this session."
    h.reconnectAnswers.push({ text: headless })
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.submits).toEqual([failureText('ws', reasonCommandAnswered(headless), 'go on')])
  })

  test('failure without reconnecting: no server name was resolved', async () => {
    const { h, r, turnEnd } = harness({ server: null })
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([failureText('ws', REASON_NO_SERVER, 'go on')])
  })

  test('a call made while the step runs is a new request for a later turn end', async () => {
    const { h, r, turnEnd } = harness()
    h.holdReconnect = true
    r.lever(undefined, 'first')
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
    r.lever(undefined, 'second')
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([successText('ws', 'first')])
    h.holdReconnect = false
    await turnEnd()
    expect(h.reconnects.length).toBe(2)
    expect(h.submits).toEqual([successText('ws', 'first'), successText('ws', 'second')])
  })

  test('a step still queued at a later turn end is queued again; the stale one stays inert', async () => {
    const { h, r, runScheduled } = harness()
    r.lever(undefined, 'go on')
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
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(h.reconnects).toEqual([])
    await turnEnd()
    expect(h.reconnects.length).toBe(1)
  })

  test('a dropped continuation is written to the transcript', async () => {
    const { h, r, turnEnd, transcript } = harness()
    h.dropSubmits = 1
    r.lever(undefined, 'go on')
    await turnEnd()
    expect(transcript()).toEqual([`ws mcp reconnect: a prompt was not submitted (dropped: a Stop stands):\n${successText('ws', 'go on')}`])
  })
})

describe('session end', () => {
  test('drops a pending request', async () => {
    const { h, r, turnEnd } = harness()
    r.lever(undefined, 'go on')
    r.end('clear')
    await turnEnd()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([])
  })

  test('drops the continuation of a step that was running', async () => {
    const { h, r, turnEnd } = harness()
    h.holdReconnect = true
    r.lever(undefined, 'go on')
    await turnEnd()
    r.end('resume')
    h.held.shift()!({})
    await tick()
    expect(h.submits).toEqual([])
  })

  test('a scheduled step that had not started does nothing after the end', async () => {
    const { h, r, runScheduled } = harness()
    r.lever(undefined, 'go on')
    r.turnComplete(undefined, 'answer')
    r.end('clear')
    await runScheduled()
    expect(h.reconnects).toEqual([])
    expect(h.submits).toEqual([])
  })
})

import { describe, expect, test } from 'claude-code/testing'

import {
  ADAPTER,
  argsPreview,
  BATCH_CALL_CAP,
  BATCH_CAP,
  BATCH_RETAIN_CAP,
  CALL_TIMEOUT_MS,
  callArgs,
  createDashboard,
  FLUSH_INTERVAL_MS,
  labelWhiteboard,
  parseInboxDrain,
  parseInterruptDrain,
  PERMISSION_PROMPT,
  RETRY_MS,
  TEXT_FLUSH_INTERVAL_MS,
  userPrompt,
  whiteboardBody,
  type DashboardEvent,
  type IntakeResult,
} from './dashboard'

const tick = async (n = 30) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

const KEY = 'lead-key-123'
const DELEGATE_KEY = 'delegate-key-456'
const PLUGIN = 'ws'
const OWN = 'mcp__plugin_ws_ws__'

/** An answer: delivered or an error result, a whole result, a transport failure, or none. */
type Answer = boolean | IntakeResult | Error | 'hang'

/** Fake edges: a manual clock with timers, and intake calls answered from a queue (delivered when empty). */
function harness(opts: { key?: string | undefined } = {}) {
  const h = {
    now: 1_000,
    key: 'key' in opts ? opts.key : KEY,
    timers: [] as { at: number; fn: () => void; cancelled: boolean }[],
    calls: [] as { session_key: string; adapter: string; events: DashboardEvent[] }[],
    answers: [] as Answer[],
    /** Whether a delivered answer's text names `status` on its accepts line. */
    accepts: false,
    logs: [] as string[],
    epochs: 0,
  }
  const dash = createDashboard({
    key: () => h.key,
    call: async (args) => {
      h.calls.push(JSON.parse(JSON.stringify(args)))
      const answer = h.answers.shift() ?? true
      if (answer === 'hang') return new Promise<IntakeResult>(() => {})
      if (answer instanceof Error) throw answer
      if (typeof answer !== 'boolean') return answer
      return { ok: answer, text: answer ? `ok: 1 of 1 events applied\n${h.accepts ? 'accepts: status\n' : ''}` : 'unknown session_key' }
    },
    ownTool: (tool) => tool.startsWith(OWN),
    now: () => h.now,
    schedule: (ms, fn) => {
      const timer = { at: h.now + ms, fn, cancelled: false }
      h.timers.push(timer)
      return () => void (timer.cancelled = true)
    },
    epoch: () => `ep${++h.epochs}`,
    log: (text) => h.logs.push(text),
  })
  /** Advances the clock by `ms`, firing due timers in order and letting their work settle. */
  const advance = async (ms = FLUSH_INTERVAL_MS) => {
    const until = h.now + ms
    for (;;) {
      await tick()
      const due = h.timers.filter((t) => !t.cancelled && t.at <= until).sort((a, b) => a.at - b.at)[0]
      if (!due) break
      h.now = Math.max(h.now, due.at)
      due.cancelled = true
      due.fn()
    }
    h.now = until
    await tick()
  }
  /** Every event sent so far, flattened in call order. */
  const sent = () => h.calls.flatMap((c) => c.events)
  /** Runs `fn`, flushes, and returns the events it caused. */
  const after = async (fn: () => void) => {
    const before = sent().length
    fn()
    await advance()
    return sent().slice(before)
  }
  return { h, dash, advance, sent, after }
}

/** A started feed whose opening reset and empty lead have been sent. */
async function started(opts: { key?: string | undefined } = {}) {
  const t = harness(opts)
  t.dash.start()
  await t.advance()
  return t
}

const kinds = (events: DashboardEvent[]) => events.map((e) => e.kind)
const lastLead = (events: DashboardEvent[]) => events.filter((e) => e.kind === 'agent' && e.agentId === undefined).at(-1)
const lastAgent = (events: DashboardEvent[], id: string) => events.filter((e) => e.kind === 'agent' && e.agentId === id).at(-1)
type View = { seq: number; toolNames: string[]; callCount: number; failed: number; endedAt?: number; running?: Record<string, unknown>; generating?: Record<string, unknown> }
const batches = (e: DashboardEvent | undefined) => ((e?.batches as View[] | undefined) ?? [])

describe('pure helpers', () => {
  test('the whiteboard label is its own first line and strips back off', () => {
    const text = labelWhiteboard(PLUGIN, 'answer: yes')
    expect(text).toBe('[user message from the ws whiteboard]\nanswer: yes')
    expect(whiteboardBody(PLUGIN, text)).toBe('answer: yes')
    expect(whiteboardBody(PLUGIN, 'answer: yes')).toBeUndefined()
    expect(whiteboardBody('wsflow', text)).toBeUndefined()
  })

  test('an inbox drain\'s JSON gives each claimed message\'s text in order; any other shape is kept whole, never dropped', () => {
    expect(parseInboxDrain('{"messages":[]}')).toEqual([])
    expect(parseInboxDrain('  ')).toEqual([])
    expect(parseInboxDrain('{"messages":[{"id":"a","text":"pick B","parts":["pick B"],"chips":[]},{"id":"b","text":"(03) ok"}]}')).toEqual(['pick B', '(03) ok'])
    expect(parseInboxDrain('{"messages":[{"id":"a"}]}')).toEqual(['{"id":"a"}'])
    expect(parseInboxDrain('drained 1\nmessage x\nhi')).toEqual(['drained 1\nmessage x\nhi'])
    expect(parseInboxDrain('{"other":1}')).toEqual(['{"other":1}'])
  })

  test('an interrupt drain\'s JSON gives each interrupt entry\'s creation time; a record without one, or any other shape, gives nothing', () => {
    const entry = (sentAt: unknown, interrupt: unknown = true) => ({ id: 'i', sent_at: sentAt, text: '', parts: [], chips: [], interrupt })
    expect(parseInterruptDrain(JSON.stringify({ messages: [entry(1_800_000_000_000), entry(1_800_000_000_500)] }))).toEqual([1_800_000_000_000, 1_800_000_000_500])
    expect(parseInterruptDrain('{"messages":[]}')).toEqual([])
    expect(parseInterruptDrain(JSON.stringify({ messages: [entry('soon'), entry(null), entry(5, false), { id: 'm', text: 'hi' }, entry(7)] }))).toEqual([7])
    expect(parseInterruptDrain('  ')).toEqual([])
    expect(parseInterruptDrain('drained 1')).toEqual([])
    expect(parseInterruptDrain('{"other":1}')).toEqual([])
  })

  test('a typed prompt is a terminal user message; this plugin\'s labelled prompt a whiteboard one; anything else none', () => {
    expect(userPrompt({ text: 'hi', origin: { kind: 'composer' } }, PLUGIN)).toEqual({ text: 'hi', origin: 'terminal' })
    expect(userPrompt({ text: 'hi', origin: { kind: 'sdk' } }, PLUGIN)).toEqual({ text: 'hi', origin: 'terminal' })
    expect(userPrompt({ text: 'hi', origin: { kind: 'unclassified' } }, PLUGIN)).toEqual({ text: 'hi', origin: 'terminal' })
    expect(userPrompt({ text: labelWhiteboard(PLUGIN, 'go'), origin: { kind: 'plugin', name: PLUGIN } }, PLUGIN)).toEqual({ text: 'go', origin: 'whiteboard' })
    // The mailbox push's prompt, another plugin's, a notification, a meta prompt, an empty one.
    expect(userPrompt({ text: 'mail arrived', origin: { kind: 'plugin', name: PLUGIN } }, PLUGIN)).toBeUndefined()
    expect(userPrompt({ text: labelWhiteboard(PLUGIN, 'go'), origin: { kind: 'plugin', name: 'other' } }, PLUGIN)).toBeUndefined()
    expect(userPrompt({ text: 'done', origin: { kind: 'task-notification' } }, PLUGIN)).toBeUndefined()
    expect(userPrompt({ text: 'hi', isMeta: true, origin: { kind: 'composer' } }, PLUGIN)).toBeUndefined()
    expect(userPrompt({ text: '  ', origin: { kind: 'composer' } }, PLUGIN)).toBeUndefined()
  })

  test('a preview flattens and caps the arguments and never carries a session key argument', () => {
    expect(argsPreview({ command: 'ls -la', timeout: 5 })).toBe('command: ls -la, timeout: 5')
    expect(argsPreview({ session_key: KEY, key: 'x' })).toBe('key: x')
    expect(argsPreview({ sessionKey: KEY })).toBe('')
    expect([...argsPreview({ text: 'a'.repeat(500) })].length).toBe(200)
    expect(callArgs({ tool: 'Bash', tool_use_id: 'u1', agentId: 'a1', command: 'ls' })).toEqual({ command: 'ls' })
  })
})

describe('the feed', () => {
  test('starting sends reset and the empty lead under the session key and adapter name', async () => {
    const { h } = await started()
    expect(h.calls.length).toBe(1)
    expect(h.calls[0]!.session_key).toBe(KEY)
    expect(h.calls[0]!.adapter).toBe(ADAPTER)
    expect(kinds(h.calls[0]!.events)).toEqual(['reset', 'agent'])
  })

  test('a stopped feed records and sends nothing', async () => {
    const { h, dash, advance } = harness()
    dash.turnStarted(undefined)
    dash.userMessage('hi', 'terminal')
    await advance(1000)
    expect(h.calls.length).toBe(0)
    expect(dash.active()).toBe(false)
  })

  test('a lead turn: thinking, then reply text streamed and finished, then the turn end', async () => {
    const { dash, after, advance, sent } = await started()
    // The lead's record has not changed, so only the turn start goes.
    expect(kinds(await after(() => dash.turnStarted(undefined)))).toEqual(['turn_start'])
    let events = await after(() => {
      dash.stepBegan(undefined, 't/0')
      dash.stepChunk(undefined, 't/0', { kind: 'thinking', index: 0, text: '' })
    })
    // Thinking shows as a phase even with no text.
    expect(batches(lastLead(events)).at(-1)?.generating).toEqual({ kind: 'thinking', phaseStartedAt: expect.any(Number) })
    events = await after(() => {
      dash.stepChunk(undefined, 't/0', { kind: 'text', index: 1, text: 'Hel' })
      dash.stepChunk(undefined, 't/0', { kind: 'text', index: 1, text: 'lo' })
    })
    expect(batches(lastLead(events)).at(-1)?.generating?.kind).toBe('replying')
    expect(events.filter((e) => e.kind === 'reply')).toEqual([{ kind: 'reply', text: 'Hello' }])
    events = await after(() => dash.stepEnded(undefined, 't/0', { answer: 'Hello', toolUses: [] }))
    expect(events.filter((e) => e.kind === 'reply')).toEqual([{ kind: 'reply', text: 'Hello', final: true, at: expect.any(Number) }])
    // A step with no tool call leaves no batch.
    expect(batches(lastLead(events))).toEqual([])
    events = await after(() => dash.turnCompleted(undefined, 'Hello', 'answer'))
    // The finished reply is not sent a second time from turn.complete's answer.
    expect(kinds(events)).toEqual(['agent', 'turn_end'])
    await advance(1000)
    expect(sent().filter((e) => e.kind === 'reply' && e.final).length).toBe(1)
  })

  test('a tool-calling step keeps its streamed text as a final reply, in step order', async () => {
    const { dash, after, advance, sent } = await started()
    dash.turnStarted(undefined)
    await advance(1000)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'Checking ' })
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'the files.' })
    dash.toolBegan(undefined, 'u1', 'Bash', { command: 'ls' })
    let events = await after(() => dash.stepEnded(undefined, 't/0', { answer: '', toolUses: [{ id: 'u1', name: 'Bash', input: { command: 'ls' } }] }))
    expect(events.filter((e) => e.kind === 'reply' && e.final)).toEqual([{ kind: 'reply', text: 'Checking the files.', final: true, at: expect.any(Number) }])
    dash.toolEnded(undefined, 'u1', false)
    dash.stepBegan(undefined, 't/1')
    dash.stepChunk(undefined, 't/1', { kind: 'text', index: 0, text: 'Done.' })
    await after(() => dash.stepEnded(undefined, 't/1', { answer: 'Done.', toolUses: [] }))
    events = await after(() => dash.turnCompleted(undefined, 'Done.', 'answer'))
    await advance(1000)
    expect(sent().filter((e) => e.kind === 'reply' && e.final).map((e) => (e as { text: string }).text)).toEqual(['Checking the files.', 'Done.'])
  })

  test('a tool-calling step with no text, or only whitespace, sends no final reply', async () => {
    const { dash, advance, sent } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.toolBegan(undefined, 'u1', 'Bash', { command: 'ls' })
    dash.stepEnded(undefined, 't/0', { answer: '', toolUses: [{ id: 'u1', name: 'Bash', input: { command: 'ls' } }] })
    dash.toolEnded(undefined, 'u1', false)
    dash.stepBegan(undefined, 't/1')
    dash.stepChunk(undefined, 't/1', { kind: 'text', index: 0, text: ' \n ' })
    dash.stepEnded(undefined, 't/1', { answer: '', toolUses: [] })
    await advance(1000)
    expect(sent().filter((e) => e.kind === 'reply' && e.final)).toEqual([])
  })

  test('turn.complete repeating a tool step\'s streamed text sends it once', async () => {
    const { dash, advance, sent } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'Only this.' })
    dash.stepEnded(undefined, 't/0', { answer: '', toolUses: [] })
    dash.turnCompleted(undefined, 'Only this.', 'answer')
    await advance(1000)
    expect(sent().filter((e) => e.kind === 'reply' && e.final).length).toBe(1)
  })

  test('streamed reply text coalesces: one intake call per text interval, not one per chunk', async () => {
    const { h, dash, advance } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    await advance()
    const before = h.calls.length
    // Three seconds of chunks, one every 50 ms.
    for (let i = 0; i < 60; i++) {
      dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'x' })
      await advance(50)
    }
    await advance(TEXT_FLUSH_INTERVAL_MS)
    const calls = h.calls.length - before
    expect(calls).toBeGreaterThanOrEqual(3)
    expect(calls).toBeLessThanOrEqual(5)
    const replies = h.calls.slice(before).flatMap((c) => c.events).filter((e) => e.kind === 'reply')
    expect(replies.at(-1)).toEqual({ kind: 'reply', text: 'x'.repeat(60) })
  })

  test('text alone waits for the slow interval, not the fast one', async () => {
    const { h, dash, advance, sent } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'a' })
    await advance()
    expect(sent().filter((e) => e.kind === 'reply').at(-1)).toEqual({ kind: 'reply', text: 'a' })
    const before = h.calls.length
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'b' })
    // The flush of 'a' was one fast interval ago.
    await advance(TEXT_FLUSH_INTERVAL_MS - FLUSH_INTERVAL_MS - 1)
    expect(h.calls.length).toBe(before)
    await advance(1)
    expect(h.calls.length).toBe(before + 1)
    expect(h.calls.at(-1)?.events).toEqual([{ kind: 'reply', text: 'ab' }])
  })

  test('a tool begin while text waits on the slow timer flushes at the fast interval, carrying the text', async () => {
    const { h, dash, advance } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'a' })
    await advance()
    const before = h.calls.length
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'b' })
    await advance(FLUSH_INTERVAL_MS / 2)
    expect(h.calls.length).toBe(before)
    dash.toolBegan(undefined, 'u1', 'Bash', { command: 'ls' })
    await advance(FLUSH_INTERVAL_MS)
    expect(h.calls.length).toBe(before + 1)
    const events = h.calls.at(-1)!.events
    expect(kinds(events)).toEqual(['agent', 'reply'])
    expect(events.at(-1)).toEqual({ kind: 'reply', text: 'ab' })
  })

  test('the reply\'s end flushes promptly, not on the slow timer', async () => {
    const { h, dash, advance } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'a' })
    await advance()
    const before = h.calls.length
    dash.stepChunk(undefined, 't/0', { kind: 'text', index: 0, text: 'b' })
    await advance(FLUSH_INTERVAL_MS / 2)
    dash.stepEnded(undefined, 't/0', { answer: 'ab', toolUses: [] })
    await advance(FLUSH_INTERVAL_MS)
    expect(h.calls.length).toBe(before + 1)
    expect(h.calls.at(-1)!.events.find((e) => e.kind === 'reply')).toMatchObject({ text: 'ab', final: true })
  })

  test('a tool step: the streaming call by name and characters, then running with its preview, then finished', async () => {
    const { dash, after } = await started()
    dash.turnStarted(undefined)
    let events = await after(() => {
      dash.stepBegan(undefined, 't/0')
      dash.stepChunk(undefined, 't/0', { kind: 'tool', index: 0, id: 'u1', name: 'Bash' })
      dash.stepChunk(undefined, 't/0', { kind: 'input', index: 0, json: '{"command":' })
      dash.stepChunk(undefined, 't/0', { kind: 'input', index: 0, json: '"ls"}' })
    })
    let views = batches(lastLead(events))
    expect(views.at(-1)?.generating).toEqual({ kind: 'tool', phaseStartedAt: expect.any(Number), toolName: 'Bash', chars: 16 })
    // The call starts while the step still streams, as the host runs it.
    events = await after(() => dash.toolBegan(undefined, 'u1', 'Bash', { command: 'ls' }))
    views = batches(lastLead(events))
    expect(views[0]?.running).toEqual({ toolName: 'Bash', argsPreview: 'command: ls', startedAt: expect.any(Number), done: 0, othersRunning: 0 })
    events = await after(() => dash.toolEnded(undefined, 'u1', false))
    // The step still streams, so its batch stays open.
    expect(batches(lastLead(events))[0]?.running?.done).toBe(1)
    events = await after(() => dash.stepEnded(undefined, 't/0', { answer: '', toolUses: [{ name: 'Bash', input: { command: 'ls' } }] }))
    views = batches(lastLead(events))
    expect(views.length).toBe(1)
    expect(views[0]).toEqual({
      epoch: expect.any(String),
      seq: 1,
      toolNames: ['Bash'],
      callCount: 1,
      failed: 0,
      startedAt: expect.any(Number),
      endedAt: expect.any(Number),
      calls: [{ name: 'Bash', startedAt: expect.any(Number), endedAt: expect.any(Number), failed: false }],
    })
  })

  test('a finished batch carries each call\'s name, clocks, and failed flag and the step\'s tokens; a running one neither', async () => {
    const { h, dash, after } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'tool', index: 0, id: 'u1', name: 'Read' })
    dash.stepChunk(undefined, 't/0', { kind: 'tool', index: 1, id: 'u2', name: 'Read' })
    dash.stepChunk(undefined, 't/0', { kind: 'tool', index: 2, id: 'u3', name: 'Bash' })
    dash.stepEnded(undefined, 't/0', {
      answer: '',
      toolUses: [{ name: 'Read', input: { file_path: '/secret' } }, { name: 'Read', input: {} }, { name: 'Bash', input: { command: 'rm -rf x' } }],
      usage: { input_tokens: 5, output_tokens: 60, cache_read_input_tokens: 9000, cache_creation_input_tokens: 0 },
    })
    const t0 = h.now
    dash.toolBegan(undefined, 'u1', 'Read', { file_path: '/secret' })
    dash.toolBegan(undefined, 'u2', 'Read', {})
    dash.toolBegan(undefined, 'u3', 'Bash', { command: 'rm -rf x' })
    let events = await after(() => {
      dash.toolEnded(undefined, 'u1', false)
      dash.toolEnded(undefined, 'u2', true)
    })
    let view = batches(lastLead(events))[0] as View & { calls?: unknown; usage?: unknown }
    expect(view.running).toBeDefined()
    expect(view.calls).toBeUndefined()
    expect(view.usage).toBeUndefined()
    h.now += 700
    const t3 = h.now
    events = await after(() => dash.toolEnded(undefined, 'u3', false))
    view = batches(lastLead(events))[0] as View & { calls?: unknown; usage?: unknown }
    expect(view.calls).toEqual([
      { name: 'Read', startedAt: t0, endedAt: t0, failed: false },
      { name: 'Read', startedAt: t0, endedAt: t0, failed: true },
      { name: 'Bash', startedAt: t0, endedAt: t3, failed: false },
    ])
    expect(view.usage).toEqual({ input: 5, output: 60, cacheRead: 9000, cacheWrite: 0 })
    const payload = JSON.stringify(h.calls.at(-1))
    expect(payload.includes('/secret')).toBe(false)
    expect(payload.includes('rm -rf')).toBe(false)
  })

  test('the calls a finished batch lists are capped; a step with no usage sends none', async () => {
    const { dash, after } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    const uses = Array.from({ length: BATCH_CALL_CAP + 4 }, (_, i) => ({ name: `T${i}`, input: {} }))
    const events = await after(() => {
      dash.stepEnded(undefined, 't/0', { answer: '', toolUses: uses, usage: null })
      dash.turnCompleted(undefined, '', 'answer')
    })
    const view = batches(lastLead(events))[0] as View & { calls?: unknown[]; usage?: unknown }
    expect(view.callCount).toBe(BATCH_CALL_CAP + 4)
    expect(view.calls?.length).toBe(BATCH_CALL_CAP)
    expect(view.usage).toBeUndefined()
  })

  test('a call still running at the turn end counts as failed; an error result counts as failed', async () => {
    const { dash, after } = await started()
    dash.turnStarted(undefined)
    dash.stepBegan(undefined, 't/0')
    dash.stepChunk(undefined, 't/0', { kind: 'tool', index: 0, id: 'u1', name: 'Read' })
    dash.stepChunk(undefined, 't/0', { kind: 'tool', index: 1, id: 'u2', name: 'Bash' })
    dash.stepEnded(undefined, 't/0', { answer: '', toolUses: [{ name: 'Read', input: {} }, { name: 'Bash', input: {} }] })
    dash.toolBegan(undefined, 'u1', 'Read', { file_path: '/a' })
    dash.toolEnded(undefined, 'u1', true)
    dash.toolBegan(undefined, 'u2', 'Bash', { command: 'sleep 9' })
    const events = await after(() => dash.turnCompleted(undefined, '', 'aborted'))
    const view = batches(lastLead(events))[0]
    expect(view?.callCount).toBe(2)
    expect(view?.failed).toBe(2)
    expect(view?.endedAt).toBeDefined()
    expect(kinds(events).at(-1)).toBe('turn_end')
  })

  const grepSteps = (dash: ReturnType<typeof harness>['dash'], from: number, count: number) => {
    for (let i = from; i < from + count; i++) {
      dash.stepBegan(undefined, `t/${i}`)
      dash.stepChunk(undefined, `t/${i}`, { kind: 'tool', index: 0, id: `u${i}`, name: 'Grep' })
      dash.toolBegan(undefined, `u${i}`, 'Grep', { pattern: 'x' })
      dash.toolEnded(undefined, `u${i}`, false)
      dash.stepEnded(undefined, `t/${i}`, { answer: '', toolUses: [{ name: 'Grep', input: {} }] })
    }
  }

  test('finished batches past the newest stay listed until a delivered call carried them, then only the newest are kept', async () => {
    const { h, dash, advance, sent } = await started()
    dash.turnStarted(undefined)
    grepSteps(dash, 0, BATCH_CAP + 3)
    // Eight finished between two calls: all go, so ws-mcp counts each.
    await advance()
    expect(batches(lastLead(sent())).map((v) => v.seq)).toEqual([1, 2, 3, 4, 5, 6, 7, 8])
    // That call was delivered: the next lists only the newest.
    grepSteps(dash, BATCH_CAP + 3, 1)
    await advance()
    expect(batches(lastLead(sent())).map((v) => v.seq)).toEqual([5, 6, 7, 8, 9])
    // An undelivered call keeps what it carried: the resync lists it all again.
    h.answers.push(new Error('server gone'))
    grepSteps(dash, BATCH_CAP + 4, BATCH_CAP + 2)
    await advance()
    const seven = [10, 11, 12, 13, 14, 15, 16]
    expect(batches(lastLead(sent())).map((v) => v.seq)).toEqual(seven)
    await advance(RETRY_MS)
    const retry = h.calls.at(-1)!.events
    expect(kinds(retry)[0]).toBe('reset')
    expect(batches(lastLead(retry)).map((v) => v.seq)).toEqual(seven)
    grepSteps(dash, 2 * BATCH_CAP + 6, 1)
    await advance()
    expect(batches(lastLead(sent())).map((v) => v.seq)).toEqual([13, 14, 15, 16, 17])
  })

  test('with no delivered call, at most BATCH_RETAIN_CAP batches are kept', async () => {
    const { h, dash, advance, sent } = await started()
    dash.turnStarted(undefined)
    h.key = undefined
    grepSteps(dash, 0, BATCH_RETAIN_CAP + 6)
    await advance()
    h.key = KEY
    dash.keyChanged()
    await advance()
    const seqs = batches(lastLead(sent())).map((v) => v.seq)
    expect(seqs.length).toBe(BATCH_RETAIN_CAP)
    expect(seqs[0]).toBe(7)
  })

  test('steps no hook saw: turn.complete\'s answer is still the finished reply, and a call opens its own batch', async () => {
    const { dash, after } = await started()
    dash.turnStarted(undefined)
    let events = await after(() => dash.toolBegan(undefined, 'u9', 'Read', { file_path: '/x' }))
    expect(batches(lastLead(events))[0]?.running?.toolName).toBe('Read')
    events = await after(() => {
      dash.toolEnded(undefined, 'u9', false)
      dash.turnCompleted(undefined, 'All done.', 'answer')
    })
    expect(events.filter((e) => e.kind === 'reply')).toEqual([{ kind: 'reply', text: 'All done.', final: true, at: expect.any(Number) }])
    expect(batches(lastLead(events))[0]?.endedAt).toBeDefined()
  })
})

describe('subagents', () => {
  test('keyed by the native agent id, labelled from the spawn even when their first event came before it', async () => {
    const { dash, after } = await started()
    // The subagent's turn starts before its spawn resolves.
    let events = await after(() => dash.turnStarted('a9820e'))
    expect(lastAgent(events, 'a9820e')).toEqual({ kind: 'agent', agentId: 'a9820e', status: 'running', batches: [] })
    events = await after(() => dash.agentSpawned({ agentId: 'a9820e', parentId: 'p1', description: 'Probe files', subagentType: 'general-purpose' }))
    expect(lastAgent(events, 'a9820e')).toEqual({
      kind: 'agent',
      agentId: 'a9820e',
      parentId: 'p1',
      label: 'Probe files',
      caption: 'general-purpose',
      status: 'running',
      batches: [],
    })
    events = await after(() => {
      dash.stepBegan('a9820e', 's/0')
      dash.stepChunk('a9820e', 's/0', { kind: 'tool', index: 0, id: 'v1', name: 'Bash' })
      dash.toolBegan('a9820e', 'v1', 'Bash', { command: 'ls' })
    })
    expect(batches(lastAgent(events, 'a9820e'))[0]?.running?.toolName).toBe('Bash')
    // Its events never touch the lead.
    expect(lastLead(events)).toBeUndefined()
    events = await after(() => dash.turnCompleted('a9820e', 'report', 'answer'))
    expect(lastAgent(events, 'a9820e')?.status).toBe('done')
    // A subagent's answer is no lead reply.
    expect(events.some((e) => e.kind === 'reply')).toBe(false)
  })

  test('a run ending in an error or an interrupt marks the agent failed; a later run makes it running again', async () => {
    const { dash, after } = await started()
    dash.turnStarted('a1')
    expect(lastAgent(await after(() => dash.turnCompleted('a1', '', 'error')), 'a1')?.status).toBe('failed')
    expect(lastAgent(await after(() => dash.turnStarted('a1')), 'a1')?.status).toBe('running')
    expect(lastAgent(await after(() => dash.turnCompleted('a1', '', 'aborted')), 'a1')?.status).toBe('failed')
  })

  test('a delegate\'s session key comes from its own call of this plugin\'s tools and travels only as sessionKey', async () => {
    const { h, dash, after } = await started()
    dash.turnStarted(undefined)
    dash.turnStarted('d1')
    // Another server's tool with a session_key is not this plugin's key.
    let events = await after(() => dash.toolBegan('d1', 'x1', 'mcp__other__thing', { session_key: 'not-ours' }))
    expect(lastAgent(events, 'd1')?.sessionKey).toBeUndefined()
    events = await after(() => dash.toolBegan('d1', 'x2', `${OWN}workflow_manual`, { session_key: DELEGATE_KEY }))
    expect(lastAgent(events, 'd1')?.sessionKey).toBe(DELEGATE_KEY)
    // The lead's own key passed to the plugin's tools never becomes an agent's.
    await after(() => dash.toolBegan(undefined, 'x3', `${OWN}todo_list`, { session_key: KEY }))
    // A permission ask and a turn on the same tools, so every field that reaches the browser is exercised.
    await after(() => {
      dash.toolChecked({ id: 'x4', tool: `${OWN}config_tune`, input: { session_key: KEY, key: 'k' }, approver: 'person' }, 'ask')
      dash.toolBegan(undefined, 'x4', `${OWN}config_tune`, { session_key: KEY, key: 'k' })
      dash.notified(PERMISSION_PROMPT)
    })
    const payload = JSON.stringify(h.calls.map((c) => c.events))
    // The delegate's key only as its relabelled sessionKey; the lead's key nowhere in the events.
    expect(payload.split(DELEGATE_KEY).length - 1).toBe(payload.split(`"sessionKey":"${DELEGATE_KEY}"`).length - 1)
    expect(payload.includes(KEY)).toBe(false)
    expect(h.calls.every((c) => c.session_key === KEY)).toBe(true)
  })

  test('finished agents past the cap leave the local tree; running ones stay', async () => {
    const { dash, h, advance } = await started()
    dash.turnStarted('keep')
    for (let i = 0; i < 8; i++) {
      dash.turnStarted(`f${i}`)
      h.now += 1
      dash.turnCompleted(`f${i}`, '', 'answer')
    }
    // A failure makes the feed resend the whole tree: what is left of it shows.
    h.answers.push(false)
    dash.userMessage('x', 'terminal')
    await advance()
    await advance(RETRY_MS)
    const resent = h.calls.at(-1)!.events.filter((e) => e.kind === 'agent' && e.agentId !== undefined).map((e) => e.agentId)
    expect(resent).toContain('keep')
    expect(resent.filter((id) => String(id).startsWith('f'))).toEqual(['f3', 'f4', 'f5', 'f6', 'f7'])
  })
})

describe('permission waits', () => {
  test('a person-approved ask shows as a running tool until the notification marks it waiting', async () => {
    const { dash, after } = await started()
    dash.turnStarted(undefined)
    let events = await after(() => {
      dash.toolChecked({ id: 'u1', tool: 'Bash', input: { command: 'touch f' }, approver: 'person' }, 'ask')
      dash.toolBegan(undefined, 'u1', 'Bash', { command: 'touch f' })
    })
    expect(events.some((e) => e.kind === 'permission')).toBe(false)
    expect(batches(lastLead(events))[0]?.running?.argsPreview).toBe('command: touch f')
    events = await after(() => dash.notified(PERMISSION_PROMPT))
    expect(events.filter((e) => e.kind === 'permission')).toEqual([{ kind: 'permission', tool: 'Bash', args: 'command: touch f' }])
    // The call's end clears it, whether the tool ran after approval or was refused.
    events = await after(() => dash.toolEnded(undefined, 'u1', false))
    expect(events.filter((e) => e.kind === 'permission')).toEqual([{ kind: 'permission', clear: true }])
  })

  test('several pending asks are all marked by one notification', async () => {
    const { dash, after } = await started()
    dash.turnStarted(undefined)
    dash.turnStarted('a1')
    dash.toolChecked({ id: 'u1', tool: 'Bash', input: { command: 'rm x' }, approver: 'person' }, 'ask')
    dash.toolChecked({ agentId: 'a1', id: 'v1', tool: 'Write', input: { file_path: '/y' }, approver: 'person' }, 'ask')
    const events = await after(() => dash.notified(PERMISSION_PROMPT))
    expect(events.filter((e) => e.kind === 'permission')).toEqual([
      { kind: 'permission', tool: 'Bash', args: 'command: rm x' },
      { kind: 'permission', agentId: 'a1', tool: 'Write', args: 'file_path: /y' },
    ])
  })

  test('no notification, no marker; and only a person-approved ask with a call id is held', async () => {
    const { dash, after, advance, sent } = await started()
    dash.turnStarted(undefined)
    dash.toolChecked({ id: 'u1', tool: 'Bash', input: {}, approver: 'person' }, 'ask')
    dash.toolBegan(undefined, 'u1', 'Bash', {})
    await advance(10_000)
    dash.toolEnded(undefined, 'u1', false)
    await advance()
    expect(sent().some((e) => e.kind === 'permission')).toBe(false)
    // Settled without a person, an allow, or a query with no call id: the notification marks none of them.
    dash.toolChecked({ id: 'u2', tool: 'Bash', input: {} }, 'ask')
    dash.toolChecked({ id: 'u3', tool: 'Bash', input: {}, approver: 'person' }, 'allow')
    dash.toolChecked({ tool: 'Bash', input: {}, approver: 'person' }, 'ask')
    const events = await after(() => dash.notified(PERMISSION_PROMPT))
    expect(events.some((e) => e.kind === 'permission')).toBe(false)
    // Another notification type marks nothing either.
    dash.toolChecked({ id: 'u4', tool: 'Bash', input: {}, approver: 'person' }, 'ask')
    expect((await after(() => dash.notified('idle_prompt'))).some((e) => e.kind === 'permission')).toBe(false)
  })
})

describe('origins', () => {
  test('a typed prompt tags the turn terminal and enters the ring as a terminal user message', async () => {
    const { dash, after } = await started()
    const events = await after(() => {
      dash.promptSubmitting('terminal', true)
      dash.turnStarted(undefined)
      dash.userMessage('please fix', 'terminal')
    })
    expect(events.find((e) => e.kind === 'turn_start')?.origin).toBe('terminal')
    expect(events.find((e) => e.kind === 'user')).toEqual({ kind: 'user', text: 'please fix', origin: 'terminal', at: expect.any(Number) })
  })

  test('a whiteboard prompt tags the turn and the user message whiteboard', async () => {
    const { dash, after } = await started()
    const events = await after(() => {
      dash.promptSubmitting('whiteboard', true)
      dash.turnStarted(undefined)
      dash.userMessage('board answer', 'whiteboard')
    })
    expect(events.find((e) => e.kind === 'turn_start')?.origin).toBe('whiteboard')
    expect(events.find((e) => e.kind === 'user')?.origin).toBe('whiteboard')
  })

  test('the mailbox push\'s prompt tags the turn mail; a failed submit leaves no mark for the next turn', async () => {
    const { dash, after } = await started()
    let events = await after(() => {
      dash.mailSubmitting()
      dash.promptSubmitting(undefined, true)
      dash.turnStarted(undefined)
    })
    expect(events.find((e) => e.kind === 'turn_start')?.origin).toBe('mail')
    await after(() => dash.turnCompleted(undefined, '', 'answer'))
    dash.mailSubmitting()
    dash.mailSubmitFailed()
    events = await after(() => dash.turnStarted(undefined))
    expect(events.find((e) => e.kind === 'turn_start')?.origin).toBeUndefined()
  })

  test('the mod\'s own whiteboard submit tags the turn and enters the ring once; a failed one leaves no mark', async () => {
    const { dash, after } = await started()
    let events = await after(() => {
      dash.whiteboardSubmitting()
      dash.turnStarted(undefined)
      dash.whiteboardSubmitted('pick B', true)
    })
    expect(events.find((e) => e.kind === 'turn_start')?.origin).toBe('whiteboard')
    expect(events.filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'pick B', origin: 'whiteboard', at: expect.any(Number) }])
    await after(() => dash.turnCompleted(undefined, '', 'answer'))
    // Should a hook see the same submit, its message is not entered a second time.
    events = await after(() => {
      dash.whiteboardSubmitting()
      dash.userMessage('pick C', 'whiteboard')
      dash.whiteboardSubmitted('pick C', true)
    })
    expect(events.filter((e) => e.kind === 'user').map((e) => e.text)).toEqual(['pick C'])
    dash.whiteboardSubmitting()
    dash.whiteboardSubmitted('dropped', false)
    events = await after(() => dash.turnStarted(undefined))
    expect(events.find((e) => e.kind === 'turn_start')?.origin).toBeUndefined()
    expect(events.some((e) => e.kind === 'user')).toBe(false)
  })

  test('a prompt typed into a running turn is a user message and leaves the turn\'s origin alone', async () => {
    const { dash, after } = await started()
    dash.mailSubmitting()
    dash.turnStarted(undefined)
    await after(() => {})
    const events = await after(() => {
      dash.promptSubmitting('terminal', false)
      dash.userMessage('also this', 'terminal')
    })
    expect(kinds(events)).toEqual(['user'])
  })
})

describe('delivery', () => {
  test('a failed call puts its conversation back and resyncs: reset, the open turn, the full state', async () => {
    const { h, dash, advance } = await started()
    dash.promptSubmitting('terminal', true)
    dash.turnStarted(undefined)
    dash.turnStarted('a1')
    await advance()
    h.answers.push(new Error('server gone'))
    dash.userMessage('lost?', 'terminal')
    await advance()
    const failed = h.calls.length
    // Nothing more until the retry wait has passed.
    await advance(RETRY_MS / 2)
    expect(h.calls.length).toBe(failed)
    await advance(RETRY_MS)
    const retry = h.calls.at(-1)!.events
    expect(kinds(retry).slice(0, 2)).toEqual(['reset', 'turn_start'])
    expect(retry[1]?.origin).toBe('terminal')
    expect(retry.some((e) => e.kind === 'user' && e.text === 'lost?')).toBe(true)
    expect(retry.some((e) => e.kind === 'agent' && e.agentId === undefined)).toBe(true)
    expect(retry.some((e) => e.kind === 'agent' && e.agentId === 'a1')).toBe(true)
    expect(h.logs.some((l) => l.includes('server gone'))).toBe(true)
  })

  test('an error result counts as not delivered too, and a call that never answers is abandoned', async () => {
    const { h, dash, advance } = await started()
    h.answers.push(false)
    dash.userMessage('one', 'terminal')
    await advance()
    await advance(RETRY_MS)
    expect(kinds(h.calls.at(-1)!.events)[0]).toBe('reset')
    h.answers.push('hang')
    dash.userMessage('two', 'terminal')
    await advance()
    const hung = h.calls.length
    await advance(CALL_TIMEOUT_MS + RETRY_MS)
    expect(h.calls.length).toBe(hung + 1)
    expect(h.calls.at(-1)!.events.some((e) => e.kind === 'user' && e.text === 'two')).toBe(true)
  })

  test('with no session key nothing is sent; the first key gets everything held, behind a reset', async () => {
    const { h, dash, advance } = await started({ key: undefined })
    dash.promptSubmitting('terminal', true)
    dash.turnStarted(undefined)
    dash.userMessage('early', 'terminal')
    await advance(10_000)
    expect(h.calls.length).toBe(0)
    h.key = KEY
    dash.keyChanged()
    await advance()
    expect(h.calls.length).toBe(1)
    const events = h.calls[0]!.events
    expect(kinds(events)[0]).toBe('reset')
    expect(events.filter((e) => e.kind === 'turn_start').length).toBe(1)
    expect(events.some((e) => e.kind === 'user' && e.text === 'early')).toBe(true)
  })

  test('a new key resyncs the new session from scratch', async () => {
    const { h, dash, advance } = await started()
    dash.turnStarted('a1')
    await advance()
    h.key = 'second-key'
    dash.keyChanged()
    await advance()
    const call = h.calls.at(-1)!
    expect(call.session_key).toBe('second-key')
    expect(kinds(call.events)[0]).toBe('reset')
    expect(call.events.some((e) => e.agentId === 'a1')).toBe(true)
  })

  test('a restart drops the conversation\'s state and resets the view; a stop sends nothing more', async () => {
    const { h, dash, advance } = await started()
    dash.turnStarted('a1')
    await advance()
    dash.restart()
    await advance()
    expect(kinds(h.calls.at(-1)!.events)).toEqual(['reset', 'agent'])
    dash.stop()
    dash.turnStarted(undefined)
    await advance(1000)
    expect(kinds(h.calls.at(-1)!.events)).toEqual(['reset', 'agent'])
  })

  test('a malformed event is logged and never throws into the host', async () => {
    const { h, dash } = await started()
    expect(() => dash.stepChunk(undefined, 't/0', null as never)).not.toThrow()
    expect(h.logs.some((l) => l.includes('dashboard step chunk failed'))).toBe(true)
  })
})

describe('usage status', () => {
  const OFF: IntakeResult = { ok: true, text: 'dashboard off: nothing recorded\n' }
  const figures = (tokens: number) => ({ context: { tokens, window: 200_000, percent: tokens / 2_000 } })
  const statuses = (events: DashboardEvent[]) => events.filter((e) => e.kind === 'status')

  test('held figures go only after a result names status, and stop when a result does not', async () => {
    const { h, dash, advance, sent } = await started()
    dash.status(figures(10_000))
    await advance(1000)
    expect(statuses(sent())).toEqual([])
    // A result naming status: the held figures go with the next call.
    h.accepts = true
    dash.turnStarted('a1')
    await advance()
    await advance()
    expect(statuses(sent())).toEqual([{ kind: 'status', ...figures(10_000) }])
    dash.status(figures(20_000))
    await advance()
    expect(statuses(sent()).at(-1)).toEqual({ kind: 'status', ...figures(20_000) })
    // The same figures again change nothing.
    const calls = h.calls.length
    dash.status(figures(20_000))
    await advance(1000)
    expect(h.calls.length).toBe(calls)
    // A "dashboard off" result names nothing: newer figures are held, not sent.
    h.answers.push(OFF)
    dash.turnStarted('a2')
    await advance()
    dash.status(figures(30_000))
    await advance(1000)
    expect(statuses(sent()).length).toBe(2)
    // Named again: the latest figures follow.
    dash.turnStarted('a3')
    await advance()
    await advance()
    expect(statuses(sent()).at(-1)).toEqual({ kind: 'status', ...figures(30_000) })
  })

  test('an undelivered call turns status off; the resync\'s result turns it on and the held figures go again', async () => {
    const { h, dash, advance, sent } = await started()
    h.accepts = true
    dash.status(figures(10_000))
    dash.turnStarted('a1')
    await advance()
    await advance()
    expect(statuses(sent()).length).toBe(1)
    h.answers.push(new Error('server gone'))
    dash.turnStarted('a2')
    await advance()
    await advance(RETRY_MS)
    const resync = h.calls.findLastIndex((c) => c.events[0]?.kind === 'reset')
    // The resync call itself carries no status: the failed call left status off.
    expect(statuses(h.calls[resync]!.events)).toEqual([])
    await advance()
    expect(statuses(h.calls.slice(resync).flatMap((c) => c.events))).toEqual([{ kind: 'status', ...figures(10_000) }])
  })

  test('a restart keeps the held figures and sends them after its reset; a stop drops them', async () => {
    const { h, dash, advance } = await started()
    h.accepts = true
    dash.status(figures(10_000))
    dash.turnStarted('a1')
    await advance()
    await advance()
    const before = h.calls.length
    dash.restart()
    await advance()
    await advance()
    const after = h.calls.slice(before).flatMap((c) => c.events)
    expect(kinds(after)[0]).toBe('reset')
    expect(statuses(after)).toEqual([{ kind: 'status', ...figures(10_000) }])
    dash.stop()
    dash.start()
    await advance()
    await advance(1000)
    expect(statuses(h.calls.slice(before).flatMap((c) => c.events)).length).toBe(1)
  })
})

describe('compaction', () => {
  const acceptsCompact: IntakeResult = { ok: true, text: 'ok: 1 of 1 events applied\naccepts: status, compact\n' }

  test('a start held before any result names compact goes once one does; an end lost with a failed call is put back', async () => {
    const { h, dash, advance, sent } = await started()
    dash.compactStarted('auto')
    await advance()
    expect(sent().some((e) => e.kind === 'compact')).toBe(false)
    // A result naming compact turns it on: the held start follows, with its own start time.
    h.answers.push(acceptsCompact, acceptsCompact)
    dash.turnStarted(undefined)
    await advance()
    await advance()
    expect(sent().filter((e) => e.kind === 'compact')).toEqual([{ kind: 'compact', phase: 'start', trigger: 'auto', at: 1_100 }])
    h.answers.push(new Error('server gone'))
    dash.compactEnded('auto', true)
    await advance()
    h.answers.push(acceptsCompact, acceptsCompact)
    await advance(RETRY_MS)
    const retry = h.calls.at(-1)!.events
    expect(kinds(retry)[0]).toBe('reset')
    // The end is a conversation line a resync cannot rebuild, so it travels again; the settled start does not.
    expect(retry.filter((e) => e.kind === 'compact')).toEqual([{ kind: 'compact', phase: 'end', trigger: 'auto', at: expect.any(Number) }])
  })

  test('an end while compact is not accepted is dropped; a restart forgets a running compaction', async () => {
    const { h, dash, advance, sent } = await started()
    dash.compactStarted('manual')
    dash.compactEnded('manual', false)
    await advance()
    expect(sent().some((e) => e.kind === 'compact')).toBe(false)
    h.answers.push(acceptsCompact, acceptsCompact, acceptsCompact)
    dash.turnStarted(undefined)
    await advance()
    // Accepted now; a compaction the conversation's end overtakes is not sent into the next one.
    dash.compactStarted(undefined)
    dash.restart()
    await advance()
    await advance()
    expect(sent().some((e) => e.kind === 'compact')).toBe(false)
    expect(kinds(h.calls.at(-1)!.events)[0]).toBe('reset')
  })
})

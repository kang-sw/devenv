import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { CONTINUATION_WINDOW_MS } from './compaction'
import { ADVISORY_KEY, advisoryText, LEVER_DESCRIPTION, LEVER_RESULT, LEVER_RESULT_NO_RESUME, RESUME_TEXT, thresholdsFor, vetoedResumeText } from './timing'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the compaction feature's wiring; the feature's state
// machine is covered in compaction.test.ts. The same file is mirrored into
// every package that ships the mod, and the kit has no plugin-name override,
// so each test reads the package identity from the registrations: the ws
// tests return early elsewhere, and the inert-package tests early in ws.

const WINDOW = { rawMaxTokens: 200_000, autoCompactThreshold: 170_000, isAutoCompactEnabled: true }
const T = thresholdsFor(WINDOW)!

type CompactAnswer = { skip: string } | 'lands' | 'rejects'

/** A compaction leaves at least one message: the summary. */
const SUMMARY = [{ role: 'user', text: 'summary', toolUses: [] }]

function world(on: On, opts: { processOk?: boolean; refuseRegister?: string; advisory?: unknown; advisoryError?: boolean } = {}) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    registered: [] as { name: string; description: string; isDeferred?: boolean; inputSchema?: unknown }[],
    usageCalls: [] as unknown[],
    /** Who compacted, and with what instructions (the kit leaves a plugin call's trigger unset; the engine sets `plugin`). */
    compactCalls: [] as { origin: string; instructions: unknown }[],
    compactAnswers: [] as CompactAnswer[],
    /** Prompts submitted from elsewhere than the composer: the mod's own, among them. */
    prompts: [] as string[],
    /** The next such prompts refused beneath the mod. */
    dropPrompts: 0,
    /** Prompts the person typed, with the context they entered with. */
    typed: [] as { text: string; context: readonly string[] | undefined }[],
    logs: [] as { text: string; to: string }[],
    /** Delays (ms on the mock clock) for the next store deletes, in call order. */
    deleteDelays: [] as number[],
    /** The advisory key's config_get calls the mod made, with their server (other features read keys of their own). */
    configGets: [] as { server: string; args: Record<string, unknown> }[],
  }
  on('mcp.connect', ($, e) => ({ value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }))
  on('mcp.call', ($, e) => {
    if (e.tool !== 'config_get') return { value: { content: [{ type: 'text', text: `no ${e.tool} here` }], isError: true } }
    if (e.args.key !== ADVISORY_KEY) return { value: { content: [{ type: 'text', text: JSON.stringify({ key: e.args.key, value: null, scope: 'unset' }) }], isError: false } }
    w.configGets.push({ server: e.server, args: e.args })
    if (opts.advisoryError) return { value: { content: [{ type: 'text', text: 'ws-mcp is unhealthy' }], isError: true } }
    const value = opts.advisory === undefined ? '50%' : opts.advisory
    return { value: { content: [{ type: 'text', text: JSON.stringify({ key: e.args.key, value, scope: 'project' }) }], isError: false } }
  })
  on('store.get', () => ({ value: undefined }))
  on('store.set', () => ({ value: undefined }))
  on('store.delete', async () => {
    const delay = w.deleteDelays.shift()
    if (delay) await clock.sleep(delay)
    return { value: undefined }
  })
  on('ui.log', ($, e) => (w.logs.push({ text: e.text, to: e.to }), { value: undefined }))
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('process.run', () =>
    opts.processOk === false
      ? { deny: 'no host commands on this surface' }
      : { value: { exitCode: 0, stdout: 'Python 3.12.0\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } },
  )
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    if (e.name === opts.refuseRegister) return { deny: 'refused by the test' }
    w.registered.push({ name: e.name, description: e.description, isDeferred: e.isDeferred, inputSchema: e.inputSchema })
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  on('session.usage', ($, e) => {
    w.usageCalls.push(e)
    return { value: { startedAt: 0, context: { window: 1_000_000, breakdown: e.breakdown ? WINDOW : undefined }, rateLimits: [] } as never }
  })
  on('session.compact', ($, e, next) => {
    w.compactCalls.push({ origin: next.origin.plugin, instructions: e.instructions })
    const answer = w.compactAnswers.shift() ?? 'lands'
    if (answer === 'rejects') throw new Error('a turn is running')
    return answer === 'lands' ? { messages: SUMMARY } : answer
  })
  on('prompt.submit', ($, e) => {
    if (e.origin?.kind === 'composer') {
      w.typed.push({ text: e.text, context: e.context })
      return { text: e.text, context: e.context }
    }
    if (w.dropPrompts > 0) {
      w.dropPrompts -= 1
      return { drop: 'blocked beneath' }
    }
    w.prompts.push(e.text)
    return { text: e.text }
  })
  return w
}

type World = ReturnType<typeof world>

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

async function measure($: Engine, w: World, tokens: number) {
  await $.session.measure({ context: { tokens, window: 1_000_000 }, rateLimits: [], changed: ['context'] } as never)
  await w.clock.settle()
}

async function turnEnd($: Engine, w: World, opts: { reason?: string; agentId?: string } = {}) {
  const reason = opts.reason ?? 'answer'
  await $.turn.complete({
    answer: '',
    durationMs: 1,
    isAborted: reason === 'aborted',
    turnId: 't',
    reason,
    ...(opts.agentId ? { agentId: opts.agentId } : {}),
  } as never)
  await w.clock.settle()
}

/** A turn starting: the main thread's, or a subagent's with `agentId`. */
async function turnStart($: Engine, w: World, agentId?: string) {
  await $.turn.start({ text: '', turnId: 't', ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
}

/** A lever call with the given tool arguments (`{ continue: true }`, or none). */
async function lever($: Engine, w: World, args: Record<string, unknown> = {}, agentId?: string) {
  const res = await $.tool.call({ tool: `mcp__${w.plugin}__request_compaction`, ...args, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

/**
 * A prompt that enters and starts a main-thread turn: typed at the composer,
 * a slash command's line with `command`, or from elsewhere with `kind`.
 */
async function prompted($: Engine, w: World, text: string, opts: { command?: { name: string; args: string }; kind?: string } = {}) {
  const res = await $.prompt.submit({ text, origin: { kind: opts.kind ?? 'composer' }, ...(opts.command ? { command: opts.command } : {}) } as never)
  await w.clock.settle()
  await turnStart($, w)
  return res
}

const isWs = (w: World) => w.plugin === 'ws'

/** The transcript (user-visible) lines the mod logged. */
const transcript = (w: World) => w.logs.filter((l) => l.to === 'transcript').map((l) => l.text)

test('ws registers the compaction request tool with its pinned description and boolean flag; another package registers none', async ($, on) => {
  const w = world(on)
  await start($, w)
  const tool = w.registered.find((r) => r.name === 'request_compaction')
  if (isWs(w)) {
    expect(tool).toEqual({ name: 'request_compaction', description: LEVER_DESCRIPTION, isDeferred: false, inputSchema: tool?.inputSchema })
    // The flag is optional: the schema lists it, and does not require it.
    expect(tool?.inputSchema).toEqual({ type: 'object', properties: { continue: { type: 'boolean' } } })
  } else {
    expect(['wsflow']).toContain(w.plugin)
    expect(tool).toBeUndefined()
  }
})

test('compaction setup does not depend on the mailbox setup', async ($, on) => {
  const w = world(on, { processOk: false })
  await start($, w)
  const names = w.registered.map((r) => r.name)
  expect(names.filter((n) => n.startsWith('mailbox_'))).toEqual([])
  if (w.plugin === 'ws') expect(names.filter((n) => n === 'request_compaction')).toEqual(['request_compaction'])
})

test('mailbox setup does not depend on the compaction setup', async ($, on) => {
  const w = world(on, { refuseRegister: 'request_compaction' })
  await start($, w)
  const names = w.registered.map((r) => r.name)
  expect(names.filter((n) => n.startsWith('mailbox_'))).toEqual(['mailbox_watch'])
  expect(names.filter((n) => n === 'request_compaction')).toEqual([])
})

test('ws: a crossing that session.measure reports submits the advisory, reading the summary breakdown once', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await measure($, w, 105_000)
  expect(w.prompts).toEqual([advisoryText(T, 104_000)])
  expect(w.usageCalls).toEqual([{ breakdown: 'summary' }])
})

test('ws: each session start reads the advisory key once over the plugin server, and a tuned value moves the advisory', async ($, on) => {
  const w = world(on, { advisory: '40%|300k' })
  await start($, w)
  if (!isWs(w)) {
    expect(w.configGets).toEqual([])
    return
  }
  expect(w.configGets).toEqual([{ server: 'plugin:ws:ws', args: { key: 'claude.compaction_advisory_percent', format: 'json' } }])
  await measure($, w, 50_000)
  await measure($, w, 79_000)
  expect(w.prompts).toEqual([])
  await measure($, w, 81_000)
  expect(w.prompts).toEqual([advisoryText(thresholdsFor(WINDOW, { percent: 40, tokens: 300_000 })!, 81_000)])
  expect(w.prompts[0]).toContain('(advisory point: 40%)')
  await start($, w)
  expect(w.configGets.length).toBe(2)
})

for (const [what, opts] of [
  ['reports an error', { advisoryError: true }],
  ['returns a stored value that does not parse', { advisory: 'lots' }],
] as const) {
  test(`ws: an advisory read that ${what} leaves the advisory at 50%`, async ($, on) => {
    const w = world(on, opts)
    await start($, w)
    if (!isWs(w)) return
    expect(w.configGets.length).toBe(1)
    await measure($, w, 50_000)
    await measure($, w, 81_000)
    expect(w.prompts).toEqual([])
    await measure($, w, 104_000)
    expect(w.prompts).toEqual([advisoryText(T, 104_000)])
  })
}

test('ws: a lever call with the flag compacts after the main-thread turn end, then submits the resume prompt once', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  const res = await lever($, w, { continue: true })
  expect(res.result).toBe(LEVER_RESULT)
  await turnEnd($, w, { agentId: 'sub-1' })
  await turnEnd($, w, { reason: 'aborted' })
  expect(w.compactCalls).toEqual([])
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'ws', instructions: undefined }])
  expect(w.prompts).toEqual([])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([RESUME_TEXT])
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(1)
  expect(w.prompts.length).toBe(1)
})

test('ws: a lever call without the flag compacts at the turn end and submits nothing, with no window', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  const res = await lever($, w)
  expect(res.result).toBe(LEVER_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'ws', instructions: undefined }])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(1)
})

test('ws: a leftover continuation argument, or a non-boolean flag, reads as unset', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  expect((await lever($, w, { continuation: 'go on' })).result).toBe(LEVER_RESULT_NO_RESUME)
  expect((await lever($, w, { continue: 'true' })).result).toBe(LEVER_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(1)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
})

test('ws: a repeat lever call replaces the flag, latest wins', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await lever($, w)
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await lever($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.compactCalls.length).toBe(2)
  expect(w.prompts).toEqual([RESUME_TEXT])
})

test("ws: the lever's own landing resets the latch, though the engine routes it past the feature's own hook", async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'ws', instructions: undefined }])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  // Usage still past the advisory after the landing: a reset latch delivers it again, from a fresh breakdown.
  await measure($, w, 104_000)
  expect(w.prompts).toEqual([advisoryText(T, 104_000), RESUME_TEXT, advisoryText(T, 104_000)])
  expect(w.usageCalls.length).toBe(2)
  expect(transcript(w)).toEqual([])
})

test('ws: a subagent lever call is refused and requests nothing', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  const fromSub = await lever($, w, { continue: true }, 'sub-1')
  expect(fromSub.deny).toBe('request_compaction compacts the main session only; called from subagent sub-1.')
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([])
  expect(w.prompts).toEqual([])
})

test('ws: a rejected compaction retries at the next main-thread turn end', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  w.compactAnswers.push('rejects')
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(1)
  expect(w.prompts).toEqual([])
  // The rejection's own text is the engine's; the notice carries it whole.
  expect(transcript(w).length).toBe(1)
  expect(transcript(w)[0]).toMatch(/^ws compaction: the requested compaction was rejected \(.+\); retrying at the next turn end$/)
  await w.clock.advance(60_000)
  expect(w.compactCalls.length).toBe(1)
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(2)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([RESUME_TEXT])
})

test('ws: a vetoed compaction with the flag still submits the noted resume prompt when no turn started', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([vetoedResumeText('blocked by policy')])
})

test('ws: a vetoed compaction with the flag submits nothing when a turn started within the window', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
  await turnStart($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
})

test('ws: a manual compaction landing first drops the compact step; the resume prompt follows at the turn end, unguarded', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await $.session.compact({ trigger: 'manual', messages: SUMMARY } as never)
  await w.clock.settle()
  // No window is open: a turn start before that turn end does not cancel it.
  await turnStart($, w)
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'engine', instructions: undefined }])
  expect(w.prompts).toEqual([RESUME_TEXT])
})

test('ws: session.end drops a pending request and starts the latch over', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await measure($, w, 50_000)
  await lever($, w, { continue: true })
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await turnEnd($, w)
  await measure($, w, 120_000)
  expect(w.compactCalls).toEqual([])
  expect(w.prompts).toEqual([])
})

test('ws: session.end resets the compaction state before awaiting the store delete', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  // A delete still pending stands for one the hook's time bound cuts short.
  w.deleteDelays.push(100)
  const ended = $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([])
  expect(w.prompts).toEqual([])
  await w.clock.advance(100)
  await ended
})

test('another package stays inert: no usage read, no prompt, no compaction, and compactions pass through untouched', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (isWs(w)) return
  await measure($, w, 50_000)
  await measure($, w, 160_000)
  await turnEnd($, w)
  const res = await $.session.compact({ trigger: 'manual', messages: SUMMARY } as never)
  await w.clock.settle()
  expect(res).toEqual({ messages: SUMMARY })
  expect(w.usageCalls).toEqual([])
  expect(w.prompts).toEqual([])
  expect(w.compactCalls).toEqual([{ origin: 'engine', instructions: undefined }])
})

test('ws: a prompt typed within the window starts the first turn, so the resume prompt is not submitted; the typed prompt carries nothing', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS - 1)
  await prompted($, w, 'what I typed meanwhile')
  expect(w.typed).toEqual([{ text: 'what I typed meanwhile', context: undefined }])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([])
})

test('ws: a slash command or a prompt not typed at the composer that starts a turn within the window cancels the resume prompt too', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await prompted($, w, '/cost', { command: { name: 'cost', args: '' } })
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await lever($, w, { continue: true })
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(2)
  await prompted($, w, 'from a phone', { kind: 'bridge' })
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual(['from a phone'])
})

test("ws: a background agent's notification turn within the window cancels the resume prompt", async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  // A turn started with no prompt through the composer: a notification, a queued submit.
  await turnStart($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
})

test('ws: a subagent turn started within the window does not cancel the resume prompt', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await turnStart($, w, 'sub-1')
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([RESUME_TEXT])
})

test('ws: a prompt typed after the window finds the resume prompt already submitted', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await prompted($, w, 'too late')
  expect(w.prompts).toEqual([RESUME_TEXT])
  expect(w.typed).toEqual([{ text: 'too late', context: undefined }])
})

test('ws: a session end inside the window records the held resume prompt in the transcript', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([`ws compaction: a prompt was not submitted (session.end clear):\n${RESUME_TEXT}`])
})

test('ws: a resume prompt refused beneath the mod is recorded in the transcript', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, { continue: true })
  await turnEnd($, w)
  w.dropPrompts = 1
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([`ws compaction: a prompt was not submitted (dropped: blocked beneath):\n${RESUME_TEXT}`])
})

test('another package attaches nothing to a typed prompt', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (isWs(w)) return
  await prompted($, w, 'hello')
  expect(w.typed).toEqual([{ text: 'hello', context: undefined }])
})

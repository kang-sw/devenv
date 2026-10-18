import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { CONTINUATION_WINDOW_MS } from './compaction'
import { advisoryText, continuationText, LEVER_DESCRIPTION, LEVER_RESULT, skippedContinuationText, thresholdsFor } from './timing'

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

function world(on: On, opts: { processOk?: boolean; refuseRegister?: string } = {}) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    registered: [] as { name: string; description: string; isDeferred?: boolean }[],
    usageCalls: [] as unknown[],
    /** Who compacted, and with what instructions (the kit leaves a plugin call's trigger unset; the engine sets `plugin`). */
    compactCalls: [] as { origin: string; instructions: unknown }[],
    compactAnswers: [] as CompactAnswer[],
    prompts: [] as string[],
    /** Prompts the person typed, with the context they entered with. */
    typed: [] as { text: string; context: readonly string[] | undefined }[],
    /** The next typed prompts refused beneath the mod. */
    dropTyped: 0,
    logs: [] as { text: string; to: string }[],
    /** Delays (ms on the mock clock) for the next store deletes, in call order. */
    deleteDelays: [] as number[],
  }
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
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('process.run', () =>
    opts.processOk === false
      ? { deny: 'no host commands on this surface' }
      : { value: { exitCode: 0, stdout: 'Python 3.12.0\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } },
  )
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    if (e.name === opts.refuseRegister) return { deny: 'refused by the test' }
    w.registered.push({ name: e.name, description: e.description, isDeferred: e.isDeferred })
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
    if (e.origin?.kind !== 'composer') return (w.prompts.push(e.text), { text: e.text })
    if (w.dropTyped > 0) {
      w.dropTyped -= 1
      return { drop: 'blocked beneath' }
    }
    w.typed.push({ text: e.text, context: e.context })
    return { text: e.text, context: e.context }
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

async function lever($: Engine, w: World, continuation: unknown, agentId?: string) {
  const res = await $.tool.call({ tool: `mcp__${w.plugin}__request_compaction`, continuation, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

/** A prompt the person typed (origin composer), or a slash command's line with `command`. */
async function type($: Engine, w: World, text: string, opts: { command?: { name: string; args: string }; kind?: string } = {}) {
  const res = await $.prompt.submit({ text, origin: { kind: opts.kind ?? 'composer' }, ...(opts.command ? { command: opts.command } : {}) } as never)
  await w.clock.settle()
  return res
}

const isWs = (w: World) => w.plugin === 'ws'

/** The transcript (user-visible) lines the mod logged. */
const transcript = (w: World) => w.logs.filter((l) => l.to === 'transcript').map((l) => l.text)

test('ws registers the compaction request tool with its pinned description; another package registers none', async ($, on) => {
  const w = world(on)
  await start($, w)
  const tool = w.registered.find((r) => r.name === 'request_compaction')
  if (isWs(w)) {
    expect(tool).toEqual({ name: 'request_compaction', description: LEVER_DESCRIPTION, isDeferred: false })
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

test('ws: a lever call compacts after the main-thread turn end, then submits its continuation once', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  const res = await lever($, w, 'continue Phase 2')
  expect(res.result).toBe(LEVER_RESULT)
  await turnEnd($, w, { agentId: 'sub-1' })
  await turnEnd($, w, { reason: 'aborted' })
  expect(w.compactCalls).toEqual([])
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'ws', instructions: undefined }])
  expect(w.prompts).toEqual([])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([continuationText('continue Phase 2')])
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(1)
  expect(w.prompts.length).toBe(1)
})

test("ws: the lever's own landing resets the latch, though the engine routes it past the feature's own hook", async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await lever($, w, 'go on')
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'ws', instructions: undefined }])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  // Usage still past the advisory after the landing: a reset latch delivers it again, from a fresh breakdown.
  await measure($, w, 104_000)
  expect(w.prompts).toEqual([advisoryText(T, 104_000), continuationText('go on'), advisoryText(T, 104_000)])
  expect(w.usageCalls.length).toBe(2)
  expect(transcript(w)).toEqual([])
})

test('ws: a subagent lever call is refused and an invalid continuation is a tool error', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  const fromSub = await lever($, w, 'go on', 'sub-1')
  expect(fromSub.deny).toBe('request_compaction compacts the main session only; called from subagent sub-1.')
  const multi = await lever($, w, 'one\ntwo')
  expect(multi.isError).toBe(true)
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([])
})

test('ws: a rejected compaction retries at the next main-thread turn end', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
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
  expect(w.prompts).toEqual([continuationText('go on')])
})

test('ws: a vetoed compaction still submits the continuation with the note', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([skippedContinuationText('blocked by policy', 'go on')])
})

test('ws: a manual compaction landing first drops the compact step; the continuation follows at the turn end', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  await $.session.compact({ trigger: 'manual', messages: SUMMARY } as never)
  await w.clock.settle()
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([{ origin: 'engine', instructions: undefined }])
  expect(w.prompts).toEqual([continuationText('go on')])
})

test('ws: session.end drops a pending request and starts the latch over', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await measure($, w, 50_000)
  await lever($, w, 'go on')
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
  await lever($, w, 'go on')
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

test('ws: a prompt typed within the window after the lever compaction carries the continuation, which starts no turn of its own', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS - 1)
  await type($, w, 'what I typed meanwhile')
  expect(w.typed).toEqual([{ text: 'what I typed meanwhile', context: [continuationText('go on')] }])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await type($, w, 'a later prompt')
  expect(w.typed[1]).toEqual({ text: 'a later prompt', context: undefined })
})

test('ws: a slash command, or a prompt not typed at the composer, leaves the continuation to the window end', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  await turnEnd($, w)
  await type($, w, '/cost', { command: { name: 'cost', args: '' } })
  await type($, w, 'from a phone', { kind: 'bridge' })
  expect(w.typed[0]).toEqual({ text: '/cost', context: undefined })
  expect(w.prompts).toEqual(['from a phone'])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual(['from a phone', continuationText('go on')])
})

test('ws: a prompt typed after the window finds the continuation already submitted', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await type($, w, 'too late')
  expect(w.prompts).toEqual([continuationText('go on')])
  expect(w.typed).toEqual([{ text: 'too late', context: undefined }])
})

test('ws: when the prompt that took the continuation is dropped, the continuation is submitted at once and once only', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  await turnEnd($, w)
  w.dropTyped = 1
  const res = await type($, w, 'refused prompt')
  expect(res.drop).toBe('blocked beneath')
  expect(w.prompts).toEqual([continuationText('go on')])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([continuationText('go on')])
})

test('ws: a session end inside the window records the held continuation in the transcript', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  await turnEnd($, w)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([`ws compaction: a prompt was not submitted (session.end clear):\n${continuationText('go on')}`])
})

test('another package attaches nothing to a typed prompt', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (isWs(w)) return
  await type($, w, 'hello')
  expect(w.typed).toEqual([{ text: 'hello', context: undefined }])
})

import { expect, mock, test, type TestBody } from 'claude-code/testing'

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

function world(on: On, opts: { processOk?: boolean } = {}) {
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
    logs: [] as { text: string; to: string }[],
  }
  on('store.get', () => ({ value: undefined }))
  on('store.set', () => ({ value: undefined }))
  on('store.delete', () => ({ value: undefined }))
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
  on('prompt.submit', ($, e) => (w.prompts.push(e.text), { text: e.text }))
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

const isWs = (w: World) => w.plugin === 'ws'

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
  if (w.plugin === 'ws') expect(names).toEqual(['request_compaction'])
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
  expect(w.prompts).toEqual([continuationText('continue Phase 2')])
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(1)
  expect(w.prompts.length).toBe(1)
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
  await w.clock.advance(60_000)
  expect(w.compactCalls.length).toBe(1)
  await turnEnd($, w)
  expect(w.compactCalls.length).toBe(2)
  expect(w.prompts).toEqual([continuationText('go on')])
})

test('ws: a vetoed compaction still submits the continuation with the note', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await lever($, w, 'go on')
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
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

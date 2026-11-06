import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { ADAPTER, INBOX_DRAIN_TOOL, INTAKE_TOOL, labelWhiteboard, type DashboardEvent } from './dashboard'
import { KNOB } from './register'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the dashboard feed: the plugin runs inside the
// engine's own host, and the hooks below stand for the engine and the MCP
// server beneath it. The same file is mirrored into every package that ships
// the mod and runs identically in each: the feed runs while its knob reads
// true through the world's config_get answers, and its key here is the one
// the mailbox push is bound to. The package name only spells tool names.

const KEY = 'k1'

type Intake = 'ok' | 'error' | 'throw'

function world(on: On, opts: { dashboard?: unknown; intake?: Intake } = {}) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    /** Every intake call's arguments, in order. */
    intakes: [] as { session_key: string; adapter: string; events: DashboardEvent[] }[],
    intake: opts.intake ?? ('ok' as Intake),
    /** Ends the latest wait child: its exit code and, for an arrival, the kinds its JSON names (mail when unset). */
    exits: [] as ((code: number, kinds?: string[]) => void)[],
    spawns: 0,
    /** Each wait child's argv. */
    argvs: [] as string[][],
    prompts: [] as string[],
    /** Each submitted prompt's origin, as the hooks beneath saw it. */
    origins: [] as unknown[],
    /** The board-answer inbox: one text per held message; a drain claims them all. */
    inbox: [] as string[],
    /** Every inbox drain call's arguments. */
    inboxDrains: [] as Record<string, unknown>[],
    recvAnswers: [] as { result: unknown; text: string }[],
    /** The chunks the next model request streams, and its result. */
    chunks: [] as unknown[],
    step: { answer: '', toolUses: [] as { name: string; input: unknown }[] },
    toolResult: { result: 'ok', text: 'ok' } as Record<string, unknown>,
    logs: [] as string[],
    /** What the permission check beneath decides. */
    check: 'allow' as 'allow' | 'ask',
  }
  const text = (t: string, isError = false) => ({ value: { content: [{ type: 'text', text: t }], isError } })
  on('mcp.connect', ($, e) => ({ value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }))
  on('mcp.call', ($, e) => {
    if (e.tool === INTAKE_TOOL) {
      w.intakes.push(JSON.parse(JSON.stringify(e.args)))
      if (w.intake === 'throw') throw new Error('server gone')
      return w.intake === 'error' ? text('unknown session_key', true) : text('ok: 1 of 1 events applied')
    }
    if (e.tool === INBOX_DRAIN_TOOL) {
      w.inboxDrains.push({ ...e.args })
      return text(JSON.stringify({ messages: w.inbox.splice(0).map((t, i) => ({ id: `m${i}`, sent_at: 1, text: t, parts: [t], chips: [] })) }))
    }
    if (e.tool !== 'config_get') return text(`no ${e.tool} here`, true)
    if (e.args.key === KNOB.dashboard && opts.dashboard === 'unreadable') return text('ws-mcp is unhealthy', true)
    const value = e.args.key === KNOB.dashboard ? (opts.dashboard ?? null) : e.args.key === KNOB.drain ? 'direct' : null
    return text(JSON.stringify({ key: e.args.key, value, scope: value === null ? 'unset' : 'project' }))
  })
  on('store.get', () => ({ value: undefined }))
  on('store.set', () => ({ value: undefined }))
  on('store.delete', () => ({ value: undefined }))
  on('ui.log', ($, e) => (w.logs.push(e.text), { value: undefined }))
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.messages', () => ({ value: [] as never }))
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('session.append', ($, e, next) => next(e))
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('turn.step', async function* ($, e) {
    for (const chunk of w.chunks) yield chunk as never
    return { turnId: e.turnId, index: e.index, answer: w.step.answer, toolUses: w.step.toolUses, stopReason: 'end_turn', usage: null } as never
  })
  on('tool.check', () => ({ decision: w.check, reason: 'the world decides' }) as never)
  on('agent.spawn', () => ({ model: 'haiku', agentId: 'a77' }) as never)
  on('classic.Notification', () => ({}) as never)
  on('process.run', () => ({ value: { exitCode: 0, stdout: 'Python 3.12.0\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }))
  on('process.spawn', async function* ($, e) {
    w.spawns += 1
    w.argvs.push([...e.argv])
    const [code, kinds] = await new Promise<[number, string[] | undefined]>((resolve) => w.exits.push((c, k) => resolve([c, k])))
    if (code === 0) yield { stream: 'stdout', text: JSON.stringify({ timed_out: false, kinds: kinds ?? ['mail'] }) } as never
    return { value: { code, signal: null } }
  })
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  on('tool.call', { tool: /__mailbox_recv$/ }, () => (w.recvAnswers.shift() ?? { result: undefined, text: 'null\n' }) as never)
  on('tool.call', { tool: /__mailbox_lookup_peers$/ }, () => ({ result: undefined, text: '{"self":{"reply_id":"id:r"},"peers":[]}' }) as never)
  on('tool.call', () => w.toolResult as never)
  on('prompt.submit', ($, e) => {
    w.prompts.push(e.text)
    w.origins.push(e.origin)
    return { text: e.text }
  })
  return w
}

type World = ReturnType<typeof world>

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
  await $.tool.call({ tool: `mcp__${w.plugin}__mailbox_watch`, session_key: KEY } as never)
  await w.clock.settle()
}

/** Lets the feed's flush timer fire and its call settle; the mailbox wait child stays pending. */
const flush = (w: World) => w.clock.advance(200)

const sent = (w: World) => w.intakes.flatMap((c) => c.events)
const kinds = (w: World) => sent(w).map((e) => e.kind)

async function step($: Engine, w: World, agentId?: string) {
  const out: unknown[] = []
  for await (const chunk of $.turn.step({ turnId: 't', index: 0, model: 'm', messageCount: 1, ...(agentId ? { agentId } : {}) } as never)) out.push(chunk)
  return out
}

test('knob on: the lead\'s turn reaches the intake tool under the bound key, by the adapter name', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.prompt.submit({ text: 'fix it', origin: { kind: 'composer' } } as never)
  await $.turn.start({ text: 'fix it', turnId: 't' })
  w.chunks = [
    { kind: 'text', index: 0, text: 'Looking' },
    { kind: 'tool', index: 1, id: 'u1', name: 'Bash' },
    { kind: 'input', index: 1, json: '{"command":"ls"}' },
    { kind: 'stop', stopReason: 'tool_use', usage: null },
  ]
  w.step = { answer: 'Looking', toolUses: [{ name: 'Bash', input: { command: 'ls' } }] }
  const chunks = await step($, w)
  // Observe-only: the stream reaches the loop unchanged.
  expect(chunks).toEqual(w.chunks)
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  await $.turn.complete({ answer: 'Looking', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await flush(w)
  expect(w.intakes.length).toBeGreaterThan(0)
  expect(w.intakes.every((c) => c.session_key === KEY && c.adapter === ADAPTER)).toBe(true)
  expect(kinds(w)[0]).toBe('reset')
  expect(sent(w).find((e) => e.kind === 'user')).toEqual({ kind: 'user', text: 'fix it', origin: 'terminal', at: expect.any(Number) })
  expect(sent(w).find((e) => e.kind === 'turn_start')?.origin).toBe('terminal')
  expect(sent(w).filter((e) => e.kind === 'reply' && e.final)).toEqual([{ kind: 'reply', text: 'Looking', final: true, at: expect.any(Number) }])
  const lastLead = sent(w).filter((e) => e.kind === 'agent' && e.agentId === undefined).at(-1)
  // The model's own bind call is the first batch; this turn's step the second.
  expect((lastLead?.batches as unknown[]).at(-1)).toEqual({
    epoch: expect.any(String),
    seq: 2,
    toolNames: ['Bash'],
    callCount: 1,
    failed: 0,
    startedAt: expect.any(Number),
    endedAt: expect.any(Number),
  })
  expect(kinds(w).at(-1)).toBe('turn_end')
})

for (const knob of [false, null, 'unreadable'] as const) {
  test(`knob ${String(knob)}: nothing reaches the intake tool`, async ($, on) => {
    const w = world(on, { dashboard: knob })
    await start($, w)
    await $.turn.start({ text: 'x', turnId: 't' })
    w.chunks = [{ kind: 'text', index: 0, text: 'hi' }]
    expect(await step($, w)).toEqual(w.chunks)
    await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
    await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
    await flush(w)
    expect(w.intakes.length).toBe(0)
  })
}

test('a subagent is keyed by its native id and labelled from its spawn; its own key travels as sessionKey', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.agent.spawn({ tool_use_id: 'u0', prompt: 'go', description: 'Probe files', subagentType: 'Explore' } as never)
  await $.turn.start({ text: 'go', turnId: 's', agentId: 'a77' })
  await $.tool.call({ tool: `mcp__plugin_${w.plugin}_${w.plugin}__workflow_manual`, tool_use_id: 'v1', session_key: 'dk-9', agentId: 'a77' } as never)
  await $.turn.complete({ answer: 'r', durationMs: 1, isAborted: false, turnId: 's', reason: 'answer', agentId: 'a77' } as never)
  await flush(w)
  const sub = sent(w).filter((e) => e.kind === 'agent' && e.agentId === 'a77').at(-1)
  expect(sub).toEqual({
    kind: 'agent',
    agentId: 'a77',
    label: 'Probe files',
    sessionKey: 'dk-9',
    caption: 'Explore',
    status: 'done',
    batches: [expect.objectContaining({ toolNames: [`mcp__plugin_${w.plugin}_${w.plugin}__workflow_manual`], callCount: 1, failed: 0 })],
  })
  // Its call's arguments never carry the key into a preview.
  expect(JSON.stringify(sent(w)).split('dk-9').length - 1).toBe(JSON.stringify(sent(w)).split('"sessionKey":"dk-9"').length - 1)
})

test('a whiteboard-labelled prompt of the plugin\'s own enters as a whiteboard message, label stripped', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.prompt.submit({ text: labelWhiteboard(w.plugin, 'pick B'), origin: { kind: 'plugin', name: w.plugin } } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'pick B', origin: 'whiteboard', at: expect.any(Number) }])
})

test('a notification after a person ask marks the wait; the call\'s end clears it', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  // The kit's check carries no approver, so the ask is raised as a person's.
  w.check = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf x' }, tool_use_id: 'u5', approver: 'person' } as never)
  await $.classic.Notification({ message: 'Claude needs your permission', notification_type: 'permission_prompt' } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'permission')).toEqual([{ kind: 'permission', tool: 'Bash', args: 'command: rm -rf x' }])
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u5', command: 'rm -rf x' } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'permission').at(-1)).toEqual({ kind: 'permission', clear: true })
})

for (const failure of ['throw', 'error'] as const) {
  test(`an intake that fails (${failure}) leaves the mailbox push, tool results and the stream as they were`, async ($, on) => {
    const w = world(on, { dashboard: true, intake: failure })
    await start($, w)
    w.toolResult = { result: 'listing', text: 'listing' }
    await $.turn.start({ text: 'x', turnId: 't' })
    w.chunks = [{ kind: 'text', index: 0, text: 'hi' }]
    expect(await step($, w)).toEqual(w.chunks)
    const res = await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
    expect(res.text).toBe('listing')
    await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
    await flush(w)
    expect(w.intakes.length).toBeGreaterThan(0)
    // Mail arrives while idle: the push drains it and submits the batch.
    w.recvAnswers.push({ result: undefined, text: '[{"reply_to":"id:peer","content":"hello","sent_at":"t1"}]' })
    w.exits[w.exits.length - 1]!(0)
    await w.clock.settle()
    expect(w.prompts.some((p) => p.includes('hello'))).toBe(true)
    // The feed keeps retrying behind its wait, each retry a resync.
    const before = w.intakes.length
    await w.clock.advance(2500)
    expect(w.intakes.length).toBeGreaterThan(before)
    expect(w.intakes.at(-1)!.events[0]?.kind).toBe('reset')
  })
}

const BOARD_FLAG = '--board-inbox'

test('knob on: an idle board wake drains the inbox and submits the message as the user\'s, labelled, entering the ring as whiteboard', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  expect(w.argvs.at(-1)).toContain(BOARD_FLAG)
  w.inbox.push('pick B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.settle()
  expect(w.inboxDrains).toEqual([{ session_key: KEY, format: 'json' }])
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'pick B')])
  expect(w.origins).toEqual([expect.objectContaining({ kind: 'plugin', name: w.plugin, asUser: true })])
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'pick B', origin: 'whiteboard', at: expect.any(Number) }])
  // Re-armed with the flag; no mail was drained for the board wake.
  expect(w.spawns).toBe(2)
  expect(w.argvs.at(-1)).toContain(BOARD_FLAG)
})

test('knob on: a mid-turn wake holds the message until the turn ends, then submits it and re-arms with the flag', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  w.inbox.push('pick B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.advance(50)
  expect(w.inboxDrains).toEqual([])
  expect(w.argvs.at(-1)).not.toContain(BOARD_FLAG)
  // A tool boundary injects nothing.
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  expect(w.inboxDrains).toEqual([])
  expect(w.prompts).toEqual([])
  await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.advance(50)
  expect(w.inboxDrains.length).toBe(1)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'pick B')])
  expect(w.argvs.at(-1)).toContain(BOARD_FLAG)
})

for (const knob of [false, 'unreadable'] as const) {
  test(`knob ${String(knob)}: the wait carries no board flag and a turn end drains no inbox`, async ($, on) => {
    const w = world(on, { dashboard: knob })
    await start($, w)
    expect(w.argvs.at(-1)).not.toContain(BOARD_FLAG)
    await $.turn.start({ text: 'x', turnId: 't' })
    await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
    await w.clock.advance(50)
    expect(w.inboxDrains).toEqual([])
  })
}

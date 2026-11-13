import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { acceptedKinds, ADAPTER, FLUSH_INTERVAL_MS, INBOX_DRAIN_TOOL, INTAKE_TOOL, labelWhiteboard, statusFigures, type DashboardEvent } from './dashboard'
import { CONTINUATION_WINDOW_MS, SETTLE_MS } from './compaction'
import { intakeResult, KNOB } from './register'
import { resumeText } from './timing'
import { STEER_DRAIN_WAIT_MS } from './watcher'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the dashboard feed: the plugin runs inside the
// engine's own host, and the hooks below stand for the engine and the MCP
// server beneath it. The same file is mirrored into every package that ships
// the mod and runs identically in each: the feed runs while its knob reads
// true through the world's config_get answers, and its key here is the one
// the mailbox push is bound to. The package name only spells tool names.

const KEY = 'k1'

/** How the intake answers: applied (`accepts` names `status` when the world's flag says so), an error, gone, or off. */
type Intake = 'ok' | 'error' | 'throw' | 'off'

const NOW = 1_800_000_000_000

function world(on: On, opts: { dashboard?: unknown; intake?: Intake } = {}) {
  const clock = mock.clock(on, { now: NOW })
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    /** Every intake call's arguments, in order. */
    intakes: [] as { session_key: string; adapter: string; events: DashboardEvent[] }[],
    intake: opts.intake ?? ('ok' as Intake),
    /** Whether an applied intake's result names `status` on its accepts line, as a ws-mcp that takes it does. */
    acceptsStatus: false,
    /** Whether an applied intake's result names `compact` on its accepts line. */
    acceptsCompact: false,
    acceptsWake: false,
    acceptsTaskCounts: false,
    /** Whether an applied intake's result names `commands` and `command` on its accepts line. */
    acceptsCommands: false,
    /** What `$.command.list` answers. */
    commands: [
      { name: 'compact', description: 'Clear the history but keep a summary', source: 'builtin' },
      { name: 'context', description: 'Show context usage', source: 'builtin' },
      { name: 'config', description: 'Open the settings panel', source: 'builtin' },
      { name: 'review', description: 'Review a branch', source: 'plugin', plugin: 'tools' },
    ] as Record<string, unknown>[],
    /** How many times the command list was read. */
    commandLists: 0,
    /** Every slash command run beneath, as its typed line; `/compact` included. */
    commandRuns: [] as string[],
    /** What a run of a listed command answers: its output text; unnamed ones answer no text. */
    commandAnswers: new Map<string, { text?: string }>(),
    /** The prompts submitted and the commands run, in order, as `prompt <text>` and `run <line>`. */
    order: [] as string[],
    /** How the compaction beneath answers: it lands, is vetoed, or throws. */
    compact: 'land' as 'land' | 'skip' | 'throw',
    /** While set, the compaction beneath waits on it, so a test can look at the feed mid-compaction. */
    compactGate: undefined as Promise<void> | undefined,
    /** What `$.session.usage` answers. */
    usage: {
      startedAt: 0,
      context: { tokens: 50_000 as number | undefined, window: 200_000 },
      rateLimits: [{ kind: 'five_hour', percentUsed: 30, resetsAt: new Date(NOW + 2 * 3600_000).toISOString() }],
      cost: { usd: 1.5 },
    },
    /** Ends the latest wait child: its exit code and, for an arrival, the kinds its JSON names (mail when unset). */
    exits: [] as ((code: number, kinds?: string[]) => void)[],
    spawns: 0,
    /** Each wait child's argv. */
    argvs: [] as string[][],
    prompts: [] as string[],
    /** Each submitted prompt's origin, as the hooks beneath saw it. */
    origins: [] as unknown[],
    /** The board-answer inbox: one text per held message, or a command entry; a drain claims them all. */
    inbox: [] as (string | { name: string; args: string })[],
    /** Held messages promoted to steering: a steer-only drain claims these alone, a full drain claims them too. */
    steered: [] as string[],
    /** Every full inbox drain call's arguments. */
    inboxDrains: [] as Record<string, unknown>[],
    /** Every steer-only inbox drain call's arguments. */
    steerDrains: [] as Record<string, unknown>[],
    /** User rows the mod appended, as their text. */
    rows: [] as string[],
    /** How many of the next user rows the host refuses. */
    refuseRows: 0,
    /** How many of the next prompts the plugin submits the host drops. */
    dropPrompts: 0,
    /** Whether a steer-only inbox drain never answers, as a hung server's. */
    hangSteer: false,
    /** Held interrupt entries, as their creation times: the interrupt-only drain claims these alone, a full drain drops them. */
    interrupts: [] as number[],
    /** Every interrupt-only inbox drain call's arguments. */
    interruptDrains: [] as Record<string, unknown>[],
    /** The turn id of every $.turn.abort the plugin asked for. */
    aborts: [] as string[],
    /** Whether the abort beneath rejects, as when the turn already ended. */
    refuseAbort: false,
    recvAnswers: [] as { result: unknown; text: string }[],
    /** The chunks the next model request streams, and its result. */
    chunks: [] as unknown[],
    /** Awaited before the model request streams its chunk at this position, so real time can pass between chunks. */
    chunkWaits: new Map<number, () => Promise<void>>(),
    step: { answer: '', toolUses: [] as { name: string; input: unknown }[], usage: null as Record<string, unknown> | null },
    toolResult: { result: 'ok', text: 'ok' } as Record<string, unknown>,
    logs: [] as string[],
    /** What the permission check beneath decides. */
    check: 'allow' as 'allow' | 'ask',
    /** What `$.agent.list` answers: the session's subagents and teammates. */
    agentList: [] as Record<string, unknown>[],
    /** The test's engine, bound by the test: the `/compact` command raises its compaction through it. */
    engine: undefined as Engine | undefined,
  }
  const text = (t: string, isError = false) => ({ value: { content: [{ type: 'text', text: t }], isError } })
  on('mcp.connect', ($, e) => ({ value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }))
  on('mcp.call', ($, e) => {
    if (e.tool === INTAKE_TOOL) {
      w.intakes.push(JSON.parse(JSON.stringify(e.args)))
      if (w.intake === 'throw') throw new Error('server gone')
      if (w.intake === 'error') return text('unknown session_key', true)
      if (w.intake === 'off') return text('dashboard off: nothing recorded\n')
      const accepts = [...(w.acceptsStatus ? ['status'] : []), ...(w.acceptsCompact ? ['compact'] : []), ...(w.acceptsWake ? ['wake'] : []), ...(w.acceptsTaskCounts ? ['task_counts'] : []), ...(w.acceptsCommands ? ['commands', 'command'] : [])]
      return text(`ok: 1 of 1 events applied\n${accepts.length > 0 ? `accepts: ${accepts.join(', ')}\n` : ''}`)
    }
    if (e.tool === INBOX_DRAIN_TOOL && e.args.interrupt_only === true) {
      w.interruptDrains.push({ ...e.args })
      const claimed = w.interrupts.splice(0)
      return text(JSON.stringify({ messages: claimed.map((t, i) => ({ id: `i${i}`, sent_at: t, text: '', parts: [], chips: [], interrupt: true })) }))
    }
    if (e.tool === INBOX_DRAIN_TOOL) {
      const steerOnly = e.args.steer_only === true
      if (!steerOnly) w.interrupts.splice(0)
      ;(steerOnly ? w.steerDrains : w.inboxDrains).push({ ...e.args })
      if (steerOnly && w.hangSteer) return new Promise<never>(() => {})
      const claimed: (string | { name: string; args: string })[] = steerOnly ? w.steered.splice(0) : [...w.inbox.splice(0), ...w.steered.splice(0)]
      return text(
        JSON.stringify({
          messages: claimed.map((t, i) =>
            typeof t === 'string'
              ? { id: `m${i}`, sent_at: 1, text: t, parts: [t], chips: [] }
              : { id: `m${i}`, sent_at: 1, text: t.args === '' ? `/${t.name}` : `/${t.name} ${t.args}`, parts: [], chips: [], command: t },
          ),
        }),
      )
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
  on('session.usage', () => ({ value: structuredClone(w.usage) as never }))
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('session.messages', () => ({ value: [] as never }))
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('session.append', ($, e, next) => {
    if (e.message.type !== 'user') return next(e)
    if (w.refuseRows > 0) {
      w.refuseRows -= 1
      return { deny: 'refused by the test' }
    }
    w.rows.push(e.message.content.map((b) => (b.type === 'text' ? b.text : '')).join(''))
    return next(e)
  })
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.abort', ($, e) => {
    w.aborts.push(e.turnId)
    if (w.refuseAbort) throw new Error(`turn ${e.turnId} is not the running turn`)
    return { value: undefined }
  })
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('turn.step', async function* ($, e) {
    for (const [i, chunk] of w.chunks.entries()) {
      await w.chunkWaits.get(i)?.()
      yield chunk as never
    }
    return { turnId: e.turnId, index: e.index, answer: w.step.answer, toolUses: w.step.toolUses, stopReason: 'end_turn', usage: w.step.usage } as never
  })
  on('session.compact', async () => {
    if (w.compactGate) await w.compactGate
    if (w.compact === 'throw') throw new Error('summarizer failed')
    if (w.compact === 'skip') return { skip: 'vetoed by the test' }
    return { messages: [{ role: 'user', text: 'summary', toolUses: [] }] } as never
  })
  // `/compact` compacts as a typed one does: a manual compaction through every plugin's session.compact hook.
  // A name the list does not hold is left to the engine beneath, which answers none, so the run rejects.
  on('command.run', async ($, e, next) => {
    const line = e.args === '' ? `/${e.command}` : `/${e.command} ${e.args}`
    w.commandRuns.push(line)
    w.order.push(`run ${line}`)
    if (!w.commands.some((c) => c.name === e.command)) return next(e)
    const answer = w.commandAnswers.get(e.command)
    if (e.command !== 'compact') return answer ?? {}
    await w.engine!.session.compact({ trigger: 'manual', messages: [{ role: 'user', text: 'long', toolUses: [] }] } as never)
    return answer ?? {}
  })
  on('command.list', () => {
    w.commandLists += 1
    return { value: structuredClone(w.commands) as never }
  })
  on('tool.check', () => ({ decision: w.check, reason: 'the world decides' }) as never)
  on('agent.spawn', () => ({ model: 'haiku', agentId: 'a77' }) as never)
  on('agent.list', () => ({ value: structuredClone(w.agentList) as never }))
  on('classic.Notification', () => ({}) as never)
  on('process.run', () => ({ value: { exitCode: 0, stdout: 'Python 3.12.0\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }))
  on('process.spawn', async function* ($, e) {
    // Exercise dashboard delivery through the older-daemon one-shot fallback.
    if (e.argv.includes('--stream')) return { value: { code: 2, signal: null } }
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
    if (w.dropPrompts > 0) {
      w.dropPrompts -= 1
      return { drop: 'dropped by the test' }
    }
    w.prompts.push(e.text)
    w.order.push(`prompt ${e.text}`)
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
const flush = (w: World) => w.clock.advance(FLUSH_INTERVAL_MS + 100)

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
  w.step = {
    answer: 'Looking',
    toolUses: [{ name: 'Bash', input: { command: 'ls' } }],
    usage: { input_tokens: 12, output_tokens: 340, cache_read_input_tokens: 45000, cache_creation_input_tokens: 1200, model: 'm' },
  }
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
    // The step result's usage, the call's own clocks, and its capped argument preview ride on the finished batch.
    calls: [{ name: 'Bash', startedAt: expect.any(Number), endedAt: expect.any(Number), failed: false, argsPreview: 'command: ls' }],
    usage: { input: 12, output: 340, cacheRead: 45000, cacheWrite: 1200 },
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
    // What the spawn beneath resolved it to run on.
    model: 'haiku',
    status: 'done',
    batches: [expect.objectContaining({ toolNames: [`mcp__plugin_${w.plugin}_${w.plugin}__workflow_manual`], callCount: 1, failed: 0 })],
  })
  // Its call's arguments never carry the key into a preview.
  expect(JSON.stringify(sent(w)).split('dk-9').length - 1).toBe(JSON.stringify(sent(w)).split('"sessionKey":"dk-9"').length - 1)
})

test('a subagent\'s request gives its event the model and effort it names, its context size, and its output rate; the lead\'s carries none', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.agent.spawn({ tool_use_id: 'u0', prompt: 'go', description: 'Probe files', subagentType: 'ws:effort-high' } as never)
  await flush(w)
  // Before its first request: the spawn's resolved model and the effort its type carries.
  expect(sent(w).filter((e) => e.kind === 'agent' && e.agentId === 'a77').at(-1)).toMatchObject({ model: 'haiku', effort: 'high' })
  await $.turn.start({ text: 'go', turnId: 's', agentId: 'a77' })
  w.chunks = [
    { kind: 'text', index: 0, text: 'Reading' },
    { kind: 'text', index: 0, text: ' the files' },
    { kind: 'stop', stopReason: 'end_turn', usage: null },
  ]
  // Block 0's two chunks arrive at least 300 ms apart in real time: the feed times blocks by arrival.
  w.chunkWaits.set(1, () => new Promise((resolve) => setTimeout(resolve, 300)))
  w.step = { answer: 'Reading the files', toolUses: [], usage: { input_tokens: 8, output_tokens: 90, cache_read_input_tokens: 20_000, cache_creation_input_tokens: 500, model: 'claude-opus-y' } }
  for await (const _ of $.turn.step({ turnId: 's', index: 0, model: 'claude-opus-y', effort: 'max', messageCount: 1, agentId: 'a77' } as never)) void _
  await flush(w)
  const sub = sent(w).filter((e) => e.kind === 'agent' && e.agentId === 'a77').at(-1)
  expect(sub).toMatchObject({ model: 'claude-opus-y', effort: 'max', contextTokens: 20_508 })
  expect(typeof sub?.outputTps).toBe('number')
  // 90 tokens over a span of at least 300 ms.
  expect(sub!.outputTps as number).toBeGreaterThan(0)
  expect(sub!.outputTps as number).toBeLessThanOrEqual(300)
  // The lead's own request names a model and effort too, and its event still carries none of the fields.
  w.chunkWaits.clear()
  for await (const _ of $.turn.step({ turnId: 't', index: 0, model: 'claude-opus-y', effort: 'high', messageCount: 1 } as never)) void _
  await flush(w)
  for (const lead of sent(w).filter((e) => e.kind === 'agent' && e.agentId === undefined)) {
    expect(['model', 'effort', 'contextTokens', 'outputTps'].filter((k) => k in lead)).toEqual([])
  }
})

test('a turn and a tool call under an agent id no spawn announced send no agent event', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  // A loop the host runs past the lead's turn end with no agent.spawn.
  await $.turn.start({ text: '', turnId: 'g', agentId: 'ad1ac9' })
  await $.tool.call({ tool: 'Read', tool_use_id: 'g1', file_path: '/x', agentId: 'ad1ac9' } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'agent' && e.agentId !== undefined)).toEqual([])
})

test('a subagent spawned before the feed started shows from the agent list: seeded at start, and looked up at its first event', async ($, on) => {
  const w = world(on, { dashboard: true })
  // Spawned before a reload: no agent.spawn reaches this feed.
  w.agentList = [{ id: 'a5', description: 'Earlier task', type: 'Explore', status: 'running' }]
  await start($, w)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'agent' && e.agentId === 'a5').at(-1)).toMatchObject({ label: 'Earlier task', caption: 'Explore', status: 'running' })
  // Another one the list names only after the start: looked up at its first event.
  w.agentList = [...w.agentList, { id: 'a6', description: 'Later task', type: 'general-purpose', status: 'running', parentId: 'a5' }]
  await $.turn.start({ text: '', turnId: 's6', agentId: 'a6' })
  await $.tool.call({ tool: 'Read', tool_use_id: 'r6', file_path: '/x', agentId: 'a6' } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'agent' && e.agentId === 'a6').at(-1)).toMatchObject({
    label: 'Later task',
    parentId: 'a5',
    batches: [expect.objectContaining({ toolNames: ['Read'], callCount: 1 })],
  })
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
  // A tool boundary injects nothing: its steer-only drain claims no unpromoted message.
  const steerDrainsBefore = w.steerDrains.length
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  expect(w.steerDrains.length).toBe(steerDrainsBefore + 1)
  expect(w.inboxDrains).toEqual([])
  expect(w.inbox).toEqual(['pick B'])
  expect(w.rows).toEqual([])
  expect(w.prompts).toEqual([])
  await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.advance(50)
  expect(w.inboxDrains.length).toBe(1)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'pick B')])
  expect(w.argvs.at(-1)).toContain(BOARD_FLAG)
})

test('knob on: a message promoted to steering is appended at the next main-thread tool boundary and enters the ring as whiteboard; the unpromoted one waits for the turn end', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  w.inbox.push('later')
  w.steered.push('now please')
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  expect(w.steerDrains.at(-1)).toEqual({ session_key: KEY, steer_only: true, format: 'json' })
  expect(w.rows).toEqual([labelWhiteboard(w.plugin, 'now please')])
  expect(w.inboxDrains).toEqual([])
  expect(w.prompts).toEqual([])
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'now please', origin: 'whiteboard', at: expect.any(Number) }])
  // A subagent's tool call is no boundary of the lead's turn.
  w.steered.push('not yet')
  const before = w.steerDrains.length
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u2', command: 'ls', agentId: 'a77' } as never)
  expect(w.steerDrains.length).toBe(before)
  w.steered.splice(0)
  // The request after the result reads the steer row.
  await step($, w)
  await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.advance(50)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'later')])
  expect(w.rows.length).toBe(1)
})

test('knob on: a refused steer row submits the claimed message at the turn end, ahead of the held one, entering the ring once', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  w.inbox.push('later')
  w.steered.push('now please')
  w.refuseRows = 1
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  expect(w.steered).toEqual([])
  expect(w.rows).toEqual([])
  expect(w.prompts).toEqual([])
  await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.advance(50)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'now please\n\nlater')])
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'now please\n\nlater', origin: 'whiteboard', at: expect.any(Number) }])
})

test('knob on: a hung steer drain holds a main-thread tool result only STEER_DRAIN_WAIT_MS', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  w.hangSteer = true
  const before = w.steerDrains.length
  let returned = false
  const call = $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never).then((r) => ((returned = true), r))
  await w.clock.advance(STEER_DRAIN_WAIT_MS - 100)
  expect(returned).toBe(false)
  await w.clock.advance(200)
  expect(returned).toBe(true)
  expect(await call).toEqual(expect.objectContaining({ text: 'ok' }))
  expect(w.steerDrains.length).toBe(before + 1)
})

test('knob on: a dropped board submit falls back to a row that still enters the ring as whiteboard', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  w.inbox.push('pick B')
  w.dropPrompts = 1
  w.exits.at(-1)!(0, ['board'])
  await w.clock.settle()
  expect(w.prompts).toEqual([])
  expect(w.rows).toEqual([labelWhiteboard(w.plugin, 'pick B')])
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'pick B', origin: 'whiteboard', at: expect.any(Number) }])
})

test('knob on: a steer row the turn ends without reading is submitted again at the turn end, entering the ring once', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  w.steered.push('now please')
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  expect(w.rows).toEqual([labelWhiteboard(w.plugin, 'now please')])
  // The person interrupted: no request follows the result.
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: true, turnId: 't', reason: 'aborted' } as never)
  await w.clock.advance(50)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'now please')])
  expect(w.rows.length).toBe(1)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([{ kind: 'user', text: 'now please', origin: 'whiteboard', at: expect.any(Number) }])
})

const INTERRUPT_FLAG = '--board-interrupt'
/** A dashboard interrupt the user made after the running turn started, on this host's wall clock. */
const afterStart = () => Date.now() + 60_000
const mailAnswer = (content: string) => ({ result: undefined, text: JSON.stringify([{ reply_to: 'id:peer', content, sent_at: 't1' }]) })

test('knob on: every wait carries the interrupt flag, the flagless re-arm after a held message included', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  expect(w.argvs.at(-1)).toEqual(expect.arrayContaining([BOARD_FLAG, INTERRUPT_FLAG]))
  await $.turn.start({ text: 'x', turnId: 't1' })
  w.inbox.push('pick B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.advance(50)
  expect(w.argvs.at(-1)).not.toContain(BOARD_FLAG)
  expect(w.argvs.at(-1)).toContain(INTERRUPT_FLAG)
})

test('knob on: an interrupt wake during the lead\'s turn is claimed by the interrupt-only drain and aborts that turn by the id its turn.start carried; a subagent\'s turn start is not it', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't1' })
  await $.turn.start({ text: 'look around', turnId: 's1', agentId: 'a77' } as never)
  const steerDrainsBefore = w.steerDrains.length
  w.inbox.push('pick B')
  w.interrupts.push(afterStart())
  w.exits.at(-1)!(0, ['interrupt'])
  await w.clock.advance(50)
  expect(w.interruptDrains).toEqual([{ session_key: KEY, interrupt_only: true, format: 'json' }])
  expect(w.aborts).toEqual(['t1'])
  // Never read as a board message: nothing else is drained or submitted.
  expect(w.inboxDrains).toEqual([])
  expect(w.steerDrains.length).toBe(steerDrainsBefore)
  expect(w.inbox).toEqual(['pick B'])
  expect(w.prompts).toEqual([])
  // Re-armed with the flag.
  expect(w.spawns).toBe(2)
  expect(w.argvs.at(-1)).toContain(INTERRUPT_FLAG)
})

test('knob on: an interrupt claimed while idle, or made before the lead\'s turn started, is dropped without an abort', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  w.interrupts.push(Date.now())
  w.exits.at(-1)!(0, ['interrupt'])
  await w.clock.advance(50)
  expect(w.interruptDrains.length).toBe(1)
  expect(w.aborts).toEqual([])
  // A click that landed as the last turn ended, read once the next one runs.
  w.interrupts.push(Date.now() - 1_000)
  await $.turn.start({ text: 'x', turnId: 't1' })
  w.exits.at(-1)!(0, ['interrupt'])
  await w.clock.advance(50)
  expect(w.interruptDrains.length).toBe(2)
  expect(w.interrupts).toEqual([])
  expect(w.aborts).toEqual([])
  expect(w.spawns).toBe(3)
})

test('knob on: a refused abort is ignored and the wait re-arms', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't1' })
  w.refuseAbort = true
  w.interrupts.push(afterStart())
  w.exits.at(-1)!(0, ['interrupt'])
  await w.clock.advance(50)
  expect(w.aborts).toEqual(['t1'])
  expect(w.logs.some((l) => l.includes('aborting the running turn was refused'))).toBe(true)
  expect(w.spawns).toBe(2)
  // The loop goes on: mail still arrives.
  w.recvAnswers.push(mailAnswer('hello'))
  w.exits.at(-1)!(0, ['mail'])
  await w.clock.advance(50)
  expect(w.spawns).toBe(3)
})

test('knob on: an interrupted turn end appends queued mail and a carried steered message as rows, a message sent after the click still submits, and the next turn start ends the hold', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't1' })
  // A steered message whose row the host refused: carried to the turn end.
  w.steered.push('now please')
  w.refuseRows = 1
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  expect(w.rows).toEqual([])
  // Mail queued for the turn.
  w.recvAnswers.push(mailAnswer('hello'))
  w.exits.at(-1)!(0, ['mail'])
  await w.clock.advance(50)
  w.interrupts.push(afterStart())
  w.exits.at(-1)!(0, ['interrupt'])
  await w.clock.advance(50)
  expect(w.aborts).toEqual(['t1'])
  w.inbox.push('after the click')
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: true, turnId: 't1', reason: 'aborted' } as never)
  await w.clock.advance(50)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'after the click')])
  expect(w.rows).toEqual([labelWhiteboard(w.plugin, 'now please'), expect.stringContaining('hello')])
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'user').map((e) => (e as { text: string }).text)).toEqual(['now please', 'after the click'])
  // The next main-thread turn: its leftovers submit as before.
  await $.turn.start({ text: 'after the click', turnId: 't2' })
  w.recvAnswers.push(mailAnswer('again'))
  w.exits.at(-1)!(0, ['mail'])
  await w.clock.advance(50)
  await $.turn.complete({ answer: 'ok', durationMs: 1, isAborted: false, turnId: 't2', reason: 'answer' } as never)
  await w.clock.advance(50)
  expect(w.prompts.length).toBe(2)
  expect(w.prompts[1]).toContain('again')
  expect(w.rows.length).toBe(2)
})

for (const knob of [false, 'unreadable'] as const) {
  test(`knob ${String(knob)}: the wait carries no board flag and a turn end drains no inbox`, async ($, on) => {
    const w = world(on, { dashboard: knob })
    await start($, w)
    expect(w.argvs.at(-1)).not.toContain(BOARD_FLAG)
    expect(w.argvs.at(-1)).not.toContain(INTERRUPT_FLAG)
    await $.turn.start({ text: 'x', turnId: 't' })
    await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
    await w.clock.advance(50)
    expect(w.inboxDrains).toEqual([])
  })
}

// ---- Wake and task-count hook wiring --------------------------------------

test('non-person prompts become wakes; person messages and dropped prompts do not', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsWake = true
  await start($, w)
  await flush(w)
  const note = '<task-notification><task-id>a77</task-id><task-type>local_agent</task-type><status>completed</status><summary>Agent finished</summary>Full report</task-notification>'
  await $.prompt.submit({ text: note, origin: { kind: 'task-notification' } } as never)
  await $.prompt.submit({ text: note, origin: { kind: 'composer' } } as never)
  w.dropPrompts = 1
  await $.prompt.submit({ text: 'later', origin: { kind: 'scheduled-trigger' } } as never)
  await $.prompt.submit({ text: 'check now', origin: { kind: 'scheduled-trigger' } } as never)
  await $.turn.start({ text: 'check now', turnId: 't' })
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'wake').map((e) => [e.cause, e.text])).toEqual([['subagent_report', note], ['scheduled', 'check now']])
  expect(sent(w).filter((e) => e.kind === 'user').map((e) => e.text)).toEqual([note])
})

test('tool results expose shell and monitor identities, terminal prompts finish counts, failed calls clear them', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  w.acceptsTaskCounts = true
  w.acceptsWake = true
  await start($, w)
  await flush(w)
  w.toolResult = { result: { backgroundTaskId: 'b1' }, text: 'running' }
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', run_in_background: true, command: 'build' } as never)
  w.toolResult = { result: { taskId: 'm1', timeoutMs: 1000 }, text: 'monitoring' }
  await $.tool.call({ tool: 'Monitor', tool_use_id: 'u2', command: 'watch' } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'status').at(-1)).toMatchObject({ background: 1, monitors: 1 })
  await $.prompt.submit({ text: '<task-notification><task-id>b1</task-id><status>completed</status><summary>Background command exited</summary></task-notification>', origin: { kind: 'task-notification' } } as never)
  w.toolResult = { result: 'stopped', text: 'stopped' }
  await $.tool.call({ tool: 'TaskStop', tool_use_id: 'stop', task_id: 'm1' } as never)
  w.toolResult = { result: 'launch failed', isError: true, text: 'launch failed' }
  await $.tool.call({ tool: 'Bash', tool_use_id: 'failed', run_in_background: true, command: 'bad' } as never)
  await flush(w)
  expect(sent(w).filter((e) => e.kind === 'status').at(-1)).toMatchObject({ background: 0, monitors: 0 })
  expect(sent(w).filter((e) => e.kind === 'wake').at(-1)?.cause).toBe('background_exit')
})

test('the mod\'s own mailbox delivery bypasses prompt.submit but still sends a mail wake', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsWake = true
  await start($, w)
  await flush(w)
  w.recvAnswers.push(mailAnswer('mail body'))
  w.exits.at(-1)!(0, ['mail'])
  await w.clock.settle()
  await flush(w)
  const wakes = sent(w).filter((e) => e.kind === 'wake')
  expect(wakes.length).toBe(1)
  expect(wakes[0]?.cause).toBe('mail')
  expect(wakes[0]?.text).toContain('mail body')
  expect(sent(w).filter((e) => e.kind === 'user')).toEqual([])
})

// ---- Usage status ---------------------------------------------------------

const statuses = (w: World) => sent(w).filter((e) => e.kind === 'status')

async function measure($: Engine, w: World, tokens: number) {
  w.usage.context.tokens = tokens
  await $.session.measure({ context: { tokens, window: w.usage.context.window }, rateLimits: [], changed: ['context'] } as never)
  await flush(w)
}

test('status is never sent while the intake results name no accepts line', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await measure($, w, 60_000)
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await measure($, w, 70_000)
  expect(w.intakes.length).toBeGreaterThan(0)
  expect(statuses(w)).toEqual([])
})

test('status goes once a result names it, follows each measure, stops after a result without it, and returns with it', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  await start($, w)
  // The first result names status: the figures held from the start go next.
  await flush(w)
  expect(statuses(w)).toEqual([
    {
      kind: 'status',
      context: { tokens: 50_000, window: 200_000, percent: 25 },
      limits: [{ kind: 'five_hour', percent: 30, resetsAt: NOW + 2 * 3600_000 }],
      cost: { usd: 1.5, estimate: false, incomplete: false },
    },
  ])
  await measure($, w, 100_000)
  expect(statuses(w).at(-1)?.context).toEqual({ tokens: 100_000, window: 200_000, percent: 50 })
  // Unchanged figures are not sent again.
  const count = statuses(w).length
  await measure($, w, 100_000)
  expect(statuses(w).length).toBe(count)
  // The dashboard is turned off: its result names nothing, so status stops.
  w.intake = 'off'
  await $.turn.start({ text: 'x', turnId: 't' })
  await flush(w)
  await measure($, w, 120_000)
  await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await flush(w)
  expect(statuses(w).length).toBe(count)
  // Back on: the next result names status again and the latest figures follow.
  w.intake = 'ok'
  await $.turn.start({ text: 'y', turnId: 't2' })
  await flush(w)
  await flush(w)
  expect(statuses(w).length).toBe(count + 1)
  expect(statuses(w).at(-1)?.context).toEqual({ tokens: 120_000, window: 200_000, percent: 60 })
})

test('status follows each model tool call mid-turn, as the band does; the mod\'s own calls read nothing', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  await start($, w)
  await flush(w)
  await $.turn.start({ text: 'x', turnId: 't1' })
  await flush(w)
  const count = statuses(w).length
  // The mod's own mailbox read while the turn runs: no fresh figures go.
  w.usage.context.tokens = 80_000
  w.recvAnswers.push(mailAnswer('hello'))
  w.exits.at(-1)!(0, ['mail'])
  await w.clock.advance(50)
  await flush(w)
  expect(statuses(w).length).toBe(count)
  // A model tool call, with no measure behind it: the page gets the context as it reads now.
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  await flush(w)
  expect(statuses(w).length).toBe(count + 1)
  expect(statuses(w).at(-1)?.context).toEqual({ tokens: 80_000, window: 200_000, percent: 40 })
  // Unchanged figures after the next call are not sent again.
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u2', command: 'ls' } as never)
  await flush(w)
  expect(statuses(w).length).toBe(count + 1)
})

test('a resync sends the held status again behind its reset',{ timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  await start($, w)
  await flush(w)
  expect(statuses(w).length).toBe(1)
  // A failed call resyncs; status waits for a result that names it again.
  w.intake = 'error'
  await $.turn.start({ text: 'x', turnId: 't' })
  await flush(w)
  w.intake = 'ok'
  await w.clock.advance(2500)
  // The figures follow the resync's answer once the retry gap has passed.
  await w.clock.advance(2500)
  const resync = w.intakes.findLastIndex((c) => c.events[0]?.kind === 'reset')
  expect(resync).toBeGreaterThan(0)
  const after = w.intakes.slice(resync).flatMap((c) => c.events)
  expect(after.filter((e) => e.kind === 'status')).toEqual([expect.objectContaining({ kind: 'status', context: { tokens: 50_000, window: 200_000, percent: 25 } })])
})

test('acceptedKinds reads the accepts line; no line accepts nothing', () => {
  expect(acceptedKinds('ok: 1 of 1 events applied\naccepts: status\n')).toEqual(['status'])
  expect(acceptedKinds('ok: 2 of 2 events applied\naccepts: status, other\nskipped event 1: x\n')).toEqual(['status', 'other'])
  expect(acceptedKinds('ok: 1 of 1 events applied\n')).toEqual([])
  expect(acceptedKinds('dashboard off: nothing recorded\n')).toEqual([])
})

test('intakeResult joins the result text and reads an error result as undelivered', () => {
  expect(intakeResult({ content: [{ type: 'text', text: 'ok: 1 of 1 events applied' }, { type: 'text', text: 'accepts: status' }] })).toEqual({
    ok: true,
    text: 'ok: 1 of 1 events applied\naccepts: status',
  })
  expect(intakeResult({ isError: true, content: [{ type: 'text', text: 'unknown session_key' }] })).toEqual({ ok: false, text: 'unknown session_key' })
  expect(intakeResult(undefined)).toEqual({ ok: true, text: '' })
})

test('statusFigures: compaction window, last-response cache hit over all input, reported limits only, exact cost', () => {
  const resetsAt = '2027-01-15T18:00:00.000Z'
  const figures = statusFigures({
    startedAt: 0,
    context: {
      tokens: 40_000,
      window: 1_000_000,
      breakdown: {
        rawMaxTokens: 160_000,
        apiUsage: { input_tokens: 1_000, output_tokens: 500, cache_read_input_tokens: 36_000, cache_creation_input_tokens: 3_000 },
      },
    },
    rateLimits: [
      { kind: 'seven_day', percentUsed: 17, resetsAt },
      { kind: 'spend_limit', percentUsed: 5 },
    ],
    cost: { usd: 2.25 },
  } as never)
  expect(figures).toEqual({
    context: { tokens: 40_000, window: 160_000, percent: 25, cacheHit: 90 },
    limits: [{ kind: 'seven_day', percent: 17, resetsAt: Date.parse(resetsAt) }],
    cost: { usd: 2.25, estimate: false, incomplete: false },
  })
  // Unknown tokens (just compacted), no limits, no cost, an unparseable reset: those parts go.
  expect(
    statusFigures({ startedAt: 0, context: { window: 200_000 }, rateLimits: [{ kind: 'five_hour', percentUsed: 42, resetsAt: 'soon' }] } as never),
  ).toEqual({ context: { tokens: null, window: 200_000 }, limits: [{ kind: 'five_hour', percent: 42 }] })
})

// ---- Compaction --------------------------------------------------------------

const compacts = (w: World) => sent(w).filter((e) => e.kind === 'compact')

async function compact($: Engine, trigger: string, agentId?: string) {
  return $.session.compact({ trigger, messages: [{ role: 'user', text: 'long', toolUses: [] }], ...(agentId ? { agentId } : {}) } as never)
}

test('a main compaction sends its start while it runs, then its end with the trigger; a landing pushes fresh status figures', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  w.acceptsCompact = true
  await start($, w)
  await flush(w)
  let open!: () => void
  w.compactGate = new Promise<void>((resolve) => (open = resolve))
  const running = compact($, 'auto')
  await flush(w)
  expect(compacts(w)).toEqual([{ kind: 'compact', phase: 'start', trigger: 'auto', at: expect.any(Number) }])
  // The compaction lands; no measure follows it, yet the page gets the context as it reads now.
  w.usage.context.tokens = 20_000
  open()
  const res = await running
  // Observe-only: the result is the compaction beneath's.
  expect(res).toEqual({ messages: [expect.objectContaining({ role: 'user', text: 'summary' })] })
  await flush(w)
  await flush(w)
  expect(compacts(w).slice(1)).toEqual([{ kind: 'compact', phase: 'end', trigger: 'auto', at: expect.any(Number) }])
  const events = sent(w)
  const end = events.findIndex((e) => e.kind === 'compact' && e.phase === 'end')
  expect(events.slice(end).filter((e) => e.kind === 'status').at(-1)?.context).toEqual({ tokens: 20_000, window: 200_000, percent: 10 })
})

for (const [outcome, label] of [
  ['skip', 'vetoed'],
  ['throw', 'failed'],
] as const) {
  test(`a ${label} compaction ends as failed and pushes no status`, { timeoutMs: 20_000 }, async ($, on) => {
    const w = world(on, { dashboard: true })
    w.acceptsStatus = true
    w.acceptsCompact = true
    await start($, w)
    await flush(w)
    const before = statuses(w).length
    w.usage.context.tokens = 20_000
    w.compact = outcome
    let threw = false
    try {
      const res = await compact($, 'manual')
      expect(res).toEqual({ skip: 'vetoed by the test' })
    } catch {
      threw = true
    }
    expect(threw).toBe(outcome === 'throw')
    await flush(w)
    expect(compacts(w).map((e) => [e.phase, e.trigger])).toEqual([
      ['start', 'manual'],
      ['failed', 'manual'],
    ])
    expect(statuses(w).length).toBe(before)
  })
}

test('a subagent\'s compaction and a precompute send nothing; a plugin\'s carries no trigger', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsCompact = true
  await start($, w)
  await flush(w)
  await compact($, 'auto', 'a77')
  await compact($, 'precompute')
  await flush(w)
  expect(compacts(w)).toEqual([])
  await compact($, 'plugin')
  await flush(w)
  expect(compacts(w)).toEqual([
    { kind: 'compact', phase: 'start', at: expect.any(Number) },
    { kind: 'compact', phase: 'end', at: expect.any(Number) },
  ])
})

test('compact is never sent while the intake results do not name it', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  await start($, w)
  await flush(w)
  await compact($, 'auto')
  await flush(w)
  expect(w.intakes.length).toBeGreaterThan(0)
  expect(compacts(w)).toEqual([])
})

test('a resync while a compaction runs sends its start again behind the reset', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsCompact = true
  await start($, w)
  await flush(w)
  let open!: () => void
  w.compactGate = new Promise<void>((resolve) => (open = resolve))
  const running = compact($, 'auto')
  await flush(w)
  expect(compacts(w).length).toBe(1)
  // A failed call resyncs; the start follows the resync's answer.
  w.intake = 'error'
  await $.turn.start({ text: 'x', turnId: 't' })
  await flush(w)
  w.intake = 'ok'
  await w.clock.advance(2500)
  await w.clock.advance(2500)
  const resync = w.intakes.findLastIndex((c) => c.events[0]?.kind === 'reset')
  expect(resync).toBeGreaterThan(0)
  const after = w.intakes.slice(resync).flatMap((c) => c.events)
  expect(after.filter((e) => e.kind === 'compact')).toEqual([{ kind: 'compact', phase: 'start', trigger: 'auto', at: compacts(w)[0]!.at }])
  open()
  await running
  // The feed's retry gap runs on the wall clock, so it may still hold the end back.
  await w.clock.advance(2500)
  expect(compacts(w).at(-1)).toEqual({ kind: 'compact', phase: 'end', trigger: 'auto', at: expect.any(Number) })
})

for (const outcome of ['land', 'skip', 'throw'] as const) {
  test(`knob on: a board wake during a main compaction holds the message; the compaction's end (${outcome}) submits it`, { timeoutMs: 20_000 }, async ($, on) => {
    const w = world(on, { dashboard: true })
    await start($, w)
    let open!: () => void
    w.compact = outcome
    w.compactGate = new Promise<void>((resolve) => (open = resolve))
    const running = compact($, 'auto').catch(() => undefined)
    await w.clock.advance(50)
    w.inbox.push('pick B')
    w.exits.at(-1)!(0, ['board'])
    await w.clock.advance(50)
    expect(w.inboxDrains).toEqual([])
    expect(w.prompts).toEqual([])
    expect(w.inbox).toEqual(['pick B'])
    expect(w.argvs.at(-1)).not.toContain(BOARD_FLAG)
    open()
    await running
    await w.clock.advance(50)
    expect(w.inboxDrains.length).toBe(1)
    expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'pick B')])
    expect(w.argvs.at(-1)).toContain(BOARD_FLAG)
  })
}

test('knob on: a subagent\'s compaction holds nothing', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  let open!: () => void
  w.compactGate = new Promise<void>((resolve) => (open = resolve))
  const running = compact($, 'auto', 'a77')
  await w.clock.advance(50)
  w.inbox.push('pick B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.advance(50)
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'pick B')])
  open()
  await running
})

test('knob on: a message held through the compaction lever\'s own compaction follows its resume prompt', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.engine = $
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.tool.call({ tool: `mcp__${w.plugin}__request_compaction`, continue: true } as never)
  let open!: () => void
  w.compactGate = new Promise<void>((resolve) => (open = resolve))
  await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  // The compact step runs and its compaction waits beneath; the turn end's own drain claimed nothing.
  await w.clock.settle()
  await w.clock.advance(SETTLE_MS)
  const drains = w.inboxDrains.length
  w.inbox.push('pick B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.advance(50)
  expect(w.inboxDrains.length).toBe(drains)
  expect(w.prompts).toEqual([])
  open()
  await w.clock.advance(50)
  // The resume window is open: the held message does not start a turn that would cancel it.
  expect(w.prompts).toEqual([])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await w.clock.advance(50)
  expect(w.prompts).toEqual([resumeText(w.plugin), labelWhiteboard(w.plugin, 'pick B')])
})

// ---- Slash commands ------------------------------------------------------

const commandLists = (w: World) => sent(w).filter((e) => e.kind === 'commands')
const commandLines = (w: World) => sent(w).filter((e) => e.kind === 'command')
const cmd = (name: string, args = '') => ({ name, args })

test('the command list is never sent while the intake results do not name commands', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsStatus = true
  await start($, w)
  await flush(w)
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await flush(w)
  await flush(w)
  expect(w.commandLists).toBeGreaterThan(0)
  expect(commandLists(w)).toEqual([])
})

test('the command list goes once a result names commands, as name, description, and source; a turn end sends it again only when it changed, and a resync sends it behind its reset', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsCommands = true
  await start($, w)
  await flush(w)
  expect(commandLists(w)).toEqual([
    {
      kind: 'commands',
      commands: [
        { name: 'compact', description: 'Clear the history but keep a summary', source: 'builtin' },
        { name: 'context', description: 'Show context usage', source: 'builtin' },
        { name: 'config', description: 'Open the settings panel', source: 'builtin' },
        { name: 'review', description: 'Review a branch', source: 'plugin' },
      ],
    },
  ])
  // An unchanged list at a turn end is read but not sent again.
  const reads = w.commandLists
  await $.turn.start({ text: 'x', turnId: 't' })
  await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await flush(w)
  expect(w.commandLists).toBe(reads + 1)
  expect(commandLists(w).length).toBe(1)
  // A command added during the next turn goes at its end.
  w.commands.push({ name: 'deploy', description: 'Ship it', source: 'user' })
  await $.turn.start({ text: 'y', turnId: 't2' })
  await $.turn.complete({ answer: 'hi', durationMs: 1, isAborted: false, turnId: 't2', reason: 'answer' } as never)
  await flush(w)
  expect(commandLists(w).length).toBe(2)
  expect((commandLists(w).at(-1)!.commands as unknown[]).at(-1)).toEqual({ name: 'deploy', description: 'Ship it', source: 'user' })
  // A failed call resyncs: the list follows the reset again, once the resync's answer names commands.
  w.intake = 'error'
  await $.turn.start({ text: 'z', turnId: 't3' })
  await flush(w)
  w.intake = 'ok'
  await w.clock.advance(2500)
  await w.clock.advance(2500)
  const resync = w.intakes.findLastIndex((c) => c.events[0]?.kind === 'reset')
  expect(resync).toBeGreaterThan(0)
  expect(w.intakes.slice(resync).flatMap((c) => c.events).filter((e) => e.kind === 'commands').length).toBe(1)
})

test('knob on: an idle drain of text, a command, and text submits the first text, runs the command as typed, and submits the last text only after the run; the command is never labelled or joined', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsCommands = true
  w.commandAnswers.set('context', { text: 'Context: 12k of 200k' })
  await start($, w)
  w.inbox.push('A', cmd('context', 'all'), 'B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.settle()
  await w.clock.settle()
  expect(w.order).toEqual([`prompt ${labelWhiteboard(w.plugin, 'A')}`, 'run /context all', `prompt ${labelWhiteboard(w.plugin, 'B')}`])
  expect(w.prompts.some((p) => p.includes('/context'))).toBe(false)
  expect(w.rows).toEqual([])
  await flush(w)
  expect(commandLines(w)).toEqual([{ kind: 'command', command: '/context all', text: 'Context: 12k of 200k', at: expect.any(Number) }])
  // The text messages enter the ring as whiteboard messages; the command only as its own line.
  expect(sent(w).filter((e) => e.kind === 'user').map((e) => e.text)).toEqual(['A', 'B'])
})

test('knob on: a run with no text sends the command line without text; a refused run sends the refusal as its text', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.acceptsCommands = true
  await start($, w)
  await flush(w)
  w.inbox.push(cmd('config'), cmd('nosuch'), 'after')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.settle()
  await w.clock.settle()
  await flush(w)
  expect(w.commandRuns).toEqual(['/config', '/nosuch'])
  expect(commandLines(w)).toEqual([
    { kind: 'command', command: '/config', at: expect.any(Number) },
    // Nothing beneath answers an unknown command, so the engine rejects the run.
    { kind: 'command', command: '/nosuch', text: expect.stringMatching(/^The command did not run: ./), at: expect.any(Number) },
  ])
  expect(w.logs.some((l) => l.includes('/nosuch failed'))).toBe(true)
  // The message after the refused command still goes.
  expect(w.prompts).toEqual([labelWhiteboard(w.plugin, 'after')])
})

test('knob on: a command\'s outcome reaches the intake only while it names command', async ($, on) => {
  const w = world(on, { dashboard: true })
  w.commandAnswers.set('context', { text: 'Context: 12k' })
  await start($, w)
  await flush(w)
  w.inbox.push(cmd('context'))
  w.exits.at(-1)!(0, ['board'])
  await w.clock.settle()
  await flush(w)
  expect(w.commandRuns).toEqual(['/context'])
  expect(commandLines(w)).toEqual([])
})

test('knob on: a command sent while the lead\'s turn runs waits for the turn end, a tool boundary included', async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  await $.turn.start({ text: 'x', turnId: 't' })
  w.inbox.push(cmd('context'))
  w.exits.at(-1)!(0, ['board'])
  await w.clock.advance(50)
  await $.tool.call({ tool: 'Bash', tool_use_id: 'u1', command: 'ls' } as never)
  await w.clock.advance(50)
  expect(w.commandRuns).toEqual([])
  expect(w.rows).toEqual([])
  await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.advance(50)
  expect(w.commandRuns).toEqual(['/context'])
  expect(w.prompts).toEqual([])
})

test('knob on: a command sent during a main compaction runs once the compaction settled', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { dashboard: true })
  await start($, w)
  let open!: () => void
  w.compactGate = new Promise<void>((resolve) => (open = resolve))
  const running = compact($, 'auto')
  await w.clock.advance(50)
  w.inbox.push(cmd('context'), 'B')
  w.exits.at(-1)!(0, ['board'])
  await w.clock.advance(50)
  expect(w.commandRuns).toEqual([])
  expect(w.prompts).toEqual([])
  open()
  await running
  await w.clock.advance(50)
  expect(w.order).toEqual(['run /context', `prompt ${labelWhiteboard(w.plugin, 'B')}`])
})

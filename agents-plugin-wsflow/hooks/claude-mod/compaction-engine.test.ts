import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { ENABLED_KEY, OWN_SETTLE_MS } from './captions'
import { CONTINUATION_WINDOW_MS, SETTLE_MS, UNOBSERVED_REASON } from './compaction'
import { KNOB } from './register'
import {
  ADVISORY_KEY,
  advisoryText,
  deliveryLine,
  LEVER_DESCRIPTION,
  milestoneText,
  LEVER_RESULT,
  LEVER_RESULT_NO_RESUME,
  resumeLine,
  resumeText,
  thresholdsFor,
  vetoedResumeLine,
  vetoedResumeText,
} from './timing'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the compaction feature's wiring; the feature's state
// machine is covered in compaction.test.ts. The same file is mirrored into
// every package that ships the mod and runs identically in each: the lever
// runs whatever the knobs read, and the advisory and its milestones only while
// the claude.compaction_advisory_enabled knob reads true, which each test sets
// through the world's config_get answers (ON).

const WINDOW = { rawMaxTokens: 200_000, autoCompactThreshold: 170_000, isAutoCompactEnabled: true }
const T = thresholdsFor(WINDOW)!

/**
 * How the lever's next `/compact` command goes: its compaction lands or is
 * vetoed beneath the mod; `rejects` is the engine refusing the command;
 * `unobserved` runs it with no compaction.
 */
type CompactAnswer = { skip: string } | 'lands' | 'rejects' | 'unobserved'

/** A compaction leaves at least one message: the summary. */
const SUMMARY = [{ role: 'user', text: 'summary', toolUses: [] }]

/** How one mcp.connect answers: after a delay (ms on the mock clock), not connected, or with a rejection. */
type Connect = number | 'down' | 'reject'

type Opts = {
  processOk?: boolean
  refuseRegister?: string
  advisory?: unknown
  advisoryError?: boolean
  /** The claude.compaction_advisory_enabled value config_get answers; unset reads as null. */
  compaction?: unknown
  /** The advisory knob's config_get answers an error result. */
  failKnob?: boolean
  /** The knob is tuned at session scope only: a read without session_key finds it unset. */
  sessionScoped?: boolean
  /** The start's mcp.connect answer. */
  connect?: Connect
  /** The caption switch (claude.display_summary_enabled) reads true. */
  captions?: boolean
}

/** The advisory knob on. */
const ON = { compaction: true } as const

function world(on: On, opts: Opts = {}) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    registered: [] as { name: string; description: string; isDeferred?: boolean; inputSchema?: unknown }[],
    usageCalls: [] as unknown[],
    /** The tokens a usage read reports: the last measure's, unless a test moves it. */
    live: undefined as number | undefined,
    /** The compactions that reached the engine, with their trigger. */
    compactCalls: [] as { trigger: unknown; instructions: unknown }[],
    /** The `/compact` commands run, and by whom. */
    commands: [] as { command: string; args: unknown; origin: string }[],
    compactAnswers: [] as CompactAnswer[],
    /** The answer the next compaction beneath the mod gives; absent, it lands. */
    pendingCompact: undefined as { skip: string } | undefined,
    /** The test's engine, bound at start: a test hook's own `$` may not raise a compaction. */
    engine: undefined as Engine | undefined,
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
    /** The advisory knob's config_get arguments. */
    knobReads: [] as Record<string, unknown>[],
    /** The advisory knob's value, for a later session start to read. */
    compaction: opts.compaction,
    /** The mod's own rows and non-composer submits, in order: `user:`, `system:` or `submit:` and the text. */
    own: [] as string[],
    /** The model requests the mod made (captions, among them). */
    completions: 0,
  }
  let connect = opts.connect
  on('mcp.connect', async ($, e) => {
    const answer = connect
    connect = undefined
    if (typeof answer === 'number') await clock.sleep(answer)
    if (answer === 'reject') throw new Error('connect failed')
    if (answer === 'down') return { value: { isConnected: false, reason: 'failed', message: 'did not connect' } }
    return { value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }
  })
  on('mcp.call', ($, e) => {
    if (e.tool !== 'config_get') return { value: { content: [{ type: 'text', text: `no ${e.tool} here` }], isError: true } }
    if (e.args.key === KNOB.compactionAdvisory) {
      w.knobReads.push({ ...e.args })
      if (opts.failKnob) return { value: { content: [{ type: 'text', text: 'ws-mcp is unhealthy' }], isError: true } }
      const value = w.compaction === undefined || (opts.sessionScoped && e.args.session_key === undefined) ? null : w.compaction
      return { value: { content: [{ type: 'text', text: JSON.stringify({ key: e.args.key, value, scope: value === null ? 'unset' : 'project' }) }], isError: false } }
    }
    if (opts.captions && e.args.key === ENABLED_KEY) {
      return { value: { content: [{ type: 'text', text: JSON.stringify({ key: e.args.key, value: true, scope: 'project' }) }], isError: false } }
    }
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
  on('session.append', ($, e, next) => {
    w.own.push(`${e.message.type}:${e.message.content.map((b) => (b.type === 'text' ? b.text : '')).join('')}`)
    return next(e)
  })
  on('session.messages', () => ({ value: [] }))
  on('model.complete', () => {
    w.completions += 1
    return { value: { isAnswered: false, reason: 'empty-reply', usage: { input_tokens: 1, output_tokens: 1, cache_creation_input_tokens: 0, cache_read_input_tokens: 0 } } }
  })
  on('session.usage', ($, e) => {
    w.usageCalls.push(e)
    return { value: { startedAt: 0, context: { tokens: w.live, window: 1_000_000, breakdown: e.breakdown ? WINDOW : undefined }, rateLimits: [] } as never }
  })
  on('session.compact', ($, e) => {
    w.compactCalls.push({ trigger: e.trigger, instructions: e.instructions })
    const answer = w.pendingCompact
    w.pendingCompact = undefined
    return answer ?? { messages: SUMMARY }
  })
  // `/compact` compacts as a typed one does: a manual compaction through every plugin's session.compact hook.
  on('command.run', async ($, e) => {
    if (e.command !== 'compact') return {}
    w.commands.push({ command: e.command, args: e.args, origin: e.origin.kind === 'plugin' ? e.origin.name : e.origin.kind })
    const answer = w.compactAnswers.shift() ?? 'lands'
    if (answer === 'rejects') throw new Error('past a Stop')
    if (answer === 'unobserved') return {}
    if (answer !== 'lands') w.pendingCompact = answer
    await w.engine!.session.compact({ trigger: 'manual', messages: SUMMARY } as never)
    return {}
  })
  on('prompt.submit', ($, e) => {
    if (e.origin?.kind === 'composer') {
      w.typed.push({ text: e.text, context: e.context })
      return { text: e.text, context: e.context }
    }
    w.own.push(`submit:${e.text}`)
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

/** The lever's `/compact` command, run by the mod. */
const LEVER_COMMAND = (w: World) => ({ command: 'compact', args: '', origin: w.plugin })
/** A manual compaction as it reached the engine. */
const MANUAL = { trigger: 'manual', instructions: undefined }

async function start($: Engine, w: World) {
  w.engine = $
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

async function measure($: Engine, w: World, tokens: number) {
  w.live = tokens
  await $.session.measure({ context: { tokens, window: 1_000_000 }, rateLimits: [], changed: ['context'] } as never)
  await w.clock.settle()
}

/** A turn end, then SETTLE_MS on the clock unless `settle` is false: a pending request's compact step runs within it. */
async function turnEnd($: Engine, w: World, opts: { reason?: string; agentId?: string; settle?: boolean } = {}) {
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
  if (opts.settle !== false) await w.clock.advance(SETTLE_MS)
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

/** The transcript (user-visible) lines the mod logged. */
const transcript = (w: World) => w.logs.filter((l) => l.to === 'transcript').map((l) => l.text)
const debug = (w: World) => w.logs.filter((l) => l.to === 'debug').map((l) => l.text)

test('every start registers the compaction request tool with its pinned description and boolean flag, whatever the advisory knob reads', async ($, on) => {
  const w = world(on)
  await start($, w)
  const tool = w.registered.find((r) => r.name === 'request_compaction')
  expect(tool).toEqual({ name: 'request_compaction', description: LEVER_DESCRIPTION, isDeferred: false, inputSchema: tool?.inputSchema })
  // The flag is optional: the schema lists it, and does not require it.
  expect(tool?.inputSchema).toEqual({ type: 'object', properties: { continue: { type: 'boolean' } } })
  // Read keyless, so a session-scope tune cannot reach it.
  expect(w.knobReads).toEqual([{ key: KNOB.compactionAdvisory, format: 'json' }])
})

test('compaction setup does not depend on the mailbox setup', async ($, on) => {
  const w = world(on, { ...ON, processOk: false })
  await start($, w)
  const names = w.registered.map((r) => r.name)
  expect(names.filter((n) => n.startsWith('mailbox_'))).toEqual([])
  expect(names.filter((n) => n === 'request_compaction')).toEqual(['request_compaction'])
})

test('mailbox setup does not depend on the compaction setup', async ($, on) => {
  const w = world(on, { ...ON, refuseRegister: 'request_compaction' })
  await start($, w)
  const names = w.registered.map((r) => r.name)
  expect(names.filter((n) => n.startsWith('mailbox_'))).toEqual(['mailbox_watch'])
  expect(names.filter((n) => n === 'request_compaction')).toEqual([])
})

test('a crossing that session.measure reports while idle submits the advisory at once, reading the summary breakdown once and the usage afresh', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await measure($, w, 105_000)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 104_000)])
  expect(w.usageCalls).toEqual([{ breakdown: 'summary' }, {}])
})

test('a crossing measured while a main-thread turn runs is delivered at the turn end, once, with the fresh figures', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  await turnStart($, w)
  // A mid-turn measure (a rate-limit move) crosses the advisory; the turn goes on growing the context.
  await measure($, w, 104_000)
  expect(w.prompts).toEqual([])
  w.live = 118_000
  await turnEnd($, w)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 118_000)])
  expect(w.prompts[0]).toContain('59% (118k / 200k tokens)')
  expect(debug(w).filter((l) => l.startsWith(`${w.plugin} compaction: advisory delivered`))).toEqual([
    `${w.plugin} compaction: advisory delivered: crossing measured at 104000 tokens (52%), fresh read 118000 tokens (59%), compaction window 200000 tokens, model window 1000000 tokens`,
  ])
  // The end-of-turn measure arriving after the turn end delivers nothing again.
  await measure($, w, 118_000)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 118_000)])
})

test('a mid-turn crossing that grew past a milestone by the turn end delivers that milestone once, skipping the lower one', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 104_000)])
  await turnStart($, w)
  await measure($, w, 125_000)
  w.live = 150_000
  await turnEnd($, w)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 104_000), milestoneText(w.plugin, 2, T, 150_000)])
  await measure($, w, 150_000)
  expect(w.prompts.length).toBe(2)
})

test('each session start reads the advisory key once over the plugin server, and a tuned value moves the advisory', async ($, on) => {
  const w = world(on, { ...ON, advisory: '40%|300k' })
  await start($, w)
  expect(w.configGets).toEqual([{ server: `plugin:${w.plugin}:${w.plugin}`, args: { key: 'claude.compaction_advisory_percent', format: 'json' } }])
  await measure($, w, 50_000)
  await measure($, w, 79_000)
  expect(w.prompts).toEqual([])
  await measure($, w, 81_000)
  expect(w.prompts).toEqual([advisoryText(w.plugin, thresholdsFor(WINDOW, { percent: 40, tokens: 300_000 })!, 81_000)])
  expect(w.prompts[0]).toContain('This is past the advisory point (40%).')
  await start($, w)
  expect(w.configGets.length).toBe(2)
  expect(w.knobReads.length).toBe(2)
})

for (const [what, opts] of [
  ['reports an error', { advisoryError: true }],
  ['returns a stored value that does not parse', { advisory: 'lots' }],
] as const) {
  test(`an advisory read that ${what} leaves the advisory at 50%`, async ($, on) => {
    const w = world(on, { ...ON, ...opts })
    await start($, w)
    expect(w.configGets.length).toBe(1)
    await measure($, w, 50_000)
    await measure($, w, 81_000)
    expect(w.prompts).toEqual([])
    await measure($, w, 104_000)
    expect(w.prompts).toEqual([advisoryText(w.plugin, T, 104_000)])
  })
}

test('a lever call with the flag compacts after the main-thread turn end, then submits the resume prompt once', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  const res = await lever($, w, { continue: true })
  expect(res.result).toBe(LEVER_RESULT)
  await turnEnd($, w, { agentId: 'sub-1' })
  await turnEnd($, w, { reason: 'aborted' })
  expect(w.commands).toEqual([])
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  expect(w.prompts).toEqual([])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([resumeText(w.plugin)])
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(w.prompts.length).toBe(1)
})

test('a lever call without the flag compacts at the turn end and submits nothing, with no window', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  const res = await lever($, w)
  expect(res.result).toBe(LEVER_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
})

test('a leftover continuation argument, or a non-boolean flag, reads as unset', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  expect((await lever($, w, { continuation: 'go on' })).result).toBe(LEVER_RESULT_NO_RESUME)
  expect((await lever($, w, { continue: 'true' })).result).toBe(LEVER_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
})

test('a repeat lever call replaces the flag, latest wins', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await lever($, w)
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await lever($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.commands.length).toBe(2)
  expect(w.prompts).toEqual([resumeText(w.plugin)])
})

test("the lever's own landing, a manual compaction through the feature's own hook, resets the latch once", async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  // Usage still past the advisory after the landing: a reset latch delivers it again, from a fresh breakdown.
  await measure($, w, 104_000)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 104_000), resumeText(w.plugin), advisoryText(w.plugin, T, 104_000)])
  // Each delivery reads the usage afresh; the landing drops the cached breakdown.
  expect(w.usageCalls).toEqual([{ breakdown: 'summary' }, {}, { breakdown: 'summary' }, {}])
  expect(transcript(w)).toEqual([])
})

test('a subagent lever call is refused and requests nothing', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  const fromSub = await lever($, w, { continue: true }, 'sub-1')
  expect(fromSub.deny).toBe('request_compaction compacts the main session only; called from subagent sub-1.')
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
})

test('a refused compact command is permanent: one notice, a noted resume, no retry at later turn ends', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  w.compactAnswers.push('rejects')
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(w.compactCalls).toEqual([])
  expect(w.prompts).toEqual([])
  // The refusal's own text is the engine's; the notice carries it whole.
  expect(transcript(w).length).toBe(1)
  expect(transcript(w)[0]).toMatch(
    new RegExp(`^${w.plugin} compaction: the requested compaction did not run \\(the compact command was refused: .+\\); a resume prompt follows with a note$`),
  )
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts.length).toBe(1)
  expect(w.prompts[0]).toMatch(/Compaction did not run \(the compact command was refused: .+\); your context is unchanged/)
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(transcript(w).length).toBe(1)
})

test('a compact command that ran with no compaction observed settles as a veto, not a landing or a retry', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  w.compactAnswers.push('unobserved')
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([vetoedResumeText(w.plugin, UNOBSERVED_REASON)])
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
})

test('the lever compacts in a headless session too: the same command, no session.compact call', async ($, on) => {
  const w = world(on, ON)
  w.engine = $
  await $.session.start({ cwd: '/tmp', surface: null, isInteractive: false } as never)
  await w.clock.settle()
  await lever($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  expect(w.compactCalls).toEqual([MANUAL])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([resumeText(w.plugin)])
})

test('the compact step waits SETTLE_MS after the turn end, and a turn started by then holds it for that turn end', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w, { settle: false })
  await w.clock.advance(SETTLE_MS - 1)
  expect(w.commands).toEqual([])
  // A queued prompt starts its turn before the step: no command is queued behind it.
  await turnStart($, w)
  await w.clock.advance(1)
  expect(w.commands).toEqual([])
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([resumeText(w.plugin)])
})

test('a vetoed compaction with the flag still submits the noted resume prompt when no turn started', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([vetoedResumeText(w.plugin, 'blocked by policy')])
})

test('a vetoed compaction with the flag submits nothing when a turn started within the window', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
  await turnStart($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
})

test('a manual compaction landing first drops the compact step; the resume prompt follows at the turn end, unguarded', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await $.session.compact({ trigger: 'manual', messages: SUMMARY } as never)
  await w.clock.settle()
  // No window is open: a turn start before that turn end does not cancel it.
  await turnStart($, w)
  await turnEnd($, w)
  expect(w.compactCalls).toEqual([MANUAL])
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([resumeText(w.plugin)])
})

test('session.end drops a pending request and starts the latch over', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  await lever($, w, { continue: true })
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await turnEnd($, w)
  await measure($, w, 120_000)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
})

test('session.end resets the compaction state before awaiting the store delete', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  // A delete still pending stands for one the hook's time bound cuts short.
  w.deleteDelays.push(100)
  const ended = $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
  await w.clock.advance(100)
  await ended
})

/** The advisory inert: no advisory read, no usage read, no prompt, no compaction, and compactions pass through untouched; the lever still works. */
async function expectInert($: Engine, w: World) {
  expect(w.registered.map((r) => r.name)).toContain('request_compaction')
  expect(w.configGets).toEqual([])
  await measure($, w, 50_000)
  await measure($, w, 160_000)
  await turnEnd($, w)
  const res = await $.session.compact({ trigger: 'manual', messages: SUMMARY } as never)
  await w.clock.settle()
  expect(res).toEqual({ messages: SUMMARY })
  expect(w.usageCalls).toEqual([])
  expect(w.prompts).toEqual([])
  expect(w.compactCalls).toEqual([MANUAL])
  expect(w.commands).toEqual([])
  await prompted($, w, 'hello')
  expect(w.typed).toEqual([{ text: 'hello', context: undefined }])
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([])
  await start($, w)
  expect((await lever($, w, { continue: true })).result).toBe(LEVER_RESULT)
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.compactCalls).toEqual([MANUAL, MANUAL])
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  expect(w.prompts).toEqual([resumeText(w.plugin)])
}

for (const [what, opts] of [
  ['unset', {}],
  ['false', { compaction: false }],
  ['a non-boolean true', { compaction: 'true' }],
  ['an error result (a failed read)', { compaction: true, failKnob: true }],
  ['tuned at session scope only', { compaction: true, sessionScoped: true }],
] as const) {
  test(`an advisory knob ${what} leaves the advisory inert, and the lever works`, async ($, on) => {
    const w = world(on, opts)
    await start($, w)
    expect(w.knobReads).toEqual([{ key: KNOB.compactionAdvisory, format: 'json' }])
    await expectInert($, w)
  })
}

for (const unresolved of ['down', 'reject'] as const) {
  test(`a server ${unresolved === 'down' ? 'not connected' : 'whose connect rejects'} at start leaves the advisory inert though its knob is on, and the lever works`, async ($, on) => {
    const w = world(on, { ...ON, connect: unresolved })
    await start($, w)
    expect(w.knobReads).toEqual([])
    await expectInert($, w)
  })
}

test('a slow server start holds neither session.start nor the lever; the advisory comes up once its knob read settles', async ($, on) => {
  const w = world(on, { ...ON, connect: 1_000 })
  let started = false
  w.engine = $
  void $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true }).then(() => (started = true))
  await w.clock.settle()
  expect(started).toBe(true)
  expect(w.registered.map((r) => r.name).sort()).toEqual(['request_compaction', 'request_mcp_reconnect'])
  expect(w.knobReads).toEqual([])
  expect((await lever($, w)).result).toBe(LEVER_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
  await w.clock.advance(1_000)
  expect(w.knobReads).toEqual([{ key: KNOB.compactionAdvisory, format: 'json' }])
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  expect(w.prompts).toEqual([advisoryText(w.plugin, T, 104_000)])
})

test('each session start reads the advisory knob afresh: a later start reading off leaves the advisory inert, and the lever answered', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  const usageCalls = w.usageCalls.length
  // A hot reload's start, after the knob was turned off at the project scope.
  w.compaction = false
  await start($, w)
  expect(w.knobReads.length).toBe(2)
  await measure($, w, 160_000)
  await turnEnd($, w)
  expect(w.usageCalls.length).toBe(usageCalls)
  expect(w.prompts).toEqual([])
  // The lever the first start registered is still answered.
  expect((await lever($, w)).result).toBe(LEVER_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.commands).toEqual([LEVER_COMMAND(w)])
})

test('a start whose advisory read outlasts a newer start reading off leaves the advisory inert', async ($, on) => {
  const w = world(on, { ...ON, connect: 1_000 })
  w.engine = $
  void $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
  // A newer start (a hot reload) connects at once and reads the knob off.
  w.compaction = false
  await start($, w)
  // The older start's read, settling last, finds it on.
  w.compaction = true
  await w.clock.advance(1_000)
  expect(w.knobReads.length).toBe(2)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await turnEnd($, w)
  expect(w.prompts).toEqual([])
})

test('a prompt typed within the window starts the first turn, so the resume prompt is not submitted; the typed prompt carries nothing', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS - 1)
  await prompted($, w, 'what I typed meanwhile')
  expect(w.typed).toEqual([{ text: 'what I typed meanwhile', context: undefined }])
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([])
})

test('a slash command or a prompt not typed at the composer that starts a turn within the window cancels the resume prompt too', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await prompted($, w, '/cost', { command: { name: 'cost', args: '' } })
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  await lever($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands.length).toBe(2)
  await prompted($, w, 'from a phone', { kind: 'bridge' })
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual(['from a phone'])
})

test("a background agent's notification turn within the window cancels the resume prompt", async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  // A turn started with no prompt through the composer: a notification, a queued submit.
  await turnStart($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
})

test('a subagent turn started within the window does not cancel the resume prompt', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await turnStart($, w, 'sub-1')
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([resumeText(w.plugin)])
})

test('a prompt typed after the window finds the resume prompt already submitted', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await prompted($, w, 'too late')
  expect(w.prompts).toEqual([resumeText(w.plugin)])
  expect(w.typed).toEqual([{ text: 'too late', context: undefined }])
})

test('a session end inside the window records the held resume prompt in the transcript', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([`${w.plugin} compaction: a prompt was not submitted (session.end clear):\n${resumeText(w.plugin)}`])
})

test('a resume prompt refused beneath the mod is recorded in the transcript', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  w.dropPrompts = 1
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([`${w.plugin} compaction: a prompt was not submitted (dropped: blocked beneath):\n${resumeText(w.plugin)}`])
})

// The caption switch on: each prompt of the feature is delivered split, the
// full text in a hidden row and a raw-text notice, then its one-liner.

/** The settle a split submit waits out before it looks for a started turn. */
const settle = (w: World) => w.clock.advance(OWN_SETTLE_MS)

/** What one split delivery of `text` with its one-liner `line` leaves, in order. */
const split = (text: string, line: string) => [`user:${text}`, `system:${text}`, `submit:${line}`]

test('captions off: the advisory is submitted whole, with no row appended', async ($, on) => {
  const w = world(on, ON)
  await start($, w)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  expect(w.own).toEqual([`submit:${advisoryText(w.plugin, T, 104_000)}`])
})

test('captions on: the advisory and both milestones are delivered split, with no model request', async ($, on) => {
  const w = world(on, { ...ON, captions: true })
  await start($, w)
  await measure($, w, 50_000)
  for (const tokens of [104_000, 128_000, 150_000]) {
    await measure($, w, tokens)
    await settle(w)
  }
  expect(w.own).toEqual([
    ...split(advisoryText(w.plugin, T, 104_000), deliveryLine(w.plugin, { kind: 'advisory' }, T, 104_000)),
    ...split(milestoneText(w.plugin, 1, T, 128_000), deliveryLine(w.plugin, { kind: 'milestone', which: 1 }, T, 128_000)),
    ...split(milestoneText(w.plugin, 2, T, 150_000), deliveryLine(w.plugin, { kind: 'milestone', which: 2 }, T, 150_000)),
  ])
  expect(w.own[2]).toBe(`submit:[${w.plugin} compaction] 52% · auto-compact 85%`)
  expect(w.own[5]).toBe(`submit:[${w.plugin} compaction] 64% · reminder 1/2 · auto-compact 85%`)
  expect(w.completions).toBe(0)
})

test('captions on: the landed and the vetoed resume prompts are delivered split', async ($, on) => {
  const w = world(on, { ...ON, captions: true })
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await settle(w)
  expect(w.own).toEqual(split(resumeText(w.plugin), resumeLine(w.plugin)))
  expect(w.own[2]).toBe(`submit:[${w.plugin} compaction] landed`)
  w.own.length = 0
  await lever($, w, { continue: true })
  w.compactAnswers.push({ skip: 'blocked by policy' })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await settle(w)
  expect(w.own).toEqual(split(vetoedResumeText(w.plugin, 'blocked by policy'), vetoedResumeLine(w.plugin, 'blocked by policy')))
  expect(w.own[2]).toBe(`submit:[${w.plugin} compaction] did not run: blocked by policy`)
  expect(w.completions).toBe(0)
})

test('captions on: a dropped one-liner appends nothing further and is logged to debug only', async ($, on) => {
  const w = world(on, { ...ON, captions: true })
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  w.dropPrompts = 1
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await settle(w)
  expect(w.own).toEqual(split(resumeText(w.plugin), resumeLine(w.plugin)))
  expect(w.prompts).toEqual([])
  expect(transcript(w).filter((l) => l.includes('not submitted'))).toEqual([])
  expect(debug(w).filter((l) => l.includes('one-liner prompt dropped')).length).toBe(1)
})

test('captions on: an advisory whose settle sees a turn start stores nothing, and goes out split after that turn ends', async ($, on) => {
  const w = world(on, { ...ON, captions: true })
  await start($, w)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await turnStart($, w)
  await settle(w)
  expect(w.own).toEqual([])
  w.live = 110_000
  await turnEnd($, w)
  await settle(w)
  expect(w.own).toEqual(split(advisoryText(w.plugin, T, 110_000), deliveryLine(w.plugin, { kind: 'advisory' }, T, 110_000)))
})

test('captions on: a resume prompt whose settle sees a turn start is not submitted', async ($, on) => {
  const w = world(on, { ...ON, captions: true })
  await start($, w)
  await lever($, w, { continue: true })
  await turnEnd($, w)
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  await turnStart($, w)
  await settle(w)
  await turnEnd($, w)
  await settle(w)
  expect(w.own).toEqual([])
  expect(w.prompts).toEqual([])
})

test('captions on: a session end during the settle submits nothing', async ($, on) => {
  const w = world(on, { ...ON, captions: true })
  await start($, w)
  await measure($, w, 50_000)
  await measure($, w, 104_000)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await settle(w)
  expect(w.own).toEqual([])
})

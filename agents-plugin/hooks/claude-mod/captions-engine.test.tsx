import { expect, mock, test, type TestBody } from 'claude-code/testing'

import {
  appendOwn,
  CONTEXT_KEY,
  DEFAULT_STYLE,
  displayToolName,
  answerPrompt,
  ENABLED_KEY,
  fileChange,
  FLUSH_MS,
  inputSubtitle,
  LANG_KEY,
  MODEL_KEY,
  OMITTED_LINE,
  rulesPrompt,
  STYLE_KEY,
  submitOwn,
  type OwnDeliveryEdges,
  type OwnPrompt,
} from './captions'
import { failureLine, REASON_TOOLS_ABSENT, reasonCommandAnswered, successLine } from './reconnect'
import { deliveryLine, resumeLine, thresholdsFor, vetoedResumeLine } from './timing'
import { batchLine, batchPrompt, mailboxLabel, noticeLine } from './watcher'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the tool-call caption feature. The same file is
// mirrored into every package that ships the mod and runs identically in
// each: captions run while their own knob reads true, set through the
// world's config_get answers.

const USAGE = { input_tokens: 1, output_tokens: 1, cache_creation_input_tokens: 0, cache_read_input_tokens: 0 }
/** A resolved call's row: not running, with its output. */
const BASE = { isRunning: false, isErrored: false, isInterrupted: false, output: 'done' }

type Message = { role: 'user' | 'assistant'; content: Array<Record<string, unknown>> }
type Reply = { text: string; tokens?: (prompt: string) => number } | 'unanswered' | 'reject'

function world(on: On, opts: { config?: Record<string, unknown>; failKeys?: string[]; messages?: Message[] } = {}) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    messages: opts.messages ?? ([] as Message[]),
    /** The model.complete requests the mod made. */
    requests: [] as { model: string; prompt: string; effort: unknown; timeoutMs: unknown; blocks: readonly { text: string; cache?: true }[] }[],
    /** The next replies, in order; an empty list answers `[]`. */
    replies: [] as Reply[],
    configKeys: [] as string[],
    compactAnswers: 0,
    /** The events the mod asked the engine to draw again. */
    invalidated: [] as string[],
    /** What tool.call answers for a tool named here; any other tool answers Bash-like output. */
    results: {} as Record<string, { result: unknown; text?: string }>,
    /** The config values config_get answers; a key absent here reads as unset. */
    config: { [ENABLED_KEY]: true, ...(opts.config ?? {}) } as Record<string, unknown>,
  }
  const config = w.config
  on('mcp.connect', ($, e) => ({ value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }))
  on('mcp.call', ($, e) => {
    const key = String(e.args.key)
    if (e.tool !== 'config_get') return { value: { content: [{ type: 'text', text: `no ${e.tool} here` }], isError: true } }
    w.configKeys.push(key)
    if (opts.failKeys?.includes(key)) return { value: { content: [{ type: 'text', text: 'ws-mcp is unhealthy' }], isError: true } }
    const value = key in config ? config[key] : null
    return { value: { content: [{ type: 'text', text: JSON.stringify({ key, value, scope: 'project' }) }], isError: false } }
  })
  on('store.get', () => ({ value: undefined }))
  on('store.set', () => ({ value: undefined }))
  on('store.delete', () => ({ value: undefined }))
  on('ui.log', () => ({ value: undefined }))
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('process.run', () => ({ deny: 'no host commands on this surface' }))
  on('prompt.submit', ($, e) => ({ text: e.text }))
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  on('session.compact', () => ((w.compactAnswers += 1), { messages: [{ role: 'user', text: 'summary', toolUses: [] }] }))
  on('session.messages', () => ({ value: w.messages }))
  on('ui.invalidate', ($, e) => {
    w.invalidated.push(e.event)
    return { value: undefined }
  })
  on('tool.call', ($, e) => w.results[e.tool] ?? ({ result: { stdout: `out of ${String((e as { command?: unknown }).command ?? e.tool)}`, stderr: '', interrupted: false } }) as never)
  on('model.complete', (_, e) => {
    w.requests.push({ model: e.model, prompt: String(e.prompt), effort: e.effort, timeoutMs: e.timeoutMs, blocks: e.promptBlocks ?? [] })
    const reply = w.replies.shift() ?? { text: '[]' }
    if (reply === 'unanswered') return { value: { isAnswered: false, reason: 'empty-reply', usage: USAGE } }
    if (reply === 'reject') throw new Error('gateway down')
    const tokens = reply.tokens?.(String(e.prompt)) ?? 1
    return { value: { isAnswered: true, text: reply.text, usage: { ...USAGE, input_tokens: tokens } } }
  })
  on('ui.render', ($, e) => {
    const { Text } = $.ui.resolve(e)
    return <Text>raw</Text>
  })
  return w
}

type World = ReturnType<typeof world>

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

/** One flush period on, so queued units are sent and their replies stored. */
async function tick(w: World) {
  await w.clock.advance(FLUSH_MS + 100)
}

const toolUse = (w: World, id: string, extra: Record<string, unknown> = {}) =>
  ({
    plugin: w.plugin,
    surface: 'terminal',
    component: 'ToolUse',
    requestId: id,
    props: { ...BASE, tool_use_id: id, tool: 'Bash', input: { command: 'ls' }, ...extra },
  }) as const

const group = (w: World, props: Record<string, unknown>) =>
  ({ plugin: w.plugin, surface: 'terminal', component: 'ToolGroup', requestId: 'g', props }) as const

/** Whether the row draws the engine's own (the test's `raw`) row. */
async function isRaw($: Engine, row: Parameters<Engine['ui']['mount']>[0]) {
  const ui = await $.ui.mount(row)
  const raw = (await ui.find({ type: 'Text', text: /raw/ }))?.text === 'raw'
  await ui.unmount()
  return raw
}

async function find($: Engine, row: Parameters<Engine['ui']['mount']>[0], pattern: RegExp) {
  const ui = await $.ui.mount(row)
  const text = (await ui.find({ type: 'Text', text: pattern }))?.text
  await ui.unmount()
  return text
}

test('a lone call is captioned once drawn, collapsed only, and its result row draws nothing', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"ls","intention":"파일 목록 확인","result":"두 개"}]' })
  // Turning on redraws: rows drawn while the settings were read answered raw.
  expect(w.invalidated).toEqual(['ui.render'])
  w.invalidated.length = 0

  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_1' } as never)
  await tick(w)
  expect(w.requests.length).toBe(0)

  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.requests[0]!.prompt).toContain('out of ls')
  expect(w.requests[0]!.effort).toBe('low')
  expect(w.requests[0]!.timeoutMs).toBe(30_000)
  // A stored caption asks the engine to draw the rows again.
  expect(w.invalidated).toEqual(['ui.render'])

  expect(await find($, toolUse(w, 'toolu_1'), /두 개/)).toContain('두 개')
  expect(await find($, toolUse(w, 'toolu_1'), /파일 목록/)).toBe('파일 목록 확인')
  const result = { plugin: w.plugin, surface: 'terminal', component: 'ToolResult', requestId: 'toolu_1', props: { tool_use_id: 'toolu_1', tool: 'Bash', output: {}, isErrored: false } } as const
  expect(await isRaw($, result)).toBe(false)
  expect(await isRaw($, toolUse(w, 'toolu_1', { isExpanded: true }))).toBe(true)
  expect(await isRaw($, { ...result, props: { ...result.props, isExpanded: true } })).toBe(true)

  // Drawn again, a captioned unit asks for nothing.
  await tick(w)
  expect(w.requests.length).toBe(1)
})

test('a finished run is captioned as one unit in one request; a live one stays raw', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitles":["parse.go","go test"],"summary":"파서 진입점 수정, 테스트 실패"}]' })

  await $.tool.call({ tool: 'Read', file_path: 'parse.go', tool_use_id: 'toolu_r1' } as never)
  await $.tool.call({ tool: 'Bash', command: 'go test', tool_use_id: 'toolu_r2' } as never)
  const calls = [
    { ...BASE, tool_use_id: 'toolu_r1', tool: 'Read', input: { file_path: 'parse.go' } },
    { ...BASE, tool_use_id: 'toolu_r2', tool: 'Bash', input: { command: 'go test' }, isErrored: true },
  ]
  const finished = { calls, isActive: false, isExpanded: false, isCallsOnly: false }

  expect(await isRaw($, group(w, { ...finished, isActive: true }))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(0)

  // A mixed group (other rows, no failed call) stays raw too.
  expect(await isRaw($, group(w, { ...finished, calls: calls.map((c) => ({ ...c, isErrored: false })) }))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(0)

  expect(await isRaw($, group(w, finished))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.requests[0]!.prompt).toContain('"id":"u1","calls":[{"tool":"Read"')
  // The prompt closes on the answer each unit takes.
  expect(w.requests[0]!.prompt.endsWith(`\n\n${answerPrompt([{ label: 'u1', calls: 2 }])}`)).toBe(true)

  expect(await find($, group(w, finished), /테스트 실패/)).toContain('파서 진입점 수정')
  expect(await find($, group(w, finished), /go test/)).toContain('go test')
  expect(await find($, group(w, finished), /2 calls/)).toContain('2 calls')
  expect(await find($, group(w, finished), /✗/)).toBe('✗ ')
  expect(await isRaw($, group(w, { ...finished, isExpanded: true }))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests.length).toBe(1)
})

test('the prompt carries the conversation and pivots past the window', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'find the parser' }] },
    { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_c1', name: 'Grep', input: { pattern: 'parse' } }] },
  ]
  const w = world(on, { messages, config: { [CONTEXT_KEY]: 2000 } })
  await start($, w)
  const reply = { text: '[]', tokens: (prompt: string) => Math.round(prompt.length / 2) }
  w.replies.push(reply, reply)

  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_c1' } as never)
  await isRaw($, toolUse(w, 'toolu_c1'))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('find the parser')
  expect(w.requests[0]!.prompt).toContain('LATEST MESSAGE')
  // Each history block ends apart from the next.
  expect(w.requests[0]!.prompt).toContain('[user]\nfind the parser\n\nLATEST MESSAGE (may be incomplete)\n[assistant]')
  // The rules open the prompt with a cache mark; the last history block carries the other.
  expect(w.requests[0]!.blocks[0]).toEqual({ text: `${rulesPrompt(DEFAULT_STYLE, undefined)}\n\n`, cache: true })
  expect(w.requests[0]!.blocks.filter((b) => b.cache).length).toBe(2)

  const big = (role: 'user' | 'assistant', mark: string): Message => ({ role, content: [{ type: 'text', text: `${mark} ${'x'.repeat(1500)}` }] })
  messages.push(
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_c1', content: 'parse.go' }] },
    big('assistant', 'big1'),
    big('user', 'big2'),
    big('assistant', 'big3'),
    big('user', 'big4'),
  )
  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_c2' } as never)
  await isRaw($, toolUse(w, 'toolu_c2'))
  await tick(w)
  expect(w.requests.length).toBe(2)
  expect(w.requests[1]!.prompt).toContain(OMITTED_LINE)
  expect(w.requests[1]!.prompt).not.toContain('find the parser')
  expect(w.requests[1]!.prompt).toContain('big3')
})

test('the history prefix stays the same between requests until a pivot', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'one' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'two' }] },
  ]
  const w = world(on, { messages })
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_p1'))
  await tick(w)
  messages.push({ role: 'user', content: [{ type: 'text', text: 'three' }] }, { role: 'assistant', content: [{ type: 'text', text: 'four' }] })
  await isRaw($, toolUse(w, 'toolu_p2'))
  await tick(w)
  expect(w.requests.length).toBe(2)
  const [first, second] = [w.requests[0]!.blocks.map((b) => b.text), w.requests[1]!.blocks.map((b) => b.text)]
  // All but the uncached tail opens the next request byte for byte.
  expect(second.slice(0, first.length - 1)).toEqual(first.slice(0, -1))
  expect(second.length).toBe(first.length + 2)
})

test('the measured tokens per character move the pivot', async ($, on) => {
  const messages: Message[] = [{ role: 'user', content: [{ type: 'text', text: 'cal start' }] }]
  const w = world(on, { messages, config: { [CONTEXT_KEY]: 2000 } })
  await start($, w)
  // Four tokens per character: far above the 0.5 the mirror assumes before a reply.
  const reply = { text: '[]', tokens: (prompt: string) => prompt.length * 4 }
  w.replies.push(reply, reply)
  await isRaw($, toolUse(w, 'toolu_k1'))
  await tick(w)
  expect(w.requests[0]!.prompt).not.toContain(OMITTED_LINE)

  const mid = (role: 'user' | 'assistant', mark: string): Message => ({ role, content: [{ type: 'text', text: `${mark} ${'y'.repeat(300)}` }] })
  messages.push(mid('assistant', 'mid1'), mid('user', 'mid2'), mid('assistant', 'mid3'), mid('user', 'mid4'))
  // About 1000 history characters: 500 tokens at the assumed rate (no pivot), 4000 at the measured one.
  await isRaw($, toolUse(w, 'toolu_k2'))
  await tick(w)
  expect(w.requests[1]!.prompt).toContain(OMITTED_LINE)
  expect(w.requests[1]!.prompt).not.toContain('cal start')
})

test('a transcript shorter than the mirror read starts the mirror over', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'alpha old' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'beta old' }] },
    { role: 'user', content: [{ type: 'text', text: 'gamma old' }] },
  ]
  const w = world(on, { messages })
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_s1'))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('alpha old')
  messages.splice(0, messages.length, { role: 'user', content: [{ type: 'text', text: 'delta fresh' }] })
  await isRaw($, toolUse(w, 'toolu_s2'))
  await tick(w)
  expect(w.requests[1]!.prompt).toContain('delta fresh')
  expect(w.requests[1]!.prompt).not.toContain('alpha old')
})

test('a call captured before a reload takes its output and size from the transcript', async ($, on) => {
  const w = world(on, {
    messages: [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_old', name: 'Bash', input: { command: 'ls' } }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_old', content: 'x'.repeat(2048) }] },
    ],
  })
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"ls","intention":"목록 확인","result":"출력 있음"}]' })

  await isRaw($, toolUse(w, 'toolu_old'))
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.requests[0]!.prompt).not.toContain('(not available)')
  expect(await find($, toolUse(w, 'toolu_old'), /KB/)).toContain('2.0 KB')
})

test('a transcript result of blocks counts and sends their text, in the history too, and marks a result with none', async ($, on) => {
  const w = world(on, {
    messages: [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_tx', name: 'Bash', input: { command: 'ls' } }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_tx', content: [{ type: 'text', text: 'x'.repeat(2048) }] }] },
      { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_img', name: 'Read', input: { file_path: '/a.png' } }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_img', content: [{ type: 'image', source: { type: 'base64', data: 'QUJD' } }] }] },
    ],
  })
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"ls","intention":"목록 확인","result":"출력 있음"}]' })

  await isRaw($, toolUse(w, 'toolu_tx'))
  await tick(w)
  expect(w.requests[0]!.prompt).not.toContain('"type":"text"')
  expect(await find($, toolUse(w, 'toolu_tx'), /KB/)).toContain('2.0 KB')

  await isRaw($, toolUse(w, 'toolu_img', { tool: 'Read', input: { file_path: '/a.png' } }))
  await tick(w)
  expect(w.requests[1]!.prompt).toContain('(no text output)')
  expect(w.requests[1]!.prompt).not.toContain('QUJD')
})

test('a call in neither the capture nor the transcript sends its drawn input and no output', async ($, on) => {
  const w = world(on)
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_gone', { input: { command: 'make lint' } }))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('make lint')
  expect(w.requests[0]!.prompt).toContain('(not available)')
})

test('captions are off by default: rows draw raw, nothing is sent, and the other settings are not read', async ($, on) => {
  const w = world(on)
  delete w.config[ENABLED_KEY]
  await start($, w)
  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_1' } as never)
  const calls = [{ ...BASE, tool_use_id: 'toolu_1', tool: 'Bash', input: { command: 'ls' } }]
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  expect(await isRaw($, group(w, { calls, isActive: false, isExpanded: false, isCallsOnly: true }))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests).toEqual([])
  expect(w.configKeys).toContain(ENABLED_KEY)
  for (const key of [MODEL_KEY, STYLE_KEY, CONTEXT_KEY, LANG_KEY]) expect(w.configKeys).not.toContain(key)
})

test('a read failure of the switch leaves captions off', async ($, on) => {
  const w = world(on, { failKeys: [ENABLED_KEY] })
  await start($, w)
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  expect(w.requests).toEqual([])
})

test('the model, style, language and window settings are applied', async ($, on) => {
  const style = 'Style: one word per field.'
  const w = world(on, { config: { [MODEL_KEY]: 'claude-sonnet-x', [STYLE_KEY]: style, [LANG_KEY]: 'Korean', [CONTEXT_KEY]: 50_000 } })
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests[0]!.model).toBe('claude-sonnet-x')
  const rules = w.requests[0]!.blocks[0]!.text.trimEnd()
  expect(rules).toBe(rulesPrompt(style, 'Korean'))
  expect(rules).toContain('Write intention, result, summary and recap in Korean only.')
  expect(rules.split('\n').at(-1)).toBe(style)
  expect(rules).not.toContain(DEFAULT_STYLE)
})

test('failed or invalid setting reads leave the defaults, and an unset language follows the user', async ($, on) => {
  // A few settled messages: a window of -3 tokens would pivot past them all.
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'first note' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'second note' }] },
    { role: 'user', content: [{ type: 'text', text: 'third note' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'fourth note' }] },
  ]
  const w = world(on, { messages, failKeys: [STYLE_KEY, MODEL_KEY], config: { [CONTEXT_KEY]: -3, [LANG_KEY]: '' } })
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests[0]!.model).toBe('haiku')
  expect(w.requests[0]!.prompt).toContain('first note')
  expect(w.requests[0]!.prompt).not.toContain(OMITTED_LINE)
  const rules = w.requests[0]!.blocks[0]!.text.trimEnd()
  expect(rules).toBe(rulesPrompt(DEFAULT_STYLE, undefined))
  expect(rules).toContain('Write intention, result, summary and recap in the language the user writes in.')
  expect(rules.split('\n').at(-1)).toBe(DEFAULT_STYLE)
})

test('a later session start in the same module stops the old ticker and reads the settings again', async ($, on) => {
  const w = world(on)
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests.length).toBe(1)

  // A tune between sessions: the next start reads the new model.
  w.config[MODEL_KEY] = 'claude-sonnet-x'
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_2'))
  await tick(w)
  expect(w.requests.length).toBe(2)
  expect(w.requests[1]!.model).toBe('claude-sonnet-x')

  // And a switch turned off takes the rows back to raw.
  w.config[ENABLED_KEY] = false
  await start($, w)
  expect(await isRaw($, toolUse(w, 'toolu_3'))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests.length).toBe(2)
})

test('the consent text of a call is not sent as its input', async ($, on) => {
  const w = world(on)
  await start($, w)
  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_k', consent: 'pressed Run in the pane' } as never)
  await isRaw($, toolUse(w, 'toolu_k'))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('out of ls')
  expect(w.requests[0]!.prompt).not.toContain('pressed Run')
})

test('a failed request is not retried', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push('unanswered')
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests.length).toBe(1)
})

test('a rejected request leaves its unit raw, and later units still flush', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push('reject', { text: '[{"id":"u1","subtitle":"pwd","intention":"위치 확인","result":"/tmp"}]' })
  await isRaw($, toolUse(w, 'toolu_x1'))
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(await isRaw($, toolUse(w, 'toolu_x1'))).toBe(true)
  await isRaw($, toolUse(w, 'toolu_x2'))
  await tick(w)
  expect(w.requests.length).toBe(2)
  expect(w.requests[1]!.prompt).not.toContain('toolu_x1')
  expect(await find($, toolUse(w, 'toolu_x2'), /위치/)).toBe('위치 확인')
})

test('a request holds at most 24 calls and never splits a unit', async ($, on) => {
  const w = world(on)
  await start($, w)
  const callsIn = (prompt: string) => prompt.split('"failed":').length - 1
  for (let i = 1; i <= 25; i += 1) await isRaw($, toolUse(w, `toolu_b${i}`))
  await tick(w)
  expect(callsIn(w.requests[0]!.prompt)).toBe(24)
  expect(w.requests[0]!.prompt).toContain('"id":"u24"')
  expect(w.requests[0]!.prompt).not.toContain('"id":"u25"')
  await tick(w)
  expect(callsIn(w.requests[1]!.prompt)).toBe(1)

  const run = (prefix: string, n: number) => ({
    calls: Array.from({ length: n }, (_, i) => ({ ...BASE, tool_use_id: `toolu_${prefix}${i}`, tool: 'Bash', input: { command: 'ls' } })),
    isActive: false,
    isExpanded: false,
    isCallsOnly: true,
  })
  await isRaw($, group(w, run('s', 20)))
  await isRaw($, group(w, run('t', 10)))
  await tick(w)
  expect(callsIn(w.requests[2]!.prompt)).toBe(20)
  await tick(w)
  expect(callsIn(w.requests[3]!.prompt)).toBe(10)
})

test('a reply keeps only its well-formed items; the rest stay raw and are not asked again', async ($, on) => {
  const w = world(on)
  await start($, w)
  // Prose and a fence around the array; u2 answers a lone call with a run's
  // fields, u4 a run without its summary, u3 is absent, u9 names no unit.
  w.replies.push({
    text: 'Here:\n```json\n[{"id":"u1","subtitle":"ls","intention":"목록 확인","result":"세 개"},{"id":"u2","subtitles":["x"],"summary":"wrong shape"},{"id":"u4","subtitles":["a","b"]},{"id":"u9","intention":"x","result":"y"}]\n```',
  })
  const run = {
    calls: [
      { ...BASE, tool_use_id: 'toolu_m4', tool: 'Bash', input: { command: 'a' } },
      { ...BASE, tool_use_id: 'toolu_m5', tool: 'Bash', input: { command: 'b' } },
    ],
    isActive: false,
    isExpanded: false,
    isCallsOnly: true,
  }
  for (const id of ['toolu_m1', 'toolu_m2', 'toolu_m3']) await isRaw($, toolUse(w, id))
  await isRaw($, group(w, run))
  const before = w.invalidated.length
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.invalidated.length).toBe(before + 1)
  expect(await find($, toolUse(w, 'toolu_m1'), /세 개/)).toContain('세 개')
  expect(await isRaw($, toolUse(w, 'toolu_m2'))).toBe(true)
  expect(await isRaw($, toolUse(w, 'toolu_m3'))).toBe(true)
  expect(await isRaw($, group(w, run))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests.length).toBe(1)
})

test('a row drawn before its call has run stays raw and unsent, then is captioned once it resolves', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"a.txt","intention":"파일 작성","result":"작성됨"}]' })
  // Input still streaming or a permission prompt up: not running, no output.
  const prerun = toolUse(w, 'toolu_w', { tool: 'Write', input: {}, output: undefined })
  expect(await isRaw($, prerun)).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(0)
  expect(await isRaw($, toolUse(w, 'toolu_w', { tool: 'Write', input: {}, isRunning: true, output: undefined }))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(0)

  await $.tool.call({ tool: 'Write', file_path: 'a.txt', content: 'hi', tool_use_id: 'toolu_w' } as never)
  const resolved = toolUse(w, 'toolu_w', { tool: 'Write', input: { file_path: 'a.txt', content: 'hi' } })
  expect(await isRaw($, resolved)).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.requests[0]!.prompt).toContain('a.txt')
  expect(w.requests[0]!.prompt).not.toContain('(not available)')
  expect(await find($, resolved, /작성됨/)).toContain('작성됨')
})

test('a group holding a call that has not run stays raw and unsent until every call resolves', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitles":["ls","a.txt"],"summary":"목록 확인 후 파일 수정"}]' })
  const done = { ...BASE, tool_use_id: 'toolu_g1', tool: 'Bash', input: { command: 'ls' } }
  const pending = { ...BASE, tool_use_id: 'toolu_g2', tool: 'Edit', input: {}, output: undefined }
  const props = { isActive: false, isExpanded: false, isCallsOnly: true }

  expect(await isRaw($, group(w, { ...props, calls: [done, pending] }))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(0)

  const calls = [done, { ...pending, input: { file_path: 'a.txt' }, output: 'edited' }]
  expect(await isRaw($, group(w, { ...props, calls }))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(await find($, group(w, { ...props, calls }), /파일 수정/)).toContain('목록 확인 후 파일 수정')
})

test('a subtitle the reply leaves blank is read off the call input', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"","intention":"시각 확인","result":"22:25:35"},{"id":"u2","subtitles":["  ","b.txt"],"summary":"두 파일 확인"}]' })
  await isRaw($, toolUse(w, 'toolu_d', { input: { command: 'date +%T', description: 'Print current time' } }))
  const calls = [
    { ...BASE, tool_use_id: 'toolu_g1', tool: 'Bash', input: { command: 'wc -c a.txt' } },
    { ...BASE, tool_use_id: 'toolu_g2', tool: 'Read', input: { file_path: 'b.txt' } },
  ]
  const finished = { calls, isActive: false, isExpanded: false, isCallsOnly: true }
  await isRaw($, group(w, finished))
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(await find($, toolUse(w, 'toolu_d', { input: { command: 'date +%T', description: 'Print current time' } }), /date/)).toContain('date +%T')
  expect(await find($, group(w, finished), /wc -c/)).toContain('wc -c a.txt')
  expect(await find($, group(w, finished), /b\.txt/)).toContain('b.txt')
})

test('the mirror starts over when the lead compaction lever lands, and on a main-thread compaction', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'alpha before compaction' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'beta before compaction' }] },
    { role: 'user', content: [{ type: 'text', text: 'gamma latest' }] },
  ]
  const w = world(on, { messages })
  await start($, w)
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('alpha before compaction')

  // The lever compacts at the main-thread turn end; the transcript is then
  // the summary and what followed, no shorter than what the mirror read.
  await $.tool.call({ tool: `mcp__${w.plugin}__request_compaction` } as never)
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.settle()
  expect(w.compactAnswers).toBe(1)
  messages.splice(0, messages.length,
    { role: 'user', content: [{ type: 'text', text: 'SUMMARY one' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'after one' }] },
    { role: 'user', content: [{ type: 'text', text: 'after two' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'after three' }] },
  )
  await isRaw($, toolUse(w, 'toolu_2'))
  await tick(w)
  expect(w.requests[1]!.prompt).toContain('SUMMARY one')
  expect(w.requests[1]!.prompt).not.toContain('alpha before compaction')

  await $.session.compact({ trigger: 'manual', messages: [{ role: 'user', text: 'summary', toolUses: [] }] } as never)
  await w.clock.settle()
  messages.splice(0, messages.length,
    { role: 'user', content: [{ type: 'text', text: 'SUMMARY two' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'later one' }] },
    { role: 'user', content: [{ type: 'text', text: 'later two' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'later three' }] },
    { role: 'user', content: [{ type: 'text', text: 'later four' }] },
  )
  await isRaw($, toolUse(w, 'toolu_3'))
  await tick(w)
  expect(w.requests[2]!.prompt).toContain('SUMMARY two')
  expect(w.requests[2]!.prompt).not.toContain('SUMMARY one')
})

test('session.end starts the mirror over and keeps the captions', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'alpha old conversation' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'beta old conversation' }] },
  ]
  const w = world(on, { messages })
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"ls","intention":"목록 확인","result":"두 개"}]' })
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('alpha old conversation')

  // After /clear no session.start follows, and the new conversation is
  // already longer than the old one, so only the end tells the mirror.
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  messages.splice(0, messages.length,
    { role: 'user', content: [{ type: 'text', text: 'new one' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'new two' }] },
    { role: 'user', content: [{ type: 'text', text: 'new three' }] },
  )
  await isRaw($, toolUse(w, 'toolu_2'))
  await tick(w)
  expect(w.requests.length).toBe(2)
  expect(w.requests[1]!.prompt).toContain('new one')
  expect(w.requests[1]!.prompt).not.toContain('alpha old conversation')
  expect(await find($, toolUse(w, 'toolu_1'), /두 개/)).toContain('두 개')
})

test('a caption row names an MCP tool as the server spells it', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.replies.push({ text: '[{"id":"u1","subtitle":"release lease","intention":"lease","result":"ok"}]' })
  const row = toolUse(w, 'toolu_m', { tool: 'mcp__plugin_ws_ws__tickets_release', input: {} })
  await isRaw($, row)
  await tick(w)
  expect(await find($, row, /plugin:ws:ws/)).toBe('plugin:ws:ws/tickets.release  release lease')
})

test('a file edit sends its change, not the original file, and its size is what the lead read', async ($, on) => {
  const w = world(on)
  await start($, w)
  const original = `${'filler line\n'.repeat(400)}const FLUSH_MS = 1500\n`
  const text = 'The file timing.ts has been updated successfully.'
  w.results.Edit = {
    result: {
      filePath: 'timing.ts',
      oldString: 'const FLUSH_MS = 1500',
      newString: 'const FLUSH_MS = 1000',
      originalFile: original,
      structuredPatch: [{ oldStart: 401, oldLines: 1, newStart: 401, newLines: 1, lines: ['-const FLUSH_MS = 1500', '+const FLUSH_MS = 1000'] }],
      userModified: false,
      replaceAll: false,
    },
    text,
  }
  w.replies.push({ text: '[{"id":"u1","subtitle":"timing.ts","intention":"주기 단축","result":"FLUSH_MS 1500 -> 1000, +1 -1"}]' })
  const input = { file_path: 'timing.ts', old_string: 'const FLUSH_MS = 1500', new_string: 'const FLUSH_MS = 1000' }
  await $.tool.call({ tool: 'Edit', ...input, tool_use_id: 'toolu_e' } as never)
  const row = toolUse(w, 'toolu_e', { tool: 'Edit', input })
  await isRaw($, row)
  await tick(w)
  const prompt = w.requests[0]!.prompt
  expect(prompt).toContain('edit timing.ts  +1 -1')
  expect(prompt).toContain('+const FLUSH_MS = 1000')
  expect(prompt).not.toContain('filler line')
  expect(prompt).toContain('Never only "file updated"')
  const size = JSON.stringify(input).length + text.length
  expect(await find($, row, /KB/)).toContain(`${(size / 1024).toFixed(1)} KB`)
})

test('file changes: an edit, a new file and an overwrite read as the change; other results are not one', () => {
  const hunk = { oldStart: 3, oldLines: 2, newStart: 3, newLines: 2, lines: [' keep', '-old', '+new'] }
  expect(fileChange({ filePath: 'a.go', oldString: 'old', newString: 'new', originalFile: 'x', structuredPatch: [hunk], userModified: false, replaceAll: true })).toBe(
    'edit a.go  +1 -1 (all occurrences)\n@@ -3,2 +3,2 @@\n keep\n-old\n+new',
  )
  expect(fileChange({ type: 'create', filePath: 'n.md', content: 'one\ntwo\nthree\n', structuredPatch: [], originalFile: null })).toBe('create n.md  +3 -0')
  expect(fileChange({ type: 'update', filePath: 'n.md', content: 'new', structuredPatch: [hunk], originalFile: 'x', userModified: true })).toBe(
    'update n.md  +1 -1 (user modified)\n@@ -3,2 +3,2 @@\n keep\n-old\n+new',
  )
  expect(fileChange({ stdout: 'ok', stderr: '' })).toBeUndefined()
  expect(fileChange('plain text')).toBeUndefined()
  expect(fileChange({ type: 'create', filePath: '/e', content: '', structuredPatch: [] })).toBe('create /e  +0 -0')
})

test('the closing lines name each unit and the fields its answer takes', () => {
  expect(answerPrompt([{ label: 'u1', calls: 1 }, { label: 'u2', calls: 3 }, { label: 'u3', calls: 1, isMail: true }])).toBe(
    [
      'Answer now: only a JSON array holding exactly these objects, ids verbatim, no prose or code fence.',
      'u1: lone call -> {id, subtitle, intention, result}',
      'u2: run of 3 calls -> {id, subtitles (3), summary}',
      'u3: mail -> {id, recap}',
    ].join('\n'),
  )
})

test('input subtitles: the first target field, its first line, the middle cut past 60 characters', () => {
  expect(inputSubtitle({ command: 'date +%T', description: 'Print current time' })).toBe('date +%T')
  expect(inputSubtitle({ file_path: '/a/b.ts', command: 'x' })).toBe('/a/b.ts')
  expect(inputSubtitle({ command: 'git status\ngit log' })).toBe('git status')
  expect(inputSubtitle({ description: 'Print current time' })).toBe('Print current time')
  const long = `/very/long/${'d/'.repeat(40)}captions.tsx`
  const cut = inputSubtitle({ file_path: long })
  expect(cut.length).toBe(60)
  expect(cut.endsWith('captions.tsx')).toBe(true)
  expect(cut).toContain('…')
  expect(inputSubtitle({ session_key: 'k' })).toBe('')
  expect(inputSubtitle({ command: '   ' })).toBe('')
  expect(inputSubtitle('ls')).toBe('')
})

test('display tool names: plugin MCP tools, other MCP servers, and built-ins', () => {
  expect(displayToolName('mcp__plugin_ws_ws__tickets_release')).toBe('plugin:ws:ws/tickets.release')
  expect(displayToolName('mcp__plugin_wsflow_wsflow__git_status')).toBe('plugin:wsflow:wsflow/git.status')
  expect(displayToolName('mcp__github__get_issue')).toBe('github/get.issue')
  expect(displayToolName('mcp__ws__note_write')).toBe('ws/note.write')
  expect(displayToolName('Bash')).toBe('Bash')
})

// The mod's own prompts, delivered split while captions are on. Engine-level
// delivery is covered beside each feature's own engine tests.

/** A window whose advisory point is 50% and auto-compact point 89% (reminders at 63% and 76%). */
const WINDOW = thresholdsFor({ rawMaxTokens: 200_000, autoCompactThreshold: 178_000, isAutoCompactEnabled: true }, { percent: 50 })!
const RECV = 'mcp__plugin_x_x__mailbox_recv'

/** Fake split-delivery edges: every append, submit, line and recap in one ordered list. */
function ownEdges(opts: { on?: boolean; submit?: 'drop' | 'reject'; refuse?: Array<'user' | 'system'> } = {}) {
  const events: string[] = []
  const recaps: Array<{ tag: string; items: unknown[] }> = []
  const refuse = [...(opts.refuse ?? [])]
  const edges: OwnDeliveryEdges = {
    isOn: () => opts.on ?? true,
    append: async (type, text) => {
      if (refuse[0] === type) {
        refuse.shift()
        throw new Error('row refused: policy')
      }
      events.push(`${type}:${text}`)
    },
    submit: async (text) => {
      events.push(`submit:${text}`)
      if (opts.submit === 'reject') throw new Error('no session')
      return opts.submit === 'drop' ? { drop: 'a Stop stands' } : { text }
    },
    line: (text) => void events.push(`line:${text}`),
    recap: (mail) => void recaps.push(mail),
    log: (text) => void events.push(`log:${text}`),
  }
  return { edges, events, recaps }
}

const fixed = (text: string, line: string): OwnPrompt => ({ text, line })

test('captions off: one submit of the full text, or one row appended, as before', async () => {
  const off = ownEdges({ on: false })
  const mail = batchPrompt('p mailbox', RECV, 'k', [{ from: 'alpha', content: 'hi' }])
  expect(await submitOwn(off.edges, [fixed('A', 'a'), mail])).toEqual({ text: `A\n\n${mail.text}` })
  await appendOwn(off.edges, [fixed('B', 'b')])
  expect(off.events).toEqual([`submit:A\n\n${mail.text}`, 'user:B'])
  expect(off.recaps).toEqual([])
  // A drop is the caller's to handle, as before.
  expect(await submitOwn(ownEdges({ on: false, submit: 'drop' }).edges, [fixed('A', 'a')])).toEqual({ drop: 'a Stop stands' })
})

test('captions on: the hidden row, then its raw-text notice, then the one-liner submit; each batch queued for a recap', async () => {
  const on = ownEdges()
  const mail = batchPrompt('p mailbox', RECV, 'k', [{ from: 'alpha', content: 'hi' }])
  const text = `A\n\n${mail.text}`
  expect(await submitOwn(on.edges, [fixed('A', 'a'), mail])).toEqual({})
  expect(on.events).toEqual([`user:${text}`, `system:${text}`, `submit:a\n${mail.line}`])
  expect(on.recaps).toEqual([mail.mail])
})

test('captions on: a dropped or rejected one-liner submit appends nothing further and logs only', async () => {
  for (const submit of ['drop', 'reject'] as const) {
    const on = ownEdges({ submit })
    expect(await submitOwn(on.edges, [fixed('A', 'a')])).toEqual({})
    expect(on.events.slice(0, 3)).toEqual(['user:A', 'system:A', 'submit:a'])
    expect(on.events.length).toBe(4)
    expect(on.events[3]).toMatch(/^log:one-liner prompt/)
  }
})

test('captions on: a refused hidden row falls back to the full-text submit; a refused notice is logged and the rest goes on', async () => {
  const noRow = ownEdges({ refuse: ['user'], submit: 'drop' })
  expect(await submitOwn(noRow.edges, [fixed('A', 'a')])).toEqual({ drop: 'a Stop stands' })
  expect(noRow.events.length).toBe(2)
  expect(noRow.events[0]).toMatch(/^log:hidden row refused/)
  expect(noRow.events[1]).toBe('submit:A')
  expect(noRow.recaps).toEqual([])
  const noNotice = ownEdges({ refuse: ['system'] })
  await submitOwn(noNotice.edges, [fixed('A', 'a')])
  expect(noNotice.events.length).toBe(3)
  expect(noNotice.events[0]).toBe('user:A')
  expect(noNotice.events[1]).toMatch(/^log:raw-text notice not appended/)
  expect(noNotice.events[2]).toBe('submit:a')
})

test('captions on, mid-turn: the hidden row and its notice, no submit, and each one-liner drawn as a transcript line', async () => {
  const on = ownEdges()
  const one = batchPrompt('p mailbox', RECV, 'k', [{ from: 'alpha', content: 'x' }])
  const two = batchPrompt('p mailbox', RECV, 'k', [{ from: 'beta', content: 'y' }])
  const text = `${one.text}\n\n${two.text}`
  await appendOwn(on.edges, [one, two])
  expect(on.events).toEqual([`user:${text}`, `system:${text}`, `line:${one.line}`, `line:${two.line}`])
  expect(on.recaps).toEqual([one.mail, two.mail])
  // A refused row rejects, having drawn and queued nothing.
  const refused = ownEdges({ refuse: ['user'] })
  expect(await appendOwn(refused.edges, [one]).then(() => 'stored', (err: unknown) => String(err))).toBe('Error: row refused: policy')
  expect(refused.events).toEqual([])
  expect(refused.recaps).toEqual([])
})

test('the one-liners, built from the inputs their templates take', () => {
  const p = 'pkg'
  expect(deliveryLine(p, { kind: 'advisory' }, WINDOW, 124_000)).toBe('[pkg compaction] 62% · auto-compact 89%')
  expect(deliveryLine(p, { kind: 'milestone', which: 1 }, WINDOW, 148_000)).toBe('[pkg compaction] 74% · reminder 1/2 · auto-compact 89%')
  expect(deliveryLine(p, { kind: 'milestone', which: 2 }, WINDOW, 154_000)).toBe('[pkg compaction] 77% · reminder 2/2 · auto-compact 89%')
  expect(resumeLine(p)).toBe('[pkg compaction] landed')
  expect(vetoedResumeLine(p, 'a hook said no')).toBe('[pkg compaction] did not run: a hook said no')
  expect(successLine(p)).toBe('[pkg mcp] reconnected')
  expect(failureLine(p, REASON_TOOLS_ABSENT)).toBe(`[pkg mcp] failed: ${REASON_TOOLS_ABSENT}`)
  expect(failureLine(p, reasonCommandAnswered('no server'))).toBe(`[pkg mcp] failed: ${reasonCommandAnswered('no server')}`)
  const label = mailboxLabel(p)
  expect(noticeLine(label)).toBe('[pkg mailbox] mail waiting')
  expect(batchLine(label, [{ from: 'a@m', content: '1' }, { reply_to: 'id:b', content: '2' }, { from: 'a@m', content: '3' }])).toBe(
    '[pkg mailbox] 3 messages from a@m, id:b',
  )
  expect(batchLine(label, [{ raw: 'garbage' }])).toBe('[pkg mailbox] 1 message from unparsed recv output')
})

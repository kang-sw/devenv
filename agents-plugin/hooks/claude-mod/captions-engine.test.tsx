import { expect, mock, test, type TestBody } from 'claude-code/testing'

import {
  CONTEXT_KEY,
  DEFAULT_STYLE,
  displayToolName,
  ENABLED_KEY,
  FLUSH_MS,
  LANG_KEY,
  MODEL_KEY,
  OMITTED_LINE,
  rulesPrompt,
  STYLE_KEY,
} from './captions'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the tool-call caption feature. The same file is
// mirrored into every package that ships the mod, and the kit has no
// plugin-name override, so each test reads the package identity from the
// registrations: the ws tests return early elsewhere, and the inert-package
// test early in ws.

const USAGE = { input_tokens: 1, output_tokens: 1, cache_creation_input_tokens: 0, cache_read_input_tokens: 0 }
const BASE = { isRunning: false, isErrored: false, isInterrupted: false }

type Message = { role: 'user' | 'assistant'; content: Array<Record<string, unknown>> }
type Reply = { text: string; tokens?: (prompt: string) => number } | 'unanswered'

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
  }
  const config = { [ENABLED_KEY]: true, ...(opts.config ?? {}) }
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
  on('tool.call', ($, e) => ({ result: { stdout: `out of ${String((e as { command?: unknown }).command ?? e.tool)}`, stderr: '', interrupted: false } }))
  on('model.complete', (_, e) => {
    w.requests.push({ model: e.model, prompt: String(e.prompt), effort: e.effort, timeoutMs: e.timeoutMs, blocks: e.promptBlocks ?? [] })
    const reply = w.replies.shift() ?? { text: '[]' }
    if (reply === 'unanswered') return { value: { isAnswered: false, reason: 'empty-reply', usage: USAGE } }
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

const isWs = (w: World) => w.plugin === 'ws'

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

test('ws: a lone call is captioned once drawn, collapsed only, and its result row draws nothing', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  w.replies.push({ text: '[{"id":"u1","subtitle":"ls","intention":"파일 목록 확인","result":"두 개"}]' })

  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_1' } as never)
  await tick(w)
  expect(w.requests.length).toBe(0)

  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.requests[0]!.prompt).toContain('out of ls')
  expect(w.requests[0]!.effort).toBe('low')
  expect(w.requests[0]!.timeoutMs).toBe(30_000)

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

test('ws: a finished run is captioned as one unit in one request; a live one stays raw', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
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

  expect(await find($, group(w, finished), /테스트 실패/)).toContain('파서 진입점 수정')
  expect(await find($, group(w, finished), /go test/)).toContain('go test')
  expect(await find($, group(w, finished), /2 calls/)).toContain('2 calls')
  expect(await find($, group(w, finished), /✗/)).toBe('✗ ')
  expect(await isRaw($, group(w, { ...finished, isExpanded: true }))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests.length).toBe(1)
})

test('ws: the prompt carries the conversation and pivots past the window', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'find the parser' }] },
    { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_c1', name: 'Grep', input: { pattern: 'parse' } }] },
  ]
  const w = world(on, { messages, config: { [CONTEXT_KEY]: 2000 } })
  await start($, w)
  if (!isWs(w)) return
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

test('ws: a call captured before a reload takes its output and size from the transcript', async ($, on) => {
  const w = world(on, {
    messages: [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_old', name: 'Bash', input: { command: 'ls' } }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_old', content: 'x'.repeat(2048) }] },
    ],
  })
  await start($, w)
  if (!isWs(w)) return
  w.replies.push({ text: '[{"id":"u1","subtitle":"ls","intention":"목록 확인","result":"출력 있음"}]' })

  await isRaw($, toolUse(w, 'toolu_old'))
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(w.requests[0]!.prompt).not.toContain('(not available)')
  expect(await find($, toolUse(w, 'toolu_old'), /KB/)).toContain('2.0 KB')
})

test('ws: a call in neither the capture nor the transcript sends its drawn input and no output', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  await isRaw($, toolUse(w, 'toolu_gone', { input: { command: 'make lint' } }))
  await tick(w)
  expect(w.requests[0]!.prompt).toContain('make lint')
  expect(w.requests[0]!.prompt).toContain('(not available)')
})

test('captions are off by default: rows draw raw, nothing is sent, and the other settings are not read', async ($, on) => {
  const w = world(on, { config: { [ENABLED_KEY]: false } })
  await start($, w)
  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_1' } as never)
  const calls = [{ ...BASE, tool_use_id: 'toolu_1', tool: 'Bash', input: { command: 'ls' } }]
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  expect(await isRaw($, group(w, { calls, isActive: false, isExpanded: false, isCallsOnly: true }))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests).toEqual([])
  if (isWs(w)) expect(w.configKeys).toContain(ENABLED_KEY)
  else expect(w.configKeys.filter((k) => k.startsWith('claude.display_summary_') || k === LANG_KEY)).toEqual([])
  for (const key of [MODEL_KEY, STYLE_KEY, CONTEXT_KEY, LANG_KEY]) expect(w.configKeys).not.toContain(key)
})

test('another package is inert even with the switch on: rows draw raw and nothing is read or sent', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (isWs(w)) return
  expect(['wsflow']).toContain(w.plugin)
  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'toolu_1' } as never)
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  expect(w.requests).toEqual([])
  expect(w.configKeys.filter((k) => k.startsWith('claude.display_summary_') || k === LANG_KEY)).toEqual([])
})

test('ws: a read failure of the switch leaves captions off', async ($, on) => {
  const w = world(on, { failKeys: [ENABLED_KEY] })
  await start($, w)
  if (!isWs(w)) return
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  expect(w.requests).toEqual([])
})

test('ws: the model, style, language and window settings are applied', async ($, on) => {
  const style = 'Style: one word per field.'
  const w = world(on, { config: { [MODEL_KEY]: 'claude-sonnet-x', [STYLE_KEY]: style, [LANG_KEY]: 'Korean', [CONTEXT_KEY]: 50_000 } })
  await start($, w)
  if (!isWs(w)) return
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests[0]!.model).toBe('claude-sonnet-x')
  const rules = w.requests[0]!.blocks[0]!.text.trimEnd()
  expect(rules).toBe(rulesPrompt(style, 'Korean'))
  expect(rules).toContain('Write intention, result and summary in Korean only.')
  expect(rules.split('\n').at(-1)).toBe(style)
  expect(rules).not.toContain(DEFAULT_STYLE)
})

test('ws: failed or invalid setting reads leave the defaults, and an unset language follows the user', async ($, on) => {
  const w = world(on, { failKeys: [STYLE_KEY, MODEL_KEY], config: { [CONTEXT_KEY]: -3, [LANG_KEY]: '' } })
  await start($, w)
  if (!isWs(w)) return
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests[0]!.model).toBe('haiku')
  const rules = w.requests[0]!.blocks[0]!.text.trimEnd()
  expect(rules).toBe(rulesPrompt(DEFAULT_STYLE, undefined))
  expect(rules).toContain('Write intention, result and summary in the language the user writes in.')
  expect(rules.split('\n').at(-1)).toBe(DEFAULT_STYLE)
})

test('ws: a failed request is not retried', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  w.replies.push('unanswered')
  await isRaw($, toolUse(w, 'toolu_1'))
  await tick(w)
  expect(w.requests.length).toBe(1)
  expect(await isRaw($, toolUse(w, 'toolu_1'))).toBe(true)
  await tick(w)
  await tick(w)
  expect(w.requests.length).toBe(1)
})

test('ws: the mirror starts over when the lead compaction lever lands, and on a main-thread compaction', async ($, on) => {
  const messages: Message[] = [
    { role: 'user', content: [{ type: 'text', text: 'alpha before compaction' }] },
    { role: 'assistant', content: [{ type: 'text', text: 'beta before compaction' }] },
    { role: 'user', content: [{ type: 'text', text: 'gamma latest' }] },
  ]
  const w = world(on, { messages })
  await start($, w)
  if (!isWs(w)) return
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

test('ws: a caption row names an MCP tool as the server spells it', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (!isWs(w)) return
  w.replies.push({ text: '[{"id":"u1","subtitle":"release lease","intention":"lease","result":"ok"}]' })
  const row = toolUse(w, 'toolu_m', { tool: 'mcp__plugin_ws_ws__tickets_release', input: {} })
  await isRaw($, row)
  await tick(w)
  expect(await find($, row, /plugin:ws:ws/)).toBe('plugin:ws:ws/tickets.release  release lease')
})

test('display tool names: plugin MCP tools, other MCP servers, and built-ins', () => {
  expect(displayToolName('mcp__plugin_ws_ws__tickets_release')).toBe('plugin:ws:ws/tickets.release')
  expect(displayToolName('mcp__plugin_wsflow_wsflow__git_status')).toBe('plugin:wsflow:wsflow/git.status')
  expect(displayToolName('mcp__github__get_issue')).toBe('github/get.issue')
  expect(displayToolName('mcp__ws__note_write')).toBe('ws/note.write')
  expect(displayToolName('Bash')).toBe('Bash')
})

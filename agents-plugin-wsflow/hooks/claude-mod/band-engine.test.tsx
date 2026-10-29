import { expect, mock, test, type TestBody } from 'claude-code/testing'

import {
  BAR_FLOOR,
  barCaps,
  barRuns,
  C,
  cells,
  createBand,
  ENABLED_KEY,
  fitChips,
  folderParts,
  parseAgenda,
  parseTodos,
  pctColor,
  row2Layout,
  titleParts,
  todoView,
  type Run,
} from './band'
import { KNOB } from './register'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the status band: the knob, the reads that follow it
// and the held key, and the drawn tree at several widths. The same file is
// mirrored into every package that ships the mod and runs identically in
// each: the band draws while its knob reads true, and its key comes only from
// the manual feature, on in a ws project with its own knob (`manual`), each
// set through the world's config_get answers.

// Byte-identical to the literals in the server's
// TestTodoAndAgendaListTextTheClaudeBandParses: its todo.list full-mode and
// agenda.list text, one todo of each status. A change on either side must
// change the other.
const TODO_TEXT = '- [x] {alpha} Alpha done\n- [~] {beta} Beta in progress\n      Beta instruction\n- [ ] {gamma} Gamma pending\n- [>] {delta} Delta deferred\n'
const AGENDA_TEXT = '- implement: {mode}\n- plan: "x"\n'

/** Seven todos: a pending one ahead of the in-progress one, two settled, four open besides the focus. */
const MANY_TODOS =
  '- [x] {alpha} Alpha done\n- [ ] {gamma} Gamma pending\n- [~] {beta} Beta in progress\n- [>] {delta} Delta deferred\n' +
  '- [ ] {eps} Eps pending\n- [ ] {zeta} Zeta pending\n- [ ] {eta} Eta pending\n'

const NOW = 1_800_000_000_000
const MODEL = 'claude-opus-x'
const STATIC_BODY = '# Workflow Manual\n\n> **Session invariant:** keep the key.\n\n## On: invoke\n\nLoad this manual.\n'

function keyedResponse(key: string): string {
  const body = STATIC_BODY.replace('keep the key.', `keep the key.\n> **Session key: \`${key}\`** — preserve verbatim.`).trimEnd()
  return `# Manuals\n- none\n\n${body}\n\n## Session Key\n${key}\n\n## Session State\n\n### Todos\n(no todos)\n`
}

type Usage = {
  startedAt: number
  context: { tokens?: number; window: number; breakdown?: { rawMaxTokens: number } }
  rateLimits: { kind: string; percentUsed: number; resetsAt?: string }[]
  cost?: { usd: number }
}

function world(on: On, opts: { config?: Record<string, unknown>; wsProject?: boolean; manual?: boolean } = {}) {
  const clock = mock.clock(on, { now: NOW })
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    store: new Map<string, unknown>(),
    sessionId: 'sess-1',
    /** The config values config_get answers; a key absent here reads as unset. */
    config: { [ENABLED_KEY]: true, ...(opts.manual ? { [KNOB.manual]: true } : {}), ...(opts.config ?? {}) } as Record<string, unknown>,
    /** While set, the knob's config_get answers an error result. */
    failKnob: false,
    failTodos: false,
    failAgenda: false,
    failLookup: false,
    /** todo_list / agenda_list text per session key; a key absent reads as empty. */
    todos: {} as Record<string, string>,
    agenda: {} as Record<string, string>,
    /** The key's own named-inbox address per session key; absent owns none. */
    addresses: {} as Record<string, string>,
    known: new Set<string>(),
    minted: 0,
    /** Every config_get's arguments, in order. */
    configCalls: [] as Record<string, unknown>[],
    /** The band's ws reads: todo_list, agenda_list, mailbox_lookup_peers. */
    wsCalls: [] as { tool: string; args: Record<string, unknown> }[],
    /** Each git argv the band ran. */
    gitRuns: [] as string[][],
    usageCalls: 0,
    usage: {
      startedAt: 0,
      context: { tokens: 50_000, window: 200_000 },
      rateLimits: [
        { kind: 'five_hour', percentUsed: 30, resetsAt: new Date(NOW + 2 * 3600_000).toISOString() },
        { kind: 'seven_day', percentUsed: 60, resetsAt: new Date(NOW + 3 * 86400_000).toISOString() },
      ],
      cost: { usd: 1.5 },
    } as Usage,
  }
  const text = (t: string) => ({ value: { content: [{ type: 'text', text: t }], isError: false } })
  const error = (t: string) => ({ value: { content: [{ type: 'text', text: t }], isError: true } })
  on('mcp.connect', ($, e) => ({ value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }))
  on('mcp.call', ($, e) => {
    const key = e.args.session_key as string
    switch (e.tool) {
      case 'config_get': {
        w.configCalls.push({ ...e.args })
        const name = String(e.args.key)
        if (name === ENABLED_KEY && w.failKnob) return error('ws-mcp is unhealthy')
        return text(JSON.stringify({ key: name, value: name in w.config ? w.config[name] : null, scope: 'project' }))
      }
      case 'ferrule': {
        const relogin = e.args.relogin_session_key
        if (typeof relogin === 'string') {
          return w.known.has(relogin) ? text(JSON.stringify({ session_key: relogin, root: '/repo' })) : error('not a known session key')
        }
        w.minted += 1
        const minted = `minted-${w.minted}`
        w.known.add(minted)
        return text(JSON.stringify({ session_key: minted, root: '/repo' }))
      }
      case 'workflow_manual':
        return text(keyedResponse(key))
      case 'playbook_read':
        return text(STATIC_BODY)
      case 'todo_list':
        w.wsCalls.push({ tool: e.tool, args: { ...e.args } })
        return w.failTodos ? error('todo list failed') : text(w.todos[key] ?? '(no todos)\n')
      case 'agenda_list':
        w.wsCalls.push({ tool: e.tool, args: { ...e.args } })
        return w.failAgenda ? error('agenda list failed') : text(w.agenda[key] ?? 'no agenda blobs\n')
      case 'mailbox_lookup_peers': {
        w.wsCalls.push({ tool: e.tool, args: { ...e.args } })
        if (w.failLookup) return error('lookup failed')
        const address = w.addresses[key]
        return text(JSON.stringify({ self: address ? { address } : {}, peers: [] }))
      }
      default:
        return error(`no ${e.tool} here`)
    }
  })
  on('store.get', ($, e) => ({ value: w.store.get(e.key) }))
  on('store.set', ($, e) => (w.store.set(e.key, e.value), { value: undefined }))
  on('store.delete', ($, e) => (w.store.delete(e.key), { value: undefined }))
  on('store.keys', () => ({ value: [...w.store.keys()] }))
  on('ui.log', () => ({ value: undefined }))
  on('ui.invalidate', () => ({ value: undefined }))
  on('session.id', () => ({ value: w.sessionId }))
  on('session.model', () => ({ value: MODEL }))
  on('session.cwd', () => ({ value: '/repo/sub' }))
  on('session.root', () => ({ value: '/repo' }))
  on('session.repo', () => ({ value: { root: '/repo', remote: null, internal: false, name: null } }))
  on('fs.exists', ($, e) => ({ value: opts.wsProject === true && e.path === '/repo/ai-docs/WORKFLOW.md' }))
  on('fs.ancestors', () => ({ value: [] }))
  on('session.usage', () => {
    w.usageCalls += 1
    return { value: structuredClone(w.usage) as never }
  })
  // Host commands answer git only: the mailbox push's probe fails, so no wait child runs.
  on('process.run', ($, e) => {
    if (e.argv[0] !== 'git') return { deny: 'no host commands on this surface' }
    w.gitRuns.push([...e.argv])
    const stdout = e.argv.includes('status') ? '## main...origin/main [ahead 2]\n M a.ts\n?? n.ts\n' : '3\t1\ta.ts\n'
    return { value: { exitCode: 0, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  // Core for the model's own tool calls.
  on('tool.call', ($, e) => {
    const out = e.tool.endsWith('__workflow_manual') ? keyedResponse((e as unknown as { session_key: string }).session_key) : 'ok'
    return { result: [{ type: 'text', text: out }], text: out } as never
  })
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('session.messages', () => ({ value: [] as never }))
  on('session.compact', () => ({ messages: [{ role: 'user', text: 'summary', toolUses: [] }] }) as never)
  on('prompt.compose', () => ({ sections: [] }))
  on('prompt.submit', ($, e) => ({ text: e.text }))
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('turn.step', async function* ($, e) {
    yield* []
    return { turnId: e.turnId, index: e.index, answer: '', toolUses: [], stopReason: 'end_turn', usage: null } as never
  })
  // The engine's own drawing: what the band leaves to it.
  on('ui.render', ($, e) => {
    const { Text } = $.ui.resolve(e)
    return <Text>raw</Text>
  })
  return w
}

type World = ReturnType<typeof world>

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/repo/sub', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

/** The model's own call of a tool on the package's MCP server. */
async function modelCall($: Engine, w: World, tool: string, args: Record<string, unknown> = {}) {
  await $.tool.call({ tool: `mcp__plugin_${w.plugin}_${w.plugin}__${tool}`, ...args } as never)
  await w.clock.settle()
}

/** One model request of the main loop, or of a subagent's with `agentId`. */
async function step($: Engine, w: World, model: string, effort?: string, agentId?: string) {
  const stream = $.turn.step({ turnId: 't', index: 0, model, messageCount: 1, ...(effort ? { effort } : {}), ...(agentId ? { agentId } : {}) } as never)
  for await (const _ of stream) {
    // The world yields nothing.
  }
  await w.clock.settle()
}

async function measure($: Engine, w: World, tokens: number) {
  w.usage.context.tokens = tokens
  await $.session.measure({ context: { tokens, window: w.usage.context.window }, rateLimits: [], changed: ['context'] } as never)
  await w.clock.settle()
}

async function turnEnd($: Engine, w: World) {
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.settle()
}

type El = { type: string; props?: Record<string, unknown>; children?: unknown[] }

/** What an element shows: its strings, its children's in document order. */
const shown = (n: unknown): string =>
  typeof n === 'string' ? n : n && typeof n === 'object' && Array.isArray((n as El).children) ? (n as El).children!.map(shown).join('') : ''

const elements = (n: unknown, out: El[] = []): El[] => {
  if (n && typeof n === 'object' && typeof (n as El).type === 'string') {
    out.push(n as El)
    for (const c of (n as El).children ?? []) elements(c, out)
  }
  return out
}

const kids = (n: El | undefined): El[] => (n?.children ?? []).filter((c): c is El => !!c && typeof c === 'object')
const byKey = (n: unknown, key: string) => elements(n).find((e) => e.props?.key === key)
/** Whether a Text in the tree shows text the pattern matches whole. */
const hasText = (n: unknown, pattern: RegExp) => elements(n).some((e) => e.type === 'Text' && pattern.test(shown(e)))

/** The band drawn at `W` cells, its frame taken apart; `raw` when the engine's own row drew. */
async function draw($: Engine, w: World, W: number, props: { hasSurvey?: boolean } = {}) {
  const ui = await $.ui.mount({
    plugin: w.plugin,
    surface: 'terminal',
    component: 'AbovePrompt',
    props: { hasSurvey: props.hasSurvey ?? false, isWorking: false, maxRows: 20, bodyColumns: W, scroll: { offset: 0, bodyRows: 19 }, view: {} },
  })
  const root = (await ui.drawn()) as unknown as El
  await ui.unmount()
  const raw = root.type === 'Text' && shown(root) === 'raw'
  const [top, body, bottom] = kids(root)
  const [lbar, rows, rbar] = kids(body)
  return { raw, root, text: shown(root), top, bottom, lbar, rbar, rows: kids(rows) }
}

type Drawn = Awaited<ReturnType<typeof draw>>

/** The frame closes: the top and bottom edges are `W` cells, each side bar has a `│` per content row. */
function expectFrame(d: Drawn, W: number, rowCount: number) {
  expect(d.raw).toBe(false)
  expect(d.root.props?.width).toBe(W)
  expect(shown(kids(d.top)[0])).toBe('╭─ ')
  expect(cells(shown(d.top))).toBe(W)
  expect(shown(d.top).endsWith('╮')).toBe(true)
  expect(shown(d.bottom)).toBe(`╰${'─'.repeat(W - 2)}╯`)
  expect(cells(shown(d.bottom))).toBe(W)
  expect(d.rows.length).toBe(rowCount)
  expect(kids(d.lbar).map(shown)).toEqual(Array(rowCount).fill('│'))
  expect(kids(d.rbar).map(shown)).toEqual(Array(rowCount).fill('│'))
}

const bandConfigCalls = (w: World) => w.configCalls.filter((a) => a.key === ENABLED_KEY)
const wsReads = (w: World, tool?: string) => w.wsCalls.filter((c) => tool === undefined || c.tool === tool)

for (const variant of ['unset', 'false'] as const) {
  test(`with the switch ${variant}, the engine's row draws and nothing past the switch is read`, async ($, on) => {
    const w = world(on, { wsProject: true, manual: true, config: variant === 'false' ? { [ENABLED_KEY]: false } : {} })
    if (variant === 'unset') delete w.config[ENABLED_KEY]
    w.todos['minted-1'] = TODO_TEXT
    await start($, w)
    expect(w.minted).toBe(1)
    expect(bandConfigCalls(w).length).toBeGreaterThan(0)
    await modelCall($, w, 'todo_check', { session_key: 'minted-1', key: 'gamma', status: 'wip' })
    await modelCall($, w, 'mailbox_send', { session_key: 'minted-1' })
    await turnEnd($, w)
    expect((await draw($, w, 80)).raw).toBe(true)
    expect(w.wsCalls).toEqual([])
    expect(w.gitRuns).toEqual([])
    expect(w.usageCalls).toBe(0)
  })
}

test('status rows only outside a ws project: the titled frame around two rows, the effort after the first main-loop request', async ($, on) => {
  const w = world(on)
  await start($, w)
  const W = 80
  let d = await draw($, w, W)
  expectFrame(d, W, 2)
  const title = kids(d.top)
  expect(shown(title[1])).toBe(MODEL)
  expect(title[1]!.props?.bold).toBe(true)
  // No effort before the first request.
  expect(title.length).toBe(3)
  expect(shown(d.top)).not.toContain('[')
  // A subagent's request leaves the title alone.
  await step($, w, 'claude-haiku-y', 'max', 'sub-1')
  d = await draw($, w, W)
  expect(shown(d.top)).not.toContain('[')
  expect(shown(kids(d.top)[1])).toBe(MODEL)
  await step($, w, 'claude-sonnet-z', 'high')
  d = await draw($, w, W)
  expectFrame(d, W, 2)
  expect(shown(kids(d.top)[1])).toBe('claude-sonnet-z')
  expect(shown(kids(d.top)[2])).toBe(' [high]')
  // Row 1: folder, branch and changes; row 2: the bar, both limits and the cost.
  expect(hasText(d.rows[0], /^ 📁 sub$/)).toBe(true)
  expect(hasText(d.rows[0], /^ 🌿 main$/)).toBe(true)
  expect(shown(d.rows[0])).toContain('+3 -1 ~1 ?1')
  expect(hasText(d.rows[1], /^ 25% 50,000$/)).toBe(true)
  expect(hasText(d.rows[1], /^\d\dH$/)).toBe(true)
  expect(hasText(d.rows[1], /^(Sun|Mon|Tue|Wed|Thu|Fri|Sat)$/)).toBe(true)
  expect(hasText(d.rows[1], /^ \$1\.50 $/)).toBe(true)
  // No key: no ws read, no mailbox capsule.
  expect(w.wsCalls).toEqual([])
  expect(d.text).not.toContain('✉')
})

test('status plus ws rows: the focus todo and count, the other open keys folding into +N, the agenda keys', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  w.todos['minted-1'] = MANY_TODOS
  w.agenda['minted-1'] = AGENDA_TEXT
  await start($, w)
  let W = 100
  let d = await draw($, w, W)
  expectFrame(d, W, 4)
  // The in-progress todo is the focus though a pending one comes first; defer counts as settled.
  const focus = d.rows[2]
  expect(focus?.props?.key).toBe('focus')
  expect(hasText(focus, /^▶ beta$/)).toBe(true)
  expect(hasText(focus, /^Beta in progress$/)).toBe(true)
  expect(hasText(focus, /^ todo 2\/7 $/)).toBe(true)
  const rest = d.rows[3]
  expect(rest?.props?.key).toBe('rest')
  expect(kids(kids(rest)[0]).map(shown)).toEqual(['○ gamma', '○ eps', '○ zeta', '○ eta'])
  const right = kids(rest)[1]
  expect(kids(right)[0]?.props?.key).toBe('agenda')
  expect(shown(kids(right)[0])).toContain(' agenda ')
  expect(shown(kids(right)[1])).toBe('implement · plan')
  // Narrow: the chips that fit, the rest folded, the agenda kept.
  W = 50
  d = await draw($, w, W)
  expectFrame(d, W, 4)
  expect(kids(kids(d.rows[3])[0]).map(shown)).toEqual(['○ gamma', '○ eps', '+2'])
  expect(shown(kids(kids(d.rows[3])[1])[1])).toBe('implement · plan')
  // A survey holds the band's place.
  expect((await draw($, w, 100, { hasSurvey: true })).raw).toBe(true)

  // The model's todo call reads the rows again: with none in progress, the first pending todo is the focus.
  w.todos['minted-1'] = '- [x] {alpha} Alpha done\n- [ ] {gamma} Gamma pending\n- [ ] {eps} Eps pending\n'
  const reads = wsReads(w, 'todo_list').length
  await modelCall($, w, 'todo_check', { session_key: 'minted-1', key: 'beta', status: 'done' })
  expect(wsReads(w, 'todo_list').length).toBe(reads + 1)
  d = await draw($, w, 100)
  expectFrame(d, 100, 4)
  expect(hasText(d.rows[2], /^▶ gamma$/)).toBe(true)
  expect(hasText(d.rows[2], /^ todo 1\/3 $/)).toBe(true)
  expect(kids(kids(d.rows[3])[0]).map(shown)).toEqual(['○ eps'])
  expect(wsReads(w, 'todo_list').at(-1)?.args).toEqual({ session_key: 'minted-1', mode: 'full' })
})

test('with no todos and no agenda the band is the two status rows', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  await start($, w)
  expect(wsReads(w, 'todo_list').length).toBe(1)
  expectFrame(await draw($, w, 80), 80, 2)
  // An agenda alone draws the rest row and no focus row.
  w.agenda['minted-1'] = AGENDA_TEXT
  await modelCall($, w, 'agenda_set', { session_key: 'minted-1', key: 'plan', value: 'x' })
  const d = await draw($, w, 80)
  expectFrame(d, 80, 3)
  expect(d.rows[2]?.props?.key).toBe('rest')
})

test('the mailbox capsule shows the key\'s own address, and is absent with none or a failed lookup', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  await start($, w)
  const lookups = wsReads(w, 'mailbox_lookup_peers')
  expect(lookups.length).toBe(1)
  expect(lookups[0]!.args).toEqual({ session_key: 'minted-1', scope: 'worktree', format: 'json' })
  expect((await draw($, w, 80)).text).not.toContain('✉')

  w.addresses['minted-1'] = 'quiet-otter'
  await modelCall($, w, 'mailbox_send', { session_key: 'minted-1' })
  let d = await draw($, w, 80)
  expect(hasText(d.rows[0], /^ ✉ quiet-otter $/)).toBe(true)

  w.failLookup = true
  await modelCall($, w, 'mailbox_recv', { session_key: 'minted-1' })
  expect(wsReads(w, 'mailbox_lookup_peers').length).toBe(3)
  d = await draw($, w, 80)
  expectFrame(d, 80, 2)
  expect(d.text).not.toContain('✉')
})

test('a config_tune call turns the band on and off without a restart, re-reading the switch with the held key', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true, config: { [ENABLED_KEY]: false } })
  w.todos['minted-1'] = TODO_TEXT
  await start($, w)
  expect((await draw($, w, 80)).raw).toBe(true)

  w.config[ENABLED_KEY] = true
  const before = bandConfigCalls(w).length
  await modelCall($, w, 'config_tune', { session_key: 'minted-1', key: ENABLED_KEY, value: true })
  expect(bandConfigCalls(w).length).toBe(before + 1)
  expect(bandConfigCalls(w).at(-1)).toEqual({ key: ENABLED_KEY, format: 'json', session_key: 'minted-1' })
  // Turning on reads every figure afresh, the key's rows included.
  let d = await draw($, w, 80)
  expectFrame(d, 80, 4)
  expect(hasText(d.rows[2], /^▶ beta$/)).toBe(true)
  expect(w.gitRuns.length).toBeGreaterThan(0)
  expect(w.usageCalls).toBeGreaterThan(0)

  w.config[ENABLED_KEY] = false
  await modelCall($, w, 'config_tune', { session_key: 'minted-1', key: ENABLED_KEY, value: false })
  expect((await draw($, w, 80)).raw).toBe(true)

  // Another tool's call does not re-read the switch.
  const calls = bandConfigCalls(w).length
  await modelCall($, w, 'config_get', { key: ENABLED_KEY })
  expect(bandConfigCalls(w).length).toBe(calls)

  w.config[ENABLED_KEY] = true
  await modelCall($, w, 'config_tune', { session_key: 'minted-1', key: ENABLED_KEY, value: true })
  d = await draw($, w, 80)
  expectFrame(d, 80, 4)
})

test('row 2 layout: the bar takes the rest down to its floor, then cost, 7d and 5h drop in that order', () => {
  const right = [
    { id: 'r5' as const, width: 8 },
    { id: 'r7' as const, width: 8 },
    { id: 'cost' as const, width: 9 },
  ]
  expect(row2Layout(60, right)).toEqual({ barW: 30, right: ['r5', 'r7', 'cost'] })
  expect(row2Layout(40, right)).toEqual({ barW: BAR_FLOOR, right: ['r5', 'r7', 'cost'] })
  expect(row2Layout(39, right)).toEqual({ barW: 19, right: ['r5', 'r7'] })
  expect(row2Layout(30, right)).toEqual({ barW: BAR_FLOOR, right: ['r5', 'r7'] })
  expect(row2Layout(29, right)).toEqual({ barW: 18, right: ['r5'] })
  expect(row2Layout(21, right)).toEqual({ barW: BAR_FLOOR, right: ['r5'] })
  expect(row2Layout(20, right)).toEqual({ barW: 18, right: [] })
  expect(row2Layout(1, right)).toEqual({ barW: 0, right: [] })
  // The order is the drop order's, not the candidates'.
  expect(row2Layout(30, [right[2]!, right[0]!, right[1]!])).toEqual({ barW: BAR_FLOOR, right: ['r5', 'r7'] })
})

test('rest row chips: all that fit, else a head of them and +N with its room kept', () => {
  expect(fitChips(['gamma', 'eps', 'zeta', 'eta'], 26)).toEqual({ shown: ['gamma', 'eps', 'zeta', 'eta'], more: 0 })
  expect(fitChips(['gamma', 'eps', 'zeta', 'eta'], 25)).toEqual({ shown: ['gamma', 'eps', 'zeta'], more: 1 })
  expect(fitChips(['gamma', 'eps', 'zeta', 'eta'], 18)).toEqual({ shown: ['gamma', 'eps'], more: 2 })
  expect(fitChips(['gamma', 'eps', 'zeta', 'eta'], 9)).toEqual({ shown: [], more: 4 })
  expect(fitChips(['gamma'], 3)).toEqual({ shown: [], more: 1 })
  expect(fitChips([], 0)).toEqual({ shown: [], more: 0 })
})

test('title parts: the effort drops first, then the model is cut, a dash always left', () => {
  expect(titleParts(MODEL, 'high', 60)).toEqual({ model: MODEL, effort: 'high', dashes: 35 })
  expect(titleParts(MODEL, 'high', 26)).toEqual({ model: MODEL, effort: 'high', dashes: 1 })
  expect(titleParts(MODEL, 'high', 25)).toEqual({ model: MODEL, effort: '', dashes: 7 })
  expect(titleParts(MODEL, 'high', 16)).toEqual({ model: 'claude-opu', effort: '', dashes: 1 })
  expect(titleParts(MODEL, '', 12)).toEqual({ model: 'claude', effort: '', dashes: 1 })
})

test('cells: wide emoji and East Asian characters take two, combining marks none', () => {
  expect(cells('📁 a')).toBe(4)
  expect(cells('⚡')).toBe(2)
  expect(cells('한글')).toBe(4)
  expect(cells('é')).toBe(1)
  expect(cells('╭─ ')).toBe(3)
})

test('narrow widths drop cost, then 7d, then 5h once the bar is at its floor, and below every floor the frame stays whole', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  w.todos['minted-1'] = MANY_TODOS
  w.agenda['minted-1'] = AGENDA_TEXT
  await start($, w)
  await step($, w, MODEL, 'high')
  const COST = /^ \$1\.50 $/
  const WEEKDAY = /^(Sun|Mon|Tue|Wed|Thu|Fri|Sat)$/
  const HOUR = /^\d\dH$/
  // Each capsule's cells, caps included, as drawn wide.
  const wide = await draw($, w, 160)
  const width = (key: string) => cells(shown(byKey(wide.rows[1], key)))
  const [w5, w7, wc] = [width('r5'), width('r7'), width('cost')]
  expect(Math.min(w5, w7, wc)).toBeGreaterThan(2)
  const present = (d: Drawn) => [hasText(d.rows[1], HOUR), hasText(d.rows[1], WEEKDAY), hasText(d.rows[1], COST)]
  const barWidth = (d: Drawn) => byKey(d.rows[1], 'bar')?.props?.width
  // The frame (4) and the bar's caps (2) around the bar, one cell before each right capsule.
  const all = BAR_FLOOR + 6 + w5 + w7 + wc + 3
  const two = BAR_FLOOR + 6 + w5 + w7 + 2
  const one = BAR_FLOOR + 6 + w5 + 1
  const cases: [number, boolean[], number][] = [
    [all, [true, true, true], BAR_FLOOR + 2],
    [all - 1, [true, true, false], all - 1 - 4 - (w5 + w7 + 2)],
    [two, [true, true, false], BAR_FLOOR + 2],
    [two - 1, [true, false, false], two - 1 - 4 - (w5 + 1)],
    [one, [true, false, false], BAR_FLOOR + 2],
    [one - 1, [false, false, false], one - 1 - 4],
  ]
  for (const [W, expected, bar] of cases) {
    const d = await draw($, w, W)
    expectFrame(d, W, 4)
    expect(present(d), `capsules at ${W}`).toEqual(expected)
    expect(barWidth(d), `bar at ${W}`).toBe(bar)
  }

  // Below every floor: the rows clip at the inner width, the frame and its title stay whole.
  for (const W of [16, 12]) {
    const d = await draw($, w, W)
    expectFrame(d, W, 4)
    expect(kids(d.top).reduce((n, t) => n + cells(shown(t)), 0)).toBe(W)
    expect(shown(d.top)).not.toContain('[high]')
    for (const row of d.rows) {
      expect(row.props?.width, `row ${String(row.props?.key)} at ${W}`).toBe(W - 4)
      expect(row.props?.overflow).toBe('hidden')
    }
    expect(present(d)).toEqual([false, false, false])
    expect(barWidth(d)).toBe(W - 4)
  }
})

test('bar caps and runs: the fill color reaches a cap only once the fill does; a label clamped into the fill sits on it', () => {
  const labels: Run[] = [
    { text: ' 50%', color: C.fg },
    { text: ' ', color: C.dim },
  ]
  expect(barCaps(0, 20)).toEqual({ left: false, right: false })
  expect(barCaps(4, 20)).toEqual({ left: false, right: false })
  expect(barCaps(50, 20)).toEqual({ left: true, right: false })
  expect(barCaps(99, 20)).toEqual({ left: true, right: false })
  expect(barCaps(100, 20)).toEqual({ left: true, right: true })
  expect(barCaps(100, 0)).toEqual({ left: false, right: false })

  const empty = barRuns(0, 20, labels, pctColor(0))
  expect(empty[0]).toEqual({ text: ' 50%', color: C.fg })
  expect(empty.some((r) => r.bg !== undefined)).toBe(false)

  const mid = barRuns(50, 20, labels, pctColor(50))
  expect(mid[0]).toEqual({ text: '█'.repeat(10), color: pctColor(50) })
  expect(mid[1]).toEqual({ text: ' 50%', color: C.fg })
  expect(mid.some((r) => r.bg !== undefined)).toBe(false)
  expect(mid.reduce((n, r) => n + cells(r.text), 0)).toBe(20)

  const full = barRuns(100, 20, labels, pctColor(100))
  expect(full.at(-1)).toEqual({ text: ' 50% ', color: C.onFill, bg: pctColor(100) })
  expect(full[0]).toEqual({ text: '█'.repeat(15), color: pctColor(100) })
  expect(full.reduce((n, r) => n + cells(r.text), 0)).toBe(20)
  expect(barRuns(50, 0, labels, pctColor(50))).toEqual([])
})

test('the drawn bar caps take the fill color at 0%, mid-range and 100%, and a measure moves the label', async ($, on) => {
  const w = world(on)
  await start($, w)
  const caps = async (tokens: number) => {
    await measure($, w, tokens)
    // Wide enough that the mid-range label fits past the fill edge.
    const d = await draw($, w, 160)
    const bar = byKey(d.rows[1], 'bar')
    const [left, runs, right] = kids(bar)
    return { left: left?.props?.color, right: right?.props?.color, runs: kids(runs), label: shown(runs) }
  }
  const empty = await caps(0)
  expect([empty.left, empty.right]).toEqual([C.pillBg2, C.pillBg2])
  expect(empty.label).toContain(' 0% 0 / 200K')

  const mid = await caps(100_000)
  expect([mid.left, mid.right]).toEqual([pctColor(50), C.pillBg2])
  expect(mid.label).toContain(' 50% 100,000 / 200K')
  expect(mid.runs.some((r) => r.props?.backgroundColor !== undefined)).toBe(false)

  const full = await caps(200_000)
  expect([full.left, full.right]).toEqual([pctColor(100), pctColor(100)])
  const label = full.runs.find((r) => shown(r).includes('100%'))
  expect(label?.props).toEqual({ color: C.onFill, backgroundColor: pctColor(100) })
})

test('a failed switch read draws nothing, and reads nothing past it', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  w.failKnob = true
  w.todos['minted-1'] = TODO_TEXT
  await start($, w)
  expect(bandConfigCalls(w).length).toBeGreaterThan(0)
  expect((await draw($, w, 80)).raw).toBe(true)
  expect(w.wsCalls).toEqual([])
  expect(w.gitRuns).toEqual([])
})

for (const variant of ['todo_list', 'agenda_list'] as const) {
  test(`a failed ${variant} read hides the ws rows and leaves the status rows`, async ($, on) => {
    const w = world(on, { wsProject: true, manual: true })
    w.todos['minted-1'] = TODO_TEXT
    w.agenda['minted-1'] = AGENDA_TEXT
    if (variant === 'todo_list') w.failTodos = true
    else w.failAgenda = true
    await start($, w)
    expect(wsReads(w, variant).length).toBe(1)
    const d = await draw($, w, 80)
    expectFrame(d, 80, 2)
    expect(d.text).not.toContain('▶')
    expect(d.text).not.toContain('agenda')
  })
}

test('an adopted key re-reads the switch, the rows and the address with it, and the old key\'s rows go', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  w.todos['minted-1'] = TODO_TEXT
  w.agenda['minted-1'] = AGENDA_TEXT
  w.addresses['minted-1'] = 'old-otter'
  w.todos.revived = '- [ ] {rv} Revived pending\n'
  w.addresses.revived = 'new-heron'
  await start($, w)
  let d = await draw($, w, 100)
  expectFrame(d, 100, 4)
  expect(hasText(d.rows[2], /^▶ beta$/)).toBe(true)
  expect(hasText(d.rows[0], /^ ✉ old-otter $/)).toBe(true)

  // A main-thread revive: the manual feature adopts the key it names.
  w.known.add('revived')
  const [configs, reads] = [bandConfigCalls(w).length, w.wsCalls.length]
  await modelCall($, w, 'workflow_manual', { session_key: 'revived' })
  expect(bandConfigCalls(w).slice(configs)).toEqual([{ key: ENABLED_KEY, format: 'json', session_key: 'revived' }])
  const after = w.wsCalls.slice(reads)
  expect(after.map((c) => c.tool).sort()).toEqual(['agenda_list', 'mailbox_lookup_peers', 'todo_list'])
  for (const c of after) expect(c.args.session_key).toBe('revived')
  d = await draw($, w, 100)
  expectFrame(d, 100, 3)
  expect(hasText(d.rows[2], /^▶ rv$/)).toBe(true)
  expect(hasText(d.rows[2], /^ todo 0\/1 $/)).toBe(true)
  expect(hasText(d.rows[0], /^ ✉ new-heron $/)).toBe(true)
  for (const old of ['beta', 'gamma', 'implement', 'old-otter']) expect(d.text).not.toContain(old)
})

test('after /clear the next conversation\'s minted key carries the reads, and the old key\'s rows go at the end', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  w.todos['minted-1'] = TODO_TEXT
  w.todos['minted-2'] = '- [~] {fresh} Fresh work\n'
  await start($, w)
  expectFrame(await draw($, w, 100), 100, 4)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  // Between the end and the next key, the status rows alone.
  expectFrame(await draw($, w, 100), 100, 2)
  w.sessionId = 'sess-2'
  await $.prompt.compose({ model: 'm', promptModel: 'm', surfaces: ['terminal'], tools: [], outputStyle: null, traits: [] } as never)
  await w.clock.settle()
  expect(w.minted).toBe(2)
  expect(bandConfigCalls(w).at(-1)).toEqual({ key: ENABLED_KEY, format: 'json', session_key: 'minted-2' })
  expect(wsReads(w, 'todo_list').at(-1)?.args).toEqual({ session_key: 'minted-2', mode: 'full' })
  expect(wsReads(w, 'agenda_list').at(-1)?.args).toEqual({ session_key: 'minted-2' })
  expect(wsReads(w, 'mailbox_lookup_peers').at(-1)?.args.session_key).toBe('minted-2')
  const d = await draw($, w, 100)
  expectFrame(d, 100, 3)
  expect(hasText(d.rows[2], /^▶ fresh$/)).toBe(true)
  expect(d.text).not.toContain('beta')
})

test('the switch is read keyless before a key is held, and with the key once it is', async ($, on) => {
  const w = world(on, { wsProject: true, manual: true })
  await start($, w)
  const calls = bandConfigCalls(w)
  expect(calls.length).toBeGreaterThanOrEqual(2)
  expect(calls[0]).toEqual({ key: ENABLED_KEY, format: 'json' })
  expect(calls.at(-1)).toEqual({ key: ENABLED_KEY, format: 'json', session_key: 'minted-1' })
  // The other features' reads stay keyless.
  for (const c of w.configCalls.filter((a) => a.key !== ENABLED_KEY)) expect(c.session_key).toBeUndefined()
})

test('a tool call refreshes git at most every two seconds, a turn end always does', async ($, on) => {
  const w = world(on)
  await start($, w)
  const statusRuns = () => w.gitRuns.filter((argv) => argv.includes('status')).length
  expect(statusRuns()).toBe(1)
  expect(w.gitRuns[0]).toEqual(['git', 'status', '--porcelain', '-b'])
  await $.tool.call({ tool: 'Bash', command: 'ls' } as never)
  await w.clock.settle()
  expect(statusRuns()).toBe(1)
  await w.clock.advance(2000)
  await $.tool.call({ tool: 'Bash', command: 'ls' } as never)
  await w.clock.settle()
  expect(statusRuns()).toBe(2)
  await turnEnd($, w)
  expect(statusRuns()).toBe(3)
  // A subagent's turn end is not the main thread's.
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer', agentId: 'sub-1' } as never)
  await w.clock.settle()
  expect(statusRuns()).toBe(3)
})

test('each model tool call re-reads the context fill and redraws with it', async ($, on) => {
  const w = world(on)
  await start($, w)
  const bar = async () => shown(kids(byKey((await draw($, w, 160)).rows[1], 'bar'))[1])
  const first = w.usageCalls
  expect(first).toBeGreaterThan(0)
  expect(await bar()).toContain(' 25% 50,000 / 200K')
  // The agent is mid-turn: no session.measure, the fill moves with each tool call, however close together.
  w.usage.context.tokens = 100_000
  await modelCall($, w, 'todo_check', { session_key: 'minted-1', key: 'gamma', status: 'wip' })
  expect(w.usageCalls).toBe(first + 1)
  expect(await bar()).toContain(' 50% 100,000 / 200K')
  w.usage.context.tokens = 120_000
  await modelCall($, w, 'todo_check', { session_key: 'minted-1', key: 'gamma', status: 'wip' })
  expect(w.usageCalls).toBe(first + 2)
  expect(await bar()).toContain(' 60% 120,000 / 200K')
  // A session.measure reads too.
  await measure($, w, 140_000)
  expect(w.usageCalls).toBeGreaterThan(first + 2)
  expect(await bar()).toContain(' 70% 140,000 / 200K')
})

test('the bar is measured against the compaction window when the breakdown carries it, else the model window', async ($, on) => {
  const w = world(on)
  w.usage.context.window = 1_000_000
  w.usage.context.breakdown = { rawMaxTokens: 200_000 }
  await start($, w)
  const bar = async () => shown(kids(byKey((await draw($, w, 160)).rows[1], 'bar'))[1])
  await measure($, w, 100_000)
  expect(await bar()).toContain(' 50% 100,000 / 200K')
  // No breakdown, or a non-positive window in it: the model's window.
  delete w.usage.context.breakdown
  await measure($, w, 100_000)
  expect(await bar()).toContain(' 10% 100,000 / 1M')
  w.usage.context.breakdown = { rawMaxTokens: 0 }
  await measure($, w, 100_000)
  expect(await bar()).toContain(' 10% 100,000 / 1M')
})

test('createBand: a usage read in flight leaves one trailing read, and the mod\'s own tool calls read nothing', async () => {
  let usageCalls = 0
  let release: (() => void) | undefined
  let gated = false
  const usage = { startedAt: 0, context: { tokens: 1, window: 100 }, rateLimits: [] } as never
  const band = createBand()
  band.start({
    readSetting: async () => true,
    call: async () => '(no todos)\n',
    address: async () => undefined,
    git: async () => null,
    usage: async () => {
      usageCalls += 1
      if (gated) await new Promise<void>((r) => (release = r))
      return usage
    },
    model: async () => 'm',
    cwd: async () => '/c',
    root: async () => '/c',
    now: async () => NOW,
    every: () => () => {},
    redraw: () => {},
    log: () => {},
  } as never)
  await band.idle()
  expect(usageCalls).toBe(1)
  // No read in flight and no time throttle: only the guard keeps the mod's own call from reading.
  band.toolCalled('Bash', undefined, false)
  await band.idle()
  expect(usageCalls).toBe(1)
  band.toolCalled('Bash', undefined, true)
  await band.idle()
  expect(usageCalls).toBe(2)
  gated = true
  band.toolCalled('Bash', undefined, true)
  await new Promise((r) => setTimeout(r, 0))
  expect(usageCalls).toBe(3)
  band.toolCalled('Bash', undefined, true)
  band.measured()
  gated = false
  release?.()
  await band.idle()
  // Both late arrivals folded into one trailing read.
  expect(usageCalls).toBe(4)
})

test('folderParts: either path separator splits the folder name from the path below the root', () => {
  expect(folderParts('/repo/sub', '/repo')).toEqual({ dirName: 'sub', rel: '/sub' })
  expect(folderParts('C:\\repo\\sub', 'C:\\repo')).toEqual({ dirName: 'sub', rel: '\\sub' })
  expect(folderParts('C:\\repo', 'C:\\repo')).toEqual({ dirName: 'repo', rel: '' })
  expect(folderParts('/repository/sub', '/repo')).toEqual({ dirName: 'sub', rel: '' })
  expect(folderParts('/repo/sub', '')).toEqual({ dirName: 'sub', rel: '' })
})

test('todo and agenda text: every todo status and agenda key the server renders is read', () => {
  expect(parseTodos(TODO_TEXT)).toEqual([
    { key: 'alpha', title: 'Alpha done', status: 'done' },
    { key: 'beta', title: 'Beta in progress', status: 'wip' },
    { key: 'gamma', title: 'Gamma pending', status: 'pending' },
    { key: 'delta', title: 'Delta deferred', status: 'defer' },
  ])
  expect(parseAgenda(AGENDA_TEXT)).toEqual(['implement', 'plan'])
  expect(parseTodos('(no todos)\n')).toEqual([])
  expect(parseAgenda('no agenda blobs\n')).toEqual([])
  expect(parseTodos('session key not found\n')).toBeUndefined()
  expect(parseAgenda('session key not found\n')).toBeUndefined()
  expect(parseTodos('')).toBeUndefined()
  // The pinned todos as the ws rows count them: defer settles, the wip one is the focus.
  const view = todoView(parseTodos(TODO_TEXT)!)
  expect([view.focus?.key, view.rest.map((t) => t.key), view.done, view.total]).toEqual(['beta', ['gamma'], 2, 4])
})

test('the band on with the manual knob off draws the key-less rows only, and reads nothing key-bound', async ($, on) => {
  const w = world(on, { wsProject: true })
  w.todos['minted-1'] = TODO_TEXT
  w.agenda['minted-1'] = AGENDA_TEXT
  w.addresses['minted-1'] = 'quiet-otter'
  await start($, w)
  // No key: the manual feature minted none, so the band holds none.
  expect(w.minted).toBe(0)
  expect(w.configCalls.filter((a) => a.key === KNOB.manual)).toEqual([{ key: KNOB.manual, format: 'json' }])
  await step($, w, MODEL, 'high')
  let d = await draw($, w, 100)
  expectFrame(d, 100, 2)
  expect(shown(kids(d.top)[1])).toBe(MODEL)
  expect(hasText(d.rows[0], /^ 📁 sub$/)).toBe(true)
  expect(hasText(d.rows[1], /^ 25% 50,000$/)).toBe(true)
  expect(d.text).not.toContain('✉')
  // The model's ws calls refresh nothing key-bound either.
  await modelCall($, w, 'todo_check', { session_key: 'minted-1', key: 'gamma', status: 'wip' })
  await modelCall($, w, 'agenda_set', { session_key: 'minted-1', key: 'plan', value: 'x' })
  await modelCall($, w, 'mailbox_send', { session_key: 'minted-1' })
  await turnEnd($, w)
  d = await draw($, w, 100)
  expectFrame(d, 100, 2)
  expect(w.wsCalls).toEqual([])
  // The switch stays keyless.
  for (const c of bandConfigCalls(w)) expect(c.session_key).toBeUndefined()
  expect(w.gitRuns.length).toBeGreaterThan(0)
})

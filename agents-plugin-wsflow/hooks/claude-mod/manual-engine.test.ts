import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { adoptionNotice, inlinedPointerText, KEY_STORE_PREFIX, KEY_WAIT_MS, MAILBOX_LINE, pointerText, preloadPointerLine, rowText } from './manual'
import { KNOB } from './register'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the manual feature's wiring; its state machine is
// covered in manual.test.ts. The same file is mirrored into every package that
// ships the mod and runs identically in each: the feature runs only in a ws
// project with the claude.manual_injection_enabled knob reading true, which
// each test sets through the world's config_get answers (ON).

const STATIC_BODY = '# Workflow Manual\n\n> **Session invariant:** keep the key.\n\n## On: invoke\n\nLoad this manual.\n'

function keyedResponse(key: string, state: string): string {
  const body = STATIC_BODY.replace('keep the key.', `keep the key.\n> **Session key: \`${key}\`** — preserve verbatim.`).trimEnd()
  return `# Manuals\n- none\n\n${body}\n\n## Session Key\n${key}\n\n## Session State\n\n### Todos\n${state}\n`
}

/** ws-mcp's fail-loud reply for an unresolvable key: a non-error result with no `## Session Key` line. */
const NO_STATE = '## Session State\n(no restorable state for session key "gone")\n'

type World = ReturnType<typeof world>

function world(
  on: On,
  opts: {
    store?: Record<string, unknown>
    workflowFile?: boolean
    repo?: 'none'
    /** fs.ancestors rejects: with repo 'none', the project check cannot tell. */
    ancestorsThrow?: boolean
    /** A minting ferrule (not a re-login) waits this long on the mock clock. */
    holdMintMs?: number
    failMint?: boolean
    /** Host commands run (the mailbox push's probe passes), after `probeDelayMs` on the mock clock. */
    processOk?: boolean
    probeDelayMs?: number
    /** The mailbox_lookup_peers a slug resolution makes answers this late on the mock clock. */
    lookupDelayMs?: number
    /** The values config_get answers by key; a key absent reads as unset, the advisory as 50%. */
    knobs?: Record<string, unknown>
    /** Keys whose config_get answers an error result. */
    failKnobs?: readonly string[]
    /** The knobs are tuned at session scope only: a read without session_key finds them unset. */
    sessionScoped?: boolean
    /** The start's mcp.connect answers not connected. */
    serverDown?: boolean
  } = {},
) {
  const clock = mock.clock(on, { now: 1_800_000_000_000 })
  mock.env(on, {})
  const w = {
    clock,
    /** The plugin's $.store. */
    store: new Map<string, unknown>(Object.entries(opts.store ?? {})),
    plugin: '',
    sessionId: 'sess-1',
    mcpCalls: [] as { server: string; tool: string; args: Record<string, unknown> }[],
    minted: 0,
    known: new Set<string>(),
    state: '(no todos)',
    /** What core answers for the model's workflow_manual call instead of the keyed render. */
    coreManual: undefined as string | undefined,
    /** The conversation the scans read. */
    messages: [] as { role: string; content: Record<string, unknown>[] }[],
    messageReads: 0,
    /** While set, the conversation read rejects. */
    messagesFail: false,
    /** The model-only user rows the mod appended, as text. */
    rows: [] as string[],
    /** While set, the mod's own rows are refused. */
    refuseRows: false,
    /** While set, the mod's own workflow_manual call answers an error result. */
    manualFails: false,
    existsChecks: [] as string[],
    prompts: [] as string[],
    logs: [] as { text: string; to: string }[],
    registered: [] as string[],
    /** Spawned `mailbox wait` argvs; the session key is at index 5. */
    spawns: [] as string[][],
    killed: [] as number[],
    /** While set, core answers the model's workflow_manual with this error. */
    coreError: undefined as string | undefined,
    /** Every config_get's arguments. */
    configCalls: [] as Record<string, unknown>[],
    /** Keys whose config_get answers an error result; a test may change it between reads. */
    failKnobs: new Set<string>(opts.failKnobs ?? []),
    /** The `/mcp reconnect` commands the mod ran. */
    reconnects: 0,
    /** What playbook_read answers; undefined answers an error result. */
    playbookText: ((name: string) => `# ${name}\n\nThe ${name} procedure.\n`) as ((name: string, key: unknown) => string | undefined),
    /** The next connect finds the server down, as at a start (a hot reload) before it is up. */
    downAtNextConnect: () => {
      serverDown = true
    },
  }
  let serverDown = opts.serverDown === true
  on('store.get', ($, e) => ({ value: w.store.get(e.key) }))
  on('store.set', ($, e) => (w.store.set(e.key, e.value), { value: undefined }))
  on('store.delete', ($, e) => (w.store.delete(e.key), { value: undefined }))
  on('store.keys', () => ({ value: [...w.store.keys()] }))
  on('ui.log', ($, e) => (w.logs.push({ text: e.text, to: e.to }), { value: undefined }))
  on('session.id', () => ({ value: w.sessionId }))
  on('session.root', () => ({ value: '/repo/sub' }))
  on('session.repo', () => ({ value: opts.repo === 'none' ? null : { root: '/repo', remote: null, internal: false, name: null } }))
  on('fs.exists', ($, e) => (w.existsChecks.push(e.path), { value: opts.workflowFile !== false && e.path === '/repo/ai-docs/WORKFLOW.md' }))
  on('fs.ancestors', () => {
    if (opts.ancestorsThrow) throw new Error('cannot walk here')
    return { value: opts.workflowFile === false ? [] : [{ dir: '/repo', name: 'ai-docs/WORKFLOW.md', content: '# Workflow', parts: [] }] }
  })
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('process.run', async () => {
    if (!opts.processOk) return { deny: 'no host commands on this surface' }
    if (opts.probeDelayMs) await clock.sleep(opts.probeDelayMs)
    return { value: { exitCode: 0, stdout: 'Python 3.12.0\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('process.spawn', async function* ($, e, next) {
    const index = w.spawns.length
    w.spawns.push([...e.argv])
    next.signal.addEventListener('abort', () => w.killed.push(index), { once: true })
    // The child never exits on its own here: these tests watch binds, not wakes.
    await new Promise<void>(() => {})
    yield* []
    return { value: { code: 0, signal: null } }
  })
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    w.registered.push(e.name)
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  on('mcp.connect', ($, e) => {
    // At start only: a later connect (a bind's) finds the server up.
    const down = serverDown
    serverDown = false
    return down
      ? { value: { isConnected: false, reason: 'failed', message: 'did not connect' } }
      : { value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }
  })
  on('mcp.call', async ($, e) => {
    const text = (t: string) => ({ value: { content: [{ type: 'text', text: t }], isError: false } })
    const error = (t: string) => ({ value: { content: [{ type: 'text', text: t }], isError: true } })
    // Knob reads: recorded apart from the manual's own ws calls.
    if (e.tool === 'config_get') {
      const key = String(e.args.key)
      w.configCalls.push({ ...e.args })
      if (w.failKnobs.has(key)) return error('ws-mcp is unhealthy')
      const knobs: Record<string, unknown> = { 'claude.compaction_advisory_percent': '50%', ...(opts.knobs ?? {}) }
      const value = key in knobs && !(opts.sessionScoped && e.args.session_key === undefined) ? knobs[key] : null
      return text(JSON.stringify({ key, value, scope: value === null ? 'unset' : 'project' }))
    }
    w.mcpCalls.push({ server: e.server, tool: e.tool, args: e.args })
    if (e.tool === 'ferrule') {
      const relogin = e.args.relogin_session_key
      if (typeof relogin === 'string') {
        return w.known.has(relogin) ? text(JSON.stringify({ session_key: relogin, root: '/repo' })) : error('not a known session key')
      }
      if (opts.failMint) return error('ws-mcp is unhealthy')
      if (opts.holdMintMs) await clock.sleep(opts.holdMintMs)
      w.minted += 1
      const key = `minted-${w.minted}`
      w.known.add(key)
      return text(JSON.stringify({ session_key: key, root: '/repo' }))
    }
    if (e.tool === 'workflow_manual') {
      if (w.manualFails) return error('workflow_manual failed')
      return text(keyedResponse(e.args.session_key as string, w.state))
    }
    if (e.tool === 'playbook_read') {
      const body = w.playbookText(String(e.args.name), e.args.session_key)
      return body === undefined ? error('no such playbook') : text(body)
    }
    return error(`unknown tool ${e.tool}`)
  })
  on('session.messages', () => {
    w.messageReads += 1
    if (w.messagesFail) throw new Error('conversation unreadable')
    return { value: w.messages as never }
  })
  on('session.append', ($, e, next) => {
    if (e.door === 'note' && w.refuseRows) return { deny: 'refused by the test' }
    if (e.door === 'note' && e.message.type === 'user') w.rows.push(e.message.content.map((b) => (b.type === 'text' ? b.text : '')).join(''))
    return next(e)
  })
  // Core for the model's ws reads: the server's own text.
  on('tool.call', async ($, e) => {
    const args = e as unknown as Record<string, unknown>
    if (opts.lookupDelayMs && e.tool.endsWith('__mailbox_lookup_peers')) await clock.sleep(opts.lookupDelayMs)
    if (w.coreError !== undefined && e.tool.endsWith('__workflow_manual')) return { isError: true, result: undefined, text: w.coreError } as never
    const text = e.tool.endsWith('__workflow_manual')
      ? (w.coreManual ?? keyedResponse(args.session_key as string, w.state))
      : e.tool.endsWith('__playbook_read')
        ? `# ${String(args.name)}\n## Steps\nwork`
        : 'other'
    return { result: [{ type: 'text', text }], text } as never
  })
  // A reconnect always brings the server's tools back.
  on('command.run', async () => ((w.reconnects += 1), {}))
  on('tool.list', () => ({ value: [{ name: `mcp__plugin_${w.plugin}_${w.plugin}__config_get`, description: 'x', mcp: true }] }))
  on('prompt.submit', ($, e) => (w.prompts.push(e.text), { text: e.text }))
  // Core for a skill's expansion: the text as the skill computed it.
  on('skill.prompt', ($, e) => ({ text: e.text }))
  return w
}

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/repo/sub', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

/** A turn starting: the main thread's, or a subagent's with `agentId`. */
async function turnStart($: Engine, w: World, agentId?: string) {
  await $.turn.start({ text: '', turnId: 't', ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
}

async function clear($: Engine, w: World) {
  await $.session.end({ reason: 'clear', sessionId: w.sessionId, resume: undefined as never })
  await w.clock.settle()
}

async function read($: Engine, w: World, tool: 'workflow_manual' | 'playbook_read', args: Record<string, unknown>, agentId?: string) {
  const res = await $.tool.call({ tool: `mcp__plugin_${w.plugin}_${w.plugin}__${tool}`, ...args, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

/** Record a read the model saw into the conversation the scans read. */
function remember(w: World, tool: string, args: Record<string, unknown>, text: string) {
  const id = `tu-${w.messages.length}`
  w.messages.push({ role: 'assistant', content: [{ type: 'tool_use', id, name: `mcp__plugin_${w.plugin}_${w.plugin}__${tool}`, input: args }] })
  w.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: id, content: [{ type: 'text', text }] }] })
}

/** Record the rows the mod appended so far into the conversation, as a request carries them. */
function rememberRows(w: World) {
  for (const text of w.rows) w.messages.push({ role: 'user', content: [{ type: 'text', text }] })
}

const seen = (res: { result?: unknown; text?: string }) => (typeof res.result === 'string' ? res.result : res.text)

/** The feature's knob on. */
const ON = { [KNOB.manual]: true }
/** The manual knob's reads. */
const manualReads = (w: World) => w.configCalls.filter((a) => a.key === KNOB.manual)

// The playbook preload: a lead skill shim's `<ws-preload>` block replaced with
// its playbook body, read with the conversation's key the manual feature holds;
// an expansion is the manual row's delivery point.

/** The block's fallback prose: the shim's heading, then the read instructions. */
const fallback = (title: string, playbook: string) => `# ${title}\n\nCall \`ws/playbook.read(name: "${playbook}")\` and execute the returned procedure.`

/** A shim with its heading and today's fallback prose inside the block, as the skill computes it. */
const shim = (title: string, playbook: string) => `<ws-preload playbook="${playbook}">\n${fallback(title, playbook)}\n</ws-preload>\n\nARGUMENTS: decide the thing\n`

/** `pluginName` null: a project skill, which comes from no plugin. */
async function expand($: Engine, w: World, skill: string, text: string, pluginName: string | null = w.plugin) {
  const res = await $.skill.prompt({ skill, text, source: pluginName === null ? 'projectSettings' : 'plugin', ...(pluginName === null ? {} : { pluginName }) })
  await w.clock.settle()
  return res.text
}

const playbookReads = (w: World) => w.mcpCalls.filter((c) => c.tool === 'playbook_read')
const manualCalls = (w: World) => w.mcpCalls.filter((c) => c.tool === 'workflow_manual')
const ferruleArgs = (w: World) => w.mcpCalls.filter((c) => c.tool === 'ferrule').map((c) => c.args)

/** The row the mod appends for `key`: the whole keyed response with `state`, the mailbox line with `push`. */
const ROW = (w: World, key: string, state = '(no todos)', push = false) => rowText(w.plugin, key, keyedResponse(key, state), push)

test('in a ws project: a key is minted at start, and nothing is read or appended before a lead skill expands', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  expect(w.mcpCalls).toEqual([{ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'ferrule', args: { root: '/repo/sub', format: 'json' } }])
  // The knob is read once, keyless: no key exists yet to read it with.
  expect(manualReads(w)).toEqual([{ key: KNOB.manual, format: 'json' }])
  // The project check reads the repository root, not the launch directory.
  expect(w.existsChecks).toEqual(['/repo/ai-docs/WORKFLOW.md'])
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  await turnStart($, w)
  expect(w.rows).toEqual([])
  expect(w.mcpCalls.length).toBe(1)
})

test('the first lead skill expansion appends one row with the key line and the whole workflow_manual response; the skill text carries the playbook only', async ($, on) => {
  const w = world(on, { knobs: ON })
  w.state = '- [ ] the secret todo'
  await start($, w)
  const before = w.mcpCalls.length
  const text = await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(text).toBe('# lead-discuss\n\nThe lead-discuss procedure.\n\nARGUMENTS: decide the thing\n')
  expect(w.rows).toEqual([ROW(w, 'minted-1', '- [ ] the secret todo')])
  expect(w.rows[0]).toContain('Your lead session key for this conversation is `minted-1`')
  // The expansion's reads: the playbook, then the manual with the conversation key.
  expect(w.mcpCalls.slice(before)).toEqual([
    { server: `plugin:${w.plugin}:${w.plugin}`, tool: 'playbook_read', args: { name: 'lead-discuss', session_key: 'minted-1' } },
    { server: `plugin:${w.plugin}:${w.plugin}`, tool: 'workflow_manual', args: { session_key: 'minted-1' } },
  ])
})

test('a second expansion after a state change appends nothing while the row is visible; after the conversation loses it, the next expansion appends again', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  rememberRows(w)
  w.state = '- [ ] a new todo'
  await expand($, w, 'lead-run', shim('Run', 'lead-run'))
  await expand($, w, `${w.plugin}:lead-ticket`, shim('Ticket', 'lead-ticket'))
  expect(w.rows.length).toBe(1)
  expect(manualCalls(w).length).toBe(1)
  // A compaction, /clear or a rewind took the row away: the next expansion delivers the current state.
  w.messages = []
  await expand($, w, 'lead-run', shim('Run', 'lead-run'))
  expect(w.rows).toEqual([ROW(w, 'minted-1'), ROW(w, 'minted-1', '- [ ] a new todo')])
})

test('a visible workflow_manual result carrying ## Session Key suppresses the row, whatever its key; a fail-loud no-restorable-state result does not', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  remember(w, 'workflow_manual', { session_key: 'gone' }, NO_STATE)
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows.length).toBe(1)
  w.messages = []
  w.rows = []
  remember(w, 'workflow_manual', { session_key: 'other-key' }, keyedResponse('other-key', '(no todos)'))
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([])
  // An errored result is no copy either.
  w.messages = [
    { role: 'assistant', content: [{ type: 'tool_use', id: 'e1', name: `mcp__plugin_${w.plugin}_${w.plugin}__workflow_manual`, input: {} }] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'e1', is_error: true, content: [{ type: 'text', text: keyedResponse('x', '') }] }] },
  ]
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows.length).toBe(1)
})

test('a failed conversation read appends the row', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  rememberRows(w)
  w.messagesFail = true
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-1')])
})

test('a failed manual read or a refused row appends nothing and keeps the expansion; the next expansion delivers', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  w.manualFails = true
  expect(await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))).toContain('The lead-discuss procedure.')
  w.manualFails = false
  w.refuseRows = true
  expect(await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))).toContain('The lead-discuss procedure.')
  expect(w.rows).toEqual([])
  w.refuseRows = false
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-1')])
})

test('a block that passes through appends nothing: lead-use-mailbox, another plugin, a project skill, a mismatched block, a failed playbook read', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  for (const [skill, text, plugin] of [
    ['lead-use-mailbox', shim('Use Mailbox', 'lead-use-mailbox'), w.plugin],
    ['lead-discuss', shim('Discuss', 'lead-discuss'), 'other-plugin'],
    ['lead-discuss', shim('Discuss', 'lead-discuss'), null],
    ['lead-discuss', shim('Discuss', 'lead-run'), w.plugin],
  ] as const) {
    expect(await expand($, w, skill, text, plugin)).toBe(text)
  }
  w.playbookText = () => undefined
  expect(await expand($, w, 'lead-ship', shim('Ship', 'lead-ship'))).toBe(shim('Ship', 'lead-ship'))
  expect(w.rows).toEqual([])
  expect(manualCalls(w)).toEqual([])
  expect(w.messageReads).toBe(0)
})

test('no preload replacement text carries the key, the mailbox line, or the session state; the row carries the mailbox line where the push runs', { timeoutMs: 20_000 }, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  w.state = '- [ ] the secret todo'
  await start($, w)
  const texts = [await expand($, w, 'lead-run', shim('Run', 'lead-run'))]
  rememberRows(w)
  // The pointer form too, once the body is visible.
  rememberSkill(w, `${w.plugin}:lead-run`, texts[0])
  w.playbookText = sameBody
  texts.push(await expand($, w, 'lead-run', shim('Run', 'lead-run')))
  w.messages.push({ role: 'user', content: [{ type: 'text', text: texts[1] }] })
  texts.push(await expand($, w, 'lead-run', shim('Run', 'lead-run')))
  expect(texts[2]).toBe(pointed('Run', 'lead-run'))
  for (const text of texts) {
    expect(text).not.toContain('minted-1')
    expect(text).not.toContain(MAILBOX_LINE)
    expect(text).not.toContain('the secret todo')
  }
  expect(w.rows).toEqual([ROW(w, 'minted-1', '- [ ] the secret todo', true)])
})

test('a body carrying the key passes through and appends nothing', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  w.playbookText = (name, key) => `# ${name}\n\nWait with --session-key ${String(key)}.\n`
  const original = shim('Review', 'lead-review')
  expect(await expand($, w, 'lead-review', original)).toBe(original)
  expect(w.rows).toEqual([])
})

test('workflow_manual results reach the model unrewritten: the current key, a repeat, another key, a sentinel without a root, a subagent call', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  const own = keyedResponse('minted-1', '(no todos)')
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toBe(own)
  remember(w, 'workflow_manual', { session_key: 'minted-1' }, own)
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toBe(own)
  // ws-mcp refuses to re-log in with an unknown key, so it is not adopted.
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'other-key' }))).toBe(keyedResponse('other-key', '(no todos)'))
  w.coreManual = `${STATIC_BODY}\nCall ferrule to bootstrap.\n`
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'obsidian-latch' }))).toBe(w.coreManual)
  w.coreManual = undefined
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }, 'sub-1'))).toBe(own)
  expect(w.messageReads).toBe(0)
})

test('without ai-docs/WORKFLOW.md at the repository root mints nothing, expands nothing, and appends nothing', async ($, on) => {
  const w = world(on, { knobs: ON, workflowFile: false })
  await start($, w)
  expect(w.mcpCalls).toEqual([])
  const original = shim('Discuss', 'lead-discuss')
  expect(await expand($, w, 'lead-discuss', original)).toBe(original)
  expect(w.rows).toEqual([])
})

test('outside a repository falls back to the ancestor walk', async ($, on) => {
  const w = world(on, { knobs: ON, repo: 'none' })
  await start($, w)
  expect(w.existsChecks).toEqual([])
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-1')])
})

test('a stored key (hot reload, resume) re-logs in instead of minting, and the row names it', async ($, on) => {
  const w = world(on, { knobs: ON, store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: 1_800_000_000_000 - 60_000 } } })
  w.known.add('kept-key')
  await start($, w)
  expect(w.mcpCalls[0].args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'kept-key' })
  expect(w.minted).toBe(0)
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'kept-key', at: 1_800_000_000_000 })
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'kept-key')])
})

test('/clear ends the conversation: the next main-thread turn mints a new key under the new session id, and stale entries are pruned at start', async ($, on) => {
  const day = 24 * 60 * 60 * 1000
  const now = 1_800_000_000_000
  const w = world(on, {
    knobs: ON,
    store: {
      [`${KEY_STORE_PREFIX}gone`]: { sessionKey: 'gone-key', at: now - 31 * day },
      [`${KEY_STORE_PREFIX}recent`]: { sessionKey: 'recent-key', at: now - day },
    },
  })
  await start($, w)
  expect([...w.store.keys()]).not.toContain(`${KEY_STORE_PREFIX}gone`)
  expect([...w.store.keys()]).toContain(`${KEY_STORE_PREFIX}recent`)
  await clear($, w)
  w.sessionId = 'sess-2'
  // A subagent's turn is not the conversation's.
  await turnStart($, w, 'sub-1')
  expect(w.minted).toBe(1)
  await turnStart($, w)
  expect(w.minted).toBe(2)
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: now })
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-2`)).toEqual({ sessionKey: 'minted-2', at: now })
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-2')])
})

test('after /clear a lead skill expansion before any turn sets up the next key itself and expands with it', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  await clear($, w)
  w.sessionId = 'sess-2'
  expect(await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))).toContain('The lead-discuss procedure.')
  expect(playbookReads(w).map((c) => c.args.session_key)).toEqual(['minted-2'])
  expect(w.rows).toEqual([ROW(w, 'minted-2')])
})

test('a failed setup at start is retried at the first main-thread turn only, and at each expansion; meanwhile blocks pass through with no row', async ($, on) => {
  const w = world(on, { knobs: ON, failMint: true })
  await start($, w)
  await turnStart($, w)
  await turnStart($, w)
  expect(ferruleArgs(w).length).toBe(2)
  const original = shim('Discuss', 'lead-discuss')
  expect(await expand($, w, 'lead-discuss', original)).toBe(original)
  expect(ferruleArgs(w).length).toBe(3)
  expect(w.rows).toEqual([])
  expect(w.store.size).toBe(0)
  // The full workflow_manual result passes through.
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toBe(keyedResponse('minted-1', '(no todos)'))
})

test('an expansion waits a bounded time for a slow mint and passes through on timeout; once minted, the next expansion delivers', async ($, on) => {
  const w = world(on, { knobs: ON, holdMintMs: 8_000 })
  await $.session.start({ cwd: '/repo/sub', surface: 'terminal', isInteractive: true })
  const original = shim('Discuss', 'lead-discuss')
  const pending = $.skill.prompt({ skill: 'lead-discuss', text: original, source: 'plugin', pluginName: w.plugin })
  await w.clock.advance(KEY_WAIT_MS)
  expect((await pending).text).toBe(original)
  expect(w.rows).toEqual([])
  await w.clock.advance(8_000 - KEY_WAIT_MS)
  expect(w.minted).toBe(1)
  expect(await expand($, w, 'lead-discuss', original)).toContain('The lead-discuss procedure.')
  expect(w.rows).toEqual([ROW(w, 'minted-1')])
})

test('a repeat playbook_read collapses to the pointer while the earlier result is in the conversation, read only on a hint match', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  const args = { name: 'lead-run', session_key: 'minted-1' }
  const full = '# lead-run\n## Steps\nwork'
  expect(seen(await read($, w, 'playbook_read', args))).toBe(full)
  expect(w.messageReads).toBe(0)
  remember(w, 'playbook_read', args, full)
  const second = await read($, w, 'playbook_read', args)
  expect(second.result).toBe(
    'The result is unchanged. Continue using the full result of your earlier `playbook_read` call in this conversation. Do not read it again (headings: # lead-run; ## Steps).',
  )
  expect(w.messageReads).toBe(1)
  remember(w, 'playbook_read', args, pointerText(full))
  expect(seen(await read($, w, 'playbook_read', args))).toBe(full)
  // Gone from the conversation (a compaction): not a repeat.
  w.messages = []
  expect(seen(await read($, w, 'playbook_read', args))).toBe(full)
  // A subagent's reads are never touched or counted.
  remember(w, 'playbook_read', args, full)
  const reads = w.messageReads
  expect(seen(await read($, w, 'playbook_read', args, 'sub-1'))).toBe(full)
  expect(w.messageReads).toBe(reads)
})

/** A live wait child makes each settle cost about a second of real time; these tests settle several times with one. */
const LIVE_CHILD = { timeoutMs: 20_000 }

/** A reconnect the model requests, run at the main-thread turn end; it resolves the server again. */
async function reconnect($: Engine, w: World) {
  await $.tool.call({ tool: `mcp__${w.plugin}__request_mcp_reconnect` } as never)
  await w.clock.settle()
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.settle()
}

test('a failed first manual knob read is read again once a reconnect resolves the server, and a true read starts the feature', async ($, on) => {
  const w = world(on, { knobs: ON, failKnobs: [KNOB.manual] })
  await start($, w)
  expect(manualReads(w).length).toBe(1)
  expect(w.minted).toBe(0)
  expect(await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))).toBe(shim('Discuss', 'lead-discuss'))
  w.failKnobs.delete(KNOB.manual)
  await reconnect($, w)
  expect(w.reconnects).toBe(1)
  expect(manualReads(w)).toEqual([
    { key: KNOB.manual, format: 'json' },
    { key: KNOB.manual, format: 'json' },
  ])
  expect(w.mcpCalls.map((c) => c.tool)).toEqual(['ferrule'])
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-1')])
  // Read once more only: a later reconnect reads nothing again.
  await reconnect($, w)
  expect(manualReads(w).length).toBe(2)
})

test('a manual knob read false is never read again, though a reconnect resolves the server', async ($, on) => {
  const w = world(on, { knobs: { [KNOB.manual]: false } })
  await start($, w)
  await reconnect($, w)
  expect(w.reconnects).toBe(1)
  expect(manualReads(w).length).toBe(1)
  expect(w.mcpCalls).toEqual([])
  expect(w.minted).toBe(0)
})

test('a retried manual knob read that fails again leaves the feature off, and is not retried again', async ($, on) => {
  const w = world(on, { knobs: ON, failKnobs: [KNOB.manual] })
  await start($, w)
  await reconnect($, w)
  expect(manualReads(w).length).toBe(2)
  await reconnect($, w)
  expect(w.reconnects).toBe(2)
  expect(manualReads(w).length).toBe(2)
  expect(w.mcpCalls).toEqual([])
  expect(w.minted).toBe(0)
  const original = shim('Discuss', 'lead-discuss')
  expect(await expand($, w, 'lead-discuss', original)).toBe(original)
  expect(w.rows).toEqual([])
})

const watchTool = (w: World) => w.registered.includes('mailbox_watch')
const boundKeys = (w: World) => w.spawns.map((argv) => argv[5])
/** The band's knob reads made with a session key: the key it holds. */
const bandKeys = (w: World) => w.configCalls.filter((a) => a.key === 'claude.status_band_enabled' && a.session_key !== undefined).map((a) => a.session_key)

test('a late open keeps the key the model bound through mailbox_watch: re-login with it, no mint, and the push, band and key entry stay on it', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, failKnobs: [KNOB.manual], processOk: true })
  await start($, w)
  expect(watchTool(w)).toBe(true)
  w.known.add('k1')
  const bound = await $.tool.call({ tool: `mcp__${w.plugin}__mailbox_watch`, session_key: 'k1' } as never)
  await w.clock.settle()
  expect(String(bound.result)).toContain('bound')
  w.failKnobs.delete(KNOB.manual)
  await reconnect($, w)
  expect(manualReads(w).length).toBe(2)
  expect(w.minted).toBe(0)
  expect(w.mcpCalls.filter((c) => c.tool === 'ferrule').map((c) => c.args)).toEqual([{ root: '/repo/sub', format: 'json', relogin_session_key: 'k1' }])
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'k1', at: 1_800_000_000_000 })
  // The automatic bind of the same key leaves the running wait alone.
  expect(boundKeys(w)).toEqual(['k1'])
  expect(w.killed).toEqual([])
  expect(bandKeys(w)).toEqual(['k1'])
  // mailbox_watch stays registered after the late open, and still rebinds.
  const rebound = await $.tool.call({ tool: `mcp__${w.plugin}__mailbox_watch`, session_key: 'k2' } as never)
  await w.clock.settle()
  expect(String(rebound.result)).toContain('rebound')
  expect(boundKeys(w)).toEqual(['k1', 'k2'])
})

test('a late open after a hot reload whose restore found the server down keeps the key the model bound: its reload entry, no mint', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, failKnobs: [KNOB.manual], processOk: true })
  await start($, w)
  w.known.add('k1')
  await $.tool.call({ tool: `mcp__${w.plugin}__mailbox_watch`, session_key: 'k1' } as never)
  await w.clock.settle()
  expect(boundKeys(w)).toEqual(['k1'])
  // A hot reload while the server is down: the old watcher stops, and the
  // restore of k1 gives up on the unresolved server, so no watcher holds it.
  w.downAtNextConnect()
  await start($, w)
  expect(w.killed).toEqual([0])
  expect(boundKeys(w)).toEqual(['k1'])
  w.failKnobs.delete(KNOB.manual)
  await reconnect($, w)
  expect(w.minted).toBe(0)
  expect(w.mcpCalls.filter((c) => c.tool === 'ferrule').map((c) => c.args)).toEqual([{ root: '/repo/sub', format: 'json', relogin_session_key: 'k1' }])
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'k1', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['k1', 'k1'])
  expect(bandKeys(w)).toEqual(['k1'])
})

test('a late open with no key bound mints one and binds the push to it', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, failKnobs: [KNOB.manual], processOk: true })
  await start($, w)
  expect(w.spawns).toEqual([])
  w.failKnobs.delete(KNOB.manual)
  await reconnect($, w)
  expect(w.minted).toBe(1)
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['minted-1'])
  expect(bandKeys(w)).toEqual(['minted-1'])
  expect(watchTool(w)).toBe(true)
})

test('a manual knob unread at start (server down) is read at a later start, and a start after the late open binds automatically, registering no bind tool', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, serverDown: true, processOk: true })
  await start($, w)
  expect(manualReads(w)).toEqual([])
  expect(w.minted).toBe(0)
  // A later start (a hot reload) resolves the server: the retry reads true and the feature starts.
  await start($, w)
  expect(manualReads(w)).toEqual([{ key: KNOB.manual, format: 'json' }])
  expect(w.minted).toBe(1)
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['minted-1'])
  const watchRegistrations = () => w.registered.filter((name) => name === 'mailbox_watch').length
  expect(watchRegistrations()).toBe(2)
  // The gate is open for every start after it: no bind tool, and the re-logged-in key is bound by the mod.
  await start($, w)
  expect(watchRegistrations()).toBe(2)
  expect(w.mcpCalls.filter((c) => c.tool === 'ferrule').at(-1)?.args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'minted-1' })
  expect(boundKeys(w)).toEqual(['minted-1', 'minted-1'])
  expect(manualReads(w).length).toBe(1)
})

for (const [what, opts] of [
  ['unset', {}],
  ['false', { knobs: { [KNOB.manual]: false } }],
  ['a non-boolean true', { knobs: { [KNOB.manual]: 'true' } }],
  ['an error result (a failed read)', { knobs: ON, failKnobs: [KNOB.manual] }],
  ['tuned at session scope only', { knobs: ON, sessionScoped: true }],
  ['unread: the server was not connected at start', { knobs: ON, serverDown: true }],
] as const) {
  test(`a manual knob ${what} in a ws project: no key, no row, no rewrite, and mailbox_watch registers and binds`, LIVE_CHILD, async ($, on) => {
    const w = world(on, { ...opts, processOk: true })
    await start($, w)
    // The project check passed; the knob closed the gate.
    expect(w.existsChecks).toEqual(['/repo/ai-docs/WORKFLOW.md'])
    expect(manualReads(w)).toEqual('serverDown' in opts ? [] : [{ key: KNOB.manual, format: 'json' }])
    expect(w.mcpCalls).toEqual([])
    await turnStart($, w)
    const original = shim('Discuss', 'lead-discuss')
    expect(await expand($, w, 'lead-discuss', original)).toBe(original)
    expect(w.rows).toEqual([])
    expect(w.mcpCalls).toEqual([])
    const args = { session_key: 'k' }
    expect(seen(await read($, w, 'workflow_manual', args))).toBe(keyedResponse('k', '(no todos)'))
    remember(w, 'playbook_read', { name: 'x' }, '# x\n## Steps\nwork')
    await read($, w, 'playbook_read', { name: 'x' })
    expect(seen(await read($, w, 'playbook_read', { name: 'x' }))).toBe('# x\n## Steps\nwork')
    expect(w.messageReads).toBe(0)
    expect([...w.store.keys()]).toEqual([])
    // The mailbox gate is the manual's: closed, so the agent binds its own key.
    expect(watchTool(w)).toBe(true)
    expect(w.spawns).toEqual([])
    const bound = await $.tool.call({ tool: `mcp__${w.plugin}__mailbox_watch`, session_key: 'k1' } as never)
    await w.clock.settle()
    expect(String(bound.result)).toContain('bound')
    expect(boundKeys(w)).toEqual(['k1'])
    expect(w.minted).toBe(0)
  })
}

test('in a ws project: no mailbox_watch tool; the minted key is bound automatically and the row says so', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  await start($, w)
  expect(watchTool(w)).toBe(false)
  expect(boundKeys(w)).toEqual(['minted-1'])
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-1', '(no todos)', true)])
  expect(w.rows[0]).toContain(MAILBOX_LINE)
  // The automatic bind leaves no lever binding behind.
  expect([...w.store.keys()].some((k) => k.startsWith('mailbox-watch:'))).toBe(false)
})

test('without host commands: no tool, no bind, and no mailbox line', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  expect(watchTool(w)).toBe(false)
  expect(w.spawns).toEqual([])
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.rows).toEqual([ROW(w, 'minted-1')])
  expect(w.rows[0]).not.toContain(MAILBOX_LINE)
})

for (const variant of ['subagent', 'errored'] as const) {
  test(`${variant === 'subagent' ? 'a subagent' : 'an errored'} workflow_manual call adopts nothing`, LIVE_CHILD, async ($, on) => {
    const w = world(on, { knobs: ON, processOk: true })
    await start($, w)
    w.known.add('revived')
    const calls = w.mcpCalls.length
    if (variant === 'subagent') {
      expect(seen(await read($, w, 'workflow_manual', { session_key: 'revived' }, 'sub-1'))).toBe(keyedResponse('revived', '(no todos)'))
    } else {
      w.coreError = 'workflow_manual failed'
      expect((await read($, w, 'workflow_manual', { session_key: 'revived' })).isError).toBe(true)
    }
    expect(w.mcpCalls.length).toBe(calls)
    expect(boundKeys(w)).toEqual(['minted-1'])
    expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  })
}

test('adopts a main-thread revive: re-login, entry rewritten, notice verbatim, push re-bound, and the next epoch row names it', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  await start($, w)
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  rememberRows(w)
  w.known.add('revived')
  const calls = w.mcpCalls.length
  const res = await read($, w, 'workflow_manual', { session_key: 'revived' })
  expect(seen(res)).toBe(
    `${keyedResponse('revived', '(no todos)')}\n\n[${w.plugin}] Lead session key changed: minted-1 -> revived. Keep passing revived as session_key; this conversation resumes with it, after compaction too. Mailbox push is bound to revived: arriving mail starts your next turn even when you are idle, so launch no mailbox wait.`,
  )
  expect(w.mcpCalls[calls]).toEqual({ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'ferrule', args: { root: '/repo/sub', format: 'json', relogin_session_key: 'revived' } })
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['minted-1', 'revived'])
  expect(w.killed).toEqual([0])
  // Again with the adopted key: unrewritten, adopted once.
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'revived' }))).toBe(keyedResponse('revived', '(no todos)'))
  // The epoch's row stays as delivered; after compaction the next expansion names the adopted key.
  await expand($, w, 'lead-run', shim('Run', 'lead-run'))
  expect(w.rows.length).toBe(1)
  w.messages = []
  await expand($, w, 'lead-run', shim('Run', 'lead-run'))
  expect(w.rows.at(-1)).toBe(ROW(w, 'revived', '(no todos)', true))
  expect(boundKeys(w)).toEqual(['minted-1', 'revived'])
})

test('adopts the key a main-thread sentinel call minted: the lead reads the result whole, and the push re-binds to it', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  await start($, w)
  // ws-mcp's FRESH-with-root answer: the manual keyed to the key it minted.
  w.known.add('fresh-1')
  w.coreManual = keyedResponse('fresh-1', '(no todos)')
  const calls = w.mcpCalls.length
  const res = await read($, w, 'workflow_manual', { session_key: 'obsidian-latch', root: '/repo/sub' })
  expect(seen(res)).toBe(`${keyedResponse('fresh-1', '(no todos)')}\n\n${adoptionNotice(w.plugin, 'minted-1', 'fresh-1', true)}`)
  expect(w.mcpCalls[calls]).toEqual({ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'ferrule', args: { root: '/repo/sub', format: 'json', relogin_session_key: 'fresh-1' } })
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'fresh-1', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['minted-1', 'fresh-1'])
  expect(w.killed).toEqual([0])
})

for (const variant of ['subagent sentinel', 'keyless sentinel', 'ferrule'] as const) {
  test(`a ${variant} call leaves the push where it is`, LIVE_CHILD, async ($, on) => {
    const w = world(on, { knobs: ON, processOk: true })
    await start($, w)
    w.known.add('fresh-1')
    const calls = w.mcpCalls.length
    if (variant === 'subagent sentinel') {
      w.coreManual = keyedResponse('fresh-1', '(no todos)')
      expect(seen(await read($, w, 'workflow_manual', { session_key: 'obsidian-latch', root: '/repo/sub' }, 'sub-1'))).toBe(w.coreManual)
    } else if (variant === 'keyless sentinel') {
      // No root: ws-mcp mints nothing and the result carries no ## Session Key line.
      w.coreManual = `${STATIC_BODY}\nCall ferrule to bootstrap.\n`
      expect(seen(await read($, w, 'workflow_manual', { session_key: 'obsidian-latch' }))).toBe(w.coreManual)
    } else {
      await $.tool.call({ tool: `mcp__plugin_${w.plugin}_${w.plugin}__ferrule`, root: '/repo/sub' } as never)
      await w.clock.settle()
    }
    expect(w.mcpCalls.length).toBe(calls)
    expect(boundKeys(w)).toEqual(['minted-1'])
    expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  })
}

test('binds automatically at a re-login on resume or hot reload, and at the lazy mint after /clear', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true, store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: 1_800_000_000_000 } } })
  w.known.add('kept-key')
  await start($, w)
  expect(boundKeys(w)).toEqual(['kept-key'])
  await clear($, w)
  expect(w.killed).toEqual([0])
  w.sessionId = 'sess-2'
  await turnStart($, w)
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-2`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['kept-key', 'minted-1'])
  // A hot reload runs session.start again: the re-login binds the same key anew.
  await start($, w)
  expect(w.mcpCalls.filter((c) => c.tool === 'ferrule').at(-1)?.args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'minted-1' })
  expect(boundKeys(w)).toEqual(['kept-key', 'minted-1', 'minted-1'])
})

test('a key produced before the watcher exists is bound once it does', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true, probeDelayMs: 1_000 })
  const started = $.session.start({ cwd: '/repo/sub', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
  const mintedEarly = w.mcpCalls.some((c) => c.tool === 'ferrule')
  const spawnedEarly = w.spawns.length
  await w.clock.advance(1_000)
  await started
  await w.clock.settle()
  expect(mintedEarly).toBe(true)
  expect(spawnedEarly).toBe(0)
  expect(boundKeys(w)).toEqual(['minted-1'])
})

test('a session end that lands while the mint runs binds nothing', async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true, holdMintMs: 1_000 })
  await start($, w)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.advance(1_000)
  expect(w.spawns).toEqual([])
  expect(w.store.has(`${KEY_STORE_PREFIX}sess-1`)).toBe(false)
})

test('a project check that cannot tell reads as closed for both features: the lever registers and nothing is minted', async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true, repo: 'none', ancestorsThrow: true })
  await start($, w)
  expect(watchTool(w)).toBe(true)
  await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(w.mcpCalls).toEqual([])
  expect(w.rows).toEqual([])
})

test('without ai-docs/WORKFLOW.md the lever registers and binds as before, the knob on and unread', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true, workflowFile: false })
  await start($, w)
  expect(manualReads(w)).toEqual([])
  expect(watchTool(w)).toBe(true)
  expect(w.spawns).toEqual([])
  const res = await $.tool.call({ tool: `mcp__${w.plugin}__mailbox_watch`, session_key: 'k1' } as never)
  await w.clock.settle()
  expect(String(res.result)).toContain('bound')
  expect(boundKeys(w)).toEqual(['k1'])
  expect(w.mcpCalls).toEqual([])
})

test('a session end before the watcher exists drops the recorded key, so the new watcher binds nothing', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true, probeDelayMs: 1_000 })
  const started = $.session.start({ cwd: '/repo/sub', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
  const minted = w.mcpCalls.some((c) => c.tool === 'ferrule')
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.advance(1_000)
  await started
  await w.clock.settle()
  expect(minted).toBe(true)
  expect(w.spawns).toEqual([])
})

test('a session end cancels an automatic bind whose slug lookup is still in flight', LIVE_CHILD, async ($, on) => {
  // The direct drain: only it resolves the key's slug through lookup_peers.
  const w = world(on, { knobs: { ...ON, [KNOB.drain]: 'direct' }, processOk: true, lookupDelayMs: 1_000 })
  await start($, w)
  expect(w.spawns).toEqual([])
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.advance(1_000)
  expect(w.spawns).toEqual([])
})

test('preload: a ws lead skill block is replaced with its playbook body, read with the conversation key', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  const text = await expand($, w, 'lead-discuss', shim('Discuss', 'lead-discuss'))
  expect(text).toBe('# lead-discuss\n\nThe lead-discuss procedure.\n\nARGUMENTS: decide the thing\n')
  expect(playbookReads(w)).toEqual([{ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'playbook_read', args: { name: 'lead-discuss', session_key: 'minted-1' } }])
  // A namespaced skill name expands the same way.
  expect(await expand($, w, `${w.plugin}:lead-ticket`, shim('Ticket', 'lead-ticket'))).toContain('The lead-ticket procedure.')
})

test('preload: another plugin, a project skill, and a block naming a tool instead of a playbook pass through unread', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  const original = shim('Discuss', 'lead-discuss')
  expect(await expand($, w, 'lead-discuss', original, 'other-plugin')).toBe(original)
  expect(await expand($, w, 'lead-discuss', original, null)).toBe(original)
  const toolBlock = '# Discuss\n\n<ws-preload tool="workflow_state">\nCall `ws/workflow_state`.\n</ws-preload>\n'
  expect(await expand($, w, 'lead-discuss', toolBlock)).toBe(toolBlock)
  expect(playbookReads(w)).toEqual([])
  expect(w.mcpCalls.filter((c) => c.tool === 'workflow_state')).toEqual([])
})

test('preload: lead-use-mailbox and a block naming another playbook pass through unread', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  const mailbox = shim('Use Mailbox', 'lead-use-mailbox')
  expect(await expand($, w, 'lead-use-mailbox', mailbox)).toBe(mailbox)
  const other = shim('Discuss', 'lead-run')
  expect(await expand($, w, 'lead-discuss', other)).toBe(other)
  expect(playbookReads(w)).toEqual([])
})

test('preload: a failed read passes the text through unchanged', async ($, on) => {
  const w = world(on, { knobs: ON })
  await start($, w)
  w.playbookText = () => undefined
  const original = shim('Ship', 'lead-ship')
  expect(await expand($, w, 'lead-ship', original)).toBe(original)
  expect(playbookReads(w).map((c) => c.args.name)).toEqual(['lead-ship'])
})

test('preload: with no key held (the knob off, or outside a ws project) the block passes through unread', async ($, on) => {
  const w = world(on)
  await start($, w)
  const original = shim('Discuss', 'lead-discuss')
  expect(await expand($, w, 'lead-discuss', original)).toBe(original)
  expect(playbookReads(w)).toEqual([])
})

// The playbook is one dedupe unit across the preload and main-thread reads,
// judged by what the conversation still shows.

/** Record an expansion the model saw as the Skill tool delivers it: its own result, then the text block. */
function rememberSkill(w: World, skill: string, text: string) {
  const id = `sk-${w.messages.length}`
  w.messages.push({ role: 'assistant', content: [{ type: 'tool_use', id, name: 'Skill', input: { skill } }] })
  w.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: id, content: `Launching skill: ${skill}` }, { type: 'text', text }] })
}

/** A playbook text whose trimmed form equals what core answers for a model read of it. */
const sameBody = (name: string) => `# ${name}\n## Steps\nwork\n`

/** The expansion's pointer form: the pointer line ahead of the block's fallback prose. */
const pointed = (title: string, playbook: string) => `${preloadPointerLine(playbook)}\n\n${fallback(title, playbook)}\n\nARGUMENTS: decide the thing\n`

test('preload dedupe: a skill invoked twice gets the pointer and its fallback prose the second time, and a read after that gets the full body', async ($, on) => {
  const w = world(on, { knobs: ON })
  w.playbookText = sameBody
  await start($, w)
  const first = await expand($, w, 'lead-run', shim('Run', 'lead-run'))
  expect(first).toBe('# lead-run\n## Steps\nwork\n\nARGUMENTS: decide the thing\n')
  // The one conversation read is the row's epoch scan: the body's hint was new.
  expect(w.messageReads).toBe(1)
  rememberSkill(w, `${w.plugin}:lead-run`, first)
  const second = await expand($, w, 'lead-run', shim('Run', 'lead-run'))
  expect(second).toBe(pointed('Run', 'lead-run'))
  rememberSkill(w, `${w.plugin}:lead-run`, second)
  // A read after a pointer of either kind means the reader lost the copy: the full body.
  const args = { name: 'lead-run', session_key: 'minted-1' }
  expect(seen(await read($, w, 'playbook_read', args))).toBe('# lead-run\n## Steps\nwork')
})

test('preload dedupe: a read after the expansion gets the pointer naming the inlined copy; gone from the conversation, the full body', async ($, on) => {
  const w = world(on, { knobs: ON })
  w.playbookText = sameBody
  await start($, w)
  rememberSkill(w, `${w.plugin}:lead-run`, await expand($, w, 'lead-run', shim('Run', 'lead-run')))
  const args = { name: 'lead-run', session_key: 'minted-1' }
  const res = await read($, w, 'playbook_read', args)
  expect(res.result).toBe(inlinedPointerText('# lead-run\n## Steps\nwork'))
  expect(res.result).not.toContain('earlier `playbook_read` call')
  // A compaction took the expansion away: the read and the next expansion are full again.
  w.messages = []
  expect(seen(await read($, w, 'playbook_read', args))).toBe('# lead-run\n## Steps\nwork')
  w.messages = []
  expect(await expand($, w, 'lead-run', shim('Run', 'lead-run'))).toBe('# lead-run\n## Steps\nwork\n\nARGUMENTS: decide the thing\n')
})

test('preload dedupe: an expansion after a visible read gets the pointer and its fallback prose; a context read with a different body stays full', async ($, on) => {
  const w = world(on, { knobs: ON })
  w.playbookText = sameBody
  await start($, w)
  const args = { name: 'lead-ticket', session_key: 'minted-1' }
  const full = seen(await read($, w, 'playbook_read', args))!
  remember(w, 'playbook_read', args, full)
  expect(await expand($, w, 'lead-ticket', shim('Ticket', 'lead-ticket'))).toBe(pointed('Ticket', 'lead-ticket'))
  // Another context that renders a different body is another unit: full.
  w.messages = []
  w.playbookText = (name, key) => (key === 'minted-1' ? `# ${name}\n## Other context\nwork\n` : undefined)
  rememberSkill(w, `${w.plugin}:lead-ticket`, await expand($, w, 'lead-ticket', shim('Ticket', 'lead-ticket')))
  const ctx = { name: 'lead-ticket', session_key: 'minted-1', context: { mode: 'x' } }
  expect(seen(await read($, w, 'playbook_read', ctx))).toBe('# lead-ticket\n## Steps\nwork')
})

test('preload dedupe: lead-use-mailbox and another plugin pass through on a repeat too, and read nothing', async ($, on) => {
  const w = world(on, { knobs: ON })
  w.playbookText = sameBody
  await start($, w)
  const mailbox = shim('Use Mailbox', 'lead-use-mailbox')
  const other = shim('Discuss', 'lead-discuss')
  for (let i = 0; i < 2; i++) {
    expect(await expand($, w, 'lead-use-mailbox', mailbox)).toBe(mailbox)
    expect(await expand($, w, 'lead-discuss', other, 'other-plugin')).toBe(other)
    rememberSkill(w, 'x', mailbox)
  }
  expect(playbookReads(w)).toEqual([])
  expect(w.messageReads).toBe(0)
  expect(w.rows).toEqual([])
})

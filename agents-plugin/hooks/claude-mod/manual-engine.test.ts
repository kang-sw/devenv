import { expect, mock, test, type TestBody } from 'claude-code/testing'

import { CONTINUATION_WINDOW_MS } from './compaction'
import { adoptionNotice, KEY_STORE_PREFIX, MAILBOX_LINE, pointerText, SECTION_ID } from './manual'
import { KNOB } from './register'
import { resumeText } from './timing'

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

/** The injected section for `key`; `plugin` is the engine-reported name, which the key line's revive skill spells. */
const INJECTED = (plugin: string, key: string) =>
  'Your session state (agenda, todos, notes) is not in this prompt. When no `workflow_manual` result is visible in your context — a fresh session, or after compaction — call `workflow_manual` before acting.\n\n' +
  `Your lead session key for this conversation is \`${key}\`; pass it as \`session_key\`. Continuing another track (for example \`${plugin}:lead-revive\` with a preserved key) is still allowed.\n\n` +
  keyedResponse(key, '').split('\n## Session Key')[0].trimEnd()

/** The section where the mailbox push runs: the mailbox line follows the key line. */
const INJECTED_PUSH = (plugin: string, key: string) => {
  const [pointer, keyed, ...rest] = INJECTED(plugin, key).split('\n\n')
  return [pointer, keyed, MAILBOX_LINE, ...rest].join('\n\n')
}

const ENGINE_SECTIONS = [
  { id: 'intro', text: 'You are Claude Code.', scope: 'shared' as const },
  { id: 'env', text: 'Platform: darwin', scope: 'session' as const },
]

type World = ReturnType<typeof world>

function world(
  on: On,
  opts: {
    store?: Record<string, unknown>
    workflowFile?: boolean
    repo?: 'none'
    /** fs.ancestors rejects: with repo 'none', the project check cannot tell. */
    ancestorsThrow?: boolean
    holdManualMs?: number
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
    /** What core answers for ws reads the model makes; the conversation the dedupe reads. */
    messages: [] as { role: string; content: Record<string, unknown>[] }[],
    messageReads: 0,
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
    // Knob reads: recorded apart from the manual's own ws calls.
    if (e.tool === 'config_get') {
      const key = String(e.args.key)
      w.configCalls.push({ ...e.args })
      if (w.failKnobs.has(key)) return { value: { content: [{ type: 'text', text: 'ws-mcp is unhealthy' }], isError: true } }
      const knobs: Record<string, unknown> = { 'claude.compaction_advisory_percent': '50%', ...(opts.knobs ?? {}) }
      const value = key in knobs && !(opts.sessionScoped && e.args.session_key === undefined) ? knobs[key] : null
      return text(JSON.stringify({ key, value, scope: value === null ? 'unset' : 'project' }))
    }
    w.mcpCalls.push({ server: e.server, tool: e.tool, args: e.args })
    if (e.tool === 'ferrule') {
      const relogin = e.args.relogin_session_key
      if (typeof relogin === 'string') {
        return w.known.has(relogin)
          ? text(JSON.stringify({ session_key: relogin, root: '/repo' }))
          : { value: { content: [{ type: 'text', text: 'not a known session key' }], isError: true } }
      }
      if (opts.failMint) return { value: { content: [{ type: 'text', text: 'ws-mcp is unhealthy' }], isError: true } }
      if (opts.holdMintMs) await clock.sleep(opts.holdMintMs)
      w.minted += 1
      const key = `minted-${w.minted}`
      w.known.add(key)
      return text(JSON.stringify({ session_key: key, root: '/repo' }))
    }
    if (e.tool === 'workflow_manual') {
      if (opts.holdManualMs) await clock.sleep(opts.holdManualMs)
      return text(keyedResponse(e.args.session_key as string, w.state))
    }
    if (e.tool === 'playbook_read') return text(STATIC_BODY)
    if (e.tool === 'workflow_state') return text(`## Session State\n\n### Todos\n${w.state}\n`)
    return { value: { content: [{ type: 'text', text: `unknown tool ${e.tool}` }], isError: true } }
  })
  on('session.messages', () => {
    w.messageReads += 1
    return { value: w.messages as never }
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
  on('prompt.compose', () => ({ sections: ENGINE_SECTIONS }))
  // A reconnect always brings the server's tools back.
  on('command.run', () => ((w.reconnects += 1), {}))
  on('tool.list', () => ({ value: [{ name: `mcp__plugin_${w.plugin}_${w.plugin}__config_get`, description: 'x', mcp: true }] }))
  on('session.compact', () => ({ messages: [{ role: 'user', text: 'summary', toolUses: [] }] }) as never)
  on('prompt.submit', ($, e) => (w.prompts.push(e.text), { text: e.text }))
  return w
}

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/repo/sub', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

async function compose($: Engine, w: World, traits: string[] = []) {
  const res = await $.prompt.compose({ model: 'm', promptModel: 'm', surfaces: ['terminal'], tools: [], outputStyle: null, traits } as never)
  await w.clock.settle()
  return res
}

const manualSection = (res: { sections: readonly { id: string; text: string; scope: string }[] }) => res.sections.find((s) => s.id === SECTION_ID)

async function read($: Engine, w: World, tool: 'workflow_manual' | 'playbook_read', args: Record<string, unknown>, agentId?: string) {
  const res = await $.tool.call({ tool: `mcp__plugin_${w.plugin}_${w.plugin}__${tool}`, ...args, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

/** Record a read the model saw into the conversation the dedupe reads. */
function remember(w: World, tool: string, args: Record<string, unknown>, text: string) {
  const id = `tu-${w.messages.length}`
  w.messages.push({ role: 'assistant', content: [{ type: 'tool_use', id, name: `mcp__plugin_${w.plugin}_${w.plugin}__${tool}`, input: args }] })
  w.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: id, content: [{ type: 'text', text }] }] })
}

const seen = (res: { result?: unknown; text?: string }) => (typeof res.result === 'string' ? res.result : res.text)

/** The feature's knob on. */
const ON = { [KNOB.manual]: true }
/** The manual knob's reads. */
const manualReads = (w: World) => w.configCalls.filter((a) => a.key === KNOB.manual)

test('in a ws project: a key is minted at start and the first compose appends the pinned section last', async ($, on) => {
  const w = world(on, { knobs: ON, knobs: ON })
  await start($, w)
  expect(w.mcpCalls.map((c) => c.tool)).toEqual(['ferrule', 'workflow_manual', 'playbook_read'])
  expect(w.mcpCalls[0]).toEqual({ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'ferrule', args: { root: '/repo/sub', format: 'json' } })
  // The knob is read once, keyless: no key exists yet to read it with.
  expect(manualReads(w)).toEqual([{ key: KNOB.manual, format: 'json' }])
  // The project check reads the repository root, not the launch directory.
  expect(w.existsChecks).toEqual(['/repo/ai-docs/WORKFLOW.md'])
  const res = await compose($, w)
  expect(res.sections.slice(0, 2)).toEqual(ENGINE_SECTIONS)
  expect(res.sections.at(-1)).toEqual({ id: 'ws:workflow-manual', text: INJECTED(w.plugin, 'minted-1'), scope: 'session' })
  // Later composes append the same section without asking ws-mcp again.
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'minted-1'))
  expect(w.mcpCalls.length).toBe(3)
})

test('without ai-docs/WORKFLOW.md at the repository root mints nothing and appends nothing', async ($, on) => {
  const w = world(on, { knobs: ON, workflowFile: false })
  await start($, w)
  expect(w.mcpCalls).toEqual([])
  expect(manualSection(await compose($, w))).toBeUndefined()
})

test('outside a repository falls back to the ancestor walk', async ($, on) => {
  const w = world(on, { knobs: ON, repo: 'none' })
  await start($, w)
  expect(w.existsChecks).toEqual([])
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'minted-1'))
})

test('a stored key (hot reload, resume) re-logs in instead of minting', async ($, on) => {
  const w = world(on, { knobs: ON, store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: 1_800_000_000_000 - 60_000 } } })
  w.known.add('kept-key')
  await start($, w)
  expect(w.mcpCalls[0].args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'kept-key' })
  expect(w.minted).toBe(0)
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'kept-key'))
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'kept-key', at: 1_800_000_000_000 })
  // A replayed snapshot: the full workflow_manual result passes through.
  const res = await read($, w, 'workflow_manual', { session_key: 'kept-key' })
  expect(seen(res)).toBe(keyedResponse('kept-key', '(no todos)'))
})

test('/clear ends the conversation; the next compose mints a new key under the new session id, and stale entries are pruned at start', async ($, on) => {
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
  await compose($, w)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  w.sessionId = 'sess-2'
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'minted-2'))
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: now })
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-2`)).toEqual({ sessionKey: 'minted-2', at: now })
})

test('the first compose waits a bounded time; on timeout it appends nothing and the full result passes through', async ($, on) => {
  const w = world(on, { knobs: ON, holdManualMs: 8_000 })
  await start($, w)
  const pending = $.prompt.compose({ model: 'm', promptModel: 'm', surfaces: ['terminal'], tools: [], outputStyle: null, traits: [] } as never)
  await w.clock.advance(5_000)
  const res = await pending
  expect(manualSection(res)).toBeUndefined()
  await w.clock.advance(3_000)
  // Cached by now: a later compose appends it at once (its answer is discarded), and no rewrite follows.
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'minted-1'))
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toBe(keyedResponse('minted-1', '(no todos)'))
})

test('a compose while the setup is still pending answers at once, without the section', async ($, on) => {
  const w = world(on, { knobs: ON, holdManualMs: 8_000 })
  await start($, w)
  const first = $.prompt.compose({ model: 'm', promptModel: 'm', surfaces: ['terminal'], tools: [], outputStyle: null, traits: [] } as never)
  await w.clock.advance(5_000)
  expect(manualSection(await first)).toBeUndefined()
  let answered: { sections: readonly { id: string }[] } | undefined
  void $.prompt.compose({ model: 'm', promptModel: 'm', surfaces: ['terminal'], tools: [], outputStyle: null, traits: [] } as never).then((res) => (answered = res))
  await w.clock.settle()
  expect(answered).toEqual({ sections: ENGINE_SECTIONS })
})

test('a failed setup appends nothing, and the full result passes through', async ($, on) => {
  const w = world(on, { knobs: ON, failMint: true })
  await start($, w)
  expect(manualSection(await compose($, w))).toBeUndefined()
  expect(w.mcpCalls.map((c) => c.tool)).toEqual(['ferrule', 'ferrule'])
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toBe(keyedResponse('minted-1', '(no todos)'))
  expect(w.store.size).toBe(0)
})

test('a workflow_manual whose anchors moved reads as workflow_state with the same key; a body-less notice passes through', async ($, on) => {
  const w = world(on, { knobs: ON, knobs: ON })
  await start($, w)
  await compose($, w)
  w.coreManual = '# Renamed Manual\n\n## Session Key\nminted-1\n\n## Session State\n'
  const res = await read($, w, 'workflow_manual', { session_key: 'minted-1', root: '/repo' })
  expect(res.result).toBe('Workflow manual is in your system prompt; this is your current session state.\n\n## Session State\n\n### Todos\n(no todos)\n')
  expect(w.mcpCalls.at(-1)).toEqual({ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'workflow_state', args: { session_key: 'minted-1' } })
  const calls = w.mcpCalls.length
  w.coreManual = '## Session State\n(no restorable state for session key "gone")\n'
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'gone' }))).toBe(w.coreManual)
  // Only the refused adoption's re-login: no workflow_state fallback.
  expect(w.mcpCalls.slice(calls).map((c) => c.tool)).toEqual(['ferrule'])
})

test('with the snapshot carrying the manual, a main-thread workflow_manual reads as state only; a subagent call is untouched', async ($, on) => {
  const w = world(on, { knobs: ON, knobs: ON })
  await start($, w)
  await compose($, w)
  w.state = '- [ ] {a} do it'
  const res = await read($, w, 'workflow_manual', { session_key: 'minted-1' })
  expect(res.result).toBe(
    'Workflow manual is in your system prompt; this is your current session state.\n\n# Manuals\n- none\n\n## Session Key\nminted-1\n\n## Session State\n\n### Todos\n- [ ] {a} do it\n',
  )
  const fromSub = await read($, w, 'workflow_manual', { session_key: 'minted-1' }, 'sub-1')
  expect(seen(fromSub)).toBe(keyedResponse('minted-1', '- [ ] {a} do it'))
  // Another key's manual differs in its static span (its key line): passed
  // through. ws-mcp refuses to re-log in with it, so it is not adopted either.
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'other-key' }))).toBe(keyedResponse('other-key', '- [ ] {a} do it'))
})

test("the lever's own landing sets the flag, though the engine routes it past the mod's session.compact hook", async ($, on) => {
  const w = world(on, { knobs: ON, store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: 1_800_000_000_000 } } })
  w.known.add('kept-key')
  await start($, w)
  await compose($, w)
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'kept-key' }))).toBe(keyedResponse('kept-key', '(no todos)'))
  await $.tool.call({ tool: `mcp__${w.plugin}__request_compaction`, continue: true } as never)
  await w.clock.settle()
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: false, turnId: 't', reason: 'answer' } as never)
  await w.clock.settle()
  await w.clock.advance(CONTINUATION_WINDOW_MS)
  expect(w.prompts).toEqual([resumeText(w.plugin)])
  await compose($, w)
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'kept-key' }))).toMatch(/^Workflow manual is in your system prompt;/)
})

test('a manual compaction landing sets the flag for a resumed conversation', async ($, on) => {
  const w = world(on, { knobs: ON, store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: 1_800_000_000_000 } } })
  w.known.add('kept-key')
  await start($, w)
  await compose($, w)
  await $.session.compact({ trigger: 'manual', messages: [{ role: 'user', text: 'summary', toolUses: [] }] } as never)
  await w.clock.settle()
  await compose($, w)
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'kept-key' }))).toMatch(/^Workflow manual is in your system prompt;/)
})

test('a repeat playbook_read collapses to the pointer while the earlier result is in the conversation, read only on a hint match', async ($, on) => {
  const w = world(on, { knobs: ON, knobs: ON })
  await start($, w)
  await compose($, w)
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
  remember(w, 'playbook_read', args, pointerText('playbook_read', full))
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

test('a repeat workflow_manual collapses to its pointer on the state-only text', async ($, on) => {
  const w = world(on, { knobs: ON, knobs: ON })
  await start($, w)
  await compose($, w)
  const first = seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))!
  remember(w, 'workflow_manual', { session_key: 'minted-1' }, first)
  const second = await read($, w, 'workflow_manual', { session_key: 'minted-1' })
  expect(second.result).toBe(
    'The result is unchanged. Continue using the full result of your earlier `workflow_manual` call in this conversation as your current session state; call workflow_manual again after you change agenda, todos, or notes. (headings: # Manuals; ## Session Key; ## Session State; ### Todos)',
  )
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
  expect(await compose($, w)).toEqual({ sections: ENGINE_SECTIONS })
  w.failKnobs.delete(KNOB.manual)
  await reconnect($, w)
  expect(w.reconnects).toBe(1)
  expect(manualReads(w)).toEqual([
    { key: KNOB.manual, format: 'json' },
    { key: KNOB.manual, format: 'json' },
  ])
  expect(w.mcpCalls.map((c) => c.tool)).toEqual(['ferrule', 'workflow_manual', 'playbook_read'])
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: 1_800_000_000_000 })
  // The first compose is past: the full result passes through until a landing carries the section.
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toBe(keyedResponse('minted-1', '(no todos)'))
  await $.session.compact({ trigger: 'manual', messages: [{ role: 'user', text: 'summary', toolUses: [] }] } as never)
  await w.clock.settle()
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'minted-1'))
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'minted-1' }))).toMatch(/^Workflow manual is in your system prompt;/)
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
  expect(await compose($, w)).toEqual({ sections: ENGINE_SECTIONS })
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
  test(`a manual knob ${what} in a ws project: no key, no section, no rewrite, and mailbox_watch registers and binds`, LIVE_CHILD, async ($, on) => {
    const w = world(on, { ...opts, processOk: true })
    await start($, w)
    // The project check passed; the knob closed the gate.
    expect(w.existsChecks).toEqual(['/repo/ai-docs/WORKFLOW.md'])
    expect(manualReads(w)).toEqual('serverDown' in opts ? [] : [{ key: KNOB.manual, format: 'json' }])
    expect(w.mcpCalls).toEqual([])
    const res = await compose($, w)
    expect(res).toEqual({ sections: ENGINE_SECTIONS })
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

test('in a ws project: no mailbox_watch tool; the minted key is bound automatically and the section says so', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  await start($, w)
  expect(watchTool(w)).toBe(false)
  expect(boundKeys(w)).toEqual(['minted-1'])
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED_PUSH(w.plugin, 'minted-1'))
  // The automatic bind leaves no lever binding behind.
  expect([...w.store.keys()].some((k) => k.startsWith('mailbox-watch:'))).toBe(false)
})

test('without host commands: no tool, no bind, and no mailbox line', async ($, on) => {
  const w = world(on, { knobs: ON, knobs: ON })
  await start($, w)
  expect(watchTool(w)).toBe(false)
  expect(w.spawns).toEqual([])
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED(w.plugin, 'minted-1'))
})

for (const variant of ['subagent', 'errored'] as const) {
  test(`${variant === 'subagent' ? 'a subagent' : 'an errored'} workflow_manual call adopts nothing`, LIVE_CHILD, async ($, on) => {
    const w = world(on, { knobs: ON, processOk: true })
    await start($, w)
    await compose($, w)
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

test('adopts a main-thread revive: re-login, entry rewritten, notice verbatim, push re-bound, and the section follows at the next landing', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  await start($, w)
  await compose($, w)
  w.known.add('revived')
  const calls = w.mcpCalls.length
  const res = await read($, w, 'workflow_manual', { session_key: 'revived' })
  expect(seen(res)).toBe(
    `${keyedResponse('revived', '(no todos)')}\n\n[${w.plugin}] Lead session key changed: minted-1 -> revived. Keep passing revived as session_key; this conversation resumes with it, and the system prompt names it after the next /compact or compaction request. Mailbox push is bound to revived: arriving mail starts your next turn even when you are idle, so launch no mailbox wait.`,
  )
  expect(w.mcpCalls[calls]).toEqual({ server: `plugin:${w.plugin}:${w.plugin}`, tool: 'ferrule', args: { root: '/repo/sub', format: 'json', relogin_session_key: 'revived' } })
  expect(w.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: 1_800_000_000_000 })
  expect(boundKeys(w)).toEqual(['minted-1', 'revived'])
  expect(w.killed).toEqual([0])
  // The snapshot still names minted-1 until a refreshing reset.
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'revived' }))).toBe(keyedResponse('revived', '(no todos)'))
  await $.session.compact({ trigger: 'manual', messages: [{ role: 'user', text: 'summary', toolUses: [] }] } as never)
  await w.clock.settle()
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED_PUSH(w.plugin, 'revived'))
  expect(seen(await read($, w, 'workflow_manual', { session_key: 'revived' }))).toMatch(/^Workflow manual is in your system prompt;/)
  expect(boundKeys(w)).toEqual(['minted-1', 'revived'])
})

test('adopts the key a main-thread sentinel call minted: the lead reads the result whole, and the push re-binds to it', LIVE_CHILD, async ($, on) => {
  const w = world(on, { knobs: ON, processOk: true })
  await start($, w)
  await compose($, w)
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
    await compose($, w)
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
  await compose($, w)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  expect(w.killed).toEqual([0])
  w.sessionId = 'sess-2'
  expect(manualSection(await compose($, w))?.text).toBe(INJECTED_PUSH(w.plugin, 'minted-1'))
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
  expect(w.mcpCalls).toEqual([])
  expect(manualSection(await compose($, w))).toBeUndefined()
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

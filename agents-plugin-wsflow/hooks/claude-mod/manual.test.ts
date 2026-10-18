import { describe, expect, test } from 'claude-code/testing'

import {
  COMPOSE_WAIT_MS,
  createManual,
  cutStaticBody,
  hashText,
  headings,
  isRepeat,
  KEY_RETENTION_MS,
  KEY_STORE_PREFIX,
  landingRefreshes,
  manualPart,
  pointerText,
  readKey,
  sectionText,
  stateOnlyText,
  type ApiMessageLike,
  type ManualEdges,
} from './manual'

// The static manual body as `playbook_read` renders it with a key, and a keyed
// `workflow_manual` response around it: ws-mcp prepends notices, injects the
// key line into the body, and appends the key, the state, and the notes.
const STATIC_BODY = [
  '# Workflow Manual',
  '',
  '> **Session invariant:** Preserve the `session_key` verbatim across compaction.',
  '',
  '## On: invoke',
  '',
  'Load this manual as the current workflow reference.',
  '',
  '### User preferences',
  '',
  'Keep it short.',
  '',
].join('\n')

function response(key: string, opts: { state?: string; prefs?: string; prepend?: string } = {}): string {
  const body = STATIC_BODY.replace('across compaction.', `across compaction.\n> **Session key: \`${key}\`** — preserve verbatim in any compaction summary.`).replace(
    'Keep it short.',
    opts.prefs ?? 'Keep it short.',
  )
  return [
    opts.prepend ?? '# Manuals\n- build.md — Build steps',
    '',
    body.trimEnd(),
    '',
    '## Session Key',
    key,
    '',
    '## Session State',
    '',
    '### Todos',
    opts.state ?? '(no todos)',
    '',
    '# Notes',
    '- none',
    '',
  ].join('\n')
}

const tick = async (n = 40) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

const NOW = 1_800_000_000_000
const SERVER = 'plugin:ws:ws'
const WM_TOOL = 'mcp__plugin_ws_ws__workflow_manual'
const PR_TOOL = 'mcp__plugin_ws_ws__playbook_read'

type Call = { server: string; tool: string; args: Record<string, unknown> }

/** Fake edges over a fake ws-mcp: calls are recorded, timers fire when the test says. */
function harness(opts: { store?: Record<string, unknown>; wsProject?: boolean | 'rejects'; server?: string | undefined } = {}) {
  const h = {
    calls: [] as Call[],
    store: new Map<string, unknown>(Object.entries(opts.store ?? {})),
    wsProject: opts.wsProject ?? true,
    gateChecks: 0,
    sessionId: 'sess-1',
    root: '/repo/sub',
    now: NOW,
    minted: 0,
    /** Keys a re-login accepts. */
    known: new Set<string>(),
    /** Tools whose calls fail while listed. */
    failing: new Set<string>(),
    /** While set, the keyed workflow_manual fetch waits on it. */
    hold: undefined as Promise<void> | undefined,
    /** The state a keyed workflow_manual renders. */
    state: '(no todos)',
    prefs: undefined as string | undefined,
    timers: [] as { ms: number; fn: () => void; cancelled: boolean }[],
    messages: [] as ApiMessageLike[],
    messageReads: 0,
    logs: [] as { text: string; to: string }[],
  }
  const m = createManual()
  const edges: ManualEdges = {
    isWsProject: async () => {
      h.gateChecks += 1
      if (h.wsProject === 'rejects') throw new Error('cannot tell')
      return h.wsProject
    },
    server: async () => ('server' in opts ? opts.server : SERVER),
    call: async (server, tool, args) => {
      h.calls.push({ server, tool, args })
      if (h.failing.has(tool)) throw new Error(`${tool} failed`)
      if (tool === 'ferrule') {
        if (typeof args.relogin_session_key === 'string') {
          if (!h.known.has(args.relogin_session_key)) throw new Error('not a known session key')
          return JSON.stringify({ session_key: args.relogin_session_key, root: '/repo' })
        }
        h.minted += 1
        const key = `minted-${h.minted}`
        h.known.add(key)
        return JSON.stringify({ session_key: key, root: '/repo' })
      }
      if (tool === 'workflow_manual') {
        if (h.hold) await h.hold
        return response(args.session_key as string, { state: h.state, prefs: h.prefs })
      }
      if (tool === 'playbook_read') return STATIC_BODY
      if (tool === 'workflow_state') return `## Session State\n\n### Todos\n${h.state}\n`
      throw new Error(`unexpected tool ${tool}`)
    },
    root: async () => h.root,
    sessionId: async () => h.sessionId,
    store: {
      get: async (k) => h.store.get(k),
      set: async (k, v) => void h.store.set(k, v),
      delete: async (k) => void h.store.delete(k),
      keys: async () => [...h.store.keys()],
    },
    now: async () => h.now,
    timer: (ms, fn) => {
      const t = { ms, fn, cancelled: false }
      h.timers.push(t)
      return { cancel: () => void (t.cancelled = true) }
    },
    messages: async () => {
      h.messageReads += 1
      return h.messages
    },
    log: (text, to) => void h.logs.push({ text, to }),
  }
  /** Fire every live timer of `ms`. */
  const fire = (ms: number) => {
    for (const t of h.timers.splice(0)) if (!t.cancelled && t.ms === ms) t.fn()
  }
  const tools = () => h.calls.map((c) => c.tool)
  return { h, m, edges, fire, tools }
}

/** A conversation holding the given earlier results of calls to `tool`. */
function conversation(tool: string, results: { input: Record<string, unknown>; text: string }[]): ApiMessageLike[] {
  return results.flatMap((r, i) => [
    { role: 'assistant', content: [{ type: 'tool_use', id: `tu-${i}`, name: tool, input: r.input }] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: `tu-${i}`, content: [{ type: 'text', text: r.text }] }] },
  ])
}

describe('pinned prose', () => {
  test('the section, the state-only line, and both pointers read as pinned', () => {
    expect(sectionText('k-1', 'MANUAL')).toBe(
      'Your session state (agenda, todos, notes) is not in this prompt. When no `workflow_manual` result is visible in your context — a fresh session, or after compaction — call `workflow_manual` before acting.\n\n' +
        'Your lead session key for this conversation is `k-1`; pass it as `session_key`. Continuing another track (for example `ws:lead-revive` with a preserved key) is still allowed.\n\n' +
        'MANUAL',
    )
    expect(stateOnlyText('STATE')).toBe('Workflow manual is in your system prompt; this is your current session state.\n\nSTATE')
    const text = '# One\nbody\n## Two\n### Three\n#### Four'
    expect(pointerText('playbook_read', text)).toBe(
      'The result is unchanged. Continue using the full result of your earlier `playbook_read` call in this conversation. Do not read it again (headings: # One; ## Two; ### Three).',
    )
    expect(pointerText('workflow_manual', text)).toBe(
      'The result is unchanged. Continue using the full result of your earlier `workflow_manual` call in this conversation as your current session state; call workflow_manual again after you change agenda, todos, or notes. (headings: # One; ## Two; ### Three)',
    )
    expect(headings('no headings here')).toBe('(no headings)')
  })
})

describe('pure helpers', () => {
  test('the injected manual is the keyed response before its ## Session Key line', () => {
    const part = manualPart(response('k-1'))!
    expect(part.startsWith('# Manuals\n')).toBe(true)
    expect(part.endsWith('Keep it short.')).toBe(true)
    expect(part).toContain('**Session key: `k-1`**')
    expect(part).not.toContain('## Session Key')
    expect(manualPart('## Session State\n(no restorable state for session key "x")\n')).toBeUndefined()
  })

  test('the cut removes the static body up to ## Session Key and keeps what ws-mcp put around it', () => {
    const r = response('k-1', { state: '- [ ] {a} do it' })
    const cut = cutStaticBody(r, STATIC_BODY)
    if (!cut.found) throw new Error('cut not found')
    expect(cut.text).toBe('# Manuals\n- build.md — Build steps\n\n## Session Key\nk-1\n\n## Session State\n\n### Todos\n- [ ] {a} do it\n\n# Notes\n- none\n')
    expect(cut.span.startsWith('# Workflow Manual\n')).toBe(true)
    expect(cut.span).toContain('**Session key: `k-1`**')
  })

  test('cut misses: no body, a missing anchor, anchors out of order', () => {
    expect(cutStaticBody('## Session State\n(no restorable state)\n', STATIC_BODY)).toEqual({ found: false, reason: 'no-body' })
    expect(cutStaticBody('# Renamed Manual\n\n## Session Key\nk\n', STATIC_BODY)).toEqual({ found: false, reason: 'start-anchor' })
    expect(cutStaticBody('# Workflow Manual\nbody\n', STATIC_BODY)).toEqual({ found: false, reason: 'end-anchor' })
    expect(cutStaticBody('## Session Key\nk\n# Workflow Manual\n', STATIC_BODY)).toEqual({ found: false, reason: 'order' })
  })

  test('a playbook_read key is name plus context, without the outer session_key; workflow_manual is keyed by session_key', () => {
    const a = readKey('playbook_read', { name: 'lead-run', context: { b: '2', a: '1' }, session_key: 'k-1' })
    expect(readKey('playbook_read', { name: 'lead-run', context: { a: '1', b: '2' }, session_key: 'k-2' })).toBe(a)
    expect(readKey('playbook_read', { name: 'lead-run', context: { a: '1', b: '3' } })).not.toBe(a)
    expect(readKey('playbook_read', { name: 'lead-run', context: { session_key: 'x' } })).not.toBe(readKey('playbook_read', { name: 'lead-run' }))
    expect(readKey('workflow_manual', { session_key: 'k-1' })).not.toBe(readKey('workflow_manual', { session_key: 'k-2' }))
    expect(readKey('workflow_manual', { session_key: 'k-1', root: '/r' })).toBe(readKey('workflow_manual', { session_key: 'k-1' }))
  })

  test('the hint hash tells texts apart', () => {
    expect(hashText('abc')).toBe(hashText('abc'))
    expect(hashText('abc')).not.toBe(hashText('abd'))
    expect(hashText('')).not.toBe(hashText(' '))
  })

  test('a repeat needs exactly one identical earlier result and no pointer to it', () => {
    const input = { name: 'lead-run', session_key: 'k-1' }
    const key = readKey('playbook_read', input)
    const body = '# Run\nsteps'
    const repeat = (results: { input: Record<string, unknown>; text: string }[], tool = PR_TOOL) =>
      isRepeat(conversation(tool, results), PR_TOOL, 'playbook_read', key, body)
    expect(repeat([])).toBe(false)
    expect(repeat([{ input, text: body }])).toBe(true)
    expect(repeat([{ input: { name: 'lead-run', session_key: 'k-2' }, text: body }])).toBe(true)
    expect(repeat([{ input, text: body }], 'mcp__other__playbook_read')).toBe(false)
    expect(repeat([{ input: { name: 'lead-ticket' }, text: body }])).toBe(false)
    expect(repeat([{ input, text: '# Run\nolder steps' }])).toBe(false)
    expect(repeat([{ input, text: body }, { input, text: pointerText('playbook_read', body) }])).toBe(false)
    expect(repeat([{ input, text: body }, { input, text: pointerText('playbook_read', body) }, { input, text: body }])).toBe(false)
    // A string tool_result content counts the same; an errored one does not.
    const asString: ApiMessageLike[] = [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'a', name: PR_TOOL, input }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'a', content: body }] },
    ]
    expect(isRepeat(asString, PR_TOOL, 'playbook_read', key, body)).toBe(true)
    const errored: ApiMessageLike[] = [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'a', name: PR_TOOL, input }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'a', content: body, is_error: true }] },
    ]
    expect(isRepeat(errored, PR_TOOL, 'playbook_read', key, body)).toBe(false)
  })

  test('manual and plugin compactions refresh the snapshot; auto is not assumed to', () => {
    expect(landingRefreshes('manual')).toBe(true)
    expect(landingRefreshes('plugin')).toBe(true)
    expect(landingRefreshes('auto')).toBe(false)
    expect(landingRefreshes('precompute')).toBe(false)
  })
})

describe('setup and compose', () => {
  test('a new session mints a key at start, stores it, and the first compose appends the keyed manual and sets the flag', async () => {
    const { h, m, edges, tools } = harness()
    m.start(edges)
    await tick()
    expect(tools()).toEqual(['ferrule', 'workflow_manual', 'playbook_read'])
    expect(h.calls[0]).toEqual({ server: SERVER, tool: 'ferrule', args: { root: '/repo/sub', format: 'json' } })
    expect(h.calls[1].args).toEqual({ session_key: 'minted-1' })
    expect(h.calls[2].args).toEqual({ name: 'lead-workflow-manual', session_key: 'minted-1' })
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(await m.compose([])).toBe(sectionText('minted-1', manualPart(response('minted-1'))!))
    expect(m.snapshot()).toBe(true)
    expect(h.timers.length).toBe(0)
  })

  test('a stored key re-logs in instead of minting; the flag stays clear though the section is appended', async () => {
    const { h, m, edges, tools } = harness({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW - 1000 } } })
    h.known.add('kept-key')
    m.start(edges)
    await tick()
    expect(tools()).toEqual(['ferrule', 'workflow_manual', 'playbook_read'])
    expect(h.calls[0].args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'kept-key' })
    expect(h.minted).toBe(0)
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'kept-key', at: NOW })
    expect(await m.compose([])).toContain('Your lead session key for this conversation is `kept-key`')
    expect(m.snapshot()).toBe(false)
  })

  test('a refused re-login mints a new key; the snapshot is still a replayed one', async () => {
    const { h, m, edges, tools } = harness({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'pruned-key', at: NOW } } })
    m.start(edges)
    await tick()
    expect(tools()).toEqual(['ferrule', 'ferrule', 'workflow_manual', 'playbook_read'])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(await m.compose([])).toContain('`minted-1`')
    expect(m.snapshot()).toBe(false)
  })

  test('outside a ws project nothing is minted, appended, or rewritten', async () => {
    const { h, m, edges } = harness({ wsProject: false })
    m.start(edges)
    await tick()
    expect(h.calls).toEqual([])
    expect(await m.compose([])).toBeUndefined()
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'k' }, response('k'))).toBeUndefined()
    expect(await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'x')).toBeUndefined()
    expect(await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'x')).toBeUndefined()
    expect(h.messageReads).toBe(0)
    // Told once: a later reset does not ask again.
    m.end('clear')
    expect(await m.compose([])).toBeUndefined()
    expect(h.gateChecks).toBe(1)
  })

  test('a gate that could not be told at start is asked again by the first compose', async () => {
    const { h, m, edges } = harness({ wsProject: 'rejects' })
    m.start(edges)
    await tick()
    expect(h.calls).toEqual([])
    h.wsProject = true
    const composed = m.compose([])
    await tick()
    expect(await composed).toContain('`minted-1`')
    expect(h.gateChecks).toBe(2)
    expect(m.snapshot()).toBe(true)
  })

  test('the first compose waits for the pending setup and appends once it lands within the bound', async () => {
    const { h, m, edges } = harness()
    let release!: () => void
    h.hold = new Promise<void>((resolve) => (release = resolve))
    m.start(edges)
    await tick()
    let done = false
    const composed = m.compose([]).then((text) => ((done = true), text))
    await tick()
    expect(done).toBe(false)
    expect(h.timers.map((t) => t.ms)).toEqual([COMPOSE_WAIT_MS])
    release()
    expect(await composed).toContain('`minted-1`')
    expect(h.timers[0].cancelled).toBe(true)
    expect(m.snapshot()).toBe(true)
  })

  test('on timeout the first compose appends nothing and clears the flag; later composes never wait', async () => {
    const { h, m, edges, fire } = harness()
    let release!: () => void
    h.hold = new Promise<void>((resolve) => (release = resolve))
    m.start(edges)
    await tick()
    const composed = m.compose([])
    await tick()
    fire(COMPOSE_WAIT_MS)
    expect(await composed).toBeUndefined()
    expect(m.snapshot()).toBe(false)
    // Still pending: a later compose answers at once, without a timer.
    expect(await m.compose([])).toBeUndefined()
    expect(h.timers.length).toBe(0)
    release()
    await tick()
    // Cached now: later composes append it (their answers are discarded), and the flag stays clear.
    expect(await m.compose([])).toContain('`minted-1`')
    expect(m.snapshot()).toBe(false)
  })

  test('a setup that failed at start is retried by the first compose', async () => {
    const { h, m, edges, tools } = harness()
    h.failing.add('ferrule')
    m.start(edges)
    await tick()
    expect(tools()).toEqual(['ferrule'])
    h.failing.clear()
    const composed = m.compose([])
    await tick()
    expect(await composed).toContain('`minted-1`')
    expect(tools()).toEqual(['ferrule', 'ferrule', 'workflow_manual', 'playbook_read'])
    expect(m.snapshot()).toBe(true)
  })

  test('an unhealthy ws-mcp costs one bounded wait per reset, not one per compose', async () => {
    const { h, m, edges, tools } = harness()
    h.failing.add('ferrule')
    m.start(edges)
    await tick()
    const composed = m.compose([])
    await tick()
    expect(await composed).toBeUndefined()
    expect(m.snapshot()).toBe(false)
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(h.timers.length).toBe(1)
    expect(await m.compose([])).toBeUndefined()
    expect(await m.compose([])).toBeUndefined()
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(h.timers.length).toBe(1)
    // The next reset's first compose tries once more.
    m.compacted('manual', undefined, { messages: [] })
    const again = m.compose([])
    await tick()
    expect(await again).toBeUndefined()
    expect(tools()).toEqual(['ferrule', 'ferrule', 'ferrule'])
  })

  test('after session.end the first compose sets up again: a new session id mints a new key; the old entry stays', async () => {
    const { h, m, edges } = harness()
    m.start(edges)
    await tick()
    await m.compose([])
    m.end('clear')
    expect(m.snapshot()).toBe(false)
    h.sessionId = 'sess-2'
    h.state = 'changed'
    const composed = m.compose([])
    await tick()
    expect(await composed).toContain('`minted-2`')
    expect(m.snapshot()).toBe(true)
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-2`)).toEqual({ sessionKey: 'minted-2', at: NOW })
  })

  test('a setup that ran across session.end fills no cache', async () => {
    const { h, m, edges } = harness()
    let release!: () => void
    h.hold = new Promise<void>((resolve) => (release = resolve))
    m.start(edges)
    await tick()
    m.end('clear')
    release()
    await tick()
    h.hold = undefined
    h.sessionId = 'sess-2'
    const composed = m.compose([])
    await tick()
    expect(await composed).toContain('`minted-2`')
  })

  test('a landed manual or plugin compaction makes the next compose decide the flag; auto, a subagent, precompute, and a veto do not', async () => {
    const { h, m, edges } = harness({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW } } })
    h.known.add('kept-key')
    m.start(edges)
    await tick()
    await m.compose([])
    expect(m.snapshot()).toBe(false)
    m.compacted('auto', undefined, { messages: [] })
    m.compacted('manual', 'sub-1', { messages: [] })
    m.compacted('precompute', undefined, { messages: [] })
    m.compacted('manual', undefined, { skip: 'blocked' })
    await m.compose([])
    expect(m.snapshot()).toBe(false)
    m.compacted('manual', undefined, { messages: [] })
    expect(await m.compose([])).toContain('`kept-key`')
    expect(m.snapshot()).toBe(true)
    // The lever's own landing arrives through landed().
    const lever = harness({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW } } })
    lever.h.known.add('kept-key')
    lever.m.start(lever.edges)
    await tick()
    await lever.m.compose([])
    lever.m.landed('plugin')
    await lever.m.compose([])
    expect(lever.m.snapshot()).toBe(true)
  })

  test('auto compaction leaves a set flag set', async () => {
    const { m, edges } = harness()
    m.start(edges)
    await tick()
    await m.compose([])
    m.compacted('auto', undefined, { messages: [] })
    await m.compose([])
    expect(m.snapshot()).toBe(true)
  })

  test('a measuring compose neither waits nor takes the first compose', async () => {
    const { h, m, edges } = harness()
    let release!: () => void
    h.hold = new Promise<void>((resolve) => (release = resolve))
    m.start(edges)
    await tick()
    expect(await m.compose(['lean', 'analysis', 'skills'])).toBeUndefined()
    expect(h.timers.length).toBe(0)
    release()
    await tick()
    expect(await m.compose(['analysis'])).toContain('`minted-1`')
    expect(m.snapshot()).toBe(false)
    await m.compose([])
    expect(m.snapshot()).toBe(true)
  })

  test('a compose before any start leaves the flag clear for that conversation only', async () => {
    const { h, m, edges } = harness()
    expect(await m.compose([])).toBeUndefined()
    m.start(edges)
    await tick()
    expect(await m.compose([])).toContain('`minted-1`')
    expect(m.snapshot()).toBe(false)
    // The next conversation's first compose decides the flag again.
    m.end('clear')
    h.sessionId = 'sess-2'
    m.start(edges)
    await tick()
    expect(await m.compose([])).toContain('`minted-2`')
    expect(m.snapshot()).toBe(true)
  })

  test('start prunes key entries past retention, keeping the current and the fresh ones', async () => {
    const day = 24 * 60 * 60 * 1000
    const { h, m, edges } = harness({
      store: {
        [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW - KEY_RETENTION_MS - day },
        [`${KEY_STORE_PREFIX}old`]: { sessionKey: 'old-key', at: NOW - KEY_RETENTION_MS - day },
        [`${KEY_STORE_PREFIX}recent`]: { sessionKey: 'recent-key', at: NOW - day },
        [`${KEY_STORE_PREFIX}broken`]: 'not an entry',
        'mailbox-watch:sess-9': { sessionKey: 'other' },
      },
    })
    h.known.add('kept-key')
    m.start(edges)
    await tick()
    expect([...h.store.keys()].sort()).toEqual(['mailbox-watch:sess-9', `${KEY_STORE_PREFIX}recent`, `${KEY_STORE_PREFIX}sess-1`])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'kept-key', at: NOW })
  })
})

describe('workflow_manual rewrite', () => {
  async function ready(opts: Parameters<typeof harness>[0] = {}) {
    const x = harness(opts)
    x.m.start(x.edges)
    await tick()
    await x.m.compose([])
    return x
  }

  test('with the flag set, the minted key reads as the state behind the pinned line', async () => {
    const { h, m } = await ready()
    h.state = '- [ ] {a} do it'
    const out = await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, response('minted-1', { state: h.state }))
    expect(out).toBe(stateOnlyText('# Manuals\n- build.md — Build steps\n\n## Session Key\nminted-1\n\n## Session State\n\n### Todos\n- [ ] {a} do it\n\n# Notes\n- none\n'))
  })

  test('a static span that differs from the injected one passes through: another key, or changed config', async () => {
    const { h, m } = await ready()
    h.known.add('other-key')
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'other-key' }, response('other-key'))).toBeUndefined()
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, response('minted-1', { prefs: 'Be verbose.' }))).toBeUndefined()
  })

  test('missed anchors fall back to workflow_state with the same key; a body-less notice passes through', async () => {
    const { h, m, tools } = await ready()
    const drifted = '# Renamed Manual\n\n## Session Key\nminted-1\n\n## Session State\n'
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1', root: '/repo' }, drifted)).toBe(
      stateOnlyText('## Session State\n\n### Todos\n(no todos)\n'),
    )
    expect(h.calls.at(-1)).toEqual({ server: SERVER, tool: 'workflow_state', args: { session_key: 'minted-1' } })
    const before = tools().length
    const notice = '## Session State\n(no restorable state for session key "gone"; ...)\n'
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'gone' }, notice)).toBeUndefined()
    expect(tools().length).toBe(before)
  })

  test('a failed workflow_state fallback passes the result through', async () => {
    const { h, m } = await ready()
    h.failing.add('workflow_state')
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, '# Renamed\n\n## Session Key\nk\n')).toBeUndefined()
  })

  test('with the flag clear the full result passes through', async () => {
    const { h, m } = await ready({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW } } })
    expect(h.known.has('kept-key')).toBe(false)
    expect(m.snapshot()).toBe(false)
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, response('minted-1'))).toBeUndefined()
  })
})

describe('repeat-read dedupe', () => {
  async function ready() {
    const x = harness()
    x.m.start(x.edges)
    await tick()
    await x.m.compose([])
    return x
  }

  test('a repeat playbook_read collapses to the pointer only while the earlier result is in the conversation, then reads in full again', async () => {
    const { h, m } = await ready()
    const input = { name: 'lead-run', session_key: 'minted-1' }
    const body = '# Run\n## Steps\nwork'
    // First read: no hint, no conversation read.
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBeUndefined()
    expect(h.messageReads).toBe(0)
    // The same read again, the first result in the conversation: the pointer.
    h.messages = conversation(PR_TOOL, [{ input, text: body }])
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBe(pointerText('playbook_read', body))
    expect(h.messageReads).toBe(1)
    // After a pointer the next identical read returns the full text again.
    h.messages = conversation(PR_TOOL, [
      { input, text: body },
      { input, text: pointerText('playbook_read', body) },
    ])
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBeUndefined()
    // Compaction took the earlier result away: not a repeat.
    h.messages = []
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBeUndefined()
  })

  test('the conversation is read only when the fresh result matches the hint', async () => {
    const { h, m } = await ready()
    const input = { name: 'lead-run' }
    await m.read('playbook_read', PR_TOOL, input, 'v1')
    await m.read('playbook_read', PR_TOOL, input, 'v2')
    await m.read('playbook_read', PR_TOOL, { name: 'lead-ticket' }, 'v2')
    expect(h.messageReads).toBe(0)
    await m.read('playbook_read', PR_TOOL, input, 'v2')
    expect(h.messageReads).toBe(1)
  })

  test('a lost hint (hot reload) only costs one missed dedupe', async () => {
    const input = { name: 'lead-run' }
    const body = '# Run'
    const { h, m } = await ready()
    h.messages = conversation(PR_TOOL, [{ input, text: body }])
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBeUndefined()
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBe(pointerText('playbook_read', body))
  })

  test('workflow_manual dedupes on the state-only text, keyed by session_key', async () => {
    const { h, m } = await ready()
    const raw = response('minted-1')
    const seen = (await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, raw))!
    expect(seen.startsWith('Workflow manual is in your system prompt;')).toBe(true)
    h.messages = conversation(WM_TOOL, [{ input: { session_key: 'minted-1' }, text: seen }])
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, raw)).toBe(pointerText('workflow_manual', seen))
    h.state = 'changed'
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, response('minted-1', { state: 'changed' }))).toMatch(
      /^Workflow manual is in your system prompt;/,
    )
  })

  test('a failed conversation read keeps the full text', async () => {
    const { m, edges } = await ready()
    edges.messages = async () => {
      throw new Error('no transcript')
    }
    await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'body')
    expect(await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'body')).toBeUndefined()
  })
})

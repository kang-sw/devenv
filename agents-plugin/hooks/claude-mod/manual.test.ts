import { describe, expect, test } from 'claude-code/testing'

import {
  adoptionNotice,
  createManual,
  epochCopyVisible,
  hashText,
  headings,
  inlinedPointerText,
  KEY_RETENTION_MS,
  KEY_STORE_PREFIX,
  KEY_WAIT_MS,
  keyLine,
  MAILBOX_LINE,
  pointerText,
  preloadPointerLine,
  readKey,
  repeatOf,
  revivedKeyToAdopt,
  rowHeader,
  rowText,
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

const FAIL_LOUD = '## Session State\n(no restorable state for session key "gone")\n'

const tick = async (n = 40) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

const NOW = 1_800_000_000_000
const SERVER = 'plugin:ws:ws'
const WM_TOOL = 'mcp__plugin_ws_ws__workflow_manual'
const PR_TOOL = 'mcp__plugin_ws_ws__playbook_read'

type Call = { server: string; tool: string; args: Record<string, unknown> }

/** Fake edges over a fake ws-mcp: calls are recorded, timers fire when the test says. */
function harness(opts: { store?: Record<string, unknown>; wsProject?: boolean | 'rejects'; server?: string | undefined; plugin?: string } = {}) {
  const h = {
    calls: [] as Call[],
    store: new Map<string, unknown>(Object.entries(opts.store ?? {})),
    wsProject: opts.wsProject ?? true,
    gateChecks: 0,
    /** While set, the project check waits on it. */
    gateHold: undefined as Promise<void> | undefined,
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
    /** While set, the keyed workflow_manual answers this text instead of the rendered manual. */
    manualAnswer: undefined as string | undefined,
    /** Whether the mailbox push runs here; while `pushHold` is set, the answer waits on it. */
    push: false,
    pushHold: undefined as Promise<void> | undefined,
    /** While set, a ferrule call whose args it answers with a promise waits on that promise. */
    ferruleHold: undefined as ((args: Record<string, unknown>) => Promise<void> | undefined) | undefined,
    /** While set, every store write rejects. */
    storeFails: false,
    /** Every key listener call, as [current, previous]. */
    keys: [] as [string, string | undefined][],
    timers: [] as { ms: number; fn: () => void; cancelled: boolean }[],
    messages: [] as ApiMessageLike[],
    /** While set, the conversation read rejects. */
    messagesFail: false,
    messageReads: 0,
    /** Every row appended to the main conversation. */
    appended: [] as string[],
    /** While set, an append rejects. */
    appendFails: false,
    logs: [] as { text: string; to: string }[],
  }
  const m = createManual()
  const edges: ManualEdges = {
    isWsProject: async () => {
      h.gateChecks += 1
      if (h.gateHold) await h.gateHold
      if (h.wsProject === 'rejects') throw new Error('cannot tell')
      return h.wsProject
    },
    server: async () => ('server' in opts ? opts.server : SERVER),
    call: async (server, tool, args) => {
      h.calls.push({ server, tool, args })
      if (h.failing.has(tool)) throw new Error(`${tool} failed`)
      if (tool === 'ferrule') {
        const held = h.ferruleHold?.(args)
        if (held) await held
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
        return h.manualAnswer ?? response(args.session_key as string, { state: h.state, prefs: h.prefs })
      }
      throw new Error(`unexpected tool ${tool}`)
    },
    pushAvailable: async () => {
      if (h.pushHold) await h.pushHold
      return h.push
    },
    keyChanged: (current, previous) => void h.keys.push([current, previous]),
    root: async () => h.root,
    sessionId: async () => h.sessionId,
    store: {
      get: async (k) => h.store.get(k),
      set: async (k, v) => {
        if (h.storeFails) throw new Error('store write failed')
        h.store.set(k, v)
      },
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
      if (h.messagesFail) throw new Error('no transcript')
      return h.messages
    },
    append: async (text) => {
      if (h.appendFails) throw new Error('append refused')
      h.appended.push(text)
    },
    log: (text, to) => void h.logs.push({ text, to }),
    plugin: opts.plugin ?? 'ws',
  }
  /** Fire every live timer of `ms`. */
  const fire = (ms: number) => {
    for (const t of h.timers.splice(0)) if (!t.cancelled && t.ms === ms) t.fn()
  }
  const tools = () => h.calls.map((c) => c.tool)
  return { h, m, edges, fire, tools }
}

/** A harness whose start has run to completion. */
async function ready(opts: Parameters<typeof harness>[0] = {}) {
  const x = harness(opts)
  x.m.start(x.edges)
  await tick()
  return x
}

/** A conversation holding the given earlier results of calls to `tool`. */
function conversation(tool: string, results: { input: Record<string, unknown>; text: string }[]): ApiMessageLike[] {
  return results.flatMap((r, i) => [
    { role: 'assistant', content: [{ type: 'tool_use', id: `tu-${i}`, name: tool, input: r.input }] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: `tu-${i}`, content: [{ type: 'text', text: r.text }] }] },
  ])
}

const userText = (text: string): ApiMessageLike => ({ role: 'user', content: [{ type: 'text', text }] })

describe('pinned prose', () => {
  test('the key line, the mailbox line, the row header and the row read as pinned', () => {
    expect(keyLine('ws', 'k-1')).toBe(
      'Your lead session key for this conversation is `k-1`; pass it as `session_key`. Continuing another track (for example `ws:lead-revive` with a preserved key) is still allowed.',
    )
    expect(MAILBOX_LINE).toBe(
      'Mailbox push is bound to this key automatically: arriving mail starts your next turn even when you are idle, so launch no mailbox wait. If a revive switches your key, the push follows it.',
    )
    expect(rowHeader('ws')).toBe(
      '[ws] Workflow manual for this conversation, with your session state as of this message. ' +
        'It is delivered once until the next compaction or /clear; read your agenda, todos, and notes again on demand with workflow_state, agenda_list, todo_list, or note_query.',
    )
    expect(rowText('ws', 'k-1', 'RESPONSE\n\n')).toBe(
      `${rowHeader('ws')}\n\n` +
        'Your lead session key for this conversation is `k-1`; pass it as `session_key`. Continuing another track (for example `ws:lead-revive` with a preserved key) is still allowed.\n\n' +
        'RESPONSE',
    )
    expect(rowText('ws', 'k-1', 'RESPONSE', true)).toBe(
      `${rowHeader('ws')}\n\n` +
        'Your lead session key for this conversation is `k-1`; pass it as `session_key`. Continuing another track (for example `ws:lead-revive` with a preserved key) is still allowed.\n\n' +
        'Mailbox push is bound to this key automatically: arriving mail starts your next turn even when you are idle, so launch no mailbox wait. If a revive switches your key, the push follows it.\n\n' +
        'RESPONSE',
    )
  })

  test('the pointers read as pinned', () => {
    const text = '# One\nbody\n## Two\n### Three\n#### Four'
    expect(pointerText(text)).toBe(
      'The result is unchanged. Continue using the full result of your earlier `playbook_read` call in this conversation. Do not read it again (headings: # One; ## Two; ### Three).',
    )
    expect(inlinedPointerText(text)).toBe(
      'The result is unchanged. Its full text is already above in this conversation, inlined when a skill loaded it; continue using that copy. Do not read it again (headings: # One; ## Two; ### Three).',
    )
    expect(preloadPointerLine('lead-run')).toBe(
      'The full `lead-run` procedure is already above in this conversation. If you can see that copy, follow it and skip the read instructions below; they are only for a context that cannot see it.',
    )
    expect(headings('no headings here')).toBe('(no headings)')
  })

  test('the adoption notice reads as pinned', () => {
    expect(adoptionNotice('ws', 'k-1', 'k-2')).toBe(
      '[ws] Lead session key changed: k-1 -> k-2. Keep passing k-2 as session_key; this conversation resumes with it, after compaction too.',
    )
    expect(adoptionNotice('ws', undefined, 'k-2')).toBe(
      '[ws] Lead session key changed: (none) -> k-2. Keep passing k-2 as session_key; this conversation resumes with it, after compaction too.',
    )
    expect(adoptionNotice('ws', 'k-1', 'k-2', true)).toBe(
      '[ws] Lead session key changed: k-1 -> k-2. Keep passing k-2 as session_key; this conversation resumes with it, after compaction too. ' +
        'Mailbox push is bound to k-2: arriving mail starts your next turn even when you are idle, so launch no mailbox wait.',
    )
  })

  test('a plugin not named ws spells its own name in the row, the adoption notice and the log lines, and ws nowhere', async () => {
    const notWs = /(?<![A-Za-z])ws(?![A-Za-z])/
    const row = rowText('pkgx', 'k-1', 'RESPONSE', true)
    expect(row.startsWith('[pkgx] Workflow manual for this conversation')).toBe(true)
    expect(row).toContain('`pkgx:lead-revive`')
    expect(row).not.toMatch(notWs)
    const notice = adoptionNotice('pkgx', 'k-1', 'k-2', true)
    expect(notice.startsWith('[pkgx] Lead session key changed: k-1 -> k-2.')).toBe(true)
    expect(notice).not.toMatch(notWs)
    const { h, m } = await ready({ plugin: 'pkgx' })
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([rowText('pkgx', 'minted-1', response('minted-1'))])
    expect(h.logs.length).toBeGreaterThan(0)
    for (const l of h.logs) expect(l.text.startsWith('pkgx manual: '), l.text).toBe(true)
  })
})

describe('pure helpers', () => {
  test('a playbook_read key is the name alone (the playbook is the unit; the body tells contexts apart)', () => {
    const a = readKey({ name: 'lead-run', context: { b: '2', a: '1' }, session_key: 'k-1' })
    expect(readKey({ name: 'lead-run', context: { a: '1', b: '3' }, session_key: 'k-2' })).toBe(a)
    expect(readKey({ name: 'lead-run' })).toBe(a)
    expect(readKey({ name: 'lead-ticket' })).not.toBe(a)
  })

  test('the key to adopt is an explicit key, or the sentinel result\'s ## Session Key, other than the sentinel and the current key', () => {
    expect(revivedKeyToAdopt(' other ', 'mine')).toBe('other')
    expect(revivedKeyToAdopt('other', undefined)).toBe('other')
    expect(revivedKeyToAdopt('other', 'mine', response('ignored'))).toBe('other')
    expect(revivedKeyToAdopt('mine', 'mine')).toBeUndefined()
    expect(revivedKeyToAdopt('obsidian-latch', 'mine', response('fresh-1'))).toBe('fresh-1')
    expect(revivedKeyToAdopt(' obsidian-latch ', undefined, response('fresh-1'))).toBe('fresh-1')
    expect(revivedKeyToAdopt('obsidian-latch', 'fresh-1', response('fresh-1'))).toBeUndefined()
    expect(revivedKeyToAdopt('obsidian-latch', 'mine', response('obsidian-latch'))).toBeUndefined()
    expect(revivedKeyToAdopt('obsidian-latch', 'mine', '# Workflow Manual\nno key here\n')).toBeUndefined()
    expect(revivedKeyToAdopt('obsidian-latch', 'mine', '## Session Key\n\n## Session State\n')).toBeUndefined()
    expect(revivedKeyToAdopt('obsidian-latch', 'mine')).toBeUndefined()
    expect(revivedKeyToAdopt('obsidian-latch', undefined)).toBeUndefined()
    expect(revivedKeyToAdopt('  ', 'mine')).toBeUndefined()
    expect(revivedKeyToAdopt(undefined, 'mine')).toBeUndefined()
    expect(revivedKeyToAdopt(42, 'mine')).toBeUndefined()
  })

  test('the hint hash tells texts apart', () => {
    expect(hashText('abc')).toBe(hashText('abc'))
    expect(hashText('abc')).not.toBe(hashText('abd'))
    expect(hashText('')).not.toBe(hashText(' '))
  })

  test('a repeat needs exactly one identical earlier result and no pointer to it', () => {
    const input = { name: 'lead-run', session_key: 'k-1' }
    const body = '# Run\nsteps'
    const repeat = (results: { input: Record<string, unknown>; text: string }[], tool = PR_TOOL) =>
      repeatOf(conversation(tool, results), PR_TOOL, input, body)
    expect(repeat([])).toBeUndefined()
    expect(repeat([{ input, text: body }])).toBe('result')
    expect(repeat([{ input: { name: 'lead-run', session_key: 'k-2' }, text: body }])).toBe('result')
    expect(repeat([{ input, text: body }], 'mcp__other__playbook_read')).toBeUndefined()
    expect(repeat([{ input: { name: 'lead-ticket' }, text: body }])).toBeUndefined()
    expect(repeat([{ input, text: '# Run\nolder steps' }])).toBeUndefined()
    expect(repeat([{ input, text: body }, { input, text: pointerText(body) }])).toBeUndefined()
    expect(repeat([{ input, text: body }, { input, text: pointerText(body) }, { input, text: body }])).toBeUndefined()
    expect(repeat([{ input, text: body }, { input, text: inlinedPointerText(body) }])).toBeUndefined()
    // Another context of the same name collapses on an identical body (compared trimmed), never on a different one.
    expect(repeat([{ input: { name: 'lead-run', context: { mode: 'x' } }, text: `${body}\n` }])).toBe('result')
    expect(repeat([{ input: { name: 'lead-run', context: { mode: 'x' } }, text: '# Run\nsteps for x' }])).toBeUndefined()
    // A string tool_result content counts the same; an errored one does not.
    const asString: ApiMessageLike[] = [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'a', name: PR_TOOL, input }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'a', content: body }] },
    ]
    expect(repeatOf(asString, PR_TOOL, input, body)).toBe('result')
    const errored: ApiMessageLike[] = [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'a', name: PR_TOOL, input }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'a', content: body, is_error: true }] },
    ]
    expect(repeatOf(errored, PR_TOOL, input, body)).toBeUndefined()
  })

  test('a playbook body a skill expansion inlined is an earlier copy, and a preload pointer line counts as a pointer', () => {
    const input = { name: 'lead-run' }
    const body = '# Run\nsteps'
    // The two shapes a skill expansion arrives in: after the Skill tool's own result, and a typed command's text block.
    const invoked = (text: string): ApiMessageLike[] => [
      { role: 'assistant', content: [{ type: 'tool_use', id: 's', name: 'Skill', input: { skill: 'ws:lead-run' } }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 's', content: 'Launching skill: ws:lead-run' }, { type: 'text', text }] },
    ]
    const typed = (text: string): ApiMessageLike[] => [
      { role: 'user', content: [{ type: 'text', text: '<command-name>/ws:lead-run</command-name>' }, { type: 'text', text }] },
    ]
    const expansion = `Base directory for this skill: /p/skills/lead-run\n\n${body}\n\nARGUMENTS: x`
    const pointed = `Base directory for this skill: /p/skills/lead-run\n\n${preloadPointerLine('lead-run')}\n\n# Run\n\nCall the read.`
    expect(repeatOf(invoked(expansion), PR_TOOL, input, `${body}\n`)).toBe('inlined')
    expect(repeatOf(typed(expansion), PR_TOOL, input, body)).toBe('inlined')
    expect(repeatOf([{ role: 'user', content: expansion }], PR_TOOL, input, body)).toBe('inlined')
    // Two copies, or a copy then a pointer of either kind: the full text again.
    expect(repeatOf([...invoked(expansion), ...typed(expansion)], PR_TOOL, input, body)).toBeUndefined()
    expect(repeatOf([...invoked(expansion), ...invoked(pointed)], PR_TOOL, input, body)).toBeUndefined()
    expect(repeatOf([...conversation(PR_TOOL, [{ input, text: body }]), ...typed(pointed)], PR_TOOL, input, body)).toBeUndefined()
    expect(repeatOf([...typed(expansion), ...conversation(PR_TOOL, [{ input, text: inlinedPointerText(body) }])], PR_TOOL, input, body)).toBeUndefined()
    // Another playbook's pointer line, or a different body, is no copy of this one.
    expect(repeatOf([...typed(expansion), ...typed(preloadPointerLine('lead-ticket'))], PR_TOOL, input, body)).toBe('inlined')
    expect(repeatOf(typed(expansion), PR_TOOL, input, '# Run\nother steps')).toBeUndefined()
    expect(repeatOf(typed(expansion), PR_TOOL, input, '  \n')).toBeUndefined()
  })
})

describe('epochCopyVisible', () => {
  const header = rowHeader('ws')

  test('the row header as a whole line of a user text block or a plain string is a copy', () => {
    expect(epochCopyVisible([userText(`${header}\n\nbody`)], WM_TOOL, 'ws')).toBe(true)
    expect(epochCopyVisible([{ role: 'user', content: `${header}\n\nbody` }], WM_TOOL, 'ws')).toBe(true)
    expect(epochCopyVisible([userText('earlier'), userText(rowText('ws', 'k-1', response('k-1')))], WM_TOOL, 'ws')).toBe(true)
  })

  test('a non-error result of the tool holding a whole ## Session Key line is a copy, whatever its key', () => {
    expect(epochCopyVisible(conversation(WM_TOOL, [{ input: { session_key: 'any' }, text: response('any') }]), WM_TOOL, 'ws')).toBe(true)
    expect(epochCopyVisible(conversation(WM_TOOL, [{ input: { session_key: 'a' }, text: FAIL_LOUD }, { input: { session_key: 'b' }, text: response('b') }]), WM_TOOL, 'ws')).toBe(true)
  })

  test('no copy: a fail-loud result, an error result, another tool, another plugin\'s header, a header that is not a whole line, an assistant message, an empty conversation', () => {
    expect(epochCopyVisible(conversation(WM_TOOL, [{ input: { session_key: 'gone' }, text: FAIL_LOUD }]), WM_TOOL, 'ws')).toBe(false)
    const errored: ApiMessageLike[] = [
      { role: 'assistant', content: [{ type: 'tool_use', id: 'a', name: WM_TOOL, input: { session_key: 'k' } }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'a', content: response('k'), is_error: true }] },
    ]
    expect(epochCopyVisible(errored, WM_TOOL, 'ws')).toBe(false)
    expect(epochCopyVisible(conversation('mcp__other__workflow_manual', [{ input: { session_key: 'k' }, text: response('k') }]), WM_TOOL, 'ws')).toBe(false)
    expect(epochCopyVisible([userText(rowHeader('pkgx'))], WM_TOOL, 'ws')).toBe(false)
    expect(epochCopyVisible([userText(`${header} and more on the same line`)], WM_TOOL, 'ws')).toBe(false)
    expect(epochCopyVisible([{ role: 'assistant', content: [{ type: 'text', text: header }] }], WM_TOOL, 'ws')).toBe(false)
    expect(epochCopyVisible([], WM_TOOL, 'ws')).toBe(false)
  })
})

describe('setup', () => {
  test('a new session mints a key at start and stores it, reading no manual', async () => {
    const { h, m, tools } = await ready()
    expect(tools()).toEqual(['ferrule'])
    expect(h.calls[0]).toEqual({ server: SERVER, tool: 'ferrule', args: { root: '/repo/sub', format: 'json' } })
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(m.key()).toBe('minted-1')
    expect(h.appended).toEqual([])
    expect(h.timers.length).toBe(0)
  })

  test('a stored key re-logs in instead of minting', async () => {
    const { h, m, edges, tools } = harness({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW - 1000 } } })
    h.known.add('kept-key')
    m.start(edges)
    await tick()
    expect(tools()).toEqual(['ferrule'])
    expect(h.calls[0].args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'kept-key' })
    expect(h.minted).toBe(0)
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'kept-key', at: NOW })
    expect(m.key()).toBe('kept-key')
  })

  test('a refused re-login mints a new key', async () => {
    const { h, m, tools } = await ready({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'pruned-key', at: NOW } } })
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(m.key()).toBe('minted-1')
  })

  test('outside a ws project nothing is minted, appended, or rewritten', async () => {
    const { h, m } = await ready({ wsProject: false })
    expect(h.calls).toEqual([])
    expect(await m.ensureKey()).toBeUndefined()
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([])
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'k' }, response('k'))).toBeUndefined()
    expect(await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'x')).toBeUndefined()
    expect(await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'x')).toBeUndefined()
    expect(h.messageReads).toBe(0)
    // Told once: a later reset does not ask again.
    m.end('clear')
    m.turnStarted(undefined)
    await tick()
    expect(await m.ensureKey()).toBeUndefined()
    expect(h.gateChecks).toBe(1)
    expect(h.calls).toEqual([])
  })

  test('a gate that could not be told reads as closed for good: the mailbox feature reads the same answer', async () => {
    const { h, m } = await ready({ wsProject: 'rejects' })
    expect(h.calls).toEqual([])
    h.wsProject = true
    expect(await m.ensureKey()).toBeUndefined()
    m.end('clear')
    m.turnStarted(undefined)
    await tick()
    expect(await m.ensureKey()).toBeUndefined()
    expect(h.calls).toEqual([])
    expect(h.gateChecks).toBe(1)
    expect(h.keys).toEqual([])
  })

  test('after session.end the next main-thread turn sets up again: a new session id mints a new key; the old entry stays', async () => {
    const { h, m } = await ready()
    m.end('clear')
    expect(m.key()).toBeUndefined()
    h.sessionId = 'sess-2'
    m.turnStarted(undefined)
    await tick()
    expect(m.key()).toBe('minted-2')
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-2`)).toEqual({ sessionKey: 'minted-2', at: NOW })
  })

  test('a setup that ran across session.end writes nothing, and the next conversation sets up under its own session id', async () => {
    const { h, m, edges } = harness()
    let release!: () => void
    const held = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = () => held
    m.start(edges)
    await tick()
    m.end('clear')
    release()
    await tick()
    expect(h.keys).toEqual([])
    expect(h.store.has(`${KEY_STORE_PREFIX}sess-1`)).toBe(false)
    expect(m.key()).toBeUndefined()
    h.ferruleHold = undefined
    h.sessionId = 'sess-2'
    m.turnStarted(undefined)
    await tick()
    expect(m.key()).toBe('minted-2')
    expect(h.keys).toEqual([['minted-2', undefined]])
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

describe('turnStarted', () => {
  test('the first main-thread turn after start retries a setup that failed at start', async () => {
    const { h, m, edges, tools } = harness()
    h.failing.add('ferrule')
    m.start(edges)
    await tick()
    expect(tools()).toEqual(['ferrule'])
    h.failing.clear()
    m.turnStarted(undefined)
    await tick()
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(m.key()).toBe('minted-1')
  })

  test('later main turns, and a subagent turn, begin nothing', async () => {
    const { h, m, edges, tools } = harness()
    h.failing.add('ferrule')
    m.start(edges)
    await tick()
    // A subagent's turn is not the first main-thread turn, and does not use it up.
    m.turnStarted('sub-1')
    await tick()
    expect(tools()).toEqual(['ferrule'])
    m.turnStarted(undefined)
    await tick()
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    // The first main turn is used up: no further retry, though no key is held.
    m.turnStarted(undefined)
    m.turnStarted(undefined)
    await tick()
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(m.key()).toBeUndefined()
  })

  test('a held key or a pending setup is left alone', async () => {
    const held = await ready()
    held.m.turnStarted(undefined)
    await tick()
    expect(held.tools()).toEqual(['ferrule'])

    const { h, m, edges, tools } = harness()
    let release!: () => void
    const gate = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = () => gate
    m.start(edges)
    await tick()
    m.turnStarted(undefined)
    await tick()
    expect(tools()).toEqual(['ferrule'])
    release()
    await tick()
    expect(tools()).toEqual(['ferrule'])
    expect(m.key()).toBe('minted-1')
    expect(h.minted).toBe(1)
  })

  test('the first main turn after session.end begins a setup; a late subagent turn first does not', async () => {
    const { h, m, tools } = await ready()
    m.end('clear')
    h.sessionId = 'sess-2'
    m.turnStarted('sub-1')
    await tick()
    expect(tools()).toEqual(['ferrule'])
    m.turnStarted(undefined)
    await tick()
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(m.key()).toBe('minted-2')
  })
})

describe('ensureKey', () => {
  test('a held key is returned without a call or a timer', async () => {
    const { h, m, tools } = await ready()
    expect(await m.ensureKey()).toBe('minted-1')
    expect(tools()).toEqual(['ferrule'])
    expect(h.timers.length).toBe(0)
  })

  test('with none held and none pending it begins a setup and returns its key', async () => {
    const { h, m, edges, tools } = harness()
    h.failing.add('ferrule')
    m.start(edges)
    await tick()
    h.failing.clear()
    const key = m.ensureKey()
    await tick()
    expect(await key).toBe('minted-1')
    expect(tools()).toEqual(['ferrule', 'ferrule'])
    expect(h.timers.map((t) => [t.ms, t.cancelled])).toEqual([[KEY_WAIT_MS, true]])
  })

  test('it waits for a pending setup, at most KEY_WAIT_MS', async () => {
    const { h, m, edges, tools } = harness()
    let release!: () => void
    const held = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = () => held
    m.start(edges)
    await tick()
    let done = false
    const waited = m.ensureKey().then((k) => ((done = true), k))
    await tick()
    expect(done).toBe(false)
    expect(h.timers.map((t) => t.ms)).toEqual([KEY_WAIT_MS])
    // A waiting call begins no second setup.
    expect(tools()).toEqual(['ferrule'])
    release()
    expect(await waited).toBe('minted-1')
    expect(h.timers[0].cancelled).toBe(true)

    // On timeout it answers undefined; the setup lands later and the key is held after that.
    const slow = harness()
    let release2!: () => void
    const held2 = new Promise<void>((resolve) => (release2 = resolve))
    slow.h.ferruleHold = () => held2
    slow.m.start(slow.edges)
    await tick()
    const timedOut = slow.m.ensureKey()
    await tick()
    slow.fire(KEY_WAIT_MS)
    expect(await timedOut).toBeUndefined()
    release2()
    await tick()
    expect(await slow.m.ensureKey()).toBe('minted-1')
  })

  test('it answers undefined on a failed setup and outside a ws project', async () => {
    const failed = harness()
    failed.h.failing.add('ferrule')
    failed.m.start(failed.edges)
    await tick()
    const key = failed.m.ensureKey()
    await tick()
    expect(await key).toBeUndefined()

    const outside = await ready({ wsProject: false })
    expect(await outside.m.ensureKey()).toBeUndefined()
    expect(outside.h.calls).toEqual([])
    expect(outside.h.timers.length).toBe(0)

    // Before any start there is nothing to ask.
    expect(await createManual().ensureKey()).toBeUndefined()
  })

  test('after session.end it sets up again under the new session id', async () => {
    const { h, m } = await ready()
    m.end('clear')
    h.sessionId = 'sess-2'
    const key = m.ensureKey()
    await tick()
    expect(await key).toBe('minted-2')
  })
})

describe('deliver', () => {
  test('with no copy visible it appends the row once: the key line, then the whole keyed response', async () => {
    const { h, m, tools } = await ready()
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([rowText('ws', 'minted-1', response('minted-1'))])
    expect(h.calls.at(-1)).toEqual({ server: SERVER, tool: 'workflow_manual', args: { session_key: 'minted-1' } })
    expect(tools()).toEqual(['ferrule', 'workflow_manual'])
    expect(h.appended[0].startsWith(`${rowHeader('ws')}\n\n${keyLine('ws', 'minted-1')}\n\n# Manuals`)).toBe(true)
    expect(h.appended[0]).not.toContain('Mailbox push is bound')
    // The row now in the conversation is a copy: the next lead skill expansion appends nothing.
    h.messages = [userText(h.appended[0])]
    await m.deliver(WM_TOOL)
    expect(h.appended.length).toBe(1)
    expect(tools()).toEqual(['ferrule', 'workflow_manual'])
    // The conversation lost it (compaction, /clear, a rewind): the next one appends again, with the current state.
    h.messages = []
    h.state = 'changed'
    await m.deliver(WM_TOOL)
    expect(h.appended.length).toBe(2)
    expect(h.appended[1]).toBe(rowText('ws', 'minted-1', response('minted-1', { state: 'changed' })))
  })

  test('the mailbox line is in the row only when the push runs here, and the row waits for the probe', async () => {
    const off = await ready()
    await off.m.deliver(WM_TOOL)
    expect(off.h.appended[0]).not.toContain(MAILBOX_LINE)

    const { h, m } = await ready()
    h.push = true
    let release!: () => void
    h.pushHold = new Promise<void>((resolve) => (release = resolve))
    let done = false
    const delivered = m.deliver(WM_TOOL).then(() => (done = true))
    await tick()
    expect(done).toBe(false)
    expect(h.appended).toEqual([])
    release()
    await delivered
    expect(h.appended).toEqual([rowText('ws', 'minted-1', response('minted-1'), true)])
    expect(h.appended[0]).toContain(MAILBOX_LINE)
  })

  test('nothing is appended while the row or a keyed workflow_manual result is visible, and no manual is read', async () => {
    const { h, m, tools } = await ready()
    h.messages = [userText(rowText('ws', 'minted-1', response('minted-1')))]
    await m.deliver(WM_TOOL)
    h.messages = conversation(WM_TOOL, [{ input: { session_key: 'other-key' }, text: response('other-key') }])
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([])
    expect(tools()).toEqual(['ferrule'])
    // A fail-loud result is no copy.
    h.messages = conversation(WM_TOOL, [{ input: { session_key: 'gone' }, text: FAIL_LOUD }])
    await m.deliver(WM_TOOL)
    expect(h.appended.length).toBe(1)
  })

  test('a rejecting conversation read appends the row', async () => {
    const { h, m } = await ready()
    h.messagesFail = true
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([rowText('ws', 'minted-1', response('minted-1'))])
  })

  test('nothing is appended, and nothing throws, when the manual read fails, its response lacks ## Session Key, or the append is refused', async () => {
    const { h, m } = await ready()
    h.failing.add('workflow_manual')
    await m.deliver(WM_TOOL)
    h.failing.clear()
    h.manualAnswer = FAIL_LOUD
    await m.deliver(WM_TOOL)
    h.manualAnswer = undefined
    h.appendFails = true
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([])
    h.appendFails = false
    await m.deliver(WM_TOOL)
    expect(h.appended.length).toBe(1)
  })

  test('nothing is appended outside a ws project, with no key, or before a start', async () => {
    const outside = await ready({ wsProject: false })
    await outside.m.deliver(WM_TOOL)
    expect(outside.h.appended).toEqual([])
    expect(outside.h.messageReads).toBe(0)

    const keyless = harness()
    keyless.h.failing.add('ferrule')
    keyless.m.start(keyless.edges)
    await tick()
    await keyless.m.deliver(WM_TOOL)
    expect(keyless.h.appended).toEqual([])
    expect(keyless.h.messageReads).toBe(0)
    expect(keyless.tools()).toEqual(['ferrule'])

    await createManual().deliver(WM_TOOL)
  })
})

describe('workflow_manual read', () => {
  test('a result that adopts nothing passes through, whatever the conversation holds', async () => {
    const { h, m, tools } = await ready()
    const before = tools().length
    h.messages = conversation(WM_TOOL, [{ input: { session_key: 'minted-1' }, text: response('minted-1') }])
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1' }, response('minted-1'))).toBeUndefined()
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'minted-1', root: '/repo' }, FAIL_LOUD)).toBeUndefined()
    expect(tools().length).toBe(before)
    expect(h.messageReads).toBe(0)
  })
})

describe('repeat-read dedupe', () => {
  test('a repeat playbook_read collapses to the pointer only while the earlier result is in the conversation, then reads in full again', async () => {
    const { h, m } = await ready()
    const input = { name: 'lead-run', session_key: 'minted-1' }
    const body = '# Run\n## Steps\nwork'
    // First read: no hint, no conversation read.
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBeUndefined()
    expect(h.messageReads).toBe(0)
    // The same read again, the first result in the conversation: the pointer.
    h.messages = conversation(PR_TOOL, [{ input, text: body }])
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBe(pointerText(body))
    expect(h.messageReads).toBe(1)
    // After a pointer the next identical read returns the full text again.
    h.messages = conversation(PR_TOOL, [
      { input, text: body },
      { input, text: pointerText(body) },
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
    expect(await m.read('playbook_read', PR_TOOL, input, body)).toBe(pointerText(body))
  })

  test('a preload and a read share one hint memory and one visibility check per playbook', async () => {
    const { h, m } = await ready()
    const body = '# Run\n## Steps\nwork'
    const expansion: ApiMessageLike = { role: 'user', content: [{ type: 'text', text: `Base directory for this skill: /p\n\n${body}\n` }] }
    // The first preload: no hint, no conversation read; it records the hint.
    expect(await m.preloaded(PR_TOOL, 'lead-run', body)).toBe(false)
    expect(h.messageReads).toBe(0)
    // A read after it, its expansion visible: the pointer naming the inlined copy.
    h.messages = [expansion]
    expect(await m.read('playbook_read', PR_TOOL, { name: 'lead-run', session_key: 'minted-1' }, `${body}\n`)).toBe(inlinedPointerText(`${body}\n`))
    expect(h.messageReads).toBe(1)
    // A preload after a read whose result is visible: a repeat.
    h.messages = conversation(PR_TOOL, [{ input: { name: 'lead-run' }, text: body }])
    expect(await m.preloaded(PR_TOOL, 'lead-run', body)).toBe(true)
    // Gone from the conversation: no repeat.
    h.messages = []
    expect(await m.preloaded(PR_TOOL, 'lead-run', body)).toBe(false)
    // A changed body moves the hint and reads nothing.
    const reads = h.messageReads
    h.messages = [expansion]
    expect(await m.preloaded(PR_TOOL, 'lead-run', '# Run\nchanged')).toBe(false)
    expect(h.messageReads).toBe(reads)
  })

  test('outside a ws project a preload is never a repeat and reads nothing', async () => {
    const { h, m } = await ready({ wsProject: false })
    h.messages = [{ role: 'user', content: 'body' }]
    expect(await m.preloaded(PR_TOOL, 'lead-run', 'body')).toBe(false)
    expect(await m.preloaded(PR_TOOL, 'lead-run', 'body')).toBe(false)
    expect(h.messageReads).toBe(0)
  })

  test('a failed conversation read keeps the full text', async () => {
    const { h, m } = await ready()
    h.messagesFail = true
    await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'body')
    expect(await m.read('playbook_read', PR_TOOL, { name: 'x' }, 'body')).toBeUndefined()
  })
})

describe('key listener', () => {
  test('told once per key change: at the mint, not at a turn, at the mint after session.end, and at a re-login after a new start', async () => {
    const { h, m, edges } = await ready()
    expect(h.keys).toEqual([['minted-1', undefined]])
    m.turnStarted(undefined)
    await m.ensureKey()
    await m.deliver(WM_TOOL)
    expect(h.keys).toEqual([['minted-1', undefined]])
    m.end('clear')
    h.sessionId = 'sess-2'
    m.turnStarted(undefined)
    await tick()
    expect(h.keys).toEqual([
      ['minted-1', undefined],
      ['minted-2', undefined],
    ])
    expect(m.key()).toBe('minted-2')
    // A hot reload or resume: a new start re-logs in and tells the same key again, since the push was stopped.
    m.start(edges)
    await tick()
    expect(h.keys.at(-1)).toEqual(['minted-2', undefined])
    expect(h.calls.filter((c) => c.tool === 'ferrule').at(-1)?.args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'minted-2' })
  })

  test('a setup that ran across session.end writes no key entry and tells the listener nothing', async () => {
    const { h, m, edges } = harness()
    let release!: () => void
    const held = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = (args) => (args.relogin_session_key === undefined ? held : undefined)
    m.start(edges)
    await tick()
    m.end('clear')
    release()
    await tick()
    expect(h.keys).toEqual([])
    expect(h.store.has(`${KEY_STORE_PREFIX}sess-1`)).toBe(false)
  })
})

describe('revive key adoption', () => {
  const revive = (m: ReturnType<typeof harness>['m'], key = 'revived') => m.read('workflow_manual', WM_TOOL, { session_key: key }, response(key))

  test('a revive with another key re-logs in, rewrites the entry, tells the listener, and appends the notice; nothing is rebuilt', async () => {
    const { h, m, tools } = await ready()
    h.known.add('revived')
    const before = tools().length
    expect(await revive(m)).toBe(`${response('revived')}\n\n${adoptionNotice('ws', 'minted-1', 'revived')}`)
    expect(h.calls[before]).toEqual({ server: SERVER, tool: 'ferrule', args: { root: '/repo/sub', format: 'json', relogin_session_key: 'revived' } })
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: NOW })
    expect(h.keys).toEqual([
      ['minted-1', undefined],
      ['revived', 'minted-1'],
    ])
    expect(m.key()).toBe('revived')
    await tick()
    expect(tools().slice(before)).toEqual(['ferrule'])
    expect(h.minted).toBe(1)
    // The adopted key again is the current one: nothing to adopt.
    expect(await revive(m)).toBeUndefined()
    expect(h.keys.length).toBe(2)
    // The next row carries the adopted key.
    await m.deliver(WM_TOOL)
    expect(h.appended).toEqual([rowText('ws', 'revived', response('revived'))])
  })

  test('the adopted key again while the adoption is in flight is not adopted twice', async () => {
    const { h, m, tools } = await ready()
    h.known.add('revived')
    await revive(m)
    await tick()
    const calls = tools().length
    expect(await revive(m)).toBeUndefined()
    expect(tools().length).toBe(calls)
    expect(h.keys.length).toBe(2)
  })

  test('a sentinel result carrying a minted key is adopted like a revive, and the lead reads it whole', async () => {
    const { h, m, tools } = await ready()
    h.known.add('fresh-1')
    const before = tools().length
    const minted = response('fresh-1')
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'obsidian-latch', root: '/repo' }, minted)).toBe(
      `${minted}\n\n${adoptionNotice('ws', 'minted-1', 'fresh-1')}`,
    )
    expect(h.calls[before]).toEqual({ server: SERVER, tool: 'ferrule', args: { root: '/repo/sub', format: 'json', relogin_session_key: 'fresh-1' } })
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'fresh-1', at: NOW })
    expect(h.keys).toEqual([
      ['minted-1', undefined],
      ['fresh-1', 'minted-1'],
    ])
    expect(m.key()).toBe('fresh-1')
    await tick()
    expect(tools().slice(before)).toEqual(['ferrule'])
    expect(h.minted).toBe(1)
  })

  test('a sentinel result for the current key, or without a ## Session Key line, reaches the lead whole and adopts nothing', async () => {
    const { h, m, tools } = await ready()
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'obsidian-latch' }, response('minted-1'))).toBeUndefined()
    const before = tools().length
    const bodyOnly = `${STATIC_BODY}\nCall ferrule to bootstrap.\n`
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'obsidian-latch' }, bodyOnly)).toBeUndefined()
    expect(tools().length).toBe(before)
    expect(h.keys).toEqual([['minted-1', undefined]])
  })

  test('every adoption notice names the push when it runs here, a revive\'s included', async () => {
    const { h, m } = await ready()
    h.push = true
    h.known.add('fresh-1')
    h.known.add('revived')
    const minted = response('fresh-1')
    expect(await m.read('workflow_manual', WM_TOOL, { session_key: 'obsidian-latch' }, minted)).toBe(`${minted}\n\n${adoptionNotice('ws', 'minted-1', 'fresh-1', true)}`)
    await tick()
    expect(await revive(m)).toBe(`${response('revived')}\n\n${adoptionNotice('ws', 'fresh-1', 'revived', true)}`)
  })

  test('no adoption for a key ws-mcp refuses', async () => {
    const { h, m, tools } = await ready()
    const before = tools().length
    expect(await revive(m, 'unknown-key')).toBeUndefined()
    expect(tools().slice(before)).toEqual(['ferrule'])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(h.keys).toEqual([['minted-1', undefined]])
    expect(m.key()).toBe('minted-1')
  })

  test('a conversation whose mint failed still adopts a revive, from none', async () => {
    const { h, m, edges } = harness()
    h.failing.add('ferrule')
    m.start(edges)
    await tick()
    h.failing.clear()
    h.known.add('revived')
    expect(await revive(m)).toBe(`${response('revived')}\n\n${adoptionNotice('ws', undefined, 'revived')}`)
    expect(h.keys).toEqual([['revived', undefined]])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: NOW })
    expect(await m.ensureKey()).toBe('revived')
  })

  test('an adoption during an in-flight mint keeps the adopted key in the entry and the listener', async () => {
    const { h, m, edges } = harness()
    h.known.add('revived')
    let release!: () => void
    const held = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = (args) => (args.relogin_session_key === undefined ? held : undefined)
    m.start(edges)
    await tick()
    expect(await revive(m)).toContain(adoptionNotice('ws', undefined, 'revived'))
    release()
    await tick()
    expect(h.minted).toBe(1)
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: NOW })
    expect(h.keys).toEqual([['revived', undefined]])
    expect(m.key()).toBe('revived')
  })

  test('an adoption during an in-flight re-login keeps the adopted key in the entry and the listener', async () => {
    const { h, m, edges } = harness({ store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'kept-key', at: NOW } } })
    h.known.add('kept-key')
    h.known.add('revived')
    let release!: () => void
    const held = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = (args) => (args.relogin_session_key === 'kept-key' ? held : undefined)
    m.start(edges)
    await tick()
    expect(await revive(m)).toContain(adoptionNotice('ws', undefined, 'revived'))
    release()
    await tick()
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: NOW })
    expect(h.keys).toEqual([['revived', undefined]])
  })

  test('a session end while the re-login runs adopts nothing', async () => {
    const { h, m } = await ready()
    h.known.add('revived')
    let release!: () => void
    const held = new Promise<void>((resolve) => (release = resolve))
    h.ferruleHold = (args) => (args.relogin_session_key === 'revived' ? held : undefined)
    const pending = revive(m)
    await tick()
    m.end('clear')
    release()
    expect(await pending).toBeUndefined()
    expect(h.keys).toEqual([['minted-1', undefined]])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
  })

  test('a failed key entry rewrite still adopts the key and gives the notice, leaving the stored entry as it was', async () => {
    const { h, m } = await ready()
    h.known.add('revived')
    h.storeFails = true
    expect(await revive(m)).toContain(adoptionNotice('ws', 'minted-1', 'revived'))
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(m.key()).toBe('revived')
    expect(h.keys.at(-1)).toEqual(['revived', 'minted-1'])
    expect(h.logs.some((l) => l.text.includes('key entry rewrite failed'))).toBe(true)
    // The conversation holds the adopted key from here on: no setup runs behind it.
    h.storeFails = false
    m.turnStarted(undefined)
    expect(await m.ensureKey()).toBe('revived')
    expect(h.minted).toBe(1)
  })
})

describe('late open', () => {
  test('an opened() while the project check is in flight keeps the gate open: the check reading closed does not shut it', async () => {
    const { h, m, edges } = harness({ wsProject: false })
    let release!: () => void
    h.gateHold = new Promise<void>((resolve) => (release = resolve))
    m.start(edges)
    await tick()
    m.opened()
    release()
    await tick()
    expect(h.minted).toBe(1)
    expect(m.key()).toBe('minted-1')
  })

  test('a late open with a held key re-logs in with it over the key entry, never minting, and the listener and entry follow it', async () => {
    const { h, m, edges, tools } = harness({ wsProject: false, store: { [`${KEY_STORE_PREFIX}sess-1`]: { sessionKey: 'stored-1', at: NOW } } })
    h.known.add('stored-1')
    h.known.add('held-1')
    m.start(edges)
    await tick()
    expect(tools()).toEqual([])
    m.opened('held-1')
    await tick()
    expect(h.minted).toBe(0)
    expect(h.calls[0].args).toEqual({ root: '/repo/sub', format: 'json', relogin_session_key: 'held-1' })
    expect(h.keys).toEqual([['held-1', undefined]])
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'held-1', at: NOW })
    expect(m.key()).toBe('held-1')
  })

  test('a session end drops the held key: the next conversation mints its own', async () => {
    const { h, m, edges } = harness({ wsProject: false })
    h.known.add('held-1')
    m.start(edges)
    await tick()
    m.opened('held-1')
    await tick()
    expect(m.key()).toBe('held-1')
    m.end('clear')
    h.sessionId = 'sess-2'
    const key = m.ensureKey()
    await tick()
    expect(await key).toBe('minted-1')
    expect(h.minted).toBe(1)
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-2`)).toEqual({ sessionKey: 'minted-1', at: NOW })
    expect(h.keys).toEqual([
      ['held-1', undefined],
      ['minted-1', undefined],
    ])
  })

  test('a late open whose held key is refused mints nothing, and neither does a later ensureKey or turn', async () => {
    const { h, m, edges } = harness({ wsProject: false })
    m.start(edges)
    await tick()
    m.opened('gone-1')
    await tick()
    expect(await m.ensureKey()).toBeUndefined()
    m.turnStarted(undefined)
    await tick()
    expect(h.minted).toBe(0)
    expect(h.keys).toEqual([])
    expect(m.key()).toBeUndefined()
  })

  test('an adoption after a late open moves the listener and the entry off the held key', async () => {
    const { h, m, edges } = harness({ wsProject: false })
    h.known.add('held-1')
    h.known.add('revived')
    m.start(edges)
    await tick()
    m.opened('held-1')
    await tick()
    await m.read('workflow_manual', WM_TOOL, { session_key: 'revived' }, response('revived'))
    await tick()
    expect(m.key()).toBe('revived')
    expect(h.store.get(`${KEY_STORE_PREFIX}sess-1`)).toEqual({ sessionKey: 'revived', at: NOW })
    expect(h.keys).toEqual([
      ['held-1', undefined],
      ['revived', 'held-1'],
    ])
    expect(h.minted).toBe(0)
  })
})

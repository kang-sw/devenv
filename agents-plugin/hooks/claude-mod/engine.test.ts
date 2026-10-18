import { expect, mock, test, type TestBody } from 'claude-code/testing'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests: the plugin under test runs inside the engine's own
// host, and the hooks below stand for the engine beneath it. The same file is
// mirrored into every package that ships the mod, so each test reads the
// package identity from the registration and asserts that package's mode.

type Recv = { result: unknown; text: string } | { deny: string } | { isError: true; result: unknown; text: string }

const empty = (): Recv => ({ result: undefined, text: 'null\n' })
const batch = (): Recv => ({
  result: undefined,
  text: '[{"reply_to":"id:peer","content":"hello from peer","sent_at":"t1"},{"from":"x@machine","content":"second","sent_at":"t2"}]',
})

function world(on: On, opts: { store?: Record<string, unknown>; processOk?: boolean; env?: Record<string, string> } = {}) {
  const clock = mock.clock(on)
  mock.env(on, opts.env ?? {})
  const w = {
    clock,
    store: new Map<string, unknown>(Object.entries(opts.store ?? {})),
    plugin: '',
    registered: [] as string[],
    spawns: [] as string[][],
    exits: [] as ((code: number) => void)[],
    killed: [] as number[],
    recvCalls: [] as { key: unknown; origin: string }[],
    recvAnswers: [] as Recv[],
    /** Delays (ms on the mock clock) for the next recv answers, in call order. */
    recvDelays: [] as number[],
    recvThrows: false,
    lookupCalls: [] as { key: unknown; origin: string }[],
    /** lookup_peers answers by session key; a key not listed has no named inbox. */
    lookupAnswers: {} as Record<string, Recv>,
    lookupDelays: [] as number[],
    connectDelays: [] as number[],
    dropSubmits: 0,
    prompts: [] as string[],
    logs: [] as { text: string; to: string }[],
  }
  on('store.get', ($, e) => ({ value: w.store.get(e.key) }))
  on('store.set', ($, e) => (w.store.set(e.key, e.value), { value: undefined }))
  on('store.delete', ($, e) => (w.store.delete(e.key), { value: undefined }))
  on('ui.log', ($, e) => (w.logs.push({ text: e.text, to: e.to }), { value: undefined }))
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('process.run', () =>
    opts.processOk === false
      ? { deny: 'no host commands on this surface' }
      : { value: { exitCode: 0, stdout: 'Python 3.12.0\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } },
  )
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    w.registered.push(e.name)
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  on('mcp.connect', async ($, e) => {
    const delay = w.connectDelays.shift()
    if (delay) await clock.sleep(delay)
    return { value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }
  })
  on('process.spawn', async function* ($, e, next) {
    const index = w.spawns.length
    w.spawns.push([...e.argv])
    next.signal.addEventListener('abort', () => w.killed.push(index), { once: true })
    const code = await new Promise<number>((resolve) => w.exits.push(resolve))
    yield* []
    return { value: { code, signal: null } }
  })
  on('tool.call', { tool: /__mailbox_recv$/ }, async ($, e, next) => {
    w.recvCalls.push({ key: (e as { session_key?: unknown }).session_key, origin: next.origin.plugin })
    if (w.recvThrows) throw new Error('recv transport failed')
    const answer = w.recvAnswers.shift() ?? empty()
    const delay = w.recvDelays.shift()
    if (delay) await clock.sleep(delay)
    return answer as never
  })
  on('tool.call', { tool: /__mailbox_lookup_peers$/ }, async ($, e, next) => {
    const key = (e as { session_key?: unknown }).session_key
    w.lookupCalls.push({ key, origin: next.origin.plugin })
    const delay = w.lookupDelays.shift()
    if (delay) await clock.sleep(delay)
    return (w.lookupAnswers[String(key)] ?? { result: undefined, text: '{"self":{"reply_id":"id:r"},"peers":[]}' }) as never
  })
  on('prompt.submit', ($, e) => {
    if (w.dropSubmits > 0) {
      w.dropSubmits -= 1
      return { drop: 'a Stop stands' }
    }
    w.prompts.push(e.text)
    return { text: e.text }
  })
  return w
}

const peers = (address: string): Recv => ({ result: undefined, text: JSON.stringify({ self: { address, auto: true }, peers: [] }) })
const waitTail = (key: string, slug?: string) => ['mailbox', 'wait', '--session-key', key, ...(slug ? ['--slug', slug] : []), '--timeout', '10m', '--format', 'json']
/** The transcript (user-visible) lines the mod logged. */
const transcript = (w: { logs: { text: string; to: string }[] }) => w.logs.filter((l) => l.to === 'transcript').map((l) => l.text)

/** The mailbox tools among every registration (the compaction feature registers its own in ws). */
const mailboxTools = (w: { registered: string[] }) => w.registered.filter((name) => name.startsWith('mailbox_'))

const recvName = (plugin: string) => `mcp__plugin_${plugin}_${plugin}__mailbox_recv`
const watchName = (plugin: string) => `mcp__${plugin}__mailbox_watch`

async function start($: Engine, w: ReturnType<typeof world>) {
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

async function bind($: Engine, w: ReturnType<typeof world>, key: string, agentId?: string) {
  const res = await $.tool.call({ tool: watchName(w.plugin), session_key: key, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

/** The model's own recv call (the test's $ raises it with the engine as origin). */
async function modelRecv($: Engine, w: ReturnType<typeof world>, key: string, answer: Recv) {
  w.recvAnswers.push(answer)
  const res = await $.tool.call({ tool: recvName(w.plugin), session_key: key } as never)
  await w.clock.settle()
  return res
}

test('bind spawns one finite wait from the plugin root; the same key is a no-op', async ($, on) => {
  const w = world(on)
  await start($, w)
  expect(mailboxTools(w)).toEqual(['mailbox_watch'])
  const bound = await bind($, w, 'k1')
  expect(String(bound.result)).toContain('bound')
  expect(w.spawns.length).toBe(1)
  const argv = w.spawns[0]!
  expect(argv[0]).toBe('python3')
  expect(argv[1]!.endsWith('/bin/ws-mcp-launcher.py')).toBe(true)
  expect(argv.slice(2)).toEqual(['mailbox', 'wait', '--session-key', 'k1', '--timeout', '10m', '--format', 'json'])

  const again = await bind($, w, 'k1')
  expect(String(again.result)).toContain('unchanged')
  expect(w.spawns.length).toBe(1)
})

test('a different key rebinds and stops the previous wait child', async ($, on) => {
  const w = world(on)
  await start($, w)
  await bind($, w, 'k1')
  const rebound = await bind($, w, 'k2')
  expect(String(rebound.result)).toContain('rebound')
  expect(w.killed).toEqual([0])
  expect(w.spawns.length).toBe(2)
  expect(w.spawns[1]).toContain('k2')
  // The stopped k1 loop does not act on its late exit.
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.prompts.length).toBe(0)
  expect(w.recvCalls.length).toBe(0)
})

test('a subagent bind is refused and does not bind', async ($, on) => {
  const w = world(on)
  await start($, w)
  const fromSub = await bind($, w, 'k3', 'sub-1')
  expect(fromSub.deny).toBe('mailbox_watch binds the main session only; called from subagent sub-1.')
  expect(w.spawns.length).toBe(0)
})

test('session.end clear stops the loop and forgets the binding', async ($, on) => {
  const w = world(on)
  await start($, w)
  await bind($, w, 'k1')
  expect(w.store.get('mailbox-watch:sess-1')).toEqual({ sessionKey: 'k1' })
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  expect(w.killed).toEqual([0])
  expect(w.store.has('mailbox-watch:sess-1')).toBe(false)
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.spawns.length).toBe(1)
  expect(w.prompts.length).toBe(0)
})

test('a WS_MAILBOX address in the environment adds its named inbox to the wait', async ($, on) => {
  const w = world(on, { env: { WS_MAILBOX: 'lead@machine' } })
  await start($, w)
  await bind($, w, 'k1')
  expect(w.spawns[0]!.slice(2)).toEqual(['mailbox', 'wait', '--session-key', 'k1', '--slug', 'lead@machine', '--timeout', '10m', '--format', 'json'])
})

test('session.end resume also stops the loop, so old-key mail is not drained into the resumed conversation', async ($, on) => {
  const w = world(on)
  await start($, w)
  await bind($, w, 'k1')
  await $.session.end({ reason: 'resume', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  expect(w.killed).toEqual([0])
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.recvCalls.length).toBe(0)
  expect(w.prompts.length).toBe(0)
  expect(w.spawns.length).toBe(1)
})

test('a reload re-binds from the stored key without an agent call', async ($, on) => {
  const w = world(on, { store: { 'mailbox-watch:sess-1': { sessionKey: 'k9' } } })
  await start($, w)
  expect(w.spawns.length).toBe(1)
  expect(w.spawns[0]).toContain('k9')
})

test('no tool is registered where host commands are unavailable', async ($, on) => {
  const w = world(on, { processOk: false })
  await start($, w)
  expect(mailboxTools(w)).toEqual([])
})

test('an error exit backs off instead of respawning at once', async ($, on) => {
  const w = world(on)
  await start($, w)
  await bind($, w, 'k1')
  w.exits[0]!(1)
  await w.clock.settle()
  expect(w.spawns.length).toBe(1)
  await w.clock.advance(999)
  expect(w.spawns.length).toBe(1)
  await w.clock.advance(1)
  expect(w.spawns.length).toBe(2)
  w.exits[1]!(1)
  await w.clock.settle()
  await w.clock.advance(1_999)
  expect(w.spawns.length).toBe(2)
  await w.clock.advance(1)
  expect(w.spawns.length).toBe(3)
  // A timeout (exit 3) re-arms at once.
  w.exits[2]!(3)
  await w.clock.settle()
  expect(w.spawns.length).toBe(4)
})

test('ws drains once and submits the whole drain as one prompt, then re-arms', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  w.recvAnswers.push(batch())
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.recvCalls).toEqual([{ key: 'k1', origin: 'ws' }])
  expect(w.prompts.length).toBe(1)
  expect(w.prompts[0]).toContain('2 message(s) arrived for session_key k1')
  expect(w.prompts[0]).toContain('mail from id:peer (t1):\nhello from peer')
  expect(w.prompts[0]).toContain('mail from x@machine (t2):\nsecond')
  expect(w.spawns.length).toBe(2)
})

test('ws with a refused drain submits one notice and suspends until the model receives', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  w.recvAnswers.push({ deny: 'Claude requested permissions to use mailbox_recv' })
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.recvCalls.length).toBe(1)
  expect(w.prompts.length).toBe(1)
  expect(w.prompts[0]).toContain(`Call ${recvName('ws')} with session_key k1`)
  expect(w.spawns.length).toBe(1) // suspended: no re-arm over the queued mail
  await w.clock.advance(120_000)
  expect(w.spawns.length).toBe(1)
  expect(w.prompts.length).toBe(1)
  await modelRecv($, w, 'k1', batch())
  expect(w.spawns.length).toBe(2)
})

test('wsflow submits one notice per arrival and never calls recv itself', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin === 'ws') return
  await bind($, w, 'k1')
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.recvCalls.length).toBe(0)
  expect(w.lookupCalls.length).toBe(0)
  expect(w.prompts.length).toBe(1)
  expect(w.prompts[0]).toContain(`Call ${recvName(w.plugin)} with session_key k1`)
  expect(w.spawns.length).toBe(1)
  await modelRecv($, w, 'k1', batch())
  expect(w.spawns.length).toBe(2)
  w.exits[1]!(0)
  await w.clock.settle()
  expect(w.prompts.length).toBe(2)
  expect(w.recvCalls.every((c) => c.origin === 'engine')).toBe(true)
})

test('after a notice only a successful model recv for the bound key re-arms', async ($, on) => {
  const w = world(on)
  await start($, w)
  await bind($, w, 'k1')
  if (w.plugin === 'ws') w.recvAnswers.push({ deny: 'refused' })
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.prompts.length).toBe(1)
  expect(w.spawns.length).toBe(1)

  await modelRecv($, w, 'k1', { deny: 'the user declined' })
  expect(w.spawns.length).toBe(1)
  await modelRecv($, w, 'k1', { isError: true, result: undefined, text: 'recv failed' })
  expect(w.spawns.length).toBe(1)
  await modelRecv($, w, 'other-key', batch())
  expect(w.spawns.length).toBe(1)
  expect(w.prompts.length).toBe(1)

  await modelRecv($, w, 'k1', empty())
  expect(w.spawns.length).toBe(2)
})

test('the package identity comes from the engine, so each mirrored copy runs its own mode', async ($, on) => {
  const w = world(on)
  await start($, w)
  // Mode-specific tests above return early for the other package; this pins
  // that each package actually exercised one of them.
  expect(['ws', 'wsflow']).toContain(w.plugin)
})

test('state notices reach the user transcript, not only the debug log', async ($, on) => {
  const w = world(on)
  await start($, w)
  await bind($, w, 'k1')
  expect(transcript(w)).toEqual([`${w.plugin} mailbox: armed k1`])
  w.exits[0]!(1)
  await w.clock.settle()
  expect(transcript(w)).toEqual([`${w.plugin} mailbox: armed k1`, `${w.plugin} mailbox: wait error, retrying with backoff`])
})

test('ws wakes on the named inbox lookup_peers reports, re-resolved on every rebind', async ($, on) => {
  const w = world(on, { env: { WS_MAILBOX: 'env@machine' } })
  w.lookupAnswers = { k1: peers('lead-ab12@worktree'), k2: peers('other-cd34@worktree') }
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  await bind($, w, 'k2')
  expect(w.lookupCalls).toEqual([
    { key: 'k1', origin: 'ws' },
    { key: 'k2', origin: 'ws' },
  ])
  expect(w.spawns.map((a) => a.slice(2))).toEqual([waitTail('k1', 'lead-ab12@worktree'), waitTail('k2', 'other-cd34@worktree')])
})

test('ws falls back to WS_MAILBOX when lookup_peers is refused or names no inbox', async ($, on) => {
  const w = world(on, { env: { WS_MAILBOX: 'env@machine' } })
  w.lookupAnswers = { k1: { deny: 'the user declined' } }
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  await bind($, w, 'k2')
  expect(w.lookupCalls.map((c) => c.key)).toEqual(['k1', 'k2'])
  expect(w.spawns.map((a) => a.slice(2))).toEqual([waitTail('k1', 'env@machine'), waitTail('k2', 'env@machine')])
})

test('overlapping binds resolve latest-wins while a slug lookup is in flight', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  w.lookupDelays.push(100)
  const first = $.tool.call({ tool: watchName(w.plugin), session_key: 'k1' } as never)
  await w.clock.settle()
  await bind($, w, 'k2')
  await w.clock.advance(100)
  expect(String((await first).result)).toContain('not changed')
  expect(w.spawns.map((a) => a[5])).toEqual(['k2'])
  expect(w.store.get('mailbox-watch:sess-1')).toEqual({ sessionKey: 'k2' })
})

test('a session end cancels a bind whose slug lookup is still in flight', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  w.lookupDelays.push(100)
  const pending = $.tool.call({ tool: watchName(w.plugin), session_key: 'k1' } as never)
  await w.clock.settle()
  expect(w.lookupCalls.map((c) => c.key)).toEqual(['k1'])
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.advance(100)
  expect(String((await pending).result)).toContain('not changed')
  expect(w.spawns).toEqual([])
  expect(w.store.has('mailbox-watch:sess-1')).toBe(false)
})

test('a session end cancels a reload restore whose slug lookup is still in flight', async ($, on) => {
  const w = world(on, { store: { 'mailbox-watch:sess-1': { sessionKey: 'k9' } } })
  w.lookupDelays.push(100)
  await start($, w)
  if (w.plugin !== 'ws') return
  expect(w.lookupCalls.map((c) => c.key)).toEqual(['k9'])
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.advance(100)
  expect(w.spawns).toEqual([])
  expect(w.store.has('mailbox-watch:sess-1')).toBe(false)
})

test('a reload restore yields to a bind made while it connects', async ($, on) => {
  const w = world(on, { store: { 'mailbox-watch:sess-1': { sessionKey: 'k9' } } })
  w.connectDelays.push(100)
  await start($, w)
  expect(w.spawns.length).toBe(0)
  await bind($, w, 'k1')
  await w.clock.advance(100)
  expect(w.spawns.map((a) => a[5])).toEqual(['k1'])
})

test('a reload restore yields to a bind made while it resolves its slug', async ($, on) => {
  const w = world(on, { store: { 'mailbox-watch:sess-1': { sessionKey: 'k9' } } })
  w.lookupDelays.push(100)
  await start($, w)
  if (w.plugin !== 'ws') return
  expect(w.lookupCalls.map((c) => c.key)).toEqual(['k9'])
  await bind($, w, 'k1')
  await w.clock.advance(100)
  expect(w.spawns.map((a) => a[5])).toEqual(['k1'])
})

test('ws records a drained batch in the transcript when its prompt is dropped', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  w.dropSubmits = 1
  w.recvAnswers.push(batch())
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.prompts).toEqual([])
  const record = transcript(w).find((t) => t.includes('drained mail was not submitted'))
  expect(record).toContain('(prompt submit failed: Error: prompt dropped: a Stop stands)')
  expect(record).toContain('2 message(s) arrived for session_key k1')
  expect(record).toContain('mail from id:peer (t1):\nhello from peer')
  expect(w.logs.some((l) => l.to === 'debug' && l.text === record)).toBe(true)
  expect(w.spawns.length).toBe(2)
})

test('ws records, not submits, a drain in flight when session.end stops the loop', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  w.recvAnswers.push(batch())
  w.recvDelays.push(100)
  w.exits[0]!(0)
  await w.clock.settle()
  expect(w.recvCalls.length).toBe(1)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.advance(100)
  expect(w.prompts).toEqual([])
  const record = transcript(w).find((t) => t.includes('drained mail was not submitted'))
  expect(record).toContain('(the push loop stopped: session.end clear)')
  expect(record).toContain('mail from x@machine (t2):\nsecond')
  expect(w.spawns.length).toBe(1)
})

test('ws still submits a drain in flight across a rebind', async ($, on) => {
  const w = world(on)
  await start($, w)
  if (w.plugin !== 'ws') return
  await bind($, w, 'k1')
  w.recvAnswers.push(batch())
  w.recvDelays.push(100)
  w.exits[0]!(0)
  await w.clock.settle()
  await bind($, w, 'k2')
  await w.clock.advance(100)
  expect(w.prompts.length).toBe(1)
  expect(w.prompts[0]).toContain('2 message(s) arrived for session_key k1')
  expect(transcript(w).some((t) => t.includes('not submitted'))).toBe(false)
})

test('a failing recv beneath the observer is never run a second time', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.recvThrows = true
  const res = await $.tool.call({ tool: recvName(w.plugin), session_key: 'k1' } as never)
  await w.clock.settle()
  expect(w.recvCalls.length).toBe(1)
  expect(res).toEqual({ isError: true, result: undefined, text: 'mailbox recv failed' })
})

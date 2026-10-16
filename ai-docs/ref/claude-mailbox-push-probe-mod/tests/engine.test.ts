import { expect, mock, test } from 'claude-code/testing'

const WATCH = 'mcp__ws-mailbox-push-probe__mailbox_watch'
const RECV = 'mcp__plugin_ws_ws__mailbox_recv'

// The test runtime has a real timer; the hooks lib (es2023, no DOM) does not declare it.
declare const setTimeout: (fn: () => void, ms: number) => unknown
const pause = () => new Promise<void>((r) => setTimeout(r, 30))

test('through the engine: bind -> spawn wait -> exit 0 -> recv via $.tool.call -> one prompt.submit -> re-arm', async ($, on) => {
  mock.clock(on)
  const spawns: string[][] = []
  const exits: ((code: number) => void)[] = []
  const recvCalls: Record<string, unknown>[] = []
  const prompts: { text: string; origin: unknown }[] = []
  const registered: string[] = []

  on('ui.status', () => ({ value: undefined }))
  on('ui.log', () => ({ value: undefined }))
  on('tool.register', ($, e) => {
    registered.push(e.name)
    return { value: { tool: `mcp__ws-mailbox-push-probe__${e.name}` } }
  })
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('process.spawn', async function* ($, e) {
    spawns.push([...e.argv])
    const code = await new Promise<number>((resolve) => exits.push(resolve))
    yield* []
    return { value: { code, signal: null } }
  })
  on('tool.call', { tool: RECV }, ($, e) => {
    recvCalls.push({ ...e })
    return { result: undefined as never, text: '[{"reply_to":"id:peer","content":"hello from peer","sent_at":"t1"},{"from":"x@machine","content":"second","sent_at":"t2"}]' }
  })
  on('prompt.submit', ($, e) => {
    prompts.push({ text: e.text, origin: e.origin })
    return { text: e.text }
  })

  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  expect(registered).toEqual(['mailbox_watch'])

  const bound = await $.tool.call({ tool: WATCH, session_key: 'k1' })
  expect(bound.deny).toBe(undefined)
  expect(String(bound.result)).toContain('bound')
  await pause()
  expect(spawns.length).toBe(1)
  expect(spawns[0]).toContain('--session-key')
  expect(spawns[0]).toContain('k1')

  const again = await $.tool.call({ tool: WATCH, session_key: 'k1' })
  expect(String(again.result)).toContain('unchanged')
  await pause()
  expect(spawns.length).toBe(1)

  exits[0]!(0)
  await pause()
  expect(recvCalls.length).toBe(1)
  expect(recvCalls[0]!.session_key).toBe('k1')
  expect(recvCalls[0]!.format).toBe('json')
  expect(prompts.length).toBe(1)
  expect(prompts[0]!.text).toContain('hello from peer')
  expect(prompts[0]!.text).toContain('second')
  expect(prompts[0]!.origin).toEqual({ kind: 'plugin', name: 'ws-mailbox-push-probe' })
  expect(spawns.length).toBe(2) // re-armed

  const rebound = await $.tool.call({ tool: WATCH, session_key: 'k2' })
  expect(String(rebound.result)).toContain('rebound')
  await pause()
  expect(spawns.length).toBe(3)
  expect(spawns[2]).toContain('k2')

  // A call carrying agentId (a subagent loop) is refused and does not rebind.
  const fromSub = await $.tool.call({ tool: WATCH, session_key: 'k3', agentId: 'sub-1' } as never)
  expect(fromSub.deny).toBe('mailbox_watch binds the main session only; called from subagent sub-1.')
  await pause()
  expect(spawns.length).toBe(3)

  await $.session.end({ reason: 'clear', sessionId: 's', resume: undefined as never })
  const before = spawns.length
  exits.forEach((x) => x(0))
  await pause()
  expect(spawns.length).toBe(before) // stopped: no re-arm after session.end clear
})

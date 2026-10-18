import { expect, mock, test, type TestBody } from 'claude-code/testing'

import {
  failureText,
  reasonCommandAnswered,
  REASON_NO_SERVER,
  REASON_TOOLS_ABSENT,
  RECONNECT_RESULT,
  RECONNECT_RESULT_NO_CONTINUATION,
  reconnectDescription,
  successText,
  TOOLS_WAIT_MS,
} from './reconnect'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the reconnect feature's wiring; its state machine is
// covered in reconnect.test.ts. The same file is mirrored into every package
// that ships the mod and the feature runs in each, so the tests read the
// package identity from the registrations and spell no package.

type CommandAnswer = { text?: string } | 'brings-back'

function world(on: On, opts: { processOk?: boolean; refuseRegister?: string; connect?: boolean } = {}) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    registered: [] as { name: string; description: string; isDeferred?: boolean; inputSchema?: unknown }[],
    /** Whether the plugin's MCP server is up: its tools are listed while it is. */
    serverUp: true,
    commands: [] as { command: string; args: string }[],
    /** Answers for the next reconnect commands; absent: the reconnect brings the server back. */
    commandAnswers: [] as CommandAnswer[],
    connects: 0,
    prompts: [] as string[],
    logs: [] as { text: string; to: string }[],
    /** Events in order, for the step's place relative to the turn end. */
    order: [] as string[],
    deleteDelays: [] as number[],
  }
  const server = () => `plugin:${w.plugin}:${w.plugin}`
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
  on('mcp.connect', ($, e) => {
    w.connects += 1
    return opts.connect === false || !w.serverUp
      ? { value: { isConnected: false, reason: 'failed', message: 'did not connect' } }
      : { value: { isConnected: true, server: `plugin:${e.server}:${e.server}` } }
  })
  on('tool.list', () => ({
    value: [
      { name: 'Read', description: 'Read a file.', mcp: false },
      { name: `mcp__${w.plugin}__request_mcp_reconnect`, description: 'mod tool', mcp: true },
      ...(w.serverUp ? [{ name: `mcp__${server().replace(/[^A-Za-z0-9_-]/g, '_')}__config_list`, description: 'x', mcp: true }] : []),
    ],
  }))
  on('command.run', ($, e) => {
    w.commands.push({ command: e.command, args: e.args })
    w.order.push('command')
    const answer = w.commandAnswers.shift() ?? 'brings-back'
    if (answer === 'brings-back') {
      w.serverUp = true
      return {}
    }
    return answer
  })
  on('prompt.submit', ($, e) => (w.prompts.push(e.text), { text: e.text }))
  return w
}

type World = ReturnType<typeof world>

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

async function turnEnd($: Engine, w: World, opts: { reason?: string; agentId?: string } = {}) {
  const reason = opts.reason ?? 'answer'
  await $.turn.complete({
    answer: '',
    durationMs: 1,
    isAborted: reason === 'aborted',
    turnId: 't',
    reason,
    ...(opts.agentId ? { agentId: opts.agentId } : {}),
  } as never)
  w.order.push('turn.complete returned')
  await w.clock.settle()
}

async function request($: Engine, w: World, continuation: unknown, agentId?: string) {
  const res = await $.tool.call({ tool: `mcp__${w.plugin}__request_mcp_reconnect`, continuation, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

const reconnectArgs = (w: World) => ({ command: 'mcp', args: `reconnect plugin:${w.plugin}:${w.plugin}` })

test('every package registers the reconnect tool with its pinned description', async ($, on) => {
  const w = world(on)
  await start($, w)
  expect(['ws', 'wsflow']).toContain(w.plugin)
  const tool = w.registered.find((r) => r.name === 'request_mcp_reconnect')
  expect(tool).toEqual({
    name: 'request_mcp_reconnect',
    description: reconnectDescription(w.plugin),
    isDeferred: false,
    inputSchema: tool?.inputSchema,
  })
  // The continuation is optional: the schema lists it, and does not require it.
  expect(tool?.inputSchema).toEqual({ type: 'object', properties: { continuation: { type: 'string' } } })
})

test('reconnect setup does not depend on the mailbox or compaction setups', async ($, on) => {
  const w = world(on, { processOk: false, refuseRegister: 'request_compaction' })
  await start($, w)
  expect(w.registered.map((r) => r.name)).toEqual(['request_mcp_reconnect'])
})

test('the mailbox and compaction setups do not depend on the reconnect setup', async ($, on) => {
  const w = world(on, { refuseRegister: 'request_mcp_reconnect' })
  await start($, w)
  const names = w.registered.map((r) => r.name)
  expect(names).toContain('mailbox_watch')
  if (w.plugin === 'ws') expect(names).toContain('request_compaction')
  expect(names).not.toContain('request_mcp_reconnect')
})

test('a subagent call is refused and a multi-line continuation is a tool error', async ($, on) => {
  const w = world(on)
  await start($, w)
  const fromSub = await request($, w, 'go on', 'sub-1')
  expect(fromSub.deny).toBe('request_mcp_reconnect serves the main session only; called from subagent sub-1.')
  expect((await request($, w, 'one\ntwo')).isError).toBe(true)
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
})

test('after the server died, the reconnect runs at the next main-thread turn end, after the hook returned, and one success continuation follows', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  const res = await request($, w, 'resume the review')
  expect(res.result).toBe(RECONNECT_RESULT)
  await turnEnd($, w, { agentId: 'sub-1' })
  await turnEnd($, w, { reason: 'aborted' })
  expect(w.commands).toEqual([])
  w.order.length = 0
  await turnEnd($, w)
  expect(w.order).toEqual(['turn.complete returned', 'command'])
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.prompts).toEqual([successText(w.plugin, 'resume the review')])
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(w.prompts.length).toBe(1)
})

test('an omitted or whitespace-only continuation is accepted: a success submits nothing, a failure still reports', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  expect((await request($, w, undefined)).result).toBe(RECONNECT_RESULT_NO_CONTINUATION)
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.prompts).toEqual([])
  const headless = "Reconnect, enable, and disable aren't available in this session."
  w.commandAnswers.push({ text: headless })
  expect((await request($, w, '   ')).result).toBe(RECONNECT_RESULT_NO_CONTINUATION)
  await turnEnd($, w)
  expect(w.prompts).toEqual([failureText(w.plugin, reasonCommandAnswered(headless), undefined)])
})

test('a repeat call replaces the continuation', async ($, on) => {
  const w = world(on)
  await start($, w)
  await request($, w, 'first')
  await request($, w, 'second')
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(w.prompts).toEqual([successText(w.plugin, 'second')])
})

test('success is judged from the tool list: tools still gone after the wait is a failure', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  w.commandAnswers.push({})
  await request($, w, 'go on')
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.prompts).toEqual([])
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_TOOLS_ABSENT, 'go on')])
})

test('a reconnect the session cannot run is a failure though the tools stay listed', async ($, on) => {
  const w = world(on)
  await start($, w)
  const headless = "Reconnect, enable, and disable aren't available in this session."
  w.commandAnswers.push({ text: headless })
  await request($, w, 'go on')
  await turnEnd($, w)
  expect(w.prompts).toEqual([failureText(w.plugin, reasonCommandAnswered(headless), 'go on')])
})

test('a request with no resolved server reports failure without reconnecting', async ($, on) => {
  const w = world(on, { connect: false })
  await start($, w)
  expect(w.connects).toBe(1)
  await request($, w, 'go on')
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_NO_SERVER, 'go on')])
})

test('the server name resolved at start serves a reconnect after the server died, and is resolved again after it', async ($, on) => {
  const w = world(on)
  await start($, w)
  expect(w.connects).toBe(1)
  w.serverUp = false
  await request($, w, 'go on')
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.connects).toBe(2)
})

test('session.end drops a pending request before awaiting the store delete', async ($, on) => {
  const w = world(on)
  await start($, w)
  await request($, w, 'go on')
  w.deleteDelays.push(100)
  const ended = $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
  await w.clock.advance(100)
  await ended
})

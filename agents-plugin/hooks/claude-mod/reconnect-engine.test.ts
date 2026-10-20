import { expect, mock, test, type TestBody } from 'claude-code/testing'

import {
  failureText,
  reasonCommandAnswered,
  REASON_NO_SERVER,
  REASON_TOOLS_ABSENT,
  RECONNECT_RESULT,
  RECONNECT_RESULT_NO_RESUME,
  reconnectDescription,
  RESUME_SENTENCE,
  successText,
  TOOLS_POLL_MS,
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
    /** How long (ms on the mock clock) the next reconnect commands take to answer, in call order. */
    commandDelays: [] as number[],
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
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
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
  on('command.run', async ($, e) => {
    w.commands.push({ command: e.command, args: e.args })
    w.order.push('command')
    const delay = w.commandDelays.shift()
    if (delay) await clock.sleep(delay)
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

/** A turn starting: the main thread's, or a subagent's with `agentId`. */
async function turnStart($: Engine, w: World, agentId?: string) {
  await $.turn.start({ text: '', turnId: 't', ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
}

/** A tool call with the given arguments (`{ continue: true }`, or none). */
async function request($: Engine, w: World, args: Record<string, unknown> = {}, agentId?: string) {
  const res = await $.tool.call({ tool: `mcp__${w.plugin}__request_mcp_reconnect`, ...args, ...(agentId ? { agentId } : {}) } as never)
  await w.clock.settle()
  return res
}

const reconnectArgs = (w: World) => ({ command: 'mcp', args: `reconnect plugin:${w.plugin}:${w.plugin}` })

test('every package registers the reconnect tool with its pinned description and boolean flag', async ($, on) => {
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
  // The flag is optional: the schema lists it, and does not require it.
  expect(tool?.inputSchema).toEqual({ type: 'object', properties: { continue: { type: 'boolean' } } })
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

test('a subagent call is refused and requests nothing', async ($, on) => {
  const w = world(on)
  await start($, w)
  const fromSub = await request($, w, { continue: true }, 'sub-1')
  expect(fromSub.deny).toBe('request_mcp_reconnect serves the main session only; called from subagent sub-1.')
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
})

test('after the server died, the reconnect runs at the next main-thread turn end, after the hook returned, and one resume prompt follows a success with the flag', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  const res = await request($, w, { continue: true })
  expect(res.result).toBe(RECONNECT_RESULT)
  await turnEnd($, w, { agentId: 'sub-1' })
  await turnEnd($, w, { reason: 'aborted' })
  expect(w.commands).toEqual([])
  w.order.length = 0
  await turnEnd($, w)
  expect(w.order).toEqual(['turn.complete returned', 'command'])
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.prompts).toEqual([successText(w.plugin)])
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(w.prompts.length).toBe(1)
})

test('an unset flag, or a leftover continuation argument, is accepted: a success submits nothing, a failure still reports without the resume sentence', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  expect((await request($, w)).result).toBe(RECONNECT_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.prompts).toEqual([])
  const headless = "Reconnect, enable, and disable aren't available in this session."
  w.commandAnswers.push({ text: headless })
  expect((await request($, w, { continuation: 'go on' })).result).toBe(RECONNECT_RESULT_NO_RESUME)
  await turnEnd($, w)
  expect(w.prompts).toEqual([failureText(w.plugin, reasonCommandAnswered(headless), false)])
  expect(w.prompts[0]).not.toContain(RESUME_SENTENCE)
})

test('a repeat call replaces the flag, latest wins', async ($, on) => {
  const w = world(on)
  await start($, w)
  await request($, w, { continue: true })
  await request($, w)
  await turnEnd($, w)
  expect(w.commands.length).toBe(1)
  expect(w.prompts).toEqual([])
  await request($, w)
  await request($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands.length).toBe(2)
  expect(w.prompts).toEqual([successText(w.plugin)])
})

test('a main-thread turn started while the reconnect command is pending: the success submits no resume prompt', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  w.commandDelays.push(100)
  await request($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  await turnStart($, w)
  await w.clock.advance(100)
  expect(w.serverUp).toBe(true)
  expect(w.prompts).toEqual([])
})

test('a main-thread turn started during the tool-list wait: the success submits no resume prompt', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  w.commandAnswers.push({})
  await request($, w, { continue: true })
  await turnEnd($, w)
  await turnStart($, w)
  w.serverUp = true
  await w.clock.advance(TOOLS_POLL_MS)
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.prompts).toEqual([])
})

test('a subagent turn started during the step does not suppress the resume prompt', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  w.commandDelays.push(100)
  await request($, w, { continue: true })
  await turnEnd($, w)
  await turnStart($, w, 'sub-1')
  await w.clock.advance(100)
  expect(w.prompts).toEqual([successText(w.plugin)])
})

test('a failure is reported though a main-thread turn started during the step; with the flag it ends with the resume sentence', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  w.commandAnswers.push({})
  await request($, w, { continue: true })
  await turnEnd($, w)
  await turnStart($, w)
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_TOOLS_ABSENT, true)])
  expect(w.prompts[0]!.endsWith(` ${RESUME_SENTENCE}`)).toBe(true)
  w.commandAnswers.push({})
  await request($, w)
  await turnEnd($, w)
  await turnStart($, w)
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_TOOLS_ABSENT, true), failureText(w.plugin, REASON_TOOLS_ABSENT, false)])
})

test('success is judged from the tool list: tools still gone after the wait is a failure', async ($, on) => {
  const w = world(on)
  await start($, w)
  w.serverUp = false
  w.commandAnswers.push({})
  await request($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.prompts).toEqual([])
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_TOOLS_ABSENT, true)])
})

test('a reconnect the session cannot run is a failure though the tools stay listed', async ($, on) => {
  const w = world(on)
  await start($, w)
  const headless = "Reconnect, enable, and disable aren't available in this session."
  w.commandAnswers.push({ text: headless })
  await request($, w, { continue: true })
  await turnEnd($, w)
  expect(w.prompts).toEqual([failureText(w.plugin, reasonCommandAnswered(headless), true)])
})

test('a request with no resolved server reports failure without reconnecting', async ($, on) => {
  const w = world(on, { connect: false })
  await start($, w)
  expect(w.connects).toBe(1)
  await request($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_NO_SERVER, true)])
})

test('the server name resolved at start serves a reconnect after the server died, and is resolved again after it', async ($, on) => {
  const w = world(on)
  await start($, w)
  expect(w.connects).toBe(1)
  w.serverUp = false
  await request($, w, { continue: true })
  await turnEnd($, w)
  expect(w.commands).toEqual([reconnectArgs(w)])
  expect(w.connects).toBe(2)
})

test('session.end drops a pending request before awaiting the store delete', async ($, on) => {
  const w = world(on)
  await start($, w)
  await request($, w, { continue: true })
  w.deleteDelays.push(100)
  const ended = $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: undefined as never })
  await w.clock.settle()
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([])
  await w.clock.advance(100)
  await ended
})

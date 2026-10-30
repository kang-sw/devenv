import { expect, mock, test, type TestBody } from 'claude-code/testing'

import {
  failureText,
  REASON_NO_SERVER,
  REASON_TOOLS_ABSENT,
  TOOLS_WAIT_MS,
  WATCHDOG_MAX_FAILURES,
  watchdogFailureText,
  watchdogSuccessText,
} from './reconnect'
import { KNOB } from './register'

type Engine = Parameters<TestBody>[0]
type On = Parameters<TestBody>[1]

// Engine-backed tests of the MCP watchdog's wiring: its knob reads, the
// package-file fallbacks for a knob that was not read and for a server that
// never resolved, and its timer. Its state machine is covered in
// reconnect.test.ts. The same file is mirrored into every package that ships
// the mod, so the tests read the package identity from the registrations and
// spell no package; the package's own files are the test's (its fs.read hook).

const MINUTE = 60_000

type Knob = unknown | Error

function world(
  on: On,
  opts: {
    /** Whether the server resolves at start. */
    connect?: boolean
    /** Keyless config_get answers by key; an Error answers an error result; absent: unset (null). */
    knobs?: Record<string, Knob>
    /** The package manifest's defaults for the watchdog knobs; null: the file cannot be read. */
    manifest?: { enabled?: unknown; interval?: unknown } | null
    /** The package plugin.json's mcpServers keys; null: the file cannot be read. */
    serverKeys?: string[] | null
  } = {},
) {
  const clock = mock.clock(on)
  mock.env(on, {})
  const w = {
    clock,
    plugin: '',
    /** The mcpServers key the host names the server by. */
    serverKey: '' as string,
    serverUp: true,
    connectOk: opts.connect !== false,
    commands: [] as string[],
    /** Answers for the next reconnect commands; absent: the reconnect brings the server back. */
    commandAnswers: [] as ({ text?: string } | 'stays-down')[],
    toolLists: 0,
    prompts: [] as string[],
    logs: [] as { text: string; to: string }[],
    reads: [] as string[],
  }
  const serverKeys = opts.serverKeys === undefined ? undefined : opts.serverKeys
  const server = () => `plugin:${w.plugin}:${w.serverKey || w.plugin}`
  const sanitized = (name: string) => name.replace(/[^A-Za-z0-9_-]/g, '_')
  on('store.get', () => ({ value: undefined }))
  on('store.set', () => ({ value: undefined }))
  on('store.delete', () => ({ value: undefined }))
  on('ui.log', ($, e) => (w.logs.push({ text: e.text, to: e.to }), { value: undefined }))
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('session.end', ($, e) => ({ sessionId: e.sessionId }))
  on('turn.start', ($, e) => ({ turnId: e.turnId }))
  on('turn.complete', ($, e) => ({ text: e.answer }))
  on('process.run', () => ({ deny: 'no host commands on this surface' }))
  on('tool.register', ($, e, next) => {
    w.plugin = next.origin.plugin
    return { value: { tool: `mcp__${w.plugin}__${e.name}` } }
  })
  on('fs.read', ($, e) => {
    w.reads.push(e.path)
    if (e.path.endsWith('/.claude-plugin/plugin.json')) {
      if (serverKeys === null) throw new Error('ENOENT')
      const keys = serverKeys ?? [w.plugin]
      return { value: JSON.stringify({ name: w.plugin, mcpServers: Object.fromEntries(keys.map((k) => [k, { command: 'x' }])) }) }
    }
    if (e.path.endsWith('/config-manifest.json')) {
      if (opts.manifest === null) throw new Error('ENOENT')
      const m = opts.manifest ?? { enabled: true, interval: 10 }
      return {
        value: JSON.stringify({
          schema_version: 1,
          namespace: 'claude.',
          keys: [
            { key: KNOB.watchdog, type: 'boolean', default: m.enabled },
            { key: KNOB.watchdogInterval, type: 'integer', default: m.interval },
          ],
        }),
      }
    }
    throw new Error(`unexpected read ${e.path}`)
  })
  // The connect names the plugin before any registration does.
  on('mcp.connect', ($, e) => {
    w.plugin = e.server
    return !w.connectOk || !w.serverUp
      ? { value: { isConnected: false, reason: 'failed', message: 'did not connect' } }
      : { value: { isConnected: true, server: server() } }
  })
  on('mcp.call', ($, e) => {
    const key = String(e.args.key)
    const knob = opts.knobs && key in opts.knobs ? opts.knobs[key] : null
    if (knob instanceof Error) return { value: { content: [{ type: 'text', text: knob.message }], isError: true } }
    return { value: { content: [{ type: 'text', text: JSON.stringify({ key, value: knob, scope: knob === null ? 'unset' : 'project' }) }], isError: false } }
  })
  on('tool.list', () => {
    w.toolLists += 1
    return {
      value: [
        { name: 'Read', description: 'Read a file.', mcp: false },
        { name: `mcp__${w.plugin}__request_mcp_reconnect`, description: 'mod tool', mcp: true },
        ...(w.serverUp ? [{ name: `mcp__${sanitized(server())}__config_list`, description: 'x', mcp: true }] : []),
      ],
    }
  })
  on('command.run', ($, e) => {
    w.commands.push(`${e.command} ${e.args}`)
    const answer = w.commandAnswers.shift()
    if (answer === 'stays-down') return {}
    if (answer) return answer
    w.serverUp = true
    w.connectOk = true
    return {}
  })
  on('prompt.submit', ($, e) => (w.prompts.push(e.text), { text: e.text }))
  return { w, server }
}

type World = ReturnType<typeof world>['w']

async function start($: Engine, w: World) {
  await $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
  await w.clock.settle()
}

async function turnStart($: Engine, w: World) {
  await $.turn.start({ text: '', turnId: 't' } as never)
  await w.clock.settle()
}

async function turnEnd($: Engine, w: World, reason = 'answer') {
  await $.turn.complete({ answer: '', durationMs: 1, isAborted: reason === 'aborted', turnId: 't', reason } as never)
  await w.clock.settle()
}

/** The time an attempt's tool-list wait takes at most, and a margin. */
const SETTLE_MS = TOOLS_WAIT_MS + 1_000

/** Move the clock `ms` on, then far enough for an attempt started on the way to finish its tool-list wait. */
async function advance(w: World, ms: number) {
  await w.clock.advance(ms)
  await w.clock.advance(SETTLE_MS)
}

const transcript = (w: World) => w.logs.filter((l) => l.to === 'transcript').map((l) => l.text)

test('a tick with the tools listed does nothing; one with them gone reconnects with one transcript line and no prompt', async ($, on) => {
  const { w, server } = world(on, { knobs: { [KNOB.watchdog]: true, [KNOB.watchdogInterval]: 10 } })
  await start($, w)
  await w.clock.advance(10 * MINUTE)
  expect(w.toolLists).toBe(1)
  expect(w.commands).toEqual([])
  w.serverUp = false
  await advance(w, 10 * MINUTE)
  expect(w.commands).toEqual([`mcp reconnect ${server()}`])
  expect(w.serverUp).toBe(true)
  expect(w.prompts).toEqual([])
  expect(transcript(w)).toEqual([watchdogSuccessText(w.plugin)])
})

test('the interval knob sets the period, and the first tick comes one interval after start', async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdog]: true, [KNOB.watchdogInterval]: 3 } })
  await start($, w)
  w.serverUp = false
  await w.clock.advance(3 * MINUTE - 1)
  expect(w.commands).toEqual([])
  await w.clock.advance(1)
  expect(w.commands.length).toBe(1)
})

test('the enabled knob off registers no timer', async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdog]: false, [KNOB.watchdogInterval]: 1 } })
  await start($, w)
  w.serverUp = false
  await w.clock.advance(120 * MINUTE)
  expect(w.toolLists).toBe(0)
  expect(w.commands).toEqual([])
})

test('a knob that was not read falls back to the package manifest: on runs the watchdog with its interval', async ($, on) => {
  const { w } = world(on, {
    knobs: { [KNOB.watchdog]: new Error('config_get failed'), [KNOB.watchdogInterval]: new Error('config_get failed') },
    manifest: { enabled: true, interval: 7 },
  })
  await start($, w)
  expect(w.reads.some((p) => p.endsWith('/config-manifest.json'))).toBe(true)
  w.serverUp = false
  await w.clock.advance(7 * MINUTE - 1)
  expect(w.commands).toEqual([])
  await w.clock.advance(1)
  expect(w.commands.length).toBe(1)
})

test('a knob that was not read falls back to the package manifest: off registers no timer', async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdog]: new Error('config_get failed') }, manifest: { enabled: false, interval: 1 } })
  await start($, w)
  w.serverUp = false
  await w.clock.advance(120 * MINUTE)
  expect(w.toolLists).toBe(0)
})

test('an invalid read takes the manifest default: an unset enabled value, an interval that is not a positive integer', async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdogInterval]: 0 }, manifest: { enabled: true, interval: 2 } })
  await start($, w)
  w.serverUp = false
  await w.clock.advance(2 * MINUTE)
  expect(w.commands.length).toBe(1)
})

test('with neither a read nor a package default the watchdog stays off', async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdog]: new Error('config_get failed') }, manifest: null })
  await start($, w)
  w.serverUp = false
  await w.clock.advance(120 * MINUTE)
  expect(w.toolLists).toBe(0)
})

test('a server unresolved at start gets the name derived from the plugin.json mcpServers key, for the watchdog and the lever', async ($, on) => {
  const { w } = world(on, { connect: false, serverKeys: ['srv'], manifest: { enabled: true, interval: 10 } })
  w.serverKey = 'srv'
  w.serverUp = false
  await start($, w)
  expect(w.reads.some((p) => p.endsWith('/.claude-plugin/plugin.json'))).toBe(true)
  await advance(w, 10 * MINUTE)
  expect(w.commands).toEqual([`mcp reconnect plugin:${w.plugin}:srv`])
  expect(transcript(w)).toEqual([watchdogSuccessText(w.plugin)])
})

test('the lever in a session whose server never resolved reconnects by the derived name', async ($, on) => {
  const { w } = world(on, { connect: false, serverKeys: ['srv'], knobs: { [KNOB.watchdog]: false } })
  w.serverKey = 'srv'
  w.serverUp = false
  await start($, w)
  await $.tool.call({ tool: `mcp__${w.plugin}__request_mcp_reconnect` } as never)
  await turnEnd($, w)
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.commands).toEqual([`mcp reconnect plugin:${w.plugin}:srv`])
  expect(w.prompts).toEqual([])
})

test('the lever with neither a resolved nor a derived name still reports no server', async ($, on) => {
  const { w } = world(on, { connect: false, serverKeys: null, knobs: { [KNOB.watchdog]: false } })
  w.serverUp = false
  await start($, w)
  await $.tool.call({ tool: `mcp__${w.plugin}__request_mcp_reconnect` } as never)
  await turnEnd($, w)
  expect(w.commands).toEqual([])
  expect(w.prompts).toEqual([failureText(w.plugin, REASON_NO_SERVER, false)])
})

test('a tick while a main-thread turn runs does nothing; the next idle tick reconnects', async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdog]: true, [KNOB.watchdogInterval]: 10 } })
  await start($, w)
  w.serverUp = false
  await turnStart($, w)
  await w.clock.advance(10 * MINUTE)
  expect(w.toolLists).toBe(0)
  await turnEnd($, w)
  await w.clock.advance(10 * MINUTE)
  expect(w.commands.length).toBe(1)
})

test(`${WATCHDOG_MAX_FAILURES} failed attempts stop the watchdog with one notice; a lever reconnect restarts it`, async ($, on) => {
  const { w } = world(on, { knobs: { [KNOB.watchdog]: true, [KNOB.watchdogInterval]: 10 } })
  await start($, w)
  w.serverUp = false
  w.commandAnswers.push(...Array.from({ length: 10 }, () => 'stays-down' as const))
  // Each tick then lands at the end of a step's first advance, its wait inside the second.
  await w.clock.advance(SETTLE_MS)
  for (let i = 0; i < WATCHDOG_MAX_FAILURES + 2; i++) await advance(w, 10 * MINUTE - SETTLE_MS)
  expect(w.commands.length).toBe(WATCHDOG_MAX_FAILURES)
  expect(transcript(w).filter((t) => t.includes('mcp watchdog'))).toEqual([
    watchdogFailureText(w.plugin, REASON_TOOLS_ABSENT, 10 * MINUTE),
  ])
  expect(w.prompts).toEqual([])
  // A lever reconnect that brings the server back resolves it again.
  w.commandAnswers.length = 0
  await $.tool.call({ tool: `mcp__${w.plugin}__request_mcp_reconnect` } as never)
  await turnEnd($, w)
  await w.clock.advance(TOOLS_WAIT_MS)
  expect(w.commands.length).toBe(WATCHDOG_MAX_FAILURES + 1)
  w.serverUp = false
  await advance(w, 10 * MINUTE)
  expect(w.commands.length).toBe(WATCHDOG_MAX_FAILURES + 2)
})

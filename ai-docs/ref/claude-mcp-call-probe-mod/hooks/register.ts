import type { Register } from 'claude-code'

// Probe for 261016-research-claude-mod-mailbox-active-push, open question:
// does $.mcp.call reach the ws server's mailbox_recv with no permission prompt
// under a prompting mode, and with which server/tool spelling.
// Inputs come from env-free constants so the probe stays self-contained;
// the runner script rewrites PROBE_KEY / LOG before each run.
const PROBE_KEY = 'baggy-disparity-external'
const LOG = '/tmp/mcp-call-probe/log.txt'
const RECV = 'mailbox_recv'
const SERVER_SPELLINGS = ['plugin:ws:ws', 'plugin_ws_ws', 'ws', 'plugin:wsflow:wsflow', 'plugin_wsflow_wsflow']
const CONTROL_TOOL = 'mcp__plugin_ws_ws__mailbox_recv'
// When true, a tool.check hook allows this plugin's own main-loop calls of the
// recv tool (next.origin.plugin is the caller) and defers everything else.
const GRANT_OWN_CALLS = false
const OWN_PLUGINS = ['mcp-call-probe', 'ws']

function show(v: unknown): string {
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

function errText(err: unknown): string {
  return err instanceof Error ? `${err.name}: ${err.message}` : show(err)
}

export const register: Register = (on) => {
  let done = false
  const lines: string[] = []

  on('tool.check', { tool: CONTROL_TOOL }, async ($, e, next) => {
    const origin = next.origin
    const own = OWN_PLUGINS.includes(origin.plugin) && e.agentId === undefined
    lines.push(`${new Date().toISOString()} tool.check hook: origin=${show(origin)} tool_use_id=${e.tool_use_id ?? '-'} agentId=${e.agentId ?? '-'} input=${show(e.input)} own=${own}`)
    if (GRANT_OWN_CALLS && own) {
      await $.fs.write(LOG, lines.join('\n') + '\n')
      return { decision: 'allow', reason: 'ws mod drains its own bound inbox' }
    }
    const beneath = await next(e)
    lines.push(`${new Date().toISOString()} tool.check hook: verdict beneath=${show(beneath)}`)
    await $.fs.write(LOG, lines.join('\n') + '\n')
    return beneath
  }).catch(() => ({ decision: 'deny' }))

  // Observer: does a tool.call hook see the model's own call of the recv tool?
  on('tool.call', { tool: CONTROL_TOOL }, async ($, e, next) => {
    lines.push(`${new Date().toISOString()} tool.call hook: origin=${show(next.origin)} agentId=${e.agentId ?? '-'} session_key=${show(e.session_key)} format=${show(e.format)} tool_use_id=${show((e as { tool_use_id?: string }).tool_use_id)}`)
    await $.fs.write(LOG, lines.join('\n') + '\n')
    const res = await next(e)
    lines.push(`${new Date().toISOString()} tool.call hook: result=${show(res).slice(0, 300)}`)
    await $.fs.write(LOG, lines.join('\n') + '\n')
    return res
  }).catch(($, e, next) => (next.called ? next(e) : next(e)))

  on('prompt.submit', async ($, e, next) => {
    if (done) return next(e)
    done = true
    const log = async (s: string) => {
      lines.push(`${new Date().toISOString()} ${s}`)
      await $.fs.write(LOG, lines.join('\n') + '\n')
    }
    try {
      await log(`version=${show(await $.session.version())}`)
    } catch (err) {
      await log(`version error ${errText(err)}`)
    }
    try {
      const tools = (await $.tool.list()).map((t) => t.name).filter((n) => n.includes('mailbox'))
      await log(`tool.list mailbox tools=${show(tools)}`)
    } catch (err) {
      await log(`tool.list error ${errText(err)}`)
    }
    // Design sequence: pre-check with $.tool.check, then drain with $.mcp.call.
    try {
      const pre = await $.tool.check({ tool: CONTROL_TOOL, input: { session_key: PROBE_KEY, format: 'json' } } as never)
      await log(`precheck tool.check(${CONTROL_TOOL}) -> ${show(pre)}`)
      const res = await $.mcp.call('plugin:ws:ws', RECV, { session_key: PROBE_KEY, format: 'json' })
      await log(`precheck-then mcp.call(plugin:ws:ws, ${RECV}) OK -> ${show(res)}`)
    } catch (err) {
      await log(`precheck sequence threw ${errText(err)}`)
    }
    const servers = [...SERVER_SPELLINGS]
    for (const server of ['ws', 'wsflow']) {
      try {
        const c = await $.mcp.connect(server)
        if (c.isConnected && !servers.includes(c.server)) servers.unshift(c.server)
        await log(`mcp.connect(${server}) -> ${show(c)}`)
      } catch (err) {
        await log(`mcp.connect(${server}) threw ${errText(err)}`)
      }
    }
    for (const server of servers) {
      try {
        const res = await $.mcp.call(server, RECV, { session_key: PROBE_KEY, format: 'json' })
        await log(`mcp.call(${server}, ${RECV}) OK -> ${show(res)}`)
      } catch (err) {
        await log(`mcp.call(${server}, ${RECV}) threw ${errText(err)}`)
      }
    }
    try {
      const chk = await $.tool.check({ tool: CONTROL_TOOL, input: { session_key: PROBE_KEY, format: 'json' } } as never)
      await log(`tool.check(${CONTROL_TOOL}) -> ${show(chk)}`)
    } catch (err) {
      await log(`tool.check(${CONTROL_TOOL}) threw ${errText(err)}`)
    }
    try {
      const res = await $.tool.call({ tool: CONTROL_TOOL, session_key: PROBE_KEY, format: 'json' } as never)
      await log(`tool.call(${CONTROL_TOOL}) -> ${show(res)}`)
    } catch (err) {
      await log(`tool.call(${CONTROL_TOOL}) threw ${errText(err)}`)
    }
    await log('done')
    return next(e)
  })
}

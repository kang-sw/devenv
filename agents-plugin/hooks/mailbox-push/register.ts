import type { HookStream, ProcessSpawnChunk, ProcessSpawnResult, Register, ToolCallResult } from 'claude-code'

import { createWatcher, mapWaitExit, parseRecvText, type DrainMode, type DrainResult, type Watcher, type WaitOutcome } from './watcher'

// Claude Code mod giving the cross-session mailbox an active push: the agent
// binds its session key through the mod-registered mailbox watch tool, the
// mod loops the one-shot `mailbox wait` as a peek-only signal, and arriving
// mail starts a turn through $.prompt.submit (see watcher.ts for the loop).
//
// One source serves every package that ships it (this directory is mirrored
// byte-identically), so package identity is read from the engine at runtime:
// $.plugin.name picks the drain mode, $.plugin.root locates the launcher, and
// $.mcp.connect(<plugin name>) resolves the MCP server. No package or server
// name is spelled here.

export const TOOL_NAME = 'mailbox_watch'
/** A finite window, as the pi adapter uses, so an orphaned wait cannot outlive its session. */
export const WAIT_TIMEOUT = '10m'
/**
 * $.store key prefix for the bound key, suffixed with the session id. $.store
 * rather than $.state: a $.state ref must spell its owning plugin as a literal,
 * which a source shared by several packages cannot. Entries are deleted at
 * session.end, so the binding lives as long as the session, like $.state.
 */
export const STORE_PREFIX = 'mailbox-watch:'

/** The package that drains through recv itself; every other package takes the notice path. */
export function drainModeFor(pluginName: string): DrainMode {
  return pluginName === 'ws' ? 'direct' : 'notice'
}

/** The model-facing name of an MCP server's tool, as the engine spells it. */
export function mcpToolName(server: string, tool: string): string {
  return `mcp__${server.replace(/[^A-Za-z0-9_-]/g, '_')}__${tool}`
}

export function launcherPath(pluginRoot: string): string {
  return `${pluginRoot.replace(/[\\/]+$/, '')}/bin/ws-mcp-launcher.py`
}

/**
 * The finite `mailbox wait` argv. Without `slug` it watches only the reply-id
 * queue; `slug` (the WS_MAILBOX address the session's MCP server claims from
 * the same environment) adds that named inbox. The wait itself owner-checks
 * the slug and falls back to reply-id-only when this key does not own it.
 */
export function waitArgv(launcher: string, key: string, slug?: string): string[] {
  const argv = ['python3', launcher, 'mailbox', 'wait', '--session-key', key]
  if (slug) argv.push('--slug', slug)
  return [...argv, '--timeout', WAIT_TIMEOUT, '--format', 'json']
}

/** Main-loop-only guard and key validation for the bind tool. */
export function checkWatchCall(agentId: string | undefined, sessionKey: unknown): { key: string } | { deny: string } {
  if (agentId !== undefined) {
    return { deny: `${TOOL_NAME} binds the main session only; called from subagent ${agentId}.` }
  }
  const key = typeof sessionKey === 'string' ? sessionKey.trim() : ''
  if (!key) return { deny: `${TOOL_NAME}: session_key (non-empty string) is required.` }
  return { key }
}

/** Classify a recv $.tool.call result: a deny is a refusal (notice path); an error result is a retryable failure. */
export function classifyRecv(res: ToolCallResult): DrainResult {
  if (res.deny !== undefined) return { kind: 'refused', reason: res.deny }
  if (res.isError) return { kind: 'failed', reason: res.text ?? 'recv reported an error' }
  if (typeof res.text === 'string') return { kind: 'mail', mail: parseRecvText(res.text) }
  const content = (res.result as { content?: { type: string; text?: string }[] } | undefined)?.content
  return { kind: 'mail', mail: parseRecvText(content?.find((b) => b.type === 'text')?.text) }
}

/**
 * Whether a recv tool.call is the model's own successful receive for the bound
 * key, the one completion a notice-suspended loop re-arms on. The mod's own
 * drain carries this plugin as origin; a dialog refusal may surface as either
 * a deny or an error result.
 */
export function isModelRecvCompletion(
  call: { tool: string; originPlugin: string; sessionKey: unknown },
  res: ToolCallResult,
  recvTool: string | undefined,
  boundKey: string | undefined,
): boolean {
  return (
    recvTool !== undefined &&
    call.tool === recvTool &&
    call.originPlugin === 'engine' &&
    boundKey !== undefined &&
    call.sessionKey === boundKey &&
    res.deny === undefined &&
    !res.isError
  )
}

function storedKey(value: unknown): string | undefined {
  const key = (value as { sessionKey?: unknown } | undefined)?.sessionKey
  return typeof key === 'string' && key.trim() ? key.trim() : undefined
}

/** Consume one spawned `mailbox wait` stream; abort ends it (returning the stream kills the child). */
async function consumeWait(
  stream: HookStream<ProcessSpawnChunk, ProcessSpawnResult>,
  signal: AbortSignal,
  debug: (text: string) => void,
): Promise<WaitOutcome> {
  let aborted = false
  const onAbort = () => {
    aborted = true
    void stream.return(undefined as never).catch(() => {})
  }
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    const consumed = (async () => {
      let stderr = ''
      for (;;) {
        const step = await stream.next()
        if (step.done) return { end: step.value, stderr }
        if (step.value.stream === 'stderr') stderr += step.value.text
      }
    })()
    const stopped = new Promise<'stopped'>((resolve) => {
      if (signal.aborted) resolve('stopped')
      signal.addEventListener('abort', () => resolve('stopped'), { once: true })
    })
    const raced = await Promise.race([consumed, stopped])
    if (raced === 'stopped') {
      consumed.catch(() => {})
      return 'stopped'
    }
    const outcome = mapWaitExit(raced.end?.code ?? null, aborted)
    if (outcome === 'error') {
      debug(`mailbox wait exited code=${raced.end?.code} signal=${raced.end?.signal} ${raced.stderr.trim().slice(0, 300)}`)
    }
    return outcome
  } finally {
    signal.removeEventListener('abort', onAbort)
  }
}

export const register: Register = (on) => {
  // Set at session.start; module variables, so a hot reload starts them over
  // and session.start restores the binding from $.store.
  let watcher: Watcher | undefined
  let toolName: string | undefined
  let recvTool: string | undefined

  on('session.start', async ($, e, next) => {
    watcher?.stop('session.start again')
    watcher = undefined
    const label = `${$.plugin.name} mailbox`
    // $.process is CLI only: without it there is no wait to run, so the tool
    // stays unregistered and the agent falls back to its shell wait.
    try {
      const probe = await $.process.run(['python3', '--version'], { timeoutMs: 10_000 })
      if (probe.exitCode !== 0) throw new Error(`python3 --version exited ${probe.exitCode}`)
    } catch (err) {
      $.ui.log(`${label}: push unavailable here (${String(err)}); ${TOOL_NAME} not registered`, { to: 'debug' })
      return next(e)
    }
    const launcher = launcherPath($.plugin.root)
    const slug = (await $.env.get('WS_MAILBOX'))?.trim() || undefined
    const created = createWatcher({
      mode: drainModeFor($.plugin.name),
      label,
      wait: async (key, signal) => {
        if (signal.aborted) return 'stopped'
        const stream = $.process.spawn({ argv: waitArgv(launcher, key, slug) })
        return consumeWait(stream, signal, (text) => $.ui.log(`${label}: ${text}`, { to: 'debug' }))
      },
      drain: async (key) => {
        if (!recvTool) return { kind: 'failed', reason: 'MCP server not resolved' }
        return classifyRecv(await $.tool.call({ tool: recvTool as `mcp__${string}__${string}`, session_key: key, format: 'json' }))
      },
      submit: async (text) => {
        await $.prompt.submit({ text })
      },
      recvTool: () => recvTool ?? 'mailbox recv',
      sleep: (ms, signal) =>
        new Promise<void>((resolve) => {
          if (signal.aborted) return resolve()
          const timer = $.clock.after(ms, () => resolve())
          signal.addEventListener('abort', () => (timer.cancel(), resolve()), { once: true })
        }),
      notice: (text) => $.ui.log(text),
      log: (text) => $.ui.log(text, { to: 'debug' }),
    })
    watcher = created
    const registered = await $.tool.register({
      name: TOOL_NAME,
      description:
        `Bind ${$.plugin.name} cross-session mailbox push to this session: pass your own session_key. ` +
        'While bound, arriving mail starts a new turn even when the session is idle, so no background mailbox wait is needed. ' +
        'The same key again is a no-op; a different key rebinds. Main session only.',
      inputSchema: {
        type: 'object',
        properties: { session_key: { type: 'string', description: 'Your own session key.' } },
        required: ['session_key'],
      },
      isDeferred: false,
    })
    toolName = registered.tool
    // Restore a binding a hot reload dropped, without delaying the first prompt.
    const restoreFrom = STORE_PREFIX + (await $.session.id())
    void (async () => {
      const key = storedKey(await $.store.get(restoreFrom))
      if (!key || watcher !== created) return
      const conn = await $.mcp.connect($.plugin.name)
      if (!conn.isConnected || watcher !== created) return
      recvTool = mcpToolName(conn.server, 'mailbox_recv')
      $.ui.log(`${label}: ${created.bind(key)} ${key} (restored)`, { to: 'debug' })
    })().catch((err) => $.ui.log(`${label}: restore failed: ${String(err)}`, { to: 'debug' }))
    return next(e)
  })

  on('tool.call', { tool: /__mailbox_watch$/ }, async ($, e, next) => {
    if (e.tool !== toolName) return next(e)
    const checked = checkWatchCall(e.agentId, (e as { session_key?: unknown }).session_key)
    if ('deny' in checked) return { deny: checked.deny }
    if (!watcher) return { deny: `${TOOL_NAME}: the session has not started yet.` }
    const conn = await $.mcp.connect($.plugin.name)
    if (!conn.isConnected) {
      return { deny: `${TOOL_NAME}: the ${$.plugin.name} MCP server is not connected (${conn.reason}); use the background mailbox wait instead.` }
    }
    recvTool = mcpToolName(conn.server, 'mailbox_recv')
    const outcome = watcher.bind(checked.key)
    await $.store.set(STORE_PREFIX + (await $.session.id()), { sessionKey: checked.key })
    return { result: `mailbox push ${outcome}: session_key=${checked.key}` }
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${TOOL_NAME}: the bind hook failed; see the debug log.` }))

  // Re-arm observer for the notice path: the model's own recv (origin engine)
  // for the bound key, completed without deny or error. The mod's own drain
  // passes here too, with this plugin as origin, and is ignored.
  on('tool.call', { tool: /__mailbox_recv$/ }, async ($, e, next) => {
    const res = await next(e)
    try {
      const boundKey = watcher?.current()
      const call = { tool: e.tool, originPlugin: next.origin.plugin, sessionKey: (e as { session_key?: unknown }).session_key }
      if (isModelRecvCompletion(call, res, recvTool, boundKey)) watcher?.recvCompleted(boundKey as string)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mailbox: recv observer failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  }).catch(($, e, next) =>
    // Never run recv a second time: a second call would drain (and drop) more mail.
    next.called ? { isError: true, result: undefined, text: 'mailbox recv failed' } : next(e),
  )

  // Every end reason stops the loop: after `clear` or `resume` the process
  // goes on under another session id, and a loop still bound to the old key
  // would drain its mail into an unrelated conversation. Compaction is not a
  // session end, so it keeps the binding.
  on('session.end', async ($, e, next) => {
    watcher?.stop(`session.end ${e.reason}`)
    try {
      await $.store.delete(STORE_PREFIX + e.sessionId)
    } catch {
      // A leftover entry only re-binds a reload of that ended session.
    }
    return next(e)
  })
}

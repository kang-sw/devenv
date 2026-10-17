import type { HookStream, ProcessSpawnChunk, ProcessSpawnResult, Register, ToolCallResult } from 'claude-code'

import { compactionEnabledFor, createCompaction } from './compaction'
import { LEVER_DESCRIPTION, TOOL_NAME as LEVER_TOOL } from './timing'
import {
  createWatcher,
  mapWaitExit,
  parseRecvText,
  type BindResult,
  type DrainMode,
  type DrainResult,
  type Watcher,
  type WaitOutcome,
} from './watcher'

// The Claude Code mod: the one hooks module a package's hooks.json names,
// carrying two features.
//
// - Mailbox push: the agent binds its session key through the mod-registered
//   mailbox watch tool, the mod loops the one-shot `mailbox wait` as a
//   peek-only signal, and arriving mail starts a turn through
//   $.prompt.submit (see watcher.ts for the loop).
// - Lead compaction timing: advisory and milestone wake turns before native
//   auto compaction, and the compaction request tool (see compaction.ts).
//
// Claude Code accepts one hooks module per plugin and one unmatched hook per
// event in it, so the lifecycle events (session.start, session.end) are
// registered once here and start each feature in isolation: neither setup
// awaits the other, and a throw in one is logged and leaves the other running.
//
// One source serves every package that ships it (this directory is mirrored
// byte-identically), so package identity is read from the engine at runtime:
// $.plugin.name picks the drain mode and gates the compaction feature,
// $.plugin.root locates the launcher, and $.mcp.connect(<plugin name>)
// resolves the MCP server. The one package name the mod spells is the one
// drainModeFor (here) and compactionEnabledFor (compaction.ts) compare
// against; no server name is spelled.

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
 * queue; `slug` (the key's own named-inbox address, see the bind's slug
 * resolution) adds that named inbox. The wait itself owner-checks the slug
 * and falls back to reply-id-only when this key does not own it.
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

/**
 * The caller's own named-inbox address from a `lookup_peers` `format: "json"`
 * result (`self.address`, present only when the key owns that inbox), or
 * undefined for a refusal, an error, or no address.
 */
export function selfAddress(res: ToolCallResult): string | undefined {
  if (res.deny !== undefined || res.isError) return undefined
  const text =
    typeof res.text === 'string'
      ? res.text
      : (res.result as { content?: { type: string; text?: string }[] } | undefined)?.content?.find((b) => b.type === 'text')?.text
  if (!text) return undefined
  try {
    const address = (JSON.parse(text) as { self?: { address?: unknown } } | null)?.self?.address
    return typeof address === 'string' && address.trim() ? address.trim() : undefined
  } catch {
    return undefined
  }
}

/**
 * The recv observer's work after the model's recv returned: re-arm a loop
 * suspended on that key. Never throws, so a fault here cannot fail the recv
 * whose mail the model already holds.
 */
export function observeRecv(
  watcher: Pick<Watcher, 'current' | 'recvCompleted'> | undefined,
  call: { tool: string; originPlugin: string; sessionKey: unknown },
  res: ToolCallResult,
  recvTool: string | undefined,
  log: (text: string) => void,
): void {
  try {
    const boundKey = watcher?.current()
    if (isModelRecvCompletion(call, res, recvTool, boundKey)) watcher?.recvCompleted(boundKey as string)
  } catch (err) {
    log(`recv observer failed: ${String(err)}`)
  }
}

function storedKey(value: unknown): string | undefined {
  const key = (value as { sessionKey?: unknown } | undefined)?.sessionKey
  return typeof key === 'string' && key.trim() ? key.trim() : undefined
}

/**
 * Consume one spawned `mailbox wait` stream; abort ends it (returning the
 * stream kills the child). Every abort listener it adds is removed when it
 * settles: the loop's signal lives as long as the binding.
 */
export async function consumeWait(
  stream: Pick<HookStream<ProcessSpawnChunk, ProcessSpawnResult>, 'next' | 'return'>,
  signal: AbortSignal,
  debug: (text: string) => void,
): Promise<WaitOutcome> {
  let aborted = false
  let resolveStopped!: (v: 'stopped') => void
  const stopped = new Promise<'stopped'>((resolve) => (resolveStopped = resolve))
  const onAbort = () => {
    aborted = true
    resolveStopped('stopped')
    void stream.return(undefined as never).catch(() => {})
  }
  if (signal.aborted) {
    void stream.return(undefined as never).catch(() => {})
    return 'stopped'
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

/** Resolve after `ms`, or early when `signal` aborts; its abort listener is removed either way. */
export function abortableSleep(
  after: (ms: number, fn: () => void) => { cancel: () => void },
  ms: number,
  signal: AbortSignal,
): Promise<void> {
  return new Promise<void>((resolve) => {
    if (signal.aborted) return resolve()
    let timer: { cancel: () => void } | undefined
    const onAbort = () => {
      timer?.cancel()
      resolve()
    }
    signal.addEventListener('abort', onAbort, { once: true })
    timer = after(ms, () => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    })
  })
}

export const register: Register = (on) => {
  // Set at session.start; module variables, so a hot reload starts them over
  // and session.start restores the binding from $.store.
  let watcher: Watcher | undefined
  let toolName: string | undefined
  let recvTool: string | undefined
  let lookupTool: string | undefined
  /** The key's own named-inbox slug for the wait; set per session.start, never throws. */
  let resolveSlug: (key: string) => Promise<string | undefined> = async () => undefined
  // Bumped by every bind attempt and every stop. The slug lookup is async, so
  // a bind applies only while it is still the newest: overlapping binds
  // resolve latest-wins, and a stop cancels a bind still resolving.
  let armSeq = 0

  const stopWatcher = (reason: string) => {
    armSeq += 1
    watcher?.stop(reason)
  }

  const useServer = (server: string) => {
    recvTool = mcpToolName(server, 'mailbox_recv')
    lookupTool = mcpToolName(server, 'mailbox_lookup_peers')
  }

  /** Resolve `key`'s slug, then bind it unless a newer bind or a stop came in meanwhile. */
  const arm = async (target: Watcher, key: string): Promise<BindResult | 'superseded'> => {
    const seq = ++armSeq
    const slug = await resolveSlug(key)
    if (seq !== armSeq || watcher !== target) return 'superseded'
    return target.bind(key, slug)
  }

  // Compaction feature state; its tool name is set at session.start, and only
  // under the package compactionEnabledFor admits.
  const compaction = createCompaction()
  let leverTool: string | undefined

  on('session.start', async ($, e, next) => {
    const label = `${$.plugin.name} mailbox`
    const mailboxSetup = (async () => {
      stopWatcher('session.start again')
      watcher = undefined
      // $.process is CLI only: without it there is no wait to run, so the tool
      // stays unregistered and the agent falls back to its shell wait.
      try {
        const probe = await $.process.run(['python3', '--version'], { timeoutMs: 10_000 })
        if (probe.exitCode !== 0) throw new Error(`python3 --version exited ${probe.exitCode}`)
      } catch (err) {
        $.ui.log(`${label}: push unavailable here (${String(err)}); ${TOOL_NAME} not registered`, { to: 'debug' })
        return
      }
      const launcher = launcherPath($.plugin.root)
      const mode = drainModeFor($.plugin.name)
      // WS_MAILBOX is the fallback slug: a re-derived auto-minted address would
      // not match the server's, so the direct mode asks the server for the
      // key's own address (lookup_peers self.address) through the same tool
      // path its drain uses. The notice mode makes no tool calls of its own.
      const envSlug = (await $.env.get('WS_MAILBOX'))?.trim() || undefined
      resolveSlug = async (key) => {
        if (mode !== 'direct' || !lookupTool) return envSlug
        try {
          const res = await $.tool.call({
            tool: lookupTool as `mcp__${string}__${string}`,
            session_key: key,
            scope: 'worktree',
            format: 'json',
          })
          return selfAddress(res) ?? envSlug
        } catch (err) {
          $.ui.log(`${label}: slug lookup failed: ${String(err)}`, { to: 'debug' })
          return envSlug
        }
      }
      const created = createWatcher({
        mode,
        label,
        wait: async (key, slug, signal) => {
          if (signal.aborted) return 'stopped'
          const stream = $.process.spawn({ argv: waitArgv(launcher, key, slug) })
          return consumeWait(stream, signal, (text) => $.ui.log(`${label}: ${text}`, { to: 'debug' }))
        },
        drain: async (key) => {
          if (!recvTool) return { kind: 'failed', reason: 'MCP server not resolved' }
          return classifyRecv(await $.tool.call({ tool: recvTool as `mcp__${string}__${string}`, session_key: key, format: 'json' }))
        },
        submit: async (text) => {
          const res = await $.prompt.submit({ text })
          if (res.drop !== undefined) throw new Error(`prompt dropped: ${res.drop}`)
        },
        recvTool: () => recvTool ?? 'mailbox recv',
        sleep: (ms, signal) => abortableSleep((d, fn) => $.clock.after(d, fn), ms, signal),
        notice: (text) => $.ui.log(text),
        record: (text) => $.ui.log(text),
        log: (text) => $.ui.log(text, { to: 'debug' }),
      })
      watcher = created
      // A bind from here on is newer than the restore below, which yields to it.
      const restoreSeq = armSeq
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
      const current = () => watcher === created && armSeq === restoreSeq
      void (async () => {
        const key = storedKey(await $.store.get(restoreFrom))
        if (!key || !current()) return
        const conn = await $.mcp.connect($.plugin.name)
        if (!conn.isConnected || !current()) return
        useServer(conn.server)
        $.ui.log(`${label}: ${await arm(created, key)} ${key} (restored)`, { to: 'debug' })
      })().catch((err) => $.ui.log(`${label}: restore failed: ${String(err)}`, { to: 'debug' }))
    })().catch((err) => $.ui.log(`${label}: setup failed: ${String(err)}`, { to: 'debug' }))

    const compactionSetup = (async () => {
      if (!compactionEnabledFor($.plugin.name)) return
      compaction.start({
        submit: (text) => $.prompt.submit({ text }),
        compact: () => $.session.compact(),
        after: (fn) => void $.clock.after(0, fn),
        log: (text, to) => $.ui.log(text, { to }),
      })
      const registered = await $.tool.register({
        name: LEVER_TOOL,
        description: LEVER_DESCRIPTION,
        inputSchema: {
          type: 'object',
          properties: { continuation: { type: 'string' } },
          required: ['continuation'],
        },
        isDeferred: false,
      })
      leverTool = registered.tool
    })().catch((err) => $.ui.log(`ws compaction: setup failed (${String(err)}); ${LEVER_TOOL} not registered`, { to: 'debug' }))

    // Both registrations are awaited, so each tool is listed by turn one.
    await Promise.all([mailboxSetup, compactionSetup])
    return next(e)
  })

  on('tool.call', { tool: /__mailbox_watch$/ }, async ($, e, next) => {
    if (e.tool !== toolName) return next(e)
    const checked = checkWatchCall(e.agentId, (e as { session_key?: unknown }).session_key)
    if ('deny' in checked) return { deny: checked.deny }
    const target = watcher
    if (!target) return { deny: `${TOOL_NAME}: the session has not started yet.` }
    const conn = await $.mcp.connect($.plugin.name)
    if (!conn.isConnected) {
      return { deny: `${TOOL_NAME}: the ${$.plugin.name} MCP server is not connected (${conn.reason}); use the background mailbox wait instead.` }
    }
    useServer(conn.server)
    const outcome = await arm(target, checked.key)
    if (outcome === 'superseded') {
      return { result: `mailbox push not changed: a newer bind or a session end superseded session_key=${checked.key}` }
    }
    await $.store.set(STORE_PREFIX + (await $.session.id()), { sessionKey: checked.key })
    return { result: `mailbox push ${outcome}: session_key=${checked.key}` }
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${TOOL_NAME}: the bind hook failed; see the debug log.` }))

  // Re-arm observer for the notice path: the model's own recv (origin engine)
  // for the bound key, completed without deny or error. The mod's own drain
  // passes here too, with this plugin as origin, and is ignored.
  on('tool.call', { tool: /__mailbox_recv$/ }, async ($, e, next) => {
    const res = await next(e)
    observeRecv(
      watcher,
      { tool: e.tool, originPlugin: next.origin.plugin, sessionKey: (e as { session_key?: unknown }).session_key },
      res,
      recvTool,
      (text) => $.ui.log(`${$.plugin.name} mailbox: ${text}`, { to: 'debug' }),
    )
    return res
  }).catch(($, e, next) =>
    // Never run recv a second time: a second call would drain (and drop) more mail.
    next.called ? { isError: true, result: undefined, text: 'mailbox recv failed' } : next(e),
  )

  on('tool.call', { tool: /__request_compaction$/ }, async ($, e, next) => {
    if (!compactionEnabledFor($.plugin.name) || e.tool !== leverTool) return next(e)
    return compaction.lever(e.agentId, (e as { continuation?: unknown }).continuation)
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${LEVER_TOOL}: the compaction request hook failed; see the debug log.` }))

  // Pushed after each main-thread turn (and on rate-limit moves); the plain
  // figures cost nothing, and the breakdown is read only when the feature's
  // cache is empty (session start or end, a landed compaction) or the window
  // changed.
  on('session.measure', async ($, e, next) => {
    if (!compactionEnabledFor($.plugin.name)) return next(e)
    const res = await next(e)
    try {
      await compaction.measure(e.context, () => $.session.usage({ breakdown: 'summary' }))
    } catch (err) {
      $.ui.log(`ws compaction: usage observation failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  })

  on('turn.complete', async ($, e, next) => {
    if (!compactionEnabledFor($.plugin.name)) return next(e)
    const res = await next(e)
    try {
      compaction.turnComplete(e.agentId, e.reason)
    } catch (err) {
      $.ui.log(`ws compaction: turn end handling failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  })

  // Observes every compaction without changing it; an observer never vetoes,
  // so a failure replays what settled beneath, or lets it run.
  on('session.compact', async ($, e, next) => {
    if (!compactionEnabledFor($.plugin.name)) return next(e)
    const res = await next(e)
    try {
      compaction.compacted(e.trigger, e.agentId, res)
    } catch (err) {
      $.ui.log(`ws compaction: compaction observation failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  }).catch(($, e, next) => next(e))

  // Each feature ends in isolation, so a throw in the compaction reset never
  // skips the mailbox stop and its $.store delete.
  on('session.end', async ($, e, next) => {
    // Mailbox: every end reason stops the loop: after `clear` or `resume` the
    // process goes on under another session id, and a loop still bound to the
    // old key would drain its mail into an unrelated conversation. Compaction
    // is not a session end, so it keeps the binding.
    try {
      stopWatcher(`session.end ${e.reason}`)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mailbox: stop failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      await $.store.delete(STORE_PREFIX + e.sessionId)
    } catch {
      // A leftover entry only re-binds a reload of that ended session.
    }
    // Compaction: every end reason starts the conversation's state over; the
    // next conversation baselines the latch from its first usage.
    try {
      if (compactionEnabledFor($.plugin.name)) compaction.end(e.reason)
    } catch (err) {
      $.ui.log(`ws compaction: reset failed: ${String(err)}`, { to: 'debug' })
    }
    return next(e)
  })
}

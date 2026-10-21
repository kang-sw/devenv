import type { HookStream, ProcessSpawnChunk, ProcessSpawnResult, Register, ToolCallResult } from 'claude-code'

import { compactionEnabledFor, createCompaction } from './compaction'
import { createManual, SECTION_ID, type ReadFamily } from './manual'
import { createReconnect, reconnectDescription, TOOL_NAME as RECONNECT_TOOL } from './reconnect'
import { ADVISORY_KEY, LEVER_DESCRIPTION, TOOL_NAME as LEVER_TOOL } from './timing'
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
// carrying four features.
//
// - Mailbox push: the mod loops the one-shot `mailbox wait` as a peek-only
//   signal for one bound session key, and arriving mail starts a turn through
//   $.prompt.submit, or, while a main-thread turn runs, joins it through
//   $.session.append at its next tool boundary (see watcher.ts for the loop).
//   Where the manual feature owns the conversation's key (ws, in a ws
//   project) the mod binds every key the conversation gets itself; elsewhere
//   the agent binds its key through the mod-registered mailbox watch tool.
// - Lead compaction timing: advisory and milestone wake turns before native
//   auto compaction, and the compaction request tool, whose opt-in resume
//   prompt yields to any turn already started after the compaction (see
//   compaction.ts).
// - MCP reconnect lever: a tool the model calls when the plugin's MCP server
//   has gone, and `/mcp reconnect` at the next main-thread turn end, with the
//   same opt-in resume prompt (see reconnect.ts).
// - Workflow manual injection (ws only, in a ws project): a lead session key
//   minted per conversation and the keyed workflow manual in the system
//   prompt, `workflow_manual` answered with the session state alone while
//   the prompt carries the manual, repeat ws reads collapsed to a pointer, and
//   a revive's key adopted as the conversation's (see manual.ts).
//
// Claude Code accepts one hooks module per plugin and one unmatched hook per
// event in it, so the lifecycle events (session.start, session.end,
// turn.start, turn.complete, session.compact) are registered once here and
// drive each feature in isolation: a throw in one is logged and leaves the
// others running. The one cross-feature wait is the shared ws-project gate: the
// mailbox setup awaits it to decide whether its bind tool registers, and the
// manual setup awaits the mailbox's host-command probe for its mailbox line.
//
// One source serves every package that ships it (this directory is mirrored
// byte-identically), so package identity is read from the engine at runtime:
// $.plugin.name picks the drain mode and gates the compaction and manual
// features,
// $.plugin.root locates the launcher, and $.mcp.connect(<plugin name>)
// resolves the MCP server, at session.start for every feature (a dead server
// cannot be resolved later). The one package name the mod spells is the one
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

/** A tool result's text as the model reads it: core's `text`, else its text blocks joined. */
export function resultText(res: ToolCallResult): string | undefined {
  if (typeof res.text === 'string') return res.text
  const r = res.result as unknown
  if (typeof r === 'string') return r
  const blocks = Array.isArray(r) ? r : (r as { content?: unknown } | undefined)?.content
  if (!Array.isArray(blocks)) return undefined
  const texts = blocks.flatMap((b) => (b?.type === 'text' && typeof b.text === 'string' ? [b.text as string] : []))
  return texts.length > 0 ? texts.join('\n') : undefined
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
  /**
   * The conversation's latest key from the manual feature, not yet bound:
   * recorded by the synchronous key listener and bound once a watcher
   * exists. Cleared at session.end, so a key never bound there stays unbound.
   */
  let autoKey: string | undefined
  /**
   * Whether a main-thread turn runs, from turn.start to turn.complete: mail
   * ready then joins that turn instead of starting one. A hot reload starts
   * it over as false, so mail during that turn is submitted as when idle.
   */
  let mainTurn = false
  /** Whether the latest session.start's mailbox probe passed; the manual's section says the push is bound only then. */
  let pushAvailable: Promise<boolean> = Promise.resolve(false)
  /**
   * Whether the session runs in a ws project under a package whose manual
   * feature runs: evaluated once per module instance and shared by the manual
   * feature (it mints keys only then) and the mailbox feature (its bind tool
   * registers only otherwise), so a session never holds both the lever and
   * the automatic bind. A failed check reads as closed for both.
   */
  let wsGate: Promise<boolean> | undefined

  const stopWatcher = (reason: string) => {
    armSeq += 1
    watcher?.stop(reason)
  }

  // The plugin's MCP server name, shared by every feature: resolved at
  // session.start, again by a bind and after a reconnect, and kept while the
  // server is down, when $.mcp.connect can no longer resolve it.
  let server: string | undefined
  /** The latest session.start's resolution, settled; a feature that needs the name at start awaits it. */
  let serverResolved: Promise<unknown> = Promise.resolve()
  const useServer = (resolved: string) => {
    server = resolved
    recvTool = mcpToolName(resolved, 'mailbox_recv')
    lookupTool = mcpToolName(resolved, 'mailbox_lookup_peers')
  }

  /** Resolve `key`'s slug, then bind it unless a newer bind or a stop came in meanwhile. */
  const arm = async (target: Watcher, key: string): Promise<BindResult | 'superseded'> => {
    const seq = ++armSeq
    const slug = await resolveSlug(key)
    if (seq !== armSeq || watcher !== target) return 'superseded'
    return target.bind(key, slug)
  }

  /** Bind the recorded key through `arm`, whose sequence makes the latest key win. */
  const bindAuto = (target: Watcher, log: (text: string) => void) => {
    const key = autoKey
    if (key === undefined) return
    autoKey = undefined
    void arm(target, key).then(
      (outcome) => log(`${outcome} ${key} (automatic)`),
      (err) => log(`automatic bind of ${key} failed: ${String(err)}`),
    )
  }

  // Compaction feature state; its tool name is set at session.start, and only
  // under the package compactionEnabledFor admits.
  const compaction = createCompaction()
  let leverTool: string | undefined

  // Reconnect feature state; its tool name is set at session.start.
  const reconnect = createReconnect()
  let reconnectTool: string | undefined

  // Manual feature state; started at session.start, and only under the
  // package compactionEnabledFor admits.
  const manual = createManual()

  on('session.start', async ($, e, next) => {
    const label = `${$.plugin.name} mailbox`
    const resolveServer = async () => {
      const conn = await $.mcp.connect($.plugin.name)
      if (conn.isConnected) useServer(conn.server)
      return conn
    }
    // Not awaited, so a slow server start never delays the first prompt.
    const resolved = resolveServer().catch((err) => {
      $.ui.log(`${$.plugin.name} mod: MCP server not resolved at start: ${String(err)}`, { to: 'debug' })
      return undefined
    })
    serverResolved = resolved
    const mailboxLog = (text: string) => $.ui.log(`${label}: ${text}`, { to: 'debug' })
    wsGate ??= compactionEnabledFor($.plugin.name)
      ? (async () => {
          const repoRoot = await $.session.repo().then(
            (repo) => repo?.root,
            () => undefined,
          )
          if (repoRoot) return $.fs.exists(`${repoRoot.replace(/[\\/]+$/, '')}/ai-docs/WORKFLOW.md`)
          return (await $.fs.ancestors({ names: ['ai-docs/WORKFLOW.md'] })).length > 0
        })().catch((err) => {
          $.ui.log(`${$.plugin.name} mod: ws project check failed (${String(err)}); reading as no ws project`, { to: 'debug' })
          return false
        })
      : Promise.resolve(false)
    const gate = wsGate
    let settlePush!: (available: boolean) => void
    pushAvailable = new Promise<boolean>((resolve) => (settlePush = resolve))
    const mailboxSetup = (async () => {
      stopWatcher('session.start again')
      watcher = undefined
      // $.process is CLI only: without it there is no wait to run, so the tool
      // stays unregistered and the agent falls back to its shell wait.
      try {
        const probe = await $.process.run(['python3', '--version'], { timeoutMs: 10_000 })
        if (probe.exitCode !== 0) throw new Error(`python3 --version exited ${probe.exitCode}`)
      } catch (err) {
        settlePush(false)
        $.ui.log(`${label}: push unavailable here (${String(err)}); ${TOOL_NAME} not registered`, { to: 'debug' })
        return
      }
      settlePush(true)
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
        turnRunning: () => mainTurn,
        append: async (text) => {
          const res = await $.session.append({ message: { type: 'user', content: [{ type: 'text', text }] } })
          if (res.deny !== undefined) throw new Error(`row refused: ${res.deny}`)
        },
        recvTool: () => recvTool ?? 'mailbox recv',
        sleep: (ms, signal) => abortableSleep((d, fn) => $.clock.after(d, fn), ms, signal),
        notice: (text) => $.ui.log(text),
        record: (text) => $.ui.log(text),
        log: (text) => $.ui.log(text, { to: 'debug' }),
      })
      watcher = created
      // A key the manual feature produced before the watcher existed.
      bindAuto(created, mailboxLog)
      // Where the manual feature owns the key, the mod binds it (bindAuto) and
      // the lever stays unregistered: no tool, no store entry, no restore (a
      // hot reload re-binds through the manual feature's re-login).
      if (await gate) {
        mailboxLog(`push binds the conversation's key automatically; ${TOOL_NAME} not registered`)
        return
      }
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
        const conn = await resolved
        if (!conn?.isConnected || !current()) return
        $.ui.log(`${label}: ${await arm(created, key)} ${key} (restored)`, { to: 'debug' })
      })().catch((err) => $.ui.log(`${label}: restore failed: ${String(err)}`, { to: 'debug' }))
    })().catch((err) => {
      settlePush(false)
      $.ui.log(`${label}: setup failed: ${String(err)}`, { to: 'debug' })
    })

    /** One ws MCP tool call's text; an error result rejects with its text. */
    const callText = async (name: string, tool: string, args: Record<string, unknown>) => {
      const res = await $.mcp.call(name, tool, args)
      const text = res.content.flatMap((b) => (b.type === 'text' && typeof b.text === 'string' ? [b.text] : [])).join('\n')
      if (res.isError) throw new Error(text || `${tool} reported an error`)
      return text
    }

    const compactionSetup = (async () => {
      if (!compactionEnabledFor($.plugin.name)) return
      compaction.start({
        submit: (text) => $.prompt.submit({ text }),
        compact: () => $.session.compact(),
        after: (fn) => void $.clock.after(0, fn),
        delay: (ms, fn) => void $.clock.after(ms, fn),
        log: (text, to) => $.ui.log(text, { to }),
        leverLanded: () => manual.landed('plugin'),
        // One config_get per session start, once the shared server resolution settles.
        readAdvisory: async () => {
          await serverResolved
          if (!server) throw new Error('MCP server not resolved')
          return (JSON.parse(await callText(server, 'config_get', { key: ADVISORY_KEY, format: 'json' })) as { value?: unknown }).value
        },
      })
      const registered = await $.tool.register({
        name: LEVER_TOOL,
        description: LEVER_DESCRIPTION,
        inputSchema: {
          type: 'object',
          properties: { continue: { type: 'boolean' } },
        },
        isDeferred: false,
      })
      leverTool = registered.tool
    })().catch((err) => $.ui.log(`ws compaction: setup failed (${String(err)}); ${LEVER_TOOL} not registered`, { to: 'debug' }))

    const reconnectSetup = (async () => {
      reconnect.start({
        plugin: $.plugin.name,
        server: () => server,
        reconnect: (name) => $.command.run({ command: 'mcp', args: `reconnect ${name}` }),
        toolsListed: async (name) => {
          const prefix = mcpToolName(name, '')
          return (await $.tool.list()).some((t) => t.name.startsWith(prefix))
        },
        resolve: async () => void (await resolveServer()),
        submit: (text) => $.prompt.submit({ text }),
        after: (fn) => void $.clock.after(0, fn),
        sleep: (ms) => new Promise<void>((resolve) => void $.clock.after(ms, resolve)),
        log: (text, to) => $.ui.log(text, { to }),
      })
      const registered = await $.tool.register({
        name: RECONNECT_TOOL,
        description: reconnectDescription($.plugin.name),
        inputSchema: {
          type: 'object',
          properties: { continue: { type: 'boolean' } },
        },
        isDeferred: false,
      })
      reconnectTool = registered.tool
    })().catch((err) =>
      $.ui.log(`${$.plugin.name} mcp reconnect: setup failed (${String(err)}); ${RECONNECT_TOOL} not registered`, { to: 'debug' }),
    )

    // Registers no tool: its setup runs on in the background, and the first
    // compose waits for it.
    try {
      if (compactionEnabledFor($.plugin.name)) {
        manual.start({
          isWsProject: () => gate,
          pushAvailable: () => pushAvailable,
          // Only records the key; the bind runs through arm once a watcher
          // exists. A key change after session.end binds nothing: the manual
          // generation drops it, and stopWatcher's armSeq bump cancels a bind
          // still resolving.
          keyChanged: (key) => {
            autoKey = key
            const target = watcher
            if (target) bindAuto(target, mailboxLog)
          },
          server: async () => {
            await serverResolved
            return server
          },
          call: callText,
          root: () => $.session.root(),
          sessionId: () => $.session.id(),
          store: {
            get: (key) => $.store.get(key),
            set: (key, value) => $.store.set(key, value),
            delete: (key) => $.store.delete(key),
            keys: () => $.store.keys(),
          },
          now: () => $.clock.now(),
          timer: (ms, fn) => $.clock.after(ms, fn),
          messages: () => $.session.messages({ as: 'api' }),
          log: (text, to) => $.ui.log(text, { to }),
        })
      }
    } catch (err) {
      $.ui.log(`ws manual: setup failed: ${String(err)}`, { to: 'debug' })
    }

    // Every registration is awaited, so each tool is listed by turn one.
    await Promise.all([mailboxSetup, compactionSetup, reconnectSetup])
    return next(e)
  })

  // The manual section, appended last. Only the first compose after a reset
  // reaches the API, so only that one waits (bounded below the hook budget);
  // the rest append what is cached. A failure appends nothing.
  on('prompt.compose', async ($, e, next) => {
    const res = await next(e)
    if (!compactionEnabledFor($.plugin.name)) return res
    const text = await manual.compose(e.traits)
    if (text === undefined || res.sections.some((s) => s.id === SECTION_ID)) return res
    return { sections: [...res.sections, { id: SECTION_ID, text, scope: 'session' as const }] }
  }).catch(($, e, next) => next(e))

  /**
   * A main-thread ws read's result as the model should see it: the state-only
   * `workflow_manual` and the repeat-read pointer. A subagent's call, another
   * server's tool, a refusal, and an error result pass through as they are.
   */
  const rewriteRead = async (
    family: ReadFamily,
    call: { tool: string; agentId?: string },
    res: ToolCallResult,
  ): Promise<ToolCallResult> => {
    if (call.agentId !== undefined || !server || call.tool !== mcpToolName(server, family)) return res
    if (res.deny !== undefined || res.isError) return res
    const text = resultText(res)
    if (text === undefined) return res
    const out = await manual.read(family, call.tool, call as unknown as Record<string, unknown>, text)
    return out === undefined ? res : { result: out }
  }

  // Mailbox steer: mail queued while a main-thread turn runs is appended at
  // the turn's next tool boundary, before the result returns, since a tool
  // result is always followed by a model request in the same turn. Only the
  // model's own main-thread calls count: the mod's own drain and slug lookup
  // pass here too, with this plugin as origin, and no request follows them.
  // A row appended at a call the person interrupted reaches the model with
  // their next prompt instead.
  // Registered first among the tool.call hooks, so it wraps the tools the mod
  // serves itself. It has no .catch: it fails only when the call beneath
  // does, and a skipped hook keeps what its next settled to, where a .catch
  // replaying next(e) would turn an answer a hook beneath gave into that
  // rejection.
  on('tool.call', async ($, e, next) => {
    const res = await next(e)
    if (e.agentId === undefined && next.origin.plugin === 'engine') {
      try {
        await watcher?.toolCompleted()
      } catch (err) {
        $.ui.log(`${$.plugin.name} mailbox: tool boundary flush failed: ${String(err)}`, { to: 'debug' })
      }
    }
    return res
  })

  // A failure keeps the result the tool gave: next(e) replays what it settled to.
  on('tool.call', { tool: /__workflow_manual$/ }, async ($, e, next) => {
    const res = await next(e)
    if (!compactionEnabledFor($.plugin.name)) return res
    return rewriteRead('workflow_manual', e, res)
  }).catch(($, e, next) => next(e))

  on('tool.call', { tool: /__playbook_read$/ }, async ($, e, next) => {
    const res = await next(e)
    if (!compactionEnabledFor($.plugin.name)) return res
    return rewriteRead('playbook_read', e, res)
  }).catch(($, e, next) => next(e))

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
    return compaction.lever(e.agentId, (e as { continue?: unknown }).continue)
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${LEVER_TOOL}: the compaction request hook failed; see the debug log.` }))

  // Served by the mod, not by the MCP server, so it stays callable after the
  // server has gone.
  on('tool.call', { tool: /__request_mcp_reconnect$/ }, async ($, e, next) => {
    if (e.tool !== reconnectTool) return next(e)
    return reconnect.lever(e.agentId, (e as { continue?: unknown }).continue)
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${RECONNECT_TOOL}: the reconnect request hook failed; see the debug log.` }))

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

  // A main-thread turn start also tells the compaction and reconnect
  // features, whose resume prompts yield to a turn already started.
  on('turn.start', async ($, e, next) => {
    if (e.agentId === undefined) mainTurn = true
    try {
      if (compactionEnabledFor($.plugin.name)) compaction.turnStarted(e.agentId)
    } catch (err) {
      $.ui.log(`ws compaction: turn start handling failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      reconnect.turnStarted(e.agentId)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mcp reconnect: turn start handling failed: ${String(err)}`, { to: 'debug' })
    }
    return next(e)
  })

  // Each feature's turn end in isolation; none waits on what it starts.
  on('turn.complete', async ($, e, next) => {
    // The mailbox's turn end runs even when a hook beneath rejects: a turn
    // flag left set would queue mail with no boundary or end to flush it.
    let res: Awaited<ReturnType<typeof next>>
    try {
      res = await next(e)
    } finally {
      if (e.agentId === undefined) {
        mainTurn = false
        try {
          watcher?.turnCompleted()
        } catch (err) {
          $.ui.log(`${$.plugin.name} mailbox: turn end handling failed: ${String(err)}`, { to: 'debug' })
        }
      }
    }
    try {
      if (compactionEnabledFor($.plugin.name)) compaction.turnComplete(e.agentId, e.reason)
    } catch (err) {
      $.ui.log(`ws compaction: turn end handling failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      reconnect.turnComplete(e.agentId, e.reason)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mcp reconnect: turn end handling failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  })

  // Observes every compaction without changing it; an observer never vetoes,
  // so a failure replays what settled beneath, or lets it run. The lever's
  // own compaction does not pass here; the compaction feature tells the
  // manual feature of that landing itself (leverLanded).
  on('session.compact', async ($, e, next) => {
    if (!compactionEnabledFor($.plugin.name)) return next(e)
    const res = await next(e)
    try {
      compaction.compacted(e.trigger, e.agentId, res)
    } catch (err) {
      $.ui.log(`ws compaction: compaction observation failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      manual.compacted(e.trigger, e.agentId, res)
    } catch (err) {
      $.ui.log(`ws manual: compaction observation failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  }).catch(($, e, next) => next(e))

  // Each step ends in isolation, so a throw in one never skips another. The
  // synchronous steps run before the one awaited: should the hook's time
  // bound cut the $.store delete short, the loop is already stopped and the
  // compaction, reconnect, and manual state already reset.
  on('session.end', async ($, e, next) => {
    // Mailbox: every end reason stops the loop: after `clear` or `resume` the
    // process goes on under another session id, and a loop still bound to the
    // old key would drain its mail into an unrelated conversation; mail still
    // queued for a turn is recorded, not delivered. Compaction is not a
    // session end, so it keeps the binding.
    try {
      autoKey = undefined
      mainTurn = false
      stopWatcher(`session.end ${e.reason}`)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mailbox: stop failed: ${String(err)}`, { to: 'debug' })
    }
    // Compaction: every end reason starts the conversation's state over; the
    // next conversation baselines the latch from its first usage.
    try {
      if (compactionEnabledFor($.plugin.name)) compaction.end(e.reason)
    } catch (err) {
      $.ui.log(`ws compaction: reset failed: ${String(err)}`, { to: 'debug' })
    }
    // Reconnect: every end reason drops a pending request, so it never acts
    // in the next conversation after `clear` or `resume`.
    try {
      reconnect.end(e.reason)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mcp reconnect: reset failed: ${String(err)}`, { to: 'debug' })
    }
    // Manual: every end reason drops the cached manual and the snapshot flag,
    // so the next conversation (no session.start follows `clear`) sets up
    // again with its own key and picks up an updated manual. Its stored key
    // stays, for a later resume of the ended session.
    try {
      if (compactionEnabledFor($.plugin.name)) manual.end(e.reason)
    } catch (err) {
      $.ui.log(`ws manual: reset failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      await $.store.delete(STORE_PREFIX + e.sessionId)
    } catch {
      // A leftover entry only re-binds a reload of that ended session.
    }
    return next(e)
  })
}

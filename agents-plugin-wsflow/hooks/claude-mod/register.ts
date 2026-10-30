import type { HookStream, ProcessSpawnChunk, ProcessSpawnResult, Register, ToolCallResult } from 'claude-code'

import { createBand, drawBand, readGit, type Mailbox } from './band'
import { appendOwn, createCaptions, drawCall, drawEmpty, drawRun, groupUnit, submitOwn, toolUseUnit, type OwnDeliveryEdges } from './captions'
import { createCompaction } from './compaction'
import { createManual, SECTION_ID, type ReadFamily } from './manual'
import { expandPreload } from './preload'
import { createReconnect, reconnectDescription, TOOL_NAME as RECONNECT_TOOL } from './reconnect'
import { ADVISORY_KEY, LEVER_DESCRIPTION, TOOL_NAME as LEVER_TOOL } from './timing'
import {
  createWatcher,
  mailboxLabel,
  mapWaitExit,
  parseRecvText,
  type BindResult,
  type DrainMode,
  type DrainResult,
  type Watcher,
  type WaitOutcome,
} from './watcher'

// The Claude Code mod: the one hooks module a package's hooks.json names,
// carrying seven features.
//
// - Mailbox push: the mod loops the one-shot `mailbox wait` as a peek-only
//   signal for one bound session key, and arriving mail starts a turn through
//   $.prompt.submit, or, while a main-thread turn runs, joins it through
//   $.session.append at its next tool boundary (see watcher.ts for the loop).
//   The drain mode is the claude.mailbox_drain knob's. Where the manual
//   feature owns the conversation's key (its knob on, in a ws project) the
//   mod binds every key the conversation gets itself; elsewhere the agent
//   binds its key through the mod-registered mailbox watch tool.
// - Lead compaction timing: the compaction request tool, always registered,
//   whose opt-in resume prompt yields to any turn already started after the
//   compaction, and (behind claude.compaction_advisory_enabled) advisory and
//   milestone wake turns before native auto compaction (see compaction.ts).
// - MCP reconnect lever: a tool the model calls when the plugin's MCP server
//   has gone, and `/mcp reconnect` at the next main-thread turn end, with the
//   same opt-in resume prompt; and (behind claude.mcp_watchdog_enabled) a
//   watchdog that runs the same reconnect from a timer when the server's
//   tools have left the tool list, so an idle session recovers with no model
//   call (see reconnect.ts).
// - Workflow manual injection (behind claude.manual_injection_enabled, in a
//   ws project): a lead session key
//   minted per conversation and the keyed workflow manual in the system
//   prompt, `workflow_manual` answered with the session state alone while
//   the prompt carries the manual, repeat ws reads collapsed to a pointer, and
//   a revive's key adopted as the conversation's (see manual.ts).
// - Playbook preload (rides on the manual feature's key): a lead skill shim's
//   `<ws-preload>` block replaced, when the skill is expanded for the model,
//   with its playbook body read with the conversation's key, so the skill
//   arrives with its procedure in it (see preload.ts).
// - Tool-call captions (off until claude.display_summary_enabled is set): a
//   finished, collapsed tool row, or a run the engine folds into one group,
//   drawn as a short caption a second model call wrote, the raw row still the
//   one the model reads and the expanded view draws (see captions.tsx). The
//   mod's own prompts are delivered split under the same switch: the model
//   reads the full text from a hidden row, the person sees a one-liner, and
//   a mailbox batch's recap draws as a transcript line.
// - Status band (off until claude.status_band_enabled is set): a framed band
//   above the prompt with the model, folder, git, context, rate-limit and
//   cost figures, and the held key's todos, agenda and mailbox address (its
//   reply id when it owns no named inbox), each refreshed by events (see
//   band.tsx).
//
// Claude Code accepts one hooks module per plugin and one unmatched hook per
// event in it, so the lifecycle events (session.start, session.end,
// turn.start, turn.step, turn.complete, session.compact) are registered once here and
// drive each feature in isolation: a throw in one is logged and leaves the
// others running. The one cross-feature wait is the shared ws-project gate: the
// mailbox setup awaits it to decide whether its bind tool registers, and the
// manual setup awaits the mailbox's host-command probe for its mailbox line.
//
// One source serves every package that ships it (this directory is mirrored
// byte-identically), so package identity is read from the engine at runtime:
// $.plugin.name names the server and the transcript lines, $.plugin.root
// locates the launcher, and $.mcp.connect(<plugin name>) resolves the MCP
// server, at session.start for every feature (a dead server cannot be
// resolved later). No package or server name is spelled: what differs per
// package is only each knob's default, in the package's config manifest. A
// server that never resolved (it was not up at start) gets a name for the
// reconnect alone, derived from the package's own plugin.json under
// $.plugin.root as `plugin:<plugin name>:<its mcpServers key>`.
//
// The advisory, manual and drain knobs (KNOB) are read keyless, once at
// session start (the manual's once per module instance, inside the ws-project
// gate), so only their project, global and built-in values apply: the manual
// knob decides whether a key is minted at all, so no key exists to read it
// with, and switching key minting or the mailbox binding mid-session is
// unsafe. A failed or refused read leaves its feature inert (the drain reads
// as notice); only a failed first manual read is read once more, at the next
// successful server resolution (a later start, a reconnect). The watchdog
// knobs are the deliberate exception: a failed, refused or invalid read takes
// the knob's default from the package's own config-manifest.json under
// $.plugin.root, so a session whose server never came up still runs the
// watchdog that can bring it back; reading the package's own file needs no
// per-package code default. The features those reads gate come up when the read settles,
// outside session.start's awaited path, so a slow server start never delays
// the first prompt: a tool registered after session.start is listed to the
// model from its next request on. The compaction lever reads no knob: it is
// registered in session.start's awaited path, so no start leaves a listed
// lever without the handler that answers it.

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

/** The keys of the knobs register.ts reads itself; captions and the band read their own. */
export const KNOB = {
  compactionAdvisory: 'claude.compaction_advisory_enabled',
  manual: 'claude.manual_injection_enabled',
  drain: 'claude.mailbox_drain',
  watchdog: 'claude.mcp_watchdog_enabled',
  watchdogInterval: 'claude.mcp_watchdog_interval_minutes',
} as const

/** Whether a value is a positive integer, the only valid watchdog interval. */
export function isPositiveInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
}

/** The first `mcpServers` key of a plugin.json text, or undefined with none or unparsable text. */
export function serverKeyOf(pluginJson: string): string | undefined {
  try {
    const servers = (JSON.parse(pluginJson) as { mcpServers?: unknown } | null)?.mcpServers
    if (!servers || typeof servers !== 'object' || Array.isArray(servers)) return undefined
    return Object.keys(servers)[0]
  } catch {
    return undefined
  }
}

/** The name the host gives a plugin's MCP server, `plugin:<plugin>:<mcpServers key>`. */
export function derivedServerName(plugin: string, key: string): string {
  return `plugin:${plugin}:${key}`
}

/** A key's `default` in a config-manifest.json text; undefined when absent or the text is unparsable. */
export function manifestDefault(manifestJson: string, key: string): unknown {
  try {
    const keys = (JSON.parse(manifestJson) as { keys?: unknown } | null)?.keys
    if (!Array.isArray(keys)) return undefined
    return (keys.find((k) => (k as { key?: unknown } | null)?.key === key) as { default?: unknown } | undefined)?.default
  } catch {
    return undefined
  }
}

/** The drain mode a claude.mailbox_drain value selects: `direct` drains through recv itself; anything else, a failed read among them, takes the notice path. */
export function drainModeOf(value: unknown): DrainMode {
  return value === 'direct' ? 'direct' : 'notice'
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

/** The `self` object of a `lookup_peers` `format: "json"` result; undefined for a refusal, an error, or unparsable text. */
function lookupSelf(res: ToolCallResult): Record<string, unknown> | undefined {
  if (res.deny !== undefined || res.isError) return undefined
  const text =
    typeof res.text === 'string'
      ? res.text
      : (res.result as { content?: { type: string; text?: string }[] } | undefined)?.content?.find((b) => b.type === 'text')?.text
  if (!text) return undefined
  try {
    const self = (JSON.parse(text) as { self?: unknown } | null)?.self
    return self && typeof self === 'object' ? (self as Record<string, unknown>) : undefined
  } catch {
    return undefined
  }
}

const trimmed = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined)

/**
 * The caller's own named-inbox address from a `lookup_peers` `format: "json"`
 * result (`self.address`, present only when the key owns that inbox), or
 * undefined for a refusal, an error, or no address. Never the reply id: this
 * is the watcher's slug, and a reply id names no inbox to wait on.
 */
export function selfAddress(res: ToolCallResult): string | undefined {
  return trimmed(lookupSelf(res)?.address)
}

/**
 * What the band's mailbox capsule shows from the same result: the named-inbox
 * address when the key owns one, else its reply id (`self.reply_id`, flagged
 * `reply` so the band draws it dimmer); undefined with neither or a failed read.
 */
export function bandMailbox(res: ToolCallResult): Mailbox | undefined {
  const self = lookupSelf(res)
  const address = trimmed(self?.address)
  if (address) return { id: address, reply: false }
  const replyId = trimmed(self?.reply_id)
  return replyId ? { id: replyId, reply: true } : undefined
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
   * Whether the session runs in a ws project with the manual knob on:
   * evaluated once per module instance and shared by the manual feature (it
   * mints keys only then) and the mailbox feature (its bind tool registers
   * only otherwise), so a session never holds both the lever and the
   * automatic bind. A failed check or knob read reads as closed for both.
   * The one exception is a failed first knob read (manualRetry): it is read
   * again once the server resolves again, and a true read then opens the
   * gate late, after the mailbox already registered its bind tool, which
   * stays. A key the model already bound through it is handed to the manual
   * feature, which re-logs in with that key instead of minting, so the late
   * open never moves the push off a key the model holds; both binds then go
   * through arm, the latest key winning. With the watcher unbound (a hot
   * reload whose restore found the server down), that key is the bind's
   * reload entry.
   */
  let wsGate: Promise<boolean> | undefined
  /**
   * Set when the first manual knob read failed (server unresolved, or the
   * read errored or was refused): the next successful server resolution
   * (a later start, a reconnect) reads it once more. A read that returned a
   * value, whatever it was, is never read again in this module instance.
   */
  let manualRetry = false
  /**
   * `playbook_read`'s text for the preload, set at session.start once the
   * mod's MCP call edge exists; undefined before, which reads as a failure.
   */
  let readPlaybook: ((name: string, key: string) => Promise<string>) | undefined
  /** Bumped by every session.start: a setup still waiting on its knob read yields to a newer start. */
  let starts = 0

  const stopWatcher = (reason: string) => {
    armSeq += 1
    watcher?.stop(reason)
  }

  // The plugin's MCP server name, shared by every feature: resolved at
  // session.start, again by a bind and after a reconnect, and kept while the
  // server is down, when $.mcp.connect can no longer resolve it.
  let server: string | undefined
  /**
   * The server name derived from the plugin's own plugin.json, for a server
   * that never resolved. It feeds only the reconnect's server edge, never the
   * shared cache above: the band, the knob reads and the recv tool would
   * otherwise target a server that never connected.
   */
  let derivedServer: string | undefined
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

  // Compaction feature state; started, and its lever registered, at every
  // session.start. Only the advisory's usage observation waits for the
  // latest start's advisory knob read to settle on.
  const compaction = createCompaction()
  let leverTool: string | undefined
  let advisoryOn = false

  // Reconnect feature state; its tool name is set at session.start.
  const reconnect = createReconnect()
  let reconnectTool: string | undefined

  // Manual feature state; started at every session.start, inert unless the
  // ws-project gate (its knob among it) opens.
  const manual = createManual()

  // Caption feature state; started at every session.start, inert until its
  // own knob reads on.
  const captions = createCaptions()

  // Status band state; started at every session.start, drawing nothing until
  // its own knob reads on.
  const band = createBand()

  on('session.start', async ($, e, next) => {
    const label = mailboxLabel($.plugin.name)
    const mailboxLog = (text: string) => $.ui.log(`${label}: ${text}`, { to: 'debug' })
    // How the mod's own prompts reach the conversation: split while captions are on (see submitOwn).
    const own: OwnDeliveryEdges = {
      isOn: () => captions.isOn(),
      append: async (type, text) => {
        const res = await $.session.append({ message: { type, content: [{ type: 'text', text }] } })
        if (res.deny !== undefined) throw new Error(`row refused: ${res.deny}`)
      },
      submit: (text) => $.prompt.submit({ text }),
      line: (text) => $.ui.log(text),
      recap: (mail) => captions.recap(mail),
      turnRunning: () => mainTurn,
      sleep: (ms) => new Promise<void>((resolve) => void $.clock.after(ms, resolve)),
      log: (text) => $.ui.log(`${$.plugin.name} captions: ${text}`, { to: 'debug' }),
    }

    /** One ws MCP tool call's text; an error result rejects with its text. */
    const callText = async (name: string, tool: string, args: Record<string, unknown>) => {
      const res = await $.mcp.call(name, tool, args)
      const text = res.content.flatMap((b) => (b.type === 'text' && typeof b.text === 'string' ? [b.text] : [])).join('\n')
      if (res.isError) throw new Error(text || `${tool} reported an error`)
      return text
    }
    readPlaybook = async (name, key) => {
      await serverResolved
      if (!server) throw new Error('MCP server not resolved')
      return callText(server, 'playbook_read', { name, session_key: key })
    }
    /** A keyless config_get once the shared server resolution settles; rejects on any failure. */
    const readSetting = async (key: string) => {
      await serverResolved
      if (!server) throw new Error('MCP server not resolved')
      return (JSON.parse(await callText(server, 'config_get', { key, format: 'json' })) as { value?: unknown }).value
    }
    /** A knob read for this start: a failure or a refusal is logged and settles undefined, the inert state. */
    const readKnob = (key: string): Promise<unknown> =>
      readSetting(key).catch((err) => {
        $.ui.log(`${$.plugin.name} mod: ${key} not read (${String(err)}); its feature stays off`, { to: 'debug' })
        return undefined
      })
    /** The one retry of a failed first manual knob read: a true read opens the gate and starts the manual feature. */
    const retryManual = () => {
      if (!manualRetry) return
      manualRetry = false
      readSetting(KNOB.manual)
        .then(async (value) => {
          if (value !== true) return
          // The key the model bound: the watcher's, else the bind's reload
          // entry, which a reload restore that found the server down never
          // re-bound. Read before the gate opens, so a failed read leaves
          // it closed rather than minting over that key.
          const reloaded = storedKey(await $.store.get(STORE_PREFIX + (await $.session.id())))
          const held = watcher?.current() ?? reloaded
          wsGate = Promise.resolve(true)
          manual.opened(held)
        })
        .catch((err) => $.ui.log(`${$.plugin.name} mod: ${KNOB.manual} not read again (${String(err)}); its feature stays off`, { to: 'debug' }))
    }
    const resolveServer = async () => {
      const conn = await $.mcp.connect($.plugin.name)
      if (conn.isConnected) {
        useServer(conn.server)
        retryManual()
        try {
          reconnect.resolved()
        } catch (err) {
          $.ui.log(`${$.plugin.name} mcp watchdog: resolution handling failed: ${String(err)}`, { to: 'debug' })
        }
      }
      return conn
    }
    // Not awaited, so a slow server start never delays the first prompt.
    const resolved = resolveServer().catch((err) => {
      $.ui.log(`${$.plugin.name} mod: MCP server not resolved at start: ${String(err)}`, { to: 'debug' })
      return undefined
    })
    serverResolved = resolved

    // The project check first: outside a ws project the manual knob is never read.
    wsGate ??= (async () => {
      const repoRoot = await $.session.repo().then(
        (repo) => repo?.root,
        () => undefined,
      )
      const isWsProject = repoRoot
        ? await $.fs.exists(`${repoRoot.replace(/[\\/]+$/, '')}/ai-docs/WORKFLOW.md`)
        : (await $.fs.ancestors({ names: ['ai-docs/WORKFLOW.md'] })).length > 0
      if (!isWsProject) return false
      try {
        return (await readSetting(KNOB.manual)) === true
      } catch (err) {
        manualRetry = true
        $.ui.log(`${$.plugin.name} mod: ${KNOB.manual} not read (${String(err)}); its feature stays off until the server resolves again`, { to: 'debug' })
        return false
      }
    })().catch((err) => {
      $.ui.log(`${$.plugin.name} mod: workflow project check failed (${String(err)}); reading as no workflow project`, { to: 'debug' })
      return false
    })
    const gate = wsGate
    const startSeq = ++starts
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
      // The watcher comes up once the drain read settles.
      const mode = drainModeOf(await readKnob(KNOB.drain))
      // WS_MAILBOX is the fallback slug: a re-derived auto-minted address would
      // not match the server's, so the direct mode asks the server for the
      // key's own address (lookup_peers self.address) through the same tool
      // path its drain uses. The notice mode makes no tool calls of its own.
      const envSlug = (await $.env.get('WS_MAILBOX'))?.trim() || undefined
      // A newer start since this one began owns the mailbox. Checked after
      // the last await before the assignment: that start stopped the watcher
      // before this one existed, so one assigned now would be overwritten
      // unstopped and its wait loop run on beside the newer one.
      if (startSeq !== starts) return
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
        submit: async (prompts, live) => {
          const res = await submitOwn(own, prompts, live)
          if (res.drop !== undefined) throw new Error(`prompt dropped: ${res.drop}`)
          return res.busy ? 'busy' : res.stale ? 'stale' : undefined
        },
        turnRunning: () => mainTurn,
        append: (prompts) => appendOwn(own, prompts),
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
      const current = () => watcher === created
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

    // Until this start's advisory knob read settles on, no usage is observed,
    // so no advisory or milestone turn is sent; the lever works regardless.
    advisoryOn = false
    const advisoryRead = readKnob(KNOB.compactionAdvisory).then((value) => value === true)
    void advisoryRead.then((enabled) => {
      if (enabled && startSeq === starts) advisoryOn = true
    })
    const compactionSetup = (async () => {
      compaction.start({
        submit: (prompt, live) => submitOwn(own, [prompt], live),
        // The slash command, not $.session.compact(): that call is refused in a
        // headless (-p / SDK) session, and the command compacts under the REPL
        // and headless alike, passing this mod's own session.compact hook.
        compact: () => $.command.run({ command: 'compact' }),
        after: (fn) => void $.clock.after(0, fn),
        delay: (ms, fn) => void $.clock.after(ms, fn),
        log: (text, to) => $.ui.log(text, { to }),
        plugin: $.plugin.name,
        turnRunning: () => mainTurn,
        readUsage: async () => (await $.session.usage()).context,
        // One config_get per session start with the advisory on, once its knob read settles.
        readAdvisory: async () => {
          if (!(await advisoryRead)) throw new Error(`${KNOB.compactionAdvisory} is off`)
          return readSetting(ADVISORY_KEY)
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
    })().catch((err) => $.ui.log(`${$.plugin.name} compaction: setup failed (${String(err)}); ${LEVER_TOOL} not registered`, { to: 'debug' }))

    const pluginRoot = $.plugin.root.replace(/[\\/]+$/, '')
    // Not awaited: read once per module instance, for a server that never resolves.
    if (derivedServer === undefined) {
      void $.fs
        .read(`${pluginRoot}/.claude-plugin/plugin.json`)
        .then((text) => {
          const key = serverKeyOf(text)
          if (key === undefined) throw new Error('no mcpServers key')
          derivedServer = derivedServerName($.plugin.name, key)
        })
        .catch((err) => $.ui.log(`${$.plugin.name} mcp reconnect: server name not derived from plugin.json (${String(err)})`, { to: 'debug' }))
    }
    /**
     * A watchdog knob: the keyless read, else (failed, refused, or not
     * `valid`) the package manifest's default, else undefined, the inert state.
     */
    const readWatchdogKnob = async <T,>(key: string, valid: (value: unknown) => value is T): Promise<T | undefined> => {
      let why: string
      try {
        const value = await readSetting(key)
        if (valid(value)) return value
        why = `invalid value ${JSON.stringify(value)}`
      } catch (err) {
        why = String(err)
      }
      try {
        const value = manifestDefault(await $.fs.read(`${pluginRoot}/config-manifest.json`), key)
        if (valid(value)) {
          $.ui.log(`${$.plugin.name} mod: ${key} not read (${why}); the package default ${JSON.stringify(value)} applies`, { to: 'debug' })
          return value
        }
        throw new Error(`invalid package default ${JSON.stringify(value)}`)
      } catch (err) {
        $.ui.log(`${$.plugin.name} mod: ${key} not read (${why}) and no package default (${String(err)}); its feature stays off`, { to: 'debug' })
        return undefined
      }
    }

    const reconnectSetup = (async () => {
      reconnect.start({
        plugin: $.plugin.name,
        server: () => server ?? derivedServer,
        reconnect: (name) => $.command.run({ command: 'mcp', args: `reconnect ${name}` }),
        toolsListed: async (name) => {
          const prefix = mcpToolName(name, '')
          return (await $.tool.list()).some((t) => t.name.startsWith(prefix))
        },
        resolve: async () => void (await resolveServer()),
        submit: (prompt, live) => submitOwn(own, [prompt], live),
        append: (prompt) => appendOwn(own, [prompt]),
        after: (fn) => void $.clock.after(0, fn),
        sleep: (ms) => new Promise<void>((resolve) => void $.clock.after(ms, resolve)),
        log: (text, to) => $.ui.log(text, { to }),
        turnRunning: () => mainTurn,
        every: (ms, fn) => {
          const timer = $.clock.every(ms, fn)
          return () => timer.cancel()
        },
      })
      // The watchdog comes up once its knob reads settle, outside the awaited path.
      void (async () => {
        if ((await readWatchdogKnob(KNOB.watchdog, (v): v is boolean => typeof v === 'boolean')) !== true) return
        const minutes = await readWatchdogKnob(KNOB.watchdogInterval, isPositiveInteger)
        if (minutes === undefined || startSeq !== starts) return
        reconnect.watch(minutes * 60_000)
      })().catch((err) => $.ui.log(`${$.plugin.name} mcp watchdog: setup failed: ${String(err)}`, { to: 'debug' }))
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

    // Registers nothing; its settings read runs on in the background, and rows
    // draw raw until it settles on.
    try {
      captions.start({
        // One keyless config_get per key, once the shared server resolution settles.
        readSetting,
        messages: () => $.session.messages({ as: 'api' }),
        complete: (request) => $.model.complete(request),
        redraw: () => $.ui.invalidate('ui.render'),
        transcript: (text) => $.ui.log(text),
        // $.clock.every hands back a Timer; the edge promises its stop function.
        every: (ms, fn) => {
          const timer = $.clock.every(ms, fn)
          return () => timer.cancel()
        },
        log: (text) => $.ui.log(text, { to: 'debug' }),
        plugin: $.plugin.name,
        // Headless forces captions off for the session, and with them the
        // split delivery of the mod's own prompts and the mailbox recaps.
        interactive: e.isInteractive,
      })
    } catch (err) {
      $.ui.log(`${$.plugin.name} captions: setup failed: ${String(err)}`, { to: 'debug' })
    }

    // Registers nothing; its knob read runs on in the background, and the band
    // draws nothing until it settles on. Started before the manual feature,
    // whose key listener it follows.
    // Without the manual feature no key arrives, so the band draws its
    // key-less rows only.
    try {
      const wsCall = async (tool: string, args: Record<string, unknown>) => {
        await serverResolved
        if (!server) throw new Error('MCP server not resolved')
        return callText(server, tool, args)
      }
      band.start({
        // Session-keyed while a key is held, so a session-scope tune is honored.
        readSetting: async (key, sessionKey) =>
          (JSON.parse(await wsCall('config_get', { key, format: 'json', ...(sessionKey ? { session_key: sessionKey } : {}) })) as { value?: unknown })
            .value,
        call: wsCall,
        address: async (sessionKey) =>
          bandMailbox({ text: await wsCall('mailbox_lookup_peers', { session_key: sessionKey, scope: 'worktree', format: 'json' }) } as ToolCallResult),
        git: (cwd) => readGit((argv, init) => $.process.run(argv, init), cwd),
        usage: () => $.session.usage({ breakdown: 'summary' }),
        model: () => $.session.model(),
        cwd: () => $.session.cwd(),
        root: () => $.session.root(),
        now: () => $.clock.now(),
        every: (ms, fn) => {
          const timer = $.clock.every(ms, fn)
          return () => timer.cancel()
        },
        redraw: () => $.ui.invalidate('ui.render'),
        log: (text) => $.ui.log(text, { to: 'debug' }),
        plugin: $.plugin.name,
      })
    } catch (err) {
      $.ui.log(`${$.plugin.name} band: setup failed: ${String(err)}`, { to: 'debug' })
    }

    // Registers no tool: its setup runs on in the background, and the first
    // compose waits for it. Inert (no key, no section, no rewrite) unless the
    // ws-project gate opens.
    try {
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
          try {
            band.keyChanged(key)
          } catch (err) {
            $.ui.log(`${$.plugin.name} band: key change handling failed: ${String(err)}`, { to: 'debug' })
          }
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
        plugin: $.plugin.name,
      })
    } catch (err) {
      $.ui.log(`${$.plugin.name} manual: setup failed: ${String(err)}`, { to: 'debug' })
    }

    // The reconnect and compaction levers' registrations are awaited, so they
    // are listed by turn one. The mailbox setup waits on knob reads, so it
    // runs on and its tool is listed from the model's next request after it
    // registers.
    void mailboxSetup
    await Promise.all([reconnectSetup, compactionSetup])
    return next(e)
  })

  // The manual section, appended last. Only the first compose after a reset
  // reaches the API, so only that one waits (bounded below the hook budget);
  // the rest append what is cached. A failure appends nothing.
  on('prompt.compose', async ($, e, next) => {
    const res = await next(e)
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

  // Playbook preload: only this plugin's skills, and only while the manual
  // feature holds the conversation's key. A failure passes the text through
  // as the hooks beneath computed it.
  on('skill.prompt', async ($, e, next) => {
    const res = await next(e)
    const read = readPlaybook
    if (!read) return res
    const text = await expandPreload({ skill: e.skill, text: res.text, pluginName: e.pluginName }, { plugin: $.plugin.name, key: () => manual.key(), read })
    return text === undefined ? res : { text }
  }).catch(($, e, next) => next(e))

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
  // The caption capture rides here too (the mod gets one unmatched hook per
  // event): wrapping the rewrite hooks below, it keeps the result the lead
  // reads.
  on('tool.call', async ($, e, next) => {
    const res = await next(e)
    if (e.agentId === undefined && next.origin.plugin === 'engine') {
      try {
        await watcher?.toolCompleted()
      } catch (err) {
        $.ui.log(`${$.plugin.name} mailbox: tool boundary flush failed: ${String(err)}`, { to: 'debug' })
      }
    }
    // Each feature checks its own knob: captions capture only while on, and
    // the band re-reads its knob after a config tune and refreshes only while on.
    try {
      captions.captured(e as unknown as Record<string, unknown>, res)
    } catch (err) {
      $.ui.log(`${$.plugin.name} captions: capture failed: ${String(err)}`, { to: 'debug' })
    }
    // The band's refresh rides here too: git, and a ws read after the
    // model's todo, agenda, mailbox or config tune call.
    try {
      band.toolCalled(e.tool, server, next.origin.plugin === 'engine')
    } catch (err) {
      $.ui.log(`${$.plugin.name} band: tool call handling failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  })

  // A failure keeps the result the tool gave: next(e) replays what it settled to.
  on('tool.call', { tool: /__workflow_manual$/ }, async ($, e, next) => {
    const res = await next(e)
    return rewriteRead('workflow_manual', e, res)
  }).catch(($, e, next) => next(e))

  on('tool.call', { tool: /__playbook_read$/ }, async ($, e, next) => {
    const res = await next(e)
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
    if (e.tool !== leverTool) return next(e)
    return compaction.lever(e.agentId, (e as { continue?: unknown }).continue)
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${LEVER_TOOL}: the compaction request hook failed; see the debug log.` }))

  // Served by the mod, not by the MCP server, so it stays callable after the
  // server has gone.
  on('tool.call', { tool: /__request_mcp_reconnect$/ }, async ($, e, next) => {
    if (e.tool !== reconnectTool) return next(e)
    return reconnect.lever(e.agentId, (e as { continue?: unknown }).continue)
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${RECONNECT_TOOL}: the reconnect request hook failed; see the debug log.` }))

  // Pushed after each main-thread turn and on rate-limit moves, so also
  // mid-turn: the feature only records a crossing then, and delivers it with a
  // fresh usage read once the main thread is idle. The plain figures cost
  // nothing, and the breakdown is read only when the feature's cache is empty
  // (session start or end, a landed compaction) or the window changed.
  on('session.measure', async ($, e, next) => {
    const res = await next(e)
    try {
      if (advisoryOn) await compaction.measure(e.context, () => $.session.usage({ breakdown: 'summary' }))
    } catch (err) {
      $.ui.log(`${$.plugin.name} compaction: usage observation failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      band.measured()
    } catch (err) {
      $.ui.log(`${$.plugin.name} band: usage observation failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  })

  // The effort is known only from each model request: the band takes the
  // main loop's, with its model.
  on('turn.step', async function* ($, e, next) {
    if (e.agentId === undefined) {
      try {
        band.stepped(e.model, e.effort)
      } catch (err) {
        $.ui.log(`${$.plugin.name} band: model request handling failed: ${String(err)}`, { to: 'debug' })
      }
    }
    return yield* next(e)
  })

  // A main-thread turn start also tells the compaction and reconnect
  // features, whose resume prompts yield to a turn already started.
  on('turn.start', async ($, e, next) => {
    if (e.agentId === undefined) mainTurn = true
    try {
      compaction.turnStarted(e.agentId)
    } catch (err) {
      $.ui.log(`${$.plugin.name} compaction: turn start handling failed: ${String(err)}`, { to: 'debug' })
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
      compaction.turnComplete(e.agentId, e.reason)
    } catch (err) {
      $.ui.log(`${$.plugin.name} compaction: turn end handling failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      if (e.agentId === undefined) band.turnCompleted()
    } catch (err) {
      $.ui.log(`${$.plugin.name} band: turn end handling failed: ${String(err)}`, { to: 'debug' })
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
  // own compaction (a `/compact` command) passes here like any other, so
  // this is the one landing fan-out for it too.
  on('session.compact', async ($, e, next) => {
    const res = await next(e)
    try {
      compaction.compacted(e.trigger, e.agentId, res)
    } catch (err) {
      $.ui.log(`${$.plugin.name} compaction: compaction observation failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      manual.compacted(e.trigger, e.agentId, res)
    } catch (err) {
      $.ui.log(`${$.plugin.name} manual: compaction observation failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      captions.compacted(e.trigger, e.agentId, res)
    } catch (err) {
      $.ui.log(`${$.plugin.name} captions: compaction observation failed: ${String(err)}`, { to: 'debug' })
    }
    return res
  }).catch(($, e, next) => next(e))

  // Tool-call captions. Fullscreen folds runs of calls into one ToolGroup, so
  // most rows are drawn there; the main screen draws calls one by one, each
  // its own unit. A unit with no caption yet draws raw and is queued; the
  // live group, a running call and the expanded view always draw raw.
  on('ui.render', { component: 'ToolGroup' }, async ($, e, next) => {
    if (!captions.isOn()) return next(e)
    const unit = groupUnit(e.props)
    if (!unit) return next(e)
    const [first] = unit.calls
    if (unit.calls.length === 1 && first) {
      const caption = captions.toolCaption(first.id)
      if (caption) return drawCall($.ui.resolve(e), first, caption, e.viewport?.columns)
    } else {
      const caption = captions.runCaption(unit.key)
      if (caption) return drawRun($.ui.resolve(e), unit.calls, caption, e.viewport?.columns)
    }
    captions.request(unit)
    return next(e)
  })

  on('ui.render', { component: 'ToolUse' }, async ($, e, next) => {
    if (!captions.isOn()) return next(e)
    const unit = toolUseUnit(e.props)
    const [call] = unit?.calls ?? []
    if (!unit || !call) return next(e)
    const caption = captions.toolCaption(call.id)
    if (caption) return drawCall($.ui.resolve(e), call, caption, e.viewport?.columns)
    captions.request(unit)
    return next(e)
  })

  // A captioned call's row already draws its result.
  on('ui.render', { component: 'ToolResult' }, async ($, e, next) => {
    if (!captions.isOn() || e.props.isExpanded || e.props.isErrored) return next(e)
    if (!captions.toolCaption(e.props.tool_use_id)) return next(e)
    return drawEmpty($.ui.resolve(e))
  })

  // The status band, drawn only while its switch reads on; a survey holds the
  // band's place first.
  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey) return next(e)
    const data = band.data()
    if (!data) return next(e)
    return drawBand($.ui.resolve(e), data, e.props.bodyColumns)
  })

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
      compaction.end(e.reason)
    } catch (err) {
      $.ui.log(`${$.plugin.name} compaction: reset failed: ${String(err)}`, { to: 'debug' })
    }
    // Reconnect: every end reason drops a pending request, so it never acts
    // in the next conversation after `clear` or `resume`.
    try {
      reconnect.end(e.reason)
    } catch (err) {
      $.ui.log(`${$.plugin.name} mcp reconnect: reset failed: ${String(err)}`, { to: 'debug' })
    }
    // Captions: the next conversation's mirror starts empty; the captions,
    // keyed by the session's own tool_use_ids, stay.
    try {
      captions.end()
    } catch (err) {
      $.ui.log(`${$.plugin.name} captions: reset failed: ${String(err)}`, { to: 'debug' })
    }
    // Band: the next conversation gets its own key, so the key's rows and
    // address go until it arrives; the switch and the figures stay.
    try {
      band.end()
    } catch (err) {
      $.ui.log(`${$.plugin.name} band: reset failed: ${String(err)}`, { to: 'debug' })
    }
    // Manual: every end reason drops the cached manual and the snapshot flag,
    // so the next conversation (no session.start follows `clear`) sets up
    // again with its own key and picks up an updated manual. Its stored key
    // stays, for a later resume of the ended session.
    try {
      manual.end(e.reason)
    } catch (err) {
      $.ui.log(`${$.plugin.name} manual: reset failed: ${String(err)}`, { to: 'debug' })
    }
    try {
      await $.store.delete(STORE_PREFIX + e.sessionId)
    } catch {
      // A leftover entry only re-binds a reload of that ended session.
    }
    return next(e)
  })
}

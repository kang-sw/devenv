import type { SessionCompactResult, SessionCompactTrigger } from 'claude-code'

/**
 * Workflow manual injection, a feature of the Claude mod (ws only, and only
 * in a ws project): the mod mints a lead session key for the conversation,
 * puts the keyed workflow manual in the system prompt, answers a main-thread
 * `workflow_manual` call with the session state alone while the system prompt
 * carries the manual, and collapses a repeat `playbook_read` or
 * `workflow_manual` result to a pointer while the earlier identical result is
 * still in the conversation.
 *
 * The engine snapshots the main thread's system prompt once per conversation:
 * only the first `prompt.compose` answer after a session start, a landed
 * compaction, or `/clear` reaches the API, and a resumed session replays its
 * recorded snapshot. So the first compose after such a reset waits a bounded
 * time for the key and the manual, later composes never wait, and the
 * snapshot flag records whether the conversation's snapshot carries the
 * section. Only while it does is a `workflow_manual` result cut to the state,
 * so the manual is never in neither place.
 *
 * The engine calls arrive as edges bound in register.ts, so the state machine
 * here is driven and tested without an engine.
 */

export const SECTION_ID = 'ws:workflow-manual'

/** The first compose after a reset waits at most this long for the key and the manual (the hook budget is 10 s). */
export const COMPOSE_WAIT_MS = 5_000

/**
 * $.store key prefix for the conversation's lead key, suffixed with the
 * session id. Unlike the mailbox binding, an entry outlives session.end, so a
 * later `--resume` of that session re-logs in with the same key.
 */
export const KEY_STORE_PREFIX = 'ws-lead-key:'

/**
 * An entry older than this is pruned at session.start: ws-mcp drops a key
 * record idle this long, so such a key could no longer re-log in. Every start
 * that uses an entry refreshes its age.
 */
export const KEY_RETENTION_MS = 30 * 24 * 60 * 60 * 1000

/** The whole line ws-mcp places after the manual body; session state and notes follow it. */
export const SESSION_KEY_ANCHOR = '## Session Key'

export const STATE_POINTER_LINE =
  'Your session state (agenda, todos, notes) is not in this prompt. When no `workflow_manual` result is visible in your context — a fresh session, or after compaction — call `workflow_manual` before acting.'

export function keyLine(key: string): string {
  return (
    `Your lead session key for this conversation is \`${key}\`; pass it as \`session_key\`. ` +
    'Continuing another track (for example `ws:lead-revive` with a preserved key) is still allowed.'
  )
}

/** The system prompt section: the state pointer, the key line, then the manual. */
export function sectionText(key: string, manual: string): string {
  return `${STATE_POINTER_LINE}\n\n${keyLine(key)}\n\n${manual}`
}

export const STATE_ONLY_LINE = 'Workflow manual is in your system prompt; this is your current session state.'

export function stateOnlyText(state: string): string {
  return `${STATE_ONLY_LINE}\n\n${state}`
}

export type ReadFamily = 'playbook_read' | 'workflow_manual'

/** The result's `#`-`###` headings joined by `; `, or `(no headings)`. */
export function headings(text: string): string {
  const found = text.match(/^#{1,3}\s+.+$/gm) ?? []
  return found.length > 0 ? found.join('; ') : '(no headings)'
}

/** What a repeat read returns in place of `text`, its unchanged result. */
export function pointerText(family: ReadFamily, text: string): string {
  const lead = `The result is unchanged. Continue using the full result of your earlier \`${family}\` call in this conversation`
  return family === 'workflow_manual'
    ? `${lead} as your current session state; call workflow_manual again after you change agenda, todos, or notes. (headings: ${headings(text)})`
    : `${lead}. Do not read it again (headings: ${headings(text)}).`
}

/** The part of a keyed `workflow_manual` response before its `## Session Key` line, or undefined without one. */
export function manualPart(response: string): string | undefined {
  const lines = response.split('\n')
  const end = lines.indexOf(SESSION_KEY_ANCHOR)
  if (end === -1) return undefined
  const part = lines.slice(0, end).join('\n').trimEnd()
  return part || undefined
}

export type Cut =
  | { found: true; text: string; span: string }
  | { found: false; reason: 'start-anchor' | 'end-anchor' | 'order' | 'no-body' }

/**
 * Cut the static manual body out of a `workflow_manual` response: from the
 * first whole line equal to `staticBody`'s first non-empty line up to (not
 * including) the first whole `## Session Key` line. What ws-mcp prepends
 * (warnings, banners, the manuals list) stays ahead of the start anchor and
 * what it appends stays from the end anchor on. `span` is what the cut
 * removes. Neither anchor present is ws-mcp's no-restorable-state notice.
 */
export function cutStaticBody(response: string, staticBody: string): Cut {
  const startLine = staticBody.split('\n').find((line) => line.length > 0)
  let start = -1
  let end = -1
  let offset = 0
  for (const line of response.split('\n')) {
    if (start === -1 && startLine !== undefined && line === startLine) start = offset
    if (end === -1 && line === SESSION_KEY_ANCHOR) end = offset
    offset += line.length + 1
  }
  if (start === -1 && end === -1) return { found: false, reason: 'no-body' }
  if (start === -1) return { found: false, reason: 'start-anchor' }
  if (end === -1) return { found: false, reason: 'end-anchor' }
  if (end <= start) return { found: false, reason: 'order' }
  return { found: true, text: response.slice(0, start) + response.slice(end), span: response.slice(start, end) }
}

function stable(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value) ?? 'null'
  if (Array.isArray(value)) return `[${value.map(stable).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record)
    .sort()
    .map((k) => `${JSON.stringify(k)}:${stable(record[k])}`)
    .join(',')}}`
}

/**
 * The dedupe key of a read's arguments: `playbook_read` by `name` and
 * `context` (its outer `session_key` only picks override layers; a
 * `context.session_key` is substitution data and stays in), `workflow_manual`
 * by `session_key`.
 */
export function readKey(family: ReadFamily, args: Record<string, unknown>): string {
  return family === 'playbook_read'
    ? stable({ name: args.name, context: args.context })
    : stable({ session_key: args.session_key ?? null })
}

/** A cheap 53-bit hash, for the memory hint only: a collision costs one conversation read, never a wrong pointer. */
export function hashText(text: string): string {
  let h1 = 0xdeadbeef
  let h2 = 0x41c6ce57
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i)
    h1 = Math.imul(h1 ^ c, 2654435761)
    h2 = Math.imul(h2 ^ c, 1597334677)
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909)
  return `${text.length}:${(4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(36)}`
}

/** One message of the conversation in Messages API form, as far as the dedupe reads it. */
export type ApiMessageLike = { role: string; content: readonly { type: string; [field: string]: unknown }[] }

function blockText(content: unknown): string | undefined {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return undefined
  const texts = content.flatMap((b) => (b?.type === 'text' && typeof b.text === 'string' ? [b.text as string] : []))
  return texts.length > 0 ? texts.join('\n') : undefined
}

/**
 * Whether a fresh read whose result the model would see as `fresh` collapses
 * to a pointer, judged by the conversation alone: among the earlier results
 * of calls to `tool` with the same key, exactly one is `fresh` itself and none
 * is its pointer. So the read after a pointer returns the full text again,
 * and an earlier result gone from the conversation (compaction, `/clear`, a
 * rewind) is not a repeat.
 */
export function isRepeat(messages: readonly ApiMessageLike[], tool: string, family: ReadFamily, key: string, fresh: string): boolean {
  const calls = new Set<string>()
  for (const m of messages) {
    if (m.role !== 'assistant') continue
    for (const b of m.content) {
      if (b.type !== 'tool_use' || b.name !== tool || typeof b.id !== 'string') continue
      const input = b.input && typeof b.input === 'object' ? (b.input as Record<string, unknown>) : {}
      if (readKey(family, input) === key) calls.add(b.id)
    }
  }
  const pointer = pointerText(family, fresh)
  let full = 0
  let pointers = 0
  for (const m of messages) {
    if (m.role !== 'user') continue
    for (const b of m.content) {
      if (b.type !== 'tool_result' || b.is_error === true || typeof b.tool_use_id !== 'string' || !calls.has(b.tool_use_id)) continue
      const text = blockText(b.content)
      if (text === fresh) full += 1
      else if (text === pointer) pointers += 1
    }
  }
  return full === 1 && pointers === 0
}

/** The stored lead key of a $.store entry, or undefined. */
export function storedKey(value: unknown): string | undefined {
  const key = (value as { sessionKey?: unknown } | undefined)?.sessionKey
  return typeof key === 'string' && key.trim() ? key.trim() : undefined
}

/** The `session_key` of a `ferrule` `format: "json"` answer, or undefined. */
export function ferruleKey(text: string): string | undefined {
  try {
    const key = (JSON.parse(text) as { session_key?: unknown } | null)?.session_key
    return typeof key === 'string' && key.trim() ? key.trim() : undefined
  } catch {
    return undefined
  }
}

/**
 * Whether a landed compaction of this trigger refreshes the snapshot. Manual
 * `/compact` and a plugin's compaction do (the first request after them
 * carries a fresh compose answer); native auto compaction is not assumed to,
 * so its landing leaves the flag as it was, which is safe either way: a set
 * flag means the cache was there for every compose since, a clear one only
 * costs full results.
 */
export function landingRefreshes(trigger: SessionCompactTrigger): boolean {
  return trigger === 'manual' || trigger === 'plugin'
}

export type ManualEdges = {
  /** Whether the session runs in a ws project (`ai-docs/WORKFLOW.md` at the repository root); rejects when it cannot tell. */
  isWsProject: () => Promise<boolean>
  /** The ws MCP server's name from the mod's shared resolution, once the start's resolution settled; undefined while unresolved. */
  server: () => Promise<string | undefined>
  /** Call a ws MCP tool: its text blocks joined; rejects on a failed call or an error result. */
  call: (server: string, tool: string, args: Record<string, unknown>) => Promise<string>
  /** The root the key binds to; ws-mcp resolves it to its Git worktree. */
  root: () => Promise<string>
  sessionId: () => Promise<string>
  store: {
    get: (key: string) => Promise<unknown>
    set: (key: string, value: unknown) => Promise<void>
    delete: (key: string) => Promise<void>
    keys: () => Promise<string[]>
  }
  now: () => Promise<number>
  /** Call `fn` once after `ms`; `cancel` stops it. */
  timer: (ms: number, fn: () => void) => { cancel: () => void }
  /** The main conversation in Messages API form. */
  messages: () => Promise<readonly ApiMessageLike[]>
  log: (text: string, to: 'transcript' | 'debug') => void
}

export type Manual = {
  /** session.start: bind the edges, start the conversation over, prune stale key entries, and begin the setup. */
  start: (edges: ManualEdges) => void
  /** session.end: drop the cache and the snapshot flag; the next compose sets up again. */
  end: (reason: string) => void
  /** A prompt.compose: the section text to append, or undefined. */
  compose: (traits: readonly string[]) => Promise<string | undefined>
  /** A session.compact as it settled beneath the mod's hook. */
  compacted: (trigger: SessionCompactTrigger, agentId: string | undefined, res: SessionCompactResult) => void
  /** A landed main-conversation compaction, including the compaction lever's own. */
  landed: (trigger: SessionCompactTrigger) => void
  /**
   * A main-thread ws read result: the text the model should read in place of
   * `text`, or undefined to keep the result as it is. `tool` is its
   * model-facing name. Never throws.
   */
  read: (family: ReadFamily, tool: string, args: Record<string, unknown>, text: string) => Promise<string | undefined>
  /** Whether the conversation's snapshot carries the section (for tests and logs). */
  snapshot: () => boolean
}

/** The key and the manual for this conversation. */
type Cache = {
  key: string
  section: string
  /** The static manual body (`playbook_read` `lead-workflow-manual` with the key); its first line is the cut's start anchor. */
  staticBody: string
  /** What the cut removes from the injected response; undefined when its anchors were not found. */
  span: string | undefined
  /**
   * Whether $.store held a key for this session id: a resumed or reloaded
   * conversation, whose snapshot this start does not refresh (session.start
   * carries no resume marker; the stored entry is the signal). True as well
   * when that key's re-login was refused and a new one minted.
   */
  reused: boolean
}

export function createManual(): Manual {
  let edges: ManualEdges | undefined
  /** Whether this is a ws project; undefined until told. Not conversation state: the root does not change. */
  let gate: boolean | undefined
  let cache: Cache | undefined
  let pending: Promise<void> | undefined
  /** Which reset the next compose is the first after; undefined once it ran. */
  let awaiting: 'start' | 'landing' | undefined
  let snapshot = false
  /** A compose ran before any session.start, so the conversation's first compose is already past. */
  let composedEarly = false
  /** Bumped at every session start or end: a setup that ran across one fills no cache. */
  let generation = 0
  /** (family, key) -> hash of the text last read: only tells when to read the conversation. */
  const hints = new Map<string, string>()

  const debug = (text: string) => edges?.log(`ws manual: ${text}`, 'debug')

  const reset = () => {
    generation += 1
    cache = undefined
    pending = undefined
    snapshot = false
    awaiting = 'start'
  }

  const relogin = async (e: ManualEdges, server: string, root: string, key: string): Promise<boolean> => {
    try {
      const text = await e.call(server, 'ferrule', { root, format: 'json', relogin_session_key: key })
      if (ferruleKey(text) === key) return true
      debug(`re-login answered another key; minting a new one`)
    } catch (err) {
      debug(`re-login with the stored key failed (${String(err)}); minting a new one`)
    }
    return false
  }

  const setup = async (gen: number) => {
    const e = edges
    if (!e) return
    if (gate === undefined) gate = await e.isWsProject()
    if (!gate) return
    const server = await e.server()
    if (!server) throw new Error('the MCP server was not resolved')
    const [sessionId, root] = await Promise.all([e.sessionId(), e.root()])
    const entry = KEY_STORE_PREFIX + sessionId
    const stored = storedKey(await e.store.get(entry))
    let key = stored !== undefined && (await relogin(e, server, root, stored)) ? stored : undefined
    if (key === undefined) {
      key = ferruleKey(await e.call(server, 'ferrule', { root, format: 'json' }))
      if (key === undefined) throw new Error('ferrule answered no session key')
    }
    await e.store.set(entry, { sessionKey: key, at: await e.now() })
    const [response, staticBody] = await Promise.all([
      e.call(server, 'workflow_manual', { session_key: key }),
      e.call(server, 'playbook_read', { name: 'lead-workflow-manual', session_key: key }),
    ])
    const manual = manualPart(response)
    if (manual === undefined) throw new Error('the keyed workflow_manual response carried no manual')
    if (!staticBody.trim()) throw new Error('the static manual body was empty')
    if (gen !== generation) return
    const cut = cutStaticBody(response, staticBody)
    cache = { key, section: sectionText(key, manual), staticBody, span: cut.found ? cut.span : undefined, reused: stored !== undefined }
    debug(`${key === stored ? 're-logged in with' : 'minted'} ${key}`)
  }

  const begin = () => {
    const gen = generation
    const run: Promise<void> = setup(gen)
      .catch((err) => debug(`setup failed: ${String(err)}`))
      .finally(() => {
        if (pending === run) pending = undefined
      })
    pending = run
  }

  /** Delete entries past retention; the current session's is left to the setup. */
  const prune = async (e: ManualEdges) => {
    const current = KEY_STORE_PREFIX + (await e.sessionId())
    const now = await e.now()
    for (const k of await e.store.keys()) {
      if (!k.startsWith(KEY_STORE_PREFIX) || k === current) continue
      const at = (((await e.store.get(k)) ?? {}) as { at?: unknown }).at
      if (typeof at !== 'number' || now - at > KEY_RETENTION_MS) await e.store.delete(k)
    }
  }

  /** Resolve once `p` settled or `ms` passed, whichever is first. */
  const bounded = (e: ManualEdges, p: Promise<void>, ms: number) =>
    new Promise<void>((resolve) => {
      const timer = e.timer(ms, resolve)
      void p.then(() => {
        timer.cancel()
        resolve()
      })
    })

  /** The text after the state-only cut, or `text` itself; never throws. */
  const stateOnly = async (args: Record<string, unknown>, text: string): Promise<string> => {
    const c = cache
    if (!snapshot || !c) return text
    const cut = cutStaticBody(text, c.staticBody)
    if (cut.found) return c.span !== undefined && cut.span === c.span ? stateOnlyText(cut.text) : text
    if (cut.reason === 'no-body') return text
    debug(`workflow_manual cut missed (${cut.reason}); reading workflow_state`)
    try {
      const server = await edges?.server()
      if (!server || !edges) return text
      return stateOnlyText(await edges.call(server, 'workflow_state', { session_key: args.session_key }))
    } catch (err) {
      debug(`workflow_state fallback failed: ${String(err)}`)
      return text
    }
  }

  const dedupe = async (family: ReadFamily, tool: string, args: Record<string, unknown>, fresh: string): Promise<string> => {
    const key = readKey(family, args)
    const hint = `${family}\n${key}`
    const hash = hashText(fresh)
    const seen = hints.get(hint) === hash
    hints.set(hint, hash)
    if (!seen || !edges) return fresh
    try {
      return isRepeat(await edges.messages(), tool, family, key, fresh) ? pointerText(family, fresh) : fresh
    } catch (err) {
      debug(`conversation read failed: ${String(err)}`)
      return fresh
    }
  }

  const landed = (trigger: SessionCompactTrigger) => {
    if (landingRefreshes(trigger)) awaiting = 'landing'
  }

  return {
    start: (bound) => {
      edges = bound
      reset()
      if (composedEarly) awaiting = undefined
      begin()
      prune(bound).catch((err) => debug(`key entry prune failed: ${String(err)}`))
    },

    end: (reason) => {
      debug(`session.end ${reason}: the manual and the snapshot flag start over`)
      reset()
    },

    compose: async (traits) => {
      // A measuring render (`/context`) sends nothing: it shows the section
      // when cached, never waits, and leaves the first compose to come.
      if (traits.includes('analysis')) return cache?.section
      const e = edges
      if (!e) {
        composedEarly = true
        return undefined
      }
      const reset = awaiting
      if (reset === undefined) return cache?.section
      awaiting = undefined
      if (gate === false) return undefined
      if (!cache && !pending) begin()
      if (pending) await bounded(e, pending, COMPOSE_WAIT_MS)
      const c = cache
      snapshot = c !== undefined && (reset === 'landing' || !c.reused)
      if (!c) debug('no manual by the first compose; the snapshot goes without it')
      return c?.section
    },

    // `precompute` installs nothing, a veto leaves the conversation as it
    // was, and a subagent's or fork's own compaction is not the main
    // conversation's.
    compacted: (trigger, agentId, res) => {
      if (agentId !== undefined || trigger === 'precompute' || res.skip !== undefined) return
      landed(trigger)
    },

    landed,

    read: async (family, tool, args, text) => {
      try {
        if (gate !== true) return undefined
        const seen = family === 'workflow_manual' ? await stateOnly(args, text) : text
        const out = await dedupe(family, tool, args, seen)
        return out === text ? undefined : out
      } catch (err) {
        debug(`${family} rewrite failed: ${String(err)}`)
        return undefined
      }
    },

    snapshot: () => snapshot,
  }
}

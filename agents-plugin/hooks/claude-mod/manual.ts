import type { SessionCompactResult, SessionCompactTrigger } from 'claude-code'

/**
 * Workflow manual injection, a feature of the Claude mod (behind its knob,
 * and only in a ws project): the mod mints a lead session key for the conversation,
 * puts the keyed workflow manual in the system prompt, answers a main-thread
 * `workflow_manual` call with the session state alone while the system prompt
 * carries the manual, and collapses a repeat `playbook_read` or
 * `workflow_manual` result to a pointer while the earlier identical result is
 * still in the conversation; a playbook body the preload inlined (preload.ts)
 * is an earlier copy too, and the preload asks the same check through
 * `preloaded`. A main-thread `workflow_manual` call with another
 * lead key (a revive), or with the fresh-bootstrap sentinel whose result
 * carries the key ws-mcp minted, is adopted as the conversation's key, and
 * every key the conversation gets is handed to the mailbox push through the
 * key listener.
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

/** The key line; `plugin` is the engine-reported plugin name, which namespaces its revive skill. */
export function keyLine(plugin: string, key: string): string {
  return (
    `Your lead session key for this conversation is \`${key}\`; pass it as \`session_key\`. ` +
    `Continuing another track (for example \`${plugin}:lead-revive\` with a preserved key) is still allowed.`
  )
}

export const MAILBOX_LINE =
  'Mailbox push is bound to this key automatically: arriving mail starts your next turn even when you are idle, so launch no mailbox wait. If a revive switches your key, the push follows it.'

/** The system prompt section: the state pointer, the key line, the mailbox line when the push runs here, then the manual. */
export function sectionText(plugin: string, key: string, manual: string, push = false): string {
  const keyed = push ? `${keyLine(plugin, key)}\n\n${MAILBOX_LINE}` : keyLine(plugin, key)
  return `${STATE_POINTER_LINE}\n\n${keyed}\n\n${manual}`
}

/**
 * ws-mcp's fresh-bootstrap sentinel: a `workflow_manual` call with it (and a
 * root) mints a separate key and returns it under `## Session Key`. A lead
 * calls it when it saw no key, which is also when the system prompt section
 * did not reach it (an organization policy can keep it out), so that minted
 * key is the one the lead holds.
 */
export const FRESH_BOOTSTRAP_SENTINEL = 'obsidian-latch'

/** The key on the line after the first whole `## Session Key` line, or undefined. */
function resultKey(text: string): string | undefined {
  const lines = text.split('\n')
  const at = lines.indexOf(SESSION_KEY_ANCHOR)
  const key = at === -1 ? undefined : lines[at + 1]?.trim()
  return key || undefined
}

/**
 * The key a main-thread `workflow_manual` call leaves the lead holding, when
 * it is to be adopted: an explicit key (a revive), or for the sentinel the key
 * its result `text` carries under `## Session Key` (none without that line),
 * in either case other than the sentinel and the conversation's current key.
 */
export function revivedKeyToAdopt(raw: unknown, current: string | undefined, text = ''): string | undefined {
  if (typeof raw !== 'string') return undefined
  const arg = raw.trim()
  const key = arg === FRESH_BOOTSTRAP_SENTINEL ? resultKey(text) : arg
  if (!key || key === FRESH_BOOTSTRAP_SENTINEL || key === current) return undefined
  return key
}

/**
 * Appended to the adopting `workflow_manual` result after a blank line; with
 * `push` (the mailbox push runs here) it says the push follows, since a lead
 * whose system prompt lacks the mailbox line would otherwise arm a wait.
 */
export function adoptionNotice(plugin: string, previous: string | undefined, current: string, push = false): string {
  return (
    `[${plugin}] Lead session key changed: ${previous ?? '(none)'} -> ${current}. ` +
    `Keep passing ${current} as session_key; this conversation resumes with it, and the system prompt names it after the next /compact or compaction request.` +
    (push ? ` Mailbox push is bound to ${current}: arriving mail starts your next turn even when you are idle, so launch no mailbox wait.` : '')
  )
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

/** What a repeat read returns in place of `text`, its unchanged result, when the earlier copy is a result of the same tool. */
export function pointerText(family: ReadFamily, text: string): string {
  const lead = `The result is unchanged. Continue using the full result of your earlier \`${family}\` call in this conversation`
  return family === 'workflow_manual'
    ? `${lead} as your current session state; call workflow_manual again after you change agenda, todos, or notes. (headings: ${headings(text)})`
    : `${lead}. Do not read it again (headings: ${headings(text)}).`
}

/**
 * What a repeat `playbook_read` returns in place of `text` when the earlier
 * copy is a skill expansion's inlined body: no earlier call exists to name.
 */
export function inlinedPointerText(text: string): string {
  return `The result is unchanged. Its full text is already above in this conversation, inlined when a skill loaded it; continue using that copy. Do not read it again (headings: ${headings(text)}).`
}

/**
 * The line a repeat preload puts ahead of its block's fallback prose. The
 * prose stays because the hook cannot tell a subagent preload from a lead
 * load: a reader that cannot see the main conversation's copy reads the
 * playbook itself.
 */
export function preloadPointerLine(playbook: string): string {
  return `The full \`${playbook}\` procedure is already above in this conversation. If you can see that copy, follow it and skip the read instructions below; they are only for a context that cannot see it.`
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
 * The dedupe key of a read's arguments: `playbook_read` by `name` alone, the
 * playbook being the unit (a `context` that renders a different body is told
 * apart by the body, so it does not split identical ones), `workflow_manual`
 * by `session_key`.
 */
export function readKey(family: ReadFamily, args: Record<string, unknown>): string {
  return family === 'playbook_read' ? stable({ name: args.name }) : stable({ session_key: args.session_key ?? null })
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

/**
 * One message of the conversation in Messages API form, as far as the dedupe
 * reads it. A skill expansion arrives as a user message's `text` block (after
 * the Skill tool's own result when the model invoked it); content may also be
 * a plain string.
 */
export type ApiMessageLike = { role: string; content: string | readonly { type: string; [field: string]: unknown }[] }

function blockText(content: unknown): string | undefined {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return undefined
  const texts = content.flatMap((b) => (b?.type === 'text' && typeof b.text === 'string' ? [b.text as string] : []))
  return texts.length > 0 ? texts.join('\n') : undefined
}

/** Which earlier copy a repeat collapses onto: a result of the same tool, or a body a skill expansion inlined. */
export type Copy = 'result' | 'inlined'

/**
 * The earlier copy a fresh read whose result the model would see as `fresh`
 * collapses onto, or undefined when it is no repeat, judged by the
 * conversation alone: exactly one earlier copy is `fresh` itself and none is
 * a pointer to it. A copy is a result of a call to `tool` with the same key;
 * for `playbook_read` it is also the body inside a user message's own text (a
 * skill expansion the preload inlined), and its pointers include the preload
 * pointer line, so a playbook is one unit across both paths. Playbook bodies
 * compare trimmed, as the preload inlines them. So the read after a pointer
 * returns the full text again, and an earlier copy gone from the conversation
 * (compaction, `/clear`, a rewind) is not a repeat.
 */
export function repeatOf(
  messages: readonly ApiMessageLike[],
  tool: string,
  family: ReadFamily,
  args: Record<string, unknown>,
  fresh: string,
): Copy | undefined {
  const playbook = family === 'playbook_read'
  const body = playbook ? fresh.trim() : fresh
  if (playbook && !body) return undefined
  const key = readKey(family, args)
  const calls = new Set<string>()
  for (const m of messages) {
    if (m.role !== 'assistant' || typeof m.content === 'string') continue
    for (const b of m.content) {
      if (b.type !== 'tool_use' || b.name !== tool || typeof b.id !== 'string') continue
      const input = b.input && typeof b.input === 'object' ? (b.input as Record<string, unknown>) : {}
      if (readKey(family, input) === key) calls.add(b.id)
    }
  }
  const pointers = playbook ? [pointerText(family, fresh), inlinedPointerText(fresh)] : [pointerText(family, fresh)]
  const line = playbook && typeof args.name === 'string' ? preloadPointerLine(args.name) : undefined
  let results = 0
  let inlined = 0
  let pointed = 0
  const ownText = (text: string) => {
    if (!playbook) return
    if (text.includes(body)) inlined += 1
    if (line !== undefined && text.includes(line)) pointed += 1
  }
  for (const m of messages) {
    if (m.role !== 'user') continue
    if (typeof m.content === 'string') {
      ownText(m.content)
      continue
    }
    for (const b of m.content) {
      if (b.type === 'text' && typeof b.text === 'string') {
        ownText(b.text)
        continue
      }
      if (b.type !== 'tool_result' || b.is_error === true || typeof b.tool_use_id !== 'string' || !calls.has(b.tool_use_id)) continue
      const text = blockText(b.content)
      if (text === undefined) continue
      if ((playbook ? text.trim() : text) === body) results += 1
      else if (pointers.includes(text)) pointed += 1
    }
  }
  if (results + inlined !== 1 || pointed !== 0) return undefined
  return results === 1 ? 'result' : 'inlined'
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
 * carries a fresh compose answer). Native auto compaction composes nothing:
 * the requests after it carry the latest non-measuring compose answer given
 * (observed on Claude Code 2.1.302), so its landing leaves the flag as it
 * was, which is safe either way: a set flag means the cache was there for
 * every compose since, a clear one only costs full results.
 */
export function landingRefreshes(trigger: SessionCompactTrigger): boolean {
  return trigger === 'manual' || trigger === 'plugin'
}

export type ManualEdges = {
  /**
   * Whether the session runs in a ws project (`ai-docs/WORKFLOW.md` at the
   * repository root). The mailbox feature reads the same answer to decide its
   * bind tool, so register.ts evaluates it once; a rejection reads as closed.
   */
  isWsProject: () => Promise<boolean>
  /** Whether the mailbox push runs here (its host-command probe passed); resolves once the probe settled. */
  pushAvailable: () => Promise<boolean>
  /**
   * Synchronous: called once per change of the conversation's key, with the
   * key it had (undefined when none). The mailbox push binds to `current`.
   */
  keyChanged: (current: string, previous: string | undefined) => void
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
  /** The engine-reported plugin name: the log lines' prefix, the notice's label, and the key line's revive skill spell it. */
  plugin: string
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
   * The ws-project gate opened after it read closed (register.ts retried a
   * failed knob read): set up now. This conversation's first compose is past,
   * so the section reaches the model only from the next landing on, and the
   * snapshot flag stays false until then. `bound` is a key the model already
   * bound the mailbox push to while the gate read closed: the conversation
   * keeps it (a re-login with it, never a mint, as an adoption's rebuild),
   * so the push, the band and the key entry stay on the key the model holds.
   * Without one the setup mints as at a start.
   */
  opened: (bound?: string) => void
  /**
   * A main-thread ws read result: the text the model should read in place of
   * `text`, or undefined to keep the result as it is. `tool` is its
   * model-facing name. A `workflow_manual` result reaching here (not refused,
   * not an error) may adopt its key. Never throws.
   */
  read: (family: ReadFamily, tool: string, args: Record<string, unknown>, text: string) => Promise<string | undefined>
  /**
   * Whether a preload's `body` of `playbook` repeats a copy still visible in
   * the main conversation, judged as a main-thread read of `tool` (the
   * model-facing `playbook_read`) with that name would be, so a playbook is one
   * dedupe unit across both paths. Records the body as the unit's hint. Never
   * throws; false outside a ws project.
   */
  preloaded: (tool: string, playbook: string, body: string) => Promise<boolean>
  /** Whether the conversation's snapshot carries the section (for tests and logs). */
  snapshot: () => boolean
  /** The conversation's key, or undefined (the playbook preload reads with it; tests and logs). */
  key: () => string | undefined
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
   * when that key's re-login was refused and a new one minted. A forked
   * resume, or a resume of a session that never stored a key, also replays
   * its snapshot yet reads as fresh here; the span guard in stateOnly is what
   * keeps that harmless.
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
  /**
   * Bumped at every session start or end and at every adoption: a setup that
   * ran across one fills no cache, writes no key entry, and tells the key
   * listener nothing.
   */
  let generation = 0
  /**
   * The conversation's key, as the key listener was last told; apart from
   * the cache's key, since an adoption switches it before the rebuild lands.
   */
  let current: string | undefined
  /**
   * The key the model held when the gate opened late (opened's `bound`): a
   * setup re-logs in with it in place of the key entry and never mints over
   * it. Dropped at every session start or end and at an adoption, which
   * moves the conversation to another key.
   */
  let lateBound: string | undefined
  /** (family, key) -> hash of the text last read: only tells when to read the conversation. */
  const hints = new Map<string, string>()

  const debug = (text: string) => edges?.log(`${edges?.plugin ?? 'plugin'} manual: ${text}`, 'debug')

  const reset = () => {
    generation += 1
    cache = undefined
    pending = undefined
    snapshot = false
    awaiting = 'start'
    current = undefined
    lateBound = undefined
  }

  /** Make `key` the conversation's key, telling the listener when it changed. */
  const switchKey = (e: ManualEdges, key: string) => {
    if (key === current) return
    const previous = current
    current = key
    try {
      e.keyChanged(key, previous)
    } catch (err) {
      debug(`key listener failed: ${String(err)}`)
    }
  }

  const relogin = async (e: ManualEdges, server: string, root: string, key: string): Promise<boolean> => {
    try {
      const text = await e.call(server, 'ferrule', { root, format: 'json', relogin_session_key: key })
      if (ferruleKey(text) === key) return true
      debug(`re-login with ${key} answered another key`)
    } catch (err) {
      debug(`re-login with ${key} failed (${String(err)})`)
    }
    return false
  }

  /**
   * Re-log in with the stored key (or `adopted`, an adoption's rebuild, or the
   * key the model held at a late open) or mint one, write the key entry, tell
   * the listener, and fill the cache. Neither an adoption's rebuild nor a late
   * open over a held key mints: a refused re-login there fails the setup, so
   * the push stays on the key the lead holds.
   */
  const setup = async (gen: number, adopted?: string) => {
    const e = edges
    if (!e) return
    if (gate === undefined) {
      const open = await e.isWsProject().catch((err) => {
        debug(`project check failed (${String(err)}); reading as no workflow project`)
        return false
      })
      // An opened() meanwhile keeps the gate open.
      gate ??= open
    }
    if (!gate) return
    const server = await e.server()
    if (!server) throw new Error('the MCP server was not resolved')
    const [sessionId, root] = await Promise.all([e.sessionId(), e.root()])
    const entry = KEY_STORE_PREFIX + sessionId
    const held = adopted ?? lateBound
    const stored = held ?? storedKey(await e.store.get(entry))
    let key = stored !== undefined && (await relogin(e, server, root, stored)) ? stored : undefined
    if (key === undefined) {
      if (held !== undefined) throw new Error(`the re-login with the held key ${held} was refused`)
      key = ferruleKey(await e.call(server, 'ferrule', { root, format: 'json' }))
      if (key === undefined) throw new Error('ferrule answered no session key')
    }
    // An adoption meanwhile owns the entry and the push: write and tell nothing.
    const at = await e.now()
    if (gen !== generation) return
    await e.store.set(entry, { sessionKey: key, at })
    if (gen !== generation) return
    switchKey(e, key)
    const [response, staticBody, push] = await Promise.all([
      e.call(server, 'workflow_manual', { session_key: key }),
      e.call(server, 'playbook_read', { name: 'lead-workflow-manual', session_key: key }),
      e.pushAvailable(),
    ])
    const manual = manualPart(response)
    if (manual === undefined) throw new Error('the keyed workflow_manual response carried no manual')
    if (!staticBody.trim()) throw new Error('the static manual body was empty')
    if (gen !== generation) return
    const cut = cutStaticBody(response, staticBody)
    cache = { key, section: sectionText(e.plugin, key, manual, push), staticBody, span: cut.found ? cut.span : undefined, reused: stored !== undefined }
    debug(`${key === stored ? 're-logged in with' : 'minted'} ${key}`)
  }

  /** Start a setup as `pending`; an adoption's runs after `before` (its key entry rewrite). */
  const begin = (adopted?: string, before?: Promise<void>) => {
    const gen = generation
    const run: Promise<void> = (before ?? Promise.resolve())
      .then(() => setup(gen, adopted))
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

  /**
   * The text after the state-only cut, or `text` itself; never throws. The
   * span guard is load-bearing, not only a drift check: the injected span
   * carries this conversation's key line, so only a call with the key the
   * injected section named is cut. Where the snapshot was replayed without
   * that section (a forked resume reads as fresh), the model never saw the
   * key, so its calls pass through whole.
   */
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

  /**
   * The one hint memory and visibility check both read paths share: a main-thread
   * read here, and a preload through `preloaded`. The conversation is read only
   * when the text matches the unit's hint; never throws.
   */
  const repeat = async (family: ReadFamily, tool: string, args: Record<string, unknown>, fresh: string): Promise<Copy | undefined> => {
    const hint = `${family}\n${readKey(family, args)}`
    const hash = hashText(family === 'playbook_read' ? fresh.trim() : fresh)
    const seen = hints.get(hint) === hash
    hints.set(hint, hash)
    if (!seen || !edges) return undefined
    try {
      return repeatOf(await edges.messages(), tool, family, args, fresh)
    } catch (err) {
      debug(`conversation read failed: ${String(err)}`)
      return undefined
    }
  }

  const dedupe = async (family: ReadFamily, tool: string, args: Record<string, unknown>, fresh: string): Promise<string> => {
    const copy = await repeat(family, tool, args, fresh)
    if (copy === undefined) return fresh
    return copy === 'inlined' ? inlinedPointerText(fresh) : pointerText(family, fresh)
  }

  /**
   * Adopt the key a main-thread `workflow_manual` call revived with (as pi
   * does) or the sentinel's result carries: re-log in with it, make it the
   * conversation's key (the push follows through the listener), rewrite the
   * key entry so a later resume re-logs in with it, and rebuild the cache
   * with it. The snapshot still
   * carries the previous key's section, so the flag goes false until the next
   * refreshing reset. The adoption notice, or undefined; never throws.
   */
  const adopt = async (args: Record<string, unknown>, text: string): Promise<string | undefined> => {
    const e = edges
    if (!e) return undefined
    const revived = revivedKeyToAdopt(args.session_key, current, text)
    if (revived === undefined) return undefined
    try {
      const gen = generation
      const server = await e.server()
      if (!server) return undefined
      if (!(await relogin(e, server, await e.root(), revived))) return undefined
      // A session start or end, or another adoption, came in meanwhile.
      if (gen !== generation) return undefined
      generation += 1
      const mine = generation
      const previous = current
      lateBound = undefined
      cache = undefined
      snapshot = false
      switchKey(e, revived)
      debug(`adopted ${revived} (was ${previous ?? 'none'})`)
      const rewrite = (async () => {
        const [entry, at] = await Promise.all([e.sessionId().then((id) => KEY_STORE_PREFIX + id), e.now()])
        if (mine === generation) await e.store.set(entry, { sessionKey: revived, at })
      })().catch((err) => debug(`key entry rewrite failed: ${String(err)}`))
      // The rebuild is pending from here on, so a first compose meanwhile waits
      // on it instead of starting a setup from the entry not yet rewritten. It
      // re-logs in with the adopted key itself, so a failed rewrite still
      // rebuilds with it (the setup writes the entry again), and it reads as
      // reused: the flag stays false until a landing.
      begin(revived, rewrite)
      const [push] = await Promise.all([e.pushAvailable().catch(() => false), rewrite])
      return adoptionNotice(e.plugin, previous, revived, push)
    } catch (err) {
      debug(`adoption of ${revived} failed: ${String(err)}`)
      return undefined
    }
  }

  const landed = (trigger: SessionCompactTrigger) => {
    if (landingRefreshes(trigger)) awaiting = 'landing'
  }

  return {
    start: (bound) => {
      edges = bound
      reset()
      if (composedEarly) {
        // Only this conversation's first compose is past; a later start's is not.
        awaiting = undefined
        composedEarly = false
      }
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

    opened: (bound) => {
      if (gate === true) return
      gate = true
      lateBound = bound
      debug(`the project gate opened late; setting up now${bound === undefined ? '' : ` with the bound key ${bound}`}`)
      if (edges && !pending && !cache) begin()
    },

    read: async (family, tool, args, text) => {
      try {
        if (gate !== true) return undefined
        // A sentinel call means the lead saw no key, so no manual either: it
        // reads the whole result, never the cut or a workflow_state fallback.
        const sentinel = typeof args.session_key === 'string' && args.session_key.trim() === FRESH_BOOTSTRAP_SENTINEL
        const whole = family !== 'workflow_manual' || sentinel
        const seen = whole ? text : await stateOnly(args, text)
        const out = await dedupe(family, tool, args, seen)
        const notice = family === 'workflow_manual' ? await adopt(args, text) : undefined
        const final = notice === undefined ? out : `${out}\n\n${notice}`
        return final === text ? undefined : final
      } catch (err) {
        debug(`${family} rewrite failed: ${String(err)}`)
        return undefined
      }
    },

    preloaded: async (tool, playbook, body) => (gate === true ? (await repeat('playbook_read', tool, { name: playbook }, body)) !== undefined : false),

    snapshot: () => snapshot,
    key: () => current,
  }
}

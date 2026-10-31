/**
 * Workflow manual delivery, a feature of the Claude mod (behind its knob, and
 * only in a ws project): the mod mints a lead session key for the
 * conversation, and when one of its lead skills is expanded (preload.ts) and
 * no copy of the manual is visible in the conversation, it appends one
 * model-only row carrying the key line, the mailbox line when the push runs
 * here, and the whole keyed `workflow_manual` response. The row is appended
 * once per conversation epoch: compaction, `/clear` and a rewind remove it,
 * and the next lead skill expansion appends it again. It is not appended
 * again when the session state changes inside an epoch, since the lead
 * authored that change and reads its state on demand.
 *
 * A row and not the skill text: Claude Code re-attaches invoked skills'
 * expanded text after compaction, so a key or state inlined there would cross
 * compaction stale, while an appended row is an ordinary message compaction
 * summarizes away. `skill.prompt` also fires for a subagent preload, and the
 * row goes to the main conversation only, so a subagent never receives the
 * key or the state.
 *
 * A repeat `playbook_read` collapses to a pointer while the earlier identical
 * result is still in the conversation; a playbook body the preload inlined is
 * an earlier copy too, and the preload asks the same check through
 * `preloaded`. `workflow_manual` results reach the model as ws-mcp answered
 * them: a main-thread call with another lead key (a revive), or with the
 * fresh-bootstrap sentinel whose result carries the key ws-mcp minted, is
 * adopted as the conversation's key, and only the adoption notice is appended
 * to it. Every key the conversation gets is handed to the mailbox push
 * through the key listener.
 *
 * The engine calls arrive as edges bound in register.ts, so the state machine
 * here is driven and tested without an engine.
 */

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

/** A lead skill expansion waits at most this long for the conversation's key (the hook budget is 10 s). */
export const KEY_WAIT_MS = 5_000

/** The whole line ws-mcp places after the manual body; session state and notes follow it. */
export const SESSION_KEY_ANCHOR = '## Session Key'

/** The key line; `plugin` is the engine-reported plugin name, which namespaces its revive skill. */
export function keyLine(plugin: string, key: string): string {
  return (
    `Your lead session key for this conversation is \`${key}\`; pass it as \`session_key\`. ` +
    `Continuing another track (for example \`${plugin}:lead-revive\` with a preserved key) is still allowed.`
  )
}

export const MAILBOX_LINE =
  'Mailbox push is bound to this key automatically: arriving mail starts your next turn even when you are idle, so launch no mailbox wait. If a revive switches your key, the push follows it.'

/**
 * The epoch row's first line, its marker: the epoch scan matches it as a
 * whole line, so it is spelled only here. `plugin` is the engine-reported name.
 */
export function rowHeader(plugin: string): string {
  return (
    `[${plugin}] Workflow manual for this conversation, with your session state as of this message. ` +
    'It is delivered once until the next compaction or /clear; read your agenda, todos, and notes again on demand with workflow_state, agenda_list, todo_list, or note_query.'
  )
}

/** The epoch row: its header, the key line, the mailbox line when the push runs here, then the whole keyed `workflow_manual` response. */
export function rowText(plugin: string, key: string, response: string, push = false): string {
  const keyed = push ? `${keyLine(plugin, key)}\n\n${MAILBOX_LINE}` : keyLine(plugin, key)
  return `${rowHeader(plugin)}\n\n${keyed}\n\n${response.trimEnd()}`
}

/**
 * ws-mcp's fresh-bootstrap sentinel: a `workflow_manual` call with it (and a
 * root) mints a separate key and returns it under `## Session Key`. A lead
 * calls it when it saw no key (no lead skill delivered the row yet, or an
 * organization policy kept it out), so that minted key is the one the lead
 * holds.
 */
export const FRESH_BOOTSTRAP_SENTINEL = 'obsidian-latch'

/** Whether `text` holds `line` as a whole line. */
function hasLine(text: string, line: string): boolean {
  return text.split('\n').some((l) => l.trimEnd() === line)
}

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
 * that saw no mailbox line would otherwise arm a wait.
 */
export function adoptionNotice(plugin: string, previous: string | undefined, current: string, push = false): string {
  return (
    `[${plugin}] Lead session key changed: ${previous ?? '(none)'} -> ${current}. ` +
    `Keep passing ${current} as session_key; this conversation resumes with it, after compaction too.` +
    (push ? ` Mailbox push is bound to ${current}: arriving mail starts your next turn even when you are idle, so launch no mailbox wait.` : '')
  )
}

export type ReadFamily = 'playbook_read' | 'workflow_manual'

/** The result's `#`-`###` headings joined by `; `, or `(no headings)`. */
export function headings(text: string): string {
  const found = text.match(/^#{1,3}\s+.+$/gm) ?? []
  return found.length > 0 ? found.join('; ') : '(no headings)'
}

/** What a repeat `playbook_read` returns in place of `text`, its unchanged result, when the earlier copy is a result of the same tool. */
export function pointerText(text: string): string {
  return `The result is unchanged. Continue using the full result of your earlier \`playbook_read\` call in this conversation. Do not read it again (headings: ${headings(text)}).`
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
 * The dedupe key of a `playbook_read`'s arguments: its `name` alone, the
 * playbook being the unit (a `context` that renders a different body is told
 * apart by the body, so it does not split identical ones).
 */
export function readKey(args: Record<string, unknown>): string {
  return stable({ name: args.name })
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
 * One message of the conversation in Messages API form, as far as the scans
 * read it. A skill expansion arrives as a user message's `text` block (after
 * the Skill tool's own result when the model invoked it), and so does an
 * appended row; content may also be a plain string.
 */
export type ApiMessageLike = { role: string; content: string | readonly { type: string; [field: string]: unknown }[] }

function blockText(content: unknown): string | undefined {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return undefined
  const texts = content.flatMap((b) => (b?.type === 'text' && typeof b.text === 'string' ? [b.text as string] : []))
  return texts.length > 0 ? texts.join('\n') : undefined
}

/** The ids of the conversation's calls to `tool` whose input `match` accepts. */
function callIds(messages: readonly ApiMessageLike[], tool: string, match: (input: Record<string, unknown>) => boolean): Set<string> {
  const ids = new Set<string>()
  for (const m of messages) {
    if (m.role !== 'assistant' || typeof m.content === 'string') continue
    for (const b of m.content) {
      if (b.type !== 'tool_use' || b.name !== tool || typeof b.id !== 'string') continue
      if (match(b.input && typeof b.input === 'object' ? (b.input as Record<string, unknown>) : {})) ids.add(b.id)
    }
  }
  return ids
}

/**
 * Visit each user message's own text (a plain string or a text block) with
 * `text`, and each non-error result of a call in `calls` with `result`.
 */
function scanUser(
  messages: readonly ApiMessageLike[],
  calls: ReadonlySet<string>,
  text: (t: string) => void,
  result: (t: string) => void,
): void {
  for (const m of messages) {
    if (m.role !== 'user') continue
    if (typeof m.content === 'string') {
      text(m.content)
      continue
    }
    for (const b of m.content) {
      if (b.type === 'text' && typeof b.text === 'string') {
        text(b.text)
        continue
      }
      if (b.type !== 'tool_result' || b.is_error === true || typeof b.tool_use_id !== 'string' || !calls.has(b.tool_use_id)) continue
      const t = blockText(b.content)
      if (t !== undefined) result(t)
    }
  }
}

/**
 * Whether the conversation holds an epoch copy of the manual: the mod's row
 * (a user text holding `rowHeader(plugin)` as a whole line), or a non-error
 * result of a call to `tool` (the model-facing `workflow_manual`) that
 * delivers the manual, whatever its key: one holding a whole `## Session Key`
 * line, which ws-mcp's fail-loud notice for an unresolvable key lacks.
 */
export function epochCopyVisible(messages: readonly ApiMessageLike[], tool: string, plugin: string): boolean {
  const header = rowHeader(plugin)
  let found = false
  scanUser(
    messages,
    callIds(messages, tool, () => true),
    (t) => {
      if (hasLine(t, header)) found = true
    },
    (t) => {
      if (hasLine(t, SESSION_KEY_ANCHOR)) found = true
    },
  )
  return found
}

/** Which earlier copy a repeat collapses onto: a result of the same tool, or a body a skill expansion inlined. */
export type Copy = 'result' | 'inlined'

/**
 * The earlier copy a fresh `playbook_read` whose result the model would see
 * as `fresh` collapses onto, or undefined when it is no repeat, judged by the
 * conversation alone: exactly one earlier copy is `fresh` itself and none is
 * a pointer to it. A copy is a result of a call to `tool` with the same name,
 * or the body inside a user message's own text (a skill expansion the preload
 * inlined), and its pointers include the preload pointer line, so a playbook
 * is one unit across both paths. Bodies compare trimmed, as the preload
 * inlines them. So the read after a pointer returns the full text again, and
 * an earlier copy gone from the conversation (compaction, `/clear`, a rewind)
 * is not a repeat.
 */
export function repeatOf(messages: readonly ApiMessageLike[], tool: string, args: Record<string, unknown>, fresh: string): Copy | undefined {
  const body = fresh.trim()
  if (!body) return undefined
  const key = readKey(args)
  const calls = callIds(messages, tool, (input) => readKey(input) === key)
  const pointers = [pointerText(fresh), inlinedPointerText(fresh)]
  const line = typeof args.name === 'string' ? preloadPointerLine(args.name) : undefined
  let results = 0
  let inlined = 0
  let pointed = 0
  scanUser(
    messages,
    calls,
    (t) => {
      if (t.includes(body)) inlined += 1
      if (line !== undefined && t.includes(line)) pointed += 1
    },
    (t) => {
      if (t.trim() === body) results += 1
      else if (pointers.includes(t)) pointed += 1
    },
  )
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
  /** Append a model-only user row of `text` to the main conversation; rejects when it is refused. */
  append: (text: string) => Promise<void>
  log: (text: string, to: 'transcript' | 'debug') => void
  /** The engine-reported plugin name: the log lines' prefix, the notice's label, the row's header and the key line's revive skill spell it. */
  plugin: string
}

export type Manual = {
  /** session.start: bind the edges, start the conversation over, prune stale key entries, and begin the setup. */
  start: (edges: ManualEdges) => void
  /** session.end: drop the conversation's key; the next main-thread turn or lead skill expansion sets up again. */
  end: (reason: string) => void
  /**
   * A turn started (`agentId` set for a subagent's). The first main-thread
   * turn after a session start or end begins a setup when the conversation
   * holds no key and none runs: no session.start follows `/clear`, and a
   * setup that failed at start is retried here once.
   */
  turnStarted: (agentId: string | undefined) => void
  /**
   * The ws-project gate opened after it read closed (register.ts retried a
   * failed knob read): set up now. `bound` is a key the model already bound
   * the mailbox push to while the gate read closed: the conversation keeps it
   * (a re-login with it, never a mint), so the push, the band and the key
   * entry stay on the key the model holds. Without one the setup mints as at
   * a start.
   */
  opened: (bound?: string) => void
  /**
   * The conversation's key for a lead skill expansion: the key held, else
   * the result of a setup begun (or already running) now, waited on at most
   * KEY_WAIT_MS; undefined outside a ws project, on a failed setup, or on
   * timeout. Never rejects.
   */
  ensureKey: () => Promise<string | undefined>
  /**
   * A lead skill's preload block was expanded: unless an epoch copy of the
   * manual is visible in the main conversation, read `workflow_manual` with
   * the conversation's key and append the epoch row. `tool` is the
   * model-facing `workflow_manual` name. A failed conversation read appends;
   * a failed manual read or refused row appends nothing. Never rejects.
   */
  deliver: (tool: string) => Promise<void>
  /**
   * A main-thread ws read result: the text the model should read in place of
   * `text`, or undefined to keep the result as it is. `tool` is its
   * model-facing name. A `playbook_read` result may collapse to a pointer; a
   * `workflow_manual` result reaching here (not refused, not an error) may
   * adopt its key and gain the adoption notice. Never throws.
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
  /** The conversation's key, or undefined (tests and logs). */
  key: () => string | undefined
}

export function createManual(): Manual {
  let edges: ManualEdges | undefined
  /** Whether this is a ws project; undefined until told. Not conversation state: the root does not change. */
  let gate: boolean | undefined
  let pending: Promise<void> | undefined
  /** Whether the next main-thread turn is the first after a session start or end. */
  let firstTurn = true
  /**
   * Bumped at every session start or end and at every adoption: a setup that
   * ran across one writes no key entry and tells the key listener nothing.
   */
  let generation = 0
  /** The conversation's key, as the key listener was last told. */
  let current: string | undefined
  /**
   * The key the model held when the gate opened late (opened's `bound`): a
   * setup re-logs in with it in place of the key entry and never mints over
   * it. Dropped at every session start or end and at an adoption, which
   * moves the conversation to another key.
   */
  let lateBound: string | undefined
  /** Playbook name -> hash of the text last read: only tells when to read the conversation. */
  const hints = new Map<string, string>()

  const debug = (text: string) => edges?.log(`${edges?.plugin ?? 'plugin'} manual: ${text}`, 'debug')

  const reset = () => {
    generation += 1
    pending = undefined
    firstTurn = true
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
   * Re-log in with the stored key (or the key the model held at a late open)
   * or mint one, write the key entry, and tell the listener. A late open over
   * a held key never mints: a refused re-login there fails the setup, so the
   * push stays on the key the lead holds.
   */
  const setup = async (gen: number) => {
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
    const held = lateBound
    const stored = held ?? storedKey(await e.store.get(entry))
    let key = stored !== undefined && (await relogin(e, server, root, stored)) ? stored : undefined
    if (key === undefined) {
      if (held !== undefined) throw new Error(`the re-login with the held key ${held} was refused`)
      key = ferruleKey(await e.call(server, 'ferrule', { root, format: 'json' }))
      if (key === undefined) throw new Error('ferrule answered no session key')
    }
    // An adoption, or a session start or end, meanwhile owns the entry and the push: write and tell nothing.
    const at = await e.now()
    if (gen !== generation) return
    await e.store.set(entry, { sessionKey: key, at })
    if (gen !== generation) return
    switchKey(e, key)
    debug(`${key === stored ? 're-logged in with' : 'minted'} ${key}`)
  }

  /** Start a setup as `pending`. */
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
    const own = KEY_STORE_PREFIX + (await e.sessionId())
    const now = await e.now()
    for (const k of await e.store.keys()) {
      if (!k.startsWith(KEY_STORE_PREFIX) || k === own) continue
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
   * The one hint memory and visibility check both playbook paths share: a
   * main-thread read here, and a preload through `preloaded`. The conversation
   * is read only when the text matches the unit's hint; never throws.
   */
  const repeat = async (tool: string, args: Record<string, unknown>, fresh: string): Promise<Copy | undefined> => {
    const hint = readKey(args)
    const hash = hashText(fresh.trim())
    const seen = hints.get(hint) === hash
    hints.set(hint, hash)
    if (!seen || !edges) return undefined
    try {
      return repeatOf(await edges.messages(), tool, args, fresh)
    } catch (err) {
      debug(`conversation read failed: ${String(err)}`)
      return undefined
    }
  }

  /**
   * Adopt the key a main-thread `workflow_manual` call revived with (as pi
   * does) or the sentinel's result carries: re-log in with it, make it the
   * conversation's key (the push follows through the listener), and rewrite
   * the key entry so a later resume re-logs in with it. The adoption notice,
   * or undefined; never throws.
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
      switchKey(e, revived)
      debug(`adopted ${revived} (was ${previous ?? 'none'})`)
      const rewrite = (async () => {
        const [entry, at] = await Promise.all([e.sessionId().then((id) => KEY_STORE_PREFIX + id), e.now()])
        if (mine === generation) await e.store.set(entry, { sessionKey: revived, at })
      })().catch((err) => debug(`key entry rewrite failed: ${String(err)}`))
      const [push] = await Promise.all([e.pushAvailable().catch(() => false), rewrite])
      return adoptionNotice(e.plugin, previous, revived, push)
    } catch (err) {
      debug(`adoption of ${revived} failed: ${String(err)}`)
      return undefined
    }
  }

  return {
    start: (bound) => {
      edges = bound
      reset()
      begin()
      prune(bound).catch((err) => debug(`key entry prune failed: ${String(err)}`))
    },

    end: (reason) => {
      debug(`session.end ${reason}: the conversation's key starts over`)
      reset()
    },

    turnStarted: (agentId) => {
      if (agentId !== undefined || !firstTurn) return
      firstTurn = false
      if (edges && gate !== false && current === undefined && !pending) begin()
    },

    opened: (bound) => {
      if (gate === true) return
      gate = true
      lateBound = bound
      debug(`the project gate opened late; setting up now${bound === undefined ? '' : ` with the bound key ${bound}`}`)
      if (edges && !pending && current === undefined) begin()
    },

    ensureKey: async () => {
      try {
        const e = edges
        if (!e || gate === false) return current
        if (current === undefined && !pending) begin()
        if (pending) await bounded(e, pending, KEY_WAIT_MS)
        return current
      } catch (err) {
        debug(`key wait failed: ${String(err)}`)
        return undefined
      }
    },

    deliver: async (tool) => {
      const e = edges
      const key = current
      if (!e || gate !== true || key === undefined) return
      try {
        let visible = false
        try {
          visible = epochCopyVisible(await e.messages(), tool, e.plugin)
        } catch (err) {
          debug(`conversation read failed (${String(err)}); appending the manual row`)
        }
        if (visible) return
        const server = await e.server()
        if (!server) throw new Error('the MCP server was not resolved')
        const [response, push] = await Promise.all([e.call(server, 'workflow_manual', { session_key: key }), e.pushAvailable().catch(() => false)])
        if (!hasLine(response, SESSION_KEY_ANCHOR)) throw new Error('the keyed workflow_manual response carried no manual')
        await e.append(rowText(e.plugin, key, response, push))
        debug(`appended the manual row for ${key}`)
      } catch (err) {
        debug(`manual row not appended: ${String(err)}`)
      }
    },

    read: async (family, tool, args, text) => {
      try {
        if (gate !== true) return undefined
        if (family === 'workflow_manual') {
          const notice = await adopt(args, text)
          return notice === undefined ? undefined : `${text}\n\n${notice}`
        }
        const copy = await repeat(tool, args, text)
        if (copy === undefined) return undefined
        return copy === 'inlined' ? inlinedPointerText(text) : pointerText(text)
      } catch (err) {
        debug(`${family} rewrite failed: ${String(err)}`)
        return undefined
      }
    },

    preloaded: async (tool, playbook, body) => (gate === true ? (await repeat(tool, { name: playbook }, body)) !== undefined : false),

    key: () => current,
  }
}

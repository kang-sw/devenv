import type { MailItem, OwnPrompt } from './captions'

/**
 * Pure mailbox-push controller for the Claude Code mod in register.ts; the
 * Claude counterpart of the pi adapter's mailbox waiter.
 *
 * `mailbox wait` only PEEKS (exit 0 = mail present, 3 = timed out, 130 =
 * interrupted, other = error) and serves purely as the block-until-mail
 * signal. Draining is mailbox recv's job, in one of two modes:
 *
 * - `direct`: the controller drains through recv itself and submits exactly
 *   what recv removed as ONE prompt, so a re-arm never re-delivers the same
 *   mail. A refused drain falls back to the notice path; any other failed
 *   drain backs off and retries, since the mail stays queued. Once recv
 *   answered with mail, the loop is that mail's only holder: anything that
 *   keeps it from being submitted (a failed submit, a stop while the drain
 *   was in flight, or a crash before it is handed to the submit path) writes
 *   it to the user-visible `record` instead.
 * - `notice`: the controller never drains. It submits one notice asking the
 *   model to call recv, then suspends: the peek would exit at once over the
 *   still-queued mail, so the loop re-arms only when the model's own recv for
 *   the bound key completes successfully (`recvCompleted`). One arrival
 *   yields one notice, and a declined recv leaves the loop suspended rather
 *   than re-noticing.
 *
 * Either mode's prompt (the batch or the notice) goes by the turn state when
 * it is ready. While the session is idle it is submitted and starts a turn.
 * While a main-thread turn runs it is queued instead and joins that turn:
 * everything queued by the next tool boundary (`toolCompleted`) is appended
 * as ONE row, which the turn's next model request reads, and anything still
 * queued when the turn ends (`turnCompleted`) is submitted as ONE prompt. A
 * queued batch still holds drained mail, so a refused row, a turn-end
 * prompt neither submitted nor appended, or a stop records it as above; a lost notice is retried as a
 * failed notice submit is.
 *
 * Every submit (an idle batch or notice, the turn-end leftovers) that the
 * host rejects or drops falls back to appending the same text as a row: the
 * host drops a plugin's submit after the person stops a turn until they send
 * input, and a row appended while idle reaches the model with their next
 * prompt. Only when that append fails too does the text count as lost.
 *
 * Board messages (the `board` edges, present while the dashboard knob reads
 * on): the messages the user sends from the dashboard page, held in the
 * session's board-answer inbox until the mod drains them. The same wait also
 * wakes on a held one (its board flag), and the drain is an atomic claim, so
 * each message is submitted once. A wake while idle drains and submits at
 * once, as the user's own message. A wake while a main-thread turn runs
 * drains nothing: the message stays held, so the page can still merge into it
 * or cancel it, and the wait re-arms without the flag, since it only peeks
 * and would wake again at once. Every main-thread turn end drains whatever is
 * held, submits it, and re-arms the wait with the flag; a tool boundary never
 * does. The held board message goes ahead of the mail still queued at that
 * turn end: mail is delivered once the board drain and submit settle (or
 * BOARD_FIRST_WAIT_MS passes), so the user's own message starts the next turn
 * and the mail joins it as the mail path delivers it. A board submit the host
 * drops or rejects is appended as a row instead, and only when that fails too
 * is the message recorded for the user. A crash between the drain and the
 * submit loses the message, the same gap mail has.
 *
 * A held message the user promoted to steering is the one exception to
 * holding it for the turn end: every main-thread tool boundary (while board
 * submission is on) claims the steered messages alone (`drainSteer`) and
 * appends them as one user row (`steer`) before the queued mail row, so the
 * turn's next model request reads them; a promoted message may overtake held
 * ones. When the host refuses that row, the claimed texts are carried to the
 * turn end and go out ahead of what the turn-end drain claims, so a steered
 * message is never lost after its claim; a stop records them instead.
 *
 * Every side effect is an injected edge so the loop is testable without a
 * live mailbox or engine.
 */

/** A wait's outcome; `board` and `both` say a held board message woke it, alone or with mail. */
export type WaitOutcome = 'mail' | 'board' | 'both' | 'timeout' | 'stopped' | 'error'

export type DrainMode = 'direct' | 'notice'

export interface MailboxEnvelope {
  from?: string
  reply_to?: string
  content: string
  sent_at?: string
}

/** One drained item: an envelope, or recv output that is not one, delivered verbatim rather than dropped. */
export type DrainedMail = MailboxEnvelope | { raw: string }

/** One drain attempt: what recv removed, a refusal (permission or hook deny), or another failure. */
export type DrainResult =
  | { kind: 'mail'; mail: DrainedMail[] }
  | { kind: 'refused'; reason: string }
  | { kind: 'failed'; reason: string }

/** The board-message side: the session's board-answer inbox. */
export interface BoardEdges {
  /** Whether board submission is on now (the dashboard knob); read at every wait arm and turn end. */
  enabled: () => boolean
  /** Claim every held message of `key`'s inbox, oldest first: their texts. Rejects when the drain failed. */
  drain: (key: string) => Promise<string[]>
  /** Submit the drained messages as one prompt, the user's own; rejects when it did not reach the conversation. */
  submit: (texts: readonly string[]) => Promise<void>
  /** Append the same prompt as a user row instead; rejects when refused. */
  append: (texts: readonly string[]) => Promise<void>
  /** Claim only the held messages of `key`'s inbox promoted to steering, oldest first: their texts. Rejects when the drain failed. */
  drainSteer: (key: string) => Promise<string[]>
  /**
   * Append steered messages into the running turn as one user row, the
   * user's own, and record them in the dashboard conversation; rejects when
   * the row was refused.
   */
  steer: (texts: readonly string[]) => Promise<void>
}

export interface WatcherEdges {
  mode: DrainMode
  /** Prefix for notice and prompt lines (`<plugin> mailbox`). */
  label: string
  /**
   * Run one `mailbox wait` for `key`, adding the binding's named inbox `slug`,
   * and with `board` also waking on a held board message; must resolve
   * `stopped` soon after `signal` aborts.
   */
  wait: (key: string, slug: string | undefined, board: boolean, signal: AbortSignal) => Promise<WaitOutcome>
  /** Drain via mailbox recv (direct mode only). */
  drain: (key: string) => Promise<DrainResult>
  /**
   * Submit one prompt, the given prompts joined, that starts a turn once the
   * session is idle; rejects when it did not reach the conversation. May
   * instead store and submit nothing: `busy` when a main-thread turn started
   * first (the prompts belong to that turn), `stale` when `live` read false.
   */
  submit: (prompts: readonly OwnPrompt[], live: () => boolean) => Promise<'busy' | 'stale' | void>
  /**
   * Whether a main-thread turn runs now: a prompt ready then is queued for
   * that turn's next tool boundary or its end rather than submitted.
   */
  turnRunning: () => boolean
  /**
   * Append one user row, the given prompts joined: a running main-thread
   * turn's next model request reads it, and while idle it rides the person's
   * next prompt. Rejects when refused.
   */
  append: (prompts: readonly OwnPrompt[]) => Promise<void>
  /** The recv tool's name as the model calls it, for prompt text. */
  recvTool: () => string
  /** Resolve after `ms`, or early (resolve, not reject) when `signal` aborts. */
  sleep: (ms: number, signal: AbortSignal) => Promise<void>
  /** Show the user (never the model) one line, once: only state changes reach it, never every re-arm. */
  notice: (text: string) => void
  /** Show the user (never the model) drained mail that could not be submitted. */
  record: (text: string) => void
  log: (text: string) => void
  /** Board messages; absent, the wait never carries the board flag and nothing is drained. */
  board?: BoardEdges
}

export type BindResult = 'bound' | 'unchanged' | 'rebound'

export const BACKOFF_MIN_MS = 1_000
export const BACKOFF_MAX_MS = 60_000
export const EMPTY_DRAIN_BACKOFF_MS = 2_000
/** At a turn end, the longest the queued mail waits for the held board message to go first. */
export const BOARD_FIRST_WAIT_MS = 5_000

export function nextBackoff(prev: number): number {
  return prev <= 0 ? BACKOFF_MIN_MS : Math.min(prev * 2, BACKOFF_MAX_MS)
}

/** Map a finished `mailbox wait` child to an outcome (the pi adapter's mapMailboxWaitExit). */
export function mapWaitExit(code: number | null, aborted: boolean): WaitOutcome {
  if (aborted) return 'stopped'
  if (code === 0) return 'mail'
  if (code === 3) return 'timeout'
  // 130 (external SIGINT/SIGTERM) is deliberately an error, not stopped: a
  // stop we did not cause must not silently end push for the session.
  return 'error'
}

function isEnvelope(c: unknown): c is MailboxEnvelope {
  return !!c && typeof c === 'object' && typeof (c as { content?: unknown }).content === 'string'
}

/**
 * Parse recv's `format: "json"` text: a bare array of envelopes, `null` when
 * empty. Recv has already removed whatever it returned, so only blank text,
 * `null`, or an empty array is an empty drain; text that is not an array, and
 * array elements that are not envelopes, come back verbatim as `raw`.
 */
export function parseRecvText(text: string | undefined): DrainedMail[] {
  const trimmed = text?.trim() ?? ''
  if (trimmed === '' || trimmed === 'null') return []
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return [{ raw: trimmed }]
  }
  if (!Array.isArray(parsed)) return [{ raw: trimmed }]
  return parsed.map((c): DrainedMail => (isEnvelope(c) ? c : { raw: JSON.stringify(c) }))
}

const field = (v: unknown): string => (typeof v === 'string' ? v.trim() : '')

/** One envelope as the pi adapter frames it: `mail from <handle> (<sent_at>):` then the body. */
export function formatEnvelope(m: DrainedMail): string {
  if (!('content' in m)) return `unparsed mailbox recv output:\n${m.raw}`
  const handle = field(m.from) || field(m.reply_to) || 'unknown'
  const stamp = field(m.sent_at)
  return `${stamp ? `mail from ${handle} (${stamp})` : `mail from ${handle}`}:\n${m.content}`
}

/** One prompt for the whole drained batch, never one per envelope. */
export function formatBatch(label: string, recvTool: string, key: string, mail: readonly DrainedMail[]): string {
  const head =
    `[${label}] ${mail.length} message(s) arrived for session_key ${key}. ` +
    `They were already received through ${recvTool}; do not call it again for these.`
  return [head, ...mail.map(formatEnvelope)].join('\n\n')
}

/** The notice-path prompt: the model drains through its own permission flow. */
export function formatNotice(label: string, recvTool: string, key: string): string {
  return `[${label}] Mail is waiting for session_key ${key}. Call ${recvTool} with session_key ${key} to receive it.`
}

/** A drained item as a caption unit reads it: its sender's handle (as formatEnvelope names it) and its body. */
export function mailItems(mail: readonly DrainedMail[]): MailItem[] {
  return mail.map((m) =>
    'content' in m ? { from: field(m.from) || field(m.reply_to) || 'unknown', body: m.content } : { from: 'unparsed recv output', body: m.raw },
  )
}

/**
 * The one-liner a batch shows the person while captions are on: the count and
 * the senders, each once, a peer-supplied handle kept to one printable line.
 */
export function batchLine(label: string, mail: readonly DrainedMail[]): string {
  const senders = [...new Set(mailItems(mail).map((m) => m.from.replace(/[\s\u0000-\u001f\u007f]+/g, ' ')))]
  return `[${label}] ${mail.length} ${mail.length === 1 ? 'message' : 'messages'} from ${senders.join(', ')}`
}

/** The one-liner the notice-path prompt shows the person while captions are on. */
export function noticeLine(label: string): string {
  return `[${label}] mail waiting`
}

/** A drained batch as one of the mod's own prompts: its text, its one-liner, and its mail for a recap. */
export function batchPrompt(label: string, recvTool: string, key: string, mail: readonly DrainedMail[]): OwnPrompt {
  return { text: formatBatch(label, recvTool, key, mail), line: batchLine(label, mail), mail: { tag: `[${label}]`, items: mailItems(mail) } }
}

/** The notice-path prompt as one of the mod's own prompts. */
export function noticePrompt(label: string, recvTool: string, key: string): OwnPrompt {
  return { text: formatNotice(label, recvTool, key), line: noticeLine(label) }
}

/** The label the mailbox's prompts carry in brackets: `<plugin> mailbox`. */
export function mailboxLabel(plugin: string): string {
  return `${plugin} mailbox`
}

export interface Watcher {
  /** Arm the loop for `key`, waking on its reply-id queue and, when given, the named inbox `slug`. */
  bind: (key: string, slug?: string) => BindResult
  stop: (reason: string) => void
  current: () => string | undefined
  /** Whether the loop for the current key waits on the model's recv after a notice. */
  isSuspended: () => boolean
  /** The model's own recv for `key` completed successfully: resume a loop suspended on that key. */
  recvCompleted: (key: string) => void
  /**
   * A model-issued main-thread tool call completed: append the board
   * messages promoted to steering, then everything queued, each as one row
   * before its result returns, so the request that follows the result reads
   * them. Never rejects.
   */
  toolCompleted: () => Promise<void>
  /**
   * The main-thread turn ended: drain and submit the held board messages,
   * then submit whatever mail is still queued as one prompt, else append it.
   * Never throws.
   */
  turnCompleted: () => void
  /** The board switch may have changed: a running wait whose board flag no longer matches is re-armed. */
  boardChanged: () => void
}

/**
 * A prompt queued for the running turn, and what to do should it never reach
 * the model; `noticeFor` marks a notice, withdrawn once the model has
 * received that key's mail on its own.
 */
type Queued = { prompt: OwnPrompt; lost: (why: string) => void; noticeFor?: string }

export function createWatcher(edges: WatcherEdges): Watcher {
  let key: string | undefined
  let slug: string | undefined
  let ctrl: AbortController | undefined
  let suspended: { key: string; resume: () => void } | undefined
  // A stop ends delivery and a rebind does not, but both only abort a loop:
  // a loop compares this count with the one it started under to learn
  // whether a stop came since, even when a rebind had already superseded it.
  let stops = 0
  let lastStop = ''
  /**
   * Prompts waiting for the running turn's next tool boundary or its end.
   * Session-wide, not per loop: a rebind keeps them, a stop gives them up.
   */
  let queued: Queued[] = []
  /**
   * Set when a board message woke the wait while a main-thread turn ran: it
   * stays held until the turn end, and the wait re-arms without the board
   * flag meanwhile. Session-wide like the queue: a rebind keeps it.
   */
  let boardHeld = false
  /** Board drains run one at a time, so a turn end and an idle wake never interleave their submits. */
  let boardChain: Promise<unknown> = Promise.resolve()
  /**
   * Steered board messages claimed at a tool boundary whose row the host
   * refused: the next board flush submits them ahead of what it drains.
   * Session-wide like the queue: a rebind keeps them, a stop records them.
   */
  let steerCarry: string[] = []
  /** The wait now running: its own abort, which re-arms it without ending the loop, and its board flag. */
  let waiting: { ctrl: AbortController; board: boolean } | undefined

  /** Whether board submission is on now. */
  function boardEnabled(): boolean {
    if (!edges.board) return false
    try {
      return edges.board.enabled()
    } catch (err) {
      edges.log(`${edges.label}: reading the board switch failed: ${String(err)}`)
      return false
    }
  }

  /** Whether the next wait carries the board flag: board submission on and nothing held for a turn end. */
  function boardWanted(): boolean {
    return !boardHeld && boardEnabled()
  }

  /** Re-arm the running wait when its board flag no longer matches. */
  function rearmStaleWait(): void {
    if (waiting && waiting.board !== boardWanted()) waiting.ctrl.abort()
  }

  /** Write board messages that will not be submitted where the user can see them, and to the debug log. */
  function keepBoard(why: string, texts: readonly string[]): void {
    const text = `${edges.label}: a message from the dashboard was not submitted (${why}); recorded here instead:\n\n${texts.join('\n\n')}`
    try {
      edges.record(text)
    } catch (err) {
      edges.log(`${edges.label}: recording a dashboard message failed: ${String(err)}`)
    }
    edges.log(text)
  }

  /**
   * Drain `myKey`'s held board messages and submit them as one prompt, after
   * any drain already running; the carried steered messages go first. With
   * `drainHeld` false only the carried ones are submitted. Never rejects:
   * answers `empty` when nothing was held or carried, `failed` when the drain
   * failed (the held messages stay held; carried ones are still submitted),
   * and `done` once claimed messages were submitted, appended, or recorded.
   */
  function flushBoard(myKey: string, drainHeld = true): Promise<'done' | 'empty' | { failed: string }> {
    const at = stops
    const run = boardChain.then(async (): Promise<'done' | 'empty' | { failed: string }> => {
      const board = edges.board
      if (!board) return 'empty'
      const carried = steerCarry
      steerCarry = []
      let texts: string[] = []
      let failed: string | undefined
      if (drainHeld) {
        try {
          texts = await board.drain(myKey)
        } catch (err) {
          failed = String(err)
        }
      }
      texts = [...carried, ...texts]
      if (texts.length === 0) return failed === undefined ? 'empty' : { failed }
      if (failed !== undefined) edges.log(`${edges.label}: dashboard message drain failed (${failed}); submitting the ${carried.length} carried steered message(s) alone`)
      // The drain claimed these: from here this is their only holder. A stop
      // since means the conversation they were for is gone.
      if (stops !== at) {
        keepBoard(`the push loop stopped: ${lastStop}`, texts)
        return 'done'
      }
      edges.log(`${edges.label}: drained ${texts.length} dashboard message(s) for ${myKey}; submitting them as the user's`)
      let submitErr: unknown
      try {
        await board.submit(texts)
        return failed === undefined ? 'done' : { failed }
      } catch (err) {
        submitErr = err
      }
      edges.log(`${edges.label}: dashboard message submit failed (${String(submitErr)}); appending it as a row instead`)
      try {
        await board.append(texts)
      } catch (err) {
        keepBoard(`prompt submit failed: ${String(submitErr)}; row append failed: ${String(err)}`, texts)
      }
      return failed === undefined ? 'done' : { failed }
    })
    boardChain = run.catch(() => {})
    return run.catch((err) => ({ failed: String(err) }))
  }

  /**
   * Claim the board messages promoted to steering and append them into the
   * running turn as one row. A refused row carries them to the next board
   * flush (the turn end), so a claimed message is never lost. Never rejects.
   */
  async function steerBoard(): Promise<void> {
    const board = edges.board
    const myKey = key
    if (!board || myKey === undefined || !boardEnabled()) return
    const at = stops
    let texts: string[]
    try {
      texts = await board.drainSteer(myKey)
    } catch (err) {
      edges.log(`${edges.label}: steered dashboard message drain failed for ${myKey}: ${String(err)}; they stay held`)
      return
    }
    if (texts.length === 0) return
    // The drain claimed these: from here this is their only holder.
    if (stops !== at) {
      keepBoard(`the push loop stopped: ${lastStop}`, texts)
      return
    }
    edges.log(`${edges.label}: appending ${texts.length} steered dashboard message(s) to the running turn as one row`)
    try {
      await board.steer(texts)
    } catch (err) {
      if (stops !== at) {
        keepBoard(`the push loop stopped: ${lastStop}; steer row: ${String(err)}`, texts)
        return
      }
      edges.log(`${edges.label}: steered dashboard message row refused (${String(err)}); submitting it at the turn end instead`)
      steerCarry.push(...texts)
      // The turn ended while the row was pending (an interrupt): its flush
      // has run, so submit the carried messages now.
      if (!edges.turnRunning()) void flushBoard(myKey, false)
    }
  }

  /** Queue `prompt` for the running main-thread turn; false when none runs, and the caller submits it. */
  function steer(prompt: OwnPrompt, lost: (why: string) => void, noticeFor?: string): boolean {
    if (!edges.turnRunning()) return false
    queued.push({ prompt, lost, noticeFor })
    return true
  }

  /** Take everything queued, in order; undefined when nothing is. */
  function takeQueued(): { items: Queued[]; prompts: OwnPrompt[] } | undefined {
    if (queued.length === 0) return undefined
    const items = queued
    queued = []
    return { items, prompts: items.map((q) => q.prompt) }
  }

  /**
   * Submit `items`' prompts, falling back to appending them as a row when the
   * submit is rejected or dropped. A turn that started first takes them into
   * the queue for its next tool boundary or its end, and a turn that ended
   * again by then gets the submit tried anew, so they are never left behind.
   * Never rejects: answers why they did not reach the conversation (a stop
   * since this call, or neither submit nor row), or undefined when they did
   * or were queued.
   */
  async function deliver(items: readonly Queued[]): Promise<string | undefined> {
    const prompts = items.map((q) => q.prompt)
    const at = stops
    let submitErr: unknown
    for (;;) {
      let res: 'busy' | 'stale' | void
      try {
        res = await edges.submit(prompts, () => stops === at)
      } catch (err) {
        submitErr = err
        break
      }
      if (res === 'stale') return `the push loop stopped: ${lastStop}`
      if (res !== 'busy') return undefined
      if (edges.turnRunning()) {
        // Ahead of anything steered in during the settle: mail keeps its arrival order.
        queued.unshift(...items)
        edges.log(`${edges.label}: a turn started before ${items.length} prompt(s) were submitted; queued for it`)
        return undefined
      }
    }
    edges.log(`${edges.label}: prompt submit failed (${String(submitErr)}); appending it as a row instead`)
    try {
      await edges.append(prompts)
      return undefined
    } catch (err) {
      return `prompt submit failed: ${String(submitErr)}; row append failed: ${String(err)}`
    }
  }

  function lose(items: readonly Queued[], why: string): void {
    for (const q of items) {
      try {
        q.lost(why)
      } catch (err) {
        edges.log(`${edges.label}: recording a lost prompt failed: ${String(err)}`)
      }
    }
  }

  /** `resumed` resolves when the model's recv for `myKey` completes, when `signal` aborts, or on `resume`. */
  function suspend(myKey: string, signal: AbortSignal): { resumed: Promise<void>; resume: () => void } {
    let resume!: () => void
    const resumed = new Promise<void>((resolve) => {
      const done = () => {
        if (suspended?.resume === done) suspended = undefined
        signal.removeEventListener('abort', done)
        resolve()
      }
      resume = done
      suspended = { key: myKey, resume: done }
      signal.addEventListener('abort', done, { once: true })
    })
    return { resumed, resume }
  }

  /** `healthy` is called after a wait that timed out with nothing queued: one whole clean cycle, which ends a crash streak. */
  async function loop(myKey: string, mySlug: string | undefined, signal: AbortSignal, healthy: () => void): Promise<void> {
    let backoff = 0
    let emptyStreak = 0
    const stopsAtStart = stops
    const backOff = async (what: string, detail: string) => {
      // Once per failure streak: a persistent fault retries every 60s and would flood the transcript.
      if (backoff === 0) edges.notice(`${edges.label}: ${what}, retrying with backoff`)
      backoff = nextBackoff(backoff)
      edges.log(`${edges.label}: ${what} for ${myKey}: ${detail}; backing off ${backoff}ms`)
      await edges.sleep(backoff, signal)
    }
    /** Deliver one notice and wait for the model's recv; false when the notice could not be delivered. */
    const notify = async (): Promise<boolean> => {
      const { resumed, resume } = suspend(myKey, signal)
      let failure: string | undefined
      const fail = (why: string) => {
        failure ??= why
        resume() // this loop's own suspension, never a rebound loop's
      }
      const notice = noticePrompt(edges.label, edges.recvTool(), myKey)
      if (steer(notice, fail, myKey)) {
        edges.log(`${edges.label}: notice for ${myKey} queued for the running turn`)
      } else {
        const why = await deliver([{ prompt: notice, lost: fail, noticeFor: myKey }])
        if (why !== undefined) fail(why)
      }
      await resumed
      if (failure === undefined) return true
      if (signal.aborted) return false
      await backOff('notice submit failed', failure)
      return false
    }
    /** Write drained mail that will not be submitted where the user can see it, and to the debug log. */
    const keep = (why: string, batch: string) => {
      const text = `${edges.label}: drained mail was not submitted (${why}); recorded here instead:\n\n${batch}`
      // The record goes first, and a throwing one must not cost the log line: it then carries the only copy.
      let recordErr: unknown
      try {
        edges.record(text)
      } catch (err) {
        recordErr = err
      }
      edges.log(text)
      if (recordErr !== undefined) edges.log(`${edges.label}: recording drained mail failed: ${String(recordErr)}; the log line above holds it`)
    }

    let emptyBoardStreak = 0
    while (!signal.aborted) {
      let outcome: WaitOutcome
      const board = boardWanted()
      const own = { ctrl: new AbortController(), board }
      const relay = () => own.ctrl.abort()
      signal.addEventListener('abort', relay, { once: true })
      waiting = own
      try {
        outcome = await edges.wait(myKey, mySlug, board, own.ctrl.signal)
      } catch (err) {
        edges.log(`${edges.label}: wait failed to start: ${String(err)}`)
        outcome = 'error'
      } finally {
        signal.removeEventListener('abort', relay)
        if (waiting === own) waiting = undefined
      }
      if (signal.aborted) return
      if (outcome === 'stopped') {
        // Its own abort re-arms it with the board flag as it now stands.
        if (own.ctrl.signal.aborted) continue
        return
      }
      if (outcome === 'timeout') {
        backoff = 0
        emptyBoardStreak = 0
        healthy()
        continue
      }
      if (outcome === 'error') {
        await backOff('wait error', 'mailbox wait exited abnormally')
        continue
      }
      if (outcome === 'board' || outcome === 'both') {
        if (edges.turnRunning()) {
          // Held for the turn end, where the page can still merge or cancel it.
          boardHeld = true
          edges.log(`${edges.label}: a dashboard message for ${myKey} is held until the running turn ends`)
        } else {
          const flushed = await flushBoard(myKey)
          if (signal.aborted) return
          // Mail that woke the same wait is still handled below; the board
          // message alone backs off, since the peek would wake again at once.
          if (typeof flushed === 'object') {
            if (outcome === 'board') {
              await backOff('dashboard message drain error', flushed.failed)
              continue
            }
            edges.log(`${edges.label}: dashboard message drain failed for ${myKey}: ${flushed.failed}; handling the mail`)
          } else if (flushed === 'empty') {
            if (outcome === 'board') {
              // The peek saw a held message the drain did not get (cancelled,
              // merged, or taken by a turn end's drain): never re-arm at once.
              emptyBoardStreak += 1
              const delay = Math.min(EMPTY_DRAIN_BACKOFF_MS * emptyBoardStreak, BACKOFF_MAX_MS)
              edges.log(`${edges.label}: dashboard wake with empty drain for ${myKey} (x${emptyBoardStreak}); sleeping ${delay}ms`)
              await edges.sleep(delay, signal)
              continue
            }
          } else {
            emptyBoardStreak = 0
            if (outcome === 'board') backoff = 0
          }
        }
        if (outcome === 'board') continue
      }
      if (edges.mode === 'notice') {
        if (await notify()) backoff = 0
        continue
      }
      let drained: DrainResult
      try {
        drained = await edges.drain(myKey)
      } catch (err) {
        drained = { kind: 'failed', reason: String(err) }
      }
      if (signal.aborted && drained.kind !== 'mail') return
      if (drained.kind === 'failed') {
        await backOff('drain error', drained.reason)
        continue
      }
      if (drained.kind === 'refused') {
        edges.log(`${edges.label}: recv refused for ${myKey} (${drained.reason}); asking the model to receive`)
        if (await notify()) backoff = 0
        continue
      }
      backoff = 0
      const mail = drained.mail
      if (mail.length === 0) {
        if (signal.aborted) return
        // wait saw mail but recv drained nothing (raced, or recv cannot reach
        // a queue wait can see): never re-arm instantly, or the peek spins.
        emptyStreak += 1
        const delay = Math.min(EMPTY_DRAIN_BACKOFF_MS * emptyStreak, BACKOFF_MAX_MS)
        edges.log(`${edges.label}: wake with empty drain for ${myKey} (x${emptyStreak}); sleeping ${delay}ms`)
        await edges.sleep(delay, signal)
        continue
      }
      emptyStreak = 0
      // recv already removed this mail, so this loop is its only holder until
      // steer or deliver takes it over. An unexpected throw before that point
      // records it and rethrows into the supervisor: redelivering would likely
      // crash on the same batch again, and a restart alone would lose it.
      let handedOff = false
      let batch: OwnPrompt | undefined
      try {
        batch = batchPrompt(edges.label, edges.recvTool(), myKey, mail)
        const text = batch.text
        // A stop means the conversation this mail was for is gone; a rebind
        // within it still delivers, since recv already removed this mail.
        if (stops !== stopsAtStart) {
          handedOff = true
          keep(`the push loop stopped: ${lastStop}`, text)
          return
        }
        const lost = (why: string) => keep(why, text)
        if (steer(batch, lost)) {
          handedOff = true
          edges.log(`${edges.label}: drained ${mail.length} message(s) for ${myKey}; queued for the running turn`)
          continue
        }
        edges.log(`${edges.label}: drained ${mail.length} message(s) for ${myKey}; submitting one prompt`)
        const why = await deliver([{ prompt: batch, lost }])
        handedOff = true // deliver never rejects once it has submitted, appended, or queued
        if (why !== undefined) lost(why)
      } catch (err) {
        if (!handedOff) {
          try {
            // Plain rendering when formatting is what failed.
            keep(`the push loop crashed: ${String(err)}`, batch?.text ?? mail.map((m) => JSON.stringify(m)).join('\n'))
          } catch {
            // the crash itself is what the supervisor must hear of
          }
        }
        throw err
      }
    }
  }

  /**
   * Run the loop for one binding, and run it again after any crash: push must
   * never end silently while its binding stands. The `signal` is the binding's
   * own, aborted by a stop and by a rebind alike, so a crash in a loop that
   * either superseded is never restarted, and a second loop never runs beside
   * the current one. The wait between restarts backs off like any other
   * failure, and the user hears of one crash streak once.
   */
  async function supervise(myKey: string, mySlug: string | undefined, signal: AbortSignal): Promise<void> {
    let backoff = 0
    edges.notice(`${edges.label}: armed ${myKey}`)
    while (!signal.aborted) {
      try {
        await loop(myKey, mySlug, signal, () => (backoff = 0))
        return
      } catch (err) {
        if (signal.aborted) {
          edges.log(`${edges.label}: loop for ${myKey} crashed after it was superseded or stopped; not restarted: ${String(err)}`)
          return
        }
        edges.log(`${edges.label}: loop crashed for ${myKey}: ${String(err)}; restarting in ${nextBackoff(backoff)}ms`)
        // Once per crash streak: a persistent fault restarts every 60s and would flood the transcript.
        if (backoff === 0) {
          try {
            edges.notice(`${edges.label}: push loop crashed, restarting with backoff`)
          } catch {
            // a failing notice edge must not end the restart
          }
        }
        backoff = nextBackoff(backoff)
        await edges.sleep(backoff, signal)
      }
    }
  }

  return {
    bind(next, nextSlug) {
      if (next === key && nextSlug === slug && ctrl && !ctrl.signal.aborted) return 'unchanged'
      const had = key !== undefined
      ctrl?.abort()
      key = next
      slug = nextSlug
      const mine = new AbortController()
      ctrl = mine
      void supervise(next, nextSlug, mine.signal).catch((err) => edges.log(`${edges.label}: push supervisor failed: ${String(err)}`))
      return had ? 'rebound' : 'bound'
    },
    stop(reason) {
      const left = takeQueued()
      if (left) lose(left.items, `the push loop stopped: ${reason}`)
      if (steerCarry.length > 0) keepBoard(`the push loop stopped: ${reason}`, steerCarry.splice(0))
      boardHeld = false
      if (!ctrl) return
      stops += 1
      lastStop = reason
      ctrl.abort()
      ctrl = undefined
      key = undefined
      slug = undefined
      edges.log(`${edges.label}: stopped (${reason})`)
    },
    current: () => key,
    isSuspended: () => suspended !== undefined && suspended.key === key,
    recvCompleted(doneKey) {
      // A notice still queued for that key would only ask for mail already received.
      queued = queued.filter((q) => q.noticeFor !== doneKey)
      if (suspended && suspended.key === doneKey && doneKey === key) suspended.resume()
    },
    async toolCompleted() {
      try {
        await steerBoard()
      } catch (err) {
        edges.log(`${edges.label}: steering dashboard messages failed: ${String(err)}`)
      }
      const taken = takeQueued()
      if (!taken) return
      edges.log(`${edges.label}: appending ${taken.items.length} queued prompt(s) to the running turn as one row`)
      try {
        await edges.append(taken.prompts)
      } catch (err) {
        lose(taken.items, `row append failed: ${String(err)}`)
      }
    },
    turnCompleted() {
      const taken = takeQueued()
      const at = stops
      const sendMail = () => {
        if (!taken) return
        // A stop while the mail waited on the board drain took its conversation.
        if (stops !== at) return lose(taken.items, `the push loop stopped: ${lastStop}`)
        edges.log(`${edges.label}: submitting ${taken.items.length} prompt(s) still queued at the turn end as one prompt`)
        void deliver(taken.items).then((why) => {
          if (why !== undefined) lose(taken.items, why)
        })
      }
      const myKey = key
      const drainHeld = boardHeld || boardWanted()
      if (myKey === undefined || !edges.board || !(drainHeld || steerCarry.length > 0)) {
        boardHeld = false
        sendMail()
        return
      }
      // The held board message goes first; the mail waits for it a bounded time.
      const flushed = flushBoard(myKey, drainHeld).then((res) => {
        if (typeof res === 'object') edges.log(`${edges.label}: turn-end dashboard drain failed (${res.failed}); the messages stay held`)
        // Cleared only now, so the wait does not wake on what this drain is about to claim.
        boardHeld = false
        rearmStaleWait()
      })
      if (!taken) return
      const timer = new AbortController()
      void Promise.race([flushed, edges.sleep(BOARD_FIRST_WAIT_MS, timer.signal)])
        .catch((err) => edges.log(`${edges.label}: waiting on the turn-end dashboard drain failed: ${String(err)}`))
        .then(() => {
          timer.abort()
          sendMail()
        })
    },
    boardChanged() {
      rearmStaleWait()
    },
  }
}

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
 *   keeps it from being submitted (a failed submit, or a stop while the drain
 *   was in flight) writes it to the user-visible `record` instead.
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
 * Every side effect is an injected edge so the loop is testable without a
 * live mailbox or engine.
 */

export type WaitOutcome = 'mail' | 'timeout' | 'stopped' | 'error'

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

export interface WatcherEdges {
  mode: DrainMode
  /** Prefix for notice and prompt lines (`<plugin> mailbox`). */
  label: string
  /** Run one `mailbox wait` for `key`, adding the binding's named inbox `slug`; must resolve `stopped` soon after `signal` aborts. */
  wait: (key: string, slug: string | undefined, signal: AbortSignal) => Promise<WaitOutcome>
  /** Drain via mailbox recv (direct mode only). */
  drain: (key: string) => Promise<DrainResult>
  /**
   * Submit one prompt, the given prompts joined, that starts a turn once the
   * session is idle; rejects when it did not reach the conversation.
   */
  submit: (prompts: readonly OwnPrompt[]) => Promise<void>
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
}

export type BindResult = 'bound' | 'unchanged' | 'rebound'

export const BACKOFF_MIN_MS = 1_000
export const BACKOFF_MAX_MS = 60_000
export const EMPTY_DRAIN_BACKOFF_MS = 2_000

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
   * A model-issued main-thread tool call completed: append everything queued
   * as one row before its result returns, so the request that follows the
   * result reads it. Never rejects.
   */
  toolCompleted: () => Promise<void>
  /** The main-thread turn ended: submit whatever is still queued as one prompt, else append it. Never throws. */
  turnCompleted: () => void
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
   * Submit `prompts`, falling back to appending them as a row when the submit
   * is rejected or dropped. Never rejects: answers why neither reached the
   * conversation, or undefined when one did.
   */
  async function deliver(prompts: readonly OwnPrompt[]): Promise<string | undefined> {
    let submitErr: unknown
    try {
      await edges.submit(prompts)
      return undefined
    } catch (err) {
      submitErr = err
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

  async function loop(myKey: string, mySlug: string | undefined, signal: AbortSignal): Promise<void> {
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
        const why = await deliver([notice])
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
      edges.log(text)
      edges.record(text)
    }

    edges.notice(`${edges.label}: armed ${myKey}`)
    while (!signal.aborted) {
      let outcome: WaitOutcome
      try {
        outcome = await edges.wait(myKey, mySlug, signal)
      } catch (err) {
        edges.log(`${edges.label}: wait failed to start: ${String(err)}`)
        outcome = 'error'
      }
      if (signal.aborted || outcome === 'stopped') return
      if (outcome === 'timeout') {
        backoff = 0
        continue
      }
      if (outcome === 'error') {
        await backOff('wait error', 'mailbox wait exited abnormally')
        continue
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
      const batch = batchPrompt(edges.label, edges.recvTool(), myKey, mail)
      // A stop means the conversation this mail was for is gone; a rebind
      // within it still delivers, since recv already removed this mail.
      if (stops !== stopsAtStart) {
        keep(`the push loop stopped: ${lastStop}`, batch.text)
        return
      }
      if (steer(batch, (why) => keep(why, batch.text))) {
        edges.log(`${edges.label}: drained ${mail.length} message(s) for ${myKey}; queued for the running turn`)
        continue
      }
      edges.log(`${edges.label}: drained ${mail.length} message(s) for ${myKey}; submitting one prompt`)
      const why = await deliver([batch])
      if (why !== undefined) keep(why, batch.text)
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
      void loop(next, nextSlug, mine.signal).catch((err) => edges.log(`${edges.label}: loop crashed: ${String(err)}`))
      return had ? 'rebound' : 'bound'
    },
    stop(reason) {
      const left = takeQueued()
      if (left) lose(left.items, `the push loop stopped: ${reason}`)
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
      if (!taken) return
      edges.log(`${edges.label}: submitting ${taken.items.length} prompt(s) still queued at the turn end as one prompt`)
      void deliver(taken.prompts).then((why) => {
        if (why !== undefined) lose(taken.items, why)
      })
    },
  }
}

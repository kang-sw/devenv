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
  /** Submit one prompt that starts a turn once the session is idle. */
  submit: (text: string) => Promise<void>
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

export interface Watcher {
  /** Arm the loop for `key`, waking on its reply-id queue and, when given, the named inbox `slug`. */
  bind: (key: string, slug?: string) => BindResult
  stop: (reason: string) => void
  current: () => string | undefined
  /** Whether the loop for the current key waits on the model's recv after a notice. */
  isSuspended: () => boolean
  /** The model's own recv for `key` completed successfully: resume a loop suspended on that key. */
  recvCompleted: (key: string) => void
}

/**
 * Why a loop's signal aborted: a stop ends delivery, a rebind does not. Both
 * end the loop, but recv may already have removed mail the loop now holds.
 */
type AbortCause = { kind: 'stop'; reason: string } | { kind: 'rebind' }

function stopCause(signal: AbortSignal): string | undefined {
  const cause = signal.reason as AbortCause | undefined
  return cause?.kind === 'stop' ? cause.reason : undefined
}

export function createWatcher(edges: WatcherEdges): Watcher {
  let key: string | undefined
  let slug: string | undefined
  let ctrl: AbortController | undefined
  let suspended: { key: string; resume: () => void } | undefined

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
    const backOff = async (what: string, detail: string) => {
      // Once per failure streak: a persistent fault retries every 60s and would flood the transcript.
      if (backoff === 0) edges.notice(`${edges.label}: ${what}, retrying with backoff`)
      backoff = nextBackoff(backoff)
      edges.log(`${edges.label}: ${what} for ${myKey}: ${detail}; backing off ${backoff}ms`)
      await edges.sleep(backoff, signal)
    }
    /** Submit one notice and wait for the model's recv; false when the notice could not be submitted. */
    const notify = async (): Promise<boolean> => {
      const { resumed, resume } = suspend(myKey, signal)
      try {
        await edges.submit(formatNotice(edges.label, edges.recvTool(), myKey))
      } catch (err) {
        resume() // this loop's own suspension, never a rebound loop's
        await resumed
        if (signal.aborted) return false
        await backOff('notice submit failed', String(err))
        return false
      }
      await resumed
      return true
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
      const batch = formatBatch(edges.label, edges.recvTool(), myKey, mail)
      // A stop means the conversation this mail was for is gone; a rebind
      // within it still delivers, since recv already removed this mail.
      const stopped = stopCause(signal)
      if (stopped !== undefined) {
        keep(`the push loop stopped: ${stopped}`, batch)
        return
      }
      edges.log(`${edges.label}: drained ${mail.length} message(s) for ${myKey}; submitting one prompt`)
      try {
        await edges.submit(batch)
      } catch (err) {
        keep(`prompt submit failed: ${String(err)}`, batch)
      }
    }
  }

  return {
    bind(next, nextSlug) {
      if (next === key && nextSlug === slug && ctrl && !ctrl.signal.aborted) return 'unchanged'
      const had = key !== undefined
      ctrl?.abort({ kind: 'rebind' } satisfies AbortCause)
      key = next
      slug = nextSlug
      const mine = new AbortController()
      ctrl = mine
      void loop(next, nextSlug, mine.signal).catch((err) => edges.log(`${edges.label}: loop crashed: ${String(err)}`))
      return had ? 'rebound' : 'bound'
    },
    stop(reason) {
      if (!ctrl) return
      ctrl.abort({ kind: 'stop', reason } satisfies AbortCause)
      ctrl = undefined
      key = undefined
      slug = undefined
      edges.log(`${edges.label}: stopped (${reason})`)
    },
    current: () => key,
    isSuspended: () => suspended !== undefined && suspended.key === key,
    recvCompleted(doneKey) {
      if (suspended && suspended.key === doneKey && doneKey === key) suspended.resume()
    },
  }
}

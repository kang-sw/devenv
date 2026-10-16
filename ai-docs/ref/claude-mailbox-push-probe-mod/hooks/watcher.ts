/**
 * Pure mailbox-push controller, mirroring agents-plugin-pi/src/mailbox-waiter.ts.
 *
 * `mailbox wait` only PEEKS (exit 0 = mail present, 3 = timed out, 130 =
 * interrupted, other = error) and is used purely as the block-until-mail
 * signal. The mailbox recv tool is the authoritative DRAIN: on a wake the
 * controller drains through recv and submits exactly what recv removed as ONE
 * prompt, so a re-arm never re-delivers the same mail (no hot loop, no double
 * delivery). An empty drain after a mail wake backs off instead of spinning.
 *
 * Every side effect is an injected edge so the loop is testable without a
 * live mailbox or engine.
 */

export type WaitOutcome = 'mail' | 'timeout' | 'stopped' | 'error'

export interface MailboxEnvelope {
  from?: string
  reply_to?: string
  content: string
  sent_at?: string
}

export interface WatcherEdges {
  /** Run one `mailbox wait` for `key`; must resolve `stopped` soon after `signal` aborts. */
  wait: (key: string, signal: AbortSignal) => Promise<WaitOutcome>
  /** Drain via mailbox recv; throws on a tool-level error. */
  drain: (key: string) => Promise<MailboxEnvelope[]>
  /** Submit one prompt that wakes the idle session. */
  submit: (text: string) => Promise<void>
  /** Resolve after `ms`, or early (resolve, not reject) when `signal` aborts. */
  sleep: (ms: number, signal: AbortSignal) => Promise<void>
  status: (text: string | undefined) => void
  log: (text: string) => void
}

export type BindResult = 'bound' | 'unchanged' | 'rebound'

export const BACKOFF_MIN_MS = 1_000
export const BACKOFF_MAX_MS = 60_000
export const EMPTY_DRAIN_BACKOFF_MS = 2_000

export function nextBackoff(prev: number): number {
  return prev <= 0 ? BACKOFF_MIN_MS : Math.min(prev * 2, BACKOFF_MAX_MS)
}

/** Map a finished `mailbox wait` child to an outcome (pi's mapMailboxWaitExit). */
export function mapWaitExit(code: number | null, aborted: boolean): WaitOutcome {
  if (aborted) return 'stopped'
  if (code === 0) return 'mail'
  if (code === 3) return 'timeout'
  // 130 (external SIGINT/SIGTERM) is deliberately an error, not stopped: a
  // stop we did not cause must not silently end push for the session.
  return 'error'
}

/** Parse recv's `format: "json"` text (a bare array of envelopes); malformed -> []. */
export function parseRecvText(text: string | undefined): MailboxEnvelope[] {
  if (!text) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  return parsed.filter(
    (c): c is MailboxEnvelope =>
      !!c && typeof c === 'object' && typeof (c as { content?: unknown }).content === 'string',
  )
}

/** One prompt for the whole drained batch, never one per envelope. */
export function formatBatch(key: string, mail: readonly MailboxEnvelope[]): string {
  const lines = [
    `[ws mailbox push] ${mail.length} message(s) arrived for session_key ${key}.`,
    'They were already drained by mailbox.recv (do not call recv for these). Reply with mailbox.send to the handle shown.',
  ]
  mail.forEach((m, i) => {
    const handle = m.from ?? m.reply_to ?? '(unknown)'
    lines.push('', `--- ${i + 1}/${mail.length} from ${handle}${m.sent_at ? ` at ${m.sent_at}` : ''} ---`, m.content)
  })
  return lines.join('\n')
}

export interface Watcher {
  bind: (key: string) => BindResult
  stop: (reason: string) => void
  current: () => string | undefined
}

export function createWatcher(edges: WatcherEdges): Watcher {
  let key: string | undefined
  let ctrl: AbortController | undefined

  async function loop(myKey: string, signal: AbortSignal): Promise<void> {
    let backoff = 0
    let emptyStreak = 0
    while (!signal.aborted) {
      edges.status(`ws mailbox: armed ${myKey}`)
      let outcome: WaitOutcome
      try {
        outcome = await edges.wait(myKey, signal)
      } catch (err) {
        edges.log(`ws mailbox: wait failed to start: ${String(err)}`)
        outcome = 'error'
      }
      if (signal.aborted || outcome === 'stopped') return
      if (outcome === 'timeout') {
        backoff = 0
        continue
      }
      if (outcome === 'error') {
        backoff = nextBackoff(backoff)
        edges.status(`ws mailbox: wait error, retry in ${backoff / 1000}s`)
        edges.log(`ws mailbox: wait error for ${myKey}; backing off ${backoff}ms`)
        await edges.sleep(backoff, signal)
        continue
      }
      let mail: MailboxEnvelope[]
      try {
        mail = await edges.drain(myKey)
      } catch (err) {
        backoff = nextBackoff(backoff)
        edges.status(`ws mailbox: drain error, retry in ${backoff / 1000}s`)
        edges.log(`ws mailbox: recv failed for ${myKey}: ${String(err)}; backing off ${backoff}ms`)
        await edges.sleep(backoff, signal)
        continue
      }
      backoff = 0
      if (mail.length === 0) {
        // wait saw mail but recv drained nothing (raced, or recv cannot reach
        // a queue wait can see): never re-arm instantly, or the peek spins.
        emptyStreak += 1
        const delay = Math.min(EMPTY_DRAIN_BACKOFF_MS * emptyStreak, BACKOFF_MAX_MS)
        edges.log(`ws mailbox: wake with empty drain for ${myKey} (x${emptyStreak}); sleeping ${delay}ms`)
        await edges.sleep(delay, signal)
        continue
      }
      emptyStreak = 0
      // Recv already removed this mail: deliver it even if a rebind raced in.
      edges.log(`ws mailbox: drained ${mail.length} message(s) for ${myKey}; submitting one prompt`)
      try {
        await edges.submit(formatBatch(myKey, mail))
      } catch (err) {
        edges.log(`ws mailbox: prompt.submit failed (mail already drained): ${String(err)}`)
      }
    }
  }

  return {
    bind(next) {
      if (next === key && ctrl && !ctrl.signal.aborted) return 'unchanged'
      const had = key !== undefined
      ctrl?.abort()
      key = next
      const mine = new AbortController()
      ctrl = mine
      void loop(next, mine.signal).catch((err) => edges.log(`ws mailbox: loop crashed: ${String(err)}`))
      return had ? 'rebound' : 'bound'
    },
    stop(reason) {
      if (!ctrl) return
      ctrl.abort()
      ctrl = undefined
      key = undefined
      edges.status(undefined)
      edges.log(`ws mailbox: stopped (${reason})`)
    },
    current: () => key,
  }
}

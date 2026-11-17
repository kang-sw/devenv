import type { OwnPrompt } from './captions'
import type { InboxCommand, InboxItem } from './dashboard'
import type { DrainMode, DrainResult, WaitOutcome, WatcherEdges } from './watcher'

type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void }
export function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}
export const tick = async (n = 30) => {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

/** Fake edges: each wait is a deferred the test resolves; drains come from a queue. */
/** Prompts joined as the watcher's edges join them for one turn. */
const joined = (prompts: readonly OwnPrompt[]) => prompts.map((p) => p.text).join('\n\n')

export function harness(mode: DrainMode) {
  const waits: { key: string; slug: string | undefined; board: boolean; interrupt: boolean; signal: AbortSignal; d: Deferred<WaitOutcome> }[] = []
  const drains: string[] = []
  const drainQueue: DrainResult[] = []
  const submitted: string[] = []
  const submitFailures: Error[] = []
  const sleeps: { ms: number; d: Deferred<void> }[] = []
  const notices: string[] = []
  const records: string[] = []
  const logs: string[] = []
  /** When set, the next drain waits on it instead of answering from the queue at once. */
  const heldDrains: Deferred<DrainResult>[] = []
  /** When set, the next submit waits on it; rejecting it fails that submit. */
  const heldSubmits: { d: Deferred<Error | undefined>; text: string }[] = []
  let holdSubmits = 0
  /** Whether a main-thread turn runs, as the turn hooks would report it. */
  const turn = { running: false }
  const appended: string[] = []
  /** Every submit attempted, as the prompts it carried. */
  const submittedPrompts: (readonly OwnPrompt[])[] = []
  /** The next submits' answers with captions on: busy (a turn started during the settle) or stale; run as the submit answers. */
  const submitAnswers: (() => 'busy' | 'stale' | Promise<'busy' | 'stale'>)[] = []
  /** Each submit's live check, to read later as the settle's end would. */
  const lives: (() => boolean)[] = []
  const appendFailures: Error[] = []
  /**
   * The board side: the held inbox (one text per held entry), its switch,
   * and what reached the conversation. `order` records every submit, mail
   * and board alike, as `mail` or `board`, in the order they were made.
   */
  const board = {
    on: false,
    inbox: [] as InboxItem[],
    drains: [] as string[],
    drainFailures: [] as Error[],
    /** When set, the next board drain waits on it. */
    heldDrains: [] as Deferred<void>[],
    /** When set, the next board submit waits on it. */
    heldSubmits: [] as Deferred<void>[],
    submitFailures: [] as Error[],
    appendFailures: [] as Error[],
    submitted: [] as (readonly string[])[],
    /** Each board submit's count of leading texts already in an unread steer row. */
    shown: [] as number[],
    appended: [] as (readonly string[])[],
    /** Held entries promoted to steering: the steer-only drain takes these alone, the full drain takes them too. */
    steeredInbox: [] as string[],
    steerDrains: [] as string[],
    steerDrainFailures: [] as Error[],
    /** When set, the next steered drain waits on it before it claims. */
    heldSteerDrains: [] as Deferred<void>[],
    steerFailures: [] as Error[],
    /** When set, the next steer row waits on it. */
    heldSteers: [] as Deferred<void>[],
    steered: [] as (readonly string[])[],
    /** Held interrupt entries, as their creation times: the interrupt drain takes these alone. */
    interrupts: [] as number[],
    interruptDrains: [] as string[],
    interruptDrainFailures: [] as Error[],
    /** When set, the next interrupt drain waits on it before it claims. */
    heldInterruptDrains: [] as Deferred<void>[],
    /** When the running turn started, on the interrupt entries' clock. */
    turnStartedAt: 0,
    /** How many aborts were asked for. */
    aborts: 0,
    abortFailures: [] as Error[],
    /** Runs inside the abort call, before it settles: how a turn end that beats the abort is staged. */
    onAbort: undefined as (() => void) | undefined,
    /** Every command run asked for, as its typed line. */
    runs: [] as string[],
    /** When set, the next command run waits on it; an Error rejects that run. */
    heldRuns: [] as Deferred<Error | undefined>[],
  }
  const order: string[] = []
  const edges: WatcherEdges = {
    mode,
    label: 'pkg mailbox',
    wait: (key, slug, flags, signal) => {
      const d = deferred<WaitOutcome>()
      signal.addEventListener('abort', () => d.resolve('stopped'), { once: true })
      waits.push({ key, slug, board: flags.board, interrupt: flags.interrupt, signal, d })
      return d.promise
    },
    drain: async (key) => {
      drains.push(key)
      const held = heldDrains.shift()
      if (held) return held.promise
      return drainQueue.shift() ?? { kind: 'mail', mail: [] }
    },
    submit: async (prompts, live) => {
      const text = joined(prompts)
      submittedPrompts.push(prompts)
      lives.push(live)
      const answer = submitAnswers.shift()
      if (answer) return answer()
      if (holdSubmits > 0) {
        holdSubmits -= 1
        const d = deferred<Error | undefined>()
        heldSubmits.push({ d, text })
        const failure = await d.promise
        if (failure) throw failure
        submitted.push(text)
        return
      }
      const failure = submitFailures.shift()
      if (failure) throw failure
      submitted.push(text)
      order.push('mail')
    },
    turnRunning: () => turn.running,
    append: async (prompts) => {
      const text = joined(prompts)
      const failure = appendFailures.shift()
      if (failure) throw failure
      appended.push(text)
    },
    recvTool: () => 'mcp__plugin_pkg_pkg__mailbox_recv',
    sleep: (ms, signal) => {
      const d = deferred<void>()
      signal.addEventListener('abort', () => d.resolve(), { once: true })
      sleeps.push({ ms, d })
      return d.promise
    },
    notice: (text) => notices.push(text),
    record: (text) => records.push(text),
    log: (text) => logs.push(text),
    board: {
      enabled: () => board.on,
      drain: async (key) => {
        board.drains.push(key)
        const held = board.heldDrains.shift()
        if (held) await held.promise
        const failure = board.drainFailures.shift()
        if (failure) throw failure
        // An atomic claim: whatever is held now, once.
        return [...board.inbox.splice(0), ...board.steeredInbox.splice(0)]
      },
      submit: async (texts, shown) => {
        const held = board.heldSubmits.shift()
        if (held) await held.promise
        const failure = board.submitFailures.shift()
        if (failure) throw failure
        board.submitted.push(texts)
        board.shown.push(shown)
        order.push('board')
      },
      append: async (texts) => {
        const failure = board.appendFailures.shift()
        if (failure) throw failure
        board.appended.push(texts)
      },
      drainSteer: async (key) => {
        board.steerDrains.push(key)
        const held = board.heldSteerDrains.shift()
        if (held) await held.promise
        const failure = board.steerDrainFailures.shift()
        if (failure) throw failure
        return board.steeredInbox.splice(0)
      },
      steer: async (texts) => {
        const held = board.heldSteers.shift()
        if (held) await held.promise
        const failure = board.steerFailures.shift()
        if (failure) throw failure
        board.steered.push(texts)
        order.push('steer')
      },
      drainInterrupt: async (key) => {
        board.interruptDrains.push(key)
        const held = board.heldInterruptDrains.shift()
        if (held) await held.promise
        const failure = board.interruptDrainFailures.shift()
        if (failure) throw failure
        return board.interrupts.splice(0)
      },
      mainTurn: () =>
        turn.running
          ? {
              startedAt: board.turnStartedAt,
              abort: async () => {
                board.aborts += 1
                order.push('abort')
                board.onAbort?.()
                const failure = board.abortFailures.shift()
                if (failure) throw failure
              },
            }
          : undefined,
      run: async (item: InboxCommand) => {
        board.runs.push(item.text)
        order.push(`run ${item.text}`)
        const held = board.heldRuns.shift()
        if (!held) return
        const failure = await held.promise
        if (failure) throw failure
      },
    },
  }
  return {
    board,
    order,
    edges,
    waits,
    drains,
    drainQueue,
    heldDrains,
    submitted,
    submittedPrompts,
    submitAnswers,
    lives,
    submitFailures,
    heldSubmits,
    holdNextSubmit: () => (holdSubmits += 1),
    sleeps,
    notices,
    records,
    logs,
    turn,
    appended,
    appendFailures,
  }
}

export const RECV = 'mcp__plugin_pkg_pkg__mailbox_recv'
export const mailA = [{ reply_to: 'id:a', content: 'first', sent_at: 't1' }]
export const mailB = [{ from: 'p@machine', content: 'second' }]

/** Resolve the latest wait with mail, with `drained` as the drain's answer (direct mode). */
export async function arrive(h: ReturnType<typeof harness>, drained?: DrainResult) {
  if (drained) h.drainQueue.push(drained)
  h.waits[h.waits.length - 1]!.d.resolve('mail')
  await tick()
}

/** Resolve the latest wait as `kind` woke it: a held board message, or one with mail too. */
export async function wake(h: ReturnType<typeof harness>, kind: 'board' | 'both', drained?: DrainResult) {
  if (drained) h.drainQueue.push(drained)
  h.waits[h.waits.length - 1]!.d.resolve(kind)
  await tick()
}

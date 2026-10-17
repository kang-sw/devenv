import type {
  PromptSubmitResult,
  SessionCompactResult,
  SessionCompactTrigger,
  SessionContextUsage,
  SessionUsage,
} from 'claude-code'

import {
  checkLeverCall,
  continuationText,
  deliveryText,
  LEVER_RESULT,
  levelAt,
  observe,
  percentOf,
  skippedContinuationText,
  thresholdsFor,
  type Level,
  type Thresholds,
} from './timing'

/**
 * Lead compaction timing, the compaction feature of the Claude mod: an
 * advisory and two milestone wake turns before native auto compaction, and
 * a lever tool the lead calls at a boundary of its own choosing. The engine
 * still writes the summary, and its auto compaction stays the hard point: the
 * feature never vetoes or preempts a compaction.
 *
 * Main session only: thresholds read the main thread's usage, the lever
 * refuses a subagent, and a subagent's or fork's own compaction is ignored.
 * The engine calls arrive as edges bound in register.ts, so the state machine
 * here is driven and tested without an engine.
 */

/** Whether the compaction feature runs for this package; every other package ships it inert. */
export function compactionEnabledFor(pluginName: string): boolean {
  return pluginName === 'ws'
}

export type CompactionEdges = {
  /** Queue a prompt that starts its own turn once the session is idle. */
  submit: (text: string) => Promise<PromptSubmitResult>
  /** Compact the conversation between turns; rejects while a turn runs. */
  compact: () => Promise<SessionCompactResult>
  /** Run `fn` once, after the calling hook has returned. */
  after: (fn: () => void) => void
  log: (text: string, to: 'transcript' | 'debug') => void
}

/** The lever tool's answer: a refusal, a tool error, or its result text. */
export type LeverAnswer = { deny: string } | { isError: true; result: string; text: string } | { result: string }

export type Compaction = {
  /** session.start: bind the edges and start the conversation's state over. */
  start: (edges: CompactionEdges) => void
  /** session.end: start the conversation's state over; any end reason. */
  end: (reason: string) => void
  /** A lever tool call. */
  lever: (agentId: string | undefined, continuation: unknown) => LeverAnswer
  /** A session.measure: compare usage with the thresholds and submit at most one threshold message. */
  measure: (context: SessionContextUsage, readBreakdown: () => Promise<SessionUsage>) => Promise<void>
  /** A turn.complete: schedule a pending request's compact step, or submit its continuation. */
  turnComplete: (agentId: string | undefined, reason: string) => void
  /** A session.compact as it settled beneath the feature's hook. */
  compacted: (trigger: SessionCompactTrigger, agentId: string | undefined, res: SessionCompactResult) => void
}

/** A lever call waiting for its turn to end. */
type Request = {
  /** The latest call's continuation. */
  continuation: string
  /** False once a compaction landed elsewhere first: the continuation is still owed, the compact step is not. */
  compact: boolean
  /** True while a compaction is scheduled or running for it. */
  scheduled: boolean
}

/** The breakdown's figures, keyed by the model window they were read under. */
type Cached = { window: number; thresholds: Thresholds | undefined }

function describeWindow(t: Thresholds | undefined): string {
  if (!t) return 'thresholds off (auto compaction off, or its point not above the advisory point)'
  const p = (tokens: number) => `${percentOf(tokens, t.raw)}%`
  return `window ${t.raw} tokens: advisory ${p(t.advisory)}, milestones ${p(t.m1)} and ${p(t.m2)}, auto compaction ${p(t.hard)}`
}

export function createCompaction(): Compaction {
  let edges: CompactionEdges | undefined
  let cached: Cached | undefined
  /** The level last delivered; undefined until the first usage observed since a session start or end. */
  let latch: Level | undefined
  let request: Request | undefined
  /** True while the lever's own compaction runs, so its landing is not read as another's. */
  let leverCompacting = false
  /**
   * Bumped at every session start or end and every landed main-conversation
   * compaction: an observation that awaited the breakdown across one of them
   * holds usage from before it and is dropped.
   */
  let generation = 0

  const debug = (text: string) => edges?.log(`ws compaction: ${text}`, 'debug')
  const notice = (text: string) => edges?.log(`ws compaction: ${text}`, 'transcript')

  const reset = () => {
    generation += 1
    cached = undefined
    latch = undefined
    request = undefined
  }

  /**
   * Queue `text`; not awaited, since a submit resolves only as its turn
   * starts. A `record`ed prompt is the only holder of what it says (a
   * continuation), so a failed submit writes it to the transcript.
   */
  const submit = (text: string, record: boolean) => {
    if (!edges) return
    const failed = (why: string) =>
      record ? notice(`a prompt was not submitted (${why}):\n${text}`) : debug(`a prompt was not submitted (${why})`)
    edges.submit(text).then(
      (res) => {
        if (res.drop !== undefined) failed(`dropped: ${res.drop}`)
      },
      (err) => failed(String(err)),
    )
  }

  /** Settle `r` with its continuation prompt. */
  const finish = (r: Request, text: string) => {
    if (request === r) request = undefined
    submit(text, true)
  }

  /**
   * The scheduled compact step. A rejection (a turn started first) keeps the
   * request for the next main-thread turn end; a veto still submits the
   * continuation, with the note that nothing was compacted.
   */
  const runLever = async (r: Request) => {
    if (!edges || request !== r) return
    if (!r.compact) return finish(r, continuationText(r.continuation))
    let res: SessionCompactResult
    leverCompacting = true
    try {
      res = await edges.compact()
    } catch (err) {
      r.scheduled = false
      debug(`compaction rejected (${String(err)}); retrying at the next turn end`)
      return
    } finally {
      leverCompacting = false
    }
    if (request !== r) return
    if (res.skip !== undefined) {
      notice(`the requested compaction did not run (${res.skip}); the continuation follows with a note`)
      finish(r, skippedContinuationText(res.skip, r.continuation))
    } else {
      finish(r, continuationText(r.continuation))
    }
  }

  return {
    start: (bound) => {
      edges = bound
      reset()
    },

    end: (reason) => {
      if (request) debug(`session.end ${reason} dropped a pending compaction request`)
      reset()
    },

    lever: (agentId, continuation) => {
      const checked = checkLeverCall(agentId, continuation)
      if ('deny' in checked) return { deny: checked.deny }
      if ('error' in checked) return { isError: true, result: checked.error, text: checked.error }
      // A repeat call while one is pending states the current next step: it
      // replaces the continuation and keeps the request's compact-step state.
      if (request) request.continuation = checked.continuation
      else request = { continuation: checked.continuation, compact: true, scheduled: false }
      debug(`compaction requested; continuation: ${checked.continuation}`)
      return { result: LEVER_RESULT }
    },

    measure: async (context, readBreakdown) => {
      const tokens = context.tokens
      if (tokens === undefined) return
      const gen = generation
      if (!cached || cached.window !== context.window) {
        const breakdown = (await readBreakdown()).context.breakdown
        if (gen !== generation || !breakdown) return
        cached = { window: context.window, thresholds: thresholdsFor(breakdown) }
        debug(describeWindow(cached.thresholds))
      }
      const t = cached.thresholds
      if (!t) return
      // No threshold message while a lever request is pending: its turn could
      // start first and make compact() reject, or land beside the
      // continuation with stale figures. The crossing stays pending in the latch.
      const step = observe(latch, levelAt(t, tokens), request === undefined)
      latch = step.latch
      if (!step.deliver) return
      debug(`${step.deliver.kind === 'advisory' ? 'advisory' : `milestone ${step.deliver.which}`} at ${percentOf(tokens, t.raw)}%`)
      submit(deliveryText(step.deliver, t, tokens), false)
    },

    // The compact step runs after the turn.complete hook has returned:
    // compact() rejects while a turn runs, and the declarations do not say the
    // turn has stopped running inside its own turn.complete. An interrupted
    // turn leaves the request pending: the human is taking over.
    turnComplete: (agentId, reason) => {
      const r = request
      if (agentId !== undefined || reason === 'aborted' || !r || r.scheduled || !edges) return
      if (!r.compact) return finish(r, continuationText(r.continuation))
      r.scheduled = true
      edges.after(() => {
        runLever(r).catch((err) => {
          r.scheduled = false
          debug(`compaction step failed: ${String(err)}`)
        })
      })
    },

    // `precompute` installs nothing, a veto leaves the conversation as it
    // was, and a subagent's or fork's own compaction is not the main
    // conversation's. A landed one resets the latch and the breakdown, and
    // drops a pending compact step unless it was the lever's own.
    compacted: (trigger, agentId, res) => {
      if (agentId !== undefined || trigger === 'precompute' || res.skip !== undefined) return
      generation += 1
      cached = undefined
      latch = 0
      // Compactions run one at a time, so one landing while the lever's own
      // runs is that one (its trigger is `plugin`).
      if (!leverCompacting && request?.compact) {
        request.compact = false
        notice(`a ${trigger} compaction landed first, so the requested compaction is dropped; its continuation follows at the next turn end`)
      }
    },
  }
}

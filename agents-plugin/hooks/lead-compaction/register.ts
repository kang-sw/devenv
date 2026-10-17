import type {
  PromptSubmitResult,
  Register,
  SessionCompactResult,
  SessionCompactTrigger,
  SessionContextUsage,
  SessionUsage,
} from 'claude-code'

import {
  checkLeverCall,
  continuationText,
  deliveryText,
  LEVER_DESCRIPTION,
  LEVER_RESULT,
  levelAt,
  observe,
  percentOf,
  skippedContinuationText,
  thresholdsFor,
  TOOL_NAME,
  type Level,
  type Thresholds,
} from './timing'

// Claude Code mod giving the lead compaction timing: an advisory and two
// milestone wake turns before native auto compaction, and a lever tool the
// lead calls at a boundary of its own choosing. The engine still writes the
// summary, and its auto compaction stays the hard point: the mod never vetoes
// or preempts a compaction.
//
// Main session only: thresholds read the main thread's usage, the lever
// refuses a subagent, and a subagent's or fork's own compaction is ignored.

/** A lever call waiting for its turn to end. */
type Request = {
  /** The latest call's continuation. */
  continuation: string
  /** False once a compaction landed elsewhere first: the continuation is still owed, the compact step is not. */
  compact: boolean
  /** True while a `$.session.compact()` is scheduled or running for it. */
  scheduled: boolean
}

/** The breakdown's figures, keyed by the model window they were read under. */
type Cached = { window: number; thresholds: Thresholds | undefined }

/**
 * The engine calls that outlive a hook (the scheduled compaction, the
 * submits, the log lines), bound in session.start: the engine refuses a
 * module that stores or passes `$` itself.
 */
type Io = {
  submit: (text: string) => Promise<PromptSubmitResult>
  compact: () => Promise<SessionCompactResult>
  after: (fn: () => void) => void
  log: (text: string, to: 'transcript' | 'debug') => void
}

function describeWindow(t: Thresholds | undefined): string {
  if (!t) return 'thresholds off (auto compaction off, or its point not above the advisory point)'
  const p = (tokens: number) => `${percentOf(tokens, t.raw)}%`
  return `window ${t.raw} tokens: advisory ${p(t.advisory)}, milestones ${p(t.m1)} and ${p(t.m2)}, auto compaction ${p(t.hard)}`
}

export const register: Register = (on) => {
  // Module variables: a hot reload starts them over, and session.start (which
  // a reload raises again) leaves the latch to be baselined afresh.
  let io: Io | undefined
  let toolName: string | undefined
  let cached: Cached | undefined
  /** The level last delivered; undefined until the first usage observed since a session start or end. */
  let latch: Level | undefined
  let request: Request | undefined
  /** True while the lever's own `$.session.compact()` runs, so its landing is not read as another's. */
  let leverCompacting = false
  /**
   * Bumped at every session start or end and every landed main-conversation
   * compaction: an observation that awaited the breakdown across one of them
   * holds usage from before it and is dropped.
   */
  let generation = 0

  const debug = (text: string) => io?.log(`ws compaction: ${text}`, 'debug')
  const notice = (text: string) => io?.log(`ws compaction: ${text}`, 'transcript')

  const reset = () => {
    generation += 1
    cached = undefined
    latch = undefined
    request = undefined
  }

  /**
   * Queue `text` as a prompt of its own; not awaited, since the call resolves
   * only as that turn starts. A `record`ed prompt is the only holder of what
   * it says (a continuation), so a failed submit writes it to the transcript.
   */
  const submit = (text: string, record: boolean) => {
    if (!io) return
    const failed = (why: string) => (record ? notice(`a prompt was not submitted (${why}):\n${text}`) : debug(`a prompt was not submitted (${why})`))
    io.submit(text).then(
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
    if (!io || request !== r) return
    if (!r.compact) return finish(r, continuationText(r.continuation))
    let res: SessionCompactResult
    leverCompacting = true
    try {
      res = await io.compact()
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

  /** A main-conversation compaction landed: reset the latch, re-read the breakdown, and drop a pending compact step unless it was the lever's own. */
  const landed = (trigger: SessionCompactTrigger) => {
    generation += 1
    cached = undefined
    latch = 0
    const ours = trigger === 'plugin' && leverCompacting
    if (!ours && request?.compact) {
      request.compact = false
      notice(`a ${trigger} compaction landed first, so the requested compaction is dropped; its continuation follows at the next turn end`)
    }
  }

  /** Compare one usage figure with the thresholds and submit at most one threshold message. */
  const observeUsage = async (context: SessionContextUsage, readBreakdown: () => Promise<SessionUsage>) => {
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
    // start first and make compact() reject, or land beside the continuation
    // with stale figures. The crossing stays pending in the latch.
    const step = observe(latch, levelAt(t, tokens), request === undefined)
    latch = step.latch
    if (!step.deliver) return
    debug(`${step.deliver.kind === 'advisory' ? 'advisory' : `milestone ${step.deliver.which}`} at ${percentOf(tokens, t.raw)}%`)
    submit(deliveryText(step.deliver, t, tokens), false)
  }

  on('session.start', async ($, e, next) => {
    reset()
    io = {
      submit: (text) => $.prompt.submit({ text }),
      compact: () => $.session.compact(),
      after: (fn) => void $.clock.after(0, fn),
      log: (text, to) => $.ui.log(text, { to }),
    }
    try {
      const registered = await $.tool.register({
        name: TOOL_NAME,
        description: LEVER_DESCRIPTION,
        inputSchema: {
          type: 'object',
          properties: { continuation: { type: 'string' } },
          required: ['continuation'],
        },
        isDeferred: false,
      })
      toolName = registered.tool
    } catch (err) {
      debug(`${TOOL_NAME} not registered (${String(err)})`)
    }
    return next(e)
  })

  on('tool.call', { tool: /__request_compaction$/ }, async ($, e, next) => {
    if (e.tool !== toolName) return next(e)
    const checked = checkLeverCall(e.agentId, (e as { continuation?: unknown }).continuation)
    if ('deny' in checked) return { deny: checked.deny }
    if ('error' in checked) return { isError: true, result: checked.error, text: checked.error }
    // A repeat call while one is pending states the current next step: it
    // replaces the continuation and keeps the request's compact-step state.
    if (request) request.continuation = checked.continuation
    else request = { continuation: checked.continuation, compact: true, scheduled: false }
    debug(`compaction requested; continuation: ${checked.continuation}`)
    return { result: LEVER_RESULT }
  }).catch(($, e, next) => (next.called ? next(e) : { deny: `${TOOL_NAME}: the compaction request hook failed; see the debug log.` }))

  // Pushed after each main-thread turn (and on rate-limit moves); the plain
  // figures cost nothing, and the breakdown is read only when the cache is
  // empty (session start or end, a landed compaction) or the window changed.
  on('session.measure', async ($, e, next) => {
    const res = await next(e)
    try {
      await observeUsage(e.context, () => $.session.usage({ breakdown: 'summary' }))
    } catch (err) {
      debug(`usage observation failed: ${String(err)}`)
    }
    return res
  })

  // The compact step runs after this hook has returned: compact() rejects
  // while a turn runs, and the declarations do not say the turn has stopped
  // running inside its own turn.complete. An interrupted turn leaves the
  // request pending: the human is taking over.
  on('turn.complete', async ($, e, next) => {
    const res = await next(e)
    try {
      const r = request
      if (e.agentId !== undefined || e.reason === 'aborted' || !r || r.scheduled || !io) return res
      if (!r.compact) {
        finish(r, continuationText(r.continuation))
        return res
      }
      r.scheduled = true
      io.after(() => {
        runLever(r).catch((err) => {
          r.scheduled = false
          debug(`compaction step failed: ${String(err)}`)
        })
      })
    } catch (err) {
      debug(`turn end handling failed: ${String(err)}`)
    }
    return res
  })

  // Observes every compaction without changing it. `precompute` installs
  // nothing, a veto leaves the conversation as it was, and a subagent's or
  // fork's own compaction (agentId) is not the main conversation's.
  on('session.compact', async ($, e, next) => {
    const res = await next(e)
    try {
      if (e.agentId === undefined && e.trigger !== 'precompute' && res.skip === undefined) landed(e.trigger)
    } catch (err) {
      debug(`compaction observation failed: ${String(err)}`)
    }
    return res
  }).catch(($, e, next) => next(e)) // an observer never vetoes: replay what settled beneath, or let it run

  // Every end reason starts the conversation's state over: after `clear` or
  // `resume` the process goes on under another conversation, which baselines
  // the latch from its first usage. Compaction is not a session end.
  on('session.end', async ($, e, next) => {
    if (request) debug(`session.end ${e.reason} dropped a pending compaction request`)
    reset()
    return next(e)
  })
}

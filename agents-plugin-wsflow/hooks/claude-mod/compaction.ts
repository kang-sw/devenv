import type {
  PromptSubmitResult,
  SessionCompactResult,
  SessionCompactTrigger,
  SessionContextUsage,
  SessionUsage,
} from 'claude-code'

import {
  checkLeverCall,
  deliveryText,
  leverResult,
  levelAt,
  observe,
  percentOf,
  RESUME_TEXT,
  thresholdsFor,
  vetoedResumeText,
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
 * The resume prompt is opt-in: a lever call without its `continue` flag
 * (work does not need to resume by itself) submits nothing after the step
 * and opens no window, and a veto is told to the person through a transcript
 * notice only.
 *
 * With the flag, the fixed resume prompt after the lever's own compaction
 * (landed or vetoed) is held for CONTINUATION_WINDOW_MS rather than submitted
 * at once, and any main-thread turn starting within the window cancels it: a
 * prompt the person typed during the compaction, or a background agent's
 * notification, already started the first turn after it, which resumes the
 * work there, so the resume would arrive only as a stale prompt after that
 * turn. The window's end submits it only when no turn started.
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

/** How long the lever's resume prompt waits for another prompt to start the first turn after its compaction. */
export const CONTINUATION_WINDOW_MS = 1500

export type CompactionEdges = {
  /** Queue a prompt that starts its own turn once the session is idle. */
  submit: (text: string) => Promise<PromptSubmitResult>
  /** Compact the conversation between turns; rejects while a turn runs. */
  compact: () => Promise<SessionCompactResult>
  /** Run `fn` once, after the calling hook has returned. */
  after: (fn: () => void) => void
  /** Run `fn` once, `ms` from now. */
  delay: (ms: number, fn: () => void) => void
  log: (text: string, to: 'transcript' | 'debug') => void
  /**
   * Told when the lever's own compaction landed. The engine routes it past
   * the mod's session.compact hook, so another feature that resets on a
   * landing hears of this one only here.
   */
  leverLanded?: () => void
}

/** The lever tool's answer: a refusal or its result text. */
export type LeverAnswer = { deny: string } | { result: string }

export type Compaction = {
  /** session.start: bind the edges and start the conversation's state over. */
  start: (edges: CompactionEdges) => void
  /** session.end: start the conversation's state over; any end reason. */
  end: (reason: string) => void
  /** A lever tool call. */
  lever: (agentId: string | undefined, flag: unknown) => LeverAnswer
  /** A session.measure: compare usage with the thresholds and submit at most one threshold message. */
  measure: (context: SessionContextUsage, readBreakdown: () => Promise<SessionUsage>) => Promise<void>
  /** A turn.start: a main-thread one cancels a held resume prompt. */
  turnStarted: (agentId: string | undefined) => void
  /** A turn.complete: schedule a pending request's compact step, or submit its resume prompt. */
  turnComplete: (agentId: string | undefined, reason: string) => void
  /** A session.compact as it settled beneath the feature's hook. */
  compacted: (trigger: SessionCompactTrigger, agentId: string | undefined, res: SessionCompactResult) => void
}

/** A lever call waiting for its turn to end. */
type Request = {
  /** Whether the latest call set the `continue` flag; unset leaves nothing to submit. */
  resume: boolean
  /** False once a compaction landed elsewhere first: the resume prompt (if any) is still owed, the compact step is not. */
  compact: boolean
  /**
   * The token of its queued compact step, from the turn end that queued it
   * until that step starts; undefined while none is queued.
   */
  scheduled: number | undefined
  /** Whether a rejection of its compaction was already noted in the transcript. */
  rejectionNoted: boolean
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
  /** The lever's resume prompt held for its window; see CONTINUATION_WINDOW_MS. */
  let held: { text: string } | undefined
  /**
   * True while the lever's own compaction runs. The engine routes a
   * `$.session.compact` past the calling plugin's own session.compact hook,
   * so runLever settles the landing itself; should the hook see it anyway,
   * this keeps it from being read as another's landing or counted twice.
   */
  let leverCompacting = false
  /** The last compact-step token handed out; see Request.scheduled. */
  let dispatches = 0
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
    held = undefined
  }

  /**
   * Queue `text`; not awaited, since a submit resolves only as its turn
   * starts. A lost `record`ed prompt (a resume) is written to the
   * transcript, so the person sees the work was meant to resume.
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

  /** A landed main-conversation compaction: the latch and the breakdown start over. */
  const landed = () => {
    generation += 1
    cached = undefined
    latch = 0
  }

  /** Settle `r` with its resume prompt, unguarded; an unset flag submits nothing. */
  const finish = (r: Request) => {
    if (request === r) request = undefined
    if (r.resume) submit(RESUME_TEXT, true)
  }

  /**
   * Settle `r` after its own compaction: hold the resume prompt, and submit
   * it at the window's end unless a main-thread turn started meanwhile.
   */
  const hold = (r: Request, text: string) => {
    if (request === r) request = undefined
    // A window still open is superseded: one resume prompt covers both.
    const h = { text }
    held = h
    try {
      edges?.delay(CONTINUATION_WINDOW_MS, () => {
        if (held !== h) return
        held = undefined
        submit(h.text, true)
      })
    } catch (err) {
      debug(`the resume window was not started (${String(err)}); submitting the resume prompt now`)
      if (held === h) held = undefined
      submit(text, true)
    }
  }

  /**
   * The scheduled compact step. A rejection (most often a turn started
   * first) keeps the request for the next main-thread turn end and is noted
   * in the transcript once per request; a veto with the flag still holds a
   * resume prompt, saying that nothing was compacted.
   */
  const runLever = async (r: Request) => {
    if (!edges || request !== r) return
    if (!r.compact) return finish(r)
    let res: SessionCompactResult
    leverCompacting = true
    try {
      res = await edges.compact()
    } catch (err) {
      const text = `the requested compaction was rejected (${String(err)}); retrying at the next turn end`
      if (r.rejectionNoted) debug(text)
      else notice(text)
      r.rejectionNoted = true
      return
    } finally {
      leverCompacting = false
    }
    if (res.skip === undefined) {
      landed()
      try {
        edges?.leverLanded?.()
      } catch (err) {
        debug(`landing notification failed: ${String(err)}`)
      }
    }
    if (request !== r) return
    if (!r.resume) {
      // No turn follows, so a veto is told to the person here: the model is
      // not started to hear it and no window opens.
      request = undefined
      if (res.skip !== undefined) notice(`the requested compaction did not run (${res.skip}); no resume was requested, so no turn follows`)
    } else if (res.skip !== undefined) {
      notice(`the requested compaction did not run (${res.skip}); a resume prompt follows with a note`)
      hold(r, vetoedResumeText(res.skip))
    } else {
      hold(r, RESUME_TEXT)
    }
  }

  return {
    start: (bound) => {
      edges = bound
      reset()
    },

    end: (reason) => {
      if (request) debug(`session.end ${reason} dropped a pending compaction request`)
      if (held) notice(`a prompt was not submitted (session.end ${reason}):\n${held.text}`)
      reset()
    },

    lever: (agentId, flag) => {
      const checked = checkLeverCall(agentId, flag)
      if ('deny' in checked) return { deny: checked.deny }
      // A repeat call while one is pending replaces the flag (latest wins, so
      // an unset one clears an earlier true) and keeps the request's
      // compact-step state.
      if (request) request.resume = checked.resume
      else request = { resume: checked.resume, compact: true, scheduled: undefined, rejectionNoted: false }
      debug(`compaction requested; resume: ${checked.resume}`)
      return { result: leverResult(checked.resume) }
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
      // start first and make compact() reject, or land beside the resume
      // prompt with stale figures. Nor while its resume prompt is held: a
      // threshold turn would start first and cancel it. The crossing stays pending in the latch.
      const step = observe(latch, levelAt(t, tokens), request === undefined && held === undefined)
      latch = step.latch
      if (!step.deliver) return
      debug(`${step.deliver.kind === 'advisory' ? 'advisory' : `milestone ${step.deliver.which}`} at ${percentOf(tokens, t.raw)}%`)
      submit(deliveryText(step.deliver, t, tokens), false)
    },

    // Any main-thread turn start, whatever started it, is the first turn
    // after the step: it resumes the work, so the held resume would only
    // arrive after it as a stale prompt. A subagent's turn is not that turn.
    turnStarted: (agentId) => {
      if (agentId !== undefined || !held) return
      held = undefined
      debug('a turn started within the resume window; the resume prompt is not submitted')
    },

    // The compact step runs after the turn.complete hook has returned:
    // compact() rejects while a turn runs, and the declarations do not say the
    // turn has stopped running inside its own turn.complete. An interrupted
    // turn leaves the request pending: the human is taking over. A step still
    // queued at a later turn end never started (the engine runs no `after`
    // callback whose dispatch a hook refused), so it is queued again; the
    // token keeps the stale one inert should it run after all.
    turnComplete: (agentId, reason) => {
      const r = request
      if (agentId !== undefined || reason === 'aborted' || !r || leverCompacting || !edges) return
      if (!r.compact) return finish(r)
      if (r.scheduled !== undefined) debug('the scheduled compaction step did not run; scheduling it again')
      const token = (dispatches += 1)
      r.scheduled = token
      try {
        edges.after(() => {
          if (r.scheduled !== token) return
          r.scheduled = undefined
          runLever(r).catch((err) => debug(`compaction step failed: ${String(err)}`))
        })
      } catch (err) {
        if (r.scheduled === token) r.scheduled = undefined
        debug(`the compaction step was not scheduled (${String(err)}); retrying at the next turn end`)
      }
    },

    // `precompute` installs nothing, a veto leaves the conversation as it
    // was, and a subagent's or fork's own compaction is not the main
    // conversation's. A landed one resets the latch and the breakdown, and
    // drops a pending compact step. Compactions run one at a time, so one
    // landing while the lever's own runs is that one, which runLever settles.
    compacted: (trigger, agentId, res) => {
      if (agentId !== undefined || trigger === 'precompute' || res.skip !== undefined || leverCompacting) return
      landed()
      if (request?.compact) {
        request.compact = false
        notice(
          `a ${trigger} compaction landed first, so the requested compaction is dropped; ` +
            (request.resume ? 'its resume prompt follows at the next turn end' : 'no resume was requested, so no prompt follows'),
        )
      }
    },
  }
}

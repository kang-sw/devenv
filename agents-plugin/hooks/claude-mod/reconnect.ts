import type { OwnPrompt } from './captions'

/**
 * The MCP reconnect lever, a feature of the Claude mod: a tool the model calls
 * when the plugin's own MCP server has gone, and a reconnect step at the next
 * main-thread turn end. An interactive session does not restart a dead plugin
 * stdio server, and `$.mcp.connect` does not either; `/mcp reconnect` does, but
 * the host refuses it from a hook the turn waits on, so the tool only records
 * the request and the step runs after the turn.complete hook has returned.
 *
 * The step always runs `/mcp reconnect <server>` (a listed-but-failing server
 * is one a skip would miss), judges the outcome from the tool list, and reports
 * it. A failure is always submitted. A success submits the fixed resume prompt
 * only for a call with the `continue` flag, and only when no main-thread turn
 * started since the step started: such a turn (a background agent's
 * notification, a typed prompt) already resumes the work, so the resume would
 * arrive after it as a stale prompt. The engine calls arrive as edges bound in
 * register.ts, so the state machine here is driven and tested without an
 * engine.
 *
 * The watchdog serves a session with no turn to call the lever in (one left
 * idle overnight): armed by register.ts when its knob reads on, it looks at the
 * tool list once per interval and, when none of the server's tools is listed,
 * runs the lever's own attempt. It runs one attempt at a time with the lever:
 * a tick does nothing while a main-thread turn runs or a lever step is
 * scheduled or running, and a lever step that comes due while a watchdog
 * attempt is in flight reports that attempt's outcome instead of reconnecting
 * again. A success writes one transcript line and submits nothing; a failure
 * writes one transcript notice per failure streak, and after
 * WATCHDOG_MAX_FAILURES attempts in a row it stops attempting until the server
 * resolves again (`resolved`), a tick finds its tools listed again, or a new
 * session start re-arms it.
 */

export const TOOL_NAME = 'request_mcp_reconnect'
/** How long the step waits for the server's tools to come back after the reconnect command settled. */
export const TOOLS_WAIT_MS = 10_000
/** How often it looks at the tool list while waiting. */
export const TOOLS_POLL_MS = 500

export function reconnectDescription(plugin: string): string {
  return (
    `Call this first when the ${plugin} MCP server's tools have left your tool list or a call reports it not connected. ` +
    `It reconnects the ${plugin} MCP server when the current turn ends. ` +
    'Set `continue` to true only when work must resume by itself after the reconnect attempt: a fixed resume prompt then follows a successful reconnect, unless another prompt has already started the next turn. ' +
    "Leave it unset when the next turn will come from the human or from a background agent's notification; a failed reconnect is always reported to you. " +
    'Main session only.'
  )
}

export const RECONNECT_RESULT =
  'Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends, and a resume prompt follows a success unless another prompt has already started the next turn. A failed reconnect is reported to you in a prompt.'

/** The tool's result for a call without the `continue` flag: a successful reconnect submits nothing. */
export const RECONNECT_RESULT_NO_RESUME =
  "Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends. A successful reconnect submits no prompt: the next turn comes from the human or from a background agent's notification. A failed reconnect is reported to you in a prompt."

/** What the tool's result says for a call with or without the `continue` flag. */
export function reconnectResult(resume: boolean): string {
  return resume ? RECONNECT_RESULT : RECONNECT_RESULT_NO_RESUME
}

/** The last sentence of the resume prompt, which also ends a failure report for a call with the flag. */
export const RESUME_SENTENCE = 'Resume the interrupted work.'

/** Opens every prompt the lever submits, so it never reads as the human's own words. */
export function label(plugin: string): string {
  return `[${plugin} mcp] `
}

export function successText(plugin: string): string {
  return `${label(plugin)}The ${plugin} MCP server reconnected; its tools are available again. ${RESUME_SENTENCE}`
}

export function failureText(plugin: string, reason: string, resume: boolean): string {
  const head =
    `${label(plugin)}The ${plugin} MCP server did not come back (${reason}). ` +
    `Do not call ${TOOL_NAME} again for this drop: a reconnect that just failed fails the same way. ` +
    'Ask the user to re-enable the server from /mcp, run /reload-plugins, or restart the session.'
  return resume ? `${head} ${RESUME_SENTENCE}` : head
}

/** The one-liner the success prompt shows the person while captions are on. */
export function successLine(plugin: string): string {
  return `${label(plugin).trimEnd()} reconnected`
}

/** The one-liner the failure prompt shows the person while captions are on. */
export function failureLine(plugin: string, reason: string): string {
  return `${label(plugin).trimEnd()} failed: ${reason}`
}

/** The failure reasons the step words; the command's own words fill the last two. */
export const REASON_NO_SERVER = 'its server name was never resolved in this session'
export const REASON_TOOLS_ABSENT = `its tools were not back within ${TOOLS_WAIT_MS / 1000} s`
export function reasonCommandFailed(err: unknown): string {
  return `the reconnect command failed: ${String(err)}`
}
export function reasonCommandAnswered(text: string): string {
  return `the reconnect command answered: ${text}`
}

/** How many failed watchdog attempts in a row stop it until the server resolves again. */
export const WATCHDOG_MAX_FAILURES = 3

/** The transcript line of a successful watchdog reconnect; no prompt follows it. */
export function watchdogSuccessText(plugin: string): string {
  return `${plugin} mcp watchdog: the ${plugin} MCP server had dropped and was reconnected; its tools are available again.`
}

/** The one transcript notice of a watchdog failure streak. */
export function watchdogFailureText(plugin: string, reason: string, intervalMs: number): string {
  return (
    `${plugin} mcp watchdog: the ${plugin} MCP server dropped and did not come back (${reason}). ` +
    `It is tried again every ${intervalMs / 60_000} min, ${WATCHDOG_MAX_FAILURES} attempts in a row at most; ` +
    'after that, re-enable it from /mcp, run /reload-plugins, or restart the session.'
  )
}

/**
 * Main-loop-only guard and input read for the tool: a refusal, or whether the
 * call set the `continue` flag. Only a literal `true` sets it; absent or any
 * other value (a non-boolean, a leftover string argument) reads as unset.
 */
export function checkReconnectCall(agentId: string | undefined, flag: unknown): { resume: boolean } | { deny: string } {
  if (agentId !== undefined) {
    return { deny: `${TOOL_NAME} serves the main session only; called from subagent ${agentId}.` }
  }
  return { resume: flag === true }
}

/**
 * The words of a `/mcp reconnect` answer that reports it did not reconnect,
 * or undefined. An interactive reconnect answers no text (its success line is
 * the transcript's); a session that cannot reconnect (headless) answers why in
 * text while the server's tools stay listed, so the tool list alone would read
 * it as a success. A text that says the server reconnected is not a failure.
 */
export function commandRefusal(res: { text?: string } | undefined): string | undefined {
  const text = res?.text?.trim()
  if (!text || /\breconnected\b/i.test(text)) return undefined
  return text
}

export type ReconnectEdges = {
  /** The package name the prose spells. */
  plugin: string
  /**
   * The MCP server name to reconnect: the resolved one, else the name derived
   * from the plugin's own files for a server that never resolved; undefined
   * with neither.
   */
  server: () => string | undefined
  /** Run `/mcp reconnect <server>`; queued by the engine until the session is idle. */
  reconnect: (server: string) => Promise<{ text?: string }>
  /** Whether the tool list holds any of the server's tools now. */
  toolsListed: (server: string) => Promise<boolean>
  /** Resolve the server name again after a reconnect, updating the shared cache. */
  resolve: () => Promise<void>
  /** Queue a prompt that starts its own turn once the session is idle. */
  submit: (prompt: OwnPrompt) => Promise<{ drop?: string }>
  /** Run `fn` once, after the calling hook has returned. */
  after: (fn: () => void) => void
  sleep: (ms: number) => Promise<void>
  log: (text: string, to: 'transcript' | 'debug') => void
  /** Whether a main-thread turn runs now: a watchdog tick then does nothing. */
  turnRunning: () => boolean
  /** Call `fn` every `ms` until the answered function stops it. */
  every: (ms: number, fn: () => void) => () => void
}

/** The tool's answer: a refusal or its result text. */
export type ReconnectAnswer = { deny: string } | { result: string }

export type Reconnect = {
  /** session.start: bind the edges and drop any request. */
  start: (edges: ReconnectEdges) => void
  /** session.end: drop a pending request and any step's report still owed; any end reason. */
  end: (reason: string) => void
  /** A tool call. */
  lever: (agentId: string | undefined, flag: unknown) => ReconnectAnswer
  /** A turn.start: a main-thread one makes a running step's success owe no resume prompt. */
  turnStarted: (agentId: string | undefined) => void
  /** A turn.complete: schedule a pending request's reconnect step. */
  turnComplete: (agentId: string | undefined, reason: string) => void
  /** Arm the watchdog with a tick every `intervalMs`, replacing one armed before. */
  watch: (intervalMs: number) => void
  /** A successful server resolution: ends a watchdog failure streak. */
  resolved: () => void
}

/** A tool call waiting for its turn to end. */
type Request = {
  /** Whether the latest call set the `continue` flag. */
  resume: boolean
  /** The token of its queued step, from the turn end that queued it until the step starts. */
  scheduled: number | undefined
}

type Outcome = { ok: true } | { ok: false; reason: string }

export function createReconnect(): Reconnect {
  let edges: ReconnectEdges | undefined
  let request: Request | undefined
  let dispatches = 0
  /** Bumped at every session start or end: a step that awaited across one owes nothing to the new conversation. */
  let generation = 0
  /** Bumped at every main-thread turn start: a step that sees it move started no resume of its own. */
  let turns = 0
  /** The attempt running now, the lever's or the watchdog's; one at a time. */
  let inFlight: { by: 'lever' | 'watchdog'; outcome: Promise<Outcome> } | undefined
  /** Stops the armed watchdog timer. */
  let unwatch: (() => void) | undefined
  let intervalMs = 0
  /** Failed watchdog attempts in a row; at WATCHDOG_MAX_FAILURES the watchdog stops attempting. */
  let failures = 0
  /** Whether a watchdog tick is under way, so ticks never overlap. */
  let ticking = false

  const label = () => `${edges?.plugin ?? 'plugin'} mcp reconnect`
  const debug = (text: string) => edges?.log(`${label()}: ${text}`, 'debug')
  const notice = (text: string) => edges?.log(`${label()}: ${text}`, 'transcript')
  const watchDebug = (text: string) => edges?.log(`${edges?.plugin ?? 'plugin'} mcp watchdog: ${text}`, 'debug')

  const reset = () => {
    generation += 1
    request = undefined
  }

  /** Reconnect, then wait a bounded time for the server's tools; never throws. */
  const attempt = async (e: ReconnectEdges): Promise<Outcome> => {
    const server = e.server()
    if (!server) return { ok: false, reason: REASON_NO_SERVER }
    let answer: { text?: string } | undefined
    try {
      answer = await e.reconnect(server)
    } catch (err) {
      return { ok: false, reason: reasonCommandFailed(err) }
    }
    const refused = commandRefusal(answer)
    if (refused !== undefined) return { ok: false, reason: reasonCommandAnswered(refused) }
    for (let waited = 0; ; waited += TOOLS_POLL_MS) {
      let listed = false
      try {
        listed = await e.toolsListed(server)
      } catch (err) {
        debug(`tool list read failed: ${String(err)}`)
      }
      if (listed) break
      if (waited >= TOOLS_WAIT_MS) return { ok: false, reason: REASON_TOOLS_ABSENT }
      await e.sleep(TOOLS_POLL_MS)
    }
    try {
      await e.resolve()
    } catch (err) {
      debug(`server name not resolved again after the reconnect: ${String(err)}`)
    }
    return { ok: true }
  }

  /** Run one attempt as the one in flight. */
  const tracked = async (by: 'lever' | 'watchdog', e: ReconnectEdges): Promise<Outcome> => {
    const entry = { by, outcome: attempt(e) }
    inFlight = entry
    try {
      return await entry.outcome
    } finally {
      if (inFlight === entry) inFlight = undefined
    }
  }

  /** Why a watchdog tick may not attempt now, or undefined when it may. */
  const busy = (e: ReconnectEdges): string | undefined => {
    if (inFlight) return `a ${inFlight.by} reconnect is in flight`
    if (request?.scheduled !== undefined) return 'a lever reconnect step is scheduled'
    if (e.turnRunning()) return 'a main-thread turn runs'
    return undefined
  }

  /**
   * One watchdog tick. A lever request left pending by an aborted turn does
   * not hold it off: that request still runs at its next turn end. Never
   * throws.
   */
  const tick = async () => {
    const e = edges
    if (!e || ticking) return
    const held = busy(e)
    if (held) return watchDebug(`tick skipped: ${held}`)
    const server = e.server()
    if (!server) return watchDebug(`tick skipped: ${REASON_NO_SERVER}`)
    ticking = true
    try {
      let listed: boolean
      try {
        listed = await e.toolsListed(server)
      } catch (err) {
        return watchDebug(`tick skipped: tool list read failed: ${String(err)}`)
      }
      if (listed) {
        // The server is evidently up, however it came back (a person's own
        // /mcp reconnect among the ways): the failure streak is over.
        if (failures > 0) watchDebug('the tools are listed again; the failure streak ended')
        failures = 0
        return
      }
      if (failures >= WATCHDOG_MAX_FAILURES) return watchDebug('tick skipped: stopped after failed attempts until the server resolves again')
      // Checked again: a lever step or a turn may have started during the read.
      const late = busy(e)
      if (late) return watchDebug(`tick skipped: ${late}`)
      watchDebug(`the ${e.plugin} MCP server's tools are not listed; reconnecting ${server}`)
      const outcome = await tracked('watchdog', e)
      if (outcome.ok) {
        failures = 0
        e.log(watchdogSuccessText(e.plugin), 'transcript')
        return
      }
      failures += 1
      watchDebug(`not reconnected (${outcome.reason}); failure ${failures} of ${WATCHDOG_MAX_FAILURES}`)
      if (failures === 1) e.log(watchdogFailureText(e.plugin, outcome.reason, intervalMs), 'transcript')
    } finally {
      ticking = false
    }
  }

  /**
   * The scheduled step. It takes the request as it starts, so a call made
   * while it runs is a new request for a later turn end. A lost submit is
   * written to the transcript, so the person sees what the model missed.
   */
  const run = async (r: Request) => {
    const e = edges
    if (!e || request !== r) return
    request = undefined
    const gen = generation
    const startedAt = turns
    // A watchdog attempt in flight is this request's reconnect: two at once
    // could race, so the step reports that attempt's outcome.
    const watchdog = inFlight?.by === 'watchdog' ? inFlight : undefined
    if (watchdog) debug('a watchdog reconnect is in flight; the step reports its outcome')
    const outcome = await (watchdog ? watchdog.outcome : tracked('lever', e))
    if (gen !== generation) {
      debug('the session ended during the reconnect step; its report is dropped')
      return
    }
    debug(outcome.ok ? 'reconnected' : `not reconnected (${outcome.reason})`)
    // A failure is always reported: the model must learn its tools are gone,
    // whatever turn runs. A success owes a resume only with the flag, and
    // only while no turn started since the step did.
    if (outcome.ok) {
      if (!r.resume) return
      if (turns !== startedAt) {
        debug('a turn started during the reconnect step; the resume prompt is not submitted')
        return
      }
    }
    const prompt = outcome.ok
      ? { text: successText(e.plugin), line: successLine(e.plugin) }
      : { text: failureText(e.plugin, outcome.reason, r.resume), line: failureLine(e.plugin, outcome.reason) }
    const failed = (why: string) => notice(`a prompt was not submitted (${why}):\n${prompt.text}`)
    e.submit(prompt).then(
      (res) => {
        if (res.drop !== undefined) failed(`dropped: ${res.drop}`)
      },
      (err) => failed(String(err)),
    )
  }

  return {
    // A new start re-reads the watchdog knobs and re-arms it with a fresh
    // failure streak; an end leaves it armed, since the server belongs to the
    // process, not the conversation.
    start: (bound) => {
      unwatch?.()
      unwatch = undefined
      failures = 0
      edges = bound
      reset()
    },

    end: (reason) => {
      if (request) debug(`session.end ${reason} dropped a pending reconnect request`)
      reset()
    },

    lever: (agentId, flag) => {
      const checked = checkReconnectCall(agentId, flag)
      if ('deny' in checked) return { deny: checked.deny }
      // A repeat call while one is pending replaces the flag (latest wins, so
      // an unset one clears an earlier true).
      if (request) request.resume = checked.resume
      else request = { resume: checked.resume, scheduled: undefined }
      debug(`reconnect requested; resume: ${checked.resume}`)
      return { result: reconnectResult(checked.resume) }
    },

    turnStarted: (agentId) => {
      if (agentId === undefined) turns += 1
    },

    // The step runs after the turn.complete hook has returned: the host refuses
    // `/mcp reconnect` from a hook the turn waits on, and the command can take
    // many seconds to settle, past any hook's time bound. An interrupted turn
    // or a subagent's leaves the request pending. A step still queued at a
    // later turn end never started, so it is queued again; the token keeps the
    // stale one inert should it run after all.
    turnComplete: (agentId, reason) => {
      const r = request
      if (agentId !== undefined || reason === 'aborted' || !r || !edges) return
      if (r.scheduled !== undefined) debug('the scheduled reconnect step did not run; scheduling it again')
      const token = (dispatches += 1)
      r.scheduled = token
      try {
        edges.after(() => {
          if (r.scheduled !== token) return
          r.scheduled = undefined
          run(r).catch((err) => debug(`reconnect step failed: ${String(err)}`))
        })
      } catch (err) {
        if (r.scheduled === token) r.scheduled = undefined
        debug(`the reconnect step was not scheduled (${String(err)}); retrying at the next turn end`)
      }
    },

    watch: (ms) => {
      const e = edges
      if (!e) return
      unwatch?.()
      intervalMs = ms
      unwatch = e.every(ms, () => {
        tick().catch((err) => watchDebug(`tick failed: ${String(err)}`))
      })
      watchDebug(`armed; a tick every ${ms / 60_000} min`)
    },

    resolved: () => {
      if (failures > 0) watchDebug('the server resolved again; the failure streak ended')
      failures = 0
    },
  }
}

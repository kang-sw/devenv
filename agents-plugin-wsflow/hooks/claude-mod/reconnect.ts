import type { PromptSubmitResult } from 'claude-code'

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

export function successText(plugin: string): string {
  return `[${plugin} mcp] The ${plugin} MCP server reconnected; its tools are available again. ${RESUME_SENTENCE}`
}

export function failureText(plugin: string, reason: string, resume: boolean): string {
  const head =
    `[${plugin} mcp] The ${plugin} MCP server did not come back (${reason}). ` +
    `Do not call ${TOOL_NAME} again for this drop: a reconnect that just failed fails the same way. ` +
    'Ask the user to re-enable the server from /mcp, run /reload-plugins, or restart the session.'
  return resume ? `${head} ${RESUME_SENTENCE}` : head
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
  /** The resolved MCP server name, or undefined while none was resolved. */
  server: () => string | undefined
  /** Run `/mcp reconnect <server>`; queued by the engine until the session is idle. */
  reconnect: (server: string) => Promise<{ text?: string }>
  /** Whether the tool list holds any of the server's tools now. */
  toolsListed: (server: string) => Promise<boolean>
  /** Resolve the server name again after a reconnect, updating the shared cache. */
  resolve: () => Promise<void>
  /** Queue a prompt that starts its own turn once the session is idle. */
  submit: (text: string) => Promise<PromptSubmitResult>
  /** Run `fn` once, after the calling hook has returned. */
  after: (fn: () => void) => void
  sleep: (ms: number) => Promise<void>
  log: (text: string, to: 'transcript' | 'debug') => void
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

  const label = () => `${edges?.plugin ?? 'plugin'} mcp reconnect`
  const debug = (text: string) => edges?.log(`${label()}: ${text}`, 'debug')
  const notice = (text: string) => edges?.log(`${label()}: ${text}`, 'transcript')

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
    const outcome = await attempt(e)
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
    const text = outcome.ok ? successText(e.plugin) : failureText(e.plugin, outcome.reason, r.resume)
    const failed = (why: string) => notice(`a prompt was not submitted (${why}):\n${text}`)
    e.submit(text).then(
      (res) => {
        if (res.drop !== undefined) failed(`dropped: ${res.drop}`)
      },
      (err) => failed(String(err)),
    )
  }

  return {
    start: (bound) => {
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
  }
}

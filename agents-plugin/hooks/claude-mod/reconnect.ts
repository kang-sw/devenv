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
 * is one a skip would miss), judges the outcome from the tool list, and submits
 * the continuation with the outcome. A call without a continuation submits
 * nothing on success and only the failure notice on failure. The engine calls arrive as edges bound in
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
    `Reconnect the ${plugin} MCP server when the current turn ends. ` +
    "Use it when that server's tools have left your tool list or a call reports the server not connected. " +
    '`continuation` is optional: pass it only when work must resume by itself after the reconnect attempt, as one line naming what to do next; it is sent as your next prompt with the outcome. ' +
    "Omit it when the next turn will come from the human or from a background agent's notification; a failed reconnect is still reported to you. " +
    'Main session only.'
  )
}

export const RECONNECT_RESULT =
  'Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends, and your continuation resumes the work.'

/** The lever's result for a call without a continuation: a successful reconnect submits nothing. */
export const RECONNECT_RESULT_NO_CONTINUATION =
  "Reconnect requested. End your turn now without further tool calls; the reconnect runs when the turn ends. A successful reconnect submits no prompt: the next turn comes from the human or from a background agent's notification. A failed reconnect is reported to you in a prompt."

/** What the tool's result says for a call with or without a continuation. */
export function reconnectResult(continuation: string | undefined): string {
  return continuation === undefined ? RECONNECT_RESULT_NO_CONTINUATION : RECONNECT_RESULT
}

export function successText(plugin: string, continuation: string): string {
  return `[${plugin} mcp] The ${plugin} MCP server reconnected; its tools are available again.\nContinue: ${continuation}`
}

export function failureText(plugin: string, reason: string, continuation: string | undefined): string {
  const head =
    `[${plugin} mcp] The ${plugin} MCP server did not come back (${reason}). ` +
    `Follow the ${plugin}:mcp-server-repair skill: keep working through its CLI fallback and relay its reconnect steps to the user.`
  return continuation === undefined ? head : `${head}\nContinue: ${continuation}`
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
 * Main-loop-only guard and input check for the tool: a refusal, a tool error,
 * or the trimmed continuation (undefined when omitted: absent, non-string, or
 * whitespace-only).
 */
export function checkReconnectCall(
  agentId: string | undefined,
  continuation: unknown,
): { continuation: string | undefined } | { deny: string } | { error: string } {
  if (agentId !== undefined) {
    return { deny: `${TOOL_NAME} serves the main session only; called from subagent ${agentId}.` }
  }
  const text = typeof continuation === 'string' ? continuation.trim() : ''
  if (!text) return { continuation: undefined }
  if (/[\r\n]/.test(text)) return { error: `${TOOL_NAME}: continuation must be one line.` }
  return { continuation: text }
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

/** The tool's answer: a refusal, a tool error, or its result text. */
export type ReconnectAnswer = { deny: string } | { isError: true; result: string; text: string } | { result: string }

export type Reconnect = {
  /** session.start: bind the edges and drop any request. */
  start: (edges: ReconnectEdges) => void
  /** session.end: drop a pending request and any step's continuation still owed; any end reason. */
  end: (reason: string) => void
  /** A tool call. */
  lever: (agentId: string | undefined, continuation: unknown) => ReconnectAnswer
  /** A turn.complete: schedule a pending request's reconnect step. */
  turnComplete: (agentId: string | undefined, reason: string) => void
}

/** A tool call waiting for its turn to end. */
type Request = {
  /** The latest call's continuation; undefined when that call omitted it. */
  continuation: string | undefined
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
   * while it runs is a new request for a later turn end. A continuation is the
   * only holder of its text, so a failed submit writes it to the transcript.
   */
  const run = async (r: Request) => {
    const e = edges
    if (!e || request !== r) return
    request = undefined
    const gen = generation
    const outcome = await attempt(e)
    if (gen !== generation) {
      debug('the session ended during the reconnect step; its continuation is dropped')
      return
    }
    debug(outcome.ok ? 'reconnected' : `not reconnected (${outcome.reason})`)
    // Without a continuation a success owes the model nothing, but a failure
    // still does: it must learn its tools are gone.
    if (outcome.ok && r.continuation === undefined) return
    const text = outcome.ok ? successText(e.plugin, r.continuation as string) : failureText(e.plugin, outcome.reason, r.continuation)
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

    lever: (agentId, continuation) => {
      const checked = checkReconnectCall(agentId, continuation)
      if ('deny' in checked) return { deny: checked.deny }
      if ('error' in checked) return { isError: true, result: checked.error, text: checked.error }
      // A repeat call while one is pending states the current next step; an
      // omitted continuation clears an earlier one.
      if (request) request.continuation = checked.continuation
      else request = { continuation: checked.continuation, scheduled: undefined }
      debug(`reconnect requested; continuation: ${checked.continuation ?? '(none)'}`)
      return { result: reconnectResult(checked.continuation) }
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

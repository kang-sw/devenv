import type { HookStream, ProcessSpawnChunk, ProcessSpawnResult, Register, ToolCallResult } from 'claude-code'

import { createWatcher, mapWaitExit, parseRecvText, type MailboxEnvelope, type Watcher, type WaitOutcome } from './watcher'

// Probe constants. LAUNCHER is the installed ws plugin's launcher (the
// `kang-sw-devenv` directory marketplace); pi spawns the same path with
// `mailbox wait`.
export const LAUNCHER = '/home/swkang/.claude/plugins/ws-plugin/ws/bin/ws-mcp-launcher.py'
// Settled design: 0 blocks until mail. Pi uses a finite "10m" so a wedged or
// orphaned wait self-heals; switch here if the live test shows stale children.
export const WAIT_TIMEOUT = '0'
export const RECV_TOOL = 'mcp__plugin_ws_ws__mailbox_recv'
export const TOOL_NAME = 'mailbox_watch'

export function waitArgv(key: string): string[] {
  return ['python3', LAUNCHER, 'mailbox', 'wait', '--session-key', key, '--timeout', WAIT_TIMEOUT, '--format', 'json']
}

/** Consume one spawned `mailbox wait` stream; abort ends it (closing the stream kills the child). */
async function consumeWait(
  stream: HookStream<ProcessSpawnChunk, ProcessSpawnResult>,
  signal: AbortSignal,
  debug: (text: string) => void,
): Promise<WaitOutcome> {
  let aborted = false
  const onAbort = () => {
    aborted = true
    void stream.return(undefined as never).catch(() => {})
  }
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    const consumed = (async () => {
      let stderr = ''
      for (;;) {
        const step = await stream.next()
        if (step.done) return { end: step.value, stderr }
        if (step.value.stream === 'stderr') stderr += step.value.text
      }
    })()
    const stopped = new Promise<'stopped'>((resolve) => {
      if (signal.aborted) resolve('stopped')
      signal.addEventListener('abort', () => resolve('stopped'), { once: true })
    })
    const raced = await Promise.race([consumed, stopped])
    if (raced === 'stopped') {
      consumed.catch(() => {})
      return 'stopped'
    }
    const outcome = mapWaitExit(raced.end?.code ?? null, aborted)
    if (outcome === 'error') {
      debug(`ws mailbox: wait exited code=${raced.end?.code} signal=${raced.end?.signal} ${raced.stderr.trim().slice(0, 300)}`)
    }
    return outcome
  } finally {
    signal.removeEventListener('abort', onAbort)
  }
}

/** Interpret a $.tool.call result of the recv tool (pi's createBridgeDrain). */
export function recvResultToMail(res: ToolCallResult): MailboxEnvelope[] {
  if (res.deny !== undefined) throw new Error(`recv denied: ${res.deny}`)
  if (res.isError) throw new Error(`recv error: ${res.text ?? String(res.result)}`)
  if (typeof res.text === 'string') return parseRecvText(res.text)
  // Fallback if core gave no `text`: MCP-shaped content blocks in `result`.
  const content = (res.result as { content?: { type: string; text?: string }[] } | undefined)?.content
  return parseRecvText(content?.find((b) => b.type === 'text')?.text)
}

/** Main-loop-only guard and key validation for the bind tool. */
export function checkWatchCall(agentId: string | undefined, sessionKey: unknown): { key: string } | { deny: string } {
  if (agentId !== undefined) {
    return { deny: `${TOOL_NAME} binds the main session only; called from subagent ${agentId}.` }
  }
  const key = typeof sessionKey === 'string' ? sessionKey.trim() : ''
  if (!key) return { deny: `${TOOL_NAME}: session_key (non-empty string) is required.` }
  return { key }
}

export const register: Register = (on) => {
  let watcher: Watcher | undefined

  on('session.start', async ($, e, next) => {
    watcher?.stop('session.start again')
    watcher = createWatcher({
      wait: async (key, signal) => {
        if (signal.aborted) return 'stopped'
        const stream = $.process.spawn({ argv: waitArgv(key) })
        return consumeWait(stream, signal, (text) => $.ui.log(text, { to: 'debug' }))
      },
      drain: async (key) => recvResultToMail(await $.tool.call({ tool: RECV_TOOL, session_key: key, format: 'json' })),
      submit: async (text) => {
        await $.prompt.submit({ text })
      },
      sleep: (ms, signal) =>
        new Promise<void>((resolve) => {
          if (signal.aborted) return resolve()
          const timer = $.clock.after(ms, () => resolve())
          signal.addEventListener('abort', () => (timer.cancel(), resolve()), { once: true })
        }),
      status: (text) => $.ui.status(text),
      log: (text) => $.ui.log(text),
    })
    await $.tool.register({
      name: TOOL_NAME,
      description:
        'Bind ws cross-session mailbox active push to this session. Pass your own ws session_key. ' +
        'While bound, arriving mail is drained via mailbox.recv and injected as a new turn even when idle. ' +
        'Same key again is a no-op; a different key rebinds. Main loop only.',
      inputSchema: {
        type: 'object',
        properties: { session_key: { type: 'string', description: 'Your own ws session key.' } },
        required: ['session_key'],
      },
      isDeferred: false,
    })
    $.ui.status('ws mailbox: unbound')
    return next(e)
  })

  on('tool.call', { tool: 'mcp__ws-mailbox-push-probe__mailbox_watch' }, ($, e) => {
    const checked = checkWatchCall(e.agentId, e.session_key)
    if ('deny' in checked) return { deny: checked.deny }
    if (!watcher) return { deny: `${TOOL_NAME}: session not started yet.` }
    const outcome = watcher.bind(checked.key)
    $.ui.log(`ws mailbox: ${outcome} ${checked.key}`)
    return { result: `mailbox push ${outcome}: session_key=${checked.key}` }
  }).catch(() => ({ deny: `${TOOL_NAME}: the bind hook failed; see the debug log.` }))

  on('session.end', ($, e, next) => {
    watcher?.stop(`session.end ${e.reason}`)
    return next(e)
  })
}

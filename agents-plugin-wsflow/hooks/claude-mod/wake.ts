// Observable wake causes and background task identity. No hook guesses from a
// person's prose: origin wins even when the person pasted a notification.
export type Wake = {
  cause: 'subagent_report' | 'subagent_done' | 'subagent_failed' | 'background_exit' | 'monitor' | 'mail' | 'message' | 'scheduled'
  source: string
  caption: string
  text: string
}
export type WakePrompt = { text: string; isMeta?: boolean; origin?: { kind?: string; name?: string; subkind?: string; source?: string; server?: string } }
export type TaskKind = 'background' | 'monitors'
export const PERSON_ORIGINS = new Set(['composer', 'bridge', 'sdk', 'unclassified', 'slack-ping'])

/** The engine's task notification fields; not an XML parser for arbitrary prose. */
export function notification(text: string): { id?: string; callId?: string; status?: string; summary?: string; result?: string; type?: string } {
  const field = (name: string) => new RegExp(`<${name}>([\\s\\S]*?)</${name}>`, 'i').exec(text)?.[1]?.trim()
  return { id: field('task-id'), callId: field('tool-use-id'), status: field('status'), summary: field('summary'), result: field('result') ?? (/<summary>[\s\S]*?<\/summary>\s*([\s\S]*?)<\/task-notification>/i.exec(text)?.[1]?.trim() || undefined), type: field('task-type') }
}

export const terminalTask = (status: string | undefined) => /^(completed|done|failed|stopped|killed|aborted|cancelled|canceled|error)$/.test(status ?? '')
const oneLine = (text: string) => text.replace(/[\u0000-\u001f\u007f-\u009f\s]+/g, ' ').trim().slice(0, 200)

/** Known harness input only; unrecognised plugin/meta prompts stay unshown. */
export function wakePrompt(prompt: WakePrompt, plugin: string, taskKind?: TaskKind, agentKnown = false): Wake | undefined {
  const origin = prompt.origin
  const kind = origin?.kind
  if (PERSON_ORIGINS.has(kind ?? '') || prompt.text.trim() === '') return undefined
  const text = prompt.text
  const task = notification(text)
  const tag = /<task-notification[>\s]/i.test(text)
  const subkind = origin?.subkind ?? kind
  let cause: Wake['cause'] | undefined
  let source = (tag ? task.id : undefined) ?? origin?.name ?? origin?.server ?? origin?.source ?? ''
  if (subkind === 'scheduled-trigger' || subkind === 'scheduled-task' || kind === 'scheduler' || kind === 'scheduled') cause = 'scheduled'
  else if (taskKind === 'monitors' || /^Monitor event:/i.test(task.summary ?? '') || /monitor/i.test(task.type ?? '') || /<monitor(?:-event)?[>\s]/i.test(text) || kind === 'monitor') cause = 'monitor'
  else if (tag) {
    if (taskKind === 'background' || /bash|shell/i.test(task.type ?? '') || /^Background (?:command|task)/i.test(task.summary ?? '')) cause = 'background_exit'
    else if (agentKnown || /agent/i.test(task.type ?? '') || task.result !== undefined || /^Agent\b/i.test(task.summary ?? '')) {
      cause = /^(failed|error|stopped|killed|aborted|cancelled|canceled)$/.test(task.status ?? '') ? 'subagent_failed' : task.result ? 'subagent_report' : 'subagent_done'
    } else cause = 'message'
    source = task.id ?? source
  } else if (kind === 'mail' || (kind === 'plugin' && origin?.name === plugin && text.startsWith(`[${plugin} mailbox]`))) cause = 'mail'
  else if (['peer', 'coordinator', 'channel', 'session-inbox', 'peer-send-message', 'teammate-message', 'other-session'].includes(subkind ?? '') || /<teammate-message[>\s]/i.test(text)) cause = 'message'
  else if (kind === 'task-notification') cause = 'message'
  if (!cause) return undefined
  source ||= cause === 'background_exit' ? 'background shell' : cause === 'scheduled' ? 'scheduler' : cause === 'mail' ? 'mailbox' : cause === 'monitor' ? 'monitor' : 'message'
  const sender = /<teammate-message\b[^>]*teammate_id="([^"]+)"/i.exec(text)?.[1]
  return { cause, source: oneLine(sender ?? source), caption: oneLine(task.summary ?? text), text }
}

/** Only identities exposed by tool calls/results and notifications are tracked.
 * A reload cannot reconstruct tasks the engine never announced to this mod. */
export function createTasks() {
  type Task = { kind: TaskKind; agentId?: string; callId: string; id?: string; launching: boolean }
  const tasks = new Map<string, Task>()
  const calls = new Map<string, { tool: string; args: Record<string, unknown>; agentId?: string }>()
  const key = (agentId: string | undefined, id: string) => `${agentId ?? ''}/${id}`
  const find = (id: string | undefined) => id ? [...tasks.entries()].find(([, t]) => t.id === id || t.callId === id) : undefined
  const toolName = (tool: string) => tool.toLowerCase()
  const shell = (tool: string) => /^(bash|powershell)$/.test(toolName(tool))
  const monitor = (tool: string) => toolName(tool) === 'monitor'
  const counts = () => {
    const out = { background: 0, monitors: 0 }
    for (const task of tasks.values()) out[task.kind]++
    return out
  }
  return {
    counts,
    clear() { tasks.clear(); calls.clear() },
    kind(id?: string, callId?: string): TaskKind | undefined { return (find(id) ?? find(callId))?.[1].kind },
    began(agentId: string | undefined, id: string | undefined, tool: string, args: Record<string, unknown>) {
      if (!id || (!shell(tool) && !monitor(tool) && !/^(taskstop|killshell)$/.test(toolName(tool)))) return
      const k = key(agentId, id)
      calls.set(k, { tool, args, agentId })
      if ((shell(tool) && args.run_in_background === true) || monitor(tool)) tasks.set(k, { kind: monitor(tool) ? 'monitors' : 'background', agentId, callId: id, launching: true })
    },
    ended(agentId: string | undefined, id: string | undefined, failed: boolean, result?: unknown) {
      if (!id) return
      const k = key(agentId, id)
      const call = calls.get(k)
      calls.delete(k)
      if (!call) return
      if (failed) { tasks.delete(k); return }
      if (/^(taskstop|killshell)$/.test(toolName(call.tool))) {
        const target = call.args.task_id ?? call.args.shell_id
        const found = typeof target === 'string' ? find(target) : undefined
        if (found) tasks.delete(found[0])
        return
      }
      // The hook exposes native output as result; older engines expose just
      // text. Accept only named identity fields, never a random output word.
      const r = taskResult(result)
      const taskId = r?.backgroundTaskId ?? r?.taskId ?? r?.task_id ?? r?.shell_id ?? r?.monitor_id
      const task = tasks.get(k)
      if (task) { task.launching = false; if (typeof taskId === 'string') task.id = taskId }
      // Bash may automatically background a foreground call; count it only
      // when the returned native result explicitly says it did.
      else if (shell(call.tool) && typeof taskId === 'string') tasks.set(k, { kind: 'background', agentId, callId: id, id: taskId, launching: false })
    },
    notified(text: string) {
      const n = notification(text)
      const found = find(n.id) ?? find(n.callId)
      const monitorEnded = found?.[1].kind === 'monitors' && /\[Monitor (?:stopped|timed out|expired)\b/i.test(text)
      if (found && (terminalTask(n.status) || monitorEnded)) {
        tasks.delete(found[0])
        calls.delete(found[0])
      }
    },
    abort(agentId: string | undefined) {
      // A turn ending does not kill a successfully detached task. Only an
      // unfinished launch belongs to the aborted turn.
      for (const [k, t] of tasks) if (t.agentId === agentId && t.launching) tasks.delete(k)
      // Keep the in-flight call identity until its result: Bash can detach
      // on an interrupt, and a late backgroundTaskId means it is still alive.
    },
  }
}

function taskResult(result: unknown): Record<string, unknown> | undefined {
  if (!result || typeof result !== 'object') return undefined
  const r = result as Record<string, unknown>
  if (r.result && typeof r.result === 'object') return r.result as Record<string, unknown>
  if (typeof r.text === 'string') {
    try { const parsed = JSON.parse(r.text); if (parsed && typeof parsed === 'object') return parsed } catch { /* Native output need not be JSON. */ }
    const id = /(?:<backgroundTaskId>|background task (?:with )?ID[: ]+)([\w-]+)/i.exec(r.text)?.[1]
    if (id) return { backgroundTaskId: id }
  }
  return r
}

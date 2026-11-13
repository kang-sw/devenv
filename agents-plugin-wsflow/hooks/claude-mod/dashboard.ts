// Dashboard feed (behind claude.dashboard_enabled): the lead's and its
// subagents' live activity handed to the session's own MCP server through
// its adapter-only intake tool, which keeps the session's live snapshot and
// conversation ring for the dashboard. The mod never talks to the dashboard
// itself.
//
// What it sends, as the intake's events:
// - `reset` ahead of the first call after the feed starts, the conversation
//   restarts, or the session key changes, so state from an earlier process
//   or conversation never lingers, and ahead of a resync;
// - `turn_start` / `turn_end` around each main-thread turn, the start
//   carrying what started it: `terminal` (a prompt the person typed),
//   `whiteboard` (a prompt the mod submitted with the whiteboard label), or
//   `mail` (a prompt the mailbox push submitted);
// - the lead's batches (no agentId) and one `agent` event per subagent an
//   `agent.spawn` announced or the session's agent list (`$.agent.list()`)
//   names, keyed by the host's native agent id, with its label, parent,
//   status, and the ws session key a delegate passes to this plugin's tools;
//   the server shows that key's reply-id and never stores the key; a
//   subagent's event also carries, once known, its model and effort, its
//   current context size, and its output rate (see the subagent telemetry
//   section below), each left out while the hooks have not supplied it;
// - the lead's streamed reply text (the step's text so far) and each
//   finished reply (`final: true`);
// - each finished user message with its origin;
// - `permission` while a person-approved tool call is waiting on its
//   dialog, and its clear when the call ends;
// - `status`, the session's usage figures (context, usage limits, cost),
//   only while the latest intake result names it on its `accepts:` line: an
//   older ws-mcp skips the kind, and a "dashboard off" or error result names
//   nothing. The latest figures are held and sent again after a resync, since
//   `reset` clears them on the server;
// - `compact` around each compaction of the main conversation: `start` when
//   it begins, then `end` when it lands or `failed` when it fails or is
//   vetoed, with its trigger (`auto` or `manual`; none for a plugin's), gated
//   on the `accepts:` line like `status`. A compaction still running is held
//   and its start sent again after a resync;
// - `commands`, the session's slash command list (`$.command.list()`: name,
//   description, source), gated on the `accepts:` line like `status`: held,
//   sent when it changes, and sent again after a resync, since `reset` clears
//   it on the server; the page offers its command picker only while the
//   server holds a list;
// - `command`, a slash command the mod ran from a dashboard command entry:
//   its typed line and the output it showed as text (none for a command that
//   drew a terminal panel or prompted the model), gated on the same line.
//
// A batch is one model request (a `turn.step`) and the tool calls it made:
// the step streams as a generating view (thinking, replying, or a tool call
// whose arguments are streaming), a tool chunk announces its call in the
// step's batch at once (calls start while the step still streams), and the
// batch ends when the step has ended and every call has. A step with no tool
// call makes no batch, so its tokens are reported nowhere. A finished batch's
// view also carries its calls (tool name, start, end, failed, and the same
// capped argument preview the running line shows; never a result) and the
// request's token usage.
//
// Each finished batch reaches ws-mcp at least once: beyond the newest
// BATCH_CAP, a buffer keeps every finished batch no delivered intake call
// carried yet (up to BATCH_RETAIN_CAP) and lists it, and ws-mcp counts a
// batch once by its epoch and seq. So batches that finish faster than calls
// deliver (a slow call, a retry) are still counted.
//
// Permission waits: a tool.check that resolves `ask` for a person to answer
// marks the call as asked; it still shows only as a running tool. A
// `permission_prompt` notification while asks are pending switches every one
// of them to waiting for approval (the notification names no tool), and the
// call's tool.call end clears it, after the tool ran or was refused. An ask
// settled with no dialog gets no notification and never shows the marker.
//
// Subagents: an agent id seen first in a turn, step, or tool event is held
// out of the tree until it is known to be a real subagent: its `agent.spawn`
// announces it, or the agent list, asked once per held id, names it. An id
// the list does not name (an engine fork, a workflow's agent) stays held. At
// start the feed seeds the tree from the list, so a subagent spawned before a
// reload or a resume shows under its label and parent.
//
// The feed never affects the session: handlers only record state and arm a
// timer, and one flush at a time drains everything into a single intake
// call, at most once per FLUSH_INTERVAL_MS, so streamed text and batch
// changes coalesce; while streamed reply text is the only pending change the
// gap stretches to TEXT_FLUSH_INTERVAL_MS, and any discrete change cuts a
// waiting slow timer back to the fast cadence. Discrete events wait in a
// bounded list. A call that fails (rejects, answers an error, times out) puts
// its conversation events back, waits RETRY_MS, and resyncs: `reset`, the
// open turn's `turn_start`, and the full lead and subagent state. With no
// session key held the feed keeps what it has and sends when a key arrives.

import type { SessionUsage } from 'claude-code'

import { createTasks, notification, PERSON_ORIGINS, wakePrompt, type Wake, type WakePrompt } from './wake'

/** The intake tool, as the MCP server names it. */
export const INTAKE_TOOL = 'dashboard_intake'
/** The board-answer inbox's drain tool, as the MCP server names it: an atomic claim of every held message. */
export const INBOX_DRAIN_TOOL = 'dashboard_inbox_drain'
/** The adapter name the intake records. */
export const ADAPTER = 'claude'
/**
 * Minimum gap between two intake calls: at most one a second. Each call is an
 * MCP call from the mod, which Claude Code's tool hooks can observe, so a
 * tighter cadence fires them that much more often.
 */
export const FLUSH_INTERVAL_MS = 1000
/**
 * Minimum gap between two intake calls while the only pending change is
 * streamed reply text: at most one every two seconds. Streamed text made the
 * terminal flicker at a fast cadence, so it stays slower than discrete
 * changes; a discrete change keeps FLUSH_INTERVAL_MS and takes the waiting
 * text with it.
 */
export const TEXT_FLUSH_INTERVAL_MS = 2000
/** A call still unanswered after this is abandoned, so a hung server cannot stop the feed. */
export const CALL_TIMEOUT_MS = 5000
/** Wait after a call that was not delivered before the next one. */
export const RETRY_MS = 2000
/** Discrete events held between flushes; past it the oldest is dropped. */
export const PENDING_EVENT_CAP = 100
/** Characters of text sent: the intake keeps 64 KiB, and one more lets it mark the text truncated. */
export const TEXT_SEND_CAP = 64 * 1024 + 1
/** The batch model's caps: batches kept per agent, distinct tool names per batch, argument preview characters. */
export const BATCH_CAP = 5
export const TOOL_NAME_CAP = 5
export const ARGS_PREVIEW_CAP = 200
/** Batches a buffer keeps at most while finished ones wait for a delivered call; past it the oldest go uncounted. */
export const BATCH_RETAIN_CAP = 32
/** Calls a finished batch's view lists; the rest are only counted. */
export const BATCH_CALL_CAP = 32
/** Finished subagents kept beside every running one, as the snapshot keeps them. */
export const FINISHED_AGENT_CAP = 5
/** Agent ids neither an `agent.spawn` nor the agent list has announced yet, held out of the tree; past it the oldest held one goes. */
export const HELD_AGENT_CAP = 16
/** The notification type that confirms a permission dialog is open. */
export const PERMISSION_PROMPT = 'permission_prompt'

/** The label the mod puts as the first line of a whiteboard message it submits as the user's. */
export function whiteboardLabel(plugin: string): string {
  return `[user message from the ${plugin} whiteboard]`
}

/** `text` with the whiteboard label as its own first line. */
export function labelWhiteboard(plugin: string, text: string): string {
  return `${whiteboardLabel(plugin)}\n${text}`
}

/** The message under a whiteboard label, or undefined when `text` does not open with it. */
export function whiteboardBody(plugin: string, text: string): string | undefined {
  const label = whiteboardLabel(plugin)
  if (text === label) return ''
  return text.startsWith(`${label}\n`) ? text.slice(label.length + 1) : undefined
}

/** A slash command entry an inbox drain claimed: the command to run and its typed line (`/name args`). */
export type InboxCommand = { command: { name: string; args: string }; text: string }

/** One claimed inbox message: a text message's text, or a command entry, which is never joined with text. */
export type InboxItem = string | InboxCommand

/** The typed line of a slash command. */
export function commandLine(name: string, args: string): string {
  return args === '' ? `/${name}` : `/${name} ${args}`
}

/**
 * The messages an inbox drain's `format: "json"` text claimed, oldest first:
 * each text message's `text`, and each command entry (a record with a
 * `command: {name, args}` field) as an InboxCommand in its place. The drain
 * has already claimed whatever it returned, so text that is not that shape
 * comes back whole as one message, not dropped.
 */
export function parseInboxDrain(text: string): InboxItem[] {
  const trimmed = text.trim()
  if (trimmed === '') return []
  let messages: unknown
  try {
    messages = (JSON.parse(trimmed) as { messages?: unknown } | null)?.messages
  } catch {
    return [trimmed]
  }
  if (!Array.isArray(messages)) return [trimmed]
  return messages.flatMap((m): InboxItem[] => {
    const r = m as { text?: unknown; command?: { name?: unknown; args?: unknown } | null } | null
    const t = r?.text
    const name = r?.command?.name
    if (typeof name === 'string' && name.trim() !== '') {
      const rawArgs = r?.command?.args
      const args = typeof rawArgs === 'string' ? rawArgs : ''
      const line = typeof t === 'string' && t.trim() !== '' ? t : commandLine(name.trim(), args)
      return [{ command: { name: name.trim(), args }, text: line }]
    }
    if (typeof t === 'string') return t.trim() === '' ? [] : [t]
    return [JSON.stringify(m)]
  })
}

/**
 * The creation times (epoch ms) of the interrupt entries an `interrupt_only`
 * inbox drain's `format: "json"` text claimed. A record that is not an
 * interrupt entry with a numeric `sent_at`, and text of another shape, yield
 * nothing: without a time the entry cannot be checked against the running
 * turn's start, and an interrupt must never cut a turn it was not meant for.
 */
export function parseInterruptDrain(text: string): number[] {
  let messages: unknown
  try {
    messages = (JSON.parse(text.trim()) as { messages?: unknown } | null)?.messages
  } catch {
    return []
  }
  if (!Array.isArray(messages)) return []
  return messages.flatMap((m): number[] => {
    const r = m as { interrupt?: unknown; sent_at?: unknown } | null
    return r?.interrupt === true && typeof r.sent_at === 'number' && Number.isFinite(r.sent_at) ? [r.sent_at] : []
  })
}

export type Origin = 'terminal' | 'whiteboard' | 'mail'

/**
 * A submitted prompt as the feed records it: the person's own prompt is a
 * `terminal` user message; this plugin's prompt under the whiteboard label is
 * a `whiteboard` one, the label stripped; anything else (mail, the mod's
 * other prompts, notifications, peers) is no user message and undefined. A
 * prompt the person does not see as typed (isMeta) is none either.
 */
export function userPrompt(
  prompt: { text: string; isMeta?: boolean; origin?: { kind?: string; name?: string } },
  plugin: string,
): { text: string; origin: 'terminal' | 'whiteboard' } | undefined {
  if (prompt.isMeta) return undefined
  const kind = prompt.origin?.kind
  if (kind === 'plugin') {
    if (prompt.origin?.name !== plugin) return undefined
    const body = whiteboardBody(plugin, prompt.text)
    return body === undefined ? undefined : { text: body, origin: 'whiteboard' }
  }
  if (kind === undefined || !PERSON_ORIGINS.has(kind)) return undefined
  return prompt.text.trim() === '' ? undefined : { text: prompt.text, origin: 'terminal' }
}

/** An argument that carries a ws session key: never part of a preview. */
const SESSION_KEY_ARG = /^session_?key$/i

/**
 * Flattens a call's arguments to one capped line: `key: value` pairs, strings
 * raw, everything else JSON. A session key argument is left out, so a
 * preview never carries one (the server scrubs keys it knows as well).
 */
export function argsPreview(args: unknown, cap = ARGS_PREVIEW_CAP): string {
  let text: string
  if (args === undefined || args === null) text = ''
  else if (typeof args === 'string') text = args
  else if (typeof args === 'object' && !Array.isArray(args)) {
    text = Object.entries(args as Record<string, unknown>)
      .filter(([key]) => !SESSION_KEY_ARG.test(key))
      .map(([key, value]) => `${key}: ${typeof value === 'string' ? value : safeJson(value)}`)
      .join(', ')
  } else text = safeJson(args)
  const flat = text.replace(/[\u0000-\u0008\u000e-\u001f\u007f-\u009f]/g, '').replace(/\s+/g, ' ').trim()
  const chars = [...flat]
  return chars.length <= cap ? flat : chars.slice(0, cap - 1).join('') + '…'
}

function safeJson(value: unknown): string {
  try {
    return JSON.stringify(value) ?? String(value)
  } catch {
    return String(value)
  }
}

/** The keys a tool.call input carries beside the tool's own arguments. */
const RESERVED_CALL_KEYS = new Set(['tool', 'tool_use_id', 'consent', 'agentId', 'requestMeta'])

/** A tool.call input's own arguments, without the envelope keys. */
export function callArgs(input: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(input)) if (!RESERVED_CALL_KEYS.has(key)) out[key] = value
  return out
}

function cleanToolName(name: string): string {
  return name.replace(/[\u0000-\u001f\u007f-\u009f]/g, ' ').trim() || 'tool'
}

// ---- The batch model ------------------------------------------------------

type Call = { id: string; toolName: string; argsPreview: string; startedAt?: number; endedAt?: number; isError?: boolean }
type Batch = { seq: number; step: string; startedAt: number; endedAt?: number; calls: Call[]; usage?: TokenUsage; delivered?: boolean }

/** A model request's token usage as the intake takes it. */
export type TokenUsage = { input: number; output: number; cacheRead: number; cacheWrite: number }
type GeneratingKind = 'thinking' | 'replying' | 'tool'
type Generating = { startedAt: number; kind: GeneratingKind; phaseStartedAt: number; toolName?: string; chars?: number }

/** One agent's bounded batch history; only the newest batch may be open. */
export type Buffer = { epoch: string; nextSeq: number; batches: Batch[]; generating?: Generating; streaming?: string }

/** A batch as the intake takes it: exactly one of endedAt, running, and generating. */
export type BatchView = {
  epoch: string
  seq: number
  toolNames: string[]
  callCount: number
  failed: number
  startedAt: number
  endedAt?: number
  running?: { toolName: string; argsPreview: string; startedAt: number; done: number; othersRunning: number }
  generating?: { kind: GeneratingKind; phaseStartedAt: number; toolName?: string; chars?: number }
  /** A finished batch's calls, up to BATCH_CALL_CAP: names, clocks, and the capped argument preview when there is one. */
  calls?: { name: string; startedAt: number; endedAt?: number; failed: boolean; argsPreview?: string }[]
  /** A finished batch's request tokens, when the step reported any. */
  usage?: TokenUsage
}

/** A step result's usage as a TokenUsage; undefined when absent or all zero. */
export function stepTokens(u: { input_tokens?: number; output_tokens?: number; cache_read_input_tokens?: number; cache_creation_input_tokens?: number } | null | undefined): TokenUsage | undefined {
  if (!u) return undefined
  const n = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? Math.round(v) : 0)
  const t = { input: n(u.input_tokens), output: n(u.output_tokens), cacheRead: n(u.cache_read_input_tokens), cacheWrite: n(u.cache_creation_input_tokens) }
  return t.input || t.output || t.cacheRead || t.cacheWrite ? t : undefined
}

export function createBuffer(epoch: string): Buffer {
  return { epoch, nextSeq: 1, batches: [] }
}

function openBatch(buf: Buffer): Batch | undefined {
  const last = buf.batches[buf.batches.length - 1]
  return last && last.endedAt === undefined ? last : undefined
}

function startBatch(buf: Buffer, step: string, now: number): Batch {
  const batch: Batch = { seq: buf.nextSeq++, step, startedAt: now, calls: [] }
  buf.batches.push(batch)
  trimBatches(buf)
  return batch
}

/** Keeps the newest BATCH_CAP batches and, older than those, the ones not yet delivered, up to BATCH_RETAIN_CAP in all. */
function trimBatches(buf: Buffer): void {
  const n = buf.batches.length
  let drop = 0
  while (n - drop > BATCH_CAP && (buf.batches[drop]!.delivered === true || n - drop > BATCH_RETAIN_CAP)) drop++
  if (drop > 0) buf.batches.splice(0, drop)
}

/**
 * A delivered intake call carried `views`: the finished batches among them
 * are delivered, and those older than the newest BATCH_CAP leave the buffer.
 */
export function markDelivered(buf: Buffer, views: readonly { epoch?: unknown; seq?: unknown; endedAt?: unknown }[]): void {
  for (const view of views) {
    if (view.epoch !== buf.epoch || view.endedAt === undefined) continue
    const batch = buf.batches.find((b) => b.seq === view.seq)
    if (batch?.endedAt !== undefined) batch.delivered = true
  }
  trimBatches(buf)
}

/** The open batch of `step`, ending an open batch of an earlier step first. */
function stepBatch(buf: Buffer, step: string, now: number): Batch {
  const open = openBatch(buf)
  if (open?.step === step) return open
  if (open) open.endedAt = now
  return startBatch(buf, step, now)
}

function settleBatch(buf: Buffer, batch: Batch, now: number): void {
  if (batch.endedAt === undefined && batch.step !== buf.streaming && batch.calls.every((c) => c.endedAt !== undefined)) batch.endedAt = now
}

/** A model request begins: its generating view opens, thinking until a block says otherwise. */
export function stepBegan(buf: Buffer, step: string, now: number): void {
  const open = openBatch(buf)
  if (open && open.step !== step) open.endedAt = now
  buf.streaming = step
  buf.generating = { startedAt: now, kind: 'thinking', phaseStartedAt: now }
}

/** One streamed chunk of the request `step`. Returns whether the buffer changed. */
export function stepChunk(buf: Buffer, step: string, chunk: { kind: string; id?: string; name?: string; json?: string }, now: number): boolean {
  const gen = buf.generating
  if (!gen || buf.streaming !== step) return false
  switch (chunk.kind) {
    case 'thinking':
    case 'text': {
      const kind: GeneratingKind = chunk.kind === 'thinking' ? 'thinking' : 'replying'
      if (gen.kind === kind) return false
      buf.generating = { startedAt: gen.startedAt, kind, phaseStartedAt: now }
      return true
    }
    case 'tool': {
      const toolName = cleanToolName(typeof chunk.name === 'string' ? chunk.name : '')
      buf.generating = { startedAt: gen.startedAt, kind: 'tool', phaseStartedAt: now, toolName, chars: 0 }
      const batch = stepBatch(buf, step, now)
      const id = typeof chunk.id === 'string' ? chunk.id : ''
      if (!id || !batch.calls.some((c) => c.id === id)) batch.calls.push({ id, toolName, argsPreview: '' })
      return true
    }
    case 'input':
      if (gen.kind !== 'tool' || typeof chunk.json !== 'string' || chunk.json.length === 0) return false
      gen.chars = (gen.chars ?? 0) + chunk.json.length
      return true
    default:
      return false
  }
}

/** The request `step` ended: its generating view goes, and its batch takes the calls' arguments and the request's tokens. */
export function stepEnded(buf: Buffer, step: string, toolUses: readonly { name: string; input: unknown }[], now: number, usage?: TokenUsage): boolean {
  const had = buf.generating !== undefined
  if (buf.streaming === step) {
    buf.streaming = undefined
    buf.generating = undefined
  }
  let batch = openBatch(buf)
  if (batch?.step !== step) batch = undefined
  if (!batch) {
    if (toolUses.length === 0) return had
    batch = stepBatch(buf, step, now)
  }
  toolUses.forEach((use, i) => {
    const call = batch.calls[i]
    if (!call) batch.calls.push({ id: '', toolName: cleanToolName(use.name), argsPreview: argsPreview(use.input) })
    else if (call.argsPreview === '') call.argsPreview = argsPreview(use.input)
  })
  if (usage) batch.usage = usage
  settleBatch(buf, batch, now)
  return true
}

/** A tool call starts running: the call its step announced, or one that joins (or opens) a batch. */
export function toolBegan(buf: Buffer, id: string, tool: string, args: unknown, now: number): void {
  let batch = openBatch(buf)
  let call = batch?.calls.find((c) => c.id === id && id !== '' && c.startedAt === undefined)
  if (!call) {
    batch ??= startBatch(buf, buf.streaming ?? '', now)
    call = { id, toolName: cleanToolName(tool), argsPreview: '' }
    batch.calls.push(call)
  }
  const preview = argsPreview(args)
  if (preview !== '') call.argsPreview = preview
  call.startedAt = now
}

/** A tool call finished; the batch ends once its step ended and every call has. Returns whether it changed. */
export function toolEnded(buf: Buffer, id: string, isError: boolean, now: number): boolean {
  const batch = openBatch(buf)
  const call = batch?.calls.find((c) => c.id === id && c.endedAt === undefined)
  if (!batch || !call) return false
  call.startedAt ??= now
  call.endedAt = now
  call.isError = isError
  settleBatch(buf, batch, now)
  return true
}

/** The turn or the agent ended: the open batch ends (unfinished calls count as failed) and the generating view goes. */
export function closeBuffer(buf: Buffer, now: number): boolean {
  const had = buf.generating !== undefined
  buf.generating = undefined
  buf.streaming = undefined
  const open = openBatch(buf)
  if (!open) return had
  open.endedAt = now
  return true
}

/** The buffer's views, oldest first; a generating request is last. */
export function batchViews(buf: Buffer): BatchView[] {
  const views = buf.batches.map((batch): BatchView => {
    const toolNames: string[] = []
    for (const call of batch.calls) {
      if (toolNames.length >= TOOL_NAME_CAP) break
      if (!toolNames.includes(call.toolName)) toolNames.push(call.toolName)
    }
    const failed = batch.calls.filter((c) => c.isError === true || (batch.endedAt !== undefined && c.endedAt === undefined)).length
    const view: BatchView = { epoch: buf.epoch, seq: batch.seq, toolNames, callCount: batch.calls.length, failed, startedAt: batch.startedAt }
    if (batch.endedAt !== undefined) {
      view.endedAt = Math.max(batch.endedAt, 1)
      view.calls = batch.calls.slice(0, BATCH_CALL_CAP).map((c) => ({
        name: c.toolName,
        startedAt: c.startedAt ?? batch.startedAt,
        ...(c.endedAt !== undefined ? { endedAt: c.endedAt } : {}),
        failed: c.isError === true || c.endedAt === undefined,
        ...(c.argsPreview !== '' ? { argsPreview: c.argsPreview } : {}),
      }))
      if (batch.usage) view.usage = { ...batch.usage }
      return view
    }
    const done = batch.calls.filter((c) => c.endedAt !== undefined).length
    const inFlight = batch.calls.filter((c) => c.startedAt !== undefined && c.endedAt === undefined)
    let reported: Call | undefined
    for (const c of inFlight) if (!reported || c.startedAt! < reported.startedAt!) reported = c
    reported ??= batch.calls.find((c) => c.endedAt === undefined)
    view.running = {
      toolName: reported?.toolName ?? 'tool',
      argsPreview: reported?.argsPreview ?? '',
      startedAt: reported?.startedAt ?? batch.startedAt,
      done,
      othersRunning: Math.max(0, inFlight.length - (reported?.startedAt !== undefined ? 1 : 0)),
    }
    return view
  })
  const gen = buf.generating
  if (gen) {
    const generating: NonNullable<BatchView['generating']> = { kind: gen.kind, phaseStartedAt: gen.phaseStartedAt }
    if (gen.kind === 'tool') {
      generating.toolName = gen.toolName ?? 'tool'
      generating.chars = gen.chars ?? 0
    }
    views.push({ epoch: buf.epoch, seq: buf.nextSeq, toolNames: [], callCount: 0, failed: 0, startedAt: gen.startedAt, generating })
  }
  return views
}

// ---- Subagent telemetry ----------------------------------------------------

/** Model requests a subagent's output rate is token-weighted over. */
export const OUTPUT_RATE_WINDOW = 8
/** A request whose summed block spans are shorter is burst-flushed and would inflate the rate. */
export const OUTPUT_RATE_MIN_SPAN_MS = 250
/** A request with fewer output tokens is too small to measure. */
export const OUTPUT_RATE_MIN_TOKENS = 8

/** An effort-carrier agent type: one whose name (after any `<plugin>:` prefix) is `effort-<level>`. */
const EFFORT_CARRIER_TYPE = /(^|:)effort-(low|medium|high|xhigh|max)$/

/** The effort an effort-carrier agent type fixes, or undefined for any other type. */
export function effortOfType(type: string | undefined): string | undefined {
  return type === undefined ? undefined : EFFORT_CARRIER_TYPE.exec(type.trim())?.[2]
}

/** A request's effort as the intake takes it: a level as named, a number as its text; undefined when absent. */
export function effortText(effort: unknown): string | undefined {
  if (typeof effort === 'number') return Number.isFinite(effort) ? String(effort) : undefined
  return typeof effort === 'string' && effort.trim() !== '' ? effort.trim() : undefined
}

/** A request's context size: its uncached input plus cache reads and cache writes (output not included). */
export function contextTokensOf(t: TokenUsage): number {
  return t.input + t.cacheRead + t.cacheWrite
}

/**
 * One agent's output decode rate (tokens per second), following the Pi
 * adapter's output-rate tracker on its branch for usage without a reasoning
 * breakdown (Claude reports none): per model request, every output token over the
 * summed span of every content block (text, thinking, a tool call with its
 * argument chunks), so latency before the first block and gaps between blocks
 * never count; token-weighted over the last OUTPUT_RATE_WINDOW eligible
 * requests, the window starting over when the answering model changes.
 *
 * The hooks stream chunks but no block start or end, so a block's span is
 * estimated from its first and last chunk's arrival. That leaves out the
 * first chunk's own decode time, and a block that arrives as one chunk spans
 * nothing, so this rate reads somewhat higher than a tracker that times real
 * block edges.
 */
export interface OutputRate {
  /** A model request begins: the spans of an earlier one that never finished are dropped. */
  begin: () => void
  /** A chunk of block `index` arrived at `now`. */
  chunk: (index: number, now: number) => void
  /** The request ended with `outputTokens` (undefined when it reported no usage), answered by `model`. */
  finish: (outputTokens: number | undefined, model: string | undefined) => void
  /** Tokens per second over the window, or undefined before the first eligible request. */
  rate: () => number | undefined
}

export function createOutputRate(): OutputRate {
  let blocks = new Map<number, { first: number; last: number }>()
  let window: { tokens: number; ms: number }[] = []
  let windowModel: string | undefined
  return {
    begin() {
      blocks = new Map()
    },
    chunk(index, now) {
      const block = blocks.get(index)
      if (block) block.last = Math.max(block.last, now)
      else blocks.set(index, { first: now, last: now })
    },
    finish(outputTokens, model) {
      let ms = 0
      for (const block of blocks.values()) ms += block.last - block.first
      blocks = new Map()
      if (outputTokens === undefined || !Number.isFinite(outputTokens)) return
      if (ms < OUTPUT_RATE_MIN_SPAN_MS || outputTokens < OUTPUT_RATE_MIN_TOKENS) return
      const answered = model ?? ''
      if (answered !== windowModel) {
        window = []
        windowModel = answered
      }
      window.push({ tokens: outputTokens, ms })
      if (window.length > OUTPUT_RATE_WINDOW) window.shift()
    },
    rate() {
      if (window.length === 0) return undefined
      let tokens = 0
      let ms = 0
      for (const sample of window) {
        tokens += sample.tokens
        ms += sample.ms
      }
      return ms > 0 ? tokens / (ms / 1000) : undefined
    },
  }
}

/** The chunk kinds that belong to a content block by `index`. */
const BLOCK_CHUNKS = new Set(['text', 'thinking', 'tool', 'input'])

// ---- Usage status ----------------------------------------------------------

/** The optional event kind that carries usage figures, sent only while the intake names it. */
export const STATUS_KIND = 'status'

/** The optional event kinds an intake result's `accepts:` line names; none without the line. */
export function acceptedKinds(text: string): string[] {
  const line = /^accepts:(.*)$/m.exec(text)
  return line ? line[1]!.split(/[\s,]+/).filter((k) => k !== '') : []
}

/** A `status` event's figures; a part with no figure is left out. */
export type StatusFigures = {
  context?: { tokens: number | null; window: number; percent?: number; cacheHit?: number }
  limits?: { kind: 'five_hour' | 'seven_day'; percent: number; resetsAt?: number }[]
  cost?: { usd: number; estimate: false; incomplete: false }
}

/**
 * The status figures of a `$.session.usage({ breakdown: 'summary' })` read.
 * The window and percent are the terminal band's (the compaction window, else
 * the model's), so the page tracks the host's own line. The cache hit rate is
 * the last response's cache reads over all its input (uncached, read,
 * written), not the band's reads-over-cached figure.
 */
export function statusFigures(u: SessionUsage): StatusFigures {
  const out: StatusFigures = {}
  const raw = u.context.breakdown?.rawMaxTokens
  const window = raw !== undefined && raw > 0 ? raw : u.context.window
  const tokens = u.context.tokens ?? null
  if (window > 0 || tokens !== null) {
    const context: NonNullable<StatusFigures['context']> = { tokens, window: Math.max(0, window) }
    if (tokens !== null && window > 0) context.percent = (tokens / window) * 100
    const api = u.context.breakdown?.apiUsage
    const input = api ? api.input_tokens + api.cache_read_input_tokens + api.cache_creation_input_tokens : 0
    if (api && input > 0) context.cacheHit = (api.cache_read_input_tokens / input) * 100
    out.context = context
  }
  const limits: NonNullable<StatusFigures['limits']> = []
  for (const kind of ['five_hour', 'seven_day'] as const) {
    const r = u.rateLimits.find((l) => l.kind === kind)
    if (!r || !Number.isFinite(r.percentUsed) || r.percentUsed < 0) continue
    const resetsAt = r.resetsAt === undefined ? NaN : Date.parse(r.resetsAt)
    limits.push({ kind, percent: r.percentUsed, ...(Number.isFinite(resetsAt) ? { resetsAt } : {}) })
  }
  if (limits.length > 0) out.limits = limits
  if (u.cost && Number.isFinite(u.cost.usd) && u.cost.usd >= 0) out.cost = { usd: u.cost.usd, estimate: false, incomplete: false }
  return out
}

// ---- Compaction ------------------------------------------------------------

/** The optional event kind that marks a main-conversation compaction, sent only while the intake names it. */
export const COMPACT_KIND = 'compact'

/** A compaction's trigger as the intake takes it; a plugin's compaction carries none. */
export type CompactTrigger = 'auto' | 'manual'

// ---- Slash commands --------------------------------------------------------

/** The optional event kind that carries the session's slash command list, sent only while the intake names it. */
export const COMMANDS_KIND = 'commands'

/** The optional event kind that records a slash command the mod ran from a dashboard command entry, sent only while the intake names it. */
export const COMMAND_KIND = 'command'

/** The most commands a list carries, as the server keeps them. */
export const COMMAND_LIST_CAP = 500

/** One slash command as a `commands` event carries it. */
export type ListedCommand = { name: string; description?: string; source?: string }

/**
 * A `$.command.list()` read as a `commands` event's list: each command's
 * name, description, and source, in the host's order, capped at
 * COMMAND_LIST_CAP; a record without a name is left out. The listing does not
 * tell user-only from agent-only commands, so no layer is sent.
 */
export function commandList(list: readonly { name?: unknown; description?: unknown; source?: unknown }[]): ListedCommand[] {
  const out: ListedCommand[] = []
  for (const c of list) {
    if (out.length === COMMAND_LIST_CAP) break
    if (typeof c?.name !== 'string' || c.name.trim() === '') continue
    out.push({
      name: c.name,
      ...(typeof c.description === 'string' && c.description !== '' ? { description: c.description } : {}),
      ...(typeof c.source === 'string' && c.source !== '' ? { source: c.source } : {}),
    })
  }
  return out
}

// ---- The feed --------------------------------------------------------------

export type DashboardEvent = {
  kind: 'reset' | 'turn_start' | 'turn_end' | 'agent' | 'permission' | 'reply' | 'user' | 'status' | 'compact' | 'commands' | 'command' | 'wake'
  [field: string]: unknown
}

/** One intake call's outcome: whether it was delivered (not an error result) and its result text. */
export type IntakeResult = { ok: boolean; text: string }

/** One agent of the session's agent list, as the feed reads `$.agent.list()`'s records. */
export type ListedAgent = { id: string; description?: string; type?: string; status?: string; parentId?: string }

/** A step result's usage: the token counts and the model that answered. */
export type StepUsage = (NonNullable<Parameters<typeof stepTokens>[0]> & { model?: unknown }) | null | undefined

export interface DashboardEdges {
  /** The session key the feed reports under: the conversation's lead key, else undefined (held until one arrives). */
  key: () => string | undefined
  /** One intake call; resolves with its outcome and result text, rejects on transport failure. */
  call: (args: { session_key: string; adapter: string; events: DashboardEvent[] }) => Promise<IntakeResult>
  /** Whether a tool is one of this plugin's MCP server tools, whose `session_key` names a delegate's key. */
  ownTool: (tool: string) => boolean
  /** The session's real subagents and teammates (`$.agent.list()`); rejects when the list cannot be read. */
  agents: () => Promise<readonly ListedAgent[]>
  now: () => number
  /** Arms a timer and returns its cancel. */
  schedule: (ms: number, fn: () => void) => () => void
  /** A fresh epoch for a batch buffer. */
  epoch: () => string
  log: (text: string) => void
}

type Ask = { agentId: string; tool: string; preview: string; marked: boolean }

type AgentRec = {
  buf: Buffer
  dirty: boolean
  label?: string
  caption?: string
  parentId?: string
  sessionKey?: string
  status: 'running' | 'done' | 'failed'
  endedAt?: number
  /** The permission wait last sent, as JSON; '' for none. */
  permissionSent: string
  /** Whether the record is in the tree: the lead always, a subagent once its `agent.spawn` or the agent list named it. */
  announced: boolean
  /** A subagent's model and effort: its latest request's, seeded by its spawn before the first. */
  model?: string
  effort?: string
  /** A subagent's context size as its latest request that reported usage left it. */
  contextTokens?: number
  /** A subagent's output rate tracker; the lead keeps none. */
  rate?: OutputRate
}

/** Agent list statuses seeded into the tree at start: a loop running a turn or held on its own work. */
const LIVE_LISTED = new Set(['running', 'waiting'])
/** The agent list status of a teammate between turns: seeded as a finished turn, as its turn end would leave it. */
const IDLE_LISTED = 'idle'

export interface Dashboard {
  /** Whether the feed runs: started and not stopped. */
  active: () => boolean
  /** Clears all state and queues `reset`; the feed runs from here. */
  start: () => void
  /** Cancels timers and drops everything; a stopped feed records and sends nothing. */
  stop: () => void
  /** The conversation ended (a clear, a resume): all state goes and `reset` is queued. */
  restart: () => void
  /** The session key may have changed: a new key gets a resync. */
  keyChanged: () => void
  /** The mailbox push is about to submit a prompt: an idle lead's next turn is mail-started. */
  mailSubmitting: () => void
  /** That submit did not enter: a mail mark it left unused goes. */
  mailSubmitFailed: () => void
  /**
   * The mod is about to submit a dashboard message as the user's (its own
   * prompt.submit hook never sees it): an idle lead's next turn is
   * whiteboard-started.
   */
  whiteboardSubmitting: () => void
  /**
   * That submit settled: entered, `text` is a whiteboard user message (empty
   * when every message in it was recorded already); not, its mark goes.
   */
  whiteboardSubmitted: (text: string, entered: boolean) => void
  /**
   * A dashboard message was appended into the conversation as a row (a
   * message promoted to steering, or a dropped submit's fallback; no
   * prompt.submit hook sees it): a whiteboard user message.
   */
  whiteboardSteered: (text: string) => void
  /** A prompt is being submitted; `startsTurn` when it starts a turn rather than joining a running one. */
  promptSubmitting: (origin: Origin | undefined, startsTurn: boolean) => void
  /** A submitted prompt entered: a user message with its origin. */
  userMessage: (text: string, origin: 'terminal' | 'whiteboard') => void
  /** A non-person prompt that entered; task notifications also settle observable background tasks. */
  wakeSubmitted: (prompt: WakePrompt, plugin: string) => void
  /** Mail delivered by this mod bypasses its own prompt.submit hook. */
  mailDelivered: (text: string) => void
  turnStarted: (agentId: string | undefined) => void
  turnCompleted: (agentId: string | undefined, answer: string, reason: string) => void
  /** A model request begins; `request` is the model and effort it names, which a subagent's record takes. */
  stepBegan: (agentId: string | undefined, step: string, request?: { model?: string; effort?: string | number }) => void
  stepChunk: (agentId: string | undefined, step: string, chunk: { kind: string; index?: number; text?: string; id?: string; name?: string; json?: string }) => void
  stepEnded: (
    agentId: string | undefined,
    step: string,
    result: { answer?: string; toolUses?: readonly { name: string; input: unknown }[]; usage?: StepUsage },
  ) => void
  toolBegan: (agentId: string | undefined, id: string | undefined, tool: string, args: Record<string, unknown>) => void
  toolEnded: (agentId: string | undefined, id: string | undefined, isError: boolean, result?: unknown) => void
  toolChecked: (check: { agentId?: string; id?: string; tool: string; input: unknown; approver?: string }, decision: string) => void
  notified: (type: string) => void
  /** A subagent started; `model` is what its spawn resolved it to run on. */
  agentSpawned: (spawn: { agentId: string; parentId?: string; description?: string; subagentType?: string; model?: string }) => void
  /** The session's latest usage figures: held, and sent while the intake accepts `status`. */
  status: (figures: StatusFigures) => void
  /** A main-conversation compaction begins: held while it runs, and its start sent while the intake accepts `compact`. */
  compactStarted: (trigger: CompactTrigger | undefined) => void
  /** That compaction settled: `landed`, else it failed or was vetoed. */
  compactEnded: (trigger: CompactTrigger | undefined, landed: boolean) => void
  /** The session's slash command list: held, and sent while the intake accepts `commands` and the list changed or a resync is due. */
  commands: (list: readonly ListedCommand[]) => void
  /**
   * A slash command from a dashboard command entry ran (or was refused):
   * its typed line and the output it showed as text, undefined for none;
   * sent while the intake accepts `command`.
   */
  commandRan: (line: string, text: string | undefined) => void
}

const LEAD = ''

export function createDashboard(edges: DashboardEdges): Dashboard {
  let stopped = true
  let pending: DashboardEvent[] = []
  let agents = new Map<string, AgentRec>()
  let asks = new Map<string, Ask>()
  /** The lead's streamed reply: text per block of the streaming step. */
  let reply: { step: string; blocks: Map<number, string> } | undefined
  let replyDirty = false
  let lastReplySent: string | undefined
  /** The last finished reply sent this turn, so turn.complete's answer is not sent twice. */
  let lastFinal: string | undefined
  let turnOpen = false
  let nextOrigin: Origin | undefined
  /**
   * A whiteboard submit of the mod's own in flight, and whether a hook already
   * recorded its user message, so it never enters the ring twice.
   */
  let ownWhiteboard: { recorded: boolean } | undefined
  let lastTurnStart: DashboardEvent | undefined
  let resyncDue = false
  let sentKey: string | undefined
  let retryAt = 0
  let inFlight = false
  let lastFlushAt: number | undefined
  let cancelFlush: (() => void) | undefined
  /** Whether the armed flush timer waits on the slow text cadence. */
  let slowArmed = false
  /** Bumped by every reset, so a call answered after a stop or a restart changes nothing. */
  let generation = 0
  /** The latest usage figures, as JSON; undefined before any. */
  let status: string | undefined
  /** Whether `status` differs from what ws-mcp holds. */
  let statusDirty = false
  /** Whether the latest intake result named `status` among the kinds it accepts. */
  let statusAccepted = false
  /** The main conversation's compaction while it runs: its trigger and start. */
  let compacting: { trigger?: CompactTrigger; at: number } | undefined
  /** Whether ws-mcp does not hold `compacting`'s start yet. */
  let compactDirty = false
  /** Whether the latest intake result named `compact` among the kinds it accepts. */
  let compactAccepted = false
  /** The latest slash command list, as JSON; undefined before any. */
  let commands: string | undefined
  /** Whether `commands` differs from what ws-mcp holds. */
  let commandsDirty = false
  /** Whether the latest intake result named `commands`, and `command`, among the kinds it accepts. */
  let commandsAccepted = false
  let commandAccepted = false
  let wakeAccepted = false
  let countsAccepted = false
  let countsDirty = true
  const tasks = createTasks()

  const lead = (): AgentRec => rec(LEAD)

  // A subagent shows only once `agent.spawn` or the agent list announced it.
  // Any other agent id (an engine fork or a workflow's agent, which sends a
  // tool call or a turn start but never a spawn or an end, and which no list
  // names) would otherwise stay "running" in the tree forever. The engine may
  // deliver a real subagent's turn start before its spawn resolves, so an
  // unannounced id's record is held, out of every event, until the list
  // lookup or the spawn shows it with what it gathered; at most
  // HELD_AGENT_CAP are held. The start's seed and the lookup cover a subagent
  // spawned before the feed (re)started, which no spawn will announce; the
  // remaining gap is a list that cannot be read, which leaves such a subagent
  // unshown.
  function rec(id: string): AgentRec {
    return agents.get(id) ?? addRec(id, id === LEAD)
  }

  /** A new record; a held one evicts the oldest held past the cap and asks the agent list about itself. */
  function addRec(id: string, announced: boolean): AgentRec {
    const r: AgentRec = { buf: createBuffer(edges.epoch()), dirty: true, status: 'running', permissionSent: '', announced }
    agents.set(id, r)
    if (!announced) {
      dropOldestHeld()
      lookUp(id)
    }
    return r
  }

  /** Puts a subagent in the tree with its parent, label, and caption: from its spawn or its agent list record. */
  function announce(id: string, f: { parentId?: string; description?: string; type?: string }): AgentRec {
    const r = agents.get(id) ?? addRec(id, true)
    r.announced = true
    if (f.parentId && f.parentId !== id) r.parentId = f.parentId
    const label = f.description?.trim() || f.type?.trim()
    if (label) r.label = label
    if (f.type?.trim()) r.caption = f.type.trim()
    markDirty(id)
    return r
  }

  /**
   * Reads the agent list, then hands it to `use` while the same feed
   * generation runs; a list that rejects is logged and changes nothing.
   */
  function readList(what: string, use: (list: readonly ListedAgent[]) => void): void {
    const callGeneration = generation
    let run: Promise<readonly ListedAgent[]>
    try {
      run = edges.agents()
    } catch (err) {
      run = Promise.reject(err)
    }
    run.then(
      (list) => {
        if (stopped || callGeneration !== generation || !Array.isArray(list)) return
        try {
          use(list)
        } catch (err) {
          edges.log(`dashboard ${what} failed: ${String(err)}`)
        }
      },
      (err) => edges.log(`dashboard ${what}: agent list failed: ${String(err)}`),
    )
  }

  /** Asks the agent list about a held id, once per record: listed, it is announced; otherwise it stays held. */
  function lookUp(id: string): void {
    readList('agent lookup', (list) => {
      const r = agents.get(id)
      const info = list.find((a) => a?.id === id)
      if (r && !r.announced && info) announce(id, info)
    })
  }

  /** Seeds the tree from the agent list: every live subagent, and every idle teammate as a finished turn. */
  function seed(): void {
    readList('agent seed', (list) => {
      for (const info of list) {
        if (typeof info?.id !== 'string' || info.id === LEAD) continue
        const idle = info.status === IDLE_LISTED
        if (!idle && !LIVE_LISTED.has(info.status ?? '')) continue
        const fresh = !agents.has(info.id)
        const r = announce(info.id, info)
        if (fresh && idle) {
          r.status = 'done'
          r.endedAt = edges.now()
        }
      }
      pruneFinished()
    })
  }

  function dropOldestHeld(): void {
    const held = [...agents.entries()].filter(([, r]) => !r.announced)
    for (const [id] of held.slice(0, Math.max(0, held.length - HELD_AGENT_CAP))) {
      agents.delete(id)
      clearAsksOf(id)
    }
  }

  /** Whether an agent's record has state to send: dirty and in the tree. */
  const due = (r: AgentRec) => r.dirty && r.announced
  const anyDue = () => [...agents.values()].some(due)

  const idOf = (agentId: string | undefined) => agentId ?? LEAD

  const statusDue = () => statusAccepted && ((statusDirty && status !== undefined) || (countsAccepted && countsDirty))
  const compactDue = () => compactDirty && compactAccepted && compacting !== undefined
  const commandsDue = () => commandsDirty && commandsAccepted && commands !== undefined

  function hasWork(): boolean {
    return pending.length > 0 || replyDirty || statusDue() || compactDue() || commandsDue() || anyDue()
  }

  /** Whether streamed reply text is the only pending change, so the slow cadence applies. */
  function textOnly(): boolean {
    return replyDirty && pending.length === 0 && !statusDue() && !compactDue() && !commandsDue() && !anyDue()
  }

  function scheduleFlush(): void {
    if (stopped || inFlight || !hasWork()) return
    const slow = textOnly()
    // A discrete change while text waits on the slow timer cuts it back to the fast cadence.
    if (cancelFlush) {
      if (slow || !slowArmed) return
      cancelFlush()
      cancelFlush = undefined
    }
    const now = edges.now()
    const interval = slow ? TEXT_FLUSH_INTERVAL_MS : FLUSH_INTERVAL_MS
    const delay = Math.max(0, lastFlushAt === undefined ? 0 : lastFlushAt + interval - now, retryAt - now)
    slowArmed = slow
    cancelFlush = edges.schedule(delay, () => {
      cancelFlush = undefined
      flush()
    })
  }

  function push(event: DashboardEvent): void {
    pending.push(event)
    if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP)
  }

  /** The permission wait an agent shows: its latest marked ask. */
  function permissionOf(id: string): Ask | undefined {
    let found: Ask | undefined
    for (const ask of asks.values()) if (ask.agentId === id && ask.marked) found = ask
    return found
  }

  /** An agent's dirty state as events: its batches and fields, then its permission change. */
  function agentEvents(id: string, r: AgentRec): DashboardEvent[] {
    r.dirty = false
    const out: DashboardEvent[] = []
    const batches = batchViews(r.buf)
    if (id === LEAD) out.push({ kind: 'agent', batches })
    else
      out.push({
        kind: 'agent',
        agentId: id,
        ...(r.parentId ? { parentId: r.parentId } : {}),
        ...(r.label ? { label: r.label } : {}),
        ...(r.sessionKey ? { sessionKey: r.sessionKey } : {}),
        ...(r.caption !== undefined ? { caption: r.caption } : {}),
        ...(r.model !== undefined ? { model: r.model } : {}),
        ...(r.effort !== undefined ? { effort: r.effort } : {}),
        ...(r.contextTokens !== undefined ? { contextTokens: r.contextTokens } : {}),
        ...outputRate(r),
        status: r.status,
        batches,
      })
    const ask = permissionOf(id)
    const permission = ask ? JSON.stringify([ask.tool, ask.preview]) : ''
    if (permission !== r.permissionSent) {
      const agentId = id === LEAD ? {} : { agentId: id }
      out.push(ask ? { kind: 'permission', ...agentId, tool: ask.tool, args: ask.preview } : { kind: 'permission', ...agentId, clear: true })
      r.permissionSent = permission
    }
    return out
  }

  function outputRate(r: AgentRec): { outputTps?: number } {
    const tps = r.rate?.rate()
    return tps !== undefined ? { outputTps: tps } : {}
  }

  /** Sets a subagent's model and effort where given; returns whether either changed. */
  function setModel(r: AgentRec, model: string | undefined, effort: string | undefined): boolean {
    let changed = false
    if (model !== undefined && model !== r.model) {
      r.model = model
      changed = true
    }
    if (effort !== undefined && effort !== r.effort) {
      r.effort = effort
      changed = true
    }
    return changed
  }

  function streamedText(r: { blocks: Map<number, string> }): string {
    return [...r.blocks.entries()]
      .sort(([a], [b]) => a - b)
      .map(([, t]) => t)
      .join('\n')
  }

  function streamEvent(): DashboardEvent | undefined {
    if (!replyDirty) return undefined
    replyDirty = false
    if (!reply) return undefined
    const text = streamedText(reply).slice(0, TEXT_SEND_CAP)
    if (text === '' || text === lastReplySent) return undefined
    lastReplySent = text
    return { kind: 'reply', text }
  }

  /** Every dirty state as events, in a stable order: the lead, the subagents, the streamed reply, a running compaction, the usage status, the command list. */
  function dirtyEvents(): DashboardEvent[] {
    const out: DashboardEvent[] = []
    for (const [id, r] of agents) if (due(r)) out.push(...agentEvents(id, r))
    const stream = streamEvent()
    if (stream) out.push(stream)
    if (compactDue()) {
      compactDirty = false
      out.push({ kind: COMPACT_KIND, phase: 'start', ...(compacting!.trigger ? { trigger: compacting!.trigger } : {}), at: compacting!.at })
    }
    if (statusDue()) {
      statusDirty = false
      countsDirty = false
      out.push({ kind: STATUS_KIND, ...(status === undefined ? {} : JSON.parse(status) as StatusFigures), ...(countsAccepted ? tasks.counts() : {}) })
    }
    if (commandsDue()) {
      commandsDirty = false
      out.push({ kind: COMMANDS_KIND, commands: JSON.parse(commands!) as ListedCommand[] })
    }
    return out
  }

  /** Puts held state ahead of a discrete event, so ordering survives coalescing. */
  function settleBefore(event: DashboardEvent): void {
    for (const e of dirtyEvents()) push(e)
    push(event)
    scheduleFlush()
  }

  function markDirty(id: string): void {
    rec(id).dirty = true
    scheduleFlush()
  }

  /** What ws-mcp holds is stale or gone: resend everything with the next call. */
  function markResync(): void {
    resyncDue = true
    for (const r of agents.values()) {
      r.dirty = true
      r.permissionSent = ''
    }
    lead()
    lastReplySent = undefined
    replyDirty = reply !== undefined
    statusDirty = status !== undefined
    countsDirty = true
    compactDirty = compacting !== undefined
    commandsDirty = commands !== undefined
  }

  /** The conversation events of an undelivered call, which a resync cannot rebuild from current state. */
  function requeue(events: DashboardEvent[]): void {
    const kept = events.filter(
      (e) => e.kind === 'user' || e.kind === 'turn_start' || e.kind === 'turn_end' || (e.kind === 'reply' && e.final === true) || (e.kind === COMPACT_KIND && e.phase !== 'start') ||
        e.kind === COMMAND_KIND || e.kind === 'wake',
    )
    pending = [...kept, ...pending]
    if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP)
  }

  /** `reset` and, when the turn it belongs to is not already among `events`, that turn's `turn_start`. */
  function resyncPrefix(events: DashboardEvent[]): DashboardEvent[] {
    const boundaries = [...events, ...pending]
    const firstStart = boundaries.findIndex((e) => e.kind === 'turn_start')
    const firstEnd = boundaries.findIndex((e) => e.kind === 'turn_end')
    const needStart = lastTurnStart !== undefined && (firstStart === -1 ? turnOpen || firstEnd !== -1 : firstEnd !== -1 && firstEnd < firstStart)
    return needStart ? [{ kind: 'reset' }, lastTurnStart!] : [{ kind: 'reset' }]
  }

  /** A delivered call carried these events: each agent's finished batches in them are delivered. */
  function markSent(events: DashboardEvent[]): void {
    for (const e of events) {
      if (e.kind !== 'agent' || !Array.isArray(e.batches)) continue
      const r = agents.get(typeof e.agentId === 'string' ? e.agentId : LEAD)
      if (r) markDelivered(r.buf, e.batches as BatchView[])
    }
  }

  /** Finished subagents past the cap leave the local tree, as they leave the snapshot's. */
  function pruneFinished(): void {
    const finished = [...agents.entries()].filter(([id, r]) => id !== LEAD && r.announced && r.status !== 'running')
    if (finished.length <= FINISHED_AGENT_CAP) return
    finished.sort(([, a], [, b]) => (b.endedAt ?? 0) - (a.endedAt ?? 0))
    for (const [id] of finished.slice(FINISHED_AGENT_CAP)) agents.delete(id)
  }

  function flush(): void {
    if (stopped || inFlight) return
    const key = edges.key()
    if (key !== sentKey) {
      if (sentKey !== undefined || key !== undefined) markResync()
    }
    // Re-check at send time too: a queued event may predate the latest
    // answer (or a replacement daemon). Old status readers reject new fields.
    // Retain the entire discrete prefix during recovery negotiation so a
    // turn cannot overtake the wake that started it.
    const hold = !wakeAccepted && pending.some((event) => event.kind === 'wake')
    const discrete = hold ? [] : pending
    if (!hold) pending = []
    let events = [...discrete, ...dirtyEvents()].flatMap((event): DashboardEvent[] => {
      if (event.kind !== STATUS_KIND) return [event]
      if (!statusAccepted) return []
      if (countsAccepted) return [event]
      const { background, monitors, ...legacy } = event
      return [legacy as DashboardEvent]
    })
    if (events.length === 0) return
    if (!key) {
      // Nobody to send to yet: keep the conversation, resend state once a key arrives.
      requeue(events)
      markResync()
      return
    }
    if (key !== sentKey) {
      sentKey = key
      resyncDue = true
    }
    if (resyncDue) {
      resyncDue = false
      events = [...resyncPrefix(events), ...events]
    }
    inFlight = true
    lastFlushAt = edges.now()
    const callGeneration = generation
    let finished = false
    let delivered = false
    let accepts: string[] = []
    const done = () => {
      if (finished) return
      finished = true
      cancelTimeout()
      if (callGeneration !== generation) return
      inFlight = false
      lastFlushAt = edges.now()
      if (delivered) markSent(events)
      // Only an answered call's `accepts:` line turns status on; any other outcome turns it off.
      const accepted = accepts.includes(STATUS_KIND)
      if (accepted && !statusAccepted) statusDirty = status !== undefined
      statusAccepted = accepted
      wakeAccepted = accepts.includes('wake')
      if (delivered && !wakeAccepted) pending = pending.filter((event) => event.kind !== 'wake')
      const countsOn = accepted && accepts.includes('task_counts')
      if (countsOn && !countsAccepted) countsDirty = true
      countsAccepted = countsOn
      const compactOn = accepts.includes(COMPACT_KIND)
      if (compactOn && !compactAccepted) compactDirty = compacting !== undefined
      compactAccepted = compactOn
      const commandsOn = accepts.includes(COMMANDS_KIND)
      if (commandsOn && !commandsAccepted) commandsDirty = commands !== undefined
      commandsAccepted = commandsOn
      commandAccepted = accepts.includes(COMMAND_KIND)
      if (!stopped && !delivered) {
        requeue(events)
        markResync()
        retryAt = edges.now() + RETRY_MS
      }
      scheduleFlush()
    }
    const cancelTimeout = edges.schedule(CALL_TIMEOUT_MS, done)
    let run: Promise<IntakeResult>
    try {
      run = edges.call({ session_key: key, adapter: ADAPTER, events })
    } catch (err) {
      run = Promise.reject(err)
    }
    run.then(
      ({ ok, text }) => {
        if (!finished && ok) delivered = true
        if (!finished) accepts = acceptedKinds(text)
        if (!ok) edges.log('dashboard intake answered an error; resyncing')
        done()
      },
      (err) => {
        edges.log(`dashboard intake failed: ${String(err)}`)
        done()
      },
    )
  }

  function reset(): void {
    generation += 1
    inFlight = false
    cancelFlush?.()
    cancelFlush = undefined
    pending = []
    agents = new Map()
    asks = new Map()
    tasks.clear()
    countsDirty = true
    reply = undefined
    replyDirty = false
    lastReplySent = undefined
    lastFinal = undefined
    turnOpen = false
    nextOrigin = undefined
    lastTurnStart = undefined
    resyncDue = false
    retryAt = 0
    // The held usage outlives a restart: the restart's `reset` clears it on the server, so it goes again.
    statusDirty = status !== undefined
    commandsDirty = commands !== undefined
    compacting = undefined
    compactDirty = false
  }

  /** Runs one handler only while the feed runs; a throw is logged and never reaches the session. */
  const guard =
    <A extends unknown[]>(name: string, fn: (...args: A) => void) =>
    (...args: A): void => {
      if (stopped) return
      try {
        fn(...args)
      } catch (err) {
        edges.log(`dashboard ${name} failed: ${String(err)}`)
      }
    }

  function finishReply(): void {
    reply = undefined
    replyDirty = false
    lastReplySent = undefined
  }

  /** Clears a call's ask; an agent whose shown wait changed is marked dirty. */
  function clearAsk(id: string | undefined): void {
    if (!id) return
    const ask = asks.get(id)
    if (!ask) return
    asks.delete(id)
    if (ask.marked) markDirty(ask.agentId)
  }

  function clearAsksOf(agentId: string): void {
    for (const [id, ask] of asks) if (ask.agentId === agentId) asks.delete(id)
  }

  function taskChanged(before: string): void {
    if (JSON.stringify(tasks.counts()) === before) return
    countsDirty = true
    scheduleFlush()
  }

  function wake(event: Wake): void {
    if (wakeAccepted) settleBefore({ kind: 'wake', ...event, text: event.text.slice(0, TEXT_SEND_CAP) })
  }

  return {
    active: () => !stopped,
    start() {
      reset()
      status = undefined
      statusDirty = false
      statusAccepted = false
      compactAccepted = false
      commands = undefined
      commandsDirty = false
      commandsAccepted = false
      commandAccepted = false
      wakeAccepted = false
      countsAccepted = false
      stopped = false
      sentKey = undefined
      resyncDue = true
      lead()
      seed()
      scheduleFlush()
    },
    stop() {
      reset()
      status = undefined
      statusDirty = false
      statusAccepted = false
      compactAccepted = false
      commands = undefined
      commandsDirty = false
      commandsAccepted = false
      commandAccepted = false
      wakeAccepted = false
      countsAccepted = false
      stopped = true
    },
    restart: guard('restart', () => {
      reset()
      resyncDue = true
      lead()
      scheduleFlush()
    }),
    keyChanged: guard('key change', () => {
      // A new key's session holds nothing yet: everything goes again, behind a reset.
      if (edges.key() !== sentKey) markResync()
      scheduleFlush()
    }),
    mailSubmitting: guard('mail submit', () => {
      if (!turnOpen) nextOrigin = 'mail'
    }),
    mailSubmitFailed: guard('mail submit', () => {
      if (!turnOpen && nextOrigin === 'mail') nextOrigin = undefined
    }),
    promptSubmitting: guard('prompt submit', (origin: Origin | undefined, startsTurn: boolean) => {
      if (startsTurn && !turnOpen && origin) nextOrigin = origin
    }),
    whiteboardSubmitting: guard('whiteboard submit', () => {
      ownWhiteboard = { recorded: false }
      if (!turnOpen) nextOrigin = 'whiteboard'
    }),
    whiteboardSubmitted: guard('whiteboard submit', (text: string, entered: boolean) => {
      const own = ownWhiteboard
      ownWhiteboard = undefined
      if (!entered) {
        if (!turnOpen && nextOrigin === 'whiteboard') nextOrigin = undefined
        return
      }
      if (own?.recorded || text === '') return
      settleBefore({ kind: 'user', text: text.slice(0, TEXT_SEND_CAP), origin: 'whiteboard', at: edges.now() })
    }),
    whiteboardSteered: guard('whiteboard steer', (text: string) => {
      settleBefore({ kind: 'user', text: text.slice(0, TEXT_SEND_CAP), origin: 'whiteboard', at: edges.now() })
    }),
    userMessage: guard('user message', (text: string, origin: 'terminal' | 'whiteboard') => {
      if (origin === 'whiteboard' && ownWhiteboard) ownWhiteboard.recorded = true
      settleBefore({ kind: 'user', text: text.slice(0, TEXT_SEND_CAP), origin, at: edges.now() })
    }),
    wakeSubmitted: guard('wake prompt', (prompt: WakePrompt, plugin: string) => {
      // A person's pasted notification must neither produce a wake nor end a task.
      if (PERSON_ORIGINS.has(prompt.origin?.kind ?? '') || userPrompt(prompt, plugin)) return
      const n = notification(prompt.text)
      const event = wakePrompt(prompt, plugin, tasks.kind(n.id, n.callId), n.id !== undefined && agents.get(n.id)?.announced === true)
      if (!event) return
      const before = JSON.stringify(tasks.counts())
      tasks.notified(prompt.text)
      taskChanged(before)
      wake(event)
    }),
    mailDelivered: guard('mail delivered', (text: string) => {
      wake({ cause: 'mail', source: 'mailbox', caption: 'Mail arrived', text })
    }),
    turnStarted: guard('turn start', (agentId: string | undefined) => {
      if (agentId !== undefined) {
        const r = rec(agentId)
        if (r.status !== 'running') {
          r.status = 'running'
          r.endedAt = undefined
        }
        markDirty(agentId)
        return
      }
      if (turnOpen) return
      turnOpen = true
      lastFinal = undefined
      lastTurnStart = { kind: 'turn_start', ...(nextOrigin ? { origin: nextOrigin } : {}), at: edges.now() }
      nextOrigin = undefined
      settleBefore(lastTurnStart)
    }),
    turnCompleted: guard('turn end', (agentId: string | undefined, answer: string, reason: string) => {
      const now = edges.now()
      const before = JSON.stringify(tasks.counts())
      tasks.abort(agentId)
      taskChanged(before)
      if (agentId !== undefined) {
        const r = rec(agentId)
        closeBuffer(r.buf, now)
        clearAsksOf(agentId)
        r.status = reason === 'answer' ? 'done' : 'failed'
        r.endedAt = now
        pruneFinished()
        markDirty(agentId)
        return
      }
      closeBuffer(lead().buf, now)
      clearAsksOf(LEAD)
      lead().dirty = true
      if (answer !== '' && answer !== lastFinal) {
        finishReply()
        settleBefore({ kind: 'reply', text: answer.slice(0, TEXT_SEND_CAP), final: true, at: now })
      }
      finishReply()
      lastFinal = undefined
      if (!turnOpen) {
        scheduleFlush()
        return
      }
      turnOpen = false
      settleBefore({ kind: 'turn_end', at: now, ...(reason ? { reason } : {}) })
    }),
    stepBegan: guard('step start', (agentId: string | undefined, step: string, request?: Parameters<Dashboard['stepBegan']>[2]) => {
      const id = idOf(agentId)
      const r = rec(id)
      stepBegan(r.buf, step, edges.now())
      if (id === LEAD) reply = { step, blocks: new Map() }
      else {
        const model = typeof request?.model === 'string' && request.model.trim() !== '' ? request.model.trim() : undefined
        setModel(r, model, effortText(request?.effort))
        ;(r.rate ??= createOutputRate()).begin()
      }
      markDirty(id)
    }),
    stepChunk: guard('step chunk', (agentId: string | undefined, step: string, chunk: Parameters<Dashboard['stepChunk']>[2]) => {
      const id = idOf(agentId)
      const r = rec(id)
      if (r.rate && r.buf.streaming === step && BLOCK_CHUNKS.has(chunk.kind) && typeof chunk.index === 'number') r.rate.chunk(chunk.index, edges.now())
      if (stepChunk(r.buf, step, chunk, edges.now())) markDirty(id)
      if (id === LEAD && chunk.kind === 'text' && reply?.step === step && typeof chunk.text === 'string') {
        const index = typeof chunk.index === 'number' ? chunk.index : 0
        reply.blocks.set(index, (reply.blocks.get(index) ?? '') + chunk.text)
        replyDirty = true
        scheduleFlush()
      }
    }),
    stepEnded: guard('step end', (agentId: string | undefined, step: string, result: Parameters<Dashboard['stepEnded']>[2]) => {
      const id = idOf(agentId)
      const r = rec(id)
      const tokens = stepTokens(result.usage)
      const telemetry = id !== LEAD && r.buf.streaming === step
      if (telemetry) {
        // A request with no usage keeps the context size and adds no rate sample.
        if (tokens) r.contextTokens = contextTokensOf(tokens)
        const answered = result.usage?.model
        r.rate?.finish(tokens?.output, typeof answered === 'string' && answered !== '' ? answered : r.model)
      }
      if (stepEnded(r.buf, step, result.toolUses ?? [], edges.now(), tokens) || telemetry) markDirty(id)
      if (id !== LEAD) return
      const answer = typeof result.answer === 'string' ? result.answer : ''
      if (answer !== '') {
        finishReply()
        lastFinal = answer
        settleBefore({ kind: 'reply', text: answer.slice(0, TEXT_SEND_CAP), final: true, at: edges.now() })
      } else if (reply?.step === step) {
        // A tool-calling step carries no answer, but the text it streamed is lead output that must stay in the conversation.
        const streamed = streamedText(reply)
        finishReply()
        if (streamed.trim() !== '') {
          // lastFinal keeps turn.complete from sending the same text again if it repeats it.
          lastFinal = streamed
          settleBefore({ kind: 'reply', text: streamed.slice(0, TEXT_SEND_CAP), final: true, at: edges.now() })
        }
      }
    }),
    toolBegan: guard('tool start', (agentId: string | undefined, id: string | undefined, tool: string, args: Record<string, unknown>) => {
      const aid = idOf(agentId)
      const r = rec(aid)
      const before = JSON.stringify(tasks.counts())
      tasks.began(agentId, id, tool, args)
      taskChanged(before)
      toolBegan(r.buf, id ?? '', tool, args, edges.now())
      const key = args.session_key
      if (aid !== LEAD && r.sessionKey === undefined && typeof key === 'string' && key.trim() !== '' && edges.ownTool(tool)) r.sessionKey = key.trim()
      markDirty(aid)
    }),
    toolEnded: guard('tool end', (agentId: string | undefined, id: string | undefined, isError: boolean, result?: unknown) => {
      const before = JSON.stringify(tasks.counts())
      tasks.ended(agentId, id, isError, result)
      taskChanged(before)
      const aid = idOf(agentId)
      if (toolEnded(rec(aid).buf, id ?? '', isError, edges.now())) markDirty(aid)
      clearAsk(id)
    }),
    toolChecked: guard('tool check', (check: Parameters<Dashboard['toolChecked']>[0], decision: string) => {
      if (decision !== 'ask' || check.approver !== 'person' || !check.id) return
      asks.set(check.id, { agentId: idOf(check.agentId), tool: check.tool, preview: argsPreview(check.input), marked: false })
    }),
    notified: guard('notification', (type: string) => {
      if (type !== PERMISSION_PROMPT) return
      for (const ask of asks.values()) {
        if (ask.marked) continue
        ask.marked = true
        markDirty(ask.agentId)
      }
    }),
    agentSpawned: guard('agent spawn', (spawn: Parameters<Dashboard['agentSpawned']>[0]) => {
      const r = announce(spawn.agentId, { parentId: spawn.parentId, description: spawn.description, type: spawn.subagentType })
      // Seeds only: a request already made (its turn can run before its spawn resolves) names the actual model and effort.
      const model = typeof spawn.model === 'string' && spawn.model.trim() !== '' ? spawn.model.trim() : undefined
      setModel(r, r.model === undefined ? model : undefined, r.effort === undefined ? effortOfType(spawn.subagentType) : undefined)
    }),
    status: guard('status', (figures: StatusFigures) => {
      const json = JSON.stringify(figures)
      if (json === status) return
      status = json
      statusDirty = true
      scheduleFlush()
    }),
    compactStarted: guard('compact start', (trigger: CompactTrigger | undefined) => {
      compacting = { ...(trigger ? { trigger } : {}), at: edges.now() }
      compactDirty = true
      scheduleFlush()
    }),
    compactEnded: guard('compact end', (trigger: CompactTrigger | undefined, landed: boolean) => {
      // A start ws-mcp does not hold yet goes ahead of its end (settleBefore drains it first).
      if (compactAccepted) settleBefore({ kind: COMPACT_KIND, phase: landed ? 'end' : 'failed', ...(trigger ? { trigger } : {}), at: edges.now() })
      compacting = undefined
      compactDirty = false
    }),
    commands: guard('commands', (list: readonly ListedCommand[]) => {
      const json = JSON.stringify(list)
      if (json === commands) return
      commands = json
      commandsDirty = true
      scheduleFlush()
    }),
    commandRan: guard('command', (line: string, text: string | undefined) => {
      if (!commandAccepted) {
        edges.log(`command ${line} ran; the intake takes no command events, so the page shows no outcome`)
        return
      }
      settleBefore({ kind: COMMAND_KIND, command: line, ...(text !== undefined && text !== '' ? { text: text.slice(0, TEXT_SEND_CAP) } : {}), at: edges.now() })
    }),
  }
}

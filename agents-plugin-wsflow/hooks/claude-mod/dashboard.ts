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
// - the lead's batches (no agentId) and one `agent` event per subagent,
//   keyed by the host's native agent id, with its label, parent, status, and
//   the ws session key a delegate passes to this plugin's tools; the server
//   shows that key's reply-id and never stores the key;
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
//   and its start sent again after a resync.
//
// A batch is one model request (a `turn.step`) and the tool calls it made:
// the step streams as a generating view (thinking, replying, or a tool call
// whose arguments are streaming), a tool chunk announces its call in the
// step's batch at once (calls start while the step still streams), and the
// batch ends when the step has ended and every call has. A step with no tool
// call makes no batch.
//
// Permission waits: a tool.check that resolves `ask` for a person to answer
// marks the call as asked; it still shows only as a running tool. A
// `permission_prompt` notification while asks are pending switches every one
// of them to waiting for approval (the notification names no tool), and the
// call's tool.call end clears it, after the tool ran or was refused. An ask
// settled with no dialog gets no notification and never shows the marker.
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

/** The intake tool, as the MCP server names it. */
export const INTAKE_TOOL = 'dashboard_intake'
/** The board-answer inbox's drain tool, as the MCP server names it: an atomic claim of every held message. */
export const INBOX_DRAIN_TOOL = 'dashboard_inbox_drain'
/** The adapter name the intake records. */
export const ADAPTER = 'claude'
/** Minimum gap between two intake calls: at most ten a second. */
export const FLUSH_INTERVAL_MS = 100
/**
 * Minimum gap between two intake calls while the only pending change is
 * streamed reply text: at most one a second. Ten calls a second while a reply
 * streams made the terminal flicker; a discrete change keeps FLUSH_INTERVAL_MS
 * and takes the waiting text with it.
 */
export const TEXT_FLUSH_INTERVAL_MS = 1000
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
/** Finished subagents kept beside every running one, as the snapshot keeps them. */
export const FINISHED_AGENT_CAP = 5
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

/**
 * The messages an inbox drain's `format: "json"` text claimed, oldest first:
 * each one's `text`. The drain has already claimed whatever it returned, so
 * text that is not that shape comes back whole as one message, not dropped.
 */
export function parseInboxDrain(text: string): string[] {
  const trimmed = text.trim()
  if (trimmed === '') return []
  let messages: unknown
  try {
    messages = (JSON.parse(trimmed) as { messages?: unknown } | null)?.messages
  } catch {
    return [trimmed]
  }
  if (!Array.isArray(messages)) return [trimmed]
  return messages.flatMap((m): string[] => {
    const t = (m as { text?: unknown } | null)?.text
    if (typeof t === 'string') return t.trim() === '' ? [] : [t]
    return [JSON.stringify(m)]
  })
}

export type Origin = 'terminal' | 'whiteboard' | 'mail'

/** Prompt origins that are the person's own message: typed, from a remote client, or an SDK host's turn. */
const PERSON_ORIGINS = new Set(['composer', 'bridge', 'sdk', 'unclassified', 'slack-ping'])

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
type Batch = { seq: number; step: string; startedAt: number; endedAt?: number; calls: Call[] }
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
  if (buf.batches.length > BATCH_CAP) buf.batches.splice(0, buf.batches.length - BATCH_CAP)
  return batch
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

/** The request `step` ended: its generating view goes, and its batch takes the calls' arguments. */
export function stepEnded(buf: Buffer, step: string, toolUses: readonly { name: string; input: unknown }[], now: number): boolean {
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

// ---- The feed --------------------------------------------------------------

export type DashboardEvent = { kind: 'reset' | 'turn_start' | 'turn_end' | 'agent' | 'permission' | 'reply' | 'user' | 'status' | 'compact'; [field: string]: unknown }

/** One intake call's outcome: whether it was delivered (not an error result) and its result text. */
export type IntakeResult = { ok: boolean; text: string }

export interface DashboardEdges {
  /** The session key the feed reports under: the conversation's lead key, else undefined (held until one arrives). */
  key: () => string | undefined
  /** One intake call; resolves with its outcome and result text, rejects on transport failure. */
  call: (args: { session_key: string; adapter: string; events: DashboardEvent[] }) => Promise<IntakeResult>
  /** Whether a tool is one of this plugin's MCP server tools, whose `session_key` names a delegate's key. */
  ownTool: (tool: string) => boolean
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
}

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
  turnStarted: (agentId: string | undefined) => void
  turnCompleted: (agentId: string | undefined, answer: string, reason: string) => void
  stepBegan: (agentId: string | undefined, step: string) => void
  stepChunk: (agentId: string | undefined, step: string, chunk: { kind: string; index?: number; text?: string; id?: string; name?: string; json?: string }) => void
  stepEnded: (agentId: string | undefined, step: string, result: { answer?: string; toolUses?: readonly { name: string; input: unknown }[] }) => void
  toolBegan: (agentId: string | undefined, id: string | undefined, tool: string, args: Record<string, unknown>) => void
  toolEnded: (agentId: string | undefined, id: string | undefined, isError: boolean) => void
  toolChecked: (check: { agentId?: string; id?: string; tool: string; input: unknown; approver?: string }, decision: string) => void
  notified: (type: string) => void
  agentSpawned: (spawn: { agentId: string; parentId?: string; description?: string; subagentType?: string }) => void
  /** The session's latest usage figures: held, and sent while the intake accepts `status`. */
  status: (figures: StatusFigures) => void
  /** A main-conversation compaction begins: held while it runs, and its start sent while the intake accepts `compact`. */
  compactStarted: (trigger: CompactTrigger | undefined) => void
  /** That compaction settled: `landed`, else it failed or was vetoed. */
  compactEnded: (trigger: CompactTrigger | undefined, landed: boolean) => void
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

  const lead = (): AgentRec => rec(LEAD)

  function rec(id: string): AgentRec {
    let r = agents.get(id)
    if (!r) {
      r = { buf: createBuffer(edges.epoch()), dirty: true, status: 'running', permissionSent: '' }
      agents.set(id, r)
    }
    return r
  }

  const idOf = (agentId: string | undefined) => agentId ?? LEAD

  const statusDue = () => statusDirty && statusAccepted && status !== undefined
  const compactDue = () => compactDirty && compactAccepted && compacting !== undefined

  function hasWork(): boolean {
    return pending.length > 0 || replyDirty || statusDue() || compactDue() || [...agents.values()].some((r) => r.dirty)
  }

  /** Whether streamed reply text is the only pending change, so the slow cadence applies. */
  function textOnly(): boolean {
    return replyDirty && pending.length === 0 && !statusDue() && !compactDue() && ![...agents.values()].some((r) => r.dirty)
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

  function streamEvent(): DashboardEvent | undefined {
    if (!replyDirty) return undefined
    replyDirty = false
    if (!reply) return undefined
    const text = [...reply.blocks.entries()]
      .sort(([a], [b]) => a - b)
      .map(([, t]) => t)
      .join('\n')
      .slice(0, TEXT_SEND_CAP)
    if (text === '' || text === lastReplySent) return undefined
    lastReplySent = text
    return { kind: 'reply', text }
  }

  /** Every dirty state as events, in a stable order: the lead, the subagents, the streamed reply, a running compaction, the usage status. */
  function dirtyEvents(): DashboardEvent[] {
    const out: DashboardEvent[] = []
    for (const [id, r] of agents) if (r.dirty) out.push(...agentEvents(id, r))
    const stream = streamEvent()
    if (stream) out.push(stream)
    if (compactDue()) {
      compactDirty = false
      out.push({ kind: COMPACT_KIND, phase: 'start', ...(compacting!.trigger ? { trigger: compacting!.trigger } : {}), at: compacting!.at })
    }
    if (statusDue()) {
      statusDirty = false
      out.push({ kind: STATUS_KIND, ...(JSON.parse(status!) as StatusFigures) })
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
    compactDirty = compacting !== undefined
  }

  /** The conversation events of an undelivered call, which a resync cannot rebuild from current state. */
  function requeue(events: DashboardEvent[]): void {
    const kept = events.filter(
      (e) => e.kind === 'user' || e.kind === 'turn_start' || e.kind === 'turn_end' || (e.kind === 'reply' && e.final === true) || (e.kind === COMPACT_KIND && e.phase !== 'start'),
    )
    pending = [...kept, ...pending]
    if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP)
  }

  /** `reset` and, when the turn it belongs to is not already among `events`, that turn's `turn_start`. */
  function resyncPrefix(events: DashboardEvent[]): DashboardEvent[] {
    const firstStart = events.findIndex((e) => e.kind === 'turn_start')
    const firstEnd = events.findIndex((e) => e.kind === 'turn_end')
    const needStart = lastTurnStart !== undefined && (firstStart === -1 ? turnOpen || firstEnd !== -1 : firstEnd !== -1 && firstEnd < firstStart)
    return needStart ? [{ kind: 'reset' }, lastTurnStart!] : [{ kind: 'reset' }]
  }

  /** Finished subagents past the cap leave the local tree, as they leave the snapshot's. */
  function pruneFinished(): void {
    const finished = [...agents.entries()].filter(([id, r]) => id !== LEAD && r.status !== 'running')
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
    let events = [...pending, ...dirtyEvents()]
    pending = []
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
      // Only an answered call's `accepts:` line turns status on; any other outcome turns it off.
      const accepted = accepts.includes(STATUS_KIND)
      if (accepted && !statusAccepted) statusDirty = status !== undefined
      statusAccepted = accepted
      const compactOn = accepts.includes(COMPACT_KIND)
      if (compactOn && !compactAccepted) compactDirty = compacting !== undefined
      compactAccepted = compactOn
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

  return {
    active: () => !stopped,
    start() {
      reset()
      status = undefined
      statusDirty = false
      statusAccepted = false
      compactAccepted = false
      stopped = false
      sentKey = undefined
      resyncDue = true
      lead()
      scheduleFlush()
    },
    stop() {
      reset()
      status = undefined
      statusDirty = false
      statusAccepted = false
      compactAccepted = false
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
      settleBefore({ kind: 'turn_end', at: now })
    }),
    stepBegan: guard('step start', (agentId: string | undefined, step: string) => {
      const id = idOf(agentId)
      stepBegan(rec(id).buf, step, edges.now())
      if (id === LEAD) reply = { step, blocks: new Map() }
      markDirty(id)
    }),
    stepChunk: guard('step chunk', (agentId: string | undefined, step: string, chunk: Parameters<Dashboard['stepChunk']>[2]) => {
      const id = idOf(agentId)
      if (stepChunk(rec(id).buf, step, chunk, edges.now())) markDirty(id)
      if (id === LEAD && chunk.kind === 'text' && reply?.step === step && typeof chunk.text === 'string') {
        const index = typeof chunk.index === 'number' ? chunk.index : 0
        reply.blocks.set(index, (reply.blocks.get(index) ?? '') + chunk.text)
        replyDirty = true
        scheduleFlush()
      }
    }),
    stepEnded: guard('step end', (agentId: string | undefined, step: string, result: Parameters<Dashboard['stepEnded']>[2]) => {
      const id = idOf(agentId)
      if (stepEnded(rec(id).buf, step, result.toolUses ?? [], edges.now())) markDirty(id)
      if (id !== LEAD) return
      const answer = typeof result.answer === 'string' ? result.answer : ''
      if (answer !== '') {
        finishReply()
        lastFinal = answer
        settleBefore({ kind: 'reply', text: answer.slice(0, TEXT_SEND_CAP), final: true, at: edges.now() })
      } else if (reply?.step === step) finishReply()
    }),
    toolBegan: guard('tool start', (agentId: string | undefined, id: string | undefined, tool: string, args: Record<string, unknown>) => {
      const aid = idOf(agentId)
      const r = rec(aid)
      toolBegan(r.buf, id ?? '', tool, args, edges.now())
      const key = args.session_key
      if (aid !== LEAD && r.sessionKey === undefined && typeof key === 'string' && key.trim() !== '' && edges.ownTool(tool)) r.sessionKey = key.trim()
      markDirty(aid)
    }),
    toolEnded: guard('tool end', (agentId: string | undefined, id: string | undefined, isError: boolean) => {
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
      const r = rec(spawn.agentId)
      if (spawn.parentId && spawn.parentId !== spawn.agentId) r.parentId = spawn.parentId
      const label = spawn.description?.trim() || spawn.subagentType?.trim()
      if (label) r.label = label
      if (spawn.subagentType?.trim()) r.caption = spawn.subagentType.trim()
      markDirty(spawn.agentId)
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
  }
}

// Pure core of the Claude mod's lead compaction timing feature: threshold
// points, the one-latch delivery rule, the lever's input check, and the
// pinned prose. compaction.ts drives these and register.ts binds them to the
// engine; nothing here touches `$`.

export const TOOL_NAME = 'request_compaction'
/** Opens every prompt the mod submits, so it never reads as the human's own words. */
export const LABEL = '[ws compaction] '
/** The config key that sets the advisory point; read once per session start. */
export const ADVISORY_KEY = 'claude.compaction_advisory_percent'

/**
 * An advisory point before it is resolved against a compaction window: a
 * percent of the window, a token count, or both (whichever is reached first).
 * At least one term is set.
 */
export type AdvisorySpec = { percent?: number; tokens?: number }

/** The advisory point with no key set, or with one that does not parse. */
export const DEFAULT_ADVISORY: AdvisorySpec = { percent: 50 }

// One term each of the advisory grammar; the key's manifest pattern accepts
// exactly the strings parseAdvisory accepts.
const PERCENT_TERM = /^(100|[1-9][0-9]?)%?$/
const TOKEN_TERM = /^([0-9]*[1-9][0-9]*(?:\.[0-9]+)?|0+\.[0-9]*[1-9][0-9]*)([km])$/

/**
 * Parses an advisory value: `50` or `50%` (an integer percent, 1-100), `150k`
 * or `1.5m` (tokens, k = 1,000 and m = 1,000,000, rounded to a whole token),
 * or one percent term and one token term joined by `|` in either order
 * (`50%|150k`). Anything else is undefined.
 */
export function parseAdvisory(value: unknown): AdvisorySpec | undefined {
  if (typeof value !== 'string') return undefined
  const terms = value.split('|')
  if (terms.length > 2) return undefined
  const spec: AdvisorySpec = {}
  for (const term of terms) {
    const percent = PERCENT_TERM.exec(term)
    const tokens = percent ? null : TOKEN_TERM.exec(term)
    if (percent && spec.percent === undefined) spec.percent = Number(percent[1])
    else if (tokens && spec.tokens === undefined) spec.tokens = Math.round(Number(tokens[1]) * (tokens[2] === 'k' ? 1_000 : 1_000_000))
    else return undefined
  }
  return spec
}

/** The configured advisory point, or the default when `value` does not parse; never fails. */
export function resolveAdvisory(value: unknown): AdvisorySpec {
  return parseAdvisory(value) ?? DEFAULT_ADVISORY
}

/**
 * The advisory point in tokens of a `raw`-token window: the earlier of its
 * terms. A token term is not clamped to the window; beyond the automatic
 * compaction point it never fires, and the other term (if any) decides.
 */
export function advisoryTokens(spec: AdvisorySpec, raw: number): number {
  return Math.min(spec.percent !== undefined ? (raw * spec.percent) / 100 : Infinity, spec.tokens ?? Infinity)
}

/** The compaction-window figures of a `summary` context breakdown that the thresholds use. */
export type CompactionWindow = {
  rawMaxTokens: number
  autoCompactThreshold?: number
  isAutoCompactEnabled: boolean
}

/** Token counts of the ordered thresholds, and the window their percentages are of. */
export type Thresholds = {
  raw: number
  advisory: number
  m1: number
  m2: number
  hard: number
}

/**
 * The thresholds, or undefined when the mod sends no advisory or milestone:
 * auto compaction off, its point unknown, or not above the advisory point.
 * Milestones sit one and two thirds of the way from the advisory to the
 * native auto-compact point.
 */
export function thresholdsFor(w: CompactionWindow, spec: AdvisorySpec = DEFAULT_ADVISORY): Thresholds | undefined {
  if (!w.isAutoCompactEnabled || w.autoCompactThreshold === undefined || !(w.rawMaxTokens > 0)) return undefined
  const advisory = advisoryTokens(spec, w.rawMaxTokens)
  const hard = w.autoCompactThreshold
  if (!(hard > advisory)) return undefined
  const step = (hard - advisory) / 3
  return { raw: w.rawMaxTokens, advisory, m1: advisory + step, m2: advisory + 2 * step, hard }
}

/** How many of the ordered thresholds (advisory, first milestone, second milestone) `tokens` has reached. */
export type Level = 0 | 1 | 2 | 3

export function levelAt(t: Thresholds, tokens: number): Level {
  return ([t.advisory, t.m1, t.m2].filter((p) => tokens >= p).length) as Level
}

export type Delivery = { kind: 'advisory' } | { kind: 'milestone'; which: 1 | 2 }

/**
 * One usage observation against the latch, the level last delivered.
 *
 * An unset latch (session start, resume, clear) is baselined from this usage
 * and delivers nothing. Usage below the latch re-arms it. A crossing above it
 * delivers one message: the advisory while it is undelivered (a milestone
 * waits for it), otherwise the milestone at or below usage; delivering latches
 * to the usage's level, covering every skipped lower threshold. A crossing
 * that cannot be delivered now leaves the latch where it was, still pending.
 */
export function observe(latch: Level | undefined, level: Level, canDeliver: boolean): { latch: Level; deliver?: Delivery } {
  if (latch === undefined) return { latch: level }
  const armed = level < latch ? level : latch
  if (level <= armed || !canDeliver) return { latch: armed }
  const deliver: Delivery = armed === 0 ? { kind: 'advisory' } : { kind: 'milestone', which: (level - 1) as 1 | 2 }
  return { latch: level, deliver }
}

/** `tokens` as a rounded whole percentage of the compaction window. */
export function percentOf(tokens: number, raw: number): number {
  return Math.round((tokens / raw) * 100)
}

export function advisoryText(t: Thresholds, tokens: number): string {
  const hard = percentOf(t.hard, t.raw)
  return (
    `${LABEL}Context usage is ${percentOf(tokens, t.raw)}% of the window (advisory point: ${percentOf(t.advisory, t.raw)}%). Claude Code compacts automatically at about ${hard}%.\n` +
    '\n' +
    'This is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.\n' +
    '\n' +
    `From here on, look for a good moment to compact before ${hard}%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, first bring durable records up to date with anything still needed that only this context holds, then call the compaction request tool and end your turn. Set its \`continue\` flag only when work must resume by itself after compaction; leave it unset when the next turn will come from the human or from a background agent's notification. Compacting at a moment you choose keeps the work coherent; at ${hard}% it happens automatically, mid-work if need be. Reminders follow at ${percentOf(t.m1, t.raw)}% and ${percentOf(t.m2, t.raw)}%.\n` +
    '\n' +
    'If now is not such a moment, end this turn without replying and keep looking for one as you work.'
  )
}

export function milestoneText(which: 1 | 2, t: Thresholds, tokens: number): string {
  const n = percentOf(tokens, t.raw)
  const hard = percentOf(t.hard, t.raw)
  const head = `${LABEL}Context window: ${n}% / ${hard}% (automatic compaction point). `
  return which === 1
    ? head +
        `Past ${hard}%, Claude Code compacts automatically, mid-work if need be. From here on, a good boundary is worth taking; if this is one, update durable records and call the compaction request tool, as the advisory describes (the \`continue\` flag only when work must resume by itself). Otherwise end this turn without replying.`
    : head +
        'This is the last reading before automatic compaction. Compacting at a boundary you choose keeps the work coherent; take this one if it fits, update durable records and call the compaction request tool, as the advisory describes (the `continue` flag only when work must resume by itself). Otherwise end this turn without replying, and take the next good boundary.'
}

export function deliveryText(d: Delivery, t: Thresholds, tokens: number): string {
  return d.kind === 'advisory' ? advisoryText(t, tokens) : milestoneText(d.which, t, tokens)
}

export const LEVER_DESCRIPTION =
  'Request compaction of this conversation when the current turn ends. Before calling, bring durable records (tickets, notes, agenda) up to date with anything still needed that only this context holds. Set `continue` to true only when work must resume by itself after compaction: a fixed resume prompt then follows the compaction, unless another prompt (the human\'s, or a background agent\'s notification) has already started the next turn. Leave it unset when the next turn will come from the human or from a background agent\'s notification. Main session only.'

export const LEVER_RESULT =
  'Compaction requested. End your turn now without further tool calls; compaction runs when the turn ends, and a resume prompt follows it unless another prompt has already started the next turn.'

/** The lever's result for a call without the `continue` flag: nothing is submitted after the compaction. */
export const LEVER_RESULT_NO_RESUME =
  'Compaction requested. End your turn now without further tool calls; compaction runs when the turn ends, and no prompt follows it: the next turn comes from the human or from a background agent\'s notification.'

/** What the lever's result says for a call with or without the `continue` flag. */
export function leverResult(resume: boolean): string {
  return resume ? LEVER_RESULT : LEVER_RESULT_NO_RESUME
}

/** The resume prompt after a compaction landed: the lever's own, or another that landed first. */
export const RESUME_TEXT = `${LABEL}Compaction landed; resume the work in progress from the summary and your durable records.`

/** The resume prompt after a hook vetoed the lever's compaction with `{ skip }`. */
export function vetoedResumeText(reason: string): string {
  return `${LABEL}Compaction did not run (${reason}); your context is unchanged. Resume the work in progress.`
}

/**
 * Main-loop-only guard and input read for the lever: a refusal, or whether
 * the call set the `continue` flag. Only a literal `true` sets it; absent or
 * any other value (a non-boolean, a leftover string argument) reads as unset.
 */
export function checkLeverCall(agentId: string | undefined, flag: unknown): { resume: boolean } | { deny: string } {
  if (agentId !== undefined) {
    return { deny: `${TOOL_NAME} compacts the main session only; called from subagent ${agentId}.` }
  }
  return { resume: flag === true }
}

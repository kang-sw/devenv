import type { EngineInterface, RenderChildren, SessionUsage } from 'claude-code'

/**
 * The status band, a display feature of the Claude mod (ws only, off until
 * `claude.status_band_enabled` is set): a framed band above the prompt with
 * the model as its border title and up to four rows.
 *
 * - Row 1: the mailbox address, the folder (spanning the spare width), then
 *   the branch and the working-tree changes.
 * - Row 2: the context bar (spanning the spare width), then the 5h and 7d
 *   rate-limit capsules and the cost.
 * - Row 3 (ws focus row): the session's in-progress or first pending todo,
 *   and the todo count.
 * - Row 4 (ws rest row): the other open todo keys, and the agenda blob keys.
 *
 * The ws rows draw only when they have content. Every figure is kept in
 * module memory, refreshed by events (never by a read per redraw), and the
 * engine is asked to draw again after each change. Every read fails closed: a
 * failed knob read is off, a failed ws read hides the ws rows, a failed
 * mailbox read leaves the address out.
 *
 * The engine calls arrive as edges bound in register.ts; the drawing takes
 * the surface's element table, never `$`.
 */

export const ENABLED_KEY = 'claude.status_band_enabled'

/** A tool call refreshes git at most this often; a turn end always does. */
export const GIT_THROTTLE_MS = 2000
/** How often the clock the budget deltas read moves on. */
export const TICK_MS = 10_000
/** The context bar shrinks to this many cells before a row 2 capsule drops. */
export const BAR_FLOOR = 10
/** Frame columns plus padding on each side of the content rows. */
const FRAME_X = 4

export type Git = {
  branch: string
  ahead: number
  behind: number
  added: number
  deleted: number
  modified: number
  untracked: number
}

export type TodoStatus = 'pending' | 'wip' | 'done' | 'defer'
export type Todo = { key: string; title: string; status: TodoStatus }
export type RateLimit = { pct: number; resetsAt?: string }

export type BandData = {
  model: string
  effort: string
  cwd: string
  root: string
  /** Input-side context tokens; undefined before the first request. */
  tokens?: number
  window: number
  cacheHit?: number
  cost?: number
  r5?: RateLimit
  r7?: RateLimit
  git: Git | null
  /** The clock the budget deltas read. */
  now: number
  /** The held key's own named-inbox address; undefined when it owns none or the read failed. */
  address?: string
  /** The held key's todos; undefined with no key or a failed read. */
  todos?: Todo[]
  /** The held key's agenda blob keys; undefined with no key or a failed read. */
  agenda?: string[]
}

export const EMPTY: BandData = { model: '', effort: '', cwd: '', root: '', window: 0, git: null, now: 0 }

export const C = {
  modelBg: '#5f005f',
  pillBg: '#262626',
  pillBg2: '#303030',
  fg: '#eeeeee',
  dim: '#8a8a8a',
  muted: '#6c6c6c',
  green: '#87d787',
  red: '#ff5f5f',
  amber: '#ffaf00',
  blue: '#5fafff',
  cost: '#d7d700',
  /** The label's text where it lies over the fill. */
  onFill: '#1c1c1c',
}

export const LCAP = ''
export const RCAP = ''

const LOW = ['#00ff00', '#5fff00', '#87ff00', '#afff00', '#d7ff00', '#ffff00']
const HIGH = ['#ffff00', '#ffd700', '#ffaf00', '#ff8700', '#ff5f00', '#ff0000']

/** A green -> yellow -> red gradient over 0-100. */
export function pctColor(p: number): string {
  const v = Math.max(0, Math.min(100, p))
  return (v <= 50 ? LOW[Math.round((v / 50) * 5)] : HIGH[Math.round(((v - 50) / 50) * 5)]) ?? '#ffffff'
}

export function commafy(n: number): string {
  return Math.trunc(n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

export function fmtMax(v: number): string {
  if (v >= 1e6) return v % 1e6 === 0 ? `${v / 1e6}M` : `${(v / 1e6).toFixed(1)}M`
  if (v >= 1e3 && v % 1e3 === 0) return `${v / 1e3}K`
  return String(v)
}

/** Usage against the linear safe line through the window; negative is under budget, "" on the line or with no reset. */
export function budgetDelta(used: number, resetsAt: string | undefined, windowSec: number, now: number): string {
  if (!resetsAt) return ''
  const reset = Date.parse(resetsAt)
  if (Number.isNaN(reset)) return ''
  const elapsed = Math.max(0, Math.min(windowSec, (now - (reset - windowSec * 1000)) / 1000))
  const d = Math.trunc(used - (elapsed / windowSec) * 100)
  return d > 0 ? `+${d}%` : d < 0 ? `${d}%` : ''
}

/**
 * The cells a terminal draws `t` in: an emoji (from U+1F300, and U+26A1) or an
 * East Asian wide character takes two, a combining mark or joiner none.
 */
export function cells(t: string): number {
  let n = 0
  for (const ch of t) {
    const cp = ch.codePointAt(0) ?? 0
    if ((cp >= 0x300 && cp <= 0x36f) || cp === 0x200d || (cp >= 0xfe00 && cp <= 0xfe0f)) continue
    const wide =
      cp >= 0x1f300 ||
      cp === 0x26a1 ||
      (cp >= 0x1100 && cp <= 0x115f) ||
      (cp >= 0x2e80 && cp <= 0xa4cf) ||
      (cp >= 0xac00 && cp <= 0xd7a3) ||
      (cp >= 0xf900 && cp <= 0xfaff) ||
      (cp >= 0xfe30 && cp <= 0xfe4f) ||
      (cp >= 0xff00 && cp <= 0xff60) ||
      (cp >= 0xffe0 && cp <= 0xffe6)
    n += wide ? 2 : 1
  }
  return n
}

/** The longest head of `t` that fits `w` cells. */
export function sliceCells(t: string, w: number): string {
  let out = ''
  let n = 0
  for (const ch of t) {
    const c = cells(ch)
    if (n + c > w) break
    out += ch
    n += c
  }
  return out
}

const BLK = ['▏', '▎', '▍', '▌', '▋', '▊', '▉']

export type Run = { text: string; color: string; bg?: string }

/**
 * A context bar of `w` cells: 1/8-block fill in `barColor`, with the label
 * pinned at cell 0. A label cell over the fill is drawn `C.onFill` on the fill
 * color, one past it keeps its run's color; the fill edge under the label
 * counts as filled from half a cell, and the 1/8 glyph draws only past the
 * label. `labels[0]` (the percent) always stays, clipped to `w` if need be;
 * the later segments drop from the last while the label overflows `w`.
 */
export function barRuns(pct: number, w: number, labels: Run[], barColor: string): Run[] {
  if (w <= 0) return []
  const v = Math.max(0, Math.min(100, pct))
  const filled = (v / 100) * w
  const full = Math.floor(filled)
  const frac = filled - full
  const width = (rs: Run[]) => rs.reduce((n, r) => n + cells(r.text), 0)
  let segs = labels
  while (segs.length > 1 && width(segs) > w) segs = segs.slice(0, -1)
  const head = segs[0]
  if (head && cells(head.text) > w) segs = [{ ...head, text: sliceCells(head.text, w) }]
  const out: Run[] = []
  const push = (text: string, color: string, bg?: string) => {
    const last = out[out.length - 1]
    if (last && last.color === color && last.bg === bg) last.text += text
    else out.push(bg === undefined ? { text, color } : { text, color, bg })
  }
  let i = 0
  for (const r of segs) {
    for (const ch of r.text) {
      const on = i < full || (i === full && frac >= 0.5)
      push(ch, on ? C.onFill : r.color, on ? barColor : undefined)
      i += cells(ch)
    }
  }
  for (; i < w; i++) {
    let ch = ' '
    if (i < full) ch = '█'
    else if (i === full) {
      const idx = Math.round(frac * 8)
      ch = idx >= 8 ? '█' : idx > 0 ? (BLK[idx - 1] ?? ' ') : ' '
    }
    push(ch, barColor)
  }
  return out
}

/** Whether each round cap of a `w`-cell bar takes the fill color: the fill reaches that end. */
export function barCaps(pct: number, w: number): { left: boolean; right: boolean } {
  if (w <= 0) return { left: false, right: false }
  const full = Math.floor((Math.max(0, Math.min(100, pct)) / 100) * w)
  return { left: full >= 1, right: full >= w }
}

/** `git status --porcelain -b` read into a Git, the change counts not yet filled; null for no header. */
export function parseGitStatus(stdout: string): Git | null {
  const [header = '', ...rest] = stdout.split('\n')
  if (!header.startsWith('## ')) return null
  let branch = header.slice(3).split('...')[0] ?? ''
  if (/^(HEAD \(no branch\)|No commits yet|Initial commit)/.test(branch)) branch = ''
  return {
    branch,
    ahead: Number(/ahead (\d+)/.exec(header)?.[1] ?? 0),
    behind: Number(/behind (\d+)/.exec(header)?.[1] ?? 0),
    added: 0,
    deleted: 0,
    modified: 0,
    untracked: rest.filter((l) => l.startsWith('??')).length,
  }
}

/** Add `git diff --numstat` rows to `git`'s change counts. */
export function addNumstat(git: Git, stdout: string): Git {
  const out = { ...git }
  for (const row of stdout.split('\n')) {
    const [a, d, f] = row.split('\t')
    if (!f) continue
    out.added += a === '-' ? 0 : Number(a) || 0
    out.deleted += d === '-' ? 0 : Number(d) || 0
    out.modified += 1
  }
  return out
}

type RunProcess = (argv: string[], init: { cwd: string; env: Record<string, string>; timeoutMs: number }) => Promise<{ exitCode: number | null; stdout: string }>

/** The working tree's branch and changes, read without taking git's optional locks; null outside a repository. */
export async function readGit(run: RunProcess, cwd: string): Promise<Git | null> {
  const env = { GIT_OPTIONAL_LOCKS: '0' }
  const status = await run(['git', 'status', '--porcelain', '-b'], { cwd, env, timeoutMs: 5000 })
  if (status.exitCode !== 0) return null
  const git = parseGitStatus(status.stdout)
  if (!git) return null
  const diff = await run(['git', '-c', 'diff.autoRefreshIndex=false', 'diff', '--numstat'], { cwd, env, timeoutMs: 5000 })
  return diff.exitCode === 0 ? addNumstat(git, diff.stdout) : git
}

const TODO_MARK: Record<string, TodoStatus> = { ' ': 'pending', '~': 'wip', x: 'done', '>': 'defer' }
const TODO_LINE = /^- \[([ ~x>])\] \{([^}]+)\} ?(.*)$/

/**
 * `todo_list` `mode: "full"` text read into todos: one `- [<mark>] {key}
 * title` line per item, an indented instruction line under some, and
 * `(no todos)` for an empty list. Undefined when the text holds neither.
 */
export function parseTodos(text: string): Todo[] | undefined {
  const lines = text.split('\n').filter((l) => l.trim() !== '')
  if (lines.length === 1 && lines[0]!.trim() === '(no todos)') return []
  const todos: Todo[] = []
  for (const line of lines) {
    const m = TODO_LINE.exec(line)
    if (m) todos.push({ key: m[2]!, title: m[3]!.trim(), status: TODO_MARK[m[1]!]! })
  }
  return todos.length > 0 ? todos : undefined
}

/** `agenda_list` text read into its blob keys: one `- <key>: <summary>` line each, `no agenda blobs` for none. */
export function parseAgenda(text: string): string[] | undefined {
  const lines = text.split('\n').filter((l) => l.trim() !== '')
  if (lines.length === 1 && lines[0]!.trim() === 'no agenda blobs') return []
  const keys = lines.flatMap((l) => {
    const m = /^- (.+?): /.exec(l)
    return m ? [m[1]!] : []
  })
  return keys.length > 0 ? keys : undefined
}

/**
 * What the ws rows draw from the todos: the focus (the first in-progress
 * todo, else the first pending one), the other open todos, and the count
 * (deferred todos count as settled, as the todo summary folds them).
 */
export function todoView(todos: readonly Todo[]): { focus?: Todo; rest: Todo[]; done: number; total: number } {
  const focus = todos.find((t) => t.status === 'wip') ?? todos.find((t) => t.status === 'pending')
  const rest = todos.filter((t) => t !== focus && (t.status === 'wip' || t.status === 'pending'))
  const done = todos.filter((t) => t.status === 'done' || t.status === 'defer').length
  return { focus, rest, done, total: todos.length }
}

export type Capsule = 'r5' | 'r7' | 'cost'
/** Row 2's right capsules drop in this order when the bar is at its floor. */
export const DROP_ORDER: readonly Capsule[] = ['cost', 'r7', 'r5']

/**
 * Row 2's split of `inner` cells: the bar's width and the right capsules
 * kept (each `width` is its cells, caps included; one cell between them and
 * before them). The bar takes the rest down to BAR_FLOOR, then capsules drop
 * in DROP_ORDER; with none left the bar takes what remains, however little.
 */
export function row2Layout(inner: number, right: ReadonlyArray<{ id: Capsule; width: number }>): { barW: number; right: Capsule[] } {
  const avail = Math.max(0, inner - 2)
  let kept = [...right]
  for (;;) {
    if (kept.length === 0) return { barW: avail, right: [] }
    const rightW = kept.reduce((n, r) => n + r.width, 0) + kept.length
    const barW = avail - rightW
    if (barW >= BAR_FLOOR) return { barW, right: kept.map((r) => r.id) }
    const drop = DROP_ORDER.find((id) => kept.some((r) => r.id === id))
    kept = kept.filter((r) => r.id !== drop)
  }
}

/** The rest-row chips that fit `avail` cells, one apart; the others fold into `+N`, whose room is kept. */
export function fitChips(keys: readonly string[], avail: number): { shown: string[]; more: number } {
  const chip = (k: string) => cells(`○ ${k}`)
  const total = keys.reduce((n, k, i) => n + chip(k) + (i > 0 ? 1 : 0), 0)
  if (total <= avail) return { shown: [...keys], more: 0 }
  const shown: string[] = []
  let used = 0
  for (const k of keys) {
    const more = keys.length - shown.length - 1
    const next = used + (shown.length > 0 ? 1 : 0) + chip(k)
    const fold = more > 0 ? 1 + cells(`+${more}`) : 0
    if (next + fold > avail) break
    shown.push(k)
    used = next
  }
  return { shown, more: keys.length - shown.length }
}

/**
 * The top edge's title in `W` cells: `╭─ <model> [<effort>] ─…─╮`. The effort
 * drops, then the model is cut, so at least one dash is left before `╮`.
 */
export function titleParts(model: string, effort: string, W: number): { model: string; effort: string; dashes: number } {
  const room = Math.max(0, W - 6)
  let e = effort
  if (e && cells(model) + cells(e) + 3 > room) e = ''
  const m = cells(model) > room ? sliceCells(model, room) : model
  const titleW = cells(m) + (e ? cells(e) + 3 : 0)
  return { model: m, effort: e, dashes: Math.max(0, W - 5 - titleW) }
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const hour = (iso?: string) => (iso && !Number.isNaN(Date.parse(iso)) ? `${String(new Date(iso).getHours()).padStart(2, '0')}H` : '')
const day = (iso?: string) => (iso && !Number.isNaN(Date.parse(iso)) ? (DAYS[new Date(iso).getDay()] ?? '') : '')
const deltaColor = (d: string) => (d.startsWith('+') ? C.red : d.startsWith('-') ? C.green : C.dim)

/** The rate-limit capsule's text between its caps: percent, reset hour or weekday, budget delta. */
export function limitText(r: RateLimit, reset: string, delta: string): string {
  return ` ${Math.trunc(r.pct)}%/${reset}${delta ? ` (${delta})` : ''} `
}

export const costText = (usd: number) => ` $${usd.toFixed(2)} `

type Elements = ReturnType<EngineInterface['ui']['resolve']>

/**
 * The band in `W` cells: a hand-drawn rounded frame (a Box border takes no
 * title) whose top edge carries the model, around rows that never wrap: each
 * content row is laid out at the inner width and clips what overflows it.
 */
export function drawBand(ui: Elements, d: BandData, W: number) {
  const { Box, Text } = ui
  const inner = Math.max(0, W - FRAME_X)

  const pill = (key: string, bg: string, ...children: RenderChildren[]) => (
    <Box key={key} flexShrink={0}>
      <Text color={bg}>{LCAP}</Text>
      <Text backgroundColor={bg} wrap="truncate-end">
        {children}
      </Text>
      <Text color={bg}>{RCAP}</Text>
    </Box>
  )

  // A pill taking the row's spare width; its text truncates first.
  const spanPill = (key: string, bg: string, ...children: RenderChildren[]) => (
    <Box key={key} flexDirection="row" flexGrow={1} flexShrink={1}>
      <Text color={bg}>{LCAP}</Text>
      <Box flexGrow={1} flexShrink={1} backgroundColor={bg}>
        <Text backgroundColor={bg} wrap="truncate-end">
          {children}
        </Text>
      </Box>
      <Text color={bg}>{RCAP}</Text>
    </Box>
  )

  const row = (key: string, ...children: RenderChildren[]) => (
    <Box key={key} flexDirection="row" justifyContent="space-between" width={inner} overflow="hidden">
      {children}
    </Box>
  )

  const rows: RenderChildren[] = []

  // Row 1: mailbox, folder | branch, changes.
  const { dirName, rel } = folderParts(d.cwd, d.root)
  const g = d.git
  rows.push(
    row(
      'status',
      <Box key="left" flexDirection="row" gap={1} flexGrow={1} flexShrink={1}>
        {d.address ? pill('mailbox', C.pillBg, <Text color={C.blue}>{` ✉ ${d.address} `}</Text>) : null}
        {spanPill(
          'folder',
          C.pillBg,
          <Text color={C.fg}>{` 📁 ${dirName}`}</Text>,
          rel ? <Text color={C.dim}>{` ${rel}`}</Text> : null,
          ' ',
        )}
      </Box>,
      g && g.branch ? (
        <Box key="right" flexDirection="row" gap={1} flexShrink={0} marginLeft={1}>
          {pill(
            'branch',
            C.pillBg,
            <Text color={C.green}>{` 🌿 ${g.branch}`}</Text>,
            g.ahead > 0 ? <Text color={C.green}>{` ↑${g.ahead}`}</Text> : null,
            g.behind > 0 ? <Text color={C.amber}>{` ↓${g.behind}`}</Text> : null,
            ' ',
          )}
          {pill(
            'changes',
            C.pillBg2,
            g.added + g.deleted + g.modified + g.untracked === 0 ? (
              <Text color={C.muted}>{' clean '}</Text>
            ) : (
              <Text>
                {' '}
                {g.added > 0 ? <Text color={C.green}>{`+${g.added} `}</Text> : null}
                {g.deleted > 0 ? <Text color={C.red}>{`-${g.deleted} `}</Text> : null}
                {g.modified > 0 ? <Text color={C.amber}>{`~${g.modified} `}</Text> : null}
                {g.untracked > 0 ? <Text color={C.blue}>{`?${g.untracked} `}</Text> : null}
              </Text>
            ),
          )}
        </Box>
      ) : null,
    ),
  )

  // Row 2: context bar | 5h, 7d, cost.
  const pct = d.tokens !== undefined && d.window > 0 ? (d.tokens / d.window) * 100 : 0
  const d5 = d.r5 ? budgetDelta(d.r5.pct, d.r5.resetsAt, 18000, d.now) : ''
  const d7 = d.r7 ? budgetDelta(d.r7.pct, d.r7.resetsAt, 604800, d.now) : ''
  const r5Text = d.r5 ? limitText(d.r5, hour(d.r5.resetsAt), d5) : ''
  const r7Text = d.r7 ? limitText(d.r7, day(d.r7.resetsAt), d7) : ''
  const candidates: { id: Capsule; width: number }[] = []
  if (d.r5) candidates.push({ id: 'r5', width: cells(r5Text) + 2 })
  if (d.r7) candidates.push({ id: 'r7', width: cells(r7Text) + 2 })
  if (d.cost !== undefined) candidates.push({ id: 'cost', width: cells(costText(d.cost)) + 2 })
  const { barW, right } = row2Layout(inner, candidates)
  const fill = pctColor(pct)
  const labels: Run[] = [
    // Kept in drop order: barRuns drops from the last, keeping the percent.
    { text: ` ${Math.trunc(pct)}%`, color: C.fg },
    { text: ` ${d.tokens === undefined ? '-' : commafy(d.tokens)}`, color: C.fg },
    { text: ` / ${fmtMax(d.window)}`, color: C.dim },
    ...(d.cacheHit !== undefined ? [{ text: ` ⚡${d.cacheHit.toFixed(1)}%`, color: pctColor(100 - d.cacheHit) }] : []),
  ]
  const runs = barRuns(pct, barW, labels, fill)
  const caps = barCaps(pct, barW)
  const limit = (key: string, r: RateLimit, reset: string, delta: string) =>
    pill(
      key,
      C.pillBg2,
      <Text color={pctColor(r.pct)}>{` ${Math.trunc(r.pct)}%`}</Text>,
      <Text color={C.dim}>/</Text>,
      <Text color={C.fg}>{reset}</Text>,
      delta ? <Text color={deltaColor(delta)}>{` (${delta})`}</Text> : null,
      ' ',
    )
  rows.push(
    row(
      'usage',
      <Box key="bar" flexDirection="row" flexShrink={0} width={barW + 2}>
        <Text color={caps.left ? fill : C.pillBg2}>{LCAP}</Text>
        <Text backgroundColor={C.pillBg2} wrap="truncate-end">
          {runs.map((r, i) =>
            r.bg ? (
              <Text key={String(i)} color={r.color} backgroundColor={r.bg}>
                {r.text}
              </Text>
            ) : (
              <Text key={String(i)} color={r.color}>
                {r.text}
              </Text>
            ),
          )}
        </Text>
        <Text color={caps.right ? fill : C.pillBg2}>{RCAP}</Text>
      </Box>,
      right.length > 0 ? (
        <Box key="limits" flexDirection="row" gap={1} flexShrink={0}>
          {right.includes('r5') && d.r5 ? limit('r5', d.r5, hour(d.r5.resetsAt), d5) : null}
          {right.includes('r7') && d.r7 ? limit('r7', d.r7, day(d.r7.resetsAt), d7) : null}
          {right.includes('cost') && d.cost !== undefined
            ? pill(
                'cost',
                C.modelBg,
                <Text color={C.cost} bold>
                  {costText(d.cost)}
                </Text>,
              )
            : null}
        </Box>
      ) : null,
    ),
  )

  // Row 3: focus todo | todo count. Row 4: other open todos | agenda.
  const view = d.todos ? todoView(d.todos) : undefined
  if (view && view.total > 0) {
    rows.push(
      row(
        'focus',
        <Box key="left" flexDirection="row" gap={2} flexShrink={1}>
          {view.focus ? (
            <Text key="key" color={C.fg} bold wrap="truncate-end">
              {`▶ ${view.focus.key}`}
            </Text>
          ) : null}
          {view.focus && view.focus.title ? (
            <Text key="title" color={C.fg} wrap="truncate-end">
              {view.focus.title}
            </Text>
          ) : null}
        </Box>,
        pill('todo', C.pillBg2, <Text color={C.fg}>{` todo ${view.done}/${view.total} `}</Text>),
      ),
    )
  }
  const agenda = d.agenda ?? []
  const restKeys = view ? view.rest.map((t) => t.key) : []
  if (restKeys.length > 0 || agenda.length > 0) {
    const agendaText = agenda.join(' · ')
    const rightW = agenda.length > 0 ? cells(' agenda ') + 2 + 1 + cells(agendaText) : 0
    const { shown, more } = fitChips(restKeys, Math.max(0, inner - (rightW > 0 ? rightW + 1 : 0)))
    rows.push(
      row(
        'rest',
        <Box key="left" flexDirection="row" gap={1} flexShrink={1}>
          {shown.map((k) => (
            <Text key={`chip:${k}`} color={C.muted} wrap="truncate-end">
              {`○ ${k}`}
            </Text>
          ))}
          {more > 0 ? (
            <Text key="more" color={C.muted}>
              {`+${more}`}
            </Text>
          ) : null}
        </Box>,
        agenda.length > 0 ? (
          <Box key="right" flexDirection="row" gap={1} flexShrink={0}>
            {pill('agenda', C.pillBg2, <Text color={C.fg}>{' agenda '}</Text>)}
            <Text color={C.dim} wrap="truncate-end">
              {agendaText}
            </Text>
          </Box>
        ) : null,
      ),
    )
  }

  const t = titleParts(d.model, d.effort, W)
  // The side bars follow the rows drawn, so the frame closes with or without the ws rows.
  const bars = rows.map((_, i) => (
    <Text key={String(i)} color={C.muted}>
      │
    </Text>
  ))
  return (
    <Box flexDirection="column" width={W}>
      <Box key="top" flexDirection="row">
        <Text color={C.muted}>{'╭─ '}</Text>
        <Text color={C.fg} bold>
          {t.model}
        </Text>
        {t.effort ? <Text color={C.dim}>{` [${t.effort}]`}</Text> : null}
        <Text color={C.muted}>{` ${'─'.repeat(t.dashes)}╮`}</Text>
      </Box>
      <Box key="body" flexDirection="row">
        <Box key="lbar" flexDirection="column" flexShrink={0}>
          {bars}
        </Box>
        <Box key="rows" flexDirection="column" flexShrink={0} width={inner + 2} paddingX={1}>
          {rows}
        </Box>
        <Box key="rbar" flexDirection="column" flexShrink={0}>
          {bars}
        </Box>
      </Box>
      <Text key="bottom" color={C.muted}>{`╰${'─'.repeat(Math.max(0, W - 2))}╯`}</Text>
    </Box>
  )
}

/** The folder capsule's name and its path below the repository root; either separator splits, for a Windows cwd. */
export function folderParts(cwd: string, root: string): { dirName: string; rel: string } {
  const dirName = cwd.split(/[\\/]/).pop() ?? ''
  const rel = root && cwd.startsWith(root) && /[\\/]/.test(cwd.charAt(root.length)) ? cwd.slice(root.length) : ''
  return { dirName, rel }
}

/**
 * The usage figures the band draws, from a `$.session.usage({ breakdown: 'summary' })` read.
 * The window is the one the session compacts at (the breakdown's
 * `rawMaxTokens`), else the model's.
 */
export function usageFigures(u: SessionUsage): Pick<BandData, 'tokens' | 'window' | 'cacheHit' | 'cost' | 'r5' | 'r7'> {
  const api = u.context.breakdown?.apiUsage
  const raw = u.context.breakdown?.rawMaxTokens
  const cached = api ? api.cache_read_input_tokens + api.cache_creation_input_tokens : 0
  const r5 = u.rateLimits.find((r) => r.kind === 'five_hour')
  const r7 = u.rateLimits.find((r) => r.kind === 'seven_day')
  return {
    tokens: u.context.tokens,
    window: raw !== undefined && raw > 0 ? raw : u.context.window,
    cacheHit: api && cached > 0 ? (api.cache_read_input_tokens / cached) * 100 : undefined,
    cost: u.cost?.usd,
    r5: r5 && { pct: r5.percentUsed, resetsAt: r5.resetsAt },
    r7: r7 && { pct: r7.percentUsed, resetsAt: r7.resetsAt },
  }
}

export type BandEdges = {
  /** One config_get's value, session-keyed when a key is given; a rejection reads as off. */
  readSetting: (key: string, sessionKey: string | undefined) => Promise<unknown>
  /** One ws MCP tool call's text; rejects on a failed call or an error result. */
  call: (tool: string, args: Record<string, unknown>) => Promise<string>
  /** The key's own named-inbox address; undefined when it owns none. May reject. */
  address: (sessionKey: string) => Promise<string | undefined>
  /** The working tree's git state at `cwd`; null outside a repository. May reject. */
  git: (cwd: string) => Promise<Git | null>
  usage: () => Promise<SessionUsage>
  model: () => Promise<string>
  cwd: () => Promise<string>
  root: () => Promise<string>
  now: () => Promise<number>
  /** Run `fn` every `ms`; the returned function stops it. */
  every: (ms: number, fn: () => void) => () => void
  /** Ask the engine to draw the band again: a figure changed. */
  redraw: () => void
  log: (text: string) => void
}

export type Band = {
  /** session.start: bind the edges, start over keyless, and read the knob (not awaited). */
  start: (edges: BandEdges) => void
  /** What the band draws, or undefined while it is off. */
  data: () => BandData | undefined
  /** The held session key changed (a login, a revive, an adoption): re-read the knob, the ws rows and the address. */
  keyChanged: (key: string) => void
  /**
   * A settled tool call. `tool` is its model-facing name and `server` the ws
   * MCP server's; `byModel` is false for the mod's own calls, which trigger
   * no ws or usage read.
   */
  toolCalled: (tool: string, server: string | undefined, byModel: boolean) => void
  /** A session.measure: the usage moved. */
  measured: () => void
  /** A main-loop model request: its model and effort. */
  stepped: (model: string, effort: string | number | undefined) => void
  /** A main-thread turn ended. */
  turnCompleted: () => void
  /** session.end: the next conversation gets its own key, so the key's rows and address go. */
  end: () => void
  /** Settles once every read started so far has settled (for tests). */
  idle: () => Promise<void>
}

export function createBand(): Band {
  let edges: BandEdges | undefined
  let on = false
  let key: string | undefined
  let d: BandData = { ...EMPTY }
  let stopTick: (() => void) | undefined
  // Each read applies only while it is the newest of its kind, so a read a
  // later key change or toggle overtook is dropped.
  let knobSeq = 0
  let wsSeq = 0
  let addrSeq = 0
  let gitAt = -Infinity
  let gitBusy = false
  let gitAgain = false
  let usageBusy = false
  let usageAgain = false
  const inflight = new Set<Promise<unknown>>()

  const log = (text: string) => {
    try {
      edges?.log(`ws band: ${text}`)
    } catch {
      // Logging is best effort.
    }
  }

  const redraw = () => {
    try {
      edges?.redraw()
    } catch (err) {
      log(`redraw failed: ${String(err)}`)
    }
  }

  const track = (label: string, p: Promise<unknown>) => {
    const run = p.catch((err) => log(`${label} failed: ${String(err)}`))
    inflight.add(run)
    void run.finally(() => inflight.delete(run))
  }

  const patch = (p: Partial<BandData>) => {
    d = { ...d, ...p }
    if (on) redraw()
  }

  const readWs = (e: BandEdges) => {
    const seq = ++wsSeq
    const k = key
    if (!on || k === undefined) {
      patch({ todos: undefined, agenda: undefined })
      return
    }
    track(
      'ws read',
      (async () => {
        let todos: Todo[] | undefined
        let agenda: string[] | undefined
        try {
          const [todoText, agendaText] = await Promise.all([
            e.call('todo_list', { session_key: k, mode: 'full' }),
            e.call('agenda_list', { session_key: k }),
          ])
          todos = parseTodos(todoText)
          agenda = parseAgenda(agendaText)
          if (todos === undefined || agenda === undefined) throw new Error('unreadable todo_list or agenda_list text')
        } catch (err) {
          log(`ws rows hidden: ${String(err)}`)
          todos = undefined
          agenda = undefined
        }
        if (seq !== wsSeq || k !== key || !on) return
        patch({ todos, agenda })
      })(),
    )
  }

  const readAddress = (e: BandEdges) => {
    const seq = ++addrSeq
    const k = key
    if (!on || k === undefined) {
      patch({ address: undefined })
      return
    }
    track(
      'address read',
      (async () => {
        const address = await e.address(k).catch((err) => {
          log(`mailbox address left out: ${String(err)}`)
          return undefined
        })
        if (seq !== addrSeq || k !== key || !on) return
        patch({ address })
      })(),
    )
  }

  /**
   * One usage read at a time, so a late answer never overwrites a newer one.
   * A read requested while one is in flight leaves one trailing read, however
   * many arrive. There is no time throttle: the read is a local estimate, and
   * the bar follows each settled model tool call.
   */
  const readUsage = (e: BandEdges) => {
    if (!on) return
    if (usageBusy) {
      usageAgain = true
      return
    }
    usageBusy = true
    track(
      'usage read',
      (async () => {
        try {
          do {
            usageAgain = false
            const figures = usageFigures(await e.usage())
            const now = await e.now()
            if (on) patch({ ...figures, now })
          } while (usageAgain && on)
        } finally {
          usageBusy = false
        }
      })(),
    )
  }

  const readGit = (e: BandEdges, force: boolean) => {
    if (!on) return
    track(
      'git read',
      (async () => {
        const now = await e.now()
        if (!force && now - gitAt < GIT_THROTTLE_MS) return
        if (gitBusy) {
          gitAgain = true
          return
        }
        gitBusy = true
        gitAt = now
        try {
          do {
            gitAgain = false
            const [cwd, root] = await Promise.all([e.cwd(), e.root()])
            const git = await e.git(cwd).catch((err) => {
              log(`git not read: ${String(err)}`)
              return null
            })
            if (on) patch({ cwd, root, git })
          } while (gitAgain && on)
        } finally {
          gitBusy = false
        }
      })(),
    )
  }

  /** The knob turned on: every figure is read afresh and the clock starts. */
  const turnOn = (e: BandEdges) => {
    on = true
    log('on')
    stopTick?.()
    stopTick = e.every(TICK_MS, () => {
      track(
        'tick',
        e.now().then((now) => {
          if (on) patch({ now })
        }),
      )
    })
    track(
      'model read',
      e.model().then((model) => {
        if (on && !d.model) patch({ model })
      }),
    )
    readUsage(e)
    readGit(e, true)
    readWs(e)
    readAddress(e)
    redraw()
  }

  const turnOff = () => {
    on = false
    stopTick?.()
    stopTick = undefined
    log('off')
    redraw()
  }

  /**
   * Read the knob with the held key, or keyless, and turn on or off on a
   * change: `turned-on` when this read turned it on (which reads every figure
   * itself), else whether it is on.
   */
  const readKnob = (e: BandEdges): Promise<'turned-on' | 'on' | 'off'> => {
    const seq = ++knobSeq
    const k = key
    const run = (async () => {
      let value: unknown
      try {
        value = await e.readSetting(ENABLED_KEY, k)
      } catch (err) {
        log(`${ENABLED_KEY} not read (${String(err)}); off`)
        value = undefined
      }
      if (seq !== knobSeq || edges !== e) return on ? 'on' : 'off'
      const want = value === true
      if (want && !on) {
        turnOn(e)
        return 'turned-on'
      }
      if (!want && on) turnOff()
      return want ? 'on' : 'off'
    })()
    track('knob read', run)
    return run
  }

  return {
    start(bound) {
      stopTick?.()
      stopTick = undefined
      edges = bound
      on = false
      key = undefined
      d = { ...EMPTY }
      gitAt = -Infinity
      wsSeq += 1
      addrSeq += 1
      void readKnob(bound)
    },
    data: () => (on ? d : undefined),
    keyChanged(next) {
      const e = edges
      if (!e || next === key) return
      key = next
      d = { ...d, todos: undefined, agenda: undefined, address: undefined }
      // A read that turns the band on reads the rows itself; one that finds
      // it on already reads them for the new key here.
      void readKnob(e).then((state) => {
        if (state === 'on' && key === next) {
          readWs(e)
          readAddress(e)
        }
      })
    },
    toolCalled(tool, server, byModel) {
      const e = edges
      if (!e) return
      const prefix = server ? `mcp__${server.replace(/[^A-Za-z0-9_-]/g, '_')}__` : undefined
      const name = prefix && tool.startsWith(prefix) ? tool.slice(prefix.length) : undefined
      if (byModel && name === 'config_tune') void readKnob(e)
      if (!on) return
      if (byModel && name !== undefined) {
        if (name.startsWith('todo_') || name.startsWith('agenda_')) readWs(e)
        if (name.startsWith('mailbox_')) readAddress(e)
      }
      readGit(e, false)
      // The mod's own calls make no model request, so the fill stays put.
      if (byModel) readUsage(e)
    },
    measured() {
      if (edges) readUsage(edges)
    },
    stepped(model, effort) {
      const next = { model, effort: effort === undefined ? '' : String(effort) }
      if (next.model === d.model && next.effort === d.effort) return
      patch(next)
    },
    turnCompleted() {
      if (edges) readGit(edges, true)
    },
    end() {
      key = undefined
      wsSeq += 1
      addrSeq += 1
      patch({ todos: undefined, agenda: undefined, address: undefined })
    },
    async idle() {
      while (inflight.size > 0) await Promise.all([...inflight])
    },
  }
}

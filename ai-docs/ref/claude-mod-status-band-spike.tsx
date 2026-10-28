// Reference copy of the 2026-10-28 status band spike, the starting point for
// 261028-feat-claude-mod-status-band. Not loaded by anything: the throwaway
// mod also had a plugin.json, hooks.json ({"modules":["./register.tsx"]})
// and a PluginState contract declaring 'statusline-spike': { line: Line | null };
// its Git / Line types are inlined below. The ws rows and the mailbox
// address are fixed strings here.

import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register, RenderChildren } from 'claude-code'

type Git = {
  branch: string
  ahead: number
  behind: number
  added: number
  deleted: number
  modified: number
  untracked: number
}

type Line = {
  model: string
  effort: string
  cwd: string
  root: string
  tokens?: number
  window: number
  cacheHit?: number
  cost?: number
  r5?: { pct: number; resetsAt?: string }
  r7?: { pct: number; resetsAt?: string }
  git: Git | null
  now: number
}

// Spike: shell/statusline.sh redrawn as an AbovePrompt band, two rows, each
// split into a left and a right span by justifyContent: space-between.

const line = atom({ plugin: 'statusline-spike', key: 'line' } as const, null)

const C = {
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
}

const LCAP = ''
const RCAP = ''

const LOW = ['#00ff00', '#5fff00', '#87ff00', '#afff00', '#d7ff00', '#ffff00']
const HIGH = ['#ffff00', '#ffd700', '#ffaf00', '#ff8700', '#ff5f00', '#ff0000']

// statusline.sh's green -> yellow -> red gradient.
function pctColor(p: number): string {
  const v = Math.max(0, Math.min(100, p))
  return (v <= 50 ? LOW[Math.round((v / 50) * 5)] : HIGH[Math.round(((v - 50) / 50) * 5)]) ?? '#ffffff'
}

function commafy(n: number): string {
  return Math.trunc(n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

function fmtMax(v: number): string {
  if (v >= 1e6) return v % 1e6 === 0 ? `${v / 1e6}M` : `${(v / 1e6).toFixed(1)}M`
  if (v >= 1e3 && v % 1e3 === 0) return `${v / 1e3}K`
  return String(v)
}

// Usage against the linear safe line; negative is under budget.
function budgetDelta(used: number, resetsAt: string | undefined, windowSec: number, now: number): string {
  if (!resetsAt) return ''
  const reset = Date.parse(resetsAt)
  const elapsed = Math.max(0, Math.min(windowSec, (now - (reset - windowSec * 1000)) / 1000))
  const d = Math.trunc(used - (elapsed / windowSec) * 100)
  return d > 0 ? `+${d}%` : d < 0 ? `${d}%` : ''
}

const BLK = ['▏', '▎', '▍', '▌', '▋', '▊', '▉']

type Run = { text: string; color: string }

// Cells of a string; an emoji or other wide glyph takes two.
function cells(t: string): number {
  let n = 0
  for (const ch of t) {
    const cp = ch.codePointAt(0) ?? 0
    n += cp >= 0x1f300 || cp === 0x26a1 ? 2 : 1
  }
  return n
}

// statusline.sh's full-width context bar (1a68831e8): 1/8-block fill with
// the label laid over it just past the fill edge, clamped to fit.
function barRuns(pct: number, w: number, labels: Run[], barColor: string): Run[] {
  const v = Math.max(0, Math.min(100, pct))
  const filled = (v / 100) * w
  const full = Math.floor(filled)
  const frac = filled - full
  const labelW = labels.reduce((n, r) => n + cells(r.text), 0)
  let lpos = full + (frac > 0.0625 ? 1 : 0)
  if (lpos + labelW > w) lpos = w - labelW
  if (lpos < 0) lpos = 0
  const out: Run[] = []
  const push = (text: string, color: string) => {
    const last = out[out.length - 1]
    if (last && last.color === color) last.text += text
    else out.push({ text, color })
  }
  for (let i = 0; i < w; ) {
    if (i === lpos && labelW > 0) {
      for (const r of labels) push(r.text, r.color)
      i += labelW
      continue
    }
    let ch = ' '
    if (i < full) ch = '█'
    else if (i === full && full < w) {
      const idx = Math.round(frac * 8)
      ch = idx >= 8 ? '█' : idx > 0 ? (BLK[idx - 1] ?? ' ') : ' '
    }
    push(ch, barColor)
    i++
  }
  return out
}

async function readGit($: EngineInterface, cwd: string): Promise<Git | null> {
  const env = { GIT_OPTIONAL_LOCKS: '0' }
  const status = await $.process.run(['git', 'status', '--porcelain', '-b'], { cwd, env, timeoutMs: 5000 })
  if (status.exitCode !== 0) return null
  const [header = '', ...rest] = status.stdout.split('\n')
  let branch = header.replace(/^## /, '').split('...')[0] ?? ''
  if (/^(HEAD \(no branch\)|No commits yet|Initial commit)/.test(branch)) branch = ''
  const git: Git = {
    branch,
    ahead: Number(/ahead (\d+)/.exec(header)?.[1] ?? 0),
    behind: Number(/behind (\d+)/.exec(header)?.[1] ?? 0),
    added: 0,
    deleted: 0,
    modified: 0,
    untracked: rest.filter(l => l.startsWith('??')).length,
  }
  const diff = await $.process.run(['git', '-c', 'diff.autoRefreshIndex=false', 'diff', '--numstat'], {
    cwd,
    env,
    timeoutMs: 5000,
  })
  for (const row of diff.stdout.split('\n')) {
    const [a, d, f] = row.split('\t')
    if (!f) continue
    git.added += a === '-' ? 0 : Number(a)
    git.deleted += d === '-' ? 0 : Number(d)
    git.modified += 1
  }
  return git
}

const EMPTY: Line = { model: '', effort: '', cwd: '', root: '', window: 0, git: null, now: 0 }

let gitAt = 0

function patch($: EngineInterface, p: Partial<Line>) {
  return update($, line, prev => ({ ...(prev ?? EMPTY), ...p }))
}

async function refreshGit($: EngineInterface, force = false) {
  const now = await $.clock.now()
  if (!force && now - gitAt < 2000) return
  gitAt = now
  const cwd = await $.session.cwd()
  await patch($, { git: await readGit($, cwd).catch(() => null), now })
}

async function refreshUsage($: EngineInterface) {
  const u = await $.session.usage({ breakdown: 'summary' })
  const api = u.context.breakdown?.apiUsage
  const cached = api ? api.cache_read_input_tokens + api.cache_creation_input_tokens : 0
  const r5 = u.rateLimits.find(r => r.kind === 'five_hour')
  const r7 = u.rateLimits.find(r => r.kind === 'seven_day')
  await patch($, {
    tokens: u.context.tokens,
    window: u.context.window,
    cacheHit: api && cached > 0 ? (api.cache_read_input_tokens / cached) * 100 : undefined,
    cost: u.cost?.usd,
    r5: r5 && { pct: r5.percentUsed, resetsAt: r5.resetsAt },
    r7: r7 && { pct: r7.percentUsed, resetsAt: r7.resetsAt },
    now: await $.clock.now(),
  })
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    await patch($, { model: await $.session.model(), cwd: await $.session.cwd(), root: await $.session.root() })
    await refreshUsage($).catch(() => undefined)
    await refreshGit($, true)
    $.clock.every(10_000, () => {
      void refreshGit($)
      void $.clock.now().then(now => patch($, { now }))
    })
    return started
  })

  on('session.measure', async ($, e, next) => {
    await refreshUsage($).catch(() => undefined)
    return next(e)
  })

  // The effort lives on each model request only; take the main loop's.
  on('turn.step', async function* ($, e, next) {
    if (e.agentId === undefined) {
      void patch($, { model: e.model, effort: e.effort === undefined ? '' : String(e.effort) })
    }
    return yield* next(e)
  })

  on('tool.call', async ($, e, next) => {
    const result = await next(e)
    void refreshGit($)
    return result
  })

  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    void refreshGit($, true)
    return result
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const l = await read($, line)
    if (e.props.hasSurvey || l === null) return next(e)

    const { Box, Text } = $.ui.resolve(e)

    const pill = (bg: string, ...children: RenderChildren[]) => (
      <Box flexShrink={0}>
        <Text color={bg}>{LCAP}</Text>
        <Text backgroundColor={bg} wrap="truncate-end">
          {children}
        </Text>
        <Text color={bg}>{RCAP}</Text>
      </Box>
    )

    // A pill that takes the row's spare width.
    const spanPill = (bg: string, ...children: RenderChildren[]) => (
      <Box flexDirection="row" flexGrow={1} flexShrink={1}>
        <Text color={bg}>{LCAP}</Text>
        <Box flexGrow={1} flexShrink={1} backgroundColor={bg}>
          <Text backgroundColor={bg} wrap="truncate-end">
            {children}
          </Text>
        </Box>
        <Text color={bg}>{RCAP}</Text>
      </Box>
    )

    const dirName = l.cwd.split('/').pop() ?? ''
    const rel = l.cwd.startsWith(`${l.root}/`) ? l.cwd.slice(l.root.length) : ''
    const pct = l.tokens !== undefined && l.window > 0 ? (l.tokens / l.window) * 100 : 0
    const d5 = l.r5 ? budgetDelta(l.r5.pct, l.r5.resetsAt, 18000, l.now) : ''
    const d7 = l.r7 ? budgetDelta(l.r7.pct, l.r7.resetsAt, 604800, l.now) : ''
    const deltaColor = (d: string) => (d.startsWith('+') ? C.red : d.startsWith('-') ? C.green : C.dim)
    const hour = (iso?: string) => (iso ? `${String(new Date(iso).getHours()).padStart(2, '0')}H` : '')
    const day = (iso?: string) => (iso ? ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'][new Date(iso).getDay()] : '')
    const g = l.git
    // Hand-drawn frame: Box borders take no title, so the top edge is a Text
    // row carrying the model as a border title.
    const W = e.props.bodyColumns
    const titleW = l.model.length + (l.effort ? l.effort.length + 3 : 0)
    // Row 2's right pills at their natural width; the bar takes the rest
    // (frame 2 + padding 2, caps 2, one gap).
    const rightTexts = [
      l.r5 ? ` ${Math.trunc(l.r5.pct)}%/${hour(l.r5.resetsAt)}${d5 ? ` (${d5})` : ''} ` : '',
      l.r7 ? ` ${Math.trunc(l.r7.pct)}%/${day(l.r7.resetsAt)}${d7 ? ` (${d7})` : ''} ` : '',
      l.cost !== undefined ? ` $${l.cost.toFixed(2)} ` : '',
    ].filter(t => t)
    const rightW = rightTexts.reduce((n, t) => n + cells(t) + 2, 0) + Math.max(0, rightTexts.length - 1)
    const barW = Math.max(10, W - 4 - 2 - rightW - (rightW > 0 ? 1 : 0))
    const labels: Run[] = [
      { text: ` ${Math.trunc(pct)}% ${l.tokens === undefined ? '-' : commafy(l.tokens)}`, color: C.fg },
      { text: ` / ${fmtMax(l.window)}`, color: C.dim },
      ...(l.cacheHit !== undefined ? [{ text: ` ⚡${l.cacheHit.toFixed(1)}%`, color: pctColor(100 - l.cacheHit) }] : []),
      { text: ' ', color: C.dim },
    ]
    const runs = barRuns(pct, barW, labels, pctColor(pct))
    const barCells = [...runs.map(r => (r.color === pctColor(pct) ? r.text : ' '.repeat(cells(r.text)))).join('')]
    const bars = [0, 1, 2, 3].map(i => <Text key={String(i)} color={C.muted}>│</Text>)

    return (
      <Box flexDirection="column" width={W}>
        <Box flexDirection="row">
          <Text color={C.muted}>╭─ </Text>
          <Text color={C.fg} bold>
            {l.model}
          </Text>
          {l.effort ? <Text color={C.dim}> [{l.effort}]</Text> : null}
          <Text color={C.muted}> {'─'.repeat(Math.max(0, W - 5 - titleW))}╮</Text>
        </Box>
        <Box flexDirection="row">
          <Box flexDirection="column" flexShrink={0}>
            {bars}
          </Box>
          <Box flexDirection="column" flexGrow={1} flexShrink={1} paddingX={1}>
        <Box flexDirection="row" justifyContent="space-between" gap={1}>
          <Box flexDirection="row" gap={1} flexGrow={1} flexShrink={1}>
            {pill(C.pillBg, <Text color={C.blue}> ✉ claude-devenv-a1b2 </Text>)}
            {spanPill(C.pillBg, <Text>
              <Text color={C.fg}> 📁 {dirName}</Text>
              {rel ? <Text color={C.dim}> {rel}</Text> : null}
              <Text> </Text>
            </Text>)}
          </Box>
          {g && g.branch ? (
            <Box flexDirection="row" gap={1} flexShrink={0}>
              {pill(C.pillBg, <Text>
                <Text color={C.green}> 🌿 {g.branch}</Text>
                {g.ahead > 0 ? <Text color={C.green}> ↑{g.ahead}</Text> : null}
                {g.behind > 0 ? <Text color={C.amber}> ↓{g.behind}</Text> : null}
                <Text> </Text>
              </Text>)}
              {pill(C.pillBg2, <Text>
                {g.added + g.deleted + g.modified + g.untracked === 0 ? (
                  <Text color={C.muted}> clean </Text>
                ) : (
                  <Text>
                    {' '}
                    {g.added > 0 ? <Text color={C.green}>+{g.added} </Text> : null}
                    {g.deleted > 0 ? <Text color={C.red}>-{g.deleted} </Text> : null}
                    {g.modified > 0 ? <Text color={C.amber}>~{g.modified} </Text> : null}
                    {g.untracked > 0 ? <Text color={C.blue}>?{g.untracked} </Text> : null}
                  </Text>
                )}
              </Text>)}
            </Box>
          ) : null}
        </Box>
        <Box flexDirection="row" justifyContent="space-between">
          <Box flexDirection="row" flexShrink={0}>
            {/* The caps take the fill color where the fill reaches them. */}
            <Text color={barCells[0] === '█' ? pctColor(pct) : C.pillBg2}>{LCAP}</Text>
            <Text backgroundColor={C.pillBg2}>
              {runs.map((r, i) => (
                <Text key={String(i)} color={r.color}>
                  {r.text}
                </Text>
              ))}
            </Text>
            <Text color={barCells[barCells.length - 1] === '█' ? pctColor(pct) : C.pillBg2}>{RCAP}</Text>
          </Box>
          <Box flexDirection="row" gap={1} flexShrink={0}>
            {l.r5 ? (
              pill(C.pillBg2, <Text>
                <Text color={pctColor(l.r5.pct)}> {Math.trunc(l.r5.pct)}%</Text>
                <Text color={C.dim}>/</Text>
                <Text color={C.fg}>{hour(l.r5.resetsAt)}</Text>
                {d5 ? <Text color={deltaColor(d5)}> ({d5})</Text> : null}
                <Text> </Text>
              </Text>)
            ) : null}
            {l.r7 ? (
              pill(C.pillBg2, <Text>
                <Text color={pctColor(l.r7.pct)}> {Math.trunc(l.r7.pct)}%</Text>
                <Text color={C.dim}>/</Text>
                <Text color={C.fg}>{day(l.r7.resetsAt)}</Text>
                {d7 ? <Text color={deltaColor(d7)}> ({d7})</Text> : null}
                <Text> </Text>
              </Text>)
            ) : null}
            {l.cost !== undefined ? (
              pill(C.modelBg, <Text>
                <Text color={C.cost} bold>
                  {' '}${l.cost.toFixed(2)}{' '}
                </Text>
              </Text>)
            ) : null}
          </Box>
        </Box>
        <Box flexDirection="row" justifyContent="space-between">
          <Box flexDirection="row" gap={1} flexShrink={1}>
            <Text color={C.fg} bold>▶ statusline-band-ux</Text>
            <Text color={C.dim} wrap="truncate-end">Discuss the claude mod status band's detailed UX</Text>
          </Box>
          {pill(C.pillBg2, <Text color={C.fg}> todo 2/5 </Text>)}
        </Box>
        <Box flexDirection="row" justifyContent="space-between">
          <Box flexDirection="row" gap={1} flexShrink={1}>
            <Text color={C.muted}>○ agents-md-followups</Text>
          </Box>
          <Box flexDirection="row" gap={1} flexShrink={0}>
            {pill(C.pillBg2, <Text color={C.fg}> agenda </Text>)}
            <Text color={C.dim}>plan · review</Text>
          </Box>
        </Box>
          </Box>
          <Box flexDirection="column" flexShrink={0}>
            {bars}
          </Box>
        </Box>
        <Text color={C.muted}>╰{'─'.repeat(Math.max(0, W - 2))}╯</Text>
      </Box>
    )
  })
}

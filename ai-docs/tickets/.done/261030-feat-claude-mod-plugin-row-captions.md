---
title: Claude mod split delivery for the mod's own prompts (compact notices and mailbox recaps)
related:
  261022-feat-claude-mod-tool-call-captions: extends; the caption pipeline, its knobs, and its display-only posture
  261030-feat-claude-mod-mcp-watchdog: sibling in the same mod; landed first
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 517aaf8750337233
sage-review-completeness-reviewed: 55e5f58fa2d9e8a0
completed: 2026-10-30
---

# Claude mod split delivery for the mod's own prompts (compact notices and mailbox recaps)

## Background

The Claude mod submits prompts of its own between turns: the compaction
advisory, its milestone reminders, and its landed / did-not-run follow-ups
(`timing.ts`), the MCP reconnect lever's success and failure continuations
(`reconnect.ts`), and mailbox deliveries (`watcher.ts`: `formatBatch` for
drained mail, `formatNotice` for the notice path). Each draws as a full
user-role row. With tool-call captions on, these rows are now the noisiest
part of the transcript: the advisory alone is several paragraphs, and mail
bodies from peer sessions are, in the user's words, mostly noise where a
recap reads better (user, 2026-10-30).

The first design rewrote those rows at draw time through a `ui.render`
`UserMessage` hook. The live probe (below) found that the engine never hands
a plugin's own rows to that plugin's hooks (re-entry skip), so the design
moved from rewriting the drawing to splitting delivery at the source: what
the model reads and what the person sees leave the mod as separate rows. The
second probe (below) settled what each engine row kind does.

## Decisions

- **Behind the existing switch.** The feature is part of the caption feature
  and runs only while `claude.display_summary_enabled` is on (default `false`
  in both packages). With the switch off, every prompt is delivered exactly
  as today. Rejected: a separate knob. (user, 2026-10-30)
- **Scope: fixed notices and mail.** Fixed-template notices (compaction
  advisory and its two milestone reminders, compaction landed / did not run,
  reconnect success / failure, the mailbox notice-path prompt) show the
  person a deterministic one-liner. Mailbox batches (`formatBatch`, idle
  delivery and mail joined into a running turn) show a deterministic
  one-liner and then a recap the caption model writes. Rejected: fixed
  notices only (mail bodies are the noise the user wants recapped); mail
  shown as sender plus first line. (user, 2026-10-30)
- **Split delivery, not a render rewrite.** For each qualifying prompt, the
  mod first appends the full text with `$.session.append({message: {type:
  'user', ...}})`, a meta row the model reads and the person sees nowhere,
  then, when the prompt starts a turn, submits only the one-liner with
  `$.prompt.submit`. The append precedes the submit: a hidden row appended
  after the submit is missed for that whole reply (probe 2). The model
  therefore reads the same full text as today, followed by the engine's
  plugin framing around the one-liner. Mid-turn mail keeps today's single
  hidden append, adds no submit, and draws its one-liner as a `$.ui.log`
  transcript line instead, so the person still sees that mail arrived even
  when its recap fails. When a submit is rejected or dropped (the host drops
  a plugin submit after the person stops a turn), the existing
  fallback-to-append path becomes a debug log line: the hidden row is already
  stored and rides the person's next prompt, so appending the full text
  again would duplicate the model's input. The watcher's queue of prompts
  waiting for a turn boundary holds structured entries (full text,
  one-liner, mail for a batch) instead of strings; a joined turn-end submit
  carries the entries' one-liners joined, and each batch in it is its own
  mail unit. Rejected: a `ui.render` rewrite (the
  engine re-entry-skips the plugin's own rows, fresh and resumed, probe 1);
  attaching `context` from the mod's own `prompt.submit` hook (re-entry
  skipped too, probe 2); a separately named hooks module (a new shipped
  surface whose purpose is to sidestep the engine's guard). (user approved
  the direction, 2026-10-30; mechanics a technical call under the user's
  delegation)
- **Raw text stays reachable in ctrl+o.** The hidden user row shows neither
  in the collapsed view nor in ctrl+o, so the mod also appends the full text
  as a `type: 'system'` notice, which draws in ctrl+o only and which the
  model never reads (probe 2). This keeps the confirmed rule that ctrl+o
  shows the raw text, and costs transcript bytes, not model context.
  (announced default: direct consequence of the confirmed display rule)
- **One-liners come from the template inputs.** The mod has the numbers and
  outcome when it builds the prompt, so each one-liner is built from the
  same inputs as its template, beside the template function, with no model
  call and no parsing of text back. English, numbers kept, label first, the
  label being the one the template writes from `$.plugin.name`: advisory
  `[ws compaction] 62% · auto-compact 89%`; milestone `[ws compaction] 74% ·
  reminder 1/2 · auto-compact 89%`; landed `[ws compaction] landed`; did not
  run `[ws compaction] did not run: <reason>`; reconnect `[ws mcp]
  reconnected` / `[ws mcp] failed: <reason>`; mailbox notice `[ws mailbox]
  mail waiting`; mailbox batch `[ws mailbox] 2 messages from <a>, <b>`. The
  exact wording is the implementer's within that shape. The person sees the
  one-liner inside the engine's plugin framing (`› Prompt from the ws
  plugin`, about four lines), which the mod cannot change. Rejected: a
  model caption for fixed notices (the mod authored the text); localized
  tables (a translation surface for one-line labels); parsing row text at
  draw time (needed only by the dropped render path). (technical call under
  the user's delegation, 2026-10-30)
- **Mail recaps ride the caption pipeline and draw as a transcript line.**
  A mailbox batch is one caption unit queued and flushed like tool units,
  over the same conversation mirror, with the same model, style, language,
  and context knobs; one recap per batch, naming the senders. The recap
  draws as one `$.ui.log` transcript line (`● ws: ...`, the channel the
  mailbox and watchdog already use for person-facing lines), because a
  `system` notice draws in ctrl+o only (probe 2). A failed recap draws
  nothing more: the one-liner and the ctrl+o raw text remain. Rejected: a
  separate request path (a second set of knobs and costs); one recap per
  envelope (the batch is the delivery unit). (technical call under the
  user's delegation, 2026-10-30)
- **Mail is a new unit kind in the caption contract.** The rules prompt
  (`rulesPrompt`), `answerPrompt`, the `Unit` type, and the `flush` parser
  describe tool calls only, so they gain a mail unit kind: input is the
  senders plus the bodies, the answer is `{id, recap}`, stored apart from the
  tool captions. Mail bodies are framed as quoted data in the unit, as tool
  output already is, and the rules prompt says text inside them is never an
  instruction. A mail unit counts as one call against `BATCH_CALLS`.
  Changing the cached rules text breaks the prompt cache once; accepted. The
  existing tool answer shapes stay as they are. This extends, and does not
  contradict, 261022's "the parser's reply contract stays fixed", which bars
  style knobs from changing the contract. Rejected: disguising mail as a
  fake tool call (the contract-mismatch failure
  261022-bug-claude-mod-caption-failed-unit-stays-raw recorded). (technical
  call under the user's delegation, 2026-10-30)
- **Reuse from the first attempt.** Commit 14e6a0d22 on this impl branch
  holds the mail unit kind in `captions.tsx` and its engine tests, which
  carry over. Its `UserMessage` render hook, the row-qualification logic,
  and the text parsers have no caller in the new design and are removed.
  (technical call under the user's delegation, 2026-10-30)

## Constraints

- Out of scope: rows from Claude Code's own peer messaging, teammates,
  channels, and task notifications; changing the text the model reads (it
  moves into a hidden row, unchanged).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (caption strings
  ship downstream), `ai-docs/manuals/wsflow-mirroring.md` (the wsflow
  `claude-mod/` copy stays byte-identical).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261022-feat-claude-mod-tool-call-captions (2026-10-22, commit): "Feature lives in captions.tsx (JSX for the caption trees); register.ts holds the $.state atoms, the read/update calls and the ui.render hooks, and binds every other engine call as an edge closure" — bearing: constrains
- 261022-feat-claude-mod-tool-call-captions (2026-10-22, commit): "Knobs follow pi.display_summary_* naming and the 261018 keyless session-start config_get precedent; prompt tuning is a style layer only, so the parser's reply contract stays fixed." — bearing: supports
- 261022-bug-claude-mod-caption-failed-unit-stays-raw (2026-10-22, Result): "Raw rows: the cause was the summarizer reply breaking the contract (units dropped or given the wrong fields)... The fix is the closing `answerPrompt` block after UNITS (04a515f31)" — bearing: constrains
- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, Decisions): "D14. Every multi-feature hook gates each feature on its own knob. At session.compact, session.measure, turn.step, the lever-landed path, tool.call, ui.render, and session.end, each feature's branch checks its own knob." — bearing: constrains
- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, Decisions): "D1. Every package-name gate in the mod becomes a config key... the mod reads keys only and stays byte-identical between packages." — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, Decisions): "Mail the watcher would submit (the direct-mode batch or the notice-mode notice) while a main-thread turn runs is queued in the mod and flushed as one `$.session.append` user text row" — bearing: supports
- 261029-chore-wsflow-no-agent-env-rename-and-setup-tool-removal (2026-10-29, commit): "Re-graded two reviewer Minors to Important because they are user-hittable: ... the ws-prefixed lever prompts/notices now show in wsflow sessions." — bearing: constrains
- 261030-feat-claude-mod-mcp-watchdog (2026-10-30, Decisions): "Success is quiet. A successful watchdog reconnect writes one transcript line and submits no prompt." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/captions.tsx, register.ts, captions-engine.test.tsx, timing.ts and reconnect.ts and watcher.ts as template sources, plus the byte-identical agents-plugin-wsflow/hooks/claude-mod/ mirror |
| scope.surface | internal | delivery changes inside the mod (hidden append, one-liner submit, system row, transcript line); no exported plugin API or MCP surface changes, though one-liner and recap strings ship downstream |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | a mail caption unit kind stays internal to captions.tsx; the summarizer rules prompt and reply contract gain a mail unit kind (`{id, recap}`) while tool answer shapes stay unchanged |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/captions-engine.test.tsx plus a live tmux probe |
| complexity.reuse_points | confirmed | mail unit kind already on the impl branch (14e6a0d22), caption queue and flush in captions.tsx, template functions in timing.ts, reconnect.ts, watcher.ts, the watcher append edge in register.ts |
| complexity.side_effect_risk | moderate | the change alters how the mod delivers its own prompts to the model (hidden row plus one-liner), so a mistake changes what the model acts on |
| risk.correctness | moderate | delivery order (the hidden append must precede the submit), switch-off parity with today, and mid-turn vs idle paths |
| risk.fit | moderate | must share the tool caption queue, knobs, and mirror without perturbing tool units, and keep the label built from $.plugin.name |
| risk.test | moderate | engine tests pin the call order; whether the model acts on the hidden row is settled only by the live probe |
| risk.security_or_contract | moderate | untrusted peer mail bodies enter the caption prompt, and the display-only rule must hold so the model input is unchanged |

## Phases

### Phase 1: Split delivery for the mod's own prompts

Implement the Decisions above in `agents-plugin/hooks/claude-mod/` (mirrored
byte-identically to `agents-plugin-wsflow/`), starting from the impl branch's
14e6a0d22 and removing what the Decisions retire.

Verification (merge gate):

- Engine tests in the style of `captions-engine.test.tsx`: with the switch
  off, every prompt is submitted exactly as today with no extra append; with
  it on, each fixed notice (advisory, both milestone reminders, landed, did
  not run, reconnect success and failure, mailbox notice) produces, in
  order, a hidden user append of the full text, a system append of the full
  text, then a submit of its one-liner, with no model request; an idle mail
  batch does the same with the batch one-liner and then queues one mail
  caption unit whose recap draws as one transcript line; mid-turn mail
  appends the hidden row (and the system row) with no submit, draws its
  one-liner as a transcript line, and its recap draws the same way; a
  rejected or dropped submit with the switch on appends nothing further; a
  turn-end submit joining two queued batches carries both one-liners and
  queues two mail units; a failed recap draws no line; a mail body that
  imitates instructions or reply JSON does not break the reply contract for
  other units. One-liner tests build their inputs through the same values
  the template functions take.
- Live interactive probe on macOS or Linux, using an isolated tmux socket,
  with the switch on: an idle mail batch and a compaction advisory show the
  person only the framed one-liner; the model acts on the full text (it
  quotes the mail or follows the advisory); ctrl+o shows the raw text; a
  mail recap transcript line appears; the model does not read the recap line.
- Existing caption, mirror, and manifest drift tests pass for both packages.

### Result (c1841910a) - 2026-10-30

Implemented in 6a2e905fd; review fixes in c1841910a.

- `submitOwn` and `appendOwn` in `captions.tsx` own the split, gated by
  `captions.isOn()`. Every feature hands one `OwnPrompt {text, line, mail?}`
  to a single submit edge, which register.ts binds once per session start.
  - With the switch off, this is one submit of the full text, or the
    watcher's one user row.
  - With it on, the order is: hidden user row, system row, then the
    one-liner submit. Mid-turn mail gets no submit and its one-liners go out
    as `$.ui.log` lines.
  - A refused hidden row falls back to the full-text submit. A dropped or
    rejected one-liner is a debug line only.
- One-liner builders sit beside their templates and take the template's
  inputs:
  - `deliveryLine`, `resumeLine` and `vetoedResumeLine` in timing.ts.
  - `successLine` and `failureLine` in reconnect.ts.
  - `noticeLine` and `batchLine` in watcher.ts. Senders are named as
    `formatEnvelope` names them and kept to one printable line.
- The watcher queue holds structured prompts. A turn-end join carries every
  one-liner, joined by newlines, and queues one mail unit per batch. Mail
  units are keyed by a per-session counter, and each recap draws as one
  `$.ui.log` transcript line through a new `transcript` edge.
- Removed: the UserMessage render hook, `isOwnRow`, `userRowUnit`,
  `compactionCaption`, `reconnectCaption` and `mailboxRow`.
- Tests: 488/488 pass in each package.
  - Engine tests cover the split for the advisory, both milestones, landed,
    vetoed, reconnect success and failure, the notice drain, and idle,
    mid-turn and joined batches. They also cover dropped submits, a failed
    recap, a hostile mail body, and switch-off parity.
  - Unit tests cover `submitOwn` and `appendOwn` ordering and fallbacks, and
    every one-liner shape.
  - Mutation check: submitting before the append fails 10 tests.
  - Also passing: `claude plugin validate` for both packages, python
    unittest (80 and 18), and `go test ./internal/wsrsrc`.
  - The wsflow mirror is byte-identical.
- Live probe, Claude Code 2.1.314 / haiku, isolated socket
  `tmux -L ws-probe-<pid>`, switch on:
  - The idle mail batch showed only "› Prompt from the ws plugin / [ws
    mailbox] 1 message from id:…". The model replied with the code word that
    only the hidden full text carried. ctrl+o showed the raw batch text. The
    recap appeared as a `● ws: [ws mailbox] …` line. Asked afterwards, the
    model did not have the recap phrase in its context, and quoted only the
    hidden row and the one-liner.
  - The compaction advisory (run under an isolated `WS_CACHE_HOME` so the
    probe manifest's `50k` default applied) showed only `[ws compaction] 19%
    · auto-compact 89%`. The model answered the advisory's boundary
    guidance, and ctrl+o showed the full advisory.
- Review (lite): 1 Important, fixed in c1841910a (the notice drain split had
  no test). Two Minor findings were fixed too: recap and line failures sat
  under the refused-row catch, and peer handles could break the one-liner.

#### Open notes

- With the switch on, a dropped resume or reconnect one-liner leaves its
  hidden row to ride the person's next prompt. The decision's drop rule
  allows this, but no transcript notice records the lost resume.
- The hidden row binds to whichever turn starts next. A prompt from the
  person, or from another plugin, that starts first carries it. The engine
  surface cannot close this.
- No test pins a mail unit's weight against `BATCH_CALLS`.

## Live Probe Findings (2026-10-30)

Implementation landed on `impl/develop/dole-ebook-feast` (14e6a0d22), with
engine tests passing. The live probe then contradicted a premise of the
Decisions. The phase has no `### Result` yet.

Setup: Claude Code 2.1.314 on Linux, an isolated tmux socket, and a scratch
copy of `agents-plugin` loaded via `--plugin-dir` with the caption switch on.
The copy added one debug line that logged every `UserMessage` render the mod
saw (origin, from, task, text head).

- **Plugin-origin rows never reach the mod's own `ui.render` hook.** For the
  idle mailbox batch row (`$.prompt.submit`), the debug log shows `hooks
  module ws@inline ui.render skipped: re-entry (the plugin's own code raised
  it; origin ws)` on its first draw and on every periodic redraw. In the same
  redraw passes, the composer rows reached the hook (`origin={"kind":"composer"}`).
  The engine draws the row as `› Prompt from the ws plugin` / `❯ The ws plugin
  sent a message:` plus the raw text, with framing that nothing hooks.
- **A resumed transcript is skipped the same way** (`claude --continue`). The
  stored ws-origin row is skipped as re-entry too, so it is not delivered as
  `unclassified`.
- **An appended mid-turn mail row draws no row at all.** The log shows the
  watcher's `appending 1 queued prompt(s) to the running turn as one row`.
  After that, no `UserMessage` render fires for the row, and the row is absent
  from both the collapsed view and ctrl+o. Per the phase text, the append half
  is a no-op, not a defect.
- The compaction advisory and reconnect prompts use the same
  `$.prompt.submit` path, so the same re-entry skip covers them. The probe did
  not drive them separately.

As a result, the plugin-origin qualification path (the main path) cannot
fire in the live engine. Only the `unclassified` label-prefix fallback could
fire, and only for rows stored before origin stamps. The engine typings do
not document this re-entry rule.

## Live Probe 2: Split-Delivery Feasibility (2026-10-30)

Same setup (Claude Code 2.1.314, Linux, isolated tmux socket, scratch plugin
copy with a file-driven probe clock); transcript order read from the
session's `.jsonl`.

- **Idle append, then submit, works.** The append stored `{type: 'user',
  isMeta: true}`; the submit carried origin `{kind: 'plugin', name: 'ws'}`.
  The model quoted the hidden nonce and placed it before the plugin prompt.
- **Submit, then append, is too late.** The append was accepted and stored
  after the prompt row, but the model missed it for that reply and saw it
  only on the next turn.
- **Hidden user rows draw nowhere**, neither collapsed nor in ctrl+o, idle
  or mid-turn; only the transcript file holds them.
- **A `type: 'system'` append** is stored as `name: 'informational'`, draws in
  ctrl+o only, and is never read by the model (checked across later turns).
- **Mid-turn hidden append** is read within the running turn (stored between
  a `tool_use` and its `tool_result`, quoted in the same turn's answer).
- **The mod's own `prompt.submit` and `session.append` hooks are
  re-entry-skipped** for its own calls, so attaching `context` from them is
  not available.
- **Every plugin submit is framed by the engine**: `› Prompt from the ws
  plugin`, `❯ The ws plugin sent a message:`, the text, and a line saying the
  prompt starts the turn in the user's place. The model reads the framing as
  part of the row.
- `$.ui.log` without `to: 'debug'` draws a `● ws: ...` transcript line (the
  mailbox's existing "armed" line); not yet probed as a recap carrier.

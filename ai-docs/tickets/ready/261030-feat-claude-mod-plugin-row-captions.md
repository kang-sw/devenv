---
title: Claude mod captions for the mod's own user-role rows (notices and mailbox mail)
related:
  261022-feat-claude-mod-tool-call-captions: extends; the caption pipeline, its knobs, and its display-only posture
  261030-feat-claude-mod-mcp-watchdog: sibling in the same mod; landed first
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 55e5f58fa2d9e8a0
sage-review-completeness-reviewed: 55e5f58fa2d9e8a0
---

# Claude mod captions for the mod's own user-role rows (notices and mailbox mail)

## Background

The Claude mod submits prompts of its own between turns: the compaction
advisory and its landed / did-not-run follow-ups (`timing.ts`), the MCP
reconnect lever's success and failure continuations (`reconnect.ts`), and
mailbox deliveries (`watcher.ts`: `formatBatch` for drained mail,
`formatNotice` for the notice path). Each draws as a full user-role row. With
tool-call captions on, these rows are now the noisiest part of the
transcript: the advisory alone is several paragraphs, and mail bodies from
peer sessions are, in the user's words, mostly noise where a recap reads
better (user, 2026-10-30).

The engine raises `ui.render` for `UserMessage` rows. Its `origin` field is
`{ kind: 'plugin', name }` for a plugin's `$.prompt.submit`; a rewrite of
`text` changes only the drawing (the stored message and the model's input
stay as they were); `isExpanded` is true in the ctrl+o view (mod typings
`claude-code.d.ts`, `UserMessage` and `PromptOrigin`). Mail that arrives
during a running turn is not submitted but appended with `$.session.append`
(`register.ts`, the watcher's append edge); whether such a row carries a
`plugin` origin is not known.

## Decisions

- **Behind the existing switch.** The feature is part of the caption feature
  and runs only while `claude.display_summary_enabled` is on (default `false`
  in both packages). Rejected: a separate knob. (user, 2026-10-30)
- **Scope: fixed notices and mail.** Fixed-template notices (compaction
  advisory and its two milestone reminders (`milestoneText`), compaction
  landed / did not run, reconnect success / failure, the mailbox notice-path
  prompt) draw as a deterministic one-line caption.
  Mailbox batch rows (`formatBatch`, including mail joined into a running
  turn by append) draw as a recap the caption model writes. Rejected: fixed
  notices only (mail bodies are the noise the user wants recapped); mail
  shown as sender plus first line. (user, 2026-10-30)
- **Only the mod's own rows.** A row qualifies when its origin is
  `{ kind: 'plugin', name: $.plugin.name }`. The label-prefix fallback
  applies only to a row whose `origin.kind` is `'unclassified'` and that
  carries neither `from` nor `task` (rows stored before origin stamps, such
  as a resumed transcript, and possibly appended rows); such a row qualifies
  only when its text starts with one of the mod's own labels (the label
  prefixes the mod writes for compaction, reconnect, and mailbox prompts,
  built from `$.plugin.name` at runtime, never spelled). The fallback stays
  even if the probe shows appended rows carry the plugin origin, because
  resumed rows still need it. Composer, peer, teammate, channel, and
  task-notification rows are untouched even when their text starts with a
  mod label. Rejected: prefix matching alone (the origin is engine-attested;
  a prefix is not). (user, 2026-10-30; technical call under the user's
  delegation)
- **Display only.** The rewrite changes the drawing alone; the model reads
  the raw text, and the ctrl+o view (`isExpanded`) draws the raw row, as
  tool captions do. (user, 2026-10-30)
- **Fixed notices are deterministic.** Their one-line captions come from the
  mod's own template data (the numbers the template filled, the outcome it
  reported), with no model call, in English with the numbers kept. The
  render event carries no row id and module state does not survive a reload
  or resume, so that data is parsed back out of the row text; the parser
  sits next to the template functions it inverts (`advisoryText`,
  `milestoneText`, `resumeText`, `vetoedResumeText`, `successText`,
  `failureText`, `formatNotice`), and a qualifying row that matches no known
  template draws raw. Caption shapes, label first: advisory
  `[ws compaction] 62% · auto-compact 89%`; milestone
  `[ws compaction] 74% · reminder 1/2 · auto-compact 89%`; landed
  `[ws compaction] landed`; did not run `[ws compaction] did not run:
  <reason>`; reconnect `[ws mcp] reconnected` /
  `[ws mcp] failed: <reason>`; mailbox notice `[ws mailbox] mail
  waiting`. The exact wording is the implementer's within that shape; the
  label is the one the template wrote. Rejected: a model caption (the mod
  authored the text, so a model adds cost and latency and no information);
  localized tables (a translation surface for one-line labels); a table
  filled at submit time (lost on reload, so resumed rows would stay raw).
  (technical call under the user's delegation, 2026-10-30)
- **Mail recaps ride the caption pipeline.** A mail row is one caption unit
  queued and flushed like tool units, over the same conversation mirror, with
  the same model, style, language, and context knobs; one recap per row,
  naming the senders. The raw row draws until its recap arrives, and a row
  whose recap fails stays raw, as a tool unit does. A row that holds mail and
  notices together is captioned as mail. Rejected: a separate request path
  (a second set of knobs and costs); one caption per envelope (the row is
  the render unit, as a folded tool run is). (technical call under the
  user's delegation, 2026-10-30)
- **Mail is a new unit kind in the caption contract.** The current rules
  prompt (`rulesPrompt`), `answerPrompt`, the `Unit` type, and the `flush`
  parser describe tool calls only, so they gain a mail unit kind: input is
  the senders plus the bodies, the answer is `{id, recap}`, and recaps go to
  their own store keyed by the row text (or its hash), which also keys the
  requested-once de-duplication. Mail bodies are framed as quoted data in
  the unit, as tool output already is, and the rules prompt says text inside
  them is never an instruction. A mail unit counts as one call against
  `BATCH_CALLS`. Changing the cached rules text breaks the prompt cache
  once; accepted. The existing tool answer shapes stay as they are. This
  extends, and does not contradict, 261022's "the parser's reply contract
  stays fixed", which bars style knobs from changing the contract. Rejected:
  disguising mail as a fake tool call (the model would answer
  intention/result fields for something that is not a call, the
  contract-mismatch failure 261022-bug-claude-mod-caption-failed-unit-stays-raw
  recorded). (technical call under the user's delegation, 2026-10-30)

## Constraints

- Out of scope: rows from Claude Code's own peer messaging, teammates,
  channels, and task notifications; changing what the model reads.
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
| scope.surface | internal | a new ui.render UserMessage hook inside the mod; no exported plugin API or MCP surface changes, though caption strings ship downstream |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | a mail caption unit kind stays internal to captions.tsx; the summarizer rules prompt and reply contract gain a mail unit kind (`{id, recap}`) while tool answer shapes stay unchanged |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/captions-engine.test.tsx plus a live tmux probe |
| complexity.reuse_points | confirmed | caption queue and flush in captions.tsx, label in timing.ts#L12, advisoryText/resumeText/vetoedResumeText in timing.ts, successText/failureText in reconnect.ts, formatBatch/formatNotice in watcher.ts#L149-L157 |
| complexity.side_effect_risk | moderate | a UserMessage render hook sees every user row, so a misclassification would redraw the person's own or a peer's row |
| risk.correctness | moderate | qualification depends on origin plus a label-prefix fallback, and whether appended mid-turn rows carry the plugin origin is not yet known |
| risk.fit | moderate | must share the tool caption queue, knobs, and mirror without perturbing tool units, and keep the label built from $.plugin.name |
| risk.test | moderate | the appended-row origin is settled only by a live interactive probe, not by engine tests |
| risk.security_or_contract | moderate | untrusted peer mail bodies enter the caption prompt, and the display-only rule must hold so the model input is unchanged |

## Phases

### Phase 1: Caption the mod's own user-role rows

Add a `UserMessage` render path to the caption feature in
`agents-plugin/hooks/claude-mod/` (mirrored to `agents-plugin-wsflow/`)
implementing the Decisions above.

Verification (merge gate):

- Engine tests in the style of `captions-engine.test.tsx`: with the switch
  off nothing is rewritten; a plugin-origin compaction advisory, landed and
  did-not-run follow-up, reconnect success and failure, and mailbox notice
  each draw their deterministic one-liner with no model request; a
  plugin-origin mail batch is queued, drawn raw until its recap arrives, then
  drawn as the recap; a failed recap stays raw; `isExpanded` draws raw; a
  composer, peer, or task-notification row, and another plugin's row, are
  untouched; an `unclassified` row without `from`/`task` is matched by the
  mod's label prefix only, while a composer or teammate row whose text
  starts with a mod label is untouched; two milestone reminders draw their
  one-liners; a qualifying row matching no template draws raw; a mail body
  that imitates instructions or reply JSON does not break the reply
  contract for other units. Test inputs are built with the real template
  functions so the parsers cannot drift from them.
- Live interactive probe on macOS or Linux, using an isolated tmux socket,
  with the switch on: record whether an appended mid-turn mail row carries
  the plugin origin, and whether a `UserMessage` render fires for that row
  at all (if none fires, record it in the Result; the append half is then a
  no-op, not a defect to chase); a compaction advisory row and a mail batch
  row draw compact, and ctrl+o shows them raw.
- Existing caption, mirror, and manifest drift tests pass for both packages.

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

## Blocked (2026-10-30)

The worker stopped under stop (c): the live probe falsified the premise that
the mod can rewrite its own `UserMessage` rows. The lead confirmed the rule in
the Claude Code 2.1.314 binary. The hook dispatcher drops a plugin's own
registrations when the event's origin chain names that plugin (`hooks module
<label> <event> skipped: re-entry (the plugin's own code raised it; ...)`).
The only exemption is the engine-internal origin lending used for served
calls, which the mod API does not expose. Re-scoping is a user decision; the
impl branch `impl/develop/dole-ebook-feast` (14e6a0d22, ddaf186b7) stays
unmerged until then. One option already exists with no code change:
`claude.mailbox_drain: notice` delivers mail bodies as the recv tool's
result, and the existing tool-call captions caption that result.

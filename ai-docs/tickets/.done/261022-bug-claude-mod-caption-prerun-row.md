---
title: Claude mod captions - a call drawn before it runs is captioned from empty data
related:
  261022-feat-claude-mod-tool-call-captions: the feature this bug is in
  261022-bug-claude-mod-caption-failed-unit-stays-raw: the remaining, unconfirmed caption issues split from this one
sage-review-design: required
sage-review-completeness: required
completed: 2026-10-22
---

# Claude mod captions - a call drawn before it runs is captioned from empty data

## Background

Dogfood: with `claude.display_summary_enabled` on, Write and Edit rows are
intermittently captioned "no output, input also empty, target unknown" with
`0.0 KB`, and the caption never changes.

Confirmed with a read-only probe mod and `claude --debug-file` (2026-10-22):
a `ToolUse` row first renders with `isRunning=false`, `input={}` and no
`output` before the call has run, while its input streams or its permission
check waits. Edit `toolu_01JZ3Eg5HvqZ7GeLbSZYT6yb` rendered so at
12:45:04.389, ran at 07.290 and ended at 07.292; Edit
`toolu_01P5jFqBV4XfTf9qkBrncNiH` the same (04.579 -> 07.550).

`toolUseUnit()` (`agents-plugin/hooks/claude-mod/captions.tsx:244`) takes
`!isRunning` as finished, so a flush tick (1.5 s) inside that window queues the
call, sends `{}` and `(not available)`, and stores the caption once
(`requested` keeps it from ever being asked again). Write and Edit have the
longest pre-run window (long streamed input, permission prompt), hence the
flaky Write/Edit-only pattern.

The mod API states the signal to use instead: `ToolUse.output` and
`ToolGroupCall.output` are "the stored result once the call has resolved ...
undefined while it runs", and for an errored, refused or interrupted call it is
the text the model read.

## Decisions

- A call is finished only when it is not running and its `output` is defined.
  Rejected: waiting for the `captured` entry - a call captured before a reload
  is legitimately absent from that map, while the mod API defines `output` as
  set exactly once the call resolved (errors and aborts included).
- A not-finished row draws raw and is not queued, so it is captioned once it
  finishes rather than never.
- `groupUnit` applies the same rule per call: a collapsed, inactive group
  holding an unresolved call draws raw and is not queued. Rejected: trusting
  `isActive` alone - `ToolGroupCall` carries the same `output`, and the same
  pre-run render can reach a group.
- The same change adds debug-log instrumentation for the unconfirmed raw-row
  issue (user directive, 2026-10-22): one debug line per queued unit key, per
  reply item skipped (unknown id or missing fields, with the unit key), per
  unit absent from a reply, and the unanswered reason with the request's
  elapsed time. Debug log only (`to: 'debug'`, as the mod's existing lines);
  nothing drawn. Rejected: a visible failure marker - the fix for raw rows is
  undecided until the log names the path.
- This ticket carries only the confirmed cause and that instrumentation; the unconfirmed raw-row and
  Edit-click issues stay in `261022-bug-claude-mod-caption-failed-unit-stays-raw`.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (Claude Mod Mirror) before editing; the
  wsflow copy of `hooks/claude-mod/` is byte-identical and resynced with
  `rsync -a --delete`.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/captions.tsx, captions-engine.test.tsx, and the byte-identical agents-plugin-wsflow/hooks/claude-mod/ mirror |
| scope.surface | internal | finished-row predicate inside the mod; no config key, prompt text, or reply contract changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing-files | agents-plugin/hooks/claude-mod/captions-engine.test.tsx |
| complexity.reuse_points | confirmed | ToolUse.output / ToolGroupCall.output in the engine's mod API types |
| complexity.side_effect_risk | low | changes only when a row is queued for a caption |
| risk.correctness | low | one predicate in two unit builders; an engine that never sets output would leave rows raw, which is the safe failure |
| risk.fit | low | follows the existing unit-builder shape |
| risk.test | low | engine tests already drive ToolUse and ToolGroup renders |
| risk.security_or_contract | low | no data-egress or contract change |

## Phases

### Phase 1: Caption a call only once it has resolved

In `toolUseUnit` and `groupUnit`, require every call to have a defined
`output` (and not be running) before building a unit. Update the engine test
fixture so finished rows carry an `output`, and add tests: a pre-run row
(`isRunning=false`, no `output`) draws raw and sends nothing, then is captioned
once it carries an `output`; the same for a group holding a pre-run call.
Add the debug-log instrumentation from `## Decisions`, with an engine test that
a reply missing a unit and an unanswered request each log a line naming the
unit key. Resync the wsflow mirror.

Edit points (read 2026-10-22): `toolUseUnit` (`captions.tsx:235-247`, add
`output` to its props type, return undefined while `output === undefined`);
`groupUnit` (`:214-232`, any call `isRunning` or without `output` returns
undefined); `request` (`:573`, log the queued key); `ask` (`:459-462`, add
elapsed ms to `request not answered`); `flush` (`:494-526`, log the unit keys
when the reply is undefined, each item skipped for an unknown id or missing
fields, and each labelled unit left without a caption). `log` (`:395`)
prefixes `ws captions: `. Test fixture: `BASE` in `captions-engine.test.tsx:26`
gains `output`, so existing finished rows stay finished; `toolUse()` at `:109`.

Verification: `claude plugin validate` and `claude plugin test` on both
packages; `go test ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate`;
then a live dogfood pass with many Write/Edit calls, confirming no empty
caption appears.

### Result (b024aabe1) - 2026-10-22

Implemented directly by the lead (user directive: no delegation, no `ready/`
promotion). `captions.tsx` gains `isResolved` (`!isRunning && output !==
undefined`); `toolUseUnit` and `groupUnit` build a unit only from resolved
calls. Debug lines (`ws captions: ` prefix): `queued <key>`, `request not
answered: <reason> after <ms> ms`, `left raw, no answer: <keys>`, `reply item
skipped, unknown id <id>`, `reply item skipped, missing fields: <key>`, `left
raw, absent from the reply: <key>`. The elapsed time needed a `now` edge on
`CaptionEdges`, wired to `$.clock.now()` in `register.ts`. Tests: `BASE` carries
`output`; three new engine tests (pre-run ToolUse, group with a pre-run call,
reply-item debug lines) and the unanswered test asserts its debug lines.
`claude plugin validate` / `test` pass on both packages (337 pass each);
`TestWsflowHooksMirrorUpToDate` passes. The live dogfood pass is pending: the
installed plugin copy is not updated by this commit.

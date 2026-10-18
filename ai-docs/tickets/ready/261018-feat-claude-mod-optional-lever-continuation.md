---
title: Make the Claude mod levers' continuation optional
blocked-by: 261018-feat-claude-mod-mid-turn-steer-delivery
related:
  261018-feat-claude-mod-mid-turn-steer-delivery: prerequisite - introduces the held continuation window in compaction.ts that an omitted continuation must skip
  261017-feat-claude-mod-lead-compaction-timing: introduced the compaction lever and its required continuation
  261018-feat-claude-mod-mcp-reconnect-lever: introduced the reconnect lever and its required continuation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 49eac49c0340ffa9
sage-review-completeness-reviewed: 49eac49c0340ffa9
---

# Make the Claude mod levers' continuation optional

## Background

The ws Claude mod (`agents-plugin/hooks/claude-mod/`) registers two levers,
`request_compaction` and `request_mcp_reconnect`. Both declare `continuation`
as required (`register.ts` inputSchema `required: ['continuation']`), and
both input checks reject an empty one (`checkLeverCall` in `timing.ts`,
`checkReconnectCall` in `reconnect.ts`). After the lever's step the mod always
submits the continuation as the next prompt.

The continuation was meant only for the case where work must resume by
itself after the step (user, 2026-10-18). Because it is required, agents
always invent one, even when the next turn will come from elsewhere: dogfood
2026-10-18 compacted a lead that was only waiting on a background worker, and
the continuation turn did nothing but revive and report "still running"; the
worker's completion notification would have started the next turn anyway.
The compaction advisory names "you are waiting only on background agents" as
a typical good moment to compact, which is exactly where a continuation is
usually unneeded.

## Decisions

- `continuation` becomes optional on both levers: it is removed from each
  inputSchema's `required`; an absent, non-string, or whitespace-only value
  counts as omitted; a supplied continuation keeps the one-line rule
  (a multi-line value is still a tool error). This supersedes the required
  continuation of 261017-feat-claude-mod-lead-compaction-timing and
  261018-feat-claude-mod-mcp-reconnect-lever, and the former's "the
  continuation is always submitted": the continuation exists only for work
  that must resume by itself after the step; a caller with such work still
  passes it, so the reconnect lever's reason for rejecting a no-argument
  tool still holds. A repeat call while a request is pending replaces its
  continuation, latest wins, and an omitted continuation on the latest call
  clears an earlier one (the prior "latest wins" rule). (user, 2026-10-18)
- Compaction lever with the continuation omitted: after its own compaction
  lands nothing is submitted and no continuation is held, so no consume
  window opens and a prompt typed during the compaction passes through the
  mod's `prompt.submit` hook untouched; when a hook vetoes the compaction
  with `{ skip }`, no turn is started and the skip is reported only through
  a user-visible `$.ui.log` notice (not a debug-only log: with no turn, the
  user is the only one who can learn the compaction did not run); when
  another compaction landed first, nothing is submitted.
  Rejected: submitting a bare "compaction done" notice - it starts the same
  idle turn this ticket removes. (user, 2026-10-18)
- Reconnect lever with the continuation omitted: a successful reconnect
  submits nothing; a failed reconnect still submits its failure notice,
  without the `Continue:` line, because the model must learn its tools are
  gone. (user, 2026-10-18)
- Text: the lever descriptions (`LEVER_DESCRIPTION`,
  `reconnectDescription`), the lever results (`LEVER_RESULT`,
  `RECONNECT_RESULT`), and the compaction advisory and milestone texts say to
  pass a continuation only when work must resume by itself after the step,
  and to omit it when the next turn will come from the user or from a
  background agent's notification. A lever result for an omitted
  continuation does not promise that a continuation resumes the work; the
  same holds for the transcript notices in `compaction.ts` that promise a
  continuation ("its continuation follows at the next turn end", "the
  continuation follows with a note"), which get an omitted-continuation
  wording. Descriptive texts that call the continuation always sent
  (`agents-plugin/hooks/hooks.json` `description`, the `register.ts` header
  comment, the `compaction.ts` and `reconnect.ts` module comments) are
  updated on contact; `skills/mcp-server-repair/SKILL.md` stays as is, since
  its caller still has an interrupted step to pass.
  (user, 2026-10-18)

## Constraints

- Build on `develop` after 261018-feat-claude-mod-mid-turn-steer-delivery
  merges; that feat adds the held-continuation window (`hold`, `claim`,
  `CONTINUATION_WINDOW_MS`) in `compaction.ts` that the omitted path must
  bypass.
- A supplied continuation keeps today's behavior on every path, including
  the held window and the vetoed-compaction text.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Scope is the ws mod; the wsflow copy under
  `agents-plugin-wsflow/hooks/claude-mod/` stays byte-identical.
- The lever and advisory texts are model-facing shipped strings.
- Do not touch the uncommitted change to
  `ai-docs/tickets/.done/261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers.md`;
  do not stage `.codex`.

## Prior Decisions

- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "The tool is `request_mcp_reconnect`, taking a required one-line `{ continuation: string }` ... Rejected: a no-argument tool (the lead's interrupted next step would not survive the turn end in its own words)." — bearing: contradiction-candidate
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, Decisions): "The continuation is always submitted. When another compaction landed ..." (the continuation is still submitted) — bearing: contradiction-candidate
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, Decisions): "An empty or multi-line `continuation` is a tool error; a repeat call while a request is pending replaces its continuation (latest wins)" — bearing: constrains
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "A call carrying `agentId` is refused (main session only); an empty or multi-line `continuation` is a tool error; a repeat call while a request is pending replaces its continuation (latest wins)" — bearing: constrains
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "the wsflow `claude-mod/` copy stays byte-identical" (lever registers in both ws and wsflow) — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, Decisions): the lever's own-compaction continuation is held for a 1500 ms window (`hold`/`claim`) so a prompt typed during compaction can take it as context — bearing: constrains
- 261004-feat-pi-explicit-post-compaction-continuation (2026-10-04, Decisions D4): "Caller intent, not delivery guarantees. The flag means 'I have work after compaction' ... Do not promise exactly one subsequent model run" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/{register.ts,timing.ts,compaction.ts,reconnect.ts} plus tests (compaction.test.ts, compaction-engine.test.ts, reconnect.test.ts, reconnect-engine.test.ts, engine.test.ts) and the byte-identical agents-plugin-wsflow/hooks/claude-mod/ copy |
| scope.surface | public-interface | both model-callable lever tools change their inputSchema (register.ts L496, L524 drop required) and their model-facing description/result/advisory prose |
| scope.new_public_symbol | no | none; existing tools request_compaction and request_mcp_reconnect only |
| scope.new_type_contract | yes | continuation becomes optional in both tool input schemas; checkLeverCall (timing.ts#L120-L131) and checkReconnectCall (reconnect.ts#L59-L70) return an omitted-continuation result instead of an error |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/compaction.test.ts, compaction-engine.test.ts, reconnect.test.ts, reconnect-engine.test.ts, engine.test.ts; the two "is required" error assertions (compaction.test.ts#L173, reconnect.test.ts#L101) must change |
| complexity.reuse_points | confirmed | finish/hold/runLever in compaction.ts#L179-L245 and run/successText/failureText in reconnect.ts; read, the omitted path branches inside them |
| complexity.side_effect_risk | moderate | changes whether a post-step prompt is submitted at all and must bypass the held-continuation window and timer in compaction.ts; a wrong branch strands or double-submits a prompt |
| risk.correctness | moderate | several distinct paths (landed, another compaction first, skip veto, rejection retry, reconnect success/failure, repeat call replacing an omitted with a supplied continuation) each need an omitted variant |
| risk.fit | moderate | follows the existing lever shape and the byte-identical mirror, but reverses two earlier recorded decisions (required continuation, always submitted) and rewrites shipped model-facing prose |
| risk.test | moderate | tests pin the prose verbatim and the "is required" errors, so many assertions change; the decisive idle-session check is a live dogfood left to the lead |
| risk.security_or_contract | moderate | model-facing tool schema and pinned prose change; the failure notice for reconnect must still reach the model; subagent-call refusal must be preserved |

## Phases

### Phase 1: Optional continuation on both levers

Make `continuation` optional per the Decisions: schema, input checks, the
compaction finish/hold paths, the reconnect outcome path, and the texts.

Verification:

- `claude plugin validate` and `claude plugin test` pass for both
  `agents-plugin` and `agents-plugin-wsflow`, and the wsflow mirror checks
  stay green.
- Tests cover, for each lever, an omitted and a whitespace-only continuation
  being accepted; a supplied continuation keeping today's submits; the
  compaction lever submitting nothing after its own landed compaction, after
  another compaction landed first, and on a `{ skip }` veto (a log only); a
  prompt typed during an omitted-continuation compaction passing without
  attached context; the reconnect lever submitting nothing on success and the
  failure notice without a `Continue:` line on failure; a repeat call in
  both orders (supplied then omitted clears it, omitted then supplied sets
  it); a failed or rejected compaction with an omitted continuation
  submitting nothing, its failure handling otherwise unchanged; neither
  inputSchema listing `continuation` as required.
- Tests pinning the lever and advisory prose verbatim are updated to the new
  wording; the wording itself is the implementer's, within the Text
  decision.
- Live dogfood (calling the compaction lever with no continuation and seeing
  the session stay idle afterwards) is the lead's after merge.

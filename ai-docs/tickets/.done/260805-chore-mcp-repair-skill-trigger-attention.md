---
title: mcp-server-repair is not self-invoked because its description does not
  match the failure vocabulary agents actually emit
related:
  260726-chore-mcp-repair-pointer-mid-procedure-skills: prior art — the pointer
    sweep that covered the in-skill route this ticket complements
  260728-chore-ws-tree-skill-pointer-guard: sibling — this ticket edits a
    ws-tree SKILL.md, the surface that ticket reports as unguarded
  260728-chore-spec-mcp-server-repair-unspecified: the repair route's total spec
    gap; this ticket's Spec Impact addresses only the trigger-surface slice
  260624-epic-pre-release-cleanup: item 8 is the repair-route coverage line this
    descends from
related-mental-model:
  - workflow-skills
spec:
  - 260806-skill-description-self-invocation-trigger
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-06
---

# mcp-server-repair is not self-invoked because its description does not match the failure vocabulary agents actually emit

## Background

`mcp-server-repair` exists so a session whose ws MCP channel dies keeps making
progress through `ws-cli` instead of dead-ending. `260726-chore-mcp-repair-pointer-mid-procedure-skills`
swept an `If this call fails to connect, run /ws:mcp-server-repair.` pointer into
every SKILL.md in both trees that calls `playbook.print`, which covers the case
where the failure happens *inside* a ws skill.

It does not cover the case where a session simply finds the tools gone. Reported
behavior in that case: agents notice the failure and say so — some form of "the
ws MCP server will not run" is the near-universal phrasing — but do not connect
that observation to the skill. Awareness is therefore confirmed and is not the
defect; the defect is that the observation never reaches the skill.

The current description is the trigger surface for that path, and it is written
in authoring vocabulary rather than the vocabulary agents produce:

```
Recover when the ws/* MCP tools are absent from the tool list, or a ws/* tool
call fails to connect. Keep working through ws-cli and relay the reconnect steps
to the user.
```

`fails to connect` matches. `absent from the tool list` is not how a model
phrases the condition. The three most common phrasings — *server is not
running*, *failed to start*, *unavailable* — appear nowhere in it.

## Decisions

- **Rewrite the description in the model's own failure vocabulary.** Settled
  text, to be used verbatim:

  ```
  The ws MCP server is not running, failed to start, is disconnected, or its
  ws/* tools are missing from the tool list. When you are about to report that
  ws MCP is unavailable, invoke this instead.
  ```

  First sentence enumerates the observable states as a state declaration, not an
  authoring-side condition clause. Second sentence names the exact moment of
  maximum leverage — the agent composing its "ws MCP is unavailable" report —
  and substitutes the action. Phrased as `X instead` rather than a prohibition,
  per the skill-authoring rule preferring a positive action over `Do not do X`.

- **Extend the wsflow substitution table with a `ws MCP` literal token.**
  `agents-plugin-tool/internal/wsrsrc/skills_mirror.go` substitutes exactly
  three tokens (`\bws:`, `\bws/`, `\bws-cli\b`). The new description is the first
  mirrored text to contain space-separated `ws MCP`, which no rule rewrites, so
  the generated wsflow mirror would read "The **ws** MCP server ..." — text that
  describes wsflow as ws, which `ai-docs/ref/wsflow-mirroring.md` forbids in
  distributed wsflow material. Add:

  ```go
  wsMCPPattern = regexp.MustCompile(`\bws MCP\b`)  // -> "wsflow MCP"
  ```

  in the same literal-token class as the existing `wsCliPattern`, carrying a
  comment that it must not be broadened to a bare `\bws\b` rule. The literal
  space keeps it from touching the `ws-mcp` binary name (`ws-mcp-launcher.py`),
  which the wsflow mirror must preserve verbatim. Keep it case-sensitive, like
  the three existing patterns; the settled description spells the token `ws MCP`
  and nothing requires matching a lowercased variant. Application order relative
  to the other three patterns is free — none of the four match overlapping
  input.

- **The eligibility guard needs no change.** `guardSubstitutionEligible` rejects
  a source containing `ws.` among other tokens; the settled description places no
  `ws` immediately before a period, so `mcp-server-repair` stays eligible for
  substitution-mirrored generation. A regeneration failure here would mean the
  wording drifted from the settled text, not that the guard needs relaxing.

- **Zero-regression precondition, verified at authoring time:** `grep -rn "ws
  MCP\|ws mcp"` over `agents-plugin/skills/` and `agents-plugin-wsflow/skills/`
  returns nothing, so the new pattern is a no-op for every existing file and
  acts only on this description.

- **Keep the skill name.** `mcp-server-repair` already breaks the `lead-` prefix
  deliberately and already shares tokens with the failure phrasing. A rename
  would touch 20+ pointer sentences across both trees, the hardcoded regexes in
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`, the curated list in
  `internal/wsrsrc/skills_mirror_test.go`, `agents-plugin/skills/manifest.json`
  hashes, `ai-docs/ref/wsflow-mirroring.md`, and
  `ai-docs/mental-model/workflow-skills.md` — large blast radius for no
  identified gain.

- **Scope is the trigger surface only.** No change to the skill's handlers,
  invariants, doctrine, or reconnect template; no change to the pointer
  sentences the sweep ticket already placed.

### Rejected alternatives

- **A workflow-manual line that pre-installs the frame.** Considered stronger
  than a description because it would sit in context before the failure rather
  than compete for attention at selection time. Rejected: the manual is loaded
  through `ws/playbook.print`, so it fails closed with the very channel it would
  rescue, and the dominant reported case is a server that never came up — where
  the manual was never loaded at all.

- **A `[skill for agent-use]` prefix on the description.** Rejected: every skill
  is agent-use, so it carries no discriminating signal, spends description
  budget, and renders into user-facing surfaces as noise. The intent behind it —
  marking the skill as self-invoked rather than user-typed — is carried by the
  second sentence's second-person imperative instead.

- **Keeping a `it makes no MCP call` clause in the description.** Rejected as a
  selection-surface restatement of body content. It would only earn its place if
  agents were observed skipping the skill on the theory that a ws skill cannot
  work while ws MCP is down; that failure mode has not been observed, and the
  skill-authoring rule is to add rules only for observed wrong executions.
  Revisit if dogfooding produces it.

- **Rewording the description to stay inside the existing substitution table**
  (using only `ws/*` forms, never bare `ws MCP`). Rejected: it would restore the
  authoring-side vocabulary the ticket exists to remove.

- **Decorating the launcher's stderr with the repair pointer.** Considered and
  deferred rather than rejected on merit: it does not help the primary case,
  because a dropped or never-started MCP channel produces no launcher output for
  a session to read. It applies only to the launcher-cannot-install-a-runtime
  class, which this ticket does not cover.

## Prior Art

- `adbf5ec3` — established the pointer wording and the repair skill.
- `260726-chore-mcp-repair-pointer-mid-procedure-skills` — the pointer sweep;
  this ticket covers the path that sweep structurally cannot reach.
- `wsCliPattern` in `skills_mirror.go` — the existing precedent for a
  literal-token substitution that is explicitly not a namespace-prefix rule.

## Spec Impact

- Target spec area: `ai-docs/spec/workflow-skills.md`, anchor
  `{#260508-skill-description-attention-policy}`.
- Why an existing stem does not already address it: that anchor describes
  descriptions as the trigger surface for **user-request** matching, and
  distinguishes strong entry triggers from lighter derived-stage triggers. It
  does not describe a description whose trigger is **observed environment
  state** and whose invocation carries no user request at all.
- Expected caller-visible change: the policy gains a self-invocation trigger
  class — a skill whose description is matched against the session's own
  observation of a broken tool surface, written in the vocabulary the agent
  emits when reporting that failure. No MCP tool contract, routing semantics, or
  skill roster changes.
- Contract-first spec: no. The behavior is a prompt-surface change; the spec
  records the policy after the wording is settled.

## Phases

### Phase 1: Retune the trigger surface and make the wsflow mirror namespace-correct

Replace the `description:` line in
`agents-plugin/skills/mcp-server-repair/SKILL.md` with the settled text above,
leaving the rest of the file untouched. Add `wsMCPPattern` to
`agents-plugin-tool/internal/wsrsrc/skills_mirror.go` alongside the existing
three patterns and apply it in `GenerateWsflowSkillBody`.

The generator change and the description change land together: the description
is the only text that needs the new pattern, and shipping it without the pattern
puts ws-branded text into the distributed wsflow package.

Regenerate both affected generated surfaces — the wsflow skills mirror and the
ws-tree skills manifest — rather than hand-editing either.

Give the new pattern a unit test alongside `TestWsCliSubstitutionPattern` in
`agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go`, following that test's
shape: assert `ws MCP` becomes `wsflow MCP`, and assert the negative cases the
in-code comment warns about — `ws-mcp`, `ws-mcp-launcher.py`, and a bare `ws`
followed by a non-`MCP` word all survive untouched. Without it the
non-broadening claim rests on a manual read of the generated mirror, and a later
widening to `\bws\b` would pass every existing gate.

**Completion criterion.** This phase ships wording; it does not demonstrate that
the wording fires. Selection behavior is not observable in CI, so no verification
here can close the causal loop the Background opens. Phase 1 is done when the
generated surfaces are correct. Whether the retuned description actually gets the
skill self-invoked is answered only by dogfooding — if a session again reports ws
MCP unavailable without reaching for the skill, that observation reopens the
question rather than this ticket having failed silently.

**Verification**

- `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
  (from `agents-plugin-tool/`; `-count=1` is mandatory, the cache silently
  no-ops otherwise), then `TestWsflowSkillsMirrorUpToDate` clean.
- `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -count=1`
  to refresh `agents-plugin/skills/manifest.json`.
- `go test ./internal/wsrsrc -count=1 -run 'SubstitutionPattern'` — the new
  `wsMCPPattern` test plus the existing `TestWsCliSubstitutionPattern`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — the bundle guard
  asserts shim shape and pointer tails. Note it forbids only `\bws/`, `\bws:`,
  `\bws\.`, `subquery`, `agents\.`, so it would pass a mirrored "The ws MCP
  server ..." — this guard does not substitute for the generator change.
- Confirm the generated
  `agents-plugin-wsflow/skills/mcp-server-repair/SKILL.md` description contains
  no bare `ws ` token and reads `wsflow MCP` / `wsflow/*` throughout, while
  `ws-mcp-launcher.py` in the body is still spelled with `ws-mcp`.

### Result (ff979d6c) - 2026-08-06

Landed as planned; no deviation from the phase text.

`wsMCPPattern` was added to the `var` block next to `wsCliPattern` and applied
last in `GenerateWsflowSkillBody`. Application order proved irrelevant as
predicted — none of the four patterns match overlapping input. The shared
comment above the `var` block now covers both literal-token rules and states the
non-broadening contract for each.

`TestWsMCPSubstitutionPattern` asserts the positive rewrite plus three negatives
in one fixture: `ws-mcp-launcher.py`, a standalone `ws-mcp`, and a bare `ws`
followed by a non-`MCP` word all survive untouched.

The eligibility guard needed no change, as predicted — `mcp-server-repair`
stayed eligible and regenerated without a guard error.

Verification, all clean:

- `go test ./internal/wsrsrc -count=1 -run 'SubstitutionPattern'` — both
  literal-token tests pass.
- Mirror regen, then manifest regen, then the full `./internal/wsrsrc/...` suite
  including `TestWsflowSkillsMirrorUpToDate`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — 10 tests OK.
- `go build ./...` clean.
- Generated wsflow description reads `The wsflow MCP server is not running, ...
  its wsflow/* tools are missing ... report that wsflow MCP is unavailable`;
  `grep '\bws '` over the generated file returns nothing, and
  `ws-mcp-launcher.py` on line 26 is unchanged.

> Forward: the completion criterion above stands — this shipped wording, not
> evidence the trigger fires. The next dogfooding session that reports ws MCP
> unavailable without reaching for the skill is the signal that the vocabulary
> lever was insufficient and the problem is selection pressure rather than
> phrasing.


## Resolution (2026-08-06)

Phase 1 landed in `ff979d6c`; the spec class it implies landed in `586e582c` as
`260806-skill-description-self-invocation-trigger`.

Closing on the completion criterion the phase states: the wording and the
namespace-correct mirror shipped and are gated by tests. This ticket does not
claim the skill is now reliably self-invoked — that is not observable in CI. The
next dogfooding session that reports ws MCP unavailable without reaching for the
skill reopens the question as a new ticket, not as a reopening of this one.

Scope stayed on the trigger surface. A follow-on attempt to also handle the mode
where the launcher cannot produce a compatible runtime was started and then
withdrawn: it was built on the untested premise that the agent would need to
detect that mode, when in fact `ws-cli` prints the launcher's reason to stderr
and exits immediately, so the mode announces itself. Nothing from that attempt
survives here. Any real defect in the repair route's behavior after the skill
fires is unaddressed and unticketed.

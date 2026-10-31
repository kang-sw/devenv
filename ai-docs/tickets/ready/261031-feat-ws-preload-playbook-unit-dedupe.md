---
title: ws-preload - dedupe inlined playbooks by playbook unit (Claude mod + Pi)
related:
  261030-feat-claude-mod-ws-preload-skill-expansion: extends (landed 449e731ec)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 21223f5fd81143b5
sage-review-completeness-reviewed: 21223f5fd81143b5
---

# ws-preload - dedupe inlined playbooks by playbook unit (Claude mod + Pi)

## Background

The ws-preload feature (`261030-feat-claude-mod-ws-preload-skill-expansion`,
merged 449e731ec) replaces a lead skill shim's `<ws-preload
playbook="<name>">` block with that playbook's body when the skill is
expanded for the model (`agents-plugin/hooks/claude-mod/preload.ts`,
`register.ts` `skill.prompt` hook; Pi: `expandWsSkillPreload` in
`agents-plugin-pi/src/lead-skills.ts`).

Live check 2026-10-31 (Claude Code, Orca headless session): invoking
`ws:lead-tune` twice inlined the full playbook body both times; Claude Code
only prefixes the second expansion with a "Re-invocation of /ws:lead-tune"
line and does not drop the body itself. Each repeat invocation of a lead
skill therefore re-spends the whole playbook in context.

The Claude mod already collapses a repeat main-thread `playbook_read` result
to a pointer, judged by the conversation itself: `manual.ts` `dedupe` /
`isRepeat` key a read by `name` + `context` (`readKey`) and return the
pointer only while the earlier identical result is still visible in
`edges.messages()`, so compaction, `/clear`, and rewind restore the full
text with no reset logic. That dedupe sees only `tool_result` blocks of
`playbook_read` calls, so it never sees a body the preload inlined, and the
preload path has no dedupe at all.

In the same check the expanded skill showed its title twice: every shim's
`# <title>` heading sits above the `<ws-preload>` block and every playbook
body opens with the same `# <title>` (all ten `<ws-preload>` shims in
`agents-plugin/skills/*/SKILL.md` match their `agents-plugin/rsrc/<name>/<name>.md`
heading).

Pi: the model-invoked `ws-skill` tool already expands the block and dedupes
through `dedupeRead(visibleEntries, toolCallId, "ws-skill", wsSkillKey(p),
text)`, keyed by the tool and skill name. A typed `/skill:<name>` is not
expanded: the 449e731ec worker concluded Pi has no hook, but Pi's `input`
event can rewrite the input (`InputEventResult` `{ action: "transform",
text }`, `pi-coding-agent` `dist/core/extensions/types.d.ts:669`) and fires
before `_expandSkillCommand` (`dist/core/agent-session.js:842-856`), whose
block format is
`<skill name="${skill.name}" location="${skill.filePath}">\nReferences are relative to ${skill.baseDir}.\n\n${body}\n</skill>`,
followed by `\n\n${args}` when args are present
(`dist/core/agent-session.js:984-994`).

## Decisions

- **One dedupe unit: the playbook.** A playbook body the preload inlined and
  a main-thread `playbook_read` result for the same playbook (`name`, plus
  `context` where a read carries one) are one dedupe unit. Whichever path
  brings the body second gets a pointer while the identical body from the
  first is still visible in the conversation: a skill invoked twice, a skill
  invoked after a `playbook_read` of its playbook, and a `playbook_read`
  after the skill's preload all collapse. Rejected: a per-skill "already
  expanded" memory reset at session start and compaction (the lead's first
  proposal in discussion; it duplicates state the conversation already
  holds, and needs its own reset logic for compaction, `/clear`, and
  rewind, which the conversation-visibility check gets for free).
- **Repeat is judged by conversation visibility.** A body is a repeat only
  while the identical body text is still visible in the conversation, now
  counting skill-expansion text in user messages as well as `playbook_read`
  `tool_result` blocks; `isRepeat`'s existing rule that a full body after a
  pointer is returned in full again stays. Compaction, `/clear`, and rewind
  therefore restore the full body with no reset logic.
- **The skill-side pointer stays self-sufficient.** When the preload path
  dedupes, the block is replaced by a pointer line followed by the block's
  original fallback prose (the `playbook.read` instructions), not by the
  pointer alone. The pointer line names the playbook, says its full
  procedure is already above in this conversation, tells a reader who can
  see that copy to follow it and skip the read instructions below, and says
  those instructions are only for a context that cannot see it. Reason:
  `skill.prompt` carries no `agentId` and also fires for a skill preloaded
  into a subagent, so the hook judges by the main conversation; a subagent
  that cannot see the main conversation's copy must still be able to read
  the playbook itself. Rejected: the bare pointer on the skill side (a
  subagent preload would receive a pointer to text it cannot see).
- **Both pointer kinds count in the after-pointer rule.** A skill-side
  pointer counts as a pointer in `isRepeat`'s tally exactly like a tool-side
  pointer: a `playbook_read` made after a skill-side pointer returns the full
  body, because a read after a pointer means the reader could not use the
  earlier copy. The pointer line's "skip the read below" wording is what
  keeps the main-thread model from making that read. Rejected: ignoring
  skill-side pointers in the tally (a reader that genuinely lost the copy
  would be pointed at it again and never recover the body).
- **The tool-side pointer names the earlier copy, not a call.** When the
  earlier visible copy is a skill expansion rather than a `playbook_read`
  result, the tool-side pointer says the full procedure is already above in
  this conversation instead of `pointerText`'s "your earlier
  `playbook_read` call" wording, which would name a call that never
  happened. The existing wording stays for a tool-result copy.
- **Identical text is the match; context does not widen it.** A
  `playbook_read` carrying a `context` collapses against a preload only when
  its rendered body is byte-identical (after the same trimming on both
  sides: the preload trims the body, so the comparison normalises both
  texts the same way); a context that renders a different body is a
  different unit and is returned in full. The same rule holds between two
  `playbook_read` results: reads of the same `name` collapse whenever their
  normalised bodies are identical, whatever their `context`, so `readKey`'s
  context component no longer separates identical bodies.
- **Pi uses the same unit and the same expansion path for typed commands.**
  Pi's dedupe collapses a repeat playbook body across `ws-skill`, a typed
  `/skill:` expansion, and `ws__playbook_read` results (today `dedupeRead`
  compares only within one family, `ws-skill` keyed by skill name over the
  whole result, and `ws__playbook_read` dedupes separately in `bridge.ts`).
  A Pi pointer keeps Pi's existing pointer form with no fallback prose: Pi
  judges by each process's own visible entries, so a fork never receives a
  pointer to text outside its own context. Its provenance must keep the
  existing guarantee that a forged or stale marker never suppresses a read:
  a copy from a typed command (a user message with no tool call id) needs a
  provenance kind of its own, such as the session entry id; if no
  provenance with that guarantee is available for typed-command copies, a
  typed command expands in full without dedupe and the worker reports it.
  A typed `/skill:<name>` for a skill under the adapter's own skills
  directory is expanded by an `input` handler returning `{ action:
  "transform", text }` with the expanded body wrapped in Pi's own `<skill
  name location>` block format above, so Pi's `_expandSkillCommand` then
  passes the text through. The handler acts only on `source` `"interactive"`
  or `"rpc"` (Pi expands `/skill:` only when `expandPromptTemplates` is on,
  and `sendUserMessage` sends `source: "extension"` with it off,
  `agent-session.js:1159-1186`). Any other input, an excluded skill
  (`lead-use-mailbox`), no bridge or key, or any failure returns `{ action:
  "continue" }` and Pi expands the raw shim as today. This supersedes
  f3ed3a77's "no expansion hook, left unexpanded" conclusion: `input` can
  transform, and reproducing the block is a small, test-pinned format.
  Rejected: leaving typed commands unexpanded (the hook exists);
  transforming a typed command into an instruction to call `ws-skill` (an
  extra model round trip, which the preload exists to remove).
- **The shim heading moves inside the block.** Each `<ws-preload>` shim's
  `# <title>` line (ws and wsflow shims) moves inside its block, so an
  expansion replaces it with the playbook's own identical heading and the
  fallback text a non-expanding harness reads is unchanged. Rejected:
  stripping a duplicate heading in expansion code (a text-matching heuristic
  in two adapters; the marker contract stays "the block is replaced whole").

## Constraints

- `<ws-preload>` contract from 449e731ec stays: allowlist by own plugin /
  own skills dir, `lead-use-mailbox` excluded, read with the lead key, no
  session state or key inlined, any failure passes the text through.
- `agents-plugin/hooks/claude-mod/` and `agents-plugin-wsflow/hooks/claude-mod/`
  stay byte-identical (`wsflow-mirroring.md`).
- Pi's block format is copied from an installed Pi internal; a test pins it
  against the installed `pi-coding-agent` output so a Pi upgrade that changes
  it fails loudly.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/skills/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261030-feat-claude-mod-ws-preload-skill-expansion (2026-10-30, Decisions): "Subagent preloads see only the playbook body. `SkillPromptInput` carries no `agentId`, and the type doc says `skill.prompt` also fires for a skill preloaded into a subagent" — bearing: constrains
- 261030-feat-claude-mod-ws-preload-skill-expansion (2026-10-30, Decisions): "Allowlist only. Expansion runs only for skills whose `pluginName` equals the mod's own `$.plugin.name`" — bearing: constrains
- 261030-feat-claude-mod-ws-preload-skill-expansion (2026-10-30, Decisions): "Failure passes through. Any expansion error returns the original text unchanged" — bearing: constrains
- 261030-feat-claude-mod-ws-preload-skill-expansion (2026-10-31, Result b22697bdd): "Observation (lite review, Minor): a repeated expansion re-inlines the full playbook, where a repeated model `playbook_read` used to collapse to a pointer through `manual.read`." — bearing: supports
- f3ed3a77 (2026-10-31, commit): "Pi /skill:<name> has no expansion hook: agent-session.js emits 'input' on the raw text before _expandSkillCommand, so expanding there would mean reimplementing Pi's expansion; left unexpanded" — bearing: constrains
- f3ed3a77 (2026-10-31, commit): "Expansion runs after the read succeeds and before dedupeRead, so a repeat expanded read collapses to a pointer as before" — bearing: supports
- b22697bd (2026-10-31, commit): "The tag keeps its ws- name in wsflow (shared byte-identical mod contract; matches none of the forbidden patterns)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/ (preload.ts, manual.ts, register.ts, tests), agents-plugin-wsflow/hooks/claude-mod/ mirror, agents-plugin/skills/*/SKILL.md and agents-plugin-wsflow/skills/*/SKILL.md, agents-plugin-pi/src/lead-skills.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/playbook-read-dedupe.ts, agents-plugin-pi/src/bridge.ts, agents-plugin-pi/test/ |
| scope.surface | cross-module | spans the claude-mod hook, shipped skill shims, the skills manifest, and the Pi adapter; the block contract is shared by two adapters |
| scope.new_public_symbol | unknown | ticket fixes no signature; a pointer builder or exported helper in preload.ts, manual.ts, or lead-skills.ts is likely but not named |
| scope.new_type_contract | unknown | ticket does not fix types; ApiMessageLike or the preload edge may widen to read user-message text (agents-plugin/hooks/claude-mod/manual.ts#L211) |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/preload.test.ts, manual-engine.test.ts, engine.test.ts, agents-plugin-pi/test/lead-skills.test.ts, agents-plugin-pi/test/playbook-read-dedupe.test.ts, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py |
| complexity.reuse_points | confirmed | isRepeat, readKey, pointerText (agents-plugin/hooks/claude-mod/manual.ts#L130, #L191, #L229) and dedupeRead, wsSkillKey (agents-plugin-pi/src/playbook-read-dedupe.ts#L41, #L134); Pi input event already used at agents-plugin-pi/src/index.ts#L709 |
| complexity.side_effect_risk | moderate | changes every lead skill expansion text, skill shims in both packages, the manifest hashes, and a second Pi input handler that must not disturb the fork-frame handler |
| risk.correctness | high | visibility judgment must count user-message skill text without a false repeat after compaction, and a subagent preload must still get the fallback prose; Phase 1 depends on a live probe of messages() visibility |
| risk.fit | moderate | the Pi typed-command transform reverses the prior worker's no-hook conclusion and copies a Pi-internal block format |
| risk.test | moderate | engine-test worlds for user-message visibility are new; Pi block-format pin depends on the installed pi-coding-agent; 31 pre-existing Pi failures blur the baseline |
| risk.security_or_contract | moderate | the preload contract (allowlist, lead-use-mailbox exclusion, no key inlined, failure pass-through) must survive, and the wsflow mirror must stay byte-identical |

## Phases

### Phase 1: Claude mod playbook-unit dedupe and heading fix

Scope: `agents-plugin/hooks/claude-mod/` (`preload.ts`, `manual.ts`,
`register.ts`, their tests) mirrored to `agents-plugin-wsflow/hooks/claude-mod/`,
and the shim heading move in `agents-plugin/skills/*/SKILL.md` and
`agents-plugin-wsflow/skills/*/SKILL.md` (regenerate the skills manifest:
`cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc -run TestGenerateRealSkillsManifest`).
The preload path and `manual.ts`'s `dedupe` share one hint memory and one
visibility check (a shared helper or a new edge), since `preload.ts` is
driven only by its edges today.

First, a live probe (an engine test cannot answer it: the test engine's
`session.messages` is a fixture the test fills itself): run a real session
with the worktree's mod, e.g. `claude -p --plugin-dir <worktree>/agents-plugin`,
invoke a lead skill twice, and debug-log what `$.session.messages({ as:
'api' })` returns at the second `skill.prompt`: whether the first expansion's
text is there, and in what shape (a user message whose `content` is a block
array or a plain string; `isRepeat` walks `content` as blocks today). If the
expansion is not visible, stop and report: the visibility decision depends
on it. Record the observed shape in the Result; engine tests then cover the
logic against that shape.

Verification: `claude plugin test` and `claude plugin validate` for both
packages; wsflow skill bundle tests
(`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`); `go test ./...`
in `agents-plugin-tool`. Engine tests cover: skill twice -> second is pointer
+ fallback; skill twice, then `playbook_read` -> full body (after-pointer
rule); skill then `playbook_read` -> pointer naming the earlier copy;
`playbook_read` then skill -> pointer + fallback; a context-bearing read
with a different body -> full; earlier copy gone from the conversation ->
full body; `lead-use-mailbox` and other plugins untouched. A test asserts
every `<ws-preload>` shim's `# <title>` sits inside its block in both
packages.

### Result (72808475a) - 2026-10-31

Landed: `manual.ts` `repeatOf` (replaces `isRepeat`) counts `playbook_read`
results (trimmed for playbooks) and the body inside user-message text as
copies, and both tool-side pointers and the preload pointer line as
pointers; `readKey` keys `playbook_read` by `name` alone; a read whose one
copy is an expansion gets `inlinedPointerText` ("already above in this
conversation, inlined when a skill loaded it"). `preload.ts` gains a
`repeat(playbook, body)` edge, bound in `register.ts` to
`Manual.preloaded`, which shares the read path's hint memory and visibility
check; a repeat replaces the block with `preloadPointerLine` plus the
block's fallback prose. Mod mirrored byte-identical to wsflow; both
`hooks.json` descriptions updated. Every `<ws-preload>` shim's `# <title>`
moved inside its block (10 ws, 12 wsflow); skills manifest regenerated.

Live probe (Claude Code 2.1.314, `claude -p`, a scratch probe plugin whose
`skill.prompt` hook logged `$.session.messages({ as: 'api' })`; a scratch
plugin avoided a name clash with the installed ws plugin): the first
expansion is visible at the second `skill.prompt`. A model-invoked Skill is
a user message whose `content` is a block array: a `tool_result` ("Launching
skill: <name>", string content) and a `text` block holding the whole
expanded skill text. A typed slash command is a user message whose text
blocks include `<command-name>...` and then the expanded text. So the
inlined body is a substring of a user text block, never a whole block;
`repeatOf` matches it by inclusion, and `ApiMessageLike.content` accepts a
string or blocks.

Verification: `claude plugin test` in `agents-plugin` and
`agents-plugin-wsflow`: 538 pass, 0 fail each; `claude plugin validate`
passes for both; `python3 -m unittest
agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`: 12 tests OK;
`go test ./...` in `agents-plugin-tool`: exit 0. Engine tests in
`manual-engine.test.ts` cover every case listed above. The heading test
(`test_preload_shim_headings_sit_inside_their_blocks`) lives in the wsflow
bundle test, which already reads both packages' skills; it checks each
heading against its rsrc playbook heading and failed on the old shims.

### Phase 2: Pi playbook-unit dedupe and typed /skill: expansion

Scope: `agents-plugin-pi/src/lead-skills.ts`, `agents-plugin-pi/src/index.ts`,
`agents-plugin-pi/src/playbook-read-dedupe.ts`, `agents-plugin-pi/src/bridge.ts`
(its `ws__playbook_read` dedupe), `agents-plugin-pi/test/`; update the
lead-skills.ts header and the `registerWsSkillTool` doc that say typed
`/skill:` has no hook.

Verification: Pi `npm test` (compare against the base: 31 failures were
reported pre-existing and environment-caused at 449e731ec's base; rerun the
base to confirm); tests cover the `input` transform for an own skill,
pass-through for a foreign skill, an excluded skill, `source:
"extension"`, no key, and a read failure; cross-path dedupe among
`ws-skill`, a typed command, and `ws__playbook_read` against a visible
earlier copy, and no suppression by a forged or stale marker; the pinned
block format; the order of the new `input` handler relative to the fork
first-input framing handler (`index.ts:709-724`), asserting that a fork's
first input is handled exactly as before this ticket (today the framing
removes the leading `/skill:`, so a fork's first input is not expanded and
the new handler must not change that); and regressions showing a transformed typed
command still collapses to `/skill:<name> <args>` and is still found as the
active playbook by `lead-compaction.ts` (`SKILL_EXPANSION_RE` /
`humanTextOf`, `findActivePlaybook`).

Live checks after a plugin reinstall stay with the owner: in Claude, invoke
a lead skill twice and see a pointer the second time; in Pi, type
`/skill:lead-tune` and see the playbook body.

### Result (21e0ea53e) - 2026-10-31

Landed: `playbook-read-dedupe.ts` `dedupePlaybook` is one playbook unit
across `ws__playbook_read` results (trimmed body equal), `ws-skill` results
containing the body, and user messages containing it (typed `/skill:`
copies, provenanced by session entry id; a copy with no id is never pointed
at). Calls key by playbook name only; any pointer candidate of the unit
(either pointer prose, a marker in a read result, or a parsed unit marker in
ws-skill or user text), valid or not, yields the full body, so forged and
stale markers never suppress a read. Pointer forms: `"result"` keeps Pi's
pointer prose; `"block"` ("The full `<name>` procedure is already present
...") replaces a preload block; both carry the
`ws-pi-read-dedupe-v1` marker, with no fallback prose. The `"playbook.read"`
family and `playbookReadKey` are gone. `bridge.ts` playbook.read uses the
result form; `ws-skill` expansion uses the block form, keeping the
whole-result dedupe only for a skill that inlined nothing.
`computeTypedSkillInput` / `registerTypedSkillExpansion` (`lead-skills.ts`)
transform an interactive or rpc `/skill:<own skill>` into Pi's own `<skill>`
block (`formatSkillBlock`) around the expanded body; every other case
continues. `index.ts` defines the preload getter once and registers the
handler after the fork first-input framing handler, with an order comment.
Header and `registerWsSkillTool` docs updated; `wsflow-mirroring.md` now
says the heading sits inside the block and the Pi adapter expands it too.

Verification: Pi `npm test`: 2514 tests, 31 fail, 1 cancelled; the failing
set is identical to the base run at acc7e77ed (2507 tests, 31 fail, 1
cancelled). New tests cover the transform, each pass-through case
(`extension` source, non-command text, unknown, foreign, excluded, no
bridge, no key, read failure, throw), cross-path dedupe among `ws-skill`,
typed, and `ws__playbook_read`, forged/stale/broken markers, an entry
without id, compaction of a typed copy, the block format pinned against the
installed `AgentSession.prototype._expandSkillCommand`, the handler order via
`discoverAndLoadExtensions` plus framed fork text continuing, and
`humanTextOf` / `findActivePlaybook` on the transformed message. Lite review
of acc7e77ed..21e0ea53e: clean.

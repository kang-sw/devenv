# Plan: 260703-feat-lead-drain-ready-queue-skill — Phase 1: Author the lead-drain-ready-queue skill

## Relevant Ticket Contract

- Author `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` as a compact,
  static-prose skill in the same inlined-body shape as `lead-verify-discussion`
  / `lead-prefer-subagent` (no rsrc playbook indirection).
- Implement: ticket selection per Decisions, `lead-prefer-subagent` posture
  applied by reference (not duplicated prose), explicit-target handoff to
  `lead-proceed`.
- Register as user-invocable `/ws:lead-drain-ready-queue` (update
  `agents-plugin/skills/manifest.json` and any Codex-facing skill registration
  list alongside the other 14 entry skills).
- Update `ai-docs/mental-model/workflow-skills.md` with a new lead-* entry,
  matching the `lead-verify-discussion` entry's description style.
- Decide during implementation whether `agents-plugin-wsflow` needs a mirror
  copy (byte-identical mirror per existing inline-skill precedent, or
  hand-authored variant, or full-ws-only); record decision + rationale in the
  ticket's Phase 1 Result.
- Ticket selection rule (must appear verbatim-equivalent in skill text):
  1. Empty `ready/` → report and stop, no handoff.
  2. Read each ready candidate's `related:`/`parent:` frontmatter for explicit
     precedence language ("prerequisite", "predecessor", "must land first",
     "blocks", "depends on") naming another ticket not yet `done`/`dropped`.
  3. If that named ticket is also in `ready/`, prefer it first.
  4. No precedence signal among current ready candidates → oldest date-prefix
     ticket (FIFO) by default.
  5. A precedence annotation naming an unresolved ticket in `todo/`/`idea/`
     (not `ready/`, not done/dropped) has no in-ready target to defer to —
     treat as no signal, fall through to FIFO.
  6. Conflicting/unresolvable precedence annotations between two candidates →
     stop and ask the user; do not guess.
  7. Precedence resolution is single-level only — do not chase transitive
     chains.
  8. Container tickets (epic/workset) are not filtered at selection time;
     `lead-proceed`'s existing `scope_blocked=container-ticket` guard handles
     that case.
- Framing constraints: single-cycle only (no internal loop over `ready/`,
  no repeated `tickets.list` polling inside the skill); apply
  `lead-prefer-subagent` posture by invoking/referencing it, not by copying
  its body; hand off to `lead-proceed` with an explicit resolved ticket path
  (never call `lead-proceed` bare expecting it to infer "pick from ready/").
- Verification (from ticket): confirm skill text matches selection-rule and
  framing decisions; confirm `manifest.json` regenerates clean; run
  `agents-plugin/tests/test_skill_dispatch_contracts.py` and
  `agents-plugin-tool/internal/wsrsrc` manifest/drift tests; add a new
  dispatch-contract test asserting the skill is an inlined static body and
  contains the FIFO-fallback + precedence-language selection text, mirroring
  `test_verify_discussion_is_inlined_static_body`.

## Out of Scope

- Changing `lead-proceed`'s own routing judges (its `judge: actionable`
  branch, `scope_blocked=*` semantics) — only its call target changes,
  from this new skill's side.
- Any new structured ticket dependency/`blocks:` frontmatter field.
- Any change to `lead-prefer-subagent` itself.
- An internal repeat-until-`ready/`-is-empty loop (explicitly rejected in the
  ticket; the `/goal` Stop-hook owns repetition).
- Free-form "most impactful ticket" judgment (explicitly rejected).

## Codebase Findings

- `agents-plugin/skills/lead-verify-discussion/SKILL.md#L1-L34` — exact shape
  to copy for a static-inline skill: YAML frontmatter (`name`, `description`),
  H1 title, then plain Markdown body with no `ws/playbook.print` indirection.
- `agents-plugin/skills/lead-prefer-subagent/SKILL.md#L1-L22` — the posture
  text this new skill must reference (by skill invocation), not restate. Its
  first line: "Maximum-delegation posture for this session: delegate all
  payload execution..." — good short paraphrase anchor for the reference
  sentence in the new skill.
- `agents-plugin/skills/lead-proceed/SKILL.md#L1-L10` — thin playbook-print
  shim; the new skill hands off by invoking `ws:lead-proceed` (or its
  underlying mechanism) with an explicit resolved ticket path as the target,
  matching the ticket's "Explicit-target handoff" decision.
- `agents-plugin/skills/manifest.json#L1-L24` — flat map of
  `"<skill-dir>/<file>": "<sha256>"`. New entry needed:
  `"lead-drain-ready-queue/SKILL.md": "<sha256-of-new-file>"`, alphabetically
  positioned between `lead-bootstrap/*` entries and `lead-forge-mental-model`
  (i.e. right after `lead-discuss/agents/openai.yaml`, before
  `lead-forge-mental-model/SKILL.md`) — the map is sorted by key.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L23-L95` — the
  manifest is regenerated via
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  (note the mandatory `-count=1`); do not hand-edit the sha256 value, run this
  regen command instead and let it rewrite `manifest.json`.
- `agents-plugin-tool/internal/wsrsrc/manifest.go#L41-L76` (`GenerateManifest`,
  `WriteManifest`) — underlying manifest generation logic invoked by the regen
  test; no direct manual edit needed beyond running the regen command.
- `ai-docs/mental-model/workflow-skills.md#L66` — style anchor for the new
  bullet: the `lead-verify-discussion` entry line describes purpose, static
  inline-body note, and links a `{#anchor}` tag. New entry should follow the
  same shape (short purpose statement, note that it's a static inline body,
  new stable `{#tag}` anchor, e.g. `{#260703-drain-ready-queue-skill}` per the
  ticket's declared `spec:` stem).
- `ai-docs/mental-model/workflow-skills.md#L17` — "Entry Points" bullet lists
  count of directly user-invocable entry skills ("13 directly
  user-invocable `ws:` entry skills"); this count needs bumping by one for the
  new skill (ticket phase text says "the other 14 entry skills" meaning it
  becomes the 15th — confirm actual current count from
  `agents-plugin/skills/manifest.json`'s top-level SKILL.md directories before
  editing this sentence, since the ticket phase text and this doc line may be
  counting slightly different sets — treat `manifest.json`'s directory count
  as ground truth, not the ticket prose's "14").
- `agents-plugin/tests/test_skill_dispatch_contracts.py#L54-L62`
  (`test_verify_discussion_is_inlined_static_body`) — exact pattern for the
  new required test: read the new `SKILL.md`, assert it does NOT contain
  `ws/playbook.print(name: "lead-drain-ready-queue")`, and assert it DOES
  contain distinguishing selection-rule substrings (e.g. a FIFO-fallback
  phrase and a precedence-language phrase such as "prerequisite" or "must
  land first").
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L16-L41` —
  `EXPECTED_SKILLS`, `EXPECTED_INLINE_SKILLS` sets are hand-maintained; if the
  wsflow-mirror decision is "yes, mirror it", both sets need
  `"lead-drain-ready-queue"` added, plus a byte-identical
  `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` file (verified
  via `diff` against the full-ws copy — see risk signal below).
- Risk signal (mirror decision): `diff` shows
  `agents-plugin/skills/lead-verify-discussion/SKILL.md` and
  `agents-plugin/skills/lead-prefer-subagent/SKILL.md` are byte-identical to
  their `agents-plugin-wsflow/skills/...` counterparts — confirming the
  established pattern for inline skills is a literal byte-for-byte copy, not
  a templated/substituted variant. If the new skill's body avoids `ws/`,
  `ws:`, `ws.` full-namespace tokens (it should, since it only names other
  skills like `lead-prefer-subagent`/`lead-proceed` and describes ticket-file
  conventions), a plain copy satisfies
  `test_skill_files_do_not_reference_full_ws_agent_surface` in
  `test_wsflow_skill_bundle.py#L82-L91` without edits. Recommend mirroring
  (matches existing precedent for checkpoint-style skills); this is a
  reasonable in-scope implementation call, not an escalation.
- `agents-plugin/tickets/../260703-feat-lead-drain-ready-queue-skill.md` cites
  `spec: 260703-drain-ready-queue-skill` in frontmatter — confirm whether
  `ai-docs/spec/` already has this stem or whether a new spec entry/anchor tag
  is expected; if a spec file exists, its exact anchor slug should be reused
  for the workflow-skills.md `{#...}` tag added in this phase. (Not directly
  verified in this survey — check `ai-docs/spec/` for a matching stem/anchor
  before writing the doc bullet; if absent, this phase phase can still add a
  descriptive bullet without spec cross-linking obligations, since spec
  authorship is not listed as a phase requirement.)

## Implementation Plan

1. Create `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`:
   - Frontmatter `name: lead-drain-ready-queue` and a `description:` line
     matching the manifest/routing style of sibling skills (short trigger
     description, third line pattern seen in `lead-prefer-subagent`).
   - H1 title (e.g. `# Drain Ready Queue`).
   - Body sections covering, in the style of `lead-verify-discussion`'s
     `## Checks` / `## Process` / `## Output` grouping (adapt names as
     needed, e.g. `## Selection Rule` / `## Process` / `## Handoff`):
     - Single-cycle framing statement (resolves at most one ticket per
       invocation; no internal loop; explicitly note the `/goal` Stop-hook
       owns repeated draining).
     - The full 6-step ticket selection rule from the ticket Decisions
       (empty-queue stop; precedence-annotation lookup in `related:`/
       `parent:`; in-ready precedence preference; FIFO default; no-in-ready-
       target-falls-through-to-FIFO case; conflict-stops-and-asks case;
       single-level-only resolution note).
     - A sentence applying `lead-prefer-subagent` posture by reference (e.g.
       "Apply the `lead-prefer-subagent` posture for this invocation" or
       equivalent skill-reference language) — do not restate its body.
     - A sentence requiring explicit-target handoff: invoke `lead-proceed`
       passing the resolved ticket path as an explicit target, never bare.
   - Do not add a `ws/playbook.print` indirection line — this must stay a
     fully inlined static body per ticket decision.
2. Compute the new file's sha256 and add it to
   `agents-plugin/skills/manifest.json` in sorted-key position (do not hand-
   compute if the toolchain provides a deterministic regenerate path — prefer
   running the regen command below over manual hash pasting).
3. Regenerate/verify the manifest via:
   `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
   run from `agents-plugin-tool/`, then re-run the plain manifest test to
   confirm it's clean.
4. Add a new bullet to `ai-docs/mental-model/workflow-skills.md`'s "Entry
   Points" or skill-list area describing `lead-drain-ready-queue`, matching
   the `lead-verify-discussion` entry's style (purpose, static-inline-body
   note, `{#tag}` anchor). Update the "13 directly user-invocable" count
   sentence at `#L17` to the corrected total once the new skill file exists
   (verify actual directory count rather than trusting "14" from ticket
   prose).
5. Decide the wsflow-mirror question per the Codebase Findings risk-signal
   note above; if mirroring:
   - Create a byte-identical
     `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md`.
   - Add `"lead-drain-ready-queue"` to both `EXPECTED_SKILLS` and
     `EXPECTED_INLINE_SKILLS` in
     `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`.
   If not mirroring, record the rationale in the ticket Phase 1 `### Result`
   section per the ticket's explicit instruction to record this decision.
6. Add a new test to `agents-plugin/tests/test_skill_dispatch_contracts.py`
   mirroring `test_verify_discussion_is_inlined_static_body` (around
   `#L54-L62`): read the new `SKILL.md`, assert absence of
   `ws/playbook.print(name: "lead-drain-ready-queue")`, and assert presence of
   distinguishing selection-rule substrings (FIFO-fallback phrase +
   precedence-language phrase, e.g. "prerequisite" and a FIFO/"oldest
   date-prefix" phrase actually used in the authored skill text).
7. If a "Codex-facing skill registration list" beyond `manifest.json` exists
   (ticket phase text mentions this alongside manifest.json) — search for any
   other enumerated skill-name list (e.g. a runtime skill index) and add the
   entry there too if found; `runtime.json` is confirmed out of scope per
   `ai-docs/mental-model/workflow-skills.md#L104` ("Entry skills are not
   listed in `runtime.json`").

## Verification Plan

- `cd agents-plugin && python -m pytest tests/test_skill_dispatch_contracts.py -q`
  (or `python -m unittest tests.test_skill_dispatch_contracts` per repo
  convention) — confirm new inline-body test passes and existing tests still
  pass.
- If wsflow mirror added: `cd agents-plugin-wsflow && python -m pytest tests/test_wsflow_skill_bundle.py -q`
  (or equivalent unittest invocation) — confirm bundle inventory/drift tests
  pass with the new skill included.
- `cd agents-plugin-tool && WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  then `go test ./internal/wsrsrc -count=1` — confirm manifest regenerates
  clean and no drift/hash-mismatch failures remain.
- Manual check: re-read the authored `SKILL.md` body against the ticket's
  Decisions section line-by-line (empty-queue stop, precedence lookup,
  in-ready preference, FIFO default, no-in-ready-target fallthrough, conflict
  stop-and-ask, single-level-only, container-ticket non-filtering,
  posture-by-reference, explicit-target handoff, single-cycle/no-internal-
  loop framing) to confirm no decision was dropped or altered.

## Escalations

- None.

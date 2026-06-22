# Brief: 22-260622-reviewer-playbooks

## Intent

Phase 2 of `260622-feat-sage-review-ticket-gate`. Add two new ticket reviewer
delegate playbooks (`ticket-reviewer-design`, `ticket-reviewer-completeness`)
and extend `lead-write-ticket` with a Sage Review Gate that invokes them in
parallel after a ticket is committed to `todo/+`.

## Scope Boundary

Phase 2 only:
- `ticket-reviewer-design` rsrc playbook (new)
- `ticket-reviewer-completeness` rsrc playbook (new)
- `lead-write-ticket` Sage Review Gate addition (modified)

Deferred (out of scope for Phase 2):
- Phase 3: `sage_review*` config key registration in `scope.go`
- Phase 3: `260620` transition tool integration (sage-review pre-condition check)
- `lead-write-ticket` integration with `tickets.create` for file creation (Decisions note, not Phase 2 scope)
- Wsflow entry-skill shims (reviewer playbooks are internal delegate playbooks, not user-invocable entry skills)

## Caller-Visible Contract

### ticket-reviewer-design

Invoked via `ws/playbook.render(name: "ticket-reviewer-design")` then spawned as a
native subagent. Input: ticket path (passed as task-specific input to the spawned agent).
Output (text returned to calling lead):

```
verdict: <pass|concern|block>

issues:
  - title: <short label>
    severity: <critical|important|minor>
    detail: <what is unclear or wrong>
    resolution: <autonomous|missing>
```

Verdict:
- `pass`: implementation can proceed without design changes.
- `concern`: issues exist but implementer can likely resolve autonomously; lead decides.
- `block`: critical design issues; ticket should be revised before implementation.

Dimensions covered: design coherence, duct-tape detection, right-problem check,
autonomous-vs-missing gap judgment.

### ticket-reviewer-completeness

Same invocation shape as design reviewer. Covers: ticket structure, missing
fields, "unclear to fresh reader" items. Same verdict schema.

Completeness reviewer does NOT read linked docs; reads only the ticket file.

### lead-write-ticket (modified)

Added step 8 "Sage Review Gate" between the existing step 7 (Commit) and step 8
(Handoff, renumbered to 9). The gate fires when:
1. Landing status is `todo/` or `ready/` (not `idea/`)
2. `sage_review` config value is `auto` or `ask`

Gate behavior:
- `ask` mode: lead asks user whether to run. "No" → write `sage-review: skipped`, commit, skip.
- `auto` (or user said yes in `ask`): spawn both reviewers in parallel, aggregate verdicts.
- Aggregation: design `block` → final block regardless of completeness. Completeness `concern` → lead judgment.
- Block outcome: write `## Blocked (YYYY-MM-DD)` section, set `sage-review: blocked` in frontmatter, commit.
- Pass/concern-resolved outcome: set `sage-review: completed` in frontmatter, commit.

If `sage_review` config is `off`, empty, or unset, the gate is skipped entirely.
(Phase 3 registers the config keys; before Phase 3, the key returns empty → gate inactive. This is correct: default `off` behavior.)

## Contract Instructions

### New file: `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`

Frontmatter:
```yaml
---
kind: render
delegates: true
role: reviewer
tier: large
variables:
  - RoleModel
---
```

File body (exact structure):

```markdown
# Ticket Reviewer — Design

You are a ticket design reviewer. You receive a ticket path, read the ticket and
its linked documents, attempt to sketch an implementation plan, and emit a
structured verdict on design quality.

Read-only: never write files, never commit, never call mutation tools. Return
verdict text only.

Alias model for this role: {{.RoleModel}}.

## Constraints

- Do not edit ticket files, commit, or call any mutation tool.
- Read the ticket file at the provided path, then any spec files in `spec:` frontmatter,
  mental-model docs in `related-mental-model:` frontmatter, and related tickets listed
  in `related:` frontmatter that have explicit constraint relevance.
- Do not load conversation history or session context.
- All output in English.

## Process

1. Read the ticket file at the provided path.
2. If `spec:` frontmatter entries present: read referenced spec sections (lookup via
   `{{.McpNamespace}}/specs.find`).
3. If `related-mental-model:` entries present: read referenced mental-model docs via
   `{{.McpNamespace}}/mental_models.status`.
4. Attempt to produce a coherent high-level implementation plan sketch for the ticket's
   current unfinished phase(s).
5. Evaluate whether a competent implementer can execute without filling in major design gaps.
6. For each identified issue, classify severity and resolution.
7. Emit verdict using the Output format below.

## Checklist

1. **Design coherence**: Is the design internally consistent? Can the stated goals be
   achieved with the described approach?
2. **Duct-tape detection**: Does the approach paper over a deeper problem instead of
   addressing root cause?
3. **Right-problem check**: Is the ticket solving the right problem, or is it a
   solution in search of a problem?
4. **Autonomous-vs-missing gap**: Can an implementer complete this without user
   decisions? Flag decisions the implementer would need but that aren't captured.

## Output

Return a text result with this exact structure:

```
verdict: <pass|concern|block>

issues:
  - title: <short label>
    severity: <critical|important|minor>
    detail: <what is unclear or wrong>
    resolution: <autonomous|missing>
```

Omit `issues:` list entirely on `pass` with no issues.

`resolution: autonomous` — the implementer can resolve this without a user decision.
`resolution: missing` — a user decision or design input is required.

## Doctrine

The reviewer optimizes for **implementer unblocking**: surface decisions the
implementer would need but cannot derive from the ticket, detect premature
commitment to the wrong solution, and flag incomplete scope that would leave a
fresh implementer guessing.
```

### New file: `agents-plugin/rsrc/ticket-reviewer-completeness/ticket-reviewer-completeness.md`

Frontmatter:
```yaml
---
kind: render
delegates: true
role: reviewer
tier: medium
variables:
  - RoleModel
---
```

File body (exact structure):

```markdown
# Ticket Reviewer — Completeness

You are a ticket completeness reviewer. You receive a ticket path, read the
ticket, and emit a structured verdict on ticket structure, fields, and clarity.

Read-only: never write files, never commit, never call mutation tools. Return
verdict text only.

Alias model for this role: {{.RoleModel}}.

## Constraints

- Do not edit ticket files, commit, or call any mutation tool.
- Read only the ticket file at the provided path; do not load linked docs, specs,
  or mental-model files.
- All output in English.

## Process

1. Read the ticket file at the provided path.
2. Evaluate structure, required fields, and fresh-reader clarity.
3. Emit verdict using the Output format below.

## Checklist

1. **Ticket structure**: `## Background`, phase sections (`### Phase N:`), and
   verification expectations present for each phase?
2. **Missing fields**: `title:` populated (not empty placeholder)? Frontmatter
   `related:` or `spec:` links present when behavior is externally visible?
3. **Fresh-reader clarity**: Can a fresh reader understand the goal, approach, and
   acceptance criteria without prior conversation context?
4. **Phase completeness**: Each phase has a clear completion boundary and does not
   have open-ended scope?
5. **Verification expectations**: Each phase has at least one explicit test, probe,
   or acceptance check?

## Output

Return a text result with this exact structure:

```
verdict: <pass|concern|block>

issues:
  - title: <short label>
    severity: <critical|important|minor>
    detail: <what is unclear or wrong>
    resolution: <autonomous|missing>
```

Omit `issues:` list entirely on `pass` with no issues.

`resolution: autonomous` — the issue can be fixed without a user decision.
`resolution: missing` — the issue requires user input or design work.

## Doctrine

The reviewer optimizes for **fresh-reader completeness**: every necessary piece
of context for an independent implementer must be in the ticket or an explicit
link; implicit knowledge gaps block implementation.
```

### Modified file: `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`

Two changes:

**Change 1: Add step 8 in "On: invoke" handler.**

The existing handler steps are:
```
1. Resolve
2. Route
3. Load
4. Consent Gate
5. Write
6. Verify
7. Commit
8. Handoff
```

Change to:
```
1. Resolve
2. Route
3. Load
4. Consent Gate
5. Write
6. Verify
7. Commit
8. Sage Review Gate
9. Handoff
```

Specifically, insert after the existing `### 7. Commit` block and before the existing
`### 8. Handoff` block:

```markdown
### 8. Sage Review Gate

1. Run **Sage Review Gate**.
```

Then rename the existing `### 8. Handoff` to `### 9. Handoff`.

**Change 2: Add "On: Sage Review Gate" handler.**

Insert the following new top-level section after the existing "On: Output Handoff"
section (after the `Ticket:` emit rule), before the "On: Cross-ticket decision review"
section:

```markdown
## On: Sage Review Gate

1. If landing status is `idea/`, skip this gate.
2. Call `{{.McpNamespace}}/config.show()` and extract the `sage_review` value.
3. If `sage_review` is `off`, empty, or unset, skip this gate.
4. If `sage_review` is `ask`: ask the user "Run sage review for this ticket?".
   - If user declines: add `sage-review: skipped` to ticket frontmatter, commit with
     `{{.McpNamespace}}/git.commit(paths: ["<ticket-path>"], title: "chore(sage): skip sage review", ai_context: ["user declined sage review in ask mode"])`,
     then skip the rest of this gate.
5. Spawn both reviewers in parallel:
   a. Render `ticket-reviewer-design`: call `{{.McpNamespace}}/playbook.render(name: "ticket-reviewer-design")`;
      spawn native subagent with rendered prompt; task input: `Ticket path: <ticket-path>`.
      Capture design verdict result.
   b. Render `ticket-reviewer-completeness`: call `{{.McpNamespace}}/playbook.render(name: "ticket-reviewer-completeness")`;
      spawn native subagent with rendered prompt; task input: `Ticket path: <ticket-path>`.
      Capture completeness verdict result.
6. Parse `verdict:` from each result.
7. Apply aggregation:
   - Design `block` → final verdict is `block` regardless of completeness.
   - Design not-block and completeness `block` → final verdict is `block`.
   - Design `concern` and completeness `pass|concern` → lead judgment: default to `pass`
     unless issues are `resolution: missing`.
   - All `pass` → final verdict is `pass`.
8. If final verdict is `block`:
   a. Write `## Blocked (YYYY-MM-DD)` section to ticket body using the **Blocked Section Template**.
   b. Add or update `sage-review: blocked` in ticket frontmatter.
   c. Commit with `{{.McpNamespace}}/git.commit(paths: ["<ticket-path>"], title: "docs(sage): block ticket on sage review", ai_context: ["sage review blocked: design and/or completeness issues"])`.
9. If final verdict is `pass` or `concern` resolved to pass:
   a. Add or update `sage-review: completed` in ticket frontmatter.
   b. Commit with `{{.McpNamespace}}/git.commit(paths: ["<ticket-path>"], title: "docs(sage): mark sage review completed", ai_context: ["sage review passed"])`.

## Templates

### Blocked Section Template

```markdown
## Blocked (YYYY-MM-DD)

### Design Reviewer — <verdict>

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | <title> | <severity> | <resolution> |

### Completeness Reviewer — <verdict>

| # | Title | Severity |
|---|-------|----------|
| 1 | <title> | <severity> |
```
```

Note: place `## Templates` section **at the end of the file** (after the existing
"## Doctrine" section), because the file currently has no Templates section. Or
place it before Judgments — follow whichever placement the file uses for similar
structured content. Check if the file already has a Templates section; if not,
add it at the end before Doctrine, consistent with skill layout order (Invariants →
Handlers → Judgments → Templates → Doctrine).

### Manifest and wsflow mirror regeneration

After creating both new playbook files:

```bash
cd agents-plugin-tool

# Step 1: Regenerate the ws rsrc manifest
WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest

# Step 2: Regenerate the wsflow rsrc mirror (byte-identical copy)
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
```

Both must succeed. After regen, `agents-plugin/rsrc/manifest.json` must contain entries
for `ticket-reviewer-design/ticket-reviewer-design.md` and
`ticket-reviewer-completeness/ticket-reviewer-completeness.md`.

### Fresh-Reader Audit

After creating/modifying files, spawn a fresh subagent with ONLY the target file text
and the instructions from skill-authoring "On: Fresh-Reader Audit". Classify findings
as fix/intentional/out-of-scope. Fix only `fix` findings. Apply to:
1. `ticket-reviewer-design.md`
2. `ticket-reviewer-completeness.md`
3. The modified sections of `lead-write-ticket.md`

## Integration Test Instructions

Run from `agents-plugin-tool/`:

```bash
# After manifest regen:
go test ./...
```

Pass criteria:
- `go test ./...` green (no new failures).
- `manifest.json` contains both new reviewer playbook entries.
- wsflow rsrc mirror contains both new reviewer playbook files.
- `lead-write-ticket.md` has step 8 "Sage Review Gate" in "On: invoke".
- `lead-write-ticket.md` has "On: Sage Review Gate" handler.
- Fresh-Reader Audit completed for all three changed files (lead may delegate this inline).

## Implementation Strategy Decisions

- **No wsflow entry-skill shims**: reviewer playbooks are internal delegate playbooks
  (`kind: render`), not user-invocable entry skills. No `EXPECTED_SKILLS` update,
  no wsflow shim `SKILL.md` needed. The wsflow rsrc mirror regen automatically copies
  them.
- **No `includes:` in ticket reviewers**: code-review-* playbooks include `code-reviewer`
  for the shared severity model and output format. Ticket reviewers have a different
  verdict schema (yaml-structured, not markdown findings) and are self-contained.
- **No `tickets.create` integration in lead-write-ticket**: the Decisions section notes
  that `lead-write-ticket` should call `tickets.create`, but this is not in Phase 2 scope.
  The sage gate writes `sage-review:` frontmatter directly. Deferred to a Phase 2 Edition
  or follow-up ticket.
- **Gate inactive before Phase 3**: `sage_review` config key is unregistered until Phase 3
  registers it in `scope.go`. `config.show` returns empty/absent → gate skips. This is
  correct behavior (default `off`).
- **`sage-review: completed` written by sage gate, not `tickets.create`**: Phase 1 writes
  `pending` via `tickets.create`. Phase 2's gate transitions it. For tickets created without
  `tickets.create` (current `lead-write-ticket` path), the gate adds the field fresh. Both
  paths result in correct frontmatter after the gate runs.
- **Blocked Section Template placement**: add a `## Templates` section to `lead-write-ticket`
  per skill layout order (Invariants → Handlers → Judgments → Templates → Doctrine). If the
  file already has a Doctrine section at end, insert Templates before it.

## Rejected Alternatives

- Making ticket reviewers include `code-reviewer`: ticket review has a different output
  schema (structured yaml verdict vs markdown findings sections). Including code-reviewer
  would inject irrelevant severity model and output templates. Rejected.
- Gating sage review on `sage-review: pending` existing in frontmatter: would silently
  skip new tickets created via direct `lead-write-ticket` (which doesn't call `tickets.create`
  yet). The gate triggers on landing-status + config, not frontmatter state. Rejected.
- Adding `sage_review` config read as a template variable: config is dynamic session state,
  not a playbook-time template variable. Must be read at runtime via `config.show`. Rejected.

## Approach

1. Create `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`.
2. Create `agents-plugin/rsrc/ticket-reviewer-completeness/ticket-reviewer-completeness.md`.
3. Modify `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`:
   a. Renumber "8. Handoff" to "9. Handoff".
   b. Insert "8. Sage Review Gate" step before Handoff.
   c. Add "On: Sage Review Gate" handler (after "On: Output Handoff").
   d. Add "## Templates" section with Blocked Section Template (before Doctrine).
4. Run manifest regen (both steps).
5. Run Fresh-Reader Audit on all three changed files; fix findings classified as `fix`.
6. Run `go test ./...`; verify green.
7. Commit.

## Constraints

- Reviewer playbooks are read-only: MUST state "never write files, never commit, never
  call mutation tools" in Constraints section.
- Both reviewers return the exact verdict schema (`verdict: pass|concern|block` +
  `issues:` list with `title/severity/detail/resolution`).
- Design reviewer tier: `large`. Completeness reviewer tier: `medium`.
- `lead-write-ticket` step numbers after change: 1-9 (Sage Review Gate at 8).
- Manifest regen MUST be run after creating new rsrc files; tests will fail without it.
- `WS_REGEN_MANIFEST=1` before `WS_REGEN_WSFLOW_RSRC=1` (order matters; wsflow mirror
  requires updated manifest as source).
- Do NOT add wsflow entry-skill shims (`EXPECTED_SKILLS`, wsflow SKILL.md) for reviewer
  playbooks — they are internal delegates.

## Out of Scope

- Phase 3: config key registration.
- Phase 3: transition tool integration.
- `lead-write-ticket` calling `tickets.create` for file creation.
- `sage_review_design_tier` / `sage_review_completeness_tier` config keys (Phase 3).
- Wsflow entry-skill shims for reviewer playbooks.
- Any playbook changes other than the three files listed above.

## Details

### Directory structure after implementation

```
agents-plugin/rsrc/
  ticket-reviewer-design/
    ticket-reviewer-design.md        ← new
  ticket-reviewer-completeness/
    ticket-reviewer-completeness.md  ← new
  lead-write-ticket/
    lead-write-ticket.md             ← modified (steps 8/9, new handler, new Templates)
```

### Verdict schema (exact, both reviewers)

```
verdict: pass | concern | block

issues:
  - title: <short label>
    severity: critical | important | minor
    detail: <what is unclear or wrong>
    resolution: autonomous | missing
```

### lead-write-ticket "On: invoke" step 8 insertion point

Current last two steps:
```markdown
### 7. Commit
...

### 8. Handoff
...
```

After change:
```markdown
### 7. Commit
...

### 8. Sage Review Gate

1. Run **Sage Review Gate**.

### 9. Handoff
...
```

### Blocked Section Template (exact format from ticket Decisions)

```markdown
## Blocked (YYYY-MM-DD)

### Design Reviewer — <verdict>

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | ...   | important | missing   |

### Completeness Reviewer — <verdict>

| # | Title | Severity |
|---|-------|----------|
| 1 | ...   | important |
```

## Verification Contract

1. `ticket-reviewer-design/ticket-reviewer-design.md` exists with `kind: render`,
   `tier: large`, identity line, Constraints, Process, Checklist, Output, Doctrine sections.
2. `ticket-reviewer-completeness/ticket-reviewer-completeness.md` exists with `kind: render`,
   `tier: medium`, same section structure.
3. Both playbooks: `verdict:` yaml block in Output section, no `includes: code-reviewer`.
4. `lead-write-ticket.md`: "On: invoke" has exactly 9 numbered steps; step 8 is
   "Sage Review Gate"; step 9 is "Handoff".
5. `lead-write-ticket.md`: "On: Sage Review Gate" handler present with config read,
   parallel reviewer spawn, aggregation logic, and frontmatter update.
6. `agents-plugin/rsrc/manifest.json` contains both new playbook file entries.
7. wsflow rsrc mirror contains both new playbook files.
8. `go test ./...` green (no new failures).
9. Fresh-Reader Audit completed for all three files; no unfixed `fix`-classified findings.

## References

- `[Must] agents-plugin/rsrc/code-review-correctness/code-review-correctness.md` — reviewer playbook frontmatter + structure pattern to mirror
- `[Must] agents-plugin/rsrc/reviewer/reviewer.md` — shared reviewer base (read to understand what NOT to include; ticket reviewers are self-contained)
- `[Must] agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` — file to modify; understand current step numbering and On: handler style
- `[Must] agents-plugin/skills/lead-skill-authoring/SKILL.md` — Fresh-Reader Audit procedure and invariant checklist
- `[Must] ai-docs/mental-model/workflow-skills.md` — Extension Points & Change Recipes: "Add a delegate prompt to a workflow" and "Add a Codex workflow skill" (for manifest regen commands)
- `[Must] ai-docs/tickets/ready/260622-feat-sage-review-ticket-gate.md` — NOT to read; instead use this brief which captures all binding Phase 2 decisions
- `[Maybe] agents-plugin/rsrc/code-review-fit/code-review-fit.md` — additional reviewer pattern reference
- `[Maybe] agents-plugin/rsrc/manifest.json` — check current format before regen

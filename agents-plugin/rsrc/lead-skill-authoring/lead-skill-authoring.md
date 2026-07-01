---
kind: print
---

# Skill Authoring

Canonical reference for skill and agent authoring.
Apply rules directly; add local procedure only when the target needs it.

## Layer Model

Every piece of skill content belongs to exactly one layer. Layers 1 and 2 are
owned by the MCP tool — delete their content from the playbook unconditionally.

| Layer | Owns | Model-accessible? | Delete from playbook? |
|-------|------|------|------|
| 1 — MCP schema | Input field names, types, enums, call format; response schema (Next: labels, state update field names) | Yes — via ToolSearch before call, tool response after | Yes — restatement drifts |
| 2 — MCP internal | Routing computation, verdict selection logic, `NextInstruction` content | No — black box; post-call instructions arrive via tool response | Yes — always invisible; "if Next: = X, do Y" playbook lines are Layer 2 output restatement |
| 3 — Playbook | Pre-call only: observation targets, soft judgments, non-obvious edge cases, step choreography, doctrine | Yes — this file | N/A — keep only what passes the destructive-first test |

Layer 3 is **pre-call only**. After the MCP call, the model follows `Next:` exactly; post-call branch handling belongs in `NextInstruction`, not in playbook prose.

### Gate: which layers apply?

- **Layer 1** applies when the skill calls a typed MCP tool (e.g. `enter.*`, `git.commit`).
- **Layer 2** applies when conditional routing/verdict logic has been moved into an MCP tool. If the logic still lives in playbook prose with no MCP tool computing it — file a Lever B migration ticket; do not audit as if Layer 2 is present.
- **Layer 3** always applies.

### Destructive-first stance

Burden of proof is on keeping content, not on deleting it.

Test for every section or rule:
- *"Would a model following only Layer 3 + MCP tool schemas reach the same execution outcome?"* — Yes → delete.
- *"Does this say what to do when Next: = X?"* — Yes → Layer 2 output restatement → delete.
- **No to both** → Layer 3; apply the invariant checklist before keeping.
- **Uncertain** → delete. A missing Layer 3 rule causes one wrong execution; a stale Layer 1/2 copy causes compounding drift.

Doctrine is Layer 3 only when at least one invariant re-derives from it; otherwise delete.

## Authoring Rules

### Reader model

- Audience is a model re-reading under attention pressure, not a leisurely human reader.
- One-liners survive pressure; paragraphs are skipped. Every rule fits one line.
- Preserve full grammar when compression could change order, ownership, or safety.

### Content

- Skills stand alone; reference other skills only as explicit invocation targets (`{{.SkillNamespace}}:<skill>`).
- Agents stand alone; do not reference session state or conversation history.
- Use examples only when they prevent repeated wrong execution.
- Prefer `Do X through Y` over `Do not do X` when a positive action exists.
- Skill-to-skill handoffs share the active conversation; write `Continue through <skill>` without a carry block.
- Skill-level user-approval gates apply only on direct user invocation; chained invocations re-ask only for safety, deletion, or explicit consent rules.
- When invoking another skill, name only the target skill and entry route; do not pre-decide its internal judgments.

### Iteration

- Repeatedly violated rule → mechanize with structure rather than restate it.
- Compress before adding: delete filler, merge duplicates, keep exact technical nouns.
- Add rules only for observed wrong executions or non-obvious constraints. Unspecified cases are intentional judgment gaps — leave them empty. If gap intentionality is unclear during authoring or audit, surface it to the human author before encoding.
- After each pass: re-read additions, cut, then apply the Layer test to every section.
- After edits: run **On: Fresh-Reader Audit**. After doctrine/routing/layout edits: also run **On: Downstream Consistency Sweep**.

### Invariant checklist

Every invariant must pass all six: **Falsifiable** (concrete violation describable?) · **Actionable** (says what to do, not just avoid?) · **One line** (fits without a paragraph?) · **Context-free** (understandable without surrounding file?) · **Non-redundant** (says something no other line covers?) · **Doctrine-aligned** (re-derives from the file's doctrine?).

Grouped invariant lists are allowed: `Group Name` / `- <invariant>`. Group names classify only; they are not rules.

### Doctrine format

Name the finite resource; state the guiding principle. Use measurable nouns ("context window", not "quality"). Keep only when at least one invariant re-derives from it.

## Skill Layout

**Routing skill** (Layer 2 applies — skill delegates decisions to `enter.*`):

Shape: `Invariants` → `On: invoke` (observe → judge → dispatch, thin) → `Judgments` → `Doctrine`

- Handlers are thin: gather facts, call `enter.*`, follow `Next:`. Routing logic belongs in the MCP tool.
- Keep `judge:` tables only for soft decisions the MCP tool cannot compute (ambiguous inputs requiring model assessment).
- H3 sub-blocks not needed for thin dispatch handlers.

**Choreography skill** (Layer 2 does not apply — sequential steps, no routing MCP tool):

Shape: `Invariants` → `On: X` handlers → `Judgments` → `Templates` → `Doctrine`

- Handlers may be multi-step; use H3 sub-blocks when a handler exceeds four steps with mixed responsibility.
- Name each sub-block by its responsibility; do not split single-purpose checklists.
- State each rule once: if an Invariant already captures a constraint, remove it from handler steps.
- Judge lives where its gated procedure lives; if the procedure is delegated to another skill, move the judge with it.

Both: directives at top, doctrine at bottom, never interleaved. Soft judgments extracted from handlers, named `judge: <name>`, referenced by name.

## Agent Layout

1. **Identity** — one sentence: what you are and what you do.
2. **Constraints** — scope boundaries, hard rules. Same checklist as skill invariants.
3. **Process** — step-by-step; typically a single linear flow.
4. **Heuristics** — decision tables, escalation criteria. Omit if decisions are purely mechanical.
5. **Output** — structured return format. Required.
6. **Doctrine** — one paragraph.

Agents start with no session context; keep self-contained. Inject communication rules from the calling skill, not the agent definition.

## On: Fresh-Reader Audit

Targets: `agents-plugin/rsrc/lead-*/lead-*.md` and `agents-plugin/skills/*/SKILL.md`.
Bar: a good-faith reader who applies the stated purpose without hunting for loopholes.

1. Run a separate fresh reviewer with only the target file; no prior conversation, project docs, or metadata.
2. Ask reviewer to flag: awkward, surprising, context-dependent, underspecified, contradictory, duplicated, orphaned, missing end-state wording, **and any section that restates an MCP tool schema (Layer 1/2 content in playbook)**.
3. Require per finding: quote, issue, severity (low/medium/high), suggested rewrite or deletion.
4. Classify: `fix` · `risk accepted` (record cost and risk) · `intentional difference` · `out of scope`.
5. Edit only `fix` findings. Max three cycles; stop when no `fix` remains or report unresolved.

## On: Routing Coverage Audit

Applies when Layer 2 is present (skill calls `enter.*` or equivalent).

1. Enumerate all `Next:` values the MCP tool can emit; use tool source or static-analysis subagent.
2. For each value: verify Layer 3 has adequate fact-gathering guidance for the model to reach it correctly.
3. For each underspecified `Next:` (post-call model judgment required): verify Layer 3 has supplemental guidance.
4. Flag branches with no Layer 3 coverage and branches where `Next:` is ambiguous without playbook support.

## On: Downstream Consistency Sweep

After doctrine, terminology, routing, layout, or audit-gate edits:

1. Select affected rsrc playbooks, entry skills, agent prompts, specs, and mirrored surfaces.
2. Conservative finding-only first pass; classify each finding as `fix` / `risk accepted` / `intentional difference` / `out of scope`.
3. Edit only `fix` findings; record intentional differences when drift would otherwise look stale.

## Doctrine

Skill and agent files are reread under attention pressure. Every choice optimizes for
**executability under pressure**: Layer-owned content deleted, skimmable imperatives
first, mechanical structure where judgment fails, preserved judgment where mechanism
would lose signal, rationale collapsed into doctrine. When ambiguous, choose what
the pressured model executes reliably.

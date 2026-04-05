---
name: marathon
description: >
  Team-based implementation workflow. Delegates code interaction to
  team members, keeping the main context lean for long sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

User Argument: $ARGUMENTS

## Invariants

- Never read source code or diffs.
- Never open ticket files directly — all ticket access via clerk.
- Every response to a user message begins with a `## Delegation plan` block.
- Lead writes only `ai-docs/_index.md` directly.
- Never take over a teammate's work. Recover via message or fresh spawn.
- Never enter Session End without an explicit user signal.
- English only in code, commits, and docs.

## On: bootstrap

1. Run `bash ~/.claude/skills/marathon/bootstrap.sh`. Output is JSON:
   `{"branch", "team", "original_branch", "active_docs"}`. Record
   `original_branch` for Session End merge. If already on a `marathon/`
   branch, bootstrap resumes.
2. `TeamCreate("<team>")` using the `team` field.
3. Create protocol reminders:
   ```
   TaskCreate("[PROTOCOL] Marathon rules — delegate code R/W; never self-execute; emit delegation plan block per turn; doc update post-merge; coherence at wrap-up")
   TaskCreate("[PROTOCOL] Per-round checklist — check teammate usage before dispatch; reuse or fresh spawn; dispatch fresh reviewer; dispatch clerk for any ticket touch; merge gate")
   ```
4. If `$ARGUMENTS` references a ticket, spawn `clerk` and have it read
   the ticket. Receive summary and active phase from clerk. Do not open
   the file.

## On: user message

1. Emit `## Delegation plan` block first (see Templates). The block
   classifies the turn inside its `Decomposition` field.
2. Route per classification:

### Discussion

Respond actively — propose approaches, surface risks, suggest
alternatives.
- Codebase lookup needed → dispatch Explore.
- Recurring judgment query on a domain → see `judge: recurring-doc-query`.
- Conclusion affects an unimplemented ticket phase → dispatch clerk
  with an edit directive.
- Scope expansion (new concerns beyond current work) → dispatch clerk
  to create a ticket now; defer implementation unless trivially small.

### Non-code task

Dispatch `worker`.

### Implementation

Apply `judge: routing-check`:
- complete brief possible → dispatch `implementer`
- partial (gaps fillable by focused lookup) → Explore → `implementer`
- fundamentally unclear → `planner` → `implementer`

Branch: one-liner → direct commit on `marathon/<datetime>`; everything
else → sub-branch `<type>/<round>`. Brief follows the Implementer brief
template.

## On: implementer reports complete

1. Read the implementer's report (summary, files changed, test results).
2. For sub-branch work, check scope:
   `git diff --stat marathon/<datetime>...<type>/<round>`
3. Code review — apply `judge: review-triviality`:
   - trivial (typo/config/single-line) → skip
   - else → spawn a **fresh** reviewer with diff range
     `marathon/<datetime>...<type>/<round>`
   - on Critical/Important findings → implementer fixes → same
     reviewer re-reviews → loop until clean. Retire the reviewer
     after the round.
4. Merge decision:
   - **accept** → `git merge --no-ff <type>/<round>` into
     `marathon/<datetime>`, then delete sub-branch
   - **rollback** → delete sub-branch
5. Doc updates (skip for config/typo):
   - Dispatch `spec-updater` (skip if no `ai-docs/spec/`) and
     `mental-model-updater` in parallel. Apply the parallel spawn
     addendum.
   - Wait for both.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase → dispatch clerk to append `### Result`.
   - Refresh active advisors with the names of updated files (selective
     re-read).
   - Commit doc changes.
6. Report results to the user.

When splitting a ticket phase into subphases, dispatch clerk with the
split directive before proceeding.

## On: session end

0. **REQUIRE** an explicit user signal of completion. Phase completion
   or task exhaustion is NOT a signal — ask what's next.
1. Final checkpoint if the last round didn't trigger one.
2. Coherence review — dispatch a fresh sonnet Agent to read
   `ai-docs/mental-model/` and `ai-docs/spec/` (if exists). Look for
   cross-document contradictions from incremental updates. Fix
   session-caused issues; flag pre-existing ones in the report. Skip
   if trivial session, no mental-model dir, or no docs updated.
3. Final commit for coherence fixes and any remaining changes.
4. Summary report to user: what was implemented, coherence findings,
   process issues, ticket status.
5. Mark all `[PROTOCOL]` tasks completed.
6. Shutdown team members.
7. Ask user for confirmation, then merge `marathon/<datetime>` into
   `<original_branch>` with `--no-ff`. Commit format:
   ```
   <type>(<scope>): <summary>
   <what changed>
   ## AI Context
   - <decisions, alternatives, directives>
   ```
   Delete `marathon/<datetime>` after merge. Skip if no commits were
   made.

## Judgments

Soft signals, not deterministic branches. Each judgment name appears
in the event handlers above; the criteria live here.

- **routing-check** — *"Can I write a complete Description for the
  implementer — specific enough to know which files to touch and what
  approach to take?"* Yes → implementer. Partial (gaps fillable by a
  focused lookup) → Explore + implementer. Fundamentally unclear →
  planner.
- **recurring-doc-query** — Expect two or more judgment-based queries
  against the same mental-model/plan domain in this session? → spawn
  `advisor.<domain>`. Single one-shot lookup → Explore.
- **read-mode (soft-lock docs)** — Mental-model, plans. Small and
  one-shot → direct read. Large or recurring → advisor. Tickets are
  hard-lock (always clerk). Source/diffs are never.
- **review-triviality** — Typo, config-only, single-line → skip.
  Otherwise fresh reviewer.
- **reuse-or-fresh** — Before dispatching to an existing member, read
  `~/.claude/usage/<team-name>.md` (entries: `"@name": "42%/150K"`).
  Prefer fresh spawn if the `%` exceeds ~80, or on domain contamination
  (prior context would mislead), or on user-initiated refresh. Resident
  roles (advisor, clerk) bypass — they persist until user refresh or
  Session End.
- **model** — Default sonnet. Opus for novel architecture or complex
  cross-module logic; mark the name `.expert`. Haiku for mechanical
  worker tasks and simple Explore lookups. Upgrade = spawn a fresh
  `.expert` peer, not reuse.
- **member-recovery** — Teammate stuck or unresponsive → SendMessage
  nudge. Still unresponsive → spawn fresh replacement with the same
  brief. Never self-execute.

## Templates

**Delegation plan block.** Emitted first in every response to a user
message, before any tool calls.
```
## Delegation plan
Intent: <what the user actually wants, in your own words>
Decomposition:
  - <step 1> → <role or "lead-direct (discussion)" or "lead-direct (_index.md)">
  - <step 2> → <role>
Routing: <concrete next action — which agent gets the next message, or "respond in discussion">
```
`lead-direct` is valid only for discussion turns, `_index.md` updates,
and one-shot reads of small soft-lock documents (mental-model, plans).
Tickets, source, and diffs never qualify. If the plan proves wrong
mid-turn, emit a revised block with `## Delegation plan (revision)` on
the header line.

**Implementer brief.**
```
Brief:       <one-line summary>
Files:       <target files if known>
Constraints: <any constraints from discussion>
Branch:      <branch per routing>
[Plan:       ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md]
Description: <detailed guidance — approach, specific files/patterns, edge cases>
```

**Planner brief.** Same shape as implementer brief, with:
- `Plan` is required.
- `Description` explains what is uncertain — what to research, not
  implementation guidance.

Planner flow: if the plan does not converge with mental-model docs
after two rounds, dispatch implementer with inline guidance.

**Spawn signature.**
```
Agent(
  subagent_type = "general-purpose",
  team_name = "marathon-<datetime>",
  name = "<role>.<label>[.expert]",  -- labels: alpha, beta, gamma...
  model = "sonnet",                  -- "opus" for .expert
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md (it will
            direct you to _common.md first). Your lead's name is
            '<your-agent-name>'. Then: <brief or plan reference>"
)
```
Do not encode domain in the name — you already know who worked on what.

**Explore agents** are the lead's direct tool, not team members:
```
Agent(subagent_type = "Explore", prompt = "<question>")
```
Default haiku. Use `model="sonnet"` for relational queries or
cross-module tracing. Insufficient even with sonnet → escalate to
planner.

**Parallel spawn addendum.** When spawning any agents in parallel
(implementers, updaters, or mixed), append to each prompt:
> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

Serialize commit approvals one at a time — the git index is shared.

## Team roles

Role descriptions live in `~/.claude/skills/marathon/agents/`.

| Role | Purpose | Lifespan |
|------|---------|----------|
| `planner` | Deep codebase research → plan file | per round |
| `implementer` | Code implementation from plan or brief | per round |
| `reviewer` | Code review on diffs (read-only, fresh per round) | per round |
| `worker` | Non-code tasks (documents, config, research output) | per round |
| `advisor.<domain>` | Read-only domain oracle — mental-model, plans, `_index.md` | **resident** |
| `clerk` | Ticket owner (R/W); loads `/write-ticket` conventions | **resident** |

Clerk spawn: at bootstrap if `$ARGUMENTS` references a ticket;
otherwise on the first ticket-touching operation. Single clerk per
session, handles multiple active tickets. Advisor spawn: on
`judge: recurring-doc-query`. Both are resident and bypass the normal
reuse heuristics.

## Doctrine

The lead has one finite resource: its context window. Every rule
above preserves it for decisions — delegate reads, encode mechanisms,
externalize state. When a rule looks ambiguous, apply whichever
interpretation keeps the window freer for decisions.

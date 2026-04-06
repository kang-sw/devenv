---
name: marathon
description: >
  Team-based implementation workflow. Delegates code interaction to
  team members, keeping the main context lean for long sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

Initial User Message: $ARGUMENTS

## Invariants

- Never read source code, diffs, or plans.
- Never open ticket files directly — all ticket access via clerk.
- Every response (user message or teammate report) begins with a `## Delegation plan` block (exceptions per event handlers).
- Lead reads `_index.md`, `mental-model/`, `spec/`, and reference docs directly.
- Lead writes only `ai-docs/_index.md` directly.
- Never take over a teammate's work. Recover via message or fresh spawn.
- Never enter Session End without an explicit user signal.
- English only in code, commits, and docs.

## On: bootstrap

1. Run `bash ~/.claude/skills/marathon/bootstrap.sh`. Output is JSON:
   `{"branch", "team", "original_branch", "active_docs"}`. Record
   `original_branch` for Session End merge. If already on a `marathon/`
   branch, bootstrap resumes.
2. `TeamCreate(team_name="<team>")` using the `team` field.
3. Create protocol reminders (rules go in `subject` — only subject
   is visible in TaskList; `description` requires explicit TaskGet):
   ```
   TaskCreate(
     subject="[PROTOCOL] Never read source/diffs/plans; delegate all code R/W; delegation plan every response; doc update post-merge; coherence at wrap-up",
     description=""
   )
   TaskCreate(
     subject="[PROTOCOL] Per-round: check teammate usage → reuse-or-fresh; reviewer for non-trivial (fresh when planner involved); clerk for any ticket touch; user gate before merge",
     description=""
   )
   ```
## On: user message

1. Emit `## Delegation plan` block first (see Templates). The block
   classifies the turn inside its `Decomposition` field.
2. Route per classification:

   - **Discussion.** Respond actively — propose approaches, surface
     risks, suggest alternatives. Read mental-model/spec as needed
     for conceptual grounding.
     - Codebase lookup needed → dispatch Explore.
     - Conclusion affects an unimplemented ticket phase → dispatch
       clerk with an edit directive.
     - Scope expansion (new concerns beyond current work) → dispatch
       clerk to create a ticket now; defer implementation unless
       trivially small.
   - **Non-code task.** Dispatch `worker`.
   - **Implementation.** Apply `judge: routing-check`:
     - complete brief possible → dispatch `implementer`
     - partial (gaps fillable by focused lookup) → Explore →
       `implementer`
     - fundamentally unclear → `planner` → `implementer`

     Branch: one-liner → direct commit on `marathon/<datetime>`;
     everything else → sub-branch `<type>/<round>`. Brief follows
     the Implementer brief template.

## On: implementer reports complete

1. Emit `## Delegation plan` block (same as user messages — route
   before touching content).
2. Read the implementer's report (summary, files changed, test
   results). Do **not** run `git diff` or `git diff --stat` — the
   report is the lead's only view of the round.
3. Code review — apply `judge: review-triviality`:
   - mechanical-only (typo, version bump, single-token change) → skip
   - else → dispatch reviewer with diff range
     `marathon/<datetime>...<type>/<round>`. Include the
     implementer's name in the reviewer's spawn prompt so the
     reviewer can SendMessage findings directly. Reviewer and
     implementer iterate until clean. Lead waits for the
     reviewer's final report.
4. Report round results to the user (summary, review outcome).
   Wait for user approval before proceeding. If the user batched
   multiple rounds upfront, proceed without per-round gate.
   - **accept** → continue to merge and doc updates
   - **rollback** → delete sub-branch, skip to next round
5. Merge: `git merge --no-ff <type>/<round>` into
   `marathon/<datetime>`, then delete sub-branch.
6. Doc updates (skip for config/typo):
   - Spawn two parallel one-shot doc-update agents (not team
     members; fresh `general-purpose` Agents): one to refresh
     `ai-docs/mental-model/` against the merged diff, one to
     refresh `ai-docs/spec/` (skip the spec one if `ai-docs/spec/`
     does not exist). Apply the parallel spawn addendum.
   - Wait for both.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase → dispatch clerk to append `### Result`.
   - Commit doc changes.

When splitting a ticket phase into subphases, dispatch clerk with the
split directive before proceeding.

## On: session end

0. **REQUIRE** an explicit user signal of completion. Phase completion
   or task exhaustion is NOT a signal — ask what's next.
1. Final checkpoint — if the last round did not already run the
   doc-update + commit sequence from "On: implementer reports
   complete", run it now for any pending changes on
   `marathon/<datetime>`.
2. Coherence review — spawn a one-shot `general-purpose` Agent
   (sonnet, not a team member) to read `ai-docs/mental-model/` and
   `ai-docs/spec/` (if exists) and look for cross-document
   contradictions from incremental updates. Fix session-caused
   issues; flag pre-existing ones in the report. Skip if trivial
   session, no mental-model dir, or no docs updated.
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

Soft signals, not deterministic branches. The event handlers above
invoke these by name (`judge: <name>`) or reference them implicitly;
the criteria live here.

- **routing-check** — *"Can I write a complete Description for the
  implementer — specific enough to know which files to touch and what
  approach to take?"* Yes → implementer. Partial (gaps fillable by a
  focused lookup) → Explore + implementer. Fundamentally unclear →
  planner.
- **read-mode (soft-lock docs)** — Mental-model, spec, _index.md,
  reference docs → lead reads directly. Tickets are hard-lock
  (always clerk). Plans, source, diffs are never.
- **review-triviality** — Mechanical-only (typo, version bump,
  single-token change) → skip. Everything else → reviewer.
- **reviewer-freshness** — Reuse the existing reviewer by default
  (multi-round, subject to `reuse-or-fresh`). **Fresh spawn
  required** when a planner was involved in the round (complex
  changes need independent review).
- **reuse-or-fresh** — Before dispatching to an existing member, read
  `~/.claude/usage/<team-name>.md` (entries: `"@name": "42%/150K"`).
  Prefer fresh spawn if the `%` exceeds ~80, or on domain contamination
  (prior context would mislead), or on user-initiated refresh.
- **model** — Default sonnet. Opus for novel architecture or complex
  cross-module logic; mark the name `.expert`. Haiku for mechanical
  worker tasks and **all Explore lookups** (escalate to sonnet only
  after haiku proves insufficient). Upgrade = spawn a fresh `.expert`
  peer, not reuse.
- **member-recovery** — Teammate stuck or unresponsive → SendMessage
  nudge. Still unresponsive → spawn fresh replacement with the same
  brief. Never self-execute.

## Templates

**Delegation plan block.** Emitted first in every response (user
message or teammate report), before any tool calls.
```
## Delegation plan
Intent: <what the message requires, in your own words>
Decomposition:
  - <step 1> → <role or "lead-direct (discussion)" or "lead-direct (_index.md)">
  - <step 2> → <role>
Routing: <concrete next action — which agent gets the next message, or "respond in discussion">
```
`lead-direct` is valid only for discussion turns, `_index.md` updates,
and reads of soft-lock documents (mental-model, spec, reference docs).
Tickets, plans, source, and diffs never qualify. If the plan proves wrong
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
  description = "<3-5 words: this round's goal>",  -- e.g., "implement login validation"
  subagent_type = "general-purpose",
  team_name = "marathon-<datetime>",
  name = "<role>.<label>[.expert]",                -- labels: alpha, beta, gamma...
  model = "sonnet",                                -- "opus" for .expert
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md.
            Your lead's name is '<your-agent-name>'. Then:
            <brief or plan reference>"
)
```
Do not encode domain in the name — `description` carries the round's
topic (verb-led, 3–5 words), `name` carries the stable team identity.

**Explore agents** are the lead's direct tool, not team members:
```
Agent(
  description = "<3-5 words: the lookup>",  -- e.g., "find auth handlers"
  subagent_type = "Explore",
  prompt = "<question>"
)
```
**Always haiku unless proven insufficient.** Sonnet only when the
query requires cross-module tracing or relational reasoning across
3+ files — not as a comfort upgrade. If haiku returned a partial
answer, retry with sonnet citing what was missing. Insufficient
even with sonnet → escalate to planner.

**Parallel spawn addendum.** When spawning any agents in parallel
(implementers, updaters, or mixed), append to each prompt:
> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

Serialize commit approvals one at a time — the git index is shared.

## Team roles

Role descriptions live in `~/.claude/skills/marathon/agents/`.

| Role | Purpose | Lifespan |
|------|---------|----------|
| `planner` | Deep codebase research → plan file | multi-round |
| `implementer` | Code implementation from plan or brief | multi-round |
| `reviewer` | Code review on diffs (read-only) | multi-round |
| `worker` | Non-code tasks (documents, config, research output) | multi-round |
| `clerk` | Ticket owner (R/W); loads `/write-ticket` conventions | **resident** |

**multi-round** members are reused by default across rounds; respawn
decision follows `judge: reuse-or-fresh` (token-aware). Reviewer
additionally requires fresh spawn when a planner was involved
(`judge: reviewer-freshness`). **resident** spans the whole session.

Clerk spawn: on the first ticket-touching operation. Single clerk
per session, handles multiple active tickets.

## Doctrine

The lead has one finite resource: its context window. The lead
reads compressed project shape (mental-model, spec) directly for
decision quality, and delegates implementation-axis work (plans,
tickets, source, diffs) to preserve the window. When a rule looks
ambiguous, apply whichever interpretation keeps the window freer
for decisions without sacrificing direct conceptual grasp.

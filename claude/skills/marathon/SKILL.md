---
name: marathon
description: >
  Team-based implementation workflow. Delegates code interaction to
  team members, keeping the main context lean for long sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

User Argument: $ARGUMENTS

## Doctrine

Marathon is **token-efficient** — keep the main context lean by
delegating. You are the **lead**: orchestrate discussion, decisions,
and review. **Do not read source code or diffs** — dispatch Explore
agents, reviewers, or team reports instead.

**Reading policy — layered.**
- **Always direct-read:** briefs, reports, reviewer verdicts, Explore
  results, monologue notes, bootstrap output, `ai-docs/_index.md`.
- **Soft-lock (mental-model, plans):** direct-read only when the
  document is small and the query is one-shot. For large documents or
  recurring queries, dispatch `advisor.<domain>`.
- **Hard-lock (tickets):** never opened directly. All ticket access —
  read or write — flows through `clerk`.
- **Never:** source code, diffs.

**Team board.** The lead maintains `ai-docs/_index.md` as its working
memory — session focus, active ticket pointers, per-domain advisor
pointers, short status notes. This is the only file the lead writes
directly; everything else is delegated.

## Delegation plan

Every response to a user message begins with a `## Delegation plan`
block, before any tool calls. The block classifies the turn and
commits the lead to a routing decision in writing.

Format:
```
## Delegation plan
Intent: <what the user actually wants, in your own words>
Decomposition:
  - <step 1> → <role or "lead-direct (discussion)" or "lead-direct (_index.md)">
  - <step 2> → <role>
Routing: <concrete next action — which agent gets the next message, or "respond in discussion">
```

**Hard rules.**
- `lead-direct` is valid only for: (1) discussion turns,
  (2) `_index.md` updates, (3) one-shot reads of small soft-lock
  documents (mental-model, plans). Tickets, source code, and diffs
  never qualify. Any other `lead-direct` entry is a rule violation
  and a signal the lead is about to self-execute.
- Discussion turns still emit the block. A one-line `Intent` plus
  `Decomposition: lead-direct (discussion)` plus `Routing: respond
  in discussion` is sufficient.
- If the plan proves wrong mid-turn, emit a revised block with
  `## Delegation plan (revision)` on the header line — do not
  silently re-route.
- Recurring `lead-direct` reads of the same soft-lock document
  across turns signal migration to an `advisor.<domain>` next turn.

**Mandatory, not conditional.** Carving exceptions re-opens the
attention loophole the block exists to close.

**Relationship to monologue.** Parallel to monologue, both mandatory:
monologue is free-form narration around tool calls, the delegation
plan is a structured entry-point checkpoint per user message.

## Step 0: Bootstrap

1. Run `bash ~/.claude/skills/marathon/bootstrap.sh`. It handles branch
   creation (`marathon/<datetime>`), token usage file init, and active
   docs listing. Output is JSON:
   ```json
   {"branch": "marathon/...", "team": "marathon-...",
    "original_branch": "...", "active_docs": "..."}
   ```
   If already on a `marathon/` branch, it resumes instead of creating.
   Record `original_branch` for Session End merge.
2. Create the team: `TeamCreate("<team>")` using the `team` field.
3. Create protocol reminders:
   ```
   TaskCreate("[PROTOCOL] Marathon rules — delegate code R/W; never self-execute; emit delegation plan block per turn; doc update post-merge; coherence at wrap-up")
   TaskCreate("[PROTOCOL] Per-round checklist — check teammate usage before dispatch; reuse or fresh spawn; dispatch fresh reviewer; dispatch clerk for any ticket touch; merge gate")
   ```
4. If `$ARGUMENTS` references a ticket, spawn `clerk` (see **Team
   Management** → clerk) and have it read the ticket; receive the
   summary and active phase from clerk. Do not open the file directly.
5. Team members are spawned on-demand. See **Team Management** below.

## Step 1: Marathon Loop

Repeat until the user signals done. Each turn is **either** discussion
or implementation — not a fixed sequence.

### Discussion (when needed)

Contribute actively — propose approaches, surface risks, suggest
alternatives. For mental-model and plan content, follow the soft-lock
reading policy (direct-read only when small and one-shot; otherwise
dispatch `advisor.<domain>`). For codebase details, dispatch an
**Explore agent**.

When a ticket exists and discussion produces conclusions that affect
unimplemented phases, dispatch **clerk** with an edit directive
(summary of the decision and the target phase). Do not open the
ticket file; clerk applies the edit.

### Implementation — routing

1. **Non-code** task → dispatch **worker**.
2. **Complex and uncertain** — change scope is unclear, multiple
   modules involved, and no ticket/plan to guide. Dispatch **planner**
   first to research the codebase and produce a plan, then
   **implementer**.
3. Otherwise → **implementer** directly (the common case).

**Routing check:** Before dispatching, ask: *"Can I write a complete
Description for this task — specific enough that the implementer knows
which files to touch and what approach to take?"* If yes → implementer
directly. If almost — use an **Explore agent** to fill the gaps. If
fundamentally unclear → planner.

**Branch:** one-liner → direct commit on `marathon/<datetime>`.
Everything else → sub-branch `<type>/<round>` from `marathon/<datetime>`.

**Implementer brief:**
```
Brief:       <one-line summary>
Files:       <target files if known>
Constraints: <any constraints from discussion>
Branch:      <branch per above>
[Plan:       ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md]
Description: <detailed guidance — approach, specific files/patterns, edge cases>
```

**Planner brief:** same structure as implementer, except:
- `Plan` is required (not optional).
- `Description` explains what is uncertain — what the planner should
  research, not implementation guidance.

**Planner flow:** If the planner's plan does not converge with mental-model docs after two rounds, dispatch implementer with inline guidance.

### After each implementation — merge gate

When the implementer reports completion:

1. Read the implementer's report (summary, files changed, test results).
2. For sub-branch work, review the scope:
   ```bash
   git diff --stat marathon/<datetime>...<type>/<round>
   ```
3. **Code review (pre-merge).** The lead MUST NOT review code or
   diffs directly — always dispatch to a fresh reviewer. Skip only
   for trivial rounds (typo, config-only, single-line). Spawn a
   **fresh reviewer** for each round with diff range
   `marathon/<datetime>...<type>/<round>`. If
   Critical/Important issues found: implementer fixes on sub-branch →
   same reviewer re-reviews → loop until clean. Retire reviewer after
   the round.

4. **Merge decision:**
   - **Accept** — `git merge --no-ff <type>/<round>` into
     `marathon/<datetime>`, then delete the sub-branch.
   - **Rollback** — delete the sub-branch entirely.
5. **Doc updates (post-merge).** Skip for config/typo changes.
   - Dispatch in parallel (fresh sonnet Agents, not team members;
     apply **parallel commit coordination**). Pass skill file paths
     so agents follow conventions:
     - **spec-updater** — skip if `ai-docs/spec/` does not exist.
     - **mental-model-updater**
   - Wait for both to complete.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase, dispatch **clerk** to append the
     `### Result` entry. Clerk owns `/write-ticket` conventions.
   - After mental-model or spec updates land, send any active
     `advisor.<domain>` a refresh directive naming the updated files
     (selective re-read only — see **advisor** in Team Management).
   - Commit doc changes.
6. Report results to the user.

When splitting a ticket phase into subphases, dispatch **clerk**
with the split directive before proceeding.

### Task discipline

Protocol tasks stay pending until Session End — they are reminders,
not work. Work tasks are optional; create only for multi-phase or
parallel work where the user benefits from progress visibility.

## Team Management

### Explore agents

Explore agents are **not team members** — they are the lead's direct
tool for codebase lookups. Spawn with `subagent_type="Explore"`:

```
Agent(
  subagent_type = "Explore",
  prompt = "<question about the codebase>"
)
```

Default model (haiku) handles file locations, signatures, and simple
pattern searches. Use `model="sonnet"` when tracing call chains or
cross-module relationships. If even a sonnet explore is insufficient,
the question likely needs a **planner**.

### Spawning team members

Role descriptions are in `~/.claude/skills/marathon/agents/`:

| Role file | Purpose |
|-----------|---------|
| `planner.md` | Deep codebase research → plan file |
| `implementer.md` | Code implementation from plan or brief |
| `reviewer.md` | Code review on diffs (read-only, fresh per round) |
| `worker.md` | Non-code tasks (documents, config, research output) |
| `advisor.md` | Read-only domain oracle — mental-model, plans, `_index.md`; resident |
| `clerk.md` | Ticket owner (R/W) — loads `/write-ticket`; resident |

Spawn general-purpose agents with a role file reference:

```
Agent(
  subagent_type = "general-purpose",
  team_name = "marathon-<datetime>",
  name = "<role>.<label>[.expert]", -- e.g., "impl.alpha", "impl.alpha.expert"
  model = "sonnet",                 -- or "opus" when .expert
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md (it will
            direct you to _common.md first). Your lead's name is
            '<your-agent-name>'. Then: <brief or plan reference>"
)
```

**Naming:** `<role>.<label>` with neutral labels (alpha, beta, gamma…).
Append `.expert` when spawning with opus — this makes the model visible
so you can make correct reuse-vs-upgrade decisions later. Examples:
`impl.alpha` (sonnet), `impl.beta.expert` (opus), `planner.alpha`.
Do not encode domain in the name — you already know who worked on what.

**Advisor spawn (reactive).** Spawn `advisor.<domain>` when two or
more judgment-based queries against the same mental-model/plan domain
are expected in the session. Single one-shot lookups still go to
Explore. Naming: `advisor.<domain>` (e.g., `advisor.auth`,
`advisor.indexing`). Resident across rounds; initial Read at spawn,
subsequent Reads only on lead-initiated refresh after known doc
updates. Default sonnet; haiku acceptable for simple lookups.

**Clerk spawn.** Spawn `clerk` at bootstrap if `$ARGUMENTS` references
a ticket; otherwise on the first ticket-touching operation
(discussion-driven phase edit, scope expansion, phase split,
post-merge `### Result` append). Single instance per session; handles
multiple active tickets. Default sonnet.

### Reuse policy

- **Default: reuse.** Send the next brief to an existing member.
- **Token-aware refresh.** Before dispatching to an existing member,
  read `~/.claude/usage/<team-name>.md`. Entries look like
  `"@name": "42%/150K"` — if the number before `%` exceeds **~80**,
  prefer spawning fresh. Updated automatically by a `TeammateIdle`
  hook.
- **User-initiated refresh.** The user will tell you when a member's
  context is getting stale. Finish the current round with them, then
  spawn fresh for the next.
- **Domain contamination.** If you judge that prior context will
  mislead a member on a new task, spawn fresh. This is your call.
- **Member recovery.** If a member appears stuck, unresponsive, or
  in a bad state: first `SendMessage` to check status and nudge.
  If still unresponsive, spawn a fresh replacement with the same
  brief. Never take over the work yourself.
- **Resident roles.** `advisor.<domain>` and `clerk` are resident for
  the session — they bypass the heuristics above and persist until
  user refresh or Session End.

### Parallel commit coordination

When spawning **any** agents in parallel (implementers, updaters, or
mixed), append to each spawn prompt:

> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

Then serialize commit approvals one at a time — the git index is
shared.

### Model selection

Default **sonnet** for all roles. When a task involves novel
architecture or complex cross-module logic, spawn with **opus** and
mark the name `.expert`. If an existing sonnet member needs upgrading,
spawn a fresh `.expert` peer instead of reusing. **Haiku** for
mechanical worker tasks and `claude -p --model haiku` exploration only.
**Explore agents** default to haiku; upgrade to **sonnet** for
relational queries (see **Explore agents** above).

## Step 2: Session End (when user signals done)

1. **Final checkpoint** — run one if the last round didn't trigger one.

2. **Coherence review** — dispatch a fresh **sonnet** Agent to read all
   of `ai-docs/mental-model/` and `ai-docs/spec/` (if exists). Look
   for cross-document contradictions from incremental updates. Fix
   session-caused issues; flag pre-existing ones in the report.
   Skip if trivial session, no mental-model dir, or no docs updated.

3. **Final commit** — coherence fixes and any remaining changes.

4. **Report** — summarize to the user:
   - What was implemented across the session
   - Coherence review findings (if any)
   - Process issues (if any)
   - Ticket status (if applicable)

5. **Clean up protocol tasks** — mark all `[PROTOCOL]` tasks as
   completed.

6. **Shutdown team** — send shutdown request to all team members.

7. **Merge** — ask user for confirmation, then merge `marathon/<datetime>`
   into `<original-branch>` with `--no-ff`. Commit message format:
   ```
   <type>(<scope>): <summary>
   <what changed>
   ## AI Context
   - <decisions, alternatives, directives>
   ```
   Delete `marathon/<datetime>` after merge. Skip if no commits were made.

## Rules

- **Never review code yourself.** The lead reads only reports,
  summaries, and reviewer verdicts — never code or diffs.
- **Never take over a member's work.** If a member is stuck or
  unresponsive, recover via SendMessage or spawn a replacement —
  never self-execute.
- **Never open ticket files directly.** All ticket access — read or
  write — flows through `clerk`.
- **Prefer advisor over direct reads** for mental-model and plans
  when queries recur or documents are large.
- **`_index.md` is the lead's working memory.** Maintain it directly;
  it is the only file the lead writes directly. Everything else is
  delegated.
- **Every response to a user message begins with a delegation plan
  block.** No exceptions. See the **Delegation plan** section above.
- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **Ticket as live document.** Keep unimplemented phases accurate as
  discussion evolves (with user agreement). Completed phases (with
  `### Result`) are immutable.
- **User controls session lifecycle.** Never enter Session End unless
  the user explicitly signals done. Completing a phase or running out
  of tasks is NOT a signal — ask what's next.
- **Scope expansion.** New concerns mid-session → dispatch `clerk`
  to create a ticket now while context is fresh (spawn clerk first
  if none exists); defer implementation unless trivially small.

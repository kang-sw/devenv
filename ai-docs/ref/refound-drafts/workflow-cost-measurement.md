---
summary: Qualitative before/after measurement of workflow cost from git history and the ticket tree; read-only, command-stated, project-neutral
---

# Workflow Cost Measurement

Measures what a workflow costs to run, from the history it already leaves:
the ticket tree, commit messages, and phase `### Result` sections. It is
applied once before and once after a workflow change, at named commits, and
the two runs are compared indicator by indicator. It produces no score and
no threshold; it produces figures with stated limits.

## Rules

- **Read-only.** Every command below prints. No branch is created, no ticket
  moved, no history rewritten. Run against a checked-out commit.
- **One window shape for both halves.** Both runs use the same selector,
  size, and exclusions. A run whose window shape differs from its
  counterpart is not comparable; record that and do not compare.
- **Record the run.** Each run records the commit hash, branch, date, the
  window definition, the exact commands as run, every indicator's value or
  `unavailable`, and any place this manual's wording proved ambiguous.
- **Missing conventions are `unavailable`, never substituted.** A project
  whose commits carry no `## AI Context`, or whose tickets carry no phases,
  records those indicators as unavailable for that window. Do not invent a
  proxy; a proxy silently changes what the comparison compares.
- **Indicators, not a score.** Do not combine them. The sample is small and
  the window is confounded by model changes and topic mix.
- **Record the commit convention in force.** Who commits (lead, per-phase
  delegate, one worker per ticket) and at what granularity. Indicators 2 and
  3 count commits, so a change in convention moves them without any change
  in abort or re-work; a comparison across conventions reports that first.

## Window

Default: the **20 most recently closed actionable tickets** on the measured
branch at the measured commit, where closed means present under
`ai-docs/tickets/.done/`, actionable means the stem's category is not
`epic`, `workset`, `research`, `idea`, or `design`, and recency is the
frontmatter `completed:` date, ties broken by stem. A project with fewer
than 20 records all of them and says so. Change the size only when both
halves change together.

Shell setup used by every command below, run from the repository root at
the measured commit. `BRANCH` is the branch the history is read on: the
project's tracked branch (`review-track` in `AGENTS.md` `### Review Policy`,
or the branch work lands on), set explicitly, never derived from `HEAD`,
because a run taken from a work branch would otherwise read that branch.

```sh
BRANCH=develop   # the tracked branch; set it, do not derive it
completed_date() { awk '/^---$/{c++; next} c==1 && /^completed:/{print $2; exit}' "$1"; }
epoch() { date -d "$1" +%s 2>/dev/null || date -j -f %Y-%m-%d "$1" +%s 2>/dev/null; }
window() {
  for f in ai-docs/tickets/.done/*.md; do
    stem=$(basename "$f" .md)
    cat=$(printf '%s' "$stem" | cut -d- -f2)
    case "$cat" in epic|workset|research|idea|design) continue;; esac
    d=$(completed_date "$f")
    if [ -z "$d" ]; then echo "skipped (no completed:): $stem" >&2; continue; fi
    printf '%s %s\n' "$d" "$stem"
  done | sort | tail -20 | awk '{print $2}'
}
```

`window` prints the stems, oldest first, and names on stderr every closed
actionable ticket it skipped for lacking `completed:`
(`window 2>&1 >/dev/null | grep -c skipped` counts them). Record both with
the run.

## Indicators

Each indicator states its command, its unit, and what a movement in it does
not license. Run each per stem in the window and report the distribution
(minimum, median, maximum, and the per-ticket values), never the mean alone.

### 1. Ticket latency

Calendar gap between creation (the stem's `YYMMDD` prefix) and `completed:`,
and the first-parent commit gap between the first commit that references the
stem and the commit that added the ticket under `.done/`.

```sh
for stem in $(window); do
  f=ai-docs/tickets/.done/$stem.md
  created=$(printf '%s' "$stem" | sed -E 's/^([0-9]{2})([0-9]{2})([0-9]{2}).*/20\1-\2-\3/')
  done_on=$(completed_date "$f")
  a=$(epoch "$done_on"); b=$(epoch "$created")
  days=$([ -n "$a" ] && [ -n "$b" ] && echo $(( (a - b) / 86400 )) || echo unavailable)
  first=$(git log "$BRANCH" --reverse --format=%h --grep="$stem" | head -1)
  close=$(git log "$BRANCH" --format=%h --diff-filter=A -- "$f" | tail -1)
  gap=$([ -n "$first" ] && [ -n "$close" ] && git rev-list --count --first-parent "$first..$close" || echo unavailable)
  printf '%s days=%s commits=%s\n' "$stem" "$days" "$gap"
done
```

Unit: days; first-parent commits. Limit: a ticket that sat in `idea/` for
weeks inflates days without any workflow cost; read days together with the
commit gap, and read neither as effort.

### 2. Commits per ticket, by type

```sh
for stem in $(window); do
  printf '%s ' "$stem"
  git log "$BRANCH" --format=%s --grep="$stem" \
    | sed -E 's/^([a-z]+)(\([^)]*\))?!?:.*/\1/' | sort | uniq -c | tr '\n' ' '
  echo
done
```

Unit: commits, per conventional-commit type prefix (`docs`, `feat`, `fix`,
`merge`, …). Limit: a ticket dominated by `docs` commits is distinguishable
from one dominated by `fix`; the count itself says nothing about size.

### 3. Corrective share

Among a ticket's commits after its first implementation commit, the share
that are corrective. Matching rule, applied verbatim: the first
implementation commit is the earliest stem-matching commit whose type prefix
is not `docs` and not `merge`; a later commit is corrective when its type
prefix is `fix`, or its subject starts with `revert` or `Revert`, or its
subject or body matches `review round`, `re-review`, `relay`, or
`fix cycle` (case-insensitive).

```sh
for stem in $(window); do
  first=$(git log "$BRANCH" --reverse --format='%h %s' --grep="$stem" \
    | grep -v -E '^[0-9a-f]+ (docs|merge)(\(|:)' | head -1 | cut -d' ' -f1)
  if [ -z "$first" ]; then printf '%s unavailable (no implementation commit)\n' "$stem"; continue; fi
  total=$(git log "$BRANCH" --format='%h' --grep="$stem" "$first..HEAD" | grep -c .)
  corrective=$(git log "$BRANCH" --format='%h' --grep="$stem" "$first..HEAD" | while read -r h; do
    git log -1 --format='%s%n%b' "$h" | grep -q -i -E '^(fix(\(|:)|revert)|review round|re-review|relay|fix cycle' && echo x
  done | grep -c x)
  printf '%s corrective=%s of %s\n' "$stem" "$corrective" "$total"
done
```

Unit: commits, as a fraction of post-implementation commits. Limit: a `fix`
commit is also how a ticket titled as a bug lands; read the share against
the ticket's category, and treat a window of few tickets as anecdote.

### 4. Escalations recorded in `### Result`

Phase Results whose text records a stop that reached the lead or the user.
Vocabulary matched, case-insensitive: `escalat`, `asked the user`,
`user decision`, `stopped for`, `blocked on`, `awaiting approval`.

```sh
for stem in $(window); do
  n=$(awk '/^### Result/{f=1; next} /^### /{f=0} f' "ai-docs/tickets/.done/$stem.md" \
    | grep -c -i -E 'escalat|asked the user|user decision|stopped for|blocked on|awaiting approval')
  printf '%s escalation_lines=%s\n' "$stem" "$n"
done
```

Unit: matching lines. Limit: a lower bound by construction; a stop nobody
wrote down is invisible, and a Result that discusses escalation in the
abstract counts. Read the matched lines before reporting the count.

### 5. Judgment items in `## AI Context`

Bullets in the window's commit bodies that record a choice among workable
alternatives, separated from bullets that restate what changed. The
separation is a judgment call: a bullet is a judgment item when it names an
alternative that was not taken, or a constraint that forced the choice; a
bullet that only narrates the diff is not. The run records how many bullets
it read and how many it classified as judgment, and quotes two borderline
cases with their classification.

```sh
for stem in $(window); do
  git log "$BRANCH" --format='%h%n%b' --grep="$stem" \
    | awk '/^[0-9a-f]{7,}$/{h=$0; next} /^## AI Context/{f=1; next} /^## /{f=0} f && /^- /{print h": "$0}'
done
```

Unit: bullets, classified by hand. Limit: the classification is the
measurer's; two runs by different people are comparable only if both quote
their borderline cases.

### 6. Aborted-run indicators

Counted over the whole tree at the measured commit, not per window ticket.

```sh
echo "blocked notes: $(grep -l '^## Blocked (' ai-docs/tickets/*/*.md ai-docs/tickets/.done/*.md 2>/dev/null | wc -l)"
echo "dropped phases: $(grep -l '\[dropped\]' ai-docs/tickets/.done/*.md ai-docs/tickets/.dropped/*.md 2>/dev/null | wc -l)"
n=0; for f in ai-docs/tickets/.dropped/*.md; do
  stem=$(basename "$f" .md)
  git log "$BRANCH" --format=%s --grep="$stem" | grep -q -v -E '^(docs|merge)(\(|:)' && n=$((n+1))
done; echo "dropped with implementation commits: $n"
echo "goal branches merged: $(git log "$BRANCH" --merges --format=%s | grep -c '^merge(goal)')"
echo "goal branches unmerged: $(git branch --list 'goal/*' | wc -l)"
```

Unit: counts. Limit: an unmerged `goal/*` branch may be in flight rather
than abandoned; list them and say which. A dropped ticket with
implementation commits is the strongest abort signal here and the rarest.

## Procedure

1. Check out the measured commit; record `git rev-parse HEAD`, the branch,
   and the date.
2. Run the shell setup and `window`; record the stems.
3. Run indicators 1 through 6; record each per-ticket value and the
   distribution, or `unavailable` with the reason.
4. Record every place a command's output did not match the shape described
   here, and every judgment call made in indicator 5.
5. For the after-run, place the before-run's record beside it. Partition
   the after-window into tickets closed after the before-run's commit and
   tickets it shares with the before-window, and report indicators 1
   through 5 for the new partition on its own as well as for the whole
   window: a window of 20 mostly shared tickets damps every movement, and
   the shared tickets ran under the old workflow. Compare per indicator,
   stating whether the window shapes match and whether the commit
   convention changed between the runs.

## What This Does Not Measure

Wall-clock time, token cost, the number of lead turns, the commit
convention itself, and the quality of the result. A run with fewer corrective commits may have been slower or
worse; a run with more escalations may have caught more. Read the figures as
evidence about the shape of the work, not as a verdict on it.

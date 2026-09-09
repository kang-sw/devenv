---
summary: Qualitative before/after measurement of workflow cost from git history and the ticket tree; read-only, every indicator states its command, project-neutral
---

# Workflow Cost Measurement

Measures what a workflow costs to run, from the history it already leaves:
the ticket tree, commit messages, and phase `### Result` sections. It is
applied once before and once after a workflow change, at two commits chosen in
advance, and the two runs are compared indicator by indicator. It produces no
score and no threshold; it produces figures with stated limits.

The before-commit is the one immediately preceding the change. The after-commit
is taken once the intended number of tickets has closed under the new workflow.
Record both choices and the reason for each: when most of the after-window is
still shared with the before-window, the partition in step 5 of the procedure
is the honest reading and the whole-window figures are damped.

## Rules

- **Read-only.** Every command below prints. No branch is created, no ticket
  moved, no history rewritten. Run against a checked-out commit.
- **One window shape for both halves.** Both runs use the same selector,
  size, and exclusions. A run whose window shape differs from its
  counterpart is not comparable; record that and do not compare.
- **Record the run.** Each run records the commit hash, branch, date, the
  window definition, the commit convention in force, the exact commands as
  run, every indicator's value or `unavailable`, and any place this manual's
  wording proved ambiguous. `## Record` below states where and in what shape.
- **Missing conventions are `unavailable`, never substituted.** A project
  whose commits carry no `## AI Context`, or whose tickets carry no phases,
  records those indicators as unavailable for that window. Do not invent a
  proxy; a proxy silently changes what the comparison compares. Every
  indicator below states its own unavailable condition, because an empty
  command output is not a zero.
- **Indicators, not a score.** Do not combine them. The sample is small and
  the window is confounded by model changes and topic mix.
- **Record the commit convention in force.** Who commits (lead, per-phase
  delegate, one worker per ticket) and at what granularity. Indicators 1, 2,
  3, and 6 all count commits, so a change in convention moves them without
  any change in abort or re-work; a comparison across conventions reports
  that first.

## Window

Default: the **20 most recently closed actionable tickets** on the measured
branch at the measured commit, where closed means present under
`ai-docs/tickets/.done/`, actionable means the stem's second dash-delimited
field — its category, the `<category>` of `YYMMDD-<category>-<name>` — is not
one of `epic`, `workset`, `research`, `idea`, or `design`, and recency is the
frontmatter `completed:` date, ties broken by stem. A project with fewer
than 20 records all of them and says so. Change the size only when both
halves change together.

Shell setup used by every command below, run from the repository root at
the measured commit. `BRANCH` is the branch the history is read on: the
project's tracked branch (`review-track` in `AGENTS.md` `### Review Policy`,
or the branch work lands on), set explicitly, never derived from `HEAD`,
because a run taken from a work branch would otherwise read that branch.
`TICKETS` is where the ticket files are read from, `SIZE` is the window size,
and `first_impl` is the shared implementation-commit selector that indicators 3
and 6 both use — defined once so the two indicators cannot drift apart.

```sh
BRANCH=develop                 # the tracked branch; set it, do not derive it
TICKETS=ai-docs/tickets        # working tree; see the note below when HEAD is elsewhere
SIZE=20
completed_date() { awk '/^---$/{c++; next} c==1 && /^completed:/{sub(/^completed:/,""); gsub(/[^0-9-]/,""); print; exit}' "$1"; }
epoch() { date -d "$1" +%s 2>/dev/null || date -j -f %Y-%m-%d "$1" +%s 2>/dev/null; }
first_impl() {   # $1 = stem; prints the first implementation commit, or nothing
  git log "$BRANCH" --reverse --format='%h %s' --grep="$1" | awk -v n="${1%%-*}" '
    { s = substr($0, index($0, " ") + 1) }
    s !~ /^(feat|fix|refactor|perf|test|build|ci|style|revert)[(:!]/ { next }
    { scope = ""
      if (match(s, /^[a-z]+\([^)]*\)/)) {
        scope = substr(s, RSTART, RLENGTH); sub(/^[a-z]+\(/, "", scope); sub(/\)$/, "", scope)
      }
      if (scope ~ /^[0-9][0-9][0-9][0-9][0-9][0-9]/ && substr(scope, 1, 6) != n) next
      print $1; exit }'
}
window() {
  for f in "$TICKETS"/.done/*.md; do
    [ -e "$f" ] || continue
    stem=$(basename "$f" .md)
    cat=$(printf '%s' "$stem" | cut -d- -f2)
    case "$cat" in epic|workset|research|idea|design) continue;; esac
    d=$(completed_date "$f")
    if [ -z "$d" ]; then echo "skipped (no completed:): $stem" >&2; continue; fi
    printf '%s %s\n' "$d" "$stem"
  done | sort | tail -"$SIZE" | awk '{print $2}'
}
```

The commit indicators read `$BRANCH`, so the ticket tree must be read at
`$BRANCH` too, or the two halves of one run disagree. When the checkout is
already on `$BRANCH`, the working tree is that tree and the default above is
right. When it is not — a run taken from a work branch — materialize the tree
read-only instead of switching branches, after the setup block above, and
record which form the run used:

```sh
TREE=$(mktemp -d); git archive "$BRANCH" ai-docs/tickets | tar -x -C "$TREE"
TICKETS=$TREE/ai-docs/tickets
```

`$TICKETS` is a filesystem path, used only to read ticket files. Git pathspecs
stay repo-relative (`ai-docs/tickets/...`) in every command below even when
`$TICKETS` points at a materialized copy; do not substitute `$TICKETS` into a
`git log -- <path>` argument, or the pathspec matches nothing and the row
reports `unavailable` for the wrong reason.

`window` prints the stems, oldest first, and names on stderr every closed
actionable ticket it skipped for lacking `completed:`
(`window 2>&1 >/dev/null | grep -c skipped` counts them). Record both with the
run, along with `window | wc -l`. A skipped ticket pulls an older one into the
window, so the window's span silently widens; if the two halves skip different
numbers, note that the two windows span different periods.

## Indicators

Each indicator states its command, its unit, its unavailable condition, and
what a movement in it does not license. Indicators 1, 3, and 4 produce one
value per stem: report the distribution (minimum, median, maximum, and the
per-ticket values), never the mean alone. Indicator 2 produces a per-type
count row per stem, indicator 5 produces window-wide bullet counts, and
indicator 6 produces whole-tree counts; none of those three has a
per-stem distribution.

Every indicator loops over `$STEMS`, captured once in step 2 of the procedure,
so all six measure the same window even if the tree changes mid-run.

### 1. Ticket latency

Calendar gap between creation (the stem's `YYMMDD` prefix) and `completed:`,
and the first-parent commit gap between the first commit that references the
stem and the commit that added the ticket under `.done/`.

```sh
for stem in $STEMS; do
  f=$TICKETS/.done/$stem.md
  created=$(printf '%s' "$stem" | sed -E 's/^([0-9]{2})([0-9]{2})([0-9]{2}).*/20\1-\2-\3/')
  done_on=$(completed_date "$f")
  a=$(epoch "$done_on"); b=$(epoch "$created")
  days=$([ -n "$a" ] && [ -n "$b" ] && echo $(( (a - b) / 86400 )) || echo unavailable)
  first=$(git log "$BRANCH" --reverse --format=%h --grep="$stem" | head -1)
  close=$(git log "$BRANCH" --format=%h --diff-filter=A -- "ai-docs/tickets/.done/$stem.md" | tail -1)
  gap=$([ -n "$first" ] && [ -n "$close" ] && git rev-list --count --first-parent "$first..$close" || echo unavailable)
  printf '%s days=%s commits=%s\n' "$stem" "$days" "$gap"
done
```

Unit: days; first-parent commits. Unavailable: `days=unavailable` when either
date is unparseable (`completed:` must be a bare `YYYY-MM-DD`);
`commits=unavailable` when no commit names the stem or the ticket file's
addition is not on `$BRANCH`. Limits: a ticket that sat in `idea/` for weeks
inflates days without any workflow cost, so read days together with the commit
gap and read neither as effort; and the commit gap is first-parent only while
indicators 2, 3, and 6 count every reachable stem-matching commit — the two
counts are different quantities and must not be compared to each other.

### 2. Commits per ticket, by type

A subject that carries no conventional-commit prefix is normalized rather
than printed whole: a `Merge ...` subject becomes `merge`, anything else
becomes `other`. Without that step one unprefixed subject is reported as if
it were a type and the row becomes unreadable.

```sh
for stem in $STEMS; do
  printf '%s ' "$stem"
  git log "$BRANCH" --format=%s --grep="$stem" | awk '
      /^[a-z]+(\([^)]*\))?!?:/ { sub(/[(!:].*/, ""); print; next }
      /^[Mm]erge /               { print "merge"; next }
                                 { print "other" }' \
    | sort | uniq -c | awk '{printf "%s=%s ", $2, $1}'
  echo
done
```

Each row reads `<stem> docs=6 fix=1 merge=2`. Unit: commits, per
conventional-commit type prefix, plus `other` for unprefixed subjects.
Unavailable: a project whose commits carry no type prefix at all reports the
indicator as unavailable rather than as one `other` column. Limits: a ticket
dominated by `docs` commits is distinguishable from one dominated by `fix`,
but the count itself says nothing about size; and `git log --grep=<stem>`
matches any commit whose message names the stem, including a commit for a
different ticket that lists this stem under `## Ticket Updates`, so a row can
carry commits the ticket did not cause.

### 3. Corrective share

Among a ticket's commits after its first implementation commit, the share
that are corrective.

Matching rule, applied verbatim, by `first_impl` in the setup block and the
`awk` below.

The **first implementation commit** is the earliest stem-matching commit whose
conventional-commit type is one of `feat`, `fix`, `refactor`, `perf`, `test`,
`build`, `ci`, `style`, `revert` — the types that change the product — *and*
whose scope, when that scope is a ticket stem, is this ticket's stem. Types
that administer the workflow rather than change the product (`docs`, `chore`,
`plan`, `ticket`, `merge`) are never the implementation commit: a
`chore(<stem>): promote ... to ready` or `chore(tickets): drop ...` is
lifecycle bookkeeping, and a `chore(<other-stem>)` commit that merely lists
this stem under `## Ticket Updates` belongs to a different ticket.

A later commit is **corrective** when its **subject line** — never the body —
is typed `fix` or `revert`. That is the whole automatic rule. Relay and
re-review evidence is counted by hand: the command prints every
post-implementation subject, the measurer marks the ones that are repairs, and
the run records how many were marked and on what reading.

The automatic rule is deliberately narrow because free-text matching cannot be
made safe here. A commit that references a ticket reproduces that ticket's own
words — the stem and title appear in `## Ticket Updates` and usually in the
subject — so matching `relay` or `review round` anywhere counts a ticket named
`...-review-relay` as corrective against every one of its own lifecycle
commits. Stripping the stem's words before matching does not rescue it either:
the strip is a substring operation, so a ticket whose stem contains `review`
also destroys `re-review` and `review round` in unrelated subjects and turns
false positives into silent false negatives. Typed `fix`/`revert` is the only
part a machine can get right; the rest is a read, and the manual says so
instead of pretending otherwise.

```sh
for stem in $STEMS; do
  first=$(first_impl "$stem")
  if [ -z "$first" ]; then printf '%s unavailable (no implementation commit)\n' "$stem"; continue; fi
  printf '%s anchor=%s %s\n' "$stem" "$first" "$(git log -1 --format=%s "$first")"
  total=$(git log --format='%h' --grep="$stem" "$first..$BRANCH" | grep -c .)
  if [ "$total" -eq 0 ]; then printf '  n/a (no post-implementation commits)\n'; continue; fi
  corrective=$(git log --format='%s' --grep="$stem" "$first..$BRANCH" \
    | grep -c -E '^(fix[(:!]|[Rr]evert[(:! ])')
  printf '  corrective=%s of %s\n' "$corrective" "$total"
  git log --format='    %h %s' --grep="$stem" "$first..$BRANCH"
done
```

Read the printed `anchor=` line for every stem and say whether that commit is
plausibly the ticket's first implementation; read the indented subject list and
mark any further repairs the typed rule missed. Both reads are part of the
indicator, not optional colour.

Unit: corrective commits out of post-implementation commits, reported as that
pair, not as a percentage. Unavailable: `unavailable (no implementation
commit)` when every stem-naming commit is `docs`/`merge`-typed or scoped to a
different ticket; `n/a (no post-implementation commits)` when the
implementation commit is the last stem-naming commit — an `n/a` row has no
share, is excluded from the distribution, and the count of such rows is
reported. Limits: a `fix` commit is also how a ticket titled as a bug lands, so
read the share against the ticket's category; a window of few tickets is
anecdote; and a project whose implementation commits do not name the stem —
because the work landed on a branch whose commits name it only in the merge —
fills up with `unavailable` rows. Count them and report them: a rise in that
count between the two halves is a change in the commit convention, not in
re-work.

The anchor carries the same weakness in a quieter form. When a ticket's real
first implementation commit does not name the stem, the selector does not
always fall through to `unavailable`: it anchors on whatever *later*
product-typed commit does name it — often a `fix` that is itself the repair,
which then becomes the anchor and is excluded from its own corrective count,
while the denominator shrinks to the tail of the ticket. Both readings
understate the ticket, and only the printed `anchor=` line makes the difference
visible, which is why reading it is required. The `unavailable` count is the
window's visible proxy for how often the convention hides implementation
commits; the wrong-anchor rows are the invisible remainder of the same gap.

### 4. Escalations recorded in `### Result`

Phase Results whose text records a stop that reached the lead or the user.
Vocabulary matched, case-insensitive: `escalat`, `asked the user`,
`user decision`, `stopped for`, `blocked on`, `awaiting approval`. Text under a
level-4 subsection inside a Result (for example `#### Edition (<short-hash>)`)
is read as part of that Result.

```sh
for stem in $STEMS; do
  f=$TICKETS/.done/$stem.md
  if ! grep -q '^### Result' "$f"; then
    printf '%s unavailable (no phase Result)\n' "$stem"; continue
  fi
  m=$(awk '/^### Result/{f=1; next} /^#{2,3} /{f=0} f' "$f" \
    | grep -i -E 'escalat|asked the user|user decision|stopped for|blocked on|awaiting approval')
  printf '%s escalation_lines=%s\n' "$stem" "$(printf '%s' "$m" | grep -c .)"
  [ -n "$m" ] && printf '%s\n' "$m" | sed 's/^/    /'
done
```

The matched lines are printed, not just counted, because the count on its own
is not reportable.

Unit: matching lines, after reading them. Unavailable: a ticket with no
`### Result` section records `unavailable`, never `0` — a project whose tickets
carry no phases has this indicator unavailable for the whole window.

Limits. This is a lower bound by construction: a stop nobody wrote down is
invisible. It is also an upper bound in the other direction, from the same
self-reference that breaks free-text matching in indicator 3 — a ticket whose
*deliverable* is escalation behaviour writes the vocabulary all over its own
Result while recording no stop at all, and there is no automatic mitigation for
that here. Report the number of lines you read and the number you judged to be
records of an actual stop, separately; the second number is the indicator, the
first is only where you started.

### 5. Judgment items in `## AI Context`

Bullets in the window's commit bodies that record a choice among workable
alternatives, separated from bullets that restate what changed. The
separation is a judgment call: a bullet is a judgment item when it names an
alternative that was not taken, or a constraint that forced the choice; a
bullet that only narrates the diff is not. The run records how many bullets
it read and how many it classified as judgment, and quotes up to two
borderline cases with their classification, stating when there were none.

```sh
for stem in $STEMS; do
  git log "$BRANCH" --format='@@%h%n%b' --grep="$stem" \
    | awk '
        function flush() { if (b != "") print h": "b; b="" }
        /^@@/            { flush(); h=substr($0,3); f=0; next }
        /^## AI Context/ { flush(); f=1; next }
        /^#/             { flush(); f=0; next }
        f && /^- /       { flush(); b=$0; next }
        f && /^[ \t]*$/  { flush(); next }
        f && b != ""     { sub(/^[ \t]+/, ""); b = b " " $0 }
        END              { flush() }'
done | awk '!seen[$0]++; END { if (NR == 0) print "unavailable (no ## AI Context in the window)" }'
```

Three details of that command are load-bearing. The `@@` prefix marks the
commit boundary so a bare hash quoted inside a bullet cannot be mistaken for
one. The `awk '!seen[$0]++'` deduplicates while preserving the order the
bullets were written in: one commit often names several window stems, and a
plain `sort -u` would both deduplicate and scramble the argument a commit's
`## AI Context` makes. The continuation branch rejoins wrapped bullets: commit
bodies are hard-wrapped, so a rule that emits only lines starting with `- `
truncates most bullets mid-sentence and the classification is then made on
half a sentence.

Unit: bullets, classified by hand. Unavailable: if no commit in the window
carries `## AI Context`, the indicator is unavailable for the window — empty
output is not zero judgment items. Limit: the classification is the measurer's;
two runs by different people are comparable only if both quote their borderline
cases.

### 6. Aborted-run indicators

Counted over the whole tree at the measured commit, not per window ticket.

This indicator reads four project conventions: a `## Blocked (<date>)` note
heading in ticket files, a `[dropped]` phase marker, `merge(goal)` merge
subjects, and a `goal/*` branch namespace for a multi-ticket run. A project
that does not use one of them records that line as `unavailable`, not zero;
check each convention exists before reading its count.

```sh
echo "blocked notes: $(grep -l '^## Blocked (' "$TICKETS"/*/*.md "$TICKETS"/.done/*.md "$TICKETS"/.dropped/*.md 2>/dev/null | wc -l)"
echo "dropped phases: $(grep -l '\[dropped\]' "$TICKETS"/*/*.md "$TICKETS"/.done/*.md "$TICKETS"/.dropped/*.md 2>/dev/null | wc -l)"
n=0; for f in "$TICKETS"/.dropped/*.md; do
  [ -e "$f" ] || continue
  stem=$(basename "$f" .md)
  h=$(first_impl "$stem")
  [ -n "$h" ] || continue
  n=$((n+1)); printf '  %s <- %s %s\n' "$stem" "$h" "$(git log -1 --format=%s "$h")"
done; echo "dropped with implementation commits: $n (read the rows above)"
echo "goal merges on $BRANCH: $(git log "$BRANCH" --merges --format=%s | grep -c '^merge(goal)')"
echo "goal branches not merged into $BRANCH: $(git branch --list --no-merged "$BRANCH" 'goal/*' | wc -l)"
git branch --list --no-merged "$BRANCH" 'goal/*'
```

Both `grep -l` lines cover every status directory: the visible ones plus
`.done/` and `.dropped/`, which the `*/` glob does not reach.

Unit: counts. Unavailable: any line whose convention the project does not use.
Limits: these counts are cumulative over the whole tree and grow with project
age, so compare the after-run against the before-run only as a difference
(after minus before), and state that the difference covers all work between the
two commits, not only the window's tickets. A branch that is not merged may be
in flight rather than abandoned; the listing is printed so the run can say
which.

The dropped-ticket loop reuses `first_impl` because a bare `--grep` counts a
stem merely listed under another ticket's `## Ticket Updates` and counts an
administrative `chore(tickets): drop ...` sweep once per ticket it closes. That
removes the sweeps but not the rest: a `fix`/`feat`/`refactor` commit that
implements something else while naming this stem as a follow-up, a
forward-dependency, or the reason the ticket is being dropped still selects.
Expect roughly a third of the rows to be that; the loop therefore prints each
stem with its selected subject, and the reported number is the count *after*
reading them. A dropped ticket with real implementation commits is the
strongest abort signal available here, which is exactly why it must not be
reported unread.

## Procedure

1. Check out the measured commit; record `git rev-parse HEAD`, the branch,
   the date, whether `TICKETS` is the working tree or a materialized
   read-only copy of `$BRANCH`, and the commit convention in force.
2. Run the shell setup, then capture the window once and record it:
   `STEMS=$(window)`, `window 2>&1 >/dev/null | grep -c skipped`, and
   `printf '%s\n' $STEMS | wc -l`.
3. Run indicators 1 through 5 over `$STEMS` and record each per-ticket value,
   plus the distribution for 1, 3, and 4, or `unavailable`/`n/a` with the
   reason. Run indicator 6 once for the tree and record its counts and the
   printed branch listing.
4. Record every place a command's output did not match the shape described
   here, and every judgment call made in indicator 5.
5. For the after-run, place the before-run's record beside it. Partition the
   after-window by stem — the new partition is the after-window's stems minus
   the before-window's stems (`comm -13 before-stems after-stems`, both
   sorted) — and report indicators 1 through 5 for the new partition on its own
   as well as for the whole window: a window mostly shared with the before-run
   damps every movement, and the shared tickets ran under the old workflow.
   Compare per indicator, stating whether the window shapes match and whether
   the commit convention changed between the runs.

## Record

Write each run wherever the project keeps durable records of a completed unit
of work — a ticket phase's `### Result`, a project note, or a file beside this
manual — one record per run, with these sections:

- **Run header** — commit hash, branch, date, `TICKETS` form, commit
  convention in force.
- **Window** — the stems, oldest first; the skipped count; the window size.
- **Indicators 1–6** — per-ticket rows, the distribution where one applies,
  and every `unavailable`/`n/a` row with its reason.
- **Ambiguities** — every place this manual's wording proved unclear during
  the run, and how the run resolved it.

A run is complete when every indicator has a value or a stated reason and the
window's stem list is recorded verbatim. Step 5 of the procedure compares two
such records.

## What This Does Not Measure

Wall-clock time, token cost, the number of lead turns, the commit
convention itself, and the quality of the result. A run with fewer corrective
commits may have been slower or worse; a run with more escalations may have
caught more. Read the figures as evidence about the shape of the work, not as
a verdict on it.

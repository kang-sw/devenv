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

- **Read-only.** No branch is created, no ticket moved, no history rewritten,
  no tracked file changed. The one thing a run writes is the scratch tree the
  setup block materializes under `mktemp -d`, which the run deletes when it
  ends. Every other command prints.
- **Pinned to a commit, not to a branch.** A branch name is a moving ref: a
  retroactive run that reads `develop` reads whatever `develop` means today,
  and then counts commits made after the change it is supposed to precede.
  Every command below reads `$COMMIT`, an immutable hash fixed before the run,
  and the ticket tree is materialized from that same hash. Record the branch
  the commit sat on, but never read history through it.
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
  command output is not a zero. Where a count of zero and an absent
  convention print the same integer, the run answers the convention question
  before it reads the integer — indicator 6 makes that answer an explicit
  input.
- **Indicators, not a score.** Do not combine them. The sample is small and
  the window is confounded by model changes and topic mix.
- **Record the commit convention in force.** Who commits (lead, per-phase
  delegate, one worker per ticket) and at what granularity. Indicators 1, 2,
  3, and 6 all count commits, so a change in convention moves them without
  any change in abort or re-work; a comparison across conventions reports
  that first.

## Window

Default: the **20 most recently closed actionable tickets** in the ticket tree
at the measured commit, where closed means present under
`ai-docs/tickets/.done/`, actionable means the stem's second dash-delimited
field — its category, the `<category>` of `YYMMDD-<category>-<name>` — is not
one of `epic`, `workset`, `research`, `idea`, or `design`, and recency is the
frontmatter `completed:` date, ties broken by stem. Change the size only when
both halves change together.

The window has four sizes that mean different things, and the run states
which it hit. The block below prints the verdict, so it is a command and not
a reading:

- **Zero stems** — the selector matched nothing: no `.done/` directory, no
  actionable stem in it, or no `completed:` frontmatter anywhere. The whole
  run is `unavailable (no closed actionable ticket window)`. No indicator is
  reported, and an empty run is never written up as a result: a project
  without these conventions is not measured by this manual, it is out of
  scope for it.
- **Fewer than `FLOOR` stems (default 5)** — below the execution floor. Run
  and record every indicator, mark the run `below floor (<n> of <FLOOR>)`,
  and do not perform the comparison step's indicator-by-indicator reading:
  place the two records side by side and read them as anecdote. One or two
  tickets move every figure in a five-ticket window.
- **Fewer than `SIZE` but at least `FLOOR`** — `short`: records all of them
  and says so. This is a smaller window, not a broken one; both halves must
  still use the same `SIZE` setting.
- **Exactly `SIZE`** — `full`: the normal case, the window the default
  describes.

Shell setup used by every command below, run from the repository root. The
checkout does not have to be at the measured commit and does not have to be on
the measured branch: the commands read `$COMMIT`, and the ticket tree is
materialized from `$COMMIT` too, so the working tree is never read and cannot
disagree with the history.

`COMMIT` is the measured commit, a hash. Take it from the environment: a
retroactive run — the usual shape for the second half of a comparison, and for
any before-run reconstructed after the change landed — exports the hash its
counterpart recorded, and the setup block then reads that and nothing else.
Only a run taken at the branch tip may let the block resolve the hash itself,
and that resolution is the act that fixes it: record the resolved value at once,
because it is the input the other half will need. The block derives `AT_TIP`
from the two, and indicator 6 reads it. `BRANCH` records which line that commit sat
on — it is a record field and the one input to indicator 6's branch listing,
never a revision the history is read through. Pick it as the branch the
project's work lands on; some projects declare a tracked branch in their
agent-context file, and if yours does, use that.

`TICKETS` is the materialized ticket tree, `SIZE` is the window size, `FLOOR`
is the execution floor above, and `first_impl` is the shared
implementation-commit selector that indicators 3 and 6 both use — defined once
so the two indicators cannot drift apart.

```sh
BRANCH=develop                 # record only; never read history through it
                               # retroactive run: COMMIT=<recorded hash> in the
                               # environment. Tip run: leave it unset, then
                               # record the hash this resolves to.
COMMIT=${COMMIT:-$(git rev-parse --verify "$BRANCH^{commit}")}
AT_TIP=no
[ "$COMMIT" = "$(git rev-parse --verify --quiet "$BRANCH^{commit}")" ] && AT_TIP=yes
SIZE=20
FLOOR=5

TREE=$(mktemp -d); git archive "$COMMIT" ai-docs/tickets | tar -x -C "$TREE"
TICKETS=$TREE/ai-docs/tickets
FPCHAIN=$TREE/first-parent-chain; git rev-list --first-parent "$COMMIT" > "$FPCHAIN"
# the teardown block after the Procedure removes $TREE; it is the run's last command

completed_date() { awk '/^---$/{c++; next} c==1 && /^completed:/{sub(/^completed:/,""); gsub(/[^0-9-]/,""); print; exit}' "$1"; }
epoch() {   # $1 = a bare YYYY-MM-DD; prints seconds, or nothing at all.
            # The shape test comes first: without it the BSD branch accepts
            # junk and answers with a value derived from the current clock.
  case "$1" in
    [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;;
    *) return 0;;
  esac
  date -d "$1" +%s 2>/dev/null || date -j -f '%Y-%m-%d' "$1" +%s 2>/dev/null || true
}
fp_pos() {   # $1 = commit; prints its position on $COMMIT's first-parent chain
  h=$(git rev-parse --verify --quiet "$1^{commit}") || return 0
  i=$(awk -v h="$h" '$0==h{print NR; exit}' "$FPCHAIN")
  if [ -z "$i" ]; then
    i=$(git rev-list --ancestry-path "$h..$COMMIT" \
        | awk 'NR==FNR{c[$0]=NR; next} ($0 in c) && c[$0]>m {m=c[$0]} END{if(m) print m}' "$FPCHAIN" -)
  fi
  printf '%s' "$i"
}
count_marker() {   # $1 = ERE; rest = files. Counts matching lines outside fenced blocks.
  pat=$1; shift
  if [ "$#" -eq 0 ]; then printf 'unavailable (no ticket files at the measured commit)\n'; return 0; fi
  awk -v pat="$pat" '
    FNR==1              { fence=0 }
    /^[ \t]*(```|~~~)/  { fence=!fence; next }
    !fence && $0 ~ pat  { n++ }
    END                 { print n+0 }' "$@"
}
first_impl() {   # $1 = stem; prints the first implementation commit, or nothing
  git log "$COMMIT" --reverse --format='%h %s' --grep="$1" | awk -v stem="$1" '
    { s = substr($0, index($0, " ") + 1) }
    s !~ /^(feat|fix|refactor|perf|test|build|ci|style|revert)[(:!]/ { next }
    { scope = ""
      if (match(s, /^[a-z]+\([^)]*\)/)) {
        scope = substr(s, RSTART, RLENGTH); sub(/^[a-z]+\(/, "", scope); sub(/\)$/, "", scope)
      }
      if (scope ~ /^[0-9][0-9][0-9][0-9][0-9][0-9]/) {
        if (substr(stem, 1, length(scope)) != scope) next
        if (length(scope) < length(stem) && substr(stem, length(scope) + 1, 1) != "-") next
      }
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

`fp_pos` is the one piece of machinery worth explaining. Indicator 1 wants the
distance between two commits *along the measured line*, and neither endpoint is
reliably on that line: work that landed through a merge sits on a side branch.
`git rev-list --count --first-parent A..B` does not give that distance — when
`A` is inside a merged branch the exclusion trims almost nothing and the count
becomes the distance back to where that branch forked, which is a fact about
merge topology and not about the ticket. `fp_pos` instead maps a commit to a
position on `$COMMIT`'s first-parent chain: its own position if it sits on the
chain, otherwise the position of the oldest chain commit that has it as an
ancestor — the point where it was integrated. Two positions subtract to a
distance that means the same thing for every row. Positions run oldest-largest,
so the subtraction is normally positive; it inverts when the ticket's `.done/`
addition was integrated onto the measured line *before* the first commit that
names the stem, which a long-lived side branch merged after the close can
produce. That row is reported `inverted`, not as a negative number, and is
excluded from the distribution.

`$TICKETS` is a filesystem path, used only to read ticket files. Git pathspecs
stay repo-relative (`ai-docs/tickets/...`) in every command below; do not
substitute `$TICKETS` into a `git log -- <path>` argument, or the pathspec
matches nothing and the row reports `unavailable` for the wrong reason.

`window` prints the stems, oldest first, and names on stderr every closed
actionable ticket it skipped for lacking `completed:`. Capture the window once
and let the block below name the size it hit:

```sh
STEMS=$(window)
SKIPPED=$(window 2>&1 >/dev/null | grep -c skipped)
N=$(printf '%s\n' "$STEMS" | grep -c .)
if   [ "$N" -eq 0 ];       then echo "window: unavailable (no closed actionable ticket window)"
elif [ "$N" -lt "$FLOOR" ]; then echo "window: below floor ($N of $FLOOR)"
elif [ "$N" -lt "$SIZE" ];  then echo "window: short ($N of $SIZE; all closures recorded)"
else                             echo "window: full ($N)"
fi
echo "skipped (no completed:): $SKIPPED"
if [ "$SKIPPED" -gt "$SIZE" ]; then
  echo "window unrepresentative (skipped $SKIPPED > SIZE $SIZE)"
fi
```

The skip count is a second axis, independent of the size: a skipped ticket
pulls an older one into the window, so the window's span silently widens, and
if the two halves skip different numbers the two windows span different
periods. Above `SIZE` the window is drawn from a minority of the project's
closures, and the honest reading compares the two halves over the calendar
period they share rather than over their whole spans. Record the size verdict,
the skip count, and the stems.

## Indicators

Each indicator states its command, its unit, its unavailable condition, and
what a movement in it does not license. Indicators 1 through 5 loop over
`$STEMS`, captured once in step 2 of the procedure, so those five measure the
same window even if the tree changes mid-run. Indicator 6 is not a window
indicator at all: it counts over the whole tree at the measured commit and is
compared only as a before/after difference.

What "the distribution" means differs by indicator, so each one names it:

- **Indicator 1** emits two values per stem and therefore two distributions:
  one over calendar days, one over the first-parent gap. Report minimum,
  median, maximum and the per-ticket values for each, separately; never the
  mean alone and never the two mixed.
- **Indicator 2** emits a per-type count row per stem. No distribution; report
  the rows and the window totals per type.
- **Indicator 3** emits a pair per stem, and a pair does not reduce to one
  number without becoming the percentage this indicator forbids. Report the
  per-ticket pairs and the window pair (corrective total of post-implementation
  total), plus the counts of `unavailable` and `n/a` rows.
- **Indicator 4** emits two numbers per stem: lines printed and lines judged to
  record an actual stop. The distribution is over the judged number; the
  printed number is reported beside it as the count the judgment started from.
- **Indicator 5** emits window-wide bullet counts. No distribution.
- **Indicator 6** emits whole-tree counts. No distribution.

### 1. Ticket latency

Calendar gap between creation (the stem's `YYMMDD` prefix) and `completed:`,
and the first-parent gap, along the measured line, between the first commit
that references the stem and the commit that added the ticket under `.done/`.

```sh
for stem in $STEMS; do
  f=$TICKETS/.done/$stem.md
  created=$(printf '%s' "$stem" | sed -E 's/^([0-9]{2})([0-9]{2})([0-9]{2}).*/20\1-\2-\3/')
  done_on=$(completed_date "$f")
  a=$(epoch "$done_on"); b=$(epoch "$created")
  days=$([ -n "$a" ] && [ -n "$b" ] && echo $(( (a - b) / 86400 )) || echo unavailable)
  first=$(git log "$COMMIT" --reverse --format=%h --grep="$stem" | head -1)
  close=$(git log "$COMMIT" --format=%h --diff-filter=A -- "ai-docs/tickets/.done/$stem.md" | tail -1)
  pf=$(fp_pos "$first"); pc=$(fp_pos "$close")
  if   [ -z "$pf" ] || [ -z "$pc" ]; then gap=unavailable
  elif [ "$pf" -lt "$pc" ];          then gap=inverted
  else                                    gap=$(( pf - pc ))
  fi
  printf '%s days=%s commits=%s\n' "$stem" "$days" "$gap"
done
```

Unit: days; first-parent commits along the measured line. Unavailable:
`days=unavailable` when either date is unparseable (`completed:` must be a bare
`YYYY-MM-DD`); `commits=unavailable` when no commit names the stem, when the
ticket file's addition is not reachable from `$COMMIT`, or when either endpoint
does not resolve to a position on the measured first-parent chain;
`commits=inverted` when the close integrated onto the measured line before the
first stem-naming commit did. An `inverted` row is excluded from the gap
distribution and counted separately, like an `unavailable` one.

Limits: a ticket that sat in `idea/` for weeks inflates days without any
workflow cost, so read days together with the commit gap and read neither as
effort. The commit gap is a first-parent distance while indicators 2, 3, and 6
count every reachable stem-matching commit — the two counts are different
quantities and must not be compared to each other. `first` is a bare
`--grep=<stem>` over every reachable commit, so it carries indicator 2's
selection limit: a commit for a *different* ticket that names this stem, in
`## Ticket Updates` or in prose, can be selected as the first reference, and
the row then starts earlier than the work did. And a positive gap does not
license reading the interval as this ticket's cost: the measured line carries
every other ticket's commits during the same interval.

### 2. Commits per ticket, by type

A subject that carries no conventional-commit prefix is normalized rather
than printed whole: a `Merge ...` subject becomes `merge`, anything else
becomes `other`. Without that step one unprefixed subject is reported as if
it were a type and the row becomes unreadable.

```sh
for stem in $STEMS; do
  row=$(git log "$COMMIT" --format=%s --grep="$stem" | awk '
      /^[a-z]+(\([^)]*\))?!?:/ { sub(/[(!:].*/, ""); print; next }
      /^[Mm]erge /               { print "merge"; next }
                                 { print "other" }' \
    | sort | uniq -c | awk '{printf "%s=%s ", $2, $1}')
  if [ -z "$row" ]; then
    printf '%s unavailable (no commit names the stem)\n' "$stem"
  else
    printf '%s %s\n' "$stem" "$row"
  fi
done
```

Each row reads `<stem> docs=6 fix=1 merge=2`. Unit: commits, per
conventional-commit type prefix, plus `other` for unprefixed subjects.
Unavailable: a stem no commit names reports `unavailable (no commit names the
stem)` rather than an empty row; a project whose commits carry no type prefix
at all reports the whole indicator as unavailable rather than as one `other`
column. Limits: a ticket dominated by `docs` commits is distinguishable from
one dominated by `fix`, but the count itself says nothing about size; and
`git log --grep=<stem>` matches any commit whose message names the stem,
including a commit for a different ticket that lists this stem under
`## Ticket Updates`, so a row can carry commits the ticket did not cause. A
window total is a sum of per-row counts, so a commit naming two stems is
counted twice; report the distinct-commit count beside it (indicator 5's
command prints it).

### 3. Corrective share

Among a ticket's commits after its first implementation commit, the share
that are corrective.

Matching rule, applied verbatim, by `first_impl` in the setup block and the
`awk` below.

The **first implementation commit** is the earliest stem-matching commit whose
conventional-commit type is one of `feat`, `fix`, `refactor`, `perf`, `test`,
`build`, `ci`, `style`, `revert` — the types that change the product — *and*
whose scope, when that scope looks like a ticket stem, belongs to this ticket.
Types that administer the workflow rather than change the product (`docs`,
`chore`, `plan`, `ticket`, `merge`, and any other type outside the product
list) are never the implementation commit: a `chore(<stem>): promote ... to
ready` or `chore(tickets): drop ...` is lifecycle bookkeeping, and a
`chore(<other-stem>)` commit that merely lists this stem under
`## Ticket Updates` belongs to a different ticket.

"Belongs to this ticket" is a prefix test on a stem boundary, not a date
comparison: a scope of `<stem>` matches that stem, a scope of just the
`YYMMDD` date matches every ticket created that day, and a scope naming a
*different* full stem with the same date does not match. The date-only scope
is a real convention and cannot be rejected, so its ambiguity is a stated
residual: on a day that created several tickets, a date-scoped commit is
credited to each of them, and the run says how many rows that affected.

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
  total=$(git log --format='%h' --grep="$stem" "$first..$COMMIT" | grep -c .)
  if [ "$total" -eq 0 ]; then printf '  n/a (no post-implementation commits)\n'; continue; fi
  corrective=$(git log --format='%s' --grep="$stem" "$first..$COMMIT" \
    | grep -c -E '^(fix[(:!]|[Rr]evert[(:! ])')
  printf '  corrective=%s of %s\n' "$corrective" "$total"
  git log --format='    %h %s' --grep="$stem" "$first..$COMMIT"
done
```

Read the printed `anchor=` line for every stem and say whether that commit is
plausibly the ticket's first implementation; read the indented subject list and
mark any further repairs the typed rule missed. Both reads are part of the
indicator, not optional colour.

Unit: corrective commits out of post-implementation commits, reported as that
pair, not as a percentage. Unavailable: `unavailable (no implementation
commit)` when every stem-naming commit carries a workflow-administration type
(`docs`, `chore`, `plan`, `ticket`, `merge`, or any other type outside the
product list) or is scoped to a different ticket; `n/a (no post-implementation
commits)` when the implementation commit is the last stem-naming commit — an
`n/a` row has no pair, is excluded from the window pair, and the count of such
rows is reported. Limits: a `fix` commit is also how a ticket titled as a bug
lands, so read the pair against the ticket's category; a window of few tickets
is anecdote; and a project whose implementation commits do not name the stem —
because the work landed on a branch whose commits name it only in the merge —
fills up with `unavailable` rows. Count them and report them: a rise in that
count between the two halves is a change in the commit convention, not in
re-work.

The anchor carries the same weakness in a quieter form. When a ticket's real
first implementation commit does not name the stem, the selector does not
always fall through to `unavailable`: it anchors on whatever *later*
product-typed commit does name it. That later commit is typically one of two
things, and the reading differs. It may be a second or third phase's
implementation, in which case the anchor sits well after the work began and the
denominator shrinks to the tail of the ticket. Or it may itself be a repair —
a `fix` that closes the phase it belongs to — in which case the anchor is a
corrective commit excluded from its own corrective count, and the row reports
zero repairs for a ticket whose visible history is a repair. Both readings
understate the ticket, and only the printed `anchor=` line makes the difference
visible, which is why reading it is required. Judge each anchor against the
ticket's phase count and against its own subject: an anchor whose subject
describes a late phase on a ticket with earlier phases, and an anchor typed
`fix` or `revert`, are both `unavailable` rows in disguise, and the second is
the stronger signal of the two. The `unavailable` count is the window's visible
proxy for how often the convention hides implementation commits; the
wrong-anchor rows are the invisible remainder of the same gap.

### 4. Escalations recorded in `### Result`

Phase Results whose text records a stop that reached the lead or the user.
Vocabulary matched, case-insensitive: `escalat`, `asked the user`,
`user decision`, `stopped for`, `blocked on`, `awaiting approval`. Text under a
level-4 subsection inside a Result (for example `#### Edition (<short-hash>)`)
is read as part of that Result, and any level-1 through level-3 heading ends
it.

```sh
for stem in $STEMS; do
  f=$TICKETS/.done/$stem.md
  if ! grep -q '^### Result' "$f"; then
    printf '%s unavailable (no phase Result)\n' "$stem"; continue
  fi
  m=$(awk '/^### Result/{f=1; next} /^(#|##|###) /{f=0} f' "$f" \
    | grep -i -E 'escalat|asked the user|user decision|stopped for|blocked on|awaiting approval')
  printf '%s escalation_lines=%s\n' "$stem" "$(printf '%s' "$m" | grep -c .)"
  if [ -n "$m" ]; then printf '%s\n' "$m" | sed 's/^/    /'; fi
done
```

The matched lines are printed, not just counted, because the count on its own
is not reportable. Report two numbers per stem: `escalation_lines` as printed,
and the number of those lines you judged to record an actual stop. The second
is the indicator.

Unit: matching lines, after reading them. Unavailable: a ticket with no
`### Result` section records `unavailable`, never `0` — a project whose tickets
carry no phases has this indicator unavailable for the whole window.

Limits. This is a lower bound by construction: a stop nobody wrote down is
invisible. It is also an upper bound in the other direction, from the same
self-reference that breaks free-text matching in indicator 3 — a ticket whose
*deliverable* is escalation behaviour writes the vocabulary all over its own
Result while recording no stop at all, and there is no automatic mitigation for
that here. A fall in the judged number does not license "fewer stops
happened": it is equally consistent with stops that stopped being written
down, which is why the printed number is reported beside it.

### 5. Judgment items in `## AI Context`

Bullets in the window's commit bodies that record a choice among workable
alternatives, separated from bullets that restate what changed. The
separation is a judgment call: a bullet is a judgment item when it names an
alternative that was not taken, or a constraint that forced the choice; a
bullet that only narrates the diff is not. The run records how many bullets
it read and how many it classified as judgment, and quotes up to two
borderline cases with their classification, stating when there were none.

```sh
commits=$(for stem in $STEMS; do git log "$COMMIT" --format=%H --grep="$stem"; done | sort -u | grep -c .)
bullets=$(for stem in $STEMS; do
  git log "$COMMIT" --format='@@commit@@%h%n%b' --grep="$stem" \
    | awk '
        function flush() { if (b != "") print h": "b; b="" }
        /^@@commit@@/    { flush(); h=substr($0,11); f=0; next }
        /^## AI Context/ { flush(); f=1; next }
        /^#/             { flush(); f=0; next }
        f && /^- /       { flush(); b=$0; next }
        f && /^[ \t]*$/  { flush(); next }
        f && b != ""     { sub(/^[ \t]+/, ""); b = b " " $0 }
        END              { flush() }'
done | awk '!seen[$0]++')
echo "distinct commits in the window: $commits"
if [ "$commits" -eq 0 ]; then echo "unavailable (no commit in the window names a stem)"
elif [ -z "$bullets" ]; then echo "unavailable (no ## AI Context in the window's commits)"
else printf '%s\n' "$bullets"
fi
```

Four details of that command are load-bearing. The `@@commit@@` prefix marks
the commit boundary; a plain `@@` would be, because commit bodies quote diff
hunks and a quoted `@@ -1,4 +1,4 @@` line would then split a bullet in half.
The `awk '!seen[$0]++'` deduplicates while preserving the order the bullets
arrive in: stem by stem in window order, and within one stem newest commit
first, because that is `git log`'s order and this command does not reverse it.
A plain `sort -u` would both deduplicate and scramble the argument a commit's
`## AI Context` makes. The continuation
branch rejoins wrapped bullets: commit bodies are hard-wrapped, so a rule that
emits only lines starting with `- ` truncates most bullets mid-sentence and the
classification is then made on half a sentence. That same branch folds an
indented sub-bullet into its parent, deliberately: a sub-bullet qualifies the
item above it and is not a second decision, so the unit counted is the
top-level bullet.

Unit: bullets, classified by hand. Unavailable: two different empty results,
distinguished by the printed commit count — `unavailable (no commit in the
window names a stem)` when the window's stems appear in no commit message at
all, and `unavailable (no ## AI Context in the window's commits)` when commits
exist but none carries the section. Empty output is not zero judgment items.

Limits: the classification is the measurer's, and two runs by different people
are comparable only if both quote their borderline cases — a movement between
two halves classified by different readers licenses nothing at all, and the
run must say who classified. Fixing the classifier does not make the count a
measure of decision quality either: a rise in judgment bullets is equally
consistent with better recording and with more churn to justify.

### 6. Aborted-run indicators

Counted over the whole tree at the measured commit, not per window ticket.

This indicator reads four project conventions: a `## Blocked` note heading in
ticket files, a `[dropped]` phase marker, `merge(goal)` merge subjects, and a
`goal/*` branch namespace for a multi-ticket run. A project that does not use
one of them records that line as `unavailable`, not zero.

A count of zero and an absent convention print the same integer, so the
question is answered before the counting, not from it: check each convention
against the project's ticket conventions and export the four flags below. They
carry no default, because a default is not an answer — a run that has not
checked prints `unavailable` four times, which is the truthful output for a
run that has not checked. A flag set to `yes` makes a printed zero a real zero.
Record the four answers and where you checked them.

```sh
# Answer these first, from the project's ticket conventions, and export them.
# Unset is the correct state until you have checked, and prints `unavailable`.
USES_BLOCKED=${USES_BLOCKED:-}              # tickets carry a `## Blocked` note heading
USES_DROPPED_PHASE=${USES_DROPPED_PHASE:-}  # tickets mark a dropped phase `[dropped]`
USES_GOAL_MERGE=${USES_GOAL_MERGE:-}        # run merges are typed `merge(goal)`
USES_GOAL_BRANCH=${USES_GOAL_BRANCH:-}      # multi-ticket work runs on `goal/*` branches

# Every ticket file that exists, in every status directory. The filter is not
# optional: an unmatched glob survives as a literal word in POSIX sh, and a
# nonexistent filename is fatal to awk. Built here, not in a function, because
# a function's positional parameters do not reach its caller.
set --
for f in "$TICKETS"/*/*.md "$TICKETS"/.done/*.md "$TICKETS"/.dropped/*.md; do
  if [ -f "$f" ]; then set -- "$@" "$f"; fi
done

if [ "$USES_BLOCKED" = yes ]; then
  echo "blocked notes: $(count_marker '^## Blocked([ (]|$)' "$@")"
else echo "blocked notes: unavailable (convention not used)"; fi
if [ "$USES_DROPPED_PHASE" = yes ]; then
  echo "dropped phases: $(count_marker '^#+ .*[[]dropped[]]' "$@")"
else echo "dropped phases: unavailable (convention not used)"; fi

n=0; for f in "$TICKETS"/.dropped/*.md; do
  [ -e "$f" ] || continue
  stem=$(basename "$f" .md)
  h=$(first_impl "$stem")
  [ -n "$h" ] || continue
  n=$((n+1)); printf '  %s <- %s %s\n' "$stem" "$h" "$(git log -1 --format=%s "$h")"
done; echo "dropped with implementation commits: $n (read the rows above)"

if [ "$USES_GOAL_MERGE" = yes ]; then
  landed=$(git log "$COMMIT" --first-parent --merges --format=%s | grep -c '^merge(goal)')
  reachable=$(git log "$COMMIT" --merges --format=%s | grep -c '^merge(goal)')
  echo "goal runs landed on the measured line: $landed"
  echo "other merge(goal) merges reachable: $((reachable - landed))"
else echo "goal merges: unavailable (convention not used)"; fi
if [ "$USES_GOAL_BRANCH" != yes ]; then
  echo "goal branches not merged: unavailable (convention not used)"
elif [ "$AT_TIP" != yes ]; then
  echo "goal branches not merged: unavailable (live ref set, run is retroactive)"
else
  echo "goal branches not merged into $COMMIT: $(git branch --list --no-merged "$COMMIT" 'goal/*' | grep -c .)"
  git branch --list --no-merged "$COMMIT" 'goal/*'
fi
```

The `set --` loop filters the three globs down to files that exist, because an
unmatched glob survives as a literal word in POSIX sh and a nonexistent
filename is a *fatal* error to awk — the count would abort before printing
anything, and a project that has dropped nothing yet has no `.dropped/`
directory. `count_marker` counts *lines*, not files, and skips fenced code
blocks. Both matter: a ticket carrying two `## Blocked` rounds is two aborts and one file,
and a ticket that documents the convention by showing it — a fenced
`## Blocked (YYYY-MM-DD)` template, a sentence containing `[dropped]` in
backticks — matches itself and inflates the count. The heading patterns are
anchored for the same reason: an unanchored `[dropped]` matches every mention
of the marker in prose, and in a workflow repository the manual describing the
marker is itself one of the false positives. The `## Blocked` pattern accepts a
bare heading as well as a dated one, because both are written.

The file list covers every status directory: the visible ones plus `.done/`
and `.dropped/`, which the `*/` glob does not reach. The fence skip has one
failure direction worth knowing: an unbalanced fence inside a ticket file
suppresses every match below it in *that* file — `FNR==1` re-zeroes the toggle
so the damage cannot spread to the next file, but the count is then a lower
bound. A run that wants the counts exactly checks fence parity per file first
and says what it found.

Unit: counts. Unavailable: any line whose convention the project does not use,
declared by the flags above.

Limits. These counts are cumulative over the whole tree and grow with project
age, so compare the after-run against the before-run only as a difference
(after minus before), and state that the difference covers all work between the
two commits, not only the window's tickets.

The two goal lines count different populations and must not be divided into
each other. `merge(goal)` is written both when a run's work lands and when a
running goal branch absorbs an update, and the subject does not say which; the
split above uses topology instead of wording, counting a merge as landed only
when it sits on the measured commit's first-parent chain. That is a lower
bound: a run whose landing merge was later re-parented, or which landed on
another line that was itself merged in, falls into the second count. Read the
second count as "merge traffic around goal branches", not as abandoned runs.
Neither line is a completion rate, because the branch listing is not history:
`git branch --list` reads the live local ref set, so it reports the branches
this checkout happens to have today, not the branches that existed at the
measured commit. `--no-merged "$COMMIT"` pins the merged-ness test, not the
ref set. A run taken at the tip of `$BRANCH` can record the listing as
observed; a retroactive run records it as `unavailable (live ref set, run is
retroactive)`, because a branch deleted or created since the measured commit is
invisible either way. A branch that is not merged may be in flight rather than
abandoned; the listing is printed so the run can say which.

The dropped-ticket loop reuses `first_impl` because a bare `--grep` counts a
stem merely listed under another ticket's `## Ticket Updates` and counts an
administrative `chore(tickets): drop ...` sweep once per ticket it closes. That
removes the sweeps but not the rest: a `fix`/`feat`/`refactor` commit that
implements something else while naming this stem as a follow-up, a
forward-dependency, or the reason the ticket is being dropped still selects,
and so does a `feat(ticket): <stem> — ...` commit that only creates the ticket.
Expect more than half the rows to be one of those; the loop therefore prints
each stem with its selected subject, and the reported number is the count
*after* reading them. Report both numbers — rows printed and rows judged real —
because the gap between them is a property of the commit convention, not noise.
A dropped ticket with real implementation commits is the strongest abort signal
available here, which is exactly why it must not be reported unread.

## Procedure

1. Fix the measured commit. A retroactive run exports the hash its counterpart
   recorded; a tip run leaves `COMMIT` unset and lets the setup block resolve
   it. Either way, record the resolved hash, the branch it sat on, the date of
   the run, the `AT_TIP` the block derived, and the commit convention in
   force. Nothing after this step reads a branch name as a revision.
2. Run the shell setup, then the window block. It prints the size verdict —
   `unavailable`, `below floor`, `short`, or `full` — and the skip count, and
   flags `window unrepresentative` when the skips exceed `SIZE`. Record all of
   it with the stem list. An `unavailable` verdict ends the run here.
3. Answer indicator 6's four convention questions against the project's ticket
   conventions, export its four flags, and record both the answers and where
   you checked.
4. Run indicators 1 through 5 over `$STEMS` and record each per-ticket value,
   plus the distribution each indicator defines, or `unavailable`/`n/a` with
   the reason. Run indicator 6 once for the tree and record its counts and the
   printed branch listing. Do the reads the indicators mandate: indicator 3's
   anchors and post-implementation subjects, indicator 4's matched lines,
   indicator 5's classification, indicator 6's dropped-ticket rows.
5. Record every place a command's output did not match the shape described
   here, and every judgment call made in indicators 3, 4, 5, and 6. Then tear
   the scratch tree down with the block below.
6. For the after-run, place the before-run's record beside it. Partition the
   after-window by stem — the new partition is the after-window's stems minus
   the before-window's stems, `comm -13 before-stems after-stems` with both
   files lexically sorted first, because `window` emits date order and `comm`
   requires `sort` order — and report indicators 1 through 5 for the new
   partition on its own as well as for the whole window: a window mostly
   shared with the before-run damps every movement, and the shared tickets ran
   under the old workflow. Compare per indicator, stating whether the window
   shapes match and whether the commit convention changed between the runs. A
   half marked `below floor` is not compared this way; a half marked
   `unavailable` is not compared at all.

Teardown, step 5's last action and the run's last command:

```sh
rm -rf "$TREE"
```

## Record

Write each run wherever the project keeps durable records of a completed unit
of work — a ticket phase's `### Result`, a project note, or a file beside this
manual — one record per run, with these sections:

- **Run header** — commit hash, the branch it sat on, date, tip or
  retroactive, commit convention in force, and why this commit was chosen as
  the before- or after-commit.
- **Commands as run** — the setup block's variable values (`BRANCH`,
  `COMMIT`, `SIZE`, `FLOOR`, the four indicator-6 flags), and every place the
  run departed from the commands as written here, quoted.
- **Window** — the stems, oldest first; the skipped count; the window size
  verdict the block printed (`unavailable`, `below floor`, `short`, or
  `full`); and whether the skips tripped `window unrepresentative`.
- **Indicators 1–6** — per-ticket rows, the distribution each indicator
  defines, the count of `unavailable` rows and of `n/a` rows per indicator
  with their reasons, and for the indicators that mandate a read, both
  numbers: printed and judged.
- **Judgments** — who did indicator 5's classification and under what
  protocol; indicator 3's anchor verdicts and how many of its rows were
  selected through a date-only scope that several same-day tickets share;
  indicator 4's judged stops; indicator 6's real-versus-printed dropped rows.
- **Ambiguities** — every place this manual's wording proved unclear during
  the run, and how the run resolved it.

A run is complete when the window's stem list is recorded verbatim, every
indicator has a value or a stated reason, and every mandated read has a
recorded judgment. A record whose indicators are present but whose judged
numbers are missing is an incomplete run, not a fast one: the printed numbers
alone are the input to the reads, not the result. Step 6 of the procedure
compares two such records.

## What This Does Not Measure

Wall-clock time, token cost, the number of lead turns, the commit
convention itself, and the quality of the result. A run with fewer corrective
commits may have been slower or worse; a run with more escalations may have
caught more. Read the figures as evidence about the shape of the work, not as
a verdict on it.

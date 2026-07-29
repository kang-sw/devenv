# Plan: 260727-chore-merge-ws-dashboard-dev-into-goal-branch — Phase 4: restore `_index.md` inventory parity in its own commit

## Relevant Ticket Contract

- Add the missing rows to the `## Tickets` table in `ai-docs/_index.md`.
  Source each row's Stem/Status/Summary from that ticket's own frontmatter and
  its status directory (Status = the directory it lives in).
- Do NOT edit any ticket body.
- Do NOT change any of the existing rows.
- Separate commit from Phase 2's merge and from Phase 3's fix, on purpose (own
  commit message, own attribution).
- Verification boundary (ticket text): the file count and row count agree,
  every new row points at a file that exists, no existing row moved.

**Stale numbers corrected.** The ticket's Phase 4 body and its "Pre-existing
conditions" section both say "128 files / 107 rows / 21 missing", measured
against the pre-Phase-2 preview tree. Re-measured on this branch (current HEAD,
post Phase 2/3 merges) below: **131 files / 108 rows / 23 missing.** The plan
below uses the re-measured 23, not the ticket's 21.

## Out of Scope

- `## Ticket Focus` (a different section; Decisions already resolved its merge
  conflict in Phase 2 and forbids restoring two stale bullets — not this
  phase's concern).
- Any section of `_index.md` other than the `## Tickets` table.
- Ticket body edits, re-triage, or status-directory moves for any of the 23
  tickets — they stay exactly where they are; only a new row is added.
- Re-sorting or touching any of the 108 existing rows, including the two
  pre-existing `ready`-status anomalies described below.
- Phase 5 (routing the inherited spec-debt note) — separate phase, not touched
  here.

## Codebase Findings

### Measurement (re-derived; commands are the verification-command set too)

```
ls ai-docs/tickets/{ready,todo,idea} | grep -c '\.md$'
# -> 131

sed -n '/^## Tickets$/,/^## Ticket Focus$/p' ai-docs/_index.md | grep -c '^| '
# -> 109 (108 data rows + 1 header row `| Stem | Status | Summary |`;
#    the separator line `|------|--------|---------|` does NOT match `^| `
#    because its second character is `-`, not a space, so it is already
#    excluded — no manual −1 needed beyond the header)

# file/table diff, both directions:
sed -n '/^## Tickets$/,/^## Ticket Focus$/p' ai-docs/_index.md \
  | grep '^| `' | sed -E 's/^\| `([^`]+)`.*/\1/' | sort -u > /tmp/table_stems.txt
find ai-docs/tickets/ready ai-docs/tickets/todo ai-docs/tickets/idea \
  -maxdepth 1 -name '*.md' -printf '%f\n' | sed 's/\.md$//' | sort -u > /tmp/file_stems.txt
comm -23 /tmp/file_stems.txt /tmp/table_stems.txt   # -> 23 stems, listed below
comm -13 /tmp/file_stems.txt /tmp/table_stems.txt   # -> empty (no dangling row today)
```

Gap = 131 − 108 = **23** (not the ticket's stale 21). All 23 are dev-side
tickets filed by Phases 2/3's own commits or pre-existing dev backlog; none
overlap the 92 (now updated) stems the merge already reconciled.

### `ai-docs/_index.md` table structure (`ai-docs/_index.md#L168-L285`)

- `L168` `## Tickets` section header, `L174` column header
  `| Stem | Status | Summary |`, `L175` separator, `L176-L283` = 108 data
  rows, `L284` blank, `L285` `## Ticket Focus` (next section).
- No frontmatter field feeds Summary other than `title` — confirmed by reading
  5+ ticket files across all three status dirs; the only common frontmatter
  keys are `title`, `related`, `related-mental-model`, `sage-review-design`,
  `spec`, `parent`. There is no `description`/`summary` field anywhere in this
  ticket system, so `title` is always the fallback with nothing else to try.

### Row ordering rule (inferred, with evidence)

The table is **status-grouped in the order `ready → todo → idea`, each group
internally sorted ascending by stem** (stems begin with a `YYMMDD`-like
prefix, so this is chronological-then-alphabetical). This holds exactly for
106 of the 108 existing rows:
- `L177-L221`: 45 rows, status `todo`, strictly ascending stem
  (`260513-...` → `260725-bug-dashboard-terminal-create-failure-silent`).
- `L223-L283`: 61 rows, status `idea`, strictly ascending stem
  (`260512-...` → `260727-chore-dashboard-e2e-helper-modules-never-type-checked`,
  the table's last row).

Two rows are `ready`-status exceptions to a clean 3-block model, and neither
should be treated as the pattern to extend:
- `L176` (`260726-chore-dashboard-verify-notification-permission-tier-manually`,
  `ready`) sits alone, first in the file.
- `L222` (`260727-chore-merge-ws-dashboard-dev-into-goal-branch`, `ready` — this
  very ticket) sits at the seam between the last `todo` row and the first
  `idea` row, i.e. it was appended where a chronologically-later row would
  land relative to its neighbors, not moved next to the other `ready` row.

Neither anomaly is disturbed by this phase (existing rows do not move — see
Verification Plan). Since exactly one of the 23 new rows is itself
`ready`-status (`260725-feat-dashboard-terminal-steady-state-stream-throughput`,
stem `260725` < `260726`), the safest option that extends an existing pattern
rather than inventing a third one is: **insert it immediately before the
existing lone `ready` row at `L176`**, forming a clean, ascending 2-row
`ready` block at the very top of the table. This does not touch the `L222`
anomaly and does not require deciding a policy for a case with no precedent.

For the 22 `todo`/`idea` rows, insert each at the position that keeps its
block ascending by stem — i.e. ordinary insertion-sort into the existing
contiguous run. Full resolved positions (evidence: `comm` diff above, cross-
checked by hand-merging into the existing sorted stem lists):

**New `ready` row (1), insert immediately above `L176`:**
- `260725-feat-dashboard-terminal-steady-state-stream-throughput`
  (`ai-docs/tickets/ready/260725-feat-dashboard-terminal-steady-state-stream-throughput.md`)

**New `todo` rows (10) — all insert as one contiguous block immediately after
the current last `todo` row (`260725-bug-dashboard-terminal-create-failure-silent`,
currently `L221`) and before the `L222` `ready` anomaly, in this order:**
1. `260725-epic-ws-dashboard-git-panel`
2. `260725-feat-workspace-workroot-alias`
3. `260725-feat-ws-dashboard-design-guide`
4. `260725-feat-ws-dashboard-git-diff-view`
5. `260725-feat-ws-dashboard-git-review-comments`
6. `260725-feat-ws-dashboard-git-tab-log-graph`
7. `260725-feat-xterm-ligatures`
8. `260725-refactor-unwire-agents-activity-badge`
9. `260726-refactor-dashboard-worktree-git-spawns-through-exec-seam`
10. `260726-refactor-ws-dashboard-long-uptime-leak-hardening`

(All ten happen to sort after every existing `todo` row, so no interleaving
with existing rows is needed — verify this by ascending-sort comparison, not
by assumption, since `260725-refactor-...` must still be checked against any
existing `260725-refactor-*` — there is none.)

**New `idea` rows (12):**

Interleaved among the existing 9 `260725`-stem idea rows (existing rows shown
for anchoring, not to be changed):
1. *(existing)* `260725-bug-agent-synthetic-load-cleanup-guard`
2. *(existing)* `260725-bug-dashboard-routes-test-terminal-helper-leak-no-reaper`
3. *(existing)* `260725-bug-dashboard-terminal-lifetime-load-fragility`
4. *(existing)* `260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`
5. *(existing)* `260725-bug-dashboard-terminal-socket-path-length-unguarded`
6. **NEW** `260725-bug-dashboard-terminal-utf8-residual-multibyte-corruption`
7. *(existing)* `260725-bug-dashboard-workroot-id-unstable-when-path-canonicalize-fails`
8. **NEW** `260725-feat-dashboard-graceful-shutdown-from-settings`
9. **NEW** `260725-feat-prefs-portability`
10. *(existing)* `260725-idea-ws-git-commit-rename-and-payload-rejections`
11. **NEW** `260725-perf-dashboard-daemon-workroot-fanout-concurrency`
12. *(existing)* `260725-refactor-dashboard-agent-gui-physical-module-isolation`
13. **NEW** `260725-research-dashboard-terminal-serverside-screen-emulation-diff-transport`
14. *(existing)* `260725-research-ws-dashboard-pty-agent-pivot`

Then, as one new contiguous `260726` block between the last `260725` idea row
above and the first existing `260727` idea row
(`260727-bug-dashboard-notification-toggle-enabled-without-api`):
15. **NEW** `260726-bug-dashboard-git-watch-probe-cache-evict-and-foreign-mount-gaps`
16. **NEW** `260726-bug-dashboard-opened-workroots-mixed-path-separators`
17. **NEW** `260726-bug-lead-implement-lost-review-relay-cycle-cap`
18. **NEW** `260726-idea-dashboard-moved-workroot-red-with-no-recovery-affordance`
19. **NEW** `260726-idea-dashboard-resources-poll-eagerly-prunes-unavailable-work-roots`

Then, appended at the very end of the table (after the current last row
`260727-chore-dashboard-e2e-helper-modules-never-type-checked`, before the
blank line preceding `## Ticket Focus`), since no existing `260728` idea row
exists to interleave with:
20. **NEW** `260728-bug-dashboard-acceptance-xterm-rows-assertions-blind-under-webgl-renderer`
21. **NEW** `260728-bug-dashboard-terminal-eviction-leaks-callback-token`

(20/21 ordered relative to each other by stem: `acceptance` < `terminal`.)

**Note on "no existing row moved."** Inserting rows necessarily shifts line
numbers below the insertion point. Read the constraint as *no existing row's
content or relative order among other rows changes* — verified mechanically
below via a diff that shows only added (`+`) lines and zero removed (`-`)
content lines.

### Summary-column sourcing rule

Sampled across all three status dirs (`ai-docs/tickets/todo/260513-feat-runtime-binary-staging-copy.md`,
`.../260524-epic-async-exec-job-surface.md`,
`.../260710-bug-project-index-ticket-focus-stale-status.md`,
`ai-docs/tickets/idea/260710-idea-dashboard-open-work-root-full-registry-redundant-rediscovery.md`)
against their current table rows:

1. **Source field: `title` only.** No ticket in this system carries a
   `description`/`summary` frontmatter field — `title` is the sole source,
   always, with no fallback branch to design for.
2. **Register, by title shape:**
   - If `title` already reads as a full descriptive clause/sentence (common
     for `bug`/`research` tickets — e.g. the multi-line quoted YAML titles on
     `260726-bug-dashboard-git-watch-probe-cache-evict-and-foreign-mount-gaps`,
     `260726-idea-dashboard-moved-workroot-red-with-no-recovery-affordance`,
     `260728-bug-dashboard-acceptance-xterm-rows-assertions-blind-under-webgl-renderer`,
     `260728-bug-dashboard-terminal-eviction-leaks-callback-token`): use it
     near-verbatim, collapsed to one line, quotes stripped.
   - If `title` is a short noun-phrase/heading (common for `feat`/`epic`/
     `refactor` tickets — e.g. "Runtime binary staging copy" →
     "Stage runtime binaries under deterministic versioned paths"; "Async exec
     job surface" → "Coordinate async exec job tools, bounded output readers,
     and later model-backed output questions"): rewrite as one
     imperative/descriptive clause carrying the same noun-phrase's action.
     Pull in one qualifying detail from the ticket's opening `## Background`/
     `## Scope` paragraph only when the title alone would be ambiguous out of
     context (e.g. the `260725-feat-ws-dashboard-git-*` family are UI-1/UI-2/
     UI-3 siblings under the `260725-epic-ws-dashboard-git-panel` parent — say
     which slice each one is).
3. **Length/format:** one sentence or clause, no trailing period (every
   sampled row omits it), backtick file paths/identifiers/commands exactly as
   the surrounding rows do.
4. **Blocked/deferred state:** if the ticket carries a `## Blocked` section,
   an explicit deferral, or other status worth flagging, append a second
   clause after a semicolon — mirroring
   `260726-chore-dashboard-verify-notification-permission-tier-manually`
   ("...now carries a `## Blocked` note — only the human-only permission-
   prompt / OS-banner residue remains...") and
   `260710-bug-project-index-ticket-focus-stale-status` ("...mechanical
   reconciliation done, recurrence-prevention mechanism still
   sage-design-blocked"). Otherwise keep it to one clause — do not manufacture
   a second clause where the ticket has no such state. Check each of the 23
   tickets for a `## Blocked` heading or similar before writing its summary.

## Implementation Plan

1. Read each of the 23 ticket files' frontmatter `title` (and, for the
   `feat`/`epic`/`refactor` ones, the opening `## Background`/`## Scope`
   paragraph and any `## Blocked` section) to compose its Summary per the rule
   above.
2. Edit `ai-docs/_index.md`'s `## Tickets` table only, inserting the 23 new
   rows at the exact positions enumerated in Codebase Findings (1 `ready` row
   above `L176`; 10 `todo` rows as one block after the current last `todo`
   row; 12 `idea` rows interleaved/appended per the three sub-groups above).
   Match the existing row format exactly:
   `` | `<stem>` | <status> | <summary> | `` — backtick the stem, lowercase
   status word matching its directory name (`ready`/`todo`/`idea`).
3. Do not touch any other line in `ai-docs/_index.md` (no edits to
   `## Ticket Focus`, no edits to the 108 existing table rows, no reordering
   of existing rows).
4. Run the Verification Plan below before committing.
5. Commit as its own commit, separate from Phases 2/3, per the ticket's
   Decisions (splitting the merge/fix/inventory commits for clean
   attribution).

## Verification Plan

```
# 1. File count == new row count agreement
ls ai-docs/tickets/{ready,todo,idea} | grep -c '\.md$'
# expect 131 (unchanged by this phase — no files added/moved)

sed -n '/^## Tickets$/,/^## Ticket Focus$/p' ai-docs/_index.md | grep -c '^| '
# expect 132 (109 + 23 new rows; still includes the 1 header line)
# -> 131 data rows, matching the 131 files: gap closed to 0

# 2. Every new row points at a file that exists
sed -n '/^## Tickets$/,/^## Ticket Focus$/p' ai-docs/_index.md \
  | grep '^| `' | sed -E 's/^\| `([^`]+)`.*/\1/' | sort -u > /tmp/table_stems_after.txt
find ai-docs/tickets/ready ai-docs/tickets/todo ai-docs/tickets/idea \
  -maxdepth 1 -name '*.md' -printf '%f\n' | sed 's/\.md$//' | sort -u > /tmp/file_stems_after.txt
diff /tmp/table_stems_after.txt /tmp/file_stems_after.txt
# expect: no output (perfect set equality both directions)

# 3. No existing row changed/moved — additive-only diff shape
git diff --stat -- ai-docs/_index.md
# expect: exactly 1 file changed, insertions == 23, deletions == 0

git diff -U0 -- ai-docs/_index.md | grep '^-' | grep -v '^---'
# expect: no output (zero removed content lines — proves no existing row was
# edited or reordered, only new lines added)

# 4. Scope fence — nothing else touched
git status --short
# expect: only ai-docs/_index.md modified; no ticket files under
# ai-docs/tickets/ appear as modified/moved
```

## Escalations

- None.

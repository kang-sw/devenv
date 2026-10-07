---
title: Pi lead compaction milestones as wake turns, and structured re-read lists in ws-compact
related:
  261006-feat-pi-compaction-advisory-standing-intent-milestones: predecessor; revises its milestone delivery and milestone prose, and the advisory's closing sentence
  261002-feat-pi-lead-ws-owned-compaction: predecessor; narrows its "no file lists" rule to historical lists
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: b8e9759a42e8850a
completed: 2026-10-07
---

# Pi lead compaction milestones as wake turns, and structured re-read lists in ws-compact

## Background

Two problems surfaced while dogfooding 261006 in the Pi adapter
(`agents-plugin-pi`).

**Milestones never lead to compaction.** 261006 delivers the two interim
milestones (60% and 70% at the defaults) as a `steer` at a `turn_end` that
has tool results (`fireCompactionTriggers` in `src/goal-loop.ts`), so a
milestone only ever arrives mid-run, and its text
(`buildContextMilestoneMessage` in `src/lead-compaction.ts`) ends with "do
not stop the current task for it". The lead correctly reads past it every
time, and in practice the session runs into the hard point and is
force-steered into preparation mid-work. A steer also mixes noise into a run
that is going well, whereas a dedicated wake turn at a run boundary makes the
lead weigh the compaction question on its own, as the advisory already does.

**The summary has no explicit "read this next" slot.** The lead's carry-forward
is prose only, and the guide says "list no files". When the next step needs a
specific part of a specific file, that need is buried in **Current work**
prose: the revived lead either misses it or re-reads broadly to rebuild
context. 261002's "no file lists" rule targeted Pi's automatically
accumulated read/modified file lists, which grew without bound; a short,
lead-curated, forward-looking list is a different thing.

## Decisions

- **Milestones are delivered as a wake turn at `agent_end`.** A milestone is
  sent at `agent_end` only, as `deliverAs: "followUp"` with
  `triggerTurn: true`, the same boundary and mode as the advisory (261004 D2,
  261006). It is no longer sent at any `turn_end`. Rejected: keeping the
  tool-turn `steer` (arrives only mid-work, where the right answer is always
  to ignore it; observed in dogfooding to never trigger compaction).
- **A milestone wake turn sets the `preparation` flag.** Like the advisory,
  the flag is set when the milestone is sent and cleared at the next
  `agent_end`, so no other trigger fires inside the milestone turn. This
  reverses 261006's "Milestones are not preparation messages ... never set
  the `preparation` flag", which held only while a milestone rode inside a
  running task; a dedicated wake turn is a preparation boundary like the
  advisory's.
- **The milestone has its own send and its own route.** It is sent with its
  `ws-lead-context-milestone` customType and no guide body, not through
  `sendPreparation`; the milestone prose points at the advisory for the
  preparation. A `ws-compact` call made from a milestone wake turn takes a
  new route `milestone` that behaves like `advisory`: no goal-less resume
  unless the lead sets `continue_after_compact`. Rejected: treating it as
  `autonomous` (always resume); the wake comes at `agent_end`, where the next
  move is usually the user's, and `continue_after_compact` already covers
  known remaining work.
- **Milestone prose strengthens per milestone, without exceptions or
  disclaimers.** The two milestones get distinct texts (pinned below): the
  first says compaction is forced past the hard point and that a good
  boundary is worth taking from here; the second says it is the last reading
  before the forced point and to take this boundary if it fits. Both refer to
  the advisory for the preparation and for what a good boundary is. The
  safe-boundary rule (no autonomous advisory compaction during active human
  discussion or while a human answer is awaited, 261004 D1) stays stated only
  in the advisory and the guide; the milestone texts neither restate nor relax
  it. Rejected: a final milestone that allows compaction while a human answer
  is awaited (the user chose to strengthen only the tone, with no exception
  clause and no over-negation).
- **The advisory's closing sentence names the milestones.** "Brief context
  readings will follow on the way there." is replaced by the pinned sentence
  below, which states the milestone points.
- **`ws-compact` takes two structured file lists.** `required_rereads`
  (must-read) and `references` (opened on demand), each an array of
  `{ path, lines?, why }`. Neither has a count limit; the schema descriptions
  tell the lead to list only what the work needs. Rejected: two more prose
  headings in `LEAD_PROSE_SECTIONS` (format held only by the prompt; no
  validation).
- **Required re-reads are inlined within a byte budget, smallest first.** At
  compaction the adapter reads each `required_rereads` entry (its `lines`
  range, or the whole file when `lines` is omitted) and measures its UTF-8
  size. Entries are taken smallest first, ties in the lead's order, while the
  running total stays within the budget; a taken entry is inlined into the
  summary in full. An entry that does not fit is never cut: it goes, whole,
  to a list the revived lead reads first. The summary therefore has three
  tiers: inlined must-reads, the must-read overflow list, and the
  references. The budget defaults to 40960 bytes, the same as the dialog
  budget. There is no per-file cap: taking the smallest first already keeps
  one large file from crowding out the rest. Rejected: listing only, with
  the revived lead reading every entry (costs a tool turn and can be
  skipped); cutting an entry to fit; a per-file cap.
- **`lines` is a strict line range.** `"<start>-<end>"`, 1-based and
  inclusive, clamped to the file's end; omitted means the whole file. A
  symbol or heading anchor goes in `why`. The adapter must be able to cut the
  range, and the lead's earlier reads carry line numbers to quote.
- **Budget key, parsing, and rendering.** The inline budget is a new config
  key `compaction_reread_budget_bytes` (default 40960), registered in
  `agents-plugin-pi/config-manifest.json` and resolved like
  `compaction_dialog_budget_bytes`. An entry whose `lines` does not parse as
  `start-end`, or whose `start` is past the file's last line, is never
  inlined and goes to the to-read-first list with its `lines` text as
  written. Selection is by size, but each section renders
  in the lead's order. An inlined file sits in a fence longer than any
  backtick run in its content. No padding lines are added around a range:
  padding would spend budget on every entry to cover the rare file edited
  after it was read, and the inlined header shows the exact lines taken.
  Rejected: padding each range by a fixed number of lines.
- **Missing paths are marked, never rejected.** An entry whose path does not
  exist or cannot be read is kept and rendered with a `(not found)` marker,
  never inlined. Rejected: refusing the lever call (a forced compaction at
  the hard point must not fail, and a retry near the context limit is
  costly).
- **The fallback summary leaves both lists empty.** The in-hook fallback
  summarizer (`buildFallbackSummaryPrompt`) does not produce them.
- **"No file lists" narrows to historical lists.** The guide, the fallback
  prompt's "Do not list files that were read or modified", and the summary
  builder's no-file-list contract keep excluding lists of files that were
  read or modified; the curated lists are the one place files are named. The
  Resume section tells the revived lead to read the overflow list first and
  open a reference or any other file only when a step needs it.

### Pinned prose

Implemented verbatim with placeholders interpolated; prompt tests pin each
text verbatim, as 261006 did.

Milestone 1 (`{n}` the rounded current usage, `{hard}` the resolved hard
threshold):

```text
Context window: {n}% / {hard}% (forced compaction point). Past {hard}%, compaction is forced, mid-work if need be. From here on, a good boundary is worth taking; if this is one, run the preparation from the advisory. Otherwise end this turn without replying.
```

Milestone 2:

```text
Context window: {n}% / {hard}% (forced compaction point). This is the last reading before compaction is forced. Compacting at a boundary you choose keeps the summary in your hands; take this one if it fits, and run the preparation from the advisory. Otherwise end this turn without replying, and take the next good boundary.
```

Advisory closing sentence, replacing "Brief context readings will follow on
the way there." at the end of the advisory head's third paragraph
(`{m1}` and `{m2}` the two milestone thresholds, rounded):

```text
Reminders follow at {m1}% and {m2}%.
```

Guide step 2 (`agents-plugin-pi/lead-compact-guide.md`): the clause "and
list no files" becomes:

```text
and keep file names out of the prose; the lever's file lists below are where files go
```

and a third sub-bullet is appended under step 2:

```text
   - **File lists.** `required_rereads` holds only the files the immediate
     next step needs, each with `lines` when only part is needed, and why.
     Every entry lands in the next context: the adapter inlines as many as
     fit its budget and lists the rest for the revived lead to read first,
     so list only what that step needs. Take line numbers from your earlier
     reads and put the symbol or heading in `why`. `references` holds files
     a later step may need, opened only then.
```

Resume section (`buildLeadCompactionSummary`), replacing its second sentence
"After `lead-revive`, resume from this summary and the immediate next step;
re-read a file only when that step needs it, not to rebuild the earlier
context.":

```text
After `lead-revive`, read the files under Required re-reads (to read first), then resume from this summary, the inlined files, and the immediate next step; open a reference or any other file only when a step needs it, not to rebuild the earlier context.
```

`ws-compact` parameter descriptions:

```text
required_rereads: Files the immediate next step needs, each with `lines` ("start-end", 1-based) when only part is needed, and why. Every entry lands in the next context: the adapter inlines as many as fit its budget, smallest first, and lists the rest to be read first, so list only what that step needs. Take line numbers from your earlier reads; put the symbol or heading in `why`.
references: Files the work ahead may need later, opened only when a step calls for them. Same entry shape as required_rereads; never inlined.
```

### Agreed shapes

Lever parameter, both optional, neither in `required`:

```ts
type FileListEntry = { path: string; lines?: string; why: string };
// ws-compact params gain:
required_rereads?: FileListEntry[];
references?: FileListEntry[];
```

Rendered summary sections, placed immediately before
`## Carried forward by the lead`, each omitted when it has no entry:

````text
## Required re-reads (inlined)
### `<path>` lines <start>-<end> - <why>
```
<content>
```
### `<path>` - <why>
```
<whole-file content>
```

## Required re-reads (to read first)
- `<path>` lines <start>-<end> - <why>
- `<path>` - <why>
- `<path>` - <why> (not found)

## On-demand references
- `<path>` lines <start>-<end> - <why>
- `<path>` - <why>
````

A whole-file entry (no `lines`) drops the `lines ...` part in every
section. An inlined heading shows the range actually taken, after clamping.

## Constraints

- Unchanged from 261006: the trigger latch, the priority hard > advisory >
  milestone, "a milestone waits for the advisory", the session_start and
  session_tree baselines, compaction dropping a pending milestone, the hard
  point's delivery and no re-steer, and the milestone `customType`
  (`ws-lead-context-milestone`), `display: true`, and `details.milestone`.
- Worker and explore sessions keep Pi's native compaction and get no
  milestones (lead only).
- The adapter's compaction push-hold, goal-loop hold/release, and the
  existing gates (no trigger while a preparation turn or a compaction is in
  progress) keep working.

## Prior Decisions

- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "Milestones are not preparation messages. They use their own `customType` ... are sent with `display: true`, and never set the `preparation` flag." — bearing: contradiction-candidate
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "Milestones are delivered as a steer, only on a turn that continues the run. A milestone is sent with `deliverAs: \"steer\"` at a `turn_end` whose `toolResults` is non-empty." — bearing: constrains
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "The advisory stays a dedicated wake turn. Its delivery (`agent_end`, `followUp`, `triggerTurn: true`) and once-per-crossing semantics are unchanged, as in 261004 D2." — bearing: supports
- 7c680e40c (2026-10-06, commit): "Priority hard > advisory > milestone via an else-if chain: a pending advisory blocks the milestone at tool-result turn_ends, and delivering latches to the highest threshold at or below usage" — bearing: constrains
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "The prose is pinned in this ticket ... the implementer does not author this prose, and the prompt tests pin each text verbatim with interpolated values." — bearing: supports
- d12237cb (2026-10-06, commit): "Dogfooding showed the lead dismisses the 50% advisory once and does not compact until 80% ... 261002 sends nothing between the thresholds." — bearing: supports
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, commit 0cda382d): "Pi native compaction and pi-better-compaction degrade long lead sessions (unbounded file lists, verbatim retention of user-role ws pushes)" — bearing: constrains
- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, Result ea1fd0795): "The `ws-compact` description and `lead-compact-guide.md` step 3 now state the route-dependent goal-less resume." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/lead-compact-guide.md, plus tests |
| scope.surface | public-interface | ws-compact lever parameter schema leadCompactParameterSchema and tool description seen by the lead model |
| scope.new_public_symbol | no | none required; FileListEntry may stay module-local |
| scope.new_type_contract | yes | ws-compact params gain required_rereads?: FileListEntry[] and references?: FileListEntry[] |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/lead-compaction.test.ts |
| complexity.reuse_points | confirmed | sendPreparation followUp triggerTurn path and leadCompactParameterSchema composition in src/goal-loop.ts were read |
| complexity.side_effect_risk | moderate | milestone delivery moves into the agent_end trigger path and sets preparation, which also drives the lever's route and resume decision |
| risk.correctness | moderate | latch, priority, advisory-wait and preparation interplay change boundary; new `milestone` lever route; budgeted smallest-first inlining |
| risk.fit | low | follows the advisory's existing agent_end followUp delivery and the continue_after_compact schema-composition pattern |
| risk.test | moderate | 261006 milestone tests must be rewritten to the new boundary and pinned prose verbatim |
| risk.security_or_contract | moderate | changes the lead-facing ws-compact tool schema and summary section layout read by extractLeadProse |

## Phases

### Phase 1: Milestone wake turns and re-read lists

- Move milestone delivery in `fireCompactionTriggers` from the tool-turn
  `steer` to `agent_end` (`followUp`, `triggerTurn: true`). The milestone
  turn sets the `preparation` flag like the advisory, so it is cleared at the
  next `agent_end` and no other trigger fires inside that turn.
- Add the `milestone` preparation kind and lever route: a milestone wake
  sets `preparationKind` to `milestone`, and `resumesAfterCompaction` treats
  it like `advisory`. In the `ws-compact` tool description, the `continue_after_compact`
  parameter description, and guide step 3,
  the route-dependent resume clause "after the advisory nudge or a user
  /compact" (guide: "after the advisory nudge or a user `/compact`") becomes
  "after the advisory nudge, a milestone, or a user /compact" (guide: with
  `/compact` in backticks).
- Replace `buildContextMilestoneMessage` with the two pinned milestone texts,
  selected by which milestone is delivered (the highest milestone at or below
  usage). Replace the advisory's closing sentence with the pinned one.
- Add `required_rereads` and `references` to `leadCompactParameterSchema` in
  `src/goal-loop.ts`, not to `leadProseParameterSchema`, so the fallback
  prompt does not gain them (the same reason `continue_after_compact` lives
  there). Add the inline budget config key beside
  `compaction_dialog_budget_bytes`. Render the three sections in
  `buildLeadCompactionSummary` as shaped above, reading each path relative to
  the session's working directory. Keep `extractLeadProse` unchanged; placing
  the sections before the prose heading keeps them out of the carried prose.
- Update the guide, the Resume sentence, and the `ws-compact` tool
  description and parameter descriptions with the pinned prose.

Done when the `agents-plugin-pi` suite passes with tests covering: milestone
delivery at `agent_end` only (none at any `turn_end`); both milestone texts
and the advisory closing sentence verbatim with interpolated values,
including non-default thresholds; the 261006 latch, priority, and
advisory-wait cases updated to the new boundary; the milestone turn setting
and clearing `preparation`; a `ws-compact` call from a milestone wake taking
the `milestone` route, with no goal-less resume unless
`continue_after_compact` is set; smallest-first inlining within the budget with
ties in input order, an entry that does not fit going whole to the
to-read-first list, a `lines` range clamped at the file's end, a `lines`
value that does not parse and one starting past the last line (both
listed to read first as written), a whole-file entry, a `(not found)`
entry, a fence longer than a backtick run inside an inlined file, and each section omitted when empty; the
budget config key and its default; the fallback summary carrying none of
the sections; and the guide and Resume text verbatim.

### Result (62b54fe38) - 2026-10-07

Landed as specified. Milestones are now an `agent_end` `followUp` wake turn
(`triggerTurn: true`) that sets `preparation` with `preparationKind:
"milestone"`; a `ws-compact` call from that turn takes the `milestone` route,
which `resumesAfterCompaction` treats like `advisory`. Both milestone texts,
the advisory closing sentence, guide steps 2-3, the Resume sentence, and the
tool/parameter descriptions follow the pinned prose. `ws-compact` gains
`required_rereads` / `references` (in `leadCompactParameterSchema` only);
`buildFileListSections` in `lead-compaction.ts` inlines must-reads smallest
first within the new `compaction_reread_budget_bytes` budget (default 40960,
in `config-manifest.json`), lists the rest to read first, and lists
references; paths resolve against `ctx.cwd`.

Verification: `npm test` in `agents-plugin-pi`: 2055 tests, 2052 pass, 0 fail,
3 skipped. Lite review: clean, one Minor left open (each required re-read is
read whole and synchronously, even for a small `lines` range or a file far
over the budget; a pre-read size check would bound it).

Decisions:
- Milestone text and `details.milestone` both use the highest milestone at
  or below usage (the one delivery latches to); `details.milestone`
  previously carried the lowest crossed one.
- The advisory trigger carries `milestonePercents` explicitly, so
  `compactionThresholds` stays the only source of the milestone points.
- A blank `lines` is treated as omitted (whole file); lever entries without
  a string `path` are dropped.
- The budget counts only the taken content's UTF-8 bytes, not headings or
  fences.

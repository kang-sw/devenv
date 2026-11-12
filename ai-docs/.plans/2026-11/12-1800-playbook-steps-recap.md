# Playbook edits: co-authoring plans by steps; a recap never asks

## Outcome

Two shipped playbook edits, one commit each (this plan file in the first).

### 1. lead-coauthor-paged-document: plan by steps, not by page count

User decisions (whiteboard (437) ok, (438) a, (439) a; final wording OK'd by
the user on the whiteboard, 2026-11-12): the co-authoring flow first settles
abstract **steps** (an ordered list of sections, each with its purpose and
the content it carries, no page count); pages are made when a step's turn
comes; progress is shown per step.

File: `agents-plugin/rsrc/lead-coauthor-paged-document/lead-coauthor-paged-document.md`
and its wsflow copy at the same path under `agents-plugin-wsflow/rsrc/` if one
exists (check `ai-docs/manuals/wsflow-mirroring.md`; if wsflow only has a shim
that reads the shared playbook, leave the shim). Update `rsrc/manifest.json`
sha256 for every changed rsrc file (the wsrsrc test checks them).

Apply these approved changes (English; adjust only for grammar and fit with
the surrounding text, keep the meaning):

- **Plan file**: `plan.md` ... is the authority for the steps, the confirmed
  decisions with their `(nn)` IDs, the page-number map of the pages built so
  far, and the review-pending list. (Was: "the outline", "the page-number map".)
- **Step 1**: "Discuss the topic: audience, purpose, and any length or
  talk-time limit. Record a limit in `plan.md` as a reference only: it fixes
  no page count, and you tell the user when the finished document exceeds
  it." (Was: "audience, purpose, and length or talk time.")
- **Step 5**: "Discuss the document's steps: an ordered list of sections,
  each with its purpose and the content it carries, and no page count; once
  the user confirms them, record them in `plan.md`." (Was: overall order and
  direction.)
- **Step 6**: build from the front, one step per round: decide the step's
  pages as you write them, render-check each new page, regenerate the
  progress section, and stop for the user's review. When a step is large
  enough that its split is itself a question, propose the split first and
  wait. Apply the corrections, including splitting or merging pages, record
  each in `plan.md`, and repeat. (Was: one or two sections per round.)
- **Progress section**: the `(<done>/<total>)` count counts steps (say
  "steps" in the text); the row item becomes "one row per step: the status
  marker (unchanged ✓ ◐ ☐), a bold step label followed by the on-screen page
  range of its built pages (none for an unbuilt step), and one line of the
  step's planned content" (was: one row per planned page, per section for A4,
  with the page number once the page exists); the example HTML's
  `Part 1 <pg>3</pg>` becomes a range such as `Part 1 <pg>3–5</pg>`.
- Make any other sentence in the playbook that still presumes a page list
  fixed up front consistent with the above (report each such edit).

### 2. whiteboard playbook: a recap is not a place to ask

User (whiteboard, 2026-11-12), after the lead put a "confirm and I'll edit"
request in a topic recap and body instead of asking it as an item: state
explicitly that a recap is not where confirmation is asked. Dogfood finding.

File: `agents-plugin/rsrc/whiteboard/whiteboard.md` (the "Topic recap"
bullet, ~line 191) and the wsflow mirror if any; manifest hashes. Add a rule
in the playbook's own style: a recap (and a topic body) states status only
and never asks the user anything; anything that waits on the user's
confirmation, including "OK this draft and I'll proceed", is a `wb:add` item
(an announced default or technical item for a plain confirmation, a policy
item for a choice), so it shows as waiting on the user. Keep it one or two
sentences; read `ai-docs/manuals/skill-authoring.md` and apply its rule tests.
If the workflow manual's `### Whiteboard` section (lead-workflow-manual rsrc)
carries an overlapping sentence, keep them consistent.

## Boundaries

- Read first: `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md` (no repo-only references in
  shipped text: no ticket stems, board numbers, ai-docs paths).
- Playbook asset content only, tier T0. No other refactors.
- Never run go build/test/vet locally.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test ./internal/wsrsrc/ ./internal/mcp -count=1`
  from the worktree root: green (manifest hashes, mirror/parity checks).
- Any local Python tests for wsflow mirrors/shims under agents-plugin*/tests.

## Return

Commits on `impl/develop/playbook-steps-recap` in
`/home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone`. Do not merge or
push. Report commits, the final wording of each changed paragraph, and test
output.

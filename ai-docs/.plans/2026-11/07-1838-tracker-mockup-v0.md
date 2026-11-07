# Plan: team tracker mockup v0 - an unstyled page to settle the data structures

## Outcome

The epic `261107-epic-ws-personal-common-store` (read it first, all of
`## Cross-Child Decisions`) was rewritten as an agent-first team tracker. Its
first child is an unstyled, fast-iteration mockup the user looks at to decide
the concrete data structures (user, 2026-11-07: "스타일을 제외한
목업(가능한 빠르게 이터레이션 할 수 있게)을 보면서 구체적으로 어떤 데이터
구조를 가져갈지 제대로 정하는 걸로"). This is v0: a first concrete proposal
the user will iterate on with the lead. It is not a design decision; it makes
a proposal visible.

Deliver `ai-docs/ref/tracker-mockup/`:

- `index.html` + one script file (plain JS, no build step, no dependencies,
  opens from `file://`) and JSON fixtures (or JS files assigning fixtures to
  a global, if `file://` fetch is blocked; pick whichever opens directly).
- Browser-default styling only (no CSS beyond minimal layout such as a
  two-column grid and `pre` wrapping). No design work.
- `README.md` (English): how to open it, the proposed data model (each
  record/event type with its fields and meaning), and an "Open questions"
  list of every data-structure choice v0 made that the user should confirm
  or change, each with the alternative.

### What the page shows

1. A store switcher: at least two stores - a team store (8-15 members, an
   admin pair, a 3-level lead/follower tree) and a personal store (one
   member).
2. A "viewing as" member switcher. Every view is computed in the browser by
   merging all members' event logs (per-member ref = per-member log, epic
   decision Storage model).
3. Views: my assignments (incoming, with who assigned and from which
   ancestor), the org tree (from the roles file), an item list, and an item
   detail with its event history, subtasks, and rolled-up progress.
4. A raw-data pane beside the views: the roles file and every member log as
   editable JSON; an "apply" button re-merges and re-renders, so the user (or
   the lead) can try a structure change in seconds. Invalid events (unsigned,
   signer not allowed by the rules, assignment outside the assigner's reach,
   relation edits by a non-admin, cycles) are shown flagged, not applied,
   with the reason (epic: clients ignore and flag rule-breaking events).
5. A few actions that append events to the current viewer's log (create an
   item, assign within N levels to all or to chosen people, split into
   subtasks for own followers, report progress, close, confirm when the task
   requires assigner confirmation), so the user sees which events each
   action produces.

### Data model to propose (fields are v0's call; list every call under Open questions)

- Roles file: admins (several; any admin signature valid), members with
  public key ids, lead relations stored as a list of leads validated to
  length 1 (epic decision Roles).
- Event envelope: author, per-author sequence, timestamp, signature
  placeholder (mock a signature as `sig: "<keyid>"`; no real crypto), type,
  payload. Merge order across logs and how ties break is an Open question.
- Item/task: feature-level work item ("특정 기능 누가 완성"), notes, simple
  TODOs (epic Scope); assignment target = everyone within N levels or chosen
  people within N (board (128) c); completion mode per task (assignee closes
  default, or assigner confirms; (125) c); parent/subtask link for
  re-delegation with roll-up ((129) a); optional free-text project
  reference ((121)).
- Agent recording: mark which events an agent writes automatically
  (progress on the user's own assigned tasks only; (123) b), e.g. an
  `via: "agent"` field - an Open question.

## Settled constraints

- Only files under `ai-docs/ref/tracker-mockup/` (plus this plan). No
  shipped code, no Go, no dashboard change. Not shipped, so the
  shipped-surface boundary does not apply; still keep it free of
  machine-local paths.
- UI strings in the mockup may be English; keep them short.
- Commit the mockup (the user wants the mockup git-tracked in its final
  state; v0 is committed as the starting point).

## Boundary

Commit on branch `delegate/tracker-mockup-v0` only; do not merge or push.
Return to the lead if a decision in the epic seems contradictory or
impossible to model.

## Verification

`node --check` on the script. Open `index.html` in headless Chromium (cached
Playwright under ~/.npm/_npx/*/node_modules/playwright) from `file://`:
no console errors; switching store and viewer changes the views; each action
appends an event and re-renders; editing the raw JSON and applying
re-renders; at least one fixture event per rejection rule shows flagged with
its reason. Report the Open questions list in your final report, plus what
was not checked.

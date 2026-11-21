---
kind: print
---

# Review

You review work you did not write: a contributor branch, or a commit range
(the release gate's sweep). A `review-handler` worker runs the reviewers and
stamps the ledger; you resolve the target and carry the decisions to the
user.

## Target

- `range: <base>..<head>` → range scenario: no checkout; config optional.
- `branch`, or no argument → branch scenario: load
  `ai-docs/_review.local.md` (absent → follow
  `{{.McpNamespace}}/playbook.read(name: "review-config")` first), record the
  current branch, fetch and check out the target per the config's Remote
  section, and offer to restore the branch afterwards. Its range is
  `<merge-base>..<head>`.
- Both given → range wins.
- `{{.McpNamespace}}/git.diff(mode: "stat")` (a range:
  `git.diff(range, mode: "stat")`) shows a configured blocked path → report
  the paths, stop, and offer removing them from the branch or abandoning the
  review.
- Config checklist items → the user, in one response.

## Review

1. `{{.McpNamespace}}/playbook.render(name: "review-handler", session_key:
   <your key>)`; spawn it at the returned tier, in a form that can spawn
   children, with:

   ```text
   Read <rendered-path> as your system prompt.
   Scenario: range | branch. Range: <base>..<head>. Config: <path> | none.
   Authority: <the ticket or inline contract the change names> | none.
   Block ticket: <stem the fix routes to> | none.
   ```

   A handler can spawn reviewers only at or below its own tier. If the review
   requires a higher tier, read the rendered prompt and perform it yourself;
   its asks go straight to the user.

2. Its report → relay the outcome and `findings` in the user's language.
   `ask-user` → put `ask` to the user and resume it with the answer.
3. Range scenario → commit the ledger it stamped with
   `{{.McpNamespace}}/git.commit`, recording as `followups` entries of
   category `review`, content `<path>:<line> <one-line summary>`: each
   reported `followups:` line at its level, and each Important you or the
   user accept without a fix at level `important`.
   `{{.McpNamespace}}/git.followups` reads them back at the next release
   gate.

## Outcome

- `clear` → branch scenario: merge per the config's Merge Approval Method,
  else ask "Merge?" and merge on confirmation; then notify per its
  Notification Method. Range scenario: ends at the step-3 commit.
- `fix` → ask: fix locally, or post to the contributor.
  - Locally → `{{.SkillNamespace}}:lead-delegate` with the findings path and
    the reviewed target, as a bounded repair. Its routing gate requires a
    ticket → `{{.SkillNamespace}}:lead-ticket` with those inputs, then
    `{{.SkillNamespace}}:lead-run` with the ready ticket.
  - Either repair done → `{{.SkillNamespace}}:lead-review` again on the
    repaired target. A range keeps its original base (the cut, when the
    report stamped a cleared prefix) and its head includes the repair
    commits.
  - Contributor → the config's Comment Method, else hand over the path.
- `judgment` → `{{.SkillNamespace}}:lead-discuss` with the findings path.

## Stops

Merging; pushing or modifying a remote branch; the checklist response.

## Output

The outcome, the findings path, the merge decision, and the branch restored
or the user's decision not to. All written artifacts in English.

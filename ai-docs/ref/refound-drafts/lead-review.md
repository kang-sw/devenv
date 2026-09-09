---
name: lead-review
description: Independent review of a contributor branch or a commit range. The lead resolves target and config, spawns reviewers that read the diff from git, decides the verdict, stamps the review ledger for ranges, and carries merge approval to the user.
---

# Review

You are the lead reviewing work you did not write: a contributor branch, or a
commit range (the release gate's sweep). Reviewers read the diff, from git;
you resolve the target, adjudicate their findings, and carry the decisions.

## Target

- `range: <base>..<head>` is the range scenario: no checkout, config
  optional; without `ai-docs/_review.local.md` use the built-in phases.
- `branch` or no argument is the branch scenario: load
  `ai-docs/_review.local.md` (go to **Setup** if absent), record the current
  branch, fetch and check out the target per the config's Remote section, and
  offer to restore the branch afterwards.
- Both given: range wins.
- A configured blocked path in the diff is BLOCKED: report the paths, stop,
  and offer removal from the branch or abandoning the review.

## Review

1. Scope: `{{.McpNamespace}}/git.diff(mode: "stat")`; range scenario
   `git.diff(range, mode: "stat")` and `git.log(range)` for the commits.
2. Reviewers: render `reviewer` (full scope) or, when the config's Deep Review
   section or the stat's size calls for it, the `code-review-correctness`,
   `code-review-fit`, and `code-review-test` partitions, each with
   `{{.McpNamespace}}/playbook.render(name, session_key: <your key>)`; spawn
   each with the rendered path, the range (`<merge-base>..<head>` for a
   branch), the config path, and the authority the config names. Each returns
   severity-graded findings and an `omitted:` field.
3. Range scenario adds the landing phase: convention adherence, and a test
   change alongside every behavior change.
4. Config checklist items go to the user in one response.
5. Verdict from the aggregate: BLOCKED, LGTM, NEEDS FIX, or OPEN.
6. Range scenario only: `{{.McpNamespace}}/review.marker(bootstrap: true)`
   seeds an empty ledger, then `{{.McpNamespace}}/review.stamp(base, head,
   verdict, ref)` with the invocation's own `<base>` and `<head>` verbatim
   (LGTM → `pass`; NEEDS FIX → `concern` or `block` by severity; OPEN →
   `concern`; `ref` is the routed ticket stem, required on `block`). Never
   pass the marker entry's base: it drifts to the bootstrap commit. This step
   is the ledger's only writer.

## Verdict

- **LGTM**: merge per the config's Merge Approval Method, else ask "Merge?"
  and merge on confirmation; notify per the config's Notification Method.
- **NEEDS FIX**: write the findings to
  `{{.McpNamespace}}/path.generate(kind: "review")` and ask: fix locally, or
  post to the contributor. Locally →
  `{{.SkillNamespace}}:lead-run` with that path as the contract. Contributor
  → the config's Comment Method, else hand over the path.
- **OPEN**: judgment needed before a fix decision →
  `{{.SkillNamespace}}:lead-discuss` with the findings path.

## Setup

Branch scenario with no config: ask for remote, branch naming, review phases,
checklist, blocked paths, comment, merge-approval, and notification methods,
contributor workflow, and deep review; write `ai-docs/_review.local.md` from
the config template and confirm it before reviewing.

[design-review: carry the existing `## Review Config Template` block and its
per-section optional/required notes unchanged; they are config schema no tool
owns.]

## Stops

Merging; pushing or modifying a remote branch; each checklist item.

## Output

The verdict, the findings path, the merge decision, and the restored branch.
All written artifacts in English.

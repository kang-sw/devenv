---
kind: print
---

# Review

You are the lead reviewing work you did not write: a contributor branch, or a
commit range (the release gate's sweep). Reviewers read the diff, from git;
you resolve the target, adjudicate their findings, and carry the decisions.

## Target

- `range: <base>..<head>` is the range scenario: no checkout, config
  optional; without `ai-docs/_review.local.md` use the config template's
  Review Phases below as the built-in default.
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
   branch), the config path, and the review authority — the ticket or inline
   contract the change names, when there is one. Each returns severity-graded
   findings and an `omitted:` field.
3. Range scenario adds the Landing Lens phase — the config's section, or the
   built-in default in the template below — plus a test change alongside every
   behavior change.
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

- **BLOCKED**: the blocked-path stop in **Target** already ran; nothing is
  reviewed and nothing is stamped.
- **LGTM**: branch scenario merges per the config's Merge Approval Method,
  else asks "Merge?" and merges on confirmation, then notifies per the
  config's Notification Method; a range scenario ends at the step-6 stamp.
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

### Review Config Template

```markdown
# Review: <project>

## Remote
<how to list and fetch MR/PR branches>

## Branch Naming                       ← optional
<prefix or regex pattern, e.g. feature/, fix/, TICKET-[0-9]+>

## Review Phases
### intent
Commit messages and ## AI Context match the stated ticket or MR purpose.
### alignment
Diff is consistent with the ticket's stated contract and the project's declared conventions.
### risk
No breaking changes, security issues, or missing tests without justification.

## Landing Lens                        ← optional to customize; range scenario always runs it (built-in default below if omitted); branch scenario never runs it
Diff follows the repo's own conventions (`AGENTS.md` and any authoring manual it
names). A document the change contradicts is updated in the same range — the
document whose own function the change invalidates, not just "any doc touched."

## Checklist                           ← optional
- [ ] <gate item>

## Blocked Paths                       ← optional
- <path pattern>

## Comment Method                      ← optional
<glab mr note / GitLab Web UI / none>

## Merge Approval Method               ← optional
<local merge → push / push → web approve → merge>

## Notification Method                 ← optional
<post-merge contributor notification method>

## Contributor Workflow                ← optional; default: mixed
mixed

## Deep Review                         ← optional
threshold: 20 files / 500 lines
```

## Stops

Merging; pushing or modifying a remote branch; the checklist response.

## Output

The verdict, the findings path, the merge decision, and the branch restored
or the user's decision not to. All written artifacts in English.

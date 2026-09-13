---
title: "Pi read-only reviewers cannot write the findings artifact required by their playbook"
related:
  260913-feat-ws-pi-delegated-write-scopes: prerequisite; provides monotonic scoped native edit/write wrappers
  260912-feat-ws-pi-bounded-web-access-for-explore: observed during independent partitioned review
  260911-feat-ws-pi-held-push-batch-delivery: second live reproduction of the same missing artifact capability
  260912-bug-ws-pi-subagent-context-meter-omits-cached-input: third live reproduction through a spawned reviewer
  260913-bug-reviewer-findings-path-write-unavailable: duplicate capture absorbed here
  260913-bug-ws-reviewer-cannot-write-findings-path: duplicate capture absorbed here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2176c4676ba387b6
sage-review-completeness-reviewed: 2176c4676ba387b6
---

# Pi read-only reviewers cannot write required findings artifacts

## Background

During Pi partitioned review, both fit and test reviewers reported that their tool surface provided only read/grep/find/ls and could not materialize the requested findings path. The rendered code-review playbooks nevertheless require writing a detailed report to that path and returning only a clean/non-clean status. The parent had to preserve their full pushed reports and write the artifacts itself.

The mismatch is between `agents-plugin-pi/src/delegation-policy.ts` read-only reviewer admission and the shared reviewer output contract in `agents-plugin/rsrc/code-reviewer.md`. This ticket leaves every other harness unchanged; Pi deliberately keeps its reviewer checkout access read-only and needs a narrower host-local capability.

## Decisions

- Keep shared reviewer and worker playbooks unchanged. This ticket does not alter the behavior or reviewer authority of Codex, Claude, or other harnesses.
- Depend on `260913-feat-ws-pi-delegated-write-scopes` rather than introducing a reviewer-specific writer or capability ceiling.
- When a full worker spawns a code reviewer, pass the exact generated findings file as one `{ kind: "file" }` entry through the prerequisite's structured `write_scopes` spawn field. Do not derive authority from reviewer prompt text. (pending 260913-feat-ws-pi-delegated-write-scopes)
- The full worker is the binding owner: its unrestricted effective write capability authorizes this narrower grant through the prerequisite's parent-to-child subset check. Runtime authorization does not require separate generated-path provenance. The reviewer receives Pi's scoped native `edit`/`write` wrappers for that file but no Bash, unrestricted checkout write, directory tree, or glob grant. (pending 260913-feat-ws-pi-delegated-write-scopes)
- Preserve the immutable binding through reviewer reload or resume according to the prerequisite contract. The reviewer may create or replace only its assigned Markdown report; another artifact or checkout path remains unavailable. (pending 260913-feat-ws-pi-delegated-write-scopes)
- Preserve full clean and non-clean reports. The worker's existing artifact consumption and remediation flow remains unchanged.

## Constraints

- Pi-extension local only; do not modify shared ws-mcp or shared `agents-plugin/rsrc` contracts.
- Reviewer repository access remains read-only except for the one exact generated findings file.
- The worker obtains the assigned path from the existing generated review-path flow and passes it as structured capability data. Adapter authorization derives from the parent's effective write scope, not from generated-path provenance or parsed prose.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 requires Pi spawn admission/dispatch changes and spawned-reviewer coverage; exact files are not yet selected |
| scope.surface | internal | no exported symbol or public schema change is assigned outside the prerequisite |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | `WriteScope` is assigned to pending 260913-feat-ws-pi-delegated-write-scopes |
| scope.test_surface | existing | agents-plugin-pi/test/recursive-worker.test.ts covers reviewer provenance and report handoff; no spawned-reviewer artifact test was found |
| complexity.reuse_points | unconfirmed | generated review paths and Pi reviewer admission exist, but the current ws-agent-spawn schema has no write_scopes field and no scoped binding |
| complexity.side_effect_risk | high | the current reviewer admission deliberately removes edit/write, so the new exception must not widen its other tools |
| risk.correctness | high | a missing or wrong binding leaves the required artifact absent or writes the wrong file |
| risk.fit | high | agents-plugin/rsrc/code-reviewer.md requires a findings-path write while Pi reviewer admission is read-only |
| risk.test | high | a real spawned reviewer must prove clean/non-clean publication and unchanged checkout restrictions |
| risk.security_or_contract | high | reviewer admission must grant exactly one generated file and no broader scope |

## Phases

### Phase 1: Bind the generated findings file to reviewer spawn

After `260913-feat-ws-pi-delegated-write-scopes` lands, extend Pi reviewer dispatch to attach the exact generated findings file as one file scope. Keep the existing reviewer prompt and worker artifact-consumption flow unchanged; the reviewer satisfies that contract through the prerequisite's scoped native edit/write wrappers.

Add a spawned-reviewer integration regression rather than only static policy coverage. Verify clean and non-clean report publication, existing worker consumption, reload/resume with the same immutable file binding, rejection of an unbound or second artifact, checkout paths, directory or glob widening, and attempts to gain Bash or unrestricted native edit/write. Preserve the context-meter, held-push, and bounded-web reproductions as provenance for the one canonical defect.

## Sage Review Round 1 (2026-09-13)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | No trusted source binds the generated findings path to the spawn | critical | missing |
| 2 | Current policy cannot delegate a reviewer-only writer from a worker | critical | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|

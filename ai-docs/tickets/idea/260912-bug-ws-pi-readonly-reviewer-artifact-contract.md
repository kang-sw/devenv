---
title: "Pi read-only reviewers cannot write the findings artifact required by their playbook"
related:
  260912-feat-ws-pi-bounded-web-access-for-explore: observed during independent partitioned review
---

# Pi read-only reviewers cannot write required findings artifacts

## Background

During Pi partitioned review, both fit and test reviewers reported that their tool surface provided only read/grep/find/ls and could not materialize the requested findings path. The rendered code-review playbooks nevertheless require writing a detailed report to that path and returning only a clean/non-clean status. The parent had to preserve their full pushed reports and write the artifacts itself.

The mismatch is between `agents-plugin-pi/src/delegation-policy.ts` read-only reviewer admission and the shared reviewer output contract in `agents-plugin/rsrc/code-reviewer.md`. Read-only source review is correct; broadening reviewer mutation authority merely to satisfy artifact prose is not automatically the right fix.

## Phases

### Phase 1: Align read-only review output with artifact ownership

Settle a host-neutral output handoff that allows a read-only reviewer to return its complete findings without losing evidence. Either make the parent own artifact materialization or provide a tightly scoped artifact-writing mechanism. Test the real Pi reviewer surface against the rendered output contract, including clean reports. Keep shared-playbook edits on develop under the Pi-track policy; do not grant arbitrary source write authority.

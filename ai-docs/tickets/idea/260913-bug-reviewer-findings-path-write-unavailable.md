---
title: Reviewer delegates cannot write required findings artifacts
---

# Reviewer delegates cannot write required findings artifacts

## Background

The partitioned code-review playbooks require every reviewer to write a detailed report to the invocation's findings path. During `260912-bug-ws-pi-subagent-context-meter-omits-cached-input`, a reviewer spawned through `ws-agent-spawn` reported that its read-only tool surface had no file-write capability, so it could return the report content only in its final message. This makes the playbook contract impossible for an otherwise correctly constrained read-only reviewer and forces the worker to reconstruct the artifact.

## Phases

### Phase 1: Align reviewer permissions with the findings-artifact contract

Trace how reviewer playbooks are rendered and how spawned reviewer tool capabilities are assigned. Preserve read-only repository access while providing a bounded way to write the allocated findings path, or change the review artifact contract so the lead owns persistence explicitly. Cover the selected contract with a regression that exercises a spawned reviewer rather than only static playbook text.

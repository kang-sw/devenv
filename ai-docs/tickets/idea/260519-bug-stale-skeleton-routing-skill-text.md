---
title: Stale skeleton routing text in implementation skills
related-mental-model:
  - workflow-skills
---

# Stale skeleton routing text in implementation skills

## Background

Dogfooding proceed/implement routing discussion exposed stale skeleton language
in the active skill texts. Specs and mental models say `lead-write-skeleton` is
deprecated from normal implementation routing and that contract checkpoints now
belong in `lead-write-code` briefs, but `lead-proceed`, `lead-implement`, and
`lead-write-ticket` still describe skeleton decisions as part of normal routing.

This stale wording can cause agents to reintroduce skeleton dispatch into new
routing design even though the intended normal route is direct edit versus
write-code, with contract-brief depth handled inside write-code.

## Expected Follow-Up

Audit the active ws and wsflow skill text for normal-routing skeleton references.
Remove or mark legacy-only references so proceed/implement dispatch discussions
and execution paths use direct-edit versus write-code decisions without a
skeleton branch.

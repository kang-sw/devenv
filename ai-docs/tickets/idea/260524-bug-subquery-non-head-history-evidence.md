---
title: Subquery can cite non-HEAD branch history as current evidence
---

# Subquery can cite non-HEAD branch history as current evidence

## Pending Removal (2026-06-09)

Resolved-by-deletion candidate under `260605-epic-ws-playbook-factory-pivot`.
`ws/subquery` is absorbed into the harness-native Explore agent and removed at
milestone M2, so this subquery-specific evidence-reliability bug ceases to
exist. Do not invest in a standalone fix; drop this ticket to `.dropped/` in the
same commit that removes subquery. Retained in place until then so git blame and
ticket scans surface the coupling.

## Background

During a broad active-ticket cleanup survey, `ws/subquery` reported several
dashboard Activity Console tickets as ready to close because it found close and
implementation commits. A direct check showed those commits were not ancestors
of the current `main` HEAD; they belonged to other local dashboard branches.

That makes the survey result misleading for current-branch ticket triage. A
caller reasonably expects current active-ticket cleanup evidence to be scoped to
the current branch unless the answer explicitly says it is using all reachable
history.

## Problem

The subquery evidence boundary is ambiguous for repository-history surveys.
When an answer cites commits outside the current branch without labeling them,
it can make active tickets look stale or completed when they are still active in
the checked-out branch.

## Follow-Up

- Decide whether `reference-discovery` and `subquery` prompts should default to
  current-HEAD ancestry for ticket status evidence.
- If cross-branch history is useful, require answers to label it as such and to
  separate current-branch conclusions from branch-local findings.
- Add a small verification pattern for stale-ticket surveys: cited completion
  commits should be checked with `git merge-base --is-ancestor <commit> HEAD`
  before recommending current-branch closure.

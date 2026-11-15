---
title: Investigate pooled worktree reuse of a historically protected deliverable checkout
---

# Investigate pooled worktree reuse of a historically protected deliverable checkout

## Background

During dogfooding on 2026-11-14, a lead called `worktree.acquire` with profile
`full`, base `2f6831c4597eb13e7aaa2272209de81f98b8bc28` and target branch
`impl/develop/swarm-easel-clay`. The pool automatically returned the checkout
named `spruce-december-suave`, with `reused: true` and `provisioned: false`.
The caller did not choose that path.

An older lead agenda explicitly reserved that checkout for a Tauri application
and native binary handoff and prohibited reset/release. The acquisition result
did not surface that historical preservation context. Static existence checks
at the remembered `agents-plugin-tool/desktop/target/debug` app/binary paths
returned false after acquire. No pre-acquire inventory exists in this report;
artifact deletion by this acquire is NOT established. The path could have been
released or repurposed under later authority absent from the caller's context.
MAIN was asked to confirm the current hold before further cleanup or release.

This is a follow-up for the workflow product, not an assumption that an agenda
entry is already a machine-enforced lease or that the pool violated one.

## Constraints

- Investigation only; no new pool policy, protected marker, API/store shape or
  compatibility tier is selected by this capture.
- Establish lease/release history and actual artifact locations before claiming
  data loss or an acquire defect. Separate stale conversational memory from
  missing machine-visible protection.
- Do not reproduce by deleting owner deliverables or reuse the protected tree
  for destructive testing. Any future fixture must be isolated.
- Relevant implementation edits read shipped-surface and release-compatibility
  manuals before changing tooling behavior.

## Phases

### Phase 1: Determine preservation evidence and the pool eligibility boundary

Inspect the incident's available lease/release and acquire evidence. Determine
whether later authorized disposal made the old hold stale, whether protection
was never represented in the pool, or whether an existing protection was
ignored. Bring any needed downstream-visible design choice to the owner before
implementation. This idea neither authorizes a fix nor promises restoration of
artifacts whose prior existence/location has not been independently established.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - A speculative observation that the pool reused a past agenda's preservation note without knowing it; no evidence of deletion.

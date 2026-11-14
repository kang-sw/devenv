---
title: Expose the Pi reviewer spawn write-scope requirement before dispatch
---

# Expose the Pi reviewer spawn write-scope requirement before dispatch

## Background

During review sweep (794), three `ws-agent-spawn` calls using rendered `code-review-correctness`, `code-review-fit`, and `code-review-test` prompts were refused with `ws-pi-agent: code reviewer requires exactly one file write scope`. The public tool description presents `write_scopes` as optional and does not expose this reviewer-specific restriction. The lead-review procedure says to render and spawn reviewers but does not mention reserving a single output file and granting exactly that file.

The refused calls returned no agent ID. Reserving one report path per reviewer with `ws/path.generate` and supplying one exact-file grant allowed all three dispatches to succeed. This is a discoverability/contract mismatch, not a request to relax reviewer write isolation.

## Decisions

- Capture only; no implementation approval or readiness claim.
- Preserve exact-file reviewer output isolation. Prefer documenting or surfacing the dispatch requirement over weakening enforcement.
- Candidate compatibility tier is T0 if the correction only clarifies tool/playbook guidance; settle the tier against the actual proposed surface before implementation.

## Phases

### Phase 1: Align reviewer dispatch guidance with enforced output grants

Verify the role-specific enforcement and relevant tool schema, lead-review playbook, and tests. Make the required single report-path allocation and file grant discoverable before a reviewer spawn. Preserve read-only source access and existing permission checks. Add focused contract coverage for successful bounded dispatch and actionable refusal of missing or multiple grants. Changes to shipped workflow guidance require the applicable authoring/boundary manuals and approval rules.

Verification boundary: test the dispatch contract without building or swapping the shared daemon. Do not treat the sweep's successful retry as implementation acceptance.

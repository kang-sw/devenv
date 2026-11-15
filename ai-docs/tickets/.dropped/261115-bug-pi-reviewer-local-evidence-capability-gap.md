---
title: "Pi design reviewer cannot obtain local Git evidence within its capability ceiling"
related:
  261114-bug-design-review-requires-unavailable-researcher-spawn: adjacent reviewer research-procedure and capability mismatch
  261115-feat-ws-dashboard-conversation-rendering-consolidation: dogfood reproduction during its ready-promotion design review
---

# Pi design reviewer cannot obtain local Git evidence within its capability ceiling

## Background

During the 2026-11-15 ready-promotion design review for conversation rendering,
a root-rendered `ticket-reviewer-design` agent in a pooled housekeeping sparse
checkout reported that `explore` was rejected before allocation with
`child network capability exceeds parent ceiling`. Its review was static and
local-only. The same reviewer reported no Git-show/shell reader, while product
source files were absent from the sparse checkout. The lead had to supply
pinned source excerpts as an artifact instead of the reviewer obtaining local
evidence independently.

The exact host refusal is reported by agent
`conversation781-design` (`fa3c6f4f-cb7b-4f03-9d07-fc29eef5a434`). This is a
caller-visible capability/discoverability gap, not evidence that the capability
ceiling itself is incorrect. No rejected child allocation, network request,
source mutation or runtime operation is claimed to have occurred.

## Constraints

- Preserve capability ceilings and the local-only review boundary. Do not
  bypass network restrictions, expand sparse checkout, or grant mutation
  authority merely to obtain source evidence.
- The observed refusal does not establish whether the defect is an inherited
  explorer default, dispatch validation, tool exposure or reviewer procedure.
  Determine that from source and a focused reproduction before selecting a fix.
- This idea captures dogfood evidence only; no harness or workflow semantics
  change, permission expansion, or implementation is approved.

## Phases

### Phase 1: Diagnose local-review evidence access and propose a compatible repair

Trace the reviewer/explorer capability request and available read-only source
primitives. Reproduce the local-only review in a housekeeping sparse checkout,
including a source path present in Git but absent on disk. Establish how the
reviewer can obtain exact source evidence without requesting network or
modifying checkout. Return any observable tool/workflow contract change for
approval before implementing it. Keep both pre-allocation refusal evidence and
successful local-read controls; do not claim a review pass from unavailable
source evidence.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 리뷰어 로컬 증거 부족 이야기로 같은 계열 티켓과 중복이고 결함이 아닌 권한 한계 관찰; 261115-research-ws-pi-explore-pinned-git-evidence-gap.

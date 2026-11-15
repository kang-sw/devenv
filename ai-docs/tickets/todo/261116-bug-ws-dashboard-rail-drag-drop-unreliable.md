---
title: "Rail drag reorder still drops unreliably even when the insertion line shows"
---

# Rail drag reorder still drops unreliably even when the insertion line shows

## Background

The fix merged at `b98a3a7a6` (rail drag keeps working during live updates)
did not fully resolve the problem. The owner checked it under (1171) on
2026-11-16 and reported, verbatim:

> 여전히 자리 바꿀 때 급하게 드롭하면 안 되고 인내심있게 기다렸다드롭하거나 해야 하는 거 같아요. 그마저도 성공률이 영 낮습니다 ...; 선이 보이는데도요.

Observed:
- A quick drop fails. Waiting before the drop helps only somewhat.
- Even with waiting, the success rate is low, although the insertion line is
  visible at the target.

## Decisions

- The visible insertion line is the contract. A drop while the line shows
  must reorder to exactly that slot on the first try, quick or slow.

## Phases

### Phase 1: Reproduce and fix the drop commit

- Reproduce in a real browser: a fast drag-and-drop of a top-level session
  near an expanded parent, under live SSE updates. Measure the success rate
  before the fix.
- Find why the drop is lost while the line shows. Suspects: the drop target
  being recomputed or replaced by a rail swap between dragover and drop; a
  hold/timer gate on the drop; a mismatch between the line's slot and the
  committed slot.
- Acceptance: an automated browser test does 20 quick drops (no dwell) with
  live updates running. All 20 land in the slot the line showed. The owner
  then checks it visually.

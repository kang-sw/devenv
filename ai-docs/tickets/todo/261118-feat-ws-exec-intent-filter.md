---
title: "ws/exec 의도 필터: 실행 전 경량 모델이 명령을 선언된 의도와 대조 검증"
blocked-by: 261117-feat-ws-exec-tools
related:
  261117-feat-ws-exec-tools: prerequisite; the filter sits on its daemon-owned command path, and its detailed design starts only after that ticket is implemented
---

# ws/exec intent filter: a lightweight model checks a command against its declared intent before it runs

## Background

Owner direction (2026-11-18, whiteboard item (103), answered b): instead of a
static pattern guard on shell commands run through the ws/exec tools, a later
stage adds an LLM filter. Before a command runs, a lightweight model judges
"does this script do what its declared intent says?" - an auto mode for our own
code.

Static block lists miss variants (for example an escaping mistake that turns a
narrow delete into a broad one); a judgment against the declared intent catches
the mismatch itself. Shell syntax can never be parsed perfectly, so the filter
can be bypassed: it is a mistake guard, not a security boundary.

The owner chose a separate ticket over a follow-up phase inside
261117-feat-ws-exec-tools, keeping that ticket unchanged and linking it here
as the prerequisite.

## Constraints

- Detailed design is TBD and is discussed with the owner only after
  261117-feat-ws-exec-tools is implemented. Open topics named by the owner:
  which model to use, a one-shot harness call versus a dedicated decision
  model, and the provider (for example OpenRouter). This ticket settles none
  of them; promotion to `ready/` requires that discussion first.

## Phases

### Phase 1: Intent filter on the ws/exec command path

Scope and mechanism are TBD (see `## Constraints`); this phase is a
placeholder until the post-exec-tools design discussion settles them.

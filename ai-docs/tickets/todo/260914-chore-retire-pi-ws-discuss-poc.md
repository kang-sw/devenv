---
title: Retire the Pi ws-discuss proof-of-concept command
sage-review-design: skipped
sage-review-completeness: skipped
---

# Retire the Pi ws-discuss proof-of-concept command

## Background

The Pi-local `/ws-discuss` slash command was introduced as a proof-of-concept acceptance probe for skill loading, the ws-mcp bridge, and persistent researcher spawning. Those mechanisms are now established independently, while the command remains a user-visible wrapper around the canonical `lead-discuss` skill and forces behavior that is not part of the canonical discussion flow.

Retire this completed proof-of-concept surface rather than maintaining it as a product feature. The explicit Sage-review posture for this hotfix is skipped for both design and completeness.

## Decisions

- Remove `/ws-discuss` as a Pi extension command and remove the implementation dedicated to constructing its kickoff prompt.
- Remove tests, current contract text, and active documentation or tickets whose live premise or sole purpose is the retired command.
- Discover the complete deletion closure from current repository and runtime evidence during implementation; this ticket does not prescribe an exhaustive path list.
- Preserve the canonical `lead-discuss` skill, the ws-mcp bridge, persistent `explore` spawning, and unrelated skill-list behavior.
- Preserve completed tickets, implementation plans, and Git history as historical evidence. Do not rewrite past records merely because they mention `/ws-discuss`.
- Prefer deletion over compatibility aliases or deprecation scaffolding. Git history is the recovery path if later evidence shows that removed material was still needed.

## Constraints

- This is a Pi-track-local change. Do not modify ws-mcp Go source or shared non-Pi workflow resources on this branch.
- Follow the repository manuals that match every implementation path discovered at runtime.
- Do not broaden the change into retirement of the canonical discussion workflow.

## Phases

### Phase 1: Remove the proof-of-concept surface and its live closure

Trace every current `/ws-discuss` registration, implementation, test, normative contract, and active supporting artifact. Delete the retired surface and clean up any remaining active text or fixtures so they describe the surviving canonical behavior without stale command-specific language.

Verify the Pi extension test suite and relevant package checks. Search the live source, tests, current contract documentation, and active ticket tree for remaining `/ws-discuss` references; any retained match must be demonstrably historical rather than an active contract or dependency.

---
title: Pi reviewer spawn requires an undisclosed exact-file write scope
---

# Pi reviewer spawn requires an undisclosed exact-file write scope

## Background

During repository dogfooding, `ws-agent-spawn` rejected a rendered `code-review-lite` dispatch with `ws-pi-agent: code reviewer requires exactly one file write scope`. Its advertised schema describes `write_scopes` as optional and explains general bounded grants, but does not state this reviewer-specific requirement. The Pi lead routing guide likewise shows spawning with an already-rendered prompt without that condition. The first dispatch therefore failed before allocation despite an explicitly named report destination. Reissuing the same dispatch with exactly one file scope for that destination succeeded. No source mutation or test execution occurred.

This is an interface discoverability finding, not evidence that the existing reviewer confinement rule is wrong. The observed failure occurred while dispatching the D-C2/UI-T5 static review on 2026-11-14; the successful reviewer is `edbd2da2-560d-4373-a078-93a83a503cf7`.

## Constraints

- Preserve existing reviewer write confinement unless a separately approved change explicitly alters it.
- This idea records the observed mismatch only; it grants no implementation, protocol change, new review gate or runtime action.

## Phases

### Phase 1: Make reviewer dispatch requirements discoverable

Verify the reviewer-specific spawn validation against the tool schema, rendered review prompts and Pi lead routing guidance. Propose the smallest consistent discoverability correction and regression coverage so a caller can construct the required bounded dispatch before its first attempt. Settle any observable behavior or protocol change before implementation.

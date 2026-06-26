---
title: "ws session-state tool ergonomics: enter.* derive + todo key surface gaps"
sage-review: recommended
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: motivating-feature
  260626-research-ws-todo-stack-nesting-model: out-of-scope-stack-semantics
---

# ws session-state tool ergonomics: enter.* derive + todo key surface gaps

## Problem

Two concrete usability gaps surfaced during the 260625 dogfood. The tools are
usable, but they make callers guess state-machine details that should be visible
or typed.

First, `ws.todo.list` renders item titles without item keys, while
`ws.todo.check`, `ws.todo.erase`, and `ws.todo.reorder` all require a key. During
dogfood, the key had to be guessed from internal knowledge instead of copied from
the list output.

Second, `ws.enter.implement(review_alloc: "single", ...)` produced a derived
todo item titled `Review (partitioned)`. That made the rendered checklist look
like it reflected a different implementation verdict than the one the caller
passed. The main fix is not to genericize titles; implementation verdict values
should become typed inputs that drive the derived checklist.

## Decisions

- Todo rendering must expose the stored key uniformly wherever the shared todo
  renderer is used: `- [ ] {key} Title` and `- [x] {key} Title`.
- `ws.todo.list` must use that format in both `summary` and `full` modes.
- Any existing output that uses the shared todo renderer, including session
  recovery summaries and enter/checkpoint summaries, should inherit the same
  `{key}` format from the shared renderer.
- Collapsed elision lines stay unchanged as `...`; they do not carry a fake key
  or checkbox marker.
- The rendered key is the stored `todoItem.Key`, not a documented slug-of-title
  derivation rule.
- Todo keys normalize to lowercase before storage and lookup.
- Allowed key characters are lowercase letters, digits, `.`, `_`, and `-`.
  Whitespace, braces, and any other punctuation are rejected. Uppercase input
  must not remain distinct after normalization.
- Duplicate detection is after normalization, so `Review` and `review` collide.
- `enter.implement` should move toward typed/enum verdict inputs and derive
  checklist titles and items from those typed values.
- Typed verdict inputs may affect item count, not only labels. For example,
  review allocation can eventually derive separate review items from an enum
  list rather than one hardcoded `Review (partitioned)` item.
- Do not make `Prep`, `Edit`, and `Review` generic as the primary fix; titles
  should remain informative once they are backed by typed values.

## Out of scope

Stack or replace semantics for nested `enter.*` flows are intentionally excluded
from this ticket. That design remains in
`260626-research-ws-todo-stack-nesting-model`.

## Phases

### Phase 1: Key-surfaced todo rendering and typed implement derivation

Completed behavior:

- The shared todo renderer prints visible keys as `- [ ] {key} Title` /
  `- [x] {key} Title`, with unchanged `...` elision lines.
- `ws.todo.list` exposes keys in `summary` and `full` output, so sibling
  mutation tools are callable without guessing.
- Todo key creation and lookup normalize lowercase and enforce the allowed
  character set.
- `enter.implement` checklist derivation no longer emits labels contradicted by
  its caller-supplied verdict inputs.
- The implementation introduces or prepares typed/enum verdict values for the
  derivation surface rather than copying freeform strings into titles.

Deferred scope:

- Multi-frame todo stack semantics.
- Full redesign of every `enter.*` schema beyond the implement verdict values
  needed to remove misleading checklist output.
- Historical migration of already persisted todo keys unless required by tests.

Verification boundary:

- Unit tests cover shared todo rendering, collapsed output, key normalization,
  invalid key rejection, duplicate-after-normalization rejection, and
  `enter.implement` derivation for at least one non-partitioned review verdict.
- Existing session-state tests remain green.

### Result (ea93b01c) - 2026-06-26

Implemented Phase 1 on branch `implement/260626-todo-enter-surface-gaps`.

- Shared todo rendering now exposes keys as `- [ ] {key} Title` /
  `- [x] {key} Title` and leaves collapsed `...` elision lines unchanged.
- Todo key creation and lookup normalize to lowercase, reject characters outside
  lowercase letters, digits, `.`, `_`, and `-`, and detect duplicates after
  normalization.
- `ws.enter.implement` derives Prep/Edit/Review labels from supplied verdict
  fields (`plan_depth`, `delegation`, `review_alloc`) so `review_alloc: "single"`
  renders `Review (single)` instead of `Review (partitioned)`.
- Updated `mcp-tools.md` from planned callout to implemented caller-visible
  contract.

Verification:

- `go test ./internal/mcp -count=1`
- `ws.spec_index.verify` -> `Spec index: ok`
- `git diff --check`

## Spec Impact

Addressed by implemented contract text in `ai-docs/spec/mcp-tools.md` under
`260625-session-state-tools`.

Expected caller-visible change: todo list output exposes `{key}` tokens, todo
key validation is specified, and `enter.implement` derivation is tied to typed
verdict values.

Contract-first spec: yes, addressed before promotion to `ready/`; implemented
contract text landed in `ea93b01c`.

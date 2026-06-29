---
title: Enter implement lacks direct-edit policy override
sage-review: required
---

# Enter implement lacks direct-edit policy override

## Surprise

During a direct current-workspace prompt-resource fix, the caller explicitly said
"Do not spawn/fork/delegate." `ws/enter.implement` still resolved
`delegation=delegated` for a narrow multi-file internal text change because the
schema has `explicit_delegation_request` but no corresponding direct-edit
override or policy field.

## Impact

The lead either violates the caller's no-delegation instruction or deviates from
the implementation verdict. This is especially visible for small generated-file
or mirrored-resource edits where multi-file scope does not imply a useful
implementer handoff.

## Follow-up

Research whether `ws/enter.implement` needs an explicit direct-edit policy input,
a narrower multi-file heuristic, or both.

## Decision (260629 sweep)

Design confirmed (260629 session): Add explicit_direct_edit_request field to implementScopeFactsInput as counterpart to explicit_delegation_request; yes value overrides all other facts to direct-edit. Implemented together with 260525-bug-lead-implement-delegation-pre-edit-guard.

## Result

Implemented: added ExplicitDirectEditRequest field to implementScopeFactsInput, normalizedImplementFacts, normalizeImplementFacts(), deriveImplementDelegation() (early-return before AND-condition block), parseImplementScopeFacts() validation, and implementConditions() output. Also extracted ExplicitDelegationRequest from the AND condition into its own early-return for yes.

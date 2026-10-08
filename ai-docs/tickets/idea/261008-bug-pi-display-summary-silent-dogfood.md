---
title: "Pi display-summary dogfood produces no visible summaries or accessible call diagnostics"
related:
  261007-feat-pi-display-summary: follow-up dogfood failure
---

# Pi display-summary dogfood produces no visible summaries or accessible call diagnostics

## Background

In a Pi lead session the user reports was opened after the feature landed,
setting `pi.display_summary_model` to catalog-listed
`openai-codex/gpt-6-luna` at session scope produced no visible summaries.
`config.tune` and `config.get` both returned the chosen model for session
`desktop-resident-skirmish`; effort remained the builtin `medium`.
Several `do-i-really-have-to-run-this-myself` and
`do-i-really-have-to-read-this-myself` results were generated as smoke-test rows.
The user still observed no change.

`agents-plugin-pi/src/display-summary.ts` catches completion exceptions and
returns silently for unresolved models, error/aborted responses, and empty
summary output. Its `log` getter exposes an in-memory conversation, not a
persisted request/error diagnostic. Current evidence cannot distinguish an
inactive summarizer, an unsuccessful provider call, or a rendering failure.
`src/display-summary-session.ts` gates creation on `ctx.mode === "tui"`
and an absent spawn role; whether the live runtime satisfies that gate needs
verification. No root cause is established yet.

## Phases

### Phase 1: Diagnose the missing live summaries

Underspecified idea capture only. Recover the actual runtime failure point and
reproduce the symptom before settling an implementation plan. Diagnostic
visibility and its storage/privacy policy remain unchosen; this ticket does
not authorize a logging design or changes to the cosmetic failure policy.

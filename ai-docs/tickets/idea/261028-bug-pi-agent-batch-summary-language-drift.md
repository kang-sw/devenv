---
title: "Investigate Japanese agent batch summaries with Korean language configured"
---

# Investigate Japanese agent batch summaries with Korean language configured

## Background

During a live Pi agent-widget smoke test, the user reported Japanese summaries after the root lead enabled `pi.agent_widget_batch_summary_enabled=true` at session scope. Runtime `config.get` confirmed `workflow.lang=한국어` at global scope. `pi.display_summary_style` also explicitly requested polite Korean, and the configured summary model was `openai-codex/gpt-6-luna`.

The visible Japanese output is user-reported; no screenshot or actual provider request/response was captured. The cause is not yet established. The catalog states that batch summaries reuse `workflow.lang`; a separate batch-language setting is not listed. This idea captures the dogfood surprise without selecting a fix or authorizing configuration or source changes.

## Phases

### Phase 1: Establish the language-drift cause

Unsettled idea: trace the effective language and request construction for batch summaries, distinguish generated text from fallback text, and obtain a minimal live reproduction before settling any implementation change. Check `agents-plugin-pi/src/batch-summary.ts` and its owner-lead wiring in `src/index.ts`; compare with display-summary language/style handling. Verification and implementation scope remain to be settled before promotion.

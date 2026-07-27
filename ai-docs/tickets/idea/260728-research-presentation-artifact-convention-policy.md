---
title: "Decide whether ai-docs/presentation/ artifacts follow convention changes or record what was presented"
related:
  260726-refactor-retire-spec-planned-marker-mechanism: first instance
---

# Decide whether ai-docs/presentation/ artifacts follow convention changes or record what was presented

## Background

The repo has no stated policy for whether material under `ai-docs/presentation/`
is a **record** of a delivered talk or a **living document** that tracks the
conventions it teaches. Every sweep scope this repo has written so far names
`ai-docs/spec/`, the embedded conventions under
`agents-plugin-tool/internal/wsdoc/conventions/`, `agents-plugin/rsrc/`, and the
three `WORKFLOW.md` files. `ai-docs/presentation/` is in none of them, so a
convention retirement leaves it untouched by construction rather than by
decision.

The question this ticket must answer: when a convention is retired, does a deck
that teaches it get edited, annotated, or left alone?

Both answers have a real cost, which is why this needs a decision rather than a
default:

- **Leave it alone** — the deck keeps instructing readers to use a removed
  feature. Unlike a `.done/` ticket body, which records completed work, a deck
  is teaching material written in live voice.
- **Edit it** — the artifact no longer records what was actually presented on
  its date, which is the property that makes a dated deck worth keeping at all.

A third option exists and is probably worth evaluating: leave the deck body
intact and add a dated erratum or a superseded-by banner, so the record survives
and the reader is warned.

## First instance

`260726-refactor-retire-spec-planned-marker-mechanism` retired the `🚧` spec
planned-marker mechanism across the spec corpus, the embedded conventions, the
playbooks, and both bootstrap lineages. Two sites in one deck still teach it in
live voice:

- `ai-docs/presentation/260513-wsflow-seminar.js:633` — a spec sample slide
  containing `"## 🚧 OAuth 연동 {#260512-oauth-link}\n"`.
- `ai-docs/presentation/260513-wsflow-seminar.js:640` — a glossary row
  `["🚧 마커", "구현 예정 항목 표시. 완료되면 마커 제거"]` ("marks planned items;
  remove the marker when done").

Scope of the instance, verified 2026-07-28: only that one deck is affected.
`260513-wsflow-seminar-v2.js`, `260616-wsflow-seminar-v3.js`, and
`ai-docs/presentation/slides/` carry zero `🚧`. The retirement ticket
deliberately did not edit the deck, because it has no authority to decide
whether presentation artifacts are records or living documents.

## What a decision needs to cover

- Which of the three postures applies (freeze / update / annotate), and whether
  it differs for the newest deck versus superseded versions.
- Whether the answer belongs in `ticket-conventions.md`, in
  `ai-docs/WORKFLOW.md`, or in a `ai-docs/presentation/README`.
- Whether future convention-retirement tickets must add `ai-docs/presentation/`
  to their sweep scope, and if so whether the sweep is blocking or advisory.
- Whether the decks in Korean stay Korean under the repo's English-content rule
  (they are human-facing presentation material, which the rule exempts) — worth
  stating explicitly so an editing posture does not accidentally trigger a
  translation pass.

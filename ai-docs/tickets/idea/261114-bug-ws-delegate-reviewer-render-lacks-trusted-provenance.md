---
title: "Worker-owned reviewer renders lack trusted provenance"
---

# Worker-owned reviewer renders lack trusted provenance

## Background

Two lead-rendered `delegate-implementer` workers, for Windows Go and launcher work, called `ws/playbook.render(name: code-reviewer)` for required independent review. Both received `ws-pi-agent: delegated playbook lacks trusted shipped provenance`. The Windows Go worker tried `playbook.read` with a temporary prompt, but its nested spawn then received `nested spawn requires trusted render provenance`. The lead instructed the workers not to retry or bypass the safeguards. Lead rendering of `code-reviewer` and spawning two reviewers succeeded.

A documented worker-owned independent review should have a trusted nested render/spawn path or an explicitly supported escalation path. Until this is investigated, the lead can supply independent reviewers; safeguards must remain enabled.

## Open Questions

- Is legitimate `delegate-implementer` render registration or provenance propagation missing, or is this a classification gap?
- What regression verification proves the worker-owned review path is trusted while preserving the existing refusal of untrusted nested renders and spawns?

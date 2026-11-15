---
title: Pinned Git evidence handoff for restricted Pi researchers
---

# Pinned Git evidence handoff for restricted Pi researchers

## Background

During Core-C1 static preparation, the Pi lead asked researcher `explore-1` (`5d43beef-f2f5-4261-ae47-faabda96cad9`) to cite code pinned to `aac39c69f381a82677f0ee6857e8f8fc5bc89322` and resolve one related ticket. The researcher reported that its available tools were read/grep/find/ls, without a Git-read command or `ws/tickets.query`. It could inspect a moving checkout but could not independently establish the requested source pin or obtain the required project-arc Git log.

This is a dogfood evidence-handoff gap, not proof that read restrictions are a security bug. No capability relaxation or harness change is selected.

## Outcome Ledger

### Verified Findings

- The lead received the capability-gap report above. The child's complete tool inventory and adapter implementation were not independently audited; its claimed tool absence remains attributed evidence.
- The lead resolved the related daemon ticket, provisioned a separate full checkout at the exact pin, and verified its clean HEAD through `ws/git.status`. The researcher was told to attribute that verification to the lead and discard unpinned moving-checkout evidence.
- The lead separately exported the pinned `git log --oneline --graph -50` into a cache artifact for filesystem-only reading. No source edit, test, build, provider or daemon operation was needed to unblock the investigation.
- Existing open-ticket discovery found a different mandatory-researcher-spawn/capability-ceiling issue; no matching pinned-Git handoff ticket was identified. This capture concerns evidence transport, not spawning at a delegation depth.

### Confirmed Decisions

None. This ticket captures the observed friction only and authorizes no tool grant, implementation or workflow change.

### Proposals

Non-authoritative: investigate a standard lead-supplied immutable-source/history handoff or a narrowly scoped read-only Git evidence primitive. Compare operational cost and security boundaries before selecting either.

### Open Questions

- Is the restricted researcher intentionally filesystem-only, and what source-pin/ticket-discovery contract should callers rely on?
- Can the existing prompt/render path disclose the restriction early enough to avoid an impossible assignment?
- What minimal regression proves that a researcher either receives verifiable pinned evidence or explicitly reports the gap without substituting current HEAD?

### Rejected Alternatives

None. Treating current checkout contents as the requested immutable commit was not used as a workaround.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - An observation that the exploration agent has no git read tool; an intended limit with no action.

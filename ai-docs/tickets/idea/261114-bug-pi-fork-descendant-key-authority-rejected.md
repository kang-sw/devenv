---
title: Pi fork rejects its freshly minted descendant key on read-only calls
---

# Pi fork rejects its freshly minted descendant key on read-only calls

## Background

A sibling native fork reported a separate authority surprise while preparing the common-interface working draft. This is distinct from missing parent identity and the live busy/prompt incident.

## Reported evidence

The root lead supplied this exact record on 2026-11-14:

1. Fork's native key `runner-undergo-prance` called `ws/ferrule` with root `/Users/kang-sw/.ws-worktrees/devenv/populace-skimmed-erasure` and parent_session_key `runner-undergo-prance`.
2. Ferrule returned descendant key `safehouse-football-think`.
3. `ws/git.status`, `ws/git.log(limit:50)`, and `ws/tickets.query(query:"common interface",limit:50)` using that returned key all failed with exactly:

```text
ws-pi-agent: session key is outside this agent's authority
```

4. Calls using the native fork key succeeded. No retry, bypass or root mutation occurred.

Expected caller behavior: a fork can use the descendant key returned by its own successful ferrule call for the scoped read-only operations it minted it to perform. Actual behavior: the adapter rejected all three calls before those operations completed. Whether that expectation should be implemented by adoption, a different minting contract or another mechanism is not settled here.

Source of record supplied and fully read by root: generated artifact `894e6d76-01-common-interface-working-draft-questions.md`, final section. This ticket preserves the root-relayed report, not an independent reproduction or a root-cause finding.

## Constraints

No harness fix, permission bypass, authority widening, protocol/API change or live probe is authorized. Preserve the distinction between returned-key usability and the separate parentless-fork issue.

## Phases

### Phase 1: Verify descendant-key admission against the recorded caller contract

Read the exact successful mint and rejected read-only calls, establish expected authority from existing contracts, and report any missing policy decision to MAIN before proposing a correction.

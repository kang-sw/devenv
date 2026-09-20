---
title: "Nested Pi RPC child stderr cascades unframed into the parent TUI"
related:
  260920-bug-pi-mailbox-wait-stderr-leaks-into-tui: same user-visible terminal contamination through a different producer and routing path
  260920-bug-pi-concurrent-skill-regeneration-race: observed native crash whose raw stderr exposed this transport behavior
---

# Nested Pi RPC child stderr cascades unframed into the parent TUI

## Background

A native filesystem crash in a nested Pi process appeared directly in the lead
TUI as an unprefixed `libc++abi` diagnostic. Pi's installed `RpcClient` spawns
children with piped streams, accumulates child stderr, and also writes each chunk
directly to `process.stderr`. Because workers can spawn their own RPC children,
raw stderr can cascade across process levels:

```text
grandchild stderr -> worker RpcClient -> worker stderr -> lead RpcClient -> lead stderr -> TUI
```

RPC stdout remains reserved for JSONL events; the observed terminal text is raw
stderr contamination even if it visually appears in the same TUI output area.
This path is separate from `MailboxWaiterDeps.onError` and the ready mailbox
stderr-leak ticket.

## Open Questions

- Should child stderr become a framed RPC diagnostic event, a captured artifact,
  or an adapter-owned bounded diagnostic sink?
- Which diagnostics remain immediately human-visible, and where should they be
  rendered without corrupting the interactive TUI?
- Does the upstream Pi `RpcClient` need the fix, or can the adapter contain the
  behavior without forking or patching installed runtime code?
- How should repeated stderr across recursive levels avoid duplication while
  retaining the originating agent identity?

## Phases

### Phase 1: Contain and attribute nested RPC stderr

Reproduce stderr emission from a grandchild and replace unframed recursive
`process.stderr` forwarding with a transport and presentation path that retains
the originating agent identity without contaminating the TUI.

Verification must cover direct-child and grandchild stderr, prove that each
diagnostic is surfaced at most once with origin context, preserve JSONL stdout
integrity, and demonstrate that native crash output no longer appears as raw TUI
text.

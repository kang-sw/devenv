---
title: agenda blob keys are not enumerable; add agenda_list or clear-all
---

# agenda blob keys are not enumerable; add agenda_list or clear-all

## Context

Found during a v0.31.1 dogfooding pass. Each of `enter_proceed`,
`enter_implement`, `enter_sprint`, and `enter_salvage` plants an agenda blob
under its own implicit key. There is no way to enumerate which agenda keys
currently exist for a session. Clearing them required guessing the key names
(`proceed`/`implement`/`sprint`/`salvage`) by reading tool descriptions rather
than querying state directly. An agent that skims descriptions, or that
doesn't know all four `enter_*` variants exist, can easily orphan a blob with
no way to discover or clean it up later.

## Suggestion

Add an `agenda_list` tool to enumerate current agenda keys (and ideally a
short summary of each blob), and/or extend `agenda_clear` with an `all: true`
option to clear every agenda blob for the session without needing to name
each key individually.

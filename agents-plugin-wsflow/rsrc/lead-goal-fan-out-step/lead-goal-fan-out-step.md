---
kind: print
delegates: true
---
# Goal Fan-Out Step

A batch-parallel `{{.SkillNamespace}}:lead-goal-step`. Its full contract — the goal-run posture, the three terminal states, `goal/*` staging, blocker-recording before yield, the "one step is not a finished goal" continuation, and conserve-context delegation — is delivered verbatim in the `<playbook name="lead-goal-step">` block appended below and governs unchanged. This overlay adds exactly one difference: when selection finds **two or more mutually-independent** advanceable tickets and recursive native dispatch is available, it advances them in parallel — one worktree-isolated mini-lead per ticket — instead of one at a time. When that condition does not hold, it simply *is* the appended Goal Step, run serially for this cycle. Read this overlay first, then the base block. Throughout, `<your key>` is the lead's own session key.

**Degenerate to serial when you cannot fan out.** Fanning out needs recursive native dispatch: each mini-lead itself spawns its own implementer and reviewer. It is known-available on Claude Code, where mini-leads must be spawned in an `Agent`-tool-retaining form (not Explore or Plan). If you cannot fan out this cycle — fewer than two independent tickets, or a mini-lead's first nested spawn is rejected — do not abort the goal run: remove any worktrees and branches you already created and run the appended Goal Step's ordinary path, dispatching one ticket to `{{.SkillNamespace}}:lead-proceed` yourself (a direct lead→implementer/reviewer dispatch, which needs no recursion). There is no degraded juggling mode between the two.

**Select the batch.** Where the base selects one advanceable ticket, select a mutually-independent set instead: disjoint edit surface, no `related:`/`parent:` ordering between them, each a self-contained contract (an open design or binding decision routes to `{{.SkillNamespace}}:lead-discuss`), and **excluding any ticket already dispatched this run and not yet merged** — a mini-lead's ticket status move lives on its own branch until merge, so the parent's `ready/` still lists in-flight tickets; read the board below to exclude them. Cap the batch to what you can juggle and the host can run, and prefer a smaller clean batch over a larger contended one. With fewer than two independent tickets, take the serial degenerate path above. Let `<stem>` be each pinned ticket's stem.

**Dispatch each worker.** Create a worktree off the parent with a fresh `impl/<stem>` branch, then mint that worker's key — `{{.McpNamespace}}/ferrule(root: <worktree>, capability: "lead", parent_session_key: <your key>)` returns `<worker key>` (a `lead` key, required because the worker runs `lead-proceed`). Native-spawn a background mini-lead (in an `Agent`-retaining form, at a lead-capable tier) with the task below; it needs nothing about its parent:

```text
You are in <worktree> on branch impl/<stem>.
1. Call {{.McpNamespace}}/workflow_manual(session_key: "<worker key>").
2. Call {{.McpNamespace}}/playbook.print(name: "lead-proceed", session_key: "<worker key>")
   and execute it on ticket <stem>: implement and review to a committed impl/<stem>
   branch, editing any ai-docs you touch (spec, _index.md, ticket Result) directly —
   you are the document owner for this ticket.
Stop at lead-proceed's merge gate — do not merge. Report your result there and stay
available; do not end your turn before that gate.
```

Then seed the board: `{{.McpNamespace}}/session.note(session_key: <your key>, child_session_key: <worker key>, text: "<stem>: dispatched")`.

**Collect and merge, one at a time.** The host signals when a worker stops at its merge gate. On each such signal, dispatch a **merge subagent** in your own worktree — the only checkout of the parent branch, so merges are physically serial. It reconciles from git evidence (expected commits present, the worker's own review clean), serial-merges `impl/<stem>` into the parent, resolves the `_index.md`/spec conflicts that overlapping doc-owners produce, and returns only a compact verdict (merge hash or blocker). For a worker whose handle you lost (e.g. after compaction) but whose branch exists, judge it from commits plus verification rather than resuming it. Advance each worker's board note to `merged` (or `blocked`) as it lands, so the next selection stops excluding it.

**When the batch settles, resume the base terminal check.** Once every dispatched worker has merged or recorded a blocker onto its own ticket, delegate one aggregate verification pass over the merged parent, then hand back to the appended Goal Step's selection-and-terminal logic unchanged: a fresh independent batch pulls another fan-out cycle, a lone advanceable ticket takes the serial path, and an empty or all-blocked `ready/` reaches the base's terminal states. End a productive cycle by naming that next selection, never with a wrap-up that reads as goal-complete.

**Board.** Keep a one-line note per worker with a fixed shape — `{{.McpNamespace}}/session.note(session_key: <your key>, child_session_key: <worker key>, text: "<stem>: <state>")`, where `<state>` starts at `dispatched` and you advance it to `merged` (or `blocked`) as the worker resolves — and read `{{.McpNamespace}}/session.children(session_key: <your key>)` to re-find workers after your own context compaction. The leading `<stem>` and state word are what make the board the in-flight ledger: the next selection excludes exactly the stems whose note is still `dispatched`. It is a self-reported scratchpad, not completion tracking (the host signal above is that) — keep it current as workers progress.

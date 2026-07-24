---
kind: print
delegates: true
---
# Fan-Out Worktree

Advance several **mutually independent** `ready/` tickets in parallel — one worktree-isolated mini-lead per ticket — keeping implementation, review, and merge noise out of the lead window. The lead holds only the shared-git spine (selection and one final verification) and a per-worker scratchpad; everything heavy is delegated. Throughout, `<your key>` is the lead's own session key.

**Precondition.** This posture needs recursive native subagent dispatch: each worker itself spawns its own implementer and reviewer. It is known-available on Claude Code, where workers must be spawned in an `Agent`-tool-retaining form (not Explore or Plan). If a worker's first nested spawn is rejected, remove any worktrees and branches already created and fall back to `{{.SkillNamespace}}:lead-goal-step`; there is no degraded single-level mode.

**Select the batch.** Pin one `ready/` ticket per worker, taking only tickets that are mutually independent this round: disjoint edit surface, no `related:`/`parent:` ordering between them, and a self-contained contract (an open design or binding decision routes to `{{.SkillNamespace}}:lead-discuss` instead). With fewer than two independent tickets, just run `{{.SkillNamespace}}:lead-goal-step` serially. Prefer a smaller clean batch over a larger contended one, and cap concurrency to what you can juggle and the host can run. Let `<stem>` be each pinned ticket's stem.

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

**Collect and merge, one at a time.** The host signals when a worker stops at its merge gate. On each such signal, dispatch a **merge subagent** in your own worktree — the only checkout of the parent branch, so merges are physically serial. It reconciles from git evidence (expected commits present, the worker's own review clean), serial-merges `impl/<stem>` into the parent, resolves the `_index.md`/spec conflicts that overlapping doc-owners produce, and returns only a compact verdict (merge hash or blocker). Repeat until every batch worker has merged; for a worker whose handle you lost (e.g. after compaction) but whose branch exists, judge it from commits plus verification rather than resuming it. Then delegate one aggregate verification pass over the merged parent and report per ticket.

**Board.** Keep a one-line `{{.McpNamespace}}/session.note(session_key: <your key>, child_session_key: <worker key>, text: ...)` per worker and read `{{.McpNamespace}}/session.children(session_key: <your key>)` to re-find workers after your own context compaction. The board is a self-reported scratchpad, not completion tracking (that is the host signal above) — its worth is surviving compaction, so keep it current as workers progress.

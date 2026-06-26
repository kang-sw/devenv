---
name: lead-revive
description: Post-compaction recovery. If this session was compacted or continued, invoke this BEFORE any other ws lead skill, passing the session_key preserved in the compaction summary, to restore agenda/todo state and reload the workflow primitives.
---

# Revive

Recover your ws `session_key` from the compaction summary, then call
`ws/workflow_manual(session_key: <recovered key>)` and execute the returned
reference inline. If no key is recoverable (genuinely fresh start), call
`ws/workflow_manual(session_key: "obsidian-latch")` to bootstrap.

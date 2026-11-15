# Topic default state and update-summary ticket preparation

## Authority

MAIN 2026-11-15T02:39:25Z relays the owner's request: every newly added topic defaults to discussing, and whiteboard updates return short applied-operation summaries including the assigned default state. MAIN 03:49:38Z requires all independent lanes to start concurrently in their own Pi worker worktrees; this documentation lane has no code dependency on settings implementation. Documentation capture and later readiness only, no product implementation.

## Settled contract

- An explicit wb:state in the same batch overrides the new-topic default.
- Existing topics are not backfilled.
- Existing ids:, full: and embed: response lines remain byte-compatible. Append new per-topic summary lines, preserving the existing lines and their values, e.g. topic added: <stem> (state: discussing, default) and topic updated: <stem>.
- Existing refusal output already lists problems and remains unchanged. Do not invent a different error protocol under the success-summary request.
- Expected compatibility is T0. Ground that claim by checking actual response consumers, including Pi/Codex adapters and tests, for strict parsing.
- Relate the work to completed 261115-feat-ws-whiteboard-topic-state-badge and todo 261115-feat-ws-revive-topic-state-summaries. Read the latter's absent-state wording and distinguish legacy missing state (still none) from defaulted new topics; do not silently backfill or change another ticket's owner-approved behavior.
- Any shipped whiteboard playbook result documentation must follow skill-authoring, shipped-surface boundaries and wsflow mirroring. Read all matching implementation manuals before authoring their required constraints.

## Assignment

Populate the existing empty todo stub 261115-feat-ws-whiteboard-topic-default-and-update-summaries with a recoverable English contract, phases, constraints, source-grounded implementation seams, relations and verification expectations. Use the feat template; satisfy the content and intent checklist, especially literal output examples, rejected alternatives and confirmed boundaries. Do not assert an unconfirmed ordering, summary cardinality or definition of updated operations as settled; flag genuine mechanism/policy gaps unnumbered for MAIN through the lead.

Inspect landed source in the worktree (base bffec047c) and only relevant related tickets. This is documentation preparation, not the independent fact-population gate. Do not run tests/builds, browser or live probes, modify shipped source, synchronize branches, or introduce a new API/store design. No daemon/provider/download/port29417/process signal action. No forks. Edit only the new ticket and this reserved plan; no unrelated ticket changes.

Return the prepared ticket uncommitted with concise source citations, relations, consumer-parser findings and any genuine gaps. Lead owns fact population, design/completeness review, ready move and the documentation commit. No source implementation, stamp, status change, extra review or commit during this preparation. Raw evidence stays /tmp or cache, not ai-docs.

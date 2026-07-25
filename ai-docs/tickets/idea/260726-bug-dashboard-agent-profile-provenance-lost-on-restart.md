---
title: Re-adopted agent terminals permanently lose profile provenance after a daemon restart
related:
  260725-feat-dashboard-pty-agent-attention-notification: found-during (Phase 2 adopt-arm CONTRACT)
  260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers: relates (registry schema constraint is why the obvious fix is closed off)
related-mental-model:
  - ws-web-dashboard
---

# Re-adopted agent terminals permanently lose profile provenance after a daemon restart

## Background

Found 2026-07-26 during Phase 2 of `260725-feat-dashboard-pty-agent-attention-notification`,
which added `profile_id: Option<String>` provenance to `TerminalSession` /
`TerminalSessionView` (in-memory only) to record which vendor profile (e.g.
`claude`) produced a terminal.

That provenance cannot be persisted: `TerminalRegistryEntry`
(`ws-dashboard/crates/daemon/src/terminal_registry_file.rs`) is under a hard
no-new-field constraint captured in
`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers` —
and the same registry file is what the adopt path reads on daemon restart.

`reconcile_entry`'s adopt arm
(`ws-dashboard/crates/daemon/src/terminal.rs:241-274`) reconstructs re-adopted
sessions via `TerminalSession::from_connection(...)` (call at line 245),
passing `None` for the profile id. There is a CONTRACT comment at that call
site (`terminal.rs:254-272`) spelling out the consequence in detail.

## Failure mode

After a daemon restart, `boot_reconcile` re-adopts every still-live terminal
it can reach. For a terminal that was running under a resolved vendor profile
(e.g. `claude`) before the restart, the re-adopted session permanently reports
`profileId: null` — the profile is never re-derived, because nothing about
the live process re-announces which profile spawned it.

## Why this is worth a ticket, not just a comment

This looks similar to the restart-loss the parent ticket already accepts for
turn state, but it is not the same shape and a reader who pattern-matches the
two will wrongly conclude this one is harmless too:

- Turn state SELF-CORRECTS on restart: adoption defaults to idle, and the
  next hook event fixes it.
- Profile provenance has NO correcting signal at all. Nothing ever
  re-establishes it, so the loss is permanent for that terminal's remaining
  lifetime, not transient.

## Why it matters

Phase 7 of the parent ticket (nav-row presentation) needs profile provenance
to tell an agent terminal from a shell terminal, so an agent terminal counts
in the AGENT counter only and never also in the plain terminal count (see
`260725-feat-dashboard-pty-agent-attention-notification` Phase 7 body, "An
agent terminal counts in the AGENT counter only... The carrier is the profile
recorded on the pane in Phase 2"). After a daemon restart, any re-adopted
agent terminal loses that carrier and silently falls out of the AGENT
counter — an under-count, not a visible error. Silent under-counting in a
status indicator is worse than a visible error, because nothing prompts
anyone to look.

## Candidate directions (not decided)

- Sniff the re-adopted process's own argv (via OS process-inspection APIs) to
  re-derive the profile.
- A sidecar provenance file outside the registry schema, avoiding the
  constrained `TerminalRegistryEntry` struct entirely.
- Revisit the registry-schema constraint itself:
  `260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`
  already names the `Option<T>` + `#[serde(default)]` shape that would be
  required to add fields to the registry safely.
- Have Phase 7 accept and display the uncertainty (e.g. an explicit "unknown"
  state) instead of silently under-counting.

## Relations

`found-during` `260725-feat-dashboard-pty-agent-attention-notification`
(Phase 2). Relates to
`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`,
which is the reason the obvious fix (persist the field) is closed off.

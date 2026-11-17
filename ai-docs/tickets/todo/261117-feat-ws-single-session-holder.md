---
title: "Single PC-wide session holder: one holder process and one daemon connection for many children and exec jobs"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-exec-tools: "this ticket is its prerequisite (exec jobs run under the single holder)"
  261116-research-ws-holder-embeddable-python-v3: "holder runtime background; not edited from here"
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch constraint; not edited from here"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
---

# Single PC-wide session holder

## Background

Today there is one holder per child (`agents-plugin-tool/internal/wsagent/holder.go`,
the doc comment near the top): one Python interpreter and one socket per agent.
Owner decisions (17)=a and (19)=a make exec a prerequisite-ordered follow-on:
`261117-feat-ws-exec-tools` needs a holder that owns many children, including
exec jobs, and that outlives the daemon.

Facts about today's holder (verified 2026-11-17): the holder listens on loopback
`127.0.0.1:0` (`agents-plugin-tool/internal/wsholder/holder.py:849`) and records
its port and token in `holders/<id>/descriptor.json`; the daemon dials it
(`wsholder/client.go:139`) and re-attaches after a daemon replacement by
scanning the holders dir (`wsholder/manager.go:264-316`); the hello carries
`protocol` and both sides accept N and N-1 (`wsholder/holder.go:55-64`); holder
scripts are staged by hash (`wsholder/holder.go:139`).

## Decisions

1. **One PC-wide Python holder process, one daemon connection, many children
   (17)=a, (19)=a.** The children are conversations and exec jobs.
2. **Exec output lives in holder memory only, no files (19)=a.** Today `execjob`
   writes stdout/stderr/combined per job. The daemon reads output from the holder
   over the connection for `exec.tail` and for completion reports. The holder
   outlives the daemon, so output survives a daemon replacement; if the holder
   dies the jobs die too, so files would buy nothing. Only the completion summary
   (exit code plus output tail) persists, through the completion mail.
3. **Per-job output ring with a cap.** On overflow keep a head slice plus the
   tail. The size (head and tail) is decided in this ticket (record the
   reasoning in the phase Result).
4. **Version coexistence ((56)=ok, owner 2026-11-17 19:25).** A new-version
   holder is started for new children while an old-version holder keeps serving
   its existing children over the N-1 protocol and exits when its child count
   reaches zero. "One PC-wide holder" therefore means one per holder protocol
   version. The connection direction stays: the holder listens, the daemon
   dials. Rejected: replace only at zero children (forces closing every
   conversation to update the holder); reversing the direction so the holder
   dials the daemon (the listening holder already outlives the daemon
   naturally).
5. **Mitigations are constraints, not options** (see Constraints).

## Risks (named once)

- One holder death kills all lead conversations and exec jobs on the PC.
- Two holder versions may run side by side during a transition; the daemon must
  keep N-1 protocol support until the old holder drains.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Blast-radius mitigations (owner-confirmed with (17)=a, (19)=a):
  - the holder is minimal: pipe/PTY ownership, the output ring and the
    multiplexed connection only;
  - the holder protocol is versioned, and the new daemon speaks the old holder
    protocol;
  - an old-version holder is never force-replaced; it drains under version
    coexistence (Decision 4).
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; interactive paths never wait on file I/O. Exec output
  stays in the holder memory ring; restore needs deltas only.
- Holder protocol is a cross-version surface: a version bump of the holder
  protocol states its tier and cites source lines.

## Compatibility

The holder protocol is versioned and the new daemon speaks the old holder
protocol (owner-confirmed constraint). The exact tier of each cross-version
surface (holder protocol, any restore-store shape) is judged in Phase 1 with
source citations per `ai-docs/manuals/release-compatibility.md`, and recorded in
the Phase 1 Result before release. A T3 boundary requires the owner's explicit
approval; T4 is never shipped.

## Phases

### Phase 1: Single holder process, multiplexed connection, versioned protocol

Move from one holder per child to one PC-wide holder with a single daemon
connection and many children (conversations and exec jobs); version the holder
protocol with the new daemon speaking the old protocol; run an old-version and a
new-version holder side by side, placing new children on the new holder and
letting the old holder exit at zero children; add the per-job output ring with head+tail
overflow handling (size decided here and recorded); expose the output read over
the connection for later `exec.tail` and completion reports.

Deferred: the `exec.*` tool surface (`261117-feat-ws-exec-tools`).

Verification: Go tests on remote w5090 covering multiplexing of several children,
new-daemon-with-old-holder protocol compatibility, coexistence of an old-version
and a new-version holder with new children placed on the new holder and the old
holder exiting at zero children, ring overflow behavior (head+tail kept), and a daemon
replacement with the holder surviving and the output still readable; syscall/s
measurement showing no per-job file writes.

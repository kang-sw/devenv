# Docs: performance golden rules (AGENTS.md core lines + ai-docs/manuals/performance.md)

## Context

The owner answered (1347)=a on 2026-11-18 at 11:22 KST. The 2026-11-16/17
daemon performance overhaul (file-system load under heavy antivirus such as
AhnLab V3) produced design principles that future changes must follow. The
mailbox memory-first store on track/agent-v2 (ticket
261117-feat-ws-mailbox-memory-first-store; commits e973b72b4, 10f95a154,
898f0a872) is the reference design.

The owner's standing rule (clone note, 2026-11-17):
- Target users are corporate PCs with heavy security software, slower than the
  owner's PC. AV cost is triggered by every file touch; each syscall costs
  10-100x more.
- Minimize file touches at the root. Files exist only for backup and
  restoration.
- Flush only what restoration needs. Write only deltas (append, not rewrite).
  Never rewrite unchanged content; no heartbeat rewrites.
- No per-event O(N) rereads. Keep live state in memory.
- Interactive paths (chat send -> delivery -> processing start, page load)
  must not wait on file I/O.
- Judge by measured syscalls/s and delivery latency on the owner's PC, not
  fixture CPU or Linux.

The mailbox reference design, as measured (before = develop, after =
agent-v2):
- Reads: before, every call locked the file and reread all of it. After, reads
  come from memory only, with zero disk access.
- Writes: before, each send, recv and heartbeat rewrote the whole JSON and
  renamed it. After, one change is one JSONL line append (a single write()).
- Expiry: before, one file per record, with a directory listing and deletes
  on each add. After, expiry is in memory, and on disk old 1 MiB segment files
  (a ring of 10) are deleted whole.
- Recovery: each segment's first line is a full snapshot, so recovery replays
  only the newest segment.
- Handles and locks are held for the daemon's lifetime instead of
  open/lock/close per call.
- Measured: 99.8-106 file operations per send before; 1 append after, plus
  about 12 at segment rotation.
- A graceful close writes the legacy files once, for downgrade compatibility.

Owner-approved rule draft v2:
1. Memory is the source of truth; files are for recovery. Reads come from
   memory only; files are read once at startup.
2. Changes are delta appends. No whole-file rewrites. Write nothing when
   nothing changed.
3. Retention deletes whole files. Never edit a live file. Make each file
   self-sufficient for recovery (first-line snapshot) so recovery reads one
   file.
4. Keep handles and locks open. No per-call open/lock/close, re-read or stat.
5. Interactive paths do not wait on file I/O. Instead of fsync, recovery trims
   a torn last line.
6. Minimize process spawns.
7. Decide by measurement: count file operations per action before and after,
   on a Windows PC with heavy security software as the baseline.
8. Exceptions need the owner's approval.

## Outcome

1. New `ai-docs/manuals/performance.md`.
   - It opens with the one-line `summary:` frontmatter that every manual has
     (see the other files in ai-docs/manuals/).
   - It holds the 8 rules, each with a short "why" and the concrete pattern.
   - It holds the mailbox design as the worked example, with the measured
     before/after numbers and commit ids.
   - It holds a short section on how to measure, kept project-neutral:
     operations per action, a heavy-AV Windows PC as the baseline, and
     delivery latency.
   - Keep it compact and positive; follow
     `ai-docs/manuals/skill-authoring.md`'s reader-model guidance for
     frequently reread docs.
2. `AGENTS.md`.
   - Add 3-4 core lines in the most fitting existing section (likely
     `## Code Standards` or `## Architecture Rules`) that point to the manual.
   - Add an `### Implementation Conventions` table row for
     `agents-plugin-tool/` -> `ai-docs/manuals/performance.md`. There is
     already an `agents-plugin-tool/internal/` row for release-compatibility;
     add a new row rather than merging them.
   - Match the neighbouring wrap width (about 79 characters).
3. The manual is repo-local; it is not shipped text. Do not edit anything under
   agents-plugin*/.

## Constraints

- Docs only; no code. Do not run Go, the ws exes or tests on this PC.
- English. No machine-local details (IPs, hostnames, credentials).
- Commit on `impl/develop/perf-golden-rules` with your own model's
  Co-Authored-By trailer, together with this plan file. Do not push.

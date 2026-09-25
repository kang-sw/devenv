---
title: Windows remote-command timeout leaves git and ssh children running
related:
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: introduced the bounded read/write timeouts whose kill path this concerns
  260925-chore-ship-preflight-misses-ci-matrix: the CI gap that let the test-side symptom reach a release run
---

# Windows remote-command timeout leaves git and ssh children running

## Background

`wsindex.ExecRunner.Run` (`agents-plugin-tool/internal/wsindex/git.go:105-113`)
runs git under a context deadline. It sets `WaitDelay = 300ms` and calls
`configureKill`:

- **POSIX** (`proc_unix.go`): git runs in its own process group, and the
  timeout SIGKILLs that whole group, including the ssh transport.
- **Windows** (`proc_windows.go`): `configureKill` does nothing. The timeout
  kills only the process Go started. `WaitDelay` then returns control without
  killing anything else.

Evidence from the v0.46.19 fix-forward (`ws-mcp CI` run 36079288933):

- **Symptom.** The hanging-ssh test transport outlived the timed-out git call.
  It held the test clone as its working directory, so `t.TempDir` cleanup
  failed with "The process cannot access the file because it is being used
  by another process."
- **Likely cause (inferred, not reproduced).** The `git` on PATH is Git for
  Windows' `git.exe` redirector. Killing it leaves the real mingw64 `git.exe`
  alive, and that process keeps the transport's stdin open.
- **Tests fixed, production not.** The tests were fixed with a stop-file
  harness (merge 30b41a3f). Production behaviour is unchanged.

Production consequence:

- **What survives.** On a hanging origin, a timed-out read or write on Windows
  can leave the real `git.exe` and `ssh.exe` running. Both keep the clone as
  their working directory, which blocks deleting the directory and removing
  worktrees.
- **How long.** `batchSSHCommand` sets only `ConnectTimeout=5` (`git.go:190`),
  with no `ServerAliveInterval`. A stalled but connected session can live on
  indefinitely.

## Open questions

- **Kill mechanism.** Options: a Windows `cmd.Cancel` that runs `taskkill /T
  /F /PID <pid>` (spawns a process, but is simple), or a Job Object with
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`. The Job Object has a race between
  start and assignment unless the process starts suspended, which
  `os/exec` does not expose directly.
- **Liveness option.** Should `batchSSHCommand` also add `-o
  ServerAliveInterval` and `ServerAliveCountMax`, so a stalled ssh ends on its
  own on every OS?
- **Other runners.** Do other remote-calling paths (for example
  `internal/wsgit` or `execjob`) share the same single-process kill on
  Windows?
- **Verification.** How to verify on Windows without a local Windows host:
  a CI-only test that counts surviving processes after a timed-out call?

# Plan: dashboard daemon owns a hidden console (Windows hotfix)

## Problem

On Windows every child process the dashboard daemon starts (`git.exe`,
`ssh-keygen`, `claude`, `pi`, shells) flashes an empty console window. The
user reports a flood of them whenever the daemon does work.

Cause: `agents-plugin-tool/internal/wsdashboard/spawn_windows.go`
`startDetached` creates the daemon with `DETACHED_PROCESS`, so the daemon has
no console. Windows allocates a new, visible console for every
console-subsystem child of a console-less parent. No `exec.Command` site in
the tree sets `CREATE_NO_WINDOW`.

## Change

In `startDetached`, replace `DETACHED_PROCESS` with `CREATE_NO_WINDOW`. Keep
`CREATE_NEW_PROCESS_GROUP`, `HideWindow: true`, and the existing
`CREATE_BREAKAWAY_FROM_JOB` attempt with its fallback. The daemon then owns its
own hidden console; console children inherit it and open no window, including
grandchildren (git-remote-https, ssh, credential helpers). The console is the
daemon's own, not the host terminal's, so closing the host terminal still does
not end the daemon. Update the doc comment to say why `CREATE_NO_WINDOW` and
not `DETACHED_PROCESS` (a trap comment at the site).

Rejected: adding `CREATE_NO_WINDOW` to each of the ~20 `exec.Command` sites -
larger diff, and it does not cover grandchildren started by non-Go tools.

## Verification

- `go build ./...` and `go vet ./internal/wsdashboard/` with `GOOS=windows`;
  the wsdashboard package tests on Linux.
- WSL interop probe (this machine is WSL2 on Windows): a throwaway Go program
  cross-compiled for Windows, kept out of the repository (scratch dir), that
  starts itself with the new flags (and, for contrast, the old
  `DETACHED_PROCESS` flags), has that child start a console grandchild
  (e.g. `cmd.exe /c` or a second mode of the probe), and the grandchild reports
  `GetConsoleWindow()` and `IsWindowVisible()`. Expected: new flags -> console
  window exists and is not visible; old flags -> grandchild got its own
  console (visible or newly allocated). Record the observed output in the
  commit's AI Context.

## Out of scope

Version bump and release - the lead ships 0.48.12 after this lands.

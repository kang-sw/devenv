---
title: "wsagent test hardcodes forward-slash path, fails on native Windows Go toolchain"
---

## Context

Discovered while dogfooding a native-Windows `go test ./...` run for
`agents-plugin-tool` (v0.31.0 ship pre-flight follow-up check). WSL/Linux
`go test` passes cleanly; running the same module under a Windows-native Go
toolchain (go1.26.3 windows/amd64) surfaces one genuine failure:

```
--- FAIL: TestBuildCodexInvocationUsesStdinPromptForFirstCall (agent_test.go:688)
codex args missing "model_instructions_file=\"C:\\Users\\...\\system.md\"":
got .../system.md with forward slashes instead
```

`internal/wsagent/agent_test.go:688` asserts a literal forward-slash path in
the expected `model_instructions_file=...` argument. The code under test
builds the path via `filepath.Join`/OS separator, which is backslash-joined
on Windows. The test is not OS-aware; it only ever passed because CI/dev runs
have so far been Linux/WSL-only.

## Suggested fix

Build the expected path in the test using `filepath.Join` (or an
OS-conditional string) instead of a hardcoded forward-slash literal, so the
assertion matches native path separators on both platforms.

## Verification note

Confirmed via a throwaway copy of `agents-plugin-tool` (plus the minimal
sibling `rsrc`/`ai-docs` assets its tests read at runtime) to a Windows-native
temp path (`/mnt/c/...`), then running `go test ./...` there through
`powershell.exe`. All other packages passed; this was the only failure. No
repo files were modified; the temp copy was deleted after the run.

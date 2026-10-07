---
title: "ws-mcp mailbox tests read the ambient WS_MAILBOX environment"
---

# ws-mcp mailbox tests read the ambient WS_MAILBOX environment

## Background

Dogfood, 2026-10-07, ws 0.46.33 ship pre-flight. `cd agents-plugin-tool && go
test ./...` failed in `internal/mcp` only:

```text
--- FAIL: TestMailboxInertByDefault
    mailbox_tools_test.go:57: machine mailbox store was created for a fully inert session: .../config/mailbox.json
--- FAIL: TestMailboxSelfAddressSurface
    mailbox_tools_test.go:296: WS_MAILBOX_AUTO identity not active: mcp.mailboxIdentity{Active:true, Name:"devenv", Scope:"machine", Auto:false}
```

The pre-flight shell had `WS_MAILBOX=devenv@machine` set (the operator's
interactive environment). The same tests pass with
`env -u WS_MAILBOX go test ./internal/mcp/ -run 'TestMailboxInertByDefault|TestMailboxSelfAddressSurface'`.

`setupMailboxTestEnv` (`internal/mcp/mailbox_tools_test.go`) isolates
`WS_CACHE_HOME`, `WS_CONFIG_HOME`, and `WS_RSRC_ROOT`, but not `WS_MAILBOX` or
`WS_MAILBOX_AUTO`. So any test that assumes "neither is set" depends on the
caller's shell. CI is unaffected because its environment has neither, but the
ship config's local `go test ./...` pre-flight fails for any operator who uses
the mailbox.

## Direction

Clear both variables in `setupMailboxTestEnv` with `t.Setenv(..., "")` or an
unset helper, and let the tests that need them set them explicitly. Check
other `internal/mcp` test helpers for the same ambient `WS_*` leak.

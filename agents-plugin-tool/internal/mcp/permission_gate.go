package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
)

// permission_gate.go enforces a spawned agent's authority on its ws tools.
// The runner puts the child's level in its attached ws-mcp's environment
// (wsagent.EnvPermissionLevel): a hosted HTTP session's hello env, or a
// stdio ws-mcp's process env; both reach getenv. Blank is no level, and
// workdir-write and full refuse nothing, so only read-only gates here.

// readOnlyRefusedTool reports a ws tool call of the agent loadout that
// changes the work directory, review ledger, or ticket board, which a
// read-only agent may not call. mailbox.send and non-bootstrap review.marker
// reads stay allowed.
func readOnlyRefusedTool(name string, args map[string]any) bool {
	switch name {
	case "git.commit", "tickets.create_empty", "tickets.acquire", "tickets.release", "tickets.close", "tickets.move", "review.stamp":
		return true
	case "review.marker":
		return boolArgument(args["bootstrap"])
	}
	return false
}

// permissionRefusal is why this server refuses tool for its permission
// level, "" when it does not.
func (s *Server) permissionRefusal(tool string, args map[string]any) string {
	level := strings.TrimSpace(s.getenv(wsagent.EnvPermissionLevel))
	if wsagent.PermissionLevel(level) != wsagent.PermissionReadOnly || !readOnlyRefusedTool(tool, args) {
		return ""
	}
	return fmt.Sprintf("%s: permission denied: this agent runs at authority %s, which the session that spawned it chose, and %s changes the work directory, review ledger, or ticket board. Do the read-only part of the task and put the change you would make in your report.", tool, level, tool)
}

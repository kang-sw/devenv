package wsagent

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// Backend attach adopts an existing worker process. It must not reconstruct
// launch-only settings, mint an MCP session, or send a new handshake.
func TestWorkerBackendsAttachToExistingHolder(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, authority, sandbox, policy string
		backend                          Backend
	}{
		{name: BackendClaude, backend: ClaudeBackend{Command: command}},
		{name: BackendCodex, authority: string(PermissionReadOnly), sandbox: "read-only", policy: "never", backend: CodexBackend{Command: command}},
		{name: BackendCodex, authority: string(PermissionWorkdirWrite), sandbox: "workspace-write", policy: "on-request", backend: CodexBackend{Command: command}},
		{name: BackendAgy, backend: AgyBackend{Command: command}},
	} {
		t.Run(tc.name+"/"+tc.authority, func(t *testing.T) {
			host := newFakeHolders()
			id := host.add(holderSession{Kind: holderKindAgentWorker, Backend: tc.name, ReplyID: "worker"})
			host.Scan()
			spec := StartSpec{
				Root: t.TempDir(), Dir: t.TempDir(), SessionID: "existing-session",
				Holder:          &holderChild{host: host, table: &holderTable{}, id: id},
				PermissionLevel: PermissionLevel(tc.authority),
			}
			if tc.name == BackendClaude {
				// The malformed MCP profile would reject a fresh launch. An
				// attach uses the already-running child's launch settings.
				spec.Profile.MCPServers = map[string]json.RawMessage{"broken": json.RawMessage("{")}
				if _, err := claudeArgs(spec); err == nil {
					t.Fatal("fixture profile must reject launch argument reconstruction")
				}
			}
			proc, err := tc.backend.Start(context.Background(), spec)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if !proc.(interface{ Detach() bool }).Detach() {
					t.Error("attached worker did not detach")
				}
			})
			if held, pid, ok := proc.(interface{ HolderInfo() (string, int, bool) }).HolderInfo(); !ok || held != id || pid != 0 {
				t.Fatalf("attached holder = %q, pid %d, held %v", held, pid, ok)
			}
			if ops, inputs := host.opList(), host.inputs(id, ""); len(ops) != 0 || len(inputs) != 0 {
				t.Fatalf("attach launched or sent to a process: ops %v, inputs %v", ops, inputs)
			}
			if p, ok := proc.(*codexProcess); ok {
				if p.mode != tc.sandbox || p.policy != tc.policy || !p.worker || p.m.readOnlyWorker != (tc.policy == "never") {
					t.Fatalf("persisted worker authority selected mode %q, policy %q, worker %v, read-only %v", p.mode, p.policy, p.worker, p.m.readOnlyWorker)
				}
			}
		})
	}
}

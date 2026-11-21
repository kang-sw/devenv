package wsagent

import "testing"

func TestPermissionLeadMapping(t *testing.T) {
	for _, tc := range []struct {
		mode  string
		level PermissionLevel
	}{
		{"plan", PermissionReadOnly}, {"bypassPermissions", PermissionFull},
		{"", PermissionWorkdirWrite}, {"auto", PermissionWorkdirWrite},
		{"default", PermissionWorkdirWrite}, {"acceptEdits", PermissionWorkdirWrite}, {"dontAsk", PermissionWorkdirWrite},
	} {
		if got := legacyLeadPermission(Record{Lead: true, Backend: BackendClaude, PermissionMode: tc.mode}); got != tc.level {
			t.Fatalf("%s = %s, want %s", tc.mode, got, tc.level)
		}
	}
	for mode, level := range map[string]PermissionLevel{
		"read-only": PermissionReadOnly, "workspace-write": PermissionWorkdirWrite, "danger-full-access": PermissionFull,
	} {
		if got := legacyLeadPermission(Record{Lead: true, Backend: BackendCodex, PermissionMode: mode}); got != level {
			t.Fatalf("codex %s = %s, want %s", mode, got, level)
		}
	}
	// Pin the contract independently: inverse projection accepts multiple native
	// modes for workdir-write and cannot establish the required launch flag.
	literalModes := map[PermissionLevel]string{PermissionReadOnly: "plan", PermissionWorkdirWrite: "acceptEdits", PermissionFull: "bypassPermissions"}
	codexModes := map[PermissionLevel]string{PermissionReadOnly: "read-only", PermissionWorkdirWrite: "workspace-write", PermissionFull: "danger-full-access"}
	for _, level := range LeadPermissionLevels() {
		if got := nativePermissionMode(BackendClaude, level); got != literalModes[level] {
			t.Fatalf("mapping %s = %s, want literal %s", level, got, literalModes[level])
		}
		if got := nativePermissionMode(BackendPi, level); got != "" {
			t.Fatalf("invented Pi mode %s", got)
		}
		if got := nativePermissionMode(BackendCodex, level); got != codexModes[level] {
			t.Fatalf("Codex mapping %s = %s, want literal %s", level, got, codexModes[level])
		}
		for _, active := range []string{"", "auto", "plan", "bypassPermissions"} {
			want := "advisory"
			if level == PermissionReadOnly && active == "plan" {
				want = "partial"
			}
			if got := permissionQuality(BackendClaude, level, active); got != want {
				t.Fatalf("quality %s/%s = %s", level, active, got)
			}
			if got := permissionQuality(BackendPi, level, active); got != "advisory" {
				t.Fatalf("Pi quality %s", got)
			}
		}
		for _, active := range []string{"", "read-only", "workspace-write", "danger-full-access", "read-only (approval never)"} {
			want := "advisory"
			if level == PermissionReadOnly && active == "read-only" {
				want = "partial"
			}
			if got := permissionQuality(BackendCodex, level, active); got != want {
				t.Fatalf("Codex quality %s/%s = %s", level, active, got)
			}
		}
	}
	if got := legacyLeadPermission(Record{Lead: true, Backend: BackendPi}); got != PermissionWorkdirWrite {
		t.Fatal(got)
	}
	if got := legacyLeadPermission(Record{Lead: true, Backend: BackendClaude, PermissionMode: "unknown"}); got != "" {
		t.Fatalf("unknown mode fabricated %s", got)
	}
	if got := legacyLeadPermission(Record{Lead: true, Backend: BackendCodex}); got != "" {
		t.Fatalf("omitted Codex mode fabricated %s", got)
	}
}

func TestPermissionLeadAuthority(t *testing.T) {
	for _, current := range LeadPermissionLevels() {
		for _, next := range LeadPermissionLevels() {
			for _, user := range []bool{false, true} {
				for _, ack := range []bool{false, true} {
					err := authorizePermission(current, next, user, ack)
					allowed := (next.rank() <= current.rank() || user) && (next != PermissionFull || (user && ack))
					if (err == nil) != allowed {
						t.Fatalf("%s -> %s user=%v ack=%v: %v", current, next, user, ack, err)
					}
				}
			}
		}
	}
	if authorizePermission(PermissionFull, "unknown", true, true) == nil {
		t.Fatal("invalid selection accepted")
	}
	if authorizePermission("", PermissionReadOnly, false, false) == nil {
		t.Fatal("unknown evidence granted agent authority")
	}
}

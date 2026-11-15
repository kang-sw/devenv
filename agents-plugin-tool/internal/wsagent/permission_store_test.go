package wsagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPermissionStoreOmitted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	level, explicit, err := readPermissionSelection(dir)
	if err != nil || explicit || level != "" {
		t.Fatalf("omitted = %q, %v, %v", level, explicit, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read wrote back: %v", err)
	}
}

func TestPermissionStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for _, level := range []PermissionLevel{PermissionFull, PermissionWorkdirWrite, PermissionReadOnly} {
		if err := writePermissionSelection(dir, level); err != nil {
			t.Fatal(err)
		}
		assertPermissionSelection(t, dir, level, false)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "permission.json"))
	if err != nil {
		t.Fatal(err)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || len(members) != 1 {
		t.Fatalf("unexpected stored fields: %s (%v)", raw, err)
	}
	path := filepath.Join(dir, "permission.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePermissionSelection(dir, PermissionFull); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("selection was modified in place rather than replaced")
	}
	assertPermissionSelection(t, dir, PermissionFull, false)
}

func TestPermissionStoreCorruption(t *testing.T) {
	for _, raw := range []string{"", "{", "null", "[]", "1", `"read-only"`, `{}`, `{"permission_level":null}`, `{"permission_level":false}`, `{"permission_level":1}`, `{"permission_level":""}`, `{"permission_level":"unknown"}`, `{"permission_level":"read-only"} trailing`} {
		t.Run(raw, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "permission.json")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			_, explicit, err := readPermissionSelection(dir)
			if err == nil || !explicit {
				t.Fatalf("corrupt selection fell back: explicit=%v, err=%v", explicit, err)
			}
			if err := writePermissionSelection(dir, PermissionReadOnly); err == nil {
				t.Fatal("write repaired corruption silently")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != raw {
				t.Fatalf("corruption overwritten: %q, %v", got, err)
			}
		})
	}
	for _, level := range []PermissionLevel{"", "invalid"} {
		dir := filepath.Join(t.TempDir(), "absent")
		if err := writePermissionSelection(dir, level); err == nil {
			t.Fatal("invalid new selection accepted")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("invalid choice wrote directory: %v", err)
		}
	}
}

func TestPermissionStoreReadError(t *testing.T) {
	for _, kind := range []string{"directory", "dangling-link", "private"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "permission.json")
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "dangling-link":
				if err := os.Symlink(filepath.Join(dir, "missing"), path); err != nil {
					t.Skip(err)
				}
			case "private":
				if err := os.WriteFile(path, []byte(`{"permission_level":"full"}`), 0); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(path, 0o600)
				if _, err := os.ReadFile(path); err == nil {
					t.Skip("filesystem permits reading mode 000 (e.g. root)")
				}
			}
			if _, explicit, err := readPermissionSelection(dir); err == nil || !explicit {
				t.Fatalf("unreadable selection fell back: %v, %v", explicit, err)
			}
			if err := writePermissionSelection(dir, PermissionReadOnly); err == nil {
				t.Fatal("overwrote unreadable selection")
			}
		})
	}
}

func assertPermissionSelection(t *testing.T, dir string, want PermissionLevel, unknown bool) {
	t.Helper()
	got, explicit, err := readPermissionSelection(dir)
	if err != nil || !explicit || got != want {
		t.Fatalf("reopen %s = %q, %v, %v; want %q", dir, got, explicit, err, want)
	}
	path := filepath.Join(dir, "permission.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("selection mode = %o", info.Mode().Perm())
	}
	if unknown {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(raw, &members); err != nil {
			t.Fatal(err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, members["future"]); err != nil {
			t.Fatal(err)
		}
		if compact.String() != `{"keep":[1,true,"x"]}` {
			t.Fatalf("unknown member lost: %s", raw)
		}
	}
}

const permissionLegacyCommit = "90eacda7e4956aded76cfc092d20c41e76d2cc1a"

// Pinned source files remain inert testdata. Extracted declarations below are
// compared byte-for-byte with the Git object before being compiled and executed.
func permissionLegacyDeclarations(t *testing.T, path string, names ...string) string {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("testdata", "permission-legacy", strings.ReplaceAll(path, "/", "-")+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "show", permissionLegacyCommit+":agents-plugin-tool/internal/"+path)
	original, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pinned Git object unavailable: %v\n%s", err, original)
	}
	extract := func(raw []byte) map[string]string {
		fs := token.NewFileSet()
		file, err := parser.ParseFile(fs, path, raw, 0)
		if err != nil {
			t.Fatal(err)
		}
		out := make(map[string]string)
		for _, decl := range file.Decls {
			key := ""
			switch d := decl.(type) {
			case *ast.FuncDecl:
				key = d.Name.Name
			case *ast.GenDecl:
				if len(d.Specs) == 1 {
					if typ, ok := d.Specs[0].(*ast.TypeSpec); ok {
						key = typ.Name.Name
					}
				}
			}
			if key != "" {
				out[key] = string(raw[fs.Position(decl.Pos()).Offset:fs.Position(decl.End()).Offset])
			}
		}
		return out
	}
	copied, pinned := extract(fixture), extract(original)
	var result strings.Builder
	for _, name := range names {
		body := copied[name]
		if body == "" || body != pinned[name] {
			t.Fatalf("fixture %s %s differs from %s", path, name, permissionLegacyCommit)
		}
		fmt.Fprintf(&result, "\n// Source: agents-plugin-tool/internal/%s @ %s\n%s\n", path, permissionLegacyCommit, body)
	}
	return result.String()
}

func TestPermissionSidecarOldWriteNewReopen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pinned fixture uses the Unix replacement helper")
	}
	source := `package permissionlegacy
import (
 "bytes"
 "encoding/json"
 "errors"
 "fmt"
 "io/fs"
 "os"
 "path/filepath"
 "reflect"
 "regexp"
 "strings"
 "testing"
 "time"
 current "github.com/kang-sw/devenv/internal/wsagent"
 "github.com/kang-sw/devenv/internal/wsinbox"
)
// Optional capability and turn payloads are not exercised by this writer fixture.
type Profile = current.Profile
type TurnResult = current.TurnResult
const (
 recordFileName = "agent.json"
 recordSchemaVersion = 1
 sessionsDirName = "sessions"
 registrationFileName = "registration.json"
 registrationSchemaVersion = 1
 AgentRecordFileName = "agent.json"
 labelFileName = "label.json"
)
var replyIDPattern = regexp.MustCompile("^[a-z0-9]{1,64}$")
var errRegistrationMismatch = errors.New("registration identity mismatch")
// Preserve the old namespaced call while executing its pinned Unix helper.
var wsstate = struct { AtomicReplaceFile func(string,string) error }{AtomicReplaceFile}
`
	source += permissionLegacyDeclarations(t, "wsagent/record.go", "Record", "UnmarshalJSON", "MarshalJSON", "PendingReport", "ParentRef", "QueuedInput", "recordPath", "readRecord", "writeRecord", "writeFileAtomic")
	source += permissionLegacyDeclarations(t, "wsagent/children.go", "steeredReport")
	source += permissionLegacyDeclarations(t, "wsagent/replace_file_other.go", "replaceFile")
	source += permissionLegacyDeclarations(t, "wsstate/replace_file_unix.go", "AtomicReplaceFile")
	source += permissionLegacyDeclarations(t, "wsdashboard/registry.go", "Registration", "ValidReplyID", "SessionDir", "ReadRegistration", "WriteRegistration", "RemoveRegistration", "PruneRegistrations", "LastSeen", "agentRecordEnd", "terminalConversation", "retired", "readAgentRecordEnd")
	source += permissionLegacyDeclarations(t, "wsdashboard/label_route.go", "labelRecord", "writeSessionLabel")
	source += permissionLegacyDeclarations(t, "wsdashboard/endpoint.go", "writePrivateFile")
	source += permissionLegacyHarness
	// A package under testdata is compiled only through this focused invocation;
	// no dashboard test package, provider, server or harness is started.
	harness, err := os.MkdirTemp(filepath.Join("testdata", "permission-legacy"), "generated-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(harness)
	if err := os.WriteFile(filepath.Join(harness, "legacy_test.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cases := []string{"record", "registration", "remove", "labelset", "labelclear", "hosted", "expired", "deleted"}
	seed := []byte("{\n  \"permission_level\": \"read-only\", \"future\": {\"keep\":[1,true,\"x\"]}\n}\n")
	for _, id := range cases {
		dir := filepath.Join(root, "sessions", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		// Model a durable lowering before an old writer touches the session.
		if err := writePermissionSelection(dir, PermissionFull); err != nil {
			t.Fatal(err)
		}
		if err := writePermissionSelection(dir, PermissionReadOnly); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "permission.json"), seed, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-v", "./"+filepath.ToSlash(harness), "-count=1", "-run", "^TestPermissionPinnedWriters$")
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "PERMISSION_FIXTURE_ROOT="+absRoot)
	output, err := cmd.CombinedOutput()
	t.Logf("pinned writer fixture output:\n%s", output)
	if err != nil || !bytes.Contains(output, []byte("--- PASS: TestPermissionPinnedWriters")) {
		t.Fatalf("pinned fixture failed or selector empty: %v", err)
	}
	checkpoints, err := os.ReadDir(filepath.Join(root, "checkpoints"))
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 14 {
		t.Fatalf("got %d old-write checkpoints, want 14", len(checkpoints))
	}
	for _, checkpoint := range checkpoints {
		dir := filepath.Join(root, "checkpoints", checkpoint.Name())
		assertPermissionSelection(t, dir, PermissionReadOnly, true)
		if err := writePermissionSelection(dir, PermissionWorkdirWrite); err != nil {
			t.Fatal(err)
		}
		assertPermissionSelection(t, dir, PermissionWorkdirWrite, true)
	}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			dir := filepath.Join(root, "sessions", id)
			if id == "expired" || id == "deleted" {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatalf("session lifetime did not end: %v", err)
				}
				if level, explicit, err := readPermissionSelection(dir); err != nil || explicit || level != "" {
					t.Fatalf("deleted selection = %q %v %v", level, explicit, err)
				}
				return
			}
			got, err := os.ReadFile(filepath.Join(dir, "permission.json"))
			if err != nil || !bytes.Equal(got, seed) {
				t.Fatalf("old write changed selection: %s (%v)", got, err)
			}
			assertPermissionSelection(t, dir, PermissionReadOnly, true)
			if err := writePermissionSelection(dir, PermissionWorkdirWrite); err != nil {
				t.Fatal(err)
			}
			assertPermissionSelection(t, dir, PermissionWorkdirWrite, true)
			if id != "record" && id != "hosted" {
				if _, err := os.Stat(filepath.Join(dir, "agent.json")); !os.IsNotExist(err) {
					t.Fatalf("CLI fixture unexpectedly has agent record: %v", err)
				}
			}
		})
	}
}

const permissionLegacyHarness = `
func TestPermissionPinnedWriters(t *testing.T) {
 root := os.Getenv("PERMISSION_FIXTURE_ROOT")
 if root == "" { t.Fatal("missing isolated fixture root") }
 now := time.Date(2026,11,15,0,0,0,0,time.UTC)
 checkpoint := 0
 check := func(id string, fn func() error) {
  t.Helper()
  path := filepath.Join(SessionDir(root,id),"permission.json")
  before, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
  if err := fn(); err != nil { t.Fatal(err) }
  after, err := os.ReadFile(path)
  if err != nil || !bytes.Equal(before,after) { t.Fatalf("%s changed selection: %v", id,err) }
  checkpoint++
  snapshot := filepath.Join(root,"checkpoints",fmt.Sprintf("%02d%s",checkpoint,id))
  if err := os.MkdirAll(snapshot,0o700); err != nil { t.Fatal(err) }
  if err := os.WriteFile(filepath.Join(snapshot,"permission.json"),after,0o600); err != nil { t.Fatal(err) }
 }
 check("record",func() error { return writeRecord(SessionDir(root,"record"),Record{ReplyID:"record",WSKey:"key",Root:root,Lead:true}) })
 rec, err := readRecord(SessionDir(root,"record")); if err != nil { t.Fatal(err) }
 rec.Model = "updated"
 check("record",func() error { return writeRecord(SessionDir(root,"record"),rec) })
 check("registration",func() error { return WriteRegistration(root,Registration{ReplyID:"registration",Project:root}) })
 check("registration",func() error { return WriteRegistration(root,Registration{ReplyID:"registration",Project:root,Harness:"pi"}) })
 check("remove",func() error { return WriteRegistration(root,Registration{ReplyID:"remove",Project:root}) })
 check("remove",func() error { return RemoveRegistration(root,"remove") })
 check("remove",func() error { return RemoveRegistration(root,"remove") })
 check("labelset",func() error { return writeSessionLabel(SessionDir(root,"labelset"),"label") })
 check("labelclear",func() error { return writeSessionLabel(SessionDir(root,"labelclear"),"label") })
 check("labelclear",func() error { return writeSessionLabel(SessionDir(root,"labelclear"),"") })
 check("hosted",func() error { return writeRecord(SessionDir(root,"hosted"),Record{ReplyID:"hosted",WSKey:"key",Root:root,Lead:true}) })
 check("hosted",func() error { return WriteRegistration(root,Registration{ReplyID:"hosted",Hosted:true,HeartbeatAt:now.Add(-30*24*time.Hour).Format(time.RFC3339)}) })
 check("expired",func() error { return WriteRegistration(root,Registration{ReplyID:"expired",HeartbeatAt:now.Add(-30*24*time.Hour).Format(time.RFC3339)}) })
 check("deleted",func() error { return WriteRegistration(root,Registration{ReplyID:"deleted",HeartbeatAt:now.Format(time.RFC3339)}) })
 if count := PruneRegistrations(root,now,7*24*time.Hour); count != 1 { t.Fatalf("pruned %d, want only expired ordinary session",count) }
 if _,err := os.Stat(SessionDir(root,"hosted")); err != nil { t.Fatal("hosted record was pruned",err) }
 if err := os.RemoveAll(SessionDir(root,"deleted")); err != nil { t.Fatal(err) }
}
`

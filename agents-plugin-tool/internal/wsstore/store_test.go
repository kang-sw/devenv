package wsstore

import (
	"context"
	"database/sql"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

func TestOpenCloseReopenCreatesWorktreeDatabase(t *testing.T) {
	root := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{CacheHome: cache, Now: func() time.Time { return testNow }})
	store, err := manager.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	path := store.Path()
	if filepath.Base(path) != "state.sqlite" {
		t.Fatalf("db path = %q, want state.sqlite", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state db missing: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Path() != path {
		t.Fatalf("reopened path = %q, want %q", reopened.Path(), path)
	}
}

func TestArtifactMetadataKeepsFileBackedStreamPath(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	stream := filepath.Join(t.TempDir(), "stdout")
	if err := os.WriteFile(stream, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := store.UpsertArtifact(context.Background(), Artifact{
		ArtifactID: "stream-1",
		Kind:       "exec.stdout",
		Path:       stream,
		State:      ArtifactStateCompleted,
		ByteCount:  5,
		ExpiresAt:  testNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Artifact(context.Background(), "stream-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Path != stream || got.ByteCount != 5 || got.Kind != "exec.stdout" {
		t.Fatalf("artifact metadata = %#v, ok=%t", got, ok)
	}
	if err := store.UpsertRetentionPolicy(context.Background(), RetentionPolicy{Scope: "exec.stdout", TTL: time.Hour, MaxRows: 100, MaxBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	count, err := store.Count(context.Background(), "retention_policies")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retention policy count = %d, want 1", count)
	}
}

func TestPruneSkipsActiveAndPinnedState(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	ctx := context.Background()
	expired := testNow.Add(-time.Hour)
	activePath := writeArtifactFile(t, "active")
	pinnedPath := writeArtifactFile(t, "pinned")
	for _, artifact := range []Artifact{
		{ArtifactID: "active", Kind: "exec.stdout", Path: activePath, State: ArtifactStateRunning, ExpiresAt: expired},
		{ArtifactID: "pinned", Kind: "exec.stdout", Path: pinnedPath, State: ArtifactStateCompleted, Pinned: true, ExpiresAt: expired},
	} {
		if err := store.UpsertArtifact(ctx, artifact); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 0 || result.Deleted != 0 {
		t.Fatalf("prune result = %#v, want skip", result)
	}
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active artifact was removed: %v", err)
	}
	if _, err := os.Stat(pinnedPath); err != nil {
		t.Fatalf("pinned artifact was removed: %v", err)
	}
}

func TestPruneDeletesExpiredCompletedArtifacts(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	ctx := context.Background()
	path := writeArtifactFile(t, "done")
	if err := store.UpsertArtifact(ctx, Artifact{ArtifactID: "done", Kind: "agent.output", Path: path, State: ArtifactStateCompleted, ExpiresAt: testNow.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("prune result = %#v, want one delete", result)
	}
	runs, err := store.Count(ctx, "prune_runs")
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("prune runs = %d, want 1", runs)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("artifact file still exists or stat failed differently: %v", err)
	}
	if _, ok, err := store.Artifact(ctx, "done"); err != nil || ok {
		t.Fatalf("artifact row remains ok=%t err=%v", ok, err)
	}
}

func TestPruneTombstoneRetry(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	ctx := context.Background()
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertArtifact(ctx, Artifact{ArtifactID: "blocked", Kind: "exec.stdout", Path: blocked, State: ArtifactStateCompleted, ExpiresAt: testNow.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tombstoned != 1 {
		t.Fatalf("prune result = %#v, want tombstone", result)
	}
	tombs, err := store.Count(ctx, "artifact_tombstones")
	if err != nil {
		t.Fatal(err)
	}
	if tombs != 1 {
		t.Fatalf("tombstones = %d, want 1", tombs)
	}
	if err := os.Remove(filepath.Join(blocked, "child")); err != nil {
		t.Fatal(err)
	}
	result, err = store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.RetrySucceeded != 1 {
		t.Fatalf("retry result = %#v, want success", result)
	}
	tombs, _ = store.Count(ctx, "artifact_tombstones")
	if tombs != 0 {
		t.Fatalf("tombstones after retry = %d, want 0", tombs)
	}
}

func openStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return testNow },
	}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeArtifactFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init")
	mustRun(t, root, "git", "config", "user.email", "a@example.com")
	mustRun(t, root, "git", "config", "user.name", "A")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-m", "init")
	return root
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}

func TestRuntimeMetadataInventoryClassifiesKnownStateFiles(t *testing.T) {
	if err := ValidateRuntimeMetadataInventory(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		source RuntimeStateSource
		field  string
		want   RuntimeFieldStorage
	}{
		{RuntimeSourceAgentJSON, "backend", RuntimeFieldSQLiteMetadata},
		{RuntimeSourceAgentJSON, "system_prompt_path", RuntimeFieldSQLiteMetadata},
		{RuntimeSourceAgentJSON, "agent_json_compatibility", RuntimeFieldTemporaryCompatOnly},
		{RuntimeSourceAgentCurrentJSON, "execution_id", RuntimeFieldSQLiteMetadata},
		{RuntimeSourceAgentCurrentJSON, "stdout_path", RuntimeFieldSQLiteMetadata},
		{RuntimeSourceExecJobJSON, "exec_key", RuntimeFieldSQLiteMetadata},
		{RuntimeSourceExecJobJSON, "stdout", RuntimeFieldFileBackedPayload},
		{RuntimeSourceExecJobJSON, "combined_bytes", RuntimeFieldSQLiteMetadata},
	}
	for _, tc := range cases {
		got, ok := RuntimeField(tc.source, tc.field)
		if !ok {
			t.Fatalf("missing classification for %s %s", tc.source, tc.field)
		}
		if got.Storage != tc.want {
			t.Fatalf("%s %s storage = %s, want %s", tc.source, tc.field, got.Storage, tc.want)
		}
	}
}

func TestRuntimeMetadataInventoryKeepsPathsInSQLiteAndPayloadsFileBacked(t *testing.T) {
	paths := []struct {
		source RuntimeStateSource
		field  string
	}{
		{RuntimeSourceAgentJSON, "system_prompt_path"},
		{RuntimeSourceAgentJSON, "last_output_path"},
		{RuntimeSourceAgentCurrentJSON, "prompt_path"},
		{RuntimeSourceAgentCurrentJSON, "stdout_path"},
		{RuntimeSourceAgentCurrentJSON, "stderr_path"},
	}
	for _, path := range paths {
		got, ok := RuntimeField(path.source, path.field)
		if !ok {
			t.Fatalf("missing path classification for %s %s", path.source, path.field)
		}
		if got.Storage != RuntimeFieldSQLiteMetadata || got.WriteAuthority != RuntimeAuthoritySQLite {
			t.Fatalf("%s %s = %#v, want sqlite path metadata", path.source, path.field, got)
		}
	}

	payloads := []struct {
		source RuntimeStateSource
		field  string
	}{
		{RuntimeSourceExecJobJSON, "stdout"},
		{RuntimeSourceExecJobJSON, "stderr"},
		{RuntimeSourceExecJobJSON, "combined"},
	}
	for _, payload := range payloads {
		got, ok := RuntimeField(payload.source, payload.field)
		if !ok {
			t.Fatalf("missing payload classification for %s %s", payload.source, payload.field)
		}
		if got.Storage != RuntimeFieldFileBackedPayload || got.WriteAuthority != RuntimeAuthorityFile {
			t.Fatalf("%s %s = %#v, want file-backed payload", payload.source, payload.field, got)
		}
	}
}

func jsonFieldSetFromSource(t *testing.T, path, typeName string, extras ...string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok.String() != "type" {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s in %s is not a struct", typeName, path)
			}
			return jsonFieldSet(structType, extras...)
		}
	}
	t.Fatalf("type %s not found in %s", typeName, path)
	return nil
}

func jsonFieldSet(typ *ast.StructType, extras ...string) map[string]bool {
	fields := map[string]bool{}
	for _, field := range typ.Fields.List {
		if field.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		tag := reflect.StructTag(raw).Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			fields[name] = true
		}
	}
	for _, extra := range extras {
		fields[extra] = true
	}
	return fields
}

func TestMissingFileBackedPayloadIsRecoverableConsistencyState(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	path := filepath.Join(t.TempDir(), "missing-output.md")
	if err := store.UpsertArtifact(context.Background(), Artifact{ArtifactID: "missing-output", Kind: "agent.output", Path: path, State: ArtifactStateCompleted}); err != nil {
		t.Fatal(err)
	}
	row, ok, err := store.Artifact(context.Background(), "missing-output")
	if err != nil || !ok {
		t.Fatalf("artifact row ok=%t err=%v", ok, err)
	}
	if got := ClassifyFileBackedPayload(row.Path); got != PayloadConsistencyMissingPayload {
		t.Fatalf("missing payload consistency = %s, want %s", got, PayloadConsistencyMissingPayload)
	}
	present := writeArtifactFile(t, "present-output")
	if got := ClassifyFileBackedPayload(present); got != PayloadConsistencyPresent {
		t.Fatalf("present payload consistency = %s, want %s", got, PayloadConsistencyPresent)
	}
}

func TestAgentJSONCompatibilityIsNotWriteAuthority(t *testing.T) {
	compat, ok := RuntimeField(RuntimeSourceAgentJSON, "agent_json_compatibility")
	if !ok {
		t.Fatal("missing agent.json compatibility classification")
	}
	if compat.Storage != RuntimeFieldTemporaryCompatOnly || compat.WriteAuthority != RuntimeAuthorityNone {
		t.Fatalf("compatibility classification = %#v, want temporary read-only", compat)
	}
	for _, item := range RuntimeMetadataInventory() {
		if item.Source == RuntimeSourceAgentJSON && item.Storage == RuntimeFieldSQLiteMetadata && item.WriteAuthority != RuntimeAuthoritySQLite {
			t.Fatalf("agent.json metadata field %s authority = %s, want sqlite", item.Field, item.WriteAuthority)
		}
	}
}

func TestAgentDefinitionsPersistSQLiteMetadata(t *testing.T) {
	root := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	store, err := NewManager(Options{CacheHome: cache}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	key, err := AgentInternalKey("implementer")
	if err != nil {
		t.Fatal(err)
	}
	def := AgentDefinition{AgentKey: key, ActorID: "actor-one", PublicName: "implementer", StatePath: "actor-dir", SchemaVersion: 1, Backend: "codex", Tier: "core", Model: "gpt-test", Status: "idle", CreatedAt: "2026-05-25T00:00:00Z", LastSeenAt: "2026-05-25T00:00:00Z", LastOutputPath: "output.md", PromptRefs: []string{"delegate-orientation"}, SystemPromptPath: "system.md", Capabilities: map[string]bool{"resume": true}, Ephemeral: true}
	if err := store.UpsertAgentDefinition(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewManager(Options{CacheHome: cache}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, ok, err := reopened.AgentDefinition(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.PublicName != "implementer" || got.SystemPromptPath != "system.md" || !got.Capabilities["resume"] || !got.Ephemeral {
		t.Fatalf("persisted agent definition mismatch: ok=%t def=%+v", ok, got)
	}
}

func TestAgentInternalKeyScopesPublicNamesByWorktreeStore(t *testing.T) {
	ctx := context.Background()
	rootA := initRepo(t)
	rootB := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	storeA, err := NewManager(Options{CacheHome: cache}).Open(rootA)
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	storeB, err := NewManager(Options{CacheHome: cache}).Open(rootB)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	key, err := AgentInternalKey("same")
	if err != nil {
		t.Fatal(err)
	}
	if err := storeA.UpsertAgentDefinition(ctx, AgentDefinition{AgentKey: key, PublicName: "same", StatePath: "root-a", SchemaVersion: 1, Status: "idle"}); err != nil {
		t.Fatal(err)
	}
	if err := storeB.UpsertAgentDefinition(ctx, AgentDefinition{AgentKey: key, PublicName: "same", StatePath: "root-b", SchemaVersion: 1, Status: "idle"}); err != nil {
		t.Fatal(err)
	}
	gotA, ok, err := storeA.AgentDefinition(ctx, key)
	if err != nil || !ok {
		t.Fatalf("root A definition ok=%t err=%v", ok, err)
	}
	gotB, ok, err := storeB.AgentDefinition(ctx, key)
	if err != nil || !ok {
		t.Fatalf("root B definition ok=%t err=%v", ok, err)
	}
	if gotA.StatePath != "root-a" || gotB.StatePath != "root-b" {
		t.Fatalf("same public name should stay distinct by worktree store: A=%+v B=%+v", gotA, gotB)
	}
}

func TestPruneAgentInstancesUsesRecordedSQLiteCandidates(t *testing.T) {
	ctx := context.Background()
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	key, err := AgentInternalKey("impl")
	if err != nil {
		t.Fatal(err)
	}
	dueDir := filepath.Join(store.Layout().AgentsDir, "due")
	currentDir := filepath.Join(store.Layout().AgentsDir, "current")
	activeDir := filepath.Join(store.Layout().AgentsDir, "active")
	recoveryDir := filepath.Join(store.Layout().AgentsDir, "recovery")
	backoffDir := filepath.Join(store.Layout().AgentsDir, "backoff")
	unrelatedDir := filepath.Join(store.Layout().AgentsDir, "unrelated")
	for _, dir := range []string{dueDir, currentDir, activeDir, recoveryDir, backoffDir, unrelatedDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := testNow.Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano)
	due := AgentDefinition{AgentKey: key, PublicName: "impl", StatePath: "due", SchemaVersion: 1, Status: "idle", CreatedAt: old, RetentionEligibleAt: testNow.Add(-time.Hour).Format(time.RFC3339Nano), RetentionNextCheckAt: testNow.Add(-time.Hour).Format(time.RFC3339Nano), CleanupState: "retired"}
	if err := store.UpsertAgentDefinition(ctx, due); err != nil {
		t.Fatal(err)
	}
	current := due
	current.StatePath = "current"
	current.CleanupState = "current"
	if err := store.UpsertAgentDefinition(ctx, current); err != nil {
		t.Fatal(err)
	}
	pinned := due
	pinned.StatePath = "pinned"
	pinned.CleanupState = "retired"
	pinned.Pinned = true
	if err := store.UpsertAgentDefinition(ctx, pinned); err != nil {
		t.Fatal(err)
	}
	activeKey, err := AgentInternalKey("active")
	if err != nil {
		t.Fatal(err)
	}
	active := due
	active.AgentKey = activeKey
	active.PublicName = "active"
	active.StatePath = "active"
	active.Status = "idle"
	if err := store.UpsertAgentDefinition(ctx, active); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(activeDir, "current"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeDir, "current", "state.json"), []byte(`{"status":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	recoveryKey, err := AgentInternalKey("recovery")
	if err != nil {
		t.Fatal(err)
	}
	recovery := due
	recovery.AgentKey = recoveryKey
	recovery.PublicName = "recovery"
	recovery.StatePath = "recovery"
	recovery.CleanupState = "recovery"
	if err := store.UpsertAgentDefinition(ctx, recovery); err != nil {
		t.Fatal(err)
	}
	backoffKey, err := AgentInternalKey("backoff")
	if err != nil {
		t.Fatal(err)
	}
	backoff := due
	backoff.AgentKey = backoffKey
	backoff.PublicName = "backoff"
	backoff.StatePath = "backoff"
	backoff.RetentionNextCheckAt = testNow.Add(time.Hour).Format(time.RFC3339Nano)
	if err := store.UpsertAgentDefinition(ctx, backoff); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAgentDefinition(ctx, key); err != nil {
		t.Fatal(err)
	}
	for _, retireKey := range []string{activeKey, recoveryKey, backoffKey} {
		if err := store.DeleteAgentDefinition(ctx, retireKey); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE agent_instances SET cleanup_state = 'recovery' WHERE agent_key = ?`, recoveryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE agent_instances SET retention_next_check_at = ? WHERE agent_key = ?`, testNow.Add(time.Hour).Format(time.RFC3339Nano), backoffKey); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(ctx, current); err != nil {
		t.Fatal(err)
	}
	res, err := store.PruneAgentInstances(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.Scanned != 2 || res.Skipped != 1 {
		t.Fatalf("cleanup result = %+v, want one deletion and one active-state skip", res)
	}
	if _, err := os.Stat(dueDir); !os.IsNotExist(err) {
		t.Fatalf("due dir still present/stat err=%v", err)
	}
	for _, dir := range []string{currentDir, activeDir, recoveryDir, backoffDir, unrelatedDir} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("dir %s should remain: %v", dir, err)
		}
	}
}

func TestPruneAgentInstancesRecordsRetryFence(t *testing.T) {
	ctx := context.Background()
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	key, err := AgentInternalKey("broken")
	if err != nil {
		t.Fatal(err)
	}
	def := AgentDefinition{AgentKey: key, PublicName: "broken", StatePath: string([]byte{'b', 'a', 'd', 0, 'p', 'a', 't', 'h'}), SchemaVersion: 1, Status: "idle", CreatedAt: testNow.Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano), RetentionEligibleAt: testNow.Add(-time.Hour).Format(time.RFC3339Nano), RetentionNextCheckAt: testNow.Add(-time.Hour).Format(time.RFC3339Nano), CleanupState: "retired"}
	if err := store.UpsertAgentDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAgentDefinition(ctx, key); err != nil {
		t.Fatal(err)
	}
	res, err := store.PruneAgentInstances(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 {
		t.Fatalf("cleanup result = %+v, want failed retry fence", res)
	}
	var state, nextCheck, cleanupErr string
	if err := store.db.QueryRowContext(ctx, `SELECT cleanup_state, retention_next_check_at, cleanup_error FROM agent_instances WHERE agent_key = ?`, key).Scan(&state, &nextCheck, &cleanupErr); err != nil {
		t.Fatal(err)
	}
	if state != "cleanup_failed" || nextCheck == "" || cleanupErr == "" {
		t.Fatalf("retry fence state=%q next=%q err=%q", state, nextCheck, cleanupErr)
	}
}

func TestExecJobMetadataRoundTripAndConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	root := initRepo(t)
	store, err := NewManager(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job := ExecJob{ExecKey: "exec-1-0000000000000001", Status: "running", SchemaVersion: 1, Root: root, WorkingDir: root, Argv: []string{"echo", "ok"}, EnvJSON: `{"A":"B"}`, StdinPresent: true, StdinBytes: 3, PID: 123, StartedAt: "2026-05-25T00:00:00Z", UpdatedAt: "2026-05-25T00:00:00Z", StdoutPath: filepath.Join(root, "stdout"), StderrPath: filepath.Join(root, "stderr"), CombinedPath: filepath.Join(root, "combined"), CleanupState: ArtifactStateRunning}
	if err := store.UpsertExecJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.ExecJob(ctx, job.ExecKey)
	if err != nil || !ok {
		t.Fatalf("ExecJob ok=%t err=%v", ok, err)
	}
	if got.Argv[0] != "echo" || !got.StdinPresent || got.StdoutPath == "" || got.CleanupState != ArtifactStateRunning {
		t.Fatalf("round trip = %#v", got)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			j := job
			j.ExecKey = fmt.Sprintf("exec-1-%016x", i+2)
			j.StdoutBytes = int64(i)
			if err := store.UpsertExecJob(ctx, j); err != nil {
				t.Errorf("concurrent upsert: %v", err)
			}
		}(i)
	}
	wg.Wait()
	count, err := store.Count(ctx, "exec_jobs")
	if err != nil {
		t.Fatal(err)
	}
	if count != 9 {
		t.Fatalf("exec_jobs count = %d, want 9", count)
	}
}

func TestExecArtifactsPruneEligibilityAndTombstones(t *testing.T) {
	ctx := context.Background()
	root := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	store, err := NewManager(Options{CacheHome: cache}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	payloadDir := t.TempDir()
	expired := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	touch := func(name string) string {
		path := filepath.Join(payloadDir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	jobs := []ExecJob{
		{ExecKey: "exec-1-0000000000000101", Status: "running", StdoutPath: touch("running"), ExpiresAt: expired},
		{ExecKey: "exec-1-0000000000000102", Status: "cancel_requested", StdoutPath: touch("cancel"), ExpiresAt: expired},
		{ExecKey: "exec-1-0000000000000103", Status: "running", LeaseID: "lease-running", StdoutPath: touch("leased-running"), ExpiresAt: expired},
		{ExecKey: "exec-1-0000000000000104", Status: "succeeded", Pinned: true, StdoutPath: touch("pinned"), ExpiresAt: expired},
		{ExecKey: "exec-1-0000000000000105", Status: "succeeded", StdoutPath: touch("completed"), ExpiresAt: expired},
		{ExecKey: "exec-1-0000000000000106", Status: "succeeded", LeaseID: "lease-terminal", StdoutPath: touch("leased-terminal"), ExpiresAt: expired},
	}
	for _, job := range jobs {
		job.SchemaVersion = 1
		job.Root = root
		job.WorkingDir = root
		job.StartedAt = testNow.Format(time.RFC3339Nano)
		job.UpdatedAt = job.StartedAt
		if err := store.UpsertExecJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("prune result = %#v", result)
	}
	for _, name := range []string{"running", "cancel", "leased-running", "leased-terminal", "pinned"} {
		if _, err := os.Stat(filepath.Join(payloadDir, name)); err != nil {
			t.Fatalf("guarded exec payload %s was pruned: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(payloadDir, "completed")); !os.IsNotExist(err) {
		t.Fatalf("completed exec payload still exists or unexpected err: %v", err)
	}

	blocked := filepath.Join(payloadDir, "blocked-dir")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertExecJob(ctx, ExecJob{ExecKey: "exec-1-0000000000000106", Status: "succeeded", SchemaVersion: 1, Root: root, WorkingDir: root, StartedAt: testNow.Format(time.RFC3339Nano), UpdatedAt: testNow.Format(time.RFC3339Nano), StdoutPath: blocked, ExpiresAt: expired}); err != nil {
		t.Fatal(err)
	}
	result, err = store.PruneExpired(ctx, PruneOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tombstoned != 1 {
		t.Fatalf("tombstone prune result = %#v", result)
	}
	if count, err := store.Count(ctx, "artifact_tombstones"); err != nil || count != 1 {
		t.Fatalf("artifact_tombstones count=%d err=%v", count, err)
	}
}

func TestIndependentHandleContentionRetriesShortWrite(t *testing.T) {
	root := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	ctx := context.Background()
	store, err := NewManager(Options{CacheHome: cache}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}

	holder, err := sql.Open("sqlite", store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(ctx, `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}

	busySeen := make(chan struct{}, 1)
	previousHook := sqliteRetryBusyHook
	sqliteRetryBusyHook = func(error) {
		select {
		case busySeen <- struct{}{}:
		default:
		}
	}
	defer func() { sqliteRetryBusyHook = previousHook }()

	errCh := make(chan error, 1)
	go func() {
		errCh <- store.UpsertAgentDefinition(ctx, AgentDefinition{AgentKey: "contended", PublicName: "contended", StatePath: "contended", SchemaVersion: 1, Status: "idle"})
	}()
	select {
	case <-busySeen:
	case err := <-errCh:
		t.Fatalf("contended write finished before observing busy retry: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for busy retry")
	}
	if _, err := holder.ExecContext(ctx, `COMMIT`); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("contended write did not recover: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("contended write timed out")
	}
	if _, ok, err := store.AgentDefinition(ctx, "contended"); err != nil || !ok {
		t.Fatalf("agent definition after contended write ok=%t err=%v", ok, err)
	}
}

func TestAgentRolePointerHistoryAndCollision(t *testing.T) {
	ctx := context.Background()
	rootA := initRepo(t)
	rootB := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	storeA, err := NewManager(Options{CacheHome: cache}).Open(rootA)
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	storeB, err := NewManager(Options{CacheHome: cache}).Open(rootB)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	key, err := AgentInternalKey("impl")
	if err != nil {
		t.Fatal(err)
	}
	first := AgentDefinition{AgentKey: key, PublicName: "impl", StatePath: "first", SchemaVersion: 1, Backend: "codex", Tier: "core", Model: "old", Status: "idle", CreatedAt: testNow.Format(time.RFC3339Nano), LastSeenAt: testNow.Format(time.RFC3339Nano), LastOutputPath: "output.md"}
	if err := storeA.UpsertAgentDefinition(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.StatePath = "second"
	second.Model = "new"
	if err := storeA.UpsertAgentDefinition(ctx, second); err != nil {
		t.Fatal(err)
	}
	other := AgentDefinition{AgentKey: key, PublicName: "impl", StatePath: "other-root", SchemaVersion: 1, Backend: "codex", Tier: "core", Model: "other", Status: "idle", CreatedAt: testNow.Format(time.RFC3339Nano), LastSeenAt: testNow.Format(time.RFC3339Nano), LastOutputPath: "output.md"}
	if err := storeB.UpsertAgentDefinition(ctx, other); err != nil {
		t.Fatal(err)
	}
	gotA, ok, err := storeA.AgentDefinition(ctx, key)
	if err != nil || !ok {
		t.Fatalf("root A role ok=%t err=%v", ok, err)
	}
	gotB, ok, err := storeB.AgentDefinition(ctx, key)
	if err != nil || !ok {
		t.Fatalf("root B role ok=%t err=%v", ok, err)
	}
	if gotA.StatePath != "second" || gotA.Model != "new" {
		t.Fatalf("root A pointer = %+v", gotA)
	}
	if gotB.StatePath != "other-root" || gotB.Model != "other" {
		t.Fatalf("root B pointer = %+v", gotB)
	}
	count, err := storeA.Count(ctx, "agent_instances")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("root A agent instance count = %d, want 2", count)
	}
}

func TestSQLiteRetryRetriesBusyAndLockedErrors(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := withSQLiteRetry(ctx, func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("synthetic SQLITE_BUSY")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	attempts = 0
	err = withSQLiteRetry(ctx, func() error {
		attempts++
		if attempts < 2 {
			return fmt.Errorf("synthetic SQLITE_LOCKED")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("locked retry returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("locked attempts = %d, want 2", attempts)
	}
}

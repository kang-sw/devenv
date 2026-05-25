package wsstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
	_ "modernc.org/sqlite"
)

const (
	schemaVersion = 1

	ArtifactStateActive          = "active"
	ArtifactStateRunning         = "running"
	ArtifactStateCancelRequested = "cancel_requested"
	ArtifactStateLeased          = "leased"
	ArtifactStateCompleted       = "completed"
	ArtifactStateCleanupFailed   = "cleanup_failed"
)

type Clock func() time.Time

type Options struct {
	CacheHome string
	Now       Clock
}

type Manager struct {
	opts Options
}

type Store struct {
	db      *sql.DB
	path    string
	layout  wsstate.Layout
	now     Clock
	writeMu *sync.Mutex
}

var (
	openMu       sync.Mutex
	writeLocksMu sync.Mutex
	writeLocks   = map[string]*sync.Mutex{}
)

type Actor struct {
	ActorID       string
	Authority     string
	RootPath      string
	WorktreeKey   string
	ParentActorID string
	Status        string
	Pinned        bool
}

type AgentDefinition struct {
	AgentKey             string
	ActorID              string
	PublicName           string
	StatePath            string
	SchemaVersion        int
	Backend              string
	Harness              string
	Tier                 string
	Model                string
	Effort               string
	SessionID            string
	Status               string
	CreatedAt            string
	LastSeenAt           string
	LastCallAt           string
	LastOutputPath       string
	PromptRefs           []string
	SystemPromptPath     string
	ChildActorID         string
	ChildActorAuthority  string
	Capabilities         map[string]bool
	Ephemeral            bool
	InstanceID           string
	RetentionEligibleAt  string
	RetentionCheckedAt   string
	RetentionNextCheckAt string
	CleanupState         string
	CleanupAttemptedAt   string
	CleanupError         string
	Pinned               bool
}

const AgentInstanceRetentionTTL = 7 * 24 * time.Hour

type AgentInstanceCleanupResult struct {
	Scanned int
	Deleted int
	Failed  int
	Skipped int
}

type ExecJob struct {
	ExecKey         string
	OwnerActorID    string
	Status          string
	LeaseID         string
	SchemaVersion   int
	Root            string
	WorkingDir      string
	Argv            []string
	Command         string
	Shell           string
	EnvJSON         string
	StdinPresent    bool
	StdinBytes      int64
	PID             int
	StartedAt       string
	UpdatedAt       string
	CompletedAt     string
	ExitCode        int
	Error           string
	CancelRequested bool
	LostWorker      bool
	StdoutPath      string
	StderrPath      string
	CombinedPath    string
	StdoutBytes     int64
	StderrBytes     int64
	CombinedBytes   int64
	Pinned          bool
	ExpiresAt       string
	CleanupState    string
}

type Artifact struct {
	ArtifactID     string
	Kind           string
	Path           string
	OwnerActorID   string
	State          string
	ByteCount      int64
	Pinned         bool
	ExpiresAt      time.Time
	LastAccessedAt time.Time
}

type RetentionPolicy struct {
	Scope    string
	TTL      time.Duration
	MaxRows  int
	MaxBytes int64
}

type PruneOptions struct {
	Limit int
}

type PruneResult struct {
	Scanned        int
	Deleted        int
	Tombstoned     int
	Retried        int
	RetrySucceeded int
}

func NewManager(opts Options) Manager {
	return Manager{opts: opts}
}

func (m Manager) Open(root string) (*Store, error) {
	if m.opts.CacheHome != "" {
		if err := os.MkdirAll(m.opts.CacheHome, 0o755); err != nil {
			return nil, err
		}
	}
	layout, _, _, err := wsstate.NewManager(wsstate.Options{
		CacheHome: m.opts.CacheHome,
		Now:       wsstate.Clock(m.now),
	}).Ensure(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(layout.WorktreeDir, "state.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	newDB := !fileExists(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path, layout: layout, now: m.now, writeMu: writeLock(path)}
	openMu.Lock()
	defer openMu.Unlock()
	if err := store.configure(context.Background(), newDB); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (m Manager) OpenWorktreeKey(worktreeKey string) (*Store, error) {
	if m.opts.CacheHome != "" {
		if err := os.MkdirAll(m.opts.CacheHome, 0o755); err != nil {
			return nil, err
		}
	}
	cacheRoot, err := wsstate.CacheRoot(wsstate.Options{CacheHome: m.opts.CacheHome})
	if err != nil {
		return nil, err
	}
	layout, err := wsstate.LayoutForWorktreeKey(cacheRoot, worktreeKey)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(layout.WorktreeDir, "state.sqlite")
	newDB := !fileExists(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path, layout: layout, now: m.now, writeMu: writeLock(path)}
	openMu.Lock()
	defer openMu.Unlock()
	if err := store.configure(context.Background(), newDB); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (m Manager) now() time.Time {
	if m.opts.Now != nil {
		return m.opts.Now()
	}
	return time.Now()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

func (s *Store) Layout() wsstate.Layout { return s.layout }

func (s *Store) configure(ctx context.Context, setJournalMode bool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	statements := []string{
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
	}
	if setJournalMode {
		statements = append([]string{`PRAGMA journal_mode=WAL`}, statements...)
	}
	for _, stmt := range statements {
		if err := withSQLiteRetry(ctx, func() error {
			_, err := s.db.ExecContext(ctx, stmt)
			return err
		}); err != nil {
			return fmt.Errorf("configure sqlite %s: %w", stmt, err)
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return withSQLiteRetry(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, stmt := range schemaStatements {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migrate state store: %w", err)
			}
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		if err := migrateAgentDefsColumns(ctx, tx); err != nil {
			return err
		}
		if err := migrateAgentInstanceColumns(ctx, tx); err != nil {
			return err
		}
		if err := migrateAgentDefinitionsToInstances(ctx, tx); err != nil {
			return err
		}
		if err := migrateExecJobsColumns(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`, schemaVersion, now); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func migrateAgentDefsColumns(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(agent_defs)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range agentDefColumnMigrations {
		if columns[column.name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE agent_defs ADD COLUMN `+column.sql); err != nil {
			return fmt.Errorf("migrate agent_defs.%s: %w", column.name, err)
		}
	}
	return nil
}

var agentDefColumnMigrations = []struct{ name, sql string }{
	{"public_name", `public_name TEXT NOT NULL DEFAULT ''`},
	{"schema_version", `schema_version INTEGER NOT NULL DEFAULT 0`},
	{"backend", `backend TEXT NOT NULL DEFAULT ''`},
	{"harness", `harness TEXT NOT NULL DEFAULT ''`},
	{"tier", `tier TEXT NOT NULL DEFAULT ''`},
	{"model", `model TEXT NOT NULL DEFAULT ''`},
	{"effort", `effort TEXT NOT NULL DEFAULT ''`},
	{"session_id", `session_id TEXT NOT NULL DEFAULT ''`},
	{"status", `status TEXT NOT NULL DEFAULT ''`},
	{"last_seen_at", `last_seen_at TEXT NOT NULL DEFAULT ''`},
	{"last_call_at", `last_call_at TEXT NOT NULL DEFAULT ''`},
	{"last_output_path", `last_output_path TEXT NOT NULL DEFAULT ''`},
	{"prompt_refs_json", `prompt_refs_json TEXT NOT NULL DEFAULT '[]'`},
	{"system_prompt_path", `system_prompt_path TEXT NOT NULL DEFAULT ''`},
	{"child_actor_id", `child_actor_id TEXT NOT NULL DEFAULT ''`},
	{"child_actor_authority", `child_actor_authority TEXT NOT NULL DEFAULT ''`},
	{"capabilities_json", `capabilities_json TEXT NOT NULL DEFAULT '{}'`},
	{"ephemeral", `ephemeral INTEGER NOT NULL DEFAULT 0`},
}

func migrateAgentInstanceColumns(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(agent_instances)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range agentInstanceColumnMigrations {
		if columns[column.name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE agent_instances ADD COLUMN `+column.sql); err != nil {
			return fmt.Errorf("migrate agent_instances.%s: %w", column.name, err)
		}
	}
	return nil
}

var agentInstanceColumnMigrations = []struct{ name, sql string }{
	{"retention_eligible_at", `retention_eligible_at TEXT NOT NULL DEFAULT ''`},
	{"retention_checked_at", `retention_checked_at TEXT NOT NULL DEFAULT ''`},
	{"retention_next_check_at", `retention_next_check_at TEXT NOT NULL DEFAULT ''`},
	{"cleanup_state", `cleanup_state TEXT NOT NULL DEFAULT ''`},
	{"cleanup_attempted_at", `cleanup_attempted_at TEXT NOT NULL DEFAULT ''`},
	{"cleanup_error", `cleanup_error TEXT NOT NULL DEFAULT ''`},
	{"pinned", `pinned INTEGER NOT NULL DEFAULT 0`},
}

func migrateAgentDefinitionsToInstances(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO agent_instances(instance_id, agent_key, actor_id, public_name, state_path, schema_version, backend, harness, tier, model, effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, prompt_refs_json, system_prompt_path, child_actor_id, child_actor_authority, capabilities_json, ephemeral, cleanup_state)
SELECT agent_key || ':' || CASE WHEN state_path = '' THEN 'default' ELSE state_path END, agent_key, actor_id, public_name, state_path, schema_version, backend, harness, tier, model, effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, prompt_refs_json, system_prompt_path, child_actor_id, child_actor_authority, capabilities_json, ephemeral, 'current'
FROM agent_defs
WHERE public_name != ''`)
	return err
}

func migrateExecJobsColumns(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(exec_jobs)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range execJobColumnMigrations {
		if columns[column.name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE exec_jobs ADD COLUMN `+column.sql); err != nil {
			return fmt.Errorf("migrate exec_jobs.%s: %w", column.name, err)
		}
	}
	return nil
}

var execJobColumnMigrations = []struct{ name, sql string }{
	{"schema_version", `schema_version INTEGER NOT NULL DEFAULT 0`},
	{"root_path", `root_path TEXT NOT NULL DEFAULT ''`},
	{"working_dir", `working_dir TEXT NOT NULL DEFAULT ''`},
	{"argv_json", `argv_json TEXT NOT NULL DEFAULT '[]'`},
	{"command", `command TEXT NOT NULL DEFAULT ''`},
	{"shell", `shell TEXT NOT NULL DEFAULT ''`},
	{"env_json", `env_json TEXT NOT NULL DEFAULT '{}'`},
	{"stdin_present", `stdin_present INTEGER NOT NULL DEFAULT 0`},
	{"stdin_bytes", `stdin_bytes INTEGER NOT NULL DEFAULT 0`},
	{"pid", `pid INTEGER NOT NULL DEFAULT 0`},
	{"started_at", `started_at TEXT NOT NULL DEFAULT ''`},
	{"completed_at", `completed_at TEXT NOT NULL DEFAULT ''`},
	{"exit_code", `exit_code INTEGER NOT NULL DEFAULT 0`},
	{"error", `error TEXT NOT NULL DEFAULT ''`},
	{"cancel_requested", `cancel_requested INTEGER NOT NULL DEFAULT 0`},
	{"lost_worker", `lost_worker INTEGER NOT NULL DEFAULT 0`},
	{"stdout_bytes", `stdout_bytes INTEGER NOT NULL DEFAULT 0`},
	{"stderr_bytes", `stderr_bytes INTEGER NOT NULL DEFAULT 0`},
	{"combined_bytes", `combined_bytes INTEGER NOT NULL DEFAULT 0`},
	{"pinned", `pinned INTEGER NOT NULL DEFAULT 0`},
	{"expires_at", `expires_at TEXT NOT NULL DEFAULT ''`},
	{"cleanup_state", `cleanup_state TEXT NOT NULL DEFAULT ''`},
}

func (s *Store) UpsertActor(ctx context.Context, actor Actor) error {
	if actor.ActorID == "" {
		return errors.New("actor_id is required")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.execWrite(ctx, `
INSERT INTO actors(actor_id, authority, root_path, worktree_key, parent_actor_id, status, pinned, created_at, last_seen_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(actor_id) DO UPDATE SET
  authority=excluded.authority,
  root_path=excluded.root_path,
  worktree_key=excluded.worktree_key,
  parent_actor_id=excluded.parent_actor_id,
  status=excluded.status,
  pinned=excluded.pinned,
  last_seen_at=excluded.last_seen_at`,
		actor.ActorID, actor.Authority, actor.RootPath, actor.WorktreeKey, actor.ParentActorID, blankDefault(actor.Status, "active"), boolInt(actor.Pinned), now, now)
	return err
}

func (s *Store) Actor(ctx context.Context, id string) (Actor, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT actor_id, authority, root_path, worktree_key, parent_actor_id, status, pinned FROM actors WHERE actor_id = ?`, id)
	var actor Actor
	var pinned int
	if err := row.Scan(&actor.ActorID, &actor.Authority, &actor.RootPath, &actor.WorktreeKey, &actor.ParentActorID, &actor.Status, &pinned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Actor{}, false, nil
		}
		return Actor{}, false, err
	}
	actor.Pinned = pinned != 0
	return actor, true, nil
}

func (s *Store) UpsertAgentDefinition(ctx context.Context, def AgentDefinition) error {
	if def.AgentKey == "" {
		return errors.New("agent_key is required")
	}
	if def.PublicName == "" {
		return errors.New("agent public name is required")
	}
	promptRefs, capabilitiesJSON, err := marshalAgentDefinitionJSON(def)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	createdAt := def.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	instanceID := def.InstanceID
	if instanceID == "" {
		instanceID = def.AgentKey + ":" + def.StatePath
	}
	if instanceID == def.AgentKey+":" {
		instanceID = def.AgentKey + ":default"
	}
	retentionEligibleAt := def.RetentionEligibleAt
	if retentionEligibleAt == "" && def.CleanupState != "current" {
		base := parseTime(def.LastCallAt)
		if base.IsZero() {
			base = parseTime(createdAt)
		}
		if !base.IsZero() {
			retentionEligibleAt = base.Add(AgentInstanceRetentionTTL).UTC().Format(time.RFC3339Nano)
		}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return withSQLiteRetry(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var oldInstanceID string
		_ = tx.QueryRowContext(ctx, `SELECT instance_id FROM agent_instances WHERE agent_key = ? AND state_path = (SELECT state_path FROM agent_defs WHERE agent_key = ?)`, def.AgentKey, def.AgentKey).Scan(&oldInstanceID)
		if oldInstanceID != "" && oldInstanceID != instanceID {
			eligible := s.agentRetentionEligibleAt(now)
			if _, err := tx.ExecContext(ctx, `UPDATE agent_instances SET cleanup_state = CASE WHEN cleanup_state = 'current' THEN 'retired' ELSE cleanup_state END, retention_eligible_at = CASE WHEN retention_eligible_at = '' THEN ? ELSE retention_eligible_at END, retention_next_check_at = CASE WHEN retention_next_check_at = '' THEN ? ELSE retention_next_check_at END, updated_at = ? WHERE instance_id = ?`, eligible, eligible, now, oldInstanceID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_instances(instance_id, agent_key, actor_id, public_name, state_path, schema_version, backend, harness, tier, model, effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, prompt_refs_json, system_prompt_path, child_actor_id, child_actor_authority, capabilities_json, ephemeral, retention_eligible_at, retention_checked_at, retention_next_check_at, cleanup_state, cleanup_attempted_at, cleanup_error, pinned)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(instance_id) DO UPDATE SET
  actor_id=excluded.actor_id, public_name=excluded.public_name, state_path=excluded.state_path, schema_version=excluded.schema_version, backend=excluded.backend, harness=excluded.harness, tier=excluded.tier, model=excluded.model, effort=excluded.effort, session_id=excluded.session_id, status=excluded.status, updated_at=excluded.updated_at, last_seen_at=excluded.last_seen_at, last_call_at=excluded.last_call_at, last_output_path=excluded.last_output_path, prompt_refs_json=excluded.prompt_refs_json, system_prompt_path=excluded.system_prompt_path, child_actor_id=excluded.child_actor_id, child_actor_authority=excluded.child_actor_authority, capabilities_json=excluded.capabilities_json, ephemeral=excluded.ephemeral, cleanup_state=excluded.cleanup_state, pinned=excluded.pinned`,
			instanceID, def.AgentKey, def.ActorID, def.PublicName, def.StatePath, def.SchemaVersion, def.Backend, def.Harness, def.Tier, def.Model, def.Effort, def.SessionID, def.Status, createdAt, now, def.LastSeenAt, def.LastCallAt, def.LastOutputPath, string(promptRefs), def.SystemPromptPath, def.ChildActorID, def.ChildActorAuthority, string(capabilitiesJSON), boolInt(def.Ephemeral), retentionEligibleAt, def.RetentionCheckedAt, blankDefault(def.RetentionNextCheckAt, retentionEligibleAt), blankDefault(def.CleanupState, "current"), def.CleanupAttemptedAt, def.CleanupError, boolInt(def.Pinned)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_defs(agent_key, actor_id, public_name, state_path, schema_version, backend, harness, tier, model, effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, prompt_refs_json, system_prompt_path, child_actor_id, child_actor_authority, capabilities_json, ephemeral)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_key) DO UPDATE SET
  actor_id=excluded.actor_id, public_name=excluded.public_name, state_path=excluded.state_path, schema_version=excluded.schema_version, backend=excluded.backend, harness=excluded.harness, tier=excluded.tier, model=excluded.model, effort=excluded.effort, session_id=excluded.session_id, status=excluded.status, updated_at=excluded.updated_at, last_seen_at=excluded.last_seen_at, last_call_at=excluded.last_call_at, last_output_path=excluded.last_output_path, prompt_refs_json=excluded.prompt_refs_json, system_prompt_path=excluded.system_prompt_path, child_actor_id=excluded.child_actor_id, child_actor_authority=excluded.child_actor_authority, capabilities_json=excluded.capabilities_json, ephemeral=excluded.ephemeral`,
			def.AgentKey, def.ActorID, def.PublicName, def.StatePath, def.SchemaVersion, def.Backend, def.Harness, def.Tier, def.Model, def.Effort, def.SessionID, def.Status, createdAt, now, def.LastSeenAt, def.LastCallAt, def.LastOutputPath, string(promptRefs), def.SystemPromptPath, def.ChildActorID, def.ChildActorAuthority, string(capabilitiesJSON), boolInt(def.Ephemeral)); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func marshalAgentDefinitionJSON(def AgentDefinition) ([]byte, []byte, error) {
	promptRefs, err := json.Marshal(def.PromptRefs)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal prompt refs: %w", err)
	}
	capabilities := def.Capabilities
	if capabilities == nil {
		capabilities = map[string]bool{}
	}
	capabilitiesJSON, err := json.Marshal(capabilities)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal capabilities: %w", err)
	}
	return promptRefs, capabilitiesJSON, nil
}

func (s *Store) agentRetentionEligibleAt(now string) string {
	base := parseTime(now)
	if base.IsZero() {
		base = s.now().UTC()
	}
	return base.Add(AgentInstanceRetentionTTL).UTC().Format(time.RFC3339Nano)
}

func (s *Store) AgentDefinition(ctx context.Context, agentKey string) (AgentDefinition, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT d.agent_key, d.actor_id, d.public_name, d.state_path, d.schema_version, d.backend, d.harness, d.tier, d.model, d.effort, d.session_id, d.status, d.created_at, d.last_seen_at, d.last_call_at, d.last_output_path, d.prompt_refs_json, d.system_prompt_path, d.child_actor_id, d.child_actor_authority, d.capabilities_json, d.ephemeral, COALESCE(i.instance_id, '') FROM agent_defs d LEFT JOIN agent_instances i ON i.agent_key = d.agent_key AND i.state_path = d.state_path WHERE d.agent_key = ?`, agentKey)
	var def AgentDefinition
	var promptRefsJSON, capabilitiesJSON string
	var ephemeral int
	if err := row.Scan(&def.AgentKey, &def.ActorID, &def.PublicName, &def.StatePath, &def.SchemaVersion, &def.Backend, &def.Harness, &def.Tier, &def.Model, &def.Effort, &def.SessionID, &def.Status, &def.CreatedAt, &def.LastSeenAt, &def.LastCallAt, &def.LastOutputPath, &promptRefsJSON, &def.SystemPromptPath, &def.ChildActorID, &def.ChildActorAuthority, &capabilitiesJSON, &ephemeral, &def.InstanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentDefinition{}, false, nil
		}
		return AgentDefinition{}, false, err
	}
	if err := json.Unmarshal([]byte(blankDefault(promptRefsJSON, "[]")), &def.PromptRefs); err != nil {
		return AgentDefinition{}, false, fmt.Errorf("parse agent prompt refs: %w", err)
	}
	if err := json.Unmarshal([]byte(blankDefault(capabilitiesJSON, "{}")), &def.Capabilities); err != nil {
		return AgentDefinition{}, false, fmt.Errorf("parse agent capabilities: %w", err)
	}
	def.Ephemeral = ephemeral != 0
	return def, true, nil
}

func (s *Store) DeleteAgentDefinition(ctx context.Context, agentKey string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	eligible := s.agentRetentionEligibleAt(now)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return withSQLiteRetry(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var statePath string
		_ = tx.QueryRowContext(ctx, `SELECT state_path FROM agent_defs WHERE agent_key = ?`, agentKey).Scan(&statePath)
		if statePath != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE agent_instances SET cleanup_state = 'retired', retention_eligible_at = CASE WHEN retention_eligible_at = '' THEN ? ELSE retention_eligible_at END, retention_next_check_at = CASE WHEN retention_next_check_at = '' THEN ? ELSE retention_next_check_at END, updated_at = ? WHERE agent_key = ? AND state_path = ?`, eligible, eligible, now, agentKey, statePath); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_defs WHERE agent_key = ?`, agentKey); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func (s *Store) UpsertExecJob(ctx context.Context, job ExecJob) error {
	if job.ExecKey == "" {
		return errors.New("exec_key is required")
	}
	if job.Status == "" {
		return errors.New("exec status is required")
	}
	argv, err := json.Marshal(job.Argv)
	if err != nil {
		return fmt.Errorf("marshal exec argv: %w", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	createdAt := job.StartedAt
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := job.UpdatedAt
	if updatedAt == "" {
		updatedAt = now
	}
	_, err = s.execWrite(ctx, `
INSERT INTO exec_jobs(exec_key, owner_actor_id, status, lease_id, schema_version, root_path, working_dir, argv_json, command, shell, env_json, stdin_present, stdin_bytes, pid, started_at, completed_at, exit_code, error, cancel_requested, lost_worker, stdout_path, stderr_path, combined_path, stdout_bytes, stderr_bytes, combined_bytes, pinned, expires_at, cleanup_state, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(exec_key) DO UPDATE SET
  owner_actor_id=excluded.owner_actor_id,
  status=excluded.status,
  lease_id=excluded.lease_id,
  schema_version=excluded.schema_version,
  root_path=excluded.root_path,
  working_dir=excluded.working_dir,
  argv_json=excluded.argv_json,
  command=excluded.command,
  shell=excluded.shell,
  env_json=excluded.env_json,
  stdin_present=excluded.stdin_present,
  stdin_bytes=excluded.stdin_bytes,
  pid=excluded.pid,
  started_at=excluded.started_at,
  completed_at=excluded.completed_at,
  exit_code=excluded.exit_code,
  error=excluded.error,
  cancel_requested=excluded.cancel_requested,
  lost_worker=excluded.lost_worker,
  stdout_path=excluded.stdout_path,
  stderr_path=excluded.stderr_path,
  combined_path=excluded.combined_path,
  stdout_bytes=excluded.stdout_bytes,
  stderr_bytes=excluded.stderr_bytes,
  combined_bytes=excluded.combined_bytes,
  pinned=excluded.pinned,
  expires_at=excluded.expires_at,
  cleanup_state=excluded.cleanup_state,
  updated_at=excluded.updated_at`,
		job.ExecKey, job.OwnerActorID, job.Status, job.LeaseID, job.SchemaVersion, job.Root, job.WorkingDir, string(argv), job.Command, job.Shell, blankDefault(job.EnvJSON, "{}"), boolInt(job.StdinPresent), job.StdinBytes, job.PID, job.StartedAt, job.CompletedAt, job.ExitCode, job.Error, boolInt(job.CancelRequested), boolInt(job.LostWorker), job.StdoutPath, job.StderrPath, job.CombinedPath, job.StdoutBytes, job.StderrBytes, job.CombinedBytes, boolInt(job.Pinned), job.ExpiresAt, job.CleanupState, createdAt, updatedAt)
	if err != nil {
		return err
	}
	return s.upsertExecStreamArtifacts(ctx, job)
}

func (s *Store) upsertExecStreamArtifacts(ctx context.Context, job ExecJob) error {
	state := execArtifactState(job)
	expires := parseTime(job.ExpiresAt)
	streams := []struct {
		name  string
		path  string
		bytes int64
	}{
		{"stdout", job.StdoutPath, job.StdoutBytes},
		{"stderr", job.StderrPath, job.StderrBytes},
		{"combined", job.CombinedPath, job.CombinedBytes},
	}
	for _, stream := range streams {
		if stream.path == "" {
			continue
		}
		if err := s.UpsertArtifact(ctx, Artifact{
			ArtifactID:   "exec:" + job.ExecKey + ":" + stream.name,
			Kind:         "exec." + stream.name,
			Path:         stream.path,
			OwnerActorID: job.OwnerActorID,
			State:        state,
			ByteCount:    stream.bytes,
			Pinned:       job.Pinned,
			ExpiresAt:    expires,
		}); err != nil {
			return err
		}
	}
	return nil
}

func execArtifactState(job ExecJob) string {
	if job.Pinned {
		return ArtifactStateCompleted
	}
	if job.LeaseID != "" {
		return ArtifactStateLeased
	}
	if job.CleanupState != "" {
		return job.CleanupState
	}
	switch job.Status {
	case "running":
		return ArtifactStateRunning
	case "cancel_requested":
		return ArtifactStateCancelRequested
	default:
		return ArtifactStateCompleted
	}
}

func (s *Store) ExecJob(ctx context.Context, key string) (ExecJob, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT exec_key, owner_actor_id, status, lease_id, schema_version, root_path, working_dir, argv_json, command, shell, env_json, stdin_present, stdin_bytes, pid, started_at, updated_at, completed_at, exit_code, error, cancel_requested, lost_worker, stdout_path, stderr_path, combined_path, stdout_bytes, stderr_bytes, combined_bytes, pinned, expires_at, cleanup_state FROM exec_jobs WHERE exec_key = ?`, key)
	var job ExecJob
	var argvJSON string
	var stdinPresent, cancelRequested, lostWorker, pinned int
	if err := row.Scan(&job.ExecKey, &job.OwnerActorID, &job.Status, &job.LeaseID, &job.SchemaVersion, &job.Root, &job.WorkingDir, &argvJSON, &job.Command, &job.Shell, &job.EnvJSON, &stdinPresent, &job.StdinBytes, &job.PID, &job.StartedAt, &job.UpdatedAt, &job.CompletedAt, &job.ExitCode, &job.Error, &cancelRequested, &lostWorker, &job.StdoutPath, &job.StderrPath, &job.CombinedPath, &job.StdoutBytes, &job.StderrBytes, &job.CombinedBytes, &pinned, &job.ExpiresAt, &job.CleanupState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExecJob{}, false, nil
		}
		return ExecJob{}, false, err
	}
	if err := json.Unmarshal([]byte(blankDefault(argvJSON, "[]")), &job.Argv); err != nil {
		return ExecJob{}, false, fmt.Errorf("parse exec argv: %w", err)
	}
	job.StdinPresent = stdinPresent != 0
	job.CancelRequested = cancelRequested != 0
	job.LostWorker = lostWorker != 0
	job.Pinned = pinned != 0
	return job, true, nil
}

func (s *Store) UpsertArtifact(ctx context.Context, artifact Artifact) error {
	if artifact.ArtifactID == "" {
		return errors.New("artifact_id is required")
	}
	if artifact.Path == "" {
		return errors.New("artifact path is required")
	}
	now := s.now().UTC()
	state := blankDefault(artifact.State, ArtifactStateCompleted)
	expiresAt := timeString(artifact.ExpiresAt)
	lastAccessed := timeString(artifact.LastAccessedAt)
	if lastAccessed == "" {
		lastAccessed = now.Format(time.RFC3339Nano)
	}
	_, err := s.execWrite(ctx, `
INSERT INTO artifacts(artifact_id, kind, path, owner_actor_id, state, byte_count, pinned, expires_at, last_accessed_at, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(artifact_id) DO UPDATE SET
  kind=excluded.kind,
  path=excluded.path,
  owner_actor_id=excluded.owner_actor_id,
  state=excluded.state,
  byte_count=excluded.byte_count,
  pinned=excluded.pinned,
  expires_at=excluded.expires_at,
  last_accessed_at=excluded.last_accessed_at,
  updated_at=excluded.updated_at`,
		artifact.ArtifactID, artifact.Kind, artifact.Path, artifact.OwnerActorID, state, artifact.ByteCount, boolInt(artifact.Pinned), expiresAt, lastAccessed, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return err
}

func (s *Store) UpsertRetentionPolicy(ctx context.Context, policy RetentionPolicy) error {
	if policy.Scope == "" {
		return errors.New("retention policy scope is required")
	}
	_, err := s.execWrite(ctx, `
INSERT INTO retention_policies(scope, ttl_seconds, max_rows, max_bytes, updated_at)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(scope) DO UPDATE SET
  ttl_seconds=excluded.ttl_seconds,
  max_rows=excluded.max_rows,
  max_bytes=excluded.max_bytes,
  updated_at=excluded.updated_at`,
		policy.Scope, int64(policy.TTL.Seconds()), policy.MaxRows, policy.MaxBytes, s.now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Artifact(ctx context.Context, id string) (Artifact, bool, error) {
	var a Artifact
	var pinned int
	var expires, last string
	err := s.db.QueryRowContext(ctx, `SELECT artifact_id, kind, path, owner_actor_id, state, byte_count, pinned, expires_at, last_accessed_at FROM artifacts WHERE artifact_id = ?`, id).Scan(&a.ArtifactID, &a.Kind, &a.Path, &a.OwnerActorID, &a.State, &a.ByteCount, &pinned, &expires, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, false, nil
	}
	if err != nil {
		return Artifact{}, false, err
	}
	a.Pinned = pinned != 0
	a.ExpiresAt = parseTime(expires)
	a.LastAccessedAt = parseTime(last)
	return a, true, nil
}

func (s *Store) PruneExpired(ctx context.Context, opts PruneOptions) (PruneResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	now := s.now().UTC()
	result := PruneResult{}
	runID, err := s.beginPruneRun(ctx, now)
	if err != nil {
		return result, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT artifact_id, path FROM artifacts
WHERE pinned = 0
  AND state NOT IN (?, ?, ?, ?)
  AND expires_at != ''
  AND expires_at <= ?
ORDER BY expires_at ASC
LIMIT ?`,
		ArtifactStateActive, ArtifactStateRunning, ArtifactStateCancelRequested, ArtifactStateLeased, now.Format(time.RFC3339Nano), limit)
	if err != nil {
		_ = s.finishPruneRun(ctx, runID, result, err)
		return result, err
	}
	defer rows.Close()
	type candidate struct{ id, path string }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.path); err != nil {
			_ = s.finishPruneRun(ctx, runID, result, err)
			return result, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		_ = s.finishPruneRun(ctx, runID, result, err)
		return result, err
	}
	for _, c := range candidates {
		result.Scanned++
		if err := removeArtifactPath(c.path); err != nil {
			result.Tombstoned++
			if tombErr := s.recordTombstone(ctx, c.id, c.path, "prune", err); tombErr != nil {
				_ = s.finishPruneRun(ctx, runID, result, tombErr)
				return result, tombErr
			}
			continue
		}
		if _, err := s.execWrite(ctx, `DELETE FROM artifacts WHERE artifact_id = ?`, c.id); err != nil {
			_ = s.finishPruneRun(ctx, runID, result, err)
			return result, err
		}
		result.Deleted++
	}
	if err := s.retryTombstones(ctx, &result, limit); err != nil {
		_ = s.finishPruneRun(ctx, runID, result, err)
		return result, err
	}
	return result, s.finishPruneRun(ctx, runID, result, nil)
}

func (s *Store) PruneAgentInstances(ctx context.Context, opts PruneOptions) (AgentInstanceCleanupResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	nowTime := s.now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	runID, err := s.beginPruneRun(ctx, nowTime)
	if err != nil {
		return AgentInstanceCleanupResult{}, err
	}
	finish := func(result AgentInstanceCleanupResult, runErr error) error {
		pruneResult := PruneResult{
			Scanned:    result.Scanned,
			Deleted:    result.Deleted,
			Tombstoned: result.Failed,
		}
		return s.finishPruneRun(ctx, runID, pruneResult, runErr)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT i.instance_id, i.state_path FROM agent_instances i
LEFT JOIN agent_defs d ON d.agent_key = i.agent_key AND d.state_path = i.state_path
WHERE d.agent_key IS NULL
  AND i.pinned = 0
  AND i.status NOT IN ('running')
  AND i.cleanup_state NOT IN ('current', 'active', 'running', 'queued', 'recovery', 'cleanup_deleted')
  AND i.retention_eligible_at != ''
  AND i.retention_eligible_at <= ?
  AND (i.retention_next_check_at = '' OR i.retention_next_check_at <= ?)
ORDER BY i.retention_eligible_at ASC
LIMIT ?`, now, now, limit)
	if err != nil {
		_ = finish(AgentInstanceCleanupResult{}, err)
		return AgentInstanceCleanupResult{}, err
	}
	defer rows.Close()
	type candidate struct{ id, path string }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.path); err != nil {
			_ = finish(AgentInstanceCleanupResult{}, err)
			return AgentInstanceCleanupResult{}, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		_ = finish(AgentInstanceCleanupResult{}, err)
		return AgentInstanceCleanupResult{}, err
	}
	result := AgentInstanceCleanupResult{}
	for _, c := range candidates {
		result.Scanned++
		absPath := c.path
		if absPath != "" && !filepath.IsAbs(absPath) {
			absPath = filepath.Join(s.layout.AgentsDir, absPath)
		}
		active, activeErr := agentInstanceHasActiveCurrentState(absPath)
		if activeErr != nil {
			result.Skipped++
			next := s.now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
			if _, err := s.execWrite(ctx, `UPDATE agent_instances SET retention_checked_at = ?, retention_next_check_at = ?, cleanup_error = ?, updated_at = ? WHERE instance_id = ?`, now, next, activeErr.Error(), now, c.id); err != nil {
				_ = finish(result, err)
				return result, err
			}
			continue
		}
		if active {
			result.Skipped++
			next := s.now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
			if _, err := s.execWrite(ctx, `UPDATE agent_instances SET retention_checked_at = ?, retention_next_check_at = ?, cleanup_error = ?, updated_at = ? WHERE instance_id = ?`, now, next, "active current call state", now, c.id); err != nil {
				_ = finish(result, err)
				return result, err
			}
			continue
		}
		if err := os.RemoveAll(absPath); err != nil {
			result.Failed++
			next := s.now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
			if _, err := s.execWrite(ctx, `UPDATE agent_instances SET cleanup_state = 'cleanup_failed', cleanup_attempted_at = ?, retention_checked_at = ?, retention_next_check_at = ?, cleanup_error = ?, updated_at = ? WHERE instance_id = ?`, now, now, next, err.Error(), now, c.id); err != nil {
				_ = finish(result, err)
				return result, err
			}
			continue
		}
		if _, err := s.execWrite(ctx, `UPDATE agent_instances SET cleanup_state = 'cleanup_deleted', cleanup_attempted_at = ?, retention_checked_at = ?, cleanup_error = '', updated_at = ? WHERE instance_id = ?`, now, now, now, c.id); err != nil {
			_ = finish(result, err)
			return result, err
		}
		result.Deleted++
	}
	return result, finish(result, nil)
}

func agentInstanceHasActiveCurrentState(agentDir string) (bool, error) {
	if agentDir == "" {
		return false, nil
	}
	if strings.ContainsRune(agentDir, 0) {
		return false, nil
	}
	raw, err := os.ReadFile(filepath.Join(agentDir, "current", "state.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	var state struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return false, err
	}
	switch state.Status {
	case "queued", "running":
		return true, nil
	default:
		return false, nil
	}
}

func (s *Store) Count(ctx context.Context, table string) (int, error) {
	switch table {
	case "artifacts", "artifact_tombstones", "actors", "prune_runs", "retention_policies", "exec_jobs", "agent_defs", "agent_instances":
	default:
		return 0, fmt.Errorf("unsupported count table %q", table)
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count)
	return count, err
}

func (s *Store) beginPruneRun(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.execWrite(ctx, `INSERT INTO prune_runs(started_at, status) VALUES(?, ?)`, now.Format(time.RFC3339Nano), "running")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) finishPruneRun(ctx context.Context, id int64, result PruneResult, runErr error) error {
	status := "succeeded"
	errText := ""
	if runErr != nil {
		status = "failed"
		errText = runErr.Error()
	}
	_, err := s.execWrite(ctx, `UPDATE prune_runs SET finished_at = ?, status = ?, scanned = ?, deleted = ?, tombstoned = ?, retried = ?, retry_succeeded = ?, error = ? WHERE run_id = ?`,
		s.now().UTC().Format(time.RFC3339Nano), status, result.Scanned, result.Deleted, result.Tombstoned, result.Retried, result.RetrySucceeded, errText, id)
	if err != nil {
		return err
	}
	return runErr
}

func (s *Store) recordTombstone(ctx context.Context, artifactID, path, reason string, cleanupErr error) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.execWrite(ctx, `
INSERT INTO artifact_tombstones(artifact_id, path, reason, attempts, next_retry_at, last_error, created_at, updated_at)
VALUES(?, ?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(artifact_id) DO UPDATE SET
  attempts=artifact_tombstones.attempts + 1,
  next_retry_at=excluded.next_retry_at,
  last_error=excluded.last_error,
  updated_at=excluded.updated_at`, artifactID, path, reason, now, cleanupErr.Error(), now, now)
	if err != nil {
		return err
	}
	_, err = s.execWrite(ctx, `UPDATE artifacts SET state = ?, updated_at = ? WHERE artifact_id = ?`, ArtifactStateCleanupFailed, now, artifactID)
	return err
}

func (s *Store) retryTombstones(ctx context.Context, result *PruneResult, limit int) error {
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, path FROM artifact_tombstones WHERE next_retry_at <= ? ORDER BY updated_at ASC LIMIT ?`, s.now().UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	type retry struct{ id, path string }
	var retries []retry
	for rows.Next() {
		var r retry
		if err := rows.Scan(&r.id, &r.path); err != nil {
			return err
		}
		retries = append(retries, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range retries {
		result.Retried++
		if err := removeArtifactPath(r.path); err != nil {
			if err := s.recordTombstone(ctx, r.id, r.path, "retry", err); err != nil {
				return err
			}
			continue
		}
		if _, err := s.execWrite(ctx, `DELETE FROM artifact_tombstones WHERE artifact_id = ?`, r.id); err != nil {
			return err
		}
		if _, err := s.execWrite(ctx, `DELETE FROM artifacts WHERE artifact_id = ?`, r.id); err != nil {
			return err
		}
		result.RetrySucceeded++
	}
	return nil
}

func removeArtifactPath(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func writeLock(path string) *sync.Mutex {
	writeLocksMu.Lock()
	defer writeLocksMu.Unlock()
	if mu, ok := writeLocks[path]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	writeLocks[path] = mu
	return mu
}

func (s *Store) execWrite(ctx context.Context, query string, args ...any) (sql.Result, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return withSQLiteResultRetry(ctx, func() (sql.Result, error) {
		return s.db.ExecContext(ctx, query, args...)
	})
}

func blankDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS actors (
		actor_id TEXT PRIMARY KEY,
		authority TEXT NOT NULL,
		root_path TEXT NOT NULL,
		worktree_key TEXT NOT NULL,
		parent_actor_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		pinned INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS mcp_sessions (
		session_id TEXT PRIMARY KEY,
		actor_id TEXT NOT NULL,
		process_id INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS agent_defs (
		agent_key TEXT PRIMARY KEY,
		actor_id TEXT NOT NULL DEFAULT '',
		public_name TEXT NOT NULL DEFAULT '',
		state_path TEXT NOT NULL DEFAULT '',
		schema_version INTEGER NOT NULL DEFAULT 0,
		backend TEXT NOT NULL DEFAULT '',
		harness TEXT NOT NULL DEFAULT '',
		tier TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		effort TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		last_seen_at TEXT NOT NULL DEFAULT '',
		last_call_at TEXT NOT NULL DEFAULT '',
		last_output_path TEXT NOT NULL DEFAULT '',
		prompt_refs_json TEXT NOT NULL DEFAULT '[]',
		system_prompt_path TEXT NOT NULL DEFAULT '',
		child_actor_id TEXT NOT NULL DEFAULT '',
		child_actor_authority TEXT NOT NULL DEFAULT '',
		capabilities_json TEXT NOT NULL DEFAULT '{}',
		ephemeral INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS agent_instances (
		instance_id TEXT PRIMARY KEY,
		agent_key TEXT NOT NULL,
		actor_id TEXT NOT NULL DEFAULT '',
		public_name TEXT NOT NULL DEFAULT '',
		state_path TEXT NOT NULL DEFAULT '',
		schema_version INTEGER NOT NULL DEFAULT 0,
		backend TEXT NOT NULL DEFAULT '',
		harness TEXT NOT NULL DEFAULT '',
		tier TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		effort TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		last_seen_at TEXT NOT NULL DEFAULT '',
		last_call_at TEXT NOT NULL DEFAULT '',
		last_output_path TEXT NOT NULL DEFAULT '',
		prompt_refs_json TEXT NOT NULL DEFAULT '[]',
		system_prompt_path TEXT NOT NULL DEFAULT '',
		child_actor_id TEXT NOT NULL DEFAULT '',
		child_actor_authority TEXT NOT NULL DEFAULT '',
		capabilities_json TEXT NOT NULL DEFAULT '{}',
		ephemeral INTEGER NOT NULL DEFAULT 0,
		retention_eligible_at TEXT NOT NULL DEFAULT '',
		retention_checked_at TEXT NOT NULL DEFAULT '',
		retention_next_check_at TEXT NOT NULL DEFAULT '',
		cleanup_state TEXT NOT NULL DEFAULT '',
		cleanup_attempted_at TEXT NOT NULL DEFAULT '',
		cleanup_error TEXT NOT NULL DEFAULT '',
		pinned INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS agent_instances_role_idx ON agent_instances(agent_key, created_at)`,
	`CREATE INDEX IF NOT EXISTS agent_instances_cleanup_idx ON agent_instances(cleanup_state, retention_next_check_at, retention_eligible_at)`,
	`CREATE TABLE IF NOT EXISTS agent_calls (
		call_id TEXT PRIMARY KEY,
		agent_key TEXT NOT NULL,
		owner_actor_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		lease_id TEXT NOT NULL DEFAULT '',
		prompt_path TEXT NOT NULL DEFAULT '',
		output_path TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS exec_jobs (
		exec_key TEXT PRIMARY KEY,
		owner_actor_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		lease_id TEXT NOT NULL DEFAULT '',
		schema_version INTEGER NOT NULL DEFAULT 0,
		root_path TEXT NOT NULL DEFAULT '',
		working_dir TEXT NOT NULL DEFAULT '',
		argv_json TEXT NOT NULL DEFAULT '[]',
		command TEXT NOT NULL DEFAULT '',
		shell TEXT NOT NULL DEFAULT '',
		env_json TEXT NOT NULL DEFAULT '{}',
		stdin_present INTEGER NOT NULL DEFAULT 0,
		stdin_bytes INTEGER NOT NULL DEFAULT 0,
		pid INTEGER NOT NULL DEFAULT 0,
		started_at TEXT NOT NULL DEFAULT '',
		completed_at TEXT NOT NULL DEFAULT '',
		exit_code INTEGER NOT NULL DEFAULT 0,
		error TEXT NOT NULL DEFAULT '',
		cancel_requested INTEGER NOT NULL DEFAULT 0,
		lost_worker INTEGER NOT NULL DEFAULT 0,
		stdout_path TEXT NOT NULL DEFAULT '',
		stderr_path TEXT NOT NULL DEFAULT '',
		combined_path TEXT NOT NULL DEFAULT '',
		stdout_bytes INTEGER NOT NULL DEFAULT 0,
		stderr_bytes INTEGER NOT NULL DEFAULT 0,
		combined_bytes INTEGER NOT NULL DEFAULT 0,
		pinned INTEGER NOT NULL DEFAULT 0,
		expires_at TEXT NOT NULL DEFAULT '',
		cleanup_state TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS worker_leases (
		lease_id TEXT PRIMARY KEY,
		owner_kind TEXT NOT NULL,
		owner_id TEXT NOT NULL,
		pid INTEGER NOT NULL DEFAULT 0,
		generation INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		heartbeat_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS retention_policies (
		scope TEXT PRIMARY KEY,
		ttl_seconds INTEGER NOT NULL DEFAULT 0,
		max_rows INTEGER NOT NULL DEFAULT 0,
		max_bytes INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS artifacts (
		artifact_id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		path TEXT NOT NULL,
		owner_actor_id TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL,
		byte_count INTEGER NOT NULL DEFAULT 0,
		pinned INTEGER NOT NULL DEFAULT 0,
		expires_at TEXT NOT NULL DEFAULT '',
		last_accessed_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS artifacts_prune_idx ON artifacts(pinned, state, expires_at)`,
	`CREATE TABLE IF NOT EXISTS artifact_tombstones (
		artifact_id TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		reason TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		next_retry_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS prune_runs (
		run_id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		scanned INTEGER NOT NULL DEFAULT 0,
		deleted INTEGER NOT NULL DEFAULT 0,
		tombstoned INTEGER NOT NULL DEFAULT 0,
		retried INTEGER NOT NULL DEFAULT 0,
		retry_succeeded INTEGER NOT NULL DEFAULT 0,
		error TEXT NOT NULL DEFAULT ''
	)`,
}

package wsstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("configure sqlite %s: %w", stmt, err)
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`, schemaVersion, now); err != nil {
		return err
	}
	return tx.Commit()
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

func (s *Store) Count(ctx context.Context, table string) (int, error) {
	switch table {
	case "artifacts", "artifact_tombstones", "actors", "prune_runs", "retention_policies":
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
	return s.db.ExecContext(ctx, query, args...)
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
		state_path TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	)`,
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
		stdout_path TEXT NOT NULL DEFAULT '',
		stderr_path TEXT NOT NULL DEFAULT '',
		combined_path TEXT NOT NULL DEFAULT '',
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

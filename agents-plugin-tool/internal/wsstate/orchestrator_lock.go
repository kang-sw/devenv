package wsstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const orchestratorLockFile = "orchestrator.lock"

type OrchestratorLock struct {
	SchemaVersion int    `json:"schema_version"`
	PID           int    `json:"pid"`
	StartedAt     string `json:"started_at"`
	Root          string `json:"root"`
	WorktreeKey   string `json:"worktree_key"`
	Version       string `json:"version"`
}

type OrchestratorLockResult struct {
	Owner bool
	Path  string
	Lock  OrchestratorLock
}

func (m Manager) AcquireOrchestratorLock(repoPath, version string) (OrchestratorLockResult, error) {
	layout, _, worktree, err := m.Ensure(repoPath)
	if err != nil {
		return OrchestratorLockResult{}, err
	}
	path := filepath.Join(layout.WorktreeLocksDir, orchestratorLockFile)
	result := OrchestratorLockResult{Path: path}
	lock := OrchestratorLock{
		SchemaVersion: schemaVersion,
		PID:           os.Getpid(),
		StartedAt:     m.now().UTC().Format(time.RFC3339),
		Root:          worktree.WorktreePath,
		WorktreeKey:   worktree.WorktreeKey,
		Version:       version,
	}
	if owner, err := createOrchestratorLock(path, lock); err != nil {
		return result, err
	} else if owner {
		result.Owner = true
		result.Lock = lock
		return result, nil
	}

	existing, err := readOrchestratorLock(path)
	if err != nil {
		return result, nil
	}
	result.Lock = existing
	if existing.PID <= 0 || processAlive(existing.PID) {
		return result, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return result, nil
	}
	if owner, err := createOrchestratorLock(path, lock); err != nil {
		return result, err
	} else if owner {
		result.Owner = true
		result.Lock = lock
	}
	return result, nil
}

func createOrchestratorLock(path string, lock OrchestratorLock) (bool, error) {
	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return false, err
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("create orchestrator lock: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(raw); err != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("write orchestrator lock: %w", err)
	}
	return true, nil
}

func readOrchestratorLock(path string) (OrchestratorLock, error) {
	var lock OrchestratorLock
	raw, err := os.ReadFile(path)
	if err != nil {
		return lock, err
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		return lock, err
	}
	return lock, nil
}

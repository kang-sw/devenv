package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// worktreeMarkerFileName is the profile marker's file name in a linked
// worktree's Git admin directory, beside the worktree lease. Unlike the lease
// it survives release: it is what ties a pooled worktree to its profile across
// acquisitions. Git deletes it with the admin directory on worktree removal.
const worktreeMarkerFileName = "ws-worktree-profile.json"

const worktreeMarkerSchemaVersion = 1

// worktreeMarker records which profile a pooled worktree belongs to and the
// provisioning hash it was provisioned under. An empty ProvisionHash never
// matches a profile, so the next claim re-provisions: that is how a freshly
// created or legacy-adopted worktree is stamped before provisioning finishes.
type worktreeMarker struct {
	SchemaVersion int    `json:"schema_version"`
	Profile       string `json:"profile"`
	ProvisionHash string `json:"provision_hash"`
	// Quarantine is set when a post_provision or post_acquire hook failed; a
	// quarantined worktree is never reused.
	Quarantine *worktreeQuarantine `json:"quarantine,omitempty"`
	// PreservedBytes is the size of the preserved ignored artifacts the last
	// release left behind, measured then (never at acquire) for the
	// stale-pool nudge.
	PreservedBytes      int64  `json:"preserved_bytes,omitempty"`
	PreservedMeasuredAt string `json:"preserved_measured_at,omitempty"`
}

// readWorktreeMarker returns wtPath's marker, or nil when it has none (a
// legacy worktree created before profiles existed).
func readWorktreeMarker(ctx context.Context, runner wsgit.Runner, wtPath string) (*worktreeMarker, error) {
	dir, err := worktreeAdminDir(ctx, runner, wtPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, worktreeMarkerFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var marker worktreeMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, fmt.Errorf("parse worktree profile marker of %q: %v", wtPath, err)
	}
	return &marker, nil
}

// writeWorktreeMarker replaces wtPath's marker atomically (temp file + rename).
func writeWorktreeMarker(ctx context.Context, runner wsgit.Runner, wtPath string, marker worktreeMarker) error {
	dir, err := worktreeAdminDir(ctx, runner, wtPath)
	if err != nil {
		return err
	}
	marker.SchemaVersion = worktreeMarkerSchemaVersion
	payload, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "ws-worktree-profile-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, worktreeMarkerFileName)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

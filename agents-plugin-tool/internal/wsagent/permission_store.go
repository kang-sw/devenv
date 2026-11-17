package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

type permissionSelection struct {
	mu       sync.Mutex
	loaded   bool
	level    PermissionLevel
	explicit bool
	err      error
	// info is permission.json's stat taken before the last read, infoAt the
	// clock before that stat, and absent that neither the file nor a link
	// existed then; revalidatePermissionSelection compares them.
	info   os.FileInfo
	infoAt time.Time
	absent bool
}

// permissionRacyWindow is how recent a selection's mtime may be, relative to
// the stat that recorded it, and still be distrusted: a same-size rewrite
// within one coarse timestamp tick keeps the stat unchanged.
const permissionRacyWindow = 2 * time.Second

var permissionSelections sync.Map // session directory -> *permissionSelection

func permissionSlot(dir string) *permissionSelection {
	v, _ := permissionSelections.LoadOrStore(dir, new(permissionSelection))
	return v.(*permissionSelection)
}

func cachedPermissionSelection(dir string) (PermissionLevel, bool, error) {
	slot := permissionSlot(dir)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if !slot.loaded {
		slot.level, slot.explicit, slot.err = readPermissionSelection(dir)
		slot.loaded = true
	}
	return slot.level, slot.explicit, slot.err
}

// refreshPermissionSelection is for explicit API/admission reads, not spawn
// projection. A new launch must never trust an earlier cached permission.
func refreshPermissionSelection(dir string) (PermissionLevel, bool, error) {
	slot := permissionSlot(dir)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	slot.loadLocked(dir)
	return slot.level, slot.explicit, slot.err
}

// revalidatePermissionSelection is the hosted page's explicit state read
// (Runner.Get): it re-reads the selection only when permission.json's stat
// moved since the last read, so a page polling an unchanged lead opens no
// file. Another process may still edit the file: any change of identity,
// size, mode or mtime, or a recent mtime (permissionRacyWindow), re-reads.
// Launch admission keeps refreshPermissionSelection's unconditional read.
func revalidatePermissionSelection(dir string) {
	slot := permissionSlot(dir)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.loaded {
		info, absent := statPermissionSelection(dir)
		if absent && slot.absent {
			return
		}
		if info != nil && wsstate.SameFileMetadata(slot.info, info) && info.ModTime().Before(slot.infoAt.Add(-permissionRacyWindow)) {
			return
		}
	}
	slot.loadLocked(dir)
}

// loadLocked reads the selection, recording the stat taken before the read:
// a write between the two leaves a stat that the next revalidation finds
// moved. Caller holds slot.mu.
func (slot *permissionSelection) loadLocked(dir string) {
	slot.infoAt = time.Now()
	slot.info, slot.absent = statPermissionSelection(dir)
	slot.level, slot.explicit, slot.err = readPermissionSelection(dir)
	slot.loaded = true
}

// statPermissionSelection stats permission.json. absent reports that
// neither the file nor a dangling link exists (readPermissionMembers treats a
// dangling link as an unreadable selection, not omission).
func statPermissionSelection(dir string) (info os.FileInfo, absent bool) {
	path := filepath.Join(dir, "permission.json")
	info, err := wsstate.Stat(path)
	if err == nil {
		return info, false
	}
	if errors.Is(err, os.ErrNotExist) {
		if _, lerr := wsstate.Lstat(path); errors.Is(lerr, os.ErrNotExist) {
			return nil, true
		}
	}
	return nil, false
}

// readPermissionMembers distinguishes absence from an invalid existing selection.
func readPermissionMembers(dir string) (map[string]json.RawMessage, PermissionLevel, bool, error) {
	path := filepath.Join(dir, "permission.json")
	raw, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// A dangling link is an existing, unreadable selection, not omission.
		if _, statErr := wsstate.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
			return nil, "", false, nil
		}
	}
	if err != nil {
		return nil, "", true, fmt.Errorf("read permission selection: %w", err)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, "", true, fmt.Errorf("parse permission selection: %w", err)
	}
	var level PermissionLevel
	if members == nil || json.Unmarshal(members["permission_level"], &level) != nil || level.rank() == 0 {
		return nil, "", true, fmt.Errorf("invalid permission_level in %s", path)
	}
	return members, level, true, nil
}

func readPermissionSelection(dir string) (level PermissionLevel, explicit bool, err error) {
	_, level, explicit, err = readPermissionMembers(dir)
	return
}

// writePermissionSelection persists an already authorized choice. The caller's
// conversation loop serializes authorization, selection updates and admission.
func writePermissionSelection(dir string, level PermissionLevel) error {
	slot := permissionSlot(dir)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	slot.loaded = false
	if level.rank() == 0 {
		return fmt.Errorf("invalid permission level %q", level)
	}
	members, _, _, err := readPermissionMembers(dir)
	if err != nil {
		return err
	}
	if members == nil {
		members = make(map[string]json.RawMessage)
	}
	members["permission_level"], _ = json.Marshal(level)
	raw, err := json.MarshalIndent(members, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, "permission.json"), append(raw, '\n'), 0o600); err != nil {
		return err
	}
	slot.level, slot.explicit, slot.err, slot.loaded = level, true, nil, true
	slot.info, slot.absent = nil, false // the next revalidation re-reads once
	return nil
}

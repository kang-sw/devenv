package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kang-sw/devenv/internal/wsstate"
)

type permissionSelection struct {
	mu       sync.Mutex
	loaded   bool
	level    PermissionLevel
	explicit bool
	err      error
}

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
	return nil
}

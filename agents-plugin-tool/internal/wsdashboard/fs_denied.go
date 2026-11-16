package wsdashboard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Keep one immutable identity snapshot, scoped to the home spellings. Refresh
// base stats once per authorization, not once per candidate ancestor. A TTL
// alone would allow an alias to newly created/replaced credentials until it
// expired; missing bases and identity changes must rotate the snapshot now.
var deniedIdentityCache struct {
	sync.Mutex
	homes string
	infos []os.FileInfo
}

func deniedBaseIdentities(homes []string) ([]os.FileInfo, bool) {
	var fresh []os.FileInfo
	for _, home := range homes {
		for _, name := range outsideDenied {
			info, err := os.Stat(filepath.Join(home, filepath.FromSlash(name)))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, false
			}
			fresh = append(fresh, info)
		}
	}
	key := strings.Join(homes, "\x00")
	deniedIdentityCache.Lock()
	defer deniedIdentityCache.Unlock()
	same := deniedIdentityCache.homes == key && len(deniedIdentityCache.infos) == len(fresh)
	if same {
		for i, info := range fresh {
			if !os.SameFile(info, deniedIdentityCache.infos[i]) {
				same = false
				break
			}
		}
	}
	if !same {
		deniedIdentityCache.homes, deniedIdentityCache.infos = key, fresh
	}
	return deniedIdentityCache.infos, true
}

// Check the resolved file and each ancestor by identity. EvalSymlinks alone
// does not canonicalize APFS names or firmlinks; SameFile does not depend on
// their spelling. Stat also covers aliases of denylisted individual files.
func deniedFileIdentity(path string, homes []string) bool {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if deniedProcEnviron(path) {
		return true
	}
	bases, ok := deniedBaseIdentities(homes)
	if !ok {
		return true
	}
	if len(bases) == 0 {
		return false
	}
	for candidate := filepath.Clean(path); ; candidate = filepath.Dir(candidate) {
		if info, err := os.Stat(candidate); err == nil {
			for _, base := range bases {
				if os.SameFile(info, base) {
					return true
				}
			}
		}
		if filepath.Dir(candidate) == candidate {
			return false
		}
	}
}

func deniedProcEnviron(path string) bool {
	if runtime.GOOS != "linux" || !filepath.IsAbs(path) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	return len(parts) >= 3 && parts[0] == "proc" && parts[len(parts)-1] == "environ" &&
		(len(parts) == 3 || (len(parts) == 5 && parts[2] == "task"))
}

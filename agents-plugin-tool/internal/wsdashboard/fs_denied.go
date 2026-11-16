package wsdashboard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Keep the default locations protected even when a tool relocates its current
// credentials. Relocated homes protect the same credential files as defaults,
// not unrelated history/session files in those directories.
func deniedBasePaths(homes []string) []string {
	var bases []string
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" {
			return
		}
		path = filepath.FromSlash(homePath(path))
		if !filepath.IsAbs(path) {
			var err error
			path, err = filepath.Abs(path)
			if err != nil {
				return
			}
		}
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			bases = append(bases, path)
		}
	}
	for _, home := range homes {
		for _, name := range outsideDenied {
			add(filepath.Join(home, filepath.FromSlash(name)))
		}
	}
	for _, key := range []string{"GH_CONFIG_DIR", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE"} {
		add(os.Getenv(key))
	}
	for _, c := range []struct{ key, name string }{
		{"XDG_CONFIG_HOME", "gh"}, {"XDG_CONFIG_HOME", "gcloud"},
		{"CODEX_HOME", "auth.json"}, {"CLAUDE_CONFIG_DIR", ".credentials.json"},
	} {
		if dir := os.Getenv(c.key); dir != "" {
			add(filepath.Join(dir, c.name))
		}
	}
	for _, path := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
		add(path)
	}
	return bases
}

// Stat each base once for this authorization, rather than once per candidate
// ancestor. No cache: freshly created/replaced credentials are protected now.
// ENOTDIR, missing, and unreadable bases cannot supply identities, but remain
// lexically protected by alwaysDenied; one broken base never blocks other reads.
func deniedBaseIdentities(homes []string) []os.FileInfo {
	var infos []os.FileInfo
	for _, base := range deniedBasePaths(homes) {
		// Do not trigger network/device access just to inspect a configured
		// credential location. Such paths remain lexically denied.
		if networkPath(base) || deviceElement(base) {
			continue
		}
		if info, err := os.Stat(base); err == nil {
			infos = append(infos, info)
		}
	}
	return infos
}

func sameDeniedFile(info os.FileInfo, bases []os.FileInfo) bool {
	for _, base := range bases {
		if os.SameFile(info, base) {
			return true
		}
	}
	return false
}

// Inspect the identity of the actual opened handle too: a pathname can change
// between authorization and open, or gain a credential hardlink in that window.
func deniedOpenedFile(info os.FileInfo) bool {
	return sameDeniedFile(info, deniedBaseIdentities(homeDirs()))
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
	bases := deniedBaseIdentities(homes)
	if len(bases) == 0 {
		return false
	}
	for candidate := filepath.Clean(path); ; candidate = filepath.Dir(candidate) {
		if info, err := os.Stat(candidate); err == nil && sameDeniedFile(info, bases) {
			return true
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

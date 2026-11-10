package wsgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// lookPath resolves the git executable (a test seam).
var lookPath = exec.LookPath

// resolved is the git executable Executable resolved and the search path it
// was resolved under; path is empty until a resolution succeeds.
var resolved struct {
	sync.Mutex
	path, under string
}

// Executable is the git executable's path, resolved once per search path: a
// bare "git" makes every command search each PATH entry (and, on Windows,
// each PATHEXT extension), and a dashboard reload spawns several. A changed
// PATH or PATHEXT resolves again (a process-wide cache pinned the first git
// in fake-git PATH tests). A failed resolution is not kept: it returns
// "git", so the command resolves it itself as before and a later PATH fix
// still takes.
func Executable() string {
	under := os.Getenv("PATH") + "\x00" + os.Getenv("PATHEXT")
	resolved.Lock()
	defer resolved.Unlock()
	if resolved.path == "" || resolved.under != under {
		path, err := lookPath("git")
		if err != nil {
			resolved.path = ""
			return "git"
		}
		resolved.path, resolved.under = path, under
	}
	return resolved.path
}

// LookPathEnv resolves name against the PATH (and, on Windows, PATHEXT) of
// environ rather than this process's: exec.Command resolves a bare name
// against the parent's PATH, so a subprocess run for another process (a
// server the dashboard daemon hosts for a launcher) would otherwise start
// the daemon's binary even with cmd.Env set to the caller's environment.
// A name with a path separator, or one not found, is returned unchanged.
func LookPathEnv(name string, environ []string) string {
	if strings.ContainsAny(name, `/\`) {
		return name
	}
	var path, pathext string
	for _, kv := range environ {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		switch key {
		case "PATH":
			path = value
		case "PATHEXT":
			pathext = value
		}
	}
	exts := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		if pathext == "" {
			pathext = ".com;.exe;.bat;.cmd"
		}
		exts = nil
		for _, ext := range filepath.SplitList(pathext) {
			if ext != "" {
				exts = append(exts, strings.ToLower(ext))
			}
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			candidate := filepath.Join(dir, name+ext)
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
				continue
			}
			return candidate
		}
	}
	return name
}

// Package wsexec runs a subprocess for another process: a server the
// dashboard daemon hosts for a launcher resolves programs against, and runs
// them under, that launcher's environment rather than the daemon's. It
// imports nothing of ws, so every package that execs may use it.
package wsexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LookPath resolves name against the PATH (and, on Windows, PATHEXT) of
// environ rather than this process's: exec.Command resolves a bare name
// against the parent's PATH, so a subprocess run for another process (a
// server the dashboard daemon hosts for a launcher) would otherwise start
// the daemon's binary even with cmd.Env set to the caller's environment.
// A name with a path separator, or one not found, is returned unchanged.
func LookPath(name string, environ []string) string {
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

// Command is exec.Command for env: with a nil env it is exec.Command itself
// (the process's own PATH and environment); otherwise name resolves against
// env's PATH and the command runs with env as its environment.
func Command(env []string, name string, args ...string) *exec.Cmd {
	if env == nil {
		return exec.Command(name, args...)
	}
	cmd := exec.Command(LookPath(name, env), args...)
	cmd.Env = env
	return cmd
}

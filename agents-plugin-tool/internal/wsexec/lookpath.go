// Package wsexec runs a subprocess for another process: a server the
// dashboard daemon hosts for a launcher resolves programs against, and runs
// them under, that launcher's environment rather than the daemon's. It
// imports nothing of ws, so every package that execs may use it.
package wsexec

import (
	"context"
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
// A name with a path separator is returned unchanged. With a nil environ
// the lookup is this process's own (exec.LookPath). A name that environ's
// PATH does not hold is an error, never a fall back to this process's PATH:
// the daemon's git standing in for a caller whose environment has none is
// exactly what the caller environment exists to prevent.
func LookPath(name string, environ []string) (string, error) {
	if strings.ContainsAny(name, `/\`) {
		return name, nil
	}
	if environ == nil {
		return exec.LookPath(name)
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
			return candidate, nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

// Command is exec.Command for env: with a nil env it is exec.Command itself
// (the process's own PATH and environment); otherwise name resolves against
// env's PATH and the command runs with env as its environment. A name env's
// PATH does not hold leaves the command unstartable (cmd.Err, as
// exec.Command does for its own lookup failure), so Start and Run report
// the missing program rather than run this process's.
func Command(env []string, name string, args ...string) *exec.Cmd {
	return CommandContext(context.Background(), env, name, args...)
}

// CommandContext is Command bounded by ctx, as exec.CommandContext is.
func CommandContext(ctx context.Context, env []string, name string, args ...string) *exec.Cmd {
	if env == nil {
		return exec.CommandContext(ctx, name, args...)
	}
	path, err := LookPath(name, env)
	if err != nil {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Err = err
		cmd.Env = env
		return cmd
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	return cmd
}

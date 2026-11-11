package wsdashboard

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// terminalShell is the shell a new terminal runs: its argv, and the whole
// environment the holder gives it.
type terminalShell struct {
	Argv []string
	Env  map[string]string
}

// terminalTerm is the TERM a shell gets when the daemon's own names no
// capable terminal.
const terminalTerm = "xterm-256color"

// daemonShell picks the shell for the machine the daemon runs on, from the
// daemon's own environment.
func daemonShell() terminalShell {
	return pickShell(runtime.GOOS, os.Environ(), func(name string) (string, bool) {
		path, err := exec.LookPath(name)
		return path, err == nil
	})
}

// pickShell picks a terminal's shell on goos from the environment environ
// (os.Environ's form); look resolves a program that can be run to its path
// (exec.LookPath).
//
// On Windows the shell is the first of pwsh.exe and powershell.exe that can
// be run, by its resolved path, then %COMSPEC%, then cmd.exe. Elsewhere it
// is $SHELL when that can be run, then /bin/sh. The shell inherits the
// environment, with one change: a TERM that is missing, empty, or "dumb" (a
// daemon started by an agent host or a service has no terminal of its own)
// becomes xterm-256color, which is what the page's emulator speaks; a
// capable TERM is kept.
func pickShell(goos string, environ []string, look func(name string) (string, bool)) terminalShell {
	env := map[string]string{}
	for _, kv := range environ {
		// Windows lists per-drive working directories as "=C:=C:\dir".
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			env[k] = v
		}
	}
	lookup := func(name string) string {
		if v, ok := env[name]; ok || goos != "windows" {
			return v
		}
		// A Windows environment's names are case-insensitive.
		for k, v := range env {
			if strings.EqualFold(k, name) {
				return v
			}
		}
		return ""
	}
	if term := strings.TrimSpace(lookup("TERM")); term == "" || term == "dumb" {
		if goos == "windows" {
			for k := range env {
				if strings.EqualFold(k, "TERM") {
					delete(env, k)
				}
			}
		}
		env["TERM"] = terminalTerm
	}
	var shell string
	if goos == "windows" {
		for _, name := range []string{"pwsh.exe", "powershell.exe"} {
			if path, ok := look(name); ok {
				shell = path
				break
			}
		}
		if shell == "" {
			shell = strings.TrimSpace(lookup("COMSPEC"))
		}
		if shell == "" {
			shell = "cmd.exe"
		}
	} else {
		shell = strings.TrimSpace(env["SHELL"])
		if _, ok := look(shell); shell == "" || !ok {
			shell = "/bin/sh"
		}
	}
	return terminalShell{Argv: []string{shell}, Env: env}
}

// Package gitbatch is the non-interactive bound every background git caller
// shares: the context kill that reaches a transport child, the credential
// config that keeps a remote command from prompting, and the environment
// that turns off terminal and askpass prompts and puts ssh in batch mode.
// Each caller keeps its own runner; this package owns only the flag set, so
// the callers cannot drift apart on it.
package gitbatch

import (
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// waitDelay bounds the wait for a child that still holds git's pipes after
// git itself was killed (an ssh subprocess can).
const waitDelay = 300 * time.Millisecond

// Bound makes cmd's context end a hard stop: the context kills git's whole
// process group, and WaitDelay bounds the wait for a lingering child, so a
// hung transport cannot outlast the caller's timeout the way a plain
// exec.CommandContext lets it.
func Bound(cmd *exec.Cmd) {
	cmd.WaitDelay = waitDelay
	configureKill(cmd)
}

const credentialFlag = "credential.interactive=false"

// RemoteArgs prefixes a remote git subcommand with config that disables
// credential prompting on git versions that understand it (older versions
// ignore the unknown key). Args whose leading -c options already carry it
// are returned unchanged.
func RemoteArgs(args ...string) []string {
	for i := 0; i+1 < len(args) && args[i] == "-c"; i += 2 {
		if args[i+1] == credentialFlag {
			return args
		}
	}
	return append([]string{"-c", credentialFlag}, args...)
}

// RemoteEnv is the environment a remote command runs under, appended to the
// caller's: no terminal prompt, no askpass helper, no interactive credential
// manager, and sshCommand (see SSHCommand) as git's ssh command.
func RemoteEnv(sshCommand string) []string {
	return []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GCM_INTERACTIVE=never",
		"GIT_SSH_COMMAND=" + sshCommand,
	}
}

// SSHCommand keeps git's own precedence for the ssh command
// (GIT_SSH_COMMAND, then core.sshCommand, then GIT_SSH, then ssh) and
// appends the non-interactive options for its variant: -batch for plink,
// BatchMode and a connect timeout otherwise. coreSSHCommand returns the
// repository's core.sshCommand ("" when unset); it runs only when the
// environment does not decide.
func SSHCommand(getenv func(string) string, coreSSHCommand func() string) string {
	base := strings.TrimSpace(getenv("GIT_SSH_COMMAND"))
	if base == "" {
		base = strings.TrimSpace(coreSSHCommand())
	}
	if base == "" {
		if program := strings.TrimSpace(getenv("GIT_SSH")); program != "" {
			base = ShellQuote(program)
		}
	}
	if base == "" {
		base = "ssh"
	}
	if isPlink(base) {
		return base + " -batch"
	}
	return base + " -o BatchMode=yes -o ConnectTimeout=5"
}

func isPlink(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	name := strings.ToLower(filepath.Base(strings.Trim(fields[0], `'"`)))
	return strings.Contains(name, "plink")
}

// ShellQuote single-quotes s for sh.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

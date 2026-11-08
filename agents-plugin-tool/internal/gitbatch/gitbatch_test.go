package gitbatch

import (
	"slices"
	"testing"
)

// SSHCommand follows git's precedence (GIT_SSH_COMMAND, core.sshCommand,
// GIT_SSH, ssh) and appends the batch options for the chosen variant.
func TestSSHCommandPrecedenceAndVariant(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		core string
		want string
	}{
		{"default ssh", nil, "", "ssh -o BatchMode=yes -o ConnectTimeout=5"},
		{"GIT_SSH_COMMAND wins over all", map[string]string{"GIT_SSH_COMMAND": "myssh -p 22", "GIT_SSH": "/bin/other"}, "ssh -i /k/id", "myssh -p 22 -o BatchMode=yes -o ConnectTimeout=5"},
		{"core.sshCommand over GIT_SSH", map[string]string{"GIT_SSH": "/bin/other"}, "ssh -i /k/id\n", "ssh -i /k/id -o BatchMode=yes -o ConnectTimeout=5"},
		{"GIT_SSH quoted", map[string]string{"GIT_SSH": "/opt/my ssh/it's"}, "", `'/opt/my ssh/it'\''s' -o BatchMode=yes -o ConnectTimeout=5`},
		{"blank values fall through", map[string]string{"GIT_SSH_COMMAND": "  ", "GIT_SSH": " "}, " ", "ssh -o BatchMode=yes -o ConnectTimeout=5"},
		{"plink through GIT_SSH", map[string]string{"GIT_SSH": `C:\tools\plink.exe`}, "", `'C:\tools\plink.exe' -batch`},
		{"plink through GIT_SSH_COMMAND", map[string]string{"GIT_SSH_COMMAND": `"C:/PuTTY/PLINK.EXE" -P 22`}, "", `"C:/PuTTY/PLINK.EXE" -P 22 -batch`},
		{"plink through core.sshCommand", nil, "/usr/bin/plink -v", "/usr/bin/plink -v -batch"},
		{"plink only as an argument is not plink", map[string]string{"GIT_SSH_COMMAND": "ssh -F plink.conf"}, "", "ssh -F plink.conf -o BatchMode=yes -o ConnectTimeout=5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SSHCommand(func(k string) string { return tc.env[k] }, func() string { return tc.core })
			if got != tc.want {
				t.Fatalf("SSHCommand = %q, want %q", got, tc.want)
			}
		})
	}
}

// core.sshCommand costs a git process; it is read only when the
// environment does not decide.
func TestSSHCommandReadsCoreOnlyWhenNeeded(t *testing.T) {
	called := false
	SSHCommand(func(k string) string {
		if k == "GIT_SSH_COMMAND" {
			return "ssh"
		}
		return ""
	}, func() string { called = true; return "" })
	if called {
		t.Fatalf("core.sshCommand must not be read when GIT_SSH_COMMAND is set")
	}
}

func TestRemoteArgsAddsTheCredentialFlagOnce(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"fetch", "origin"}, []string{"-c", "credential.interactive=false", "fetch", "origin"}},
		{[]string{"-c", "credential.interactive=false", "push", "origin"}, []string{"-c", "credential.interactive=false", "push", "origin"}},
		{[]string{"-c", "core.x=1", "-c", "credential.interactive=false", "fetch"}, []string{"-c", "core.x=1", "-c", "credential.interactive=false", "fetch"}},
		{[]string{"-c", "core.x=1", "fetch"}, []string{"-c", "credential.interactive=false", "-c", "core.x=1", "fetch"}},
		{[]string{"fetch", "-c", "credential.interactive=false"}, []string{"-c", "credential.interactive=false", "fetch", "-c", "credential.interactive=false"}},
	}
	for _, tc := range cases {
		if got := RemoteArgs(tc.in...); !slices.Equal(got, tc.want) {
			t.Fatalf("RemoteArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRemoteEnvTurnsOffEveryPrompt(t *testing.T) {
	want := []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GCM_INTERACTIVE=never", "GIT_SSH_COMMAND=ssh -o BatchMode=yes"}
	if got := RemoteEnv("ssh -o BatchMode=yes"); !slices.Equal(got, want) {
		t.Fatalf("RemoteEnv = %q, want %q", got, want)
	}
}

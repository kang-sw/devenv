package wsdashboard

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchdSpawn starts exe as a launchd job with ProcessType Interactive.
// A process started from inside a utility-clamped tree (a Claude Code Bash
// tool inside a holder-hosted session) inherits that clamp, and so does
// every holder and agent session it later starts; a launchd job does not
// inherit it, and ProcessType Interactive starts it at the DEFAULT class.
// The job's wrapper boots the job out when exe exits, so no label outlives
// its process. An error means nothing was started and the caller falls back
// to a plain spawn.
func launchdSpawn(exe string, args []string, dir string) error {
	launchctl, err := exec.LookPath("launchctl")
	if err != nil {
		return err
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	label := fmt.Sprintf("dev.ws.dashboard.%d.%s", os.Getpid(), hex.EncodeToString(b[:]))
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	plist := launchdPlist(label, target, exe, args, dir, os.Environ())

	tmp, err := os.MkdirTemp("", "ws-launchd-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	path := filepath.Join(tmp, label+".plist")
	if err := os.WriteFile(path, plist, 0o600); err != nil {
		return err
	}
	out, err := exec.Command(launchctl, "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// launchdPlist renders the job. The wrapper shell runs exe as a child, then
// boots the job out so the label is removed once exe exits.
func launchdPlist(label, target, exe string, args []string, dir string, env []string) []byte {
	var w bytes.Buffer
	str := func(s string) {
		w.WriteString("<string>")
		_ = xml.EscapeText(&w, []byte(s))
		w.WriteString("</string>")
	}
	w.WriteString(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>`)
	w.WriteString("<key>Label</key>")
	str(label)
	w.WriteString("<key>ProgramArguments</key><array>")
	for _, a := range append([]string{"/bin/sh", "-c", `t=$1; shift; "$@"; /bin/launchctl bootout "$t"`, "sh", target, exe}, args...) {
		str(a)
	}
	w.WriteString("</array><key>EnvironmentVariables</key><dict>")
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		w.WriteString("<key>")
		_ = xml.EscapeText(&w, []byte(k))
		w.WriteString("</key>")
		str(v)
	}
	w.WriteString("</dict><key>WorkingDirectory</key>")
	str(dir)
	w.WriteString("<key>ProcessType</key><string>Interactive</string>")
	w.WriteString("<key>AbandonProcessGroup</key><true/><key>RunAtLoad</key><true/>")
	w.WriteString("<key>StandardOutPath</key><string>/dev/null</string><key>StandardErrorPath</key><string>/dev/null</string>")
	w.WriteString("</dict></plist>")
	return w.Bytes()
}

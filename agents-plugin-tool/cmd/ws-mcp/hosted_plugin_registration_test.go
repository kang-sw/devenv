package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func acceptHostedPackageHello(t *testing.T, host *proxyHost, mode, root, helloVersion string) {
	t.Helper()
	session, err := host.Accept(wsdashboard.ProxyHello{
		Mode: mode, Version: helloVersion, Cwd: root, OwnerPID: os.Getpid(),
		Env: map[string]string{
			wsrsrc.EnvRsrcRoot: filepath.Join(root, "rsrc"),
			"WS_CACHE_HOME":    t.TempDir(), "WS_CONFIG_HOME": t.TempDir(),
			"WS_MCP_NO_DASHBOARD": "1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mcpSession, ok := session.(*proxyMCPSession); ok {
		mcpSession.server.Close()
	}
}

func TestHostedPackageRegistrationMissingManifestIsQuiet(t *testing.T) {
	for _, mode := range []string{wsdashboard.ProxyModeCLI, wsdashboard.ProxyModeMCP} {
		for _, kind := range []string{"missing", "pi", "malformed", "unsupported"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				switch kind {
				case "pi":
					if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"ws-pi-bridge","version":"0.50.1"}`), 0o600); err != nil {
						t.Fatal(err)
					}
				case "malformed", "unsupported":
					if err := os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o700); err != nil {
						t.Fatal(err)
					}
					body := `{"name":`
					if kind == "unsupported" {
						body = `{"name":"unrelated","version":"0.50.1"}`
					}
					if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), []byte(body), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				var log bytes.Buffer
				host := &proxyHost{dir: t.TempDir(), logw: &log}
				acceptHostedPackageHello(t, host, mode, root, "0.50.1")
				logged := strings.Contains(log.String(), "hosted plugin root not registered:")
				wantLog := kind == "malformed" || kind == "unsupported"
				if logged != wantLog {
					t.Fatalf("registration logged = %v, want %v; log: %s", logged, wantLog, &log)
				}
			})
		}
	}
}

func TestHostedPackageAcceptOlderHelloCannotDowngrade(t *testing.T) {
	for _, mode := range []string{wsdashboard.ProxyModeCLI, wsdashboard.ProxyModeMCP} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WS_CONFIG_HOME", t.TempDir())
			t.Setenv("WS_CACHE_HOME", t.TempDir())
			dir, err := wsdashboard.Dir("")
			if err != nil {
				t.Fatal(err)
			}
			newer := hostedPackageFixture(t, "wsflow")
			older := hostedPackageFixture(t, "wsflow")
			manifest := filepath.Join(older, ".claude-plugin", "plugin.json")
			raw, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifest, bytes.ReplaceAll(raw, []byte("0.50.1"), []byte("0.50.0")), 0o600); err != nil {
				t.Fatal(err)
			}
			starter := hostedPackageFixture(t, "ws")
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(starter, "rsrc"))
			var log bytes.Buffer
			host := &proxyHost{dir: dir, logw: &log}
			acceptHostedPackageHello(t, host, mode, newer, "0.50.1")
			acceptHostedPackageHello(t, host, mode, older, "0.50.0")
			root, warning, err := wsagent.ResolveHostedPlugin("0.50.1")
			if err != nil || root != newer || warning != "" {
				t.Fatalf("older hello replaced newer root: %q %q %v; log: %s", root, warning, err, &log)
			}
		})
	}
}

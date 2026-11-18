package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestHostedPackageHellosNeverReadOrWriteLegacyRegistry(t *testing.T) {
	for _, mode := range []string{wsdashboard.ProxyModeCLI, wsdashboard.ProxyModeMCP} {
		for _, legacy := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/absent", true: "/stale"}[legacy], func(t *testing.T) {
				t.Setenv("WS_CONFIG_HOME", t.TempDir())
				t.Setenv("WS_CACHE_HOME", t.TempDir())
				dir, err := wsdashboard.Dir("")
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "hosted-plugin-roots.json")
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if legacy {
					if err := os.WriteFile(path, []byte("{broken registry"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				root := t.TempDir()
				var log bytes.Buffer
				host := &proxyHost{dir: dir, logw: &log}
				session, err := host.Accept(wsdashboard.ProxyHello{
					Mode: mode, Version: "0.50.1", Cwd: root, OwnerPID: os.Getpid(),
					Env: map[string]string{wsrsrc.EnvRsrcRoot: filepath.Join(root, "missing", "rsrc"), "WS_CACHE_HOME": os.Getenv("WS_CACHE_HOME"), "WS_CONFIG_HOME": os.Getenv("WS_CONFIG_HOME"), "WS_MCP_NO_DASHBOARD": "1"},
				})
				if err != nil {
					t.Fatal(err)
				}
				if mcp, ok := session.(*proxyMCPSession); ok {
					mcp.server.Close()
				}
				name, warning, err := wsagent.ResolveHostedPackage()
				if err != nil || name != "wsflow" || warning != "" {
					t.Fatalf("%q %q %v", name, warning, err)
				}
				data, err := os.ReadFile(path)
				if legacy {
					if err != nil || string(data) != "{broken registry" {
						t.Fatal("legacy registry changed")
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf("hello wrote registry: %v", err)
				}
				if bytes.Contains(log.Bytes(), []byte("hosted plugin root")) {
					t.Fatalf("registry diagnostic: %s", &log)
				}
			})
		}
	}
}

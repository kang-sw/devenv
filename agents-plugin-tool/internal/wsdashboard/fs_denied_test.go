package wsdashboard

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func assertSensitiveReadsRefused(t *testing.T, o *outsideFixture, path string) {
	t.Helper()
	for _, route := range []string{"file", "fs/preview", "fs/raw", "image"} {
		target := "/s/livesession/" + route + "?path=" + url.QueryEscape(path)
		if r := o.read(target); r.Code != http.StatusNotFound {
			t.Errorf("sensitive %s = %d %s", target, r.Code, r.Body.String())
		}
	}
	if code, a := o.open(t, o.local, path); code != http.StatusNotFound {
		t.Errorf("sensitive mint %q = %d %+v", path, code, a)
	}
}

func assertSensitiveDirRefused(t *testing.T, o *outsideFixture, path string) {
	t.Helper()
	for _, query := range []string{"dir=" + url.QueryEscape(path), "root=" + url.QueryEscape(path)} {
		if r := o.read("/s/livesession/fs/list?" + query); r.Code != http.StatusNotFound {
			t.Errorf("sensitive listing %s = %d %s", query, r.Code, r.Body.String())
		}
	}
}

func TestFSLocalDeniedCredentialFileIdentities(t *testing.T) {
	o := startOutsideFixture(t)
	for i, name := range []string{
		".netrc", ".git-credentials", ".codex/auth.json", ".claude/.credentials.json",
		".npmrc", ".pypirc", ".cargo/credentials", ".cargo/credentials.toml", ".vault-token",
	} {
		t.Run(name, func(t *testing.T) {
			secret := o.write(t, o.home, name, "DENIED-CREDENTIAL")
			assertSensitiveReadsRefused(t, o, secret)
			// A hardlink has no resolved pathname pointing back into HOME.
			// Only the file identity, not string or symlink checks, denies it.
			alias := filepath.Join(o.away, string(rune('a'+i))+".png")
			if err := os.Link(secret, alias); err != nil {
				t.Skipf("hardlinks unavailable: %v", err)
			}
			assertSensitiveReadsRefused(t, o, alias)
		})
	}
}

func TestFSLocalDeniedIdentitiesFollowReplacement(t *testing.T) {
	o := startOutsideFixture(t)
	plain := o.write(t, o.away, "plain.png", "PLAIN")
	if alwaysDenied(plain, homeDirs()) {
		t.Fatal("ordinary path refused while credential bases are absent")
	}
	base := o.write(t, o.home, ".codex/auth.json", "FIRST")
	first := filepath.Join(o.away, "first.png")
	if err := os.Link(base, first); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	assertSensitiveReadsRefused(t, o, first)
	if err := os.Rename(base, base+".old"); err != nil {
		t.Fatal(err)
	}
	o.write(t, o.home, ".codex/auth.json", "REPLACED")
	replaced := filepath.Join(o.away, "replaced.png")
	if err := os.Link(base, replaced); err != nil {
		t.Fatal(err)
	}
	assertSensitiveReadsRefused(t, o, replaced)
	if alwaysDenied(first, homeDirs()) {
		t.Error("identity checks still use the replaced base")
	}
	// A newly created base must be denied immediately, without a restart.
	added := o.write(t, o.home, ".claude/.credentials.json", "ADDED")
	alias := filepath.Join(o.away, "added.png")
	if err := os.Link(added, alias); err != nil {
		t.Fatal(err)
	}
	assertSensitiveReadsRefused(t, o, alias)
}

func TestFSLocalDeniedDaemonState(t *testing.T) {
	for _, relocated := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "relocated"}[relocated], func(t *testing.T) {
			o := startOutsideFixture(t)
			t.Setenv("WS_CACHE_HOME", "")
			if relocated {
				t.Setenv("WS_CACHE_HOME", filepath.Join(o.away, "cache"))
			}
			dir, err := Dir("")
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{localhostSecretFile, LANTokenFileName, endpointFileName} {
				secret := o.write(t, dir, name, "DENIED-DAEMON-CREDENTIAL")
				assertSensitiveReadsRefused(t, o, secret)
				alias := filepath.Join(o.away, name+".png")
				if err := os.Link(secret, alias); err != nil {
					t.Skipf("hardlinks unavailable: %v", err)
				}
				assertSensitiveReadsRefused(t, o, alias)
				info, err := os.Stat(alias)
				if err != nil || !deniedOpenedFile(info) {
					t.Errorf("opened daemon credential alias not denied: %v", err)
				}
			}
			assertSensitiveDirRefused(t, o, dir)
			aliasDir := filepath.Join(o.away, "state-alias")
			if err := os.Symlink(dir, aliasDir); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			assertSensitiveDirRefused(t, o, aliasDir)
			assertSensitiveReadsRefused(t, o, filepath.Join(aliasDir, localhostSecretFile))
			assertOrdinaryLocalReads(t, o, o.write(t, o.away, "ordinary.txt", "ORDINARY"))
		})
	}
}

func TestFSLocalDeniedCredentialDirectories(t *testing.T) {
	o := startOutsideFixture(t)
	for _, name := range []string{".kube", ".config/gcloud", ".azure", ".password-store", "Library/Keychains"} {
		t.Run(name, func(t *testing.T) {
			secret := o.write(t, o.home, name+"/config.png", "DENIED-CREDENTIAL")
			assertSensitiveReadsRefused(t, o, secret)
			assertSensitiveDirRefused(t, o, filepath.Dir(secret))
		})
	}
}

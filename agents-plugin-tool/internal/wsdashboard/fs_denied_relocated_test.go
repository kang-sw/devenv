package wsdashboard

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func assertOrdinaryLocalReads(t *testing.T, o *outsideFixture, path string) {
	t.Helper()
	routes := []string{"file", "fs/preview"}
	if _, image := imageType(path); image {
		routes = append(routes, "fs/raw", "image")
	}
	for _, route := range routes {
		if r := o.read("/s/livesession/" + route + "?path=" + url.QueryEscape(path)); r.Code != 200 {
			t.Errorf("ordinary %s %q = %d %s", route, path, r.Code, r.Body.String())
		}
	}
	if code, _ := o.open(t, o.local, path); code != 200 {
		t.Errorf("ordinary mint %q = %d", path, code)
	}
	if r := o.read("/s/livesession/fs/list?root=" + url.QueryEscape(filepath.Dir(path))); r.Code != 200 {
		t.Errorf("ordinary listing = %d %s", r.Code, r.Body.String())
	}
}

func TestFSLocalBrokenCredentialBaseDoesNotDenyOtherReads(t *testing.T) {
	for _, c := range []struct{ parent, leaf string }{
		{".codex", "auth.json"}, {".claude", ".credentials.json"}, {".config", "gh"},
	} {
		t.Run(c.parent, func(t *testing.T) {
			o := startOutsideFixture(t)
			o.write(t, o.home, c.parent, "THIS-IS-A-FILE-NOT-A-DIRECTORY")
			ordinary := o.write(t, o.away, "ordinary.png", "ORDINARY")
			assertOrdinaryLocalReads(t, o, ordinary)
			assertSensitiveReadsRefused(t, o, filepath.Join(o.home, c.parent, c.leaf))
		})
	}
}

func TestFSLocalUnreadableCredentialBaseDoesNotDenyOtherReads(t *testing.T) {
	o := startOutsideFixture(t)
	secret := o.write(t, o.home, ".claude/.credentials.json", "DENIED")
	parent := filepath.Dir(secret)
	if err := os.Chmod(parent, 0); err != nil {
		t.Skip(err)
	}
	defer os.Chmod(parent, 0o700)
	if _, err := os.Stat(secret); err == nil {
		t.Skip("host still permits reading the restricted directory")
	}
	assertOrdinaryLocalReads(t, o, o.write(t, o.away, "ordinary.txt", "ORDINARY"))
	assertSensitiveReadsRefused(t, o, secret)
}

func TestFSLocalPostOpenRejectsNewCredentialHardlink(t *testing.T) {
	o := startOutsideFixture(t)
	alias := o.write(t, o.project, "alias.png", "CREDENTIAL")
	base := filepath.Join(o.home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
		t.Fatal(err)
	}
	if alwaysDenied(alias, homeDirs()) {
		t.Fatal("alias denied before it becomes a credential")
	}
	called := false
	f, _, _, ok := openContainedChecked(o.project, "alias.png", func(full, real string) bool {
		called = true
		if alwaysDenied(full, homeDirs()) || alwaysDenied(real, homeDirs()) {
			t.Fatal("pre-open check unexpectedly denied alias")
		}
		// Introduce the credential identity after path authorization but
		// before Open; Lstat and f.Stat still see the same ordinary inode.
		if err := os.Link(alias, base); err != nil {
			t.Skipf("hardlinks unavailable: %v", err)
		}
		return true
	})
	if f != nil {
		f.Close()
	}
	if !called || ok {
		t.Errorf("post-open credential identity check: callback=%v, opened=%v", called, ok)
	}
	assertSensitiveReadsRefused(t, o, alias)
}

func TestFSLocalDeniedRelocatedCredentials(t *testing.T) {
	for _, c := range []struct {
		key, child string
		directory  bool
	}{
		{"GH_CONFIG_DIR", "", true},
		{"XDG_CONFIG_HOME", "gh", true}, {"XDG_CONFIG_HOME", "gcloud", true},
		{"CODEX_HOME", "auth.json", false}, {"CLAUDE_CONFIG_DIR", ".credentials.json", false},
		{"AWS_SHARED_CREDENTIALS_FILE", "", false}, {"AWS_CONFIG_FILE", "", false},
	} {
		t.Run(c.key+"/"+c.child, func(t *testing.T) {
			o := startOutsideFixture(t)
			location := filepath.Join(o.away, "relocated")
			base := filepath.Join(location, c.child)
			secret := base
			if c.directory {
				secret = filepath.Join(base, "secret.png")
			}
			o.write(t, filepath.Dir(secret), filepath.Base(secret), "DENIED-RELOCATED")
			t.Setenv(c.key, location)
			assertSensitiveReadsRefused(t, o, secret)
			if c.directory {
				assertSensitiveDirRefused(t, o, base)
			}
			alias := filepath.Join(o.project, "credential-alias.png")
			if !c.directory {
				if err := os.Link(secret, alias); err != nil {
					t.Skipf("hardlinks unavailable: %v", err)
				}
			} else {
				if err := os.Symlink(base, filepath.Join(o.project, "credential-dir")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				alias = filepath.Join(o.project, "credential-dir", "secret.png")
			}
			assertSensitiveReadsRefused(t, o, alias)
			if c.child != "" && !c.directory {
				assertOrdinaryLocalReads(t, o, o.write(t, location, "history.txt", "ORDINARY-HISTORY"))
			}
		})
	}
}

func TestFSLocalDeniedKubeConfigList(t *testing.T) {
	o := startOutsideFixture(t)
	first := o.write(t, o.away, "kube/first", "DENIED-KUBE-ONE")
	second := o.write(t, o.away, "kube/second", "DENIED-KUBE-TWO")
	sep := string(os.PathListSeparator)
	t.Setenv("KUBECONFIG", first+sep+sep+second)
	for _, path := range []string{first, second} {
		assertSensitiveReadsRefused(t, o, path)
	}
	assertOrdinaryLocalReads(t, o, o.write(t, o.away, "ordinary.txt", "ORDINARY"))
}

func TestFSLocalDeniedHomeRelativeEnvironment(t *testing.T) {
	o := startOutsideFixture(t)
	secret := o.write(t, o.home, "relocated-gh/secret.png", "DENIED")
	t.Setenv("GH_CONFIG_DIR", "~/relocated-gh")
	assertSensitiveReadsRefused(t, o, secret)
	assertSensitiveDirRefused(t, o, filepath.Dir(secret))
}

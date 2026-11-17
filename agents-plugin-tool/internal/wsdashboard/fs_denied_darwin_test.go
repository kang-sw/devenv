package wsdashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSLocalDeniedDarwinFilesystemAliases(t *testing.T) {
	o := startOutsideFixture(t)
	ssh := o.write(t, o.home, ".ssh/secret.png", "DENIED-SSH")
	docker := o.write(t, o.home, ".docker/secret.png", "DENIED-DOCKER")
	dataHome := "/System/Volumes/Data" + o.home
	if strings.HasPrefix(o.home, "/System/Volumes/Data/") {
		dataHome = strings.TrimPrefix(o.home, "/System/Volumes/Data")
	}
	for _, c := range []struct{ name, alias, secret string }{
		{"ASCII case", filepath.Join(o.home, ".SSH", "secret.png"), ssh},
		{"long s case fold", filepath.Join(o.home, ".ſsh", "secret.png"), ssh},
		{"Kelvin sign case fold", filepath.Join(o.home, ".docKer", "secret.png"), docker},
		{"Data volume firmlink", filepath.Join(dataHome, ".ssh", "secret.png"), ssh},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Only test real aliases supported by this test volume; no fixture
			// ever reads or creates anything under the daemon user's real HOME.
			actual, err := os.Stat(c.secret)
			if err != nil {
				t.Fatal(err)
			}
			aliased, err := os.Stat(c.alias)
			if err != nil || !os.SameFile(actual, aliased) {
				t.Skipf("filesystem does not expose this alias: %v", err)
			}
			assertSensitiveReadsRefused(t, o, c.alias)
			assertSensitiveDirRefused(t, o, filepath.Dir(c.alias))
		})
	}
}

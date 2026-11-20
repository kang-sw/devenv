package wsharnessenv

import (
	"slices"
	"testing"
)

func TestDropForOSMatchesWindowsEnvironmentNamesCaseInsensitively(t *testing.T) {
	env := []string{"PATH=C:\\bin", "Ws_MaIlBoX=parent@machine", "ws_hosted_session=1", "USER_SETTING=kept"}
	if got := DropForOS("windows", env); !slices.Equal(got, []string{"PATH=C:\\bin", "USER_SETTING=kept"}) {
		t.Fatalf("Windows drop = %v", got)
	}
	if got := DropForOS("linux", env); !slices.Equal(got, env) {
		t.Fatalf("Unix environment names should remain case-sensitive: %v", got)
	}
}

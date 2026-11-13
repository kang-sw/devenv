package wsconfig

import (
	"reflect"
	"testing"
)

func TestListScalarsLayersClearAndReset(t *testing.T) {
	sess := newFakeSessionStore()
	r, _ := newRepoTestResolver(t, sess)
	for _, row := range []struct {
		key, value string
		scope      Scope
	}{
		{"preference.shared", "global", ScopeGlobal},
		{"preference.shared", "repo", ScopeRepo},
		{"preference.shared", "project", ScopeProject},
		{"preference.shared", "", ScopeSession},
		{"preference.global", "G", ScopeGlobal},
		{"preference.repo", "R", ScopeRepo},
		{"preference.project", "P", ScopeProject},
		{"preference.session", "S", ScopeSession},
		{"other.key", "ignored", ScopeSession},
	} {
		if err := r.Set(row.key, row.value, SetOptions{ExplicitScope: row.scope, SessionKey: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	want := []ScalarValue{
		{"preference.global", "G", ScopeGlobal},
		{"preference.project", "P", ScopeProject},
		{"preference.repo", "R", ScopeRepo},
		{"preference.session", "S", ScopeSession},
		{"preference.shared", "", ScopeSession},
	}
	got, err := r.ListScalars("s", PreferencePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, row := range []struct {
		remove, winner Scope
		value          string
	}{
		{ScopeSession, ScopeProject, "project"},
		{ScopeProject, ScopeRepo, "repo"},
		{ScopeRepo, ScopeGlobal, "global"},
	} {
		if err := r.Unset("preference.shared", SetOptions{ExplicitScope: row.remove, SessionKey: "s"}); err != nil {
			t.Fatal(err)
		}
		got, err = r.ListScalars("s", PreferencePrefix)
		if err != nil {
			t.Fatal(err)
		}
		last := got[len(got)-1]
		if last.Scope != row.winner || last.Value != row.value {
			t.Fatalf("after removing %s: %+v", row.remove, last)
		}
	}
	keyless, err := r.ListScalars("", PreferencePrefix)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range keyless {
		if value.Scope == ScopeSession {
			t.Fatal("keyless list included session")
		}
	}
}

func TestPreferenceKeyValidationAndDefaultScope(t *testing.T) {
	for _, slot := range []string{"default", "a", "0", "my-style_2"} {
		key := PreferencePrefix + slot
		if !ValidPreferenceKey(key) || DefaultScope(key) != ScopeGlobal {
			t.Errorf("invalid key/default scope: %s", key)
		}
	}
	for _, key := range []string{"preference.", "preference.A", "preference._a", "preference.-a", "preference.a.b", "preference.é", "other.a"} {
		if ValidPreferenceKey(key) {
			t.Errorf("accepted %s", key)
		}
	}
}

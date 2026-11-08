package wstracker

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// The display-name and avatar rules, in the cases the reference engine's
// fixtures and README show.
func TestProfileDisplayAndAvatar(t *testing.T) {
	cases := []struct {
		name         string
		p            Profile
		display, avt string
	}{
		{"Korean", Profile{"SW개발팀", "홍", "길동"}, "홍길동", "길동"},
		{"Korean compound surname, one-letter given", Profile{"SW개발팀", "남궁", "민"}, "남궁민", "민"},
		{"Korean given longer than two", Profile{"", "김", "민수아"}, "김민수아", "민수"},
		{"Japanese kana and kanji", Profile{"", "山田", "はなこ"}, "山田はなこ", "はな"},
		{"Chinese", Profile{"", "王", "小明"}, "王小明", "小明"},
		{"Latin", Profile{"Platform", "Park", "Ada"}, "Ada Park", "AP"},
		{"Latin lower case", Profile{"", "ortiz", "maya"}, "maya ortiz", "MO"},
		{"single Latin name", Profile{"Infra", "", "Sven"}, "Sven", "SV"},
		{"single one-letter name", Profile{"", "", "X"}, "X", "X"},
		{"single Korean name", Profile{"", "", "서연"}, "서연", "서연"},
		{"Latin given with Hangul family reads Hangul style", Profile{"", "박", "Ada"}, "박Ada", "Ad"},
		{"org never joins the name", Profile{"SW개발팀", "", "Ada"}, "Ada", "AD"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.p.DisplayName(); got != c.display {
				t.Errorf("DisplayName = %q, want %q", got, c.display)
			}
			if got := c.p.Avatar(); got != c.avt {
				t.Errorf("Avatar = %q, want %q", got, c.avt)
			}
		})
	}
}

// profileOf keeps a pre-profile member readable: the bare name, else the id,
// becomes the given name.
func TestProfileOfLegacyAndPartial(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Profile
	}{
		{"profile", `{"profile":{"org":" Ops ","family":"Park","given":"Ada"}}`, Profile{"Ops", "Park", "Ada"}},
		{"bare name", `{"name":"Omar"}`, Profile{"", "", "Omar"}},
		{"neither", `{}`, Profile{"", "", "pia"}},
		{"profile without given keeps org and family", `{"profile":{"org":"Ops","family":"Park"},"name":"Ada"}`, Profile{"Ops", "Park", "Ada"}},
		{"blank given falls back", `{"profile":{"org":"Ops","given":"  "}}`, Profile{"Ops", "", "pia"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := profileOf(parsePayload(json.RawMessage(c.in)), "pia"); got != c.want {
				t.Errorf("profileOf = %+v, want %+v", got, c.want)
			}
		})
	}
}

func profileCase(t *testing.T, viewer string) *View {
	t.Helper()
	doc, src := loadGolden(t, "testdata/golden/case-member-profile__"+viewer+".json")
	now, err := time.Parse(time.RFC3339Nano, doc.Now)
	if err != nil {
		t.Fatal(err)
	}
	return Derive(src, doc.Viewer, now, doc.StaleDays)
}

func eventOf(t *testing.T, v *View, id string) EventView {
	t.Helper()
	for _, e := range v.Events {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no event %s", id)
	return EventView{}
}

// The member-profile case (testdata/cases/member-profile.json), through the
// Go fold: self edit, admin edit of another member, a non-admin edit of
// another flagged, an empty given flagged, and the concurrent per-field merge.
func TestMemberProfileFold(t *testing.T) {
	v := profileCase(t, "ada.pc")
	t.Run("self edit", func(t *testing.T) {
		if e := eventOf(t, v, "lena.pc#1"); e.Status != "applied" {
			t.Fatalf("self edit is %s", e.Status)
		}
		if got := v.Members["lena"].Profile; got != (Profile{"Search", "Weiss-Berg", "Lena"}) {
			t.Errorf("lena = %+v", got)
		}
	})
	t.Run("admin edit of another member", func(t *testing.T) {
		if e := eventOf(t, v, "ada.pc#1"); e.Status != "applied" || e.Member != "ada" {
			t.Fatalf("admin edit is %s by %s", e.Status, e.Member)
		}
		// omar was a bare name in the genesis: the given name, then the admin's fields
		if got := v.Members["omar"]; got.Profile != (Profile{"Search", "Haddad", "Omar"}) || got.Name != "Omar Haddad" || got.Avatar != "OH" {
			t.Errorf("omar = %+v", got)
		}
	})
	t.Run("non-admin edit of another member is flagged", func(t *testing.T) {
		e := eventOf(t, v, "maya.pc#1")
		if e.Status != "flagged" || deref(e.Reason) != "only an admin changes another member's profile (maya is not an admin)" {
			t.Fatalf("got %s %q", e.Status, deref(e.Reason))
		}
		if got := v.Members["lena"].Profile.Org; got != "Search" {
			t.Errorf("a flagged write changed lena's org to %q", got)
		}
	})
	t.Run("empty given is flagged", func(t *testing.T) {
		e := eventOf(t, v, "ada.pc#3")
		if e.Status != "flagged" || deref(e.Reason) != "profile.given cannot be empty" {
			t.Fatalf("got %s %q", e.Status, deref(e.Reason))
		}
		if got := v.Members["jon"]; got.Profile.Given != "길동" || got.Name != "홍길동" {
			t.Errorf("a flagged write changed jon: %+v", got)
		}
	})
	t.Run("concurrent field merge", func(t *testing.T) {
		// maya wrote org and family; ada concurrently wrote org: org is a conflict
		// (ada's later write shows, both sides marked), family only maya's, which stands.
		if got := v.Members["maya"].Profile; got != (Profile{"Platform", "Ortiz-Ruiz", "Maya"}) {
			t.Errorf("maya = %+v", got)
		}
		for _, id := range []string{"maya.pc#2", "ada.pc#2"} {
			e := eventOf(t, v, id)
			if len(e.Markers) != 1 || e.Markers[0].Field != "org of maya" || e.Markers[0].Winner != "ada.pc#2" {
				t.Errorf("%s markers = %+v", id, e.Markers)
			}
		}
	})
	t.Run("viewer carries the profile and the display name", func(t *testing.T) {
		m := profileCase(t, "maya.pc").Viewer
		if m.Name != "Maya Ortiz-Ruiz" || m.Profile == nil || *m.Profile != (Profile{"Platform", "Ortiz-Ruiz", "Maya"}) {
			t.Errorf("viewer = %+v", m)
		}
	})
	t.Run("Korean members read family then given", func(t *testing.T) {
		if got := v.Members["uma"]; got.Name != "남궁민" || got.Avatar != "민" {
			t.Errorf("uma = %+v", got)
		}
		if got := v.Members["sven"]; got.Name != "Sven" || got.Avatar != "SV" {
			t.Errorf("sven = %+v", got)
		}
	})
}

// A member.join carries the profile in its payload; a bare name (what the
// protocol writes today) becomes the given name.
func TestMemberJoinProfile(t *testing.T) {
	genesis := json.RawMessage(`{"admins":["ada"],"members":[{"id":"ada","profile":{"org":"Platform","family":"Park","given":"Ada"},"keys":["ada-k1"],"leads":[]}]}`)
	evs := history([]step{
		{"pia.pc", wsledger.TypeMemberJoin, map[string]any{"member": "pia", "key": "pia-k1", "profile": map[string]any{"org": "Infra", "family": "Novak", "given": "Pia"}}},
		{"hong.pc", wsledger.TypeMemberJoin, map[string]any{"member": "hong", "key": "hong-k1", "profile": map[string]any{"org": "SW개발팀", "family": "홍", "given": "길동"}}},
		{"bare.pc", wsledger.TypeMemberJoin, map[string]any{"member": "bare", "key": "bare-k1", "name": "Bare Name"}},
		{"nogiven.pc", wsledger.TypeMemberJoin, map[string]any{"member": "nogiven", "key": "nogiven-k1", "profile": map[string]any{"org": "Ops"}}},
		{"pia.pc", TypeMemberProfile, map[string]any{"profile": map[string]any{"org": "Platform"}}},
	})
	v := Derive(newMemSource(genesis, "ada.pc", evs), "ada.pc", time.Time{}, 0)
	want := map[string]Profile{
		"pia":     {"Platform", "Novak", "Pia"}, // the join's org, then her own later edit
		"hong":    {"SW개발팀", "홍", "길동"},
		"bare":    {"", "", "Bare Name"},
		"nogiven": {"Ops", "", "nogiven"},
	}
	for id, p := range want {
		if got := v.Members[id].Profile; got != p {
			t.Errorf("%s = %+v, want %+v", id, got, p)
		}
	}
	if got := v.Members["hong"]; got.Name != "홍길동" || got.Avatar != "길동" {
		t.Errorf("hong = %+v", got)
	}
	if got := v.Members["bare"]; got.Name != "Bare Name" || got.Avatar != "BA" {
		t.Errorf("bare = %+v", got)
	}
}

// A genesis profile without a given name is a problem of the roles file.
func TestRolesProblemsProfileGiven(t *testing.T) {
	genesis := json.RawMessage(`{"admins":["ada"],"members":[{"id":"ada","profile":{"org":"Platform"},"keys":["ada-k1"],"leads":[]}]}`)
	v := Derive(newMemSource(genesis, "ada.pc", nil), "ada.pc", time.Time{}, 0)
	if len(v.RolesProblems) != 1 || v.RolesProblems[0] != "ada: profile.given required" {
		t.Errorf("rolesProblems = %v", v.RolesProblems)
	}
	if got := v.Members["ada"].Name; got != "ada" {
		t.Errorf("a profile with no given reads as the id, got %q", got)
	}
}

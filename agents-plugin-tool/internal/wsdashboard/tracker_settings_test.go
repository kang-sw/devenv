package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestTrackerStoreSettingsLeaveTheRolesDialog pins the Team tab's store
// settings (tracker.js settingsHtml): the priority levels and the
// attachment limit are a section of the Team card with their own dialogs,
// and the roles-event dialog no longer offers them, names each type it does
// offer by a table key, and shows only the selected type's fields.
func TestTrackerStoreSettingsLeaveTheRolesDialog(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	types := regexp.MustCompile(`var ROLE_TYPES = \[[^\]]*\];`).FindString(js)
	if types == "" {
		t.Fatal("tracker.js holds no ROLE_TYPES")
	}
	for _, gone := range []string{"set_priority_levels", "set_attachment_limit"} {
		if strings.Contains(types, gone) {
			t.Errorf("the roles-event dialog still offers %s: %s", gone, types)
		}
	}

	admin := regexp.MustCompile(`      case 'admin': \{\n[\s\S]*?\n        break;\n      \}\n`).FindString(js)
	if admin == "" {
		t.Fatal("dialogHtml holds no admin case")
	}
	for _, want := range []string{"wsT(ROLE_NAME[t])", "rfld(mt, ", "rfld('roles.add_member roles.set_lead', ", "rfld('roles.add_member roles.add_key roles.revoke_key', "} {
		if !strings.Contains(admin, want) {
			t.Errorf("the admin dialog lacks %q", want)
		}
	}
	for _, gone := range []string{"pnames", "pdefault", "maxbytes"} {
		if strings.Contains(admin, gone) {
			t.Errorf("the admin dialog still carries the %s field", gone)
		}
	}
	if !strings.Contains(js, "    rtypeShow(f);\n") || !strings.Contains(js, "if (t.name === 'rtype') rtypeShow(df);") {
		t.Error("the type fields are not shown per type on open and on change")
	}

	team := regexp.MustCompile(`function cardTeam\(v\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(team, "h += settingsHtml(!!me.admin);") {
		t.Error("the Team card does not draw the store settings")
	}
	set := regexp.MustCompile(`function settingsHtml\(admin\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	for _, want := range []string{"(admin ? btn(act, ", "'sprio')", "'satt')"} {
		if !strings.Contains(set, want) {
			t.Errorf("settingsHtml lacks %q", want)
		}
	}
	if !strings.Contains(js, "var DIALOGS = { profile: 1, admin: 1, sprio: 1, satt: 1") {
		t.Error("the settings dialogs are not dialog kinds")
	}
}

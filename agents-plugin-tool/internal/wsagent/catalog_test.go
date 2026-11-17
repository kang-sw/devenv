package wsagent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// validateCatalog checks a profile's shape: known items and grades, a
// readable guaranteed range, well-formed entry ranges, and no two entries
// of one item that both apply to a role at some version.
func validateCatalog(p *HarnessProfile) error {
	oldest, ok := parseHarnessVersion(p.Oldest)
	if !ok {
		return fmt.Errorf("oldest %q is not a version", p.Oldest)
	}
	newest, ok := parseHarnessVersion(p.Newest)
	if !ok {
		return fmt.Errorf("newest %q is not a version", p.Newest)
	}
	if compareHarnessVersions(oldest, newest) > 0 {
		return fmt.Errorf("oldest %s is above newest %s", p.Oldest, p.Newest)
	}
	for i, e := range p.Entries {
		if !slices.Contains(CatalogItems, e.Item) {
			return fmt.Errorf("entry %d: unknown item %q", i, e.Item)
		}
		switch e.Grade {
		case GradeNative, GradeSubstitute, GradeUnsupported:
		default:
			return fmt.Errorf("entry %d (%s): unknown grade %q", i, e.Item, e.Grade)
		}
		switch e.Role {
		case "", RoleLead, RoleWorker:
		default:
			return fmt.Errorf("entry %d (%s): unknown role %q", i, e.Item, e.Role)
		}
		if e.Source == "" {
			return fmt.Errorf("entry %d (%s): no source cited", i, e.Item)
		}
		lo, loOK := parseHarnessVersion(e.Since)
		hi, hiOK := parseHarnessVersion(e.Until)
		if (e.Since != "" && !loOK) || (e.Until != "" && !hiOK) {
			return fmt.Errorf("entry %d (%s): unreadable range [%q, %q)", i, e.Item, e.Since, e.Until)
		}
		if loOK && hiOK && compareHarnessVersions(lo, hi) >= 0 {
			return fmt.Errorf("entry %d (%s): empty range [%s, %s)", i, e.Item, e.Since, e.Until)
		}
		for j, f := range p.Entries[:i] {
			if f.Item != e.Item || (f.Role != "" && e.Role != "" && f.Role != e.Role) {
				continue
			}
			if rangesOverlap(f.Since, f.Until, e.Since, e.Until) {
				return fmt.Errorf("entries %d and %d (%s) overlap", j, i, e.Item)
			}
		}
	}
	return nil
}

// rangesOverlap reports whether [aSince, aUntil) and [bSince, bUntil)
// share a version; empty bounds are open.
func rangesOverlap(aSince, aUntil, bSince, bUntil string) bool {
	below := func(until, since string) bool { // until <= since
		if until == "" || since == "" {
			return false
		}
		u, _ := parseHarnessVersion(until)
		s, _ := parseHarnessVersion(since)
		return compareHarnessVersions(u, s) <= 0
	}
	return !below(aUntil, bSince) && !below(bUntil, aSince)
}

func TestParseHarnessVersion(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want []int
		ok   bool
	}{
		{"2.1.326", []int{2, 1, 326}, true},
		{"2.1.326 (Claude Code)", []int{2, 1, 326}, true},
		{"v1.5.2", []int{1, 5, 2}, true},
		{" 1.5.2-beta.1 ", []int{1, 5, 2}, true},
		{"1.5.2+build", []int{1, 5, 2}, true},
		{"2.1", []int{2, 1}, true},
		{"", nil, false},
		{"claude", nil, false},
		{"2.x.1", nil, false},
		{"2..1", nil, false},
	} {
		got, ok := parseHarnessVersion(tt.in)
		if ok != tt.ok || !slices.Equal(got, tt.want) {
			t.Errorf("parseHarnessVersion(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	a, _ := parseHarnessVersion("2.1")
	b, _ := parseHarnessVersion("2.1.0")
	if compareHarnessVersions(a, b) != 0 {
		t.Error("2.1 and 2.1.0 must compare equal")
	}
	c, _ := parseHarnessVersion("2.1.10")
	d, _ := parseHarnessVersion("2.1.9")
	if compareHarnessVersions(c, d) <= 0 {
		t.Error("2.1.10 must order above 2.1.9 (numeric, not lexical)")
	}
}

func TestVersionInRange(t *testing.T) {
	for _, tt := range []struct {
		version, since, until string
		want                  bool
	}{
		{"2.1.320", "", "", true},
		{"garbage", "", "", true},
		{"2.1.320", "2.1.320", "", true},
		{"2.1.319", "2.1.320", "", false},
		{"2.1.320", "", "2.1.320", false},
		{"2.1.319", "", "2.1.320", true},
		{"2.1.321", "2.1.320", "2.1.322", true},
		{"2.1.322", "2.1.320", "2.1.322", false},
		{"garbage", "2.1.320", "", false},
	} {
		if got := versionInRange(tt.version, tt.since, tt.until); got != tt.want {
			t.Errorf("versionInRange(%q, %q, %q) = %v, want %v", tt.version, tt.since, tt.until, got, tt.want)
		}
	}
}

// fixtureCatalog exercises version ranges and roles independent of any
// real harness's values.
func fixtureCatalog() *HarnessProfile {
	return &HarnessProfile{
		Oldest: "1.2.0",
		Newest: "1.4.0",
		Entries: []CapabilityEntry{
			{Item: ItemSteer, Role: RoleLead, Grade: GradeNative, Source: "fixture"},
			{Item: ItemSteer, Role: RoleWorker, Until: "1.3.0", Grade: GradeUnsupported, Source: "fixture"},
			{Item: ItemSteer, Role: RoleWorker, Since: "1.3.0", Grade: GradeSubstitute, Source: "fixture"},
			{Item: ItemToolRemoval, Grade: GradeNative, Values: []string{"a", "b"}, Source: "fixture"},
			{Item: ItemCompaction, Since: "1.3.5", Grade: GradeNative, Source: "fixture"},
		},
	}
}

func TestResolveVersionRanges(t *testing.T) {
	p := fixtureCatalog()
	if err := validateCatalog(p); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name           string
		role           CatalogRole
		version        string
		profileVersion string
		unverified     bool
		steer          CapabilityGrade
		compaction     CapabilityGrade
	}{
		{"in range, before a worker change", RoleWorker, "1.2.5", "1.2.5", false, GradeUnsupported, GradeUnsupported},
		{"in range, at a worker change", RoleWorker, "1.3.0", "1.3.0", false, GradeSubstitute, GradeUnsupported},
		{"in range, at the lower bound", RoleWorker, "1.2.0", "1.2.0", false, GradeUnsupported, GradeUnsupported},
		{"in range, at the upper bound", RoleWorker, "1.4.0", "1.4.0", false, GradeSubstitute, GradeNative},
		{"lead entry has no range", RoleLead, "1.2.0", "1.2.0", false, GradeNative, GradeUnsupported},
		{"below range resolves at the oldest", RoleWorker, "1.1.9", "1.2.0", true, GradeUnsupported, GradeUnsupported},
		{"above range resolves at the newest", RoleWorker, "9.0.0", "1.4.0", true, GradeSubstitute, GradeNative},
		{"unknown version resolves at the newest", RoleWorker, "", "1.4.0", true, GradeSubstitute, GradeNative},
		{"unreadable version resolves at the newest", RoleWorker, "nightly", "1.4.0", true, GradeSubstitute, GradeNative},
		{"version line with a suffix", RoleWorker, "1.3.6 (Harness)", "1.3.6 (Harness)", false, GradeSubstitute, GradeNative},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caps := p.Resolve(tt.role, tt.version)
			if caps.ProfileVersion != tt.profileVersion || caps.Unverified != tt.unverified || caps.Version != tt.version || caps.Role != tt.role {
				t.Fatalf("resolved %+v; want profile %q unverified %v", caps, tt.profileVersion, tt.unverified)
			}
			if got := caps.Grade(ItemSteer); got != tt.steer {
				t.Errorf("steer = %s, want %s", got, tt.steer)
			}
			if got := caps.Grade(ItemCompaction); got != tt.compaction {
				t.Errorf("compaction = %s, want %s", got, tt.compaction)
			}
		})
	}
}

func TestResolveFillsEveryItem(t *testing.T) {
	caps := fixtureCatalog().Resolve(RoleWorker, "1.3.0")
	if len(caps.Items) != len(CatalogItems) {
		t.Fatalf("resolved %d items, want all %d", len(caps.Items), len(CatalogItems))
	}
	if got := caps.Grade(ItemRewind); got != GradeUnsupported {
		t.Errorf("undeclared rewind = %s, want unsupported", got)
	}
	if caps.Allows(ItemRewind) || !caps.Allows(ItemSteer) || !caps.Allows(ItemToolRemoval) {
		t.Errorf("Allows disagrees with grades: %+v", caps.Items)
	}
	if got := (Capabilities{}).Grade(ItemSteer); got != GradeUnsupported {
		t.Errorf("zero Capabilities grade = %s, want unsupported", got)
	}
}

// TestResolveCopiesValues: one profile is shared by every reader through
// its registration, so a resolved value range must not alias its entries.
func TestResolveCopiesValues(t *testing.T) {
	p := fixtureCatalog()
	caps := p.Resolve(RoleWorker, "1.3.0")
	caps.Items[ItemToolRemoval].Values[0] = "edited"
	if got := p.Entries[3].Values; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("profile values = %v after editing a resolved copy", got)
	}
	if got := p.Resolve(RoleWorker, "1.3.0").Items[ItemToolRemoval].Values; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("re-resolved values = %v after editing a resolved copy", got)
	}
}

func TestValidateCatalogRejectsOverlap(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []CapabilityEntry
	}{
		{"same role open ranges", []CapabilityEntry{
			{Item: ItemSteer, Role: RoleLead, Grade: GradeNative, Source: "x"},
			{Item: ItemSteer, Role: RoleLead, Grade: GradeUnsupported, Source: "x"},
		}},
		{"every-role entry over a role entry", []CapabilityEntry{
			{Item: ItemSteer, Grade: GradeNative, Source: "x"},
			{Item: ItemSteer, Role: RoleWorker, Since: "1.3.0", Grade: GradeUnsupported, Source: "x"},
		}},
		{"overlapping bounded ranges", []CapabilityEntry{
			{Item: ItemSteer, Until: "1.3.1", Grade: GradeNative, Source: "x"},
			{Item: ItemSteer, Since: "1.3.0", Grade: GradeUnsupported, Source: "x"},
		}},
		{"unknown grade", []CapabilityEntry{{Item: ItemSteer, Grade: "maybe", Source: "x"}}},
		{"unknown item", []CapabilityEntry{{Item: "telepathy", Grade: GradeNative, Source: "x"}}},
		{"empty range", []CapabilityEntry{{Item: ItemSteer, Since: "1.3.0", Until: "1.3.0", Grade: GradeNative, Source: "x"}}},
		{"no source", []CapabilityEntry{{Item: ItemSteer, Grade: GradeNative}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := &HarnessProfile{Oldest: "1.0.0", Newest: "2.0.0", Entries: tt.entries}
			if err := validateCatalog(p); err == nil {
				t.Fatal("validateCatalog accepted an invalid profile")
			}
		})
	}
	adjacent := &HarnessProfile{Oldest: "1.0.0", Newest: "2.0.0", Entries: []CapabilityEntry{
		{Item: ItemSteer, Until: "1.3.0", Grade: GradeNative, Source: "x"},
		{Item: ItemSteer, Since: "1.3.0", Grade: GradeUnsupported, Source: "x"},
		{Item: ItemApproval, Role: RoleLead, Grade: GradeNative, Source: "x"},
		{Item: ItemApproval, Role: RoleWorker, Grade: GradeUnsupported, Source: "x"},
	}}
	if err := validateCatalog(adjacent); err != nil {
		t.Fatalf("adjacent ranges and split roles rejected: %v", err)
	}
}

// TestClaudeCatalogValues pins the reference profile: each grade is what
// the ws adapter delivers today, per role.
func TestClaudeCatalogValues(t *testing.T) {
	p := ClaudeCatalog()
	if err := validateCatalog(p); err != nil {
		t.Fatal(err)
	}
	want := map[CatalogRole]map[CapabilityItem]CapabilityGrade{
		RoleLead: {
			ItemSkillList: GradeUnsupported, ItemSkillInjection: GradeNative, ItemCompaction: GradeNative,
			ItemRewind: GradeUnsupported, ItemToolRemoval: GradeUnsupported, ItemSubagentRemoval: GradeUnsupported,
			ItemModelEffort: GradeNative, ItemApproval: GradeNative, ItemSteer: GradeNative,
			ItemInterrupt: GradeNative, ItemFollowUp: GradeSubstitute, ItemResume: GradeNative,
			ItemReasoningStream: GradeUnsupported, ItemUsageQuery: GradeNative,
		},
		RoleWorker: {
			ItemSkillList: GradeUnsupported, ItemSkillInjection: GradeNative, ItemCompaction: GradeUnsupported,
			ItemRewind: GradeUnsupported, ItemToolRemoval: GradeNative, ItemSubagentRemoval: GradeNative,
			ItemModelEffort: GradeNative, ItemApproval: GradeUnsupported, ItemSteer: GradeUnsupported,
			ItemInterrupt: GradeNative, ItemFollowUp: GradeSubstitute, ItemResume: GradeNative,
			ItemReasoningStream: GradeUnsupported, ItemUsageQuery: GradeUnsupported,
		},
	}
	for _, version := range []string{p.Oldest, p.Newest} {
		for role, items := range want {
			caps := p.Resolve(role, version)
			if caps.Unverified {
				t.Errorf("%s at %s resolved unverified", role, version)
			}
			for _, item := range CatalogItems {
				if got := caps.Grade(item); got != items[item] {
					t.Errorf("%s %s at %s = %s, want %s", role, item, version, got, items[item])
				}
			}
		}
	}
	worker := p.Resolve(RoleWorker, p.Newest)
	if got := worker.Items[ItemSubagentRemoval].Values; !slices.Equal(got, []string{"Agent"}) {
		t.Errorf("worker subagent removal values = %v, want [Agent]", got)
	}
	if got := worker.Items[ItemToolRemoval].Values; !slices.Equal(got, []string{CatalogAnyValue}) {
		t.Errorf("worker tool removal values = %v, want [%s]", got, CatalogAnyValue)
	}
	// Model/effort choices and permission modes stay sourced from the
	// backend and its registration: the catalog duplicates neither.
	for _, role := range []CatalogRole{RoleLead, RoleWorker} {
		caps := p.Resolve(role, p.Newest)
		for _, item := range []CapabilityItem{ItemModelEffort, ItemApproval} {
			if v := caps.Items[item].Values; v != nil {
				t.Errorf("%s %s carries values %v; they belong to the registration", role, item, v)
			}
		}
	}
	// Each bound of the guaranteed range is a recorded fixture's version.
	recorded := claudeFixtureVersions(t)
	for _, bound := range []string{p.Oldest, p.Newest} {
		if !recorded[bound] {
			t.Errorf("bound %s has no recorded fixture; recorded %v", bound, recorded)
		}
	}
}

// claudeFixtureVersions collects the claude_code_version values of the
// recorded Claude stream fixtures.
func claudeFixtureVersions(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "claude-*.jsonl"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Claude fixtures: %v", err)
	}
	re := regexp.MustCompile(`"claude_code_version":"([^"]+)"`)
	out := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllSubmatch(data, -1) {
			out[string(m[1])] = true
		}
	}
	return out
}

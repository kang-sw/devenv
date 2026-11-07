package wsledger

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePatch(t *testing.T) {
	const day = "events/2026/11/08.jsonl"
	hdr := "diff --git a/" + day + " b/" + day + "\n"
	cases := []struct {
		name  string
		patch string
		// want is the accepted append; ignored when bad is set.
		want rawCommit
		bad  string // substring of PatchBad; "" means accepted
	}{
		{
			name: "valid append",
			patch: "\n" + hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2,0 +3,2 @@\n" +
				"+{\"seq\":3}\n" +
				"+{\"seq\":4}\n",
			want: rawCommit{Path: day, OldStart: 2, Added: []string{`{"seq":3}`, `{"seq":4}`}},
		},
		{
			name: "new file",
			patch: hdr +
				"new file mode 100644\n" +
				"index 0000000..2222222\n" +
				"--- /dev/null\n" +
				"+++ b/" + day + "\n" +
				"@@ -0,0 +1 @@\n" +
				"+{\"seq\":1}\n",
			want: rawCommit{Path: day, NewFile: true, OldStart: 0, Added: []string{`{"seq":1}`}},
		},
		{
			name: "removed line",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2 +1,0 @@\n" +
				"-{\"seq\":2}\n",
			bad: "more than append",
		},
		{
			name: "replaced line",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2 +2,2 @@\n" +
				"-{\"seq\":2}\n" +
				"+{\"seq\":2,\"x\":1}\n" +
				"+{\"seq\":3}\n",
			bad: "more than append",
		},
		{
			name: "two files",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2,0 +3 @@\n" +
				"+{\"seq\":3}\n" +
				"diff --git a/events/2026/11/09.jsonl b/events/2026/11/09.jsonl\n" +
				"new file mode 100644\n" +
				"index 0000000..3333333\n" +
				"--- /dev/null\n" +
				"+++ b/events/2026/11/09.jsonl\n" +
				"@@ -0,0 +1 @@\n" +
				"+{\"seq\":4}\n",
			bad: "more than append",
		},
		{
			name: "second file after a hunkless diff",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"diff --git a/x b/x\n",
			bad: "more than one file",
		},
		{
			name: "mode change",
			patch: hdr +
				"old mode 100644\n" +
				"new mode 100755\n",
			bad: "unexpected patch line",
		},
		{
			name: "binary file",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"Binary files a/" + day + " and b/" + day + " differ\n",
			bad: "unexpected patch line",
		},
		{
			name: "no newline at end of file",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2,0 +3 @@\n" +
				"+{\"seq\":3}\n" +
				"\\ No newline at end of file\n",
			bad: "more than append",
		},
		{
			name: "executable new file",
			patch: hdr +
				"new file mode 100755\n" +
				"index 0000000..2222222\n" +
				"--- /dev/null\n" +
				"+++ b/" + day + "\n" +
				"@@ -0,0 +1 @@\n" +
				"+{\"seq\":1}\n",
			bad: "non-regular mode",
		},
		{
			name: "symlink on index line",
			patch: hdr +
				"index 1111111..2222222 120000\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2,0 +3 @@\n" +
				"+{\"seq\":3}\n",
			bad: "non-regular mode",
		},
		{
			name:  "empty patch",
			patch: "",
			bad:   "changes no event file",
		},
		{
			name:  "blank lines only",
			patch: "\n\n\n",
			bad:   "changes no event file",
		},
		{
			name: "header without a hunk",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n",
			bad: "appends no event line",
		},
		{
			name: "hunk count disagrees with the lines",
			patch: hdr +
				"index 1111111..2222222 100644\n" +
				"--- a/" + day + "\n" +
				"+++ b/" + day + "\n" +
				"@@ -2,0 +3,3 @@\n" +
				"+{\"seq\":3}\n",
			bad: "more than append",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c rawCommit
			parsePatch(&c, tc.patch)
			if tc.bad != "" {
				if !strings.Contains(c.PatchBad, tc.bad) {
					t.Fatalf("PatchBad = %q, want it to mention %q", c.PatchBad, tc.bad)
				}
				if c.Added != nil {
					t.Fatalf("a rejected patch kept added lines: %q", c.Added)
				}
				return
			}
			if c.PatchBad != "" {
				t.Fatalf("PatchBad = %q, want accepted", c.PatchBad)
			}
			got := rawCommit{Path: c.Path, NewFile: c.NewFile, OldStart: c.OldStart, Added: c.Added}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsed %+v, want %+v", got, tc.want)
			}
		})
	}
}

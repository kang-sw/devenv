package wsinbox

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEntryExcerptStripsOnlyValidTransportGroups(t *testing.T) {
	group, err := SelectionBlock(Selection{Mentions: []Mention{{Topic: "t", Text: "source"}}, Comment: "memo", Images: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	e := Entry{Parts: []string{group + "\n\n" + MentionBlock([]Mention{{Topic: "t", Text: "legacy"}}) + "\n<script>literal</script> [Image #1]\nImage #1: /tmp/a.png"}}
	if got := e.Excerpt(); got != "<script>literal</script>" {
		t.Fatalf("excerpt %q", got)
	}
	malformed := "<wb:selection>\ninvalid [Image #1]\n</wb:selection>"
	if got := (Entry{Parts: []string{malformed}}).Excerpt(); got != malformed {
		t.Fatalf("malformed group changed: %q", got)
	}
	if got := (Entry{Parts: []string{strings.Repeat("語", 241)}}).Excerpt(); utf8.RuneCountInString(got) != 240 || !utf8.ValidString(got) {
		t.Fatal("rune boundary", got)
	}
}

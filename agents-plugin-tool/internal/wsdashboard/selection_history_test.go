package wsdashboard

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func selectionHistoryBlock(t *testing.T, s wsinbox.Selection) string {
	t.Helper()
	block, err := wsinbox.SelectionBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	return block
}

func selectionHistoryRender(t *testing.T, entry wsinbox.Entry, attachDir string, held bool, cache *entryCache) string {
	t.Helper()
	in := pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Now: testNow, AttachDir: attachDir}
	region := regionEntries
	if held {
		in.Held = []wsinbox.Record{{Name: "held.json", Entry: entry}}
		region = regionConvLive
	} else {
		in.Ring = []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: entry.Text()})}
	}
	set, err := renderSessionRegions(buildSessionView(in), cache)
	if err != nil {
		t.Fatal(err)
	}
	return string(set[region].HTML)
}

func TestSelectionHistoryKeepsPushesSourceMemoAndImagesTogether(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 3)
	for i, name := range []string{"0123456789abcdef-1.png", "0123456789abcdef-2.png", "0123456789abcdef-3.png"} {
		paths[i] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[i], []byte(pngBytes), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Literal path lines and markers in source/memo are not attachment refs.
	first := wsinbox.Selection{Mentions: []wsinbox.Mention{
		{Topic: "plan", ID: "3", Entry: "e7", Text: "first <quote> [Image #8]\nImage #1: " + paths[0]},
		{Topic: "plan", Entry: "e8", Text: "second excerpt"},
	}, Comment: "memo <script> [Image #9] &#91;", Images: []int{1}}
	second := wsinbox.Selection{Mentions: []wsinbox.Mention{{Topic: "other", Entry: "e9", Text: "last excerpt"}}, Comment: "", Images: []int{2}}
	text := "before\n" + selectionHistoryBlock(t, first) + "\nbetween\n" + selectionHistoryBlock(t, second) + "\nafter [Image #3]"
	entry := wsinbox.Entry{Parts: []string{text}, Images: paths, ImageNums: []int{1, 2, 3}, SentAt: testNow.UnixMilli()}
	before, _ := json.Marshal(entry)
	for _, held := range []bool{true, false} {
		for pass := 0; pass < 2; pass++ {
			output := selectionHistoryRender(t, entry, dir, held, newEntryCache())
			groups := regexp.MustCompile(`(?s)<section class="selection-bundle">.*?</section>`).FindAllString(output, -1)
			if len(groups) != 2 {
				t.Fatalf("held=%v: want two push groups, got %d: %s", held, len(groups), output)
			}
			for i, s := range []wsinbox.Selection{first, second} {
				group := groups[i]
				if strings.Count(group, `class="selection-comment"`) != 1 || !strings.Contains(group, `<div class="selection-comment">`+html.EscapeString(s.Comment)+`</div>`) {
					t.Fatalf("held=%v: group %d lost its one memo: %s", held, i, group)
				}
				last := -1
				for _, m := range s.Mentions {
					at := strings.Index(group, `data-wb-entry="`+m.Entry+`"`)
					if at <= last || !strings.Contains(group, html.EscapeString(m.Text)) {
						t.Fatalf("held=%v: group %d lost ordered exact source %+v: %s", held, i, m, group)
					}
					last = at
				}
				name := filepath.Base(paths[i])
				if strings.Count(output, `src="/s/self/attachment/`+name+`"`) != 1 || !strings.Contains(group, `src="/s/self/attachment/`+name+`"`) {
					t.Fatalf("held=%v: image %d escaped its owner: %s", held, i+1, output)
				}
				for _, other := range paths {
					if other != paths[i] && strings.Contains(group, `src="/s/self/attachment/`+filepath.Base(other)+`"`) {
						t.Fatal("another selection or loose image moved into this group")
					}
				}
			}
			if !strings.Contains(output, "before") || !strings.Contains(output, "between") || !strings.Contains(output, "after <span class=\"imgtag\"") || strings.Contains(strings.Join(groups, ""), `class="imgtag"`) {
				t.Fatal("ordinary text or literal source/memo marker handling changed")
			}
			if strings.Contains(output, "<script>") || strings.Contains(output, "&lt;wb:selection&gt;") || strings.Contains(output, "&amp;#91;") && !strings.Contains(output, html.EscapeString(first.Comment)) {
				t.Fatal("unsafe, raw or double-decoded grouped content")
			}
		}
	}
	after, _ := json.Marshal(entry)
	if !bytes.Equal(before, after) {
		t.Fatal("history mutated the send")
	}
}

func TestSelectionHistoryMalformedGroupsStayWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0123456789abcdef-1.png")
	if err := os.WriteFile(path, []byte(pngBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := selectionHistoryBlock(t, wsinbox.Selection{Mentions: []wsinbox.Mention{{Topic: "t", Entry: "e3", Text: "nested"}}, Comment: "memo"})
	for _, raw := range []string{
		"<wb:selection>\n" + valid + "\n</wb:selection>",
		"<wb:selection>\n<wb:mention topic=\"t\">source</wb:mention>\n<unknown/>\n[Image #1]\nImage #1: " + path + "\n</wb:selection>",
		"<wb:selection>\n<wb:mention topic=\"t\">source</wb:mention>",
	} {
		for _, held := range []bool{true, false} {
			output := selectionHistoryRender(t, wsinbox.Entry{Parts: []string{raw}}, dir, held, nil)
			if !strings.Contains(output, html.EscapeString(raw)) || strings.Contains(output, `class="selection-bundle"`) || strings.Contains(output, `class="imgtag"`) || strings.Contains(output, `src="/s/self/attachment/`) {
				t.Fatalf("held=%v: malformed envelope was partially interpreted: %s", held, output)
			}
		}
	}
	// A failed sibling remains literal while an independent valid group renders.
	raw := "<wb:selection>\n<unknown/>\n</wb:selection>"
	output := selectionHistoryRender(t, wsinbox.Entry{Parts: []string{raw + "\n" + valid}}, dir, false, nil)
	if !strings.Contains(output, html.EscapeString(raw)) || strings.Count(output, `class="selection-bundle"`) != 1 {
		t.Fatal("malformed sibling swallowed or exposed the independent valid group")
	}
}

func TestSelectionHistoryCacheTracksOwnedAttachmentRemoval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0123456789abcdef-1.png")
	if err := os.WriteFile(path, []byte(pngBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	block := selectionHistoryBlock(t, wsinbox.Selection{Mentions: []wsinbox.Mention{{Topic: "t", Entry: "e7", Text: "source"}}, Comment: "memo", Images: []int{1}})
	entry := wsinbox.Entry{Parts: []string{block}, Images: []string{path}, ImageNums: []int{1}}
	cache := newEntryCache()
	for i := 0; i < 2; i++ {
		output := selectionHistoryRender(t, entry, dir, false, cache)
		if !strings.Contains(output, `class="selection-bundle"`) || !strings.Contains(output, `src="/s/self/attachment/`) {
			t.Fatal("existing group image has no thumbnail")
		}
	}
	if cache.buildCount() != 1 {
		t.Fatal("unchanged owned image missed cache")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	output := selectionHistoryRender(t, entry, dir, false, cache)
	if cache.buildCount() != 2 || strings.Contains(output, `src="/s/self/attachment/`) || !strings.Contains(output, "Image #1: "+path) || !strings.Contains(output, `class="selection-comment">memo`) {
		t.Fatalf("removed owned image froze thumbnail or lost fallback: builds=%d %s", cache.buildCount(), output)
	}
}

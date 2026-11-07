package wsi18n

import (
	"strings"
	"testing"
)

func TestLanguagesAreDiscoveredFromTables(t *testing.T) {
	langs := Languages()
	if len(langs) < 2 || langs[0].Code != Default {
		t.Fatalf("Languages() = %+v, want English first and at least one more", langs)
	}
	byCode := map[string]Language{}
	for _, l := range langs {
		byCode[l.Code] = l
	}
	if byCode["en"].Name != "English" || byCode["ko"].Name != "한국어" {
		t.Fatalf("language names come from lang.name: %+v", byCode)
	}
	if byCode["ko"].Tag != "KO" {
		t.Fatalf("Tag = %q, want KO", byCode["ko"].Tag)
	}
}

func TestTFallsBackToEnglishForAMissingKey(t *testing.T) {
	const key = "test.only.english"
	tables[Default][key] = "english only {0}"
	defer delete(tables[Default], key)
	if got := T("ko", key, 3); got != "english only 3" {
		t.Fatalf("T(ko) with a key only English has = %q", got)
	}
	if got := T("xx", "board.empty"); got != tables[Default]["board.empty"] {
		t.Fatalf("an unsupported language renders English, got %q", got)
	}
	if got := T("en", "no.such.key"); got != "no.such.key" {
		t.Fatalf("a key no table has renders as itself, got %q", got)
	}
}

func TestPluralAndFill(t *testing.T) {
	if got := Plural("en", "batch.calls", 1); got != "1 call" {
		t.Fatalf("Plural 1 = %q", got)
	}
	if got := Plural("en", "batch.calls", 4); got != "4 calls" {
		t.Fatalf("Plural 4 = %q", got)
	}
	if got := Plural("ko", "batch.calls", 4); got != "호출 4회" {
		t.Fatalf("Plural ko = %q", got)
	}
	if got := Fill("{1}-{0}-{1}", "a", "b"); got != "b-a-b" {
		t.Fatalf("Fill = %q", got)
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name, cookie, accept, want string
	}{
		{"cookie wins over the browser", "ko", "en-US,en;q=0.9", "ko"},
		{"unknown cookie follows the browser", "xx", "ko-KR,ko;q=0.9", "ko"},
		{"no cookie, Korean browser", "", "ko-KR,ko;q=0.9,en;q=0.8", "ko"},
		{"first supported language by quality", "", "nl;q=0.9,ko;q=0.8,en;q=0.7", "ko"},
		{"quality order beats header order", "", "en;q=0.5,ko;q=0.9", "ko"},
		{"unsupported browser language", "", "nl-NL,sv;q=0.8", "en"},
		{"French browser", "", "fr-CA,fr;q=0.9", "fr"},
		{"Japanese browser", "", "ja-JP,ja;q=0.9,en;q=0.8", "ja"},
		{"German beats English by quality", "", "en;q=0.5,de-AT;q=0.9", "de"},
		{"Brazilian Portuguese", "", "pt-BR,pt;q=0.9", "pt-BR"},
		{"European Portuguese takes the same table", "", "pt-PT", "pt-BR"},
		{"bare zh is Simplified", "", "zh", "zh-Hans"},
		{"zh-CN is Simplified", "", "zh-CN,zh;q=0.9", "zh-Hans"},
		{"zh-SG is Simplified", "", "zh-SG", "zh-Hans"},
		{"zh-Hans is Simplified", "", "zh-Hans-CN", "zh-Hans"},
		{"zh-TW is Traditional", "", "zh-TW,zh;q=0.9", "zh-Hant"},
		{"zh-HK is Traditional", "", "zh-HK", "zh-Hant"},
		{"zh-MO is Traditional", "", "zh-MO", "zh-Hant"},
		{"zh-Hant is Traditional", "", "zh-Hant", "zh-Hant"},
		{"a script beats a region", "", "zh-Hant-CN", "zh-Hant"},
		{"tag case does not matter", "", "ZH-tw", "zh-Hant"},
		{"cookie in any case names the table", "zh-hant", "en", "zh-Hant"},
		{"cookie names a Chinese script", "zh-Hans", "zh-TW", "zh-Hans"},
		{"no header", "", "", "en"},
		{"q=0 refuses a language", "", "ko;q=0,en", "en"},
	}
	for _, c := range cases {
		if got := Resolve(c.cookie, c.accept); got != c.want {
			t.Errorf("%s: Resolve(%q, %q) = %q, want %q", c.name, c.cookie, c.accept, got, c.want)
		}
	}
	if Pref("zh-Hant") != "zh-Hant" || Pref("ZH-HANT") != "zh-Hant" || Pref("zh") != "" {
		t.Fatal("Pref admits a table code in any case, not a bare language")
	}
	if Pref("ko") != "ko" || Pref("") != "" || Pref("xx") != "" {
		t.Fatal("Pref admits only an available language")
	}
}

func TestClientTableHoldsOnlyClientKeysWithEnglishFallback(t *testing.T) {
	const key = ClientPrefix + "test.only.english"
	tables[Default][key] = "x"
	defer delete(tables[Default], key)
	ko := ClientTable("ko")
	if ko[key] != "x" {
		t.Fatalf("a client key missing from ko falls back to English, got %q", ko[key])
	}
	for k := range ko {
		if !strings.HasPrefix(k, ClientPrefix) {
			t.Fatalf("client table holds the non-client key %q", k)
		}
	}
	if ko["js.ago"] != "{0} 전" {
		t.Fatalf("ko client table = %q for js.ago", ko["js.ago"])
	}
}

func TestScriptEscapesAndDefaults(t *testing.T) {
	js := string(Script("ko"))
	if !strings.Contains(js, `lang: "ko"`) || !strings.Contains(js, "window.wsT") {
		t.Fatalf("Script(ko) = %.200s", js)
	}
	if strings.Contains(js, "</script") {
		t.Fatal("Script must not close its element")
	}
	if !strings.Contains(string(Script("zz")), `lang: "en"`) {
		t.Fatal("an unsupported language scripts as English")
	}
}

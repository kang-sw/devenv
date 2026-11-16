package wsi18n

import (
	"fmt"
	"html/template"
	"strings"
	"sync"
	"testing"
)

// Keep cache state local to each test without racing concurrent requests.
// These tests deliberately do not run in parallel with table-mutating tests.
func resetSubsetCache(t *testing.T) {
	t.Helper()
	reset := func() {
		subsetScripts.Lock()
		defer subsetScripts.Unlock()
		subsetScripts.entries = make(map[subsetScriptKey]template.JS)
		subsetScripts.order = [subsetCacheLimit]subsetScriptKey{}
		subsetScripts.next = 0
	}
	reset()
	t.Cleanup(reset)
}

func uncachedSubset(lang, prefix string) template.JS {
	return filteredScript(lang, func(k string) bool { return strings.HasPrefix(k, prefix) })
}

func TestSubsetScriptByteContract(t *testing.T) {
	resetSubsetCache(t)
	languages := []string{"", "unknown", "EN", "KO", "zh-hant"}
	for _, lang := range Languages() {
		languages = append(languages, lang.Code)
	}
	for _, lang := range languages {
		for _, prefix := range []string{"", "js.", "js.img.", "js.files.", "lang.", "missing", "</script>", strings.Repeat("x", subsetCacheMaxPrefix+1)} {
			want := uncachedSubset(lang, prefix)
			for pass := 0; pass < 2; pass++ {
				if got := SubsetScript(lang, prefix); got != want {
					t.Fatalf("SubsetScript(%q, %q), pass %d differs from uncached bytes", lang, prefix, pass)
				}
			}
		}
	}
}

func TestSubsetScriptFallbackAndEscaping(t *testing.T) {
	load()
	resetSubsetCache(t)
	const key = "js.test.subset.fallback"
	tables[Default][key] = "</script><script>&\u2028\u2029 {0}"
	t.Cleanup(func() { delete(tables[Default], key) })
	for _, lang := range []string{"en", "ko", "unsupported"} {
		got := SubsetScript(lang, key)
		want := script(func() string {
			if Supported(lang) {
				return lang
			}
			return Default
		}(), map[string]string{key: tables[Default][key]})
		if got != want || strings.Contains(string(got), "</script>") {
			t.Fatalf("fallback/escaping changed for %q: %s", lang, got)
		}
	}
}

func TestSubsetScriptCacheHitsAndBounds(t *testing.T) {
	resetSubsetCache(t)
	want := SubsetScript(Default, "js.img.")
	for _, lang := range []string{"", "not-a-language", "EN"} {
		if got := SubsetScript(lang, "js.img."); got != want {
			t.Fatalf("unsupported %q must share the English result", lang)
		}
	}
	if len(subsetScripts.entries) != 1 {
		t.Fatal("fallback languages must share one effective-language entry")
	}
	if allocs := testing.AllocsPerRun(100, func() { SubsetScript(Default, "js.img.") }); allocs != 0 {
		t.Fatalf("warm cache hit allocated %g times; must not rebuild the script", allocs)
	}
	SubsetScript("ko", "js.img.")
	if len(subsetScripts.entries) != 2 {
		t.Fatal("supported languages must have separate entries")
	}
	for i := 0; i < subsetCacheLimit*3; i++ {
		prefix := fmt.Sprintf("absent.%d", i)
		if got := SubsetScript(Default, prefix); got != uncachedSubset(Default, prefix) {
			t.Fatalf("churn changed output for %q", prefix)
		}
		if len(subsetScripts.entries) > subsetCacheLimit {
			t.Fatal("cache exceeded its entry limit")
		}
	}
	if _, ok := subsetScripts.entries[subsetScriptKey{Default, "js.img."}]; ok {
		t.Fatal("oldest entry was not evicted")
	}
	next := subsetScripts.next
	SubsetScript(Default, strings.Repeat("x", subsetCacheMaxPrefix+1))
	if subsetScripts.next != next {
		t.Fatal("oversized prefix must bypass the cache")
	}
	if got := SubsetScript(Default, "js.img."); got != want {
		t.Fatal("eviction and rebuild changed output")
	}
	for key := range subsetScripts.entries {
		if len(key.prefix) > subsetCacheMaxPrefix {
			t.Fatal("cache retained an oversized prefix")
		}
	}
}

func TestSubsetScriptConcurrentHitsAndEvictions(t *testing.T) {
	resetSubsetCache(t)
	type request struct {
		lang, prefix string
		want         template.JS
	}
	var requests []request
	for _, lang := range []string{Default, "ko", "zh-Hant", "unknown"} {
		for _, prefix := range []string{"js.img.", "js.files.", "", strings.Repeat("x", subsetCacheMaxPrefix+1)} {
			requests = append(requests, request{lang, prefix, uncachedSubset(lang, prefix)})
		}
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				r := requests[(worker+i)%len(requests)]
				if got := SubsetScript(r.lang, r.prefix); got != r.want {
					t.Errorf("concurrent output differs for %q, %q", r.lang, r.prefix)
					return
				}
				SubsetScript(Default, fmt.Sprintf("churn.%d.%d", worker, i))
			}
		}(worker)
	}
	close(start)
	wg.Wait()
	if len(subsetScripts.entries) > subsetCacheLimit {
		t.Fatal("concurrent cache exceeded its entry limit")
	}
}

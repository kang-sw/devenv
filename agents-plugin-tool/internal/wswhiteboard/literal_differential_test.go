package wswhiteboard

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

type literalSnapshot struct {
	Commands []map[string]any
	Problems int
	Complete bool
}

// Only stdlib and the original parser/grammar are compiled. Every archived byte
// is verified before extracting the two parser files and the grammar definition.
func buildLiteralV050Parser(t *testing.T) string {
	t.Helper()
	fixture := filepath.Join("testdata", "panel-v050")
	var manifest struct {
		Commit     string
		ArchiveSHA string `json:"archive_sha256"`
		Files      []struct{ Path, SHA256 string }
	}
	if err := json.Unmarshal(folderRead(t, filepath.Join(fixture, "manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Commit != "4be663726eb2cc6fa9779b8f55b152f76c3a2c00" {
		t.Fatal("wrong frozen release")
	}
	archive := folderRead(t, filepath.Join(fixture, "source.tar.gz"))
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != manifest.ArchiveSHA {
		t.Fatal("archive hash mismatch")
	}
	hashes := map[string]string{}
	for _, f := range manifest.Files {
		hashes[f.Path] = f.SHA256
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	dir := t.TempDir()
	tr := tar.NewReader(gz)
	write := func(path string, data []byte) {
		t.Helper()
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg || !filepath.IsLocal(filepath.FromSlash(h.Name)) {
			t.Fatal("invalid archive path")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hashes[h.Name] != hex.EncodeToString(sum[:]) {
			t.Fatalf("source hash mismatch: %s", h.Name)
		}
		delete(hashes, h.Name)
		if strings.HasPrefix(h.Name, "wsxmlcmd/") {
			write(filepath.Join("internal", h.Name), data)
		}
		if h.Name == "wswhiteboard/commands.go" {
			write("commands.go", bytes.Replace(data, []byte("package wswhiteboard"), []byte("package main"), 1))
		}
	}
	if len(hashes) != 0 {
		t.Fatal("missing frozen source")
	}
	write("go.mod", []byte("module github.com/kang-sw/devenv\n\ngo 1.23\n"))
	write("internal/wsxmlcmd/complete.go", []byte(`package wsxmlcmd
func ParseComplete(p *Problems,g Grammar,s string)([]Command,bool){
 r:=&parser{tokenizer:tokenizer{s:s,p:p},g:g}
 for !r.fatal {
  r.skipSpace();if r.i>=len(r.s){break}
  if r.s[r.i]!='<'{r.skipText("outside a command");continue}
  if r.strayClosing(){continue}
  tag,ok:=r.readOpen();if !ok{break}
  if g.isGroup(tag.name){r.group(tag);continue}
  if !r.command(tag,nil){break}
 }
 return r.out,!r.fatal
}
`))
	write("main.go", []byte(literalV050Driver))
	binary := filepath.Join(dir, "parser")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "-C", dir, "build", "-p=1", "-mod=mod", "-o", binary, ".")
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("frozen parser build: %v\n%s", err, out)
	}
	return binary
}

const literalV050Driver = `package main
import("encoding/json";"os";"github.com/kang-sw/devenv/internal/wsxmlcmd")
type snapshot struct{Commands []map[string]any;Problems int;Complete bool}
func main(){var inputs []string;if err:=json.NewDecoder(os.Stdin).Decode(&inputs);err!=nil{panic(err)};out:=make([]snapshot,0,len(inputs));for _,s:=range inputs{p:=&problems{};cmds,complete:=wsxmlcmd.ParseComplete(p,grammar,s);var normalized []map[string]any;for _,c:=range cmds{raw,_:=json.Marshal(c);var m map[string]any;json.Unmarshal(raw,&m);m["Children"]=nil;normalized=append(normalized,m)};out=append(out,snapshot{normalized,len(p.List),complete})};json.NewEncoder(os.Stdout).Encode(out)}
`

func oldLiteralSnapshots(t *testing.T, binary string, inputs []string) []literalSnapshot {
	t.Helper()
	raw, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("frozen parser: %v\n%s", err, out)
	}
	var result []literalSnapshot
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != len(inputs) {
		t.Fatal("incomplete frozen result")
	}
	return result
}

func literalDifferentialCorpus() []string {
	table := literalBoundaryRepros()
	names := make([]string, 0, len(table))
	for k := range table {
		names = append(names, k)
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		out = append(out, table[name])
	}
	for _, tc := range literalAmbiguityTransitions() {
		out = append(out, tc.input)
	}
	out = append(out, literalRawSpanOracleFixture)
	out = append(out, literalWrapperContextInputs()...)
	out = append(out, literalPanelOriginInputs()...)
	out = append(out, literalR5Crossings()...)
	out = append(out, literalWrapperInlineCounterexample)
	for _, body := range literalWrapperPositiveBodies() {
		out = append(out, "<wb:topic name=\"t\"><wb:panel name=\"p\" recap=\"r\">\n"+body+"\n</wb:panel></wb:topic>")
	}
	// New wrapper syntax is outside I1's release grammar, but belongs to I3.
	// Raw operands may contain their parent wrapper's close as plain text.
	for _, field := range []string{"old", "new"} {
		for _, mark := range []string{"`", "``", "```", "~~~x\n"} {
			for _, tail := range []string{" y ` z", " y `` z", "\n~~~\n", "<![CDATA[`</wb:change>]]>"} {
				for _, text := range []string{"", " stray ` text "} {
					change := func(body string) string {
						if field == "old" {
							return "<wb:change><wb:old>" + body + "</wb:old><wb:new>n</wb:new></wb:change>"
						}
						return "<wb:change><wb:old>o</wb:old><wb:new>" + body + "</wb:new></wb:change>"
					}
					out = append(out, `<wb:topic name="t"><wb:panel name="p">`+change("a "+mark)+text+change("x </wb:change>"+tail)+`</wb:panel></wb:topic>`)
				}
			}
		}
	}
	out = append(out,
		"<wb:topic name=\"ok\"><wb:log>keep</wb:log></wb:topic><wb:topic name=\"bad\">junk<wb:panel name=\"work\"><wb:change><wb:old>a `</wb:old><wb:new>b `</wb:new></wb:change></wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"work\"><wb:change><wb:old>old</wb:old><wb:new>new</wb:new></wb:change></wb:panel><wb:log>later `</wb:log></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">Writing ` <wb:panel id=\"p2\"> is how</wb:panel><wb:log>done ` ok</wb:log><wb:panel recap=\"s\">two</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">a `</wb:panel><wb:add class=\"technical\"><wb:title>literal </wb:add> `</wb:title></wb:add><wb:panel recap=\"s\">two</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:log>close with `<![CDATA[</wb:log>]]>` tag</wb:log></wb:topic>",
	)
	// Raw elements hide a close from code the way CDATA does (outside I1).
	out = append(out,
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">`<wb:raw></wb:panel></wb:raw>` <wb:raw>`</wb:raw> tail `x`</wb:panel><wb:log>a `<wb:raw></wb:log>`</wb:raw> b</wb:log></wb:topic>",
		"<wb:topic name=\"t\"><wb:add class=\"technical\"><wb:title><wb:raw>`</wb:title>`</wb:raw></wb:title><wb:why>\n<wb:raw>\n``\n</wb:raw>\n</wb:why></wb:add></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">```\n<wb:raw></wb:panel>\n```\n</wb:raw>\n```\nplain</wb:panel></wb:topic>",
	)
	// Prior-round variants use the same enclosing-close trap in other leaves.
	out = append(out,
		"<wb:topic name=\"t\"><wb:body>Writing ` <wb:body> is how</wb:body><wb:log>done ` ok</wb:log><wb:panel recap=\"s\">two</wb:panel></wb:topic>",
		"<wb:answer id=\"1\" value=\"ok\">Writing ` <wb:answer> is how</wb:answer><wb:topic name=\"t\"><wb:log>done ` ok</wb:log><wb:panel recap=\"s\">two</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:add class=\"technical\"><wb:title>`<![CDATA[</wb:title>]]>`</wb:title></wb:add></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">`</wb:panel>"+strings.Repeat("<a?", 33300)+"`</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">"+strings.Repeat("~a", 1000)+"</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel recap=\"r\">```\n"+strings.Repeat("```x\n", 400)+"</wb:panel></wb:topic>",
	)
	for _, field := range []string{"why", "recommend", "link"} {
		for _, next := range []string{"<wb:log>c `d</wb:log>", "<wb:panel recap=\"c `\">end</wb:panel>"} {
			out = append(out, `<wb:topic name="t"><wb:add class="technical"><wb:title>x</wb:title><wb:`+field+">a `b</wb:"+field+`></wb:add>`+next+`</wb:topic>`)
		}
	}
	out = append(out, "<wb:topic name=\"t\"><wb:edit><wb:old>a</wb:old><wb:new>c `d</wb:new></wb:edit><wb:log>e `f</wb:log></wb:topic>")
	for _, mark := range []string{"`", "``", "```xml\n", "~~~xml\n"} {
		close := mark
		if strings.HasSuffix(mark, "\n") {
			close = "\n" + strings.TrimSuffix(mark, "xml\n") + "\n"
		}
		for _, nested := range []string{`</wb:panel><wb:panel recap="s">two</wb:panel>`, `</wb:panel><wb:add class="technical"><wb:title>x</wb:title><wb:why>why</wb:why></wb:add>`, `<![CDATA[</wb:panel> ` + "`" + `]]>`} {
			out = append(out, `<wb:topic name="t"><wb:panel recap="r">`+mark+nested+close+`</wb:panel></wb:topic>`)
		}
	}
	// Seeded token corpus: raw mixtures, structured multi-body commands, fields,
	// scopes and root commands. Some are invalid; I1 applies only to old successes.
	tokens := []string{"`", "``", "```", "```x", "~~~", "\n", "  ", "</wb:panel>", `<wb:panel recap="s">`, `<wb:panel name="named" recap="r">`, "<wb:log>", "</wb:log>", "<wb:body>", "<wb:title>", "<wb:why>", "<wb:link>", "</wb:topic><wb:topic name=\"next\">", "<wb:answer id=\"1\" value=\"ok\">", "'", "\"", "<b title='", "<div class=\"", "<![CDATA[plain]]>", "<![CDATA[`</wb:panel>]]>", "<![CDATA[``]]>", ">", "\\", "plain words"}
	rng := rand.New(rand.NewSource(261116))
	body := func() string {
		var b strings.Builder
		n := 1 + rng.Intn(10)
		for j := 0; j < n; j++ {
			b.WriteString(tokens[rng.Intn(len(tokens))])
		}
		return b.String()
	}
	for i := 0; i < 3000; i++ {
		var b strings.Builder
		if i%5 == 0 {
			b.WriteString(body())
		} else {
			b.WriteString(`<wb:topic name="t">`)
			for j := 0; j < 1+rng.Intn(5); j++ {
				switch rng.Intn(5) {
				case 0:
					b.WriteString(`<wb:add class="technical"><wb:title>` + body() + `</wb:title><wb:why>` + body() + `</wb:why><wb:link>` + body() + `</wb:link></wb:add>`)
				case 1:
					b.WriteString(`<wb:body>` + body() + `</wb:body>`)
				case 2:
					b.WriteString(`<wb:log>` + body() + `</wb:log>`)
				default:
					b.WriteString(`<wb:panel recap="r">` + body() + `</wb:panel>`)
				}
			}
			b.WriteString(`</wb:topic>`)
			if i%7 == 0 {
				b.WriteString(`<wb:answer id="1" value="ok">` + body() + `</wb:answer>`)
			}
		}
		out = append(out, b.String())
	}
	// Additional guaranteed old-valid cases keep the success filter substantial.
	for i := 0; i < 1000; i++ {
		safe := []string{"`", "``", "```x", "~~~", "\n", "  ", "'", "\"", "<b title='", "<div class=\"", "<![CDATA[`</wb:panel>]]>", ">", "\\", "plain"}
		var b strings.Builder
		b.WriteString(`<wb:topic name="t">`)
		for j := 0; j < 3; j++ {
			b.WriteString(`<wb:panel recap="r">`)
			for k := 0; k < 5; k++ {
				b.WriteString(safe[rng.Intn(len(safe))])
			}
			b.WriteString(`</wb:panel>`)
		}
		b.WriteString(`</wb:topic>`)
		out = append(out, b.String())
	}
	return out
}

// Delta-debug failures without hiding the old-success precondition.
func minimizeLiteralInput(s string, fails func(string) bool) string {
	for width := len(s) / 2; width > 0; width /= 2 {
		for at := 0; at+width <= len(s); {
			candidate := s[:at] + s[at+width:]
			if fails(candidate) {
				s = candidate
			} else {
				at++
			}
		}
	}
	return s
}

func TestLiteralCodeV050Differential(t *testing.T) {
	start := time.Now()
	binary := buildLiteralV050Parser(t)
	corpus := literalDifferentialCorpus()
	old := oldLiteralSnapshots(t, binary, corpus)
	checked := 0
	violates := func(input string, baseline literalSnapshot) bool {
		if literalOutsideV050(input) || baseline.Problems != 0 || !baseline.Complete {
			return false
		}
		for _, ordered := range []bool{false, true} {
			p := &problems{}
			var cmds []command
			complete := true
			if ordered {
				cmds, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
			} else {
				cmds = wsxmlcmd.Parse(p, grammar, input)
			}
			if len(p.List) != 0 || !complete || !reflect.DeepEqual(canonicalCommands(cmds), baseline.Commands) {
				return true
			}
		}
		return false
	}
	for i, input := range corpus {
		if literalOutsideV050(input) || old[i].Problems != 0 || !old[i].Complete {
			continue
		}
		checked++
		if violates(input, old[i]) {
			minimal := minimizeLiteralInput(input, func(s string) bool { return violates(s, oldLiteralSnapshots(t, binary, []string{s})[0]) })
			t.Fatalf("I1 violation corpus[%d]; minimized input:\n%s", i, minimal)
		}
	}
	t.Logf("I1 corpus=%d old-valid=%d violations=0 elapsed=%s", len(corpus), checked, time.Since(start))
}

// P5 wrapper syntax and raw elements are deliberately outside the old-parser
// comparison, and so is any entity: v0.50 decoded item fields only, while
// every body decodes now. I1 pins code-delimiter behavior, not the codec.
func literalOutsideV050(input string) bool {
	return strings.Contains(input, "<wb:change") || strings.Contains(input, "<wb:raw>") || strings.Contains(input, "&")
}

type literalRawSpan struct{ start, end int }

// Source openings are already known-valid. Quotes may contain '>', so the
// opening boundary cannot be found with an unqualified strings.IndexByte.
func literalOpeningEnd(source string, at int) int {
	var quote byte
	for ; at < len(source); at++ {
		c := source[at]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		} else if c == '>' {
			return at + 1
		}
	}
	panic("missing source opening end")
}

// Normalize a candidate raw field span independently of code recognition.
// Comparing with the reader's returned Field.Body identifies its actual close,
// including repeated fields and code-quoted earlier occurrences of that close.
func literalRawFieldBody(raw string) (string, bool) {
	type segment struct {
		text            string
		verbatim, isRaw bool
	}
	var segments []segment
	for len(raw) > 0 {
		at, open, close := strings.Index(raw, "<![CDATA["), "<![CDATA[", "]]>"
		if r := strings.Index(raw, "<wb:raw>"); r >= 0 && (at < 0 || r < at) {
			at, open, close = r, "<wb:raw>", "</wb:raw>"
		}
		if at < 0 {
			segments = append(segments, segment{text: raw})
			break
		}
		segments = append(segments, segment{text: raw[:at]})
		raw = raw[at+len(open):]
		end := strings.Index(raw, close)
		if end < 0 {
			return "", false
		}
		segments = append(segments, segment{text: raw[:end], verbatim: true, isRaw: open == "<wb:raw>"})
		raw = raw[end+len(close):]
	}
	// Edge newlines are trimmed outside raw segments only.
	for i := range segments {
		if segments[i].isRaw {
			break
		}
		if segments[i].text != "" {
			if strings.HasPrefix(segments[i].text, "\r\n") {
				segments[i].text = segments[i].text[2:]
			} else {
				segments[i].text = strings.TrimPrefix(segments[i].text, "\n")
			}
			break
		}
	}
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i].isRaw {
			break
		}
		if segments[i].text != "" {
			if strings.HasSuffix(segments[i].text, "\r\n") {
				segments[i].text = strings.TrimSuffix(segments[i].text, "\r\n")
			} else {
				segments[i].text = strings.TrimSuffix(segments[i].text, "\n")
			}
			break
		}
	}
	var out strings.Builder
	for _, s := range segments {
		if !s.verbatim {
			out.WriteString(wsxmlcmd.Decode(s.text))
		} else {
			out.WriteString(s.text)
		}
	}
	return out.String(), true
}

func literalRawSpans(c command) []literalRawSpan {
	source := c.Source
	open := literalOpeningEnd(source, 0)
	if source[open-2] == '/' {
		return nil
	}
	spec := grammar.Commands[strings.TrimPrefix(c.Tag, grammar.Prefix)]
	if spec.Kind == wsxmlcmd.Leaf {
		return []literalRawSpan{{open, strings.LastIndex(source, "</"+c.Tag+">")}}
	}
	var spans []literalRawSpan
	cursor := open
	for _, f := range c.Fields {
		for cursor < len(source) && strings.ContainsRune(" \n\r\t", rune(source[cursor])) {
			cursor++
		}
		if !strings.HasPrefix(source[cursor:], "<"+f.Tag) {
			panic("field source order mismatch")
		}
		start := literalOpeningEnd(source, cursor)
		if source[start-2] == '/' {
			cursor = start
			continue
		}
		closing := "</" + f.Tag + ">"
		search := start
		for {
			at := strings.Index(source[search:], closing)
			if at < 0 {
				panic("missing field source close")
			}
			at += search
			if body, valid := literalRawFieldBody(source[start:at]); valid && body == f.Body {
				spans = append(spans, literalRawSpan{start, at})
				cursor = at + len(closing)
				break
			}
			search = at + len(closing)
		}
	}
	return spans
}

const literalRawSpanOracleFixture = "<wb:topic name=\"t\"><wb:panel recap=\"> `\">clean body</wb:panel><wb:add class=\"technical\"><wb:title>&#96;\n</wb:title><wb:why><![CDATA[`</wb:why>]]></wb:why><wb:link>plain\n</wb:link></wb:add><wb:log>later `</wb:log></wb:topic>"

func TestLiteralCodeInsertionUsesRawSourceNotDecodedBody(t *testing.T) {
	// The recap tick is not body text, the decoded title tick is not a raw
	// delimiter, and the CDATA tick IS in the raw body even though it is inert.
	// Only panel, title and link are eligible: three runs each.
	failure, n := literalInsertionFailure(literalRawSpanOracleFixture)
	if failure != "" || n != 9 {
		t.Fatalf("failure=%q mutations=%d want 9", failure, n)
	}
}

// I2r mutates only a delimiter-free RAW body, never an already-active span or
// fence. Every mutation must itself be clean; no success-only result filtering.
func literalInsertionFailure(input string) (string, int) {
	p, pPlain := &problems{}, &problems{}
	cmds, complete := wsxmlcmd.ParseOrdered(p, grammar, input)
	plain := wsxmlcmd.Parse(pPlain, grammar, input)
	if !complete || len(p.List) != 0 || len(pPlain.List) != 0 {
		return "", 0
	}
	checked, sourceAt := 0, 0
	for _, c := range cmds {
		if c.IsGroup() {
			continue
		}
		start := strings.Index(input[sourceAt:], c.Source)
		if start < 0 {
			panic("missing command source")
		}
		start += sourceAt
		sourceAt = start + len(c.Source)
		for _, span := range literalRawSpans(c) {
			if span.end < span.start {
				panic("invalid raw body span")
			}
			if strings.ContainsAny(c.Source[span.start:span.end], "`~") {
				continue
			}
			at := start + span.end
			for _, run := range []string{"`", "``", "```"} {
				mutated := input[:at] + run + input[at:]
				q, qPlain := &problems{}, &problems{}
				next, complete := wsxmlcmd.ParseOrdered(q, grammar, mutated)
				nextPlain := wsxmlcmd.Parse(qPlain, grammar, mutated)
				checked++
				want, got := canonicalCommands(cmds), canonicalCommands(next)
				bad := !complete || len(q.List) != 0 || len(qPlain.List) != 0 || len(got) != len(want) || !reflect.DeepEqual(want, canonicalCommands(plain)) || !reflect.DeepEqual(got, canonicalCommands(nextPlain))
				if !bad {
					for j, m := range want {
						if int(m["Pos"].(float64)) != c.Pos && !reflect.DeepEqual(m, got[j]) {
							bad = true
						}
					}
				}
				if bad {
					return mutated, checked
				}
			}
		}
	}
	return "", checked
}

func TestLiteralCodeDelimiterFreeInsertionMetamorphic(t *testing.T) {
	checked := 0
	corpus := literalDifferentialCorpus()
	for i, input := range corpus {
		mutated, n := literalInsertionFailure(input)
		checked += n
		if mutated != "" {
			minimal := minimizeLiteralInput(input, func(s string) bool { m, _ := literalInsertionFailure(s); return m != "" })
			minimalMutation, _ := literalInsertionFailure(minimal)
			t.Fatalf("I2r violation corpus[%d]; minimized input:\n%s\nmutated:\n%s", i, minimal, minimalMutation)
		}
	}
	if checked == 0 {
		t.Fatal("no eligible delimiter-free bodies")
	}
	t.Logf("I2r corpus=%d mutations=%d violations=0", len(corpus), checked)
}

func literalStructuralSplitFailure(input string) (bool, bool) {
	want, eligible := literalStructuralExpected(input)
	if !eligible {
		return false, false
	}
	p, q := &problems{}, &problems{}
	plain := wsxmlcmd.Parse(p, grammar, input)
	ordered, complete := wsxmlcmd.ParseOrdered(q, grammar, input)
	// D-C/D-E permit loud refusal. Check each entry point independently:
	// a refusal by one cannot hide a clean, structurally different sibling.
	badPlain := len(p.List) == 0 && !reflect.DeepEqual(want, canonicalCommands(plain))
	badOrdered := len(q.List) == 0 && (!complete || !reflect.DeepEqual(want, canonicalCommands(ordered)))
	return badPlain || badOrdered, true
}

func TestLiteralCodeCleanStructuralSplitWins(t *testing.T) {
	checked, wrappers := 0, 0
	corpus := literalDifferentialCorpus()
	for i, input := range corpus {
		bad, eligible := literalStructuralSplitFailure(input)
		if eligible {
			checked++
			if strings.Contains(input, "<wb:change") {
				wrappers++
			}
		}
		if bad {
			minimal := minimizeLiteralInput(input, func(s string) bool { bad, _ := literalStructuralSplitFailure(s); return bad })
			t.Fatalf("I3 violation corpus[%d]; minimized input:\n%s", i, minimal)
		}
	}
	if checked == 0 || wrappers == 0 {
		t.Fatal("no clean structural splits or wrapper inputs")
	}
	t.Logf("I3 corpus=%d clean-structural=%d wrappers=%d violations=0", len(corpus), checked, wrappers)
}

package mcpframe

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadLineBoundariesAndOversizeRecovery(t *testing.T) {
	t.Run("exact limit LF, CRLF, and EOF", func(t *testing.T) {
		const limit = 32
		exact := strings.Repeat("a", limit)
		input := exact + "\n" + exact + "\r\n\n" + exact
		r := bufio.NewReaderSize(strings.NewReader(input), 16)

		for i, want := range []string{exact, exact, "", exact} {
			line, tooLarge, err := ReadLine(r, limit)
			if err != nil || tooLarge || string(line) != want {
				t.Fatalf("line %d: got (%q, tooLarge=%v, err=%v), want (%q, false, nil)", i, line, tooLarge, err, want)
			}
		}
		if line, tooLarge, err := ReadLine(r, limit); err != io.EOF || tooLarge || len(line) != 0 {
			t.Fatalf("after final line: got (%q, %v, %v), want (nil, false, EOF)", line, tooLarge, err)
		}
	})

	t.Run("oversize discards remainder and preserves subsequent lines", func(t *testing.T) {
		const limit = 48
		oversize := `{"jsonrpc":"2.0","id":7,"pad":"` + strings.Repeat("x", 512) + `"}`
		next := `{"jsonrpc":"2.0","id":8,"method":"ping"}`
		r := bufio.NewReaderSize(strings.NewReader(oversize+"\n"+next+"\n"), 16)

		prefix, tooLarge, err := ReadLine(r, limit)
		if err != nil || !tooLarge {
			t.Fatalf("oversize: got (len=%d, tooLarge=%v, err=%v), want tooLarge=true, err=nil", len(prefix), tooLarge, err)
		}
		if len(prefix) != limit {
			t.Fatalf("oversize prefix len = %d, want %d", len(prefix), limit)
		}
		if gotID := string(ExtractID(prefix)); gotID != "7" {
			t.Fatalf("ExtractID(prefix) = %q, want 7", gotID)
		}

		line, tooLarge, err := ReadLine(r, limit)
		if err != nil || tooLarge || string(line) != next {
			t.Fatalf("next line: got (%q, tooLarge=%v, err=%v), want (%q, false, nil)", line, tooLarge, err, next)
		}
		if _, _, err := ReadLine(r, limit); err != io.EOF {
			t.Fatalf("after next line: err = %v, want EOF", err)
		}
	})

	t.Run("oversize final line without trailing newline", func(t *testing.T) {
		const limit = 32
		oversize := `{"id":"e1","pad":"` + strings.Repeat("z", 128) + `"}`
		r := bufio.NewReaderSize(strings.NewReader(oversize), 16)

		prefix, tooLarge, err := ReadLine(r, limit)
		if err != nil || !tooLarge {
			t.Fatalf("got (len=%d, tooLarge=%v, err=%v), want tooLarge=true, err=nil", len(prefix), tooLarge, err)
		}
		if gotID := string(ExtractID(prefix)); gotID != `"e1"` {
			t.Fatalf("ExtractID = %q, want %q", gotID, `"e1"`)
		}
		if _, _, err := ReadLine(r, limit); err != io.EOF {
			t.Fatalf("second read err = %v, want EOF", err)
		}
	})

	t.Run("fast path oversize inside single buffer", func(t *testing.T) {
		const limit = 20
		r := bufio.NewReaderSize(strings.NewReader(`{"id":9,"pad":"0123456789"}`+"\n"+`{"id":10}`+"\n"), 1024)
		prefix, tooLarge, err := ReadLine(r, limit)
		if err != nil || !tooLarge || len(prefix) != limit {
			t.Fatalf("fast path oversize = (%q, %v, %v)", prefix, tooLarge, err)
		}
		if gotID := string(ExtractID(prefix)); gotID != "9" {
			t.Fatalf("ExtractID = %q, want 9", gotID)
		}
		line, tooLarge, err := ReadLine(r, limit)
		if err != nil || tooLarge || string(line) != `{"id":10}` {
			t.Fatalf("following line = (%q, %v, %v)", line, tooLarge, err)
		}
	})

	t.Run("propagates non-EOF read errors", func(t *testing.T) {
		wantErr := errors.New("boom")
		r := bufio.NewReaderSize(&failAfterReader{data: []byte("partial"), err: wantErr}, 16)
		if _, _, err := ReadLine(r, 64); !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want %v", err, wantErr)
		}
	})
}

type failAfterReader struct {
	data []byte
	err  error
}

func (f *failAfterReader) Read(p []byte) (int, error) {
	if len(f.data) > 0 {
		n := copy(p, f.data)
		f.data = f.data[n:]
		return n, nil
	}
	return 0, f.err
}

func TestExtractID(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "numeric id before truncated payload", prefix: `{"jsonrpc":"2.0","id":42,"method":"tools/call","params":{"x":"aaa`, want: `42`},
		{name: "negative and fractional id", prefix: `{"id":-3.5,"x":"aaa`, want: `-3.5`},
		{name: "string id with escapes", prefix: `{"id":"req-\"1\"\n","params":"aaa`, want: `"req-\"1\"\n"`},
		{name: "empty string id", prefix: `{"id":"","params":"aaa`, want: `""`},
		{name: "null id", prefix: `{"id":null,"params":"aaa`, want: `null`},
		{name: "escaped id key", prefix: `{"\u0069d":11,"params":"aaa`, want: `11`},
		{name: "id after sibling object with nested id", prefix: `{"params":{"id":999,"nested":{"text":"} \"id\":888"}},"id":42,"tail":"aaa`, want: `42`},
		{name: "nested id only when top-level id truncated away", prefix: `{"params":{"id":999,"big":"aaa`, want: ``},
		{name: "id inside string value ignored", prefix: `{"method":"\"id\":999","params":"aaa`, want: ``},
		{name: "id truncated mid-number at end of prefix", prefix: `{"jsonrpc":"2.0","id":1234`, want: ``},
		{name: "id truncated mid-string at end of prefix", prefix: `{"jsonrpc":"2.0","id":"req-1`, want: ``},
		{name: "id invalid JSON number", prefix: `{"id":01,"x":1}`, want: ``},
		{name: "id invalid object type", prefix: `{"id":{"a":1},"x":1}`, want: ``},
		{name: "id invalid boolean type", prefix: `{"id":true,"x":1}`, want: ``},
		{name: "batch array rejected", prefix: `[{"jsonrpc":"2.0","id":1}]`, want: ``},
		{name: "non-JSON garbage", prefix: `not json`, want: ``},
		{name: "oversize id string capped", prefix: `{"id":"` + strings.Repeat("a", maxExtractedIDBytes+10) + `","x":1}`, want: ``},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := []byte(tc.prefix)
			got := ExtractID(buf)
			if string(got) != tc.want {
				t.Fatalf("ExtractID(%q) = %q, want %q", tc.prefix, got, tc.want)
			}
			// Returned slice must not alias the caller's prefix buffer.
			if len(got) > 0 && len(buf) > 0 {
				orig := string(got)
				for i := range buf {
					buf[i] = 'Z'
				}
				if string(got) != orig {
					t.Fatalf("ExtractID result aliased prefix buffer: %q != %q", got, orig)
				}
			}
		})
	}
}

func TestTooLargeMessage(t *testing.T) {
	if got := TooLargeMessage(MaxRequestBytes); got != "message exceeds 67108864 bytes" {
		t.Fatalf("TooLargeMessage(MaxRequestBytes) = %q", got)
	}
	if !bytes.Equal([]byte(TooLargeMessage(1024)), []byte("message exceeds 1024 bytes")) {
		t.Fatalf("TooLargeMessage(1024) = %q", TooLargeMessage(1024))
	}
}

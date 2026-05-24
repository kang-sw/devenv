package textreader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailReadAndGrep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\ntwo regex-42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tail, err := Tail(path, 2)
	if err != nil || tail != "three\ntwo regex-42\n" {
		t.Fatalf("Tail = %q, %v", tail, err)
	}
	read, err := Read(path, 4, 3)
	if err != nil || read.Text != "two" || read.NextOffset != 7 || read.EOF {
		t.Fatalf("Read = %#v, %v", read, err)
	}
	lit, err := Grep([]string{path}, "regex-42", 1, 0, 10, false)
	if err != nil || len(lit.Matches) != 1 || lit.Matches[0].Before[0] != "three" {
		t.Fatalf("literal Grep = %#v, %v", lit, err)
	}
	re, err := Grep([]string{path}, `regex-\d+`, 0, 0, 10, true)
	if err != nil || len(re.Matches) != 1 || !strings.Contains(re.Matches[0].Text, "regex-42") {
		t.Fatalf("regex Grep = %#v, %v", re, err)
	}
	if _, err := Grep([]string{path}, `(`, 0, 0, 10, true); err == nil {
		t.Fatal("invalid regex returned nil error")
	}
}

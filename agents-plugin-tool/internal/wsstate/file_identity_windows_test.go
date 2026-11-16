package wsstate

import (
	"os"
	"syscall"
	"testing"
	"time"
)

type identityTestInfo struct {
	size int64
	mode os.FileMode
	mod  time.Time
	sys  any
}

func (i identityTestInfo) Name() string       { return "test" }
func (i identityTestInfo) Size() int64        { return i.size }
func (i identityTestInfo) Mode() os.FileMode  { return i.mode }
func (i identityTestInfo) ModTime() time.Time { return i.mod }
func (i identityTestInfo) IsDir() bool        { return i.mode.IsDir() }
func (i identityTestInfo) Sys() any           { return i.sys }

func TestSameFileMetadataWindows(t *testing.T) {
	created := syscall.Filetime{LowDateTime: 123, HighDateTime: 456}
	base := identityTestInfo{size: 42, mode: 0600, mod: time.Unix(100, 0), sys: &syscall.Win32FileAttributeData{CreationTime: created}}
	for _, tc := range []struct {
		name string
		edit func(*identityTestInfo)
		want bool
	}{
		{"equal", func(*identityTestInfo) {}, true},
		{"creation", func(i *identityTestInfo) {
			i.sys = &syscall.Win32FileAttributeData{CreationTime: syscall.Filetime{LowDateTime: 124, HighDateTime: 456}}
		}, false},
		{"creation high", func(i *identityTestInfo) {
			i.sys = &syscall.Win32FileAttributeData{CreationTime: syscall.Filetime{LowDateTime: 123, HighDateTime: 457}}
		}, false},
		{"size", func(i *identityTestInfo) { i.size++ }, false},
		{"mtime", func(i *identityTestInfo) { i.mod = i.mod.Add(time.Nanosecond) }, false},
		{"mode", func(i *identityTestInfo) { i.mode = 0644 }, false},
		{"directory", func(i *identityTestInfo) { i.mode = os.ModeDir | 0600 }, false},
		{"missing sys", func(i *identityTestInfo) { i.sys = nil }, false},
		{"wrong sys", func(i *identityTestInfo) { i.sys = "unexpected" }, false},
		{"nil sys pointer", func(i *identityTestInfo) { i.sys = (*syscall.Win32FileAttributeData)(nil) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			tc.edit(&other)
			if got := SameFileMetadata(base, other); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if got := SameFileMetadata(other, base); got != tc.want {
				t.Fatalf("reversed: got %v, want %v", got, tc.want)
			}
		})
	}
	if SameFileMetadata(nil, base) || SameFileMetadata(base, nil) {
		t.Fatal("nil metadata matched")
	}
}

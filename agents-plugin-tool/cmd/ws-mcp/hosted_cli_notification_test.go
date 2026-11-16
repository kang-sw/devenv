package main

import (
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kang-sw/devenv/internal/mcp"
)

func TestHostedCLIWiresWhiteboardChanged(t *testing.T) {
	root := t.TempDir()
	nudges := mcp.NewWhiteboardNudges(filepath.Join(root, "dashboard"), nil, io.Discard)
	defer nudges.Close()
	for _, tc := range []struct {
		name string
		host *proxyHost
		want bool
	}{
		{"hosted", &proxyHost{nudges: nudges}, true},
		{"no scheduler", &proxyHost{}, false},
		{"manual", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &hostedCLI{host: tc.host, stderr: io.Discard}
			server := c.server(root)
			defer server.Close()
			present := !reflect.ValueOf(server).Elem().FieldByName("whiteboardChanged").IsNil()
			if present != tc.want {
				t.Fatalf("callback present = %v, want %v", present, tc.want)
			}
		})
	}
}

package wsdashboard

import (
	"fmt"
	"io"

	"github.com/kang-sw/devenv/internal/wsdevassets"
)

// DevSourceEnv names the environment variable that turns dev assets on for a
// daemon: the agents-plugin-tool source root whose dashboard and whiteboard
// assets the daemon serves from disk, re-read on every request, instead of its
// embedded copies. It is read by `ws-mcp dashboard` only, so the MCP server's
// own whiteboard pages keep the embedded assets. Unset is off.
const DevSourceEnv = "WS_MCP_DASHBOARD_DEV_SOURCE"

// EnableDevAssets turns dev assets on for this process from source, the
// source root; an empty source leaves them off. It fails when the root or its
// asset directories do not exist, rather than serving the embedded copy while
// the caller believes it edits the served one.
func EnableDevAssets(source string) (*wsdevassets.Root, error) {
	if source == "" {
		return nil, nil
	}
	root, err := wsdevassets.Open(source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", DevSourceEnv, err)
	}
	SetDevAssets(root)
	return root, nil
}

// LogDevAssets writes the one-line notice that the daemon serves dev assets.
func LogDevAssets(w io.Writer, root *wsdevassets.Root) {
	if root != nil && w != nil {
		fmt.Fprintf(w, "dashboard: dev assets on: serving dashboard and whiteboard assets from %s (re-read per request; mermaid stays embedded)\n", root.Dir())
	}
}

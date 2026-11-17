package mcp

import (
	"slices"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Image dependencies are derived without rendering deferred frames/templates.
// The board memo retains this list until the authoritative folder read changes;
// replacement daemons derive the same watches even before a history request.
func servedImagePaths(record sessionRecord) []string {
	paths := wswhiteboard.FileImagePaths(record.Whiteboard)
	for i, path := range paths {
		paths[i] = docImagePath(record.Root, path)
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The opt-in old source fixture creates a board, loses its page/stamp, and
// recreates them with the old renderer. Reopening with this version must keep
// both its decoded state and every authoritative byte.
func TestWhiteboardReopensOldRecreatedPages(t *testing.T) {
	dir, state := os.Getenv("WS_WHITEBOARD_COMPAT_BOARD"), os.Getenv("WS_WHITEBOARD_COMPAT_STATE")
	if dir == "" || state == "" {
		t.Skip("set old-source compatibility fixture paths")
	}
	before := whiteboardTreeForTest(t, dir)
	board, err := (wswhiteboard.FolderStore{Dir: dir}).Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(board)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, bytes.TrimSpace(readFileForTest(t, state))) {
		t.Fatal("new reader changed old board state")
	}
	assertWhiteboardTreeForTest(t, dir, before)
}

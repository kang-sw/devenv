package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func assertWhiteboardPublicationError(t *testing.T, resp, cause string) {
	t.Helper()
	if !isToolError(resp) {
		t.Fatalf("publication failure must return an error: %s", resp)
	}
	text := toolText(t, resp)
	for _, want := range []string{"board committed; ", cause, `retry whiteboard.update with commands: "" (render-only)`, "do not resend accepted commands"} {
		if !strings.Contains(text, want) {
			t.Fatalf("publication error missing %q: %s", want, text)
		}
	}
}

func TestWhiteboardPublicationDirectoryAndMermaidFailuresRepairWithoutDuplicateEvents(t *testing.T) {
	for _, failure := range []string{"directory", "mermaid_asset"} {
		t.Run(failure, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			f.update(t, ``)
			beforeStamp := f.stamp(t)
			assetName, assetData := wswhiteboard.MermaidAsset()
			injected := errors.New("injected " + failure + " publication failure")
			origMkdir, origRename := whiteboardMkdirAll, whiteboardRename
			t.Cleanup(func() {
				whiteboardMkdirAll, whiteboardRename = origMkdir, origRename
			})
			switch failure {
			case "directory":
				whiteboardMkdirAll = func(path string, mode os.FileMode) error {
					if path == f.pageDir() {
						return injected
					}
					return origMkdir(path, mode)
				}
			case "mermaid_asset":
				whiteboardRename = func(from, to string) error {
					if to == filepath.Join(f.pageDir(), assetName) {
						return injected
					}
					return origRename(from, to)
				}
			}
			commands := "<wb:topic name=\"publication\"><wb:add class=\"policy\"><wb:title>committed-marker</wb:title><wb:why>\n```mermaid\ngraph LR\nA-->B\n```\n</wb:why></wb:add></wb:topic>"
			resp := f.call(t, "whiteboard.update", map[string]any{"commands": commands})
			assertWhiteboardPublicationError(t, resp, injected.Error())
			if failure == "directory" && !strings.Contains(toolText(t, resp), "create output directory "+f.pageDir()) {
				t.Fatalf("directory failure lost its output path: %s", resp)
			}
			if failure == "mermaid_asset" && !strings.Contains(toolText(t, resp), "write "+assetName) {
				t.Fatalf("asset failure lost its asset name: %s", resp)
			}
			committed, err := newSessionStore().readWhiteboard(f.key)
			if err != nil {
				t.Fatal(err)
			}
			board := committed.Whiteboard
			if board == nil || board.Counter != 1 || len(board.Items) != 1 || board.Items[0].Title != "committed-marker" || board.Seq != 2 || board.EntrySeq != 1 || len(board.Topics) != 1 || len(board.Topics[0].Entries) != 1 {
				t.Fatalf("publication failure lost or duplicated accepted state: %+v", board)
			}
			event := board.Topics[0].Entries[0]
			if event.Kind != wswhiteboard.EntrySystem || event.ItemID != board.Items[0].ID || !strings.Contains(event.Text, "committed-marker") {
				t.Fatalf("accepted item lacks its single durable event: %+v", event)
			}
			if got := f.stamp(t); got != beforeStamp {
				t.Fatalf("failed publication advanced the stamp: %q -> %q", beforeStamp, got)
			}
			logs, err := filepath.Glob(filepath.Join(f.pageDir(), "*", "log.jsonl"))
			if err != nil || len(logs) != 1 {
				t.Fatalf("find committed log: %v, %v", logs, err)
			}
			logBefore := string(readFileForTest(t, logs[0]))

			// Repair only derived output: never replay the already accepted add.
			whiteboardMkdirAll, whiteboardRename = origMkdir, origRename
			text := f.update(t, ``)
			repaired, err := newSessionStore().readWhiteboard(f.key)
			if err != nil {
				t.Fatal(err)
			}
			after := repaired.Whiteboard
			if after == nil || after.Counter != board.Counter || after.EntrySeq != board.EntrySeq || len(after.Topics) != len(board.Topics) || !reflect.DeepEqual(after.Items, board.Items) || !reflect.DeepEqual(after.Topics[0].Entries, board.Topics[0].Entries) || string(readFileForTest(t, logs[0])) != logBefore {
				t.Fatalf("render-only repair changed accepted items/events: %+v", after)
			}
			if f.stamp(t) != "wbStamp(\"3\");\n" || !strings.Contains(f.fullPage(t), "committed-marker") {
				t.Fatal("render-only retry did not repair the page and stamp")
			}
			for _, page := range []string{f.pagePath(), filepath.Join(f.root, resultLine(t, text, "embed: "))} {
				if got := readFileForTest(t, filepath.Join(filepath.Dir(page), assetName)); len(got) != len(assetData) {
					t.Fatalf("repaired page %s lacks its complete Mermaid asset", page)
				}
				if !strings.Contains(string(readFileForTest(t, page)), `<script src="`+assetName+`"></script>`) {
					t.Fatalf("repaired page %s does not load its Mermaid asset", page)
				}
			}
		})
	}
}

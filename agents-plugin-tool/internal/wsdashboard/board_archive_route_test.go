package wsdashboard

import (
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// BoardArchive serves a fixed zip for every session the fake has a board for.
func (p *pageFake) BoardArchive(reg Registration) ([]byte, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.boards[reg.ReplyID] == nil {
		return nil, false, nil
	}
	return []byte("PK-zip-of-" + reg.ReplyID), true, nil
}

func TestBoardArchiveRouteDownloadsTheBoard(t *testing.T) {
	f := startPageFixture(t)
	f.src.mu.Lock()
	f.src.boards["boardsession"] = closedTopicsBoard(1)
	f.src.mu.Unlock()

	resp, err := testClient().Get(f.base + "/s/boardsession/board.zip")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "PK-zip-of-boardsession" || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("GET board.zip = %d %q %q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	want := regexp.MustCompile(`^attachment; filename=whiteboard_` + regexp.QuoteMeta(filepath.Base(f.project)) + `_boardsession_\d{6}_\d{4}\.zip$`)
	if got := resp.Header.Get("Content-Disposition"); !want.MatchString(got) {
		t.Fatalf("Content-Disposition %q", got)
	}

	// A session without a board, and an unknown one, answer not found.
	for _, rid := range []string{"livesession", "nosuchsession"} {
		if code, _ := get(t, f.base+"/s/"+rid+"/board.zip"); code != http.StatusNotFound {
			t.Errorf("GET /s/%s/board.zip = %d, want 404", rid, code)
		}
	}
}

func TestBoardArchiveName(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 5, 0, 0, time.Local)
	worktree := Session{Registration: Registration{ReplyID: "rid", Project: "/w/261116-1227", GitCommonDir: "/r/devenv/.git"}, Label: `리뷰 a/b: "c"`}
	if got, want := boardArchiveName(worktree, "impl/track/x", now), `whiteboard_devenv_impl-track-x_리뷰 a-b- -c-_261118_0905.zip`; got != want {
		t.Errorf("worktree name %q, want %q", got, want)
	}
	plain := Session{Registration: Registration{ReplyID: "rid", Project: "/w/notes"}}
	if got, want := boardArchiveName(plain, "", now), "whiteboard_notes_rid_261118_0905.zip"; got != want {
		t.Errorf("plain folder name %q, want %q", got, want)
	}
}

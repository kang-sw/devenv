package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestDashboardTicketGraphReadsRecordRoot(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	original := ticketMapJSON
	t.Cleanup(func() { ticketMapJSON = original })
	var roots []string
	want := []byte(`{"meta":{"v":1}}`)
	ticketMapJSON = func(root string) ([]byte, error) {
		roots = append(roots, root)
		return want, nil
	}
	got, ok, err := src.ReadTicketGraph(reg)
	if !ok || err != nil || !bytes.Equal(got, want) || len(roots) != 1 || roots[0] != f.root {
		t.Fatalf("read = %s %v %v roots=%v, want %s from %s", got, ok, err, roots, want, f.root)
	}
	ticketMapJSON = func(string) ([]byte, error) { return nil, errors.New("boom") }
	if _, ok, err := src.ReadTicketGraph(reg); !ok || err == nil {
		t.Fatalf("error read = ok %v err %v, want ok with the error", ok, err)
	}
}

func TestDashboardTicketGraphMissingBoardIsEmpty(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	original := ticketMapJSON
	t.Cleanup(func() { ticketMapJSON = original })
	ticketMapJSON = func(string) ([]byte, error) {
		return nil, fmt.Errorf("tickets directory not found: %w", os.ErrNotExist)
	}
	want, err := wsdoc.EmptyTicketMapJSON()
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := src.ReadTicketGraph(reg)
	if !ok || err != nil || !bytes.Equal(got, want) {
		t.Fatalf("missing board = %s %v %v, want %s", got, ok, err, want)
	}
	for _, s := range []string{`"open":0`, `"clusters":[]`, `"clusterKeywords":{}`, `"nodes":[]`, `"v":1`} {
		if !bytes.Contains(got, []byte(s)) {
			t.Fatalf("empty payload lacks %s: %s", s, got)
		}
	}
}

func TestDashboardTicketGraphUnknownSession(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	original := ticketMapJSON
	t.Cleanup(func() { ticketMapJSON = original })
	ticketMapJSON = func(string) ([]byte, error) {
		t.Fatal("unknown session read the board")
		return nil, nil
	}
	if _, ok, err := src.ReadTicketGraph(wsdashboard.Registration{ReplyID: "nosuchsession", Project: f.root}); ok || err != nil {
		t.Fatalf("unknown session = ok %v err %v", ok, err)
	}
}

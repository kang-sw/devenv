package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in, single-attempt new-headless Chromium check. Headless shell has no
// PDF viewer and must never be used as evidence that a PDF rendered.
func TestFSPDFBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for full-Chromium PDF verification")
	}
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	writeFile(t, filepath.Join(f.project, "doc.pdf"), pdfBrowserDocument())
	rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/raw?path=doc.pdf", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("PDF route: %d", rec.Code)
	}
	input, err := json.Marshal(map[string]any{
		"pdf": rec.Body.Bytes(), "headers": rec.Header(),
		"preview": mustAsset(t, "preview.js"), "css": mustAsset(t, "dashboard.css"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/preview-pdf.browser.cjs")
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("full-Chromium PDF check (one attempt): %v\n%s", err, output)
	}
	if strings.HasPrefix(string(output), "SKIP:") {
		t.Skip(strings.TrimSpace(string(output)))
	}
	t.Log(strings.TrimSpace(string(output)))
}

func pdfBrowserDocument() string {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	stream := "BT /F1 18 Tf 40 120 Td (Phase three PDF preview) Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.String()
}

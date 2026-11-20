// tracker_attachment_routes_test.go tests tracker attachment upload and image reads.
package wsdashboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestTrackerAttachmentUpload(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	const upload = "/tracker/stores/team/attachments"

	rec := callRaw(d, http.MethodPost, upload+"?name=shot.png", tinyPNG, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	var got TrackerAttachment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(tinyPNG)
	want := TrackerAttachment{OID: hex.EncodeToString(sum[:]), Name: "shot.png", Size: int64(len(tinyPNG)), Mime: "image/png"}
	if got != want {
		t.Fatalf("upload answered %+v, want %+v", got, want)
	}

	if rec := callRaw(d, http.MethodPost, upload+"?name=a.txt", []byte("just text"), nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("non-image upload = %d %s, want 415", rec.Code, rec.Body)
	}
	big := append(append([]byte(nil), tinyPNG...), make([]byte, fake.maxBytes)...)
	if rec := callRaw(d, http.MethodPost, upload+"?name=big.png", big, nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize upload = %d %s, want 413", rec.Code, rec.Body)
	}
	before := len(fake.recorded())
	if rec := callRaw(d, http.MethodPost, upload, tinyPNG, map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin upload = %d %s, want 403", rec.Code, rec.Body)
	}
	if got := fake.recorded(); len(got) != before {
		t.Errorf("a cross-origin upload reached the host: %v", got[before:])
	}
	if rec := callRaw(d, http.MethodGet, upload, nil, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET upload path = %d, want 405", rec.Code)
	}
	if rec := callRaw(d, http.MethodPost, "/tracker/stores/other/attachments", tinyPNG, nil); rec.Code != http.StatusNotFound {
		t.Errorf("upload to an unknown store = %d, want 404", rec.Code)
	}
}

func TestTrackerAttachmentRead(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	rec := callRaw(d, http.MethodPost, "/tracker/stores/team/attachments?name=shot.png", tinyPNG, nil)
	var att TrackerAttachment
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &att) != nil {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	read := "/tracker/stores/team/attachments/" + att.OID

	rec = callRaw(d, http.MethodGet, read, nil, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), tinyPNG) {
		t.Fatalf("read = %d %q", rec.Code, rec.Body)
	}
	h := rec.Header()
	for k, want := range map[string]string{
		"Content-Type":                 "image/png",
		"X-Content-Type-Options":       "nosniff",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Content-Security-Policy":      imageContentSecurityPolicy,
		"Content-Length":               strconv.Itoa(len(tinyPNG)),
	} {
		if got := h.Get(k); got != want {
			t.Errorf("read %s = %q, want %q", k, got, want)
		}
	}
	if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": "same-origin"}); rec.Code != http.StatusOK {
		t.Errorf("same-origin read = %d, want 200", rec.Code)
	}

	for name, path := range map[string]string{
		"unknown oid":   "/tracker/stores/team/attachments/" + strings.Repeat("0", 64),
		"malformed oid": "/tracker/stores/team/attachments/abc",
		"uppercase oid": "/tracker/stores/team/attachments/" + strings.ToUpper(att.OID),
		"unknown store": "/tracker/stores/other/attachments/" + att.OID,
	} {
		if rec := callRaw(d, http.MethodGet, path, nil, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s read = %d, want 404", name, rec.Code)
		}
	}
	if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusNotFound {
		t.Errorf("cross-site read = %d, want 404", rec.Code)
	}
	if rec := callRaw(d, http.MethodPost, read, tinyPNG, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST read path = %d, want 405", rec.Code)
	}

	// Content that does not sniff as an accepted image is never served.
	sum := sha256.Sum256([]byte("<svg/>"))
	oid := hex.EncodeToString(sum[:])
	fake.mu.Lock()
	fake.blobs[oid] = []byte("<svg/>")
	fake.mu.Unlock()
	if rec := callRaw(d, http.MethodGet, "/tracker/stores/team/attachments/"+oid, nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("non-image content read = %d, want 404", rec.Code)
	}
}

func TestTrackerWikiImageRead(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	sum := sha256.Sum256(tinyPNG)
	oid := hex.EncodeToString(sum[:])
	textSum := sha256.Sum256([]byte("plain text"))
	textOID := hex.EncodeToString(textSum[:])
	fake.mu.Lock()
	fake.wikiBlobs["notes/"+oid] = tinyPNG
	fake.wikiBlobs["notes/"+textOID] = []byte("plain text")
	fake.mu.Unlock()
	read := "/tracker/stores/team/wiki/notes/attachments/" + oid

	rec := callRaw(d, http.MethodGet, read, nil, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), tinyPNG) {
		t.Fatalf("read = %d %q", rec.Code, rec.Body)
	}
	h := rec.Header()
	for k, want := range map[string]string{
		"Content-Type":                 "image/png",
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      imageContentSecurityPolicy,
		"Cache-Control":                "private, max-age=31536000, immutable",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Content-Length":               strconv.Itoa(len(tinyPNG)),
	} {
		if got := h.Get(k); got != want {
			t.Errorf("read %s = %q, want %q", k, got, want)
		}
	}
	for _, site := range []string{"none", "same-origin"} {
		if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": site}); rec.Code != http.StatusOK {
			t.Errorf("Sec-Fetch-Site %s read = %d, want 200", site, rec.Code)
		}
	}
	if rec := callRaw(d, http.MethodGet, read, nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusNotFound {
		t.Errorf("cross-site read = %d, want 404", rec.Code)
	}

	for name, path := range map[string]string{
		"non-image content": "/tracker/stores/team/wiki/notes/attachments/" + textOID,
		"unknown oid":       "/tracker/stores/team/wiki/notes/attachments/" + strings.Repeat("0", 64),
		"unknown wiki":      "/tracker/stores/team/wiki/other/attachments/" + oid,
		"uppercase wiki":    "/tracker/stores/team/wiki/Notes/attachments/" + oid,
		"leading hyphen":    "/tracker/stores/team/wiki/-notes/attachments/" + oid,
		"uppercase oid":     "/tracker/stores/team/wiki/notes/attachments/" + strings.ToUpper(oid),
		"malformed oid":     "/tracker/stores/team/wiki/notes/attachments/abc",
		"no attachments":    "/tracker/stores/team/wiki/notes/" + oid,
		"unknown store":     "/tracker/stores/other/wiki/notes/attachments/" + oid,
	} {
		if rec := callRaw(d, http.MethodGet, path, nil, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s read = %d, want 404", name, rec.Code)
		}
	}
	if rec := callRaw(d, http.MethodPost, read, tinyPNG, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST read path = %d, want 405", rec.Code)
	}
}

package wsledger

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestKeyFingerprint(t *testing.T) {
	pub := make(ed25519.PublicKey, ed25519.PublicKeySize)
	for i := range pub {
		pub[i] = byte(i)
	}
	var blob []byte
	for _, part := range [][]byte{[]byte("ssh-ed25519"), pub} {
		blob = binary.BigEndian.AppendUint32(blob, uint32(len(part)))
		blob = append(blob, part...)
	}
	b64 := base64.StdEncoding.EncodeToString(blob)
	sum := sha256.Sum256(blob)
	wantFP := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])

	norm, fp, err := KeyFingerprint("  ssh-ed25519 " + b64 + " someone@host ")
	if err != nil {
		t.Fatal(err)
	}
	if norm != "ssh-ed25519 "+b64 || fp != wantFP {
		t.Fatalf("got (%q, %q), want (%q, %q)", norm, fp, "ssh-ed25519 "+b64, wantFP)
	}
	if fp != keyFP("ssh-ed25519 "+b64) {
		t.Fatal("KeyFingerprint and keyFP disagree")
	}
	for _, bad := range []string{"", "maya-k1", "ssh-rsa " + b64, "ssh-ed25519 !!!"} {
		if _, _, err := KeyFingerprint(bad); err == nil {
			t.Errorf("KeyFingerprint(%q) accepted a malformed key", bad)
		}
	}
}

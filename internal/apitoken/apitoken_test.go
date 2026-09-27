package apitoken

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewAndHash(t *testing.T) {
	a, ha, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := New()
	if !strings.HasPrefix(a, "mlt_") || len(a) != 4+43 || a == b {
		t.Fatalf("tokens %q %q", a, b)
	}
	if !bytes.Equal(ha, Hash(a)) || len(ha) != 32 || bytes.Contains(ha, []byte(a)) {
		t.Fatal("hash")
	}
}

func TestFromHeader(t *testing.T) {
	tok, _, _ := New()
	// Shortcuts and phone keyboards add invisible whitespace: any whitespace between the
	// scheme and the key, and around them, is accepted; the key itself stays strict.
	for h, ok := range map[string]bool{"Bearer " + tok: true, "bearer " + tok: true,
		"Bearer  " + tok: true, "Bearer\u00a0" + tok: true, "Bearer\t" + tok: true,
		"Bearer " + tok + "\n": true, " Bearer " + tok + " ": true, "Bearer" + tok: false,
		"Bearer " + tok + " extra": false, "Basic " + tok: false, tok: false,
		"Bearer mlt_short": false, "Bearer other_" + tok[4:]: false, "": false} {
		if got, gotOK := FromHeader(h); gotOK != ok || (ok && got != tok) {
			t.Errorf("FromHeader(%q) = %q, %v", h, got, gotOK)
		}
	}
}

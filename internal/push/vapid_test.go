package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestVAPIDHeaderVerifies(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	v, err := ParseKey(priv, "https://matlistan.example.org")
	if err != nil {
		t.Fatal(err)
	}
	if v.PublicKey() != pub {
		t.Fatalf("public key %q, want %q", v.PublicKey(), pub)
	}
	now := time.Date(2026, 10, 4, 7, 0, 0, 0, time.UTC)
	h, err := v.Header("https://web.push.apple.com/QGuQyavXutnMH/abc", now)
	if err != nil {
		t.Fatal(err)
	}
	var tok, k string
	for _, part := range strings.Split(strings.TrimPrefix(h, "vapid "), ", ") {
		switch {
		case strings.HasPrefix(part, "t="):
			tok = part[2:]
		case strings.HasPrefix(part, "k="):
			k = part[2:]
		}
	}
	if k != pub || !strings.HasPrefix(h, "vapid t=") {
		t.Fatalf("header %q", h)
	}
	seg := strings.Split(tok, ".")
	if len(seg) != 3 {
		t.Fatalf("token %q", tok)
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	raw, _ := base64.RawURLEncoding.DecodeString(seg[1])
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://web.push.apple.com" || claims.Sub != "https://matlistan.example.org" ||
		claims.Exp != now.Add(12*time.Hour).Unix() {
		t.Fatalf("claims %+v", claims)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(seg[2])
	pubBytes, _ := base64.RawURLEncoding.DecodeString(pub)
	x, y := elliptic.Unmarshal(elliptic.P256(), pubBytes) //nolint:staticcheck // test only
	key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	sum := sha256.Sum256([]byte(seg[0] + "." + seg[1]))
	if len(sig) != 64 || !ecdsa.Verify(key, sum[:], new(big.Int).SetBytes(sig[:32]),
		new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
}

func TestParseKeyRejects(t *testing.T) {
	priv, _, _ := GenerateKey()
	for _, c := range []struct{ key, subject string }{
		{"not base64!", "https://a.example"}, {"AAAA", "https://a.example"},
		{priv, ""}, {priv, "http://a.example"}, {priv, "ftp://a.example"},
	} {
		if _, err := ParseKey(c.key, c.subject); err == nil {
			t.Errorf("ParseKey(%q, %q) accepted", c.key, c.subject)
		}
	}
	if _, err := ParseKey(" "+priv+"\n", "mailto:admin@example.org"); err != nil {
		t.Errorf("trimmed key with mailto refused: %v", err)
	}
}

// cert-manager writes PKCS#8 PEM; SEC1 PEM and the vapid-keys form give the same key.
func TestParseKeyFormats(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(k)
	sec1, _ := x509.MarshalECPrivateKey(k)
	raw, _ := k.Bytes()
	forms := map[string]string{
		"pkcs8":     string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		"sec1":      string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})),
		"base64url": base64.RawURLEncoding.EncodeToString(raw),
	}
	var want string
	for name, f := range forms {
		v, err := ParseKey(f, "https://a.example")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want == "" {
			want = v.PublicKey()
		} else if v.PublicKey() != want {
			t.Errorf("%s gives a different public key", name)
		}
	}
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsaDER, _ := x509.MarshalPKCS8PrivateKey(rsaKey)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384DER, _ := x509.MarshalPKCS8PrivateKey(p384)
	for name, bad := range map[string]string{
		"rsa":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaDER})),
		"p384": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: p384DER})),
		"cert": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
	} {
		if _, err := ParseKey(bad, "https://a.example"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

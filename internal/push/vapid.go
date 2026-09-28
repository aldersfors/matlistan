package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var _b64 = base64.RawURLEncoding

// VAPID signs RFC 8292 tokens that tell a push service the messages come from us.
type VAPID struct {
	key     *ecdsa.PrivateKey
	public  string
	subject string
}

// GenerateKey returns a new P-256 key pair as base64url: the private scalar and the
// uncompressed public point for the browser.
func GenerateKey() (private, public string, err error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	raw, err := k.Bytes()
	if err != nil {
		return "", "", err
	}
	pub, err := k.PublicKey.Bytes()
	if err != nil {
		return "", "", err
	}
	return _b64.EncodeToString(raw), _b64.EncodeToString(pub), nil
}

// ParseKey reads the private key and the contact subject, which RFC 8292 requires to be a
// mailto: or https: URL. The key is PEM (PKCS#8 as cert-manager writes it, or SEC1) or the
// base64url scalar vapid-keys prints. Errors never include key material.
func ParseKey(key, subject string) (*VAPID, error) {
	k, err := parsePrivateKey(strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(subject)
	https := err == nil && u.Scheme == "https" && u.Host != ""
	mailto := err == nil && u.Scheme == "mailto" && u.Opaque != ""
	if !https && !mailto {
		return nil, errors.New("push: VAPID subject must be a mailto: or https: URL")
	}
	pub, err := k.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	return &VAPID{key: k, public: _b64.EncodeToString(pub), subject: subject}, nil
}

var errNotP256 = errors.New("push: VAPID key is not a P-256 ECDSA private key")

func parsePrivateKey(key string) (*ecdsa.PrivateKey, error) {
	if b, _ := pem.Decode([]byte(key)); b != nil {
		var parsed any
		var err error
		switch b.Type {
		case "PRIVATE KEY":
			parsed, err = x509.ParsePKCS8PrivateKey(b.Bytes)
		case "EC PRIVATE KEY":
			parsed, err = x509.ParseECPrivateKey(b.Bytes)
		default:
			return nil, errNotP256
		}
		k, ok := parsed.(*ecdsa.PrivateKey)
		if err != nil || !ok || k.Curve != elliptic.P256() {
			return nil, errNotP256
		}
		return k, nil
	}
	raw, err := _b64.DecodeString(key)
	if err != nil {
		return nil, errNotP256
	}
	k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, errNotP256
	}
	return k, nil
}

// PublicKey is the applicationServerKey for PushManager.subscribe.
func (v *VAPID) PublicKey() string { return v.public }

// Header is the Authorization value for one push to endpoint, valid for 12 hours.
func (v *VAPID) Header(endpoint string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{"aud": u.Scheme + "://" + u.Host,
		"exp": now.Add(12 * time.Hour).Unix(), "sub": v.subject})
	if err != nil {
		return "", err
	}
	signing := _b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." +
		_b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, v.key, sum[:])
	if err != nil {
		return "", fmt.Errorf("push: sign: %w", err)
	}
	sig := make([]byte, 64) // JWS wants r and s as two fixed 32-byte halves, not ASN.1
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return "vapid t=" + signing + "." + _b64.EncodeToString(sig) + ", k=" + v.public, nil
}

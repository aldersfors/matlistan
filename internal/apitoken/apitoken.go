// Package apitoken creates and checks the keys the iOS Shortcut uses.
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
)

const (
	_prefix   = "mlt_"
	_rawBytes = 32
)

var _encodedLen = base64.RawURLEncoding.EncodedLen(_rawBytes)

// Token is a stored key without its secret.
type Token struct {
	ID         int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// New returns a fresh key and its hash. Only the hash is stored.
func New() (string, []byte, error) {
	b := make([]byte, _rawBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plain := _prefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, Hash(plain), nil
}

// Hash is the stored form of a key.
func Hash(plain string) []byte {
	h := sha256.Sum256([]byte(plain))
	return h[:]
}

// FromHeader reads "Bearer mlt_..." and checks the key's shape before any lookup.
func FromHeader(h string) (string, bool) {
	scheme, tok, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(tok, _prefix) ||
		len(tok) != len(_prefix)+_encodedLen {
		return "", false
	}
	if _, err := base64.RawURLEncoding.DecodeString(tok[len(_prefix):]); err != nil {
		return "", false
	}
	return tok, true
}

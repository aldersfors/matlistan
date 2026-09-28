// Package push sends Web Push notifications with the standard library: RFC 8291 message
// encryption, RFC 8292 VAPID, and a sender that only reaches known push services.
package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// Subscription is what a browser's PushManager.subscribe returns.
type Subscription struct {
	Endpoint     string
	P256DH, Auth []byte
}

const (
	_recordSize = 4096
	_pointLen   = 65 // an uncompressed P-256 point
)

// ValidKeys reports whether a subscription's keys have the sizes RFC 8291 needs and the
// point is on P-256.
func ValidKeys(p256dh, auth []byte) bool {
	if len(auth) != 16 || len(p256dh) != _pointLen {
		return false
	}
	_, err := ecdh.P256().NewPublicKey(p256dh)
	return err == nil
}

// Encrypt is RFC 8291 with the aes128gcm content coding of RFC 8188 and one record. Salt
// and serverKey are parameters so the RFC example can be reproduced; use Seal otherwise.
func Encrypt(sub Subscription, plaintext, salt []byte, serverKey *ecdh.PrivateKey) ([]byte,
	error) {
	if !ValidKeys(sub.P256DH, sub.Auth) || len(salt) != 16 {
		return nil, errors.New("push: invalid subscription keys or salt")
	}
	uaPub, _ := ecdh.P256().NewPublicKey(sub.P256DH)
	secret, err := serverKey.ECDH(uaPub)
	if err != nil {
		return nil, fmt.Errorf("push: ecdh: %w", err)
	}
	asPub := serverKey.PublicKey().Bytes()
	keyInfo := "WebPush: info\x00" + string(sub.P256DH) + string(asPub)
	ikm, err := hkdf.Key(sha256.New, secret, sub.Auth, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 0, 16+4+1+len(asPub))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, _recordSize)
	header = append(header, _pointLen) // keyid length: the uncompressed P-256 point
	header = append(header, asPub...)
	// The last (and only) record ends with the 0x02 padding delimiter.
	return gcm.Seal(header, nonce, append(append([]byte{}, plaintext...), 0x02), nil), nil
}

// Seal encrypts with a fresh salt and a fresh server key, as every message must.
func Seal(sub Subscription, plaintext []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return Encrypt(sub, plaintext, salt, key)
}

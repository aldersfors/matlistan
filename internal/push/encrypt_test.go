package push

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"testing"
)

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// RFC 8291 Section 5, byte for byte.
func TestEncryptMatchesRFC8291(t *testing.T) {
	serverKey, err := ecdh.P256().NewPrivateKey(b64(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	sub := Subscription{
		Endpoint: "https://push.example.net/push/JzLQ3raZJfFBR0aqvOMsLrt54w4rJUsV",
		P256DH:   b64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		Auth:     b64(t, "BTBZMqHH6r4Tts7J_aSIgg"),
	}
	got, err := Encrypt(sub, []byte("When I grow up, I want to be a watermelon"),
		b64(t, "DGv6ra1nlYgDCS1FRnbzlw"), serverKey)
	if err != nil {
		t.Fatal(err)
	}
	want := b64(t, "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN")
	if !bytes.Equal(got, want) {
		t.Fatalf("Encrypt =\n%x\nwant\n%x", got, want)
	}
}

func TestSealUsesFreshKeys(t *testing.T) {
	sub := Subscription{P256DH: b64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		Auth: b64(t, "BTBZMqHH6r4Tts7J_aSIgg")}
	a, err := Seal(sub, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Seal(sub, []byte("x"))
	if bytes.Equal(a[:16], b[:16]) || bytes.Equal(a[21:86], b[21:86]) {
		t.Fatal("salt or server key reused")
	}
}

func TestEncryptRejectsBadKeys(t *testing.T) {
	k, _ := ecdh.P256().GenerateKey(nil)
	if _, err := Encrypt(Subscription{P256DH: []byte{4, 1}, Auth: make([]byte, 16)}, []byte("x"),
		make([]byte, 16), k); err == nil {
		t.Fatal("bad p256dh accepted")
	}
}

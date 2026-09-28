package web

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/push"
	"github.com/aldersfors/matlistan/internal/store"
)

const _pushBodyMax = 4 << 10

type pushSubscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *server) readPushJSON(w http.ResponseWriter, r *http.Request) (pushSubscriptionJSON,
	bool) {
	var v pushSubscriptionJSON
	if s.PushKey == "" {
		s.notFound(w, r)
		return v, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, _pushBodyMax)
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid", http.StatusUnprocessableEntity)
		}
		return v, false
	}
	return v, true
}

// subscribePush stores this device's subscription after checking the endpoint and keys.
func (s *server) subscribePush(w http.ResponseWriter, r *http.Request) {
	v, ok := s.readPushJSON(w, r)
	if !ok {
		return
	}
	// Browsers send unpadded base64url; tolerate padding too.
	p256dh, err1 := base64.RawURLEncoding.DecodeString(strings.TrimRight(v.Keys.P256DH, "="))
	authKey, err2 := base64.RawURLEncoding.DecodeString(strings.TrimRight(v.Keys.Auth, "="))
	if err1 != nil || err2 != nil || push.CheckEndpoint(v.Endpoint) != nil ||
		!push.ValidKeys(p256dh, authKey) {
		http.Error(w, "invalid", http.StatusUnprocessableEntity)
		return
	}
	me, _ := auth.SessionFrom(r.Context())
	err := s.Store.SavePushSubscription(r.Context(), me.Subject,
		push.Subscription{Endpoint: v.Endpoint, P256DH: p256dh, Auth: authKey})
	switch {
	case errors.Is(err, store.ErrTooMany):
		http.Error(w, s.Catalog.T("push.too_many"), http.StatusConflict)
	case err != nil:
		s.fail(w, r, err)
	default:
		s.writeDevices(w, r)
	}
}

// writeDevices answers with the new device count as shown in Settings, so the page can
// update it in place.
func (s *server) writeDevices(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.CountPushSubscriptions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"devices": s.Catalog.N("push.devices", n)})
}

// unsubscribePush removes this device; removing one that is already gone is fine.
func (s *server) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	v, ok := s.readPushJSON(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeletePushSubscription(r.Context(), v.Endpoint); err != nil &&
		!errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	s.writeDevices(w, r)
}

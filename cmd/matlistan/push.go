package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aldersfors/matlistan/internal/config"
	"github.com/aldersfors/matlistan/internal/push"
)

// vapidKeys prints a new VAPID key pair. Store the private key in the secret store; the
// public key is derived from it at runtime, so it is printed only for reference.
func vapidKeys(_ context.Context, e env, _ []string) int {
	priv, pub, err := push.GenerateKey()
	if err != nil {
		_, _ = fmt.Fprintln(e.stderr, "vapid-keys:", err)
		return 1
	}
	_, _ = fmt.Fprintf(e.stdout, "private-key: %s\npublic-key: %s\n", priv, pub)
	return 0
}

// loadVAPID reads the key file; nil means push is off.
func loadVAPID(p config.Push) (*push.VAPID, error) {
	if !p.Enabled() {
		return nil, nil //nolint:nilnil // off is not an error
	}
	b, err := os.ReadFile(p.KeyFile) //nolint:gosec // operator configuration
	if err != nil {
		return nil, fmt.Errorf("read VAPID key: %w", err)
	}
	v, err := push.ParseKey(string(b), p.Subject)
	if err != nil {
		return nil, err // ParseKey never includes the key in its errors
	}
	return v, nil
}

// vapidPublic is the key the browser subscribes with; "" turns web push off.
func vapidPublic(v *push.VAPID) string {
	if v == nil {
		return ""
	}
	return v.PublicKey()
}

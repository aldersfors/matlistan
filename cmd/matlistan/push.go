package main

import (
	"context"
	"fmt"

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

// Package views holds the page components and the chrome every page shares.
package views

import "context"

// Viewer is the signed-in person, as the page chrome shows them.
type Viewer struct {
	Initial, Name, Email string
}

// Build is the running build, formatted for the page footer. Empty fields are unknown.
type Build struct {
	Version, Short, URL     string
	Dirty                   bool
	Committed, CommittedISO string
	Built, BuiltISO         string
}

// Chrome is what every page shows around its content: who is signed in (nil when no one
// is) and the build in the footer.
type Chrome struct {
	Viewer *Viewer
	Build  Build
}

type chromeKey struct{}

// WithChrome returns ctx carrying c for the layout.
func WithChrome(ctx context.Context, c Chrome) context.Context {
	return context.WithValue(ctx, chromeKey{}, c)
}

func chromeFrom(ctx context.Context) Chrome {
	c, _ := ctx.Value(chromeKey{}).(Chrome)
	return c
}

package i18n

import "context"

type ctxKey struct{}

// WithCatalog stores c in ctx; the web layer does this for every request.
func WithCatalog(ctx context.Context, c *Catalog) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// From returns the catalog in ctx. A missing catalog is a programming error.
func From(ctx context.Context) *Catalog {
	c, ok := ctx.Value(ctxKey{}).(*Catalog)
	if !ok {
		panic("invariant violated: no i18n catalog in context")
	}
	return c
}

// T is From(ctx).T.
func T(ctx context.Context, key string, args ...any) string { return From(ctx).T(key, args...) }

// N is From(ctx).N.
func N(ctx context.Context, key string, n int, args ...any) string {
	return From(ctx).N(key, n, args...)
}

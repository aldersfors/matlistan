package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/declared"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/store"
)

type householdStore interface {
	SyncHousehold(ctx context.Context, members []household.Member, recs []recipes.Recipe,
		links []string) (store.SyncResult, error)
	ReleaseHousehold(ctx context.Context) (int64, error)
}

// syncHousehold applies the household file at startup and returns the declared links that
// still need importing. Without a file, rows git owned before are handed back to the app.
func syncHousehold(ctx context.Context, path string, lang i18n.Locale, now time.Time,
	msg func(string) string, st householdStore, log zerolog.Logger) ([]string, error) {
	if path == "" {
		n, err := st.ReleaseHousehold(ctx)
		if n > 0 {
			log.Info().Int64("rows", n).Msg("household no longer declared: unlocked")
		}
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator configuration
	if err != nil {
		return nil, fmt.Errorf("household file: %w", err)
	}
	h, err := declared.Parse(data, lang, now, msg)
	if err != nil {
		return nil, err
	}
	res, err := st.SyncHousehold(ctx, h.Members, h.Recipes, h.RecipeURLs)
	if err != nil {
		return nil, err
	}
	log.Info().Int("members", res.Members).Int("recipes", res.Recipes).Int("links", res.Links).
		Int("archived", res.Archived).Int("to_import", len(res.MissingLinks)).Msg("household synced")
	return res.MissingLinks, nil
}

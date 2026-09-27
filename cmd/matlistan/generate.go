package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/config"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/planner"
	"github.com/aldersfors/matlistan/internal/planner/claude"
	"github.com/aldersfors/matlistan/internal/planner/openai"
	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/web"
	"github.com/aldersfors/matlistan/internal/week"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// generate drafts one week, by default the upcoming one. It does nothing when that week
// already has dinners, so a CronJob can run it more than once.
func generate(ctx context.Context, e env, args []string) int {
	log := newLogger(e.stderr)
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	weekFlag := fs.String("week", "", "ISO week to plan, for example 2026-W40")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.ParseGenerate(e.getenv)
	if err != nil {
		log.Error().Err(err).Msg("config")
		return 1
	}
	k := weekplan.KeyOf(week.Upcoming(time.Now().In(cfg.Location)).Days[0])
	if *weekFlag != "" {
		if k, err = parseWeekFlag(*weekFlag); err != nil {
			_, _ = fmt.Fprintln(e.stderr, err)
			return 2
		}
	}
	db, err := store.Open(ctx, store.Options{URL: cfg.Database.URL, CAFile: cfg.Database.CAFile,
		Log: log})
	if err != nil {
		log.Error().Err(err).Msg("store")
		return 1
	}
	defer db.Close()
	svc, _, err := newPlanner(cfg.Planner, db, cfg.Locale, cfg.Location, log)
	if err != nil {
		log.Error().Err(err).Msg("planner")
		return 1
	}
	if p, err := db.GetPlan(ctx, k); err == nil && (len(p.Entries) > 0 ||
		p.Status == weekplan.StatusApproved) {
		log.Info().Str("week", k.String()).Msg("week already planned")
		return 0
	}
	if err := svc.Generate(ctx, k); err != nil {
		log.Error().Err(err).Str("week", k.String()).Msg("generate")
		return 1
	}
	log.Info().Str("week", k.String()).Msg("week planned")
	return 0
}

func parseWeekFlag(s string) (weekplan.Key, error) {
	var y, w int
	if _, err := fmt.Sscanf(strings.ToUpper(s), "%d-W%d", &y, &w); err != nil {
		return weekplan.Key{}, fmt.Errorf("--week %q: want YYYY-Www", s)
	}
	k, err := weekplan.NewKey(y, w)
	if err != nil {
		return weekplan.Key{}, fmt.Errorf("--week %q: %w", s, err)
	}
	return k, nil
}

// newPlanner builds the planning service and the model client it uses; both nil when
// planning is not configured. The client also serves recipe import.
func newPlanner(cfg config.Planner, db *store.Store, l i18n.Locale, loc *time.Location,
	log zerolog.Logger) (*planner.Service, planner.LLM, error) {
	if !cfg.Enabled() {
		return nil, nil, nil
	}
	var key string
	if cfg.APIKeyFile != "" {
		b, err := os.ReadFile(cfg.APIKeyFile) //nolint:gosec // operator configuration
		if err != nil {
			return nil, nil, fmt.Errorf("api key: %w", err)
		}
		key = strings.TrimSpace(string(b))
	}
	llm := newLLM(cfg, key)
	return planner.NewService(db, llm, l, loc, log,
		func(err error) bool { return errors.Is(err, store.ErrNotFound) }), llm, nil
}

// newLLM builds the client for the configured provider.
func newLLM(cfg config.Planner, key string) planner.LLM {
	if cfg.Provider == config.ProviderOpenAI {
		return openai.New(openai.Config{APIKey: key, Model: cfg.Model,
			BaseURL: cfg.OpenAIBaseURL(), Timeout: 10 * time.Minute, MaxTokens: cfg.MaxOutputTokens})
	}
	return claude.New(claude.Config{APIKey: key, Model: cfg.Model, Timeout: 10 * time.Minute})
}

// webProvider names the provider for the Family page; Host only for a server other than
// api.openai.com.
func webProvider(cfg config.Planner) web.Provider {
	p := web.Provider{Name: cfg.Provider}
	if cfg.Provider == config.ProviderOpenAI && cfg.BaseURL != "" {
		if u, err := url.Parse(cfg.BaseURL); err == nil && u.Hostname() != "api.openai.com" {
			p.Host = u.Hostname()
		}
	}
	return p
}

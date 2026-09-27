package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/config"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/theme"
	"github.com/aldersfors/matlistan/internal/web"
)

func serve(ctx context.Context, e env, _ []string) int {
	log := newLogger(e.stderr)
	cfg, err := config.Parse(e.getenv)
	if err != nil {
		log.Error().Err(err).Msg("config")
		return 1
	}
	if err := serveWith(ctx, cfg, log); err != nil {
		log.Error().Err(err).Msg("serve")
		return 1
	}
	return 0
}

func serveWith(ctx context.Context, cfg config.Config, log zerolog.Logger) error {
	catalog, err := i18n.Load(cfg.Locale)
	if err != nil {
		return err
	}
	var th theme.Theme
	if cfg.ThemeFile != "" {
		if th, err = theme.Load(cfg.ThemeFile); err != nil {
			return err
		}
	}
	db, err := store.Open(ctx, store.Options{URL: cfg.Database.URL, CAFile: cfg.Database.CAFile,
		Log: log})
	if err != nil {
		return err
	}
	defer db.Close()
	now := func() time.Time { return time.Now().In(cfg.Location) }
	go pruneAuthEvents(ctx, db, now, log)
	svc, err := newPlanner(cfg.Planner, db, cfg.Locale, cfg.Location, log)
	if err != nil {
		return err
	}
	if svc == nil {
		log.Info().Msg("planning disabled: MATLISTAN_ANTHROPIC_API_KEY_FILE is not set")
	} else {
		log.Info().Str("model", cfg.Planner.Model).Msg("planning enabled")
	}
	var pl web.Planner // stays a nil interface when planning is off
	if svc != nil {
		pl = svc
	}

	key, err := auth.LoadKey(cfg.SessionKeyFile)
	if err != nil {
		return err
	}
	codec, err := auth.NewCodec(key, nil)
	if err != nil {
		return err
	}
	secret, err := os.ReadFile(cfg.OIDC.ClientSecretFile) //nolint:gosec // operator configuration
	if err != nil {
		return fmt.Errorf("oidc client secret: %w", err)
	}
	pool, err := caPool(cfg.OIDC.CAFile)
	if err != nil {
		return err
	}
	authn, err := auth.New(ctx, auth.Config{Issuer: cfg.OIDC.Issuer, ClientID: cfg.OIDC.ClientID,
		ClientSecret: strings.TrimSpace(string(secret)), RedirectURL: cfg.RedirectURL(),
		CAPool: pool, Claim: cfg.OIDC.Claim, Allowed: cfg.OIDC.Allowed,
		SessionAgeMax: 30 * 24 * time.Hour}, codec, db, log, now)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.Addr, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute,
		Handler: web.New(web.Deps{Catalog: catalog, Theme: th, Auth: authn, Store: db, Planner: pl,
			BaseURL: cfg.BaseURL,
			Now:     now,
			Log:     log})}
	metrics := &http.Server{Addr: cfg.MetricsAddr, ReadHeaderTimeout: 10 * time.Second,
		Handler: web.Metrics()}
	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServe() }()
	go func() { errc <- metrics.ListenAndServe() }()
	log.Info().Str("addr", cfg.Addr).Str("locale", string(cfg.Locale)).Msg("listening")
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		// ctx is already cancelled; keep its values but give in-flight requests 15s to finish.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_ = metrics.Shutdown(shutdown) // best effort; scrapes are not in-flight user work
		if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func caPool(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, nil //nolint:nilnil // nil pool means system roots
	}
	b, err := os.ReadFile(path) //nolint:gosec // operator configuration
	if err != nil {
		return nil, fmt.Errorf("oidc ca: %w", err)
	}
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(b) {
		return nil, errors.New("oidc ca: no certificates found")
	}
	return p, nil
}

// Auth events hold email addresses; keep them only as long as they help an investigation.
const authEventsRetention = 90 * 24 * time.Hour

func pruneAuthEvents(ctx context.Context, db *store.Store, now func() time.Time,
	log zerolog.Logger) {
	tick := time.NewTicker(24 * time.Hour)
	defer tick.Stop()
	for {
		n, err := db.PruneAuthEvents(ctx, now().Add(-authEventsRetention))
		if err != nil {
			log.Warn().Err(err).Msg("prune auth events")
		} else if n > 0 {
			log.Info().Int64("deleted", n).Msg("auth events pruned")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

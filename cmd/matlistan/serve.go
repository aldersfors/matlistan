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

	"github.com/jalet/matlistan/internal/auth"
	"github.com/jalet/matlistan/internal/config"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/store"
	"github.com/jalet/matlistan/internal/theme"
	"github.com/jalet/matlistan/internal/web"
)

func serve(ctx context.Context, e env) int {
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
	now := func() time.Time { return time.Now().In(cfg.Location) }
	authn, err := auth.New(ctx, auth.Config{Issuer: cfg.OIDC.Issuer, ClientID: cfg.OIDC.ClientID,
		ClientSecret: strings.TrimSpace(string(secret)), RedirectURL: cfg.RedirectURL(),
		CAPool: pool, Claim: cfg.OIDC.Claim, Allowed: cfg.OIDC.Allowed,
		SessionAgeMax: 30 * 24 * time.Hour}, codec, db, log, now)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.Addr, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute,
		Handler: web.New(web.Deps{Catalog: catalog, Theme: th, Auth: authn, DB: db, Now: now, Log: log})}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info().Str("addr", cfg.Addr).Str("locale", string(cfg.Locale)).Msg("listening")
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		// ctx is already cancelled; keep its values but give in-flight requests 15s to finish.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
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

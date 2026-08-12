// evogo-connect: servidor principal. Inicializa config, log, store, bridge,
// migrations e expõe o router HTTP. Graceful shutdown em SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/edbentto22/evogo-connect/internal/bridge"
	"github.com/edbentto22/evogo-connect/internal/config"
	"github.com/edbentto22/evogo-connect/internal/crypto"
	"github.com/edbentto22/evogo-connect/internal/httpapi"
	applog "github.com/edbentto22/evogo-connect/internal/log"
	"github.com/edbentto22/evogo-connect/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Default().Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Carrega config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Logger
	applog.Init(cfg.LogLevel, cfg.LogFormat)
	slog.Default().Info("evogo-connect starting",
		"version", "0.1.0",
		"server_addr", cfg.ServerAddr,
		"log_level", cfg.LogLevel,
	)

	// Cipher para segredos em repouso
	cipher, err := crypto.New(cfg.ConnectMasterKey)
	if err != nil {
		return fmt.Errorf("crypto: %w", err)
	}

	// Store (Postgres)
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelBoot()
	st, err := store.New(bootCtx, cfg.DatabaseURL, cfg.DatabaseMaxConns, cfg.DatabaseMinConns, cipher)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()
	slog.Default().Info("connected to postgres")

	// Migrations
	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	slog.Default().Info("migrations applied")

	// Bridge core
	core := bridge.New(st, cfg.IdempotencyTTL, cfg.BridgePaused)

	// Router HTTP
	router := httpapi.NewRouter(httpapi.Deps{
		Store:        st,
		Bridge:       core,
		AdminToken:   cfg.AdminToken,
		ReplayWindow: cfg.HMACReplayWindow,
	})

	srv := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      router,
		ReadTimeout:  cfg.ServerReadTimeout,
		WriteTimeout: cfg.ServerWriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	// Inicia em goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Default().Info("http server listening", "addr", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Espera shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		slog.Default().Info("shutdown signal received", "signal", sig.String())
	case err := <-serverErr:
		slog.Default().Error("server error", "err", err)
		return err
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Default().Error("graceful shutdown failed", "err", err)
		return err
	}
	slog.Default().Info("evogo-connect stopped cleanly")
	return nil
}

func runMigrations(dsn string) error {
	m, err := migrate.New(
		"file://migrations",
		dsn,
	)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

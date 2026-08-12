package main

import (
	"context"
	"fmt"

	"github.com/edbentto22/evogo-connect/internal/chatwoot"
	"github.com/edbentto22/evogo-connect/internal/config"
	"github.com/edbentto22/evogo-connect/internal/crypto"
	"github.com/edbentto22/evogo-connect/internal/store"
)

// loadStoreCLI carrega config e store a partir do env. Retorna o config
// (caso precise para outras coisas) e o store.
func loadStoreCLI() (*config.Config, *store.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	cipher, err := crypto.New(cfg.ConnectMasterKey)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: %w", err)
	}
	ctx := context.TODO()
	st, err := store.New(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns, cfg.DatabaseMinConns, cipher)
	if err != nil {
		return nil, nil, fmt.Errorf("store: %w", err)
	}
	return cfg, st, nil
}

// chatwootClientFor monta um chatwoot.Client a partir de um tenant carregado.
func chatwootClientFor(t *store.Tenant) *chatwoot.Client {
	return chatwoot.NewClient(t.ChatwootBaseURL, t.ChatwootAccountID, t.ChatwootToken)
}

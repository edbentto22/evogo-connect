// Package config carrega configuração via env vars usando envconfig.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config é a configuração raiz do evogo-connect.
type Config struct {
	// Server
	ServerAddr         string        `envconfig:"SERVER_ADDR" default:":9090"`
	ServerReadTimeout  time.Duration `envconfig:"SERVER_READ_TIMEOUT" default:"15s"`
	ServerWriteTimeout time.Duration `envconfig:"SERVER_WRITE_TIMEOUT" default:"30s"`

	// Log
	LogLevel  string `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat string `envconfig:"LOG_FORMAT" default:"json"`

	// Database
	DatabaseURL      string `envconfig:"DATABASE_URL" required:"true"`
	DatabaseMaxConns int    `envconfig:"DATABASE_MAX_CONNS" default:"10"`
	DatabaseMinConns int    `envconfig:"DATABASE_MIN_CONNS" default:"1"`

	// Security
	ConnectMasterKey string `envconfig:"CONNECT_MASTER_KEY" required:"true"`
	AdminToken       string `envconfig:"ADMIN_TOKEN" required:"true"`

	// Kill switch
	BridgePaused bool `envconfig:"BRIDGE_PAUSED" default:"false"`

	// Rate limit
	RateLimitPerMinute int `envconfig:"RATE_LIMIT_PER_MINUTE" default:"120"`

	// Idempotência
	IdempotencyTTL time.Duration `envconfig:"IDEMPOTENCY_TTL" default:"24h"`

	// HMAC
	HMACReplayWindow time.Duration `envconfig:"HMAC_REPLAY_WINDOW" default:"5m"`
}

// Load carrega e valida a configuração do ambiente.
func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, fmt.Errorf("config: load: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if len(c.ConnectMasterKey) < 16 {
		return fmt.Errorf("config: CONNECT_MASTER_KEY too short (need ≥16 chars / base64 32 bytes)")
	}
	if c.AdminToken == "" || c.AdminToken == "change-me-to-a-random-secret" {
		return fmt.Errorf("config: ADMIN_TOKEN not set or still default")
	}
	if c.RateLimitPerMinute < 1 {
		return fmt.Errorf("config: RATE_LIMIT_PER_MINUTE must be ≥ 1")
	}
	return nil
}

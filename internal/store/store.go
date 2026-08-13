// Package store encapsula o acesso ao Postgres (pgxpool).
//
// Responsabilidades:
//   - Conectar pgxpool
//   - Migrar schema (chamado pelo main na inicialização)
//   - CRUD de tenants, contact_map, idempotency, bridge_log, bridge_paused
//   - Cifrar/decifrar segredos ao persistir/ler (AES-GCM via crypto.Cipher)
//
// Todas as queries são parametrizadas (sem string concat). Toda escrita
// passa por transações quando há mais de uma tabela envolvida.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edbentto22/evogo-connect/internal/crypto"
)

// Store é o ponto único de acesso ao banco.
type Store struct {
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
}

// New cria um Store a partir de uma connect string.
func New(ctx context.Context, dsn string, maxConns, minConns int, cipher *crypto.Cipher) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = int32(maxConns)
	}
	if minConns > 0 {
		cfg.MinConns = int32(minConns)
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{pool: pool, cipher: cipher}, nil
}

// Close fecha o pool.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Pool expõe o pool (uso avançado — preferir métodos).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Ping verifica a conectividade sem expor o pool aos handlers HTTP.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("store: ping: %w", err)
	}
	return nil
}

// ─── Tenants ──────────────────────────────────────────────────────────────

// Tenant representa um par (Chatwoot inbox) ↔ (evolution-go instance).
type Tenant struct {
	ID                uuid.UUID
	Name              string
	ChatwootAccountID int
	ChatwootInboxID   int
	ChatwootBaseURL   string
	ChatwootToken     string
	ChatwootHMAC      string // vazio se não configurado
	EvoInstanceName   string
	EvoBaseURL        string
	EvoAPIKey         string
	EvoWebhookSecret  string // vazio se não configurado
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateTenant persiste um tenant novo. Tokens são cifrados em repouso.
func (s *Store) CreateTenant(ctx context.Context, t *Tenant) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	tokEnc, err := s.cipher.EncryptString(t.ChatwootToken)
	if err != nil {
		return fmt.Errorf("store: encrypt chatwoot token: %w", err)
	}
	evoKeyEnc, err := s.cipher.EncryptString(t.EvoAPIKey)
	if err != nil {
		return fmt.Errorf("store: encrypt evo key: %w", err)
	}
	var hmacEnc, evoSecretEnc []byte
	if t.ChatwootHMAC != "" {
		hmacEnc, err = s.cipher.EncryptString(t.ChatwootHMAC)
		if err != nil {
			return fmt.Errorf("store: encrypt chatwoot hmac: %w", err)
		}
	}
	if t.EvoWebhookSecret != "" {
		evoSecretEnc, err = s.cipher.EncryptString(t.EvoWebhookSecret)
		if err != nil {
			return fmt.Errorf("store: encrypt evo secret: %w", err)
		}
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO tenants (
			id, name, chatwoot_account_id, chatwoot_inbox_id, chatwoot_base_url,
			chatwoot_token_enc, chatwoot_hmac_enc,
			evo_instance_name, evo_base_url, evo_api_key_enc, evo_webhook_secret_enc
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, t.ID, t.Name, t.ChatwootAccountID, t.ChatwootInboxID, t.ChatwootBaseURL,
		tokEnc, hmacEnc, t.EvoInstanceName, t.EvoBaseURL, evoKeyEnc, evoSecretEnc)
	if err != nil {
		return fmt.Errorf("store: insert tenant: %w", err)
	}
	return nil
}

// UpdateTenantIntegration atualiza credenciais e metadados de um tenant já
// existente, preservando seu ID, inbox e relacionamentos locais.
func (s *Store) UpdateTenantIntegration(ctx context.Context, t *Tenant) error {
	chatwootTokenEnc, err := s.cipher.EncryptString(t.ChatwootToken)
	if err != nil {
		return fmt.Errorf("store: encrypt chatwoot token: %w", err)
	}
	chatwootSecretEnc, err := s.cipher.EncryptString(t.ChatwootHMAC)
	if err != nil {
		return fmt.Errorf("store: encrypt chatwoot secret: %w", err)
	}
	evoTokenEnc, err := s.cipher.EncryptString(t.EvoAPIKey)
	if err != nil {
		return fmt.Errorf("store: encrypt evo token: %w", err)
	}
	evoSecretEnc, err := s.cipher.EncryptString(t.EvoWebhookSecret)
	if err != nil {
		return fmt.Errorf("store: encrypt evo webhook secret: %w", err)
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE tenants SET
			chatwoot_account_id = $2,
			chatwoot_base_url = $3,
			chatwoot_token_enc = $4,
			chatwoot_hmac_enc = $5,
			evo_instance_name = $6,
			evo_base_url = $7,
			evo_api_key_enc = $8,
			evo_webhook_secret_enc = $9,
			updated_at = now()
		WHERE id = $1
	`, t.ID, t.ChatwootAccountID, t.ChatwootBaseURL, chatwootTokenEnc,
		chatwootSecretEnc, t.EvoInstanceName, t.EvoBaseURL, evoTokenEnc, evoSecretEnc)
	if err != nil {
		return fmt.Errorf("store: update tenant integration: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

// GetTenantByEvoInstance busca o tenant pela instância Evolution Go. O nome
// é usado apenas para resolver o tenant antes de validar o segredo da URL.
func (s *Store) GetTenantByEvoInstance(ctx context.Context, instance string) (*Tenant, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, chatwoot_account_id, chatwoot_inbox_id, chatwoot_base_url,
		       chatwoot_token_enc, chatwoot_hmac_enc,
		       evo_instance_name, evo_base_url, evo_api_key_enc, evo_webhook_secret_enc,
		       created_at, updated_at
		FROM tenants WHERE evo_instance_name = $1
	`, instance)
	return s.scanTenant(row)
}

// GetTenantByChatwootInbox busca tenant pelo inbox_id do Chatwoot.
func (s *Store) GetTenantByChatwootInbox(ctx context.Context, inboxID int) (*Tenant, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, chatwoot_account_id, chatwoot_inbox_id, chatwoot_base_url,
		       chatwoot_token_enc, chatwoot_hmac_enc,
		       evo_instance_name, evo_base_url, evo_api_key_enc, evo_webhook_secret_enc,
		       created_at, updated_at
		FROM tenants WHERE chatwoot_inbox_id = $1
	`, inboxID)
	return s.scanTenant(row)
}

// GetTenantByName busca tenant pelo name (slug).
func (s *Store) GetTenantByName(ctx context.Context, name string) (*Tenant, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, chatwoot_account_id, chatwoot_inbox_id, chatwoot_base_url,
		       chatwoot_token_enc, chatwoot_hmac_enc,
		       evo_instance_name, evo_base_url, evo_api_key_enc, evo_webhook_secret_enc,
		       created_at, updated_at
		FROM tenants WHERE name = $1
	`, name)
	return s.scanTenant(row)
}

// ListTenants lista todos os tenants (uso admin).
func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, chatwoot_account_id, chatwoot_inbox_id, chatwoot_base_url,
		       chatwoot_token_enc, chatwoot_hmac_enc,
		       evo_instance_name, evo_base_url, evo_api_key_enc, evo_webhook_secret_enc,
		       created_at, updated_at
		FROM tenants ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list tenants: %w", err)
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		t, err := s.scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) scanTenant(row pgx.Row) (*Tenant, error) {
	var t Tenant
	var tokEnc, hmacEnc, evoKeyEnc, evoSecretEnc []byte
	err := row.Scan(
		&t.ID, &t.Name, &t.ChatwootAccountID, &t.ChatwootInboxID, &t.ChatwootBaseURL,
		&tokEnc, &hmacEnc,
		&t.EvoInstanceName, &t.EvoBaseURL, &evoKeyEnc, &evoSecretEnc,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: scan tenant: %w", err)
	}
	t.ChatwootToken, err = s.cipher.DecryptString(tokEnc)
	if err != nil {
		return nil, fmt.Errorf("store: decrypt chatwoot token: %w", err)
	}
	if len(hmacEnc) > 0 {
		t.ChatwootHMAC, err = s.cipher.DecryptString(hmacEnc)
		if err != nil {
			return nil, fmt.Errorf("store: decrypt hmac: %w", err)
		}
	}
	t.EvoAPIKey, err = s.cipher.DecryptString(evoKeyEnc)
	if err != nil {
		return nil, fmt.Errorf("store: decrypt evo key: %w", err)
	}
	if len(evoSecretEnc) > 0 {
		t.EvoWebhookSecret, err = s.cipher.DecryptString(evoSecretEnc)
		if err != nil {
			return nil, fmt.Errorf("store: decrypt evo secret: %w", err)
		}
	}
	return &t, nil
}

// ─── Contact Map ──────────────────────────────────────────────────────────

// ContactMap mapeia JID WhatsApp ↔ contato Chatwoot.
type ContactMap struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	JID               string
	ChatwootContactID int
	SourceID          string
	DisplayName       string
	CreatedAt         time.Time
}

// CreateContact insere um mapeamento.
func (s *Store) CreateContact(ctx context.Context, c *ContactMap) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO contact_map (id, tenant_id, jid, chatwoot_contact_id, source_id, display_name)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, jid) DO UPDATE SET
			chatwoot_contact_id = EXCLUDED.chatwoot_contact_id,
			source_id = EXCLUDED.source_id,
			display_name = EXCLUDED.display_name
	`, c.ID, c.TenantID, c.JID, c.ChatwootContactID, c.SourceID, c.DisplayName)
	if err != nil {
		return fmt.Errorf("store: insert contact: %w", err)
	}
	return nil
}

// GetContactByJID busca mapeamento por tenant + JID.
func (s *Store) GetContactByJID(ctx context.Context, tenantID uuid.UUID, jid string) (*ContactMap, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, jid, chatwoot_contact_id, source_id, display_name, created_at
		FROM contact_map WHERE tenant_id = $1 AND jid = $2
	`, tenantID, jid)
	var c ContactMap
	err := row.Scan(&c.ID, &c.TenantID, &c.JID, &c.ChatwootContactID, &c.SourceID, &c.DisplayName, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: scan contact: %w", err)
	}
	return &c, nil
}

// ─── Idempotency ──────────────────────────────────────────────────────────

// IdempotencyClaim descreve o estado observado ao adquirir uma chave.
type IdempotencyClaim string

const (
	ClaimAcquired   IdempotencyClaim = "acquired"
	ClaimInProgress IdempotencyClaim = "in_progress"
	ClaimCompleted  IdempotencyClaim = "completed"
)

// ClaimIdempotency tenta reservar atomicamente uma chave antes do efeito
// externo. Claims falhos ou expirados podem ser retomados.
func (s *Store) ClaimIdempotency(ctx context.Context, direction, key string, tenantID uuid.UUID, ttl time.Duration, claimToken string) (IdempotencyClaim, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO idempotency (key, direction, tenant_id, status, detail, expires_at)
		VALUES ($1,$2,$3,'processing',jsonb_build_object('claim_token',$5::text), now() + $4::interval)
		ON CONFLICT (direction, key) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			status = 'processing',
			detail = EXCLUDED.detail,
			created_at = now(),
			expires_at = EXCLUDED.expires_at
		WHERE idempotency.status = 'failed' OR idempotency.expires_at <= now()
		RETURNING true
	`, key, direction, tenantID, ttl.String(), claimToken)
	var claimed bool
	if err := row.Scan(&claimed); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("store: claim idempotency: %w", err)
		}
		var status string
		if err := s.pool.QueryRow(ctx, `
			SELECT status FROM idempotency WHERE direction = $1 AND key = $2
		`, direction, key).Scan(&status); err != nil {
			return "", fmt.Errorf("store: read idempotency state: %w", err)
		}
		if status == "sent" {
			return ClaimCompleted, nil
		}
		return ClaimInProgress, nil
	}
	if !claimed {
		return "", errors.New("store: claim idempotency returned invalid state")
	}
	return ClaimAcquired, nil
}

// CompleteDelivery marca o claim como entregue e grava sua auditoria na mesma
// transação, evitando estado sent sem trilha de auditoria.
func (s *Store) CompleteDelivery(ctx context.Context, direction, key, claimToken string, detail []byte, ttl time.Duration, entry BridgeLogEntry) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin delivery completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE idempotency
		SET status = 'sent', detail = $3, expires_at = now() + $4::interval
		WHERE direction = $1 AND key = $2 AND status = 'processing'
		  AND detail->>'claim_token' = $5
	`, direction, key, detail, ttl.String(), claimToken)
	if err != nil {
		return fmt.Errorf("store: complete idempotency: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("store: complete delivery: active claim not found")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO bridge_log (tenant_id, direction, external_message_id, jid,
		                       payload_sha256, status, error_code, error_detail, latency_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, entry.TenantID, entry.Direction, entry.ExternalMessageID, entry.JID,
		entry.PayloadSHA256, entry.Status, nullIfEmpty(entry.ErrorCode), nullIfEmpty(entry.ErrorDetail), entry.LatencyMS)
	if err != nil {
		return fmt.Errorf("store: insert delivery audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit delivery completion: %w", err)
	}
	return nil
}

// ReleaseIdempotency torna uma tentativa falha imediatamente reenviável.
func (s *Store) ReleaseIdempotency(ctx context.Context, direction, key, claimToken string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE idempotency
		SET status = 'failed', detail = '{}'::jsonb
		WHERE direction = $1 AND key = $2 AND status = 'processing'
		  AND detail->>'claim_token' = $3
	`, direction, key, claimToken)
	if err != nil {
		return fmt.Errorf("store: release idempotency: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("store: release idempotency: active claim not found")
	}
	return nil
}

// ─── Bridge Log ───────────────────────────────────────────────────────────

// BridgeLogEntry é a entrada de audit log.
type BridgeLogEntry struct {
	TenantID          uuid.UUID
	Direction         string
	ExternalMessageID string
	JID               string
	PayloadSHA256     []byte
	Status            string
	ErrorCode         string
	ErrorDetail       string
	LatencyMS         int
}

// LogBridge grava uma entrada de audit. Sem PII — só IDs e hash.
func (s *Store) LogBridge(ctx context.Context, e BridgeLogEntry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bridge_log (tenant_id, direction, external_message_id, jid,
		                       payload_sha256, status, error_code, error_detail, latency_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, e.TenantID, e.Direction, e.ExternalMessageID, e.JID,
		e.PayloadSHA256, e.Status, nullIfEmpty(e.ErrorCode), nullIfEmpty(e.ErrorDetail), e.LatencyMS)
	if err != nil {
		return fmt.Errorf("store: log bridge: %w", err)
	}
	return nil
}

// ─── Kill Switch ──────────────────────────────────────────────────────────

// IsPaused verifica se o bridge está pausado (qualquer linha em bridge_paused).
func (s *Store) IsPaused(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bridge_paused)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: check paused: %w", err)
	}
	return exists, nil
}

// SetPaused ativa/desativa o kill switch.
func (s *Store) SetPaused(ctx context.Context, paused bool, reason string) error {
	if paused {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO bridge_paused (id, reason) VALUES (1, $1)
			ON CONFLICT (id) DO UPDATE SET paused_at = now(), reason = EXCLUDED.reason
		`, reason)
		if err != nil {
			return fmt.Errorf("store: set paused: %w", err)
		}
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM bridge_paused`)
	if err != nil {
		return fmt.Errorf("store: unset paused: %w", err)
	}
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────

// ErrNotFound é retornado quando uma entidade não existe.
var ErrNotFound = errors.New("store: not found")

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

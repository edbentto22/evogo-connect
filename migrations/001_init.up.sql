-- 001_init.up.sql
-- Schema inicial do evogo-connect (Etapa 1 — reverse bridge).

-- Tenants: mapeia (Chatwoot inbox) ↔ (evolution-go instance).
-- Cada tenant é um par isolado de credenciais.
CREATE TABLE IF NOT EXISTS tenants (
    id                   UUID PRIMARY KEY,
    name                 TEXT NOT NULL UNIQUE,
    chatwoot_account_id  INTEGER NOT NULL,
    chatwoot_inbox_id    INTEGER NOT NULL UNIQUE,
    chatwoot_base_url    TEXT NOT NULL,
    chatwoot_token_enc   BYTEA NOT NULL,           -- AES-256-GCM(ciphertext + tag)
    chatwoot_hmac_enc    BYTEA,                    -- opcional
    evo_instance_name    TEXT NOT NULL,
    evo_base_url         TEXT NOT NULL,
    evo_api_key_enc      BYTEA NOT NULL,
    evo_webhook_secret_enc BYTEA,                  -- para Etapa 2
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tenants_chatwoot_inbox ON tenants(chatwoot_inbox_id);

-- contact_map: mapeia JID do WhatsApp ↔ contato do Chatwoot.
-- Populado por `connect add-contact` (Etapa 1) e por forward bridge (Etapa 2).
CREATE TABLE IF NOT EXISTS contact_map (
    id                    UUID PRIMARY KEY,
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    jid                   TEXT NOT NULL,            -- ex: 5511999999999@s.whatsapp.net
    chatwoot_contact_id   INTEGER NOT NULL,
    source_id             TEXT NOT NULL,            -- eco do source_id do Chatwoot
    display_name          TEXT,                     -- opcional
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, jid)
);

CREATE INDEX IF NOT EXISTS idx_contact_map_tenant_jid ON contact_map(tenant_id, jid);
CREATE INDEX IF NOT EXISTS idx_contact_map_tenant_contact ON contact_map(tenant_id, chatwoot_contact_id);

-- idempotency: chaves para deduplicar retries de webhooks.
-- key: ID externo da mensagem (chatwoot_message_id ou evo_message_id)
-- direction: "c2w" ou "w2c"
CREATE TABLE IF NOT EXISTS idempotency (
    key            TEXT NOT NULL,
    direction      TEXT NOT NULL,
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    status         TEXT NOT NULL,                   -- "sent", "skipped_duplicate", "failed"
    detail         JSONB,                            -- payload de resposta ou erro (sem PII)
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (direction, key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_expires ON idempotency(expires_at);

-- bridge_log: audit log de toda mensagem bridgeada. Sem conteúdo — só IDs e hashes.
CREATE TABLE IF NOT EXISTS bridge_log (
    id                  BIGSERIAL PRIMARY KEY,
    tenant_id           UUID NOT NULL,
    direction           TEXT NOT NULL,                -- "c2w" ou "w2c"
    external_message_id TEXT,                          -- id da msg no sistema de origem
    jid                 TEXT,                          -- telefone mascarado em logs
    payload_sha256      BYTEA,                          -- hash do conteúdo (correlação)
    status              TEXT NOT NULL,                  -- "ok", "skipped", "error"
    error_code          TEXT,
    error_detail        TEXT,                          -- sem PII
    latency_ms          INTEGER,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bridge_log_tenant_created ON bridge_log(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bridge_log_status ON bridge_log(status) WHERE status = 'error';

-- bridge_paused: kill switch global (substitui o env BRIDGE_PAUSED para permitir toggle on-the-fly).
-- Se houver qualquer linha, o bridge está pausado.
CREATE TABLE IF NOT EXISTS bridge_paused (
    id           SMALLINT PRIMARY KEY DEFAULT 1,
    paused_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason       TEXT,
    CHECK (id = 1)
);

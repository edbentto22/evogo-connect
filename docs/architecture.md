# Architecture — evogo-connect

## Visão geral

O `evogo-connect` é um bridge HTTP bidirecional entre dois sistemas:

- **evolution-go** (provedor WhatsApp via whatsmeow) — REST + WebSocket
- **Chatwoot** (plataforma de atendimento self-hosted) — REST + Webhooks

O connector **não** altera o código de nenhum dos dois lados. Apenas:
- Configura um webhook URL no evolution-go (apontando pro nosso `/webhook/evo/<instance>`)
- Cria uma inbox API no Chatwoot (apontando pro nosso `/webhook/chatwoot`)
- Persiste tokens e mapeamentos no Postgres

## Diagrama

```
┌──────────────┐  webhooks   ┌──────────────────┐  REST   ┌──────────┐
│  evolution-  │ ──────────► │ evogo-connect    │ ──────► │ Chatwoot │
│     go       │             │ (Go binary)      │         │ (self-   │
│              │ ◄────────── │                  │ ◄────── │  hosted) │
└──────────────┘   send msg  └──────────────────┘ webhooks└──────────┘
                 (REST)        ▲
                               │ /admin (ops)
                               ▼
                         (PostgreSQL p/ idempotência + audit)
```

## Camadas

```
cmd/evogo-connect/main.go   — entry point, graceful shutdown
        │
        ▼
internal/bridge             — core: resolve tenant → contact → JID → send
   ├── HandleChatwootWebhook (Etapa 1)
   └── HandleEvoWebhook      (Etapa 2 — futuro)
        │
        ├─► internal/store    — pgxpool (tenants, contact_map, idempotency, audit)
        ├─► internal/crypto   — AES-GCM (segredos em repouso)
        ├─► internal/chatwoot — client HTTP + tipos
        ├─► internal/evogo    — client HTTP + tipos
        └─► internal/metrics  — Prometheus
        ▲
        │
internal/httpapi            — Gin router
   ├── /webhook/chatwoot     — entrada de msgs outgoing do Chatwoot
   ├── /webhook/evo/:inst    — entrada de msgs do evolution-go (Etapa 2)
   ├── /admin/*              — kill switch, listagem
   ├── /healthz, /readyz     — health probes
   └── /metrics              — Prometheus exposition

cmd/connect-cli/main.go     — CLI de provisionamento
   ├── setup                 — cria inbox no Chatwoot, persiste tenant
   ├── add-contact           — cria contato no Chatwoot, mapeia JID
   ├── list                  — lista tenants
   ├── status                — estado + audit recente
   ├── pause / resume        — kill switch
```

## Fluxo: agente responde no Chatwoot → cliente recebe no WhatsApp (Etapa 1)

1. **Agente** digita resposta no Chatwoot e envia.
2. **Chatwoot** POSTa envelope JSON em `/webhook/chatwoot` com
   `event=message_created`, `message_type=outgoing`, headers de assinatura.
3. **evogo-connect** (`httpapi.webhook_chatwoot.go`):
   - Lê body cru (5MB max)
   - Decodifica envelope, extrai `inbox_id`
   - Resolve tenant via `store.GetTenantByChatwootInbox`
   - Valida HMAC com `tenant.ChatwootHMAC` (constant-time)
4. **bridge.Core.HandleChatwootWebhook** (`internal/bridge/bridge.go`):
   - Checa kill switch (env + DB)
   - Filtra: `message_created` + `outgoing` + `!private`
   - Re-valida HMAC (defesa em profundidade)
   - Resolve JID via `conversation.contact_inbox.source_id`
   - Idempotência: `CheckIdempotency("c2w", key)` — se já enviado, responde 200 sem reenviar
   - Cria `evogo.Client`, chama `SendText` ou `SendMedia`
   - Grava `RecordIdempotency` + `bridge_log` (sem PII)
   - Retorna 200 (ou 503 se pausado)
5. **evolution-go** recebe o POST `/message/sendText/{instance}` e entrega
   via whatsmeow.
6. **Cliente** recebe a mensagem no WhatsApp.

Retry do Chatwoot: se a resposta for != 2xx, Chatwoot retenta (5x por
padrão, com backoff). Idempotency garante que retries não duplicam.

## Fluxo: cliente manda msg WhatsApp → agente recebe no Chatwoot (Etapa 2 — futuro)

(planejado; ainda não implementado)

1. evolution-go dispara `MESSAGES_UPSERT` no webhook configurado em
   `/webhook/evo/<instance>`.
2. evogo-connect valida `X-Evogo-Secret` (replay window 5 min).
3. Get-or-create contact no Chatwoot com `source_id = remoteJid`.
4. Get-or-create conversation (status=open).
5. POST message incoming no Chatwoot.
6. Mídia: download da URL evolution-go + upload multipart Chatwoot.

## Modelo de dados (Postgres)

```
tenants         (id, name, chatwoot_*, evo_*, *_enc, created_at, updated_at)
contact_map     (id, tenant_id, jid, chatwoot_contact_id, source_id, display_name)
idempotency     (key, direction, tenant_id, status, detail, created_at, expires_at)
bridge_log      (id, tenant_id, direction, external_message_id, jid,
                 payload_sha256, status, error_code, error_detail, latency_ms)
bridge_paused   (id, paused_at, reason)   -- 0 ou 1 linha
```

Detalhes do schema em `migrations/001_init.up.sql`.

## Trade-offs e porquês

- **Binário Go único, sem plugin:** deploy trivial, debug simples,
  mesmo stack do evolution-go. Custo: 1 container a mais pra rodar.
- **Postgres separado do evolution-go:** isolamento de concern (audit
  log e segredos não misturam com storage do whatsmeow). Compartilhar
  Postgres seria arriscado (migrations colidem).
- **AES-GCM próprio (não Vault):** simplicidade operacional. Em produção
  alta-escala, mover chave mestra pro Vault e refatorar `crypto.New`.
- **Idempotência em DB (não Redis):** corretude > performance neste
  volume. Redis adiciona mais um componente a operar.
- **Sem framework de injeção de dependência:** o `Deps` struct do
  `httpapi` é wiring manual. Suficiente para um serviço deste tamanho.
- **Sem ORM (pgx puro):** queries explícitas, fácil de auditar SQL.

## Próximas evoluções (Etapas 2-7)

Ver `.context/plans/2026-08-12-evogo-chatwoot-connector.md` (Etapas 2-8).

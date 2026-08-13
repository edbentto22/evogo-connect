# Architecture — evogo-connect

## Visão geral

O `evogo-connect` é um bridge HTTP bidirecional entre dois sistemas:

- **evolution-go** (provedor WhatsApp via whatsmeow) — REST + WebSocket
- **Chatwoot** (plataforma de atendimento self-hosted) — REST + Webhooks

O connector **não** altera o código de nenhum dos dois lados. Nesta etapa ele:

- Cria uma inbox API no Chatwoot apontando para `/webhook/chatwoot`.
- Persiste o `secret` da inbox e o token individual da instância Evolution Go.
- Cria o contato e o vínculo `contact_inbox` com `source_id = JID`.
- Configura um webhook exclusivo por instância na Evolution Go.

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
   └── HandleEvogoWebhook    (WhatsApp → Chatwoot)
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
   ├── /webhook/evo/:inst/:secret — entrada autenticada de msgs do evolution-go
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
   - Resolve JID via `conversation.contact_inbox.source_id`; para webhooks
     outgoing do Fazer.ai que omitirem esse campo, usa
     `conversation.meta.sender.identifier` como fallback validado.
   - Adquire atomicamente o claim de idempotência antes do envio
   - Cria `evogo.Client`, chama `SendText` ou `SendMedia`
   - Conclui o claim e grava `bridge_log` sem PII; falha de envio libera retry
   - Retorna 200 (ou 503 se pausado)
5. **Evolution Go** recebe `POST /send/text` ou `POST /send/media`, autenticado
   pelo token individual da instância, e entrega
   via whatsmeow.
6. **Cliente** recebe a mensagem no WhatsApp.

No Chatwoot 4.16.2, API inbox não oferece retry automático confiável. Se um
webhook for reenviado manualmente ou por automação externa, a idempotência
garante que uma entrega concluída não seja duplicada; claims ainda em
processamento retornam erro reenviável, não um falso 200.

## Fluxo: cliente manda texto WhatsApp → agente recebe no Chatwoot

1. Evolution Go envia o evento de mensagem (`MESSAGE` ou `MESSAGES_UPSERT`) para a URL exclusiva da instância.
2. O conector valida o segredo no caminho em tempo constante e limita o body.
3. Aceita apenas texto direto recebido (`fromMe=false`); grupos, mídia e
   eventos desconhecidos retornam 200 sem efeito.
4. O core cria/reutiliza contato e vínculo da inbox, publica uma mensagem
   `incoming`, registra o mapeamento e conclui a idempotência/auditoria.
5. Falhas conhecidas do Chatwoot retornam 503 e liberam retry; timeout de rede
   mantém o claim para reduzir risco de duplicação.

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

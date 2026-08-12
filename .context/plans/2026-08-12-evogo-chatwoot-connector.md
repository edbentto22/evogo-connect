# EVOGO-CONNECT — Plano do Conector Evolution Go ↔ Chatwoot

**Data:** 2026-08-12
**Autor:** Mavis (Mavis / MiniMax Code)
**Status:** Investigação completa. Aguardando decisão de arquitetura para Etapa 1.

---

## 1. TL;DR

O **evolution-go** (sister project Go do evolution-api, Apache 2.0 + termos de marca)
**não tem conector nativo para Chatwoot** — só suporta webhook/WS/AMQP/NATS como saída
de eventos, e a REST API para enviar mensagens. O caminho mais limpo é um **serviço
conector standalone** que escuta webhooks do evolution-go, conversa com a API do
Chatwoot (canal API), e devolve as respostas dos agentes via evolution-go → WhatsApp.

Recomendação: **binário Go único + Docker**, deployável como sidecar dos dois lados.
Mesmo stack do evolution-go, baixa sobrecarga operacional, mesmo ferramental de
observabilidade.

---

## 2. Findings da investigação

### 2.1 evolution-go (repositório)

| Item | Valor |
|---|---|
| Repo | `github.com/evolution-foundation/evolution-go` |
| Linguagem | Go 1.24+ |
| Framework HTTP | Gin |
| WhatsApp lib | `whatsmeow` (Tulir) |
| Banco | PostgreSQL + GORM |
| Storage | MinIO/S3 opcional para mídia |
| Licença | Apache 2.0 + **cláusula de proteção de marca** (logo + Usage Notification obrigatório) |
| Ativação | Requer licença via `/manager/login` (heartbeat) |
| Porta padrão | 8080 |
| Auth da API | Header `apikey: <GLOBAL_API_KEY>` |

**Endpoints confirmados (do README + docs oficiais):**

```
POST /instance/create         — criar instância WhatsApp
GET  /instance/{name}/qrcode  — obter QR (pareamento)
POST /instance/connect        — conectar instância (aqui setamos webhook)
POST /instance/{name}/logout
POST /instance/{name}/restart
GET  /instance/{name}/status
DELETE /instance/{name}

POST /message/sendText        — {number, text}
POST /message/sendMedia       — {number, mediatype, media, fileName, ...}
```

**Eventos disponíveis (configuráveis no webhook):**

`APPLICATION_STARTUP`, `QRCODE_UPDATED`, `MESSAGES_SET`, `MESSAGES_UPSERT`,
`MESSAGES_UPDATE`, `MESSAGES_DELETE`, `SEND_MESSAGE`, `CONTACTS_SET`,
`CONTACTS_UPSERT`, `CONTACTS_UPDATE`, `PRESENCE_UPDATE`, `CHATS_SET`,
`CHATS_UPSERT`, `CHATS_UPDATE`, `CHATS_DELETE`, `GROUPS_UPSERT`, `GROUPS_UPDATE`,
`GROUP_PARTICIPANTS_UPDATE`, `CONNECTION_UPDATE`, `CALL`, `NEW_JWT_TOKEN`,
`NewsletterJoin`.

**Política de retry do webhook:** até 5 tentativas, 30s entre cada, exige HTTP 2xx.

**Formato de payload `MESSAGES_UPSERT` (do whatsmeow):**

```json
{
  "event": "MESSAGES_UPSERT",
  "instance": "minha-instancia",
  "data": {
    "key": {
      "remoteJid": "5511999999999@s.whatsapp.net",
      "fromMe": false,
      "id": "3EB0B430A3B8C3B1D4E2"
    },
    "pushName": "João da Silva",
    "message": { "conversation": "Olá, bom dia" },
    "messageType": "conversation",
    "messageTimestamp": 1698765432
  }
}
```

### 2.2 Chatwoot (self-hosted)

| Item | Valor |
|---|---|
| Canal de integração | **API Channel** (`channel.type=api`) |
| Auth | `api_access_token: <token>` (Profile → Access Token ou Platform App) |
| Webhooks | Recebe em `channel.webhook_url` quando há `message_created`/`conversation_*` |
| HMAC | Disponível via `Inbox.find(id).channel.hmac_token` (verificação opcional) |
| URL de agentes (real-time) | `/cable` (ActionCable) — fora do escopo, usamos webhook |

**Endpoints que vamos usar:**

```
POST   /api/v1/accounts/{account_id}/inboxes                       — criar inbox API
GET    /api/v1/accounts/{account_id}/inboxes                       — listar
POST   /api/v1/accounts/{account_id}/contacts                      — criar/obter contato
POST   /api/v1/accounts/{account_id}/conversations                 — criar conversa
POST   /api/v1/accounts/{account_id}/conversations/{id}/messages   — enviar msg
POST   /api/v1/accounts/{account_id}/conversations/{id}/toggle_status
POST   /api/v1/accounts/{account_id}/conversations/{id}/assignments
```

`contact.inbox.source_id` é o nosso **anchor estável** — sobrevive a renomeação do
contato e é a chave que linka a JID do WhatsApp (`5511999999999@s.whatsapp.net`).

### 2.3 O gap

A Evolution API (Node, irmã) **tem** conector Chatwoot via `CHATWOOT_*` env vars
criando a inbox automaticamente. A evolution-go **não** — eventos saem, mensagens
entram, mas não há orquestração com Chatwoot. O connector é o que falta.

---

## 3. Arquitetura — opções

### Opção A — Conector Go standalone (recomendado)

```
┌──────────────┐  webhooks   ┌──────────────────┐  REST   ┌──────────┐
│  evolution-  │ ──────────► │ evogo-connect    │ ──────► │ Chatwoot │
│     go       │             │ (Go binary)      │         │ (self-   │
│              │ ◄────────── │                  │ ◄────── │  hosted) │
└──────────────┘   send msg  └──────────────────┘ webhooks└──────────┘
                 (REST)        ▲
                               │ /admin (ops)
                               ▼
                         (PostgreSQL p/ idempotência + audit log)
```

**Prós:**
- Mesmo stack (Go) do evolution-go — reuso de libs, observabilidade, build
- Zero invasão nos dois lados — apenas configura webhook e cria inbox
- Imagem Docker pequena (`< 30 MB`)
- Idempotência e audit log no nosso lado (não dependemos do banco deles)
- Pode virar produto open-source (Apache 2.0 compatível com evolution-go)

**Contras:**
- Mais um container pra rodar
- Manutenção própria (mas pequena — bridge fino)

### Opção B — Sidecar Python (FastAPI)

**Prós:** ecossistema Python rico pra WhatsApp, Playwright pra coisas visuais
**Contras:** 2 stacks diferentes pra debugar, container maior, Mavis já é Go-friendly

### Opção C — AgentBot dentro do Chatwoot

**Prós:** vive dentro do Chatwoot, sem container extra
**Contras:** amarrado ao runtime Rails do Chatwoot, deploy mais invasivo,
pouco flexível

### Recomendação: **Opção A** (Go standalone)

Justificativas:
1. evolution-go já é Go — reaproveitamos `go.mod`, padrões, debug
2. Container único, deploy trivial (`docker compose up`)
3. Mantém o conector distribuível como projeto open-source independente
4. Sem mexer no código dos dois lados (least surprise)

---

## 4. Modelo de segurança (defense in depth)

| Camada | Mecanismo |
|---|---|
| **Auth evolution-go → connector** | Header customizado `X-Evogo-Secret` validado por HMAC; secret configurado por instância |
| **Auth Chatwoot → connector** | `X-Chatwoot-Signature` (HMAC-SHA256 do body, padrão Chatwoot) com tolerância de 5 min (replay protection) |
| **Auth connector → evolution-go** | `apikey: <GLOBAL_API_KEY>` em env (nunca logado) |
| **Auth connector → Chatwoot** | `api_access_token` por inbox, armazenado criptografado (AES-GCM) no Postgres com key em env |
| **Segredos em runtime** | Vault opcional, senão `.env` montado via Docker secret, NUNCA commitado |
| **TLS** | Obrigatório na borda (Caddy/Traefik) — connector escuta em HTTP puro atrás do proxy |
| **Rate limit** | Token bucket por `(inbox_id, direction)` — protege contra loops e DoS |
| **Idempotência** | Tabela `idempotency_keys` com TTL 24h — chave = `evo_message_id` ou `chatwoot_message_id` |
| **Audit log** | Toda mensagem bridged vai pra `bridge_log` (inbox, direction, payload_hash, status, ts) — LGPD-friendly |
| **PII mínima em logs** | Telefone mascarado (`55****9999`), `pushName` truncado, `content` hasheado em info-level |
| **PII zero em error logs** | Erros logam só IDs e códigos, nunca conteúdo |
| **Sandbox de mídia** | Validação de MIME + tamanho + magic bytes antes de repassar (anti-malware) |
| **Multi-tenancy** | Cada (evo-go instance ↔ chatwoot inbox) é um "tenant" isolado no nosso store |
| **Kill switch** | `POST /admin/pause` pausa o bridge sem reiniciar; retorna 503 pra webhooks |

---

## 5. Roadmap por etapas

Cada etapa termina com **commit, smoke test, e PR documente o que entrou**.

### Etapa 0 — Investigação & docs ✅ (esta etapa)
- Doc de findings (este arquivo)
- Decisão de arquitetura (aguardando Ed)
- Repo skeleton + AGENTS.md + CI mínimo

### Etapa 1 — Skeleton Go + reverse bridge (Chatwoot → WhatsApp)
**Escopo mínimo viável — foca em SAIR uma mensagem do agente.**

- Repo Go (`github.com/edbentto22/evogo-connect` ou similar) com `cmd/evogo-connect`
- Gin server, config via env, healthcheck, graceful shutdown
- **Endpoint inbound** `POST /webhook/chatwoot`:
  - Valida HMAC do header `X-Chatwoot-Signature`
  - Filtra só eventos `message_created` com `message_type=outgoing`
  - Extrai `conversation.id`, `message.content`, `message.attachments`
  - Resolve `chatwoot_inbox_id → evolution_instance_name` via store local
  - Resolve `chatwoot_contact.source_id` → JID do WhatsApp
  - Chama `POST /message/sendText` ou `/message/sendMedia` no evolution-go
  - Persiste `idempotency_key = chatwoot_message_id` (evita duplo envio)
  - Loga no `bridge_log`
- **Endpoint admin** `GET /healthz`, `GET /readyz`, `GET /metrics` (Prometheus)
- **CLI `connect setup`** que:
  - Cria inbox no Chatwoot (type=api, webhook apontando pro nosso `/webhook/chatwoot`)
  - Persiste `inbox_id`, `access_token`, `hmac_token`, `account_id`
  - Configura webhook no evolution-go (apontando pro `/webhook/evo/<instance>`)
- **Dockerfile** + `docker-compose.yml` com stack mínima
- **Testes:** unit (handlers com httptest) + E2E (subir evolution-go + chatwoot em compose, mandar msg)
- **Critério de aceite:**
  1. `docker compose up` sobe tudo
  2. `connect setup --instance demo --chatwoot-url ... --chatwoot-token ... --evo-url ...` cria inbox e webhook
  3. Agente responde no Chatwoot → mensagem chega no WhatsApp
  4. Reenvio do mesmo webhook Chatwoot (simulando retry) **não duplica** no WhatsApp

### Etapa 2 — Forward bridge (WhatsApp → Chatwoot)
**Foco em ENTRAR mensagem do cliente no Chatwoot.**

- **Endpoint inbound** `POST /webhook/evo/<instance>`:
  - Valida secret por instância
  - Filtra `MESSAGES_UPSERT` com `fromMe=false`
  - Resolve `inbox_id` da instância
  - **Get-or-create contact** no Chatwoot com `source_id = remoteJid` (telefone)
  - **Get-or-create conversation** (status=`open`)
  - **POST message** com `message_type=incoming`, content = `data.message.conversation`
  - Anexos: baixa mídia do evolution-go (URL assinada) e reenvia pro Chatwoot via multipart
- Idempotência: `evo_message_id` no `source_id` da mensagem Chatwoot (Chatwoot dedup natural)
- **Critério de aceite:**
  1. Cliente manda msg WhatsApp → aparece como `incoming` no Chatwoot
  2. Contato é criado/atualizado com nome do `pushName` e telefone do JID
  3. Mídia chega como anexo no Chatwoot

### Etapa 3 — Hardening & observabilidade
- Prometheus metrics: `bridge_messages_total{direction,status}`,
  `bridge_latency_seconds_bucket{direction}`, `bridge_errors_total{code}`
- Logs estruturados (slog) com trace ID por mensagem
- pprof endpoint atrás de auth
- Rate limiter (golang.org/x/time/rate) por inbox
- Documentação OpenAPI (swaggo)
- Testes de carga (k6) — 100 msg/s sustentado

### Etapa 4 — Contact sync avançado
- Avatar: download do `profilePicUrl` do whatsmeow → upload pro Chatwoot
- Nome: atualiza `pushName` no contato a cada msg
- Custom attributes no contato: `whatsapp_jid`, `whatsapp_lid`, `last_seen_at`

### Etapa 5 — Mídia rica
- Suporte completo a `imageMessage`, `audioMessage`, `videoMessage`,
  `documentMessage`, `stickerMessage`, `ptt` (voice note)
- Transcrição opcional de áudio (Whisper local) — feature flag

### Etapa 6 — Status updates & read receipts
- `MESSAGES_UPDATE` do evolution-go → `POST /messages/{id}/update` no Chatwoot
- Status `sent`/`delivered`/`read` refletidos na UI do agente

### Etapa 7 — Multi-instância & grupos (opcional)
- 1 connector atendendo N evolution-go instances × M chatwoot inboxes
- Suporte experimental a grupos WhatsApp (criar 1 conversation por grupo)

### Etapa 8 — Polish & release
- README com badges, GIF de demo, docker-compose full
- Helm chart
- Publicação no GitHub
- Trademark attribution: `Powered by Evolution Go` no README (cumpre a cláusula)

---

## 6. Layout proposto do repo

```
evogo-connect/
├── cmd/
│   └── evogo-connect/
│       └── main.go
├── internal/
│   ├── config/         # env loading (koanf)
│   ├── chatwoot/       # client + types
│   ├── evogo/          # client + types
│   ├── bridge/         # core: idempotência, audit, dispatch
│   ├── webhook/        # handlers inbound
│   ├── auth/           # HMAC verification
│   ├── store/          # Postgres (idempotência, audit, config)
│   ├── metrics/        # Prometheus
│   └── setup/          # CLI "connect setup"
├── migrations/         # SQL (golang-migrate)
├── deploy/
│   ├── docker-compose.yml
│   ├── Caddyfile
│   └── evogo-connect.dockerfile
├── docs/
│   ├── architecture.md
│   ├── security.md
│   └── operations.md
├── .context/           # docs internas (decisões, runbooks)
├── AGENTS.md
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 7. Pendências / decisões pra confirmar com Ed

1. **Arquitetura:** confirmar Opção A (Go standalone) — ou preferir B/C?
2. **Naming do repo:** `evogo-connect` (proposto) ou outro? `evolution-go-chatwoot-bridge`?
3. **Onde publicar:** GitHub pessoal (`edbentto22`) ou org separada?
4. **Licença do connector:** Apache 2.0 (mesma do evolution-go) ou MIT?
5. **Banco do connector:** Postgres separado ou reusar o do evolution-go?
   (Recomendo **separado** — concern de isolamento; evolution-go já tem o dele)
6. **Plano de testes E2E:** podemos subir chatwoot+evolution-go em compose local pra dev?
7. **Escopo do Etapa 1:** só reverse bridge (agente → WhatsApp) ou já incluir o forward?
   (Recomendo só reverse na Etapa 1 — primeiro fluxo fechado, depois amplia)

---

## 8. Riscos & mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| evolution-go mudar formato de webhook | Bridge quebra | Pinned version, suite de contract tests, monitoramento de schema |
| Chatwoot mudar `X-Chatwoot-Signature` HMAC | Bridge quebra | Pin version do Chatwoot no compose, validação tolerante |
| License do evolution-go exigirUsage Notification | Atraso no release | Adicionar attribution desde Etapa 0 (atende já a cláusula) |
| Loop de mensagens (bridge reenvia msg que ele mesmo criou) | Duplicação | `source_id` da mensagem Chatwoot contém `evo_message_id`; ignorar no forward bridge |
| WhatsApp ban por uso de whatsmeow | Conta do cliente bloqueada | Documentar risco, sugerir WhatsApp Business oficial pra alto volume |
| Memory leak em conexões whatsmeow | evolution-go reinicia | Não é nosso problema; mas monitorar `connection.update` no Etapa 6 |

---

## 9. Próximo passo

Após Ed confirmar a arquitetura e escopo da Etapa 1:
1. Criar repo local `evogo-connect/`
2. Skeleton Go (`cmd/evogo-connect/main.go` + Gin + healthz)
3. Implementar **só** o reverse bridge (Etapa 1)
4. Smoke E2E com Chatwoot + evolution-go reais
5. Commit + PR + doc

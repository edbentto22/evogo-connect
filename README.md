# evogo-connect

> Bridge standalone entre **[evolution-go](https://github.com/evolution-foundation/evolution-go)**
> (API WhatsApp) e **[Chatwoot](https://www.chatwoot.com/)** (plataforma de
> atendimento self-hosted).

[![Status](https://img.shields.io/badge/status-Etapa%201-blue)](#roadmap)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8)](#stack)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue)](LICENSE)

A [evolution-go](https://github.com/evolution-foundation/evolution-go) (a versão
Go da Evolution API, mantida pela Evolution Foundation) **não tem conector
nativo para o Chatwoot** — diferente da sua irmã Node, a Evolution API. O
`evogo-connect` preenche esse gap: um binário Go único que escuta webhooks do
Chatwoot e fala com a REST API do evolution-go (e vice-versa, nas próximas
etapas).

## Status atual — Etapa 1

- ✅ **Reverse bridge (Chatwoot → WhatsApp):** mensagem que o agente envia no
  Chatwoot é enviada ao WhatsApp, com idempotência atômica, audit log e HMAC.
- ✅ Contratos automatizados homologados para **Evolution Go 0.7.2** e
  **Chatwoot 4.16.2**.
- ✅ Pacote de deploy candidato a produção para **Coolify**, com Postgres
  privado, segredos persistentes e healthchecks.
- ⏳ Forward bridge (WhatsApp → Chatwoot) — Etapa 2.
- ⏳ Smoke E2E em VPS com instâncias reais — gate antes de liberar tráfego.
- ⏳ Mídia rica, status updates, grupos — Etapas 5-7.

O código da Etapa 1 possui testes de contrato, retry e concorrência. Para
produção, use `deploy/docker-compose.coolify.yml` e siga `docs/coolify.md`.
Antes de liberar uma VPS para tráfego real, execute `scripts/smoke-e2e.sh`
contra as instâncias que serão usadas. O compose `deploy/docker-compose.yml`
permanece uma referência local.

Ver `.context/plans/2026-08-12-evogo-chatwoot-connector.md` para o roadmap
completo e `.context/research/` para a investigação.

## Por que este projeto existe

- A Evolution API (Node) tem `CHATWOOT_*` env vars que criam a inbox
  automaticamente. A evolution-go não.
- Existem conectores de terceiros para a Evolution API, mas nenhum focado em
  evolution-go + self-hosted, com audit log e segurança de produção.
- O [trademark](TRADEMARKS.md) da Evolution Foundation exige atribuição;
  este projeto respeita isso.

## Stack

- Go 1.24+, Gin, pgx/v5, Cobra, slog, Prometheus, envconfig
- Postgres 16+ (idempotência, audit, config)
- Docker / docker-compose para deploy
- Mesma família de stack do evolution-go (Go + Postgres) — pouca curva

## Quick start

```bash
# 1. Clonar
git clone https://github.com/edbentto22/evogo-connect.git
cd evogo-connect

# 2. Subir Postgres
docker compose -f deploy/docker-compose.yml up -d postgres

# 3. Copiar e editar .env
cp .env.example .env
# editar CONNECT_MASTER_KEY, DATABASE_URL, ADMIN_TOKEN

# 4. Rodar migrations
make migrate-up

# 5. Build
make build

# 6. Subir o servidor
./bin/evogo-connect

# 7. Em outro terminal, provisionar a primeira inbox
# Defina CHATWOOT_TOKEN e EVO_INSTANCE_TOKEN no ambiente deste terminal.
./bin/connect setup --name demo \
  --chatwoot-url https://chatwoot.example.com \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-instance demo \
  --connect-url http://localhost:9090

# 8. Adicionar um contato (JID WhatsApp → contato Chatwoot)
./bin/connect add-contact --tenant demo \
  --jid 5511999999999@s.whatsapp.net \
  --name "João da Silva"
```

Pronto. Quando um agente responder no Chatwoot, a mensagem chega no WhatsApp
do João via evolution-go.

## Compatibilidade

| Componente | Versão homologada | Observação |
|---|---:|---|
| Evolution Go | 0.7.2 | `/send/text`, `/send/media`, `/instance/status`; token individual da instância |
| Chatwoot | 4.16.2 | API inbox com `secret` e assinatura `timestamp.body` |
| PostgreSQL | 16+ | Persistência, idempotência e auditoria |

Outras versões não estão bloqueadas artificialmente, mas precisam passar pelo
smoke E2E antes de uso em produção.

## Arquitetura

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

Detalhes em `docs/architecture.md`.

## Segurança

- HMAC do webhook Chatwoot com janela anti-replay
- Tokens armazenados AES-GCM criptografados no Postgres
- Idempotência por `message_id` (evita duplicação em retries)
- PII nunca em logs (telefone mascarado, content hasheado)
- Audit log completo em `bridge_log`
- Kill switch via `POST /admin/pause`

O webhook Evolution Go → Chatwoot e sua autenticação serão adicionados na
Etapa 2; eles não são anunciados nem configurados nesta entrega.

Modelo completo em `docs/security.md`.

## Operações

Ver `docs/coolify.md` para deploy de produção e `docs/operations.md` para
health, métricas e troubleshooting.

## License

Apache 2.0 (mesma do evolution-go). Ver [LICENSE](LICENSE).

## Trademarks

"Evolution Foundation", "Evolution" e "Evolution Go" são marcas registradas da
Evolution Foundation. Este projeto é independente e não afiliado oficialmente.
Powered by Evolution Go.

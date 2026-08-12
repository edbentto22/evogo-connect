# AGENTS.md

Convenções que qualquer agente de IA (ou humano) deve seguir ao trabalhar neste repo.

## Projeto

**`evogo-connect`** é um conector standalone em Go entre o **evolution-go** (WhatsApp
API, Go + whatsmeow) e o **Chatwoot** (plataforma de atendimento self-hosted).
Documentação viva em `.context/` e `docs/`. Plano de etapas em
`.context/plans/2026-08-12-evogo-chatwoot-connector.md`.

## Stack

- Go 1.24+
- Gin (HTTP), pgx/v5 (Postgres), Cobra (CLI), slog (logging)
- Prometheus (métricas), envconfig (config), golang-migrate (schema)
- Postgres 16+ como store (idempotência, audit, config)

## Layout

```
cmd/
  evogo-connect/      # servidor HTTP principal
  connect-cli/        # CLI de setup (connect setup / add-contact / status)
internal/
  config/             # envconfig (Config struct)
  log/                # slog setup (PII masking)
  crypto/             # AES-GCM helpers
  store/              # pgxpool + queries (tenants, contact_map, idempotency, bridge_log)
  chatwoot/           # client + types + HMAC verify
  evogo/              # client + types
  bridge/             # core dispatch (resolve tenant → contact → JID → send)
  httpapi/            # gin router + handlers (webhook_chatwoot, admin, metrics, health)
  metrics/            # Prometheus collectors
migrations/           # SQL files (golang-migrate)
deploy/               # Dockerfile, docker-compose, Caddyfile
docs/                 # security.md, operations.md, architecture.md
```

## Regras de código

- **Mimic existing patterns.** Antes de criar arquivo novo, olhe os vizinhos.
- **Erros:** sempre com contexto (`fmt.Errorf("action: %w", err)`); nunca descartar.
- **Logs:** usar `slog` (nunca `fmt.Println` em prod). PII sempre mascarada
  (telefone, pushName, content). `error` level só pra erros de operação.
- **Secrets:** NUNCA em código, NUNCA em logs. Sempre via env. Storage no DB é
  AES-GCM criptografado com `CONNECT_MASTER_KEY`.
- **Tests:** arquivos `*_test.go` ao lado do código. Mínimo 1 teste por
  componente novo. Usar `testify/assert` + `httptest` (sem framework externo).
- **Naming:** pacotes lowercase single-word; tipos PascalCase; funções
  camelCase; constantes SCREAMING_SNAKE.
- **Imports:** stdlib primeiro, depois externos, depois internal — separados
  por linha em branco.
- **go.mod:** manter limpo. `go mod tidy` antes de qualquer commit.
- **Idempotência:** qualquer handler que produza efeito externo (envio de
  mensagem) DEVE checar `idempotency_keys` antes.

## Regras de segurança (defense in depth)

Ver `docs/security.md` para o modelo completo. Resumo operacional:

1. **HMAC bidirecional:** webhook Chatwoot validado por `X-Chatwoot-Signature`;
   webhook evolution-go validado por `X-Evogo-Secret` (Etapa 2).
2. **Constant-time compare** em qualquer verificação de HMAC.
3. **Replay window** de 5 minutos (timestamp no header evolution-go).
4. **PII nunca em logs:** telefone `55****9999`, pushName truncado, content
   hasheado em info-level.
5. **TLS obrigatório** na borda (Caddy/Traefik). Connector escuta em HTTP puro.
6. **Rate limit** por inbox (golang.org/x/time/rate).
7. **Kill switch** em `POST /admin/pause` (auth via header `X-Admin-Token`).
8. **Audit log** completo em `bridge_log` (sem conteúdo, só IDs e hashes).

## Comandos essenciais

```bash
# Build
go build ./...
go vet ./...
go test ./...

# Run local (com Postgres via docker-compose)
docker compose -f deploy/docker-compose.yml up -d postgres
make migrate-up
make run-server

# CLI
./bin/connect setup --name demo \
  --chatwoot-url https://cw.example.com \
  --chatwoot-token <token> \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-key <key> \
  --connect-url http://localhost:9090
```

## Antes de abrir PR

- [ ] `go build ./...` verde
- [ ] `go vet ./...` verde
- [ ] `go test ./...` verde (suite existente + novo teste)
- [ ] `gofmt -l .` vazio
- [ ] Migration nova em `migrations/` com `.up.sql` e `.down.sql`
- [ ] Doc atualizado (README / docs/) se mudou contrato externo
- [ ] Sem secrets commitados (checar `git diff` antes de push)

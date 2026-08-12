# Security model — evogo-connect

> Defense in depth. Cada camada cobre um vetor de ataque diferente.
> Toda decisão abaixo é registrada aqui pra audit e revisão.

## 1. Autenticação bidirecional

| Sentido | Mecanismo |
|---|---|
| Chatwoot → evogo-connect | Header `X-Chatwoot-Signature` comparado com `tenants.chatwoot_hmac` (constant-time). Modo `plain` (default) ou `hmac` (HMAC-SHA256 do body). |
| evogo-connect → evolution-go | Header `apikey: <GLOBAL_API_KEY>` por tenant (env não-global, escopado). |
| evolution-go → evogo-connect (Etapa 2) | Header `X-Evogo-Secret` validado por tenant; replay window 5 min. |
| Admin (`/admin/*`) | Header `X-Admin-Token` (config). Token NUNCA em log. |

**Constant-time compare** em qualquer verificação de assinatura (`hmac.Equal`).

## 2. Segredos em repouso

Tokens persistidos no Postgres são cifrados com **AES-256-GCM**:
- Chave: `CONNECT_MASTER_KEY` (32 bytes base64, env, NUNCA commitada).
- Nonce aleatório de 12 bytes por registro.
- Formato: `nonce || ciphertext || tag (16 bytes)`.

```sql
SELECT pg_typeof(chatwoot_token_enc) FROM tenants;
-- bytea (opaco)
```

A chave mestra é carregada uma vez na inicialização; não há API pra
exportá-la. Em produção, montar via Docker secret / Vault.

## 3. PII em logs

- **Telefone (JID):** sempre mascarado (`55****9999`) via `log.MaskPhone`.
- **pushName:** truncado em 32 chars + hash curto (`TruncName`).
- **Conteúdo de mensagem:** **NUNCA** logado em info; apenas SHA256 hex
  (`ContentHash`) para correlação entre bridge_log e audit externo.
- **Tokens:** NUNCA logados, mesmo em error.

Regra aplicada via `log/slog` (handler global) e `With("jid_masked", ...)`.

## 4. Idempotência

Toda chamada que produz efeito externo (envio de mensagem) é deduplicada
por chave `(direction, key)`:
- `key = c2w:<tenant>:<chatwoot_message_id>` (Etapa 1)
- `key = w2c:<tenant>:<evo_message_id>` (Etapa 2, futuro)

TTL: `IDEMPOTENCY_TTL` (default 24h). Chave expirada é sobrescrita.

## 5. Audit log

Tabela `bridge_log` — **sem conteúdo**:
- IDs externos (chatwoot_message_id, evo_message_id)
- Direção (`c2w` / `w2c`)
- JID (texto, mas só pra auditoria interna; nunca expor)
- SHA256 do payload (correlação)
- Status, error_code, latency_ms
- Timestamp

Sem nomes, sem telefone completo, sem conteúdo. Permite reconstruir a
trilha sem armazenar PII.

## 6. Rate limit

Por `(tenant_id, direction)` — token bucket via `golang.org/x/time/rate`.
Default 120 msg/min. Excedente → 429 + log + métrica.

## 7. Kill switch

- Via env: `BRIDGE_PAUSED=true` na inicialização.
- Via DB: `INSERT INTO bridge_paused VALUES (1, ...)`. Toggle on-the-fly
  via `POST /admin/pause` (header `X-Admin-Token`).

Quando pausado, webhooks do Chatwoot retornam **503** (Chatwoot retenta).

## 8. Tamanho máximo de body

Webhook do Chatwoot tem `io.LimitReader(c.Request.Body, 5*1024*1024)` (5 MB).
Acima disso → 400 Bad Request antes de processar.

## 9. TLS na borda

O connector escuta em **HTTP puro** (assume TLS terminado no proxy).
Em produção: **sempre** usar Caddy/Traefik/ALB com TLS válido.

Configuração Caddy de exemplo em `deploy/Caddyfile`.

## 10. Timeouts

- HTTP server: read 15s, write 30s.
- Webhook handler: ctx 25s (gera métricas se demorar).
- Evolution-go client: 30s timeout por request.
- Chatwoot client: 30s timeout por request.

## 11. Audit checklist de release

Antes de subir pra produção, validar:

- [ ] `CONNECT_MASTER_KEY` gerado (`openssl rand -base64 32`)
- [ ] `ADMIN_TOKEN` gerado (`openssl rand -hex 32`) e NUNCA em git
- [ ] Postgres com TLS (`sslmode=require` ou `verify-full`)
- [ ] TLS válido no Caddy/Traefik (Let's Encrypt via domínio real)
- [ ] `BRIDGE_PAUSED=false` confirmado
- [ ] Backups do Postgres configurados
- [ ] Métricas Prometheus scrapeando `/metrics`
- [ ] Audit log acessível apenas para admin
- [ ] Sem PII em logs (rodar `grep` em busca de telefone em produção)
- [ ] Trademarks: `Powered by Evolution Go` visível no README (cláusula da license)

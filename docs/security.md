# Security model — evogo-connect

> Defense in depth. Cada camada cobre um vetor de ataque diferente.
> Toda decisão abaixo é registrada aqui pra audit e revisão.

## 1. Autenticação bidirecional

| Sentido | Mecanismo |
|---|---|
| Chatwoot → evogo-connect | `X-Chatwoot-Signature: sha256=<digest>` validado em tempo constante sobre `X-Chatwoot-Timestamp + "." + body`, com janela de 5 minutos. |
| evogo-connect → Evolution Go | Header `apikey` recebe o token individual da instância; a chave global não é aceita no fluxo de envio. |
| evolution-go → evogo-connect | Segredo aleatório por instância no caminho da URL, validado em tempo constante. A Evolution Go 0.7.2 não oferece header secreto configurável; acesso aos logs do proxy deve ser restrito. |
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
exportá-la. Em plataformas com suporte do aplicativo, prefira Docker secret ou
Vault. O pacote Coolify atual usa variável marcada **somente runtime**, com
injeção de build args desabilitada, porque o binário ainda lê configuração por
env.

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

O claim `processing` é adquirido atomicamente antes do envio. Duplicatas
concorrentes não executam o efeito externo. Uma falha de envio muda o claim
para `failed`, permitindo retry; uma entrega concluída fica terminal como
`sent` até o TTL.

## 5. Audit log

Tabela `bridge_log` — **sem conteúdo**:
- IDs externos (chatwoot_message_id, evo_message_id)
- Direção (`c2w` / `w2c`)
- JID mascarado (nunca o valor completo)
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

Quando pausado, webhooks do Chatwoot retornam **503**. Em API inbox do
Chatwoot 4.16.2, isso pode marcar a mensagem como `failed` sem retry automático;
o operador precisa reenviá-la após retomar o bridge.

## 8. Tamanho máximo de body

Webhook do Chatwoot usa `http.MaxBytesReader` com limite de 5 MB.
Acima disso → 400 Bad Request antes de processar.

## 9. TLS na borda

O connector escuta em **HTTP puro** (assume TLS terminado no proxy).
Em produção no Coolify, **sempre** associar um domínio HTTPS ao serviço
`connector` na porta interna `9090`; o proxy gerenciado pelo Coolify termina o
TLS. Não inclua Caddy ou Traefik no compose de produção.

O Caddy em `deploy/Caddyfile` existe somente para desenvolvimento local.

## 10. Timeouts

- HTTP server: read 15s, write 30s.
- Webhook handler: ctx 25s (gera métricas se demorar).
- Evolution-go client: 30s timeout por request.
- Chatwoot client: 30s timeout por request.

## 11. Audit checklist de release

Antes de subir pra produção, validar:

- [ ] Magic variables do Coolify geradas, persistidas e copiadas para um cofre
- [ ] `Inject Build Args to Dockerfile` desativado e segredos somente runtime
- [ ] `CONNECT_MASTER_KEY` original incluída no plano de disaster recovery
- [ ] Postgres sem porta publicada e isolado na rede interna do stack
- [ ] TLS válido no proxy do Coolify e domínio apontando para a porta interna 9090
- [ ] Primeiro deploy com `BRIDGE_PAUSED=true`; `false` somente durante o smoke/liberação
- [ ] Backups do Postgres configurados
- [ ] Restore testado com um backup e a chave mestra original
- [ ] Métricas Prometheus scrapeando `/metrics`
- [ ] Audit log acessível apenas para admin
- [ ] Sem PII em logs (rodar `grep` em busca de telefone em produção)
- [ ] Trademarks: `Powered by Evolution Go` visível no README (cláusula da license)

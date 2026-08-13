# Guia de uso — evogo-connect

> **O que este conector faz (Etapa 1 — já funcionando):**
> Quando um agente responde uma conversa no Chatwoot, a mensagem chega
> no WhatsApp do cliente via evolution-go. Idempotente, auditado, com HMAC.

---

## 1. Pré-requisitos

| Componente | Versão | Por quê |
|---|---|---|
| Go | 1.25+ | Compilar os binários |
| Docker + docker compose | 24+ | Postgres + deploy |
| PostgreSQL | 16+ | Idempotência, audit log, config |
| Chatwoot | 3.x+ self-hosted | Caixa de entrada API + UI do agente |
| evolution-go | latest | Provedor WhatsApp (roda standalone) |

> **Não precisa** de Redis, Kafka, nem nada extra. É um único binário Go + Postgres.

---

## 2. Setup local (do zero, em 5 min)

### 2.1. Clonar e preparar

```bash
git clone https://github.com/edbentto22/evogo-connect.git
cd evogo-connect

# Variáveis de ambiente obrigatórias
cp .env.example .env

# Gerar chave mestra AES (32 bytes)
echo "CONNECT_MASTER_KEY=$(openssl rand -base64 32)" >> .env

# Gerar admin token
echo "ADMIN_TOKEN=$(openssl rand -hex 32)" >> .env

# Editar DATABASE_URL se necessário (default localhost:5432)
```

### 2.2. Subir Postgres

```bash
docker compose -f deploy/docker-compose.yml up -d postgres
# Aguarda ~5s até o healthcheck passar
```

### 2.3. Build + migrations

```bash
make build        # gera bin/evogo-connect e bin/connect
make migrate-up   # cria as 5 tabelas
```

> O `migrate-up` é automático quando você roda o `evogo-connect` — o
> binário aplica as migrations na inicialização. O `make migrate-up` é só
> pra rodar manualmente se preferir.

### 2.4. Subir o servidor

```bash
./bin/evogo-connect
# → {"time":"...","level":"INFO","msg":"evogo-connect starting",...}
# → {"time":"...","level":"INFO","msg":"connected to postgres"}
# → {"time":"...","level":"INFO","msg":"migrations applied"}
# → {"time":"...","level":"INFO","msg":"http server listening","addr":":9090"}
```

Em outro terminal, valide:

```bash
curl -s http://localhost:9090/healthz   # {"status":"ok"}
curl -s http://localhost:9090/readyz    # {"status":"ready"}
curl -s http://localhost:9090/metrics | head -5   # métricas Prometheus
```

---

## 3. Provisionar o primeiro tenant (Chatwoot + evolution-go)

> **Você precisa ter em mãos:**
> - URL do Chatwoot (ex: `https://chatwoot.empresa.com`)
> - `api_access_token` do Chatwoot (Profile → Access Token no Chatwoot)
> - ID da account do Chatwoot (default `1`)
> - URL do evolution-go (ex: `http://localhost:8080`)
> - `GLOBAL_API_KEY` do evolution-go (env dele)
> - Nome da instância no evolution-go (ex: `demo`, pareada com QR)
> - URL pública deste connector (ex: `https://evogo.empresa.com` — pra
>   onde o Chatwoot vai mandar webhooks)

```bash
./bin/connect setup \
  --name demo \
  --chatwoot-url https://chatwoot.empresa.com \
  --chatwoot-token "$CW_TOKEN" \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-key "$EVO_KEY" \
  --evo-instance demo \
  --connect-url https://evogo.empresa.com
```

Saída esperada:

```
✓ Inbox criada no Chatwoot: id=7
✓ Webhook configurado no evolution-go: https://evogo.empresa.com/webhook/evo/demo
✓ Tenant 'demo' registrado (id=...)

Próximo passo — adicione um contato:
  connect add-contact --tenant demo --jid 5511999999999@s.whatsapp.net --name "João"
```

O que aconteceu:
1. **Inbox API no Chatwoot** foi criada com `webhook_url = https://evogo.empresa.com/webhook/chatwoot` e HMAC obrigatório.
2. **Webhook do evolution-go** foi configurado pra apontar pra `https://evogo.empresa.com/webhook/evo/demo` com eventos `MESSAGES_UPSERT`, `MESSAGES_UPDATE`, `CONNECTION_UPDATE`.
3. **Tenant persistido** no Postgres (tokens cifrados com `CONNECT_MASTER_KEY`).

---

## 4. Adicionar contatos

> Em produção (Etapa 2), o próprio forward bridge vai criar contatos
> automaticamente quando o cliente mandar a primeira mensagem WhatsApp.
> Por enquanto (Etapa 1), você precisa adicionar manualmente:

```bash
./bin/connect add-contact \
  --tenant demo \
  --jid 5511999999999@s.whatsapp.net \
  --name "João da Silva"
```

Isso cria o contato no Chatwoot (com `source_id = JID`) e salva o
mapeamento no `contact_map`. Pronto — o agente já pode responder e a
mensagem vai chegar no WhatsApp do João.

### Adicionar mais contatos

Repita o comando pra cada JID que você quer atender. Pra listar:

```bash
./bin/connect list    # lista tenants
```

(TODO Etapa 5: comando `connect list-contacts` por tenant.)

---

## 5. Testar end-to-end

### 5.1. Caminho feliz (manual)

1. Abra o **Chatwoot** no browser
2. Settings → Inboxes → `evogo-connect/demo` → confirme que o webhook
   aponta pra `https://evogo.empresa.com/webhook/chatwoot`
3. Sidebar → Conversations → "New conversation" → selecione o contato
   "João da Silva" na inbox `evogo-connect/demo`
4. Como agente, envie: `Olá João, teste do evogo-connect`
5. Verifique:
   - **WhatsApp do João** recebeu a mensagem
   - `curl -s https://evogo.empresa.com/admin/tenants -H "X-Admin-Token: $TOKEN"` lista o tenant
   - Logs do `evogo-connect` mostram `bridge: c2w delivered`

### 5.2. Validar idempotência (simulando retry do Chatwoot)

O Chatwoot retenta webhooks em 5x se a resposta for != 2xx. Nosso
conector é idempotente — duplicatas são descartadas.

```bash
# Disparar o mesmo webhook 3 vezes (mesmo chatwoot_message_id)
WEBHOOK_BODY='{"event":"message_created","message_type":"outgoing","id":99999,...}'
for i in 1 2 3; do
  curl -X POST https://evogo.empresa.com/webhook/chatwoot \
    -H "Content-Type: application/json" \
    -H "X-Chatwoot-Signature: $HMAC_TOKEN" \
    -d "$WEBHOOK_BODY"
done

# A 1ª entrega envia. A 2ª e 3ª caem em idempotência.
# Métrica idempotency_hits_total incrementa em 2.
curl -s https://evogo.empresa.com/metrics | grep idempotency
```

### 5.3. Validar kill switch

```bash
# Pausar
./bin/connect pause --reason "manutenção"
# → ✓ Bridge pausado

# Tentar responder como agente → WhatsApp NÃO recebe (webhook retorna 503)
# Chatwoot retenta 5x em 30s, todos falham — não envia

# Verificar
./bin/connect status
# → Estado: PAUSADO (kill switch ativo)

# Retomar
./bin/connect resume
# → ✓ Bridge rodando
```

### 5.4. Verificar audit log

```bash
psql postgres://connect:connect@localhost:5432/evogo_connect \
  -c "SELECT created_at, direction, status, jid, latency_ms
      FROM bridge_log ORDER BY created_at DESC LIMIT 10;"

# ou via CLI:
./bin/connect status   # mostra últimas 5 entradas
```

---

## 6. Deploy em produção

### 6.1. Stack completa (docker-compose)

```bash
# No servidor:
git clone https://github.com/edbentto22/evogo-connect.git
cd evogo-connect

cp deploy/.env.example .env
# Editar .env com chaves reais
echo "CONNECT_MASTER_KEY=$(openssl rand -base64 32)" >> .env
echo "ADMIN_TOKEN=$(openssl rand -hex 32)" >> .env

# Subir tudo
docker compose -f deploy/docker-compose.yml up -d

# Verificar
curl -s https://evogo.empresa.com/readyz
```

O compose sobe:
- **postgres** (porta 5432, volume persistente)
- **caddy** (porta 8080, proxy reverso com TLS)
- **connector** (porta interna 9090, atrás do Caddy)

### 6.2. Configurar domínio + TLS

Edite `deploy/Caddyfile` e troque `:8080` pelo seu domínio:

```caddyfile
evogo.empresa.com {
    reverse_proxy connector:9090
    encode gzip zstd
    log
}
```

Caddy gera TLS automático via Let's Encrypt se o domínio apontar pro
servidor. Pra dev local, mantenha `:8080`.

### 6.3. Backup

```bash
# Diário
pg_dump -Fc evogo_connect > backup-$(date +%F).dump

# Restore
pg_restore -d evogo_connect backup-2026-08-13.dump
```

Crítico: a tabela `tenants` (contém chaves cifradas). Sem ela, todo o
bridge precisa ser reprovisionado.

### 6.4. Upgrade

```bash
docker compose -f deploy/docker-compose.yml pull
docker compose -f deploy/docker-compose.yml up -d
# Migrations rodam automaticamente na inicialização.
```

---

## 7. Operação do dia-a-dia

### Comandos úteis

| Comando | O que faz |
|---|---|
| `./bin/connect list` | Lista tenants configurados |
| `./bin/connect status` | Estado do bridge + últimas 5 do audit log |
| `./bin/connect pause --reason "..."` | Mata o bridge (Chatwoot retenta) |
| `./bin/connect resume` | Liga o bridge |
| `curl /admin/tenants -H "X-Admin-Token: ..."` | Lista via API (debug) |
| `curl /admin/paused` | Status do kill switch |
| `curl /metrics` | Métricas Prometheus |

### Métricas Prometheus (importantes)

| Métrica | Quando alertar |
|---|---|
| `bridge_messages_total{status="error"}` | taxa > 0.5/s por 5min |
| `bridge_latency_seconds_bucket` | p95 > 5s |
| `bridge_errors_total{code="hmac_invalid"}` | > 0 (spoof em curso) |
| `idempotency_hits_total` | aumento súbito (loop?) |
| `bridge_messages_total{status="rate_limited"}` | recorrente (subir limite) |

Exemplo de scrape config (Prometheus):

```yaml
scrape_configs:
  - job_name: 'evogo-connect'
    static_configs:
      - targets: ['evogo.empresa.com:9090']
    bearer_token: ''  # /metrics é público por design (rede interna)
```

### Logs

- Formato JSON (default) ou text (LOG_FORMAT=text).
- PII nunca em logs — telefone sempre mascarado, content hasheado.
- Exemplo de linha:

```json
{"time":"2026-08-13T15:30:00Z","level":"INFO","msg":"bridge: c2w delivered",
 "event":"message_created","message_id":12345,"tenant":"demo",
 "jid_masked":"55****9999","content_hash":"a3f5...","latency_ms":234}
```

---

## 8. Troubleshooting

### "Agente respondeu, mas WhatsApp não recebeu"

1. `./bin/connect status` → bridge tá rodando?
2. `curl /admin/paused -H "X-Admin-Token: ..."` → 0?
3. Audit log tem erro?
   ```sql
   SELECT * FROM bridge_log WHERE status='error' ORDER BY created_at DESC LIMIT 5;
   ```
4. evolution-go tá conectado? `curl -H "apikey: $EVO_KEY" http://evo:8080/instance/status/demo`
5. Webhook do evolution-go configurado? `curl -H "apikey: $EVO_KEY" http://evo:8080/webhook/find/demo`
6. Teste manual:
   ```bash
   curl -X POST http://evo:8080/message/sendText/demo \
     -H "apikey: $EVO_KEY" -H "Content-Type: application/json" \
     -d '{"number":"5511999999999","text":"teste"}'
   ```

### "Chatwoot retorna 401 no webhook"

1. `X-Chatwoot-Signature` confere com o token gravado no DB?
2. Conferir token no Chatwoot: `GET /api/v1/accounts/{id}/inboxes/{inbox_id}` → campo `channel.hmac_token`.
3. Comparar com `tenants.chatwoot_hmac_enc` (decifrado em runtime).
4. Métrica: `bridge_errors_total{code="hmac_invalid"}` deve estar > 0.

### "Postgres crescendo muito"

Tabela `idempotency` tem TTL 24h. Limpeza manual:

```sql
DELETE FROM idempotency WHERE expires_at < now();
DELETE FROM bridge_log WHERE created_at < now() - interval '90 days';
```

Recomendado: pg_cron para automatizar (Etapa 6).

### "Como adicionar mais um número?"

```bash
# Repita o setup com outro nome
./bin/connect setup --name loja2 \
  --chatwoot-url https://chatwoot.empresa.com \
  --chatwoot-token "$CW_TOKEN" \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-key "$EVO_KEY" \
  --evo-instance loja2 \
  --connect-url https://evogo.empresa.com

./bin/connect add-contact --tenant loja2 \
  --jid 5511888888888@s.whatsapp.net --name "Maria"
```

Cada tenant = 1 inbox Chatwoot + 1 instância evolution-go. Suporta N
tenants em paralelo (multi-empresa).

---

## 9. Limites da Etapa 1

- **Reverse only** — agente → WhatsApp funciona. Cliente → Chatwoot (forward)
  entra na Etapa 2.
- **Mídia:** só o primeiro anexo da mensagem é enviado (Etapa 5 amplia pra
  múltiplos anexos + transcrição de áudio).
- **Contactos:** precisa adicionar manualmente via `connect add-contact` até
  a Etapa 2 (que vai auto-criar).
- **Grupos WhatsApp:** fora de escopo (Etapa 7).
- **Status updates** (delivered/read): fora de escopo (Etapa 6).

---

## 10. Próximos passos (Etapas 2+)

- **Etapa 2** — Forward bridge: cliente manda WhatsApp → recebe no Chatwoot
  como `incoming`. Auto-cria contato no Chatwoot. Resolve o TODO de
  `connect add-contact` manual.
- **Etapa 3** — Hardening: testes de carga, OpenAPI, pprof, exemplos
  de dashboards Grafana.
- **Etapa 5** — Mídia rica: múltiplos anexos, voice notes com transcrição
  opcional (Whisper local), stickers.
- **Etapa 6** — Status updates: MESSAGES_UPDATE do evolution-go refletido
  no Chatwoot (delivered/read).

Quando você terminar de validar a Etapa 1 (rodar o smoke E2E com Chatwoot
+ evolution-go reais), me avisa que a gente segue pra Etapa 2.

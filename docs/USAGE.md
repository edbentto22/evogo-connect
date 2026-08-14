# Guia de uso — evogo-connect

> **O que este conector faz:** integra os dois sentidos: respostas de agentes
> no Chatwoot chegam ao WhatsApp e textos recebidos no WhatsApp aparecem como
> mensagens incoming na inbox correta. Ambos são idempotentes e auditados.

---

## 1. Pré-requisitos

| Componente | Versão | Por quê |
|---|---|---|
| Go | 1.25+ | Compilar os binários |
| Docker + docker compose | 24+ | Postgres + deploy |
| PostgreSQL | 16+ | Idempotência, audit log, config |
| Chatwoot | 4.16.2 | Versão homologada da inbox API e assinatura de webhook |
| Evolution Go | 0.7.2 | Versão homologada das rotas `/send/*` |

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
> - Token individual da instância Evolution Go (não a chave global)
> - Nome da instância no evolution-go (ex: `demo`, pareada com QR)
> - URL pública deste connector (ex: `https://evogo.empresa.com` — pra
>   onde o Chatwoot vai mandar webhooks)

```bash
# CW_TOKEN e EVO_INSTANCE_TOKEN devem ser exportados de forma segura.
CHATWOOT_TOKEN="$CW_TOKEN" \
EVO_INSTANCE_TOKEN="$EVO_INSTANCE_TOKEN" \
./bin/connect setup \
  --name demo \
  --chatwoot-url https://chatwoot.empresa.com \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-instance demo \
  --connect-url https://evogo.empresa.com
```

Saída esperada:

```
✓ Token da instância Evolution Go validado: demo
✓ Inbox criada no Chatwoot: id=7
✓ Tenant 'demo' registrado (id=...)

Próximo passo — adicione um contato:
  connect add-contact --tenant demo --jid 5511999999999@s.whatsapp.net --name "João"
```

O que aconteceu:
1. **Inbox API no Chatwoot** foi criada com `webhook_url = https://evogo.empresa.com/webhook/chatwoot` e HMAC obrigatório.
2. O campo top-level **`secret`** retornado pelo Chatwoot foi persistido para validar `X-Chatwoot-Signature` e `X-Chatwoot-Timestamp`.
3. O token individual da instância Evolution Go foi validado em `/instance/status`.
4. **Tenant e segredo do webhook** são persistidos no Postgres (cifrados com
   `CONNECT_MASTER_KEY`).
5. A Evolution Go recebe uma URL exclusiva por instância, com o segredo no
   caminho e assinatura apenas da categoria `MESSAGE` (o evento recebido pode
   ser chamado de `MESSAGE` ou `MESSAGES_UPSERT`). O conector aceita tanto o
   formato de chave `data.key` quanto o formato nativo `data.info` documentado
   pela Evolution Go. Se este último usar um LID interno, o conector usa apenas
   `senderAlt` ou `recipientAlt` que sejam JIDs diretos válidos. Não copie a
   URL para logs, tickets ou chats.

### Sincronização de mensagens manuais

Mensagens de texto diretas enviadas pelo aplicativo WhatsApp do número
conectado também aparecem na conversa correspondente como mensagens de saída
do Chatwoot. O conector ignora grupos, listas de transmissão, status,
newsletters e mídia. Uma mensagem enviada pelo próprio Chatwoot não volta para
a conversa como cópia, e a mensagem criada pelo conector no Chatwoot não é
reenviada ao WhatsApp.

---

## 4. Adicionar contatos

> O conector cria contatos, vínculo com a inbox e conversa automaticamente
> quando chega o primeiro texto do WhatsApp. `add-contact` continua útil para
> iniciar uma conversa outbound antes de uma mensagem do cliente:

```bash
./bin/connect add-contact \
  --tenant demo \
  --jid 5511999999999@s.whatsapp.net \
  --name "João da Silva"
```

Isso cria ou reutiliza o contato pelo campo `identifier`, garante um
`contact_inbox` cujo `source_id` é exatamente o JID e salva o mapeamento no
`contact_map`. Pronto — o agente já pode responder e a
mensagem vai chegar no WhatsApp do João.

Se precisar abrir a conversa do contato pela linha de comando, sem usar um
token do Chatwoot no terminal:

```bash
./bin/connect start-conversation \
  --tenant demo \
  --jid 5511999999999@s.whatsapp.net
```

Abra a conversa retornada pelo comando e responda nela. Caso uma requisição
expire, confirme a conversa no Chatwoot antes de repetir o comando, pois o
Chatwoot pode ter criado a conversa mesmo sem a resposta chegar ao connector.

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

O Chatwoot 4.16.2 não faz retry automático confiável para webhooks de API
inbox: ele pode marcar a mensagem como `failed`. Nosso conector é idempotente
quando o mesmo webhook é reenviado, mas o reenvio deve ser acionado pelo
operador/Chatwoot ou por uma automação externa.

```bash
# Disparar o mesmo webhook 3 vezes (mesmo chatwoot_message_id)
WEBHOOK_BODY='{"event":"message_created","message_type":"outgoing","id":99999,"content":"teste","private":false,"inbox_id":7,"conversation":{"inbox_id":7,"contact_inbox":{"inbox_id":7,"source_id":"5511999999999@s.whatsapp.net"}}}'
for i in 1 2 3; do
  TIMESTAMP=$(date +%s)
  SIGNATURE=$(printf '%s.%s' "$TIMESTAMP" "$WEBHOOK_BODY" \
    | openssl dgst -sha256 -hmac "$CHATWOOT_WEBHOOK_SECRET" -hex \
    | sed 's/^.*= /sha256=/')
  curl -X POST https://evogo.empresa.com/webhook/chatwoot \
    -H "Content-Type: application/json" \
    -H "X-Chatwoot-Timestamp: $TIMESTAMP" \
    -H "X-Chatwoot-Signature: $SIGNATURE" \
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
# O Chatwoot pode marcar a mensagem como failed; reenvie após retomar o bridge

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

## 6. Deploy de produção no Coolify

Use exclusivamente `deploy/docker-compose.coolify.yml` para produção no
Coolify compatível (requisito mínimo `v4.0.0-beta.411`). O pacote contém apenas o conector e um PostgreSQL
privado e persistente; TLS e domínio pertencem ao proxy do Coolify. O compose
local `deploy/docker-compose.yml` mantém Caddy e portas de desenvolvimento e
não deve ser usado como modelo de produção.

Resumo:

1. Importe o repositório como recurso Docker Compose.
2. Selecione `deploy/docker-compose.coolify.yml`.
3. Associe `https://evogo.empresa.com:9090` ao serviço `connector`.
4. Mantenha segredos somente runtime, desative build args e aguarde os serviços ficarem healthy.
5. Abra o terminal do `connector` e execute `/app/connect setup`.
6. Faça backup das magic variables e do banco.
7. Execute o smoke real antes de liberar tráfego.

O Coolify gera as senhas, a chave AES-256 e o token administrativo pelas magic
environment variables do compose. Não regenere esses valores em redeploys. Em
especial, perder a chave mestra torna os tokens cifrados irrecuperáveis.

O procedimento completo de DNS, provisionamento, backup/restore, upgrade e
troubleshooting está em [Deploy de produção no Coolify](coolify.md).

---

## 7. Operação do dia-a-dia

### Comandos úteis

| Comando | O que faz |
|---|---|
| `./bin/connect list` | Lista tenants configurados |
| `./bin/connect status` | Estado do bridge + últimas 5 do audit log |
| `./bin/connect pause --reason "..."` | Pausa o bridge; mensagens podem ficar `failed` no Chatwoot |
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
4. Evolution Go está conectado? `curl -H "apikey: $EVO_INSTANCE_TOKEN" http://evo:8080/instance/status`
5. Teste manual:
   ```bash
   curl -X POST http://evo:8080/send/text \
     -H "apikey: $EVO_INSTANCE_TOKEN" -H "Content-Type: application/json" \
     -d '{"number":"5511999999999","text":"teste"}'
   ```

### "Chatwoot retorna 401 no webhook"

1. O relógio da VPS está sincronizado? A janela padrão é 5 minutos.
2. Conferir o campo top-level `secret` no retorno de `GET /api/v1/accounts/{id}/inboxes/{inbox_id}`.
3. Confirmar que ele corresponde ao valor cifrado em `tenants.chatwoot_hmac_enc`.
4. Verificar os headers `X-Chatwoot-Timestamp` e `X-Chatwoot-Signature: sha256=...`.
5. Métrica: `bridge_errors_total{code="hmac_invalid"}` deve estar > 0.

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
CHATWOOT_TOKEN="$CW_TOKEN" \
EVO_INSTANCE_TOKEN="$EVO_INSTANCE_TOKEN" \
./bin/connect setup --name loja2 \
  --chatwoot-url https://chatwoot.empresa.com \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-instance loja2 \
  --connect-url https://evogo.empresa.com

./bin/connect add-contact --tenant loja2 \
  --jid 5511888888888@s.whatsapp.net --name "Maria"
```

Cada tenant = 1 inbox Chatwoot + 1 instância evolution-go. Suporta N
tenants em paralelo (multi-empresa).

---

## 9. Limites atuais

- **Texto direto** — ambos os sentidos funcionam, inclusive mensagens diretas
  enviadas manualmente pelo número conectado. Mensagens de grupo, mídia e
  status recebidos da Evolution Go são ignorados nesta versão.
- **Mídia:** só o primeiro anexo da mensagem é enviado (Etapa 5 amplia pra
  múltiplos anexos + transcrição de áudio).
- **Contactos:** mensagens recebidas criam o contato e vínculo automaticamente.
  `connect add-contact` é apenas um atalho para iniciar atendimento outbound.
- **Grupos WhatsApp:** fora de escopo (Etapa 7).
- **Status updates** (delivered/read): fora de escopo (Etapa 6).

---

## 10. Próximos passos

- **Hardening** — testes de carga, OpenAPI, pprof, exemplos
  de dashboards Grafana.
- **Etapa 5** — Mídia rica: múltiplos anexos, voice notes com transcrição
  opcional (Whisper local), stickers.
- **Etapa 6** — Status updates: MESSAGES_UPDATE do evolution-go refletido
  no Chatwoot (delivered/read).

Depois do deploy, faça um smoke: envie um texto do WhatsApp e confirme a
mensagem `incoming` na inbox correta do Chatwoot.

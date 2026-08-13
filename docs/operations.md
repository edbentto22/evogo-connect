# Operations runbook — evogo-connect

## Health checks

| Endpoint | Verifica | Códigos |
|---|---|---|
| `GET /healthz` | processo vivo | sempre 200 |
| `GET /readyz` | DB conectado | 200 ok / 503 db_down |
| `GET /metrics` | Prometheus exposition | 200 |

Liveness/readiness para Kubernetes:
```yaml
livenessProbe:
  httpGet: { path: /healthz, port: 9090 }
  periodSeconds: 30
readinessProbe:
  httpGet: { path: /readyz, port: 9090 }
  periodSeconds: 10
```

## Métricas Prometheus

| Métrica | Tipo | Labels | Significado |
|---|---|---|---|
| `bridge_messages_total` | counter | direction, status | mensagens processadas |
| `bridge_latency_seconds` | histogram | direction | latência do bridge |
| `bridge_errors_total` | counter | code, component | erros categorizados |
| `http_requests_total` | counter | method, path, status | tráfego HTTP |
| `idempotency_hits_total` | counter | — | dedupes |

Exemplo de alerta:
```yaml
- alert: BridgeErrorsHigh
  expr: rate(bridge_errors_total[5m]) > 0.5
  for: 5m
  labels: { severity: warning }
  annotations:
    summary: "Taxa de erros do bridge acima de 0.5/s"
```

## Comandos operacionais

```bash
# Pausar / retomar
connect pause --reason "manutenção evol-go"
connect resume

# Status
connect status
connect list

# Logs
docker logs -f evogo-connect-connector-1 | jq

# Inspecionar audit log
psql -c "SELECT direction, status, count(*) FROM bridge_log
         WHERE created_at > now() - interval '1 hour'
         GROUP BY 1,2;"

# Verificar idempotência pendente
psql -c "SELECT direction, key, status, created_at FROM idempotency
         WHERE expires_at > now() ORDER BY created_at DESC LIMIT 20;"
```

## Cenários de troubleshooting

### "Mensagem não chega no WhatsApp depois de o agente responder"

1. Verificar `connect status` — está pausado?
2. `curl http://localhost:9090/admin/paused -H "X-Admin-Token: $TOKEN"`
3. Ver `bridge_log` para erros:
   ```sql
   SELECT created_at, direction, status, error_code, error_detail
   FROM bridge_log
   WHERE created_at > now() - interval '10 minutes'
   ORDER BY created_at DESC LIMIT 20;
   ```
4. Confirmar que evolution-go está conectado:
   ```bash
   curl -H "apikey: $EVO_INSTANCE_TOKEN" http://localhost:8080/instance/status
   ```
5. Verificar logs do evolution-go: o webhook chegou lá?
6. Testar envio manual:
   ```bash
   curl -X POST http://localhost:8080/send/text \
     -H "apikey: $EVO_INSTANCE_TOKEN" -H "Content-Type: application/json" \
     -d '{"number":"5511999999999","text":"teste"}'
   ```

### "Chatwoot retorna 401 no webhook"

1. Conferir o campo top-level `secret` no retorno de `GET /api/v1/accounts/{id}/inboxes/{inbox_id}`.
2. Confirmar que esse segredo foi persistido cifrado em `tenants.chatwoot_hmac_enc`.
3. Conferir se o relógio da VPS está sincronizado; a tolerância padrão é 5 minutos.
4. Ver `bridge_errors_total{code="hmac_invalid"}`.

### "Postgres lotado"

Tabela `idempotency` cresce 1 linha por mensagem. Limpeza:
```sql
DELETE FROM idempotency WHERE expires_at < now();
```

Recomenda-se job agendado (pg_cron, etc) ou limpeza via `evogo-connect`
em background (Etapa 6).

### "Como adicionar mais um número (nova inbox)?"

```bash
connect setup --name loja2 \
  --chatwoot-url https://cw.example.com \
  --chatwoot-token $CW_TOKEN \
  --chatwoot-account 1 \
  --evo-url http://localhost:8080 \
  --evo-key $EVO_INSTANCE_TOKEN \
  --evo-instance loja2 \
  --connect-url https://evogo-connect.example.com

connect add-contact --tenant loja2 \
  --jid 5511888888888@s.whatsapp.net \
  --name "Maria"
```

## Backup

Crítico: tabela `tenants` (contém chaves cifradas). Sem ela, não é possível
recriar o bridge. Recomenda-se backup diário do Postgres com retenção de 30d.

```bash
# Backup
pg_dump -Fc evogo_connect > backup-$(date +%F).dump

# Restore
pg_restore -d evogo_connect backup-2026-08-12.dump
```

## Upgrade

Tenants criados por versões anteriores guardavam `hmac_token` e a chave global
do Evolution Go. Depois de subir a nova imagem, execute novamente `connect
setup` com o **mesmo `--name`**, o token individual da instância e as
credenciais atuais do Chatwoot. O comando atualiza o tenant e reutiliza a inbox,
sem perder contatos nem IDs locais.

```bash
# 1. Puxar imagem nova
docker compose -f deploy/docker-compose.yml pull

# 2. Parar connector (Postgres continua)
docker compose -f deploy/docker-compose.yml stop connector

# 3. Subir com imagem nova (migrations rodam na inicialização)
docker compose -f deploy/docker-compose.yml up -d connector

# 4. Confirmar /readyz
curl http://localhost:9090/readyz
```

Migrations são **aditivas** na Etapa 1+. Não há DROP COLUMN no schema
inicial — toda evolução é compatível com rollback pra versão anterior.

#!/usr/bin/env bash
# Smoke E2E manual do evogo-connect.
# Pré-requisitos:
#   - Postgres rodando (docker compose -f deploy/docker-compose.yml up -d postgres)
#   - Chatwoot rodando (local ou remoto) com uma account criada
#   - evolution-go rodando com uma instância pareada
#   - .env exportando CONNECT_MASTER_KEY, ADMIN_TOKEN, DATABASE_URL

set -euo pipefail

CONNECT_URL="${CONNECT_URL:-http://localhost:9090}"
ADMIN_TOKEN="${ADMIN_TOKEN:?need ADMIN_TOKEN in env}"
CHATWOOT_URL="${CHATWOOT_URL:?need CHATWOOT_URL}"
CHATWOOT_TOKEN="${CHATWOOT_TOKEN:?need CHATWOOT_TOKEN}"
CHATWOOT_ACCOUNT="${CHATWOOT_ACCOUNT:-1}"
EVO_URL="${EVO_URL:?need EVO_URL}"
EVO_KEY="${EVO_KEY:?need EVO_KEY}"
EVO_INSTANCE="${EVO_INSTANCE:?need EVO_INSTANCE}"
TEST_JID="${TEST_JID:-5511999999999@s.whatsapp.net}"
TEST_NAME="${TEST_NAME:-Teste E2E}"

echo "═══ 1. Health ═══"
curl -fsS "$CONNECT_URL/healthz" | tee /tmp/health.json
echo
curl -fsS "$CONNECT_URL/readyz" | tee /tmp/ready.json
echo

echo "═══ 2. Setup tenant ═══"
./bin/connect setup \
  --name "smoke-$(date +%s)" \
  --chatwoot-url "$CHATWOOT_URL" \
  --chatwoot-token "$CHATWOOT_TOKEN" \
  --chatwoot-account "$CHATWOOT_ACCOUNT" \
  --evo-url "$EVO_URL" \
  --evo-key "$EVO_KEY" \
  --evo-instance "$EVO_INSTANCE" \
  --connect-url "$CONNECT_URL"

echo "═══ 3. Add contact ═══"
TENANT=$(./bin/connect list | head -1 | awk '{print $1}')
./bin/connect add-contact --tenant "$TENANT" --jid "$TEST_JID" --name "$TEST_NAME"

echo "═══ 4. List ═══"
./bin/connect list

echo "═══ 5. Status ═══"
./bin/connect status

echo "═══ 6. Inspect webhook Chatwoot URL ═══"
echo "  → $CONNECT_URL/webhook/chatwoot"
echo
echo "No Chatwoot UI: Settings → Inboxes → evogo-connect/<tenant> → Configuration"
echo "  - Webhook URL deve ser $CONNECT_URL/webhook/chatwoot"
echo "  - Hmac Mandatory: true"
echo
echo "═══ 7. Test (manual) ═══"
echo "Para testar de verdade:"
echo "  1. Abra o Chatwoot"
echo "  2. Selecione a inbox 'evogo-connect/<tenant>'"
echo "  3. Inicie uma conversa com o contato '$TEST_NAME'"
echo "  4. Responda algo como agente"
echo "  5. Verifique:"
echo "     - Status 200 nos logs do evogo-connect"
echo "     - Mensagem chegou no WhatsApp de $TEST_JID"
echo "     - Audit log em ./bin/connect status tem a entrada"
echo
echo "═══ 8. Métricas ═══"
curl -fsS "$CONNECT_URL/metrics" | grep -E "^(bridge_|idempotency_)" | head -20

echo
echo "✓ Smoke OK"

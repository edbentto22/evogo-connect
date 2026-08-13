#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deploy/docker-compose.coolify.yml"

fail() {
  printf 'ERRO: %s\n' "$*" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker não encontrado"
docker compose version >/dev/null 2>&1 || fail "docker compose não encontrado"
test -f "$COMPOSE_FILE" || fail "arquivo ausente: deploy/docker-compose.coolify.yml"
umask 077

required_magic_variables=(
  'SERVICE_PASSWORD_64_POSTGRES'
  'SERVICE_REALBASE64_32_CONNECTOR'
  'SERVICE_HEX_64_CONNECTOR_ADMIN'
)

for variable in "${required_magic_variables[@]}"; do
  grep -Fq "\${$variable}" "$COMPOSE_FILE" || fail "magic variable ausente: $variable"
done

# Valores locais descartáveis existem apenas para o parser do Compose. Eles
# sobrescrevem o ambiente deste processo para nunca copiar segredos reais para
# o arquivo renderizado. Em produção, o Coolify gera e preserva os valores.
export SERVICE_PASSWORD_64_POSTGRES="compose-validation-password"
export SERVICE_REALBASE64_32_CONNECTOR="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
export SERVICE_HEX_64_CONNECTOR_ADMIN="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
export CHATWOOT_TOKEN=""
export EVO_INSTANCE_TOKEN=""
export BRIDGE_PAUSED="true"

rendered_file="$(mktemp)"
postgres_file="$(mktemp)"
connector_file="$(mktemp)"
trap 'rm -f "$rendered_file" "$postgres_file" "$connector_file"' EXIT

docker compose -f "$COMPOSE_FILE" config >"$rendered_file"

extract_service() {
  local service="$1"
  local destination="$2"
  awk -v service="$service" '
    $0 == "  " service ":" { in_service = 1 }
    in_service && /^  [[:alnum:]_.-]+:$/ && $0 != "  " service ":" { exit }
    in_service && /^[^ ]/ && $0 != "services:" { exit }
    in_service { print }
  ' "$rendered_file" >"$destination"
}

services="$(docker compose -f "$COMPOSE_FILE" config --services)"
test "$services" = $'connector\npostgres' || test "$services" = $'postgres\nconnector' || \
  fail "o stack deve conter somente connector e postgres"

extract_service postgres "$postgres_file"
extract_service connector "$connector_file"

grep -Eq '^    healthcheck:$' "$postgres_file" || fail "postgres sem healthcheck"
grep -Fq 'pg_isready' "$postgres_file" || fail "healthcheck do postgres não usa pg_isready"
if grep -Eq '^    ports:$|published:' "$postgres_file"; then
  fail "postgres não pode publicar portas"
fi
grep -Fq '/var/lib/postgresql/data' "$postgres_file" || fail "postgres sem volume persistente"
grep -Fq 'type: volume' "$postgres_file" || fail "persistência do postgres não usa volume nomeado"
grep -Fq 'source: postgres_data' "$postgres_file" || fail "postgres não monta postgres_data"
grep -Fq 'POSTGRES_PASSWORD: compose-validation-password' "$postgres_file" || \
  fail "postgres não usa a senha gerada pelo Coolify"

# O Coolify cria e gerencia a rede isolada do recurso. Redes declaradas no
# compose podem fazer o proxy selecionar um IP inacessível de forma aleatória.
if grep -Eq '^networks:|^    networks:' "$COMPOSE_FILE"; then
  fail "não declare redes customizadas; o Coolify gerencia a rede do recurso"
fi

grep -Eq '^    healthcheck:$' "$connector_file" || fail "connector sem healthcheck"
grep -Fq '/app/evogo-connect' "$connector_file" || fail "healthcheck do connector não usa o binário"
grep -Fq -- '--healthcheck' "$connector_file" || fail "healthcheck do connector não consulta /readyz"
grep -Fq 'condition: service_healthy' "$connector_file" || fail "connector não aguarda postgres saudável"
grep -Eq '^    expose:$' "$connector_file" || fail "connector não expõe porta interna ao proxy"
grep -Fq -- '- "9090"' "$connector_file" || fail "connector não declara a porta interna 9090"
grep -Fq 'DATABASE_URL: postgres://connect:compose-validation-password@postgres:5432/evogo_connect?sslmode=disable' "$connector_file" || \
  fail "DATABASE_URL não reutiliza a senha do postgres"
grep -Fq 'BRIDGE_PAUSED: "true"' "$connector_file" || fail "primeiro deploy não inicia pausado"
grep -Fq 'CHATWOOT_TOKEN:' "$connector_file" || fail "credencial temporária do Chatwoot não é injetável"
grep -Fq 'EVO_INSTANCE_TOKEN:' "$connector_file" || fail "credencial temporária do Evolution Go não é injetável"
grep -Fq 'read_only: true' "$connector_file" || fail "filesystem do connector não está read-only"
grep -Fq 'no-new-privileges:true' "$connector_file" || fail "connector permite elevação de privilégios"
grep -Fq 'cap_drop:' "$connector_file" || fail "connector não remove capabilities Linux"
grep -Fq 'COPY --from=build /src/migrations /app/migrations' "$ROOT_DIR/deploy/Dockerfile" || \
  fail "imagem não inclui migrations para execução automática"
if grep -Fxq '*.sql' "$ROOT_DIR/.dockerignore"; then
  fail ".dockerignore exclui as migrations SQL do contexto de build"
fi

volumes="$(docker compose -f "$COMPOSE_FILE" config --volumes)"
grep -Fxq 'postgres_data' <<<"$volumes" || fail "volume nomeado postgres_data ausente"

printf 'OK: compose Coolify renderiza com persistência, isolamento, hardening e healthchecks declarados.\n'

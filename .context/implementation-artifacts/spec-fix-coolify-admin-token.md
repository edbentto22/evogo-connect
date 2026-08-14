---
title: 'Corrigir geração do token administrativo no Coolify'
type: 'bugfix'
created: '2026-08-13'
status: 'done'
route: 'one-shot'
context: []
---

# Corrigir geração do token administrativo no Coolify

## Intent

**Problem:** A magic variable `SERVICE_HEX_64_CONNECTOR_ADMIN` chegou vazia ao container, fazendo o processo reiniciar continuamente com `ADMIN_TOKEN not set`.

**Approach:** Usar um identificador simples suportado pelo padrão oficial, exigir todos os segredos na renderização do Compose e testar automaticamente a ausência de cada um.

## Suggested Review Order

**Contrato de configuração**

- Gera o token por identificador simples e bloqueia qualquer segredo vazio antes do container.
  [`docker-compose.coolify.yml:10`](../../deploy/docker-compose.coolify.yml#L10)

**Proteção e migração**

- Prova que remover individualmente cada segredo faz o Compose falhar.
  [`validate-coolify-compose.sh:21`](../../scripts/validate-coolify-compose.sh#L21)

- Preserva eventual token antigo não vazio durante o upgrade.
  [`coolify.md:31`](../../docs/coolify.md#L31)

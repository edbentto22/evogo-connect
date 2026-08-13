---
title: Corrigir tipo do token no claim de idempotência
type: implementation-spec
created: 2026-08-13
status: done
route: one-shot
---

# Corrigir tipo do token no claim de idempotência

## Intent

**Problema:** PostgreSQL não consegue inferir o tipo do token passado para
`jsonb_build_object`, impedindo todo envio Chatwoot → WhatsApp antes da
chamada à Evolution.

**Abordagem:** declarar o parâmetro como `text` na query de claim, que é o tipo
do token de idempotência armazenado no JSONB.

## Suggested Review Order

1. `../../../internal/store/store.go`

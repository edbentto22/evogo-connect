---
title: Aceitar evento Evolution Go com ponto
type: bugfix
created: 2026-08-13
status: done
route: one-shot
---

# Aceitar evento Evolution Go com ponto

## Intent

**Problem:** a Evolution Go envia `messages.upsert`, mas o conector aceitava
somente a variação com sublinhado e retornava 200 como evento ignorado.

**Approach:** normalizar separadores no nome do evento, mantendo a aceitação
restrita às duas denominações equivalentes de mensagens recebidas.

## Suggested Review Order

1. `../../../internal/evogo/types.go`
2. `../../../internal/evogo/types_test.go`

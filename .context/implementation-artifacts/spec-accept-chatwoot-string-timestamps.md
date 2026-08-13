---
title: 'Aceitar timestamps textuais do Chatwoot'
type: 'bugfix'
created: '2026-08-13'
status: 'done'
route: 'one-shot'
---

# Aceitar timestamps textuais do Chatwoot

## Intent

**Problem:** Webhooks reais do Chatwoot eram rejeitados com HTTP 400 quando `created_at` chegava como texto, impedindo qualquer chamada à Evolution Go.

**Approach:** Aceitar Unix seconds numérico, Unix seconds textual e RFC3339, normalizando todos para o contrato temporal numérico e rejeitando valores inválidos.

## Suggested Review Order

**Compatibilidade do webhook**

- Normaliza as representações reais sem flexibilizar tipos JSON inválidos.
  [`types.go:13`](../../internal/chatwoot/types.go#L13)

- Aplica o tipo compatível aos timestamps da mensagem e da conversa.
  [`types.go:52`](../../internal/chatwoot/types.go#L52)

**Regressão e limites**

- Reproduz o formato textual observado e preserva o contrato numérico anterior.
  [`types_test.go:13`](../../internal/chatwoot/types_test.go#L13)

- Garante normalização, round-trip numérico e rejeição de valores impróprios.
  [`types_test.go:92`](../../internal/chatwoot/types_test.go#L92)

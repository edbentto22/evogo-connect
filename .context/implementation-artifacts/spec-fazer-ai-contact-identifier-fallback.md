---
title: Compatibilidade do identifier de contato Fazer.ai
type: implementation-spec
created: 2026-08-13
status: done
route: one-shot
---

# Compatibilidade do identifier de contato Fazer.ai

## Intent

**Problema:** o Fazer.ai pode omitir `conversation.contact_inbox.source_id` em
webhooks outgoing, impedindo o envio mesmo em uma conversa corretamente criada.

**Abordagem:** preservar o campo canônico quando existir e usar o identifier do
contato no payload como fallback, validando-o como JID direto e registrando o
caminho usado sem expor o número.

## Suggested Review Order

1. `../../../internal/bridge/bridge.go`
2. `../../../internal/chatwoot/types.go`
3. `../../../internal/bridge/bridge_test.go`
4. `../../../docs/architecture.md`

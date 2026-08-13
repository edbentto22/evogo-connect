---
title: Criar conversa Chatwoot pelo CLI
type: implementation-spec
created: 2026-08-13
status: done
route: one-shot
---

# Criar conversa Chatwoot pelo CLI

## Intent

**Problema:** o terminal do Coolify não mantém o token temporário do Chatwoot
após o setup; tentar criar uma conversa diretamente pela API expõe credenciais
e falha de forma pouco diagnóstica.

**Abordagem:** adicionar `connect start-conversation`, que valida o JID já
mapeado, reutiliza a credencial cifrada do tenant e cria uma conversa na inbox
correta. O comando não imprime credenciais nem conteúdo de resposta do
Chatwoot.

## Suggested Review Order

1. `cmd/connect-cli/start_conversation.go`
2. `internal/chatwoot/client.go` e testes
3. `docs/coolify.md` e `docs/USAGE.md`

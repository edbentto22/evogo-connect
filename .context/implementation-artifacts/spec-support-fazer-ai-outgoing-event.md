---
title: 'Suportar evento de saída do Chatwoot fazer.ai'
type: 'bugfix'
created: '2026-08-13'
status: 'done'
route: 'one-shot'
---

# Suportar evento de saída do Chatwoot fazer.ai

## Intent

**Problem:** A instalação `4.15.1-fazer-ai-pro.104` emite o evento adicional `message_outgoing`, que era aceito pelo endpoint mas ignorado antes de chamar a Evolution Go.

**Approach:** Processar `message_outgoing` com os mesmos filtros e a mesma chave idempotente de `message_created`, validar o ID da mensagem e manter métricas de eventos ignorados com cardinalidade limitada.

## Suggested Review Order

**Compatibilidade e segurança**

- Aceita o evento fazer.ai preservando filtros, HMAC e idempotência existentes.
  [`bridge.go:125`](../../internal/bridge/bridge.go#L125)

- Exige ID real antes de qualquer lookup, claim ou efeito externo.
  [`bridge.go:157`](../../internal/bridge/bridge.go#L157)

- Categoriza eventos normais sem usar entrada arbitrária como label.
  [`bridge.go:292`](../../internal/bridge/bridge.go#L292)

**Regressão e concorrência**

- Fixture fazer.ai confirma endpoint, número, conteúdo e ID determinístico.
  [`bridge_test.go:144`](../../internal/bridge/bridge_test.go#L144)

- Eventos duplicados, invertidos ou concorrentes produzem apenas um envio.
  [`bridge_test.go:186`](../../internal/bridge/bridge_test.go#L186)

- Mensagens incoming, privadas e eventos alheios permanecem sem efeitos.
  [`bridge_test.go:245`](../../internal/bridge/bridge_test.go#L245)

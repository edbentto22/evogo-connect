---
title: 'Resolver JID alternativo para mensagens próprias da Evolution Go'
type: 'bugfix'
created: '2026-08-14'
status: 'done'
baseline_commit: 'ab2a8cd'
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/docs/security.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** mensagens enviadas manualmente pelo número conectado podem chegar no webhook com `data.info.chat` em formato LID em vez de um JID WhatsApp direto. O conector não conhece os campos alternativos da Evolution Go e encerra o processamento com `invalid direct WhatsApp JID`; por isso a mensagem não aparece no Chatwoot.

**Approach:** normalizar também os JIDs alternativos documentados pela Evolution Go. Para mensagem própria, escolher somente o destinatário direto válido; para mensagem recebida, escolher somente o remetente direto válido. A escolha não pode transformar grupos, listas ou o próprio número conectado em contato de destino.

## Boundaries & Constraints

**Always:** manter suporte aos formatos `data.key` e `data.info`; aceitar somente JID `@s.whatsapp.net` direto; preservar o filtro de grupos, broadcast e newsletters; não alterar a lógica de idempotência, supressão de loop ou criação de mensagens; não escrever valores de JID, texto ou nome em logs.

**Ask First:** pedir autorização antes de introduzir suporte para grupos, LID sem alternativa direta, mídia, status ou newsletter; pedir autorização antes de persistir mapeamentos LID no banco.

**Never:** não encaminhar a mensagem manual para o remetente (número conectado); não usar o primeiro campo disponível sem validar seu domínio; não substituir um JID direto já fornecido em `data.key`; não relaxar `ParseDirectJID`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|---------------|----------------------------|----------------|
| Manual com destinatário alternativo | `info.isFromMe=true`, `chat=@lid`, `recipientAlt=@s.whatsapp.net` | Publica `outgoing` na conversa do destinatário alternativo | Fluxo normal de idempotência e retry |
| Entrada com remetente alternativo | `info.isFromMe=false`, `chat/sender=@lid`, `senderAlt=@s.whatsapp.net` | Publica `incoming` para o remetente alternativo | Fluxo normal de idempotência e retry |
| JID direto canônico | `data.key.remoteJid` ou `info.chat=@s.whatsapp.net` | Mantém o JID existente | Nenhuma regressão |
| Grupo/lista | `info.chat=@g.us`, broadcast ou newsletter, mesmo com outros campos | Nenhum efeito no Chatwoot | Skip seguro, sem retry |
| Sem alternativa direta | Somente LID ou domínio desconhecido | Nenhum efeito externo | Erro reenviável, sem PII |

</frozen-after-approval>

## Code Map

- `internal/evogo/types.go` -- normalização da chave, `info` e seleção do JID para o bridge.
- `internal/evogo/types_test.go` -- contratos de payload próprio, recebido, LID e JID alternativo.
- `internal/bridge/bridge_test.go` -- garante que o JID normalizado chega ao fluxo correto sem regressão de loop.
- `docs/architecture.md` -- descrição do contrato de mensagens diretas aceitas.

## Tasks & Acceptance

**Execution:**
- [x] `internal/evogo/types.go` -- adicionar `recipientAlt` e `senderAlt` ao payload nativo e selecionar candidatos conforme o sentido da mensagem -- resolve LID sem adivinhar destino.
- [x] `internal/evogo/types_test.go` -- cobrir saída manual com `recipientAlt`, entrada com `senderAlt`, grupo e precedência de `key` -- protege a seleção segura.
- [x] `internal/bridge/bridge_test.go` -- cobrir encaminhamento da mensagem própria usando o destinatário alternativo -- confirma a integração funcional.
- [x] `docs/architecture.md` -- documentar os JIDs alternativos suportados e o limite de mensagens diretas -- orienta operação.

**Acceptance Criteria:**
- Given mensagem manual própria cujo `chat` seja LID e `recipientAlt` seja JID direto, when o webhook chega, then ela aparece uma vez como `outgoing` no Chatwoot para o destinatário alternativo.
- Given mensagem recebida cujo remetente principal seja LID e `senderAlt` seja JID direto, when o webhook chega, then ela aparece uma vez como `incoming` no Chatwoot para o remetente alternativo.
- Given JID canônico em `data.key`, when campos alternativos também existirem, then o conector usa o JID canônico.
- Given grupo, broadcast, newsletter ou alternativa sem domínio direto válido, when o webhook chega, then nenhuma mensagem é criada e não há PII no log.

## Verification

**Commands:**
- `go build ./...` -- build completo sem erro.
- `go vet ./...` -- análise estática sem erro.
- `go test -race -count=1 ./...` -- cenários de JID e concorrência verdes.
- `go mod tidy -diff && test -z "$(gofmt -l .)" && git diff --check` -- árvore formatada e dependências íntegras.

## Suggested Review Order

**Normalização segura de identidade**

- Preserva JID canônico e resolve LID apenas por alternativas diretas seguras.
  [`types.go:105`](../../internal/evogo/types.go#L105)

- Separa destinatário próprio de remetente recebido, sem transformar grupos em conversas diretas.
  [`types.go:127`](../../internal/evogo/types.go#L127)

**Integração e contratos**

- Exercita a publicação outgoing real usando `recipientAlt` em evento LID.
  [`bridge_test.go:277`](../../internal/bridge/bridge_test.go#L277)

- Cobre LID, alternativas, grupos e precedência da chave canônica.
  [`types_test.go:156`](../../internal/evogo/types_test.go#L156)

**Operação**

- Documenta o comportamento suportado para LID no fluxo de entrada.
  [`architecture.md:97`](../../docs/architecture.md#L97)

- Esclarece a configuração e o limite atual de mensagens manuais.
  [`USAGE.md:123`](../../docs/USAGE.md#L123)

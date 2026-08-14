---
title: 'Sincronizar mensagens manuais do WhatsApp ao Chatwoot'
type: 'feature'
created: '2026-08-13'
status: 'done'
baseline_commit: 'bdcda8034ada115b5fb4b5f355d8092a090e4e3f'
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/docs/security.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** quando um atendente envia uma mensagem diretamente pelo aplicativo WhatsApp do número conectado, ela não aparece na conversa correspondente no Chatwoot. Hoje o conector descarta toda mensagem marcada como própria para impedir que o webhook gerado por um envio originado no Chatwoot crie uma cópia.

**Approach:** sincronizar mensagens diretas próprias que vierem da Evolution Go como mensagens `outgoing` no Chatwoot e reconhecer, pelo ID estável da mensagem Evolution, as mensagens que o próprio conector acabou de enviar para não duplicá-las.

## Boundaries & Constraints

**Always:** aceitar somente texto direto com JID WhatsApp válido; manter grupos, broadcast, newsletter, mídia e eventos desconhecidos fora do Chatwoot; preservar idempotência, limite por tenant, kill switch, auditoria sem conteúdo e mascaramento de PII; mensagens Chatwoot → WhatsApp nunca podem reaparecer como nova mensagem no Chatwoot; uma falha transitória ao publicar no Chatwoot deve ser reenviável sem duplicar uma publicação concluída.

**Ask First:** solicitar autorização antes de sincronizar grupos, status, listas de transmissão, mídia ou qualquer tipo de mensagem que não seja texto direto; solicitar autorização antes de alterar o schema do banco além do mecanismo de idempotência já existente.

**Never:** não registrar conteúdo, nomes, JIDs completos, tokens ou URL secreta; não tratar mensagens próprias sem ID de mensagem como entregues; não substituir o fluxo de saída atual do Chatwoot; não criar uma nova conversa quando houver uma aberta do mesmo contato e inbox.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|---------------|----------------------------|----------------|
| Mensagem manual própria | Webhook `MESSAGE` direto, `fromMe=true`, ID Evolution novo e texto | Mensagem `outgoing` criada na conversa aberta correspondente no Chatwoot; idempotência/auditoria concluídas | Erro Chatwoot libera claim, grava auditoria de erro e responde como reenviável |
| Mensagem originada no Chatwoot | Webhook próprio cujo ID coincide com o ID determinístico usado no envio c2w | Nenhuma mensagem adicional é criada no Chatwoot | Retorna sucesso/skip idempotente |
| Reentrega Evolution | Mesmo ID próprio recebido novamente após publicação concluída | Nenhuma duplicidade | Retorna sucesso por idempotência concluída |
| Evento não direto | Grupo, broadcast, newsletter ou identificador alternativo sem remetente direto válido | Nenhuma publicação no Chatwoot | Skip seguro, sem PII e sem retry |
| Própria sem ID | Mensagem própria sem chave de identificação estável | Nenhum efeito externo | Erro reenviável e auditoria apropriada, sem conteúdo no log |

</frozen-after-approval>

## Code Map

- `internal/bridge/bridge.go` -- orquestra envio Chatwoot → Evolution e recebimento Evolution → Chatwoot, incluindo idempotência/auditoria.
- `internal/evogo/types.go` -- normaliza os contratos `key` e `info` do webhook e classifica remetente/texto/JID.
- `internal/chatwoot/client.go` -- garante contato/conversa e cria a mensagem `incoming`; receberá operação explícita para `outgoing`.
- `internal/bridge/bridge_test.go` -- testes do core com store fake e HTTP upstream simulado.
- `internal/evogo/types_test.go` e `internal/chatwoot/client_test.go` -- testes de classificação e contrato HTTP.
- `docs/architecture.md` e `docs/USAGE.md` -- contrato operacional da sincronização bidirecional.

## Tasks & Acceptance

**Execution:**
- [x] `internal/evogo/types.go` -- expor de forma segura se a mensagem direta é própria, sem perder a normalização de `key`/`info` -- permite selecionar o sentido correto do sync.
- [x] `internal/bridge/bridge.go` -- reservar e concluir uma chave de idempotência por ID Evolution; distinguir manual própria de envio c2w pelo ID determinístico; encaminhar somente a manual para Chatwoot como saída -- evita loops e cópias.
- [x] `internal/chatwoot/client.go` e testes adjacentes -- criar operação para mensagem pública `outgoing` usando o endpoint já homologado -- preserva semântica de conversa do Chatwoot.
- [x] `internal/bridge/bridge_test.go` e `internal/evogo/types_test.go` -- testar publicação manual, supressão c2w, reentrega, falha Chatwoot e skips -- protege a matriz de casos.
- [x] `docs/architecture.md` e `docs/USAGE.md` -- documentar o que sincroniza e o que continua ignorado -- torna operação previsível.

**Acceptance Criteria:**
- Given uma conversa aberta e uma mensagem de texto direta enviada manualmente pelo WhatsApp conectado, when a Evolution Go entrega o webhook próprio, then a mensagem aparece uma vez como saída na mesma conversa do Chatwoot.
- Given uma mensagem de saída criada no Chatwoot, when a Evolution Go notifica a própria mensagem, then nenhuma segunda mensagem é criada no Chatwoot.
- Given uma nova entrega do mesmo webhook próprio, when a publicação anterior foi concluída, then o conector responde com sucesso sem nova publicação.
- Given falha temporária do Chatwoot, when o webhook é recebido, then o conector não confirma a entrega como concluída e uma tentativa posterior pode finalizar sem duplicidade.
- Given grupo, broadcast, newsletter, mídia ou evento desconhecido, when o webhook é recebido, then nenhuma mensagem ou conversa é criada no Chatwoot e nenhum dado pessoal aparece no log.

## Design Notes

O identificador do envio Chatwoot → Evolution já é determinístico (`CW` + hash de tenant e ID de mensagem Chatwoot). O webhook de volta deve comparar esse formato antes de publicar uma mensagem própria. Mensagens próprias com outro ID são consideradas manuais e usam uma chave `w2c` separada, baseada no ID recebido da Evolution. Dessa forma a mesma proteção persistente cobre reentregas e falhas intermediárias.

## Verification

**Commands:**
- `go build ./...` -- build completo sem erro.
- `go vet ./...` -- análise estática sem erro.
- `go test -race -count=1 ./...` -- testes funcionais e de concorrência verdes.
- `go mod tidy -diff && test -z "$(gofmt -l .)" && git diff --check` -- dependências, formatação e diff limpos.

## Suggested Review Order

**Prevenção de loops e duplicidade**

- Registra a origem antes do envio e bloqueia o eco da Evolution Go.
  [`bridge.go:218`](../../internal/bridge/bridge.go#L218)

- Publica mensagens manuais como saída e reserva sua supressão no retorno.
  [`bridge.go:401`](../../internal/bridge/bridge.go#L401)

- Conclui as duas chaves e auditorias na mesma transação PostgreSQL.
  [`store.go:413`](../../internal/store/store.go#L413)

**Contratos de integração**

- Cria mensagem pública de saída e exige ID estável do Chatwoot.
  [`client.go:208`](../../internal/chatwoot/client.go#L208)

- Diferencia mensagens próprias das recebidas após normalizar o webhook.
  [`types.go:62`](../../internal/evogo/types.go#L62)

**Cobertura operacional**

- Exercita publicação manual, reentrega, supressão e falha transitória.
  [`bridge_test.go:246`](../../internal/bridge/bridge_test.go#L246)

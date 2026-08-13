---
title: 'Homologar o reverse bridge com Evolution Go e Chatwoot atuais'
type: 'bugfix'
created: '2026-08-13'
status: 'done'
baseline_commit: '2be79e0a50e8fe09dbb7c3cbcc30accd1b19fff1'
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/.context/plans/2026-08-12-evogo-chatwoot-connector.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O fluxo declarado como concluído usa rotas, tokens, payloads e assinatura incompatíveis com Evolution Go 0.7.2 e Chatwoot 4.16.2. O provisionamento também não garante que `contact_inbox.source_id` seja o JID, impedindo o envio confiável ao WhatsApp.

**Approach:** Alinhar clientes e CLI aos contratos oficiais fixados, validar corretamente o webhook assinado do Chatwoot e cobrir o fluxo Chatwoot → WhatsApp com testes automatizados de contrato, handler e retry.

## Boundaries & Constraints

**Always:** Manter tokens fora de logs; usar token individual da instância Evolution Go nas rotas autenticadas; validar `X-Chatwoot-Signature` em tempo constante sobre `timestamp.body`; rejeitar timestamp fora da janela; preservar compatibilidade de leitura do banco existente; usar erros com contexto e testes ao lado do código.

**Ask First:** Qualquer mudança destrutiva de schema, alteração que exija modificar Evolution Go ou Chatwoot upstream, ou ampliação para o fluxo WhatsApp → Chatwoot.

**Never:** Aceitar webhook sem assinatura quando a inbox possuir `secret`; usar `GLOBAL_API_KEY` para `/send/*`; registrar corpo, conteúdo, token, JID completo ou resposta externa bruta em logs/audit; implementar mídia múltipla, status de leitura ou grupos nesta entrega.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|---------------|----------------------------|----------------|
| Texto válido | `message_created`, outgoing, assinatura válida, JID válido | `POST /send/text` com token da instância e resposta 200 | Auditoria `ok` e idempotência `sent` |
| Mídia válida | Um anexo Chatwoot | `POST /send/media` com `url`, `type`, `filename`, `caption` | Erro upstream sanitizado e retry permitido |
| Assinatura inválida/expirada | Header ausente, digest inválido ou timestamp fora da janela | Nenhum efeito externo | 401 sem detalhes internos |
| Retry do mesmo evento | ID já concluído | Não reenviar | 200 idempotente |
| Primeira tentativa falha | Evolution Go retorna erro transitório | Evento permanece reenviável | Resposta não-2xx; retry posterior tenta novamente |
| Contato novo/existente | JID e inbox válidos | `contact_inbox.source_id` termina exatamente como JID | Erro de conflito tratado por busca/reuso |

</frozen-after-approval>

## Code Map

- `internal/evogo/client.go` e `types.go` -- rotas, autenticação e payloads Evolution Go.
- `internal/chatwoot/client.go`, `types.go` e `hmac.go` -- provisionamento, contatos e assinatura.
- `cmd/connect-cli/setup.go` e `add_contact.go` -- fluxo operacional de configuração.
- `internal/httpapi/webhook_chatwoot.go` -- autenticação e respostas HTTP.
- `internal/bridge/bridge.go` e `internal/store/store.go` -- dispatch, retry e idempotência.
- `scripts/smoke-e2e.sh`, `README.md` e `docs/` -- homologação e contratos documentados.

## Tasks & Acceptance

**Execution:**
- [x] `internal/evogo/*` -- implementar contratos 0.7.2 (`/send/text`, `/send/media`, `/instance/connect`, token por instância) e testes `httptest`.
- [x] `internal/chatwoot/*` -- interpretar resposta flat da inbox, usar `secret`, criar/reusar contato e `contact_inbox`, URL-encode de busca e testes de contrato.
- [x] `internal/chatwoot/hmac.go` + `internal/httpapi/webhook_chatwoot.go` -- verificar assinatura `sha256=` com timestamp e replay window, sem expor erros internos.
- [x] `internal/bridge/bridge.go` + `internal/store/store.go` -- impedir duplicação concorrente, permitir retry após falha e não ignorar falha de persistência crítica.
- [x] `cmd/connect-cli/*` -- tornar setup consistente e falhar claramente quando provisionamento ficar incompleto.
- [x] testes e documentação -- cobrir matriz, corrigir smoke test e declarar versões suportadas/limitações reais.

**Acceptance Criteria:**
- Given Evolution Go 0.7.2 e Chatwoot 4.16.2, when um agente envia texto em uma conversa provisionada, then o conector produz exatamente uma chamada válida a `/send/text`.
- Given uma falha transitória da Evolution Go, when o Chatwoot repete o webhook, then a mensagem é tentada novamente sem ser marcada falsamente como entregue.
- Given dois webhooks concorrentes com o mesmo ID, when ambos são processados, then no máximo um produz efeito externo.
- Given a suíte local, when build, vet, race tests, formatação e tidy são executados, then todos passam sem alterações pendentes geradas pelas ferramentas.

## Spec Change Log

## Design Notes

Fixar contratos por versão e manter fixtures representativas reduz regressões silenciosas. A idempotência deve adquirir a chave antes do efeito externo com operação atômica; falha transitória libera ou torna o claim retomável, enquanto `sent` permanece terminal.

## Verification

Automated validation on 2026-08-13: build, vet, race suite, formatting,
`go mod tidy -diff`, shell syntax, compose parsing and `git diff --check` passed.
The live VPS smoke remains an explicit pre-production gate because it requires
the operator's Evolution Go and Chatwoot instances.

**Commands:**
- `go build ./...` -- todos os binários compilam.
- `go vet ./...` -- nenhuma falha estática.
- `go test -race -count=1 ./...` -- suíte e concorrência verdes.
- `gofmt -l .` -- saída vazia.
- `go mod tidy -diff` -- saída vazia.

**Manual checks (if no CLI):**
- Executar o smoke E2E contra instâncias homologadas fixadas e confirmar texto, mídia, retry e rejeição de assinatura inválida.

## Suggested Review Order

**Entrada autenticada**

- Resolve inbox, valida assinatura temporal e retorna erros sem detalhes internos.
  [`webhook_chatwoot.go:33`](../../internal/httpapi/webhook_chatwoot.go#L33)

- Implementa HMAC `timestamp.body` com comparação constante e janela anti-replay.
  [`hmac.go:15`](../../internal/chatwoot/hmac.go#L15)

**Entrega e consistência**

- Orquestra filtro, JID, claim, rate limit, envio e conclusão transacional.
  [`bridge.go:136`](../../internal/bridge/bridge.go#L136)

- Adquire claims com fencing e distingue processamento de entrega concluída.
  [`store.go:324`](../../internal/store/store.go#L324)

- Persiste idempotência e auditoria atomicamente na mesma transação.
  [`store.go:361`](../../internal/store/store.go#L361)

**Contratos externos**

- Usa rotas 0.7.2, token individual, ID determinístico e redirects bloqueados.
  [`client.go:43`](../../internal/evogo/client.go#L43)

- Cria/reutiliza contatos e garante `contact_inbox.source_id` exato.
  [`client.go:96`](../../internal/chatwoot/client.go#L96)

- Modela payload real de anexos e envelopes do Chatwoot 4.16.2.
  [`types.go:8`](../../internal/chatwoot/types.go#L8)

**Provisionamento e upgrade**

- Valida token, cria tenant e migra credenciais antigas in-place.
  [`setup.go:65`](../../cmd/connect-cli/setup.go#L65)

- Normaliza JID e trata criação concorrente de contato.
  [`add_contact.go:45`](../../cmd/connect-cli/add_contact.go#L45)

**Evidência automatizada**

- Cobre texto, mídia, retry e duplicatas concorrentes.
  [`bridge_test.go:119`](../../internal/bridge/bridge_test.go#L119)

- Cobre assinatura inválida, expirada e fallbacks de inbox.
  [`webhook_chatwoot_test.go:74`](../../internal/httpapi/webhook_chatwoot_test.go#L74)

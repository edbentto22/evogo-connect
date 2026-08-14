---
title: Bridge WhatsApp para Chatwoot
type: feature
created: 2026-08-13
status: done
baseline_commit: 46f9b88
context:
  - docs/security.md
  - .context/plans/2026-08-12-evogo-chatwoot-connector.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** mensagens recebidas no WhatsApp chegam à Evolution Go, mas não
entram no Chatwoot porque o conector não possui endpoint ou processamento para
o sentido Evolution Go → Chatwoot.

**Approach:** implementar um endpoint por instância, registrar a URL de webhook
na Evolution Go, aceitar apenas mensagens diretas recebidas, deduplicá-las e
criar/reutilizar o contato e a conversa API no Chatwoot antes de publicar a
mensagem como incoming.

## Boundaries & Constraints

**Always:** aceitar somente texto direto de `messages.upsert`/`MESSAGE` com
`fromMe=false`; validar JID; limitar o corpo; respeitar kill switch, rate limit,
idempotência e audit sem conteúdo ou telefone em claro; armazenar o segredo
cifrado no banco; manter Chatwoot → WhatsApp funcionando; retornar 2xx apenas
para entrega concluída, duplicata ou evento ignorado.

**Ask First:** Evolution Go 0.7.2 documenta URL de webhook, mas não um header
secreto configurável. Autorizar URL por instância com segredo aleatório no
caminho (`/webhook/evo/<instância>/<segredo>`), validado em tempo constante e
nunca incluído nos logs do conector. O segredo ainda pode existir nos registros
de acesso do proxy; a VPS/Coolify deve restringir acesso aos logs. Alternativa
mais forte exige um controle externo de rede/proxy não fornecido pelo conector.

**Never:** não aceitar grupos, mensagens próprias, eventos de status ou mídia
nesta entrega; não usar token da instância como segredo público; não registrar
conteúdo, nome ou JID completo; não alterar registros de clientes já existentes
fora do contato relacionado à mensagem; não configurar um webhook global da
Evolution Go.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|---------------|---------------------------|----------------|
| Texto recebido | Evento `messages.upsert` ou `MESSAGE`, JID direto, `fromMe=false` | Contato/vínculo e conversa abertos no Chatwoot; mensagem `incoming` criada uma vez | 200 após auditoria e idempotência completas |
| Repetição | Mesmo ID da Evolution | Nenhuma segunda mensagem no Chatwoot | 200 com duplicata ignorada |
| Evento não suportado | Grupo, `fromMe=true`, mídia ou tipo desconhecido | Sem efeito externo | 200 com evento ignorado |
| Autenticação inválida | Segredo de URL ausente ou incorreto | Nenhum acesso ao bridge | 401 sem revelar segredo |
| Falha Chatwoot | API indisponível ou rejeita criação | Evolution pode tentar novamente | 503, claim liberado e audit de erro |

</frozen-after-approval>

## Code Map

- `internal/evogo/types.go` -- envelope e dados de mensagem recebida.
- `internal/evogo/client.go` -- configuração por instância via `/instance/connect`.
- `internal/httpapi/router.go` -- registro do endpoint de entrada.
- `internal/httpapi/webhook_chatwoot.go` -- referência de limite de body, timeout e respostas seguras.
- `internal/bridge/bridge.go` -- rate limit, idempotência, audit e bridge reverso existente.
- `internal/chatwoot/client.go` -- ciclo de contato, conversa e mensagem incoming.
- `internal/store/store.go` -- tenants e estado transacional de idempotência.
- `cmd/connect-cli/setup.go` -- provisionamento e armazenamento cifrado por tenant.

## Tasks & Acceptance

**Execution:**
- [x] `internal/evogo/` -- normalizar o contrato de webhook Evolution Go e configurar somente eventos de mensagens necessárias.
- [x] `internal/store/` e `cmd/connect-cli/` -- gerar/persistir o segredo por tenant e atualizar de forma segura tenants já configurados.
- [x] `internal/httpapi/` -- expor e autenticar endpoint por instância, com limite de body e respostas de retry adequadas.
- [x] `internal/bridge/` -- processar texto recebido, manter idempotência/audit/rate limit e publicar no Chatwoot.
- [x] `internal/chatwoot/` -- garantir contato, vínculo, conversa e criação incoming usando contratos compatíveis com Fazer.ai.
- [x] `internal/*/*_test.go` -- cobrir matriz de I/O, autenticação, duplicata e falha transitória.
- [x] `docs/` -- documentar configuração no Coolify, smoke test e limites de segurança.

**Acceptance Criteria:**
- Given uma instância configurada com URL segura, when um cliente envia texto direto no WhatsApp, then uma única mensagem incoming aparece na inbox correta do Chatwoot.
- Given o mesmo webhook é reenviado, when o conector o recebe, then o Chatwoot não recebe mensagem duplicada.
- Given o endpoint recebe segredo inválido, when qualquer payload é enviado, then retorna 401 e não acessa Chatwoot ou Evolution.
- Given o Chatwoot está temporariamente indisponível, when chega mensagem válida, then o conector retorna resposta reenviável sem marcar entrega como concluída.

## Design Notes

O contrato oficial da Evolution Go descreve `POST /instance/connect` autenticado
com o token individual, `webhookUrl` e assinatura apenas dos eventos de mensagem.
Como não há prova de suporte a cabeçalho customizado nessa rota, o segredo na URL
é uma compensação explícita, não uma equivalência a HMAC. A implementação deve
documentar como rotacioná-lo e não deve exibir a URL completa em status, logs ou
erros. [Documentação de instâncias](https://github.com/evolution-foundation/evolution-go/blob/main/docs/wiki/guias-api/api-instances.md)

## Verification

**Commands:**
- `go build ./...` -- esperado: compilação verde.
- `go vet ./...` -- esperado: análise verde.
- `go test -race -count=1 ./...` -- esperado: testes da matriz verdes.
- `go mod tidy -diff` e `gofmt -l .` -- esperado: nenhuma alteração pendente.

**Manual checks:**
- No Coolify, configurar o webhook somente pela CLI do conector; enviar texto do WhatsApp e confirmar a mensagem incoming na inbox correspondente.

## Suggested Review Order

**Entrada e segurança**

- Autentica a rota por instância e traduz falhas para retries controlados.
  [`webhook_evogo.go:20`](../../../internal/httpapi/webhook_evogo.go#L20)

- Configura URL segura, valida a resposta upstream e permite rotação explícita.
  [`setup.go:185`](../../../cmd/connect-cli/setup.go#L185)

**Entrega e consistência**

- Aplica filtros, idempotência, auditoria e entrega incoming no Chatwoot.
  [`bridge.go:306`](../../../internal/bridge/bridge.go#L306)

- Reutiliza uma conversa aberta e pagina respostas Fazer.ai compatíveis.
  [`client.go:174`](../../../internal/chatwoot/client.go#L174)

- Impede que duas inboxes compartilhem a mesma instância Evolution.
  [`002_unique_evo_instance.up.sql:2`](../../../migrations/002_unique_evo_instance.up.sql#L2)

**Contrato e operação**

- Aceita texto simples e estendido, descartando eventos não conversacionais.
  [`types.go:29`](../../../internal/evogo/types.go#L29)

- Exercita autenticação, retry, duplicata e contratos de integração.
  [`webhook_chatwoot_test.go:83`](../../../internal/httpapi/webhook_chatwoot_test.go#L83)

- Explica o re-provisionamento seguro para tenants existentes no Coolify.
  [`coolify.md:151`](../../../docs/coolify.md#L151)

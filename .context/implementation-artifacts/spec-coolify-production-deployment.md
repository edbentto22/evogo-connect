---
title: 'Criar deploy de produção simples para Coolify'
type: 'feature'
created: '2026-08-13'
status: 'done'
baseline_commit: '8ceb3fd4f23e8e668ffb6904a066f3169d55874a'
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/.context/plans/2026-08-12-evogo-chatwoot-connector.md'
  - '{project-root}/docs/security.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O compose atual é voltado a desenvolvimento: publica o Postgres, adiciona um Caddy que duplica o proxy do Coolify e depende de configuração manual sujeita a erro. Isso impede tratar a implantação na VPS como simples e pronta para produção.

**Approach:** Entregar um compose exclusivo para Coolify que gere e preserve os segredos pelo próprio stack, mantenha o banco privado e persistente, exponha somente o conector pelo domínio/TLS do Coolify e seja acompanhado por um guia curto de implantação, provisionamento, backup e validação.

## Boundaries & Constraints

**Always:** Usar o Docker Compose como fonte de verdade; manter o compose local existente; usar volume nomeado persistente; não publicar porta do Postgres; executar migrations automaticamente; declarar healthcheck real em `/readyz`; manter segredos apenas em variáveis runtime persistentes do Coolify; indicar que `CONNECT_MASTER_KEY` é indispensável para restaurar dados cifrados; documentar exatamente onde informar o domínio com porta interna `9090` e como executar `/app/connect` no terminal do serviço.

**Ask First:** Suportar versão antiga do Coolify sem magic environment variables; trocar o Postgres embutido por banco gerenciado; alterar schema, contratos HTTP, criptografia ou comportamento do bridge; qualquer migração destrutiva de volume.

**Never:** Incluir Evolution Go ou Chatwoot no mesmo stack; manter Caddy/Traefik próprio no pacote Coolify; expor `5432` no host; fixar senha/token no repositório; enviar segredos como build args; anunciar produção aprovada antes do smoke com as instâncias reais; implementar WhatsApp → Chatwoot nesta entrega.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|---------------|----------------------------|----------------|
| Primeiro deploy | Git source em Coolify atual e compose selecionado | Coolify gera segredos estáveis, inicia Postgres privado, migra e marca o conector healthy | Variável/config inválida impede saúde e aparece claramente nos logs |
| Domínio público | Domínio HTTPS associado ao serviço na porta interna 9090 | `/healthz`, `/readyz` e webhook chegam pelo proxy/TLS do Coolify | Guia explica DNS, porta interna e diagnóstico de 404/502 |
| Rede privada | Inspeção do compose e containers | Só o conector recebe rota pública; Postgres é acessível apenas pelo nome `postgres` na rede do stack | Validador falha se houver publicação de porta do banco |
| Redeploy/restart | Stack já possui tenant e volume | Banco, segredos e IDs permanecem; migrations aditivas rodam sem recriar dados | Guia proíbe regenerar master key e orienta backup antes de upgrade |
| Provisionamento | Serviços healthy e credenciais externas válidas | `/app/connect setup` registra ou atualiza tenant pelo terminal do conector | Falhas externas não são descritas como deploy concluído; operador corrige e repete com o mesmo nome |
| Restore | Backup do volume/DB e chave original disponíveis | Tenant volta a ser decifrável e o bridge inicia normalmente | Sem chave original, guia declara o restore dos segredos irrecuperável |

</frozen-after-approval>

## Code Map

- `deploy/docker-compose.yml` e `deploy/Caddyfile` -- referências locais que não devem ser usadas como stack de produção no Coolify.
- `deploy/Dockerfile` e `cmd/evogo-connect/main.go` -- imagem Alpine mínima não-root com CLI operacional, migrations automáticas e healthcheck binário.
- `internal/config/config.go` e `internal/crypto/aesgcm.go` -- requisitos exatos de runtime e chave AES-256 em base64.
- `cmd/connect-cli/*` -- provisionamento executável como `/app/connect` dentro do serviço.
- `docs/operations.md`, `docs/security.md`, `docs/USAGE.md` e `README.md` -- contratos operacionais que ainda descrevem o deploy local.

## Tasks & Acceptance

**Execution:**
- [x] `deploy/docker-compose.coolify.yml` -- criar stack `connector + postgres` com magic environment variables estáveis, volume, rede privada, dependência saudável e sem proxy interno.
- [x] `.dockerignore` e `deploy/Dockerfile` -- reduzir contexto de build e manter imagem/healthcheck compatíveis com Coolify sem incluir artefatos ou segredos locais.
- [x] `scripts/validate-coolify-compose.sh` e `Makefile` -- validar resolução do compose, segredos obrigatórios, persistência, healthchecks e ausência de porta pública no banco.
- [x] `docs/coolify.md` -- fornecer fluxo guiado do zero, provisionamento, smoke, backup/restore, upgrade e troubleshooting.
- [x] `README.md`, `docs/USAGE.md`, `docs/operations.md` e `docs/security.md` -- apontar o pacote de produção correto e remover instruções conflitantes.

**Acceptance Criteria:**
- Given um Coolify compatível com magic variables e o repositório importado, when o operador seleciona `deploy/docker-compose.coolify.yml` e associa `https://dominio:9090` ao serviço `connector`, then o stack sobe sem precisar criar manualmente senhas internas.
- Given o compose renderizado, when a validação automatizada é executada, then ela comprova volume persistente, healthchecks e nenhuma publicação de porta no Postgres.
- Given um redeploy sem apagar o volume nem alterar as magic variables, when o conector reinicia, then `/readyz` volta a 200 e os tenants permanecem utilizáveis.
- Given o guia, when um operador segue a sequência, then ele consegue distinguir “stack saudável” de “integração homologada” e só libera tráfego após o smoke real.

## Spec Change Log

## Design Notes

O pacote exige no mínimo Coolify `v4.0.0-beta.411`, versão em que magic variables também são suportadas para compose vindo de Git; cada versão instalada ainda precisa passar pela validação. `SERVICE_PASSWORD_64_POSTGRES` evita caracteres problemáticos no DSN, `SERVICE_REALBASE64_32_CONNECTOR` fornece a chave AES-256 e `SERVICE_HEX_64_ADMIN` fornece o token administrativo; esses valores persistem entre deployments e podem ser guardados para disaster recovery. O domínio usa `https://host:9090`: a porta seleciona o destino dentro do container, enquanto o proxy do Coolify publica HTTPS normalmente.

O compose não declara redes customizadas: o Coolify cria a rede isolada do recurso e conecta seu proxy, evitando seleção intermitente de um IP inacessível. O runtime Alpine opera como usuário não-root, com filesystem read-only, capabilities removidas e `no-new-privileges`, preservando o terminal necessário para `/app/connect` sem abrir escrita no container.

## Verification

Validação automatizada em 2026-08-13: compose renderizado, build, vet, suíte
com race detector, formatação, tidy, sintaxe shell e whitespace passaram. O
build real da imagem não foi executado porque o daemon Docker local estava
desligado. Deploy, TLS, redeploy, restore e smoke com serviços reais permanecem
gates explícitos no Coolify antes da liberação.

**Commands:**
- `bash scripts/validate-coolify-compose.sh` -- compose válido, privado, persistente e saudável.
- `go build ./... && go vet ./...` -- binários e análise estática verdes.
- `go test -race -count=1 ./...` -- suíte existente sem regressões.
- `gofmt -l .` e `go mod tidy -diff` -- nenhuma alteração pendente.
- `git diff --check` -- documentação e YAML sem erros de whitespace.

**Manual checks:**
- Em Coolify, confirmar `connector` healthy, `postgres` sem domínio/porta pública e `/readyz` 200 pelo domínio HTTPS.
- Executar `/app/connect setup` e o smoke E2E com Chatwoot 4.16.2 e Evolution Go 0.7.2 reais antes de liberar produção.

## Suggested Review Order

**Arquitetura e isolamento**

- Define o stack mínimo, segredos estáveis, pausa segura e Postgres sem porta pública.
  [`docker-compose.coolify.yml:1`](../../deploy/docker-compose.coolify.yml#L1)

- Mantém terminal operacional com runtime mínimo e usuário sem privilégios.
  [`Dockerfile:13`](../../deploy/Dockerfile#L13)

**Jornada operacional**

- Guia o primeiro deploy sem redes customizadas nem segredos em build args.
  [`coolify.md:12`](../../docs/coolify.md#L12)

- Provisiona credenciais temporárias sem expô-las nos argumentos do processo.
  [`coolify.md:65`](../../docs/coolify.md#L65)

- Separa saúde do stack do smoke real e da liberação de tráfego.
  [`coolify.md:105`](../../docs/coolify.md#L105)

- Torna backup e restore verificáveis, exclusivos e dependentes da chave original.
  [`coolify.md:125`](../../docs/coolify.md#L125)

**Provisionamento seguro**

- Resolve tokens pelo ambiente mantendo flags apenas para compatibilidade.
  [`setup.go:64`](../../cmd/connect-cli/setup.go#L64)

- Centraliza a precedência flag/env e falha claramente quando falta credencial.
  [`setup.go:161`](../../cmd/connect-cli/setup.go#L161)

**Evidência e orientação**

- Valida estruturalmente segredo compartilhado, persistência, isolamento e hardening.
  [`validate-coolify-compose.sh:56`](../../scripts/validate-coolify-compose.sh#L56)

- Cobre precedência e ausência das novas credenciais de ambiente.
  [`setup_test.go:9`](../../cmd/connect-cli/setup_test.go#L9)

- Declara o pacote como candidato até o smoke E2E na VPS.
  [`README.md:18`](../../README.md#L18)

---
title: 'Corrigir contexto de build no Coolify'
type: 'bugfix'
created: '2026-08-13'
status: 'done'
route: 'one-shot'
context: []
---

# Corrigir contexto de build no Coolify

## Intent

**Problem:** O Coolify executa o compose com a raiz clonada como `--project-directory`; `context: ..` fazia o build procurar `/artifacts/deploy` fora do checkout e falhar antes de abrir o Dockerfile.

**Approach:** Resolver build context e Dockerfile a partir da raiz do repositório e fazer o validador local reproduzir a mesma semântica de caminhos usada pelo Coolify.

## Suggested Review Order

**Correção de deploy**

- Ancora o contexto e o Dockerfile na raiz clonada pelo Coolify.
  [`docker-compose.coolify.yml:3`](../../deploy/docker-compose.coolify.yml#L3)

**Proteção contra regressão**

- Reproduz `--project-directory` e exige os caminhos-fonte portáteis.
  [`validate-coolify-compose.sh:19`](../../scripts/validate-coolify-compose.sh#L19)

- Documenta a invocação correta fora da plataforma.
  [`coolify.md:24`](../../docs/coolify.md#L24)

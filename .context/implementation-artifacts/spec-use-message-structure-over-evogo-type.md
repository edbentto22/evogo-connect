---
title: Aceitar texto pela estrutura Evolution Go
type: bugfix
created: 2026-08-13
status: done
route: one-shot
---

# Aceitar texto pela estrutura Evolution Go

## Intent

**Problem:** a Evolution Go em produção varia `messageType` e fazia o conector
descartar uma mensagem que continha texto direto válido.

**Approach:** aceitar somente as estruturas textuais `conversation` e
`extendedTextMessage.text`, sem confiar no rótulo variável do upstream.

## Suggested Review Order

1. `../../../internal/evogo/types.go`
2. `../../../internal/evogo/types_test.go`

---
title: Registrar motivo de descarte Evolution Go
type: bugfix
created: 2026-08-13
status: done
route: one-shot
---

# Registrar motivo de descarte Evolution Go

## Intent

**Problem:** eventos recebidos pela Evolution podem ser retornados como 200
quando ignorados, sem indicar no log qual parte do contrato não foi aceita.

**Approach:** registrar somente evento, tipo e motivo técnico do descarte, sem
conteúdo, telefone ou segredo.

## Suggested Review Order

1. `../../../internal/bridge/bridge.go`

---
description: 静的検査優先と AI 検査補完の規約 (FLM_GEN_0004)
paths:
  - cli/internal/check/**
  - cli/internal/root/check/**
  - .markdownlint*
  - .shellcheckrc
  - .yamllint*
  - .golangci.yaml
  - .claude/agents/*-reviewer.md
  - plugins/**/agents/*-reviewer.md
  - docs/adr/**/*.md
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/static-check-priority.md](../../vendor/flame/.claude/rules/static-check-priority.md)

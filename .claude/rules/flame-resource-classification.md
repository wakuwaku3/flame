---
description: flame の資産を internal と downstream に分類し vendor/flame/ 配下を downstream の SoT とする規約 (FLM_GEN_0007)
paths:
  - docs/adr/**
  - vendor/flame/**
  - "**/CLAUDE.md"
  - .envrc
  - .yamllint
  - .markdownlint-cli2.yaml
  - flame.yaml
  - flame.lock
  - .claude/rules/**/*.md
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/resource-classification.md](../../vendor/flame/.claude/rules/resource-classification.md)

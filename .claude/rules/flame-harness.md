---
description: flame harness を 3 チャネル分散 (Claude Code plugin / reusable workflow / vendor) で配布する規約 (FLM_FEA_0003)
paths:
  - vendor/flame/**
  - flame.yaml
  - flame.lock
  - "**/*.flame-overlay"
  - "**/*.flame-overlay.*"
  - .claude-plugin/**
  - plugins/**
  - .github/workflows/wf__*.yaml
  - .github/workflows/flame-trg__*.yaml
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/harness.md](../../vendor/flame/.claude/rules/harness.md)

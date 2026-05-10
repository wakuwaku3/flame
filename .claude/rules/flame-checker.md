---
description: checker — 静的検査の単位と hook / CI の共通実装 (FLM_FEA_0001)
paths:
  - cli/internal/check/**
  - cli/internal/root/check/**
  - cli/internal/root/ai/hook/**
  - .github/workflows/wf__check*.yaml
  - vendor/flame/.github/workflows/trg__pull_request__opened_synchronize_reopened.yaml
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/checker.md](../../vendor/flame/.claude/rules/checker.md)

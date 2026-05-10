---
description: 配布対象の release policy (FLM_FEA_0004)
paths:
  - .github/workflows/wf__deploy*.yaml
  - "**/cmd/*_tool/**"
  - "**/*_lib/**"
  - lib/**
  - "**/scripts/install.sh"
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/release-policy.md](../../vendor/flame/.claude/rules/release-policy.md)

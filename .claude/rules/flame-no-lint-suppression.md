---
description: lint の局所抑制を最小化する (FLM_GEN_0006)
paths:
  - "**/*.sh"
  - "**/*.md"
  - "**/*.yaml"
  - "**/*.yml"
  - "**/*.go"
  - .shellcheckrc
  - .markdownlint*
  - .yamllint*
  - actionlint.yaml
  - "**/.golangci.yaml"
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/no-lint-suppression.md](../../vendor/flame/.claude/rules/no-lint-suppression.md)

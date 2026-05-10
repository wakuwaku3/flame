---
description: 開発環境マネージャ devbox + direnv の採用規約 (FLM_ENG_0002)
paths:
  - devbox.json
  - devbox.lock
  - devbox/**/*
  - .envrc
  - scripts/**/*.sh
  - .github/scripts/**/*.sh
  - .github/workflows/**/*.yaml
  - .claude/skills/**/SKILL.md
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/devbox.md](../../vendor/flame/.claude/rules/devbox.md)

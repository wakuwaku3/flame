---
description: AI 開発における 3 層フィードバックループ (FLM_GEN_0003)
paths:
  - cli/internal/root/ai/hook/**
  - plugins/**/hooks/**
  - plugins/**/agents/**
  - .claude/settings.json
  - .claude/agents/**/*.md
  - .github/workflows/**/*.yml
  - .github/workflows/**/*.yaml
---

# vendor SoT 側 rule への参照

本 rule は flame self における [FLM_GEN_0007](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub に従う stub。 vendor SoT 側 rule の内容に従う:

[vendor/flame/.claude/rules/feedback-loop.md](../../vendor/flame/.claude/rules/feedback-loop.md)

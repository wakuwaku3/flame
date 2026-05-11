---
description: flame self における Go module 配布の release 経路規範 (FLI_APP_0001)
paths:
  - "**/go.mod"
  - "**/go.sum"
  - "**/go.work"
  - "**/go.work.sum"
  - .golangci.yaml
  - .gitignore
  - .github/workflows/*.yaml
  - "**/scripts/install.sh"
---

# FLI_APP_0001

[FLI_APP_0001: flame self では module 配布対象の依存を release 経路に固定し独立 PR sequence を強制する](../../docs/adr/application/FLI_APP_0001__go_module_resolution.md)

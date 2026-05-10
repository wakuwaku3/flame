---
name: go
description: Go ファイル (`*.go` / `go.mod` / `go.sum`) を作成・編集する工程 (FLM_APP_0007) で起動する。
---

# go skill

参照 ADR: [FLM_APP_0007: Go を主開発言語として採用する](../../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md)

## 起動条件

`*.go` / `go.mod` / `go.sum` を作成または編集する工程で必ず起動する。新規 Go module を作成する工程 (新しい `go.mod` を置く工程) も対象。

## 手順

ADR の規約に従ったうえで、本 skill が起動した工程では以下の procedural を完了させる。

### 1. 検査を通す

```sh
flame check go lint <変更ファイル>...
flame check go build <変更ファイル>...
flame check go test <変更ファイル>...
```

`flame check go` の各 subcommand (lint / build / test) で Go 種別 checker が dispatch される (詳細は [FLM_APP_0007](../../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md))。

### 2. 動作確認の結果を完了報告に含める

`flame check go <subcommand>` の終了コードと出力 (どの checker が走ったか / 違反内容) を完了報告に明示する。

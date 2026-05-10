# JSON ファイルの基本ルール

## 背景

- flame では設定・メタデータの表現に JSON を使う場面がある (例: `devbox.json`、`.claude/settings.json`)
- JSON には W3C / ECMA で標準化された構文仕様があり、構文妥当性を機械的に検査できる
- 軽量な JSON 構文検査ツール (`jq`) は CLI として広く普及しており devbox からも導入できる
- flame は静的にチェックできるルールを静的チェックで担保する方針である ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame では JSON ファイルを以下のルールで扱う。

### 配置

- リポジトリで横断的に使う設定 JSON はリポジトリルートまたは関連ツール所定の位置に配置する
- 特定モジュール / 特定コードベースに密結合した JSON は当該モジュールのディレクトリに配置する

### フォーマット

- JSON ファイルは拡張子 `.json` で記述する
- 文字エンコーディングは UTF-8 とする

### lint

- 全 JSON ファイルは JSON 構文 lint の検査対象とする
- 検査は flame の FB ループ規約 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) に従い hook と CI の両方で実行する

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (汎用 AI による生成で十分) |
| lint | 整備 (`scripts/check-json.sh` から JSON 構文検査を実行) |
| build | 省略 (静的データのため出力生成の概念がない) |
| test | 省略 (build 成果物が存在しないため) |
| ADR ルール検査 skill | 省略 (lint で完結する範囲に限定) |

## 影響

- リポジトリに JSON 構文 lint ツール (`jq` 等) の導入・バージョン管理コストが発生する
- 構文不正な JSON は hook / CI 段階で検出され merge 前に修正される
- スキーマ単位での意味検査 (例: JSON Schema による型検査) は本 ADR の対象外であり、必要になった時点で別途 ADR を起こす
- 整形 (インデント幅、キー順) のフォーマッタ統一は本 ADR の対象外であり、必要になった時点で別途 ADR を起こす
- AI が新規 JSON ファイルを生成する際に拡張子・配置・構文妥当性の規約に従う圧力が働く

## 評価

代替案として以下を検討した。

- **lint を導入しない**: 構文ミスが AI / 人間のレビュー時にしか検出されず、flame の静的チェック優先方針 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) と整合しない。
- **JSON Schema による意味検査までを ADR の必須範囲にする**: 現時点で flame に導入されている JSON ファイルは数件の設定ファイルに限られ、スキーマを書くコストが効果に見合わない。スキーマ検査が必要な JSON が増えた段階で別 ADR で導入する。
- **JSON フォーマッタを ADR で固定する**: 現状 JSON ファイルが少なく、フォーマッタの選択 (jq / dprint / prettier 等) を固定するメリットが薄い。整形差分が問題化した段階で別 ADR で導入する。
- **JSON を採用せず別フォーマット (YAML / TOML 等) に統一する**: `devbox.json`・`.claude/settings.json` など JSON が必須の外部ツール設定が存在し、JSON を排除できない。むしろ JSON / YAML / TOML の使い分けは外部ツールの要求に従う。

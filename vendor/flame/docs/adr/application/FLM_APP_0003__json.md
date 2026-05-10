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

### flame 独自型の schema 規約

flame が SoT として独自に型を定義する JSON ファイル (= 利用側に伝播する flame 規約として固有のフィールド・構造を持つ JSON。 現時点で該当ファイルは存在しないが将来予約) を以下の policy で扱う。

- 当該 JSON ファイルには **JSON Schema 仕様** で記述した schema を併設する
- schema 自体のシリアライズ形式は対象ファイルと同言語 (= JSON。 本 ADR の JSON 採用方針と整合する)
- schema の配置先は flame harness の SoT 規約 ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §schema の機械可読化) に従う
- 対象 JSON ファイルの top-level に schema 参照を必須化する (具体プロパティ名は JSON Schema 仕様に従う)
- 外部ツールが SoT として型を定義する JSON (例: `devbox.json` / `.claude/settings.json`) は本規約の対象外とする (= flame 側からは schema 併設を強制しない。 当該ツール側の schema を IDE で参照してよい)

なお、 同等の規約は YAML 側にも対称的に適用する ([FLM_APP_0004](FLM_APP_0004__yaml.md) §flame 独自型の schema 規約)。

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
- スキーマ単位での意味検査 (例: JSON Schema による型検査) のうち、 flame 独自型に対する schema 必須化と検査経路は §flame 独自型の schema 規約 で扱う。 外部ツールが定義する型に対する schema 検査は本 ADR の対象外であり、 必要になった時点で別途 ADR を起こす
- §flame 独自型の schema 規約 の具体実装は以下に従う ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §schema の機械可読化 と整合):
  - schema 配置先 path: `vendor/flame/schemas/<対象 file 名>.schema.json`
  - top-level の schema 参照: JSON Schema 仕様に従い `"$schema": "<schema への相対 path>"` property を付ける
- §flame 独自型の schema 規約 の運用は `flame check json` の以下 2 経路の lint 拡張で機械的に強制される (= 規範: 利用側 repo にも flame CLI 経由で同経路が適用される):
  1. **schema 参照の存在検査**: flame 独自型 (= `vendor/flame/schemas/<name>.schema.json` が併設されている JSON ファイル) に対して、 top-level の `"$schema"` property が付いていることを検査する。 property 不在 / 別 path を指す参照はいずれも fail
  2. **schema validation**: 上記 `"$schema"` 経由で参照される schema を load し、 当該 JSON ファイルが schema に conform するかを検査する。 検査対象データから top-level `"$schema"` property を validation 入力前に取り除く (= IDE / lint 用 metadata であり対象 type の data ではないため、 各 schema 著者が `$schema` を allowed property として明示する負担を避ける)
  対象 JSON ファイルの判定 (= flame 独自型か否か) は対応 schema の存在で行う (= `vendor/flame/schemas/<basename>.schema.json` を search-upward で探し、 実在すれば flame 独自型と判定)。 現時点で flame 独自定義の JSON 型は存在しないため、 lint 経路は将来対象が登場した時点で素通しの no-op から実検査に移行する (= 対象 schema を `vendor/flame/schemas/` に置くだけで自動的に検査が有効化される)
- 上記 lint 経路の flame self における実装手段 (= 利用側 repo は flame CLI バイナリを GitHub Release から install するため当該 library 選定の意思決定に関与しない / 参考情報): `github.com/santhosh-tekuri/jsonschema/v6` (Draft 2020-12 対応 JSON Schema validator) を `cli/` モジュールに追加。 実装は `cli/internal/check/schemavalidate/` の helper package と `cli/internal/root/check/json/` の subcommand の協調で完結する
- 整形 (インデント幅、キー順) のフォーマッタ統一は本 ADR の対象外であり、必要になった時点で別途 ADR を起こす
- AI が新規 JSON ファイルを生成する際に拡張子・配置・構文妥当性の規約に従う圧力が働く

## 評価

代替案として以下を検討した。

- **lint を導入しない**: 構文ミスが AI / 人間のレビュー時にしか検出されず、flame の静的チェック優先方針 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) と整合しない。
- **JSON Schema による意味検査を全 JSON ファイルに必須化する**: flame は外部ツール (devbox / Claude Code 等) が SoT として型を定義する JSON を多く扱うが、 これらの schema は外部ツール側が提供しており flame 側で再定義する義務は無い。 schema 必須化の対象を「flame が独自に SoT として型を定義する JSON」 に絞る形 (= §flame 独自型の schema 規約) を採用した
- **flame 独自型に schema 併設を必須化しない (任意とする)**: schema を書くコストを発生させない利点はあるが、 flame 独自型は利用側に伝播する規約として機械的に検査可能であるべきで、 任意化は §flame 独自型の schema 規約 の主旨と相容れない。 本 ADR の対象を「flame が SoT として独自定義する型」 に絞ることで対象数を抑え、 その範囲では schema 必須を採用した
- **JSON フォーマッタを ADR で固定する**: 現状 JSON ファイルが少なく、フォーマッタの選択 (jq / dprint / prettier 等) を固定するメリットが薄い。整形差分が問題化した段階で別 ADR で導入する。
- **JSON を採用せず別フォーマット (YAML / TOML 等) に統一する**: `devbox.json`・`.claude/settings.json` など JSON が必須の外部ツール設定が存在し、JSON を排除できない。むしろ JSON / YAML / TOML の使い分けは外部ツールの要求に従う。

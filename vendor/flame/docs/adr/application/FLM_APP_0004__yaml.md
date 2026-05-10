# YAML ファイルの基本ルール

## 背景

- flame では設定の表現に YAML を使う場面がある (例: `.markdownlint-cli2.yaml`)
- YAML には公式仕様 (yaml.org/spec) があり、構文・スタイル両面を機械的に検査できる
- YAML 構文・スタイル lint ツール (`yamllint`) は CLI として広く普及しており devbox からも導入できる
- YAML は JSON と異なり、インデント・引用符・真偽値表記 (`yes`/`no` 等) の曖昧さに起因する事故が起きやすく、スタイル lint の意義が大きい
- flame は静的にチェックできるルールを静的チェックで担保する方針である ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame では YAML ファイルを以下のルールで扱う。

### 配置

- リポジトリで横断的に使う設定 YAML はリポジトリルートまたは関連ツール所定の位置に配置する
- 特定モジュール / 特定コードベースに密結合した YAML は当該モジュールのディレクトリに配置する

### フォーマット

- YAML ファイルは拡張子 `.yaml` で記述する
- 外部ツールが `.yaml` を読まないと判明している場合に限り、当該ツールに合わせて `.yml` を許容する
- 文字エンコーディングは UTF-8 とする

### lint

- 全 YAML ファイルは YAML 構文 + スタイル lint の検査対象とする
- 検査は flame の FB ループ規約 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) に従い hook と CI の両方で実行する
- lint の警告レベルも失敗扱いとする (warning を放置しない)

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (汎用 AI による生成で十分) |
| lint | 整備 (`scripts/check-yaml.sh` から YAML 構文 + スタイル検査を実行) |
| build | 省略 (静的データのため出力生成の概念がない) |
| test | 省略 (build 成果物が存在しないため) |
| ADR ルール検査 skill | 省略 (lint で完結する範囲に限定) |

## 影響

- リポジトリに YAML lint ツール (`yamllint` 等) の導入・バージョン管理コストが発生する
- 構文不正・スタイル違反な YAML は hook / CI 段階で検出され merge 前に修正される
- lint ツールのデフォルトルール (例: ファイル先頭の `---`、行末空白の禁止、真偽値の正規化) を初期採用すると、既存ファイルが警告対象となる場合があり、本 ADR 導入時に既存ファイルの追従が必要になる
- 拡張子は原則 `.yaml` だが、`.yaml` を読まない外部ツールに合わせて `.yml` を採用したファイルが発生した場合も検査対象に含める
- スキーマ単位での意味検査 (例: JSON Schema / Cue による型検査) は本 ADR の対象外であり、必要になった時点で別途 ADR を起こす
- リポジトリ専用の lint 設定 (`.yamllint`) を repo root に配置し、 `extends: default` で yamllint の default ruleset を継承したうえで、 flame の構造的事情と衝突する以下 2 件を [FLM_GEN_0006](../general/FLM_GEN_0006__no_lint_suppression.md) §グローバル無効化 経路で設定ファイル側で調整する:
  - `truthy: disable` — yamllint の `truthy` rule は YAML 1.1 互換解釈で `on` / `yes` / `no` / `off` 等を boolean として扱い、 GitHub Actions の予約キー `on:` を always-true な boolean と誤検出する。 GitHub Actions workflow file では `on:` が必須 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) のため、 当該 rule は構造的 false positive となる
  - `line-length.max: 120` — default の line-length 上限 (80) では action SHA pinning で `uses: <owner>/<action>@<40-char SHA>` の 1 行が超過する。 SHA pinning は flame の workflow 規約 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) で必須のため、 SHA pinning 込みで収まる 120 に拡張する

## 評価

代替案として以下を検討した。

- **lint を導入しない**: 構文ミス・インデント崩れ・真偽値の曖昧表記が AI / 人間のレビュー時にしか検出されず、flame の静的チェック優先方針 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) と整合しない。
- **lint で warning を許容する (strict 化しない)**: 警告レベルの指摘が放置され、lint の品質保証としての効力が弱まる。flame では warning も失敗扱いにすることで lint の意味を維持する。
- **拡張子を `.yml` に統一する / `.yaml` に統一する**: 多くの YAML 対応ツール (GitHub Actions / markdownlint-cli2 等) は `.yaml` / `.yml` の両方を読むが、ツールによっては片方しか読まないものも存在しうる。`.yaml` 一本化を強制すると `.yaml` を読まないツールに対応できないため、原則 `.yaml`、`.yaml` を読まない外部ツールに合わせる場合のみ `.yml` を許容する形を採用した。
- **リポジトリ専用 yamllint 設定を持たない (lint default のみ)**: 一見 学習コストが低いが、 GitHub Actions の `on:` キーが yamllint default の `truthy` rule で誤検出されるため flame の workflow 規約と構造的に衝突し、 また action SHA pinning の 40-char SHA が default line-length (80) を超えるため SHA pinning 規約と衝突する。 default のままでは flame で運用できないため、 設定を持つ方を採用した。
- **YAML を採用せず別フォーマット (JSON / TOML 等) に統一する**: 外部ツール (markdownlint-cli2 等) が YAML 設定を前提とするケースがあり、YAML を排除できない。フォーマット選択は外部ツール要求に従う。

過去に採用していた決定として以下の経緯がある。

- 当初は `.yml` を許容する根拠として「外部ツールが `.yml` を要求するため」と記述し、その例として GitHub Actions (`.yml`) と markdownlint-cli2 (`.yaml`) を挙げていた。GitHub Actions は実際には `.yml` / `.yaml` の両方を読み込み、片方を要求しているわけではないことを確認したため、当該事実誤認を本 ADR から取り除いた。`.yml` を許容する根拠は「外部ツールが `.yaml` を読まないと判明している場合に限る」一般則として整理し直し、flame 内に該当する具体例が現時点で存在しないことを許容する。GitHub Actions ワークフローの拡張子は [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) で `.yaml` を採用しており、本 ADR の `.yml` 例外には該当しない。

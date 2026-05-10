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

### flame 独自型の schema 規約

flame が SoT として独自に型を定義する YAML ファイル (= 利用側に伝播する flame 規約として固有のフィールド・構造を持つ YAML、 例 `flame.yaml`) を以下の policy で扱う。

- 当該 YAML ファイルには **JSON Schema 仕様** で記述した schema を併設する
- schema 自体のシリアライズ形式は対象ファイルと同言語 (= YAML。 本 ADR の YAML 採用方針と整合する。 schema 内に説明コメントを書ける副次効果もある)
- schema の配置先は flame harness の SoT 規約 ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §schema の機械可読化) に従う
- 対象 YAML ファイル先頭に schema 参照 directive を必須化する (具体 directive 文字列形式は yaml-language-server 仕様に従う)
- 外部ツールが SoT として型を定義する YAML (例: `.golangci.yaml` / GitHub Actions workflow) は本規約の対象外とする (= flame 側からは schema 併設を強制しない。 当該ツール側の schema を IDE で参照してよい)
- CLI が自動生成・更新するため手動編集対象でないファイル (例: `flame.lock`) は本規約の対象外とする (= IDE 補完・即時 lint の主用途と整合しないため、 schema 併設は別途必要性が顕在化した時点で個別判断する)

なお、 同等の規約は JSON 側にも対称的に適用する ([FLM_APP_0003](FLM_APP_0003__json.md) §flame 独自型の schema 規約)。

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
- スキーマ単位での意味検査 (例: JSON Schema / Cue による型検査) のうち、 flame 独自型に対する schema 必須化と検査経路は §flame 独自型の schema 規約 で扱う。 外部ツールが定義する型に対する schema 検査は本 ADR の対象外であり、 必要になった時点で別途 ADR を起こす
- §flame 独自型の schema 規約 の具体実装は以下に従う ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §schema の機械可読化 と整合):
  - schema 配置先 path: `vendor/flame/schemas/<対象 file 名>.schema.yaml`
  - 対象 YAML ファイル先頭の schema 参照: yaml-language-server 仕様に従い `# yaml-language-server: $schema=<schema への相対 path>` directive を付ける
- §flame 独自型の schema 規約 の運用は `flame check yaml` の以下 2 経路の lint 拡張で機械的に強制される (= 規範: 利用側 repo にも flame CLI 経由で同経路が適用される):
  1. **schema directive の存在検査**: flame 独自型 (= `vendor/flame/schemas/<name>.schema.yaml` が併設されている YAML ファイル) に対して、 ファイル先頭 (= scan 対象は冒頭数行) の `# yaml-language-server: $schema=<相対 path>` directive が付いていることを検査する。 directive 不在 / 別 path を指す directive はいずれも fail
  2. **schema validation**: 上記 directive 経由で参照される schema を load し、 当該 YAML ファイルが schema に conform するかを検査する
  対象 YAML ファイルの判定 (= flame 独自型か否か) は対応 schema の存在で行う (= `vendor/flame/schemas/<basename>.schema.yaml` を search-upward で探し、 実在すれば flame 独自型と判定)。 search-upward 経路により仮想 repo root 配下の test fixture でも本検査が成立する
- 上記 lint 経路の flame self における実装手段 (= 利用側 repo は flame CLI バイナリを GitHub Release から install するため当該 library 選定の意思決定に関与しない / 参考情報): `github.com/santhosh-tekuri/jsonschema/v6` (Draft 2020-12 対応 JSON Schema validator) と `gopkg.in/yaml.v3` を `cli/` モジュールに追加。 実装は `cli/internal/check/schemavalidate/` の helper package と `cli/internal/root/check/yaml/` の subcommand の協調で完結する
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
- **flame 独自型の schema を JSON シリアライズ形式 (`.json`) で記述する**: schemastore.org 等の慣習に合致し外部 IDE エコシステムとの互換性が高いが、 flame は YAML 設定を主軸に据えているため schema も YAML で書く方が自プロジェクトの SoT 言語と整合する。 また YAML 化により schema 内に説明コメント (`#`) を書ける副次効果もある。 yaml-language-server / Red Hat YAML 拡張は YAML シリアライズの JSON Schema を解釈できるため IDE 補完・即時 lint の機能差は無く、 YAML 採用とした
- **flame 独自型に schema 併設を必須化しない (任意とする)**: schema を書くコストを発生させない利点はあるが、 flame 独自型は利用側に伝播する規約として機械的に検査可能であるべきで、 任意化は §flame 独自型の schema 規約 の主旨と相容れない。 本 ADR の対象を「flame が SoT として独自定義する型」 に絞ることで対象数を抑え、 その範囲では schema 必須を採用した
- **schema directive の存在検査と schema validation を別 endpoint に分離する**: 検査対象の生成過程は同じ (= ファイル先頭の directive 抽出) であり、 endpoint を分けると `flame check yaml` 1 回で完結しない。 既存 `flame check yaml` の拡張として 1 endpoint に統合する形を採用した

過去に採用していた決定として以下の経緯がある。

- 当初は `.yml` を許容する根拠として「外部ツールが `.yml` を要求するため」と記述し、その例として GitHub Actions (`.yml`) と markdownlint-cli2 (`.yaml`) を挙げていた。GitHub Actions は実際には `.yml` / `.yaml` の両方を読み込み、片方を要求しているわけではないことを確認したため、当該事実誤認を本 ADR から取り除いた。`.yml` を許容する根拠は「外部ツールが `.yaml` を読まないと判明している場合に限る」一般則として整理し直し、flame 内に該当する具体例が現時点で存在しないことを許容する。GitHub Actions ワークフローの拡張子は [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) で `.yaml` を採用しており、本 ADR の `.yml` 例外には該当しない。

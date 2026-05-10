# GitHub Label の運用

## 背景

- flame は GitHub をホスティングに採用しており、 GitHub Actions ワークフローで自動化を行う ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md))
- GitHub は repository 単位で任意の Label 集合を管理し、 PR / issue に複数の label を付けられる
- GitHub CLI (`gh`) は label の作成 (`gh label create`) と PR への label 付与 (`gh pr edit --add-label`) を提供する。 付与時に label が repo に存在しなければ操作は失敗する
- GitHub の検索 API は label 名と日時範囲を組み合わせた merged PR 検索 (`gh pr list --state merged --label <name> --search "merged:>=<datetime>"`) をサポートする
- flame では top-level ディレクトリで `go.mod` を持つものを 1 つの module として扱い、 配布対象アプリケーションは module 内の `cmd/<name>_tool/` に置く ([FLM_APP_0007](../application/FLM_APP_0007__go.md))
- 配布対象アプリケーションのリリースは module ごとに独立に進む ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md))
- flame は変更ファイルのパスから label を derive する自動付与経路を持つ ([FLM_FEA_0002](../feature/FLM_FEA_0002__path_based_label.md))
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame では GitHub Label を以下のルールで扱う。 本 ADR は label 一般の policy のみを扱い、 個別の自動付与方式 (path から derive する等) は別 ADR ([FLM_FEA_0002](../feature/FLM_FEA_0002__path_based_label.md) 等) で扱う。 人間が UI から付ける任意 label の意味付けは本 ADR の対象外とする。

### Label の用途と分類

- Label は PR / issue を機械的に分類する目的で使う
- Label は次の 2 種類に分類する。 本 ADR は「自動付与 label」 のみを規約化する
  - **自動付与 label**: CI / 自動化機構が機械的に付与する label。 命名・付与方式・consumer は本 ADR / 別 ADR で規定する
  - **人間任意 label**: 人間が UI から任意目的で付ける label。 命名・運用は本 ADR の対象外

### 命名 namespace

- 自動付与 label は `<category>/<value>` 形式に固定する
- `<category>` は label の意味カテゴリを表す。 当該 category を消費する側 ADR がカテゴリを定義する
- 同一 repo 内で `<category>` の名前空間は重複させない (1 つの category 名を 1 つの自動付与 ADR が排他で持つ)

### 命名カテゴリ

- 現時点では category として `module` を採用する。 `module/<name>` の `<name>` は flame リポジトリ内で `go.mod` を持つ top-level ディレクトリ名 ([FLM_APP_0007](../application/FLM_APP_0007__go.md)) と一致させる
- 新規 category を導入する場合は本 ADR に決定として追加する

### 不在時の自動作成

- 自動付与の workflow は label を付与する直前に、 当該 label が repo に存在しなければ自動生成する
- label の色・description は固定値を持たせる

### Label の利用先

- 自動付与 label は機械的に消費されることを前提とする。 個別 consumer (release ノート生成等) の規約は当該 consumer ADR ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) 等) で扱う

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。 label 自動付与の実装は GitHub Actions ワークフロー ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md)) と shell スクリプト ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) で行うため、 種別固有の整備は持たない。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (実装は GitHub Actions skill ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md)) と shell スクリプト skill ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) に委譲) |
| lint | 省略 (実装側の YAML / shell の各 lint に委譲) |
| build | 省略 (静的設定のため出力生成の概念がない) |
| test | 省略 (実 label 付与は PR 上でのみ起こるため、 ローカル test は持たない) |
| ADR ルール検査 skill | 省略 (実装側の lint に委譲) |

## 影響

- repo の label 集合は自動付与 workflow 経由で増えていく。 同 repo 内で `<category>/` prefix を 2 つの自動付与 ADR が共有しない暗黙制約が運用上発生する
- 人間任意 label は本 ADR の規約外であるため、 label 一覧には自動付与 label と人間任意 label が混在する。 区別が必要な場面では命名 prefix (`module/...` 等) で識別する
- `<category>` の名前空間を ADR で固定するため、 新たな自動付与経路を入れたいときには本 ADR 改訂か新規 ADR 起草を経由する必要がある
- 自動付与 label の consumer (release ノート生成等) は label の存在を前提に走るため、 自動付与経路が壊れた / 抑止された PR は consumer から取りこぼされる
- 自動作成方針により、 module を新設すると新 label が repo に増える。 release ノートには新 label による絞り込みが反映される
- 個別の自動付与方式 (path-based 等) は本 ADR では扱わないため、 「いつ・どんな条件で label を付けるか」 を知るには consumer / 自動付与 ADR ([FLM_FEA_0002](../feature/FLM_FEA_0002__path_based_label.md) 等) を参照する必要がある

## 評価

代替案として以下を検討した。

- **label 命名を `area/<name>` 等の慣習名にする**: GitHub の OSS で広く使われる prefix だが、 flame の `module` 概念 ([FLM_APP_0007](../application/FLM_APP_0007__go.md)) に直接対応させる方が consumer (release ノート生成、 [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md)) との結合が読みやすい。 `module/<name>` を採用した。
- **label を repo 管理者が事前に手動作成する**: workflow に label 自動作成権限を持たせずに済むが、 module を新設するたびに label を手作成するメンテナンスコストが発生する。 自動作成を採用した。
- **label 一般 ADR と path-based 自動付与 ADR を 1 本に統合する**: 1 ADR で完結する明快さがある一方、 (1) 「label をどう運用するか (general policy)」 と「path-based でどう自動付与するか (実装方式)」 は粒度が異なり、 将来 path-based 以外の自動付与方式 (commit author / branch 名 / PR タイトル等から derive) を追加する場合に統合 ADR では扱いきれなくなる、 (2) consumer (release ノート等) は「label を消費する」 という抽象だけ知っていれば良く、 自動付与方式の詳細を知る必要は無い。 label 一般を本 ADR、 path-based 自動付与を [FLM_FEA_0002](../feature/FLM_FEA_0002__path_based_label.md) として分離した。
- **`<category>` を ADR で固定せず自動付与 ADR ごとに自由命名する**: 自動付与 ADR ごとに category 命名を判断できる柔軟性が得られるが、 同一 repo の label 集合を見たときに category 命名規約が散乱する懸念がある。 本 ADR で `<category>/<value>` 形式に固定し、 category そのものは個別 ADR で定義する形を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初は本 ADR に「PR の opened / synchronize / reopened を契機に変更ファイルパスから影響 module を機械的に列挙して該当 label を付与する」 等の path-based 自動付与の具体運用を含めていた。 path-based は label 自動付与方式の 1 つに過ぎず、 「label をどう運用するか (一般)」 と「path から label を derive する具体方式」 を分けた方が将来別方式の追加を吸収しやすいため、 path-based 部分を [FLM_FEA_0002](../feature/FLM_FEA_0002__path_based_label.md) に切り出した。 本 ADR には label 一般の policy (用途分類・命名 namespace・category 規約・自動作成・consumer との結合形) のみを残した。

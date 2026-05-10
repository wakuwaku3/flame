# flame の ADR PREFIX と internal / downstream の対応

## 背景

- flame の ADR 規約は [FLM_GEN_0001](../../../vendor/flame/docs/adr/general/FLM_GEN_0001__adr.md) §ファイル名 / §採番 / §依存側プロジェクトへの要請 で「同一プロジェクト内で internal / downstream を区別する場合は別 PREFIX を採用し各空間で 1 から採番する」 一般規約を定義している
- flame の資産分類軸 (internal / downstream) は [FLM_GEN_0007](../../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) で定義されている
- 上記一般規約は flame 以外の依存側プロジェクト (= flame harness を install する側) でも適用される downstream ADR である。 一方、 flame self が採用する具体 PREFIX (= internal / downstream に何の文字列を割り当てるか) は flame 固有の運用判断であり、 flame-internal な ADR で記録するのが整合的

## 決定

flame では ADR PREFIX を以下の対応で運用する。

### PREFIX の対応

| 分類 | PREFIX | 配置 |
| --- | --- | --- |
| downstream | `FLM` | `vendor/flame/docs/adr/<category>/` |
| flame-internal | `FLI` | flame self の `docs/adr/<category>/` |

### 採番

各 PREFIX × カテゴリで 1 から欠番なく採番する ([FLM_GEN_0001](../../../vendor/flame/docs/adr/general/FLM_GEN_0001__adr.md) §採番)。

## 影響

- flame の ADR 内で `FLI_*` / `FLM_*` の用法は本 ADR で固定される。 PREFIX の追加・変更が必要になった場合は本 ADR を改訂する
- 本 ADR は flame-internal なので利用側 repo には配布されない。 利用側 repo は [FLM_GEN_0001](../../../vendor/flame/docs/adr/general/FLM_GEN_0001__adr.md) §依存側プロジェクトへの要請 に従い、 自プロジェクトの PREFIX 規約を別途決定する (= 利用側で internal / downstream の区別を持つ場合は別 PREFIX を採用、 flame の `FLM` / `FLI` と重複しないこと)

## 評価

代替案として以下を検討した。

- **`FLI` / `FLM` を [FLM_GEN_0001](../../../vendor/flame/docs/adr/general/FLM_GEN_0001__adr.md) (downstream ADR) に直接記載する**: PREFIX 規約と具体採用が 1 ADR で完結する利点があるが、 (1) 「具体 PREFIX 採用」 は flame 固有の運用判断で、 ADR が他 repo (= flame harness を install する側) から参照された場合に flame の固有名詞 (`FLI` / `FLM`) が当該 repo の PREFIX 規約と無関係に伝播する、 (2) [FLM_GEN_0007](../../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) の「flame-internal な決定は flame self の `docs/adr/` に置く」 規約と整合しない、 という不利益がある。 一般規約 ([FLM_GEN_0001](../../../vendor/flame/docs/adr/general/FLM_GEN_0001__adr.md)) と具体採用 (本 ADR) を分離する方を採用した
- **PREFIX 採用を `FIN` (= flame internal) / `FOU` (= flame outer) 等の別文字列にする**: 候補として検討したが、 既存 `FLM` (= flame の略) の慣習が確立しており、 internal も `FLI` (= 同じ 3 文字 prefix で I を internal に充てる) が短くて読みやすい。 既存慣習の継承を優先した

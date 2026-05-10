# flame の資産を internal と downstream に分類し vendor/flame/ 配下を downstream の SoT とする

## 背景

- flame は AI 開発における品質保証 harness を提供する基本思想を持つ ([FLM_GEN_0002](FLM_GEN_0002__flame.md))
- flame harness は 3 配布チャネル (Claude Code plugin / reusable workflow / vendor) で配布される ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md))
- flame は補助処理を集約する flame CLI を持ち、 GitHub Release 経由で配布される (具体 ADR は flame self の internal ADR 群を参照)
- flame の各種資産には性質の異なる 2 種類が混在する:
  - flame 自身の source 開発のみに必要な資産 (例: flame CLI の source、 GitHub Release 経路の規約、 内部実装に閉じる ADR)
  - flame harness を install した repository の開発にも適用される共有資産 (例: 各種ファイル形式の規約、 Claude Code 拡張、 lint 設定、 配布規約自体)
- 前者は flame 内部に閉じ、 後者は harness 配布対象として利用側で参照される
- ADR の依存側プロジェクト伝播性は ADR 起草時点で各 ADR の §影響 で「依存側プロジェクトに伝播する」 と表明する形、 および ADR カテゴリ (`SPC` 等) で表明する形で部分的に運用されてきたが、 ADR 以外の resource (rule / skill / workflow / config / source code 等) には伝播性を表明する明示的な機構が無い
- vendor は SoT として配布される性質上、 利用側で編集すると drift が発生する ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §integrity check)
- 利用側にとって vendor が readonly であるという規範ルールは現時点で明示されておらず、 機構的な検査 (sha256) のみで運用されている
- `flame.yaml` 自身は vendor 化対象外のため、 利用側で `flame.yaml.files[].sha256.vendor` を改ざんすれば機構的な検査を素通りできる運用穴がある
- Claude Code は plugin 機構として `.claude-plugin/` 配下を、 GitHub Actions は reusable workflow として `.github/workflows/` 配下を、 それぞれ repo root に置くことを仕様で固定している ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル A / §チャネル B)

## 決定

flame の資産を **internal** と **downstream** の 2 種類に分類し、 各分類ごとに **物理レイアウト** と **編集権限** を規約化する。

### 分類定義

| 分類 | 説明 |
| --- | --- |
| internal | flame 自身の source 開発のみに必要な資産。 利用側 repository では参照されない |
| downstream | flame harness を install した repository の開発に適用される共有資産。 利用側 repository でも参照される |

### 物理レイアウトによる判定

各 resource は **物理配置** で internal / downstream を機械的に判定可能とする。

- **downstream**: `vendor/flame/` 配下を SoT とする
  - 例外: 配布機構が plugin / reusable workflow である資産は [FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル A / B の規定により別 path を SoT とする (`.claude-plugin/` / `.github/workflows/wf__*.yaml`)
- **internal**: flame repository 内の `vendor/flame/` 配下「以外」 の path に置く

### 主要資産種別の典型分類

各資産種別の典型的な分類は以下。 個別判断が必要な resource は分類意識を持って配置先を決める。

| 資産種別 | 典型分類 | downstream の SoT 配置 / internal の SoT 配置 |
| --- | --- | --- |
| ADR | 個別判断 | downstream: `vendor/flame/docs/adr/<category>/` / internal: flame self の `docs/adr/<category>/` |
| rule (`.claude/rules/`) | downstream | `vendor/flame/.claude/rules/` |
| skill / agent / hooks | downstream | `.claude-plugin/` ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル A) |
| `CLAUDE.md` | downstream | `vendor/flame/CLAUDE.md` |
| 実体層 workflow (`wf__*.yaml`) | downstream | `.github/workflows/` ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル B) |
| トリガー層 workflow (`trg__*.yaml`) | downstream | `vendor/flame/.github/workflows/` |
| lint config / `devbox.json` / `.envrc` / `.vscode/` | downstream | `vendor/flame/` 配下対応 path |
| flame CLI / lib source code (`cli/` / `lib/`) | internal | flame repo の `cli/` / `lib/` |
| flame self の運用ドキュメント (例: `docs/notes/`) | internal | flame repo の対応 path |
| flame self の `README.md` | internal | flame repo root |

### ADR の振り分け基準

ADR は資産種別の中でも個別判断の比率が高いため、 起草時に **internal / downstream の判定を必ず行う** ものとする。 判定軸:

- 当該決定が **flame harness を install した repository の開発にも適用される** か
  - yes → downstream (= `vendor/flame/docs/adr/<category>/` に配置)
  - no → internal (= `docs/adr/<category>/` に配置)

ADR のカテゴリ (GEN / APP / ENG / FEA / INF) と本分類は **直交軸** として独立に決まる ([FLM_GEN_0001](FLM_GEN_0001__adr.md) §カテゴリ)。

具体 ADR の振り分け list は当該リポジトリの internal ADR (= 当該 repo の `docs/adr/` 配下) で別途管理する (flame self は `FLI_GEN_0001`)。 一般原則は本 ADR §決定 §物理レイアウトによる判定 が定める通り、 当該 ADR の物理配置で判別する。

### vendor/flame 配下の編集権限

`vendor/flame/` 配下のファイルは **利用側 repository では readonly** とする。 直接編集は drift として整合性検査 (`flame verify`) で fail させる。 利用側拡張は副ファイル overlay ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §副ファイル overlay 機構) 経由のみ許容する。

例外として、 当該 repository が **harness の source 提供元** である場合のみ writable とする。 source 提供元では `vendor/flame/` 配下が SoT そのものであり、 編集 → 利用側へ伝播のサイクルを担う。

#### source 提供元の判定

- `flame.yaml.harness.source` (例: `github.com/wakuwaku3/flame`) と当該 repository の identity が一致するかで判定する
- 当該 repository の identity は git remote URL (`origin` の URL) から `<owner>/<repo>` 部分を抽出して比較する
- 環境変数 `FLAME_HARNESS_SOURCE_OVERRIDE` で識別 override 可能 (CI / 動作検証等で source 提供元の挙動を再現する用途)

source 提供元判定は git remote URL ベースで独立に決まり、 `flame.yaml` の改ざん (= `sha256.vendor` の書き換え等) では取得できない。

### repo root における downstream resource の取り込み形式

以下の resource は flame self / 利用側 repository ともに **repo root 側で vendor SoT を取り込み + 必要なら repo 独自の拡張ルールを後ろに追加する形** を取る。 vendor の install copy は行わない。

| Resource | 取り込み機構 | repo root での内容 |
| --- | --- | --- |
| `CLAUDE.md` | 自然言語の指示 (Claude Code は file include を解釈しないため、 AI が link を辿る規約) | (a) `vendor/flame/CLAUDE.md` を必ず読むよう AI に指示する stub + (b) repo 独自の拡張ルール |
| `.envrc` | direnv の `source_env_if_exists` (= 別 .envrc を実行する機構) | (a) `source_env_if_exists vendor/flame/.envrc` で vendor の env 設定を取り込み + (b) repo 独自の env 拡張 |

取り込み snippet (= install 先 file 内に出現する vendor 取り込みフレーズ) は `flame.lock` の `embeds[]` に記録する ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §flame.lock)。 entry は `install` (= install 先 path) / `target` (= 取り込み元 vendor path) / `snippet` (= 取り込みフレーズ本体) を持ち、 vendor 側のパスが変更された際に CLI が flame.lock を読んで install 先 file 内の snippet を replace する経路を提供する。 例:

```yaml
embeds:
  - install: .envrc
    target: vendor/flame/.envrc
    snippet: source_env_if_exists vendor/flame/.envrc
  - install: CLAUDE.md
    target: vendor/flame/CLAUDE.md
    snippet: |
      [vendor/flame/CLAUDE.md](vendor/flame/CLAUDE.md)
```

利用側 repository の `flame install` 時には、 flame.lock の embeds に基づいて install 先 file (root の `.envrc` / `CLAUDE.md` 等) の snippet を生成 / 検証する。

本セクションの resource は `flame.yaml` の `files[]` 管理対象 (= install copy 対象) から **除外** する。 利用側 repository では `flame install` 時にこれらを copy せず、 利用者は repo root の対応 file を独自に書く (= vendor 取り込み + 拡張)。

flame self における repo 独自拡張ルールの典型例: 「downstream 配布対象の設定変更は `vendor/flame/` 配下を修正する」 が拡張ルール (= source 提供元としての責務)。

取り込み機構を持つ他の tool config (= `include` / `extends` / `source` 等の機構を持つ resource) も同様の取り込み形式を採用する。 現時点で本形式を採るのは以下:

- `CLAUDE.md` (Claude Code が repo root の `CLAUDE.md` を無条件注入する仕様を利用し、 自然言語の link 経由で vendor の `CLAUDE.md` を辿らせる)
- `.envrc` (direnv の `source_env_if_exists` 機構で vendor の `.envrc` を source する)
- `.yamllint` (yamllint の `extends:` 機構で vendor の `.yamllint` を継承する)

それ以外の取り込み機構を持つ resource を取り込み形式に追加する場合は本 ADR の本一覧を改訂する (採用判断は ADR 改訂で残す)。

`.markdownlint-cli2.yaml` は markdownlint-cli2 の `extends:` プロパティが `.markdownlint-cli2.yaml` の top-level では公式に未サポートであり (`config:` block 内に置いた場合のみ markdownlint config 形式の継承として動作する) 、 vendor の `.markdownlint-cli2.yaml` を継承する経路を取れない。 このため本取り込み形式の対象から **除外** し、 install copy 形式 (= `flame.lock.files[]` の `merge: deep`) で配置する ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §副ファイル overlay 機構)。

### flame self の install 先における downstream resource の stub

以下の resource は flame self に限り install copy せず vendor SoT を直接参照する経路を取る。 利用側 repository では通常通り install copy される。

| Resource | flame self での扱い |
| --- | --- |
| ADR (downstream) | flame self の `docs/adr/` には **置かない**。 vendor SoT (`vendor/flame/docs/adr/`) を直接参照する |
| rule (downstream) | flame self の `.claude/rules/` には **vendor 側 rule を参照する stub** を置く。 stub の本文は `vendor/flame/.claude/rules/<vendor-rule-name>.md` への link、 paths frontmatter は vendor 側 rule と同じ範囲を保持 |
| その他 (lint config / devbox / `.vscode/` / トリガー層 wf 等で取り込み機構を持たないもの) | install copy を行う。 これらは utility tool が repo root 直下を直接読む仕様で、 stub では機能しないため |

flame self の `.claude/rules/` における命名規約:

- **vendor 参照 stub**: `flame-<vendor-rule-name>.md` (例: `flame-harness.md` = `vendor/flame/.claude/rules/harness.md` を参照)
- **internal rule** (= internal な ADR を参照する rule): `flame-` prefix を **付けない** (vendor 側 rule との命名衝突を避ける)

`flame.yaml` の管理対象 (`files[]`) は **install copy が発生する resource のみ** とする。 stub / 直接参照経路 (= ADR / rule、 および §repo root における downstream resource の取り込み形式 で扱う `CLAUDE.md` / `.envrc`) は flame.yaml 管理対象外で entry を持たない。

### CLAUDE.md への記載責務

本 ADR で定める分類規約および vendor/flame 編集権限は flame の最重要ルールとして CLAUDE.md に短文直書きする。 ADR 改訂時には CLAUDE.md 短文も同期更新する責務を負う ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md) §Root の `CLAUDE.md`)。 ただし flame self の `CLAUDE.md` は本セクション §repo root における downstream resource の取り込み形式 に従って vendor 取り込み stub となるため、 短文直書きの実体は vendor の `CLAUDE.md` (= 本 ADR が SoT として配置されたもの) で行う。

## 影響

- flame の全資産 (ADR / rule / skill / workflow / config / source code 等) に対して internal / downstream の分類意識を持つ必要がある
- 新規資産追加時には起草時点で配置先を判定する。 downstream なら `vendor/flame/` 配下 (または [FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル A / B の SoT)、 internal なら flame repo の vendor 配下「以外」
- 利用側 repository で `vendor/flame/` 配下を直接編集すると整合性検査で fail する。 利用側拡張は副ファイル overlay 経路のみ
- source 提供元 repository では vendor/flame が writable のため、 開発者は SoT として直接編集する
- `flame verify` は当該 repo が source 提供元か否かを判定し、 vendor/flame 編集の許可 / 禁止を分岐する実装が必要 (本 ADR 採択時点では未実装、 follow-up タスクとして CLI 側で対応)
- FEA カテゴリには internal / downstream の ADR が混在しうるため、 本 ADR 採択後に internal な ADR の物理配置 (当該 repo の `docs/adr/` 配下、 flame self であれば PREFIX `FLI_*`) と downstream な ADR の vendor SoT 化 (= 当該 repo の `docs/adr/` から削除) を区別する
- flame self の `docs/adr/` には internal な ADR の SoT のみが存在し、 downstream ADR は vendor SoT を直接参照する。 flame self の `.claude/rules/` には vendor 参照 stub と internal rule のみが存在する。 flame self の `CLAUDE.md` は vendor の `CLAUDE.md` を読むよう AI に指示する自然言語 stub
- `flame.yaml` の `files[]` 管理対象は install copy が発生する resource (lint config / devbox / `.envrc` / `.vscode/` / トリガー層 workflow 等) のみに縮む。 ADR / rule / CLAUDE.md は管理対象外
- CLAUDE.md は最重要ルールの短文直書きで構成され、 本 ADR 短文も含めた最重要ルール群が利用側にも配布される
- [FLM_GEN_0001](FLM_GEN_0001__adr.md) §決定 §カテゴリ から SPC カテゴリが削除される (本 ADR で扱う物理レイアウト + 編集権限規約が SPC の役割を吸収するため)。 また FLM_GEN_0001 §決定 §依存側プロジェクトへの要請 から SPC への参照も削除される
- ADR 起草時に「カテゴリ選定 (GEN / APP / ENG / FEA / INF)」 と「internal / downstream 判定」 の 2 つの直交軸を意識する必要がある
- 本 ADR の規約は依存側プロジェクトへも伝播する (本 ADR は downstream のため)
- repo root の `CLAUDE.md` / `.envrc` 等の取り込み snippet は flame.lock の `embeds[]` で管理される。 vendor 側 path 変更時の追従は CLI (`flame install`) が embed snippet を replace する経路で機能する

## 評価

代替案として以下を検討した。

- **[FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) を拡張して論理分類 + 編集権限も併記する**: 配布規約 (= 物理経路) と論理分類 + 編集権限を 1 ADR に集約する案。 抽象度が混在し ADR が肥大化する。 物理経路 = FLM_FEA_0003、 論理分類 + 権限 = 本 ADR と分離する方を採用した
- **分類対象を ADR のみに絞る**: ADR の振り分けだけを規約化する案。 flame の他の資産 (rule / skill / workflow / config / source code 等) も同じ分類軸の話だが、 各個別 ADR に分散して扱うことになり整合性を保ちにくい。 全資産を統一的に扱う方を採用した
- **ADR 単位の internal / downstream を `SPC` カテゴリで表明する**: [FLM_GEN_0001](FLM_GEN_0001__adr.md) §決定 §カテゴリ にあった SPC カテゴリ (= 「当該リポジトリ内に閉じる、 依存側プロジェクトへの伝播を意図しない決定」) で ADR 単位の internal / downstream を表明する案。 ADR 単位では機能するが、 ADR 以外の resource (rule / skill / workflow / config / source code 等) には適用できない。 全資産共通の判定軸として物理レイアウトで分離する方を採用し、 SPC カテゴリは削除した
- **frontmatter / メタデータで `audience: internal | downstream` を管理する**: 物理レイアウトを変えずメタデータで分類する案。 flame install / verify が resource ごとに audience を読み取り配布対象を分岐する実装が必要。 物理レイアウトで分離する方が「どの資産がどの分類か」 を一目で判別できるため、 物理分離を採用した。 メタデータ案は組み合わせ可能だが本 ADR では物理レイアウトを正とし、 メタデータは設けない
- **vendor/flame の編集権限を「常に readonly」 とする (例外なし)**: flame self も含め全 repo で vendor/flame が readonly。 SoT を別 path (例: `source/`) に置く案。 SoT と install path の分離が物理レイアウトでさらに明確になる利点があるが、 dogfooding の install 経路と SoT 編集経路を二重化する必要があり flame の運用コストが増える。 source 提供元のみ writable とする例外条項で対称性を保つ方を採用した
- **source 提供元の判定基準として GitHub repo ID (numeric) を使う**: git remote URL の文字列比較は owner/repo の rename で破綻するが、 GitHub repo ID (数値) は不変。 GitHub API access が必要となり offline 環境で判定不能になる。 git remote URL を一次経路、 環境変数 override を補助経路とする方を採用した
- **vendor/flame 編集権限を internal な ADR として書く**: flame 自身でしか編集しないなら internal で十分という案。 利用側でも「自身が source 提供元か否か」 を判定する責務はあるため (常に readonly になるだけでも、 ADR の規範ルールは知る必要がある)、 downstream に置くのが筋。 利用側にとっては「常に readonly」 として効くが、 ADR 上は両主体に対する対称的な規範ルールとして表現する
- **`flame.yaml` を vendor 化する**: SoT に置けば編集権限規約も vendor 経由で適用できる利点があるが、 `flame.yaml` は install 状態のリポジトリ固有記録 ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §評価) であり SoT 側に持つべき情報ではない。 vendor 化非対象を維持し、 source 提供元判定を git remote URL ベースで独立に決める方を採用した

過去に採用していた決定として以下の経緯がある。

- 当初は flame の各資産が「internal で参照されるか」 「downstream で配布されるか」 を区別する明示的な分類基準は無く、 各 ADR の §影響 で「依存側プロジェクトに伝播する」 と書く形、 および ADR の SPC カテゴリ ([FLM_GEN_0001](FLM_GEN_0001__adr.md) 当時) で内部閉じ ADR を表明する形で間接的に運用していた。 ADR 数が増え、 vendor 化 ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md)) 導入後は「全 ADR が利用側に配布される」 構造が成立したが、 利用側で参照されない internal な ADR も配布対象に含まれていた。 加えて ADR 以外の resource にはそもそも分類軸が無かった。 全資産共通の判定軸として物理レイアウトで分離する規約を新設し、 ADR 単位の専用機構だった SPC カテゴリは削除した
- 当初は vendor/flame の編集権限について明示的な規約は無く、 flame self は vendor を編集する慣習で運用していた。 利用側にとって vendor が readonly であることは integrity check (sha256) で機構的に強制されるが、 規範ルールとして表明されていなかった。 利用側にとっての行動規範を明文化し、 source 提供元のみ writable とする例外条項で対称構造を保つ形に改訂した
- 当初は flame self でも dogfooding install で downstream resource (ADR / rule / CLAUDE.md) を install path にコピーする運用としていた。 これにより flame self の `docs/adr/` には internal + downstream が混在し SoT 側を見ないと分類が判別できず、 `flame.yaml` の `files[]` 管理対象も大量 (= 全 ADR / rule / CLAUDE.md を含む) になっていた。 flame self では install copy を行わず vendor SoT を直接参照する形 (= rule は vendor 参照 stub、 CLAUDE.md は vendor を読むよう AI に指示する stub、 ADR は internal のみ flame self に配置) に改訂し、 flame self の `docs/adr/` を internal SoT 専用に整理した。 `flame.yaml` の管理対象も install copy が発生する resource のみに縮小した
- 当初は flame self の `.claude/rules/` に既存命名 (例: `flame-philosophy.md` = FLM_GEN_0002 を指す) を持つ rule が存在し、 命名規約として `flame-` prefix が「flame の概念を扱う rule」 を意味していた。 §flame self の install 先における downstream resource の stub で `flame-` prefix が「vendor 参照 stub」 の意味を持つようになり、 既存命名と衝突した。 既存 `flame-philosophy.md` 等は意味的整合のため整理 (= `flame-` prefix の意味を「vendor 参照 stub」 に統一) する必要が生じる
- 当初は repo root の取り込み形式 resource (CLAUDE.md / .envrc) について、 取り込み snippet の vendor path を install 先 file に固定文字列として書き、 vendor 側 path 変更時には install 先 file を手動で書き換える運用としていた。 vendor 側構造変更が発生した際に install 先 file の snippet 追従が漏れる事故を構造的に防ぐため、 取り込み snippet を `flame.lock` の `embeds[]` に記録し、 CLI が install 時に install 先 file 内 snippet を replace する経路を新設した

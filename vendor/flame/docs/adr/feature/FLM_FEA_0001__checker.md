# checker — 静的検査の単位と hook / CI の共通実装

## 背景

- flame の品質保証は (1) AI ターン内 hook、(2) CI、(3) 監視の 3 層で構成し、hook で実装可能な検査は CI 側でも重複実行する ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- flame は静的にチェックできるルールを静的チェックで担保する ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- flame ではコンテンツ種別ごとに「lint」を整備する ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- 1 つのファイルが複数の種別に該当しうる (例: ADR は Markdown でもある) ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- 静的検査の対象範囲は種別によって異なる: 1 ファイル単位 (syntax / style など)、種別該当ファイル群を集合として渡す単位 (集合内の整合性、連番の dense 性、相互参照など)、リポジトリ全体の状態を見る単位 (project-wide の参照解決など)
- コンテンツ種別によっては、 1 つの module / リポジトリ内に意味的に異なる粒度の検査単位が併存する。 例えば Go module は次のように複数の operation が異なる粒度を取る:
  - lint (gofmt / go vet) は package 単位で実行できる
  - build は main package 単位で実行する。 1 module に main package が複数あれば build は複数回必要
  - test は package 単位、 さらに重い service-level test 等は chunk (build tag、 test 名集合、 shard 等) 単位に分割しないと CI 時間が膨らむ
- 同じ種別の同じ module 内でも operation ごとに対象 (target) の粒度が異なるため、 「種別 → 1 つの module-unit target」の単純な対応では parallelism と意味的整合の両方を捉えられない
- 検査の意味的影響範囲は operation ごとに異なる。 ファイル変更がそれ単体に閉じる operation (gofmt のような副作用なし整形検査) と、 module 内の他ファイル / 他 package へ依存解決を介して影響が波及する operation (build / test 等の compile / link を伴う検査) では、 「変更を契機に再起動すべき検査範囲 (trigger blast radius)」が異なる。 build / test では module 内のどこか 1 ファイルが変わっただけでも依存先が再 compile される可能性があり、 安全側に倒すと module 全体が再検査範囲になる
- このため、 検査の単位として「変更ファイル → 1 ファイル → 1 target」の素朴な 1:1 対応が成り立たず、 「変更を契機 (trigger) に enumerate される target 集合」を classifier が解決する必要がある
- 検査ツールには違反を機械的に修正できるもの (gofmt -w、 markdownlint --fix、 go mod tidy 等) と修正手段を持たないもの (型検査、 build、 test 等) が混在する。 hook 層では auto-fix できる違反は機械に解かせる方が AI / 人間のターン内修正の手数を減らせる一方、 CI 層は読み取り専用環境で auto-fix を適用してもリポジトリへ commit されないため意味がなく、 違反として fail させる方が適切
- AI ターン内 hook の hook 層 (Claude Code の Stop hook) は AI ターン終端で同期的に発火し、当該ターンで変更されたファイルに対して検査を起動する ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md), [FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- CI 層 (GitHub Actions) は PR 起点で発火し、変更ファイルに対して検査を起動する ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- hook 層は単一プロセス内で実行する制約を持つが、プロセス内のジョブ並列 (background job + セマフォ) は可能であり、CI 層は GitHub Actions の matrix strategy で並列実行できる ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- hook 層と CI 層の両方で「変更ファイル一覧 → 種別ごとに分割 → 種別ごとの静的検査 → 結果集約」という処理経路が必要となる
- flame の静的検査の実行環境は devbox に固定されている ([FLM_ENG_0002](../engineering/FLM_ENG_0002__devbox.md))

## 決定

flame の静的検査は **checker** を単位として組み立てる。

### trigger と target の分離

各 checker は次の 2 つの粒度を独立に持つ。

- **trigger (起動契機の blast radius)**: どの範囲の変更を契機に当該 checker を再起動するかを示す論理的な単位。 file-local (変更されたファイル自身に閉じる) / directory / package / module / repo-wide のいずれかを取りうる。 検査が依存解決を介して影響を波及させる operation (build / test 等) では、 安全側に倒すため広い trigger を取る
- **target (1 起動が処理する execution unit)**: 1 回の checker 起動が受け取る単位。 file / directory / package / module / chunk のいずれかを取りうる。 trigger より細かい粒度を選ぶことで matrix での並列化粒度を細かく取れる

両者の関係:

- file-local な検査 (Markdown lint 等): trigger = file、 target = file (1:1 で同一)
- 依存追跡を要する検査 (Go の build / test 等): trigger = module、 target = package (1:N、 1 ファイル変更 → 当該 module 内の関連 target を全 enumerate)

### checker の定義

checker は flame における静的検査の実装単位である。

- 入力: ある 1 つのコンテンツ種別 ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md)) に該当する **検査対象 (target / check unit) の配列**。 target の粒度は当該 checker が決め、 種別固有の自然な粒度を取る。 取りうる粒度は固定列挙ではなく、 例として次のような粒度が考えうる:
  - **file**: 1 ファイル単位で検査の正当性が成立する種別が該当する (Markdown、 JSON、 YAML、 Shell など)
  - **directory / package**: ディレクトリ配下のファイル群を 1 単位として評価する種別 (Go の package 単位 lint / build / test、 言語パッケージ単位の検査など)
  - **module / repo-wide**: モジュール全体・リポジトリ全体を解決単位とする検査
  - **chunk**: 任意の集合 (test 名・build tag・shard 等で分割した実行単位など、 並列化のために細分化された単位)
- 出力: 違反の有無を exit code で返し (0 = 違反なし、非 0 = 違反あり)、違反内容を標準出力 / 標準エラー出力に整形して書き出す
- 内部の検査範囲: 当該種別が必要とする検査範囲 (1 target 単位 / 種別該当 target 集合単位 / project-wide) を組み合わせて 1 checker 内に閉じ込める。1 checker の中で複数の検査範囲を併存させてよい
- コンテンツ種別と checker は **1:N 対応** を許容する。 1 種別の中で operation 別に checker を分けてよい (例: Go の lint / build / test を別 checker にして CI matrix で並列化)。 operation が 1 つしかない種別では degenerate に 1:1 対応となる
- 1 ファイルが複数の種別に該当する場合、当該ファイルは該当する各種別の checker の入力に target として重複して含まれる
- 1 種別の中で 1 ファイル変更が複数 (checker, target) ペアに展開される場合がある (例: 1 つの Go ファイル変更 → (lint checker, package), (build checker, main package), (test checker, test chunk))。 同一 (checker, target) ペアは重複排除する

### 検査経路の構成

静的検査の経路は以下の 4 段で構成する。

1. **変更ファイル抽出**: 検査対象となる変更ファイル一覧を計算する
2. **種別判定 + target 解決 (classifier)**: 変更ファイルから (a) 関連する種別を判定し、 (b) 当該種別の各 checker について trigger blast radius を確定し、 (c) blast radius 内を scan して当該 checker の target 粒度に enumerate し、 (d) 重複を排除して、 **(checker, target 配列) のマッピング** を得る。 trigger = target の checker (file-local) では (b)〜(c) は識字関数となり変更ファイルをそのまま target に流す。 trigger > target の checker (Go 等) では (c) で blast radius (例えば影響 module) 内を scan して関連 target (例えば全 package) を enumerate する
3. **dispatch**: 各 (checker, target 配列) を対応する checker に振り分けて起動する
4. **集約**: 全 checker の結果を集約し、いずれかが違反を返したら全体を fail とする

### hook 層 / CI 層の共通化境界

hook 層と CI 層は以下を共有する単一情報源とする。

- 種別判定 (classifier) の実装
- 各種別の checker の実装
- 検査の実行環境 ([FLM_ENG_0002](../engineering/FLM_ENG_0002__devbox.md))

hook 層と CI 層で異なってよいのは以下に限る。

- 変更ファイル抽出方法 (hook 層と CI 層では検査対象集合の意味が異なる)
- dispatch 機構 (hook 層はプロセス内セマフォ並列、CI 層は GitHub Actions matrix 並列)
- 結果集約・通知方法 (hook 層は AI ターン内に違反を返す、CI 層は CI 機構の job 結果として外部に顕在化させる)

### checker の独立性

- 1 checker の失敗が他 checker の実行を止めない (hook 層・CI 層ともに全 checker を走らせ切る)
- checker 同士は実行順依存を持たない
- checker は他 checker の存在を前提とせず、当該種別の検査単独で完結する

### 起動モード (fix / diagnose)

各 checker は次の 2 つの起動モードを受ける。

- **fix モード**: auto-fix 可能な違反は適用したうえで、 残った違反 (auto-fix で解消できないもの) を diagnose し、 残違反があれば非 0 で終了する。 auto-fix の手段を持たない検査 (型検査・ build・ test 等) では diagnose のみを行う
- **diagnose モード**: ファイルを変更せず違反を読み取り専用で報告する。 違反があれば非 0 で終了する

routing は次の通り:

- **hook 層** (Stop hook 経由の checker 起動): **fix モード** で起動する。 機械的に解ける違反 (format / module manifest tidy 等) を機械に任せ、 AI / 人間のターン内修正サイクルを意味的修正に集中させる
- **CI 層** (CI ワークフロー経由の checker 起動): **diagnose モード** で起動する。 CI workspace は読み取り専用前提でリポジトリに書き戻さないため fix を適用しても commit に乗らず無意味、 違反は fail として開発者に明示する

mechanism:

- 起動側は環境変数経由で各 checker に mode を伝える
- default は diagnose (誤 invocation で意図せずファイルを書き換えないための安全側)
- hook 層スクリプトは checker 起動前に fix を明示的に伝搬する
- CI 層スクリプトは default のまま (もしくは diagnose を明示) で起動する
- auto-fix capability を持たない checker は mode を参照せず diagnose 動作のみを行ってよい (両 mode で同じ動作)

## 影響

- 種別判定 (classifier) は hook 層と CI 層の単一情報源として `scripts/detect.sh` に実装される。検査対象種別を増減するときの変更箇所は `scripts/detect.sh` と対応する checker のみに収まる
- ファイル → target の解決ロジック (trigger blast radius の判定、 当該範囲内の target 粒度への enumerate、 重複排除など) は classifier に集約される。 dispatch 後の checker は trigger / target の粒度を意識する必要はなく、 受け取った target に対する検査ロジックに専念できる
- trigger > target の checker では classifier が blast radius 内を scan する処理 (Go なら影響 module 内の package 列挙) を持つため、 classifier 側に当該種別固有の構造知識 (Go module レイアウト等) が入る。 file-local checker (Markdown / Shell / JSON / YAML 等) ではこの追加処理は発生せず、 従来通り「変更ファイル → target」の素朴な対応で完結する
- checker は `scripts/check-<type>.sh` (operation 単一の場合) または `scripts/check-<type>-<operation>.sh` (operation 別に分割する場合) の形式で実装され、 hook 層からは `scripts/check.sh` 経由で起動 (`scripts/check.sh` は bash の background job と `wait -n` ベースのセマフォで checker を並列ディスパッチし、 checker ごとに stdout / stderr を buffer して detect.sh 入力順に flush する)、 CI 層からは `wf__check.yaml` の matrix strategy 経由で並列起動される。 種別追加 / operation 追加時の作業は (1) `scripts/detect.sh` への分類 + target 解決ロジック追加、 (2) `scripts/check-<type>(-<operation>)?.sh` の新規作成、 の 2 点に収まる
- 検査の実行環境は devbox に固定されるため、hook 層は checker 冒頭の `scripts/devbox-activate.sh` を介した self-activation で、CI 層は `devbox run -- bash scripts/<checker>` 経由で checker を起動する ([FLM_ENG_0002](../engineering/FLM_ENG_0002__devbox.md))。devbox 環境の変更は両経路に同時に波及する
- 1 ファイルが複数種別に該当する / 1 種別の中で複数 (checker, target) に展開される場合、当該ファイルは複数 checker に target として重複入力される。 検査時間は (checker, target) ペア数分かかるが、 ペア間の責務が明確に分離される ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) の階層継承および同一種別内 operation 分割の具体形)
- directory / package / module 粒度の target では同一 directory 内の複数ファイルが変更されても classifier 段で 1 target に畳み込まれるため、 同一 target に対する重複検査は発生しない。 一方、 1 ファイルだけ変更しても当該 target 配下全体が検査範囲となり、 検査時間が target 規模に依存する
- trigger > target の checker では 1 ファイル変更を契機に blast radius 内の全 target が enumerate されるため、 変更されていない target も再検査される。 検査時間は blast radius (例えば 1 module) 内の target 数に比例する。 これは依存影響 (例: `internal/foo` 変更が `cmd/server` の build に影響) を取りこぼさないための安全側の設計である
- hook 層はプロセス内セマフォで並列起動するため、(checker, target) ペア数の増加に対する hook 応答時間の伸びは並列度上限まで吸収される (上限超過分は順次化されるため線形成分は残る)。 CI 層は matrix で並列化されるため、 ペア数の増加が並列化に直結し総 CI 時間を抑制できる。 重い operation (Go の build / test 等) は target 粒度を細かく切ることで CI 時間短縮の効果が大きい
- 1 種別の中で同一 operation の checker が 1 つだけある場合 (Markdown lint 等) は 1:1 対応の degenerate ケースとなり、 既存種別の構成に影響しない
- 1 checker 内で 1 target 単位の検査と種別該当集合単位の検査を併存させられるため、 当該 checker が必要とする検査範囲を 1 checker に集約できる (例: ADR の checker は 1 ファイル単位の filename / required sections と、 project-wide の連番 dense 性検査の両方を持つ)
- 同一 checker を hook 層と CI 層で共有するため、 hook 層で発見できなかった違反 (AI のハルシネーション、 hook 層がバイパスされたケース、 AI を介さない直接編集等 — [FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) は CI 層でも同じ checker に同じ違反として検出される
- checker の独立性により、 新 checker 追加時に他 checker への影響を考慮する必要がない。 一方、 複数種別にまたがる整合性検査 (例: ADR と補助ドキュメントの sync) は 1 checker 内に閉じない場合に「どの checker に集約させるか」「別 checker として独立させるか」を設計時に判断する必要がある
- hook 層が fix モードで起動するため、 auto-fix capability を持つ checker (gofmt / markdownlint --fix / go mod tidy 等) は AI / 人間のターン内修正サイクルにおいて機械的修正を肩代わりする。 hook の応答時間は fix 処理分だけ伸びるが、 AI のターン消費・トークン消費は減る
- fix モードで auto-fix が走った場合、 stop-hook が静かにファイルを変更する (AI が直前のターンで触れていないファイルでも、 同 module / 同 package 内であれば classifier の trigger blast radius 内で対象になり得るため修正対象に含まれる)。 変更内容は次回 hook fire 時に state baseline として反映される
- CI 層は diagnose モード固定のため CI workspace は読み取り専用で安定する。 fix が必要な違反は CI で fail として顕在化し、 PR 作成者がローカル (hook fix) もしくは手動 fix で解消する
- auto-fix を持つ tool / 持たない tool が混在するため、 同じ fix モード起動でも具体的な動作は checker ごとに異なる。 持たない checker は両 mode で diagnose 動作のみとなり、 mode 機構は no-op として degenerate する
- mode 切替の影響範囲は checker 内部に閉じ、 dispatch (`scripts/check.sh`) や classifier (`scripts/detect.sh`) は mode を意識しない。 mode 切替の責務は呼び出し元 (hook / CI) と checker 実装に二分される
- mode 伝搬は環境変数 `FLAME_CHECKER_MODE` (値: `fix` / `diagnose`) で実装する。 hook 層 (`scripts/stop-hook-review.sh`) は `env FLAME_CHECKER_MODE=fix bash scripts/check.sh ...` の形で fix を明示し、 CI 層 (`.github/scripts/run-checker.sh`) は `env FLAME_CHECKER_MODE=diagnose ...` で diagnose を明示する。 default は `diagnose` で、 手動 `bash scripts/check.sh` 起動時もファイルを変更しない
- 本 ADR の規約は依存側プロジェクトへも伝播する (本 ADR は FEA カテゴリのため)

## 評価

代替案として以下を検討した。

- **hook 層用と CI 層用の checker を別実装する**: hook 層は同期で軽量、CI 層は並列で機械可読出力等、要件が異なるため別実装が一見最適化される。しかし [FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) は「hook で実装可能な検査も CI 側で重複実行する」を要請しており、両経路で同じ違反集合を検出する保証が必要となる。別実装にすると種別追加・ルール変更ごとに両側を同期させる保守コストが発生し、長期的な乖離 (一方だけがルール変更に追従する事故) を招く。同一 checker を両経路から呼び出す方を採用した。
- **classifier も hook 層と CI 層で別実装する**: 種別判定ロジックが二重化し、種別追加・変更時の整合維持コストが二重に発生する。`scripts/detect.sh` を単一情報源として共有することで、種別追加・変更の影響範囲を狭める方を採用した。
- **checker を 1 つにまとめ、内部で種別判定する**: シェル 1 ファイルにすべての検査を畳み込む構成。CI 層で並列化の利点が失われ、種別追加が単一ファイルへの追記となって当該ファイルが肥大化する。種別ごとに checker を切り出して 1:1 対応とする方を採用した。
- **検査範囲 (1 ファイル単位 / 集合単位 / project-wide) を横軸として checker を細分化する**: 種別 × 検査範囲の直交分解で checker 数が爆発し、種別追加時の作業も増える。種別 1 軸で 1 checker とし、検査範囲の混在は checker 内で吸収する方を採用した。
- **checker の失敗で他 checker の実行を止める (fail-fast)**: 早期失敗でリソース節約の利点はあるが、AI / 人間が 1 ターン / 1 PR 中の全違反を把握するには再実行が必要となり、修正サイクルが長くなる。AI が同一ターン / 同一 PR で全違反を一度に把握して fix できることを優先し、failure 非伝播を採用した。
- **1 ファイルの複数種別該当を排他化する (1 ファイル = 1 種別)**: ADR が Markdown でもある等の階層継承 ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md)) を表現できなくなる。1 ファイルが複数 checker に重複入力される構成を採用した。
- **checker の入力単位を全種別 file-unit に固定する (target = ファイルパスのみ)**: 入力契約が単一で classifier の責務も「分類のみ」に閉じる単純さがある。 一方、 Go module のように検査の最小単位がファイルではなく package / build / test 等の高位単位である種別では、 (1) checker 側で「受け取った各ファイルから所属単位を再解決して重複排除する」というロジックを各種別ごとに再実装する必要が生じる、 (2) 同一単位内の複数ファイル変更で同じ単位を複数回検査する重複が発生する、 (3) 「ファイルが入力」という契約と「高位単位に対して検査する」という実装が乖離して checker 内部の見通しが悪くなる、 という不利益がある。 ファイル → target の解決を classifier に集約する方を採用した。
- **checker の入力単位を「file-unit / module-unit」の 2 値に固定する**: 種別ごとに file か module を選べる柔軟さはあるが、 同一 module 内に異なる粒度 (lint = package、 build = main package、 test = chunk) が併存する Go のような種別では「module-unit に固定すると粒度が粗すぎて parallelism と意味的整合の両方を捉えきれない」。 target の粒度を file / module の 2 択ではなく **任意** とし、 種別 (および種別内の operation) ごとに自然な粒度を選ぶ方を採用した。
- **コンテンツ種別と checker の対応を 1:1 に固定する**: 種別追加時の作業が単一 checker の追加で済む明快さはある。 一方、 1 種別の中で性質の異なる operation (Go の lint / build / test) を 1 checker に詰めると、 (1) CI matrix で operation 別並列化ができず、 重い operation (build / test) で総時間が膨らむ、 (2) operation の追加・削除が単一ファイルへの集中変更となる、 という不利益がある。 1 種別あたり 1+ checker を許容し、 operation 別の checker 分割を可能にする方を採用した。 operation が 1 つしかない種別では従来通りの 1:1 対応となる。
- **(checker, target) ペアごとに CI matrix entry を必ず 1 つずつ展開する (per-pair flatten)**: 最大の並列度を取れるが、 軽量 checker (Markdown lint など) では target 数 × checker 数の matrix entry がそれぞれ checkout + devbox 起動コストを払うため総時間が逆に伸びる。 detect.sh は 1 行 = 1 (checker, target 配列) を emit する形を維持し、 重い operation を細粒度化したい場合のみ checker / detect 側で 1 行 = 1 (checker, 単一 target) に分割する選択を取れる構成にした。
- **target 単位を種別ごとに固定せず、 1 種別の中で複数粒度を混在させる**: 1 種別の中で「同じ operation を異なる粒度で実行する」必要が出るのは想定が薄い。 異なる粒度が必要な場合は別 operation として扱える (= 別 checker に分ける) ため、 1 checker は 1 粒度を取る形で十分。
- **trigger を全 checker で file-local に固定する (変更ファイル → 当該ファイルだけ再検査)**: classifier が単純化される一方、 依存追跡を要する operation (Go の build / test 等) で変更ファイルの所属 package のみ再検査になり、 依存先 package への波及 (例: `internal/foo` の変更が `cmd/server` の build に影響) が検出できない。 trigger を operation ごとに選べるようにして、 依存ありの operation には wider な trigger (module 等) を割り当てる方を採用した。
- **trigger > target の検査について classifier ではなく checker 側で enumerate する**: 「ファイルが変更された」という入力を checker に渡し、 checker が内部で「自分の所属 module を引き、 全 package を enumerate」する。 一見 classifier をシンプルに保てるが、 (1) 同種の enumerate ロジックが checker ごとに重複する、 (2) 「変更ファイルが入力」という契約と「依存範囲全体に対して検査する」という実装が乖離して checker 内部の見通しが悪くなる、 (3) classifier の出力が「dispatch 後に何が走るか」を予測できなくなり (CI matrix 設計や hook 状態管理に必要な情報が無くなる)、 という不利益がある。 trigger blast radius と target enumerate を classifier に集約する方を採用した。
- **hook 層も diagnose のみとし、 fix も AI に行わせる**: AI が format / manifest tidy 等の機械的修正を毎ターン行うことになり、 トークン / レイテンシ / context のいずれも消費する。 機械的に解ける fix は機械にやらせる方が AI ターン内サイクル ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) を意味的修正に集中できるため、 hook = fix を採用した。
- **CI 層でも fix を走らせて auto-commit する (pre-commit.ci 的な動き)**: format 違反が PR に持ち込まれても CI が修正 commit を追加して fail を回避できる利点がある。 一方、 (1) CI に PR への push 権限を持たせる必要がある、 (2) PR 著者の意図しないコミット履歴を生む、 (3) CI の責務が「検証」から「修正適用」に膨らみ「最後の砦」 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) としての位置付けが曖昧になる、 という不利益がある。 CI = diagnose 固定とし、 fix が必要な違反は CI fail として PR 著者に明示する方を採用した。
- **mode を環境変数ではなく flag (`--mode=fix`) で渡す**: explicit さは増すが全 checker で flag parsing コードが重複し、 既存の checker 引数 (target list) と混在するため flag 位置の規約 (前置 / 後置) を別途定める必要がある。 環境変数経由なら dispatch (`scripts/check.sh`) は何も意識せず checker に伝搬し、 各 checker は `${FLAME_CHECKER_MODE:-diagnose}` を読むだけで済む。 環境変数を採用した。
- **default を `fix` にする**: hook 起動時に何も export せずとも fix が default で動く利点があるが、 default 動作がファイル変更を伴うのは安全側に倒せていない。 手動で `bash scripts/check.sh ...` を実行した開発者が想定外にファイル変更を受けるリスクもある。 default は diagnose とし、 fix を要する hook 側で明示的に export する方を採用した。
- **hook 層の checker dispatch も逐次起動のままにする**: 実装が単純で出力もそのまま流せる利点があるが、 checker 数が増えるほど hook 応答時間が線形に伸び、 AI ターン終端のフィードバック遅延が体感に影響する。 プロセス内セマフォ並列に変更し、 出力決定性は checker ごとの buffer + 入力順 flush で担保する形を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初は checker の入力契約を「変更ファイル群 (ファイルパスの配列)」に固定し、 classifier の責務も「ファイルを種別に振り分けるだけ」としていた。 既存種別 (Markdown / Shell / JSON / YAML 等) はいずれも 1 ファイル単位で検査の正当性が成立するため当該契約で十分だった。 Go 種別を追加した際、 `go vet` / `go test` / `go build` などが module / package を解決単位とすることが判明し、 「ファイル入力 → checker 内部で所属単位を再解決 → 重複排除」という変換ロジックが checker 側に染み出した。 ファイル → target 解決を classifier 側 (`scripts/detect.sh`) に集約し、 入力契約を「target 配列」へ拡張する形に改訂した。
- 上記改訂の初版では target 単位を「file-unit / module-unit」の 2 値に固定し、 種別ごとにいずれかを選ぶ形にしていた。 しかし Go では同一 module 内に lint = package、 build = main package、 test = chunk と異なる粒度が併存するため、 「module-unit 1 つに畳み込む」と build / test の並列化粒度が module 単位に縛られて重い operation の CI 時間を抑えられない問題が出た。 target 粒度を「任意」にし、 種別 (および種別内の operation) ごとに自然な粒度を選べる形に再改訂した。 これに伴い「コンテンツ種別と checker の対応」も 1:1 から 1:N に緩和し、 operation 別 checker 分割を許容することで CI matrix での並列化粒度を operation 単位まで細かく取れるようにした。
- 上記改訂後も「変更ファイル → 当該ファイル enclosing package = target」の素朴な 1:1 解決でしか classifier を実装していなかった。 これでは Go の build / test のように依存追跡を要する operation で「変更ファイルの所属 package のみ再検査」となり、 依存先 package への波及を検出できなかった。 trigger (起動契機の blast radius) と target (1 起動の execution unit) を概念的に分離し、 trigger > target の場合に classifier が blast radius 内の関連 target を全 enumerate する責務を明示する形に再改訂した。 これにより file-local な検査 (Markdown / Shell 等、 trigger = target) と依存追跡を要する検査 (Go の build / test 等、 trigger = module / target = package) を同一規約で扱えるようになった。
- 当初は checker の起動モードを単一 (diagnose のみ) としていた。 hook 層でも検査結果を AI に返すだけで、 format / module manifest tidy などの機械的に解ける違反を AI が手作業で fix する運用となっていた。 (1) 機械的に解ける問題に AI のターン / トークンを消費する、 (2) 同種の修正 (gofmt 違反のたびに gofmt 実行) が繰り返される、 という非効率が顕在化したため、 fix / diagnose の 2 モードを導入し、 hook = fix / CI = diagnose の routing で機械的修正を機械に任せる構成に改訂した。 mode 切替の機構は環境変数 `FLAME_CHECKER_MODE` を採用し、 default を diagnose (安全側) として、 hook 層スクリプトが明示的に fix を export する形にした。
- 当初は hook 層の dispatch を `scripts/check.sh` での逐次 (foreground 直列) 起動としていた。 AI ターン終端の hook 応答時間が checker 数に対して線形に伸びることを背景に、 `scripts/check.sh` の dispatch を bash の background job + `wait -n` セマフォによる並列化に変更した。 並列化に伴い checker ごとに stdout / stderr を buffer し detect.sh の入力順で flush する形にして、 出力順序の決定性を担保している。

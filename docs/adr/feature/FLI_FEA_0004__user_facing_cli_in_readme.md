# user-facing な flame CLI コマンドの root README への記載義務

## 背景

- flame の root README は当該リポジトリの reference の入口として、 「公開する成果物の利用者向けの最小情報 (= 何を install してどう使うか、 主要コマンドの一例等)」 を含める一般 policy が定まっている ([FLM_APP_0005](../../../vendor/flame/docs/adr/application/FLM_APP_0005__readme.md) §リポジトリルートの README に書くこと)
- flame CLI は公開 surface と内部 endpoint を区別する ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md) §公開 surface と内部 endpoint の区別)。 内部 endpoint は subcommand 名 `__` prefix で help 出力から除外される
- flame CLI が集約する補助処理の責務カテゴリは 5 種ある ([FLI_FEA_0002](FLI_FEA_0002__flame_cli.md) §flame の責務カテゴリ具体 list)。 「**harness 導入補助**」 (例: `flame init` / `flame install`) は利用者が手元で能動的に起動する経路、 一方 「静的検査」 「AI hook」 「CI 補助」 「devbox 補助」 は Claude Code hook / GitHub Actions workflow / devbox init_hook 等の自動化機構から呼び出される経路を中心とする
- 利用側 repository が flame harness を導入する流れは `flame init` → `flame.yaml` 編集 → `flame install` の 3 段階で確定している ([FLI_FEA_0002](FLI_FEA_0002__flame_cli.md) §flame init による flame.yaml の初期生成、 [FLM_FEA_0003](../../../vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md) §導入手順)

## 決定

flame self の root README には **user-facing な flame CLI コマンド** の利用方法を記載する。 「user-facing」 とは「flame の利用者が手元のシェルから手動で実行する経路を持つ」 ことを指し、 §user-facing / 自動化-only の判定基準 で定義する。

### user-facing / 自動化-only の判定基準

flame CLI の各 subcommand について、 以下のいずれかに該当すれば **user-facing** と判定し、 root README への記載対象とする。

- 利用者が **flame の導入 / 設定 / 更新を能動的に起動する経路** を持つ (例: `flame init` / `flame install`)
- 利用者が **flame の運用補助を手動で呼び出す経路** を持つ (例: 将来追加されうる `flame doctor` 等の診断系)

逆に、 以下のいずれかに該当する場合は **自動化-only** と判定し、 root README には記載しない (`flame --help` および ADR / 個別 reference ドキュメントで documentation する)。

- Claude Code hook (Stop / PreToolUse) からのみ起動される (例: `flame ai hook pre-push`)
- GitHub Actions workflow からのみ起動される (例: `flame check run <key>` / `flame ci ...`)
- devbox init_hook からのみ起動される (例: `flame devbox init`)
- release / 自動化機構が消費する内部 endpoint (`__` prefix) ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md) §公開 surface と内部 endpoint の区別)

判定は subcommand 単位で行う。 user-facing と自動化-only の両経路で起動される subcommand が将来現れた場合は user-facing 扱いとする (利用者から見える経路があれば README 記載対象に含める)。

### root README への記載粒度

各 user-facing コマンドについて、 root README には以下を記載する。

- コマンド名と 1 行説明 (= 何のためのコマンドか)
- 典型的な起動例 (利用者が最初に試す形)
- 詳細仕様 (= 全 flag / 全モード / エッジケース) は ADR ([FLI_FEA_0002](FLI_FEA_0002__flame_cli.md) §flame init による flame.yaml の初期生成 等) または `docs/reference/<topic>.md` に書き、 README からは link で navigation する

### 追加 / 削除 / 改名時の更新義務

user-facing コマンドを **追加 / 削除 / 改名** する PR では、 同 PR 内で root README を追従更新する。 追従漏れは AI レビュー観点 ([FLM_GEN_0003](../../../vendor/flame/docs/adr/general/FLM_GEN_0003__feedback_loop.md) の `git push` 直前 hook) で検出する。 自動化-only コマンドの追加 / 削除 / 改名は本義務の対象外。

## 影響

- 利用側 repository の作業者が `git clone` 直後に root README を読むだけで harness 導入の最小経路 (`flame init` → `flame install`) を把握できるようになる
- root README に記載される情報は user-facing コマンドの一覧 + 典型例に限定されるため、 README 肥大化は防がれる ([FLM_APP_0005](../../../vendor/flame/docs/adr/application/FLM_APP_0005__readme.md) §肥大化を防ぐための工夫 と整合)
- 自動化-only コマンド (例: `flame check run` / `flame ai hook pre-push`) の追加 / 改名は root README の追従を要求しない。 これらの documentation は `flame --help` と ADR / hook 設定が担う
- user-facing / 自動化-only の判定境界が将来曖昧になる subcommand (例: 利用者が手動で診断目的に呼びうるが automation でも呼ばれるもの) が現れた場合は本 ADR の判定基準に従い user-facing 扱いとする (= README に記載する)
- 本 ADR は flame self の internal ADR ([FLI_GEN_0001](../general/FLI_GEN_0001__adr_prefix.md))。 利用側 repo には配布されない (= 利用側 repo の root README は [FLM_APP_0005](../../../vendor/flame/docs/adr/application/FLM_APP_0005__readme.md) の一般 policy のみ適用される)

## 評価

代替案として以下を検討した。

- **user-facing / 自動化-only の区別を持たず全 subcommand を README に列挙する**: README が `flame --help` の出力と等価に肥大化し、 [FLM_APP_0005](../../../vendor/flame/docs/adr/application/FLM_APP_0005__readme.md) §肥大化を防ぐための工夫 §機械的に取得できる情報を書きそうになったら、 それは「書かないこと」 として削る と衝突する。 自動化-only コマンドの存在は利用者の関心事ではなく、 README からは外す方を採用した
- **user-facing コマンドの documentation を README に置かず `docs/reference/<topic>.md` のみに置く**: README は完全に entry point + link 集として最小化できる利点がある。 一方、 (1) `git clone` 直後の利用者が「最初に何を打てば動くか」 を見つけるための 1 hop 余分なクリックが入る、 (2) [FLM_APP_0005](../../../vendor/flame/docs/adr/application/FLM_APP_0005__readme.md) §root README に書くこと (2) 「公開する成果物の利用者向けの最小情報 (= 何を install してどう使うか、 主要コマンドの一例等)」 と整合しない、 という不利益がある。 主要 user-facing コマンドの最小情報 (名前 + 1 行説明 + 1 例) は README 直書きとし、 詳細仕様は ADR / `docs/reference/<topic>.md` への link で委譲する形を採用した
- **user-facing / 自動化-only の判定を subcommand のディレクトリ配置 (例: `cli/internal/root/<cmd>/` 直下 = user-facing、 `cli/internal/root/<group>/<cmd>/` = 自動化-only) で機械化する**: 判定が機械的になり README 反映漏れを静的検査で防げる利点がある。 一方、 (1) 既存のディレクトリ配置 ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md) §subcommand package の階層) は user-facing / 自動化-only の軸ではなく CLI コマンド階層と物理ディレクトリの 1:1 対応で決まっている (例: `flame install` は user-facing だが `cli/internal/root/install/`、 `flame check run` は自動化-only だが `cli/internal/root/check/run/` で同じ階層 1 段)、 (2) 物理配置を user-facing 軸で動かすと既存の階層規約と衝突する、 という不利益がある。 判定基準を ADR §user-facing / 自動化-only の判定基準 で文字記述し、 反映漏れは AI レビューで検出する形を採用した

## 過去経緯

過去に採用していた決定として以下の経緯がある。

- 当初は user-facing flame CLI コマンドの root README 記載は [FLM_APP_0005](../../../vendor/flame/docs/adr/application/FLM_APP_0005__readme.md) §root README に書くこと (2) 「公開する成果物の利用者向けの最小情報 (= 何を install してどう使うか、 主要コマンドの一例等)」 の解釈に委ねられていた。 一般 policy のみでは「flame self においてどの subcommand が利用者向けで、 どの subcommand が自動化-only か」 の判定基準が ADR 上に明文化されておらず、 PR レビュー時に「README に書くべきか書かなくて良いか」 が都度議論になっていた。 本 ADR で flame self の責務カテゴリ list ([FLI_FEA_0002](FLI_FEA_0002__flame_cli.md) §flame の責務カテゴリ具体 list) と連動する判定基準を明文化し、 反映漏れを AI レビュー観点で機械化する経路に整理した

# flame self では module 配布対象の依存を release 経路に固定し独立 PR sequence を強制する

## 背景

- flame self は複数の配布対象 module を持つ。 Go module としては `lib/` (配布対象 library。 [FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置) と `cli/cmd/flame_tool/` (配布対象 binary)、 配布チャネルとしては `.claude-plugin/` (Claude Code plugin) と `.github/workflows/wf__*.yaml` (reusable workflow) ([FLM_FEA_0003](../../../vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md) §3 配布チャネル)
- 各 module は GitHub Release を経由して外部 caller (= 利用側 repo の Go module fetch、 flame CLI installer、 reusable workflow ref pin 等) に届く ([FLI_FEA_0001](../feature/FLI_FEA_0001__github_release.md))
- module 間には方向性のある依存関係がある: `cli` は `lib` を Go module path (`github.com/wakuwaku3/flame/lib`) 経由で `require` する。 `wf__*.yaml` は `cli` の subcommand を起動する。 利用側 repo の wf は flame の `wf__*.yaml` を `uses:` で参照する
- Go toolchain は依存解決の override 機構として 2 種を持つ:
  - `go.mod` の `replace` directive (local path 形式 / remote module 形式) で個別 require を別解決に差し替える
  - Go workspace (`go.work` / `go.work.sum`、 Go 1.18+) で複数 module を 1 つの workspace として束ね、 同 workspace 内の module 間依存を local に解決する
- GitHub Actions workflow の `run:` 内では任意 shell を実行可能なため、 `go build -o ~/.local/bin/flame ./cmd/flame_tool` のような形で PR head の flame CLI source を直接 build して run することも syntactically 可能
- 上記 3 経路 (replace / go.work / wf 内 source-build) はいずれも「下位 module の GitHub Release を待たずに上位 module から下位の新機能を参照する」 shortcut として機能する
- shortcut を許容すると 1 PR で 「下位 module の改修 + 上位 module から下位の新機能を使用」 を同時に詰め込めるが、 同 PR が merge された瞬間に main の release 経路が更新されるため、 外部 caller (利用側 repo / 他 module の release-installed flame) では下位の新版が届く前に上位の利用が走る状況が起きうる
- 既に downstream の Go ADR ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §go.mod directives) は `replace` directive を全面禁止する規約を持ち、 `.golangci.yaml` の `gomoddirectives` linter で静的検査されている。 一方 `go.work` と wf 内 source-build については flame 全体で規約が存在しない
- 既存 `wf__check.yaml` の `install_drift` job は PR head の flame CLI source 存在を判定して `go build` で head 反映 binary を作る分岐 (`if [[ -d cli/cmd/flame_tool ]]; then go build ...; else use-release; fi`) を持つ。 これは PR 内 cli 改修の自己テストを意図したものだが、 同 PR で「cli 改修 + 新 cli 機能を使う wf 改修」 を同時に行える経路として機能している

## 決定

flame self では module 配布対象の依存を **release 経路に固定** する。 同一 PR で下位 module の release 待ちを skip する shortcut を採用しない。 具体的には以下を禁ずる。

### 1. Go workspace (`go.work` / `go.work.sum`) を採用しない

- flame self repo では `go.work` / `go.work.sum` を **tracked file として保持しない**
- 開発者が local で workspace 駆動検証を行う場合も commit してはならない

### 2. `replace` directive を採用しない

[FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §go.mod directives の規約 (local path 形式 / remote module 形式の双方を禁止) を flame-internal な規範として表明する。 静的検査経路は同 ADR §影響 で導入済の `.golangci.yaml` `gomoddirectives` (`replace-local: false` / `replace-allow-list: []`) を継承する。

### 3. GitHub Actions workflow 内で flame CLI を source-build しない

- `.github/workflows/wf__*.yaml` 配下の `run:` から flame CLI を `go build` 等で source-build した binary を実行することを禁ずる
- flame CLI は **GitHub Release で配布された binary** (= `cli/scripts/install.sh` 経由で取得) のみを利用する
- 既存 `wf__check.yaml` の `install_drift` 等で採用していた `if [[ -d cli/cmd/flame_tool ]]; then go build ...; else use-release; fi` 形の分岐を撤去し、 release 経由のみに統一する

### PR 分割規約

下位 module の改修と上位 module 内での当該改修利用は **別 PR に分割** する。 具体的には:

- `lib` の API を新規追加して `cli` から利用する場合: lib 変更 PR → merge → release → cli 変更 PR の 2 段階
- `cli` の subcommand を新規追加して `wf__*.yaml` から起動する場合: cli 変更 PR → merge → release → wf 変更 PR の 2 段階
- 上記が連鎖する場合 (lib → cli → wf): 3 段階

各下位 PR が release されたことを確認してから上位 PR を開く。 release 経路 ([FLI_FEA_0001](../feature/FLI_FEA_0001__github_release.md)) は main への merge を契機に走るため、 lower の release tag が green になるまで上位 PR を待つ運用とする。

### 後方互換性の確保

上位 caller から見て backward compatible な subcommand 拡張 (= 新規 env / flag を未設定でも degrade して動く形) を採る場合は、 1 PR で完結できる。 具体的には:

- cli の subcommand が新規 env を required で読む変更は禁ずる (既存 wf からの呼出を壊す)
- 既存 env / flag を追加読みする場合は optional として、 未設定なら従来挙動と等価になるよう実装する
- 上記後方互換実装で release した後に、 wf 側で当該 env を set する変更を別 PR で行う

## 影響

- flame self では複数 module を local で connect する手段が無くなる。 lib → cli の利用は GitHub Release 経由の `require` 公開版に固定される
- `wf__check.yaml` の既存 `install_drift` job は build-from-source 分岐を失い、 PR head の cli install 実装を CI で自己検証できなくなる。 install 実装変更は release 後に install_drift gate で確認する形に変わる
- 新規 cli subcommand を wf から呼ぶ場合、 cli 改修 PR と wf 改修 PR の 2 段階に分かれる。 1 PR 内で完結したい誘惑があっても release sequence 強制のため許容しない
- `.gitignore` に `go.work` / `go.work.sum` を登録する (偶発的な `git add` 防衛層)
- replace directive の静的検査は [FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §影響 由来の既存 `gomoddirectives` 設定をそのまま継承する。 本 ADR では独自の grep 検査を新設しない
- `go.work` の静的検査は flame CLI 側に `flame ci block-go-workspace` 等の endpoint を新設し、 wf__check.yaml に独立 job として組み込む。 ただし当該実装は本 ADR とは別 PR (cli 改修 PR → release → wf 改修 PR の 2 段階) で導入する
- wf 内 source-build の静的検査は同様に別 PR で flame CLI 側に endpoint を追加して導入する。 本 ADR では既存違反 (install_drift の build-from-source 分岐) の撤去のみを併せて行う
- 本 ADR は flame-internal (FLI prefix) なため利用側 repo には伝播しない。 利用側 repo は自身の運用に従う

## 評価

代替案として以下を検討した。

- **3 つの shortcut のうち `go.work` / `replace` のみ禁止し、 wf 内 source-build は許容**: wf 自己テストの利便性 (PR 内 cli 改修を CI で即時検証) は得られるが、 release sequence 強制という本 ADR の意図が形骸化する。 同一 PR で「cli 改修 + 新 cli 機能を wf で利用」 を依然詰め込め、 release 待ちを skip する経路が残る。 3 shortcut を一律禁止する方を採用した
- **wf 内 source-build を install_drift 等の self-test job 限定で carve-out**: install_drift の意図 (PR head の install 実装が drift を生まないことの検証) は維持されるが、 carve-out の境界が「self-test job」 という曖昧基準になり拡張防止が効かない。 後続 PR で別 job が同様の carve-out を主張し始めると規約が骨抜きになる。 carve-out を持たず一律禁止する方を採用した
- **shortcut 禁止を ADR で固定せず PR レビューで都度判断**: その場の事情に応じて柔軟運用できる利点はあるが、 静的にチェックできるルールを静的チェックで担保する flame 方針 ([FLM_GEN_0004](../../../vendor/flame/docs/adr/general/FLM_GEN_0004__static_check.md)) と整合しない。 AI ターン内 / PreToolUse hook で機械的に弾けない経路は事故が起きる。 ADR で固定し静的検査を別 PR で導入する方を採用した
- **release 経路を `cli/scripts/install.sh` 以外にも複線化 (例: 事前 build artifact を別 path で expose)**: source-build 禁止下でも head 反映 binary を CI で扱える経路を別建てで残す案。 release 経路の単一情報源性 ([FLI_FEA_0001](../feature/FLI_FEA_0001__github_release.md)) が崩れ、 「release binary」 と「事前 build artifact」 の整合性を別検査が要するようになる。 複線化せず release 1 経路に固定する方を採用した
- **FLM_APP_0007 §go.mod directives を拡張して `go.work` / wf 内 source-build も downstream で禁止する**: 利用側 repo にも自動伝播する利点があるが、 (1) 利用側 repo の運用 (複数 module 採否、 wf の build 経路) を flame 側から強制するのは過剰な踏み込み、 (2) `replace` 禁止と異なり `go.work` / wf 内 source-build は repo 構造依存で利用側に Go workspace 採用や inline build の自由を残したい余地が大きい。 flame-internal な ADR として分離し、 利用側 repo に伝播させない方を採用した

過去に採用していた決定として以下の経緯がある。

- 当初 `wf__check.yaml` の `install_drift` job 等では `if [[ -d cli/cmd/flame_tool ]]; then devbox run -- sh -c 'cd cli && go build -o ~/.local/bin/flame ./cmd/flame_tool'; else bash .flame-tool/cli/scripts/install.sh; fi` 形の分岐を採用していた。 PR 内 cli 改修を自己テストする利便性を意図したものだったが、 同経路により「下位 module の release を待たず上位 module の利用変更を同一 PR に詰める」 shortcut として機能していた。 release sequence 強制の規律を CI 経路で強制するため、 本 ADR で当該 build-from-source 分岐を全面禁止に改めた

# flame CLI の導入と Go content type の整備 (spec)

## 目的

現在 `scripts/check.sh` / `scripts/detect.sh` 等の shell スクリプトで実装している検査 dispatcher を、Go + cobra で書かれた `flame` CLI に置き換える。同時に、flame に Go を新しい content type として導入し、CLI を支える基盤 (lib / internal の分離、共通拡張 package 群、lint / build / test ハーネス、CD パイプライン、install スクリプト、作成 skill、テストレビュー agent) を一括で整備する。

CLI 化の動機は次の通り:

- shell スクリプトでは構造化が破綻しつつある (`detect.sh` の種別判定・`check.sh` の dispatch ロジックが今後 Go / TypeScript / 他 content type の追加に伴って肥大化する)
- 静的検査の優先方針 ([FLM_GEN_0004](../../adr/general/FLM_GEN_0004__static_check.md)) に従い、判定ロジックを型のある言語で書きテストで担保したい
- 将来 `flame-lib` リポジトリに切り出す共通ライブラリ (cobra 拡張、error 拡張、log 拡張、種別判定など) を flame 側で先に育てる場が必要

## スコープ

本 spec は以下の決定を ADR 化し、関連リソースを実装する根拠ドキュメントとなる。実装作業は spec 確定後に派生タスクで進めるが、本 spec のスコープには「設計だけでなく動くものをデリバする」までを含める。

含むもの:

- CLI を導入するための ADR 群 (Go content type、CLI 構成、lib 設計、テスト戦略、リリース運用)
- `cli/` ディレクトリレイアウトと package 構造の規約
- 共通拡張 package (`x` suffix) 群の初期実装と使い方
- Go content type の 5 項目 ([FLM_GEN_0005](../../adr/general/FLM_GEN_0005__content_type.md)) の整備方針
- ローカル / CI 両層 ([FLM_GEN_0003](../../adr/general/FLM_GEN_0003__feedback_loop.md)) での Go ハーネス
- テスト 3 段階モデル (e2e / service level / unit) と専用 AI レビュー agent (`test-reviewer`)
- CD ワークフロー (CLI コマンドツリー差分による semver 採番、GitHub Release、マルチプラットフォームビルド)
- install スクリプト (`curl ... | bash` 配布、shell completion 同梱)
- CLI 実装修正用 skill
- **`flame check` / `flame detect` の実装** (`scripts/check.sh` / `scripts/detect.sh` と等価の挙動)
- **`scripts/check.sh` および `scripts/detect.sh` の削除** (CLI への切り替え完了と同時。両者の併存期間は設けない)
- **Stop hook を `flame check` 経由に切り替える** (`scripts/stop-hook-review.sh` の更新)

含まないもの:

- 個別の `scripts/check-{type}.sh` の Go 実装化 (CLI 側からは当面 `os/exec` で既存 shell を呼び出すブリッジ実装で繋ぐ。各 checker の Go 化は段階的に別タスクで進める)
- 他言語 (TypeScript / Python 等) の content type 整備 (Go のみを対象とし、他言語は本 spec の構造を参考に別 spec で扱う)

前提とする既整備物:

- GitHub Actions による CI ワークフロー ([spec](../202605031551__spec__github_actions_ci/index.md) を ADR 化して整備済み)。`.github/workflows/` 配下の `trg__*.yaml` / `wf__*.yaml` 命名規約、`wf__check.yaml` を中心とした種別別 matrix、`scripts/detect.sh` を SoT とした dispatch、`actionlint` + `scripts/check-github-actions.sh` による静的検査、`.claude/rules/github-actions.md` による auto-inject、`.claude/skills/github-actions/` の作成 skill が稼働している
- YAML 拡張子の `.yaml` 統一 ([FLM_APP_0004](../../adr/application/FLM_APP_0004__yaml.md) の改修済み)

## 設計

### 命名と全体像

CLI 名は `flame` (実行コマンド)。`cmd/flame_tool/main.go` というディレクトリ命名は CD パイプラインがリリース対象を発見するための suffix 規約 (後述「CD パイプライン」参照) に過ぎず、ユーザがコマンド呼び出しで目にする名前は常に `flame` である。

```text
リポジトリルート
├── cli/                          ← Go モジュール境界 (go.mod を置く)
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── flame_tool/           ← CD 検出用ディレクトリ名 (binary 名は flame)
│   │       └── main.go           ← エントリポイント (cobra root 起動のみ)
│   ├── internal/                 ← 非公開 package (cli モジュール外から import 不可)
│   │   └── root/                 ← cobra のコマンド階層を写像
│   │       ├── root.go           ← `flame` の root command
│   │       ├── check/
│   │       │   ├── check.go      ← `flame check`
│   │       │   ├── json/
│   │       │   │   └── json.go   ← `flame check json`
│   │       │   └── ...           ← 種別ごとに 1 ディレクトリ
│   │       ├── detect/
│   │       │   └── detect.go     ← `flame detect`
│   │       └── schema/
│   │           └── schema.go     ← `flame schema` (CD の semver 比較用)
│   ├── lib/                      ← 公開 package。後日 flame-lib リポジトリへ移動予定
│   │   ├── clix/                 ← cobra 拡張
│   │   ├── ex/                   ← error 拡張 (stacktrace 付与)
│   │   ├── slogx/                ← slog 拡張
│   │   ├── execx/                ← os/exec 拡張 (shell ブリッジ用)
│   │   ├── iox/                  ← io 拡張 (stdin/stdout DI)
│   │   └── testx/                ← テストヘルパ (service level test 共通基盤)
│   └── scripts/
│       └── install.sh            ← curl 配布される install entry
└── docs/adr/engineering/
    ├── FLM_ENG_{Ng}__go.md          ← Go 全般 (lint / format / build / go mod tidy)
    ├── FLM_ENG_{Nc}__cli.md         ← CLI (cobra) 構成・命名・配置・lib 境界
    ├── FLM_ENG_{Nt}__test.md        ← テスト戦略 (3 段階モデルと役割分担)
    └── FLM_ENG_{Nr}__release.md     ← CD / install スクリプト / semver 採番方式
```

ADR の採番は確定時に決定する。

### 配置の決定

- **`cli/` というモジュール境界を切る理由**: flame リポジトリは Go アプリではなくドキュメント + ハーネス資産が主であり、リポジトリルートに `go.mod` を置くと Go モジュールが flame 全体を内包する形になる。`cli/` 配下にモジュールを閉じることで「この CLI とその shared lib」だけを Go モジュールにし、将来 `flame-lib` を分離する際にも `cli/lib/` を別リポジトリに移植しやすくする。
- **エントリポイントを `cli/cmd/flame_tool/main.go` とする理由**: CD が `go.mod からの相対パス cmd/{name}_tool/main.go` を検出してリリース対象とする規約のため、ディレクトリ名側に `_tool` 接尾辞を必ず付ける。一方 CLI バイナリ名 (`-o` で出力する名前 / install 後にユーザが叩くコマンド名) は接尾辞を外した `flame` で固定する (= ディレクトリ命名は CD 用、コマンド名は UX 用、と役割を分離)。
- **`cli/internal/` と `cli/lib/` の境界**: cobra コマンド実装やハーネス内部用ヘルパは `internal/`、ドメイン非依存で他リポジトリからも再利用したい要素は `lib/` に置く。`lib/` の package 階層は flat (サブディレクトリを切らず、package 名だけで識別) とする — 将来 `flame-lib` への移植時にディレクトリ移動が単純になる。

### cobra コマンド階層と package 配置の対応規約

cobra のコマンド階層をディレクトリ構造に 1:1 で写像する。

- `flame` (root) → `cli/internal/root/root.go`
- `flame {sub}` → `cli/internal/root/{sub}/{sub}.go`
- `flame {sub} {leaf}` → `cli/internal/root/{sub}/{leaf}/{leaf}.go`

各 package は当該コマンドの定義 (`*cobra.Command` を返す関数) を 1 つ export し、親 package の生成関数から `AddCommand` で連結する。コマンド以外のロジックは同 package 内、または `cli/lib/` 配下の独立 package に切り出す。

### `cli/lib/` の拡張 package 群

`cli/lib/` 配下の package は、Go 標準ライブラリや外部依存ライブラリの拡張であることを明示するため `x` suffix を付ける。各 package の責務は次の通り。

| package | 拡張対象 | 主な責務 |
| --- | --- | --- |
| `clix` | `spf13/cobra` | コマンド定義の boilerplate を圧縮するヘルパ。命名規約 (`flame {sub} {leaf}` ↔ FS パス) を保つための型付き登録 API、`schema` ダンプの共通実装 |
| `ex` | `errors` | error に stacktrace を自動付与する wrapper。`ex.Wrap(err)` / `ex.New(msg)` で生成し、root command の panic / error ハンドラで stacktrace を出力する |
| `slogx` | `log/slog` | flame で使う handler 設定 (json / human-readable の切替、verbosity フラグ連携、`ex` の stacktrace 統合表示) |
| `execx` | `os/exec` | shell ブリッジ実装 (個別 `check-{type}.sh` を呼ぶ) で使う wrapper。stdout/stderr のストリーミング、exit code と error の統合、コマンドラインのログ出力 |
| `iox` | `io` | stdin / stdout / stderr を `io.Reader` / `io.Writer` で差し替え可能にする DI 基盤。service level test がコマンドの I/O を捕捉できるようにする |
| `testx` | `testing` | service level test 共通基盤 (cobra root を組み立て → 引数で `Execute` → I/O キャプチャ → exit code 確認、を 1 関数で書ける形)。golden file 比較ヘルパ |

初期に整備する package は上記 6 つ。実装中に必要が見えた時点で追加する候補:

- `pathx` — `path/filepath` 拡張。リポジトリ root 探索 (`.git` を辿る等)、相対パス正規化
- `osx` — `os` 拡張。環境変数の構造化アクセス、`os.Exit` の差し替え基盤 (テスト容易化)
- `jsonx` — `encoding/json` 拡張。schema ダンプ等で構造化された出力を整形
- `must` — generic helper (`must.Get(v, err)` で error 時に panic)。`ex` で stacktrace 化されることを前提

これら候補は本 spec で名前と意図のみ確定し、実装時に必要が顕在化したものから追加する。

### 既存 scripts からの移植

`scripts/` 配下の検査 dispatcher を以下のように Go へ移行する。本 spec の完了時点で `scripts/check.sh` / `scripts/detect.sh` は削除済みとなる。

| 既存スクリプト | 移植先 | 備考 |
| --- | --- | --- |
| `scripts/detect.sh` | `flame detect` (`cli/internal/root/detect/`) | 種別判定ロジックは `cli/internal/` 内に閉じる (汎用化が必要になった時点で `cli/lib/` へ昇格)。CLI 完成時に削除 |
| `scripts/check.sh` | `flame check` (`cli/internal/root/check/`) | 種別ごとのサブコマンド (`flame check json` 等) を生やし、種別の追加が package 追加だけで済む形にする。CLI 完成時に削除 |
| `scripts/check-{type}.sh` | `flame check {type}` の内部実装が `execx` で呼び出す | 個別 checker の Go 化は本 spec のスコープ外。当面は CLI が `os/exec` 経由で既存 shell を呼ぶ |
| `scripts/stop-hook-review.sh` | `flame check` に依存する形に更新 | hook script 自体は残し、内部で `flame detect` / `flame check` を呼ぶ |

### Go content type の 5 項目 ([FLM_GEN_0005](../../adr/general/FLM_GEN_0005__content_type.md))

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | CLI コマンドの新設・修正用 skill (`.claude/skills/cli/`) を整備。cobra 階層と package 配置の対応規約・新規 leaf 追加時の手順を定型化する |
| lint | `golangci-lint` を採用。`gofmt` / `goimports` / `go vet` 相当を含む。設定は `cli/.golangci.yaml` に置く |
| build | `go build ./...` を `cli/` ルートで実行。`cmd/flame_tool` のビルド可能性を CI で担保 |
| test | 後述の「テスト戦略」に従い 3 段階で実装。`go test ./...` を `cli/` ルートで実行 |
| ADR ルール検査 skill | テスト分布 (service level vs unit) の妥当性は静的化が困難なため、専用 AI レビュー agent (`test-reviewer`) を後述の通り整備する |

加えて Go 固有の整備項目として以下を追加する。

- **`go mod tidy` 差分なし検査**: `go mod tidy` 実行後に `git diff --exit-code go.mod go.sum` を実行し、差分があれば fail (依存追加忘れ・不要依存残存の防止)
- **cobra ↔ package 配置整合性検査**: cobra コマンドツリーを runtime で探索し、各コマンドの実装 package パスがコマンド階層と一致するかを検証する。Go test として書き、`go test ./...` で同時に走らせる

### テスト戦略

flame の Go コードに対するテストは以下の 3 段階で構成する。テスト戦略自体を独立 ADR (`FLM_ENG_{Nt}__test.md`) として記録する。

| 段階 | 対象 | flame での扱い |
| --- | --- | --- |
| e2e test | システム全体 (実環境のリポジトリに対して install → 実行 → 結果確認) | 採用しない (CLI は単体プロセスのため、service level test で十分) |
| service level test | コマンド単位 (cobra root を組み立て、引数を渡して `Execute`、I/O / exit code を観察) | **正常系の網羅をここで担う**。`flame check` / `flame detect` 等の各サブコマンドについて、入力 → 出力の組を全パターン書く |
| unit test | 関数 / 構造体単位 | **エッジケースのみ**。service level test で網羅しきれない境界条件・エラーパス・lib 内ロジックの細部に絞る |

設計の意図:

- 正常系を service level に集約することで、内部リファクタ (関数分割の再編・lib への切り出し等) が unit test を壊しにくくなる
- unit test を「カバレッジ稼ぎ」にしないため、unit はエッジケース専用と明示する
- service level の共通基盤は `lib/testx/` に置き、cobra root 組み立て・引数渡し・I/O キャプチャ・golden file 比較を 1 関数で書ける形にする

### AI レビュー agent (`test-reviewer`)

[FLM_ENG_0001](../../adr/engineering/FLM_ENG_0001__claude_code.md) の AI レビュー構成に従い、テストの観点を独立 subagent として追加する。

- 配置: `.claude/agents/test-reviewer.md`
- 観点: テスト 3 段階モデルへの準拠
  - 正常系が service level test に書かれているか (unit test に正常系だけのケースが寄っていないか)
  - unit test がエッジケース・境界条件に絞られているか
  - service level test が `lib/testx/` の共通基盤を経由しているか (boilerplate 重複の検知)
  - 新規 / 更新コマンドに対応する service level test が存在するか
- 起動条件: 当該ターンの変更に Go ファイル (`*.go`) または `cli/` 配下の追加・更新が含まれる場合
- 配置段階: `general-practices-reviewer` と修正対象が排他的なため、段階 1 (並列) に配置する
- Stop hook script (`scripts/stop-hook-review.sh`) の起動条件判定に Go 種別判定を追加する

### ハーネス (ローカル / CI)

[FLM_GEN_0003](../../adr/general/FLM_GEN_0003__feedback_loop.md) の 3 層モデルに従い、上記 lint / build / test / `go mod tidy` / cobra 整合性検査を以下で実行する。

#### ローカル層 (Stop hook)

- `flame detect` の判定ロジックに Go 種別 (`*.go` / `go.mod` / `go.sum`) を追加し、`flame check go` に dispatch する
- `flame check go` は `cli/` ディレクトリで以下を順に実行する: `go mod tidy` → `git diff --exit-code go.mod go.sum` → `golangci-lint run ./...` → `go build ./...` → `go test ./...`
- 個別の Go ファイル変更に対しても最小範囲を絞らず `cli/` 全体を回す方針を採用する (`go.mod` / `go.sum` への影響、相互依存、cobra 整合性検査がツリー全体を見る性質のため。実行時間は test 規模が小さいうちは許容)
- Stop hook (`scripts/stop-hook-review.sh`) は `flame check` を呼ぶ形に更新される

#### CI 層

既に稼働している `wf__check.yaml` の matrix に `go` 種別の job を 1 つ追加する。本 spec では既存の CI 構造 (差分計算 → 種別 dispatch → matrix 並列実行) を変更せず、種別の追加のみを行う。job の中身はローカル層と同じ `flame check go` を呼ぶ。

ローカル / CI 双方を完全に同じ実行系で揃えるため、CI でも devbox 経由で Go ツールチェーン (`go` / `golangci-lint`) を解決する。`actions/setup-go` 等の専用 action は使わず、`devbox shell -- ...` または `devbox run ...` でハーネスを起動する。これにより:

- 「ローカルで通ったが CI で落ちる (またはその逆)」という Go バージョンずれが原理的に発生しない
- ツール追加・更新の場所が `devbox.json` の 1 箇所に閉じる
- CI のキャッシュは devbox 自体のキャッシュ + Go の `GOMODCACHE` / `GOCACHE` を `actions/cache` で別途扱う

### CD パイプライン

#### 起動契機とリリース対象の発見

- 起点: `main` への push (= マージ完了)
- リリース対象の発見: リポジトリ全体を走査し、`go.mod` を持つディレクトリ配下で `cmd/{name}_tool/main.go` が存在するパスを列挙。各 `{name}` を独立したリリース単位として扱う
- 本リポジトリでは `cli/cmd/flame_tool/main.go` が該当し、binary 名 `flame` でリリースされる

リポジトリ間で同じ規約を再利用できるよう、CD ワークフローは reusable workflow (`wf__release.yaml`) として実装する。`wf__release.yaml` は既存の `wf__*.yaml` 命名規約 (整備済みの GitHub Actions ADR) に従い、`workflow_call` と `workflow_dispatch` の両入口を備える。`main` への push を受ける起動層は `trg__push__main.yaml` として新設する。

#### semver 採番: コマンドツリー差分による自動判定

conventional commits には依存しない。代わりに、CLI のコマンドインターフェース (= ユーザに見える契約) の差分から bump 種別を決定する。

採番アルゴリズム:

1. 現バージョンのバイナリで `flame schema` を実行し、コマンドツリー JSON を取得 (`schema_new.json`)
2. 前回リリースのタグから artifact (`schema.json`) を `gh release download` で取得 (`schema_old.json`)
3. 両 schema を構造的に比較し、以下のルールで bump 種別を決定:
   - **major**: 既存コマンド / 既存フラグの「型変更」「required 化」「意味変更につながる構造変更」が検出された場合
   - **minor**: 上記に該当せず、コマンド / フラグの「追加」「削除」「リネーム」のいずれかが検出された場合 (削除・リネームは厳密には破壊的変更だが、ユーザ視点での影響度を踏まえ minor で扱う)
   - **patch**: 上記いずれにも該当しない (= 内部実装の変更のみ)
4. 前回タグなし (初回リリース) は `v0.1.0` を発行

`flame schema` コマンドの責務:

- root command 以下の subcommand 階層、各 command の flag 名 / flag 型 / required / 短縮名 / positional argument の shape を JSON で出力する
- `clix` package 内に共通実装を置き、cobra のコマンドツリーを反射的に走査する
- 出力は確定的 (キー順序固定) で、diff が決定論的に取れる形式

schema diff 算出ロジック:

- `clix` 内に `clix.DiffSchemas(old, new) BumpKind` を実装し、CLI から `flame schema diff` で呼び出せるようにする (CD ワークフローからは `flame schema diff --from <prev> --to <curr>` の形で呼ぶ)
- 採番判定は単一の関数に閉じ、CD ワークフロー側はその出力を受けてタグを切るだけにする

#### ビルドとリリース

- マルチプラットフォーム: `GOOS` × `GOARCH` の組み合わせを matrix で展開する。最低限 `linux/amd64` `linux/arm64` `darwin/amd64` `darwin/arm64` を対象とし、Windows サポートは初期は対象外 (install スクリプトが POSIX shell 依存のため)
- ビルド: `go build -ldflags "-X main.version={tag}" -o {name}-{goos}-{goarch} ./cmd/{name}_tool` (binary 名は `_tool` 接尾辞を外した `{name}`)
- アーカイブ: `tar.gz` (Linux / macOS) で `{name}-{goos}-{goarch}.tar.gz` 形式
- artifact に `schema.json` (= `flame schema` 出力) を必ず含める。次回リリースの diff 計算で参照される
- `gh release create` で GitHub Release を作成し、artifact をアップロード。タグは `{name}/v{semver}` (複数 binary が同居するケースに備えてプレフィックス付き)
- リリースノートはコマンドツリー差分のサマリ + 該当期間の commit 一覧から自動生成

#### install スクリプト

- 配置: `cli/scripts/install.sh` (cli モジュール固有のため `scripts/` ではなくモジュール内 [FLM_APP_0002](../../adr/application/FLM_APP_0002__shell_script.md))
- 配布: GitHub Release の latest asset として `install.sh` を同梱。`curl -fsSL https://github.com/{owner}/{repo}/releases/latest/download/install.sh | bash` で実行できる
- 機能:
  - `uname -s` / `uname -m` で OS / arch を判定し、対応する artifact を DL
  - `~/.local/bin/{name}` に配置 (PATH に含まれる前提。含まれない場合の警告は出す)
  - `flame completion {bash|zsh|fish}` を呼んで完了スクリプトを生成し、shell ごとの慣例パス (`~/.local/share/bash-completion/completions/` 等) に配置
  - インストールするバージョンを `--version` フラグで指定可能。未指定時は latest
- shell-script ルール ([FLM_APP_0002](../../adr/application/FLM_APP_0002__shell_script.md)) に準拠

### 作成 skill

CLI 実装修正用の skill を `.claude/skills/cli/SKILL.md` で整備する。

- description: 「`flame` CLI のコマンド追加・修正時に呼ぶ。cobra 階層と package 配置の規約・新規 leaf 追加手順・テスト雛形までを完了させる」
- 本文: ADR (CLI 構成 ADR) リンク + procedural 部分のみ。新規 leaf 追加の 4 ステップ (1. package ディレクトリ作成、2. `clix` 経由で `*cobra.Command` 生成関数を実装、3. 親 package の生成関数で `AddCommand` 配線、4. service level test を `lib/testx/` 経由で追加し `flame check go` 実行)

rule (`.claude/rules/cli.md` および `.claude/rules/go.md`) も合わせて整備し、`.go` / `cli/**` 編集時に対応 ADR を auto-inject させる。

## 制約・前提

- devbox に Go ツールチェーン (`go`、`golangci-lint`) を追加する ([FLM_ENG_0002](../../adr/engineering/FLM_ENG_0002__devbox.md) に従いバージョン明示)。ローカル / CI とも devbox 経由で起動する
- 本 spec は `scripts/check.sh` / `scripts/detect.sh` を CLI へ完全置換するため、Stop hook も同タイミングで `flame check` 経由に切り替える。両者の併存期間は設けない
- 個別 `scripts/check-{type}.sh` は当面残し、CLI が `execx` 経由で呼ぶブリッジ実装で繋ぐ
- 既存 ADR ([FLM_GEN_0005](../../adr/general/FLM_GEN_0005__content_type.md) / [FLM_APP_0002](../../adr/application/FLM_APP_0002__shell_script.md) など) を破壊しない。Go ADR は新設、shell-script ADR は変更しない
- リリースは GitHub Release 1 経路に限定し、Homebrew tap / apt / 他パッケージマネージャ対応は本 spec の範囲外
- Windows サポートは install スクリプトの POSIX 依存のため初期対象外
- CD ワークフローは整備済みの GitHub Actions 規約 (`trg__*.yaml` / `wf__*.yaml` 命名、`workflow_call` + `workflow_dispatch` の両入口、トリガー層と実体層の分離) に準拠する。本 spec はこれらの規約を変更せず、対応する `.github/workflows/` ファイル (`trg__push__main.yaml` / `wf__release.yaml`) を追加するのみ
- 本 spec は既存 CI (`wf__check.yaml` 中心の差分検査 matrix) を破壊しない。Go 種別の追加は matrix エントリと `flame detect` の dispatch 追加のみで吸収される
- `cli/lib/` を flat 構造とする方針は将来 `flame-lib` への抽出時の都合を優先したものであり、Go 慣習 (機能ごとに sub-package を切る) からは外れる
- `x` suffix は flame の lib package を「標準ライブラリ / 外部ライブラリの拡張」として識別するための強制規約とする

## 未解決の論点

- **`{name}_tool` ディレクトリ命名 vs バイナリ名の二段構造**: コマンド名は `flame`、ディレクトリ名は `flame_tool` という非対称が読み手の混乱を生む可能性がある。CD 検出規約のためにディレクトリ命名で意図を表す案と、別ファイル (`.flame-release.json` 等) でリリース対象を明示する案を ADR の評価セクションで比較する
- **タグ命名 `{name}/v{semver}` vs `v{semver}`**: 1 リポジトリ 1 binary の場合は `v{semver}` で十分だが、複数 binary を同居させる将来を見越して prefix を付ける案を採用した。Go module の semantic import versioning (`v2+` で module path に `/v2` を含める) と衝突しないかは確認要
- **`flame schema` の出力安定性**: cobra のリフレクションで取得した情報を JSON 化する際、フィールド順序・追加メタの扱い (例: 短縮名 / hidden flag) を仕様化しないと、版差分で誤検知が発生する。schema スキーマ自体のバージョン管理が必要か (= `schema.json` 内に schema version を埋め込むか) を検討する
- **schema diff の major 判定基準**: 「型変更」「required 化」のみを major としたが、flag の意味的変更 (例: 動作モードを切り替える enum 値の追加・削除) は schema には現れない。本 spec では「schema に現れない変更は patch 扱い」としているが、実装中に違和感が出た時点で再評価する
- **`cli/lib/` の flat 構造の現実性**: package 数が増えた際に flat では衝突回避のため package 名が長くなる。将来 sub-package を解禁する場合の移行コストを許容するか、最初から sub-package を許すかは初期実装で再評価する
- **install スクリプトの署名 / 検証**: `curl | bash` 経路は中間者攻撃のリスクがある。GPG 署名・SHA256 verification を install スクリプト内で行うかは別 spec で扱うか本 spec の制約条件として記すか
- **`test-reviewer` の起動条件粒度**: Go ファイルの変更すべてで起動するか、テストファイル (`*_test.go`) の変更時のみ起動するか、新規コマンド追加検出時のみ起動するか。粒度が粗いと毎回 token cost を払い、細かいとテスト不足の見逃しが起きる
- **service level test の `os.Exit` 取り扱い**: cobra の `Execute()` は内部で `os.Exit` する経路を持つ。テストから捕捉するには `iox` / `osx` 拡張で exit を差し替える必要があり、その差し替え基盤を `lib/` のどこに置くかは初期実装で確定する

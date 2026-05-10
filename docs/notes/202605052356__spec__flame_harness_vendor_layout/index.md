# flame harness を vendor/ 配下に集約し flame.yaml で manifest 化する (spec)

## 目的

flame が育てている共有可能な「harness」資産 (`.claude/`、`.github/`、各種 repository rule、devbox 関連、ADR) を、他リポジトリへ容易に install できる形に再構成する。最終的には `flame install <version>` のような CLI 経由でセットアップ可能にすることを目標とするが、本 spec は CLI 実装をスコープアウトし、その前提となる以下を確定する。

- harness の対象範囲
- vendor 配下のレイアウト (`vendor/flame/`)
- repo に配置済みの「install 後ファイル」と vendor の対応関係を記録する manifest (`flame.yaml`) の schema
- 利用側が install 先ファイルを拡張するための副ファイル overlay 機構と合成方式
- ハッシュによる整合性検査の方針
- `.gitignore` の取り扱い (`vendor/*` + `!vendor/flame`)
- flame 自身も同じ構成にする (dogfooding) ための運用方針

## スコープ

含むもの:

- harness として vendor 化する対象範囲の確定
- `vendor/flame/` 直下のディレクトリレイアウト規約
- `flame.yaml` の schema (vendor → install path のマッピング、ハッシュ、overlay、合成方式の override)
- 副ファイル overlay の命名規約と形式ごとの default 合成方式
- ハッシュ (sha256) の意味と、整合性検査での参照方向
- `.gitignore` への `vendor/*` + `!vendor/flame` の追加
- flame 自身の dogfooding 運用 (vendor が SoT、install path は derived とする)
- ローカル hook / CI の双方で `flame verify` を呼ぶフィードバックループ構成

含まないもの:

- `flame install` / `flame verify` 等の CLI 実装 (別 spec / 別タスク)
- 他リポジトリへの導入手順の細部 (CLI 実装と合わせて別 spec)

前提とする既整備物:

- flame CLI ([FLI_FEA_0002](../../adr/feature/FLI_FEA_0002__flame_cli.md)) が稼働しており、 旧 `scripts/` および `.github/scripts/` 配下の shell スクリプトは全て Go 化されて削除済み。 Stop hook / PreToolUse hook (`git push` 直前) も `flame` CLI 経由で動作する
- ADR ([FLM_GEN_0001](../../../vendor/flame/docs/adr/general/FLM_GEN_0001__adr.md)) / 静的検査 ([FLM_GEN_0004](../../../vendor/flame/docs/adr/general/FLM_GEN_0004__static_check.md)) / lint 抑制方針 ([FLM_GEN_0006](../../../vendor/flame/docs/adr/general/FLM_GEN_0006__no_lint_suppression.md)) / Markdown ([FLM_APP_0001](../../../vendor/flame/docs/adr/application/FLM_APP_0001__document.md)) / YAML ([FLM_APP_0004](../../../vendor/flame/docs/adr/application/FLM_APP_0004__yaml.md)) / Shell ([FLM_APP_0002](../../../vendor/flame/docs/adr/application/FLM_APP_0002__shell_script.md)) / Go ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md)) / Go CLI ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md)) / test ([FLM_APP_0009](../../../vendor/flame/docs/adr/application/FLM_APP_0009__test.md)) / コメント ([FLM_APP_0010](../../../vendor/flame/docs/adr/application/FLM_APP_0010__code_comment.md)) / devbox ([FLM_ENG_0002](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0002__devbox.md)) / Claude Code ([FLM_ENG_0001](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0001__claude_code.md)) / GitHub Actions ([FLM_ENG_0003](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md)) / VSCode ([FLM_ENG_0005](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0005__vscode.md)) のルールが整備済み
- コンテンツ種別の整備項目 ([FLM_GEN_0005](../../../vendor/flame/docs/adr/general/FLM_GEN_0005__content_type.md)) の 5 項目モデルに従う

## 設計

### harness の対象範囲

flame が他リポジトリへ展開可能にする「harness」の対象を以下に確定する。

**対象 (vendor 化する)**:

| 区分 | パス | 備考 |
| --- | --- | --- |
| Claude Code 設定 | `.claude/agents/` | レビュー用 subagent 群 |
| Claude Code 設定 | `.claude/rules/` | ADR への auto-inject mapping |
| Claude Code 設定 | `.claude/skills/` | type ごとの skill |
| Claude Code 設定 | `.claude/settings.json` | hooks 配線 |
| GitHub Actions | `.github/workflows/` | trg / wf 命名規約に従う全 workflow |
| ADR | `docs/adr/` | 全 ADR (general / application / engineering / feature) |
| devbox | `devbox.json` / `devbox.lock` / `devbox/init.sh` | 開発環境定義 |
| direnv | `.envrc` | devbox 起動 |
| repository rule | `.golangci.yaml` / `.markdownlint-cli2.yaml` / `.shellcheckrc` / `.yamllint` | 各 lint の設定 |
| Claude Code 起動文 | `CLAUDE.md` | 最重要ルールの最小化済み本文 |
| VSCode | `.vscode/` | direnv 推奨拡張等 ([FLM_ENG_0005](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0005__vscode.md)) |

**非対象 (リポジトリ固有のため install しない)**:

- `cli/` (flame CLI の Go ソース。 distributable は GitHub Release で配布される)
- `lib/` (flame の公開 Go module 群 (`clix` / `ex` 等)。 利用側は `go get` で取得するため vendor 化しない)
- `docs/notes/` (flow ドキュメント、リポジトリ固有の時系列記録)
- `README.md` (リポジトリ固有)
- `.git/`、`.devbox/`、`.direnv/`、`tmp/`
- `.claude/.cache/`、`.claude/scheduled_tasks.lock`、`.claude/settings.local.json`
- `flame.yaml` 自体 (manifest はリポジトリ固有の install 状態を記録するため vendor 化しない)

### vendor レイアウト

```text
vendor/
└── flame/
    └── harness/
        ├── .claude/
        │   ├── agents/
        │   ├── rules/
        │   ├── skills/
        │   └── settings.json
        ├── .github/
        │   └── workflows/
        ├── .vscode/
        ├── docs/
        │   └── adr/
        ├── devbox/
        │   └── init.sh
        ├── devbox.json
        ├── devbox.lock
        ├── .envrc
        ├── .golangci.yaml
        ├── .markdownlint-cli2.yaml
        ├── .shellcheckrc
        ├── .yamllint
        └── CLAUDE.md
```

レイアウト規約:

- `vendor/flame/` 配下のファイル / ディレクトリ命名は **install 先と完全に同じ** (dot prefix を含めて維持)。`.claude/` は vendor 側でも `.claude/`、`.envrc` は `.envrc`、`.golangci.yaml` は `.golangci.yaml`
- vendor 側のディレクトリ構造と install 先は repo root を起点とした相対パスとして 1:1 対応する (= `vendor/flame/` を repo root に重ね合わせると install 先のレイアウトになる)
- 1 vendor file = 1 install path (1:1 マッピング、テンプレート展開や条件付きコピーは持たない)
- 利用側拡張は副ファイル overlay (後述) で表現し、vendor 側に複数バリアントを持たない

### `flame.yaml` schema

repo root に新規作成する。

```yaml
flame:
  harness:
    source: github.com/quartz-dx/flame    # vendor の取得元
    version: v0.0.0                       # flame ツール (CLI + harness) のバージョン

    files:
      - vendor: vendor/flame/.claude/agents/adr-reviewer.md
        install: .claude/agents/adr-reviewer.md
        sha256:
          vendor: <hex digest>
          installed: <hex digest>

      - vendor: vendor/flame/.claude/settings.json
        install: .claude/settings.json
        sha256:
          vendor: <hex digest>
          installed: <hex digest>
        overlay:
          path: .claude/settings.flame-overlay.json
          sha256: <hex digest>
        merge: deep              # 省略時は形式から自動推論 (後述)

      - vendor: vendor/flame/devbox.lock
        install: devbox.lock
        sha256:
          vendor: <hex digest>
          installed: <hex digest>
        merge: replace           # 生成物のため overlay 不可
```

フィールドの意味:

- `flame.harness.source`: vendor の取得元リポジトリ。CLI が `flame install` 時に fetch 元として参照する
- `flame.harness.version`: install 済みの flame harness のバージョン。flame CLI と同一バージョンを採番する (= flame ツール全体のバージョン。harness 単独で別 semver は持たない)
- `flame.harness.files[]`: install されている各ファイルのレコード
  - `vendor`: SoT 側のパス (repo root 相対)
  - `install`: install 先のパス (repo root 相対)
  - `sha256.vendor`: vendor 側ファイル内容の sha256 (16 進小文字)
  - `sha256.installed`: install 直後の install 先ファイル (= 合成結果) の sha256
  - `overlay` (optional): 利用側拡張の副ファイル情報。存在しない場合はフィールドごと省略
    - `overlay.path`: 副ファイルのパス (repo root 相対)
    - `overlay.sha256`: 副ファイル内容の sha256
  - `merge` (optional): 合成方式の override。省略時はファイル形式から自動推論 (後述)

設計の意図:

- `vendor` / `install` を分離することで、将来 vendor レイアウトの変更 (例: `vendor/flame/v1/` のようなバージョンディレクトリ) があっても install 先は変わらない
- `sha256` を `vendor` / `installed` の 2 種類に分割することで、overlay が存在するファイルでも整合性検査が機能する (vendor 変更 / overlay 変更 / install 後直接編集を区別して検出可能)
- `overlay` を optional にすることで、拡張不要なファイル (= 大多数) のレコードは簡潔に保たれる
- 各レコードを list で持つことで、将来「同 vendor file → 複数 install path」「複数 vendor → 同 install path に merge」等の拡張余地を残す
- フォーマットは YAML を採用 ([FLM_APP_0004](../../../vendor/flame/docs/adr/application/FLM_APP_0004__yaml.md))

### 副ファイル overlay 機構

利用側が install 先ファイルを拡張するため、副ファイル方式を採用する (install 先ファイルへの直接編集は drift として検出する)。

#### 副ファイルの命名規約

副ファイルは install 先と同じディレクトリに配置し、以下の命名で識別する。

- 拡張子のあるファイル: `<basename-without-ext>.flame-overlay.<ext>`
  - 例: `.claude/settings.json` → `.claude/settings.flame-overlay.json`
  - 例: `.golangci.yaml` → `.golangci.flame-overlay.yaml`
  - 例: `CLAUDE.md` → `CLAUDE.flame-overlay.md`
- 拡張子のないファイル: `<basename>.flame-overlay`
  - 例: `.envrc` → `.envrc.flame-overlay`
  - 例: `.shellcheckrc` → `.shellcheckrc.flame-overlay`

副ファイルが存在する場合のみ flame.yaml の `overlay` フィールドが記録される。

#### 形式ごとの default 合成方式

`flame.yaml.files[].merge` を省略した場合、CLI はファイル形式から default 挙動を決定する。

| 形式 / 拡張子 | default 合成方式 | 配列扱い (構造化のみ) |
| --- | --- | --- |
| `.json` (JSON) | deep merge | append (vendor 末尾に overlay を連結) |
| `.yaml` / `.yml` (YAML) | deep merge | append |
| `.md` (Markdown) | append (空行 1 行を区切りとして連結) | — |
| `.sh` (shell) | append (副ファイル末尾追記、shebang は overlay 側に書かない) | — |
| `.envrc` / `.shellcheckrc` 等 (拡張子なしテキスト) | append | — |
| `.lock` 等 (生成物) | replace のみ (overlay 不可。flame.yaml で `merge: replace` 明示) | — |
| その他 (バイナリ含む) | replace のみ | — |

#### `merge` フィールドによる override

形式の default から外れる挙動が必要な場合、flame.yaml の各レコードで明示する。

- `merge: deep` — 構造化 deep merge を強制 (拡張子から推論できないファイルに使う)
- `merge: append` — テキスト末尾追記を強制
- `merge: replace` — overlay 不可 / vendor のみで上書き (生成物に使う)
- `merge_array: append | replace | unique` — 構造化 deep merge 時の配列扱い override (default は append)

具体ケースの想定:

- `devbox.json` の `packages[]` を unique union にしたい場合: `merge: deep, merge_array: unique`
- `.golangci.yaml` の `linters.disable[]` を replace したい場合: `merge: deep, merge_array: replace` (ただし全配列に影響する。配列単位の override は本 spec の schema では持たず、必要なら CLI 実装時に拡張)
- `devbox.lock` のような生成物: `merge: replace`

#### 単一ファイル前提のツールへの対応

利用側ツールが副ファイルを認識しないファイル (例: `CLAUDE.md` は Claude Code が単一ファイルとして読む、`.envrc` は direnv が単一ファイルとして実行) でも、上記の合成方式により「副ファイルを書く → CLI が install 時に合成して install 先 1 ファイルを生成」というモデルで一貫させる。利用側ツールは合成後の install 先ファイルだけを見る。

### ハッシュの意味と整合性検査

`sha256.vendor` と `sha256.installed` の 2 種類を flame.yaml に記録し、`overlay` がある場合は `overlay.sha256` も記録する。

整合性検査の判定 (CLI 側で `flame verify` 等として実装する想定。本 spec ではコマンド名のみ予約):

- `vendor` 側: `flame.yaml.files[].sha256.vendor` と現 `vendor/flame/...` のハッシュを比較 → 不一致なら vendor 改変
- `overlay` 側: `flame.yaml.files[].overlay.sha256` と現 overlay ファイルのハッシュを比較 → 不一致なら overlay 改変
- `installed` 側: `flame.yaml.files[].sha256.installed` と現 install 先ファイルのハッシュを比較 → 不一致なら install 後の直接編集

判定マトリクス:

| vendor | overlay | installed | 状態 | 推奨アクション |
| --- | --- | --- | --- | --- |
| 一致 | 一致 (or なし) | 一致 | 健全 | なし |
| 不一致 | — | 一致 | vendor 更新あり / 未 install | `flame install` で再合成 |
| 一致 | 不一致 | 一致 | overlay 更新あり / 未 install | `flame install` で再合成 |
| — | — | 不一致 | install 後の直接編集 (drift) | 編集を overlay または vendor へ反映 → 再 install、または manifest から eject |
| 不一致 | 不一致 | — | vendor + overlay 双方更新 | 通常運用 (install 前) |

flame 自身の dogfooding 運用:

- vendor/flame/ が SoT、install path は derived
- 開発者が harness を変更する際は vendor 側 (または overlay 側) を編集し、`flame install` で repo の install path に同期する
- install path 側を直接編集するのは原則禁止 (整合性検査で fail させる)

### `.gitignore` の取り扱い

現状の `.gitignore`:

```text
tmp
.devbox
.direnv
.claude/.cache/
.claude/scheduled_tasks.lock
```

本 spec で追加する内容:

```text
# flame harness は vendor/flame/ 配下を SoT として追跡する
vendor/*
!vendor/flame
```

決め事:

- `vendor/*` で `vendor/` 直下の子だけを ignore してから `!vendor/flame` で flame だけ追跡対象に戻す
- 親 dir 自体 (`vendor/` trailing slash) を ignore する形は Git が当該 dir 配下を辿らなくなるため `!vendor/flame/` が機能しない (`gitignore(5)` の "It is not possible to re-include a file if a parent directory of that file is excluded" 制約)
- これにより、他リポジトリで Go の `go mod vendor` 等で `vendor/<other>/` が生成されてもデフォルトで無視される一方、flame 配下は確実にコミットされる
- flame 自身では現状 vendor 利用ケースは無いが、この設定は他リポジトリへの導入時にもそのまま機能するよう、flame で先行採用する

### shell スクリプト dispatcher の扱い (移行完了済み)

旧 `scripts/` および `.github/scripts/` 配下の shell スクリプト群 ([FLI_FEA_0002](../../adr/feature/FLI_FEA_0002__flame_cli.md)) は flame CLI への Go 移植が完了し、 両ディレクトリは repo から削除済みである。 本 spec の vendor 対象は `flame` CLI を前提として組み立てればよく、 移行中の中間状態を schema 側で考慮する必要は無い。

本 spec の schema は「ある時点で何が install されているか」だけを記録するため、 将来 vendor 対象ファイルが増減する場合も flame.yaml の files 配列の追加・削除で表現できる。

### CI 整合性検査

flame 自身も `flame verify` をローカル hook / CI の双方で動かし、harness の vendor / overlay / installed / manifest の整合性を main マージ前に強制する。検査ロジックは CLI に閉じるため、他リポジトリでも同じ仕組みが reproduce される。

#### 検査対象

`flame verify` は以下を全て検査する。

1. **vendor 側ハッシュ整合**: `vendor/flame/<path>` の現ハッシュ == `flame.yaml.files[].sha256.vendor`
2. **overlay 側ハッシュ整合**: overlay 副ファイル (存在する場合) の現ハッシュ == `flame.yaml.files[].overlay.sha256`
3. **install 側ハッシュ整合**: install 先ファイルの現ハッシュ == `flame.yaml.files[].sha256.installed`
4. **vendor 孤児検出**: `vendor/flame/` 配下に `flame.yaml.files[].vendor` で参照されていないファイルが存在しないか
5. **参照実在性**: `flame.yaml.files[].vendor` / `overlay.path` / `install` で指すパスが実在するか
6. (将来追加) **合成再現性**: vendor + overlay の合成結果 (CLI 内で再計算したハッシュ) が `sha256.installed` と一致するか — CLI 実装が成熟したタイミングで追加

#### 実行層 ([FLM_GEN_0003](../../../vendor/flame/docs/adr/general/FLM_GEN_0003__feedback_loop.md))

| 層 | 起動 | 挙動 |
| --- | --- | --- |
| ローカル (Stop hook / PreToolUse hook (`git push` 直前)) | ファイル編集後 / push 前 | 検査項目のいずれかで fail なら中断 |
| CI (`wf__check.yaml` matrix) | PR / `main` への push | 同上で fail (= PR ブロック) |

flame の `flame check` dispatch ([FLI_FEA_0002](../../adr/feature/FLI_FEA_0002__flame_cli.md)) が `harness` 種別を判定し、`flame verify` を呼ぶ形で接続する。`wf__check.yaml` matrix への `harness` 種別追加は本 spec の決定事項とし、具体実装は CLI 側 spec で扱う。

#### drift 検出時の挙動

- 全てのハッシュ不一致 / 孤児ファイル / 参照欠落を fail として扱う (warning にはしない)
- fail メッセージにはドリフト方向 (vendor 改変 / overlay 改変 / install 直接編集 / 孤児 / 欠落) と該当パスを明示
- 復旧は 3 通り: vendor / overlay 編集 → `flame install` で再合成 / install 先編集を overlay に反映 → `flame install` / 該当ファイルを manifest から eject

### 5 項目の整備状況 ([FLM_GEN_0005](../../../vendor/flame/docs/adr/general/FLM_GEN_0005__content_type.md))

「harness manifest (`flame.yaml` + `vendor/flame/`)」を 1 つのコンテンツ種別として整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 整備しない (CLI の `flame install` で完結。新規 vendor file の追加は CLI のサブコマンド経由) |
| lint | flame.yaml の schema 検査 (フィールド型・必須項目・パスの存在性) + vendor / overlay / installed のハッシュ整合性検査。CLI のサブコマンド (`flame verify` 仮称) として実装 |
| build | 該当なし (静的 manifest のため。ただし overlay 合成は build 的側面を持つ。CLI 側で吸収) |
| test | CLI 側 service level test として配置 (vendor / overlay / flame.yaml / install 先の 4 者を fixture として食わせ、verify と install が期待通りに動くか) |
| ADR ルール検査 skill | 不要 (lint で完結) |

### 既存 ADR との関係

本 spec の決定は以下の ADR で記録する想定 (現時点で空き番号は `FLM_FEA_0003`。 実装着手時に最終確定):

- `FLM_FEA_0003__harness.md` (新規想定): harness の対象範囲、vendor レイアウト、`flame.yaml` schema、副ファイル overlay 機構と形式ごとの合成方式、ハッシュによる整合性検査方針

既存 ADR への影響 (本 spec では変更しない):

- [FLM_GEN_0005](../../../vendor/flame/docs/adr/general/FLM_GEN_0005__content_type.md): 新コンテンツ種別 (harness manifest) を追加する想定だが、5 項目モデル自体は変更しない
- [FLM_ENG_0002](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0002__devbox.md): devbox 関連ファイルを vendor 化することは ADR の決定事項を破壊しない
- [FLM_ENG_0001](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0001__claude_code.md): `.claude/` 配下の vendor 化は ADR の決定事項を破壊しない
- [FLM_ENG_0005](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0005__vscode.md): `.vscode/` を vendor 対象に含めることで direnv 推奨拡張等の共通配布が可能になる

## 制約・前提

- 旧 shell スクリプト dispatcher は [FLI_FEA_0002](../../adr/feature/FLI_FEA_0002__flame_cli.md) で `flame` CLI へ移行済み。 `scripts/` および `.github/scripts/` は repo から削除されており、 本 spec はその完了状態を前提に組み立てる
- `flame install` / `flame verify` 等の CLI 実装は別 spec / 別タスク。本 spec はディレクトリレイアウト、`flame.yaml` schema、副ファイル overlay 機構、`.gitignore` の決定事項に閉じる
- vendor/flame/ は flame リポジトリの dogfooding 用途も兼ねる。flame 自身は vendor を SoT、install path は derived とする
- 他リポジトリへの導入時、`flame.yaml.harness.version` で flame の特定リリースタグから harness を fetch する (CLI 側で実装)
- harness の version は flame ツール (CLI + harness) 全体のバージョンと同一採番。harness 単独の semver は持たない
- 既存 ADR を破壊しない。本 spec の決定は新規 ADR (`FLM_FEA_0003__harness.md` 想定) として記録される
- Go の `go mod vendor` は flame の `cli/` モジュールでは現状採用していない。将来採用された場合、`cli/vendor/` は repo root の `vendor/` とは別ディレクトリのため `.gitignore` のパターンと干渉しない
- `flame.yaml` 自体は vendor 化しない (manifest はリポジトリ固有の install 状態を記録するため、vendor が SoT として持つべきものではない)
- 副ファイル overlay の合成は CLI が install 時に行い、合成結果を install 先ファイルとして書き出す。利用側ツールは合成後ファイルだけを見る

## 未解決の論点

- **配列単位の `merge_array` override**: 同一ファイル内で配列ごとに append / replace / unique を切り替えたい要件 (例: `.golangci.yaml` の `linters.disable` は replace、`linters.enable` は append) が出ると、ファイル単位の `merge_array` field では不足する。JSON Pointer / YAML path 単位の override 機構は本 spec では持たず、要件が顕在化したら schema 拡張で対応する

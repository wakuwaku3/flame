# 開発環境マネージャとして devbox + direnv を採用する

## 背景

- flame では Markdown lint・shell lint などの静的検査を AI ターン終端 hook と CI の両方で実行する方針である ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md), [FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md), [FLM_APP_0001](../application/FLM_APP_0001__document.md), [FLM_APP_0002](../application/FLM_APP_0002__shell_script.md))
- これらの検査ツール (markdownlint-cli2、shellcheck 等) は OS 標準ではインストールされていないため、ローカル開発者と CI で同一バージョンを揃えて導入する必要がある
- devbox は Nix パッケージカタログをバックエンドに、`devbox.json` で宣言したパッケージを解決して隔離されたツールチェイン環境を提供する開発環境マネージャである
- `devbox.json` 内のバージョン指定としては、具体的なバージョン番号と `@latest` のような浮動指定の両方が受け入れられる
- devbox は `devbox.json` から解決した実際のパッケージ store path を `devbox.lock` に書き出し、以降は lock の内容に基づいて環境を再構築する
- devbox は shell 起動時に任意のスクリプトを実行する `init_hook` 機構を持つ
- direnv はリポジトリのルートに `.envrc` を置くことで、当該ディレクトリ配下への `cd` 時に環境を自動切替する
- devbox は direnv 連携用の `.envrc` 雛形を `devbox generate direnv` で出力できる
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame の開発環境は以下のルールで構築する。

### パッケージマネージャ

- 開発に必要なツール (lint、formatter、検査用 CLI、言語ランタイム等) は **devbox** 経由で導入する
- パッケージ宣言時のバージョンは具体的な番号で明示し、`@latest` 等の浮動指定は使用しない
- パッケージ宣言ファイルおよびその解決結果を pin した lock ファイルはリポジトリで追跡する

### 自動アクティベーション

- devbox shell の自動アクティベーションは **direnv** との連携で行う
- direnv 設定は devbox 公式の direnv 連携機構を通じて生成する

### ローカル / CI での devbox 起動方式

- ローカル開発では direnv 連携により devbox shell が常時 active になっているため、対話 shell からは devbox 管理下のツール (lint / formatter / 検査 CLI / 言語ランタイム等) を **直接呼び出す** (`bash scripts/...`、`jq ...`、`shellcheck ...` 等)
- `scripts/` 配下のスクリプトのうち devbox 管理下のツールを呼び出すもの (各 check スクリプト、test スクリプト、Claude Code の hook から呼ばれるもの、skill 内の手順例) は冒頭で `scripts/devbox-activate.sh` を source して devbox 環境を現プロセスに自己注入する。これにより devbox 環境が active でない呼び出し経路 (新規 worktree、`direnv allow` 未実施、CI 等) でも追加の手動操作なしに devbox 管理下のツールが使える。devbox 管理下のツールを呼ばない pure-bash スクリプト (dispatcher、classifier 等) は対象外
- self-activation の実装は `eval "$(devbox shellenv --install)"` で行い、不足パッケージの取得 (`devbox install` 相当) も同時に走らせる。スクリプトは「ツール未検出 → 手動 `direnv allow` / `devbox shell` を促してエラー終了」する経路を持たない
- CI など direnv も `scripts/devbox-activate.sh` 経由の起動も使えない経路は `devbox run -- ...` でラップする

### devbox 関連スクリプトの配置

- devbox の `init_hook` から呼び出すスクリプトは `devbox/` ディレクトリに集約する

### ローカル状態の扱い

- devbox および direnv が生成するローカルキャッシュ・内部メタデータはリポジトリで非追跡とする

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 整備 (`.claude/skills/devbox/` で `devbox.lock` 再生成と同一コミット化まで完了させる skill を整備) |
| lint | 継承 (`devbox.json` は [FLM_APP_0003](../application/FLM_APP_0003__json.md) の JSON lint、`devbox/` 配下の shell スクリプトは [FLM_APP_0002](../application/FLM_APP_0002__shell_script.md) の shell lint) |
| build | 整備 (`devbox install` 等で `devbox.lock` を再生成) |
| test | 省略 (作成 skill 内のパッケージ動作確認で代替) |
| ADR ルール検査 skill | 省略 (lint と継承元 ADR の規約でカバー) |

## 影響

- 各開発者は手元で devbox と direnv をインストールする必要がある
- ローカル向けスクリプトと AI ターン終端 hook は devbox 管理下のツールを直接呼び出す前提で書け、`devbox run -- ...` でラップする必要は無い (CI ワークフロー側でのみ devbox 経由起動を意識する)
- `devbox.json` / `devbox.lock` の変更は通常のコードレビューで扱え、ツール追加・更新がコミット履歴に残る
- `devbox.lock` をリポジトリで追跡することで、ローカル開発者と CI で解決される store path が一致する
- `devbox.json` 側で具体バージョンを明示するため、宣言ファイル単体から採用バージョンが読み取れ、`devbox update` 等による解決時刻依存のバージョン変動が発生しない
- パッケージのアップグレードは `devbox.json` 上の番号を明示的に書き換えるコミットとして残る
- 採用可能なツールは devbox がラップする Nix パッケージカタログの範囲に制約される
- `.envrc` の初回読み込み時に `direnv allow` の手動操作が必要となる (対話 shell の自動切替のためだけに必要であり、`scripts/` 配下の self-activate するスクリプトはこの操作を要求しない)
- `scripts/devbox-activate.sh` 経由の self-activation は direnv の「`.envrc` 改変時の手動同意」trust gate を経由しないため、`.envrc` / `devbox.json` の改変が AI / hook 経由の起動には手動同意なしに反映される
- devbox の `init_hook` 内で `set -e` 等の shell オプションや一時変数を直書きすると、利用者の対話 shell に副作用が漏れる懸念があるため、`devbox/` 配下のスクリプトをサブシェル化等で隔離する必要がある
- `init_hook` から呼ばれる shell スクリプトも flame の shell スクリプト規約 ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) と shell lint の対象となる
- devbox / direnv のローカル状態ディレクトリは `.gitignore` への追加で除外する
- 依存側プロジェクトにも devbox + direnv の採用が伝播する (本 ADR は ENG カテゴリのため)

## 評価

代替案として以下を検討した。

- **devbox を採用せず、各開発者がローカル OS のパッケージマネージャ (apt / brew 等) で個別にツールを導入する**: バージョン整合がツールと開発者任せになり、CI と齟齬が発生しやすい。再現可能な単一の真実 (lock ファイル) をリポジトリに置く方を採用した。
- **`devbox.lock` をリポジトリで追跡しない**: ローカルと CI で解決時刻が異なれば store path が一致せず、再現性が崩れる。`devbox.lock` も追跡対象とした。
- **`devbox.json` で `@latest` 等の浮動バージョン指定を許容する**: lock ファイルで store path は固定されるため再現性自体は保てるが、`devbox.json` 単体からは採用バージョンが読めず、`devbox update` 等で意図せずバージョンが進んだ場合の検知が `devbox.lock` の差分頼みになる。`devbox.json` 側で具体バージョンを明示することで、宣言ファイルから採用バージョンが直接読め、更新の意図がコミット差分として明示される方を採用した。
- **mise / asdf を採用する**: 言語ランタイムのバージョン管理に特化しており、shell 系の補助 CLI (shellcheck 等) は別経路で導入する必要がある。devbox は Nix カタログをバックエンドにして言語ランタイムと CLI を同一機構で扱えるため、入手経路を 1 本化できる側を採用した。
- **Docker / devcontainer を採用する**: ホスト shell と分離されるため、hook の起動オーバーヘッドや対話的開発体験のコストが一段重くなる。flame は AI ターン終端 hook を短いレイテンシで回す必要があり ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))、隔離コストの低い devbox を採用した。
- **Nix を直接採用する**: より柔軟だが学習コストが高く、`devbox.json` で宣言できる範囲を超える要求が現時点では無い。devbox という薄いラッパーを介する方が初期コストが低い。
- **direnv を使わず `devbox shell` を手動で起動する**: 起動忘れにより lint 未実行のまま AI / 人間が変更をコミットする事故が起きうる。FB ループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) の hook 層を確実に動かすため、自動アクティベーションを採用した。
- **ローカル実行時も `devbox run -- ...` で常時ラップする**: direnv による自動アクティベーションが既に同じツールチェインを環境に注入しているため、上書きでも新しい保証は得られず、devbox の解決オーバーヘッドと冗長な引用符エスケープが純粋なコストとして残る。ラップ無しを採用し、`devbox run` の利用範囲は CI 等の自動アクティベーションが効かない環境に限定した。
- **devbox の `init_hook` に shell ロジックを直書きする**: `init_hook` の実行コンテキストは利用者の対話 shell と同一プロセスであり、`set -e` 等の shell オプションや一時変数が漏れて副作用を生む。`devbox/` 配下のスクリプトをサブシェルで呼び出す形に分離する方を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初はローカル前提のスクリプト (各 check スクリプト、Claude Code の hook、skill 内の手順例) について、devbox 管理下のツールを直接呼び出し、ツール未検出を検知したら `direnv allow` / `devbox shell` への誘導メッセージを stderr に出してエラー終了する形を採っていた。worktree 増殖時 (worktree ごとに `direnv allow` が必要) や CI 等の自動化経路で摩擦が大きく、特に AI / hook 起点の呼び出しでは「`direnv allow` してください」という案内が何の解決にもならない (AI が自分で `direnv allow` を実行することに本質的な意味は無く、結局 `devbox shellenv` 相当を eval することになる) ため、`scripts/devbox-activate.sh` を冒頭で source する self-activation 方式に切り替えた。direnv の `.envrc` 改変時 trust gate を経由しなくなる代わりに、devbox.json の `init_hook` 副作用禁止条項が load-bearing になる。

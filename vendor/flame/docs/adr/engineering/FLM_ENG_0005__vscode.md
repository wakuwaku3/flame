# レビュー用エディタとして VSCode を採用する

## 背景

- flame は AI エージェントと協働した開発を前提として設計する ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame の協働モデルは AI が判断して進め、 ユーザがレビューで指摘する流れを取る ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- AI 開発 harness は CLI 形式の Claude Code を採用しており、 編集 / コミット / push 等のターン内操作は CLI 上で完結する ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md))
- flame の静的検査は Claude Code の Stop hook で AI ターン終端ごとに強制起動され、 違反は同一ターン内で消化または Stop ブロックされる ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md))。 さらに CI でも push 時に静的検査が走る ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md))
- flame で扱うコンテンツ種別は Go ([FLM_APP_0007](../application/FLM_APP_0007__go.md))、 Markdown ([FLM_APP_0001](../application/FLM_APP_0001__document.md))、 Shell ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md))、 JSON ([FLM_APP_0003](../application/FLM_APP_0003__json.md))、 YAML ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md))、 GitHub Actions ワークフロー ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md)) がある
- Go は静的型付き言語で、 言語サーバ (gopls) が definition jump / 参照検索 / 型情報 hover 等のコード理解支援を提供する
- VSCode は workspace 単位の `.vscode/extensions.json` で推奨拡張を宣言でき、 workspace を開いた利用者にインストールプロンプトを表示する仕様を持つ
- VSCode 本体は gutter blame・Source Control view・Timeline view・diff viewer 等の git 履歴閲覧機能と、 主要言語の構文ハイライト / Markdown プレビュー等を標準搭載している
- VSCode マーケットプレースには各種言語サーバ (gopls 等) や、 GitHub PR / Actions と連携する公式拡張が公開されている
- flame は開発環境マネージャとして devbox + direnv を採用しており ([FLM_ENG_0002](FLM_ENG_0002__devbox.md))、 言語サーバを含む開発ツールは `.envrc` 経由で注入される PATH で解決される。 VSCode 本体は direnv を解釈しないため、 direnv が active な親 shell から VSCode を起動するか、 direnv 連携拡張を介して `.envrc` の環境変数を VSCode プロセスに伝搬させない限り、 言語サーバ拡張は devbox 管理下のバイナリを発見できない
- flame ではコンテンツ種別ごとに 「作成 skill / lint / build / test / ADR ルール検査 skill」 の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame では人間によるレビュー作業の主要エディタとして **VSCode** を採用する。

### 推奨拡張の選定基準

- 推奨拡張はリポジトリで配布する
- 推奨対象は以下のいずれかに該当する拡張に限定する:
  - **言語サーバによるコード理解支援**: 静的型付き言語の definition jump / 参照検索 / hover / 型情報表示等、 レビュアーが変更コードの呼び出し先・呼び出し元・型を辿るのに直接寄与するもの
  - **GitHub の review workflow との連携**: PR diff / インラインコメント / Actions runs を VSCode 上で扱えるもの
  - **開発環境マネージャとの連携**: devbox + direnv ([FLM_ENG_0002](FLM_ENG_0002__devbox.md)) で管理する言語サーバ / CLI を VSCode プロセスから利用可能にするための direnv 連携拡張等。 VSCode 自体は `.envrc` を解釈しないため、 これが入らないと上記の言語サーバ系拡張が devbox 管理下のバイナリを発見できない
- 上記基準に該当しない拡張は推奨対象外とする。 特に以下は除外する:
  - **静的検査のインライン診断 (lint / formatter / 構文チェック等)**: flame の静的検査は Stop hook で AI ターン終端ごとに強制起動され ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md))、 review 時点では違反は同一ターン内で消化済みである。 残存違反も CI で fail するため PR ステータスから検知できる ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md))。 inline 表示は重複であり review 上の付加価値が無い
  - **VSCode 本体に同等機能が標準搭載されている拡張**: git history / blame / commit graph viewer、 主要言語の構文ハイライト、 Markdown プレビュー等
  - **review の判断に直接寄与しない quality-of-life 拡張**: テーマ・キーバインド・統計表示等

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (静的設定のため作成手順が単純) |
| lint | 継承 ([FLM_APP_0003](../application/FLM_APP_0003__json.md) の JSON lint) |
| build | 省略 (静的設定のため出力生成の概念がない) |
| test | 省略 (推奨は VSCode 起動時に提示されるのみで実行可能成果物を持たない) |
| ADR ルール検査 skill | 省略 (継承元の lint と本 ADR の規約でカバー) |

## 影響

- 各開発者は手元で VSCode をインストールし、 推奨拡張のインストールプロンプトを承認する操作が発生する
- レビュー時にコードの definition jump / 参照検索 / 型 hover、 GitHub PR diff / インラインコメント、 Actions runs の状況確認が VSCode 上で揃い、 PR ページ往復・別 IDE 起動の手間が減る
- VSCode 以外のエディタを編集主軸とする開発者は、 レビュー時のみ VSCode に切り替える必要がある (Cursor 等の VSCode 互換 fork は `.vscode/extensions.json` の recommendations を解釈するが、 拡張可用性は fork ごとに差異が残る)
- direnv 連携拡張を推奨に含めるため、 VSCode を任意の経路 (OS 標準ランチャ、 別 worktree への切替、 IDE 再起動等) で起動しても言語サーバ拡張が devbox 管理下のバイナリを解決できる。 `.envrc` の初回 `direnv allow` は引き続き必要 ([FLM_ENG_0002](FLM_ENG_0002__devbox.md))
- 推奨拡張は `.vscode/extensions.json` の `recommendations` 配列で管理されるため、 拡張の追加・削除がコミット差分として追跡できる
- 推奨拡張は VSCode の workspace recommendations として動作するため、 個別開発者のグローバル設定 (ユーザー設定) を上書きしない
- 静的検査拡張を推奨に含めないため、 lint 違反のフィードバック経路は引き続き Stop hook ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md)) と CI ([FLM_ENG_0003](FLM_ENG_0003__github_actions.md)) に集約される。 review 時に VSCode 上で lint 違反を独自に発見する経路は持たない
- `.vscode/extensions.json` は JSON ファイルとして [FLM_APP_0003](../application/FLM_APP_0003__json.md) の lint 対象となる
- 新しいコンテンツ種別を flame に追加する際、 当該種別に言語サーバが存在しレビュー時のコード理解を補助できる場合に限り `.vscode/extensions.json` への追記を検討する。 静的検査連携系の拡張は本 ADR の選定基準により追加対象外
- 依存側プロジェクトにも VSCode 採用が伝播する (本 ADR は ENG カテゴリのため)

## 評価

代替案として以下を検討した。

- **エディタ選定をしない (各開発者の自由)**: review workflow が個別最適化され、 PR 上の文脈把握フォーマットが揃わない。 リポジトリ単位の推奨拡張配布機構が VSCode の `.vscode/extensions.json` で軽量に取れるため、 review 用エディタを 1 つに揃える方を採用した。
- **JetBrains IDE / Zed / Helix 等を採用する**: 各エディタにも GitHub PR 連携 / 言語サーバ統合は存在するが、 (1) 推奨プラグインをリポジトリ同梱で配布する標準機構が VSCode の `.vscode/extensions.json` ほど軽量でない、 (2) flame 開発者の習熟が VSCode に集中している、 を理由に VSCode を採用した。
- **静的検査拡張 (markdownlint / shellcheck / yaml schema / errorlens 等) を推奨に含め CI と同じ違反を VSCode 上で表示する**: review 時に違反を即座に確認できる利点を意図したが、 flame の静的検査は Stop hook で AI ターン終端ごとに強制起動される ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md)) ため review 段階の差分には違反が原則残らない。 残存した場合も CI で fail として PR ステータスに表面化する。 inline 表示の review 上の付加価値が無く、 拡張の lint バージョン / ルール設定を CI 側 ([FLM_ENG_0002](FLM_ENG_0002__devbox.md) の devbox 管理下) に追従させるメンテコストだけが残るため、 推奨対象から外した。
- **GitLens / Git Graph 等の git history viewer を推奨に含める**: blame / history / commit graph は review 文脈の把握に寄与しうるが、 (1) VSCode 本体に gutter blame・Source Control view・Timeline view・組み込み diff viewer が標準搭載されており、 多くの review tasks はこれらで完結する、 (2) これらの拡張が提供する追加機能 (inline blame hover、 graph 描画等) は review に必須ではなく好みの差が大きい、 を理由に推奨対象から外し、 必要な開発者は個別にユーザーレベルでインストールする扱いとした。
- **推奨拡張を README 等のドキュメントで列挙する**: VSCode の起動時インストール提案機構が動かず、 開発者ごとに導入漏れが累積する。 `.vscode/extensions.json` で機械的に提示する方を採用した。
- **推奨拡張を各開発者のユーザー設定 (`settings.json`) で管理する**: ユーザー設定は開発者個別の global 設定であり、 リポジトリで配布する手段にならない。 workspace 推奨 (`.vscode/extensions.json`) を採用した。
- **direnv 連携拡張を推奨せず、 各開発者が direnv 適用済み shell から VSCode を起動する運用に統一する**: shell 起動経路に依存し、 worktree 切替や OS 標準ランチャ経由の起動・IDE 再起動で言語サーバが動かなくなる。 `.envrc` の解釈を VSCode プロセスに直接組み込む拡張で機械的に解決する方を採用した。

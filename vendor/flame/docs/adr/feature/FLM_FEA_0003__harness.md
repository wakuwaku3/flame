# flame harness を 3 チャネル分散 (Claude Code plugin / reusable workflow / vendor) で配布する

## 背景

- flame は AI 開発における品質保証 harness を提供する基本思想を持つ ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame の品質保証は (1) AI ターン内 hook、 (2) CI、 (3) 監視の 3 層で構成される ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- flame は AI 開発 harness として Claude Code を採用し、 `.claude/agents/` / `.claude/rules/` / `.claude/skills/` / `.claude/settings.json` を整備している ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md))
- flame は CI を GitHub Actions ワークフロー (`.github/workflows/` 配下) で整備している ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- flame は開発環境マネージャとして devbox + direnv (`devbox.json` / `devbox.lock` / `.envrc`) を採用している ([FLM_ENG_0002](../engineering/FLM_ENG_0002__devbox.md))
- flame は VSCode 拡張推奨設定を `.vscode/` 配下に配置している ([FLM_ENG_0005](../engineering/FLM_ENG_0005__vscode.md))
- flame はリポジトリ全体に作用する各種 lint 設定 (`.golangci.yaml` / `.markdownlint-cli2.yaml` / `.shellcheckrc` / `.yamllint`) を repo root に配置している
- flame は ADR を `docs/adr/` 配下のカテゴリディレクトリに整備している ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md))
- flame は最重要ルールを `CLAUDE.md` に最小化された形で配置し、 詳細は ADR を参照する形で運用している
- flame は補助処理を集約する flame CLI (= `flame` コマンド、 single binary) を持つ ([FLM_FEA_0005](FLM_FEA_0005__cli_surface.md))
- flame ツール本体 (CLI + harness) は GitHub Release 経由で配布される ([FLM_FEA_0004](FLM_FEA_0004__release_policy.md))
- 上記の harness 資産は本質的に他リポジトリで再利用可能な共有資産であり、 リポジトリ固有の運用情報ではない
- Claude Code は plugin 機構 (`.claude-plugin/plugin.json` + `agents/` / `skills/` / `hooks/` 等) を提供し、 marketplace 経由で agents / skills / hooks / commands / MCP servers を外部リポジトリから配布できる
- GitHub Actions は reusable workflow 機構 (`uses: <owner>/<repo>/.github/workflows/<file>.yaml@<ref>`) を提供し、 実体層ワークフローを外部リポジトリから参照できる
- Claude Code の `.claude/rules/` (paths frontmatter ベースの自動 context 注入機構) は plugin の正式配布 component に含まれず project-local 機構として運用される
- `.golangci.yaml` / `.shellcheckrc` / `.yamllint` / `.markdownlint-cli2.yaml` 等の lint 設定および `devbox.json` / `.envrc` 等の開発環境設定は、 外部 tool が利用側 repo root から直接読む仕様であり物理コピー以外の配布経路を持たない
- GitHub Actions のトリガー層 (`on: push` / `on: pull_request` で発火するワークフロー) は caller 側 repo で発火する必要があり、 利用側 repo に物理ファイルとして存在しなければならない
- 副ファイル overlay 機構は CLI が install 時に合成して install 先 1 ファイルに書き出す設計のため、 plugin / reusable workflow のような外部参照型配布では適用できない (利用側ツールが副ファイルを認識しない / 合成タイミングが無い)

## 決定

flame の harness 資産を **配布チャネル 3 種に分散** する。 各チャネルは異なる install 経路と integrity check 機構を持つ。

| チャネル | SoT 配置 | 配布機構 | 副ファイル overlay | integrity check |
| --- | --- | --- | --- | --- |
| A (Claude Code plugin) | `.claude-plugin/` | Claude Code marketplace + `/plugin install` | 不可 | Claude Code plugin manifest version |
| B (reusable workflow) | `.github/workflows/wf__*.yaml` | `uses: <owner>/<repo>/.github/workflows/<f>.yaml@<ref>` | 不可 | git ref pin |
| C (vendor) | `vendor/flame/` | `flame install` | 可 (`*.flame-overlay.*`) | flame.lock content snapshot |

flame 自身も dogfooding として上記 3 チャネルを利用側と同じ経路で参照する。

### チャネル A: Claude Code plugin

repo root に `.claude-plugin/` (marketplace 定義) と plugin 本体ディレクトリ `plugins/<plugin-name>/` を配置し、 Claude Code plugin として配布する。

#### 配置

- `.claude-plugin/marketplace.json`: marketplace 定義。 1 marketplace に複数 plugin を載せられるが flame は `flame` 単独で配信する。 `source: "./plugins/<plugin-name>"` で plugin 本体 dir を指す
- `plugins/<plugin-name>/.claude-plugin/plugin.json`: plugin manifest。 必須フィールド `name` (= `flame`)、 任意フィールドの `version` を flame ツール単一 version と同一採番
- `plugins/<plugin-name>/agents/`: AI レビュー subagent 定義
- `plugins/<plugin-name>/skills/`: skill 定義
- `plugins/<plugin-name>/hooks/hooks.json`: Stop hook / PreToolUse hook 設定

hook command 内で plugin 内ファイルを参照する場合は Claude Code の `${CLAUDE_PLUGIN_ROOT}` を用いる。 cwd は session working directory 側のため、 hook 実行コンテキストは利用側 repo に対して効く。

`plugins/<plugin-name>/` 配下は **source 提供元 repo にのみ存在** する (= 利用側 repo は marketplace + `/plugin install` 経由で Claude Code の plugin loader にロードされた状態を消費し、 repo tree には plugins/ ディレクトリを持たない)。 source 提供元では dogfooding 用 wrapper (例: `scripts/claude`) で当該 dir を `--plugin-dir` 引数として直接 load する経路もあわせて運用してよい (詳細: 各 source 提供元 repo の internal ADR、 例 flame self なら FLI_FEA_0003)。

#### 利用側 install

利用側は `flame install` (= flame CLI のサブコマンド) を起動すると、 plugin marketplace 登録 (`/plugin marketplace add wakuwaku3/flame` 相当) と plugin install (`/plugin install flame@flame` 相当) を CLI 内部で自動実行する (= 利用者は Claude Code セッション内で `/plugin` コマンドを手動で叩く必要は無い)。

`.claude-plugin/marketplace.json` を flame repo に併設する場合は別 marketplace 専用 repo を切らずに同居させる。

#### Version 採番

- `plugin.json.version` を flame ツール (CLI + harness + plugin) 単一 version と同期する
- 利用側は `/plugin update flame@flame` で plugin 側 version bump を取り込む

#### 利用側拡張

副ファイル overlay は plugin 仕様で表現できないため不可。 利用側拡張が必要な場合は project-local `.claude/agents/` / `.claude/skills/` に **異なる名前** で追加する (Claude Code 側の namespace 機構により plugin 側と共存する)。

#### Integrity check

plugin 側 SoT と install 状態の整合は Claude Code 側 plugin manifest version が管理するため、 `flame verify` の対象外とする。

### チャネル B: Reusable workflow

flame の `.github/workflows/` を実体層 reusable workflow の SoT とし、 vendor からは削除する。

#### 配置 (チャネル B)

- 実体層 (`wf__check.yaml` / `wf__check__diff.yaml` / `wf__deploy.yaml` / `wf__deploy_lib.yaml` / `wf__deploy_tool.yaml` / `wf__label__path.yaml`) は flame repo の `.github/workflows/` のみ
- vendor (`vendor/flame/.github/workflows/`) には実体層を含めない
- トリガー層 (`trg__*.yaml`) はチャネル C (vendor) で配布する (caller 側 repo で発火が必要なため)

#### Caller 側参照経路

利用側 caller (= 利用側 repo の `.github/workflows/trg__*.yaml`) は実体層を以下の形式で参照する。

```yaml
jobs:
  check:
    uses: wakuwaku3/flame/.github/workflows/wf__check.yaml@v1.2.14
```

`@<ref>` は flame ツールの version (git tag) を指す。 利用側は flame ツールと同一 version を tag 指定する。

#### 実体層内部の uses 連鎖

実体層から別の実体層を呼ぶ場合 (例: `wf__check.yaml` → `wf__check__diff.yaml`) も `uses: ./...` を禁止し、 absolute 参照で書く。

```yaml
# OK
uses: wakuwaku3/flame/.github/workflows/wf__check__diff.yaml@<ref>

# NG (caller-relative になり caller repo の同名 workflow を呼びにいく)
uses: ./.github/workflows/wf__check__diff.yaml
```

flame 自身が dogfooding する際は `@<ref>` を `@main` (もしくは PR ブランチ名) に揃える。

#### Caller-less な flame CLI 取得経路

実体層 reusable workflow は caller の `actions/checkout` 結果に依存しない (= caller のソースツリー前提を持たない) ように構成する。 flame CLI install のために `actions/checkout: wakuwaku3/flame` を **別 path** (`path: .flame-tool`) に追加し、 `bash .flame-tool/cli/scripts/install.sh` で install する。

```yaml
- name: Checkout flame tool
  uses: actions/checkout@<sha>
  with:
    repository: wakuwaku3/flame
    ref: <ref>
    path: .flame-tool
- name: Install flame CLI
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    bash .flame-tool/cli/scripts/install.sh
    echo "$HOME/.local/bin" >> "$GITHUB_PATH"
```

flame CLI install 自体は GitHub Release asset 取得 ([FLM_FEA_0004](FLM_FEA_0004__release_policy.md)) のため、 caller 側 repo の checkout 内容には依存しない。

#### 利用側拡張 (チャネル B)

副ファイル overlay は reusable workflow に適用できないため不可。 利用側が実体層の挙動を変えたい場合は、 (1) 実体層に `inputs:` を追加する PR を flame に出す、 (2) caller 側で実体層呼び出しの前後に独自 step を挟む、 のいずれかで対応する。

#### Integrity check (チャネル B)

reusable workflow 側 SoT と利用側参照の整合は git ref pin (`@<tag>`) が担保するため、 `flame verify` の対象外とする。

### チャネル C: vendor + overlay

`vendor/flame/` 配下を以下の対象に縮小して維持する。

| 区分 | パス | vendor 残置の理由 |
| --- | --- | --- |
| Claude Code rule | `.claude/rules/` | plugin 配布対象外 (project-local 機構) |
| ADR | `docs/adr/` | rule / skill / CLAUDE.md から相対参照される |
| Claude Code 起動文 | `CLAUDE.md` | 利用側 repo root 直下に必要 (Claude Code が無条件注入) |
| 共通 lint | `.golangci.yaml` / `.markdownlint-cli2.yaml` / `.shellcheckrc` / `.yamllint` | 外部 tool が repo root から直接読む |
| devbox | `devbox.json` (および `devbox/` 配下の補助スクリプト) | 利用側 repo root 直下に必要。 `devbox.lock` は **vendor 化しない** (devbox CLI が `devbox.json` から再生成する性質上、 利用側 repo の root 直下に各 repo 固有の lockfile が生成される。 vendor で SoT 管理する意味がない) |
| direnv | `.envrc` | 利用側 repo root 直下に必要 |
| VSCode | `.vscode/` | 利用側 repo の `.vscode/` 直下が必要 |
| GitHub Actions トリガー層 | `.github/workflows/trg__*.yaml` | caller 側 repo で発火が必要 (`on: push` 等は外部参照不可)。 install 時に `flame-` prefix 付きで scaffold 配置 (= 1 回限りの bootstrap) し、 **`flame.lock` の整合性検査対象外**。 ファイル内容は absolute uses pin (`uses: github.com/<owner>/<repo>/.github/workflows/wf__*.yaml@<tag>`) のみで version 追従は git ref pin 経由で完結する |
| GitHub Actions test 共通ヘルパ | `.github/workflows/tests/shared/` | flame self / 利用側ともに **install 先には置かず vendor SoT を直接参照する** (= test scripts が vendor 配下から source / load する経路を取る)。 GitHub Actions の発火対象ではないため install 配置は不要 |
| 機械可読 schema | `schemas/` | flame self / 利用側ともに **install 先には置かず vendor SoT を直接参照する** (= `flame.yaml` の `$schema` directive が vendor 配下を直接指す)。 IDE 補完・即時 lint 用途のため install 配置は不要 (`tests/shared/` と同じ運用)。 詳細は §schema の機械可読化 |

vendor の SoT 構造、 install path との 1:1 マッピング、 副ファイル overlay 機構 (`*.flame-overlay.*`) と形式ごとの default 合成方式、 `flame.lock` への合成結果 snapshot (full text content) による integrity check、 `flame.yaml` manifest と `flame.lock` (生成時情報) の 2 ファイル分離は本チャネル範囲のみで従来通り運用する。

#### install 先の read-only 強制

`flame install` は install copy 経路で配置したファイル (= `flame.files[].install`) と `flame-` prefix で識別される install copy 群 (`flame-` prefix の rule stub `.claude/rules/flame-*.md` および workflow scaffold `.github/workflows/flame-*.yaml`) を **install 直後に `chmod 444` (read-only)** で確定させる。 これにより利用側が install 先を直接編集する経路を OS 層で塞ぎ、 vendor SoT への合流を強制する。 利用側拡張は副ファイル overlay (`*.flame-overlay.*`) 経由でのみ行う。

git は file mode を 100644 / 100755 の 2 値しか追跡しないため、 `chmod 444` は clone 後に消える。 `flame install` の冪等再実行で再付与される運用とする。

例外として、 `flame.lock` 整合性検査の対象外である GitHub Actions トリガー層 (`.github/workflows/flame-trg__*.yaml`) は read-only 化対象だが、 利用側 repo が独自に event 追加 (例: `workflow_dispatch` の input 変更) を行いたい場合は副ファイル overlay (`flame-trg__*.flame-overlay.yaml`) で扱う (§副ファイル overlay 機構)。

利用側が install 先を編集可能にしたい場合 (= 非推奨) は `flame.yaml` の `ignore` directive に機能 ID `read-only` を宣言することで chmod 444 工程のみを skip できる (§flame.yaml manifest §機能単位 ignore)。

#### 動的マージ対象ファイル

vendor チャネルの install copy ファイル全件 (`flame.files[]`) を **動的マージ対象** として扱い、 `flame.files[].merge` を必須 field として明示記録する (= `deep` / `append` / `replace` のいずれか)。 拡張子からの推論には依存せず lockfile 単体で合成方式を確定可能とすることで、 lockfile を読む CLI / 人間 / レビュアーが追加情報なしで挙動を把握できるようにする。

- `.golangci.yaml` / `.yamllint` 系 / JSON 系 → `merge: deep` (構造化 3-way merge)
- `.shellcheckrc` / `.envrc` 等の拡張子なしテキスト → `merge: append` (line-based 3-way merge)
- バイナリ / 生成物 (該当があれば) → `merge: replace`

利用側拡張が今は無い repo (= 副ファイル overlay 不在) でも `merge` field は省略しない。 これにより:

- 利用側が後から overlay を追加しても、 `flame.lock` の差分のみで挙動変化が読み取れる
- CLI の整合性検査が `merge` strategy ごとに必要な検査経路 (3-way 再現 / append 末尾検証 / 完全一致) を分岐できる

形式ごとの default 合成方式は §副ファイル overlay 機構 の表に従う (`merge` field は当該 default を明示固定する役割を担う)。

例外として、 GitHub Actions が `.github/workflows/` 直下を発火対象として scan する制約から外れる resource (= `tests/` 配下の test scripts や shared/ 配下の共通ヘルパ) は install copy しない。 当該 resource を参照する側 (= flame self の `wf__*.sh` test scripts や flame CLI の `flame check github-actions` 等) は vendor SoT (`vendor/flame/.github/workflows/tests/shared/...`) を直接参照する経路を取る。 利用側 repository でも同様に `flame install` 時に `.github/workflows/tests/` 配下に flame harness 由来 resource は配置されず、 vendor SoT から直接参照される。

#### vendor の SoT

repo root 配下に `vendor/flame/` を配置し、 これを **チャネル C の SoT** とする。

- `vendor/flame/` 配下のディレクトリ構造は **install 先と完全に同じ** (dot prefix を含めて維持)
- 1 vendor file = 1 install path とし、 テンプレート展開や条件付きコピーは持たない
- 利用側拡張は副ファイル overlay (後述) で表現し、 vendor 側に複数バリアントを持たない
- 例外: `.github/workflows/` 配下の resource (= トリガー層 `trg__*.yaml` と対応する `tests/trg__*.sh`) は install 先で `flame-` prefix を付けたファイル名で配置する (後述 §workflow の install 命名規約)

#### workflow の install 命名規約

`vendor/flame/.github/workflows/` 配下の resource (= トリガー層 `trg__*.yaml` と対応する `tests/trg__*.sh`) は、 install 先で **`flame-` prefix を付けたファイル名** で配置する。

| 項目 | 値 |
| --- | --- |
| vendor SoT (元のファイル名) | `vendor/flame/.github/workflows/trg__push__main.yaml` |
| install 先 (flame self / 利用側 共通) | `.github/workflows/flame-trg__push__main.yaml` |

vendor 側 SoT のファイル名は変更しない (= `trg__*.yaml` のまま)。 install 時のみ rename する。

flame self / 利用側ともに同じ install 規約 (= 全 repo で `flame-trg__*.yaml` 命名)。

これは [FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) §flame self の install 先における downstream resource の stub の `.claude/rules/` における `flame-` prefix 戦略と整合する (= vendor 由来資産の識別)。 利用側 repo で独自の `trg__*.yaml` を持っても命名衝突しない。

#### 副ファイル overlay 機構

利用側が install 先ファイルを拡張する手段として **副ファイル方式** を採用する。 install 先ファイルへの直接編集は drift として整合性検査で fail させる。

##### overlay の意味論

副ファイル overlay には install 先にこうあってほしい **「最終形 (vendor base + 利用者拡張をすべて反映した結果)」** を書く。 利用者は「どこを変えたか」 ではなく「最終的にどうなっていてほしいか」 を直接記述し、 CLI が vendor (= flame の base) と overlay (= 利用者の最終形) と前回 vendor snapshot (= 共通祖先) の 3 者から **3-way merge** で install 先を合成する。

3-way merge の入力源:

| 入力 | 由来 |
| --- | --- |
| base | 前回 install 時の vendor file (`flame.files[].vendor_content` の snapshot) |
| their | 現在の vendor file |
| our | 現在の overlay 副ファイル |

3-way の合流結果を install 先に書き出す (= install 先は 3-way merge の output であり、 SoT は vendor + overlay の 2 ファイル)。

##### 副ファイルの命名

副ファイルは install 先と同じディレクトリに配置し、 以下の命名で識別する。

- 拡張子のあるファイル: `<basename-without-ext>.flame-overlay.<ext>`
  - 例: `.golangci.yaml` → `.golangci.flame-overlay.yaml`
- 拡張子のないファイル: `<basename>.flame-overlay`
  - 例: `.envrc` → `.envrc.flame-overlay`

##### 形式ごとの default 合成方式

形式ごとの default 合成方式 (`flame.yaml.files[].merge` 省略時):

| 形式 / 拡張子 | default 合成方式 |
| --- | --- |
| `.json` | 構造化 3-way merge |
| `.yaml` / `.yml` | 構造化 3-way merge |
| `.md` | line-based 3-way merge |
| `.sh` | line-based 3-way merge (shebang は overlay 側に書かない) |
| 拡張子なしテキスト (`.envrc` / `.shellcheckrc` 等) | line-based 3-way merge |
| `.lock` 等 (生成物) | replace のみ (overlay 不可) |
| その他 (バイナリ含む) | replace のみ |

`merge` フィールドによる override:

- `merge: deep` — 構造化 3-way merge を強制 (拡張子から推論できないファイルに使う)
- `merge: append` — line-based 3-way merge を強制
- `merge: replace` — overlay 不可 / vendor のみで上書き (生成物に使う)

`merge: deep` の構造化 3-way では、 配列要素の append / replace / unique 戦略は base / their / our の 3 者から **CLI が自動推論** する (= mapping は key 単位、 sequence は要素位置単位の 3-way 合流)。 利用者が戦略を明示する schema field (`merge_array` 等) は持たない。

##### conflict 発生時の挙動

3-way merge で their と our が同じ箇所を非互換に変更した場合、 CLI は overlay 副ファイルに git 形式の conflict marker (`<<<<<<<` / `=======` / `>>>>>>>`) を書き戻し、 install 先の更新は行わず、 `flame install` を exit code 1 で終了させる。

- 構造化 3-way (`merge: deep`): mapping value / sequence 要素レベルの conflict は該当箇所に YAML / JSON scalar として conflict marker を埋め込む
- line-based 3-way (`merge: append`): git RCS-merge と同等の行ベース marker を overlay に書き出す

利用者は overlay の marker を解決した上で再度 `flame install` を起動する。 marker が overlay に残ったまま `flame install` を再起動した場合、 manifest load 時の syntax check (= overlay parse) で fail させ install を中断する。

##### 利用側ツールへの透過性

副ファイル方式は、 利用側ツールが副ファイルを認識しないファイル (例: `.envrc` は direnv が単一ファイルとして実行) に対しても、 **合成は CLI が install 時に行い install 先ファイル 1 つに書き出す** という運用で対応する。 利用側ツールは合成後の install 先ファイルだけを見る。

#### flame.yaml (manifest)

repo root に `flame.yaml` を配置し、 vendor チャネルの取得元と version、 および harness が読む repo 固有設定 (= **不変情報** = 手動編集で更新するメタデータ) を記録する。 schema:

```yaml
flame:
  source: github.com/<owner>/<repo>
  version: vX.Y.Z
  ignore:                                # optional, 機能単位 skip マーカー
    - gitignore
  ai:                                    # optional, AI hook 拡張領域
    pre_push:                            # optional
      stage1_extra_agents:               # optional
        - <agent-name>
```

各フィールドの意味:

- `flame.source`: vendor の取得元リポジトリ
- `flame.version`: install 済み harness のバージョン。 flame ツール単一 version と同一採番。 通常は `vX.Y.Z` 形式の release tag を指定するが、 **harness の source 提供元 repo (= flame self) では特別マーカー値 `self` を指定する**。 `self` は「当該 repo 自身の作業ツリー (= vendor SoT のソースコード) と install 先を同期する」 意味を持ち、 release artifact からの取得経路ではなく working tree からの直接コピー経路を CLI に取らせる
- `flame.ignore` (optional): `flame install` の通常工程のうち skip するものの list。 値は **機能単位の機能 ID** (= 後述 §機能単位 ignore) で、 工程 / 配布 resource type / 採用 tool いずれかの粒度に対応する。 値 list のうち未知 ID が混入した場合は manifest load 時に error として install を中断する (= typo / 古い ID の即時検出)
- `flame.ai` (optional): AI hook の挙動を repo ごとに拡張するための設定領域。 harness 既定挙動を変えずに repo 固有 resource (例: `.claude/agents/<name>.md` 由来の subagent) を追加する用途に限る。 既定挙動の上書きは扱わず、 上書きが必要な場合は ADR を起こして既定そのものを変える
  - `flame.ai.pre_push.stage1_extra_agents` (optional, 文字列の list): `flame ai hook pre-push` 段階 1 並列レビューに repo-local な追加 reviewer agent を組み込むための agent 名 list。 各要素は当該 repo の `.claude/agents/<name>.md` の `name` frontmatter と一致する文字列とする。 harness 既定の reviewer agent list に append され、 重複は先勝ち de-dup される。 list の記述順は block reason での出現順を決める。 file 不在 / parse error / フィールド欠如はすべて soft fail (= 追加 agent なし) として扱う

`flame.yaml` は **手動編集対象** (= version bump 等を人間 / 上位 tool が行う)。 install 時の生成情報 (`files[]` / `embeds[]` 等) は持たない。

YAML を採用する ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md))。

`flame.yaml` 自体は vendor 化しない (利用側固有の取得設定記録のため)。

##### flame.yaml の探索

`flame install` は **起動時の cwd を固定の起点** として `flame.yaml` を探索する (= cwd 直下に `flame.yaml` が無ければ即時 error: "flame.yaml not found in current directory")。 上方向探索 (parent directory への walk-up) は行わない。 これは npm の `package.json` 探索と同じ慣習を採用したもので、 利用者が「どの repo に対して install しているか」 を cwd の場所で曖昧さなく判別できるようにするため。

##### 機能単位 ignore

`flame.ignore` の各要素は CLI 側で定義された機能 ID (= `Feature` 値) と一致する文字列とする。 機能 ID は以下の 3 系列から成る:

- **工程単位**: `vendor-sync` / `vendor-readonly` / `read-only` / `gitignore` / `claude/plugins` / `trigger-workflow`
- **resource type / tool 単位**: `claude/rules` / `claude/skills` / `golangci-lint` / `markdown-lint` / `shellcheck` / `devbox` / `vscode` / `adr`
- **embed 単位** (取り込み形式の snippet 注入): `embed/claude-md` / `embed/envrc` / `embed/yamllint`

各機能 ID の意味は CLI 実装側 (機能 ID 定数) と install 工程の対応関係で機械的に確定し、 ADR では網羅列挙のみを行う (= ID と install path / 工程の具体マッピングは実装詳細とし、 利用側 / レビュアーは ID 文字列で機能を識別する)。

主要機能 ID の意味の補足:

- `vendor-sync`: vendor の clone / 再 fetch 工程を skip する。 **harness の source 提供元 repo (= flame self) では vendor が working tree そのものであるため必ず宣言する**。 利用者が独自の経路で vendor 配下を管理する場合にも使う
- `vendor-readonly`: clone 後の vendor 配下を chmod 444 (file) / 555 (dir) で readonly 化する工程を skip する。 source 提供元 repo (= flame self) では vendor を編集しながら開発するため必ず宣言する
- `read-only`: install 先ファイルの chmod 444 を skip する。 install 先を直接編集可能にしたい利用者向け (= 副ファイル overlay 経由の拡張を捨てる宣言、 非推奨)
- `gitignore`: `.gitignore` への flame block 追記工程を skip する。 source 提供元 repo (= flame self) では vendor を commit するため必ず宣言する
- `claude/plugins`: Claude Code plugin marketplace 登録 + plugin install の自動工程を skip する。 source 提供元 repo (= flame self) は marketplace 兼 plugin 提供元のため必ず宣言する (具体経路は当該 repo の internal ADR で別途定める)

source 提供元 repo の特例 (旧来の暗黙的 self mode 検査) は廃止し、 self mode の挙動はすべて `flame.ignore` の明示宣言で表現する (= flame self の `flame.yaml` は `ignore: [gitignore, claude/plugins, vendor-sync, vendor-readonly]` を最低限宣言する形に揃える)。 これにより CLI は version 値 (= `self` か否か) と ignore directive の重複検査を持たず、 利用者から見ても「source 提供元 repo の特殊性が ignore directive に明文化される」 利点を得る。

##### schema の機械可読化

`flame.yaml` の schema は本 ADR §flame.yaml (manifest) を SoT (= 規範) とし、 同 schema の機械可読版を併設して IDE 補完・即時 lint 専用に供する。

- 機械可読 schema は **JSON Schema** 仕様で記述する
- 機械可読 schema 自体のシリアライズ形式は対象 manifest と同言語 (= YAML) とする ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md) §flame 独自型の schema 規約 に従う)
- 機械可読 schema は `vendor/flame/schemas/` を SoT とする。 install 経路を取らず利用側 repo / flame self の双方が当該ディレクトリを直接参照する (= `.github/workflows/tests/shared/` と同じ運用)
- `flame.yaml` 先頭に IDE 向け schema 参照 directive を付ける
- ADR の YAML 例 / フィールド semantics と機械可読 schema が乖離した場合は ADR を SoT として両者を同期する責務を ADR 改訂者が負う (= 機械可読 schema 側に独自ルールを足さない)
- `flame.ai.*` は repo ごとの hook 拡張領域として今後フィールドが増えうるため、 機械可読 schema は `flame.ai` 直下の未知 hook category を素通しさせ、 既知 hook category (`pre_push` 等) のみ厳密検査する運用とする (= ADR 改訂で新 hook category を追加する前段でも IDE が誤検出しない)
- 機械可読 schema は IDE 補完・即時 lint と `flame check yaml` lint 経路の双方で利用される。 schema 自体の正当性検査 (= 機械可読 schema として well-formed か / `flame.yaml` が当該 schema に conform するか) は `flame check yaml` 拡張 ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md) §flame 独自型の schema 規約) が機械的に強制する

#### flame.lock (生成時情報)

repo root に `flame.lock` を配置し、 vendor チャネルの install 時に **CLI が自動生成・更新する生成時情報** を記録する。 手動編集対象ではない。 schema:

```yaml
flame:
  installed:
    source: github.com/<owner>/<repo>    # manifest.flame.source と同値
    version: vX.Y.Z                      # manifest.flame.version と同値
    tree_hash: sha256:<hex>              # vendor/flame/ 配下の content hash (self mode では記録しない)
  files:
    - install: <path>                    # install 先のパス (repo root 相対)
      vendor: vendor/flame/<path>        # SoT 側のパス (repo root 相対)
      merge: deep                        # 必須 (deep / append / replace のいずれか)
      content: |                         # install 直後のファイル内容 (合成結果) を full text で snapshot
        <file content>
      vendor_content: |                  # 前回 install 時の vendor file の内容 snapshot (3-way merge の base 用)
        <vendor file content>
      overlay:                           # optional
        path: <path>.flame-overlay.<ext>
        content: |                       # 前回 install 時の overlay file の内容 snapshot (3-way merge 補助)
          <overlay file content>

  embeds:
    - install: <install_path>            # 取り込み形式の install 先 (例: CLAUDE.md / .envrc)
      target: <vendor_path>              # 取り込み元 vendor path
      snippet: <snippet>                 # install 先 file 内に出現する取り込み snippet
```

各フィールドの意味:

- `flame.installed`: 前回 install 時に解決された vendor の取得元 / version / 内容 hash の snapshot。 次回 `flame install` 起動時に manifest と比較して **version bump 検知 → vendor 再 fetch** を駆動する
  - `source`: 前回 install 時の `flame.source` 値
  - `version`: 前回 install 時の `flame.version` 値
  - `tree_hash`: 前回 install 完了時点の `vendor/flame/` 配下の content hash。 vendor/flame 配下を walk して各 file の sha256 を path 順で連結し、 全体を sha256 した値 (= git の tree hash 相当だが、 git に依存しない自前計算)。 形式は `sha256:<hex>`。 **version == "self" のときは記録しない (working tree が常時変動するため記録しても CI 検査が安定しないため)**
- `flame.files[]`: install copy 経路で配置された各ファイルのレコード (vendor チャネル limited)
  - `install`: install 先のパス (repo root 相対)
  - `vendor`: SoT 側のパス (repo root 相対)
  - `merge`: 合成方式 (`deep` / `append` / `replace`)。 全 entry で必須記録 (拡張子からの推論には依存しない、 lockfile 単体で合成方式が決定可能であるべきという考えから明示)
  - `content`: install 直後のファイル内容 (= 合成結果) を full text で snapshot として保持。 これにより:
    - 利用側が install 先を直接編集した場合の drift 検出 (現在の install 先 file vs `content` 比較)
    - ハッシュだけでは復元できない過去の合成結果を lockfile から再構築可能
  - `vendor_content`: 前回 install 時に取り込んだ vendor file の内容 snapshot。 次回 `flame install` 時の 3-way merge の **base** として利用する (= base = 前回 vendor、 their = 現在の vendor、 our = 現在の overlay)
  - `overlay` (optional): 利用側拡張の副ファイル情報
    - `path`: overlay 副ファイルのパス
    - `content`: 前回 install 時の overlay file の内容 snapshot。 3-way merge の補助に利用 (= 利用者が overlay を編集したか判定できる)

ハッシュ (sha256) は本 lockfile schema では (`installed.tree_hash` を除き) 記録しない。 `content` / `vendor_content` / `overlay.content` を full text で持つため、 必要な検査 (vendor / installed / overlay の整合) は CLI が full text 同士を直接比較して行う。 lockfile size は snapshot の分膨らむが、 3-way merge を CLI が成立させるためには base = 前回 vendor の full text が不可欠であり、 snapshot 保持を採用する。

- `flame.embeds[]`: 取り込み形式 ([FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) §repo root における downstream resource の取り込み形式) で配置された各ファイルのレコード
  - `install`: 取り込み形式の install 先 (例: `CLAUDE.md` / `.envrc`)
  - `target`: 取り込み元 vendor path (例: `vendor/flame/CLAUDE.md` / `vendor/flame/.envrc`)
  - `snippet`: install 先 file 内に出現する取り込み snippet 文字列。 vendor path 変更時に CLI が install 先 file 内の旧 snippet を新 snippet で replace する経路で利用

`flame.lock` は **git tracked** だが、 CLI (`flame install`) が自動生成・更新するため手動編集対象ではない。 利用側 repository では `flame.yaml` (= 不変情報) と `flame.lock` (= 生成時情報) の両方を repo root に持つ。

`flame.lock` 自体は vendor 化しない (利用側固有の install 状態記録のため)。

`flame.yaml` (manifest) と `flame.lock` (生成時情報) の責務分離は、 npm の `package.json` / `package-lock.json`、 cargo の `Cargo.toml` / `Cargo.lock` 等の慣習に従う。

##### vendor sync (version bump 検知)

`flame install` は起動時に `flame.lock.installed.{source, version}` と現在の `flame.yaml.flame.{source, version}` を比較し、 不一致 (= version bump / source 切り替え) を検知した場合に既存の `vendor/flame/` 配下を chmod 戻して削除し、 新 version での再 clone を実行する。 一致時は既存の vendor を再利用する (= 通常 install 時の余計な fetch を避ける)。

self mode (= `flame.version == "self"`) は対象外 (vendor が working tree そのもののため)。 `flame.ignore` に `vendor-sync` が含まれる場合は本工程全体を skip する (= 利用者が vendor を独自経路で管理する宣言)。

##### vendor の load 時 readonly

`SyncVendor` 完了直後に `vendor/flame/` 配下を walk して file は chmod 444、 dir は chmod 555 で readonly 化する。 利用者が誤って vendor を直接編集する経路を OS 層で塞ぎ、 vendor 改変は flame self 側で行うことを強制する。

self mode (= vendor が working tree) は対象外。 `flame.ignore` に `vendor-readonly` が含まれる場合は本工程を skip する。 再 install 時は CLI が一度 chmod を戻して上書き → 再度 readonly 化する。

#### Integrity check (チャネル C)

`flame.files[]` の `content` field に install 直後の合成結果を full text snapshot として記録する。 検査時は `content` (= 前回合成結果) と現状の vendor / overlay / install 先 file を直接比較して状態判定する。

判定マトリクス:

| vendor 比較 | overlay 比較 | install 先比較 | 状態 | 推奨アクション |
| --- | --- | --- | --- | --- |
| 再合成結果 == content | (or overlay なし) | install == content | 健全 | なし |
| 再合成結果 != content | — | install == content | vendor 更新あり / 未 install | `flame install` で再合成 |
| 再合成結果 != content | overlay 更新あり | install == content | overlay 更新あり / 未 install | `flame install` で再合成 |
| — | — | install != content | install 後の直接編集 (drift) | 編集を overlay または vendor へ反映 → 再 install、 または manifest から eject |
| 再合成結果 != content | overlay 更新あり | — | vendor + overlay 双方更新 | 通常運用 (install 前) |

ここで「再合成結果」 = 現状の vendor file (および overlay 副ファイルがある場合はその内容) を `merge` strategy に従って合成し直した結果を指す。

検査対象:

1. **再合成整合**: 現状の vendor (および overlay) を `merge` strategy で再合成した結果 == `flame.files[].content`
2. **install 側整合**: install 先ファイル == `flame.files[].content`
3. **vendor 孤児検出**: `vendor/flame/` 配下に `flame.files[].vendor` で参照されていないファイルが存在しないか (取り込み形式 = `flame.embeds[].target` で参照されるものは除外)
4. **参照実在性**: `flame.files[].vendor` / `overlay.path` / `install`、 および `flame.embeds[].target` / `install` で指すパスが実在するか
5. **embeds snippet 実在性**: `flame.embeds[].install` で指す install 先 file 内に `flame.embeds[].snippet` が出現するか

実行層 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)):

| 層 | 起動 | 挙動 |
| --- | --- | --- |
| ローカル (Stop hook / PreToolUse hook (`git push` 直前)) | ファイル編集後 / push 前 | 検査項目のいずれかで fail なら中断 |
| CI (`wf__check.yaml` matrix) | PR / `main` への push | 同上で fail (= PR ブロック) |

drift 検出時の挙動:

- 全ての content 不一致 / 孤児ファイル / 参照欠落を fail として扱う (warning にはしない)
- fail メッセージにはドリフト方向 (vendor 改変 / overlay 改変 / install 直接編集 / 孤児 / 欠落) と該当パスを明示
- 復旧経路は 3 通り: vendor / overlay 編集 → `flame install` で再合成 / install 先編集を overlay に反映 → `flame install` / 該当ファイルを manifest から eject

3-way merge 時に conflict が検出された場合は overlay 副ファイルに git 形式の conflict marker を書き戻し、 install 先は前回の合成結果のまま据え置き、 `flame install` 全体を exit code 1 で終了する (§副ファイル overlay 機構 §conflict 発生時の挙動)。 利用者は overlay の marker を解決して再度 `flame install` を起動する。

### dogfooding

flame 自身も上記 3 チャネルを利用側と同じ経路で参照する。

- チャネル A (plugin): flame repo の `plugins/flame/` を SoT とし、 開発者は Claude Code セッションで `/plugin marketplace add wakuwaku3/flame` → `/plugin install flame@flame` で plugin を有効化する (もしくは `--plugin-dir plugins/flame` で開発時 ad-hoc ロード)
- チャネル B (reusable WF): flame 自身の install 済 `flame-trg__*.yaml` も `uses: wakuwaku3/flame/.github/workflows/<f>.yaml@main` で外部参照する
- チャネル C (vendor): `vendor/flame/` が SoT、 install path は flame repo root の各位置。 開発者が harness を変更する際は vendor 側 (または overlay 側) を編集し、 `flame install` で repo の install path に同期する。 同期時に `.github/workflows/` 配下の vendor resource は flame self の install 先でも `flame-trg__*.yaml` で配置される (利用側と同じ install 規約)。 `flame.lock` も flame self で生成・追跡する

### vendor の git 追跡

`vendor/flame/` 配下は **デフォルトで git untracked** とする (= 利用側 repo では vendor 配下を commit しない)。 vendor の取得は `flame install` の vendor sync 経路 (= `flame.lock.installed.{source, version}` と manifest の比較に基づく自動 re-fetch) で行うため、 vendor 配下を git tracked にして PR diff に含める意義は失われた。 利用側の `.gitignore` scaffold は `vendor/*` で vendor 配下全体を ignore する形を取る。

source 提供元 repo (= flame self) は `vendor/flame/` を working tree (SoT) として commit する必要があるため、 `flame.ignore` に `gitignore` を宣言して flame 側 scaffold を skip し、 自前で `vendor/flame/` のみを git tracked にする (= flame self 側の `.gitignore` 規約は当該 repo の internal ADR で別途定める)。

### 利用側 setup 手順

利用側リポジトリで flame harness を導入する手順は **2 ステップ** に集約する:

1. **flame CLI install** — flame ツール本体の install スクリプトを curl 経由で実行
2. **`flame install` 実行** — `flame.yaml` (= 不変情報 = `source` / `version`) を repo root に手動で配置した状態で repo root を cwd として `flame install` を実行する (cwd 固定探索のため別 dir からは実行しない)。 CLI が以下を一括で実行する:
   - **チャネル A (plugin)**: Claude Code plugin marketplace の登録 (`/plugin marketplace add wakuwaku3/flame` 相当) と plugin install (`/plugin install flame@flame` 相当) の自動実行
   - **チャネル B (reusable workflow)**: install 結果の `.github/workflows/flame-trg__*.yaml` で `uses: wakuwaku3/flame/.github/workflows/<f>.yaml@<ref>` を解決させる経路の整備 (= ref 切り替えは副ファイル overlay 経由)
   - **チャネル C (vendor)**: `flame.lock.installed` と現在の manifest を比較して vendor を fetch (初回 / version bump 時) → vendor を chmod 444/555 で readonly 化 → `vendor/flame/` を install 先に同期。 `.github/workflows/` 配下の vendor resource は `flame-` prefix 付きファイル名 (例: `flame-trg__push__main.yaml`) で配置。 `flame.lock` (= 生成時情報 + embeds + installed) を生成・更新
   - **`.gitignore` scaffold**: 初回 install 時に repo root の `.gitignore` 末尾に flame block を追記する。 block 内容は以下の固定 list:

     ```text
     tmp
     .devbox
     .direnv
     .local
     .claude/.ccache
     .claude/scheduled_tasks.lock
     vendor/*
     ```

     - `tmp` / `.devbox` / `.direnv` / `.local` は典型的に commit したくない作業 dir
     - `.claude/.ccache` / `.claude/scheduled_tasks.lock` は Claude Code が生成する local state
     - `vendor/*` は `vendor/flame/` を含む vendor 配下全体を untrack 化 (= `!vendor/flame` の unignore は持たない、 vendor は `flame install` の re-fetch で再構成される前提)

利用者が個別操作するのは `flame install` の起動だけ。 plugin marketplace 登録 / plugin install / vendor 同期 / workflow 配置 / `flame.lock` 生成等は CLI 内部で完結する。

### 5 項目の整備状況 ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

「harness 配布 (3 チャネル分散 + flame.yaml manifest)」を 1 つのコンテンツ種別として整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 不要 (新規ファイルの追加は CLI のサブコマンド経由 / Claude Code plugin 標準経路で完結) |
| lint | チャネル C のみ flame.yaml / flame.lock の schema 検査 (フィールド型・必須項目・パスの存在性) + 再合成結果と `flame.files[].content` の整合性検査 + install 先と content の整合性検査 + embeds snippet の実在性検査。 `flame verify` として実装。 チャネル A / B は外部機構 (Claude Code plugin manifest / git ref pin) に委譲 |
| build | 該当なし (静的 manifest)。 ただしチャネル C の overlay 合成は build 的側面を持ち、 CLI 側で吸収する |
| test | CLI 側 service-level test として配置 (vendor / overlay / flame.yaml / flame.lock / install 先の 5 者を fixture として食わせ、 verify と install が期待通りに動くか) |
| ADR ルール検査 skill | 不要 (lint で完結) |

## 影響

- harness 資産が 3 チャネルに分散され、 vendor 規模が大幅に縮小する (agents / skills / 実体層 wf を plugin / reusable へ移管)
- 利用側拡張経路がチャネルごとに異なる: チャネル C は副ファイル overlay、 チャネル A / B は外部参照型のため利用側拡張は project-local に namespace 同居 (Claude Code) または caller 側 step 挟み込み (GitHub Actions) で対応する
- integrity check が 3 チャネルで責務分担: チャネル A は Claude Code plugin manifest version、 チャネル B は git ref pin、 チャネル C は flame verify
- flame 単一 version は plugin manifest version + git tag (reusable WF 参照) + harness vendor version の 3 箇所に同期される
- flame 自身も dogfooding として 3 チャネルを外部参照経路で使う (実装と利用側経路の二重化を避ける)
- caller workflow からの flame CLI 取得は別 path checkout 経路に統一 (caller の作業ツリーを汚染しない)
- reusable wf 内部の `uses: ./...` 禁止により、 flame 内部の実体層連鎖呼び出しも absolute 参照に変わる
- `flame.files[]` の追加・削除でチャネル C の対象ファイルの増減を表現できる。 schema 自体を変更せず対象を伸縮できる
- 合成結果 snapshot (`flame.files[].content`) を full text で持つことで、 vendor 改変 / overlay 改変 / install 後直接編集を区別して検出できる。 次回 install 時の 3-way merge 入力 (base = 前回 vendor / their = 新 vendor / our = 現在の overlay) は別途 `vendor_content` / `overlay.content` snapshot を保持して成立させる
- flame 自身も dogfooding するため、 install path を直接編集すると整合性検査 (チャネル C のみ) で fail する。 vendor 側 (または overlay 側) を編集 → `flame install` のサイクルを踏む必要がある
- `flame verify` がローカル hook と CI の両方で起動され、 vendor チャネルの drift は main マージ前に強制的に解消される
- 利用側 `.gitignore` scaffold は `vendor/*` で vendor 配下全体を ignore する形に拡張し、 `!vendor/flame` の unignore は持たない (= vendor は `flame install` の re-fetch で再構成される前提)。 利用側で Go の `go mod vendor` を採用しても `vendor/<other>/` は同様に untrack されるため干渉しない
- harness の version は flame ツール (CLI + harness + plugin) 全体のバージョンと同一採番のため、 利用側は flame の version 1 つだけ追えばよい
- `flame.yaml` (manifest) と `flame.lock` (生成時情報) を別ファイルに分離する。 利用側 / flame self ともに 2 ファイルを repo root で git tracked とし、 `flame.yaml` は手動編集 (= version bump 等)、 `flame.lock` は CLI が `flame install` 時に自動生成・更新する。 `flame.yaml` / `flame.lock` 自体は vendor 化しないため、 利用側リポジトリでは個別に追跡する
- `flame.embeds[]` で取り込み形式 ([FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) §repo root における downstream resource の取り込み形式) の install 先 (例: `CLAUDE.md` / `.envrc`) と vendor target / 取り込み snippet を記録する。 vendor path 変更時に CLI が install 先 file 内の旧 snippet を新 snippet で replace する経路を持てる
- `flame.yaml` の schema を `flame.harness.{source, version, ignore, ai}` から `flame.{source, version, ignore, ai}` に flatten した。 `flame.lock` も `flame.harness.{files, embeds}` から `flame.{installed, files, embeds}` に flatten。 中間段の `harness` ネスト撤廃により、 manifest / lock の field アクセス経路が 1 段浅くなり、 利用者が手書きする `flame.yaml` の縦の階層深さも縮む
- `flame.lock.installed.{source, version, tree_hash}` セクションを追加した。 `flame install` 起動時に CLI が前回の `installed.{source, version}` と現在の manifest を比較し、 不一致 (= version bump / source 切り替え) を検知した場合に既存 vendor を削除して再 fetch する。 これにより `flame.yaml` の version を bump するだけで vendor が自動的に新 version に追従し、 利用者が手動で `vendor/flame/` を削除する経路が不要になる
- `flame.lock.installed.tree_hash` は vendor/flame 配下の content hash (= 各 file の sha256 を path 順で連結し全体を sha256 した値、 `sha256:<hex>` 形式) で、 self mode (= `flame.version == "self"`) では記録しない (working tree が常時変動するため CI 検査が安定しないため)
- `flame.files[].vendor_content` / `flame.files[].overlay.content` snapshot field を追加した。 3-way merge の base = 前回 vendor、 their = 現在の vendor、 our = 現在の overlay、 という入力 3 者を CLI が lockfile から復元できるようにする
- `flame.ignore` directive を機能単位 (機能 ID) に変更した。 利用側は工程 / resource type / tool 単位で flame の挙動を細かく制御できる (例: 利用側で `golangci-lint` を採用しない場合は `ignore: [golangci-lint]` で `.golangci.yaml` の install を skip)。 source 提供元 repo (= flame self) の特例 (旧来の暗黙的 self mode 検査) は廃止し、 self mode に必要な skip も `ignore: [vendor-sync, vendor-readonly, gitignore, claude/plugins]` 等で明示宣言する
- 旧 ignore 値 `.gitignore` / `.claude/plugins` (= リテラルなパス文字列) は撤廃し、 新命名 `gitignore` / `claude/plugins` (= 機能 ID) に置き換えた。 既存利用側 repo の `flame.yaml` は manifest 側の rename が必要
- `flame install` の `flame.yaml` 探索は cwd 固定 (上方向 walk-up を撤廃) とした。 npm の `package.json` 探索と同じ慣習で、 利用者が「どの repo に対して install しているか」 を cwd の場所で曖昧さなく判別できる。 cwd 直下に `flame.yaml` が無い場合は即時 error: "flame.yaml not found in current directory"
- 副ファイル overlay の意味論を「拡張のみ (vendor + overlay の合成は CLI に任せる差分記述)」 から「最終形 (vendor base + 拡張をすべて反映した完成形)」 に変更した。 利用者は overlay に「install 先にこうあってほしい完成形」 を直接書け、 CLI が vendor / overlay / 前回 vendor snapshot から 3-way merge で install 先を合成する。 配列の append / replace / unique 戦略は CLI が 3-way の結果として自動推論するため、 利用者が `merge_array` を明示指定する schema field は撤廃された
- 3-way merge で their と our が同じ箇所を非互換に変更した場合、 CLI は overlay 副ファイルに git 形式の conflict marker (`<<<<<<<` / `=======` / `>>>>>>>`) を書き戻し、 install 先は更新せず、 `flame install` を exit code 1 で終了する。 構造化 (`merge: deep`) は YAML / JSON scalar への marker 埋め込み、 line-based (`merge: append`) は git RCS-merge と同等の行ベース marker。 marker が overlay に残ったまま再 install を起動した場合は manifest load 時の syntax check で fail
- vendor sync (version bump 検知) と vendor readonly (chmod 444/555) を install 工程として追加した。 vendor sync は `flame.lock.installed` と manifest の比較で再 fetch を駆動、 readonly は利用者が誤って vendor を直接編集する経路を OS 層で塞ぐ。 self mode は両工程とも対象外 (vendor が working tree のため)。 `flame.ignore` の `vendor-sync` / `vendor-readonly` で個別 skip 可能
- 利用側 `.gitignore` scaffold を `tmp` / `.devbox` / `.direnv` / `.local` / `.claude/.ccache` / `.claude/scheduled_tasks.lock` / `vendor/*` の 7 行に拡張し、 `!vendor/flame` の unignore を撤去した。 vendor は `flame install` の vendor sync 経路で再構成されるため git tracked にする必要がなくなり、 利用側 repo の PR diff から harness 内容変化が消える
- `vendor/flame/.github/workflows/` 配下の resource (= トリガー層 `trg__*.yaml` と対応する `tests/trg__*.sh`) は install 先で `flame-` prefix を付ける (例: `.github/workflows/flame-trg__push__main.yaml`)。 vendor 側 SoT のファイル名は変更しない。 利用側 repo の独自 trigger workflow との命名衝突を避け、 vendor 由来資産であることを install 先からも識別可能にする
- workflow install 命名規約は flame self / 利用側で同一 (= 全 repo で `flame-trg__*.yaml`)。 flame self の `.github/workflows/` も vendor の `trg__*.yaml` を `flame-trg__*.yaml` で install する
- 本 ADR の規約は依存側プロジェクトへも伝播する (本 ADR は FEA カテゴリのため)
- 本 ADR で予約する CLI コマンド名は `flame install` / `flame verify` の 2 つで、 具体実装は CLI 側で行う
- Claude Code plugin の正式配布 component (agents / skills / hooks / commands / MCP) に rules が含まれない仕様変更が発生した場合は、 rules も plugin チャネルへ移管できる余地がある (現時点では rules は project-local 機構として vendor 残置)
- `.github/workflows/tests/shared/assertions.sh` の `assert_uses_targets_exist` / `assert_inputs_parity` は `./` 形式の caller-relative `uses:` のみを repo-local として実在性 / inputs parity 検証する。 `uses: ./...` を禁止して absolute 参照に統一した結果、 これらの検証は現実装ではすべて skip される (= 外部 reusable workflow として静的検証不能扱い)。 同 repo を指す absolute 参照 (例: `wakuwaku3/flame/.github/workflows/<f>.yaml@<ref>`) を repo-local として再検証する経路は assertions.sh の拡張で対応する (本 ADR のスコープ外、 follow-up タスク)
- flame 開発セッションでは plugin が auto load されないため、 セッション起動時に何らかの形で plugin を有効化する経路が必要となる (flame self での具体起動経路は当該 repo の internal ADR で定める)
- `flame.yaml` の schema は ADR を SoT としつつ、 機械可読版を JSON Schema 仕様で記述したものを YAML シリアライズ形式で `vendor/flame/schemas/flame.yaml.schema.yaml` に併設する。 利用側 / flame self は `flame.yaml` 先頭の `# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml` directive 経由で IDE 補完・即時 lint を受け取れる
- 機械可読 schema は install 経路を取らず利用側 / flame self が `vendor/flame/schemas/` を直接参照する (= `tests/shared/` と同じ運用)。 install 先の物理コピーが増えない代わりに、 `flame.yaml` の `$schema` 参照は利用側 repo にも `vendor/flame/` が install 済みであることに依存する
- `flame.ai.*` は AI hook の repo 固有拡張領域として正式に予約される。 既知 hook category (`pre_push` 等) のフィールドは ADR で定義、 未知 hook category は今後の ADR 改訂で追加。 機械可読 schema は既知 hook category を厳密検査し未知 category は素通しとする
- ADR と機械可読 schema が乖離すると IDE 上の検査結果と ADR 規定が衝突する。 ADR 改訂者は §flame.yaml (manifest) のフィールド更新時に schema ファイルも同コミットで追従させる責務を負う
- `flame.yaml` ↔ schema の conform 検査は `flame check yaml` 拡張で機械化される ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md) §flame 独自型の schema 規約 / §影響)。 schema 自体の構文妥当性は yamllint / `flame check yaml` で検証されるが、 schema の semantic な正当性 (= JSON Schema 仕様への適合) を超えた meta 検査は本 ADR スコープ外
- [FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md) §4 ADR rule 追加時の静的 lint 評価義務 に従う評価ラベリング: 「IDE 補完・即時 lint」 経路は yaml-language-server / Red Hat YAML 拡張等の既存 IDE 拡張で静的化される (= 設定変更は `flame.yaml` 先頭の `# yaml-language-server: $schema=./vendor/flame/schemas/flame.yaml.schema.yaml` directive のみで、 別途 linter / カスタム静的検査の追加は不要)。 「`flame.yaml` ↔ schema の conform 検査」 経路は `flame check yaml` 拡張 ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md) §flame 独自型の schema 規約 / §影響) で機械化される。 「ADR と機械可読 schema (= フィールド semantics の同期)」 経路のみは静的化困難として AI レビュー (adr-reviewer / 当該 PR レビュアー手作業) で補完する立場を取る

## 評価

代替案として以下を検討した。

- **全 vendor 維持 (チャネル A / B を採用しない)**: SoT と install 先の二重管理が全資産で発生し、 vendor 規模が累積する。 Claude Code / GitHub Actions の標準配布機構に乗せられる資産まで vendor で持つのは利用側のメンテ負荷を増やすため、 標準配布機構を持つ資産は plugin / reusable へ移管する方を採用した
- **チャネル A (plugin) のみ採用、 reusable WF は採用しない**: workflow が依然 vendor のため flame 側の改善が利用側に伝播するまで `flame install` の手間が必要。 reusable も同時採用することで version bump で workflow が即時更新される構成を採用した
- **チャネル B (reusable WF) のみ採用、 plugin は採用しない**: agents / skills が vendor のままで、 利用側 Claude Code セッション開始毎の sync が必要。 plugin も同時採用することで Claude Code 標準の plugin install / update 経路に乗せられる構成を採用した
- **rules も plugin で配布する**: Claude Code plugin の正式な配布 component に rules は含まれず、 project-local 機構として運用される。 rules を plugin 経路で配布しても利用側 Claude Code セッションで認識されない。 rules は vendor 残置 (チャネル C) とし、 plugin との二重配布は避けた
- **lint configs (`.golangci.yaml` 等) を plugin / reusable で配布する**: 外部 tool (golangci-lint / shellcheck / yamllint) が利用側 repo root から直接読む仕様のため、 plugin / reusable で配布しても tool が見つけられない。 vendor 残置 (チャネル C) で物理コピー必須
- **GitHub Actions トリガー層 (`trg__*.yaml`) も reusable workflow にする**: トリガー層は `on: push` / `on: pull_request` で発火する必要があり、 caller 側 repo で発火しない reusable workflow にはできない。 vendor 残置 (チャネル C) で利用側 repo に物理ファイルとして install する形を維持
- **副ファイル overlay 機構を plugin / reusable にも適用する**: plugin / reusable は read-only な外部参照のため install 時に CLI が合成する余地がない (利用側ツールが副ファイル概念を持たない / 合成タイミングが無い)。 overlay 不可は外部参照型配布の本質的制約として受け入れた
- **plugin / reusable の version を flame 本体と別 semver で採番する**: チャネルごとに breaking change を独立に管理できる利点があるが、 利用側は plugin version + git tag + harness vendor version の互換マトリクスを管理する必要が生じる。 flame ツール (CLI + harness + plugin) を 1 つの version で運用し、 利用側は flame の version 1 つだけ追えばよい構成を採用した
- **flame 自身は plugin / reusable WF を local path 参照する (dogfooding 経路を二重化)**: flame 内では `--plugin-dir plugins/flame` や `uses: ./...` を使い、 利用側のみ外部参照という構成。 実装は単純だが flame 内の参照経路と利用側経路が乖離するため、 利用側で発生する不具合 (例: reusable WF の uses 連鎖が caller-relative に解決される問題) を flame 開発時に再現できない。 flame 自身も外部参照経路 (`@main` 等) で動かすことで実態と一致させる方を採用した
- **reusable wf 内部で flame CLI を caller checkout 経由で取得する**: 現行 (本 ADR 改訂前) は caller の `cli/scripts/install.sh` を呼ぶ前提。 caller 側 repo に flame source が無い場合に動作しない。 `actions/checkout: wakuwaku3/flame` を別 path に追加して install.sh を直接実行することで、 caller の作業ツリーを汚染せず flame CLI を取得する方を採用した
- **reusable wf 内部の `uses: ./...` を許容する**: 実体層 → 実体層の呼び出しを caller-relative `./...` で書くと、 caller 側 repo の同名 workflow が呼ばれる (= flame 側 reusable 連鎖が壊れる)。 absolute 参照 (`uses: <owner>/<repo>/.github/workflows/<f>.yaml@<ref>`) に統一する方を採用した
- **`flame.yaml` も vendor 化する**: vendor の SoT で repo 横断的に install 状態を共有する案。 しかし `flame.yaml` (取得元 / version) と `flame.lock` (生成時情報) はいずれも **リポジトリ固有記録** であり、 SoT 側に持つべき情報ではない。 これら 2 ファイルは各リポジトリで個別管理する方を採用した
- **manifest と lock を 1 ファイル (`flame.yaml`) に集約する**: ファイル数が増えず一見シンプル。 しかし手動編集対象 (`source` / `version`) と CLI 自動生成対象 (`files[]` / `embeds[]`) が同居すると、 人間が `version` を bump したい diff と CLI が install 結果を更新する diff が混ざり、 review しづらく conflict を起こしやすい。 npm / cargo 等の慣習通り manifest と lock を分離する方を採用した
- **`flame.lock` を gitignore (生成物として非追跡)**: lock ファイルを生成物扱いし利用側 repo で git tracked にしない案。 ただし利用側 repo の install 状態が history に残らず、 CI 上での integrity check (合成結果 snapshot との照合) も commit 履歴と切り離されるため、 vendor 改変 / overlay 改変 / install 直接編集の判定が困難になる。 npm / cargo の lock ファイルと同様に git tracked とする方を採用した
- **`.github/workflows/` 配下の vendor resource を install 先でも `trg__*.yaml` のまま (prefix なし)**: install 先のファイル名を vendor SoT と一致させる案。 利用側で独自 trigger workflow を持つと命名衝突する。 また install 先からは vendor 由来か repo 独自かが見分けられず、 利用側開発者が install 先 file を直接編集する事故 (drift) が起きやすい。 [FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) の `.claude/rules/` における flame-prefix 戦略と整合させ、 install 時に `flame-` prefix を付ける方を採用した
- **vendor 側 SoT も `flame-trg__*.yaml` 命名にする**: vendor 側と install 先で同名にする案。 vendor 側 SoT 内で他の vendor 資産と命名規約が乖離する (vendor 内では `trg__*.yaml` が自然な命名で、 vendor 自身は flame 配下のため `flame-` prefix 自体が冗長)。 vendor 側は `trg__*.yaml`、 install 時のみ `flame-` prefix を付ける形を採用した
- **install 先への直接編集を許可する**: 開発者の運用が直感的になる利点があるが、 利用側で install 後の編集が SoT との drift を生み、 次回 `flame install` で上書きされる事故が起きる。 install 先の直接編集を整合性検査で fail させ、 副ファイル overlay 経由の拡張に統一する方 (= chmod 444 read-only 強制 + overlay 経由 3-way merge) を採用した。 本決定は overlay 意味論変更 (「拡張のみ」 → 「最終形」) 後も維持する (= 3-way merge を install 先で行わず overlay で行うため、 install 先の chmod 444 と整合する)
- **副ファイル overlay を持たず、 install 先への直接編集を意図的に許容する**: vendor 更新があった場合に install 先の利用側拡張が無視される (上書きで失われる) 危険がある。 副ファイルを SoT として明示し、 3-way merge は overlay (= 最終形) 経由で CLI が行う形を採用した
- **副ファイル合成をランタイム (利用側ツール) に任せる**: 利用側ツールが直接 overlay を読む案。 ツールごとに副ファイル認識の対応が必要 (Claude Code / direnv / golangci-lint 等は仕様を変えられない) で実現性が無い。 install 時に 1 ファイルに合成して書き出す方を採用した
- **ハッシュ (sha256 等) を `flame.lock` に記録する**: 当初は vendor / overlay / installed の sha256 を記録して整合性を判定する設計だった。 ただし sha256 だけでは変更の方向 (vendor 改変 / overlay 改変 / install 直接編集) を区別する条件分岐は組めても、 過去の合成結果を復元できないため 3-way merge の入力 (base = 前回 vendor 等) が失われる。 lock の役割を「整合性検査の hash 比較台帳」 から 「過去 snapshot 群 (`content` / `vendor_content` / `overlay.content` の full text)」 に拡張する形で記録量を増やし、 sha256 は file 単位では冗長になるため取りやめた。 例外として `flame.lock.installed.tree_hash` は vendor 配下 1 ツリーの一括変化検知に使うため sha256 を記録する
- **`vendor/` をそのまま全追跡 (gitignore に何も書かない)**: 利用側で `go mod vendor` 等を導入したときに大量の vendor ファイルが追跡されるリスクがある。 `vendor/*` で vendor 配下全体を ignore する方を採用した (= vendor/flame は `flame install` の vendor sync で再構成されるため git tracked にする必要が無い)
- **`vendor/flame` のみ git tracked にして PR diff に harness 変化を残す**: 旧来は `vendor/*` で直接子を ignore してから `!vendor/flame` で flame だけ unignore する形で運用していた。 ただし vendor sync (= `flame.lock.installed` と manifest 比較に基づく自動 re-fetch) を導入した結果、 利用側 repo で vendor を git tracked にする必要が消えた (= harness の更新は flame self 側 PR の release tag bump で表現される)。 `!vendor/flame` の unignore を撤去し vendor 配下全体を untrack 化する形に倒した
- **配列単位の `merge_array` override (JSON Pointer / YAML path 単位)**: 同一ファイル内で配列ごとに append / replace / unique を切り替えたい要件 (例: `.golangci.yaml` の `linters.disable` は replace、 `linters.enable` は append) が出るが、 overlay の意味論が「最終形」 に変わった結果、 利用者は overlay に「最終的にどうあってほしい配列」 を直接書け、 戦略の自動推論は CLI が 3-way merge で行う。 path 単位の override 機構を schema に持つ必要は消えた
- **機械可読 schema を持たず ADR 内の YAML 例のみで運用する**: ADR を SoT として運用するシンプルさはあるが、 IDE 補完・即時 lint を受けられず、 `flame.yaml` を編集する開発者は ADR を都度開いてフィールド名・型を照合する必要がある。 ADR を SoT に保ったまま機械可読版 (JSON Schema) を併設し、 IDE が同 schema を解釈する経路を採用した
- **機械可読 schema として CUE / Cue lang を採用する**: より強力な制約 (型 + 値 domain + 関係制約) を表現できる利点があるが、 IDE 側の対応が JSON Schema に比べて狭く `yaml-language-server` / Red Hat YAML 拡張等の標準 IDE エコシステムから外れる。 IDE 補完を主目的とする本用途では JSON Schema (Draft 2020-12) を採用した
- **JSON Schema を `vendor/flame/` 直下に置く**: `vendor/flame/<filename>` 形式で flat に並べる案もあるが、 vendor 内には既に `CLAUDE.md` / `.envrc` / lint 設定 / `devbox.json` 等の install 対象が並んでおり、 install 経路を取らない meta-resource (schema) を識別しにくい。 `vendor/flame/schemas/` ディレクトリを切り、 install 対象 / 非対象を物理レイアウトで区別する形を採用した
- **JSON Schema を install 経路に乗せて利用側 repo root に配置する**: `vendor/flame/` 配下に依存しない参照経路 (例: repo root 直下 `flame.yaml.schema.yaml`) を作る案。 install 経路で配置する場合は `flame.files[]` の管理対象になり、 schema 改訂時に整合性検査の drift が発生する。 schema は `flame.yaml` 編集時の参照対象であり利用側拡張対象ではないため、 install 経路を持たず vendor SoT を直接参照する形 (= `tests/shared/` と同じ運用) を採用した
- **機械可読 schema を JSON シリアライズ形式 (`.json`) で記述する**: schemastore.org 等のエコシステム慣習と合致するが、 flame は YAML 設定を主軸に据えており ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md))、 schema も YAML で記述する方が自プロジェクトの SoT 言語と整合する。 yaml-language-server / Red Hat YAML 拡張は YAML シリアライズの JSON Schema を解釈できるため IDE 補完・即時 lint の機能差は無い。 YAML 化により schema 内に説明コメント (`#`) を書ける副次効果もある
- **`flame.ai.*` を持たず repo 固有拡張は別ファイル (例 `flame-extras.yaml`) に分離する**: manifest を「不変情報のみ」 に保てる利点があるが、 `flame ai hook pre-push` 等の hook 実装側で読むファイルが増え、 利用側は 2 ファイル管理になる。 `flame.yaml` を「flame harness が参照する repo 固有設定の単一 manifest」 と再定義し、 hook 拡張領域も同ファイルに同居させる方を採用した
- **`flame.ai.*` の schema を ADR で規定せず schema ファイル側だけで定義する**: 機械可読 schema が独自にフィールドを増やせる柔軟性はあるが、 ADR を SoT とする §schema の機械可読化 と矛盾する。 ADR で `flame.ai.*` を formal に規定し、 schema ファイルは ADR 規定の機械可読版に留める形を採用した
- **`flame.ai` 直下に `additionalProperties: false` を強制する**: 未知フィールドを型エラーにできる厳密さがあるが、 hook category を新設する ADR 改訂前に `flame.yaml` に新フィールドを書くと IDE 上で誤検出される。 既知 hook category は厳密検査、 未知 category は素通しの折衷を採用した

過去に採用していた決定として以下の経緯がある。

- 当初は harness 資産を repo root の install 先 (= 現状の `.claude/` / `.github/` / `docs/adr/` 等) のみで運用していた。 利用側リポジトリへの展開経路を確立する段になって SoT と install 先を分離する必要が生じ、 vendor 化を導入した
- 当初の vendor 化決定では harness 資産を全て `vendor/flame/` 配下に集約し、 副ファイル overlay 機構で利用側拡張を扱う設計だった (本 ADR 旧版)。 Claude Code の plugin 機構と GitHub Actions の reusable workflow が標準配布機構として既に存在するため、 これらに乗せられる資産は外部参照型配布に移し vendor 規模を縮小する形に改訂した。 副ファイル overlay は外部参照型配布で適用できないため、 plugin / reusable 配布対象は overlay 経路を捨てる代わりに標準配布機構に乗る方を採用した
- 当初は `flame.yaml` の `files[]` が install 状態の生成時情報 (sha256 等) も含んでいたが、 manifest (= 不変情報 = 手動編集対象) と lock (= 生成時情報 = CLI 自動生成対象) を 1 ファイルに混在させると手動編集と CLI 自動生成の責務が交差していた。 npm の `package.json` / `package-lock.json`、 cargo の `Cargo.toml` / `Cargo.lock` 等の慣習に従い、 `flame.yaml` を manifest、 `flame.lock` を生成時情報の lock ファイルとして分離した
- 当初は vendor の `.github/workflows/` 配下の install 先ファイル名を vendor 側と同じ (= `trg__*.yaml`) にしていたが、 利用側で独自 trigger workflow と命名衝突する可能性が顕在化した。 [FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) の `.claude/rules/` における flame-prefix 戦略 (= flame self の install 先で vendor 参照 stub に `flame-` prefix を付け、 vendor 由来資産を識別する) と整合させ、 install 先で `flame-` prefix を付ける形に改訂した。 vendor 側 SoT のファイル名は変更しない (= rename は install 時のみ)
- 当初は `flame.yaml` の schema を本 ADR §flame.yaml (manifest) の YAML 例 + フィールド semantics のみで運用していたが、 IDE 補完・即時 lint を提供できず開発者が `flame.yaml` を編集する都度 ADR を開いてフィールド名・型を照合する必要があった。 ADR を SoT に保ったまま機械可読版 (JSON Schema) を `vendor/flame/schemas/` 配下に併設し、 `flame.yaml` 先頭の `$schema` directive 経由で IDE が同 schema を解釈する経路に改訂した
- 当初 `flame.yaml` は `flame.harness.*` のみを規定し repo 固有の AI hook 拡張領域を持たなかったため、 `flame ai hook pre-push` 段階 1 並列レビューに repo-local な reviewer agent を組み込む手段が無かった。 当時は CLI 側の partial parse (`flame.ai.pre_push.stage1_extra_agents` を unknown field として ad-hoc 解釈) で運用しており、 ADR と CLI 実装が乖離していた。 本改訂で `flame.ai.*` を AI hook の repo 固有拡張領域として正式に予約し、 既知 hook category (`pre_push` 等) のフィールドを ADR に formal に規定する形に揃えた
- 当初は機械可読 schema を JSON シリアライズ形式 (`flame.yaml.schema.json`) で記述し schemastore.org 等のエコシステム慣習と合致させていたが、 flame は YAML を主軸に据えており [FLM_APP_0004](../application/FLM_APP_0004__yaml.md) §flame 独自型の schema 規約 で「schema 自体のシリアライズ形式は対象ファイルと同言語」 と定めたため、 機械可読 schema も YAML シリアライズ (`flame.yaml.schema.yaml`) に改訂した。 yaml-language-server / Red Hat YAML 拡張は YAML シリアライズの JSON Schema を解釈できるため IDE 補完・即時 lint の機能差は無く、 YAML 化により schema 内に説明コメント (`#`) を書ける副次効果もある
- 当初は副ファイル overlay の意味論を「拡張のみ (利用者は『追加分』 を書く、 vendor との合成 = append / deep merge / unique は CLI 任せ)」 と規定していたため、 同一ファイル内で配列ごとに append / replace / unique を切り替えたい要件が出ると `flame.yaml.files[].merge_array` (replace / append / unique の戦略指定) を schema として用意する必要があった。 利用者は「この overlay でどの戦略を使うか」 を一々宣言しなければならず、 さらに base 入力が無い 2-way merge では戦略の決定論性も担保しづらかった。 本改訂で overlay の意味論を「最終形 (vendor base + 拡張をすべて反映した完成形)」 に変え、 CLI が vendor / overlay / 前回 vendor snapshot から 3-way merge で install 先を合成する形に切り替えた。 配列の戦略 (append / replace / unique) は base / their / our の 3 者から CLI が自動推論できるようになり、 `merge_array` 戦略を schema field として持つ必要が消えた (= 利用者 ergonomics の改善)。 過去に「3-way merge を CLI が持つ責務を避けて副ファイル overlay 方式に倒した」 とする運用上の選好も同時に撤回し、 3-way merge 自体の実装責務を CLI が引き受ける形に揃えた
- 当初は `flame.yaml` の schema を `flame.harness.{source, version, ignore, ai}` のネストで規定し、 `flame.lock` も `flame.harness.{files, embeds}` で揃えていた。 `harness` 中間段は「flame の harness 領域」 を分節化する意図で導入されたが、 manifest / lock 双方で唯一の中間段として残り、 利用者が手書きする `flame.yaml` の縦の階層が無駄に深かった。 本改訂で中間段を撤廃し `flame.{source, version, ignore, ai}` / `flame.{installed, files, embeds}` に flatten した
- 当初は `flame.ignore` directive がリテラルなパス文字列 (`.gitignore` / `.claude/plugins`) のみで、 source 提供元 repo (= flame self) 特例 (vendor を commit する / plugin を repo 自身が提供する) を表明する用途に限られていた。 利用側で「特定機能 (例: golangci-lint) だけ無効化したい」 「自前の vendor 管理経路を持つので vendor sync を skip したい」 等の要件が顕在化し、 ignore の粒度が不足していた。 本改訂で ignore の値を機能 ID (= 工程 / resource type / tool 単位の識別子、 例 `vendor-sync` / `golangci-lint` / `claude/rules`) に拡張し、 旧リテラル `.gitignore` / `.claude/plugins` も新命名 `gitignore` / `claude/plugins` に置き換えた。 同時に source 提供元 repo の特例 (旧来の暗黙的 self mode 検査) を廃止し、 self mode に必要な skip は `ignore: [vendor-sync, vendor-readonly, gitignore, claude/plugins]` で明示宣言する形に揃えた (= CLI の検査経路の単純化と、 source 提供元 repo の特殊性の文書化)
- 当初は `flame install` が起動 cwd から上方向に walk-up して `flame.yaml` を探索していた。 npm の旧仕様などの慣習に倣ったものだったが、 利用者が repo の subdirectory から `flame install` を起動した場合に意図しない上位 repo の `flame.yaml` を拾う事故 (= 別 repo に install してしまう) が起きうる構造だった。 本改訂で探索を cwd 固定にし、 cwd 直下に `flame.yaml` が無ければ即時 error にする形に切り替えた (= npm install / cargo build の現代的な慣習と一致)
- 当初は `flame install` の vendor 取得 (`SyncVendor`) が「初回のみ動作 (= `vendor/flame/` が存在しなければ clone)」 する設計で、 利用者が `flame.yaml` の `version` を bump しても既存 vendor がそのまま再利用されていた。 利用者は新 version に追従するために手動で `vendor/flame/` を削除する必要があり、 hands-on を強いる経路だった。 本改訂で `flame.lock.installed.{source, version, tree_hash}` を新設し、 install 起動時に lock の `installed.{source, version}` と manifest を比較して不一致なら既存 vendor を chmod 戻して削除 → 再 clone する経路に変更した。 利用者は `flame.yaml` の version を書き換えるだけで vendor が自動的に新 version に追従する
- 当初は clone 直後の vendor 配下が writable で、 利用者が誤って `vendor/flame/` 直下を直接編集する経路 (= flame self に commit すべき変更を利用側 repo の vendor に書いてしまう事故) が開いていた。 本改訂で `SyncVendor` 完了直後に vendor 配下を walk して file は chmod 444、 dir は chmod 555 に固定し、 利用者の直接編集経路を OS 層で塞ぐ形に変更した。 self mode (vendor が working tree) は対象外、 `ignore: [vendor-readonly]` で個別に skip 可能
- 当初は利用側 `.gitignore` scaffold が flame 関連 2 行 (`vendor/*` + `!vendor/flame`) のみで、 `tmp` / `.devbox` / `.direnv` 等の典型的に commit したくない作業 dir をカバーしていなかった。 また `!vendor/flame` の unignore により vendor/flame は git tracked となり、 PR diff から harness 内容変化が見えてしまう構造だった。 本改訂で scaffold を `tmp` / `.devbox` / `.direnv` / `.local` / `.claude/.ccache` / `.claude/scheduled_tasks.lock` / `vendor/*` の 7 行に拡張し、 `!vendor/flame` を撤去して vendor 配下全体を untrack 化した (= vendor は `flame install` の vendor sync で再構成される前提に揃えた)

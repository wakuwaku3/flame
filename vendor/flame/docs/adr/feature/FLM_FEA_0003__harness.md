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

vendor の SoT 構造、 install path との 1:1 マッピング、 副ファイル overlay 機構 (`*.flame-overlay.*`) と形式ごとの default 合成方式、 `flame.lock` への合成結果 snapshot (full text content) による integrity check、 `flame.yaml` manifest と `flame.lock` (生成時情報) の 2 ファイル分離は本チャネル範囲のみで従来通り運用する。

#### install 先の read-only 強制

`flame install` は install copy 経路で配置したファイル (= `flame.lock.files[].install`) と `flame-` prefix で識別される install copy 群 (`flame-` prefix の rule stub `.claude/rules/flame-*.md` および workflow scaffold `.github/workflows/flame-*.yaml`) を **install 直後に `chmod 444` (read-only)** で確定させる。 これにより利用側が install 先を直接編集する経路を OS 層で塞ぎ、 vendor SoT への合流を強制する。 利用側拡張は副ファイル overlay (`*.flame-overlay.*`) 経由でのみ行う。

git は file mode を 100644 / 100755 の 2 値しか追跡しないため、 `chmod 444` は clone 後に消える。 `flame install` の冪等再実行で再付与される運用とする。

例外として、 `flame.lock` 整合性検査の対象外である GitHub Actions トリガー層 (`.github/workflows/flame-trg__*.yaml`) は read-only 化対象だが、 利用側 repo が独自に event 追加 (例: `workflow_dispatch` の input 変更) を行いたい場合は副ファイル overlay (`flame-trg__*.flame-overlay.yaml`) で扱う (§副ファイル overlay 機構)。

#### 動的マージ対象ファイル

vendor チャネルの install copy ファイル全件 (`flame.lock.files[]`) を **動的マージ対象** として扱い、 `flame.lock.files[].merge` を必須 field として明示記録する (= `deep` / `append` / `replace` のいずれか)。 拡張子からの推論には依存せず lockfile 単体で合成方式を確定可能とすることで、 lockfile を読む CLI / 人間 / レビュアーが追加情報なしで挙動を把握できるようにする。

- `.golangci.yaml` / `.yamllint` 系 / JSON 系 → `merge: deep` (構造化 deep merge)
- `.shellcheckrc` / `.envrc` 等の拡張子なしテキスト → `merge: append`
- バイナリ / 生成物 (該当があれば) → `merge: replace`

利用側拡張が今は無い repo (= 副ファイル overlay 不在) でも `merge` field は省略しない。 これにより:

- 利用側が後から overlay を追加しても、 `flame.lock` の差分のみで挙動変化が読み取れる
- CLI の整合性検査が `merge` strategy ごとに必要な検査経路 (deep merge 再現 / append 末尾検証 / 完全一致) を分岐できる

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

副ファイルは install 先と同じディレクトリに配置し、 以下の命名で識別する。

- 拡張子のあるファイル: `<basename-without-ext>.flame-overlay.<ext>`
  - 例: `.golangci.yaml` → `.golangci.flame-overlay.yaml`
- 拡張子のないファイル: `<basename>.flame-overlay`
  - 例: `.envrc` → `.envrc.flame-overlay`

形式ごとの default 合成方式 (`flame.yaml.files[].merge` 省略時):

| 形式 / 拡張子 | default 合成方式 | 配列扱い (構造化のみ) |
| --- | --- | --- |
| `.json` | deep merge | append (vendor 末尾に overlay を連結) |
| `.yaml` / `.yml` | deep merge | append |
| `.md` | append (空行 1 行を区切りとして連結) | — |
| `.sh` | append (副ファイル末尾追記、 shebang は overlay 側に書かない) | — |
| 拡張子なしテキスト (`.envrc` / `.shellcheckrc` 等) | append | — |
| `.lock` 等 (生成物) | replace のみ (overlay 不可) | — |
| その他 (バイナリ含む) | replace のみ | — |

`merge` フィールドによる override:

- `merge: deep` — 構造化 deep merge を強制 (拡張子から推論できないファイルに使う)
- `merge: append` — テキスト末尾追記を強制
- `merge: replace` — overlay 不可 / vendor のみで上書き (生成物に使う)
- `merge_array: append | replace | unique` — 構造化 deep merge 時の配列扱い override

副ファイル方式は、 利用側ツールが副ファイルを認識しないファイル (例: `.envrc` は direnv が単一ファイルとして実行) に対しても、 **合成は CLI が install 時に行い install 先ファイル 1 つに書き出す** という運用で対応する。 利用側ツールは合成後の install 先ファイルだけを見る。

#### flame.yaml (manifest)

repo root に `flame.yaml` を配置し、 vendor チャネルの取得元と version (= **不変情報** = 手動編集で更新するメタデータ) のみを記録する。 schema:

```yaml
flame:
  harness:
    source: github.com/<owner>/<repo>
    version: vX.Y.Z
    ignore:                              # optional, 通常 install 工程の skip マーカー
      - .gitignore
```

各フィールドの意味:

- `flame.harness.source`: vendor の取得元リポジトリ
- `flame.harness.version`: install 済み harness のバージョン。 flame ツール単一 version と同一採番。 通常は `vX.Y.Z` 形式の release tag を指定するが、 **harness の source 提供元 repo (= flame self) では特別マーカー値 `self` を指定する**。 `self` は「当該 repo 自身の作業ツリー (= vendor SoT のソースコード) と install 先を同期する」 意味を持ち、 release artifact からの取得経路ではなく working tree からの直接コピー経路を CLI に取らせる
- `flame.harness.ignore` (optional): `flame install` の通常工程のうち skip するものの list。 valid value:
  - `.gitignore` — `vendor/flame/` を `.gitignore` に登録する工程を skip する。 利用側は通常 vendor は commit せず `.gitignore` 登録するが、 例外として **harness の source 提供元 repo (= flame self) は vendor を commit する必要があるため `.gitignore` 登録を skip する** ことを宣言するマーカー
  - `.claude/plugins` — `/plugin marketplace add <source>` + `/plugin install <plugin>@<source>` (= Claude Code plugin marketplace 登録 + plugin install) の自動工程を skip する。 利用側は通常 上記経由で plugin (チャネル A) を有効化するが、 **harness の source 提供元 repo (= flame self) は `.claude-plugin/marketplace.json` および `plugins/<plugin-name>/` を repo 自身が同居して持つため (= source 提供元 repo は marketplace 兼 plugin 提供元)、 plugin の有効化経路は当該 repo 内部で別途規定する** ことを宣言するマーカー (具体経路は source 提供元 repo の internal ADR で別途定める。 例 flame self なら FLI_FEA_0003 で `scripts/claude` wrapper + `--plugin-dir plugins/flame` を採用)
  - `flame.harness.source` の git remote URL が当該 repo 自身の origin URL と一致するかでも判定可能だが、 ignore directive を明示しておくことで CLI 側の検査コードを単純化し、 また人間 reader にも本 repo の特殊性を文書化する

`flame.yaml` は **手動編集対象** (= version bump 等を人間 / 上位 tool が行う)。 install 時の生成情報 (`files[]` / `embeds[]` 等) は持たない。

YAML を採用する ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md))。

`flame.yaml` 自体は vendor 化しない (利用側固有の取得設定記録のため)。

#### flame.lock (生成時情報)

repo root に `flame.lock` を配置し、 vendor チャネルの install 時に **CLI が自動生成・更新する生成時情報** を記録する。 手動編集対象ではない。 schema:

```yaml
flame:
  harness:
    files:
      - install: <path>                  # install 先のパス (repo root 相対)
        vendor: vendor/flame/<path>  # SoT 側のパス (repo root 相対)
        merge: deep                       # 必須 (deep / append / replace のいずれか)
        content: |                        # install 直後のファイル内容 (合成結果) を full text で snapshot
          <file content>
        overlay:                          # optional
          path: <path>.flame-overlay.<ext>
        merge_array: append               # optional, 構造化 deep merge 時の配列扱い

    embeds:
      - install: <install_path>           # 取り込み形式の install 先 (例: CLAUDE.md / .envrc)
        target: <vendor_path>             # 取り込み元 vendor path
        snippet: <snippet>                # install 先 file 内に出現する取り込み snippet
```

各フィールドの意味:

- `flame.harness.files[]`: install copy 経路で配置された各ファイルのレコード (vendor チャネル limited)
  - `install`: install 先のパス (repo root 相対)
  - `vendor`: SoT 側のパス (repo root 相対)
  - `merge`: 合成方式 (`deep` / `append` / `replace`)。 全 entry で必須記録 (拡張子からの推論には依存しない、 lockfile 単体で合成方式が決定可能であるべきという考えから明示)
  - `content`: install 直後のファイル内容 (= 合成結果) を full text で snapshot として保持。 これにより:
    - 利用側が install 先を直接編集した場合の drift 検出 (現在の install 先 file vs `content` 比較)
    - 次回 install 時の 3-way merge 入力 (前回 merge 結果 / 新 vendor / 利用側 overlay) として利用
    - ハッシュだけでは復元できない過去の合成結果を lockfile から再構築可能
  - `overlay` (optional): 利用側拡張の副ファイル情報 (path のみ。 内容は副ファイル自体が SoT)
  - `merge_array` (optional): 構造化 deep merge 時の配列扱い (`append` / `replace` / `unique`)

ハッシュ (sha256) は本 lockfile schema では記録しない。 `content` を full text で持つため、 必要な検査 (vendor / installed / overlay の整合) は CLI が `content` と他経路 (vendor file / 副ファイル) を直接比較して行う。 lockfile size は full text snapshot の分膨らむが、 update 時の 3-way merge 経路を成立させるためには vendor + overlay だけでは情報不足であり (= 前回の合成結果 = 利用側拡張の歴史的痕跡が必要)、 snapshot 保持を採用する。

- `flame.harness.embeds[]`: 取り込み形式 ([FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) §repo root における downstream resource の取り込み形式) で配置された各ファイルのレコード
  - `install`: 取り込み形式の install 先 (例: `CLAUDE.md` / `.envrc`)
  - `target`: 取り込み元 vendor path (例: `vendor/flame/CLAUDE.md` / `vendor/flame/.envrc`)
  - `snippet`: install 先 file 内に出現する取り込み snippet 文字列。 vendor path 変更時に CLI が install 先 file 内の旧 snippet を新 snippet で replace する経路で利用

`flame.lock` は **git tracked** だが、 CLI (`flame install`) が自動生成・更新するため手動編集対象ではない。 利用側 repository では `flame.yaml` (= 不変情報) と `flame.lock` (= 生成時情報) の両方を repo root に持つ。

`flame.lock` 自体は vendor 化しない (利用側固有の install 状態記録のため)。

`flame.yaml` (manifest) と `flame.lock` (生成時情報) の責務分離は、 npm の `package.json` / `package-lock.json`、 cargo の `Cargo.toml` / `Cargo.lock` 等の慣習に従う。

#### Integrity check (チャネル C)

`flame.lock.files[]` の `content` field に install 直後の合成結果を full text snapshot として記録する。 検査時は `content` (= 前回合成結果) と現状の vendor / overlay / install 先 file を直接比較して状態判定する。

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

1. **再合成整合**: 現状の vendor (および overlay) を `merge` strategy で再合成した結果 == `flame.lock.files[].content`
2. **install 側整合**: install 先ファイル == `flame.lock.files[].content`
3. **vendor 孤児検出**: `vendor/flame/` 配下に `flame.lock.files[].vendor` で参照されていないファイルが存在しないか (取り込み形式 = `flame.lock.embeds[].target` で参照されるものは除外)
4. **参照実在性**: `flame.lock.files[].vendor` / `overlay.path` / `install`、 および `flame.lock.embeds[].target` / `install` で指すパスが実在するか
5. **embeds snippet 実在性**: `flame.lock.embeds[].install` で指す install 先 file 内に `flame.lock.embeds[].snippet` が出現するか

実行層 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)):

| 層 | 起動 | 挙動 |
| --- | --- | --- |
| ローカル (Stop hook / PreToolUse hook (`git push` 直前)) | ファイル編集後 / push 前 | 検査項目のいずれかで fail なら中断 |
| CI (`wf__check.yaml` matrix) | PR / `main` への push | 同上で fail (= PR ブロック) |

drift 検出時の挙動:

- 全ての content 不一致 / 孤児ファイル / 参照欠落を fail として扱う (warning にはしない)
- fail メッセージにはドリフト方向 (vendor 改変 / overlay 改変 / install 直接編集 / 孤児 / 欠落) と該当パスを明示
- 復旧経路は 3 通り: vendor / overlay 編集 → `flame install` で再合成 / install 先編集を overlay に反映 → `flame install` / 該当ファイルを manifest から eject

### dogfooding

flame 自身も上記 3 チャネルを利用側と同じ経路で参照する。

- チャネル A (plugin): flame repo の `plugins/flame/` を SoT とし、 開発者は Claude Code セッションで `/plugin marketplace add wakuwaku3/flame` → `/plugin install flame@flame` で plugin を有効化する (もしくは `--plugin-dir plugins/flame` で開発時 ad-hoc ロード)
- チャネル B (reusable WF): flame 自身の install 済 `flame-trg__*.yaml` も `uses: wakuwaku3/flame/.github/workflows/<f>.yaml@main` で外部参照する
- チャネル C (vendor): `vendor/flame/` が SoT、 install path は flame repo root の各位置。 開発者が harness を変更する際は vendor 側 (または overlay 側) を編集し、 `flame install` で repo の install path に同期する。 同期時に `.github/workflows/` 配下の vendor resource は flame self の install 先でも `flame-trg__*.yaml` で配置される (利用側と同じ install 規約)。 `flame.lock` も flame self で生成・追跡する

### vendor の git 追跡

`vendor/flame/` 配下を harness の SoT としてリポジトリ追跡対象に保ち、 `vendor/<other>/` (利用側で Go の `go mod vendor` 等が生成する vendored 依存ツリー) はデフォルトで非追跡とする。 具体的な ignore / unignore パターンの記述は `.gitignore` 本体に置く。

### 利用側 setup 手順

利用側リポジトリで flame harness を導入する手順は **2 ステップ** に集約する:

1. **flame CLI install** — flame ツール本体の install スクリプトを curl 経由で実行
2. **`flame install` 実行** — `flame.yaml` (= 不変情報 = `source` / `version`) を repo root に手動で配置した状態で `flame install` を実行する。 CLI が以下を一括で実行する:
   - **チャネル A (plugin)**: Claude Code plugin marketplace の登録 (`/plugin marketplace add wakuwaku3/flame` 相当) と plugin install (`/plugin install flame@flame` 相当) の自動実行
   - **チャネル B (reusable workflow)**: install 結果の `.github/workflows/flame-trg__*.yaml` で `uses: wakuwaku3/flame/.github/workflows/<f>.yaml@<ref>` を解決させる経路の整備 (= ref 切り替えは副ファイル overlay 経由)
   - **チャネル C (vendor)**: `vendor/flame/` を install 先に同期。 `.github/workflows/` 配下の vendor resource は `flame-` prefix 付きファイル名 (例: `flame-trg__push__main.yaml`) で配置。 `flame.lock` (= 生成時情報 + embeds) を生成・更新

利用者が個別操作するのは `flame install` の起動だけ。 plugin marketplace 登録 / plugin install / vendor 同期 / workflow 配置 / `flame.lock` 生成等は CLI 内部で完結する。

### 5 項目の整備状況 ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

「harness 配布 (3 チャネル分散 + flame.yaml manifest)」を 1 つのコンテンツ種別として整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 不要 (新規ファイルの追加は CLI のサブコマンド経由 / Claude Code plugin 標準経路で完結) |
| lint | チャネル C のみ flame.yaml / flame.lock の schema 検査 (フィールド型・必須項目・パスの存在性) + 再合成結果と `flame.lock.files[].content` の整合性検査 + install 先と content の整合性検査 + embeds snippet の実在性検査。 `flame verify` として実装。 チャネル A / B は外部機構 (Claude Code plugin manifest / git ref pin) に委譲 |
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
- `flame.lock.files[]` の追加・削除でチャネル C の対象ファイルの増減を表現できる。 schema 自体を変更せず対象を伸縮できる
- 合成結果 snapshot (`flame.lock.files[].content`) を full text で持つことで、 vendor 改変 / overlay 改変 / install 後直接編集を区別して検出でき、 さらに次回 install 時の 3-way merge 入力 (前回合成結果 / 新 vendor / overlay) として再利用できる
- flame 自身も dogfooding するため、 install path を直接編集すると整合性検査 (チャネル C のみ) で fail する。 vendor 側 (または overlay 側) を編集 → `flame install` のサイクルを踏む必要がある
- `flame verify` がローカル hook と CI の両方で起動され、 vendor チャネルの drift は main マージ前に強制的に解消される
- `vendor/*` で直接子だけを `.gitignore` してから `!vendor/flame` で例外指定するため、 利用側で Go の `go mod vendor` を採用しても `vendor/<other>/` が干渉しない
- harness の version は flame ツール (CLI + harness + plugin) 全体のバージョンと同一採番のため、 利用側は flame の version 1 つだけ追えばよい
- `flame.yaml` (manifest) と `flame.lock` (生成時情報) を別ファイルに分離する。 利用側 / flame self ともに 2 ファイルを repo root で git tracked とし、 `flame.yaml` は手動編集 (= version bump 等)、 `flame.lock` は CLI が `flame install` 時に自動生成・更新する。 `flame.yaml` / `flame.lock` 自体は vendor 化しないため、 利用側リポジトリでは個別に追跡する
- `flame.lock.embeds[]` で取り込み形式 ([FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) §repo root における downstream resource の取り込み形式) の install 先 (例: `CLAUDE.md` / `.envrc`) と vendor target / 取り込み snippet を記録する。 vendor path 変更時に CLI が install 先 file 内の旧 snippet を新 snippet で replace する経路を持てる
- `vendor/flame/.github/workflows/` 配下の resource (= トリガー層 `trg__*.yaml` と対応する `tests/trg__*.sh`) は install 先で `flame-` prefix を付ける (例: `.github/workflows/flame-trg__push__main.yaml`)。 vendor 側 SoT のファイル名は変更しない。 利用側 repo の独自 trigger workflow との命名衝突を避け、 vendor 由来資産であることを install 先からも識別可能にする
- workflow install 命名規約は flame self / 利用側で同一 (= 全 repo で `flame-trg__*.yaml`)。 flame self の `.github/workflows/` も vendor の `trg__*.yaml` を `flame-trg__*.yaml` で install する
- 本 ADR の規約は依存側プロジェクトへも伝播する (本 ADR は FEA カテゴリのため)
- 本 ADR で予約する CLI コマンド名は `flame install` / `flame verify` の 2 つで、 具体実装は CLI 側で行う
- Claude Code plugin の正式配布 component (agents / skills / hooks / commands / MCP) に rules が含まれない仕様変更が発生した場合は、 rules も plugin チャネルへ移管できる余地がある (現時点では rules は project-local 機構として vendor 残置)
- `.github/workflows/tests/shared/assertions.sh` の `assert_uses_targets_exist` / `assert_inputs_parity` は `./` 形式の caller-relative `uses:` のみを repo-local として実在性 / inputs parity 検証する。 `uses: ./...` を禁止して absolute 参照に統一した結果、 これらの検証は現実装ではすべて skip される (= 外部 reusable workflow として静的検証不能扱い)。 同 repo を指す absolute 参照 (例: `wakuwaku3/flame/.github/workflows/<f>.yaml@<ref>`) を repo-local として再検証する経路は assertions.sh の拡張で対応する (本 ADR のスコープ外、 follow-up タスク)
- flame 開発セッションでは plugin が auto load されないため、 セッション起動時に何らかの形で plugin を有効化する経路が必要となる (flame self での具体起動経路は当該 repo の internal ADR で定める)

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
- **install 先への直接編集を許可する**: 開発者の運用が直感的になる利点があるが、 利用側で install 後の編集が SoT との drift を生み、 次回 `flame install` で上書きされる事故が起きる。 install 先の直接編集を整合性検査で fail させ、 副ファイル overlay 経由の拡張に統一する方を採用した
- **副ファイル overlay を持たず、 install 先への直接編集を意図的に許容する**: vendor 更新があった場合に install 先の利用側拡張が無視される (上書きで失われる) 危険があり、 利用側拡張を 3-way merge する責務を CLI に持たせる必要が生じる。 副ファイルを SoT として明示し、 合成 = CLI 任せにする方を採用した
- **副ファイル合成をランタイム (利用側ツール) に任せる**: 利用側ツールが直接 overlay を読む案。 ツールごとに副ファイル認識の対応が必要 (Claude Code / direnv / golangci-lint 等は仕様を変えられない) で実現性が無い。 install 時に 1 ファイルに合成して書き出す方を採用した
- **ハッシュ (sha256 等) を `flame.lock` に記録する**: 当初は vendor / overlay / installed の sha256 を記録して整合性を判定する設計だった。 ただし sha256 だけでは変更の方向 (vendor 改変 / overlay 改変 / install 直接編集) を区別する条件分岐は組めても、 過去の合成結果を復元できないため次回 install 時の 3-way merge 入力 (前回合成結果) が失われる。 lock の役割を「整合性検査の hash 比較台帳」 から 「過去合成結果の snapshot」 に拡張する形で `content` field (full text) を採用し、 sha256 は冗長になるため記録を取りやめた
- **`vendor/` をそのまま全追跡 (gitignore に何も書かない)**: 利用側で `go mod vendor` 等を導入したときに大量の vendor ファイルが追跡されるリスクがある。 `vendor/*` で直接子のみを ignore してから `!vendor/flame` で flame だけ追跡に戻す方を採用した
- **`vendor/` (trailing slash 付き) で親 dir 全体を ignore してから `!vendor/flame/` で unignore**: 一見直感的だが Git は ignored 親 dir 配下を辿らないため `!vendor/flame/` が機能せず、 vendor/flame の中身が一切追跡されない。 `vendor/*` で直接子のみを ignore する形に倒した
- **配列単位の `merge_array` override (JSON Pointer / YAML path 単位)**: 同一ファイル内で配列ごとに append / replace / unique を切り替えたい要件 (例: `.golangci.yaml` の `linters.disable` は replace、 `linters.enable` は append) が出ると、 ファイル単位の `merge_array` field では不足する。 JSON Pointer / YAML path 単位の override 機構は本 ADR の schema には持たず、 要件が顕在化したら schema 拡張で対応する

過去に採用していた決定として以下の経緯がある。

- 当初は harness 資産を repo root の install 先 (= 現状の `.claude/` / `.github/` / `docs/adr/` 等) のみで運用していた。 利用側リポジトリへの展開経路を確立する段になって SoT と install 先を分離する必要が生じ、 vendor 化を導入した
- 当初の vendor 化決定では harness 資産を全て `vendor/flame/` 配下に集約し、 副ファイル overlay 機構で利用側拡張を扱う設計だった (本 ADR 旧版)。 Claude Code の plugin 機構と GitHub Actions の reusable workflow が標準配布機構として既に存在するため、 これらに乗せられる資産は外部参照型配布に移し vendor 規模を縮小する形に改訂した。 副ファイル overlay は外部参照型配布で適用できないため、 plugin / reusable 配布対象は overlay 経路を捨てる代わりに標準配布機構に乗る方を採用した
- 当初は `flame.yaml` の `files[]` が install 状態の生成時情報 (sha256 等) も含んでいたが、 manifest (= 不変情報 = 手動編集対象) と lock (= 生成時情報 = CLI 自動生成対象) を 1 ファイルに混在させると手動編集と CLI 自動生成の責務が交差していた。 npm の `package.json` / `package-lock.json`、 cargo の `Cargo.toml` / `Cargo.lock` 等の慣習に従い、 `flame.yaml` を manifest、 `flame.lock` を生成時情報の lock ファイルとして分離した
- 当初は vendor の `.github/workflows/` 配下の install 先ファイル名を vendor 側と同じ (= `trg__*.yaml`) にしていたが、 利用側で独自 trigger workflow と命名衝突する可能性が顕在化した。 [FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) の `.claude/rules/` における flame-prefix 戦略 (= flame self の install 先で vendor 参照 stub に `flame-` prefix を付け、 vendor 由来資産を識別する) と整合させ、 install 先で `flame-` prefix を付ける形に改訂した。 vendor 側 SoT のファイル名は変更しない (= rename は install 時のみ)

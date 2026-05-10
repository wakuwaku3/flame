---
name: github-actions
description: GitHub Actions ワークフロー (`.github/workflows/*.yaml`) を新規作成または更新する工程で起動する。命名規約・必須要素確認・actionlint / カスタム静的検査・act によるトリガーテストまで完了させる。
---

# github-actions skill

参照 ADR: [FLM_ENG_0003: GitHub Actions ワークフローによる CI 検査の整備](../../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md)

## 起動条件

`.github/workflows/` 配下のファイルを作成または編集する工程で必ず起動する。`trg__*.yaml` と `wf__*.yaml` のいずれを触る場合も対象。

## 手順

冒頭の [FLM_ENG_0003](../../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md) を読み (継承元の [FLM_APP_0004](../../../../vendor/flame/docs/adr/application/FLM_APP_0004__yaml.md) / [FLM_APP_0002](../../../../vendor/flame/docs/adr/application/FLM_APP_0002__shell_script.md) / [FLM_APP_0001](../../../../vendor/flame/docs/adr/application/FLM_APP_0001__document.md) を含む)、その規約を満たすよう作成 / 編集したうえで以下を実行する。

### 1. ワークフロー新規追加時は対応 test script もセットで配置する

`.github/workflows/<basename>.yaml` を追加 / 改名する場合は、 同時に `.github/workflows/tests/<basename_without_yaml>.sh` を 1 本追加する ([FLM_ENG_0003](../../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md) §test の必須化と配置)。 共通アサーションは `.github/workflows/tests/shared/assertions.sh` を source して組み立てる。

### 2. 静的検査と test を 1 起動で通す

```sh
flame check yaml .github/workflows/<file>.yaml
flame check github-actions .github/workflows/<file>.yaml
```

`flame check yaml` で yamllint が走り、 `flame check github-actions` で actionlint + ファイル名規約・キー集合制約・inputs parity 等のカスタム静的ルール + 対応 test script の実在検証および動的実行が走る。 後者は lint と test が同一 entrypoint に束ねられているため、 lint pass = 対応 test も走った が保証される。 違反は同一ターン内で fix する ([FLM_GEN_0003](../../../../vendor/flame/docs/adr/general/FLM_GEN_0003__feedback_loop.md))。

### 3. 動作確認の結果を完了報告に含める

`flame check yaml` / `flame check github-actions` の終了コードを完了報告に明示する。

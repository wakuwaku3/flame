---
name: flow-document
description: flow ドキュメント (spec / tips / report) を `docs/notes/` 配下に新規作成する工程で起動する。「spec 作って」「調査を report にまとめて」「tips にメモして」「設計仕様を書いて」「ハマりどころを記録して」「調査結果を残して」など、type 名が明示されない場合でも flow ドキュメントの作成・記録が依頼された場合は本 skill を必ず起動する。FLM_APP_0006 のディレクトリ命名・index.md scaffold・静的検査まで完了させる。
---

# flow-document skill

参照 ADR: [FLM_APP_0006: flow ドキュメントの配置・命名](../../../../vendor/flame/docs/adr/application/FLM_APP_0006__flow_document.md)

## 手順

ADR の規約に従って flow ドキュメントを作成する。本 skill 固有の procedural として以下を完了させる。

### 1. type を判定する

ユーザの依頼内容から ADR で列挙された type のうち 1 つを選ぶ。type 名が依頼で明示されていない場合は ADR の type 説明と依頼内容を照合して 1 つに決める。

### 2. 現在時刻を取得する

`date '+%Y%m%d%H%M'` を実行して ADR で要求される形式の現在時刻文字列を得る。会話履歴の日付や推測値で代替しない (実時刻と乖離するとディレクトリ名が事実と異なる)。

### 3. snake_case title を確定する

会話の文脈から ADR の title 規約に合致する文字列を決める。形式は `flame check flow-document` で検証されるため、規約から外れると後段で fail する。

### 4. ディレクトリと index.md を作成する

ADR で定めた配置にディレクトリを作り、本文は当該 type の既定骨子 (後述) で初期化する。附属物 (画像・ダンプ・補助スクリプト等) を伴う場合は同じディレクトリ配下に置く。

### 5. 静的検査を通す

`flame check flow-document <作成した index.md>` と `flame check document <作成した index.md>` を実行し、命名規約・index.md 存在・Markdown lint がすべて通ることを確認する。失敗は同一ターン内で fix する。

## type ごとの既定骨子

本 skill が用意する初期テンプレートであり、ADR で規約化された構造ではない。著者の判断でセクションの増減・改名は許容する。

### spec

```markdown
# {タイトル}

## 目的

## スコープ

## 設計

## 制約・前提

## 未解決の論点
```

### tips

```markdown
# {タイトル}

## 状況

## 発見・ハマりどころ

## 補足
```

### report

```markdown
# {タイトル}

## 目的

## 経過

## 結果

## 残課題
```

# flame の最重要ルール

- コメントは Why (なぜそうなっているか) のみ書く。 How (どう動くか / 何をしているか / コードを読めばわかること) は書かない (詳細: [FLM_APP_0010](docs/adr/application/FLM_APP_0010__code_comment.md))。
- ルール本文は ADR ([FLM_GEN_0001](docs/adr/general/FLM_GEN_0001__adr.md)) に記載する。
- flame の全資産 (ADR / rule / skill / workflow / config / source code 等) は internal (`vendor/flame/` 配下「以外」) と downstream (`vendor/flame/` 配下を SoT) のいずれかに分類する (詳細: [FLM_GEN_0007](docs/adr/general/FLM_GEN_0007__resource_classification.md))。
- `vendor/flame/` 配下は基本 readonly。 例外: 当該 repository が harness の source 提供元の場合のみ writable (詳細: [FLM_GEN_0007](docs/adr/general/FLM_GEN_0007__resource_classification.md))。
- 編集対象に応じてどの ADR を参照すべきかは `.claude/rules/` 配下のマッピングから辿る。
- ドキュメントは stock (上書きで最新化) と flow (時系列で追記) のいずれかとして扱う (詳細: [FLM_APP_0001](docs/adr/application/FLM_APP_0001__document.md))。

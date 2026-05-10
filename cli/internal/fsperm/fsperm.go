// Package fsperm は flame CLI 内で OS file/dir permission の magic number 出現を 1 か所に集約する (mnd lint 警告対策および FLM_GEN_0006 §lint 局所抑制を回避する観点)。 利用側は本 package の定数 (`File` / `Dir` / `Exec`) を介して permission を渡し、 lint suppression を caller に散らさない。
package fsperm

import "os"

// File は flame CLI が書き出す通常 file の owner-only perm (= 0o600)。 step output / state.tsv / spec / release notes / shell 起動 stub 等、 機密相当の content は本 perm を使う。
const File os.FileMode = 0o600

// Dir は flame CLI が作る directory の owner-only perm (= 0o750)。 group 読み実行は CI runner 側 build user の便宜で許容する (gosec G301 の閾値: <= 0o750)。
const Dir os.FileMode = 0o750

// Exec は test fixture や install 経路で生成する実行可能ファイルの perm (= 0o700)。 world / group の書き実行は禁止する (gosec G306)。
const Exec os.FileMode = 0o700

package adr_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wakuwaku3/flame/cli/internal/root/check/adr"
	"github.com/wakuwaku3/flame/lib/clix"
)

// service-level test (FLM_APP_0009): `flame check adr` endpoint の入力空間 (filename / category 配置 / 必須セクション / 番号連番 / template 存在 / 引数なし) を境界含めて 7 ケースで覆う。 production 経路 (cwd = repo root 起点での `docs/adr/...` 検査) を変えずに t.Chdir で repo root を擬似する。

const minimalADRBody = "## 背景\nbg\n## 決定\ndc\n## 影響\neff\n## 評価\nev\n"

// setupRepo は test 用の擬似 repo root を作成し、 cwd を切り替える。 shell 版と同じく cwd = repo root を前提にした production 経路を test でも再現するための helper。 各 sub test 起点で呼び、 t.Chdir で cleanup を testing harness に任せる。
func setupRepo(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	for _, dir := range []string{"general", "application", "engineering", "feature", "infrastructure", "specific"} {
		require.NoError(tb, os.MkdirAll(filepath.Join(root, "docs", "adr", dir), 0o750))
	}
	require.NoError(tb, os.WriteFile(filepath.Join(root, "docs", "adr", "adr_template.md"), []byte("template"), 0o600))
	tb.Chdir(root)
	return root
}

func writeADR(tb testing.TB, root, category, name, body string) string {
	tb.Helper()
	path := filepath.Join(root, "docs", "adr", category, name)
	require.NoError(tb, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// t.Chdir はプロセス global な cwd を切り替えるため Go 1.24 の testing 規約上 parallel test と併用禁止 (https://pkg.go.dev/testing#T.Chdir "must not be called concurrently with parallel tests")。 本 test は repo 全体走査 (template 存在 / 番号連番) を含む production 経路を再現する都合上、 個別の引数注入経路ではなく cwd 切り替えで擬似 repo root を与える設計を採るため、 paralleltest の 「全 t.Run に t.Parallel()」 規約はここで真の false positive となる。 グローバル無効化すると他 test 全件の parallel 漏れ検出を失うため局所抑制で対応する (FLM_GEN_0006 §局所抑制が真に避けられない場合のみ、 理由を併記して例外的に許す)。
//
//nolint:paralleltest // 上記コメント参照
func TestRun(t *testing.T) {
	t.Run("全件 valid な ADR は no-op success", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		a := writeADR(t, root, "application", "FLM_APP_0001__a.md", minimalADRBody)
		b := writeADR(t, root, "general", "FLM_GEN_0001__b.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", a, b})

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		require.NoError(t, err)
		fake.Verify(t, "", "")
	})

	t.Run("filename 規約違反は FAIL 行を出して exit code 1 を伝搬する", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		bad := filepath.Join(root, "docs", "adr", "application", "bad_name.md")
		require.NoError(t, os.WriteFile(bad, []byte(minimalADRBody), 0o600))
		writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", bad})
		expectedStderr := "FAIL: docs/adr/application/bad_name.md: filename does not match {PREFIX}_{CATEGORY}_{NUMBER}__{snake_case_title}.md\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("カテゴリ配置が不一致なら FAIL 行を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		misplaced := writeADR(t, root, "general", "FLM_APP_0002__misplaced.md", minimalADRBody)
		writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		writeADR(t, root, "general", "FLM_GEN_0001__seed.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", misplaced})
		expectedStderr := "FAIL: docs/adr/general/FLM_APP_0002__misplaced.md: category 'APP' should live in 'docs/adr/application/' (found in 'docs/adr/general/')\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("必須セクション欠落は欠落分だけ FAIL 行を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		body := "## 背景\nbg\n## 決定\ndc\n"
		incomplete := writeADR(t, root, "application", "FLM_APP_0001__incomplete.md", body)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", incomplete})
		expectedStderr := "FAIL: docs/adr/application/FLM_APP_0001__incomplete.md: missing required section: ## 影響\n" +
			"FAIL: docs/adr/application/FLM_APP_0001__incomplete.md: missing required section: ## 評価\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("番号連番違反 (0001 が無く 0002 から始まる) は FAIL 行を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		seed := writeADR(t, root, "application", "FLM_APP_0002__skip.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", seed})
		expectedStderr := "FAIL: docs/adr/application/: numbering must be dense from 0001 (expected 0001, got 0002)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("template 不在は FAIL 行を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		require.NoError(t, os.Remove(filepath.Join(root, "docs", "adr", "adr_template.md")))
		seed := writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", seed})
		const expectedStderr = "FAIL: missing template: docs/adr/adr_template.md\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("入力 path から ADR root を導出するため vendor SoT 配下の ADR でも template / numbering 検査が当該 root に対して走る", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		vendorRoot := filepath.Join(root, "vendor", "flame", "docs", "adr")
		require.NoError(t, os.MkdirAll(filepath.Join(vendorRoot, "application"), 0o750))
		// 2 root を抱えた状態で repo-local 側 template を消すと 「repo-local root の template 不在」 と
		// 「vendor root の numbering 連番違反」 を独立に検出できることを確認する (= 多 root 検査の証跡)。
		require.NoError(t, os.Remove(filepath.Join(root, "docs", "adr", "adr_template.md")))
		require.NoError(t, os.WriteFile(filepath.Join(vendorRoot, "adr_template.md"), []byte("template"), 0o600))
		repoLocal := writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		vendorADR := filepath.Join(vendorRoot, "application", "FLM_APP_0002__skip.md")
		require.NoError(t, os.WriteFile(vendorADR, []byte(minimalADRBody), 0o600))
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", repoLocal, vendorADR})
		expectedStderr := "FAIL: missing template: docs/adr/adr_template.md\n" +
			"FAIL: vendor/flame/docs/adr/application/: numbering must be dense from 0001 (expected 0001, got 0002)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("不在 path / dir 指定は file does not exist の FAIL を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		missing := filepath.Join(root, "docs", "adr", "application", "FLM_APP_0099__missing.md")
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", missing})
		expectedStderr := "FAIL: docs/adr/application/FLM_APP_0099__missing.md: file does not exist\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("filename regex 通過後 unknown category id (ZZZ) は専用 FAIL を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		// numberRe は filename regex 通過時のみ trigger するため、 unknown category 単体検査では
		// 同 dir 内の連番衝突を引き起こさないよう ZZZ category 自体には 0002 を割り当てて
		// FLM_APP_0001 と numbering 上衝突しないようにする。 不一致確認は category id のみ。
		bad := filepath.Join(root, "docs", "adr", "application", "FLM_ZZZ_0002__unknown.md")
		require.NoError(t, os.WriteFile(bad, []byte(minimalADRBody), 0o600))
		writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", bad})
		expectedStderr := "FAIL: docs/adr/application/FLM_ZZZ_0002__unknown.md: unknown category id 'ZZZ' (allowed: APP ENG FEA GEN INF SPC)\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("同一 category 内で同番号の ADR が 2 件以上ある場合 duplicate ADR numbers detected を出す", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		seedA := writeADR(t, root, "application", "FLM_APP_0001__a.md", minimalADRBody)
		writeADR(t, root, "application", "FLM_APP_0001__b.md", minimalADRBody)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", seedA})
		expectedStderr := "FAIL: docs/adr/application/: duplicate ADR numbers detected\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("単一 file で複数違反 (category 配置 + 必須セクション欠落) は FAIL 行を蓄積して同時出力する", func(t *testing.T) {
		// Arrange
		root := setupRepo(t)
		writeADR(t, root, "application", "FLM_APP_0001__seed.md", minimalADRBody)
		writeADR(t, root, "general", "FLM_GEN_0001__seed.md", minimalADRBody)
		// category 不一致 (APP filename を general dir に置く) と必須セクション欠落 (背景 / 決定 のみ) を同時に起こす。
		body := "## 背景\nbg\n## 決定\ndc\n"
		multi := writeADR(t, root, "general", "FLM_APP_0002__multi.md", body)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr", multi})
		expectedStderr := "FAIL: docs/adr/general/FLM_APP_0002__multi.md: category 'APP' should live in 'docs/adr/application/' (found in 'docs/adr/general/')\n" +
			"FAIL: docs/adr/general/FLM_APP_0002__multi.md: missing required section: ## 影響\n" +
			"FAIL: docs/adr/general/FLM_APP_0002__multi.md: missing required section: ## 評価\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 1, code)
		fake.Verify(t, "", expectedStderr)
	})

	t.Run("引数なし起動は usage を stderr に出して exit code 2 を伝搬する", func(t *testing.T) {
		// Arrange
		setupRepo(t)
		r := clix.NewRoot(clix.NewRootConfig("flame", "test"))
		r.AddCommand(adr.New())
		fake := clix.NewFakeIO(t, []string{"adr"})
		const expectedStderr = "usage: flame check adr <adr_file>...\n"

		// Act
		err := r.Run(t.Context(), fake)

		// Assert
		code, ok := clix.ExitCodeOf(err)
		require.True(t, ok)
		assert.Equal(t, 2, code)
		fake.Verify(t, "", expectedStderr)
	})
}

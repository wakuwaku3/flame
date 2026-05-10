package notes_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wakuwaku3/flame/cli/internal/release/ghapi"
	"github.com/wakuwaku3/flame/cli/internal/release/notes"
)

const compareAPIPath = "/repos/owner/name/compare/lib/v1.0.0...HEADSHA"

// service-level test (FLM_APP_0009): HasModuleChangesSincePriorTag は compare API の `.files[].filename` を当該 module dir prefix で絞り込む path ベース判定を行うため、 (a) prefix にマッチする / しない、 (b) compare API 失敗、 (c) compare REST truncation 兆候 (commits 不足 / files 上限張り付き) の主要 4 経路を覆う。
//
//nolint:paralleltest // ghapi.Stub は test 内 closure に閉じるため parallel 可だが、 他 case との fixture 干渉を避ける目的で direct subtest 形式に揃える
func TestHasModuleChangesSincePriorTag(t *testing.T) {
	t.Run("module dir 配下の file change がある場合は true", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		gh.APIResponses[compareAPIPath] = []byte(`{"total_commits":1,"commits":[{"sha":"a"}],"files":[{"filename":"lib/clix/io.go"}]}`)
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "lib")

		// Assert
		assert.True(t, got)
		assert.Empty(t, warn.String(), "正常経路では warn 出力なし")
	})

	t.Run("module dir 配下の file change が無い場合は false", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		gh.APIResponses[compareAPIPath] = []byte(`{"total_commits":1,"commits":[{"sha":"a"}],"files":[{"filename":"docs/adr/general/x.md"}]}`)
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "lib")

		// Assert
		assert.False(t, got)
		assert.Empty(t, warn.String())
	})

	t.Run("compare API 失敗時は安全側で true を返し warn を残す", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		gh.APIErrors[compareAPIPath] = assert.AnError
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "lib")

		// Assert
		assert.True(t, got)
		assert.Contains(t, warn.String(), "warn: failed to fetch compare")
		assert.Contains(t, warn.String(), "assuming module changes are present")
	})

	t.Run("commits truncation 兆候 (returned < total) なら安全側で true", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		gh.APIResponses[compareAPIPath] = []byte(`{"total_commits":300,"commits":[{"sha":"a"}],"files":[{"filename":"docs/adr/general/x.md"}]}`)
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "lib")

		// Assert
		assert.True(t, got)
		assert.Contains(t, warn.String(), "compare REST may be truncated")
	})

	t.Run("priorTag 空文字は API を呼ばずに safe-side true で即返", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "", "HEADSHA", "lib")

		// Assert
		assert.True(t, got)
		assert.Empty(t, gh.APICalls, "priorTag 空なら compare API を呼ばない")
		assert.Empty(t, warn.String())
	})

	t.Run("modulePath 空文字は API を呼ばずに safe-side true で即返", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "")

		// Assert
		assert.True(t, got)
		assert.Empty(t, gh.APICalls)
		assert.Empty(t, warn.String())
	})

	t.Run("compare response の JSON parse 失敗は安全側で true を返し warn を残す", func(t *testing.T) {
		// Arrange
		gh := ghapi.NewStub(t)
		gh.APIResponses[compareAPIPath] = []byte(`{not-json`)
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "lib")

		// Assert
		assert.True(t, got)
		assert.Contains(t, warn.String(), "warn: failed to parse compare response")
		assert.Contains(t, warn.String(), "assuming module changes are present")
	})

	t.Run("files truncation 兆候 (300 件張り付き) なら安全側で true", func(t *testing.T) {
		// Arrange
		const fileCount = 300
		var sb strings.Builder
		sb.WriteString(`{"total_commits":1,"commits":[{"sha":"a"}],"files":[`)
		for i := range fileCount {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`{"filename":"docs/adr/general/x.md"}`)
		}
		sb.WriteString(`]}`)
		gh := ghapi.NewStub(t)
		gh.APIResponses[compareAPIPath] = []byte(sb.String())
		var warn bytes.Buffer

		// Act
		got := notes.HasModuleChangesSincePriorTag(context.Background(), &warn, gh, "owner/name", "lib/v1.0.0", "HEADSHA", "lib")

		// Assert
		assert.True(t, got)
		assert.Contains(t, warn.String(), "compare REST may be truncated")
	})
}

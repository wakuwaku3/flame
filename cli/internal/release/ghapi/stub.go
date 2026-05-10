package ghapi

import (
	"context"
	"os"
	"testing"

	"github.com/wakuwaku3/flame/cli/internal/fsperm"
	"github.com/wakuwaku3/flame/lib/ex"
)

// Stub は test 用 in-memory fake (FLM_APP_0009 §mock を採用しない / fake を採用する)。 test 内で API path → JSON response の対応表を組み、 想定外 path は error を返して欠落 fixture を test fail として可視化する。 APICalls / DownloadCalls は呼び出し履歴を記録する観測 channel で、 test の Assert で参照する。
type Stub struct {
	APIResponses   map[string][]byte
	DownloadAssets map[string][]byte
	APIErrors      map[string]error
	DownloadErrors map[string]error
	APICalls       []string
	DownloadCalls  []string
}

var _ Client = (*Stub)(nil)

// NewStub は第一引数 tb で production code 経路から呼べないことを compile-time に保証する (FLM_APP_0009 §test helper signature)。 APICalls / DownloadCalls は append で履歴を積むため nil 初期化のままで安全。
func NewStub(tb testing.TB) *Stub {
	tb.Helper()
	return &Stub{
		APIResponses:   make(map[string][]byte),
		DownloadAssets: make(map[string][]byte),
		APIErrors:      make(map[string]error),
		DownloadErrors: make(map[string]error),
		APICalls:       nil,
		DownloadCalls:  nil,
	}
}

func (s *Stub) API(_ context.Context, path string) ([]byte, error) {
	s.APICalls = append(s.APICalls, path)
	if err, ok := s.APIErrors[path]; ok {
		return nil, err
	}
	if data, ok := s.APIResponses[path]; ok {
		return data, nil
	}
	return nil, ex.Errorf("ghapi.Stub: no fixture for API path %s", path)
}

func (s *Stub) ReleaseDownload(_ context.Context, tag, _, outPath string) error {
	s.DownloadCalls = append(s.DownloadCalls, tag)
	if err, ok := s.DownloadErrors[tag]; ok {
		return err
	}
	data, ok := s.DownloadAssets[tag]
	if !ok {
		return ex.Errorf("ghapi.Stub: no fixture for release tag %s", tag)
	}
	if err := os.WriteFile(outPath, data, fsperm.File); err != nil {
		return ex.Wrap(err)
	}
	return nil
}

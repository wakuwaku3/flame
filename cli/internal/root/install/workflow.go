package install

import (
	"fmt"
	"regexp"
	"strings"
)

// pinUsesRef は flame-trg__*.yaml 内の `uses: <source>/.github/workflows/<f>.yaml@<ref>` を指定 ref で書き換える (FLM_FEA_0003 §チャネル B)。 vendor 側 SoT は `@main` だが、 利用側 repo に install する際は flame.yaml の version (= release tag) で pin する。 flame self は dogfooding として `@main` のまま。
func pinUsesRef(content []byte, source, version string) ([]byte, error) {
	owner, repo, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%s/%s/.github/workflows/", owner, repo)
	ref := pinRefValue(version)
	pattern := regexp.MustCompile(`(uses:\s*` + regexp.QuoteMeta(prefix) + `[A-Za-z0-9._/-]+\.yaml)@[A-Za-z0-9._/-]+`)
	replacement := []byte("$1@" + ref)
	return pattern.ReplaceAll(content, replacement), nil
}

func pinRefValue(version string) string {
	if version == SelfVersion {
		return "main"
	}
	return strings.TrimSpace(version)
}

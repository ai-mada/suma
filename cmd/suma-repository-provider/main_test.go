package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRepositoryProviderCLIHelpAndLoopbackBoundary(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		var out, err bytes.Buffer
		code := run([]string{"--language", language, "--help"}, &out, &err, func(string) (string, bool) { return "", false })
		if code != 0 || !strings.Contains(out.String(), "--config") || !strings.Contains(out.String(), "--state") || !strings.Contains(out.String(), "--addr") {
			t.Fatal(language, code, out.String(), err.String())
		}
	}
}

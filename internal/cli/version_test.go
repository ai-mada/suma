package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCLIVersionReadOnly(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", "not-a-control-origin", "--language", locale, "--version"}, strings.NewReader(""), &out, &stderr)
		if code != 0 || out.String() != "suma 1.0.0\n" || stderr.Len() != 0 {
			t.Fatalf("read-only release version: code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
		}
	}
}

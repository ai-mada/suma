package cli

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
)

const modelOptionsUsage = "models options requires --tool ID --expected-tool-version N"

func modelExecutionCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "options" {
		return usage(modelOptionsUsage)
	}
	fs := commandFlags("models options")
	tool := fs.String("tool", "", "current approved tool profile")
	version := fs.String("expected-tool-version", "", "exact current tool record version")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || duplicateModelOptions(args[1:]) || !compositionCLIIdentifier(*tool, 64) {
		return usage(modelOptionsUsage)
	}
	n, err := strconv.Atoi(*version)
	if err != nil || n < 1 || n > 2147483646 || strconv.Itoa(n) != *version {
		return usage(modelOptionsUsage)
	}
	q := url.Values{"tool_profile_id": {*tool}, "expected_tool_version": {*version}}
	return contextRead(ctx, c, "/api/v1/model-execution/options?"+q.Encode(), out)
}

func duplicateModelOptions(args []string) bool {
	seen := map[string]bool{}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		key := strings.SplitN(strings.TrimLeft(arg, "-"), "=", 2)[0]
		if key != "tool" && key != "expected-tool-version" {
			continue
		}
		if seen[key] {
			return true
		}
		seen[key] = true
	}
	return false
}

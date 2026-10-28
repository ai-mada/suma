package cli

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const workspaceTreeCLIUsage = "workspace files accepts --directory RELATIVE, --limit 1..200 and --before CURSOR"

// Fetch one bounded page only. The server owns current authority and cursor
// consistency; the CLI never walks the host filesystem or auto-retries pages.
func workspaceTreeCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("workspace files")
	directory := flags.String("directory", "", "workspace-relative directory; empty for root")
	limit := flags.Int("limit", 100, "entries per page, 1..200")
	before := flags.String("before", "", "next_cursor from the preceding page")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *limit < 1 || *limit > 200 || len(*directory) > 1024 || *directory != "" && !workspaceGitCLIPath(*directory) || len(*before) > 1024 || !utf8.ValidString(*before) || strings.IndexFunc(*before, unicode.IsControl) >= 0 {
		return usage(workspaceTreeCLIUsage)
	}
	q := url.Values{"directory": {*directory}, "limit": {strconv.Itoa(*limit)}}
	if *before != "" {
		q.Set("before", *before)
	}
	return contextRead(ctx, c, "/api/v1/workspace/tree?"+q.Encode(), out)
}

package cli

import (
	"context"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const workspaceGitCLIUsage = "workspace requires git or diff --path RELATIVE [--source worktree|index]"

// This command intentionally exposes only two project-scoped reads. Editing,
// credentials and arbitrary Git operations remain separate authorized actions.
func workspaceGitCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && args[0] == "git" {
		return contextRead(ctx, c, "/api/v1/workspace/git", out)
	}
	if len(args) == 0 || args[0] != "diff" {
		return usage(workspaceGitCLIUsage)
	}
	flags := commandFlags("workspace diff")
	file := flags.String("path", "", "project-relative file")
	source := flags.String("source", "worktree", "worktree or index")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !workspaceGitCLIPath(*file) || *source != "worktree" && *source != "index" {
		return usage(workspaceGitCLIUsage)
	}
	q := url.Values{"path": {*file}, "source": {*source}}
	return contextRead(ctx, c, "/api/v1/workspace/git/diff?"+q.Encode(), out)
}

func workspaceGitCLIPath(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\:") || strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(strings.ToLower(value), "/") {
		if part == ".." || part == "." || part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		switch part {
		case ".git", ".suma", ".cache", "node_modules", ".aws", ".ssh", ".codex", ".agents", ".kube", ".azure", ".config", ".docker", ".npmrc", ".pypirc", ".netrc", ".git-credentials", "credentials", "credentials.json", "secrets", "secrets.json", "id_rsa", "id_ed25519":
			return false
		}
		if strings.HasPrefix(part, ".env") || strings.HasPrefix(part, ".suma-save-") {
			return false
		}
		for _, suffix := range []string{".pem", ".key", ".p12", ".pfx"} {
			if strings.HasSuffix(part, suffix) {
				return false
			}
		}
	}
	return true
}

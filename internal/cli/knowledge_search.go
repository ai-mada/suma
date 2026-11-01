package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const knowledgeSearchUsage = "knowledge search accepts --query up to 200 UTF-8 bytes, --kind memory|spec|skill, --limit 1..20, the original --cursor up to 2048 bytes and --format json|table; use knowledge --help"
const knowledgeSearchResponseInvalid = "knowledge discovery must preserve current-project publication references and bounded scan continuation"

var knowledgeSearchID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Search is a current-authority read of eligible publication revisions. It is
// separate from the existing draft/metadata list and never adopts or dispatches.
func knowledgeSearchCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		_, err := io.WriteString(out, projectContextHelp(c.locale))
		return err
	}
	flags := commandFlags("knowledge search")
	query := flags.String("query", "", "optional keyword text; at most 200 UTF-8 bytes")
	kind := flags.String("kind", "", "memory, spec or skill")
	limit := flags.Int("limit", 10, "maximum hits from one bounded scan: 1..20")
	cursor := flags.String("cursor", "", "original next_cursor; at most 2048 UTF-8 bytes")
	format := flags.String("format", "json", "json or table")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !knowledgeSearchTextValid(*query, 200) || !knowledgeSearchTextValid(*cursor, 2048) || !knowledgeSearchKind(*kind, true) || *limit < 1 || *limit > 20 || *format != "json" && *format != "table" {
		return usage(knowledgeSearchUsage)
	}
	values := url.Values{"limit": {strconv.Itoa(*limit)}}
	for key, value := range map[string]string{"q": *query, "kind": *kind, "cursor": *cursor} {
		if value != "" {
			values.Set(key, value)
		}
	}
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/project-knowledge/discovery?"+values.Encode(), nil, true)
	if err != nil {
		return err
	}
	var result platform.ProjectKnowledgeDiscovery
	// The shared decoder requires every non-optional field recursively (including
	// scanned and an explicitly false exhausted); it never fills missing facts.
	if strictjson.Decode(raw, &result, 48<<10) != nil || !validKnowledgeDiscovery(result, c.project, *query, *kind, *limit) {
		return &failure{Code: "response", Message: knowledgeSearchResponseInvalid}
	}
	if *format == "json" {
		return writeJSON(out, raw, c.token)
	}
	return writeKnowledgeDiscoveryTable(out, result, c.locale, c.token)
}

func knowledgeSearchTextValid(value string, maximum int) bool {
	return len(value) <= maximum && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}
func knowledgeSearchKind(kind string, empty bool) bool {
	return kind == "memory" || kind == "spec" || kind == "skill" || empty && kind == ""
}
func knowledgeSearchPublishedText(value string, maximum int) bool {
	return utf8.ValidString(value) && len(value) <= maximum && !strings.ContainsRune(value, 0)
}

func validKnowledgeDiscovery(result platform.ProjectKnowledgeDiscovery, project, query, kind string, limit int) bool {
	if result.Items == nil || len(result.Items) > limit || result.Mode != "keyword" || result.Scope != "current_project_publications" || result.ScanLimit != 200 || result.Scanned < 0 || result.Scanned > 200 || len(result.Items) > result.Scanned || !knowledgeSearchTextValid(result.NextCursor, 2048) || result.Exhausted != (result.NextCursor == "") {
		return false
	}
	seen := map[string]bool{}
	for _, hit := range result.Items {
		ref := hit.Reference
		if !knowledgeSearchID.MatchString(ref.ID) || seen[ref.ID] || ref.Revision < 1 || ref.Revision > 2147483647 || !tc.ValidDigest(ref.SHA256) || hit.ProjectID != project || !knowledgeSearchKind(hit.Kind, false) || kind != "" && hit.Kind != kind || !knowledgeSearchPublishedText(hit.Title, 200) || strings.TrimSpace(hit.Title) == "" || !knowledgeSearchPublishedText(hit.Summary, 1000) || !knowledgeSearchPublishedText(hit.Snippet, 600) || hit.SourceCount < 0 || hit.SourceCount > 8 {
			return false
		}
		seen[ref.ID] = true
		if strings.TrimSpace(query) == "" {
			if hit.MatchedField != "none" {
				return false
			}
		} else if hit.MatchedField != "title" && hit.MatchedField != "summary" && hit.MatchedField != "content" {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, hit.PublishedAt); err != nil {
			return false
		}
	}
	return true
}

func writeKnowledgeDiscoveryTable(out io.Writer, result platform.ProjectKnowledgeDiscovery, locale localization.Locale, token string) error {
	text := func(en, zh string) string {
		if locale == localization.EnUS {
			return en
		}
		return zh
	}
	quoted := func(value string) string {
		if token != "" {
			value = strings.ReplaceAll(value, token, "[redacted]")
		}
		return strconv.Quote(value)
	}
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, text("Published knowledge · keyword discovery", "已发布知识 · 关键词发现"))
	rows := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
	fmt.Fprintf(rows, "%s\t%s\t%s\n", text("Type", "类型"), text("Title", "标题"), text("Revision", "修订"))
	for _, hit := range result.Items {
		kind := hit.Kind
		if locale != localization.EnUS {
			kind = map[string]string{"memory": "记忆", "spec": "规范", "skill": "技能"}[hit.Kind]
		}
		fmt.Fprintf(rows, "%s\t%s\t%d\n", kind, quoted(hit.Title), hit.Reference.Revision)
	}
	if err := rows.Flush(); err != nil {
		return err
	}
	for _, hit := range result.Items {
		ref, err := json.Marshal(hit.Reference)
		if err != nil {
			return err
		}
		fmt.Fprintf(&buffer, "\n%s: %s\n%s: %s\n", text("Exact reference", "准确引用"), ref, text("Excerpt", "命中片段"), quoted(hit.Snippet))
	}
	fmt.Fprintf(&buffer, "\n%s: %d/%d · %s: %d\n", text("Candidates scanned this page", "本页扫描候选"), result.Scanned, result.ScanLimit, text("Matches", "命中"), len(result.Items))
	if result.Exhausted {
		fmt.Fprintln(&buffer, text("End of candidates for this page. Publication eligibility is checked again when reading or selecting a reference.", "本页已到候选末尾；读取或选择准确引用时仍会重新核验当前发布资格。"))
	} else {
		fmt.Fprintln(&buffer, text("Continue with the original next_cursor using --cursor; an empty bounded page does not mean this project has no matching knowledge.", "请通过 --cursor 携带原 next_cursor 继续；有界空页不代表项目没有匹配知识。"))
		fmt.Fprintf(&buffer, "next_cursor: %s\n", quoted(result.NextCursor))
	}
	value := buffer.String()
	if token != "" {
		value = strings.ReplaceAll(value, token, "[redacted]")
	}
	_, err := io.WriteString(out, value)
	return err
}

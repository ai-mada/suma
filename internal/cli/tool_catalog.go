package cli

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	toolCatalogUsage           = "tools catalog requires --project and accepts --direction all|suma_to_tool|tool_to_suma and --query TEXT (at most 120 UTF-8 bytes)"
	toolCatalogResponseInvalid = "tool catalog response must preserve the requested project and contain a valid adapter catalog"
)

type toolCatalogPage struct {
	ProjectID string                 `json:"project_id"`
	AsOf      string                 `json:"as_of"`
	Items     []tc.AdapterDescriptor `json:"items"`
}

func toolCatalogCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("tools catalog")
	direction := flags.String("direction", "all", "all, suma_to_tool or tool_to_suma")
	query := flags.String("query", "", "case-insensitive search terms, at most 120 UTF-8 bytes")
	if flags.Parse(args) != nil || flags.NArg() != 0 || c.project == "" ||
		(*direction != "all" && *direction != "suma_to_tool" && *direction != "tool_to_suma") ||
		len(*query) > 120 || !utf8.ValidString(*query) || strings.ContainsFunc(*query, func(r rune) bool { return unicode.IsControl(r) && !unicode.IsSpace(r) }) {
		return usage(toolCatalogUsage)
	}
	// The project catalog accepts no filter parameters. Scope and caller
	// headers remain with the shared client, and all filtering happens here.
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/adapter-catalog", nil, true)
	if err != nil {
		return err
	}
	var page toolCatalogPage
	if strictjson.Decode(raw, &page, 16<<20) != nil || !validToolCatalogPage(page, c.project) {
		return &failure{Code: "response", Message: toolCatalogResponseInvalid}
	}
	terms := strings.Fields(strings.ToLower(*query))
	filtered := page.Items[:0]
	for _, row := range page.Items {
		if (*direction == "all" || row.Direction == *direction) && toolCatalogMatches(row, terms) {
			filtered = append(filtered, row)
		}
	}
	page.Items = filtered
	return writeJSON(out, page, c.token)
}

func validToolCatalogPage(page toolCatalogPage, project string) bool {
	if timestamp, err := time.Parse(time.RFC3339Nano, page.AsOf); err != nil || timestamp.IsZero() || page.ProjectID != project || page.Items == nil || len(page.Items) > 64 {
		return false
	}
	seen := make(map[string]bool, len(page.Items))
	for _, row := range page.Items {
		if row.ID == "" || len(row.ID) > 100 || seen[row.ID] || row.Product == "" || len(row.Product) > 100 || row.EntrypointKind == "" || len(row.Capabilities) > 100 ||
			(row.Direction != "suma_to_tool" && row.Direction != "tool_to_suma") ||
			(row.Implementation != "implemented" && row.Implementation != "guide_only" && row.Implementation != "planned") ||
			(row.Implementation == "guide_only" && row.SetupKind != "manual_guide") {
			return false
		}
		switch row.SetupKind {
		case "manual_guide":
		case "runner_profile":
			if row.Direction != "suma_to_tool" {
				return false
			}
		case "client_export":
			if row.Direction != "tool_to_suma" {
				return false
			}
		default:
			return false
		}
		seen[row.ID] = true
		caps := make(map[string]bool, len(row.Capabilities))
		for _, cap := range row.Capabilities {
			if cap.Key == "" || caps[cap.Key] || (cap.Support != "supported" && cap.Support != "unsupported" && cap.Support != "unverified") {
				return false
			}
			caps[cap.Key] = true
		}
	}
	return true
}

func toolCatalogMatches(row tc.AdapterDescriptor, terms []string) bool {
	searchable := strings.ToLower(strings.Join([]string{row.ID, row.Product, row.EntrypointKind, row.Adapter, row.Protocol}, " "))
	for _, term := range terms {
		if !strings.Contains(searchable, term) {
			return false
		}
	}
	return true
}

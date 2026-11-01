package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
)

const toolReplacementUsage = "tools replace requires --id ORIGINAL_PROFILE --file JSON_FILE|- and explicit --confirm; tools replacements accepts --id PROFILE, --direction outgoing|incoming, --limit 1..100 and the original --cursor; use tools --help"
const toolReplacementInputInvalid = "replacement requires the exact project and original version, complete target profile and worker, explicit pause_original, a create name or exact nested reuse, and the original stable operation key"
const toolReplacementBrowserRequired = "tools replace requires an existing login session and captured original --expected-user; the server verifies current project approval"
const toolReplacementUnconfirmed = "replacement receipt is unconfirmed; preserve the original account, organization, profile ID, JSON payload and operation key; retry only that same confirmed request"
const toolReplacementHistoryInvalid = "replacement history must preserve the exact project, profile, direction, immutable snapshots and original page cursor"

func toolReplacementCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	flags := commandFlags("tools replace")
	id := flags.String("id", "", "exact original tool profile ID")
	file := flags.String("file", "", "original typed JSON file, or - for stdin")
	confirm := flags.Bool("confirm", false, "confirm this exact tool configuration handoff")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !toolDoctorID(*id) || !toolDoctorID(c.project) || *file == "" || !*confirm {
		return usage(toolReplacementUsage)
	}
	// Local presence improves usability, never proves credential kind or rights.
	if c.token == "" || c.expectedUser == "" {
		return usage(toolReplacementBrowserRequired)
	}
	reader := input
	if *file != "-" {
		handle, err := os.Open(*file)
		if err != nil {
			return usage("cannot open JSON file")
		}
		defer handle.Close()
		reader = handle
	}
	if reader == nil {
		return usage(toolReplacementInputInvalid)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, toolMatchesInputLimit+1))
	if err != nil || len(raw) > toolMatchesInputLimit {
		return usage(toolReplacementInputInvalid)
	}
	var request platform.ToolProfileReplacementRequest
	if strictjson.Decode(raw, &request, toolMatchesInputLimit) != nil || request.ProjectID != c.project || !platform.ValidToolProfileReplacementRequest(request, *id) {
		return usage(toolReplacementInputInvalid)
	}
	var originalFields map[string]json.RawMessage
	if json.Unmarshal(raw, &originalFields) != nil {
		return usage(toolReplacementInputInvalid)
	}
	if _, present := originalFields["name"]; request.Reuse != nil && present {
		return usage(toolReplacementInputInvalid)
	}
	var body map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil {
		return usage(toolReplacementInputInvalid)
	}
	// The operator's retained file is the recovery basis. Do not mint a key,
	// refresh identity/version, choose a reuse match or retry this POST here.
	response, err := c.request(ctx, http.MethodPost, "/api/v1/tool-profiles/"+url.PathEscape(*id)+"/replacements", body, true)
	if err != nil {
		var apiFailure *failure
		if errors.As(err, &apiFailure) && apiFailure.Status >= 400 && apiFailure.Status < 500 {
			return err
		}
		return &failure{Code: "unconfirmed", Message: toolReplacementUnconfirmed}
	}
	var receipt platform.ToolProfileReplacement
	if strictjson.Decode(response, &receipt, 16<<20) != nil || !platform.ToolProfileReplacementMatches(receipt, request, *id) || receipt.CreatedBy != c.expectedUser {
		return &failure{Code: "unconfirmed", Message: toolReplacementUnconfirmed}
	}
	return writeJSON(out, response, c.token)
}

func toolReplacementHistoryCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("tools replacements")
	id := flags.String("id", "", "exact tool profile ID")
	direction := flags.String("direction", "outgoing", "outgoing or incoming")
	limit := flags.Int("limit", 20, "page size from 1 to 100")
	cursor := flags.String("cursor", "", "original opaque history cursor")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !toolDoctorID(*id) || !toolDoctorID(c.project) || *direction != "outgoing" && *direction != "incoming" || *limit < 1 || *limit > 100 || !toolReplacementCursor(*cursor) {
		return usage(toolReplacementUsage)
	}
	query := url.Values{"direction": {*direction}, "limit": {strconv.Itoa(*limit)}}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/tool-profiles/"+url.PathEscape(*id)+"/replacements?"+query.Encode(), nil, true)
	if err != nil {
		return err
	}
	var page platform.ToolProfileReplacementList
	if strictjson.Decode(raw, &page, 16<<20) != nil || page.ProjectID != c.project || page.ProfileID != *id || page.Direction != *direction || page.Items == nil || len(page.Items) > *limit || !toolReplacementCursor(page.NextCursor) || len(page.Items) == 0 && page.NextCursor != "" {
		return &failure{Code: "response", Message: toolReplacementHistoryInvalid}
	}
	seen := map[string]bool{}
	for _, item := range page.Items {
		if seen[item.ID] || item.ProjectID != c.project || !platform.ValidToolProfileReplacement(item) || *direction == "outgoing" && item.Original.ID != *id || *direction == "incoming" && item.Replacement.ID != *id {
			return &failure{Code: "response", Message: toolReplacementHistoryInvalid}
		}
		seen[item.ID] = true
	}
	return writeJSON(out, raw, c.token)
}

func toolReplacementCursor(value string) bool {
	return len(value) <= 2048 && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

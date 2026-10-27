package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const compositionCLIRevisionInvalid = "repair revision requires exact plan, task, conflict and selected workflow/tool versions, acknowledgement, reason and original operation key"

type compositionCLIRevision struct {
	ProjectID              string      `json:"project_id"`
	ExpectedVersion        json.Number `json:"expected_version"`
	ExpectedTaskVersion    json.Number `json:"expected_task_version"`
	ExpectedConflictSHA256 string      `json:"expected_conflict_sha256"`
	WorkflowID             string      `json:"workflow_id"`
	WorkflowVersion        json.Number `json:"workflow_version"`
	ProducerNodeID         string      `json:"producer_node_id"`
	ToolProfileID          string      `json:"tool_profile_id"`
	ExpectedToolVersion    json.Number `json:"expected_tool_version"`
	ExpectedRecoverySHA256 string      `json:"expected_recovery_sha256,omitempty"`
	Reason                 string      `json:"reason"`
	Acknowledge            bool        `json:"acknowledge"`
	IdempotencyKey         string      `json:"idempotency_key"`
}

func compositionCLIHash(value string) bool {
	return len(value) == 64 && strings.IndexFunc(value, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) < 0
}

// Preserve original JSON numeric tokens and operation identity. This validates
// transport only: current revision/recovery authority is resolved by the server.
func compositionCLIRevisionObject(decoder *json.Decoder, body map[string]any) bool {
	var q compositionCLIRevision
	if decoder.Decode(&q) != nil || !diagnosisCLIFields(body, "project_id expected_version expected_task_version expected_conflict_sha256 workflow_id workflow_version producer_node_id tool_profile_id expected_tool_version expected_recovery_sha256 reason acknowledge idempotency_key") {
		return false
	}
	for _, field := range []string{"expected_version", "expected_task_version", "workflow_version", "expected_tool_version"} {
		if !compositionCLINumber(body, field, 1) {
			return false
		}
	}
	if _, present := body["expected_recovery_sha256"]; present && !compositionCLIHash(q.ExpectedRecoverySHA256) {
		return false
	}
	return compositionCLIHash(q.ExpectedConflictSHA256) && compositionCLIIdentifier(q.WorkflowID, 100) && compositionCLIIdentifier(q.ProducerNodeID, 150) && compositionCLIIdentifier(q.ToolProfileID, 64) && diagnosisCLIText(q.Reason, 2000) && q.Acknowledge && handoffCLIOperation(q.IdempotencyKey)
}

func taskRunHistoryCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 64) {
		return usage(compositionCLIReadInvalid)
	}
	fs := commandFlags("tasks run-history")
	limit := fs.String("limit", "20", "page size 1..100")
	before := fs.String("before", "", "original scoped next_cursor")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return usage(compositionCLIReadInvalid)
	}
	size, err := strconv.Atoi(*limit)
	if err != nil || size < 1 || size > 100 || strconv.Itoa(size) != *limit || len(*before) > 4096 || !utf8.ValidString(*before) || strings.IndexFunc(*before, unicode.IsControl) >= 0 {
		return usage(compositionCLIReadInvalid)
	}
	query := url.Values{"limit": {*limit}}
	if *before != "" {
		query.Set("before", *before)
	}
	return contextRead(ctx, c, "/api/v1/tasks/"+url.PathEscape(args[0])+"/run-history?"+query.Encode(), out)
}

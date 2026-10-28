package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// Preserve only the progress event's original JSON through the public envelope
// and authorization checks. Remarshalling its map would lose UTF-16 provenance.
func decodeRunnerProgressEnvelope(r *http.Request) (map[string]any, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, invalid("请求无法读取或超过大小限制")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	m, err := decode(r)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, invalid("请求必须是 JSON 对象")
	}
	if event, present := fields["tool_event"]; present {
		m["tool_event"] = event
	}
	if err = preserveHostOperationJSON(raw, m); err != nil {
		return nil, invalid("Invalid Go workspace operation progress")
	}
	return m, nil
}

func decodeRunnerProgressEvent(value any) (*toolcontract.Event, error) {
	if value == nil {
		return nil, nil
	}
	raw, ok := value.(json.RawMessage)
	if !ok {
		return nil, invalid("工具进度必须是受限事件对象")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	event, err := toolcontract.DecodeEvent(raw)
	if errors.Is(err, toolcontract.ErrInvalidDiagnostic) {
		return nil, localization.AnnotateError(fmt.Errorf("%w: %v", dag.ErrInvalid, err), "dag.tool.diagnostic", nil)
	}
	if err != nil {
		return nil, invalid("工具进度必须是受限事件对象")
	}
	return &event, nil
}

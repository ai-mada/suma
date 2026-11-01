package runner

import (
	"context"
	"encoding/json"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/acpstdio"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

// Only native vocabulary differs. The original scoped HTTP, immutable answer,
// before-write poll, durable observations and receipt recovery stay shared.
func cursorToolInputHandler(a Assignment, cfg ExecConfig) acpstdio.CursorInputHandler {
	h := newToolInputHandler(a, cfg)
	requestFor := func(source cu.CursorInputRequest) ic.ToolInputRequest {
		raw, _ := json.Marshal(struct {
			Assignment string
			Source     cu.CursorInputRequest
		}{key(a), source})
		return ic.ToolInputRequest{IdempotencyKey: "cursor-input-" + digest(raw), Protocol: ac.Protocol, Cursor: &source}
	}
	return acpstdio.CursorInputHandler{
		Open: func(ctx context.Context, source cu.CursorInputRequest) (string, error) {
			return h.Open(ctx, requestFor(source))
		},
		Answer: func(ctx context.Context, source cu.CursorInputRequest, id string) (*acpstdio.CursorInputAnswer, error) {
			answer, err := h.Answer(ctx, requestFor(source), id)
			if err != nil {
				return nil, err
			}
			if answer == nil || answer.Cursor == nil {
				return nil, errToolInput
			}
			return &acpstdio.CursorInputAnswer{Decision: *answer.Cursor, AnswerSHA256: answer.AnswerSHA256}, nil
		},
		BeforeWrite: func(ctx context.Context, source cu.CursorInputRequest, id string, reply acpstdio.CursorInputAnswer, o delivery.Observation) error {
			r := requestFor(source)
			cp, err := cfg.Journal.toolInput(a, r)
			if err != nil || cp.InputID != id || cp.Answer == nil || cp.Answer.Cursor == nil || cp.Answer.AnswerSHA256 != reply.AnswerSHA256 || !samePermissionValue(*cp.Answer.Cursor, reply.Decision) {
				return errToolInput
			}
			return h.BeforeWrite(ctx, r, id, *cp.Answer, o)
		},
		Observe: func(ctx context.Context, source cu.CursorInputRequest, id string, o delivery.Observation) error {
			return h.Observe(ctx, requestFor(source), id, o)
		},
	}
}

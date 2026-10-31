package main

import (
	"context"
	"log/slog"
	"time"
)

type cloudTurnLocalDrainer interface {
	CloseCloudTurnRuntimes(context.Context) error
}

// Drain local connections after the HTTP listener exits. This interface grants
// no remote cancellation, reconnect, Prompt or workflow completion capability.
// It also keeps control shutdown bounded while preserving unknown turn facts.
func closeWorkBuddyCloudTurns(connection cloudTurnLocalDrainer, text func(string, string) string, logger *slog.Logger) {
	if connection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := connection.CloseCloudTurnRuntimes(ctx); err != nil && logger != nil {
		logger.Warn(text("本地云会话排空未确认", "Local cloud conversation drain is unconfirmed"), "event", "control.cloud_turn_drain_pending", "error", err)
	}
}

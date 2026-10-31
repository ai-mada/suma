package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type cloudTurnDrainProbe struct {
	calls   int
	bounded bool
	err     error
}

func (p *cloudTurnDrainProbe) CloseCloudTurnRuntimes(ctx context.Context) error {
	p.calls++
	deadline, ok := ctx.Deadline()
	p.bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 10*time.Second
	return p.err
}

func TestControlCloudTurnDrainIsBoundedAndReportsLocalUncertainty(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var output bytes.Buffer
		probe := &cloudTurnDrainProbe{}
		if failed {
			probe.err = errors.New("local drain deadline")
		}
		closeWorkBuddyCloudTurns(probe, func(_, en string) string { return en }, slog.New(slog.NewJSONHandler(&output, nil)))
		if probe.calls != 1 || !probe.bounded {
			t.Fatal("local drain was skipped, duplicated or unbounded")
		}
		if failed && (!strings.Contains(output.String(), "control.cloud_turn_drain_pending") || strings.Contains(output.String(), "remote stopped")) {
			t.Fatal("local uncertainty lost or misreported")
		}
		if !failed && output.Len() != 0 {
			t.Fatal("successful local drain created a warning")
		}
	}
}

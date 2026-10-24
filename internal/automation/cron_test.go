package automation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func cronTime(t *testing.T, raw string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestCronDSTFoldGapAndLordHowe(t *testing.T) {
	for _, test := range []struct{ expr, zone, after, want string }{
		{"30 1 * * *", "America/New_York", "2026-11-01T05:29:00Z", "2026-11-01T05:30:00Z"},
		{"30 1 * * *", "America/New_York", "2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"},
		{"30 2 * * *", "America/New_York", "2026-03-08T00:00:00Z", "2026-03-09T06:30:00Z"},
		{"45 1 * * *", "Australia/Lord_Howe", "2026-04-04T14:45:00Z", "2026-04-04T15:15:00Z"},
		{"15 2 * * *", "Australia/Lord_Howe", "2026-10-03T00:00:00Z", "2026-10-04T15:15:00Z"},
	} {
		t.Run(test.zone+test.after, func(t *testing.T) {
			s, err := Parse(test.expr, test.zone)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := s.Next(cronTime(t, test.after))
			if err != nil || !actual.Equal(cronTime(t, test.want)) {
				t.Fatalf("got %s %v, want %s", actual, err, test.want)
			}
		})
	}
}
func TestCronCalendarORListsRangesStepsAndPrevious(t *testing.T) {
	for _, expression := range []string{"0 9 1,* * 1", "0 9 1,*/1 * 1"} {
		s, err := Parse(expression, "UTC")
		if err != nil {
			t.Fatal(err)
		}
		next, err := s.Next(cronTime(t, "2026-10-20T09:00:00Z"))
		if err != nil || !next.Equal(cronTime(t, "2026-10-26T09:00:00Z")) {
			t.Fatalf("wildcard list must keep weekday restriction: %s => %s %v", expression, next, err)
		}
	}
	s, err := Parse("0,30 9-17/2 13 * 1", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.Next(cronTime(t, "2026-10-12T17:30:00Z"))
	if err != nil || !next.Equal(cronTime(t, "2026-10-13T09:00:00Z")) {
		t.Fatal(next, err)
	}
	next, err = s.Next(cronTime(t, "2026-10-13T09:00:00Z"))
	if err != nil || !next.Equal(cronTime(t, "2026-10-13T09:30:00Z")) {
		t.Fatal(next, err)
	}
	next, err = s.Next(cronTime(t, "2026-10-13T17:30:00Z"))
	if err != nil || !next.Equal(cronTime(t, "2026-10-19T09:00:00Z")) {
		t.Fatal(next, err)
	}
	prev, err := s.Previous(cronTime(t, "2026-10-19T09:01:00Z"))
	if err != nil || !prev.Equal(next) {
		t.Fatal(prev, err)
	}
	s, err = Parse("*/15 * * * 7", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	next, err = s.Next(cronTime(t, "2026-10-24T23:59:59Z"))
	if err != nil || !next.Equal(cronTime(t, "2026-10-25T00:00:00Z")) {
		t.Fatal(next, err)
	}
	s, err = Parse("5/1 * * * *", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	next, err = s.Next(cronTime(t, "2026-10-24T09:05:00Z"))
	if err != nil || !next.Equal(cronTime(t, "2026-10-24T09:06:00Z")) {
		t.Fatal("numeric step one did not continue to field end", next, err)
	}
}
func TestCronValidationSparseBoundAndCancellation(t *testing.T) {
	for _, expr := range []string{"* * * *", "* * * * * *", "@daily", "0 0 31 2 *", "60 * * * *", "*/0 * * * *", "0 24 * * *", "0 0 0 * *", "0 0 * 13 *", "0 0 * * 8", "0 0 * * MON", "0 0 * * 3-1", "+1 * * * *", "1,,2 * * * *"} {
		if _, err := Parse(expr, "UTC"); err == nil {
			t.Errorf("accepted %q", expr)
		}
	}
	if _, err := Parse("* * * * *", "Local"); err == nil {
		t.Fatal("accepted host-dependent timezone")
	}
	if _, err := Parse("* * * * *", "Imaginary/Zone"); err == nil {
		t.Fatal("accepted unknown timezone")
	}
	start := time.Now()
	s, err := Parse("0 0 29 2 *", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.Next(cronTime(t, "2025-03-01T00:00:00Z"))
	if err != nil || !next.Equal(cronTime(t, "2028-02-29T00:00:00Z")) {
		t.Fatal(next, err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("sparse cron exceeded bounded latency: %s", elapsed)
	} else {
		t.Logf("sparse leap-day calculation: %s", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.NextContext(ctx, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled cron calculation did not stop", err)
	}
}

package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	toolInventoryUsage           = "tools inventory requires --project and exactly one --worker WORKER"
	toolInventoryResponseInvalid = "tool inventory response must preserve the requested scope and contain valid observations"
)

func toolInventoryCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("tools inventory")
	var worker string
	flags.Func("worker", "exact execution node ID", func(value string) error {
		if worker != "" || !tc.ValidCommandID(value) {
			return errors.New(toolInventoryUsage)
		}
		worker = value
		return nil
	})
	if flags.Parse(args) != nil || flags.NArg() != 0 || worker == "" || c.project == "" {
		return usage(toolInventoryUsage)
	}
	report, _, err := readToolInventory(ctx, c, worker)
	if err != nil {
		return err
	}
	return writeJSON(out, report, c.token)
}

func readToolInventory(ctx context.Context, c *client, worker string) (platform.ToolDiscovery, time.Time, error) {
	if c.project == "" || !tc.ValidCommandID(worker) {
		return platform.ToolDiscovery{}, time.Time{}, usage(toolInventoryUsage)
	}
	started := time.Now()
	query := url.Values{"worker_id": {worker}}
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/tool-discovery?"+query.Encode(), nil, true)
	if err != nil {
		return platform.ToolDiscovery{}, time.Time{}, err
	}
	var report platform.ToolDiscovery
	invalid := &failure{Code: "response", Message: toolInventoryResponseInvalid}
	if strictjson.Decode(raw, &report, 16<<20) != nil || report.ProjectID != c.project || report.WorkerID != worker ||
		report.Items == nil || len(report.Items) > 128 {
		return platform.ToolDiscovery{}, time.Time{}, invalid
	}
	remaining, ok := toolInventoryRemaining(report)
	if !ok {
		return platform.ToolDiscovery{}, time.Time{}, invalid
	}
	seen := make(map[string]bool, len(report.Items))
	for _, observation := range report.Items {
		command := observation.Profile.CommandID
		if seen[command] || tc.ValidateObservation(observation) != nil {
			return platform.ToolDiscovery{}, time.Time{}, invalid
		}
		seen[command] = true
	}
	if remaining == 0 {
		return report, time.Time{}, nil
	}
	// Use the server's relative lifetime, with the request start as a
	// conservative local monotonic origin for a subsequent matches read.
	return report, started.Add(remaining), nil
}

func toolInventoryRemaining(report platform.ToolDiscovery) (time.Duration, bool) {
	asOf, err := time.Parse(time.RFC3339Nano, report.AsOf)
	if err != nil {
		return 0, false
	}
	noTimestamps := report.ObservedAt == "" && report.ExpiresAt == ""
	switch report.State {
	case "unreported":
		return 0, noTimestamps && len(report.Items) == 0
	case "stale":
		if len(report.Items) != 0 {
			return 0, false
		}
		// The server omits both timestamps for an invalid stored time.
		if noTimestamps {
			return 0, true
		}
	case "fresh":
	default:
		return 0, false
	}
	observed, observedErr := time.Parse(time.RFC3339Nano, report.ObservedAt)
	expires, expiresErr := time.Parse(time.RFC3339Nano, report.ExpiresAt)
	if observedErr != nil || expiresErr != nil || !expires.Equal(observed.Add(60*time.Second)) {
		return 0, false
	}
	inWindow := !asOf.Before(observed) && asOf.Before(expires)
	if report.State == "stale" {
		return 0, !inWindow
	}
	if !inWindow {
		return 0, false
	}
	return expires.Sub(asOf), true
}

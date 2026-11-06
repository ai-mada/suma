package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	toolInventoryUsage           = "tools inventory requires --project and exactly one --worker WORKER"
	toolInventoryResponseInvalid = "tool inventory response must preserve the requested scope and contain valid observations"
	toolInventoryConnectUsage    = "tools inventory connect link requires one valid --connect-command and an exact project ID; use tools --help"
	toolInventoryOriginInvalid   = "--workbench-origin must be an http(s) origin without credentials, path, query, or fragment"
	toolInventoryLinkSensitive   = "the workbench link would expose the current credential"
)

func toolInventoryCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("tools inventory")
	var worker, command, workbenchOrigin string
	flags.Func("worker", "exact execution node ID", func(value string) error {
		if worker != "" || !tc.ValidCommandID(value) {
			return errors.New(toolInventoryUsage)
		}
		worker = value
		return nil
	})
	flags.Func("connect-command", "exact reported command ID for a workbench link", func(value string) error {
		if command != "" || !tc.ValidCommandID(value) {
			return errors.New(toolInventoryConnectUsage)
		}
		command = value
		return nil
	})
	flags.Func("workbench-origin", "explicit browser origin for the connect link", func(value string) error {
		if workbenchOrigin != "" || value == "" {
			return errors.New(toolInventoryOriginInvalid)
		}
		workbenchOrigin = value
		return nil
	})
	if flags.Parse(args) != nil || flags.NArg() != 0 || worker == "" || c.project == "" {
		return usage(toolInventoryUsage)
	}
	if command != "" {
		if !tc.ValidCommandID(c.project) {
			return usage(toolInventoryConnectUsage)
		}
		return toolInventoryConnectLink(ctx, c, worker, command, workbenchOrigin, out)
	}
	if workbenchOrigin != "" {
		return usage(toolInventoryConnectUsage)
	}
	report, _, err := readToolInventory(ctx, c, worker)
	if err != nil {
		return err
	}
	return writeJSON(out, report, c.token)
}

func toolInventoryConnectLink(ctx context.Context, c *client, worker, command, workbenchOrigin string, out io.Writer) error {
	if workbenchOrigin == "" {
		workbenchOrigin = c.base
	}
	origin, err := parseWorkbenchOrigin(workbenchOrigin)
	if err != nil {
		return err
	}
	report, deadline, err := readToolInventory(ctx, c, worker)
	if err != nil {
		return err
	}
	observation, err := selectToolInventoryCommand(report, deadline, command)
	if err != nil {
		return err
	}

	// readToolInventory already checked the timestamp. The browser link is
	// only a public locator; the workbench reads the current report again.
	observedAt, _ := time.Parse(time.RFC3339Nano, report.ObservedAt)
	panel := strings.Join([]string{"tool-inventory-connect", c.project, worker, command,
		observation.Profile.ProfileSHA256, observedAt.UTC().Format(time.RFC3339Nano)}, ":")
	origin.Path = "/"
	origin.RawQuery = url.Values{"project": {c.project}, "view": {"settings"}, "sub": {"tools"}, "panel": {panel}}.Encode()
	link := origin.String()
	if c.token != "" && (strings.Contains(link, c.token) || strings.Contains(panel, c.token)) {
		return &failure{Code: "response", Message: toolInventoryLinkSensitive}
	}
	// Print a directly copyable URL in the terminal as well as valid JSON.
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(map[string]string{"url": link})
}

func selectToolInventoryCommand(report platform.ToolDiscovery, deadline time.Time, command string) (tc.Observation, error) {
	if report.State != "fresh" || !time.Now().Before(deadline) {
		return tc.Observation{}, &failure{Code: "inventory_unusable", Message: toolInventoryMatchUnusable}
	}
	for _, observation := range report.Items {
		if observation.Profile.CommandID != command {
			continue
		}
		if !observation.Available {
			return tc.Observation{}, &failure{Code: "inventory_command_unavailable", Message: toolInventoryMatchUnavailable}
		}
		return observation, nil
	}
	return tc.Observation{}, &failure{Code: "inventory_command_not_found", Message: toolInventoryMatchMissing}
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

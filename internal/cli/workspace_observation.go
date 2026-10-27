package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const workspaceObservationUsage = "workspace requires targets, observe --file JSON_FILE [--wait], or observation ID [--wait]"

// Remote workspace commands use only the authenticated control API. They never
// connect to a worker's filesystem, MCP bearer, terminal or local port directly.
func workspaceObservationCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	u, err := url.Parse(c.base)
	if err != nil || u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return usage(workspaceObservationUsage)
	}
	if len(args) == 1 && args[0] == "targets" {
		return contextRead(ctx, c, "/api/v1/workspace/targets", out)
	}
	if len(args) == 0 {
		return usage(workspaceObservationUsage)
	}
	flags := commandFlags("workspace observation")
	file := flags.String("file", "", "bounded UTF-8 observation JSON")
	wait := flags.Bool("wait", false, "wait at most one request lifetime")
	var current woc.Observation
	if args[0] == "observe" {
		if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *file == "" {
			return usage(workspaceObservationUsage)
		}
		reader := in
		if *file != "-" {
			f, e := os.Open(*file)
			if e != nil {
				return usage("cannot open JSON file")
			}
			defer f.Close()
			reader = f
		}
		if reader == nil {
			return usage("cannot read JSON input")
		}
		raw, e := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
		var request woc.CreateRequest
		if e != nil || woc.Decode(raw, &request) != nil || woc.ValidateScope(request.Scope) != nil || woc.ValidateOperation(request.Operation, request.Arguments, request.ExpectedSHA256) != nil {
			return usage(workspaceObservationUsage)
		}
		if request.ProjectID == "" {
			request.ProjectID = c.project
		}
		if request.ProjectID != c.project {
			return usage("JSON project_id differs from --project")
		}
		encoded, _ := json.Marshal(request)
		d := json.NewDecoder(bytes.NewReader(encoded))
		d.UseNumber()
		body := map[string]any{}
		_ = d.Decode(&body)
		if request.IdempotencyKey == "" {
			delete(body, "idempotency_key")
		}
		if e = c.operation(http.MethodPost, "/api/v1/workspace/observations", body); e != nil {
			return e
		}
		raw, e = c.request(ctx, http.MethodPost, "/api/v1/workspace/observations", body, true)
		if e != nil {
			return e
		}
		if woc.Decode(raw, &current) != nil {
			return &failure{Code: "client", Message: workspaceObservationUsage}
		}
	} else if args[0] == "observation" {
		if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) || flags.Parse(args[2:]) != nil || flags.NArg() != 0 || *file != "" {
			return usage(workspaceObservationUsage)
		}
		raw, e := c.request(ctx, http.MethodGet, "/api/v1/workspace/observations/"+url.PathEscape(args[1]), nil, true)
		if e != nil {
			return e
		}
		if woc.Decode(raw, &current) != nil {
			return &failure{Code: "client", Message: workspaceObservationUsage}
		}
	} else {
		return usage(workspaceObservationUsage)
	}
	deadline := time.Now().Add(time.Duration(woc.RequestTTLSeconds) * time.Second)
	for *wait && (current.State == "requested" || current.State == "leased") && time.Now().Before(deadline) {
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		raw, e := c.request(ctx, http.MethodGet, "/api/v1/workspace/observations/"+url.PathEscape(current.ID), nil, true)
		if e != nil {
			return e
		}
		var next woc.Observation
		if woc.Decode(raw, &next) != nil || next.ID != current.ID || next.Scope != current.Scope {
			return &failure{Code: "client", Message: workspaceObservationUsage}
		}
		current = next
	}
	return writeJSON(out, current, c.token)
}

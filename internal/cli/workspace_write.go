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

	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

const workspaceWriteUsage = "workspace requires save --file JSON [--wait], save-status ID [--wait], or inspect-save ID --expected-version N [--operation KEY] [--wait]"
const workspaceWriteUnconfirmed = "workspace save has no confirmed applied receipt; inspect the original command before another save"

// File text travels only in an explicitly supplied save request. Reads and
// inspection show the original command metadata, never cached source bodies.
func workspaceWriteCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	u, err := url.Parse(c.base)
	if err != nil || u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return usage(workspaceWriteUsage)
	}
	if len(args) == 0 {
		return usage(workspaceWriteUsage)
	}
	flags := commandFlags("workspace save")
	file := flags.String("file", "", "bounded UTF-8 save request JSON")
	wait := flags.Bool("wait", false, "wait for the original command, never retry its effect")
	version := flags.Int("expected-version", 0, "reviewed command version")
	key := flags.String("operation", "", "stable inspection operation key")
	var requestedID string
	var original *wwc.CreateRequest
	var command wwc.Command
	path := "/api/v1/workspace/writes"
	var raw []byte
	if args[0] == "save" {
		if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *file == "" || *version != 0 || *key != "" {
			return usage(workspaceWriteUsage)
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
		raw, err = io.ReadAll(io.LimitReader(reader, (2<<20)+1))
		var request wwc.CreateRequest
		if err != nil || wwc.Decode(raw, &request) != nil {
			return usage(workspaceWriteUsage)
		}
		if request.ProjectID == "" {
			request.ProjectID = c.project
		}
		if request.ProjectID != c.project {
			return usage("JSON project_id differs from --project")
		}
		if request.ContentSHA256 == "" {
			request.ContentSHA256 = wwc.ContentDigest(request.Content)
		}
		if wwc.ValidateCreate(request) != nil {
			return usage(workspaceWriteUsage)
		}
		original = &request
		body := workspaceWriteMap(request)
		if request.IdempotencyKey == "" {
			delete(body, "idempotency_key")
		}
		if err = c.operation(http.MethodPost, path, body); err != nil {
			return err
		}
		raw, err = c.request(ctx, http.MethodPost, path, body, true)
	} else {
		if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) || flags.Parse(args[2:]) != nil || flags.NArg() != 0 || *file != "" {
			return usage(workspaceWriteUsage)
		}
		requestedID = args[1]
		path += "/" + url.PathEscape(args[1])
		switch args[0] {
		case "save-status":
			if *version != 0 || *key != "" {
				return usage(workspaceWriteUsage)
			}
			raw, err = c.request(ctx, http.MethodGet, path, nil, true)
		case "inspect-save":
			if *version < 1 {
				return usage(workspaceWriteUsage)
			}
			body := map[string]any{"project_id": c.project, "expected_version": *version}
			if *key != "" {
				body["idempotency_key"] = *key
			}
			if err = c.operation(http.MethodPost, path+"/inspect", body); err != nil {
				return err
			}
			raw, err = c.request(ctx, http.MethodPost, path+"/inspect", body, true)
		default:
			return usage(workspaceWriteUsage)
		}
	}
	if err != nil {
		return err
	}
	if wwc.Decode(raw, &command) != nil || wwc.ValidateCommand(command) != nil || command.ProjectID != c.project || requestedID != "" && command.ID != requestedID || original != nil && command.RequestSHA256 != wwc.RequestDigest(*original) {
		return &failure{Code: "response", Message: workspaceWriteUnconfirmed}
	}
	id, requestSHA := command.ID, command.RequestSHA256
	deadline := time.Now().Add(time.Duration(wwc.RequestTTLSeconds) * time.Second)
	for *wait && (command.State == "requested" || command.State == "leased" || command.InspectionState == "requested" || command.InspectionState == "leased") && time.Now().Before(deadline) {
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		raw, err = c.request(ctx, http.MethodGet, "/api/v1/workspace/writes/"+url.PathEscape(id), nil, true)
		if err != nil {
			return err
		}
		var next wwc.Command
		if wwc.Decode(raw, &next) != nil || wwc.ValidateCommand(next) != nil || next.ID != id || next.ProjectID != c.project || next.RequestSHA256 != requestSHA {
			return &failure{Code: "response", Message: workspaceWriteUnconfirmed}
		}
		command = next
	}
	if err = writeJSON(out, command, c.token); err != nil {
		return err
	}
	if *wait && command.State != "applied" {
		return &failure{Code: "save_unconfirmed", Message: workspaceWriteUnconfirmed}
	}
	return nil
}

func workspaceWriteMap(value any) map[string]any {
	raw, _ := json.Marshal(value)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	body := map[string]any{}
	_ = d.Decode(&body)
	return body
}

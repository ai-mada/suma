package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/ai-mada/suma/internal/buildinfo"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
)

const help = `suma — 速码 control client

Global flags (before command): --control http://127.0.0.1:4310 --project ID
  --token TOKEN (prefer SUMA_TOKEN) --timeout 15s
  Browser commands: --expected-user ID [--expected-organization ID]
  Capture the original session view; no automatic identity refresh on retries.
  Project-scoped API credentials do not need browser view flags.
  Language: --language > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN

Read commands:
  --version                   Print product release version without an API request
  doctor | projects | requirements | tasks | runs | workflows
  task-owners --help          Preview and atomically change explicit task owners
  task-priority --help        Preview or apply a task priority batch
  task-status --help          Preview or apply a task planning-status batch
  run show ID
  run events ID [--after SEQUENCE]
  run watch RUN [--after SEQUENCE] [--max-events 1..100000]
  run scheduling RUN --node NODE
  run queue [--limit 1..100] [--cursor CURSOR]
  tasks dispatch-options TASK --expected-version N --workflow ID --workflow-version N --node ID
  tasks dispatch-test-option --help   Check an exact independent test tool
  tasks dispatch-plan PLAN
  models options --tool ID --expected-tool-version N
  tools list [--state STATE] [--query TEXT] [--limit 1..200] [--cursor NEXT_CURSOR]
  tools matches --file JSON   Find exact existing profiles using a login session (no writes)
  workspace save-status ID [--wait]
  workspace targets | workspace observe --file request.json [--wait] | workspace observation ID [--wait]
  workspace files [--directory RELATIVE] [--limit 100] [--before CURSOR]
  workspace git | workspace diff --path RELATIVE [--source worktree|index]
  developer check             Verify current MCP tools with a real read-only call
  tui                         Read-only Bubble Tea browser (default on a terminal)

Explicit writes:
  tasks dispatch-preview --file REQUEST_JSON|-
  tasks dispatch-start PLAN --expected-version N --operation KEY
  workspace save --file request.json [--wait]
  workspace inspect-save ID --expected-version N [--operation KEY] [--wait]
  webhook deliver --ingress ID --delivery ID --file event.json [--secret-env SUMA_WEBHOOK_SECRET]
  run command ID --file command.json
  workflow publish --file workflow.json
  run start --workflow ID --version N --operation KEY [--task ID] [--expected-version N]
  api /RESOURCE [--method GET|POST|PUT|PATCH|DELETE] [--file request.json]

Webhook keys are read only from the named process environment; --file - reads stdin.
Webhook delivery preserves exact body bytes and delivery ID; it never sends a session token.
JSON API files retain their idempotency_key or receive a deterministic content key.
Non-terminal default/tui prints doctor JSON and exits. Exit: 0 success, 1 API/transport, 2 usage.
`

// Execute serves both scripts and the terminal UI. All writes require an explicit command.
func Execute(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) int {
	fs := flag.NewFlagSet("suma", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	control := fs.String("control", "http://127.0.0.1:4310", "control origin")
	project := fs.String("project", "", "project ID")
	token := fs.String("token", os.Getenv("SUMA_TOKEN"), "Bearer token")
	expectedUser := fs.String("expected-user", os.Getenv("SUMA_EXPECTED_USER"), "original-view account ID for browser commands")
	expectedOrganization := fs.String("expected-organization", os.Getenv("SUMA_EXPECTED_ORGANIZATION"), "original-view organization ID for browser commands")
	timeout := fs.Duration("timeout", 15*time.Second, "per-request timeout")
	language := fs.String("language", "", "display language: zh-CN or en-US")
	showVersion := fs.Bool("version", false, "print product release version")
	parseErr := fs.Parse(args)
	languageSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "language" {
			languageSet = true
		}
	})
	locale, languageErr := cliLocale(*language, languageSet)
	if languageErr != nil {
		writeJSON(stderr, map[string]any{"error": localizeFailure(locale, &failure{Code: "usage", Message: "--language must be zh-CN or en-US (supported aliases are accepted)"})}, "")
		return 2
	}
	if err := parseErr; err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(out, helpForLocale(locale))
			return 0
		}
		writeJSON(stderr, map[string]any{"error": localizeFailure(locale, &failure{Code: "usage", Message: "invalid global flags; use --help"})}, "")
		return 2
	}
	if *showVersion {
		_, err := io.WriteString(out, "suma "+buildinfo.Version+"\n")
		if err != nil {
			return 1
		}
		return 0
	}
	c, err := newClient(*control, *project, *token, *timeout)
	if err == nil {
		c.locale = locale
		err = c.setOriginalView(*expectedUser, *expectedOrganization)
		if err == nil {
			err = dispatch(ctx, c, fs.Args(), in, out)
		}
	}
	if err == nil {
		return 0
	}
	var f *failure
	if !errors.As(err, &f) {
		f = &failure{Code: "client", Message: "client operation failed"}
	}
	writeJSON(stderr, map[string]any{"error": localizeFailure(locale, f)}, *token)
	if f.Code == "usage" {
		return 2
	}
	return 1
}

func dispatch(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && args[0] == "tools" {
		return toolProfilesCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "worklogs" {
		return worklogsCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "models" {
		return modelExecutionCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "developer" {
		return developerConnectionCommand(ctx, c, args[1:], out)
	}
	if len(args) > 1 && args[0] == "workspace" && (args[1] == "save" || args[1] == "save-status" || args[1] == "inspect-save") {
		return workspaceWriteCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 1 && args[0] == "workspace" && (args[1] == "targets" || args[1] == "observe" || args[1] == "observation") {
		return workspaceObservationCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 1 && args[0] == "workspace" && args[1] == "files" {
		return workspaceTreeCommand(ctx, c, args[2:], out)
	}
	if len(args) > 0 && args[0] == "workspace" {
		return workspaceGitCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "task-views" {
		return sharedTaskViewsCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 1 && args[0] == "tasks" {
		switch args[1] {
		case "dispatch-options":
			return taskDispatchOptionsCommand(ctx, c, args[2:], out)
		case "dispatch-test-option", "dispatch-preview", "dispatch-plan", "dispatch-start":
			return taskDispatchCommand(ctx, c, args[1], args[2:], in, out)
		}
	}
	if len(args) > 0 && args[0] == "inbox" {
		return notificationInboxCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "discussion" {
		return taskDiscussionsCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "review-requests" {
		return taskReviewRequestsCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 1 && args[0] == "tasks" && args[1] == "run-history" {
		return taskRunHistoryCommand(ctx, c, args[2:], out)
	}
	if len(args) > 1 && args[0] == "tasks" && (args[1] == "list" || args[1] == "groups" || args[1] == "help" || args[1] == "--help") {
		return taskCollectionCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "candidate-compositions" {
		return candidateCompositionCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "task-code-baseline" {
		return taskCodeBaselineCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "failure-diagnosis" {
		return failureDiagnosisCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "team-plan" {
		return expertPlansCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "skill-catalog" {
		return skillCatalogCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && (args[0] == "task-priority" || args[0] == "task-status") {
		return taskBatchCommand(ctx, c, args, in, out)
	}
	if len(args) > 0 && args[0] == "task-owners" {
		return taskOwnersCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "handoffs" {
		return handoffsCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "knowledge" {
		return knowledgeCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 1 && args[0] == "requirement" && args[1] == "design" {
		return requirementDesignCommand(ctx, c, args[2:], in, out)
	}
	if len(args) > 0 && args[0] == "webhook" {
		return webhookCommand(ctx, c, args[1:], in, out)
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "tui") {
		if input, ok := in.(*os.File); ok {
			if output, ok := out.(*os.File); ok && isatty.IsTerminal(input.Fd()) && isatty.IsTerminal(output.Fd()) {
				return runTUI(ctx, c, input, output)
			}
		}
		return doctor(ctx, c, out)
	}
	if args[0] == "help" {
		_, err := io.WriteString(out, helpForLocale(c.locale))
		return err
	}
	if args[0] == "doctor" && len(args) == 1 {
		return doctor(ctx, c, out)
	}
	if len(args) == 1 {
		switch args[0] {
		case "projects", "requirements", "tasks", "runs", "workflows":
			raw, err := c.request(ctx, "GET", "/api/v1/"+args[0], nil, args[0] != "projects")
			if err != nil {
				return err
			}
			return writeJSON(out, raw, c.token)
		}
	}
	if args[0] == "run" && len(args) > 1 {
		return runCommand(ctx, c, args[1:], in, out)
	}
	if args[0] == "workflow" && len(args) > 1 && args[1] == "publish" {
		fs := commandFlags("workflow publish")
		file := fs.String("file", "", "JSON file")
		if fs.Parse(args[2:]) != nil || *file == "" || fs.NArg() != 0 {
			return usage("workflow publish requires --file")
		}
		return submitFile(ctx, c, "POST", "/api/v1/workflows", *file, in, out, false)
	}
	if args[0] == "api" {
		return apiCommand(ctx, c, args[1:], in, out)
	}
	return usage("unknown command or arguments; use --help")
}

func commandFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func runCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	switch args[0] {
	case "watch", "scheduling", "queue":
		return runObservationCommand(ctx, c, args[0], args[1:], out)
	case "show", "events", "command":
		if len(args) < 2 || args[1] == "" || strings.ContainsAny(args[1], "/\\?#%") || args[1] == "." || args[1] == ".." {
			return usage("run command requires a valid run ID")
		}
		path := "/api/v1/runs/" + url.PathEscape(args[1])
		fs := commandFlags("run " + args[0])
		after := fs.Int64("after", 0, "event cursor")
		file := fs.String("file", "", "command file")
		if fs.Parse(args[2:]) != nil || fs.NArg() != 0 || *after < 0 {
			return usage("invalid run flags")
		}
		if args[0] == "command" {
			if *file == "" {
				return usage("run command requires --file with action and expected_version")
			}
			return submitFile(ctx, c, "POST", path+"/commands", *file, in, out, true)
		}
		if *file != "" {
			return usage("--file is only supported by run command")
		}
		if args[0] == "events" {
			path += "/events?after=" + strconv.FormatInt(*after, 10)
		}
		raw, err := c.request(ctx, "GET", path, nil, true)
		if err != nil {
			return err
		}
		return writeJSON(out, raw, c.token)
	case "start":
		fs := commandFlags("run start")
		workflow := fs.String("workflow", "", "workflow ID")
		version := fs.Int("version", 0, "workflow version")
		task := fs.String("task", "", "task ID")
		operation := fs.String("operation", "", "stable operation key")
		expected := fs.Int("expected-version", 0, "task version")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *workflow == "" || *version < 1 || len(*operation) < 8 || *expected < 0 {
			return usage("run start requires --workflow, positive --version, and --operation (at least 8 characters)")
		}
		body := map[string]any{"workflow_id": *workflow, "workflow_version": *version, "idempotency_key": *operation}
		if *task != "" {
			body["task_id"] = *task
		}
		if *expected > 0 {
			body["expected_version"] = *expected
		}
		raw, err := c.request(ctx, "POST", "/api/v1/runs", body, true)
		if err != nil {
			return err
		}
		return writeJSON(out, raw, c.token)
	}
	return usage("unknown run command")
}

func submitFile(ctx context.Context, c *client, method, path, file string, in io.Reader, out io.Writer, command bool) error {
	body, err := readObject(file, in)
	if err != nil {
		return err
	}
	if command {
		action, ok := body["action"].(string)
		version, versionOK := body["expected_version"].(json.Number)
		n, versionErr := version.Int64()
		if !ok || action == "" || !versionOK || versionErr != nil || n < 1 {
			return usage("command file requires action and positive integer expected_version")
		}
	}
	if err = c.operation(method, path, body); err != nil {
		return err
	}
	raw, err := c.request(ctx, method, path, body, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func apiCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return usage("api requires a resource path")
	}
	path := args[0]
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "://") {
		return usage("api requires a relative resource path")
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		path = "/api/v1" + path
	}
	fs := commandFlags("api")
	method := fs.String("method", "GET", "HTTP method")
	file := fs.String("file", "", "request JSON")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return usage("invalid api flags")
	}
	*method = strings.ToUpper(*method)
	if *method != "GET" && *method != "POST" && *method != "PUT" && *method != "PATCH" && *method != "DELETE" {
		return usage("unsupported API method")
	}
	var body map[string]any
	if *method != "GET" {
		if *file == "" {
			return usage("API mutations require explicit --method and --file")
		}
		var err error
		body, err = readObject(*file, in)
		if err != nil {
			return err
		}
		if !requestIsReadOnly(*method, path) {
			if err = c.operation(*method, path, body); err != nil {
				return err
			}
		}
	} else if *file != "" {
		return usage("GET does not accept --file")
	}
	scoped := path != "/api/v1/projects" && !strings.HasPrefix(path, "/api/v1/auth/")
	raw, err := c.request(ctx, *method, path, body, scoped)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func doctor(ctx context.Context, c *client, out io.Writer) error {
	report := map[string]any{"control": c.base, "project_id": c.project, "token_present": c.token != ""}
	var first error
	for _, check := range []struct {
		name, path string
		scope      bool
	}{{"health", "/healthz", false}, {"session", "/api/v1/auth/session", false}, {"capabilities", "/api/v1/capabilities", true}} {
		if check.scope && c.project == "" {
			report[check.name] = map[string]string{"status": "not_checked", "reason": c.text("请提供 --project", "provide --project")}
			continue
		}
		raw, err := c.request(ctx, "GET", check.path, nil, check.scope)
		if err != nil {
			if first == nil {
				first = err
			}
			var f *failure
			errors.As(err, &f)
			report[check.name] = map[string]any{"error": localizeFailure(c.locale, f)}
		} else {
			report[check.name] = raw
		}
	}
	if err := writeJSON(out, report, c.token); err != nil {
		return err
	}
	return first
}

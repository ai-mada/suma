package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/ai-mada/suma/internal/runner"
	"log"
	"os"
	"os/signal"
	"path/filepath"
)

func main() {
	base := flag.String("control", "http://127.0.0.1:4310", "control service URL")
	project := flag.String("project", "", "project ID")
	id := flag.String("worker", "", "stable worker ID")
	workspace := flag.String("workspace", ".", "execution workspace root")
	repository := flag.String("repository", "", "administrator Git checkout root for write commands")
	baseCommit := flag.String("base-commit", "", "full immutable Git commit for isolated attempts")
	worktrees := flag.String("worktrees", "", "attempt directory under repository .cache (default .cache/runner/worktrees)")
	preflight := flag.String("merge-preflight", "", "reviewed local JSON request; validate evidence and heads without merging")
	state := flag.String("state", ".cache/runner", "durable runner state directory")
	commands := flag.String("commands", "", "local JSON command allowlist")
	externalConfig := flag.String("external-config", "", "administrator JSON map of external workflow plugin endpoints and token environment names")
	allow := flag.Bool("allow-exec", false, "enable execution of local allowlisted commands")
	describe := flag.Bool("describe-tools", false, "read-only local executable/version/protocol observations; no platform token or model request")
	poolConfig := flag.String("pool-config", "", "private JSON configuration of an independent pool machine and per-project execution scopes")
	flag.Parse()
	if *poolConfig != "" {
		if *project != "" || *id != "" || *commands != "" || *repository != "" || *baseCommit != "" || *worktrees != "" || *externalConfig != "" || *preflight != "" || (!*describe && os.Getenv("SUMA_RUNNER_TOKEN") != "") {
			log.Fatal("pool mode is independent of --project/--worker and project-runner configuration or credentials")
		}
		if err := runPool(*base, *poolConfig, *state, *allow, *describe); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *preflight != "" {
		b, e := os.ReadFile(*preflight)
		if e != nil {
			log.Fatal(e)
		}
		var request runner.MergePreflightRequest
		if e = json.Unmarshal(b, &request); e != nil {
			log.Fatal(e)
		}
		result, e := runner.MergePreflight(context.Background(), runner.ExecConfig{Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees}, request)
		if e != nil {
			log.Fatal(e)
		}
		if e = json.NewEncoder(os.Stdout).Encode(result); e != nil {
			log.Fatal(e)
		}
		return
	}
	root, err := filepath.Abs(*workspace)
	if err != nil {
		log.Fatal(err)
	}
	specs := map[string]runner.CommandSpec{}
	if *commands != "" {
		b, e := os.ReadFile(*commands)
		if e != nil {
			log.Fatal(e)
		}
		if specs, e = runner.DecodeCommands(b); e != nil {
			log.Fatal(e)
		}
	}
	if err = runner.ValidateToolConfiguration(runner.ExecConfig{Commands: specs}); err != nil {
		log.Fatal(err)
	}
	if *describe {
		items, describeErr := runner.DescribeTools(context.Background(), runner.ExecConfig{AllowExec: *allow, Commands: specs, Workspace: root, Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees})
		if describeErr != nil {
			log.Fatal(describeErr)
		}
		if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"items": items, "execution_enabled": *allow}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *project == "" || *id == "" || os.Getenv("SUMA_RUNNER_TOKEN") == "" {
		log.Fatal("--project, --worker and SUMA_RUNNER_TOKEN are required; worker must have a registered execution credential")
	}
	journal, err := runner.OpenJournal(*state)
	if err != nil {
		log.Fatal(err)
	}
	defer journal.Close()
	external := map[string]runner.ExternalSpec{}
	if *externalConfig != "" {
		b, e := os.ReadFile(*externalConfig)
		if e != nil {
			log.Fatal(e)
		}
		if e = json.Unmarshal(b, &external); e != nil {
			log.Fatal(e)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	w := runner.Worker{Client: &runner.Client{BaseURL: *base, Token: os.Getenv("SUMA_RUNNER_TOKEN"), ProjectID: *project}, Journal: journal, ID: *id, Exec: runner.ExecConfig{AllowExec: *allow, Workspace: root, Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees, Commands: specs, External: external, Secrets: []string{os.Getenv("SUMA_RUNNER_TOKEN")}}}
	if err = w.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

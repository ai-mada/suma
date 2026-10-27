package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

var version = "1.0.0"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }

func run(args []string, out, stderr io.Writer, lookup func(string) (string, bool)) int {
	app := commandlocale.New("suma-runner", "速码执行器", "suma execution worker", out, stderr, lookup)
	base := app.String("control", "http://127.0.0.1:4310", "控制面服务地址", "control service URL")
	project := app.String("project", "", "项目标识", "project ID")
	id := app.String("worker", "", "稳定的执行器标识", "stable worker ID")
	workspace := app.String("workspace", ".", "执行工作区根目录", "execution workspace root")
	repository := app.String("repository", "", "管理员 Git 检出根目录，供写命令使用", "administrator Git checkout root for write commands")
	baseCommit := app.String("base-commit", "", "隔离尝试使用的完整不可变 Git 提交", "full immutable Git commit for isolated attempts")
	worktrees := app.String("worktrees", "", "仓库 .cache 下的尝试目录（默认 .cache/runner/worktrees）", "attempt directory under repository .cache (default .cache/runner/worktrees)")
	preflight := app.String("merge-preflight", "", "已审核的本地 JSON 请求；只验证证据和提交，不执行合并", "reviewed local JSON request; validate evidence and heads without merging")
	state := app.String("state", ".cache/runner", "执行器持久状态目录", "durable runner state directory")
	commands := app.String("commands", "", "本地 JSON 命令允许列表", "local JSON command allowlist")
	modelDeployments := app.String("model-deployments", "", "管理员本地模型部署 JSON 允许列表；凭证只使用环境变量引用", "administrator local model deployment JSON allowlist; credentials use environment references")
	externalConfig := app.String("external-config", "", "管理员 JSON 配置：外部工作流插件端点和凭证环境变量名", "administrator JSON map of external workflow plugin endpoints and token environment names")
	allow := app.Bool("allow-exec", false, "允许执行本地允许列表中的命令", "enable execution of local allowlisted commands")
	describe := app.Bool("describe-tools", false, "只读探测本地程序、版本及协议；不读取平台凭证或请求模型", "read-only local executable/version/protocol observations; no platform token or model request")
	poolConfig := app.String("pool-config", "", "独立资源池机器及各项目执行范围的私有 JSON 配置", "private JSON configuration of an independent pool machine and per-project execution scopes")
	if stop, code := app.Parse(args, version); stop {
		return code
	}
	// Errors from runner, the OS and JSON decoding retain their original text.
	fail := func(err error) int { fmt.Fprintln(stderr, err); return 1 }
	if *poolConfig != "" {
		if *project != "" || *id != "" || *commands != "" || *modelDeployments != "" || *repository != "" || *baseCommit != "" || *worktrees != "" || *externalConfig != "" || *preflight != "" || (!*describe && app.Getenv("SUMA_RUNNER_TOKEN") != "") {
			return app.Failure("资源池模式不能与 --project/--worker、项目执行器配置或凭证混用", "pool mode is independent of --project/--worker and project-runner configuration or credentials")
		}
		if err := runPool(app, *base, *poolConfig, *state, *allow, *describe); err != nil {
			return fail(err)
		}
		return 0
	}
	if *preflight != "" {
		b, e := os.ReadFile(*preflight)
		if e != nil {
			return fail(e)
		}
		var request runner.MergePreflightRequest
		if e = json.Unmarshal(b, &request); e != nil {
			return fail(e)
		}
		result, e := runner.MergePreflight(context.Background(), runner.ExecConfig{Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees}, request)
		if e != nil {
			return fail(e)
		}
		if e = json.NewEncoder(out).Encode(result); e != nil {
			return fail(e)
		}
		return 0
	}
	root, err := filepath.Abs(*workspace)
	if err != nil {
		return fail(err)
	}
	specs := map[string]runner.CommandSpec{}
	localModels := []runner.ModelDeploymentSpec{}
	if *modelDeployments != "" {
		raw, readErr := os.ReadFile(*modelDeployments)
		if readErr != nil {
			return fail(readErr)
		}
		localModels, err = runner.DecodeModelDeployments(raw)
		if err != nil {
			return fail(err)
		}
	}
	if *commands != "" {
		b, e := os.ReadFile(*commands)
		if e != nil {
			return fail(e)
		}
		if specs, e = runner.DecodeCommands(b); e != nil {
			return fail(e)
		}
	}
	if err = runner.ValidateToolConfiguration(runner.ExecConfig{Commands: specs, ModelDeployments: localModels}); err != nil {
		return fail(err)
	}
	if *describe {
		items, describeErr := runner.DescribeTools(context.Background(), runner.ExecConfig{AllowExec: *allow, Commands: specs, Workspace: root, Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees, ModelDeployments: localModels})
		if describeErr != nil {
			return fail(describeErr)
		}
		if err = json.NewEncoder(out).Encode(map[string]any{"items": items, "execution_enabled": *allow}); err != nil {
			return fail(err)
		}
		return 0
	}
	if *project == "" || *id == "" || app.Getenv("SUMA_RUNNER_TOKEN") == "" {
		return app.Failure("必须提供 --project、--worker 和 SUMA_RUNNER_TOKEN；执行器须持有已登记的执行凭证", "--project, --worker and SUMA_RUNNER_TOKEN are required; worker must have a registered execution credential")
	}
	journal, err := runner.OpenJournal(*state)
	if err != nil {
		return fail(err)
	}
	defer journal.Close()
	external := map[string]runner.ExternalSpec{}
	if *externalConfig != "" {
		b, e := os.ReadFile(*externalConfig)
		if e != nil {
			return fail(e)
		}
		if e = json.Unmarshal(b, &external); e != nil {
			return fail(e)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	w := runner.Worker{Client: &runner.Client{BaseURL: *base, Token: app.Getenv("SUMA_RUNNER_TOKEN"), ProjectID: *project}, Journal: journal, ID: *id, Exec: runner.ExecConfig{AllowExec: *allow, Workspace: root, Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees, Commands: specs, ModelDeployments: localModels, External: external, Secrets: []string{app.Getenv("SUMA_RUNNER_TOKEN")}}}
	if err = w.Run(ctx); err != nil && ctx.Err() == nil {
		return fail(err)
	}
	return 0
}
